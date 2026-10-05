package deployment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLexicalPathWithinRoot(t *testing.T) {
	root := "/srv/project"
	cases := []struct {
		name    string
		cand    string
		wantErr bool
	}{
		{"relative inside", "data/config.yml", false},
		{"relative nested inside", "data/sub/cfg.yml", false},
		{"relative parent escape", "../secret", true},
		{"relative nested escape", "data/../../secret", true},
		{"absolute inside", "/srv/project/data/cfg.yml", false},
		{"absolute outside", "/etc/passwd", true},
		{"absolute root sibling", "/srv/other", true},
		{"dot path", ".", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := LexicalPathWithinRoot(root, tc.cand)
			if tc.wantErr && err == nil {
				t.Fatalf("expected escape error for %q", tc.cand)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.cand, err)
			}
		})
	}
}

func TestResolveWithinRootRejectsEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}
	if _, err := ResolveWithinRoot(root, "escape/secret"); err == nil {
		t.Fatal("symlink escaping root must be rejected")
	}
}

func TestResolveWithinRootAllowsSymlinkInsideRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}
	if _, err := ResolveWithinRoot(root, "alias/file"); err != nil {
		t.Fatalf("symlink staying inside root must resolve: %v", err)
	}
	if _, err := ResolveWithinRoot(root, "data/newfile"); err != nil {
		t.Fatalf("non-existing path inside root must resolve: %v", err)
	}
}

func TestPathAllowedByRootsRejectsBindThroughEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}
	if pathAllowedByRoots(filepath.Join(root, "escape", "media"), []string{root}) {
		t.Fatal("bind path resolving outside an allowed root must be rejected")
	}
	if !pathAllowedByRoots(filepath.Join(root, "new", "media"), []string{root}) {
		t.Fatal("non-existing bind path inside an allowed root must remain allowed")
	}
}

func TestResolveWithinRootAllowsRootSymlinkAlias(t *testing.T) {
	base := t.TempDir()
	realRoot := filepath.Join(base, "real")
	aliasRoot := filepath.Join(base, "alias")
	if err := os.MkdirAll(filepath.Join(realRoot, "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRoot, aliasRoot); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}
	if _, err := ResolveWithinRoot(aliasRoot, filepath.Join(aliasRoot, "media")); err != nil {
		t.Fatalf("allowed root symlink alias must resolve inside its real root: %v", err)
	}
}
