package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSensitiveLoggingPatternsAreAbsent(t *testing.T) {
	tests := []struct {
		file      string
		forbidden []string
	}{
		{
			file: "../../pkg/system/discovery.go",
			forbidden: []string{
				"baseUrl=%s",
				"Query: %s, args=%v",
				"LAN: %s, WAN: %s",
			},
		},
		{
			file: "../../api/terminal.go",
			forbidden: []string{
				"执行容器命令: %s",
				"使用入口命令: %s",
				"收到WebSocket消息: type=%s",
			},
		},
		{
			file: "../../api/image.go",
			forbidden: []string{
				"HTTP=%s, HTTPS=%s",
				"url=%s",
				"完整镜像名: %s",
				"Docker 代理设置:",
			},
		},
		{
			file: "../../api/image_registry.go",
			forbidden: []string{
				"url=%s",
			},
		},
		{
			file: "../../pkg/database/registry.go",
			forbidden: []string{
				"URL=%s",
				"url=%s",
			},
		},
		{
			file: "../../api/appstore_read.go",
			forbidden: []string{
				"优选 IP: %s",
				"listApps fetch: %s",
				"Requesting Server: %s",
			},
		},
		{
			file: "../../api/appstore_deploy.go",
			forbidden: []string{
				"submitAppStoreDeploymentCount: %s",
			},
		},
		{
			file: "../../pkg/cloudflare/cdn_client.go",
			forbidden: []string{
				"IP %s",
				"请求 %s",
			},
		},
		{
			file: "../../pkg/cloudflare/cfspeedtest.go",
			forbidden: []string{
				"正在下载: %s",
				"当前首选 %s",
				"输出: %s",
			},
		},
		{
			file: "../../pkg/settings/settings.go",
			forbidden: []string{
				"lanUrl=%s",
				"tutorialRSSURL=%s",
				"aiBaseUrl=%s",
			},
		},
		{
			file: "../../pkg/docker/client.go",
			forbidden: []string{
				"io.Copy(os.Stdout",
			},
		},
	}

	for _, testCase := range tests {
		t.Run(filepath.Base(testCase.file), func(t *testing.T) {
			source, err := os.ReadFile(testCase.file)
			if err != nil {
				t.Fatalf("read %s: %v", testCase.file, err)
			}
			for _, forbidden := range testCase.forbidden {
				if strings.Contains(string(source), forbidden) {
					t.Errorf("%s contains sensitive logging pattern %q", testCase.file, forbidden)
				}
			}
		})
	}
}

func TestPersistedMessagesUseUniversalURLRedaction(t *testing.T) {
	tests := []struct {
		file     string
		expected string
	}{
		{file: "../../pkg/system/events.go", expected: "logging.RedactText("},
		{file: "../../pkg/system/volume_backup.go", expected: "logging.RedactText("},
	}

	for _, testCase := range tests {
		source, err := os.ReadFile(testCase.file)
		if err != nil {
			t.Fatalf("read %s: %v", testCase.file, err)
		}
		if !strings.Contains(string(source), testCase.expected) {
			t.Errorf("%s does not apply universal URL redaction", testCase.file)
		}
	}
}
