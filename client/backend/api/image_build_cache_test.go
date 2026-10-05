package api

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"dockerpanel/backend/pkg/docker"

	"github.com/docker/docker/api/types"
)

func TestDockerBuildCacheDiskUsageFallsBackAfterFilteredEOF(t *testing.T) {
	calls := make([]types.DiskUsageOptions, 0, 2)
	expected := types.DiskUsage{
		BuildCache: []*types.BuildCache{{ID: "cache-1", Size: 4096}},
	}
	client := &docker.MockDockerClient{
		DiskUsageFunc: func(_ context.Context, options types.DiskUsageOptions) (types.DiskUsage, error) {
			calls = append(calls, options)
			if len(calls) == 1 {
				return types.DiskUsage{}, io.EOF
			}
			return expected, nil
		},
	}

	usage, err := dockerBuildCacheDiskUsage(context.Background(), client)
	if err != nil {
		t.Fatalf("dockerBuildCacheDiskUsage returned error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("DiskUsage calls = %d, want 2", len(calls))
	}
	if len(calls[0].Types) != 1 || calls[0].Types[0] != types.BuildCacheObject {
		t.Fatalf("first request types = %#v, want build-cache filter", calls[0].Types)
	}
	if len(calls[1].Types) != 0 {
		t.Fatalf("fallback request types = %#v, want unfiltered request", calls[1].Types)
	}
	if len(usage.BuildCache) != 1 || usage.BuildCache[0].ID != "cache-1" {
		t.Fatalf("unexpected fallback usage: %#v", usage.BuildCache)
	}
}

func TestDockerBuildCacheDiskUsageReturnsBothCompatibilityErrors(t *testing.T) {
	client := &docker.MockDockerClient{
		DiskUsageFunc: func(_ context.Context, options types.DiskUsageOptions) (types.DiskUsage, error) {
			if len(options.Types) > 0 {
				return types.DiskUsage{}, io.EOF
			}
			return types.DiskUsage{}, errors.New("daemon unavailable")
		},
	}

	_, err := dockerBuildCacheDiskUsage(context.Background(), client)
	if err == nil {
		t.Fatal("expected compatibility error")
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("error should preserve filtered EOF: %v", err)
	}
	if got := err.Error(); got == "" || !containsAll(got, "filtered", "fallback", "daemon unavailable") {
		t.Fatalf("error lacks compatibility context: %v", err)
	}
}

func TestDockerBuildCacheDiskUsageDoesNotFallbackForOperationalError(t *testing.T) {
	calls := 0
	permissionErr := errors.New("permission denied while connecting to Docker")
	client := &docker.MockDockerClient{
		DiskUsageFunc: func(_ context.Context, _ types.DiskUsageOptions) (types.DiskUsage, error) {
			calls++
			return types.DiskUsage{}, permissionErr
		},
	}

	_, err := dockerBuildCacheDiskUsage(context.Background(), client)
	if !errors.Is(err, permissionErr) {
		t.Fatalf("expected original operational error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("DiskUsage calls = %d, want 1", calls)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
