package templatecompiler

import (
	"fmt"
	"regexp"
	"strings"
)

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func parseDotenv(content, file string) (map[string]string, []Diagnostic) {
	values := make(map[string]string)
	seen := make(map[string]int)
	diagnostics := make([]Diagnostic, 0)
	for idx, raw := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		if !envKeyPattern.MatchString(key) {
			diagnostics = append(diagnostics, Diagnostic{Code: "invalid_dotenv_key", Severity: "error", Message: fmt.Sprintf("invalid dotenv key %q", key), File: file, Line: idx + 1})
			continue
		}
		value := ""
		if len(parts) == 2 {
			value = strings.TrimSpace(parts[1])
			if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
				value = value[1 : len(value)-1]
			}
		}
		if previous, ok := seen[key]; ok {
			diagnostics = append(diagnostics, Diagnostic{Code: "duplicate_dotenv_key", Severity: "error", Message: fmt.Sprintf("dotenv key %s is defined more than once (first at line %d)", key, previous), File: file, Line: idx + 1})
			continue
		}
		seen[key] = idx + 1
		values[key] = value
	}
	return values, diagnostics
}

func sensitiveKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, token := range []string{"PASSWORD", "PASSWD", "SECRET", "TOKEN", "API_KEY", "PRIVATE_KEY"} {
		if strings.Contains(upper, token) {
			return true
		}
	}
	return false
}
