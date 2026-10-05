package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"dockerpanel/backend/internal/templatecompiler"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/deployment"
	"dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/logging"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

type DeployRequest struct {
	Compose                string                            `json:"compose"`
	Env                    map[string]string                 `json:"env"`
	Dotenv                 string                            `json:"dotenv"`
	Config                 []Variable                        `json:"config"`
	Secrets                map[string]string                 `json:"secrets"`
	ProjectName            string                            `json:"projectName"`
	ManifestDigest         string                            `json:"manifestDigest,omitempty"` // Legacy alias for unedited source.
	BaseManifestDigest     string                            `json:"baseManifestDigest,omitempty"`
	OverrideManifestDigest string                            `json:"overrideManifestDigest,omitempty"`
	ValuesByInputID        map[string]string                 `json:"valuesByInputId,omitempty"`
	MappingOverlays        []templatecompiler.MappingOverlay `json:"mappingOverlays,omitempty"`
	SourceOverride         *templatecompiler.SourceBundle    `json:"sourceOverride,omitempty"`
	PreflightToken         string                            `json:"preflightToken,omitempty"`
	IdempotencyKey         string                            `json:"idempotencyKey,omitempty"`
}

var appStoreComposeCommand = runDockerCombinedOutput
var appStoreFinishTask = database.FinishTask

func finishAppStoreTask(taskID, status string, result any, detail string) error {
	// Retry only the database write, never the Docker operation.
	if err := appStoreFinishTask(taskID, status, result, detail); err != nil {
		if retryErr := appStoreFinishTask(taskID, status, result, detail); retryErr != nil {
			return &deployment.TerminalPersistenceError{Status: status, Result: result, ErrorText: detail, Err: retryErr}
		}
	}
	return nil
}

func verifyAppStoreDeployment(ctx context.Context, project string) (deployment.VerificationResult, error) {
	result, err := composeDeploymentVerifier(ctx, project)
	if err == nil && !result.Verified {
		err = fmt.Errorf("部署验证未通过")
	}
	return result, err
}

func rollbackAppStoreDeployment(composeDir string, commandEnv []string) error {
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	output, err := appStoreComposeCommand(rollbackCtx, composeDir, commandEnv, []string{"compose", "down"})
	if err != nil {
		detail := strings.TrimSpace(logging.RedactText(string(output)))
		if len(detail) > 500 {
			detail = detail[:500] + "..."
		}
		if detail != "" {
			return fmt.Errorf("Compose 资源清理失败: %w (%s)", err, detail)
		}
		return fmt.Errorf("Compose 资源清理失败: %w", err)
	}
	return nil
}

func normalizeProjectName(name string) string {
	lower := strings.ToLower(name)
	buf := make([]rune, 0, len(lower))
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			buf = append(buf, r)
		}
	}
	if len(buf) == 0 {
		return "project"
	}

	out := string(buf)
	out = strings.TrimLeftFunc(out, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	if out == "" {
		return "project"
	}
	return out
}

func reserveAppStoreProjectDir(root, baseName string) (string, string, error) {
	baseName = normalizeProjectName(baseName)
	if _, ok := validateComposeProjectName(baseName); !ok {
		return "", "", fmt.Errorf("应用默认项目名无效")
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", "", err
	}
	for suffix := 0; suffix < 10000; suffix++ {
		candidate := baseName
		if suffix > 0 {
			candidate = fmt.Sprintf("%s_%d", baseName, suffix)
		}
		projectDir, err := resolveProjectDir(root, candidate)
		if err != nil {
			return "", "", err
		}
		if err := os.Mkdir(projectDir, 0755); err == nil {
			return candidate, projectDir, nil
		} else if os.IsExist(err) {
			continue
		} else {
			return "", "", err
		}
	}
	return "", "", fmt.Errorf("无法为项目 %s 分配可用目录", baseName)
}

func claimLocalAppStoreTask(appID string, request DeployRequest) (string, bool, error) {
	appID = strings.TrimSpace(appID)
	idempotencyKey := strings.TrimSpace(request.IdempotencyKey)
	if idempotencyKey == "" {
		taskID := fmt.Sprintf("%d", time.Now().UnixNano())
		if err := database.UpsertTask(taskID, "appstore_deploy", "pending"); err != nil {
			return "", false, err
		}
		return taskID, true, nil
	}
	if len(idempotencyKey) > 256 {
		return "", false, fmt.Errorf("部署幂等键过长")
	}
	digest := sha256.Sum256([]byte(appID + "\x00" + idempotencyKey))
	taskID := fmt.Sprintf("appstore-local-%x", digest[:16])
	request.IdempotencyKey = ""
	body, err := json.Marshal(request)
	if err != nil {
		return "", false, err
	}
	requestDigest := sha256.Sum256(body)
	created, err := database.CreateTaskIfAbsent(taskID, "appstore_deploy", "pending", fmt.Sprintf("%x", requestDigest))
	return taskID, created, err
}

func removeExplicitContainerNames(composeContent string) (string, error) {
	var composeMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(composeContent), &composeMap); err != nil {
		return "", err
	}

	servicesRaw, ok := composeMap["services"]
	if !ok {
		return composeContent, nil
	}
	services, ok := servicesRaw.(map[string]interface{})
	if !ok {
		return composeContent, nil
	}

	changed := false
	for _, serviceRaw := range services {
		service, ok := serviceRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if _, ok := service["container_name"]; ok {
			delete(service, "container_name")
			changed = true
		}
	}

	if !changed {
		return composeContent, nil
	}
	out, err := marshalComposeYAMLOrdered(composeMap)
	if err != nil {
		return "", err
	}
	return out, nil
}

func mapHostPortsToContainerIDs(containers []types.Container) map[int]string {
	portToContainer := make(map[int]string)
	for _, ctr := range containers {
		for _, p := range ctr.Ports {
			if p.PublicPort == 0 {
				continue
			}
			if strings.ToLower(p.Type) != "tcp" {
				continue
			}
			portToContainer[int(p.PublicPort)] = ctr.ID
		}
	}
	return portToContainer
}

func collectComposePublishedTCPPorts(composeContent string) []int {
	var document map[string]interface{}
	if err := yaml.Unmarshal([]byte(composeContent), &document); err != nil {
		return nil
	}
	services, _ := document["services"].(map[string]interface{})
	seen := make(map[int]struct{})
	add := func(value interface{}) {
		raw := strings.TrimSpace(fmt.Sprintf("%v", value))
		bounds := strings.SplitN(raw, "-", 2)
		start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
		if err != nil || start <= 0 || start > 65535 {
			return
		}
		end := start
		if len(bounds) == 2 {
			end, err = strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil || end < start || end > 65535 {
				return
			}
		}
		for port := start; port <= end; port++ {
			seen[port] = struct{}{}
		}
	}
	for _, rawService := range services {
		service, _ := rawService.(map[string]interface{})
		ports, _ := service["ports"].([]interface{})
		for _, rawPort := range ports {
			switch port := rawPort.(type) {
			case string:
				value := strings.TrimSpace(port)
				protocol := "tcp"
				if slash := strings.LastIndex(value, "/"); slash >= 0 {
					protocol = strings.ToLower(strings.TrimSpace(value[slash+1:]))
					value = value[:slash]
				}
				if protocol != "tcp" {
					continue
				}
				parts := strings.Split(value, ":")
				if len(parts) >= 2 {
					add(parts[len(parts)-2])
				}
			case map[string]interface{}:
				protocol := "tcp"
				if rawProtocol, exists := port["protocol"]; exists && rawProtocol != nil {
					if parsed := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", rawProtocol))); parsed != "" {
						protocol = parsed
					}
				}
				if protocol == "tcp" {
					add(port["published"])
				}
			}
		}
	}
	result := make([]int, 0, len(seen))
	for port := range seen {
		result = append(result, port)
	}
	sort.Ints(result)
	return result
}

func deployApp(c *gin.Context) {
	id := c.Param("id")
	logging.Debug("AppStore deployment requested", "template", id)

	var req DeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}
	if handleEditionAppStoreDeployment(c, id, req) {
		return
	}

	if len(strings.TrimSpace(req.IdempotencyKey)) > 256 {
		respondError(c, http.StatusBadRequest, "部署幂等键过长", nil)
		return
	}
	taskID, created, err := claimLocalAppStoreTask(id, req)
	if err != nil {
		if errors.Is(err, database.ErrTaskRequestConflict) {
			respondError(c, http.StatusConflict, "本次部署请求的参数已变化，请重新发起部署", nil)
			return
		}
		respondError(c, http.StatusInternalServerError, "创建部署任务失败", err)
		return
	}
	if !created {
		c.JSON(http.StatusOK, gin.H{
			"message": "部署任务已存在",
			"taskId":  taskID,
		})
		return
	}
	seq := int64(0)
	appendLog := func(logType string, message string) {
		seq++
		_ = database.AppendTaskLogWithSeq(taskID, seq, time.Now(), logType, message)
	}
	appendLog("info", fmt.Sprintf("开始部署应用 (ID: %s)", id))

	// 先在启动 goroutine 前注册取消回调，避免 cancel API 在任务刚创建、goroutine 尚未注册
	// 的窗口期被调用时，取消信号丢失导致任务无法被终止。
	ctx, cancel := context.WithCancel(context.Background())
	registerComposeTaskCancel(taskID, cancel)

	go func(taskId string, appId string, deployReq DeployRequest, startSeq int64, ctx context.Context) {
		seq := startSeq
		var logMu sync.Mutex
		appendLog := func(logType string, message string) {
			logMu.Lock()
			defer logMu.Unlock()
			seq++
			_ = database.AppendTaskLogWithSeq(taskId, seq, time.Now(), logType, message)
		}
		finish := func(status string, result any, errStr string) {
			if err := finishAppStoreTask(taskId, status, result, errStr); err != nil {
				logging.Error("AppStore task result persistence failed", "task_id", taskId, "status", status, "error", err)
				appendLog("error", "部署结果保存失败，请检查任务状态；不会自动重复部署")
			}
		}
		defer unregisterComposeTaskCancel(taskId)
		cancelIfRequested := func() bool {
			if ctx.Err() == nil {
				return false
			}
			appendLog("warning", "部署已取消")
			finish("canceled", nil, "部署已取消")
			return true
		}

		_ = database.UpsertTask(taskId, "appstore_deploy", "running")
		notifyName := strings.TrimSpace(appId)
		defer func() {
			summary, err := database.GetTask(taskId)
			if err != nil {
				return
			}
			st := strings.ToLower(strings.TrimSpace(summary.Status))
			if st != "success" && st != "error" && st != "failed" && st != "completed" {
				return
			}
			msg := fmt.Sprintf("应用 %s 部署任务结束", notifyName)
			typ := "info"
			if st == "success" || st == "completed" {
				typ = "success"
				msg = fmt.Sprintf("应用 %s 部署成功", notifyName)
			} else {
				typ = "error"
				errText := strings.TrimSpace(summary.Error)
				if errText == "" {
					errText = "未知错误"
				}
				msg = fmt.Sprintf("应用 %s 部署失败（%s）", notifyName, errText)
			}
			_ = database.SaveNotification(&database.Notification{
				Type:     typ,
				Category: "deploy_task",
				Message:  msg,
				Read:     false,
			})
		}()

		logging.Debug("AppStore deployment parameters prepared", "template", appId, "config_count", len(deployReq.Config), "env_count", len(deployReq.Env))

		appendLog("info", "正在获取应用配置...")
		app, err := getAppFromCacheOrServer(appId)
		if err != nil {
			appendLog("error", fmt.Sprintf("获取应用详情失败: %v", err))
			finish("error", nil, err.Error())
			return
		}
		if strings.TrimSpace(app.Name) != "" {
			notifyName = strings.TrimSpace(app.Name)
		}
		if cancelIfRequested() {
			return
		}

		baseName := normalizeProjectName(app.Name)
		if _, ok := validateComposeProjectName(baseName); !ok {
			baseName = "project"
		}
		if isSelfProjectName(baseName) {
			errMsg := "容器化部署模式下，禁止部署到自身项目目录"
			appendLog("error", errMsg)
			finish("error", nil, errMsg)
			return
		}

		renderedBundle, _, renderErr := renderAppStoreDeployment(app, deployReq)
		if renderErr != nil {
			errMsg := "渲染应用配置失败: " + renderErr.Error()
			appendLog("error", errMsg)
			finish("error", nil, errMsg)
			return
		}

		projectName, composeDir, dirErr := reserveAppStoreProjectDir(getProjectsBaseDir(), baseName)
		if dirErr != nil {
			errMsg := "创建部署目录失败: " + dirErr.Error()
			appendLog("error", errMsg)
			finish("error", nil, errMsg)
			return
		}
		appendLog("info", fmt.Sprintf("准备部署目录: %s", projectName))

		composeFile := filepath.Join(composeDir, "docker-compose.yml")
		composeContent := renderedBundle.Compose
		if projectName != baseName {
			if modified, err := removeExplicitContainerNames(composeContent); err == nil {
				composeContent = modified
			} else {
				appendLog("warning", fmt.Sprintf("移除 container_name 失败: %v", err))
			}
		}

		if pathErrs := validateComposeAssetPaths(composeContent); len(pathErrs) > 0 {
			errMsg := strings.Join(pathErrs, "; ")
			appendLog("error", errMsg)
			finish("error", nil, errMsg)
			os.RemoveAll(composeDir)
			return
		}

		composeContentToWrite := composeContent
		if !composeYAMLHasServices([]byte(composeContentToWrite)) {
			errMsg := "Compose YAML 根节点必须包含非空 services 映射"
			appendLog("error", errMsg)
			finish("error", nil, errMsg)
			os.RemoveAll(composeDir)
			return
		}

		renderedBundle.Compose = composeContentToWrite
		_, writeErr := writeRenderedAppStoreBundle(composeDir, composeFile, renderedBundle)
		if writeErr != nil {
			appendLog("error", fmt.Sprintf("写入应用文件失败: %v", writeErr))
			finish("error", nil, writeErr.Error())
			os.RemoveAll(composeDir)
			return
		}

		// The rendered bundle owns dotenv values; the shared command builder
		// delegates quoting, comments and interpolation to Docker Compose.
		commandEnv := []string{"COMPOSE_PROGRESS=plain", "COMPOSE_NO_COLOR=1"}

		appendLog("info", "正在校验 Docker Compose 配置...")
		if output, configErr := runDockerCombinedOutput(ctx, composeDir, commandEnv, []string{"compose", "config", "--quiet"}); configErr != nil {
			errMsg := strings.TrimSpace(string(output))
			if errMsg == "" {
				errMsg = configErr.Error()
			}
			appendLog("error", "Compose 配置校验失败: "+errMsg)
			finish("error", nil, errMsg)
			_ = os.RemoveAll(composeDir)
			return
		}

		appendLog("info", "开始执行 Docker Compose 部署...")
		if cancelIfRequested() {
			_ = os.RemoveAll(composeDir)
			return
		}
		cmd, cleanupRuntime, err := newDockerCommand(ctx, composeDir, commandEnv, []string{"compose", "up", "-d"})
		if err != nil {
			appendLog("error", fmt.Sprintf("准备部署命令失败: %v", err))
			finish("error", nil, err.Error())
			_ = os.RemoveAll(composeDir)
			return
		}
		defer cleanupRuntime()

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			appendLog("error", fmt.Sprintf("创建输出管道失败: %v", err))
			finish("error", nil, err.Error())
			_ = os.RemoveAll(composeDir)
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			appendLog("error", fmt.Sprintf("创建错误管道失败: %v", err))
			finish("error", nil, err.Error())
			_ = os.RemoveAll(composeDir)
			return
		}

		if err := cmd.Start(); err != nil {
			appendLog("error", fmt.Sprintf("启动部署命令失败: %v", err))
			finish("error", nil, err.Error())
			os.RemoveAll(composeDir)
			return
		}

		streamPipe := func(r io.Reader) {
			buf := make([]byte, 4096)
			var acc []byte
			lastFlush := time.Now()
			flush := func() {
				if len(acc) == 0 {
					return
				}
				chunks := strings.Split(strings.ReplaceAll(string(acc), "\r", "\n"), "\n")
				for _, c := range chunks {
					line := strings.TrimSpace(c)
					if line != "" {
						appendLog("info", line)
					}
				}
				acc = acc[:0]
				lastFlush = time.Now()
			}
			for {
				n, err := r.Read(buf)
				if n > 0 {
					acc = append(acc, buf[:n]...)
					if bytes.Contains(buf[:n], []byte{'\n'}) || bytes.Contains(buf[:n], []byte{'\r'}) || time.Since(lastFlush) > 2*time.Second {
						flush()
					}
				}
				if err != nil {
					flush()
					return
				}
			}
		}

		var streamWG sync.WaitGroup
		streamWG.Add(2)
		go func() {
			defer streamWG.Done()
			streamPipe(stdout)
		}()
		go func() {
			defer streamWG.Done()
			streamPipe(stderr)
		}()

		streamWG.Wait()
		waitErr := cmd.Wait()
		if ctx.Err() != nil {
			appendLog("warning", "部署已取消，正在清理已启动的资源...")
			appendLog("info", "项目配置和已生成数据将保留，可在 Compose 页面管理")
			if cleanupErr := rollbackAppStoreDeployment(composeDir, commandEnv); cleanupErr != nil {
				message := "部署已取消，但资源清理失败；项目目录已保留，请在 Compose 页面检查后重试清理: " + cleanupErr.Error()
				appendLog("error", message)
				finish("error", nil, message)
				return
			}
			finish("canceled", nil, "部署已取消")
			return
		}
		if waitErr != nil {
			appendLog("error", fmt.Sprintf("部署命令执行失败: %v", waitErr))
			appendLog("info", "项目配置和已生成数据将保留，可在 Compose 页面管理")
			appendLog("info", "正在清理失败的部署资源...")
			if cleanupErr := rollbackAppStoreDeployment(composeDir, commandEnv); cleanupErr != nil {
				message := waitErr.Error() + "; 资源清理失败，项目目录已保留: " + cleanupErr.Error()
				appendLog("error", message)
				finish("error", nil, message)
				return
			}
			finish("error", nil, waitErr.Error())
			return
		}

		appendLog("info", "正在核验部署状态...")
		verification, verifyErr := verifyAppStoreDeployment(ctx, projectName)
		if verifyErr != nil {
			if cancelIfRequested() {
				return
			}
			appendLog("error", "部署验证失败，保留项目和容器供检查: "+verifyErr.Error())
			finish("error", gin.H{"app_id": app.ID, "project": projectName, "verification": verification}, verifyErr.Error())
			return
		}
		appendLog("success", fmt.Sprintf("应用 %s 部署成功！", app.Name))

		usedPorts := collectComposePublishedTCPPorts(composeContentToWrite)
		if len(usedPorts) > 0 {
			appendLog("info", fmt.Sprintf("正在登记端口使用情况: %v", usedPorts))
			owners := map[string][]int{}
			if cli, err := docker.NewDockerClient(); err == nil {
				defer cli.Close()
				containers, cerr := cli.ContainerList(context.Background(), types.ContainerListOptions{
					All:     true,
					Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+projectName)),
				})
				if cerr == nil && len(containers) == 0 {
					containers, _ = cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
					var filtered []types.Container
					for _, ctr := range containers {
						if ctr.Labels["com.docker.compose.project"] == projectName {
							filtered = append(filtered, ctr)
							continue
						}
						if wd := strings.TrimSpace(ctr.Labels["com.docker.compose.project.working_dir"]); wd != "" && filepath.Base(wd) == projectName {
							filtered = append(filtered, ctr)
						}
					}
					containers = filtered
				}
				portToContainer := mapHostPortsToContainerIDs(containers)
				for _, p := range usedPorts {
					if cid := strings.TrimSpace(portToContainer[p]); cid != "" {
						owners[cid] = append(owners[cid], p)
					} else {
						owners[projectName] = append(owners[projectName], p)
					}
				}
			} else {
				for _, p := range usedPorts {
					owners[projectName] = append(owners[projectName], p)
				}
			}

			tx, err := database.GetDB().Begin()
			if err != nil {
				appendLog("warning", "无法开启数据库事务进行端口登记")
			} else {
				ok := true
				for owner, ports := range owners {
					if rerr := database.ReservePortsTx(tx, ports, owner, "TCP", "App"); rerr != nil {
						ok = false
						appendLog("warning", fmt.Sprintf("端口登记失败: %v", rerr))
						break
					}
				}
				if !ok {
					_ = tx.Rollback()
				} else if cerr := tx.Commit(); cerr != nil {
					appendLog("warning", fmt.Sprintf("端口登记提交失败: %v", cerr))
				} else {
					appendLog("info", "端口登记完成")
				}
			}
		}

		finish("success", gin.H{"app_id": app.ID, "project": projectName, "verification": verification}, "")
	}(taskID, id, req, seq, ctx)

	c.JSON(http.StatusOK, gin.H{
		"message": "部署任务已提交",
		"taskId":  taskID,
	})
}

func listAppStoreTasks(c *gin.Context) {
	statusesRaw := strings.TrimSpace(c.Query("statuses"))
	limitRaw := strings.TrimSpace(c.Query("limit"))
	var statuses []string
	if statusesRaw != "" {
		for _, s := range strings.Split(statusesRaw, ",") {
			if v := strings.TrimSpace(s); v != "" {
				statuses = append(statuses, v)
			}
		}
	}

	limit := 20
	if limitRaw != "" {
		if v, err := strconv.Atoi(limitRaw); err == nil && v > 0 {
			limit = v
		}
	}

	environmentID, ok := composeTaskEnvironmentScope(c)
	if !ok {
		return
	}
	list, err := database.ListTasksInEnvironment(environmentID, []string{"appstore_deploy", "remote_appstore_deploy"}, statuses, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取应用商店部署任务失败", err)
		return
	}
	c.JSON(http.StatusOK, list)
}

func getAppStoreTask(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		respondError(c, http.StatusBadRequest, "任务ID不能为空", nil)
		return
	}
	environmentID, ok := composeTaskEnvironmentScope(c)
	if !ok {
		return
	}
	t, err := database.GetTaskInEnvironment(environmentID, taskID)
	if err != nil {
		respondError(c, http.StatusNotFound, "任务不存在", err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func taskEvents(c *gin.Context) {
	environmentID, ok := composeTaskEnvironmentScope(c)
	if !ok {
		return
	}
	streamDatabaseTaskEventsInEnvironment(c, environmentID, strings.TrimSpace(c.Param("id")))
}

func getAppStatus(c *gin.Context) {
	id := c.Param("id")
	app, err := getAppFromCacheOrServer(id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取应用信息失败", err)
		return
	}

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "label",
			Value: "com.docker.compose.project=" + app.Name,
		}),
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取容器列表失败", err)
		return
	}

	total := len(containers)
	running := 0
	for _, container := range containers {
		if container.State == "running" {
			running++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":       id,
		"name":     app.Name,
		"total":    total,
		"running":  running,
		"deployed": total > 0,
		"healthy":  total > 0 && running == total,
	})
}

func injectEnvToYaml(content string, env map[string]string) (string, error) {
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &data); err != nil {
		return "", err
	}
	services, ok := data["services"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("no services found or invalid format")
	}

	for _, service := range services {
		svcMap, ok := service.(map[string]interface{})
		if !ok {
			continue
		}
		envData, hasEnv := svcMap["environment"]
		if !hasEnv {
			newEnv := make(map[string]string)
			for k, v := range env {
				newEnv[k] = v
			}
			svcMap["environment"] = newEnv
			continue
		}
		switch e := envData.(type) {
		case map[string]interface{}:
			for k, v := range env {
				e[k] = v
			}
		case []interface{}:
			for k, v := range env {
				e = append(e, fmt.Sprintf("%s=%s", k, v))
			}
			svcMap["environment"] = e
		}
	}

	out, err := marshalComposeYAMLOrdered(data)
	if err != nil {
		return "", err
	}
	return out, nil
}
