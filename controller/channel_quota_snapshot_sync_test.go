package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChannelQuotaSnapshotSyncIsOptInAndUsesSafeDefaults(t *testing.T) {
	t.Setenv("CHANNEL_QUOTA_SYNC_ENABLED", "")
	t.Setenv("CHANNEL_QUOTA_SYNC_INTERVAL", "")
	t.Setenv("CHANNEL_QUOTA_SYNC_MAX_CHANNELS", "")
	handler := channelQuotaSnapshotSyncHandler{}
	require.False(t, handler.Enabled())
	require.Equal(t, channelQuotaSnapshotSyncDefaultInterval, handler.Interval())
	require.Equal(t, channelQuotaSnapshotSyncDefaultMaxChannels, channelQuotaSnapshotSyncMaxChannelsConfigured())

	t.Setenv("CHANNEL_QUOTA_SYNC_ENABLED", "true")
	t.Setenv("CHANNEL_QUOTA_SYNC_INTERVAL", "30m")
	t.Setenv("CHANNEL_QUOTA_SYNC_MAX_CHANNELS", "12")
	require.True(t, handler.Enabled())
	require.Equal(t, 30*time.Minute, handler.Interval())
	require.Equal(t, 12, channelQuotaSnapshotSyncMaxChannelsConfigured())
}

func TestChannelQuotaSnapshotSyncRejectsHotOrInvalidConfiguration(t *testing.T) {
	handler := channelQuotaSnapshotSyncHandler{}
	for _, value := range []string{"0", "30s", "bad", "-5m"} {
		t.Setenv("CHANNEL_QUOTA_SYNC_INTERVAL", value)
		require.Equal(t, channelQuotaSnapshotSyncDefaultInterval, handler.Interval(), "interval=%s", value)
	}
	t.Setenv("CHANNEL_QUOTA_SYNC_INTERVAL", "5000h")
	require.Equal(t, 24*time.Hour, handler.Interval())

	for _, value := range []string{"0", "-1", "bad"} {
		t.Setenv("CHANNEL_QUOTA_SYNC_MAX_CHANNELS", value)
		require.Equal(t, channelQuotaSnapshotSyncDefaultMaxChannels, channelQuotaSnapshotSyncMaxChannelsConfigured(), "max=%s", value)
	}
	t.Setenv("CHANNEL_QUOTA_SYNC_MAX_CHANNELS", "999999")
	require.Equal(t, channelQuotaSnapshotSyncMaxChannels, channelQuotaSnapshotSyncMaxChannelsConfigured())
}
