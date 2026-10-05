package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestEnvironmentScope(t *testing.T) {
	for _, tc := range []struct {
		query, header, want string
		valid               bool
	}{
		{"", "", "local", true},
		{"?environment_id=remote-a", "", "remote-a", true},
		{"?environmentId=remote-a", "remote-a", "remote-a", true},
		{"?environmentId=remote-a&environment_id=local", "", "", false},
		{"?environmentId=remote-a&environmentId=local", "", "", false},
	} {
		t.Run(tc.query+tc.header, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/api/compose"+tc.query, nil)
			c.Request.Header.Set(remoteEnvironmentHeader, tc.header)
			got, valid := requestEnvironmentScope(c)
			if got != tc.want || valid != tc.valid {
				t.Fatalf("got %q/%v, want %q/%v", got, valid, tc.want, tc.valid)
			}
		})
	}
}

func TestLocalEnvironmentScopeRejectsRemoteHeader(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/cleanup", nil)
	c.Request.Header.Set(remoteEnvironmentHeader, "remote-office")
	if _, ok := localEnvironmentScope(c); ok {
		t.Fatal("remote header must not execute locally")
	}
}
