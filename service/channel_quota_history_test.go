package service

import (
	"math"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/require"
)

func TestQuotaHistoryBucketAndTimezone(t *testing.T) {
	timestamp := time.Date(2026, time.January, 2, 23, 45, 0, 0, time.UTC).Unix()
	bucket := quotaHistoryBucketStart(timestamp, QuotaHistoryDay, 480)
	require.Equal(t, time.Date(2026, time.January, 2, 16, 0, 0, 0, time.UTC).Unix(), bucket)
	require.Equal(t, int64(120), quotaHistoryBucketStart(179, QuotaHistoryMinute, 0))
	require.Equal(t, int64(0), quotaHistoryBucketStart(299, QuotaHistoryFiveMinutes, 0))
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
	aggregated := aggregateQuotaHistorySnapshots(snapshots, QuotaHistoryHour, 0)
	require.Len(t, aggregated, 1)
	require.Equal(t, "success", aggregated[0].Status)
	require.Equal(t, float64(6), aggregated[0].Available)

	aggregated = aggregateQuotaHistorySnapshots(snapshots[:2], QuotaHistoryHour, 0)
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
	aggregated := aggregateQuotaHistorySnapshots(snapshots, QuotaHistoryHour, 0)
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
	alert := DeriveQuotaHistoryAlert(&model.ChannelQuotaSnapshot{Available: 5, Total: ptrFloat(100), Status: "success"})
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
		alert := DeriveQuotaHistoryAlert(&model.ChannelQuotaSnapshot{Available: test.available, Total: ptrFloat(100), Status: test.status})
		require.Equal(t, test.want, alert.Status)
	}
	alert := DeriveQuotaHistoryAlert(&model.ChannelQuotaSnapshot{Available: 5, Status: "success"})
	require.Equal(t, "unavailable", alert.Status)
}

func TestBuildQuotaHistoryPointsRetainsBucketFailureAndContinuity(t *testing.T) {
	usedTen, usedTwenty, usedThirty := 10.0, 20.0, 30.0
	points := buildQuotaHistoryPoints([]model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 100, Available: 90, Used: &usedTen, Total: ptrFloat(100), Status: "success", Source: "codex_wham_usage_primary"},
		{Id: 2, ObservedAt: 200, Status: "error", ErrorCode: "upstream_http", Source: "codex_wham_usage"},
		{Id: 3, ObservedAt: 300, Available: 70, Used: &usedThirty, Total: ptrFloat(100), Status: "success", Source: "codex_wham_usage_primary"},
		{Id: 4, ObservedAt: 3_700, Available: 80, Used: &usedTwenty, Total: ptrFloat(100), Status: "success", Source: "codex_wham_usage_primary"},
	}, QuotaHistoryHour, 0, "codex_wham_usage_primary")
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

func TestQuotaHistoryConsumptionSurvivesGranularityAndPointCompaction(t *testing.T) {
	snapshots := []model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 3500, Available: 100, Status: "success"},
		{Id: 2, ObservedAt: 3560, Available: 90, Status: "success"},
		{Id: 3, ObservedAt: 3620, Available: 85, Status: "success"},
		{Id: 4, ObservedAt: 3680, Status: "error"},
		{Id: 5, ObservedAt: 3740, Available: 80, Status: "success"},
		{Id: 6, ObservedAt: 3800, Available: 78, Status: "success"},
	}
	for _, granularity := range []QuotaHistoryGranularity{QuotaHistoryRaw, QuotaHistoryMinute, QuotaHistoryFiveMinutes, QuotaHistoryHour, QuotaHistoryDay} {
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

func TestQuotaHistoryAlertRejectsInvalidUsedEvenWithHealthyAvailable(t *testing.T) {
	previousEnabled := common.ChannelQuotaAlertEnabled
	common.ChannelQuotaAlertEnabled = true
	t.Cleanup(func() { common.ChannelQuotaAlertEnabled = previousEnabled })
	for _, used := range []float64{-1, math.NaN(), math.Inf(1)} {
		snapshot := model.ChannelQuotaSnapshot{Status: "success", Available: 90, Total: ptrFloat(100), Used: ptrFloat(used)}
		require.Equal(t, "error", QuotaHistorySnapshotStatus(snapshot))
		alert := DeriveQuotaHistoryAlert(&snapshot)
		require.Equal(t, "unavailable", alert.Status)
		require.Nil(t, alert.RatioPercent)
	}
}

func ptrFloat(value float64) *float64 { return &value }
