package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestR1UsageReviewConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/r1-review.db")
			if engine.env != "" {
				if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
					t.Skip("requires explicitly configured disposable loopback B2 database")
				}
				dsn := strings.TrimSpace(os.Getenv(engine.env))
				require.NotEmpty(t, dsn)
				var err error
				dialector, err = b2SubmissionDatabaseDialector(engine.name, dsn)
				require.NoError(t, err)
			}
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			pool, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, pool.Ping())
			t.Cleanup(func() { require.NoError(t, pool.Close()) })
			entities := []any{&User{}, &Token{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionPreConsumeRecord{}, &AccountQuotaMutationReceipt{}, &AccountQuotaReservationHead{}, &AccountQuotaTerminalRecoveryObligation{}, &AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}, &AccountQuotaRefundFact{}, &QuotaWriterEpoch{}, &QuotaProjectionObligation{}, &QuotaWorkCursor{}, &LegacyUsageReservation{}, &UsageReviewDecision{}, &Log{}, &BillingLogProjectionIdentity{}, &TokenBudget{}, &TokenBudgetReservation{}, &TokenBudgetPolicyChange{}, &ChannelQuotaSnapshot{}, &UserUsagePolicyChange{}, &Option{}}
			require.NoError(t, db.AutoMigrate(entities...))
			// Repeat the schemas changed by this iteration. Unrelated legacy
			// tables have application-specific migrations outside this fixture.
			require.NoError(t, db.AutoMigrate(&AccountQuotaTerminalRecoveryObligation{}, &LegacyUsageReservation{}, &UsageReviewDecision{}))
			require.NoError(t, ValidateUsageReviewSchema(db))
			require.NoError(t, ValidateTokenBudgetSchema(db))
			oldDB, oldLogDB := DB, LOG_DB
			oldRedis := common.RedisEnabled
			oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.RedisEnabled = false
			common.SetDatabaseTypes(common.DatabaseType(engine.name), common.DatabaseType(engine.name))
			t.Cleanup(func() {
				DB, LOG_DB = oldDB, oldLogDB
				common.RedisEnabled = oldRedis
				common.SetDatabaseTypes(oldMain, oldLog)
				InitColumnNamesForTest()
			})
			require.NoError(t, EnsureQuotaWriterEpochStateWithDB(db))
			epoch, err := GetQuotaWriterEpochState(db)
			require.NoError(t, err)
			setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, epoch.Epoch+1)
			// Timestamp namespaces fixture rows only; no timing assertions or sleeps.
			namespace := fmt.Sprintf("r1%x", time.Now().UnixNano())
			verifyTextDispatchTransitions(t, db, namespace+"td")
			user := User{Username: namespace, AffCode: namespace, Password: "fixture-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 1000, AuthVersion: 1}
			require.NoError(t, db.Create(&user).Error)
			token := Token{UserId: user.Id, Key: namespace, Status: common.TokenStatusEnabled, RemainQuota: 1000, ExpiredTime: -1}
			require.NoError(t, db.Create(&token).Error)
			root := User{Username: namespace + "r", AffCode: namespace + "r", Password: "fixture-password", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AuthVersion: 1}
			require.NoError(t, db.Create(&root).Error)
			ctx := context.Background()
			reserve, err := ReserveAccountQuota(ctx, db, AccountQuotaReserveInput{RequestID: namespace, UserID: user.Id, TokenID: token.Id, RequestedQuota: 100, BillingPreference: "wallet_only", BillingContext: AccountBillingContext{Version: 1, OriginModelName: "r1-fixture", BillingPreference: "wallet_only"}})
			require.NoError(t, err)
			_, err = HoldAccountQuotaUnknownUsage(ctx, db, namespace, reserve.ID, "missing")
			require.NoError(t, err)
			_, err = GetUsageReview(ctx, db, root.Id, strings.ToUpper(namespace))
			require.ErrorIs(t, err, gorm.ErrRecordNotFound, "request identity must not depend on database collation")
			_, err = RefundAccountQuota(ctx, db, AccountQuotaTerminalInput{RequestID: namespace, ReserveReceiptID: reserve.ID, AuditKey: "ordinary-refund"})
			require.Error(t, err)
			_, err = ReconcileUsageReview(ctx, db, user.Id, namespace, 120, "synthetic proof")
			require.ErrorIs(t, err, ErrAccountQuotaMutationIneligible)
			view, err := ReconcileUsageReview(ctx, db, root.Id, namespace, 120, "synthetic proof")
			require.NoError(t, err)
			require.NotNil(t, view.Decision)
			require.NoError(t, ProjectUsageReviewDecision(ctx, db, db, view.Decision.ID))
			_, err = ReconcileUsageReview(ctx, db, root.Id, namespace, 120, "synthetic proof")
			require.NoError(t, err)
			require.NoError(t, ProjectUsageReviewDecision(ctx, db, db, view.Decision.ID))
			require.NoError(t, db.First(&user, user.Id).Error)
			require.NoError(t, db.First(&token, token.Id).Error)
			assert.Equal(t, 880, user.Quota)
			assert.Equal(t, 880, token.RemainQuota)
			assert.Equal(t, 120, user.UsedQuota)
			assert.Equal(t, 1, user.RequestCount)
			var terminals int64
			require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).Where("request_id = ? AND phase = ?", namespace, AccountQuotaPhaseSettle).Count(&terminals).Error)
			assert.EqualValues(t, 1, terminals)
			tokenBudgetCoupledReviewDatabaseContract(t, db, root.Id, user.Id, token.Id, namespace+"b")
			feeBudgetCoupledReviewDatabaseContract(t, db, root.Id, user.Id, token.Id, namespace+"f")
			selfUseCoupledReviewDatabaseContract(t, db, root.Id, namespace)
			textDispatchRecoveryDatabaseContract(t, db, root.Id, namespace)
			verifyLegacyUsageExtension(t, db, namespace+"e")
		})
	}
}

// Reuses the real configured database fixture to verify the two ledgers and
// projection recover from persisted evidence, with no live billing session.
func tokenBudgetCoupledReviewDatabaseContract(t *testing.T, db *gorm.DB, rootID, userID, tokenID int, requestID string) {
	t.Helper()
	ctx := context.Background()
	_, err := ConfigureTokenBudget(ctx, db, rootID, TokenBudgetPolicyInput{ID: fmt.Sprintf("%064x", time.Now().UnixNano()), TokenID: tokenID, Enabled: true, Limit: 100})
	require.NoError(t, err)
	_, err = ReserveAccountQuota(ctx, db, AccountQuotaReserveInput{RequestID: requestID, UserID: userID, TokenID: tokenID, RequestedQuota: 100, BillingPreference: "wallet_only", BillingContext: AccountBillingContext{Version: 1, OriginModelName: "r1-fixture", BillingPreference: "wallet_only"}})
	require.NoError(t, err)
	require.NoError(t, ReserveTokenBudget(ctx, db, TokenBudgetReservation{RequestID: requestID, UserID: userID, TokenID: tokenID, ChannelID: 7, ModelName: "r1-fixture", PayloadSHA256: strings.Repeat("a", 64), BoundSource: TokenBudgetBoundOpenAIResponses, InputTokens: 10, MaxOutputTokens: 20, PricingEvidence: `{"strict_token_budget":true}`}))
	_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: tokenID, RequestID: requestID, Action: "send"})
	require.NoError(t, err)
	view, err := PrepareTokenBudgetUsageReview(ctx, db, rootID, tokenID, requestID)
	require.NoError(t, err)
	require.NotNil(t, view.TokenBudget)
	_, err = ReconcileUsageReview(ctx, db, rootID, requestID, 20, "synthetic cross-ledger proof")
	require.ErrorIs(t, err, ErrTokenBudgetInvalid)
	counts := UsageReviewTokenCounts{Input: 10, Output: 5}
	// A lost decision write keeps both reservations. Once manual review starts,
	// late automatic usage cannot race ahead and settle different token counts.
	injected := errors.New("synthetic decision persistence interruption")
	callback := "r1:token-budget-review-failure"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "UsageReviewDecision" {
			tx.AddError(injected)
		}
	}))
	_, err = ReconcileUsageReview(ctx, db, rootID, requestID, 20, "synthetic cross-ledger proof", counts)
	require.Error(t, err)
	require.NoError(t, db.Callback().Create().Remove(callback))
	_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: tokenID, RequestID: requestID, Action: "settle", Input: 10, Output: 6})
	require.ErrorIs(t, err, ErrTokenBudgetPending)
	for range 2 {
		view, err = ReconcileUsageReview(ctx, db, rootID, requestID, 20, "synthetic cross-ledger proof", counts)
		require.NoError(t, err)
		require.NoError(t, ProjectUsageReviewDecision(ctx, db, db, view.Decision.ID))
	}
	budget, err := ReadTokenBudget(ctx, db, rootID, tokenID)
	require.NoError(t, err)
	assert.EqualValues(t, 15, budget.Policy.Used)
	assert.Zero(t, budget.Policy.Reserved)
	assert.Nil(t, budget.Pending)
	var logs []Log
	require.NoError(t, db.Where("request_id = ? AND type = ?", requestID, LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, 10, logs[0].PromptTokens)
	assert.Equal(t, 5, logs[0].CompletionTokens)
	assert.Contains(t, logs[0].Other, `"token_counts_confirmed":true`)
	var user User
	require.NoError(t, db.First(&user, userID).Error)
	assert.Equal(t, 860, user.Quota)
	assert.Equal(t, 140, user.UsedQuota)
	assert.Equal(t, 2, user.RequestCount)
}

func feeBudgetCoupledReviewDatabaseContract(t *testing.T, db *gorm.DB, rootID, userID, tokenID int, requestID string) {
	t.Helper()
	ctx := context.Background()
	policy, err := LookupTokenBudget(ctx, db, tokenID)
	require.NoError(t, err)
	_, err = ConfigureTokenBudget(ctx, db, rootID, TokenBudgetPolicyInput{ID: fmt.Sprintf("%064x", time.Now().UnixNano()), TokenID: tokenID, ExpectedRevision: policy.Revision, Enabled: true, Limit: 100, Fee: &FeeBudgetPolicyInput{Enabled: true, LimitUSD: "0.001"}})
	require.NoError(t, err)
	_, err = ReserveAccountQuota(ctx, db, AccountQuotaReserveInput{RequestID: requestID, UserID: userID, TokenID: tokenID, RequestedQuota: 100, BillingPreference: "wallet_only", BillingContext: AccountBillingContext{Version: 1, OriginModelName: "r1-fixture", BillingPreference: "wallet_only"}})
	require.NoError(t, err)
	require.NoError(t, ReserveTokenBudget(ctx, db, TokenBudgetReservation{RequestID: requestID, UserID: userID, TokenID: tokenID, ChannelID: 7, ModelName: "r1-fixture", PayloadSHA256: strings.Repeat("a", 64), BoundSource: TokenBudgetBoundOpenAIResponses, InputTokens: 10, MaxOutputTokens: 20, PricingEvidence: `{"strict_token_budget":true}`, RequestServiceTier: "default", FeeEnabled: true, FeeReservedUSD: "0.0003", FeePriceEvidence: `{"scope":"synthetic-three-db"}`}))
	_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: tokenID, RequestID: requestID, Action: "send"})
	require.NoError(t, err)
	_, err = PrepareTokenBudgetUsageReview(ctx, db, rootID, tokenID, requestID)
	require.ErrorIs(t, err, ErrFeeBudgetInvalid)
	before, err := GetUsageReview(ctx, db, rootID, requestID)
	require.NoError(t, err)
	assert.Equal(t, AccountQuotaTerminalRecoveryOpen, before.State, "missing fee evidence cannot mutate the old quota lifecycle")
	amount := "0.00012"
	_, err = PrepareTokenBudgetUsageReview(ctx, db, rootID, tokenID, requestID, &amount)
	require.NoError(t, err)
	for range 2 {
		view, err := ReconcileUsageReview(ctx, db, rootID, requestID, 20, "verified USD fixture evidence", UsageReviewTokenCounts{Input: 10, Output: 5, FeeUSD: &amount})
		require.NoError(t, err)
		require.NoError(t, ProjectUsageReviewDecision(ctx, db, db, view.Decision.ID))
	}
	policy, err = LookupTokenBudget(ctx, db, tokenID)
	require.NoError(t, err)
	assert.Equal(t, amount, policy.FeeUsedUSD)
	assert.Equal(t, "0", policy.FeeReservedUSD)
	assert.EqualValues(t, 30, policy.Used)
	var logs []Log
	require.NoError(t, db.Where("request_id = ? AND type = ?", requestID, LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0].Other, `"confirmed_api_usage_cost_usd":"0.00012"`)
	var user User
	require.NoError(t, db.First(&user, userID).Error)
	assert.Equal(t, 840, user.Quota)
	assert.Equal(t, 160, user.UsedQuota)
	assert.Equal(t, 3, user.RequestCount)
}
