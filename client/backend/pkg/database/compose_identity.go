package database

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var storedComposeProjectNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type ComposeProjectIdentity struct {
	EnvironmentID      string    `json:"environmentId"`
	RelativePath       string    `json:"relativePath"`
	ComposeProjectName string    `json:"composeProjectName"`
	Source             string    `json:"source"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func normalizeComposeIdentityScope(environmentID, relativePath string) (string, string, error) {
	environmentID = strings.TrimSpace(environmentID)
	if environmentID == "" {
		environmentID = LocalEnvironmentID
	}
	if !validEnvironmentID(environmentID) {
		return "", "", fmt.Errorf("环境 ID 不合法")
	}
	if _, err := GetEnvironment(environmentID); err != nil {
		return "", "", err
	}
	relativePath = strings.TrimSpace(strings.ReplaceAll(relativePath, `\`, "/"))
	if relativePath == "" || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, "../") || strings.HasPrefix(relativePath, "/") {
		return "", "", fmt.Errorf("项目相对路径不合法")
	}
	return environmentID, relativePath, nil
}

func UpsertComposeProjectIdentity(record ComposeProjectIdentity) error {
	environmentID, relativePath, err := normalizeComposeIdentityScope(record.EnvironmentID, record.RelativePath)
	if err != nil {
		return err
	}
	projectName := strings.TrimSpace(record.ComposeProjectName)
	if !storedComposeProjectNamePattern.MatchString(projectName) {
		return fmt.Errorf("Compose 项目名不合法")
	}
	source := strings.TrimSpace(record.Source)
	switch source {
	case "docker_label", "dotenv", "yaml", "directory", "generated", "restore_manifest":
	default:
		return fmt.Errorf("Compose 项目身份来源不合法")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var conflictingPath string
	err = tx.QueryRow(`
		SELECT relative_path
		FROM compose_project_identities
		WHERE environment_id = ? AND compose_project_name = ? AND relative_path <> ?
		LIMIT 1
	`, environmentID, projectName, relativePath).Scan(&conflictingPath)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		return fmt.Errorf("Compose 项目名 %s 已绑定目录 %s", projectName, conflictingPath)
	}
	if _, err = tx.Exec(`
		INSERT INTO compose_project_identities (
			environment_id, relative_path, compose_project_name, source, created_at, updated_at
		) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(environment_id, relative_path) DO UPDATE SET
			compose_project_name = excluded.compose_project_name,
			source = excluded.source,
			updated_at = CURRENT_TIMESTAMP
	`, environmentID, relativePath, projectName, source); err != nil {
		return err
	}
	return tx.Commit()
}

func GetComposeProjectIdentity(environmentID, relativePath string) (ComposeProjectIdentity, bool, error) {
	environmentID, relativePath, err := normalizeComposeIdentityScope(environmentID, relativePath)
	if err != nil {
		return ComposeProjectIdentity{}, false, err
	}
	var record ComposeProjectIdentity
	err = db.QueryRow(`
		SELECT environment_id, relative_path, compose_project_name, source, created_at, updated_at
		FROM compose_project_identities
		WHERE environment_id = ? AND relative_path = ?
	`, environmentID, relativePath).Scan(
		&record.EnvironmentID,
		&record.RelativePath,
		&record.ComposeProjectName,
		&record.Source,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return ComposeProjectIdentity{}, false, nil
	}
	return record, err == nil, err
}

func ListComposeProjectIdentities(environmentID string) ([]ComposeProjectIdentity, error) {
	environmentID = strings.TrimSpace(environmentID)
	if environmentID == "" {
		environmentID = LocalEnvironmentID
	}
	if !validEnvironmentID(environmentID) {
		return nil, fmt.Errorf("环境 ID 不合法")
	}
	if _, err := GetEnvironment(environmentID); err != nil {
		return nil, err
	}
	rows, err := db.Query(`
		SELECT environment_id, relative_path, compose_project_name, source, created_at, updated_at
		FROM compose_project_identities
		WHERE environment_id = ?
		ORDER BY relative_path
	`, environmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]ComposeProjectIdentity, 0)
	for rows.Next() {
		var record ComposeProjectIdentity
		if err := rows.Scan(
			&record.EnvironmentID,
			&record.RelativePath,
			&record.ComposeProjectName,
			&record.Source,
			&record.CreatedAt,
			&record.UpdatedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func DeleteComposeProjectIdentity(environmentID, relativePath string) error {
	environmentID, relativePath, err := normalizeComposeIdentityScope(environmentID, relativePath)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		DELETE FROM compose_project_identities
		WHERE environment_id = ? AND relative_path = ?
	`, environmentID, relativePath)
	return err
}
