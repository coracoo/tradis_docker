package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/gin-gonic/gin"
)

func RegisterVolumeRoutes(r *gin.RouterGroup) {
	group := r.Group("/volumes")
	{
		group.GET("", listVolumes)
		group.POST("", createVolume)
		group.DELETE("/:name", removeVolume)
		group.POST("/prune", pruneVolumes) // 添加新路由
		group.POST("/:name/browse/start", startVolumeBrowse)
		group.GET("/browse/:sid/ui", volumeBrowseUI)
		group.POST("/browse/:sid/heartbeat", volumeBrowseHeartbeat)
		group.POST("/browse/:sid/close", volumeBrowseClose)
		group.Any("/browse/:sid/fb/*path", volumeBrowseProxy)
	}
}

// 添加清除无用卷的处理函数
func pruneVolumes(c *gin.Context) {
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	pruneReport, err := cli.VolumesPrune(c.Request.Context(), filters.Args{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "清理存储卷失败", err)
		return
	}

	if len(pruneReport.VolumesDeleted) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "没有可清除的无用存储卷",
			"report":  pruneReport,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("已清除 %d 个存储卷，释放空间 %d bytes",
			len(pruneReport.VolumesDeleted),
			pruneReport.SpaceReclaimed),
		"deletedVolumes": pruneReport.VolumesDeleted,
		"spaceReclaimed": pruneReport.SpaceReclaimed,
		"report":         pruneReport,
	})
}

// 辅助函数：获取卷名列表
func getVolumeNames(volumes []*volume.Volume) []string {
	names := make([]string, len(volumes))
	for i, v := range volumes {
		names[i] = v.Name
	}
	return names
}

// 定义一个自定义的卷信息结构体，包含容器使用信息
type VolumeInfo struct {
	*volume.Volume
	InUse      bool                    `json:"InUse"`
	Containers map[string]ContainerRef `json:"Containers"`
}

// 容器引用信息
type ContainerRef struct {
	Name string `json:"Name"`
}

func containersUsingVolume(cli interface {
	ContainerList(context.Context, types.ContainerListOptions) ([]types.Container, error)
}, volumeName string) (map[string]ContainerRef, error) {
	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}

	return containersUsingVolumeFromList(containers, volumeName), nil
}

func containersUsingVolumeFromList(containers []types.Container, volumeName string) map[string]ContainerRef {
	refs := make(map[string]ContainerRef)
	for _, container := range containers {
		for _, mount := range container.Mounts {
			if mount.Type != "volume" || strings.TrimSpace(mount.Name) != volumeName {
				continue
			}
			name := container.ID
			if len(container.Names) > 0 {
				name = strings.TrimPrefix(container.Names[0], "/")
			}
			refs[container.ID] = ContainerRef{Name: name}
		}
	}
	return refs
}

func listVolumes(c *gin.Context) {
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	// 获取卷列表
	volumeList, err := cli.VolumeList(c.Request.Context(), volume.ListOptions{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取卷列表失败", err)
		return
	}

	containers, err := cli.ContainerList(c.Request.Context(), types.ContainerListOptions{All: true})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取容器列表失败", err)
		return
	}

	// 创建增强的卷信息列表
	enhancedVolumes := make([]*VolumeInfo, 0, len(volumeList.Volumes))

	// 更新卷的使用信息
	for _, vol := range volumeList.Volumes {
		volumeInfo := &VolumeInfo{
			Volume:     vol,
			InUse:      false,
			Containers: make(map[string]ContainerRef),
		}

		refs := containersUsingVolumeFromList(containers, vol.Name)
		volumeInfo.Containers = refs
		volumeInfo.InUse = len(refs) > 0

		enhancedVolumes = append(enhancedVolumes, volumeInfo)
	}

	// 返回自定义结构的响应
	c.JSON(http.StatusOK, gin.H{
		"Volumes":  enhancedVolumes,
		"Warnings": volumeList.Warnings,
	})
}

func createVolume(c *gin.Context) {
	var req struct {
		Name       string            `json:"name" binding:"required"`
		Driver     string            `json:"driver"`
		DriverOpts map[string]string `json:"driverOpts"`
		Labels     map[string]string `json:"labels"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	name := strings.TrimSpace(req.Name)
	if name == "" {
		respondError(c, http.StatusBadRequest, "存储卷名称不能为空", nil)
		return
	}

	driver := strings.TrimSpace(req.Driver)
	if driver == "" {
		driver = "local"
	}
	vol, err := cli.VolumeCreate(c.Request.Context(), volume.CreateOptions{
		Name:       name,
		Driver:     driver,
		DriverOpts: req.DriverOpts,
		Labels:     req.Labels,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "创建存储卷失败", err)
		return
	}

	c.JSON(http.StatusOK, vol)
}

func removeVolume(c *gin.Context) {
	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	name := c.Param("name")
	force := strings.EqualFold(c.Query("force"), "true") || c.Query("force") == "1"
	if _, err := cli.VolumeInspect(c.Request.Context(), name); err != nil {
		respondError(c, http.StatusInternalServerError, "获取存储卷失败", err)
		return
	}
	if !force {
		refs, err := containersUsingVolume(cli, name)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "检查存储卷使用状态失败", err)
			return
		}
		if len(refs) > 0 {
			c.JSON(http.StatusConflict, gin.H{
				"code":       "VOLUME_IN_USE",
				"message":    "存储卷正在被容器使用，拒绝删除",
				"error":      "存储卷正在被容器使用，拒绝删除",
				"containers": refs,
			})
			return
		}
	}

	if err := cli.VolumeRemove(c.Request.Context(), name, force); err != nil {
		respondError(c, http.StatusInternalServerError, "删除存储卷失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "数据卷已删除"})
}
