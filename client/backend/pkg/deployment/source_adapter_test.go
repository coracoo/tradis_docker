package deployment

import (
	"context"
	"errors"
	"testing"
)

func TestSourceAdapterDispatchesAndNormalizesInspection(t *testing.T) {
	adapter := NewSourceAdapter(map[SourceType]SourceInspectorFunc{
		SourceTypeGitHub: func(_ context.Context, source Source) (Intent, Evidence, error) {
			return Intent{
				EnvironmentID: "local",
				ProjectName:   "demo",
				Kind:          IntentKindCompose,
				ComposeYAML:   "services:\n  app:\n    image: nginx:latest\n",
			}, Evidence{Ready: true, Strategy: EvidenceStrategyCompose}, nil
		},
	})

	source := Source{Type: SourceTypeGitHub, Ref: " https://github.com/example/demo ", PathHint: " /docker/ "}
	intent, evidence, err := adapter.InspectSource(context.Background(), source)
	if err != nil {
		t.Fatalf("InspectSource: %v", err)
	}
	if intent.Source.Type != SourceTypeGitHub || intent.Source.Ref != "https://github.com/example/demo" || intent.Source.PathHint != "docker" {
		t.Fatalf("source was not normalized and attached to intent: %#v", intent.Source)
	}
	if !evidence.Ready || evidence.Strategy != EvidenceStrategyCompose {
		t.Fatalf("unexpected evidence: %#v", evidence)
	}
}

func TestSourceAdapterRejectsUnsupportedAndInvalidReadyInspection(t *testing.T) {
	adapter := NewSourceAdapter(map[SourceType]SourceInspectorFunc{
		SourceTypePrompt: func(context.Context, Source) (Intent, Evidence, error) {
			return Intent{}, Evidence{Ready: true, Strategy: EvidenceStrategyImage}, nil
		},
		SourceTypeTutorial: func(context.Context, Source) (Intent, Evidence, error) {
			return Intent{}, Evidence{}, errors.New("feed unavailable")
		},
	})

	_, _, err := adapter.InspectSource(context.Background(), Source{Type: SourceTypeCompose})
	var structured *StructuredError
	if !errors.As(err, &structured) || structured.Code != ErrorCodeInvalidInput {
		t.Fatalf("unsupported source must return structured invalid input: %#v", err)
	}

	_, _, err = adapter.InspectSource(context.Background(), Source{Type: SourceTypePrompt})
	if !errors.As(err, &structured) || structured.Code != ErrorCodeInvalidInput {
		t.Fatalf("ready inspection without intent kind must fail: %#v", err)
	}

	_, _, err = adapter.InspectSource(context.Background(), Source{Type: SourceTypeTutorial, Ref: "demo"})
	if !errors.As(err, &structured) || structured.Code != ErrorCodeSourceUnavailable || !structured.Retryable {
		t.Fatalf("reader failure must return retryable source error: %#v", err)
	}
}

func TestSourceAdapterAllowsInsufficientEvidenceWithoutInventingIntent(t *testing.T) {
	adapter := NewSourceAdapter(map[SourceType]SourceInspectorFunc{
		SourceTypePrompt: func(context.Context, Source) (Intent, Evidence, error) {
			return Intent{}, Evidence{
				Ready:          false,
				Strategy:       EvidenceStrategyUnknown,
				ReasonCode:     "application_not_identified",
				RequiredInputs: []string{"application"},
			}, nil
		},
	})

	intent, evidence, err := adapter.InspectSource(context.Background(), Source{Type: SourceTypePrompt})
	if err != nil {
		t.Fatalf("InspectSource: %v", err)
	}
	if intent.Kind != "" || evidence.Ready || evidence.ReasonCode != "application_not_identified" {
		t.Fatalf("insufficient evidence must not invent an intent: %#v %#v", intent, evidence)
	}
}
