package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSampleCodexChannelUsageSeparatesQueryAndPersistenceFailures(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// Deliberately do not migrate the snapshots table. A malformed credential
	// still emits a normalized failure marker, and the sampler must expose both
	// the provider/credential error and the write error to its caller.
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	err = sampleCodexChannelUsage(context.Background(), &model.Channel{Id: 993, Type: constant.ChannelTypeCodex, Key: "not-json"})
	require.Error(t, err)
	var classified *channelQuotaSamplingError
	require.True(t, errors.As(err, &classified))
	require.Error(t, classified.QueryErr)
	require.Error(t, classified.PersistErr)
}

func TestSampleCodexChannelUsageReportsPersistenceOnlyFailureSeparately(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":10,"reset_at":1900000000,"limit_window_seconds":18000}}}`))
	}))
	defer server.Close()
	channel := &model.Channel{
		Id:      994,
		Type:    constant.ChannelTypeCodex,
		Key:     `{"access_token":"test-access","account_id":"account-1","type":"codex"}`,
		BaseURL: func() *string { value := server.URL; return &value }(),
	}

	err = sampleCodexChannelUsage(context.Background(), channel)
	require.Error(t, err)
	var classified *channelQuotaSamplingError
	require.True(t, errors.As(err, &classified))
	require.NoError(t, classified.QueryErr)
	require.Error(t, classified.PersistErr)
}

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

func TestSampleCodexChannelUsageClassifiesSuccessfulUnsupportedPayload(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// A provider/login proxy may return 2xx while omitting the official
		// rate-limit object. This must not count as a sampled quota point.
		_, _ = w.Write([]byte(`{"message":"login required"}`))
	}))
	defer server.Close()
	baseURL := server.URL
	channel := &model.Channel{
		Id:      990,
		Type:    constant.ChannelTypeCodex,
		Key:     `{"access_token":"test-access","account_id":"account-1","type":"codex"}`,
		BaseURL: &baseURL,
	}

	err = sampleCodexChannelUsage(context.Background(), channel)
	var classified *channelQuotaSamplingError
	require.ErrorAs(t, err, &classified)
	require.ErrorIs(t, classified.QueryErr, errChannelQuotaUnsupported)
	require.Nil(t, classified.PersistErr)

	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&snapshot).Error)
	require.Equal(t, "unsupported", snapshot.Status)
	require.Equal(t, "invalid_payload", snapshot.ErrorCode)
}
