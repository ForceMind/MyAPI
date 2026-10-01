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

func TestResponsesStreamPreservesActualUsageAndMarksEstimates(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		final                           string
		terminal                        string
		prompt, output, cached, written int
		estimated                       bool
	}{
		{"actual zero is not missing", `{"input_tokens":0,"output_tokens":0,"total_tokens":0}`, "completed", 0, 0, 0, 0, false},
		{"actual cache categories", `{"input_tokens":100,"output_tokens":10,"total_tokens":110,"input_tokens_details":{"cached_tokens":40,"cache_write_tokens":20},"output_tokens_details":{"reasoning_tokens":4}}`, "completed", 100, 10, 40, 20, false},
		{"incomplete still reports consumed tokens", `{"input_tokens":100,"output_tokens":10,"total_tokens":110,"input_tokens_details":{"cached_tokens":40,"cache_write_tokens":20},"output_tokens_details":{"reasoning_tokens":4}}`, "incomplete", 100, 10, 40, 20, false},
		{"missing usage is estimated", `null`, "completed", 99, -1, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			oldTimeout := constant.StreamingTimeout
			constant.StreamingTimeout = 30
			t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Set(common.RequestIdKey, "responses-usage-fixture")
			info := &relaycommon.RelayInfo{OriginModelName: "gpt-4o", DisablePing: true,
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4o"}}
			info.SetEstimatePromptTokens(99)
			body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello world\"}\n\n" +
				"data: {\"type\":\"response." + tc.terminal + "\",\"response\":{\"status\":\"" + tc.terminal + "\",\"usage\":" + tc.final + "}}\n\n" +
				"data: [DONE]\n\n"
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)),
				Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
			usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, tc.prompt, usage.PromptTokens)
			if tc.output >= 0 {
				assert.Equal(t, tc.output, usage.CompletionTokens)
			} else {
				assert.Positive(t, usage.CompletionTokens)
			}
			assert.Equal(t, tc.cached, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, tc.written, usage.PromptTokensDetails.CacheWriteTokens)
			if tc.output == 10 {
				assert.Equal(t, 4, usage.CompletionTokenDetails.ReasoningTokens)
			}
			require.NotNil(t, usage.BillingUsage)
			assert.Equal(t, dto.BillingUsageSourceOAIResponses, usage.BillingUsage.Source)
			assert.Equal(t, tc.estimated, usage.BillingUsage.Estimated)
			require.NotNil(t, usage.BillingUsage.OpenAIUsage)
			assert.Equal(t, usage.TotalTokens, usage.BillingUsage.OpenAIUsage.TotalTokens)
		})
	}
}

func TestResponsesHandlersPreserveRawCacheUsageForSettlement(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler func(*gin.Context, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{"responses", func(c *gin.Context, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
			return OaiResponsesHandler(c, &relaycommon.RelayInfo{}, resp)
		}},
		{"compaction", OaiResponsesCompactionHandler},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			body := `{"usage":{"input_tokens":100,"output_tokens":10,"total_tokens":110,"input_tokens_details":{"cached_tokens":40,"cache_write_tokens":20,"text_tokens":100},"output_tokens_details":{"reasoning_tokens":4}}}`
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)),
				Header: http.Header{"Content-Type": []string{"application/json"}}}
			usage, apiErr := tc.handler(c, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage.BillingUsage)
			assert.False(t, usage.BillingUsage.Estimated)
			assert.Equal(t, 100, usage.PromptTokens)
			assert.Equal(t, 10, usage.CompletionTokens)
			assert.Equal(t, 40, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, 20, usage.PromptTokensDetails.CacheWriteTokens)
			assert.Equal(t, 100, usage.PromptTokensDetails.TextTokens)
			assert.Equal(t, 4, usage.CompletionTokenDetails.ReasoningTokens)
			raw := usage.BillingUsage.OpenAIUsage
			require.NotNil(t, raw.InputTokensDetails)
			assert.Equal(t, 100, raw.InputTokens)
			assert.Equal(t, 40, raw.InputTokensDetails.CachedTokens)
			require.NotNil(t, raw.OutputTokensDetails)
			assert.Equal(t, 4, raw.OutputTokensDetails.ReasoningTokens)
			usage.PromptTokensDetails.CachedTokens = 0
			assert.Equal(t, 40, raw.InputTokensDetails.CachedTokens)
		})
	}
}
