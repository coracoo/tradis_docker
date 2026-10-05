package deployment

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type composeDocument struct {
	root     map[string]any
	services map[string]any
}

func parseComposeDocument(raw string) (composeDocument, error) {
	if strings.TrimSpace(raw) == "" {
		return composeDocument{}, fmt.Errorf("compose YAML is empty")
	}
	decoded := map[string]any{}
	if err := yaml.Unmarshal([]byte(raw), &decoded); err != nil {
		return composeDocument{}, fmt.Errorf("parse compose YAML: %w", err)
	}
	normalized, err := normalizeComposeMap(decoded)
	if err != nil {
		return composeDocument{}, err
	}
	services, ok := normalized["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return composeDocument{}, fmt.Errorf("compose services must be a non-empty mapping")
	}
	return composeDocument{root: normalized, services: services}, nil
}

func normalizeComposeMap(input map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(input))
	for key, value := range input {
		normalized, err := normalizeComposeValue(value)
		if err != nil {
			return nil, err
		}
		out[key] = normalized
	}
	return out, nil
}

func normalizeComposeValue(value any) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		return normalizeComposeMap(typed)
	case map[any]any:
		mapped := make(map[string]any, len(typed))
		for key, item := range typed {
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("compose mapping key %v is not a string", key)
			}
			mapped[name] = item
		}
		return normalizeComposeMap(mapped)
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			normalized, err := normalizeComposeValue(item)
			if err != nil {
				return nil, err
			}
			out[index] = normalized
		}
		return out, nil
	default:
		return value, nil
	}
}

func marshalComposeDocument(document composeDocument) (string, error) {
	raw, err := yaml.Marshal(document.root)
	if err != nil {
		return "", fmt.Errorf("marshal compose YAML: %w", err)
	}
	return string(raw), nil
}
