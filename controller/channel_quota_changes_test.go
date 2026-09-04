package controller

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/stretchr/testify/require"
)

func TestBuildQuotaChangeItemsUsesAdjacentSamplesAndSignedRate(t *testing.T) {
	rows := []model.ChannelQuotaAggregateRow{
		{ID: 1, ChannelID: 7, ChannelName: "Codex A", ObservedAt: 100, Available: 90, Total: ptrChangeFloat(100), MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary", Status: "success", Unit: "percent"},
		{ID: 2, ChannelID: 7, ChannelName: "Codex A", ObservedAt: 160, Available: 80, Total: ptrChangeFloat(100), MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary", Status: "success", Unit: "percent"},
		{ID: 3, ChannelID: 7, ChannelName: "Codex A", ObservedAt: 220, Available: 79, MetricType: "codex_rate_limit", WindowType: "weekly", Source: "codex_wham_usage_secondary", Status: "success", Unit: "percent"},
	}
	items, quality := buildQuotaChangeItems(rows)
	require.Len(t, items, 2)
	primary := items[0]
	if primary.Source != "codex_wham_usage_primary" {
		primary = items[1]
	}
	require.Equal(t, "decrease", primary.Direction)
	require.InDelta(t, -10, *primary.ChangePerMinute, 0.0001)
	require.InDelta(t, 10, *primary.AbsChangePerMinute, 0.0001)
	require.Equal(t, int64(60), primary.SampleSpanSeconds)
	require.Equal(t, 2, primary.DataQuality.SuccessCount)
	require.Equal(t, 3, quality.SuccessCount)
}

func TestBuildQuotaChangeItemsDoesNotCrossResetOrFailure(t *testing.T) {
	rows := []model.ChannelQuotaAggregateRow{
		{ID: 1, ChannelID: 1, ChannelName: "A", ObservedAt: 100, Available: 90, ResetAt: 200, Status: "success", MetricType: "balance", WindowType: "none"},
		{ID: 2, ChannelID: 1, ChannelName: "A", ObservedAt: 160, Status: "error", ErrorCode: "upstream_http", MetricType: "balance", WindowType: "none"},
		{ID: 3, ChannelID: 1, ChannelName: "A", ObservedAt: 220, Available: 10, ResetAt: 300, Status: "success", MetricType: "balance", WindowType: "none"},
	}
	items, _ := buildQuotaChangeItems(rows)
	require.Len(t, items, 1)
	require.Nil(t, items[0].ChangePerMinute)
	require.Equal(t, "unknown", items[0].Direction)
	require.Equal(t, 1, items[0].DataQuality.ErrorCount)
	require.Equal(t, 1, items[0].DataQuality.ResetBoundaries)
}

func TestBuildQuotaChangeItemsSeparatesUnsupportedFromErrors(t *testing.T) {
	rows := []model.ChannelQuotaAggregateRow{
		{ID: 1, ChannelID: 9, ChannelName: "Claude", ObservedAt: 100, Status: "unsupported", MetricType: "balance", WindowType: "none"},
		{ID: 2, ChannelID: 9, ChannelName: "Claude", ObservedAt: 200, Status: "error", MetricType: "balance", WindowType: "none"},
	}
	items, quality := buildQuotaChangeItems(rows)
	require.Len(t, items, 1)
	require.Equal(t, 1, items[0].DataQuality.UnsupportedCount)
	require.Equal(t, 1, items[0].DataQuality.ErrorCount)
	require.Equal(t, 1, quality.UnsupportedCount)
	require.Equal(t, 1, quality.ErrorCount)
}

func TestBuildQuotaChangeItemsSeparatesSubscriptionPlans(t *testing.T) {
	rows := []model.ChannelQuotaAggregateRow{
		{ID: 1, ChannelID: 21, ChannelName: "Codex", ObservedAt: 100, Available: 90, MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary", PlanType: "team", Unit: "percent", WindowSeconds: 18000, Status: "success"},
		{ID: 2, ChannelID: 21, ChannelName: "Codex", ObservedAt: 160, Available: 80, MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary", PlanType: "team", Unit: "percent", WindowSeconds: 18000, Status: "success"},
		{ID: 3, ChannelID: 21, ChannelName: "Codex", ObservedAt: 100, Available: 50, MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary", PlanType: "pro", Unit: "percent", WindowSeconds: 18000, Status: "success"},
		{ID: 4, ChannelID: 21, ChannelName: "Codex", ObservedAt: 160, Available: 40, MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary", PlanType: "pro", Unit: "percent", WindowSeconds: 18000, Status: "success"},
	}
	items, _ := buildQuotaChangeItems(rows)
	require.Len(t, items, 2)
	for _, item := range items {
		require.Equal(t, "decrease", item.Direction)
		require.InDelta(t, -10, *item.ChangePerMinute, 0.0001)
	}
}

func TestBuildQuotaChangeItemsExposesReadOnlyAlertState(t *testing.T) {
	previousEnabled := common.ChannelQuotaAlertEnabled
	previousWarning := common.ChannelQuotaAlertWarningPercent
	previousCritical := common.ChannelQuotaAlertCriticalPercent
	defer func() {
		common.ChannelQuotaAlertEnabled = previousEnabled
		common.ChannelQuotaAlertWarningPercent = previousWarning
		common.ChannelQuotaAlertCriticalPercent = previousCritical
	}()
	common.ChannelQuotaAlertEnabled = true
	common.ChannelQuotaAlertWarningPercent = 20
	common.ChannelQuotaAlertCriticalPercent = 10

	items, _ := buildQuotaChangeItems([]model.ChannelQuotaAggregateRow{
		{ID: 1, ChannelID: 11, ChannelName: "Codex", ObservedAt: 100, Available: 15, Total: ptrChangeFloat(100), Status: "success", MetricType: "balance", WindowType: "none"},
	})
	require.Len(t, items, 1)
	require.NotNil(t, items[0].Alert)
	require.Equal(t, "warning", items[0].Alert.Status)
	require.InDelta(t, 15, *items[0].Alert.RatioPercent, 0.0001)

	items, _ = buildQuotaChangeItems([]model.ChannelQuotaAggregateRow{
		{ID: 2, ChannelID: 12, ChannelName: "Claude", ObservedAt: 100, Available: 15, Status: "unsupported", MetricType: "balance", WindowType: "none"},
	})
	require.NotNil(t, items[0].Alert)
	require.Equal(t, "unavailable", items[0].Alert.Status)
}

func TestSortQuotaChangeItemsDefaultsToLargestAbsoluteRate(t *testing.T) {
	items := []quotaChangeItem{
		{ChannelID: 1, Name: "one", AbsChangePerMinute: ptrChangeFloat(2)},
		{ChannelID: 2, Name: "two", AbsChangePerMinute: ptrChangeFloat(5)},
	}
	sortQuotaChangeItems(items, "")
	require.Equal(t, 2, items[0].ChannelID)
}

func ptrChangeFloat(value float64) *float64 { return &value }

func TestQuotaChangesGenericFailureIsAnEventNotAnExtraAccount(t *testing.T) {
	rows := []model.ChannelQuotaAggregateRow{
		{ID: 1, ChannelID: 7, ChannelName: "Codex", ObservedAt: 100, Available: 90, Status: "success", MetricType: "codex_rate_limit", WindowType: "weekly", Source: "codex_wham_usage_primary", PlanType: "pro", Unit: "percent"},
		{ID: 2, ChannelID: 7, ChannelName: "Codex", ObservedAt: 160, Available: 80, Status: "success", MetricType: "codex_rate_limit", WindowType: "weekly", Source: "codex_wham_usage_primary", PlanType: "pro", Unit: "percent"},
		{ID: 3, ChannelID: 7, ChannelName: "Codex", ObservedAt: 220, Status: "error", MetricType: "codex_rate_limit", WindowType: "none", Source: "codex_wham_usage", Unit: "percent"},
	}
	items, quality := buildQuotaChangeItems(rows)
	require.Len(t, items, 1)
	require.Equal(t, "codex_wham_usage_primary", items[0].Source)
	require.Equal(t, "weekly", items[0].WindowType)
	require.Equal(t, "error", items[0].Status)
	require.Nil(t, items[0].CurrentAvailable)
	require.Nil(t, items[0].ChangePerMinute)
	require.Equal(t, 10.0, *items[0].Consumption.Observed)
	require.Equal(t, 1, quality.ErrorCount)
	require.Equal(t, -10.0, *items[0].PeakDropPerMinute)
}

func TestQuotaChangesHistoryPeakDoesNotBecomeLatestZero(t *testing.T) {
	rows := []model.ChannelQuotaAggregateRow{
		{ID: 1, ChannelID: 1, ObservedAt: 100, Available: 100, Status: "success"},
		{ID: 2, ChannelID: 1, ObservedAt: 160, Available: 90, Status: "success"},
		{ID: 3, ChannelID: 1, ObservedAt: 220, Available: 90, Status: "success"},
	}
	items, _ := buildQuotaChangeItems(rows)
	require.Len(t, items, 1)
	require.Equal(t, 0.0, *items[0].ChangePerMinute)
	require.Equal(t, 10.0, *quotaChangeSummary(items)["max_abs_change_per_minute"].(*float64))
	require.Equal(t, -10.0, *quotaChangeSummary(items)["max_drop_per_minute"].(*float64))
}

func TestQuotaChangesDoesNotBridgeFailureWithinSameReset(t *testing.T) {
	rows := []model.ChannelQuotaAggregateRow{
		{ID: 1, ChannelID: 1, ObservedAt: 100, Available: 100, Status: "success"},
		{ID: 2, ChannelID: 1, ObservedAt: 130, Status: "error"},
		{ID: 3, ChannelID: 1, ObservedAt: 160, Available: 90, Status: "success"},
	}
	items, _ := buildQuotaChangeItems(rows)
	require.Len(t, items, 1)
	require.Nil(t, items[0].ChangePerMinute)
	require.Nil(t, items[0].Consumption.Observed)
}

func TestQuotaChangesAnalysisUsesLatestPostResetSegment(t *testing.T) {
	rows := []model.ChannelQuotaAggregateRow{
		{ID: 1, ChannelID: 1, ObservedAt: 100, Available: 100, ResetAt: 220, Status: "success"},
		{ID: 2, ChannelID: 1, ObservedAt: 160, Available: 80, ResetAt: 220, Status: "success"},
		{ID: 3, ChannelID: 1, ObservedAt: 220, Available: 100, ResetAt: 1_000, Status: "success"},
		{ID: 4, ChannelID: 1, ObservedAt: 280, Available: 98, ResetAt: 1_000, Status: "success"},
	}
	items, _ := buildQuotaChangeItemsWithAnalysis(rows, 100, 280, 60)
	require.Len(t, items, 1)
	require.Equal(t, "observed_window", items[0].Analysis.DefaultMethod)
	require.InDelta(t, 2, *items[0].Analysis.Methods.ObservedWindow.RatePerMinute, 1e-9)
	require.Equal(t, int64(60), items[0].Analysis.Methods.ObservedWindow.ObservedSeconds)
	require.Equal(t, int64(280), *items[0].Analysis.Methods.ObservedWindow.ObservedAt)
	require.InDelta(t, 1.0/3.0, items[0].Analysis.Methods.ObservedWindow.Coverage, 1e-9)
	require.Equal(t, service.QuotaETAResetBeforeDepletion, items[0].Analysis.Methods.ObservedWindow.ETA.Outcome)
}

func TestQuotaChangesLimitBoundsAnalysisWork(t *testing.T) {
	const seriesCount = 400
	rows := make([]model.ChannelQuotaAggregateRow, 0, seriesCount*2)
	for channelID := 1; channelID <= seriesCount; channelID++ {
		rows = append(rows,
			model.ChannelQuotaAggregateRow{ID: channelID * 2, ChannelID: channelID, ObservedAt: 100, Available: 100, Status: "success"},
			model.ChannelQuotaAggregateRow{ID: channelID*2 + 1, ChannelID: channelID, ObservedAt: 160, Available: 99, Status: "success"},
		)
	}

	items, quality := buildQuotaChangeItemsLightweight(rows)
	require.Len(t, items, seriesCount)
	require.Equal(t, seriesCount*2, quality.SuccessCount)
	allItems := items
	analysisCalls := 0
	items = finalizeQuotaChangeItems(items, "", 1, 100, 160, 30, 3, func(consumption service.QuotaConsumptionResult, start, end, halfLife int64) service.QuotaAnalysis {
		analysisCalls++
		return service.QuotaAnalysis{DefaultMethod: "sentinel"}
	})

	require.Len(t, items, 1)
	require.Equal(t, 1, analysisCalls)
	require.NotNil(t, items[0].Analysis)
	require.Equal(t, "sentinel", items[0].Analysis.DefaultMethod)
	require.NotNil(t, items[0].OverviewPoints)
	require.LessOrEqual(t, len(*items[0].OverviewPoints), 3)
	for index := 1; index < len(allItems); index++ {
		require.Nil(t, allItems[index].Analysis)
		require.Nil(t, allItems[index].OverviewPoints)
		require.Nil(t, allItems[index].analysisRows)
	}
}

func TestQuotaChangesOmitsOverviewProjectionByDefault(t *testing.T) {
	items, _ := buildQuotaChangeItems([]model.ChannelQuotaAggregateRow{
		{ID: 1, ChannelID: 1, ObservedAt: 100, Available: 100, Status: "success"},
		{ID: 2, ChannelID: 1, ObservedAt: 160, Available: 90, Status: "success"},
	})
	require.Len(t, items, 1)
	require.Nil(t, items[0].OverviewPoints)
}
