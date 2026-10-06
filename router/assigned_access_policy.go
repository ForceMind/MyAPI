package router

import (
	"github.com/ForceMind/MyAPI/controller"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/gin-gonic/gin"
)

func registerAssignedAccessPolicyRoutes(api *gin.RouterGroup) {
	admin := api.Group("/access-policy")
	admin.Use(middleware.AdminAuth(), middleware.DisableCache())
	admin.GET("/:subject/:id", controller.GetAssignedAccessPolicy)
	admin.PUT("/:subject/:id", middleware.DashboardSessionOriginGuard(), middleware.CriticalRateLimit(), controller.UpdateAssignedAccessPolicy)
	admin.DELETE("/:subject/:id", middleware.DashboardSessionOriginGuard(), middleware.CriticalRateLimit(), controller.DeleteAssignedAccessPolicy)
	admin.POST("/:subject/:id/preview", middleware.CriticalRateLimit(), controller.PreviewAssignedAccessPolicy)
	api.GET("/token/:id/access", middleware.UserAuth(), middleware.DisableCache(), controller.GetTokenAssignedAccess)
}
