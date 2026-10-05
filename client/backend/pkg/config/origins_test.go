package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAllowedOrigin(t *testing.T) {
	allowedList := []string{
		"http://localhost:33339",
		"https://example.com",
	}

	tests := []struct {
		name     string
		origin   string
		expected bool
	}{
		{
			name:     "exact match localhost",
			origin:   "http://localhost:33339",
			expected: true,
		},
		{
			name:     "exact match domain",
			origin:   "https://example.com",
			expected: true,
		},
		{
			name:     "case insensitive match",
			origin:   "HTTPS://EXAMPLE.COM",
			expected: true,
		},
		{
			name:     "not allowed origin",
			origin:   "http://evil.com",
			expected: false,
		},
		{
			name:     "empty origin allowed",
			origin:   "",
			expected: true,
		},
		{
			name:     "different port",
			origin:   "http://localhost:8080",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAllowedOrigin(tt.origin, allowedList)
			assert.Equal(t, tt.expected, result, "origin: %s", tt.origin)
		})
	}
}

func TestIsAllowedOrigin_Wildcard(t *testing.T) {
	allowedList := []string{"*"}

	tests := []struct {
		name   string
		origin string
	}{
		{
			name:   "any domain",
			origin: "http://anything.com",
		},
		{
			name:   "different protocol",
			origin: "https://other.org",
		},
		{
			name:   "with port",
			origin: "http://test.site:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, IsAllowedOrigin(tt.origin, allowedList))
		})
	}
}

func TestIsAllowedOrigin_SubdomainWildcard(t *testing.T) {
	allowedList := []string{"*.example.com"}

	tests := []struct {
		name     string
		origin   string
		expected bool
	}{
		{
			name:     "subdomain match",
			origin:   "https://app.example.com",
			expected: true,
		},
		{
			name:     "deep subdomain match",
			origin:   "https://api.v1.example.com",
			expected: true,
		},
		{
			name:     "subdomain with port",
			origin:   "https://app.example.com:8443",
			expected: true,
		},
		{
			name:     "case insensitive subdomain",
			origin:   "HTTPS://APP.EXAMPLE.COM",
			expected: true,
		},
		{
			name:     "exact domain not match",
			origin:   "https://example.com",
			expected: false,
		},
		{
			name:     "different domain",
			origin:   "https://example.org",
			expected: false,
		},
		{
			name:     "fake subdomain",
			origin:   "https://fake-example.com",
			expected: false,
		},
		{
			name:     "invalid origin",
			origin:   "not-a-url.example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAllowedOrigin(tt.origin, allowedList)
			assert.Equal(t, tt.expected, result, "origin: %s", tt.origin)
		})
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		sep      string
		expected []string
	}{
		{
			name:     "comma separated",
			input:    "a,b,c",
			sep:      ",",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "with spaces",
			input:    " a , b , c ",
			sep:      ",",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty items filtered",
			input:    "a,,b,",
			sep:      ",",
			expected: []string{"a", "b"},
		},
		{
			name:     "single item",
			input:    "only-one",
			sep:      ",",
			expected: []string{"only-one"},
		},
		{
			name:     "empty string",
			input:    "",
			sep:      ",",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAndTrim(tt.input, tt.sep)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCORSOrigins_Priority(t *testing.T) {
	// 注意：此测试可能受到环境变量影响
	// 在实际运行时需要确保 CORS_ALLOWED_ORIGINS 和 ALLOWED_ORIGINS 未设置
	// 这里主要测试函数逻辑

	origins := GetCORSOrigins()
	// 如果没有设置环境变量，应该返回 AllowedOrigins（开发环境默认值）
	assert.NotNil(t, origins)
	assert.GreaterOrEqual(t, len(origins), 1)
}

func TestGetWSOrigins_Priority(t *testing.T) {
	// 类似 GetCORSOrigins 的测试
	origins := GetWSOrigins()
	assert.NotNil(t, origins)
	assert.GreaterOrEqual(t, len(origins), 1)
}
