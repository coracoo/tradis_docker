package secrets

import (
	"fmt"
	"strings"
	"testing"
)

func TestRedactMapHandlesDotenvTokensPasswordsAndNestedValues(t *testing.T) {
	input := map[string]any{
		"safe":      "kept",
		"API_TOKEN": "token-value",
		"nested": map[string]any{
			"db_password":    "password-value",
			"webhook_secret": "webhook-value",
		},
		"dotenv": "APP_MODE=prod\nDB_PASSWORD=password-value\nAPI_TOKEN=token-value\n",
		"items":  []any{map[string]any{"private_key": "private-value"}},
	}

	redacted := RedactMap(input)
	if redacted["safe"] != "kept" {
		t.Fatalf("safe value changed: %#v", redacted)
	}
	for _, secret := range []string{"token-value", "password-value", "webhook-value", "private-value"} {
		if strings.Contains(fmt.Sprintf("%#v", redacted), secret) {
			t.Fatalf("redacted map leaked %q: %#v", secret, redacted)
		}
	}
	if redacted["API_TOKEN"] != RedactedValue {
		t.Fatalf("token key not redacted: %#v", redacted)
	}
	nested := redacted["nested"].(map[string]any)
	if nested["db_password"] != RedactedValue || nested["webhook_secret"] != RedactedValue {
		t.Fatalf("nested secrets not redacted: %#v", nested)
	}
	if dotenv := redacted["dotenv"].(string); !strings.Contains(dotenv, "DB_PASSWORD="+RedactedValue) || !strings.Contains(dotenv, "API_TOKEN="+RedactedValue) {
		t.Fatalf("dotenv was not redacted: %q", dotenv)
	}
}

func TestRedactStringMasksAssignmentsAndSensitiveQueryParameters(t *testing.T) {
	raw := "Authorization=Bearer abc123 password: hunter2 https://example.test/hook?token=url-token&safe=1"
	redacted := RedactString(raw)
	for _, secret := range []string{"abc123", "hunter2", "url-token"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("RedactString leaked %q: %q", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "safe=1") {
		t.Fatalf("RedactString removed a safe query parameter: %q", redacted)
	}
}
