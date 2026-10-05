package api

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dockerpanel/backend/pkg/database"
)

// TestRecordImageRemoteDigestFailureIfLive 验证 M4：运行 ctx 存活时记录失败并写
// 退避；ctx 已取消（前端断连/run 正在拆除）时跳过失败记录与退避写，避免断连毒化
// 退避表把镜像标记成 unavailable。
func TestRecordImageRemoteDigestFailureIfLive(t *testing.T) {
	_ = database.Close()
	if err := database.InitDB(filepath.Join(t.TempDir(), "image-remote-digest-failure.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	resolveErr := errors.New("registry 直连检查失败(context deadline exceeded)，daemon 兜底检查失败")

	t.Run("live ctx records failure and backoff", func(t *testing.T) {
		const tag = "registry.example.com/team/app:1.0"
		recordImageRemoteDigestFailureIfLive(context.Background(), tag, resolveErr)

		st, err := database.GetImageRemoteDigestStatus(tag)
		if err != nil {
			t.Fatalf("GetImageRemoteDigestStatus: %v", err)
		}
		if st.FailCount != 1 {
			t.Fatalf("FailCount = %d, want 1", st.FailCount)
		}
		if st.NextCheckAt == "" {
			t.Fatal("expected next_check_at backoff to be written")
		}
		if st.Unavailable {
			t.Fatal("single failure must not mark unavailable")
		}
	})

	t.Run("canceled ctx skips failure record and backoff", func(t *testing.T) {
		const tag = "nginx:latest"
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		recordImageRemoteDigestFailureIfLive(ctx, tag, resolveErr)

		st, err := database.GetImageRemoteDigestStatus(tag)
		if err != nil {
			t.Fatalf("GetImageRemoteDigestStatus: %v", err)
		}
		if st.FailCount != 0 || st.Unavailable || st.NextCheckAt != "" {
			t.Fatalf("canceled ctx must not write backoff, got %+v", st)
		}
	})

	t.Run("canceled ctx after several live failures keeps existing count", func(t *testing.T) {
		const tag = "redis:7"
		for i := 0; i < 2; i++ {
			recordImageRemoteDigestFailureIfLive(context.Background(), tag, resolveErr)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		recordImageRemoteDigestFailureIfLive(ctx, tag, resolveErr)

		st, err := database.GetImageRemoteDigestStatus(tag)
		if err != nil {
			t.Fatalf("GetImageRemoteDigestStatus: %v", err)
		}
		if st.FailCount != 2 {
			t.Fatalf("FailCount = %d, want 2 (canceled run must not bump the count)", st.FailCount)
		}
	})
}

// TestMatchRegistryDeterministic 验证 M5：strip scheme 后 hostname 精确匹配优先；
// 无命中按 "." 边界后缀匹配（notdocker.io 不误配 docker.io；docker.io 别名仍可命中
// docker.io 已存凭证）；多条命中按 registry URL 字典序取第一个（结果确定）。
func TestMatchRegistryDeterministic(t *testing.T) {
	reg := func(url, user string) *database.Registry {
		return &database.Registry{Name: user, URL: url, Username: user}
	}

	t.Run("exact hostname match strips scheme", func(t *testing.T) {
		want := reg("https://registry.example.com", "exact-user")
		regs := map[string]*database.Registry{
			"https://registry.example.com": want,
		}
		if got := matchRegistry(regs, "registry.example.com"); got != want {
			t.Fatalf("matchRegistry = %+v, want %+v", got, want)
		}
	})

	t.Run("exact match keeps port", func(t *testing.T) {
		want := reg("registry.example.com:5000", "port-user")
		regs := map[string]*database.Registry{
			"http://registry.example.com:5000": want,
		}
		if got := matchRegistry(regs, "registry.example.com:5000"); got != want {
			t.Fatalf("matchRegistry = %+v, want %+v", got, want)
		}
	})

	t.Run("exact match wins over suffix candidate", func(t *testing.T) {
		exact := reg("https://index.docker.io", "index-user")
		hub := reg("docker.io", "hub-user")
		regs := map[string]*database.Registry{
			"docker.io":               hub,
			"https://index.docker.io": exact,
		}
		if got := matchRegistry(regs, "index.docker.io"); got != exact {
			t.Fatalf("matchRegistry = %+v, want exact index.docker.io entry", got)
		}
	})

	t.Run("notdocker.io must not match docker.io", func(t *testing.T) {
		hub := reg("docker.io", "hub-user")
		regs := map[string]*database.Registry{"docker.io": hub}
		if got := matchRegistry(regs, "notdocker.io"); got != nil {
			t.Fatalf("notdocker.io matched %+v, want nil", got)
		}
		if got := matchRegistry(regs, "mydocker.io"); got != nil {
			t.Fatalf("mydocker.io matched %+v, want nil", got)
		}
		if got := matchRegistry(regs, "docker.io.evil.example.com"); got != nil {
			t.Fatalf("docker.io.evil.example.com matched %+v, want nil", got)
		}
	})

	t.Run("docker.io must not pick up notdocker.io credentials", func(t *testing.T) {
		hub := reg("docker.io", "hub-user")
		notDocker := reg("https://notdocker.io", "not-user")
		regs := map[string]*database.Registry{
			"docker.io":            hub,
			"https://notdocker.io": notDocker,
		}
		for i := 0; i < 20; i++ {
			if got := matchRegistry(regs, "docker.io"); got != hub {
				t.Fatalf("iteration %d: docker.io matched %+v, want docker.io entry (deterministic)", i, got)
			}
		}
	})

	t.Run("docker.io aliases hit docker.io credentials", func(t *testing.T) {
		hub := reg("https://docker.io", "hub-user")
		regs := map[string]*database.Registry{"https://docker.io": hub}
		for _, host := range []string{"docker.io", "index.docker.io", "registry-1.docker.io"} {
			if got := matchRegistry(regs, host); got != hub {
				t.Fatalf("host %q matched %+v, want docker.io entry", host, got)
			}
		}
	})

	t.Run("docker.io host hits credentials stored under any alias", func(t *testing.T) {
		// docker login 经典地址是 index.docker.io：任意别名方向保存的凭证都必须命中。
		index := reg("https://index.docker.io/v1/", "index-user")
		regs := map[string]*database.Registry{"https://index.docker.io/v1/": index}
		for _, host := range []string{"docker.io", "index.docker.io", "registry-1.docker.io"} {
			if got := matchRegistry(regs, host); got != index {
				t.Fatalf("host %q matched %+v, want index.docker.io entry", host, got)
			}
		}
	})

	t.Run("multiple suffix candidates pick first by URL lexical order", func(t *testing.T) {
		bReg := reg("https://b.example.com", "b-user")
		aReg := reg("https://a.example.com", "a-user")
		regs := map[string]*database.Registry{
			"https://b.example.com": bReg,
			"https://a.example.com": aReg,
			"https://example.com":   reg("https://example.com", "root-user"),
		}
		// host 同时是 a.example.com 和 example.com 的后缀子域：必须稳定取
		// 字典序第一个（https://a.example.com < https://example.com）。
		for i := 0; i < 20; i++ {
			if got := matchRegistry(regs, "x.a.example.com"); got != aReg {
				t.Fatalf("iteration %d: matchRegistry = %+v, want a.example.com entry (deterministic)", i, got)
			}
		}
		if got := matchRegistry(regs, "y.b.example.com"); got != bReg {
			t.Fatalf("matchRegistry = %+v, want b.example.com entry", got)
		}
	})

	t.Run("nil and empty inputs", func(t *testing.T) {
		if got := matchRegistry(nil, "docker.io"); got != nil {
			t.Fatalf("nil regs matched %+v, want nil", got)
		}
		hub := reg("docker.io", "hub-user")
		if got := matchRegistry(map[string]*database.Registry{"docker.io": hub}, ""); got != nil {
			t.Fatalf("empty host matched %+v, want nil", got)
		}
	})
}

// TestRunImageUpdateCheckSingletonGuard 验证单飞保护：已有实例在运行时，第二次
// 调用立即返回"已在运行"错误（不触碰 docker/daemon），消除调度器与手动/计划
// 任务并发跑两份全量检测的问题。
func TestRunImageUpdateCheckSingletonGuard(t *testing.T) {
	imageUpdateCheckInFlight.Lock()
	defer imageUpdateCheckInFlight.Unlock()

	done := make(chan error, 1)
	go func() {
		_, err := runImageUpdateCheck(context.Background(), false)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, errImageUpdateCheckInFlight) {
			t.Fatalf("err = %v, want errImageUpdateCheckInFlight", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second runImageUpdateCheck call was not rejected promptly (singleton guard missing?)")
	}
}

// TestNormalizeImageVariantsDigestRef 验证 L1：repo:tag@sha256 写法先按 @ 分离
// 再解析，产出裸 repo:tag 变体，让 compose 更新后的记录清理能命中。
func TestNormalizeImageVariantsDigestRef(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "pinned short name produces bare tag variant",
			raw:  "nginx:1.25@sha256:" + strings.Repeat("a", 64),
			want: []string{
				"nginx:1.25@sha256:" + strings.Repeat("a", 64),
				"nginx:1.25",
				"library/nginx:1.25",
				"docker.io/nginx:1.25",
				"docker.io/library/nginx:1.25",
			},
		},
		{
			name: "pinned name without tag defaults to latest",
			raw:  "nginx@sha256:" + strings.Repeat("b", 64),
			want: []string{
				"nginx@sha256:" + strings.Repeat("b", 64),
				"nginx:latest",
				"library/nginx:latest",
				"docker.io/nginx:latest",
				"docker.io/library/nginx:latest",
			},
		},
		{
			name: "pinned custom registry ref keeps repo path",
			raw:  "registry.example.com/team/app:1.0@sha256:" + strings.Repeat("c", 64),
			want: []string{
				"registry.example.com/team/app:1.0@sha256:" + strings.Repeat("c", 64),
				"registry.example.com/team/app:1.0",
			},
		},
		{
			name: "plain ref without digest keeps existing variants",
			raw:  "nginx:1.25",
			want: []string{
				"nginx:1.25",
				"library/nginx:1.25",
				"docker.io/nginx:1.25",
				"docker.io/library/nginx:1.25",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeImageVariants(tc.raw)
			set := make(map[string]struct{}, len(got))
			for _, v := range got {
				set[v] = struct{}{}
			}
			for _, want := range tc.want {
				if _, ok := set[want]; !ok {
					t.Fatalf("normalizeImageVariants(%q) = %v, missing %q", tc.raw, got, want)
				}
			}
			if tc.name == "plain ref without digest keeps existing variants" && len(got) != len(tc.want) {
				t.Fatalf("normalizeImageVariants(%q) = %v, want exactly %v", tc.raw, got, tc.want)
			}
		})
	}

	if got := normalizeImageVariants(""); got != nil {
		t.Fatalf("normalizeImageVariants(\"\") = %v, want nil", got)
	}
}
