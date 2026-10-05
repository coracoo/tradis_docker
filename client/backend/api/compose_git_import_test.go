package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dockerpanel/backend/pkg/database"
)

func TestNormalizePublicGitHubRepoURL(t *testing.T) {
	got, repo, err := normalizePublicGitHubRepoURL("https://github.com/example/demo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://github.com/example/demo.git" || repo != "demo" {
		t.Fatalf("unexpected result: url=%s repo=%s", got, repo)
	}

	for _, raw := range []string{
		"http://github.com/example/demo",
		"https://gitlab.com/example/demo",
		"https://user:token@github.com/example/demo",
		"https://github.com/example/demo?token=secret",
		"https://github.com/example/demo/extra",
	} {
		if _, _, err := normalizePublicGitHubRepoURL(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestValidateGitRef(t *testing.T) {
	for _, ref := range []string{"", "main", "release/v1.2.0", "v2.0.0"} {
		if !validateGitRef(ref) {
			t.Fatalf("expected %q to be valid", ref)
		}
	}
	for _, ref := range []string{"--upload-pack=x", "../main", "bad ref", "topic~1"} {
		if validateGitRef(ref) {
			t.Fatalf("expected %q to be rejected", ref)
		}
	}
}

func TestNormalizeGitAcceleratorURL(t *testing.T) {
	got, err := normalizeGitAcceleratorURL("https://ghfast.top/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://ghfast.top" {
		t.Fatalf("unexpected accelerator: %s", got)
	}

	for _, raw := range []string{
		"http://ghfast.top",
		"https://localhost",
		"https://127.0.0.1",
		"https://user:pass@example.com",
		"https://example.com/?token=secret",
	} {
		if _, err := normalizeGitAcceleratorURL(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestFindRootComposeFileRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "compose.yml")
	if err := os.WriteFile(outside, []byte("services: {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "compose.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := findRootComposeFile(root); err == nil {
		t.Fatal("expected symlink compose file to be rejected")
	}
}

func TestFindImportedComposeFile(t *testing.T) {
	validCompose := []byte("services:\n  app:\n    image: nginx\n")
	writeCompose := func(t *testing.T, root, relativePath string, content []byte) string {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("prefers root standard file over nested files", func(t *testing.T) {
		root := t.TempDir()
		want := writeCompose(t, root, "compose.yml", validCompose)
		writeCompose(t, root, "docker/docker-compose.yaml", validCompose)
		got, err := findImportedComposeFile(root, "")
		if err != nil || got != want {
			t.Fatalf("findImportedComposeFile() = %q, %v; want %q, nil", got, err, want)
		}
	})

	t.Run("prefers docker before deploy", func(t *testing.T) {
		root := t.TempDir()
		want := writeCompose(t, root, "docker/compose.yaml", validCompose)
		writeCompose(t, root, "deploy/docker-compose.yml", validCompose)
		got, err := findImportedComposeFile(root, "")
		if err != nil || got != want {
			t.Fatalf("findImportedComposeFile() = %q, %v; want %q, nil", got, err, want)
		}
	})

	t.Run("finds deploy compose", func(t *testing.T) {
		root := t.TempDir()
		want := writeCompose(t, root, "deploy/compose.yml", validCompose)
		got, err := findImportedComposeFile(root, "")
		if err != nil || got != want {
			t.Fatalf("findImportedComposeFile() = %q, %v; want %q, nil", got, err, want)
		}
	})

	t.Run("prefers other first-level directories before second-level directories", func(t *testing.T) {
		root := t.TempDir()
		want := writeCompose(t, root, "app/compose.yml", validCompose)
		writeCompose(t, root, "examples/demo/docker-compose.yaml", validCompose)
		got, err := findImportedComposeFile(root, "")
		if err != nil || got != want {
			t.Fatalf("findImportedComposeFile() = %q, %v; want %q, nil", got, err, want)
		}
	})

	t.Run("finds second-level compose", func(t *testing.T) {
		root := t.TempDir()
		want := writeCompose(t, root, "examples/demo/docker-compose.yaml", validCompose)
		got, err := findImportedComposeFile(root, "")
		if err != nil || got != want {
			t.Fatalf("findImportedComposeFile() = %q, %v; want %q, nil", got, err, want)
		}
	})

	t.Run("rejects out-of-scope and invalid candidates", func(t *testing.T) {
		root := t.TempDir()
		writeCompose(t, root, "examples/demo/third/compose.yml", validCompose)
		writeCompose(t, root, "invalid/compose.yml", []byte("not: [valid\n"))
		writeCompose(t, root, "empty/compose.yml", []byte("services: {}\n"))
		if err := os.MkdirAll(filepath.Join(root, "special", "compose.yml"), 0755); err != nil {
			t.Fatal(err)
		}
		outside := writeCompose(t, t.TempDir(), "compose.yml", validCompose)
		if err := os.MkdirAll(filepath.Join(root, "linked"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "linked", "compose.yml")); err != nil {
			t.Fatal(err)
		}

		if _, err := findImportedComposeFile(root, ""); err == nil {
			t.Fatal("expected no eligible Compose file")
		}
	})

	t.Run("reuses a valid saved path then rediscovers after it is removed", func(t *testing.T) {
		root := t.TempDir()
		preferred := writeCompose(t, root, "examples/demo/compose.yml", validCompose)
		fallback := writeCompose(t, root, "deploy/docker-compose.yml", validCompose)

		got, err := findImportedComposeFile(root, "examples/demo/compose.yml")
		if err != nil || got != preferred {
			t.Fatalf("findImportedComposeFile() = %q, %v; want saved path %q, nil", got, err, preferred)
		}
		if err := os.Remove(preferred); err != nil {
			t.Fatal(err)
		}
		got, err = findImportedComposeFile(root, "examples/demo/compose.yml")
		if err != nil || got != fallback {
			t.Fatalf("findImportedComposeFile() = %q, %v; want fallback %q, nil", got, err, fallback)
		}
	})

	t.Run("rejects saved path below a symlinked parent directory", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		writeCompose(t, outside, "compose.yml", validCompose)
		if err := os.Symlink(outside, filepath.Join(root, "docker")); err != nil {
			t.Fatal(err)
		}

		if _, err := findImportedComposeFile(root, "docker/compose.yml"); err == nil {
			t.Fatal("expected saved path below symlinked parent to be rejected")
		}
	})

	t.Run("does not reuse a saved nonstandard compose filename", func(t *testing.T) {
		root := t.TempDir()
		writeCompose(t, root, "docker/stack.yaml", validCompose)
		want := writeCompose(t, root, "deploy/compose.yml", validCompose)

		got, err := findImportedComposeFile(root, "docker/stack.yaml")
		if err != nil || got != want {
			t.Fatalf("findImportedComposeFile() = %q, %v; want discovered standard path %q, nil", got, err, want)
		}
	})
}

func TestGitImportAllowsComposeRelativeAssetsInsideProjectRoot(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	sourceDir := filepath.Join(t.TempDir(), "source")
	composePath := filepath.Join(sourceDir, "docker", "compose.yml")
	if err := os.MkdirAll(filepath.Join(sourceDir, "docker"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceDir, "secrets"), 0755); err != nil {
		t.Fatal(err)
	}
	compose := `services:
  app:
    image: nginx
    env_file: ../.env.runtime
secrets:
  app_token:
    file: ../secrets/token.txt
`
	if err := os.WriteFile(composePath, []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, ".env.runtime"), []byte("TAG=latest\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "secrets", "token.txt"), []byte("token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"},
		{"add", "."},
		{"-c", "user.name=TRADIS Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fixture"},
	} {
		command := exec.Command("git", args...)
		command.Dir = sourceDir
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v: %s", args, err, output)
		}
	}

	runComposeGitSyncTask("git-import-nested-assets", "compose_git_import", "nested-assets", sourceDir, "", "", false)

	task, err := database.GetTask("git-import-nested-assets")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "success" {
		t.Fatalf("Git import status = %q, error = %q", task.Status, task.Error)
	}
	source, err := database.GetComposeGitSourceInEnvironment(database.LocalEnvironmentID, "nested-assets")
	if err != nil {
		t.Fatal(err)
	}
	if source.ComposePath != "docker/compose.yml" {
		t.Fatalf("saved ComposePath = %q, want docker/compose.yml", source.ComposePath)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, "nested-assets", "docker", "compose.yml")); err != nil {
		t.Fatal(err)
	}
}

func TestGitSourceUpsertFailureReportsRetainedBackupWhenRestoreFails(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	projectName := "git-rollback-failure"
	taskID := "git-sync-rollback-failure"
	projectDir := filepath.Join(projectRoot, projectName)
	backupDir := projectDir + ".git-backup-" + taskID
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yml"), []byte("services:\n  old:\n    image: nginx:old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "old-state.txt"), []byte("preserve me"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertComposeGitSource(database.ComposeGitSource{
		ProjectName: projectName,
		RepoURL:     "https://github.com/example/old.git",
		ComposePath: "compose.yml",
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertTask(taskID, "compose_git_sync", "pending"); err != nil {
		t.Fatal(err)
	}

	sourceDir := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "compose.yml"), []byte("services:\n  new:\n    image: nginx:new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"},
		{"add", "."},
		{"-c", "user.name=TRADIS Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fixture"},
	} {
		command := exec.Command("git", args...)
		command.Dir = sourceDir
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v: %s", args, err, output)
		}
	}

	originalUpsert := composeGitSourceUpsert
	originalRename := composeGitRename
	originalRemoveAll := composeGitRemoveAll
	t.Cleanup(func() {
		composeGitSourceUpsert = originalUpsert
		composeGitRename = originalRename
		composeGitRemoveAll = originalRemoveAll
	})
	upsertFailure := errors.New("injected Git source persistence failure")
	restoreFailure := errors.New("injected backup restore failure")
	composeGitSourceUpsert = func(string, database.ComposeGitSource) error { return upsertFailure }
	composeGitRename = func(oldPath, newPath string) error {
		if oldPath == backupDir && newPath == projectDir {
			return restoreFailure
		}
		return os.Rename(oldPath, newPath)
	}
	composeGitRemoveAll = os.RemoveAll

	runComposeGitSyncTask(taskID, "compose_git_sync", projectName, sourceDir, "", "", true)

	task, err := database.GetTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "error" || !strings.Contains(task.Error, upsertFailure.Error()) || !strings.Contains(task.Error, restoreFailure.Error()) || !strings.Contains(task.Error, backupDir) {
		t.Fatalf("task result = %#v", task)
	}
	if content, err := os.ReadFile(filepath.Join(backupDir, "old-state.txt")); err != nil || string(content) != "preserve me" {
		t.Fatalf("retained backup content = %q, err=%v", content, err)
	}
	if _, err := os.Stat(projectDir); !os.IsNotExist(err) {
		t.Fatalf("new project directory was not removed: %v", err)
	}
	logs, err := database.GetTaskLogsAfter(taskID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	foundRecoveryPath := false
	for _, entry := range logs {
		if strings.Contains(entry.Message, backupDir) {
			foundRecoveryPath = true
			break
		}
	}
	if !foundRecoveryPath {
		t.Fatalf("task logs do not contain manual recovery path %q: %#v", backupDir, logs)
	}
}

func TestValidateGitImportedComposeAssetPathsRejectsAssetsOutsideProjectRoot(t *testing.T) {
	projectDir := t.TempDir()
	composeDir := filepath.Join(projectDir, "docker")
	compose := `services:
  app:
    image: nginx
    env_file: ../../outside.env
secrets:
  app_token:
    file: /tmp/token.txt
`

	errs := validateGitImportedComposeAssetPaths(projectDir, composeDir, compose)
	if len(errs) != 2 {
		t.Fatalf("validation errors = %#v, want one env_file and one secret error", errs)
	}
}

func TestValidateGitImportedComposeAssetPathsRejectsSymlinkedAssetParents(t *testing.T) {
	projectDir := t.TempDir()
	composeDir := filepath.Join(projectDir, "docker")
	if err := os.MkdirAll(composeDir, 0755); err != nil {
		t.Fatal(err)
	}
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "environment"), []byte("TAG=latest\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "token.txt"), []byte("token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(composeDir, "assets")); err != nil {
		t.Fatal(err)
	}
	compose := `services:
  app:
    image: nginx
    env_file: assets/environment
secrets:
  app_token:
    file: assets/token.txt
`

	errs := validateGitImportedComposeAssetPaths(projectDir, composeDir, compose)
	if len(errs) != 2 {
		t.Fatalf("validation errors = %#v, want symlinked env_file and secret file to be rejected", errs)
	}
}

func buildTestGitArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range entries {
		header := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestExtractGitHubTarGz(t *testing.T) {
	archive := buildTestGitArchive(t, map[string]string{
		"demo-main/compose.yml":  "services:\n  web:\n    image: nginx\n",
		"demo-main/.env.example": "PORT=8080\n",
	})
	destination := t.TempDir()
	if _, err := extractGitHubTarGz(bytes.NewReader(archive), destination); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte("services:")) {
		t.Fatalf("unexpected compose content: %s", content)
	}
}

func TestExtractGitHubTarGzRejectsPathTraversal(t *testing.T) {
	archive := buildTestGitArchive(t, map[string]string{
		"demo-main/../../escape.txt": "escape",
	})
	if _, err := extractGitHubTarGz(bytes.NewReader(archive), t.TempDir()); err == nil {
		t.Fatal("expected path traversal archive to be rejected")
	}
}

func TestRejectGitSubmoduleClone(t *testing.T) {
	t.Run("rejects gitmodules", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".gitmodules"), []byte("[submodule \"x\"]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := rejectGitSubmoduleClone(dir); err == nil {
			t.Fatal("expected submodule repository to be rejected")
		}
	})
	t.Run("allows clean repo", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte("services: {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := rejectGitSubmoduleClone(dir); err != nil {
			t.Fatalf("clean repo must not be rejected: %v", err)
		}
	})
	t.Run("allows empty gitmodules", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".gitmodules"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := rejectGitSubmoduleClone(dir); err != nil {
			t.Fatalf("empty .gitmodules must not be rejected: %v", err)
		}
	})
	t.Run("allows comment only gitmodules", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".gitmodules"), []byte("# no active submodules\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := rejectGitSubmoduleClone(dir); err != nil {
			t.Fatalf("comment-only .gitmodules must not be rejected: %v", err)
		}
	})
}

func buildTestGitArchiveWithHeaders(t *testing.T, headers ...*tar.Header) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, header := range headers {
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestExtractGitHubTarGzRejectsLinkAndSpecialEntries(t *testing.T) {
	cases := []struct {
		name   string
		header *tar.Header
	}{
		{"absolute symlink", &tar.Header{Name: "demo-main/evil", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0644}},
		{"relative escape symlink", &tar.Header{Name: "demo-main/escape", Typeflag: tar.TypeSymlink, Linkname: "../../etc/passwd", Mode: 0644}},
		{"hardlink escape", &tar.Header{Name: "demo-main/hl", Typeflag: tar.TypeLink, Linkname: "../../etc/passwd", Mode: 0644}},
		{"char device", &tar.Header{Name: "demo-main/dev", Typeflag: tar.TypeChar, Mode: 0644}},
		{"fifo", &tar.Header{Name: "demo-main/pipe", Typeflag: tar.TypeFifo, Mode: 0644}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			archive := buildTestGitArchiveWithHeaders(t, tc.header)
			if _, err := extractGitHubTarGz(bytes.NewReader(archive), t.TempDir()); err == nil {
				t.Fatalf("expected archive with %s to be rejected", tc.name)
			}
		})
	}
}

func TestGitHubArchiveURL(t *testing.T) {
	got, err := githubArchiveURL("https://github.com/example/demo.git", "release/v1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://github.com/example/demo/archive/release%2Fv1.tar.gz" {
		t.Fatalf("unexpected archive URL: %s", got)
	}
}
