package docker

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type poolTestTransport func(*http.Request) (*http.Response, error)

func (transport poolTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport(req)
}

func poolTestClient(t *testing.T, ping func(*http.Request) error) *client.Client {
	t.Helper()
	cli, err := client.NewClientWithOpts(
		client.WithHost("tcp://docker.test:2375"), client.WithVersion("1.43"),
		client.WithHTTPClient(&http.Client{Transport: poolTestTransport(func(req *http.Request) (*http.Response, error) {
			if err := ping(req); err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
		})}),
	)
	require.NoError(t, err)
	return cli
}

func TestPoolHealthChecksDoNotHoldGlobalLock(t *testing.T) {
	for _, action := range []string{"get", "cleanup"} {
		t.Run(action, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var signal sync.Once
			underlying := poolTestClient(t, func(r *http.Request) error {
				signal.Do(func() { close(entered) })
				<-release
				return nil
			})
			cli := &Client{Client: underlying}
			pool := &Pool{maxSize: 1, clients: []*Client{cli}, stats: PoolStats{TotalCreated: 1, CurrentIdle: 1}}
			done := make(chan struct{})
			go func() {
				defer close(done)
				switch action {
				case "get":
					_, _ = pool.Get()
				default:
					pool.cleanupIdleConnections()
				}
			}()
			<-entered
			statsDone := make(chan struct{})
			go func() { _ = pool.Stats(); close(statsDone) }()
			blocked := false
			select {
			case <-statsDone:
			case <-time.After(100 * time.Millisecond):
				blocked = true
			}
			close(release)
			<-done
			<-statsDone
			_ = underlying.Close()
			require.False(t, blocked, "slow Docker Ping blocked unrelated pool operations")
			stats := pool.Stats()
			if action == "get" {
				require.Equal(t, int64(0), stats.CurrentIdle)
				require.Equal(t, int64(1), stats.CurrentActive)
			} else {
				require.Equal(t, int64(1), stats.CurrentIdle)
				require.Equal(t, int64(0), stats.CurrentActive)
			}
		})
	}
}

func TestPoolDropsDeadIdleWithoutNegativeCounts(t *testing.T) {
	underlying := poolTestClient(t, func(*http.Request) error { return assert.AnError })
	pool := &Pool{maxSize: 1, clients: []*Client{{Client: underlying}}, stats: PoolStats{TotalCreated: 1, CurrentIdle: 1}}
	got, err := pool.Get()
	require.NoError(t, err)
	stats := pool.Stats()
	require.Equal(t, int64(0), stats.CurrentIdle)
	require.Equal(t, int64(1), stats.CurrentActive)
	require.Equal(t, int64(1), stats.TotalClosed)
	require.NoError(t, pool.Close())
	pool.Put(got)
	require.Equal(t, int64(0), pool.Stats().CurrentActive)
	_, err = pool.Get()
	require.Error(t, err)
}

func TestPoolWaitsForReturnedClient(t *testing.T) {
	underlying := poolTestClient(t, func(*http.Request) error { return nil })
	held := &Client{Client: underlying, fromPool: true}
	pool := &Pool{maxSize: 1, stats: PoolStats{TotalCreated: 1, CurrentActive: 1}}
	defer pool.Close()
	type outcome struct {
		cli *Client
		err error
	}
	got := make(chan outcome, 1)
	go func() {
		cli, err := pool.Get()
		got <- outcome{cli, err}
	}()
	select {
	case result := <-got:
		pool.Put(held)
		t.Fatalf("full pool returned before a client was released: %v", result.err)
	case <-time.After(30 * time.Millisecond):
	}
	pool.Put(held)
	select {
	case result := <-got:
		require.NoError(t, result.err)
		pool.Put(result.cli)
	case <-time.After(time.Second):
		t.Fatal("returned client did not wake the waiting request")
	}
	require.Equal(t, int64(0), pool.Stats().CurrentActive)
	require.Equal(t, int64(1), pool.Stats().CurrentIdle)
}

func TestPoolReturnDoesNotPingDocker(t *testing.T) {
	var calls atomic.Int32
	underlying := poolTestClient(t, func(*http.Request) error { calls.Add(1); return nil })
	pool := &Pool{maxSize: 1, stats: PoolStats{TotalCreated: 1, CurrentActive: 1}}
	defer pool.Close()
	pool.Put(&Client{Client: underlying, fromPool: true})
	require.Equal(t, int32(0), calls.Load(), "returning a client must not wait for the daemon")
	require.Equal(t, int64(1), pool.Stats().CurrentIdle)
}

func TestPoolWaitCanBeCanceledOrClosed(t *testing.T) {
	for _, action := range []string{"cancel", "close"} {
		t.Run(action, func(t *testing.T) {
			pool := &Pool{maxSize: 1, stats: PoolStats{TotalCreated: 1, CurrentActive: 1}}
			defer pool.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := pool.GetContext(ctx); done <- err }()
			require.Eventually(t, func() bool {
				pool.mu.RLock()
				defer pool.mu.RUnlock()
				return pool.available != nil
			}, time.Second, time.Millisecond)
			if action == "cancel" {
				cancel()
			} else {
				require.NoError(t, pool.Close())
			}
			select {
			case err := <-done:
				if action == "cancel" {
					require.ErrorIs(t, err, context.Canceled)
				} else {
					require.ErrorContains(t, err, "closed")
				}
			case <-time.After(time.Second):
				t.Fatal("waiting request did not exit")
			}
			require.Equal(t, int64(1), pool.Stats().CurrentActive)
		})
	}
}

func TestPoolBusyHasBoundedWait(t *testing.T) {
	pool := &Pool{maxSize: 1, stats: PoolStats{TotalCreated: 1, CurrentActive: 1}}
	defer pool.Close()
	started := time.Now()
	_, err := pool.Get()
	require.ErrorIs(t, err, ErrPoolBusy)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), poolAcquireTimeout+time.Second)
	require.Equal(t, int64(1), pool.Stats().GetFailures)
}

func TestPoolCanceledHealthCheckReturnsCapacity(t *testing.T) {
	entered := make(chan struct{})
	var calls atomic.Int32
	underlying := poolTestClient(t, func(req *http.Request) error {
		if calls.Add(1) == 1 {
			close(entered)
			<-req.Context().Done()
			return req.Context().Err()
		}
		return nil
	})
	pool := &Pool{maxSize: 1, clients: []*Client{{Client: underlying}}, stats: PoolStats{TotalCreated: 1, CurrentIdle: 1}}
	defer pool.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := pool.GetContext(ctx); done <- err }()
	<-entered
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.Equal(t, int64(0), pool.Stats().CurrentActive)
	cli, err := pool.Get()
	require.NoError(t, err)
	pool.Put(cli)
	require.Equal(t, int64(1), pool.Stats().CurrentIdle)
	require.Equal(t, int64(0), pool.Stats().TotalClosed)
}

func TestPoolBurstAboveCapacityRecovers(t *testing.T) {
	underlying := poolTestClient(t, func(*http.Request) error { return nil })
	pool := &Pool{maxSize: 1, clients: []*Client{{Client: underlying}}, stats: PoolStats{TotalCreated: 1, CurrentIdle: 1}}
	defer pool.Close()
	var workers sync.WaitGroup
	errors := make(chan error, 15)
	for i := 0; i < 15; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			cli, err := pool.Get()
			if err != nil {
				errors <- err
				return
			}
			time.Sleep(time.Millisecond)
			pool.Put(cli)
		}()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.Equal(t, int64(0), pool.Stats().CurrentActive)
	require.Equal(t, int64(1), pool.Stats().CurrentIdle)
	require.Equal(t, int64(1), pool.Stats().TotalCreated)
}

func TestCloseStandaloneClientDoesNotEnterPool(t *testing.T) {
	previous := globalPool
	pool := &Pool{initialized: true, maxSize: 1}
	globalPool = pool
	t.Cleanup(func() { globalPool = previous; _ = pool.Close() })
	cli, err := NewStreamingClient()
	require.NoError(t, err)
	CloseClient(cli)
	require.Equal(t, PoolStats{}, pool.Stats())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewDockerClientWithContext(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, PoolStats{}, pool.Stats())
}

// TestNewPool 测试连接池初始化
func TestNewPool(t *testing.T) {
	pool := &Pool{
		clients: make([]*Client, 0, 10),
		maxSize: 10,
	}

	assert.Equal(t, 10, pool.maxSize)
	assert.Equal(t, 0, len(pool.clients))
	assert.False(t, pool.initialized)
}

// TestPoolStats 测试连接池统计
func TestPoolStats(t *testing.T) {
	pool := &Pool{
		clients: make([]*Client, 0, 10),
		maxSize: 10,
		stats: PoolStats{
			TotalCreated:  5,
			TotalReused:   3,
			TotalClosed:   2,
			CurrentActive: 2,
			CurrentIdle:   1,
			GetFailures:   0,
		},
	}

	stats := pool.Stats()
	assert.Equal(t, int64(5), stats.TotalCreated)
	assert.Equal(t, int64(3), stats.TotalReused)
	assert.Equal(t, int64(2), stats.TotalClosed)
	assert.Equal(t, int64(2), stats.CurrentActive)
	assert.Equal(t, int64(1), stats.CurrentIdle)
	assert.Equal(t, int64(0), stats.GetFailures)
}

// TestMockDockerClient_ContainerOperations 使用MockDockerClient测试容器操作
func TestMockDockerClient_ContainerOperations(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*MockDockerClient)
		action   func(DockerClient) error
		wantErr  bool
		errCheck func(error) bool
	}{
		{
			name: "start container success",
			setup: func(m *MockDockerClient) {
				m.ContainerStartFunc = func(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
					return nil
				}
			},
			action: func(c DockerClient) error {
				return c.ContainerStart(context.Background(), "test", types.ContainerStartOptions{})
			},
			wantErr: false,
		},
		{
			name: "start container error",
			setup: func(m *MockDockerClient) {
				m.ContainerStartFunc = func(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
					return assert.AnError
				}
			},
			action: func(c DockerClient) error {
				return c.ContainerStart(context.Background(), "test", types.ContainerStartOptions{})
			},
			wantErr: true,
		},
		{
			name: "stop container success",
			setup: func(m *MockDockerClient) {
				m.ContainerStopFunc = func(ctx context.Context, containerID string, timeout *int) error {
					return nil
				}
			},
			action: func(c DockerClient) error {
				timeout := 30
				return c.ContainerStop(context.Background(), "test", &timeout)
			},
			wantErr: false,
		},
		{
			name: "restart container success",
			setup: func(m *MockDockerClient) {
				m.ContainerRestartFunc = func(ctx context.Context, containerID string, timeout *int) error {
					return nil
				}
			},
			action: func(c DockerClient) error {
				timeout := 30
				return c.ContainerRestart(context.Background(), "test", &timeout)
			},
			wantErr: false,
		},
		{
			name: "pause container success",
			setup: func(m *MockDockerClient) {
				m.ContainerPauseFunc = func(ctx context.Context, containerID string) error {
					return nil
				}
			},
			action: func(c DockerClient) error {
				return c.ContainerPause(context.Background(), "test")
			},
			wantErr: false,
		},
		{
			name: "unpause container success",
			setup: func(m *MockDockerClient) {
				m.ContainerUnpauseFunc = func(ctx context.Context, containerID string) error {
					return nil
				}
			},
			action: func(c DockerClient) error {
				return c.ContainerUnpause(context.Background(), "test")
			},
			wantErr: false,
		},
		{
			name: "kill container success",
			setup: func(m *MockDockerClient) {
				m.ContainerKillFunc = func(ctx context.Context, containerID, signal string) error {
					return nil
				}
			},
			action: func(c DockerClient) error {
				return c.ContainerKill(context.Background(), "test", "SIGKILL")
			},
			wantErr: false,
		},
		{
			name: "rename container success",
			setup: func(m *MockDockerClient) {
				m.ContainerRenameFunc = func(ctx context.Context, containerID, newContainerName string) error {
					return nil
				}
			},
			action: func(c DockerClient) error {
				return c.ContainerRename(context.Background(), "old", "new")
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDockerClient{}
			tt.setup(mock)

			err := tt.action(mock)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestMockDockerClient_ContextTimeout 测试上下文超时
func TestMockDockerClient_ContextTimeout(t *testing.T) {
	mock := &MockDockerClient{
		ContainerListFunc: func(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return []types.Container{}, nil
			}
		},
	}

	// 测试正常情况
	ctx := context.Background()
	containers, err := mock.ContainerList(ctx, types.ContainerListOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, containers)

	// 测试超时情况
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	containers, err = mock.ContainerList(ctx, types.ContainerListOptions{})
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
	assert.Nil(t, containers)
}

// TestPool_ConcurrentAccess 测试并发访问
func TestPool_ConcurrentAccess(t *testing.T) {
	pool := &Pool{
		clients:     make([]*Client, 0, 10),
		maxSize:     10,
		initialized: true,
	}

	// 并发获取统计
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = pool.Stats()
			done <- true
		}()
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for goroutine")
		}
	}
}

// TestPool_Lifecycle 测试连接池生命周期
func TestPool_Lifecycle(t *testing.T) {
	pool := &Pool{
		clients:     make([]*Client, 0, 5),
		maxSize:     5,
		initialized: true,
	}

	// 初始状态
	stats := pool.Stats()
	assert.Equal(t, int64(0), stats.TotalCreated)
	assert.Equal(t, int64(0), stats.CurrentIdle)
	assert.Equal(t, int64(0), stats.CurrentActive)

	// 关闭连接池
	err := pool.Close()
	assert.NoError(t, err)

	// 关闭后状态
	stats = pool.Stats()
	assert.Equal(t, int64(0), stats.CurrentIdle)
}

// BenchmarkPool_Stats 基准测试
func BenchmarkPool_Stats(b *testing.B) {
	pool := &Pool{
		clients: make([]*Client, 0, 10),
		maxSize: 10,
		stats: PoolStats{
			TotalCreated: 100,
			TotalReused:  50,
		},
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = pool.Stats()
		}
	})
}
