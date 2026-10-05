//go:build community

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/settings"

	"github.com/gin-gonic/gin"
)

func TestCommunityVersionRoutesDoNotExposeOfficialReleaseState(t *testing.T) {
	_ = database.Close()
	if err := database.InitDB(filepath.Join(t.TempDir(), "data.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := settings.InitSettingsTable(); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"appstore_server_version":     "v99.0.0",
		"appstore_has_new_version":    "true",
		"appstore_version_checked_at": "2026-01-01T00:00:00Z",
		"appstore_release_payload":    `{"mandatory":true}`,
	} {
		if err := settings.SetValue(key, value); err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api"))

	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/settings/version-status", nil))
	if status.Code != http.StatusOK {
		t.Fatalf("version status = %d: %s", status.Code, status.Body.String())
	}
	var payload versionStatusResponse
	if err := json.Unmarshal(status.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.HasNewVersion || payload.ServerVersion != "" || payload.Release != nil || payload.Channel != "community" {
		t.Fatalf("community version status leaked official state: %#v", payload)
	}

	check := httptest.NewRecorder()
	router.ServeHTTP(check, httptest.NewRequest(http.MethodPost, "/api/settings/version-check", nil))
	if check.Code != http.StatusNotImplemented {
		t.Fatalf("version check = %d: %s", check.Code, check.Body.String())
	}
	if !strings.Contains(check.Body.String(), "community_release_manifest_unavailable") {
		t.Fatalf("unexpected community update response: %s", check.Body.String())
	}
}
