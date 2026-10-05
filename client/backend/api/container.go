// backend/api/container.go
package api // 必须声明包名

import (
	"context"
	"dockerpanel/backend/pkg/config"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/system"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/gin-gonic/gin"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

func errorCodeFromStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "INVALID_PARAM"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusTooManyRequests:
		return "RATE_LIMIT"
	default:
		if status >= 500 {
			return "INTERNAL"
		}
		return "INTERNAL"
	}
}

func respondError(c *gin.Context, status int, message string, err error) {
	respondErrorWithCode(c, status, errorCodeFromStatus(status), message, err)
}

func respondErrorWithCode(c *gin.Context, status int, code, message string, err error) {
	code = strings.TrimSpace(code)
	if code == "" {
		code = errorCodeFromStatus(status)
	}
	details := ""
	if err != nil && !config.IsProduction() {
		details = err.Error()
	}
	errorText := message
	if details != "" {
		errorText = fmt.Sprintf("%s: %s", message, details)
	}
	payload := gin.H{
		"code":    code,
		"message": message,
		"error":   errorText,
	}
	if details != "" {
		payload["details"] = details
	}
	c.JSON(status, payload)
}

func respondErrorWithDetail(c *gin.Context, status int, message string, detail string) {
	if strings.TrimSpace(detail) == "" {
		respondError(c, status, message, nil)
		return
	}
	respondError(c, status, message, errors.New(detail))
}

func getDockerClient(c *gin.Context) (*docker.Client, bool) {
	started := time.Now()
	cli, err := docker.NewDockerClientWithContext(c.Request.Context())
	recordDockerReadTiming(c, "pool", started)
	if err != nil {
		if errors.Is(err, docker.ErrPoolBusy) {
			respondErrorWithCode(c, http.StatusServiceUnavailable, "DOCKER_POOL_BUSY", "Docker 正忙，请稍后重试", err)
			return nil, false
		}
		respondError(c, http.StatusInternalServerError, "连接Docker失败", err)
		return nil, false
	}
	return cli, true
}

func getDockerClientSSE(c *gin.Context) (*docker.Client, bool) {
	cli, err := docker.NewStreamingClient()
	if err != nil {
		c.String(http.StatusInternalServerError, "data: %s\n\n", `{"error":"连接Docker失败"}`)
		return nil, false
	}
	return cli, true
}

// 路由注册需导出的函数
func RegisterContainerRoutes(r *gin.RouterGroup) {
	group := r.Group("/containers")
	{
		group.GET("", ListContainers)
		group.GET("/tasks/:taskId", getContainerUpdateTask)
		group.GET("/tasks/:taskId/events", containerUpdateTaskEvents)
		group.GET("/:id", GetContainer) // 添加获取单个容器详情的路由
		group.POST("/create", createContainer)
		group.POST("/:id/rename", renameContainer) // 添加重命名容器路由（通过创建新容器实现）
		group.POST("/:id/start", startContainer)
		group.POST("/:id/stop", stopContainer)
		group.POST("/:id/kill", killContainer)
		group.POST("/:id/restart", restartContainer)
		group.POST("/:id/pause", pauseContainer)
		group.POST("/:id/unpause", unpauseContainer)
		group.POST("/prune", pruneContainers) // 注册清理容器路由，注意要放在 :id 路由之前，避免冲突，或者使用不同的路径
		group.DELETE("/:id", removeContainer)
		// 注册容器终端与命令执行路由（WebSocket + CLI）
		group.GET("/:id/update/events", updateContainerEvents)
		group.POST("/:id/update/tasks", startContainerUpdateTask)
		group.GET("/:id/logs", getContainerLogs)
		group.GET("/:id/logs/events", getContainerLogsEvents)
		group.GET("/:id/terminal", containerTerminal)
		group.GET("/:id/stats", getContainerStats)
		group.GET("/:id/stats/stream", streamContainerStats)
	}
}

// 清理停止的容器
func pruneContainers(c *gin.Context) {
	selfID, selfName, _, _ := getSelfIdentity()
	if selfID != "" || selfName != "" {
		respondError(c, http.StatusForbidden, "容器化部署模式下，禁止执行全局清理操作", nil)
		return
	}

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	report, err := cli.ContainersPrune(c.Request.Context(), filters.Args{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "清理容器失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "清理完成",
		"deletedCount":   len(report.ContainersDeleted),
		"spaceReclaimed": report.SpaceReclaimed,
	})
}

// 容器列表
func ListContainers(c *gin.Context) {
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	level := system.CurrentLoadLevel()
	c.Header("X-Tradis-Load-Level", string(level))
	c.Header("X-Tradis-Suggested-Refresh-Seconds", strconv.Itoa(system.SuggestedRefreshSeconds(level)))

	ctx := c.Request.Context()

	listStarted := time.Now()
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true})
	recordDockerReadTiming(c, "list", listStarted)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取容器列表失败", err)
		return
	}

	updateTTL := 5 * time.Second
	switch level {
	case system.LoadLevelHigh:
		updateTTL = 10 * time.Second
	case system.LoadLevelCritical:
		updateTTL = 30 * time.Second
	}
	updateMap, unavailableMap, updateImageIDs := getCachedImageUpdateMaps(updateTTL)

	inspectConcurrency := 8
	inspectRunningOnly := false
	switch level {
	case system.LoadLevelHigh:
		inspectConcurrency = 4
		inspectRunningOnly = true
	case system.LoadLevelCritical:
		inspectConcurrency = 0
	}

	detailStarted := time.Now()
	containersWithDetails := buildContainerListItems(
		ctx,
		cli,
		containers,
		inspectConcurrency,
		inspectRunningOnly,
		updateMap,
		unavailableMap,
		updateImageIDs,
	)
	recordDockerReadTiming(c, "inspect", detailStarted)

	c.JSON(http.StatusOK, containersWithDetails)
}

type containerListInspector interface {
	ContainerInspect(context.Context, string) (types.ContainerJSON, error)
}

func buildContainerListItems(
	ctx context.Context,
	cli containerListInspector,
	containers []types.Container,
	inspectConcurrency int,
	inspectRunningOnly bool,
	updateMap map[string]bool,
	unavailableMap map[string]bool,
	updateImageIDs map[string]string,
) []gin.H {
	items := make([]gin.H, len(containers))
	if inspectConcurrency <= 0 {
		for index, container := range containers {
			items[index] = buildContainerListItem(container, types.ContainerJSON{}, false, updateMap, unavailableMap, updateImageIDs)
		}
		return items
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, inspectConcurrency)
	for index, container := range containers {
		if inspectRunningOnly && strings.ToLower(container.State) != "running" {
			items[index] = buildContainerListItem(container, types.ContainerJSON{}, false, updateMap, unavailableMap, updateImageIDs)
			continue
		}

		index, container := index, container
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			inspect, err := cli.ContainerInspect(ctx, container.ID)
			items[index] = buildContainerListItem(container, inspect, err == nil, updateMap, unavailableMap, updateImageIDs)
		}()
	}
	wg.Wait()
	return items
}

var imageUpdateMapCache struct {
	mu             sync.Mutex
	expires        time.Time
	updateMap      map[string]bool
	unavailableMap map[string]bool
	updateImageIDs map[string]string
}

var containerUpdateLeases sync.Map

func invalidateImageUpdateMapCache() {
	imageUpdateMapCache.mu.Lock()
	defer imageUpdateMapCache.mu.Unlock()
	imageUpdateMapCache.expires = time.Time{}
	imageUpdateMapCache.updateMap = nil
	imageUpdateMapCache.unavailableMap = nil
	imageUpdateMapCache.updateImageIDs = nil
}

func getCachedImageUpdateMaps(ttl time.Duration) (map[string]bool, map[string]bool, map[string]string) {
	imageUpdateMapCache.mu.Lock()
	defer imageUpdateMapCache.mu.Unlock()

	now := time.Now()
	if imageUpdateMapCache.updateMap != nil && now.Before(imageUpdateMapCache.expires) {
		return imageUpdateMapCache.updateMap, imageUpdateMapCache.unavailableMap, imageUpdateMapCache.updateImageIDs
	}

	updateMap := make(map[string]bool)
	unavailableMap := make(map[string]bool)
	updateImageIDs := make(map[string]string)

	// 首先获取所有远程不可用的镜像
	unavailableImages, _ := database.GetAllUnavailableImageRemoteDigests()
	for _, repoTag := range unavailableImages {
		if strings.TrimSpace(repoTag) == "" {
			continue
		}
		for _, key := range normalizeImageVariants(repoTag) {
			unavailableMap[key] = true
		}
	}

	// 处理有更新的镜像
	if allUpdates, _ := database.GetAllImageUpdates(); len(allUpdates) > 0 {
		for _, u := range allUpdates {
			if strings.TrimSpace(u.RepoTag) == "" {
				continue
			}
			// 如果已经在 unavailableMap 中，跳过
			if unavailableMap[u.RepoTag] {
				continue
			}
			if u.LocalDigest != u.RemoteDigest {
				for _, key := range normalizeImageVariants(u.RepoTag) {
					updateMap[key] = true
					updateImageIDs[key] = u.ImageID
				}
			}
		}
	}

	imageUpdateMapCache.updateMap = updateMap
	imageUpdateMapCache.unavailableMap = unavailableMap
	imageUpdateMapCache.updateImageIDs = updateImageIDs
	imageUpdateMapCache.expires = now.Add(ttl)
	return updateMap, unavailableMap, updateImageIDs
}

func buildContainerListItem(container types.Container, inspect types.ContainerJSON, inspectOk bool, updateMap map[string]bool, unavailableMap map[string]bool, updateImageIDs ...map[string]string) gin.H {
	formattedPorts := make([]gin.H, 0, len(container.Ports))
	for _, port := range container.Ports {
		portInfo := gin.H{
			"PrivatePort": port.PrivatePort,
			"Type":        port.Type,
		}

		if port.PublicPort != 0 {
			hostIP := "0.0.0.0"
			if port.IP != "" {
				hostIP = port.IP
			}
			portInfo["PublicPort"] = port.PublicPort
			portInfo["IP"] = hostIP
		}

		formattedPorts = append(formattedPorts, portInfo)
	}

	runningTime := "未运行"
	if strings.ToLower(container.State) == "running" {
		runningTime = "运行中"
		if inspectOk {
			if startTime, err := time.Parse(time.RFC3339, inspect.State.StartedAt); err == nil {
				runningTime = time.Since(startTime).Round(time.Second).String()
			}
		}
	}

	nameCandidate := ""
	if len(container.Names) > 0 {
		nameCandidate = strings.TrimPrefix(container.Names[0], "/")
	}

	labels := container.Labels
	if inspectOk && inspect.Config != nil && inspect.Config.Labels != nil {
		labels = inspect.Config.Labels
	}

	imageID := container.ImageID
	if inspectOk && strings.TrimSpace(inspect.Image) != "" {
		imageID = inspect.Image
	}

	var networkSettings any
	var hostConfig any
	if inspectOk {
		networkSettings = inspect.NetworkSettings
		hostConfig = inspect.HostConfig
	}
	healthStatus := containerHealthStatus(container.Status, inspect, inspectOk)
	var recordedImageIDs map[string]string
	if len(updateImageIDs) > 0 {
		recordedImageIDs = updateImageIDs[0]
	}

	return gin.H{
		"Id":              container.ID,
		"Names":           container.Names,
		"Image":           container.Image,
		"ImageID":         imageID,
		"State":           container.State,
		"Status":          container.Status,
		"HealthStatus":    healthStatus,
		"Created":         container.Created,
		"Ports":           formattedPorts,
		"NetworkSettings": networkSettings,
		"HostConfig":      hostConfig,
		"RunningTime":     runningTime,
		"Labels":          labels,
		"isSelf":          isSelfOrProtectedContainer(container.ID, nameCandidate, container.Image, labels),
		"UpdateAvailable": containerUpdateAvailable(updateMap, unavailableMap, recordedImageIDs, container.Image, imageID),
	}
}

func containerUpdateAvailable(updateMap map[string]bool, unavailableMap map[string]bool, updateImageIDs map[string]string, image, imageID string) bool {
	if !hasImageUpdate(updateMap, unavailableMap, image) {
		return false
	}
	for _, key := range normalizeImageVariants(image) {
		if unavailableMap != nil && unavailableMap[key] {
			continue
		}
		if !updateMap[key] {
			continue
		}
		recordedID := strings.TrimSpace(updateImageIDs[key])
		return recordedID == "" || recordedID == strings.TrimSpace(imageID)
	}
	return false
}

func containerHealthStatus(statusText string, inspect types.ContainerJSON, inspectOk bool) string {
	if inspectOk && inspect.State != nil && inspect.State.Health != nil {
		return strings.ToLower(strings.TrimSpace(inspect.State.Health.Status))
	}
	text := strings.ToLower(strings.TrimSpace(statusText))
	switch {
	case strings.Contains(text, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(text, "(healthy)"):
		return "healthy"
	case strings.Contains(text, "(health: starting)") || strings.Contains(text, "(health:starting)"):
		return "starting"
	default:
		return ""
	}
}

// 获取单个容器的资源使用情况
func getContainerStats(c *gin.Context) {
	id := c.Param("id")
	if forbidIfSelfContainer(c, id) {
		return
	}

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	// 获取容器统计信息（单次快照）
	resp, err := cli.ContainerStats(c.Request.Context(), id, false)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取容器统计信息失败", err)
		return
	}
	defer resp.Body.Close()

	var statsJSON types.StatsJSON
	if err := json.NewDecoder(resp.Body).Decode(&statsJSON); err != nil {
		respondError(c, http.StatusInternalServerError, "解析容器统计信息失败", err)
		return
	}

	// 计算CPU使用率
	cpuDelta := float64(statsJSON.CPUStats.CPUUsage.TotalUsage - statsJSON.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(statsJSON.CPUStats.SystemUsage - statsJSON.PreCPUStats.SystemUsage)
	cpuPercent := 0.0
	if systemDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(len(statsJSON.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}

	// 计算内存
	memoryUsage := float64(statsJSON.MemoryStats.Usage)
	memoryLimit := float64(statsJSON.MemoryStats.Limit)
	memoryPercent := 0.0
	if memoryLimit > 0 {
		memoryPercent = (memoryUsage / memoryLimit) * 100.0
	}

	// 聚合网络流量（累计字节）
	var rxBytes, txBytes uint64
	for _, nw := range statsJSON.Networks {
		rxBytes += nw.RxBytes
		txBytes += nw.TxBytes
	}

	c.JSON(http.StatusOK, gin.H{
		"cpu_percent":    cpuPercent,
		"memory_usage":   memoryUsage,
		"memory_limit":   memoryLimit,
		"memory_percent": memoryPercent,
		"net_rx_bytes":   rxBytes,
		"net_tx_bytes":   txBytes,
		"timestamp":      time.Now().Unix(),
	})
}

// 流式推送容器资源使用情况 (SSE)
func streamContainerStats(c *gin.Context) {
	id := c.Param("id")
	if forbidIfSelfContainer(c, id) {
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	lastEventIDRaw := strings.TrimSpace(c.GetHeader("Last-Event-ID"))
	nextID := int64(1)
	if lastEventIDRaw != "" {
		if v, err := strconv.ParseInt(lastEventIDRaw, 10, 64); err == nil && v >= 0 {
			nextID = v + 1
		}
	}

	cli, ok := getDockerClientSSE(c)
	if !ok {
		return
	}
	defer cli.Close()

	resp, err := cli.ContainerStats(c.Request.Context(), id, true)
	if err != nil {
		c.String(http.StatusInternalServerError, "data: %s\n\n", `{"error":"获取容器统计信息失败"}`)
		return
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	var prevTime time.Time
	var prevRx, prevTx uint64
	first := true

	for {
		select {
		case <-c.Request.Context().Done():
			return
		default:
		}

		var statsJSON types.StatsJSON
		if err := dec.Decode(&statsJSON); err != nil {
			if err == io.EOF {
				return
			}
			// 解析失败，结束流
			return
		}

		cpuDelta := float64(statsJSON.CPUStats.CPUUsage.TotalUsage - statsJSON.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(statsJSON.CPUStats.SystemUsage - statsJSON.PreCPUStats.SystemUsage)
		cpuPercent := 0.0
		if systemDelta > 0 && cpuDelta > 0 {
			cpuPercent = (cpuDelta / systemDelta) * float64(len(statsJSON.CPUStats.CPUUsage.PercpuUsage)) * 100.0
		}

		memoryUsage := float64(statsJSON.MemoryStats.Usage)
		memoryLimit := float64(statsJSON.MemoryStats.Limit)
		memoryPercent := 0.0
		if memoryLimit > 0 {
			memoryPercent = (memoryUsage / memoryLimit) * 100.0
		}

		var rxBytes, txBytes uint64
		for _, nw := range statsJSON.Networks {
			rxBytes += nw.RxBytes
			txBytes += nw.TxBytes
		}

		// 读取时间
		now := time.Now()
		if !statsJSON.Read.IsZero() {
			now = statsJSON.Read
		}

		var upRate, downRate float64
		if !first && !prevTime.IsZero() {
			dt := now.Sub(prevTime).Seconds()
			if dt < 1 {
				dt = 1
			}
			upRate = float64(txBytes-prevTx) / dt
			downRate = float64(rxBytes-prevRx) / dt
		} else {
			upRate = 0
			downRate = 0
			first = false
		}

		prevRx = rxBytes
		prevTx = txBytes
		prevTime = now

		payload := map[string]interface{}{
			"cpu_percent":    cpuPercent,
			"memory_usage":   memoryUsage,
			"memory_limit":   memoryLimit,
			"memory_percent": memoryPercent,
			"net_rx_bytes":   rxBytes,
			"net_tx_bytes":   txBytes,
			"up_rate":        upRate,
			"down_rate":      downRate,
			"timestamp":      now.Unix(),
		}

		b, _ := json.Marshal(payload)
		_, _ = c.Writer.Write([]byte(fmt.Sprintf("id: %d\ndata: %s\n\n", nextID, string(b))))
		nextID++
		if f, ok := c.Writer.(http.Flusher); ok {
			f.Flush()
		} else {
			c.Writer.Flush()
		}
	}
}

// 获取单个容器详情
func GetContainer(c *gin.Context) {
	id := c.Param("id")

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	// 获取容器基础信息
	inspect, err := cli.ContainerInspect(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusNotFound, "容器不存在", err)
		return
	}

	// 获取容器统计信息（用于 CPU 和内存使用率）
	// 暂时只返回基础信息，实时监控数据建议通过 WebSocket 推送
	// stats, err := cli.ContainerStats(context.Background(), id, false)
	// var cpuPercent, memPercent, memUsage, memLimit float64
	// var netRx, netTx float64

	// if err == nil {
	// 	var statsJSON types.StatsJSON
	// 	// 这里简化处理，实际需要解析流式数据或只读取一次
	// 	// 由于 ContainerStats 返回的是流，直接读取可能会阻塞或需要复杂处理
	// 	// 暂时只返回基础信息，实时监控数据建议通过 WebSocket 推送
	// 	stats.Body.Close()
	// }

	// 处理端口映射
	formattedPorts := make([]gin.H, 0)
	// 使用 inspect 中的 NetworkSettings.Ports
	for port, bindings := range inspect.NetworkSettings.Ports {
		for _, binding := range bindings {
			portInfo := gin.H{
				"PrivatePort": port.Port(),
				"Type":        port.Proto(),
				"PublicPort":  binding.HostPort,
				"IP":          binding.HostIP,
			}
			formattedPorts = append(formattedPorts, portInfo)
		}
	}

	// 计算运行时间
	var runningTime string
	if inspect.State.Running {
		startTime, parseErr := time.Parse(time.RFC3339Nano, inspect.State.StartedAt)
		if parseErr != nil {
			runningTime = "时间解析错误"
		} else {
			runningTime = time.Since(startTime).Round(time.Second).String()
		}
	} else {
		runningTime = "未运行"
	}

	// 处理挂载卷
	formattedMounts := make([]gin.H, 0)
	for _, mount := range inspect.Mounts {
		mountInfo := gin.H{
			"Source":      mount.Source,
			"Destination": mount.Destination,
			"Type":        mount.Type,
			"Mode":        mount.Mode,
			"RW":          mount.RW,
		}
		formattedMounts = append(formattedMounts, mountInfo)
	}

	// 处理网络信息
	formattedNetworks := make([]string, 0)
	for netName := range inspect.NetworkSettings.Networks {
		formattedNetworks = append(formattedNetworks, netName)
	}

	// 获取镜像详细信息，以提取默认 Cmd 和 Entrypoint
	var imageConfig *container.Config
	imageInspect, _, err := cli.ImageInspectWithRaw(c.Request.Context(), inspect.Image)
	if err == nil {
		imageConfig = imageInspect.Config
	}

	var env []string
	var labels map[string]string
	var configImage string
	if inspect.Config != nil {
		env = inspect.Config.Env
		labels = inspect.Config.Labels
		configImage = inspect.Config.Image
	}

	containerInfo := gin.H{
		"Id":              inspect.ID,
		"Name":            inspect.Name, // 注意：inspect.Name 通常包含前导斜杠
		"Image":           configImage,
		"State":           inspect.State.Status,
		"Status":          inspect.State.Status, // 兼容前端
		"Created":         inspect.Created,
		"Ports":           formattedPorts,
		"Mounts":          formattedMounts,
		"Networks":        formattedNetworks,
		"RestartPolicy":   inspect.HostConfig.RestartPolicy.Name,
		"NetworkSettings": inspect.NetworkSettings,
		"HostConfig":      inspect.HostConfig,
		"RunningTime":     runningTime,
		"Path":            inspect.Path,
		"Args":            inspect.Args,
		"Config":          inspect.Config,
		"ImageConfig":     imageConfig,
		"Env":             env,
		"Labels":          labels,
		"isSelf":          isSelfOrProtectedContainer(inspect.ID, strings.TrimPrefix(inspect.Name, "/"), configImage, labels),
	}

	c.JSON(http.StatusOK, containerInfo)
}

// 重启容器
func restartContainer(c *gin.Context) {
	id := c.Param("id")
	if forbidIfSelfContainer(c, id) {
		return
	}
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	if err := cli.ContainerRestart(c.Request.Context(), id, container.StopOptions{}); err != nil {
		respondError(c, http.StatusInternalServerError, "重启容器失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "容器已重启"})
}

// 暂停容器
func pauseContainer(c *gin.Context) {
	id := c.Param("id")
	if forbidIfSelfContainer(c, id) {
		return
	}
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	if err := cli.ContainerPause(c.Request.Context(), id); err != nil {
		respondError(c, http.StatusInternalServerError, "暂停容器失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "容器已暂停"})
}

// 恢复容器
func unpauseContainer(c *gin.Context) {
	id := c.Param("id")
	if forbidIfSelfContainer(c, id) {
		return
	}
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	if err := cli.ContainerUnpause(c.Request.Context(), id); err != nil {
		respondError(c, http.StatusInternalServerError, "恢复容器失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "容器已恢复"})
}

// 启动容器
func startContainer(c *gin.Context) {
	id := c.Param("id")
	if forbidIfSelfContainer(c, id) {
		return
	}
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	// 先检查容器是否存在
	inspect, err := cli.ContainerInspect(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusNotFound, "容器不存在", err)
		return
	}

	// 修正：前面 inspect 已经获取了详细信息，直接用 inspect 判断状态更准确
	if inspect.State.Running {
		respondError(c, http.StatusBadRequest, "容器已经在运行中", nil)
		return
	}

	// 尝试启动容器
	err = cli.ContainerStart(c.Request.Context(), id, types.ContainerStartOptions{})
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "bind: address already in use"):
			// 提取端口信息，匹配格式为 0.0.0.0:端口号 的模式
			portRegex := regexp.MustCompile(`0.0.0.0:(\d+)`)
			matches := portRegex.FindStringSubmatch(errMsg)
			if len(matches) > 1 {
				respondError(c, http.StatusInternalServerError, fmt.Sprintf("端口冲突，%s，请检查端口", matches[1]), nil)
			} else {
				respondError(c, http.StatusInternalServerError, "端口冲突，请检查端口配置", nil)
			}
		case strings.Contains(errMsg, "no such file or directory"):
			// 提取路径信息
			pathRegex := regexp.MustCompile(`path\s+([^\s]+)\s+`)
			matches := pathRegex.FindStringSubmatch(errMsg)
			if len(matches) > 1 {
				respondError(c, http.StatusInternalServerError, fmt.Sprintf("路径不存在，请检查宿主机路径%s", matches[1]), nil)
			} else {
				respondError(c, http.StatusInternalServerError, "路径不存在，请检查宿主机路径配置", nil)
			}
		default:
			respondErrorWithDetail(c, http.StatusInternalServerError, "启动容器失败", errMsg)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "容器已启动"})
}

// 停止容器
func stopContainer(c *gin.Context) {
	id := c.Param("id")
	if forbidIfSelfContainer(c, id) {
		return
	}
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	// 先检查容器是否存在
	_, err := cli.ContainerInspect(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusNotFound, "容器不存在", nil)
		return
	}

	// 尝试停止容器
	timeout := 2 // 设置超时时间为 2 秒，加快响应速度
	err = cli.ContainerStop(c.Request.Context(), id, container.StopOptions{
		Timeout: &timeout,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "停止容器失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "容器已停止"})
}

// errContainerNotFound 表示目标容器不存在，用于映射 404。
var errContainerNotFound = errors.New("container not found")

// containerForceStopClient 是强制停止所需的最小 Docker 客户端能力，
// 真实 *docker.Client 与测试 Mock 均满足该接口。
type containerForceStopClient interface {
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
}

// killContainerByID 先确认容器存在，再以 0 宽限期停止（立即 SIGKILL）。
// 必须用 ContainerStop 而不是 ContainerKill：只有 stop 会写入手动停止标记，
// restart=always/unless-stopped 的容器才不会被守护进程当作崩溃再次拉起。
func killContainerByID(ctx context.Context, cli containerForceStopClient, id string) error {
	if _, err := cli.ContainerInspect(ctx, id); err != nil {
		if errdefs.IsNotFound(err) {
			return errContainerNotFound
		}
		return fmt.Errorf("inspect container before force stop: %w", err)
	}
	timeout := 0
	return cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
}

// 强制停止容器（0 宽限期立即 SIGKILL，并按手动停止处理重启策略）
func killContainer(c *gin.Context) {
	id := c.Param("id")
	if forbidIfSelfContainer(c, id) {
		return
	}
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	if err := killContainerByID(c.Request.Context(), cli, id); err != nil {
		if errors.Is(err, errContainerNotFound) {
			respondError(c, http.StatusNotFound, "容器不存在", nil)
			return
		}
		respondError(c, http.StatusInternalServerError, "强制停止容器失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "容器已强制停止"})
}

// 删除容器
func removeContainer(c *gin.Context) {
	id := c.Param("id")
	if forbidIfSelfContainer(c, id) {
		return
	}
	force := strings.EqualFold(c.Query("force"), "true") || c.Query("force") == "1"
	removeVolumes := strings.EqualFold(c.Query("v"), "true") || c.Query("v") == "1"

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	var hostPorts []int
	inspect, ierr := cli.ContainerInspect(c.Request.Context(), id)
	if ierr == nil {
		dedup := make(map[int]struct{})
		for _, bindings := range inspect.NetworkSettings.Ports {
			for _, b := range bindings {
				if b.HostPort != "" {
					if p, perr := strconv.Atoi(b.HostPort); perr == nil && p > 0 {
						if _, ok := dedup[p]; !ok {
							hostPorts = append(hostPorts, p)
							dedup[p] = struct{}{}
						}
					}
				}
			}
		}
	}
	if !force {
		shouldStop := true
		if ierr == nil {
			shouldStop = inspect.State != nil && inspect.State.Running
		}
		if shouldStop {
			timeout := 2 // 设置超时时间为 2 秒
			err := cli.ContainerStop(c.Request.Context(), id, container.StopOptions{
				Timeout: &timeout,
			})
			if err != nil {
				respondError(c, http.StatusInternalServerError, "停止容器失败", err)
				return
			}
		}
	}

	err := cli.ContainerRemove(c.Request.Context(), id, types.ContainerRemoveOptions{
		Force:         force,
		RemoveVolumes: removeVolumes,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "删除容器失败", err)
		return
	}

	if len(hostPorts) > 0 {
		if tx, txErr := database.GetDB().Begin(); txErr == nil {
			if derr := database.DeleteReservedPortsByPortsTx(tx, hostPorts); derr != nil {
				_ = tx.Rollback()
			} else {
				_ = tx.Commit()
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "容器已删除"})
}

// CreateContainerRequest 定义创建容器的请求结构
type CreateContainerRequest struct {
	Name          string   `json:"name"`
	Image         string   `json:"image"`
	Ports         []string `json:"ports"`          // 格式: "8080:80"
	Env           []string `json:"env"`            // 格式: "KEY=VALUE"
	Volumes       []string `json:"volumes"`        // 格式: "/host/path:/container/path"
	NetworkMode   string   `json:"network_mode"`   // 网络模式
	RestartPolicy string   `json:"restart_policy"` // 重启策略
	Command       []string `json:"command"`        // 启动命令
	Entrypoint    []string `json:"entrypoint"`     // 入口点
	Privileged    bool     `json:"privileged"`     // 特权模式
	Devices       []string `json:"devices"`        // 格式: "/dev/sda:/dev/xda:rwm"
	User          string   `json:"user"`           // 运行用户 uid:gid
	PUID          int      `json:"puid"`           // 进程用户ID (PUID)
	PGID          int      `json:"pgid"`           // 进程组ID (PGID)
}

// createContainer 创建容器
func createContainer(c *gin.Context) {
	var req CreateContainerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	// 1. 拉取镜像（如果不存在）
	// 这里简化处理，假设镜像已存在或自动拉取
	// 实际生产中可能需要显式拉取镜像

	// 2. 配置容器
	// 如果前端传来的 Command 为空，不要强制设为空切片，让 Docker 使用镜像默认的 CMD
	// 但是这里 req.Command 即使为空切片，赋给 Cmd 后，Docker API 可能会认为是要清空 CMD
	// 所以如果为空，最好设为 nil
	var cmd []string
	if len(req.Command) > 0 {
		cmd = req.Command
	}

	var entrypoint []string
	if len(req.Entrypoint) > 0 {
		entrypoint = req.Entrypoint
	}

	config := &container.Config{
		Image:      req.Image,
		Env:        req.Env,
		Cmd:        cmd,
		Entrypoint: entrypoint,
		User:       req.User, // 设置运行用户
	}

	// 解析设备映射
	var devices []container.DeviceMapping
	for _, d := range req.Devices {
		parts := strings.Split(d, ":")
		if len(parts) >= 2 {
			dev := container.DeviceMapping{
				PathOnHost:        parts[0],
				PathInContainer:   parts[1],
				CgroupPermissions: "rwm",
			}
			if len(parts) > 2 {
				dev.CgroupPermissions = parts[2]
			}
			devices = append(devices, dev)
		}
	}

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: req.RestartPolicy,
		},
		NetworkMode:  container.NetworkMode(req.NetworkMode),
		Binds:        req.Volumes,
		PortBindings: make(map[nat.Port][]nat.PortBinding),
		Privileged:   req.Privileged,
		Resources: container.Resources{
			Devices: devices,
		},
	}

	// 处理端口映射
	for _, p := range req.Ports {
		// 格式: "HostPort:ContainerPort" 或 "HostPort:ContainerPort/Protocol"
		parts := strings.Split(p, ":")
		if len(parts) >= 2 {
			hostPort := parts[0]
			containerPortRaw := parts[1]

			// 解析协议
			protocol := "tcp"
			containerPort := containerPortRaw
			if strings.Contains(containerPortRaw, "/") {
				cpParts := strings.Split(containerPortRaw, "/")
				containerPort = cpParts[0]
				protocol = cpParts[1]
			}

			portKey, portErr := nat.NewPort(protocol, containerPort)
			if portErr == nil {
				hostConfig.PortBindings[portKey] = []nat.PortBinding{
					{
						HostPort: hostPort,
					},
				}
			}
		}
	}

	// 3. 创建容器
	resp, err := cli.ContainerCreate(c.Request.Context(), config, hostConfig, nil, nil, req.Name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "创建容器失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": resp.ID, "message": "容器创建成功"})
}

// RenameContainerRequest 定义重命名容器的请求结构
type RenameContainerRequest struct {
	NewName string `json:"newName" binding:"required"`
}

// renameContainer 重命名容器（实际上是创建新容器并替换）
// 注意：Docker API 的 rename 只是改名，不会改变配置。
// 如果用户想改名，通常期望的是用新名字运行完全一样的服务。
// 这里我们先只实现简单的 rename，如果需要完整的“克隆+改名”，逻辑会更复杂。
// 根据用户需求：“容器重命名→创建新容器→验证新容器状态→删除旧容器”
// 这个逻辑主要在前端控制，后端提供 Create 接口即可。
// 但为了方便，我们可以提供一个 rename 接口，或者让前端分步调用。
// 这里我们提供一个 rename 接口，直接调用 Docker 的 rename API
func renameContainer(c *gin.Context) {
	id := c.Param("id")
	if forbidIfSelfContainer(c, id) {
		return
	}
	var req RenameContainerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", nil)
		return
	}

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	if err := cli.ContainerRename(c.Request.Context(), id, req.NewName); err != nil {
		respondError(c, http.StatusInternalServerError, "重命名失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "容器重命名成功"})
}

// updateContainerEvents 更新容器（拉取镜像并重建）并推送 SSE 事件
func shouldPullBeforeContainerRecreate(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func shouldStartRecreatedContainer(state *types.ContainerState) bool {
	return state != nil && state.Running
}

func bindMountTarget(raw string) string {
	separator := strings.Index(raw, ":")
	if separator < 0 || separator == len(raw)-1 {
		return ""
	}
	targetAndMode := raw[separator+1:]
	if modeSeparator := strings.Index(targetAndMode, ":"); modeSeparator >= 0 {
		targetAndMode = targetAndMode[:modeSeparator]
	}
	return strings.TrimSpace(targetAndMode)
}

func recreateSourceContainerID(requested string, inspected types.ContainerJSON) string {
	if inspected.ContainerJSONBase != nil && inspected.ID != "" {
		return inspected.ID
	}
	return requested
}

func preserveRecreatedContainerVolumes(hostConfig *container.HostConfig, existing []types.MountPoint) {
	if hostConfig == nil {
		return
	}

	volumeByTarget := make(map[string]types.MountPoint)
	for _, current := range existing {
		if current.Type == mount.TypeVolume && current.Name != "" && current.Destination != "" {
			volumeByTarget[current.Destination] = current
		}
	}

	coveredTargets := make(map[string]struct{})
	for index := range hostConfig.Mounts {
		configured := &hostConfig.Mounts[index]
		coveredTargets[configured.Target] = struct{}{}
		if configured.Type == mount.TypeVolume && configured.Source == "" {
			if current, ok := volumeByTarget[configured.Target]; ok {
				configured.Source = current.Name
			}
		}
	}

	for _, rawBind := range hostConfig.Binds {
		if target := bindMountTarget(rawBind); target != "" {
			coveredTargets[target] = struct{}{}
		}
	}

	for _, current := range existing {
		if current.Type != mount.TypeVolume || current.Name == "" || current.Destination == "" {
			continue
		}
		if _, covered := coveredTargets[current.Destination]; covered {
			continue
		}
		hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
			Type:     mount.TypeVolume,
			Source:   current.Name,
			Target:   current.Destination,
			ReadOnly: !current.RW,
		})
		coveredTargets[current.Destination] = struct{}{}
	}
}

func updateContainerEvents(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	if forbidIfSelfContainer(c, id) {
		_, _ = c.Writer.Write([]byte("event: log\ndata: error: 禁止操作自身容器\n\n"))
		c.Writer.Flush()
		return
	}
	release, acquired := acquireContainerUpdateLease(id)
	if !acquired {
		_, _ = c.Writer.Write([]byte("event: log\ndata: error: 该容器已有更新任务正在运行\n\n"))
		c.Writer.Flush()
		return
	}
	defer release()

	cli, ok := getDockerClientSSE(c)
	if !ok {
		return
	}
	defer cli.Close()

	send := func(msg string) {
		_, _ = c.Writer.Write([]byte(fmt.Sprintf("event: log\ndata: %s\n\n", msg)))
		c.Writer.Flush()
	}

	_ = runContainerRecreate(c.Request.Context(), cli, id, shouldPullBeforeContainerRecreate(c.Query("pull")), send)
}

func startContainerUpdateTask(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		respondError(c, http.StatusBadRequest, "容器 ID 不能为空", nil)
		return
	}
	if forbidIfSelfContainer(c, id) {
		return
	}
	release, acquired := acquireContainerUpdateLease(id)
	if !acquired {
		respondError(c, http.StatusConflict, "该容器已有更新任务正在运行", nil)
		return
	}

	taskID := fmt.Sprintf("container-update-%d", time.Now().UnixNano())
	if err := database.UpsertTask(taskID, "container_update", "pending"); err != nil {
		release()
		respondError(c, http.StatusInternalServerError, "创建容器更新任务失败", err)
		return
	}

	pull := shouldPullBeforeContainerRecreate(c.Query("pull"))
	go func() {
		defer release()
		runContainerUpdateTask(taskID, id, pull)
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message": "容器更新任务已提交",
		"taskId":  taskID,
	})
}

// clearImageUpdateRecordsAfterContainerRecreate 重建式更新（带 pull）成功后，
// 容器镜像 ref 已追平远端，清掉挂在该 ref 上的更新记录；无 pull 的纯重建不改动
// 镜像，既有记录（说明镜像确有更新）仍然成立，保持不动。
func clearImageUpdateRecordsAfterContainerRecreate(pull bool, imageRef string) {
	if !pull {
		return
	}
	imageRef = strings.TrimSpace(imageRef)
	if imageRef == "" {
		return
	}
	clearImageUpdateRecordsByImageRefs(imageRef)
}

func runContainerUpdateTask(taskID string, id string, pull bool) {
	seq := int64(0)
	appendLog := func(raw string) {
		seq++
		logType, message := splitContainerOperationLog(raw)
		_ = database.AppendTaskLogWithSeq(taskID, seq, time.Now(), logType, message)
	}

	_ = database.UpsertTask(taskID, "container_update", "running")
	containerName := ""
	containerImage := ""
	cli, err := docker.NewDockerClient()
	if err == nil {
		defer cli.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if details, inspectErr := cli.ContainerInspect(ctx, id); inspectErr == nil {
			containerName = strings.TrimPrefix(strings.TrimSpace(details.Name), "/")
			if details.Config != nil {
				containerImage = strings.TrimSpace(details.Config.Image)
			}
		}
		err = runContainerRecreate(ctx, cli, id, pull, appendLog)
	}
	if err != nil {
		if seq == 0 {
			appendLog("error: " + err.Error())
		}
		_ = database.FinishTask(taskID, "error", nil, err.Error())
		_ = database.SaveNotification(&database.Notification{
			Type:      "error",
			EventType: "container_update",
			Category:  "deploy_task",
			Message:   containerUpdateNotificationMessage(containerName, false),
			Read:      false,
		})
		return
	}

	// 带 pull 的重建式更新成功后，容器镜像 ref 已追平远端，清掉挂在该 ref 上的
	// 更新记录，避免 Images 页角标错一个周期
	clearImageUpdateRecordsAfterContainerRecreate(pull, containerImage)
	invalidateComposeProjectListCache()
	invalidateImageUpdateMapCache()
	_ = database.FinishTask(taskID, "success", gin.H{"containerId": id}, "")
	_ = database.SaveNotification(&database.Notification{
		Type:      "success",
		EventType: "container_update",
		Category:  "deploy_task",
		Message:   containerUpdateNotificationMessage(containerName, true),
		Read:      false,
	})
}

func containerUpdateNotificationMessage(name string, success bool) string {
	name = strings.TrimSpace(name)
	result := "失败"
	if success {
		result = "完成"
	}
	if name == "" {
		return "容器更新" + result
	}
	return fmt.Sprintf("容器 %s 更新%s", name, result)
}

func getContainerUpdateTask(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("taskId"))
	task, err := database.GetTask(taskID)
	if err != nil || task.Type != "container_update" {
		respondError(c, http.StatusNotFound, "容器更新任务不存在", err)
		return
	}
	c.JSON(http.StatusOK, task)
}

func containerUpdateTaskEvents(c *gin.Context) {
	streamDatabaseTaskEvents(c, strings.TrimSpace(c.Param("taskId")))
}

func splitContainerOperationLog(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	for _, prefix := range []string{"error", "warn", "warning", "success", "info"} {
		marker := prefix + ":"
		if strings.HasPrefix(strings.ToLower(raw), marker) {
			logType := prefix
			if logType == "warn" {
				logType = "warning"
			}
			return logType, strings.TrimSpace(raw[len(marker):])
		}
	}
	return "info", raw
}

type containerRecreateClient interface {
	ContainerList(context.Context, types.ContainerListOptions) ([]types.Container, error)
	ContainerInspect(context.Context, string) (types.ContainerJSON, error)
	ImagePull(context.Context, string, types.ImagePullOptions) (io.ReadCloser, error)
	ContainerRename(context.Context, string, string) error
	ContainerStop(context.Context, string, container.StopOptions) error
	ContainerCreate(context.Context, *container.Config, *container.HostConfig, *network.NetworkingConfig, *specs.Platform, string) (container.CreateResponse, error)
	ContainerStart(context.Context, string, types.ContainerStartOptions) error
	ContainerRemove(context.Context, string, types.ContainerRemoveOptions) error
}

func runContainerRecreate(ctx context.Context, cli containerRecreateClient, id string, pull bool, send func(string)) error {
	if send == nil {
		send = func(string) {}
	}
	send("info: 开始检查容器配置...")

	// 1. Inspect old container
	oldContainer, err := cli.ContainerInspect(ctx, id)
	if err != nil {
		send(fmt.Sprintf("error: 获取容器信息失败: %v", err))
		return err
	}
	if oldContainer.Config == nil || oldContainer.HostConfig == nil {
		err = errors.New("容器配置不完整，无法安全重建")
		send("error: " + err.Error())
		return err
	}

	imageName := oldContainer.Config.Image
	containerName := strings.TrimPrefix(oldContainer.Name, "/")
	sourceID := recreateSourceContainerID(id, oldContainer)
	wasRunning := shouldStartRecreatedContainer(oldContainer.State)

	// 2. Pull image when explicitly requested. Resetting after a separate image
	// pull uses the local tag directly and avoids downloading the same image twice.
	if pull {
		send(fmt.Sprintf("info: 正在拉取最新镜像 %s...", imageName))
		out, pullErr := cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
		if pullErr != nil {
			send(fmt.Sprintf("error: 拉取镜像失败: %v", pullErr))
			return pullErr
		}
		if pullErr := consumeDockerPullResponse(out); pullErr != nil {
			send(fmt.Sprintf("error: 拉取镜像失败: %v", pullErr))
			return pullErr
		}
		send("info: 镜像拉取完成")
	} else {
		send(fmt.Sprintf("info: 使用本地镜像 %s 重建容器...", imageName))
	}

	// 3. Rename old container
	backupName := fmt.Sprintf("%s_backup_%d", containerName, time.Now().Unix())
	send(fmt.Sprintf("info: 重命名旧容器为 %s...", backupName))

	if err := cli.ContainerRename(ctx, sourceID, backupName); err != nil {
		send(fmt.Sprintf("error: 重命名容器失败: %v", err))
		return err
	}

	// 4. Stop old container only when it was running. A stopped container should
	// remain stopped after recreation instead of failing on a redundant stop.
	if wasRunning {
		send("info: 停止旧容器...")
		timeout := 10
		if err := cli.ContainerStop(ctx, sourceID, container.StopOptions{Timeout: &timeout}); err != nil {
			send(fmt.Sprintf("error: 停止容器失败: %v. 正在回滚...", err))
			_ = cli.ContainerRename(ctx, sourceID, containerName)
			return err
		}
	} else {
		send("info: 容器当前已停止，将保留停止状态")
	}

	// 5. Create new container
	send("info: 创建新容器...")

	endpointsConfig := make(map[string]*network.EndpointSettings)
	if oldContainer.NetworkSettings != nil {
		for k, v := range oldContainer.NetworkSettings.Networks {
			if v == nil {
				continue
			}
			endpointsConfig[k] = &network.EndpointSettings{
				IPAMConfig: v.IPAMConfig,
				Links:      v.Links,
				Aliases:    v.Aliases,
				NetworkID:  v.NetworkID,
			}
		}
	}
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: endpointsConfig,
	}
	preserveRecreatedContainerVolumes(oldContainer.HostConfig, oldContainer.Mounts)

	createdBody, err := cli.ContainerCreate(ctx, oldContainer.Config, oldContainer.HostConfig, networkingConfig, nil, containerName)
	if err != nil {
		send(fmt.Sprintf("error: 创建新容器失败: %v. 正在回滚...", err))
		_ = cli.ContainerRename(ctx, sourceID, containerName)
		if wasRunning {
			_ = cli.ContainerStart(ctx, sourceID, types.ContainerStartOptions{})
		}
		return err
	}

	// 6. Start new container
	if wasRunning {
		send("info: 启动新容器...")
		if err := cli.ContainerStart(ctx, createdBody.ID, types.ContainerStartOptions{}); err != nil {
			send(fmt.Sprintf("error: 启动新容器失败: %v. 正在回滚...", err))
			_ = cli.ContainerRemove(ctx, createdBody.ID, types.ContainerRemoveOptions{Force: true})
			_ = cli.ContainerRename(ctx, sourceID, containerName)
			_ = cli.ContainerStart(ctx, sourceID, types.ContainerStartOptions{})
			return err
		}
	} else {
		send("info: 新容器已创建并保持停止状态")
	}

	// 7. Remove old container
	send("info: 移除旧容器...")
	if err := cli.ContainerRemove(ctx, sourceID, types.ContainerRemoveOptions{Force: true}); err != nil {
		send(fmt.Sprintf("warn: 移除旧容器失败: %v (新容器已正常运行)", err))
	}

	clearContainerImageUpdateIfUnused(ctx, cli, imageName, oldContainer.Image)
	send("success: 容器更新完成")
	return nil
}

func acquireContainerUpdateLease(containerID string) (func(), bool) {
	key := strings.TrimSpace(containerID)
	if key == "" {
		return func() {}, false
	}
	token := &struct{}{}
	if _, loaded := containerUpdateLeases.LoadOrStore(key, token); loaded {
		return func() {}, false
	}
	return func() {
		containerUpdateLeases.CompareAndDelete(key, token)
	}, true
}

func consumeDockerPullResponse(reader io.ReadCloser) error {
	if reader == nil {
		return fmt.Errorf("镜像仓库未返回拉取结果")
	}
	defer reader.Close()
	decoder := json.NewDecoder(reader)
	for {
		var message struct {
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if err := decoder.Decode(&message); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if detail := strings.TrimSpace(message.ErrorDetail.Message); detail != "" {
			return errors.New(detail)
		}
		if messageText := strings.TrimSpace(message.Error); messageText != "" {
			return errors.New(messageText)
		}
	}
}

func clearContainerImageUpdateIfUnused(ctx context.Context, cli containerRecreateClient, imageName, oldImageID string) {
	oldImageID = strings.TrimSpace(oldImageID)
	if oldImageID == "" {
		return
	}
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return
	}
	for _, current := range containers {
		if strings.TrimSpace(current.ImageID) == oldImageID {
			return
		}
	}
	clearImageUpdateRecordsByImageRefs(imageName)
}
