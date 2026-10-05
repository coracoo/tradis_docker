package logging

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGinMiddlewareLevelsAndPathSafety(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		level      slog.Level
		status     int
		wantLevel  string
		wantOutput bool
	}{
		{name: "successful request hidden at info", level: slog.LevelInfo, status: http.StatusOK},
		{name: "successful request visible at debug", level: slog.LevelDebug, status: http.StatusOK, wantLevel: "DEBUG", wantOutput: true},
		{name: "client error is warning", level: slog.LevelInfo, status: http.StatusNotFound, wantLevel: "WARN", wantOutput: true},
		{name: "server error is error", level: slog.LevelInfo, status: http.StatusInternalServerError, wantLevel: "ERROR", wantOutput: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			restore := setDefaultLogger(newLogger(&output, testCase.level))
			t.Cleanup(restore)

			router := gin.New()
			router.Use(GinMiddleware())
			router.GET("/items/:id", func(c *gin.Context) {
				c.Status(testCase.status)
			})

			request := httptest.NewRequest(http.MethodGet, "/items/42?token=secret", nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			got := output.String()
			if !testCase.wantOutput {
				if got != "" {
					t.Fatalf("unexpected access log: %s", got)
				}
				return
			}
			for _, expected := range []string{
				"level=" + testCase.wantLevel,
				`method=GET`,
				`path=/items/:id`,
				`status=` + strconv.Itoa(testCase.status),
			} {
				if !strings.Contains(got, expected) {
					t.Fatalf("missing %q in access log: %s", expected, got)
				}
			}
			if strings.Contains(got, "token") || strings.Contains(got, "secret") || strings.Contains(got, "/items/42") {
				t.Fatalf("access log leaked request URL details: %s", got)
			}
		})
	}
}

func TestGinRecoveryDoesNotLogRequestSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var output bytes.Buffer
	restore := setDefaultLogger(newLogger(&output, slog.LevelInfo))
	t.Cleanup(restore)

	router := gin.New()
	router.Use(GinMiddleware(), GinRecovery())
	router.GET("/panic", func(c *gin.Context) {
		panic("request to https://official.example/private failed")
	})

	request := httptest.NewRequest(http.MethodGet, "/panic?token=query-secret", nil)
	request.Header.Set("Authorization", "Bearer header-secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	got := output.String()
	for _, forbidden := range []string{
		"official.example",
		"query-secret",
		"header-secret",
		"Authorization",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("recovery log leaked %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "panic recovered") || !strings.Contains(got, "level=ERROR") {
		t.Fatalf("recovery error log missing: %s", got)
	}
}
