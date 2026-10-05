package composehistory

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestSnapshotRoundTripUsesScopedEncryptionContext(t *testing.T) {
	configureSnapshotTestKey(t)
	yaml := "services:\n  web:\n    image: private.example/app:latest\n"

	sealed, err := SealSnapshot(yaml, "local", "demo", "history-1")
	if err != nil {
		t.Fatalf("SealSnapshot() error = %v", err)
	}
	if strings.Contains(sealed, "private.example") || strings.Contains(sealed, "services") {
		t.Fatalf("sealed snapshot contains plaintext YAML: %q", sealed)
	}

	opened, err := OpenSnapshot(sealed, "local", "demo", "history-1")
	if err != nil {
		t.Fatalf("OpenSnapshot() error = %v", err)
	}
	if opened != yaml {
		t.Fatalf("OpenSnapshot() = %q, want %q", opened, yaml)
	}
}

func TestSnapshotRejectsDifferentScope(t *testing.T) {
	configureSnapshotTestKey(t)
	sealed, err := SealSnapshot("services:\n  web:\n    image: nginx\n", "local", "demo", "history-1")
	if err != nil {
		t.Fatal(err)
	}

	for _, scope := range []struct {
		environment string
		project     string
		history     string
	}{
		{environment: "remote", project: "demo", history: "history-1"},
		{environment: "local", project: "other", history: "history-1"},
		{environment: "local", project: "demo", history: "history-2"},
	} {
		if _, err := OpenSnapshot(sealed, scope.environment, scope.project, scope.history); err == nil {
			t.Fatalf("OpenSnapshot accepted wrong scope: %#v", scope)
		}
	}
}

func TestNewHistoryIDIsNonEmptyAndUnique(t *testing.T) {
	first := NewHistoryID()
	second := NewHistoryID()
	if first == "" || second == "" || first == second {
		t.Fatalf("history IDs are not unique: %q %q", first, second)
	}
}

func configureSnapshotTestKey(t *testing.T) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	t.Setenv("TRADIS_MASTER_KEY", base64.StdEncoding.EncodeToString(key))
	t.Setenv("TRADIS_MASTER_KEY_FILE", "")
	t.Setenv("TRADIS_PREVIOUS_MASTER_KEYS", "")
}
