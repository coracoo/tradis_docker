//go:build community

package api

import (
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/settings"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCommunityNASReadsConfiguredCDNWithoutOfficialFallback(t *testing.T) {
	_ = database.Close()
	root := t.TempDir()
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
		case "/api/nas/list":
			_, _ = w.Write([]byte(`[{"id":"demo","name":"Demo","image_url":"https://official.example/api/nas/images/one","purchase_url":"https://official.example/api/nas/redirect/demo","official_url":"https://brand.example/device"}]`))
		case "/api/nas/categories":
			_, _ = w.Write([]byte(`[{"id":"home","name":"家庭"}]`))
		case "/api/nas/topics/demo":
			_, _ = w.Write([]byte(`{"Title":"Demo","Content":"# Guide"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer cdn.Close()
	t.Setenv("APPSTORE_CDN_URL", cdn.URL)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterCommunityRoutes(router, router.Group("/api"))

	for _, path := range []string{"/api/nas/list", "/api/nas/categories", "/api/nas/topics/demo"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, response.Code, response.Body.String())
		}
		if path == "/api/nas/list" && (strings.Contains(response.Body.String(), "/api/nas/redirect/") || strings.Contains(response.Body.String(), "/api/nas/images/")) {
			t.Fatalf("Community list exposed official Server URLs: %s", response.Body.String())
		}
		if path == "/api/nas/list" && !strings.Contains(response.Body.String(), `"official_url":"https://brand.example/device"`) {
			t.Fatalf("Community list dropped a direct public manufacturer link: %s", response.Body.String())
		}
	}
	if len(paths) != 3 {
		t.Fatalf("unexpected CDN requests: %v", paths)
	}
	for _, path := range []string{"/api/nas/device", "/api/nas/import"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("POST %s unexpectedly available: %d", path, response.Code)
		}
	}
}

func TestCommunityNASDoesNotFallbackWhenCDNFails(t *testing.T) {
	_ = database.Close()
	root := t.TempDir()
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
	RegisterCommunityRoutes(router, router.Group("/api"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/nas/list", nil))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("CDN failure must be explicit, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCommunityNASRemovesExternalImagesAndFullOnlyBannerLinks(t *testing.T) {
	_ = database.Close()
	root := t.TempDir()
	if err := database.InitDB(filepath.Join(root, "test.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := settings.InitSettingsTable(); err != nil {
		t.Fatal(err)
	}
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/nas/banners":
			_, _ = w.Write([]byte(`[{"Title":"NAS","ImageURL":"https://official.example/api/nas/images/banner","LinkURL":"/ai-agent"}]`))
		case "/api/nas/reviews/demo":
			_, _ = w.Write([]byte(`{"Title":"Review","CoverImage":"https://official.example/api/nas/images/cover"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer cdn.Close()
	t.Setenv("APPSTORE_CDN_URL", cdn.URL)
	router := gin.New()
	RegisterCommunityRoutes(router, router.Group("/api"))
	for _, path := range []string{"/api/nas/banners", "/api/nas/reviews/demo"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: %d", path, response.Code)
		}
		if strings.Contains(response.Body.String(), "official.example") || strings.Contains(response.Body.String(), "/ai-agent") {
			t.Fatalf("GET %s exposed Full-only link: %s", path, response.Body.String())
		}
	}
}
