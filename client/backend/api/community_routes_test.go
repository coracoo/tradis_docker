//go:build community

package api

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterCommunityRoutesRegistersLocalRoutesWithoutCommercialRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	protected := router.Group("/api")

	RegisterCommunityRoutes(router, protected)

	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	for _, route := range []string{
		"GET /api/containers",
		"GET /api/images",
		"POST /api/images/mirrors/check",
		"GET /api/volumes",
		"GET /api/networks",
		"GET /api/compose/projects",
		"GET /api/appstore/apps",
		"GET /api/settings",
		"GET /api/scheduled-tasks",
		"GET /api/notification-channels",
		"GET /api/containers/:id/exec",
		"POST /api/ai/navigation/enrich",
		"POST /api/ai/compose/generate",
		"POST /api/ai/test",
		"POST /api/ai/models",
		"GET /api/nas/list",
		"GET /api/nas/topics/:slug",
	} {
		if _, ok := routes[route]; !ok {
			t.Errorf("community route %q was not registered", route)
		}
	}

	for _, route := range []string{
		"GET /api/license/status",
		"GET /api/github-apps/search",
		"GET /api/ai/agent/sessions",
		"GET /api/environments",
		"GET /api/protection/policies",
		"POST /api/safe-updates/projects/:name/tasks",
		"POST /api/nas/device",
		"GET /api/nas/redirect/:id",
		"GET /api/self-update/status",
	} {
		if _, ok := routes[route]; ok {
			t.Errorf("commercial route %q was registered", route)
		}
	}
}
