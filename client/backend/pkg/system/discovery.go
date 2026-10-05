package system

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"dockerpanel/backend/pkg/database"
	tradisdocker "dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/settings"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

var containerDiscoveryHooksMu sync.RWMutex
var listContainersForDiscovery = listContainersForDiscoveryWithDocker

var navigationCleanupHooksMu sync.RWMutex
var listContainersForNavigationCleanup = listContainersForNavigationCleanupWithDocker

const (
	navHiddenReasonManual               = "manual"
	navHiddenReasonContainerMissing     = "container_missing"
	navHiddenReasonContainerRemoved     = "container_removed"
	navHiddenReasonContainerStopped     = "container_stopped"
	navHiddenReasonContainerPaused      = "container_paused"
	navHiddenReasonContainerUnavailable = "container_unavailable"
)

func cleanupOrphanAutoNavigation(validSourceKeys map[string]struct{}) {
	cleanupOrphanAutoNavigationContext(context.Background(), validSourceKeys)
}

func cleanupOrphanAutoNavigationContext(ctx context.Context, validSourceKeys map[string]struct{}) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	db := database.GetDB()
	if db == nil {
		return
	}
	rows, err := db.QueryContext(ctx, "SELECT id, container_id FROM navigation_items WHERE is_auto = 1")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		if ctx.Err() != nil {
			return
		}
		var id int
		var containerID sql.NullString
		if scanErr := rows.Scan(&id, &containerID); scanErr != nil {
			continue
		}
		if !containerID.Valid || strings.TrimSpace(containerID.String) == "" {
			if ctx.Err() != nil {
				return
			}
			_, _ = db.ExecContext(ctx, "DELETE FROM navigation_items WHERE id = ?", id)
			continue
		}
		if _, ok := validSourceKeys[strings.TrimSpace(containerID.String)]; !ok {
			hideAutoNavigationByIDContext(ctx, id, navHiddenReasonContainerMissing)
		}
	}
}

func CleanupOrphanAutoNavigationNow() {
	CleanupOrphanAutoNavigationNowContext(context.Background())
}

func listContainersForNavigationCleanupWithDocker(ctx context.Context) ([]types.Container, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	return cli.ContainerList(ctx, types.ContainerListOptions{All: true})
}

func CleanupOrphanAutoNavigationNowContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	navigationCleanupHooksMu.RLock()
	listContainers := listContainersForNavigationCleanup
	navigationCleanupHooksMu.RUnlock()
	containers, err := listContainers(ctx)
	if err != nil {
		return
	}
	cleanupOrphanAutoNavigationContext(ctx, buildValidNavigationSourceKeys(containers))
}

func listContainersForDiscoveryWithDocker(ctx context.Context) ([]types.Container, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	return cli.ContainerList(ctx, types.ContainerListOptions{All: true})
}

// ProcessContainerDiscovery 扫描现有容器并注册导航项
func ProcessContainerDiscovery() {
	processContainerDiscovery(context.Background(), true)
}

// ProcessContainerDiscoveryContext scans containers while observing cancellation.
func ProcessContainerDiscoveryContext(ctx context.Context) {
	processContainerDiscovery(ctx, false)
}

func processContainerDiscovery(ctx context.Context, allowAsyncAI bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	logging.Debug("container discovery started")
	if ctx.Err() != nil {
		return
	}
	if database.GetDB() == nil {
		return
	}

	containerDiscoveryHooksMu.RLock()
	listContainers := listContainersForDiscovery
	containerDiscoveryHooksMu.RUnlock()
	containers, err := listContainers(ctx)
	if err != nil {
		logging.Warn("container discovery could not list containers", "error", err)
		return
	}
	logging.Debug("container discovery listed containers", "count", len(containers))

	if ctx.Err() != nil {
		return
	}
	cleanupOrphanAutoNavigationContext(ctx, buildValidNavigationSourceKeys(containers))
	if ctx.Err() != nil {
		return
	}
	registered := updateNavigationForContainersWithContext(ctx, containers, allowAsyncAI)
	logging.Debug("container discovery completed", "registered", registered)

	// 完整版会补偿此前未完成的 AI 导航识别；社区版保持本地发现，
	// 但不把 AI 回填挂入启动路径。
	if attempted := editionRunNavigationAIBackfill(ctx, 50); attempted > 0 {
		logging.Debug("navigation AI backfill scheduled", "count", attempted)
	}
}

// RebuildAutoNavigationAll 清空并重新生成所有自动发现的导航项（不会影响手动添加的导航项）。
func RebuildAutoNavigationAll() {
	logging.Info("automatic navigation rebuild started")
	editionSuppressAutoNavigationAIEnrichFor(20 * time.Second)
	db := database.GetDB()
	result, err := db.Exec("DELETE FROM navigation_items WHERE is_auto = 1")
	if err != nil {
		logging.Warn("automatic navigation rebuild could not clear existing items", "error", err)
	} else {
		if affected, _ := result.RowsAffected(); affected > 0 {
			logging.Debug("automatic navigation items cleared", "count", affected)
		} else {
			logging.Debug("automatic navigation rebuild found no existing items")
		}
	}

	ProcessContainerDiscovery()

	// 检查生成后的数量
	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM navigation_items WHERE is_auto = 1 AND is_deleted = 0").Scan(&count)
	logging.Info("automatic navigation rebuild completed", "count", count)
}

// RebuildAutoNavigationForContainer 仅针对指定容器重建自动导航项（容器不存在则仅清理）。
func RebuildAutoNavigationForContainer(containerID string) {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return
	}
	editionSuppressAutoNavigationAIEnrichFor(20 * time.Second)

	db := database.GetDB()
	_, _ = db.Exec("DELETE FROM navigation_items WHERE container_id = ? AND is_auto = 1", containerID)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logging.Warn("container navigation rebuild could not create Docker client", "error", err)
		return
	}
	defer cli.Close()

	ctr, err := cli.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return
	}

	if project := composeProjectFromLabels(ctr.Config.Labels); project != "" {
		RebuildAutoNavigationForComposeProject(project)
		return
	}

	title := buildTitle(ctr.Name, ctr.Config.Labels)
	processContainer(ctr.ID, title, ctr.NetworkSettings.Ports, ctr.Config.Labels, ctr.Config.Image)
}

// RebuildAutoNavigationForComposeProject 仅针对指定 Compose 项目重建自动导航项（不会影响其他项目）。
func RebuildAutoNavigationForComposeProject(projectName string) {
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return
	}
	editionSuppressAutoNavigationAIEnrichFor(20 * time.Second)

	db := database.GetDB()
	sourceKey := navigationComposeSourceKey(projectName)
	_, _ = db.Exec("DELETE FROM navigation_items WHERE is_auto = 1 AND (container_id = ? OR title = ? OR title LIKE ?)", sourceKey, projectName, projectName+"-%")

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logging.Warn("project navigation rebuild could not create Docker client", "error", err)
		return
	}
	defer cli.Close()

	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+projectName),
		),
	})
	if err != nil {
		logging.Warn("project navigation rebuild could not list containers", "project", projectName, "error", err)
		return
	}

	containerIDs := make([]any, 0, len(containers))
	for _, ctr := range containers {
		containerIDs = append(containerIDs, ctr.ID)
	}
	for _, id := range containerIDs {
		_, _ = db.Exec("DELETE FROM navigation_items WHERE container_id = ? AND is_auto = 1", id)
	}
	updateNavigationForContainers(containers)
}

// WatchContainerEvents 监听容器事件以更新导航项。
// 该函数会阻塞当前 goroutine 直到进程退出；调用方应通过 `go WatchContainerEvents()` 启动。
// 监听流断开后会自动重连，避免导航自动发现因一次性错误永久失效。
func WatchContainerEvents() {
	for {
		watchContainerEventsOnce()
		// 断开后等待一段时间再重试，避免连接抖动
		time.Sleep(5 * time.Second)
	}
}

// watchContainerEventsOnce 建立一次 Docker 事件监听直到出错返回。
// 每次调用都创建并关闭独立的 client，保证资源不泄漏。
func watchContainerEventsOnce() {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logging.Warn("navigation event watcher could not create Docker client", "error", err)
		return
	}
	defer cli.Close()

	msgs, errs := cli.Events(context.Background(), types.EventsOptions{
		Filters: filters.NewArgs(
			filters.Arg("type", "container"),
			filters.Arg("event", "start"),
			filters.Arg("event", "unpause"),
			filters.Arg("event", "pause"),
			filters.Arg("event", "destroy"),
			filters.Arg("event", "die"),
			filters.Arg("event", "rename"),
		),
	})

	for {
		select {
		case event := <-msgs:
			handleContainerEvent(cli, event)
		case err, ok := <-errs:
			if !ok {
				// 通道关闭，退出本次监听，由外层 WatchContainerEvents 重连
				return
			}
			if err != nil {
				logging.Warn("navigation event watcher stopped", "error", err)
				return
			}
		}
	}
}

func handleContainerEvent(cli *client.Client, event events.Message) {
	switch event.Action {
	case "start", "unpause", "rename":
		// Inspect container to get details
		container, err := cli.ContainerInspect(context.Background(), event.Actor.ID)
		if err != nil {
			logging.Warn("navigation event could not inspect container", "container_id", event.Actor.ID, "error", err)
			return
		}

		if project := composeProjectFromLabels(container.Config.Labels); project != "" {
			refreshAutoNavigationForComposeProject(project)
			return
		}

		// 处理容器端口映射和导航注册
		title := buildTitle(container.Name, container.Config.Labels)
		processContainer(container.ID, title, container.NetworkSettings.Ports, container.Config.Labels, container.Config.Image)

	case "destroy":
		if project := composeProjectFromLabels(event.Actor.Attributes); project != "" {
			refreshAutoNavigationForComposeProject(project, navHiddenReasonContainerRemoved)
		} else {
			hideNavigationForContainer(event.Actor.ID, navHiddenReasonContainerRemoved)
		}
		if tx, err := database.GetDB().Begin(); err == nil {
			if derr := database.DeleteReservedPortsByOwnerTx(tx, event.Actor.ID); derr != nil {
				_ = tx.Rollback()
			} else {
				_ = tx.Commit()
			}
		}

	case "die":
		if project := composeProjectFromLabels(event.Actor.Attributes); project != "" {
			refreshAutoNavigationForComposeProject(project, navHiddenReasonContainerStopped)
		} else {
			hideNavigationForContainer(event.Actor.ID, navHiddenReasonContainerStopped)
		}
	case "pause":
		if project := composeProjectFromLabels(event.Actor.Attributes); project != "" {
			refreshAutoNavigationForComposeProject(project, navHiddenReasonContainerPaused)
		} else {
			hideNavigationForContainer(event.Actor.ID, navHiddenReasonContainerPaused)
		}
		return
	}
}

// refreshAutoNavigationForComposeProject 只刷新 Compose 项目的端口和地址。
// 容器启停属于运行状态变化，不应删除已有标题、分类、图标和 AI 状态。
func refreshAutoNavigationForComposeProject(projectName string, emptyReasons ...string) {
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logging.Warn("project navigation refresh could not create Docker client", "error", err)
		return
	}
	defer cli.Close()

	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+projectName),
		),
	})
	if err != nil {
		logging.Warn("project navigation refresh could not list containers", "project", projectName, "error", err)
		return
	}
	if len(containers) == 0 {
		// compose update/down-up 的 destroy 与 create 之间可能暂时没有容器。
		// 保留稳定项目导航，后续 start 事件会继续刷新。
		if len(emptyReasons) > 0 && strings.TrimSpace(emptyReasons[0]) != "" {
			hideAutoNavigationBySource(navigationComposeSourceKey(projectName), emptyReasons[0])
		}
		return
	}
	updateNavigationForContainers(containers)
}

func shouldProbeWebPorts(s settings.Settings) bool {
	if !navigationAIConfigurationFor(s).Enabled {
		return false
	}
	return strings.TrimSpace(s.LanUrl) != "" || strings.TrimSpace(s.WanUrl) != ""
}

type navigationContainerCandidate struct {
	ContainerID string
	Title       string
	Ports       []int
	Labels      map[string]string
	Image       string
	Name        string
	State       string
	Project     string
	Service     string
}

type navigationPortChoice struct {
	Candidate navigationContainerCandidate
	Port      int
}

func updateNavigationForContainers(containers []types.Container) int {
	return updateNavigationForContainersWithContext(context.Background(), containers, true)
}

func updateNavigationForContainersContext(ctx context.Context, containers []types.Container) int {
	return updateNavigationForContainersWithContext(ctx, containers, false)
}

func updateNavigationForContainersWithContext(ctx context.Context, containers []types.Container, allowAsyncAI bool) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return 0
	}
	s, err := settings.GetSettings()
	if err != nil {
		logging.Warn("container discovery could not load settings", "error", err)
		return 0
	}
	if ctx.Err() != nil {
		return 0
	}

	standalone := make([]navigationContainerCandidate, 0, len(containers))
	composeGroups := make(map[string][]navigationContainerCandidate)
	composeSeen := make(map[string]string)

	for _, container := range containers {
		if ctx.Err() != nil {
			return 0
		}
		candidate := navigationCandidateFromContainer(container)
		running := strings.EqualFold(strings.TrimSpace(candidate.State), "running")
		hiddenReason := navigationHiddenReasonForContainerState(candidate.State)
		if candidate.Project != "" {
			if _, ok := composeSeen[candidate.Project]; !ok || hiddenReason == navHiddenReasonContainerPaused {
				composeSeen[candidate.Project] = hiddenReason
			}
		}
		if !running {
			if candidate.Project == "" {
				hideNavigationForContainerContext(ctx, candidate.ContainerID, hiddenReason)
			}
			continue
		}
		if len(candidate.Ports) == 0 && isHostNetworkContainer(container) {
			// host 网络容器：无端口映射，经 docker socket 探测其自身暴露端口。
			// 先用列表里的 NetworkMode 预过滤，避免对普通容器逐个 inspect。
			if hostPorts, ok := hostNetworkTCPPorts(ctx, candidate.ContainerID, candidate.Labels); ok {
				candidate.Ports = hostPorts
				logging.Debug("host network container ports discovered", "container", candidate.Name, "ports", hostPorts)
			}
		}
		if len(candidate.Ports) == 0 {
			logging.Debug("container has no public TCP ports", "container", candidate.Name)
			if candidate.Project == "" {
				hideNavigationForContainerContext(ctx, candidate.ContainerID, navHiddenReasonContainerUnavailable)
			}
			continue
		}
		logging.Debug("container public TCP ports discovered", "container", candidate.Name, "port_count", len(candidate.Ports))

		if candidate.Project != "" {
			composeGroups[candidate.Project] = append(composeGroups[candidate.Project], candidate)
			continue
		}
		standalone = append(standalone, candidate)
	}

	registered := 0
	for _, candidate := range standalone {
		if ctx.Err() != nil {
			return registered
		}
		resolveAndRegisterNavigationWithContext(ctx, candidate.ContainerID, candidate.Title, candidate.Ports, candidate.Labels, candidate.Image, s, allowAsyncAI)
		registered++
	}
	for project, candidates := range composeGroups {
		if ctx.Err() != nil {
			return registered
		}
		if updateNavigationForComposeProjectWithContext(ctx, project, candidates, s, allowAsyncAI) {
			registered++
		}
	}
	for project, reason := range composeSeen {
		if ctx.Err() != nil {
			return registered
		}
		if _, ok := composeGroups[project]; ok {
			continue
		}
		hideAutoNavigationBySourceContext(ctx, navigationComposeSourceKey(project), reason)
	}
	return registered
}

// isHostNetworkContainer 从 ContainerList 结果判断 host 网络模式，
// 用于在 inspect 之前预过滤（列表接口已携带 HostConfig.NetworkMode）。
func isHostNetworkContainer(container types.Container) bool {
	return strings.EqualFold(strings.TrimSpace(container.HostConfig.NetworkMode), "host")
}

// hostNetworkTCPPorts 通过 docker socket 识别 host 网络容器的可访问端口：
// host 模式没有端口映射（PublicPort=0），可直接访问的就是容器自身端口。
// 注意 host 模式下 inspect.NetworkSettings.Ports 恒为空，端口来自
// 镜像 EXPOSE 声明（Config.ExposedPorts）；未声明 EXPOSE 的镜像可用
// tradis.navigation.port 标签显式补充。
func hostNetworkTCPPorts(ctx context.Context, containerID string, labels map[string]string) ([]int, bool) {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return nil, false
	}
	var ports []int
	err := tradisdocker.WithClient(ctx, func(cli *tradisdocker.Client) error {
		inspect, inspectErr := cli.ContainerInspect(ctx, containerID)
		if inspectErr != nil {
			return inspectErr
		}
		if inspect.HostConfig == nil || !strings.EqualFold(strings.TrimSpace(string(inspect.HostConfig.NetworkMode)), "host") {
			return nil
		}
		if inspect.Config == nil {
			return nil
		}
		seen := make(map[int]struct{}, len(inspect.Config.ExposedPorts))
		for portProto := range inspect.Config.ExposedPorts {
			proto := string(portProto)
			if !strings.HasSuffix(proto, "/tcp") {
				continue
			}
			value, parseErr := strconv.Atoi(strings.SplitN(proto, "/", 2)[0])
			if parseErr != nil || value <= 0 {
				continue
			}
			if _, dup := seen[value]; dup {
				continue
			}
			seen[value] = struct{}{}
			ports = append(ports, value)
		}
		return nil
	})
	if err != nil {
		return nil, false
	}
	if labelPort := explicitNavigationLabelPort(labels); labelPort > 0 {
		ports = append(ports, labelPort)
	}
	if len(ports) == 0 {
		return nil, false
	}
	sort.Ints(ports)
	return dedupeSortedInts(ports), true
}

// explicitNavigationLabelPort 读取显式指定的导航端口标签。
func explicitNavigationLabelPort(labels map[string]string) int {
	if labels == nil {
		return 0
	}
	raw := strings.TrimSpace(labels["tradis.navigation.port"])
	if raw == "" {
		raw = strings.TrimSpace(labels["trabis.navigation.port"])
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func dedupeSortedInts(values []int) []int {
	if len(values) == 0 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func navigationCandidateFromContainer(container types.Container) navigationContainerCandidate {
	name := ""
	if len(container.Names) > 0 {
		name = strings.TrimPrefix(container.Names[0], "/")
	}
	labels := container.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	project := composeProjectFromLabels(labels)
	service := strings.TrimSpace(labels["com.docker.compose.service"])
	return navigationContainerCandidate{
		ContainerID: container.ID,
		Title:       buildTitle(name, labels),
		Ports:       publicTCPPortsFromContainer(container),
		Labels:      labels,
		Image:       container.Image,
		Name:        name,
		State:       container.State,
		Project:     project,
		Service:     service,
	}
}

func navigationHiddenReasonForContainerState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "paused":
		return navHiddenReasonContainerPaused
	case "exited", "dead", "removing", "created":
		return navHiddenReasonContainerStopped
	default:
		return navHiddenReasonContainerUnavailable
	}
}

func publicTCPPortsFromContainer(container types.Container) []int {
	ports := make([]int, 0, 4)
	seen := make(map[int]struct{}, 4)
	for _, p := range container.Ports {
		if p.Type != "tcp" || p.PublicPort == 0 {
			continue
		}
		port := int(p.PublicPort)
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports
}

func buildValidNavigationSourceKeys(containers []types.Container) map[string]struct{} {
	keys := make(map[string]struct{}, len(containers)*2)
	for _, ctr := range containers {
		if strings.TrimSpace(ctr.ID) != "" {
			keys[ctr.ID] = struct{}{}
		}
		// 停止后的 Compose 容器可能不再返回端口，但项目和导航记录仍然有效。
		// 保留稳定的 compose:<project> 源，避免启动时重新创建导航和重复 AI 识别。
		if project := composeProjectFromLabels(ctr.Labels); project != "" {
			base := navigationComposeSourceKey(project)
			keys[base] = struct{}{}
			// 多服务导航的稳定键也参与孤儿校验，避免被清理误删。
			if service := strings.TrimSpace(ctr.Labels["com.docker.compose.service"]); service != "" {
				keys[composeServiceNavigationSourceKey(project, service)] = struct{}{}
			}
		}
	}
	return keys
}

func composeProjectFromLabels(labels map[string]string) string {
	if labels == nil {
		return ""
	}
	return strings.TrimSpace(labels["com.docker.compose.project"])
}

func navigationComposeSourceKey(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return ""
	}
	return "compose:" + project
}

func updateNavigationForComposeProject(project string, candidates []navigationContainerCandidate, s settings.Settings) bool {
	return updateNavigationForComposeProjectWithContext(context.Background(), project, candidates, s, true)
}

func updateNavigationForComposeProjectWithContext(ctx context.Context, project string, candidates []navigationContainerCandidate, s settings.Settings, allowAsyncAI bool) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return false
	}
	project = strings.TrimSpace(project)
	if project == "" || len(candidates) == 0 {
		return false
	}
	// 每个服务独立挑选端口：多 HTTP 服务的 Compose 项目为每个可访问服务
	// 各注册一条导航（标题 project-service），单服务保持项目名标题兼容。
	// 非 Web 噪音控制：只有显式 label、HTTP 探测通过或导航评分为正的服务
	// 才逐服务注册；都不满足时退回项目级单条目兜底（旧行为）。
	entries := make([]navigationPortChoice, 0, len(candidates))
	seenServicePort := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		choice, ok := selectServiceWebPortChoice(candidate, s)
		if !ok || choice.Port == 0 {
			continue
		}
		identity := candidate.Service + "\x00" + strconv.Itoa(choice.Port)
		if _, exists := seenServicePort[identity]; exists {
			continue
		}
		seenServicePort[identity] = struct{}{}
		entries = append(entries, choice)
	}
	if len(entries) == 0 {
		// 项目级兜底：没有任何服务满足 Web 门槛时保持旧的单条目选择。
		if choice, ok := selectBestNavigationPortChoice(candidates, s); ok && choice.Port != 0 {
			entries = append(entries, choice)
		}
	}
	sourceKey := navigationComposeSourceKey(project)
	if len(entries) == 0 {
		markAutoNavigationDeletedContext(ctx, sourceKey, navHiddenReasonContainerUnavailable)
		return false
	}

	db := database.GetDB()
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return false
		}
		if strings.TrimSpace(candidate.ContainerID) != "" {
			_, _ = db.ExecContext(ctx, "DELETE FROM navigation_items WHERE container_id = ? AND is_auto = 1", candidate.ContainerID)
		}
	}
	if ctx.Err() != nil {
		return false
	}
	// 清理项目级旧键（单键与历史服务键），但保留本次要写入的键：
	// registerNavigation 按 container_id 键更新并保留已有图标。
	keepKeys := make([]any, 0, len(entries))
	if len(entries) == 1 {
		keepKeys = append(keepKeys, sourceKey)
	} else {
		for _, entry := range entries {
			keepKeys = append(keepKeys, composeServiceNavigationSourceKey(project, composeServiceNavigationLabel(entry.Candidate)))
		}
	}
	placeholders := make([]string, 0, len(keepKeys))
	for range keepKeys {
		placeholders = append(placeholders, "?")
	}
	_, _ = db.ExecContext(ctx,
		"DELETE FROM navigation_items WHERE is_auto = 1 AND (container_id = ? OR container_id LIKE ? ESCAPE '\\') AND container_id NOT IN ("+strings.Join(placeholders, ",")+")",
		append([]any{sourceKey, escapeSQLLikePattern(sourceKey) + ":%"}, keepKeys...)...)
	if ctx.Err() != nil {
		return false
	}

	for _, entry := range entries {
		service := composeServiceNavigationLabel(entry.Candidate)
		title := normalizeNavigationTitle(project)
		if len(entries) > 1 {
			title = normalizeNavigationTitle(project + "-" + service)
		}
		if title == "" {
			title = entry.Candidate.Title
		}
		entrySourceKey := sourceKey
		if len(entries) > 1 {
			entrySourceKey = composeServiceNavigationSourceKey(project, service)
		}
		labels := mergeNavigationLabels(entry.Candidate.Labels, map[string]string{
			"com.docker.compose.project": project,
			"com.docker.compose.service": entry.Candidate.Service,
		})
		registerNavigationWithContext(ctx, entrySourceKey, title, strconv.Itoa(entry.Port), labels, entry.Candidate.Image, s, allowAsyncAI)
	}
	return true
}

// escapeSQLLikePattern 转义 LIKE 模式中的 \、%、_，
// 防止项目名里的 _ 被当作单字符通配符误删相近项目的导航键。
func escapeSQLLikePattern(raw string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(raw)
}

// composeServiceNavigationLabel 服务展示名：compose 服务标签优先，缺省用容器名。
func composeServiceNavigationLabel(candidate navigationContainerCandidate) string {
	if service := strings.TrimSpace(candidate.Service); service != "" {
		return service
	}
	if name := strings.TrimSpace(candidate.Name); name != "" {
		return name
	}
	return strings.TrimSpace(candidate.ContainerID)
}

// composeServiceNavigationSourceKey 多服务 Compose 的稳定导航源键：
// compose:<project>:<service>，与单服务键 compose:<project> 区分。
func composeServiceNavigationSourceKey(project, service string) string {
	base := navigationComposeSourceKey(project)
	service = strings.TrimSpace(service)
	if base == "" || service == "" {
		return base
	}
	return base + ":" + service
}

func mergeNavigationLabels(base map[string]string, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	return out
}

// selectServiceWebPortChoice 为单个服务挑选导航端口，带 Web 门槛：
// 显式 label 指定的端口始终可用；探测开启时只接受探测通过的端口；
// 探测关闭时复用既有导航评分（Web 名称/端口为正、数据库类为负），
// 只注册评分为正的服务，避免把数据库等非 Web 端口逐服务注册成导航噪音。
func selectServiceWebPortChoice(candidate navigationContainerCandidate, s settings.Settings) (navigationPortChoice, bool) {
	choices := make([]navigationPortChoice, 0, len(candidate.Ports))
	for _, port := range candidate.Ports {
		if port <= 0 {
			continue
		}
		choices = append(choices, navigationPortChoice{Candidate: candidate, Port: port})
	}
	if len(choices) == 0 {
		return navigationPortChoice{}, false
	}
	for _, choice := range choices {
		if navigationPortExplicitlySelected(choice.Candidate.Labels, choice.Port) {
			return choice, true
		}
	}
	if !shouldProbeWebPorts(s) {
		sortNavigationChoices(choices)
		if navigationChoiceScore(choices[0]) > 0 {
			return choices[0], true
		}
		return navigationPortChoice{}, false
	}
	webChoices := make([]navigationPortChoice, 0, len(choices))
	for _, choice := range choices {
		if isWebPort(s.LanUrl, s.WanUrl, strconv.Itoa(choice.Port)) {
			webChoices = append(webChoices, choice)
		}
	}
	if len(webChoices) == 0 {
		return navigationPortChoice{}, false
	}
	sortNavigationChoices(webChoices)
	return webChoices[0], true
}

func selectBestNavigationPortChoice(candidates []navigationContainerCandidate, s settings.Settings) (navigationPortChoice, bool) {
	choices := make([]navigationPortChoice, 0)
	for _, candidate := range candidates {
		for _, port := range candidate.Ports {
			if port <= 0 {
				continue
			}
			choices = append(choices, navigationPortChoice{Candidate: candidate, Port: port})
		}
	}
	if len(choices) == 0 {
		return navigationPortChoice{}, false
	}

	for _, choice := range choices {
		if !navigationPortExplicitlySelected(choice.Candidate.Labels, choice.Port) {
			continue
		}
		return choice, true
	}

	if shouldProbeWebPorts(s) {
		webChoices := make([]navigationPortChoice, 0, len(choices))
		for _, choice := range choices {
			if isWebPort(s.LanUrl, s.WanUrl, strconv.Itoa(choice.Port)) {
				webChoices = append(webChoices, choice)
			}
		}
		if len(webChoices) > 0 {
			sortNavigationChoices(webChoices)
			return webChoices[0], true
		}
	}

	sortNavigationChoices(choices)
	return choices[0], true
}

func navigationPortExplicitlySelected(labels map[string]string, port int) bool {
	if labels == nil {
		return false
	}
	if parseBoolLabel(labels["tradis.navigation.primary"]) || parseBoolLabel(labels["trabis.navigation.primary"]) {
		if raw := strings.TrimSpace(labels["tradis.navigation.port"]); raw != "" {
			p, err := strconv.Atoi(raw)
			return err == nil && p == port
		}
		return true
	}
	raw := strings.TrimSpace(labels["tradis.navigation.port"])
	if raw == "" {
		raw = strings.TrimSpace(labels["trabis.navigation.port"])
	}
	if raw == "" {
		return false
	}
	p, err := strconv.Atoi(raw)
	return err == nil && p == port
}

func parseBoolLabel(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on", "primary":
		return true
	default:
		return false
	}
}

func sortNavigationChoices(choices []navigationPortChoice) {
	sort.SliceStable(choices, func(i, j int) bool {
		left := navigationChoiceScore(choices[i])
		right := navigationChoiceScore(choices[j])
		if left != right {
			return left > right
		}
		if choices[i].Candidate.Project != choices[j].Candidate.Project {
			return choices[i].Candidate.Project < choices[j].Candidate.Project
		}
		if choices[i].Candidate.Service != choices[j].Candidate.Service {
			return choices[i].Candidate.Service < choices[j].Candidate.Service
		}
		return choices[i].Port < choices[j].Port
	})
}

func navigationChoiceScore(choice navigationPortChoice) int {
	return portScore(choice.Port) + serviceNameScore(choice.Candidate.Service, choice.Candidate.Name, choice.Candidate.Image)
}

func serviceNameScore(values ...string) int {
	score := 0
	for _, value := range values {
		v := strings.ToLower(strings.TrimSpace(value))
		if v == "" {
			continue
		}
		for _, token := range []string{"web", "ui", "frontend", "front", "app", "dashboard", "console", "admin", "nginx", "caddy", "traefik"} {
			if strings.Contains(v, token) {
				score += 350
				break
			}
		}
		for _, token := range []string{"db", "postgres", "mysql", "mariadb", "redis", "mongo", "worker", "queue", "metrics", "exporter", "prometheus", "api", "backend"} {
			if strings.Contains(v, token) {
				score -= 250
				break
			}
		}
	}
	return score
}

func resolveAndRegisterNavigation(containerID string, title string, ports []int, labels map[string]string, image string, s settings.Settings) {
	resolveAndRegisterNavigationWithContext(context.Background(), containerID, title, ports, labels, image, s, true)
}

func resolveAndRegisterNavigationWithContext(ctx context.Context, containerID string, title string, ports []int, labels map[string]string, image string, s settings.Settings, allowAsyncAI bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	candidate := navigationContainerCandidate{
		ContainerID: containerID,
		Title:       title,
		Ports:       ports,
		Labels:      labels,
		Image:       image,
		Name:        title,
	}
	choice, ok := selectBestNavigationPortChoice([]navigationContainerCandidate{candidate}, s)
	if ok && choice.Port != 0 {
		registerNavigationWithContext(ctx, containerID, title, strconv.Itoa(choice.Port), labels, image, s, allowAsyncAI)
		return
	}
	markAutoNavigationDeletedContext(ctx, containerID, navHiddenReasonContainerUnavailable)
}

// updateNavigationForContainer 检查容器标签并更新导航表
func updateNavigationForContainer(container types.Container) {
	// 容器名称通常以 / 开头，去除它
	name := strings.TrimPrefix(container.Names[0], "/")
	title := buildTitle(name, container.Labels)

	// 从 Ports 中提取映射信息
	// ContainerList 返回的 Ports 结构与 Inspect 不同，需要转换或直接使用
	// types.Port: IP, PrivatePort, PublicPort, Type

	// 为了复用 processContainer 逻辑，我们需要构造类似 nat.PortMap 的结构，或者直接在这里处理
	// 简单起见，我们重新实现一个针对 types.Container 的处理逻辑，或者只提取第一个公开的 TCP 端口

	s, err := settings.GetSettings()
	if err != nil {
		logging.Warn("navigation discovery could not load settings", "error", err)
		return
	}

	publicPorts := make([]int, 0, 4)
	for _, p := range container.Ports {
		if p.Type != "tcp" || p.PublicPort == 0 {
			continue
		}
		publicPorts = append(publicPorts, int(p.PublicPort))
	}
	resolveAndRegisterNavigation(container.ID, title, publicPorts, container.Labels, container.Image, s)
}

func processContainer(containerID, title string, ports nat.PortMap, labels map[string]string, image string) {
	s, err := settings.GetSettings()
	if err != nil {
		logging.Warn("navigation discovery could not load settings", "error", err)
		return
	}

	hostPorts := make([]int, 0, 4)
	for portProto, bindings := range ports {
		if !strings.HasSuffix(string(portProto), "/tcp") || len(bindings) == 0 {
			continue
		}
		for _, b := range bindings {
			p, err := strconv.Atoi(strings.TrimSpace(b.HostPort))
			if err != nil || p <= 0 {
				continue
			}
			hostPorts = append(hostPorts, p)
		}
	}

	resolveAndRegisterNavigation(containerID, title, hostPorts, labels, image, s)
}

func registerNavigation(containerID, title, publicPort string, labels map[string]string, image string, s settings.Settings) {
	registerNavigationWithContext(context.Background(), containerID, title, publicPort, labels, image, s, true)
}

func registerNavigationWithContext(ctx context.Context, containerID, title, publicPort string, labels map[string]string, image string, s settings.Settings, allowAsyncAI bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	lanBaseUrl := strings.TrimSpace(s.LanUrl)
	wanBaseUrl := strings.TrimSpace(s.WanUrl)

	// 辅助函数：构建 URL
	buildUrl := func(base string, port string) string {
		if base == "" {
			return ""
		}
		u, err := url.Parse(strings.TrimSpace(base))
		if err == nil && u.Scheme != "" && u.Host != "" {
			host := u.Hostname()
			if host == "" {
				return ""
			}
			u.Host = host + ":" + port
			return u.String()
		}
		raw := strings.TrimRight(strings.TrimSpace(base), "/")
		raw = stripTrailingPort(raw)
		if raw == "" {
			return ""
		}
		return fmt.Sprintf("%s:%s", raw, port)
	}

	lanUrl := buildUrl(lanBaseUrl, publicPort)
	wanUrl := buildUrl(wanBaseUrl, publicPort)

	// 兼容旧的 url 字段，优先使用 lanUrl
	finalUrl := lanUrl
	if finalUrl == "" {
		finalUrl = wanUrl
	}

	// 写入数据库
	db := database.GetDB()
	if db == nil || ctx.Err() != nil {
		return
	}

	var existingID int
	var existingIcon sql.NullString
	var extraIDs []int
	rows, err := db.QueryContext(ctx, "SELECT id, icon FROM navigation_items WHERE container_id = ? AND is_auto = 1 ORDER BY id ASC", containerID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			var iconValue sql.NullString
			if scanErr := rows.Scan(&id, &iconValue); scanErr != nil {
				continue
			}
			if existingID == 0 {
				existingID = id
				existingIcon = iconValue
			} else {
				extraIDs = append(extraIDs, id)
			}
		}
	}
	for _, id := range extraIDs {
		if ctx.Err() != nil {
			return
		}
		_, _ = db.ExecContext(ctx, "DELETE FROM navigation_items WHERE id = ?", id)
	}

	icon := cacheNavigationIcon(normalizeAndResolveNavigationIconContext(ctx, "", lanUrl, wanUrl), containerID)
	category := "未分类"
	title = normalizeNavigationTitle(title)
	if title == "" {
		title = containerID
	}

	created := false
	if existingID == 0 {
		if ctx.Err() != nil {
			return
		}
		// Create new
		result, err := db.ExecContext(ctx,
			"INSERT INTO navigation_items (title, url, lan_url, wan_url, icon, category, is_auto, container_id) VALUES (?, ?, ?, ?, ?, ?, 1, ?)",
			title, finalUrl, lanUrl, wanUrl, icon, category, containerID,
		)
		if err != nil {
			logging.Warn("automatic navigation registration failed", "title", title, "error", err)
		} else {
			if id, ierr := result.LastInsertId(); ierr == nil {
				existingID = int(id)
			}
			created = existingID > 0
			logging.Debug("automatic navigation registered", "title", title)
		}
	} else {
		existingIconValue := strings.TrimSpace(existingIcon.String)
		if isGenericNavigationIcon(existingIconValue) {
			// 通用图标只代表此前未发现站点图标；后续扫描应重新探测，
			// 以覆盖容器刚启动时 Web 服务尚未就绪的情况。
			icon = cacheNavigationIcon(normalizeAndResolveNavigationIconContext(ctx, "", lanUrl, wanUrl), containerID)
		} else {
			// 已识别出的站点图标 URL 已经持久化在 navigation_items.icon，
			// 后续容器扫描直接复用，避免每次扫描再次请求外部站点。
			icon = cacheNavigationIcon(existingIconValue, containerID)
			if icon == existingIconValue && isRemoteNavigationIcon(existingIconValue) {
				// 兼容历史上保存的 localhost favicon。Client 改为 bridge 网络后，
				// 容器内 localhost 不再是 Docker 宿主机，需要从当前 LAN/WAN
				// 地址重新发现一次并迁移为本地图标。
				candidate := normalizeAndResolveNavigationIconContext(ctx, "", lanUrl, wanUrl)
				if cached := cacheNavigationIcon(candidate, containerID); strings.HasPrefix(cached, "/data/pic/") {
					icon = cached
				}
			}
		}
		if ctx.Err() != nil {
			return
		}
		// Update existing
		_, err = db.ExecContext(ctx,
			"UPDATE navigation_items SET title = ?, url = ?, lan_url = ?, wan_url = ?, icon = ?, is_deleted = CASE WHEN COALESCE(hidden_reason, '') = ? THEN is_deleted ELSE 0 END, hidden_reason = CASE WHEN COALESCE(hidden_reason, '') = ? THEN hidden_reason ELSE '' END, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
			title, finalUrl, lanUrl, wanUrl, icon, navHiddenReasonManual, navHiddenReasonManual, existingID,
		)
		if err != nil {
			logging.Warn("automatic navigation update failed", "title", title, "error", err)
		}
	}

	if allowAsyncAI && created && existingID > 0 {
		editionScheduleNavigationAIEnrichment(existingID, labels, image, s, false)
	}
}

func hideNavigationForContainer(containerID string, reason string) {
	hideNavigationForContainerContext(context.Background(), containerID, reason)
}

func hideNavigationForContainerContext(ctx context.Context, containerID string, reason string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	db := database.GetDB()
	if db == nil {
		return
	}
	_, err := db.ExecContext(ctx, "UPDATE navigation_items SET is_deleted = 1, hidden_reason = ?, updated_at = CURRENT_TIMESTAMP WHERE container_id = ? AND is_auto = 1 AND COALESCE(hidden_reason, '') != ?", normalizeNavigationHiddenReason(reason), containerID, navHiddenReasonManual)
	if err != nil {
		logging.Warn("automatic navigation hide failed", "container_id", containerID, "error", err)
	}
}

func hideAutoNavigationBySource(sourceKey string, reason string) {
	hideAutoNavigationBySourceContext(context.Background(), sourceKey, reason)
}

func hideAutoNavigationBySourceContext(ctx context.Context, sourceKey string, reason string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	db := database.GetDB()
	if db == nil {
		return
	}
	_, _ = db.ExecContext(ctx, "UPDATE navigation_items SET is_deleted = 1, hidden_reason = ?, updated_at = CURRENT_TIMESTAMP WHERE container_id = ? AND is_auto = 1 AND COALESCE(hidden_reason, '') != ?", normalizeNavigationHiddenReason(reason), sourceKey, navHiddenReasonManual)
}

func hideAutoNavigationByID(id int, reason string) {
	hideAutoNavigationByIDContext(context.Background(), id, reason)
}

func hideAutoNavigationByIDContext(ctx context.Context, id int, reason string) {
	if id <= 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	db := database.GetDB()
	if db == nil {
		return
	}
	_, _ = db.ExecContext(ctx, "UPDATE navigation_items SET is_deleted = 1, hidden_reason = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND is_auto = 1 AND COALESCE(hidden_reason, '') != ?", normalizeNavigationHiddenReason(reason), id, navHiddenReasonManual)
}

func markAutoNavigationDeleted(containerID string, reason string) {
	markAutoNavigationDeletedContext(context.Background(), containerID, reason)
}

func markAutoNavigationDeletedContext(ctx context.Context, containerID string, reason string) {
	hideAutoNavigationBySourceContext(ctx, containerID, reason)
}

func normalizeNavigationHiddenReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case navHiddenReasonContainerMissing,
		navHiddenReasonContainerRemoved,
		navHiddenReasonContainerStopped,
		navHiddenReasonContainerPaused,
		navHiddenReasonContainerUnavailable:
		return strings.TrimSpace(reason)
	default:
		return navHiddenReasonContainerUnavailable
	}
}

func buildTitle(name string, labels map[string]string) string {
	project := strings.TrimSpace(labels["com.docker.compose.project"])
	service := strings.TrimSpace(labels["com.docker.compose.service"])
	if project != "" && service != "" {
		if project == service {
			return normalizeNavigationTitle(project)
		}
		return normalizeNavigationTitle(fmt.Sprintf("%s-%s", project, service))
	}
	return normalizeNavigationTitle(strings.TrimPrefix(name, "/"))
}

func normalizeNavigationTitle(title string) string {
	title = strings.Trim(strings.TrimSpace(strings.TrimPrefix(title, "/")), "-_ ")
	if title == "" {
		return ""
	}
	for _, sep := range []string{"-", "_"} {
		parts := strings.Split(title, sep)
		if len(parts) < 2 || len(parts)%2 != 0 {
			continue
		}
		half := len(parts) / 2
		matched := true
		for i := 0; i < half; i++ {
			if parts[i] != parts[i+half] {
				matched = false
				break
			}
		}
		if matched {
			return strings.Join(parts[:half], sep)
		}
	}
	return title
}

func selectBestPublicPort(publicPorts []int, lanBaseUrl string, wanBaseUrl string, aiEnabled bool) int {
	dedup := make(map[int]struct{}, len(publicPorts))
	ports := make([]int, 0, len(publicPorts))
	for _, p := range publicPorts {
		if p <= 0 {
			continue
		}
		if _, ok := dedup[p]; ok {
			continue
		}
		dedup[p] = struct{}{}
		ports = append(ports, p)
	}
	if len(ports) == 0 {
		return 0
	}

	sort.Slice(ports, func(i, j int) bool {
		return portScore(ports[i]) > portScore(ports[j])
	})

	if !aiEnabled {
		return ports[0]
	}
	if strings.TrimSpace(lanBaseUrl) == "" && strings.TrimSpace(wanBaseUrl) == "" {
		return ports[0]
	}

	for _, p := range ports {
		if isWebPort(lanBaseUrl, wanBaseUrl, strconv.Itoa(p)) {
			return p
		}
	}
	return ports[0]
}

func portScore(p int) int {
	switch p {
	case 443:
		return 1000
	case 80:
		return 990
	case 8443:
		return 950
	case 8080:
		return 930
	case 8000:
		return 920
	case 3000:
		return 910
	case 9000:
		return 900
	case 9090:
		return 890
	case 5000:
		return 880
	default:
		if p >= 1024 && p <= 65535 {
			return 100
		}
		return 0
	}
}

func isWebPort(lanBaseUrl string, wanBaseUrl string, port string) bool {
	lan := buildHostUrl(lanBaseUrl, port)
	wan := buildHostUrl(wanBaseUrl, port)
	candidates := make([]string, 0, 4)
	if lan != "" {
		candidates = append(candidates, lan)
	}
	if wan != "" && wan != lan {
		candidates = append(candidates, wan)
	}
	if len(candidates) == 0 {
		return false
	}
	for _, u := range candidates {
		if probeWeb(u) {
			return true
		}
		if strings.HasPrefix(u, "http://") {
			if probeWeb("https://" + strings.TrimPrefix(u, "http://")) {
				return true
			}
		} else if strings.HasPrefix(u, "https://") {
			if probeWeb("http://" + strings.TrimPrefix(u, "https://")) {
				return true
			}
		}
	}
	return false
}

func buildHostUrl(baseUrl string, port string) string {
	if strings.TrimSpace(baseUrl) == "" {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(baseUrl))
	if err != nil || u.Scheme == "" || u.Host == "" {
		raw := strings.TrimRight(strings.TrimSpace(baseUrl), "/")
		raw = stripTrailingPort(raw)
		if raw == "" {
			return ""
		}
		return raw + ":" + port
	}
	host := u.Hostname()
	if host == "" {
		return ""
	}
	u.Host = host + ":" + port
	return u.String()
}

func stripTrailingPort(raw string) string {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			host := u.Hostname()
			if host == "" {
				return raw
			}
			u.Host = host
			u.Path = strings.TrimRight(u.Path, "/")
			return u.String()
		}
	}
	return raw
}

func probeWeb(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest(http.MethodHead, target, nil)
	if err == nil {
		req.Header.Set("User-Agent", "tradis-discovery/1.0")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.CopyN(io.Discard, resp.Body, 256)
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return true
			}
			if resp.StatusCode == http.StatusMethodNotAllowed {
				// fallthrough to GET
			} else {
				return false
			}
		}
	}

	getReq, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return false
	}
	getReq.Header.Set("User-Agent", "tradis-discovery/1.0")
	getReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	resp, err := client.Do(getReq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		_, _ = io.CopyN(io.Discard, resp.Body, 256)
		return false
	}
	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	body := strings.TrimSpace(string(snippet))
	if strings.HasPrefix(body, "{") {
		lower := strings.ToLower(body)
		if strings.Contains(ct, "application/json") && (strings.Contains(lower, "\"detail\"") && strings.Contains(lower, "not found")) {
			return false
		}
	}
	return true
}

func probeImage(target string) bool {
	return probeImageContext(context.Background(), target)
}

func probeImageContext(ctx context.Context, target string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "tradis-discovery/1.0")
	req.Header.Set("Accept", "image/*,application/octet-stream;q=0.9,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.CopyN(io.Discard, resp.Body, 256)
		return false
	}

	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	snippet = bytes.TrimSpace(snippet)
	lowerText := strings.ToLower(string(snippet))

	// SVG 可能带 <?xml ... <!DOCTYPE svg ...> 声明，需优先按 Content-Type 识别，
	// 避免被下方的 HTML DOCTYPE 兜底规则误杀（如 we-mp-rss 的 /static/logo.svg）。
	isSVG := func(text string) bool {
		return strings.Contains(text, "<svg")
	}
	if strings.Contains(ct, "image/svg") {
		return isSVG(lowerText)
	}

	if strings.Contains(ct, "text/html") || strings.Contains(ct, "application/json") {
		return false
	}
	if strings.HasPrefix(lowerText, "{") {
		if strings.Contains(lowerText, "\"detail\"") || strings.Contains(lowerText, "\"error\"") || strings.Contains(lowerText, "not found") {
			return false
		}
	}
	if strings.Contains(lowerText, "<!doctype html") || strings.Contains(lowerText, "<html") || strings.Contains(lowerText, "<head") || strings.Contains(lowerText, "not found") || strings.Contains(lowerText, "404") {
		return false
	}

	isICO := func(b []byte) bool {
		return len(b) >= 4 && b[0] == 0x00 && b[1] == 0x00 && b[2] == 0x01 && b[3] == 0x00
	}
	isPNG := func(b []byte) bool {
		return len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a})
	}
	isJPG := func(b []byte) bool {
		return len(b) >= 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff
	}
	isGIF := func(b []byte) bool {
		return len(b) >= 6 && (bytes.Equal(b[:6], []byte("GIF87a")) || bytes.Equal(b[:6], []byte("GIF89a")))
	}
	isWEBP := func(b []byte) bool {
		return len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP"))
	}

	if strings.Contains(ct, "image/png") {
		return isPNG(snippet)
	}
	if strings.Contains(ct, "image/jpeg") || strings.Contains(ct, "image/jpg") {
		return isJPG(snippet)
	}
	if strings.Contains(ct, "image/gif") {
		return isGIF(snippet)
	}
	if strings.Contains(ct, "image/webp") {
		return isWEBP(snippet)
	}

	if strings.Contains(ct, "image/x-icon") || strings.Contains(ct, "image/vnd.microsoft.icon") {
		return isICO(snippet)
	}

	if strings.HasPrefix(ct, "image/") {
		return len(snippet) > 0
	}

	if strings.Contains(ct, "application/octet-stream") {
		if isICO(snippet) || isPNG(snippet) || isJPG(snippet) || isGIF(snippet) || isWEBP(snippet) || isSVG(lowerText) {
			return true
		}
		if u, err := url.Parse(target); err == nil {
			p := strings.ToLower(strings.TrimSpace(u.Path))
			if strings.HasSuffix(p, ".svg") {
				return isSVG(lowerText)
			}
			if strings.HasSuffix(p, ".ico") {
				return isICO(snippet)
			}
			if strings.HasSuffix(p, ".png") {
				return isPNG(snippet)
			}
		}
		return false
	}

	return false
}

func hostnameFromAbsoluteURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.TrimSpace(u.Hostname())
}

func originFromAbsoluteURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.TrimSpace(u.Scheme) + "://" + strings.TrimSpace(u.Host)
}

func resolveFaviconIcon(lanUrl string, wanUrl string) string {
	return resolveFaviconIconContext(context.Background(), lanUrl, wanUrl)
}

func resolveFaviconIconContext(parent context.Context, lanUrl string, wanUrl string) string {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
	}()

	type faviconResult struct {
		index int
		icon  string
	}

	originalURLs := []string{strings.TrimSpace(lanUrl), strings.TrimSpace(wanUrl)}
	rawURLs := append([]string{}, originalURLs...)
	if runningInContainer() {
		for _, rawURL := range originalURLs {
			if fallback := dockerHostFallbackURL(rawURL); fallback != "" && fallback != rawURL {
				rawURLs = append(rawURLs, fallback)
			}
		}
	}
	results := make(chan faviconResult, len(rawURLs))
	pending := 0
	for index, rawURL := range rawURLs {
		if originFromAbsoluteURL(rawURL) == "" {
			continue
		}
		pending++
		workers.Add(1)
		go func(index int, rawURL string) {
			defer workers.Done()
			results <- faviconResult{index: index, icon: resolveFaviconFromSiteContext(ctx, rawURL)}
		}(index, rawURL)
	}

	resolved := make([]string, len(rawURLs))
	for i := 0; i < pending; i++ {
		var result faviconResult
		select {
		case result = <-results:
		case <-ctx.Done():
			return ""
		}
		resolved[result.index] = result.icon
	}
	for _, icon := range resolved {
		if icon != "" {
			return icon
		}
	}

	duckResults := make(chan faviconResult, len(originalURLs))
	pending = 0
	for index, rawURL := range originalURLs {
		host := hostnameFromAbsoluteURL(rawURL)
		if host == "" {
			continue
		}
		pending++
		workers.Add(1)
		go func(index int, host string) {
			defer workers.Done()
			candidate := "https://icons.duckduckgo.com/ip3/" + host + ".ico"
			if probeImageContext(ctx, candidate) {
				duckResults <- faviconResult{index: index, icon: candidate}
				return
			}
			duckResults <- faviconResult{index: index}
		}(index, host)
	}
	resolved = make([]string, len(originalURLs))
	for i := 0; i < pending; i++ {
		var result faviconResult
		select {
		case result = <-duckResults:
		case <-ctx.Done():
			return ""
		}
		resolved[result.index] = result.icon
	}
	for _, icon := range resolved {
		if icon != "" {
			return icon
		}
	}
	return ""
}

var faviconLinkTagRe = regexp.MustCompile(`(?is)<link\b[^>]*>`)
var faviconAttrRe = regexp.MustCompile(`(?is)\b([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>` + "`" + `]+))`)

func resolveFaviconFromOrigin(origin string) string {
	return resolveFaviconFromSiteContext(context.Background(), origin)
}

func resolveFaviconFromSite(rawURL string) string {
	return resolveFaviconFromSiteContext(context.Background(), rawURL)
}

func resolveFaviconFromSiteContext(parent context.Context, rawURL string) string {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
	}()

	origin := strings.TrimRight(originFromAbsoluteURL(rawURL), "/")
	if origin == "" {
		return ""
	}

	pageURLs := []string{strings.TrimSpace(rawURL)}
	rootURL := origin + "/"
	if pageURLs[0] == "" || pageURLs[0] == origin {
		pageURLs[0] = rootURL
	} else if pageURLs[0] != rootURL {
		pageURLs = append(pageURLs, rootURL)
	}

	type candidateProbe struct {
		index int
		url   string
	}
	pageResults := make([]chan string, len(pageURLs))
	for index, pageURL := range pageURLs {
		pageResults[index] = make(chan string, 1)
		result := pageResults[index]
		workers.Add(1)
		go func() {
			defer workers.Done()
			result <- resolveFaviconFromHTMLContext(ctx, pageURL)
		}()
	}

	directCandidates := []candidateProbe{
		{index: 0, url: origin + "/favicon.ico"},
		{index: 1, url: origin + "/favicon.png"},
		{index: 2, url: origin + "/favicon.svg"},
	}
	directResults := make(chan candidateProbe, len(directCandidates))
	for _, candidate := range directCandidates {
		candidate := candidate
		workers.Add(1)
		go func() {
			defer workers.Done()
			if !probeImageContext(ctx, candidate.url) {
				candidate.url = ""
			}
			directResults <- candidate
		}()
	}

	// 页面显式声明的图标优先，其次才是约定路径。所有候选并行探测，
	// 避免一个返回 JSON/HTML 的伪 favicon 阻塞真实图标发现。
	for _, result := range pageResults {
		select {
		case icon := <-result:
			if icon != "" {
				return icon
			}
		case <-ctx.Done():
			return ""
		}
	}

	resolved := make([]string, len(directCandidates))
	for range directCandidates {
		var result candidateProbe
		select {
		case result = <-directResults:
		case <-ctx.Done():
			return ""
		}
		resolved[result.index] = result.url
	}
	for _, icon := range resolved {
		if icon != "" {
			return icon
		}
	}
	return ""
}

func resolveFaviconFromHTML(htmlURL string) string {
	return resolveFaviconFromHTMLContext(context.Background(), htmlURL)
}

func resolveFaviconFromHTMLContext(ctx context.Context, htmlURL string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, htmlURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "tradis-discovery/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		_, _ = io.CopyN(io.Discard, resp.Body, 256)
		return ""
	}
	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if !strings.Contains(ct, "text/html") {
		_, _ = io.CopyN(io.Discard, resp.Body, 256)
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	href := extractFaviconHref(body)
	if href == "" {
		return ""
	}
	base, err := url.Parse(htmlURL)
	if err != nil {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	candidate := base.ResolveReference(ref).String()
	if probeImageContext(ctx, candidate) {
		return candidate
	}
	return ""
}

func extractFaviconHref(body []byte) string {
	for _, tag := range faviconLinkTagRe.FindAll(body, 32) {
		attrs := map[string]string{}
		for _, m := range faviconAttrRe.FindAllSubmatch(tag, -1) {
			if len(m) < 5 {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(string(m[1])))
			value := ""
			for _, part := range m[2:] {
				if len(part) > 0 {
					value = strings.TrimSpace(string(part))
					break
				}
			}
			if key != "" {
				attrs[key] = value
			}
		}
		rel := strings.ToLower(attrs["rel"])
		if rel == "" || !strings.Contains(rel, "icon") {
			continue
		}
		if href := strings.TrimSpace(attrs["href"]); href != "" {
			return href
		}
	}
	return ""
}

func normalizeAndResolveNavigationIcon(icon string, lan string, wan string) string {
	return normalizeAndResolveNavigationIconContext(context.Background(), icon, lan, wan)
}

func normalizeAndResolveNavigationIconContext(ctx context.Context, icon string, lan string, wan string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return "mdi-docker"
	}
	out := strings.TrimSpace(icon)
	out = strings.Trim(out, "`")
	if out == "" {
		out = "mdi-docker"
	}
	if strings.HasPrefix(out, "/icons/ray") {
		out = "mdi-docker"
	}
	if !isAllowedNavigationIconValue(out) {
		out = "mdi-docker"
	}
	if strings.HasPrefix(out, "http://") || strings.HasPrefix(out, "https://") {
		if !probeImageContext(ctx, out) {
			out = "mdi-docker"
		}
	}
	if out == "mdi-docker" {
		if candidate := resolveFaviconIconContext(ctx, lan, wan); candidate != "" {
			out = candidate
		}
	}
	return out
}

const maxCachedNavigationIconBytes = 5 << 20

// cacheNavigationIcon 将自动发现的远程图标保存到 Client 数据目录。
// 这样业务容器停止后，导航图标仍由 TRADIS 自身提供。
func cacheNavigationIcon(icon string, sourceKey string) string {
	icon = strings.TrimSpace(icon)
	if !isRemoteNavigationIcon(icon) {
		return icon
	}

	data, contentType, resolvedURL, err := fetchNavigationIcon(icon)
	if err != nil {
		if parsed, parseErr := url.Parse(icon); parseErr == nil && strings.EqualFold(filepath.Ext(parsed.Path), ".svg") {
			return "container"
		}
		return icon
	}
	ext := cachedNavigationIconExtension(contentType, resolvedURL, data)
	if ext == "" {
		return "container"
	}

	picDir := filepath.Join(settings.GetDataDir(), "pic")
	if err := os.MkdirAll(picDir, 0755); err != nil {
		return icon
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(sourceKey)))
	filename := fmt.Sprintf("navigation-auto-%x%s", sum[:12], ext)
	target := filepath.Join(picDir, filename)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return icon
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return icon
	}
	return "/data/pic/" + filename
}

func fetchNavigationIcon(rawURL string) ([]byte, string, string, error) {
	candidates := []string{rawURL}
	if runningInContainer() {
		if fallback := dockerHostFallbackURL(rawURL); fallback != "" && fallback != rawURL {
			candidates = append(candidates, fallback)
		}
	}

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	var lastErr error
	for _, candidate := range candidates {
		req, err := http.NewRequest(http.MethodGet, candidate, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "tradis-discovery/1.0")
		req.Header.Set("Accept", "image/*,application/octet-stream;q=0.9,*/*;q=0.8")
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("unexpected icon status: %s", resp.Status)
			_, _ = io.CopyN(io.Discard, resp.Body, 256)
			resp.Body.Close()
			continue
		}
		if resp.ContentLength > maxCachedNavigationIconBytes {
			resp.Body.Close()
			lastErr = fmt.Errorf("navigation icon exceeds size limit")
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxCachedNavigationIconBytes+1))
		resp.Body.Close()
		if readErr != nil || len(data) == 0 || len(data) > maxCachedNavigationIconBytes {
			lastErr = fmt.Errorf("invalid navigation icon body")
			continue
		}
		return data, resp.Header.Get("Content-Type"), candidate, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no navigation icon URL available")
	}
	return nil, "", "", lastErr
}

func dockerHostFallbackURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
	default:
		return ""
	}
	if port := parsed.Port(); port != "" {
		parsed.Host = net.JoinHostPort("host.docker.internal", port)
	} else {
		parsed.Host = "host.docker.internal"
	}
	return parsed.String()
}

func runningInContainer() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

func cachedNavigationIconExtension(contentType string, rawURL string, data []byte) string {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch ct {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/x-icon", "image/vnd.microsoft.icon":
		return ".ico"
	case "image/svg+xml":
		// 不把远程 SVG 缓存为同源静态资源，避免引入主动内容。
		return ""
	}

	detected := strings.ToLower(http.DetectContentType(data))
	switch {
	case strings.Contains(detected, "image/png"):
		return ".png"
	case strings.Contains(detected, "image/jpeg"):
		return ".jpg"
	case strings.Contains(detected, "image/gif"):
		return ".gif"
	case strings.Contains(detected, "image/webp"):
		return ".webp"
	}
	if len(data) >= 4 && data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x01 && data[3] == 0x00 {
		return ".ico"
	}
	if parsed, err := url.Parse(rawURL); err == nil {
		switch ext := strings.ToLower(filepath.Ext(parsed.Path)); ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico":
			return ext
		}
	}
	return ""
}

func isRemoteNavigationIcon(icon string) bool {
	icon = strings.TrimSpace(icon)
	return strings.HasPrefix(icon, "http://") || strings.HasPrefix(icon, "https://")
}

func isGenericNavigationIcon(icon string) bool {
	switch strings.ToLower(strings.TrimSpace(icon)) {
	case "", "mdi-docker", "mdi:docker", "container", "box":
		return true
	default:
		return false
	}
}

func isAllowedNavigationIconValue(icon string) bool {
	v := strings.TrimSpace(icon)
	if v == "" {
		return false
	}
	if strings.HasPrefix(v, "mdi-") {
		return true
	}
	if strings.HasPrefix(v, "lucide:") {
		return true
	}
	if strings.HasPrefix(v, "/icons/clay/") {
		return true
	}
	if strings.HasPrefix(v, "/data/pic/") || strings.HasPrefix(v, "/uploads/icons/") {
		return true
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return true
	}
	return false
}
