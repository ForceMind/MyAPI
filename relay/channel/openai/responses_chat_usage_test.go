package openai

import (
	"net/http"
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
	"github.com/tidwall/gjson"
)

func TestResponsesChatUsageCompatibilityAcrossHandlers(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, handler := range []struct {
		name string
		run  func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{"json", OaiResponsesToChatHandler},
		{"buffered", OaiResponsesToChatBufferedStreamHandler},
		{"stream", OaiResponsesToChatStreamHandler},
	} {
		for _, sample := range []struct {
			name, details, cached string
			wantCache             int
			native                bool
		}{
			{"missing", ``, "", 0, false},
			{"null", `,"input_tokens_details":{"cached_tokens":null}`, "null", 0, true},
			{"zero", `,"input_tokens_details":{"cached_tokens":0}`, "0", 0, true},
			{"positive", `,"input_tokens_details":{"cached_tokens":7}`, "7", 7, true},
			{"alias", `,"prompt_tokens_details":{"cached_tokens":7}`, "7", 7, false},
			{"canonical_zero_wins", `,"input_tokens_details":{"cached_tokens":0},"prompt_tokens_details":{"cached_tokens":7}`, "0", 0, true},
			{"canonical_missing_wins", `,"input_tokens_details":{},"prompt_tokens_details":{"cached_tokens":7}`, "", 0, true},
			{"canonical_null_wins", `,"input_tokens_details":{"cached_tokens":null},"prompt_tokens_details":{"cached_tokens":7}`, "null", 0, true},
		} {
			t.Run(handler.name+"/"+sample.name, func(t *testing.T) {
				rawUsage := `{"input_tokens":20,"output_tokens":3,"total_tokens":23` + sample.details + `}`
				body := `{"id":"synthetic","model":"gpt-test","service_tier":"default","status":"completed","output":[],"usage":` + rawUsage + `}`
				if handler.name != "json" {
					body = "data: " + `{"type":"response.completed","response":` + body + `}` + "\n\ndata: [DONE]\n\n"
				}
				c, recorder, resp, info := newResponsesChatTestContext(t, body, handler.name == "stream")
				usage, apiErr := handler.run(c, info, resp)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				assert.Equal(t, 20, usage.PromptTokens)
				assert.Equal(t, 3, usage.CompletionTokens)
				assert.Equal(t, sample.wantCache, usage.PromptTokensDetails.CachedTokens)
				require.NotNil(t, usage.BillingUsage)
				assert.False(t, usage.BillingUsage.Incomplete)
				assert.False(t, usage.BillingUsage.Estimated)
				assert.Equal(t, sample.native, usage.BillingUsage.ResponsesTextEvidence != nil)
				client := recorder.Body.String()
				assert.NotContains(t, client, `"billing_usage"`)
				if handler.name == "stream" {
					for _, line := range strings.Split(client, "\n") {
						if strings.HasPrefix(line, "data: ") && gjson.Get(strings.TrimPrefix(line, "data: "), "usage").IsObject() {
							assert.Equal(t, sample.cached, gjson.Get(strings.TrimPrefix(line, "data: "), "usage.prompt_tokens_details.cached_tokens").Raw)
						}
					}
				} else {
					assert.Equal(t, sample.cached, gjson.Get(client, "usage.prompt_tokens_details.cached_tokens").Raw)
				}
			})
		}
	}
}

func TestResponsesChatUsageRootAliasAndUntrustedBilling(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, buffered := range []bool{false, true} {
		t.Run(map[bool]string{false: "stream", true: "buffered"}[buffered], func(t *testing.T) {
			body := `data: {"type":"response.completed","response":{"id":"synthetic","model":"gpt-test","status":"completed","output":[]},"usage":{"input_tokens":20,"output_tokens":3,"total_tokens":23,"input_tokens_details":{"cached_tokens":7},"billing_usage":{"source":"claude_messages","incomplete":false,"openai_usage":{"input_tokens":999}}}}` + "\n\ndata: [DONE]\n\n"
			c, recorder, resp, info := newResponsesChatTestContext(t, body, !buffered)
			handler := OaiResponsesToChatStreamHandler
			if buffered {
				handler = OaiResponsesToChatBufferedStreamHandler
			}
			usage, apiErr := handler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 20, usage.PromptTokens)
			assert.Equal(t, 3, usage.CompletionTokens)
			assert.Equal(t, 7, usage.PromptTokensDetails.CachedTokens)
			require.NotNil(t, usage.BillingUsage)
			assert.Equal(t, dto.BillingUsageSourceOAIResponses, usage.BillingUsage.Source)
			assert.Equal(t, 20, usage.BillingUsage.OpenAIUsage.InputTokens)
			assert.Nil(t, usage.BillingUsage.ResponsesTextEvidence, "compatible root usage must not become native fee proof")
			assert.False(t, usage.BillingUsage.Incomplete)
			assert.NotContains(t, recorder.Body.String(), `"billing_usage"`)
			assert.Contains(t, recorder.Body.String(), `"cached_tokens":7`)
		})
	}
}

func TestResponsesChatUsageInvalidCountsCannotClaimCompleteEvidence(t *testing.T) {
	for _, counts := range []string{
		`"input_tokens":20,"output_tokens":3,"total_tokens":23,"input_tokens_details":{"cached_tokens":21}`,
		`"input_tokens":20,"output_tokens":3,"total_tokens":23,"prompt_tokens_details":{"cached_tokens":-1}`,
		`"input_tokens":20,"output_tokens":3,"total_tokens":24,"input_tokens_details":{"cached_tokens":7}`,
		`"input_tokens":20,"total_tokens":20,"input_tokens_details":{"cached_tokens":7}`,
	} {
		t.Run(counts, func(t *testing.T) {
			body := `{"model":"gpt-test","status":"completed","output":[],"usage":{` + counts + `}}`
			c, _, resp, info := newResponsesChatTestContext(t, body, false)
			usage, apiErr := OaiResponsesToChatHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.NotNil(t, usage.BillingUsage)
			assert.True(t, usage.BillingUsage.Incomplete)
		})
	}
}

func TestResponsesChatUsageLatestTerminalKeepsUnknownAndExplicitZero(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, sample := range []struct{ latest, want string }{
		{`{"input_tokens":20,"output_tokens":3,"total_tokens":23,"input_tokens_details":{"cached_tokens":0}}`, "0"},
		{`{"input_tokens":20,"output_tokens":3,"total_tokens":23,"input_tokens_details":{"cached_tokens":null}}`, "null"},
		{`{"input_tokens":20,"output_tokens":3,"total_tokens":23}`, ""},
		{`null`, "7"},
	} {
		t.Run(sample.latest, func(t *testing.T) {
			body := `data: {"type":"response.completed","response":{"usage":{"input_tokens":20,"output_tokens":3,"total_tokens":23,"input_tokens_details":{"cached_tokens":7}}}}` + "\n\n" +
				`data: {"type":"response.done","response":{"usage":` + sample.latest + `}}` + "\n\ndata: [DONE]\n\n"
			c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
			_, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			frames := 0
			for _, line := range strings.Split(recorder.Body.String(), "\n") {
				value := strings.TrimPrefix(line, "data: ")
				if gjson.Get(value, "usage").IsObject() {
					frames++
					assert.Equal(t, sample.want, gjson.Get(value, "usage.prompt_tokens_details.cached_tokens").Raw)
				}
			}
			assert.Equal(t, 1, frames)
		})
	}
}

func TestResponsesChatUsageCanonicalNullDoesNotUseRootAlias(t *testing.T) {
	body := []byte(`{"type":"response.completed","response":{"usage":null},"usage":{"input_tokens":20,"output_tokens":3,"input_tokens_details":{"cached_tokens":7}}}`)
	normalized, observed, err := normalizeResponsesChatUsage(body, true)
	require.NoError(t, err)
	assert.False(t, observed.present)
	assert.Equal(t, "null", gjson.GetBytes(normalized, "response.usage").Raw)
	var event dto.ResponsesStreamResponse
	require.NoError(t, common.Unmarshal(normalized, &event))
	require.NotNil(t, event.Response)
	assert.Nil(t, event.Response.Usage)
}
