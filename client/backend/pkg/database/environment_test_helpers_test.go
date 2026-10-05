package database

import (
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupEnvironmentTestDB(t *testing.T) string {
	t.Helper()
	_ = Close()
	path := filepath.Join(t.TempDir(), "data.db")
	if err := InitDB(path); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = Close() })
	return path
}
