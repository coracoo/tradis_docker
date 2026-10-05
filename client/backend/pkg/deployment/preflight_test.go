package deployment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreflightRequiredBindSources(t *testing.T) {
	root := t.TempDir()
	for _, fixture := range []struct{ name, volumes, code string }{
		{"missing_file", "      - type: bind\n        source: ./Caddyfile\n        target: /etc/caddy/Caddyfile\n        bind:\n          create_host_path: false\n", "bind_source_missing"},
		{"data_directory", "      - ./data:/data\n", ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			deps := healthyPreflightDependencies()
			deps.PathStatus = nil
			result := Preflight(context.Background(), Intent{Kind: IntentKindCompose, BuildContext: root, ComposeYAML: "services:\n  proxy:\n    image: caddy:2\n    volumes:\n" + fixture.volumes}, validPreflightProfile(root), deps)
			issues := preflightIssueMap(result)
			if fixture.code != "" {
				if _, ok := issues[fixture.code]; !ok {
					t.Fatalf("missing %s: %#v", fixture.code, result.Issues)
				}
			} else if result.HasBlocking() {
				t.Fatalf("ordinary missing data bind must remain valid: %#v", result.Issues)
			}
		})
	}
}

func TestPreflightComposeDiagnosticIsBoundedAndRedacted(t *testing.T) {
	deps := healthyPreflightDependencies()
	deps.ComposeConfig = func(context.Context, Intent, string) error {
		return errors.New("service refers to undefined volume cache; PASSWORD=private-value; invalid literal opaque secret phrase " + strings.Repeat("x", 5000))
	}
	result := Preflight(context.Background(), Intent{ComposeYAML: "services:\n  app:\n    image: nginx\n", Parameters: map[string]any{"APP_SECRET": "opaque secret phrase"}}, validPreflightProfile(t.TempDir()), deps)
	diagnostic, _ := preflightIssueMap(result)["compose_config_invalid"].Details["diagnostic"].(string)
	if !strings.Contains(diagnostic, "undefined volume") || strings.Contains(diagnostic, "private-value") || strings.Contains(diagnostic, "opaque secret phrase") || len(diagnostic) > 2048 {
		t.Fatalf("unsafe or missing diagnostic: %q", diagnostic)
	}
}

func TestPreflightPlannedConfigFilesRemainTyped(t *testing.T) {
	for _, directory := range []bool{false, true} {
		root := t.TempDir()
		path := filepath.Join(root, "config", "Caddyfile")
		if directory {
			if err := os.MkdirAll(path, 0755); err != nil {
				t.Fatal(err)
			}
		}
		deps := healthyPreflightDependencies()
		deps.PathStatus = nil
		deps.PlannedConfigFiles = map[string]bool{path: true}
		result := Preflight(context.Background(), Intent{Kind: IntentKindCompose, BuildContext: root, ComposeYAML: "services:\n  proxy:\n    image: caddy:2\n    volumes:\n      - type: bind\n        source: ./config/Caddyfile\n        target: /etc/caddy/Caddyfile\n        bind:\n          create_host_path: false\n"}, validPreflightProfile(root), deps)
		issues := preflightIssueMap(result)
		if directory {
			if _, ok := issues["config_file_type_conflict"]; !ok {
				t.Fatalf("directory must not pass as planned file: %#v", result.Issues)
			}
		} else if result.HasBlocking() {
			t.Fatalf("validated pending file should be materialized before execution: %#v", result.Issues)
		}
	}
}

func healthyPreflightDependencies() PreflightDependencies {
	return PreflightDependencies{
		DockerInfo: func(context.Context) (DockerRuntimeInfo, error) {
			return DockerRuntimeInfo{APIVersion: "1.43", Architecture: "amd64"}, nil
		},
		PathStatus: func(string) (PathProbeResult, error) {
			return PathProbeResult{Exists: true, Directory: true, Writable: true}, nil
		},
		DiskAvailable:  func(string) (uint64, error) { return 20 << 30, nil },
		PortAvailable:  func(context.Context, int, string) (bool, error) { return true, nil },
		NetworkExists:  func(context.Context, string) (bool, error) { return true, nil },
		ImageReachable: func(context.Context, string) (bool, error) { return true, nil },
		DeviceExists:   func(string) bool { return true },
		ComposeConfig:  func(context.Context, Intent, string) error { return nil },
	}
}

func validPreflightProfile(root string) NASProfile {
	return NASProfile{
		EnvironmentID:        "local",
		ProjectRoot:          root,
		ContainerProjectRoot: root,
		PUID:                 1000,
		PGID:                 1000,
		Timezone:             "Asia/Shanghai",
		Architecture:         "amd64",
		PortRange:            PortRange{Start: 50000, End: 51000, AutoAllocate: true},
		AllowedBindRoots:     []string{root},
		Devices:              []string{"/dev/dri"},
		NetworkPolicy:        NetworkPolicy{DefaultNetwork: "bridge", AllowHost: true},
	}
}

func preflightIssueMap(result PreflightResult) map[string]PreflightIssue {
	out := make(map[string]PreflightIssue, len(result.Issues))
	for _, issue := range result.Issues {
		out[issue.Code] = issue
	}
	return out
}

func TestPreflightAcceptsCompleteComposeForNASProfile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("APP_TOKEN=configured\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	intent := Intent{
		EnvironmentID: "local",
		ProjectName:   "media",
		Kind:          IntentKindCompose,
		BuildContext:  root,
		Parameters:    map[string]any{"APP_TOKEN": "configured"},
		ComposeYAML: `services:
  app:
    image: ghcr.io/example/media:latest
    build:
      context: .
      dockerfile: Dockerfile
    environment:
      APP_TOKEN: ${APP_TOKEN:?required}
    env_file:
      - .env
    ports:
      - "50080:8080/tcp"
    volumes:
      - media-data:/data
      - ./config:/config
    networks: [proxy]
    devices:
      - /dev/dri:/dev/dri
networks:
  proxy:
    external: true
volumes:
  media-data: {}
`,
	}

	deps := healthyPreflightDependencies()
	deps.PathStatus = nil
	result := Preflight(context.Background(), intent, validPreflightProfile(root), deps)
	if result.HasBlocking() {
		t.Fatalf("complete deployment should pass preflight: %#v", result.Issues)
	}
}

func TestPreflightAcceptsDistinctMappedHostAndContainerProjectRoots(t *testing.T) {
	containerRoot := t.TempDir()
	profile := validPreflightProfile(containerRoot)
	profile.ProjectRoot = "/volume1/docker"
	profile.AllowedBindRoots = []string{profile.ProjectRoot}

	result := Preflight(context.Background(), Intent{
		EnvironmentID: "local",
		ProjectName:   "demo",
		Kind:          IntentKindCompose,
		ComposeYAML: `services:
  app:
    image: example/app:latest
    volumes:
      - /volume1/docker/demo/data:/data
`,
	}, profile, healthyPreflightDependencies())

	if issue, exists := preflightIssueMap(result)["project_root_mismatch"]; exists {
		t.Fatalf("paired host/container roots are expected to differ: %#v", issue)
	}
	if result.HasBlocking() {
		t.Fatalf("mapped host/container roots must pass preflight: %#v", result.Issues)
	}
}

func TestPreflightClassifiesNASDeploymentFailures(t *testing.T) {
	profile := validPreflightProfile("/srv/apps")
	profile.ContainerProjectRoot = "/app/projects"
	profile.Architecture = "amd64"
	profile.PortRange = PortRange{Start: 60000, End: 60001, AutoAllocate: true}
	profile.AllowedBindRoots = []string{"/srv/apps"}
	profile.NetworkPolicy.AllowHost = false

	deps := healthyPreflightDependencies()
	deps.PathStatus = func(path string) (PathProbeResult, error) {
		switch filepath.Clean(path) {
		case "/app/projects":
			return PathProbeResult{Exists: true, Directory: true, Writable: false}, nil
		case "/source":
			return PathProbeResult{Exists: true, Directory: true, Writable: true}, nil
		default:
			return PathProbeResult{Exists: false}, nil
		}
	}
	deps.DiskAvailable = func(string) (uint64, error) { return 64 << 20, nil }
	deps.PortAvailable = func(context.Context, int, string) (bool, error) { return false, nil }
	deps.NetworkExists = func(context.Context, string) (bool, error) { return false, nil }
	deps.ImageReachable = func(context.Context, string) (bool, error) { return false, nil }
	deps.DeviceExists = func(string) bool { return false }

	result := Preflight(context.Background(), Intent{
		EnvironmentID: "local",
		ProjectName:   "broken",
		Kind:          IntentKindCompose,
		BuildContext:  "/source",
		Options:       map[string]any{"requiredDiskBytes": uint64(1 << 30)},
		ComposeYAML: `services:
  app:
    image: registry.invalid/example/app:latest
    platform: linux/arm64
    network_mode: host
    build:
      context: ./missing
      dockerfile: Dockerfile
    environment:
      REQUIRED: ${REQUIRED:?required}
    env_file: [.env]
    ports: ["8080:80"]
    volumes:
      - /outside/data:/data
    networks: [proxy]
    devices: [/dev/dri:/dev/dri]
  worker:
    image: registry.invalid/example/worker:latest
    build:
      context: .
      dockerfile: Missing.Dockerfile
    ports: ["8081:81"]
networks:
  proxy:
    external: true
`,
	}, profile, deps)

	issues := preflightIssueMap(result)
	for _, code := range []string{
		"project_root_not_writable",
		"disk_space_low",
		"architecture_mismatch",
		"port_in_use",
		"port_pool_exhausted",
		"required_env_missing",
		"env_file_missing",
		"external_network_missing",
		"build_context_missing",
		"dockerfile_missing",
		"device_missing",
		"image_unreachable",
		"bind_root_not_allowed",
		"host_network_not_allowed",
	} {
		if _, ok := issues[code]; !ok {
			t.Errorf("missing preflight issue %q: %#v", code, result.Issues)
		}
	}
	if !result.HasBlocking() {
		t.Fatal("unsafe NAS deployment must contain blocking issues")
	}
	if issues["image_unreachable"].Severity != PreflightWarning {
		t.Fatalf("registry reachability must remain retryable warning: %#v", issues["image_unreachable"])
	}
}

func TestPreflightReportsDockerAndComposeFailures(t *testing.T) {
	deps := healthyPreflightDependencies()
	deps.DockerInfo = func(context.Context) (DockerRuntimeInfo, error) {
		return DockerRuntimeInfo{}, errors.New("socket unavailable")
	}
	result := Preflight(context.Background(), Intent{
		EnvironmentID: "local",
		ProjectName:   "invalid",
		Kind:          IntentKindCompose,
		ComposeYAML:   "services: [",
	}, validPreflightProfile(t.TempDir()), deps)

	issues := preflightIssueMap(result)
	for _, code := range []string{"docker_unavailable", "compose_invalid"} {
		if issue, ok := issues[code]; !ok || issue.Severity != PreflightBlocking || len(issue.NextActions) == 0 {
			t.Errorf("expected actionable blocking issue %q: %#v", code, issue)
		}
	}
}

func TestPreflightRejectsEscapingEnvFilePath(t *testing.T) {
	root := t.TempDir()
	intent := Intent{
		EnvironmentID: "local", ProjectName: "evil-env", Kind: IntentKindCompose,
		BuildContext: root,
		ComposeYAML: `services:
  app:
    image: scratch
    env_file:
      - ../../../etc/passwd
`,
	}
	result := Preflight(context.Background(), intent, validPreflightProfile(root), healthyPreflightDependencies())
	if _, ok := preflightIssueMap(result)["env_file_path_escape"]; !ok {
		t.Fatalf("expected env_file_path_escape blocking issue, got %#v", result.Issues)
	}
}

func TestPreflightExpandsEnvFileDefaultBeforeCheckingPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("APP_MODE=prod\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deps := healthyPreflightDependencies()
	deps.PathStatus = nil
	result := Preflight(context.Background(), Intent{
		EnvironmentID: "local", ProjectName: "env-default", Kind: IntentKindCompose,
		BuildContext: root,
		ComposeYAML: `services:
  app:
    image: scratch
    env_file: ${APP_ENV_FILE:-.env}
`,
	}, validPreflightProfile(root), deps)
	if issue, ok := preflightIssueMap(result)["env_file_missing"]; ok {
		t.Fatalf("expanded default .env exists and must pass: %#v", issue)
	}
	if issue, ok := preflightIssueMap(result)["env_file_interpolation_invalid"]; ok {
		t.Fatalf("valid default interpolation must pass: %#v", issue)
	}
}

func TestPreflightChecksExpandedEnvFileBoundary(t *testing.T) {
	root := t.TempDir()
	result := Preflight(context.Background(), Intent{
		EnvironmentID: "local", ProjectName: "env-escape", Kind: IntentKindCompose,
		BuildContext: root,
		Parameters:   map[string]any{"APP_ENV_FILE": "../../outside.env"},
		ComposeYAML: `services:
  app:
    image: scratch
    env_file: ${APP_ENV_FILE:-.env}
`,
	}, validPreflightProfile(root), healthyPreflightDependencies())
	if _, ok := preflightIssueMap(result)["env_file_path_escape"]; !ok {
		t.Fatalf("expanded env_file escaping the source root must be rejected: %#v", result.Issues)
	}
}

func TestPreflightRejectsEscapingBuildContext(t *testing.T) {
	root := t.TempDir()
	intent := Intent{
		EnvironmentID: "local", ProjectName: "evil-build", Kind: IntentKindCompose,
		BuildContext: root,
		ComposeYAML: `services:
  app:
    image: scratch
    build:
      context: ../../outside
      dockerfile: Dockerfile
`,
	}
	result := Preflight(context.Background(), intent, validPreflightProfile(root), healthyPreflightDependencies())
	if _, ok := preflightIssueMap(result)["build_context_path_escape"]; !ok {
		t.Fatalf("expected build_context_path_escape blocking issue, got %#v", result.Issues)
	}
}

func TestPreflightRejectsEscapingDockerfilePath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	intent := Intent{
		EnvironmentID: "local", ProjectName: "evil-dockerfile", Kind: IntentKindCompose,
		BuildContext: root,
		ComposeYAML: `services:
  app:
    image: scratch
    build:
      context: .
      dockerfile: ../../outside/Dockerfile
`,
	}
	result := Preflight(context.Background(), intent, validPreflightProfile(root), healthyPreflightDependencies())
	if _, ok := preflightIssueMap(result)["dockerfile_path_escape"]; !ok {
		t.Fatalf("expected dockerfile_path_escape blocking issue, got %#v", result.Issues)
	}
}

func TestPreflightAcceptsSymlinkedSourceRootWhenCandidateStaysInsideResolvedRoot(t *testing.T) {
	realRoot := t.TempDir()
	linkParent := t.TempDir()
	linkedRoot := filepath.Join(linkParent, "source")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realRoot, ".env"), []byte("TOKEN=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := Preflight(context.Background(), Intent{
		EnvironmentID: "local", ProjectName: "linked-source", Kind: IntentKindCompose,
		BuildContext: linkedRoot,
		ComposeYAML: `services:
  app:
    image: nginx:latest
    env_file: [.env]
`,
	}, validPreflightProfile(linkedRoot), healthyPreflightDependencies())
	if issue, exists := preflightIssueMap(result)["env_file_path_escape"]; exists {
		t.Fatalf("path inside a resolved symlink root must pass: %#v", issue)
	}
}

func TestPreflightChecksExpandedComposeConfiguration(t *testing.T) {
	deps := healthyPreflightDependencies()
	deps.ComposeConfig = func(_ context.Context, intent Intent, sourceDir string) error {
		if intent.ComposeYAML == "" || sourceDir == "" {
			t.Fatal("compose config validator must receive source YAML and project directory")
		}
		return errors.New("required variable is not set")
	}

	result := Preflight(context.Background(), Intent{
		EnvironmentID: "local",
		ProjectName:   "invalid-config",
		Kind:          IntentKindCompose,
		ComposeYAML: `services:
  app:
    image: nginx:latest
`,
	}, validPreflightProfile(t.TempDir()), deps)

	issue, exists := preflightIssueMap(result)["compose_config_invalid"]
	if !exists || issue.Severity != PreflightBlocking {
		t.Fatalf("expanded Compose errors must block deployment: %#v", result.Issues)
	}
	if len(issue.NextActions) == 0 {
		t.Fatalf("expanded Compose errors must be actionable: %#v", issue)
	}
}

func TestPreflightAllowsDeviceBelowConfiguredDeviceRoot(t *testing.T) {
	root := t.TempDir()
	profile := validPreflightProfile(root)
	profile.Devices = []string{"/dev/dri"}
	deps := healthyPreflightDependencies()

	result := Preflight(context.Background(), Intent{
		EnvironmentID: "local",
		ProjectName:   "hardware",
		Kind:          IntentKindCompose,
		ComposeYAML: `services:
  app:
    image: example/media:latest
    devices:
      - /dev/dri/renderD128:/dev/dri/renderD128
`,
	}, profile, deps)

	if issue, exists := preflightIssueMap(result)["device_not_allowed"]; exists {
		t.Fatalf("device below configured root must be allowed: %#v", issue)
	}
}

func TestPreflightFindsRequiredVariablesOutsideServiceEnvironment(t *testing.T) {
	root := t.TempDir()
	result := Preflight(context.Background(), Intent{
		EnvironmentID: "local",
		ProjectName:   "required-image",
		Kind:          IntentKindCompose,
		ComposeYAML: `services:
  app:
    image: ${REGISTRY_HOST:?required}/example/app:latest
`,
	}, validPreflightProfile(root), healthyPreflightDependencies())

	issue, exists := preflightIssueMap(result)["required_env_missing"]
	if !exists {
		t.Fatalf("required interpolation outside environment was not detected: %#v", result.Issues)
	}
	names, _ := issue.Details["names"].([]string)
	if len(names) != 1 || names[0] != "REGISTRY_HOST" {
		t.Fatalf("unexpected missing environment names: %#v", issue.Details)
	}
}
