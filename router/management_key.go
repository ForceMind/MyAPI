package router

import (
	"github.com/ForceMind/MyAPI/controller"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/gin-gonic/gin"
)

func registerManagementKeyRoutes(api *gin.RouterGroup) {
	api.GET("/management/capabilities", middleware.AdminAuth(), middleware.DisableCache(), middleware.ManagementCapabilities)
	keys := api.Group("/management/keys", middleware.AdminAuth(), middleware.RequireManagementSession(), middleware.DisableCache())
	keys.GET("", controller.ListManagementKeys)
	keys.POST("", middleware.DashboardSessionOriginGuard(), middleware.CriticalRateLimit(), controller.CreateManagementKey)
	keys.DELETE("/:id", middleware.DashboardSessionOriginGuard(), middleware.CriticalRateLimit(), controller.RevokeManagementKey)
}
