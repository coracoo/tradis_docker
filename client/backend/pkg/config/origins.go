package config

import (
	"net/url"
	"os"
	"strings"
)

var (
	// AllowedOrigins 统一允许的源列表
	AllowedOrigins []string
)

func init() {
	loadOrigins()
}

// loadOrigins 从环境变量加载允许的源
func loadOrigins() {
	// 优先读取统一的 ALLOWED_ORIGINS
	origins := os.Getenv("ALLOWED_ORIGINS")

	if origins == "" {
		// 开发环境默认值
		AllowedOrigins = []string{
			"http://localhost:33339",
			"http://localhost:5173",
			"http://localhost:8080",
			"http://127.0.0.1:33339",
			"http://127.0.0.1:5173",
			"http://127.0.0.1:8080",
		}
	} else {
		AllowedOrigins = splitAndTrim(origins, ",")
	}
}

// GetCORSOrigins 获取CORS允许的源
// 优先使用 CORS_ALLOWED_ORIGINS，其次使用统一的 ALLOWED_ORIGINS
func GetCORSOrigins() []string {
	if origins := os.Getenv("CORS_ALLOWED_ORIGINS"); origins != "" {
		return splitAndTrim(origins, ",")
	}
	return AllowedOrigins
}

// GetWSOrigins 获取WebSocket允许的源
// 优先使用 WS_ALLOWED_ORIGINS，其次使用统一的 ALLOWED_ORIGINS
func GetWSOrigins() []string {
	if origins := os.Getenv("WS_ALLOWED_ORIGINS"); origins != "" {
		return splitAndTrim(origins, ",")
	}
	return AllowedOrigins
}

// IsAllowedOrigin 检查来源是否允许
// 支持以下匹配模式:
// - 完整匹配: https://example.com
// - 通配符 *: 允许所有来源（不推荐生产环境使用）
// - 子域名通配符: *.example.com 匹配所有子域名
func IsAllowedOrigin(origin string, allowedList []string) bool {
	// 允许空origin（某些客户端可能不发送）
	if origin == "" {
		return true
	}

	normalizedOrigin := strings.TrimRight(strings.TrimSpace(origin), "/")
	parsedOrigin, parseErr := url.Parse(normalizedOrigin)

	for _, allowed := range allowedList {
		allowed = strings.TrimRight(strings.TrimSpace(allowed), "/")

		// 完全匹配
		if strings.EqualFold(normalizedOrigin, allowed) {
			return true
		}

		// 通配符 * - 允许所有来源
		if allowed == "*" {
			return true
		}

		// 子域名通配符: *.example.com
		if strings.HasPrefix(allowed, "*.") {
			if parseErr != nil || parsedOrigin.Scheme == "" || parsedOrigin.Hostname() == "" {
				continue
			}
			domain := strings.ToLower(strings.TrimSuffix(allowed[2:], "."))
			hostname := strings.ToLower(strings.TrimSuffix(parsedOrigin.Hostname(), "."))

			// 必须是真实子域名，不能匹配根域名或 fake-example.com。
			if hostname != domain && strings.HasSuffix(hostname, "."+domain) {
				return true
			}
		}
	}
	return false
}

// splitAndTrim 分割字符串并去除空白
func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// IsProduction 检查是否为生产环境
func IsProduction() bool {
	return os.Getenv("GIN_MODE") == "release"
}
