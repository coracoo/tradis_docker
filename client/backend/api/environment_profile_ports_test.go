package api

import (
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/require"
)

func TestComposePreflightPortOwnershipIsScopedToResolvedDirectory(t *testing.T) {
	root := t.TempDir()
	target := composeOperationTarget{ProjectDir: filepath.Join(root, "demo"), ComposeProjectName: "demo"}
	owned := types.Container{Labels: map[string]string{"com.docker.compose.project": "demo", "com.docker.compose.project.working_dir": target.ProjectDir}, Ports: []types.Port{{IP: "127.0.0.1", PublicPort: 3000, Type: "tcp"}}}
	for _, scenario := range []string{"own", "foreign", "same_name_other_path", "unknown_path", "udp", "ordinary_deploy"} {
		t.Run(scenario, func(t *testing.T) {
			items := []types.Container{owned}
			owner := target
			wantConflict := false
			switch scenario {
			case "foreign":
				items = append(items, types.Container{Labels: map[string]string{"com.docker.compose.project": "other"}, Ports: []types.Port{{IP: "0.0.0.0", PublicPort: 3000, Type: "tcp"}}})
				wantConflict = true
			case "same_name_other_path":
				items = append(items, types.Container{Labels: map[string]string{"com.docker.compose.project": "demo", "com.docker.compose.project.working_dir": filepath.Join(root, "other")}, Ports: owned.Ports})
				wantConflict = true
			case "unknown_path":
				items = []types.Container{{Labels: map[string]string{"com.docker.compose.project": "demo"}, Ports: owned.Ports}}
				wantConflict = true
			case "udp":
				items = append(items, types.Container{Ports: []types.Port{{PublicPort: 3000, Type: "udp"}}})
			case "ordinary_deploy":
				owner = composeOperationTarget{}
				wantConflict = true
			}
			ports := composePreflightUsedPorts(items, owner)
			_, conflict := ports[portKey(3000, "tcp")]
			require.Equal(t, wantConflict, conflict)
			if scenario == "udp" {
				_, conflict = ports[portKey(3000, "udp")]
				require.True(t, conflict)
			}
		})
	}
}
