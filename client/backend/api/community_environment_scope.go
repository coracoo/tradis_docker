//go:build community

package api

import (
	"net/http"

	"dockerpanel/backend/pkg/database"

	"github.com/gin-gonic/gin"
)

// CommunityEnvironmentRoutingMiddleware makes the local-only target explicit.
// A stale remote target must fail rather than silently running against the NAS
// hosting the community panel.
func CommunityEnvironmentRoutingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		environmentID, valid := requestEnvironmentScope(c)
		if !valid {
			c.Abort()
			return
		}
		if environmentID != database.LocalEnvironmentID {
			respondErrorWithCode(c, http.StatusBadRequest, "environment_unsupported", "社区版本仅支持管理本机环境", nil)
			c.Abort()
			return
		}
		c.Request.Header.Set(remoteEnvironmentHeader, database.LocalEnvironmentID)
		c.Next()
	}
}
