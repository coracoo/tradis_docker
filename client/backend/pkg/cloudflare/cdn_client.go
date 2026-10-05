package cloudflare

import (
	"context"
	"crypto/tls"
	"dockerpanel/backend/pkg/logging"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dockerpanel/backend/internal/templatecompiler"
)

// CDNClient CDN 客户端
type CDNClient struct {
	baseURL    string
	httpClient *http.Client
	repository bool
	sourceErr  error
}

// TemplateManifest 模板清单 (列表视图)
type TemplateManifest struct {
	ID              uint   `json:"id"`
	SortOrder       int    `json:"sort_order"`
	Name            string `json:"name"`
	Category        string `json:"category"`
	Description     string `json:"description"`
	Version         string `json:"version"`
	Logo            string `json:"logo"`
	Website         string `json:"website"`
	DeploymentCount int    `json:"deployment_count"`
	File            string `json:"file"`
}

// TemplateDetail 模板详情
type TemplateDetail struct {
	ID                 uint                                  `json:"id"`
	SortOrder          int                                   `json:"sort_order"`
	Name               string                                `json:"name"`
	Category           string                                `json:"category"`
	Description        string                                `json:"description"`
	Version            string                                `json:"version"`
	Logo               string                                `json:"logo"`
	Website            string                                `json:"website"`
	Tutorial           string                                `json:"tutorial"`
	Dotenv             string                                `json:"dotenv"`
	Compose            string                                `json:"compose"`
	SourceFiles        []templatecompiler.SourceFile         `json:"source_files,omitempty"`
	InputMetadata      templatecompiler.PresentationMetadata `json:"input_metadata,omitempty"`
	Manifest           *templatecompiler.Manifest            `json:"manifest,omitempty"`
	SourceDigest       string                                `json:"source_digest,omitempty"`
	ManifestDigest     string                                `json:"manifest_digest,omitempty"`
	CompilerVersion    string                                `json:"compiler_version,omitempty"`
	CompileDiagnostics []templatecompiler.Diagnostic         `json:"compile_diagnostics,omitempty"`
	Screenshots        []string                              `json:"screenshots"`
	Schema             []Variable                            `json:"schema"`
	DeploymentCount    int                                   `json:"deployment_count"`
	Enabled            bool                                  `json:"enabled"`
}

// Variable 变量定义
type Variable struct {
	InputID     string `json:"inputId,omitempty"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Category    string `json:"category"`
	ServiceName string `json:"serviceName"`
	ParamType   string `json:"paramType"`
	EnvFile     string `json:"envFile,omitempty"`
}

// NewCDNClient 创建 CDN 客户端
func NewCDNClient(baseURL string) *CDNClient {
	baseURL, repository, sourceErr := normalizeTemplateRepository(baseURL)
	return &CDNClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		repository: repository,
		sourceErr:  sourceErr,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *CDNClient) shouldUseOptimizedIP(path string) bool {
	if c.repository || c.sourceErr != nil {
		return false
	}
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return true
	}
	proxyURL, err := http.ProxyFromEnvironment(req)
	return err != nil || proxyURL == nil
}

// fetchWithIP 使用指定 IP 请求
func (c *CDNClient) fetchWithIP(ip, path string) ([]byte, error) {
	targetURL := c.baseURL + path
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, err
	}

	parsed, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		port = "443"
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			ServerName: host,
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip, port))
		},
	}
	client := &http.Client{
		Timeout:   c.httpClient.Timeout,
		Transport: transport,
	}
	defer transport.CloseIdleConnections()

	req.Header.Set("User-Agent", "TRADIS-CDN-Client/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// fetchWithDomain 使用域名请求 (备用方案)
func (c *CDNClient) fetchWithDomain(path string) ([]byte, error) {
	url := c.baseURL + path

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// FetchTemplates 获取模板列表
func (c *CDNClient) FetchTemplates() ([]TemplateManifest, error) {
	if c.sourceErr != nil {
		return nil, c.sourceErr
	}
	if c.repository {
		return c.fetchRepositoryIndex()
	}
	path := "/api/templates"

	// 尝试使用最优 IP
	bestIP := GetBestIP()
	if bestIP != "" && c.shouldUseOptimizedIP(path) {
		data, err := c.fetchWithIP(bestIP, path)
		if err == nil {
			var templates []TemplateManifest
			if err := json.Unmarshal(data, &templates); err == nil {
				return templates, nil
			}
			logging.Debug("CDN optimized response parse failed")
		} else {
			logging.Debug("CDN optimized request failed; retrying domain route")
			ReportIPFailure(bestIP)
		}
	}

	// 回退到域名
	data, err := c.fetchWithDomain(path)
	if err != nil {
		return nil, err
	}

	var templates []TemplateManifest
	if err := json.Unmarshal(data, &templates); err != nil {
		return nil, err
	}

	return templates, nil
}

// FetchTemplate 获取单个模板详情
func (c *CDNClient) FetchTemplate(idOrName string) (*TemplateDetail, error) {
	if c.sourceErr != nil {
		return nil, c.sourceErr
	}
	if c.repository {
		return c.fetchRepositoryTemplate(idOrName)
	}
	path := fmt.Sprintf("/api/templates/%s", idOrName)

	// 尝试使用最优 IP
	bestIP := GetBestIP()
	if bestIP != "" && c.shouldUseOptimizedIP(path) {
		data, err := c.fetchWithIP(bestIP, path)
		if err == nil {
			var template TemplateDetail
			if err := json.Unmarshal(data, &template); err == nil {
				return &template, nil
			}
			logging.Debug("CDN optimized template response parse failed")
		} else {
			logging.Debug("CDN optimized template request failed; retrying domain route")
			ReportIPFailure(bestIP)
		}
	}

	// 回退到域名
	data, err := c.fetchWithDomain(path)
	if err != nil {
		return nil, err
	}

	var template TemplateDetail
	if err := json.Unmarshal(data, &template); err != nil {
		return nil, err
	}

	return &template, nil
}

// FetchRaw 原始请求 (用于 compose.yaml 等)
func (c *CDNClient) FetchRaw(path string) ([]byte, error) {
	if c.sourceErr != nil {
		return nil, c.sourceErr
	}
	if c.repository {
		return c.fetchRepositoryRaw(path)
	}
	// 尝试使用最优 IP
	bestIP := GetBestIP()
	if bestIP != "" && c.shouldUseOptimizedIP(path) {
		data, err := c.fetchWithIP(bestIP, path)
		if err == nil {
			return data, nil
		}
		logging.Debug("CDN optimized raw request failed; retrying domain route")
		ReportIPFailure(bestIP)
	}

	// 回退到域名
	return c.fetchWithDomain(path)
}

// FetchWithRetry 带重试的请求
func (c *CDNClient) FetchWithRetry(path string, maxRetries int) ([]byte, error) {
	if c.sourceErr != nil || c.repository {
		return c.FetchRaw(path)
	}
	// 先尝试最优 IP
	bestIPs := GetBestIPs()
	if len(bestIPs) > 0 && c.shouldUseOptimizedIP(path) {
		for i, ip := range bestIPs {
			if i >= maxRetries {
				break
			}
			data, err := c.fetchWithIP(ip, path)
			if err == nil {
				return data, nil
			}
			logging.Debug("CDN optimized request retry failed", "attempt", i+1, "limit", maxRetries)
			ReportIPFailure(ip)
		}
	}

	// 回退到域名
	data, err := c.fetchWithDomain(path)
	if err == nil {
		return data, nil
	}
	return nil, err
}
