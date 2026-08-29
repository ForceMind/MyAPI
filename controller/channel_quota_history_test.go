package controller

import (
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
