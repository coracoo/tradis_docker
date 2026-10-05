package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/secrets"
)

// UpsertTask 任务落库：不存在则创建，存在则更新 type/status/updated_at。
func UpsertTask(id string, taskType string, status string) error {
	return UpsertTaskInEnvironment(LocalEnvironmentID, id, taskType, status)
}

// CreateTaskIfAbsent atomically claims a task ID without changing an existing task.
func CreateTaskIfAbsent(id string, taskType string, status string, requestDigest string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("数据库连接未初始化")
	}
	id = strings.TrimSpace(id)
	taskType = strings.TrimSpace(taskType)
	status = strings.TrimSpace(status)
	if id == "" || taskType == "" || status == "" {
		return false, fmt.Errorf("任务参数不完整")
	}

	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	result, err := db.Exec(`
	    INSERT INTO tasks (id, environment_id, type, status, result_json, error, created_at, updated_at, request_digest)
	    VALUES (?, ?, ?, ?, '', '', ?, ?, ?) ON CONFLICT(id) DO NOTHING
	`, id, LocalEnvironmentID, taskType, status, now, now, requestDigest)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows == 0 {
		var storedDigest, storedEnvironment, storedType string
		if err := db.QueryRow("SELECT request_digest, environment_id, type FROM tasks WHERE id = ?", id).Scan(&storedDigest, &storedEnvironment, &storedType); err != nil {
			return false, err
		}
		if storedDigest != requestDigest || storedEnvironment != LocalEnvironmentID || storedType != taskType {
			return false, ErrTaskRequestConflict
		}
	}
	return rows == 1, err
}

func UpsertTaskInEnvironment(environmentID string, id string, taskType string, status string) error {
	if db == nil {
		return fmt.Errorf("数据库连接未初始化")
	}
	var err error
	environmentID, err = normalizeTaskEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	taskType = strings.TrimSpace(taskType)
	status = strings.TrimSpace(status)
	if id == "" || taskType == "" || status == "" {
		return fmt.Errorf("任务参数不完整")
	}

	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	_, err = db.Exec(`
	    INSERT INTO tasks (id, environment_id, type, status, result_json, error, created_at, updated_at)
	    VALUES (?, ?, ?, ?, '', '', ?, ?)
	    ON CONFLICT(id) DO UPDATE SET
	      environment_id = excluded.environment_id,
	      type = excluded.type,
	      status = excluded.status,
	      updated_at = excluded.updated_at
	`, id, environmentID, taskType, status, now, now)
	return err
}

// UpdateTaskStatus 更新任务状态（不改变 result_json）。
func UpdateTaskStatus(id string, status string) error {
	return UpdateTaskStatusInEnvironment(LocalEnvironmentID, id, status)
}

func UpdateTaskStatusInEnvironment(environmentID string, id string, status string) error {
	if db == nil {
		return fmt.Errorf("数据库连接未初始化")
	}
	var err error
	environmentID, err = normalizeTaskEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	if id == "" || status == "" {
		return fmt.Errorf("任务参数不完整")
	}
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	_, err = db.Exec(`UPDATE tasks SET status = ?, updated_at = ? WHERE id = ? AND environment_id = ?`, status, now, id, environmentID)
	return err
}

// FinishTask 结束任务并写入 result_json/error/status。
func FinishTask(id string, status string, result any, errStr string) error {
	return FinishTaskInEnvironment(LocalEnvironmentID, id, status, result, errStr)
}

func FinishTaskInEnvironment(environmentID string, id string, status string, result any, errStr string) error {
	if db == nil {
		return fmt.Errorf("数据库连接未初始化")
	}
	var err error
	environmentID, err = normalizeTaskEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	if id == "" || status == "" {
		return fmt.Errorf("任务参数不完整")
	}

	resultJSON := ""
	if result != nil {
		if b, err := json.Marshal(secrets.RedactValue(result)); err == nil {
			resultJSON = logging.RedactText(string(b))
		}
	}

	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	_, err = db.Exec(`
	    UPDATE tasks
	    SET status = ?, result_json = ?, error = ?, updated_at = ?
	    WHERE id = ? AND environment_id = ?
	`, status, resultJSON, logging.RedactText(secrets.RedactString(strings.TrimSpace(errStr))), now, id, environmentID)
	return err
}

// AppendTaskLogWithSeq 追加任务日志（seq 在业务侧生成，用于稳定的 SSE 断线续看）。
func AppendTaskLogWithSeq(taskID string, seq int64, at time.Time, logType string, message string) error {
	return AppendTaskLogWithSeqInEnvironment(LocalEnvironmentID, taskID, seq, at, logType, message)
}

func AppendTaskLogWithSeqInEnvironment(environmentID string, taskID string, seq int64, at time.Time, logType string, message string) error {
	if db == nil {
		return fmt.Errorf("数据库连接未初始化")
	}
	var err error
	environmentID, err = normalizeTaskEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || seq <= 0 {
		return fmt.Errorf("日志参数不完整")
	}
	logType = strings.TrimSpace(logType)
	message = logging.RedactText(secrets.RedactString(strings.TrimSpace(message)))
	if message == "" {
		return nil
	}
	now := at
	if now.IsZero() {
		now = time.Now()
	}
	nowStr := FormatStamp(now)

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
	    INSERT OR IGNORE INTO task_logs (task_id, environment_id, seq, time, type, message)
	    VALUES (?, ?, ?, ?, ?, ?)
	`, taskID, environmentID, seq, nowStr, logType, message); err != nil {
		return err
	}
	_, _ = tx.Exec(`UPDATE tasks SET updated_at = ? WHERE id = ? AND environment_id = ?`, nowStr, taskID, environmentID)
	return tx.Commit()
}

// GetTask 获取任务详情（用于进度查询/断线续看）。
func GetTask(id string) (TaskRecord, error) {
	return GetTaskInEnvironment(LocalEnvironmentID, id)
}

func GetTaskInEnvironment(environmentID string, id string) (TaskRecord, error) {
	var t TaskRecord
	if db == nil {
		return t, fmt.Errorf("数据库连接未初始化")
	}
	var err error
	environmentID, err = normalizeTaskEnvironmentID(environmentID)
	if err != nil {
		return t, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return t, sql.ErrNoRows
	}
	err = db.QueryRow(`
	    SELECT id, environment_id, type, status, COALESCE(result_json, ''), COALESCE(error, ''), COALESCE(created_at, ''), COALESCE(updated_at, '')
	    FROM tasks
	    WHERE id = ? AND environment_id = ?
	`, id, environmentID).Scan(&t.ID, &t.EnvironmentID, &t.Type, &t.Status, &t.ResultJSON, &t.Error, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func buildInPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, "?")
	}
	return strings.Join(parts, ",")
}

// LatestComposeUpdatesInEnvironment returns persisted results per project/path,
// independently of task-list pagination. Invalid legacy JSON is excluded.
func LatestComposeUpdatesInEnvironment(environmentID string) ([]TaskRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}
	environmentID, err := normalizeTaskEnvironmentID(environmentID)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`
	WITH valid AS (
	  SELECT id,environment_id,type,status,result_json,error,created_at,updated_at,
	    COALESCE(NULLIF(json_extract(result_json,'$.composeProjectName'),''), json_extract(result_json,'$.project')) AS project_name,
	    COALESCE(json_extract(result_json,'$.projectDir'),'') AS project_dir
	  FROM tasks WHERE environment_id=? AND type='compose_update'
	    AND status IN ('success','error','cancelled','canceled') AND json_valid(result_json)
	), ranked AS (
	  SELECT *, ROW_NUMBER() OVER(PARTITION BY project_name,project_dir ORDER BY created_at DESC,id DESC) AS ranking
	  FROM valid WHERE project_name IS NOT NULL AND project_name<>''
	)
	SELECT id,environment_id,type,status,COALESCE(result_json,''),COALESCE(error,''),COALESCE(created_at,''),COALESCE(updated_at,'')
	FROM ranked WHERE ranking=1 ORDER BY created_at DESC,id DESC`, environmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []TaskRecord
	for rows.Next() {
		var task TaskRecord
		if err := rows.Scan(&task.ID, &task.EnvironmentID, &task.Type, &task.Status, &task.ResultJSON, &task.Error, &task.CreatedAt, &task.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// ListTasks 列出任务（支持按 type/status 过滤，默认按 updated_at 倒序）。
func ListTasks(taskTypes []string, statuses []string, limit int) ([]TaskRecord, error) {
	return ListTasksInEnvironment(LocalEnvironmentID, taskTypes, statuses, limit)
}

func ListTasksInEnvironment(environmentID string, taskTypes []string, statuses []string, limit int) ([]TaskRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}
	var err error
	environmentID, err = normalizeTaskEnvironmentID(environmentID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	args := []any{environmentID}
	conds := []string{"environment_id = ?"}

	if len(taskTypes) > 0 {
		ph := buildInPlaceholders(len(taskTypes))
		conds = append(conds, "type IN ("+ph+")")
		for _, v := range taskTypes {
			args = append(args, strings.TrimSpace(v))
		}
	}
	if len(statuses) > 0 {
		ph := buildInPlaceholders(len(statuses))
		conds = append(conds, "status IN ("+ph+")")
		for _, v := range statuses {
			args = append(args, strings.TrimSpace(v))
		}
	}

	query := `
	    SELECT id, environment_id, type, status, COALESCE(result_json, ''), COALESCE(error, ''), COALESCE(created_at, ''), COALESCE(updated_at, '')
	    FROM tasks
	`
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY updated_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]TaskRecord, 0)
	for rows.Next() {
		var t TaskRecord
		if err := rows.Scan(&t.ID, &t.EnvironmentID, &t.Type, &t.Status, &t.ResultJSON, &t.Error, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// FailActiveTasksByTypes settles every pending/running task of the supplied
// types after a process restart without a list-size cap.
func FailActiveTasksByTypes(taskTypes []string, errorText string) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("数据库连接未初始化")
	}
	cleanTypes := make([]string, 0, len(taskTypes))
	for _, taskType := range taskTypes {
		if taskType = strings.TrimSpace(taskType); taskType != "" {
			cleanTypes = append(cleanTypes, taskType)
		}
	}
	if len(cleanTypes) == 0 {
		return 0, fmt.Errorf("任务类型不能为空")
	}
	resultJSON, _ := json.Marshal(map[string]any{"interrupted": true})
	args := []any{
		"error",
		string(resultJSON),
		logging.RedactText(secrets.RedactString(strings.TrimSpace(errorText))),
		time.Now().In(chinaLocation).Format(time.RFC3339),
		LocalEnvironmentID,
	}
	for _, taskType := range cleanTypes {
		args = append(args, taskType)
	}
	result, err := db.Exec(`
		UPDATE tasks
		SET status = ?, result_json = ?, error = ?, updated_at = ?
		WHERE environment_id = ?
		  AND type IN (`+buildInPlaceholders(len(cleanTypes))+`)
		  AND status IN ('pending', 'running')
	`, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetTaskLogsAfter 按 seq 游标获取任务日志（用于 SSE 断线续看）。
func GetTaskLogsAfter(taskID string, afterSeq int64, limit int) ([]TaskLogRecord, error) {
	return GetTaskLogsAfterInEnvironment(LocalEnvironmentID, taskID, afterSeq, limit)
}

func GetTaskLogsAfterInEnvironment(environmentID string, taskID string, afterSeq int64, limit int) ([]TaskLogRecord, error) {
	return getTaskLogsInEnvironment(environmentID, taskID, afterSeq, limit, false)
}

// GetTaskLogTail returns the latest bounded task logs in chronological order.
func GetTaskLogTail(taskID string, limit int) ([]TaskLogRecord, error) {
	return GetTaskLogTailInEnvironment(LocalEnvironmentID, taskID, limit)
}

func GetTaskLogTailInEnvironment(environmentID, taskID string, limit int) ([]TaskLogRecord, error) {
	return getTaskLogsInEnvironment(environmentID, taskID, 0, limit, true)
}

func getTaskLogsInEnvironment(environmentID, taskID string, afterSeq int64, limit int, newestFirst bool) ([]TaskLogRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}
	var err error
	environmentID, err = normalizeTaskEnvironmentID(environmentID)
	if err != nil {
		return nil, err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	if afterSeq < 0 {
		afterSeq = 0
	}
	order := "ASC"
	if newestFirst {
		order = "DESC"
	}

	rows, err := db.Query(`
	    SELECT task_id, environment_id, seq, COALESCE(time, ''), COALESCE(type, ''), COALESCE(message, '')
	    FROM task_logs
	    WHERE environment_id = ? AND task_id = ? AND seq > ?
	    ORDER BY seq `+order+`
	    LIMIT ?
	`, environmentID, taskID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]TaskLogRecord, 0)
	for rows.Next() {
		var r TaskLogRecord
		if err := rows.Scan(&r.TaskID, &r.EnvironmentID, &r.Seq, &r.Time, &r.Type, &r.Message); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if newestFirst {
		for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
			out[left], out[right] = out[right], out[left]
		}
	}
	return out, rows.Err()
}

func normalizeTaskEnvironmentID(environmentID string) (string, error) {
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
