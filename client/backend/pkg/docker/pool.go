package docker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/client"
)

// Pool 是一个线程安全的Docker客户端连接池
type Pool struct {
	mu          sync.RWMutex
	clients     []*Client
	maxSize     int
	initialized bool
	closed      bool
	done        chan struct{}
	available   chan struct{}
	// 统计信息
	stats PoolStats
}

// PoolStats 连接池统计信息
type PoolStats struct {
	TotalCreated  int64
	TotalReused   int64
	TotalClosed   int64
	CurrentActive int64
	CurrentIdle   int64
	GetFailures   int64
}

var (
	// 全局连接池实例
	globalPool *Pool
	once       sync.Once
)

// ErrPoolBusy indicates that no client became available within the acquisition budget.
var ErrPoolBusy = errors.New("Docker connection pool is busy")

const poolAcquireTimeout = 5 * time.Second

// InitPool 初始化全局连接池
func InitPool() error {
	var initErr error
	once.Do(func() {
		maxSize := 10
		if env := os.Getenv("DOCKER_POOL_MAX_SIZE"); env != "" {
			if n, err := strconv.Atoi(env); err == nil && n > 0 {
				maxSize = n
			}
		}

		globalPool = &Pool{
			clients: make([]*Client, 0, maxSize),
			maxSize: maxSize,
			done:    make(chan struct{}),
		}

		// 预热：创建初始连接
		minIdle := 2
		if env := os.Getenv("DOCKER_POOL_MIN_IDLE"); env != "" {
			if n, err := strconv.Atoi(env); err == nil && n >= 0 {
				minIdle = n
			}
		}

		for i := 0; i < minIdle && i < maxSize; i++ {
			cli, err := createClient()
			if err != nil {
				initErr = fmt.Errorf("预热连接池失败: %w", err)
				return
			}
			globalPool.clients = append(globalPool.clients, cli)
			globalPool.stats.TotalCreated++
			globalPool.stats.CurrentIdle++
		}

		globalPool.initialized = true
		log.Printf("[DockerPool] 初始化完成，最大连接数: %d, 初始连接: %d", maxSize, len(globalPool.clients))

		// 启动后台清理协程
		go globalPool.cleanupLoop()
	})

	return initErr
}

// GetPool 获取全局连接池实例
func GetPool() (*Pool, error) {
	if globalPool == nil || !globalPool.initialized {
		if err := InitPool(); err != nil {
			return nil, err
		}
	}
	return globalPool, nil
}

// Get 从连接池获取一个客户端，容量不足时限时等待。
func (p *Pool) Get() (*Client, error) {
	return p.GetContext(context.Background())
}

func (p *Pool) GetContext(parent context.Context) (*Client, error) {
	ctx, cancel := context.WithTimeout(parent, poolAcquireTimeout)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return nil, p.acquireError(parent, err)
		}
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, fmt.Errorf("Docker connection pool is closed")
		}
		if len(p.clients) > 0 {
			cli := p.clients[len(p.clients)-1]
			p.clients = p.clients[:len(p.clients)-1]
			p.stats.CurrentIdle--
			p.stats.CurrentActive++
			p.mu.Unlock()
			healthy := isHealthyContext(ctx, cli)
			if err := ctx.Err(); err != nil {
				p.Put(cli)
				return nil, p.acquireError(parent, err)
			}
			p.mu.Lock()
			if healthy && !p.closed {
				p.stats.TotalReused++
				cli.fromPool = true
				p.mu.Unlock()
				return cli, nil
			}
			p.stats.CurrentActive--
			p.stats.TotalClosed++
			p.notifyAvailableLocked()
			p.mu.Unlock()
			if cli != nil && cli.Client != nil {
				_ = cli.Client.Close()
			}
			continue
		}
		// Closed clients do not consume capacity, even after many replacements.
		if p.stats.TotalCreated-p.stats.TotalClosed < int64(p.maxSize) {
			cli, err := createClient()
			if err != nil {
				p.stats.GetFailures++
				p.mu.Unlock()
				return nil, fmt.Errorf("创建Docker客户端失败: %w", err)
			}
			p.stats.TotalCreated++
			p.stats.CurrentActive++
			cli.fromPool = true
			p.mu.Unlock()
			return cli, nil
		}
		if p.available == nil {
			p.available = make(chan struct{})
		}
		available := p.available
		p.mu.Unlock()
		select {
		case <-available:
		case <-ctx.Done():
			return nil, p.acquireError(parent, ctx.Err())
		}
	}
}

func (p *Pool) acquireError(parent context.Context, err error) error {
	p.mu.Lock()
	p.stats.GetFailures++
	p.mu.Unlock()
	if parent.Err() != nil {
		return parent.Err()
	}
	return fmt.Errorf("%w: %w", ErrPoolBusy, err)
}

func (p *Pool) notifyAvailableLocked() {
	if p.available != nil {
		close(p.available)
		p.available = nil
	}
}

// Put 将客户端归还到连接池
// 归还不执行网络请求；健康检查在获取和后台清理时进行。
func (p *Pool) Put(cli *Client) {
	if cli == nil {
		return
	}

	p.mu.Lock()

	p.stats.CurrentActive--
	cli.fromPool = false // 重置标志

	// 池关闭后不再接收归还连接。
	if p.closed {
		p.stats.TotalClosed++
		p.notifyAvailableLocked()
		p.mu.Unlock()
		if cli.Client != nil {
			_ = cli.Client.Close()
		}
		return
	}

	// 如果池未满，放回队列
	if len(p.clients) < p.maxSize {
		p.clients = append(p.clients, cli)
		p.stats.CurrentIdle++
		p.notifyAvailableLocked()
		p.mu.Unlock()
	} else {
		// 池已满，直接关闭
		p.stats.TotalClosed++
		p.notifyAvailableLocked()
		p.mu.Unlock()
		_ = cli.Client.Close()
	}
}

// Stats 获取连接池统计信息
func (p *Pool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

// Close 关闭连接池中的所有连接
func (p *Pool) Close() error {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		p.notifyAvailableLocked()
		if p.done != nil {
			close(p.done)
		}
	}
	clients := p.clients
	p.clients = nil
	p.stats.TotalClosed += int64(len(clients))
	p.stats.CurrentIdle = 0
	closed := p.stats.TotalClosed
	p.mu.Unlock()
	for _, cli := range clients {
		if cli != nil && cli.Client != nil {
			_ = cli.Client.Close()
		}
	}
	log.Printf("[DockerPool] 连接池已关闭，共关闭 %d 个连接", closed)
	return nil
}

// createClient 创建新的Docker客户端
func createClient() (*Client, error) {
	dockerHost := strings.TrimSpace(os.Getenv("DOCKER_HOST"))
	if dockerHost == "" {
		dockerSock := strings.TrimSpace(os.Getenv("DOCKER_SOCK"))
		if dockerSock == "" {
			dockerSock = "/var/run/docker.sock"
		}
		if strings.HasPrefix(dockerSock, "unix://") {
			dockerHost = dockerSock
		} else {
			dockerHost = "unix://" + dockerSock
		}
	}

	opts := []client.Opt{client.WithAPIVersionNegotiation()}
	if strings.TrimSpace(os.Getenv("DOCKER_HOST")) != "" {
		opts = append(opts, client.FromEnv)
	} else {
		opts = append(opts, client.WithHost(dockerHost))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}
	return &Client{Client: cli, fromPool: false}, nil
}

// isHealthy 检查客户端连接是否健康
func isHealthy(cli *Client) bool {
	return isHealthyContext(context.Background(), cli)
}

func isHealthyContext(parent context.Context, cli *Client) bool {
	if cli == nil || cli.Client == nil {
		return false
	}
	// 使用Ping检查连接状态
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()

	_, err := cli.Ping(ctx)
	return err == nil
}

// cleanupLoop 定期清理后台任务
func (p *Pool) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.cleanupIdleConnections()
			p.logStats()
		}
	}
}

// cleanupIdleConnections 清理不健康的空闲连接
func (p *Pool) cleanupIdleConnections() {
	p.mu.Lock()
	clients := p.clients
	p.clients = nil
	p.stats.CurrentIdle = 0
	// Detached clients remain reserved while checked outside the lock.
	p.stats.CurrentActive += int64(len(clients))
	p.mu.Unlock()
	for _, cli := range clients {
		if isHealthy(cli) {
			p.Put(cli)
			continue
		}
		p.mu.Lock()
		p.stats.CurrentActive--
		p.stats.TotalClosed++
		p.notifyAvailableLocked()
		p.mu.Unlock()
		if cli != nil && cli.Client != nil {
			_ = cli.Client.Close()
		}
	}
}

// logStats 记录统计信息（调试用）
func (p *Pool) logStats() {
	if os.Getenv("DOCKER_POOL_DEBUG") != "true" {
		return
	}
	p.mu.RLock()
	stats := p.stats
	p.mu.RUnlock()

	log.Printf("[DockerPool] Stats: created=%d, reused=%d, closed=%d, active=%d, idle=%d, failures=%d",
		stats.TotalCreated, stats.TotalReused, stats.TotalClosed,
		stats.CurrentActive, stats.CurrentIdle, stats.GetFailures)
}

// WithClient 使用连接池执行操作
// 自动获取和归还客户端
func WithClient(ctx context.Context, fn func(*Client) error) error {
	pool, err := GetPool()
	if err != nil {
		return err
	}

	cli, err := pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer pool.Put(cli)

	return fn(cli)
}
