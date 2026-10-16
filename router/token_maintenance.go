package router

import (
	"github.com/ForceMind/MyAPI/controller"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/gin-gonic/gin"
)

func registerTokenMaintenanceRoutes(tokenRoute *gin.RouterGroup) {
	maintenance := tokenRoute.Group("")
	maintenance.Use(middleware.RootAuth(), middleware.DisableCache(), middleware.TokenMaintenanceSessionAuth())
	maintenance.GET("/:id/maintenance", controller.GetTokenMaintenance)
	maintenance.POST("/:id/account-migration/preview", middleware.DashboardSessionOriginGuard(), middleware.CriticalRateLimit(), controller.PreviewTokenAccountMigration)
	maintenance.POST("/:id/account-migration", middleware.DashboardSessionOriginGuard(), middleware.CriticalRateLimit(), controller.CommitTokenAccountMigration)
	maintenance.GET("/:id/maintenance/operations/:operation_id", controller.GetTokenMaintenanceOperation)
}
