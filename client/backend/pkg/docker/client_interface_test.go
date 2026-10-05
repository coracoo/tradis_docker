package docker

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
)

// TestMockDockerClient_ContainerList 测试容器列表mock
func TestMockDockerClient_ContainerList(t *testing.T) {
	mock := &MockDockerClient{
		ContainerListFunc: func(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
			return []types.Container{
				{ID: "container1", Names: []string{"/test1"}},
				{ID: "container2", Names: []string{"/test2"}},
			}, nil
		},
	}

	containers, err := mock.ContainerList(context.Background(), types.ContainerListOptions{})
	assert.NoError(t, err)
	assert.Len(t, containers, 2)
	assert.Equal(t, "container1", containers[0].ID)
	assert.Equal(t, "/test1", containers[0].Names[0])
}

// TestMockDockerClient_ContainerList_Error 测试错误场景
func TestMockDockerClient_ContainerList_Error(t *testing.T) {
	expectedErr := errors.New("docker daemon not running")
	mock := &MockDockerClient{
		ContainerListFunc: func(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
			return nil, expectedErr
		},
	}

	containers, err := mock.ContainerList(context.Background(), types.ContainerListOptions{})
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Nil(t, containers)
}

// TestMockDockerClient_ContainerInspect 测试容器详情
func TestMockDockerClient_ContainerInspect(t *testing.T) {
	mock := &MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:   "container1",
					Name: "/test",
					State: &types.ContainerState{
						Status: "running",
					},
				},
				Config: &container.Config{
					Image: "nginx:latest",
				},
			}, nil
		},
	}

	info, err := mock.ContainerInspect(context.Background(), "container1")
	assert.NoError(t, err)
	assert.Equal(t, "container1", info.ID)
	assert.Equal(t, "/test", info.Name)
	assert.Equal(t, "running", info.State.Status)
	assert.Equal(t, "nginx:latest", info.Config.Image)
}

// TestMockDockerClient_ContainerCreate 测试容器创建
func TestMockDockerClient_ContainerCreate(t *testing.T) {
	mock := &MockDockerClient{
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
			return container.CreateResponse{
				ID:       "newcontainer123",
				Warnings: nil,
			}, nil
		},
	}

	resp, err := mock.ContainerCreate(
		context.Background(),
		&container.Config{Image: "nginx"},
		&container.HostConfig{},
		nil,
		nil,
		"my-nginx",
	)
	assert.NoError(t, err)
	assert.Equal(t, "newcontainer123", resp.ID)
}

// TestMockDockerClient_ContainerStartStop 测试容器启停
func TestMockDockerClient_ContainerStartStop(t *testing.T) {
	startCalled := false
	stopCalled := false

	mock := &MockDockerClient{
		ContainerStartFunc: func(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
			startCalled = true
			assert.Equal(t, "container1", containerID)
			return nil
		},
		ContainerStopFunc: func(ctx context.Context, containerID string, timeout *int) error {
			stopCalled = true
			assert.Equal(t, "container1", containerID)
			return nil
		},
	}

	err := mock.ContainerStart(context.Background(), "container1", types.ContainerStartOptions{})
	assert.NoError(t, err)
	assert.True(t, startCalled)

	timeout := 30
	err = mock.ContainerStop(context.Background(), "container1", &timeout)
	assert.NoError(t, err)
	assert.True(t, stopCalled)
}

// TestMockDockerClient_ImageList 测试镜像列表
func TestMockDockerClient_ImageList(t *testing.T) {
	mock := &MockDockerClient{
		ImageListFunc: func(ctx context.Context, options types.ImageListOptions) ([]types.ImageSummary, error) {
			return []types.ImageSummary{
				{ID: "image1", RepoTags: []string{"nginx:latest"}, Size: 1000000},
				{ID: "image2", RepoTags: []string{"redis:latest"}, Size: 500000},
			}, nil
		},
	}

	images, err := mock.ImageList(context.Background(), types.ImageListOptions{})
	assert.NoError(t, err)
	assert.Len(t, images, 2)
	assert.Equal(t, "image1", images[0].ID)
	assert.Contains(t, images[0].RepoTags, "nginx:latest")
}

// TestMockDockerClient_ImagePull 测试镜像拉取
func TestMockDockerClient_ImagePull(t *testing.T) {
	mock := &MockDockerClient{
		ImagePullFunc: func(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error) {
			assert.Equal(t, "nginx:latest", refStr)
			return io.NopCloser(strings.NewReader("pulling image...")), nil
		},
	}

	reader, err := mock.ImagePull(context.Background(), "nginx:latest", types.ImagePullOptions{})
	assert.NoError(t, err)
	defer reader.Close()

	content, _ := io.ReadAll(reader)
	assert.Equal(t, "pulling image...", string(content))
}

// TestMockDockerClient_VolumeOperations 测试卷操作
func TestMockDockerClient_VolumeOperations(t *testing.T) {
	mock := &MockDockerClient{
		VolumeListFunc: func(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error) {
			return volume.ListResponse{
				Volumes: []*volume.Volume{
					{Name: "vol1", Driver: "local"},
					{Name: "vol2", Driver: "local"},
				},
			}, nil
		},
		VolumeCreateFunc: func(ctx context.Context, options volume.CreateOptions) (volume.Volume, error) {
			assert.Equal(t, "newvol", options.Name)
			return volume.Volume{Name: "newvol", Driver: "local"}, nil
		},
		VolumeRemoveFunc: func(ctx context.Context, volumeID string, force bool) error {
			assert.Equal(t, "oldvol", volumeID)
			assert.True(t, force)
			return nil
		},
	}

	// 测试列表
	listResp, err := mock.VolumeList(context.Background(), volume.ListOptions{})
	assert.NoError(t, err)
	assert.Len(t, listResp.Volumes, 2)

	// 测试创建
	newVol, err := mock.VolumeCreate(context.Background(), volume.CreateOptions{Name: "newvol"})
	assert.NoError(t, err)
	assert.Equal(t, "newvol", newVol.Name)

	// 测试删除
	err = mock.VolumeRemove(context.Background(), "oldvol", true)
	assert.NoError(t, err)
}

// TestMockDockerClient_NetworkOperations 测试网络操作
func TestMockDockerClient_NetworkOperations(t *testing.T) {
	mock := &MockDockerClient{
		NetworkListFunc: func(ctx context.Context, options types.NetworkListOptions) ([]types.NetworkResource, error) {
			return []types.NetworkResource{
				{ID: "net1", Name: "bridge"},
				{ID: "net2", Name: "custom-network"},
			}, nil
		},
		NetworkCreateFunc: func(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error) {
			assert.Equal(t, "mynetwork", name)
			return types.NetworkCreateResponse{ID: "newnet123"}, nil
		},
		NetworkInspectFunc: func(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error) {
			assert.Equal(t, "net1", networkID)
			return types.NetworkResource{ID: "net1", Name: "test"}, nil
		},
	}

	// 测试列表
	networks, err := mock.NetworkList(context.Background(), types.NetworkListOptions{})
	assert.NoError(t, err)
	assert.Len(t, networks, 2)

	// 测试创建
	createResp, err := mock.NetworkCreate(context.Background(), "mynetwork", types.NetworkCreate{})
	assert.NoError(t, err)
	assert.Equal(t, "newnet123", createResp.ID)

	// 测试详情
	info, err := mock.NetworkInspect(context.Background(), "net1", types.NetworkInspectOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "net1", info.ID)
}

// TestMockDockerClient_Ping 测试Ping
func TestMockDockerClient_Ping(t *testing.T) {
	mock := &MockDockerClient{
		PingFunc: func(ctx context.Context) (types.Ping, error) {
			return types.Ping{APIVersion: "1.43"}, nil
		},
	}

	ping, err := mock.Ping(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "1.43", ping.APIVersion)
}

// TestMockDockerClient_ServerVersion 测试版本信息
func TestMockDockerClient_ServerVersion(t *testing.T) {
	mock := &MockDockerClient{
		ServerVersionFunc: func(ctx context.Context) (types.Version, error) {
			return types.Version{
				Version:    "24.0.6",
				APIVersion: "1.43",
				Os:         "linux",
				Arch:       "amd64",
			}, nil
		},
	}

	version, err := mock.ServerVersion(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "24.0.6", version.Version)
	assert.Equal(t, "1.43", version.APIVersion)
}

// TestMockDockerClient_Close 测试关闭连接
func TestMockDockerClient_Close(t *testing.T) {
	closeCalled := false
	mock := &MockDockerClient{
		CloseFunc: func() error {
			closeCalled = true
			return nil
		},
	}

	err := mock.Close()
	assert.NoError(t, err)
	assert.True(t, closeCalled)
}

// TestMockDockerClient_DefaultBehavior 测试默认行为（未设置函数时返回零值）
func TestMockDockerClient_DefaultBehavior(t *testing.T) {
	mock := &MockDockerClient{}

	// 所有方法都应该返回零值和nil错误
	containers, err := mock.ContainerList(context.Background(), types.ContainerListOptions{})
	assert.NoError(t, err)
	assert.Nil(t, containers)

	info, err := mock.ContainerInspect(context.Background(), "test")
	assert.NoError(t, err)
	// ContainerInspect返回空结构体，内部指针字段可能为nil
	// 不直接访问info.ID，因为ContainerJSONBase可能是nil
	assert.NotNil(t, info)

	images, err := mock.ImageList(context.Background(), types.ImageListOptions{})
	assert.NoError(t, err)
	assert.Nil(t, images)

	volList, err := mock.VolumeList(context.Background(), volume.ListOptions{})
	assert.NoError(t, err)
	assert.Empty(t, volList.Volumes)

	networks, err := mock.NetworkList(context.Background(), types.NetworkListOptions{})
	assert.NoError(t, err)
	assert.Nil(t, networks)

	ping, err := mock.Ping(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, ping.APIVersion)
}

// TestDockerClient_InterfaceCompliance 测试接口合规性
func TestDockerClient_InterfaceCompliance(t *testing.T) {
	// 确保MockDockerClient实现了DockerClient接口
	var _ DockerClient = (*MockDockerClient)(nil)
	
	// 确保Client结构体实现了DockerClient接口
	// 注意：这需要真实的Docker客户端，这里只检查类型
	// 实际测试需要Docker环境
}

// BenchmarkMockContainerList 基准测试
func BenchmarkMockContainerList(b *testing.B) {
	mock := &MockDockerClient{
		ContainerListFunc: func(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
			return []types.Container{
				{ID: "container1", Names: []string{"/test1"}},
				{ID: "container2", Names: []string{"/test2"}},
			}, nil
		},
	}

	ctx := context.Background()
	opts := types.ContainerListOptions{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mock.ContainerList(ctx, opts)
	}
}
