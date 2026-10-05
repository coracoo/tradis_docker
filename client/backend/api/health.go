package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthHandler returns the locally built panel version. It remains available
// in every edition so a container orchestrator can distinguish a live panel
// from a failed startup without enabling self-update.
func HealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "version": resolveLocalClientVersion()})
}
