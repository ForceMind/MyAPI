package controller

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestQuotaHistoryGranularityAndTimezone(t *testing.T) {
	granularity, err := parseQuotaHistoryGranularity("auto", 0, 7*24*60*60)
	require.NoError(t, err)
	require.Equal(t, quotaHistoryFifteenMinutes, granularity)
	granularity, err = parseQuotaHistoryGranularity("auto", 0, 60*60)
	require.NoError(t, err)
	require.Equal(t, quotaHistoryMinute, granularity)
	granularity, err = parseQuotaHistoryGranularity("5m", 0, 60*60)
	require.NoError(t, err)
	require.Equal(t, quotaHistoryFiveMinutes, granularity)
	granularity, err = parseQuotaHistoryGranularity("15m", 0, 6*60*60)
	require.NoError(t, err)
	require.Equal(t, quotaHistoryFifteenMinutes, granularity)
	oneHour, ok := quotaHistoryRangeSeconds("1h")
	require.True(t, ok)
	require.Equal(t, int64(60*60), oneHour)
	sixHours, ok := quotaHistoryRangeSeconds("6h")
	require.True(t, ok)
	require.Equal(t, int64(6*60*60), sixHours)

	offset, err := parseQuotaHistoryTimezoneOffset("480")
	require.NoError(t, err)
	require.Equal(t, 480, offset)

	timestamp := time.Date(2026, time.January, 2, 23, 45, 0, 0, time.UTC).Unix()
	bucket := quotaHistoryBucketStart(timestamp, quotaHistoryDay, 480)
	require.Equal(t, time.Date(2026, time.January, 2, 16, 0, 0, 0, time.UTC).Unix(), bucket)
	require.Equal(t, int64(120), quotaHistoryBucketStart(179, quotaHistoryMinute, 0))
	require.Equal(t, int64(0), quotaHistoryBucketStart(299, quotaHistoryFiveMinutes, 0))
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

func TestResolveQuotaHistorySeriesFilterFillsMissingDimensions(t *testing.T) {
	windowSeconds := int64(18000)
	filter := resolveQuotaHistorySeriesFilter(
		model.ChannelQuotaSnapshotQuery{MetricType: "rate_limit"},
		model.ChannelQuotaSnapshot{
			MetricType:    "rate_limit",
			WindowType:    "five_hour",
			Source:        "codex",
			PlanType:      "team",
			Unit:          "percent",
			Currency:      "quota",
			WindowSeconds: 18000,
		},
	)
	require.Equal(t, "rate_limit", filter.MetricType)
	require.Equal(t, "five_hour", filter.WindowType)
	require.Equal(t, "codex", filter.Source)
	require.Equal(t, "team", filter.PlanType)
	require.Equal(t, "percent", filter.Unit)
	require.Equal(t, "quota", filter.Currency)
	require.NotNil(t, filter.WindowSeconds)
	require.Equal(t, windowSeconds, *filter.WindowSeconds)
}

func TestAggregateQuotaHistoryKeepsLatestFailureVisible(t *testing.T) {
	snapshots := []model.ChannelQuotaSnapshot{
		{ObservedAt: 100, Available: 8, Status: "success"},
		{ObservedAt: 200, Status: "error", ErrorCode: "query_failed"},
		{ObservedAt: 300, Available: 6, Status: "success"},
	}
	aggregated := aggregateQuotaHistorySnapshots(snapshots, quotaHistoryHour, 0)
	require.Len(t, aggregated, 1)
	require.Equal(t, "success", aggregated[0].Status)
	require.Equal(t, float64(6), aggregated[0].Available)

	aggregated = aggregateQuotaHistorySnapshots(snapshots[:2], quotaHistoryHour, 0)
	require.Len(t, aggregated, 1)
	require.Equal(t, "error", aggregated[0].Status)
	require.Equal(t, "query_failed", aggregated[0].ErrorCode)
}

func TestAggregateQuotaHistoryKeepsIndependentSeriesInSameBucket(t *testing.T) {
	snapshots := []model.ChannelQuotaSnapshot{
		{ObservedAt: 100, Available: 8, Status: "success", MetricType: "balance", WindowType: "none", Source: "provider", PlanType: "standard", Unit: "usd", Currency: "USD"},
		{ObservedAt: 200, Available: 7, Status: "success", MetricType: "balance", WindowType: "none", Source: "provider", PlanType: "standard", Unit: "usd", Currency: "USD"},
		{ObservedAt: 100, Available: 80, Status: "success", MetricType: "rate_limit", WindowType: "five_hour", Source: "provider", PlanType: "pro", Unit: "percent", WindowSeconds: 18000},
		{ObservedAt: 200, Available: 70, Status: "success", MetricType: "rate_limit", WindowType: "five_hour", Source: "provider", PlanType: "pro", Unit: "percent", WindowSeconds: 18000},
	}
	aggregated := aggregateQuotaHistorySnapshots(snapshots, quotaHistoryHour, 0)
	require.Len(t, aggregated, 2)
	seen := map[string]float64{}
	for _, snapshot := range aggregated {
		seen[snapshot.PlanType] = snapshot.Available
	}
	require.Equal(t, float64(7), seen["standard"])
	require.Equal(t, float64(70), seen["pro"])
}

func TestDeriveQuotaHistoryMetricsDeclineAndForecast(t *testing.T) {
	now := time.Now().Unix()
	metrics := deriveQuotaHistoryMetrics([]model.ChannelQuotaSnapshot{
		{ObservedAt: now - 2*24*60*60, Available: 100, Status: "success"},
		{ObservedAt: now - 24*60*60, Available: 80, Status: "success"},
		{ObservedAt: now, Available: 60, Status: "success"},
	})
	require.NotNil(t, metrics.DropRatePerDay)
	require.InDelta(t, -20.0, *metrics.DropRatePerDay, 0.0001)
	require.NotNil(t, metrics.ForecastZeroAt)
	require.InDelta(t, float64(now+3*24*60*60), float64(*metrics.ForecastZeroAt), 2)
	require.Equal(t, "high", metrics.ForecastConfidence)
	require.Equal(t, 3, metrics.DataQuality.SuccessCount)
	require.Equal(t, int64(2*24*60*60), metrics.DataQuality.SpanSeconds)
}

func TestDeriveQuotaHistoryMetricsHandlesFailuresInvalidValuesAndShortSpan(t *testing.T) {
	now := time.Now().Unix()
	metrics := deriveQuotaHistoryMetrics([]model.ChannelQuotaSnapshot{
		{ObservedAt: now - 30*60, Available: 100, Status: "success"},
		{ObservedAt: now - 15*60, Available: math.NaN(), Status: "success"},
		{ObservedAt: now, Status: "error", ErrorCode: "query_failed"},
	})
	require.Nil(t, metrics.DropRatePerDay)
	require.Nil(t, metrics.ForecastZeroAt)
	require.Equal(t, "insufficient", metrics.ForecastConfidence)
	require.Equal(t, 1, metrics.DataQuality.SuccessCount)
	require.Equal(t, 1, metrics.DataQuality.InvalidCount)
	require.Equal(t, 1, metrics.DataQuality.ErrorCount)
	require.Equal(t, int64(0), metrics.DataQuality.SpanSeconds)
}

func TestDeriveQuotaHistoryMetricsSeparatesUnsupportedSamples(t *testing.T) {
	now := time.Now().Unix()
	metrics := deriveQuotaHistoryMetrics([]model.ChannelQuotaSnapshot{
		{ObservedAt: now - 30*60, Status: "unsupported"},
		{ObservedAt: now, Status: "error", ErrorCode: "query_failed"},
	})
	require.Equal(t, 1, metrics.DataQuality.UnsupportedCount)
	require.Equal(t, 1, metrics.DataQuality.ErrorCount)
}

func TestDeriveQuotaHistoryMetricsDoesNotCrossQuotaReset(t *testing.T) {
	now := time.Now().Unix()
	metrics := deriveQuotaHistoryMetrics([]model.ChannelQuotaSnapshot{
		{ObservedAt: now - 3*24*60*60, Available: 100, Status: "success", ResetAt: now - 24*60*60},
		{ObservedAt: now - 2*24*60*60, Available: 80, Status: "success", ResetAt: now - 24*60*60},
		{ObservedAt: now - 24*60*60, Available: 200, Status: "success", ResetAt: now + 7*24*60*60},
		{ObservedAt: now, Available: 180, Status: "success", ResetAt: now + 7*24*60*60},
	})
	require.Equal(t, 1, metrics.DataQuality.ResetBoundaries)
	require.NotNil(t, metrics.DropRatePerDay)
	require.InDelta(t, -20.0, *metrics.DropRatePerDay, 0.0001)
	// The pre-reset samples span two days but must not raise confidence for the
	// post-reset two-point estimate.
	require.Equal(t, "low", metrics.ForecastConfidence)
}

func TestDeriveQuotaHistoryMetricsDoesNotForecastGrowthOrDepletedBalance(t *testing.T) {
	now := time.Now().Unix()
	for _, snapshots := range [][]model.ChannelQuotaSnapshot{
		{{ObservedAt: now - 24*60*60, Available: 10, Status: "success"}, {ObservedAt: now, Available: 20, Status: "success"}},
		{{ObservedAt: now - 24*60*60, Available: 10, Status: "success"}, {ObservedAt: now, Available: 0, Status: "success"}},
	} {
		metrics := deriveQuotaHistoryMetrics(snapshots)
		require.Nil(t, metrics.ForecastZeroAt)
	}
}

func TestDeriveQuotaHistoryAlertIsDisabledByDefault(t *testing.T) {
	originalEnabled := common.ChannelQuotaAlertEnabled
	defer func() { common.ChannelQuotaAlertEnabled = originalEnabled }()
	common.ChannelQuotaAlertEnabled = false
	alert := deriveQuotaHistoryAlert(&model.ChannelQuotaSnapshot{Available: 5, Total: ptrFloat(100), Status: "success"})
	require.False(t, alert.Enabled)
	require.Equal(t, "disabled", alert.Status)
	require.Nil(t, alert.RatioPercent)
}

func TestDeriveQuotaHistoryAlertThresholdsAndUnavailable(t *testing.T) {
	originalEnabled := common.ChannelQuotaAlertEnabled
	originalWarning := common.ChannelQuotaAlertWarningPercent
	originalCritical := common.ChannelQuotaAlertCriticalPercent
	defer func() {
		common.ChannelQuotaAlertEnabled = originalEnabled
		common.ChannelQuotaAlertWarningPercent = originalWarning
		common.ChannelQuotaAlertCriticalPercent = originalCritical
	}()
	common.ChannelQuotaAlertEnabled = true
	common.ChannelQuotaAlertWarningPercent = 20
	common.ChannelQuotaAlertCriticalPercent = 10
	for _, test := range []struct {
		available float64
		status    string
		want      string
	}{
		{available: 5, status: "success", want: "critical"},
		{available: 15, status: "success", want: "warning"},
		{available: 50, status: "success", want: "healthy"},
		{available: 50, status: "error", want: "unavailable"},
	} {
		alert := deriveQuotaHistoryAlert(&model.ChannelQuotaSnapshot{Available: test.available, Total: ptrFloat(100), Status: test.status})
		require.Equal(t, test.want, alert.Status)
	}
	alert := deriveQuotaHistoryAlert(&model.ChannelQuotaSnapshot{Available: 5, Status: "success"})
	require.Equal(t, "unavailable", alert.Status)
}

func TestBuildQuotaHistoryPointsRetainsBucketFailureAndContinuity(t *testing.T) {
	usedTen, usedTwenty, usedThirty := 10.0, 20.0, 30.0
	points := buildQuotaHistoryPoints([]model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 100, Available: 90, Used: &usedTen, Total: ptrFloat(100), Status: "success", Source: "codex_wham_usage_primary"},
		{Id: 2, ObservedAt: 200, Status: "error", ErrorCode: "upstream_http", Source: "codex_wham_usage"},
		{Id: 3, ObservedAt: 300, Available: 70, Used: &usedThirty, Total: ptrFloat(100), Status: "success", Source: "codex_wham_usage_primary"},
		{Id: 4, ObservedAt: 3_700, Available: 80, Used: &usedTwenty, Total: ptrFloat(100), Status: "success", Source: "codex_wham_usage_primary"},
	}, quotaHistoryHour, 0, "codex_wham_usage_primary")
	require.Len(t, points, 2)
	require.Equal(t, "success", points[0].Status)
	require.Equal(t, 3, points[0].SampleCount)
	require.Equal(t, 1, points[0].FailedCount)
	require.True(t, points[0].ContinuityBreak)
	require.NotNil(t, points[0].Available)
	require.Equal(t, 70.0, *points[0].Available)
	require.Equal(t, "success", points[1].Status, "the next bucket remains independently observable")
}

func TestDeriveQuotaHistorySummaryAndConsumptionDoNotCrossFailureOrReset(t *testing.T) {
	usedTen, usedTwenty, usedThirty, usedFifteen := 10.0, 20.0, 30.0, 15.0
	snapshots := []model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 100, Available: 90, Used: &usedTen, Total: ptrFloat(100), ResetAt: 1_000, Status: "success"},
		{Id: 2, ObservedAt: 160, Available: 80, Used: &usedTwenty, Total: ptrFloat(100), ResetAt: 1_000, Status: "success"},
		{Id: 3, ObservedAt: 180, Status: "error", ErrorCode: "upstream_http"},
		{Id: 4, ObservedAt: 220, Available: 70, Used: &usedThirty, Total: ptrFloat(100), ResetAt: 1_000, Status: "success"},
		{Id: 5, ObservedAt: 280, Available: 90, Used: &usedTen, Total: ptrFloat(100), ResetAt: 2_000, Status: "success"},
		{Id: 6, ObservedAt: 340, Available: 85, Used: &usedFifteen, Total: ptrFloat(100), ResetAt: 2_000, Status: "success"},
	}
	metrics := deriveQuotaHistoryMetrics(snapshots)
	summary := deriveQuotaHistorySummary(snapshots, metrics, "percent")
	require.NotNil(t, summary)
	require.NotNil(t, summary.Used)
	require.NotNil(t, summary.Total)
	// The available/used/total summaries represent only the latest continuous
	// segment after the provider reset, rather than crossing the earlier error.
	require.InDelta(t, 90, *summary.Available.Start, 0.0001)
	require.InDelta(t, 85, *summary.Available.End, 0.0001)
	require.InDelta(t, 10, *summary.Used.Start, 0.0001)
	require.InDelta(t, 15, *summary.Used.End, 0.0001)
	require.InDelta(t, 100, *summary.Total.Start, 0.0001)
	require.InDelta(t, 100, *summary.Total.End, 0.0001)
	require.NotNil(t, summary.Consumption.Observed)
	require.InDelta(t, 15, *summary.Consumption.Observed, 0.0001)
	require.Equal(t, "used", summary.Consumption.Basis)
	require.Equal(t, 2, summary.Consumption.PairCount)
	require.Equal(t, 1, summary.Consumption.InterruptedCount)
	require.Equal(t, 1, summary.Consumption.ResetBoundaries)
	require.NotNil(t, summary.Consumption.PeakRatePerMinute)
	require.InDelta(t, 10, *summary.Consumption.PeakRatePerMinute, 0.0001)
	require.NotNil(t, summary.Consumption.PeakRateObservedAt)
	require.Equal(t, int64(160), *summary.Consumption.PeakRateObservedAt)
	// The forecasting segment also cannot bridge the error marker.
	require.Equal(t, 1, metrics.DataQuality.ResetBoundaries)
	require.Equal(t, 1, metrics.DataQuality.ErrorCount)
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

func TestQuotaHistoryConsumptionSurvivesGranularityAndPointCompaction(t *testing.T) {
	snapshots := []model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 3500, Available: 100, Status: "success"},
		{Id: 2, ObservedAt: 3560, Available: 90, Status: "success"},
		{Id: 3, ObservedAt: 3620, Available: 85, Status: "success"},
		{Id: 4, ObservedAt: 3680, Status: "error"},
		{Id: 5, ObservedAt: 3740, Available: 80, Status: "success"},
		{Id: 6, ObservedAt: 3800, Available: 78, Status: "success"},
	}
	for _, granularity := range []quotaHistoryGranularity{quotaHistoryRaw, quotaHistoryMinute, quotaHistoryFiveMinutes, quotaHistoryHour, quotaHistoryDay} {
		t.Run(string(granularity), func(t *testing.T) {
			points := buildQuotaHistoryPoints(snapshots, granularity, 0, "")
			compressed, _ := limitQuotaHistoryPoints(points, 1)
			require.Len(t, compressed, 1)
			require.NotNil(t, compressed[0].Consumption)
			require.InDelta(t, 17, *compressed[0].Consumption, 1e-9)
			require.Equal(t, int64(180), compressed[0].ObservedSeconds)
			require.Equal(t, 3, compressed[0].IntervalCount)
			require.InDelta(t, 10, *compressed[0].PeakRatePerMinute, 1e-9)
			require.Equal(t, 1, compressed[0].FailedCount)
			require.True(t, compressed[0].ContinuityBreak)
		})
	}
}

func TestQuotaHistoryLatestFailureDoesNotEraseObservedSummary(t *testing.T) {
	snapshots := []model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 100, Available: 90, Status: "success"},
		{Id: 2, ObservedAt: 160, Available: 80, Status: "success"},
		{Id: 3, ObservedAt: 220, Status: "error"},
	}
	summary := deriveQuotaHistorySummary(snapshots, deriveQuotaHistoryMetrics(snapshots), "percent")
	require.NotNil(t, summary)
	require.Equal(t, 10.0, *summary.Consumption.Observed)
	require.Equal(t, int64(60), summary.Consumption.ObservedSeconds)
	require.Equal(t, 1, summary.Consumption.InterruptedCount)
	require.Equal(t, "error", quotaHistoryCurrent(&snapshots[2], "")["status"])
}

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

func TestQuotaHistoryAlertRejectsInvalidUsedEvenWithHealthyAvailable(t *testing.T) {
	previousEnabled := common.ChannelQuotaAlertEnabled
	common.ChannelQuotaAlertEnabled = true
	t.Cleanup(func() { common.ChannelQuotaAlertEnabled = previousEnabled })
	for _, used := range []float64{-1, math.NaN(), math.Inf(1)} {
		snapshot := model.ChannelQuotaSnapshot{Status: "success", Available: 90, Total: ptrFloat(100), Used: ptrFloat(used)}
		require.Equal(t, "error", quotaHistorySnapshotStatus(snapshot))
		alert := deriveQuotaHistoryAlert(&snapshot)
		require.Equal(t, "unavailable", alert.Status)
		require.Nil(t, alert.RatioPercent)
	}
}
