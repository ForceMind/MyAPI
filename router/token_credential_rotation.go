package router

import (
	"github.com/ForceMind/MyAPI/controller"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/gin-gonic/gin"
)

// This registrar is not connected to SetApiRouter until the isolated
// compatibility qualification has passed. Tests can exercise the complete
// route chain without exposing a production submission endpoint prematurely.
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
