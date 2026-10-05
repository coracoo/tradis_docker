package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ComposeHistoryRecord struct {
	ID             string `json:"id"`
	EnvironmentID  string `json:"environmentId"`
	ProjectName    string `json:"projectName"`
	ComposePath    string `json:"composePath"`
	YAMLHash       string `json:"yamlHash"`
	SnapshotSealed string `json:"-"`
	SummaryJSON    string `json:"-"`
	Source         string `json:"source"`
	CreatedAt      string `json:"createdAt"`
}

func createComposeHistoryTable() error {
	if db == nil {
		return fmt.Errorf("数据库连接未初始化")
	}
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS compose_config_history (
			id TEXT PRIMARY KEY,
			environment_id TEXT NOT NULL DEFAULT 'local',
			project_name TEXT NOT NULL,
			compose_path TEXT NOT NULL,
			yaml_hash TEXT NOT NULL,
			snapshot_sealed TEXT NOT NULL,
			summary_json TEXT NOT NULL DEFAULT '{}',
			source TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_compose_config_history_project
			ON compose_config_history(environment_id, project_name, created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_compose_config_history_hash
			ON compose_config_history(environment_id, project_name, yaml_hash)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func SaveComposeHistory(record ComposeHistoryRecord, limit int) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("数据库连接未初始化")
	}
	record = normalizeComposeHistoryRecord(record)
	if err := validateComposeHistoryRecord(record); err != nil {
		return false, err
	}
	if limit <= 0 {
		limit = 10
	}

	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM compose_config_history
		WHERE environment_id = ? AND project_name = ? AND yaml_hash = ?`,
		record.EnvironmentID, record.ProjectName, record.YAMLHash).Scan(&existing); err != nil {
		return false, err
	}
	if existing > 0 {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}

	if _, err := tx.Exec(`INSERT INTO compose_config_history
		(id, environment_id, project_name, compose_path, yaml_hash, snapshot_sealed, summary_json, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID, record.EnvironmentID, record.ProjectName, record.ComposePath, record.YAMLHash,
		record.SnapshotSealed, record.SummaryJSON, record.Source, record.CreatedAt); err != nil {
		return false, err
	}

	if _, err := tx.Exec(`DELETE FROM compose_config_history WHERE id IN (
		SELECT id FROM compose_config_history
		WHERE environment_id = ? AND project_name = ?
		ORDER BY created_at DESC, id DESC
		LIMIT -1 OFFSET ?
	)`, record.EnvironmentID, record.ProjectName, limit); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// ImportComposeHistory restores project-scoped history as one transaction.
// Existing records are preserved, identical records and YAML hashes are
// skipped, and an ID collision with different content aborts the whole batch.
func ImportComposeHistory(environmentID, projectName string, records []ComposeHistoryRecord, limit int) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("数据库连接未初始化")
	}
	environmentID = normalizeComposeHistoryEnvironment(environmentID)
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return 0, fmt.Errorf("Compose 项目名称不能为空")
	}
	if limit <= 0 {
		limit = 10
	}

	normalized := make([]ComposeHistoryRecord, 0, len(records))
	for _, record := range records {
		record = normalizeComposeHistoryRecord(record)
		if record.EnvironmentID != environmentID || record.ProjectName != projectName {
			return 0, fmt.Errorf("Compose 历史记录范围不匹配")
		}
		if err := validateComposeHistoryRecord(record); err != nil {
			return 0, err
		}
		normalized = append(normalized, record)
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	inserted := 0
	for _, record := range normalized {
		var existing ComposeHistoryRecord
		err := scanComposeHistoryRecord(tx.QueryRow(`SELECT id, environment_id, project_name, compose_path,
			yaml_hash, snapshot_sealed, summary_json, source, COALESCE(created_at, '')
			FROM compose_config_history WHERE id = ?`, record.ID), &existing)
		switch {
		case err == nil:
			if !sameComposeHistoryRecord(existing, record) {
				return 0, fmt.Errorf("Compose 历史记录 ID 冲突: %s", record.ID)
			}
			continue
		case !errors.Is(err, sql.ErrNoRows):
			return 0, err
		}

		var duplicateID string
		err = tx.QueryRow(`SELECT id FROM compose_config_history
			WHERE environment_id = ? AND project_name = ? AND yaml_hash = ? LIMIT 1`,
			record.EnvironmentID, record.ProjectName, record.YAMLHash).Scan(&duplicateID)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}

		if _, err := tx.Exec(`INSERT INTO compose_config_history
			(id, environment_id, project_name, compose_path, yaml_hash, snapshot_sealed, summary_json, source, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			record.ID, record.EnvironmentID, record.ProjectName, record.ComposePath, record.YAMLHash,
			record.SnapshotSealed, record.SummaryJSON, record.Source, record.CreatedAt); err != nil {
			return 0, err
		}
		inserted++
	}

	if _, err := tx.Exec(`DELETE FROM compose_config_history WHERE id IN (
		SELECT id FROM compose_config_history
		WHERE environment_id = ? AND project_name = ?
		ORDER BY created_at DESC, id DESC
		LIMIT -1 OFFSET ?
	)`, environmentID, projectName, limit); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

func sameComposeHistoryRecord(left, right ComposeHistoryRecord) bool {
	return left.ID == right.ID && left.EnvironmentID == right.EnvironmentID &&
		left.ProjectName == right.ProjectName && left.ComposePath == right.ComposePath &&
		left.YAMLHash == right.YAMLHash && left.SnapshotSealed == right.SnapshotSealed &&
		left.SummaryJSON == right.SummaryJSON && left.Source == right.Source &&
		left.CreatedAt == right.CreatedAt
}

func ListComposeHistory(environmentID, projectName string, limit int) ([]ComposeHistoryRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}
	environmentID = normalizeComposeHistoryEnvironment(environmentID)
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return nil, fmt.Errorf("Compose 项目名称不能为空")
	}
	if limit <= 0 {
		limit = 10
	}
	rows, err := db.Query(`SELECT id, environment_id, project_name, compose_path, yaml_hash,
		snapshot_sealed, summary_json, source, COALESCE(created_at, '')
		FROM compose_config_history
		WHERE environment_id = ? AND project_name = ?
		ORDER BY created_at DESC, id DESC LIMIT ?`, environmentID, projectName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []ComposeHistoryRecord{}
	for rows.Next() {
		var record ComposeHistoryRecord
		if err := scanComposeHistoryRecord(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func GetComposeHistory(environmentID, projectName, historyID string) (ComposeHistoryRecord, error) {
	if db == nil {
		return ComposeHistoryRecord{}, fmt.Errorf("数据库连接未初始化")
	}
	var record ComposeHistoryRecord
	err := scanComposeHistoryRecord(db.QueryRow(`SELECT id, environment_id, project_name, compose_path,
		yaml_hash, snapshot_sealed, summary_json, source, COALESCE(created_at, '')
		FROM compose_config_history WHERE environment_id = ? AND project_name = ? AND id = ?`,
		normalizeComposeHistoryEnvironment(environmentID), strings.TrimSpace(projectName), strings.TrimSpace(historyID)), &record)
	return record, err
}

func DeleteComposeHistory(environmentID, projectName, historyID string) error {
	if db == nil {
		return fmt.Errorf("数据库连接未初始化")
	}
	_, err := db.Exec(`DELETE FROM compose_config_history
		WHERE environment_id = ? AND project_name = ? AND id = ?`,
		normalizeComposeHistoryEnvironment(environmentID), strings.TrimSpace(projectName), strings.TrimSpace(historyID))
	return err
}

func DeleteComposeHistoryForProject(environmentID, projectName string) error {
	if db == nil {
		return fmt.Errorf("数据库连接未初始化")
	}
	_, err := db.Exec(`DELETE FROM compose_config_history WHERE environment_id = ? AND project_name = ?`,
		normalizeComposeHistoryEnvironment(environmentID), strings.TrimSpace(projectName))
	return err
}

func PruneComposeHistory(environmentID, projectName string, limit int) error {
	if db == nil {
		return fmt.Errorf("数据库连接未初始化")
	}
	if limit <= 0 {
		limit = 10
	}
	_, err := db.Exec(`DELETE FROM compose_config_history WHERE id IN (
		SELECT id FROM compose_config_history
		WHERE environment_id = ? AND project_name = ?
		ORDER BY created_at DESC, id DESC
		LIMIT -1 OFFSET ?
	)`, normalizeComposeHistoryEnvironment(environmentID), strings.TrimSpace(projectName), limit)
	return err
}

type composeHistoryScanner interface {
	Scan(dest ...any) error
}

func scanComposeHistoryRecord(scanner composeHistoryScanner, record *ComposeHistoryRecord) error {
	return scanner.Scan(&record.ID, &record.EnvironmentID, &record.ProjectName, &record.ComposePath,
		&record.YAMLHash, &record.SnapshotSealed, &record.SummaryJSON, &record.Source, &record.CreatedAt)
}

func normalizeComposeHistoryRecord(record ComposeHistoryRecord) ComposeHistoryRecord {
	record.ID = strings.TrimSpace(record.ID)
	record.EnvironmentID = normalizeComposeHistoryEnvironment(record.EnvironmentID)
	record.ProjectName = strings.TrimSpace(record.ProjectName)
	record.ComposePath = strings.TrimSpace(record.ComposePath)
	record.YAMLHash = strings.TrimSpace(record.YAMLHash)
	record.SnapshotSealed = strings.TrimSpace(record.SnapshotSealed)
	record.Source = strings.TrimSpace(record.Source)
	if record.SummaryJSON == "" || !json.Valid([]byte(record.SummaryJSON)) {
		record.SummaryJSON = `{}`
	}
	if strings.TrimSpace(record.CreatedAt) == "" {
		record.CreatedAt = time.Now().Format("2006-01-02 15:04:05.000000000")
	}
	return record
}

func validateComposeHistoryRecord(record ComposeHistoryRecord) error {
	if record.ID == "" || record.ProjectName == "" || record.ComposePath == "" || record.YAMLHash == "" ||
		record.SnapshotSealed == "" || record.Source == "" {
		return fmt.Errorf("Compose 历史记录字段不完整")
	}
	return nil
}

func normalizeComposeHistoryEnvironment(environmentID string) string {
	if environmentID = strings.TrimSpace(environmentID); environmentID != "" {
		return environmentID
	}
	return "local"
}

var _ composeHistoryScanner = (*sql.Row)(nil)
var _ composeHistoryScanner = (*sql.Rows)(nil)
