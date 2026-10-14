package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Runs inside the existing guarded disposable SQLite/MySQL/PostgreSQL suite.
// It exercises real accounting writes, not service or database mocks.
func selfUseCoupledReviewDatabaseContract(t *testing.T, db *gorm.DB, rootID int, namespace string) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&UserUsagePolicyChange{}, &User{}, &LegacyUsageReservation{}))
	require.NoError(t, ValidateUserUsagePolicySchema(db))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeDisabled)
		return err
	}))
	oldBatch := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	t.Cleanup(func() { common.BatchUpdateEnabled = oldBatch })
	InitColumnNamesForTest()
	ctx := context.Background()
	channel := Channel{Name: namespace + "-self-use-review", Key: namespace + "-self-use-review", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	require.Positive(t, channel.Id)
	for index, mode := range []QuotaWriterMode{QuotaWriterModeAuthoritative, QuotaWriterModeLegacy} {
		epoch, err := GetQuotaWriterEpochState(db)
		require.NoError(t, err)
		setQuotaWriterStateForTest(t, db, mode, epoch.Epoch+1)
		id := fmt.Sprintf("%ss%d", namespace, index)
		user := User{Username: id, AffCode: id, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 0}
		require.NoError(t, db.Create(&user).Error)
		token := Token{UserId: user.Id, Key: id, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100}
		require.NoError(t, db.Create(&token).Error)
		operationID, err := budgetDigest(id + "policy")
		require.NoError(t, err)
		policy, err := ConfigureUserUsagePolicy(ctx, db, rootID, UserUsagePolicyInput{ID: operationID, UserID: user.Id, NoBalance: true})
		require.NoError(t, err)
		replay, err := ConfigureUserUsagePolicy(ctx, db, rootID, UserUsagePolicyInput{ID: operationID, UserID: user.Id, NoBalance: true})
		require.NoError(t, err)
		assert.Equal(t, policy, replay)
		var reserveReceiptID int64
		if mode == QuotaWriterModeAuthoritative {
			receipt, err := ReserveAccountQuota(ctx, db, AccountQuotaReserveInput{RequestID: id, UserID: user.Id, TokenID: token.Id, RequestedQuota: 50, BillingPreference: "wallet_only", BillingContext: AccountBillingContext{Version: 1, OriginModelName: "self-use-fixture", BillingPreference: "wallet_only"}})
			require.NoError(t, err)
			assert.Equal(t, BillingSourceSelfUse, receipt.BillingSource)
			assert.Zero(t, receipt.BillingContext.ChannelID)
			reserveReceiptID = receipt.ID
		} else {
			require.NoError(t, ClaimLegacyUsageRequest(ctx, db, id, user.Id, token.Id))
			reserved, err := TryReserveTokenQuota(token.Id, token.Key, 50, false)
			require.NoError(t, err)
			require.True(t, reserved)
			require.NoError(t, BindLegacyUsageReservation(ctx, db, LegacyUsageReservation{RequestID: id, UserID: user.Id, TokenID: token.Id, FundingSource: BillingSourceSelfUse, ReservedQuota: 50, TokenReservedQuota: 50, ModelName: "self-use-fixture"}))
			legacy, err := FindLegacyUsageReservation(ctx, db, id)
			require.NoError(t, err)
			assert.Zero(t, legacy.ChannelID)
			_, err = EnsureAccountQuotaRefundFact(ctx, db, AccountQuotaRefundFactInput{EventKey: "forbidden-wallet-refund:" + id, RequestID: id, Kind: AccountQuotaRefundFactKindLegacyWallet, UserID: user.Id, TokenID: token.Id, WalletQuota: 50, TokenQuota: 50})
			require.Error(t, err, "self-use cannot manufacture wallet credits")
		}
		budgetID, err := budgetDigest(id + "budget")
		require.NoError(t, err)
		_, err = ConfigureTokenBudget(ctx, db, rootID, TokenBudgetPolicyInput{ID: budgetID, TokenID: token.Id, Enabled: true, Limit: 100, Fee: &FeeBudgetPolicyInput{Enabled: true, LimitUSD: "0.001"}})
		require.NoError(t, err)
		require.NoError(t, ReserveTokenBudget(ctx, db, TokenBudgetReservation{RequestID: id, UserID: user.Id, TokenID: token.Id, ChannelID: channel.Id, ModelName: "self-use-fixture", PayloadSHA256: strings.Repeat("a", 64), BoundSource: TokenBudgetBoundOpenAIResponses, InputTokens: 10, MaxOutputTokens: 20, PricingEvidence: `{"strict_token_budget":true}`, RequestServiceTier: "default", FeeEnabled: true, FeeReservedUSD: "0.0003", FeePriceEvidence: `{"scope":"synthetic-three-db"}`}))
		_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: token.Id, RequestID: id, Action: "send"})
		require.NoError(t, err)
		amount := "0.00012"
		preparedReview, err := PrepareTokenBudgetUsageReview(ctx, db, rootID, token.Id, id, &amount)
		require.NoError(t, err)
		assert.Equal(t, channel.Id, preparedReview.ChannelID)
		assert.Equal(t, "self-use-fixture", preparedReview.ModelName)
		changeID, err := budgetDigest(id + "restore-limit")
		require.NoError(t, err)
		_, err = ConfigureUserUsagePolicy(ctx, db, rootID, UserUsagePolicyInput{ID: changeID, UserID: user.Id, ExpectedRevision: policy.Revision, NoBalance: false})
		require.NoError(t, err)
		_, err = ReconcileUsageReview(ctx, db, rootID, id, 20, "verified synthetic self-use proof", UsageReviewTokenCounts{Input: 10, Output: 5})
		require.ErrorIs(t, err, ErrFeeBudgetInvalid)
		var review *UsageReviewDetail
		for range 2 {
			review, err = ReconcileUsageReview(ctx, db, rootID, id, 20, "verified synthetic self-use proof", UsageReviewTokenCounts{Input: 10, Output: 5, FeeUSD: &amount})
			require.NoError(t, err)
			require.NoError(t, ProjectUsageReviewDecision(ctx, db, db, review.Decision.ID))
		}
		require.NotNil(t, review)
		require.NotNil(t, review.Decision)
		assert.Equal(t, channel.Id, review.Decision.ChannelID)
		assert.Equal(t, "self-use-fixture", review.Decision.ModelName)
		if mode == QuotaWriterModeAuthoritative {
			var reserve AccountQuotaMutationReceipt
			require.NoError(t, db.First(&reserve, reserveReceiptID).Error)
			assert.Zero(t, reserve.BillingContext.ChannelID, "read attribution must not rewrite the immutable reserve receipt")
		} else {
			legacy, err := FindLegacyUsageReservation(ctx, db, id)
			require.NoError(t, err)
			assert.Zero(t, legacy.ChannelID, "read attribution must not rewrite the legacy reservation")
		}
		require.NoError(t, db.First(&user, user.Id).Error)
		require.NoError(t, db.First(&token, token.Id).Error)
		assert.Zero(t, user.Quota, "no charge or credit is applied to the user wallet")
		assert.Equal(t, int64(20), user.UsedQuota)
		assert.Equal(t, 1, user.RequestCount)
		assert.Equal(t, int64(80), token.RemainQuota)
		assert.Equal(t, int64(20), token.UsedQuota)
		budget, err := LookupTokenBudget(ctx, db, token.Id)
		require.NoError(t, err)
		assert.EqualValues(t, 15, budget.Used)
		assert.Equal(t, amount, budget.FeeUsedUSD)
		assert.Zero(t, budget.Reserved)
		assert.Equal(t, "0", budget.FeeReservedUSD)
		var logs []Log
		require.NoError(t, db.Where("request_id = ? AND type = ?", id, LogTypeConsume).Find(&logs).Error)
		require.Len(t, logs, 1)
		assert.Equal(t, channel.Id, logs[0].ChannelId)
		assert.Equal(t, "self-use-fixture", logs[0].ModelName)
		assert.Contains(t, logs[0].Other, `"billing_source":"self_use"`)
		require.NoError(t, db.First(&channel, channel.Id).Error)
		assert.Equal(t, int64((index+1)*20), channel.UsedQuota, "each reviewed request updates channel statistics exactly once")
	}
}
