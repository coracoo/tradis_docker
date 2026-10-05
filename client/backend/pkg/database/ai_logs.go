package database

import (
	"dockerpanel/backend/pkg/logging"
	"strings"
)

func AppendAILog(scope string, level string, message string, details string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(
		"INSERT INTO ai_logs (scope, level, message, details) VALUES (?, ?, ?, ?)",
		strings.TrimSpace(scope),
		strings.TrimSpace(level),
		logging.RedactText(strings.TrimSpace(message)),
		logging.RedactText(strings.TrimSpace(details)),
	)
	return err
}
