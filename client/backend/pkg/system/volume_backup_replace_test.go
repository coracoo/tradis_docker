package system

import (
	"context"
	"errors"
	"testing"

	"dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/settings"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/require"
)

func TestVolumeBackupReplacementPreservesOldOnCreateFailure(t *testing.T) {
	initSettingsBackgroundTestDB(t)
	t.Cleanup(setVolumeBackupDockerClientForTest(func() (*docker.Client, error) { return newVolumeBackupTestDockerClient(t), nil }))
	var removed []string
	created := false
	t.Cleanup(setVolumeBackupRuntimeHooksForTest(func(hooks *volumeBackupRuntimeHooks) {
		hooks.ensureImageReady = func(context.Context, *docker.Client, string) error { return nil }
		hooks.findContainer = func(context.Context, *docker.Client) (string, string, bool) { return "old", "old-hash", true }
		hooks.createContainer = func(context.Context, *docker.Client, *container.Config, *container.HostConfig, string) (container.CreateResponse, error) {
			created = true
			return container.CreateResponse{}, errors.New("create failed")
		}
		hooks.removeContainer = func(_ context.Context, _ *docker.Client, id string) error { removed = append(removed, id); return nil }
		hooks.saveNotification = func(string, string) {}
		hooks.renameContainer = func(context.Context, *docker.Client, string, string) error { return nil }
		hooks.stopContainer = func(context.Context, *docker.Client, string) error { return nil }
	}))
	EnsureVolumeBackupContainerContext(context.Background(), settings.Settings{VolumeBackupEnabled: true, VolumeBackupVolumes: []string{"data"}})
	require.True(t, created)
	require.NotContains(t, removed, "old")
}

func TestVolumeBackupReplacementRecoveryAndSuccess(t *testing.T) {
	for _, mode := range []string{"success", "start-failure", "canceled", "cleanup-failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var actions []string
			hooks := volumeBackupRuntimeHooks{
				renameContainer: func(ctx context.Context, _ *docker.Client, id, name string) error {
					require.NoError(t, ctx.Err())
					actions = append(actions, "rename:"+id+":"+name)
					return nil
				},
				createContainer: func(context.Context, *docker.Client, *container.Config, *container.HostConfig, string) (container.CreateResponse, error) {
					actions = append(actions, "create:new")
					return container.CreateResponse{ID: "new"}, nil
				},
				stopContainer: func(context.Context, *docker.Client, string) error { actions = append(actions, "stop:old"); return nil },
				startContainer: func(ctx context.Context, _ *docker.Client, id string) error {
					require.NoError(t, ctx.Err())
					actions = append(actions, "start:"+id)
					if id == "new" && mode == "canceled" {
						cancel()
						return context.Canceled
					}
					if id == "new" && mode == "start-failure" {
						return errors.New("start failed")
					}
					return nil
				},
				removeContainer: func(ctx context.Context, _ *docker.Client, id string) error {
					require.NoError(t, ctx.Err())
					actions = append(actions, "remove:"+id)
					if mode == "cleanup-failure" {
						return errors.New("cleanup failed")
					}
					return nil
				},
			}
			err := replaceVolumeBackupContainer(ctx, nil, hooks, "old", true, nil, nil)
			prefix := []string{"rename:old:" + volumeBackupContainerName + "-previous-old", "create:new", "stop:old", "start:new"}
			if mode == "success" || mode == "cleanup-failure" {
				require.NoError(t, err)
				require.Equal(t, append(prefix, "remove:old"), actions)
			} else {
				require.Error(t, err)
				require.Equal(t, append(prefix, "remove:new", "rename:old:"+volumeBackupContainerName, "start:old"), actions)
			}
		})
	}
}
