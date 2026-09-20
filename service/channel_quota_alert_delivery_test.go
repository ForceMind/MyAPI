package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type channelQuotaAlertRecordingClient struct {
	statusCode int
	err        error
	requests   []*http.Request
	bodies     [][]byte
}

type channelQuotaAlertLeaseRaceClient struct {
	now               *time.Time
	competitorClaimed int
	requests          int
}

func (client *channelQuotaAlertLeaseRaceClient) Do(_ *http.Request) (*http.Response, error) {
	client.requests++
	*client.now = client.now.Add(channelQuotaAlertDeliveryTimeout)
	if client.requests == 7 {
		events, err := model.ClaimChannelQuotaAlertEvents(
			context.Background(), "competing-worker", client.now.Unix(),
			int64(channelQuotaAlertDeliveryLease/time.Second), 1,
		)
		if err != nil {
			return nil, err
		}
		client.competitorClaimed = len(events)
	}
	return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
}

func (client *channelQuotaAlertRecordingClient) Do(req *http.Request) (*http.Response, error) {
	client.requests = append(client.requests, req)
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	client.bodies = append(client.bodies, body)
	if client.err != nil {
		return nil, client.err
	}
	return &http.Response{
		StatusCode: client.statusCode,
		Body:       io.NopCloser(strings.NewReader("ignored response body")),
		Header:     make(http.Header),
	}, nil
}

func setupChannelQuotaAlertDeliveryEvent(t *testing.T) (*gorm.DB, model.ChannelQuotaAlertEvent) {
	t.Helper()
	previousDB := model.DB
	common.OptionMapRWMutex.Lock()
	previousEnabled := common.ChannelQuotaAlertEnabled
	previousWarning := common.ChannelQuotaAlertWarningPercent
	previousCritical := common.ChannelQuotaAlertCriticalPercent
	previousCooldown := common.ChannelQuotaAlertCooldownSeconds
	previousRecovery := common.ChannelQuotaAlertNotifyOnRecovery
	common.ChannelQuotaAlertEnabled = true
	common.ChannelQuotaAlertWarningPercent = 20
	common.ChannelQuotaAlertCriticalPercent = 10
	common.ChannelQuotaAlertCooldownSeconds = 3600
	common.ChannelQuotaAlertNotifyOnRecovery = true
	common.OptionMapRWMutex.Unlock()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.ChannelQuotaAlertState{}, &model.ChannelQuotaAlertEvent{}))
	model.DB = db
	require.NoError(t, db.Create(&model.Channel{Id: 71}).Error)
	t.Cleanup(func() {
		model.DB = previousDB
		common.OptionMapRWMutex.Lock()
		common.ChannelQuotaAlertEnabled = previousEnabled
		common.ChannelQuotaAlertWarningPercent = previousWarning
		common.ChannelQuotaAlertCriticalPercent = previousCritical
		common.ChannelQuotaAlertCooldownSeconds = previousCooldown
		common.ChannelQuotaAlertNotifyOnRecovery = previousRecovery
		common.OptionMapRWMutex.Unlock()
	})

	total := 100.0
	require.NoError(t, model.RecordChannelQuotaSnapshotBatchWithContext(context.Background(), []model.ChannelQuotaSnapshot{{
		ChannelId: 71, AccountRef: model.ChannelQuotaAccountRef("codex", "account-a"),
		ObservedAt: 1000, SampleID: "sample-alert-delivery", Available: 5, Total: &total,
		MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary",
		Unit: "percent", WindowSeconds: 18000, ResetAt: 19000, Status: "success",
	}}, model.ChannelQuotaSnapshotBatchOptions{}))
	var event model.ChannelQuotaAlertEvent
	require.NoError(t, db.First(&event).Error)
	return db, event
}

func newTestChannelQuotaAlertDeliveryWorker(client channelQuotaAlertHTTPDoer) *ChannelQuotaAlertDeliveryWorker {
	return &ChannelQuotaAlertDeliveryWorker{
		workerID: "test-worker", client: client, now: func() time.Time { return time.Unix(2000, 0) }, batchSize: 10,
		policyEnabled: func() bool { return true },
		settings: func() (setting.ChannelQuotaAlertDeliverySettings, error) {
			return setting.ChannelQuotaAlertDeliverySettings{
				WebhookURL: "https://alerts.example.com/hooks/quota", WebhookSecret: "0123456789abcdef",
			}, nil
		},
	}
}

func TestChannelQuotaAlertDeliveryWorkerSendsSignedRedactedPayloadAndMarksDelivered(t *testing.T) {
	db, event := setupChannelQuotaAlertDeliveryEvent(t)
	client := &channelQuotaAlertRecordingClient{statusCode: http.StatusNoContent}
	worker := newTestChannelQuotaAlertDeliveryWorker(client)

	summary, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, ChannelQuotaAlertDeliverySummary{Enabled: true, Claimed: 1, Delivered: 1}, summary)
	require.Len(t, client.requests, 1)
	require.Equal(t, http.MethodPost, client.requests[0].Method)
	require.Equal(t, event.EventKey, client.requests[0].Header.Get("X-MyAPI-Event-ID"))
	digest := hmac.New(sha256.New, []byte("0123456789abcdef"))
	_, _ = digest.Write(client.bodies[0])
	require.Equal(t, "sha256="+hex.EncodeToString(digest.Sum(nil)), client.requests[0].Header.Get("X-MyAPI-Signature"))
	require.NotContains(t, string(client.bodies[0]), "account-a")
	require.NotContains(t, string(client.bodies[0]), "0123456789abcdef")
	require.Contains(t, string(client.bodies[0]), `"channel_id":71`)
	require.Contains(t, string(client.bodies[0]), `"available":5`)

	var stored model.ChannelQuotaAlertEvent
	require.NoError(t, db.First(&stored, event.ID).Error)
	require.Equal(t, "delivered", stored.State)
	require.Equal(t, 1, stored.AttemptCount)
	require.NotNil(t, stored.DeliveredAt)
}

func TestChannelQuotaAlertDeliveryWorkerRetriesThenQuarantinesWithoutNetworkFixtures(t *testing.T) {
	t.Run("retryable", func(t *testing.T) {
		db, event := setupChannelQuotaAlertDeliveryEvent(t)
		worker := newTestChannelQuotaAlertDeliveryWorker(&channelQuotaAlertRecordingClient{statusCode: http.StatusServiceUnavailable})
		summary, err := worker.RunOnce(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, summary.Retryable)
		var stored model.ChannelQuotaAlertEvent
		require.NoError(t, db.First(&stored, event.ID).Error)
		require.Equal(t, "retryable", stored.State)
		require.Equal(t, "http_503", stored.LastErrorCode)
		require.Equal(t, int64(2030), stored.NextAttemptAt)
	})

	t.Run("quarantined at bounded attempt", func(t *testing.T) {
		db, event := setupChannelQuotaAlertDeliveryEvent(t)
		require.NoError(t, db.Model(&model.ChannelQuotaAlertEvent{}).Where("id = ?", event.ID).Update("attempt_count", model.ChannelQuotaAlertMaxAttempts()-1).Error)
		worker := newTestChannelQuotaAlertDeliveryWorker(&channelQuotaAlertRecordingClient{err: errors.New("synthetic network failure")})
		summary, err := worker.RunOnce(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, summary.Quarantined)
		var stored model.ChannelQuotaAlertEvent
		require.NoError(t, db.First(&stored, event.ID).Error)
		require.Equal(t, "quarantined", stored.State)
		require.Equal(t, model.ChannelQuotaAlertMaxAttempts(), stored.AttemptCount)
		require.Equal(t, "network_error", stored.LastErrorCode)
		require.Zero(t, stored.NextAttemptAt)
	})
}

func TestChannelQuotaAlertDeliveryWorkerClaimsEachEventImmediatelyBeforeSending(t *testing.T) {
	_, event := setupChannelQuotaAlertDeliveryEvent(t)
	total := 100.0
	for index := 1; index < 7; index++ {
		available := 90.0
		if index%2 == 0 {
			available = 5
		}
		require.NoError(t, model.RecordChannelQuotaSnapshotBatchWithContext(context.Background(), []model.ChannelQuotaSnapshot{{
			ChannelId: 71, AccountRef: model.ChannelQuotaAccountRef("codex", "account-a"),
			ObservedAt: int64(1000 + index), SampleID: fmt.Sprintf("sample-alert-delivery-%d", index), Available: available, Total: &total,
			MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary",
			Unit: "percent", WindowSeconds: 18000, ResetAt: 19000, Status: "success",
		}}, model.ChannelQuotaSnapshotBatchOptions{}))
	}

	now := time.Unix(2000, 0)
	client := &channelQuotaAlertLeaseRaceClient{now: &now}
	worker := newTestChannelQuotaAlertDeliveryWorker(client)
	worker.batchSize = 10
	worker.now = func() time.Time { return now }

	summary, err := worker.RunOnce(context.Background())

	require.NoError(t, err)
	require.Zero(t, client.competitorClaimed, "each in-flight event receives a fresh lease, so a competing worker cannot claim the seventh normal-timeout delivery")
	require.Equal(t, 7, client.requests)
	require.Equal(t, ChannelQuotaAlertDeliverySummary{Enabled: true, Claimed: 7, Delivered: 7}, summary)
	var delivered model.ChannelQuotaAlertEvent
	require.NoError(t, model.DB.First(&delivered, event.ID).Error)
	require.Equal(t, "delivered", delivered.State)
}

func TestChannelQuotaAlertDeliveryWorkerDoesNotClaimWhenDisabledOrUnsafe(t *testing.T) {
	for _, test := range []struct {
		name          string
		policyEnabled bool
		settings      setting.ChannelQuotaAlertDeliverySettings
		expectError   bool
	}{
		{name: "policy disabled", settings: setting.ChannelQuotaAlertDeliverySettings{}, policyEnabled: false},
		{name: "webhook absent", settings: setting.ChannelQuotaAlertDeliverySettings{}, policyEnabled: true},
		{name: "private target", settings: setting.ChannelQuotaAlertDeliverySettings{WebhookURL: "https://127.0.0.1/hook", WebhookSecret: "0123456789abcdef"}, policyEnabled: true, expectError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, event := setupChannelQuotaAlertDeliveryEvent(t)
			client := &channelQuotaAlertRecordingClient{statusCode: http.StatusNoContent}
			worker := newTestChannelQuotaAlertDeliveryWorker(client)
			worker.policyEnabled = func() bool { return test.policyEnabled }
			worker.settings = func() (setting.ChannelQuotaAlertDeliverySettings, error) { return test.settings, nil }

			_, err := worker.RunOnce(context.Background())
			if test.expectError {
				require.ErrorIs(t, err, ErrChannelQuotaAlertDeliveryDisabled)
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, client.requests)
			var stored model.ChannelQuotaAlertEvent
			require.NoError(t, db.First(&stored, event.ID).Error)
			require.Equal(t, "pending", stored.State)
			require.Zero(t, stored.AttemptCount)
		})
	}
}

func TestChannelQuotaAlertWebhookClientRejectsRedirects(t *testing.T) {
	client := newChannelQuotaAlertWebhookHTTPClient()
	req, err := http.NewRequest(http.MethodPost, "https://redirect.example.com/hook", nil)
	require.NoError(t, err)
	err = client.CheckRedirect(req, nil)
	require.ErrorIs(t, err, errChannelQuotaAlertRedirectBlocked)
}

func TestChannelQuotaAlertWebhookClientRejectsPrivateDNSBeforeDial(t *testing.T) {
	dialed := false
	client := newChannelQuotaAlertWebhookHTTPClientWithDialer(
		staticSSRFResolver{"alerts.example.com": {{IP: net.ParseIP("127.0.0.1")}}},
		func(context.Context, string, string) (net.Conn, error) {
			dialed = true
			return nil, errors.New("dial must not run")
		},
	)
	req, err := http.NewRequest(http.MethodPost, "https://alerts.example.com/hook", strings.NewReader("{}"))
	require.NoError(t, err)
	response, err := client.Do(req)
	require.Error(t, err)
	require.Nil(t, response)
	require.False(t, dialed)
	require.Contains(t, err.Error(), "private IP address not allowed")
}
