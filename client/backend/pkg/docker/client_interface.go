package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

// DockerClient 是Docker客户端的接口定义
// 便于在测试中mock Docker操作
type DockerClient interface {
	// 容器相关
	ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error)
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error
	ContainerStop(ctx context.Context, containerID string, timeout *int) error
	ContainerRestart(ctx context.Context, containerID string, timeout *int) error
	ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error
	ContainerKill(ctx context.Context, containerID, signal string) error
	ContainerPause(ctx context.Context, containerID string) error
	ContainerUnpause(ctx context.Context, containerID string) error
	ContainerRename(ctx context.Context, containerID, newContainerName string) error
	ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	ContainerLogs(ctx context.Context, container string, options types.ContainerLogsOptions) (io.ReadCloser, error)
	ContainerExecCreate(ctx context.Context, container string, config types.ExecConfig) (types.IDResponse, error)
	ContainerExecAttach(ctx context.Context, execID string, config types.ExecStartCheck) (types.HijackedResponse, error)
	ContainerExecInspect(ctx context.Context, execID string) (types.ContainerExecInspect, error)
	ContainerStats(ctx context.Context, containerID string, stream bool) (types.ContainerStats, error)
	ContainerTop(ctx context.Context, containerID string, arguments []string) (container.ContainerTopOKBody, error)

	// 镜像相关
	ImageList(ctx context.Context, options types.ImageListOptions) ([]types.ImageSummary, error)
	ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
	ImagePull(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error)
	ImageRemove(ctx context.Context, imageID string, options types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error)
	ImageTag(ctx context.Context, source, target string) error

	// 卷相关
	VolumeList(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error)
	VolumeCreate(ctx context.Context, options volume.CreateOptions) (volume.Volume, error)
	VolumeInspect(ctx context.Context, volumeID string) (volume.Volume, error)
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
	VolumesPrune(ctx context.Context, pruneFilter filters.Args) (types.VolumesPruneReport, error)

	// 网络相关
	NetworkList(ctx context.Context, options types.NetworkListOptions) ([]types.NetworkResource, error)
	NetworkInspect(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error)
	NetworkCreate(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error)
	NetworkRemove(ctx context.Context, networkID string) error

	// 系统相关
	Ping(ctx context.Context) (types.Ping, error)
	ServerVersion(ctx context.Context) (types.Version, error)
	Info(ctx context.Context) (types.Info, error)
	DiskUsage(ctx context.Context, options types.DiskUsageOptions) (types.DiskUsage, error)
	BuildCachePrune(ctx context.Context, opts types.BuildCachePruneOptions) (*types.BuildCachePruneReport, error)

	// 关闭连接
	Close() error
}

// MockDockerClient 是一个mock实现，用于测试
type MockDockerClient struct {
	// 容器相关mock函数
	ContainerListFunc        func(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error)
	ContainerInspectFunc     func(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerCreateFunc      func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error)
	ContainerStartFunc       func(ctx context.Context, containerID string, options types.ContainerStartOptions) error
	ContainerStopFunc        func(ctx context.Context, containerID string, timeout *int) error
	ContainerRestartFunc     func(ctx context.Context, containerID string, timeout *int) error
	ContainerRemoveFunc      func(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error
	ContainerKillFunc        func(ctx context.Context, containerID, signal string) error
	ContainerPauseFunc       func(ctx context.Context, containerID string) error
	ContainerUnpauseFunc     func(ctx context.Context, containerID string) error
	ContainerRenameFunc      func(ctx context.Context, containerID, newContainerName string) error
	ContainerWaitFunc        func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	ContainerLogsFunc        func(ctx context.Context, container string, options types.ContainerLogsOptions) (io.ReadCloser, error)
	ContainerExecCreateFunc  func(ctx context.Context, container string, config types.ExecConfig) (types.IDResponse, error)
	ContainerExecAttachFunc  func(ctx context.Context, execID string, config types.ExecStartCheck) (types.HijackedResponse, error)
	ContainerExecInspectFunc func(ctx context.Context, execID string) (types.ContainerExecInspect, error)
	ContainerStatsFunc       func(ctx context.Context, containerID string, stream bool) (types.ContainerStats, error)
	ContainerTopFunc         func(ctx context.Context, containerID string, arguments []string) (container.ContainerTopOKBody, error)

	// 镜像相关mock函数
	ImageListFunc           func(ctx context.Context, options types.ImageListOptions) ([]types.ImageSummary, error)
	ImageInspectWithRawFunc func(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
	ImagePullFunc           func(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error)
	ImageRemoveFunc         func(ctx context.Context, imageID string, options types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error)
	ImageTagFunc            func(ctx context.Context, source, target string) error

	// 卷相关mock函数
	VolumeListFunc    func(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error)
	VolumeCreateFunc  func(ctx context.Context, options volume.CreateOptions) (volume.Volume, error)
	VolumeInspectFunc func(ctx context.Context, volumeID string) (volume.Volume, error)
	VolumeRemoveFunc  func(ctx context.Context, volumeID string, force bool) error
	VolumesPruneFunc  func(ctx context.Context, pruneFilter filters.Args) (types.VolumesPruneReport, error)

	// 网络相关mock函数
	NetworkListFunc    func(ctx context.Context, options types.NetworkListOptions) ([]types.NetworkResource, error)
	NetworkInspectFunc func(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error)
	NetworkCreateFunc  func(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error)
	NetworkRemoveFunc  func(ctx context.Context, networkID string) error

	// 系统相关mock函数
	PingFunc          func(ctx context.Context) (types.Ping, error)
	ServerVersionFunc func(ctx context.Context) (types.Version, error)
	InfoFunc          func(ctx context.Context) (types.Info, error)
	DiskUsageFunc       func(ctx context.Context, options types.DiskUsageOptions) (types.DiskUsage, error)
	BuildCachePruneFunc func(ctx context.Context, opts types.BuildCachePruneOptions) (*types.BuildCachePruneReport, error)

	CloseFunc func() error
}

// 实现DockerClient接口的所有方法

func (m *MockDockerClient) ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
	if m.ContainerListFunc != nil {
		return m.ContainerListFunc(ctx, options)
	}
	return nil, nil
}

func (m *MockDockerClient) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	if m.ContainerInspectFunc != nil {
		return m.ContainerInspectFunc(ctx, containerID)
	}
	return types.ContainerJSON{}, nil
}

func (m *MockDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
	if m.ContainerCreateFunc != nil {
		return m.ContainerCreateFunc(ctx, config, hostConfig, networkingConfig, platform, containerName)
	}
	return container.CreateResponse{}, nil
}

func (m *MockDockerClient) ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
	if m.ContainerStartFunc != nil {
		return m.ContainerStartFunc(ctx, containerID, options)
	}
	return nil
}

func (m *MockDockerClient) ContainerStop(ctx context.Context, containerID string, timeout *int) error {
	if m.ContainerStopFunc != nil {
		return m.ContainerStopFunc(ctx, containerID, timeout)
	}
	return nil
}

func (m *MockDockerClient) ContainerRestart(ctx context.Context, containerID string, timeout *int) error {
	if m.ContainerRestartFunc != nil {
		return m.ContainerRestartFunc(ctx, containerID, timeout)
	}
	return nil
}

func (m *MockDockerClient) ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error {
	if m.ContainerRemoveFunc != nil {
		return m.ContainerRemoveFunc(ctx, containerID, options)
	}
	return nil
}

func (m *MockDockerClient) ContainerKill(ctx context.Context, containerID, signal string) error {
	if m.ContainerKillFunc != nil {
		return m.ContainerKillFunc(ctx, containerID, signal)
	}
	return nil
}

func (m *MockDockerClient) ContainerPause(ctx context.Context, containerID string) error {
	if m.ContainerPauseFunc != nil {
		return m.ContainerPauseFunc(ctx, containerID)
	}
	return nil
}

func (m *MockDockerClient) ContainerUnpause(ctx context.Context, containerID string) error {
	if m.ContainerUnpauseFunc != nil {
		return m.ContainerUnpauseFunc(ctx, containerID)
	}
	return nil
}

func (m *MockDockerClient) ContainerRename(ctx context.Context, containerID, newContainerName string) error {
	if m.ContainerRenameFunc != nil {
		return m.ContainerRenameFunc(ctx, containerID, newContainerName)
	}
	return nil
}

func (m *MockDockerClient) ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
	if m.ContainerWaitFunc != nil {
		return m.ContainerWaitFunc(ctx, containerID, condition)
	}
	return nil, nil
}

func (m *MockDockerClient) ContainerLogs(ctx context.Context, container string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
	if m.ContainerLogsFunc != nil {
		return m.ContainerLogsFunc(ctx, container, options)
	}
	return nil, nil
}

func (m *MockDockerClient) ContainerExecCreate(ctx context.Context, container string, config types.ExecConfig) (types.IDResponse, error) {
	if m.ContainerExecCreateFunc != nil {
		return m.ContainerExecCreateFunc(ctx, container, config)
	}
	return types.IDResponse{}, nil
}

func (m *MockDockerClient) ContainerExecAttach(ctx context.Context, execID string, config types.ExecStartCheck) (types.HijackedResponse, error) {
	if m.ContainerExecAttachFunc != nil {
		return m.ContainerExecAttachFunc(ctx, execID, config)
	}
	return types.HijackedResponse{}, nil
}

func (m *MockDockerClient) ContainerExecInspect(ctx context.Context, execID string) (types.ContainerExecInspect, error) {
	if m.ContainerExecInspectFunc != nil {
		return m.ContainerExecInspectFunc(ctx, execID)
	}
	return types.ContainerExecInspect{}, nil
}

func (m *MockDockerClient) ContainerStats(ctx context.Context, containerID string, stream bool) (types.ContainerStats, error) {
	if m.ContainerStatsFunc != nil {
		return m.ContainerStatsFunc(ctx, containerID, stream)
	}
	return types.ContainerStats{}, nil
}

func (m *MockDockerClient) ContainerTop(ctx context.Context, containerID string, arguments []string) (container.ContainerTopOKBody, error) {
	if m.ContainerTopFunc != nil {
		return m.ContainerTopFunc(ctx, containerID, arguments)
	}
	return container.ContainerTopOKBody{}, nil
}

func (m *MockDockerClient) ImageList(ctx context.Context, options types.ImageListOptions) ([]types.ImageSummary, error) {
	if m.ImageListFunc != nil {
		return m.ImageListFunc(ctx, options)
	}
	return nil, nil
}

func (m *MockDockerClient) ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error) {
	if m.ImageInspectWithRawFunc != nil {
		return m.ImageInspectWithRawFunc(ctx, imageID)
	}
	return types.ImageInspect{}, nil, nil
}

func (m *MockDockerClient) ImagePull(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error) {
	if m.ImagePullFunc != nil {
		return m.ImagePullFunc(ctx, refStr, options)
	}
	return nil, nil
}

func (m *MockDockerClient) ImageRemove(ctx context.Context, imageID string, options types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error) {
	if m.ImageRemoveFunc != nil {
		return m.ImageRemoveFunc(ctx, imageID, options)
	}
	return nil, nil
}

func (m *MockDockerClient) ImageTag(ctx context.Context, source, target string) error {
	if m.ImageTagFunc != nil {
		return m.ImageTagFunc(ctx, source, target)
	}
	return nil
}

func (m *MockDockerClient) VolumeList(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error) {
	if m.VolumeListFunc != nil {
		return m.VolumeListFunc(ctx, options)
	}
	return volume.ListResponse{}, nil
}

func (m *MockDockerClient) VolumeCreate(ctx context.Context, options volume.CreateOptions) (volume.Volume, error) {
	if m.VolumeCreateFunc != nil {
		return m.VolumeCreateFunc(ctx, options)
	}
	return volume.Volume{}, nil
}

func (m *MockDockerClient) VolumeInspect(ctx context.Context, volumeID string) (volume.Volume, error) {
	if m.VolumeInspectFunc != nil {
		return m.VolumeInspectFunc(ctx, volumeID)
	}
	return volume.Volume{}, nil
}

func (m *MockDockerClient) VolumeRemove(ctx context.Context, volumeID string, force bool) error {
	if m.VolumeRemoveFunc != nil {
		return m.VolumeRemoveFunc(ctx, volumeID, force)
	}
	return nil
}

func (m *MockDockerClient) VolumesPrune(ctx context.Context, pruneFilter filters.Args) (types.VolumesPruneReport, error) {
	if m.VolumesPruneFunc != nil {
		return m.VolumesPruneFunc(ctx, pruneFilter)
	}
	return types.VolumesPruneReport{}, nil
}

func (m *MockDockerClient) NetworkList(ctx context.Context, options types.NetworkListOptions) ([]types.NetworkResource, error) {
	if m.NetworkListFunc != nil {
		return m.NetworkListFunc(ctx, options)
	}
	return nil, nil
}

func (m *MockDockerClient) NetworkInspect(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error) {
	if m.NetworkInspectFunc != nil {
		return m.NetworkInspectFunc(ctx, networkID, options)
	}
	return types.NetworkResource{}, nil
}

func (m *MockDockerClient) NetworkCreate(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error) {
	if m.NetworkCreateFunc != nil {
		return m.NetworkCreateFunc(ctx, name, options)
	}
	return types.NetworkCreateResponse{}, nil
}

func (m *MockDockerClient) NetworkRemove(ctx context.Context, networkID string) error {
	if m.NetworkRemoveFunc != nil {
		return m.NetworkRemoveFunc(ctx, networkID)
	}
	return nil
}

func (m *MockDockerClient) Ping(ctx context.Context) (types.Ping, error) {
	if m.PingFunc != nil {
		return m.PingFunc(ctx)
	}
	return types.Ping{}, nil
}

func (m *MockDockerClient) ServerVersion(ctx context.Context) (types.Version, error) {
	if m.ServerVersionFunc != nil {
		return m.ServerVersionFunc(ctx)
	}
	return types.Version{}, nil
}

func (m *MockDockerClient) Info(ctx context.Context) (types.Info, error) {
	if m.InfoFunc != nil {
		return m.InfoFunc(ctx)
	}
	return types.Info{}, nil
}

func (m *MockDockerClient) DiskUsage(ctx context.Context, options types.DiskUsageOptions) (types.DiskUsage, error) {
	if m.DiskUsageFunc != nil {
		return m.DiskUsageFunc(ctx, options)
	}
	return types.DiskUsage{}, nil
}

func (m *MockDockerClient) BuildCachePrune(ctx context.Context, opts types.BuildCachePruneOptions) (*types.BuildCachePruneReport, error) {
	if m.BuildCachePruneFunc != nil {
		return m.BuildCachePruneFunc(ctx, opts)
	}
	return &types.BuildCachePruneReport{}, nil
}

func (m *MockDockerClient) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// 确保MockDockerClient实现了DockerClient接口
var _ DockerClient = (*MockDockerClient)(nil)
