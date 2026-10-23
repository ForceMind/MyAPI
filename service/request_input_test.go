package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestInputEstimateDoesNotBecomeBillingUsage(t *testing.T) {
	request := &dto.OpenAIResponsesRequest{}
	require.NoError(t, common.UnmarshalJsonStr(`{"model":"gpt-6.1-sol","input":"hello world"}`, request))
	info := &relaycommon.RelayInfo{OriginModelName: "gpt-6.1-sol", Request: request, RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeCodex}}
	info.SetEstimatePromptTokens(99)
	old := constant.CountToken
	constant.CountToken = false
	t.Cleanup(func() { constant.CountToken = old })
	PrepareRequestInputEstimate(nil, info)
	require.NotNil(t, info.RequestInputEstimate)
	assert.EqualValues(t, 9, info.RequestInputEstimate.InputTokens, "two text tokens, one item frame and terminal framing")
	assert.True(t, info.RequestInputEstimate.Complete)
	assert.Equal(t, "o200k_base_text_estimate", info.RequestInputEstimate.Method)
	assert.Equal(t, 99, info.GetEstimatePromptTokens(), "diagnostics must not change billing input")
	other := map[string]interface{}{}
	AppendRequestInputDiagnostics(nil, info, other)
	assert.Equal(t, info.RequestInputEstimate, other["request_input_estimate"])
	assert.NotContains(t, other, "usage_outcome")
}

func TestRequestInputChecksSelectedContextWithoutChangingRoutingState(t *testing.T) {
	require.NoError(t, i18n.Init())
	old := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = false
	t.Cleanup(func() { constant.ErrorLogEnabled = old })
	for _, existing := range []*relaycommon.ChannelMeta{nil, {
		ChannelId:      1,
		ChannelSetting: dto.ChannelSettings{InputTokenLimits: map[string]int64{"gpt-6.1-sol": 100}},
	}} {
		request := &dto.OpenAIResponsesRequest{}
		require.NoError(t, common.UnmarshalJsonStr(`{"model":"client-alias","input":"hello world"}`, request))
		info := &relaycommon.RelayInfo{OriginModelName: "gpt-6.1-sol", Request: request, RelayMode: relayconstant.RelayModeResponses, ChannelMeta: existing}
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		common.SetContextKey(ctx, constant.ContextKeyChannelId, 2)
		common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeCodex)
		common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{InputTokenLimits: map[string]int64{"gpt-6.1-sol": 8}})

		PrepareRequestInputEstimate(ctx, info)
		inputErr := EnforceRequestInputLimit(ctx, info)
		require.NotNil(t, inputErr)
		assert.Equal(t, types.ErrorCode("request_input_limit_exceeded"), inputErr.GetErrorCode())
		require.NotNil(t, info.RequestInputEstimate)
		assert.EqualValues(t, 8, info.RequestInputEstimate.LimitTokens, "use the selected channel, including retries")
		assert.Equal(t, existing, info.ChannelMeta, "diagnostics must not advance channel selection or change the initial hold")
		assert.Equal(t, "client-alias", request.Model, "preflight must not initialize protocol conversion")
	}
}

func TestRequestInputLimitExactPolicyAndRetryChannel(t *testing.T) {
	require.NoError(t, i18n.Init())
	old := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = false
	t.Cleanup(func() { constant.ErrorLogEnabled = old })
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{OriginModelName: "gpt-6.1-sol", ChannelMeta: &relaycommon.ChannelMeta{},
		RequestInputEstimate: &relaycommon.RequestInputEstimate{InputTokens: 100, Complete: true}}
	for _, tc := range []struct {
		name     string
		limits   map[string]int64
		complete bool
		code     string
	}{
		{"no policy", nil, true, ""},
		{"other exact model does not match", map[string]int64{"gpt-6.1-sol-alias": 50}, true, ""},
		{"boundary allowed", map[string]int64{"gpt-6.1-sol": 100}, true, ""},
		{"smaller retry channel limit blocks", map[string]int64{"gpt-6.1-sol": 99}, true, "request_input_limit_exceeded"},
		{"incomplete media estimate blocks configured limit", map[string]int64{"gpt-6.1-sol": 200}, false, "request_input_limit_unavailable"},
		{"incomplete estimate without policy preserves admission", nil, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info.ChannelSetting.InputTokenLimits = tc.limits
			info.RequestInputEstimate.Complete = tc.complete
			err := EnforceRequestInputLimit(ctx, info)
			if tc.code == "" {
				require.Nil(t, err)
				return
			}
			require.NotNil(t, err)
			assert.Equal(t, types.ErrorCode(tc.code), err.GetErrorCode())
			assert.Equal(t, http.StatusBadRequest, err.StatusCode)
			assert.True(t, types.IsSkipRetryError(err))
		})
	}
}

func TestRequestInputEstimateIncludesToolPayloadAndRejectsUncountedContext(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		complete   bool
		minimum    int64
	}{
		{"function result", `{"input":[{"type":"function_call_output","call_id":"synthetic","output":"hello world"}]}`, true, 10},
		{"inline file", `{"input":[{"role":"user","content":[{"type":"input_file","file_data":"data:application/pdf;base64,AAAA"}]}]}`, false, 0},
		{"encrypted reasoning", `{"input":[{"type":"reasoning","encrypted_content":"synthetic-opaque"}]}`, false, 0},
		{"stored history", `{"input":"hello","previous_response_id":"synthetic-prior-response"}`, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := &dto.OpenAIResponsesRequest{}
			require.NoError(t, common.UnmarshalJsonStr(tc.body, request))
			info := &relaycommon.RelayInfo{OriginModelName: "gpt-6.1-sol", Request: request, RelayMode: relayconstant.RelayModeResponses, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeCodex}}
			PrepareRequestInputEstimate(nil, info)
			require.NotNil(t, info.RequestInputEstimate)
			assert.Equal(t, tc.complete, info.RequestInputEstimate.Complete)
			assert.GreaterOrEqual(t, info.RequestInputEstimate.InputTokens, tc.minimum)
		})
	}
}

func TestUnknownUsageLogRetainsInputEstimateAndSafeErrorAcrossWriters(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			require.NoError(t, db.AutoMigrate(&model.Log{}))
			oldLogDB := model.LOG_DB
			model.LOG_DB = db
			t.Cleanup(func() { model.LOG_DB = oldLogDB })
			user, token := seedAuthoritativeBilling(t, db, "request-input-log", 1000, 1000, false)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := authoritativeRelay(user, token, "request-input-log")
			info.ChannelMeta = &relaycommon.ChannelMeta{}
			info.StartTime = time.Now()
			ctx.Set(common.RequestIdKey, info.RequestId)
			session, apiErr := NewBillingSession(ctx, info, 100)
			require.Nil(t, apiErr)
			info.Billing = session
			request, err := http.NewRequest(http.MethodPost, "https://synthetic.invalid/responses", nil)
			require.NoError(t, err)
			require.NoError(t, PrepareTextUsageDispatch(ctx, request, info))
			info.RequestInputEstimate = &relaycommon.RequestInputEstimate{InputTokens: 922000, Method: "o200k_base_text_estimate", Complete: true}
			info.UpstreamErrorCode = "context_length_exceeded"
			require.True(t, FinalizeTextUsageDispatch(ctx, info))
			var logs []model.Log
			require.NoError(t, db.Where("request_id = ?", info.RequestId).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Zero(t, logs[0].PromptTokens)
			assert.Zero(t, logs[0].CompletionTokens)
			assert.Contains(t, logs[0].Other, `"input_tokens":922000`)
			assert.Contains(t, logs[0].Other, `"upstream_error_code":"context_length_exceeded"`)
			assert.Contains(t, logs[0].Other, `"actual_quota":null`)
			assert.Contains(t, logs[0].Other, `"usage_accuracy":"unknown"`)
		})
	}
}
