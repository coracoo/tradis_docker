// Package registryprobe provides credential-free, public Registry V2 connectivity checks.
// It is shared by Full and Community; it does not pull images or configure Docker.
package registryprobe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const probeTimeout = 6 * time.Second

var errRestrictedAddress = errors.New("registry resolved to a restricted address")

type Result struct {
	URL        string `json:"url"`
	Status     string `json:"status"`
	CheckedAt  string `json:"checkedAt"`
	LatencyMS  int64  `json:"latencyMs"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
}

func NormalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf("镜像源必须是无凭证的 HTTPS 地址")
	}
	if u.Path != "" && u.Path != "/" && u.Path != "/v2" && u.Path != "/v2/" {
		return "", fmt.Errorf("镜像源地址不能包含仓库路径")
	}
	host := strings.ToLower(u.Hostname())
	if strings.HasSuffix(host, ".") || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return "", fmt.Errorf("仅允许检测公网镜像源")
	}
	if ip := net.ParseIP(host); ip != nil {
		if restrictedIP(ip) {
			return "", fmt.Errorf("仅允许检测公网镜像源")
		}
	} else if !strings.Contains(host, ".") {
		return "", fmt.Errorf("仅允许检测公网镜像源")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return "", fmt.Errorf("镜像源端口无效")
		}
	}
	u.Host = strings.ToLower(u.Host)
	u.Path, u.RawPath = "", ""
	return u.String(), nil
}

func restrictedIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return true
	}
	for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

func newHTTPClient(resolve func(context.Context, string) ([]net.IPAddr, error), dial func(context.Context, string, string) (net.Conn, error)) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableKeepAlives = true
	transport.TLSHandshakeTimeout = probeTimeout
	transport.ResponseHeaderTimeout = probeTimeout
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := resolve(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(addresses) == 0 {
			return nil, fmt.Errorf("no registry addresses")
		}
		// Validate the entire answer, then dial the approved IP directly: no second DNS lookup.
		for _, resolved := range addresses {
			if restrictedIP(resolved.IP) {
				return nil, errRestrictedAddress
			}
		}
		for _, resolved := range addresses {
			conn, dialErr := dial(ctx, network, net.JoinHostPort(resolved.IP.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			err = dialErr
		}
		return nil, err
	}
	return &http.Client{
		Timeout: probeTimeout, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func Check(ctx context.Context, raw string) Result {
	base, err := NormalizeURL(raw)
	if err != nil {
		return Result{Status: "invalid", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	}
	dialer := &net.Dialer{Timeout: probeTimeout}
	return check(ctx, base, newHTTPClient(net.DefaultResolver.LookupIPAddr, dialer.DialContext))
}

func check(ctx context.Context, base string, client *http.Client) Result {
	started := time.Now()
	result := Result{URL: base, Status: "unreachable", CheckedAt: started.UTC().Format(time.RFC3339)}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v2/", nil)
	if err != nil {
		return result
	}
	req.Header.Set("User-Agent", "TRADIS-Registry-Probe")
	response, err := client.Do(req)
	result.LatencyMS = time.Since(started).Milliseconds()
	if err != nil {
		var netErr net.Error
		var dnsErr *net.DNSError
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			result.Status = "timeout"
		} else if errors.Is(err, context.Canceled) {
			result.Status = "cancelled"
		} else if errors.Is(err, errRestrictedAddress) {
			result.Status = "network_restricted"
		} else if errors.As(err, &dnsErr) {
			result.Status = "dns_failed"
		}
		return result
	}
	defer response.Body.Close()
	result.HTTPStatus = response.StatusCode
	result.Status = "unexpected_response"
	isRegistry := strings.EqualFold(strings.TrimSpace(response.Header.Get("Docker-Distribution-Api-Version")), "registry/2.0")
	switch {
	case response.StatusCode == http.StatusOK && isRegistry:
		result.Status = "reachable"
	case response.StatusCode == http.StatusUnauthorized && (isRegistry || strings.HasPrefix(strings.ToLower(response.Header.Get("WWW-Authenticate")), "bearer ")):
		result.Status = "auth_required"
	case response.StatusCode == http.StatusTooManyRequests:
		result.Status = "rate_limited"
	}
	return result
}
