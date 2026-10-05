//go:build community

package api

import "github.com/gin-gonic/gin"

// RegisterCommercialRoutes is intentionally empty in the community build.
// Keeping the symbol avoids conditional callers while ensuring excluded route
// handlers are neither registered nor required by the local-only runtime.
func RegisterCommercialRoutes(_ *gin.RouterGroup) {}
