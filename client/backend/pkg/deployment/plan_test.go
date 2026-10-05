package deployment

import "testing"

func TestBuildDeploymentPlanKeepsSourceRuntimeAndFieldChanges(t *testing.T) {
	intent := Intent{
		EnvironmentID: "local",
		ProjectName:   "demo",
		Kind:          IntentKindCompose,
		ComposeYAML:   "services:\n  app:\n    ports: [\"8080:80\"]\n",
	}
	localized := LocalizedCompose{
		SourceYAML:  intent.ComposeYAML,
		RuntimeYAML: "services:\n  app:\n    ports: [\"50000:80\"]\n",
		Changes: []PlanChange{{
			Type:       "port",
			Target:     "services.app.ports[0].published",
			Before:     8080,
			After:      50000,
			ReasonCode: "auto_allocate_port",
		}},
	}
	preflight := PreflightResult{Issues: []PreflightIssue{{
		Code:        "image_unreachable",
		Severity:    PreflightWarning,
		NextActions: []string{"retry_registry"},
	}}}

	plan := BuildDeploymentPlan(intent, localized, preflight)
	if plan.Intent.ComposeYAML != intent.ComposeYAML || plan.RuntimeCompose != localized.RuntimeYAML {
		t.Fatalf("plan lost source/runtime snapshots: %#v", plan)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Target != localized.Changes[0].Target {
		t.Fatalf("plan lost field changes: %#v", plan.Changes)
	}
	if !plan.RequiresConfirmation {
		t.Fatal("warning preflight must require confirmation")
	}

	localized.Changes[0].Target = "mutated"
	preflight.Issues[0].Code = "mutated"
	if plan.Changes[0].Target == "mutated" || plan.Preflight.Issues[0].Code == "mutated" {
		t.Fatal("deployment plan must own immutable slice snapshots")
	}
}

func TestBuildDeploymentPlanDoesNotRequireConfirmationForSafeAutomaticChanges(t *testing.T) {
	intent := Intent{EnvironmentID: "local", ProjectName: "demo", Kind: IntentKindCompose, ComposeYAML: "services: {}"}
	plan := BuildDeploymentPlan(intent, LocalizedCompose{
		SourceYAML:  intent.ComposeYAML,
		RuntimeYAML: intent.ComposeYAML,
		Changes:     []PlanChange{{Type: "port", ReasonCode: "auto_allocate_port"}},
	}, PreflightResult{Issues: []PreflightIssue{{Code: "docker_ready", Severity: PreflightInfo}}})

	if plan.RequiresConfirmation {
		t.Fatal("safe deterministic changes must not interrupt ordinary deployment")
	}
}
