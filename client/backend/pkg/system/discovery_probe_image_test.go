package system

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeImage_FalsePositiveGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("404 Not Found"))
		case "/ok.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	if probeImage(srv.URL + "/favicon.ico") {
		t.Fatalf("expected false for fake icon body")
	}
	if !probeImage(srv.URL + "/ok.ico") {
		t.Fatalf("expected true for valid ico header")
	}
}

