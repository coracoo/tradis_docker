//go:build community

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func registerEditionVersionRoutes(group *gin.RouterGroup) {
	group.POST("/version-check", checkCommunityVersionNow)
}

func editionVersionStatus() (versionStatusResponse, error) {
	return versionStatusResponse{
		LocalVersion: resolveLocalClientVersion(),
		Channel:      "community",
	}, nil
}

func checkCommunityVersionNow(c *gin.Context) {
	respondErrorWithCode(c, http.StatusNotImplemented, "community_release_manifest_unavailable", "社区版本暂不提供自动更新，请按发行说明手动更新", nil)
}
