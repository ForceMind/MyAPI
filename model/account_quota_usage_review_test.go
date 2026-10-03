package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUnknownUsageHoldsReservationWithoutChargingOrRefunding(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	f := newAccountQuotaFixture(t, db, "usage-hold", 1000, 1000, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(f, "usage-hold-request", "wallet_only", 100))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	hold, err := HoldAccountQuotaUnknownUsage(ctx, db, reserve.RequestID, reserve.ID, "missing")
	require.NoError(t, err)
	assert.Equal(t, AccountQuotaTerminalRecoveryUsageUnknown, hold.State)
	assert.Empty(t, hold.Phase)
	assert.Zero(t, hold.TerminalReceiptID)
	replay, err := HoldAccountQuotaUnknownUsage(context.Background(), db, reserve.RequestID, reserve.ID, "estimated")
	require.NoError(t, err)
	assert.Equal(t, hold.LockVersion, replay.LockVersion)
	assert.Equal(t, "usage_missing", replay.LastError)
	_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 120})
	require.Error(t, err)
	_, err = RefundAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "ordinary-refund"})
	require.Error(t, err)
	_, _, err = EnsureAccountQuotaRefundRecovery(context.Background(), db, AccountQuotaTerminalInput{RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "background-refund"}, nil)
	require.Error(t, err)
	_, err = ExtendAccountQuotaReservation(context.Background(), db, reserve.ID, 150)
	require.Error(t, err)
	user, token, _ := loadAccountBalances(t, db, f)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 900, token.RemainQuota)
	var terminals int64
	require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).Where("request_id = ? AND phase IN ?", reserve.RequestID, []string{AccountQuotaPhaseSettle, AccountQuotaPhaseRefund}).Count(&terminals).Error)
	assert.Zero(t, terminals)
	blocked, err := AccountQuotaUsageNeedsReview(context.Background(), db, token.Id)
	require.NoError(t, err)
	assert.True(t, blocked)
}

func TestUnknownUsageResolutionRequiresAuthorityAndIsIdempotent(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	f := newAccountQuotaFixture(t, db, "usage-resolve", 1000, 1000, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(f, "usage-resolve", "wallet_only", 100))
	require.NoError(t, err)
	_, err = HoldAccountQuotaUnknownUsage(context.Background(), db, reserve.RequestID, reserve.ID, "estimated")
	require.NoError(t, err)
	input := AccountQuotaTerminalInput{RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 120}
	evidence := strings.Repeat("a", 64)
	_, err = ResolveAccountQuotaUnknownUsage(context.Background(), db, f.User.Id, input, evidence)
	require.ErrorIs(t, err, ErrAccountQuotaMutationIneligible)
	root := User{Username: "usage-review-root", AffCode: "usage-review-root", Status: common.UserStatusEnabled, Role: common.RoleRootUser}
	require.NoError(t, db.Create(&root).Error)
	_, err = ResolveAccountQuotaUnknownUsage(context.Background(), db, root.Id, input, "not-a-digest")
	require.ErrorIs(t, err, ErrAccountQuotaMutationInvalidInput)
	first, err := ResolveAccountQuotaUnknownUsage(context.Background(), db, root.Id, input, evidence)
	require.NoError(t, err)
	assert.Contains(t, first.AuditKey, evidence)
	replay, err := ResolveAccountQuotaUnknownUsage(context.Background(), db, root.Id, input, evidence)
	require.NoError(t, err)
	assert.Equal(t, first.ID, replay.ID)
	input.ActualQuota = 130
	_, err = ResolveAccountQuotaUnknownUsage(context.Background(), db, root.Id, input, evidence)
	require.Error(t, err)
	user, token, _ := loadAccountBalances(t, db, f)
	assert.Equal(t, 880, user.Quota)
	assert.Equal(t, 880, token.RemainQuota)
	blocked, err := AccountQuotaUsageNeedsReview(context.Background(), db, token.Id)
	require.NoError(t, err)
	assert.False(t, blocked)
}

func TestUnknownUsageSurvivesDatabaseReopenAndFailedResolution(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	f := newAccountQuotaFixture(t, db, "usage-reopen", 150, 150, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(f, "usage-reopen-request", "wallet_only", 100))
	require.NoError(t, err)
	_, err = HoldAccountQuotaUnknownUsage(context.Background(), db, reserve.RequestID, reserve.ID, "partial")
	require.NoError(t, err)
	root := User{Username: "usage-reopen-root", AffCode: "usage-reopen-root", Status: common.UserStatusEnabled, Role: common.RoleRootUser}
	require.NoError(t, db.Create(&root).Error)
	// Reopen the actual SQLite file with a new pool, rather than reusing a row
	// or process-local session as evidence of persistence.
	var database struct{ File string }
	require.NoError(t, db.Raw("PRAGMA database_list").Scan(&database).Error)
	require.NotEmpty(t, database.File)
	pool, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	reopened, err := gorm.Open(sqlite.Open(database.File), &gorm.Config{})
	require.NoError(t, err)
	newPool, err := reopened.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = newPool.Close() })
	require.NoError(t, InitializeAccountQuotaReservationHeadsWithDB(reopened))
	blocked, err := AccountQuotaUsageNeedsReview(context.Background(), reopened, f.Token.Id)
	require.NoError(t, err)
	assert.True(t, blocked)
	// A persistence failure must roll back reopening the lifecycle. Do not
	// redefine the existing settlement rule for already consumed overage.
	require.NoError(t, reopened.Callback().Create().Before("gorm:create").Register("test:reject-usage-terminal", func(tx *gorm.DB) {
		if receipt, ok := tx.Statement.Dest.(*AccountQuotaMutationReceipt); ok && receipt.Phase == AccountQuotaPhaseSettle {
			tx.AddError(errors.New("terminal storage unavailable"))
		}
	}))
	_, err = ResolveAccountQuotaUnknownUsage(context.Background(), reopened, root.Id,
		AccountQuotaTerminalInput{RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 200}, strings.Repeat("b", 64))
	require.Error(t, err)
	var held AccountQuotaTerminalRecoveryObligation
	require.NoError(t, reopened.Where("request_id = ?", reserve.RequestID).First(&held).Error)
	assert.Equal(t, AccountQuotaTerminalRecoveryUsageUnknown, held.State)
	user, token, _ := loadAccountBalances(t, reopened, f)
	assert.Equal(t, 50, user.Quota)
	assert.Equal(t, 50, token.RemainQuota)
}

func TestUnknownUsageConcurrentResolutionHasOneTerminalCharge(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	f := newAccountQuotaFixture(t, db, "usage-concurrent", 1000, 1000, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(f, "usage-concurrent", "wallet_only", 100))
	require.NoError(t, err)
	_, err = HoldAccountQuotaUnknownUsage(context.Background(), db, reserve.RequestID, reserve.ID, "missing")
	require.NoError(t, err)
	root := User{Username: "usage-concurrent-root", AffCode: "usage-concurrent-root", Status: common.UserStatusEnabled, Role: common.RoleRootUser}
	require.NoError(t, db.Create(&root).Error)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := ResolveAccountQuotaUnknownUsage(context.Background(), db, root.Id,
				AccountQuotaTerminalInput{RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 120}, strings.Repeat("d", 64))
			results <- err
		}()
	}
	close(start)
	for range 2 {
		require.NoError(t, <-results)
	}
	user, token, _ := loadAccountBalances(t, db, f)
	assert.Equal(t, 880, user.Quota)
	assert.Equal(t, 880, token.RemainQuota)
	var count int64
	require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).Where("request_id = ? AND phase = ?", reserve.RequestID, AccountQuotaPhaseSettle).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestUnknownUsageRejectsWrongReservationAndTerminalReplay(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	f := newAccountQuotaFixture(t, db, "usage-identity", 1000, 1000, false, 0, 0, false)
	first, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(f, "usage-first", "wallet_only", 100))
	require.NoError(t, err)
	second, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(f, "usage-second", "wallet_only", 100))
	require.NoError(t, err)
	_, err = HoldAccountQuotaUnknownUsage(context.Background(), db, first.RequestID, second.ID, "missing")
	require.ErrorIs(t, err, ErrAccountQuotaMutationStaleReceipt)
	_, err = HoldAccountQuotaUnknownUsage(context.Background(), db, first.RequestID, first.ID, "unbounded upstream error")
	require.ErrorIs(t, err, ErrAccountQuotaMutationInvalidInput)
	_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: first.RequestID, ReserveReceiptID: first.ID, ActualQuota: 120})
	require.NoError(t, err)
	_, err = HoldAccountQuotaUnknownUsage(context.Background(), db, first.RequestID, first.ID, "estimated")
	require.ErrorIs(t, err, ErrAccountQuotaMutationTerminal)
	blocked, err := AccountQuotaUsageNeedsReview(context.Background(), db, f.Token.Id)
	require.NoError(t, err)
	assert.False(t, blocked)
}
