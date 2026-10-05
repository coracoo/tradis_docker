package api

import (
	"context"
	"dockerpanel/backend/pkg/cloudflare"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/settings"
	"dockerpanel/backend/pkg/system"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

var systemStatsCache = struct {
	sync.Mutex
	data      gin.H
	updatedAt time.Time
}{
	data: gin.H{},
}

var systemStatsRefreshMu sync.Mutex

var hostCPUSnapshotState = struct {
	sync.RWMutex
	provider func() system.CPUSnapshot
}{
	provider: func() system.CPUSnapshot { return system.CPUSnapshot{} },
}

func SetHostCPUSnapshotProvider(provider func() system.CPUSnapshot) {
	hostCPUSnapshotState.Lock()
	defer hostCPUSnapshotState.Unlock()
	if provider == nil {
		hostCPUSnapshotState.provider = func() system.CPUSnapshot { return system.CPUSnapshot{} }
		return
	}
	hostCPUSnapshotState.provider = provider
}

func currentHostCPUSnapshot() system.CPUSnapshot {
	hostCPUSnapshotState.RLock()
	provider := hostCPUSnapshotState.provider
	hostCPUSnapshotState.RUnlock()
	return provider()
}

func cpuInfoResponseFields(snapshot system.CPUSnapshot) gin.H {
	sampledAt := int64(0)
	if !snapshot.SampledAt.IsZero() {
		sampledAt = snapshot.SampledAt.UnixMilli()
	}
	return gin.H{
		"CpuUsage":          snapshot.Percent,
		"CpuSampleWindowMs": snapshot.Window.Milliseconds(),
		"CpuSampledAt":      sampledAt,
		"CpuSampleReady":    snapshot.Ready,
		"CpuSampleStale":    snapshot.Stale,
	}
}

func cpuStatsResponseFields(snapshot system.CPUSnapshot) gin.H {
	sampledAt := int64(0)
	if !snapshot.SampledAt.IsZero() {
		sampledAt = snapshot.SampledAt.UnixMilli()
	}
	return gin.H{
		"cpu_percent":          snapshot.Percent,
		"cpu_sample_window_ms": snapshot.Window.Milliseconds(),
		"cpu_sampled_at":       sampledAt,
		"cpu_sample_ready":     snapshot.Ready,
		"cpu_sample_stale":     snapshot.Stale,
	}
}

func RegisterSystemRoutes(r *gin.RouterGroup) {
	group := r.Group("/system")
	{
		group.GET("/info", getSystemInfo)
		group.GET("/stats", getSystemStats)
		group.GET("/disk-usage", getDiskUsage)
		group.GET("/events", getSystemEvents)
		group.GET("/diagnostics/export", exportDiagnostics)
		group.GET("/cdn/status", getCDNStatus)
		group.POST("/cdn/retest", refreshCDNSpeedTest)
		group.POST("/notifications", addNotification)
		group.GET("/notifications", getNotifications)
		group.GET("/notifications/summary", getNotificationSummary)
		group.DELETE("/notifications/:id", deleteNotification)
		group.DELETE("/notifications", clearAllNotifications)
		group.POST("/notifications/read", markNotificationsRead)
		group.POST("/navigation/rebuild", rebuildNavigation)
		group.POST("/volume-backup/rebuild", rebuildVolumeBackup)
	}
}

func getCDNStatusSummary() gin.H {
	s, _ := settings.GetSettings()
	speedtestDisabled := !cloudflare.IsEnabled()
	status := cloudflare.GetStatus(strings.TrimSpace(s.AppStoreCDNURL) != "")
	message := "应用商店 CDN 未配置"
	typeClass := "warning"
	if speedtestDisabled {
		if status.Enabled {
			typeClass = "success"
			message = "应用商店 CDN 已配置"
		}
	} else if status.Enabled {
		typeClass = "info"
		if status.BestIP != "" {
			typeClass = "success"
			message = fmt.Sprintf("应用商店 CDN 已启用，当前优选 IP: %s", status.BestIP)
		} else if status.Running {
			typeClass = "info"
			message = "应用商店 CDN 已启用，正在执行本地优选测速"
		} else if status.LastError != "" {
			typeClass = "warning"
			message = fmt.Sprintf("应用商店 CDN 已启用，优选 IP 未就绪：%s", status.LastError)
		} else {
			message = "应用商店 CDN 已启用，当前使用域名直连或等待测速完成"
		}
	}

	return gin.H{
		"enabled":           status.Enabled,
		"running":           status.Running,
		"speedtestDisabled": speedtestDisabled,
		"appStoreCDNURL":    strings.TrimSpace(s.AppStoreCDNURL),
		"bestIp":            status.BestIP,
		"bestIps":           status.BestIPs,
		"lastTestTime":      status.LastTestTime,
		"testIntervalSec":   status.TestIntervalSec,
		"needsRetest":       status.NeedsRetest,
		"binPath":           status.BinPath,
		"binExists":         status.BinExists,
		"resultPath":        status.ResultPath,
		"resultExists":      status.ResultExists,
		"lastError":         status.LastError,
		"summary":           message,
		"typeClass":         typeClass,
	}
}

func getCDNStatus(c *gin.Context) {
	c.JSON(http.StatusOK, getCDNStatusSummary())
}

func refreshCDNSpeedTest(c *gin.Context) {
	if err := cloudflare.RefreshSpeedTestAsync("手动优选 IP 刷新"); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, cloudflare.ErrSpeedTestRunning) {
			status = http.StatusConflict
		}
		respondError(c, status, err.Error(), nil)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"accepted": true,
		"message":  "已废弃旧优选 IP，并开始后台重新测速",
	})
}

func rebuildNavigation(c *gin.Context) {
	containerID := strings.TrimSpace(c.Query("container_id"))
	projectName := strings.TrimSpace(c.Query("project"))

	if containerID != "" {
		system.RebuildAutoNavigationForContainer(containerID)
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
		return
	}
	if projectName != "" {
		system.RebuildAutoNavigationForComposeProject(projectName)
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
		return
	}

	system.RebuildAutoNavigationAll()
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func rebuildVolumeBackup(c *gin.Context) {
	s, err := settings.GetSettings()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to get settings", err)
		return
	}
	go system.RebuildVolumeBackupContainer(s)
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func parseDockerMajorVersion(raw string) int {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func parseAPIVersion(raw string) (major int, minor int, ok bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, 0, false
	}
	parts := strings.SplitN(s, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	maj, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || maj < 0 {
		return 0, 0, false
	}
	min, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || min < 0 {
		return 0, 0, false
	}
	return maj, min, true
}

func isAPIVersionGE(raw string, wantMajor int, wantMinor int) bool {
	maj, min, ok := parseAPIVersion(raw)
	if !ok {
		return false
	}
	if maj != wantMajor {
		return maj > wantMajor
	}
	return min >= wantMinor
}

func shouldApplyMinAPIVersionFix(engineVersion string, apiVersion string) bool {
	if parseDockerMajorVersion(engineVersion) >= 29 {
		return true
	}
	return isAPIVersionGE(apiVersion, 1, 52)
}

// 获取系统信息
func getSystemInfo(c *gin.Context) {
	level := system.CurrentLoadLevel()
	c.Header("X-Tradis-Load-Level", string(level))
	c.Header("X-Tradis-Suggested-Refresh-Seconds", strconv.Itoa(system.SuggestedRefreshSeconds(level)))

	// 创建Docker客户端
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	ctx := c.Request.Context()

	info, err := cli.Info(ctx)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取Docker信息失败", err)
		return
	}

	versionInfo, err := cli.ServerVersion(ctx)
	if err != nil {
		versionInfo = types.Version{}
	}

	engineVersion := strings.TrimSpace(versionInfo.Version)
	if engineVersion == "" {
		engineVersion = strings.TrimSpace(info.ServerVersion)
	}

	daemonMinAPIVersion := ""
	daemonCfg, daemonErr := docker.GetDaemonConfig()
	if daemonErr == nil && daemonCfg != nil {
		daemonMinAPIVersion = strings.TrimSpace(daemonCfg.MinAPIVersion)
	}

	minAPIVersionFixTarget := "1.43"
	minAPIVersionFixNeeded := shouldApplyMinAPIVersionFix(engineVersion, versionInfo.APIVersion)
	minAPIVersionFixApplied := false
	minAPIVersionFixError := ""
	if minAPIVersionFixNeeded && daemonMinAPIVersion == "" {
		if err := docker.UpdateDaemonConfig(&docker.DaemonConfig{MinAPIVersion: minAPIVersionFixTarget}); err != nil {
			minAPIVersionFixError = err.Error()
		} else {
			minAPIVersionFixApplied = true
			daemonMinAPIVersion = minAPIVersionFixTarget
		}
	}

	// 获取数据卷列表以统计数量
	volumeList, err := cli.VolumeList(ctx, volume.ListOptions{})
	volumeCount := 0
	if err == nil {
		volumeCount = len(volumeList.Volumes)
	}

	// 获取网络列表以统计数量
	networkList, err := cli.NetworkList(ctx, types.NetworkListOptions{})
	networkCount := 0
	if err == nil {
		networkCount = len(networkList)
	}

	// 获取系统内存信息
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取内存信息失败", err)
		return
	}

	cpuSnapshot := currentHostCPUSnapshot()

	// 获取磁盘信息
	diskInfo, err := disk.Usage("/")
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取磁盘信息失败", err)
		return
	}

	// 计算Docker运行时间
	startTime, err := time.Parse(time.RFC3339, info.SystemTime)
	if err != nil {
		startTime = time.Now()
	}
	uptime := int64(time.Since(startTime).Seconds())

	// 构建响应
	response := gin.H{
		"ServerVersion":           info.ServerVersion,
		"DockerVersion":           versionInfo.Version,
		"DockerAPIVersion":        versionInfo.APIVersion,
		"DaemonMinAPIVersion":     daemonMinAPIVersion,
		"MinAPIVersionFixNeeded":  minAPIVersionFixNeeded,
		"MinAPIVersionFixApplied": minAPIVersionFixApplied,
		"MinAPIVersionFixError":   minAPIVersionFixError,
		"MinAPIVersionFixTarget":  minAPIVersionFixTarget,
		"DaemonConfigReadable":    daemonErr == nil,
		"NCPU":                    info.NCPU,
		"MemTotal":                memInfo.Total,
		"MemUsage":                memInfo.Used,
		"DiskTotal":               diskInfo.Total,
		"DiskUsage":               diskInfo.Used,
		"SystemTime":              info.SystemTime,
		"SystemUptime":            uptime,
		"OS":                      runtime.GOOS,
		"Arch":                    runtime.GOARCH,
		"Containers":              info.Containers,
		"ContainersRunning":       info.ContainersRunning,
		"ContainersPaused":        info.ContainersPaused,
		"ContainersStopped":       info.ContainersStopped,
		"Images":                  info.Images,
		"Volumes":                 volumeCount,
		"Networks":                networkCount,
		"ProjectRoot":             settings.GetProjectRoot(),
		"HostProjectRoot":         effectiveHostProjectRoot(),
		"ConfiguredProjectRoot":   settings.GetHostProjectRoot(),
	}
	for key, value := range cpuInfoResponseFields(cpuSnapshot) {
		response[key] = value
	}

	c.JSON(http.StatusOK, response)
}

// 获取系统实时监控数据
func getSystemStats(c *gin.Context) {
	systemStatsCache.Lock()
	if systemStatsCache.data != nil && !systemStatsCache.updatedAt.IsZero() && time.Since(systemStatsCache.updatedAt) < 5*time.Second {
		cached := gin.H{}
		for key, value := range systemStatsCache.data {
			cached[key] = value
		}
		cached["cached"] = true
		cached["cache_age_ms"] = time.Since(systemStatsCache.updatedAt).Milliseconds()
		systemStatsCache.Unlock()
		c.JSON(http.StatusOK, cached)
		return
	}
	systemStatsCache.Unlock()

	systemStatsRefreshMu.Lock()
	defer systemStatsRefreshMu.Unlock()

	systemStatsCache.Lock()
	if systemStatsCache.data != nil && !systemStatsCache.updatedAt.IsZero() && time.Since(systemStatsCache.updatedAt) < 5*time.Second {
		cached := gin.H{}
		for key, value := range systemStatsCache.data {
			cached[key] = value
		}
		cached["cached"] = true
		cached["cache_age_ms"] = time.Since(systemStatsCache.updatedAt).Milliseconds()
		systemStatsCache.Unlock()
		c.JSON(http.StatusOK, cached)
		return
	}
	systemStatsCache.Unlock()

	cpuSnapshot := currentHostCPUSnapshot()

	// 获取内存使用率
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取内存信息失败", err)
		return
	}

	// 获取磁盘使用率
	diskInfo, err := disk.Usage("/")
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取磁盘信息失败", err)
		return
	}

	// 获取Docker容器资源使用情况
	containerStats, err := getContainersStats(c.Request.Context())
	if err != nil {
		log.Printf("[ERROR] 获取容器统计信息失败: %v\n", err)
		// 继续执行，不返回错误
	}

	// 构建响应
	response := gin.H{
		"memory_percent":  memInfo.UsedPercent,
		"disk_percent":    diskInfo.UsedPercent,
		"container_stats": containerStats,
		"timestamp":       time.Now().Unix(),
		"cached":          false,
	}
	for key, value := range cpuStatsResponseFields(cpuSnapshot) {
		response[key] = value
	}

	systemStatsCache.Lock()
	systemStatsCache.data = response
	systemStatsCache.updatedAt = time.Now()
	systemStatsCache.Unlock()

	c.JSON(http.StatusOK, response)
}

func getDiskUsage(c *gin.Context) {
	usage, ok := readDockerDisplayUsage(c, "all")
	if !ok {
		return
	}

	var totalImagesSize int64
	for _, img := range usage.Images {
		totalImagesSize += img.Size
	}

	var totalContainersSize int64
	for _, ctr := range usage.Containers {
		totalContainersSize += ctr.SizeRw
	}

	var totalVolumesSize int64
	for _, vol := range usage.Volumes {
		if vol.UsageData != nil {
			totalVolumesSize += vol.UsageData.Size
		}
	}

	var totalBuildCacheSize int64
	activeBuildCache := 0
	for _, bc := range usage.BuildCache {
		totalBuildCacheSize += bc.Size
		if bc.InUse {
			activeBuildCache++
		}
	}

	reclaimableSize := totalImagesSize + totalContainersSize + totalVolumesSize + totalBuildCacheSize - usage.LayersSize
	if reclaimableSize < 0 {
		reclaimableSize = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"layers_size":            usage.LayersSize,
		"images":                 usage.Images,
		"containers":             usage.Containers,
		"volumes":                usage.Volumes,
		"build_cache":            usage.BuildCache,
		"total_images_size":      totalImagesSize,
		"total_containers_size":  totalContainersSize,
		"total_volumes_size":     totalVolumesSize,
		"total_build_cache_size": totalBuildCacheSize,
		"build_cache_count":      len(usage.BuildCache),
		"active_build_cache":     activeBuildCache,
		"reclaimable_size":       reclaimableSize,
	})
}

// 获取所有容器的资源使用情况
func getContainersStats(ctx context.Context) ([]gin.H, error) {
	cli, err := docker.NewDockerClient()
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	// 获取所有运行中的容器
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
		All: false, // 只获取运行中的容器
	})
	if err != nil {
		return nil, err
	}

	var stats []gin.H
	for _, container := range containers {
		// 获取容器统计信息
		containerStats, err := cli.ContainerStats(ctx, container.ID, false)
		if err != nil {
			continue
		}

		// 解析统计信息
		var statsJSON types.StatsJSON
		if err := json.NewDecoder(containerStats.Body).Decode(&statsJSON); err != nil {
			_ = containerStats.Body.Close()
			continue
		}
		_ = containerStats.Body.Close()

		// 计算CPU使用率
		cpuDelta := float64(statsJSON.CPUStats.CPUUsage.TotalUsage - statsJSON.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(statsJSON.CPUStats.SystemUsage - statsJSON.PreCPUStats.SystemUsage)
		cpuPercent := 0.0
		if systemDelta > 0 && cpuDelta > 0 {
			cpuPercent = (cpuDelta / systemDelta) * float64(len(statsJSON.CPUStats.CPUUsage.PercpuUsage)) * 100.0
		}

		// 计算内存使用率
		memoryUsage := float64(statsJSON.MemoryStats.Usage)
		memoryLimit := float64(statsJSON.MemoryStats.Limit)
		memoryPercent := 0.0
		if memoryLimit > 0 {
			memoryPercent = (memoryUsage / memoryLimit) * 100.0
		}

		var networkRxBytes uint64
		var networkTxBytes uint64
		for _, nw := range statsJSON.Networks {
			networkRxBytes += nw.RxBytes
			networkTxBytes += nw.TxBytes
		}

		var blockReadBytes uint64
		var blockWriteBytes uint64
		for _, entry := range statsJSON.BlkioStats.IoServiceBytesRecursive {
			switch strings.ToLower(entry.Op) {
			case "read":
				blockReadBytes += entry.Value
			case "write":
				blockWriteBytes += entry.Value
			}
		}

		// 添加到结果
		name := container.ID[:12]
		if len(container.Names) > 0 {
			name = strings.TrimPrefix(container.Names[0], "/")
		}
		labels := container.Labels
		projectName := ""
		if labels != nil {
			projectName = strings.TrimSpace(labels["com.docker.compose.project"])
		}
		stats = append(stats, gin.H{
			"id":             container.ID[:12],
			"container_id":   container.ID,
			"name":           name,
			"project":        projectName,
			"cpu_percent":    cpuPercent,
			"memory_percent": memoryPercent,
			"memory_usage":   memoryUsage,
			"memory_limit":   memoryLimit,
			"network_rx":     networkRxBytes,
			"network_tx":     networkTxBytes,
			"block_read":     blockReadBytes,
			"block_write":    blockWriteBytes,
		})
	}

	return stats, nil
}

// 获取Docker版本信息
func getDockerVersion() (string, error) {
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// 获取主机名
func getHostname() (string, error) {
	cmd := exec.Command("hostname")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// 获取系统负载
func getSystemLoad() (float64, float64, float64, error) {
	if runtime.GOOS == "windows" {
		// Windows不支持获取负载平均值，返回CPU使用率
		cpuPercent, err := cpu.Percent(0, false)
		if err != nil {
			return 0, 0, 0, err
		}
		return cpuPercent[0] / 100, 0, 0, nil
	}

	// Linux/Unix系统获取负载平均值
	cmd := exec.Command("cat", "/proc/loadavg")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, 0, err
	}

	parts := strings.Fields(string(output))
	if len(parts) < 3 {
		return 0, 0, 0, fmt.Errorf("无法解析负载平均值")
	}

	load1, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, 0, 0, err
	}

	load5, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, 0, 0, err
	}

	load15, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, 0, 0, err
	}

	return load1, load5, load15, nil
}

// getSystemEvents 获取系统事件（改为从本地日志文件读取）
func getSystemEvents(c *gin.Context) {
	logs, err := system.GetRecentLogs(100)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "读取系统日志失败", err)
		return
	}

	// 转换格式以匹配前端期望
	// 注：应用商店 CDN 状态不再作为系统事件注入（用户已在设置页配置，事件列表里重复提示是噪音）。
	// CDN 状态仍可通过 GET /system/cdn-status 单独查询。
	eventList := make([]gin.H, 0, len(logs))
	for _, log := range logs {
		eventList = append(eventList, gin.H{
			"id":        log.ID,
			"type":      log.Type,
			"typeClass": log.TypeClass,
			"time":      log.Time,
			"message":   log.Message,
			"timestamp": log.Timestamp,
		})
	}

	c.JSON(http.StatusOK, eventList)
}

type notificationRequest struct {
	Type      string `json:"type"`
	EventType string `json:"eventType"`
	Category  string `json:"category"`
	Message   string `json:"message"`
}

func addNotification(c *gin.Context) {
	var req notificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的通知参数", err)
		return
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		respondError(c, http.StatusBadRequest, "message is required", nil)
		return
	}
	n := &database.Notification{
		Type:      req.Type,
		EventType: req.EventType,
		Category:  req.Category,
		Message:   message,
		Read:      false,
	}
	if err := database.SaveNotification(n); err != nil {
		respondError(c, http.StatusInternalServerError, "保存通知失败", err)
		return
	}
	c.JSON(http.StatusOK, n)
}

func getNotifications(c *gin.Context) {
	limitStr := c.Query("limit")
	limit := 50
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	if pageSizeRaw := c.Query("pageSize"); pageSizeRaw != "" {
		if v, err := strconv.Atoi(pageSizeRaw); err == nil && v > 0 {
			limit = v
		}
	}
	page := 1
	if pageRaw := c.Query("page"); pageRaw != "" {
		if v, err := strconv.Atoi(pageRaw); err == nil && v > 0 {
			page = v
		}
	}
	beforeID := int64(0)
	if beforeIDRaw := c.Query("before_id"); beforeIDRaw != "" {
		if v, err := strconv.ParseInt(beforeIDRaw, 10, 64); err == nil && v > 0 {
			beforeID = v
		}
	}

	var list []database.Notification
	var err error
	if beforeID > 0 {
		list, err = database.GetNotificationsBeforeID(limit, beforeID)
	} else {
		offset := (page - 1) * limit
		list, err = database.GetNotificationsPage(limit, offset)
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取通知失败", err)
		return
	}
	c.JSON(http.StatusOK, list)
}

func getNotificationSummary(c *gin.Context) {
	summary, err := database.GetNotificationSummary()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取通知摘要失败", err)
		return
	}
	c.JSON(http.StatusOK, summary)
}

func deleteNotification(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		respondError(c, http.StatusBadRequest, "invalid id", err)
		return
	}
	if err := database.DeleteNotification(id); err != nil {
		respondError(c, http.StatusInternalServerError, "删除通知失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func clearAllNotifications(c *gin.Context) {
	if err := database.ClearAllNotifications(); err != nil {
		respondError(c, http.StatusInternalServerError, "清空通知失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func markNotificationsRead(c *gin.Context) {
	if err := database.MarkAllNotificationsRead(); err != nil {
		respondError(c, http.StatusInternalServerError, "标记通知已读失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}
