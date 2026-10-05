//go:build community

package main

import (
	"context"
	"dockerpanel/backend/api"
	"dockerpanel/backend/pkg/background"

	"github.com/gin-gonic/gin"
)

// registerEditionRoutes deliberately has no community implementation. The
// shared route registration already installs the local-only API surface.
func registerEditionRoutes(_ *gin.RouterGroup) {}

func registerEditionRequestMiddleware(protected *gin.RouterGroup) {
	protected.Use(api.CommunityEnvironmentRoutingMiddleware())
}

func editionCORSAllowedHeaders() []string { return nil }

func editionCORSExposedHeaders() []string { return nil }

func handleEditionUpdater() (bool, int) { return false, 0 }

func prepareEditionRuntime() error { return nil }

func registerEditionSchedulerCallbacks() {}

func startEditionBackground(context.Context, *background.Runner) error { return nil }

func recoverEditionState(context.Context) {}

func startEditionRecurring() {}
