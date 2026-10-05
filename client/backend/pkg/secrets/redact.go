package secrets

import (
	"encoding/json"
	"net/url"
	"reflect"
	"regexp"
	"strings"
)

const RedactedValue = "[REDACTED]"

var sensitiveKeyPattern = regexp.MustCompile(`(?i)(?:password|passwd|pwd|secret|token|api[_-]?key|authorization|cookie|private[_-]?key|dotenv|credential|access[_-]?key|signature|(?:database|redis)[_-]?url|mongodb[_-]?(?:uri|url)|connection[_-]?string|dsn)`)
var sensitiveAssignmentPattern = regexp.MustCompile(`(?i)(\b[a-z0-9_.-]*(?:password|passwd|pwd|secret|token|api[_-]?key|authorization|cookie|private[_-]?key|credential|access[_-]?key|signature|(?:database|redis)[_-]?url|mongodb[_-]?(?:uri|url)|connection[_-]?string|dsn)[a-z0-9_.-]*\s*[:=]\s*)(?:bearer\s+)?[^\s,;&#]+`)
var sensitiveQueryPattern = regexp.MustCompile(`(?i)([?&](?:password|passwd|pwd|secret|token|api[_-]?key|authorization|cookie|private[_-]?key|credential|access[_-]?key|signature)=)[^&#\s]+`)

// IsSensitiveKey identifies map keys and dotenv names whose values must never
// be persisted in visible logs, job records, notifications, or API responses.
func IsSensitiveKey(key string) bool {
	return sensitiveKeyPattern.MatchString(strings.TrimSpace(key))
}

// RedactString removes common assignment and URL-query secret values while
// preserving surrounding diagnostic text.
func RedactString(value string) string {
	redacted := sensitiveAssignmentPattern.ReplaceAllString(value, "${1}"+RedactedValue)
	return sensitiveQueryPattern.ReplaceAllString(redacted, "${1}"+RedactedValue)
}

// RedactMap returns a new JSON-safe map without mutating the caller's values.
func RedactMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		if strings.EqualFold(strings.TrimSpace(key), "dotenv") {
			output[key] = RedactString(stringValue(value))
			continue
		}
		if IsSensitiveKey(key) {
			output[key] = RedactedValue
			continue
		}
		output[key] = RedactValue(value)
	}
	return output
}

// RedactValue recursively produces a JSON-safe presentation value. It is for
// persistence and responses only; it must not be used for execution inputs.
func RedactValue(value any) any {
	return redactReflectValue(reflect.ValueOf(value))
}

func redactReflectValue(value reflect.Value) any {
	if !value.IsValid() {
		return nil
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.String:
		return RedactString(value.String())
	case reflect.Map:
		output := make(map[string]any, value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			key := strings.TrimSpace(toString(iterator.Key()))
			if strings.EqualFold(key, "dotenv") {
				output[key] = RedactString(stringValue(iterator.Value().Interface()))
				continue
			}
			if IsSensitiveKey(key) {
				output[key] = RedactedValue
				continue
			}
			output[key] = redactReflectValue(iterator.Value())
		}
		return output
	case reflect.Slice, reflect.Array:
		output := make([]any, value.Len())
		for index := 0; index < value.Len(); index++ {
			output[index] = redactReflectValue(value.Index(index))
		}
		return output
	case reflect.Bool:
		return value.Interface()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Interface()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value.Interface()
	case reflect.Float32, reflect.Float64:
		return value.Interface()
	case reflect.Struct:
		if value.CanInterface() {
			raw, err := json.Marshal(value.Interface())
			if err == nil {
				var generic any
				if json.Unmarshal(raw, &generic) == nil {
					return RedactValue(generic)
				}
			}
		}
		return nil
	default:
		if value.CanInterface() {
			return value.Interface()
		}
		return nil
	}
}

func toString(value reflect.Value) string {
	if value.IsValid() && value.Kind() == reflect.String {
		return value.String()
	}
	return ""
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

// RedactURLQuery is for callers that need a URL string specifically. It keeps
// malformed URLs safe through RedactString rather than returning raw input.
func RedactURLQuery(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return RedactString(raw)
	}
	query := parsed.Query()
	for key := range query {
		if IsSensitiveKey(key) {
			query.Set(key, RedactedValue)
		}
	}
	parsed.RawQuery = query.Encode()
	return RedactString(parsed.String())
}
