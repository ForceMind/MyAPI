package controller

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelQuotaSamplingStatusIsReadOnlyAndReflectsConfiguration(t *testing.T) {
	t.Setenv("CHANNEL_QUOTA_SYNC_ENABLED", "true")
	t.Setenv("CHANNEL_QUOTA_SYNC_INTERVAL", "30m")
	t.Setenv("CHANNEL_QUOTA_SYNC_MAX_CHANNELS", "12")

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("GET", "/api/channel/quota/status", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = req
	GetChannelQuotaSamplingStatus(ctx)

	if recorder.Code != 200 {
		t.Fatalf("status code = %d, want 200", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"enabled":true`) || !strings.Contains(body, `"interval_seconds":1800`) || !strings.Contains(body, `"max_channels":12`) {
		t.Fatalf("unexpected status response: %s", body)
	}
	if strings.Contains(body, "secret") || strings.Contains(body, "token") {
		t.Fatalf("sampling status leaked sensitive fields: %s", body)
	}
}

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
