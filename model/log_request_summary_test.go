package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRecordedRequestSummaryTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousLogDB, previousLogType := LOG_DB, common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogType)
	})
	return db
}

func TestGetRecordedRequestSummaryDeduplicatesRetriesAndReportsLegacyRows(t *testing.T) {
	db := setupRecordedRequestSummaryTest(t)
	assertRecordedRequestSummaryDatabaseBehavior(t, db)
}

func assertRecordedRequestSummaryDatabaseBehavior(t *testing.T, db *gorm.DB) {
	t.Helper()
	logs := []Log{
		{UserId: 1, CreatedAt: 100, Type: LogTypeError, RequestId: "retry-success"},
		{UserId: 1, CreatedAt: 101, Type: LogTypeConsume, RequestId: "retry-success"},
		{UserId: 1, CreatedAt: 102, Type: LogTypeError, RequestId: "failed"},
		{UserId: 1, CreatedAt: 103, Type: LogTypeError, RequestId: "failed"},
		{UserId: 2, CreatedAt: 104, Type: LogTypeConsume, RequestId: "success"},
		{UserId: 1, CreatedAt: 105, Type: LogTypeConsume},
		{UserId: 1, CreatedAt: 106, Type: LogTypeError},
		{UserId: 1, CreatedAt: 107, Type: LogTypeError},
		{UserId: 1, CreatedAt: 108, Type: LogTypeManage, RequestId: "not-a-relay-outcome"},
		{UserId: 1, CreatedAt: 109, Type: LogTypeConsume, RequestId: "billing-projection", BillingEventID: "billing-event"},
		{UserId: 1, CreatedAt: 111, Type: LogTypeConsume, RequestId: "violation", Content: "Violation fee charged"},
		{UserId: 1, CreatedAt: 112, Type: LogTypeConsume, RequestId: "task-adjustment", Other: `{"task_id":"task-1"}`},
		{UserId: 1, CreatedAt: 200, Type: LogTypeConsume, RequestId: "outside-window"},
	}
	require.NoError(t, db.Create(&logs).Error)
	require.NoError(t, db.Table("logs").Create(map[string]interface{}{
		"user_id": 1, "created_at": 110, "type": LogTypeConsume,
		"request_id": "nullable-billing-event", "billing_event_id": nil,
	}).Error)

	for _, logType := range []int{LogTypeConsume, LogTypeError} {
		require.NoError(t, db.Table("logs").Create(map[string]interface{}{
			"user_id": 1, "created_at": 113, "type": logType,
			"request_id": nil, "billing_event_id": "",
		}).Error)
	}

	summary, err := GetRecordedRequestSummary(100, 199, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(10), summary.TotalRequests)
	assert.Equal(t, int64(5), summary.SuccessfulRequests)
	assert.Equal(t, int64(5), summary.FailedRequests)
	assert.Equal(t, int64(5), summary.IdentifiedRequests)
	assert.Equal(t, int64(5), summary.UnidentifiedLogRows)
	require.NotNil(t, summary.SuccessRate)
	assert.Equal(t, 50.0, *summary.SuccessRate)

	userID := 1
	userSummary, err := GetRecordedRequestSummary(100, 199, &userID)
	require.NoError(t, err)
	assert.Equal(t, int64(9), userSummary.TotalRequests)
	assert.Equal(t, int64(4), userSummary.SuccessfulRequests)
	assert.Equal(t, int64(5), userSummary.FailedRequests)
}

func runRecordedRequestSummaryDatabaseContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&Log{}))
	assertRecordedRequestSummaryDatabaseBehavior(t, db)
}

func TestGetRecordedRequestSummaryReturnsNilRateForNoRequests(t *testing.T) {
	setupRecordedRequestSummaryTest(t)

	summary, err := GetRecordedRequestSummary(1, 2, nil)
	require.NoError(t, err)
	assert.Zero(t, summary.TotalRequests)
	assert.Zero(t, summary.SuccessfulRequests)
	assert.Zero(t, summary.FailedRequests)
	assert.Nil(t, summary.SuccessRate)
}
