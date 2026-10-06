package common

import "context"

// This is an observed account/window routing precondition, not a per-Key
// percentage consumption ledger or a reservation against shared-account use.
type AccountQuotaThreshold struct {
	TokenID             int
	Revision            int64
	MinimumRemainingBPS int
	MaxAgeSeconds       int64
}

type accountQuotaThresholdContextKey struct{}

func WithAccountQuotaThreshold(ctx context.Context, policy AccountQuotaThreshold) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, accountQuotaThresholdContextKey{}, policy)
}

func AccountQuotaThresholdFromContext(ctx context.Context) (AccountQuotaThreshold, bool) {
	if ctx == nil {
		return AccountQuotaThreshold{}, false
	}
	policy, ok := ctx.Value(accountQuotaThresholdContextKey{}).(AccountQuotaThreshold)
	return policy, ok
}

func ValidAccountQuotaThreshold(policy AccountQuotaThreshold) bool {
	return policy.MinimumRemainingBPS >= 0 && policy.MinimumRemainingBPS <= 10000 && policy.MaxAgeSeconds >= 30 && policy.MaxAgeSeconds <= 3600
}
