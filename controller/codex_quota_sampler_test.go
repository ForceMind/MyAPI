package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSampleCodexChannelUsagePersistsNormalizedWindows(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/wham/usage", r.URL.Path)
		require.Equal(t, "Bearer test-access", r.Header.Get("Authorization"))
		require.Equal(t, "account-1", r.Header.Get("chatgpt-account-id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"plan_type":"team",
			"rate_limit":{
				"primary_window":{"used_percent":25,"reset_at":1900000000,"limit_window_seconds":18000},
				"secondary_window":{"used_percent":60,"reset_at":1900600000,"limit_window_seconds":604800}
			}
		}`))
	}))
	defer server.Close()

	baseURL := server.URL
	channel := &model.Channel{
		Id:      991,
		Type:    constant.ChannelTypeCodex,
		Key:     `{"access_token":"test-access","account_id":"account-1","type":"codex"}`,
		BaseURL: &baseURL,
	}
	require.NoError(t, sampleCodexChannelUsage(context.Background(), channel))

	var snapshots []model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Order("source ASC").Find(&snapshots).Error)
	require.Len(t, snapshots, 2)
	require.Equal(t, "codex_rate_limit", snapshots[0].MetricType)
	require.Equal(t, "success", snapshots[0].Status)
	require.Equal(t, "team", snapshots[0].PlanType)
	require.NotNil(t, snapshots[0].Total)
	require.Equal(t, float64(100), *snapshots[0].Total)

	bySource := make(map[string]model.ChannelQuotaSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		bySource[snapshot.Source] = snapshot
	}
	require.Equal(t, float64(75), bySource["codex_wham_usage_primary"].Available)
	require.Equal(t, "five_hour", bySource["codex_wham_usage_primary"].WindowType)
	require.Equal(t, float64(40), bySource["codex_wham_usage_secondary"].Available)
	require.Equal(t, "weekly", bySource["codex_wham_usage_secondary"].WindowType)
}

func TestSampleCodexChannelUsageRecordsTransportFailure(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	baseURL := "http://127.0.0.1:1"
	channel := &model.Channel{
		Id:      992,
		Type:    constant.ChannelTypeCodex,
		Key:     `{"access_token":"test-access","account_id":"account-1","type":"codex"}`,
		BaseURL: &baseURL,
	}
	require.Error(t, sampleCodexChannelUsage(context.Background(), channel))

	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&snapshot).Error)
	require.Equal(t, "error", snapshot.Status)
	require.Equal(t, "upstream_transport", snapshot.ErrorCode)
}
