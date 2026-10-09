package router

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/require"
)

func TestTokenBudgetRecoveryRejectsAppliedAmountBeforePrepare(t *testing.T) {
	f := setupChatBudgetFlow(t)
	db := model.DB
	ctx := context.Background()
	token := model.Token{UserId: f.user.Id, Key: "applied-review-key", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000}
	require.NoError(t, db.Create(&token).Error)
	id := "applied-review-request"
	_, err := model.PrepareLegacyUsageReservation(ctx, db, model.LegacyUsageReservation{RequestID: id, UserID: f.user.Id, TokenID: token.Id, FundingSource: "wallet"})
	require.NoError(t, err)
	intent, err := model.EnsureAccountQuotaSettlementIntent(ctx, db, model.AccountQuotaSettlementFactInput{EventKey: "billing-settlement:" + id + ":v1", RequestID: id, Kind: model.AccountQuotaSettlementKindLegacyWallet, UserID: f.user.Id, TokenID: token.Id, Delta: 60, ApplyToken: true})
	require.NoError(t, err)
	_, _, err = model.RecoverAccountQuotaSettlementIntent(ctx, db, intent, "endpoint-review")
	require.NoError(t, err)
	_, err = model.ConfigureTokenBudget(ctx, db, f.root.Id, model.TokenBudgetPolicyInput{ID: strings.Repeat("e", 64), TokenID: token.Id, Enabled: true, Limit: 100})
	require.NoError(t, err)
	require.NoError(t, model.ReserveTokenBudget(ctx, db, model.TokenBudgetReservation{RequestID: id, UserID: f.user.Id, TokenID: token.Id, ChannelID: 7, ModelName: "fixture", PayloadSHA256: strings.Repeat("a", 64), BoundSource: model.TokenBudgetBoundOpenAIResponses, InputTokens: 10, MaxOutputTokens: 20, PricingEvidence: `{"strict_token_budget":true}`}))
	_, err = model.MutateTokenBudgetRequest(ctx, db, model.TokenBudgetMutation{TokenID: token.Id, RequestID: id, Action: "send"})
	require.NoError(t, err)
	before, err := model.FindLegacyUsageReservation(ctx, db, id)
	require.NoError(t, err)
	beforeBudget, err := model.LookupTokenBudget(ctx, db, token.Id)
	require.NoError(t, err)
	var beforeUser model.User
	require.NoError(t, db.First(&beforeUser, f.user.Id).Error)
	path := fmt.Sprintf("/api/token/%d/budget/recover", token.Id)
	body := fmt.Sprintf(`{"request_id":%q,"action":"reconcile","actual_quota":0,"actual_input_tokens":10,"actual_output_tokens":5,"evidence_reference":"verified counts, wrong account amount","confirmed_reliable_evidence":true}`, id)
	response := f.request(http.MethodPost, path, *f.root.AccessToken, body)
	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	after, err := model.FindLegacyUsageReservation(ctx, db, id)
	require.NoError(t, err)
	require.Equal(t, before, after, "rejected amount cannot prepare or modify the quota journal")
	afterBudget, err := model.LookupTokenBudget(ctx, db, token.Id)
	require.NoError(t, err)
	require.Equal(t, beforeBudget, afterBudget)
	var decisions int64
	require.NoError(t, db.Model(&model.UsageReviewDecision{}).Where("request_id = ?", id).Count(&decisions).Error)
	require.Zero(t, decisions)
	var afterUser model.User
	require.NoError(t, db.First(&afterUser, f.user.Id).Error)
	require.Equal(t, beforeUser.Quota, afterUser.Quota)
	require.NoError(t, db.First(&token, token.Id).Error)
	require.Equal(t, 940, token.RemainQuota)
}
