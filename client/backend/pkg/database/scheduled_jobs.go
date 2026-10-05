package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ScheduledJob 表示一条定时任务调度规则。
// 与 tasks 表的区别：tasks 记录"一次执行"，ScheduledJob 记录"调度规则"，
// 二者通过 LastTaskID 软关联。
type ScheduledJob struct {
	ID            int64
	EnvironmentID string `json:"environmentId"`
	Name          string
	TaskType      string // container_start / container_stop / container_restart / image_update_check / build_cache_prune
	CronExpr      string // robfig/cron 表达式
	Target        string // 目标（如容器 ID，多个用逗号分隔）
	PayloadJSON   string // 额外参数 JSON
	Enabled       bool

	LastRunAt  *time.Time
	NextRunAt  *time.Time
	LastTaskID string
	LastStatus string // running / success / failed
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

var ErrScheduledJobNotFound = errors.New("scheduled job not found")

func (j *ScheduledJob) scan(s interface{ Scan(...interface{}) error }) error {
	var enabled int
	var lastRun, nextRun, createdAt, updatedAt sql.NullString
	var lastTaskID, lastStatus sql.NullString
	if err := s.Scan(
		&j.ID, &j.EnvironmentID, &j.Name, &j.TaskType, &j.CronExpr, &j.Target, &j.PayloadJSON,
		&enabled,
		&lastRun, &nextRun,
		&lastTaskID, &lastStatus,
		&createdAt, &updatedAt,
	); err != nil {
		return err
	}
	j.Enabled = enabled == 1
	j.LastTaskID = lastTaskID.String
	j.LastStatus = lastStatus.String
	if lastRun.Valid {
		if t, ok := ParseSQLiteTime(lastRun.String); ok {
			j.LastRunAt = &t
		}
	}
	if nextRun.Valid {
		if t, ok := ParseSQLiteTime(nextRun.String); ok {
			j.NextRunAt = &t
		}
	}
	if createdAt.Valid {
		if t, ok := ParseSQLiteTime(createdAt.String); ok {
			j.CreatedAt = t
		}
	}
	if updatedAt.Valid {
		if t, ok := ParseSQLiteTime(updatedAt.String); ok {
			j.UpdatedAt = t
		}
	}
	return nil
}

const scheduledJobColumns = `
    id, environment_id, name, task_type, cron_expr, target, payload_json,
    enabled,
    last_run_at, next_run_at,
    last_task_id, last_status,
    created_at, updated_at
`

// CreateScheduledJob 创建任务并回填 ID / 时间戳。
func CreateScheduledJob(j *ScheduledJob) error {
	return CreateScheduledJobInEnvironment(LocalEnvironmentID, j)
}

func CreateScheduledJobInEnvironment(environmentID string, j *ScheduledJob) error {
	if j == nil {
		return errors.New("job is nil")
	}
	var err error
	environmentID, err = normalizeScheduledJobEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	j.EnvironmentID = environmentID
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	enabled := boolToInt(j.Enabled)
	res, err := db.Exec(`
	    INSERT INTO scheduled_jobs (environment_id, name, task_type, cron_expr, target, payload_json, enabled, created_at, updated_at)
	    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, j.EnvironmentID, j.Name, j.TaskType, j.CronExpr, j.Target, j.PayloadJSON, enabled, now, now)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	j.ID = id
	return GetScheduledJobInEnvironment(j.EnvironmentID, id, j)
}

// UpdateScheduledJob 更新任务定义（不含运行时字段 last_run_at/next_run_at/last_task_id/last_status）。
func UpdateScheduledJob(j *ScheduledJob) error {
	return UpdateScheduledJobInEnvironment(LocalEnvironmentID, j)
}

func UpdateScheduledJobInEnvironment(environmentID string, j *ScheduledJob) error {
	if j == nil || j.ID == 0 {
		return errors.New("invalid job id")
	}
	var err error
	environmentID, err = normalizeScheduledJobEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	j.EnvironmentID = environmentID
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	enabled := boolToInt(j.Enabled)
	_, err = db.Exec(`
	    UPDATE scheduled_jobs SET
	      name = ?,
	      task_type = ?,
	      cron_expr = ?,
	      target = ?,
	      payload_json = ?,
	      enabled = ?,
	      updated_at = ?
	    WHERE id = ? AND environment_id = ?
	`, j.Name, j.TaskType, j.CronExpr, j.Target, j.PayloadJSON, enabled, now, j.ID, j.EnvironmentID)
	if err != nil {
		return err
	}
	return GetScheduledJobInEnvironment(j.EnvironmentID, j.ID, j)
}

// UpdateScheduledJobRunResult 由调度器在触发后回填运行时字段。
func UpdateScheduledJobRunResult(id int64, lastRunAt time.Time, nextRunAt *time.Time, lastTaskID, lastStatus string) error {
	return UpdateScheduledJobRunResultInEnvironment(LocalEnvironmentID, id, lastRunAt, nextRunAt, lastTaskID, lastStatus)
}

func UpdateScheduledJobRunResultInEnvironment(environmentID string, id int64, lastRunAt time.Time, nextRunAt *time.Time, lastTaskID, lastStatus string) error {
	var err error
	environmentID, err = normalizeScheduledJobEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	lastRunStr := lastRunAt.In(chinaLocation).Format(time.RFC3339)
	var nextRunStr interface{}
	if nextRunAt != nil {
		nextRunStr = nextRunAt.In(chinaLocation).Format(time.RFC3339)
	}
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	_, err = db.Exec(`
	    UPDATE scheduled_jobs SET
	      last_run_at = ?,
	      next_run_at = ?,
	      last_task_id = ?,
	      last_status = ?,
	      updated_at = ?
	    WHERE id = ? AND environment_id = ?
	`, lastRunStr, nextRunStr, lastTaskID, lastStatus, now, id, environmentID)
	return err
}

// SetScheduledJobEnabled 启用/禁用任务。
func SetScheduledJobEnabled(id int64, enabled bool) error {
	return SetScheduledJobEnabledInEnvironment(LocalEnvironmentID, id, enabled)
}

func SetScheduledJobEnabledInEnvironment(environmentID string, id int64, enabled bool) error {
	var err error
	environmentID, err = normalizeScheduledJobEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	_, err = db.Exec(`UPDATE scheduled_jobs SET enabled = ?, updated_at = ? WHERE id = ? AND environment_id = ?`, boolToInt(enabled), now, id, environmentID)
	return err
}

// GetScheduledJob 按 ID 查询单条任务。
func GetScheduledJob(id int64, j *ScheduledJob) error {
	return GetScheduledJobInEnvironment(LocalEnvironmentID, id, j)
}

func GetScheduledJobInEnvironment(environmentID string, id int64, j *ScheduledJob) error {
	if j == nil {
		return errors.New("job is nil")
	}
	var err error
	environmentID, err = normalizeScheduledJobEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	row := db.QueryRow(`SELECT `+scheduledJobColumns+` FROM scheduled_jobs WHERE id = ? AND environment_id = ?`, id, environmentID)
	err = j.scan(row)
	if err == sql.ErrNoRows {
		return ErrScheduledJobNotFound
	}
	return err
}

// ListScheduledJobs 列出全部任务（按 id 倒序，最新创建在前）。
func ListScheduledJobs() ([]ScheduledJob, error) {
	return ListScheduledJobsInEnvironment(LocalEnvironmentID)
}

func ListScheduledJobsInEnvironment(environmentID string) ([]ScheduledJob, error) {
	var err error
	environmentID, err = normalizeScheduledJobEnvironmentID(environmentID)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT `+scheduledJobColumns+` FROM scheduled_jobs WHERE environment_id = ? ORDER BY id DESC`, environmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ScheduledJob
	for rows.Next() {
		var j ScheduledJob
		if err := j.scan(rows); err != nil {
			return nil, err
		}
		list = append(list, j)
	}
	return list, rows.Err()
}

// ListEnabledScheduledJobs 返回所有启用的任务（调度器启动时加载用）。
func ListEnabledScheduledJobs() ([]ScheduledJob, error) {
	return ListEnabledScheduledJobsInEnvironment(LocalEnvironmentID)
}

func ListEnabledScheduledJobsInEnvironment(environmentID string) ([]ScheduledJob, error) {
	var err error
	environmentID, err = normalizeScheduledJobEnvironmentID(environmentID)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT `+scheduledJobColumns+` FROM scheduled_jobs WHERE environment_id = ? AND enabled = 1 ORDER BY id ASC`, environmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ScheduledJob
	for rows.Next() {
		var j ScheduledJob
		if err := j.scan(rows); err != nil {
			return nil, err
		}
		list = append(list, j)
	}
	return list, rows.Err()
}

// DeleteScheduledJob 按 ID 删除任务。
func DeleteScheduledJob(id int64) error {
	return DeleteScheduledJobInEnvironment(LocalEnvironmentID, id)
}

func DeleteScheduledJobInEnvironment(environmentID string, id int64) error {
	var err error
	environmentID, err = normalizeScheduledJobEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	_, err = db.Exec(`DELETE FROM scheduled_jobs WHERE id = ? AND environment_id = ?`, id, environmentID)
	return err
}

// ListScheduledJobRuns 查询某个 job 的历史执行记录。
// task type 形如 "scheduled_job_<id>"，用前缀匹配。
func ListScheduledJobRuns(id int64, limit int) ([]TaskRecord, error) {
	return ListScheduledJobRunsInEnvironment(LocalEnvironmentID, id, limit)
}

func ListScheduledJobRunsInEnvironment(environmentID string, id int64, limit int) ([]TaskRecord, error) {
	var err error
	environmentID, err = normalizeScheduledJobEnvironmentID(environmentID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	pattern := fmt.Sprintf("scheduled_job_%d", id) + "%"
	rows, err := db.Query(`
	    SELECT id, environment_id, type, status, COALESCE(result_json, ''), COALESCE(error, ''), COALESCE(created_at, ''), COALESCE(updated_at, '')
	    FROM tasks
	    WHERE environment_id = ? AND type LIKE ?
	    ORDER BY updated_at DESC
	    LIMIT ?
	`, environmentID, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []TaskRecord
	for rows.Next() {
		var t TaskRecord
		if err := rows.Scan(&t.ID, &t.EnvironmentID, &t.Type, &t.Status, &t.ResultJSON, &t.Error, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func normalizeScheduledJobEnvironmentID(environmentID string) (string, error) {
	environmentID = strings.TrimSpace(environmentID)
	if environmentID == "" {
		environmentID = LocalEnvironmentID
	}
	if !validEnvironmentID(environmentID) {
		return "", fmt.Errorf("环境 ID 不合法")
	}
	if _, err := GetEnvironment(environmentID); err != nil {
		return "", err
	}
	return environmentID, nil
}
