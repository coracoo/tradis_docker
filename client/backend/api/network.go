package api

import (
	"context"
	"dockerpanel/backend/pkg/docker"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	networktypes "github.com/docker/docker/api/types/network"
	"github.com/gin-gonic/gin"
)

type networkRequest struct {
	Name        string            `json:"name"`
	Driver      string            `json:"driver"`
	IPv4Subnet  string            `json:"ipv4Subnet"`
	IPv4Gateway string            `json:"ipv4Gateway"`
	IPv4IPRange string            `json:"ipv4IpRange"`
	IPv6Subnet  string            `json:"ipv6Subnet"`
	IPv6Gateway string            `json:"ipv6Gateway"`
	IPv6IPRange string            `json:"ipv6IpRange"`
	EnableIPv6  bool              `json:"enableIPv6"`
	Internal    bool              `json:"internal"`
	Attachable  bool              `json:"attachable"`
	Options     map[string]string `json:"options"`
	Labels      map[string]string `json:"labels"`
	IPAMOptions map[string]string `json:"ipamOptions"`
	Parent      string            `json:"parent"`
}

func newNetworkRollbackContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func firstNonEmptyNetwork(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeNetworkDriver(driver string) string {
	driver = strings.TrimSpace(driver)
	if driver == "" {
		return "bridge"
	}
	return driver
}

func buildNetworkCreateOptions(req networkRequest, fallbackDriver string) types.NetworkCreate {
	driver := normalizeNetworkDriver(firstNonEmptyNetwork(req.Driver, fallbackDriver))
	ipamConfigs := []networktypes.IPAMConfig{}

	ipv4Subnet := strings.TrimSpace(req.IPv4Subnet)
	if ipv4Subnet != "" {
		ipamConfigs = append(ipamConfigs, networktypes.IPAMConfig{
			Subnet:  ipv4Subnet,
			Gateway: strings.TrimSpace(req.IPv4Gateway),
			IPRange: strings.TrimSpace(req.IPv4IPRange),
		})
	}
	if req.EnableIPv6 && req.IPv6Subnet != "" {
		ipamConfigs = append(ipamConfigs, networktypes.IPAMConfig{
			Subnet:  strings.TrimSpace(req.IPv6Subnet),
			Gateway: strings.TrimSpace(req.IPv6Gateway),
			IPRange: strings.TrimSpace(req.IPv6IPRange),
		})
	}

	options := map[string]string{}
	for k, v := range req.Options {
		if strings.TrimSpace(k) != "" {
			options[k] = v
		}
	}
	if driver == "macvlan" && strings.TrimSpace(req.Parent) != "" {
		options["parent"] = strings.TrimSpace(req.Parent)
	}

	return types.NetworkCreate{
		CheckDuplicate: true,
		Driver:         driver,
		EnableIPv6:     req.EnableIPv6,
		Internal:       req.Internal,
		Attachable:     req.Attachable,
		IPAM: &networktypes.IPAM{
			Driver:  "default",
			Options: req.IPAMOptions,
			Config:  ipamConfigs,
		},
		Options: options,
		Labels:  req.Labels,
	}
}

func buildNetworkCreateOptionsFromResource(resource types.NetworkResource) types.NetworkCreate {
	ipam := resource.IPAM
	options := map[string]string{}
	for key, value := range resource.Options {
		options[key] = value
	}
	labels := map[string]string{}
	for key, value := range resource.Labels {
		labels[key] = value
	}

	return types.NetworkCreate{
		CheckDuplicate: true,
		Driver:         normalizeNetworkDriver(resource.Driver),
		EnableIPv6:     resource.EnableIPv6,
		Internal:       resource.Internal,
		Attachable:     resource.Attachable,
		Ingress:        resource.Ingress,
		IPAM:           &ipam,
		Options:        options,
		Labels:         labels,
	}
}

func isDefaultNetworkName(name string) bool {
	switch strings.TrimSpace(name) {
	case "bridge", "host", "none":
		return true
	default:
		return false
	}
}

func RegisterNetworkRoutes(r *gin.RouterGroup) {
	group := r.Group("/networks")
	{
		group.GET("", listNetworks)
		group.GET("/:id", getNetwork)
		group.POST("", createNetwork)
		group.PUT("/:id", updateNetwork)
		group.DELETE("/:id", removeNetwork)
		group.POST("/bridge/enable-ipv6", enableDefaultBridgeIPv6)
		group.POST("/prune", pruneNetworks)
	}
}

func pruneNetworks(c *gin.Context) {
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	report, err := cli.NetworksPrune(c.Request.Context(), filters.Args{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "清理网络失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "已清理未使用的网络",
		"deletedNetworks": report.NetworksDeleted,
	})
}

func listNetworks(c *gin.Context) {
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	// 使用超时上下文（派生自请求上下文，客户端断开可取消）
	ctx, cancel := docker.WithShortTimeoutFrom(c.Request.Context())
	defer cancel()

	// 获取网络列表
	networks, err := cli.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取网络列表失败", err)
		return
	}

	// 检查是否只需要简要信息
	brief := c.Query("brief") == "true"
	if brief {
		containers, listErr := cli.ContainerList(ctx, types.ContainerListOptions{All: true})
		if listErr == nil {
			for i := range networks {
				if networks[i].Containers == nil {
					networks[i].Containers = make(map[string]types.EndpointResource)
				}
			}
			indexByName := make(map[string]int, len(networks))
			for i, network := range networks {
				indexByName[network.Name] = i
			}
			for _, container := range containers {
				if container.NetworkSettings == nil {
					continue
				}
				name := container.ID
				if len(container.Names) > 0 {
					name = strings.TrimPrefix(container.Names[0], "/")
				}
				for networkName, endpoint := range container.NetworkSettings.Networks {
					idx, ok := indexByName[networkName]
					if !ok {
						continue
					}
					resource := types.EndpointResource{Name: name}
					if endpoint != nil {
						resource.EndpointID = endpoint.EndpointID
						resource.MacAddress = endpoint.MacAddress
						resource.IPv4Address = endpoint.IPAddress
						if endpoint.IPPrefixLen > 0 && resource.IPv4Address != "" {
							resource.IPv4Address = resource.IPv4Address + "/" + strconv.Itoa(endpoint.IPPrefixLen)
						}
						resource.IPv6Address = endpoint.GlobalIPv6Address
						if endpoint.GlobalIPv6PrefixLen > 0 && resource.IPv6Address != "" {
							resource.IPv6Address = resource.IPv6Address + "/" + strconv.Itoa(endpoint.GlobalIPv6PrefixLen)
						}
					}
					networks[idx].Containers[container.ID] = resource
				}
			}
		}
		c.JSON(http.StatusOK, networks)
		return
	}

	// 并发获取每个网络的详细信息
	type result struct {
		index   int
		network types.NetworkResource
		err     error
	}

	resultChan := make(chan result, len(networks))
	semaphore := make(chan struct{}, 5) // 限制并发数

	for i, network := range networks {
		go func(idx int, netID string) {
			semaphore <- struct{}{}        // 获取信号量
			defer func() { <-semaphore }() // 释放信号量

			ctx, cancel := docker.WithMediumTimeout()
			defer cancel()

			networkDetail, err := cli.NetworkInspect(ctx, netID, types.NetworkInspectOptions{})
			resultChan <- result{
				index:   idx,
				network: networkDetail,
				err:     err,
			}
		}(i, network.ID)
	}

	// 收集结果
	for i := 0; i < len(networks); i++ {
		res := <-resultChan
		if res.err == nil {
			networks[res.index] = res.network
		}
	}

	c.JSON(http.StatusOK, networks)
}

func getNetwork(c *gin.Context) {
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	ctx, cancel := docker.WithMediumTimeoutFrom(c.Request.Context())
	defer cancel()

	network, err := cli.NetworkInspect(ctx, c.Param("id"), types.NetworkInspectOptions{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取网络详情失败", err)
		return
	}

	c.JSON(http.StatusOK, network)
}

func createNetwork(c *gin.Context) {
	var req networkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Driver = normalizeNetworkDriver(req.Driver)
	if req.Name == "" {
		respondError(c, http.StatusBadRequest, "网络名称不能为空", nil)
		return
	}

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	ctx := c.Request.Context()

	existing, _ := cli.NetworkList(ctx, types.NetworkListOptions{})
	for _, n := range existing {
		if n.Name == req.Name {
			respondError(c, http.StatusBadRequest, "同名网络已存在", nil)
			return
		}
	}

	createOpts := buildNetworkCreateOptions(req, req.Driver)
	resp, err := cli.NetworkCreate(ctx, req.Name, createOpts)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "创建网络失败", err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func removeNetwork(c *gin.Context) {
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	id := c.Param("id")
	ctx := c.Request.Context()

	// 获取网络信息
	network, err := cli.NetworkInspect(ctx, id, types.NetworkInspectOptions{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取网络信息失败", err)
		return
	}

	// 检查是否为默认网络
	if isDefaultNetworkName(network.Name) {
		respondError(c, http.StatusBadRequest, "不能删除默认网络", nil)
		return
	}
	if len(network.Containers) > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"code":       "NETWORK_IN_USE",
			"message":    "网络仍有容器连接，拒绝删除",
			"error":      "网络仍有容器连接，拒绝删除",
			"containers": network.Containers,
		})
		return
	}

	if err := cli.NetworkRemove(ctx, id); err != nil {
		respondError(c, http.StatusInternalServerError, "删除网络失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "网络已删除"})
}

func enableDefaultBridgeIPv6(c *gin.Context) {
	var req struct {
		FixedCIDRv6 string `json:"fixedCIDRv6"`
	}
	_ = c.ShouldBindJSON(&req)
	cfg := &docker.DaemonConfig{
		IPv6:        true,
		FixedCIDRv6: req.FixedCIDRv6,
	}
	if err := docker.UpdateDaemonConfig(cfg); err != nil {
		respondError(c, http.StatusInternalServerError, "更新 Docker 配置失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已启用默认 bridge IPv6，请重启 Docker 服务"})
}

func updateNetwork(c *gin.Context) {
	id := c.Param("id")
	var req networkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()
	ctx := c.Request.Context()

	network, err := cli.NetworkInspect(ctx, id, types.NetworkInspectOptions{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取网络信息失败", err)
		return
	}

	// 禁止修改默认网络
	if isDefaultNetworkName(network.Name) {
		respondError(c, http.StatusBadRequest, "不能修改默认网络", nil)
		return
	}

	// 若网络仍连接容器，提示先断开
	if len(network.Containers) > 0 {
		respondError(c, http.StatusBadRequest, "该网络已连接容器，无法修改，请先断开所有容器", nil)
		return
	}

	createOpts := buildNetworkCreateOptions(req, network.Driver)

	// 删除旧网络
	if err := cli.NetworkRemove(ctx, network.ID); err != nil {
		respondError(c, http.StatusInternalServerError, "删除旧网络失败", err)
		return
	}

	// 重新创建网络，保留原名称与驱动。Docker 不支持就地修改网络 IPAM。
	resp, err := cli.NetworkCreate(ctx, network.Name, createOpts)
	if err != nil {
		rollbackCtx, cancelRollback := newNetworkRollbackContext()
		defer cancelRollback()
		if _, rollbackErr := cli.NetworkCreate(rollbackCtx, network.Name, buildNetworkCreateOptionsFromResource(network)); rollbackErr != nil {
			respondError(c, http.StatusInternalServerError, "创建新网络失败，且旧网络回滚失败", rollbackErr)
			return
		}
		respondError(c, http.StatusInternalServerError, "创建新网络失败，已回滚旧网络", err)
		return
	}

	c.JSON(http.StatusOK, resp)
}
