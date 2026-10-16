package middleware

import (
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/gin-gonic/gin"
)

// TokenMaintenanceSessionAuth follows RootAuth and excludes opaque console
// access tokens from credential maintenance, including its read-only preview.
func TokenMaintenanceSessionAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := GetSessionAuthIdentity(c)
		if !ok || c.GetBool("use_access_token") || identity.UserID != c.GetInt("id") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"code":    "token_maintenance_session_required",
				"message": common.TranslateMessage(c, i18n.MsgAuthNotLoggedIn),
			})
			return
		}
		c.Next()
	}
}
