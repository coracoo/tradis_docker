//go:build community

package api

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommunityComposeSourcesExcludeAgentWorkspaceHooks(t *testing.T) {
	command := exec.Command("go", "list", "-json", "-tags", "community", ".")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list community api package: %v", err)
	}

	var listed struct {
		Dir     string
		GoFiles []string
	}
	if err := json.Unmarshal(output, &listed); err != nil {
		t.Fatalf("decode go list output: %v", err)
	}

	for _, file := range listed.GoFiles {
		if !strings.HasPrefix(file, "compose") && file != "ai_compose_community.go" {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(listed.Dir, file))
		if readErr != nil {
			t.Fatalf("read compiled source %s: %v", file, readErr)
		}
		for _, forbidden := range []string{
			"ai_agent_deploy_compose",
			"shouldReconcileAIWorkspaceOwnership",
			"filterAIComposeDotenv",
			"shouldHideAgentWorkspaceDraft",
		} {
			if strings.Contains(string(content), forbidden) {
				t.Fatalf("community compose source %s unexpectedly contains %q", file, forbidden)
			}
		}
	}
}
