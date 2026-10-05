package templatecompiler

import (
	"encoding/json"
	"os"
	"testing"
)

func TestManifestV1JSONContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/multi-service/expected-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ManifestVersion != 1 {
		t.Fatalf("manifest version = %d", manifest.ManifestVersion)
	}
	if len(manifest.Inputs) == 0 || manifest.Inputs[0].ID != "project:POSTGRES_PASSWORD" {
		t.Fatalf("unexpected first input: %#v", manifest.Inputs)
	}
	if manifest.SourceDigest == "" || manifest.ManifestDigest == "" {
		t.Fatal("manifest digests must not be empty")
	}
}

func TestCompilerVersionTracksPresentationNormalization(t *testing.T) {
	if CompilerVersion != "1.0.1" {
		t.Fatalf("compiler version=%q, want 1.0.1", CompilerVersion)
	}
}
