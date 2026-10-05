package registrycheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"dockerpanel/backend/pkg/registrycheck"
)

// TestResolveDockerIOPrefixedRefs 验证 H1：containerd 存储下 RepoTag 首段带
// docker.io / index.docker.io / registry-1.docker.io 前缀时，归一为 docker.io
// 短名语义——走 mirrors 优先链 + registry-1.docker.io 本体，repo 路径补 library/。
func TestResolveDockerIOPrefixedRefs(t *testing.T) {
	t.Run("docker.io prefix uses mirrors first then registry-1 direct", func(t *testing.T) {
		mirrorRec := &recorder{}
		mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			mirrorRec.add(req)
			if req.URL.Path != "/v2/library/nginx/manifests/latest" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Docker-Content-Digest", validDigest("mirror-hit"))
			w.WriteHeader(http.StatusOK)
		}))
		defer mirror.Close()

		directRec := &recorder{}
		direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			directRec.add(req)
			w.Header().Set("Docker-Content-Digest", validDigest("direct"))
			w.WriteHeader(http.StatusOK)
		}))
		defer direct.Close()

		r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: direct.URL}
		res, err := r.Resolve(context.Background(), "docker.io/nginx:latest", registrycheck.Options{
			Mirrors:      []string{mirror.URL},
			LocalDigests: []string{validDigest("mirror-hit")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if !res.UpToDate || res.Digest != validDigest("mirror-hit") {
			t.Fatalf("unexpected result: %+v", res)
		}
		if res.Source != registrycheck.SourceMirror {
			t.Fatalf("Source = %q, want mirror", res.Source)
		}
		// registry-1 本体（注入端点）在 mirror 命中后不得被查询
		if directRec.count() != 0 {
			t.Fatalf("direct source must not be queried after mirror hit, saw %d requests", directRec.count())
		}
	})

	t.Run("index.docker.io library ref hits registry-1 with library repo", func(t *testing.T) {
		directRec := &recorder{}
		direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			directRec.add(req)
			if req.URL.Path != "/v2/library/nginx/manifests/1.25" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Docker-Content-Digest", validDigest("hub-1.25"))
			w.WriteHeader(http.StatusOK)
		}))
		defer direct.Close()

		r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: direct.URL}
		res, err := r.Resolve(context.Background(), "index.docker.io/library/nginx:1.25", registrycheck.Options{
			LocalDigests: []string{validDigest("hub-1.25")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if !res.UpToDate || res.Digest != validDigest("hub-1.25") {
			t.Fatalf("unexpected result: %+v", res)
		}
		if directRec.count() == 0 {
			t.Fatal("registry-1 direct source was not queried")
		}
	})

	t.Run("registry-1.docker.io team repo keeps repo path", func(t *testing.T) {
		directRec := &recorder{}
		direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			directRec.add(req)
			if req.URL.Path != "/v2/team/app/manifests/1.0" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Docker-Content-Digest", validDigest("team-app"))
			w.WriteHeader(http.StatusOK)
		}))
		defer direct.Close()

		r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: direct.URL}
		res, err := r.Resolve(context.Background(), "registry-1.docker.io/team/app:1.0", registrycheck.Options{
			LocalDigests: []string{validDigest("team-app")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if !res.UpToDate || res.Digest != validDigest("team-app") {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("docker.io prefix with mirror failure falls back to direct", func(t *testing.T) {
		mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer mirror.Close()

		directRec := &recorder{}
		direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			directRec.add(req)
			w.Header().Set("Docker-Content-Digest", validDigest("direct-fallback"))
			w.WriteHeader(http.StatusOK)
		}))
		defer direct.Close()

		r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: direct.URL}
		res, err := r.Resolve(context.Background(), "docker.io/nginx:latest", registrycheck.Options{
			Mirrors:      []string{mirror.URL},
			LocalDigests: []string{validDigest("direct-fallback")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if !res.UpToDate || res.Source != registrycheck.SourceDirect {
			t.Fatalf("unexpected result: %+v", res)
		}
		if directRec.count() == 0 {
			t.Fatal("direct source was not used as fallback after mirror failure")
		}
	})
}

// TestResolveRejectsMalformedDigestHeader 验证 M2：伪造/异常 Docker-Content-Digest
// 头（形状不合法）视为该来源失败并走下一来源，不污染判定；大小写兼容。
func TestResolveRejectsMalformedDigestHeader(t *testing.T) {
	t.Run("malformed mirror header falls through to registry-1 direct", func(t *testing.T) {
		bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Docker-Content-Digest", "sha256:XYZ")
			w.WriteHeader(http.StatusOK)
		}))
		defer bad.Close()

		directRec := &recorder{}
		direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			directRec.add(req)
			w.Header().Set("Docker-Content-Digest", validDigest("direct-ok"))
			w.WriteHeader(http.StatusOK)
		}))
		defer direct.Close()

		r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: direct.URL}
		res, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
			Mirrors:      []string{bad.URL},
			LocalDigests: []string{validDigest("direct-ok")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if !res.UpToDate || res.Digest != validDigest("direct-ok") {
			t.Fatalf("unexpected result: %+v", res)
		}
		if res.Source != registrycheck.SourceDirect {
			t.Fatalf("Source = %q, want direct", res.Source)
		}
	})

	t.Run("all sources malformed yields aggregated failure", func(t *testing.T) {
		malformed := []string{"not-a-digest", "sha256:XYZ", ""}
		for _, header := range malformed {
			bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if header != "" {
					w.Header().Set("Docker-Content-Digest", header)
				}
				w.WriteHeader(http.StatusOK)
			}))
			reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if header != "" {
					w.Header().Set("Docker-Content-Digest", header)
				}
				w.WriteHeader(http.StatusOK)
			}))

			r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: reg.URL}
			_, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
				Mirrors: []string{bad.URL},
			})
			if err == nil {
				t.Fatalf("header %q: expected aggregated source failure", header)
			}
			if !strings.Contains(err.Error(), "invalid Docker-Content-Digest header") &&
				!strings.Contains(err.Error(), "missing Docker-Content-Digest header") {
				t.Fatalf("header %q: error = %q, want digest header validation failure", header, err.Error())
			}
			bad.Close()
			reg.Close()
		}
	})

	t.Run("uppercase hex digest is accepted", func(t *testing.T) {
		upper := "sha256:" + strings.ToUpper(validDigest("upper")[7:])
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Docker-Content-Digest", upper)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		r := &registrycheck.Resolver{HTTPClient: directClient(), DockerHubEndpoint: srv.URL}
		res, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
			LocalDigests: []string{upper},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if !res.UpToDate || res.Digest != upper {
			t.Fatalf("unexpected result: %+v", res)
		}
	})
}

// TestResolveBudgetCoversWorstCasePerSource 验证 M3：单来源最坏路径为
// 2×headTimeout + tokenTimeout + childrenTimeout（匿名 HEAD 401 后带 token 重试
// 一次 + index 子项展开）。预算按最坏路径计算时链尾来源的合法慢响应不被截断。
func TestResolveBudgetCoversWorstCasePerSource(t *testing.T) {
	t.Run("single direct source full token flow fits worst-case budget", func(t *testing.T) {
		// Head=600ms / Token=300ms / Children=300ms：修正前预算 1200ms 会在
		// 认证 HEAD 期间截断（540+270+540=1350>1200）；修正后 1800ms 覆盖 1620ms。
		const headSleep = 540 * time.Millisecond
		const shortSleep = 270 * time.Millisecond

		var serverURL atomic.Value
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			switch {
			case req.URL.Path == "/token":
				time.Sleep(shortSleep)
				_ = json.NewEncoder(w).Encode(map[string]string{"token": "slow-tok"})
			case req.Method == http.MethodGet:
				time.Sleep(shortSleep)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"manifests": []map[string]any{{"digest": validDigest("slow-child")}},
				})
			default:
				time.Sleep(headSleep)
				if req.Header.Get("Authorization") != "Bearer slow-tok" {
					realm, _ := serverURL.Load().(string)
					w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="slow.example",scope="repository:team/app:pull"`, realm))
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Docker-Content-Digest", validDigest("slow-head"))
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer srv.Close()
		serverURL.Store(srv.URL)

		r := &registrycheck.Resolver{
			HTTPClient:      directClient(),
			HeadTimeout:     600 * time.Millisecond,
			TokenTimeout:    300 * time.Millisecond,
			ChildrenTimeout: 300 * time.Millisecond,
		}
		res, err := r.Resolve(context.Background(), hostOfServer(t, srv)+"/team/app:1.0", registrycheck.Options{
			LocalDigests: []string{validDigest("slow-child")},
		})
		if err != nil {
			t.Fatalf("Resolve error (tail source killed by underestimated budget?): %v", err)
		}
		if !res.UpToDate {
			t.Fatal("expected up-to-date via children attribution within worst-case budget")
		}
		if res.Digest != validDigest("slow-head") {
			t.Fatalf("Digest = %q, want %q", res.Digest, validDigest("slow-head"))
		}
	})

	t.Run("multi-source chain tail survives with worst-case budget", func(t *testing.T) {
		// mirror 慢成功（head+children ≈ 560ms）后，direct 走完整 401→token→head
		// →children 最坏路径；总耗时 ≈ 560+280×3=1400ms，必须在预算内完成。
		mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Method == http.MethodGet {
				time.Sleep(280 * time.Millisecond)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"manifests": []map[string]any{{"digest": validDigest("mirror-child")}},
				})
				return
			}
			time.Sleep(280 * time.Millisecond)
			w.Header().Set("Docker-Content-Digest", validDigest("mirror-head"))
			w.WriteHeader(http.StatusOK)
		}))
		defer mirror.Close()

		var serverURL atomic.Value
		direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			switch {
			case req.URL.Path == "/token":
				time.Sleep(280 * time.Millisecond)
				_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
			case req.Method == http.MethodGet:
				time.Sleep(280 * time.Millisecond)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"manifests": []map[string]any{{"digest": validDigest("direct-child")}},
				})
			default:
				time.Sleep(280 * time.Millisecond)
				if req.Header.Get("Authorization") != "Bearer tok" {
					realm, _ := serverURL.Load().(string)
					w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="hub.example",scope="repository:library/nginx:pull"`, realm))
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Docker-Content-Digest", validDigest("direct-head"))
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer direct.Close()
		serverURL.Store(direct.URL)

		r := &registrycheck.Resolver{
			HTTPClient:        directClient(),
			DockerHubEndpoint: direct.URL,
			HeadTimeout:       300 * time.Millisecond,
			TokenTimeout:      300 * time.Millisecond,
			ChildrenTimeout:   300 * time.Millisecond,
		}
		res, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
			Mirrors:      []string{mirror.URL},
			LocalDigests: []string{validDigest("direct-child")},
		})
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if !res.UpToDate || res.Digest != validDigest("direct-head") {
			t.Fatalf("unexpected result: %+v", res)
		}
		if res.Source != registrycheck.SourceDirect {
			t.Fatalf("Source = %q, want direct", res.Source)
		}
	})
}
