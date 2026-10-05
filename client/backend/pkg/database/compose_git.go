package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type ComposeGitSource struct {
	EnvironmentID  string     `json:"environmentId"`
	ProjectName    string     `json:"projectName"`
	RepoURL        string     `json:"repoUrl"`
	Branch         string     `json:"branch"`
	AcceleratorURL string     `json:"acceleratorUrl"`
	CommitHash     string     `json:"commitHash"`
	ComposePath    string     `json:"composePath"`
	SyncedAt       *time.Time `json:"syncedAt,omitempty"`
}

func UpsertComposeGitSource(source ComposeGitSource) error {
	return UpsertComposeGitSourceInEnvironment(LocalEnvironmentID, source)
}

// UpsertComposeGitSourceInEnvironment stores a Git source inside one Docker
// environment. The legacy helper above remains local-only for compatibility.
func UpsertComposeGitSourceInEnvironment(environmentID string, source ComposeGitSource) error {
	environmentID, err := normalizeComposeGitSourceEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	source.EnvironmentID = environmentID
	source.ProjectName = strings.TrimSpace(source.ProjectName)
	if source.ProjectName == "" {
		return fmt.Errorf("项目名称不能为空")
	}
	_, err = db.Exec(`
		INSERT INTO compose_git_sources (
			environment_id, project_name, repo_url, branch, accelerator_url, commit_hash, compose_path, synced_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(environment_id, project_name) DO UPDATE SET
			repo_url = excluded.repo_url,
			branch = excluded.branch,
			accelerator_url = excluded.accelerator_url,
			commit_hash = excluded.commit_hash,
			compose_path = excluded.compose_path,
			synced_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
	`,
		source.EnvironmentID,
		strings.TrimSpace(source.ProjectName),
		strings.TrimSpace(source.RepoURL),
		strings.TrimSpace(source.Branch),
		strings.TrimSpace(source.AcceleratorURL),
		strings.TrimSpace(source.CommitHash),
		strings.TrimSpace(source.ComposePath),
	)
	return err
}

func GetComposeGitSource(projectName string) (*ComposeGitSource, error) {
	return GetComposeGitSourceInEnvironment(LocalEnvironmentID, projectName)
}

func GetComposeGitSourceInEnvironment(environmentID string, projectName string) (*ComposeGitSource, error) {
	environmentID, err := normalizeComposeGitSourceEnvironmentID(environmentID)
	if err != nil {
		return nil, err
	}
	var source ComposeGitSource
	var branch sql.NullString
	var accelerator sql.NullString
	var commit sql.NullString
	var composePath sql.NullString
	var syncedAt sql.NullTime
	err = db.QueryRow(`
		SELECT environment_id, project_name, repo_url, branch, accelerator_url, commit_hash, compose_path, synced_at
		FROM compose_git_sources
		WHERE environment_id = ? AND project_name = ?
	`, environmentID, strings.TrimSpace(projectName)).Scan(
		&source.EnvironmentID,
		&source.ProjectName,
		&source.RepoURL,
		&branch,
		&accelerator,
		&commit,
		&composePath,
		&syncedAt,
	)
	if err != nil {
		return nil, err
	}
	source.Branch = strings.TrimSpace(branch.String)
	source.AcceleratorURL = strings.TrimSpace(accelerator.String)
	source.CommitHash = strings.TrimSpace(commit.String)
	source.ComposePath = strings.TrimSpace(composePath.String)
	if syncedAt.Valid {
		value := syncedAt.Time
		source.SyncedAt = &value
	}
	return &source, nil
}

func ListComposeGitSources() (map[string]ComposeGitSource, error) {
	return ListComposeGitSourcesInEnvironment(LocalEnvironmentID)
}

func ListComposeGitSourcesInEnvironment(environmentID string) (map[string]ComposeGitSource, error) {
	environmentID, err := normalizeComposeGitSourceEnvironmentID(environmentID)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`
		SELECT environment_id, project_name, repo_url, branch, accelerator_url, commit_hash, compose_path, synced_at
		FROM compose_git_sources
		WHERE environment_id = ?
	`, environmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]ComposeGitSource)
	for rows.Next() {
		var source ComposeGitSource
		var branch sql.NullString
		var accelerator sql.NullString
		var commit sql.NullString
		var composePath sql.NullString
		var syncedAt sql.NullTime
		if err := rows.Scan(
			&source.EnvironmentID,
			&source.ProjectName,
			&source.RepoURL,
			&branch,
			&accelerator,
			&commit,
			&composePath,
			&syncedAt,
		); err != nil {
			return nil, err
		}
		source.Branch = strings.TrimSpace(branch.String)
		source.AcceleratorURL = strings.TrimSpace(accelerator.String)
		source.CommitHash = strings.TrimSpace(commit.String)
		source.ComposePath = strings.TrimSpace(composePath.String)
		if syncedAt.Valid {
			value := syncedAt.Time
			source.SyncedAt = &value
		}
		result[source.ProjectName] = source
	}
	return result, rows.Err()
}

func DeleteComposeGitSource(projectName string) error {
	return DeleteComposeGitSourceInEnvironment(LocalEnvironmentID, projectName)
}

func DeleteComposeGitSourceInEnvironment(environmentID string, projectName string) error {
	environmentID, err := normalizeComposeGitSourceEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	_, err = db.Exec("DELETE FROM compose_git_sources WHERE environment_id = ? AND project_name = ?", environmentID, strings.TrimSpace(projectName))
	return err
}

func normalizeComposeGitSourceEnvironmentID(environmentID string) (string, error) {
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

// ensureComposeGitSourceEnvironmentKey upgrades the original project_name-only
// primary key. Keeping that key would make same-named projects from two NAS
// environments overwrite each other before Node Agent support is enabled.
func ensureComposeGitSourceEnvironmentKey() error {
	rows, err := db.Query(`PRAGMA table_info(compose_git_sources)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	primaryKeyOrder := map[string]int{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		primaryKeyOrder[name] = primaryKey
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if primaryKeyOrder["environment_id"] == 1 && primaryKeyOrder["project_name"] == 2 {
		_, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_compose_git_sources_environment ON compose_git_sources(environment_id, updated_at DESC)`)
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
		CREATE TABLE compose_git_sources_next (
			environment_id TEXT NOT NULL DEFAULT 'local',
			project_name TEXT NOT NULL,
			repo_url TEXT NOT NULL,
			branch TEXT,
			accelerator_url TEXT,
			commit_hash TEXT,
			compose_path TEXT,
			synced_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (environment_id, project_name)
		)
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO compose_git_sources_next (
			environment_id, project_name, repo_url, branch, accelerator_url, commit_hash, compose_path, synced_at, created_at, updated_at
		)
		SELECT COALESCE(NULLIF(environment_id, ''), 'local'), project_name, COALESCE(repo_url, ''),
		       branch, accelerator_url, commit_hash, COALESCE(compose_path, ''), synced_at, created_at, updated_at
		FROM compose_git_sources
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE compose_git_sources`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE compose_git_sources_next RENAME TO compose_git_sources`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX idx_compose_git_sources_environment ON compose_git_sources(environment_id, updated_at DESC)`); err != nil {
		return err
	}
	return tx.Commit()
}
