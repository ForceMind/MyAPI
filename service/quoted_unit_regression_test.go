package service

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextSettlementDoesNotRepriceCapturedQuotaUnit(t *testing.T) {
	oldUnit := common.QuotaPerUnit
	t.Cleanup(func() { common.QuotaPerUnit = oldUnit })
	common.QuotaPerUnit = 100
	info := &relaycommon.RelayInfo{OriginModelName: "quoted-unit-fixture", StartTime: time.Now(), PriceData: hosttypes.PriceData{UsePrice: true, ModelPrice: 2, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
	require.NoError(t, info.PriceData.CaptureQuotaUnit(common.QuotaPerUnit))
	common.QuotaPerUnit = 400
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	summary := calculateTextQuotaSummary(c, info, &dto.Usage{PromptTokens: 10, CompletionTokens: 10})
	assert.Equal(t, 200, summary.Quota)
}

func TestToolSurchargeUsesCapturedUnitWithTokenPrices(t *testing.T) {
	oldUnit := common.QuotaPerUnit
	t.Cleanup(func() { common.QuotaPerUnit = oldUnit; operation_setting.DeleteToolPriceForTest("quoted_tool") })
	operation_setting.SetToolPriceForTest("quoted_tool", 5)
	common.QuotaPerUnit = 1000
	info := &relaycommon.RelayInfo{OriginModelName: "quoted-tool-fixture", StartTime: time.Now(), PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
	require.NoError(t, info.PriceData.CaptureQuotaUnit(common.QuotaPerUnit))
	info.CountBillableToolCall(dto.BuildInCallFunctionCall, "quoted_tool")
	common.QuotaPerUnit = 4000
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	summary := calculateTextQuotaSummary(c, info, &dto.Usage{PromptTokens: 10, CompletionTokens: 10})
	assert.Equal(t, 25, summary.Quota)
}

func TestOpenRouterCacheEstimateUsesCapturedQuotaUnit(t *testing.T) {
	oldUnit := common.QuotaPerUnit
	t.Cleanup(func() { common.QuotaPerUnit = oldUnit })
	common.QuotaPerUnit = 100
	price := hosttypes.PriceData{ModelRatio: 1, CacheCreationRatio: 1.25}
	require.NoError(t, price.CaptureQuotaUnit(common.QuotaPerUnit))
	common.QuotaPerUnit = 400
	got, err := CalcOpenRouterCacheCreateTokens(dto.Usage{Cost: 0.25}, price)
	require.NoError(t, err)
	assert.Equal(t, 100, got)
}

func TestAudioAndRealtimeFixedPriceSettlementKeepsQuote(t *testing.T) {
	oldUnit := common.QuotaPerUnit
	t.Cleanup(func() { common.QuotaPerUnit = oldUnit })
	for _, mode := range []string{"audio", "realtime"} {
		t.Run(mode, func(t *testing.T) {
			truncate(t)
			seedUser(t, 89, 1000)
			seedChannel(t, 89)
			common.QuotaPerUnit = 100
			settler := &textQuotaTestSettler{preConsumed: 200}
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 89},
				UserId:      89, UserQuota: 1000, StartTime: time.Now(), OriginModelName: "audio-unit-fixture",
				Billing: settler, FinalPreConsumedQuota: 200,
				PriceData: hosttypes.PriceData{UsePrice: true, ModelPrice: 2, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
			}
			require.NoError(t, info.PriceData.CaptureQuotaUnit(common.QuotaPerUnit))
			common.QuotaPerUnit = 400
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/audio", nil)
			if mode == "audio" {
				PostAudioConsumeQuota(ctx, info, &dto.Usage{PromptTokens: 10, CompletionTokens: 10, TotalTokens: 20}, "")
			} else {
				PostWssConsumeQuota(ctx, info, info.OriginModelName, &dto.RealtimeUsage{InputTokens: 10, OutputTokens: 10, TotalTokens: 20}, "")
			}
			assert.Equal(t, []int{200}, settler.settled)
			assert.Zero(t, settler.refund)
			used, requests := getUserUsageAccounting(t, 89)
			assert.Equal(t, 200, used)
			assert.Equal(t, 1, requests)
			log := getLastLog(t)
			require.NotNil(t, log)
			assert.Equal(t, 200, log.Quota)
		})
	}
}

func TestToolSettlementKeepsCapturedPricesIncludingZeroAndUnknown(t *testing.T) {
	for _, quoted := range []float64{5, 0} {
		t.Run(fmt.Sprintf("quoted price %g", quoted), func(t *testing.T) {
			operation_setting.SetToolPriceForTest("snapshot_tool", quoted)
			t.Cleanup(func() {
				operation_setting.DeleteToolPriceForTest("snapshot_tool")
				operation_setting.DeleteToolPriceForTest("added_after_quote")
			})
			info := &relaycommon.RelayInfo{OriginModelName: "tool-snapshot-fixture", StartTime: time.Now(),
				PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
			require.NoError(t, info.PriceData.CaptureQuotaUnit(1000))
			require.NoError(t, info.PriceData.CaptureToolPrices(operation_setting.CaptureToolPricesForModel(info.OriginModelName)))
			operation_setting.SetToolPriceForTest("snapshot_tool", 20)
			operation_setting.SetToolPriceForTest("added_after_quote", 30)
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, "snapshot_tool")
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, "added_after_quote")
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			summary := calculateTextQuotaSummary(ctx, info, &dto.Usage{PromptTokens: 10, CompletionTokens: 10})
			assert.Equal(t, 20+int(quoted), summary.Quota)
			if quoted == 0 {
				assert.Empty(t, summary.ToolSurchargeItems)
			} else {
				require.Len(t, summary.ToolSurchargeItems, 1)
				assert.Equal(t, quoted, summary.ToolSurchargeItems[0].Price)
			}
		})
	}
}

func TestAudioAndRealtimeTokenSettlementUsesQuotedRatios(t *testing.T) {
	oldCompletion := ratio_setting.CompletionRatio2JSONString()
	oldAudio := ratio_setting.AudioRatio2JSONString()
	oldAudioCompletion := ratio_setting.AudioCompletionRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
		require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(oldAudio))
		require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(oldAudioCompletion))
	})
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"audio-ratio-fixture":9}`))
	require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(`{"audio-ratio-fixture":8}`))
	require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(`{"audio-ratio-fixture":7}`))
	for _, mode := range []string{"audio", "realtime"} {
		t.Run(mode, func(t *testing.T) {
			truncate(t)
			seedUser(t, 90, 1000)
			seedChannel(t, 90)
			settler := &textQuotaTestSettler{preConsumed: 100}
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 90}, UserId: 90, UserQuota: 1000,
				StartTime: time.Now(), OriginModelName: "audio-ratio-fixture", Billing: settler,
				FinalPreConsumedQuota: 100,
				PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2, AudioRatio: 3, AudioCompletionRatio: 4,
					GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
			}
			require.NoError(t, info.PriceData.CaptureQuotaUnit(100))
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/audio", nil)
			input := dto.InputTokenDetails{TextTokens: 10, AudioTokens: 2}
			output := dto.OutputTokenDetails{TextTokens: 5, AudioTokens: 1}
			if mode == "audio" {
				PostAudioConsumeQuota(ctx, info, &dto.Usage{PromptTokens: 12, CompletionTokens: 6, TotalTokens: 18,
					PromptTokensDetails: input, CompletionTokenDetails: output}, "")
			} else {
				PostWssConsumeQuota(ctx, info, info.OriginModelName, &dto.RealtimeUsage{InputTokens: 12, OutputTokens: 6,
					TotalTokens: 18, InputTokenDetails: input, OutputTokenDetails: output}, "")
			}
			assert.Equal(t, []int{38}, settler.settled)
			assert.Equal(t, 62, settler.refund)
			log := getLastLog(t)
			require.NotNil(t, log)
			assert.Equal(t, 38, log.Quota)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
			assert.Equal(t, 2.0, other["completion_ratio"])
			assert.Equal(t, 3.0, other["audio_ratio"])
			assert.Equal(t, 4.0, other["audio_completion_ratio"])
		})
	}
}
