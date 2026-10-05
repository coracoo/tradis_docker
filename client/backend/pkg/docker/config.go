package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
)

var daemonConfigMu sync.Mutex

// DaemonConfig 定义 Docker daemon.json 的配置结构
type DaemonConfig struct {
	RegistryMirrors []string            `json:"registry-mirrors,omitempty"`
	Proxies         *ProxyConfig        `json:"proxies,omitempty"`
	ClearProxies    bool                `json:"-"`
	Registries      map[string]Registry `json:"registries,omitempty"`
	IPv6            bool                `json:"ipv6,omitempty"`
	FixedCIDRv6     string              `json:"fixed-cidr-v6,omitempty"`
	MinAPIVersion   string              `json:"min-api-version,omitempty"`
}

// ProxyConfig 定义代理配置结构
type ProxyConfig struct {
	HTTPProxy  string `json:"http-proxy,omitempty"`
	HTTPSProxy string `json:"https-proxy,omitempty"`
	NoProxy    string `json:"no-proxy,omitempty"`
}

// UpdateDaemonConfig 更新 Docker daemon.json 配置
func UpdateDaemonConfig(config *DaemonConfig) error {
	configPath, err := GetDaemonConfigPath()
	if err != nil {
		return fmt.Errorf("获取配置路径失败: %v", err)
	}
	return updateDaemonConfigAt(configPath, config)
}

func updateDaemonConfigAt(configPath string, config *DaemonConfig) error {
	if config == nil {
		return fmt.Errorf("Docker 配置不能为空")
	}
	daemonConfigMu.Lock()
	defer daemonConfigMu.Unlock()

	var existing map[string]interface{}
	if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
		existing = make(map[string]interface{})
	} else {
		raw, readErr := os.ReadFile(configPath)
		if readErr != nil {
			return fmt.Errorf("读取配置文件失败: %v", readErr)
		}
		if len(raw) == 0 {
			existing = make(map[string]interface{})
		} else {
			if unmarshalErr := json.Unmarshal(raw, &existing); unmarshalErr != nil {
				return fmt.Errorf("解析配置文件失败: %v", unmarshalErr)
			}
		}
	}

	if existing == nil {
		return fmt.Errorf("Docker 配置必须是 JSON 对象")
	}
	if config.RegistryMirrors != nil {
		existing["registry-mirrors"] = config.RegistryMirrors
	}

	if config.IPv6 {
		existing["ipv6"] = true
	}
	if config.FixedCIDRv6 != "" {
		existing["fixed-cidr-v6"] = config.FixedCIDRv6
	}

	if config.Proxies != nil {
		proxies := map[string]string{}
		if config.Proxies.HTTPProxy != "" {
			proxies["http-proxy"] = config.Proxies.HTTPProxy
		}
		if config.Proxies.HTTPSProxy != "" {
			proxies["https-proxy"] = config.Proxies.HTTPSProxy
		}
		if config.Proxies.NoProxy != "" {
			proxies["no-proxy"] = config.Proxies.NoProxy
		}
		existing["proxies"] = proxies
	} else if config.ClearProxies {
		delete(existing, "proxies")
	}

	if config.Registries != nil {
		existing["registries"] = config.Registries
	}

	if config.MinAPIVersion != "" {
		existing["min-api-version"] = config.MinAPIVersion
	}

	data, err := json.MarshalIndent(existing, "", "    ")
	if err != nil {
		return err
	}
	if err := writeDaemonConfig(configPath, data, 0o644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}
	return nil
}

func writeDaemonConfig(path string, data []byte, mode os.FileMode) error {
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".tradis-daemon-*.tmp")
	if err != nil {
		// A single-file bind mount can be writable while its parent directory is
		// read-only. Keep that supported; the process mutex still prevents two API
		// requests from interleaving their read/modify/write cycle.
		return writeDaemonConfigInPlace(path, data, mode)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		if errors.Is(err, syscall.EBUSY) {
			return writeDaemonConfigInPlace(path, data, mode)
		}
		return err
	}
	return nil
}

func writeDaemonConfigInPlace(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

type Registry struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// GetDaemonConfigPath 获取 daemon.json 文件路径
func GetDaemonConfigPath() (string, error) {
	var configPath string

	switch runtime.GOOS {
	case "windows":
		configPath = filepath.Join(os.Getenv("ProgramData"), "Docker", "config", "daemon.json")
	case "linux":
		configPath = "/etc/docker/daemon.json"
	case "darwin":
		configPath = filepath.Join(os.Getenv("HOME"), "Library", "Containers", "com.docker.docker", "Data", "daemon.json")
	default:
		return "", fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}

	return configPath, nil
}

// GetDaemonConfig 读取 Docker daemon.json 配置
func GetDaemonConfig() (*DaemonConfig, error) {
	daemonConfigMu.Lock()
	defer daemonConfigMu.Unlock()
	configPath, err := GetDaemonConfigPath()
	if err != nil {
		return nil, err
	}

	// 检查文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// 如果文件不存在，返回空配置
		return &DaemonConfig{}, nil
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取 daemon.json 失败: %v", err)
	}

	// 如果文件为空，返回空配置
	if len(data) == 0 {
		return &DaemonConfig{}, nil
	}

	// 解析 JSON
	var config DaemonConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析 daemon.json 失败: %v", err)
	}

	return &config, nil
}

func checkConfigPermissions(configPath string) error {
	// 检查文件是否存在
	if _, err := os.Stat(configPath); err == nil {
		// 尝试打开文件进行写入测试
		f, err := os.OpenFile(configPath, os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("无写入权限: %v", err)
		}
		f.Close()
	}
	return nil
}
