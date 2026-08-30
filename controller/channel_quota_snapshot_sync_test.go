package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateChannelBalanceWithPollingLockSerializesSameChannel(t *testing.T) {
	// Use an unsupported channel type so the call returns without touching a
	// database or provider. Holding the shared lock first proves that manual
	// balance requests wait for the sampler's in-flight operation instead of
	// issuing a concurrent query.
	channel := &model.Channel{Id: 996, Type: constant.ChannelTypeAzure}
	lock := model.GetChannelPollingLock(channel.Id)
	lock.Lock()
	released := false
	defer func() {
		if !released {
			lock.Unlock()
		}
	}()

	done := make(chan error, 1)
	go func() {
		_, err, _ := updateChannelBalanceWithPollingLock(channel)
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("balance request bypassed the polling lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	lock.Unlock()
	released = true
	select {
	case err := <-done:
		require.ErrorIs(t, err, errChannelQuotaUnsupported)
	case <-time.After(time.Second):
		t.Fatal("balance request did not proceed after polling lock was released")
	}
}

func TestWithChannelPollingLockKeepsSnapshotWorkInCriticalSection(t *testing.T) {
	channelID := 997
	lock := model.GetChannelPollingLock(channelID)
	lock.Lock()
	released := false
	defer func() {
		if !released {
			lock.Unlock()
		}
	}()

	started := make(chan struct{})
	finished := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		withChannelPollingLock(channelID, func() {
			close(started)
			<-finished // stand in for provider query + snapshot persistence
		})
	}()

	select {
	case <-started:
		t.Fatal("channel operation entered while its polling lock was held")
	case <-time.After(20 * time.Millisecond):
	}
	lock.Unlock()
	released = true

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("channel operation did not enter after polling lock was released")
	}
	close(finished)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("channel operation did not finish after snapshot work was released")
	}
}

func TestChannelQuotaSnapshotSyncSamplesStandardChannelAndRecordsFailure(t *testing.T) {
	previousDB := model.DB
	previousRequestInterval := common.RequestInterval
	common.RequestInterval = 0
	t.Cleanup(func() {
		model.DB = previousDB
		common.RequestInterval = previousRequestInterval
	})

	for _, test := range []struct {
		name       string
		statusCode int
	}{
		{name: "success", statusCode: http.StatusOK},
		{name: "upstream failure", statusCode: http.StatusBadGateway},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
			model.DB = db

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.statusCode != http.StatusOK {
					w.WriteHeader(test.statusCode)
					return
				}
				switch r.URL.Path {
				case "/v1/dashboard/billing/subscription":
					_, _ = w.Write([]byte(`{"object":"billing_subscription","has_payment_method":true,"hard_limit_usd":10}`))
				case "/v1/dashboard/billing/usage":
					_, _ = w.Write([]byte(`{"object":"list","total_usage":100}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			baseURL := server.URL
			channel := &model.Channel{
				Type:    constant.ChannelTypeCustom,
				Status:  common.ChannelStatusEnabled,
				Name:    "standard-subscription",
				Key:     "test-key",
				BaseURL: &baseURL,
			}
			require.NoError(t, db.Create(channel).Error)

			summary, err := runChannelQuotaSnapshotSyncOnce(context.Background(), 1, nil)
			if test.statusCode == http.StatusOK {
				require.NoError(t, err)
				require.Equal(t, 1, summary.Sampled)
				require.Zero(t, summary.Failed)
			} else {
				// Provider failures are represented by an error snapshot and a
				// Failed count; the bounded task itself still completes so one
				// unavailable channel does not abort the remaining batch.
				require.NoError(t, err)
				require.Zero(t, summary.Sampled)
				require.Equal(t, 1, summary.Failed)
			}

			var snapshot model.ChannelQuotaSnapshot
			require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&snapshot).Error)
			require.Equal(t, "balance", snapshot.MetricType)
			require.Equal(t, "none", snapshot.WindowType)
			require.Equal(t, "channel_type_8", snapshot.Source)
			if test.statusCode == http.StatusOK {
				require.Equal(t, "success", snapshot.Status)
				require.Equal(t, 9.0, snapshot.Available)
			} else {
				require.Equal(t, "error", snapshot.Status)
				require.Equal(t, "query_failed", snapshot.ErrorCode)
			}
		})
	}
}

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
