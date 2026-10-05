//go:build community

package database

import (
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommunityDatabaseExcludesCommercialTables(t *testing.T) {
	_ = Close()
	if err := InitDB(filepath.Join(t.TempDir(), "community.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close() })

	for _, table := range []string{
		"ai_agent_sessions",
		"ai_agent_runs",
		"remote_agent_authorities",
		"remote_deployments",
		"protection_policies",
		"update_runs",
		"tutorial_assets",
		"github_app_subscriptions",
	} {
		if communityTableExists(t, table) {
			t.Fatalf("community database unexpectedly created commercial table %q", table)
		}
	}
	for _, table := range []string{
		"users",
		"compose_project_metadata",
		"deployment_jobs",
		"scheduled_jobs",
		"notification_channels",
		"ai_logs",
	} {
		if !communityTableExists(t, table) {
			t.Fatalf("community database did not create required table %q", table)
		}
	}
}

func TestCommunityBuildExcludesCommercialPersistenceSource(t *testing.T) {
	command := exec.Command("go", "list", "-json", "-tags", "community", ".")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list community database package: %v", err)
	}

	var listed struct {
		Dir     string
		GoFiles []string
	}
	if err := json.Unmarshal(output, &listed); err != nil {
		t.Fatalf("decode go list output: %v", err)
	}

	for _, file := range listed.GoFiles {
		content, readErr := os.ReadFile(filepath.Join(listed.Dir, file))
		if readErr != nil {
			t.Fatalf("read compiled source %s: %v", file, readErr)
		}
		for _, forbidden := range []string{
			"type AgentRunRecord",
			"func CreateAgentRun",
			"ai_agent_",
			"github_app_",
			"remote_agent_",
			"protection_policies",
			"tutorial_assets",
			"update_runs",
		} {
			if strings.Contains(string(content), forbidden) {
				t.Fatalf("community source %s unexpectedly contains %q", file, forbidden)
			}
		}
	}
}

func communityTableExists(t *testing.T, table string) bool {
	t.Helper()
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true
}
