package controller

import (
	"math"
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
)

// Playground receives the exact same real-key context and middleware gates as
// the public relay. There is deliberately no temporary or unlimited token.
func Playground(c *gin.Context) {
	Relay(c, types.RelayFormatOpenAI)
}

type playgroundKey struct {
	StrictTokenBudget  bool   `json:"strict_token_budget"`
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	Status             int    `json:"status"`
	Group              string `json:"group"`
	AccessProfileID    string `json:"access_profile_id"`
	RemainQuota        int    `json:"remain_quota"`
	UsedQuota          int    `json:"used_quota"`
	UnlimitedQuota     bool   `json:"unlimited_quota"`
	ExpiredTime        int64  `json:"expired_time"`
	ModelLimitsEnabled bool   `json:"model_limits_enabled"`
}

// PlaygroundKeys returns selection metadata only. Never embed model.Token:
// even a masked credential is unnecessary for selecting a key by ID.
func PlaygroundKeys(c *gin.Context) {
	page := common.GetPageQuery(c)
	if page.Page < 1 {
		page.Page = 1
	}
	if page.PageSize < 1 {
		page.PageSize = 10
	}
	if page.Page > math.MaxInt/page.PageSize {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgOperationFailed)})
		return
	}
	userID := c.GetInt("id")
	tokens, err := model.GetAllUserTokens(userID, page.GetStartIdx(), page.PageSize)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	total, err := model.CountUserTokens(userID)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	items := make([]playgroundKey, 0, len(tokens))
	for _, token := range tokens {
		budget, err := model.LookupTokenBudget(c.Request.Context(), model.DB, token.Id)
		if err != nil || budget != nil && budget.UserID != userID {
			common.ApiErrorI18n(c, i18n.MsgDatabaseError)
			return
		}
		items = append(items, playgroundKey{
			StrictTokenBudget: budget != nil && (budget.Enabled || budget.FeeEnabled),
			ID:                token.Id, Name: token.Name, Status: token.Status, Group: token.Group,
			AccessProfileID: token.AccessProfileID, RemainQuota: token.RemainQuota,
			UsedQuota: token.UsedQuota, UnlimitedQuota: token.UnlimitedQuota,
			ExpiredTime: token.ExpiredTime, ModelLimitsEnabled: token.ModelLimitsEnabled,
		})
	}
	page.SetTotal(int(total))
	page.SetItems(items)
	common.ApiSuccess(c, page)
}

func PlaygroundListModels(c *gin.Context) {
	listModels(c, constant.ChannelTypeOpenAI, true)
}
