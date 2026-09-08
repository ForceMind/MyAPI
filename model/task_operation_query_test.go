package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTaskOperationQueryTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
		initCol()
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &TaskSubmissionOperation{}))
	return db
}

func TestReadTaskOperationForOwnerScopesEveryLookup(t *testing.T) {
	db := setupTaskOperationQueryTest(t)
	require.NoError(t, db.Exec(
		"INSERT INTO task_submission_operations (public_id, user_id, token_id, http_method, operation_kind, idempotency_key_hash, request_fingerprint, status, reason_code, resolution_source, lock_version, created_at, updated_at, dispatch_started_at, resolved_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"task_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 11, 101, "POST", TaskSubmissionOperationKindVideoCreate,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		TaskSubmissionOperationStatusSucceeded, "private_reason", "private_source", 4, int64(1000), int64(1010), int64(1002), int64(1010),
	).Error)

	sessionView, err := ReadTaskOperationForOwner("task_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 11, nil)
	require.NoError(t, err)
	require.NotNil(t, sessionView)
	assert.Equal(t, TaskSubmissionOperationKindVideoCreate, sessionView.OperationKind)
	assert.Equal(t, TaskSubmissionOperationStatusSucceeded, sessionView.Status)

	tokenID := 101
	tokenView, err := ReadTaskOperationForOwner("task_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 11, &tokenID)
	require.NoError(t, err)
	require.NotNil(t, tokenView)

	wrongTokenID := 102
	wrongToken, err := ReadTaskOperationForOwner("task_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 11, &wrongTokenID)
	require.NoError(t, err)
	assert.Nil(t, wrongToken)

	wrongUser, err := ReadTaskOperationForOwner("task_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 12, nil)
	require.NoError(t, err)
	assert.Nil(t, wrongUser)

	unknown, err := ReadTaskOperationForOwner("task_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 11, nil)
	require.NoError(t, err)
	assert.Nil(t, unknown)
}

func TestReadTaskOperationAPIIdentityUsesPrimaryDatabaseSoftDeleteScope(t *testing.T) {
	db := setupTaskOperationQueryTest(t)
	user := &User{Username: "task-query-user", Password: "unused-password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "task-query-aff"}
	require.NoError(t, db.Create(user).Error)
	allowIPs := "198.51.100.0/24"
	token := &Token{UserId: user.Id, Key: "taskquerykey", Status: common.TokenStatusExhausted, AllowIps: &allowIPs}
	require.NoError(t, db.Create(token).Error)

	identity, err := ReadTaskOperationAPIIdentity(token.Key)
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, token.Id, identity.TokenID)
	assert.Equal(t, user.Id, identity.UserID)
	assert.Equal(t, common.TokenStatusExhausted, identity.TokenStatus)
	assert.Equal(t, common.UserStatusEnabled, identity.UserStatus)
	require.NotNil(t, identity.AllowIPs)
	assert.Equal(t, allowIPs, *identity.AllowIPs)

	require.NoError(t, db.Delete(token).Error)
	identity, err = ReadTaskOperationAPIIdentity(token.Key)
	require.NoError(t, err)
	assert.Nil(t, identity)
}

func TestReadTaskOperationForOwnerRejectsInvalidPublicProjection(t *testing.T) {
	db := setupTaskOperationQueryTest(t)
	publicIDs := []string{
		"task_caaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"task_daaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"task_eaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"task_faaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"task_gaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"task_haaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"task_iaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"task_jaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	rows := []struct {
		status       TaskSubmissionOperationStatus
		createdAt    int64
		updatedAt    int64
		dispatchedAt *int64
		resolvedAt   *int64
	}{
		{status: TaskSubmissionOperationStatusPrepared, createdAt: 0, updatedAt: 10},
		{status: TaskSubmissionOperationStatusPrepared, createdAt: 20, updatedAt: 19},
		{status: TaskSubmissionOperationStatusPrepared, createdAt: 20, updatedAt: 30, dispatchedAt: taskOperationTestTimestamp(25)},
		{status: TaskSubmissionOperationStatusDispatching, createdAt: 20, updatedAt: 30},
		{status: TaskSubmissionOperationStatusDispatching, createdAt: 20, updatedAt: 30, dispatchedAt: taskOperationTestTimestamp(19)},
		{status: TaskSubmissionOperationStatusSucceeded, createdAt: 20, updatedAt: 30, dispatchedAt: taskOperationTestTimestamp(25)},
		{status: TaskSubmissionOperationStatusAccepted, createdAt: 20, updatedAt: 30, dispatchedAt: taskOperationTestTimestamp(25), resolvedAt: taskOperationTestTimestamp(30)},
		{status: TaskSubmissionOperationStatusRejected, createdAt: 20, updatedAt: 30, resolvedAt: taskOperationTestTimestamp(31)},
	}
	for index, row := range rows {
		require.NoError(t, db.Exec(
			"INSERT INTO task_submission_operations (public_id, user_id, token_id, http_method, operation_kind, idempotency_key_hash, request_fingerprint, status, lock_version, created_at, updated_at, dispatch_started_at, resolved_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			publicIDs[index], 31, 301+index, "POST", TaskSubmissionOperationKindVideoCreate,
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			row.status, 1, row.createdAt, row.updatedAt, row.dispatchedAt, row.resolvedAt,
		).Error)
	}

	for _, publicID := range publicIDs {
		operation, err := ReadTaskOperationForOwner(publicID, 31, nil)
		assert.ErrorIs(t, err, ErrTaskOperationQueryUnavailable, publicID)
		assert.Nil(t, operation, publicID)
	}
}

func taskOperationTestTimestamp(value int64) *int64 {
	return &value
}
