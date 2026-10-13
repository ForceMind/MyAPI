package model

import (
	"context"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type strictUsageReviewAttributionFixture struct {
	db      *gorm.DB
	root    User
	account accountQuotaFixture
	receipt *AccountQuotaMutationReceipt
}

func newStrictUsageReviewAttributionFixture(t *testing.T, requestID string, billingChannelID, budgetChannelID int) strictUsageReviewAttributionFixture {
	t.Helper()
	db := openAccountQuotaTestDB(t)
	require.NoError(t, db.AutoMigrate(&UsageReviewDecision{}, &TokenBudget{}, &TokenBudgetReservation{}, &TokenBudgetPolicyChange{}, &Log{}, &BillingLogProjectionIdentity{}))
	root := User{Username: requestID + "-root", AffCode: requestID + "-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	account := newAccountQuotaFixture(t, db, requestID, 1000, 1000, false, 0, 0, false)
	receipt, err := ReserveAccountQuota(context.Background(), db, AccountQuotaReserveInput{
		RequestID: requestID, UserID: account.User.Id, TokenID: account.Token.Id, RequestedQuota: 100, BillingPreference: "wallet_only",
		BillingContext: AccountBillingContext{Version: 1, OriginModelName: "origin-model", BillingPreference: "wallet_only", ChannelID: billingChannelID},
	})
	require.NoError(t, err)
	require.NotNil(t, receipt)
	_, err = HoldAccountQuotaUnknownUsage(context.Background(), db, requestID, receipt.ID, "missing", `{"strict_token_budget":true}`)
	require.NoError(t, err)
	_, err = ConfigureTokenBudget(context.Background(), db, root.Id, TokenBudgetPolicyInput{ID: strings.Repeat("d", 64), TokenID: account.Token.Id, Enabled: true, Limit: 100})
	require.NoError(t, err)
	require.NoError(t, ReserveTokenBudget(context.Background(), db, TokenBudgetReservation{
		RequestID: requestID, UserID: account.User.Id, TokenID: account.Token.Id, ChannelID: budgetChannelID, ModelName: "upstream-model",
		PayloadSHA256: strings.Repeat("a", 64), BoundSource: TokenBudgetBoundOpenAIResponses, InputTokens: 10, MaxOutputTokens: 20,
		PricingEvidence: `{"strict_token_budget":true}`,
	}))
	_, err = MutateTokenBudgetRequest(context.Background(), db, TokenBudgetMutation{TokenID: account.Token.Id, RequestID: requestID, Action: "send"})
	require.NoError(t, err)
	return strictUsageReviewAttributionFixture{db: db, root: root, account: account, receipt: receipt}
}

func assertUsageReviewAttributionHasNoEffects(t *testing.T, fixture strictUsageReviewAttributionFixture) {
	t.Helper()
	var decisions, logs int64
	require.NoError(t, fixture.db.Model(&UsageReviewDecision{}).Where("request_id = ?", fixture.receipt.RequestID).Count(&decisions).Error)
	require.NoError(t, fixture.db.Model(&Log{}).Where("request_id = ?", fixture.receipt.RequestID).Count(&logs).Error)
	assert.Zero(t, decisions)
	assert.Zero(t, logs)
	var user User
	require.NoError(t, fixture.db.First(&user, fixture.account.User.Id).Error)
	assert.Zero(t, user.UsedQuota)
	assert.Zero(t, user.RequestCount)
}

func TestUsageReviewStrictChannelAttributionValidation(t *testing.T) {
	t.Run("same positive channel", func(t *testing.T) {
		fixture := newStrictUsageReviewAttributionFixture(t, "strict-channel-same", 17, 17)
		view, err := GetUsageReview(context.Background(), fixture.db, fixture.root.Id, fixture.receipt.RequestID)
		require.NoError(t, err)
		assert.Equal(t, 17, view.ChannelID)
		assert.Equal(t, "origin-model", view.ModelName, "the strict outbound model must not replace the origin model")
		assertUsageReviewAttributionHasNoEffects(t, fixture)
	})

	t.Run("positive channel conflict", func(t *testing.T) {
		fixture := newStrictUsageReviewAttributionFixture(t, "strict-channel-conflict", 17, 19)
		view, err := GetUsageReview(context.Background(), fixture.db, fixture.root.Id, fixture.receipt.RequestID)
		require.ErrorIs(t, err, ErrTokenBudgetConflict)
		assert.Nil(t, view)
		assertUsageReviewAttributionHasNoEffects(t, fixture)
	})

	t.Run("invalid durable channel", func(t *testing.T) {
		fixture := newStrictUsageReviewAttributionFixture(t, "strict-channel-invalid", 0, 23)
		require.NoError(t, fixture.db.Model(&TokenBudgetReservation{}).Where("request_id = ?", fixture.receipt.RequestID).Update("channel_id", 0).Error)
		view, err := GetUsageReview(context.Background(), fixture.db, fixture.root.Id, fixture.receipt.RequestID)
		require.ErrorIs(t, err, ErrTokenBudgetConflict)
		assert.Nil(t, view)
		assertUsageReviewAttributionHasNoEffects(t, fixture)
	})
}

func TestUsageReviewChannelAttributionPreservesNonStrictAndHistoricalDecisions(t *testing.T) {
	t.Run("ordinary non-strict review", func(t *testing.T) {
		db := openAccountQuotaTestDB(t)
		require.NoError(t, db.AutoMigrate(&UsageReviewDecision{}))
		root := User{Username: "ordinary-review-root", AffCode: "ordinary-review-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
		require.NoError(t, db.Create(&root).Error)
		account := newAccountQuotaFixture(t, db, "ordinary-review", 1000, 1000, false, 0, 0, false)
		receipt, err := ReserveAccountQuota(context.Background(), db, AccountQuotaReserveInput{RequestID: "ordinary-review", UserID: account.User.Id, TokenID: account.Token.Id, RequestedQuota: 100, BillingPreference: "wallet_only", BillingContext: AccountBillingContext{Version: 1, OriginModelName: "origin-model", BillingPreference: "wallet_only"}})
		require.NoError(t, err)
		_, err = HoldAccountQuotaUnknownUsage(context.Background(), db, receipt.RequestID, receipt.ID, "missing")
		require.NoError(t, err)
		view, err := GetUsageReview(context.Background(), db, root.Id, receipt.RequestID)
		require.NoError(t, err)
		assert.Zero(t, view.ChannelID)
		assert.Nil(t, view.TokenBudget)
	})

	t.Run("historical decision stays immutable", func(t *testing.T) {
		fixture := newStrictUsageReviewAttributionFixture(t, "strict-channel-history", 0, 29)
		historical := UsageReviewDecision{RequestID: fixture.receipt.RequestID, ActorID: fixture.root.Id, UserID: fixture.account.User.Id, TokenID: fixture.account.Token.Id, ChannelID: 0, ModelName: "origin-model", ActualQuota: 100, EvidenceReference: "historical evidence", EvidenceDigest: strings.Repeat("e", 64), CreatedAt: 1}
		require.NoError(t, fixture.db.Create(&historical).Error)
		view, err := GetUsageReview(context.Background(), fixture.db, fixture.root.Id, fixture.receipt.RequestID)
		require.NoError(t, err)
		assert.Equal(t, 29, view.ChannelID)
		require.NotNil(t, view.Decision)
		assert.Zero(t, view.Decision.ChannelID)
		var stored UsageReviewDecision
		require.NoError(t, fixture.db.First(&stored, historical.ID).Error)
		assert.Zero(t, stored.ChannelID)
		assert.False(t, stored.StatisticsApplied)
		assert.False(t, stored.LogProjected)
		var logs int64
		require.NoError(t, fixture.db.Model(&Log{}).Where("request_id = ?", fixture.receipt.RequestID).Count(&logs).Error)
		assert.Zero(t, logs)
	})
}
