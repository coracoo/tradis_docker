package api

import (
	"path/filepath"
	"sort"
	"testing"

	"dockerpanel/backend/pkg/database"
)

func staleCleanupSet(items ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}
	return set
}

// staleVariantSet 与生产路径 localRepoTagVariantSet 同口径：把本地 RepoTags
// 展开为全量拼写变体集合。
func staleVariantSet(tags ...string) map[string]struct{} {
	return localRepoTagVariantSet(staleCleanupSet(tags...))
}

func staleRepoTags(tags []string) []string {
	sorted := append([]string(nil), tags...)
	sort.Strings(sorted)
	return sorted
}

func TestStaleImageUpdateRepoTags(t *testing.T) {
	cases := []struct {
		name          string
		existing      []database.ImageUpdate
		localImageIDs map[string]struct{}
		localRepoTags []string
		wantStale     []string
	}{
		{
			name: "record kept when image id present and tag variant present",
			existing: []database.ImageUpdate{
				{RepoTag: "nginx:latest", ImageID: "sha256:keep"},
			},
			localImageIDs: staleCleanupSet("sha256:keep"),
			localRepoTags: []string{"nginx:latest"},
			wantStale:     nil,
		},
		{
			name: "docker rmi tag flags record while image stays",
			existing: []database.ImageUpdate{
				{RepoTag: "nginx:latest", ImageID: "sha256:keep", LocalDigest: "sha256:local"},
			},
			localImageIDs: staleCleanupSet("sha256:keep"),
			localRepoTags: []string{"redis:7"},
			wantStale:     []string{"nginx:latest"},
		},
		{
			name: "variant spelling repo_tag kept against prefixed local tag",
			existing: []database.ImageUpdate{
				{RepoTag: "nginx:latest", ImageID: "sha256:keep"},
			},
			localImageIDs: staleCleanupSet("sha256:keep"),
			localRepoTags: []string{"docker.io/library/nginx:latest"},
			wantStale:     nil,
		},
		{
			name: "prefixed repo_tag kept against short local tag",
			existing: []database.ImageUpdate{
				{RepoTag: "docker.io/library/nginx:latest", ImageID: "sha256:keep"},
			},
			localImageIDs: staleCleanupSet("sha256:keep"),
			localRepoTags: []string{"nginx:latest"},
			wantStale:     nil,
		},
		{
			name: "docker rmi flags only removed tag on multi tag image",
			existing: []database.ImageUpdate{
				{RepoTag: "app:1.0", ImageID: "sha256:multi"},
				{RepoTag: "app:1.1", ImageID: "sha256:multi"},
			},
			localImageIDs: staleCleanupSet("sha256:multi"),
			localRepoTags: []string{"app:1.1", "app:latest"},
			wantStale:     []string{"app:1.0"},
		},
		{
			name: "removed image record flagged",
			existing: []database.ImageUpdate{
				{RepoTag: "nginx:latest", ImageID: "sha256:gone", LocalDigest: "sha256:local"},
			},
			localImageIDs: staleCleanupSet("sha256:other"),
			localRepoTags: []string{"nginx:latest"},
			wantStale:     []string{"nginx:latest"},
		},
		{
			name: "empty image id with variant spelling present kept",
			existing: []database.ImageUpdate{
				{RepoTag: "nginx:latest", ImageID: ""},
			},
			localImageIDs: staleCleanupSet("sha256:present"),
			localRepoTags: []string{"docker.io/library/nginx:latest"},
			wantStale:     nil,
		},
		{
			name: "empty image id with missing tag flagged",
			existing: []database.ImageUpdate{
				{RepoTag: "registry.example.com/team/app:1.0", ImageID: ""},
			},
			localImageIDs: staleCleanupSet("sha256:present"),
			localRepoTags: []string{"nginx:latest"},
			wantStale:     []string{"registry.example.com/team/app:1.0"},
		},
		{
			name: "retagged image judged by image id only",
			existing: []database.ImageUpdate{
				// tag 仍在场，但记录指向的旧镜像已删除（主循环结束后读到的是
				// 刷新过 image_id 的记录；此处模拟刷新未发生的残留，按 image_id 口径清理）
				{RepoTag: "redis:7", ImageID: "sha256:old"},
			},
			localImageIDs: staleCleanupSet("sha256:new"),
			localRepoTags: []string{"redis:7"},
			wantStale:     []string{"redis:7"},
		},
		{
			name: "mixed scenario flags only stale records",
			existing: []database.ImageUpdate{
				{RepoTag: "nginx:latest", ImageID: "sha256:present"},
				{RepoTag: "redis:7", ImageID: "sha256:gone"},
				{RepoTag: "alpine:3.20", ImageID: ""},
				{RepoTag: "registry.example.com/team/app:1.0", ImageID: ""},
				{RepoTag: "docker.io/library/busybox:1.36", ImageID: "sha256:also-present"},
			},
			localImageIDs: staleCleanupSet("sha256:present", "sha256:also-present"),
			localRepoTags: []string{"nginx:latest", "alpine:3.20", "docker.io/library/busybox:1.36", "app:1.0"},
			wantStale:     []string{"redis:7", "registry.example.com/team/app:1.0"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := staleRepoTags(staleImageUpdateRepoTags(tc.existing, tc.localImageIDs, staleVariantSet(tc.localRepoTags...)))
			want := staleRepoTags(tc.wantStale)
			if len(got) != len(want) {
				t.Fatalf("staleImageUpdateRepoTags() = %v, want %v", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("staleImageUpdateRepoTags() = %v, want %v", got, want)
				}
			}
		})
	}
}

func TestCleanupStaleImageUpdateRecordsKeepsOnlyLocalRecords(t *testing.T) {
	_ = database.Close()
	if err := database.InitDB(filepath.Join(t.TempDir(), "image-update-stale-cleanup.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	// 模拟"check 主循环已写入/保留"之后的表内容：在场镜像记录、已删除镜像记录、
	// 空 image_id 记录、镜像在场但 tag 被 docker rmi 移除的幽灵记录混合，随后在主
	// 循环结束位置执行一次清理。
	seed := []database.ImageUpdate{
		{RepoTag: "nginx:latest", ImageID: "sha256:present", LocalDigest: "sha256:local", RemoteDigest: "sha256:remote"},
		{RepoTag: "redis:7", ImageID: "sha256:gone"},
		{RepoTag: "alpine:3.20", ImageID: ""},
		{RepoTag: "registry.example.com/team/app:1.0", ImageID: ""},
		{RepoTag: "ghost:1.0", ImageID: "sha256:present"},
		{RepoTag: "prefixed:keep", ImageID: "sha256:also-present"},
	}
	for i := range seed {
		if err := database.SaveImageUpdate(&seed[i]); err != nil {
			t.Fatalf("seed SaveImageUpdate(%q): %v", seed[i].RepoTag, err)
		}
	}

	localImageIDs := staleCleanupSet("sha256:present", "sha256:also-present")
	localRepoTags := staleCleanupSet("nginx:latest", "alpine:3.20", "docker.io/library/prefixed:keep")

	cleanupStaleImageUpdateRecords(localImageIDs, localRepoTags)

	remaining, err := database.GetAllImageUpdates()
	if err != nil {
		t.Fatalf("GetAllImageUpdates after cleanup: %v", err)
	}
	got := make([]string, 0, len(remaining))
	for _, u := range remaining {
		got = append(got, u.RepoTag)
	}
	want := []string{"alpine:3.20", "nginx:latest", "prefixed:keep"}
	if got := staleRepoTags(got); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("remaining repo_tags after cleanup = %v, want %v", got, want)
	}

	// 清理幂等：对已清理过的状态再执行一次，结果不变
	cleanupStaleImageUpdateRecords(localImageIDs, localRepoTags)
	again, err := database.GetAllImageUpdates()
	if err != nil {
		t.Fatalf("GetAllImageUpdates after second cleanup: %v", err)
	}
	if len(again) != len(remaining) {
		t.Fatalf("second cleanup changed record count: %d, want %d", len(again), len(remaining))
	}
}
