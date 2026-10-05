package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestLevelFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
		debug    string
		want     slog.Level
	}{
		{name: "default", want: slog.LevelInfo},
		{name: "invalid", logLevel: "verbose", want: slog.LevelInfo},
		{name: "debug", logLevel: "debug", want: slog.LevelDebug},
		{name: "info", logLevel: "info", want: slog.LevelInfo},
		{name: "warn", logLevel: "warn", want: slog.LevelWarn},
		{name: "error", logLevel: "error", want: slog.LevelError},
		{name: "legacy debug", debug: "true", want: slog.LevelDebug},
		{name: "explicit level wins", logLevel: "warn", debug: "true", want: slog.LevelWarn},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", testCase.logLevel)
			t.Setenv("DEBUG", testCase.debug)
			if got := LevelFromEnv(); got != testCase.want {
				t.Fatalf("LevelFromEnv() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestRedactTextRemovesRemoteURLs(t *testing.T) {
	input := `request failed: Get "https://user:secret@template.example:33333/api/items?token=abc#part"; fallback http://cdn.example/feed.xml`
	got := RedactText(input)
	for _, secret := range []string{
		"https://",
		"http://",
		"user",
		"secret",
		"template.example",
		"33333",
		"token=abc",
		"cdn.example",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("RedactText() leaked %q: %s", secret, got)
		}
	}
	if count := strings.Count(got, "<redacted-url>"); count != 2 {
		t.Fatalf("redacted URL count = %d, want 2: %s", count, got)
	}
}

func TestLegacyWriterFiltersAndMapsLevels(t *testing.T) {
	var output bytes.Buffer
	logger := newLogger(&output, slog.LevelInfo)
	writer := &legacyWriter{logger: logger}

	_, _ = writer.Write([]byte("[DEBUG] hidden detail\n"))
	_, _ = writer.Write([]byte("normal startup\n"))
	_, _ = writer.Write([]byte("[WARN] degraded\n"))
	_, _ = writer.Write([]byte("后台任务执行失败\n"))
	_, _ = writer.Write([]byte("[ERROR] request to https://official.example/api failed\n"))

	got := output.String()
	if strings.Contains(got, "hidden detail") {
		t.Fatalf("debug message was not filtered: %s", got)
	}
	for _, expected := range []string{"level=INFO", "normal startup", "level=WARN", "degraded", "后台任务执行失败", "level=ERROR"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing %q in output: %s", expected, got)
		}
	}
	if strings.Contains(got, "official.example") || strings.Contains(got, "https://") {
		t.Fatalf("legacy output leaked URL: %s", got)
	}
}

func TestLoggerFiltersDebugAtInfo(t *testing.T) {
	var output bytes.Buffer
	logger := newLogger(&output, slog.LevelInfo)

	logger.Debug("hidden")
	logger.Info("visible")

	got := output.String()
	if strings.Contains(got, "hidden") {
		t.Fatalf("debug output was not filtered: %s", got)
	}
	if !strings.Contains(got, "visible") || !strings.Contains(got, "level=INFO") {
		t.Fatalf("info output missing: %s", got)
	}
}
