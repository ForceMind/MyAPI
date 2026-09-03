package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupQuotaTaskContextTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	return db
}

func TestQuotaTaskProgressAndLeaseRespectRunDeadlineWhenPoolIsFull(t *testing.T) {
	db := setupQuotaTaskContextTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	heldConnection, err := sqlDB.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = heldConnection.Close(); _ = sqlDB.Close() })
	task := &model.SystemTask{TaskID: "fixture-quota-task", Type: model.SystemTaskTypeChannelQuotaSnapshotSync}
	t.Run("initial progress", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		NewSystemTaskProgressReporterWithContext(ctx, task, "fixture-runner")(0, 2)
		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	})
	t.Run("lease heartbeat", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		require.ErrorIs(t, renewSystemTaskLease(ctx, task, "fixture-runner"), context.DeadlineExceeded)
	})
}

func TestQuotaTaskWritesHaveFiniteBudgetsWithoutChangingLegacyRenewal(t *testing.T) {
	db := setupQuotaTaskContextTestDB(t)
	var deadlines []bool
	stop := errors.New("fixture stops before database update")
	require.NoError(t, db.Callback().Update().Before("gorm:begin_transaction").Register("fixture_capture_quota_write_deadline", func(tx *gorm.DB) {
		deadline, hasDeadline := tx.Statement.Context.Deadline()
		deadlines = append(deadlines, hasDeadline)
		if hasDeadline {
			require.LessOrEqual(t, time.Until(deadline), quotaSystemTaskWriteTimeout)
		}
		tx.AddError(stop)
	}))
	task := &model.SystemTask{TaskID: "fixture-quota-task", Type: model.SystemTaskTypeChannelQuotaSnapshotSync}
	NewSystemTaskProgressReporterWithContext(context.Background(), task, "fixture-runner")(0, 2)
	require.ErrorIs(t, renewSystemTaskLease(context.Background(), task, "fixture-runner"), stop)
	task.Type = model.SystemTaskTypeChannelTest
	require.ErrorIs(t, renewSystemTaskLease(context.Background(), task, "fixture-runner"), stop)
	NewSystemTaskProgressReporter(task, "fixture-runner")(0, 2)
	require.Equal(t, []bool{true, true, false, false}, deadlines)
}

func TestQuotaTaskCompletionCancelsLeaseContext(t *testing.T) {
	var runContext context.Context
	runWithLeaseHeartbeat(&model.SystemTask{TaskID: "fixture-quota-task", Type: model.SystemTaskTypeChannelQuotaSnapshotSync}, "fixture-runner", func(ctx context.Context) {
		runContext = ctx
	})
	require.NotNil(t, runContext)
	require.ErrorIs(t, runContext.Err(), context.Canceled)
}
