package docker

import (
	"context"
	"os"
	"time"
)

// TimeoutConfig 定义不同操作的超时时间
type TimeoutConfig struct {
	Short   time.Duration // 快速操作 (List等)
	Medium  time.Duration // 一般操作 (Inspect等)
	Long    time.Duration // 耗时操作 (Pull, Build等)
	Stream  time.Duration // 流式操作 (Logs, Stats等)
	Cleanup time.Duration // Docker 磁盘评估
}

var defaultTimeouts = TimeoutConfig{
	Short:   5 * time.Second,
	Medium:  10 * time.Second,
	Long:    5 * time.Minute,
	Stream:  30 * time.Second, // 流式操作的初始连接超时
	Cleanup: 90 * time.Second,
}

// GetTimeoutConfig 获取超时配置
// 可以通过环境变量覆盖默认值
func GetTimeoutConfig() TimeoutConfig {
	cfg := defaultTimeouts

	// 从环境变量读取自定义配置
	if v := getDurationEnv("DOCKER_TIMEOUT_SHORT"); v > 0 {
		cfg.Short = v
	}
	if v := getDurationEnv("DOCKER_TIMEOUT_MEDIUM"); v > 0 {
		cfg.Medium = v
	}
	if v := getDurationEnv("DOCKER_TIMEOUT_LONG"); v > 0 {
		cfg.Long = v
	}
	if v := getDurationEnv("DOCKER_TIMEOUT_STREAM"); v > 0 {
		cfg.Stream = v
	}
	if v := getDurationEnv("DOCKER_TIMEOUT_CLEANUP"); v > 0 {
		cfg.Cleanup = v
	}

	return cfg
}

// WithShortTimeout 创建短超时上下文
func WithShortTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), GetTimeoutConfig().Short)
}

// WithShortTimeoutFrom 创建基于父上下文的短超时上下文。
// 用于在请求处理器中派生超时上下文，使客户端断开能取消 Docker 调用。
func WithShortTimeoutFrom(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, GetTimeoutConfig().Short)
}

// WithMediumTimeout 创建中等超时上下文
func WithMediumTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), GetTimeoutConfig().Medium)
}

// WithMediumTimeoutFrom 创建基于父上下文的中等超时上下文。
func WithMediumTimeoutFrom(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, GetTimeoutConfig().Medium)
}

// WithLongTimeout 创建长超时上下文
func WithLongTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), GetTimeoutConfig().Long)
}

// WithStreamTimeout 创建流式操作超时上下文
func WithStreamTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), GetTimeoutConfig().Stream)
}

// WithCleanupTimeoutFrom 创建 Docker 磁盘评估上下文。
func WithCleanupTimeoutFrom(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, GetTimeoutConfig().Cleanup)
}

// WithTimeout 创建指定超时的上下文
func WithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// WithTimeoutFrom 创建基于父上下文的指定超时上下文。
func WithTimeoutFrom(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

// getDurationEnv 从环境变量读取时间间隔
func getDurationEnv(key string) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 0
}
