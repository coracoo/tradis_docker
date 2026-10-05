package logging

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
)

var remoteURLPattern = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)

type Logger struct {
	inner *slog.Logger
}

var (
	defaultMu     sync.RWMutex
	defaultLogger = newLogger(os.Stderr, slog.LevelInfo)
)

func init() {
	Configure()
}

func LevelFromEnv() slog.Level {
	if configured := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))); configured != "" {
		switch configured {
		case "debug":
			return slog.LevelDebug
		case "info":
			return slog.LevelInfo
		case "warn", "warning":
			return slog.LevelWarn
		case "error":
			return slog.LevelError
		default:
			return slog.LevelInfo
		}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG"))) {
	case "1", "true", "yes", "on":
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

func IsDebugEnabled() bool {
	return LevelFromEnv() <= slog.LevelDebug
}

func Configure() {
	logger := newLogger(os.Stdout, LevelFromEnv())
	setDefaultLogger(logger)
	slog.SetDefault(logger.inner)
	log.SetFlags(0)
	log.SetPrefix("")
	log.SetOutput(&legacyWriter{logger: logger})
}

func RedactText(value string) string {
	return remoteURLPattern.ReplaceAllString(value, "<redacted-url>")
}

func newLogger(out io.Writer, level slog.Level) *Logger {
	handler := slog.NewTextHandler(
		&redactingWriter{out: out},
		&slog.HandlerOptions{Level: level},
	)
	return &Logger{inner: slog.New(handler)}
}

func setDefaultLogger(logger *Logger) func() {
	defaultMu.Lock()
	previous := defaultLogger
	defaultLogger = logger
	defaultMu.Unlock()
	return func() {
		defaultMu.Lock()
		defaultLogger = previous
		defaultMu.Unlock()
	}
}

func currentLogger() *Logger {
	defaultMu.RLock()
	logger := defaultLogger
	defaultMu.RUnlock()
	return logger
}

func Enabled(level slog.Level) bool {
	return currentLogger().inner.Enabled(context.Background(), level)
}

func Debug(message string, args ...any) {
	currentLogger().Debug(message, args...)
}

func Info(message string, args ...any) {
	currentLogger().Info(message, args...)
}

func Warn(message string, args ...any) {
	currentLogger().Warn(message, args...)
}

func Error(message string, args ...any) {
	currentLogger().Error(message, args...)
}

func Debugf(format string, args ...any) {
	Debug(fmt.Sprintf(format, args...))
}

func Infof(format string, args ...any) {
	Info(fmt.Sprintf(format, args...))
}

func Warnf(format string, args ...any) {
	Warn(fmt.Sprintf(format, args...))
}

func Errorf(format string, args ...any) {
	Error(fmt.Sprintf(format, args...))
}

func (l *Logger) Debug(message string, args ...any) {
	l.inner.Debug(RedactText(message), sanitizeArgs(args)...)
}

func (l *Logger) Info(message string, args ...any) {
	l.inner.Info(RedactText(message), sanitizeArgs(args)...)
}

func (l *Logger) Warn(message string, args ...any) {
	l.inner.Warn(RedactText(message), sanitizeArgs(args)...)
}

func (l *Logger) Error(message string, args ...any) {
	l.inner.Error(RedactText(message), sanitizeArgs(args)...)
}

func sanitizeArgs(args []any) []any {
	if len(args) == 0 {
		return nil
	}
	sanitized := make([]any, len(args))
	for i, value := range args {
		switch typed := value.(type) {
		case string:
			sanitized[i] = RedactText(typed)
		case error:
			sanitized[i] = RedactText(typed.Error())
		default:
			sanitized[i] = value
		}
	}
	return sanitized
}

type redactingWriter struct {
	out io.Writer
}

func (w *redactingWriter) Write(data []byte) (int, error) {
	redacted := []byte(RedactText(string(data)))
	_, err := w.out.Write(redacted)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

type legacyWriter struct {
	logger *Logger
}

func (w *legacyWriter) Write(data []byte) (int, error) {
	message := strings.TrimSpace(string(data))
	level, message := legacyLevel(message)
	switch level {
	case slog.LevelDebug:
		w.logger.Debug(message)
	case slog.LevelWarn:
		w.logger.Warn(message)
	case slog.LevelError:
		w.logger.Error(message)
	default:
		w.logger.Info(message)
	}
	return len(data), nil
}

func legacyLevel(message string) (slog.Level, string) {
	prefixes := []struct {
		prefix string
		level  slog.Level
	}{
		{prefix: "[DEBUG]", level: slog.LevelDebug},
		{prefix: "[Debug]", level: slog.LevelDebug},
		{prefix: "[INFO]", level: slog.LevelInfo},
		{prefix: "[WARN]", level: slog.LevelWarn},
		{prefix: "[WARNING]", level: slog.LevelWarn},
		{prefix: "[ERROR]", level: slog.LevelError},
		{prefix: "[FATAL]", level: slog.LevelError},
	}
	for _, item := range prefixes {
		if strings.HasPrefix(message, item.prefix) {
			return item.level, strings.TrimSpace(strings.TrimPrefix(message, item.prefix))
		}
	}
	if strings.HasPrefix(message, "警告") {
		return slog.LevelWarn, message
	}
	lower := strings.ToLower(message)
	if strings.Contains(message, "失败") ||
		strings.Contains(message, "错误") ||
		strings.HasPrefix(lower, "failed") ||
		strings.HasPrefix(lower, "error") ||
		strings.Contains(lower, " failed") ||
		strings.Contains(lower, " error") {
		return slog.LevelWarn, message
	}
	return slog.LevelInfo, message
}
