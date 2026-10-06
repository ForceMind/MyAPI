package controller

import (
	"fmt"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLogProjectionBackfillHandlerGateOffIsNoOp(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousLegacyGate := common.TaskRecoveryEnabled
	previousNewGate := common.TaskRecoveryNewSubmissionsEnabled
	previousRecoveryGate := common.TaskRecoveryObligationRecoveryEnabled
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.TaskRecoveryEnabled = false
	common.TaskRecoveryNewSubmissionsEnabled = false
	common.TaskRecoveryObligationRecoveryEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.TaskRecoveryEnabled = previousLegacyGate
		common.TaskRecoveryNewSubmissionsEnabled = previousNewGate
		common.TaskRecoveryObligationRecoveryEnabled = previousRecoveryGate
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, db.AutoMigrate(
		&model.SystemTask{},
		&model.SystemTaskLock{},
		&model.LogProjectionBackfillState{},
		&model.Log{},
	))
	task, err := model.CreateSystemTask(model.SystemTaskTypeLogProjectionBackfill, nil, nil)
	require.NoError(t, err)
	claimed, won, err := model.ClaimSystemTask(task.ID, task.Type, "gate-off-runner", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, won)

	logProjectionBackfillHandler{}.Run(t.Context(), claimed, "gate-off-runner")

	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finished)
	assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	var stateRows int64
	require.NoError(t, db.Model(&model.LogProjectionBackfillState{}).Count(&stateRows).Error)
	assert.Zero(t, stateRows, "gate-off execution must not create or mutate migration state")
	var logRows int64
	require.NoError(t, db.Model(&model.Log{}).Count(&logRows).Error)
	assert.Zero(t, logRows)
}

func TestLogProjectionBackfillHandlerResumesAcrossSystemTasksAndReplaysNoOp(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousLegacyGate := common.TaskRecoveryEnabled
	previousNewGate := common.TaskRecoveryNewSubmissionsEnabled
	previousRecoveryGate := common.TaskRecoveryObligationRecoveryEnabled
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.TaskRecoveryEnabled = false
	common.TaskRecoveryNewSubmissionsEnabled = false
	common.TaskRecoveryObligationRecoveryEnabled = true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.TaskRecoveryEnabled = previousLegacyGate
		common.TaskRecoveryNewSubmissionsEnabled = previousNewGate
		common.TaskRecoveryObligationRecoveryEnabled = previousRecoveryGate
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, db.AutoMigrate(
		&model.SystemTask{},
		&model.SystemTaskLock{},
		&model.LogProjectionBackfillState{},
		&model.Log{},
	))
	require.NoError(t, model.EnsureLogProjectionSchemaWithDB(db))
	for id := 1; id <= service.DefaultLogProjectionBackfillBatchSize+1; id++ {
		require.NoError(t, db.Exec(
			"INSERT INTO logs (id, billing_event_id, content, request_id, billing_projection_digest, log_row_key) VALUES (?, ?, 'legacy', ?, '', '')",
			id, fmt.Sprintf("system-event-%03d", id), fmt.Sprintf("system-request-%03d", id),
		).Error)
	}

	run := func(sequence int) *model.SystemTask {
		t.Helper()
		task, createErr := model.CreateSystemTask(model.SystemTaskTypeLogProjectionBackfill, nil, nil)
		require.NoError(t, createErr)
		runnerID := fmt.Sprintf("backfill-runner-%d", sequence)
		claimed, won, claimErr := model.ClaimSystemTask(task.ID, task.Type, runnerID, common.GetTimestamp()+60)
		require.NoError(t, claimErr)
		require.True(t, won)
		logProjectionBackfillHandler{}.Run(t.Context(), claimed, runnerID)
		finished, loadErr := model.GetSystemTaskByTaskID(task.TaskID)
		require.NoError(t, loadErr)
		require.NotNil(t, finished)
		require.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
		return finished
	}

	run(1)
	state, err := model.GetLogProjectionBackfillState(t.Context(), db)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, service.DefaultLogProjectionBackfillBatchSize, state.LastID)
	assert.Equal(t, model.LogProjectionBackfillPhaseRelationalIdentity, state.Phase)

	run(2)
	state, err = model.GetLogProjectionBackfillState(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, service.DefaultLogProjectionBackfillBatchSize+1, state.LastID)
	assert.Equal(t, model.LogProjectionBackfillPhaseRelationalIndexes, state.Phase)

	run(3)
	run(4)
	run(5)
	state, err = model.GetLogProjectionBackfillState(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, model.LogProjectionBackfillStatusCompleted, state.Status)
	assert.Equal(t, model.LogProjectionBackfillPhaseCompleted, state.Phase)
	assert.NotZero(t, state.CompletedAt)
	assert.False(t, logProjectionBackfillHandler{}.Enabled())

	replayed := run(6)
	var replayResult service.LogProjectionBackfillBatchResult
	require.NoError(t, common.UnmarshalJsonStr(replayed.Result, &replayResult))
	assert.True(t, replayResult.NoOp)
	assert.Equal(t, model.LogProjectionBackfillStatusCompleted, replayResult.Status)
	var stateRows int64
	require.NoError(t, db.Model(&model.LogProjectionBackfillState{}).Count(&stateRows).Error)
	assert.Equal(t, int64(1), stateRows)
}
