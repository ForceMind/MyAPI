package service

import (
	"math"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/stretchr/testify/require"
)

func TestCalcViolationFeeQuotaUsesCheckedSaturation(t *testing.T) {
	tests := []struct {
		name       string
		amount     float64
		groupRatio float64
		want       int
		wantClamp  common.QuotaClampKind
	}{
		{name: "normal", amount: 0.01, groupRatio: 2, want: 10000},
		{name: "overflow", amount: math.MaxFloat64, groupRatio: 2, want: 0, wantClamp: common.QuotaClampOverflow},
		{name: "positive infinity ratio", amount: 1, groupRatio: math.Inf(1), want: 0, wantClamp: common.QuotaClampOverflow},
		{name: "nan", amount: math.NaN(), groupRatio: 1, want: 0, wantClamp: common.QuotaClampNaN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quota, clamp := calcViolationFeeQuota(tt.amount, tt.groupRatio)
			require.Equal(t, tt.want, quota)
			if tt.wantClamp == "" {
				require.Nil(t, clamp)
				return
			}
			require.NotNil(t, clamp)
			require.Equal(t, tt.wantClamp, clamp.Kind)
		})
	}
}

func TestNoteQuotaClampCapturesFirstClamp(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	clamp := &common.QuotaClamp{Op: "QuotaRound", Kind: common.QuotaClampOverflow}

	noteQuotaClamp(info, clamp)
	require.Same(t, clamp, info.QuotaClamp)
}
