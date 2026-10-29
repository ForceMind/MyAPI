package openai

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
)

// The Responses->Chat handlers must keep the same raw cache-field presence the
// native Responses handlers keep: a missing or null cached_tokens is unknown,
// an explicit 0 is a reported zero, and a positive value is preserved.
func TestResponsesToChatHandlersPreserveCacheFieldPresence(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	zero, seven := 0, 7
	handlers := []struct {
		name  string
		build func(usage string) (string, bool)
		run   func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{"json", func(usage string) (string, bool) {
			return `{"id":"resp_1","object":"response","model":"gpt-test","service_tier":"default","status":"completed","output":[],"usage":` + usage + `}`, false
		}, OaiResponsesToChatHandler},
		{"buffered_sse", func(usage string) (string, bool) {
			return `data: {"type":"response.completed","response":{"model":"gpt-test","service_tier":"default","status":"completed","usage":` + usage + `}}` + "\n\ndata: [DONE]\n\n", false
		}, OaiResponsesToChatBufferedStreamHandler},
		{"stream", func(usage string) (string, bool) {
			return `data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000,"usage":null}}` + "\n\n" +
				`data: {"type":"response.completed","response":{"model":"gpt-test","service_tier":"default","status":"completed","usage":` + usage + `}}` + "\n\ndata: [DONE]\n\n", true
		}, OaiResponsesToChatStreamHandler},
	}
	states := []struct {
		name        string
		details     string
		hasEvidence bool
		wantRead    *int
		wantCompat  int
	}{
		{"no details object", ``, false, nil, 0},
		{"cached_tokens missing", `,"input_tokens_details":{}`, true, nil, 0},
		{"cached_tokens null", `,"input_tokens_details":{"cached_tokens":null}`, true, nil, 0},
		{"cached_tokens zero", `,"input_tokens_details":{"cached_tokens":0}`, true, &zero, 0},
		{"cached_tokens positive", `,"input_tokens_details":{"cached_tokens":7}`, true, &seven, 7},
	}

	for _, handler := range handlers {
		for _, state := range states {
			t.Run(handler.name+"/"+state.name, func(t *testing.T) {
				body, stream := handler.build(`{"input_tokens":20,"output_tokens":3,"total_tokens":23` + state.details + `}`)
				c, recorder, resp, info := newResponsesChatTestContext(t, body, stream)

				usage, apiErr := handler.run(c, info, resp)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				require.NotNil(t, usage.BillingUsage)

				assert.Equal(t, 20, usage.PromptTokens)
				assert.Equal(t, 3, usage.CompletionTokens)
				assert.Equal(t, state.wantCompat, usage.PromptTokensDetails.CachedTokens)
				assert.False(t, usage.BillingUsage.Incomplete)
				assert.False(t, usage.BillingUsage.Estimated)
				assert.NotContains(t, recorder.Body.String(), "responses_text_evidence", "internal evidence must not reach the client")

				evidence := usage.BillingUsage.ResponsesTextEvidence
				if !state.hasEvidence {
					assert.Nil(t, evidence)
					return
				}
				require.NotNil(t, evidence)
				assert.Equal(t, "gpt-test", evidence.Model)
				assert.Equal(t, "default", evidence.ServiceTier)
				assert.Nil(t, evidence.CacheWrite)
				if state.wantRead == nil {
					assert.Nil(t, evidence.CacheRead)
					return
				}
				require.NotNil(t, evidence.CacheRead)
				assert.Equal(t, *state.wantRead, *evidence.CacheRead)
			})
		}
	}
}

// Evidence must come from the same terminal event whose usage the converter
// keeps: a later terminal usage replaces an earlier one, including a legitimate
// reported zero, and a later terminal event without usage replaces nothing.
func TestResponsesToChatStreamCacheEvidenceFollowsTerminalUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	zero, seven := 0, 7
	usageWithCache := func(cached string) string {
		return `{"input_tokens":20,"output_tokens":3,"total_tokens":23,"input_tokens_details":{"cached_tokens":` + cached + `}}`
	}
	for _, sample := range []struct {
		name       string
		second     string
		wantRead   *int
		wantCompat int
	}{
		{"later terminal reports zero", usageWithCache("0"), &zero, 0},
		{"later terminal without usage", `null`, &seven, 7},
	} {
		t.Run(sample.name, func(t *testing.T) {
			body := strings.Join([]string{
				`data: {"type":"response.completed","response":{"model":"gpt-test","status":"completed","usage":` + usageWithCache("7") + `}}`,
				`data: {"type":"response.done","response":{"model":"gpt-test","status":"completed","usage":` + sample.second + `}}`,
				`data: [DONE]`,
				``,
			}, "\n\n")
			c, _, resp, info := newResponsesChatTestContext(t, body, true)

			usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.NotNil(t, usage.BillingUsage)
			require.NotNil(t, usage.BillingUsage.ResponsesTextEvidence)
			require.NotNil(t, usage.BillingUsage.ResponsesTextEvidence.CacheRead)
			assert.Equal(t, sample.wantCompat, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, *sample.wantRead, *usage.BillingUsage.ResponsesTextEvidence.CacheRead)
		})
	}
}

// Usage that falls back to a local estimate is not upstream evidence, so no
// native cache-field evidence may be attached to it.
func TestResponsesToChatEstimatedUsageCarriesNoCacheEvidence(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := `{"id":"resp_1","object":"response","model":"gpt-test","status":"completed","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0,"input_tokens_details":{"cached_tokens":0}}}`
	c, _, resp, info := newResponsesChatTestContext(t, body, false)

	usage, apiErr := OaiResponsesToChatHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	if usage.BillingUsage != nil {
		assert.Nil(t, usage.BillingUsage.ResponsesTextEvidence)
	}
}
