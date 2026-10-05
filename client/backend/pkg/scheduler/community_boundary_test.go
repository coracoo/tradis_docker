//go:build community

package scheduler

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommunityBuildExcludesProtectionScheduledTaskImplementation(t *testing.T) {
	command := exec.Command("go", "list", "-json", "-tags", "community", ".")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list community scheduler package: %v", err)
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
			"protection_backup",
			"ProtectionBackup",
			"RegisterProtectionBackupRunner",
		} {
			if strings.Contains(string(content), forbidden) {
				t.Fatalf("community source %s unexpectedly contains %q", file, forbidden)
			}
		}
	}
}
