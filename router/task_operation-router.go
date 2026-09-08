package router

import (
	"github.com/ForceMind/MyAPI/controller"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/gin-gonic/gin"
)

// SetTaskOperationRouter registers the isolated, read-only durable operation
// lookup. It has no submission, billing, upstream, distribution or feature-gate
// middleware.
func SetTaskOperationRouter(router *gin.Engine) {
	operationRouter := router.Group("/v1/task-operations")
	operationRouter.Use(middleware.DisableCache())
	operationRouter.Use(middleware.RouteTag("relay"))
	operationRouter.Use(middleware.TaskOperationAuth())
	operationRouter.GET("/:id", controller.GetTaskOperation)
}
