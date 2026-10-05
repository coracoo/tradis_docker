package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dockerpanel/backend/pkg/composehistory"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/deployment"
	"dockerpanel/backend/pkg/logging"

	"github.com/gin-gonic/gin"
)

const localComposeHistoryEnvironment = "local"

var composeHistoryReplaceFiles = composehistory.ReplaceFiles

type composeConfigCandidateRequest struct {
	YAML string `json:"yaml"`
	Env  string `json:"env"`
}

type composeConfigApplyRequest struct {
	YAML     string `json:"yaml"`
	Env      string `json:"env"`
	BaseHash string `json:"baseHash"`
}

type composeHistoryRestoreRequest struct {
	BaseHash string `json:"baseHash"`
}

type composeHistoryItem struct {
	ID          string                       `json:"id"`
	Current     bool                         `json:"current"`
	YAMLHash    string                       `json:"yamlHash"`
	ComposePath string                       `json:"composePath"`
	Source      string                       `json:"source"`
	CreatedAt   string                       `json:"createdAt"`
	Summary     composehistory.ChangeSummary `json:"summary"`
}

type composeHistoryListResponse struct {
	Current composeHistoryItem   `json:"current"`
	Items   []composeHistoryItem `json:"items"`
}

type loadedComposeConfig struct {
	ProjectDir   string
	YAMLPath     string
	RelativePath string
	EnvPath      string
	YAML         string
	Env          string
	ModifiedAt   time.Time
}

func previewComposeConfig(c *gin.Context) {
	target, ok := prepareComposeHistoryRequest(c)
	if !ok {
		return
	}
	var request composeConfigCandidateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "无效的配置预览请求", err)
		return
	}
	if strings.TrimSpace(request.YAML) == "" {
		respondError(c, http.StatusBadRequest, "Compose YAML 不能为空", nil)
		return
	}
	current, err := loadComposeConfigTarget(target)
	if err != nil {
		respondComposeHistoryLoadError(c, err)
		return
	}
	preview, err := composehistory.BuildPreview(current.YAML, normalizeConfigText(request.YAML), current.Env, normalizeConfigText(request.Env))
	if err != nil {
		respondError(c, http.StatusBadRequest, "无法生成配置变更预览", err)
		return
	}
	c.JSON(http.StatusOK, preview)
}

func applyComposeConfig(c *gin.Context) {
	target, ok := prepareComposeHistoryRequest(c)
	if !ok {
		return
	}
	var request composeConfigApplyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "无效的配置保存请求", err)
		return
	}
	request.YAML = normalizeConfigText(request.YAML)
	request.Env = normalizeConfigText(request.Env)
	request.BaseHash = strings.TrimSpace(request.BaseHash)
	if strings.TrimSpace(request.YAML) == "" || request.BaseHash == "" {
		respondError(c, http.StatusBadRequest, "Compose YAML 和预览基准不能为空", nil)
		return
	}

	name := target.DisplayName
	unlock := lockComposeProjectMutation(target.ComposeProjectName)
	defer unlock()
	current, err := loadComposeConfigTarget(target)
	if err != nil {
		respondComposeHistoryLoadError(c, err)
		return
	}
	preview, err := composehistory.BuildPreview(current.YAML, request.YAML, current.Env, request.Env)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无法验证配置变更", err)
		return
	}
	if preview.BaseHash != request.BaseHash {
		respondComposeConfigConflict(c)
		return
	}
	if !preview.HasChanges {
		c.JSON(http.StatusOK, gin.H{
			"changed": false, "yamlChanged": false, "envChanged": false,
			"historyCreated": false, "currentHash": preview.BaseHash,
			"message": "配置未发生变化",
		})
		return
	}

	historyInserted, err := persistComposeConfigChange(name, current, preview, request.YAML, request.Env, "manual_edit")
	if err != nil {
		respondError(c, http.StatusInternalServerError, "保存 Compose 配置失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"changed": true, "yamlChanged": preview.YAMLChanged, "envChanged": preview.EnvChanged,
		"historyCreated": historyInserted, "currentHash": preview.CandidateHash,
		"message": "配置已保存",
	})
}

// Callers hold the project mutation lock and check their own preview baseline.
func persistComposeConfigChange(name string, current loadedComposeConfig, preview composehistory.Preview, candidateYAML, candidateEnv, source string, extra ...composehistory.FileUpdate) (bool, error) {
	historyID, historyInserted, err := persistCurrentComposeSnapshot(name, current, preview, source)
	if err != nil {
		return false, fmt.Errorf("保存当前配置历史失败: %w", err)
	}
	updates := make([]composehistory.FileUpdate, 0, 2)
	if preview.YAMLChanged {
		updates = append(updates, composehistory.FileUpdate{Path: current.YAMLPath, Content: []byte(candidateYAML), Mode: 0644})
	}
	if preview.EnvChanged {
		updates = append(updates, composehistory.FileUpdate{Path: current.EnvPath, Content: []byte(candidateEnv), Mode: 0600})
	}
	updates = append(updates, extra...)
	if err := composeHistoryReplaceFiles(updates); err != nil {
		if historyInserted {
			_ = database.DeleteComposeHistory(localComposeHistoryEnvironment, name, historyID)
		}
		return false, fmt.Errorf("保存 Compose 配置失败，原配置已恢复: %w", err)
	}
	pruneComposeHistory(name)
	invalidateComposeProjectListCache()
	logging.Info("Compose configuration applied",
		"project", name, "history_id", historyID, "change_count", preview.Summary.Total,
		"yaml_changed", preview.YAMLChanged, "env_changed", preview.EnvChanged)
	return historyInserted, nil
}

// Include dotenv and file location: YAML-only hashes cannot detect an env edit
// or switching to a different Compose file between approval and execution.
func composeConfigFingerprint(current loadedComposeConfig) string {
	raw, _ := json.Marshal([]string{current.YAMLPath, current.YAML, current.Env})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func verifyComposeConfigFiles(root string, digests map[string]string) error {
	for relative, expected := range digests {
		path, err := deployment.ResolveWithinRoot(root, relative)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
			return errors.New("附加配置文件缺失、类型错误或超限，请重新预览")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(raw)) != expected {
			return errors.New("附加配置文件已变化，请重新预览")
		}
	}
	return nil
}

func listComposeConfigHistory(c *gin.Context) {
	target, ok := prepareComposeHistoryRequest(c)
	if !ok {
		return
	}
	name := target.DisplayName
	current, err := loadComposeConfigTarget(target)
	if err != nil {
		respondComposeHistoryLoadError(c, err)
		return
	}
	currentHash, err := composehistory.YAMLHash(current.YAML)
	if err != nil {
		respondError(c, http.StatusBadRequest, "当前 Compose YAML 无效", err)
		return
	}
	records, err := database.ListComposeHistory(localComposeHistoryEnvironment, name, composehistory.RetentionLimit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "读取配置历史失败", err)
		return
	}
	items := make([]composeHistoryItem, 0, len(records))
	for _, record := range records {
		var summary composehistory.ChangeSummary
		_ = json.Unmarshal([]byte(record.SummaryJSON), &summary)
		items = append(items, composeHistoryItem{
			ID: record.ID, YAMLHash: record.YAMLHash, ComposePath: record.ComposePath,
			Source: record.Source, CreatedAt: record.CreatedAt, Summary: summary,
		})
	}
	c.JSON(http.StatusOK, composeHistoryListResponse{
		Current: composeHistoryItem{
			ID: "current", Current: true, YAMLHash: currentHash, ComposePath: current.RelativePath,
			Source: "current", CreatedAt: current.ModifiedAt.Format(time.RFC3339),
		},
		Items: items,
	})
}

func previewComposeHistoryRestore(c *gin.Context) {
	target, ok := prepareComposeHistoryRequest(c)
	if !ok {
		return
	}
	current, historicalYAML, ok := loadHistoricalComposeCandidate(c, target)
	if !ok {
		return
	}
	preview, err := composehistory.BuildPreview(current.YAML, historicalYAML, current.Env, current.Env)
	if err != nil {
		respondError(c, http.StatusBadRequest, "历史配置无效，无法恢复", err)
		return
	}
	c.JSON(http.StatusOK, preview)
}

func restoreComposeHistory(c *gin.Context) {
	target, ok := prepareComposeHistoryRequest(c)
	if !ok {
		return
	}
	var request composeHistoryRestoreRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "无效的历史恢复请求", err)
		return
	}
	request.BaseHash = strings.TrimSpace(request.BaseHash)
	if request.BaseHash == "" {
		respondError(c, http.StatusBadRequest, "恢复预览基准不能为空", nil)
		return
	}

	name := target.DisplayName
	unlock := lockComposeProjectMutation(target.ComposeProjectName)
	defer unlock()
	current, historicalYAML, ok := loadHistoricalComposeCandidate(c, target)
	if !ok {
		return
	}
	preview, err := composehistory.BuildPreview(current.YAML, historicalYAML, current.Env, current.Env)
	if err != nil {
		respondError(c, http.StatusBadRequest, "历史配置无效，无法恢复", err)
		return
	}
	if preview.BaseHash != request.BaseHash {
		respondComposeConfigConflict(c)
		return
	}
	if !preview.YAMLChanged {
		c.JSON(http.StatusOK, gin.H{"restored": false, "deployed": false, "message": "当前配置与所选历史一致"})
		return
	}

	historyID, historyInserted, err := persistCurrentComposeSnapshot(name, current, preview, "history_restore")
	if err != nil {
		respondError(c, http.StatusInternalServerError, "保存恢复前配置历史失败", err)
		return
	}
	if err := composeHistoryReplaceFiles([]composehistory.FileUpdate{{
		Path: current.YAMLPath, Content: []byte(historicalYAML), Mode: 0644,
	}}); err != nil {
		if historyInserted {
			_ = database.DeleteComposeHistory(localComposeHistoryEnvironment, name, historyID)
		}
		respondError(c, http.StatusInternalServerError, "恢复配置失败，当前配置已保留", err)
		return
	}
	pruneComposeHistory(name)
	invalidateComposeProjectListCache()
	logging.Info("Compose configuration restored",
		"project", name, "history_id", c.Param("historyId"), "snapshot_id", historyID,
		"change_count", preview.Summary.Total)
	c.JSON(http.StatusOK, gin.H{"restored": true, "deployed": false, "message": "配置已恢复，尚未部署"})
}

func prepareComposeHistoryRequest(c *gin.Context) (composeOperationTarget, bool) {
	return resolveComposeRequestTarget(c)
}

func loadComposeConfig(reference string) (loadedComposeConfig, error) {
	target, err := resolveComposeOperationTarget(context.Background(), reference)
	if err != nil {
		return loadedComposeConfig{}, err
	}
	return loadComposeConfigTarget(target)
}

func loadComposeConfigTarget(target composeOperationTarget) (loadedComposeConfig, error) {
	projectDir := target.ProjectDir
	if info, statErr := os.Stat(projectDir); statErr != nil || !info.IsDir() {
		if statErr == nil {
			statErr = fmt.Errorf("项目路径不是目录")
		}
		return loadedComposeConfig{}, fmt.Errorf("项目目录不存在: %w", statErr)
	}
	yamlPath, err := findComposeFile(projectDir)
	if err != nil {
		return loadedComposeConfig{}, fmt.Errorf("未找到 Compose YAML: %w", err)
	}
	yamlContent, err := os.ReadFile(yamlPath)
	if err != nil {
		return loadedComposeConfig{}, fmt.Errorf("读取 Compose YAML 失败: %w", err)
	}
	info, err := os.Stat(yamlPath)
	if err != nil {
		return loadedComposeConfig{}, fmt.Errorf("读取 Compose YAML 状态失败: %w", err)
	}
	relativePath, err := filepath.Rel(projectDir, yamlPath)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return loadedComposeConfig{}, fmt.Errorf("Compose YAML 路径超出项目目录")
	}
	envPath := filepath.Join(projectDir, ".env")
	envContent, err := os.ReadFile(envPath)
	if err != nil && !os.IsNotExist(err) {
		return loadedComposeConfig{}, fmt.Errorf("读取 .env 失败: %w", err)
	}
	return loadedComposeConfig{
		ProjectDir: projectDir, YAMLPath: yamlPath, RelativePath: filepath.ToSlash(relativePath),
		EnvPath: envPath, YAML: normalizeConfigText(string(yamlContent)), Env: normalizeConfigText(string(envContent)),
		ModifiedAt: info.ModTime(),
	}, nil
}

func loadHistoricalComposeCandidate(c *gin.Context, target composeOperationTarget) (loadedComposeConfig, string, bool) {
	name := target.DisplayName
	historyID := strings.TrimSpace(c.Param("historyId"))
	if historyID == "" || historyID == "current" {
		respondError(c, http.StatusBadRequest, "历史记录 ID 无效", nil)
		return loadedComposeConfig{}, "", false
	}
	current, err := loadComposeConfigTarget(target)
	if err != nil {
		respondComposeHistoryLoadError(c, err)
		return loadedComposeConfig{}, "", false
	}
	record, err := database.GetComposeHistory(localComposeHistoryEnvironment, name, historyID)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(c, http.StatusNotFound, "配置历史不存在", nil)
		return loadedComposeConfig{}, "", false
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "读取配置历史失败", err)
		return loadedComposeConfig{}, "", false
	}
	historicalYAML, err := composehistory.OpenSnapshot(record.SnapshotSealed, localComposeHistoryEnvironment, name, historyID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "配置历史不可用，无法解密", nil)
		return loadedComposeConfig{}, "", false
	}
	historicalYAML = normalizeConfigText(historicalYAML)
	if _, err := composehistory.YAMLHash(historicalYAML); err != nil {
		respondError(c, http.StatusBadRequest, "历史配置无效，无法恢复", err)
		return loadedComposeConfig{}, "", false
	}
	return current, historicalYAML, true
}

func persistCurrentComposeSnapshot(name string, current loadedComposeConfig, preview composehistory.Preview, source string) (string, bool, error) {
	if !preview.YAMLChanged {
		return "", false, nil
	}
	historyID := composehistory.NewHistoryID()
	sealed, err := composehistory.SealSnapshot(current.YAML, localComposeHistoryEnvironment, name, historyID)
	if err != nil {
		return "", false, err
	}
	summaryJSON, err := json.Marshal(preview.Summary)
	if err != nil {
		return "", false, err
	}
	inserted, err := database.SaveComposeHistory(database.ComposeHistoryRecord{
		ID: historyID, EnvironmentID: localComposeHistoryEnvironment, ProjectName: name,
		ComposePath: current.RelativePath, YAMLHash: preview.BaseHash, SnapshotSealed: sealed,
		SummaryJSON: string(summaryJSON), Source: source,
	}, composehistory.RetentionLimit+1)
	return historyID, inserted, err
}

func pruneComposeHistory(name string) {
	if err := database.PruneComposeHistory(localComposeHistoryEnvironment, name, composehistory.RetentionLimit); err != nil {
		logging.Warn("Compose history retention cleanup failed", "project", name, "error", err)
	}
}

func respondComposeConfigConflict(c *gin.Context) {
	c.JSON(http.StatusConflict, gin.H{
		"code": "compose_config_conflict", "message": "配置已被修改，请重新预览后再保存",
		"error": "配置已被修改，请重新预览后再保存",
	})
}

func respondComposeHistoryLoadError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "不存在") || strings.Contains(err.Error(), "未找到") {
		respondError(c, http.StatusNotFound, "Compose 项目或配置不存在", nil)
		return
	}
	respondError(c, http.StatusInternalServerError, "读取 Compose 配置失败", err)
}

func normalizeConfigText(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}
