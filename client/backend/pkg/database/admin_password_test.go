package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func TestValidateAdminPassword(t *testing.T) {
	if err := ValidateAdminPassword("short"); err == nil {
		t.Fatal("expected short password to be rejected")
	}
	if err := ValidateAdminPassword(strings.Repeat("x", 129)); err == nil {
		t.Fatal("expected oversized password to be rejected")
	}
	if err := ValidateAdminPassword(strings.Repeat("x", 73)); err == nil {
		t.Fatal("expected password longer than bcrypt's 72-byte limit to be rejected")
	}
	if err := ValidateAdminPassword("12345678"); err != nil {
		t.Fatalf("expected valid password: %v", err)
	}
}

func TestResetAdminPasswordAtStoresBcryptHash(t *testing.T) {
	_ = Close()
	t.Setenv("ADMIN_PASSWORD", "old-password")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	if err := Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := ResetAdminPasswordAt(dbPath, "new-password"); err != nil {
		t.Fatalf("ResetAdminPasswordAt() error = %v", err)
	}

	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var stored string
	if err := conn.QueryRow("SELECT password FROM users WHERE username = 'admin'").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == "new-password" {
		t.Fatal("password was stored as plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored), []byte("new-password")); err != nil {
		t.Fatalf("stored hash does not match new password: %v", err)
	}
}

func TestResetAdminPasswordAtRequiresExistingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")
	if err := ResetAdminPasswordAt(dbPath, "new-password"); err == nil {
		t.Fatal("expected missing database error")
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("reset command unexpectedly created database: %v", err)
	}
}

func TestResetAdminPasswordAtMigratesLegacyUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = conn.Exec("CREATE TABLE users (username TEXT PRIMARY KEY, password TEXT, updated_at DATETIME); INSERT INTO users(username,password) VALUES ('admin','old')")
	if err != nil {
		t.Fatal(err)
	}
	if err := ResetAdminPasswordAt(path, "new-password"); err != nil {
		t.Fatal(err)
	}
	var version int
	if err := conn.QueryRow("SELECT token_version FROM users WHERE username = 'admin'").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("token version = %d, want 2", version)
	}
}

func TestResetAdminPasswordAtRequiresAdminUser(t *testing.T) {
	_ = Close()
	t.Setenv("ADMIN_PASSWORD", "old-password")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	if _, err := GetDB().Exec("DELETE FROM users WHERE username = 'admin'"); err != nil {
		t.Fatal(err)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}

	if err := ResetAdminPasswordAt(dbPath, "new-password"); err == nil {
		t.Fatal("expected missing admin user error")
	}
}
