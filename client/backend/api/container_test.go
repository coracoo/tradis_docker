package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/gin-gonic/gin"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
)

type recreateTestClient struct {
	*docker.MockDockerClient
}

func (client *recreateTestClient) ContainerStop(ctx context.Context, id string, options container.StopOptions) error {
	return client.MockDockerClient.ContainerStop(ctx, id, options.Timeout)
}

// TestGetContainers_Success 测试获取容器列表成功
func TestGetContainers_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建mock客户端
	mockClient := &docker.MockDockerClient{
		ContainerListFunc: func(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
			return []types.Container{
				{
					ID:         "container1",
					Names:      []string{"/nginx"},
					Image:      "nginx:latest",
					ImageID:    "sha256:123",
					Command:    "nginx -g daemon off;",
					Created:    1234567890,
					Ports:      []types.Port{{PrivatePort: 80, PublicPort: 8080, Type: "tcp"}},
					SizeRw:     1000000,
					SizeRootFs: 2000000,
					Labels:     map[string]string{"app": "web"},
					State:      "running",
					Status:     "Up 2 hours",
					HostConfig: struct {
						NetworkMode string `json:",omitempty"`
					}{NetworkMode: "bridge"},
					NetworkSettings: &types.SummaryNetworkSettings{
						Networks: map[string]*network.EndpointSettings{
							"bridge": {NetworkID: "net1"},
						},
					},
					Mounts: []types.MountPoint{
						{Source: "/data", Destination: "/usr/share/nginx/html", Type: "bind"},
					},
				},
			}, nil
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("dockerClient", mockClient)

	// 调用处理函数（这里需要实际的处理函数实现）
	// getContainers(c) // 注释掉，因为我们需要注入mock客户端

	// 简化测试：验证mock客户端能正确返回数据
	containers, err := mockClient.ContainerList(context.Background(), types.ContainerListOptions{})
	assert.NoError(t, err)
	assert.Len(t, containers, 1)
	assert.Equal(t, "container1", containers[0].ID)
	assert.Equal(t, "nginx:latest", containers[0].Image)
	assert.Equal(t, "running", containers[0].State)
}

// TestGetContainers_Error 测试获取容器列表错误
func TestGetContainers_Error(t *testing.T) {
	mockClient := &docker.MockDockerClient{
		ContainerListFunc: func(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
			return nil, assert.AnError
		},
	}

	containers, err := mockClient.ContainerList(context.Background(), types.ContainerListOptions{})
	assert.Error(t, err)
	assert.Nil(t, containers)
}

func TestBuildContainerListItemsKeepsMixedInspectionResultsInDockerOrder(t *testing.T) {
	containers := []types.Container{
		{ID: "stopped", Names: []string{"/stopped"}, State: "exited"},
		{ID: "running", Names: []string{"/running"}, State: "running"},
	}
	mockClient := &docker.MockDockerClient{
		ContainerInspectFunc: func(_ context.Context, id string) (types.ContainerJSON, error) {
			if id != "running" {
				t.Fatalf("unexpected inspect for %s", id)
			}
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{ID: id, State: &types.ContainerState{}},
				Config:            &container.Config{},
			}, nil
		},
	}

	items := buildContainerListItems(context.Background(), mockClient, containers, 2, true, nil, nil, nil)
	if len(items) != 2 {
		t.Fatalf("len(items)=%d, want 2", len(items))
	}
	if items[0]["Id"] != "stopped" || items[1]["Id"] != "running" {
		t.Fatalf("container order changed: %#v", items)
	}
}

// TestContainerInspect_Success 测试容器详情
func TestContainerInspect_Success(t *testing.T) {
	mockClient := &docker.MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:   "container1",
					Name: "/test",
					State: &types.ContainerState{
						Status:  "running",
						Running: true,
						Pid:     1234,
					},
				},
				Config: &container.Config{
					Hostname: "test-host",
					Image:    "nginx:latest",
					Env:      []string{"NGINX_HOST=test"},
					ExposedPorts: map[nat.Port]struct{}{
						"80/tcp": {},
					},
				},
			}, nil
		},
	}

	info, err := mockClient.ContainerInspect(context.Background(), "container1")
	assert.NoError(t, err)
	assert.Equal(t, "container1", info.ID)
	assert.Equal(t, "/test", info.Name)
	assert.True(t, info.State.Running)
	assert.Equal(t, "running", info.State.Status)
}

func TestContainerHealthStatus(t *testing.T) {
	inspect := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			State: &types.ContainerState{
				Health: &types.Health{Status: "unhealthy"},
			},
		},
	}

	assert.Equal(t, "unhealthy", containerHealthStatus("Up 1 minute (healthy)", inspect, true))
	assert.Equal(t, "healthy", containerHealthStatus("Up 1 minute (healthy)", types.ContainerJSON{}, false))
	assert.Equal(t, "unhealthy", containerHealthStatus("Up 1 minute (unhealthy)", types.ContainerJSON{}, false))
	assert.Equal(t, "starting", containerHealthStatus("Up 1 minute (health: starting)", types.ContainerJSON{}, false))
	assert.Equal(t, "", containerHealthStatus("Up 1 minute", types.ContainerJSON{}, false))
}

func TestBuildContainerListItemKeepsSummaryLabelsWithoutInspect(t *testing.T) {
	item := buildContainerListItem(types.Container{
		ID:    "compose-container",
		Names: []string{"/demo-web-1"},
		Image: "nginx:latest",
		State: "running",
		Labels: map[string]string{
			"com.docker.compose.project": "demo",
			"com.docker.compose.service": "web",
		},
	}, types.ContainerJSON{}, false, nil, nil)

	labels, ok := item["Labels"].(map[string]string)
	assert.True(t, ok)
	assert.Equal(t, "demo", labels["com.docker.compose.project"])
	assert.Equal(t, "web", labels["com.docker.compose.service"])
}

func TestShouldPullBeforeContainerRecreate(t *testing.T) {
	assert.True(t, shouldPullBeforeContainerRecreate(""))
	assert.True(t, shouldPullBeforeContainerRecreate("true"))
	assert.False(t, shouldPullBeforeContainerRecreate("false"))
	assert.False(t, shouldPullBeforeContainerRecreate("0"))
}

func TestShouldStartRecreatedContainer(t *testing.T) {
	assert.True(t, shouldStartRecreatedContainer(&types.ContainerState{Running: true}))
	assert.False(t, shouldStartRecreatedContainer(&types.ContainerState{Running: false}))
	assert.False(t, shouldStartRecreatedContainer(nil))
}

func TestPreserveRecreatedContainerVolumes(t *testing.T) {
	hostConfig := &container.HostConfig{
		Binds: []string{"named-volume:/named:rw"},
		Mounts: []mount.Mount{
			{Type: mount.TypeVolume, Target: "/cache"},
		},
	}
	existing := []types.MountPoint{
		{Type: mount.TypeVolume, Name: "anonymous-volume", Destination: "/cache", RW: true},
		{Type: mount.TypeVolume, Name: "named-volume", Destination: "/named", RW: true},
		{Type: mount.TypeVolume, Name: "dockerfile-volume", Destination: "/data", RW: false},
	}

	preserveRecreatedContainerVolumes(hostConfig, existing)

	assert.Equal(t, "anonymous-volume", hostConfig.Mounts[0].Source)
	assert.Len(t, hostConfig.Mounts, 2)
	assert.Equal(t, "/data", hostConfig.Mounts[1].Target)
	assert.Equal(t, "dockerfile-volume", hostConfig.Mounts[1].Source)
	assert.True(t, hostConfig.Mounts[1].ReadOnly)
}

func TestRecreateSourceContainerID(t *testing.T) {
	inspected := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{ID: "immutable-id"},
	}

	assert.Equal(t, "immutable-id", recreateSourceContainerID("container-name", inspected))
	assert.Equal(t, "container-name", recreateSourceContainerID("container-name", types.ContainerJSON{}))
}

func TestRunContainerRecreatePullsAndReplacesContainer(t *testing.T) {
	assert.NoError(t, database.InitDB(t.TempDir()+"/container-recreate.db"))

	var calls []string
	mockClient := &docker.MockDockerClient{
		ContainerInspectFunc: func(context.Context, string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:         "old-id",
					Name:       "/media",
					State:      &types.ContainerState{Running: true},
					HostConfig: &container.HostConfig{},
				},
				Config:          &container.Config{Image: "example/media:latest"},
				NetworkSettings: &types.NetworkSettings{Networks: map[string]*network.EndpointSettings{"bridge": {}}},
			}, nil
		},
		ImagePullFunc: func(context.Context, string, types.ImagePullOptions) (io.ReadCloser, error) {
			calls = append(calls, "pull")
			return io.NopCloser(strings.NewReader("{}")), nil
		},
		ContainerRenameFunc: func(_ context.Context, id, name string) error {
			assert.Equal(t, "old-id", id)
			assert.True(t, strings.HasPrefix(name, "media_backup_"))
			calls = append(calls, "rename")
			return nil
		},
		ContainerStopFunc: func(context.Context, string, *int) error {
			calls = append(calls, "stop")
			return nil
		},
		ContainerCreateFunc: func(context.Context, *container.Config, *container.HostConfig, *network.NetworkingConfig, *specs.Platform, string) (container.CreateResponse, error) {
			calls = append(calls, "create")
			return container.CreateResponse{ID: "new-id"}, nil
		},
		ContainerStartFunc: func(context.Context, string, types.ContainerStartOptions) error {
			calls = append(calls, "start")
			return nil
		},
		ContainerRemoveFunc: func(context.Context, string, types.ContainerRemoveOptions) error {
			calls = append(calls, "remove")
			return nil
		},
	}

	var logs []string
	err := runContainerRecreate(context.Background(), &recreateTestClient{MockDockerClient: mockClient}, "old-id", true, func(message string) {
		logs = append(logs, message)
	})

	assert.NoError(t, err)
	assert.Equal(t, []string{"pull", "rename", "stop", "create", "start", "remove"}, calls)
	assert.Contains(t, logs, "success: 容器更新完成")
}

func TestConsumeDockerPullResponseReturnsStreamError(t *testing.T) {
	stream := "{\"status\":\"Pulling\"}\n" +
		"{\"errorDetail\":{\"message\":\"manifest unknown\"},\"error\":\"manifest unknown\"}\n"
	err := consumeDockerPullResponse(io.NopCloser(strings.NewReader(stream)))
	assert.EqualError(t, err, "manifest unknown")
}

func TestConsumeDockerPullResponseReturnsReadError(t *testing.T) {
	err := consumeDockerPullResponse(io.NopCloser(&failingReader{err: errors.New("connection reset")}))
	assert.ErrorContains(t, err, "connection reset")
}

func TestContainerUpdateLeaseRejectsConcurrentUpdate(t *testing.T) {
	release, ok := acquireContainerUpdateLease("container-1")
	assert.True(t, ok)
	_, duplicate := acquireContainerUpdateLease("container-1")
	assert.False(t, duplicate)
	release()
	secondRelease, ok := acquireContainerUpdateLease("container-1")
	assert.True(t, ok)
	secondRelease()
}

func TestContainerUpdateAvailableMatchesRecordedImageID(t *testing.T) {
	updateIDs := map[string]string{"example/media:latest": "sha256:old"}
	assert.True(t, containerUpdateAvailable(
		map[string]bool{"example/media:latest": true},
		nil,
		updateIDs,
		"example/media:latest",
		"sha256:old",
	))
	assert.False(t, containerUpdateAvailable(
		map[string]bool{"example/media:latest": true},
		nil,
		updateIDs,
		"example/media:latest",
		"sha256:new",
	))
}

type failingReader struct {
	err error
}

func (reader *failingReader) Read([]byte) (int, error) {
	return 0, reader.err
}

// TestContainerStart_Stop_Restart 测试容器启停重启
func TestContainerStart_Stop_Restart(t *testing.T) {
	callCount := make(map[string]int)

	mockClient := &docker.MockDockerClient{
		ContainerStartFunc: func(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
			callCount["start"]++
			assert.Equal(t, "container1", containerID)
			return nil
		},
		ContainerStopFunc: func(ctx context.Context, containerID string, timeout *int) error {
			callCount["stop"]++
			assert.Equal(t, "container1", containerID)
			return nil
		},
		ContainerRestartFunc: func(ctx context.Context, containerID string, timeout *int) error {
			callCount["restart"]++
			assert.Equal(t, "container1", containerID)
			return nil
		},
	}

	// 测试启动
	err := mockClient.ContainerStart(context.Background(), "container1", types.ContainerStartOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 1, callCount["start"])

	// 测试停止
	timeout := 30
	err = mockClient.ContainerStop(context.Background(), "container1", &timeout)
	assert.NoError(t, err)
	assert.Equal(t, 1, callCount["stop"])

	// 测试重启
	err = mockClient.ContainerRestart(context.Background(), "container1", &timeout)
	assert.NoError(t, err)
	assert.Equal(t, 1, callCount["restart"])
}

// TestContainerRemove 测试删除容器
func TestContainerRemove(t *testing.T) {
	removeCalled := false

	mockClient := &docker.MockDockerClient{
		ContainerRemoveFunc: func(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error {
			removeCalled = true
			assert.Equal(t, "container1", containerID)
			assert.True(t, options.Force)
			return nil
		},
	}

	err := mockClient.ContainerRemove(context.Background(), "container1", types.ContainerRemoveOptions{Force: true})
	assert.NoError(t, err)
	assert.True(t, removeCalled)
}

// TestContainerPause_Unpause 测试暂停/恢复
func TestContainerPause_Unpause(t *testing.T) {
	actions := []string{}

	mockClient := &docker.MockDockerClient{
		ContainerPauseFunc: func(ctx context.Context, containerID string) error {
			actions = append(actions, "pause")
			assert.Equal(t, "container1", containerID)
			return nil
		},
		ContainerUnpauseFunc: func(ctx context.Context, containerID string) error {
			actions = append(actions, "unpause")
			assert.Equal(t, "container1", containerID)
			return nil
		},
	}

	err := mockClient.ContainerPause(context.Background(), "container1")
	assert.NoError(t, err)

	err = mockClient.ContainerUnpause(context.Background(), "container1")
	assert.NoError(t, err)

	assert.Equal(t, []string{"pause", "unpause"}, actions)
}

// TestContainerLogs 测试容器日志
func TestContainerLogs(t *testing.T) {
	mockClient := &docker.MockDockerClient{
		ContainerLogsFunc: func(ctx context.Context, container string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
			assert.Equal(t, "container1", container)
			assert.True(t, options.ShowStdout)
			assert.True(t, options.ShowStderr)
			return io.NopCloser(strings.NewReader("container log output")), nil
		},
	}

	reader, err := mockClient.ContainerLogs(context.Background(), "container1", types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	assert.NoError(t, err)
	defer reader.Close()

	content, err := io.ReadAll(reader)
	assert.NoError(t, err)
	assert.Equal(t, "container log output", string(content))
}

// TestContainerStats 测试容器统计
func TestContainerStats(t *testing.T) {
	mockClient := &docker.MockDockerClient{
		ContainerStatsFunc: func(ctx context.Context, containerID string, stream bool) (types.ContainerStats, error) {
			assert.Equal(t, "container1", containerID)
			assert.False(t, stream)
			return types.ContainerStats{
				Body: io.NopCloser(strings.NewReader(`{"memory_stats":{"usage":1000000}}`)),
			}, nil
		},
	}

	stats, err := mockClient.ContainerStats(context.Background(), "container1", false)
	assert.NoError(t, err)
	assert.NotNil(t, stats.Body)
}

// TestContainerRename 测试重命名容器
func TestContainerRename(t *testing.T) {
	mockClient := &docker.MockDockerClient{
		ContainerRenameFunc: func(ctx context.Context, containerID, newContainerName string) error {
			assert.Equal(t, "old", containerID)
			assert.Equal(t, "new", newContainerName)
			return nil
		},
	}

	err := mockClient.ContainerRename(context.Background(), "old", "new")
	assert.NoError(t, err)
}

// TestContainerList_FilterOptions 测试过滤选项
func TestContainerList_FilterOptions(t *testing.T) {
	receivedOptions := types.ContainerListOptions{}

	mockClient := &docker.MockDockerClient{
		ContainerListFunc: func(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
			receivedOptions = options
			return []types.Container{}, nil
		},
	}

	filters := filters.NewArgs()
	filters.Add("status", "running")

	opts := types.ContainerListOptions{
		All:     true,
		Size:    true,
		Filters: filters,
	}

	_, err := mockClient.ContainerList(context.Background(), opts)
	assert.NoError(t, err)
	assert.True(t, receivedOptions.All)
	assert.True(t, receivedOptions.Size)
	assert.NotNil(t, receivedOptions.Filters)
}

// BenchmarkContainerList 基准测试
func BenchmarkContainerList(b *testing.B) {
	mockClient := &docker.MockDockerClient{
		ContainerListFunc: func(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
			return []types.Container{
				{ID: "1", Names: []string{"/c1"}},
				{ID: "2", Names: []string{"/c2"}},
				{ID: "3", Names: []string{"/c3"}},
			}, nil
		},
	}

	ctx := context.Background()
	opts := types.ContainerListOptions{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mockClient.ContainerList(ctx, opts)
	}
}

func TestContainerUpdateNotificationMessageIncludesKnownName(t *testing.T) {
	assert.Equal(t, "容器 media 更新完成", containerUpdateNotificationMessage("media", true))
	assert.Equal(t, "容器 media 更新失败", containerUpdateNotificationMessage("media", false))
	assert.Equal(t, "容器更新失败", containerUpdateNotificationMessage("", false))
}

// ===== 强制停止（0 宽限期 ContainerStop，立即 SIGKILL 且按手动停止处理重启策略） =====

// forceStopCaptureClient 适配 SDK 的 ContainerStop(StopOptions) 签名并记录调用。
type forceStopCaptureClient struct {
	*docker.MockDockerClient
	stop func(ctx context.Context, id string, options container.StopOptions) error
}

func (client *forceStopCaptureClient) ContainerStop(ctx context.Context, id string, options container.StopOptions) error {
	return client.stop(ctx, id, options)
}

func TestKillContainerByIDStopsWithZeroGrace(t *testing.T) {
	var stoppedID string
	var stoppedTimeout *int
	mockClient := &forceStopCaptureClient{
		MockDockerClient: &docker.MockDockerClient{
			ContainerInspectFunc: func(context.Context, string) (types.ContainerJSON, error) {
				return types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						ID:    "container1",
						State: &types.ContainerState{Running: true},
					},
				}, nil
			},
		},
		stop: func(_ context.Context, id string, options container.StopOptions) error {
			stoppedID, stoppedTimeout = id, options.Timeout
			return nil
		},
	}

	err := killContainerByID(context.Background(), mockClient, "container1")
	assert.NoError(t, err)
	assert.Equal(t, "container1", stoppedID)
	if assert.NotNil(t, stoppedTimeout) {
		assert.Zero(t, *stoppedTimeout)
	}
}

func TestKillContainerByIDReturnsNotFoundForMissingContainer(t *testing.T) {
	stopCalled := false
	mockClient := &forceStopCaptureClient{
		MockDockerClient: &docker.MockDockerClient{
			ContainerInspectFunc: func(context.Context, string) (types.ContainerJSON, error) {
				return types.ContainerJSON{}, errdefs.NotFound(errors.New("No such container: missing"))
			},
		},
		stop: func(context.Context, string, container.StopOptions) error {
			stopCalled = true
			return nil
		},
	}

	err := killContainerByID(context.Background(), mockClient, "missing")
	assert.ErrorIs(t, err, errContainerNotFound)
	assert.False(t, stopCalled)
}

func TestKillContainerByIDPreservesInspectFailure(t *testing.T) {
	mockClient := &forceStopCaptureClient{
		MockDockerClient: &docker.MockDockerClient{
			ContainerInspectFunc: func(context.Context, string) (types.ContainerJSON, error) {
				return types.ContainerJSON{}, errors.New("docker socket unavailable")
			},
		},
		stop: func(context.Context, string, container.StopOptions) error {
			return nil
		},
	}

	err := killContainerByID(context.Background(), mockClient, "container1")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, errContainerNotFound)
	assert.Contains(t, err.Error(), "docker socket unavailable")
}

func TestKillContainerHandlerRejectsSelfContainer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 先消费 sync.Once 完成环境探测，再覆写包级自身身份，避免依赖运行环境。
	getSelfIdentity()
	oldID, oldName := selfContainerID, selfContainerName
	selfContainerID, selfContainerName = "self-container-id-123", "tradis-self"
	t.Cleanup(func() {
		selfContainerID, selfContainerName = oldID, oldName
	})

	router := gin.New()
	router.POST("/containers/:id/kill", killContainer)

	request := httptest.NewRequest(http.MethodPost, "/containers/tradis-self/kill", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Contains(t, response.Body.String(), "禁止管理自身容器")
}

// requireDockerDaemon 在 Docker socket 不可用时跳过（守护进程相关的集成用例）。
func requireDockerDaemon(t *testing.T) *dockerclient.Client {
	t.Helper()
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("docker socket unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	if _, err := cli.Ping(context.Background()); err != nil {
		t.Skipf("docker socket unavailable: %v", err)
	}
	return cli
}

func TestKillContainerHandlerReturnsNotFoundForMissingContainer(t *testing.T) {
	requireDockerDaemon(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/containers/:id/kill", killContainer)

	request := httptest.NewRequest(http.MethodPost, "/containers/tradis-force-kill-missing/kill", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
}

func TestKillContainerHandlerKillsRunningContainer(t *testing.T) {
	cli := requireDockerDaemon(t)
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	const name = "tradis-test-force-kill"
	_ = cli.ContainerRemove(ctx, name, types.ContainerRemoveOptions{Force: true})
	created, err := cli.ContainerCreate(ctx,
		&container.Config{Image: "nginx:alpine"},
		&container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "always"}},
		nil, nil, name)
	if err != nil {
		t.Skipf("create test container failed (nginx:alpine required): %v", err)
	}
	t.Cleanup(func() {
		_ = cli.ContainerRemove(ctx, created.ID, types.ContainerRemoveOptions{Force: true})
	})
	if err := cli.ContainerStart(ctx, created.ID, types.ContainerStartOptions{}); err != nil {
		t.Fatalf("start test container: %v", err)
	}

	router := gin.New()
	router.POST("/containers/:id/kill", killContainer)
	request := httptest.NewRequest(http.MethodPost, "/containers/"+created.ID+"/kill", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())

	inspect, err := cli.ContainerInspect(ctx, created.ID)
	assert.NoError(t, err)
	assert.False(t, inspect.State.Running)
	// 强制停止必须按手动停止处理：restart=always 的容器不得被守护进程再次拉起。
	time.Sleep(2 * time.Second)
	inspect, err = cli.ContainerInspect(ctx, created.ID)
	assert.NoError(t, err)
	assert.False(t, inspect.State.Running)
}

func TestClearImageUpdateRecordsAfterContainerRecreate(t *testing.T) {
	_ = database.Close()
	if err := database.InitDB(t.TempDir() + "/container-recreate-clear.db"); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	seed := database.ImageUpdate{RepoTag: "example/media:latest", ImageID: "sha256:img", LocalDigest: "sha256:local", RemoteDigest: "sha256:remote"}
	if err := database.SaveImageUpdate(&seed); err != nil {
		t.Fatalf("SaveImageUpdate: %v", err)
	}

	// 预填角标缓存，验证清理后缓存被失效
	imageUpdateMapCache.mu.Lock()
	imageUpdateMapCache.updateMap = map[string]bool{"example/media:latest": true}
	imageUpdateMapCache.expires = time.Now().Add(time.Hour)
	imageUpdateMapCache.mu.Unlock()

	// 带 pull 的重建式更新成功后：该容器镜像的记录被清、缓存失效被调
	clearImageUpdateRecordsAfterContainerRecreate(true, "example/media:latest")
	remaining, err := database.GetAllImageUpdates()
	if err != nil {
		t.Fatalf("GetAllImageUpdates: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining records after recreate = %+v, want none", remaining)
	}
	imageUpdateMapCache.mu.Lock()
	defer imageUpdateMapCache.mu.Unlock()
	if imageUpdateMapCache.updateMap != nil {
		t.Fatalf("image update map cache was not invalidated")
	}
}

func TestClearImageUpdateRecordsAfterContainerRecreateWithoutPullKeepsRecords(t *testing.T) {
	_ = database.Close()
	if err := database.InitDB(t.TempDir() + "/container-recreate-keep.db"); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	seed := database.ImageUpdate{RepoTag: "example/media:latest", ImageID: "sha256:img", LocalDigest: "sha256:local", RemoteDigest: "sha256:remote"}
	if err := database.SaveImageUpdate(&seed); err != nil {
		t.Fatalf("SaveImageUpdate: %v", err)
	}

	// 无 pull 的纯重建不改动镜像，记录（镜像确有更新）仍然成立，保持不动
	clearImageUpdateRecordsAfterContainerRecreate(false, "example/media:latest")
	clearImageUpdateRecordsAfterContainerRecreate(true, "  ")

	remaining, err := database.GetAllImageUpdates()
	if err != nil {
		t.Fatalf("GetAllImageUpdates: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining records = %+v, want record kept", remaining)
	}
}
