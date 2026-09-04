package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relay/channel/codex"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type quotaSamplingRoundTripper func(*http.Request) (*http.Response, error)

func (transport quotaSamplingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestCodexSamplingSharesOneDeadlineAcrossUsageRefreshAndRetry(t *testing.T) {
	previousDB := model.DB
	previousMemoryCache := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.SystemTaskLock{}))
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCache
	})
	baseURL := "https://quota-fixture.invalid"
	channel := &model.Channel{
		Id: 42, Type: constant.ChannelTypeCodex, BaseURL: &baseURL,
		Key: `{"access_token":"fixture-access","refresh_token":"fixture-refresh","account_id":"fixture-account"}`,
	}
	require.NoError(t, db.Create(channel).Error)
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })

	var deadlines []time.Time
	var requestPaths []string
	client.Transport = quotaSamplingRoundTripper(func(request *http.Request) (*http.Response, error) {
		deadline, present := request.Context().Deadline()
		require.True(t, present, "all sampling requests must have a finite deadline")
		deadlines = append(deadlines, deadline)
		requestPaths = append(requestPaths, request.URL.Path)
		status := http.StatusOK
		body := `{"rate_limit":{"primary_window":{"used_percent":12,"limit_window_seconds":18000}}}`
		if len(requestPaths) == 1 {
			status = http.StatusUnauthorized
			body = `{}`
		} else if request.URL.Path == "/oauth/token" {
			body = `{"access_token":"fixture-replacement","refresh_token":"fixture-refresh-replacement","expires_in":3600}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})

	before := time.Now()
	require.NoError(t, sampleCodexChannelUsage(context.Background(), channel))
	require.Equal(t, []string{"/backend-api/wham/usage", "/oauth/token", "/backend-api/wham/usage"}, requestPaths)
	require.Len(t, deadlines, 3)
	require.True(t, deadlines[0].After(before))
	require.LessOrEqual(t, deadlines[0].Sub(before), 21*time.Second)
	require.Equal(t, deadlines[0], deadlines[2], "retry must not restart the sample budget")
	require.False(t, deadlines[1].After(deadlines[0]), "credential refresh must fit inside the same budget")
	var snapshots []model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Find(&snapshots).Error)
	require.Len(t, snapshots, 1)
	require.Equal(t, "success", snapshots[0].Status)
}

func TestCodexSamplingHonorsShorterCallerDeadlineAndRecordsTimeout(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	callerDeadline := time.Now().Add(5 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), callerDeadline)
	defer cancel()
	client.Transport = quotaSamplingRoundTripper(func(request *http.Request) (*http.Response, error) {
		deadline, present := request.Context().Deadline()
		require.True(t, present)
		require.Equal(t, callerDeadline, deadline)
		return nil, context.DeadlineExceeded
	})
	baseURL := "https://quota-fixture.invalid"
	err = sampleCodexChannelUsage(ctx, &model.Channel{
		Id: 43, Type: constant.ChannelTypeCodex, BaseURL: &baseURL,
		Key: `{"access_token":"fixture-access","account_id":"fixture-account"}`,
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", 43).First(&snapshot).Error)
	require.Equal(t, "error", snapshot.Status)
	require.Equal(t, "upstream_timeout", snapshot.ErrorCode)
}

func TestCodexSamplingRefreshWriteFailureIsReportedAndSafelyRecorded(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.SystemTaskLock{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	baseURL := "https://quota-fixture.invalid"
	channel := &model.Channel{Id: 44, Type: constant.ChannelTypeCodex, BaseURL: &baseURL,
		Key: `{"access_token":"fixture-access","refresh_token":"fixture-refresh","account_id":"fixture-account"}`}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fixture_reject_credential_update", func(tx *gorm.DB) {
		tx.AddError(errors.New("fixture credential write unavailable"))
	}))
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	requests := 0
	client.Transport = quotaSamplingRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests++
		status, body := http.StatusUnauthorized, `{}`
		if request.URL.Path == "/oauth/token" {
			status, body = http.StatusOK, `{"access_token":"fixture-replacement","refresh_token":"fixture-rotated","expires_in":3600}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	sampleErr := sampleCodexChannelUsage(context.Background(), channel)
	var classified *channelQuotaSamplingError
	require.ErrorAs(t, sampleErr, &classified)
	require.Error(t, classified.PersistErr)
	require.Error(t, classified.QueryErr)
	require.Equal(t, 2, requests, "do not report a successful retry while rotating credentials were lost")
	require.NotContains(t, sampleErr.Error(), "fixture-rotated")
	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.First(&snapshot).Error)
	require.Equal(t, "error", snapshot.Status)
	require.Equal(t, "credential_persist_failed", snapshot.ErrorCode)
}

func TestManualCodexUsageFailsAndRecordsCredentialPersistenceError(t *testing.T) {
	previousDB, previousCache := model.DB, common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.SystemTaskLock{}))
	model.DB, common.MemoryCacheEnabled = db, false
	t.Cleanup(func() { model.DB, common.MemoryCacheEnabled = previousDB, previousCache })
	baseURL := "https://manual-quota-fixture.invalid"
	channel := &model.Channel{Id: 46, Type: constant.ChannelTypeCodex, BaseURL: &baseURL,
		Key: `{"access_token":"fixture-access","refresh_token":"fixture-refresh","account_id":"fixture-account"}`}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fixture_reject_manual_credential_update", func(tx *gorm.DB) {
		tx.AddError(errors.New("fixture credential write unavailable"))
	}))
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	client.Transport = quotaSamplingRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"fixture-replacement","refresh_token":"fixture-rotated","expires_in":3600}`)),
			Request:    request,
		}, nil
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/api/channel/46/codex/usage", nil)
	ginContext.Params = gin.Params{{Key: "id", Value: "46"}}
	fetchCalls := 0
	fetch := func(context.Context, *http.Client, string, string, string) (int, []byte, error) {
		fetchCalls++
		return http.StatusUnauthorized, []byte(`{}`), nil
	}

	fetchCodexChannelWhamData(ginContext, fetch, "fixture usage", "safe user message", true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), "safe user message")
	assert.NotContains(t, recorder.Body.String(), "fixture-rotated")
	assert.Equal(t, 1, fetchCalls, "a persistence failure must stop before retrying usage")
	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&snapshot).Error)
	assert.Equal(t, "credential_persist_failed", snapshot.ErrorCode)
	var lockCount int64
	require.NoError(t, db.Model(&model.SystemTaskLock{}).Count(&lockCount).Error)
	assert.Zero(t, lockCount)
}

func TestCodexSamplingSavesRotatedCredentialsAfterRequestCancellation(t *testing.T) {
	previousDB, previousCache := model.DB, common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.SystemTaskLock{}))
	model.DB, common.MemoryCacheEnabled = db, false
	t.Cleanup(func() { model.DB, common.MemoryCacheEnabled = previousDB, previousCache })
	baseURL := "https://quota-fixture.invalid"
	channel := &model.Channel{Id: 45, Type: constant.ChannelTypeCodex, BaseURL: &baseURL,
		Key: `{"access_token":"fixture-access","refresh_token":"fixture-refresh","account_id":"fixture-account"}`}
	require.NoError(t, db.Create(channel).Error)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writes := 0
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fixture_inspect_credential_context", func(tx *gorm.DB) {
		writes++
		require.ErrorIs(t, ctx.Err(), context.Canceled)
		require.NoError(t, tx.Statement.Context.Err())
		deadline, present := tx.Statement.Context.Deadline()
		require.True(t, present)
		require.LessOrEqual(t, time.Until(deadline), channelQuotaPersistenceTimeout)
	}))
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	client.Transport = quotaSamplingRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Context().Err() != nil {
			return nil, request.Context().Err()
		}
		status, body := http.StatusUnauthorized, `{}`
		if request.URL.Path == "/oauth/token" {
			cancel()
			status, body = http.StatusOK, `{"access_token":"fixture-replacement","refresh_token":"fixture-rotated","expires_in":3600}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	sampleErr := sampleCodexChannelUsage(ctx, channel)
	require.ErrorIs(t, sampleErr, context.Canceled)
	require.Equal(t, 1, writes)
	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	storedKey, err := codex.ParseOAuthKey(stored.Key)
	require.NoError(t, err)
	require.Equal(t, "fixture-rotated", storedKey.RefreshToken)
	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.First(&snapshot).Error)
	require.Equal(t, "sampling_canceled", snapshot.ErrorCode)
}
