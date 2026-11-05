package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdvancedBalanceCapabilityPreservesConfigurationAndQueryFailures(t *testing.T) {
	for _, test := range []struct {
		name        string
		settings    string
		status      int
		unsupported bool
		requests    int32
	}{
		{name: "valid relay without balance route", settings: `{"advanced_custom":{"advanced_routes":[{"incoming_path":"/v1/chat/completions","upstream_path":"/chat"}]}}`, status: http.StatusOK, unsupported: true},
		{name: "malformed settings remain an error", settings: `{"advanced_custom":`, status: http.StatusOK},
		{name: "missing required configuration remains an error", settings: `{}`, status: http.StatusOK},
		{name: "invalid configuration remains an error", settings: `{"advanced_custom":{"advanced_routes":[{"incoming_path":"invalid","upstream_path":"/chat"}]}}`, status: http.StatusOK},
		{name: "configured endpoint failure remains an error", settings: `{"advanced_custom":{"advanced_routes":[{"incoming_path":"/v1/dashboard/billing/credit_grants","upstream_path":"/balance"}]}}`, status: http.StatusBadGateway, requests: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, "/balance", r.URL.Path)
				w.WriteHeader(test.status)
			}))
			defer server.Close()
			baseURL := server.URL
			channel := &model.Channel{Type: constant.ChannelTypeAdvancedCustom, BaseURL: &baseURL, Key: "capability-fixture", OtherSettings: test.settings}
			_, err := fetchAdvancedCustomBalance(context.Background(), channel)
			require.Error(t, err)
			if test.unsupported {
				require.ErrorIs(t, err, errChannelQuotaUnsupported)
			} else {
				require.NotErrorIs(t, err, errChannelQuotaUnsupported)
			}
			require.Equal(t, test.requests, requests.Load())
			require.Equal(t, test.settings, channel.OtherSettings, "a capability check must not rewrite saved settings")
		})
	}
}

func TestAdvancedBalanceWithoutQueryRoutePersistsNotConfigured(t *testing.T) {
	previousDB := model.DB
	previousInterval := common.RequestInterval
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.ChannelQuotaSamplingTarget{}))
	setupChannelQuotaIdentityFixture(t, db)
	model.DB = db
	common.RequestInterval = 0
	t.Cleanup(func() { model.DB = previousDB; common.RequestInterval = previousInterval })
	channel := &model.Channel{Type: constant.ChannelTypeAdvancedCustom, Status: common.ChannelStatusEnabled, Key: "capability-fixture"}
	channel.SetOtherSettings(dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/chat/completions", UpstreamPath: "/chat"}}}})
	require.NoError(t, db.Create(channel).Error)
	summary, err := runChannelQuotaSnapshotSyncOnce(context.Background(), 1, nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Unsupported)
	require.Zero(t, summary.Sampled)
	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&snapshot).Error)
	require.Equal(t, "unsupported", snapshot.Status)
	require.Equal(t, "quota_not_configured", snapshot.ErrorCode)
	var target model.ChannelQuotaSamplingTarget
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&target).Error)
	require.Equal(t, "unsupported", target.LastResult)
	require.Empty(t, target.ConfirmedSubjectRef)

	// Manual sampling uses the same classification and never fabricates a balance.
	require.NoError(t, recordChannelBalanceSnapshot(channel, channelBalanceResult{}, errChannelBalanceNotConfigured))
	var manual model.ChannelQuotaSnapshot
	require.NoError(t, db.Order("id DESC").First(&manual).Error)
	require.Equal(t, "unsupported", manual.Status)
	require.Equal(t, "quota_not_configured", manual.ErrorCode)
}

func TestQuotaChangeProjectsLatestCapabilityReasonWithoutDroppingErrors(t *testing.T) {
	rows := []model.ChannelQuotaAggregateRow{
		{ID: 1, ChannelID: 7, ObservedAt: 100, Status: "error", ErrorCode: "query_failed", MetricType: "balance", Unit: "usd"},
		{ID: 2, ChannelID: 7, ObservedAt: 200, Status: "unsupported", ErrorCode: "quota_not_configured", MetricType: "balance", Unit: "usd"},
	}
	item := buildQuotaChangeItem(rows)
	require.Equal(t, "unsupported", item.Status)
	require.Equal(t, "quota_not_configured", item.ErrorCode)
	require.Equal(t, 1, item.DataQuality.ErrorCount)
	require.Equal(t, 1, item.DataQuality.UnsupportedCount)
	require.Nil(t, item.CurrentAvailable)
	payload, err := common.Marshal(item)
	require.NoError(t, err)
	var projected map[string]interface{}
	require.NoError(t, common.Unmarshal(payload, &projected))
	require.Equal(t, "quota_not_configured", projected["error_code"])
	require.NotContains(t, projected, "current_available")
	rows = append(rows, model.ChannelQuotaAggregateRow{ID: 3, ChannelID: 7, ObservedAt: 300, Status: "success", Available: 5, MetricType: "balance", Unit: "usd"})
	item = buildQuotaChangeItem(rows)
	require.Empty(t, item.ErrorCode, "a new observation must not inherit an older failure reason")
}
