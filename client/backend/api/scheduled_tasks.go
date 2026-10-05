package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/scheduler"
	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
)

var scheduledTaskDatabaseUpdate = database.UpdateScheduledJobInEnvironment
var scheduledTaskRuntimeUpdate = func(job database.ScheduledJob) error {
	return scheduler.Default.UpdateJob(job)
}
var registerScheduledImageUpdateChecker = scheduler.RegisterImageUpdateChecker
var registerScheduledContainerActionGuard = scheduler.RegisterContainerActionGuard

// RegisterScheduledTaskRoutes 注册定时任务系统的路由。
func RegisterScheduledTaskRoutes(r *gin.RouterGroup) {
	group := r.Group("/scheduled-tasks")
	{
		group.GET("", listScheduledTasks)
		group.POST("", createScheduledTask)
		group.GET("/cron-preview", nextRunPreview)
		group.PUT("/:id", updateScheduledTask)
		group.DELETE("/:id", deleteScheduledTask)
		group.POST("/:id/run", runScheduledTaskNow)
		group.GET("/:id/runs", listScheduledTaskRuns)
	}
}

// RegisterCommunitySchedulerCallbacks installs callbacks needed by the local
// scheduler. It deliberately leaves application protection unregistered.
func RegisterCommunitySchedulerCallbacks() {
	registerScheduledImageUpdateChecker(func(ctx context.Context, force bool) error {
		_, err := runImageUpdateCheck(ctx, force)
		return err
	})
	registerScheduledContainerActionGuard(validateContainerManagementTarget)
}

// scheduledTaskInput 创建/更新请求体。
type scheduledTaskInput struct {
	Name        string `json:"name"`
	TaskType    string `json:"task_type"`
	CronExpr    string `json:"cron_expr"`
	Target      string `json:"target"`
	PayloadJSON string `json:"payload_json"`
	Enabled     *bool  `json:"enabled"` // 指针：区分"未传"和"传 false"
}

func validTaskType(t string) bool {
	return isScheduledTaskTypeAvailable(t)
}

// validateCronExpr 用 robfig/cron 解析表达式，返回是否合法。
func validateCronExpr(expr string) bool {
	p := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err := p.Parse(expr); err != nil {
		return false
	}
	return true
}

func (in *scheduledTaskInput) validate(isCreate bool) (string, bool) {
	in.Name = strings.TrimSpace(in.Name)
	in.TaskType = strings.TrimSpace(in.TaskType)
	in.CronExpr = strings.TrimSpace(in.CronExpr)
	in.Target = strings.TrimSpace(in.Target)

	if in.Name == "" {
		return "任务名称不能为空", false
	}
	if in.TaskType != "" && !validTaskType(in.TaskType) {
		return "任务类型无效", false
	}
	if isCreate && in.TaskType == "" {
		return "任务类型无效", false
	}
	if message, ok := validateScheduledTaskEditionInput(*in, isCreate); !ok {
		return message, false
	}
	// 容器操作必须有目标
	if (in.TaskType == scheduler.TaskContainerStart || in.TaskType == scheduler.TaskContainerStop || in.TaskType == scheduler.TaskContainerRestart) && in.Target == "" {
		return "容器操作必须指定目标容器", false
	}
	if !validateCronExpr(in.CronExpr) {
		return "cron 表达式无效（支持 5 字段或 @daily/@hourly 等）", false
	}
	return "", true
}

func listScheduledTasks(c *gin.Context) {
	environmentID, ok := localEnvironmentScope(c)
	if !ok {
		return
	}
	jobs, err := database.ListScheduledJobsInEnvironment(environmentID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取定时任务列表失败", err)
		return
	}
	visible := jobs[:0]
	for _, job := range jobs {
		if isScheduledTaskTypeAvailable(job.TaskType) {
			visible = append(visible, job)
		}
	}
	c.JSON(http.StatusOK, gin.H{"jobs": visible})
}

func createScheduledTask(c *gin.Context) {
	environmentID, ok := localEnvironmentScope(c)
	if !ok {
		return
	}
	var in scheduledTaskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		respondError(c, http.StatusBadRequest, "请求参数无效", err)
		return
	}
	if msg, ok := in.validate(true); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}

	job := database.ScheduledJob{
		Name:        in.Name,
		TaskType:    in.TaskType,
		CronExpr:    in.CronExpr,
		Target:      in.Target,
		PayloadJSON: in.PayloadJSON,
		Enabled:     true,
	}
	if in.Enabled != nil {
		job.Enabled = *in.Enabled
	}
	if err := database.CreateScheduledJobInEnvironment(environmentID, &job); err != nil {
		respondError(c, http.StatusInternalServerError, "创建定时任务失败", err)
		return
	}
	if err := scheduler.Default.AddJob(job); err != nil {
		// DB 创建成功但 cron 注册失败（表达式理论上已校验过，此处兜底）
		_ = database.DeleteScheduledJobInEnvironment(environmentID, job.ID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "注册到调度器失败: " + err.Error()})
		return
	}
	// 回填 next_run_at
	_ = database.GetScheduledJobInEnvironment(environmentID, job.ID, &job)
	c.JSON(http.StatusOK, gin.H{"job": job})
}

func updateScheduledTask(c *gin.Context) {
	environmentID, ok := localEnvironmentScope(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}
	var in scheduledTaskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		respondError(c, http.StatusBadRequest, "请求参数无效", err)
		return
	}
	if msg, ok := in.validate(false); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}

	var existing database.ScheduledJob
	if err := database.GetScheduledJobInEnvironment(environmentID, id, &existing); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if status, err := validateScheduledTaskEditionUpdate(existing, in); err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	previous := existing
	existing.Name = in.Name
	existing.CronExpr = in.CronExpr
	existing.Target = scheduledTaskTarget(existing, in.Target)
	existing.PayloadJSON = in.PayloadJSON
	if in.TaskType != "" {
		existing.TaskType = in.TaskType
	}
	if in.Enabled != nil {
		existing.Enabled = *in.Enabled
	}
	if err := applyScheduledTaskUpdate(previous, existing); err != nil {
		respondError(c, http.StatusInternalServerError, "更新定时任务失败", err)
		return
	}
	_ = database.GetScheduledJobInEnvironment(environmentID, id, &existing)
	c.JSON(http.StatusOK, gin.H{"job": existing})
}

func applyScheduledTaskUpdate(previous, updated database.ScheduledJob) error {
	environmentID := strings.TrimSpace(updated.EnvironmentID)
	if environmentID == "" {
		environmentID = database.LocalEnvironmentID
	}
	if err := scheduledTaskDatabaseUpdate(environmentID, &updated); err != nil {
		return err
	}
	if err := scheduledTaskRuntimeUpdate(updated); err != nil {
		rollbackDBErr := scheduledTaskDatabaseUpdate(environmentID, &previous)
		rollbackRuntimeErr := scheduledTaskRuntimeUpdate(previous)
		return errors.Join(err, rollbackDBErr, rollbackRuntimeErr)
	}
	if err := applyScheduledTaskEditionUpdate(updated); err != nil {
		rollbackDBErr := scheduledTaskDatabaseUpdate(environmentID, &previous)
		rollbackRuntimeErr := scheduledTaskRuntimeUpdate(previous)
		return errors.Join(err, rollbackDBErr, rollbackRuntimeErr)
	}
	return nil
}

func deleteScheduledTask(c *gin.Context) {
	environmentID, ok := localEnvironmentScope(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}
	var job database.ScheduledJob
	if err := database.GetScheduledJobInEnvironment(environmentID, id, &job); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	scheduler.Default.RemoveJob(id)
	if err := database.DeleteScheduledJobInEnvironment(environmentID, id); err != nil {
		respondError(c, http.StatusInternalServerError, "删除定时任务失败", err)
		return
	}
	if err := afterScheduledTaskDelete(job); err != nil {
		respondError(c, http.StatusInternalServerError, "同步应用保护计划失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已删除"})
}

func runScheduledTaskNow(c *gin.Context) {
	environmentID, ok := localEnvironmentScope(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}
	var job database.ScheduledJob
	if err := database.GetScheduledJobInEnvironment(environmentID, id, &job); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if !isScheduledTaskTypeAvailable(job.TaskType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "当前版本不支持该定时任务类型"})
		return
	}
	if err := scheduler.Default.RunNow(job); err != nil {
		respondError(c, http.StatusInternalServerError, "触发任务失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已触发"})
}

func listScheduledTaskRuns(c *gin.Context) {
	environmentID, ok := localEnvironmentScope(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}
	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	runs, err := database.ListScheduledJobRunsInEnvironment(environmentID, id, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取执行历史失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs})
}

// nextRunPreview 预览 cron 表达式的下次触发时间（给前端创建表单做实时校验用）。
func nextRunPreview(c *gin.Context) {
	expr := strings.TrimSpace(c.Query("expr"))
	if expr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 expr 参数"})
		return
	}
	if !validateCronExpr(expr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "表达式无效", "valid": false})
		return
	}
	p := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	sched, err := p.Parse(expr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "valid": false})
		return
	}
	next := sched.Next(time.Now())
	c.JSON(http.StatusOK, gin.H{"valid": true, "next_run": next.Format(time.RFC3339)})
}
