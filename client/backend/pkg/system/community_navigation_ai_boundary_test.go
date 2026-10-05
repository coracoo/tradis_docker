//go:build community

package system

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestCommunityBuildIncludesFreeNavigationAI(t *testing.T) {
	command := exec.Command("go", "list", "-json", "-tags", "community", ".")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list community system package: %v", err)
	}

	var listed struct {
		GoFiles []string
	}
	if err := json.Unmarshal(output, &listed); err != nil {
		t.Fatalf("decode go list output: %v", err)
	}

	compiled := make(map[string]bool, len(listed.GoFiles))
	for _, file := range listed.GoFiles {
		compiled[file] = true
	}
	for _, file := range []string{"discovery_navigation_ai_full.go", "discovery_edition_full.go"} {
		if !compiled[file] {
			t.Fatalf("free AI source %s is missing from community build", file)
		}
	}
}
