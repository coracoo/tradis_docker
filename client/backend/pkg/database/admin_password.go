package database

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func ValidateAdminPassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("new password must be at least 8 characters")
	}
	if len([]byte(password)) > 72 {
		return fmt.Errorf("new password must not exceed 72 bytes")
	}
	return nil
}

func ResetAdminPasswordAt(dbPath, password string) error {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return fmt.Errorf("database path is required")
	}
	if err := ValidateAdminPassword(password); err != nil {
		return err
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		return fmt.Errorf("open existing database: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("database path is a directory: %s", dbPath)
	}

	conn, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer conn.Close()
	if err := conn.Ping(); err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	// The recovery command also works before the upgraded server has started.
	var versionColumn int
	if err := conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'token_version'").Scan(&versionColumn); err != nil {
		return fmt.Errorf("inspect user schema: %w", err)
	}
	if versionColumn == 0 {
		if _, err := conn.Exec("ALTER TABLE users ADD COLUMN token_version INTEGER NOT NULL DEFAULT 1"); err != nil {
			return fmt.Errorf("upgrade user schema: %w", err)
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	result, err := conn.Exec(
		"UPDATE users SET password = ?, token_version = token_version + 1, updated_at = CURRENT_TIMESTAMP WHERE username = 'admin'",
		string(hash),
	)
	if err != nil {
		return fmt.Errorf("update admin password: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read update result: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("admin user not found")
	}
	return nil
}
