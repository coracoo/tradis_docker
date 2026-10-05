package api

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dockerpanel/backend/pkg/database"
	tradisdocker "dockerpanel/backend/pkg/docker"

	"github.com/docker/docker/api/types"
)

func TestPullImageWithOldCleanupReleasesTemporaryReferenceAfterSuccess(t *testing.T) {
	var taggedSource string
	var taggedTarget string
	var removedReference string
	var removedOptions types.ImageRemoveOptions
	store := &tradisdocker.MockDockerClient{
		ImageInspectWithRawFunc: func(context.Context, string) (types.ImageInspect, []byte, error) {
			return types.ImageInspect{ID: "sha256:old"}, nil, nil
		},
		ImageTagFunc: func(_ context.Context, source string, target string) error {
			taggedSource = source
			taggedTarget = target
			return nil
		},
		ImagePullFunc: func(context.Context, string, types.ImagePullOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("{\"status\":\"Downloaded\"}\n")), nil
		},
		ImageRemoveFunc: func(_ context.Context, reference string, options types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error) {
			removedReference = reference
			removedOptions = options
			return nil, nil
		},
	}

	result, err := pullImageWithOldCleanup(
		context.Background(),
		store,
		"nginx:latest",
		types.ImagePullOptions{},
		"task-1",
		true,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if taggedSource != "sha256:old" {
		t.Fatalf("tag source = %q, want sha256:old", taggedSource)
	}
	if taggedTarget == "" || removedReference != taggedTarget {
		t.Fatalf("tag target = %q, removed = %q", taggedTarget, removedReference)
	}
	if !removedOptions.Force || removedOptions.PruneChildren {
		t.Fatalf("remove options = %#v", removedOptions)
	}
	if !result.CleanupRequested || !result.CleanupSucceeded || result.OldImageID != "sha256:old" {
		t.Fatalf("result = %#v", result)
	}
}

func TestPullImageWithOldCleanupReleasesTemporaryReferenceAfterPullFailure(t *testing.T) {
	var taggedTarget string
	var removedReference string
	store := &tradisdocker.MockDockerClient{
		ImageInspectWithRawFunc: func(context.Context, string) (types.ImageInspect, []byte, error) {
			return types.ImageInspect{ID: "sha256:old"}, nil, nil
		},
		ImageTagFunc: func(_ context.Context, _ string, target string) error {
			taggedTarget = target
			return nil
		},
		ImagePullFunc: func(context.Context, string, types.ImagePullOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("{\"error\":\"registry denied\"}\n")), nil
		},
		ImageRemoveFunc: func(_ context.Context, reference string, _ types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error) {
			removedReference = reference
			return nil, nil
		},
	}

	_, err := pullImageWithOldCleanup(
		context.Background(),
		store,
		"nginx:latest",
		types.ImagePullOptions{},
		"task-2",
		true,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "registry denied") {
		t.Fatalf("error = %v", err)
	}
	if taggedTarget == "" || removedReference != taggedTarget {
		t.Fatalf("temporary reference was not released: tagged=%q removed=%q", taggedTarget, removedReference)
	}
}

func TestPullImageWithoutCleanupDoesNotTagOrRemove(t *testing.T) {
	inspectCalled := false
	tagCalled := false
	removeCalled := false
	store := &tradisdocker.MockDockerClient{
		ImageInspectWithRawFunc: func(context.Context, string) (types.ImageInspect, []byte, error) {
			inspectCalled = true
			return types.ImageInspect{}, nil, errors.New("unexpected inspect")
		},
		ImageTagFunc: func(context.Context, string, string) error {
			tagCalled = true
			return nil
		},
		ImagePullFunc: func(context.Context, string, types.ImagePullOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("{\"status\":\"Downloaded\"}\n")), nil
		},
		ImageRemoveFunc: func(context.Context, string, types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error) {
			removeCalled = true
			return nil, nil
		},
	}

	result, err := pullImageWithOldCleanup(
		context.Background(),
		store,
		"nginx:latest",
		types.ImagePullOptions{},
		"task-3",
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if inspectCalled || tagCalled || removeCalled || result.CleanupRequested {
		t.Fatalf("ordinary pull changed image references: inspect=%v tag=%v remove=%v result=%#v", inspectCalled, tagCalled, removeCalled, result)
	}
}

func TestRemovedImageTagsFromDeleteItems(t *testing.T) {
	items := []types.ImageDeleteResponseItem{
		{Untagged: "nginx:latest"},
		{Deleted: "sha256:layer1"},
		{Untagged: "  "},
		{Untagged: "docker.io/library/redis:7"},
	}
	got := removedImageTagsFromDeleteItems(items)
	want := []string{"nginx:latest", "docker.io/library/redis:7"}
	if len(got) != len(want) {
		t.Fatalf("removedImageTagsFromDeleteItems() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("removedImageTagsFromDeleteItems() = %v, want %v", got, want)
		}
	}
	if got := removedImageTagsFromDeleteItems(nil); got != nil {
		t.Fatalf("removedImageTagsFromDeleteItems(nil) = %v, want nil", got)
	}
	if got := removedImageTagsFromDeleteItems([]types.ImageDeleteResponseItem{{Deleted: "sha256:layer"}}); got != nil {
		t.Fatalf("removedImageTagsFromDeleteItems(Deleted-only) = %v, want nil", got)
	}
}

// removeImage/pruneImages 共用同一条清理链路：按 daemon 返回的 Untagged 清单
// 精确清理被移除 tag 的更新记录（模拟 removeImage 删除响应驱动清理）。
func TestClearImageUpdateRecordsByRemovedTags(t *testing.T) {
	_ = database.Close()
	if err := database.InitDB(filepath.Join(t.TempDir(), "image-remove-cleanup.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	seed := []database.ImageUpdate{
		{RepoTag: "nginx:latest", ImageID: "sha256:one", LocalDigest: "sha256:local", RemoteDigest: "sha256:remote"},
		{RepoTag: "redis:7", ImageID: "sha256:two", LocalDigest: "sha256:local", RemoteDigest: "sha256:remote"},
	}
	for i := range seed {
		if err := database.SaveImageUpdate(&seed[i]); err != nil {
			t.Fatalf("SaveImageUpdate(%q): %v", seed[i].RepoTag, err)
		}
	}

	// 预填角标缓存，验证清理链路会将其失效
	imageUpdateMapCache.mu.Lock()
	imageUpdateMapCache.updateMap = map[string]bool{"nginx:latest": true}
	imageUpdateMapCache.expires = time.Now().Add(time.Hour)
	imageUpdateMapCache.mu.Unlock()

	report := []types.ImageDeleteResponseItem{
		{Untagged: "nginx:latest"},
		{Deleted: "sha256:layer"},
	}
	clearImageUpdateRecordsByImageRefs(removedImageTagsFromDeleteItems(report)...)

	remaining, err := database.GetAllImageUpdates()
	if err != nil {
		t.Fatalf("GetAllImageUpdates: %v", err)
	}
	if len(remaining) != 1 || remaining[0].RepoTag != "redis:7" {
		t.Fatalf("remaining records = %+v, want only redis:7", remaining)
	}
	imageUpdateMapCache.mu.Lock()
	defer imageUpdateMapCache.mu.Unlock()
	if imageUpdateMapCache.updateMap != nil {
		t.Fatalf("image update map cache was not invalidated")
	}
}

func TestStoredImageUpdateInfosFiltersCaughtUpRecords(t *testing.T) {
	items := []database.ImageUpdate{
		{RepoTag: "nginx:latest", LocalDigest: "sha256:same", RemoteDigest: "sha256:same", Notified: true},
		{RepoTag: "empty:record", LocalDigest: "", RemoteDigest: ""},
		{RepoTag: "redis:7", LocalDigest: "sha256:local", RemoteDigest: "sha256:remote", Notified: false},
	}
	got := storedImageUpdateInfos(items)
	if len(got) != 1 {
		t.Fatalf("storedImageUpdateInfos() len = %d, want 1: %+v", len(got), got)
	}
	if got[0].RepoTag != "redis:7" || got[0].Notified {
		t.Fatalf("storedImageUpdateInfos()[0] = %+v, want redis:7 unnotified", got[0])
	}
	if got := storedImageUpdateInfos(nil); got != nil {
		t.Fatalf("storedImageUpdateInfos(nil) = %v, want nil", got)
	}
}

func retagInspectClient(repoTags map[string][]string) *tradisdocker.MockDockerClient {
	return &tradisdocker.MockDockerClient{
		ImageInspectWithRawFunc: func(_ context.Context, imageID string) (types.ImageInspect, []byte, error) {
			tags, ok := repoTags[imageID]
			if !ok {
				return types.ImageInspect{}, nil, errors.New("No such image: " + imageID)
			}
			return types.ImageInspect{ID: "sha256:img", RepoTags: tags}, nil, nil
		},
	}
}

func TestCleanupRetaggedSourceImageUpdateRecordsClearsOldTag(t *testing.T) {
	_ = database.Close()
	if err := database.InitDB(filepath.Join(t.TempDir(), "image-retag-cleanup.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	seed := database.ImageUpdate{RepoTag: "nginx:old", ImageID: "sha256:img", LocalDigest: "sha256:local", RemoteDigest: "sha256:remote"}
	if err := database.SaveImageUpdate(&seed); err != nil {
		t.Fatalf("SaveImageUpdate: %v", err)
	}

	// 改名生效：旧 tag 已不在镜像上 → 清掉旧 tag 的更新记录
	cli := retagInspectClient(map[string][]string{
		"nginx:old": {"nginx:old"},
		"nginx:new": {"nginx:new"},
	})
	cleanupRetaggedSourceImageUpdateRecords(context.Background(), cli, "nginx:old", "nginx:new")

	remaining, err := database.GetAllImageUpdates()
	if err != nil {
		t.Fatalf("GetAllImageUpdates: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining records after retag = %+v, want none", remaining)
	}
}

func TestCleanupRetaggedSourceImageUpdateRecordsKeepsRecords(t *testing.T) {
	_ = database.Close()
	if err := database.InitDB(filepath.Join(t.TempDir(), "image-retag-keep.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	seed := database.ImageUpdate{RepoTag: "nginx:old", ImageID: "sha256:img", LocalDigest: "sha256:local", RemoteDigest: "sha256:remote"}
	if err := database.SaveImageUpdate(&seed); err != nil {
		t.Fatalf("SaveImageUpdate: %v", err)
	}
	assertKept := func(name string, cli *tradisdocker.MockDockerClient, source string) {
		t.Helper()
		cleanupRetaggedSourceImageUpdateRecords(context.Background(), cli, source, "nginx:new")
		remaining, err := database.GetAllImageUpdates()
		if err != nil {
			t.Fatalf("%s: GetAllImageUpdates: %v", name, err)
		}
		if len(remaining) != 1 {
			t.Fatalf("%s: remaining records = %+v, want old tag record kept", name, remaining)
		}
	}

	// 追加标签：旧 tag 仍在场，记录与 notified 状态保留
	assertKept("add tag", retagInspectClient(map[string][]string{
		"nginx:old": {"nginx:old"},
		"nginx:new": {"nginx:old", "nginx:new"},
	}), "nginx:old")

	// 镜像 ID 引用（前端改名流程的入参形态）：不动任何记录
	assertKept("image id source", retagInspectClient(map[string][]string{
		"sha256:img": {"nginx:old"},
		"nginx:new":  {"nginx:old", "nginx:new"},
	}), "sha256:img")

	// 源引用无法解析（tag 已不存在/未知引用）：不动任何记录
	assertKept("unresolvable source", retagInspectClient(map[string][]string{}), "nginx:old")
}
