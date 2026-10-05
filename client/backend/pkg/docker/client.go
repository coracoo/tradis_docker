package docker

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// 新增Client结构体封装Docker客户端
type Client struct {
	*client.Client
	fromPool bool // 标记是否来自连接池
}

func (c *Client) PruneVolumes(ctx context.Context) (types.VolumesPruneReport, error) {
	// 使用原生的 Docker SDK 方法
	return c.Client.VolumesPrune(ctx, filters.NewArgs())
}

// NewStreamingClient 创建专用流式客户端（日志/统计等长连接）
// 流式连接会长期占用一个 Docker 连接（follow=true 的日志流不会自行结束），
// 若从共享连接池（默认最多 10 个）获取，几个同时打开的日志视图就会占满连接池，
// 导致其它 API 与新的日志视图报"连接池已满"。因此流式连接绕过连接池单独创建，
// 由调用方在流结束时通过 Close 关闭（fromPool=false 会直接关闭底层连接）。
func NewStreamingClient() (*Client, error) {
	return createClient()
}

// NewDockerClient 获取Docker客户端
// 优先从连接池获取，如果连接池未初始化则直接创建
func NewDockerClient() (*Client, error) {
	return NewDockerClientWithContext(context.Background())
}

func NewDockerClientWithContext(ctx context.Context) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 尝试从连接池获取
	if globalPool != nil && globalPool.initialized {
		return globalPool.GetContext(ctx)
	}

	// 回退：直接创建新连接
	return createClient()
}

// CloseClient 归还或关闭客户端
// 如果是从连接池获取的，归还到池中；否则直接关闭
func CloseClient(cli *Client) {
	if cli == nil {
		return
	}

	_ = cli.Close()
}

// Close 关闭或归还客户端
// 如果客户端来自连接池，归还到池中；否则直接关闭
func (cli *Client) Close() error {
	if cli == nil {
		return nil
	}

	// 如果来自连接池，归还到池中
	if cli.fromPool && globalPool != nil && globalPool.initialized {
		globalPool.Put(cli)
		return nil
	}

	// 否则直接关闭
	return cli.Client.Close()
}
