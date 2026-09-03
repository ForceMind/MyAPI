package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type quotaBalanceResponseBody struct {
	io.Reader
	closed bool
}

func (body *quotaBalanceResponseBody) Close() error {
	body.closed = true
	return nil
}

type quotaSamplingErrorReader struct{ err error }

func (reader quotaSamplingErrorReader) Read([]byte) (int, error) { return 0, reader.err }

func TestQuotaBalanceResponseIsBoundedAndOversizeErrorIsSafe(t *testing.T) {
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	for _, size := range []int{maxChannelBalanceResponseBytes, maxChannelBalanceResponseBytes + 128} {
		reader := strings.NewReader(strings.Repeat("x", size))
		body := &quotaBalanceResponseBody{Reader: reader}
		client.Transport = quotaSamplingRoundTripper(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: request}, nil
		})
		result, readErr := GetResponseBody(context.Background(), http.MethodGet, "https://quota-fixture.invalid/balance?credential=fixture-private", &model.Channel{}, nil)
		require.True(t, body.closed)
		if size == maxChannelBalanceResponseBytes {
			require.NoError(t, readErr)
			require.Len(t, result, size)
		} else {
			require.Nil(t, result)
			require.EqualError(t, readErr, "balance response exceeds 262144 bytes")
			require.Equal(t, maxChannelBalanceResponseBytes+1, size-reader.Len())
			require.NotContains(t, readErr.Error(), "fixture-private")
		}
	}
}

func TestQuotaBalanceDatabaseFailureRemainsPersistenceFailure(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fixture_reject_balance_update", func(tx *gorm.DB) {
		tx.AddError(errors.New("fixture balance write unavailable"))
	}))
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	client.Transport = quotaSamplingRoundTripper(func(request *http.Request) (*http.Response, error) {
		body := `{"has_payment_method":true,"hard_limit_usd":10}`
		if strings.HasSuffix(request.URL.Path, "/usage") {
			body = `{"total_usage":100}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	baseURL := "https://quota-fixture.invalid"
	channel := &model.Channel{Id: 31, Type: constant.ChannelTypeCustom, BaseURL: &baseURL, Key: "fixture"}
	result, sampleErr := updateChannelBalanceWithContext(context.Background(), channel)
	var classified *channelQuotaSamplingError
	require.ErrorAs(t, sampleErr, &classified)
	require.NoError(t, classified.QueryErr)
	require.Error(t, classified.PersistErr)
	require.Equal(t, 9.0, result.Balance)
	require.NoError(t, recordChannelBalanceSnapshot(channel, result, classified.QueryErr))
	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.First(&snapshot).Error)
	require.Equal(t, "success", snapshot.Status, "a database write failure must not falsify the successful provider observation")
}

func TestAdvancedQuotaSamplingPreservesTimeoutWithoutLeakingRequestURL(t *testing.T) {
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	baseURL := "https://quota-fixture.invalid"
	settings, err := common.Marshal(dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{
		Routes: []dto.AdvancedCustomRoute{{
			IncomingPath: dto.AdvancedCustomBalancePath,
			UpstreamPath: "/provider/balance",
			Auth:         &dto.AdvancedCustomRouteAuth{Type: dto.AdvancedCustomAuthTypeQuery, Name: "credential", Value: "{api_key}"},
		}},
	}})
	require.NoError(t, err)
	channel := &model.Channel{Type: constant.ChannelTypeAdvancedCustom, BaseURL: &baseURL, Key: "fixture-confidential-value", OtherSettings: string(settings)}
	for _, failDuringBody := range []bool{false, true} {
		client.Transport = quotaSamplingRoundTripper(func(request *http.Request) (*http.Response, error) {
			if !failDuringBody {
				return nil, context.DeadlineExceeded
			}
			return &http.Response{
				StatusCode: http.StatusOK, Header: make(http.Header), Request: request,
				Body: io.NopCloser(quotaSamplingErrorReader{err: context.DeadlineExceeded}),
			}, nil
		})
		_, queryErr := updateChannelBalanceWithContext(context.Background(), channel)
		require.ErrorIs(t, queryErr, context.DeadlineExceeded)
		require.NotContains(t, queryErr.Error(), channel.Key)
		require.NotContains(t, queryErr.Error(), "credential=")
	}
}

func TestQuotaBalanceSamplingSharesCallerDeadlineAndClosesRejectedResponse(t *testing.T) {
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	deadline := time.Now().Add(5 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	var bodies []*quotaBalanceResponseBody
	client.Transport = quotaSamplingRoundTripper(func(request *http.Request) (*http.Response, error) {
		requestDeadline, present := request.Context().Deadline()
		require.True(t, present)
		require.Equal(t, deadline, requestDeadline)
		status := http.StatusOK
		payload := `{"has_payment_method":true,"hard_limit_usd":10}`
		if len(bodies) == 1 {
			status = http.StatusBadGateway
			payload = "provider temporarily unavailable"
		}
		body := &quotaBalanceResponseBody{Reader: strings.NewReader(payload)}
		bodies = append(bodies, body)
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: body, Request: request}, nil
	})
	baseURL := "https://quota-fixture.invalid"
	_, err = updateChannelBalanceWithContext(ctx, &model.Channel{Type: constant.ChannelTypeCustom, BaseURL: &baseURL, Key: "fixture"})
	require.Error(t, err)
	require.Len(t, bodies, 2)
	require.True(t, bodies[0].closed)
	require.True(t, bodies[1].closed, "non-200 provider response must close its body")
}

func TestQuotaSnapshotSyncStopsAfterCanceledAttemptAndPersistsFailure(t *testing.T) {
	previousDB := model.DB
	previousInterval := common.RequestInterval
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	model.DB = db
	common.RequestInterval = 0
	t.Cleanup(func() {
		model.DB = previousDB
		common.RequestInterval = previousInterval
	})
	baseURL := "https://quota-fixture.invalid"
	channels := []model.Channel{
		{Id: 10, Type: constant.ChannelTypeCustom, BaseURL: &baseURL, Key: "fixture", Status: common.ChannelStatusEnabled},
		{Id: 11, Type: constant.ChannelTypeCustom, BaseURL: &baseURL, Key: "fixture", Status: common.ChannelStatusEnabled},
	}
	require.NoError(t, db.Create(&channels).Error)
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := 0
	client.Transport = quotaSamplingRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests++
		cancel()
		return nil, request.Context().Err()
	})
	summary, err := runChannelQuotaSnapshotSyncOnce(ctx, 2, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, requests)
	require.Equal(t, 1, summary.Failed)
	require.Equal(t, 1, summary.Deferred)
	var snapshots []model.ChannelQuotaSnapshot
	require.NoError(t, db.Find(&snapshots).Error)
	require.Len(t, snapshots, 1)
	require.Equal(t, 10, snapshots[0].ChannelId)
	require.Equal(t, "sampling_canceled", snapshots[0].ErrorCode)
	next, err := model.GetChannelsForQuotaSnapshotSync(1)
	require.NoError(t, err)
	require.Len(t, next, 1)
	require.Equal(t, 11, next[0].Id)
}

func TestQuotaSamplingRunBudgetUsesConfiguredCadenceWithHardMaximum(t *testing.T) {
	t.Setenv("CHANNEL_QUOTA_SYNC_INTERVAL", "1m")
	handler := channelQuotaSnapshotSyncHandler{}
	require.Equal(t, time.Minute, handler.RunTimeout())
	t.Setenv("CHANNEL_QUOTA_SYNC_INTERVAL", "3m")
	require.Equal(t, 3*time.Minute, handler.RunTimeout())
	t.Setenv("CHANNEL_QUOTA_SYNC_INTERVAL", "1h")
	require.Equal(t, 5*time.Minute, handler.RunTimeout())
}

func TestQuotaSamplingDefaultAndLargeMinuteValuesRemainBounded(t *testing.T) {
	require.Equal(t, time.Minute, parseChannelQuotaSyncInterval("invalid"))
	require.Equal(t, time.Minute, parseChannelQuotaSyncInterval("30s"))
	require.Equal(t, 24*time.Hour, parseChannelQuotaSyncInterval("9223372036854775807"))
}
