// Package registrycheck 实现镜像更新检测的"直连 registry 查询链"：
// 按优先级链对镜像 tag 做远程 digest 解析（HEAD /v2/{repo}/manifests/{tag}），
// 供 api 层 runImageUpdateCheck 的 check 路径使用；全链失败由调用方回退 daemon
// DistributionInspect 兜底。设计约束见仓库根 update.md ③。
package registrycheck

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dockerpanel/backend/pkg/logging"
)

const (
	mediaTypeManifestList = "application/vnd.docker.distribution.manifest.list.v2+json"
	mediaTypeOCIIndex     = "application/vnd.oci.image.index.v1+json"
	mediaTypeManifest     = "application/vnd.docker.distribution.manifest.v2+json"
	mediaTypeOCIManifest  = "application/vnd.oci.image.manifest.v1+json"
)

const (
	// SourceMirror 表示 digest 命中/选自 daemon registry-mirrors 中的镜像站来源。
	SourceMirror = "mirror"
	// SourceDirect 表示 digest 命中/选自目标 registry 本体直连来源。
	SourceDirect = "direct"
)

const (
	defaultHeadTimeout     = 5 * time.Second
	defaultTokenTimeout    = 5 * time.Second
	defaultChildrenTimeout = 8 * time.Second
	defaultTotalBudget     = 60 * time.Second

	// defaultDockerHubEndpoint 是 docker.io 的 v2 registry 端点（镜像引用里的
	// docker.io 是索引域名，v2 API 走 registry-1）。
	defaultDockerHubEndpoint = "https://registry-1.docker.io"
)

var (
	// headAccept 覆盖 manifest list / OCI index / 单 arch manifest 四种类型，
	// 与 pull-by-tag 落到 RepoDigests 的口径一致。
	headAccept = strings.Join([]string{mediaTypeManifestList, mediaTypeOCIIndex, mediaTypeManifest, mediaTypeOCIManifest}, ", ")
	// childrenAccept 只保留 list + index，用于展开子 manifest 做归属比对。
	childrenAccept = strings.Join([]string{mediaTypeManifestList, mediaTypeOCIIndex}, ", ")
)

// Credential 是 registry 凭证，由 api 层按已存 registry 记录匹配后注入
// （pkg 不直接依赖 database）。
type Credential struct {
	Username      string
	Password      string
	ServerAddress string
}

// Options 是一次解析运行的输入。
type Options struct {
	// Mirrors 是 daemon.json registry-mirrors 的原始条目（可带/不带 scheme），
	// 仅 docker.io 镜像使用；读取失败时传 nil 即可（按无镜像处理）。
	Mirrors []string
	// Credential 是目标 registry 的已存凭证（可 nil = 匿名优先）。
	Credential *Credential
	// LocalDigests 是本地镜像全部 RepoDigests 的 digest 值集合，用于 any-match 归属判定。
	LocalDigests []string
}

// Result 是一次解析运行的输出。
type Result struct {
	// Digest 是选中的远端 digest：已最新时为命中来源的 digest；有更新时为
	// 优先级最高来源的 digest（registry 本体 > mirror 顺序）。
	Digest string
	// UpToDate 为 true 表示任一来源 digest（含 manifest list 子项展开）命中本地集合。
	UpToDate bool
	// Source 是命中/选中来源（SourceMirror / SourceDirect），daemon 兜底来源由 api 层标记。
	Source string
	// SourceHost 是命中/选中来源的 registry host。
	SourceHost string
}

// Resolver 按优先级链解析镜像 tag 的远程 digest。零值可用，全部默认值通过
// NewResolver 或惰性兜底生效；字段可覆盖以便测试注入。
type Resolver struct {
	// HTTPClient 默认使用标准 DefaultTransport（Proxy=ProxyFromEnvironment，
	// 尊重面板容器 HTTP(S)_PROXY/NO_PROXY；不继承 daemon 代理）。
	HTTPClient *http.Client
	// DockerHubEndpoint 覆盖 docker.io 本体直连端点（默认 registry-1.docker.io），
	// 主要供测试注入。
	DockerHubEndpoint string
	HeadTimeout       time.Duration
	TokenTimeout      time.Duration
	ChildrenTimeout   time.Duration
	TotalBudget       time.Duration
}

// NewResolver 返回零值 Resolver（全部默认行为）。
func NewResolver() *Resolver {
	return &Resolver{}
}

func (r *Resolver) headTimeout() time.Duration {
	if r.HeadTimeout > 0 {
		return r.HeadTimeout
	}
	return defaultHeadTimeout
}

func (r *Resolver) tokenTimeout() time.Duration {
	if r.TokenTimeout > 0 {
		return r.TokenTimeout
	}
	return defaultTokenTimeout
}

func (r *Resolver) childrenTimeout() time.Duration {
	if r.ChildrenTimeout > 0 {
		return r.ChildrenTimeout
	}
	return defaultChildrenTimeout
}

func (r *Resolver) totalBudget() time.Duration {
	if r.TotalBudget > 0 {
		return r.TotalBudget
	}
	return defaultTotalBudget
}

func defaultHTTPClient() *http.Client {
	// &http.Client{} 的空 Transport 落到 http.DefaultTransport：
	// Proxy=ProxyFromEnvironment 等标准默认值。
	return &http.Client{}
}

// Resolve 执行优先级链：docker.io 镜像按 [mirrors...] → 本体顺序逐来源 HEAD，
// 任一来源 digest（含 index 子项展开）∈ LocalDigests 即返回已最新；全部来源拿到
// digest 但均不命中返回有更新（digest 取最高优先级来源）；全部来源失败返回 error。
// 引用名首段为 docker.io 别名（docker.io / index.docker.io / registry-1.docker.io）
// 时在入口归一为 docker.io 短名语义。非 docker.io registry 无 mirrors 步骤，
// 直连该 host + 已存凭证。
func (r *Resolver) Resolve(ctx context.Context, repoTag string, opts Options) (Result, error) {
	name, tag := splitRef(repoTag)
	name = normalizeDockerHubName(name)
	if name == "" {
		return Result{}, fmt.Errorf("invalid image reference %q", repoTag)
	}
	sources := r.buildSources(name, opts.Mirrors)
	if len(sources) == 0 {
		return Result{}, fmt.Errorf("no registry sources for %q", repoTag)
	}

	// perSource 按最坏路径计算：匿名 HEAD 失败后带 token 重试一次（2×headTimeout），
	// 加 token 换取与 index 子项展开。预算不足会截断链尾来源的合法慢响应。
	perSource := 2*r.headTimeout() + r.tokenTimeout() + r.childrenTimeout()
	budget := r.totalBudget()
	if perSourceBudget := time.Duration(len(sources)) * perSource; perSourceBudget < budget {
		budget = perSourceBudget
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	client := r.HTTPClient
	if client == nil {
		client = defaultHTTPClient()
	}

	local := make(map[string]struct{}, len(opts.LocalDigests))
	for _, d := range opts.LocalDigests {
		d = strings.TrimSpace(d)
		if d != "" {
			local[d] = struct{}{}
		}
	}

	run := &runState{tokens: map[string]string{}}
	var failures []string
	directCandidate := candidate{}
	mirrorCandidate := candidate{}

	for _, src := range sources {
		digest, reason := r.fetchDigest(ctx, client, src, tag, opts.Credential, run)
		if reason != "" {
			// 该来源失败：记录脱敏原因继续下一来源
			failures = append(failures, fmt.Sprintf("%s %s: %s", src.kind, src.host(), reason))
			logging.Debug("registry source failed", "image", repoTag, "source", src.kind, "host", src.host(), "reason", reason)
			continue
		}
		if _, ok := local[digest]; ok {
			logging.Debug("image remote digest matched local set", "image", repoTag, "source", src.kind, "host", src.host(), "digest", digest)
			return Result{Digest: digest, UpToDate: true, Source: src.kind, SourceHost: src.host()}, nil
		}
		// HEAD digest 未命中：GET index 展开子 manifest 做二次归属比对（Dockhand #1367）。
		// 子项解析失败保持"不符"结论，继续下一来源。
		if r.childrenMatch(ctx, client, src, tag, local, run) {
			logging.Debug("image remote index child digest matched local set", "image", repoTag, "source", src.kind, "host", src.host(), "digest", digest)
			return Result{Digest: digest, UpToDate: true, Source: src.kind, SourceHost: src.host()}, nil
		}
		cand := candidate{digest: digest, host: src.host()}
		if src.kind == SourceDirect {
			if directCandidate.digest == "" {
				directCandidate = cand
			}
		} else if mirrorCandidate.digest == "" {
			mirrorCandidate = cand
		}
	}

	// 汇总：所有拿到 digest 的来源均 ∉ 本地集合 → 有更新。
	// digest 优先级：registry 本体 > mirror 顺序（遍历顺序即优先级）。
	if directCandidate.digest != "" {
		logging.Debug("image remote digest not in local set", "image", repoTag, "source", SourceDirect, "host", directCandidate.host, "digest", directCandidate.digest, "failures", strings.Join(failures, "; "))
		return Result{Digest: directCandidate.digest, UpToDate: false, Source: SourceDirect, SourceHost: directCandidate.host}, nil
	}
	if mirrorCandidate.digest != "" {
		logging.Debug("image remote digest not in local set", "image", repoTag, "source", SourceMirror, "host", mirrorCandidate.host, "digest", mirrorCandidate.digest, "failures", strings.Join(failures, "; "))
		return Result{Digest: mirrorCandidate.digest, UpToDate: false, Source: SourceMirror, SourceHost: mirrorCandidate.host}, nil
	}
	return Result{}, fmt.Errorf("all %d registry sources failed: %s", len(sources), strings.Join(failures, "; "))
}

// candidate 记录一个拿到 digest 但未命中的来源（digest + host）。
type candidate struct {
	digest string
	host   string
}
