package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiPerTokenSettlementRequiresCompleteRawUsage(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      []int
	}{
		{"missing input", `{"candidatesTokenCount":5,"totalTokenCount":5}`, nil},
		{"missing output", `{"promptTokenCount":10,"totalTokenCount":15}`, nil},
		{"missing total", `{"promptTokenCount":10,"candidatesTokenCount":5}`, nil},
		{"bad total", `{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":1}`, nil},
		{"cache exceeds input", `{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15,"cachedContentTokenCount":11}`, nil},
		{"reported", `{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}`, []int{20}},
		{"reported zero", `{"promptTokenCount":0,"candidatesTokenCount":0,"totalTokenCount":0}`, []int{0}},
		{"cache included once", `{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15,"cachedContentTokenCount":4}`, []int{20}},
		{"thinking included once", `{"promptTokenCount":10,"candidatesTokenCount":5,"thoughtsTokenCount":3,"totalTokenCount":18}`, []int{26}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 88, 1000)
			seedChannel(t, 88)
			var raw dto.GeminiUsageMetadata
			require.NoError(t, common.Unmarshal([]byte(tc.raw), &raw))
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			ctx.Set(common.RequestIdKey, "gemini-usage-contract")
			settler := &textQuotaTestSettler{preConsumed: 100}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 88}, UserId: 88, UserQuota: 1000, OriginModelName: "gemini-contract", StartTime: time.Now(), Billing: settler, FinalPreConsumedQuota: 100, PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2, CacheRatio: 1, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
			PostTextConsumeQuota(ctx, info, &dto.Usage{BillingUsage: dto.NewGeminiChatBillingUsage(&raw)}, nil)
			assert.Equal(t, tc.want, settler.settled)
			if tc.want == nil {
				assert.Zero(t, settler.refund)
			} else {
				assert.Equal(t, 100-tc.want[0], settler.refund)
			}
		})
	}
}
