package model

import (
	"context"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func openQuotaWriterTransitionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openQuotaProjectionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}, &QuotaWriterModeTransition{}, &QuotaBalanceBatchDrain{}, &QuotaBalanceBatchSubject{}, &QuotaWorkCursor{}))
	return db
}

func seedCompleteQuotaWorkCursors(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, name := range []string{quotaWorkCursorAccountMigration, quotaWorkCursorBalanceMigration, quotaWorkCursorRefundRecovery} {
		require.NoError(t, db.Clauses(clause.OnConflict{DoNothing: true}).Create(&QuotaWorkCursor{Name: name, Complete: true, UpdatedAt: 1}).Error)
	}
}

func TestApplyQuotaWriterTransitionLegacyToBridge(t *testing.T) {
	db := openQuotaWriterTransitionTestDB(t)

	// Without an acknowledgement and without a note the apply must fail closed
	// and persist failed evidence.
	transition, err := ApplyQuotaWriterModeTransition(context.Background(), db, QuotaWriterModeTransitionInput{
		TargetMode: QuotaWriterModeBridge, ExpectedEpoch: 1, OperatorUserId: 42,
	})
	var conditionsErr *QuotaWriterTransitionConditionsError
	require.ErrorAs(t, err, &conditionsErr)
	assert.Equal(t, []string{"cluster_drain_ack"}, conditionsErr.Missing)
	require.NotNil(t, transition, "condition failures must still return the evidence row")
	assert.Equal(t, QuotaWriterTransitionStatusFailed, transition.Status)
	assert.Equal(t, "cluster_drain_ack", transition.FailureReason)
	assert.Equal(t, string(QuotaWriterModeLegacy), transition.FromMode)
	var stored QuotaWriterModeTransition
	require.NoError(t, db.First(&stored, transition.ID).Error)
	assert.Equal(t, QuotaWriterTransitionStatusFailed, stored.Status)
	assert.NotEmpty(t, stored.PreAudit)

	state, err := GetQuotaWriterEpochState(db)
	require.NoError(t, err)
	assert.Equal(t, string(QuotaWriterModeLegacy), state.Mode, "a failed apply must not move the epoch row")
	assert.EqualValues(t, 1, state.Epoch)

	// Supplying the ack note records the epoch-bound acknowledgement and
	// completes the transition.
	transition, err = ApplyQuotaWriterModeTransition(context.Background(), db, QuotaWriterModeTransitionInput{
		TargetMode: QuotaWriterModeBridge, ExpectedEpoch: 1, OperatorUserId: 42, ClusterDrainAckNote: "cluster drained for bridge",
	})
	require.NoError(t, err)
	require.NotNil(t, transition)
	assert.Equal(t, QuotaWriterTransitionStatusSucceeded, transition.Status)
	assert.EqualValues(t, 1, transition.FromEpoch)
	assert.EqualValues(t, 2, transition.ToEpoch)
	assert.True(t, transition.ClusterDrainAck)
	assert.Equal(t, "cluster drained for bridge", transition.AckNote)
	assert.NotZero(t, transition.FinishedAt)
	assert.NotEmpty(t, transition.PostAudit)

	state, err = GetQuotaWriterEpochState(db)
	require.NoError(t, err)
	assert.Equal(t, string(QuotaWriterModeBridge), state.Mode)
	assert.EqualValues(t, 2, state.Epoch)
	assert.EqualValues(t, 2, state.LockVersion)

	ack, err := loadQuotaWriterClusterDrainAck(db)
	require.NoError(t, err)
	require.NotNil(t, ack)
	assert.Equal(t, 42, ack.OperatorUserId)
	assert.EqualValues(t, 1, ack.Epoch, "the acknowledgement stays bound to the epoch it confirmed")
	valid, err := quotaWriterClusterDrainAckValid(db, state.Epoch)
	require.NoError(t, err)
	assert.False(t, valid, "a successful transition advances the epoch and invalidates the old acknowledgement")

	// Repeating the same apply now conflicts: the epoch already moved.
	_, err = ApplyQuotaWriterModeTransition(context.Background(), db, QuotaWriterModeTransitionInput{
		TargetMode: QuotaWriterModeBridge, ExpectedEpoch: 1, OperatorUserId: 42, ClusterDrainAckNote: "retry",
	})
	require.ErrorIs(t, err, ErrQuotaWriterTransitionConflict)

	// An invalid target is rejected before any state change.
	_, err = ApplyQuotaWriterModeTransition(context.Background(), db, QuotaWriterModeTransitionInput{
		TargetMode: QuotaWriterModeLegacy, ExpectedEpoch: 2, OperatorUserId: 42,
	})
	require.ErrorIs(t, err, ErrQuotaWriterTransitionInvalid)
}

func TestApplyQuotaWriterTransitionConcurrentConflict(t *testing.T) {
	db := openQuotaWriterTransitionTestDB(t)

	const workers = 2
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := ApplyQuotaWriterModeTransition(context.Background(), db, QuotaWriterModeTransitionInput{
				TargetMode: QuotaWriterModeBridge, ExpectedEpoch: 1, OperatorUserId: 7, ClusterDrainAckNote: "concurrent drain ack",
			})
			errs[index] = err
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		assert.ErrorIs(t, err, ErrQuotaWriterTransitionConflict)
	}
	assert.Equal(t, 1, succeeded, "exactly one concurrent apply may win the epoch compare-and-swap")

	var succeededCount int64
	require.NoError(t, db.Model(&QuotaWriterModeTransition{}).Where("status = ?", QuotaWriterTransitionStatusSucceeded).Count(&succeededCount).Error)
	assert.EqualValues(t, 1, succeededCount)

	state, err := GetQuotaWriterEpochState(db)
	require.NoError(t, err)
	assert.Equal(t, string(QuotaWriterModeBridge), state.Mode)
	assert.EqualValues(t, 2, state.Epoch)
}

func TestApplyQuotaWriterTransitionBridgeToAuthoritative(t *testing.T) {
	db := openQuotaWriterTransitionTestDB(t)
	seedCompleteQuotaWorkCursors(t, db)
	server, _ := useQuotaProjectionRedis(t)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeBridge, 7)
	server.Set(quotaWriterEpochRedisKey, "7")

	apply := func(note string) (*QuotaWriterModeTransition, error) {
		return ApplyQuotaWriterModeTransition(context.Background(), db, QuotaWriterModeTransitionInput{
			TargetMode: QuotaWriterModeAuthoritative, ExpectedEpoch: 7, OperatorUserId: 9, ClusterDrainAckNote: note,
		})
	}

	// A required writer reported as not migrated still fails closed with
	// persisted evidence. The production inventory itself is fully migrated.
	t.Run("unmigrated required writer", func(t *testing.T) {
		previous := quotaWriterRegistrationsForAudit
		quotaWriterRegistrationsForAudit = func() []QuotaWriterRegistration {
			return append(ProductionQuotaWriterRegistrations(), QuotaWriterRegistration{Name: "test_required_writer", Migrated: false})
		}
		t.Cleanup(func() { quotaWriterRegistrationsForAudit = previous })

		transition, err := apply("bridge drained")
		var conditionsErr *QuotaWriterTransitionConditionsError
		require.ErrorAs(t, err, &conditionsErr)
		assert.Equal(t, []string{"production_writer_registration"}, conditionsErr.Missing)
		require.NotNil(t, transition)
		assert.Equal(t, QuotaWriterTransitionStatusFailed, transition.Status)
		assert.Contains(t, transition.PreAudit, "production_writer_registration")
	})

	// Projection backlog blocks the transition.
	require.NoError(t, db.Create(&QuotaProjectionObligation{
		SchemaVersion: quotaProjectionObligationSchemaVersion, ReceiptKind: quotaProjectionReceiptKindUser, ReceiptID: 1, EventKey: "transition:blocking:1",
		WriterEpoch: 7, UserID: 1, State: string(QuotaProjectionObligationStatePending),
		Fence: 1, LockVersion: 1, CreatedAt: 1, UpdatedAt: 1,
	}).Error)
	_, err := apply("bridge drained")
	var conditionsErr *QuotaWriterTransitionConditionsError
	require.ErrorAs(t, err, &conditionsErr)
	assert.Contains(t, conditionsErr.Missing, "projection_pending")
	require.NoError(t, db.Where("event_key = ?", "transition:blocking:1").Delete(&QuotaProjectionObligation{}).Error)

	// In-flight sessions block the transition.
	TrackQuotaWriterInflightStart()
	_, err = apply("bridge drained")
	require.ErrorAs(t, err, &conditionsErr)
	assert.Contains(t, conditionsErr.Missing, "inflight_zero")
	TrackQuotaWriterInflightFinish()

	// Without any acknowledgement the cluster drain check blocks the transition.
	_, err = apply("")
	require.ErrorAs(t, err, &conditionsErr)
	assert.Contains(t, conditionsErr.Missing, "cluster_drain_ack")

	// Redis disabled fails the epoch consistency check exactly like the audit.
	oldEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	_, err = apply("bridge drained")
	require.ErrorAs(t, err, &conditionsErr)
	assert.Contains(t, conditionsErr.Missing, "redis_epoch")
	common.RedisEnabled, common.RDB = oldEnabled, oldRDB

	// All conditions green: the transition commits, bumps epoch and lock
	// version, publishes the Redis epoch and marks the evidence succeeded.
	transition, err := apply("bridge drained")
	require.NoError(t, err)
	require.NotNil(t, transition)
	assert.Equal(t, QuotaWriterTransitionStatusSucceeded, transition.Status)
	assert.EqualValues(t, 7, transition.FromEpoch)
	assert.EqualValues(t, 8, transition.ToEpoch)

	state, err := GetQuotaWriterEpochState(db)
	require.NoError(t, err)
	assert.Equal(t, string(QuotaWriterModeAuthoritative), state.Mode)
	assert.EqualValues(t, 8, state.Epoch)
	previousLockVersion := state.LockVersion
	assert.Greater(t, previousLockVersion, int64(1))

	redisEpoch, err := server.Get(quotaWriterEpochRedisKey)
	require.NoError(t, err)
	assert.Equal(t, "8", redisEpoch, "the committed epoch must be mirrored to Redis")

	// Authoritative cannot be downgraded.
	_, err = ApplyQuotaWriterModeTransition(context.Background(), db, QuotaWriterModeTransitionInput{
		TargetMode: QuotaWriterModeBridge, ExpectedEpoch: 8, OperatorUserId: 9,
	})
	require.ErrorIs(t, err, ErrQuotaWriterTransitionInvalid)

	// The failure evidence rows from every blocked attempt remain queryable.
	var failedCount int64
	require.NoError(t, db.Model(&QuotaWriterModeTransition{}).Where("status = ?", QuotaWriterTransitionStatusFailed).Count(&failedCount).Error)
	assert.EqualValues(t, 5, failedCount)
}

func TestQuotaWriterClusterDrainAckValidation(t *testing.T) {
	db := openQuotaWriterTransitionTestDB(t)

	valid, err := quotaWriterClusterDrainAckValid(db, 3)
	require.NoError(t, err)
	assert.False(t, valid, "no record means no acknowledgement")

	_, err = RecordQuotaWriterClusterDrainAck(db, 5, 3, "   ")
	require.ErrorIs(t, err, ErrQuotaWriterTransitionInvalid)

	ack, err := RecordQuotaWriterClusterDrainAck(db, 5, 3, "drained node-a and node-b")
	require.NoError(t, err)
	assert.Equal(t, 5, ack.OperatorUserId)
	assert.EqualValues(t, 3, ack.Epoch)
	assert.NotZero(t, ack.Timestamp)

	valid, err = quotaWriterClusterDrainAckValid(db, 3)
	require.NoError(t, err)
	assert.True(t, valid)
	valid, err = quotaWriterClusterDrainAckValid(db, 4)
	require.NoError(t, err)
	assert.False(t, valid, "the acknowledgement is bound to its epoch")

	// A newer acknowledgement replaces the older one (upsert on the option key).
	_, err = RecordQuotaWriterClusterDrainAck(db, 6, 4, "re-drained for epoch 4")
	require.NoError(t, err)
	valid, err = quotaWriterClusterDrainAckValid(db, 4)
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestDriveQuotaWriterDrainsToZero(t *testing.T) {
	db := openQuotaWriterTransitionTestDB(t)
	useQuotaProjectionModelDB(t, db)
	user := createQuotaProjectionUser(t, db, "drain", 100)

	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUserQuota][user.Id] = 50
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	t.Cleanup(func() {
		batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
		delete(batchUpdateStores[BatchUpdateTypeUserQuota], user.Id)
		batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	})

	report, err := DriveQuotaWriterDrains(context.Background(), db, 10)
	require.NoError(t, err)
	assert.True(t, report.Complete)
	assert.Zero(t, report.BatchQueueRemaining)
	assert.Zero(t, report.BalanceDrainPending)
	assert.Zero(t, report.ProjectionPending)

	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, 150, stored.Quota, "the drained batch delta must reach the user balance")

	var pending int64
	require.NoError(t, db.Model(&QuotaBalanceBatchDrain{}).Where("state NOT IN ?", []string{quotaBalanceBatchDrainApplied, quotaBalanceBatchDrainCancelled}).Count(&pending).Error)
	assert.Zero(t, pending)
}

func TestDriveQuotaWriterDrainsProjectionBacklog(t *testing.T) {
	db := openQuotaWriterTransitionTestDB(t)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 1)
	useQuotaProjectionModelDB(t, db)
	_, client := useQuotaProjectionRedis(t)
	user := createQuotaProjectionUser(t, db, "drain-projection", 100)
	require.NoError(t, populateUserCache(user))

	badClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	common.RDB = badClient
	input := UserQuotaMutationInput{UserID: user.Id, Delta: -30, MutationType: "test", BusinessEventKey: "projection:drain:1", ReasonCode: "test"}
	_, err := MutateUserQuota(db, input)
	require.NoError(t, err)
	require.NoError(t, badClient.Close())
	common.RDB = client

	report, err := DriveQuotaWriterDrains(context.Background(), db, 3)
	require.NoError(t, err)
	assert.EqualValues(t, 1, report.ProjectionPending, "the recovery gate defaults off, so the backlog is reported but not replayed")
	assert.False(t, report.Complete)

	restoreGate := setQuotaProjectionRecoveryGateForTest(true)
	defer restoreGate()
	require.NoError(t, db.Model(&QuotaProjectionObligation{}).Where("state <> ?", string(QuotaProjectionObligationStateApplied)).Update("next_attempt_at", 0).Error)
	report, err = DriveQuotaWriterDrains(context.Background(), db, 5)
	require.NoError(t, err)
	assert.True(t, report.Complete)
	assert.Zero(t, report.ProjectionPending)

	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 70, cached.Quota)
}

func TestListQuotaWriterModeTransitionsPagination(t *testing.T) {
	db := openQuotaWriterTransitionTestDB(t)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&QuotaWriterModeTransition{
			OperatorUserId: 1, FromMode: string(QuotaWriterModeLegacy), ToMode: string(QuotaWriterModeBridge),
			FromEpoch: int64(i + 1), ToEpoch: int64(i + 2), Status: QuotaWriterTransitionStatusFailed,
			PreAudit: "{}", PostAudit: "{}", FailureReason: "cluster_drain_ack", CreatedAt: int64(i + 1), FinishedAt: int64(i + 1),
		}).Error)
	}
	items, total, err := ListQuotaWriterModeTransitions(db, 0, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, items, 2)
	assert.Greater(t, items[0].ID, items[1].ID, "history is newest-first")
	items, total, err = ListQuotaWriterModeTransitions(db, 2, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	assert.Len(t, items, 1)
}

func TestQuotaWriterInflightCounterBounds(t *testing.T) {
	before := QuotaWriterInflightSessions()
	TrackQuotaWriterInflightStart()
	assert.Equal(t, before+1, QuotaWriterInflightSessions())
	TrackQuotaWriterInflightFinish()
	assert.Equal(t, before, QuotaWriterInflightSessions())

	// A finish without a matching start clamps at zero instead of going
	// negative.
	for QuotaWriterInflightSessions() > 0 {
		TrackQuotaWriterInflightFinish()
	}
	TrackQuotaWriterInflightFinish()
	assert.Zero(t, QuotaWriterInflightSessions())
}
