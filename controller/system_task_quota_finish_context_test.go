package controller

import (
	"context"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestQuotaTaskFinishUsesBoundedIndependentWriteContext(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	var deadlines []bool
	require.NoError(t, db.Callback().Update().Before("gorm:begin_transaction").Register("fixture_inspect_finish_context", func(tx *gorm.DB) {
		deadline, present := tx.Statement.Context.Deadline()
		deadlines = append(deadlines, present)
		if present {
			require.NoError(t, tx.Statement.Context.Err())
			require.LessOrEqual(t, time.Until(deadline), channelQuotaPersistenceTimeout)
		}
		tx.AddError(context.DeadlineExceeded)
	}))
	finishSystemTaskHandler(&model.SystemTask{TaskID: "fixture-quota-task", Type: model.SystemTaskTypeChannelQuotaSnapshotSync}, "fixture-runner", model.SystemTaskStatusFailed, nil, context.Canceled)
	finishSystemTaskHandler(&model.SystemTask{TaskID: "fixture-test-task", Type: model.SystemTaskTypeChannelTest}, "fixture-runner", model.SystemTaskStatusFailed, nil, context.Canceled)
	require.Equal(t, []bool{true, false}, deadlines)
}
