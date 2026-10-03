package openai

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
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesUsageEvidenceDistinguishesMissingCountersFromReportedZero(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, handler := range []struct {
		name   string
		stream bool
		run    func(*gin.Context, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{"responses", false, func(c *gin.Context, r *http.Response) (*dto.Usage, *types.NewAPIError) {
			return OaiResponsesHandler(c, &relaycommon.RelayInfo{}, r)
		}},
		{"compaction", false, OaiResponsesCompactionHandler},
		{"stream", true, func(c *gin.Context, r *http.Response) (*dto.Usage, *types.NewAPIError) {
			return OaiResponsesStreamHandler(c, &relaycommon.RelayInfo{DisablePing: true}, r)
		}},
	} {
		for _, sample := range []struct {
			name, raw  string
			incomplete bool
		}{
			{"empty object", `{}`, true},
			{"missing output", `{"input_tokens":10}`, true},
			{"missing input", `{"output_tokens":10}`, true},
			{"null counter", `{"input_tokens":null,"output_tokens":0}`, true},
			{"contradictory zero total", `{"input_tokens":10,"output_tokens":0,"total_tokens":0}`, true},
			{"explicit zero", `{"input_tokens":0,"output_tokens":0,"total_tokens":0}`, false},
			{"two actual counters", `{"input_tokens":10,"output_tokens":0}`, false},
		} {
			t.Run(handler.name+"/"+sample.name, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				body := `{"usage":` + sample.raw + `}`
				contentType := "application/json"
				if handler.stream {
					body = "data: {\"type\":\"response.completed\",\"response\":" + body + "}\n\ndata: [DONE]\n\n"
					contentType = "text/event-stream"
				}
				usage, apiErr := handler.run(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))})
				require.Nil(t, apiErr)
				require.NotNil(t, usage.BillingUsage)
				encoded, err := common.Marshal(usage.BillingUsage)
				require.NoError(t, err)
				var evidence map[string]any
				require.NoError(t, common.Unmarshal(encoded, &evidence))
				assert.Equal(t, sample.incomplete, evidence["incomplete"] == true, "missing fields must not be promoted to reported zero usage")
				assert.False(t, usage.BillingUsage.Estimated, "partial upstream evidence remains distinct from a local estimate")
			})
		}
	}
}

func TestChatUsageEvidencePreservesZeroAndPartialCounters(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, stream := range []bool{false, true} {
		for _, sample := range []struct {
			name, raw string
			partial   bool
		}{
			{"explicit zero", `{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}`, false},
			{"zero input", `{"prompt_tokens":0,"completion_tokens":4,"total_tokens":4}`, false},
			{"empty", `{}`, true},
			{"missing input", `{"completion_tokens":4}`, true},
			{"missing output", `{"prompt_tokens":10}`, true},
			{"null input", `{"prompt_tokens":null,"completion_tokens":4}`, true},
			{"contradictory total", `{"prompt_tokens":10,"completion_tokens":4,"total_tokens":0}`, true},
		} {
			name := "json/"
			if stream {
				name = "stream/"
			}
			t.Run(name+sample.name, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}, DisablePing: true, RelayFormat: types.RelayFormatOpenAI}
				info.SetEstimatePromptTokens(99)
				body := `{"id":"fixture","choices":[],"usage":` + sample.raw + `}`
				contentType := "application/json"
				handler := OpenaiHandler
				if stream {
					body = "data: " + body + "\n\ndata: [DONE]\n\n"
					contentType = "text/event-stream"
					handler = OaiStreamHandler
				}
				usage, apiErr := handler(c, info, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))})
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				require.NotNil(t, usage.BillingUsage)
				assert.Equal(t, sample.partial, usage.BillingUsage.Incomplete)
				assert.False(t, usage.BillingUsage.Estimated)
				var raw dto.Usage
				require.NoError(t, common.UnmarshalJsonStr(sample.raw, &raw))
				assert.Equal(t, raw.PromptTokens, usage.PromptTokens)
				assert.Equal(t, raw.CompletionTokens, usage.CompletionTokens)
			})
		}
	}
}

func TestChatUsageMissingIsEstimatedAndNeverSentAsInternalEvidence(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, stream := range []bool{false, true} {
		body := `{"id":"fixture","choices":[]}`
		if stream {
			body = "data: " + body + "\n\ndata: [DONE]\n\n"
		}
		c, recorder, response, info := newResponsesChatTestContext(t, body, stream)
		info.SetEstimatePromptTokens(99)
		handler := OpenaiHandler
		if stream {
			handler = OaiStreamHandler
		}
		usage, apiErr := handler(c, info, response)
		require.Nil(t, apiErr)
		require.NotNil(t, usage.BillingUsage)
		assert.True(t, usage.BillingUsage.Estimated)
		assert.Equal(t, 99, usage.PromptTokens)
		assert.NotContains(t, recorder.Body.String(), "billing_usage")
	}
}

func TestChatStreamPreservesTerminalUsageEvidence(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, sample := range []struct {
		name, model, chunks string
		partial, estimated  bool
		input               int
	}{
		{"intermediate is not final", "gpt-test", "data: {\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":4}}\n\ndata: {\"choices\":[]}\n\n", false, true, 99},
		{"audio penultimate zero", "gpt-audio", "data: {\"usage\":{\"prompt_tokens\":0,\"completion_tokens\":0}}\n\ndata: {\"choices\":[]}\n\n", false, false, 0},
		{"final partial supersedes audio", "gpt-audio", "data: {\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":4}}\n\ndata: {\"usage\":{\"prompt_tokens\":15}}\n\n", true, false, 15},
	} {
		t.Run(sample.name, func(t *testing.T) {
			c, _, response, info := newResponsesChatTestContext(t, sample.chunks+"data: [DONE]\n\n", true)
			info.UpstreamModelName = sample.model
			info.SetEstimatePromptTokens(99)
			usage, apiErr := OaiStreamHandler(c, info, response)
			require.Nil(t, apiErr)
			require.NotNil(t, usage.BillingUsage)
			assert.Equal(t, sample.partial, usage.BillingUsage.Incomplete)
			assert.Equal(t, sample.estimated, usage.BillingUsage.Estimated)
			assert.Equal(t, sample.input, usage.PromptTokens)
		})
	}
}

func TestChatUsageEvidenceFreezesIncludedSubcategories(t *testing.T) {
	usage := &dto.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110,
		PromptTokensDetails:    dto.InputTokenDetails{CachedTokens: 40},
		CompletionTokenDetails: dto.OutputTokenDetails{ReasoningTokens: 4}}
	require.True(t, captureChatUsageEvidence(usage, []byte(`{"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110}}`)))
	usage.PromptTokensDetails.CachedTokens = 99
	usage.CompletionTokenDetails.ReasoningTokens = 9
	assert.Equal(t, 40, usage.BillingUsage.OpenAIUsage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 4, usage.BillingUsage.OpenAIUsage.CompletionTokenDetails.ReasoningTokens)
	assert.Equal(t, 110, usage.BillingUsage.OpenAIUsage.TotalTokens)
}
