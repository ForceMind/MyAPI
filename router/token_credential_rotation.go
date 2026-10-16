package router

import (
	"github.com/ForceMind/MyAPI/controller"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/gin-gonic/gin"
)

// Rotation is available only after per-Key database qualification; the
// dedicated chain preserves Root session, Origin and security-proof checks.
func registerTokenCredentialRotationRoutes(apiRouter *gin.RouterGroup) {
	rotation := apiRouter.Group("/token")
	rotation.Use(func(c *gin.Context) {
		c.Header("X-MyAPI-Credential-Rotation-Disposition", "not_accepted")
		c.Next()
	}, middleware.UserAuth(), middleware.RootAuth(), middleware.DisableCache(), middleware.TokenMaintenanceSessionAuth())
	rotation.POST("/:id/credential-rotation/preview", middleware.DashboardSessionOriginGuard(), middleware.CriticalRateLimit(), controller.PreviewTokenCredentialRotation)
	rotation.POST("/:id/credential-rotation", middleware.DashboardSessionOriginGuard(), middleware.CriticalRateLimit(), controller.CommitTokenCredentialRotation)
	rotation.GET("/:id/credential-rotation/operations/:operation_id", controller.GetTokenCredentialRotationOperation)
}
