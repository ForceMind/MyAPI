package model

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestUserForQuotaMutation(t *testing.T, id int, initialQuota int) *User {
	t.Helper()
	user := &User{
		Id:           id,
		Username:     fmt.Sprintf("test-mutation-user-%d", id),
		Password:     "hashed_pass",
		Role:         common.RoleCommonUser,
		Status:       common.UserStatusEnabled,
		Group:        "default",
		Quota:        initialQuota,
		QuotaVersion: 0,
		AuthVersion:  1,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func TestUserQuotaMutationBasic(t *testing.T) {
	truncateTables(t)
	user := createTestUserForQuotaMutation(t, 8001, 1000)

	// 1. Credit mutation (+500)
	input1 := UserQuotaMutationInput{
		UserID:           user.Id,
		Delta:            500,
		MutationType:     "recharge",
		BusinessEventKey: "topup:order:1001",
		ReasonCode:       "stripe_payment",
		OperatorUserID:   0,
		Metadata:         map[string]interface{}{"payment_id": "pi_123"},
	}

	receipt1, err := MutateUserQuota(DB, input1)
	require.NoError(t, err)
	require.NotNil(t, receipt1)
	assert.Equal(t, int64(1), receipt1.ID)
	assert.Equal(t, "recharge", receipt1.MutationType)
	assert.Equal(t, "topup:order:1001", receipt1.BusinessEventKey)
	assert.Equal(t, int64(500), receipt1.Delta)
	assert.Equal(t, 1000, receipt1.QuotaBefore)
	assert.Equal(t, 1500, receipt1.QuotaAfter)
	assert.Equal(t, int64(0), receipt1.QuotaVersionBefore)
	assert.Equal(t, int64(1), receipt1.QuotaVersionAfter)
	assert.NotEmpty(t, receipt1.RequestFingerprint)
	assert.NotZero(t, receipt1.CreatedAt)

	// Verify DB state
	var updatedUser User
	require.NoError(t, DB.Where("id = ?", user.Id).First(&updatedUser).Error)
	assert.Equal(t, 1500, updatedUser.Quota)
	assert.Equal(t, int64(1), updatedUser.QuotaVersion)

	// 2. Debit mutation (-300)
	input2 := UserQuotaMutationInput{
		UserID:           user.Id,
		Delta:            -300,
		MutationType:     "subscription_purchase",
		BusinessEventKey: "sub:order:2001",
		ReasonCode:       "plan_monthly",
		OperatorUserID:   user.Id,
	}

	receipt2, err := MutateUserQuota(DB, input2)
	require.NoError(t, err)
	require.NotNil(t, receipt2)
	assert.Equal(t, int64(2), receipt2.ID)
	assert.Equal(t, "subscription_purchase", receipt2.MutationType)
	assert.Equal(t, int64(-300), receipt2.Delta)
	assert.Equal(t, 1500, receipt2.QuotaBefore)
	assert.Equal(t, 1200, receipt2.QuotaAfter)
	assert.Equal(t, int64(1), receipt2.QuotaVersionBefore)
	assert.Equal(t, int64(2), receipt2.QuotaVersionAfter)

	// Verify DB state
	require.NoError(t, DB.Where("id = ?", user.Id).First(&updatedUser).Error)
	assert.Equal(t, 1200, updatedUser.Quota)
	assert.Equal(t, int64(2), updatedUser.QuotaVersion)
}

func TestUserQuotaMutationIdempotencyAndConflict(t *testing.T) {
	truncateTables(t)
	user := createTestUserForQuotaMutation(t, 8002, 500)

	input := UserQuotaMutationInput{
		UserID:           user.Id,
		Delta:            200,
		MutationType:     "redemption",
		BusinessEventKey: "redeem:code:ABC123XYZ",
		ReasonCode:       "gift_card",
		OperatorUserID:   user.Id,
	}

	// First execution
	receipt1, err := MutateUserQuota(DB, input)
	require.NoError(t, err)
	require.NotNil(t, receipt1)
	assert.Equal(t, 700, receipt1.QuotaAfter)
	assert.Equal(t, int64(1), receipt1.QuotaVersionAfter)

	// Exact replay: same BusinessEventKey + identical payload
	receipt2, err := MutateUserQuota(DB, input)
	require.NoError(t, err, "exact replay must succeed")
	require.NotNil(t, receipt2)
	assert.Equal(t, receipt1.ID, receipt2.ID, "exact replay must return original receipt")
	assert.Equal(t, receipt1.QuotaAfter, receipt2.QuotaAfter)
	assert.Equal(t, receipt1.QuotaVersionAfter, receipt2.QuotaVersionAfter)

	// Verify DB state was NOT modified a second time
	var dbUser User
	require.NoError(t, DB.Where("id = ?", user.Id).First(&dbUser).Error)
	assert.Equal(t, 700, dbUser.Quota, "replay must not duplicate quota credit")
	assert.Equal(t, int64(1), dbUser.QuotaVersion, "replay must not bump version again")

	// Conflict: same BusinessEventKey + different payload (e.g. delta changed to 300)
	conflictInput := input
	conflictInput.Delta = 300
	receiptConflict, err := MutateUserQuota(DB, conflictInput)
	assert.ErrorIs(t, err, ErrUserQuotaMutationConflict, "conflicting payload on same key must be rejected")
	assert.Nil(t, receiptConflict)

	// Another conflict: same key, different reason code
	conflictReason := input
	conflictReason.ReasonCode = "tampered_reason"
	receiptConflict2, err := MutateUserQuota(DB, conflictReason)
	assert.ErrorIs(t, err, ErrUserQuotaMutationConflict)
	assert.Nil(t, receiptConflict2)
}

func TestUserQuotaMutationInsufficientQuota(t *testing.T) {
	truncateTables(t)
	user := createTestUserForQuotaMutation(t, 8003, 100)

	// Attempt to debit 150 from 100 balance
	input := UserQuotaMutationInput{
		UserID:           user.Id,
		Delta:            -150,
		MutationType:     "manual_audit",
		BusinessEventKey: "audit:deduct:8003:1",
		ReasonCode:       "penalty",
	}

	receipt, err := MutateUserQuota(DB, input)
	assert.ErrorIs(t, err, ErrInsufficientUserQuota)
	assert.Nil(t, receipt)

	// Attempt delta == 0
	zeroDeltaInput := UserQuotaMutationInput{
		UserID:           user.Id,
		Delta:            0,
		MutationType:     "manual_audit",
		BusinessEventKey: "audit:zero:8003:1",
		ReasonCode:       "test",
	}
	zeroReceipt, err := MutateUserQuota(DB, zeroDeltaInput)
	assert.ErrorIs(t, err, ErrUserQuotaMutationInvalidInput)
	assert.Nil(t, zeroReceipt)

	// Balance and version in DB must remain completely unchanged
	var dbUser User
	require.NoError(t, DB.Where("id = ?", user.Id).First(&dbUser).Error)
	assert.Equal(t, 100, dbUser.Quota)
	assert.Zero(t, dbUser.QuotaVersion)
}

func TestUserQuotaMutationReceiptImmutability(t *testing.T) {
	truncateTables(t)
	user := createTestUserForQuotaMutation(t, 8004, 500)

	input := UserQuotaMutationInput{
		UserID:           user.Id,
		Delta:            100,
		MutationType:     "admin_adjustment",
		BusinessEventKey: "admin:adj:8004:1",
		ReasonCode:       "compensation",
		OperatorUserID:   1,
	}

	receipt, err := MutateUserQuota(DB, input)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// 1. Direct GORM update on receipt struct must be blocked by BeforeUpdate hook
	err = DB.Model(receipt).Update("delta", 9999).Error
	assert.ErrorIs(t, err, ErrUserQuotaMutationReceiptImmutable)

	// 2. Direct Table update must be blocked by GORM write guard callback
	err = DB.Table("user_quota_mutation_receipts").Where("id = ?", receipt.ID).Update("delta", 9999).Error
	assert.ErrorIs(t, err, ErrUserQuotaMutationReceiptImmutable)

	// 3. Direct GORM delete must be blocked by BeforeDelete hook
	err = DB.Delete(receipt).Error
	assert.ErrorIs(t, err, ErrUserQuotaMutationReceiptImmutable)

	// 4. Direct Table delete must be blocked by GORM write guard callback
	err = DB.Table("user_quota_mutation_receipts").Where("id = ?", receipt.ID).Delete(&UserQuotaMutationReceipt{}).Error
	assert.ErrorIs(t, err, ErrUserQuotaMutationReceiptImmutable)

	// 5. Direct unprivileged GORM create must be blocked by BeforeCreate hook
	unprivilegedReceipt := &UserQuotaMutationReceipt{
		ReceiptVersion:     UserQuotaMutationReceiptVersion,
		MutationType:       "admin_adjustment",
		BusinessEventKey:   "admin:adj:unprivileged:1",
		RequestFingerprint: "f000000000000000000000000000000000000000000000000000000000000009",
		UserID:             user.Id,
		Delta:              100,
		QuotaBefore:        500,
		QuotaAfter:         600,
		QuotaVersionBefore: 0,
		QuotaVersionAfter:  1,
		ReasonCode:         "hack",
	}
	err = DB.Create(unprivilegedReceipt).Error
	assert.ErrorIs(t, err, ErrUserQuotaMutationReceiptImmutable)

	// Verify receipt data in DB was not modified or deleted
	var stored UserQuotaMutationReceipt
	require.NoError(t, DB.Where("id = ?", receipt.ID).First(&stored).Error)
	assert.Equal(t, int64(100), stored.Delta)
}

func TestUserQuotaMutationConcurrencyCAS(t *testing.T) {
	truncateTables(t)
	initialQuota := 10000
	user := createTestUserForQuotaMutation(t, 8005, initialQuota)

	numWorkers := 20
	deltas := make([]int64, numWorkers)
	// Mix of credits and debits
	for i := 0; i < numWorkers; i++ {
		if i%2 == 0 {
			deltas[i] = int64((i + 1) * 50) // positive credit
		} else {
			deltas[i] = -int64((i + 1) * 30) // negative debit
		}
	}

	var expectedDeltaSum int64
	for _, d := range deltas {
		expectedDeltaSum += d
	}

	var wg sync.WaitGroup
	errCh := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		workerIdx := i
		go func() {
			defer wg.Done()
			input := UserQuotaMutationInput{
				UserID:           user.Id,
				Delta:            deltas[workerIdx],
				MutationType:     "concurrent_test",
				BusinessEventKey: fmt.Sprintf("concurrent:worker:%d", workerIdx),
				ReasonCode:       "load_test",
			}

			// In a high contention concurrent environment, retry transient CAS errors with backoff
			var workerErr error
			for attempt := 0; attempt < 50; attempt++ {
				_, workerErr = MutateUserQuota(DB, input)
				if workerErr == nil || workerErr == ErrUserQuotaMutationConflict {
					break
				}
				time.Sleep(time.Duration(attempt*5) * time.Millisecond)
			}
			if workerErr != nil {
				errCh <- fmt.Errorf("worker %d failed: %w", workerIdx, workerErr)
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	// Verify DB state
	var finalUser User
	require.NoError(t, DB.Where("id = ?", user.Id).First(&finalUser).Error)
	expectedFinalQuota := int(int64(initialQuota) + expectedDeltaSum)
	assert.Equal(t, expectedFinalQuota, finalUser.Quota, "final balance must precisely equal initial + sum(deltas)")
	assert.Equal(t, int64(numWorkers), finalUser.QuotaVersion, "quota version must advance exactly once per mutation")

	// Verify receipts count
	var receiptCount int64
	require.NoError(t, DB.Model(&UserQuotaMutationReceipt{}).Where("user_id = ?", user.Id).Count(&receiptCount).Error)
	assert.Equal(t, int64(numWorkers), receiptCount, "all concurrent workers must have generated an immutable receipt")
}

func TestUserQuotaMutationWithCacheHydration(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	user := createTestUserForQuotaMutation(t, 8006, 2000)

	// 1. Initial warm cache via GetUserCache
	cached, err := GetUserCache(user.Id)
	require.NoError(t, err)
	require.NotNil(t, cached)
	assert.Equal(t, 2000, cached.Quota)
	assert.Zero(t, cached.QuotaVersion)

	key := getUserCacheKey(user.Id)
	assert.Equal(t, "2000", server.HGet(key, "Quota"))
	assert.Equal(t, "0", server.HGet(key, "QuotaVersion"))

	// 2. Perform authoritative mutation (+1000)
	input := UserQuotaMutationInput{
		UserID:           user.Id,
		Delta:            1000,
		MutationType:     "recharge",
		BusinessEventKey: "topup:cache:8006:1",
		ReasonCode:       "stripe",
	}

	receipt, err := MutateUserQuota(DB, input)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Equal(t, 3000, receipt.QuotaAfter)
	assert.Equal(t, int64(1), receipt.QuotaVersionAfter)

	// Verify Redis cache was safely hydrated with the new quota and version
	assert.Equal(t, "3000", server.HGet(key, "Quota"), "cache must reflect committed quota")
	assert.Equal(t, "1", server.HGet(key, "QuotaVersion"), "cache must reflect committed quota version")
}

func TestUserQuotaMutationConcurrentReplay(t *testing.T) {
	truncateTables(t)
	user := createTestUserForQuotaMutation(t, 8007, 1000)

	const numCallers = 10
	input := UserQuotaMutationInput{
		UserID:           user.Id,
		Delta:            500,
		MutationType:     "recharge",
		BusinessEventKey: "topup:concurrent:replay:8007",
		ReasonCode:       "idempotency_race",
	}

	var wg sync.WaitGroup
	receipts := make([]*UserQuotaMutationReceipt, numCallers)
	errors := make([]error, numCallers)

	for i := 0; i < numCallers; i++ {
		wg.Add(1)
		callerIdx := i
		go func() {
			defer wg.Done()
			receipts[callerIdx], errors[callerIdx] = MutateUserQuota(DB, input)
		}()
	}

	wg.Wait()

	var firstReceiptID int64
	for i := 0; i < numCallers; i++ {
		require.NoError(t, errors[i], "concurrent replay caller %d must not error", i)
		require.NotNil(t, receipts[i])
		if i == 0 {
			firstReceiptID = receipts[i].ID
		} else {
			assert.Equal(t, firstReceiptID, receipts[i].ID, "all concurrent callers must receive identical receipt")
		}
	}

	// Verify User quota was modified only once
	var dbUser User
	require.NoError(t, DB.Where("id = ?", user.Id).First(&dbUser).Error)
	assert.Equal(t, 1500, dbUser.Quota, "quota must advance exactly once")
	assert.Equal(t, int64(1), dbUser.QuotaVersion, "quota version must advance exactly once")

	// Verify only 1 receipt row in DB
	var receiptCount int64
	require.NoError(t, DB.Model(&UserQuotaMutationReceipt{}).Where("user_id = ?", user.Id).Count(&receiptCount).Error)
	assert.Equal(t, int64(1), receiptCount, "only one receipt must be inserted")
}

func TestUserQuotaMutationReplayDoesNotOverwriteLiveCache(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	user := createTestUserForQuotaMutation(t, 8008, 2000)

	// 1. Initial warm cache via GetUserCache
	_, err := GetUserCache(user.Id)
	require.NoError(t, err)

	key := getUserCacheKey(user.Id)
	assert.Equal(t, "2000", server.HGet(key, "Quota"))
	assert.Equal(t, "0", server.HGet(key, "QuotaVersion"))

	// 2. Perform authoritative mutation (+1000)
	input := UserQuotaMutationInput{
		UserID:           user.Id,
		Delta:            1000,
		MutationType:     "recharge",
		BusinessEventKey: "topup:cache:preserve:8008",
		ReasonCode:       "stripe",
	}

	receipt1, err := MutateUserQuota(DB, input)
	require.NoError(t, err)
	assert.Equal(t, 3000, receipt1.QuotaAfter)
	assert.Equal(t, int64(1), receipt1.QuotaVersionAfter)

	assert.Equal(t, "3000", server.HGet(key, "Quota"))
	assert.Equal(t, "1", server.HGet(key, "QuotaVersion"))

	// 3. Simulate live Relay requests decrementing Quota in Redis by 400
	server.HSet(key, "Quota", "2600")
	assert.Equal(t, "2600", server.HGet(key, "Quota"))

	// 4. Replay the exact same mutation
	receipt2, err := MutateUserQuota(DB, input)
	require.NoError(t, err)
	assert.Equal(t, receipt1.ID, receipt2.ID)

	// 5. Assert that Redis quota was NOT overwritten back to 3000!
	assert.Equal(t, "2600", server.HGet(key, "Quota"), "replay must not roll back decremented live cache balance")
	assert.Equal(t, "1", server.HGet(key, "QuotaVersion"))
}

