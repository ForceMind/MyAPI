package common

import "errors"

// MaxQuotaBalance is the largest persisted quota state that remains exactly
// representable across the Go, JSON, and Redis/Lua boundaries used by MyAPI.
// It is a storage and transport bound, not a per-request charge limit. Single
// request billing continues to use MaxQuota and MinQuota.
const MaxQuotaBalance int64 = (1 << 53) - 1

// ErrQuotaBalanceOutOfRange reports that an existing quota state or its target
// cannot be represented exactly by the shared quota-state contract.
var ErrQuotaBalanceOutOfRange = errors.New("quota balance is outside the exact representable range")

// CheckedQuotaBalance applies delta to a signed persisted quota balance. The
// current state is validated before delta, and the target is checked using
// boundary subtraction before addition so even MinInt64 cannot wrap.
func CheckedQuotaBalance(current, delta int64) (int64, error) {
	return checkedQuotaBalanceTarget(current, delta, -MaxQuotaBalance, MaxQuotaBalance)
}

// CheckedQuotaUsed applies delta to a non-negative persisted used-quota or
// quota-statistics state. Refund-shaped negative deltas are allowed only when
// the resulting state remains non-negative.
func CheckedQuotaUsed(current, delta int64) (int64, error) {
	return checkedQuotaBalanceTarget(current, delta, 0, MaxQuotaBalance)
}

func checkedQuotaBalanceTarget(current, delta, minimum, maximum int64) (int64, error) {
	if current < minimum || current > maximum {
		return 0, ErrQuotaBalanceOutOfRange
	}
	if delta > 0 && delta > maximum-current {
		return 0, ErrQuotaBalanceOutOfRange
	}
	if delta < 0 && delta < minimum-current {
		return 0, ErrQuotaBalanceOutOfRange
	}
	return current + delta, nil
}
