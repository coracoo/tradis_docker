package templatecompiler

import (
	"strings"
	"testing"
)

func multiServiceBundle() SourceBundle {
	return SourceBundle{
		ComposePath: "compose.yaml",
		Compose: `services:
  api:
    image: postgres:${POSTGRES_TAG:-16}
    env_file:
      - path: api.env
        required: true
    environment:
      DATABASE_PASSWORD: ${POSTGRES_PASSWORD:?required}
      ALIAS_PASSWORD: ${POSTGRES_PASSWORD}
    ports:
      - "15432:5432"
    volumes:
      - ./api-data:/var/lib/postgresql/data
      - shared-data:/shared
    devices:
      - /dev/dri:/dev/dri
    secrets:
      - db_password
      - registry_key
    x-extra:
      keep: true
  worker:
    image: worker:${POSTGRES_TAG:-16}
    env_file: worker.env
    environment:
      - DATABASE_PASSWORD=${POSTGRES_PASSWORD}
volumes:
  shared-data: {}
secrets:
  db_password:
    file: ./secrets/db_password
  registry_key:
    external: true
`,
		Dotenv: "POSTGRES_PASSWORD=\nPOSTGRES_TAG=16\n",
		Files: []SourceFile{
			{Path: "api.env", Content: "SHARED_KEY=api\n"},
			{Path: "worker.env", Content: "SHARED_KEY=worker\n"},
		},
	}
}

func TestCompileKeepsScopedInputsAndComposeTypes(t *testing.T) {
	result, err := Compile(multiServiceBundle(), nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest := result.Manifest
	if !manifest.Complete {
		t.Fatalf("manifest incomplete: %#v", manifest.Diagnostics)
	}
	if strings.Join(manifest.Services, ",") != "api,worker" {
		t.Fatalf("services=%v", manifest.Services)
	}
	assertInput(t, manifest, "project:POSTGRES_PASSWORD", "env")
	assertInput(t, manifest, "env_file:api.env:SHARED_KEY", "env")
	assertInput(t, manifest, "env_file:worker.env:SHARED_KEY", "env")
	assertInput(t, manifest, "secret:db_password", "secret")
	assertNoInput(t, manifest, "secret:registry_key")
	assertMapping(t, manifest, "port:api:5432/tcp", "port", true)
	assertMapping(t, manifest, "bind:api:/var/lib/postgresql/data", "bind", true)
	assertMapping(t, manifest, "volume:api:/shared", "volume", false)
	assertMapping(t, manifest, "device:api:/dev/dri", "device", true)

	bindings := 0
	for _, binding := range manifest.Bindings {
		if binding.InputID == "project:POSTGRES_PASSWORD" {
			bindings++
		}
	}
	if bindings != 3 {
		t.Fatalf("POSTGRES_PASSWORD bindings = %d, want 3", bindings)
	}
}

func TestCompileRejectsSecretContentInSource(t *testing.T) {
	bundle := multiServiceBundle()
	bundle.Files = append(bundle.Files, SourceFile{Path: "secrets/db_password", Content: "leaked"})
	result, err := Compile(bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Complete {
		t.Fatal("manifest with managed secret content must be incomplete")
	}
	found := false
	for _, diagnostic := range result.Manifest.Diagnostics {
		if diagnostic.Code == "secret_content_in_source" && diagnostic.Severity == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing secret_content_in_source diagnostic: %#v", result.Manifest.Diagnostics)
	}
}

func TestCompileNormalizesPresentationGroups(t *testing.T) {
	bundle := SourceBundle{
		ComposePath: "compose.yaml",
		Compose: `services:
  app:
    image: example/app:${TAG:-latest}
    environment:
      REQUIRED_TOKEN: ${REQUIRED_TOKEN:?required}
      OPTIONAL_MODE: ${OPTIONAL_MODE:-normal}
`,
	}
	metadata := PresentationMetadata{
		"project:TAG":            {Group: " Advanced "},
		"project:REQUIRED_TOKEN": {Group: "advanced"},
		"project:OPTIONAL_MODE":  {Group: "expert"},
	}

	result, err := Compile(bundle, metadata)
	if err != nil {
		t.Fatal(err)
	}
	inputs := make(map[string]Input, len(result.Manifest.Inputs))
	for _, input := range result.Manifest.Inputs {
		inputs[input.ID] = input
	}
	if got := inputs["project:TAG"].Presentation.Group; got != "advanced" {
		t.Fatalf("TAG group=%q, want advanced", got)
	}
	if got := inputs["project:OPTIONAL_MODE"].Presentation.Group; got != "basic" {
		t.Fatalf("OPTIONAL_MODE group=%q, want basic", got)
	}
	if got := inputs["project:REQUIRED_TOKEN"].Presentation.Group; got != "basic" {
		t.Fatalf("REQUIRED_TOKEN group=%q, want basic", got)
	}

	foundPromotion := false
	for _, diagnostic := range result.Manifest.Diagnostics {
		if diagnostic.Code == "required_input_promoted_to_basic" && diagnostic.Severity == "warning" {
			foundPromotion = true
		}
	}
	if !foundPromotion {
		t.Fatalf("missing required promotion diagnostic: %#v", result.Manifest.Diagnostics)
	}
}

func TestCompileDefaultsEveryEditableInputToBasic(t *testing.T) {
	result, err := Compile(multiServiceBundle(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range result.Manifest.Inputs {
		if input.Presentation.Group != "basic" {
			t.Fatalf("input %s group=%q, want basic", input.ID, input.Presentation.Group)
		}
	}
}

func TestCompileRejectsUnsafeComposePath(t *testing.T) {
	bundle := multiServiceBundle()
	bundle.ComposePath = "../compose.yaml"

	_, err := Compile(bundle, nil)
	if err == nil || !strings.Contains(err.Error(), "unsafe Compose path") {
		t.Fatalf("expected unsafe Compose path error, got %v", err)
	}
}

func TestCompileAssignsDistinctIDsToDuplicatePortTargets(t *testing.T) {
	bundle := SourceBundle{
		ComposePath: "compose.yaml",
		Compose: `services:
  web:
    image: nginx:alpine
    ports:
      - "127.0.0.1:8080:80"
      - "0.0.0.0:8081:80"
`,
	}

	result, err := Compile(bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Manifest.Mappings) != 2 {
		t.Fatalf("mapping count=%d", len(result.Manifest.Mappings))
	}
	if result.Manifest.Mappings[0].ID == result.Manifest.Mappings[1].ID {
		t.Fatalf("duplicate mapping id=%q", result.Manifest.Mappings[0].ID)
	}
}

func TestCompileRejectsDotenvDuplicatedAsSourceFile(t *testing.T) {
	bundle := multiServiceBundle()
	bundle.Files = append(bundle.Files, SourceFile{Path: ".env", Content: "POSTGRES_TAG=17\n"})

	_, err := Compile(bundle, nil)
	if err == nil || !strings.Contains(err.Error(), "reserved source file path") {
		t.Fatalf("expected reserved source file path error, got %v", err)
	}
}

func TestVerifyDetectsStaleSource(t *testing.T) {
	bundle := multiServiceBundle()
	result, err := Compile(bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Dotenv = strings.ReplaceAll(bundle.Dotenv, "POSTGRES_TAG=16", "POSTGRES_TAG=17")
	if err := Verify(bundle, result.Manifest); err == nil {
		t.Fatal("Verify accepted a stale manifest")
	}
}

func TestVerifyRejectsUnsupportedCompilerVersion(t *testing.T) {
	bundle := multiServiceBundle()
	result, err := Compile(bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	result.Manifest.CompilerVersion = "0.9.0"
	if err := setManifestDigest(&result.Manifest); err != nil {
		t.Fatal(err)
	}
	if err := Verify(bundle, result.Manifest); err == nil {
		t.Fatal("Verify accepted a manifest from an unsupported compiler")
	}
}

func TestVerifyRejectsSelfConsistentManifestThatDoesNotMatchSource(t *testing.T) {
	bundle := multiServiceBundle()
	result, err := Compile(bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	result.Manifest.Mappings = result.Manifest.Mappings[1:]
	if err := setManifestDigest(&result.Manifest); err != nil {
		t.Fatal(err)
	}
	if err := Verify(bundle, result.Manifest); err == nil {
		t.Fatal("Verify accepted a self-consistent manifest not produced from the source")
	}
}

func assertInput(t *testing.T, manifest Manifest, id, kind string) {
	t.Helper()
	for _, input := range manifest.Inputs {
		if input.ID == id {
			if input.Kind != kind {
				t.Fatalf("input %s kind = %s", id, input.Kind)
			}
			return
		}
	}
	t.Fatalf("input %s not found", id)
}

func assertNoInput(t *testing.T, manifest Manifest, id string) {
	t.Helper()
	for _, input := range manifest.Inputs {
		if input.ID == id {
			t.Fatalf("unexpected input %s", id)
		}
	}
}

func assertMapping(t *testing.T, manifest Manifest, id, kind string, editable bool) {
	t.Helper()
	for _, mapping := range manifest.Mappings {
		if mapping.ID == id {
			if mapping.Kind != kind || mapping.Editable != editable {
				t.Fatalf("mapping %s = %#v", id, mapping)
			}
			return
		}
	}
	t.Fatalf("mapping %s not found in %#v", id, manifest.Mappings)
}
