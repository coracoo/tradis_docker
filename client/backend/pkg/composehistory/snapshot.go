package composehistory

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"dockerpanel/backend/pkg/secrets"
)

const RetentionLimit = 10

func NewHistoryID() string {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("compose-history-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("compose-history-%d-%s", time.Now().UnixNano(), hex.EncodeToString(random))
}

func SealSnapshot(yamlContent, environmentID, projectName, historyID string) (string, error) {
	if err := validateSnapshotScope(environmentID, projectName, historyID); err != nil {
		return "", err
	}
	return secrets.Seal(yamlContent, snapshotContext(environmentID, projectName, historyID))
}

func OpenSnapshot(sealed, environmentID, projectName, historyID string) (string, error) {
	if err := validateSnapshotScope(environmentID, projectName, historyID); err != nil {
		return "", err
	}
	return secrets.Open(sealed, snapshotContext(environmentID, projectName, historyID))
}

func snapshotContext(environmentID, projectName, historyID string) string {
	return "compose-history:" + strings.TrimSpace(environmentID) + ":" +
		strings.TrimSpace(projectName) + ":" + strings.TrimSpace(historyID)
}

func validateSnapshotScope(environmentID, projectName, historyID string) error {
	if strings.TrimSpace(environmentID) == "" || strings.TrimSpace(projectName) == "" || strings.TrimSpace(historyID) == "" {
		return fmt.Errorf("Compose 历史加密上下文不完整")
	}
	return nil
}
