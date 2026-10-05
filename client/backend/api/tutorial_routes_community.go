//go:build community

package api

import "github.com/gin-gonic/gin"

func registerEditionTutorialRoutes(r *gin.RouterGroup) {
	RegisterCommunityTutorialRoutes(r)
}
