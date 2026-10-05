package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dockerpanel/backend/pkg/registryprobe"
	"github.com/gin-gonic/gin"
)

func mirrorCheckRequest(t *testing.T, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterImageRoutes(router.Group("/api"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/images/mirrors/check", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(remoteEnvironmentHeader, target)
	router.ServeHTTP(w, r)
	return w
}

func TestMirrorCheckRejectsRemoteAndInvalidBeforeProbe(t *testing.T) {
	original := registryMirrorProbeCheck
	defer func() { registryMirrorProbeCheck = original }()
	registryMirrorProbeCheck = func(context.Context, string) registryprobe.Result {
		t.Fatal("rejected request must not probe the local device")
		return registryprobe.Result{}
	}
	for _, tc := range []struct {
		target, body string
		code         int
	}{
		{"remote-test", `{"url":"https://mirror.example"}`, http.StatusNotImplemented},
		{"local", `{"url":"https://localhost"}`, http.StatusBadRequest},
		{"local", `{"url":"https://mirror.example?token=secret"}`, http.StatusBadRequest},
		{"local", "{" + strings.Repeat(" ", 5000), http.StatusBadRequest},
	} {
		response := mirrorCheckRequest(t, tc.target, tc.body)
		if response.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.target, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "secret") {
			t.Fatal("response leaked invalid input")
		}
	}
}

func TestMirrorCheckReportsReachabilityWithoutChangingConfiguration(t *testing.T) {
	original := registryMirrorProbeCheck
	defer func() { registryMirrorProbeCheck = original }()
	registryMirrorProbeCheck = func(_ context.Context, url string) registryprobe.Result {
		return registryprobe.Result{URL: url, Status: "reachable", CheckedAt: "2026-10-01T01:00:00Z"}
	}
	response := mirrorCheckRequest(t, "local", `{"url":"https://mirror.example/v2/"}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"reachable"`) {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}

func TestMirrorCheckBusyReturnsWithoutWaiting(t *testing.T) {
	for i := 0; i < cap(registryMirrorProbeSlots); i++ {
		registryMirrorProbeSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(registryMirrorProbeSlots); i++ {
			<-registryMirrorProbeSlots
		}
	}()
	response := mirrorCheckRequest(t, "local", `{"url":"https://mirror.example"}`)
	if response.Code != http.StatusTooManyRequests || !strings.Contains(response.Body.String(), "mirror_probe_busy") {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}
