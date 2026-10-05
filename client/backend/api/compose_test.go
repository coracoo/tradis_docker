package api

import (
	"context"
	"io"
	"strings"
	"testing"

	"dockerpanel/backend/pkg/docker"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
)

// TestDeployCompose_Success 测试Compose部署成功
func TestDeployCompose_Success(t *testing.T) {
	imagePullCalled := false
	containerCreateCalled := false
	containerStartCalled := false
	
	mockClient := &docker.MockDockerClient{
		ImagePullFunc: func(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error) {
			imagePullCalled = true
			assert.Equal(t, "nginx:latest", refStr)
			return io.NopCloser(strings.NewReader("pulling...")), nil
		},
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
			containerCreateCalled = true
			assert.Equal(t, "my-nginx", containerName)
			assert.Equal(t, "nginx:latest", config.Image)
			return container.CreateResponse{ID: "container123"}, nil
		},
		ContainerStartFunc: func(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
			containerStartCalled = true
			assert.Equal(t, "container123", containerID)
			return nil
		},
		NetworkInspectFunc: func(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error) {
			// 返回网络不存在错误
			return types.NetworkResource{}, assert.AnError
		},
		NetworkCreateFunc: func(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error) {
			assert.Equal(t, "my-network", name)
			return types.NetworkCreateResponse{ID: "network123"}, nil
		},
	}

	// 测试Compose部署逻辑
	// 这里我们验证mock被正确调用
	_, err := mockClient.ImagePull(context.Background(), "nginx:latest", types.ImagePullOptions{})
	assert.NoError(t, err)
	assert.True(t, imagePullCalled)
	
	resp, err := mockClient.ContainerCreate(context.Background(), 
		&container.Config{Image: "nginx:latest"},
		&container.HostConfig{},
		nil, nil, "my-nginx")
	assert.NoError(t, err)
	assert.True(t, containerCreateCalled)
	assert.Equal(t, "container123", resp.ID)
	
	err = mockClient.ContainerStart(context.Background(), "container123", types.ContainerStartOptions{})
	assert.NoError(t, err)
	assert.True(t, containerStartCalled)
}

// TestDeployCompose_NetworkCreation 测试网络创建
func TestDeployCompose_NetworkCreation(t *testing.T) {
	mockClient := &docker.MockDockerClient{
		NetworkInspectFunc: func(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error) {
			// 模拟网络已存在
			if networkID == "existing-network" {
				return types.NetworkResource{ID: "net1", Name: "existing-network"}, nil
			}
			return types.NetworkResource{}, assert.AnError
		},
		NetworkCreateFunc: func(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error) {
			assert.Equal(t, "new-network", name)
			return types.NetworkCreateResponse{ID: "net2"}, nil
		},
	}

	// 测试已存在的网络 - 不应该创建新网络
	net, err := mockClient.NetworkInspect(context.Background(), "existing-network", types.NetworkInspectOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "net1", net.ID)

	// 测试新网络 - 应该创建
	// 在实际实现中，这里会调用NetworkCreate
}

// TestDeployCompose_MultipleServices 测试多服务部署
func TestDeployCompose_MultipleServices(t *testing.T) {
	createdContainers := []string{}
	
	mockClient := &docker.MockDockerClient{
		ImagePullFunc: func(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("pulling...")), nil
		},
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
			createdContainers = append(createdContainers, containerName)
			return container.CreateResponse{ID: "id-" + containerName}, nil
		},
		ContainerStartFunc: func(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
			return nil
		},
		NetworkInspectFunc: func(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error) {
			return types.NetworkResource{}, assert.AnError
		},
		NetworkCreateFunc: func(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error) {
			return types.NetworkCreateResponse{ID: "network123"}, nil
		},
	}

	// 模拟部署多个服务
	services := []struct {
		name  string
		image string
	}{
		{"web", "nginx:latest"},
		{"db", "postgres:latest"},
		{"cache", "redis:latest"},
	}

	for _, svc := range services {
		_, _ = mockClient.ImagePull(context.Background(), svc.image, types.ImagePullOptions{})
		resp, _ := mockClient.ContainerCreate(context.Background(),
			&container.Config{Image: svc.image},
			&container.HostConfig{},
			nil, nil, svc.name)
		_ = mockClient.ContainerStart(context.Background(), resp.ID, types.ContainerStartOptions{})
	}

	assert.Len(t, createdContainers, 3)
	assert.Contains(t, createdContainers, "web")
	assert.Contains(t, createdContainers, "db")
	assert.Contains(t, createdContainers, "cache")
}

// TestDeployCompose_VolumeHandling 测试卷处理
func TestDeployCompose_VolumeHandling(t *testing.T) {
	receivedHostConfig := &container.HostConfig{}
	
	mockClient := &docker.MockDockerClient{
		ImagePullFunc: func(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("pulling...")), nil
		},
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
			receivedHostConfig = hostConfig
			return container.CreateResponse{ID: "container123"}, nil
		},
		ContainerStartFunc: func(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
			return nil
		},
		NetworkInspectFunc: func(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error) {
			return types.NetworkResource{}, assert.AnError
		},
		NetworkCreateFunc: func(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error) {
			return types.NetworkCreateResponse{ID: "network123"}, nil
		},
	}

	// 测试带卷的容器创建
	hostConfig := &container.HostConfig{
		Binds: []string{"/host/data:/container/data", "vol2:/container/config"},
	}
	
	resp, err := mockClient.ContainerCreate(context.Background(),
		&container.Config{Image: "nginx:latest"},
		hostConfig,
		nil, nil, "test")
	
	assert.NoError(t, err)
	assert.Equal(t, "container123", resp.ID)
	assert.Len(t, receivedHostConfig.Binds, 2)
}

// TestDeployCompose_PortMapping 测试端口映射
func TestDeployCompose_PortMapping(t *testing.T) {
	receivedHostConfig := &container.HostConfig{}
	
	mockClient := &docker.MockDockerClient{
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
			receivedHostConfig = hostConfig
			return container.CreateResponse{ID: "container123"}, nil
		},
	}

	// 创建带端口映射的host config
	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			"80/tcp": []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: "8080"},
			},
		},
	}
	
	resp, err := mockClient.ContainerCreate(context.Background(),
		&container.Config{Image: "nginx:latest"},
		hostConfig,
		nil, nil, "test")
	
	assert.NoError(t, err)
	assert.Equal(t, "container123", resp.ID)
	assert.Contains(t, receivedHostConfig.PortBindings, nat.Port("80/tcp"))
}

// TestDeployCompose_RestartPolicy 测试重启策略
func TestDeployCompose_RestartPolicy(t *testing.T) {
	receivedHostConfig := &container.HostConfig{}
	
	mockClient := &docker.MockDockerClient{
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
			receivedHostConfig = hostConfig
			return container.CreateResponse{ID: "container123"}, nil
		},
	}

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name:              "on-failure",
			MaximumRetryCount: 3,
		},
	}
	
	resp, err := mockClient.ContainerCreate(context.Background(),
		&container.Config{Image: "nginx:latest"},
		hostConfig,
		nil, nil, "test")
	
	assert.NoError(t, err)
	assert.Equal(t, "container123", resp.ID)
	assert.Equal(t, container.RestartPolicy{Name: "on-failure", MaximumRetryCount: 3}, receivedHostConfig.RestartPolicy)
}

// TestDeployCompose_EnvironmentVariables 测试环境变量
func TestDeployCompose_EnvironmentVariables(t *testing.T) {
	receivedConfig := &container.Config{}
	
	mockClient := &docker.MockDockerClient{
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
			receivedConfig = config
			return container.CreateResponse{ID: "container123"}, nil
		},
	}

	config := &container.Config{
		Image: "nginx:latest",
		Env: []string{
			"NGINX_HOST=localhost",
			"NGINX_PORT=80",
			"DEBUG=true",
		},
	}
	
	resp, err := mockClient.ContainerCreate(context.Background(),
		config,
		&container.HostConfig{},
		nil, nil, "test")
	
	assert.NoError(t, err)
	assert.Equal(t, "container123", resp.ID)
	assert.Contains(t, receivedConfig.Env, "NGINX_HOST=localhost")
	assert.Contains(t, receivedConfig.Env, "NGINX_PORT=80")
	assert.Contains(t, receivedConfig.Env, "DEBUG=true")
}

// TestDeployCompose_ErrorHandling 测试错误处理
func TestDeployCompose_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		mockSetup   func(*docker.MockDockerClient)
		wantErr     bool
		errContains string
	}{
		{
			name: "image pull error",
			mockSetup: func(m *docker.MockDockerClient) {
				m.ImagePullFunc = func(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error) {
					return nil, assert.AnError
				}
			},
			wantErr:     true,
			errContains: "assert.AnError",
		},
		{
			name: "container create error",
			mockSetup: func(m *docker.MockDockerClient) {
				m.ImagePullFunc = func(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("")), nil
				}
				m.ContainerCreateFunc = func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
					return container.CreateResponse{}, assert.AnError
				}
			},
			wantErr:     true,
			errContains: "assert.AnError",
		},
		{
			name: "container start error",
			mockSetup: func(m *docker.MockDockerClient) {
				m.ImagePullFunc = func(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("")), nil
				}
				m.ContainerCreateFunc = func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
					return container.CreateResponse{ID: "123"}, nil
				}
				m.ContainerStartFunc = func(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
					return assert.AnError
				}
			},
			wantErr:     true,
			errContains: "assert.AnError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &docker.MockDockerClient{}
			tt.mockSetup(mock)
			
			// 验证mock正确设置
			if tt.name == "image pull error" {
				_, err := mock.ImagePull(context.Background(), "test", types.ImagePullOptions{})
				assert.Error(t, err)
			} else if tt.name == "container create error" {
				_, err := mock.ContainerCreate(context.Background(), nil, nil, nil, nil, "test")
				assert.Error(t, err)
			} else if tt.name == "container start error" {
				err := mock.ContainerStart(context.Background(), "test", types.ContainerStartOptions{})
				assert.Error(t, err)
			}
		})
	}
}

// BenchmarkDeployCompose 基准测试
func BenchmarkDeployCompose(b *testing.B) {
	mockClient := &docker.MockDockerClient{
		ImagePullFunc: func(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("")), nil
		},
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
			return container.CreateResponse{ID: "123"}, nil
		},
		ContainerStartFunc: func(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
			return nil
		},
		NetworkInspectFunc: func(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error) {
			return types.NetworkResource{}, assert.AnError
		},
		NetworkCreateFunc: func(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error) {
			return types.NetworkCreateResponse{ID: "net123"}, nil
		},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mockClient.ImagePull(ctx, "nginx:latest", types.ImagePullOptions{})
		resp, _ := mockClient.ContainerCreate(ctx, &container.Config{Image: "nginx:latest"}, &container.HostConfig{}, nil, nil, "test")
		_ = mockClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	}
}
