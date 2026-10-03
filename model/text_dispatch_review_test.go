package model

import (
	"context"
	"fmt"
	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"strings"
	"testing"
	"time"
)

func TestTextDispatchReviewBlocksLateAutomaticSettlementIntent(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	require.NoError(t, db.AutoMigrate(&AccountQuotaSettlementIntent{}))
	f := newAccountQuotaFixture(t, db, "dispatch-review", 1000, 1000, false, 0, 0, false)
	receipt, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(f, "dispatch-review", "wallet_only", 100))
	require.NoError(t, err)
	_, err = HoldAccountQuotaUnknownUsage(context.Background(), db, receipt.RequestID, receipt.ID, "missing", `{"version":1,"text_dispatch_pending":true}`)
	require.NoError(t, err)
	_, err = EnsureAccountQuotaSettlementIntent(context.Background(), db, AccountQuotaSettlementFactInput{EventKey: "billing-settlement:dispatch-review:v1", RequestID: receipt.RequestID, Kind: AccountQuotaSettlementKindAuthoritative, UserID: receipt.UserID, TokenID: receipt.TokenID, ReserveReceiptID: receipt.ID, WriterEpoch: receipt.WriterEpoch, ActualQuota: 80})
	require.ErrorIs(t, err, ErrAccountQuotaUsageUnresolved)
}

func textDispatchRecoveryDatabaseContract(t *testing.T, db *gorm.DB, rootID int, namespace string) {
	t.Helper()
	ctx := context.Background()
	pricing := `{"version":1,"model":"fixture","quota_unit":500000,"quota_unit_captured":true,"model_ratio":1,"completion_ratio":1,"group_ratio":1}`
	for index, mode := range []QuotaWriterMode{QuotaWriterModeAuthoritative, QuotaWriterModeLegacy} {
		t.Run("recover-dispatch-"+string(mode), func(t *testing.T) {
			epoch, err := GetQuotaWriterEpochState(db)
			require.NoError(t, err)
			setQuotaWriterStateForTest(t, db, mode, epoch.Epoch+1)
			name := fmt.Sprintf("rv%x", time.Now().UnixNano())
			user := User{Username: name, AffCode: name, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000, AuthVersion: 1}
			require.NoError(t, db.Create(&user).Error)
			token := Token{UserId: user.Id, Key: name, Status: common.TokenStatusEnabled, RemainQuota: 1000, ExpiredTime: -1}
			require.NoError(t, db.Create(&token).Error)
			requestID := fmt.Sprintf("%sr%d", namespace, index)
			var receiptID int64
			if mode == QuotaWriterModeAuthoritative {
				receipt, err := ReserveAccountQuota(ctx, db, AccountQuotaReserveInput{RequestID: requestID, UserID: user.Id, TokenID: token.Id, RequestedQuota: 100, BillingPreference: "wallet_only", BillingContext: AccountBillingContext{Version: 1, OriginModelName: "fixture", BillingPreference: "wallet_only"}})
				require.NoError(t, err)
				receiptID = receipt.ID
			} else {
				require.NoError(t, db.Model(&user).Updates(map[string]any{"quota": 900, "quota_version": 1}).Error)
				require.NoError(t, db.Model(&token).Updates(map[string]any{"remain_quota": 900, "used_quota": 100, "quota_version": 1}).Error)
				_, err := PrepareLegacyUsageReservation(ctx, db, LegacyUsageReservation{RequestID: requestID, UserID: user.Id, TokenID: token.Id, FundingSource: "wallet", ReservedQuota: 100, TokenReservedQuota: 100})
				require.NoError(t, err)
			}
			_, err = RecoverTextDispatchUsage(ctx, db, rootID, requestID, 20, "synthetic completed evidence")
			require.ErrorIs(t, err, ErrAccountQuotaUsageUnresolved, "old open/prepared is not send evidence")
			require.NoError(t, SetTextDispatchEvidence(ctx, db, requestID, user.Id, token.Id, receiptID, pricing, true))
			_, err = RecoverTextDispatchUsage(ctx, db, user.Id, requestID, 20, "synthetic completed evidence")
			require.ErrorIs(t, err, ErrAccountQuotaMutationIneligible)
			_, err = ListPendingUsageReviews(ctx, db, user.Id, string(mode), 0)
			require.ErrorIs(t, err, ErrAccountQuotaMutationIneligible)
			page, err := ListPendingUsageReviews(ctx, db, rootID, string(mode), 0)
			require.NoError(t, err)
			found := false
			for _, item := range page.Items {
				if item.RequestID == requestID {
					found = true
					assert.True(t, item.CanRecoverTextDispatch)
				}
			}
			assert.True(t, found)
			// Lose the decision write after the lifecycle was safely fenced. Changed
			// manual inputs must not overwrite the first audited recovery instruction.
			callback := "dispatch-review-decision-failure"
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "UsageReviewDecision" {
					tx.AddError(ErrAccountQuotaMutationCASLost)
				}
			}))
			_, err = RecoverTextDispatchUsage(ctx, db, rootID, requestID, 20, "synthetic completed evidence")
			require.Error(t, err)
			require.NoError(t, db.Callback().Create().Remove(callback))
			_, err = RecoverTextDispatchUsage(ctx, db, rootID, requestID, 21, "synthetic completed evidence")
			require.ErrorIs(t, err, ErrAccountQuotaMutationConflict)
			for range 2 {
				view, err := RecoverTextDispatchUsage(ctx, db, rootID, requestID, 20, "synthetic completed evidence")
				require.NoError(t, err)
				require.NotNil(t, view.Decision)
				require.NoError(t, ProjectUsageReviewDecision(ctx, db, db, view.Decision.ID))
			}
			require.NoError(t, db.First(&user, user.Id).Error)
			require.NoError(t, db.First(&token, token.Id).Error)
			assert.Equal(t, 980, user.Quota)
			assert.Equal(t, 980, token.RemainQuota)
			assert.Equal(t, 20, user.UsedQuota)
			assert.Equal(t, 1, user.RequestCount)
			if mode == QuotaWriterModeAuthoritative {
				receipt, err := FindAccountQuotaReserveReceipt(db, requestID)
				require.NoError(t, err)
				_, err = EnsureAccountQuotaSettlementIntent(ctx, db, AccountQuotaSettlementFactInput{EventKey: "late:" + requestID, RequestID: requestID, Kind: AccountQuotaSettlementKindAuthoritative, UserID: user.Id, TokenID: token.Id, ReserveReceiptID: receipt.ID, WriterEpoch: receipt.WriterEpoch, ActualQuota: 20})
				require.ErrorIs(t, err, ErrAccountQuotaUsageUnresolved, "even matching late automatic amounts cannot create new work after manual resolution")
			} else {
				_, err = EnsureAccountQuotaSettlementFact(ctx, db, AccountQuotaSettlementFactInput{EventKey: "late:" + requestID, RequestID: requestID, Kind: AccountQuotaSettlementKindLegacyWallet, UserID: user.Id, TokenID: token.Id, Delta: -80, ApplyToken: true})
				require.ErrorIs(t, err, ErrAccountQuotaUsageUnresolved, "a new late fact must not apply a reviewed delta again")
			}

		})
	}
}

func TestTextDispatchReviewRejectsKnownSettlementAndKeepsItsRecovery(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	require.NoError(t, db.AutoMigrate(&AccountQuotaSettlementIntent{}, &UsageReviewDecision{}))
	ctx := context.Background()
	f := newAccountQuotaFixture(t, db, "known-dispatch", 1000, 1000, false, 0, 0, false)
	root := User{Username: "known-root", AffCode: "known-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	receipt, err := ReserveAccountQuota(ctx, db, accountReserveInput(f, "known-dispatch", "wallet_only", 100))
	require.NoError(t, err)
	require.NoError(t, SetTextDispatchEvidence(ctx, db, receipt.RequestID, f.User.Id, f.Token.Id, receipt.ID, `{"version":1}`, true))
	_, err = EnsureAccountQuotaSettlementIntent(ctx, db, AccountQuotaSettlementFactInput{EventKey: "billing-settlement:known-dispatch:v1", RequestID: receipt.RequestID, Kind: AccountQuotaSettlementKindAuthoritative, UserID: receipt.UserID, TokenID: receipt.TokenID, ReserveReceiptID: receipt.ID, WriterEpoch: receipt.WriterEpoch, ActualQuota: 80})
	require.NoError(t, err)
	_, err = RecoverTextDispatchUsage(ctx, db, root.Id, receipt.RequestID, 20, "synthetic evidence")
	require.ErrorIs(t, err, ErrAccountQuotaSettlementPending)
	view, err := GetUsageReview(ctx, db, root.Id, receipt.RequestID)
	require.NoError(t, err)
	assert.Equal(t, AccountQuotaTerminalRecoveryOpen, view.State)
}

func TestPendingUsageReviewPagesDoNotSkipRequestsOrExposeOtherRoles(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	require.NoError(t, db.AutoMigrate(&LegacyUsageReservation{}, &UsageReviewDecision{}))
	ctx := context.Background()
	root := User{Username: "page-root", AffCode: "page-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	for i := 0; i < 21; i++ {
		id := fmt.Sprintf("page-%02d", i)
		_, err := PrepareLegacyUsageReservation(ctx, db, LegacyUsageReservation{RequestID: id, UserID: root.Id, TokenID: 1, FundingSource: "wallet"})
		require.NoError(t, err)
		require.NoError(t, SetTextDispatchEvidence(ctx, db, id, root.Id, 1, 0, `{"version":1}`, true))
	}
	page, err := ListPendingUsageReviews(ctx, db, root.Id, "legacy", 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 20)
	assert.Equal(t, "page-00", page.Items[0].RequestID)
	var after int64
	_, err = fmt.Sscan(page.NextAfter, &after)
	require.NoError(t, err)
	next, err := ListPendingUsageReviews(ctx, db, root.Id, "legacy", after)
	require.NoError(t, err)
	require.Len(t, next.Items, 1)
	assert.Equal(t, "page-20", next.Items[0].RequestID)
	assert.Empty(t, next.NextAfter)
	_, err = ListPendingUsageReviews(ctx, db, root.Id, strings.Repeat("x", 50), 0)
	require.ErrorIs(t, err, ErrAccountQuotaMutationInvalidInput)
}
