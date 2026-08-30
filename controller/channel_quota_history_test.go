package controller

import (
	"math"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/require"
)

func TestQuotaHistoryGranularityAndTimezone(t *testing.T) {
	granularity, err := parseQuotaHistoryGranularity("auto", 0, 7*24*60*60)
	require.NoError(t, err)
	require.Equal(t, quotaHistoryDay, granularity)

	offset, err := parseQuotaHistoryTimezoneOffset("480")
	require.NoError(t, err)
	require.Equal(t, 480, offset)

	timestamp := time.Date(2026, time.January, 2, 23, 45, 0, 0, time.UTC).Unix()
	bucket := quotaHistoryBucketStart(timestamp, quotaHistoryDay, 480)
	require.Equal(t, time.Date(2026, time.January, 2, 16, 0, 0, 0, time.UTC).Unix(), bucket)
}

func TestAggregateQuotaHistoryKeepsSuccessfulObservation(t *testing.T) {
	snapshots := []model.ChannelQuotaSnapshot{
		{ObservedAt: 100, Available: 8, Status: "success"},
		{ObservedAt: 200, Status: "error", ErrorCode: "query_failed"},
		{ObservedAt: 300, Available: 6, Status: "success"},
	}
	aggregated := aggregateQuotaHistorySnapshots(snapshots, quotaHistoryHour, 0)
	require.Len(t, aggregated, 1)
	require.Equal(t, "success", aggregated[0].Status)
	require.Equal(t, float64(6), aggregated[0].Available)
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
