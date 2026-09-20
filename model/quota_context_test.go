package model

import (
	"context"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestQuotaContextVariantsRequireBusinessKeyAndDisableLegacyAPIs(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	useQuotaProjectionModelDB(t, db)
	fixture := newAccountQuotaFixture(t, db, "context", 1000, 500, false, 0, 0, false)

	require.ErrorIs(t, DecreaseUserQuota(fixture.User.Id, 10, false), ErrLegacyQuotaWriterModeDisabled)
	_, err := TryReserveUserQuota(fixture.User.Id, 10)
	require.ErrorIs(t, err, ErrLegacyQuotaWriterModeDisabled)
	require.ErrorIs(t, DecreaseTokenQuota(fixture.Token.Id, fixture.Token.Key, 10), ErrLegacyQuotaWriterModeDisabled)
	_, err = TryReserveTokenQuota(fixture.Token.Id, fixture.Token.Key, 10, false)
	require.ErrorIs(t, err, ErrLegacyQuotaWriterModeDisabled)

	_, err = TryReserveUserQuotaWithContext(QuotaMutationContext{}, fixture.User.Id, 10)
	require.ErrorIs(t, err, ErrAccountQuotaMutationInvalidInput)
	reserved, err := TryReserveUserQuotaWithContext(QuotaMutationContext{Context: context.Background(), EventKey: "context:user:reserve", ReasonCode: "relay_reserve"}, fixture.User.Id, 100)
	require.NoError(t, err)
	assert.True(t, reserved)
	reserved, err = TryReserveUserQuotaWithContext(QuotaMutationContext{Context: context.Background(), EventKey: "context:user:reserve", ReasonCode: "relay_reserve"}, fixture.User.Id, 100)
	require.NoError(t, err)
	assert.True(t, reserved)

	reserved, err = TryReserveTokenQuotaWithContext(QuotaMutationContext{Context: context.Background(), EventKey: "context:token:reserve", ReasonCode: "relay_reserve"}, fixture.Token.Id, fixture.Token.Key, 100, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	reserved, err = TryReserveTokenQuotaWithContext(QuotaMutationContext{Context: context.Background(), EventKey: "context:token:reserve", ReasonCode: "relay_reserve"}, fixture.Token.Id, fixture.Token.Key, 100, false)
	require.NoError(t, err)
	assert.True(t, reserved)

	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 400, token.RemainQuota)
	assert.Equal(t, 100, token.UsedQuota)
}

func TestQuotaContextLegacyBatchFailsClosedWithoutRedisAuthority(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeLegacy, 21)
	useQuotaProjectionModelDB(t, db)
	fixture := newAccountQuotaFixture(t, db, "legacy-batch", 1000, 500, false, 0, 0, false)
	oldBatch, oldRedis, oldRDB := common.BatchUpdateEnabled, common.RedisEnabled, common.RDB
	common.BatchUpdateEnabled, common.RedisEnabled, common.RDB = true, false, nil
	for _, kind := range []int{BatchUpdateTypeUserQuota, BatchUpdateTypeTokenQuota} {
		batchUpdateLocks[kind].Lock()
		batchUpdateStores[kind] = make(map[int]int)
		batchUpdateLocks[kind].Unlock()
	}
	t.Cleanup(func() {
		common.BatchUpdateEnabled, common.RedisEnabled, common.RDB = oldBatch, oldRedis, oldRDB
		for _, kind := range []int{BatchUpdateTypeUserQuota, BatchUpdateTypeTokenQuota} {
			batchUpdateLocks[kind].Lock()
			batchUpdateStores[kind] = make(map[int]int)
			batchUpdateLocks[kind].Unlock()
		}
	})

	require.ErrorIs(t, IncreaseUserQuotaWithContext(QuotaMutationContext{EventKey: "legacy:user:credit"}, fixture.User.Id, 25), ErrBatchQuotaCacheUnavailable)
	require.ErrorIs(t, DecreaseTokenQuotaWithContext(QuotaMutationContext{EventKey: "legacy:token:debit"}, fixture.Token.Id, fixture.Token.Key, 10), ErrBatchQuotaCacheUnavailable)
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 500, token.RemainQuota)
	assert.True(t, quotaBatchQueueEmpty())
}

func TestQuotaMutationContextCancellationReachesModeAndTokenQueries(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeLegacy, 31)
	useQuotaProjectionModelDB(t, db)
	fixture := newAccountQuotaFixture(t, db, "context-cancel", 1000, 500, false, 0, 0, false)
	oldBatch := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	t.Cleanup(func() { common.BatchUpdateEnabled = oldBatch })

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	err := IncreaseUserQuotaWithContext(QuotaMutationContext{Context: cancelled, EventKey: "context:cancelled-user"}, fixture.User.Id, 25)
	require.ErrorIs(t, err, context.Canceled)
	reserved, err := TryReserveTokenQuotaWithContext(QuotaMutationContext{Context: cancelled, EventKey: "context:cancelled-token"}, fixture.Token.Id, fixture.Token.Key, 25, false)
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, reserved)

	const modeCallback = "test:quota-context-mode-deadline"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(modeCallback, func(tx *gorm.DB) {
		if tx.Statement.Table == "quota_writer_epochs" && tx.Statement.Context != nil {
			<-tx.Statement.Context.Done()
			tx.AddError(tx.Statement.Context.Err())
		}
	}))
	modeCtx, modeCancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	started := time.Now()
	err = IncreaseUserQuotaWithContext(QuotaMutationContext{Context: modeCtx, EventKey: "context:deadline-mode"}, fixture.User.Id, 25)
	modeCancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(started), 250*time.Millisecond)
	require.NoError(t, db.Callback().Query().Remove(modeCallback))

	const callbackName = "test:quota-context-token-deadline"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" && tx.Statement.Context != nil {
			<-tx.Statement.Context.Done()
			tx.AddError(tx.Statement.Context.Err())
		}
	}))
	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer deadlineCancel()
	started = time.Now()
	err = DecreaseTokenQuotaWithContext(QuotaMutationContext{Context: deadlineCtx, EventKey: "context:deadline-token"}, fixture.Token.Id, fixture.Token.Key, 25)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(started), 250*time.Millisecond)
	require.NoError(t, db.Callback().Query().Remove(callbackName))

	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 500, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
}
