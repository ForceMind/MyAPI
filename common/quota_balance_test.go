package common

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckedQuotaBalance(t *testing.T) {
	assert.Equal(t, int64(1<<53)-1, MaxQuotaBalance)

	tests := []struct {
		name    string
		current int64
		delta   int64
		want    int64
		valid   bool
	}{
		{name: "zero", valid: true},
		{name: "crosses positive int32", current: int64(MaxQuota), delta: 1, want: int64(MaxQuota) + 1, valid: true},
		{name: "crosses negative int32", current: int64(MinQuota), delta: -1, want: int64(MinQuota) - 1, valid: true},
		{name: "positive boundary", current: MaxQuotaBalance - 1, delta: 1, want: MaxQuotaBalance, valid: true},
		{name: "negative boundary", current: -MaxQuotaBalance + 1, delta: -1, want: -MaxQuotaBalance, valid: true},
		{name: "wide transition delta", current: -MaxQuotaBalance, delta: 2 * MaxQuotaBalance, want: MaxQuotaBalance, valid: true},
		{name: "positive overflow", current: MaxQuotaBalance, delta: 1},
		{name: "negative overflow", current: -MaxQuotaBalance, delta: -1},
		{name: "invalid positive current", current: MaxQuotaBalance + 1},
		{name: "invalid negative current", current: -MaxQuotaBalance - 1},
		{name: "invalid current cannot be repaired", current: MaxQuotaBalance + 1, delta: -1},
		{name: "max int delta", delta: math.MaxInt64},
		{name: "min int delta", delta: math.MinInt64},
		{name: "min int delta from boundary", current: MaxQuotaBalance, delta: math.MinInt64},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CheckedQuotaBalance(test.current, test.delta)
			if test.valid {
				require.NoError(t, err)
				assert.Equal(t, test.want, got)
				return
			}
			require.ErrorIs(t, err, ErrQuotaBalanceOutOfRange)
			assert.Zero(t, got)
		})
	}
}

func TestCheckedQuotaUsed(t *testing.T) {
	tests := []struct {
		name    string
		current int64
		delta   int64
		want    int64
		valid   bool
	}{
		{name: "zero", valid: true},
		{name: "crosses int32", current: int64(MaxQuota), delta: 1, want: int64(MaxQuota) + 1, valid: true},
		{name: "maximum", current: MaxQuotaBalance - 1, delta: 1, want: MaxQuotaBalance, valid: true},
		{name: "refund to zero", current: MaxQuotaBalance, delta: -MaxQuotaBalance, valid: true},
		{name: "negative current", current: -1},
		{name: "negative current cannot be repaired", current: -1, delta: 1},
		{name: "current above maximum", current: MaxQuotaBalance + 1},
		{name: "underflow", current: 0, delta: -1},
		{name: "overflow", current: MaxQuotaBalance, delta: 1},
		{name: "max int delta", delta: math.MaxInt64},
		{name: "min int delta", current: MaxQuotaBalance, delta: math.MinInt64},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CheckedQuotaUsed(test.current, test.delta)
			if test.valid {
				require.NoError(t, err)
				assert.Equal(t, test.want, got)
				return
			}
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrQuotaBalanceOutOfRange))
			assert.Zero(t, got)
		})
	}
}
