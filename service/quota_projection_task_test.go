package service

import (
	"context"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestQuotaProjectionSystemTaskRegistrationAndGate(t *testing.T) {
	previous := common.TaskRecoveryObligationRecoveryEnabled
	common.TaskRecoveryObligationRecoveryEnabled = false
	t.Cleanup(func() { common.TaskRecoveryObligationRecoveryEnabled = previous })
	handler := quotaProjectionRecoveryHandler{}
	assert.Equal(t, model.SystemTaskTypeQuotaProjectionRecovery, handler.Type())
	assert.False(t, handler.Enabled())
	common.TaskRecoveryObligationRecoveryEnabled = true
	assert.True(t, handler.Enabled())
	found := false
	for _, registered := range registeredSystemTaskHandlers() {
		if registered.Type() == model.SystemTaskTypeQuotaProjectionRecovery {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestTaskEngineRunnerProcessesDueQuotaProjectionObligation(t *testing.T) {
	db := setupRecoveryTestDB(t)
	fixture := newRecoveryTestFixture(t, db, "projection-runner", 100, 100)
	var receipt *model.UserQuotaMutationReceipt
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		receipt, err = model.MutateUserQuotaAuthoritative(tx, model.UserQuotaMutationInput{
			UserID: fixture.User.Id, Delta: -10, MutationType: "test",
			BusinessEventKey: "projection:runner:1", ReasonCode: "test",
		})
		return err
	}))

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousEnabled, previousRDB, previousGate := common.RedisEnabled, common.RDB, common.TaskRecoveryObligationRecoveryEnabled
	common.RedisEnabled, common.RDB, common.TaskRecoveryObligationRecoveryEnabled = true, client, true
	t.Cleanup(func() {
		_ = client.Close()
		common.RedisEnabled, common.RDB, common.TaskRecoveryObligationRecoveryEnabled = previousEnabled, previousRDB, previousGate
	})

	runner := NewTaskEngineRunner(TaskEngineConfig{WorkerID: "projection-engine", BatchSize: 10, LogDB: db}, db)
	report := runner.RunOnce(context.Background())
	assert.Equal(t, 1, report.QuotaProjectionProcessed)
	assert.False(t, report.HasErrors(), report.Errors)
	var obligation model.QuotaProjectionObligation
	require.NoError(t, db.Where("receipt_kind = ? AND receipt_id = ?", "user", receipt.ID).First(&obligation).Error)
	assert.Equal(t, string(model.QuotaProjectionObligationStateApplied), obligation.State)
}
