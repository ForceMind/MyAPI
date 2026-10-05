package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const chatBudgetBody = `{"model":"gpt-6.1-sol","messages":[{"role":"user","content":"hello"}],"max_completion_tokens":20,"service_tier":"default"}`

func chatBudgetOutbound(t *testing.T, body string) (*http.Request, *relaycommon.RelayInfo) {
	t.Helper()
	request, info := tokenBudgetOutboundFixture(t, body)
	request.URL.Path = "/v1/chat/completions"
	info.UpstreamModelName = model.TokenBudgetOpenAIChatModel
	return request, info
}

func TestTokenBudgetChatFinalPayloadBound(t *testing.T) {
	client := tokenBudgetCountTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("Chat qualification must not call the Responses count endpoint")
	})
	request, info := chatBudgetOutbound(t, chatBudgetBody)
	info.SetEstimatePromptTokens(1)
	bound, err := CountTokenBudgetBound(context.Background(), client, request, info)
	require.NoError(t, err)
	assert.Equal(t, model.TokenBudgetBoundOpenAIChat, bound.BoundSource)
	assert.Equal(t, model.TokenBudgetOpenAIChatContext, bound.InputTokens)
	assert.EqualValues(t, 20, bound.MaxOutputTokens)
	assert.Nil(t, request.GetBody)
	body, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	assert.Equal(t, chatBudgetBody, string(body))
	for _, body := range []string{
		strings.Replace(chatBudgetBody, "gpt-6.1-sol", "gpt-6.1-sol-alias", 1),
		strings.Replace(chatBudgetBody, "max_completion_tokens", "max_tokens", 1),
		strings.Replace(chatBudgetBody, ":20", ":128001", 1),
		strings.Replace(chatBudgetBody, ":20", ":0", 1),
		strings.Replace(chatBudgetBody, `"content":"hello"`, `"content":null`, 1),
		strings.Replace(chatBudgetBody, `"content":"hello"`, `"content":[{"type":"image_url","image_url":"https://example.invalid/a"}]`, 1),
		strings.Replace(chatBudgetBody, `"service_tier":"default"`, `"service_tier":"standard"`, 1),
		strings.Replace(chatBudgetBody, `"service_tier":"default"`, `"n":2`, 1),
		strings.Replace(chatBudgetBody, `"service_tier":"default"`, `"stream":true`, 1),
		strings.Replace(chatBudgetBody, `"service_tier":"default"`, `"tools":[]`, 1),
		strings.Replace(chatBudgetBody, `"model":`, `"Model":`, 1),
	} {
		req, relay := chatBudgetOutbound(t, body)
		_, err := CountTokenBudgetBound(context.Background(), client, req, relay)
		require.ErrorIs(t, err, ErrTokenBudgetUnsupported, body)
	}
	for _, change := range []func(*http.Request, *relaycommon.RelayInfo){
		func(r *http.Request, _ *relaycommon.RelayInfo) { r.URL.Host = "compatible.invalid" },
		func(r *http.Request, _ *relaycommon.RelayInfo) { r.URL.RawQuery = "x=1" },
		func(r *http.Request, _ *relaycommon.RelayInfo) { r.Header.Set("OpenAI-Beta", "unreviewed") },
		func(_ *http.Request, i *relaycommon.RelayInfo) {
			i.ParamOverride = map[string]interface{}{"model": "other"}
		},
	} {
		req, relay := chatBudgetOutbound(t, chatBudgetBody)
		change(req, relay)
		_, err := CountTokenBudgetBound(context.Background(), client, req, relay)
		require.ErrorIs(t, err, ErrTokenBudgetUnsupported)
	}
}

func chatBudgetUsage(input, output int) *dto.Usage {
	raw := &dto.Usage{PromptTokens: input, CompletionTokens: output, TotalTokens: input + output}
	evidence := &dto.ChatTextEvidence{Model: model.TokenBudgetOpenAIChatModel, ServiceTier: "default", PromptTokens: input, CompletionTokens: output, TotalTokens: input + output, CacheRead: common.GetPointer(0), CacheWrite: common.GetPointer(0)}
	return &dto.Usage{BillingUsage: &dto.BillingUsage{Source: dto.BillingUsageSourceOAIChat, Semantic: dto.BillingUsageSemanticOpenAI, OpenAIUsage: raw, ChatTextEvidence: evidence}}
}

func TestStrictChatBudgetReportedUnknownAndRecoveryBothWriters(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, unknown := range []bool{false, true} {
			t.Run(string(mode)+map[bool]string{false: "-actual", true: "-unknown"}[unknown], func(t *testing.T) {
				db, ctx, info, root := strictTokenBudgetFixture(t, mode)
				_, err := model.ConfigureTokenBudget(context.Background(), db, root, model.TokenBudgetPolicyInput{ID: strings.Repeat("d", 64), TokenID: info.TokenId, ExpectedRevision: 1, Enabled: true, Limit: model.TokenBudgetOpenAIChatContext})
				require.NoError(t, err)
				info.RelayMode, info.UpstreamModelName, info.OriginModelName = relayconstant.RelayModeChatCompletions, model.TokenBudgetOpenAIChatModel, model.TokenBudgetOpenAIChatModel
				ctx.Request.URL.Path = "/v1/chat/completions"
				request, _ := chatBudgetOutbound(t, chatBudgetBody)
				client := tokenBudgetCountTestClient(t, func(http.ResponseWriter, *http.Request) { t.Error("no count call") })
				require.NoError(t, PrepareTokenBudgetDispatch(ctx, client, request, info))
				usage := chatBudgetUsage(10, 5)
				if unknown {
					usage.BillingUsage.ChatTextEvidence = nil
				}
				PostTextConsumeQuota(ctx, info, usage, nil)
				assert.Equal(t, unknown, FinalizeTokenBudgetDispatch(ctx, info))
				policy, err := model.LookupTokenBudget(context.Background(), db, info.TokenId)
				require.NoError(t, err)
				if unknown {
					assert.Equal(t, model.TokenBudgetOpenAIChatContext, policy.Reserved)
					assert.Zero(t, policy.Used)
					assert.False(t, info.Billing.NeedsRefund())
					_, err := model.PrepareTokenBudgetUsageReview(context.Background(), db, root, info.TokenId, info.RequestId)
					require.NoError(t, err)
					for range 2 {
						view, err := model.ReconcileUsageReview(context.Background(), db, root, info.RequestId, 20, "synthetic Chat actual usage", model.UsageReviewTokenCounts{Input: 10, Output: 5})
						require.NoError(t, err)
						require.NoError(t, model.ProjectUsageReviewDecision(context.Background(), db, db, view.Decision.ID))
					}
				}
				policy, err = model.LookupTokenBudget(context.Background(), db, info.TokenId)
				require.NoError(t, err)
				assert.EqualValues(t, 15, policy.Used)
				assert.Zero(t, policy.Reserved)
				var logs []model.Log
				require.NoError(t, db.Where("request_id = ? AND type = ?", info.RequestId, model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1)
				if !unknown {
					assert.Contains(t, logs[0].Other, `"bound_source":"openai_chat_context_window"`)
					assert.Contains(t, logs[0].Other, `"actual_input":10`)
				}
			})
		}
	}
}

func TestStrictChatTokenTierQualificationAndProtocolIdentity(t *testing.T) {
	row := &model.TokenBudgetReservation{ModelName: model.TokenBudgetOpenAIChatModel, RequestServiceTier: "default"}
	for _, tier := range []string{"", "auto", "priority", "flex", "standard", "default"} {
		usage := chatBudgetUsage(10, 5)
		usage.BillingUsage.ChatTextEvidence.ServiceTier = tier
		_, err := qualifiedChatBudgetUsage(row, usage)
		if tier == "default" {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, model.ErrTokenBudgetPending)
		}
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req, info := tokenBudgetOutboundFixture(t, `{"model":"other-responses-model","input":"hi","max_output_tokens":20}`)
	info.StrictTokenBudget = true
	require.ErrorIs(t, PrepareTokenBudgetDispatch(ctx, &http.Client{}, req, info), ErrTokenBudgetUnsupported, "Chat cannot acquire Responses qualification through conversion")
}
