package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newSettleTestFixture(t *testing.T, db *gorm.DB, label string, quota int64) TaskQuotaReservationInput {
	t.Helper()
	digest := sha256.Sum256([]byte(t.Name() + ":" + label))
	name := fmt.Sprintf("qs-%x", digest[:8])
	userQuota := 1000
	tokenRemain := 500
	if quota > 500 {
		tokenRemain = int(quota) + 500
	}
	if quota > 1000 {
		userQuota = int(quota) + 1000
	}
	user := User{Username: name, AffCode: name, Password: "synthetic-fixture", Status: common.UserStatusEnabled, Quota: userQuota}
	require.NoError(t, db.Create(&user).Error)
	token := Token{UserId: user.Id, Key: name, Status: common.TokenStatusEnabled, RemainQuota: tokenRemain, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, db.FirstOrCreate(&Channel{Id: 61, Name: "settle-test"}, Channel{Id: 61}).Error)
	operation := newB2SubmissionOperation(t, token.Id, "POST", TaskSubmissionOperationKindVideoCreate, name, "{}")
	operation.UserID = user.Id
	intent, err := CreateOrLoadTaskSubmissionIntent(db, operation, &TaskSubmissionAttempt{
		AttemptNo: 1, ChannelID: 61, Provider: "fixture", RequestClass: "video",
	})
	require.NoError(t, err)
	return TaskQuotaReservationInput{
		OperationID: intent.Operation.ID, UserID: user.Id, TokenID: token.Id,
		ChannelID: 61, ExpectedOperationVersion: intent.Operation.LockVersion,
		Quota: quota, BillingSource: "wallet", ApplyStatistics: true,
		BillingContext: TaskBillingContext{
			Version: TaskBillingContextVersion, Complete: true, ModelPrice: 1,
			GroupRatio: 1, OriginModelName: "quota-settle-fixture", PerCallBilling: true,
		},
	}
}

func newSettleSubscriptionFixture(t *testing.T, db *gorm.DB, label string, quota int64) TaskQuotaReservationInput {
	t.Helper()
	digest := sha256.Sum256([]byte(t.Name() + ":" + label))
	name := fmt.Sprintf("qsub-%x", digest[:8])
	user := User{Username: name, AffCode: name, Password: "synthetic-fixture", Status: common.UserStatusEnabled, Quota: 1000}
	require.NoError(t, db.Create(&user).Error)
	token := Token{UserId: user.Id, Key: name, Status: common.TokenStatusEnabled, RemainQuota: 500, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, db.FirstOrCreate(&Channel{Id: 61, Name: "settle-test"}, Channel{Id: 61}).Error)
	subscription := UserSubscription{
		UserId: user.Id, PlanId: 1, AmountTotal: 1000, AmountUsed: 200,
		Status: "active", StartTime: 1, EndTime: 1<<31 - 1,
	}
	require.NoError(t, db.Create(&subscription).Error)
	operation := newB2SubmissionOperation(t, token.Id, "POST", TaskSubmissionOperationKindVideoCreate, name, "{}")
	operation.UserID = user.Id
	intent, err := CreateOrLoadTaskSubmissionIntent(db, operation, &TaskSubmissionAttempt{
		AttemptNo: 1, ChannelID: 61, Provider: "fixture", RequestClass: "video",
	})
	require.NoError(t, err)
	return TaskQuotaReservationInput{
		OperationID: intent.Operation.ID, UserID: user.Id, TokenID: token.Id,
		ChannelID: 61, ExpectedOperationVersion: intent.Operation.LockVersion,
		Quota: quota, BillingSource: "subscription", SubscriptionID: subscription.Id, ApplyStatistics: true,
		BillingContext: TaskBillingContext{
			Version: TaskBillingContextVersion, Complete: true, ModelPrice: 1,
			GroupRatio: 1, OriginModelName: "quota-settle-sub-fixture", PerCallBilling: true,
		},
	}
}

// Matrix 1: T4 Release (全额释放/退款)
func TestTaskQuotaRelease_Matrix1(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)

	t.Run("wallet_release_restores_balances", func(t *testing.T) {
		input := newSettleTestFixture(t, db, "wallet-release", 100)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)
		require.NotNil(t, reserveReceipt)

		// Before release: user=900, token remain=400, token used=100
		assertTaskQuotaWallet(t, db, input, 900, 400, 100, 1)

		releaseInput := TaskQuotaReleaseInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ReasonCode:               "task_rejected",
			BillingContext:           input.BillingContext,
		}
		receipt, err := ReleaseTaskQuotaReservation(db, releaseInput)
		require.NoError(t, err)
		require.NotNil(t, receipt)

		// After release: user=1000, token remain=500, token used=0, versions advanced to 2
		assertTaskQuotaWallet(t, db, input, 1000, 500, 0, 2)
		assert.Equal(t, string(TaskBillingEventTypeRefund), receipt.MutationType)
		assert.Equal(t, int64(100), receipt.Quota)

		// Check stored receipt
		stored, err := FindTaskQuotaReceipt(db, input.OperationID, string(TaskBillingEventTypeRefund), input.UserID, input.TokenID)
		require.NoError(t, err)
		require.NotNil(t, stored)
		assert.Equal(t, receipt.MutationKey, stored.MutationKey)
	})

	t.Run("subscription_release_restores_usage", func(t *testing.T) {
		input := newSettleSubscriptionFixture(t, db, "sub-release", 100)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)
		require.NotNil(t, reserveReceipt)

		var sub UserSubscription
		require.NoError(t, db.First(&sub, input.SubscriptionID).Error)
		assert.Equal(t, int64(300), sub.AmountUsed) // 200 + 100

		releaseInput := TaskQuotaReleaseInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ReasonCode:               "task_failed",
			BillingContext:           input.BillingContext,
		}
		receipt, err := ReleaseTaskQuotaReservation(db, releaseInput)
		require.NoError(t, err)
		require.NotNil(t, receipt)

		require.NoError(t, db.First(&sub, input.SubscriptionID).Error)
		assert.Equal(t, int64(200), sub.AmountUsed) // restored to 200
		assert.Equal(t, int64(2), sub.QuotaVersion)
	})

	t.Run("zero_quota_release", func(t *testing.T) {
		input := newSettleTestFixture(t, db, "zero-release", 0)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		releaseInput := TaskQuotaReleaseInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ReasonCode:               "task_canceled",
			BillingContext:           input.BillingContext,
		}
		receipt, err := ReleaseTaskQuotaReservation(db, releaseInput)
		require.NoError(t, err)
		require.NotNil(t, receipt)
		assert.Equal(t, int64(0), receipt.Quota)
	})

	t.Run("idempotent_replay_and_conflict", func(t *testing.T) {
		input := newSettleTestFixture(t, db, "replay-release", 50)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		releaseInput := TaskQuotaReleaseInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ReasonCode:               "task_failed",
			BillingContext:           input.BillingContext,
		}
		firstReceipt, err := ReleaseTaskQuotaReservation(db, releaseInput)
		require.NoError(t, err)

		// Replay exact same input
		secondReceipt, err := ReleaseTaskQuotaReservation(db, releaseInput)
		require.NoError(t, err)
		assert.Equal(t, firstReceipt.ID, secondReceipt.ID)
		assert.Equal(t, firstReceipt.RequestFingerprint, secondReceipt.RequestFingerprint)

		// Different reason code -> fingerprint mismatch -> conflict error
		conflictingInput := releaseInput
		conflictingInput.ReasonCode = "different_reason"
		_, err = ReleaseTaskQuotaReservation(db, conflictingInput)
		require.ErrorIs(t, err, ErrTaskQuotaReservationConflict)
	})
}

// Matrix 2: T3 Terminal Settlement (终态结算多退少补)
func TestTaskQuotaSettlement_Matrix2(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)

	t.Run("surplus_settlement_refunds_delta", func(t *testing.T) {
		// Reserved 100, actual consumed 60 -> surplus refund 40
		input := newSettleTestFixture(t, db, "surplus", 100)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		settleInput := TaskQuotaSettlementInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ActualQuota:              60,
			ReasonCode:               "task_succeeded",
			BillingContext:           input.BillingContext,
		}
		receipt, err := SettleTaskQuotaReservation(db, settleInput)
		require.NoError(t, err)
		require.NotNil(t, receipt)

		// User had 900 after reserve -> now 900 + 40 = 940
		// Token had remain 400 -> now 400 + 40 = 440; used 100 -> now 100 - 40 = 60
		assertTaskQuotaWallet(t, db, input, 940, 440, 60, 2)
		assert.Equal(t, string(TaskBillingEventTypeTerminalSettlement), receipt.MutationType)
		assert.Equal(t, int64(60), receipt.Quota)

		// Verify event has quota_delta = +40
		event, err := GetTaskBillingEventByEventID(db, receipt.BillingEventID)
		require.NoError(t, err)
		assert.Equal(t, int64(40), event.QuotaDelta)
	})

	t.Run("deficit_settlement_charges_extra", func(t *testing.T) {
		// Reserved 100, actual consumed 150 -> deficit charge extra 50
		input := newSettleTestFixture(t, db, "deficit", 100)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		settleInput := TaskQuotaSettlementInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ActualQuota:              150,
			ReasonCode:               "task_succeeded",
			BillingContext:           input.BillingContext,
		}
		receipt, err := SettleTaskQuotaReservation(db, settleInput)
		require.NoError(t, err)
		require.NotNil(t, receipt)

		// User had 900 after reserve -> now 900 - 50 = 850
		// Token had remain 400 -> now 400 - 50 = 350; used 100 -> now 100 + 50 = 150
		assertTaskQuotaWallet(t, db, input, 850, 350, 150, 2)

		event, err := GetTaskBillingEventByEventID(db, receipt.BillingEventID)
		require.NoError(t, err)
		assert.Equal(t, int64(-50), event.QuotaDelta)
	})

	t.Run("exact_settlement_zero_delta", func(t *testing.T) {
		// Reserved 100, actual consumed 100 -> delta 0
		input := newSettleTestFixture(t, db, "exact", 100)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		settleInput := TaskQuotaSettlementInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ActualQuota:              100,
			ReasonCode:               "task_succeeded",
			BillingContext:           input.BillingContext,
		}
		receipt, err := SettleTaskQuotaReservation(db, settleInput)
		require.NoError(t, err)
		require.NotNil(t, receipt)

		// Balances unchanged from reserve state: user 900, remain 400, used 100, version 1
		assertTaskQuotaWallet(t, db, input, 900, 400, 100, 1)

		event, err := GetTaskBillingEventByEventID(db, receipt.BillingEventID)
		require.NoError(t, err)
		assert.Equal(t, int64(0), event.QuotaDelta)
	})

	t.Run("subscription_settlement", func(t *testing.T) {
		// Subscription: reserved 100, actual 70 -> surplus 30
		input := newSettleSubscriptionFixture(t, db, "sub-settle", 100)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		settleInput := TaskQuotaSettlementInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ActualQuota:              70,
			ReasonCode:               "task_succeeded",
			BillingContext:           input.BillingContext,
		}
		receipt, err := SettleTaskQuotaReservation(db, settleInput)
		require.NoError(t, err)
		require.NotNil(t, receipt)

		var sub UserSubscription
		require.NoError(t, db.First(&sub, input.SubscriptionID).Error)
		// initial 200 + 100 reserved = 300, now surplus 30 refunded -> 270 (200 + 70 actual)
		assert.Equal(t, int64(270), sub.AmountUsed)
	})

	t.Run("subscription_deficit_settlement", func(t *testing.T) {
		// Subscription: reserved 100, actual 150 -> deficit 50 charged
		input := newSettleSubscriptionFixture(t, db, "sub-deficit", 100)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		settleInput := TaskQuotaSettlementInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ActualQuota:              150,
			ReasonCode:               "task_succeeded",
			BillingContext:           input.BillingContext,
		}
		receipt, err := SettleTaskQuotaReservation(db, settleInput)
		require.NoError(t, err)
		require.NotNil(t, receipt)

		var sub UserSubscription
		require.NoError(t, db.First(&sub, input.SubscriptionID).Error)
		// initial 200 + 100 reserved = 300, now deficit 50 charged -> 350 (200 + 150 actual)
		assert.Equal(t, int64(350), sub.AmountUsed)
	})

	t.Run("settlement_idempotency_and_conflict", func(t *testing.T) {
		input := newSettleTestFixture(t, db, "settle-replay", 80)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		settleInput := TaskQuotaSettlementInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ActualQuota:              50,
			ReasonCode:               "task_succeeded",
			BillingContext:           input.BillingContext,
		}
		firstReceipt, err := SettleTaskQuotaReservation(db, settleInput)
		require.NoError(t, err)

		// Replay exact
		secondReceipt, err := SettleTaskQuotaReservation(db, settleInput)
		require.NoError(t, err)
		assert.Equal(t, firstReceipt.ID, secondReceipt.ID)

		// Conflict with different actual quota
		conflicting := settleInput
		conflicting.ActualQuota = 60
		_, err = SettleTaskQuotaReservation(db, conflicting)
		require.ErrorIs(t, err, ErrTaskQuotaReservationConflict)
	})
}

// Matrix 3: Mutual Exclusion (结算与退款互斥)
func TestTaskQuota_MutualExclusion_Matrix3(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)

	t.Run("settle_after_refund_is_forbidden", func(t *testing.T) {
		input := newSettleTestFixture(t, db, "mutex-refund-first", 100)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		// Refund first
		releaseInput := TaskQuotaReleaseInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ReasonCode:               "task_failed",
			BillingContext:           input.BillingContext,
		}
		_, err = ReleaseTaskQuotaReservation(db, releaseInput)
		require.NoError(t, err)

		// Attempt to Settle -> must fail
		settleInput := TaskQuotaSettlementInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ActualQuota:              50,
			ReasonCode:               "task_succeeded",
			BillingContext:           input.BillingContext,
		}
		_, err = SettleTaskQuotaReservation(db, settleInput)
		require.ErrorIs(t, err, ErrTaskQuotaAlreadyRefunded)
	})

	t.Run("refund_after_settle_is_forbidden", func(t *testing.T) {
		input := newSettleTestFixture(t, db, "mutex-settle-first", 100)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		// Settle first
		settleInput := TaskQuotaSettlementInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ActualQuota:              80,
			ReasonCode:               "task_succeeded",
			BillingContext:           input.BillingContext,
		}
		_, err = SettleTaskQuotaReservation(db, settleInput)
		require.NoError(t, err)

		// Attempt to Refund -> must fail
		releaseInput := TaskQuotaReleaseInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ReasonCode:               "task_failed",
			BillingContext:           input.BillingContext,
		}
		_, err = ReleaseTaskQuotaReservation(db, releaseInput)
		require.ErrorIs(t, err, ErrTaskQuotaAlreadySettled)
	})
}

// Matrix 4: Math Boundaries & Safety
func TestTaskQuota_MathBoundaries_Matrix4(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)

	t.Run("negative_actual_quota_rejected", func(t *testing.T) {
		input := newSettleTestFixture(t, db, "neg-actual", 100)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		settleInput := TaskQuotaSettlementInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ActualQuota:              -10, // negative!
			ReasonCode:               "task_succeeded",
			BillingContext:           input.BillingContext,
		}
		_, err = SettleTaskQuotaReservation(db, settleInput)
		require.ErrorIs(t, err, ErrTaskQuotaSettlementInvalidInput)
	})

	t.Run("overdraft_allows_negative_user_quota_safely", func(t *testing.T) {
		// User starts with 1000, reserve 900 -> 100 remaining
		// Actual quota 1000 -> extra deficit charge 100.
		// What if actual quota 1200? Extra deficit 300 -> user quota goes to 100 - 300 = -200 (overdraft allowed).
		input := newSettleTestFixture(t, db, "overdraft", 900)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		settleInput := TaskQuotaSettlementInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ActualQuota:              1200,
			ReasonCode:               "task_succeeded",
			BillingContext:           input.BillingContext,
		}
		receipt, err := SettleTaskQuotaReservation(db, settleInput)
		require.NoError(t, err)
		require.NotNil(t, receipt)

		var user User
		require.NoError(t, db.First(&user, input.UserID).Error)
		assert.Equal(t, -200, user.Quota) // safe negative balance without overflow
	})
}

// Matrix 5: Concurrency & Racing
func TestTaskQuota_Concurrency_Matrix5(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)

	t.Run("concurrent_settle_same_operation", func(t *testing.T) {
		input := newSettleTestFixture(t, db, "concurrent-settle", 100)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		settleInput := TaskQuotaSettlementInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ActualQuota:              80,
			ReasonCode:               "task_succeeded",
			BillingContext:           input.BillingContext,
		}

		concurrency := 6
		results := make(chan error, concurrency)
		var wg sync.WaitGroup

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := SettleTaskQuotaReservation(db, settleInput)
				results <- err
			}()
		}
		wg.Wait()
		close(results)

		for err := range results {
			// All concurrent identical requests must succeed or be idempotent
			require.NoError(t, err)
		}

		// Balances must be exactly settled once
		assertTaskQuotaWallet(t, db, input, 920, 420, 80, 2)
	})

	t.Run("concurrent_settle_vs_refund_race", func(t *testing.T) {
		input := newSettleTestFixture(t, db, "race-settle-refund", 100)
		reserveReceipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)

		settleInput := TaskQuotaSettlementInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ActualQuota:              80,
			ReasonCode:               "task_succeeded",
			BillingContext:           input.BillingContext,
		}

		releaseInput := TaskQuotaReleaseInput{
			OperationID:              input.OperationID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ReasonCode:               "task_failed",
			BillingContext:           input.BillingContext,
		}

		var wg sync.WaitGroup
		var settleErr, refundErr error

		wg.Add(2)
		go func() {
			defer wg.Done()
			_, settleErr = SettleTaskQuotaReservation(db, settleInput)
		}()
		go func() {
			defer wg.Done()
			_, refundErr = ReleaseTaskQuotaReservation(db, releaseInput)
		}()
		wg.Wait()

		// Exactly one must win, and the other must be rejected due to mutual exclusion or CAS loss
		if settleErr == nil {
			require.Error(t, refundErr)
			assert.True(t, errors.Is(refundErr, ErrTaskQuotaAlreadySettled) || errors.Is(refundErr, ErrTaskQuotaReservationCASLost))
		} else if refundErr == nil {
			require.Error(t, settleErr)
			assert.True(t, errors.Is(settleErr, ErrTaskQuotaAlreadyRefunded) || errors.Is(settleErr, ErrTaskQuotaReservationCASLost))
		} else {
			t.Fatalf("at least one of settle or refund must succeed, got settleErr: %v, refundErr: %v", settleErr, refundErr)
		}
	})
}
