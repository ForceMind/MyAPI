package router

import (
	"github.com/ForceMind/MyAPI/controller"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/gin-gonic/gin"
)

// registerChannelQuotaAlertRoutes attaches the root-only delivery operations
// to the existing /api/channel group, which already carries AdminAuth.
func registerChannelQuotaAlertRoutes(channelRoute *gin.RouterGroup) {
	readMiddleware := []gin.HandlerFunc{
		middleware.RootAuth(),
		middleware.DisableCache(),
	}
	writeMiddleware := []gin.HandlerFunc{
		middleware.RootAuth(),
		middleware.DisableCache(),
		middleware.DashboardSessionOriginGuard(),
		middleware.CriticalRateLimit(),
		middleware.SecureVerificationRequired(),
	}

	channelRoute.GET("/quota/alerts", append(readMiddleware, controller.GetChannelQuotaAlertDeliveryEvents)...)
	channelRoute.GET("/quota/alerts/delivery", append(readMiddleware, controller.GetChannelQuotaAlertDeliveryStatus)...)
	channelRoute.PUT("/quota/alerts/delivery", append(writeMiddleware, controller.UpdateChannelQuotaAlertDeliverySettings)...)
	channelRoute.POST("/quota/alerts/delivery/run", append(writeMiddleware, controller.RunChannelQuotaAlertDelivery)...)
}
