package database

import (
	"database/sql"
	"fmt"
	"strings"
)

func normalizeComposeMetadataScope(environmentID, projectName string) (string, string, error) {
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
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return "", "", fmt.Errorf("项目名称不能为空")
	}
	return environmentID, projectName, nil
}

func SetComposeProjectRemark(environmentID, projectName, remark string) error {
	environmentID, projectName, err := normalizeComposeMetadataScope(environmentID, projectName)
	if err != nil {
		return err
	}
	remark = strings.TrimSpace(remark)
	if remark == "" {
		return DeleteComposeProjectMetadata(environmentID, projectName)
	}
	_, err = db.Exec(`
		INSERT INTO compose_project_metadata (environment_id, project_name, remark, created_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(environment_id, project_name) DO UPDATE SET
			remark = excluded.remark,
			updated_at = CURRENT_TIMESTAMP
	`, environmentID, projectName, remark)
	return err
}

func GetComposeProjectRemark(environmentID, projectName string) (string, error) {
	environmentID, projectName, err := normalizeComposeMetadataScope(environmentID, projectName)
	if err != nil {
		return "", err
	}
	var remark string
	err = db.QueryRow(`
		SELECT remark FROM compose_project_metadata
		WHERE environment_id = ? AND project_name = ?
	`, environmentID, projectName).Scan(&remark)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return strings.TrimSpace(remark), err
}

func ListComposeProjectRemarks(environmentID string) (map[string]string, error) {
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
		SELECT project_name, remark FROM compose_project_metadata
		WHERE environment_id = ?
	`, environmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var projectName, remark string
		if err := rows.Scan(&projectName, &remark); err != nil {
			return nil, err
		}
		result[projectName] = strings.TrimSpace(remark)
	}
	return result, rows.Err()
}

func DeleteComposeProjectMetadata(environmentID, projectName string) error {
	environmentID, projectName, err := normalizeComposeMetadataScope(environmentID, projectName)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		DELETE FROM compose_project_metadata
		WHERE environment_id = ? AND project_name = ?
	`, environmentID, projectName)
	return err
}
