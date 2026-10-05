package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRollbackAppStoreDeploymentPreservesProjectWhenComposeDownFails(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := appStoreComposeCommand
	appStoreComposeCommand = func(context.Context, string, []string, []string) ([]byte, error) {
		return []byte("daemon unavailable"), errors.New("down failed")
	}
	t.Cleanup(func() { appStoreComposeCommand = original })

	if err := rollbackAppStoreDeployment(projectDir, nil); err == nil {
		t.Fatal("expected rollback error")
	}
	if _, err := os.Stat(projectDir); err != nil {
		t.Fatalf("project directory should be preserved after failed rollback: %v", err)
	}
}

func TestRollbackAppStoreDeploymentPreservesDataAfterComposeDown(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := appStoreComposeCommand
	appStoreComposeCommand = func(context.Context, string, []string, []string) ([]byte, error) {
		return nil, nil
	}
	t.Cleanup(func() { appStoreComposeCommand = original })
	sentinel := filepath.Join(projectDir, "user-data")
	if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := rollbackAppStoreDeployment(projectDir, nil); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "keep" {
		t.Fatalf("rollback lost generated data: %v", err)
	}
}
