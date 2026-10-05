package api

import (
	"context"
	"errors"
	"testing"

	"dockerpanel/backend/pkg/registrycheck"
)

// TestResolveRemoteImageDigestFallsBackToDaemon 验证全链失败时集成点回退 daemon
// 兜底（hook 注入断言被调用），且兜底成功时不报错、不标记已最新。
func TestResolveRemoteImageDigestFallsBackToDaemon(t *testing.T) {
	origCred, origFallback := imageRegistryCredential, daemonImageDigestFallback
	t.Cleanup(func() {
		imageRegistryCredential, daemonImageDigestFallback = origCred, origFallback
	})
	imageRegistryCredential = func(repoTag string) *registrycheck.Credential { return nil }

	called := 0
	daemonImageDigestFallback = func(repoTag string) (string, error) {
		called++
		return "sha256:daemonfallback", nil
	}

	// 127.0.0.1:1 必然连接拒绝：直连链全失败，必须走 daemon 兜底
	digest, upToDate, err := imageRemoteDigestResolve(context.Background(), "127.0.0.1:1/unreachable/app:1.0", nil, nil)
	if err != nil {
		t.Fatalf("expected daemon fallback to succeed, got error: %v", err)
	}
	if called == 0 {
		t.Fatal("daemon fallback hook was not invoked")
	}
	if digest != "sha256:daemonfallback" {
		t.Fatalf("digest = %q, want sha256:daemonfallback", digest)
	}
	if upToDate {
		t.Fatal("daemon fallback must not claim up-to-date")
	}
}

// TestResolveRemoteImageDigestErrorWhenFallbackFails 验证直连链与 daemon 兜底
// 都失败时返回 error（调用方走既有失败路径：退避表 + 不写库）。
func TestResolveRemoteImageDigestErrorWhenFallbackFails(t *testing.T) {
	origCred, origFallback := imageRegistryCredential, daemonImageDigestFallback
	t.Cleanup(func() {
		imageRegistryCredential, daemonImageDigestFallback = origCred, origFallback
	})
	imageRegistryCredential = func(repoTag string) *registrycheck.Credential { return nil }
	daemonImageDigestFallback = func(repoTag string) (string, error) {
		return "", errors.New("daemon inspect unavailable")
	}

	_, _, err := imageRemoteDigestResolve(context.Background(), "127.0.0.1:1/unreachable/app:1.0", nil, nil)
	if err == nil {
		t.Fatal("expected error when registry chain and daemon fallback both fail")
	}
}
