package middleware

import (
	"fmt"

	"github.com/ForceMind/MyAPI/common"
	"github.com/gin-gonic/gin"
)

const (
	RouteTagKey              = "route_tag"
	paymentWebhookLogPathKey = "payment_webhook_log_path"
)

func RouteTag(tag string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(RouteTagKey, tag)
		c.Next()
	}
}

func SetUpLogger(server *gin.Engine) {
	accessLogger := gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var requestID string
		if param.Keys != nil {
			requestID, _ = param.Keys[common.RequestIdKey].(string)
		}
		tag, _ := param.Keys[RouteTagKey].(string)
		if tag == "" {
			tag = "web"
		}
		path := param.Path
		if webhookPath, ok := param.Keys[paymentWebhookLogPathKey].(string); ok {
			path = webhookPath
		}
		return fmt.Sprintf("[GIN] %s | %s | %s | %3d | %13v | %15s | %7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			tag,
			requestID,
			param.StatusCode,
			param.Latency,
			param.ClientIP,
			param.Method,
			path,
		)
	})
	server.Use(func(c *gin.Context) {
		// Gin appends RawQuery to its access-log path. Payment callbacks must
		// use the matched route template instead, including an untrusted :env.
		// Do not change the request used for signature verification or routing.
		switch route := c.FullPath(); route {
		case "/api/stripe/webhook", "/api/creem/webhook", "/api/waffo/webhook", "/api/waffo-pancake/webhook/:env":
			c.Set(paymentWebhookLogPathKey, route)
		}
		accessLogger(c)
	})
}
