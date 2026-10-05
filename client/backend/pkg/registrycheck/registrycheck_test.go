package registrycheck_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dockerpanel/backend/pkg/registrycheck"
)

// validDigest 生成形状合法的测试 digest（algo:hex64）：M2 起远端 digest 头必须通过
// 格式校验，旧的 validDigest("hub") 式短串会被视为来源失败。
func validDigest(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// directClient 返回不走任何环境代理的 HTTP client，保证用例不受
// 运行环境 HTTP(S)_PROXY 影响（代理行为由 proxy_test.go 单独覆盖）。
func directClient() *http.Client {
	return &http.Client{Transport: &http.Transport{}}
}

type recordedReq struct {
	method string
	path   string
	accept string
	auth   string
}

type recorder struct {
	mu   sync.Mutex
	reqs []recordedReq
}

func (r *recorder) add(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, recordedReq{
		method: req.Method,
		path:   req.URL.Path,
		accept: req.Header.Get("Accept"),
		auth:   req.Header.Get("Authorization"),
	})
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func (r *recorder) all() []recordedReq {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedReq, len(r.reqs))
	copy(out, r.reqs)
	return out
}

// digestHandler 返回一个总是以指定 digest 响应 HEAD/GET manifest 请求的服务器。
func digestHandler(rec *recorder, digest string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		rec.add(req)
		w.Header().Set("Docker-Content-Digest", digest)
		w.WriteHeader(http.StatusOK)
	}
}

func hostOfServer(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestResolveMirrorChainFallsThrough(t *testing.T) {
	t.Run("first mirror 500 then second mirror 200", func(t *testing.T) {
		m1Rec := &recorder{}
		m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			m1Rec.add(req)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer m1.Close()

		m2Rec := &recorder{}
		m2 := httptest.NewServer(digestHandler(m2Rec, validDigest("mirror2")))
		defer m2.Close()

		r := &registrycheck.Resolver{HTTPClient: directClient()}
		res, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
			Mirrors:      []string{m1.URL, m2.URL},
			LocalDigests: []string{validDigest("mirror2")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if !res.UpToDate {
			t.Fatal("expected up-to-date result")
		}
		if res.Digest != validDigest("mirror2") {
			t.Fatalf("Digest = %q, want sha256:mirror2", res.Digest)
		}
		if res.Source != registrycheck.SourceMirror {
			t.Fatalf("Source = %q, want mirror", res.Source)
		}
		if res.SourceHost != hostOfServer(t, m2) {
			t.Fatalf("SourceHost = %q, want %q", res.SourceHost, hostOfServer(t, m2))
		}
		if m2Rec.count() != 1 {
			t.Fatalf("second mirror saw %d requests, want 1", m2Rec.count())
		}
	})

	t.Run("first mirror timeout then second mirror 200", func(t *testing.T) {
		m1Rec := &recorder{}
		m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			m1Rec.add(req)
			time.Sleep(300 * time.Millisecond)
			w.Header().Set("Docker-Content-Digest", validDigest("tooLate"))
			w.WriteHeader(http.StatusOK)
		}))
		defer m1.Close()

		m2Rec := &recorder{}
		m2 := httptest.NewServer(digestHandler(m2Rec, validDigest("mirror2")))
		defer m2.Close()

		r := &registrycheck.Resolver{HTTPClient: directClient(), HeadTimeout: 50 * time.Millisecond}
		res, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
			Mirrors:      []string{m1.URL, m2.URL},
			LocalDigests: []string{validDigest("mirror2")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if !res.UpToDate || res.Digest != validDigest("mirror2") {
			t.Fatalf("unexpected result: %+v", res)
		}
		if res.Source != registrycheck.SourceMirror {
			t.Fatalf("Source = %q, want mirror", res.Source)
		}
	})
}

func TestResolveDockerHubTokenFlow(t *testing.T) {
	authRec := &recorder{}
	var authHits int32
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt32(&authHits, 1)
		authRec.add(req)
		if req.URL.Path != "/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// 匿名 token：不应带 Basic 凭证
		if req.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := req.URL.Query().Get("service"); got != "registry.docker.io" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if got := req.URL.Query().Get("scope"); got != "repository:library/nginx:pull" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok123"})
	}))
	defer auth.Close()

	regRec := &recorder{}
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		regRec.add(req)
		if req.URL.Path != "/v2/library/nginx/manifests/latest" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if req.Header.Get("Authorization") != "Bearer tok123" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="registry.docker.io",scope="repository:library/nginx:pull"`, auth.URL))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", validDigest("hub"))
		w.WriteHeader(http.StatusOK)
	}))
	defer registry.Close()

	r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: registry.URL}
	res, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
		LocalDigests: []string{validDigest("hub")},
	})
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if !res.UpToDate {
		t.Fatal("expected up-to-date result")
	}
	if res.Digest != validDigest("hub") {
		t.Fatalf("Digest = %q, want sha256:hub", res.Digest)
	}
	if res.Source != registrycheck.SourceDirect {
		t.Fatalf("Source = %q, want direct", res.Source)
	}
	if res.SourceHost != hostOfServer(t, registry) {
		t.Fatalf("SourceHost = %q, want %q", res.SourceHost, hostOfServer(t, registry))
	}
	if atomic.LoadInt32(&authHits) != 1 {
		t.Fatalf("token endpoint hit %d times, want 1 (token should be cached)", authHits)
	}
	reqs := regRec.all()
	if len(reqs) != 2 {
		t.Fatalf("registry saw %d requests, want 2 (401 challenge + authorized head)", len(reqs))
	}
	if strings.Count(reqs[0].accept, "application/") != 4 {
		t.Fatalf("HEAD Accept header = %q, want 4 media types", reqs[0].accept)
	}
}

func TestResolveIndexChildrenAttribution(t *testing.T) {
	newServer := func(t *testing.T, failChildren bool) (*httptest.Server, *recorder) {
		rec := &recorder{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			rec.add(req)
			if req.Method != http.MethodGet {
				w.Header().Set("Docker-Content-Digest", validDigest("index"))
				w.WriteHeader(http.StatusOK)
				return
			}
			if failChildren {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			body := map[string]any{
				"schemaVersion": 2,
				"mediaType":     "application/vnd.docker.distribution.manifest.list.v2+json",
				"manifests": []map[string]any{
					{"digest": validDigest("child-amd64"), "platform": map[string]string{"os": "linux", "architecture": "amd64"}},
					{"digest": validDigest("child-arm64"), "platform": map[string]string{"os": "linux", "architecture": "arm64"}},
				},
			}
			_ = json.NewEncoder(w).Encode(body)
		}))
		t.Cleanup(srv.Close)
		return srv, rec
	}

	t.Run("child digest in local set is up to date", func(t *testing.T) {
		srv, rec := newServer(t, false)
		r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: srv.URL}
		res, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
			LocalDigests: []string{validDigest("child-arm64")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if !res.UpToDate {
			t.Fatal("expected up-to-date via child digest attribution")
		}
		if res.Digest != validDigest("index") {
			t.Fatalf("Digest = %q, want sha256:index", res.Digest)
		}
		// GET（index 子项展开）的 Accept 只含 list + index 两种；HEAD 的 Accept 含 4 种
		for _, q := range rec.all() {
			if q.method != http.MethodGet {
				continue
			}
			wantAccept := "application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.index.v1+json"
			if q.accept != wantAccept {
				t.Fatalf("children GET Accept header = %q, want %q", q.accept, wantAccept)
			}
		}
	})

	t.Run("children not in local set means update available", func(t *testing.T) {
		srv, _ := newServer(t, false)
		r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: srv.URL}
		res, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
			LocalDigests: []string{validDigest("local-old")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if res.UpToDate {
			t.Fatal("expected update-available result")
		}
		if res.Digest != validDigest("index") {
			t.Fatalf("Digest = %q, want sha256:index", res.Digest)
		}
	})

	t.Run("children fetch failure keeps mismatch conclusion", func(t *testing.T) {
		srv, _ := newServer(t, true)
		r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: srv.URL}
		res, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
			LocalDigests: []string{validDigest("child-amd64")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if res.UpToDate {
			t.Fatal("children GET 500 must keep mismatch conclusion")
		}
		if res.Digest != validDigest("index") {
			t.Fatalf("Digest = %q, want sha256:index", res.Digest)
		}
	})
}

func TestResolvePrivateRepoSkipsMirror(t *testing.T) {
	mirrorRec := &recorder{}
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mirrorRec.add(req)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer mirror.Close()

	var sawBasic atomic.Value
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		user, pass, ok := req.BasicAuth()
		if !ok || user != "alice" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		sawBasic.Store(true)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "priv-token"})
	}))
	defer auth.Close()

	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer priv-token" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="registry.docker.io",scope="repository:team/app:pull"`, auth.URL))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", validDigest("privapp"))
		w.WriteHeader(http.StatusOK)
	}))
	defer registry.Close()

	r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: registry.URL}
	res, err := r.Resolve(context.Background(), "team/app:1.0", registrycheck.Options{
		Mirrors:      []string{mirror.URL},
		Credential:   &registrycheck.Credential{Username: "alice", Password: "secret"},
		LocalDigests: []string{validDigest("privapp")},
	})
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if !res.UpToDate || res.Digest != validDigest("privapp") {
		t.Fatalf("unexpected result: %+v", res)
	}
	// 镜像站 401 应直接跳过（不做 token 尝试），只收到最初的 HEAD
	if got := mirrorRec.count(); got != 1 {
		t.Fatalf("mirror saw %d requests, want exactly 1 (private repo must skip mirror)", got)
	}
	if sawBasic.Load() != true {
		t.Fatal("token endpoint did not receive expected Basic credentials")
	}
}

func TestResolveNonDockerHubDirectWithChallenge(t *testing.T) {
	mirrorRec := &recorder{}
	unusedMirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mirrorRec.add(req)
		w.Header().Set("Docker-Content-Digest", validDigest("wrong"))
		w.WriteHeader(http.StatusOK)
	}))
	defer unusedMirror.Close()

	var serverURL atomic.Value
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/token" {
			user, pass, ok := req.BasicAuth()
			if !ok || user != "robot" || pass != "pass" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "gh-token"})
			return
		}
		if req.URL.Path != "/v2/team/app/manifests/2.0" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if req.Header.Get("Authorization") != "Bearer gh-token" {
			realm, _ := serverURL.Load().(string)
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="ghcr.example",scope="repository:team/app:pull"`, realm))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", validDigest("ghcr"))
		w.WriteHeader(http.StatusOK)
	}))
	defer registry.Close()
	serverURL.Store(registry.URL)

	host := hostOfServer(t, registry)
	r := &registrycheck.Resolver{HTTPClient: directClient()}
	res, err := r.Resolve(context.Background(), host+"/team/app:2.0", registrycheck.Options{
		Mirrors:      []string{unusedMirror.URL},
		Credential:   &registrycheck.Credential{Username: "robot", Password: "pass"},
		LocalDigests: []string{validDigest("ghcr")},
	})
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if !res.UpToDate || res.Digest != validDigest("ghcr") {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Source != registrycheck.SourceDirect {
		t.Fatalf("Source = %q, want direct", res.Source)
	}
	if got := mirrorRec.count(); got != 0 {
		t.Fatalf("non-docker.io registry must not use mirrors, mirror saw %d requests", got)
	}
}

func TestResolveAllSourcesFail(t *testing.T) {
	m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer m1.Close()
	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer reg.Close()

	r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: reg.URL}
	_, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
		Mirrors: []string{m1.URL},
	})
	if err == nil {
		t.Fatal("expected error when all sources fail")
	}
	if !strings.Contains(err.Error(), "registry sources failed") {
		t.Fatalf("error = %q, want aggregated source failure", err.Error())
	}
}

func TestResolveSummaryPriority(t *testing.T) {
	t.Run("direct digest wins over mirror digests", func(t *testing.T) {
		m1 := httptest.NewServer(digestHandler(&recorder{}, validDigest("m1")))
		defer m1.Close()
		m2 := httptest.NewServer(digestHandler(&recorder{}, validDigest("m2")))
		defer m2.Close()
		reg := httptest.NewServer(digestHandler(&recorder{}, validDigest("direct")))
		defer reg.Close()

		r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: reg.URL}
		res, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
			Mirrors:      []string{m1.URL, m2.URL},
			LocalDigests: []string{validDigest("unrelated")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if res.UpToDate {
			t.Fatal("expected update-available result")
		}
		if res.Digest != validDigest("direct") {
			t.Fatalf("Digest = %q, want sha256:direct (direct has priority)", res.Digest)
		}
		if res.Source != registrycheck.SourceDirect {
			t.Fatalf("Source = %q, want direct", res.Source)
		}
	})

	t.Run("first mirror wins when direct fails", func(t *testing.T) {
		reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer reg.Close()
		m1 := httptest.NewServer(digestHandler(&recorder{}, validDigest("m1first")))
		defer m1.Close()
		m2 := httptest.NewServer(digestHandler(&recorder{}, validDigest("m2second")))
		defer m2.Close()

		r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: reg.URL}
		res, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
			Mirrors:      []string{m1.URL, m2.URL},
			LocalDigests: []string{validDigest("unrelated")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if res.UpToDate {
			t.Fatal("expected update-available result")
		}
		if res.Digest != validDigest("m1first") {
			t.Fatalf("Digest = %q, want sha256:m1first (mirror order)", res.Digest)
		}
		if res.Source != registrycheck.SourceMirror {
			t.Fatalf("Source = %q, want mirror", res.Source)
		}
	})

	t.Run("mirror hit short-circuits before direct", func(t *testing.T) {
		regRec := &recorder{}
		reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			regRec.add(req)
			w.Header().Set("Docker-Content-Digest", validDigest("never"))
			w.WriteHeader(http.StatusOK)
		}))
		defer reg.Close()
		m1 := httptest.NewServer(digestHandler(&recorder{}, validDigest("hit")))
		defer m1.Close()

		r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: reg.URL}
		res, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
			Mirrors:      []string{m1.URL},
			LocalDigests: []string{validDigest("hit")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if !res.UpToDate || res.Source != registrycheck.SourceMirror {
			t.Fatalf("unexpected result: %+v", res)
		}
		if regRec.count() != 0 {
			t.Fatalf("direct source must not be queried after mirror hit, saw %d requests", regRec.count())
		}
	})
}
