package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiStreamActualUsageRequiresTerminalEvidence(t *testing.T) {
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() { constant.StreamingTimeout = old })
	const partial = `{"candidates":[{"index":0,"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13}}`
	const terminal = `{"candidates":[{"index":0,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`
	for _, tc := range []struct {
		name       string
		chunks     []string
		unresolved bool
		zero       bool
	}{
		{"nonterminal", []string{partial}, true, false},
		{"terminal", []string{partial, terminal}, false, false},
		{"late usage", []string{`{"candidates":[{"index":0,"finishReason":"STOP"}]}`, `{"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`}, false, false},
		{"stale usage before terminal", []string{partial, `{"candidates":[{"index":0,"finishReason":"STOP"}]}`}, true, false},
		{"regressing usage", []string{partial, `{"candidates":[{"index":0,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"totalTokenCount":12}}`}, true, false},
		{"new candidate after terminal", []string{terminal, `{"candidates":[{"index":1,"content":{"parts":[{"text":"late candidate"}]}}]}`}, true, false},
		{"null tail", []string{terminal, `null`}, true, false},
		{"malformed tail", []string{terminal, `{not json}`}, true, false},
		{"incomplete tail", []string{terminal, `{"usageMetadata":{}}`}, true, false},
		{"explicit zero", []string{`{"candidates":[{"index":0,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0,"totalTokenCount":0}}`}, false, true},
		{"unfinished second candidate", []string{`{"candidates":[{"index":0,"finishReason":"STOP"},{"index":1,"content":{"parts":[{"text":"partial"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{OriginModelName: "gemini-contract", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-contract"}}
			info.SetEstimatePromptTokens(99)
			var body strings.Builder
			for _, chunk := range tc.chunks {
				body.WriteString("data: " + chunk + "\n\n")
			}
			body.WriteString("data: [DONE]\n\n")
			usage, apiErr := geminiStreamHandler(ctx, info, &http.Response{Body: io.NopCloser(strings.NewReader(body.String()))}, func(string, *dto.GeminiChatResponse) bool { return true })
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.NotNil(t, usage.BillingUsage)
			assert.Equal(t, tc.unresolved, usage.BillingUsage.Incomplete || usage.BillingUsage.Estimated)
			if tc.zero {
				assert.Zero(t, usage.PromptTokens)
				assert.Zero(t, usage.CompletionTokens)
				assert.False(t, usage.BillingUsage.Estimated)
			}
		})
	}
}

func TestGeminiNonstreamExplicitZeroDoesNotBecomeFallbackEstimate(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		output    int
	}{
		{"all zero", `{"promptTokenCount":0,"candidatesTokenCount":0,"totalTokenCount":0}`, 0},
		{"zero input", `{"promptTokenCount":0,"candidatesTokenCount":5,"totalTokenCount":5}`, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response dto.GeminiChatResponse
			require.NoError(t, common.Unmarshal([]byte(`{"candidates":[{"content":{"parts":[{"text":"provider-reported zero input"}]}}],"usageMetadata":`+tc.raw+`}`), &response))
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-contract"}}
			info.SetEstimatePromptTokens(99)
			usage := buildUsageFromGeminiResponse(ctx, info, &response)
			assert.Zero(t, usage.PromptTokens)
			assert.Equal(t, tc.output, usage.CompletionTokens)
			require.NotNil(t, usage.BillingUsage)
			assert.False(t, usage.BillingUsage.Incomplete)
			assert.False(t, usage.BillingUsage.Estimated)
		})
	}
}
