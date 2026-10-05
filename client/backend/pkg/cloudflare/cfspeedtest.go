package cloudflare

import (
	"bufio"
	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/settings"
	"dockerpanel/backend/pkg/system"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// CFSpeedTestVersion CloudflareSpeedTest 版本
	CFSpeedTestVersion = "v2.3.4"
	// DefaultDownloadURL 下载地址
	DefaultDownloadURL = "https://github.com/XIU2/CloudflareSpeedTest/releases/download/" + CFSpeedTestVersion + "/"
	// DefaultBinDirName 二进制存放目录名称
	DefaultBinDirName = "cloudflare-speedtest"
	// DefaultIPListFile IP 列表文件
	DefaultIPListFile = "ip.txt"
	// DefaultResultFile 测速结果文件
	DefaultResultFile = "result.csv"
	// DefaultTestInterval 默认测速间隔 (24小时)
	DefaultTestInterval = 24 * time.Hour
)

var (
	// bestIPs 最优 IP 列表
	bestIPs   []string
	bestIPsMu sync.RWMutex

	// lastSpeedTestTime 上次测速时间
	lastSpeedTestTime time.Time

	// binDir CloudflareSpeedTest 二进制目录
	binDir = defaultBinDir()

	// testInterval 测速间隔
	testInterval = DefaultTestInterval

	lastSpeedTestError string
	speedTestRunning   bool

	prepareMu      sync.Mutex
	speedTestRunMu sync.Mutex
	backgroundOnce sync.Once
)

var ErrSpeedTestRunning = errors.New("CloudflareSpeedTest 正在运行")

// ErrSpeedTestDisabled 表示 CloudflareSpeedTest 通过 CLOUDFLARE_SPEED_TEST 环境变量禁用。
var ErrSpeedTestDisabled = errors.New("CloudflareSpeedTest 已禁用（设置 CLOUDFLARE_SPEED_TEST=true 启用）")

// IsEnabled 报告 CloudflareSpeedTest 是否启用。
// 默认禁用。当 App Store CDN 不在 Cloudflare（例如 Deno Deploy）时，
// IP 优选既无效也浪费 CPU/网络，应保持禁用。
func IsEnabled() bool {
	v := strings.TrimSpace(os.Getenv("CLOUDFLARE_SPEED_TEST"))
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

func defaultBinDir() string {
	if custom := strings.TrimSpace(os.Getenv("CF_SPEEDTEST_BIN_DIR")); custom != "" {
		return custom
	}
	return filepath.Join(settings.GetDataDir(), DefaultBinDirName)
}

func ensureWritableBinDir() (string, error) {
	candidates := make([]string, 0, 3)
	push := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		for _, existing := range candidates {
			if existing == path {
				return
			}
		}
		candidates = append(candidates, path)
	}

	push(binDir)
	push(filepath.Join(settings.GetDataDir(), DefaultBinDirName))
	push(filepath.Join(os.TempDir(), "tradis", DefaultBinDirName))

	var lastErr error
	for _, candidate := range candidates {
		if err := os.MkdirAll(candidate, 0755); err != nil {
			lastErr = err
			continue
		}
		probeFile := filepath.Join(candidate, ".write-test")
		if err := os.WriteFile(probeFile, []byte("ok"), 0644); err != nil {
			lastErr = err
			continue
		}
		_ = os.Remove(probeFile)
		if binDir != candidate {
			logging.Debug("CloudflareSpeedTest selected writable work directory")
			bestIPsMu.Lock()
			lastSpeedTestError = fmt.Sprintf("CloudflareSpeedTest 工作目录已回退到 %s", candidate)
			bestIPsMu.Unlock()
		}
		binDir = candidate
		return candidate, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("未找到可写目录")
	}
	return "", lastErr
}

// SpeedTestResult 测速结果
type SpeedTestResult struct {
	IPs      []string  // 最优 IP 列表
	TestedAt time.Time // 测速时间
	Latency  string    // 延迟 (可选)
	Speed    string    // 带宽 (可选)
}

type Status struct {
	Enabled         bool      `json:"enabled"`
	Running         bool      `json:"running"`
	BinPath         string    `json:"binPath"`
	BinExists       bool      `json:"binExists"`
	ResultPath      string    `json:"resultPath"`
	ResultExists    bool      `json:"resultExists"`
	BestIP          string    `json:"bestIp"`
	BestIPs         []string  `json:"bestIps"`
	LastTestTime    time.Time `json:"lastTestTime"`
	TestIntervalSec int64     `json:"testIntervalSec"`
	NeedsRetest     bool      `json:"needsRetest"`
	LastError       string    `json:"lastError"`
}

// Config 配置
type Config struct {
	BinDir       string        // 二进制目录
	TestInterval time.Duration // 测速间隔
	DownloadURL  string        // 下载地址
}

// SetConfig 设置配置 (应在 init 前调用)
func SetConfig(c Config) {
	if c.BinDir != "" {
		binDir = c.BinDir
	}
	if c.TestInterval > 0 {
		testInterval = c.TestInterval
	}
}

// GetBinPath 获取 cfst 二进制路径
func GetBinPath() string {
	dir, err := ensureWritableBinDir()
	if err != nil {
		dir = binDir
	}
	return filepath.Join(dir, "cfst")
}

// GetIPListPath 获取 IP 列表文件路径
func GetIPListPath() string {
	dir, err := ensureWritableBinDir()
	if err != nil {
		dir = binDir
	}
	return filepath.Join(dir, DefaultIPListFile)
}

// GetResultPath 获取测速结果文件路径
func GetResultPath() string {
	dir, err := ensureWritableBinDir()
	if err != nil {
		dir = binDir
	}
	return filepath.Join(dir, DefaultResultFile)
}

// GetBestIP 获取当前最优 IP (第一个)
func GetBestIP() string {
	if !IsEnabled() {
		return ""
	}
	bestIPsMu.RLock()
	defer bestIPsMu.RUnlock()
	if len(bestIPs) == 0 {
		return ""
	}
	return bestIPs[0]
}

// GetBestIPs 获取最优 IP 列表
func GetBestIPs() []string {
	if !IsEnabled() {
		return nil
	}
	bestIPsMu.RLock()
	defer bestIPsMu.RUnlock()
	if bestIPs == nil {
		return nil
	}
	result := make([]string, len(bestIPs))
	copy(result, bestIPs)
	return result
}

// ReportIPFailure 将请求失败的优选 IP 从当前候选中移除。
// 候选耗尽后自动触发重测；缓存文件同时删除，避免容器重启后重新加载失效结果。
func ReportIPFailure(ip string) {
	if !IsEnabled() {
		return
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return
	}

	bestIPsMu.Lock()
	index := -1
	for i, candidate := range bestIPs {
		if candidate == ip {
			index = i
			break
		}
	}
	if index < 0 {
		bestIPsMu.Unlock()
		return
	}
	bestIPs = append(bestIPs[:index], bestIPs[index+1:]...)
	remaining := len(bestIPs)
	lastSpeedTestError = fmt.Sprintf("优选 IP %s 请求失败，已从候选中移除", ip)
	bestIPsMu.Unlock()

	_ = os.Remove(GetResultPath())
	logging.Debug("CloudflareSpeedTest candidate invalidated", "remaining", remaining)
	if remaining == 0 {
		runSpeedTestAsync("优选 IP 失效重测", true)
	}
}

func GetStatus(enabled bool) Status {
	if !IsEnabled() {
		return Status{
			Enabled:   enabled,
			LastError: "CloudflareSpeedTest 已禁用（CLOUDFLARE_SPEED_TEST 未设置为 true），CDN 请求走域名直连",
		}
	}
	bestIPsMu.RLock()
	defer bestIPsMu.RUnlock()

	bestIP := ""
	if len(bestIPs) > 0 {
		bestIP = bestIPs[0]
	}

	result := make([]string, len(bestIPs))
	copy(result, bestIPs)

	_, binErr := os.Stat(GetBinPath())
	_, resultErr := os.Stat(GetResultPath())

	return Status{
		Enabled:         enabled,
		Running:         speedTestRunning,
		BinPath:         GetBinPath(),
		BinExists:       binErr == nil,
		ResultPath:      GetResultPath(),
		ResultExists:    resultErr == nil,
		BestIP:          bestIP,
		BestIPs:         result,
		LastTestTime:    lastSpeedTestTime,
		TestIntervalSec: int64(testInterval.Seconds()),
		NeedsRetest:     enabled && (len(bestIPs) == 0 || time.Since(lastSpeedTestTime) > testInterval),
		LastError:       lastSpeedTestError,
	}
}

func setSpeedTestRunning(running bool) {
	bestIPsMu.Lock()
	speedTestRunning = running
	bestIPsMu.Unlock()
}

// IsSpeedTestNeeded 是否需要重新测速
func IsSpeedTestNeeded() bool {
	bestIPsMu.RLock()
	hasIPs := len(bestIPs) > 0
	bestIPsMu.RUnlock()

	if !hasIPs {
		return true
	}

	// 检查是否过期
	if time.Since(lastSpeedTestTime) > testInterval {
		return true
	}

	return false
}

// EnsureSpeedTestBin 确保 CloudflareSpeedTest 二进制已准备好。
// 下载过程串行执行，允许启动初始化和配置变更同时触发而不会重复下载。
func EnsureSpeedTestBin() error {
	if _, err := os.Stat(GetBinPath()); err == nil {
		return nil
	}

	prepareMu.Lock()
	defer prepareMu.Unlock()

	if _, err := os.Stat(GetBinPath()); err == nil {
		return nil
	}
	return DownloadSpeedTestBin()
}

func speedTestArchiveArch(goarch string) (string, error) {
	switch goarch {
	case "amd64", "386", "arm64":
		return goarch, nil
	case "arm":
		// TRADIS 的 32 位 ARM 镜像目标为 linux/arm/v7。
		return "armv7", nil
	default:
		return "", fmt.Errorf("不支持的系统架构: %s", goarch)
	}
}

// DownloadSpeedTestBin 下载 CloudflareSpeedTest 二进制
func DownloadSpeedTestBin() error {
	workDir, err := ensureWritableBinDir()
	if err != nil {
		bestIPsMu.Lock()
		lastSpeedTestError = fmt.Sprintf("准备 CloudflareSpeedTest 目录失败: %v", err)
		bestIPsMu.Unlock()
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 确定架构
	arch, err := speedTestArchiveArch(runtime.GOARCH)
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("cfst_linux_%s.tar.gz", arch)
	downloadURL := DefaultDownloadURL + filename

	logging.Info("CloudflareSpeedTest binary download started")

	// 下载
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("下载返回状态码: %d", resp.StatusCode)
	}

	// 保存到临时文件
	tmpFile := filepath.Join(os.TempDir(), "cfst.tar.gz")
	out, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer os.Remove(tmpFile)

	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return fmt.Errorf("保存文件失败: %w", err)
	}

	// 解压到 binDir
	cmd := exec.Command("tar", "-xzf", tmpFile, "-C", workDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}

	// 设置执行权限
	binPath := GetBinPath()
	if err := os.Chmod(binPath, 0755); err != nil {
		return fmt.Errorf("设置执行权限失败: %w", err)
	}

	logging.Info("CloudflareSpeedTest binary download completed")
	return nil
}

// UpdateIPList 更新 IP 列表 (从 CloudflareSpeedTest 官方获取)
func UpdateIPList() error {
	if _, err := ensureWritableBinDir(); err != nil {
		return fmt.Errorf("准备 CloudflareSpeedTest 目录失败: %w", err)
	}

	// 如果已经下载了 cfst，可以用它的 ip.txt
	// 否则直接下载官方 ip.txt
	ipListURL := "https://raw.githubusercontent.com/XIU2/CloudflareSpeedTest/master/ip.txt"

	resp, err := http.Get(ipListURL)
	if err != nil {
		return fmt.Errorf("下载 IP 列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("下载 IP 列表返回状态码: %d", resp.StatusCode)
	}

	// 保存
	out, err := os.Create(GetIPListPath())
	if err != nil {
		return fmt.Errorf("保存 IP 列表失败: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("写入 IP 列表失败: %w", err)
	}

	logging.Debug("CloudflareSpeedTest candidate list updated")
	return nil
}

// RunSpeedTest 运行测速
// 返回最优 IP 列表
func RunSpeedTest() (*SpeedTestResult, error) {
	if !IsEnabled() {
		return nil, ErrSpeedTestDisabled
	}
	if !speedTestRunMu.TryLock() {
		return nil, ErrSpeedTestRunning
	}
	defer speedTestRunMu.Unlock()

	setSpeedTestRunning(true)
	defer setSpeedTestRunning(false)

	return runSpeedTest()
}

func runSpeedTest() (*SpeedTestResult, error) {
	binPath := GetBinPath()

	// 检查二进制是否存在
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		if err := EnsureSpeedTestBin(); err != nil {
			return nil, fmt.Errorf("下载 CloudflareSpeedTest 失败: %w", err)
		}
	}

	// 更新 IP 列表
	if err := UpdateIPList(); err != nil {
		logging.Warn("CloudflareSpeedTest candidate list update failed; using existing list", "error", err)
	}

	// 运行测速
	// 参数说明:
	// - f ip.txt 指定 IP 列表文件
	// - o result.csv 输出结果
	// - dd 不下载测速文件 (只测延迟和带宽)
	// - t 延迟测速次数 (默认 10)
	// - p 打印进度
	cmd := exec.Command(binPath,
		"-f", GetIPListPath(),
		"-o", GetResultPath(),
		"-dd",     // 不下载测速
		"-t", "5", // 减少测速次数加快速度
	)

	logging.Info("CloudflareSpeedTest started")
	start := time.Now()

	output, err := cmd.CombinedOutput()
	if err != nil {
		logging.Warn("CloudflareSpeedTest command failed; attempting existing result", "error", err)
		// 不返回错误，继续尝试解析现有结果
	}

	logging.Info("CloudflareSpeedTest completed", "duration_ms", time.Since(start).Milliseconds())

	// 解析结果
	ips, err := parseResultCSV(GetResultPath())
	if err != nil {
		bestIPsMu.Lock()
		if os.IsNotExist(err) {
			if strings.TrimSpace(string(output)) != "" {
				lastSpeedTestError = fmt.Sprintf("测速命令已执行但未生成结果文件：%s", strings.TrimSpace(string(output)))
			} else {
				lastSpeedTestError = "测速命令已执行但结果文件尚未生成"
			}
		} else {
			lastSpeedTestError = err.Error()
		}
		bestIPsMu.Unlock()
		return nil, fmt.Errorf("解析测速结果失败: %w", err)
	}

	// 更新全局状态
	bestIPsMu.Lock()
	bestIPs = ips
	lastSpeedTestTime = time.Now()
	lastSpeedTestError = ""
	bestIPsMu.Unlock()

	return &SpeedTestResult{
		IPs:      ips,
		TestedAt: lastSpeedTestTime,
	}, nil
}

// RefreshSpeedTestAsync 废弃当前缓存的优选 IP，并在后台强制重新测速。
// 清空旧结果后，测速完成前 CDN 请求会安全回退到域名，避免继续使用失效 IP。
func RefreshSpeedTestAsync(reason string) error {
	if !IsEnabled() {
		return ErrSpeedTestDisabled
	}
	if !isCDNConfigured() {
		return fmt.Errorf("应用商店 CDN 未配置")
	}
	if !speedTestRunMu.TryLock() {
		return ErrSpeedTestRunning
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "手动刷新"
	}

	bestIPsMu.Lock()
	bestIPs = nil
	lastSpeedTestTime = time.Time{}
	lastSpeedTestError = fmt.Sprintf("%s，正在执行 CloudflareSpeedTest", reason)
	speedTestRunning = true
	bestIPsMu.Unlock()
	_ = os.Remove(GetResultPath())

	go func() {
		defer speedTestRunMu.Unlock()
		defer setSpeedTestRunning(false)

		result, err := runSpeedTest()
		if err != nil {
			bestIPsMu.Lock()
			lastSpeedTestError = err.Error()
			bestIPsMu.Unlock()
			logging.Warn("CloudflareSpeedTest background run failed", "reason", reason, "error", err)
			system.LogSimpleEvent("warning", fmt.Sprintf("Cloudflare %s失败：%s", reason, logging.RedactText(err.Error())))
			return
		}

		logging.Info("CloudflareSpeedTest background run completed", "reason", reason, "candidate_count", len(result.IPs))
		if len(result.IPs) > 0 {
			system.LogSimpleEvent("success", fmt.Sprintf("Cloudflare %s完成，获得 %d 个候选地址", reason, len(result.IPs)))
		}
	}()

	return nil
}

// parseResultCSV 解析测速结果 CSV
func parseResultCSV(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ips []string
	reader := csv.NewReader(file)

	// 跳过第一行 (标题行)
	if _, err := reader.Read(); err != nil {
		return nil, err
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		if len(record) < 1 {
			continue
		}
		ip := strings.TrimSpace(record[0])
		if ip != "" {
			ips = append(ips, ip)
		}
	}

	return ips, nil
}

// LoadCachedResult 从已有文件加载缓存结果
func LoadCachedResult() error {
	resultPath := GetResultPath()

	info, err := os.Stat(resultPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("结果文件不存在")
	}
	if err != nil {
		return err
	}

	ips, err := parseResultCSV(resultPath)
	if err != nil {
		bestIPsMu.Lock()
		lastSpeedTestError = err.Error()
		bestIPsMu.Unlock()
		return err
	}

	bestIPsMu.Lock()
	bestIPs = ips
	lastSpeedTestTime = info.ModTime()
	lastSpeedTestError = ""
	bestIPsMu.Unlock()

	logging.Debug("CloudflareSpeedTest cached result loaded", "candidate_count", len(ips))
	return nil
}

func isCDNConfigured() bool {
	s, err := settings.GetSettings()
	if err != nil || strings.TrimSpace(s.AppStoreCDNURL) == "" {
		return false
	}
	_, repository, sourceErr := normalizeTemplateRepository(s.AppStoreCDNURL)
	return !repository && sourceErr == nil
}

func runSpeedTestAsync(reason string, force bool) {
	if !isCDNConfigured() || (!force && !IsSpeedTestNeeded()) {
		return
	}

	bestIPsMu.Lock()
	lastSpeedTestError = fmt.Sprintf("%s，正在执行 CloudflareSpeedTest", reason)
	bestIPsMu.Unlock()

	go func() {
		result, err := RunSpeedTest()
		if errors.Is(err, ErrSpeedTestRunning) {
			return
		}
		if err != nil {
			bestIPsMu.Lock()
			lastSpeedTestError = err.Error()
			bestIPsMu.Unlock()
			logging.Warn("CloudflareSpeedTest background run failed", "reason", reason, "error", err)
			system.LogSimpleEvent("warning", fmt.Sprintf("Cloudflare %s失败：%s", reason, logging.RedactText(err.Error())))
			return
		}

		logging.Info("CloudflareSpeedTest background run completed", "reason", reason, "candidate_count", len(result.IPs))
		if len(result.IPs) > 0 {
			system.LogSimpleEvent("success", fmt.Sprintf("Cloudflare %s完成，获得 %d 个候选地址", reason, len(result.IPs)))
		}
	}()
}

func shouldTriggerForCDNChange(previousURL, currentURL string) bool {
	previousURL = strings.TrimRight(strings.TrimSpace(previousURL), "/")
	currentURL = strings.TrimRight(strings.TrimSpace(currentURL), "/")
	_, repository, err := normalizeTemplateRepository(currentURL)
	return currentURL != "" && previousURL != currentURL && !repository && err == nil
}

// NotifyCDNURLChanged 在有效 CDN 地址新增或变化时异步触发一次重新测速。
func NotifyCDNURLChanged(previousURL, currentURL string) {
	if !IsEnabled() {
		return
	}
	if !shouldTriggerForCDNChange(previousURL, currentURL) {
		return
	}
	logging.Info("AppStore CDN configuration changed; scheduling CloudflareSpeedTest")
	runSpeedTestAsync("CDN 配置变更重测", true)
}

// StartBackgroundSpeedTest 启动二进制后台准备与单例定时测速服务。
func StartBackgroundSpeedTest() {
	if !IsEnabled() {
		logging.Debug("CloudflareSpeedTest background service disabled")
		return
	}
	backgroundOnce.Do(func() {
		// 二进制准备与 CDN 配置解耦：服务启动后立即在后台下载。
		go func() {
			if err := EnsureSpeedTestBin(); err != nil {
				bestIPsMu.Lock()
				lastSpeedTestError = err.Error()
				bestIPsMu.Unlock()
				logging.Warn("CloudflareSpeedTest binary preparation failed", "error", err)
				return
			}
			logging.Debug("CloudflareSpeedTest binary ready")
		}()

		if err := LoadCachedResult(); err != nil {
			logging.Debug("CloudflareSpeedTest cache unavailable", "error", err)
		}
		runSpeedTestAsync("优选 IP 初始化", false)

		go func() {
			checkInterval := time.Hour
			if testInterval < checkInterval {
				checkInterval = testInterval
			}
			ticker := time.NewTicker(checkInterval)
			defer ticker.Stop()

			for range ticker.C {
				runSpeedTestAsync("定时测速", false)
			}
		}()
	})
}

// ReadIPList 读取 IP 列表文件
func ReadIPList() ([]string, error) {
	file, err := os.Open(GetIPListPath())
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ips []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			ips = append(ips, line)
		}
	}
	return ips, scanner.Err()
}
