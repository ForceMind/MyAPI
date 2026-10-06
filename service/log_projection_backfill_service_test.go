package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func openLogProjectionBackfillSQLite(t *testing.T, suffix string) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name()+suffix, "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return db
}

func migrateLogProjectionBackfillFixture(t *testing.T, mainDB *gorm.DB, logDB *gorm.DB) {
	t.Helper()
	require.NoError(t, mainDB.AutoMigrate(&model.LogProjectionBackfillState{}, &model.SystemTaskLock{}))
	require.NoError(t, logDB.AutoMigrate(&model.Log{}))
	require.NoError(t, model.EnsureLogProjectionSchemaWithDB(logDB))
}

func runLogProjectionBackfillBatchForTest(t *testing.T, mainDB *gorm.DB, logDB *gorm.DB, limit int) (LogProjectionBackfillBatchResult, error) {
	t.Helper()
	taskID := "test-task-" + common.NewRequestId()
	runnerID := "test-runner-" + common.NewRequestId()
	const fenceToken int64 = 1
	require.NoError(t, mainDB.Create(&model.SystemTaskLock{
		Type: model.SystemTaskTypeLogProjectionBackfill, TaskID: taskID, LockedBy: runnerID,
		LockedUntil: common.GetTimestamp() + 60, FenceToken: fenceToken,
	}).Error)
	defer func() {
		require.NoError(t, mainDB.Where("type = ?", model.SystemTaskTypeLogProjectionBackfill).Delete(&model.SystemTaskLock{}).Error)
	}()
	return RunLogProjectionBackfillBatch(t.Context(), mainDB, logDB, taskID, runnerID, fenceToken, limit)
}

func insertLegacyBackfillLog(t *testing.T, db *gorm.DB, id int, eventID string) {
	t.Helper()
	require.NoError(t, db.Exec(
		"INSERT INTO logs (id, billing_event_id, content, request_id, billing_projection_digest, log_row_key) VALUES (?, ?, ?, ?, '', '')",
		id, eventID, "legacy", fmt.Sprintf("request-%d", id),
	).Error)
}

func TestLogProjectionBackfillSystemBatchesResumeAndComplete(t *testing.T) {
	mainDB := openLogProjectionBackfillSQLite(t, "_main")
	logDB := openLogProjectionBackfillSQLite(t, "_log")
	migrateLogProjectionBackfillFixture(t, mainDB, logDB)
	for id := 1; id <= 3; id++ {
		insertLegacyBackfillLog(t, logDB, id, fmt.Sprintf("event-%d", id))
	}

	first, err := runLogProjectionBackfillBatchForTest(t, mainDB, logDB, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, first.Processed)
	assert.Equal(t, 2, first.LastID)
	assert.Equal(t, model.LogProjectionBackfillPhaseRelationalIdentity, first.Phase)
	assert.Equal(t, model.LogProjectionBackfillStatusPending, first.Status)

	second, err := runLogProjectionBackfillBatchForTest(t, mainDB, logDB, 2)
	require.NoError(t, err)
	assert.Equal(t, 1, second.Processed)
	assert.Equal(t, 3, second.LastID)
	assert.Equal(t, model.LogProjectionBackfillPhaseRelationalIndexes, second.Phase)

	third, err := runLogProjectionBackfillBatchForTest(t, mainDB, logDB, 2)
	require.NoError(t, err)
	assert.Equal(t, model.LogProjectionBackfillPhaseRelationalIndexes, third.Phase)
	assert.True(t, logDB.Migrator().HasIndex(&model.Log{}, "idx_logs_billing_canonical"))
	assert.False(t, logDB.Migrator().HasIndex(&model.Log{}, "idx_logs_row_key"))

	fourth, err := runLogProjectionBackfillBatchForTest(t, mainDB, logDB, 2)
	require.NoError(t, err)
	assert.Equal(t, model.LogProjectionBackfillPhaseRelationalIndexes, fourth.Phase)
	assert.True(t, logDB.Migrator().HasIndex(&model.Log{}, "idx_logs_row_key"))

	completed, err := runLogProjectionBackfillBatchForTest(t, mainDB, logDB, 2)
	require.NoError(t, err)
	assert.Equal(t, model.LogProjectionBackfillPhaseCompleted, completed.Phase)
	assert.Equal(t, model.LogProjectionBackfillStatusCompleted, completed.Status)

	replayed, err := runLogProjectionBackfillBatchForTest(t, mainDB, logDB, 2)
	require.NoError(t, err)
	assert.True(t, replayed.NoOp)
	assert.Equal(t, model.LogProjectionBackfillStatusCompleted, replayed.Status)

	var stateRows int64
	require.NoError(t, mainDB.Model(&model.LogProjectionBackfillState{}).Count(&stateRows).Error)
	assert.Equal(t, int64(1), stateRows, "all system-task runs must reuse the unique durable state row")
	persistedState, err := model.GetLogProjectionBackfillState(t.Context(), mainDB)
	require.NoError(t, err)
	require.NotNil(t, persistedState)
	assert.NotZero(t, persistedState.UpdatedAt)
	assert.NotZero(t, persistedState.CompletedAt)
	var missing int64
	require.NoError(t, logDB.Model(&model.Log{}).Where("log_row_key = '' OR billing_projection_digest = ''").Count(&missing).Error)
	assert.Zero(t, missing)
}

func TestLogProjectionBackfillPersistsFailedCheckpointAndRecovers(t *testing.T) {
	mainDB := openLogProjectionBackfillSQLite(t, "_main")
	logDB := openLogProjectionBackfillSQLite(t, "_log")
	migrateLogProjectionBackfillFixture(t, mainDB, logDB)
	insertLegacyBackfillLog(t, logDB, 1, "event-1")
	insertLegacyBackfillLog(t, logDB, 2, "event-2")

	failure := errors.New("injected log projection update failure")
	updates := 0
	require.NoError(t, logDB.Callback().Update().Before("gorm:update").Register("test:projection-backfill-failure", func(tx *gorm.DB) {
		updates++
		if updates == 2 {
			tx.AddError(failure)
		}
	}))

	result, err := runLogProjectionBackfillBatchForTest(t, mainDB, logDB, 2)
	require.ErrorIs(t, err, failure)
	assert.Equal(t, 1, result.Processed)
	state, loadErr := model.GetLogProjectionBackfillState(t.Context(), mainDB)
	require.NoError(t, loadErr)
	require.NotNil(t, state)
	assert.Equal(t, 1, state.LastID)
	assert.Equal(t, model.LogProjectionBackfillStatusFailed, state.Status)
	assert.Contains(t, state.LastError, failure.Error())

	require.NoError(t, logDB.Callback().Update().Remove("test:projection-backfill-failure"))
	resumed, err := runLogProjectionBackfillBatchForTest(t, mainDB, logDB, 2)
	require.NoError(t, err)
	assert.Equal(t, 1, resumed.Processed)
	assert.Equal(t, 2, resumed.LastID)
	assert.Equal(t, model.LogProjectionBackfillPhaseRelationalIndexes, resumed.Phase)
}

func TestLogProjectionBackfillMySQLIndexFailureStopsForManualReview(t *testing.T) {
	mainDB := openLogProjectionBackfillSQLite(t, "_main")
	require.NoError(t, mainDB.AutoMigrate(&model.LogProjectionBackfillState{}, &model.SystemTaskLock{}))
	require.NoError(t, mainDB.Create(&model.LogProjectionBackfillState{
		TaskKey:           model.LogProjectionBackfillTaskKey,
		SchemaVersion:     model.LogProjectionBackfillSchemaVersion,
		Phase:             model.LogProjectionBackfillPhaseRelationalIndexes,
		Status:            model.LogProjectionBackfillStatusPending,
		MaterializeStatus: model.LogProjectionMaterializeStatusPending,
	}).Error)

	inertSQLite := openLogProjectionBackfillSQLite(t, "_mysql_inert")
	connection, err := inertSQLite.DB()
	require.NoError(t, err)
	mysqlDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      connection,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)

	result, err := runLogProjectionBackfillBatchForTest(t, mainDB, mysqlDB, 10)
	require.Error(t, err)
	assert.Equal(t, model.LogProjectionBackfillStatusManualReview, result.Status)
	state, loadErr := model.GetLogProjectionBackfillState(t.Context(), mainDB)
	require.NoError(t, loadErr)
	require.NotNil(t, state)
	assert.Equal(t, model.LogProjectionBackfillStatusManualReview, state.Status)
	assert.Contains(t, state.LastError, "idx_logs_billing_canonical")

	previousGate := common.TaskRecoveryObligationRecoveryEnabled
	common.TaskRecoveryObligationRecoveryEnabled = true
	t.Cleanup(func() { common.TaskRecoveryObligationRecoveryEnabled = previousGate })
	assert.False(t, ShouldScheduleLogProjectionBackfill(t.Context(), mainDB), "manual review must stop automatic retries")
}

func TestLogProjectionMaterializeStateReconciliation(t *testing.T) {
	t.Run("running", func(t *testing.T) {
		state := &model.LogProjectionBackfillState{Status: model.LogProjectionBackfillStatusRunning}
		manualReview, err := reconcileClickHouseProjectionMutation(state, &model.ClickHouseProjectionMutation{MutationID: "mutation-1"})
		require.NoError(t, err)
		assert.False(t, manualReview)
		assert.Equal(t, model.LogProjectionBackfillStatusPending, state.Status)
		assert.Equal(t, model.LogProjectionMaterializeStatusRunning, state.MaterializeStatus)
	})

	t.Run("completed", func(t *testing.T) {
		state := &model.LogProjectionBackfillState{Status: model.LogProjectionBackfillStatusRunning}
		manualReview, err := reconcileClickHouseProjectionMutation(state, &model.ClickHouseProjectionMutation{MutationID: "mutation-2", IsDone: 1})
		require.NoError(t, err)
		assert.False(t, manualReview)
		assert.Equal(t, model.LogProjectionBackfillStatusCompleted, state.Status)
		assert.Equal(t, model.LogProjectionBackfillPhaseCompleted, state.Phase)
		assert.Equal(t, model.LogProjectionMaterializeStatusCompleted, state.MaterializeStatus)
		assert.NotZero(t, state.CompletedAt)
	})

	t.Run("failed", func(t *testing.T) {
		state := &model.LogProjectionBackfillState{Status: model.LogProjectionBackfillStatusRunning}
		manualReview, err := reconcileClickHouseProjectionMutation(state, &model.ClickHouseProjectionMutation{
			MutationID:       "mutation-3",
			IsDone:           1,
			LatestFailReason: "materialize failed",
		})
		require.ErrorContains(t, err, "materialize failed")
		assert.True(t, manualReview)
		assert.Equal(t, model.LogProjectionMaterializeStatusFailed, state.MaterializeStatus)
	})

	t.Run("unqueryable", func(t *testing.T) {
		state := &model.LogProjectionBackfillState{Status: model.LogProjectionBackfillStatusRunning}
		manualReview, err := reconcileClickHouseProjectionMutation(state, nil)
		require.ErrorContains(t, err, "status is unavailable")
		assert.True(t, manualReview)
		assert.Equal(t, model.LogProjectionMaterializeStatusFailed, state.MaterializeStatus)
	})
}

func TestClickHouseLogProjectionBackfillConflictPersistsQuarantineAndContinues(t *testing.T) {
	mainDB := openLogProjectionBackfillSQLite(t, "_main")
	require.NoError(t, mainDB.AutoMigrate(&model.LogProjectionBackfillState{}, &model.SystemTaskLock{}))
	logDB := openTaskBillingClickHouseSQLiteFixture(t)
	for id, content := range []string{"first", "different"} {
		require.NoError(t, logDB.Exec(
			"INSERT INTO logs (id, billing_event_id, content, request_id) VALUES (?, ?, ?, ?)",
			id+1, "conflicting-history-event", content, "same-request",
		).Error)
	}

	result, err := runLogProjectionBackfillBatchForTest(t, mainDB, logDB, 10)
	require.NoError(t, err)
	assert.Equal(t, model.LogProjectionBackfillStatusPending, result.Status)
	assert.Equal(t, model.LogProjectionBackfillPhaseClickHouseMaterial, result.Phase)
	assert.Equal(t, int64(1), result.QuarantinedEvents)
	state, loadErr := model.GetLogProjectionBackfillState(t.Context(), mainDB)
	require.NoError(t, loadErr)
	require.NotNil(t, state)
	assert.Equal(t, "conflicting-history-event", state.LastEventID)
	assert.Empty(t, state.LastError)
}

func TestLogProjectionBackfillFenceRejectsExpiredRunnerLateCheckpoint(t *testing.T) {
	mainDB := openLogProjectionBackfillSQLite(t, "_fence_main")
	logDB := openLogProjectionBackfillSQLite(t, "_fence_log")
	require.NoError(t, mainDB.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}, &model.LogProjectionBackfillState{}))
	require.NoError(t, logDB.AutoMigrate(&model.Log{}))
	require.NoError(t, model.EnsureLogProjectionSchemaWithDB(logDB))
	insertLegacyBackfillLog(t, logDB, 1, "fenced-event")

	previousDB := model.DB
	model.DB = mainDB
	t.Cleanup(func() { model.DB = previousDB })

	taskA, err := model.CreateSystemTask(model.SystemTaskTypeLogProjectionBackfill, nil, nil)
	require.NoError(t, err)
	claimedA, won, err := model.ClaimSystemTask(taskA.ID, taskA.Type, "runner-a", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, won)
	staleState, err := model.GetOrCreateLogProjectionBackfillState(t.Context(), mainDB, logDB)
	require.NoError(t, err)

	require.NoError(t, mainDB.Model(&model.SystemTaskLock{}).Where("task_id = ?", taskA.TaskID).Update("locked_until", common.GetTimestamp()-1).Error)
	require.NoError(t, model.MarkSystemTaskLeaseExpired(taskA.TaskID))
	taskB, err := model.CreateSystemTask(model.SystemTaskTypeLogProjectionBackfill, nil, nil)
	require.NoError(t, err)
	claimedB, won, err := model.ClaimSystemTask(taskB.ID, taskB.Type, "runner-b", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, won)
	require.Greater(t, claimedB.FenceToken, claimedA.FenceToken)

	result, err := RunLogProjectionBackfillBatch(t.Context(), mainDB, logDB, claimedB.TaskID, "runner-b", claimedB.FenceToken, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Processed)

	staleState.LastID = 999
	err = model.SaveLogProjectionBackfillStateFenced(t.Context(), mainDB, staleState, claimedA.TaskID, "runner-a", claimedA.FenceToken, staleState.LockVersion)
	require.ErrorIs(t, err, model.ErrSystemTaskLockLost)
	persisted, err := model.GetLogProjectionBackfillState(t.Context(), mainDB)
	require.NoError(t, err)
	assert.NotEqual(t, 999, persisted.LastID)
}

func TestIdentifyClickHouseProjectionMutationRequiresUniqueGenerationCandidate(t *testing.T) {
	state := &model.LogProjectionBackfillState{MaterializeGeneration: 7, MaterializeRequestedAt: 1000}
	_, err := identifyClickHouseProjectionMutation(state, nil)
	require.ErrorContains(t, err, "0 identifiable mutations")
	_, err = identifyClickHouseProjectionMutation(state, []model.ClickHouseProjectionMutation{{MutationID: "a"}, {MutationID: "b"}})
	require.ErrorContains(t, err, "2 identifiable mutations")
	mutation, err := identifyClickHouseProjectionMutation(state, []model.ClickHouseProjectionMutation{{MutationID: "only"}})
	require.NoError(t, err)
	assert.Equal(t, "only", mutation.MutationID)
}

func TestClickHouseMaterializePersistsStartingBeforeAlterAndDoesNotReplayStarting(t *testing.T) {
	mainDB := openLogProjectionBackfillSQLite(t, "_materialize_main")
	require.NoError(t, mainDB.AutoMigrate(&model.LogProjectionBackfillState{}, &model.SystemTaskLock{}))
	logDB := openTaskBillingClickHouseSQLiteFixture(t)
	state := &model.LogProjectionBackfillState{
		TaskKey: model.LogProjectionBackfillTaskKey, SchemaVersion: model.LogProjectionBackfillSchemaVersion,
		Phase: model.LogProjectionBackfillPhaseClickHouseMaterial, Status: model.LogProjectionBackfillStatusPending,
		MaterializeStatus: model.LogProjectionMaterializeStatusPending,
	}
	require.NoError(t, mainDB.Create(state).Error)

	result, err := runLogProjectionBackfillBatchForTest(t, mainDB, logDB, 10)
	require.Error(t, err)
	assert.Equal(t, model.LogProjectionBackfillStatusManualReview, result.Status)
	persisted, loadErr := model.GetLogProjectionBackfillState(t.Context(), mainDB)
	require.NoError(t, loadErr)
	require.NotNil(t, persisted)
	assert.NotZero(t, persisted.MaterializeGeneration)
	assert.NotZero(t, persisted.MaterializeRequestedAt)

	persisted.Status = model.LogProjectionBackfillStatusPending
	persisted.MaterializeStatus = model.LogProjectionMaterializeStatusStarting
	persisted.LastError = ""
	require.NoError(t, mainDB.Model(&model.LogProjectionBackfillState{}).Where("task_key = ?", model.LogProjectionBackfillTaskKey).Updates(map[string]interface{}{
		"status": model.LogProjectionBackfillStatusPending, "materialize_status": model.LogProjectionMaterializeStatusStarting, "last_error": "",
	}).Error)
	_, err = runLogProjectionBackfillBatchForTest(t, mainDB, logDB, 10)
	require.Error(t, err)
	persisted, loadErr = model.GetLogProjectionBackfillState(t.Context(), mainDB)
	require.NoError(t, loadErr)
	assert.Equal(t, model.LogProjectionBackfillStatusFailed, persisted.Status)
	assert.Equal(t, model.LogProjectionMaterializeStatusStarting, persisted.MaterializeStatus)
	assert.Contains(t, strings.ToLower(persisted.LastError), "near \"table\"")
	assert.NotContains(t, strings.ToLower(persisted.LastError), "materialize")
}

func TestClickHouseMaterializeFindFailureRetriesStartingAndNextRunAdopts(t *testing.T) {
	mainDB := openLogProjectionBackfillSQLite(t, "_materialize_retry_main")
	require.NoError(t, mainDB.AutoMigrate(&model.LogProjectionBackfillState{}, &model.SystemTaskLock{}))
	logDB := openTaskBillingClickHouseSQLiteFixture(t)
	require.NoError(t, mainDB.Create(&model.LogProjectionBackfillState{
		TaskKey: model.LogProjectionBackfillTaskKey, SchemaVersion: model.LogProjectionBackfillSchemaVersion,
		Phase: model.LogProjectionBackfillPhaseClickHouseMaterial, Status: model.LogProjectionBackfillStatusPending,
		MaterializeStatus: model.LogProjectionMaterializeStatusPending,
	}).Error)

	originalStart, originalFind := startClickHouseProjectionMaterialize, findClickHouseProjectionMutations
	startCalls, findCalls := 0, 0
	startClickHouseProjectionMaterialize = func(context.Context, *gorm.DB) error {
		startCalls++
		return nil
	}
	findClickHouseProjectionMutations = func(context.Context, *gorm.DB, int64) ([]model.ClickHouseProjectionMutation, error) {
		findCalls++
		if findCalls == 1 {
			return nil, errors.New("temporary mutation query failure")
		}
		return []model.ClickHouseProjectionMutation{{MutationID: "mutation-adopted"}}, nil
	}
	t.Cleanup(func() {
		startClickHouseProjectionMaterialize, findClickHouseProjectionMutations = originalStart, originalFind
	})

	first, err := runLogProjectionBackfillBatchForTest(t, mainDB, logDB, 10)
	require.ErrorContains(t, err, "temporary mutation query failure")
	assert.Equal(t, model.LogProjectionBackfillStatusFailed, first.Status)
	state, err := model.GetLogProjectionBackfillState(t.Context(), mainDB)
	require.NoError(t, err)
	assert.Equal(t, model.LogProjectionMaterializeStatusStarting, state.MaterializeStatus)
	assert.Empty(t, state.MaterializeMutationID)

	second, err := runLogProjectionBackfillBatchForTest(t, mainDB, logDB, 10)
	require.NoError(t, err)
	assert.Equal(t, model.LogProjectionBackfillStatusPending, second.Status)
	assert.Equal(t, "mutation-adopted", second.MaterializeMutationID)
	assert.Equal(t, 1, startCalls, "starting recovery must not issue a second ALTER")
}
