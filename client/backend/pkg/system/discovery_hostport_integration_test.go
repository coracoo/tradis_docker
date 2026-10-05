//go:build !short

package system

import (
	"context"
	"io"
	"os"
	"testing"

	tradisdocker "dockerpanel/backend/pkg/docker"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// 真实 Docker 集成验证：host 网络容器的端口识别走镜像 EXPOSE 声明。
// 默认跳过，需 TRADIS_DOCKER_INTEGRATION=1 且本机有可用 docker socket。
func TestHostNetworkTCPPortsAgainstRealDocker(t *testing.T) {
	if os.Getenv("TRADIS_DOCKER_INTEGRATION") != "1" {
		t.Skip("set TRADIS_DOCKER_INTEGRATION=1 to run against a real docker socket")
	}
	ctx := context.Background()
	if err := tradisdocker.WithClient(ctx, func(cli *tradisdocker.Client) error {
		_, err := cli.Ping(ctx)
		return err
	}); err != nil {
		t.Skipf("docker socket unavailable: %v", err)
	}

	const image = "nginx:alpine"
	if err := tradisdocker.WithClient(ctx, func(cli *tradisdocker.Client) error {
		reader, err := cli.ImagePull(ctx, image, types.ImagePullOptions{})
		if err != nil {
			return err
		}
		defer reader.Close()
		_, _ = io.Copy(io.Discard, reader)
		return nil
	}); err != nil {
		t.Fatalf("pull %s: %v", image, err)
	}

	createHostContainer := func(name string, labels map[string]string) string {
		t.Helper()
		var id string
		err := tradisdocker.WithClient(ctx, func(cli *tradisdocker.Client) error {
			_ = cli.ContainerRemove(ctx, name, types.ContainerRemoveOptions{Force: true})
			created, err := cli.ContainerCreate(ctx, &container.Config{
				Image:  image,
				Labels: labels,
			}, &container.HostConfig{NetworkMode: "host"}, nil, nil, name)
			if err != nil {
				return err
			}
			id = created.ID
			return nil
		})
		if err != nil {
			t.Fatalf("create host-network container: %v", err)
		}
		t.Cleanup(func() {
			_ = tradisdocker.WithClient(ctx, func(cli *tradisdocker.Client) error {
				return cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{Force: true})
			})
		})
		return id
	}

	// nginx:alpine 声明 EXPOSE 80/tcp；host 模式下应识别出 80。
	id := createHostContainer("tradis-test-hostport", nil)
	ports, ok := hostNetworkTCPPorts(ctx, id, nil)
	if !ok {
		t.Fatal("hostNetworkTCPPorts returned not-ok for a host-network container")
	}
	found := false
	for _, port := range ports {
		if port == 80 {
			found = true
		}
	}
	if !found {
		t.Fatalf("ports = %v, want to contain 80 from the image EXPOSE declaration", ports)
	}

	// 显式标签优先：tradis.navigation.port 覆盖 EXPOSE。
	overrideID := createHostContainer("tradis-test-hostport-label", map[string]string{"tradis.navigation.port": "8080"})
	overridePorts, ok := hostNetworkTCPPorts(ctx, overrideID, map[string]string{"tradis.navigation.port": "8080"})
	if !ok || len(overridePorts) == 0 {
		t.Fatalf("label override ports = %v, ok = %v", overridePorts, ok)
	}
	labelFound := false
	for _, port := range overridePorts {
		if port == 8080 {
			labelFound = true
		}
	}
	if !labelFound {
		t.Fatalf("override ports = %v, want to contain 8080 from tradis.navigation.port", overridePorts)
	}

	// 非 host 容器不适用该路径。
	bridgeID := ""
	if err := tradisdocker.WithClient(ctx, func(cli *tradisdocker.Client) error {
		_ = cli.ContainerRemove(ctx, "tradis-test-bridgeport", types.ContainerRemoveOptions{Force: true})
		created, err := cli.ContainerCreate(ctx, &container.Config{Image: image}, nil, nil, nil, "tradis-test-bridgeport")
		if err != nil {
			return err
		}
		bridgeID = created.ID
		return nil
	}); err != nil {
		t.Fatalf("create bridge container: %v", err)
	}
	t.Cleanup(func() {
		_ = tradisdocker.WithClient(ctx, func(cli *tradisdocker.Client) error {
			return cli.ContainerRemove(ctx, bridgeID, types.ContainerRemoveOptions{Force: true})
		})
	})
	ports, ok = hostNetworkTCPPorts(ctx, bridgeID, nil)
	if ok && len(ports) > 0 {
		t.Fatalf("bridge container must not be treated as host network, got ports %v", ports)
	}
}
