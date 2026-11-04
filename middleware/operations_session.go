package middleware

import (
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
)

func OperationsSessionRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := GetSessionAuthIdentity(c)
		if !ok || c.GetBool("use_access_token") {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "OPERATIONS_SESSION_REQUIRED", "message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege)})
			return
		}
		owner, err := model.GetUserById(identity.UserID, false)
		if err != nil || owner.Status != common.UserStatusEnabled || owner.Role != common.RoleRootUser {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "OPERATIONS_SESSION_REQUIRED", "message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege)})
			return
		}
		c.Next()
	}
}
