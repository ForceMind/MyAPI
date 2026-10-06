package setting

import (
	"errors"
	"strconv"

	"github.com/ForceMind/MyAPI/common"
)

// paymentPublishMaxConflictRetries bounds the compare-and-swap retry loop when
// a concurrent writer advances the runtime between snapshot capture and
// publish. Option writers are serialized by the model package's mutation
// lock, so more than a handful of conflicts indicates a logic error, not
// contention.
const paymentPublishMaxConflictRetries = 3

// PaymentMutationFromCanonicalOption converts one committed option value (the
// canonical string form stored in the options table / OptionMap) into a fully
// validated runtime mutation for the same key:
//
//   - empty string values become PaymentMutationClear (an absent key means
//     "not configured", matching the legacy zero-value semantics);
//   - non-empty values become PaymentMutationSet with the typed value
//     required by the option's registered kind;
//   - values that cannot be represented in the runtime (malformed number,
//     invalid JSON, out-of-range constraint) return an error so the caller
//     can fail the save instead of publishing a partial configuration.
//
// The webhook keyring metadata option is not representable as a canonical
// option string and is rejected here; it is written by the keyring
// implementation, not by option persistence.
func PaymentMutationFromCanonicalOption(key PaymentOptionKey, value string) (PaymentMutation, error) {
	spec, ok := paymentOptionSpecs[key]
	if !ok {
		return PaymentMutation{}, ErrPaymentRuntimeUnknownOption
	}
	if spec.kind == PaymentValueKeyringMetadata {
		return PaymentMutation{}, ErrPaymentRuntimeValueType
	}
	if value == "" {
		return PaymentMutation{Key: key, Action: PaymentMutationClear}, nil
	}
	var (
		typed PaymentValue
		err   error
	)
	switch spec.kind {
	case PaymentValueString:
		typed, err = NewPaymentStringValue(value)
	case PaymentValueInteger:
		var integer int64
		integer, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil {
			return PaymentMutation{}, ErrPaymentRuntimeInvalidValue
		}
		typed = NewPaymentIntegerValue(integer)
	case PaymentValueNumber:
		var number float64
		number, parseErr := strconv.ParseFloat(value, 64)
		if parseErr != nil {
			return PaymentMutation{}, ErrPaymentRuntimeInvalidValue
		}
		typed, err = NewPaymentNumberValue(number)
	case PaymentValueBoolean:
		typed = NewPaymentBooleanValue(value == "true")
	case PaymentValueJSON:
		var decoded any
		if parseErr := common.UnmarshalJsonStr(value, &decoded); parseErr != nil {
			return PaymentMutation{}, ErrPaymentRuntimeInvalidValue
		}
		typed, err = NewPaymentJSONValue(decoded)
	default:
		return PaymentMutation{}, ErrPaymentRuntimeValueType
	}
	if err != nil {
		return PaymentMutation{}, err
	}
	if _, err = validatedPaymentValue(key, typed); err != nil {
		return PaymentMutation{}, err
	}
	return PaymentMutation{Key: key, Action: PaymentMutationSet, Value: typed}, nil
}

// paymentMutationMatchesSnapshot reports whether applying the mutation to the
// snapshot would leave the generation unchanged, letting publishers skip
// no-op republishes (option reload re-publishes every persisted key).
func paymentMutationMatchesSnapshot(snapshot PaymentSnapshot, mutation PaymentMutation) bool {
	if snapshot.generation == nil {
		return false
	}
	switch mutation.Action {
	case PaymentMutationClear:
		_, exists := snapshot.generation.values[mutation.Key]
		return !exists
	case PaymentMutationSet:
		current, exists := snapshot.generation.canonicalOptions[string(mutation.Key)]
		if !exists {
			return false
		}
		encoded, err := canonicalPaymentValue(mutation.Value)
		return err == nil && current == encoded
	default:
		return false
	}
}

// PublishPaymentCanonicalMutation publishes a single committed payment option
// as its own runtime generation. It returns false when the current generation
// already carries the mutation (no revision bump). Conflicts against a
// concurrent publisher are retried; validation errors abort immediately.
func PublishPaymentCanonicalMutation(mutation PaymentMutation) (bool, error) {
	return PublishPaymentCanonicalMutations([]PaymentMutation{mutation})
}

// PublishPaymentCanonicalMutations publishes a group of committed payment
// options as exactly one runtime generation. Either the whole group becomes
// visible atomically or the runtime stays on the previous generation; a
// failed publish never leaves a partial group behind.
func PublishPaymentCanonicalMutations(mutations []PaymentMutation) (bool, error) {
	for attempt := 0; attempt < paymentPublishMaxConflictRetries; attempt++ {
		base := packagePaymentRuntime.Current()
		changed := false
		for _, mutation := range mutations {
			if !paymentMutationMatchesSnapshot(base, mutation) {
				changed = true
				break
			}
		}
		if !changed {
			return false, nil
		}
		candidate, err := packagePaymentRuntime.PrepareCandidate(base, mutations)
		if err != nil {
			return false, err
		}
		if err := candidate.Publish(); err != nil {
			if errors.Is(err, ErrPaymentRuntimeConflict) {
				continue
			}
			return false, err
		}
		return true, nil
	}
	return false, ErrPaymentRuntimeConflict
}
