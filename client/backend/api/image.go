package api

import (
	"archive/tar"
	"context"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/registrycheck"
	"dockerpanel/backend/pkg/settings"
	"dockerpanel/backend/pkg/system"
	"encoding/base64"
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
	"sync/atomic"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	registrytypes "github.com/docker/docker/api/types/registry"
	"github.com/gin-gonic/gin"
)

var (
	// imageRemoteDigestResolve 是 runImageUpdateCheck 的远端 digest 解析入口
	// （直连 registry 链），正常路径不再直接调 getDockerHubDigest。
	imageRemoteDigestResolve = resolveRemoteImageDigestViaChain
	// daemonImageDigestFallback 是全链失败后的 daemon DistributionInspect 兜底
	// （保底=今天行为），做成 hook 便于测试注入。
	daemonImageDigestFallback = getDockerHubDigest
	// imageRegistryCredential 按已存 registry 记录为 repoTag 匹配凭证后注入 pkg 层。
	imageRegistryCredential = registryCredentialForRepoTag
)

var registryDigestResolver = registrycheck.NewResolver()

// errImageUpdateCheckInFlight 表示镜像更新检测已有实例在运行（单飞保护）。
var errImageUpdateCheckInFlight = errors.New("镜像更新检测已在运行")

// imageUpdateCheckInFlight 是 runImageUpdateCheck 的单飞锁：调度器、手动触发与
// 计划任务可能并发进入，正在运行时的后续调用直接拒绝，避免两份并发跑全量镜像。
var imageUpdateCheckInFlight sync.Mutex

var remoteDigestErrorMu sync.Mutex
var remoteDigestErrorLast = make(map[string]time.Time)

func allowRemoteDigestErrorLog(repoTag string) bool {
	repoTag = strings.TrimSpace(repoTag)
	if repoTag == "" {
		return true
	}

	now := time.Now()
	window := 10 * time.Minute

	remoteDigestErrorMu.Lock()
	defer remoteDigestErrorMu.Unlock()

	if last, ok := remoteDigestErrorLast[repoTag]; ok {
		if now.Sub(last) < window {
			return false
		}
	}

	remoteDigestErrorLast[repoTag] = now

	if len(remoteDigestErrorLast) > 1000 {
		cutoff := now.Add(-30 * time.Minute)
		for k, ts := range remoteDigestErrorLast {
			if ts.Before(cutoff) {
				delete(remoteDigestErrorLast, k)
			}
		}
	}

	return true
}

func RegisterImageRoutes(r *gin.RouterGroup) {
	group := r.Group("/images")
	{
		group.GET("", listImages)
		group.DELETE("/:id", removeImage)
		group.GET("/updates", checkImageUpdates)
		group.GET("/updates/status", listStoredImageUpdates)
		group.POST("/updates/clear", clearImageUpdate)
		group.POST("/updates/apply", applyImageUpdates)
		group.POST("/pull", pullImage)
		group.POST("/pull/tasks", startImagePullTask)
		group.GET("/pull/tasks", listImagePullTasks)
		group.GET("/pull/tasks/:id", getImagePullTask)
		group.GET("/pull/tasks/:id/events", imagePullTaskEvents)
		group.GET("/pull/progress", pullImageProgress)
		group.GET("/proxy", getDockerProxy)
		group.GET("/proxy/history", getDockerProxyHistory)
		group.POST("/proxy", updateDockerProxy)
		group.POST("/mirrors/check", checkRegistryMirror)
		group.POST("/tag", tagImage)
		group.GET("/export/:id", exportImage)
		group.GET("/inspect/:id", inspectImage)
		group.GET("/history/:id", imageHistory)
		group.POST("/import", importImage)
		group.POST("/prune", pruneImages)
		group.GET("/build-cache", getBuildCache)
		group.POST("/build-cache/prune", pruneBuildCache)
	}
}

// 清理未使用的镜像
func pruneImages(c *gin.Context) {
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	report, err := cli.ImagesPrune(c.Request.Context(), filters.Args{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "清理镜像失败", err)
		return
	}

	// 同步清理被清理镜像的更新记录（按 daemon 返回的 Untagged 清单精确清理）
	clearImageUpdateRecordsByImageRefs(removedImageTagsFromDeleteItems(report.ImagesDeleted)...)

	c.JSON(http.StatusOK, gin.H{
		"message": "已清理未使用的镜像",
		"report":  report,
	})
}

// 导入镜像
func importImage(c *gin.Context) {
	// 获取上传的文件
	file, err := c.FormFile("file")
	if err != nil {
		respondError(c, http.StatusBadRequest, "获取上传文件失败", err)
		return
	}

	// 创建临时文件
	tempFile, err := os.CreateTemp("", "docker-image-*.tar")
	if err != nil {
		respondError(c, http.StatusInternalServerError, "创建临时文件失败", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// 保存上传的文件到临时文件
	src, err := file.Open()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "打开上传文件失败", err)
		return
	}
	defer src.Close()

	if _, err = io.Copy(tempFile, src); err != nil {
		respondError(c, http.StatusInternalServerError, "保存上传文件失败", err)
		return
	}

	// 关闭临时文件
	tempFile.Close()

	// 从tar文件中解析镜像信息
	imageInfo, err := extractImageInfoFromTar(tempFile.Name())
	if err != nil {
		logging.Warn("image archive metadata parse failed", "error", err)
	} else {
		logging.Debug("image archive metadata parsed", "field_count", len(imageInfo))
	}

	// 创建Docker客户端
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	// 打开临时文件用于导入
	importFile, err := os.Open(tempFile.Name())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "读取临时文件失败", err)
		return
	}
	defer importFile.Close()

	// 导入镜像
	response, err := cli.ImageLoad(c.Request.Context(), importFile, true)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "导入镜像失败", err)
		return
	}
	defer response.Body.Close()

	// 读取响应
	body, err := readImageLoadResponse(response)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "读取导入响应失败", err)
		return
	}

	// 返回结果，优先使用从tar文件解析的信息
	if imageInfo != nil {
		c.JSON(http.StatusOK, gin.H{
			"message":   "镜像导入成功",
			"details":   string(body),
			"imageInfo": imageInfo,
		})
	} else {
		// 如果无法从tar文件解析，则返回基本信息
		c.JSON(http.StatusOK, gin.H{
			"message": "镜像导入成功",
			"details": string(body),
		})
	}
}

func readImageLoadResponse(response types.ImageLoadResponse) ([]byte, error) {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.JSON {
		if err := consumeDockerPullResponse(io.NopCloser(strings.NewReader(string(body)))); err != nil {
			return nil, err
		}
	}
	return body, nil
}

// 从tar文件中提取镜像信息
func extractImageInfoFromTar(tarPath string) (map[string]interface{}, error) {
	// 打开tar文件
	f, err := os.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("打开tar文件失败: %v", err)
	}
	defer f.Close()

	// 创建tar读取器
	tr := tar.NewReader(f)

	// 查找manifest.json文件
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取tar文件条目失败: %v", err)
		}

		// 检查是否是manifest.json文件
		if filepath.Base(header.Name) == "manifest.json" {
			// 读取manifest.json内容
			manifestData, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("读取manifest.json失败: %v", err)
			}

			// 解析manifest.json
			var manifests []struct {
				Config   string   `json:"Config"`
				RepoTags []string `json:"RepoTags"`
				Layers   []string `json:"Layers"`
			}
			if err := json.Unmarshal(manifestData, &manifests); err != nil {
				return nil, fmt.Errorf("解析manifest.json失败: %v", err)
			}

			// 如果找到了manifest信息
			if len(manifests) > 0 {
				imageID := ""
				if manifests[0].Config != "" {
					// 从Config文件名中提取镜像ID
					imageID = strings.TrimSuffix(manifests[0].Config, ".json")
				}

				repoTags := manifests[0].RepoTags
				if len(repoTags) == 0 {
					repoTags = []string{"<none>:<none>"}
				}

				return map[string]interface{}{
					"id":       imageID,
					"repoTags": repoTags,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("未在tar文件中找到manifest.json或有效的镜像信息")
}

// Docker代理配置结构
type DockerConfig struct {
	Enabled         bool                       `json:"enabled"`
	HTTPProxy       string                     `json:"HTTP Proxy"`
	HTTPSProxy      string                     `json:"HTTPS Proxy"`
	NoProxy         string                     `json:"No Proxy"`
	RegistryMirrors []string                   `json:"registry-mirrors"`
	Registries      map[string]docker.Registry `json:"registries"`
	Effective       *DockerEffectiveConfig     `json:"effective,omitempty"`
	ProxySource     string                     `json:"proxy_source,omitempty"`
	MirrorSource    string                     `json:"mirror_source,omitempty"`
	PendingRestart  bool                       `json:"pending_restart"`
}

type DockerEffectiveConfig struct {
	Available       bool     `json:"available"`
	Enabled         bool     `json:"enabled"`
	HTTPProxy       string   `json:"HTTP Proxy"`
	HTTPSProxy      string   `json:"HTTPS Proxy"`
	NoProxy         string   `json:"No Proxy"`
	RegistryMirrors []string `json:"registry-mirrors"`
}

// 类型转换函数
func convertRegistryToDocker(r *database.Registry) docker.Registry {
	return docker.Registry{
		Name:     r.Name,
		URL:      r.URL,
		Username: r.Username,
		Password: r.Password,
	}
}

func convertRegistryToDatabase(r docker.Registry) *database.Registry {
	return &database.Registry{
		Name:     r.Name,
		URL:      r.URL,
		Username: r.Username,
		Password: r.Password,
	}
}

// 获取 Docker 代理配置
func getDockerProxy(c *gin.Context) {
	resolved := loadDockerNetworkConfig(c.Request.Context())

	// 获取注册表配置
	dbRegistries, err := database.GetAllRegistries()
	if err != nil {
		logging.Warn("registry configuration could not be read", "error", err)
		respondError(c, http.StatusInternalServerError, "获取注册表配置失败", nil)
		return
	}

	// 转换为 docker.Registry 类型
	registries := make(map[string]docker.Registry)
	for k, v := range dbRegistries {
		registries[k] = convertRegistryToDocker(v)
	}

	config := resolved.Config
	config.Registries = registries
	config.ProxySource = resolved.ProxySource
	config.MirrorSource = resolved.MirrorSource
	config.PendingRestart = resolved.PendingRestart
	config.Effective = &DockerEffectiveConfig{
		Available:       resolved.RuntimeAvailable,
		Enabled:         hasDockerProxy(resolved.Effective.HTTPProxy, resolved.Effective.HTTPSProxy),
		HTTPProxy:       strings.TrimSpace(resolved.Effective.HTTPProxy),
		HTTPSProxy:      strings.TrimSpace(resolved.Effective.HTTPSProxy),
		NoProxy:         strings.TrimSpace(resolved.Effective.NoProxy),
		RegistryMirrors: append([]string(nil), resolved.Effective.RegistryMirrors...),
	}

	c.JSON(http.StatusOK, config)
}

// 更新 Docker 代理配置
func updateDockerProxy(c *gin.Context) {
	var config DockerConfig
	raw, readErr := c.GetRawData()
	if readErr != nil {
		respondError(c, http.StatusBadRequest, "无效的配置格式", readErr)
		return
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		respondError(c, http.StatusBadRequest, "无效的配置格式", err)
		return
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		respondError(c, http.StatusBadRequest, "无效的配置格式", err)
		return
	}
	_, hasEnabled := fields["enabled"]
	_, hasMirrors := fields["registry-mirrors"]
	_, hasRegistries := fields["registries"]

	logging.Debug(
		"Docker network configuration received",
		"enabled", config.Enabled,
		"has_http_proxy", strings.TrimSpace(config.HTTPProxy) != "",
		"has_https_proxy", strings.TrimSpace(config.HTTPSProxy) != "",
		"has_no_proxy", strings.TrimSpace(config.NoProxy) != "",
		"mirror_count", len(config.RegistryMirrors),
	)

	// 更新 daemon.json 配置
	daemonConfig := &docker.DaemonConfig{}
	if hasMirrors {
		daemonConfig.RegistryMirrors = config.RegistryMirrors
	}

	if hasEnabled && config.Enabled {
		daemonConfig.Proxies = &docker.ProxyConfig{
			HTTPProxy:  config.HTTPProxy,
			HTTPSProxy: config.HTTPSProxy,
			NoProxy:    config.NoProxy,
		}
	} else if hasEnabled {
		daemonConfig.ClearProxies = true
	}

	// 保存到 daemon.json
	if hasMirrors || hasEnabled {
		if err := docker.UpdateDaemonConfig(daemonConfig); err != nil {
			logging.Error("Docker daemon configuration update failed", "error", err)
			respondError(c, http.StatusInternalServerError, "更新配置失败", err)
			return
		}
	}

	// 保存注册表配置到数据库
	if hasRegistries {
		for key, registry := range config.Registries {
			dbRegistry := &database.Registry{
				Name:      registry.Name,
				URL:       registry.URL,
				Username:  registry.Username,
				Password:  registry.Password,
				IsDefault: key == "docker.io", // docker.io 为默认注册表
			}

			// 确保 URL 不为空
			if dbRegistry.URL == "" {
				dbRegistry.URL = key
				logging.Debug("registry address derived from configuration key", "key", key)
			}

			logging.Debug("registry configuration save started", "key", key, "name", dbRegistry.Name)

			if err := database.SaveRegistry(dbRegistry); err != nil {
				logging.Error("registry configuration save failed", "key", key, "error", err)
				respondError(c, http.StatusInternalServerError, "保存注册表配置失败", err)
				return
			}
		}
	}

	// 保存到数据库作为备用配置
	if hasEnabled || hasMirrors {
		proxy, err := database.GetDockerProxy()
		if err != nil {
			logging.Warn("stored Docker proxy configuration could not be read", "error", err)
			proxy = &database.DockerProxy{}
		}
		if hasEnabled {
			proxy.Enabled = config.Enabled
			if config.Enabled {
				proxy.HTTPProxy = config.HTTPProxy
				proxy.HTTPSProxy = config.HTTPSProxy
				proxy.NoProxy = config.NoProxy
			} else {
				proxy.HTTPProxy = ""
				proxy.HTTPSProxy = ""
				proxy.NoProxy = ""
			}
		}
		if hasMirrors {
			proxy.RegistryMirrors = database.MarshalRegistryMirrors(config.RegistryMirrors)
		}
		if proxy.Enabled || proxy.RegistryMirrors != "" {
			if err := database.SaveDockerProxy(proxy); err != nil {
				logging.Error("Docker proxy configuration persistence failed", "error", err)
			}
		} else if hasEnabled {
			if err := database.DeleteDockerProxy(); err != nil {
				logging.Error("stored Docker proxy configuration removal failed", "error", err)
			}
		}
		_ = database.SaveProxyHistory(&database.ProxyHistory{
			Enabled:         proxy.Enabled,
			HTTPProxy:       proxy.HTTPProxy,
			HTTPSProxy:      proxy.HTTPSProxy,
			NoProxy:         proxy.NoProxy,
			RegistryMirrors: proxy.RegistryMirrors,
			ChangeType:      "updated",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Docker配置已更新，请运行以下命令重启 Docker 服务：\nsudo systemctl restart docker",
	})
}

// 获取代理历史记录
func getDockerProxyHistory(c *gin.Context) {
	list, err := database.GetProxyHistory(20)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取历史记录失败", err)
		return
	}
	c.JSON(http.StatusOK, list)
}

type imagePullTaskRequest struct {
	Image           string `json:"name" binding:"required"`
	Registry        string `json:"registry"`
	CleanupOldImage bool   `json:"cleanupOldImage"`
}

type imagePullCleanupStore interface {
	imageTagRemovalStore
	ImageInspectWithRaw(context.Context, string) (types.ImageInspect, []byte, error)
	ImagePull(context.Context, string, types.ImagePullOptions) (io.ReadCloser, error)
}

type imagePullCleanupResult struct {
	CleanupRequested   bool   `json:"cleanupRequested"`
	CleanupSucceeded   bool   `json:"cleanupSucceeded"`
	CleanupError       string `json:"cleanupError,omitempty"`
	OldImageID         string `json:"oldImageId,omitempty"`
	TemporaryReference string `json:"-"`
}

func imageUpdateTemporaryReference(operationID string) string {
	normalized := strings.ToLower(strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, operationID))
	normalized = strings.Trim(normalized, "-_.")
	if normalized == "" {
		normalized = strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	if len(normalized) > 96 {
		normalized = normalized[:96]
	}
	return "tradis.local/tradis-image-update:" + normalized
}

func pullImageWithOldCleanup(
	ctx context.Context,
	store imagePullCleanupStore,
	imageName string,
	options types.ImagePullOptions,
	operationID string,
	cleanupOldImage bool,
	onProgress func(map[string]any),
) (imagePullCleanupResult, error) {
	result := imagePullCleanupResult{CleanupRequested: cleanupOldImage}
	if store == nil {
		return result, fmt.Errorf("Docker 镜像存储不可用")
	}

	if cleanupOldImage {
		inspect, _, err := store.ImageInspectWithRaw(ctx, imageName)
		if err != nil {
			return result, fmt.Errorf("读取更新前镜像失败: %w", err)
		}
		result.OldImageID = strings.TrimSpace(inspect.ID)
		if result.OldImageID == "" {
			return result, fmt.Errorf("更新前镜像缺少镜像 ID")
		}
		result.TemporaryReference = imageUpdateTemporaryReference(operationID)
		if err := retainImageReferenceWithStore(ctx, store, result.OldImageID, result.TemporaryReference); err != nil {
			return result, fmt.Errorf("保留更新前镜像失败: %w", err)
		}
	}

	releaseTemporaryReference := func() error {
		if result.TemporaryReference == "" {
			return nil
		}
		if err := releaseImageReferenceWithStore(ctx, store, result.TemporaryReference); err != nil {
			result.CleanupError = err.Error()
			return fmt.Errorf("清理更新前镜像失败: %w", err)
		}
		result.CleanupSucceeded = true
		return nil
	}

	reader, err := store.ImagePull(ctx, imageName, options)
	if err != nil {
		return result, errors.Join(err, releaseTemporaryReference())
	}

	dec := json.NewDecoder(reader)
	var pullErr error
	for {
		var payload map[string]any
		if err := dec.Decode(&payload); err != nil {
			if !errors.Is(err, io.EOF) {
				pullErr = fmt.Errorf("读取镜像拉取进度失败: %w", err)
			}
			break
		}
		if errVal, hasErr := payload["error"]; hasErr {
			pullErr = errors.New(fmt.Sprint(errVal))
			break
		}
		if onProgress != nil {
			onProgress(payload)
		}
	}
	_ = reader.Close()

	cleanupErr := releaseTemporaryReference()
	if pullErr != nil {
		return result, errors.Join(pullErr, cleanupErr)
	}
	// 拉取已经成功时，清理失败作为警告返回，避免把已完成的镜像更新误报为拉取失败。
	return result, nil
}

func resolveImagePullOptions(imageName string, registry string) (string, types.ImagePullOptions, error) {
	var options types.ImagePullOptions
	imageName = strings.TrimSpace(imageName)
	registry = strings.TrimSpace(registry)
	if imageName == "" {
		return "", options, errors.New("镜像名称不能为空")
	}

	if registry == "" {
		return imageName, options, nil
	}

	registries, err := database.GetAllRegistries()
	if err != nil {
		return "", options, err
	}

	reg, ok := registries[registry]
	if !ok {
		return imageName, options, nil
	}

	if registry != "docker.io" {
		imageName = strings.TrimRight(reg.URL, "/") + "/" + imageName
	}

	if reg.Username != "" && reg.Password != "" {
		authConfig := types.AuthConfig{
			Username: reg.Username,
			Password: reg.Password,
		}
		encodedJSON, marshalErr := json.Marshal(authConfig)
		if marshalErr != nil {
			return "", options, marshalErr
		}
		options.RegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
	}

	return imageName, options, nil
}

func startImagePullTask(c *gin.Context) {
	var req imagePullTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}

	taskID := fmt.Sprintf("%d", time.Now().UnixNano())
	_ = database.UpsertTask(taskID, "image_pull", "pending")

	go runImagePullTask(taskID, strings.TrimSpace(req.Image), strings.TrimSpace(req.Registry), req.CleanupOldImage)

	c.JSON(http.StatusOK, gin.H{
		"message": "镜像拉取任务已提交",
		"taskId":  taskID,
	})
}

func runImagePullTask(taskID string, imageName string, registry string, cleanupOldImage bool) {
	seq := int64(0)
	appendLog := func(logType string, message string) {
		seq++
		_ = database.AppendTaskLogWithSeq(taskID, seq, time.Now(), logType, message)
	}
	finish := func(status string, result any, errStr string) {
		_ = database.FinishTask(taskID, status, result, errStr)
	}

	_ = database.UpsertTask(taskID, "image_pull", "running")
	appendLog("info", "开始拉取镜像: "+imageName)

	resolvedImage, pullOptions, err := resolveImagePullOptions(imageName, registry)
	if err != nil {
		appendLog("error", "准备拉取参数失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}
	if resolvedImage != imageName {
		appendLog("info", "使用注册表镜像地址: "+resolvedImage)
	}

	cli, err := docker.NewDockerClient()
	if err != nil {
		appendLog("error", "创建 Docker 客户端失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}
	defer cli.Close()

	cleanupResult, err := pullImageWithOldCleanup(
		context.Background(),
		cli,
		resolvedImage,
		pullOptions,
		taskID,
		cleanupOldImage,
		func(payload map[string]any) {
			if b, marshalErr := json.Marshal(payload); marshalErr == nil {
				appendLog("progress", string(b))
			}
		},
	)
	if err != nil {
		appendLog("error", "拉取镜像失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}
	if cleanupResult.CleanupSucceeded {
		appendLog("success", "已清理更新前的旧镜像")
	} else if cleanupResult.CleanupError != "" {
		appendLog("warning", "镜像已更新，但旧镜像清理失败: "+cleanupResult.CleanupError)
	}

	clearImageUpdateRecordsByImageRefs(resolvedImage, imageName)
	appendLog("success", "镜像拉取完成: "+imageName)
	finish("success", gin.H{
		"image":            imageName,
		"resolvedImage":    resolvedImage,
		"cleanupOldImage":  cleanupOldImage,
		"cleanupSucceeded": cleanupResult.CleanupSucceeded,
		"cleanupError":     cleanupResult.CleanupError,
	}, "")
}

func listImagePullTasks(c *gin.Context) {
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
	list, err := database.ListTasks([]string{"image_pull"}, statuses, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取镜像拉取任务失败", err)
		return
	}
	c.JSON(http.StatusOK, list)
}

func getImagePullTask(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		respondError(c, http.StatusBadRequest, "任务ID不能为空", nil)
		return
	}
	t, err := database.GetTask(taskID)
	if err != nil {
		respondError(c, http.StatusNotFound, "任务不存在", err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func imagePullTaskEvents(c *gin.Context) {
	streamDatabaseTaskEvents(c, strings.TrimSpace(c.Param("id")))
}

// 拉取进度监听
func pullImageProgress(c *gin.Context) {
	// 从查询参数获取镜像名称和注册表
	imageName := c.Query("name")
	registry := c.Query("registry")

	if imageName == "" {
		respondError(c, http.StatusBadRequest, "镜像名称不能为空", nil)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		respondError(c, http.StatusInternalServerError, "不支持流式输出", nil)
		return
	}

	ctx := c.Request.Context()

	cli, ok := getDockerClient(c)
	if !ok {
		flusher.Flush()
		return
	}
	defer cli.Close()

	var options types.ImagePullOptions

	// 如果指定了仓库，使用仓库配置
	if registry != "" {
		registries, getErr := database.GetAllRegistries()
		if getErr != nil {
			respondError(c, http.StatusInternalServerError, "获取注册表配置失败", getErr)
			return
		}

		if reg, ok := registries[registry]; ok {
			// 如果不是 docker.io，则拼接注册表地址
			if registry != "docker.io" {
				imageName = reg.URL + "/" + imageName
			}

			if reg.Username != "" && reg.Password != "" {
				authConfig := types.AuthConfig{
					Username: reg.Username,
					Password: reg.Password,
				}
				encodedJSON, marshalErr := json.Marshal(authConfig)
				if marshalErr == nil {
					options.RegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
				}
			}
		}
	}

	reader, err := cli.ImagePull(ctx, imageName, options)
	if err != nil {
		c.String(http.StatusInternalServerError, "data: %s\n\n", fmt.Sprintf(`{"error":%q}`, err.Error()))
		flusher.Flush()
		return
	}
	defer reader.Close()

	enc := json.NewEncoder(c.Writer)
	push := func(v any) {
		_, _ = c.Writer.Write([]byte("data: "))
		_ = enc.Encode(v)
		_, _ = c.Writer.Write([]byte("\n"))
		flusher.Flush()
	}

	dec := json.NewDecoder(reader)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var payload map[string]any
		if err := dec.Decode(&payload); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			logging.Debug("image pull progress stream closed", "error", err)
			push(map[string]any{"error": err.Error()})
			return
		}

		push(payload)
		if _, hasErr := payload["error"]; hasErr {
			return
		}
	}

	clearImageUpdateRecordsByImageRefs(imageName)
	push(map[string]any{"type": "done"})
}

// 拉取镜像
func pullImage(c *gin.Context) {
	var req struct {
		Image    string `json:"name" binding:"required"`
		Registry string `json:"registry"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		logging.Warn("image pull request rejected", "error", err)
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}

	logging.Info("image pull started", "image", req.Image, "registry_key", req.Registry)

	cli, ok := getDockerClient(c)
	if !ok {
		logging.Error("image pull could not create Docker client")
		return
	}
	defer cli.Close()

	var options types.ImagePullOptions
	imageName := req.Image

	// 如果指定了仓库，使用仓库配置
	if req.Registry != "" {
		registries, getErr := database.GetAllRegistries()
		if getErr != nil {
			logging.Error("image pull could not load registry configuration", "error", getErr)
			respondError(c, http.StatusInternalServerError, "获取注册表配置失败", getErr)
			return
		}

		if registry, ok := registries[req.Registry]; ok {
			// 如果不是 docker.io，则拼接注册表地址
			if req.Registry != "docker.io" {
				imageName = registry.URL + "/" + req.Image
			}
			logging.Debug("image pull selected configured registry", "registry", registry.Name)

			if registry.Username != "" && registry.Password != "" {
				authConfig := types.AuthConfig{
					Username: registry.Username,
					Password: registry.Password,
				}
				encodedJSON, marshalErr := json.Marshal(authConfig)
				if marshalErr == nil {
					options.RegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
					logging.Debug("image pull will use registry authentication")
				}
			}
		} else {
			logging.Warn("image pull registry configuration was not found", "registry_key", req.Registry)
		}
	} else {
		logging.Debug("image pull selected default registry")
	}

	reader, err := cli.ImagePull(c.Request.Context(), imageName, options)
	if err != nil {
		logging.Error("image pull failed", "image", req.Image, "error", err)
		respondError(c, http.StatusInternalServerError, "拉取镜像失败", err)
		return
	}
	defer reader.Close()

	response, err := io.ReadAll(reader)
	if err != nil {
		logging.Error("image pull response could not be read", "image", req.Image, "error", err)
		respondError(c, http.StatusInternalServerError, "读取响应失败", err)
		return
	}
	logging.Info("image pull completed", "image", req.Image)
	clearImageUpdateRecordsByImageRefs(imageName, req.Image)
	c.JSON(http.StatusOK, gin.H{"message": "镜像拉取成功", "details": string(response)})
}

// 展示镜像
func listImages(c *gin.Context) {
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	started := time.Now()
	images, err := cli.ImageList(c.Request.Context(), types.ImageListOptions{})
	recordDockerReadTiming(c, "list", started)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取镜像列表失败", err)
		return
	}

	c.JSON(http.StatusOK, images)
}

func imageRefMatches(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	return strings.TrimPrefix(a, "sha256:") == strings.TrimPrefix(b, "sha256:")
}

func containersUsingImage(ctx context.Context, cli interface {
	ContainerList(context.Context, types.ContainerListOptions) ([]types.Container, error)
}, imageID string, repoTag string) ([]gin.H, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}

	used := make([]gin.H, 0)
	for _, ctr := range containers {
		match := imageRefMatches(ctr.ImageID, imageID)
		if !match && repoTag != "" && strings.TrimSpace(ctr.Image) == strings.TrimSpace(repoTag) {
			match = true
		}
		if !match {
			continue
		}
		name := ctr.ID
		if len(ctr.Names) > 0 {
			name = strings.TrimPrefix(ctr.Names[0], "/")
		}
		used = append(used, gin.H{
			"id":     ctr.ID,
			"name":   name,
			"image":  ctr.Image,
			"state":  ctr.State,
			"status": ctr.Status,
		})
	}
	return used, nil
}

func inspectImage(c *gin.Context) {
	imageID := c.Param("id")

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	inspect, raw, err := cli.ImageInspectWithRaw(c.Request.Context(), imageID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取镜像详情失败", err)
		return
	}

	used, err := containersUsingImage(c.Request.Context(), cli, inspect.ID, "")
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取关联容器失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"image":      inspect,
		"raw":        json.RawMessage(raw),
		"containers": used,
	})
}

func imageHistory(c *gin.Context) {
	imageID := c.Param("id")

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	history, err := cli.ImageHistory(c.Request.Context(), imageID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取镜像历史失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"history": history})
}

// 删除镜像
func removeImage(c *gin.Context) {
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	id := c.Param("id")
	repoTag := c.Query("repoTag")
	force := strings.EqualFold(c.Query("force"), "true") || c.Query("force") == "1"

	target := id
	if repoTag != "" {
		target = repoTag
	}

	if !force {
		inspect, _, err := cli.ImageInspectWithRaw(c.Request.Context(), target)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "获取镜像详情失败", err)
			return
		}
		used, err := containersUsingImage(c.Request.Context(), cli, inspect.ID, repoTag)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "检查镜像使用状态失败", err)
			return
		}
		if len(used) > 0 {
			c.JSON(http.StatusConflict, gin.H{
				"code":       "IMAGE_IN_USE",
				"message":    "镜像正在被容器使用，拒绝删除",
				"error":      "镜像正在被容器使用，拒绝删除",
				"containers": used,
			})
			return
		}
	}

	report, err := cli.ImageRemove(c.Request.Context(), target, types.ImageRemoveOptions{
		Force:         force,
		PruneChildren: true,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "删除镜像失败", err)
		return
	}

	// 同步清理被移除 tag 的镜像更新记录：按 daemon 返回的 Untagged 清单精确清理，
	// 多 tag 镜像只删其中一个 tag 时，其余 tag 的记录与 notified 状态不受影响
	clearImageUpdateRecordsByImageRefs(removedImageTagsFromDeleteItems(report)...)

	c.JSON(http.StatusOK, gin.H{"message": "镜像已删除"})
}

// removedImageTagsFromDeleteItems 从镜像删除响应中提取被解除的 tag 清单。
func removedImageTagsFromDeleteItems(items []types.ImageDeleteResponseItem) []string {
	var tags []string
	for _, item := range items {
		if tag := strings.TrimSpace(item.Untagged); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// 标签处理
func tagImage(c *gin.Context) {
	var req struct {
		ID   string `json:"id"`
		Repo string `json:"repo"`
		Tag  string `json:"tag"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	newTag := fmt.Sprintf("%s:%s", req.Repo, req.Tag)

	if err := cli.ImageTag(c.Request.Context(), req.ID, newTag); err != nil {
		respondError(c, http.StatusInternalServerError, "修改标签失败", err)
		return
	}

	// 改名场景：旧 tag 从镜像上消失后，挂在它上面的更新记录即幽灵记录，同步清理，
	// 避免下轮检测沿旧 tag 重建记录造成重复通知
	cleanupRetaggedSourceImageUpdateRecords(c.Request.Context(), cli, req.ID, newTag)

	c.JSON(http.StatusOK, gin.H{"message": "标签修改成功"})
}

// imageRetagCleanupClient 是改名记录清理需要的最小 docker 客户端面。
type imageRetagCleanupClient interface {
	ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
}

// cleanupRetaggedSourceImageUpdateRecords 处理改名（retag）场景：源引用是镜像已有
// tag，且打新标签后该 tag 已从镜像上消失（真改名，而非追加标签）时，清理挂在旧
// tag 上的更新记录。追加标签（旧 tag 仍在场）与镜像 ID 引用不动记录及其 notified
// 状态，避免记录被误删后重建导致同一远端目标重复通知。
func cleanupRetaggedSourceImageUpdateRecords(ctx context.Context, cli imageRetagCleanupClient, source, newTag string) {
	if source == "" || source == newTag {
		return
	}
	before, _, err := cli.ImageInspectWithRaw(ctx, source)
	if err != nil {
		// 源引用解析不出镜像已有 tag（ID 引用或 tag 已不存在），没有可清理的旧记录
		return
	}
	sourceWasTag := false
	for _, tag := range before.RepoTags {
		if tag == source {
			sourceWasTag = true
			break
		}
	}
	if !sourceWasTag {
		return
	}
	after, _, err := cli.ImageInspectWithRaw(ctx, newTag)
	if err != nil {
		return
	}
	for _, tag := range after.RepoTags {
		if tag == source {
			return
		}
	}
	clearImageUpdateRecordsByImageRefs(source)
}

// 导出镜像
func exportImage(c *gin.Context) {
	imageID := c.Param("id")

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	inspect, _, err := cli.ImageInspectWithRaw(c.Request.Context(), imageID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取镜像信息失败", err)
		return
	}

	fileName := strings.TrimPrefix(imageID, "sha256:")[:12]
	if len(inspect.RepoTags) > 0 {
		fileName = strings.Replace(inspect.RepoTags[0], "/", "_", -1)
		fileName = strings.Replace(fileName, ":", "_", -1)
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.tar", fileName))
	c.Header("Content-Type", "application/x-tar")

	var names []string
	if len(inspect.RepoTags) > 0 {
		names = inspect.RepoTags
	} else {
		names = []string{imageID}
	}

	logging.Info("image export started", "image_count", len(names))

	reader, err := cli.ImageSave(c.Request.Context(), names)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "导出镜像失败", err)
		return
	}
	defer reader.Close()

	_, err = io.Copy(c.Writer, reader)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "写入响应失败", err)
		return
	}
}

type imageUpdateInfo struct {
	RepoTag      string `json:"repoTag"`
	LocalDigest  string `json:"localDigest"`
	RemoteDigest string `json:"remoteDigest"`
	Notified     bool   `json:"notified"`
}

type imageUpdateCheckResult struct {
	Updates            []imageUpdateInfo
	TotalImages        int
	FoundUpdates       int
	RemoteErrors       int
	SkippedBackoff     int
	SkippedUnavailable int
	WriteErrors        int
	Duration           time.Duration
}

var imageUpdateLastRun int64

func formatImageUpdateMessage(repoTags []string) string {
	tags := make([]string, 0, len(repoTags))
	seen := make(map[string]struct{}, len(repoTags))
	for _, t := range repoTags {
		tt := strings.TrimSpace(t)
		if tt == "" {
			continue
		}
		if _, ok := seen[tt]; ok {
			continue
		}
		seen[tt] = struct{}{}
		tags = append(tags, tt)
	}
	if len(tags) == 0 {
		return ""
	}

	sort.Strings(tags)
	display := tags
	if len(display) > 3 {
		display = display[:3]
	}
	suffix := ""
	if len(tags) > 3 {
		suffix = fmt.Sprintf(" 等 %d 个", len(tags))
	}
	return fmt.Sprintf("检测到镜像更新：%s%s", strings.Join(display, "、"), suffix)
}

type imageReconcileAction int

const (
	// imageReconcileSkip 跳过本轮远程检查（本地未变化且非 force，或非 force 下本地无法核验）
	imageReconcileSkip imageReconcileAction = iota
	// imageReconcileRecheck 旧记录可能失效（本地已追平/已变化，或 force 下本地无法核验），
	// 继续远程复查；记录的删除或原地更新由 decideImageUpdateRecordWrite 按远端结果决定
	imageReconcileRecheck
	// imageReconcileCheck 直接进行远程检查（无旧记录或 force，localDigest 可能为空）
	imageReconcileCheck
)

type imageReconcileDecision struct {
	action      imageReconcileAction
	localDigest string
}

// imageRepoNames 返回镜像引用可能对应的仓库名归一化变体集合（口径同 normalizeImageVariants）
func imageRepoNames(ref string) map[string]struct{} {
	names := map[string]struct{}{}
	for _, variant := range normalizeImageVariants(ref) {
		name, _ := splitImageRef(variant)
		if name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}

// selectLocalRepoDigest 从 ImageList 返回的 RepoDigests 中选出 repo 部分与 repoTag
// 匹配的 digest。RepoDigests 顺序不保证且可能包含无关仓库（多 tag/多仓库/经 mirror
// 拉取），无脑取 [0] 会把旧 digest 当成本地状态。无匹配且仅有一条时回退使用该条；
// 多条均不匹配视为无本地 digest。
func selectLocalRepoDigest(repoTag string, repoDigests []string) string {
	want := imageRepoNames(repoTag)
	single := ""
	valid := 0
	for _, entry := range repoDigests {
		repo, digest, found := strings.Cut(entry, "@")
		if !found || strings.TrimSpace(digest) == "" {
			continue
		}
		valid++
		single = strings.TrimSpace(digest)
		for name := range imageRepoNames(repo) {
			if _, ok := want[name]; ok {
				return single
			}
		}
	}
	if valid == 1 {
		return single
	}
	return ""
}

// decideImageUpdateReconcile 是单条 repoTag 的对账决策纯函数：根据本地 digest、
// 已有更新记录和 force 决定本轮跳过还是继续远程检查。
func decideImageUpdateReconcile(repoTag string, repoDigests []string, old database.ImageUpdate, hasOld bool, force bool) imageReconcileDecision {
	localDigest := selectLocalRepoDigest(repoTag, repoDigests)
	if localDigest == "" {
		// 本地无法核验（无 RepoDigests 或多 registry retag）：force 语义要求继续
		// 远程检查，有旧记录时复查；非 force 不为不可核验镜像浪费远程调用，有旧记录
		// 时保留原地跳过（避免 force 建/非 force 删的每日提醒环），无记录直接跳过。
		if force {
			if hasOld {
				return imageReconcileDecision{action: imageReconcileRecheck}
			}
			return imageReconcileDecision{action: imageReconcileCheck}
		}
		return imageReconcileDecision{action: imageReconcileSkip}
	}
	if hasOld {
		// 本地 digest 已追平远端 digest，或本地已不同于记录值：旧记录失效，重新检查远端
		if old.RemoteDigest != "" && localDigest == old.RemoteDigest {
			return imageReconcileDecision{action: imageReconcileRecheck, localDigest: localDigest}
		}
		if old.LocalDigest != "" && localDigest != old.LocalDigest {
			return imageReconcileDecision{action: imageReconcileRecheck, localDigest: localDigest}
		}
		if !force {
			return imageReconcileDecision{action: imageReconcileSkip, localDigest: localDigest}
		}
	}
	return imageReconcileDecision{action: imageReconcileCheck, localDigest: localDigest}
}

// repoDigestValues 提取全部 RepoDigests 条目的 digest 部分，用于"远端 digest ∈
// 本地 digest 集合"的 any-match 追平判定。digest 字符串本身无需归一化，按原值比较；
// 归一化只作用于 repo 匹配（见 selectLocalRepoDigest）。重复 digest 只保留一份。
func repoDigestValues(repoDigests []string) []string {
	values := make([]string, 0, len(repoDigests))
	seen := make(map[string]struct{}, len(repoDigests))
	for _, entry := range repoDigests {
		_, digest, found := strings.Cut(entry, "@")
		digest = strings.TrimSpace(digest)
		if !found || digest == "" {
			continue
		}
		if _, ok := seen[digest]; ok {
			continue
		}
		seen[digest] = struct{}{}
		values = append(values, digest)
	}
	return values
}

// decideImageUpdateRecordWrite 是远端结果落地纯函数：远端 digest 命中本地 digest
// 集合即视为已追平（调用方删除记录）；否则生成原地更新的记录。远端目标
// （remote_digest）未变时沿用旧记录的 notified（同一远端版本只提醒一次），
// 目标前进或无旧记录时视为新通知并写 notified=false。
func decideImageUpdateRecordWrite(old database.ImageUpdate, hasOld bool, localDigests []string, selectedLocal, remoteDigest string) (upToDate bool, record database.ImageUpdate, notify bool) {
	for _, digest := range localDigests {
		if digest != "" && digest == remoteDigest {
			return true, database.ImageUpdate{}, false
		}
	}
	record = database.ImageUpdate{
		RepoTag:      old.RepoTag,
		LocalDigest:  selectedLocal,
		RemoteDigest: remoteDigest,
	}
	if hasOld && old.RemoteDigest == remoteDigest {
		record.Notified = old.Notified
		return false, record, false
	}
	return false, record, true
}

func runImageUpdateCheck(ctx context.Context, force bool) (imageUpdateCheckResult, error) {
	if !imageUpdateCheckInFlight.TryLock() {
		return imageUpdateCheckResult{}, errImageUpdateCheckInFlight
	}
	defer imageUpdateCheckInFlight.Unlock()

	start := time.Now()
	result := imageUpdateCheckResult{}

	cli, err := docker.NewDockerClient()
	if err != nil {
		return result, err
	}
	defer cli.Close()

	images, err := cli.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		return result, err
	}
	result.TotalImages = len(images)

	// 获取已有的更新记录，避免重复轮询同一镜像标签
	existingUpdates, err := database.GetAllImageUpdates()
	if err != nil {
		return result, err
	}
	existingByTag := make(map[string]database.ImageUpdate, len(existingUpdates))
	for _, u := range existingUpdates {
		if u.RepoTag != "" {
			existingByTag[u.RepoTag] = u
		}
	}

	var updates []imageUpdateInfo
	remoteErrors := 0
	writeErrors := 0

	// 顺手收集本地镜像 ID 与 RepoTags 集合，供主循环结束后清理已删除镜像的残留记录
	localImageIDs := make(map[string]struct{}, len(images))
	localRepoTags := make(map[string]struct{})

	// registry-mirrors 每次运行只读一次，沿镜像循环向下传（读取失败按无镜像处理）
	mirrors := daemonRegistryMirrors()

	for _, img := range images {
		localImageIDs[img.ID] = struct{}{}
		for _, tag := range img.RepoTags {
			localRepoTags[tag] = struct{}{}
			if tag == "<none>:<none>" {
				continue
			}
			old, hasOld := existingByTag[tag]
			decision := decideImageUpdateReconcile(tag, img.RepoDigests, old, hasOld, force)
			localDigest := decision.localDigest
			switch decision.action {
			case imageReconcileSkip:
				// 仍是旧版本且记录存在（非 force），或非 force 下本地无法核验，跳过本轮检测
				continue
			case imageReconcileRecheck:
				// 旧记录可能失效（本地追平远端、本地 digest 已变化，或 force 下本地无法
				// 核验）：继续远程复查；记录不再先删后插，删除或原地更新由远端结果决定
			case imageReconcileCheck:
			}

			if force {
				_ = database.ResetImageRemoteDigestStatus(tag)
			} else {
				st, serr := database.GetImageRemoteDigestStatus(tag)
				if serr == nil {
					if st.Unavailable {
						result.SkippedUnavailable++
						continue
					}
					if t, ok := database.ParseSQLiteTime(st.NextCheckAt); ok && time.Now().Before(t) {
						result.SkippedBackoff++
						continue
					}
				}
			}

			localDigestValues := repoDigestValues(img.RepoDigests)
			remoteDigest, remoteUpToDate, derr := imageRemoteDigestResolve(ctx, tag, localDigestValues, mirrors)
			if derr != nil {
				remoteErrors++
				recordImageRemoteDigestFailureIfLive(ctx, tag, derr)
				continue
			}
			_ = database.ResetImageRemoteDigestStatus(tag)
			if remoteDigest == "" {
				continue
			}
			if remoteUpToDate {
				// 直连链任一来源（含 index 子项展开）命中本地 RepoDigests 集合，镜像已追平，清除更新记录
				clearImageUpdateRecordsByImageRefs(tag)
				delete(existingByTag, tag)
				continue
			}
			upToDate, record, notify := decideImageUpdateRecordWrite(old, hasOld, localDigestValues, localDigest, remoteDigest)
			if upToDate {
				// 远端 digest 已命中本地 RepoDigests 集合，镜像已追平，清除更新记录
				clearImageUpdateRecordsByImageRefs(tag)
				delete(existingByTag, tag)
				continue
			}
			record.RepoTag = tag
			record.ImageID = img.ID
			updates = append(updates, imageUpdateInfo{
				RepoTag:      tag,
				LocalDigest:  record.LocalDigest,
				RemoteDigest: record.RemoteDigest,
				Notified:     record.Notified,
			})
			logging.Debug("image update record write", "image", tag, "notify", notify)

			if err := database.SaveImageUpdate(&record); err != nil {
				writeErrors++
				logging.Error("image update state persistence failed", "image", tag, "error", err)
			}
		}
	}

	// 清理镜像已从本机删除后残留的 image_updates 记录，避免过期的"有更新"角标/通知
	cleanupStaleImageUpdateRecords(localImageIDs, localRepoTags)

	result.Updates = updates
	result.FoundUpdates = len(updates)
	result.RemoteErrors = remoteErrors
	result.WriteErrors = writeErrors
	result.Duration = time.Since(start)

	return result, nil
}

// recordImageRemoteDigestFailureIfLive 在运行 ctx 仍然存活时记录远端 digest 获取
// 失败并写退避表；ctx 已取消（前端断连/手动中断）时整体跳过——整个 run 正在拆除，
// 剩余 tag 的失败不是 registry 的错，不应毒化退避（连续断连会把镜像标记成
// unavailable）。daemon 兜底走 Background，不受调用方 ctx 取消影响。
func recordImageRemoteDigestFailureIfLive(ctx context.Context, tag string, resolveErr error) {
	if ctx == nil || ctx.Err() != nil {
		return
	}
	policy := settings.GetImageRemoteDigestBackoffPolicy()
	_, _ = database.RecordImageRemoteDigestFailure(tag, resolveErr.Error(), policy.FirstFailBackoff, policy.SecondFailBackoff, policy.MaxConsecutiveFail)
	if allowRemoteDigestErrorLog(tag) {
		logging.Warn("remote image digest check failed", "image", tag, "error", resolveErr)
		system.LogSimpleEvent("error", fmt.Sprintf("镜像远端摘要获取失败: %s, 错误: %v", tag, resolveErr))
	}
}

// saveImageUpdateNotification 是 database.SaveNotification 的注入点，便于测试
// 通知写库失败时的 at-least-once 行为。
var saveImageUpdateNotification = database.SaveNotification

// imageUpdateNotificationCategoryEnabled 判定镜像更新通知使用的 system 分类当前
// 是否启用。读取全局通知分类设置（settings 层与 database 层分类门控同一份
// global_settings 数据，此处用既有读函数避免改动共享的 db 层）；读取失败按启用
// 处理，最终投递仍由 SaveNotification 的分类门控兜底。
func imageUpdateNotificationCategoryEnabled() bool {
	s, err := settings.GetSettings()
	if err != nil {
		return true
	}
	for _, category := range s.NotificationEnabledCategories {
		if strings.EqualFold(strings.TrimSpace(category), "system") {
			return true
		}
	}
	return false
}

// deliverImageUpdateNotifications 将一轮检测确认的未通知镜像更新投递为系统通知，
// 并在投递成功后标记 notified。两条"不消费 notified"的路径：
//   - 通知分类被用户关闭：整段跳过，记录保留 unnotified，用户重开分类后下轮补发；
//   - 通知写库失败：记录日志后不标记，下轮调度重发（at-least-once）。
func deliverImageUpdateNotifications(unnotified []database.ImageUpdate) {
	if !imageUpdateNotificationCategoryEnabled() {
		// 分类被关闭：不投递也不标记，记录保留 unnotified，重开分类后下轮补发
		return
	}
	repoTags := make([]string, 0, len(unnotified))
	for _, u := range unnotified {
		repoTags = append(repoTags, u.RepoTag)
	}
	msg := formatImageUpdateMessage(repoTags)
	if msg != "" {
		system.LogSimpleEvent("info", msg)
		if err := saveImageUpdateNotification(&database.Notification{Type: "info", Category: "system", Message: msg, Read: false}); err != nil {
			logging.Error("image update notification save failed; records stay unnotified for retry", "error", err)
			system.LogSimpleEvent("error", fmt.Sprintf("镜像更新通知保存失败，下轮检测将重试: %v", err))
			return
		}
	}
	if err := database.MarkImageUpdatesNotifiedByRepoTags(repoTags); err != nil {
		logging.Error("image update notified mark failed", "error", err)
	}
}

func StartImageUpdateScheduler() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s, err := settings.GetSettings()
			if err != nil {
				logging.Warn("image update scheduler could not load settings", "error", err)
				continue
			}
			interval := s.ImageUpdateIntervalMinutes
			if interval <= 0 {
				interval = 30
			}

			last := time.Unix(atomic.LoadInt64(&imageUpdateLastRun), 0)
			if !last.IsZero() && time.Since(last) < time.Duration(interval)*time.Minute {
				continue
			}

			ctx := context.Background()
			result, cerr := runImageUpdateCheck(ctx, false)
			now := time.Now()
			atomic.StoreInt64(&imageUpdateLastRun, now.Unix())

			if cerr != nil {
				system.LogSimpleEvent("error", fmt.Sprintf("自动镜像更新检测失败: %v", cerr))
				continue
			}

			if result.WriteErrors > 0 {
				system.LogSimpleEvent("error", fmt.Sprintf(
					"自动镜像更新检测部分失败: 总镜像 %d, 可更新 %d, 写库失败 %d, 远端错误 %d, 耗时 %.1fs",
					result.TotalImages,
					result.FoundUpdates,
					result.WriteErrors,
					result.RemoteErrors,
					result.Duration.Seconds(),
				))
				continue
			}

			unnotified, err := database.GetUnnotifiedImageUpdates()
			if err != nil {
				system.LogSimpleEvent("error", fmt.Sprintf("读取待通知的镜像更新失败: %v", err))
				continue
			}
			if len(unnotified) > 0 {
				deliverImageUpdateNotifications(unnotified)
			} else if result.RemoteErrors > 0 {
				system.LogSimpleEvent("warning", fmt.Sprintf(
					"自动镜像更新检测存在远端错误: 总镜像 %d, 远端错误 %d, 耗时 %.1fs",
					result.TotalImages,
					result.RemoteErrors,
					result.Duration.Seconds(),
				))
			}
		}
	}()
}

func checkImageUpdates(c *gin.Context) {
	force := false
	v := strings.TrimSpace(strings.ToLower(c.Query("force")))
	if v == "1" || v == "true" || v == "yes" || v == "on" {
		force = true
	}
	result, err := runImageUpdateCheck(c.Request.Context(), force)
	if err != nil {
		if errors.Is(err, errImageUpdateCheckInFlight) {
			respondError(c, http.StatusConflict, "镜像更新检测正在运行，请稍后再试", nil)
			return
		}
		system.LogSimpleEvent("error", fmt.Sprintf("手动镜像更新检测失败: %v", err))
		respondError(c, http.StatusInternalServerError, "写入镜像更新记录失败", err)
		return
	}
	if result.WriteErrors > 0 {
		system.LogSimpleEvent("error", fmt.Sprintf(
			"手动镜像更新检测部分失败: 总镜像 %d, 可更新 %d, 写库失败 %d, 远端错误 %d, 耗时 %.1fs",
			result.TotalImages,
			result.FoundUpdates,
			result.WriteErrors,
			result.RemoteErrors,
			result.Duration.Seconds(),
		))
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("写入镜像更新记录失败: %d 条", result.WriteErrors), nil)
		return
	}
	if result.FoundUpdates > 0 {
		repoTags := make([]string, 0, len(result.Updates))
		for _, u := range result.Updates {
			repoTags = append(repoTags, u.RepoTag)
		}
		msg := formatImageUpdateMessage(repoTags)
		if msg != "" {
			system.LogSimpleEvent("info", msg)
		}
		// 手动检查的结果已直接展示给用户，标记 notified，下个调度 tick 不再
		// 重复推送同一批已知更新
		markManualImageUpdatesSeen(result.Updates)
	} else if result.RemoteErrors > 0 {
		system.LogSimpleEvent("warning", fmt.Sprintf(
			"手动镜像更新检测存在远端错误: 总镜像 %d, 远端错误 %d, 耗时 %.1fs",
			result.TotalImages,
			result.RemoteErrors,
			result.Duration.Seconds(),
		))
	}
	c.JSON(http.StatusOK, gin.H{
		"updates":            result.Updates,
		"remoteErrors":       result.RemoteErrors,
		"skippedBackoff":     result.SkippedBackoff,
		"skippedUnavailable": result.SkippedUnavailable,
	})
}

func listStoredImageUpdates(c *gin.Context) {
	items, err := database.GetAllImageUpdates()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取镜像更新记录失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"updates": storedImageUpdateInfos(items)})
}

// storedImageUpdateInfos 把 image_updates 记录转换为接口返回结构，与容器角标
// 缓存同口径过滤掉 LocalDigest == RemoteDigest 的记录（已追平/历史脏数据不展示，
// 避免前端出现"可更新"的误导信息）。
func storedImageUpdateInfos(items []database.ImageUpdate) []imageUpdateInfo {
	var updates []imageUpdateInfo
	for _, item := range items {
		if item.LocalDigest == item.RemoteDigest {
			continue
		}
		updates = append(updates, imageUpdateInfo{
			RepoTag:      item.RepoTag,
			LocalDigest:  item.LocalDigest,
			RemoteDigest: item.RemoteDigest,
			Notified:     item.Notified,
		})
	}
	return updates
}

// markManualImageUpdatesSeen 手动检查的结果已在 UI 直接展示，标记本轮 updates
// notified，避免下个调度 tick 对同一批已知更新重复推送通知。检查失败的调用方
// 在标记之前就已返回，不会走到这里。
func markManualImageUpdatesSeen(updates []imageUpdateInfo) {
	if len(updates) == 0 {
		return
	}
	repoTags := make([]string, 0, len(updates))
	for _, u := range updates {
		repoTags = append(repoTags, u.RepoTag)
	}
	if err := database.MarkImageUpdatesNotifiedByRepoTags(repoTags); err != nil {
		logging.Error("manual image update notified mark failed", "error", err)
	}
}

func clearImageUpdateRecordsByImageRefs(refs ...string) {
	seen := make(map[string]struct{})
	for _, ref := range refs {
		for _, repoTag := range normalizeImageVariants(ref) {
			repoTag = strings.TrimSpace(repoTag)
			if repoTag == "" {
				continue
			}
			if _, ok := seen[repoTag]; ok {
				continue
			}
			seen[repoTag] = struct{}{}
			_ = database.DeleteImageUpdateByRepoTag(repoTag)
		}
	}
	if len(seen) > 0 {
		invalidateImageUpdateMapCache()
	}
}

// staleImageUpdateRepoTags 汇总需要清理的 image_updates 记录 repo_tag：
// image_id 非空且镜像已删除；或镜像在场（以及 image_id 为空的退化记录）但
// repo_tag 不在本地 RepoTags 的全量变体集合里（tag 已从镜像上移除，即
// docker rmi <tag> 删的是 tag 引用而非镜像本体）。在场镜像的记录只要 repo_tag
// 命中任一本地拼写变体即保留，retag/多拼写场景下不会被误删后重建（重建会让
// notified 归零，对同一远端目标重复通知）。
func staleImageUpdateRepoTags(existing []database.ImageUpdate, localImageIDs, localRepoTagVariants map[string]struct{}) []string {
	var stale []string
	for _, u := range existing {
		if u.ImageID != "" {
			if _, ok := localImageIDs[u.ImageID]; !ok {
				stale = append(stale, u.RepoTag)
				continue
			}
			if _, ok := localRepoTagVariants[u.RepoTag]; !ok {
				stale = append(stale, u.RepoTag)
			}
			continue
		}
		if _, ok := localRepoTagVariants[u.RepoTag]; !ok {
			stale = append(stale, u.RepoTag)
		}
	}
	return stale
}

// localRepoTagVariantSet 把本地 RepoTags 集合展开为全量拼写变体集合
// （normalizeImageVariants 并集）：同一镜像可能被以短名/docker.io 前缀/
// library 前缀或 repo:tag@sha256 等形式引用，记录的 repo_tag 命中任一变体
// 即视为在场。
func localRepoTagVariantSet(localRepoTags map[string]struct{}) map[string]struct{} {
	variants := make(map[string]struct{}, len(localRepoTags))
	for tag := range localRepoTags {
		for _, variant := range normalizeImageVariants(tag) {
			variants[variant] = struct{}{}
		}
	}
	return variants
}

// cleanupStaleImageUpdateRecords 在主循环结束后清理一次已删除镜像残留的
// image_updates 记录。读取或删除失败只记日志，不影响本轮检查结果。
func cleanupStaleImageUpdateRecords(localImageIDs, localRepoTags map[string]struct{}) {
	existing, err := database.GetAllImageUpdates()
	if err != nil {
		logging.Warn("stale image update records cleanup skipped", "error", err)
		return
	}
	removed := false
	for _, repoTag := range staleImageUpdateRepoTags(existing, localImageIDs, localRepoTagVariantSet(localRepoTags)) {
		if err := database.DeleteImageUpdateByRepoTag(repoTag); err != nil {
			logging.Warn("stale image update record cleanup failed", "image", repoTag, "error", err)
			continue
		}
		removed = true
	}
	if removed {
		invalidateImageUpdateMapCache()
	}
}

func clearImageUpdate(c *gin.Context) {
	var req struct {
		RepoTag string `json:"repoTag"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}
	if req.RepoTag == "" {
		respondError(c, http.StatusBadRequest, "repoTag is required", nil)
		return
	}
	clearImageUpdateRecordsByImageRefs(req.RepoTag)
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func applyImageUpdates(c *gin.Context) {
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	items, err := database.GetAllImageUpdates()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取镜像更新信息失败", err)
		return
	}

	containers, err := cli.ContainerList(c.Request.Context(), types.ContainerListOptions{All: true})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取容器列表失败", err)
		return
	}

	usedImageIDs := make(map[string]bool)
	usedImageTags := make(map[string]bool)
	for _, ctr := range containers {
		if ctr.ImageID != "" {
			usedImageIDs[ctr.ImageID] = true
		}
		if ctr.Image != "" {
			usedImageTags[ctr.Image] = true
		}
	}

	total := len(items)
	attempted := 0
	success := 0
	failed := 0
	skippedUsed := 0
	var failedTags []string

	for index, item := range items {
		// 计算镜像是否正在使用中
		used := false
		if item.ImageID != "" && usedImageIDs[item.ImageID] {
			used = true
		}
		if usedImageTags[item.RepoTag] {
			used = true
		}
		if used {
			skippedUsed++
			system.LogSimpleEvent("warning", fmt.Sprintf("跳过正在使用中的镜像: %s", item.RepoTag))
			continue
		}

		attempted++

		// 为非 docker.io 的镜像设置仓库认证
		pullOpts := types.ImagePullOptions{}
		if name, _ := parseImageName(item.RepoTag); name != "" {
			if host := imageHost(name); host != "" {
				if regs, e := database.GetAllRegistries(); e == nil {
					if r := matchRegistry(regs, host); r != nil && (r.Username != "" || r.Password != "") {
						authCfg := types.AuthConfig{
							Username: r.Username,
							Password: r.Password,
						}
						if r.URL != "" {
							authCfg.ServerAddress = r.URL
						}
						if enc, mErr := json.Marshal(authCfg); mErr == nil {
							pullOpts.RegistryAuth = base64.URLEncoding.EncodeToString(enc)
						}
					}
				}
			}
		}

		cleanupResult, err := pullImageWithOldCleanup(
			c.Request.Context(),
			cli,
			item.RepoTag,
			pullOpts,
			fmt.Sprintf("batch-%d-%d", time.Now().UnixNano(), index),
			true,
			nil,
		)
		if err != nil {
			failed++
			failedTags = append(failedTags, item.RepoTag)
			system.LogSimpleEvent("error", fmt.Sprintf("镜像拉取失败: %s, 错误: %v", item.RepoTag, err))
			continue
		}

		success++
		if cleanupResult.CleanupError != "" {
			system.LogSimpleEvent("warning", fmt.Sprintf("镜像已更新，但旧镜像清理失败: %s, 错误: %s", item.RepoTag, cleanupResult.CleanupError))
		} else {
			system.LogSimpleEvent("success", fmt.Sprintf("镜像更新并清理旧镜像成功: %s", item.RepoTag))
		}
		clearImageUpdateRecordsByImageRefs(item.RepoTag)
	}

	if failed > 0 {
		system.LogSimpleEvent("warning", fmt.Sprintf(
			"批量镜像更新完成: 待更新 %d, 实际尝试 %d, 成功 %d, 失败 %d, 跳过使用中 %d",
			total,
			attempted,
			success,
			failed,
			skippedUsed,
		))
	} else {
		system.LogSimpleEvent("success", fmt.Sprintf(
			"批量镜像更新完成: 待更新 %d, 实际尝试 %d, 成功 %d, 跳过使用中 %d",
			total,
			attempted,
			success,
			skippedUsed,
		))
	}

	c.JSON(http.StatusOK, gin.H{
		"total":       total,
		"attempted":   attempted,
		"success":     success,
		"failed":      failed,
		"skippedUsed": skippedUsed,
		"failedTags":  failedTags,
	})
}

func getDockerHubDigest(repoTag string) (string, error) {
	name, tag := parseImageName(repoTag)
	if name == "" {
		return "", fmt.Errorf("invalid image")
	}

	host := imageHost(name)
	fullRef := ""
	if host == "" {
		path := name
		if !strings.Contains(name, "/") {
			path = "library/" + name
		}
		fullRef = "docker.io/" + path + ":" + tag
	} else {
		fullRef = name + ":" + tag
	}

	cli, err := docker.NewDockerClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()

	// 私有仓库兜底同样需要凭证：非 docker.io 按 host 匹配；docker.io 短名
	// （无 host）按 docker.io 查已存凭证（index.docker.io / registry-1.docker.io
	// 别名由 matchRegistry 后缀匹配命中）。
	credentialHosts := []string{host}
	if host == "" {
		credentialHosts = []string{"docker.io"}
	}
	encoded := ""
	if regs, e := database.GetAllRegistries(); e == nil {
		for _, h := range credentialHosts {
			r := matchRegistry(regs, h)
			if r == nil || (r.Username == "" && r.Password == "") {
				continue
			}
			auth := registrytypes.AuthConfig{
				Username:      r.Username,
				Password:      r.Password,
				ServerAddress: r.URL,
			}
			if s, ee := registrytypes.EncodeAuthConfig(auth); ee == nil {
				encoded = s
			}
			break
		}
	}

	di, err := cli.DistributionInspect(context.Background(), fullRef, encoded)
	if err != nil {
		return "", err
	}

	d := string(di.Descriptor.Digest)
	if d == "" {
		return "", fmt.Errorf("empty digest")
	}
	return d, nil
}

// daemonRegistryMirrors 读取 daemon.json 的 registry-mirrors（每次 runImageUpdateCheck
// 运行读一次）；读取失败按无镜像处理。
func daemonRegistryMirrors() []string {
	cfg, err := docker.GetDaemonConfig()
	if err != nil || cfg == nil {
		if err != nil {
			logging.Debug("read daemon config for registry mirrors failed", "error", err)
		}
		return nil
	}
	return cfg.RegistryMirrors
}

// registryCredentialForRepoTag 复用 matchRegistry/GetAllRegistries 的凭证匹配思路：
// 非 docker.io 镜像按 host 精确匹配；docker.io 镜像按 docker.io /
// registry-1.docker.io 查已存凭证（私有仓库 Basic 换 Bearer token）。pkg 层接收
// 凭证作为参数，不直接依赖 database。
func registryCredentialForRepoTag(repoTag string) *registrycheck.Credential {
	name, _ := parseImageName(repoTag)
	host := imageHost(name)
	hosts := []string{host}
	if host == "" {
		hosts = []string{"docker.io", "registry-1.docker.io"}
	}
	regs, err := database.GetAllRegistries()
	if err != nil {
		return nil
	}
	for _, h := range hosts {
		if h == "" {
			continue
		}
		if r := matchRegistry(regs, h); r != nil && (r.Username != "" || r.Password != "") {
			return &registrycheck.Credential{
				Username:      r.Username,
				Password:      r.Password,
				ServerAddress: r.URL,
			}
		}
	}
	return nil
}

// resolveRemoteImageDigestViaChain 通过直连 registry 优先级链解析远端 digest
// （docker.io：mirrors 依次 → 本体；非 docker.io：直连该 host + 已存凭证）。
// 任一来源 digest（含 index 子项展开）∈ 本地集合即"已最新"；全链失败回退 daemon
// DistributionInspect 兜底（保底=今天行为）。
func resolveRemoteImageDigestViaChain(ctx context.Context, repoTag string, localDigests []string, mirrors []string) (string, bool, error) {
	res, err := registryDigestResolver.Resolve(ctx, repoTag, registrycheck.Options{
		Mirrors:      mirrors,
		Credential:   imageRegistryCredential(repoTag),
		LocalDigests: localDigests,
	})
	if err == nil {
		logging.Debug("image remote digest resolved via registry chain",
			"image", repoTag, "source", res.Source, "host", res.SourceHost,
			"upToDate", res.UpToDate, "digest", res.Digest)
		return res.Digest, res.UpToDate, nil
	}
	logging.Debug("registry chain digest check failed, falling back to daemon inspect", "image", repoTag, "error", err)
	digest, derr := daemonImageDigestFallback(repoTag)
	if derr != nil {
		return "", false, fmt.Errorf("registry 直连检查失败(%v)，daemon 兜底检查失败(%v)", err, derr)
	}
	logging.Debug("image remote digest resolved via daemon fallback", "image", repoTag, "source", "daemon", "digest", digest)
	return digest, false, nil
}

func imageHost(name string) string {
	segments := strings.Split(name, "/")
	if len(segments) == 0 {
		return ""
	}
	host := segments[0]
	if strings.Contains(host, ".") || strings.Contains(host, ":") || host == "localhost" {
		return host
	}
	return ""
}

// registryKeyHostname 归一 registry 记录的 key/URL 为 hostname（strip scheme、
// 去路径和尾部斜杠，保留端口）。
func registryKeyHostname(key string) string {
	k := strings.TrimSpace(key)
	k = strings.TrimPrefix(k, "https://")
	k = strings.TrimPrefix(k, "http://")
	if i := strings.Index(k, "/"); i >= 0 {
		k = k[:i]
	}
	return strings.TrimRight(strings.TrimSpace(k), "/")
}

// matchRegistry 按 host 在已存 registry 中确定性匹配：strip scheme 后 hostname
// 精确匹配优先（字面 host 最先；docker.io 别名族由 registrycheck.EquivalentHosts
// 展开，任意别名下保存的凭证互相命中）；无命中再按 "." 边界后缀匹配（host 以
// "."+registry hostname 结尾，杜绝 notdocker.io 误配 docker.io）；多条命中按
// registry URL 字典序取第一个，消除 map 遍历随机。
func matchRegistry(regs map[string]*database.Registry, host string) *database.Registry {
	if len(regs) == 0 {
		return nil
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	hostLower := strings.ToLower(host)
	hosts := registrycheck.EquivalentHosts(host)
	hostSet := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		hostSet[h] = struct{}{}
	}
	inHostSet := func(h string) bool {
		_, ok := hostSet[strings.ToLower(strings.TrimSpace(h))]
		return ok
	}
	// pick 按 registry URL 字典序取第一个，保证结果与 map 遍历顺序无关。
	pick := func(keys []string) *database.Registry {
		sort.Strings(keys)
		return regs[keys[0]]
	}

	// 1) 字面 hostname 精确匹配
	var exactKeys []string
	for key := range regs {
		if strings.ToLower(registryKeyHostname(key)) == hostLower {
			exactKeys = append(exactKeys, key)
		}
	}
	if len(exactKeys) > 0 {
		return pick(exactKeys)
	}

	// 2) docker.io 别名族等价匹配（字面匹配无命中时，族内任意别名凭证互相命中）
	var familyKeys []string
	for key := range regs {
		h := strings.ToLower(registryKeyHostname(key))
		if h == hostLower || !inHostSet(h) {
			continue
		}
		familyKeys = append(familyKeys, key)
	}
	if len(familyKeys) > 0 {
		return pick(familyKeys)
	}

	// 3) "." 边界后缀匹配
	var suffixKeys []string
	for key := range regs {
		h := registryKeyHostname(key)
		if h == "" || inHostSet(h) {
			continue
		}
		if strings.HasSuffix(hostLower, "."+strings.ToLower(h)) {
			suffixKeys = append(suffixKeys, key)
		}
	}
	if len(suffixKeys) > 0 {
		return pick(suffixKeys)
	}
	return nil
}

func parseImageName(repoTag string) (string, string) {
	if repoTag == "" {
		return "", ""
	}
	lastSlash := strings.LastIndex(repoTag, "/")
	lastColon := strings.LastIndex(repoTag, ":")
	if lastColon <= lastSlash {
		return repoTag, "latest"
	}
	name := repoTag[:lastColon]
	tag := repoTag[lastColon+1:]
	if tag == "" {
		tag = "latest"
	}
	return name, tag
}

type dockerDiskUsageClient interface {
	DiskUsage(context.Context, types.DiskUsageOptions) (types.DiskUsage, error)
}

func dockerBuildCacheDiskUsage(ctx context.Context, cli dockerDiskUsageClient) (types.DiskUsage, error) {
	usage, filteredErr := cli.DiskUsage(ctx, types.DiskUsageOptions{
		Types: []types.DiskUsageObject{types.BuildCacheObject},
	})
	if filteredErr == nil {
		return usage, nil
	}
	if ctx.Err() != nil {
		return types.DiskUsage{}, filteredErr
	}
	if !dockerDiskUsageCompatibilityError(filteredErr) {
		return types.DiskUsage{}, filteredErr
	}

	logging.Debug("filtered Docker build cache query unsupported; retrying compatible query", "error", filteredErr)
	usage, fallbackErr := cli.DiskUsage(ctx, types.DiskUsageOptions{})
	if fallbackErr != nil {
		return types.DiskUsage{}, fmt.Errorf(
			"filtered Docker DiskUsage failed: %w; fallback Docker DiskUsage failed: %v",
			filteredErr,
			fallbackErr,
		)
	}
	return usage, nil
}

func dockerDiskUsageCompatibilityError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, marker := range []string{
		"unexpected eof",
		"invalid filter",
		"unsupported filter",
		"unknown filter",
		"unknown query parameter",
		"invalid query parameter",
		"unsupported query parameter",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func getBuildCache(c *gin.Context) {
	usage, ok := readDockerDisplayUsage(c, "build")
	if !ok {
		return
	}

	buildCache := usage.BuildCache
	if buildCache == nil {
		buildCache = []*types.BuildCache{}
	}

	var totalSize int64
	activeCount := 0
	for _, bc := range buildCache {
		totalSize += bc.Size
		if bc.InUse {
			activeCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"build_cache":  buildCache,
		"total_size":   totalSize,
		"count":        len(buildCache),
		"active_count": activeCount,
	})
}

func pruneBuildCache(c *gin.Context) {
	dockerDiskDisplayCache.invalidate()
	defer dockerDiskDisplayCache.invalidate()
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	var req struct {
		All         bool  `json:"all"`
		KeepStorage int64 `json:"keep_storage"`
	}
	c.ShouldBindJSON(&req)

	opts := types.BuildCachePruneOptions{
		All:         req.All,
		KeepStorage: req.KeepStorage,
	}

	report, err := cli.BuildCachePrune(c.Request.Context(), opts)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "清理构建缓存失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"caches_deleted":  report.CachesDeleted,
		"space_reclaimed": report.SpaceReclaimed,
		"deleted_count":   len(report.CachesDeleted),
	})
}
