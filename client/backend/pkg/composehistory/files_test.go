package composehistory

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceFilesReplacesExistingAndCreatesMissingTargets(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "compose.yml")
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(yamlPath, []byte("old-yaml"), 0600); err != nil {
		t.Fatal(err)
	}

	err := ReplaceFiles([]FileUpdate{
		{Path: yamlPath, Content: []byte("new-yaml"), Mode: 0644},
		{Path: envPath, Content: []byte("TOKEN=new"), Mode: 0640},
	})
	if err != nil {
		t.Fatalf("ReplaceFiles() error = %v", err)
	}
	assertFileContent(t, yamlPath, "new-yaml")
	assertFileContent(t, envPath, "TOKEN=new")
	assertFileMode(t, yamlPath, 0600)
	assertFileMode(t, envPath, 0640)
	assertNoReplacementArtifacts(t, dir)
}

func TestReplaceFilesRollsBackEveryTargetWhenSecondReplaceFails(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "compose.yml")
	envPath := filepath.Join(dir, ".env")
	const originalYAML = "services:\n  web:\n    image: nginx:1\n"
	const originalEnv = "TOKEN=old\n"
	if err := os.WriteFile(yamlPath, []byte(originalYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte(originalEnv), 0600); err != nil {
		t.Fatal(err)
	}

	originalRename := replaceFilesRename
	replaceFilesRename = func(oldPath, newPath string) error {
		if newPath == envPath && strings.Contains(filepath.Base(oldPath), ".tradis-config-") {
			return errors.New("injected env replacement failure")
		}
		return os.Rename(oldPath, newPath)
	}
	t.Cleanup(func() { replaceFilesRename = originalRename })

	err := ReplaceFiles([]FileUpdate{
		{Path: yamlPath, Content: []byte("services:\n  web:\n    image: nginx:2\n"), Mode: 0644},
		{Path: envPath, Content: []byte("TOKEN=new\n"), Mode: 0600},
	})
	if err == nil {
		t.Fatal("expected replacement failure")
	}
	assertFileContent(t, yamlPath, originalYAML)
	assertFileContent(t, envPath, originalEnv)
	assertNoReplacementArtifacts(t, dir)
}

func TestReplaceFilesDoesNotTouchTargetsWhenStagingFails(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(yamlPath, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	err := ReplaceFiles([]FileUpdate{
		{Path: yamlPath, Content: []byte("new"), Mode: 0644},
		{Path: filepath.Join(dir, "missing", ".env"), Content: []byte("TOKEN=new"), Mode: 0600},
	})
	if err == nil {
		t.Fatal("expected staging failure")
	}
	assertFileContent(t, yamlPath, "old")
	assertNoReplacementArtifacts(t, dir)
}

func TestReplaceFilesRejectsDuplicateTargets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compose.yml")
	err := ReplaceFiles([]FileUpdate{
		{Path: path, Content: []byte("first"), Mode: 0644},
		{Path: path, Content: []byte("second"), Mode: 0644},
	})
	if err == nil {
		t.Fatal("expected duplicate path rejection")
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("content of %s = %q, want %q", path, content, want)
	}
}

func assertFileMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want.Perm() {
		t.Fatalf("mode of %s = %o, want %o", path, info.Mode().Perm(), want.Perm())
	}
}

func assertNoReplacementArtifacts(t *testing.T, dir string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(entry.Name(), ".tradis-config-") || strings.Contains(entry.Name(), ".tradis-rollback-") {
			t.Errorf("replacement artifact remains: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
