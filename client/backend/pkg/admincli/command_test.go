package admincli

import (
	"bytes"
	"dockerpanel/backend/pkg/database"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func setupAdminCommandDatabase(t *testing.T) string {
	t.Helper()
	_ = database.Close()
	t.Setenv("ADMIN_PASSWORD", "old-password")
	dataDir := t.TempDir()
	t.Setenv("TRADIS_DATA_DIR", dataDir)
	if err := database.InitDB(filepath.Join(dataDir, "data.db")); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return dataDir
}

func TestRunResetPassword(t *testing.T) {
	setupAdminCommandDatabase(t)
	var output bytes.Buffer

	handled, err := Run(
		[]string{"admin", "reset-password"},
		strings.NewReader("new-password\nnew-password\n"),
		&output,
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !handled {
		t.Fatal("reset command was not handled")
	}

	var stored string
	if err := database.GetDB().QueryRow("SELECT password FROM users WHERE username = 'admin'").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored), []byte("new-password")); err != nil {
		t.Fatalf("stored password mismatch: %v", err)
	}
	if !strings.Contains(output.String(), "管理员密码已重置") {
		t.Fatalf("missing success output: %q", output.String())
	}
}

func TestRunResetPasswordRejectsConfirmationMismatch(t *testing.T) {
	setupAdminCommandDatabase(t)

	handled, err := Run(
		[]string{"admin", "reset-password"},
		strings.NewReader("new-password\nother-password\n"),
		&bytes.Buffer{},
	)
	if !handled {
		t.Fatal("reset command was not handled")
	}
	if err == nil || !strings.Contains(err.Error(), "两次输入") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
}

func TestRunIgnoresUnrelatedArguments(t *testing.T) {
	handled, err := Run([]string{"serve"}, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if handled {
		t.Fatal("unrelated command was handled")
	}
}
