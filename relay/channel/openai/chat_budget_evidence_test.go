package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const chatQualifiedUsage = `{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"prompt_tokens_details":{"cached_tokens":40,"cache_write_tokens":30},"completion_tokens_details":{"reasoning_tokens":4}}`

func chatQualifiedResponse(usage string, stream bool) string {
	object, choices := "chat.completion", `[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]`
	if stream {
		object, choices = "chat.completion.chunk", `[]`
	}
	return `{"id":"synthetic","object":"` + object + `","model":"gpt-6.1-sol","service_tier":"default","choices":` + choices + `,"usage":` + usage + `}`
}

func TestStrictChatNativeRawEvidenceAndZero(t *testing.T) {
	for _, stream := range []bool{false, true} {
		valid := chatQualifiedResponse(chatQualifiedUsage, stream)
		evidence := captureChatTextEvidence([]byte(valid))
		require.NotNil(t, evidence)
		assert.Equal(t, 100, evidence.PromptTokens)
		assert.Equal(t, 30, *evidence.CacheWrite)
		zero := chatQualifiedResponse(`{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0,"prompt_tokens_details":{"cached_tokens":0,"cache_write_tokens":0}}`, stream)
		require.NotNil(t, captureChatTextEvidence([]byte(zero)))
		for _, bad := range []string{
			strings.Replace(valid, `"prompt_tokens":100`, `"prompt_tokens":100,"Prompt_Tokens":100`, 1),
			strings.Replace(valid, `"model":"gpt-6.1-sol"`, `"model":"gpt-6.1-sol","MODEL":"gpt-6.1-sol"`, 1),
			strings.Replace(valid, `"cached_tokens":40`, `"cached_tokens":40,"cached_tokens":0`, 1),
			strings.Replace(valid, `"total_tokens":110`, `"total_tokens":0`, 1),
			strings.Replace(valid, `"cache_write_tokens":30`, `"cache_write_tokens":61`, 1),
			strings.Replace(valid, `"reasoning_tokens":4`, `"reasoning_tokens":11`, 1),
			strings.Replace(valid, `"reasoning_tokens":4`, `"reasoning_tokens":4,"accepted_prediction_tokens":1`, 1),
			strings.Replace(valid, `"cached_tokens":40`, `"cached_tokens":40,"audio_tokens":1`, 1),
			strings.Replace(valid, `"prompt_tokens":100`, `"prompt_tokens":null`, 1),
			strings.TrimSuffix(valid, "}") + `,"error":{"type":"server_error","message":"interrupted"}}`,
			strings.TrimSuffix(valid, "}") + `,"ERROR":{"type":"server_error"}}`,
		} {
			assert.Nil(t, captureChatTextEvidence([]byte(bad)), bad)
		}
		missing := strings.Replace(valid, `,"cache_write_tokens":30`, "", 1)
		evidence = captureChatTextEvidence([]byte(missing))
		require.NotNil(t, evidence)
		assert.Nil(t, evidence.CacheWrite, "omitted fee category remains absent")
	}
	original := &dto.BillingUsage{ChatTextEvidence: captureChatTextEvidence([]byte(chatQualifiedResponse(chatQualifiedUsage, false)))}
	clone := dto.CloneBillingUsage(original)
	*clone.ChatTextEvidence.CacheWrite = 99
	assert.Equal(t, 30, *original.ChatTextEvidence.CacheWrite)

}

func TestStrictChatAdapterStreamRequiresTerminalUnambiguousUsage(t *testing.T) {
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = old })
	content := `data: {"id":"synthetic","object":"chat.completion.chunk","model":"gpt-6.1-sol","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}],"usage":null}` + "\n\n"
	usage := "data: " + chatQualifiedResponse(chatQualifiedUsage, true) + "\n\n"
	for _, tc := range []struct {
		name, body string
		valid      bool
		cancel     bool
	}{
		{name: "terminal", body: content + usage + "data: [DONE]\n\n", valid: true},
		{name: "bare EOF", body: content + usage},
		{name: "malformed terminator", body: content + usage + "data: [DONE]garbage\n\n"},
		{name: "repeated usage", body: content + usage + usage + "data: [DONE]\n\n"},
		{name: "content after usage", body: usage + content + "data: [DONE]\n\n"},
		{name: "malformed intermediate", body: "data: {bad}\n\n" + usage + "data: [DONE]\n\n"},
		{name: "error with valid usage", body: "data: " + strings.TrimSuffix(chatQualifiedResponse(chatQualifiedUsage, true), "}") + `,"error":{"type":"server_error"}}` + "\n\ndata: [DONE]\n\n"},
		{name: "error with valid intermediate", body: strings.Replace(content, `"usage":null`, `"usage":null,"error":{"type":"server_error"}`, 1) + usage + "data: [DONE]\n\n"},
		{name: "alternate intermediate tier", body: strings.Replace(content, `"usage":null`, `"usage":null,"service_tier":"priority"`, 1) + usage + "data: [DONE]\n\n"},
		{name: "client canceled", body: content + usage + "data: [DONE]\n\n", cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			if tc.cancel {
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: model.TokenBudgetOpenAIChatModel}, StrictTokenBudget: true, IsStream: true, DisablePing: true, RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: types.RelayFormatOpenAI}
			response, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(tc.body))})
			require.Nil(t, apiErr)
			require.NotNil(t, response.BillingUsage)
			qualified := response.BillingUsage.ChatTextEvidence != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone && !info.StreamStatus.HasErrors() && c.Request.Context().Err() == nil
			assert.Equal(t, tc.valid, qualified)
		})
	}
}

func TestStrictChatRawEvidenceDoesNotAdoptCompatibilityCache(t *testing.T) {
	body := strings.Replace(chatQualifiedResponse(chatQualifiedUsage, false), `"cached_tokens":40`, `"cached_tokens":0`, 1)
	body = strings.TrimSuffix(body, "}") + `,"timings":{"cache_n":40}}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}, RelayFormat: types.RelayFormatOpenAI}
	usage, apiErr := OpenaiHandler(c, info, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, apiErr)
	require.NotNil(t, usage.BillingUsage.ChatTextEvidence)
	assert.Zero(t, *usage.BillingUsage.ChatTextEvidence.CacheRead)
	assert.Equal(t, 40, usage.BillingUsage.OpenAIUsage.PromptTokensDetails.CachedTokens, "existing ordinary compatibility behavior retained; strict service rejects disagreement")
	encoded, err := common.Marshal(usage.BillingUsage)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"cache_read":0`)
}

type finalChatWriteFailure struct {
	*httptest.ResponseRecorder
	match  string
	failed bool
}

func (w *finalChatWriteFailure) Write(data []byte) (int, error) {
	if strings.Contains(string(data), w.match) {
		w.failed = true
		return 0, io.ErrClosedPipe
	}
	return w.ResponseRecorder.Write(data)
}
func (w *finalChatWriteFailure) WriteString(data string) (int, error) { return w.Write([]byte(data)) }

func TestStrictChatFinalUsageAndDoneWriteErrorsPreventSettlement(t *testing.T) {
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = old })
	for _, match := range []string{`"prompt_tokens":100`, "[DONE]"} {
		t.Run(match, func(t *testing.T) {
			writer := &finalChatWriteFailure{ResponseRecorder: httptest.NewRecorder(), match: match}
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: model.TokenBudgetOpenAIChatModel}, StrictTokenBudget: true, IsStream: true, DisablePing: true, ShouldIncludeUsage: true, RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: types.RelayFormatOpenAI}
			body := "data: " + chatQualifiedResponse(chatQualifiedUsage, true) + "\n\ndata: [DONE]\n\n"
			usage, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))})
			require.Nil(t, apiErr)
			require.NotNil(t, usage.BillingUsage)
			require.True(t, writer.failed)
			assert.True(t, info.StreamStatus.HasErrors(), "even a terminal [DONE] cannot erase a subsequent downstream write failure")
		})
	}
}

type chatDeadlineWriter struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *chatDeadlineWriter) SetWriteDeadline(value time.Time) error { w.deadline = value; return nil }
func TestStrictChatWriteObserverPreservesResponseControllerDeadline(t *testing.T) {
	writer := &chatDeadlineWriter{ResponseRecorder: httptest.NewRecorder()}
	ctx, _ := gin.CreateTestContext(writer)
	observed := &strictChatWriteObserver{ResponseWriter: ctx.Writer}
	expected := time.Date(2026, 10, 5, 0, 0, 30, 0, time.UTC)
	require.NoError(t, http.NewResponseController(observed).SetWriteDeadline(expected))
	assert.Equal(t, expected, writer.deadline)
}
