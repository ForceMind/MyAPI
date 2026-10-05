package middleware

import (
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
)

// Called after normal key authentication for every token-authenticated relay,
// including task/media routes that do not use the shared HTTP adaptor.
func checkTokenBudgetAdmission(c *gin.Context, token *model.Token) bool {
	budget, err := model.LookupTokenBudget(c.Request.Context(), model.DB, token.Id)
	if err != nil {
		abortWithOpenAiMessage(c, http.StatusServiceUnavailable, common.TranslateMessage(c, i18n.MsgDatabaseError), types.ErrorCode("token_budget_unavailable"))
		return false
	}
	if budget == nil {
		return true
	}
	if budget.UserID != token.UserId {
		abortWithOpenAiMessage(c, http.StatusForbidden, common.TranslateMessage(c, i18n.MsgOperationFailed), types.ErrorCode("token_budget_identity_conflict"))
		return false
	}
	if budget.AccountThresholdEnabled {
		c.Request = c.Request.WithContext(common.WithAccountQuotaThreshold(c.Request.Context(), common.AccountQuotaThreshold{TokenID: token.Id, Revision: budget.Revision, MinimumRemainingBPS: budget.AccountMinRemainingBPS, MaxAgeSeconds: budget.AccountMaxAgeSeconds}))
	}
	if !budget.Enabled && !budget.FeeEnabled {
		return true
	}
	// Model discovery is not a generation. Other routes are unsupported until
	// their own reliable pre-send bound and complete settlement are connected.
	if c.Request.Method == http.MethodGet && (c.Request.URL.Path == "/v1/models" || strings.HasPrefix(c.Request.URL.Path, "/v1/models/") || c.Request.URL.Path == "/v1beta/models" || c.Request.URL.Path == "/v1beta/openai/models") {
		return true
	}
	if c.Request.Method != http.MethodPost || (c.Request.URL.Path != "/v1/responses" && c.Request.URL.Path != "/v1/chat/completions") || c.Request.URL.RawQuery != "" {
		abortWithOpenAiMessage(c, http.StatusBadRequest, common.TranslateMessage(c, i18n.MsgTokenBudgetUnsupported), types.ErrorCode("token_budget_unsupported_request"))
		return false
	}
	if budget.PendingRequestID != "" || budget.Reserved != 0 {
		abortWithOpenAiMessage(c, http.StatusConflict, common.TranslateMessage(c, i18n.MsgTokenBudgetPending), types.ErrorCode("token_budget_pending"))
		return false
	}
	if budget.Enabled && budget.Used >= budget.Limit {
		abortWithOpenAiMessage(c, http.StatusForbidden, common.TranslateMessage(c, i18n.MsgTokenBudgetExceeded), types.ErrorCode("token_budget_exceeded"))
		return false
	}
	if model.CheckFeeBudgetAdmission(budget) != nil {
		abortWithOpenAiMessage(c, http.StatusForbidden, common.TranslateMessage(c, i18n.MsgFeeBudgetExceeded), types.ErrorCode("token_budget_fee_exceeded"))
		return false
	}
	common.SetContextKey(c, constant.ContextKeyStrictTokenBudget, true)
	return true
}
