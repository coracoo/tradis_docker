package deployment

import "testing"

func TestBuildIdempotencyKeyNormalizesJSONAndSource(t *testing.T) {
	sourceA := Source{
		Type:     SourceTypeGitHub,
		Ref:      " https://github.com/example/project ",
		RepoID:   42,
		PathHint: " docker/ ",
		Metadata: map[string]any{
			"branch": "main",
			"labels": map[string]any{"tier": "official", "arch": "amd64"},
		},
	}
	sourceB := Source{
		Type:     SourceType(" GITHUB "),
		Ref:      "https://github.com/example/project",
		RepoID:   42,
		PathHint: "docker/",
		Metadata: map[string]any{
			"labels": map[string]any{"arch": "amd64", "tier": "official"},
			"branch": "main",
		},
	}

	first, err := BuildIdempotencyKey(
		" local ",
		sourceA,
		" project ",
		`{ "ports": [50000, 50001], "environment": { "TZ": "Asia/Shanghai", "PUID": 1000 } }`,
	)
	if err != nil {
		t.Fatalf("BuildIdempotencyKey first: %v", err)
	}
	second, err := BuildIdempotencyKey(
		"local",
		sourceB,
		"project",
		`{"environment":{"PUID":1000,"TZ":"Asia/Shanghai"},"ports":[50000,50001]}`,
	)
	if err != nil {
		t.Fatalf("BuildIdempotencyKey second: %v", err)
	}

	if first != second {
		t.Fatalf("equivalent input produced different keys:\n%s\n%s", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("expected SHA-256 hex key, got %q", first)
	}
}

func TestBuildIdempotencyKeyChangesForBusinessInput(t *testing.T) {
	source := Source{Type: SourceTypeCompose, Ref: "manual"}
	first, err := BuildIdempotencyKey("local", source, "project", map[string]any{"port": 50000})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildIdempotencyKey("local", source, "project", map[string]any{"port": 50001})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("different deployment input must not share an idempotency key")
	}
}

func TestBuildIdempotencyKeyPreservesSourceIdentityBehindPresentationRedaction(t *testing.T) {
	first, err := BuildIdempotencyKey("local", Source{
		Type:     SourceTypeGitHub,
		Ref:      "https://example.test/project?token=first-token",
		PathHint: "docker",
		Metadata: map[string]any{"github_token": "first-metadata-token", "branch": "main"},
	}, "project", map[string]any{"pull": true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildIdempotencyKey("local", Source{
		Type:     SourceTypeGitHub,
		Ref:      "https://example.test/project?token=second-token",
		PathHint: "docker",
		Metadata: map[string]any{"github_token": "second-metadata-token", "branch": "main"},
	}, "project", map[string]any{"pull": true})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("distinct source credentials must not collide after presentation redaction")
	}
}

func TestBuildIdempotencyKeyRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		source      Source
		project     string
		input       any
	}{
		{name: "environment", source: Source{Type: SourceTypeCompose}, project: "project", input: map[string]any{}},
		{name: "source", environment: "local", project: "project", input: map[string]any{}},
		{name: "project", environment: "local", source: Source{Type: SourceTypeCompose}, input: map[string]any{}},
		{name: "json", environment: "local", source: Source{Type: SourceTypeCompose}, project: "project", input: `{"broken":`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := BuildIdempotencyKey(test.environment, test.source, test.project, test.input); err == nil {
				t.Fatal("expected invalid idempotency input to fail")
			}
		})
	}
}
