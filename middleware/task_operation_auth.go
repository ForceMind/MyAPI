package middleware

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
)

const taskOperationTokenScopedContextKey = "task_operation_token_scoped"

// TaskOperationAuth authenticates either a live dashboard session or an API
// token for the isolated read-only task-operation route. Relay TokenAuth and
// TokenAuthReadOnly are intentionally not reused: this endpoint allows
// expired/exhausted tokens while requiring an authoritative primary-database
// token lookup and continuing to reject disabled, deleted and unknown tokens.
func TaskOperationAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := authorizationToken(c.GetHeader("Authorization"))
		if !ok {
			abortWithOpenAiMessage(c, http.StatusUnauthorized, common.TranslateMessage(c, i18n.MsgTokenNotProvided))
			return
		}

		identity, internal, err := service.ParseDashboardAccessToken(raw)
		if internal {
			if err != nil {
				writeDashboardAuthError(c, err)
				return
			}
			_, user, err := service.ValidateLoginSession(identity)
			if err != nil {
				writeDashboardAuthError(c, err)
				return
			}
			setDashboardAuthContext(c, user, identity, false)
			c.Set(taskOperationTokenScopedContextKey, false)
			c.Next()
			return
		}

		key := strings.TrimPrefix(raw, "sk-")
		keyParts := strings.Split(key, "-")
		key = keyParts[0]
		if key == "" {
			abortWithOpenAiMessage(c, http.StatusUnauthorized, common.TranslateMessage(c, i18n.MsgTokenInvalid))
			return
		}
		apiIdentity, err := model.ReadTaskOperationAPIIdentity(key)
		if err != nil {
			if errors.Is(err, model.ErrTaskOperationQueryUnavailable) {
				abortWithOpenAiMessage(c, http.StatusInternalServerError, common.TranslateMessage(c, i18n.MsgDatabaseError))
				return
			}
			abortWithOpenAiMessage(c, http.StatusUnauthorized, common.TranslateMessage(c, i18n.MsgTokenInvalid))
			return
		}
		if apiIdentity == nil || apiIdentity.UserID <= 0 || apiIdentity.TokenID <= 0 || !taskOperationReadableTokenStatus(apiIdentity.TokenStatus) {
			abortWithOpenAiMessage(c, http.StatusUnauthorized, common.TranslateMessage(c, i18n.MsgTokenInvalid))
			return
		}
		if apiIdentity.UserStatus != common.UserStatusEnabled {
			abortWithOpenAiMessage(c, http.StatusForbidden, common.TranslateMessage(c, i18n.MsgAuthUserBanned))
			return
		}

		ipLimits := (&model.Token{AllowIps: apiIdentity.AllowIPs}).GetIpLimits()
		if len(ipLimits) > 0 {
			clientIP := net.ParseIP(c.ClientIP())
			if clientIP == nil || !common.IsIpInCIDRList(clientIP, ipLimits) {
				abortWithOpenAiMessage(c, http.StatusForbidden, common.TranslateMessage(c, i18n.MsgForbidden), types.ErrorCodeAccessDenied)
				return
			}
		}

		c.Set("id", apiIdentity.UserID)
		c.Set("token_id", apiIdentity.TokenID)
		c.Set(taskOperationTokenScopedContextKey, true)
		c.Next()
	}
}

func taskOperationReadableTokenStatus(status int) bool {
	switch status {
	case common.TokenStatusEnabled, common.TokenStatusExpired, common.TokenStatusExhausted:
		return true
	default:
		return false
	}
}

// GetTaskOperationTokenID returns an API-token owner scope. Dashboard session
// requests return false and must be queried by user ID only. Once marked token
// scoped, an invalid ID is returned with true so the controller fails closed
// instead of silently broadening the lookup to the whole user.
func GetTaskOperationTokenID(c *gin.Context) (int, bool) {
	if !c.GetBool(taskOperationTokenScopedContextKey) {
		return 0, false
	}
	tokenID := c.GetInt("token_id")
	return tokenID, true
}
