package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type legacyQuotaProjectionObligationV1 struct {
	ID                   int64 `gorm:"primaryKey"`
	SchemaVersion        int
	ReceiptKind          string `gorm:"type:varchar(16);uniqueIndex:uidx_quota_projection_receipt,priority:1;uniqueIndex:uidx_quota_projection_event,priority:1"`
	ReceiptID            int64  `gorm:"uniqueIndex:uidx_quota_projection_receipt,priority:2"`
	EventKey             string `gorm:"type:varchar(128);uniqueIndex:uidx_quota_projection_event,priority:2"`
	WriterEpoch          int64
	UserID               int
	TokenID              int
	ExpectedUserVersion  int64
	ExpectedTokenVersion int64
	State                string
	LeaseOwner           string
	LeaseUntil           int64
	Fence                int64
	Attempts             int
	LastError            string
	CreatedAt            int64
	UpdatedAt            int64
}

func (legacyQuotaProjectionObligationV1) TableName() string { return "quota_projection_obligations" }

func openQuotaProjectionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "quota-projection.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &UserQuotaMutationReceipt{}, &QuotaWriterEpoch{}, &QuotaProjectionObligation{}))
	require.NoError(t, EnsureQuotaWriterEpochStateWithDB(db))
	return db
}

func useQuotaProjectionRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldEnabled, oldRDB, oldSync := common.RedisEnabled, common.RDB, common.SyncFrequency
	common.RedisEnabled, common.RDB, common.SyncFrequency = true, client, 60
	t.Cleanup(func() {
		_ = client.Close()
		common.RedisEnabled, common.RDB, common.SyncFrequency = oldEnabled, oldRDB, oldSync
	})
	return server, client
}

func setQuotaWriterStateForTest(t *testing.T, db *gorm.DB, mode QuotaWriterMode, epoch int64) {
	t.Helper()
	require.True(t, validQuotaWriterMode(mode))
	require.Greater(t, epoch, int64(0))
	require.NoError(t, EnsureQuotaWriterEpochStateWithDB(db))
	previous, err := GetQuotaWriterEpochState(db)
	require.NoError(t, err)
	previousCopy := *previous
	t.Cleanup(func() {
		_ = db.Model(&QuotaWriterEpoch{}).Where("id = ?", quotaWriterEpochSingletonID).Updates(map[string]interface{}{
			"mode": previousCopy.Mode, "epoch": previousCopy.Epoch, "lock_version": previousCopy.LockVersion, "updated_at": previousCopy.UpdatedAt,
		}).Error
	})
	require.NoError(t, db.Model(&QuotaWriterEpoch{}).Where("id = ?", quotaWriterEpochSingletonID).Updates(map[string]interface{}{
		"mode": string(mode), "epoch": epoch, "lock_version": gorm.Expr("lock_version + ?", 1),
	}).Error)
}

func useQuotaProjectionModelDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	old := DB
	DB = db
	t.Cleanup(func() { DB = old })
}

func createQuotaProjectionUser(t *testing.T, db *gorm.DB, label string, quota int) User {
	t.Helper()
	user := User{Username: "projection-" + label, Password: "fixture-password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: quota, AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func TestQuotaWriterEpochMigrationAndFailClosedPlan(t *testing.T) {
	db := openQuotaProjectionTestDB(t)
	require.NoError(t, db.AutoMigrate(&QuotaWriterEpoch{}, &QuotaProjectionObligation{}))
	require.NoError(t, EnsureQuotaWriterEpochStateWithDB(db))
	state, err := GetQuotaWriterEpochState(db)
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterEpochSchemaVersion, state.SchemaVersion)
	assert.Equal(t, string(QuotaWriterModeLegacy), state.Mode)
	assert.EqualValues(t, 1, state.Epoch)
	assert.EqualValues(t, 1, state.LockVersion)
	assert.NotZero(t, state.UpdatedAt)

	plan, err := PlanQuotaWriterModeTransition(context.Background(), db, QuotaWriterModeBridge)
	require.NoError(t, err)
	assert.False(t, plan.Ready, "legacy to bridge requires a real distributed drain acknowledgement")
	assert.Contains(t, plan.Validation, "cluster_drain_ack")
	assert.EqualValues(t, 2, plan.ProposedEpoch)
	assert.False(t, plan.Audit.CanEnable)
	assert.Equal(t, string(QuotaWriterModeLegacy), plan.Current.Mode, "planning must not mutate persisted mode")

	audit, err := CanEnableDurableQuotaWrites(context.Background(), db)
	require.NoError(t, err)
	assert.False(t, audit.CanEnable)
	assert.False(t, audit.AllWritersMigrated)
	assert.Contains(t, audit.MissingOrFailedChecks, "redis_epoch")
	assert.Contains(t, audit.MissingOrFailedChecks, "cluster_drain_ack")
	assert.Contains(t, audit.MissingOrFailedChecks, "inflight_zero")
}

func TestQuotaWriterModeTransitionPlanDoesNotOverflowExhaustedEpoch(t *testing.T) {
	db := openQuotaProjectionTestDB(t)
	require.NoError(t, db.Model(&QuotaWriterEpoch{}).Where("id = ?", quotaWriterEpochSingletonID).Updates(map[string]interface{}{
		"epoch": math.MaxInt64, "lock_version": 9,
	}).Error)

	plan, err := PlanQuotaWriterModeTransition(context.Background(), db, QuotaWriterModeBridge)
	require.NoError(t, err)
	assert.False(t, plan.Ready)
	assert.Contains(t, plan.Validation, "epoch_exhausted")
	assert.EqualValues(t, math.MaxInt64, plan.Current.Epoch)
	assert.EqualValues(t, math.MaxInt64, plan.ProposedEpoch, "exhausted plans must not wrap to a negative epoch")

	state, err := GetQuotaWriterEpochState(db)
	require.NoError(t, err)
	assert.EqualValues(t, math.MaxInt64, state.Epoch, "planning must remain read-only")
	assert.Equal(t, string(QuotaWriterModeLegacy), state.Mode)
}

func TestQuotaProjectionWarningCorrelationUsesAccurateLabels(t *testing.T) {
	task := (&QuotaMutationReceipt{RequestID: "request-123", MutationKey: "task:event:1"}).quotaProjectionDescriptor()
	assert.Equal(t, "request_id=request-123", quotaProjectionWarningCorrelation(task, task.EventKey))

	user := (&UserQuotaMutationReceipt{BusinessEventKey: "user:event:1"}).quotaProjectionDescriptor()
	assert.Empty(t, user.RequestID)
	assert.Equal(t, "correlation_key=user:event:1", quotaProjectionWarningCorrelation(user, user.EventKey))

	legacyTask := (&QuotaMutationReceipt{MutationKey: "task:legacy:1"}).quotaProjectionDescriptor()
	assert.Equal(t, "correlation_key=task:legacy:1", quotaProjectionWarningCorrelation(legacyTask, legacyTask.EventKey))

	failure := errors.New("obligation lookup failed")
	assert.Equal(t,
		"quota projection obligation unavailable: request_id=request-123 event=task:event:1 receipt=task/0 error=obligation lookup failed",
		quotaProjectionObligationUnavailableWarning(task, failure),
	)
	assert.Equal(t,
		"quota projection obligation unavailable: correlation_key=user:event:1 event=user:event:1 receipt=user/0 error=obligation lookup failed",
		quotaProjectionObligationUnavailableWarning(user, failure),
	)
}

func TestQuotaProjectionRedisDisabledPreservesLedgerAndObligation(t *testing.T) {
	db := openQuotaProjectionTestDB(t)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 1)
	useQuotaProjectionModelDB(t, db)
	oldEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldEnabled, oldRDB })
	user := createQuotaProjectionUser(t, db, "redis-disabled", 100)

	receipt, err := MutateUserQuota(db, UserQuotaMutationInput{
		UserID: user.Id, Delta: -25, MutationType: "test",
		BusinessEventKey: "projection:redis-disabled:1", ReasonCode: "test",
	})
	require.NoError(t, err)
	assert.Equal(t, 75, receipt.QuotaAfter)
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, 75, stored.Quota)
	var obligation QuotaProjectionObligation
	require.NoError(t, db.Where("receipt_kind = ? AND receipt_id = ?", quotaProjectionReceiptKindUser, receipt.ID).First(&obligation).Error)
	assert.Equal(t, string(QuotaProjectionObligationStateRetryable), obligation.State)
	assert.Equal(t, 1, obligation.Attempts)
	assert.Contains(t, obligation.LastError, "redis projection unavailable")
}

func TestQuotaProjectionHydrateEpochAndVersionMatrix(t *testing.T) {
	db := openQuotaProjectionTestDB(t)
	useQuotaProjectionModelDB(t, db)
	server, _ := useQuotaProjectionRedis(t)
	user := createQuotaProjectionUser(t, db, "matrix", 100)
	require.NoError(t, populateUserCache(user))

	key := getUserCacheKey(user.Id)
	require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(user.Id, 90, 0, 1))
	assert.Equal(t, "100", server.HGet(key, "Quota"), "same epoch and same QV must not overwrite")
	require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(user.Id, 80, 1, 1))
	assert.Equal(t, "80", server.HGet(key, "Quota"))
	require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(user.Id, 60, 0, 2))
	assert.Equal(t, "60", server.HGet(key, "Quota"), "newer epoch wins even with lower QV")
	assert.Equal(t, "2", server.HGet(key, "QuotaWriterEpoch"))
	require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(user.Id, 70, 99, 1))
	assert.Equal(t, "60", server.HGet(key, "Quota"), "older epoch must not overwrite")

	missingID := user.Id + 1000
	require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(missingID, 10, 1, 2))
	assert.False(t, server.Exists(getUserCacheKey(missingID)), "cache miss must not create a partial hash")
}

func TestLegacyWriterEpochMismatchFallsBackToDBAndInvalidates(t *testing.T) {
	db := openQuotaProjectionTestDB(t)
	useQuotaProjectionModelDB(t, db)
	server, _ := useQuotaProjectionRedis(t)
	user := createQuotaProjectionUser(t, db, "legacy-fallback", 100)
	require.NoError(t, populateUserCache(user))
	require.NoError(t, common.RDB.Set(context.Background(), quotaWriterEpochRedisKey, 2, 0).Err())

	reserved, err := TryReserveUserQuota(user.Id, 25)
	require.NoError(t, err)
	assert.True(t, reserved)
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, 75, stored.Quota, "DB remains authoritative")
	assert.False(t, server.Exists(getUserCacheKey(user.Id)), "epoch mismatch must invalidate instead of mutating the new epoch cache")
}

func TestQuotaProjectionObligationSurvivesCommitAndWorkerRestart(t *testing.T) {
	db := openQuotaProjectionTestDB(t)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 1)
	useQuotaProjectionModelDB(t, db)
	_, client := useQuotaProjectionRedis(t)
	user := createQuotaProjectionUser(t, db, "restart", 100)
	require.NoError(t, populateUserCache(user))

	badClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	common.RDB = badClient
	input := UserQuotaMutationInput{UserID: user.Id, Delta: -30, MutationType: "test", BusinessEventKey: "projection:restart:1", ReasonCode: "test"}
	receipt, err := MutateUserQuota(db, input)
	require.NoError(t, err)
	require.NoError(t, badClient.Close())
	common.RDB = client
	assert.Equal(t, 70, receipt.After.Quota)
	assert.EqualValues(t, 1, receipt.After.QuotaVersion)

	var obligation QuotaProjectionObligation
	require.NoError(t, db.Where("receipt_kind = ? AND receipt_id = ?", quotaProjectionReceiptKindUser, receipt.ID).First(&obligation).Error)
	assert.Equal(t, string(QuotaProjectionObligationStateRetryable), obligation.State)
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 100, cached.Quota, "failed projection must not fabricate a cache success")

	require.NoError(t, db.Model(&QuotaProjectionObligation{}).Where("id = ?", obligation.ID).Update("next_attempt_at", 0).Error)
	restoreGate := setQuotaProjectionRecoveryGateForTest(true)
	defer restoreGate()
	processed, err := RunQuotaProjectionObligations(context.Background(), db, "restart-worker", 10)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	cached, err = cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 70, cached.Quota)
	assert.EqualValues(t, 1, cached.QuotaVersion)

	replayed, err := MutateUserQuota(db, input)
	require.NoError(t, err)
	assert.Equal(t, receipt.ID, replayed.ID)
	var count int64
	require.NoError(t, db.Model(&QuotaProjectionObligation{}).Where("receipt_kind = ? AND receipt_id = ?", quotaProjectionReceiptKindUser, receipt.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, 70, stored.Quota, "receipt replay must not repeat the balance mutation")
}

func TestHistoricalUserReceiptWithoutAfterSnapshotNeverMarksProjectionApplied(t *testing.T) {
	db := openQuotaProjectionTestDB(t)
	useQuotaProjectionModelDB(t, db)
	server, _ := useQuotaProjectionRedis(t)
	user := createQuotaProjectionUser(t, db, "historical-after", 80)
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota_version", 1).Error)
	input := UserQuotaMutationInput{
		UserID: user.Id, Delta: -20, MutationType: "test",
		BusinessEventKey: "projection:historical-after:1", ReasonCode: "test",
	}
	_, fingerprint, metadata, err := normalizeUserQuotaMutationInput(input)
	require.NoError(t, err)
	historical := &UserQuotaMutationReceipt{
		ReceiptVersion: UserQuotaMutationReceiptVersion, WriterEpoch: 1, MutationType: input.MutationType,
		BusinessEventKey: input.BusinessEventKey, RequestFingerprint: fingerprint,
		UserID: user.Id, Delta: input.Delta, QuotaBefore: 100, QuotaAfter: 80,
		QuotaVersionBefore: 0, QuotaVersionAfter: 1, ReasonCode: input.ReasonCode, Metadata: metadata,
	}
	require.NoError(t, userQuotaMutationReceiptCreateDB(db).Create(historical).Error)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 1)

	replayed, err := MutateUserQuota(db, input)
	require.NoError(t, err, "historical exact replay behavior must remain successful")
	assert.Equal(t, historical.ID, replayed.ID)
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, 80, stored.Quota)
	assert.EqualValues(t, 1, stored.QuotaVersion)
	assert.False(t, server.Exists(getUserCacheKey(user.Id)), "invalid historical snapshot must not create a cache projection")

	var obligation QuotaProjectionObligation
	require.NoError(t, db.Where("receipt_kind = ? AND receipt_id = ?", quotaProjectionReceiptKindUser, historical.ID).First(&obligation).Error)
	assert.Equal(t, string(QuotaProjectionObligationStateRetryable), obligation.State)
	assert.NotEqual(t, string(QuotaProjectionObligationStateApplied), obligation.State)
	assert.Contains(t, obligation.LastError, "after snapshot is missing or inconsistent")
}

func TestQuotaProjectionPartialFailureInvalidatesBothCaches(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	useQuotaProjectionModelDB(t, db)
	server, _ := useQuotaProjectionRedis(t)
	input := newTaskQuotaReservationFixture(t, db, "projection-partial")
	var user User
	require.NoError(t, db.First(&user, input.UserID).Error)
	require.NoError(t, populateUserCache(user))

	receipt, err := ReserveTaskQuota(db, input)
	require.NoError(t, err)
	var token Token
	require.NoError(t, db.First(&token, input.TokenID).Error)
	require.NoError(t, common.RDB.Set(context.Background(), getTokenCacheKey(token.Key), "wrong-type", 0).Err())

	err = ProjectQuotaMutationReceipt(context.Background(), db, receipt)
	require.Error(t, err)
	assert.False(t, server.Exists(getUserCacheKey(user.Id)), "user success followed by token failure must invalidate user")
	assert.False(t, server.Exists(getTokenCacheKey(token.Key)), "token failure must invalidate token")
	var obligation QuotaProjectionObligation
	require.NoError(t, db.Where("receipt_kind = ? AND receipt_id = ?", quotaProjectionReceiptKindTask, receipt.ID).First(&obligation).Error)
	assert.Equal(t, string(QuotaProjectionObligationStateRetryable), obligation.State)
}

func TestQuotaProjectionConcurrentClaimAndGate(t *testing.T) {
	db := openQuotaProjectionTestDB(t)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 1)
	useQuotaProjectionModelDB(t, db)
	useQuotaProjectionRedis(t)
	user := createQuotaProjectionUser(t, db, "claim", 100)
	input := UserQuotaMutationInput{UserID: user.Id, Delta: -10, MutationType: "test", BusinessEventKey: "projection:claim:1", ReasonCode: "test"}
	var receipt *UserQuotaMutationReceipt
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		receipt, _, err = mutateUserQuotaAuthoritative(tx, input)
		return err
	}))

	processed, err := RunQuotaProjectionObligations(context.Background(), db, "gate-off", 10)
	require.NoError(t, err)
	assert.Zero(t, processed, "runner must be inert while obligation gate is off")

	var obligation QuotaProjectionObligation
	require.NoError(t, db.Where("receipt_kind = ? AND receipt_id = ?", quotaProjectionReceiptKindUser, receipt.ID).First(&obligation).Error)
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	errorsCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			_, won, claimErr := claimQuotaProjectionObligation(db, obligation.ID, fmt.Sprintf("worker-%d", worker), 30)
			results <- won
			errorsCh <- claimErr
		}(i)
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	wins := 0
	for won := range results {
		if won {
			wins++
		}
	}
	for claimErr := range errorsCh {
		require.NoError(t, claimErr)
	}
	assert.Equal(t, 1, wins, "fence/CAS must allow only one concurrent worker")
}

func TestDurableKernelModeGatesAndExactReplay(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	input := newTaskQuotaReservationFixture(t, db, "mode-gate")

	setQuotaWriterStateForTest(t, db, QuotaWriterModeLegacy, 1)
	receipt, err := ReserveTaskQuota(db, input)
	assert.Nil(t, receipt)
	assert.ErrorIs(t, err, ErrDurableQuotaWriterModeDisabled)
	assertTaskQuotaWallet(t, db, input, 1000, 500, 0, 0)

	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 2)
	receipt, err = ReserveTaskQuota(db, input)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.EqualValues(t, 2, receipt.WriterEpoch)
	assertTaskQuotaWallet(t, db, input, 900, 400, 100, 1)

	setQuotaWriterStateForTest(t, db, QuotaWriterModeLegacy, 2)
	replayed, err := ReserveTaskQuota(db, input)
	require.NoError(t, err, "exact reserve replay must not require a new durable write")
	assert.Equal(t, receipt.ID, replayed.ID)
	assertTaskQuotaWallet(t, db, input, 900, 400, 100, 1)

	_, err = ReleaseTaskQuotaReservation(db, TaskQuotaReleaseInput{
		OperationID: input.OperationID, UserID: input.UserID, TokenID: input.TokenID,
		ChannelID: input.ChannelID, ExpectedOperationVersion: receipt.OperationVersionAfter,
		ReasonCode: "mode_gate_release", BillingContext: input.BillingContext,
	})
	assert.ErrorIs(t, err, ErrQuotaProjectionModeDisabled)

	user := createQuotaProjectionUser(t, db, "mode-user", 100)
	mutation := UserQuotaMutationInput{UserID: user.Id, Delta: -10, MutationType: "test", BusinessEventKey: "mode:user:1", ReasonCode: "test"}
	mutated, err := MutateUserQuota(db, mutation)
	assert.Nil(t, mutated)
	assert.ErrorIs(t, err, ErrDurableQuotaWriterModeDisabled)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 2)
	mutated, err = MutateUserQuota(db, mutation)
	require.NoError(t, err)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeLegacy, 2)
	mutatedReplay, err := MutateUserQuota(db, mutation)
	require.NoError(t, err)
	assert.Equal(t, mutated.ID, mutatedReplay.ID)
}

func TestDurableAndLegacyQuotaWritersCannotInterleave(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	useQuotaProjectionModelDB(t, db)
	useQuotaProjectionRedis(t)
	input := newTaskQuotaReservationFixture(t, db, "writer-interleave")
	var user User
	var token Token
	require.NoError(t, db.First(&user, input.UserID).Error)
	require.NoError(t, db.First(&token, input.TokenID).Error)
	require.NoError(t, populateUserCache(user))
	_, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)

	receipt, err := ReserveTaskQuota(db, input)
	require.NoError(t, err)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeBridge, receipt.WriterEpoch)

	reserved, err := TryReserveUserQuota(user.Id, 1)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, ErrLegacyQuotaWriterModeDisabled)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 1, false)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, ErrLegacyQuotaWriterModeDisabled)
	assertTaskQuotaWallet(t, db, input, 900, 400, 100, 1)

	released, err := ReleaseTaskQuotaReservation(db, TaskQuotaReleaseInput{
		OperationID: input.OperationID, UserID: input.UserID, TokenID: input.TokenID,
		ChannelID: input.ChannelID, ExpectedOperationVersion: receipt.OperationVersionAfter,
		ReasonCode: "writer_interleave_release", BillingContext: input.BillingContext,
	})
	require.NoError(t, err)
	assert.EqualValues(t, receipt.WriterEpoch, released.WriterEpoch)
	assertTaskQuotaWallet(t, db, input, 1000, 500, 0, 2)
}

func TestQuotaProjectionRetryBackoffManualAndAuditedRepair(t *testing.T) {
	db := openQuotaProjectionTestDB(t)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 1)
	useQuotaProjectionModelDB(t, db)
	useQuotaProjectionRedis(t)
	user := createQuotaProjectionUser(t, db, "repair", 100)
	require.NoError(t, populateUserCache(user))
	input := UserQuotaMutationInput{UserID: user.Id, Delta: -20, MutationType: "test", BusinessEventKey: "projection:repair:1", ReasonCode: "test"}
	var receipt *UserQuotaMutationReceipt
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		receipt, _, err = mutateUserQuotaAuthoritative(tx, input)
		return err
	}))
	var obligation QuotaProjectionObligation
	require.NoError(t, db.Where("receipt_kind = ? AND receipt_id = ?", quotaProjectionReceiptKindUser, receipt.ID).First(&obligation).Error)

	var tenthClaim *QuotaProjectionObligation
	for attempt := 1; attempt <= quotaProjectionMaxAttempts; attempt++ {
		require.NoError(t, db.Model(&QuotaProjectionObligation{}).Where("id = ?", obligation.ID).Update("next_attempt_at", 0).Error)
		claimed, won, err := claimQuotaProjectionObligation(db, obligation.ID, fmt.Sprintf("failure-%d", attempt), 30)
		require.NoError(t, err)
		require.True(t, won)
		if attempt == quotaProjectionMaxAttempts {
			tenthClaim = claimed
		}
		require.NoError(t, finishQuotaProjectionObligation(db, claimed, errors.New("forced projection failure")))
		require.NoError(t, db.First(&obligation, obligation.ID).Error)
		if attempt < quotaProjectionMaxAttempts {
			assert.Equal(t, string(QuotaProjectionObligationStateRetryable), obligation.State)
			assert.Greater(t, obligation.NextAttemptAt, obligation.UpdatedAt)
			if attempt == 1 {
				assert.EqualValues(t, quotaProjectionRetryBaseSeconds, obligation.NextAttemptAt-obligation.UpdatedAt)
			}
			if attempt == 9 {
				assert.EqualValues(t, quotaProjectionRetryMaxSeconds, obligation.NextAttemptAt-obligation.UpdatedAt)
			}
		} else {
			assert.Equal(t, string(QuotaProjectionObligationStateManual), obligation.State)
			assert.Zero(t, obligation.NextAttemptAt)
		}
	}

	err := ProjectQuotaMutationReceipt(context.Background(), db, receipt)
	assert.ErrorIs(t, err, ErrQuotaProjectionManual)

	repaired, err := RequeueQuotaProjectionObligation(db, QuotaProjectionRepairInput{
		ObligationID: obligation.ID, AuditCommandID: "audit-repair-001", Reason: "verified redis recovery",
	})
	require.NoError(t, err)
	assert.Equal(t, string(QuotaProjectionObligationStateRetryable), repaired.State)
	assert.Zero(t, repaired.Attempts)
	assert.Equal(t, "audit-repair-001", repaired.RepairAuditCommandID)
	assert.Equal(t, "verified redis recovery", repaired.RepairReason)
	assert.NotZero(t, repaired.RepairedAt)
	assert.ErrorIs(t, finishQuotaProjectionObligation(db, tenthClaim, nil), ErrQuotaProjectionLeaseLost, "old fence/owner must not finish after repair")

	require.NoError(t, ProjectQuotaMutationReceipt(context.Background(), db, receipt))
	require.NoError(t, db.First(&obligation, obligation.ID).Error)
	assert.Equal(t, string(QuotaProjectionObligationStateApplied), obligation.State)
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 80, cached.Quota)
}

func TestQuotaProjectionNotDueAndRepairCanFailAgainSafely(t *testing.T) {
	db := openQuotaProjectionTestDB(t)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 1)
	useQuotaProjectionModelDB(t, db)
	user := createQuotaProjectionUser(t, db, "repair-fail", 100)
	var receipt *UserQuotaMutationReceipt
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		receipt, _, err = mutateUserQuotaAuthoritative(tx, UserQuotaMutationInput{UserID: user.Id, Delta: -10, MutationType: "test", BusinessEventKey: "projection:repair-fail:1", ReasonCode: "test"})
		return err
	}))
	var obligation QuotaProjectionObligation
	require.NoError(t, db.Where("receipt_kind = ? AND receipt_id = ?", quotaProjectionReceiptKindUser, receipt.ID).First(&obligation).Error)
	claimed, won, err := claimQuotaProjectionObligation(db, obligation.ID, "not-due-worker", 30)
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, finishQuotaProjectionObligation(db, claimed, errors.New("redis unavailable")))
	assert.ErrorIs(t, ProjectQuotaMutationReceipt(context.Background(), db, receipt), ErrQuotaProjectionNotDue)

	require.NoError(t, db.Model(&QuotaProjectionObligation{}).Where("id = ?", obligation.ID).Updates(map[string]interface{}{
		"state": string(QuotaProjectionObligationStateManual), "next_attempt_at": 0,
	}).Error)
	_, err = RequeueQuotaProjectionObligation(db, QuotaProjectionRepairInput{ObligationID: obligation.ID, AuditCommandID: "audit-retry-002", Reason: "retry after inspection"})
	require.NoError(t, err)
	oldEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldEnabled, oldRDB })
	err = ProjectQuotaMutationReceipt(context.Background(), db, receipt)
	require.Error(t, err)
	require.NoError(t, db.First(&obligation, obligation.ID).Error)
	assert.Equal(t, string(QuotaProjectionObligationStateRetryable), obligation.State)
	assert.Equal(t, 1, obligation.Attempts)
}

func TestQuotaWriterTransitionPlannerIsTargetAwareAndFailClosed(t *testing.T) {
	db := openQuotaProjectionTestDB(t)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeBridge, 7)
	plan, err := PlanQuotaWriterModeTransition(context.Background(), db, QuotaWriterModeAuthoritative)
	require.NoError(t, err)
	assert.False(t, plan.Ready)
	assert.NotContains(t, plan.Validation, "bridge_mode_required")
	assert.Contains(t, plan.Validation, "durable_write_audit_failed")
	assert.Contains(t, plan.Audit.MissingOrFailedChecks, "cluster_drain_ack")
	assert.Contains(t, plan.Audit.MissingOrFailedChecks, "inflight_zero")

	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 7)
	downgrade, err := PlanQuotaWriterModeTransition(context.Background(), db, QuotaWriterModeBridge)
	require.NoError(t, err)
	assert.False(t, downgrade.Ready)
	assert.Contains(t, downgrade.Validation, "authoritative_mode_is_not_downgradable")
}

func TestQuotaProjectionObligationMigrationInitializesHistoricalRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota-projection-migration.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&legacyQuotaProjectionObligationV1{}))
	require.NoError(t, db.Create(&legacyQuotaProjectionObligationV1{
		ID: 1, SchemaVersion: 1, ReceiptKind: "user", ReceiptID: 7, EventKey: "historical:projection",
		WriterEpoch: 3, UserID: 9, ExpectedUserVersion: 4, State: string(QuotaProjectionObligationStateRetryable),
		Fence: 2, Attempts: 1, LastError: "old failure", CreatedAt: 10, UpdatedAt: 10,
	}).Error)
	for range 2 {
		require.NoError(t, db.AutoMigrate(&QuotaProjectionObligation{}))
		require.NoError(t, InitializeQuotaProjectionObligationsWithDB(db))
	}
	var obligation QuotaProjectionObligation
	require.NoError(t, db.First(&obligation, 1).Error)
	assert.Equal(t, quotaProjectionObligationSchemaVersion, obligation.SchemaVersion)
	assert.EqualValues(t, 1, obligation.LockVersion)
	assert.Zero(t, obligation.NextAttemptAt)
	assert.Equal(t, string(QuotaProjectionObligationStateRetryable), obligation.State)
	assert.Equal(t, "old failure", obligation.LastError)
}
