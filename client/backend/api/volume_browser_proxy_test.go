package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestVolumeBrowseProxy_StaticAssetNoDup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Session ID 必须是24字符的十六进制字符串
	sid := "a1b2c3d4e5f6a1b2c3d4e5f6"

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, `<!doctype html><html><head><script>window.FileBrowser={"AuthMethod":"noauth","BaseURL":"","StaticURL":"/static","LogoutPage":"/login","NoAuth":true};</script><link rel="stylesheet" href="/static/assets/index.css"></head><body><script src="/static/assets/index.js"></script></body></html>`)
		case "/static/assets/index.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			io.WriteString(w, `console.log("ok")`)
		case "/static/assets/index.css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			io.WriteString(w, `body{background:#fff}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(up.Close)

	u, err := url.Parse(up.URL)
	if err != nil {
		t.Fatal(err)
	}

	volumeBrowseMu.Lock()
	volumeBrowseSessions[sid] = &volumeBrowseSession{ID: sid, TargetHost: u.Host}
	volumeBrowseMu.Unlock()
	t.Cleanup(func() {
		volumeBrowseMu.Lock()
		delete(volumeBrowseSessions, sid)
		volumeBrowseMu.Unlock()
	})

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("username", "admin")
		c.Next()
	})
	r.GET("/api/volumes/browse/:sid/fb/*path", volumeBrowseProxy)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	resp1, err := http.Get(srv.URL + "/api/volumes/browse/" + sid + "/fb/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp1.Body.Close()
	body1, _ := io.ReadAll(resp1.Body)
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", resp1.StatusCode, string(body1))
	}
	if !strings.Contains(strings.ToLower(resp1.Header.Get("Content-Type")), "text/html") {
		t.Fatalf("unexpected content-type: %s", resp1.Header.Get("Content-Type"))
	}
	if strings.Contains(string(body1), "/fb/api/volumes/browse/") {
		t.Fatalf("unexpected duplicated path in html: %s", string(body1))
	}
	if !strings.Contains(string(body1), `"BaseURL":"/api/volumes/browse/`+sid+`/fb"`) {
		t.Fatalf("expected BaseURL to be rewritten, got: %s", string(body1))
	}
	if !strings.Contains(string(body1), `"StaticURL":"/api/volumes/browse/`+sid+`/fb/static"`) {
		t.Fatalf("expected StaticURL to be rewritten, got: %s", string(body1))
	}

	resp2, err := http.Get(srv.URL + "/api/volumes/browse/" + sid + "/fb/static/assets/index.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", resp2.StatusCode, string(body2))
	}
	if !strings.Contains(strings.ToLower(resp2.Header.Get("Content-Type")), "application/javascript") {
		t.Fatalf("unexpected content-type: %s", resp2.Header.Get("Content-Type"))
	}
	if !strings.Contains(string(body2), `console.log("ok")`) {
		t.Fatalf("unexpected js body: %s", string(body2))
	}

	resp3, err := http.Get(srv.URL + "/api/volumes/browse/" + sid + "/fb/static/assets/index.css")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp3.Body)
		t.Fatalf("unexpected status: %d body=%s", resp3.StatusCode, string(b))
	}
	if ct := strings.ToLower(resp3.Header.Get("Content-Type")); !strings.Contains(ct, "text/css") {
		t.Fatalf("unexpected content-type: %s", resp3.Header.Get("Content-Type"))
	}
}
