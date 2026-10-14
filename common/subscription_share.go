package common

import "context"

type subscriptionShareAdmissionKey struct{}

type SubscriptionShareAdmission struct {
	TokenID int
	Bound   bool
}

func WithSubscriptionShareAdmission(ctx context.Context, admission SubscriptionShareAdmission) context.Context {
	return context.WithValue(ctx, subscriptionShareAdmissionKey{}, admission)
}

func SubscriptionShareAdmissionFromContext(ctx context.Context) (SubscriptionShareAdmission, bool) {
	if ctx == nil {
		return SubscriptionShareAdmission{}, false
	}
	admission, ok := ctx.Value(subscriptionShareAdmissionKey{}).(SubscriptionShareAdmission)
	return admission, ok && admission.TokenID > 0
}
