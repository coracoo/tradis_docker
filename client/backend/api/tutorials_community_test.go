//go:build community

package api

import (
	"context"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/settings"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCommunityTutorialsReadPublicRSSFromConfiguredCDN(t *testing.T) {
	_ = database.Close()
	root := t.TempDir()
	t.Setenv("TRADIS_DATA_DIR", root)
	if err := database.InitDB(filepath.Join(root, "test.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := settings.InitSettingsTable(); err != nil {
		t.Fatal(err)
	}
	var paths []string
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/api/community/tutorials/feed.xml" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/"><channel><title>Public tutorials</title><item><guid>guide-one</guid><title>First guide</title><link>https://example.com/guide</link><description>Summary</description><content:encoded><![CDATA[<h2>Install</h2><p>Start here</p>]]></content:encoded></item></channel></rss>`))
	}))
	defer cdn.Close()
	t.Setenv("APPSTORE_CDN_URL", cdn.URL)
	configured, err := settings.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	configured.TutorialRSSURL = cdn.URL + "/api/community/tutorials/feed.xml"
	if err := settings.UpdateSettings(configured); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterCommunityTutorialRoutes(router.Group("/api"))
	manifestResponse := httptest.NewRecorder()
	router.ServeHTTP(manifestResponse, httptest.NewRequest(http.MethodGet, "/api/tutorials/manifest", nil))
	if manifestResponse.Code != http.StatusOK || !strings.Contains(manifestResponse.Body.String(), "First guide") {
		t.Fatalf("RSS manifest = %d: %s", manifestResponse.Code, manifestResponse.Body.String())
	}
	var manifest communityTutorialManifest
	if err := json.Unmarshal(manifestResponse.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Articles) != 1 {
		t.Fatalf("RSS manifest articles = %d", len(manifest.Articles))
	}
	articleResponse := httptest.NewRecorder()
	router.ServeHTTP(articleResponse, httptest.NewRequest(http.MethodGet, "/api/tutorials/"+manifest.Articles[0].Slug, nil))
	if articleResponse.Code != http.StatusOK || !strings.Contains(articleResponse.Body.String(), "Start here") {
		t.Fatalf("RSS article = %d: %s", articleResponse.Code, articleResponse.Body.String())
	}
	if len(paths) != 1 || paths[0] != "/api/community/tutorials/feed.xml" {
		t.Fatalf("RSS must be cached, got requests %v", paths)
	}
}

func TestCommunityTutorialRSSSourceMustBePublicMirror(t *testing.T) {
	base := "https://tradis-templates.coracoo.deno.net"
	for _, source := range []string{
		"http://127.0.0.1/feed.xml",
		"https://private.example.com/feed.xml",
		base + "/api/private/feed.xml",
		"https://raw.githubusercontent.com/another/repo/main/tutorials/community/feed.xml",
	} {
		if communityTutorialRSSURLAllowed(source, base) {
			t.Errorf("accepted non-public RSS source %q", source)
		}
	}
	for _, source := range []string{
		base + "/api/community/tutorials/feed.xml",
		"https://raw.githubusercontent.com/coracoo/tradis_templates/main/tutorials/community/feed.xml",
	} {
		if !communityTutorialRSSURLAllowed(source, base) {
			t.Errorf("rejected public RSS source %q", source)
		}
	}
}

func TestCommunityTutorialRSSRejectsRedirectOutsidePublicDirectory(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/community/tutorials/feed.xml" {
			http.Redirect(w, r, "/api/private/feed.xml", http.StatusFound)
			return
		}
		t.Errorf("followed redirect outside public tutorials: %s", r.URL.Path)
	}))
	defer cdn.Close()
	_, err := fetchCommunityTutorialRSS(context.Background(), cdn.URL+"/api/community/tutorials/feed.xml", cdn.URL)
	if err == nil || !strings.Contains(err.Error(), "public tutorial directory") {
		t.Fatalf("outside redirect error = %v", err)
	}
}

func TestCommunityTutorialsReadOnlyFromPublicCDN(t *testing.T) {
	_ = database.Close()
	root := t.TempDir()
	t.Setenv("TRADIS_DATA_DIR", root)
	if err := database.InitDB(filepath.Join(root, "test.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := settings.InitSettingsTable(); err != nil {
		t.Fatal(err)
	}
	var paths []string
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/community/tutorials/manifest":
			_, _ = w.Write([]byte(`{"format_version":1,"articles":[{"slug":"intro","title":"入门"}],"total_count":1}`))
		case "/api/community/tutorials/articles/intro":
			_, _ = w.Write([]byte(`{"slug":"intro","title":"入门","content":"# Hello"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer cdn.Close()
	t.Setenv("APPSTORE_CDN_URL", cdn.URL)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterCommunityRoutes(router, router.Group("/api"))
	for _, path := range []string{"/api/tutorials/manifest", "/api/tutorials/intro"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, response.Code, response.Body.String())
		}
	}
	if len(paths) != 2 || paths[0] != "/api/community/tutorials/manifest" || paths[1] != "/api/community/tutorials/articles/intro" {
		t.Fatalf("unexpected CDN paths: %v", paths)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/tutorials/intro/summary", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("Community summary route must be absent: %d", response.Code)
	}
}

func TestCommunityTutorialsRejectUnlistedArticle(t *testing.T) {
	_ = database.Close()
	root := t.TempDir()
	t.Setenv("TRADIS_DATA_DIR", root)
	if err := database.InitDB(filepath.Join(root, "test.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := settings.InitSettingsTable(); err != nil {
		t.Fatal(err)
	}
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/community/tutorials/manifest" {
			t.Errorf("unexpected CDN request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"format_version":1,"articles":[{"slug":"intro","title":"入门"}]}`))
	}))
	defer cdn.Close()
	t.Setenv("APPSTORE_CDN_URL", cdn.URL)
	router := gin.New()
	RegisterCommunityTutorialRoutes(router.Group("/api"))
	for _, path := range []string{"/api/tutorials/other", "/api/tutorials/%2e%2e"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code == http.StatusOK {
			t.Fatalf("GET %s unexpectedly succeeded: %s", path, response.Body.String())
		}
	}
}

func TestCommunityTutorialsDoNotFallbackToOfficialServer(t *testing.T) {
	_ = database.Close()
	root := t.TempDir()
	t.Setenv("TRADIS_DATA_DIR", root)
	if err := database.InitDB(filepath.Join(root, "test.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := settings.InitSettingsTable(); err != nil {
		t.Fatal(err)
	}
	cdn := httptest.NewServer(http.NotFoundHandler())
	defer cdn.Close()
	t.Setenv("APPSTORE_CDN_URL", cdn.URL)
	router := gin.New()
	RegisterCommunityTutorialRoutes(router.Group("/api"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/tutorials/manifest", nil))
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "公开教程") {
		t.Fatalf("unexpected missing public catalog response: %d %s", response.Code, response.Body.String())
	}
}
