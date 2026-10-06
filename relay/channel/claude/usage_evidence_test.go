package claude

import (
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeRawUsageRequiresExplicitInputAndOutput(t *testing.T) {
	for _, tc := range []struct {
		name, raw  string
		incomplete bool
	}{
		{"empty", `{}`, true},
		{"missing input", `{"output_tokens":5}`, true},
		{"missing output", `{"input_tokens":10}`, true},
		{"null output", `{"input_tokens":10,"output_tokens":null}`, true},
		{"spoofed evidence", `{"input_tokens":10,"RawUsageObserved":false,"InputTokensReported":true,"OutputTokensReported":true,"billing_usage":{"incomplete":false}}`, true},
		{"negative cache", `{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":-1}`, true},
		{"contradictory cache split", `{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":3,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":2}}`, true},
		{"complete cache split", `{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":4,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":2}}`, false},
		{"reported", `{"input_tokens":10,"output_tokens":5}`, false},
		{"explicit zero", `{"input_tokens":0,"output_tokens":0}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw dto.ClaudeUsage
			require.NoError(t, common.Unmarshal([]byte(tc.raw), &raw))
			evidence := dto.NewClaudeMessagesBillingUsage(&raw)
			require.NotNil(t, evidence)
			assert.Equal(t, tc.incomplete, evidence.Incomplete)
			assert.False(t, evidence.Estimated)
		})
	}
}

func TestClaudeStreamBillingRequiresFinalReportedUsage(t *testing.T) {
	for _, tc := range []struct {
		name, start, delta string
		known              bool
		output             int
	}{
		{"start alone is not final", `{"input_tokens":10,"output_tokens":1}`, "", false, 0},
		{"missing final output", `{"input_tokens":10,"output_tokens":1}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{}}`, false, 0},
		{"missing initial input", `{"output_tokens":1}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`, false, 0},
		{"nonterminal delta", `{"input_tokens":10,"output_tokens":1}`, `{"type":"message_delta","usage":{"output_tokens":5}}`, false, 0},
		{"regressing cumulative output", `{"input_tokens":10,"output_tokens":8}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`, false, 0},
		{"reported final", `{"input_tokens":10,"output_tokens":1}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`, true, 5},
		{"reported zero", `{"input_tokens":0,"output_tokens":0}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}`, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-fixture"}}
			info.SetEstimatePromptTokens(100)
			state := &ClaudeResponseInfo{Usage: &dto.Usage{}}
			require.Nil(t, HandleStreamResponseData(ctx, info, state, `{"type":"message_start","message":{"id":"synthetic","model":"claude-fixture","usage":`+tc.start+`}}`))
			if tc.delta != "" {
				require.Nil(t, HandleStreamResponseData(ctx, info, state, tc.delta))
			}
			HandleStreamFinalResponse(ctx, info, state)
			evidence := state.Usage.BillingUsage
			require.NotNil(t, evidence)
			if tc.known {
				assert.False(t, evidence.Incomplete)
				assert.False(t, evidence.Estimated)
				require.NotNil(t, evidence.ClaudeUsage)
				assert.Equal(t, tc.output, evidence.ClaudeUsage.OutputTokens)
				if tc.name == "reported zero" {
					assert.Zero(t, evidence.ClaudeUsage.InputTokens)
				} else {
					assert.Equal(t, 10, evidence.ClaudeUsage.InputTokens)
				}
			} else {
				assert.True(t, evidence.Incomplete || evidence.Estimated, "partial or inferred usage must remain unresolved")
			}
		})
	}
}

func TestClaudeCumulativeStreamCountsCacheOnce(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-fixture"}}
	state := &ClaudeResponseInfo{Usage: &dto.Usage{}}
	for _, event := range []string{
		`{"type":"message_start","message":{"id":"synthetic","model":"claude-fixture","usage":{"input_tokens":10,"output_tokens":1,"cache_read_input_tokens":20,"cache_creation_input_tokens":30,"cache_creation":{"ephemeral_5m_input_tokens":30,"ephemeral_1h_input_tokens":0}}}}`,
		`{"type":"message_delta","usage":{"output_tokens":3}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
	} {
		require.Nil(t, HandleStreamResponseData(ctx, info, state, event))
	}
	HandleStreamFinalResponse(ctx, info, state)
	require.NotNil(t, state.Usage.BillingUsage)
	assert.False(t, state.Usage.BillingUsage.Incomplete)
	assert.False(t, state.Usage.BillingUsage.Estimated)
	raw := state.Usage.BillingUsage.ClaudeUsage
	require.NotNil(t, raw)
	assert.Equal(t, 10, raw.InputTokens)
	assert.Equal(t, 20, raw.CacheReadInputTokens)
	assert.Equal(t, 30, raw.CacheCreationInputTokens)
	assert.Equal(t, 5, raw.OutputTokens)
	normalized := buildOpenAIStyleUsageFromClaudeUsage(state.Usage)
	assert.Equal(t, 60, normalized.PromptTokens)
	assert.Equal(t, 5, normalized.CompletionTokens)
	assert.Equal(t, 65, normalized.TotalTokens)
	// An incomplete later usage event must not leave a prior clean snapshot.
	require.Nil(t, HandleStreamResponseData(ctx, info, state, `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`))
	HandleStreamFinalResponse(ctx, info, state)
	assert.True(t, state.Usage.BillingUsage.Incomplete)

}
