package api

import (
	"testing"

	"dockerpanel/backend/pkg/database"
)

func TestSelectLocalRepoDigest(t *testing.T) {
	cases := []struct {
		name        string
		repoTag     string
		repoDigests []string
		want        string
	}{
		{
			name:        "no digests",
			repoTag:     "nginx:latest",
			repoDigests: nil,
			want:        "",
		},
		{
			name:        "single entry matches exactly",
			repoTag:     "nginx:latest",
			repoDigests: []string{"nginx@sha256:aaa"},
			want:        "sha256:aaa",
		},
		{
			name:        "single entry docker.io library variant",
			repoTag:     "nginx:latest",
			repoDigests: []string{"docker.io/library/nginx@sha256:aaa"},
			want:        "sha256:aaa",
		},
		{
			name:        "library tag against short repo digest",
			repoTag:     "docker.io/library/nginx:latest",
			repoDigests: []string{"nginx@sha256:aaa"},
			want:        "sha256:aaa",
		},
		{
			name:    "multiple digests picks the matching repo",
			repoTag: "nginx:latest",
			repoDigests: []string{
				"mirror.example.com/library/nginx@sha256:old",
				"nginx@sha256:new",
			},
			want: "sha256:new",
		},
		{
			name:    "multiple digests picks docker.io variant",
			repoTag: "library/nginx:latest",
			repoDigests: []string{
				"other/repo@sha256:other",
				"docker.io/library/nginx@sha256:match",
			},
			want: "sha256:match",
		},
		{
			name:        "single unrelated entry falls back to it",
			repoTag:     "nginx:latest",
			repoDigests: []string{"mirror.example.com/library/nginx@sha256:mirror"},
			want:        "sha256:mirror",
		},
		{
			name:    "multiple unrelated entries yield no digest",
			repoTag: "nginx:latest",
			repoDigests: []string{
				"mirror.example.com/library/nginx@sha256:m1",
				"other.example.com/nginx@sha256:m2",
			},
			want: "",
		},
		{
			name:        "entry without digest suffix ignored",
			repoTag:     "nginx:latest",
			repoDigests: []string{"nginx"},
			want:        "",
		},
		{
			name:        "custom registry matches host variant",
			repoTag:     "registry.example.com/team/app:1.0",
			repoDigests: []string{"registry.example.com/team/app@sha256:reg"},
			want:        "sha256:reg",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := selectLocalRepoDigest(tc.repoTag, tc.repoDigests); got != tc.want {
				t.Fatalf("selectLocalRepoDigest(%q, %v) = %q, want %q", tc.repoTag, tc.repoDigests, got, tc.want)
			}
		})
	}
}

func TestDecideImageUpdateReconcile(t *testing.T) {
	digests := []string{"nginx@sha256:local"}
	record := database.ImageUpdate{
		RepoTag:      "nginx:latest",
		LocalDigest:  "sha256:local",
		RemoteDigest: "sha256:remote",
	}
	legacyEmpty := database.ImageUpdate{RepoTag: "nginx:latest"}
	cases := []struct {
		name        string
		repoTag     string
		repoDigests []string
		old         database.ImageUpdate
		hasOld      bool
		force       bool
		wantAction  imageReconcileAction
		wantDigest  string
	}{
		{
			name:        "no record checks remote",
			repoTag:     "nginx:latest",
			repoDigests: digests,
			wantAction:  imageReconcileCheck,
			wantDigest:  "sha256:local",
		},
		{
			name:        "empty local digest without record skips",
			repoTag:     "nginx:latest",
			repoDigests: nil,
			wantAction:  imageReconcileSkip,
		},
		{
			name:        "empty local digest checks remote with force",
			repoTag:     "nginx:latest",
			repoDigests: nil,
			force:       true,
			wantAction:  imageReconcileCheck,
		},
		{
			name:        "empty local digest keeps record and skips without force",
			repoTag:     "nginx:latest",
			repoDigests: nil,
			old:         record,
			hasOld:      true,
			wantAction:  imageReconcileSkip,
		},
		{
			name:        "empty local digest rechecks with force",
			repoTag:     "nginx:latest",
			repoDigests: nil,
			old:         record,
			hasOld:      true,
			force:       true,
			wantAction:  imageReconcileRecheck,
		},
		{
			name:        "legacy empty record with verifiable digest skips without force",
			repoTag:     "nginx:latest",
			repoDigests: digests,
			old:         legacyEmpty,
			hasOld:      true,
			wantAction:  imageReconcileSkip,
			wantDigest:  "sha256:local",
		},
		{
			name:        "legacy empty record with verifiable digest checks with force",
			repoTag:     "nginx:latest",
			repoDigests: digests,
			old:         legacyEmpty,
			hasOld:      true,
			force:       true,
			wantAction:  imageReconcileCheck,
			wantDigest:  "sha256:local",
		},
		{
			name:        "legacy empty record without local digest skips without force",
			repoTag:     "nginx:latest",
			repoDigests: nil,
			old:         legacyEmpty,
			hasOld:      true,
			wantAction:  imageReconcileSkip,
		},
		{
			name:        "legacy empty record without local digest rechecks with force",
			repoTag:     "nginx:latest",
			repoDigests: nil,
			old:         legacyEmpty,
			hasOld:      true,
			force:       true,
			wantAction:  imageReconcileRecheck,
		},
		{
			name:        "unchanged local digest skips without force",
			repoTag:     "nginx:latest",
			repoDigests: digests,
			old:         record,
			hasOld:      true,
			wantAction:  imageReconcileSkip,
			wantDigest:  "sha256:local",
		},
		{
			name:        "unchanged local digest rechecks with force",
			repoTag:     "nginx:latest",
			repoDigests: digests,
			old:         record,
			hasOld:      true,
			force:       true,
			wantAction:  imageReconcileCheck,
			wantDigest:  "sha256:local",
		},
		{
			name:        "caught-up local digest deletes and rechecks remote",
			repoTag:     "nginx:latest",
			repoDigests: []string{"nginx@sha256:remote"},
			old:         record,
			hasOld:      true,
			wantAction:  imageReconcileRecheck,
			wantDigest:  "sha256:remote",
		},
		{
			name:        "changed local digest deletes and rechecks remote",
			repoTag:     "nginx:latest",
			repoDigests: []string{"nginx@sha256:changed"},
			old:         record,
			hasOld:      true,
			wantAction:  imageReconcileRecheck,
			wantDigest:  "sha256:changed",
		},
		{
			name:        "wrong RepoDigests[0] still matches by repo",
			repoTag:     "nginx:latest",
			repoDigests: []string{"unrelated/repo@sha256:unrelated", "nginx@sha256:remote"},
			old:         record,
			hasOld:      true,
			wantAction:  imageReconcileRecheck,
			wantDigest:  "sha256:remote",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decideImageUpdateReconcile(tc.repoTag, tc.repoDigests, tc.old, tc.hasOld, tc.force)
			if got.action != tc.wantAction {
				t.Fatalf("action = %v, want %v", got.action, tc.wantAction)
			}
			if got.localDigest != tc.wantDigest {
				t.Fatalf("localDigest = %q, want %q", got.localDigest, tc.wantDigest)
			}
		})
	}
}

func TestRepoDigestValues(t *testing.T) {
	cases := []struct {
		name        string
		repoDigests []string
		want        []string
	}{
		{
			name:        "nil yields empty",
			repoDigests: nil,
			want:        []string{},
		},
		{
			name:        "extracts digest parts",
			repoDigests: []string{"nginx@sha256:aaa", "docker.io/library/nginx@sha256:bbb"},
			want:        []string{"sha256:aaa", "sha256:bbb"},
		},
		{
			name:        "skips entries without digest",
			repoDigests: []string{"nginx", "repo@sha256:ok", "@sha256:noRepo"},
			want:        []string{"sha256:ok", "sha256:noRepo"},
		},
		{
			name:        "deduplicates repeated digests",
			repoDigests: []string{"nginx@sha256:same", "mirror.example.com/nginx@sha256:same"},
			want:        []string{"sha256:same"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := repoDigestValues(tc.repoDigests)
			if len(got) != len(tc.want) {
				t.Fatalf("repoDigestValues(%v) = %v, want %v", tc.repoDigests, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("repoDigestValues(%v)[%d] = %q, want %q", tc.repoDigests, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestDecideImageUpdateRecordWrite(t *testing.T) {
	indexAndPlatform := []string{
		"sha256:index",
		"sha256:platform-amd64",
		"sha256:platform-arm64",
	}
	cases := []struct {
		name          string
		old           database.ImageUpdate
		hasOld        bool
		localDigests  []string
		selectedLocal string
		remoteDigest  string
		wantUpToDate  bool
		wantNotify    bool
		wantLocal     string
		wantRemote    string
		wantNotified  bool
	}{
		{
			name:          "remote index digest matches index entry in set",
			localDigests:  indexAndPlatform,
			selectedLocal: "sha256:index",
			remoteDigest:  "sha256:index",
			wantUpToDate:  true,
		},
		{
			name:          "remote platform digest hits one of multiple entries",
			localDigests:  indexAndPlatform,
			selectedLocal: "sha256:index",
			remoteDigest:  "sha256:platform-amd64",
			wantUpToDate:  true,
		},
		{
			name:          "remote digest outside set writes record",
			localDigests:  indexAndPlatform,
			selectedLocal: "sha256:index",
			remoteDigest:  "sha256:newRemote",
			wantNotify:    true,
			wantLocal:     "sha256:index",
			wantRemote:    "sha256:newRemote",
		},
		{
			name:          "single entry equal to remote is up to date",
			localDigests:  []string{"sha256:only"},
			selectedLocal: "sha256:only",
			remoteDigest:  "sha256:only",
			wantUpToDate:  true,
		},
		{
			name: "unchanged remote target keeps notified and does not notify again",
			old: database.ImageUpdate{
				RepoTag:      "nginx:latest",
				LocalDigest:  "sha256:oldLocal",
				RemoteDigest: "sha256:targetB",
				Notified:     true,
			},
			hasOld:        true,
			localDigests:  []string{"sha256:newLocal"},
			selectedLocal: "sha256:newLocal",
			remoteDigest:  "sha256:targetB",
			wantNotify:    false,
			wantLocal:     "sha256:newLocal",
			wantRemote:    "sha256:targetB",
			wantNotified:  true,
		},
		{
			name: "advanced remote target notifies again",
			old: database.ImageUpdate{
				RepoTag:      "nginx:latest",
				LocalDigest:  "sha256:local",
				RemoteDigest: "sha256:targetB",
				Notified:     true,
			},
			hasOld:        true,
			localDigests:  []string{"sha256:local"},
			selectedLocal: "sha256:local",
			remoteDigest:  "sha256:targetC",
			wantNotify:    true,
			wantLocal:     "sha256:local",
			wantRemote:    "sha256:targetC",
		},
		{
			name:          "no old record notifies",
			localDigests:  []string{"sha256:local"},
			selectedLocal: "sha256:local",
			remoteDigest:  "sha256:targetB",
			wantNotify:    true,
			wantLocal:     "sha256:local",
			wantRemote:    "sha256:targetB",
		},
		{
			name: "legacy record without remote digest notifies",
			old: database.ImageUpdate{
				RepoTag:     "nginx:latest",
				LocalDigest: "sha256:local",
			},
			hasOld:        true,
			localDigests:  []string{"sha256:local"},
			selectedLocal: "sha256:local",
			remoteDigest:  "sha256:targetB",
			wantNotify:    true,
			wantLocal:     "sha256:local",
			wantRemote:    "sha256:targetB",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upToDate, record, notify := decideImageUpdateRecordWrite(tc.old, tc.hasOld, tc.localDigests, tc.selectedLocal, tc.remoteDigest)
			if upToDate != tc.wantUpToDate {
				t.Fatalf("upToDate = %v, want %v", upToDate, tc.wantUpToDate)
			}
			if notify != tc.wantNotify {
				t.Fatalf("notify = %v, want %v", notify, tc.wantNotify)
			}
			if upToDate {
				return
			}
			if record.LocalDigest != tc.wantLocal {
				t.Fatalf("record.LocalDigest = %q, want %q", record.LocalDigest, tc.wantLocal)
			}
			if record.RemoteDigest != tc.wantRemote {
				t.Fatalf("record.RemoteDigest = %q, want %q", record.RemoteDigest, tc.wantRemote)
			}
			if record.Notified != tc.wantNotified {
				t.Fatalf("record.Notified = %v, want %v", record.Notified, tc.wantNotified)
			}
		})
	}
}

func TestParseComposeConfigImages(t *testing.T) {
	output := "nginx:1.27\nregistry.example.com/team/app:2.0\n\n  \nredis:7\n"
	want := []string{"nginx:1.27", "registry.example.com/team/app:2.0", "redis:7"}
	got := parseComposeConfigImages(output)
	if len(got) != len(want) {
		t.Fatalf("parseComposeConfigImages() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseComposeConfigImages()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if got := parseComposeConfigImages(""); got != nil {
		t.Fatalf("parseComposeConfigImages(\"\") = %v, want nil", got)
	}
}
