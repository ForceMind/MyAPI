package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSmartRoutingChannelFailureIsIndependentFromRetryPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	channelError := types.NewError(errors.New("mapped model is invalid"), types.ErrorCodeChannelModelMappedError)
	skippedChannelError := types.NewError(errors.New("mapped model is invalid"), types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	badBody := types.NewError(errors.New("malformed upstream response"), types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())

	assert.True(t, isSmartRoutingChannelFailure(ctx, channelError))
	assert.True(t, isSmartRoutingChannelFailure(ctx, skippedChannelError))
	assert.True(t, isSmartRoutingChannelFailure(ctx, badBody))
	assert.True(t, shouldRetry(ctx, channelError, 1))
	assert.False(t, shouldRetry(ctx, skippedChannelError, 1))
	assert.False(t, shouldRetry(ctx, badBody, 1))
}

func TestSmartRoutingChannelFailureClassifiesUpstreamAndLocalOutcomes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	tests := []struct {
		name string
		err  *types.NewAPIError
		want bool
	}{
		{name: "upstream gateway timeout", err: types.NewErrorWithStatusCode(errors.New("gateway timeout"), types.ErrorCodeBadResponseStatusCode, http.StatusGatewayTimeout, types.ErrOptionWithSkipRetry()), want: true},
		{name: "cloudflare timeout", err: types.NewErrorWithStatusCode(errors.New("origin timeout"), types.ErrorCodeBadResponseStatusCode, 524, types.ErrOptionWithSkipRetry()), want: true},
		{name: "upstream rate limit", err: types.NewErrorWithStatusCode(errors.New("limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests), want: true},
		{name: "upstream unauthorized", err: types.NewErrorWithStatusCode(errors.New("invalid key"), types.ErrorCodeBadResponseStatusCode, http.StatusUnauthorized), want: true},
		{name: "upstream forbidden", err: types.NewErrorWithStatusCode(errors.New("account forbidden"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden), want: true},
		{name: "upstream client rejection", err: types.NewErrorWithStatusCode(errors.New("invalid input"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest), want: false},
		{name: "parsed provider server error", err: types.WithOpenAIError(types.OpenAIError{Message: "provider unavailable", Code: "provider_unavailable"}, http.StatusServiceUnavailable), want: true},
		{name: "parsed provider client error", err: types.WithOpenAIError(types.OpenAIError{Message: "invalid input", Code: "invalid_input"}, http.StatusBadRequest), want: false},
		{name: "request body", err: types.NewError(errors.New("bad request"), types.ErrorCodeReadRequestBodyFailed), want: false},
		{name: "request conversion", err: types.NewError(errors.New("unsupported request"), types.ErrorCodeConvertRequestFailed), want: false},
		{name: "local pricing", err: types.NewError(errors.New("price missing"), types.ErrorCodeModelPriceError), want: false},
		{name: "client cancellation", err: types.NewError(context.Canceled, types.ErrorCodeDoRequestFailed), want: false},
		{name: "downstream closed pipe", err: types.NewError(io.ErrClosedPipe, types.ErrorCodeBadResponse), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSmartRoutingChannelFailure(ctx, tt.err))
		})
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	canceledCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	canceledCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(canceled)
	assert.False(t, isSmartRoutingChannelFailure(canceledCtx,
		types.NewError(errors.New("transport stopped"), types.ErrorCodeDoRequestFailed)))
}

func TestSmartRoutingStreamFailureSeparatesUpstreamFromDownstreamTermination(t *testing.T) {
	tests := []struct {
		name       string
		reason     relaycommon.StreamEndReason
		wantFailed bool
		wantReason string
	}{
		{name: "upstream scanner", reason: relaycommon.StreamEndReasonScannerErr, wantFailed: true, wantReason: "upstream_stream_scanner_error"},
		{name: "upstream timeout", reason: relaycommon.StreamEndReasonTimeout, wantFailed: true, wantReason: "upstream_stream_timeout"},
		{name: "local stream handler panic", reason: relaycommon.StreamEndReasonPanic},
		{name: "client gone", reason: relaycommon.StreamEndReasonClientGone},
		{name: "downstream ping write", reason: relaycommon.StreamEndReasonPingFail},
		{name: "completed", reason: relaycommon.StreamEndReasonDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := relaycommon.NewStreamStatus()
			status.SetEndReason(tt.reason, errors.New("stream stopped"))
			failed, reason := smartRoutingStreamFailure(&relaycommon.RelayInfo{StreamStatus: status})
			assert.Equal(t, tt.wantFailed, failed)
			assert.Equal(t, tt.wantReason, reason)
		})
	}
}

func TestShouldRetryNeverOverridesSpecificChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Set("specific_channel_id", "9")
	channelError := types.NewError(errors.New("upstream failed"), types.ErrorCodeChannelModelMappedError)
	assert.False(t, shouldRetry(ctx, channelError, 1))
}
