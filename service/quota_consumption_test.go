package service

import (
	"math"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/require"
)

func quotaConsumptionFixture(times []int64) []model.ChannelQuotaSnapshot {
	rows := make([]model.ChannelQuotaSnapshot, len(times))
	for i, at := range times {
		rows[i] = model.ChannelQuotaSnapshot{Id: i + 1, ChannelId: 7, ObservedAt: at, Available: 100 - float64(i), Status: "success", MetricType: "codex_rate_limit", Source: "codex_wham_usage_primary", WindowType: "weekly", Unit: "percent"}
	}
	return rows
}

func TestQuotaConsumptionRatesUseActualElapsedSeconds(t *testing.T) {
	for _, test := range []struct {
		name    string
		seconds int64
		rate    float64
	}{{"one minute", 60, 2}, {"ten minute interval", 600, 0.2}} {
		t.Run(test.name, func(t *testing.T) {
			rows := quotaConsumptionFixture([]int64{100, 100 + test.seconds})
			rows[1].Available = 98
			result := DeriveQuotaConsumption(rows, "percent")
			require.NotNil(t, result.Summary.Observed)
			require.Equal(t, 2.0, *result.Summary.Observed)
			require.Equal(t, test.seconds, result.Summary.ObservedSeconds)
			require.InDelta(t, test.rate, *result.Summary.PeakRatePerMinute, 1e-9)
			require.InDelta(t, test.rate, *result.Observations[1].RatePerMinute, 1e-9)
			require.Equal(t, "interval_end", result.Summary.Allocation)
		})
	}
}

func TestQuotaConsumptionInfersHistoricalCadenceAndIsolatesOutages(t *testing.T) {
	for _, test := range []struct {
		name        string
		times       []int64
		gaps, pairs int
	}{
		{"regular minute", []int64{0, 60, 120, 180, 240}, 0, 4},
		{"historical fifteen minutes", []int64{0, 900, 1800, 2700, 3600}, 0, 4},
		{"minute to fifteen minute", []int64{0, 60, 120, 180, 1080, 1980, 2880}, 0, 6},
		{"fifteen minute to minute", []int64{0, 900, 1800, 2700, 2760, 2820, 2880}, 0, 6},
		{"one long outage", []int64{0, 60, 120, 3720, 3780, 3840}, 1, 4},
		{"two isolated points", []int64{0, 600}, 0, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := DeriveQuotaConsumption(quotaConsumptionFixture(test.times), "percent")
			require.Equal(t, test.gaps, result.Summary.GapCount)
			require.Equal(t, test.pairs, result.Summary.PairCount)
			require.InDelta(t, float64(test.pairs), *result.Summary.Observed, 1e-9)
		})
	}
}

func TestQuotaConsumptionRejectsFailureResetAndBaselineBridges(t *testing.T) {
	for _, test := range []struct {
		name                         string
		mutate                       func([]model.ChannelQuotaSnapshot)
		reset, baseline, interrupted int
	}{
		{"failure", func(rows []model.ChannelQuotaSnapshot) { rows[1].Status = "error" }, 0, 0, 1},
		{"invalid", func(rows []model.ChannelQuotaSnapshot) { rows[1].Used = quotaNumber(math.NaN()) }, 0, 0, 1},
		{"reset", func(rows []model.ChannelQuotaSnapshot) { rows[1].ResetAt = 1000; rows[2].ResetAt = 1000 }, 1, 0, 0},
		{"total changes", func(rows []model.ChannelQuotaSnapshot) {
			rows[0].Total = quotaNumber(100)
			rows[1].Total = quotaNumber(200)
			rows[2].Total = quotaNumber(200)
		}, 0, 1, 0},
		{"used basis changes", func(rows []model.ChannelQuotaSnapshot) { rows[1].Used = quotaNumber(1); rows[2].Used = quotaNumber(2) }, 0, 1, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := quotaConsumptionFixture([]int64{0, 60, 120})
			test.mutate(rows)
			result := DeriveQuotaConsumption(rows, "percent")
			require.Nil(t, result.Observations[1].Consumption)
			require.True(t, result.Observations[1].ContinuityBreak)
			require.Equal(t, test.reset, result.Summary.ResetBoundaries)
			require.Equal(t, test.baseline, result.Summary.BaselineChangeCount)
			require.Equal(t, test.interrupted, result.Summary.InterruptedCount)
		})
	}
}

func TestQuotaConsumptionPreservesEarlierPeakAndTotalWhenLatestFails(t *testing.T) {
	rows := quotaConsumptionFixture([]int64{0, 60, 120, 180, 240})
	rows[1].Available = 90
	rows[2].Available = 90
	rows[3].Available = 95
	rows[4].Status = "error"
	result := DeriveQuotaConsumption(rows, "percent")
	// A recovery is not negative consumption; a later error cannot erase prior
	// observed consumption or turn the historical peak into the latest zero.
	require.InDelta(t, 10, *result.Summary.Observed, 1e-9)
	require.InDelta(t, 10, *result.Summary.PeakRatePerMinute, 1e-9)
	require.Equal(t, int64(60), *result.Summary.PeakRateObservedAt)
	require.Equal(t, int64(120), result.Summary.ObservedSeconds)
	require.Equal(t, 1, result.Summary.RecoveryCount)
	require.Nil(t, result.Observations[3].Consumption)
	require.Nil(t, result.Observations[4].Consumption)
}

func TestQuotaConsumptionSameTimestampCannotYieldAnInfiniteRate(t *testing.T) {
	rows := quotaConsumptionFixture([]int64{60, 60})
	result := DeriveQuotaConsumption(rows, "percent")
	require.Nil(t, result.Summary.Observed)
	require.Nil(t, result.Summary.PeakRatePerMinute)
	require.Equal(t, 1, result.Summary.GapCount)
}

// Relative timestamps and anonymous provider-window metadata reproduce the
// observed rounding/initialization patterns without account identifiers.
func quotaCodexResetFixture(times, resets []int64, used []float64) []model.ChannelQuotaSnapshot {
	rows := quotaConsumptionFixture(times)
	for index := range rows {
		rows[index].ResetAt = resets[index]
		rows[index].Used = quotaNumber(used[index])
		rows[index].Available = 100 - used[index]
		rows[index].Total = quotaNumber(100)
	}
	return rows
}

func TestQuotaConsumptionCodexFutureResetRoundingDoesNotCreateFalseResets(t *testing.T) {
	for _, test := range []struct {
		name     string
		used     []float64
		consumed float64
	}{
		{"unchanged one percent", []float64{1, 1, 1}, 0},
		{"fully used window", []float64{100, 100, 100}, 0},
		{"consumption survives timestamp jitter", []float64{1, 1, 2}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := quotaCodexResetFixture([]int64{100, 180, 255}, []int64{10000, 9999, 10000}, test.used)
			result := DeriveQuotaConsumption(rows, "percent")
			require.Zero(t, result.Summary.ResetBoundaries)
			require.Zero(t, result.Summary.BaselineChangeCount)
			require.Equal(t, 2, result.Summary.PairCount)
			require.Equal(t, int64(155), result.Summary.ObservedSeconds)
			require.NotNil(t, result.Summary.Observed)
			require.InDelta(t, test.consumed, *result.Summary.Observed, 1e-9)
			for _, point := range result.Observations {
				require.False(t, point.Reset)
				require.False(t, point.ContinuityBreak)
			}
		})
	}
}

func TestQuotaConsumptionCodexUnusedFutureWindowCanMoveWithoutReset(t *testing.T) {
	rows := quotaCodexResetFixture([]int64{100, 180, 270, 345}, []int64{10000, 10080, 10170, 10245}, []float64{0, 0, 0, 0})
	result := DeriveQuotaConsumption(rows, "percent")
	require.Zero(t, result.Summary.ResetBoundaries)
	require.Zero(t, result.Summary.BaselineChangeCount)
	require.Equal(t, 3, result.Summary.PairCount)
	require.Equal(t, int64(245), result.Summary.ObservedSeconds)
	require.NotNil(t, result.Summary.Observed)
	require.Zero(t, *result.Summary.Observed)
	require.Zero(t, *result.Summary.PeakRatePerMinute)
	for index, point := range result.Observations {
		require.Equal(t, rows[index].ResetAt, point.Snapshot.ResetAt, "provider metadata remains intact")
		require.False(t, point.Reset)
		require.False(t, point.ContinuityBreak)
	}
}

func TestQuotaConsumptionCodexResetToleranceHasStrictBoundaries(t *testing.T) {
	for _, test := range []struct {
		name      string
		mutate    func([]model.ChannelQuotaSnapshot)
		wantReset bool
	}{
		{"plus two seconds", func(rows []model.ChannelQuotaSnapshot) { rows[1].ResetAt = 10002 }, false},
		{"minus two seconds", func(rows []model.ChannelQuotaSnapshot) { rows[1].ResetAt = 9998 }, false},
		{"three seconds is not rounding", func(rows []model.ChannelQuotaSnapshot) { rows[1].ResetAt = 10003 }, true},
		{"large nonzero reset movement", func(rows []model.ChannelQuotaSnapshot) { rows[1].ResetAt = 10100 }, true},
		{"zero to first usage with shifted window", func(rows []model.ChannelQuotaSnapshot) {
			rows[0].Used = quotaNumber(0)
			rows[0].Available = 100
			rows[1].ResetAt = 10100
		}, true},
		{"cross prior reset despite rounding", func(rows []model.ChannelQuotaSnapshot) { rows[0].ResetAt = 180; rows[1].ResetAt = 181 }, true},
		{"cross unchanged reset", func(rows []model.ChannelQuotaSnapshot) { rows[0].ResetAt = 180; rows[1].ResetAt = 180 }, true},
		{"updated reset is already past", func(rows []model.ChannelQuotaSnapshot) { rows[0].ResetAt = 181; rows[1].ResetAt = 179 }, true},
		{"missing prior reset", func(rows []model.ChannelQuotaSnapshot) { rows[0].ResetAt = 0 }, true},
		{"missing new reset", func(rows []model.ChannelQuotaSnapshot) { rows[1].ResetAt = 0 }, true},
		{"other provider remains strict", func(rows []model.ChannelQuotaSnapshot) {
			for index := range rows {
				rows[index].MetricType = "provider_rate_limit"
			}
		}, true},
		{"other unit remains strict", func(rows []model.ChannelQuotaSnapshot) {
			for index := range rows {
				rows[index].Unit = "tokens"
			}
		}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := quotaCodexResetFixture([]int64{100, 180}, []int64{10000, 10001}, []float64{1, 2})
			test.mutate(rows)
			result := DeriveQuotaConsumption(rows, rows[0].Unit)
			require.Equal(t, test.wantReset, result.Observations[1].Reset)
			if test.wantReset {
				require.Equal(t, 1, result.Summary.ResetBoundaries)
				require.Nil(t, result.Summary.Observed)
			} else {
				require.Zero(t, result.Summary.ResetBoundaries)
				require.Equal(t, 1.0, *result.Summary.Observed)
			}
		})
	}
}

func TestQuotaConsumptionCodexZeroUsageExceptionDoesNotBypassOtherGuards(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func([]model.ChannelQuotaSnapshot)
	}{
		{"crossing actual reset", func(rows []model.ChannelQuotaSnapshot) { rows[0].ResetAt = 180; rows[1].ResetAt = 260 }},
		{"usage is absent not zero", func(rows []model.ChannelQuotaSnapshot) { rows[0].Used = nil; rows[1].Used = nil }},
		{"unknown total", func(rows []model.ChannelQuotaSnapshot) { rows[0].Total = nil; rows[1].Total = nil }},
		{"inconsistent zero usage", func(rows []model.ChannelQuotaSnapshot) { rows[1].Available = 99 }},
		{"different plan", func(rows []model.ChannelQuotaSnapshot) { rows[1].PlanType = "other" }},
		{"same timestamp", func(rows []model.ChannelQuotaSnapshot) { rows[1].ObservedAt = 100 }},
		{"invalid value", func(rows []model.ChannelQuotaSnapshot) { rows[1].Used = quotaNumber(math.NaN()) }},
		{"failure", func(rows []model.ChannelQuotaSnapshot) { rows[1].Status = "error" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := quotaCodexResetFixture([]int64{100, 180}, []int64{10000, 10080}, []float64{0, 0})
			test.mutate(rows)
			result := DeriveQuotaConsumption(rows, "percent")
			require.Nil(t, result.Observations[1].Consumption)
			require.Zero(t, result.Summary.PairCount)
			require.True(t, result.Observations[1].ContinuityBreak)
		})
	}
	// Moving unused-window metadata must not turn a missing sampling period
	// into measured zero consumption or valid observed duration.
	rows := quotaCodexResetFixture([]int64{100, 180, 260, 3860, 3940, 4020}, []int64{10000, 10080, 10160, 13760, 13840, 13920}, []float64{0, 0, 0, 0, 0, 0})
	result := DeriveQuotaConsumption(rows, "percent")
	require.Zero(t, result.Summary.ResetBoundaries)
	require.Equal(t, 1, result.Summary.GapCount)
	require.Nil(t, result.Observations[3].Consumption)
	require.Equal(t, int64(320), result.Summary.ObservedSeconds)
}

func TestQuotaConsumptionCodexResetRoundingDoesNotBridgeFailedSampling(t *testing.T) {
	rows := quotaCodexResetFixture([]int64{100, 180, 260, 340}, []int64{10000, 0, 10001, 10000}, []float64{1, 0, 2, 3})
	rows[1].Status = "error"
	result := DeriveQuotaConsumption(rows, "percent")
	require.Zero(t, result.Summary.ResetBoundaries)
	require.Equal(t, 1, result.Summary.InterruptedCount)
	require.Equal(t, 1, result.Summary.PairCount)
	require.Equal(t, 1.0, *result.Summary.Observed)
	require.Equal(t, int64(80), result.Summary.ObservedSeconds)
	require.Nil(t, result.Observations[2].Consumption)
	require.True(t, result.Observations[2].ContinuityBreak)
}

func TestQuotaConsumptionAvailableBasisOverridesReportedUsage(t *testing.T) {
	usedStart, usedEnd := 10.0, 20.0
	rows := []model.ChannelQuotaSnapshot{
		{Id: 1, ChannelId: 7, ObservedAt: 100, Available: 90, Used: &usedStart, Status: "success", MetricType: "balance", WindowType: "none", Source: "provider", Unit: "credits"},
		{Id: 2, ChannelId: 7, ObservedAt: 160, Available: 85, Used: &usedEnd, Status: "success", MetricType: "balance", WindowType: "none", Source: "provider", Unit: "credits"},
	}

	automatic := DeriveQuotaConsumption(rows, "credits")
	require.Equal(t, "used", automatic.Summary.Basis)
	require.InDelta(t, 10, *automatic.Summary.Observed, 1e-9)
	require.InDelta(t, 10, *automatic.Observations[1].RatePerMinute, 1e-9)

	available := DeriveQuotaConsumptionWithBasis(rows, "credits", QuotaConsumptionBasisAvailable)
	require.Equal(t, "available", available.Summary.Basis)
	require.InDelta(t, 5, *available.Summary.Observed, 1e-9)
	require.InDelta(t, 5, *available.Observations[1].RatePerMinute, 1e-9)
}

func TestQuotaConsumptionAvailableBasisDoesNotBreakWhenReportedUsageAppears(t *testing.T) {
	usedMiddle, usedEnd := 10.0, 20.0
	rows := []model.ChannelQuotaSnapshot{
		{Id: 1, ChannelId: 7, ObservedAt: 100, Available: 100, Status: "success", MetricType: "balance", WindowType: "none", Source: "provider", Unit: "credits"},
		{Id: 2, ChannelId: 7, ObservedAt: 160, Available: 90, Used: &usedMiddle, Status: "success", MetricType: "balance", WindowType: "none", Source: "provider", Unit: "credits"},
		{Id: 3, ChannelId: 7, ObservedAt: 220, Available: 80, Used: &usedEnd, Status: "success", MetricType: "balance", WindowType: "none", Source: "provider", Unit: "credits"},
	}

	automatic := DeriveQuotaConsumption(rows, "credits")
	require.Equal(t, 1, automatic.Summary.BaselineChangeCount)
	require.Equal(t, 1, automatic.Summary.PairCount)

	available := DeriveQuotaConsumptionWithBasis(rows, "credits", QuotaConsumptionBasisAvailable)
	require.Zero(t, available.Summary.BaselineChangeCount)
	require.Equal(t, 2, available.Summary.PairCount)
	require.Equal(t, int64(120), available.Summary.ObservedSeconds)
	require.Equal(t, "available", available.Summary.Basis)
	require.InDelta(t, 20, *available.Summary.Observed, 1e-9)
}

func TestQuotaConsumptionAvailableBasisKeepsContinuityGuards(t *testing.T) {
	t.Run("remaining quota increase is a recovery", func(t *testing.T) {
		usedStart, usedEnd := 10.0, 20.0
		rows := []model.ChannelQuotaSnapshot{
			{Id: 1, ChannelId: 7, ObservedAt: 100, Available: 90, Used: &usedStart, Status: "success", MetricType: "balance", WindowType: "none", Source: "provider", Unit: "credits"},
			{Id: 2, ChannelId: 7, ObservedAt: 160, Available: 95, Used: &usedEnd, Status: "success", MetricType: "balance", WindowType: "none", Source: "provider", Unit: "credits"},
		}
		result := DeriveQuotaConsumptionWithBasis(rows, "credits", QuotaConsumptionBasisAvailable)
		require.Nil(t, result.Summary.Observed)
		require.Equal(t, 1, result.Summary.RecoveryCount)
		require.True(t, result.Observations[1].ContinuityBreak)
	})

	t.Run("provider reset is not bridged", func(t *testing.T) {
		rows := quotaConsumptionFixture([]int64{60, 180})
		rows[0].ResetAt = 120
		rows[1].ResetAt = 120
		result := DeriveQuotaConsumptionWithBasis(rows, "percent", QuotaConsumptionBasisAvailable)
		require.Nil(t, result.Summary.Observed)
		require.Equal(t, 1, result.Summary.ResetBoundaries)
		require.True(t, result.Observations[1].Reset)
	})

	t.Run("long sampling gap is not reconstructed", func(t *testing.T) {
		rows := quotaConsumptionFixture([]int64{0, 60, 120, 3720, 3780, 3840})
		result := DeriveQuotaConsumptionWithBasis(rows, "percent", QuotaConsumptionBasisAvailable)
		require.Equal(t, 1, result.Summary.GapCount)
		require.Nil(t, result.Observations[3].Consumption)
		require.True(t, result.Observations[3].ContinuityBreak)
	})
}
