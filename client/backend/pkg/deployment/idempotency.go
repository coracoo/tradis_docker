package deployment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var ErrInvalidIdempotencyInput = errors.New("invalid deployment idempotency input")

type idempotencySource Source

func BuildIdempotencyKey(environmentID string, source Source, projectName string, normalizedInput any) (string, error) {
	environmentID = strings.TrimSpace(environmentID)
	projectName = strings.TrimSpace(projectName)
	source.Type = SourceType(strings.ToLower(strings.TrimSpace(string(source.Type))))
	source.Ref = strings.TrimSpace(source.Ref)
	source.PathHint = strings.TrimSpace(source.PathHint)
	if environmentID == "" || source.Type == "" || projectName == "" {
		return "", ErrInvalidIdempotencyInput
	}

	input, err := normalizeJSONValue(normalizedInput)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidIdempotencyInput, err)
	}
	payload := struct {
		EnvironmentID string            `json:"environmentId"`
		Source        idempotencySource `json:"source"`
		ProjectName   string            `json:"projectName"`
		Input         any               `json:"input"`
	}{
		EnvironmentID: environmentID,
		Source:        idempotencySource(source),
		ProjectName:   projectName,
		Input:         input,
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidIdempotencyInput, err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func normalizeJSONValue(value any) (any, error) {
	var raw []byte
	switch typed := value.(type) {
	case string:
		raw = []byte(typed)
	case []byte:
		raw = typed
	case json.RawMessage:
		raw = typed
	default:
		var err error
		raw, err = json.Marshal(value)
		if err != nil {
			return nil, err
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return normalized, nil
}
