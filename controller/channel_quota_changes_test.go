package controller

import (
	"testing"

	"github.com/ForceMind/MyAPI/model"
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

func TestSortQuotaChangeItemsDefaultsToLargestAbsoluteRate(t *testing.T) {
	items := []quotaChangeItem{
		{ChannelID: 1, Name: "one", AbsChangePerMinute: ptrChangeFloat(2)},
		{ChannelID: 2, Name: "two", AbsChangePerMinute: ptrChangeFloat(5)},
	}
	sortQuotaChangeItems(items, "")
	require.Equal(t, 2, items[0].ChannelID)
}

func ptrChangeFloat(value float64) *float64 { return &value }
