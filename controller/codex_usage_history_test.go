package controller

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCodexUsageSnapshots(t *testing.T) {
	body := []byte(`{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":12.5,"reset_at":1700000100,"limit_window_seconds":18000},"secondary_window":{"used_percent":44,"reset_at":1700400000,"limit_window_seconds":604800}}}`)
	snapshots := normalizeCodexUsageSnapshots(7, 1700000000, 200, body)
	require.Len(t, snapshots, 2)
	require.Equal(t, "codex_rate_limit", snapshots[0].MetricType)
	require.Equal(t, "five_hour", snapshots[0].WindowType)
	require.Equal(t, int64(18000), snapshots[0].WindowSeconds)
	require.Equal(t, "pro", snapshots[0].PlanType)
	require.InDelta(t, 87.5, snapshots[0].Available, 0.001)
	require.Equal(t, "weekly", snapshots[1].WindowType)
}

func TestNormalizeCodexUsageSnapshotsRejectsInvalidPercent(t *testing.T) {
	body := []byte(`{"rate_limit":{"primary_window":{"used_percent":101},"secondary_window":{"used_percent":-1}}}`)
	snapshots := normalizeCodexUsageSnapshots(7, 10, 200, body)
	require.Len(t, snapshots, 1)
	require.Equal(t, "unsupported", snapshots[0].Status)
	require.Equal(t, "invalid_payload", snapshots[0].ErrorCode)
	snapshots = normalizeCodexUsageSnapshots(7, 10, 200, []byte(`not-json`))
	require.Len(t, snapshots, 1)
	require.Equal(t, "unsupported", snapshots[0].Status)
}

func TestNormalizeCodexUsageSnapshotsRecordsUpstreamFailure(t *testing.T) {
	snapshots := normalizeCodexUsageSnapshots(7, 1700000000, 503, nil)
	require.Len(t, snapshots, 1)
	require.Equal(t, "error", snapshots[0].Status)
	require.Equal(t, "upstream_http", snapshots[0].ErrorCode)
	require.Equal(t, int64(1700000000), snapshots[0].ObservedAt)
}

func TestNormalizeCodexUsageSnapshotsRecordsTransportFailure(t *testing.T) {
	snapshots := normalizeCodexUsageSnapshots(7, 1700000000, 0, nil)
	require.Len(t, snapshots, 1)
	require.Equal(t, "upstream_transport", snapshots[0].ErrorCode)
}

func TestFiniteCodexPercent(t *testing.T) {
	for _, value := range []float64{0, 50, 100} {
		require.True(t, finiteCodexPercent(value))
	}
	for _, value := range []float64{-1, 101, math.NaN(), math.Inf(1)} {
		require.False(t, finiteCodexPercent(value))
	}
}
