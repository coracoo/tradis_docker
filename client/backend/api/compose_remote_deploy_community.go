//go:build community

package api

import (
	"net/http"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/deployment"

	"github.com/gin-gonic/gin"
)

func registerEditionComposeRoutes(_ *gin.RouterGroup) {}

func handleEditionComposeDeployment(_ *gin.Context, _ composeDeployRequest, _ string, _ deployment.DeploymentCandidate) bool {
	return false
}

func handleEditionComposeTaskCancel(c *gin.Context, environmentID, _ string) bool {
	if environmentID == database.LocalEnvironmentID {
		return false
	}
	respondErrorWithCode(c, http.StatusBadRequest, "environment_unsupported", "社区版本仅支持管理本机环境", nil)
	return true
}
