package composehistory

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildPreviewIgnoresFormattingOnlyChanges(t *testing.T) {
	preview, err := BuildPreview(
		"services:\n  web:\n    image: nginx:latest\n",
		"services: {web: {image: nginx:latest}}\n",
		"TOKEN=old\n",
		"TOKEN=old\n",
	)
	if err != nil {
		t.Fatal(err)
	}
	if preview.HasChanges || preview.YAMLChanged || preview.EnvChanged {
		t.Fatalf("format-only edit reported as change: %#v", preview)
	}
	if preview.BaseHash == "" || preview.BaseHash != preview.CandidateHash {
		t.Fatalf("equivalent YAML hashes differ: %#v", preview)
	}
}

func TestBuildPreviewReportsSafeComposeFields(t *testing.T) {
	current := `services:
  web:
    image: nginx:1
    ports:
      - "8080:80"
    volumes:
      - data:/var/lib/app
    privileged: false
volumes:
  data: {}
`
	candidate := `services:
  web:
    image: nginx:2
    build:
      context: .
    ports:
      - "8081:80"
    volumes:
      - ./data:/var/lib/app
    privileged: true
  worker:
    image: busybox:latest
volumes: {}
`

	preview, err := BuildPreview(current, candidate, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !preview.HasChanges || !preview.YAMLChanged || preview.EnvChanged {
		t.Fatalf("unexpected preview flags: %#v", preview)
	}
	assertChange(t, preview, "services.web", "image", ChangeModified, "nginx:1", "nginx:2")
	assertChange(t, preview, "services.web", "build", ChangeAdded, "", "context: .")
	assertChange(t, preview, "services.web", "ports", ChangeModified, "8080:80", "8081:80")
	assertChange(t, preview, "services.web", "volumes", ChangeModified, "data:/var/lib/app", "./data:/var/lib/app")
	assertChange(t, preview, "services.web", "privileged", ChangeModified, "false", "true")
	assertChangeWithoutValues(t, preview, "services.worker", "service", ChangeAdded)
	assertChangeWithoutValues(t, preview, "volumes", "data", ChangeRemoved)
}

func TestBuildPreviewRedactsEnvironmentAndGenericValues(t *testing.T) {
	preview, err := BuildPreview(
		"services:\n  web:\n    image: nginx:1\n    command: secret-old\n    environment:\n      API_TOKEN: old-yaml-secret\n",
		"services:\n  web:\n    image: nginx:2\n    command: secret-new\n    environment:\n      API_TOKEN: new-yaml-secret\n      NEW_KEY: value\n",
		"API_TOKEN=old-env-secret\n",
		"API_TOKEN=new-env-secret\nNEW_KEY=value\n",
	)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"secret-old", "secret-new", "old-yaml-secret", "new-yaml-secret",
		"old-env-secret", "new-env-secret", "NEW_KEY=value",
	} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatalf("preview leaked %q: %s", forbidden, raw)
		}
	}

	assertChange(t, preview, "services.web", "image", ChangeModified, "nginx:1", "nginx:2")
	assertChangeWithoutValues(t, preview, "services.web", "command", ChangeModified)
	assertChangeWithoutValues(t, preview, "services.web", "environment.API_TOKEN", ChangeModified)
	assertChangeWithoutValues(t, preview, "services.web", "environment.NEW_KEY", ChangeAdded)
	assertChangeWithoutValues(t, preview, "environment", "API_TOKEN", ChangeModified)
	assertChangeWithoutValues(t, preview, "environment", "NEW_KEY", ChangeAdded)
}

func TestBuildPreviewCoversSecurityAndRuntimeFields(t *testing.T) {
	current := `services:
  app:
    user: "1000:1000"
    cap_add: [NET_ADMIN]
    cap_drop: []
    read_only: false
    restart: unless-stopped
    network_mode: bridge
    devices: ["/dev/dri:/dev/dri"]
    healthcheck:
      test: [CMD, old-check]
`
	candidate := `services:
  app:
    user: "1001:1001"
    cap_add: [SYS_ADMIN]
    cap_drop: [ALL]
    read_only: true
    restart: always
    network_mode: host
    devices: ["/dev/fuse:/dev/fuse"]
    healthcheck:
      test: [CMD, new-check]
`
	preview, err := BuildPreview(current, candidate, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"user", "cap_add", "cap_drop", "read_only", "restart", "network_mode", "devices", "healthcheck"} {
		if !hasChange(preview, "services.app", field, ChangeModified) {
			t.Errorf("missing safe runtime change for %s: %#v", field, preview.Groups)
		}
	}
}

func TestBuildPreviewRejectsInvalidComposeDocuments(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{name: "invalid yaml", yaml: "services: ["},
		{name: "missing services", yaml: "networks:\n  default: {}\n"},
		{name: "empty services", yaml: "services: {}\n"},
		{name: "services list", yaml: "services: []\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildPreview("services:\n  web:\n    image: nginx\n", tc.yaml, "", "")
			if err == nil {
				t.Fatalf("BuildPreview accepted %q", tc.yaml)
			}
			if strings.Contains(err.Error(), "nginx") {
				t.Fatalf("validation error leaked document content: %v", err)
			}
		})
	}
}

func TestYAMLHashPreservesSequenceOrder(t *testing.T) {
	first, err := YAMLHash("services:\n  web:\n    ports: [8080:80, 8081:81]\n")
	if err != nil {
		t.Fatal(err)
	}
	second, err := YAMLHash("services:\n  web:\n    ports: [8081:81, 8080:80]\n")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("sequence order was discarded from semantic hash")
	}
}

func assertChange(t *testing.T, preview Preview, group, field string, kind ChangeKind, before, after string) {
	t.Helper()
	for _, item := range preview.Groups {
		if item.Key != group {
			continue
		}
		for _, change := range item.Changes {
			if change.Field == field && change.Kind == kind && change.Before == before && change.After == after {
				return
			}
		}
	}
	t.Fatalf("missing change group=%q field=%q kind=%q before=%q after=%q in %#v", group, field, kind, before, after, preview.Groups)
}

func assertChangeWithoutValues(t *testing.T, preview Preview, group, field string, kind ChangeKind) {
	t.Helper()
	for _, item := range preview.Groups {
		if item.Key != group {
			continue
		}
		for _, change := range item.Changes {
			if change.Field == field && change.Kind == kind {
				if change.Before != "" || change.After != "" {
					t.Fatalf("change leaked values: %#v", change)
				}
				return
			}
		}
	}
	t.Fatalf("missing redacted change group=%q field=%q kind=%q in %#v", group, field, kind, preview.Groups)
}

func hasChange(preview Preview, group, field string, kind ChangeKind) bool {
	for _, item := range preview.Groups {
		if item.Key != group {
			continue
		}
		for _, change := range item.Changes {
			if change.Field == field && change.Kind == kind {
				return true
			}
		}
	}
	return false
}
