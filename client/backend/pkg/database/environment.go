package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const LocalEnvironmentID = "local"

const (
	EnvironmentAccessObserve = "observe"
	EnvironmentAccessManage  = "manage"
)

type EnvironmentRecord struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ConnectionMode   string `json:"connectionMode"`
	Status           string `json:"status"`
	AccessMode       string `json:"accessMode"`
	CapabilitiesJSON string `json:"capabilitiesJson"`
	LastSeenAt       string `json:"lastSeenAt"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
}

type NASProfileRecord struct {
	EnvironmentID string `json:"environmentId"`
	ProfileJSON   string `json:"profileJson"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

func createEnvironmentAndDeploymentTables() error {
	if db == nil {
		return fmt.Errorf("数据库连接未初始化")
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS environments (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			connection_mode TEXT NOT NULL DEFAULT 'local',
			status TEXT NOT NULL DEFAULT 'offline',
			access_mode TEXT NOT NULL DEFAULT 'observe',
			capabilities_json TEXT NOT NULL DEFAULT '{}',
			last_seen_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS nas_profiles (
			environment_id TEXT PRIMARY KEY,
			profile_json TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS deployment_jobs (
			id TEXT PRIMARY KEY,
			environment_id TEXT NOT NULL DEFAULT 'local',
			agent_run_id TEXT,
			kind TEXT NOT NULL,
			source_type TEXT,
			source_ref TEXT,
			project_name TEXT,
			status TEXT NOT NULL DEFAULT 'queued',
			idempotency_key TEXT NOT NULL UNIQUE,
			input_json TEXT NOT NULL DEFAULT '{}',
			result_json TEXT,
			error_code TEXT,
			error TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			started_at DATETIME,
			finished_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS deployment_job_steps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			type TEXT NOT NULL,
			stage TEXT,
			message TEXT,
			payload_json TEXT,
			error_code TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(job_id, seq)
		)`,
		`CREATE TABLE IF NOT EXISTS deployment_manifests (
			job_id TEXT PRIMARY KEY,
			environment_id TEXT NOT NULL DEFAULT 'local',
			source_compose TEXT,
			runtime_compose TEXT,
			image_digests_json TEXT NOT NULL DEFAULT '{}',
			changes_json TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}

	if err := ensureTableColumns("environments", []columnSpec{
		{Name: "name", AddColumnSQL: "name TEXT"},
		{Name: "connection_mode", AddColumnSQL: "connection_mode TEXT DEFAULT 'local'", BackfillSQL: []string{"UPDATE environments SET connection_mode = 'local' WHERE connection_mode IS NULL OR connection_mode = ''"}},
		{Name: "status", AddColumnSQL: "status TEXT DEFAULT 'offline'", BackfillSQL: []string{"UPDATE environments SET status = 'offline' WHERE status IS NULL OR status = ''"}},
		{Name: "access_mode", AddColumnSQL: "access_mode TEXT DEFAULT 'observe'", BackfillSQL: []string{"UPDATE environments SET access_mode = 'observe' WHERE access_mode IS NULL OR access_mode NOT IN ('observe', 'manage')", "UPDATE environments SET access_mode = 'manage' WHERE id = 'local'"}},
		{Name: "capabilities_json", AddColumnSQL: "capabilities_json TEXT DEFAULT '{}'", BackfillSQL: []string{"UPDATE environments SET capabilities_json = '{}' WHERE capabilities_json IS NULL OR capabilities_json = ''"}},
		{Name: "last_seen_at", AddColumnSQL: "last_seen_at DATETIME"},
		{Name: "created_at", AddColumnSQL: "created_at DATETIME", BackfillSQL: []string{"UPDATE environments SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL"}},
		{Name: "updated_at", AddColumnSQL: "updated_at DATETIME", BackfillSQL: []string{"UPDATE environments SET updated_at = CURRENT_TIMESTAMP WHERE updated_at IS NULL"}},
	}); err != nil {
		return err
	}
	if err := ensureTableColumns("nas_profiles", []columnSpec{
		{Name: "profile_json", AddColumnSQL: "profile_json TEXT DEFAULT '{}'", BackfillSQL: []string{"UPDATE nas_profiles SET profile_json = '{}' WHERE profile_json IS NULL OR profile_json = ''"}},
		{Name: "created_at", AddColumnSQL: "created_at DATETIME", BackfillSQL: []string{"UPDATE nas_profiles SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL"}},
		{Name: "updated_at", AddColumnSQL: "updated_at DATETIME", BackfillSQL: []string{"UPDATE nas_profiles SET updated_at = CURRENT_TIMESTAMP WHERE updated_at IS NULL"}},
	}); err != nil {
		return err
	}
	if err := ensureTableColumns("deployment_jobs", []columnSpec{
		{Name: "environment_id", AddColumnSQL: "environment_id TEXT DEFAULT 'local'", BackfillSQL: []string{"UPDATE deployment_jobs SET environment_id = 'local' WHERE environment_id IS NULL OR environment_id = ''"}},
		{Name: "agent_run_id", AddColumnSQL: "agent_run_id TEXT"},
		{Name: "kind", AddColumnSQL: "kind TEXT"},
		{Name: "source_type", AddColumnSQL: "source_type TEXT"},
		{Name: "source_ref", AddColumnSQL: "source_ref TEXT"},
		{Name: "project_name", AddColumnSQL: "project_name TEXT"},
		{Name: "status", AddColumnSQL: "status TEXT DEFAULT 'queued'", BackfillSQL: []string{"UPDATE deployment_jobs SET status = 'queued' WHERE status IS NULL OR status = ''"}},
		{Name: "idempotency_key", AddColumnSQL: "idempotency_key TEXT"},
		{Name: "input_json", AddColumnSQL: "input_json TEXT DEFAULT '{}'", BackfillSQL: []string{"UPDATE deployment_jobs SET input_json = '{}' WHERE input_json IS NULL OR input_json = ''"}},
		{Name: "result_json", AddColumnSQL: "result_json TEXT"},
		{Name: "error_code", AddColumnSQL: "error_code TEXT"},
		{Name: "error", AddColumnSQL: "error TEXT"},
		{Name: "created_at", AddColumnSQL: "created_at DATETIME", BackfillSQL: []string{"UPDATE deployment_jobs SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL"}},
		{Name: "updated_at", AddColumnSQL: "updated_at DATETIME", BackfillSQL: []string{"UPDATE deployment_jobs SET updated_at = CURRENT_TIMESTAMP WHERE updated_at IS NULL"}},
		{Name: "started_at", AddColumnSQL: "started_at DATETIME"},
		{Name: "finished_at", AddColumnSQL: "finished_at DATETIME"},
	}); err != nil {
		return err
	}
	if err := ensureTableColumns("deployment_job_steps", []columnSpec{
		{Name: "job_id", AddColumnSQL: "job_id TEXT"},
		{Name: "seq", AddColumnSQL: "seq INTEGER"},
		{Name: "type", AddColumnSQL: "type TEXT"},
		{Name: "stage", AddColumnSQL: "stage TEXT"},
		{Name: "message", AddColumnSQL: "message TEXT"},
		{Name: "payload_json", AddColumnSQL: "payload_json TEXT"},
		{Name: "error_code", AddColumnSQL: "error_code TEXT"},
		{Name: "created_at", AddColumnSQL: "created_at DATETIME", BackfillSQL: []string{"UPDATE deployment_job_steps SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL"}},
	}); err != nil {
		return err
	}
	if err := ensureTableColumns("deployment_manifests", []columnSpec{
		{Name: "environment_id", AddColumnSQL: "environment_id TEXT DEFAULT 'local'", BackfillSQL: []string{"UPDATE deployment_manifests SET environment_id = 'local' WHERE environment_id IS NULL OR environment_id = ''"}},
		{Name: "source_compose", AddColumnSQL: "source_compose TEXT"},
		{Name: "runtime_compose", AddColumnSQL: "runtime_compose TEXT"},
		{Name: "image_digests_json", AddColumnSQL: "image_digests_json TEXT DEFAULT '{}'", BackfillSQL: []string{"UPDATE deployment_manifests SET image_digests_json = '{}' WHERE image_digests_json IS NULL OR image_digests_json = ''"}},
		{Name: "changes_json", AddColumnSQL: "changes_json TEXT DEFAULT '[]'", BackfillSQL: []string{"UPDATE deployment_manifests SET changes_json = '[]' WHERE changes_json IS NULL OR changes_json = ''"}},
		{Name: "created_at", AddColumnSQL: "created_at DATETIME", BackfillSQL: []string{"UPDATE deployment_manifests SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL"}},
		{Name: "updated_at", AddColumnSQL: "updated_at DATETIME", BackfillSQL: []string{"UPDATE deployment_manifests SET updated_at = CURRENT_TIMESTAMP WHERE updated_at IS NULL"}},
	}); err != nil {
		return err
	}

	if _, err := db.Exec(`
		INSERT OR IGNORE INTO environments (
			id, name, connection_mode, status, access_mode, capabilities_json, last_seen_at, created_at, updated_at
		) VALUES (?, '本机', 'local', 'online', 'manage', '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, LocalEnvironmentID); err != nil {
		return err
	}
	if _, err := db.Exec(`
		INSERT OR IGNORE INTO nas_profiles (environment_id, profile_json, created_at, updated_at)
		VALUES (?, '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, LocalEnvironmentID); err != nil {
		return err
	}

	indexes := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_deployment_jobs_idempotency ON deployment_jobs(idempotency_key)`,
		`CREATE INDEX IF NOT EXISTS idx_deployment_jobs_recovery ON deployment_jobs(status, updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_deployment_jobs_environment ON deployment_jobs(environment_id, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_deployment_job_steps_job_seq ON deployment_job_steps(job_id, seq)`,
	}
	for _, statement := range indexes {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return createEditionEnvironmentTables()
}

func validEnvironmentID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for index, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || (char == '-' && index > 0) {
			continue
		}
		return false
	}
	return true
}

type environmentScanner interface {
	Scan(dest ...any) error
}

func scanEnvironment(scanner environmentScanner) (EnvironmentRecord, error) {
	var out EnvironmentRecord
	err := scanner.Scan(
		&out.ID,
		&out.Name,
		&out.ConnectionMode,
		&out.Status,
		&out.AccessMode,
		&out.CapabilitiesJSON,
		&out.LastSeenAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	return out, err
}

func GetEnvironment(id string) (EnvironmentRecord, error) {
	if db == nil {
		return EnvironmentRecord{}, fmt.Errorf("数据库连接未初始化")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return EnvironmentRecord{}, sql.ErrNoRows
	}
	return scanEnvironment(db.QueryRow(`
		SELECT id, COALESCE(name, ''), COALESCE(connection_mode, ''), COALESCE(status, ''),
		       COALESCE(access_mode, 'observe'),
		       COALESCE(capabilities_json, '{}'), COALESCE(last_seen_at, ''),
		       COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM environments WHERE id = ?
	`, id))
}

func ListEnvironments() ([]EnvironmentRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}
	rows, err := db.Query(`
		SELECT id, COALESCE(name, ''), COALESCE(connection_mode, ''), COALESCE(status, ''),
		       COALESCE(access_mode, 'observe'),
		       COALESCE(capabilities_json, '{}'), COALESCE(last_seen_at, ''),
		       COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM environments
		WHERE status != 'revoked'
		ORDER BY CASE WHEN id = 'local' THEN 0 ELSE 1 END, name ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]EnvironmentRecord, 0)
	for rows.Next() {
		item, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func GetNASProfile(environmentID string) (NASProfileRecord, error) {
	if db == nil {
		return NASProfileRecord{}, fmt.Errorf("数据库连接未初始化")
	}
	environmentID = strings.TrimSpace(environmentID)
	if environmentID == "" {
		return NASProfileRecord{}, sql.ErrNoRows
	}
	var record NASProfileRecord
	err := db.QueryRow(`
		SELECT environment_id, COALESCE(profile_json, '{}'),
		       COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM nas_profiles WHERE environment_id = ?
	`, environmentID).Scan(
		&record.EnvironmentID,
		&record.ProfileJSON,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	return record, err
}

func UpsertNASProfile(environmentID string, profileJSON string) (NASProfileRecord, error) {
	if db == nil {
		return NASProfileRecord{}, fmt.Errorf("数据库连接未初始化")
	}
	environmentID = strings.TrimSpace(environmentID)
	if !validEnvironmentID(environmentID) {
		return NASProfileRecord{}, fmt.Errorf("环境 ID 不合法")
	}
	if _, err := GetEnvironment(environmentID); err != nil {
		return NASProfileRecord{}, err
	}
	profileJSON = strings.TrimSpace(profileJSON)
	if profileJSON == "" {
		profileJSON = "{}"
	}
	if !json.Valid([]byte(profileJSON)) {
		return NASProfileRecord{}, fmt.Errorf("NAS 画像不是有效 JSON")
	}
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO nas_profiles (environment_id, profile_json, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(environment_id) DO UPDATE SET
			profile_json = excluded.profile_json,
			updated_at = excluded.updated_at
	`, environmentID, profileJSON, now, now)
	if err != nil {
		return NASProfileRecord{}, err
	}
	return GetNASProfile(environmentID)
}
