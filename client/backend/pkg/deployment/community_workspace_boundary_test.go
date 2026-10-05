//go:build community

package deployment

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestCommunityBuildExcludesWorkspaceExecutionSources(t *testing.T) {
	command := exec.Command("go", "list", "-json", "-tags", "community", ".")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list community deployment package: %v", err)
	}

	var listed struct {
		GoFiles []string
	}
	if err := json.Unmarshal(output, &listed); err != nil {
		t.Fatalf("decode go list output: %v", err)
	}

	for _, forbidden := range []string{
		"workspace.go",
		"workspace_service.go",
		"workspace_runner.go",
		"workspace_runner_docker.go",
	} {
		for _, file := range listed.GoFiles {
			if file == forbidden {
				t.Fatalf("community build unexpectedly compiles %s", forbidden)
			}
		}
	}
}
