package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// 任务类型常量。
const (
	TaskContainerStart   = "container_start"
	TaskContainerStop    = "container_stop"
	TaskContainerRestart = "container_restart"
	TaskImageUpdateCheck = "image_update_check"
	TaskBuildCachePrune  = "build_cache_prune"
)

// imageUpdateChecker 由 api 包通过 RegisterImageUpdateChecker 注入，
// 避免scheduler 包反向依赖 api 包（会造成循环依赖）。
var imageUpdateChecker func(ctx context.Context, force bool) error

// containerActionGuard is supplied by the API layer so scheduled container
// actions share the same self-management protection as interactive actions.
var containerActionGuard func(ctx context.Context, containerID string) error

// RegisterImageUpdateChecker 由 api 包在 init 阶段调用，注入 runImageUpdateCheck 的包装。
func RegisterImageUpdateChecker(fn func(ctx context.Context, force bool) error) {
	imageUpdateChecker = fn
}

func RegisterContainerActionGuard(fn func(ctx context.Context, containerID string) error) {
	containerActionGuard = fn
}

// executeJob 执行单个 job，写 task 记录，回填运行时字段。
// manual=true 表示手动触发（不受 enabled 限制，不影响防重入外的下次调度）。
func executeJob(job database.ScheduledJob, manual bool) error {
	if !manual && !job.Enabled {
		return nil
	}

	// 防 per-job 重入
	Default.mu.Lock()
	if Default.running[job.ID] {
		Default.mu.Unlock()
		log.Printf("[scheduler] job %d (%s) 仍在运行，跳过本次触发", job.ID, job.Name)
		return nil
	}
	Default.running[job.ID] = true
	Default.mu.Unlock()
	defer func() {
		Default.mu.Lock()
		delete(Default.running, job.ID)
		Default.mu.Unlock()
	}()

	taskID := fmt.Sprintf("scheduled-%d-%d", job.ID, time.Now().UnixNano())
	now := time.Now()
	status := "running"

	_ = database.UpsertTask(taskID, scheduledTaskType(job.ID), status)
	_ = database.UpdateScheduledJobRunResult(job.ID, now, nil, taskID, status)
	var seq int64
	seq++
	_ = database.AppendTaskLogWithSeq(taskID, seq, time.Now(), "info", jobStartMessage(job, manual))

	var jobErr error
	defer func() {
		endStatus := "success"
		if jobErr != nil {
			endStatus = "failed"
		}
		if r := recover(); r != nil {
			endStatus = "failed"
			jobErr = fmt.Errorf("panic: %v", r)
			log.Printf("[scheduler] job %d panic: %v", job.ID, r)
		}
		seq++
		_ = database.AppendTaskLogWithSeq(taskID, seq, time.Now(), endStatusLabel(endStatus), endMessage(job, jobErr))
		_ = database.FinishTask(taskID, endStatus, nil, errString(jobErr))
		nextRun := Default.NextRun(job.ID)
		_ = database.UpdateScheduledJobRunResult(job.ID, now, nextRun, taskID, endStatus)
		if jobErr != nil {
			_ = database.SaveNotification(&database.Notification{
				Type: "error", Category: "system",
				Message: fmt.Sprintf("定时任务「%s」执行失败: %v", job.Name, jobErr),
				Read:    false,
			})
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), scheduledTaskTimeout(job))
	defer cancel()

	jobErr = dispatch(ctx, job, taskID, &seq)
	return jobErr
}

// dispatch 按 task_type 分发到具体动作。
func dispatch(ctx context.Context, job database.ScheduledJob, taskID string, seq *int64) error {
	switch job.TaskType {
	case TaskContainerStart, TaskContainerStop, TaskContainerRestart:
		return runContainerAction(ctx, job, taskID, seq)
	case TaskImageUpdateCheck:
		return runImageUpdateCheckAction(ctx, job, taskID, seq)
	case TaskBuildCachePrune:
		return runBuildCachePrune(ctx, job, taskID, seq)
	default:
		return dispatchEditionJob(ctx, job, taskID, seq)
	}
}

func runContainerAction(ctx context.Context, job database.ScheduledJob, taskID string, seq *int64) error {
	targets := splitTargets(job.Target)
	if len(targets) == 0 {
		return fmt.Errorf("未指定目标容器")
	}
	if containerActionGuard != nil {
		for _, id := range targets {
			if err := containerActionGuard(ctx, id); err != nil {
				return fmt.Errorf("禁止操作目标容器 %s: %w", shortID(id), err)
			}
		}
	}
	cli, err := docker.NewDockerClient()
	if err != nil {
		return fmt.Errorf("创建 docker 客户端失败: %w", err)
	}
	defer cli.Close()

	var errs []string
	succeeded := 0
	for _, id := range targets {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		var actionErr error
		switch job.TaskType {
		case TaskContainerStart:
			actionErr = cli.ContainerStart(ctx, id, types.ContainerStartOptions{})
		case TaskContainerStop:
			timeout := 10
			actionErr = cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
		case TaskContainerRestart:
			timeout := 10
			actionErr = cli.ContainerRestart(ctx, id, container.StopOptions{Timeout: &timeout})
		}
		if actionErr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", shortID(id), actionErr))
			*seq++
			_ = database.AppendTaskLogWithSeq(taskID, *seq, time.Now(), "error", fmt.Sprintf("[%s] 失败: %v", shortID(id), actionErr))
		} else {
			succeeded++
			*seq++
			_ = database.AppendTaskLogWithSeq(taskID, *seq, time.Now(), "success", fmt.Sprintf("[%s] %s 成功", shortID(id), job.TaskType))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d/%d 失败: %s", len(errs), len(targets), strings.Join(errs, "; "))
	}
	*seq++
	_ = database.AppendTaskLogWithSeq(taskID, *seq, time.Now(), "info", fmt.Sprintf("全部 %d 个容器操作成功", succeeded))
	return nil
}

func runImageUpdateCheckAction(ctx context.Context, job database.ScheduledJob, taskID string, seq *int64) error {
	if imageUpdateChecker == nil {
		return fmt.Errorf("镜像更新检测器未注册")
	}
	*seq++
	_ = database.AppendTaskLogWithSeq(taskID, *seq, time.Now(), "info", "开始检测镜像更新")
	if err := imageUpdateChecker(ctx, true); err != nil {
		return fmt.Errorf("镜像更新检测失败: %w", err)
	}
	*seq++
	_ = database.AppendTaskLogWithSeq(taskID, *seq, time.Now(), "success", "镜像更新检测完成")
	return nil
}

// buildCachePrunePayload 是 build_cache_prune 任务的 payload 结构。
type buildCachePrunePayload struct {
	All         bool  `json:"all"`
	KeepStorage int64 `json:"keep_storage"`
}

func runBuildCachePrune(ctx context.Context, job database.ScheduledJob, taskID string, seq *int64) error {
	var payload buildCachePrunePayload
	if job.PayloadJSON != "" {
		if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
			return fmt.Errorf("解析 payload 失败: %w", err)
		}
	}
	cli, err := docker.NewDockerClient()
	if err != nil {
		return fmt.Errorf("创建 docker 客户端失败: %w", err)
	}
	defer cli.Close()

	*seq++
	_ = database.AppendTaskLogWithSeq(taskID, *seq, time.Now(), "info", fmt.Sprintf("开始清理构建缓存 (all=%v)", payload.All))
	report, err := cli.BuildCachePrune(ctx, types.BuildCachePruneOptions{
		All:         payload.All,
		KeepStorage: payload.KeepStorage,
	})
	if err != nil {
		return fmt.Errorf("清理构建缓存失败: %w", err)
	}
	*seq++
	_ = database.AppendTaskLogWithSeq(taskID, *seq, time.Now(), "success",
		fmt.Sprintf("已清理 %d 个缓存，释放 %s", len(report.CachesDeleted), formatBytes(report.SpaceReclaimed)))
	return nil
}

// --- helpers ---

func splitTargets(target string) []string {
	if target == "" {
		return nil
	}
	parts := strings.Split(target, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func scheduledTaskType(jobID int64) string {
	return fmt.Sprintf("scheduled_job_%d", jobID)
}

func jobStartMessage(job database.ScheduledJob, manual bool) string {
	prefix := "自动触发"
	if manual {
		prefix = "手动触发"
	}
	return fmt.Sprintf("%s定时任务: %s [%s]", prefix, job.Name, job.TaskType)
}

func endStatusLabel(status string) string {
	if status == "success" {
		return "success"
	}
	return "error"
}

func endMessage(job database.ScheduledJob, err error) string {
	if err != nil {
		return fmt.Sprintf("任务失败: %v", err)
	}
	return fmt.Sprintf("任务完成: %s", job.Name)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
