package system

import (
	"bufio"
	"context"
	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/settings"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
)

var (
	mutex sync.Mutex
	// 近期事件ID缓存，用于去重
	recentIDs = make(map[string]int64)
)

func getLogFilePath() string {
	return filepath.Join(settings.GetDataDir(), "docker.logs")
}

// LogEntry 定义日志条目结构
type LogEntry struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	TypeClass string `json:"typeClass"`
	Time      string `json:"time"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// StartEventLogger 启动事件监听和日志记录
func StartEventLogger() {
	// 确保目录存在
	logPath := getLogFilePath()
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("[ERROR] 创建日志目录失败: %v\n", err)
	}
	if info, err := os.Stat(logDir); err == nil && !info.IsDir() {
		log.Printf("[WARN] 日志目录路径存在但不是目录: %s\n", logDir)
		return
	}
	log.Printf("[INFO] 事件日志文件路径: %s\n", logPath)

	// 初始填充历史日志
	fillHistoryLogs()

	// 启动实时监听
	go func() {
		for {
			watchEvents()
			// 如果连接断开，等待一段时间后重试
			time.Sleep(5 * time.Second)
		}
	}()
}

func fillHistoryLogs() {
	// 确保日志文件存在
	if _, err := os.Stat(getLogFilePath()); os.IsNotExist(err) {
		f, cerr := os.OpenFile(getLogFilePath(), os.O_CREATE|os.O_WRONLY, 0644)
		if cerr == nil {
			f.Close()
		}
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("[ERROR] Error creating docker client: %v\n", err)
		return
	}
	defer cli.Close()

	// 获取最近24小时的事件
	msgs, errs := cli.Events(context.Background(), types.EventsOptions{
		Since: time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
		Until: time.Now().Format(time.RFC3339),
	})

	// 收集所有历史事件
	var events []events.Message
loop:
	for {
		select {
		case event := <-msgs:
			events = append(events, event)
		case err := <-errs:
			if err != nil && err != io.EOF {
				log.Printf("[ERROR] Error reading history events: %v\n", err)
			}
			break loop
		case <-time.After(5 * time.Second):
			break loop
		}
	}

	// 写入文件
	for _, event := range events {
		processEvent(event)
	}

	// 如果没有历史事件，写入初始化记录，确保文件不为空
	if len(events) == 0 {
		entry := LogEntry{
			ID:        fmt.Sprintf("init-%d", time.Now().UnixNano()),
			Type:      "info",
			TypeClass: "info",
			Time:      time.Now().Format("2006-01-02 15:04"),
			Message:   "docker events logger initialized",
			Timestamp: time.Now().Unix(),
		}
		appendLog(entry)
	}
}

func watchEvents() {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("[ERROR] Error creating docker client: %v\n", err)
		return
	}
	defer cli.Close()

	// 从现在开始监听
	msgs, errs := cli.Events(context.Background(), types.EventsOptions{
		Since: time.Now().Format(time.RFC3339),
	})

	for {
		select {
		case event := <-msgs:
			processEvent(event)
		case err := <-errs:
			if err != nil {
				log.Printf("[ERROR] Error reading docker events: %v\n", err)
				return
			}
		}
	}
}

func processEvent(event events.Message) {
	// 只记录关心的事件类型
	if event.Type != "container" {
		return
	}

	// 过滤逻辑
	eventType := "info"
	typeClass := "info"

	switch event.Action {
	case "start", "create", "unpause":
		eventType = "success"
		typeClass = "success"
	case "stop", "die", "kill", "pause", "oom":
		eventType = "warning"
		typeClass = "warning"
	case "destroy", "delete":
		eventType = "error"
		typeClass = "danger"
	default:
		// 忽略其他事件以减少噪音
		return
	}

	timeStr := time.Unix(event.Time, 0).Format("2006-01-02 15:04")
	name := event.Actor.Attributes["name"]
	if name == "" && len(event.ID) >= 12 {
		name = event.ID[:12]
	}

	entry := LogEntry{
		ID:        fmt.Sprintf("%s-%d", event.ID, event.TimeNano),
		Type:      eventType,
		TypeClass: typeClass,
		Time:      timeStr,
		Message:   fmt.Sprintf("%s %s: %s", event.Type, event.Action, name),
		Timestamp: event.Time,
	}

	appendLog(entry)
}

func appendLog(entry LogEntry) {
	mutex.Lock()
	defer mutex.Unlock()

	entry.Message = logging.RedactText(settings.RedactAppStoreURL(entry.Message))

	// 去重：同一事件ID不重复写入
	if _, exists := recentIDs[entry.ID]; exists {
		return
	}
	recentIDs[entry.ID] = time.Now().Unix()
	// 简单清理：保留最多1000个，移除超过5分钟的旧记录
	if len(recentIDs) > 1000 {
		cutoff := time.Now().Add(-5 * time.Minute).Unix()
		for k, ts := range recentIDs {
			if ts < cutoff {
				delete(recentIDs, k)
			}
		}
	}

	path := getLogFilePath()
	dir := filepath.Dir(path)

	// 确保目录存在
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[ERROR] 创建日志目录失败: %v\n", err)
		return
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		log.Printf("[WARN] 日志目录路径存在但不是目录: %s\n", dir)
		return
	}

	// 检查文件大小，如果超过 5MB 则轮转
	if info, err := os.Stat(path); err == nil && info.Size() > 5*1024*1024 {
		_ = os.Rename(path, path+".1")
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[ERROR] 打开日志文件失败，路径: %s，错误: %v\n", path, err)
		return
	}
	defer file.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	_, _ = file.Write(data)
	_, _ = file.WriteString("\n")
}

// GetRecentLogs 从文件读取最近的日志
func GetRecentLogs(limit int) ([]LogEntry, error) {
	mutex.Lock()
	defer mutex.Unlock()

	var allLogs []LogEntry
	path := getLogFilePath()

	// 读取主日志文件
	readLogsFromFile(path, &allLogs)

	// 如果不够，且有备份文件，也读取备份文件
	if len(allLogs) < limit {
		readLogsFromFile(path+".1", &allLogs)
	}

	// 按时间戳排序（因为读取多个文件可能顺序不对，或者文件中可能有乱序）
	sort.Slice(allLogs, func(i, j int) bool {
		return allLogs[i].Timestamp > allLogs[j].Timestamp
	})

	if len(allLogs) > limit {
		allLogs = allLogs[:limit]
	}

	return allLogs, nil
}

func readLogsFromFile(path string, logs *[]LogEntry) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			*logs = append(*logs, entry)
		}
	}
}

func LogSimpleEvent(eventType string, message string) {
	t := strings.ToLower(eventType)
	typeClass := "info"
	switch t {
	case "success":
		typeClass = "success"
	case "warning":
		typeClass = "warning"
	case "error":
		typeClass = "danger"
	default:
		t = "info"
		typeClass = "info"
	}
	entry := LogEntry{
		ID:        fmt.Sprintf("custom-%d", time.Now().UnixNano()),
		Type:      t,
		TypeClass: typeClass,
		Time:      time.Now().Format("2006-01-02 15:04"),
		Message:   message,
		Timestamp: time.Now().Unix(),
	}
	appendLog(entry)
}
