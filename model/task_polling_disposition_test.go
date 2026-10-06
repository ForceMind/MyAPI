package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTaskPollingDispositionDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Task{}))
	return db
}

func TestTaskPollingQueryReservesCapacityForDueRetryable(t *testing.T) {
	db := setupTaskPollingDispositionDB(t)
	oldDB := DB
	DB = db
	t.Cleanup(func() { DB = oldDB })
	now := time.Now().Unix()
	for i := 0; i < 8; i++ {
		task := Task{TaskID: fmt.Sprintf("manual-%d", i), Platform: constant.TaskPlatform("kling"), Status: TaskStatusInProgress, Progress: "50%", PollingDisposition: TaskPollingDispositionManual}
		require.NoError(t, db.Create(&task).Error)
	}
	for i := 0; i < 6; i++ {
		task := Task{TaskID: fmt.Sprintf("fresh-%d", i), Platform: constant.TaskPlatform("kling"), Status: TaskStatusInProgress, Progress: "10%"}
		require.NoError(t, db.Create(&task).Error)
	}
	retryable := Task{TaskID: "due-retryable", Platform: constant.TaskPlatform("kling"), Status: TaskStatusInProgress, Progress: "50%", PollingDisposition: TaskPollingDispositionRetryable, NextPollAt: now - 1}
	require.NoError(t, db.Create(&retryable).Error)

	tasks, err := GetAllUnfinishedSyncTasksForPolling(4, now)
	require.NoError(t, err)
	require.Len(t, tasks, 4)
	assert.Equal(t, "fresh-0", tasks[0].TaskID)
	assert.Equal(t, "fresh-1", tasks[1].TaskID)
	assert.Equal(t, "fresh-2", tasks[2].TaskID)
	assert.Equal(t, "due-retryable", tasks[3].TaskID)
}

func TestTaskPollingRetryableBackoffBecomesDue(t *testing.T) {
	db := setupTaskPollingDispositionDB(t)
	oldDB := DB
	DB = db
	t.Cleanup(func() { DB = oldDB })
	now := time.Now().Unix()
	task := Task{TaskID: "retry", Platform: constant.TaskPlatform("kling"), Status: TaskStatusInProgress, Progress: "50%"}
	require.NoError(t, db.Create(&task).Error)

	won, err := MarkTaskPollingRetryable(db, &task, "video_http_5xx", now)
	require.NoError(t, err)
	require.True(t, won)
	assert.Equal(t, TaskPollingDispositionRetryable, task.PollingDisposition)
	assert.Equal(t, now+15, task.NextPollAt)
	before, err := GetAllUnfinishedSyncTasksForPolling(10, now+14)
	require.NoError(t, err)
	assert.Empty(t, before)
	due, err := GetAllUnfinishedSyncTasksForPolling(10, now+15)
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, task.ID, due[0].ID)
	fresh := Task{TaskID: "fresh-alongside-due", Platform: constant.TaskPlatform("kling"), Status: TaskStatusInProgress, Progress: "10%"}
	require.NoError(t, db.Create(&fresh).Error)
	one, err := GetAllUnfinishedSyncTasksForPolling(1, now+15)
	require.NoError(t, err)
	require.Len(t, one, 1)
	assert.Equal(t, task.ID, one[0].ID)
}
