package deployment

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func validDeploymentCandidate() DeploymentCandidate {
	return DeploymentCandidate{
		Version:       DeploymentCandidateVersion,
		EnvironmentID: "remote-node",
		ProjectName:   "demo",
		SourceType:    DeploymentSourceManualCompose,
		ComposeYAML:   "services:\n  app:\n    image: nginx:alpine\n",
		Options: DeploymentOptions{
			AutoStart: true,
		},
	}
}

func TestNormalizeDeploymentCandidateRejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*DeploymentCandidate)
	}{
		{
			name: "empty environment",
			mutate: func(candidate *DeploymentCandidate) {
				candidate.EnvironmentID = ""
			},
		},
		{
			name: "unsupported source",
			mutate: func(candidate *DeploymentCandidate) {
				candidate.SourceType = "github"
			},
		},
		{
			name: "unsupported version",
			mutate: func(candidate *DeploymentCandidate) {
				candidate.Version = DeploymentCandidateVersion + 1
			},
		},
		{
			name: "compose without services",
			mutate: func(candidate *DeploymentCandidate) {
				candidate.ComposeYAML = "name: demo\n"
			},
		},
		{
			name: "duplicate extra file",
			mutate: func(candidate *DeploymentCandidate) {
				candidate.Files = []DeploymentFile{{Path: "config/app.ini", Content: "a"}, {Path: "config/app.ini", Content: "b"}}
			},
		},
		{
			name: "reserved dotenv file",
			mutate: func(candidate *DeploymentCandidate) {
				candidate.Files = []DeploymentFile{{Path: ".env", Content: "KEY=value"}}
			},
		},
		{
			name: "absolute file",
			mutate: func(candidate *DeploymentCandidate) {
				candidate.Files = []DeploymentFile{{Path: "/etc/app.ini", Content: "a"}}
			},
		},
		{
			name: "traversing file",
			mutate: func(candidate *DeploymentCandidate) {
				candidate.Files = []DeploymentFile{{Path: "../app.ini", Content: "a"}}
			},
		},
		{
			name: "unsupported mode",
			mutate: func(candidate *DeploymentCandidate) {
				candidate.Files = []DeploymentFile{{Path: "config/app.ini", Content: "a", Mode: 0o755}}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := validDeploymentCandidate()
			test.mutate(&candidate)

			_, err := NormalizeDeploymentCandidate(candidate)

			require.Error(t, err)
			require.Contains(t, err.Error(), "deployment candidate")
		})
	}
}

func TestHashDeploymentCandidateCanonicalizesFilesAndIncludesDeploymentInputs(t *testing.T) {
	left := validDeploymentCandidate()
	left.Dotenv = "TAG=stable\n"
	left.Files = []DeploymentFile{
		{Path: "config/a.yml", Content: "a", Mode: 0o600},
		{Path: "config/b.yml", Content: "b"},
	}
	right := left
	right.Files = []DeploymentFile{left.Files[1], left.Files[0]}

	require.Equal(t, HashDeploymentCandidate(left), HashDeploymentCandidate(right))

	changedDotenv := left
	changedDotenv.Dotenv = "TAG=edge\n"
	require.NotEqual(t, HashDeploymentCandidate(left), HashDeploymentCandidate(changedDotenv))

	changedOption := left
	changedOption.Options.Pull = true
	require.NotEqual(t, HashDeploymentCandidate(left), HashDeploymentCandidate(changedOption))
}

func TestNormalizeDeploymentCandidateAssignsAndChecksDigest(t *testing.T) {
	candidate := validDeploymentCandidate()
	normalized, err := NormalizeDeploymentCandidate(candidate)
	require.NoError(t, err)
	require.Len(t, normalized.CandidateHash, 64)

	candidate.CandidateHash = strings.Repeat("0", 64)
	_, err = NormalizeDeploymentCandidate(candidate)
	require.Error(t, err)
	require.Contains(t, err.Error(), "candidate hash")
}

func TestVisibleComposeProjectDirectoryRejectsHiddenEntries(t *testing.T) {
	for _, name := range []string{".tradis-preflight-eDMMIh", ".tradis-deploy-abcd", ".git"} {
		require.False(t, IsVisibleComposeProjectDirectory(name), name)
	}

	for _, name := range []string{"liyuan", "media-library", "好好的11"} {
		require.True(t, IsVisibleComposeProjectDirectory(name), name)
	}
}
