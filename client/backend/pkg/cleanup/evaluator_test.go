package cleanup

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
)

func TestEvaluateClassifiesCandidatesWithoutDuplicates(t *testing.T) {
	snapshot := Snapshot{
		EvaluatedAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.Local),
		DiskUsage: types.DiskUsage{
			Containers: []*types.Container{
				{ID: "running", ImageID: "used-image", State: "running"},
				{ID: "stopped", ImageID: "stopped-image", State: "exited", SizeRw: 4096},
			},
			Images: []*types.ImageSummary{
				{ID: "used-image", RepoTags: []string{"demo:latest"}, Size: 100},
				{ID: "stopped-image", RepoTags: []string{"stopped:latest"}, Size: 200},
				{ID: "dangling", RepoTags: []string{"<none>:<none>"}, Size: 300},
				{ID: "unused", RepoTags: []string{"unused:latest"}, Size: 400},
			},
			BuildCache: []*types.BuildCache{
				{ID: "cache-free", InUse: false, Size: 500},
				{ID: "cache-active", InUse: true, Size: 600},
			},
		},
		Networks: []types.NetworkResource{
			{ID: "bridge", Name: "bridge"},
			{ID: "free-net", Name: "demo-net", Containers: map[string]types.EndpointResource{}},
		},
	}

	got := Evaluate(snapshot)
	requireCandidate(t, got, CategoryDanglingImage, "dangling")
	requireCandidate(t, got, CategoryUnusedImage, "unused")
	requireCandidate(t, got, CategoryStoppedContainer, "stopped")
	requireCandidate(t, got, CategoryUnusedNetwork, "free-net")
	requireNoCandidate(t, got, "used-image")
	requireNoCandidate(t, got, "stopped-image")
	requireNoCandidate(t, got, "bridge")

	cache := requireCandidate(t, got, CategoryBuildCache, BuildCacheAggregateID)
	if cache.SizeBytes != 500 || cache.SizeState != SizeKnown {
		t.Fatalf("build cache candidate = %#v, want known 500 bytes", cache)
	}
	if got.Summary.KnownReclaimableBytes != 5296 {
		t.Fatalf("known bytes = %d, want 5296", got.Summary.KnownReclaimableBytes)
	}
}

func TestEvaluateClassifiesVolumeSizeStatesAndRisk(t *testing.T) {
	got := Evaluate(Snapshot{DiskUsage: types.DiskUsage{Volumes: []*volume.Volume{
		{Name: "known", Driver: "local", UsageData: &volume.UsageData{RefCount: 0, Size: 2048}},
		{Name: "unknown", Driver: "nfs", UsageData: &volume.UsageData{RefCount: 0, Size: -1}},
		{Name: "missing-usage", Driver: "custom"},
		{Name: "used", Driver: "local", UsageData: &volume.UsageData{RefCount: 1, Size: 4096}},
	}}})

	category := requireCategory(t, got, CategoryUnusedVolume)
	if category.Risk != RiskHigh || category.DefaultSelected {
		t.Fatalf("volume category risk/default = %s/%v, want high/false", category.Risk, category.DefaultSelected)
	}
	if item := requireCandidate(t, got, CategoryUnusedVolume, "known"); item.SizeState != SizeKnown || item.SizeBytes != 2048 {
		t.Fatalf("known volume = %#v", item)
	}
	if item := requireCandidate(t, got, CategoryUnusedVolume, "unknown"); item.SizeState != SizeUnknown {
		t.Fatalf("unknown volume = %#v", item)
	}
	if item := requireCandidate(t, got, CategoryUnusedVolume, "missing-usage"); item.SizeState != SizeUnknown {
		t.Fatalf("missing usage volume = %#v", item)
	}
	requireNoCandidate(t, got, "used")
	if got.Summary.UnknownSizeCount != 2 {
		t.Fatalf("unknown count = %d, want 2", got.Summary.UnknownSizeCount)
	}
}

func TestEvaluateExcludesSystemUsedAndProtectedResources(t *testing.T) {
	got := Evaluate(Snapshot{
		DiskUsage: types.DiskUsage{Containers: []*types.Container{
			{ID: "self-id", Names: []string{"/tradis-client"}, State: "exited"},
			{ID: "protected-by-name", Names: []string{"/tradis-server"}, State: "created"},
			{ID: "restarting", Names: []string{"/unstable"}, State: "restarting"},
		}},
		Networks: []types.NetworkResource{
			{ID: "bridge-id", Name: "bridge"},
			{ID: "host-id", Name: "host"},
			{ID: "none-id", Name: "none"},
			{ID: "ingress-id", Name: "ingress-net", Ingress: true},
			{ID: "used-net", Name: "used", Containers: map[string]types.EndpointResource{"container": {Name: "demo"}}},
			{ID: "free-net", Name: "free"},
		},
		ProtectedContainerIDs:   map[string]struct{}{"self-id": {}},
		ProtectedContainerNames: map[string]struct{}{"tradis-server": {}},
	})

	requireNoCandidate(t, got, "self-id")
	requireNoCandidate(t, got, "protected-by-name")
	requireNoCandidate(t, got, "restarting")
	requireNoCandidate(t, got, "bridge-id")
	requireNoCandidate(t, got, "host-id")
	requireNoCandidate(t, got, "none-id")
	requireNoCandidate(t, got, "ingress-id")
	requireNoCandidate(t, got, "used-net")

	item := requireCandidate(t, got, CategoryUnusedNetwork, "free-net")
	if item.SizeState != SizeNotApplicable {
		t.Fatalf("network size state = %q, want %q", item.SizeState, SizeNotApplicable)
	}
	if got.Summary.UnknownSizeCount != 0 {
		t.Fatalf("network must not increase unknown count: %d", got.Summary.UnknownSizeCount)
	}
}

func requireCategory(t *testing.T, evaluation Evaluation, key Category) CategoryEvaluation {
	t.Helper()
	for _, category := range evaluation.Categories {
		if category.Key == key {
			return category
		}
	}
	t.Fatalf("category %q not found in %#v", key, evaluation.Categories)
	return CategoryEvaluation{}
}

func requireCandidate(t *testing.T, evaluation Evaluation, categoryKey Category, id string) Item {
	t.Helper()
	category := requireCategory(t, evaluation, categoryKey)
	for _, item := range category.Items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("candidate %q not found in category %q: %#v", id, categoryKey, category.Items)
	return Item{}
}

func requireNoCandidate(t *testing.T, evaluation Evaluation, id string) {
	t.Helper()
	for _, category := range evaluation.Categories {
		for _, item := range category.Items {
			if item.ID == id {
				t.Fatalf("unexpected candidate %q in category %q", id, category.Key)
			}
		}
	}
}
