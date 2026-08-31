package controller

import (
	"math"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettleTestQuotaUsesCheckedTokenConversion(t *testing.T) {
	tests := []struct {
		name            string
		completion      int
		completionRatio float64
		modelRatio      float64
		want            int
		wantKind        common.QuotaClampKind
	}{
		{name: "normal", completion: 3, completionRatio: 2, modelRatio: 1.5, want: 17},
		{name: "overflow", completion: 1, completionRatio: math.Inf(1), modelRatio: 1, want: 0, wantKind: common.QuotaClampOverflow},
		{name: "nan", completion: 1, completionRatio: math.NaN(), modelRatio: 1, want: 0, wantKind: common.QuotaClampNaN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{}
			quota, result := settleTestQuota(info, hosttypes.PriceData{
				CompletionRatio: tt.completionRatio,
				ModelRatio:      tt.modelRatio,
			}, &dto.Usage{PromptTokens: 5, CompletionTokens: tt.completion})
			require.Nil(t, result)
			assert.Equal(t, tt.want, quota)
			if tt.wantKind == "" {
				assert.Nil(t, info.QuotaClamp)
			} else {
				require.NotNil(t, info.QuotaClamp)
				assert.Equal(t, tt.wantKind, info.QuotaClamp.Kind)
			}
		})
	}
}

func TestSettleTestQuotaUsesCheckedPriceConversion(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	quota, result := settleTestQuota(info, hosttypes.PriceData{
		UsePrice:   true,
		ModelPrice: math.Inf(1),
	}, &dto.Usage{})

	require.Nil(t, result)
	assert.Zero(t, quota)
	require.NotNil(t, info.QuotaClamp)
	assert.Equal(t, common.QuotaClampOverflow, info.QuotaClamp.Kind)
}
