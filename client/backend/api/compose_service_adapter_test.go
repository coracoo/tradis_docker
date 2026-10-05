package api

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dockerpanel/backend/pkg/database"
	"github.com/stretchr/testify/require"
)

func TestLocalComposeRuntimeDotenvCLIContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "compose", "version").Run(); err != nil {
		t.Skip("Docker Compose CLI is unavailable")
	}
	t.Setenv("TRADIS_QUOTED", "backend-value")
	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	require.NoError(t, os.WriteFile(composePath, []byte("name: dotenv-contract\nservices:\n  app:\n    image: busybox\n    environment:\n      QUOTED: ${TRADIS_QUOTED}\n      LITERAL: ${TRADIS_LITERAL}\n      EXPANDED: ${TRADIS_EXPANDED}\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("TRADIS_QUOTED=\"hello world\" # comment\nTRADIS_LITERAL='${TRADIS_QUOTED}'\nTRADIS_EXPANDED=${TRADIS_QUOTED}\n"), 0600))
	runtime := localComposeServiceRuntime{}
	require.NoError(t, runtime.ValidateConfig(ctx, dir, composePath, nil))
	var lines []string
	require.NoError(t, runtime.Run(ctx, dir, nil, []string{"compose", "config", "--format", "json"}, func(line string) { lines = append(lines, line) }))
	var config struct {
		Services map[string]struct{ Environment map[string]string }
	}
	require.NoError(t, json.Unmarshal([]byte(strings.Join(lines, "\n")), &config))
	env := config.Services["app"].Environment
	require.Equal(t, "hello world", env["QUOTED"])
	require.Equal(t, "hello world", env["EXPANDED"])
	// Compose versions may escape the literal dollar again when serializing config.
	require.Contains(t, []string{"${TRADIS_QUOTED}", "$${TRADIS_QUOTED}"}, env["LITERAL"])
}

func TestLocalComposeRuntimePassesExplicitEnvironment(t *testing.T) {
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nprintf '%s\\n' \"$TRADIS_ENV_PROBE\"\n"), 0700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  app:\n    image: busybox\n"), 0600))
	var lines []string
	err := (localComposeServiceRuntime{}).Run(context.Background(), dir, []string{"TRADIS_ENV_PROBE=explicit"}, []string{"compose", "up", "-d"}, func(line string) { lines = append(lines, line) })
	require.NoError(t, err)
	require.Contains(t, lines, "explicit")
}

func TestLocalComposeSinkRetriesPersistenceWithoutChangingOutcome(t *testing.T) {
	setupComposeDeployContractTest(t)
	sink := &localComposeTaskSink{taskID: "finish-retry", taskType: "compose_deploy", projectName: "demo"}
	require.NoError(t, sink.SetRunning())
	_, err := database.GetDB().Exec(`CREATE TRIGGER fail_finish BEFORE UPDATE ON tasks BEGIN SELECT RAISE(FAIL, 'write unavailable'); END`)
	require.NoError(t, err)
	result := map[string]any{"project": "demo", "verified": true}
	require.Error(t, sink.Finish("success", result, ""))
	require.False(t, sink.finished)
	_, err = database.GetDB().Exec(`DROP TRIGGER fail_finish`)
	require.NoError(t, err)
	require.NoError(t, sink.Finish("success", result, ""))
	require.True(t, sink.finished)
	task, err := database.GetTask(sink.taskID)
	require.NoError(t, err)
	require.Equal(t, "success", task.Status)
}

func TestLocalComposeSinkNotificationFailureDoesNotChangeOutcome(t *testing.T) {
	setupComposeDeployContractTest(t)
	sink := &localComposeTaskSink{taskID: "notify-failure", taskType: "compose_deploy", projectName: "demo"}
	require.NoError(t, sink.SetRunning())
	_, err := database.GetDB().Exec(`CREATE TRIGGER fail_notify BEFORE INSERT ON notifications BEGIN SELECT RAISE(FAIL, 'notification unavailable'); END`)
	require.NoError(t, err)
	require.NoError(t, sink.Finish("success", nil, ""))
	require.True(t, sink.finished)
	task, err := database.GetTask(sink.taskID)
	require.NoError(t, err)
	require.Equal(t, "success", task.Status)
}
