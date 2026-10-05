//go:build community

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCommunityEnvironmentMiddlewareRejectsRemoteTargets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CommunityEnvironmentRoutingMiddleware())
	called := false
	router.GET("/api/containers", func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})

	remote := httptest.NewRequest(http.MethodGet, "/api/containers?environmentId=remote-office", nil)
	remote.Header.Set(remoteEnvironmentHeader, "remote-office")
	remoteRecorder := httptest.NewRecorder()
	router.ServeHTTP(remoteRecorder, remote)
	if remoteRecorder.Code != http.StatusBadRequest {
		t.Fatalf("remote target status = %d, want %d", remoteRecorder.Code, http.StatusBadRequest)
	}
	if called {
		t.Fatal("community build must not route a remote-target request to local resources")
	}

	local := httptest.NewRequest(http.MethodGet, "/api/containers?environmentId=local", nil)
	localRecorder := httptest.NewRecorder()
	router.ServeHTTP(localRecorder, local)
	if localRecorder.Code != http.StatusNoContent || !called {
		t.Fatalf("local target status = %d, called=%v", localRecorder.Code, called)
	}
}
