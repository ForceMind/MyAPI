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
)

// These contracts concern per-token pricing, not fixed per-call or independently
// known tool fees. Unknown usage must not become a confirmed actual charge.
func TestPerTokenSettlementRequiresReportedUsage(t *testing.T) {
	estimated := dto.NewOpenAIResponsesBillingUsage(&dto.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110})
	estimated.Estimated = true
	for _, tc := range []struct {
		name  string
		usage *dto.Usage
		want  []int
	}{
		{"missing usage stays unresolved", nil, nil},
		{"estimated usage stays unresolved", &dto.Usage{BillingUsage: estimated}, nil},
		{"reported usage settles exact categories", &dto.Usage{BillingUsage: dto.NewOpenAIResponsesBillingUsage(&dto.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110})}, []int{120}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 88, 1000)
			seedChannel(t, 88)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			ctx.Set(common.RequestIdKey, "usage-settlement-fixture")
			settler := &textQuotaTestSettler{preConsumed: 100}
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 88},
				UserId:      88, UserQuota: 1000, OriginModelName: "usage-settlement-fixture", StartTime: time.Now(),
				Billing: settler, FinalPreConsumedQuota: 100,
				PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2,
					GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
			}
			info.SetEstimatePromptTokens(100)
			PostTextConsumeQuota(ctx, info, tc.usage, nil)
			assert.Equal(t, tc.want, settler.settled)
			assert.Zero(t, settler.refund)
		})
	}
}
