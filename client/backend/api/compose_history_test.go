package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dockerpanel/backend/pkg/composehistory"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/secrets"
	"dockerpanel/backend/pkg/settings"

	"github.com/gin-gonic/gin"
)

func TestComposeConfigPreviewIsSemanticAndRedacted(t *testing.T) {
	router, projectDir := setupComposeHistoryAPI(t, true)
	currentYAML := "services:\n  web:\n    image: nginx:1\n    command: old-command-secret\n"
	writeComposeHistoryProject(t, projectDir, currentYAML, "API_TOKEN=old-env-secret\n")

	response := performComposeHistoryRequest(t, router, http.MethodPost, "/api/compose/demo/config/preview", map[string]any{
		"yaml": "services:\n  web:\n    image: nginx:2\n    command: new-command-secret\n",
		"env":  "API_TOKEN=new-env-secret\n",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("preview status = %d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, forbidden := range []string{"old-command-secret", "new-command-secret", "old-env-secret", "new-env-secret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("preview leaked %q: %s", forbidden, body)
		}
	}
	var preview composehistory.Preview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.HasChanges || !preview.YAMLChanged || !preview.EnvChanged || preview.BaseHash == "" {
		t.Fatalf("unexpected preview: %#v", preview)
	}
}

func TestComposeConfigApplyRejectsStalePreviewAndStoresEncryptedSnapshot(t *testing.T) {
	router, projectDir := setupComposeHistoryAPI(t, true)
	originalYAML := "services:\n  web:\n    image: nginx:1\n"
	writeComposeHistoryProject(t, projectDir, originalYAML, "TOKEN=old\n")
	candidateYAML := "services:\n  web:\n    image: nginx:2\n"

	preview := requestComposePreview(t, router, candidateYAML, "TOKEN=new\n")
	externalYAML := "services:\n  web:\n    image: nginx:external\n"
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yml"), []byte(externalYAML), 0644); err != nil {
		t.Fatal(err)
	}
	conflict := performComposeHistoryRequest(t, router, http.MethodPost, "/api/compose/demo/config/apply", map[string]any{
		"yaml": candidateYAML, "env": "TOKEN=new\n", "baseHash": preview.BaseHash,
	})
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "compose_config_conflict") {
		t.Fatalf("conflict response = %d %s", conflict.Code, conflict.Body.String())
	}
	assertComposeHistoryFile(t, filepath.Join(projectDir, "compose.yml"), externalYAML)

	preview = requestComposePreview(t, router, candidateYAML, "TOKEN=new\n")
	applied := performComposeHistoryRequest(t, router, http.MethodPost, "/api/compose/demo/config/apply", map[string]any{
		"yaml": candidateYAML, "env": "TOKEN=new\n", "baseHash": preview.BaseHash,
	})
	if applied.Code != http.StatusOK {
		t.Fatalf("apply response = %d %s", applied.Code, applied.Body.String())
	}
	assertComposeHistoryFile(t, filepath.Join(projectDir, "compose.yml"), candidateYAML)
	assertComposeHistoryFile(t, filepath.Join(projectDir, ".env"), "TOKEN=new\n")

	records, err := database.ListComposeHistory("local", "demo", 10)
	if err != nil || len(records) != 1 {
		t.Fatalf("history records = %#v, %v", records, err)
	}
	if strings.Contains(records[0].SnapshotSealed, "nginx:external") {
		t.Fatal("stored snapshot contains plaintext YAML")
	}
	opened, err := composehistory.OpenSnapshot(records[0].SnapshotSealed, "local", "demo", records[0].ID)
	if err != nil || opened != externalYAML {
		t.Fatalf("opened snapshot = %q, %v", opened, err)
	}
}

func TestComposeConfigApplyEnvOnlyDoesNotCreateYAMLHistory(t *testing.T) {
	router, projectDir := setupComposeHistoryAPI(t, true)
	yamlContent := "services:\n  web:\n    image: nginx:1\n"
	writeComposeHistoryProject(t, projectDir, yamlContent, "TOKEN=old\n")
	preview := requestComposePreview(t, router, yamlContent, "TOKEN=new\n")

	response := performComposeHistoryRequest(t, router, http.MethodPost, "/api/compose/demo/config/apply", map[string]any{
		"yaml": yamlContent, "env": "TOKEN=new\n", "baseHash": preview.BaseHash,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("env-only apply = %d %s", response.Code, response.Body.String())
	}
	var result map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &result)
	if result["yamlChanged"] != false || result["envChanged"] != true || result["historyCreated"] != false {
		t.Fatalf("unexpected env-only result: %#v", result)
	}
	records, err := database.ListComposeHistory("local", "demo", 10)
	if err != nil || len(records) != 0 {
		t.Fatalf("env-only apply created history: %#v, %v", records, err)
	}
}

func TestComposeHistoryRestoreChangesOnlyYAMLAndDoesNotDeploy(t *testing.T) {
	router, projectDir := setupComposeHistoryAPI(t, true)
	first := "services:\n  web:\n    image: nginx:1\n"
	second := "services:\n  web:\n    image: nginx:2\n"
	third := "services:\n  web:\n    image: nginx:3\n"
	writeComposeHistoryProject(t, projectDir, first, "TOKEN=keep\n")
	applyComposeCandidate(t, router, second, "TOKEN=keep\n")
	applyComposeCandidate(t, router, third, "TOKEN=keep\n")

	list := performComposeHistoryRequest(t, router, http.MethodGet, "/api/compose/demo/config/history", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("history list = %d %s", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), "services:") || strings.Contains(list.Body.String(), "tradis:v1:") {
		t.Fatalf("history list leaked snapshot: %s", list.Body.String())
	}
	var payload struct {
		Current struct {
			ID       string `json:"id"`
			YAMLHash string `json:"yamlHash"`
		} `json:"current"`
		Items []struct {
			ID       string `json:"id"`
			YAMLHash string `json:"yamlHash"`
		} `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	firstHash, _ := composehistory.YAMLHash(first)
	historyID := ""
	for _, item := range payload.Items {
		if item.YAMLHash == firstHash {
			historyID = item.ID
		}
	}
	if payload.Current.ID != "current" || historyID == "" {
		t.Fatalf("unexpected history list: %#v", payload)
	}

	previewResponse := performComposeHistoryRequest(t, router, http.MethodPost,
		"/api/compose/demo/config/history/"+historyID+"/preview", nil)
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("restore preview = %d %s", previewResponse.Code, previewResponse.Body.String())
	}
	var preview composehistory.Preview
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}

	restored := performComposeHistoryRequest(t, router, http.MethodPost,
		"/api/compose/demo/config/history/"+historyID+"/restore", map[string]any{"baseHash": preview.BaseHash})
	if restored.Code != http.StatusOK || !strings.Contains(restored.Body.String(), `"deployed":false`) {
		t.Fatalf("restore = %d %s", restored.Code, restored.Body.String())
	}
	assertComposeHistoryFile(t, filepath.Join(projectDir, "compose.yml"), first)
	assertComposeHistoryFile(t, filepath.Join(projectDir, ".env"), "TOKEN=keep\n")
}

func TestComposeConfigApplyCompensatesHistoryWhenFileReplacementFails(t *testing.T) {
	router, projectDir := setupComposeHistoryAPI(t, true)
	original := "services:\n  web:\n    image: nginx:1\n"
	writeComposeHistoryProject(t, projectDir, original, "TOKEN=old\n")
	preview := requestComposePreview(t, router, "services:\n  web:\n    image: nginx:2\n", "TOKEN=new\n")

	originalReplace := composeHistoryReplaceFiles
	composeHistoryReplaceFiles = func([]composehistory.FileUpdate) error { return errors.New("injected write failure") }
	t.Cleanup(func() { composeHistoryReplaceFiles = originalReplace })

	response := performComposeHistoryRequest(t, router, http.MethodPost, "/api/compose/demo/config/apply", map[string]any{
		"yaml": "services:\n  web:\n    image: nginx:2\n", "env": "TOKEN=new\n", "baseHash": preview.BaseHash,
	})
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("apply failure = %d %s", response.Code, response.Body.String())
	}
	assertComposeHistoryFile(t, filepath.Join(projectDir, "compose.yml"), original)
	records, err := database.ListComposeHistory("local", "demo", 20)
	if err != nil || len(records) != 0 {
		t.Fatalf("failed apply retained history: %#v, %v", records, err)
	}
}

func TestComposeConfigEditableWithoutAdvancedMode(t *testing.T) {
	router, projectDir := setupComposeHistoryAPI(t, false)
	writeComposeHistoryProject(t, projectDir, "services:\n  web:\n    image: nginx:1\n", "")
	response := performComposeHistoryRequest(t, router, http.MethodPost, "/api/compose/demo/config/preview", map[string]any{
		"yaml": "services:\n  web:\n    image: nginx:2\n", "env": "",
	})
	if response.Code == http.StatusForbidden {
		t.Fatalf("compose config edit should not require advanced mode, got 403: %s", response.Body.String())
	}
}

func setupComposeHistoryAPI(t *testing.T, advanced bool) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	_ = database.Close()
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	t.Setenv("TRADIS_DATA_DIR", dataDir)
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 11)
	}
	t.Setenv("TRADIS_MASTER_KEY", base64.StdEncoding.EncodeToString(key))
	t.Setenv("TRADIS_MASTER_KEY_FILE", "")
	t.Setenv("TRADIS_PREVIOUS_MASTER_KEYS", "")
	if err := database.InitDB(filepath.Join(dataDir, "data.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := settings.InitSettingsTable(); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetDB().Exec(`INSERT OR REPLACE INTO global_settings(key, value) VALUES('advanced_mode', ?)`,
		map[bool]string{true: "true", false: "false"}[advanced]); err != nil {
		t.Fatal(err)
	}
	if err := secrets.ConfigurePersistentMasterKey(dataDir); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	RegisterComposeRoutes(router.Group("/api"))
	projectDir := filepath.Join(settings.GetProjectRoot(), "demo")
	return router, projectDir
}

func writeComposeHistoryProject(t *testing.T, projectDir, yamlContent, envContent string) {
	t.Helper()
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(envContent), 0600); err != nil {
		t.Fatal(err)
	}
}

func performComposeHistoryRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func requestComposePreview(t *testing.T, router http.Handler, yamlContent, envContent string) composehistory.Preview {
	t.Helper()
	response := performComposeHistoryRequest(t, router, http.MethodPost, "/api/compose/demo/config/preview", map[string]any{
		"yaml": yamlContent, "env": envContent,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", response.Code, response.Body.String())
	}
	var preview composehistory.Preview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	return preview
}

func applyComposeCandidate(t *testing.T, router http.Handler, yamlContent, envContent string) {
	t.Helper()
	preview := requestComposePreview(t, router, yamlContent, envContent)
	response := performComposeHistoryRequest(t, router, http.MethodPost, "/api/compose/demo/config/apply", map[string]any{
		"yaml": yamlContent, "env": envContent, "baseHash": preview.BaseHash,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", response.Code, response.Body.String())
	}
}

func assertComposeHistoryFile(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != want {
		t.Fatalf("%s = %q, want %q", path, content, want)
	}
}
