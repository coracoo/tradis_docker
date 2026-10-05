package deployment

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

type EvidenceStrategy string

const (
	EvidenceStrategyCompose  EvidenceStrategy = "compose"
	EvidenceStrategyBuild    EvidenceStrategy = "build"
	EvidenceStrategyImage    EvidenceStrategy = "image"
	EvidenceStrategyAppStore EvidenceStrategy = "appstore"
	EvidenceStrategyScript   EvidenceStrategy = "script"
	EvidenceStrategyUnknown  EvidenceStrategy = "unknown"
)

type EvidenceFile struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type Evidence struct {
	Ready          bool             `json:"ready"`
	Strategy       EvidenceStrategy `json:"strategy"`
	ReasonCode     string           `json:"reasonCode,omitempty"`
	Files          []EvidenceFile   `json:"files,omitempty"`
	Links          []string         `json:"links,omitempty"`
	RequiredInputs []string         `json:"requiredInputs,omitempty"`
	Metadata       map[string]any   `json:"metadata,omitempty"`
}

type SourceInspectorFunc func(context.Context, Source) (Intent, Evidence, error)

type SourceAdapter struct {
	inspectors map[SourceType]SourceInspectorFunc
}

func NewSourceAdapter(inspectors map[SourceType]SourceInspectorFunc) *SourceAdapter {
	copyInspectors := make(map[SourceType]SourceInspectorFunc, len(inspectors))
	for sourceType, inspect := range inspectors {
		sourceType = normalizeSourceType(sourceType)
		if sourceType != "" && inspect != nil {
			copyInspectors[sourceType] = inspect
		}
	}
	return &SourceAdapter{inspectors: copyInspectors}
}

func (adapter *SourceAdapter) InspectSource(ctx context.Context, source Source) (Intent, Evidence, error) {
	source, err := normalizeInspectionSource(source)
	if err != nil {
		return Intent{}, Evidence{}, NewStructuredError(
			ErrorCodeInvalidInput,
			err,
			false,
			[]string{"fix_source"},
			map[string]any{"sourceType": source.Type},
		)
	}
	if adapter == nil || adapter.inspectors[source.Type] == nil {
		return Intent{}, Evidence{}, NewStructuredError(
			ErrorCodeInvalidInput,
			fmt.Errorf("unsupported deployment source type %q", source.Type),
			false,
			[]string{"select_supported_source"},
			map[string]any{"sourceType": source.Type},
		)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	intent, evidence, inspectErr := adapter.inspectors[source.Type](ctx, source)
	if inspectErr != nil {
		var structured *StructuredError
		if errors.As(inspectErr, &structured) {
			return Intent{}, Evidence{}, inspectErr
		}
		return Intent{}, Evidence{}, NewStructuredError(
			ErrorCodeSourceUnavailable,
			inspectErr,
			true,
			[]string{"retry_source", "ask_user"},
			map[string]any{"sourceType": source.Type},
		)
	}

	evidence = normalizeSourceEvidence(evidence)
	if evidence.Ready {
		if err := validateInspectedIntent(intent, evidence); err != nil {
			return Intent{}, Evidence{}, NewStructuredError(
				ErrorCodeInvalidInput,
				err,
				false,
				[]string{"inspect_source", "ask_user"},
				map[string]any{"sourceType": source.Type, "strategy": evidence.Strategy},
			)
		}
	} else if intent.Kind != "" {
		return Intent{}, Evidence{}, NewStructuredError(
			ErrorCodeInvalidInput,
			errors.New("an unresolved source must not produce an executable intent"),
			false,
			[]string{"inspect_source"},
			map[string]any{"sourceType": source.Type},
		)
	}
	if intent.Source.Type == "" {
		intent.Source = source
	} else {
		intent.Source, err = normalizeInspectionSource(intent.Source)
		if err != nil {
			return Intent{}, Evidence{}, NewStructuredError(
				ErrorCodeInvalidInput,
				err,
				false,
				[]string{"inspect_source"},
				map[string]any{"sourceType": source.Type},
			)
		}
	}
	intent.Source.Metadata = cloneStringAnyMap(intent.Source.Metadata)
	intent.Parameters = cloneStringAnyMap(intent.Parameters)
	intent.Options = cloneStringAnyMap(intent.Options)
	return intent, evidence, nil
}

func normalizeInspectionSource(source Source) (Source, error) {
	source.Type = normalizeSourceType(source.Type)
	source.Ref = strings.TrimSpace(source.Ref)
	source.PathHint = filepath.ToSlash(strings.Trim(strings.TrimSpace(source.PathHint), "/\\"))
	source.Metadata = cloneStringAnyMap(source.Metadata)
	if source.Type == "" {
		return source, errors.New("deployment source type is required")
	}
	if strings.IndexFunc(source.Ref, unicode.IsControl) >= 0 || strings.IndexFunc(source.PathHint, unicode.IsControl) >= 0 {
		return source, errors.New("deployment source contains control characters")
	}
	for _, segment := range strings.Split(source.PathHint, "/") {
		if segment == ".." {
			return source, errors.New("deployment source path escapes its root")
		}
	}
	switch source.Type {
	case SourceTypeGitHub:
		if source.Ref == "" && source.RepoID <= 0 {
			return source, errors.New("GitHub source requires ref or repoId")
		}
	case SourceTypeTutorial, SourceTypeAppStore:
		if source.Ref == "" {
			return source, fmt.Errorf("%s source requires ref", source.Type)
		}
	case SourceTypePrompt, SourceTypeCompose:
	default:
		return source, fmt.Errorf("unsupported deployment source type %q", source.Type)
	}
	return source, nil
}

func normalizeSourceType(value SourceType) SourceType {
	return SourceType(strings.ToLower(strings.TrimSpace(string(value))))
}

func normalizeSourceEvidence(evidence Evidence) Evidence {
	evidence.Strategy = EvidenceStrategy(strings.ToLower(strings.TrimSpace(string(evidence.Strategy))))
	if evidence.Strategy == "" {
		evidence.Strategy = EvidenceStrategyUnknown
	}
	evidence.ReasonCode = strings.TrimSpace(evidence.ReasonCode)
	evidence.RequiredInputs = normalizeNextActions(evidence.RequiredInputs)
	evidence.Links = normalizeNextActions(evidence.Links)
	evidence.Metadata = cloneStringAnyMap(evidence.Metadata)
	if evidence.Files != nil {
		files := make([]EvidenceFile, 0, len(evidence.Files))
		seen := make(map[string]struct{}, len(evidence.Files))
		for _, file := range evidence.Files {
			file.Path = filepath.ToSlash(strings.Trim(strings.TrimSpace(file.Path), "/"))
			file.Kind = strings.ToLower(strings.TrimSpace(file.Kind))
			if file.Path == "" {
				continue
			}
			key := file.Kind + "\x00" + file.Path
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			files = append(files, file)
		}
		evidence.Files = files
	}
	return evidence
}

func validateInspectedIntent(intent Intent, evidence Evidence) error {
	switch intent.Kind {
	case IntentKindCompose:
		if strings.TrimSpace(intent.ComposeYAML) == "" {
			return errors.New("Compose inspection did not provide Compose YAML")
		}
	case IntentKindImage:
		if strings.TrimSpace(intent.Image) == "" {
			return errors.New("image inspection did not provide an image reference")
		}
	case IntentKindBuild:
		if strings.TrimSpace(intent.BuildContext) == "" && strings.TrimSpace(intent.DockerfilePath) == "" && strings.TrimSpace(intent.ComposeYAML) == "" {
			return errors.New("build inspection did not provide source evidence")
		}
	default:
		return errors.New("ready inspection did not provide a supported intent kind")
	}
	if evidence.Strategy == EvidenceStrategyUnknown {
		return errors.New("ready inspection requires a deterministic strategy")
	}
	return nil
}
