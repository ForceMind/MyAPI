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
		if mode == QuotaWriterModeAuthoritative {
			receipt, err := ReserveAccountQuota(ctx, db, AccountQuotaReserveInput{RequestID: id, UserID: user.Id, TokenID: token.Id, RequestedQuota: 50, BillingPreference: "wallet_only", BillingContext: AccountBillingContext{Version: 1, OriginModelName: "self-use-fixture", BillingPreference: "wallet_only"}})
			require.NoError(t, err)
			assert.Equal(t, BillingSourceSelfUse, receipt.BillingSource)
		} else {
			require.NoError(t, ClaimLegacyUsageRequest(ctx, db, id, user.Id, token.Id))
			reserved, err := TryReserveTokenQuota(token.Id, token.Key, 50, false)
			require.NoError(t, err)
			require.True(t, reserved)
			require.NoError(t, BindLegacyUsageReservation(ctx, db, LegacyUsageReservation{RequestID: id, UserID: user.Id, TokenID: token.Id, FundingSource: BillingSourceSelfUse, ReservedQuota: 50, TokenReservedQuota: 50, ModelName: "self-use-fixture"}))
			_, err = EnsureAccountQuotaRefundFact(ctx, db, AccountQuotaRefundFactInput{EventKey: "forbidden-wallet-refund:" + id, RequestID: id, Kind: AccountQuotaRefundFactKindLegacyWallet, UserID: user.Id, TokenID: token.Id, WalletQuota: 50, TokenQuota: 50})
			require.Error(t, err, "self-use cannot manufacture wallet credits")
		}
		budgetID, err := budgetDigest(id + "budget")
		require.NoError(t, err)
		_, err = ConfigureTokenBudget(ctx, db, rootID, TokenBudgetPolicyInput{ID: budgetID, TokenID: token.Id, Enabled: true, Limit: 100, Fee: &FeeBudgetPolicyInput{Enabled: true, LimitUSD: "0.001"}})
		require.NoError(t, err)
		require.NoError(t, ReserveTokenBudget(ctx, db, TokenBudgetReservation{RequestID: id, UserID: user.Id, TokenID: token.Id, ChannelID: 7, ModelName: "self-use-fixture", PayloadSHA256: strings.Repeat("a", 64), BoundSource: TokenBudgetBoundOpenAIResponses, InputTokens: 10, MaxOutputTokens: 20, PricingEvidence: `{"strict_token_budget":true}`, RequestServiceTier: "default", FeeEnabled: true, FeeReservedUSD: "0.0003", FeePriceEvidence: `{"scope":"synthetic-three-db"}`}))
		_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: token.Id, RequestID: id, Action: "send"})
		require.NoError(t, err)
		amount := "0.00012"
		_, err = PrepareTokenBudgetUsageReview(ctx, db, rootID, token.Id, id, &amount)
		require.NoError(t, err)
		changeID, err := budgetDigest(id + "restore-limit")
		require.NoError(t, err)
		_, err = ConfigureUserUsagePolicy(ctx, db, rootID, UserUsagePolicyInput{ID: changeID, UserID: user.Id, ExpectedRevision: policy.Revision, NoBalance: false})
		require.NoError(t, err)
		_, err = ReconcileUsageReview(ctx, db, rootID, id, 20, "verified synthetic self-use proof", UsageReviewTokenCounts{Input: 10, Output: 5})
		require.ErrorIs(t, err, ErrFeeBudgetInvalid)
		for range 2 {
			review, err := ReconcileUsageReview(ctx, db, rootID, id, 20, "verified synthetic self-use proof", UsageReviewTokenCounts{Input: 10, Output: 5, FeeUSD: &amount})
			require.NoError(t, err)
			require.NoError(t, ProjectUsageReviewDecision(ctx, db, db, review.Decision.ID))
		}
		require.NoError(t, db.First(&user, user.Id).Error)
		require.NoError(t, db.First(&token, token.Id).Error)
		assert.Zero(t, user.Quota, "no charge or credit is applied to the user wallet")
		assert.Equal(t, 20, user.UsedQuota)
		assert.Equal(t, 1, user.RequestCount)
		assert.Equal(t, 80, token.RemainQuota)
		assert.Equal(t, 20, token.UsedQuota)
		budget, err := LookupTokenBudget(ctx, db, token.Id)
		require.NoError(t, err)
		assert.EqualValues(t, 15, budget.Used)
		assert.Equal(t, amount, budget.FeeUsedUSD)
		assert.Zero(t, budget.Reserved)
		assert.Equal(t, "0", budget.FeeReservedUSD)
		var logs []Log
		require.NoError(t, db.Where("request_id = ? AND type = ?", id, LogTypeConsume).Find(&logs).Error)
		require.Len(t, logs, 1)
		assert.Contains(t, logs[0].Other, `"billing_source":"self_use"`)
	}
}
