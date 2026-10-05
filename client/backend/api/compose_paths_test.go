package api

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dockerpanel/backend/pkg/database"
)

func TestNormalizeComposeBindMountsForRuntimeUsesHostProjectRoot(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "/srv/tradis/projects")

	input := `
services:
  app:
    image: nginx:latest
    volumes:
      - ./data:/data
      - ../shared:/shared
      - named-data:/var/lib/data
      - type: bind
        source: .
        target: /workspace
volumes:
  named-data:
`

	out, err := normalizeComposeBindMountsForRuntime(input, "demo")
	if err != nil {
		t.Fatalf("normalizeComposeBindMountsForRuntime returned error: %v", err)
	}

	for _, expected := range []string{
		"${PROJECT_ROOT}/demo/data:/data",
		"${PROJECT_ROOT}/shared:/shared",
		"source: ${PROJECT_ROOT}/demo",
		"named-data:/var/lib/data",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected normalized compose to contain %q, got:\n%s", expected, out)
		}
	}
}

func TestComposeHasRelativeBindMounts(t *testing.T) {
	input := `
services:
  app:
    image: nginx:latest
    volumes:
      - ./data:/data
  db:
    image: postgres:16
    volumes:
      - db-data:/var/lib/postgresql/data
volumes:
  db-data:
`

	if !composeHasRelativeBindMounts(input) {
		t.Fatal("expected relative bind mount to be detected")
	}
}

func TestNormalizeComposeBindMountsRejectsProjectRootEscape(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "/srv/tradis/projects")

	input := `
services:
  app:
    image: nginx:latest
    volumes:
      - ../../etc:/host-etc
`

	_, err := normalizeComposeBindMountsForRuntime(input, "demo")
	if err == nil {
		t.Fatal("expected PROJECT_ROOT escape to be rejected")
	}
	if !strings.Contains(err.Error(), "超出 PROJECT_ROOT") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeComposeBindMountsAllowsSharedPathWithinProjectRoot(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "/srv/tradis/projects")

	input := `
services:
  app:
    image: nginx:latest
    volumes:
      - ../shared:/shared
`

	out, err := normalizeComposeBindMountsForRuntime(input, "demo")
	if err != nil {
		t.Fatalf("expected shared path within PROJECT_ROOT to be allowed: %v", err)
	}
	if !strings.Contains(out, "${PROJECT_ROOT}/shared:/shared") {
		t.Fatalf("unexpected normalized compose:\n%s", out)
	}
}

func TestValidatePathWithinRootHandlesAbsolutePaths(t *testing.T) {
	root := "/srv/tradis/projects"
	if err := validatePathWithinRoot(root, "/srv/tradis/projects/demo/data"); err != nil {
		t.Fatalf("expected absolute child path to be allowed: %v", err)
	}
	if err := validatePathWithinRoot(root, "/srv/tradis/etc"); err == nil {
		t.Fatal("expected absolute path outside root to be rejected")
	}
}

func TestResolveProjectDirRejectsExistingSymlinkOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "demo")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if _, err := resolveProjectDir(root, "demo"); err == nil {
		t.Fatal("project directory symlink escaping root must be rejected")
	}
	if err := validatePathWithinRoot(root, filepath.Join(root, "demo", "compose.yml")); err == nil {
		t.Fatal("path below project symlink escaping root must be rejected")
	}
}

func TestResolveExistingManagedProjectDirPreservesExactDirectoryName(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"MyApp", "好好的11", "Media Project"} {
		expected := filepath.Join(root, name)
		if err := os.MkdirAll(expected, 0755); err != nil {
			t.Fatal(err)
		}
		got, err := resolveExistingManagedProjectDir(root, name)
		if err != nil {
			t.Fatalf("resolveExistingManagedProjectDir(%q) error = %v", name, err)
		}
		if got != expected {
			t.Fatalf("resolveExistingManagedProjectDir(%q) = %q, want %q", name, got, expected)
		}
	}
}

func TestResolveExistingManagedProjectDirRejectsUnsafeReferences(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"", ".", "..", "../outside", "nested/project", `nested\project`} {
		if _, err := resolveExistingManagedProjectDir(root, name); err == nil {
			t.Fatalf("resolveExistingManagedProjectDir(%q) should fail", name)
		}
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if _, err := resolveExistingManagedProjectDir(root, "linked"); err == nil {
		t.Fatal("symlink escaping the managed root should fail")
	}
}

func TestNormalizeComposeBindMountsTranslatesAbsoluteContainerPaths(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "/srv/tradis/projects")

	containerRoot := getProjectsBaseDir()
	input := fmt.Sprintf(`
services:
  app:
    image: nginx:latest
    volumes:
      - %s/demo/data:/data
      - %s/shared:/shared:ro
      - type: bind
        source: %s/demo/config
        target: /config
`, containerRoot, containerRoot, containerRoot)

	out, err := normalizeComposeBindMountsForRuntime(input, "demo")
	if err != nil {
		t.Fatalf("expected absolute container path translation to succeed: %v", err)
	}
	for _, expected := range []string{
		"${PROJECT_ROOT}/demo/data:/data",
		"${PROJECT_ROOT}/shared:/shared:ro",
		"source: ${PROJECT_ROOT}/demo/config",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected normalized compose to contain %q, got:\n%s", expected, out)
		}
	}
}

func TestNormalizeComposeBindMountsForRuntimeUsesProjectRootVariable(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "/srv/tradis/projects")

	input := `
services:
  app:
    image: nginx:latest
    volumes:
      - ./data:/data
      - type: bind
        source: ../shared
        target: /shared
`

	out, err := normalizeComposeBindMountsForRuntime(input, "demo")
	if err != nil {
		t.Fatalf("normalizeComposeBindMountsForRuntime returned error: %v", err)
	}
	for _, expected := range []string{
		"${PROJECT_ROOT}/demo/data:/data",
		"source: ${PROJECT_ROOT}/shared",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected runtime compose to contain %q, got:\n%s", expected, out)
		}
	}
	if composeHasRelativeBindMounts(out) {
		t.Fatalf("runtime compose still contains relative bind mounts:\n%s", out)
	}
}

func TestRuntimeNormalizationPreservesAbsoluteBindAndEnvFile(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "/srv/tradis/projects")
	input := `
services:
  app:
    image: nginx:latest
    env_file:
      - ./service.env
    volumes:
      - /mnt/media:/media:ro
      - named-data:/data
volumes:
  named-data:
`

	out, err := normalizeComposeBindMountsForRuntime(input, "demo")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"/mnt/media:/media:ro",
		"named-data:/data",
		"./service.env",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q to be preserved, got:\n%s", expected, out)
		}
	}
}

func TestRuntimeNormalizationExpandsRelativeVolumeDefault(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "/srv/tradis/projects")
	input := `
services:
  app:
    image: nginx:latest
    volumes:
      - ${DATA_PATH:-./data/uploads}:/uploads
`
	out, err := normalizeComposeBindMountsForRuntime(input, "demo", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "${PROJECT_ROOT}/demo/data/uploads:/uploads") {
		t.Fatalf("unexpected runtime compose:\n%s", out)
	}

	out, err = normalizeComposeBindMountsForRuntime(input, "demo", map[string]string{
		"DATA_PATH": "/mnt/storage/uploads",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "${DATA_PATH:-./data/uploads}:/uploads") {
		t.Fatalf("expected explicit absolute variable path to remain user-controlled:\n%s", out)
	}
}

func TestCopyFirstStandardExample(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".env.example"), []byte("PORT=8080\n"), 0640); err != nil {
		t.Fatal(err)
	}
	example, err := copyFirstStandardExample(projectDir, ".env")
	if err != nil {
		t.Fatal(err)
	}
	if example != ".env.example" {
		t.Fatalf("unexpected example: %s", example)
	}
	content, err := os.ReadFile(filepath.Join(projectDir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "PORT=8080\n" {
		t.Fatalf("unexpected copied content: %s", content)
	}
}

func TestPrepareComposeCommandKeepsContainerProjectDirectoryAndUsesRuntimeFile(t *testing.T) {
	hostRoot := filepath.Join(t.TempDir(), "host-projects")
	t.Setenv("PROJECT_ROOT", hostRoot)

	projectDir := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(projectDir, "docker-compose.yml")
	source := "services:\n  app:\n    image: nginx:latest\n    volumes:\n      - ./data:/data\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte("TAG=latest\n"), 0644); err != nil {
		t.Fatal(err)
	}

	args, env, cleanup, err := prepareComposeCommand(projectDir, []string{"compose", "up", "-d"})
	if err != nil {
		t.Fatalf("prepareComposeCommand returned error: %v", err)
	}
	defer cleanup()

	joined := strings.Join(args, "\n")
	for _, expected := range []string{
		"--project-directory\n" + projectDir,
		"--env-file\n" + filepath.Join(projectDir, ".env"),
		"up\n-d",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected args to contain %q, got: %#v", expected, args)
		}
	}

	runtimePath := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--file" {
			runtimePath = args[i+1]
			break
		}
	}
	if runtimePath == "" || runtimePath == sourcePath {
		t.Fatalf("expected a separate runtime compose file, got %q", runtimePath)
	}
	expectedRuntimePath := filepath.Join(hostRoot, "demo", filepath.Base(sourcePath))
	if runtimePath != expectedRuntimePath {
		t.Fatalf("expected runtime path %q, got %q", expectedRuntimePath, runtimePath)
	}
	runtimeContent, err := os.ReadFile(runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runtimeContent), "${PROJECT_ROOT}/demo/data:/data") {
		t.Fatalf("unexpected runtime compose:\n%s", runtimeContent)
	}
	originalContent, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(originalContent) != source {
		t.Fatalf("source compose was modified:\n%s", originalContent)
	}
	if !containsString(env, "PROJECT_ROOT="+hostRoot) {
		t.Fatalf("PROJECT_ROOT missing from command env: %#v", env)
	}
	cleanup()
	if _, err := os.Stat(runtimePath); !os.IsNotExist(err) {
		t.Fatalf("expected runtime compose file to be removed, got %v", err)
	}
}

func TestComposeCommandEnvironmentPrefersProjectDotenvOverBackendEnvironment(t *testing.T) {
	t.Setenv("JWT_SECRET", "tradis-backend-secret")
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("JWT_SECRET=project-secret\nAPP_MODE=project\n"), 0600); err != nil {
		t.Fatal(err)
	}

	env := composeCommandEnvironment(
		dir,
		[]string{"compose", "--env-file", envPath, "config"},
		[]string{"APP_MODE=explicit"},
		[]string{"PROJECT_ROOT=/volume1/docker"},
	)
	lastValue := func(key string) string {
		value := ""
		for _, entry := range env {
			if name, current, ok := strings.Cut(entry, "="); ok && name == key {
				value = current
			}
		}
		return value
	}
	if got := lastValue("JWT_SECRET"); got != "" {
		t.Fatalf("project dotenv must be parsed by Compose, not overridden by process environment, got %q", got)
	}
	if got := lastValue("APP_MODE"); got != "explicit" {
		t.Fatalf("explicit deployment environment must override project dotenv, got %q", got)
	}
	if got := lastValue("PROJECT_ROOT"); got != "/volume1/docker" {
		t.Fatalf("compose runtime environment missing, got %q", got)
	}
}

func TestPrepareComposeCommandWithSourceUsesExplicitProjectLocalCompose(t *testing.T) {
	hostRoot := filepath.Join(t.TempDir(), "host-projects")
	t.Setenv("PROJECT_ROOT", hostRoot)
	projectDir := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services:\n  app:\n    image: nginx:latest\n"), 0644); err != nil {
		t.Fatal(err)
	}
	rollbackPath := filepath.Join(projectDir, ".tradis-safe-update.yaml")
	if err := os.WriteFile(rollbackPath, []byte("services:\n  app:\n    image: nginx@sha256:previous\n"), 0644); err != nil {
		t.Fatal(err)
	}

	args, _, cleanup, err := prepareComposeCommandWithSource(projectDir, []string{"compose", "up", "-d"}, rollbackPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	runtimePath := ""
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--file" {
			runtimePath = args[index+1]
			break
		}
	}
	content, err := os.ReadFile(runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "nginx@sha256:previous") || strings.Contains(string(content), "nginx:latest") {
		t.Fatalf("runtime compose did not use supplied source:\n%s", content)
	}
}

func TestPrepareComposeCommandWithSourceKeepsNestedComposePathSemantics(t *testing.T) {
	hostRoot := filepath.Join(t.TempDir(), "host-projects")
	t.Setenv("PROJECT_ROOT", hostRoot)
	projectDir := filepath.Join(t.TempDir(), "demo")
	composeDir := filepath.Join(projectDir, "docker")
	if err := os.MkdirAll(composeDir, 0755); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(composeDir, "compose.yml")
	content := `services:
  app:
    build: ..
    env_file: .env.runtime
    volumes:
      - ./data:/data
`
	if err := os.WriteFile(composePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(composeDir, ".env.runtime"), []byte("TAG=latest\n"), 0644); err != nil {
		t.Fatal(err)
	}

	args, _, cleanup, err := prepareComposeCommandWithSource(projectDir, []string{"compose", "up", "-d"}, composePath)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	joined := strings.Join(args, "\n")
	for _, expected := range []string{
		"--project-name\ndemo",
		"--project-directory\n" + composeDir,
		"--file\n" + filepath.Join(hostRoot, "demo", "docker", "compose.yml"),
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected args to contain %q, got: %#v", expected, args)
		}
	}

	runtimePath := filepath.Join(hostRoot, "demo", "docker", "compose.yml")
	runtimeContent, err := os.ReadFile(runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runtimeContent), "${PROJECT_ROOT}/demo/docker/data:/data") {
		t.Fatalf("nested bind mount did not retain Compose directory: %s", runtimeContent)
	}
	if !strings.Contains(string(runtimeContent), "build: ..") || !strings.Contains(string(runtimeContent), "env_file: .env.runtime") {
		t.Fatalf("nested Compose-relative fields changed unexpectedly: %s", runtimeContent)
	}
}

func TestPrepareGitComposeProjectFilesResolvesTemplatesFromNestedComposeDirectory(t *testing.T) {
	_ = database.Close()
	if err := database.InitDB(filepath.Join(t.TempDir(), "data.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	projectDir := filepath.Join(t.TempDir(), "demo")
	composeDir := filepath.Join(projectDir, "docker")
	if err := os.MkdirAll(composeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.runtime.example"), []byte("TAG=latest\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertComposeGitSource(database.ComposeGitSource{
		ProjectName: "demo",
		RepoURL:     "https://github.com/example/demo.git",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := prepareGitComposeProjectFiles(projectDir, composeDir, "services:\n  app:\n    image: nginx\n    env_file: ../.env.runtime\n")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(projectDir, ".env.runtime"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "TAG=latest\n" {
		t.Fatalf("unexpected copied template: %q", content)
	}
}

func TestPrepareComposeCommandDoesNotCleanupAliasedSourceFile(t *testing.T) {
	root := t.TempDir()
	realHostRoot := filepath.Join(root, "real-projects")
	aliasHostRoot := filepath.Join(root, "alias-projects")
	if err := os.MkdirAll(realHostRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realHostRoot, aliasHostRoot); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROJECT_ROOT", aliasHostRoot)

	projectDir := filepath.Join(realHostRoot, "demo")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(projectDir, "docker-compose.yml")
	source := "services:\n  app:\n    image: nginx:latest\n    volumes:\n      - ./data:/data\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	args, _, cleanup, err := prepareComposeCommand(projectDir, []string{"compose", "up", "-d"})
	if err != nil {
		t.Fatalf("prepareComposeCommand returned error: %v", err)
	}

	runtimePath := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--file" {
			runtimePath = args[i+1]
			break
		}
	}
	if runtimePath == "" {
		t.Fatalf("--file not found in args: %#v", args)
	}
	if !strings.Contains(runtimePath, string(filepath.Separator)+".tradis-runtime"+string(filepath.Separator)) {
		t.Fatalf("expected aliased source to use hidden runtime file, got %q", runtimePath)
	}

	cleanup()
	if _, err := os.Stat(runtimePath); !os.IsNotExist(err) {
		t.Fatalf("expected hidden runtime file to be removed, got %v", err)
	}
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("source compose should remain after cleanup: %v", err)
	}
	if string(content) != source {
		t.Fatalf("source compose was modified:\n%s", content)
	}
}

func TestPrepareComposeCommandUsesHostNamedPathWithoutRelativeVolumes(t *testing.T) {
	hostRoot := filepath.Join(t.TempDir(), "host-projects")
	t.Setenv("PROJECT_ROOT", hostRoot)

	projectDir := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(projectDir, "compose.yaml")
	source := "services:\n  app:\n    image: nginx:latest\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	args, _, cleanup, err := prepareComposeCommand(projectDir, []string{"compose", "up", "-d"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	expectedRuntimePath := filepath.Join(hostRoot, "demo", "compose.yaml")
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--file" {
			if args[i+1] != expectedRuntimePath {
				t.Fatalf("expected runtime path %q, got %q", expectedRuntimePath, args[i+1])
			}
			content, err := os.ReadFile(args[i+1])
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(content), "image: nginx:latest") {
				t.Fatalf("unexpected runtime compose:\n%s", content)
			}
			return
		}
	}
	t.Fatalf("--file not found in args: %#v", args)
}

func TestComposeUpdateCommandsCreateWithoutStarting(t *testing.T) {
	pullArgs, createArgs := composeUpdateCommands()
	if got := strings.Join(pullArgs, " "); got != "compose pull" {
		t.Fatalf("unexpected pull command: %s", got)
	}
	if got := strings.Join(createArgs, " "); got != "compose create --remove-orphans" {
		t.Fatalf("unexpected create command: %s", got)
	}
	for _, arg := range createArgs {
		if arg == "up" || arg == "start" || arg == "-d" {
			t.Fatalf("update command must not start containers: %#v", createArgs)
		}
	}
}

func TestComposeUpdateApplyCommandPreservesProjectState(t *testing.T) {
	label, args, result := composeUpdateApplyCommand(true)
	if label == "" || result == "" {
		t.Fatal("running update should include user-facing status")
	}
	if got := strings.Join(args, " "); got != "compose up -d --remove-orphans" {
		t.Fatalf("running project should be restored, got: %s", got)
	}

	label, args, result = composeUpdateApplyCommand(false)
	if label == "" || result == "" {
		t.Fatal("stopped update should include user-facing status")
	}
	if got := strings.Join(args, " "); got != "compose create --remove-orphans" {
		t.Fatalf("stopped project should remain created, got: %s", got)
	}
}

func TestComposeYAMLHasServicesAtRootRegardlessOfOrder(t *testing.T) {
	valid := []byte(`
name: demo
networks:
  default: {}
services:
  app:
    image: nginx:latest
`)
	if !composeYAMLHasServices(valid) {
		t.Fatal("expected root-level services mapping to be accepted")
	}

	for _, invalid := range [][]byte{
		[]byte("metadata:\n  services:\n    app: nginx\n"),
		[]byte("services: []\n"),
		[]byte("services: {}\n"),
		[]byte("not: [valid\n"),
	} {
		if composeYAMLHasServices(invalid) {
			t.Fatalf("expected invalid compose YAML to be rejected:\n%s", invalid)
		}
	}
}

func TestFindComposeFileIgnoresUnrelatedYAML(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "config.yaml"), []byte("theme: dark\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := findComposeFile(projectDir); !os.IsNotExist(err) {
		t.Fatalf("expected unrelated YAML to be ignored, got %v", err)
	}

	customPath := filepath.Join(projectDir, "stack.yaml")
	if err := os.WriteFile(customPath, []byte("name: demo\nservices:\n  app:\n    image: nginx\n"), 0644); err != nil {
		t.Fatal(err)
	}
	found, err := findComposeFile(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if found != customPath {
		t.Fatalf("expected %s, got %s", customPath, found)
	}
}

func TestFindComposeFileDistinguishesGitSourceAndDatabaseErrors(t *testing.T) {
	_ = database.Close()
	if err := database.InitDB(filepath.Join(t.TempDir(), "compose-find.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	originalLookup := composeGitSourceLookup
	t.Cleanup(func() { composeGitSourceLookup = originalLookup })
	writeCustomCompose := func(t *testing.T) (string, string) {
		t.Helper()
		projectDir := t.TempDir()
		customPath := filepath.Join(projectDir, "stack.yaml")
		if err := os.WriteFile(customPath, []byte("services:\n  app:\n    image: nginx\n"), 0644); err != nil {
			t.Fatal(err)
		}
		return projectDir, customPath
	}

	t.Run("manual project falls back after no Git source", func(t *testing.T) {
		projectDir, customPath := writeCustomCompose(t)
		composeGitSourceLookup = func(string, string) (*database.ComposeGitSource, error) {
			return nil, sql.ErrNoRows
		}
		found, err := findComposeFile(projectDir)
		if err != nil || found != customPath {
			t.Fatalf("findComposeFile() = %q, %v; want %q, nil", found, err, customPath)
		}
	})

	t.Run("Git project does not fall back when bounded discovery fails", func(t *testing.T) {
		projectDir, customPath := writeCustomCompose(t)
		composeGitSourceLookup = func(string, string) (*database.ComposeGitSource, error) {
			return &database.ComposeGitSource{ProjectName: filepath.Base(projectDir), ComposePath: "docker/compose.yml"}, nil
		}
		found, err := findComposeFile(projectDir)
		if err == nil || found != "" || !strings.Contains(err.Error(), "Git Compose") {
			t.Fatalf("findComposeFile() = %q, %v; must not fall back to %q", found, err, customPath)
		}
	})

	t.Run("database failure is returned", func(t *testing.T) {
		projectDir, _ := writeCustomCompose(t)
		databaseFailure := errors.New("compose source database unavailable")
		composeGitSourceLookup = func(string, string) (*database.ComposeGitSource, error) {
			return nil, databaseFailure
		}
		_, err := findComposeFile(projectDir)
		if !errors.Is(err, databaseFailure) {
			t.Fatalf("findComposeFile() error = %v, want database failure", err)
		}
	})
}

func TestCanonicalComposeProjectPathMapsHostPathToContainerPath(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "/srv/tradis/projects")
	containerRoot := filepath.Clean(getProjectsBaseDir())

	got := canonicalComposeProjectPath("/srv/tradis/projects/demo", "different-label")
	want := filepath.Join(containerRoot, "demo")
	if got != want {
		t.Fatalf("expected canonical path %s, got %s", want, got)
	}

	got = canonicalComposeProjectPath(filepath.Join(containerRoot, "demo"), "demo")
	if got != want {
		t.Fatalf("expected container path to remain %s, got %s", want, got)
	}
}

func TestCanonicalComposeProjectPathCollapsesNestedWorkingDirectoryToProjectName(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "/srv/tradis/projects")
	containerRoot := filepath.Clean(getProjectsBaseDir())
	want := filepath.Join(containerRoot, "demo")

	for _, workingDir := range []string{
		"/srv/tradis/projects/demo/docker",
		filepath.Join(containerRoot, "demo", "docker"),
	} {
		if got := canonicalComposeProjectPath(workingDir, "demo"); got != want {
			t.Fatalf("canonicalComposeProjectPath(%q, demo) = %q, want %q", workingDir, got, want)
		}
	}
}

func TestCanonicalComposeObservedProjectPathDoesNotClaimExternalSameNameStack(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	if err := os.MkdirAll(filepath.Join(projectRoot, "demo"), 0755); err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(t.TempDir(), "external-stack")
	if err := os.MkdirAll(externalPath, 0755); err != nil {
		t.Fatal(err)
	}

	if got := canonicalComposeObservedProjectPath(externalPath, "demo"); got != filepath.Clean(externalPath) {
		t.Fatalf("strict observed path = %q, want %q", got, filepath.Clean(externalPath))
	}
	if got := canonicalComposeProjectPath(externalPath, "demo"); got != filepath.Join(projectRoot, "demo") {
		t.Fatalf("legacy fallback path = %q, want %q", got, filepath.Join(projectRoot, "demo"))
	}
}

func containsString(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}
