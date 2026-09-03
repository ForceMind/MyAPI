package openai

import (
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIRequestOpenRouterAnthropicThinking(t *testing.T) {
	tests := []struct {
		name, upstream, thinking string
		wantErr                  string
		cleared                  bool
		wantReasoning            bool
		maxTokens                int
	}{
		{"enabled", "anthropic/claude", `{"type":"enabled","budget_tokens":8}`, "", true, true, 8},
		{"missing budget", "anthropic/claude", `{"type":"enabled"}`, "BudgetTokens is nil when thinking is enabled", false, false, 0},
		{"malformed", "anthropic/claude", `{`, "error Unmarshal thinking", false, false, 0},
		{"nonenabled", "anthropic/claude", `{"type":"disabled"}`, "", true, false, 0},
		{"nonAnthropic", "openai/gpt", `{"type":"enabled","budget_tokens":8}`, "", false, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			req := &dto.GeneralOpenAIRequest{THINKING: []byte(tt.thinking)}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter, UpstreamModelName: tt.upstream}}
			_, err := (&Adaptor{}).ConvertOpenAIRequest(c, info, req)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Equal(t, tt.cleared, req.THINKING == nil)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.cleared, req.THINKING == nil)
			if tt.wantReasoning {
				var got struct {
					Enabled   bool `json:"enabled"`
					MaxTokens int  `json:"max_tokens"`
				}
				require.NoError(t, common.Unmarshal(req.Reasoning, &got))
				require.True(t, got.Enabled)
				require.Equal(t, tt.maxTokens, got.MaxTokens)
			} else {
				require.Empty(t, req.Reasoning)
			}
		})
	}
}
