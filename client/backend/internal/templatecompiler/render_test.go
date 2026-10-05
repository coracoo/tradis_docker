package templatecompiler

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRenderPatchesOwnedMappingsAndFiles(t *testing.T) {
	bundle := multiServiceBundle()
	compiled, err := Compile(bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := Render(bundle, compiled.Manifest, map[string]string{
		"project:POSTGRES_PASSWORD":         "changed",
		"port:api:5432/tcp":                 "50123",
		"bind:api:/var/lib/postgresql/data": "/mnt/postgres",
		"env_file:api.env:SHARED_KEY":       "api-changed",
		"env_file:worker.env:SHARED_KEY":    "worker-changed",
		"secret:db_password":                "secret-value",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.Compose, "50123:5432") {
		t.Fatalf("rendered port missing:\n%s", rendered.Compose)
	}
	if !strings.Contains(rendered.Compose, "/mnt/postgres:/var/lib/postgresql/data") {
		t.Fatalf("rendered bind missing:\n%s", rendered.Compose)
	}
	if !strings.Contains(rendered.Compose, "shared-data:/shared") {
		t.Fatalf("named volume was lost:\n%s", rendered.Compose)
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(rendered.Compose), &doc); err != nil {
		t.Fatal(err)
	}
	services := doc["services"].(map[string]any)
	api := services["api"].(map[string]any)
	if api["x-extra"] == nil {
		t.Fatal("opaque extension was lost")
	}
	assertRenderedFile(t, rendered.Files, ".env", "POSTGRES_PASSWORD=changed", 0o644)
	assertRenderedFile(t, rendered.Files, "api.env", "SHARED_KEY=api-changed", 0o644)
	assertRenderedFile(t, rendered.Files, "worker.env", "SHARED_KEY=worker-changed", 0o644)
	assertRenderedFile(t, rendered.Files, "secrets/db_password", "secret-value", 0o600)
}

func TestRenderRejectsStaleManifest(t *testing.T) {
	bundle := multiServiceBundle()
	compiled, err := Compile(bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Dotenv += "NEW_VALUE=1\n"
	if _, err := Render(bundle, compiled.Manifest, nil, nil); err == nil {
		t.Fatal("Render accepted stale source")
	}
}

func TestRenderOverlayUpdatesExistingPortTarget(t *testing.T) {
	bundle := multiServiceBundle()
	compiled, err := Compile(bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := Render(bundle, compiled.Manifest, map[string]string{
		"project:POSTGRES_PASSWORD": "changed",
		"secret:db_password":        "secret-value",
	}, []MappingOverlay{{
		ID:       "port:api:5432/tcp",
		Kind:     "port",
		Service:  "api",
		Source:   "50124",
		Target:   "6432",
		Protocol: "tcp",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.Compose, "50124:6432") {
		t.Fatalf("existing port was not updated:\n%s", rendered.Compose)
	}
	if strings.Count(rendered.Compose, "50124:6432") != 1 {
		t.Fatalf("existing port was duplicated:\n%s", rendered.Compose)
	}
	if strings.Contains(rendered.Compose, "15432:5432") {
		t.Fatalf("old port mapping remained after update:\n%s", rendered.Compose)
	}
}

func TestRenderAppendsEnvironmentOverlayWithoutRebuildingExistingEnvironment(t *testing.T) {
	bundle := multiServiceBundle()
	compiled, err := Compile(bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := Render(bundle, compiled.Manifest, map[string]string{
		"project:POSTGRES_PASSWORD": "changed",
		"secret:db_password":        "secret-value",
	}, []MappingOverlay{{
		ID:      "custom:environment:api:TZ",
		Kind:    "environment",
		Service: "api",
		Source:  "TZ",
		Target:  "Asia/Shanghai",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.Compose, "TZ: Asia/Shanghai") {
		t.Fatalf("environment overlay missing:\n%s", rendered.Compose)
	}
	if !strings.Contains(rendered.Compose, "DATABASE_PASSWORD: ${POSTGRES_PASSWORD:?required}") {
		t.Fatalf("existing environment was rebuilt incorrectly:\n%s", rendered.Compose)
	}
}

func TestRenderKeepsTargetOnlyPortValidWhenEditingContainerPort(t *testing.T) {
	bundle := SourceBundle{
		ComposePath: "compose.yaml",
		Compose:     "services:\n  web:\n    image: nginx\n    ports:\n      - '8080'\n",
	}
	compiled, err := Compile(bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := Render(bundle, compiled.Manifest, nil, []MappingOverlay{{
		ID:       "port:web:8080/tcp",
		Kind:     "port",
		Service:  "web",
		Target:   "8081",
		Protocol: "tcp",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.Compose, "- '8081'") && !strings.Contains(rendered.Compose, "- \"8081\"") {
		t.Fatalf("target-only port became invalid:\n%s", rendered.Compose)
	}
	if strings.Contains(rendered.Compose, ":8081") {
		t.Fatalf("target-only port gained an empty published prefix:\n%s", rendered.Compose)
	}
}

func TestRenderPreservesPortHostIPAndDevicePermissions(t *testing.T) {
	bundle := SourceBundle{
		ComposePath: "compose.yaml",
		Compose: `services:
  web:
    image: nginx
    ports:
      - "127.0.0.1:8080:80/tcp"
    devices:
      - "/dev/dri:/dev/dri:rwm"
`,
	}
	compiled, err := Compile(bundle, nil)
	if err != nil {
		t.Fatal(err)
	}

	untouched, err := Render(bundle, compiled.Manifest, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(untouched.Compose, "127.0.0.1:8080:80/tcp") || !strings.Contains(untouched.Compose, "/dev/dri:/dev/dri:rwm") {
		t.Fatalf("untouched mapping metadata was lost:\n%s", untouched.Compose)
	}

	changed, err := Render(bundle, compiled.Manifest, map[string]string{
		"port:web:80/tcp":     "50080",
		"device:web:/dev/dri": "/dev/dri-new",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(changed.Compose, "127.0.0.1:50080:80/tcp") {
		t.Fatalf("port host IP was lost:\n%s", changed.Compose)
	}
	if !strings.Contains(changed.Compose, "/dev/dri-new:/dev/dri:rwm") {
		t.Fatalf("device permissions were lost:\n%s", changed.Compose)
	}
}

func assertRenderedFile(t *testing.T, files []SourceFile, name, contains string, mode uint32) {
	t.Helper()
	for _, file := range files {
		if file.Path == name {
			if !strings.Contains(file.Content, contains) || file.Mode != mode {
				t.Fatalf("file %s = %#v", name, file)
			}
			return
		}
	}
	t.Fatalf("rendered file %s not found", name)
}
