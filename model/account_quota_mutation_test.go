package model

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openAccountQuotaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "account-quota.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &SubscriptionPlan{}, &UserSubscription{}, &UserQuotaMutationReceipt{}, &AccountQuotaMutationReceipt{}, &AccountQuotaReservationHead{}, &AccountQuotaTerminalRecoveryObligation{}, &AccountQuotaRefundFact{}, &QuotaWorkCursor{}, &QuotaWriterEpoch{}, &QuotaProjectionObligation{}))
	require.NoError(t, EnsureQuotaWriterEpochStateWithDB(db))
	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 7)
	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldRedisEnabled, oldRDB })
	return db
}

type accountQuotaFixture struct {
	User  User
	Token Token
	Sub   *UserSubscription
}

func newAccountQuotaFixture(t *testing.T, db *gorm.DB, label string, userQuota, tokenQuota int, withSubscription bool, subscriptionTotal, subscriptionUsed int64, allowOverflow bool) accountQuotaFixture {
	t.Helper()
	name := fmt.Sprintf("account-%s-%d", label, time.Now().UnixNano())
	user := User{Username: name, AffCode: name, Password: "fixture-password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: userQuota, QuotaVersion: 0, AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	token := Token{UserId: user.Id, Key: name, Status: common.TokenStatusEnabled, RemainQuota: tokenQuota, UsedQuota: 0, QuotaVersion: 0, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	fixture := accountQuotaFixture{User: user, Token: token}
	if withSubscription {
		plan := SubscriptionPlan{Title: name, Enabled: true, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: SubscriptionResetNever}
		require.NoError(t, db.Create(&plan).Error)
		sub := UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: subscriptionTotal, AmountUsed: subscriptionUsed,
			QuotaVersion: 0, StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), Status: "active", AllowWalletOverflow: allowOverflow}
		require.NoError(t, db.Create(&sub).Error)
		fixture.Sub = &sub
	}
	return fixture
}

func accountReserveInput(f accountQuotaFixture, requestID, preference string, quota int64) AccountQuotaReserveInput {
	return AccountQuotaReserveInput{
		RequestID: requestID, UserID: f.User.Id, TokenID: f.Token.Id, RequestedQuota: quota, BillingPreference: preference,
		BillingContext: AccountBillingContext{Version: 1, OriginModelName: "fixture-model", BillingPreference: preference},
	}
}

func loadAccountBalances(t *testing.T, db *gorm.DB, f accountQuotaFixture) (User, Token, *UserSubscription) {
	t.Helper()
	var user User
	var token Token
	require.NoError(t, db.First(&user, f.User.Id).Error)
	require.NoError(t, db.First(&token, f.Token.Id).Error)
	var sub *UserSubscription
	if f.Sub != nil {
		var row UserSubscription
		require.NoError(t, db.First(&row, f.Sub.Id).Error)
		sub = &row
	}
	return user, token, sub
}

func TestAccountQuotaReservePreferencesAndFallback(t *testing.T) {
	cases := []struct {
		name          string
		preference    string
		wallet        int
		subscription  bool
		subTotal      int64
		subUsed       int64
		allowOverflow bool
		wantSource    string
		wantErr       error
	}{
		{name: "wallet only", preference: "wallet_only", wallet: 200, wantSource: "wallet"},
		{name: "subscription only", preference: "subscription_only", wallet: 200, subscription: true, subTotal: 500, wantSource: "subscription"},
		{name: "wallet first fallback", preference: "wallet_first", wallet: 20, subscription: true, subTotal: 500, wantSource: "subscription"},
		{name: "subscription first fallback allowed", preference: "subscription_first", wallet: 200, subscription: true, subTotal: 50, allowOverflow: true, wantSource: "wallet"},
		{name: "subscription first strict", preference: "subscription_first", wallet: 200, subscription: true, subTotal: 50, allowOverflow: false, wantErr: ErrAccountQuotaMutationInsufficient},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			fixture := newAccountQuotaFixture(t, db, tc.name, tc.wallet, 500, tc.subscription, tc.subTotal, tc.subUsed, tc.allowOverflow)
			receipt, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "pref-"+tc.name, tc.preference, 100))
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, receipt)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, receipt)
			assert.Equal(t, tc.wantSource, receipt.BillingSource)
			assert.EqualValues(t, 100, receipt.AppliedQuota)
		})
	}
}

func TestAccountQuotaSettleAndRefundLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name       string
		actual     int64
		wantUser   int
		wantRemain int
		wantUsed   int
	}{
		{name: "exact", actual: 100, wantUser: 900, wantRemain: 400, wantUsed: 100},
		{name: "surplus", actual: 60, wantUser: 940, wantRemain: 440, wantUsed: 60},
		{name: "deficit", actual: 140, wantUser: 860, wantRemain: 360, wantUsed: 140},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			fixture := newAccountQuotaFixture(t, db, tc.name, 1000, 500, false, 0, 0, false)
			input := accountReserveInput(fixture, "settle-"+tc.name, "wallet_only", 100)
			reserve, err := ReserveAccountQuota(context.Background(), db, input)
			require.NoError(t, err)
			terminal, err := SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: tc.actual})
			require.NoError(t, err)
			assert.Equal(t, AccountQuotaPhaseSettle, terminal.Phase)
			user, token, _ := loadAccountBalances(t, db, fixture)
			assert.Equal(t, tc.wantUser, user.Quota)
			assert.Equal(t, tc.wantRemain, token.RemainQuota)
			assert.Equal(t, tc.wantUsed, token.UsedQuota)
			replay, err := SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: tc.actual})
			require.NoError(t, err)
			assert.Equal(t, terminal.ID, replay.ID)
		})
	}

	t.Run("refund", func(t *testing.T) {
		db := openAccountQuotaTestDB(t)
		fixture := newAccountQuotaFixture(t, db, "refund", 1000, 500, false, 0, 0, false)
		input := accountReserveInput(fixture, "refund-request", "wallet_only", 100)
		reserve, err := ReserveAccountQuota(context.Background(), db, input)
		require.NoError(t, err)
		refund, err := RefundAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "upstream-failed"})
		require.NoError(t, err)
		assert.Equal(t, AccountQuotaPhaseRefund, refund.Phase)
		assert.Equal(t, "upstream-failed", refund.AuditKey)
		user, token, _ := loadAccountBalances(t, db, fixture)
		assert.Equal(t, 1000, user.Quota)
		assert.Equal(t, 500, token.RemainQuota)
		assert.Zero(t, token.UsedQuota)
		_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 80})
		require.ErrorIs(t, err, ErrAccountQuotaMutationTerminal)
	})
}

func TestAccountQuotaReplayConflictRollbackAndCrashWindow(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "replay", 1000, 50, false, 0, 0, false)
	input := accountReserveInput(fixture, "same-request", "wallet_only", 100)
	_, err := ReserveAccountQuota(context.Background(), db, input)
	require.ErrorIs(t, err, ErrAccountQuotaMutationInsufficient)
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 50, token.RemainQuota)

	fixture.Token.RemainQuota = 500
	require.NoError(t, db.Model(&Token{}).Where("id = ?", fixture.Token.Id).Update("remain_quota", 500).Error)
	reserve, err := ReserveAccountQuota(context.Background(), db, input)
	require.NoError(t, err)
	replay, err := ReserveAccountQuota(context.Background(), db, input)
	require.NoError(t, err)
	assert.Equal(t, reserve.ID, replay.ID)
	conflict := input
	conflict.RequestedQuota = 101
	_, err = ReserveAccountQuota(context.Background(), db, conflict)
	require.ErrorIs(t, err, ErrAccountQuotaMutationConflict)

	storedReserve, terminal, err := InspectAccountQuotaRecovery(db, input.RequestID)
	require.NoError(t, err)
	require.NotNil(t, storedReserve)
	assert.Nil(t, terminal)
	user, token, _ = loadAccountBalances(t, db, fixture)
	assert.Equal(t, 900, user.Quota, "a crash after reserve keeps the reservation")
	assert.Equal(t, 400, token.RemainQuota)
}

func TestAccountQuotaBridgeOnlyFinishesExistingEpoch(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "bridge", 1000, 500, false, 0, 0, false)
	input := accountReserveInput(fixture, "bridge-existing", "wallet_only", 100)
	reserve, err := ReserveAccountQuota(context.Background(), db, input)
	require.NoError(t, err)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeBridge, reserve.WriterEpoch)
	_, err = ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "bridge-new", "wallet_only", 10))
	require.ErrorIs(t, err, ErrDurableQuotaWriterModeDisabled)
	_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 80})
	require.NoError(t, err)
}

func TestAccountQuotaConcurrentReplay(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "concurrent", 1000, 1000, false, 0, 0, false)
	input := accountReserveInput(fixture, "concurrent-same", "wallet_only", 100)
	var wg sync.WaitGroup
	receipts := make(chan *AccountQuotaMutationReceipt, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, err := ReserveAccountQuota(context.Background(), db, input)
			receipts <- receipt
			errs <- err
		}()
	}
	wg.Wait()
	close(receipts)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var id int64
	for receipt := range receipts {
		require.NotNil(t, receipt)
		if id == 0 {
			id = receipt.ID
		}
		assert.Equal(t, id, receipt.ID)
	}
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 900, token.RemainQuota)
	assert.Equal(t, 100, token.UsedQuota)

	terminal, err := FindAccountQuotaTerminalReceipt(db, input.RequestID)
	require.NoError(t, err)
	assert.Nil(t, terminal)
	assert.False(t, errors.Is(err, ErrAccountQuotaMutationConflict))
}

func TestAccountQuotaRedisProjectionFailureKeepsCommittedObligation(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "projection-failure", 1000, 500, false, 0, 0, false)
	input := accountReserveInput(fixture, "projection-failure", "wallet_only", 100)
	receipt, err := ReserveAccountQuota(context.Background(), db, input)
	require.NoError(t, err)
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 400, token.RemainQuota)
	var obligation QuotaProjectionObligation
	require.NoError(t, db.Where("receipt_kind = ? AND receipt_id = ?", quotaProjectionReceiptKindAccount, receipt.ID).First(&obligation).Error)
	assert.Equal(t, string(QuotaProjectionObligationStateRetryable), obligation.State)
	assert.Contains(t, obligation.LastError, "redis projection unavailable")
}

func TestAccountQuotaSubscriptionSettlementAndRefund(t *testing.T) {
	t.Run("settlement", func(t *testing.T) {
		db := openAccountQuotaTestDB(t)
		fixture := newAccountQuotaFixture(t, db, "subscription-settle", 1000, 500, true, 1000, 100, true)
		input := accountReserveInput(fixture, "subscription-settle", "subscription_only", 100)
		reserve, err := ReserveAccountQuota(context.Background(), db, input)
		require.NoError(t, err)
		_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 60})
		require.NoError(t, err)
		user, token, sub := loadAccountBalances(t, db, fixture)
		assert.Equal(t, 1000, user.Quota)
		assert.Equal(t, 440, token.RemainQuota)
		assert.Equal(t, 60, token.UsedQuota)
		require.NotNil(t, sub)
		assert.EqualValues(t, 160, sub.AmountUsed)
	})

	t.Run("refund", func(t *testing.T) {
		db := openAccountQuotaTestDB(t)
		fixture := newAccountQuotaFixture(t, db, "subscription-refund", 1000, 500, true, 1000, 100, true)
		input := accountReserveInput(fixture, "subscription-refund", "subscription_only", 100)
		reserve, err := ReserveAccountQuota(context.Background(), db, input)
		require.NoError(t, err)
		_, err = RefundAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "provider-rejected"})
		require.NoError(t, err)
		_, token, sub := loadAccountBalances(t, db, fixture)
		assert.Equal(t, 500, token.RemainQuota)
		assert.Zero(t, token.UsedQuota)
		require.NotNil(t, sub)
		assert.EqualValues(t, 100, sub.AmountUsed)
	})
}

func TestAccountQuotaTerminalConflictAndEpochMismatch(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "terminal-conflict", 1000, 500, false, 0, 0, false)
	input := accountReserveInput(fixture, "terminal-conflict", "wallet_only", 100)
	reserve, err := ReserveAccountQuota(context.Background(), db, input)
	require.NoError(t, err)
	_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 80})
	require.NoError(t, err)
	_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 81})
	require.ErrorIs(t, err, ErrAccountQuotaMutationConflict)

	second := newAccountQuotaFixture(t, db, "epoch", 1000, 500, false, 0, 0, false)
	secondInput := accountReserveInput(second, "epoch-mismatch", "wallet_only", 100)
	secondReserve, err := ReserveAccountQuota(context.Background(), db, secondInput)
	require.NoError(t, err)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeBridge, secondReserve.WriterEpoch+1)
	_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: secondInput.RequestID, ReserveReceiptID: secondReserve.ID, ActualQuota: 100})
	require.ErrorIs(t, err, ErrQuotaWriterEpochMismatch)
}

func TestAccountQuotaConcurrentDifferentRequests(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "concurrent-different", 1000, 1000, false, 0, 0, false)
	inputs := []AccountQuotaReserveInput{
		accountReserveInput(fixture, "concurrent-a", "wallet_only", 100),
		accountReserveInput(fixture, "concurrent-b", "wallet_only", 150),
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(inputs))
	for i := range inputs {
		input := inputs[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ReserveAccountQuota(context.Background(), db, input)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, 750, user.Quota)
	assert.Equal(t, 750, token.RemainQuota)
	assert.Equal(t, 250, token.UsedQuota)
}

func TestAccountQuotaReservationHeadConcurrentAdjustAndOldReceiptTerminal(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "head-adjust", 1000, 1000, false, 0, 0, false)
	input := accountReserveInput(fixture, "head-adjust", "wallet_only", 100)
	root, err := ReserveAccountQuota(context.Background(), db, input)
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, target := range []int64{140, 180} {
		wg.Add(1)
		go func(target int64) {
			defer wg.Done()
			_, extendErr := ExtendAccountQuotaReservation(context.Background(), db, root.ID, target)
			errs <- extendErr
		}(target)
	}
	wg.Wait()
	close(errs)
	for extendErr := range errs {
		require.NoError(t, extendErr)
	}

	head, err := findAccountQuotaReservationHead(db, input.RequestID)
	require.NoError(t, err)
	assert.EqualValues(t, 180, head.AppliedQuota)
	assert.NotEqual(t, root.ID, head.CurrentReceiptID)
	terminal, err := SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{
		RequestID: input.RequestID, ReserveReceiptID: root.ID, ActualQuota: 160,
	})
	require.NoError(t, err)
	assert.Equal(t, head.CurrentReceiptID, terminal.ParentReceiptID)
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, 840, user.Quota)
	assert.Equal(t, 840, token.RemainQuota)
	assert.Equal(t, 160, token.UsedQuota)
}

func TestAccountQuotaOldReceiptRefundUsesCurrentHead(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "head-refund", 1000, 1000, false, 0, 0, false)
	input := accountReserveInput(fixture, "head-refund", "wallet_only", 100)
	root, err := ReserveAccountQuota(context.Background(), db, input)
	require.NoError(t, err)
	current, err := ExtendAccountQuotaReservation(context.Background(), db, root.ID, 180)
	require.NoError(t, err)
	refund, err := RefundAccountQuota(context.Background(), db, AccountQuotaTerminalInput{
		RequestID: input.RequestID, ReserveReceiptID: root.ID, AuditKey: "old-receipt-refund",
	})
	require.NoError(t, err)
	assert.Equal(t, current.ID, refund.ParentReceiptID)
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
}

func TestAccountQuotaAdjustCommitUnknownReplaysStableIdentity(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "adjust-unknown", 1000, 1000, false, 0, 0, false)
	input := accountReserveInput(fixture, "adjust-unknown", "wallet_only", 100)
	root, err := ReserveAccountQuota(context.Background(), db, input)
	require.NoError(t, err)
	calls := 0
	accountQuotaTransactionAfterCommitHook = func(eventKey string) error {
		if strings.Contains(eventKey, "reserve-adjust") && calls == 0 {
			calls++
			return errors.New("commit acknowledgement lost")
		}
		return nil
	}
	t.Cleanup(func() { accountQuotaTransactionAfterCommitHook = nil })
	adjusted, err := ExtendAccountQuotaReservation(context.Background(), db, root.ID, 180)
	require.NoError(t, err)
	assert.EqualValues(t, 180, adjusted.AppliedQuota)
	assert.Equal(t, 1, calls)
	var count int64
	require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).Where("request_id = ?", input.RequestID).Count(&count).Error)
	assert.EqualValues(t, 2, count)
}

func TestAccountQuotaAdjustVersusTerminalSerializesOnHead(t *testing.T) {
	for round := 0; round < 8; round++ {
		db := openAccountQuotaTestDB(t)
		fixture := newAccountQuotaFixture(t, db, fmt.Sprintf("adjust-terminal-%d", round), 1000, 1000, false, 0, 0, false)
		requestID := fmt.Sprintf("adjust-terminal-%d", round)
		root, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, requestID, "wallet_only", 100))
		require.NoError(t, err)
		start := make(chan struct{})
		var wg sync.WaitGroup
		var adjustErr, settleErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, adjustErr = ExtendAccountQuotaReservation(context.Background(), db, root.ID, 180)
		}()
		go func() {
			defer wg.Done()
			<-start
			_, settleErr = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: requestID, ReserveReceiptID: root.ID, ActualQuota: 160})
		}()
		close(start)
		wg.Wait()
		require.NoError(t, settleErr)
		if adjustErr != nil {
			require.ErrorIs(t, adjustErr, ErrAccountQuotaMutationTerminal)
		}
		user, token, _ := loadAccountBalances(t, db, fixture)
		assert.Equal(t, 840, user.Quota)
		assert.Equal(t, 840, token.RemainQuota)
		assert.Equal(t, 160, token.UsedQuota)
	}
}

func TestAccountQuotaTerminalAllowsBoundedDeficit(t *testing.T) {
	for _, source := range []string{"wallet_only", "subscription_only"} {
		t.Run(source, func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			withSubscription := source == "subscription_only"
			fixture := newAccountQuotaFixture(t, db, "terminal-deficit-"+source, 100, 100, withSubscription, 100, 0, false)
			input := accountReserveInput(fixture, "terminal-deficit-"+source, source, 100)
			root, err := ReserveAccountQuota(context.Background(), db, input)
			require.NoError(t, err)
			_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: root.ID, ActualQuota: 150})
			require.NoError(t, err)
			user, token, subscription := loadAccountBalances(t, db, fixture)
			assert.Equal(t, -50, token.RemainQuota)
			assert.Equal(t, 150, token.UsedQuota)
			if withSubscription {
				assert.Equal(t, 100, user.Quota)
				require.NotNil(t, subscription)
				assert.EqualValues(t, 150, subscription.AmountUsed)
			} else {
				assert.Equal(t, -50, user.Quota)
			}
		})
	}
}

func TestAccountQuotaFreeAndPaidZeroMatrix(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, freeModel := range []bool{false, true} {
			for _, actual := range []int64{0, 3} {
				name := fmt.Sprintf("%s/free=%t/actual=%d", preference, freeModel, actual)
				t.Run(name, func(t *testing.T) {
					db := openAccountQuotaTestDB(t)
					withSubscription := preference == "subscription_only"
					fixture := newAccountQuotaFixture(t, db, name, 10, 10, withSubscription, 10, 0, false)
					input := accountReserveInput(fixture, "matrix-"+strings.ReplaceAll(name, "/", "-"), preference, 0)
					input.BillingContext.FreeModel = freeModel
					root, err := ReserveAccountQuota(context.Background(), db, input)
					require.NoError(t, err)
					if freeModel {
						assert.Equal(t, "free", root.BillingSource)
						assert.Zero(t, root.AppliedQuota)
					} else {
						assert.EqualValues(t, 1, root.AppliedQuota)
					}
					_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: root.ID, ActualQuota: actual})
					require.NoError(t, err)
					user, token, subscription := loadAccountBalances(t, db, fixture)
					charged := int(actual)
					if freeModel {
						charged = 0
					}
					assert.Equal(t, 10-charged, token.RemainQuota)
					assert.Equal(t, charged, token.UsedQuota)
					if withSubscription {
						assert.Equal(t, 10, user.Quota)
						require.NotNil(t, subscription)
						assert.EqualValues(t, charged, subscription.AmountUsed)
					} else {
						assert.Equal(t, 10-charged, user.Quota)
					}
				})
			}
		}
	}
}

func TestAccountQuotaReceiptFingerprintsRecomputeAndTamperingConflicts(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "fingerprint", 1000, 1000, false, 0, 0, false)
	input := accountReserveInput(fixture, "fingerprint", "wallet_only", 100)
	root, err := ReserveAccountQuota(context.Background(), db, input)
	require.NoError(t, err)
	adjusted, err := ExtendAccountQuotaReservation(context.Background(), db, root.ID, 140)
	require.NoError(t, err)
	terminal, err := SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: root.ID, ActualQuota: 120})
	require.NoError(t, err)
	refundInput := accountReserveInput(fixture, "fingerprint-refund", "wallet_only", 40)
	refundRoot, err := ReserveAccountQuota(context.Background(), db, refundInput)
	require.NoError(t, err)
	refund, err := RefundAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: refundInput.RequestID, ReserveReceiptID: refundRoot.ID, AuditKey: "fingerprint-refund"})
	require.NoError(t, err)
	for _, receipt := range []*AccountQuotaMutationReceipt{root, adjusted, terminal, refundRoot, refund} {
		recomputed, recomputeErr := RecomputeAccountQuotaReceiptFingerprint(receipt)
		require.NoError(t, recomputeErr)
		assert.Equal(t, receipt.RequestFingerprint, recomputed)
	}
	require.NoError(t, db.Exec("UPDATE account_quota_mutation_receipts SET audit_key = ? WHERE id = ?", "tampered", terminal.ID).Error)
	_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: root.ID, ActualQuota: 120})
	require.ErrorIs(t, err, ErrAccountQuotaMutationConflict)
}

func TestAccountQuotaSQLiteConcurrentReplayMultipleRounds(t *testing.T) {
	for round := 0; round < 5; round++ {
		t.Run(fmt.Sprintf("round-%d", round), func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			fixture := newAccountQuotaFixture(t, db, fmt.Sprintf("sqlite-replay-%d", round), 5000, 5000, false, 0, 0, false)
			requestID := fmt.Sprintf("sqlite-same-%d", round)
			input := accountReserveInput(fixture, requestID, "wallet_only", 100)
			const workers = 12
			reserveIDs := make(chan int64, workers)
			errs := make(chan error, workers)
			var wg sync.WaitGroup
			for range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					receipt, err := ReserveAccountQuota(context.Background(), db, input)
					if err == nil {
						reserveIDs <- receipt.ID
					}
					errs <- err
				}()
			}
			wg.Wait()
			close(errs)
			close(reserveIDs)
			for err := range errs {
				require.NoError(t, err)
			}
			var rootID int64
			for id := range reserveIDs {
				if rootID == 0 {
					rootID = id
				}
				assert.Equal(t, rootID, id)
			}

			adjustIDs := make(chan int64, workers)
			errs = make(chan error, workers)
			for range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					receipt, err := ExtendAccountQuotaReservation(context.Background(), db, rootID, 180)
					if err == nil {
						adjustIDs <- receipt.ID
					}
					errs <- err
				}()
			}
			wg.Wait()
			close(errs)
			close(adjustIDs)
			for err := range errs {
				require.NoError(t, err)
			}
			var adjustID int64
			for id := range adjustIDs {
				if adjustID == 0 {
					adjustID = id
				}
				assert.Equal(t, adjustID, id)
			}

			terminalIDs := make(chan int64, workers)
			errs = make(chan error, workers)
			for range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					receipt, err := SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: requestID, ReserveReceiptID: rootID, ActualQuota: 160})
					if err == nil {
						terminalIDs <- receipt.ID
					}
					errs <- err
				}()
			}
			wg.Wait()
			close(errs)
			close(terminalIDs)
			for err := range errs {
				require.NoError(t, err)
			}
			var terminalID int64
			for id := range terminalIDs {
				if terminalID == 0 {
					terminalID = id
				}
				assert.Equal(t, terminalID, id)
			}

			refundRequest := fmt.Sprintf("sqlite-refund-%d", round)
			refundRoot, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, refundRequest, "wallet_only", 50))
			require.NoError(t, err)
			refundIDs := make(chan int64, workers)
			errs = make(chan error, workers)
			for range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					receipt, err := RefundAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: refundRequest, ReserveReceiptID: refundRoot.ID, AuditKey: "duplicate-refund"})
					if err == nil {
						refundIDs <- receipt.ID
					}
					errs <- err
				}()
			}
			wg.Wait()
			close(errs)
			close(refundIDs)
			for err := range errs {
				require.NoError(t, err)
			}
			var refundID int64
			for id := range refundIDs {
				if refundID == 0 {
					refundID = id
				}
				assert.Equal(t, refundID, id)
			}

			// Different events contend on the same balances but each applies once.
			errs = make(chan error, workers)
			for worker := 0; worker < workers; worker++ {
				wg.Add(1)
				go func(worker int) {
					defer wg.Done()
					_, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, fmt.Sprintf("sqlite-different-%d-%d", round, worker), "wallet_only", 10))
					errs <- err
				}(worker)
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				require.NoError(t, err)
			}
			user, token, _ := loadAccountBalances(t, db, fixture)
			assert.Equal(t, 5000-160-workers*10, user.Quota)
			assert.Equal(t, 5000-160-workers*10, token.RemainQuota)
		})
	}
}

func TestAccountQuotaCommitUnknownReplaysReserveSettleAndRefund(t *testing.T) {
	for _, phase := range []string{AccountQuotaPhaseReserve, AccountQuotaPhaseSettle, AccountQuotaPhaseRefund} {
		t.Run(phase, func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			fixture := newAccountQuotaFixture(t, db, "unknown-"+phase, 1000, 1000, false, 0, 0, false)
			requestID := "unknown-" + phase
			calls := 0
			accountQuotaTransactionAfterCommitHook = func(eventKey string) error {
				if strings.Contains(eventKey, ":"+phase+":") && calls == 0 {
					calls++
					return errors.New("commit acknowledgement lost")
				}
				return nil
			}
			t.Cleanup(func() { accountQuotaTransactionAfterCommitHook = nil })
			root, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, requestID, "wallet_only", 100))
			require.NoError(t, err)
			if phase == AccountQuotaPhaseSettle {
				_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: requestID, ReserveReceiptID: root.ID, ActualQuota: 80})
				require.NoError(t, err)
			} else if phase == AccountQuotaPhaseRefund {
				_, err = RefundAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: requestID, ReserveReceiptID: root.ID, AuditKey: "commit-unknown"})
				require.NoError(t, err)
			}
			assert.Equal(t, 1, calls)
			var eventCount int64
			require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).Where("event_key = ?", accountQuotaEventKey(requestID, phase)).Count(&eventCount).Error)
			assert.EqualValues(t, 1, eventCount)
		})
	}
}

func TestAccountQuotaReserveCreatesLifecycleObligationAtomically(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "lifecycle-atomic", 1000, 500, false, 0, 0, false)
	receipt, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "lifecycle-created", "wallet_only", 100))
	require.NoError(t, err)
	var obligation AccountQuotaTerminalRecoveryObligation
	require.NoError(t, db.Where("request_id = ?", receipt.RequestID).First(&obligation).Error)
	assert.Equal(t, AccountQuotaTerminalRecoveryOpen, obligation.State)
	assert.Equal(t, receipt.ID, obligation.ReserveReceiptID)
	assert.Equal(t, receipt.ReserveFingerprint, obligation.RequestFingerprint)

	const callbackName = "test:fail_account_quota_lifecycle_create"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*AccountQuotaTerminalRecoveryObligation); ok {
			tx.AddError(errors.New("lifecycle obligation create failed"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })
	failedFixture := newAccountQuotaFixture(t, db, "lifecycle-rollback", 1000, 500, false, 0, 0, false)
	_, err = ReserveAccountQuota(context.Background(), db, accountReserveInput(failedFixture, "lifecycle-rollback", "wallet_only", 100))
	require.Error(t, err)
	user, token, _ := loadAccountBalances(t, db, failedFixture)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 500, token.RemainQuota)
	var receiptCount, headCount, obligationCount int64
	require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).Where("request_id = ?", "lifecycle-rollback").Count(&receiptCount).Error)
	require.NoError(t, db.Model(&AccountQuotaReservationHead{}).Where("request_id = ?", "lifecycle-rollback").Count(&headCount).Error)
	require.NoError(t, db.Model(&AccountQuotaTerminalRecoveryObligation{}).Where("request_id = ?", "lifecycle-rollback").Count(&obligationCount).Error)
	assert.Zero(t, receiptCount)
	assert.Zero(t, headCount)
	assert.Zero(t, obligationCount)
}

func TestAccountQuotaRefundPendingBlocksSettleAndExtendUntilRecovery(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "refund-pending", 1000, 500, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "refund-pending", "wallet_only", 100))
	require.NoError(t, err)
	input := AccountQuotaTerminalInput{RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "upstream-failure"}
	obligation, terminal, err := EnsureAccountQuotaRefundRecovery(context.Background(), db, input, errors.New("upstream failed"))
	require.NoError(t, err)
	require.Nil(t, terminal)
	assert.Equal(t, AccountQuotaTerminalRecoveryRefundPending, obligation.State)
	_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 50})
	require.ErrorIs(t, err, ErrAccountQuotaMutationTerminal)
	_, err = ExtendAccountQuotaReservation(context.Background(), db, reserve.ID, 150)
	require.ErrorIs(t, err, ErrAccountQuotaMutationTerminal)

	restoreGate := setQuotaProjectionRecoveryGateForTest(true)
	defer restoreGate()
	processed, err := RunAccountQuotaTerminalRecoveryObligations(context.Background(), db, "refund-worker", 10)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.NoError(t, db.First(obligation, obligation.ID).Error)
	assert.Equal(t, AccountQuotaTerminalRecoveryApplied, obligation.State)
	var terminalReceipt AccountQuotaMutationReceipt
	require.NoError(t, db.First(&terminalReceipt, obligation.TerminalReceiptID).Error)
	assert.Equal(t, AccountQuotaPhaseRefund, terminalReceipt.Phase)
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 500, token.RemainQuota)
}

func TestAccountQuotaCommitUnknownReadbackIgnoresCancelledContext(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "detached-readback", 1000, 500, false, 0, 0, false)
	ctx, cancel := context.WithCancel(context.Background())
	accountQuotaTransactionAfterCommitHook = func(string) error {
		cancel()
		return context.Canceled
	}
	t.Cleanup(func() { accountQuotaTransactionAfterCommitHook = nil })
	receipt, err := ReserveAccountQuota(ctx, db, accountReserveInput(fixture, "detached-readback", "wallet_only", 100))
	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Equal(t, AccountQuotaPhaseReserve, receipt.Phase)
}

func TestAccountQuotaBadRecoveryObligationDoesNotStarveLaterRefund(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	firstFixture := newAccountQuotaFixture(t, db, "bad-obligation", 1000, 500, false, 0, 0, false)
	secondFixture := newAccountQuotaFixture(t, db, "good-obligation", 1000, 500, false, 0, 0, false)
	first, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(firstFixture, "bad-obligation", "wallet_only", 100))
	require.NoError(t, err)
	second, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(secondFixture, "good-obligation", "wallet_only", 100))
	require.NoError(t, err)
	firstObligation, _, err := EnsureAccountQuotaRefundRecovery(context.Background(), db, AccountQuotaTerminalInput{RequestID: first.RequestID, ReserveReceiptID: first.ID, AuditKey: "first"}, errors.New("first"))
	require.NoError(t, err)
	secondObligation, _, err := EnsureAccountQuotaRefundRecovery(context.Background(), db, AccountQuotaTerminalInput{RequestID: second.RequestID, ReserveReceiptID: second.ID, AuditKey: "second"}, errors.New("second"))
	require.NoError(t, err)
	require.NoError(t, db.Model(&AccountQuotaTerminalRecoveryObligation{}).Where("id = ?", firstObligation.ID).Update("request_fingerprint", strings.Repeat("0", 64)).Error)
	restoreGate := setQuotaProjectionRecoveryGateForTest(true)
	defer restoreGate()
	processed, runErr := RunAccountQuotaTerminalRecoveryObligations(context.Background(), db, "recovery-worker", 10)
	require.Error(t, runErr)
	assert.Equal(t, 1, processed)
	require.NoError(t, db.First(firstObligation, firstObligation.ID).Error)
	require.NoError(t, db.First(secondObligation, secondObligation.ID).Error)
	assert.Equal(t, AccountQuotaTerminalRecoveryManual, firstObligation.State)
	assert.Equal(t, AccountQuotaTerminalRecoveryApplied, secondObligation.State)
}

func TestAccountQuotaTerminalLifecycleCloseIsAtomic(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "terminal-atomic", 1000, 500, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "terminal-atomic", "wallet_only", 100))
	require.NoError(t, err)
	const callbackName = "test:fail_account_quota_lifecycle_close"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == (AccountQuotaTerminalRecoveryObligation{}).TableName() {
			tx.AddError(errors.New("lifecycle close failed"))
		}
	}))
	_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 60})
	require.Error(t, err)
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 400, token.RemainQuota)
	var terminalCount int64
	require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).Where("request_id = ? AND phase = ?", reserve.RequestID, AccountQuotaPhaseSettle).Count(&terminalCount).Error)
	assert.Zero(t, terminalCount)
	var head AccountQuotaReservationHead
	require.NoError(t, db.Where("request_id = ?", reserve.RequestID).First(&head).Error)
	assert.Zero(t, head.TerminalReceiptID)
	var obligation AccountQuotaTerminalRecoveryObligation
	require.NoError(t, db.Where("request_id = ?", reserve.RequestID).First(&obligation).Error)
	assert.Equal(t, AccountQuotaTerminalRecoveryOpen, obligation.State)

	require.NoError(t, db.Callback().Update().Remove(callbackName))
	terminal, err := SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 60})
	require.NoError(t, err)
	require.NoError(t, db.First(&obligation, obligation.ID).Error)
	assert.Equal(t, AccountQuotaTerminalRecoveryApplied, obligation.State)
	assert.Equal(t, terminal.ID, obligation.TerminalReceiptID)
}

func TestAccountQuotaRefundIntentUpdateAndCommitUnknownReadback(t *testing.T) {
	t.Run("update failure remains open", func(t *testing.T) {
		db := openAccountQuotaTestDB(t)
		fixture := newAccountQuotaFixture(t, db, "intent-update-fail", 1000, 500, false, 0, 0, false)
		reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "intent-update-fail", "wallet_only", 100))
		require.NoError(t, err)
		const callbackName = "test:refund_intent_update_failure"
		require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
			if tx.Statement.Table == (AccountQuotaTerminalRecoveryObligation{}).TableName() {
				tx.AddError(errors.New("refund intent update failed"))
			}
		}))
		_, _, err = EnsureAccountQuotaRefundRecovery(context.Background(), db, AccountQuotaTerminalInput{
			RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "update-failure",
		}, nil)
		require.ErrorContains(t, err, "refund intent update failed")
		require.NoError(t, db.Callback().Update().Remove(callbackName))
		var obligation AccountQuotaTerminalRecoveryObligation
		require.NoError(t, db.Where("request_id = ?", reserve.RequestID).First(&obligation).Error)
		assert.Equal(t, AccountQuotaTerminalRecoveryOpen, obligation.State)
	})

	t.Run("commit success response lost is proven", func(t *testing.T) {
		db := openAccountQuotaTestDB(t)
		fixture := newAccountQuotaFixture(t, db, "intent-response-lost", 1000, 500, false, 0, 0, false)
		reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "intent-response-lost", "wallet_only", 100))
		require.NoError(t, err)
		accountQuotaRefundIntentAfterCommitHook = func(string) error { return errors.New("intent commit response lost") }
		t.Cleanup(func() { accountQuotaRefundIntentAfterCommitHook = nil })
		obligation, terminal, err := EnsureAccountQuotaRefundRecovery(context.Background(), db, AccountQuotaTerminalInput{
			RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "response-lost",
		}, nil)
		require.NoError(t, err)
		require.Nil(t, terminal)
		require.NotNil(t, obligation)
		assert.Equal(t, AccountQuotaTerminalRecoveryRefundPending, obligation.State)
	})

	t.Run("readback failure stays typed unknown", func(t *testing.T) {
		db := openAccountQuotaTestDB(t)
		fixture := newAccountQuotaFixture(t, db, "intent-readback-fail", 1000, 500, false, 0, 0, false)
		reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "intent-readback-fail", "wallet_only", 100))
		require.NoError(t, err)
		accountQuotaRefundIntentAfterCommitHook = func(string) error { return errors.New("intent commit response lost") }
		accountQuotaRefundIntentReadbackHook = func() error { return errors.New("intent readback unavailable") }
		t.Cleanup(func() {
			accountQuotaRefundIntentAfterCommitHook = nil
			accountQuotaRefundIntentReadbackHook = nil
		})
		_, _, err = EnsureAccountQuotaRefundRecovery(context.Background(), db, AccountQuotaTerminalInput{
			RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "readback-failure",
		}, nil)
		require.ErrorIs(t, err, ErrAccountQuotaRefundIntentUnknown)
		var obligation AccountQuotaTerminalRecoveryObligation
		require.NoError(t, db.Where("request_id = ?", reserve.RequestID).First(&obligation).Error)
		assert.Equal(t, AccountQuotaTerminalRecoveryRefundPending, obligation.State)
	})
}

func TestAccountQuotaRecoveryKeysetSkipsStructuralRowsAndChecksManualUpdate(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	badRows := make([]AccountQuotaTerminalRecoveryObligation, 0, 31)
	for index := 0; index < 30; index++ {
		badRows = append(badRows, AccountQuotaTerminalRecoveryObligation{
			RequestID: fmt.Sprintf("missing-head-%03d", index), ReserveReceiptID: int64(index + 1), Phase: AccountQuotaPhaseRefund,
			AuditKey: "missing-head", WriterEpoch: 7, RequestFingerprint: strings.Repeat("a", 64),
			State: AccountQuotaTerminalRecoveryRefundPending, LockVersion: 1, CreatedAt: int64(index + 1), UpdatedAt: int64(index + 1),
		})
	}
	require.NoError(t, db.CreateInBatches(&badRows, 20).Error)
	missingReceiptHead := AccountQuotaReservationHead{
		RequestID: "missing-receipt", WriterEpoch: 7, RootReceiptID: 999001, CurrentReceiptID: 999001,
		ReserveFingerprint: strings.Repeat("b", 64), UserID: 1, TokenID: 1, BillingSource: "wallet", LockVersion: 1, CreatedAt: 40, UpdatedAt: 40,
	}
	require.NoError(t, db.Create(&missingReceiptHead).Error)
	missingReceipt := AccountQuotaTerminalRecoveryObligation{
		RequestID: missingReceiptHead.RequestID, ReserveReceiptID: missingReceiptHead.CurrentReceiptID, Phase: AccountQuotaPhaseRefund,
		AuditKey: "missing-receipt", WriterEpoch: 7, RequestFingerprint: strings.Repeat("c", 64),
		State: AccountQuotaTerminalRecoveryRefundPending, LockVersion: 1, CreatedAt: 40, UpdatedAt: 40,
	}
	require.NoError(t, db.Create(&missingReceipt).Error)

	fixture := newAccountQuotaFixture(t, db, "keyset-good", 1000, 500, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "keyset-good", "wallet_only", 100))
	require.NoError(t, err)
	good, _, err := EnsureAccountQuotaRefundRecovery(context.Background(), db, AccountQuotaTerminalInput{
		RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "good",
	}, nil)
	require.NoError(t, err)
	failedManualID := badRows[0].ID
	accountQuotaRecoveryManualUpdateHook = func(candidate *AccountQuotaTerminalRecoveryObligation) error {
		if candidate.ID == failedManualID {
			return errors.New("manual update failed")
		}
		return nil
	}
	t.Cleanup(func() { accountQuotaRecoveryManualUpdateHook = nil })
	restoreGate := setQuotaProjectionRecoveryGateForTest(true)
	defer restoreGate()
	processed, runErr := RunAccountQuotaTerminalRecoveryObligations(context.Background(), db, "keyset-worker", 32)
	require.ErrorContains(t, runErr, "manual update failed")
	assert.Equal(t, 1, processed)
	require.NoError(t, db.First(good, good.ID).Error)
	assert.Equal(t, AccountQuotaTerminalRecoveryApplied, good.State)
	var failedManual AccountQuotaTerminalRecoveryObligation
	require.NoError(t, db.First(&failedManual, failedManualID).Error)
	assert.Equal(t, AccountQuotaTerminalRecoveryRefundPending, failedManual.State)
	var manualCount int64
	require.NoError(t, db.Model(&AccountQuotaTerminalRecoveryObligation{}).Where("state = ?", AccountQuotaTerminalRecoveryManual).Count(&manualCount).Error)
	assert.EqualValues(t, 30, manualCount)
}

func TestAccountQuotaRecoveryLimitBoundsScannedAndPersistsCursor(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	badRows := make([]AccountQuotaTerminalRecoveryObligation, 0, 15)
	for index := 0; index < 15; index++ {
		badRows = append(badRows, AccountQuotaTerminalRecoveryObligation{
			RequestID: fmt.Sprintf("limited-missing-%02d", index), ReserveReceiptID: int64(index + 1), Phase: AccountQuotaPhaseRefund,
			AuditKey: "limited", WriterEpoch: 7, RequestFingerprint: strings.Repeat("d", 64),
			State: AccountQuotaTerminalRecoveryRefundPending, LockVersion: 1, CreatedAt: int64(index + 1), UpdatedAt: int64(index + 1),
		})
	}
	require.NoError(t, db.CreateInBatches(&badRows, 10).Error)
	fixture := newAccountQuotaFixture(t, db, "limited-good", 1000, 500, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "limited-good", "wallet_only", 100))
	require.NoError(t, err)
	good, _, err := EnsureAccountQuotaRefundRecovery(context.Background(), db, AccountQuotaTerminalInput{RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "good"}, nil)
	require.NoError(t, err)
	restoreGate := setQuotaProjectionRecoveryGateForTest(true)
	defer restoreGate()
	processed, runErr := RunAccountQuotaTerminalRecoveryObligations(context.Background(), db, "limited-worker", 10)
	require.Error(t, runErr)
	assert.Zero(t, processed)
	var manualCount int64
	require.NoError(t, db.Model(&AccountQuotaTerminalRecoveryObligation{}).Where("state = ?", AccountQuotaTerminalRecoveryManual).Count(&manualCount).Error)
	assert.EqualValues(t, 10, manualCount)
	var cursor QuotaWorkCursor
	require.NoError(t, db.Where("name = ?", quotaWorkCursorRefundRecovery).First(&cursor).Error)
	assert.Equal(t, badRows[9].ID, cursor.LastID)

	processed, runErr = RunAccountQuotaTerminalRecoveryObligations(context.Background(), db, "limited-worker", 10)
	require.Error(t, runErr)
	assert.Equal(t, 1, processed)
	require.NoError(t, db.First(good, good.ID).Error)
	assert.Equal(t, AccountQuotaTerminalRecoveryApplied, good.State)
	require.NoError(t, db.Model(&AccountQuotaTerminalRecoveryObligation{}).Where("state = ?", AccountQuotaTerminalRecoveryManual).Count(&manualCount).Error)
	assert.EqualValues(t, 15, manualCount)
}

func TestAccountQuotaContextAwareOperationsHonorCancellation(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "cancel-context", 1000, 500, false, 0, 0, false)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ReserveAccountQuota(cancelled, db, accountReserveInput(fixture, "cancel-before-reserve", "wallet_only", 100))
	require.ErrorIs(t, err, context.Canceled)
	var count int64
	require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).Where("request_id = ?", "cancel-before-reserve").Count(&count).Error)
	assert.Zero(t, count)

	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "cancel-terminal", "wallet_only", 100))
	require.NoError(t, err)
	_, err = SettleAccountQuota(cancelled, db, AccountQuotaTerminalInput{RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 50})
	require.ErrorIs(t, err, context.Canceled)
	var head AccountQuotaReservationHead
	require.NoError(t, db.Where("request_id = ?", reserve.RequestID).First(&head).Error)
	assert.Zero(t, head.TerminalReceiptID)

	fact, err := EnsureAccountQuotaRefundFact(context.Background(), db, AccountQuotaRefundFactInput{
		EventKey: "billing-refund:cancel-terminal:v1", Kind: AccountQuotaRefundFactKindAuthoritative,
		RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "cancelled",
		WriterEpoch: reserve.WriterEpoch, UserID: reserve.UserID, TokenID: reserve.TokenID,
	})
	require.NoError(t, err)
	processed, err := RunAccountQuotaRefundFacts(cancelled, db, "cancelled-worker", 10)
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, processed)
	require.NoError(t, db.First(fact, fact.ID).Error)
	assert.Equal(t, AccountQuotaRefundFactPending, fact.State)
}

func TestAccountQuotaTerminalRecoveryHighWatermarkPreventsContinuousInsertStarvation(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "terminal-fairness", 1000, 500, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "terminal-fairness-old", "wallet_only", 100))
	require.NoError(t, err)
	old, _, err := EnsureAccountQuotaRefundRecovery(context.Background(), db, AccountQuotaTerminalInput{
		RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "terminal-fairness",
	}, nil)
	require.NoError(t, err)
	require.NoError(t, db.Model(old).Updates(map[string]any{
		"state": AccountQuotaTerminalRecoveryRetryable, "next_attempt_at": time.Now().Add(time.Hour).Unix(),
	}).Error)
	poison := func(requestID string) AccountQuotaTerminalRecoveryObligation {
		return AccountQuotaTerminalRecoveryObligation{
			RequestID: requestID, ReserveReceiptID: 999999, Phase: AccountQuotaPhaseRefund, AuditKey: "poison",
			WriterEpoch: 7, RequestFingerprint: strings.Repeat("f", 64), State: AccountQuotaTerminalRecoveryRefundPending,
			LockVersion: 1, CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		}
	}
	newer := poison("terminal-fairness-newer")
	require.NoError(t, db.Create(&newer).Error)
	restoreGate := setQuotaProjectionRecoveryGateForTest(true)
	defer restoreGate()
	_, firstErr := RunAccountQuotaTerminalRecoveryObligations(context.Background(), db, "terminal-fairness-worker", 1)
	require.Error(t, firstErr)

	require.NoError(t, db.Model(old).Updates(map[string]any{"next_attempt_at": int64(0)}).Error)
	for index := 0; index < 3; index++ {
		row := poison(fmt.Sprintf("terminal-fairness-continuous-%d", index))
		require.NoError(t, db.Create(&row).Error)
	}
	processed, err := RunAccountQuotaTerminalRecoveryObligations(context.Background(), db, "terminal-fairness-worker", 1)
	require.NoError(t, err)
	assert.Zero(t, processed)
	row := poison("terminal-fairness-after-reset")
	require.NoError(t, db.Create(&row).Error)
	processed, err = RunAccountQuotaTerminalRecoveryObligations(context.Background(), db, "terminal-fairness-worker", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.NoError(t, db.First(old, old.ID).Error)
	assert.Equal(t, AccountQuotaTerminalRecoveryApplied, old.State)
}

func TestAuthoritativeEntryFailsClosedWhenMaintenanceCursorIsIncomplete(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "authoritative-cursor-guard", 1000, 500, false, 0, 0, false)
	require.NoError(t, db.Model(&QuotaWorkCursor{}).Where("name = ?", quotaWorkCursorAccountMigration).Updates(map[string]any{
		"complete": false, "last_id": int64(0), "high_watermark": int64(0),
	}).Error)

	_, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "authoritative-cursor-blocked", "wallet_only", 100))
	require.ErrorIs(t, err, ErrQuotaMaintenanceBackfillIncomplete)
}

func TestAuthoritativeEntryFailsClosedWhileV1ReceiptBackfillIsIncomplete(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "authoritative-backfill-guard", 1000, 500, false, 0, 0, false)
	legacy := AccountQuotaMutationReceipt{
		ReceiptVersion: AccountQuotaMutationReceiptVersion, FingerprintVersion: 1, WriterEpoch: 7,
		RequestID: "authoritative-v1-replay", Phase: AccountQuotaPhaseReserve,
		EventKey:           accountQuotaEventKey("authoritative-v1-replay", AccountQuotaPhaseReserve),
		MutationSlot:       accountQuotaMutationSlot("authoritative-v1-replay", AccountQuotaPhaseReserve),
		RequestFingerprint: strings.Repeat("a", 64), BillingSource: "wallet", BillingPreference: "wallet_only",
		UserID: fixture.User.Id, TokenID: fixture.Token.Id, RequestedQuota: 100, AppliedQuota: 100,
		BillingContext: AccountBillingContext{Version: 1, OriginModelName: "fixture-model", BillingPreference: "wallet_only"}, CreatedAt: 1,
	}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&legacy).Error)
	require.NoError(t, db.Model(&QuotaWorkCursor{}).Where("name = ?", quotaWorkCursorAccountMigration).Updates(map[string]any{
		"complete": false, "last_id": int64(0),
	}).Error)

	_, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "authoritative-new-blocked", "wallet_only", 100))
	require.ErrorIs(t, err, ErrQuotaMaintenanceBackfillIncomplete)
	_, err = ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, legacy.RequestID, "wallet_only", 100))
	require.ErrorIs(t, err, ErrQuotaMaintenanceBackfillIncomplete)
	_, err = SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{
		RequestID: legacy.RequestID, ReserveReceiptID: legacy.ID, ActualQuota: 80,
	})
	require.ErrorIs(t, err, ErrQuotaMaintenanceBackfillIncomplete)
	_, err = RefundAccountQuota(context.Background(), db, AccountQuotaTerminalInput{
		RequestID: legacy.RequestID, ReserveReceiptID: legacy.ID, AuditKey: "blocked-refund",
	})
	require.ErrorIs(t, err, ErrQuotaMaintenanceBackfillIncomplete)
}
