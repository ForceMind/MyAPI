package middleware

import (
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
)

// Old task/media writers are not allowed to silently fall back to a wallet
// under the new no-balance policy. Keep those routes unchanged for legacy
// users; qualify additional protocols only with their own frozen-source tests.
func checkUserUsageAdmission(c *gin.Context, token *model.Token) bool {
	active, err := model.ResolveSelfUseNoBalanceAdmission(c.Request.Context(), model.DB, token.UserId)
	if err != nil {
		abortWithOpenAiMessage(c, http.StatusServiceUnavailable, common.TranslateMessage(c, i18n.MsgDatabaseError), types.ErrorCode("user_usage_policy_unavailable"))
		return false
	}
	if !active {
		return true
	}
	path := c.Request.URL.Path
	if c.Request.Method == http.MethodGet && (path == "/v1/models" || strings.HasPrefix(path, "/v1/models/")) {
		return true
	}
	if service.SupportsSelfUseMeteredRequest(c.Request) {
		return true
	}
	abortWithOpenAiMessage(c, http.StatusBadRequest, common.TranslateMessage(c, i18n.MsgSelfUseUnsupported), types.ErrorCode("self_use_unsupported_request"))
	return false
}
