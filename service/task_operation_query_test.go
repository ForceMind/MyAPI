package service

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGetTaskOperationBuildsOnlyPublicContract(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.TaskSubmissionOperation{}))
	dispatchedAt, resolvedAt := int64(201), int64(250)
	require.NoError(t, db.Exec(
		"INSERT INTO task_submission_operations (public_id, user_id, token_id, http_method, operation_kind, idempotency_key_hash, request_fingerprint, status, reason_code, resolution_source, lock_version, created_at, updated_at, dispatch_started_at, resolved_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"task_cccccccccccccccccccccccccccccccc", 21, 201, "POST", model.TaskSubmissionOperationKindVideoRemix,
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		model.TaskSubmissionOperationStatusFailed, "must_not_escape", "must_not_escape", 5, int64(200), int64(250), dispatchedAt, resolvedAt,
	).Error)

	tokenID := 201
	response, err := GetTaskOperation("task_cccccccccccccccccccccccccccccccc", 21, &tokenID)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, dto.TaskOperationObject, response.Object)
	assert.Equal(t, model.TaskSubmissionOperationKindVideoRemix, response.Kind)
	assert.Equal(t, string(model.TaskSubmissionOperationStatusFailed), response.Status)
	assert.Equal(t, &dispatchedAt, response.DispatchStartedAt)
	assert.Equal(t, &resolvedAt, response.ResolvedAt)

	encoded, err := common.Marshal(response)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "must_not_escape")
	assert.NotContains(t, string(encoded), "token_id")
	assert.NotContains(t, string(encoded), "user_id")
	assert.NotContains(t, string(encoded), "fingerprint")

	require.NoError(t, db.Exec(
		"INSERT INTO task_submission_operations (public_id, user_id, token_id, http_method, operation_kind, idempotency_key_hash, request_fingerprint, status, lock_version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"task_dddddddddddddddddddddddddddddddd", 21, 201, "POST", model.TaskSubmissionOperationKindVideoCreate,
		"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		model.TaskSubmissionOperationStatusDispatching, 1, int64(300), int64(310),
	).Error)
	invalid, err := GetTaskOperation("task_dddddddddddddddddddddddddddddddd", 21, &tokenID)
	assert.ErrorIs(t, err, ErrTaskOperationUnavailable)
	assert.Nil(t, invalid)
}
