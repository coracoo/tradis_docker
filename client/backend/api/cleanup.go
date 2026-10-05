package api

import (
	"context"
	"database/sql"
	"dockerpanel/backend/pkg/cleanup"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/logging"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/errdefs"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
)

const (
	cleanupTaskType           = "docker_cleanup"
	cleanupEvaluationCacheTTL = 30 * time.Second
	cleanupVolumeSizeTimeout  = 10 * time.Second
)

var (
	cleanupTaskStartMu       sync.Mutex
	errCleanupTaskActive     = errors.New("已有空间清理任务正在运行")
	errCleanupCandidateStale = errors.New("资源已不再符合清理条件")
	cleanupEvaluationFlights singleflight.Group
	cleanupEvaluationCache   struct {
		sync.RWMutex
		value     cleanup.Evaluation
		expiresAt time.Time
		valid     bool
	}
)

type cleanupDockerClient interface {
	DiskUsage(context.Context, types.DiskUsageOptions) (types.DiskUsage, error)
	NetworkList(context.Context, types.NetworkListOptions) ([]types.NetworkResource, error)
	ContainerRemove(context.Context, string, types.ContainerRemoveOptions) error
	ImageRemove(context.Context, string, types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error)
	VolumeRemove(context.Context, string, bool) error
	NetworkRemove(context.Context, string) error
	BuildCachePrune(context.Context, types.BuildCachePruneOptions) (*types.BuildCachePruneReport, error)
	VolumeList(context.Context, volume.ListOptions) (volume.ListResponse, error)
	Close() error
}

var cleanupDockerClientFactory = func() (cleanupDockerClient, error) {
	return docker.NewDockerClient()
}

var cleanupProtectedIdentity = func() (string, string) {
	id, name, _, _ := getSelfIdentity()
	return id, name
}

type cleanupTaskRequest struct {
	Items           []cleanup.Selection `json:"items"`
	ConfirmHighRisk bool                `json:"confirmHighRisk"`
}

func RegisterCleanupRoutes(group *gin.RouterGroup) {
	routes := group.Group("/cleanup")
	routes.GET("/evaluation", getCleanupEvaluation)
	routes.GET("/tasks", listCleanupTasks)
	routes.POST("/tasks", startCleanupTask)
	routes.GET("/tasks/:id", getCleanupTask)
	routes.GET("/tasks/:id/events", cleanupTaskEvents)
}

func getCleanupEvaluation(c *gin.Context) {
	if _, ok := localEnvironmentScope(c); !ok {
		return
	}
	evaluation, err := cachedCleanupEvaluation(c.Request.Context(), c.Query("refresh") == "true")
	if err != nil {
		respondError(c, cleanupEvaluationHTTPStatus(err), cleanupEvaluationErrorMessage(err), err)
		return
	}
	c.JSON(http.StatusOK, evaluation)
}

func listCleanupTasks(c *gin.Context) {
	environmentID, ok := localEnvironmentScope(c)
	if !ok {
		return
	}
	statuses := splitCleanupQuery(c.Query("statuses"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	tasks, err := database.ListTasksInEnvironment(environmentID, []string{cleanupTaskType}, statuses, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "读取空间清理任务失败", err)
		return
	}
	c.JSON(http.StatusOK, tasks)
}

func getCleanupTask(c *gin.Context) {
	environmentID, ok := localEnvironmentScope(c)
	if !ok {
		return
	}
	task, err := database.GetTaskInEnvironment(environmentID, strings.TrimSpace(c.Param("id")))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondError(c, http.StatusNotFound, "空间清理任务不存在", nil)
			return
		}
		respondError(c, http.StatusInternalServerError, "读取空间清理任务失败", err)
		return
	}
	if task.Type != cleanupTaskType {
		respondError(c, http.StatusNotFound, "空间清理任务不存在", nil)
		return
	}
	c.JSON(http.StatusOK, task)
}

func startCleanupTask(c *gin.Context) {
	if _, ok := localEnvironmentScope(c); !ok {
		return
	}
	var request cleanupTaskRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "清理请求格式不正确", err)
		return
	}
	request.Items = normalizeCleanupSelections(request.Items)
	if len(request.Items) == 0 {
		respondError(c, http.StatusBadRequest, "请至少选择一个清理项目", nil)
		return
	}
	if len(request.Items) > 500 {
		respondError(c, http.StatusBadRequest, "单次最多清理 500 个项目", nil)
		return
	}
	if containsHighRiskCleanup(request.Items) && !request.ConfirmHighRisk {
		respondError(c, http.StatusBadRequest, "清理数据卷需要再次确认", nil)
		return
	}
	if active, err := hasActiveCleanupTask(); err != nil {
		respondError(c, http.StatusInternalServerError, "检查空间清理任务失败", err)
		return
	} else if active {
		respondError(c, http.StatusConflict, errCleanupTaskActive.Error(), nil)
		return
	}
	cli, err := cleanupDockerClientFactory()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "连接 Docker 失败", err)
		return
	}
	ctx, cancel := docker.WithCleanupTimeoutFrom(c.Request.Context())
	evaluation, evaluationErr := collectCleanupEvaluation(ctx, cli)
	cancel()
	_ = cli.Close()
	if evaluationErr != nil {
		respondError(c, http.StatusInternalServerError, "验证空间清理候选失败", evaluationErr)
		return
	}
	valid, skipped := validateCleanupSelections(request.Items, evaluation)
	if len(skipped) > 0 || len(valid) != len(request.Items) {
		respondError(c, http.StatusBadRequest, "部分清理项目已失效，请重新评估", nil)
		return
	}

	taskID := fmt.Sprintf("cleanup-%d", time.Now().UnixNano())
	if err := reserveCleanupTask(taskID); errors.Is(err, errCleanupTaskActive) {
		respondError(c, http.StatusConflict, errCleanupTaskActive.Error(), nil)
		return
	} else if err != nil {
		respondError(c, http.StatusInternalServerError, "创建空间清理任务失败", err)
		return
	}
	go runCleanupTask(taskID, request)
	c.JSON(http.StatusAccepted, gin.H{"taskId": taskID, "status": "pending"})
}

func cleanupTaskEvents(c *gin.Context) {
	streamDatabaseTaskEvents(c, strings.TrimSpace(c.Param("id")))
}

func runCleanupTask(taskID string, request cleanupTaskRequest) {
	ctx, cancel := docker.WithLongTimeout()
	defer cancel()
	seq := int64(0)
	appendLog := func(kind, message string) {
		seq++
		_ = database.AppendTaskLogWithSeq(taskID, seq, time.Now(), kind, message)
	}
	finish := func(status string, result cleanup.TaskResult, errText string) {
		_ = database.FinishTask(taskID, status, result, errText)
		notificationType := "success"
		if status != "success" || len(result.Failed) > 0 {
			notificationType = "error"
		}
		message := fmt.Sprintf("空间清理完成：删除 %d 项，跳过 %d 项，失败 %d 项", len(result.Deleted), len(result.Skipped), len(result.Failed))
		if status != "success" && strings.TrimSpace(errText) != "" {
			message = "空间清理失败：" + errText
		}
		_ = database.SaveNotification(&database.Notification{
			Type: notificationType, EventType: cleanupTaskType, Category: "system", Message: message,
		})
	}

	_ = database.UpsertTask(taskID, cleanupTaskType, "running")
	appendLog("info", "正在重新评估清理候选")
	cli, err := cleanupDockerClientFactory()
	if err != nil {
		appendLog("error", "连接 Docker 失败")
		finish("error", emptyCleanupResult(), err.Error())
		return
	}
	defer cli.Close()

	evaluation, err := collectCleanupEvaluation(ctx, cli)
	if err != nil {
		appendLog("error", "重新评估失败："+err.Error())
		finish("error", emptyCleanupResult(), err.Error())
		return
	}
	valid, skippedSelections := validateCleanupSelections(request.Items, evaluation)
	result := executeCleanupItems(ctx, cli, valid, appendLog)
	invalidateCleanupEvaluationCache()
	for _, selection := range skippedSelections {
		result.Skipped = append(result.Skipped, cleanup.Outcome{Selection: selection, Message: "资源已被使用或不再符合清理条件"})
	}

	status := "success"
	errText := ""
	if len(result.Failed) > 0 {
		status = "error"
		if len(result.Deleted) == 0 {
			errText = "所选清理项目均执行失败"
		} else {
			errText = fmt.Sprintf("部分清理项目执行失败：成功 %d 项，失败 %d 项", len(result.Deleted), len(result.Failed))
		}
	}
	finish(status, result, errText)
}

func collectCleanupEvaluation(ctx context.Context, cli cleanupDockerClient) (cleanup.Evaluation, error) {
	usage, err := cli.DiskUsage(ctx, types.DiskUsageOptions{Types: []types.DiskUsageObject{
		types.ContainerObject,
		types.ImageObject,
		types.BuildCacheObject,
	}})
	if err != nil {
		if !dockerDiskUsageCompatibilityError(err) {
			return cleanup.Evaluation{}, err
		}
		usage, err = cli.DiskUsage(ctx, types.DiskUsageOptions{})
		if err != nil {
			return cleanup.Evaluation{}, err
		}
	} else {
		volumeCtx, cancel := context.WithTimeout(ctx, cleanupVolumeSizeTimeout)
		volumeUsage, volumeErr := cli.DiskUsage(volumeCtx, types.DiskUsageOptions{Types: []types.DiskUsageObject{types.VolumeObject}})
		cancel()
		if volumeErr == nil {
			usage.Volumes = volumeUsage.Volumes
		} else {
			if ctx.Err() != nil {
				return cleanup.Evaluation{}, ctx.Err()
			}
			listed, listErr := cli.VolumeList(ctx, volume.ListOptions{
				Filters: filters.NewArgs(filters.Arg("dangling", "true")),
			})
			if listErr != nil {
				return cleanup.Evaluation{}, fmt.Errorf("读取数据卷列表失败: %w", listErr)
			}
			usage.Volumes = listed.Volumes
			logging.Warn("Docker volume size scan incomplete; using metadata-only fallback", "error", volumeErr)
		}
	}
	networks, err := cli.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return cleanup.Evaluation{}, err
	}
	protectedIDs := make(map[string]struct{})
	protectedNames := make(map[string]struct{})
	selfID, selfName := cleanupProtectedIdentity()
	if selfID = strings.TrimSpace(selfID); selfID != "" {
		protectedIDs[selfID] = struct{}{}
	}
	if selfName = strings.TrimPrefix(strings.TrimSpace(selfName), "/"); selfName != "" {
		protectedNames[selfName] = struct{}{}
	}
	for _, item := range usage.Containers {
		if item == nil {
			continue
		}
		name := cleanupContainerName(*item)
		if isSelfOrProtectedContainer(item.ID, name, item.Image, item.Labels) {
			protectedIDs[item.ID] = struct{}{}
			if name != "" {
				protectedNames[name] = struct{}{}
			}
		}
	}
	return cleanup.Evaluate(cleanup.Snapshot{
		EvaluatedAt: time.Now(), DiskUsage: usage, Networks: networks,
		ProtectedContainerIDs: protectedIDs, ProtectedContainerNames: protectedNames,
	}), nil
}

func cachedCleanupEvaluation(ctx context.Context, force bool) (cleanup.Evaluation, error) {
	if !force {
		if evaluation, ok := readCleanupEvaluationCache(time.Now()); ok {
			return evaluation, nil
		}
	}
	result := cleanupEvaluationFlights.DoChan("local", func() (any, error) {
		if !force {
			if evaluation, ok := readCleanupEvaluationCache(time.Now()); ok {
				return evaluation, nil
			}
		}
		scanCtx, cancel := docker.WithCleanupTimeoutFrom(context.Background())
		defer cancel()
		cli, err := cleanupDockerClientFactory()
		if err != nil {
			return cleanup.Evaluation{}, fmt.Errorf("连接 Docker 失败: %w", err)
		}
		defer cli.Close()
		evaluation, err := collectCleanupEvaluation(scanCtx, cli)
		if err != nil {
			return cleanup.Evaluation{}, err
		}
		writeCleanupEvaluationCache(evaluation, time.Now().Add(cleanupEvaluationCacheTTL))
		return evaluation, nil
	})
	select {
	case <-ctx.Done():
		return cleanup.Evaluation{}, ctx.Err()
	case call := <-result:
		if call.Err != nil {
			return cleanup.Evaluation{}, call.Err
		}
		return call.Val.(cleanup.Evaluation), nil
	}
}

func readCleanupEvaluationCache(now time.Time) (cleanup.Evaluation, bool) {
	cleanupEvaluationCache.RLock()
	defer cleanupEvaluationCache.RUnlock()
	return cleanupEvaluationCache.value, cleanupEvaluationCache.valid && now.Before(cleanupEvaluationCache.expiresAt)
}

func writeCleanupEvaluationCache(evaluation cleanup.Evaluation, expiresAt time.Time) {
	cleanupEvaluationCache.Lock()
	cleanupEvaluationCache.value = evaluation
	cleanupEvaluationCache.expiresAt = expiresAt
	cleanupEvaluationCache.valid = true
	cleanupEvaluationCache.Unlock()
}

func invalidateCleanupEvaluationCache() {
	cleanupEvaluationCache.Lock()
	cleanupEvaluationCache.value = cleanup.Evaluation{}
	cleanupEvaluationCache.expiresAt = time.Time{}
	cleanupEvaluationCache.valid = false
	cleanupEvaluationCache.Unlock()
}

func cleanupEvaluationHTTPStatus(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	return http.StatusInternalServerError
}

func cleanupEvaluationErrorMessage(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "Docker 空间统计超时，请稍后重试"
	}
	return "评估 Docker 可清理空间失败"
}

func validateCleanupSelections(selections []cleanup.Selection, evaluation cleanup.Evaluation) ([]cleanup.Item, []cleanup.Selection) {
	candidates := make(map[string]cleanup.Item)
	for _, category := range evaluation.Categories {
		for _, item := range category.Items {
			candidates[item.Key] = item
		}
	}
	valid := make([]cleanup.Item, 0, len(selections))
	skipped := make([]cleanup.Selection, 0)
	for _, selection := range normalizeCleanupSelections(selections) {
		key := string(selection.Category) + ":" + selection.ID
		if item, ok := candidates[key]; ok {
			valid = append(valid, item)
		} else {
			skipped = append(skipped, selection)
		}
	}
	return valid, skipped
}

func executeCleanupItems(ctx context.Context, cli cleanupDockerClient, items []cleanup.Item, appendLog func(string, string)) cleanup.TaskResult {
	result := emptyCleanupResult()
	for _, item := range items {
		outcome := cleanup.Outcome{Selection: cleanup.Selection{Category: item.Category, ID: item.ID}, Name: item.Name}
		reclaimed, err := executeCleanupItem(ctx, cli, item)
		if err != nil {
			outcome.Message = err.Error()
			if errors.Is(err, errCleanupCandidateStale) || errdefs.IsConflict(err) || errdefs.IsNotFound(err) {
				result.Skipped = append(result.Skipped, outcome)
				cleanupLog(appendLog, "warning", fmt.Sprintf("已跳过 %s：%s", item.Name, err.Error()))
			} else {
				result.Failed = append(result.Failed, outcome)
				cleanupLog(appendLog, "error", fmt.Sprintf("清理 %s 失败：%s", item.Name, err.Error()))
			}
			continue
		}
		result.Deleted = append(result.Deleted, outcome)
		if reclaimed >= 0 {
			result.SpaceReclaimed += reclaimed
		} else if item.SizeState == cleanup.SizeKnown {
			result.SpaceReclaimed += item.SizeBytes
			result.SpaceReclaimedEstimated = true
		}
		cleanupLog(appendLog, "success", "已清理 "+item.Name)
	}
	return result
}

func executeCleanupItem(ctx context.Context, cli cleanupDockerClient, item cleanup.Item) (int64, error) {
	switch item.Category {
	case cleanup.CategoryBuildCache:
		report, err := cli.BuildCachePrune(ctx, types.BuildCachePruneOptions{All: false})
		if err != nil {
			return 0, err
		}
		if len(report.CachesDeleted) == 0 {
			return 0, errCleanupCandidateStale
		}
		return int64(report.SpaceReclaimed), nil
	case cleanup.CategoryDanglingImage, cleanup.CategoryUnusedImage:
		_, err := cli.ImageRemove(ctx, item.ID, types.ImageRemoveOptions{Force: false, PruneChildren: false})
		return -1, err
	case cleanup.CategoryStoppedContainer:
		return -1, cli.ContainerRemove(ctx, item.ID, types.ContainerRemoveOptions{Force: false, RemoveVolumes: false})
	case cleanup.CategoryUnusedNetwork:
		return 0, cli.NetworkRemove(ctx, item.ID)
	case cleanup.CategoryUnusedVolume:
		return -1, cli.VolumeRemove(ctx, item.ID, false)
	default:
		return 0, fmt.Errorf("不支持的清理类别 %q", item.Category)
	}
}

func normalizeCleanupSelections(items []cleanup.Selection) []cleanup.Selection {
	seen := make(map[string]struct{})
	result := make([]cleanup.Selection, 0, len(items))
	for _, item := range items {
		item.Category = cleanup.Category(strings.TrimSpace(string(item.Category)))
		item.ID = strings.TrimSpace(item.ID)
		if !validCleanupCategory(item.Category) || item.ID == "" {
			continue
		}
		key := string(item.Category) + ":" + item.ID
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func validCleanupCategory(category cleanup.Category) bool {
	switch category {
	case cleanup.CategoryBuildCache, cleanup.CategoryDanglingImage, cleanup.CategoryUnusedImage,
		cleanup.CategoryStoppedContainer, cleanup.CategoryUnusedNetwork, cleanup.CategoryUnusedVolume:
		return true
	default:
		return false
	}
}

func containsHighRiskCleanup(items []cleanup.Selection) bool {
	for _, item := range items {
		if item.Category == cleanup.CategoryUnusedVolume {
			return true
		}
	}
	return false
}

func emptyCleanupResult() cleanup.TaskResult {
	return cleanup.TaskResult{Deleted: []cleanup.Outcome{}, Skipped: []cleanup.Outcome{}, Failed: []cleanup.Outcome{}}
}

func cleanupContainerName(item types.Container) string {
	for _, name := range item.Names {
		if name = strings.TrimPrefix(strings.TrimSpace(name), "/"); name != "" {
			return name
		}
	}
	return ""
}

func cleanupLog(appendLog func(string, string), kind, message string) {
	if appendLog != nil {
		appendLog(kind, message)
	}
}

func splitCleanupQuery(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func reserveCleanupTask(taskID string) error {
	cleanupTaskStartMu.Lock()
	defer cleanupTaskStartMu.Unlock()
	active, err := hasActiveCleanupTask()
	if err != nil {
		return err
	}
	if active {
		return errCleanupTaskActive
	}
	return database.UpsertTask(taskID, cleanupTaskType, "pending")
}

func hasActiveCleanupTask() (bool, error) {
	active, err := database.ListTasks([]string{cleanupTaskType}, []string{"pending", "running"}, 1)
	if err != nil {
		return false, err
	}
	return len(active) > 0, nil
}

func RecoverCleanupTasks() {
	tasks, err := database.ListTasks([]string{cleanupTaskType}, []string{"pending", "running"}, 200)
	if err != nil {
		return
	}
	for _, task := range tasks {
		_ = database.FinishTask(task.ID, "error", nil, "服务重启，空间清理任务已中断；为避免重复删除，任务不会自动恢复")
	}
}
