//go:build community

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dockerpanel/backend/pkg/settings"
	"github.com/gin-gonic/gin"
)

func TestCommunityAppStoreSourcesExcludeOfficialDeploymentCounts(t *testing.T) {
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
		if !strings.HasPrefix(file, "appstore") {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(listed.Dir, file))
		if readErr != nil {
			t.Fatalf("read compiled source %s: %v", file, readErr)
		}
		for _, forbidden := range []string{
			"fetchOfficialDeploymentCounts",
			"submitAppStoreDeploymentCount",
			"official-deployment-counts.json",
			"appStoreDeploymentCountsEnabled",
		} {
			if strings.Contains(string(content), forbidden) {
				t.Fatalf("community AppStore source %s unexpectedly contains %q", file, forbidden)
			}
		}
	}
}

func TestCommunityAppStoreSourcesExcludeOfficialReadFallback(t *testing.T) {
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
		if !strings.HasPrefix(file, "appstore") {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(listed.Dir, file))
		if readErr != nil {
			t.Fatalf("read compiled source %s: %v", file, readErr)
		}
		for _, forbidden := range []string{
			"doAppStoreOfficialRequest",
			"getAppStoreReadFallbackURL",
			"getAppStoreOriginURL",
			"fetchOriginAppListMeta",
			"officialAppStoreServerURL",
			"writeOfficialAppCache",
		} {
			if strings.Contains(string(content), forbidden) {
				t.Fatalf("community AppStore source %s unexpectedly contains %q", file, forbidden)
			}
		}
	}
}

func TestCommunityAppStoreDoesNotRegisterDeploymentCountRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAppStoreRoutes(router)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/appstore/deploy_counts", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("community deployment-count route status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestCommunityAppStoreNeverFallsBackToOfficialOrigin(t *testing.T) {
	originalSettings := settingsGetSettings
	originalCacheDir := getAppCacheDirFunc
	settingsGetSettings = func() (settings.Settings, error) { return settings.Settings{}, nil }
	getAppCacheDirFunc = func() string { return filepath.Join(t.TempDir(), "apps") }
	appListCacheMu.Lock()
	appListCache = appListCacheState{}
	appListCacheMu.Unlock()
	t.Cleanup(func() {
		settingsGetSettings = originalSettings
		getAppCacheDirFunc = originalCacheDir
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAppStoreRoutes(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/appstore/apps?refresh=1", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("community AppStore status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestCommunityAppStoreReadsConfiguredCDNWithoutOfficialFallback(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/templates" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode([]map[string]any{{
			"id":          42,
			"name":        "community-demo",
			"category":    "demo",
			"description": "served by a configured CDN",
			"version":     "1.0.0",
		}})
	}))
	defer cdn.Close()

	originalSettings := settingsGetSettings
	originalCacheDir := getAppCacheDirFunc
	settingsGetSettings = func() (settings.Settings, error) {
		return settings.Settings{AppStoreCDNURL: cdn.URL}, nil
	}
	getAppCacheDirFunc = func() string { return filepath.Join(t.TempDir(), "apps") }
	appListCacheMu.Lock()
	appListCache = appListCacheState{}
	appListCacheMu.Unlock()
	t.Cleanup(func() {
		settingsGetSettings = originalSettings
		getAppCacheDirFunc = originalCacheDir
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAppStoreRoutes(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/appstore/apps?refresh=1", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("community AppStore status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var apps []App
	if err := json.Unmarshal(recorder.Body.Bytes(), &apps); err != nil {
		t.Fatalf("decode AppStore response: %v", err)
	}
	if len(apps) != 1 || apps[0].Name != "community-demo" {
		t.Fatalf("community AppStore response = %#v, want configured CDN template", apps)
	}
	if apps[0].ShowDeployCount || apps[0].DeploymentCount != 0 {
		t.Fatalf("community AppStore must not expose official deployment counts: %#v", apps[0])
	}
}

func TestCommunityAppStoreDetailStripsTutorialAndWritesLocalCache(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/templates/42" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id":          42,
			"name":        "community-detail",
			"description": "served by a configured CDN",
			"tutorial":    "https://example.test/private-tutorial",
			"compose":     "services:\n  app:\n    image: nginx:alpine\n",
		})
	}))
	defer cdn.Close()

	originalSettings := settingsGetSettings
	originalCacheDir := getAppCacheDirFunc
	settingsGetSettings = func() (settings.Settings, error) {
		return settings.Settings{AppStoreCDNURL: cdn.URL}, nil
	}
	cacheDir := filepath.Join(t.TempDir(), "apps")
	getAppCacheDirFunc = func() string { return cacheDir }
	appDetailCacheMu.Lock()
	originalDetailCache := appDetailCache
	appDetailCache = make(map[string]appDetailCacheEntry)
	appDetailCacheMu.Unlock()
	t.Cleanup(func() {
		settingsGetSettings = originalSettings
		getAppCacheDirFunc = originalCacheDir
		appDetailCacheMu.Lock()
		appDetailCache = originalDetailCache
		appDetailCacheMu.Unlock()
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAppStoreRoutes(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/appstore/apps/42", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("community AppStore detail status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response App
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode AppStore detail response: %v", err)
	}
	if response.Tutorial != "" {
		t.Fatalf("community AppStore detail retained tutorial %q", response.Tutorial)
	}

	content, err := os.ReadFile(filepath.Join(cacheDir, "community-detail.json"))
	if err != nil {
		t.Fatalf("read local AppStore detail cache: %v", err)
	}
	var cached App
	if err := json.Unmarshal(content, &cached); err != nil {
		t.Fatalf("decode local AppStore detail cache: %v", err)
	}
	if cached.Tutorial != "" {
		t.Fatalf("community AppStore cache retained tutorial %q", cached.Tutorial)
	}
}

func TestCommunityAppStoreServesDiskCacheWithoutCDN(t *testing.T) {
	originalSettings := settingsGetSettings
	originalCacheDir := getAppCacheDirFunc
	settingsGetSettings = func() (settings.Settings, error) { return settings.Settings{}, nil }
	cacheDir := filepath.Join(t.TempDir(), "apps")
	getAppCacheDirFunc = func() string { return cacheDir }
	appListCacheMu.Lock()
	originalListCache := appListCache
	appListCache = appListCacheState{}
	appListCacheMu.Unlock()
	t.Cleanup(func() {
		settingsGetSettings = originalSettings
		getAppCacheDirFunc = originalCacheDir
		appListCacheMu.Lock()
		appListCache = originalListCache
		appListCacheMu.Unlock()
	})

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("create cache directory: %v", err)
	}
	payload, err := json.Marshal(appListDiskCache{
		Apps:        []App{{ID: 42, Name: "community-cache", Version: "1.0.0"}},
		VersionHash: "sha256:cached",
		FetchedAt:   time.Now().Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("encode list cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "app-list-cache.json"), payload, 0644); err != nil {
		t.Fatalf("write list cache: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAppStoreRoutes(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/appstore/apps", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("community AppStore cache status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var apps []App
	if err := json.Unmarshal(recorder.Body.Bytes(), &apps); err != nil {
		t.Fatalf("decode cached AppStore response: %v", err)
	}
	if len(apps) != 1 || apps[0].Name != "community-cache" {
		t.Fatalf("community AppStore cache response = %#v", apps)
	}
}
