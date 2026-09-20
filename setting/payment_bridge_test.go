package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// restorePaymentRuntimeOptions republishes the captured canonical options for
// the touched keys after the test, so the process-wide package runtime is
// left as it was found.
func restorePaymentRuntimeOptions(t *testing.T, keys ...PaymentOptionKey) {
	t.Helper()
	canonical := CurrentPaymentRuntime().CanonicalOptions()
	t.Cleanup(func() {
		mutations := make([]PaymentMutation, 0, len(keys))
		for _, key := range keys {
			if previous, ok := canonical[string(key)]; ok {
				mutation, err := PaymentMutationFromCanonicalOption(key, previous)
				require.NoError(t, err)
				mutations = append(mutations, mutation)
			} else {
				mutations = append(mutations, PaymentMutation{Key: key, Action: PaymentMutationClear})
			}
		}
		_, err := PublishPaymentCanonicalMutations(mutations)
		require.NoError(t, err)
	})
}

func TestPaymentMutationFromCanonicalOptionKinds(t *testing.T) {
	testCases := []struct {
		name      string
		key       PaymentOptionKey
		value     string
		assertion func(t *testing.T, mutation PaymentMutation)
	}{
		{name: "string", key: PaymentOptionStripeAPISecret, value: "sk_test_kind", assertion: func(t *testing.T, mutation PaymentMutation) {
			assert.Equal(t, PaymentMutationSet, mutation.Action)
			text, ok := mutation.Value.StringValue()
			require.True(t, ok)
			assert.Equal(t, "sk_test_kind", text)
		}},
		{name: "empty string clears", key: PaymentOptionStripeAPISecret, value: "", assertion: func(t *testing.T, mutation PaymentMutation) {
			assert.Equal(t, PaymentMutationClear, mutation.Action)
		}},
		{name: "integer", key: PaymentOptionStripeMinTopUp, value: "3", assertion: func(t *testing.T, mutation PaymentMutation) {
			integer, ok := mutation.Value.IntegerValue()
			require.True(t, ok)
			assert.Equal(t, int64(3), integer)
		}},
		{name: "number", key: PaymentOptionStripeUnitPrice, value: "8.5", assertion: func(t *testing.T, mutation PaymentMutation) {
			number, ok := mutation.Value.NumberValue()
			require.True(t, ok)
			assert.Equal(t, 8.5, number)
		}},
		{name: "boolean true", key: PaymentOptionCreemTestMode, value: "true", assertion: func(t *testing.T, mutation PaymentMutation) {
			boolean, ok := mutation.Value.BooleanValue()
			require.True(t, ok)
			assert.True(t, boolean)
		}},
		{name: "boolean non-true is false", key: PaymentOptionCreemTestMode, value: "off", assertion: func(t *testing.T, mutation PaymentMutation) {
			boolean, ok := mutation.Value.BooleanValue()
			require.True(t, ok)
			assert.False(t, boolean)
		}},
		{name: "json", key: PaymentOptionCreemProducts, value: `[{"productId":"prod_kind"}]`, assertion: func(t *testing.T, mutation PaymentMutation) {
			var decoded []map[string]string
			require.NoError(t, mutation.Value.DecodeJSON(&decoded))
			require.Len(t, decoded, 1)
			assert.Equal(t, "prod_kind", decoded[0]["productId"])
		}},
		{name: "funding mode", key: PaymentOptionUserFundingMode, value: "enabled", assertion: func(t *testing.T, mutation PaymentMutation) {
			text, ok := mutation.Value.StringValue()
			require.True(t, ok)
			assert.Equal(t, "enabled", text)
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mutation, err := PaymentMutationFromCanonicalOption(testCase.key, testCase.value)
			require.NoError(t, err)
			assert.Equal(t, testCase.key, mutation.Key)
			testCase.assertion(t, mutation)
		})
	}
}

func TestPaymentMutationFromCanonicalOptionRejectsInvalidValues(t *testing.T) {
	testCases := []struct {
		name  string
		key   PaymentOptionKey
		value string
	}{
		{name: "unknown option", key: PaymentOptionKey("NoSuchPaymentOption"), value: "x"},
		{name: "keyring metadata is not option-bridgeable", key: PaymentOptionWebhookKeyringMetadata, value: "[]"},
		{name: "malformed integer", key: PaymentOptionStripeMinTopUp, value: "not-an-int"},
		{name: "zero violates positive integer", key: PaymentOptionStripeMinTopUp, value: "0"},
		{name: "negative violates positive integer", key: PaymentOptionMinTopUp, value: "-2"},
		{name: "malformed number", key: PaymentOptionStripeUnitPrice, value: "not-a-number"},
		{name: "zero violates positive number", key: PaymentOptionStripeUnitPrice, value: "0"},
		{name: "negative violates positive number", key: PaymentOptionPrice, value: "-1"},
		{name: "null-like string", key: PaymentOptionStripeAPISecret, value: "null"},
		{name: "malformed json", key: PaymentOptionCreemProducts, value: "not-json"},
		{name: "scalar json", key: PaymentOptionPayMethods, value: `"text"`},
		{name: "invalid funding mode", key: PaymentOptionUserFundingMode, value: "sometimes"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := PaymentMutationFromCanonicalOption(testCase.key, testCase.value)
			assert.Error(t, err)
		})
	}
}

func TestPublishPaymentCanonicalMutationSkipsUnchangedValues(t *testing.T) {
	restorePaymentRuntimeOptions(t, PaymentOptionStripePriceID)
	mutation, err := PaymentMutationFromCanonicalOption(PaymentOptionStripePriceID, "price_skip_fixture")
	require.NoError(t, err)

	published, err := PublishPaymentCanonicalMutation(mutation)
	require.NoError(t, err)
	assert.True(t, published)
	revision := CurrentPaymentRuntime().Revision()

	published, err = PublishPaymentCanonicalMutation(mutation)
	require.NoError(t, err)
	assert.False(t, published, "republishing an identical value must not bump the generation")
	assert.Equal(t, revision, CurrentPaymentRuntime().Revision())
}

func TestPaymentConfigPinsOneGenerationAcrossPublishes(t *testing.T) {
	restorePaymentRuntimeOptions(t, PaymentOptionStripeWebhookSecret, PaymentOptionStripeUnitPrice)
	publish, err := PaymentMutationFromCanonicalOption(PaymentOptionStripeWebhookSecret, "whsec_pinned_v1")
	require.NoError(t, err)
	_, err = PublishPaymentCanonicalMutation(publish)
	require.NoError(t, err)

	pinned := CapturePaymentConfig()
	assert.Equal(t, "whsec_pinned_v1", pinned.StripeWebhookSecret())
	pinnedRevision := pinned.Revision()

	// A concurrent save advances the runtime; the pinned request configuration
	// must keep reading its own generation while fresh captures observe the
	// new one.
	updated, err := PaymentMutationFromCanonicalOption(PaymentOptionStripeWebhookSecret, "whsec_pinned_v2")
	require.NoError(t, err)
	_, err = PublishPaymentCanonicalMutation(updated)
	require.NoError(t, err)

	assert.Equal(t, "whsec_pinned_v1", pinned.StripeWebhookSecret())
	assert.Equal(t, pinnedRevision, pinned.Revision())
	assert.Equal(t, "whsec_pinned_v2", CapturePaymentConfig().StripeWebhookSecret())
}

func TestPaymentConfigFallsBackToLegacyVariablesForAbsentKeys(t *testing.T) {
	restorePaymentRuntimeOptions(t, PaymentOptionStripePriceID)
	_, err := PublishPaymentCanonicalMutation(PaymentMutation{Key: PaymentOptionStripePriceID, Action: PaymentMutationClear})
	require.NoError(t, err)

	previous := StripePriceId
	StripePriceId = "price_legacy_fallback"
	t.Cleanup(func() { StripePriceId = previous })

	assert.Equal(t, "price_legacy_fallback", CapturePaymentConfig().StripePriceId())
}
