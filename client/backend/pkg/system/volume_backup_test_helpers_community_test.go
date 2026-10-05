//go:build community

package system

import (
	"database/sql"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
	"path/filepath"
	"testing"

	dockerclient "github.com/docker/docker/client"
)

func setVolumeBackupDockerClientForTest(fn func() (*docker.Client, error)) func() {
	volumeBackupDockerClientMu.Lock()
	previous := newVolumeBackupDockerClient
	newVolumeBackupDockerClient = fn
	volumeBackupDockerClientMu.Unlock()
	return func() {
		volumeBackupDockerClientMu.Lock()
		newVolumeBackupDockerClient = previous
		volumeBackupDockerClientMu.Unlock()
	}
}

func setVolumeBackupRuntimeHooksForTest(update func(*volumeBackupRuntimeHooks)) func() {
	volumeBackupHooksMu.Lock()
	previous := volumeBackupHooks
	update(&volumeBackupHooks)
	volumeBackupHooksMu.Unlock()
	return func() {
		volumeBackupHooksMu.Lock()
		volumeBackupHooks = previous
		volumeBackupHooksMu.Unlock()
	}
}

func newVolumeBackupTestDockerClient(t *testing.T) *docker.Client {
	t.Helper()
	client, err := dockerclient.NewClientWithOpts(
		dockerclient.WithHost("unix:///tmp/tradis-volume-backup-test.sock"),
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return &docker.Client{Client: client}
}

func initSettingsBackgroundTestDB(t *testing.T) *sql.DB {
	t.Helper()
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := database.InitDB(filepath.Join(t.TempDir(), "settings-background.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	return database.GetDB()
}
