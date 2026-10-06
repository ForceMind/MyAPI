package model

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openLegacyBalanceBatchTest(t *testing.T, label string) (*gormTestFixture, accountQuotaFixture) {
	dbFixture, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, label)
	return dbFixture, fixture
}

func openLegacyBalanceBatchTestWithRedis(t *testing.T, label string) (*gormTestFixture, accountQuotaFixture, *miniredis.Miniredis) {
	t.Helper()
	db := openAccountQuotaTestDB(t)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeLegacy, 100+int64(len(label)))
	useQuotaProjectionModelDB(t, db)
	require.NoError(t, db.AutoMigrate(&QuotaBalanceBatchDrain{}, &QuotaBalanceBatchSubject{}))
	require.NoError(t, EnsureQuotaMaintenanceBackfillCursorsWithDB(db))
	server, client := useQuotaProjectionRedis(t)
	state, err := GetQuotaWriterEpochState(db)
	require.NoError(t, err)
	require.NoError(t, client.Set(context.Background(), quotaWriterEpochRedisKey, state.Epoch, 0).Err())
	oldBatch := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	fixture := newAccountQuotaFixture(t, db, label, 1000, 500, false, 0, 0, false)
	require.NoError(t, populateUserCache(fixture.User))
	_, err = cacheInitToken(fixture.Token)
	require.NoError(t, err)
	for _, kind := range []int{BatchUpdateTypeUserQuota, BatchUpdateTypeTokenQuota} {
		batchUpdateLocks[kind].Lock()
		batchUpdateStores[kind] = make(map[int]int)
		batchUpdateLocks[kind].Unlock()
	}
	t.Cleanup(func() {
		common.BatchUpdateEnabled = oldBatch
		quotaBalanceBatchBeforeCreateHook = nil
		quotaBalanceBatchBeforeConfirmHook = nil
		quotaBalanceBatchCacheResponseHook = nil
		quotaBalanceBatchJournalReadHook = nil
		quotaBalanceBatchAfterCompensateHook = nil
		quotaBalanceMigrationBatchHook = nil
		quotaBalanceSubjectLockTTL = 30 * time.Second
		quotaBalanceSubjectLockRenewalDisabled.Store(false)
		quotaBalanceBatchPersistAfterCreateHook = nil
		quotaBalancePendingGeneration = nil
		for _, kind := range []int{BatchUpdateTypeUserQuota, BatchUpdateTypeTokenQuota} {
			batchUpdateLocks[kind].Lock()
			batchUpdateStores[kind] = make(map[int]int)
			batchUpdateLocks[kind].Unlock()
		}
	})
	return &gormTestFixture{DB: db}, fixture, server
}

func TestQuotaBalanceSubjectLockLeaseRenewalAndExpiredOwnerFence(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "subject-lock-lease")
	quotaBalanceSubjectLockTTL = 90 * time.Millisecond
	lock, err := acquireQuotaBalanceSubjectLock(BatchUpdateTypeUserQuota, fixture.User.Id)
	require.NoError(t, err)
	require.Len(t, lock.Token, 48)
	server.FastForward(60 * time.Millisecond)
	time.Sleep(40 * time.Millisecond)
	assert.True(t, server.Exists(lock.Key), "renewal must keep the lease alive")
	releaseQuotaBalanceSubjectLock(lock)

	quotaBalanceSubjectLockRenewalDisabled.Store(true)
	oldLock, err := acquireQuotaBalanceSubjectLock(BatchUpdateTypeUserQuota, fixture.User.Id)
	require.NoError(t, err)
	server.FastForward(100 * time.Millisecond)
	newLock, err := acquireQuotaBalanceSubjectLock(BatchUpdateTypeUserQuota, fixture.User.Id)
	require.NoError(t, err)
	require.NotEqual(t, oldLock.Token, newLock.Token)
	require.NoError(t, common.RDB.HSet(context.Background(), getUserCacheKey(fixture.User.Id), "Quota", "777").Err())
	err = invalidateQuotaBalanceSubjectCache(oldLock, getUserCacheKey(fixture.User.Id), "")
	require.ErrorIs(t, err, ErrQuotaBalanceMutationUnknown)
	assert.Equal(t, "777", server.HGet(getUserCacheKey(fixture.User.Id), "Quota"))
	staleUser := fixture.User
	staleUser.Quota = 1
	err = writeUserCacheWithQuotaBalanceOwner(staleUser.ToBaseUser(), true, oldLock)
	require.ErrorIs(t, err, ErrQuotaBalanceMutationUnknown)
	assert.Equal(t, "777", server.HGet(getUserCacheKey(fixture.User.Id), "Quota"))
	releaseQuotaBalanceSubjectLock(oldLock)
	storedToken, err := server.Get(newLock.Key)
	require.NoError(t, err)
	assert.Equal(t, newLock.Token, storedToken)
	releaseQuotaBalanceSubjectLock(newLock)

	oldTokenLock, err := acquireQuotaBalanceSubjectLock(BatchUpdateTypeTokenQuota, fixture.Token.Id)
	require.NoError(t, err)
	server.FastForward(100 * time.Millisecond)
	newTokenLock, err := acquireQuotaBalanceSubjectLock(BatchUpdateTypeTokenQuota, fixture.Token.Id)
	require.NoError(t, err)
	require.NoError(t, common.RDB.HSet(context.Background(), getTokenCacheKey(fixture.Token.Key), "RemainQuota", "333").Err())
	err = invalidateQuotaBalanceSubjectCache(oldTokenLock, getTokenCacheKey(fixture.Token.Key), getTokenCacheFenceKey(fixture.Token.Key))
	require.ErrorIs(t, err, ErrQuotaBalanceMutationUnknown)
	staleToken := fixture.Token
	staleToken.RemainQuota = 1
	_, err = cacheInitTokenWithQuotaBalanceOwner(staleToken, oldTokenLock)
	require.ErrorIs(t, err, ErrQuotaBalanceMutationUnknown)
	assert.Equal(t, "333", server.HGet(getTokenCacheKey(fixture.Token.Key), "RemainQuota"))
	releaseQuotaBalanceSubjectLock(oldTokenLock)
	releaseQuotaBalanceSubjectLock(newTokenLock)
}

func TestQuotaBalanceSubjectLockProtectsUnconfirmedRedisGeneration(t *testing.T) {
	tests := []struct {
		name      string
		cacheKey  func(accountQuotaFixture) string
		debit     func(accountQuotaFixture) error
		refund    func(accountQuotaFixture) error
		assertAll func(*testing.T, *gormTestFixture, accountQuotaFixture)
	}{
		{
			name:     "user",
			cacheKey: func(f accountQuotaFixture) string { return getUserCacheKey(f.User.Id) },
			debit:    func(f accountQuotaFixture) error { return DecreaseUserQuota(f.User.Id, 25, false) },
			refund:   func(f accountQuotaFixture) error { return IncreaseUserQuota(f.User.Id, 25, false) },
			assertAll: func(t *testing.T, dbFixture *gormTestFixture, f accountQuotaFixture) {
				user, _, _ := loadAccountBalances(t, dbFixture.DB, f)
				assert.Equal(t, 1000, user.Quota)
			},
		},
		{
			name:     "token",
			cacheKey: func(f accountQuotaFixture) string { return getTokenCacheKey(f.Token.Key) },
			debit:    func(f accountQuotaFixture) error { return DecreaseTokenQuota(f.Token.Id, f.Token.Key, 25) },
			refund:   func(f accountQuotaFixture) error { return IncreaseTokenQuota(f.Token.Id, f.Token.Key, 25) },
			assertAll: func(t *testing.T, dbFixture *gormTestFixture, f accountQuotaFixture) {
				_, token, _ := loadAccountBalances(t, dbFixture.DB, f)
				assert.Equal(t, 500, token.RemainQuota)
				assert.Zero(t, token.UsedQuota)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dbFixture, fixture := openLegacyBalanceBatchTest(t, "subject-lock-"+tc.name)
			redisApplied := make(chan struct{})
			releaseConfirm := make(chan struct{})
			quotaBalanceBatchCacheResponseHook = func(_ *QuotaBalanceBatchDrain, result cacheQuotaResult, err error) (cacheQuotaResult, error) {
				quotaBalanceBatchCacheResponseHook = nil
				close(redisApplied)
				<-releaseConfirm
				return result, err
			}
			debitErr := make(chan error, 1)
			go func() { debitErr <- tc.debit(fixture) }()
			<-redisApplied
			require.NoError(t, common.RDB.Del(context.Background(), tc.cacheKey(fixture)).Err())
			refundErr := make(chan error, 1)
			go func() { refundErr <- tc.refund(fixture) }()
			crossedLock := false
			var earlyRefundErr error
			select {
			case earlyRefundErr = <-refundErr:
				crossedLock = true
			case <-time.After(25 * time.Millisecond):
			}
			close(releaseConfirm)
			debitResult := <-debitErr
			var refundResult error
			if crossedLock {
				refundResult = earlyRefundErr
			} else {
				refundResult = <-refundErr
			}
			assert.False(t, crossedLock, "refund crossed the unconfirmed subject lock: %v", earlyRefundErr)
			assert.NoError(t, debitResult)
			assert.NoError(t, refundResult)
			if debitResult != nil || refundResult != nil {
				return
			}
			flushQuotaBalanceBatchGenerations()
			tc.assertAll(t, dbFixture, fixture)
		})
	}
}

func TestQuotaBalanceGenerationErrorCASPreservesTerminalAndRecoveryStates(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "error-cas")
	for index, state := range []string{quotaBalanceBatchDrainUnknown, quotaBalanceBatchDrainCancelled, quotaBalanceBatchDrainCompensating} {
		payload := QuotaBalanceBatchPayload{UserQuota: map[int]int{fixture.User.Id: -1}}
		fingerprint, err := quotaBalanceBatchPayloadFingerprint(payload)
		require.NoError(t, err)
		generation := QuotaBalanceBatchDrain{
			SchemaVersion: quotaBalanceBatchDrainSchemaVersion, GenerationKey: fmt.Sprintf("error-cas-%d", index), WriterEpoch: 999,
			Payload: payload, PayloadFingerprint: fingerprint, CacheApplied: true, State: state,
			LeaseOwner: "worker", LeaseUntil: GetDBTimestamp() + 30, LockVersion: 1, CreatedAt: 1, UpdatedAt: 1,
		}
		require.NoError(t, dbFixture.DB.Create(&generation).Error)
		_ = applyQuotaBalanceBatchGeneration(generation.ID)
		require.NoError(t, dbFixture.DB.First(&generation, generation.ID).Error)
		assert.Equal(t, state, generation.State)
	}
}

func TestQuotaBalanceBatchScriptSuccessWithLostClientResponseCompletesGeneration(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "batch-lost-response")
	quotaBalanceBatchCacheResponseHook = func(_ *QuotaBalanceBatchDrain, result cacheQuotaResult, _ error) (cacheQuotaResult, error) {
		quotaBalanceBatchCacheResponseHook = nil
		return result, errors.New("redis response lost")
	}
	require.NoError(t, DecreaseUserQuota(fixture.User.Id, 25, false))
	user, _, _ := loadAccountBalances(t, dbFixture.DB, fixture)
	assert.Equal(t, 975, user.Quota)
	userCache, _, _ := cachedQuotaValues(t, fixture)
	assert.Equal(t, 975, userCache)
	var generations []QuotaBalanceBatchDrain
	require.NoError(t, dbFixture.DB.Find(&generations).Error)
	require.Len(t, generations, 1)
	assert.Equal(t, quotaBalanceBatchDrainApplied, generations[0].State)
	assert.True(t, generations[0].CacheApplied)
}

func TestQuotaBalanceBatchLostResponseJournalFailureStaysPendingUnknown(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "batch-journal-unknown")
	quotaBalanceBatchCacheResponseHook = func(_ *QuotaBalanceBatchDrain, result cacheQuotaResult, _ error) (cacheQuotaResult, error) {
		quotaBalanceBatchCacheResponseHook = nil
		return result, errors.New("redis response lost")
	}
	quotaBalanceBatchJournalReadHook = func(context.Context, string) (string, error, bool) {
		return "", errors.New("journal connection unavailable"), true
	}
	err := DecreaseUserQuota(fixture.User.Id, 25, false)
	require.ErrorIs(t, err, ErrQuotaBalanceMutationUnknown)
	var generation QuotaBalanceBatchDrain
	require.NoError(t, dbFixture.DB.First(&generation).Error)
	assert.Equal(t, quotaBalanceBatchDrainPending, generation.State)
	assert.False(t, generation.CacheApplied)
	user, _, _ := loadAccountBalances(t, dbFixture.DB, fixture)
	assert.Equal(t, 1000, user.Quota)
	userCache, _, _ := cachedQuotaValues(t, fixture)
	assert.Equal(t, 975, userCache)

	quotaBalanceBatchJournalReadHook = nil
	require.NoError(t, recoverQuotaBalanceBatchPreparations())
	user, _, _ = loadAccountBalances(t, dbFixture.DB, fixture)
	assert.Equal(t, 975, user.Quota)
}

func TestQuotaBalanceBatchRecoverFlushInterleaveKeepsForegroundSuccessSingleApply(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "batch-fixed-interleave")
	scriptDone := make(chan struct{})
	releaseResponse := make(chan struct{})
	quotaBalanceBatchCacheResponseHook = func(_ *QuotaBalanceBatchDrain, result cacheQuotaResult, _ error) (cacheQuotaResult, error) {
		close(scriptDone)
		<-releaseResponse
		return result, errors.New("redis response lost")
	}
	foregroundErr := make(chan error, 1)
	go func() { foregroundErr <- DecreaseUserQuota(fixture.User.Id, 25, false) }()
	<-scriptDone
	require.NoError(t, recoverQuotaBalanceBatchPreparations())
	flushQuotaBalanceBatchGenerations()
	close(releaseResponse)
	require.NoError(t, <-foregroundErr)
	var generations []QuotaBalanceBatchDrain
	require.NoError(t, dbFixture.DB.Find(&generations).Error)
	require.Len(t, generations, 1)
	assert.Equal(t, quotaBalanceBatchDrainApplied, generations[0].State)
	user, _, _ := loadAccountBalances(t, dbFixture.DB, fixture)
	assert.Equal(t, 975, user.Quota)
	userCache, _, _ := cachedQuotaValues(t, fixture)
	assert.Equal(t, 975, userCache)
}

type gormTestFixture struct{ DB *gorm.DB }

func cachedQuotaValues(t *testing.T, fixture accountQuotaFixture) (int, int, int) {
	t.Helper()
	userQuota, err := common.RDB.HGet(context.Background(), getUserCacheKey(fixture.User.Id), "Quota").Int()
	require.NoError(t, err)
	tokenValues, err := common.RDB.HMGet(context.Background(), getTokenCacheKey(fixture.Token.Key), "RemainQuota", "UsedQuota").Result()
	require.NoError(t, err)
	var remain, used int
	_, err = fmt.Sscan(fmt.Sprint(tokenValues[0]), &remain)
	require.NoError(t, err)
	_, err = fmt.Sscan(fmt.Sprint(tokenValues[1]), &used)
	require.NoError(t, err)
	return userQuota, remain, used
}

func TestQuotaBalanceBatchDrainQueuedAndFlushInterleave(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "batch-interleave")
	require.NoError(t, DecreaseUserQuota(fixture.User.Id, 100, false))
	require.NoError(t, DecreaseTokenQuota(fixture.Token.Id, fixture.Token.Key, 100))
	require.NoError(t, DecreaseUserQuota(fixture.User.Id, 40, false))
	require.NoError(t, DecreaseTokenQuota(fixture.Token.Id, fixture.Token.Key, 40))

	userCache, remainCache, usedCache := cachedQuotaValues(t, fixture)
	assert.Equal(t, 860, userCache)
	assert.Equal(t, 360, remainCache)
	assert.Equal(t, 140, usedCache)
	var queued int64
	require.NoError(t, dbFixture.DB.Model(&QuotaBalanceBatchDrain{}).Where("state = ?", quotaBalanceBatchDrainPending).Count(&queued).Error)
	assert.EqualValues(t, 4, queued)

	flushQuotaBalanceBatchGenerations()
	user, token, _ := loadAccountBalances(t, dbFixture.DB, fixture)
	assert.Equal(t, 860, user.Quota)
	assert.Equal(t, 360, token.RemainQuota)
	assert.Equal(t, 140, token.UsedQuota)
	pending, inflight, err := quotaBalanceBatchDrainAudit(dbFixture.DB)
	require.NoError(t, err)
	assert.Zero(t, pending)
	assert.False(t, inflight)
}

func TestQuotaBalanceBatchGenerationFailureDoesNotMutateRedis(t *testing.T) {
	_, fixture := openLegacyBalanceBatchTest(t, "batch-create-failure")
	beforeUser, beforeRemain, beforeUsed := cachedQuotaValues(t, fixture)
	quotaBalanceBatchBeforeCreateHook = func(*QuotaBalanceBatchDrain) error { return errors.New("generation create failed") }
	require.Error(t, DecreaseUserQuota(fixture.User.Id, 50, false))
	require.Error(t, DecreaseTokenQuota(fixture.Token.Id, fixture.Token.Key, 50))
	afterUser, afterRemain, afterUsed := cachedQuotaValues(t, fixture)
	assert.Equal(t, beforeUser, afterUser)
	assert.Equal(t, beforeRemain, afterRemain)
	assert.Equal(t, beforeUsed, afterUsed)
}

func TestQuotaBalanceBatchConfirmFailureCompensatesCreditAndDebit(t *testing.T) {
	cases := []struct {
		name string
		run  func(accountQuotaFixture) error
	}{
		{name: "user credit", run: func(f accountQuotaFixture) error { return IncreaseUserQuota(f.User.Id, 25, false) }},
		{name: "user debit", run: func(f accountQuotaFixture) error { return DecreaseUserQuota(f.User.Id, 25, false) }},
		{name: "token credit", run: func(f accountQuotaFixture) error { return IncreaseTokenQuota(f.Token.Id, f.Token.Key, 25) }},
		{name: "token debit", run: func(f accountQuotaFixture) error { return DecreaseTokenQuota(f.Token.Id, f.Token.Key, 25) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dbFixture, fixture := openLegacyBalanceBatchTest(t, tc.name)
			beforeUser, beforeRemain, beforeUsed := cachedQuotaValues(t, fixture)
			quotaBalanceBatchBeforeConfirmHook = func(*QuotaBalanceBatchDrain) error { return errors.New("generation confirmation failed") }
			require.Error(t, tc.run(fixture))
			afterUser, afterRemain, afterUsed := cachedQuotaValues(t, fixture)
			assert.Equal(t, beforeUser, afterUser)
			assert.Equal(t, beforeRemain, afterRemain)
			assert.Equal(t, beforeUsed, afterUsed)
			var generation QuotaBalanceBatchDrain
			require.NoError(t, dbFixture.DB.Order("id DESC").First(&generation).Error)
			assert.Equal(t, quotaBalanceBatchDrainCancelled, generation.State)
			assert.False(t, generation.CacheApplied)
		})
	}
}

func TestQuotaBalanceBatchCrashWindowRecoveredFromRedisJournal(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "batch-crash-window")
	generation, err := prepareQuotaBalanceBatchGeneration(QuotaBalanceBatchPayload{UserQuota: map[int]int{fixture.User.Id: -75}})
	require.NoError(t, err)
	require.NoError(t, prepareQuotaBalanceJournal(generation.GenerationKey))
	result, err := cacheApplyUserQuotaDeltaJournaled(fixture.User.Id, -75, generation.GenerationKey)
	require.NoError(t, err)
	assert.Equal(t, cacheQuotaOK, result)
	userCache, _, _ := cachedQuotaValues(t, fixture)
	assert.Equal(t, 925, userCache)

	// Simulate restart after Redis committed but before the SQL acknowledgement.
	require.NoError(t, recoverQuotaBalanceBatchPreparations())
	require.NoError(t, dbFixture.DB.First(generation, generation.ID).Error)
	assert.True(t, generation.CacheApplied)
	flushQuotaBalanceBatchGenerations()
	user, _, _ := loadAccountBalances(t, dbFixture.DB, fixture)
	assert.Equal(t, 925, user.Quota)
}

func TestQuotaBalanceBatchDrainFailureIsAuditableAndAtomic(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "batch-failure")
	generation, err := prepareQuotaBalanceBatchGeneration(QuotaBalanceBatchPayload{
		UserQuota: map[int]int{fixture.User.Id: -100}, TokenQuota: map[int]int{fixture.Token.Id: -100},
	})
	require.NoError(t, err)
	require.NoError(t, dbFixture.DB.Delete(&Token{}, fixture.Token.Id).Error)
	require.NoError(t, finishQuotaBalanceBatchPreparation(generation, true, quotaBalanceBatchDrainPending, ""))
	require.Error(t, applyQuotaBalanceBatchGeneration(generation.ID))
	var user User
	require.NoError(t, dbFixture.DB.First(&user, fixture.User.Id).Error)
	assert.Equal(t, 1000, user.Quota)
	require.NoError(t, dbFixture.DB.First(generation, generation.ID).Error)
	assert.Equal(t, quotaBalanceBatchDrainFailed, generation.State)
	assert.NotEmpty(t, generation.LastError)
}

func TestQuotaBalanceBatchDisabledOutsideLegacy(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	useQuotaProjectionModelDB(t, db)
	err := addNewRecord(BatchUpdateTypeUserQuota, 123456, 1)
	require.ErrorIs(t, err, ErrLegacyQuotaWriterModeDisabled)
}

func TestQuotaBalanceBatchPendingApplyingAndFailedBlockDrainReady(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "batch-ready")
	for index, state := range []string{quotaBalanceBatchDrainPending, quotaBalanceBatchDrainApplying, quotaBalanceBatchDrainCompensating, quotaBalanceBatchDrainFailed} {
		payload := QuotaBalanceBatchPayload{UserQuota: map[int]int{fixture.User.Id: index + 1}}
		fingerprint, err := quotaBalanceBatchPayloadFingerprint(payload)
		require.NoError(t, err)
		require.NoError(t, dbFixture.DB.Create(&QuotaBalanceBatchDrain{
			SchemaVersion: quotaBalanceBatchDrainSchemaVersion, GenerationKey: fmt.Sprintf("ready-%d", index), WriterEpoch: 1,
			Payload: payload, PayloadFingerprint: fingerprint, CacheApplied: true, State: state, CreatedAt: 1, UpdatedAt: 1,
		}).Error)
	}
	pending, inflight, err := quotaBalanceBatchDrainAudit(dbFixture.DB)
	require.NoError(t, err)
	assert.EqualValues(t, 4, pending)
	assert.False(t, inflight)
	audit, err := CanEnableDurableQuotaWrites(context.Background(), dbFixture.DB)
	require.NoError(t, err)
	assert.False(t, audit.BalanceDrainInflightZero)
	assert.False(t, audit.CanEnable)
	assert.Contains(t, audit.MissingOrFailedChecks, "balance_drain_inflight_zero")
}

func TestQuotaBalanceGenerationCompletionReadbackStates(t *testing.T) {
	t.Run("pending cache applied drains once", func(t *testing.T) {
		dbFixture, fixture := openLegacyBalanceBatchTest(t, "completion-pending")
		generation, err := prepareQuotaBalanceBatchGeneration(QuotaBalanceBatchPayload{UserQuota: map[int]int{fixture.User.Id: -20}})
		require.NoError(t, err)
		require.NoError(t, prepareQuotaBalanceJournal(generation.GenerationKey))
		result, err := cacheApplyUserQuotaDeltaJournaled(fixture.User.Id, -20, generation.GenerationKey)
		require.NoError(t, err)
		assert.Equal(t, cacheQuotaOK, result)
		require.NoError(t, finishQuotaBalanceBatchPreparation(generation, true, quotaBalanceBatchDrainPending, ""))
		completed, err := completeQuotaBalanceGeneration(generation)
		require.NoError(t, err)
		assert.True(t, completed)
		user, _, _ := loadAccountBalances(t, dbFixture.DB, fixture)
		assert.Equal(t, 980, user.Quota)
	})

	t.Run("applied cache applied is success", func(t *testing.T) {
		dbFixture, fixture := openLegacyBalanceBatchTest(t, "completion-applied")
		generation, err := prepareQuotaBalanceBatchGeneration(QuotaBalanceBatchPayload{UserQuota: map[int]int{fixture.User.Id: -20}})
		require.NoError(t, err)
		require.NoError(t, prepareQuotaBalanceJournal(generation.GenerationKey))
		result, err := cacheApplyUserQuotaDeltaJournaled(fixture.User.Id, -20, generation.GenerationKey)
		require.NoError(t, err)
		assert.Equal(t, cacheQuotaOK, result)
		require.NoError(t, finishQuotaBalanceBatchPreparation(generation, true, quotaBalanceBatchDrainPending, ""))
		require.NoError(t, applyQuotaBalanceBatchGeneration(generation.ID))
		completed, err := completeQuotaBalanceGeneration(generation)
		require.NoError(t, err)
		assert.True(t, completed)
		user, _, _ := loadAccountBalances(t, dbFixture.DB, fixture)
		assert.Equal(t, 980, user.Quota)
	})

	t.Run("applying remains typed unknown without compensation", func(t *testing.T) {
		dbFixture, fixture := openLegacyBalanceBatchTest(t, "completion-applying")
		generation, err := prepareQuotaBalanceBatchGeneration(QuotaBalanceBatchPayload{UserQuota: map[int]int{fixture.User.Id: -20}})
		require.NoError(t, err)
		require.NoError(t, prepareQuotaBalanceJournal(generation.GenerationKey))
		result, err := cacheApplyUserQuotaDeltaJournaled(fixture.User.Id, -20, generation.GenerationKey)
		require.NoError(t, err)
		assert.Equal(t, cacheQuotaOK, result)
		require.NoError(t, finishQuotaBalanceBatchPreparation(generation, true, quotaBalanceBatchDrainApplying, ""))
		completed, err := completeQuotaBalanceGeneration(generation)
		assert.False(t, completed)
		require.ErrorIs(t, err, ErrQuotaBalanceMutationUnknown)
		var stored QuotaBalanceBatchDrain
		require.NoError(t, dbFixture.DB.First(&stored, generation.ID).Error)
		assert.Equal(t, quotaBalanceBatchDrainApplying, stored.State)
		user, _, _ := loadAccountBalances(t, dbFixture.DB, fixture)
		assert.Equal(t, 1000, user.Quota)
		userCache, _, _ := cachedQuotaValues(t, fixture)
		assert.Equal(t, 980, userCache)
	})
}

func TestQuotaBalanceLostResponseRejectsTamperedGenerationIdentity(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "lost-response-tamper")
	quotaBalanceBatchCacheResponseHook = func(generation *QuotaBalanceBatchDrain, result cacheQuotaResult, _ error) (cacheQuotaResult, error) {
		require.NoError(t, dbFixture.DB.Model(&QuotaBalanceBatchDrain{}).Where("id = ?", generation.ID).Update("writer_epoch", generation.WriterEpoch+1).Error)
		return result, errors.New("redis response lost")
	}
	err := DecreaseUserQuota(fixture.User.Id, 20, false)
	require.ErrorIs(t, err, ErrQuotaBalanceMutationUnknown)
	user, _, _ := loadAccountBalances(t, dbFixture.DB, fixture)
	assert.Equal(t, 1000, user.Quota)
	userCache, _, _ := cachedQuotaValues(t, fixture)
	assert.Equal(t, 980, userCache)
}

func TestQuotaBalanceCompensationRecoveryBeforeAndAfterCacheCompensation(t *testing.T) {
	t.Run("crash before compensation", func(t *testing.T) {
		dbFixture, fixture := openLegacyBalanceBatchTest(t, "compensate-before")
		generation, err := prepareQuotaBalanceBatchGeneration(QuotaBalanceBatchPayload{UserQuota: map[int]int{fixture.User.Id: -25}})
		require.NoError(t, err)
		require.NoError(t, prepareQuotaBalanceJournal(generation.GenerationKey))
		result, err := cacheApplyUserQuotaDeltaJournaled(fixture.User.Id, -25, generation.GenerationKey)
		require.NoError(t, err)
		assert.Equal(t, cacheQuotaOK, result)
		claimed, won, err := claimQuotaBalanceCompensation(generation, "before-crash", "confirm failed")
		require.NoError(t, err)
		require.True(t, won)
		require.NoError(t, dbFixture.DB.Model(&QuotaBalanceBatchDrain{}).Where("id = ?", claimed.ID).Update("lease_until", GetDBTimestamp()-1).Error)
		require.NoError(t, recoverQuotaBalanceCompensations())
		require.NoError(t, dbFixture.DB.First(generation, generation.ID).Error)
		assert.Equal(t, quotaBalanceBatchDrainCancelled, generation.State)
		userCache, _, _ := cachedQuotaValues(t, fixture)
		assert.Equal(t, 1000, userCache)
	})

	t.Run("crash after compensation", func(t *testing.T) {
		dbFixture, fixture := openLegacyBalanceBatchTest(t, "compensate-after")
		quotaBalanceBatchBeforeConfirmHook = func(*QuotaBalanceBatchDrain) error { return errors.New("confirm failed") }
		quotaBalanceBatchAfterCompensateHook = func(*QuotaBalanceBatchDrain) error { return errors.New("crash after compensation") }
		err := DecreaseUserQuota(fixture.User.Id, 25, false)
		require.Error(t, err)
		var generation QuotaBalanceBatchDrain
		require.NoError(t, dbFixture.DB.First(&generation).Error)
		assert.Equal(t, quotaBalanceBatchDrainCompensating, generation.State)
		journal, err := readQuotaBalanceJournalDetached(generation.GenerationKey)
		require.NoError(t, err)
		assert.Equal(t, quotaBalanceJournalCompensated, journal)
		userCache, _, _ := cachedQuotaValues(t, fixture)
		assert.Equal(t, 1000, userCache)
		quotaBalanceBatchAfterCompensateHook = nil
		require.NoError(t, dbFixture.DB.Model(&QuotaBalanceBatchDrain{}).Where("id = ?", generation.ID).Update("lease_until", GetDBTimestamp()-1).Error)
		require.NoError(t, recoverQuotaBalanceCompensations())
		require.NoError(t, dbFixture.DB.First(&generation, generation.ID).Error)
		assert.Equal(t, quotaBalanceBatchDrainCancelled, generation.State)
	})
}

func TestQuotaBalanceMissingJournalInvalidatesVerifiableSubjectCaches(t *testing.T) {
	t.Run("user", func(t *testing.T) {
		dbFixture, fixture := openLegacyBalanceBatchTest(t, "missing-user-journal")
		generation, err := prepareQuotaBalanceBatchGeneration(QuotaBalanceBatchPayload{UserQuota: map[int]int{fixture.User.Id: -20}})
		require.NoError(t, err)
		require.NoError(t, prepareQuotaBalanceJournal(generation.GenerationKey))
		result, err := cacheApplyUserQuotaDeltaJournaled(fixture.User.Id, -20, generation.GenerationKey)
		require.NoError(t, err)
		assert.Equal(t, cacheQuotaOK, result)
		require.NoError(t, common.RDB.Del(context.Background(), quotaBalanceJournalRedisKey(generation.GenerationKey)).Err())
		require.ErrorIs(t, recoverQuotaBalanceBatchPreparations(), ErrQuotaBalanceMutationUnknown)
		require.NoError(t, dbFixture.DB.First(generation, generation.ID).Error)
		assert.Equal(t, quotaBalanceBatchDrainUnknown, generation.State)
		assert.EqualValues(t, 1, common.RDB.Exists(context.Background(), getUserCacheKey(fixture.User.Id)).Val())
		userCache, err := GetUserCache(fixture.User.Id)
		require.NoError(t, err)
		assert.Equal(t, 1000, userCache.Quota)
	})

	t.Run("token", func(t *testing.T) {
		dbFixture, fixture := openLegacyBalanceBatchTest(t, "missing-token-journal")
		generation, err := prepareQuotaBalanceBatchGeneration(QuotaBalanceBatchPayload{TokenQuota: map[int]int{fixture.Token.Id: -20}})
		require.NoError(t, err)
		locator := generation.Payload.TokenCacheKeys[fixture.Token.Id]
		assert.Equal(t, getTokenCacheKey(fixture.Token.Key), locator)
		require.NoError(t, prepareQuotaBalanceJournal(generation.GenerationKey))
		result, err := cacheApplyTokenQuotaDeltaJournaled(fixture.Token.Id, fixture.Token.Key, -20, generation.GenerationKey)
		require.NoError(t, err)
		assert.Equal(t, cacheQuotaOK, result)
		require.NoError(t, common.RDB.Del(context.Background(), quotaBalanceJournalRedisKey(generation.GenerationKey)).Err())
		require.ErrorIs(t, recoverQuotaBalanceBatchPreparations(), ErrQuotaBalanceMutationUnknown)
		require.NoError(t, dbFixture.DB.First(generation, generation.ID).Error)
		assert.Equal(t, quotaBalanceBatchDrainUnknown, generation.State)
		assert.EqualValues(t, 1, common.RDB.Exists(context.Background(), locator).Val())
		assert.Zero(t, common.RDB.Exists(context.Background(), getTokenCacheFenceKey(fixture.Token.Key)).Val())
		cached, err := cacheGetTokenByKey(fixture.Token.Key)
		require.NoError(t, err)
		assert.Equal(t, 500, cached.RemainQuota)
		assert.Zero(t, cached.UsedQuota)
	})
}

func TestLegacyBatchRefundRehydratesAfterCacheDeletionAndDrainsInOrder(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "cache-delete-refund")
	require.NoError(t, DecreaseUserQuota(fixture.User.Id, 100, false))
	require.NoError(t, DecreaseTokenQuota(fixture.Token.Id, fixture.Token.Key, 100))
	require.NoError(t, common.RDB.Del(context.Background(), getUserCacheKey(fixture.User.Id), getTokenCacheKey(fixture.Token.Key)).Err())
	require.NoError(t, IncreaseUserQuota(fixture.User.Id, 100, false))
	require.NoError(t, IncreaseTokenQuota(fixture.Token.Id, fixture.Token.Key, 100))
	flushQuotaBalanceBatchGenerations()
	user, token, _ := loadAccountBalances(t, dbFixture.DB, fixture)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 500, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	userCache, tokenRemain, tokenUsed := cachedQuotaValues(t, fixture)
	assert.Equal(t, 1000, userCache)
	assert.Equal(t, 500, tokenRemain)
	assert.Zero(t, tokenUsed)
}

func TestQuotaBalanceJournalAndGenerationRetention(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "retention")
	generation, err := prepareQuotaBalanceBatchGeneration(QuotaBalanceBatchPayload{UserQuota: map[int]int{fixture.User.Id: -10}})
	require.NoError(t, err)
	require.NoError(t, prepareQuotaBalanceJournal(generation.GenerationKey))
	ttl, err := common.RDB.TTL(context.Background(), quotaBalanceJournalRedisKey(generation.GenerationKey)).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	outcome, err := cancelPreparedQuotaBalanceJournal(generation.GenerationKey)
	require.NoError(t, err)
	assert.Equal(t, quotaBalanceJournalCancelled, outcome)
	ttl, err = common.RDB.TTL(context.Background(), quotaBalanceJournalRedisKey(generation.GenerationKey)).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))

	now := GetDBTimestamp()
	for index, state := range []string{quotaBalanceBatchDrainApplied, quotaBalanceBatchDrainCancelled, quotaBalanceBatchDrainUnknown} {
		payload := QuotaBalanceBatchPayload{UserQuota: map[int]int{fixture.User.Id: index + 1}}
		fingerprint, fingerprintErr := quotaBalanceBatchPayloadFingerprint(payload)
		require.NoError(t, fingerprintErr)
		require.NoError(t, dbFixture.DB.Create(&QuotaBalanceBatchDrain{
			SchemaVersion: quotaBalanceBatchDrainSchemaVersion, GenerationKey: fmt.Sprintf("retention-%d", index), WriterEpoch: generation.WriterEpoch,
			Payload: payload, PayloadFingerprint: fingerprint, CacheApplied: true, State: state, LockVersion: 1,
			CreatedAt: now - quotaBalanceGenerationRetention - 1, UpdatedAt: now - quotaBalanceGenerationRetention - 1,
		}).Error)
	}
	deleted, err := cleanupQuotaBalanceBatchGenerations(dbFixture.DB, now)
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted)
	var unknownCount int64
	require.NoError(t, dbFixture.DB.Model(&QuotaBalanceBatchDrain{}).Where("state = ?", quotaBalanceBatchDrainUnknown).Count(&unknownCount).Error)
	assert.EqualValues(t, 1, unknownCount)
}

func TestQuotaBalanceSubjectLockContentionIsKnownNotExecuted(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "subject-lock-busy")
	lock, err := acquireQuotaBalanceSubjectLock(BatchUpdateTypeUserQuota, fixture.User.Id)
	require.NoError(t, err)
	started := time.Now()
	err = IncreaseUserQuota(fixture.User.Id, 25, false)
	require.ErrorIs(t, err, ErrQuotaBalanceSubjectBusy)
	assert.GreaterOrEqual(t, time.Since(started), 1900*time.Millisecond)
	var generationCount int64
	require.NoError(t, dbFixture.DB.Model(&QuotaBalanceBatchDrain{}).Count(&generationCount).Error)
	assert.Zero(t, generationCount, "lock contention occurs before any durable or Redis mutation")
	releaseQuotaBalanceSubjectLock(lock)
}

func TestQuotaBalanceSubjectPreparationIndexAndReleaseDeadline(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "subject-preparation-index")
	assert.True(t, dbFixture.DB.Migrator().HasIndex(&QuotaBalanceBatchDrain{}, "idx_quota_balance_subject_preparation"))
	payload := QuotaBalanceBatchPayload{UserQuota: map[int]int{fixture.User.Id: -1}}
	fingerprint, err := quotaBalanceBatchPayloadFingerprint(payload)
	require.NoError(t, err)
	generation := QuotaBalanceBatchDrain{
		SchemaVersion: quotaBalanceBatchDrainSchemaVersion, GenerationKey: "indexed-subject-preparation", WriterEpoch: 1,
		Payload: payload, PayloadFingerprint: fingerprint, SubjectKind: BatchUpdateTypeUserQuota + 1, SubjectID: fixture.User.Id,
		State: quotaBalanceBatchDrainPending, CacheApplied: false, LockVersion: 1, CreatedAt: 1, UpdatedAt: 1,
	}
	require.NoError(t, dbFixture.DB.Create(&generation).Error)
	var plans []struct {
		Detail string `gorm:"column:detail"`
	}
	require.NoError(t, dbFixture.DB.Raw(
		"EXPLAIN QUERY PLAN SELECT id FROM quota_balance_batch_drains WHERE subject_kind = ? AND subject_id = ? AND state = ? AND cache_applied = ? ORDER BY id ASC LIMIT 25",
		BatchUpdateTypeUserQuota+1, fixture.User.Id, quotaBalanceBatchDrainPending, false,
	).Scan(&plans).Error)
	planText := ""
	for _, plan := range plans {
		planText += plan.Detail
	}
	assert.Contains(t, planText, "idx_quota_balance_subject_preparation")
	plans = nil
	require.NoError(t, dbFixture.DB.Raw(
		"EXPLAIN QUERY PLAN SELECT id FROM quota_balance_batch_drains WHERE subject_kind = ? AND subject_id = ? AND state IN (?, ?) AND cache_applied = ? LIMIT 1",
		quotaBalanceSubjectKindUnknown, 0, quotaBalanceBatchDrainPending, quotaBalanceBatchDrainCompensating, false,
	).Scan(&plans).Error)
	planText = ""
	for _, plan := range plans {
		planText += plan.Detail
	}
	assert.Contains(t, planText, "idx_quota_balance_subject_preparation")

	blocked := &quotaBalanceSubjectLock{Key: "quota:test:release-deadline", Token: "owner", done: make(chan struct{}), renewCancel: func() {}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = releaseQuotaBalanceSubjectLockContext(ctx, blocked)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(started), 200*time.Millisecond)
	close(blocked.done)
}

func TestQuotaBalanceSubjectDrainUsesAssociationBudgetAndCursoredRetry(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "subject-drain-budget")
	assert.True(t, dbFixture.DB.Migrator().HasIndex(&QuotaBalanceBatchSubject{}, "idx_quota_balance_subject_generation"))
	assert.True(t, dbFixture.DB.Migrator().HasIndex(&QuotaBalanceBatchSubject{}, "idx_quota_balance_subject_lookup"))
	state, err := GetQuotaWriterEpochState(dbFixture.DB)
	require.NoError(t, err)
	generations := make([]QuotaBalanceBatchDrain, 0, quotaBalanceSubjectDrainLimit+5)
	for index := 0; index < quotaBalanceSubjectDrainLimit+5; index++ {
		payload := QuotaBalanceBatchPayload{UserQuota: map[int]int{fixture.User.Id: -1}}
		fingerprint, fingerprintErr := quotaBalanceBatchPayloadFingerprint(payload)
		require.NoError(t, fingerprintErr)
		generations = append(generations, QuotaBalanceBatchDrain{
			SchemaVersion: quotaBalanceBatchDrainSchemaVersion, GenerationKey: fmt.Sprintf("subject-drain-%03d", index), WriterEpoch: state.Epoch,
			Payload: payload, PayloadFingerprint: fingerprint, SubjectKind: BatchUpdateTypeUserQuota + 1, SubjectID: fixture.User.Id,
			CacheApplied: true, State: quotaBalanceBatchDrainPending, LockVersion: 1, CreatedAt: int64(index + 1), UpdatedAt: int64(index + 1),
		})
	}
	require.NoError(t, dbFixture.DB.CreateInBatches(&generations, 50).Error)
	for index := range generations {
		require.NoError(t, ensureQuotaBalanceBatchSubjects(dbFixture.DB, &generations[index]))
	}
	err = drainQuotaBalanceGenerationsForSubjectWithDB(dbFixture.DB, BatchUpdateTypeUserQuota, fixture.User.Id)
	require.ErrorIs(t, err, ErrQuotaWorkIncomplete)
	var applied int64
	require.NoError(t, dbFixture.DB.Model(&QuotaBalanceBatchDrain{}).Where("state = ?", quotaBalanceBatchDrainApplied).Count(&applied).Error)
	assert.EqualValues(t, quotaBalanceSubjectDrainLimit, applied)
	require.NoError(t, drainQuotaBalanceGenerationsForSubjectWithDB(dbFixture.DB, BatchUpdateTypeUserQuota, fixture.User.Id))
	require.NoError(t, dbFixture.DB.Model(&QuotaBalanceBatchDrain{}).Where("state = ?", quotaBalanceBatchDrainApplied).Count(&applied).Error)
	assert.EqualValues(t, quotaBalanceSubjectDrainLimit+5, applied)
	var user User
	require.NoError(t, dbFixture.DB.First(&user, fixture.User.Id).Error)
	assert.Equal(t, 1000-quotaBalanceSubjectDrainLimit-5, user.Quota)
}
