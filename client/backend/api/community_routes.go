package api

import "github.com/gin-gonic/gin"

// RegisterCommunityRoutes registers the local-only HTTP surface shared by the
// full product and the community edition. It deliberately excludes every
// route that needs a TRADIS account, official content service, remote Agent,
// or Go-only workflow.
func RegisterCommunityRoutes(engine *gin.Engine, protected *gin.RouterGroup) {
	protected = protected.Group("", dockerDisplayInvalidationMiddleware())
	RegisterContainerRoutes(protected)
	RegisterImageRoutes(protected)
	RegisterVolumeRoutes(protected)
	RegisterNetworkRoutes(protected)
	RegisterComposeRoutes(protected)
	RegisterImageRegistryRoutes(protected)
	RegisterSystemRoutes(protected)
	RegisterNavigationRoutes(protected)
	RegisterFreeAIRoutes(protected)
	registerEditionTutorialRoutes(protected)
	RegisterSettingsRoutes(protected)
	RegisterPortRoutes(protected)
	RegisterCleanupRoutes(protected)
	RegisterScheduledTaskRoutes(protected)
	RegisterNotificationChannelRoutes(protected)
	RegisterTerminalRoutes(protected)

	RegisterAppStoreRoutes(engine)
	RegisterAppStoreProtectedRoutes(protected)
}
