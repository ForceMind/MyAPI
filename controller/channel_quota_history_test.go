package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestQuotaHistoryGranularityAndTimezone(t *testing.T) {
	granularity, err := parseQuotaHistoryGranularity("auto", 0, 7*24*60*60)
	require.NoError(t, err)
	require.Equal(t, service.QuotaHistoryFifteenMinutes, granularity)
	granularity, err = parseQuotaHistoryGranularity("auto", 0, 60*60)
	require.NoError(t, err)
	require.Equal(t, service.QuotaHistoryMinute, granularity)
	granularity, err = parseQuotaHistoryGranularity("5m", 0, 60*60)
	require.NoError(t, err)
	require.Equal(t, service.QuotaHistoryFiveMinutes, granularity)
	granularity, err = parseQuotaHistoryGranularity("15m", 0, 6*60*60)
	require.NoError(t, err)
	require.Equal(t, service.QuotaHistoryFifteenMinutes, granularity)
	oneHour, ok := quotaHistoryRangeSeconds("1h")
	require.True(t, ok)
	require.Equal(t, int64(60*60), oneHour)
	sixHours, ok := quotaHistoryRangeSeconds("6h")
	require.True(t, ok)
	require.Equal(t, int64(6*60*60), sixHours)

	offset, err := parseQuotaHistoryTimezoneOffset("480")
	require.NoError(t, err)
	require.Equal(t, 480, offset)
}

func TestParseQuotaHistoryWindowSeconds(t *testing.T) {
	value, err := parseQuotaHistoryWindowSeconds("")
	require.NoError(t, err)
	require.Nil(t, value)
	value, err = parseQuotaHistoryWindowSeconds("18000")
	require.NoError(t, err)
	require.NotNil(t, value)
	require.Equal(t, int64(18000), *value)
	for _, input := range []string{"-1", "five-hours"} {
		_, err = parseQuotaHistoryWindowSeconds(input)
		require.Error(t, err)
	}
}

func TestParseQuotaAnalysisDurationsAreIndependentAndBounded(t *testing.T) {
	rateWindow, err := parseQuotaAnalysisDuration("", 6*60*60, "rate_window")
	require.NoError(t, err)
	require.Equal(t, int64(6*60*60), rateWindow)

	rateWindow, err = parseQuotaAnalysisDuration("1h", 6*60*60, "rate_window")
	require.NoError(t, err)
	require.Equal(t, int64(60*60), rateWindow)

	rateWindow, err = parseQuotaAnalysisDuration("900", 6*60*60, "rate_window")
	require.NoError(t, err)
	require.Equal(t, int64(900), rateWindow)

	for _, value := range []string{"0", "invalid", "7d"} {
		_, err = parseQuotaAnalysisDuration(value, 6*60*60, "rate_window")
		require.Error(t, err, value)
	}

	require.Equal(t, int64(30*60), defaultQuotaAnalysisHalfLife(60*60))
	require.Equal(t, int64(1), defaultQuotaAnalysisHalfLife(1))
}

func setupChannelQuotaHistoryHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousMemoryCache := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCache
	})
	require.NoError(t, db.Create(&model.Channel{Id: 971, Name: "quota-history-test", Key: "test"}).Error)
	return db
}

func getChannelQuotaHistoryTestData(t *testing.T, query string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "971"}}
	ctx.Request = httptest.NewRequest("GET", "/api/channel/971/quota/history?"+query, nil)
	GetChannelQuotaHistory(ctx)
	require.Equal(t, 200, recorder.Code)
	var response struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	require.NotNil(t, response.Data)
	return response.Data
}

func TestGetChannelQuotaHistoryReadsFullRangeBeforePointLimitAndKeepsRawCurrent(t *testing.T) {
	db := setupChannelQuotaHistoryHandlerTestDB(t)
	base := int64(1_700_000_000)
	for index, available := range []float64{100, 80, 70} {
		used := 100 - available
		require.NoError(t, db.Create(&model.ChannelQuotaSnapshot{
			ChannelId: 971, ObservedAt: base + int64(index*3600+17),
			Available: available, Used: &used, Total: ptrFloat(100),
			MetricType: "codex_rate_limit", WindowType: "five_hour",
			Source: "codex_wham_usage_primary", PlanType: "pro", Unit: "percent",
			WindowSeconds: 18000, ResetAt: 2_000_000_000, Status: "success",
		}).Error)
	}
	data := getChannelQuotaHistoryTestData(t, "start=1700000000&end=1700008000&granularity=hour&limit=2")
	require.Equal(t, float64(3), data["raw_observations"])
	require.Equal(t, float64(3), data["available_points"])
	require.Equal(t, float64(2), data["returned_points"])
	require.Equal(t, true, data["source_complete"])
	require.Equal(t, false, data["points_complete"])
	require.Equal(t, false, data["complete"])
	require.Equal(t, true, data["truncated"])
	require.Equal(t, "point_limit", data["truncation_reason"])
	summary, ok := data["summary"].(map[string]any)
	require.True(t, ok)
	require.InDelta(t, 100, summary["start_available"], 0.0001)
	require.InDelta(t, 70, summary["end_available"], 0.0001)
	require.InDelta(t, -30, summary["change"], 0.0001)
	current, ok := data["current"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(base+7_217), current["observed_at"])
	require.Equal(t, "success", current["status"])
	require.NotEmpty(t, data["series_id"])
}

func TestGetChannelQuotaHistoryAnalysisDoesNotDependOnGranularity(t *testing.T) {
	db := setupChannelQuotaHistoryHandlerTestDB(t)
	base := int64(1_700_050_000)
	for index, available := range []float64{100, 94, 88} {
		require.NoError(t, db.Create(&model.ChannelQuotaSnapshot{
			ChannelId: 971, ObservedAt: base + int64(index*60), Available: available,
			MetricType: "balance", WindowType: "none", Source: "provider", Unit: "usd", Status: "success",
		}).Error)
	}
	query := "start=1700050000&end=1700050180&rate_window=180&ewma_half_life=60&limit=10&granularity="
	raw := getChannelQuotaHistoryTestData(t, query+"raw")
	hour := getChannelQuotaHistoryTestData(t, query+"hour")

	require.Equal(t, raw["analysis"], hour["analysis"])
	analysis := raw["analysis"].(map[string]any)
	require.Equal(t, "observed_window", analysis["default_method"])
	require.Equal(t, float64(180), analysis["rate_window_seconds"])
	require.Equal(t, float64(60), analysis["ewma_half_life_seconds"])
	require.Equal(t, float64(1_700_050_180), analysis["as_of"])
	methods := analysis["methods"].(map[string]any)
	observed := methods["observed_window"].(map[string]any)
	require.InDelta(t, 6, observed["rate_per_minute"], 1e-9)
	require.InDelta(t, 360, observed["rate_per_hour"], 1e-9)
	require.InDelta(t, 2.0/3.0, observed["coverage"], 1e-9)
	require.Equal(t, float64(1_700_050_120), observed["observed_at"])
}

func TestGetChannelQuotaHistoryRejectsAnalysisWindowsOutsideSelectedRange(t *testing.T) {
	setupChannelQuotaHistoryHandlerTestDB(t)
	for _, query := range []string{
		"range=1h&rate_window=6h",
		"range=1h&rate_window=1h&ewma_half_life=6h",
	} {
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "id", Value: "971"}}
		ctx.Request = httptest.NewRequest("GET", "/api/channel/971/quota/history?"+query, nil)
		GetChannelQuotaHistory(ctx)
		require.Equal(t, 200, recorder.Code)
		var response struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.False(t, response.Success)
		require.Contains(t, response.Message, "selected range")
	}
}

func TestGetChannelQuotaHistoryUsesLatestGenericFailureForCurrentAndBucket(t *testing.T) {
	db := setupChannelQuotaHistoryHandlerTestDB(t)
	base := int64(1_700_100_000)
	used := 10.0
	require.NoError(t, db.Create(&model.ChannelQuotaSnapshot{
		ChannelId: 971, ObservedAt: base + 10, Available: 90, Used: &used, Total: ptrFloat(100),
		MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary",
		PlanType: "pro", Unit: "percent", WindowSeconds: 18000, Status: "success",
	}).Error)
	require.NoError(t, db.Create(&model.ChannelQuotaSnapshot{
		ChannelId: 971, ObservedAt: base + 20, MetricType: "codex_rate_limit", WindowType: "none",
		Source: "codex_wham_usage", Unit: "percent", Status: "error", ErrorCode: "upstream_http",
	}).Error)
	data := getChannelQuotaHistoryTestData(t, "start=1700100000&end=1700100100&granularity=minute&limit=10")
	require.Equal(t, float64(2), data["raw_observations"])
	current, ok := data["current"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "error", current["status"])
	require.Equal(t, "upstream_http", current["error_code"])
	require.Equal(t, "codex_wham_usage", current["event_source"])
	points, ok := data["points"].([]any)
	require.True(t, ok)
	require.Len(t, points, 1)
	point, ok := points[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "error", point["status"])
	require.Equal(t, float64(1), point["failed_count"])
	require.Equal(t, true, point["continuity_break"])
	quality, ok := data["data_quality"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(1), quality["success_count"])
	require.Equal(t, float64(1), quality["error_count"])
}

func ptrFloat(value float64) *float64 { return &value }

func TestQuotaHistoryResolvedEmptyIdentityDoesNotMixCurrencyOrPlanEvents(t *testing.T) {
	db := setupChannelQuotaHistoryHandlerTestDB(t)
	rows := []model.ChannelQuotaSnapshot{
		{ChannelId: 971, ObservedAt: 100, Available: 100, Status: "success", MetricType: "balance", WindowType: "none", Source: "provider", Unit: "usd", Currency: "USD"},
		{ChannelId: 971, ObservedAt: 160, Available: 90, Status: "success", MetricType: "balance", WindowType: "none", Source: "provider", Unit: "usd"},
		{ChannelId: 971, ObservedAt: 220, Status: "error", MetricType: "balance", WindowType: "none", Source: "provider", Unit: "usd", PlanType: "other-plan"},
	}
	require.NoError(t, db.Create(&rows).Error)
	data := getChannelQuotaHistoryTestData(t, "range=custom&start=1&end=300&granularity=raw")
	require.Equal(t, float64(1), data["raw_observations"])
	current := data["current"].(map[string]any)
	require.Equal(t, "success", current["status"])
	require.Equal(t, float64(160), current["observed_at"])
}
