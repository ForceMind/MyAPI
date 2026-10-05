package service

import (
	"math"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareNonTieredBillingForSelectedGroupUsesFrozenQuote(t *testing.T) {
	oldUnit, oldPreConsume := common.QuotaPerUnit, common.PreConsumedQuota
	common.QuotaPerUnit, common.PreConsumedQuota = 7, 12345
	t.Cleanup(func() { common.QuotaPerUnit, common.PreConsumedQuota = oldUnit, oldPreConsume })
	for _, test := range []struct {
		name                                     string
		fixed                                    bool
		initialGroup, selectedGroup, coefficient float64
		tokens, initialReserve, wantEstimate     int
		wantReserve                              bool
	}{
		{"token higher group", false, .5, 1, .5, 1000, 250, 500, true},
		{"fixed higher group includes ratios", true, .5, 1, .01, 0, 7500, 15000, true},
		{"token same group preserves reservation", false, 1, 1, .5, 1000, 250, 500, false},
		{"token paid to free retains held funds", false, 1, 0, .5, 1000, 500, 0, false},
		{"fixed paid to free retains held funds", true, 1, 0, .01, 0, 15000, 0, false},
		{"token cheaper group refunds only at settlement", false, 2, 1, .5, 1000, 1000, 500, true},
		{"token fractional estimate truncates", false, 0, 1, .5, 3, 0, 1, true},
		{"zero token coefficient remains free", false, 0, 1, 0, 1000, 0, 0, false},
		{"zero fixed price remains free", true, 0, 1, 0, 0, 0, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			price := types.PriceData{FreeModel: test.initialGroup == 0 && test.initialReserve == 0, UsePrice: test.fixed, ModelRatio: test.coefficient, ModelPrice: test.coefficient,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: test.initialGroup}, QuotaToPreConsume: test.wantEstimate}
			require.NoError(t, price.CaptureQuotaUnit(500000))
			require.NoError(t, price.CapturePreConsumeTokens(test.tokens))
			price.AddOtherRatio("n", 3)
			price.GroupRatioInfo.GroupRatio = test.selectedGroup
			billing := &recordingBillingSettler{preConsumedQuota: test.initialReserve}
			info := &relaycommon.RelayInfo{PriceData: price, Billing: billing, FinalPreConsumedQuota: test.initialReserve}
			require.Nil(t, PrepareTieredBillingForSelectedGroup(nil, info))
			assert.Equal(t, test.wantEstimate, info.PriceData.QuotaToPreConsume)
			if test.wantReserve {
				assert.Equal(t, []int{test.wantEstimate}, billing.reserveTargets)
			} else {
				assert.Empty(t, billing.reserveTargets)
			}
			wantHeld := test.initialReserve
			if test.wantReserve {
				wantHeld = max(wantHeld, test.wantEstimate)
			}
			assert.Equal(t, wantHeld, info.FinalPreConsumedQuota)
			assert.Equal(t, test.initialGroup == 0 && test.coefficient == 0, info.PriceData.FreeModel, "only a genuinely zero-priced quote keeps skipped pre-consume")
		})
	}
}

func TestPrepareNonTieredBillingForSelectedGroupRejectsOverflow(t *testing.T) {
	for _, fixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "token", true: "fixed"}[fixed], func(t *testing.T) {
			price := types.PriceData{UsePrice: fixed, ModelRatio: math.MaxInt32, ModelPrice: math.MaxInt32,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0}}
			require.NoError(t, price.CaptureQuotaUnit(500000))
			require.NoError(t, price.CapturePreConsumeTokens(10))
			price.GroupRatioInfo.GroupRatio = 1
			billing := &recordingBillingSettler{}
			info := &relaycommon.RelayInfo{PriceData: price, Billing: billing}
			apiErr := PrepareTieredBillingForSelectedGroup(nil, info)
			require.NotNil(t, apiErr)
			assert.Equal(t, 400, apiErr.StatusCode)
			assert.Empty(t, billing.reserveTargets)
			assert.Zero(t, info.FinalPreConsumedQuota)
		})
	}
}
