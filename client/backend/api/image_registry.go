package api

import (
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/logging"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
)

func RegisterImageRegistryRoutes(r *gin.RouterGroup) {
	group := r.Group("/image-registry") // 修改路由路径
	{
		group.GET("", getImageRegistries)
		group.POST("", updateImageRegistries)
	}
}

// 获取所有镜像注册表配置
func getImageRegistries(c *gin.Context) {
	registries, err := database.GetAllRegistries()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取镜像注册表配置失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"registries": registries})
}

// 更新镜像注册表配置
func updateImageRegistries(c *gin.Context) {
	raw, err := c.GetRawData()
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	var payload struct {
		Registries map[string]database.Registry `json:"registries"`
	}
	var registries map[string]database.Registry
	if err := json.Unmarshal(raw, &payload); err == nil && payload.Registries != nil {
		registries = payload.Registries
	} else if err := json.Unmarshal(raw, &registries); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	logging.Debug("registry configuration batch received", "count", len(registries))
	for key, registry := range registries {
		logging.Debug("registry configuration received", "key", key, "name", registry.Name)
	}

	// 清除现有配置
	if err := database.ClearRegistries(); err != nil {
		respondError(c, http.StatusInternalServerError, "清除镜像注册表配置失败", err)
		return
	}

	// 保存新配置
	for key, registry := range registries {
		// 确保 URL 不为空
		if registry.URL == "" {
			registry.URL = key // 如果 URL 为空，使用键作为 URL
			logging.Debug("registry address derived from configuration key", "key", key)
		}

		registry.IsDefault = (key == "docker.io")
		logging.Debug("registry configuration save started", "key", key, "name", registry.Name, "default", registry.IsDefault)

		if err := database.SaveRegistry(&registry); err != nil {
			logging.Error("registry configuration save failed", "key", key, "error", err)
			respondError(c, http.StatusInternalServerError, "保存镜像注册表配置失败", err)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "镜像注册表配置已更新"})
}
