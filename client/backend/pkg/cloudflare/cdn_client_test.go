package cloudflare

import (
	"encoding/json"
	"testing"
)

func TestTemplateDetailDecodesLegacyAndManifestContracts(t *testing.T) {
	legacy := `{"id":1,"name":"legacy","compose":"services: {}","dotenv":"A=1","schema":[{"name":"A"}]}`
	var legacyDetail TemplateDetail
	if err := json.Unmarshal([]byte(legacy), &legacyDetail); err != nil {
		t.Fatalf("decode legacy template: %v", err)
	}
	if legacyDetail.Name != "legacy" || legacyDetail.Manifest != nil {
		t.Fatalf("unexpected legacy detail: %+v", legacyDetail)
	}

	modern := `{
  "id": 2,
  "name": "modern",
  "compose": "services: {}",
  "source_files": [{"path":"config/app.env","content":"MODE=prod\n"}],
  "manifest": {"manifest_version":1,"compiler_version":"1.0.0","source_digest":"sha256:source","manifest_digest":"sha256:manifest","inputs":[],"bindings":[],"mappings":[],"fixed_values":[],"opaque_nodes":[],"diagnostics":[],"complete":true},
  "source_digest": "sha256:source",
  "manifest_digest": "sha256:manifest",
  "compiler_version": "1.0.0",
  "future_field": true
}`
	var modernDetail TemplateDetail
	if err := json.Unmarshal([]byte(modern), &modernDetail); err != nil {
		t.Fatalf("decode manifest template: %v", err)
	}
	if modernDetail.Manifest == nil || !modernDetail.Manifest.Complete || len(modernDetail.SourceFiles) != 1 {
		t.Fatalf("unexpected manifest detail: %+v", modernDetail)
	}
}
