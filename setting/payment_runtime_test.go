package setting

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentRuntimePublishesDetachedImmutableGeneration(t *testing.T) {
	payMethods := []map[string]string{{
		"name": "Fixture Pay",
		"type": "fixture",
	}}
	payMethodsValue, err := NewPaymentJSONValue(payMethods)
	require.NoError(t, err)

	priceValue, err := NewPaymentNumberValue(7.3)
	require.NoError(t, err)
	exchangeValue, err := NewPaymentNumberValue(7.3)
	require.NoError(t, err)
	epayKeyValue, err := NewPaymentStringValue("epay-sensitive-fixture")
	require.NoError(t, err)
	stripeSecretValue, err := NewPaymentStringValue("stripe-sensitive-fixture-v1")
	require.NoError(t, err)
	creemKeyValue, err := NewPaymentStringValue("creem-sensitive-fixture")
	require.NoError(t, err)
	waffoKeyValue, err := NewPaymentStringValue("waffo-sensitive-fixture")
	require.NoError(t, err)
	pancakeKeyValue, err := NewPaymentStringValue("pancake-sensitive-fixture")
	require.NoError(t, err)
	creemProductsValue, err := NewPaymentJSONValue([]map[string]string{{"productId": "fixture-product"}})
	require.NoError(t, err)

	keyring := []PaymentKeyMetadata{{
		KeyID:           "stripe-key-v1",
		Provider:        PaymentProviderStripe,
		Revision:        1,
		ActivatedAtUnix: 100,
		ExpiresAtUnix:   300,
	}}
	keyringValue, err := NewPaymentKeyringMetadataValue(keyring)
	require.NoError(t, err)

	initial := []PaymentInitialValue{
		{Key: PaymentOptionPrice, Value: priceValue},
		{Key: PaymentOptionMinTopUp, Value: NewPaymentIntegerValue(1)},
		{Key: PaymentOptionUSDExchangeRate, Value: exchangeValue},
		{Key: PaymentOptionPayMethods, Value: payMethodsValue},
		{Key: PaymentOptionEpayKey, Value: epayKeyValue},
		{Key: PaymentOptionStripeAPISecret, Value: stripeSecretValue},
		{Key: PaymentOptionCreemAPIKey, Value: creemKeyValue},
		{Key: PaymentOptionCreemProducts, Value: creemProductsValue},
		{Key: PaymentOptionWaffoPrivateKey, Value: waffoKeyValue},
		{Key: PaymentOptionWaffoPancakePrivateKey, Value: pancakeKeyValue},
		{Key: PaymentOptionWebhookKeyringMetadata, Value: keyringValue},
	}
	runtime, err := NewPaymentRuntime(initial)
	require.NoError(t, err)

	// Mutating every caller-owned collection after construction must not reach
	// the runtime generation.
	payMethods[0]["name"] = "caller mutation"
	keyring[0].KeyID = "caller-mutation"
	initial[0].Value = NewPaymentIntegerValue(999)

	base := runtime.Current()
	assert.Equal(t, uint64(1), base.Revision())

	updatedStripeSecret, err := NewPaymentStringValue("stripe-sensitive-fixture-v2")
	require.NoError(t, err)
	mutations := []PaymentMutation{
		{Key: PaymentOptionPrice, Action: PaymentMutationKeep},
		{Key: PaymentOptionMinTopUp, Action: PaymentMutationSet, Value: NewPaymentIntegerValue(2)},
		{Key: PaymentOptionEpayKey, Action: PaymentMutationClear},
		{Key: PaymentOptionStripeAPISecret, Action: PaymentMutationSet, Value: updatedStripeSecret},
	}
	candidate, err := runtime.PrepareCandidate(base, mutations)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), candidate.Revision())

	mutations[1].Value = NewPaymentIntegerValue(999)
	candidateOptions := candidate.CanonicalOptions()
	candidateOptions[string(PaymentOptionStripeAPISecret)] = "caller mutation"
	delete(candidateOptions, string(PaymentOptionPrice))

	require.NoError(t, candidate.Publish())
	published := runtime.Current()
	assert.Equal(t, uint64(2), published.Revision())

	price, ok := published.Value(PaymentOptionPrice)
	require.True(t, ok)
	actualPrice, ok := price.NumberValue()
	require.True(t, ok)
	assert.Equal(t, 7.3, actualPrice)

	minimum, ok := published.Value(PaymentOptionMinTopUp)
	require.True(t, ok)
	actualMinimum, ok := minimum.IntegerValue()
	require.True(t, ok)
	assert.Equal(t, int64(2), actualMinimum)

	_, ok = published.Value(PaymentOptionEpayKey)
	assert.False(t, ok)
	stripeSecret, ok := published.Value(PaymentOptionStripeAPISecret)
	require.True(t, ok)
	actualStripeSecret, ok := stripeSecret.StringValue()
	require.True(t, ok)
	assert.Equal(t, "stripe-sensitive-fixture-v2", actualStripeSecret)
	assert.True(t, stripeSecret.Sensitive())

	methodsValue, ok := published.Value(PaymentOptionPayMethods)
	require.True(t, ok)
	var decodedMethods []map[string]string
	require.NoError(t, methodsValue.DecodeJSON(&decodedMethods))
	require.Len(t, decodedMethods, 1)
	assert.Equal(t, "Fixture Pay", decodedMethods[0]["name"])
	decodedMethods[0]["name"] = "returned mutation"
	var decodedAgain []map[string]string
	require.NoError(t, methodsValue.DecodeJSON(&decodedAgain))
	assert.Equal(t, "Fixture Pay", decodedAgain[0]["name"])

	rawMethods, ok := methodsValue.JSONValue()
	require.True(t, ok)
	rawMethods[0] = '!'
	actualMethodsValue, ok := published.Value(PaymentOptionPayMethods)
	require.True(t, ok)
	var decodedAfterByteMutation []map[string]string
	require.NoError(t, actualMethodsValue.DecodeJSON(&decodedAfterByteMutation))
	assert.Equal(t, "Fixture Pay", decodedAfterByteMutation[0]["name"])

	publishedKeyring, ok := published.Value(PaymentOptionWebhookKeyringMetadata)
	require.True(t, ok)
	metadata, ok := publishedKeyring.KeyringMetadata()
	require.True(t, ok)
	require.Len(t, metadata, 1)
	assert.Equal(t, "stripe-key-v1", metadata[0].KeyID)
	metadata[0].KeyID = "returned mutation"
	metadataAgain, ok := publishedKeyring.KeyringMetadata()
	require.True(t, ok)
	assert.Equal(t, "stripe-key-v1", metadataAgain[0].KeyID)

	detachedValues := published.Values()
	delete(detachedValues, PaymentOptionPrice)
	detachedOptions := published.CanonicalOptions()
	detachedOptions[string(PaymentOptionMinTopUp)] = "999"
	assert.Contains(t, published.Values(), PaymentOptionPrice)
	assert.Equal(t, "2", published.CanonicalOptions()[string(PaymentOptionMinTopUp)])
}

func TestPaymentRuntimeRejectsInvalidMutationSemanticsWithoutPublication(t *testing.T) {
	price, err := NewPaymentNumberValue(7.3)
	require.NoError(t, err)
	runtime, err := NewPaymentRuntime([]PaymentInitialValue{{Key: PaymentOptionPrice, Value: price}})
	require.NoError(t, err)
	base := runtime.Current()
	wantOptions := base.CanonicalOptions()

	stringValue, err := NewPaymentStringValue("fixture-value")
	require.NoError(t, err)
	negativeNumber, err := NewPaymentNumberValue(-1)
	require.NoError(t, err)

	tests := []struct {
		name      string
		mutations []PaymentMutation
		wantError error
	}{
		{
			name:      "unknown",
			mutations: []PaymentMutation{{Key: PaymentOptionKey("not-a-payment-option"), Action: PaymentMutationSet, Value: stringValue}},
			wantError: ErrPaymentRuntimeUnknownOption,
		},
		{
			name: "duplicate",
			mutations: []PaymentMutation{
				{Key: PaymentOptionPrice, Action: PaymentMutationKeep},
				{Key: PaymentOptionPrice, Action: PaymentMutationClear},
			},
			wantError: ErrPaymentRuntimeDuplicateOption,
		},
		{
			name:      "set zero value",
			mutations: []PaymentMutation{{Key: PaymentOptionPrice, Action: PaymentMutationSet}},
			wantError: ErrPaymentRuntimeNullLikeValue,
		},
		{
			name:      "keep with value",
			mutations: []PaymentMutation{{Key: PaymentOptionPrice, Action: PaymentMutationKeep, Value: price}},
			wantError: ErrPaymentRuntimeInvalidValue,
		},
		{
			name:      "clear with value",
			mutations: []PaymentMutation{{Key: PaymentOptionPrice, Action: PaymentMutationClear, Value: price}},
			wantError: ErrPaymentRuntimeInvalidValue,
		},
		{
			name:      "wrong type",
			mutations: []PaymentMutation{{Key: PaymentOptionPrice, Action: PaymentMutationSet, Value: NewPaymentIntegerValue(8)}},
			wantError: ErrPaymentRuntimeValueType,
		},
		{
			name:      "negative price",
			mutations: []PaymentMutation{{Key: PaymentOptionPrice, Action: PaymentMutationSet, Value: negativeNumber}},
			wantError: ErrPaymentRuntimeInvalidValue,
		},
		{
			name:      "invalid action",
			mutations: []PaymentMutation{{Key: PaymentOptionPrice, Action: PaymentMutationInvalid}},
			wantError: ErrPaymentRuntimeInvalidValue,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate, prepareErr := runtime.PrepareCandidate(base, test.mutations)
			assert.Nil(t, candidate)
			assert.ErrorIs(t, prepareErr, test.wantError)
			assert.Equal(t, base.Revision(), runtime.Current().Revision())
			assert.Equal(t, wantOptions, runtime.Current().CanonicalOptions())
		})
	}

	for _, nullLike := range []string{"", "   ", "null", "NULL", `"null"`} {
		_, valueErr := NewPaymentStringValue(nullLike)
		assert.ErrorIs(t, valueErr, ErrPaymentRuntimeNullLikeValue)
	}
	_, err = NewPaymentJSONValue(nil)
	assert.ErrorIs(t, err, ErrPaymentRuntimeNullLikeValue)
	_, err = NewPaymentJSONValue("not-an-object-or-array")
	assert.ErrorIs(t, err, ErrPaymentRuntimeInvalidValue)
	_, err = NewPaymentKeyringMetadataValue(nil)
	assert.ErrorIs(t, err, ErrPaymentRuntimeNullLikeValue)

	_, err = NewPaymentRuntime([]PaymentInitialValue{
		{Key: PaymentOptionPrice, Value: price},
		{Key: PaymentOptionPrice, Value: price},
	})
	assert.ErrorIs(t, err, ErrPaymentRuntimeDuplicateOption)
}

func TestPaymentCandidateAbortAndStalePublication(t *testing.T) {
	price, err := NewPaymentNumberValue(7.3)
	require.NoError(t, err)
	runtime, err := NewPaymentRuntime([]PaymentInitialValue{{Key: PaymentOptionPrice, Value: price}})
	require.NoError(t, err)
	base := runtime.Current()

	priceA, err := NewPaymentNumberValue(8)
	require.NoError(t, err)
	priceB, err := NewPaymentNumberValue(9)
	require.NoError(t, err)
	candidateA, err := runtime.PrepareCandidate(base, []PaymentMutation{{
		Key: PaymentOptionPrice, Action: PaymentMutationSet, Value: priceA,
	}})
	require.NoError(t, err)
	candidateB, err := runtime.PrepareCandidate(base, []PaymentMutation{{
		Key: PaymentOptionPrice, Action: PaymentMutationSet, Value: priceB,
	}})
	require.NoError(t, err)

	require.NoError(t, candidateA.Publish())
	assert.ErrorIs(t, candidateA.Publish(), ErrPaymentRuntimeCandidateFinalized)
	assert.False(t, candidateA.Abort())
	assert.ErrorIs(t, candidateB.Publish(), ErrPaymentRuntimeConflict)

	aborted, err := runtime.PrepareCandidate(runtime.Current(), []PaymentMutation{{
		Key: PaymentOptionPrice, Action: PaymentMutationKeep,
	}})
	require.NoError(t, err)
	assert.True(t, aborted.Abort())
	assert.False(t, aborted.Abort())
	assert.ErrorIs(t, aborted.Publish(), ErrPaymentRuntimeCandidateAborted)
}

func TestPaymentRuntimeConcurrentPublishKeepsWholeGeneration(t *testing.T) {
	price, err := NewPaymentNumberValue(7.3)
	require.NoError(t, err)
	runtime, err := NewPaymentRuntime([]PaymentInitialValue{
		{Key: PaymentOptionPrice, Value: price},
		{Key: PaymentOptionMinTopUp, Value: NewPaymentIntegerValue(1)},
	})
	require.NoError(t, err)
	base := runtime.Current()

	priceA, err := NewPaymentNumberValue(8)
	require.NoError(t, err)
	priceB, err := NewPaymentNumberValue(9)
	require.NoError(t, err)
	candidateA, err := runtime.PrepareCandidate(base, []PaymentMutation{
		{Key: PaymentOptionPrice, Action: PaymentMutationSet, Value: priceA},
		{Key: PaymentOptionMinTopUp, Action: PaymentMutationSet, Value: NewPaymentIntegerValue(2)},
	})
	require.NoError(t, err)
	candidateB, err := runtime.PrepareCandidate(base, []PaymentMutation{
		{Key: PaymentOptionPrice, Action: PaymentMutationSet, Value: priceB},
		{Key: PaymentOptionMinTopUp, Action: PaymentMutationSet, Value: NewPaymentIntegerValue(3)},
	})
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, candidate := range []*PaymentCandidate{candidateA, candidateB} {
		wait.Add(1)
		go func(candidate *PaymentCandidate) {
			defer wait.Done()
			<-start
			results <- candidate.Publish()
		}(candidate)
	}
	close(start)
	wait.Wait()
	close(results)

	var publishedCount int
	var conflictCount int
	for publishErr := range results {
		switch {
		case publishErr == nil:
			publishedCount++
		case errors.Is(publishErr, ErrPaymentRuntimeConflict):
			conflictCount++
		default:
			require.NoError(t, publishErr)
		}
	}
	assert.Equal(t, 1, publishedCount)
	assert.Equal(t, 1, conflictCount)

	published := runtime.Current()
	publishedPrice, ok := published.Value(PaymentOptionPrice)
	require.True(t, ok)
	actualPrice, ok := publishedPrice.NumberValue()
	require.True(t, ok)
	publishedMinimum, ok := published.Value(PaymentOptionMinTopUp)
	require.True(t, ok)
	actualMinimum, ok := publishedMinimum.IntegerValue()
	require.True(t, ok)
	assert.True(t,
		(actualPrice == 8 && actualMinimum == 2) || (actualPrice == 9 && actualMinimum == 3),
		"published values must come from one complete candidate generation",
	)
}

func TestPaymentRuntimeKeepsTopupGroupRatioInTheSameDetachedGeneration(t *testing.T) {
	initialRatio := map[string]float64{"default": 1}
	initialValue, err := NewPaymentJSONValue(initialRatio)
	require.NoError(t, err)
	runtime, err := NewPaymentRuntime([]PaymentInitialValue{{
		Key: PaymentOptionTopupGroupRatio, Value: initialValue,
	}})
	require.NoError(t, err)

	initialRatio["default"] = 99
	base := runtime.Current()
	storedInitial, ok := base.Value(PaymentOptionTopupGroupRatio)
	require.True(t, ok)
	var decodedInitial map[string]float64
	require.NoError(t, storedInitial.DecodeJSON(&decodedInitial))
	assert.Equal(t, 1.0, decodedInitial["default"])

	updatedRatio := map[string]float64{"default": 0.8}
	updatedValue, err := NewPaymentJSONValue(updatedRatio)
	require.NoError(t, err)
	candidate, err := runtime.PrepareCandidate(base, []PaymentMutation{{
		Key: PaymentOptionTopupGroupRatio, Action: PaymentMutationSet, Value: updatedValue,
	}})
	require.NoError(t, err)
	updatedRatio["default"] = 0.1
	require.NoError(t, candidate.Publish())

	published := runtime.Current()
	storedUpdated, ok := published.Value(PaymentOptionTopupGroupRatio)
	require.True(t, ok)
	var decodedUpdated map[string]float64
	require.NoError(t, storedUpdated.DecodeJSON(&decodedUpdated))
	assert.Equal(t, 0.8, decodedUpdated["default"])

	clearCandidate, err := runtime.PrepareCandidate(published, []PaymentMutation{{
		Key: PaymentOptionTopupGroupRatio, Action: PaymentMutationClear,
	}})
	require.NoError(t, err)
	require.NoError(t, clearCandidate.Publish())
	_, ok = runtime.Current().Value(PaymentOptionTopupGroupRatio)
	assert.False(t, ok)

	unknownValue, err := NewPaymentJSONValue(map[string]float64{"default": 1})
	require.NoError(t, err)
	unknownCandidate, prepareErr := runtime.PrepareCandidate(runtime.Current(), []PaymentMutation{{
		Key: PaymentOptionKey("TopupGroupRatioLegacy"), Action: PaymentMutationSet, Value: unknownValue,
	}})
	assert.Nil(t, unknownCandidate)
	assert.ErrorIs(t, prepareErr, ErrPaymentRuntimeUnknownOption)
}

func TestPaymentRuntimeFormattingAndErrorsRedactSensitiveValues(t *testing.T) {
	const sensitiveFixture = "payment-sensitive-fixture-must-not-leak"
	secret, err := NewPaymentStringValue(sensitiveFixture)
	require.NoError(t, err)
	initial := PaymentInitialValue{Key: PaymentOptionStripeAPISecret, Value: secret}
	runtime, err := NewPaymentRuntime([]PaymentInitialValue{initial})
	require.NoError(t, err)
	snapshot := runtime.Current()
	storedSecret, ok := snapshot.Value(PaymentOptionStripeAPISecret)
	require.True(t, ok)
	mutation := PaymentMutation{Key: PaymentOptionStripeAPISecret, Action: PaymentMutationSet, Value: secret}
	candidate, err := runtime.PrepareCandidate(snapshot, []PaymentMutation{mutation})
	require.NoError(t, err)
	values := snapshot.Values()
	options := snapshot.CanonicalOptions()
	assert.Equal(t, sensitiveFixture, options[string(PaymentOptionStripeAPISecret)])

	metadata := PaymentKeyMetadata{
		KeyID:           sensitiveFixture,
		Provider:        PaymentProviderStripe,
		Revision:        1,
		ActivatedAtUnix: 100,
	}
	formatted := []any{secret, storedSecret, initial, mutation, runtime, snapshot, candidate, values, options, metadata}
	for _, value := range formatted {
		output := fmt.Sprintf("%v|%+v|%#v", value, value, value)
		assert.NotContains(t, output, sensitiveFixture)
	}

	_, err = NewPaymentRuntime([]PaymentInitialValue{{
		Key: PaymentOptionKey(sensitiveFixture), Value: secret,
	}})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), sensitiveFixture)
}
