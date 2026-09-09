package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRecordedRequestSummaryControllerTest(t *testing.T) {
	t.Helper()
	previousLogDB, previousLogType := model.LOG_DB, common.LogDatabaseType()
	previousConsumeEnabled, previousErrorEnabled := common.LogConsumeEnabled, constant.ErrorLogEnabled
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	model.LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogType)
		common.LogConsumeEnabled = previousConsumeEnabled
		constant.ErrorLogEnabled = previousErrorEnabled
	})
}

func TestGetRecordedRequestSelfSummaryScopesUserAndReportsCoverage(t *testing.T) {
	setupRecordedRequestSummaryControllerTest(t)
	common.LogConsumeEnabled = true
	constant.ErrorLogEnabled = false
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{
		{UserId: 7, CreatedAt: 100, Type: model.LogTypeConsume, RequestId: "mine"},
		{UserId: 8, CreatedAt: 100, Type: model.LogTypeError, RequestId: "other"},
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/self/request-summary?start_timestamp=100&end_timestamp=200", nil)
	ctx.Set("id", 7)

	GetRecordedRequestSelfSummary(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			TotalRequests int64    `json:"total_requests"`
			SuccessRate   *float64 `json:"success_rate"`
			Coverage      struct {
				Complete            bool   `json:"complete"`
				ConsumeLogsEnabled  bool   `json:"consume_logs_enabled"`
				ErrorLogsEnabled    bool   `json:"error_logs_enabled"`
				Reason              string `json:"reason"`
				IdentifiedRequests  int64  `json:"identified_requests"`
				UnidentifiedLogRows int64  `json:"unidentified_log_rows"`
			} `json:"coverage"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, int64(1), response.Data.TotalRequests)
	require.NotNil(t, response.Data.SuccessRate)
	assert.Equal(t, 100.0, *response.Data.SuccessRate)
	assert.False(t, response.Data.Coverage.Complete)
	assert.True(t, response.Data.Coverage.ConsumeLogsEnabled)
	assert.False(t, response.Data.Coverage.ErrorLogsEnabled)
	assert.Equal(t, "recorded_logs_only", response.Data.Coverage.Reason)
	assert.Equal(t, int64(1), response.Data.Coverage.IdentifiedRequests)
	assert.Zero(t, response.Data.Coverage.UnidentifiedLogRows)
}

func TestGetRecordedRequestSummaryRejectsUnboundedRange(t *testing.T) {
	setupRecordedRequestSummaryControllerTest(t)
	gin.SetMode(gin.TestMode)

	for _, target := range []string{
		"/api/log/request-summary?end_timestamp=200",
		"/api/log/request-summary?start_timestamp=100&end_timestamp=99",
		"/api/log/request-summary?start_timestamp=100&end_timestamp=100",
		"/api/log/request-summary?start_timestamp=1&end_timestamp=2678402",
		"/api/log/request-summary?start_timestamp=4102444700&end_timestamp=4102444800",
	} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
		GetRecordedRequestSummary(ctx)

		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.False(t, response.Success)
	}
}

func TestGetRecordedRequestSummaryLocalizesDatabaseFailure(t *testing.T) {
	setupRecordedRequestSummaryControllerTest(t)
	require.NoError(t, i18n.Init())
	require.NoError(t, model.LOG_DB.Migrator().DropTable(&model.Log{}))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/request-summary?start_timestamp=100&end_timestamp=200", nil)
	ctx.Request.Header.Set("Accept-Language", "en")
	GetRecordedRequestSummary(ctx)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "Database error, please contact the administrator", response.Message)
}
