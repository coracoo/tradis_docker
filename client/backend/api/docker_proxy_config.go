package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/logging"
)

const (
	dockerConfigSourceDaemon   = "daemon"
	dockerConfigSourceRuntime  = "runtime"
	dockerConfigSourceDatabase = "database"
)

type dockerRuntimeNetworkConfig struct {
	HTTPProxy       string
	HTTPSProxy      string
	NoProxy         string
	RegistryMirrors []string
}

type resolvedDockerNetworkConfig struct {
	Config           DockerConfig
	Effective        dockerRuntimeNetworkConfig
	ProxySource      string
	MirrorSource     string
	PendingRestart   bool
	HostAvailable    bool
	RuntimeAvailable bool
}

func resolveDockerNetworkConfig(
	daemonConfig *docker.DaemonConfig,
	daemonAvailable bool,
	runtimeConfig dockerRuntimeNetworkConfig,
	runtimeAvailable bool,
	stored *database.DockerProxy,
) resolvedDockerNetworkConfig {
	if daemonConfig == nil {
		daemonConfig = &docker.DaemonConfig{}
	}
	if stored == nil {
		stored = &database.DockerProxy{}
	}

	storedMirrors := []string{}
	if strings.TrimSpace(stored.RegistryMirrors) != "" {
		_ = json.Unmarshal([]byte(stored.RegistryMirrors), &storedMirrors)
	}

	result := resolvedDockerNetworkConfig{
		HostAvailable:    daemonAvailable || runtimeAvailable,
		RuntimeAvailable: runtimeAvailable,
		Effective:        runtimeConfig,
	}

	switch {
	case daemonAvailable && daemonConfig.Proxies != nil:
		result.Config.Enabled = hasDockerProxy(daemonConfig.Proxies.HTTPProxy, daemonConfig.Proxies.HTTPSProxy)
		result.Config.HTTPProxy = strings.TrimSpace(daemonConfig.Proxies.HTTPProxy)
		result.Config.HTTPSProxy = strings.TrimSpace(daemonConfig.Proxies.HTTPSProxy)
		result.Config.NoProxy = strings.TrimSpace(daemonConfig.Proxies.NoProxy)
		result.ProxySource = dockerConfigSourceDaemon
	case runtimeAvailable:
		result.Config.Enabled = hasDockerProxy(runtimeConfig.HTTPProxy, runtimeConfig.HTTPSProxy)
		result.Config.HTTPProxy = strings.TrimSpace(runtimeConfig.HTTPProxy)
		result.Config.HTTPSProxy = strings.TrimSpace(runtimeConfig.HTTPSProxy)
		result.Config.NoProxy = strings.TrimSpace(runtimeConfig.NoProxy)
		result.ProxySource = dockerConfigSourceRuntime
	case daemonAvailable:
		result.ProxySource = dockerConfigSourceDaemon
	default:
		result.Config.Enabled = stored.Enabled
		result.Config.HTTPProxy = strings.TrimSpace(stored.HTTPProxy)
		result.Config.HTTPSProxy = strings.TrimSpace(stored.HTTPSProxy)
		result.Config.NoProxy = strings.TrimSpace(stored.NoProxy)
		result.ProxySource = dockerConfigSourceDatabase
	}
	if !result.Config.Enabled {
		result.Config.HTTPProxy = ""
		result.Config.HTTPSProxy = ""
	}

	switch {
	case daemonAvailable && daemonConfig.RegistryMirrors != nil:
		result.Config.RegistryMirrors = append([]string(nil), daemonConfig.RegistryMirrors...)
		result.MirrorSource = dockerConfigSourceDaemon
	case runtimeAvailable:
		result.Config.RegistryMirrors = append([]string(nil), runtimeConfig.RegistryMirrors...)
		result.MirrorSource = dockerConfigSourceRuntime
	case daemonAvailable:
		result.Config.RegistryMirrors = []string{}
		result.MirrorSource = dockerConfigSourceDaemon
	default:
		result.Config.RegistryMirrors = storedMirrors
		result.MirrorSource = dockerConfigSourceDatabase
	}

	if runtimeAvailable {
		result.PendingRestart = !sameDockerProxy(result.Config, runtimeConfig) ||
			!sameMirrorList(result.Config.RegistryMirrors, runtimeConfig.RegistryMirrors)
	}
	return result
}

func hasDockerProxy(httpProxy, httpsProxy string) bool {
	return strings.TrimSpace(httpProxy) != "" || strings.TrimSpace(httpsProxy) != ""
}

func sameDockerProxy(config DockerConfig, runtime dockerRuntimeNetworkConfig) bool {
	return config.Enabled == hasDockerProxy(runtime.HTTPProxy, runtime.HTTPSProxy) &&
		strings.TrimSpace(config.HTTPProxy) == strings.TrimSpace(runtime.HTTPProxy) &&
		strings.TrimSpace(config.HTTPSProxy) == strings.TrimSpace(runtime.HTTPSProxy) &&
		normalizeNoProxy(config.NoProxy) == normalizeNoProxy(runtime.NoProxy)
}

func normalizeNoProxy(value string) string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return strings.Join(out, ",")
}

func sameMirrorList(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if normalizeMirror(left[i]) != normalizeMirror(right[i]) {
			return false
		}
	}
	return true
}

func normalizeMirror(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func readDockerRuntimeNetworkConfig(ctx context.Context) (dockerRuntimeNetworkConfig, error) {
	runtimeContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cli, err := docker.NewDockerClient()
	if err != nil {
		return dockerRuntimeNetworkConfig{}, err
	}
	defer cli.Close()
	info, err := cli.Info(runtimeContext)
	if err != nil {
		return dockerRuntimeNetworkConfig{}, err
	}
	mirrors := []string{}
	if info.RegistryConfig != nil {
		mirrors = append(mirrors, info.RegistryConfig.Mirrors...)
	}
	return dockerRuntimeNetworkConfig{
		HTTPProxy:       info.HTTPProxy,
		HTTPSProxy:      info.HTTPSProxy,
		NoProxy:         info.NoProxy,
		RegistryMirrors: mirrors,
	}, nil
}

func loadDockerNetworkConfig(ctx context.Context) resolvedDockerNetworkConfig {
	daemonAvailable := false
	daemonConfig, daemonErr := docker.GetDaemonConfig()
	if daemonErr == nil {
		if configPath, pathErr := docker.GetDaemonConfigPath(); pathErr == nil {
			if stat, statErr := os.Stat(configPath); statErr == nil && !stat.IsDir() {
				daemonAvailable = true
			}
		}
	}
	if daemonErr != nil {
		logging.Warn("Docker daemon configuration could not be read", "error", daemonErr)
	}
	runtimeConfig, runtimeErr := readDockerRuntimeNetworkConfig(ctx)
	if runtimeErr != nil {
		logging.Warn("Docker runtime network configuration could not be read", "error", runtimeErr)
	}
	stored, storedErr := database.GetDockerProxy()
	if storedErr != nil {
		logging.Warn("stored Docker proxy configuration could not be read", "error", storedErr)
		stored = &database.DockerProxy{}
	}
	return resolveDockerNetworkConfig(
		daemonConfig,
		daemonAvailable,
		runtimeConfig,
		runtimeErr == nil,
		stored,
	)
}

// SyncHostDockerNetworkConfig reconciles the editable daemon configuration and
// effective Docker runtime state into the database fallback on every startup.
func SyncHostDockerNetworkConfig(ctx context.Context) error {
	resolved := loadDockerNetworkConfig(ctx)
	if !resolved.HostAvailable {
		return fmt.Errorf("宿主机 Docker 网络配置不可用")
	}
	proxy := &database.DockerProxy{
		Enabled:         resolved.Config.Enabled,
		HTTPProxy:       resolved.Config.HTTPProxy,
		HTTPSProxy:      resolved.Config.HTTPSProxy,
		NoProxy:         resolved.Config.NoProxy,
		RegistryMirrors: database.MarshalRegistryMirrors(resolved.Config.RegistryMirrors),
	}
	if !proxy.Enabled && proxy.RegistryMirrors == "" {
		return database.DeleteDockerProxy()
	}
	return database.SaveDockerProxy(proxy)
}
