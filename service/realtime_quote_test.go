package service

import (
	"errors"
	"math"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preserveRealtimePricing(t *testing.T) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	oldModels := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModels))
	})
}

func TestRealtimePreconsumeKeepsCapturedModelAndGroup(t *testing.T) {
	preserveRealtimePricing(t)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"fixture-model":9}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.group_ratio": `{"default":3,"auto":7}`,
	}))
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			user, token := seedAuthoritativeBilling(t, db, "quoted-pre-"+string(mode), 1000, 500, false)
			info := authoritativeRelay(user, token, "quoted-pre-"+string(mode))
			info.UsingGroup = "default"
			info.PriceData = hosttypes.PriceData{ModelRatio: 2, CompletionRatio: 1, AudioRatio: 1, AudioCompletionRatio: 1,
				GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 2}}
			require.NoError(t, info.PriceData.CaptureQuotaUnit(100))
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("auto_group", "auto")
			require.NoError(t, PreWssConsumeQuota(ctx, info, &dto.RealtimeUsage{
				InputTokens: 4, TotalTokens: 4, InputTokenDetails: dto.InputTokenDetails{TextTokens: 4},
			}))
			quota, remain, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 984, quota)
			assert.Equal(t, 484, remain)
			assert.Equal(t, 16, used)
			assert.Equal(t, "default", info.UsingGroup, "session group must not change after its quote")
		})
	}
}

func TestRealtimeSegmentsUseBillingReservationInsteadOfDoubleCharge(t *testing.T) {
	preserveRealtimePricing(t)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"fixture-model":1}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.group_ratio": `{"default":1}`,
	}))
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			user, token := seedAuthoritativeBilling(t, db, "quoted-session-"+string(mode), 1000, 500, false)
			info := authoritativeRelay(user, token, "quoted-session-"+string(mode))
			// Notification delivery is outside this accounting fixture. As in
			// the existing fallback contract, avoid launching delivery workers
			// that outlive the isolated DB/cache lifecycle.
			info.UserQuota = 1 << 30
			info.ForcePreConsume = true
			info.UsingGroup = "default"
			info.PriceData = hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, AudioRatio: 1, AudioCompletionRatio: 1,
				GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}
			require.NoError(t, info.PriceData.CaptureQuotaUnit(100))
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
			require.Nil(t, PreConsumeBilling(ctx, 100, info))
			// Legacy admission refreshes the notification snapshot from the DB.
			// Keep that non-accounting snapshot high after admission as well;
			// actual user/key balances and all settlement assertions stay intact.
			info.UserQuota = 1 << 30
			usage := &dto.RealtimeUsage{InputTokens: 38, TotalTokens: 38,
				InputTokenDetails: dto.InputTokenDetails{TextTokens: 38}}
			for range 3 { // three distinct provider responses, not a replay
				require.NoError(t, PreWssConsumeQuota(ctx, info, usage))
			}
			require.NoError(t, SettleBilling(ctx, info, 114))
			quota, remain, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 886, quota)
			assert.Equal(t, 386, remain)
			assert.Equal(t, 114, used)
			require.NoError(t, SettleBilling(ctx, info, 114))
			quota, remain, used = loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 886, quota)
			assert.Equal(t, 386, remain)
			assert.Equal(t, 114, used)
		})
	}
}

func TestRealtimeReservationRejectsUnavailableIncrementAndOverflow(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			user, token := seedAuthoritativeBilling(t, db, "realtime-limit-"+string(mode), 1000, 120, false)
			info := authoritativeRelay(user, token, "realtime-limit-"+string(mode))
			info.ForcePreConsume = true
			info.PriceData = hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, AudioRatio: 1, AudioCompletionRatio: 1,
				GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}
			require.NoError(t, info.PriceData.CaptureQuotaUnit(100))
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
			require.Nil(t, PreConsumeBilling(ctx, 100, info))
			usage := &dto.RealtimeUsage{InputTokens: 38, TotalTokens: 38, InputTokenDetails: dto.InputTokenDetails{TextTokens: 38}}
			for range 3 {
				require.NoError(t, PreWssConsumeQuota(ctx, info, usage))
			}
			require.Error(t, PreWssConsumeQuota(ctx, info, usage), "increment exceeds remaining key quota")
			assert.Equal(t, 114, info.RealtimeQuotedQuota)
			assert.Equal(t, 3, info.RealtimeConsumeSeq)
			quota, remain, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 886, quota)
			assert.Equal(t, 6, remain)
			assert.Equal(t, 114, used)

			info.RealtimeQuotedQuota = common.MaxQuota
			require.Error(t, PreWssConsumeQuota(ctx, info, usage), "cumulative quota must not overflow")
			assert.Equal(t, common.MaxQuota, info.RealtimeQuotedQuota)
			require.NotNil(t, info.QuotaClamp)
			assert.Equal(t, 3, info.RealtimeConsumeSeq)
			quota, remain, used = loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 886, quota)
			assert.Equal(t, 6, remain)
			assert.Equal(t, 114, used)
			info.RealtimeQuotedQuota = 114
			info.PriceData.ModelRatio = math.MaxFloat64
			require.Error(t, PreWssConsumeQuota(ctx, info, usage), "one segment must not saturate into a permitted charge")
			assert.Equal(t, 114, info.RealtimeQuotedQuota)
			quota, remain, used = loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 886, quota)
			assert.Equal(t, 6, remain)
			assert.Equal(t, 114, used)
		})
	}
}

type realtimeReserveFailure struct {
	textQuotaTestSettler
	targets []int
	err     error
}

func (s *realtimeReserveFailure) Reserve(target int) error {
	s.targets = append(s.targets, target)
	return s.err
}

func TestRealtimeFailedReserveDoesNotAdvanceQuotaOrSequence(t *testing.T) {
	failure := errors.New("synthetic reservation failure")
	settler := &realtimeReserveFailure{textQuotaTestSettler: textQuotaTestSettler{preConsumed: 100}, err: failure}
	info := &relaycommon.RelayInfo{OriginModelName: "failed-reserve-fixture", Billing: settler,
		PriceData: hosttypes.PriceData{ModelRatio: 2, CompletionRatio: 1, AudioRatio: 1, AudioCompletionRatio: 1,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
	require.NoError(t, info.PriceData.CaptureQuotaUnit(100))
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	usage := &dto.RealtimeUsage{InputTokens: 4, TotalTokens: 4, InputTokenDetails: dto.InputTokenDetails{TextTokens: 4}}
	require.ErrorIs(t, PreWssConsumeQuota(ctx, info, usage), failure)
	assert.Zero(t, info.RealtimeQuotedQuota)
	assert.Zero(t, info.RealtimeConsumeSeq)
	settler.err = nil
	require.NoError(t, PreWssConsumeQuota(ctx, info, usage))
	assert.Equal(t, []int{8, 8}, settler.targets, "retry must not count the failed segment twice")
	assert.Equal(t, 8, info.RealtimeQuotedQuota)
	assert.Equal(t, 1, info.RealtimeConsumeSeq)
}
