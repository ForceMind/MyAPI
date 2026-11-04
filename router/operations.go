package router

import (
	"github.com/ForceMind/MyAPI/controller"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/gin-gonic/gin"
)

func registerOperationsRoutes(api *gin.RouterGroup) {
	routes := api.Group("/operations", middleware.RootAuth(), middleware.OperationsSessionRequired(), middleware.DisableCache())
	routes.GET("/plans", controller.ListOperationsPlans)
	routes.GET("/plans/:id", controller.GetOperationsPlan)
	routes.GET("/instances", controller.ListOperationsInstances)
	routes.POST("/plans/:id/approve", middleware.DashboardSessionOriginGuard(), middleware.CriticalRateLimit(), controller.ApproveOperationsPlan)
}
