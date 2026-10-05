package deployment

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPreflightResultClassifiesIssues(t *testing.T) {
	result := PreflightResult{
		Issues: []PreflightIssue{
			{Code: "docker_unavailable", Severity: PreflightBlocking},
			{Code: "bind_path_review", Severity: PreflightWarning, NextActions: []string{"ask_user"}},
			{Code: "image_cached", Severity: PreflightInfo},
		},
	}
	if !result.HasBlocking() {
		t.Fatal("blocking preflight issue must stop automatic execution")
	}
	if got := result.IssuesBySeverity(PreflightWarning); len(got) != 1 || got[0].Code != "bind_path_review" {
		t.Fatalf("unexpected warning issues: %#v", got)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"next_actions":["ask_user"]`) || strings.Contains(string(raw), `"nextActions"`) {
		t.Fatalf("preflight actions must use the structured error contract: %s", raw)
	}

	result.Issues = result.Issues[1:]
	if result.HasBlocking() {
		t.Fatal("warnings and info must not be classified as blocking")
	}
}

func TestStructuredErrorContract(t *testing.T) {
	cause := errors.New("dial timeout")
	err := NewStructuredError(
		ErrorCodeSourceUnavailable,
		cause,
		true,
		[]string{"retry_source", "ask_user"},
		map[string]any{"source": "github"},
	)

	if !errors.Is(err, cause) {
		t.Fatal("structured error must preserve its cause")
	}
	if !strings.Contains(err.Error(), string(ErrorCodeSourceUnavailable)) || !strings.Contains(err.Error(), "无法读取部署来源") {
		t.Fatalf("unexpected error text: %q", err.Error())
	}
	raw, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	text := string(raw)
	for _, field := range []string{`"error_code":"source_unavailable"`, `"retryable":true`, `"next_actions"`, `"details"`} {
		if !strings.Contains(text, field) {
			t.Errorf("structured error JSON missing %s: %s", field, text)
		}
	}
	if strings.Contains(text, "dial timeout") {
		t.Fatalf("internal cause must not be serialized: %s", text)
	}

	unsafeLiteral := &StructuredError{
		Code:    ErrorCodeExecutionFailed,
		Details: map[string]any{"api_token": "literal-secret"},
	}
	raw, marshalErr = json.Marshal(unsafeLiteral)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(raw), "literal-secret") || !strings.Contains(string(raw), RedactedValue) {
		t.Fatalf("structured error serialization must redact literal details: %s", raw)
	}
}

func TestSourceJSONRedactsStandaloneAndNestedValues(t *testing.T) {
	source := Source{
		Type: SourceTypeGitHub,
		Ref:  "https://example.test/project?token=source-ref-secret&branch=main",
		Metadata: map[string]any{
			"github_token": "source-metadata-secret",
			"branch":       "main",
		},
	}

	for name, value := range map[string]any{
		"standalone": source,
		"nested": PreflightIssue{
			Code:     "source_check",
			Severity: PreflightInfo,
			Details:  map[string]any{"source": source},
		},
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			text := string(raw)
			if strings.Contains(text, "source-ref-secret") || strings.Contains(text, "source-metadata-secret") {
				t.Fatalf("serialized source leaked credentials: %s", text)
			}
			if !strings.Contains(text, "branch") || !strings.Contains(text, RedactedValue) {
				t.Fatalf("serialized source lost safe context or marker: %s", text)
			}
		})
	}
}

func TestPlanChangeJSONRedactsStandaloneValues(t *testing.T) {
	change := PlanChange{
		Type:       "environment",
		Target:     "services.app.environment",
		Before:     map[string]any{"password": "before-secret", "path": "/data/old"},
		After:      map[string]any{"api_token": "after-secret", "path": "/data/new"},
		ReasonCode: "localize_environment",
	}
	raw, err := json.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "before-secret") || strings.Contains(text, "after-secret") {
		t.Fatalf("standalone plan change leaked credentials: %s", text)
	}
	for _, safe := range []string{"/data/old", "/data/new", "localize_environment", RedactedValue} {
		if !strings.Contains(text, safe) {
			t.Errorf("standalone plan change lost %q: %s", safe, text)
		}
	}
	if change.Before.(map[string]any)["password"] != "before-secret" {
		t.Fatal("plan change serialization must not mutate execution data")
	}
}

func TestIntentAndPlanJSONContract(t *testing.T) {
	plan := Plan{
		Intent: Intent{
			EnvironmentID: "local",
			ProjectName:   "demo",
			Kind:          IntentKindCompose,
			Source:        Source{Type: SourceTypeTutorial, Ref: "tutorial-slug"},
			ComposeYAML:   "services: {}",
		},
		RuntimeCompose: "services: {}",
		Changes:        []PlanChange{{Type: "port", Target: "services.app.ports", ReasonCode: "auto_port"}},
		Preflight:      PreflightResult{},
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, field := range []string{`"environmentId":"local"`, `"projectName":"demo"`, `"kind":"compose"`, `"runtimeCompose"`, `"reasonCode":"auto_port"`} {
		if !strings.Contains(text, field) {
			t.Errorf("deployment plan JSON missing %s: %s", field, text)
		}
	}
}

func TestPlanJSONUsesRedactedPresentationSnapshot(t *testing.T) {
	plan := Plan{
		Intent: Intent{
			EnvironmentID: "local",
			ProjectName:   "demo",
			Kind:          IntentKindCompose,
			Source: Source{
				Type:     SourceTypeGitHub,
				Ref:      "https://example.test/project?token=source-ref-secret",
				Metadata: map[string]any{"github_token": "source-metadata-secret", "branch": "main"},
			},
			ComposeYAML: "services:\n  app:\n    environment:\n      API_TOKEN: intent-compose-secret\n",
			Parameters:  map[string]any{"password": "parameter-secret", "timezone": "Asia/Shanghai"},
		},
		RuntimeCompose: "services:\n  app:\n    environment:\n      DB_PASSWORD: runtime-compose-secret\n",
		Changes: []PlanChange{{
			Type:   "environment",
			Target: "services.app.environment",
			After:  map[string]any{"client_secret": "change-secret"},
		}},
		Preflight: PreflightResult{Issues: []PreflightIssue{{
			Code:     "missing_env",
			Severity: PreflightBlocking,
			Details:  map[string]any{"api_key": "preflight-secret", "name": "API_KEY"},
		}}},
		Metadata: map[string]any{"authorization": "Bearer plan-secret", "attempt": 1},
	}

	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, secret := range []string{
		"source-ref-secret",
		"source-metadata-secret",
		"intent-compose-secret",
		"parameter-secret",
		"runtime-compose-secret",
		"change-secret",
		"preflight-secret",
		"plan-secret",
	} {
		if strings.Contains(text, secret) {
			t.Errorf("serialized plan contains %q: %s", secret, text)
		}
	}
	for _, safe := range []string{"Asia/Shanghai", "branch", "API_KEY", `"attempt":1`, RedactedValue} {
		if !strings.Contains(text, safe) {
			t.Errorf("serialized plan lost safe value %q: %s", safe, text)
		}
	}
	if plan.Intent.Parameters["password"] != "parameter-secret" || !strings.Contains(plan.RuntimeCompose, "runtime-compose-secret") {
		t.Fatal("safe presentation serialization must not mutate execution data")
	}
}
