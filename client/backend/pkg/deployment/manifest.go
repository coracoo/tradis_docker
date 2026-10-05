package deployment

import (
	"dockerpanel/backend/pkg/secrets"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

const RedactedValue = secrets.RedactedValue

type Manifest struct {
	JobID          string            `json:"jobId"`
	EnvironmentID  string            `json:"environmentId"`
	Source         Source            `json:"source"`
	SourceCompose  string            `json:"sourceCompose,omitempty"`
	RuntimeCompose string            `json:"runtimeCompose,omitempty"`
	ImageDigests   map[string]string `json:"imageDigests,omitempty"`
	Changes        []PlanChange      `json:"changes,omitempty"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
}

func (source Source) MarshalJSON() ([]byte, error) {
	source.Ref = redactSourceRef(source.Ref)
	source.Metadata = redactMap(source.Metadata)
	type sourceJSON Source
	return json.Marshal(sourceJSON(source))
}

func (change PlanChange) MarshalJSON() ([]byte, error) {
	change.Before = RedactSensitiveValues(change.Before)
	change.After = RedactSensitiveValues(change.After)
	type planChangeJSON PlanChange
	return json.Marshal(planChangeJSON(change))
}

func (manifest Manifest) MarshalJSON() ([]byte, error) {
	redacted, err := RedactManifest(manifest)
	if err != nil {
		return nil, err
	}
	type manifestJSON Manifest
	return json.Marshal(manifestJSON(redacted))
}

func (intent Intent) MarshalJSON() ([]byte, error) {
	redacted, err := redactIntent(intent)
	if err != nil {
		return nil, err
	}
	type intentJSON Intent
	return json.Marshal(intentJSON(redacted))
}

func (plan Plan) MarshalJSON() ([]byte, error) {
	redacted, err := redactPlan(plan)
	if err != nil {
		return nil, err
	}
	type planJSON Plan
	return json.Marshal(planJSON(redacted))
}

func (issue PreflightIssue) MarshalJSON() ([]byte, error) {
	issue.NextActions = normalizeNextActions(issue.NextActions)
	issue.Details = redactMap(issue.Details)
	type preflightIssueJSON PreflightIssue
	return json.Marshal(preflightIssueJSON(issue))
}

func (structured StructuredError) MarshalJSON() ([]byte, error) {
	type structuredErrorJSON struct {
		Code        ErrorCode      `json:"error_code"`
		Message     string         `json:"message"`
		Retryable   bool           `json:"retryable"`
		NextActions []string       `json:"next_actions,omitempty"`
		Details     map[string]any `json:"details,omitempty"`
	}
	return json.Marshal(structuredErrorJSON{
		Code:        structured.Code,
		Message:     structured.PublicMessage(),
		Retryable:   structured.Retryable,
		NextActions: normalizeNextActions(structured.NextActions),
		Details:     redactMap(structured.Details),
	})
}

func redactIntent(intent Intent) (Intent, error) {
	redacted := intent
	redacted.Source.Ref = redactSourceRef(intent.Source.Ref)
	redacted.Source.Metadata = redactMap(intent.Source.Metadata)
	redacted.Parameters = redactMap(intent.Parameters)
	redacted.Options = redactMap(intent.Options)

	var err error
	redacted.ComposeYAML, err = redactComposeYAML(intent.ComposeYAML)
	if err != nil {
		return Intent{}, fmt.Errorf("redact intent compose: %w", err)
	}
	return redacted, nil
}

func redactPlan(plan Plan) (Plan, error) {
	redacted := plan
	var err error
	redacted.Intent, err = redactIntent(plan.Intent)
	if err != nil {
		return Plan{}, err
	}
	redacted.RuntimeCompose, err = redactComposeYAML(plan.RuntimeCompose)
	if err != nil {
		return Plan{}, fmt.Errorf("redact plan compose: %w", err)
	}
	redacted.Changes = redactPlanChanges(plan.Changes)
	redacted.Preflight = redactPreflightResult(plan.Preflight)
	redacted.Metadata = redactMap(plan.Metadata)
	return redacted, nil
}

func redactPlanChanges(changes []PlanChange) []PlanChange {
	if changes == nil {
		return nil
	}
	redacted := make([]PlanChange, len(changes))
	for index, change := range changes {
		redacted[index] = change
		redacted[index].Before = RedactSensitiveValues(change.Before)
		redacted[index].After = RedactSensitiveValues(change.After)
	}
	return redacted
}

func redactPreflightResult(result PreflightResult) PreflightResult {
	if result.Issues == nil {
		return result
	}
	redacted := result
	redacted.Issues = make([]PreflightIssue, len(result.Issues))
	for index, issue := range result.Issues {
		redacted.Issues[index] = issue
		redacted.Issues[index].NextActions = normalizeNextActions(issue.NextActions)
		redacted.Issues[index].Details = redactMap(issue.Details)
	}
	return redacted
}

func RedactManifest(manifest Manifest) (Manifest, error) {
	redacted := manifest
	redacted.Source.Ref = redactSourceRef(manifest.Source.Ref)
	redacted.Source.Metadata = redactMap(manifest.Source.Metadata)
	redacted.ImageDigests = cloneStringMap(manifest.ImageDigests)
	redacted.Metadata = redactMap(manifest.Metadata)
	redacted.Changes = redactPlanChanges(manifest.Changes)

	var err error
	redacted.SourceCompose, err = redactComposeYAML(manifest.SourceCompose)
	if err != nil {
		return Manifest{}, fmt.Errorf("redact source compose: %w", err)
	}
	redacted.RuntimeCompose, err = redactComposeYAML(manifest.RuntimeCompose)
	if err != nil {
		return Manifest{}, fmt.Errorf("redact runtime compose: %w", err)
	}
	return redacted, nil
}

func redactSourceRef(ref string) string {
	parsed, err := url.Parse(strings.TrimSpace(ref))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ref
	}
	parsed.User = nil
	parsed.Fragment = ""
	query := parsed.Query()
	for key := range query {
		if isSensitiveURLParameter(key) {
			query.Set(key, RedactedValue)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func isSensitiveURLParameter(key string) bool {
	if isSensitiveField(key) {
		return true
	}
	normalized := normalizeSensitiveKey(key)
	return normalized == "key" || normalized == "sig" || strings.Contains(normalized, "signature")
}

func RedactSensitiveValues(value any) any {
	return secrets.RedactValue(value)
}

func redactMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	return secrets.RedactMap(input)
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func redactComposeYAML(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
		return "", err
	}
	redactYAMLNode(&document, "", false)
	redacted, err := yaml.Marshal(&document)
	if err != nil {
		return "", err
	}
	return string(redacted), nil
}

func redactYAMLNode(node *yaml.Node, parentKey string, inSecretDefinitions bool) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			redactYAMLNode(child, parentKey, inSecretDefinitions)
		}
	case yaml.MappingNode:
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index].Value
			value := node.Content[index+1]
			isSecretDefinitions := normalizeSensitiveKey(key) == "secrets"
			if !inSecretDefinitions && !isSecretDefinitions && isSensitiveField(key) {
				setYAMLRedacted(value)
				continue
			}
			redactYAMLNode(value, key, inSecretDefinitions || isSecretDefinitions)
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if normalizeSensitiveKey(parentKey) == "environment" && child.Kind == yaml.ScalarNode {
				key, _, found := strings.Cut(child.Value, "=")
				if found && isSensitiveField(key) {
					child.Value = key + "=" + RedactedValue
				}
				continue
			}
			redactYAMLNode(child, parentKey, inSecretDefinitions)
		}
	}
}

func setYAMLRedacted(node *yaml.Node) {
	node.Kind = yaml.ScalarNode
	node.Tag = "!!str"
	node.Value = RedactedValue
	node.Content = nil
	node.Alias = nil
}

func isSensitiveField(key string) bool {
	if secrets.IsSensitiveKey(key) {
		return true
	}
	normalized := normalizeSensitiveKey(key)
	if normalized == "" {
		return false
	}
	for _, fragment := range []string{
		"password",
		"passwd",
		"token",
		"secret",
		"apikey",
		"accesskey",
		"privatekey",
		"licensekey",
		"credential",
		"authorization",
		"authheader",
		"dockerconfigjson",
		"signature",
	} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	switch normalized {
	case "pwd", "auth", "cookie", "dotenv", "dsn", "databaseurl", "redisurl", "mongodburi", "connectionstring":
		return true
	default:
		return false
	}
}

func normalizeSensitiveKey(key string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			return unicode.ToLower(char)
		}
		return -1
	}, strings.TrimSpace(key))
}
