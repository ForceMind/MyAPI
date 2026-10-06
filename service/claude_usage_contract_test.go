package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudePerTokenSettlementRequiresCompleteRawUsage(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      []int
	}{
		{"empty", `{}`, nil},
		{"missing output", `{"input_tokens":10}`, nil},
		{"missing input", `{"output_tokens":5}`, nil},
		{"null output", `{"input_tokens":10,"output_tokens":null}`, nil},
		{"cache alias exceeds count bound", `{"input_tokens":10,"output_tokens":5,"claude_cache_creation_5_m_tokens":2147483648}`, nil},
		{"reported", `{"input_tokens":10,"output_tokens":5}`, []int{20}},
		{"reported zero", `{"input_tokens":0,"output_tokens":0}`, []int{0}},
		{"cache only is not zero usage", `{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":20,"cache_creation_input_tokens":30}`, []int{50}},
		{"cache included once", `{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":20,"cache_creation_input_tokens":30}`, []int{70}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 88, 1000)
			seedChannel(t, 88)
			var raw dto.ClaudeUsage
			require.NoError(t, common.Unmarshal([]byte(tc.raw), &raw))
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			ctx.Set(common.RequestIdKey, "claude-usage-contract")
			settler := &textQuotaTestSettler{preConsumed: 100}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 88}, UserId: 88, UserQuota: 1000, OriginModelName: "claude-contract", StartTime: time.Now(), Billing: settler, FinalPreConsumedQuota: 100, PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2, CacheRatio: 1, CacheCreationRatio: 1, CacheCreation5mRatio: 1, CacheCreation1hRatio: 1, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
			info.SetEstimatePromptTokens(100)
			PostTextConsumeQuota(ctx, info, &dto.Usage{BillingUsage: dto.NewClaudeMessagesBillingUsage(&raw)}, nil)
			assert.Equal(t, tc.want, settler.settled)
			if tc.want == nil {
				assert.Zero(t, settler.refund)
			} else {
				assert.Equal(t, 100-tc.want[0], settler.refund)
			}
			if tc.name == "reported zero" {
				logs, _, err := model.GetUserLogs(88, model.LogTypeConsume, 0, 0, "", "", 0, 10, "", "", "")
				require.NoError(t, err)
				require.NotEmpty(t, logs)
				assert.NotContains(t, logs[0].Content, "上游没有返回计费信息")
			}
		})
	}
}
