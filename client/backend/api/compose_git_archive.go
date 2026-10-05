package api

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	maxGitArchiveDownloadSize = int64(256 * 1024 * 1024)
	maxGitArchiveExtractSize  = int64(1024 * 1024 * 1024)
	maxGitArchiveFiles        = 50000
)

var fullGitCommitPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func validatePublicDownloadURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("下载地址必须是无认证信息的 HTTPS URL")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".local") {
		return fmt.Errorf("下载地址不能指向本机或局域网")
	}
	addresses, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("无法解析下载地址: %w", err)
	}
	for _, ip := range addresses {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("下载地址解析到了内网 IP")
		}
	}
	return nil
}

func githubArchiveURL(repoURL, ref string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") {
		return "", fmt.Errorf("GitHub 仓库地址无效")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("GitHub 仓库路径无效")
	}
	repo := strings.TrimSuffix(parts[1], ".git")
	archiveRef := "HEAD"
	if strings.TrimSpace(ref) != "" {
		archiveRef = url.PathEscape(strings.TrimSpace(ref))
	}
	return fmt.Sprintf("https://github.com/%s/%s/archive/%s.tar.gz", parts[0], repo, archiveRef), nil
}

// rejectGitSubmoduleClone blocks repositories that declare submodules. TRADIS
// clones with --depth 1 and never recurses into submodules, so a gitlink entry
// would otherwise be silently dropped; require an explicit submodule-free source
// instead of producing an incomplete working copy.
func rejectGitSubmoduleClone(destination string) error {
	raw, err := os.ReadFile(filepath.Join(destination, ".gitmodules"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 .gitmodules 失败: %w", err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		return fmt.Errorf("仓库包含 submodule（.gitmodules），TRADIS 不递归子模块，请改用不含子模块的来源或 Compose")
	}
	return nil
}

func extractGitHubTarGz(reader io.Reader, destination string) (string, error) {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return "", fmt.Errorf("源码归档不是有效的 gzip: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	var totalSize int64
	fileCount := 0
	rootName := ""

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("读取源码归档失败: %w", err)
		}
		fileCount++
		if fileCount > maxGitArchiveFiles {
			return "", fmt.Errorf("源码归档文件数量超过限制")
		}

		archiveName := filepath.ToSlash(header.Name)
		if strings.HasPrefix(archiveName, "/") {
			return "", fmt.Errorf("源码归档包含绝对路径: %s", header.Name)
		}
		rawParts := strings.Split(archiveName, "/")
		for _, part := range rawParts {
			if part == ".." {
				return "", fmt.Errorf("源码归档包含路径穿越: %s", header.Name)
			}
		}
		cleanName := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(archiveName)), "/")
		parts := strings.SplitN(cleanName, "/", 2)
		if len(parts) < 2 || parts[0] == "" {
			continue
		}
		if rootName == "" {
			rootName = parts[0]
		} else if rootName != parts[0] {
			return "", fmt.Errorf("源码归档包含多个顶层目录")
		}

		relative := filepath.Clean(filepath.FromSlash(parts[1]))
		if relative == "." || relative == "" {
			continue
		}
		target := filepath.Join(destination, relative)
		rel, err := filepath.Rel(destination, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return "", fmt.Errorf("源码归档包含路径穿越: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return "", err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return "", fmt.Errorf("源码归档包含无效文件大小")
			}
			totalSize += header.Size
			if totalSize > maxGitArchiveExtractSize {
				return "", fmt.Errorf("源码归档解压后大小超过限制")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return "", err
			}
			mode := os.FileMode(0644)
			if header.FileInfo().Mode()&0111 != 0 {
				mode = 0755
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return "", err
			}
			_, copyErr := io.CopyN(file, tarReader, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return "", copyErr
			}
			if closeErr != nil {
				return "", closeErr
			}
		case tar.TypeSymlink, tar.TypeLink:
			return "", fmt.Errorf("源码归档包含禁止的链接条目 %s（linkname=%s），仓库不得通过符号/硬链接读写归档外路径", header.Name, header.Linkname)
		case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			return "", fmt.Errorf("源码归档包含禁止的特殊文件条目: %s", header.Name)
		default:
			return "", fmt.Errorf("源码归档包含不允许的链接或特殊文件: %s", header.Name)
		}
	}

	if rootName == "" {
		return "", fmt.Errorf("源码归档为空")
	}
	if index := strings.LastIndex(rootName, "-"); index >= 0 {
		candidate := rootName[index+1:]
		if fullGitCommitPattern.MatchString(candidate) {
			return strings.ToLower(candidate), nil
		}
	}
	return "", nil
}

func downloadGitHubArchive(ctx context.Context, archiveURL, destination string) (string, error) {
	if err := validatePublicDownloadURL(archiveURL); err != nil {
		return "", err
	}
	client := &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 10 {
				return fmt.Errorf("下载重定向次数过多")
			}
			return validatePublicDownloadURL(req.URL.String())
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "tradis-compose-git/1.0")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("源码归档下载失败: %s", response.Status)
	}
	if response.ContentLength > maxGitArchiveDownloadSize {
		return "", fmt.Errorf("源码归档下载大小超过限制")
	}

	limited := &io.LimitedReader{R: response.Body, N: maxGitArchiveDownloadSize + 1}
	commitHash, err := extractGitHubTarGz(limited, destination)
	if err != nil {
		return "", err
	}
	if limited.N <= 0 {
		return "", fmt.Errorf("源码归档下载大小超过限制")
	}
	return commitHash, nil
}

func downloadPublicGitHubRepository(
	repoURL string,
	branch string,
	acceleratorURL string,
	destination string,
	appendLog func(string, string),
) (string, error) {
	return downloadPublicGitHubRepositoryWithContext(context.Background(), repoURL, branch, acceleratorURL, destination, appendLog)
}

func downloadPublicGitHubRepositoryWithContext(
	ctx context.Context,
	repoURL string,
	branch string,
	acceleratorURL string,
	destination string,
	appendLog func(string, string),
) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := exec.LookPath("git"); err == nil {
		cloneURLs := make([]string, 0, 2)
		if acceleratorURL != "" {
			if acceleratedURL, accelerateErr := buildAcceleratedGitURL(repoURL, acceleratorURL); accelerateErr == nil {
				cloneURLs = append(cloneURLs, acceleratedURL)
			} else {
				appendLog("warning", "Git 加速地址不可用，将回退原始地址: "+accelerateErr.Error())
			}
		}
		cloneURLs = append(cloneURLs, repoURL)

		for index, cloneURL := range cloneURLs {
			_ = os.RemoveAll(destination)
			if index == 0 && acceleratorURL != "" && cloneURL != repoURL {
				appendLog("info", "优先通过 Git 加速地址克隆")
			} else {
				appendLog("info", "通过 GitHub 原始地址克隆")
			}
			args := []string{"clone", "--depth", "1"}
			if branch != "" {
				args = append(args, "--branch", branch, "--single-branch")
			}
			args = append(args, "--", cloneURL, destination)
			cloneCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			command := exec.CommandContext(cloneCtx, "git", args...)
			command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
			output, cloneErr := command.CombinedOutput()
			if text := strings.TrimSpace(string(output)); text != "" {
				appendLog("info", text)
			}
			if cloneErr == nil {
				if rejectErr := rejectGitSubmoduleClone(destination); rejectErr != nil {
					cancel()
					_ = os.RemoveAll(destination)
					appendLog("warning", rejectErr.Error())
					return "", rejectErr
				}
				commitOutput, commitErr := exec.CommandContext(cloneCtx, "git", "-C", destination, "rev-parse", "HEAD").Output()
				cancel()
				if commitErr == nil {
					return strings.TrimSpace(string(commitOutput)), nil
				}
				return "", nil
			}
			cancel()
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			appendLog("warning", "当前 Git 克隆链路失败: "+cloneErr.Error())
		}
		appendLog("warning", "Git CLI 克隆失败，尝试源码归档下载")
	} else {
		appendLog("warning", "运行环境未安装 Git CLI，自动改用 GitHub 源码归档下载")
	}

	archiveURL, err := githubArchiveURL(repoURL, branch)
	if err != nil {
		return "", err
	}
	archiveURLs := make([]string, 0, 2)
	if acceleratorURL != "" {
		if acceleratedURL, accelerateErr := buildAcceleratedGitURL(archiveURL, acceleratorURL); accelerateErr == nil {
			archiveURLs = append(archiveURLs, acceleratedURL)
		} else {
			appendLog("warning", "源码归档加速地址不可用: "+accelerateErr.Error())
		}
	}
	archiveURLs = append(archiveURLs, archiveURL)

	var lastErr error
	for index, candidate := range archiveURLs {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		_ = os.RemoveAll(destination)
		if err := os.MkdirAll(destination, 0755); err != nil {
			return "", err
		}
		if index == 0 && acceleratorURL != "" && candidate != archiveURL {
			appendLog("info", "优先通过加速地址下载 GitHub 源码归档")
		} else {
			appendLog("info", "通过 GitHub 原始地址下载源码归档")
		}
		downloadCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		commitHash, downloadErr := downloadGitHubArchive(downloadCtx, candidate, destination)
		cancel()
		if downloadErr == nil {
			return commitHash, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		lastErr = downloadErr
		appendLog("warning", "当前源码归档下载链路失败: "+downloadErr.Error())
	}
	return "", fmt.Errorf("Git 克隆和源码归档下载均失败: %w", lastErr)
}
