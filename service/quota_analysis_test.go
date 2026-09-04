package service

import (
	"math"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeQuotaConsumptionCalculatesAllRateMethodsFromRawIntervals(t *testing.T) {
	result := DeriveQuotaConsumption([]model.ChannelQuotaSnapshot{
		{Id: 1, ChannelId: 7, ObservedAt: 1_000, Available: 100, Status: "success"},
		{Id: 2, ChannelId: 7, ObservedAt: 1_060, Available: 94, Status: "success"},
		{Id: 3, ChannelId: 7, ObservedAt: 1_180, Available: 88, Status: "success"},
	}, "percent")

	analysis := AnalyzeQuotaConsumption(result, 1_000, 1_180, 60)

	require.Equal(t, "observed_window", analysis.DefaultMethod)
	require.Equal(t, int64(180), analysis.RateWindowSeconds)
	require.Equal(t, int64(60), analysis.EWMAHalfLifeSeconds)
	require.Equal(t, int64(1_180), analysis.AsOf)
	require.True(t, analysis.Complete)

	require.InDelta(t, 3, *analysis.Methods.LatestInterval.RatePerMinute, 1e-9)
	require.InDelta(t, 180, *analysis.Methods.LatestInterval.RatePerHour, 1e-9)
	require.InDelta(t, 2.0/3.0, analysis.Methods.LatestInterval.Coverage, 1e-9)
	require.Equal(t, int64(120), analysis.Methods.LatestInterval.ObservedSeconds)
	require.Equal(t, 1, analysis.Methods.LatestInterval.IntervalCount)
	require.Equal(t, int64(1_180), *analysis.Methods.LatestInterval.ObservedAt)

	require.InDelta(t, 4, *analysis.Methods.ObservedWindow.RatePerMinute, 1e-9)
	require.InDelta(t, 240, *analysis.Methods.ObservedWindow.RatePerHour, 1e-9)
	require.InDelta(t, 1, analysis.Methods.ObservedWindow.Coverage, 1e-9)
	require.Equal(t, int64(180), analysis.Methods.ObservedWindow.ObservedSeconds)
	require.Equal(t, 2, analysis.Methods.ObservedWindow.IntervalCount)

	// Continuous exponential integration weights the complete time covered by
	// each interval rather than treating its endpoint as an instantaneous point.
	require.InDelta(t, 24.0/7.0, *analysis.Methods.EWMA.RatePerMinute, 1e-9)
	require.InDelta(t, 1440.0/7.0, *analysis.Methods.EWMA.RatePerHour, 1e-9)
	require.InDelta(t, 1, analysis.Methods.EWMA.Coverage, 1e-9)
}

func TestAnalyzeQuotaConsumptionUsesOnlyLatestContinuousSegment(t *testing.T) {
	result := DeriveQuotaConsumption([]model.ChannelQuotaSnapshot{
		{Id: 1, ChannelId: 7, ObservedAt: 100, Available: 100, ResetAt: 220, Status: "success"},
		{Id: 2, ChannelId: 7, ObservedAt: 160, Available: 80, ResetAt: 220, Status: "success"},
		{Id: 3, ChannelId: 7, ObservedAt: 220, Available: 100, ResetAt: 1_000, Status: "success"},
		{Id: 4, ChannelId: 7, ObservedAt: 280, Available: 98, ResetAt: 1_000, Status: "success"},
	}, "percent")

	analysis := AnalyzeQuotaConsumption(result, 100, 280, 60)

	require.InDelta(t, 2, *analysis.Methods.LatestInterval.RatePerMinute, 1e-9)
	require.InDelta(t, 2, *analysis.Methods.ObservedWindow.RatePerMinute, 1e-9)
	require.InDelta(t, 2, *analysis.Methods.EWMA.RatePerMinute, 1e-9)
	require.Equal(t, int64(60), analysis.Methods.ObservedWindow.ObservedSeconds)
	require.InDelta(t, 1.0/3.0, analysis.Methods.ObservedWindow.Coverage, 1e-9)
}

func TestAnalyzeQuotaConsumptionRestartsAfterEveryContinuityBoundary(t *testing.T) {
	tests := []struct {
		name string
		rows []model.ChannelQuotaSnapshot
	}{
		{
			name: "failure",
			rows: []model.ChannelQuotaSnapshot{
				{Id: 1, ObservedAt: 100, Available: 100, Status: "success"},
				{Id: 2, ObservedAt: 160, Available: 80, Status: "success"},
				{Id: 3, ObservedAt: 220, Status: "error"},
				{Id: 4, ObservedAt: 280, Available: 90, Status: "success"},
				{Id: 5, ObservedAt: 340, Available: 88, Status: "success"},
			},
		},
		{
			name: "recovery",
			rows: []model.ChannelQuotaSnapshot{
				{Id: 1, ObservedAt: 100, Available: 100, Status: "success"},
				{Id: 2, ObservedAt: 160, Available: 80, Status: "success"},
				{Id: 3, ObservedAt: 220, Available: 90, Status: "success"},
				{Id: 4, ObservedAt: 280, Available: 88, Status: "success"},
			},
		},
		{
			name: "baseline change",
			rows: []model.ChannelQuotaSnapshot{
				{Id: 1, ObservedAt: 100, Available: 100, Total: quotaNumber(100), Status: "success"},
				{Id: 2, ObservedAt: 160, Available: 80, Total: quotaNumber(100), Status: "success"},
				{Id: 3, ObservedAt: 220, Available: 90, Total: quotaNumber(200), Status: "success"},
				{Id: 4, ObservedAt: 280, Available: 88, Total: quotaNumber(200), Status: "success"},
			},
		},
		{
			name: "sampling gap",
			rows: []model.ChannelQuotaSnapshot{
				{Id: 1, ObservedAt: 100, Available: 100, Status: "success"},
				{Id: 2, ObservedAt: 160, Available: 99, Status: "success"},
				{Id: 3, ObservedAt: 220, Available: 98, Status: "success"},
				{Id: 4, ObservedAt: 3_820, Available: 90, Status: "success"},
				{Id: 5, ObservedAt: 3_880, Available: 88, Status: "success"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			analysis := AnalyzeQuotaConsumption(
				DeriveQuotaConsumption(test.rows, "percent"),
				test.rows[0].ObservedAt,
				test.rows[len(test.rows)-1].ObservedAt,
				60,
			)
			require.InDelta(t, 2, *analysis.Methods.ObservedWindow.RatePerMinute, 1e-9)
			require.Equal(t, int64(60), analysis.Methods.ObservedWindow.ObservedSeconds)
			require.Equal(t, 1, analysis.Methods.ObservedWindow.IntervalCount)
		})
	}
}

func TestAnalyzeQuotaConsumptionDoesNotUseAnIntervalCrossingRateWindowStart(t *testing.T) {
	result := DeriveQuotaConsumption([]model.ChannelQuotaSnapshot{
		{Id: 1, ChannelId: 7, ObservedAt: 100, Available: 100, Status: "success"},
		{Id: 2, ChannelId: 7, ObservedAt: 200, Available: 90, Status: "success"},
		{Id: 3, ChannelId: 7, ObservedAt: 260, Available: 89, Status: "success"},
	}, "percent")

	analysis := AnalyzeQuotaConsumption(result, 160, 260, 60)

	require.InDelta(t, 1, *analysis.Methods.ObservedWindow.RatePerMinute, 1e-9)
	require.Equal(t, int64(60), analysis.Methods.ObservedWindow.ObservedSeconds)
	require.InDelta(t, 0.6, analysis.Methods.ObservedWindow.Coverage, 1e-9)
}

func TestAnalyzeQuotaConsumptionReturnsResetAwareETAForEachMethod(t *testing.T) {
	for _, test := range []struct {
		name             string
		resetAt          int64
		want             string
		wantDepletionETA bool
	}{
		{name: "depletes before reset", resetAt: 1_000, want: QuotaETADepletesBeforeReset, wantDepletionETA: true},
		{name: "reset before depletion", resetAt: 300, want: QuotaETAResetBeforeDepletion},
		{name: "reset wins exact depletion tie", resetAt: 400, want: QuotaETAResetBeforeDepletion},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := DeriveQuotaConsumption([]model.ChannelQuotaSnapshot{
				{Id: 1, ChannelId: 7, ObservedAt: 100, Available: 10, ResetAt: test.resetAt, Status: "success"},
				{Id: 2, ChannelId: 7, ObservedAt: 160, Available: 8, ResetAt: test.resetAt, Status: "success"},
			}, "percent")
			analysis := AnalyzeQuotaConsumption(result, 100, 160, 60)

			for _, method := range []QuotaRateMethodAnalysis{
				analysis.Methods.LatestInterval,
				analysis.Methods.ObservedWindow,
				analysis.Methods.EWMA,
			} {
				require.Equal(t, test.want, method.ETA.Outcome)
				require.Equal(t, test.resetAt, *method.ETA.ResetAt)
				if test.wantDepletionETA {
					require.Equal(t, int64(240), *method.ETA.SecondsToDepletion)
					require.Equal(t, int64(400), *method.ETA.EstimatedDepletionAt)
				} else {
					require.Nil(t, method.ETA.SecondsToDepletion)
					require.Nil(t, method.ETA.EstimatedDepletionAt)
				}
			}
		})
	}
}

func TestAnalyzeQuotaConsumptionHandlesZeroAndNegativeAvailableETA(t *testing.T) {
	zero := AnalyzeQuotaConsumption(DeriveQuotaConsumption([]model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 100, Available: 0, ResetAt: 1_000, Status: "success"},
		{Id: 2, ObservedAt: 160, Available: 0, ResetAt: 1_000, Status: "success"},
	}, "percent"), 100, 160, 60)
	require.Equal(t, QuotaETADepletesBeforeReset, zero.Methods.ObservedWindow.ETA.Outcome)
	require.Equal(t, int64(0), *zero.Methods.ObservedWindow.ETA.SecondsToDepletion)
	require.Equal(t, int64(160), *zero.Methods.ObservedWindow.ETA.EstimatedDepletionAt)

	negative := AnalyzeQuotaConsumption(DeriveQuotaConsumption([]model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 100, Available: 0, Status: "success"},
		{Id: 2, ObservedAt: 160, Available: -1, Status: "success"},
	}, "percent"), 100, 160, 60)
	require.Equal(t, QuotaETAInsufficientData, negative.Methods.ObservedWindow.ETA.Outcome)
	require.Nil(t, negative.Methods.ObservedWindow.ETA.SecondsToDepletion)
}

func TestAnalyzeQuotaConsumptionEWMAIsInvariantToEquivalentIntervalSplits(t *testing.T) {
	unsplit := AnalyzeQuotaConsumption(DeriveQuotaConsumption([]model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 100, Available: 100, Status: "success"},
		{Id: 2, ObservedAt: 220, Available: 88, Status: "success"},
		{Id: 3, ObservedAt: 280, Available: 86, Status: "success"},
	}, "percent"), 100, 280, 60)
	split := AnalyzeQuotaConsumption(DeriveQuotaConsumption([]model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 100, Available: 100, Status: "success"},
		{Id: 2, ObservedAt: 160, Available: 94, Status: "success"},
		{Id: 3, ObservedAt: 220, Available: 88, Status: "success"},
		{Id: 4, ObservedAt: 280, Available: 86, Status: "success"},
	}, "percent"), 100, 280, 60)

	require.InDelta(t, *unsplit.Methods.EWMA.RatePerMinute, *split.Methods.EWMA.RatePerMinute, 1e-12)
	require.InDelta(t, *unsplit.Methods.EWMA.RatePerHour, *split.Methods.EWMA.RatePerHour, 1e-10)
}

func TestAnalyzeQuotaConsumptionAvoidsFiniteExtremeIntermediateOverflow(t *testing.T) {
	maximum := math.MaxFloat64
	analysis := AnalyzeQuotaConsumption(DeriveQuotaConsumption([]model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 100, Available: maximum, Status: "success"},
		{Id: 2, ObservedAt: 220, Available: maximum / 2, Status: "success"},
	}, "units"), 100, 220, 60)

	require.NotNil(t, analysis.Methods.ObservedWindow.RatePerMinute)
	require.True(t, quotaFinite(*analysis.Methods.ObservedWindow.RatePerMinute))
	require.InDelta(t, maximum/4, *analysis.Methods.ObservedWindow.RatePerMinute, maximum*1e-15)
	require.Equal(t, QuotaETADepletesBeforeReset, analysis.Methods.ObservedWindow.ETA.Outcome)
	require.Equal(t, int64(120), *analysis.Methods.ObservedWindow.ETA.SecondsToDepletion)
}

func TestAnalyzeQuotaConsumptionDistinguishesStableAndInsufficientETA(t *testing.T) {
	stable := AnalyzeQuotaConsumption(DeriveQuotaConsumption([]model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 100, Available: 8, Status: "success"},
		{Id: 2, ObservedAt: 160, Available: 8, Status: "success"},
	}, "percent"), 100, 160, 60)
	require.NotNil(t, stable.Methods.ObservedWindow.RatePerMinute)
	require.Zero(t, *stable.Methods.ObservedWindow.RatePerMinute)
	require.Equal(t, QuotaETAStableOrNoObservedConsumption, stable.Methods.ObservedWindow.ETA.Outcome)

	insufficient := AnalyzeQuotaConsumption(DeriveQuotaConsumption([]model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 100, Available: 8, Status: "success"},
	}, "percent"), 100, 160, 60)
	require.Nil(t, insufficient.Methods.ObservedWindow.RatePerMinute)
	require.Equal(t, QuotaETAInsufficientData, insufficient.Methods.ObservedWindow.ETA.Outcome)
}

func TestAnalyzeQuotaConsumptionKeepsLastObservedRateWhenCurrentSampleFails(t *testing.T) {
	analysis := AnalyzeQuotaConsumption(DeriveQuotaConsumption([]model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 100, Available: 100, Status: "success"},
		{Id: 2, ObservedAt: 160, Available: 90, Status: "success"},
		{Id: 3, ObservedAt: 220, Status: "error"},
	}, "percent"), 100, 220, 60)

	require.InDelta(t, 10, *analysis.Methods.ObservedWindow.RatePerMinute, 1e-9)
	require.Equal(t, int64(60), analysis.Methods.ObservedWindow.ObservedSeconds)
	require.InDelta(t, 0.5, analysis.Methods.ObservedWindow.Coverage, 1e-9)
	require.Equal(t, QuotaETAInsufficientData, analysis.Methods.ObservedWindow.ETA.Outcome)
	require.Nil(t, analysis.Methods.ObservedWindow.ETA.EstimatedDepletionAt)
	require.Equal(t, int64(160), *analysis.Methods.ObservedWindow.ObservedAt)
}

func TestBuildQuotaOverviewPointsCompactsWithoutConnectingContinuityBreaks(t *testing.T) {
	consumption := DeriveQuotaConsumption([]model.ChannelQuotaSnapshot{
		{Id: 1, ObservedAt: 100, Available: 100, ResetAt: 280, Status: "success"},
		{Id: 2, ObservedAt: 160, Available: 90, ResetAt: 280, Status: "success"},
		{Id: 3, ObservedAt: 220, Available: 80, ResetAt: 280, Status: "success"},
		{Id: 4, ObservedAt: 280, Available: 100, ResetAt: 1_000, Status: "success"},
		{Id: 5, ObservedAt: 340, Available: 90, ResetAt: 1_000, Status: "success"},
		{Id: 6, ObservedAt: 400, Status: "error"},
		{Id: 7, ObservedAt: 460, Available: 80, ResetAt: 1_000, Status: "success"},
		{Id: 8, ObservedAt: 520, Available: 70, ResetAt: 1_000, Status: "success"},
	}, "percent")

	points := BuildQuotaOverviewPoints(consumption.Observations, 4)

	require.Len(t, points, 4)
	require.Equal(t, int64(160), points[0].Timestamp)
	require.Equal(t, 90.0, *points[0].Available)
	require.False(t, points[0].ContinuityBreak)
	require.Equal(t, int64(280), points[1].Timestamp)
	require.Equal(t, 100.0, *points[1].Available)
	require.True(t, points[1].ContinuityBreak)
	require.Equal(t, int64(400), points[2].Timestamp)
	require.Nil(t, points[2].Available)
	require.True(t, points[2].ContinuityBreak)
	require.Equal(t, int64(520), points[3].Timestamp)
	require.Equal(t, 70.0, *points[3].Available)
	require.True(t, points[3].ContinuityBreak)

	uncompressed := BuildQuotaOverviewPoints(consumption.Observations, 120)
	require.Len(t, uncompressed, len(consumption.Observations))
}
