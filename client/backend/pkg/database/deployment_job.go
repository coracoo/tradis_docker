package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"dockerpanel/backend/pkg/deployment"
)

const (
	DeploymentJobStateQueued       = string(deployment.JobStateQueued)
	DeploymentJobStatePlanning     = string(deployment.JobStatePlanning)
	DeploymentJobStateWaitingInput = string(deployment.JobStateWaitingInput)
	DeploymentJobStateExecuting    = string(deployment.JobStateExecuting)
	DeploymentJobStateVerifying    = string(deployment.JobStateVerifying)
	DeploymentJobStateSuccess      = string(deployment.JobStateSuccess)
	DeploymentJobStateFailed       = string(deployment.JobStateFailed)
	DeploymentJobStateCanceled     = string(deployment.JobStateCanceled)
)

var ErrInvalidDeploymentJobTransition = errors.New("非法的部署任务状态转换")

type DeploymentJobRecord struct {
	ID             string `json:"id"`
	EnvironmentID  string `json:"environmentId"`
	AgentRunID     string `json:"agentRunId"`
	Kind           string `json:"kind"`
	SourceType     string `json:"sourceType"`
	SourceRef      string `json:"sourceRef"`
	ProjectName    string `json:"projectName"`
	Status         string `json:"status"`
	IdempotencyKey string `json:"idempotencyKey"`
	InputJSON      string `json:"inputJson"`
	ResultJSON     string `json:"resultJson"`
	ErrorCode      string `json:"errorCode"`
	Error          string `json:"error"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
	StartedAt      string `json:"startedAt"`
	FinishedAt     string `json:"finishedAt"`
}

type DeploymentJobStepRecord struct {
	ID          int64  `json:"id"`
	JobID       string `json:"jobId"`
	Seq         int64  `json:"seq"`
	Type        string `json:"type"`
	Stage       string `json:"stage"`
	Message     string `json:"message"`
	PayloadJSON string `json:"payloadJson"`
	ErrorCode   string `json:"errorCode"`
	CreatedAt   string `json:"createdAt"`
}

type DeploymentJobStepInput struct {
	Type      string
	Stage     string
	Message   string
	Payload   any
	ErrorCode string
}

type DeploymentJobStateUpdate struct {
	Status    string
	Result    any
	ErrorCode string
	Error     string
}

type DeploymentManifestRecord struct {
	JobID            string `json:"jobId"`
	EnvironmentID    string `json:"environmentId"`
	SourceCompose    string `json:"sourceCompose"`
	RuntimeCompose   string `json:"runtimeCompose"`
	ImageDigestsJSON string `json:"imageDigestsJson"`
	ChangesJSON      string `json:"changesJson"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
}

type deploymentJobScanner interface {
	Scan(dest ...any) error
}

func normalizeDeploymentJobState(state string) string {
	return string(deployment.NormalizeJobState(state))
}

func validDeploymentJobState(state string) bool {
	return deployment.IsValidJobState(deployment.JobState(state))
}

func terminalDeploymentJobState(state string) bool {
	return deployment.IsTerminalJobState(deployment.JobState(state))
}

func canTransitionDeploymentJob(from string, to string) bool {
	return deployment.CanTransitionJobState(deployment.JobState(from), deployment.JobState(to))
}

func scanDeploymentJob(scanner deploymentJobScanner) (DeploymentJobRecord, error) {
	var out DeploymentJobRecord
	err := scanner.Scan(
		&out.ID,
		&out.EnvironmentID,
		&out.AgentRunID,
		&out.Kind,
		&out.SourceType,
		&out.SourceRef,
		&out.ProjectName,
		&out.Status,
		&out.IdempotencyKey,
		&out.InputJSON,
		&out.ResultJSON,
		&out.ErrorCode,
		&out.Error,
		&out.CreatedAt,
		&out.UpdatedAt,
		&out.StartedAt,
		&out.FinishedAt,
	)
	return out, err
}

func deploymentJobSelectSQL(where string) string {
	return `SELECT id, COALESCE(environment_id, 'local'), COALESCE(agent_run_id, ''),
	              COALESCE(kind, ''), COALESCE(source_type, ''), COALESCE(source_ref, ''),
	              COALESCE(project_name, ''), COALESCE(status, ''), COALESCE(idempotency_key, ''),
	              COALESCE(input_json, '{}'), COALESCE(result_json, ''), COALESCE(error_code, ''),
	              COALESCE(error, ''), COALESCE(created_at, ''), COALESCE(updated_at, ''),
	              COALESCE(started_at, ''), COALESCE(finished_at, '')
	       FROM deployment_jobs ` + where
}

func GetDeploymentJob(id string) (DeploymentJobRecord, error) {
	if db == nil {
		return DeploymentJobRecord{}, fmt.Errorf("数据库连接未初始化")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return DeploymentJobRecord{}, sql.ErrNoRows
	}
	return scanDeploymentJob(db.QueryRow(deploymentJobSelectSQL("WHERE id = ?"), id))
}

func GetDeploymentJobByIdempotencyKey(key string) (DeploymentJobRecord, error) {
	if db == nil {
		return DeploymentJobRecord{}, fmt.Errorf("数据库连接未初始化")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return DeploymentJobRecord{}, sql.ErrNoRows
	}
	return getDeploymentJobByIdempotencyKey(key)
}

func getDeploymentJobByIdempotencyKey(key string) (DeploymentJobRecord, error) {
	return scanDeploymentJob(db.QueryRow(deploymentJobSelectSQL("WHERE idempotency_key = ?"), key))
}

func CreateDeploymentJob(record DeploymentJobRecord) (DeploymentJobRecord, error) {
	if db == nil {
		return DeploymentJobRecord{}, fmt.Errorf("数据库连接未初始化")
	}
	record.ID = strings.TrimSpace(record.ID)
	if record.ID == "" {
		record.ID = fmt.Sprintf("deployment-job-%d", time.Now().UnixNano())
	}
	record.EnvironmentID = strings.TrimSpace(record.EnvironmentID)
	if record.EnvironmentID == "" {
		record.EnvironmentID = LocalEnvironmentID
	}
	if _, err := GetEnvironment(record.EnvironmentID); err != nil {
		return DeploymentJobRecord{}, fmt.Errorf("部署环境不存在: %w", err)
	}
	record.Kind = strings.TrimSpace(record.Kind)
	if record.Kind == "" {
		return DeploymentJobRecord{}, fmt.Errorf("部署任务类型不能为空")
	}
	record.IdempotencyKey = strings.TrimSpace(record.IdempotencyKey)
	if record.IdempotencyKey == "" {
		return DeploymentJobRecord{}, fmt.Errorf("部署任务幂等键不能为空")
	}
	if existing, err := getDeploymentJobByIdempotencyKey(record.IdempotencyKey); err == nil {
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return DeploymentJobRecord{}, err
	}

	record.Status = normalizeDeploymentJobState(record.Status)
	if record.Status == "" {
		record.Status = DeploymentJobStateQueued
	}
	if record.Status != DeploymentJobStateQueued {
		return DeploymentJobRecord{}, fmt.Errorf("部署任务必须从 queued 状态创建")
	}
	record.InputJSON = strings.TrimSpace(record.InputJSON)
	if record.InputJSON == "" {
		record.InputJSON = "{}"
	}
	if !json.Valid([]byte(record.InputJSON)) {
		return DeploymentJobRecord{}, fmt.Errorf("部署任务输入不是有效 JSON")
	}

	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO deployment_jobs (
			id, environment_id, agent_run_id, kind, source_type, source_ref, project_name,
			status, idempotency_key, input_json, result_json, error_code, error,
			created_at, updated_at, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', '', ?, ?, NULL, NULL)
	`,
		record.ID,
		record.EnvironmentID,
		strings.TrimSpace(record.AgentRunID),
		record.Kind,
		strings.TrimSpace(record.SourceType),
		strings.TrimSpace(record.SourceRef),
		strings.TrimSpace(record.ProjectName),
		record.Status,
		record.IdempotencyKey,
		record.InputJSON,
		now,
		now,
	)
	if err != nil {
		if existing, lookupErr := getDeploymentJobByIdempotencyKey(record.IdempotencyKey); lookupErr == nil {
			return existing, nil
		}
		return DeploymentJobRecord{}, err
	}
	return GetDeploymentJob(record.ID)
}

func UpdateDeploymentJobState(id string, update DeploymentJobStateUpdate) (DeploymentJobRecord, error) {
	if db == nil {
		return DeploymentJobRecord{}, fmt.Errorf("数据库连接未初始化")
	}
	id = strings.TrimSpace(id)
	nextState := normalizeDeploymentJobState(update.Status)
	if id == "" || !validDeploymentJobState(nextState) {
		return DeploymentJobRecord{}, fmt.Errorf("部署任务状态参数不完整")
	}

	tx, err := db.Begin()
	if err != nil {
		return DeploymentJobRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := scanDeploymentJob(tx.QueryRow(deploymentJobSelectSQL("WHERE id = ?"), id))
	if err != nil {
		return DeploymentJobRecord{}, err
	}
	if !canTransitionDeploymentJob(current.Status, nextState) {
		return DeploymentJobRecord{}, fmt.Errorf("%w: %s -> %s", ErrInvalidDeploymentJobTransition, current.Status, nextState)
	}
	if normalizeDeploymentJobState(current.Status) == nextState {
		return current, nil
	}

	resultJSON := current.ResultJSON
	if update.Result != nil {
		encoded, err := json.Marshal(deployment.RedactSensitiveValues(update.Result))
		if err != nil {
			return DeploymentJobRecord{}, err
		}
		resultJSON = string(encoded)
	}
	errorCode := strings.TrimSpace(update.ErrorCode)
	errorText := strings.TrimSpace(update.Error)
	if nextState != DeploymentJobStateFailed {
		errorCode = ""
		errorText = ""
	}

	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	startedAt := current.StartedAt
	if startedAt == "" && nextState == DeploymentJobStatePlanning {
		startedAt = now
	}
	finishedAt := current.FinishedAt
	if terminalDeploymentJobState(nextState) {
		finishedAt = now
	}
	_, err = tx.Exec(`
		UPDATE deployment_jobs
		SET status = ?, result_json = ?, error_code = ?, error = ?,
		    updated_at = ?, started_at = NULLIF(?, ''), finished_at = NULLIF(?, '')
		WHERE id = ?
	`, nextState, resultJSON, errorCode, errorText, now, startedAt, finishedAt, id)
	if err != nil {
		return DeploymentJobRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return DeploymentJobRecord{}, err
	}
	return GetDeploymentJob(id)
}

func ListRecoverableDeploymentJobs(limit int) ([]DeploymentJobRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	query := deploymentJobSelectSQL(`WHERE status IN (?, ?, ?, ?, ?)
		ORDER BY updated_at ASC, created_at ASC LIMIT ?`)
	rows, err := db.Query(
		query,
		DeploymentJobStateQueued,
		DeploymentJobStatePlanning,
		DeploymentJobStateWaitingInput,
		DeploymentJobStateExecuting,
		DeploymentJobStateVerifying,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]DeploymentJobRecord, 0)
	for rows.Next() {
		item, err := scanDeploymentJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func ListDeploymentJobsByAgentRun(runID string, limit int) ([]DeploymentJobRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("Agent Run ID 不能为空")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := db.Query(deploymentJobSelectSQL(`WHERE agent_run_id = ?
		ORDER BY created_at DESC, rowid DESC LIMIT ?`), runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DeploymentJobRecord, 0)
	for rows.Next() {
		item, err := scanDeploymentJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func AppendDeploymentJobStep(jobID string, input DeploymentJobStepInput) (DeploymentJobStepRecord, error) {
	var out DeploymentJobStepRecord
	if db == nil {
		return out, fmt.Errorf("数据库连接未初始化")
	}
	jobID = strings.TrimSpace(jobID)
	stepType := strings.TrimSpace(input.Type)
	if jobID == "" || stepType == "" {
		return out, fmt.Errorf("部署任务步骤参数不完整")
	}
	if _, err := GetDeploymentJob(jobID); err != nil {
		return out, err
	}
	payloadJSON := ""
	if input.Payload != nil {
		encoded, err := json.Marshal(deployment.RedactSensitiveValues(input.Payload))
		if err != nil {
			return out, err
		}
		payloadJSON = string(encoded)
	}
	stage := strings.TrimSpace(input.Stage)
	message := strings.TrimSpace(input.Message)
	errorCode := strings.TrimSpace(input.ErrorCode)
	now := time.Now().In(chinaLocation).Format(time.RFC3339)

	for attempt := 0; attempt < 10; attempt++ {
		tx, err := db.Begin()
		if err != nil {
			if retryableSQLiteWriteError(err) && attempt < 9 {
				continue
			}
			return out, err
		}

		res, err := tx.Exec(`
			INSERT INTO deployment_job_steps (
				job_id, seq, type, stage, message, payload_json, error_code, created_at
			)
			SELECT ?, COALESCE(MAX(seq), 0) + 1, ?, ?, ?, ?, ?, ?
			FROM deployment_job_steps
			WHERE job_id = ?
		`, jobID, stepType, stage, message, payloadJSON, errorCode, now, jobID)
		if err != nil {
			_ = tx.Rollback()
			if retryableSQLiteWriteError(err) && attempt < 9 {
				continue
			}
			return out, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			_ = tx.Rollback()
			return out, err
		}
		if err := tx.QueryRow(`
			SELECT id, job_id, seq, COALESCE(type, ''), COALESCE(stage, ''),
			       COALESCE(message, ''), COALESCE(payload_json, ''), COALESCE(error_code, ''),
			       COALESCE(created_at, '')
			FROM deployment_job_steps WHERE id = ?
		`, id).Scan(
			&out.ID,
			&out.JobID,
			&out.Seq,
			&out.Type,
			&out.Stage,
			&out.Message,
			&out.PayloadJSON,
			&out.ErrorCode,
			&out.CreatedAt,
		); err != nil {
			_ = tx.Rollback()
			return DeploymentJobStepRecord{}, err
		}
		if _, err := tx.Exec(`UPDATE deployment_jobs SET updated_at = ? WHERE id = ?`, now, jobID); err != nil {
			_ = tx.Rollback()
			return DeploymentJobStepRecord{}, err
		}
		if err := tx.Commit(); err != nil {
			if retryableSQLiteWriteError(err) && attempt < 9 {
				continue
			}
			return DeploymentJobStepRecord{}, err
		}
		return out, nil
	}
	return DeploymentJobStepRecord{}, fmt.Errorf("追加部署任务步骤失败：并发重试耗尽")
}

func retryableSQLiteWriteError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "unique constraint")
}

func GetDeploymentJobStepsAfter(jobID string, afterSeq int64, limit int) ([]DeploymentJobStepRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, nil
	}
	if afterSeq < 0 {
		afterSeq = 0
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	rows, err := db.Query(`
		SELECT id, job_id, seq, COALESCE(type, ''), COALESCE(stage, ''),
		       COALESCE(message, ''), COALESCE(payload_json, ''), COALESCE(error_code, ''),
		       COALESCE(created_at, '')
		FROM deployment_job_steps
		WHERE job_id = ? AND seq > ?
		ORDER BY seq ASC
		LIMIT ?
	`, jobID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]DeploymentJobStepRecord, 0)
	for rows.Next() {
		var item DeploymentJobStepRecord
		if err := rows.Scan(
			&item.ID,
			&item.JobID,
			&item.Seq,
			&item.Type,
			&item.Stage,
			&item.Message,
			&item.PayloadJSON,
			&item.ErrorCode,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func SaveDeploymentManifest(manifest deployment.Manifest) (DeploymentManifestRecord, error) {
	if db == nil {
		return DeploymentManifestRecord{}, fmt.Errorf("数据库连接未初始化")
	}
	manifest.JobID = strings.TrimSpace(manifest.JobID)
	if manifest.JobID == "" {
		return DeploymentManifestRecord{}, fmt.Errorf("部署 manifest 缺少 jobId")
	}
	if _, err := GetDeploymentJob(manifest.JobID); err != nil {
		return DeploymentManifestRecord{}, err
	}
	if strings.TrimSpace(manifest.EnvironmentID) == "" {
		manifest.EnvironmentID = LocalEnvironmentID
	}
	redacted, err := deployment.RedactManifest(manifest)
	if err != nil {
		return DeploymentManifestRecord{}, err
	}
	imageDigests, err := json.Marshal(redacted.ImageDigests)
	if err != nil {
		return DeploymentManifestRecord{}, err
	}
	changes, err := json.Marshal(redacted.Changes)
	if err != nil {
		return DeploymentManifestRecord{}, err
	}
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	_, err = db.Exec(`
		INSERT INTO deployment_manifests (
			job_id, environment_id, source_compose, runtime_compose,
			image_digests_json, changes_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(job_id) DO UPDATE SET
			environment_id = excluded.environment_id,
			source_compose = excluded.source_compose,
			runtime_compose = excluded.runtime_compose,
			image_digests_json = excluded.image_digests_json,
			changes_json = excluded.changes_json,
			updated_at = excluded.updated_at
	`,
		redacted.JobID,
		redacted.EnvironmentID,
		redacted.SourceCompose,
		redacted.RuntimeCompose,
		string(imageDigests),
		string(changes),
		now,
		now,
	)
	if err != nil {
		return DeploymentManifestRecord{}, err
	}
	return GetDeploymentManifest(redacted.JobID)
}

func GetDeploymentManifest(jobID string) (DeploymentManifestRecord, error) {
	var out DeploymentManifestRecord
	if db == nil {
		return out, fmt.Errorf("数据库连接未初始化")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return out, sql.ErrNoRows
	}
	err := db.QueryRow(`
		SELECT job_id, COALESCE(environment_id, 'local'), COALESCE(source_compose, ''),
		       COALESCE(runtime_compose, ''), COALESCE(image_digests_json, '{}'),
		       COALESCE(changes_json, '[]'), COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM deployment_manifests WHERE job_id = ?
	`, jobID).Scan(
		&out.JobID,
		&out.EnvironmentID,
		&out.SourceCompose,
		&out.RuntimeCompose,
		&out.ImageDigestsJSON,
		&out.ChangesJSON,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	return out, err
}
