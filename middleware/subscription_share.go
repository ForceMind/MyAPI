package middleware

import (
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
)

// All authenticated entry points pass here, including media and WebSocket
// routes that bypass the shared HTTP adaptor. Model discovery is read-only.
func checkSubscriptionShareAdmission(c *gin.Context, token *model.Token) bool {
	bound, err := model.HasSubscriptionShareBinding(c.Request.Context(), model.DB, token.Id)
	if err != nil {
		abortWithOpenAiMessage(c, http.StatusServiceUnavailable, common.TranslateMessage(c, i18n.MsgSubscriptionShareUnavailable), types.ErrorCode("subscription_share_unavailable"))
		return false
	}
	c.Request = c.Request.WithContext(common.WithSubscriptionShareAdmission(c.Request.Context(), common.SubscriptionShareAdmission{TokenID: token.Id, Bound: bound}))
	if !bound {
		return true
	}
	path := c.Request.URL.Path
	if c.Request.Method == http.MethodGet && (path == "/v1/models" || strings.HasPrefix(path, "/v1/models/") || path == "/v1beta/models" || path == "/v1beta/openai/models") {
		return true
	}
	if c.Request.Method == http.MethodPost && c.Request.URL.RawQuery == "" && (path == "/v1/chat/completions" || path == "/v1/responses" || path == "/v1/responses/compact" || path == "/v1/alpha/search") {
		return true
	}
	abortWithOpenAiMessage(c, http.StatusForbidden, common.TranslateMessage(c, i18n.MsgSubscriptionShareDenied), types.ErrorCode("subscription_share_denied"))
	return false
}
