package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
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
	setupChannelQuotaIdentityFixture(t, db)

	err = sampleCodexChannelUsage(context.Background(), &model.Channel{Id: 993, Type: constant.ChannelTypeCodex, Key: "not-json"})
	require.Error(t, err)
	var classified *channelQuotaSamplingError
	require.True(t, errors.As(err, &classified))
	require.Error(t, classified.QueryErr)
	require.Error(t, classified.PersistErr)
}

func TestCodexSamplingCompletionClockDoesNotReleaseFreshExhaustionAfterReset(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	setupChannelQuotaIdentityFixture(t, db)
	var clock atomic.Int64
	clock.Store(1700000000)
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("fixture_completion_clock", func(tx *gorm.DB) {
		if tx.Statement.SQL.String() == "SELECT strftime('%s','now')" {
			tx.Statement.SQL.Reset()
			tx.Statement.SQL.WriteString(fmt.Sprintf("SELECT %d", clock.Load()))
		}
	}))
	base := "https://quota-fixture.invalid"
	channel := model.Channel{Type: constant.ChannelTypeCodex, Key: `{"access_token":"synthetic","account_id":"clock-account"}`, BaseURL: &base, Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	client.Transport = quotaSamplingRoundTripper(func(r *http.Request) (*http.Response, error) {
		clock.Store(1700000010)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: r, Body: io.NopCloser(strings.NewReader(`{"rate_limit":{"primary_window":{"used_percent":100,"reset_at":1700000005,"limit_window_seconds":18000}}}`))}, nil
	})
	require.NoError(t, sampleCodexChannelUsage(context.Background(), &channel))
	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.First(&snapshot).Error)
	require.EqualValues(t, 1700000010, snapshot.ObservedAt)
	excluded, eligible, err := service.CodexQuotaEligibleKeys(context.Background(), &channel)
	require.NoError(t, err)
	require.False(t, eligible, "the zero observed after reset must remain exhausted")
	require.True(t, excluded[0])
}

func TestSampleCodexCredentialUsageStopsBeforeNetworkWhenDatabaseClockUnavailable(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	setupChannelQuotaIdentityFixture(t, db)
	credential := `{"access_token":"fixture-access","account_id":"fixture-account"}`
	identity, err := resolveQuotaSamplingIdentity(context.Background(), constant.ChannelTypeCodex, common.ChannelQuotaIdentityKindCredential, []byte(credential))
	require.NoError(t, err)
	clockErr := errors.New("database clock unavailable")
	const callbackName = "test:codex-sampler-database-clock"
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register(callbackName, func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "strftime('%s','now')") {
			tx.AddError(clockErr)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Row().Remove(callbackName)) })
	baseURL := "https://quota-fixture.invalid"
	channel := &model.Channel{Id: 995, Type: constant.ChannelTypeCodex, Key: credential, BaseURL: &baseURL}
	err = sampleCodexCredentialUsage(context.Background(), channel, credential, identity, nil, nil, nil)
	require.ErrorIs(t, err, clockErr)
	var classified *channelQuotaSamplingError
	require.True(t, errors.As(err, &classified))
	require.ErrorIs(t, classified.QueryErr, clockErr)
	require.NoError(t, classified.PersistErr)
	var count int64
	require.NoError(t, db.Model(&model.ChannelQuotaSnapshot{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestManualCodexUsageSnapshotUsesDatabaseClock(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	const callbackName = "test:manual-codex-fixed-database-clock"
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.SQL.String() == "SELECT strftime('%s','now')" {
			tx.Statement.SQL.Reset()
			tx.Statement.SQL.WriteString("SELECT 1700000000")
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Row().Remove(callbackName)) })
	channel := &model.Channel{Id: 996, Type: constant.ChannelTypeCodex}
	body := []byte(`{"rate_limit":{"primary_window":{"used_percent":100,"reset_at":1700000100,"limit_window_seconds":18000}}}`)
	require.NoError(t, recordManualCodexUsageSnapshots(context.Background(), channel,
		`{"access_token":"fixture-access","account_id":"manual-account"}`, "manual-account", http.StatusOK, body))
	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ? AND status = ?", channel.Id, "success").Take(&snapshot).Error)
	require.EqualValues(t, 1700000000, snapshot.ObservedAt)
	require.Zero(t, snapshot.Available)
}

func TestSampleCodexChannelUsageReportsPersistenceOnlyFailureSeparately(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	setupChannelQuotaIdentityFixture(t, db)

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
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	setupChannelQuotaIdentityFixture(t, db)
	const databaseNow int64 = 1700000000
	const callbackName = "test:codex-sampler-fixed-database-clock"
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.SQL.String() == "SELECT strftime('%s','now')" {
			tx.Statement.SQL.Reset()
			tx.Statement.SQL.WriteString("SELECT 1700000000")
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Row().Remove(callbackName)) })

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
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, sampleCodexChannelUsage(context.Background(), channel))

	var snapshots []model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Order("source ASC").Find(&snapshots).Error)
	require.Len(t, snapshots, 2)
	require.Equal(t, "codex_rate_limit", snapshots[0].MetricType)
	require.Equal(t, "success", snapshots[0].Status)
	require.Equal(t, "team", snapshots[0].PlanType)
	require.NotEmpty(t, snapshots[0].SampleID)
	require.Equal(t, snapshots[0].SampleID, snapshots[1].SampleID)
	require.NotNil(t, snapshots[0].Total)
	require.Equal(t, float64(100), *snapshots[0].Total)

	bySource := make(map[string]model.ChannelQuotaSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		bySource[snapshot.Source] = snapshot
		require.Equal(t, databaseNow, snapshot.ObservedAt)
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
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	setupChannelQuotaIdentityFixture(t, db)

	baseURL := "http://127.0.0.1:1"
	channel := &model.Channel{
		Id:      992,
		Type:    constant.ChannelTypeCodex,
		Key:     `{"access_token":"test-access","account_id":"account-1","type":"codex"}`,
		BaseURL: &baseURL,
	}
	require.NoError(t, db.Create(channel).Error)
	require.Error(t, sampleCodexChannelUsage(context.Background(), channel))

	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&snapshot).Error)
	require.Equal(t, "error", snapshot.Status)
	require.Equal(t, "upstream_transport", snapshot.ErrorCode)
	require.NotEmpty(t, snapshot.SampleID)
}

func TestSampleCodexChannelUsageClassifiesSuccessfulUnsupportedPayload(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	setupChannelQuotaIdentityFixture(t, db)

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

	require.NoError(t, db.Create(channel).Error)

	err = sampleCodexChannelUsage(context.Background(), channel)
	var classified *channelQuotaSamplingError
	require.ErrorAs(t, err, &classified)
	require.ErrorIs(t, classified.QueryErr, errChannelQuotaUnsupported)
	require.Nil(t, classified.PersistErr)

	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&snapshot).Error)
	require.Equal(t, "unsupported", snapshot.Status)
	require.Equal(t, "invalid_payload", snapshot.ErrorCode)
	require.Equal(t, model.ChannelQuotaIdentityQualityCredentialScoped, snapshot.IdentityQuality)
}
