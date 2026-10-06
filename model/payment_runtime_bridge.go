package model

import (
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
)

// Bridge between committed payment options and the payment runtime
// (setting.PaymentRuntime). The runtime generation is the authoritative read
// for payment order and webhook paths; OptionMap and the legacy package-level
// variables remain as transition write targets.
//
// Two publication modes share this file:
//
//   - Single-key publication (updateOptionMap): every committed payment key
//     becomes its own generation. Used by option load and the generic option
//     bulk path, matching their existing per-key publication semantics.
//   - Group publication (UpdatePaymentFundingOptionsBulk): the whole funding
//     set plus the funding mode/epoch barrier becomes one candidate. Any
//     failure aborts the candidate and the runtime stays on the previous
//     generation, so a half-saved payment configuration is never observable.

// prepareCommittedPaymentMutation validates a committed option value against
// the payment runtime schema and returns the corresponding mutation. It
// returns nil for keys outside the payment funding set. Running it before any
// in-memory write lets a validation failure abort the key publish without
// touching OptionMap or the legacy variables.
func prepareCommittedPaymentMutation(key string, value string) (*setting.PaymentMutation, error) {
	if !IsPaymentFundingOptionKey(key) {
		return nil, nil
	}
	mutation, err := setting.PaymentMutationFromCanonicalOption(setting.PaymentOptionKey(key), value)
	if err != nil {
		return nil, err
	}
	return &mutation, nil
}

// paymentRuntimePublishMutation publishes one validated payment mutation as
// its own runtime generation. It is a variable so tests can inject
// publication failures; production behavior on failure is abort-the-publish
// (the runtime keeps the previous generation).
var paymentRuntimePublishMutation = func(mutation setting.PaymentMutation) error {
	_, err := setting.PublishPaymentCanonicalMutation(mutation)
	return err
}

// paymentFundingOptionPublish performs the legacy per-key publication inside
// the payment funding bulk loop. It is a variable so tests can inject a
// mid-loop failure and verify that the whole group then aborts with the
// runtime kept on the previous generation.
var paymentFundingOptionPublish = updateOptionMapWithoutRuntimeBridge

// buildPaymentFundingMutations assembles the group mutation for the payment
// funding bulk path: every committed payment key plus the funding mode/epoch
// barrier values from the committed funding state.
func buildPaymentFundingMutations(normalized map[string]string, next UserFundingStateSnapshot) ([]setting.PaymentMutation, error) {
	mutations := make([]setting.PaymentMutation, 0, len(normalized)+2)
	for key, value := range normalized {
		if key == operation_setting.UserFundingModeOptionKey {
			continue
		}
		mutation, err := setting.PaymentMutationFromCanonicalOption(setting.PaymentOptionKey(key), value)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, mutation)
	}
	modeMutation, err := setting.PaymentMutationFromCanonicalOption(
		setting.PaymentOptionUserFundingMode, string(next.Mode))
	if err != nil {
		return nil, err
	}
	mutations = append(mutations, modeMutation)
	mutations = append(mutations, setting.PaymentMutation{
		Key:    setting.PaymentOptionUserFundingEpoch,
		Action: setting.PaymentMutationSet,
		Value:  setting.NewPaymentIntegerValue(int64(next.Epoch)),
	})
	return mutations, nil
}
