package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Every scenario owns different subjects and an intent. This fixture is shared
// by SQLite and the disposable minimum-version MySQL/PostgreSQL CI databases.
func newTaskQuotaReservationFixture(t *testing.T, db *gorm.DB, label string) TaskQuotaReservationInput {
	t.Helper()
	digest := sha256.Sum256([]byte(t.Name() + ":" + label))
	name := fmt.Sprintf("qr-%x", digest[:8])
	user := User{Username: name, AffCode: name, Password: "synthetic-fixture", Status: common.UserStatusEnabled, Quota: 1000}
	require.NoError(t, db.Create(&user).Error)
	token := Token{UserId: user.Id, Key: name, Status: common.TokenStatusEnabled, RemainQuota: 500, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	operation := newB2SubmissionOperation(t, token.Id, "POST", TaskSubmissionOperationKindVideoCreate, name, "{}")
	operation.UserID = user.Id
	intent, err := CreateOrLoadTaskSubmissionIntent(db, operation, &TaskSubmissionAttempt{
		AttemptNo: 1, ChannelID: 61, Provider: "fixture", RequestClass: "video",
	})
	require.NoError(t, err)
	return TaskQuotaReservationInput{
		OperationID: intent.Operation.ID, UserID: user.Id, TokenID: token.Id,
		ChannelID: 61, ExpectedOperationVersion: intent.Operation.LockVersion,
		Quota: 100, BillingSource: "wallet",
		BillingContext: TaskBillingContext{
			Version: TaskBillingContextVersion, Complete: true, ModelPrice: 1,
			GroupRatio: 1, OriginModelName: "quota-reserve-fixture", PerCallBilling: true,
		},
	}
}

func assertTaskQuotaWallet(t *testing.T, db *gorm.DB, input TaskQuotaReservationInput, quota, remaining, used int, version int64) {
	t.Helper()
	var user User
	var token Token
	require.NoError(t, db.First(&user, input.UserID).Error)
	require.NoError(t, db.First(&token, input.TokenID).Error)
	assert.Equal(t, quota, user.Quota)
	assert.Zero(t, user.UsedQuota, "a reservation is not terminal user usage")
	assert.Equal(t, remaining, token.RemainQuota)
	assert.Equal(t, used, token.UsedQuota)
	assert.Equal(t, version, user.QuotaVersion)
	assert.Equal(t, version, token.QuotaVersion)
}

func TestTaskQuotaReservationSQLite(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	runTaskQuotaReservationContract(t, db)
}

func TestTaskQuotaReservationConfiguredDatabases(t *testing.T) {
	if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
		t.Skip("disposable B2 database tests require MYAPI_B2_DATABASE_TESTS=1")
	}
	for _, engine := range []struct{ name, env string }{
		{"mysql", "MYAPI_B2_MYSQL_DSN"},
		{"postgres", "MYAPI_B2_POSTGRES_DSN"},
	} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			require.NotEmpty(t, dsn, "%s must be configured when MYAPI_B2_DATABASE_TESTS=1", engine.env)
			dialector, err := b2SubmissionDatabaseDialector(engine.name, dsn)
			require.NoError(t, err)
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			require.NoError(t, registerTaskRecoveryGormGuards(db))
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Run("quota-reservation", func(t *testing.T) { runTaskQuotaReservationContract(t, db) })
		})
	}
}

func runTaskQuotaReservationContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	t.Run("wallet-replay-conflict-and-owner-query", func(t *testing.T) {
		input := newTaskQuotaReservationFixture(t, db, "wallet")
		receipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)
		require.NotNil(t, receipt)
		assertTaskQuotaWallet(t, db, input, 900, 400, 100, 1)
		var operation TaskSubmissionOperation
		require.NoError(t, db.First(&operation, input.OperationID).Error)
		assert.Equal(t, TaskSubmissionOperationStatusReserved, operation.Status)
		var event TaskBillingEvent
		require.NoError(t, db.Where("operation_id = ?", input.OperationID).First(&event).Error)
		assert.Equal(t, TaskBillingEventTypeReserve, event.EventType)
		assert.Equal(t, TaskBillingEventStateApplied, event.State)
		assert.Equal(t, int64(-100), event.QuotaDelta)
		replay, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)
		assert.Equal(t, receipt, replay)
		assertTaskQuotaWallet(t, db, input, 900, 400, 100, 1)
		changed := input
		changed.Quota++
		_, err = ReserveTaskQuota(db, changed)
		require.Error(t, err)
		changed = input
		changed.BillingContext.GroupRatio = 2
		_, err = ReserveTaskQuota(db, changed)
		require.Error(t, err, "equal amounts do not authorize a different price snapshot")
		found, err := FindTaskQuotaReservation(db.Session(&gorm.Session{NewDB: true}), input.OperationID, input.UserID, input.TokenID)
		require.NoError(t, err)
		assert.Equal(t, receipt, found)
		_, err = FindTaskQuotaReservation(db, input.OperationID, input.UserID+1, input.TokenID)
		require.Error(t, err)
		assertTaskQuotaWallet(t, db, input, 900, 400, 100, 1)
		var count int64
		require.NoError(t, db.Model(&TaskBillingEvent{}).Where("operation_id = ?", input.OperationID).Count(&count).Error)
		assert.Equal(t, int64(1), count)
	})

	t.Run("replay-after-token-disabled", func(t *testing.T) {
		input := newTaskQuotaReservationFixture(t, db, "disabled-replay")
		receipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)
		require.NoError(t, db.Model(&Token{}).Where("id = ?", input.TokenID).Update("status", common.TokenStatusDisabled).Error)
		replay, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)
		assert.Equal(t, receipt, replay, "replay reads an obligation; it is not a new authorization")
	})

	t.Run("subscription-and-token-atomic-reserve", func(t *testing.T) {
		input := newTaskQuotaReservationFixture(t, db, "subscription")
		now, err := taskRecoveryDBTimestamp(db)
		require.NoError(t, err)
		subscription := UserSubscription{UserId: input.UserID, Status: "active", AmountTotal: 300, AmountUsed: 20, StartTime: now - 10, EndTime: now + 3600}
		require.NoError(t, db.Create(&subscription).Error)
		input.BillingSource, input.SubscriptionID = "subscription", subscription.Id
		receipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)
		var loaded UserSubscription
		require.NoError(t, db.First(&loaded, subscription.Id).Error)
		assert.Equal(t, int64(120), loaded.AmountUsed)
		assert.Equal(t, int64(1), loaded.QuotaVersion)
		var user User
		var token Token
		require.NoError(t, db.First(&user, input.UserID).Error)
		require.NoError(t, db.First(&token, input.TokenID).Error)
		assert.Equal(t, 1000, user.Quota)
		assert.Zero(t, user.QuotaVersion)
		assert.Equal(t, 400, token.RemainQuota)
		assert.Equal(t, 100, token.UsedQuota)
		require.NoError(t, db.Model(&UserSubscription{}).Where("id = ?", subscription.Id).Update("status", "cancelled").Error)
		replay, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)
		assert.Equal(t, receipt, replay)
	})

	t.Run("unlimited-token-retains-bounded-accounting", func(t *testing.T) {
		input := newTaskQuotaReservationFixture(t, db, "unlimited-token")
		require.NoError(t, db.Model(&Token{}).Where("id = ?", input.TokenID).Updates(map[string]interface{}{"unlimited_quota": true, "remain_quota": 0}).Error)
		_, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)
		assertTaskQuotaWallet(t, db, input, 900, -100, 100, 1)
	})

	for _, scenario := range []struct {
		name   string
		change func(*TaskQuotaReservationInput)
	}{
		{"negative-reservation", func(input *TaskQuotaReservationInput) { input.Quota = -1 }},
		{"oversized-reservation", func(input *TaskQuotaReservationInput) { input.Quota = int64(common.MaxQuota) + 1 }},
		{"wrong-owner", func(input *TaskQuotaReservationInput) { input.UserID++ }},
		{"wrong-channel", func(input *TaskQuotaReservationInput) { input.ChannelID++ }},
		{"stale-operation-version", func(input *TaskQuotaReservationInput) { input.ExpectedOperationVersion++ }},
		{"incomplete-price", func(input *TaskQuotaReservationInput) { input.BillingContext.Complete = false }},
		{"nonfinite-price", func(input *TaskQuotaReservationInput) { input.BillingContext.GroupRatio = math.NaN() }},
		{"invalid-ratio", func(input *TaskQuotaReservationInput) {
			input.BillingContext.OtherRatios = map[string]float64{"duration": -1}
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			input := newTaskQuotaReservationFixture(t, db, scenario.name)
			changed := input
			scenario.change(&changed)
			_, err := ReserveTaskQuota(db, changed)
			require.Error(t, err)
			assertTaskQuotaWallet(t, db, input, 1000, 500, 0, 0)
			var count int64
			require.NoError(t, db.Model(&TaskBillingEvent{}).Where("operation_id = ?", input.OperationID).Count(&count).Error)
			assert.Zero(t, count)
		})
	}

	for _, scenario := range []struct {
		name         string
		tokenUpdates map[string]interface{}
	}{
		{"insufficient-token", map[string]interface{}{"remain_quota": 99}},
		{"overflow-token-used", map[string]interface{}{"used_quota": common.MaxQuota}},
		{"overflow-version", map[string]interface{}{"quota_version": int64(math.MaxInt64)}},
		{"disabled-token", map[string]interface{}{"status": common.TokenStatusDisabled}},
		{"expired-token", map[string]interface{}{"expired_time": int64(1)}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			input := newTaskQuotaReservationFixture(t, db, scenario.name)
			require.NoError(t, db.Model(&Token{}).Where("id = ?", input.TokenID).Updates(scenario.tokenUpdates).Error)
			var before Token
			require.NoError(t, db.First(&before, input.TokenID).Error)
			_, err := ReserveTaskQuota(db, input)
			require.Error(t, err)
			var user User
			var after Token
			require.NoError(t, db.First(&user, input.UserID).Error)
			require.NoError(t, db.First(&after, input.TokenID).Error)
			assert.Equal(t, 1000, user.Quota)
			assert.Equal(t, before, after)
		})
	}

	t.Run("outer-transaction-rollback", func(t *testing.T) {
		input := newTaskQuotaReservationFixture(t, db, "outer-rollback")
		injected := errors.New("rollback caller transaction")
		err := db.Transaction(func(tx *gorm.DB) error {
			_, err := ReserveTaskQuota(tx, input)
			if err != nil {
				return err
			}
			return injected
		})
		require.ErrorIs(t, err, injected)
		assertTaskQuotaWallet(t, db, input, 1000, 500, 0, 0)
		_, err = FindTaskQuotaReservation(db, input.OperationID, input.UserID, input.TokenID)
		require.Error(t, err)
		_, err = ReserveTaskQuota(db, input)
		require.NoError(t, err, "a rolled-back reservation can be attempted without hidden partial state")
	})

	t.Run("caught-receipt-error-rolls-back-savepoint", func(t *testing.T) {
		input := newTaskQuotaReservationFixture(t, db, "savepoint")
		injected := errors.New("injected receipt persistence failure")
		callback := "test:quota-receipt-failure"
		require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
			if tx.Statement.Table == "quota_mutation_receipts" {
				tx.AddError(injected)
			}
		}))
		t.Cleanup(func() { require.NoError(t, db.Callback().Create().Remove(callback)) })
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			_, err := ReserveTaskQuota(tx, input)
			require.ErrorIs(t, err, injected)
			return nil // A caller catching the error must not commit half a debit.
		}))
		assertTaskQuotaWallet(t, db, input, 1000, 500, 0, 0)
		var operation TaskSubmissionOperation
		require.NoError(t, db.First(&operation, input.OperationID).Error)
		assert.Equal(t, TaskSubmissionOperationStatusPrepared, operation.Status)
		var count int64
		require.NoError(t, db.Model(&TaskBillingEvent{}).Where("operation_id = ?", input.OperationID).Count(&count).Error)
		assert.Zero(t, count)
	})

	t.Run("receipt-immutability-and-guards", func(t *testing.T) {
		input := newTaskQuotaReservationFixture(t, db, "immutability")
		receipt, err := ReserveTaskQuota(db, input)
		require.NoError(t, err)
		require.NotNil(t, receipt)

		// Direct creation without private marker must be rejected.
		unauthorized := *receipt
		unauthorized.ID = 0
		unauthorized.CreatedAt = 0
		err = db.Create(&unauthorized).Error
		require.ErrorIs(t, err, ErrQuotaMutationReceiptImmutable)

		// Direct GORM Update must be blocked by write guard and BeforeUpdate.
		err = db.Model(&QuotaMutationReceipt{}).Where("id = ?", receipt.ID).Update("quota", 0).Error
		require.ErrorIs(t, err, ErrQuotaMutationReceiptImmutable)

		// Controlled-write handle must ALSO be rejected; receipts have no valid update path.
		err = taskRecoveryControlledWrite(db).Table("quota_mutation_receipts").Where("id = ?", receipt.ID).Update("quota", 0).Error
		require.ErrorIs(t, err, ErrQuotaMutationReceiptImmutable)

		// Direct GORM Delete must be rejected.
		err = db.Delete(receipt).Error
		require.ErrorIs(t, err, ErrQuotaMutationReceiptImmutable)
	})

	t.Run("zero-quota-reservation-and-overdrawn-accounts", func(t *testing.T) {
		// Valid zero quota reservation
		zeroInput := newTaskQuotaReservationFixture(t, db, "zero-quota")
		zeroInput.Quota = 0
		receipt, err := ReserveTaskQuota(db, zeroInput)
		require.NoError(t, err)
		require.NotNil(t, receipt)
		assert.Equal(t, int64(0), receipt.Quota)
		assertTaskQuotaWallet(t, db, zeroInput, 1000, 500, 0, 0) // QuotaVersion unchanged

		replay, err := ReserveTaskQuota(db, zeroInput)
		require.NoError(t, err)
		assert.Equal(t, receipt, replay)

		// Overdrawn wallet (negative quota) must be rejected even with Quota == 0
		overdrawnWalletInput := newTaskQuotaReservationFixture(t, db, "overdrawn-wallet")
		overdrawnWalletInput.Quota = 0
		require.NoError(t, db.Model(&User{}).Where("id = ?", overdrawnWalletInput.UserID).Update("quota", -10).Error)
		_, err = ReserveTaskQuota(db, overdrawnWalletInput)
		require.ErrorIs(t, err, ErrTaskQuotaReservationInsufficientQuota)

		// Overdrawn token (negative remain_quota) must be rejected even with Quota == 0
		overdrawnTokenInput := newTaskQuotaReservationFixture(t, db, "overdrawn-token")
		overdrawnTokenInput.Quota = 0
		require.NoError(t, db.Model(&Token{}).Where("id = ?", overdrawnTokenInput.TokenID).Updates(map[string]interface{}{
			"remain_quota": -5, "unlimited_quota": false,
		}).Error)
		_, err = ReserveTaskQuota(db, overdrawnTokenInput)
		require.ErrorIs(t, err, ErrTaskQuotaReservationInsufficientQuota)
	})

	t.Run("amount-and-version-boundaries", func(t *testing.T) {
		// Exact balance depletion
		exactInput := newTaskQuotaReservationFixture(t, db, "exact-wallet")
		require.NoError(t, db.Model(&User{}).Where("id = ?", exactInput.UserID).Update("quota", 100).Error)
		receipt, err := ReserveTaskQuota(db, exactInput)
		require.NoError(t, err)
		require.NotNil(t, receipt)
		assertTaskQuotaWallet(t, db, exactInput, 0, 400, 100, 1)

		// Insufficient by 1
		insufficientInput := newTaskQuotaReservationFixture(t, db, "insufficient-by-1")
		require.NoError(t, db.Model(&User{}).Where("id = ?", insufficientInput.UserID).Update("quota", 99).Error)
		_, err = ReserveTaskQuota(db, insufficientInput)
		require.ErrorIs(t, err, ErrTaskQuotaReservationInsufficientQuota)

		// Subscription exact total
		subInput := newTaskQuotaReservationFixture(t, db, "exact-subscription")
		now, err := taskRecoveryDBTimestamp(db)
		require.NoError(t, err)
		sub := UserSubscription{
			UserId: subInput.UserID, Status: "active",
			AmountTotal: 200, AmountUsed: 100,
			StartTime: now - 10, EndTime: now + 3600,
		}
		require.NoError(t, db.Create(&sub).Error)
		subInput.BillingSource, subInput.SubscriptionID = "subscription", sub.Id
		receipt, err = ReserveTaskQuota(db, subInput)
		require.NoError(t, err)
		require.NotNil(t, receipt)
		var loadedSub UserSubscription
		require.NoError(t, db.First(&loadedSub, sub.Id).Error)
		assert.Equal(t, int64(200), loadedSub.AmountUsed)

		// Subscription insufficient by 1
		subInput2 := newTaskQuotaReservationFixture(t, db, "insufficient-sub-by-1")
		sub2 := UserSubscription{
			UserId: subInput2.UserID, Status: "active",
			AmountTotal: 200, AmountUsed: 101,
			StartTime: now - 10, EndTime: now + 3600,
		}
		require.NoError(t, db.Create(&sub2).Error)
		subInput2.BillingSource, subInput2.SubscriptionID = "subscription", sub2.Id
		_, err = ReserveTaskQuota(db, subInput2)
		require.ErrorIs(t, err, ErrTaskQuotaReservationInsufficientQuota)

		// Unlimited subscription (AmountTotal == 0)
		unlimitedSubInput := newTaskQuotaReservationFixture(t, db, "unlimited-subscription")
		unlimitedSub := UserSubscription{
			UserId: unlimitedSubInput.UserID, Status: "active",
			AmountTotal: 0, AmountUsed: 500,
			StartTime: now - 10, EndTime: now + 3600,
		}
		require.NoError(t, db.Create(&unlimitedSub).Error)
		unlimitedSubInput.BillingSource, unlimitedSubInput.SubscriptionID = "subscription", unlimitedSub.Id
		receipt, err = ReserveTaskQuota(db, unlimitedSubInput)
		require.NoError(t, err)
		require.NotNil(t, receipt)
		var loadedUnlimitedSub UserSubscription
		require.NoError(t, db.First(&loadedUnlimitedSub, unlimitedSub.Id).Error)
		assert.Equal(t, int64(600), loadedUnlimitedSub.AmountUsed)
	})

	t.Run("concurrent-same-operation-replay", func(t *testing.T) {
		input := newTaskQuotaReservationFixture(t, db, "concurrent-replay")
		start := make(chan struct{})
		type res struct {
			receipt *QuotaMutationReceipt
			err     error
		}
		results := make(chan res, 2)
		for range 2 {
			go func() {
				<-start
				receipt, err := ReserveTaskQuota(db.Session(&gorm.Session{NewDB: true}), input)
				results <- res{receipt: receipt, err: err}
			}()
		}
		close(start)
		r1 := <-results
		r2 := <-results
		require.NoError(t, r1.err)
		require.NoError(t, r2.err)
		require.NotNil(t, r1.receipt)
		require.NotNil(t, r2.receipt)
		assert.Equal(t, r1.receipt.ID, r2.receipt.ID)
		assert.Equal(t, r1.receipt.RequestFingerprint, r2.receipt.RequestFingerprint)
		assertTaskQuotaWallet(t, db, input, 900, 400, 100, 1)

		var eventCount int64
		require.NoError(t, db.Model(&TaskBillingEvent{}).Where("operation_id = ?", input.OperationID).Count(&eventCount).Error)
		assert.Equal(t, int64(1), eventCount)
	})

	t.Run("concurrent-distinct-operations-quota-contention", func(t *testing.T) {
		input1 := newTaskQuotaReservationFixture(t, db, "contention-1")
		// User has 150 quota, Token has 150 remain quota. Each operation requests 100 quota.
		require.NoError(t, db.Model(&User{}).Where("id = ?", input1.UserID).Update("quota", 150).Error)
		require.NoError(t, db.Model(&Token{}).Where("id = ?", input1.TokenID).Update("remain_quota", 150).Error)

		input2 := newSecondOperationForExistingOwner(t, db, input1, "contention-2")

		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			_, err := ReserveTaskQuota(db.Session(&gorm.Session{NewDB: true}), input1)
			results <- err
		}()
		go func() {
			<-start
			_, err := ReserveTaskQuota(db.Session(&gorm.Session{NewDB: true}), input2)
			results <- err
		}()
		close(start)
		err1 := <-results
		err2 := <-results

		// Exactly one must succeed, the other must fail.
		var successCount, failCount int
		for _, err := range []error{err1, err2} {
			if err == nil {
				successCount++
			} else {
				failCount++
				assert.True(t, errors.Is(err, ErrTaskQuotaReservationInsufficientQuota) || errors.Is(err, ErrTaskQuotaReservationCASLost), "unexpected failure error: %v", err)
			}
		}
		assert.Equal(t, 1, successCount, "exactly one reservation must succeed")
		assert.Equal(t, 1, failCount, "competing reservation must fail")

		var user User
		require.NoError(t, db.First(&user, input1.UserID).Error)
		assert.Equal(t, 50, user.Quota, "wallet quota must be exactly 50 and never negative")
		assert.Equal(t, int64(1), user.QuotaVersion)

		var token Token
		require.NoError(t, db.First(&token, input1.TokenID).Error)
		assert.Equal(t, 50, token.RemainQuota)
		assert.Equal(t, 100, token.UsedQuota)
		assert.Equal(t, int64(1), token.QuotaVersion)
	})
}

func newSecondOperationForExistingOwner(t *testing.T, db *gorm.DB, base TaskQuotaReservationInput, label string) TaskQuotaReservationInput {
	t.Helper()
	digest := sha256.Sum256([]byte(t.Name() + ":" + label))
	key := fmt.Sprintf("qr2-%x", digest[:8])
	operation := newB2SubmissionOperation(t, base.TokenID, "POST", TaskSubmissionOperationKindVideoCreate, key, "{}")
	operation.UserID = base.UserID
	intent, err := CreateOrLoadTaskSubmissionIntent(db, operation, &TaskSubmissionAttempt{
		AttemptNo: 1, ChannelID: 61, Provider: "fixture", RequestClass: "video",
	})
	require.NoError(t, err)
	second := base
	second.OperationID = intent.Operation.ID
	second.ExpectedOperationVersion = intent.Operation.LockVersion
	return second
}
