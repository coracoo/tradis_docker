package registrycheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// source 是一个候选查询来源（镜像站或 registry 本体直连）。
type source struct {
	kind string // SourceMirror / SourceDirect
	base string // scheme://host（含端口），mirror 条目保留原 scheme
	repo string // /v2/ 之后的仓库路径
}

func (s source) host() string {
	if u, err := url.Parse(s.base); err == nil && u.Host != "" {
		return u.Host
	}
	return s.base
}

func (s source) manifestURL(tag string) string {
	return s.base + "/v2/" + s.repo + "/manifests/" + url.PathEscape(tag)
}

// runState 是一次 Resolve 运行内的共享状态（token 按 registry 缓存）。
type runState struct {
	tokens map[string]string // source.base → Bearer token
}

// dockerHubAliasFamily 是 docker.io 的索引/后端域名别名族。containerd 存储下
// docker.io 镜像的 RepoTag 首段常带这些前缀，必须归一为 docker.io 短名语义，
// 否则会被当成独立 registry：跳过 mirrors、直连 docker.io（302→HTML）导致
// 整条查询链静默失效。
var dockerHubAliasFamily = []string{"docker.io", "index.docker.io", "registry-1.docker.io"}

func isDockerHubAliasHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, alias := range dockerHubAliasFamily {
		if host == alias {
			return true
		}
	}
	return false
}

// EquivalentHosts 返回与 host 等价的 registry hostname 集合：host 属于 docker.io
// 别名族时返回全族（任意别名下保存的凭证互相命中），否则返回仅 host 自身。
// 供 api 层做凭证匹配时消除别名方向差异。
func EquivalentHosts(host string) []string {
	if isDockerHubAliasHost(host) {
		out := make([]string, len(dockerHubAliasFamily))
		copy(out, dockerHubAliasFamily)
		return out
	}
	return []string{strings.ToLower(strings.TrimSpace(host))}
}

// normalizeDockerHubName 在 pkg 入口归一镜像引用名：首段是 docker.io 别名时
// 剥掉前缀，剩余部分按 docker.io 仓库路径处理（单段名由 dockerHubRepoPath 补
// library/）。只影响传给 Resolve 的引用，不影响调用方的 DB 记录 key。
func normalizeDockerHubName(name string) string {
	segs := strings.SplitN(name, "/", 2)
	if len(segs) != 2 {
		return name
	}
	if isDockerHubAliasHost(segs[0]) {
		return segs[1]
	}
	return name
}

// buildSources 组装候选来源：docker.io 镜像为 [mirrors...（顺序）] + [本体直连]；
// 非 docker.io registry 只有直连来源，无 mirrors 步骤。
func (r *Resolver) buildSources(name string, mirrors []string) []source {
	host := hostOfName(name)
	if host == "" {
		// docker.io 系：repo 路径对齐 normalizeImageVariants 口径补 library/ 前缀
		repo := dockerHubRepoPath(name)
		out := make([]source, 0, len(mirrors)+1)
		seen := map[string]struct{}{}
		for _, m := range mirrors {
			base := normalizeMirrorBase(m)
			if base == "" {
				continue
			}
			if _, dup := seen[base]; dup {
				continue
			}
			seen[base] = struct{}{}
			out = append(out, source{kind: SourceMirror, base: base, repo: repo})
		}
		direct := strings.TrimRight(strings.TrimSpace(r.DockerHubEndpoint), "/")
		if direct == "" {
			direct = defaultDockerHubEndpoint
		}
		out = append(out, source{kind: SourceDirect, base: direct, repo: repo})
		return out
	}
	return []source{{kind: SourceDirect, base: defaultBaseForHost(host), repo: strings.TrimPrefix(name, host+"/")}}
}

// splitRef 拆镜像引用为名和 tag；无 tag 或空 tag 归一为 latest（口径同 api.parseImageName）。
func splitRef(repoTag string) (name, tag string) {
	repoTag = strings.TrimSpace(repoTag)
	if repoTag == "" {
		return "", ""
	}
	lastSlash := strings.LastIndex(repoTag, "/")
	lastColon := strings.LastIndex(repoTag, ":")
	if lastColon <= lastSlash {
		return repoTag, "latest"
	}
	name, tag = repoTag[:lastColon], repoTag[lastColon+1:]
	if tag == "" {
		tag = "latest"
	}
	return name, tag
}

// hostOfName 判断仓库名第一段的 host 口径（同 api.imageHost）：无 . / : 且非
// localhost 的第一段不是 host，视为 docker.io 短名。
func hostOfName(name string) string {
	segments := strings.Split(name, "/")
	if len(segments) == 0 {
		return ""
	}
	host := segments[0]
	if strings.Contains(host, ".") || strings.Contains(host, ":") || host == "localhost" {
		return host
	}
	return ""
}

// dockerHubRepoPath 对齐 normalizeImageVariants 的 docker.io 口径：单段名补
// library/。docker.io 别名前缀已在 Resolve 入口归一剥掉。
func dockerHubRepoPath(name string) string {
	if !strings.Contains(name, "/") {
		return "library/" + name
	}
	return name
}

// normalizeMirrorBase 归一 daemon registry-mirrors 条目：去空白和尾部斜杠；
// 无 scheme 时按 host 性质补默认 scheme。
func normalizeMirrorBase(mirror string) string {
	m := strings.TrimRight(strings.TrimSpace(mirror), "/")
	if m == "" {
		return ""
	}
	if !strings.HasPrefix(m, "http://") && !strings.HasPrefix(m, "https://") {
		m = defaultBaseForHost(m)
	}
	return m
}

// defaultBaseForHost 为 registry host 选择默认 scheme：回环/本机地址用 http（本地
// registry 常见部署），其余 https。
func defaultBaseForHost(host string) string {
	if isLoopbackHost(host) {
		return "http://" + host
	}
	return "https://" + host
}

func isLoopbackHost(host string) bool {
	h := host
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		h = parsed
	}
	h = strings.Trim(h, "[]")
	return h == "localhost" || h == "::1" || strings.HasPrefix(h, "127.")
}

// digestHeaderRe 校验 Docker-Content-Digest 头的形状（algo:hex，大小写兼容）。
// 伪造或异常的头不合法时视为该来源失败，继续下一来源，避免被脏数据污染判定。
var digestHeaderRe = regexp.MustCompile(`(?i)^[a-z0-9_+.-]+:[0-9a-f]{64,}$`)

func validDigestHeader(digest string) bool {
	return digestHeaderRe.MatchString(strings.TrimSpace(digest))
}

// fetchDigest 对单个来源执行 HEAD 取 Docker-Content-Digest；401 时解析
// WWW-Authenticate Bearer challenge 取 token 后带 Bearer 重试（匿名优先，
// 有凭证则 Basic 换 token）。返回 digest；失败时返回脱敏原因摘要。
// mirrors 上 401 视为私有仓库，跳过该 mirror（私有不走镜像站）。
func (r *Resolver) fetchDigest(ctx context.Context, client *http.Client, src source, tag string, cred *Credential, run *runState) (string, string) {
	digest, status, challenge, err := r.headOnce(ctx, client, src, tag, "")
	if err != nil {
		return "", "head request failed: " + sanitizeNetErr(err)
	}
	if status == http.StatusOK {
		if !validDigestHeader(digest) {
			if digest == "" {
				return "", "missing Docker-Content-Digest header"
			}
			return "", "invalid Docker-Content-Digest header"
		}
		return digest, ""
	}
	if status != http.StatusUnauthorized {
		return "", fmt.Sprintf("head returned status %d", status)
	}
	if src.kind == SourceMirror {
		return "", "mirror requires auth (401), private repositories skip mirrors"
	}
	token, reason := r.bearerToken(ctx, client, src, challenge, cred, run)
	if reason != "" {
		return "", reason
	}
	digest, status, _, err = r.headOnce(ctx, client, src, tag, token)
	if err != nil {
		return "", "authenticated head request failed: " + sanitizeNetErr(err)
	}
	if status == http.StatusOK {
		if !validDigestHeader(digest) {
			if digest == "" {
				return "", "missing Docker-Content-Digest header"
			}
			return "", "invalid Docker-Content-Digest header"
		}
		return digest, ""
	}
	if status == http.StatusUnauthorized {
		return "", "bearer token rejected (401)"
	}
	return "", fmt.Sprintf("authenticated head returned status %d", status)
}

// headOnce 执行一次 HEAD manifest 请求，返回 digest 头、状态码和 WWW-Authenticate。
func (r *Resolver) headOnce(ctx context.Context, client *http.Client, src source, tag, bearer string) (digest string, status int, challenge string, err error) {
	reqCtx, cancel := context.WithTimeout(ctx, r.headTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodHead, src.manifestURL(tag), nil)
	if err != nil {
		return "", 0, "", err
	}
	req.Header.Set("Accept", headAccept)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
	return strings.TrimSpace(resp.Header.Get("Docker-Content-Digest")), resp.StatusCode, resp.Header.Get("WWW-Authenticate"), nil
}

// challengeParamRe 解析 WWW-Authenticate Bearer challenge 的键值对。
var challengeParamRe = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9_-]*)="([^"]*)"`)

// parseBearerChallenge 解析 Bearer realm/service/scope。
func parseBearerChallenge(v string) (realm, service, scope string, err error) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(strings.ToLower(v), "bearer ") {
		return "", "", "", fmt.Errorf("not a bearer challenge")
	}
	params := map[string]string{}
	for _, m := range challengeParamRe.FindAllStringSubmatch(v, -1) {
		params[strings.ToLower(m[1])] = m[2]
	}
	realm = params["realm"]
	if realm == "" {
		return "", "", "", fmt.Errorf("bearer challenge missing realm")
	}
	return realm, params["service"], params["scope"], nil
}

// bearerToken 按 challenge 取 Bearer token（公开仓库匿名 token；有凭证时 Basic 换
// token），按 registry（source.base）在本次运行内缓存。
func (r *Resolver) bearerToken(ctx context.Context, client *http.Client, src source, challenge string, cred *Credential, run *runState) (string, string) {
	if token, ok := run.tokens[src.base]; ok {
		return token, ""
	}
	realm, service, scope, err := parseBearerChallenge(challenge)
	if err != nil {
		return "", "unparseable WWW-Authenticate challenge"
	}
	q := url.Values{}
	q.Set("service", service)
	if scope != "" {
		q.Set("scope", scope)
	}
	tokenURL := realm + "?" + q.Encode()
	tokenCtx, cancel := context.WithTimeout(ctx, r.tokenTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(tokenCtx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", "token request build failed"
	}
	if cred != nil && (cred.Username != "" || cred.Password != "") {
		req.SetBasicAuth(cred.Username, cred.Password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "token request failed: " + sanitizeNetErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Sprintf("token endpoint returned status %d", resp.StatusCode)
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", "token response decode failed"
	}
	token := body.Token
	if token == "" {
		token = body.AccessToken
	}
	if token == "" {
		return "", "token endpoint returned empty token"
	}
	run.tokens[src.base] = token
	return token, ""
}

// childrenMatch GET 同一 manifest URL（Accept 只含 list+index），展开 index 子项
// digest 与本地集合比对。任何解析失败都返回 false，保持"不符"结论由调用方继续下一来源。
func (r *Resolver) childrenMatch(ctx context.Context, client *http.Client, src source, tag string, local map[string]struct{}, run *runState) bool {
	getCtx, cancel := context.WithTimeout(ctx, r.childrenTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(getCtx, http.MethodGet, src.manifestURL(tag), nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", childrenAccept)
	if token := run.tokens[src.base]; token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var index struct {
		Manifests []struct {
			Digest string `json:"digest"`
		} `json:"manifests"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&index); err != nil {
		return false
	}
	for _, m := range index.Manifests {
		if _, ok := local[m.Digest]; ok {
			return true
		}
	}
	return false
}

// urlCredRe 用于脱敏网络错误串中可能出现的 user:pass@ 片段。
var urlCredRe = regexp.MustCompile(`(https?://)[^\s/@]+@`)

// sanitizeNetErr 脱敏网络错误串（剥离内嵌凭证，保留原始错误语义）。
func sanitizeNetErr(err error) string {
	if err == nil {
		return ""
	}
	return urlCredRe.ReplaceAllString(err.Error(), "${1}***@")
}
