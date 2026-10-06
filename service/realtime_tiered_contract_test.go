package service

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/setting/billing_setting"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealtimeTieredSettlementUsesReportedTokenDimensions(t *testing.T) {
	truncate(t)
	seedUser(t, 91, 10000)
	seedChannel(t, 91)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	settler := &textQuotaTestSettler{preConsumed: 100}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 91}, UserId: 91,
		UserQuota: 1 << 30, OriginModelName: "realtime-tiered-fixture", StartTime: time.Now(), Billing: settler,
		PriceData: hosttypes.PriceData{GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{BillingMode: billing_setting.BillingModeTieredExpr,
			ExprString: "p + cr * 0.1 + ai * 10 + c * 2 + ao * 20", QuotaPerUnit: 1_000_000, GroupRatio: 1},
	}
	info.TieredBillingSnapshot.ExprHash = billingexpr.ExprHashString(info.TieredBillingSnapshot.ExprString)
	require.NoError(t, info.PriceData.CaptureQuotaUnit(1_000_000))
	usage := &dto.RealtimeUsage{InputTokens: 132, OutputTokens: 121, TotalTokens: 253,
		InputTokenDetails: dto.InputTokenDetails{TextTokens: 119, AudioTokens: 13, CachedTokens: 64,
			CachedTokensDetails: dto.NewCachedTokenDetails(64, 0, 0)},
		OutputTokenDetails: dto.OutputTokenDetails{TextTokens: 30, AudioTokens: 91}}
	PostWssConsumeQuota(ctx, info, info.OriginModelName, usage, "")
	// Synthetic rates: (132-64-13) + 64*.1 + 13*10 + (121-91)*2 + 91*20.
	assert.Equal(t, []int{2071}, settler.settled)
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, 2071, log.Quota)
}

func TestRealtimeTieredSettlementKeepsPerResponseContextThreshold(t *testing.T) {
	truncate(t)
	seedUser(t, 92, 10000)
	seedChannel(t, 92)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	settler := &textQuotaTestSettler{preConsumed: 100}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 92}, UserId: 92,
		UserQuota: 1 << 30, OriginModelName: "realtime-tiered-fixture", StartTime: time.Now(), Billing: settler,
		PriceData: hosttypes.PriceData{GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{BillingMode: billing_setting.BillingModeTieredExpr,
			ExprString: `len <= 10 ? tier("short", p) : tier("long", p * 10)`, QuotaPerUnit: 1_000_000, GroupRatio: 1},
	}
	info.TieredBillingSnapshot.ExprHash = billingexpr.ExprHashString(info.TieredBillingSnapshot.ExprString)
	require.NoError(t, info.PriceData.CaptureQuotaUnit(1_000_000))
	segment := &dto.RealtimeUsage{InputTokens: 8, TotalTokens: 8, InputTokenDetails: dto.InputTokenDetails{TextTokens: 8}}
	require.NoError(t, RecordRealtimeTieredResponse(info, segment))
	require.NoError(t, PreWssConsumeQuota(ctx, info, segment))
	require.NoError(t, RecordRealtimeTieredResponse(info, segment))
	require.NoError(t, PreWssConsumeQuota(ctx, info, segment))
	PostWssConsumeQuota(ctx, info, info.OriginModelName, &dto.RealtimeUsage{InputTokens: 16, TotalTokens: 16,
		InputTokenDetails: dto.InputTokenDetails{TextTokens: 16}}, "")
	assert.Equal(t, []int{16}, settler.settled, "two short responses must not become one long-context bill")
	log := getLastLog(t)
	require.NotNil(t, log)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, "per_response", other["realtime_pricing_scope"])
	assert.Equal(t, 2.0, other["realtime_priced_responses"])
	assert.NotContains(t, other, "matched_tier", "connection totals have no single matched context tier")
	assert.NotContains(t, other, "expr_b64", "single-context UI must not reconstruct a false aggregate bill")
}

func TestRealtimeTieredQuoteKeepsKnownPriceOnReserveFailure(t *testing.T) {
	expression := `len <= 10 ? p : p * 10`
	failure := &realtimeReserveFailure{textQuotaTestSettler: textQuotaTestSettler{preConsumed: 100}, err: fmt.Errorf("synthetic reservation failure")}
	info := &relaycommon.RelayInfo{Billing: failure, PriceData: hosttypes.PriceData{GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expression,
			ExprHash: billingexpr.ExprHashString(expression), QuotaPerUnit: 1_000_000, GroupRatio: 1}}
	require.NoError(t, info.PriceData.CaptureQuotaUnit(1_000_000))
	usage := &dto.RealtimeUsage{InputTokens: 8, TotalTokens: 8, InputTokenDetails: dto.InputTokenDetails{TextTokens: 8}}
	require.NoError(t, RecordRealtimeTieredResponse(info, usage))
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.Error(t, PreWssConsumeQuota(ctx, info, usage))
	assert.Equal(t, 8, info.RealtimeTieredPricing.Quota)
	assert.Equal(t, 1, info.RealtimeTieredPricing.Responses)
	assert.Zero(t, info.RealtimeQuotedQuota, "reservation success and known spent quote are different facts")
}

func TestRealtimeTieredQuoteRejectsChangedSnapshotAndOverflow(t *testing.T) {
	expression := "p"
	info := &relaycommon.RelayInfo{TieredBillingSnapshot: &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expression,
		ExprHash: billingexpr.ExprHashString(expression), QuotaPerUnit: 1_000_000, GroupRatio: 1}}
	usage := &dto.RealtimeUsage{InputTokens: 8, TotalTokens: 8}
	require.NoError(t, RecordRealtimeTieredResponse(info, usage))
	info.TieredBillingSnapshot.QuotaPerUnit = 2_000_000
	require.Error(t, RecordRealtimeTieredResponse(info, usage))
	assert.Equal(t, 8, info.RealtimeTieredPricing.Quota)
	assert.True(t, info.RealtimeTieredPricing.Incomplete)
	info.TieredBillingSnapshot.QuotaPerUnit = 1_000_000
	info.RealtimeTieredPricing.Incomplete = false
	info.RealtimeTieredPricing.Quota = common.MaxQuota
	require.Error(t, RecordRealtimeTieredResponse(info, usage))
	assert.Equal(t, common.MaxQuota, info.RealtimeTieredPricing.Quota)
	assert.Equal(t, 1, info.RealtimeTieredPricing.Responses, "overflow must not partially commit token or response totals")
	require.NotNil(t, info.QuotaClamp)
	assert.True(t, info.RealtimeTieredPricing.Incomplete)
}
