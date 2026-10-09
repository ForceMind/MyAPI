package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"strings"
	"testing"
)

// The configured usage-review harness also runs this persisted-row contract
// against MySQL/PostgreSQL. No live billing session is needed for the read.
func usageReviewSettlementDatabaseContract(t *testing.T, db *gorm.DB, rootID int, namespace string) {
	t.Helper()
	ctx := context.Background()
	for index, mode := range []QuotaWriterMode{QuotaWriterModeLegacy, QuotaWriterModeAuthoritative} {
		t.Run("settlement-read-"+string(mode), func(t *testing.T) {
			epoch, err := GetQuotaWriterEpochState(db)
			require.NoError(t, err)
			setQuotaWriterStateForTest(t, db, mode, epoch.Epoch+1)
			fixture := newAccountQuotaFixture(t, db, "read", 1000, 1000, false, 0, 0, false)
			id := fmt.Sprintf("%ssr%d", namespace, index)
			input := AccountQuotaSettlementFactInput{EventKey: "billing-settlement:" + id + ":v1", RequestID: id, Kind: AccountQuotaSettlementKindLegacyWallet, UserID: fixture.User.Id, TokenID: fixture.Token.Id, Delta: 60, ApplyToken: true}
			state := LegacyUsagePrepared
			if mode == QuotaWriterModeLegacy {
				_, err = PrepareLegacyUsageReservation(ctx, db, LegacyUsageReservation{RequestID: id, UserID: fixture.User.Id, TokenID: fixture.Token.Id, FundingSource: "wallet"})
				require.NoError(t, err)
			} else {
				reserve, err := ReserveAccountQuota(ctx, db, accountReserveInput(fixture, id, "wallet_only", 0))
				require.NoError(t, err)
				input = AccountQuotaSettlementFactInput{EventKey: "billing-settlement:" + id + ":v2", RequestID: id, Kind: AccountQuotaSettlementKindAuthoritative, UserID: fixture.User.Id, TokenID: fixture.Token.Id, ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch, ActualQuota: 60}
				state = AccountQuotaTerminalRecoveryOpen
			}
			require.NoError(t, SetTextDispatchEvidence(ctx, db, id, fixture.User.Id, fixture.Token.Id, input.ReserveReceiptID, `{"version":1}`, true))
			view, err := GetUsageReview(ctx, db, rootID, id)
			require.NoError(t, err)
			require.True(t, view.CanRecoverTextDispatch)
			intent, err := EnsureAccountQuotaSettlementIntent(ctx, db, input)
			require.NoError(t, err)
			view, err = GetUsageReview(ctx, db, fixture.User.Id, id)
			require.NoError(t, err)
			assert.False(t, view.CanRecoverTextDispatch, "a durable automatic intent owns recovery")
			assert.Equal(t, state, view.State)
			assert.Nil(t, view.ActualQuota, "intended amount is not an applied charge")
			assertReviewSettlementJSON(t, view, "pending", "automatic_settlement_pending", false)
			_, err = RecoverTextDispatchUsage(ctx, db, rootID, id, 20, "must not replace automatic amount")
			require.ErrorIs(t, err, ErrAccountQuotaSettlementPending)
			_, fact, err := RecoverAccountQuotaSettlementIntent(ctx, db, intent, "read-projection")
			require.NoError(t, err)
			require.Equal(t, AccountQuotaSettlementApplied, fact.State)
			beforeUser, beforeToken, _ := loadAccountBalances(t, db, fixture)
			for range 2 {
				view, err = GetUsageReview(ctx, db, fixture.User.Id, id)
				require.NoError(t, err)
				require.NotNil(t, view.ActualQuota)
				assert.EqualValues(t, 60, *view.ActualQuota)
				assert.False(t, view.CanRecoverTextDispatch)
				assert.False(t, view.TextDispatchPending)
				status := "applied"
				if mode == QuotaWriterModeLegacy {
					status = "applied_journal_pending"
					assert.Equal(t, LegacyUsagePrepared, view.State, "keep the journal's durable state")
				}
				assertReviewSettlementJSON(t, view, status, "automatic_settlement_applied", false)
			}
			afterUser, afterToken, _ := loadAccountBalances(t, db, fixture)
			assert.Equal(t, beforeUser, afterUser)
			assert.Equal(t, beforeToken, afterToken)
			assert.Equal(t, 940, afterUser.Quota)
			assert.Equal(t, 940, afterToken.RemainQuota)
			if mode == QuotaWriterModeLegacy {
				row, err := FindLegacyUsageReservation(ctx, db, id)
				require.NoError(t, err)
				assert.Equal(t, LegacyUsagePrepared, row.State)
				assert.Nil(t, row.ActualQuota)
				assert.Contains(t, row.ReviewMetadata, `"text_dispatch_pending":true`)
			}
			outsider := User{Username: id + "o", AffCode: id + "o", Status: common.UserStatusEnabled, Role: common.RoleAdminUser}
			require.NoError(t, db.Create(&outsider).Error)
			_, err = GetUsageReview(ctx, db, outsider.Id, id)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		})
	}
}

func assertReviewSettlementJSON(t *testing.T, view *UsageReviewDetail, status, reason string, canReconcile bool) {
	t.Helper()
	encoded, err := common.Marshal(view)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, common.Unmarshal(encoded, &fields))
	assert.Equal(t, status, fields["settlement_status"])
	assert.Equal(t, reason, fields["recovery_block_reason"])
	assert.Equal(t, canReconcile, fields["can_reconcile_usage"])
}

func TestUsageReviewSettlementReadProjection(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	require.NoError(t, db.AutoMigrate(&AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}, &UsageReviewDecision{}))
	root := User{Username: "review-root", AffCode: "review-root", Status: common.UserStatusEnabled, Role: common.RoleRootUser}
	require.NoError(t, db.Create(&root).Error)
	usageReviewSettlementDatabaseContract(t, db, root.Id, "read-projection")
}

func TestUsageReviewSettlementLookupFailsClosed(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	require.NoError(t, db.AutoMigrate(&AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}, &UsageReviewDecision{}))
	fixture := newAccountQuotaFixture(t, db, "lookup", 1000, 1000, false, 0, 0, false)
	_, err := PrepareLegacyUsageReservation(context.Background(), db, LegacyUsageReservation{RequestID: "lookup", UserID: fixture.User.Id, TokenID: fixture.Token.Id, FundingSource: "wallet"})
	require.NoError(t, err)
	require.NoError(t, SetTextDispatchEvidence(context.Background(), db, "lookup", fixture.User.Id, fixture.Token.Id, 0, `{"version":1}`, true))
	for _, table := range []string{"account_quota_settlement_intents", "account_quota_settlement_facts"} {
		t.Run(table, func(t *testing.T) {
			callback := "read-projection-failure"
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					tx.AddError(errors.New("private database connection failure"))
				}
			}))
			view, err := GetUsageReview(context.Background(), db, fixture.User.Id, "lookup")
			require.NoError(t, db.Callback().Query().Remove(callback))
			require.Error(t, err)
			assert.Nil(t, view)
			assert.False(t, strings.Contains(err.Error(), "private database"))
		})
	}
}

func TestUsageReviewSettlementStatusIsAllowlisted(t *testing.T) {
	for _, source := range []string{"intent", "fact"} {
		for _, state := range []string{AccountQuotaSettlementPending, AccountQuotaSettlementMaterialized, AccountQuotaSettlementClaimed, AccountQuotaSettlementRetryable, AccountQuotaSettlementManual, "unrecognized"} {
			t.Run(source+"/"+state, func(t *testing.T) {
				db := openAccountQuotaTestDB(t)
				fixture := newAccountQuotaFixture(t, db, "status", 1000, 1000, false, 0, 0, false)
				id := "status-review"
				_, err := PrepareLegacyUsageReservation(context.Background(), db, LegacyUsageReservation{RequestID: id, UserID: fixture.User.Id, TokenID: fixture.Token.Id, FundingSource: "wallet"})
				require.NoError(t, err)
				require.NoError(t, SetTextDispatchEvidence(context.Background(), db, id, fixture.User.Id, fixture.Token.Id, 0, `{"version":1}`, true))
				input := AccountQuotaSettlementFactInput{EventKey: "billing-settlement:" + id + ":v1", RequestID: id, Kind: AccountQuotaSettlementKindLegacyWallet, UserID: fixture.User.Id, TokenID: fixture.Token.Id, Delta: 60, ApplyToken: true}
				table := "account_quota_settlement_intents"
				if source == "intent" {
					_, err = EnsureAccountQuotaSettlementIntent(context.Background(), db, input)
				} else {
					_, err = EnsureAccountQuotaSettlementFact(context.Background(), db, input)
					table = "account_quota_settlement_facts"
				}
				require.NoError(t, err)
				require.NoError(t, db.Table(table).Where("request_id = ?", id).Updates(map[string]any{"state": state, "last_error": "private upstream response and credential sentinel"}).Error)
				view, err := GetUsageReview(context.Background(), db, fixture.User.Id, id)
				require.NoError(t, err)
				expected, reason := "pending", "automatic_settlement_pending"
				if state == AccountQuotaSettlementManual || state == "unrecognized" {
					expected, reason = "manual", "automatic_settlement_manual"
				}
				assertReviewSettlementJSON(t, view, expected, reason, false)
				require.False(t, view.CanRecoverTextDispatch)
				require.Nil(t, view.ActualQuota)
				encoded, err := common.Marshal(view)
				require.NoError(t, err)
				assert.NotContains(t, string(encoded), "private upstream")
			})
		}
	}
}

func TestUsageReviewSettlementRejectsInvalidAppliedEvidence(t *testing.T) {
	cases := []struct {
		name    string
		changes map[string]any
	}{
		{"wrong-owner", map[string]any{"user_id": 999}},
		{"wrong-token", map[string]any{"token_id": 999}},
		{"wrong-kind", map[string]any{"kind": AccountQuotaSettlementKindLegacySubscription}},
		{"fingerprint", map[string]any{"request_fingerprint": strings.Repeat("0", 64)}},
		{"unapplied-token", map[string]any{"token_applied": false}},
		{"unapplied-funding", map[string]any{"funding_applied": false}},
		{"negative-final", map[string]any{"delta": -60}},
		{"overflow-delta", map[string]any{"delta": int64(common.MaxQuota) + 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			fixture := newAccountQuotaFixture(t, db, "invalid", 1000, 1000, false, 0, 0, false)
			id := "invalid-evidence"
			_, err := PrepareLegacyUsageReservation(context.Background(), db, LegacyUsageReservation{RequestID: id, UserID: fixture.User.Id, TokenID: fixture.Token.Id, FundingSource: "wallet"})
			require.NoError(t, err)
			require.NoError(t, SetTextDispatchEvidence(context.Background(), db, id, fixture.User.Id, fixture.Token.Id, 0, `{"version":1}`, true))
			fact, err := EnsureAccountQuotaSettlementFact(context.Background(), db, AccountQuotaSettlementFactInput{EventKey: "billing-settlement:" + id + ":v1", RequestID: id, Kind: AccountQuotaSettlementKindLegacyWallet, UserID: fixture.User.Id, TokenID: fixture.Token.Id, Delta: 60, ApplyToken: true})
			require.NoError(t, err)
			fact, err = RecoverAccountQuotaSettlementFact(context.Background(), db, fact, "invalid-evidence")
			require.NoError(t, err)
			require.Equal(t, AccountQuotaSettlementApplied, fact.State)
			require.NoError(t, db.Table("account_quota_settlement_facts").Where("id = ?", fact.ID).Updates(tc.changes).Error)
			view, err := GetUsageReview(context.Background(), db, fixture.User.Id, id)
			require.NoError(t, err)
			assertReviewSettlementJSON(t, view, "manual", "automatic_settlement_manual", false)
			require.Nil(t, view.ActualQuota)
			require.False(t, view.CanRecoverTextDispatch)
		})
	}
}

func usageReviewAppliedSettlementTokenBudgetContract(t *testing.T, db *gorm.DB, rootID int, namespace string) {
	t.Helper()
	ctx := context.Background()
	epoch, err := GetQuotaWriterEpochState(db)
	require.NoError(t, err)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeLegacy, epoch.Epoch+1)
	// The shared fixture adds account- and a nanosecond suffix; keep aff_code within varchar(32).
	f := newAccountQuotaFixture(t, db, "sr", 1000, 1000, false, 0, 0, false)
	require.LessOrEqual(t, len(f.User.AffCode), 32)
	id := namespace + "strict-read"
	_, err = PrepareLegacyUsageReservation(ctx, db, LegacyUsageReservation{RequestID: id, UserID: f.User.Id, TokenID: f.Token.Id, FundingSource: "wallet"})
	require.NoError(t, err)
	intent, err := EnsureAccountQuotaSettlementIntent(ctx, db, AccountQuotaSettlementFactInput{EventKey: "billing-settlement:" + id + ":v1", RequestID: id, Kind: AccountQuotaSettlementKindLegacyWallet, UserID: f.User.Id, TokenID: f.Token.Id, Delta: 60, ApplyToken: true})
	require.NoError(t, err)
	_, _, err = RecoverAccountQuotaSettlementIntent(ctx, db, intent, "strict-read")
	require.NoError(t, err)
	_, err = ConfigureTokenBudget(ctx, db, rootID, TokenBudgetPolicyInput{ID: fmt.Sprintf("%064x", f.Token.Id), TokenID: f.Token.Id, Enabled: true, Limit: 100})
	require.NoError(t, err)
	require.NoError(t, ReserveTokenBudget(ctx, db, TokenBudgetReservation{RequestID: id, UserID: f.User.Id, TokenID: f.Token.Id, ChannelID: 7, ModelName: "fixture", PayloadSHA256: strings.Repeat("a", 64), BoundSource: TokenBudgetBoundOpenAIResponses, InputTokens: 10, MaxOutputTokens: 20, PricingEvidence: `{"strict_token_budget":true}`}))
	_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: f.Token.Id, RequestID: id, Action: "send"})
	require.NoError(t, err)
	beforePrepare, err := ReadTokenBudget(ctx, db, rootID, f.Token.Id)
	require.NoError(t, err)
	require.NotNil(t, beforePrepare.Review)
	require.True(t, beforePrepare.Review.CanReconcileUsage, "applied accounting must not hide unresolved strict token review")
	require.NotNil(t, beforePrepare.Review.TokenBudget)
	unchanged, err := FindLegacyUsageReservation(ctx, db, id)
	require.NoError(t, err)
	require.Equal(t, LegacyUsagePrepared, unchanged.State, "the read must not prepare a hold")
	view, err := PrepareTokenBudgetUsageReview(ctx, db, rootID, f.Token.Id, id)
	require.NoError(t, err)
	assert.Equal(t, LegacyUsageUnknown, view.State)
	assertReviewSettlementJSON(t, view, "applied_journal_pending", "", true)
	require.NotNil(t, view.ActualQuota)
	require.EqualValues(t, 60, *view.ActualQuota)
	counts := UsageReviewTokenCounts{Input: 10, Output: 5}
	_, err = ReconcileUsageReview(ctx, db, rootID, id, 0, "must not erase applied quota", counts)
	require.ErrorIs(t, err, ErrAccountQuotaMutationConflict)
	var decisions int64
	require.NoError(t, db.Model(&UsageReviewDecision{}).Where("request_id = ?", id).Count(&decisions).Error)
	require.Zero(t, decisions, "wrong amount cannot create a decision")
	var pending TokenBudgetReservation
	require.NoError(t, db.First(&pending, "request_id = ?", id).Error)
	require.Equal(t, TokenBudgetSent, pending.State, "wrong amount cannot freeze or settle token usage")
	for range 2 {
		view, err = ReconcileUsageReview(ctx, db, rootID, id, 60, "confirmed counts for applied quota", counts)
		require.NoError(t, err)
		require.NotNil(t, view.Decision)
		assertReviewSettlementJSON(t, view, "none", "", true)
	}
	user, token, _ := loadAccountBalances(t, db, f)
	assert.Equal(t, 940, user.Quota)
	assert.Equal(t, 940, token.RemainQuota)
	budget, err := LookupTokenBudget(ctx, db, f.Token.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 15, budget.Used)
	assert.Zero(t, budget.Reserved)
}

func TestUsageReviewSettlementUsesAuthoritativeTerminalAfterLostResponse(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	f := newAccountQuotaFixture(t, db, "terminal-read", 1000, 1000, false, 0, 0, false)
	ctx := context.Background()
	reserve, err := ReserveAccountQuota(ctx, db, accountReserveInput(f, "terminal-read", "wallet_only", 0))
	require.NoError(t, err)
	require.NoError(t, SetTextDispatchEvidence(ctx, db, reserve.RequestID, f.User.Id, f.Token.Id, reserve.ID, `{"version":1}`, true))
	intent, err := EnsureAccountQuotaSettlementIntent(ctx, db, AccountQuotaSettlementFactInput{EventKey: "billing-settlement:terminal-read:v2", RequestID: reserve.RequestID, Kind: AccountQuotaSettlementKindAuthoritative, UserID: f.User.Id, TokenID: f.Token.Id, ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch, ActualQuota: 60})
	require.NoError(t, err)
	accountQuotaSettlementAfterTerminalCommitHook = func(string) error { return errors.New("private lost response") }
	t.Cleanup(func() { accountQuotaSettlementAfterTerminalCommitHook = nil })
	_, fact, err := RecoverAccountQuotaSettlementIntent(ctx, db, intent, "terminal-read")
	require.Error(t, err)
	require.NotNil(t, fact)
	require.NotEqual(t, AccountQuotaSettlementApplied, fact.State)
	view, err := GetUsageReview(ctx, db, f.User.Id, reserve.RequestID)
	require.NoError(t, err)
	assertReviewSettlementJSON(t, view, "applied_journal_pending", "automatic_settlement_applied", false)
	require.NotNil(t, view.ActualQuota)
	assert.EqualValues(t, 60, *view.ActualQuota)
	assert.False(t, view.TextDispatchPending)
	assert.Equal(t, AccountQuotaTerminalRecoveryApplied, view.State)
	user, token, _ := loadAccountBalances(t, db, f)
	assert.Equal(t, 940, user.Quota)
	assert.Equal(t, 940, token.RemainQuota)
}

func TestUsageReviewSettlementRejectsMixedJournalSnapshots(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	f := newAccountQuotaFixture(t, db, "snapshot-read", 1000, 1000, false, 0, 0, false)
	ctx := context.Background()
	row, err := PrepareLegacyUsageReservation(ctx, db, LegacyUsageReservation{RequestID: "snapshot-read", UserID: f.User.Id, TokenID: f.Token.Id, FundingSource: "wallet"})
	require.NoError(t, err)
	fact, err := EnsureAccountQuotaSettlementFact(ctx, db, AccountQuotaSettlementFactInput{EventKey: "billing-settlement:snapshot-read:v1", RequestID: row.RequestID, Kind: AccountQuotaSettlementKindLegacyWallet, UserID: f.User.Id, TokenID: f.Token.Id, Delta: 60, ApplyToken: true})
	require.NoError(t, err)
	_, err = RecoverAccountQuotaSettlementFact(ctx, db, fact, "snapshot-read")
	require.NoError(t, err)
	changed := false
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("change-journal-after-fact-read", func(tx *gorm.DB) {
		if tx.Statement.Table == "account_quota_settlement_facts" && !changed {
			changed = true
			require.NoError(t, db.Model(&LegacyUsageReservation{}).Where("id = ?", row.ID).Updates(map[string]any{"reserved_quota": 100, "lock_version": gorm.Expr("lock_version + ?", 1)}).Error)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("change-journal-after-fact-read") })
	view, err := GetUsageReview(ctx, db, f.User.Id, row.RequestID)
	require.True(t, changed)
	require.ErrorIs(t, err, ErrAccountQuotaSettlementFactUnknown)
	assert.Nil(t, view, "do not combine a newer fact with an older reserved amount")
}

func TestUsageReviewSettlementDerivesLegacyFinalAmount(t *testing.T) {
	for _, actual := range []int64{0, 60, 120} {
		t.Run(fmt.Sprint(actual), func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			f := newAccountQuotaFixture(t, db, "final-amount", 1000, 1000, false, 0, 0, false)
			ctx := context.Background()
			require.NoError(t, db.Model(&User{}).Where("id = ?", f.User.Id).Update("quota", 900).Error)
			require.NoError(t, db.Model(&Token{}).Where("id = ?", f.Token.Id).Updates(map[string]any{"remain_quota": 900, "used_quota": 100}).Error)
			row, err := PrepareLegacyUsageReservation(ctx, db, LegacyUsageReservation{RequestID: "final-amount", UserID: f.User.Id, TokenID: f.Token.Id, FundingSource: "wallet", ReservedQuota: 100, TokenReservedQuota: 100})
			require.NoError(t, err)
			fact, err := EnsureAccountQuotaSettlementFact(ctx, db, AccountQuotaSettlementFactInput{EventKey: "billing-settlement:final-amount:v1", RequestID: row.RequestID, Kind: AccountQuotaSettlementKindLegacyWallet, UserID: f.User.Id, TokenID: f.Token.Id, Delta: actual - 100, ApplyToken: true})
			require.NoError(t, err)
			_, err = RecoverAccountQuotaSettlementFact(ctx, db, fact, "final-amount")
			require.NoError(t, err)
			view, err := GetUsageReview(ctx, db, f.User.Id, row.RequestID)
			require.NoError(t, err)
			require.NotNil(t, view.ActualQuota)
			assert.Equal(t, actual, *view.ActualQuota, "use reserved plus delta, including confirmed zero")
			assertReviewSettlementJSON(t, view, "applied_journal_pending", "automatic_settlement_applied", false)
			user, token, _ := loadAccountBalances(t, db, f)
			assert.EqualValues(t, 1000-actual, user.Quota)
			assert.EqualValues(t, 1000-actual, token.RemainQuota)
		})
	}
}

func TestUsageReviewSettlementRejectsForeignTerminalReceipt(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	ctx := context.Background()
	own := newAccountQuotaFixture(t, db, "own-terminal", 1000, 1000, false, 0, 0, false)
	foreign := newAccountQuotaFixture(t, db, "foreign-terminal", 1000, 1000, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(ctx, db, accountReserveInput(own, "own-terminal", "wallet_only", 0))
	require.NoError(t, err)
	otherReserve, err := ReserveAccountQuota(ctx, db, accountReserveInput(foreign, "foreign-terminal", "wallet_only", 0))
	require.NoError(t, err)
	terminal, err := SettleAccountQuota(ctx, db, AccountQuotaTerminalInput{RequestID: otherReserve.RequestID, ReserveReceiptID: otherReserve.ID, ActualQuota: 60})
	require.NoError(t, err)
	_, err = EnsureAccountQuotaSettlementIntent(ctx, db, AccountQuotaSettlementFactInput{EventKey: "billing-settlement:own-terminal:v2", RequestID: reserve.RequestID, Kind: AccountQuotaSettlementKindAuthoritative, UserID: own.User.Id, TokenID: own.Token.Id, ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch, ActualQuota: 60})
	require.NoError(t, err)
	require.NoError(t, db.Model(&AccountQuotaReservationHead{}).Where("request_id = ?", reserve.RequestID).Update("terminal_receipt_id", terminal.ID).Error)
	view, err := GetUsageReview(ctx, db, own.User.Id, reserve.RequestID)
	require.ErrorIs(t, err, ErrAccountQuotaMutationConflict)
	require.Nil(t, view, "same amount from someone else's receipt is not applied proof")
}

func TestUsageReviewSettlementPreparedTokenFailsClosed(t *testing.T) {
	for _, source := range []string{"intent", "fact"} {
		t.Run(source, func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			require.NoError(t, db.AutoMigrate(&TokenBudget{}, &TokenBudgetReservation{}, &TokenBudgetPolicyChange{}, &UsageReviewDecision{}))
			ctx := context.Background()
			f := newAccountQuotaFixture(t, db, "prepared-token", 1000, 1000, false, 0, 0, false)
			root := User{Username: "prepared-root", AffCode: "prepared-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
			require.NoError(t, db.Create(&root).Error)
			id := "prepared-token"
			_, err := PrepareLegacyUsageReservation(ctx, db, LegacyUsageReservation{RequestID: id, UserID: f.User.Id, TokenID: f.Token.Id, FundingSource: "wallet"})
			require.NoError(t, err)
			input := AccountQuotaSettlementFactInput{EventKey: "billing-settlement:" + id + ":v1", RequestID: id, Kind: AccountQuotaSettlementKindLegacyWallet, UserID: f.User.Id, TokenID: f.Token.Id, Delta: 60, ApplyToken: true}
			if source == "intent" {
				_, err = EnsureAccountQuotaSettlementIntent(ctx, db, input)
			} else {
				_, err = EnsureAccountQuotaSettlementFact(ctx, db, input)
			}
			require.NoError(t, err)
			_, err = ConfigureTokenBudget(ctx, db, root.Id, TokenBudgetPolicyInput{ID: strings.Repeat("d", 64), TokenID: f.Token.Id, Enabled: true, Limit: 100})
			require.NoError(t, err)
			require.NoError(t, ReserveTokenBudget(ctx, db, TokenBudgetReservation{RequestID: id, UserID: f.User.Id, TokenID: f.Token.Id, ChannelID: 7, ModelName: "fixture", PayloadSHA256: strings.Repeat("a", 64), BoundSource: TokenBudgetBoundOpenAIResponses, InputTokens: 10, MaxOutputTokens: 20, PricingEvidence: `{"strict_token_budget":true}`}))
			for _, actor := range []int{root.Id, f.User.Id} {
				view, err := ReadTokenBudget(ctx, db, actor, f.Token.Id)
				require.NoError(t, err)
				require.NotNil(t, view.Review)
				assertReviewSettlementJSON(t, view.Review, "manual", "automatic_settlement_manual", false)
				require.Equal(t, TokenBudgetPrepared, view.Pending.State)
				require.Nil(t, view.Review.ActualQuota)
			}
		})
	}
}

func TestUsageReviewSettlementRejectsRefundAsAppliedProof(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	f := newAccountQuotaFixture(t, db, "refund-proof", 1000, 1000, false, 0, 0, false)
	ctx := context.Background()
	reserve, err := ReserveAccountQuota(ctx, db, accountReserveInput(f, "refund-proof", "wallet_only", 0))
	require.NoError(t, err)
	fact, err := EnsureAccountQuotaSettlementFact(ctx, db, AccountQuotaSettlementFactInput{EventKey: "billing-settlement:refund-proof:v2", RequestID: reserve.RequestID, Kind: AccountQuotaSettlementKindAuthoritative, UserID: f.User.Id, TokenID: f.Token.Id, ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch, ActualQuota: 60})
	require.NoError(t, err)
	fact, err = RecoverAccountQuotaSettlementFact(ctx, db, fact, "refund-proof")
	require.NoError(t, err)
	require.NoError(t, db.Table("account_quota_mutation_receipts").Where("id = ?", fact.TerminalReceiptID).Update("phase", AccountQuotaPhaseRefund).Error)
	view, err := GetUsageReview(ctx, db, f.User.Id, reserve.RequestID)
	require.NoError(t, err)
	assertReviewSettlementJSON(t, view, "manual", "automatic_settlement_manual", false)
	require.Nil(t, view.ActualQuota, "a refund phase cannot prove an applied settlement")
}
