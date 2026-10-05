//go:build community

package main

import (
	"context"
	"testing"

	"dockerpanel/backend/pkg/background"
	"github.com/gin-gonic/gin"
)

func TestCommunityRuntimeDoesNotRegisterFullProductRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	protected := router.Group("/api")

	registerEditionRoutes(protected)

	if routes := router.Routes(); len(routes) != 0 {
		t.Fatalf("community runtime registered %d full-product routes", len(routes))
	}
}

func TestCommunityAPIRouteAssemblyUsesOnlyLocalSurface(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	registerClientAPIRoutes(router)

	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	if _, ok := routes["GET /api/containers"]; !ok {
		t.Fatal("community API assembly omitted local container routes")
	}
	if _, ok := routes["GET /api/license/status"]; ok {
		t.Fatal("community API assembly registered a license route")
	}
}

func TestCommunityRuntimeSkipsFullProductLifecycle(t *testing.T) {
	if handled, exitCode := handleEditionUpdater(); handled || exitCode != 0 {
		t.Fatalf("community runtime entered updater mode: handled=%v exitCode=%d", handled, exitCode)
	}
	if err := prepareEditionRuntime(); err != nil {
		t.Fatalf("community runtime preparation failed: %v", err)
	}
	registerEditionSchedulerCallbacks()
	if err := startEditionBackground(context.Background(), background.New(context.Background())); err != nil {
		t.Fatalf("community runtime background startup failed: %v", err)
	}
	recoverEditionState(context.Background())
	startEditionRecurring()
}

func TestCommunityCORSDoesNotExposeRemoteProtocolHeaders(t *testing.T) {
	for _, header := range append(editionCORSAllowedHeaders(), editionCORSExposedHeaders()...) {
		if header == "X-TRADIS-Environment" || header == "X-Tradis-Remote" || header == "X-Tradis-Stale" || header == "X-Tradis-Captured-At" {
			t.Fatalf("community CORS exposed remote protocol header %q", header)
		}
	}
}
