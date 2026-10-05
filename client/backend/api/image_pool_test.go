package api

import (
	"context"
	"testing"
	"time"

	"dockerpanel/backend/pkg/docker"
	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/require"
)

func TestContainersUsingImageHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cli := &docker.MockDockerClient{ContainerListFunc: func(ctx context.Context, opts types.ContainerListOptions) ([]types.Container, error) {
		require.True(t, opts.All)
		return nil, ctx.Err()
	}}
	_, err := containersUsingImage(ctx, cli, "sha256:abc", "")
	require.ErrorIs(t, err, context.Canceled)
}

func TestContainersUsingImageHasBoundedDeadline(t *testing.T) {
	cli := &docker.MockDockerClient{ContainerListFunc: func(ctx context.Context, opts types.ContainerListOptions) ([]types.Container, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.InDelta(t, 30, time.Until(deadline).Seconds(), 1)
		return []types.Container{
			{ID: "used", ImageID: "sha256:abc", Names: []string{"/example"}},
			{ID: "unrelated", ImageID: "sha256:def"},
		}, nil
	}}
	used, err := containersUsingImage(context.Background(), cli, "abc", "")
	require.NoError(t, err)
	require.Len(t, used, 1)
	require.Equal(t, "example", used[0]["name"])
}

func TestContainersUsingImagePreservesShorterDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	wanted, _ := ctx.Deadline()
	cli := &docker.MockDockerClient{ContainerListFunc: func(ctx context.Context, _ types.ContainerListOptions) ([]types.Container, error) {
		deadline, _ := ctx.Deadline()
		require.Equal(t, wanted, deadline)
		return nil, nil
	}}
	_, err := containersUsingImage(ctx, cli, "abc", "")
	require.NoError(t, err)
}
