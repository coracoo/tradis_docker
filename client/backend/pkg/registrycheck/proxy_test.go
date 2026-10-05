package registrycheck_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"dockerpanel/backend/pkg/registrycheck"
)

// nonLoopbackListener 在本机非回环地址上监听。Go 的 httpproxy 对所有回环目标
// （127.0.0.1/::1/localhost）一律绕过代理，因此"请求经过代理"用例的目标
// registry 必须绑非回环地址；无可用地址时跳过。
func nonLoopbackListener(t *testing.T) net.Listener {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skipf("cannot enumerate interface addresses: %v", err)
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.IsLinkLocalUnicast() || ipnet.IP.IsLinkLocalMulticast() {
			continue
		}
		ln, err := net.Listen("tcp", ipnet.IP.String()+":0")
		if err == nil {
			return ln
		}
	}
	t.Skip("no usable non-loopback address available")
	return nil
}

// TestResolveUsesHTTPProxyFromEnvironment 断言默认 Resolver（生产配置）的请求经过
// HTTP_PROXY 指向的本地代理。用 t.Setenv 注入代理环境；其余用例均使用无代理
// client，不会提前触发标准库 ProxyFromEnvironment 的环境快照，保证本用例独立。
func TestResolveUsesHTTPProxyFromEnvironment(t *testing.T) {
	var targetHits int32
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt32(&targetHits, 1)
		w.Header().Set("Docker-Content-Digest", validDigest("proxied"))
		w.WriteHeader(http.StatusOK)
	}))
	target.Listener = nonLoopbackListener(t)
	target.Start()
	defer target.Close()

	// 真实正向代理：按 absolute-form URL 直连目标并回拷响应
	direct := &http.Transport{}
	var proxyHits int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt32(&proxyHits, 1)
		outReq := req.Clone(req.Context())
		outReq.RequestURI = ""
		resp, err := direct.RoundTrip(outReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer proxy.Close()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")

	r := registrycheck.NewResolver() // 默认 client：DefaultTransport + ProxyFromEnvironment
	r.DockerHubEndpoint = target.URL
	res, err := r.Resolve(context.Background(), "nginx:latest", registrycheck.Options{
		LocalDigests: []string{validDigest("proxied")},
	})
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if !res.UpToDate {
		t.Fatal("expected up-to-date result")
	}
	if atomic.LoadInt32(&proxyHits) == 0 {
		t.Fatal("request did not go through the configured HTTP proxy")
	}
	if atomic.LoadInt32(&targetHits) == 0 {
		t.Fatal("proxied request never reached the target registry")
	}
}
