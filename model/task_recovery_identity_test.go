package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestTaskRecoveryInitDBRejectsInvalidSecretBeforeOpeningDatabase(t *testing.T) {
	t.Setenv("TASK_RECOVERY_ENABLED", "true")
	t.Setenv("SQL_DSN", "local")
	t.Setenv("LOG_SQL_DSN", "")
	previousDB, previousPath := DB, common.SQLitePath
	previousGate, previousMaster := common.TaskRecoveryEnabled, common.IsMasterNode
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SQLitePath = filepath.Join(t.TempDir(), "must-not-open.db")
	common.TaskRecoveryEnabled, common.IsMasterNode = false, false
	t.Cleanup(func() {
		if DB != nil && DB != previousDB {
			if connection, err := DB.DB(); err == nil {
				_ = connection.Close()
			}
		}
		DB, common.SQLitePath = previousDB, previousPath
		common.TaskRecoveryEnabled, common.IsMasterNode = previousGate, previousMaster
		common.SetMainDatabaseType(previousMainType)
		common.SetLogDatabaseType(previousLogType)
	})
	for _, secret := range []string{"", strings.Repeat("a", 63), strings.Repeat("x", 64)} {
		t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", secret)
		require.ErrorIs(t, InitDB(), common.ErrTaskRecoveryIdempotencySecret)
		assert.Same(t, previousDB, DB)
		_, err := os.Stat(common.SQLitePath)
		assert.True(t, os.IsNotExist(err), "invalid recovery configuration must fail before opening the database")
	}
}

// The shared contract is called by the SQLite and isolated CI SQL fixtures.
func runB2TaskRecoveryIdentityContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	require.NoError(t, EnsureTaskRecoveryIdentity(db))
	require.NoError(t, EnsureTaskRecoveryIdentity(db))
	var binding TaskRecoveryIdentity
	require.NoError(t, db.First(&binding, 1).Error)
	operation := newB2SubmissionOperation(t, 601, "POST", "video", "database-key-binding", `{}`)
	require.NoError(t, db.Create(operation).Error)
	var before int64
	require.NoError(t, db.Model(&TaskSubmissionOperation{}).Count(&before).Error)

	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("2b", 32))
	require.ErrorIs(t, EnsureTaskRecoveryIdentity(db), ErrTaskRecoveryIdentityMismatch)
	replayWithWrongKey := newB2SubmissionOperation(t, 601, "POST", "video", "database-key-binding", `{}`)
	require.ErrorIs(t, db.Create(replayWithWrongKey).Error, ErrTaskRecoveryIdentityMismatch)
	var after int64
	require.NoError(t, db.Model(&TaskSubmissionOperation{}).Count(&after).Error)
	assert.Equal(t, before, after)
	var unchanged TaskRecoveryIdentity
	require.NoError(t, db.First(&unchanged, 1).Error)
	assert.Equal(t, binding, unchanged)

	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	require.NoError(t, EnsureTaskRecoveryIdentity(db))
	loaded, err := FindTaskSubmissionOperationByIdempotencyScope(db, TaskSubmissionIdempotencyScope{
		TokenID: operation.TokenID, HTTPMethod: operation.HTTPMethod,
		OperationKind: operation.OperationKind, IdempotencyKeyHash: operation.IdempotencyKeyHash,
	})
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, operation.PublicID, loaded.PublicID)
}

func TestTaskRecoveryIdentityPersistsAfterReopenAndRejectsWrongKey(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	fixturePath := filepath.Join(t.TempDir(), "identity.db")
	db, err := gorm.Open(sqlite.Open(fixturePath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	firstConnection, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = firstConnection.Close() })
	require.NoError(t, db.AutoMigrate(&TaskRecoveryIdentity{}, &TaskSubmissionOperation{}))
	require.NoError(t, EnsureTaskRecoveryIdentity(db))
	operation := newB2SubmissionOperation(t, 501, "POST", "video", "restart-client-key", `{}`)
	require.NoError(t, db.Create(operation).Error)

	// Close the original connection and reopen the persisted store. Resolving
	// a replay cannot depend on a connection or process-local identity cache.
	require.NoError(t, firstConnection.Close())
	fresh, err := gorm.Open(sqlite.Open(fixturePath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	secondConnection, err := fresh.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, secondConnection.Close()) })
	require.NoError(t, EnsureTaskRecoveryIdentity(fresh))
	scope := TaskSubmissionIdempotencyScope{
		TokenID: 501, HTTPMethod: "POST", OperationKind: "video", IdempotencyKeyHash: operation.IdempotencyKeyHash,
	}
	loaded, err := FindTaskSubmissionOperationByIdempotencyScope(fresh, scope)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, operation.PublicID, loaded.PublicID)

	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("2b", 32))
	require.ErrorIs(t, EnsureTaskRecoveryIdentity(fresh), ErrTaskRecoveryIdentityMismatch)
	second := newB2SubmissionOperation(t, 501, "POST", "video", "restart-client-key", `{}`)
	require.ErrorIs(t, fresh.Create(second).Error, ErrTaskRecoveryIdentityMismatch)
	var count int64
	require.NoError(t, fresh.Model(&TaskSubmissionOperation{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", "")
	require.Error(t, EnsureTaskRecoveryIdentity(fresh))
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	require.NoError(t, EnsureTaskRecoveryIdentity(fresh))
}

func TestTaskRecoveryIdentityRefusesAmbiguousExistingOperations(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db := openB2SubmissionSQLite(t)
	require.NoError(t, db.AutoMigrate(&TaskRecoveryIdentity{}, &TaskSubmissionOperation{}))
	// Simulate a historical row from before identity binding, without invoking
	// the new application constructor. It must not be assigned a guessed key.
	require.NoError(t, db.Exec("INSERT INTO task_submission_operations (public_id, user_id, token_id, http_method, operation_kind, idempotency_key_hash, request_fingerprint, status, lock_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"task_"+strings.Repeat("a", 32), 11, 501, "POST", "video", strings.Repeat("a", 64), strings.Repeat("b", 64), "prepared", 1).Error)
	require.ErrorIs(t, EnsureTaskRecoveryIdentity(db), ErrTaskRecoveryIdentityMissing)
	var count int64
	require.NoError(t, db.Model(&TaskRecoveryIdentity{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestTaskRecoveryIdentityCannotBeUpdatedOrDeleted(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db := openB2SubmissionSQLite(t)
	require.NoError(t, db.AutoMigrate(&TaskRecoveryIdentity{}, &TaskSubmissionOperation{}))
	require.NoError(t, EnsureTaskRecoveryIdentity(db))
	var original TaskRecoveryIdentity
	require.NoError(t, db.First(&original, 1).Error)
	invalidTime := original
	invalidTime.CreatedAt = -1
	require.ErrorIs(t, db.Create(&invalidTime).Error, ErrTaskRecoveryIdentityMismatch)
	changed := original
	changed.KeyVerifier = strings.Repeat("f", 64)
	require.ErrorIs(t, db.Save(&changed).Error, ErrTaskRecoveryIdentityImmutable)
	require.ErrorIs(t, db.Model(&original).Updates(map[string]interface{}{"key_verifier": changed.KeyVerifier}).Error, ErrTaskRecoveryIdentityImmutable)
	require.ErrorIs(t, db.Delete(&original).Error, ErrTaskRecoveryIdentityImmutable)
	var actual TaskRecoveryIdentity
	require.NoError(t, db.First(&actual, 1).Error)
	assert.Equal(t, original, actual)
}
