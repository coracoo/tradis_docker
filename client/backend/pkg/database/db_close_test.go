package database

import (
	"path/filepath"
	"testing"
)

func TestCloseClearsGlobalDatabaseHandle(t *testing.T) {
	if err := InitDB(filepath.Join(t.TempDir(), "data.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	if GetDB() == nil {
		t.Fatal("expected initialized database handle")
	}

	if err := Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if GetDB() != nil {
		t.Fatal("expected Close to clear the global database handle")
	}
}
