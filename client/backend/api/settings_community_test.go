//go:build community

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/deployment"
	"dockerpanel/backend/pkg/settings"

	"github.com/gin-gonic/gin"
)

func TestCommunitySettingsRejectCommercialFieldsAndDoNotExposeThem(t *testing.T) {
	_ = database.Close()
	t.Setenv("APPSTORE_CDN_URL", "https://cdn.example.com")
	if err := database.InitDB(filepath.Join(t.TempDir(), "data.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := settings.InitSettingsTable(); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api"))

	freeAI := httptest.NewRecorder()
	router.ServeHTTP(freeAI, httptest.NewRequest(http.MethodPost, "/api/settings/global", strings.NewReader(`{"aiEnabled":true,"aiBaseUrl":"https://models.example/v1","aiModel":"example-model"}`)))
	if freeAI.Code != http.StatusOK {
		t.Fatalf("free AI settings update = %d: %s", freeAI.Code, freeAI.Body.String())
	}
	rss := httptest.NewRecorder()
	router.ServeHTTP(rss, httptest.NewRequest(http.MethodPost, "/api/settings/global", strings.NewReader(`{"tutorialRSSURL":" https://cdn.example.com/api/community/tutorials/feed.xml "}`)))
	if rss.Code != http.StatusOK {
		t.Fatalf("public RSS setting update = %d: %s", rss.Code, rss.Body.String())
	}
	invalidRSS := httptest.NewRecorder()
	router.ServeHTTP(invalidRSS, httptest.NewRequest(http.MethodPost, "/api/settings/global", strings.NewReader(`{"tutorialRSSURL":"http://127.0.0.1/private.xml"}`)))
	if invalidRSS.Code != http.StatusBadRequest {
		t.Fatalf("private RSS source update = %d: %s", invalidRSS.Code, invalidRSS.Body.String())
	}

	unsupported := httptest.NewRecorder()
	router.ServeHTTP(unsupported, httptest.NewRequest(http.MethodPost, "/api/settings/global", strings.NewReader(`{"agentSafetyMode":"ask"}`)))
	if unsupported.Code != http.StatusBadRequest || !strings.Contains(unsupported.Body.String(), "community_feature_unavailable") {
		t.Fatalf("Agent settings update = %d: %s", unsupported.Code, unsupported.Body.String())
	}

	unknown := httptest.NewRecorder()
	router.ServeHTTP(unknown, httptest.NewRequest(http.MethodPost, "/api/settings/global", strings.NewReader(`{"unsupportedSetting":true}`)))
	if unknown.Code != http.StatusBadRequest || !strings.Contains(unknown.Body.String(), "community_feature_unavailable") {
		t.Fatalf("unknown settings update = %d: %s", unknown.Code, unknown.Body.String())
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/settings/global", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("community settings = %d: %s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["aiEnabled"] != true || payload["aiModel"] != "example-model" {
		t.Fatalf("free AI settings missing: %s", response.Body.String())
	}
	if payload["tutorialRSSURL"] != "https://cdn.example.com/api/community/tutorials/feed.xml" {
		t.Fatalf("public RSS setting missing: %s", response.Body.String())
	}
	for _, field := range []string{"aiAgentPrompt", "agentSafetyMode", "githubAppSearchToken"} {
		if _, exists := payload[field]; exists {
			t.Fatalf("community settings payload exposed %q: %s", field, response.Body.String())
		}
	}
}

func TestCommunitySettingsHandlerDoesNotCompileCommercialFieldNames(t *testing.T) {
	command := exec.Command("go", "list", "-json", "-tags", "community", ".")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list community api package: %v", err)
	}

	var listed struct {
		Dir     string
		GoFiles []string
	}
	if err := json.Unmarshal(output, &listed); err != nil {
		t.Fatalf("decode go list output: %v", err)
	}

	for _, file := range listed.GoFiles {
		if file != "settings_community.go" {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(listed.Dir, file))
		if readErr != nil {
			t.Fatalf("read compiled source %s: %v", file, readErr)
		}
		for _, forbidden := range []string{
			"agentSafetyMode",
			"githubAppSearchToken",
		} {
			if strings.Contains(string(content), forbidden) {
				t.Fatalf("community settings source unexpectedly contains %q", forbidden)
			}
		}
		return
	}
	t.Fatal("settings_community.go is not compiled by the community build")
}

func TestCommunityDeploymentDefaultsRemainLocalAndPersist(t *testing.T) {
	_ = database.Close()
	root := t.TempDir()
	if err := database.InitDB(filepath.Join(root, "data.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := settings.InitSettingsTable(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROJECT_ROOT", root)
	t.Setenv("PUID", "1026")
	t.Setenv("PGID", "100")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api"))

	initial := httptest.NewRecorder()
	router.ServeHTTP(initial, httptest.NewRequest(http.MethodGet, "/api/settings/deployment-defaults", nil))
	if initial.Code != http.StatusOK {
		t.Fatalf("GET deployment defaults = %d: %s", initial.Code, initial.Body.String())
	}
	var initialPayload struct {
		Profile deployment.NASProfile `json:"profile"`
	}
	if err := json.Unmarshal(initial.Body.Bytes(), &initialPayload); err != nil {
		t.Fatal(err)
	}
	if initialPayload.Profile.EnvironmentID != database.LocalEnvironmentID || initialPayload.Profile.PUID != 1026 || initialPayload.Profile.PGID != 100 {
		t.Fatalf("unexpected initial local profile: %#v", initialPayload.Profile)
	}

	mediaPath := filepath.Join(t.TempDir(), "media")
	update := httptest.NewRecorder()
	body := []byte(`{"puid":1001,"pgid":1002,"timezone":"Asia/Tokyo","networkPolicy":{"allowHost":false},"libraryPaths":{"media":"` + mediaPath + `"}}`)
	router.ServeHTTP(update, httptest.NewRequest(http.MethodPut, "/api/settings/deployment-defaults", bytes.NewReader(body)))
	if update.Code != http.StatusOK {
		t.Fatalf("PUT deployment defaults = %d: %s", update.Code, update.Body.String())
	}
	var updatePayload struct {
		Profile deployment.NASProfile `json:"profile"`
	}
	if err := json.Unmarshal(update.Body.Bytes(), &updatePayload); err != nil {
		t.Fatal(err)
	}
	if updatePayload.Profile.PUID != 1001 || updatePayload.Profile.PGID != 1002 || updatePayload.Profile.Timezone != "Asia/Tokyo" || updatePayload.Profile.NetworkPolicy.AllowHost || updatePayload.Profile.LibraryPaths.Media != mediaPath {
		t.Fatalf("saved profile mismatch: %#v", updatePayload.Profile)
	}

	legacyEnvironmentRoute := httptest.NewRecorder()
	router.ServeHTTP(legacyEnvironmentRoute, httptest.NewRequest(http.MethodGet, "/api/environments/local/profile", nil))
	if legacyEnvironmentRoute.Code != http.StatusNotFound {
		t.Fatalf("community must not register /environments: %d", legacyEnvironmentRoute.Code)
	}
}
