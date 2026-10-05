package deployment

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactManifestRemovesSensitiveValues(t *testing.T) {
	manifest := Manifest{
		JobID:         "job-1",
		EnvironmentID: "local",
		Source: Source{
			Type: SourceTypeGitHub,
			Ref:  "https://user:source-password@github.com/example/project?access_token=ref-token&branch=main&key=generic-key&X-Amz-Signature=signed-secret#access_token=fragment-token",
			Metadata: map[string]any{
				"github_token": "source-token",
				"branch":       "main",
			},
		},
		SourceCompose: `services:
  app:
    image: example/app:latest
    environment:
      API_TOKEN: source-compose-token
      TZ: Asia/Shanghai
    secrets:
      - app_password
secrets:
  app_password:
    file: ./app_password.txt
`,
		RuntimeCompose: `services:
  app:
    image: example/app:latest
    ports:
      - 50000:8080
    environment:
      - DB_PASSWORD=runtime-password
      - PUID=1000
`,
		ImageDigests: map[string]string{"example/app:latest": "sha256:abc"},
		Changes: []PlanChange{
			{
				Type:   "environment",
				Target: "services.app.environment",
				Before: map[string]any{"client_secret": "before-secret", "TZ": "UTC"},
				After:  map[string]any{"client_secret": "after-secret", "TZ": "Asia/Shanghai"},
			},
		},
		Metadata: map[string]any{
			"authorization": "Bearer manifest-token",
			"safe":          map[string]any{"projectRoot": "/data/projects"},
		},
	}

	redacted, err := RedactManifest(manifest)
	if err != nil {
		t.Fatalf("RedactManifest: %v", err)
	}
	raw, err := json.Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, secret := range []string{
		"source-token",
		"source-password",
		"ref-token",
		"generic-key",
		"signed-secret",
		"fragment-token",
		"source-compose-token",
		"runtime-password",
		"before-secret",
		"after-secret",
		"manifest-token",
	} {
		if strings.Contains(text, secret) {
			t.Errorf("redacted manifest still contains %q", secret)
		}
	}
	for _, safe := range []string{
		"example/app:latest",
		"sha256:abc",
		"Asia/Shanghai",
		"PUID=1000",
		"50000:8080",
		"./app_password.txt",
		"/data/projects",
		"branch=main",
	} {
		if !strings.Contains(text, safe) {
			t.Errorf("redacted manifest lost safe value %q: %s", safe, text)
		}
	}
	if strings.Count(text, RedactedValue) < 6 {
		t.Fatalf("expected sensitive fields to use the redaction marker: %s", text)
	}

	if manifest.Source.Metadata["github_token"] != "source-token" ||
		!strings.Contains(manifest.RuntimeCompose, "runtime-password") {
		t.Fatal("RedactManifest must not mutate its input")
	}

	direct, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	for _, secret := range []string{"source-token", "source-compose-token", "runtime-password", "manifest-token"} {
		if strings.Contains(string(direct), secret) {
			t.Errorf("direct manifest serialization contains %q: %s", secret, direct)
		}
	}
}

func TestRedactManifestRejectsInvalidCompose(t *testing.T) {
	_, err := RedactManifest(Manifest{
		JobID:          "job-1",
		EnvironmentID:  "local",
		SourceCompose:  "services: [",
		RuntimeCompose: "services: {}",
	})
	if err == nil {
		t.Fatal("invalid Compose must not be stored without redaction")
	}
}

func TestRedactSensitiveValuesHandlesNestedCollections(t *testing.T) {
	input := map[string]any{
		"items": []any{
			map[string]any{"api-key": "secret", "name": "first"},
			map[string]any{"database_url": "postgres://user:pass@db/app", "port": 5432},
		},
	}
	redacted := RedactSensitiveValues(input).(map[string]any)
	items := redacted["items"].([]any)
	if items[0].(map[string]any)["api-key"] != RedactedValue {
		t.Fatalf("API key was not redacted: %#v", redacted)
	}
	if items[1].(map[string]any)["database_url"] != RedactedValue {
		t.Fatalf("connection URL was not redacted: %#v", redacted)
	}
	if items[0].(map[string]any)["name"] != "first" || items[1].(map[string]any)["port"] != 5432 {
		t.Fatalf("safe values changed: %#v", redacted)
	}
}
