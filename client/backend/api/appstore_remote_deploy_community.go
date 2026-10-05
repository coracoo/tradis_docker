//go:build community

package api

import "github.com/gin-gonic/gin"

// The request middleware rejects remote targets before this hook. Keeping the
// local deployment path explicit prevents a stale client from becoming a local
// deployment by accident.
func handleEditionAppStoreDeployment(_ *gin.Context, _ string, _ DeployRequest) bool { return false }
