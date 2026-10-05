package system

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeAndResolveNavigationIcon_Remote404Fallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<!doctype html><html><head><link rel="icon" href="/real.ico"></head><body>ok</body></html>`))
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			w.WriteHeader(http.StatusNotFound)
		case "/real.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	got := normalizeAndResolveNavigationIcon(srv.URL+"/favicon.ico", srv.URL, "")
	if got != srv.URL+"/real.ico" {
		t.Fatalf("unexpected icon: %s", got)
	}
}

func TestNormalizeAndResolveNavigationIcon_EmptyUsesWebsiteFavicon(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10})
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<!doctype html><html><body>ok</body></html>`))
		}
	}))
	t.Cleanup(srv.Close)

	got := normalizeAndResolveNavigationIcon("", srv.URL, "")
	if got != srv.URL+"/favicon.ico" {
		t.Fatalf("unexpected icon: %s", got)
	}
}

func TestExtractFaviconHref_AllowsHrefBeforeRel(t *testing.T) {
	body := []byte(`<!doctype html><html><head><link href="/assets/icon.png" rel="apple-touch-icon"></head></html>`)

	got := extractFaviconHref(body)
	if got != "/assets/icon.png" {
		t.Fatalf("unexpected href: %s", got)
	}
}

func TestResolveFaviconIcon_JSONDefaultAndSVGLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("null"))
		case "/favicon.png", "/favicon.svg":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<!doctype html><html><head><link rel="icon" type="image/svg+xml" href="/static/logo.svg"></head></html>`))
		case "/", "/login":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<!doctype html><html><head><link rel="icon" type="image/svg+xml" href="/static/logo.svg"></head></html>`))
		case "/static/logo.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0h10v10z"/></svg>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	got := resolveFaviconIcon(srv.URL+"/login", "")
	if got != srv.URL+"/static/logo.svg" {
		t.Fatalf("unexpected icon: %s", got)
	}
}

func TestResolveFaviconIcon_HTMLAndConventionalPathsRunInParallel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico", "/favicon.png", "/favicon.svg":
			time.Sleep(600 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("null"))
		case "/", "/login":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<link rel="icon" href="/static/logo.svg">`))
		case "/static/logo.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	start := time.Now()
	got := resolveFaviconIcon(srv.URL+"/login", "")
	if got != srv.URL+"/static/logo.svg" {
		t.Fatalf("unexpected icon: %s", got)
	}
	if elapsed := time.Since(start); elapsed >= 400*time.Millisecond {
		t.Fatalf("favicon resolution was serialized: %s", elapsed)
	}
}

func TestIsGenericNavigationIcon(t *testing.T) {
	for _, icon := range []string{"", "mdi-docker", "mdi:docker", "container", "box"} {
		if !isGenericNavigationIcon(icon) {
			t.Fatalf("expected %q to be generic", icon)
		}
	}
	if isGenericNavigationIcon("lucide:rss") {
		t.Fatal("specific icon must not be treated as generic")
	}
}

func TestCacheNavigationIcon_PersistsRasterIcon(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(png)
	}))
	t.Cleanup(srv.Close)

	got := cacheNavigationIcon(srv.URL+"/favicon.png", "compose:demo")
	if got == "" || got == srv.URL+"/favicon.png" {
		t.Fatalf("expected local cached icon path, got %q", got)
	}
	if filepath.Ext(got) != ".png" {
		t.Fatalf("expected png path, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "data", "pic", filepath.Base(got))); err != nil {
		t.Fatalf("expected cached icon file: %v", err)
	}
}

func TestCachedNavigationIconExtension_RejectsSVG(t *testing.T) {
	if got := cachedNavigationIconExtension("image/svg+xml", "https://example.com/icon.svg", []byte("<svg/>")); got != "" {
		t.Fatalf("expected SVG caching to be rejected, got %q", got)
	}
}

func TestCacheNavigationIcon_FallsBackWhenRemoteSVGCannotBeCached(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0h10v10z"/></svg>`))
	}))
	t.Cleanup(srv.Close)

	if got := cacheNavigationIcon(srv.URL+"/vite.svg", "container:test"); got != "container" {
		t.Fatalf("expected safe generic fallback, got %q", got)
	}
}

func TestDockerHostFallbackURL_RewritesLoopbackOnly(t *testing.T) {
	got := dockerHostFallbackURL("http://localhost:38080/static/favicon.png")
	if got != "http://host.docker.internal:38080/static/favicon.png" {
		t.Fatalf("unexpected fallback URL: %q", got)
	}
	if got := dockerHostFallbackURL("https://example.com/favicon.png"); got != "" {
		t.Fatalf("public URL must not be rewritten: %q", got)
	}
}
