package model

import (
	"errors"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// capturePaymentRuntimeKeys snapshots the canonical form of the given runtime
// keys and restores them when the test finishes, keeping the process-wide
// payment runtime isolated between tests.
func capturePaymentRuntimeKeys(t *testing.T, keys ...setting.PaymentOptionKey) {
	t.Helper()
	canonical := setting.CurrentPaymentRuntime().CanonicalOptions()
	t.Cleanup(func() {
		mutations := make([]setting.PaymentMutation, 0, len(keys))
		for _, key := range keys {
			if previous, ok := canonical[string(key)]; ok {
				mutation, err := setting.PaymentMutationFromCanonicalOption(key, previous)
				require.NoError(t, err)
				mutations = append(mutations, mutation)
			} else {
				mutations = append(mutations, setting.PaymentMutation{Key: key, Action: setting.PaymentMutationClear})
			}
		}
		_, err := setting.PublishPaymentCanonicalMutations(mutations)
		require.NoError(t, err)
	})
}

func seedComplianceOptions(t *testing.T, db *gorm.DB) {
	t.Helper()
	for key, value := range map[string]string{
		string(setting.PaymentOptionComplianceConfirmed):    "true",
		string(setting.PaymentOptionComplianceTermsVersion): operation_setting.CurrentComplianceTermsVersion,
	} {
		require.NoError(t, db.Create(&Option{Key: key, Value: value}).Error)
	}
}

func TestPaymentFundingBulkPublishesOneRuntimeGeneration(t *testing.T) {
	db, before := userFundingPolicyTestDB(t, operation_setting.UserFundingModeDisabled)
	capturePaymentRuntimeKeys(t,
		setting.PaymentOptionStripeAPISecret,
		setting.PaymentOptionStripeWebhookSecret,
		setting.PaymentOptionStripePriceID,
		setting.PaymentOptionStripeUnitPrice,
		setting.PaymentOptionUserFundingMode,
		setting.PaymentOptionUserFundingEpoch,
	)
	seedComplianceOptions(t, db)
	previousRevision := setting.CurrentPaymentRuntime().Revision()
	previousStripeSecret := setting.StripeApiSecret
	t.Cleanup(func() { setting.StripeApiSecret = previousStripeSecret })

	state, err := UpdatePaymentFundingOptionsBulk(map[string]string{
		operation_setting.UserFundingModeOptionKey:       "enabled",
		string(setting.PaymentOptionStripeAPISecret):     "sk_test_n2b_generation",
		string(setting.PaymentOptionStripeWebhookSecret): "whsec_n2b_generation",
		string(setting.PaymentOptionStripePriceID):       "price_n2b_generation",
		string(setting.PaymentOptionStripeUnitPrice):     "9.5",
	})
	require.NoError(t, err)

	snapshot := setting.CurrentPaymentRuntime()
	assert.Greater(t, snapshot.Revision(), previousRevision)
	paymentConfig := setting.CapturePaymentConfig()
	assert.Equal(t, "sk_test_n2b_generation", paymentConfig.StripeApiSecret())
	assert.Equal(t, "whsec_n2b_generation", paymentConfig.StripeWebhookSecret())
	assert.Equal(t, "price_n2b_generation", paymentConfig.StripePriceId())
	assert.Equal(t, 9.5, paymentConfig.StripeUnitPrice())
	assert.Equal(t, snapshot.Revision(), paymentConfig.Revision())

	// The funding barrier values ride in the same generation.
	modeValue, ok := snapshot.Value(setting.PaymentOptionUserFundingMode)
	require.True(t, ok)
	mode, ok := modeValue.StringValue()
	require.True(t, ok)
	assert.Equal(t, string(operation_setting.UserFundingModeEnabled), mode)
	epochValue, ok := snapshot.Value(setting.PaymentOptionUserFundingEpoch)
	require.True(t, ok)
	epoch, ok := epochValue.IntegerValue()
	require.True(t, ok)
	assert.Equal(t, int64(state.Epoch), epoch)
	assert.Greater(t, state.Epoch, before.Epoch)

	// The legacy variable remains as a transition write target.
	assert.Equal(t, "sk_test_n2b_generation", setting.StripeApiSecret)
}

func TestPaymentFundingBulkAbortsWholeGroupWhenPublishFails(t *testing.T) {
	db, _ := userFundingPolicyTestDB(t, operation_setting.UserFundingModeDisabled)
	capturePaymentRuntimeKeys(t,
		setting.PaymentOptionCreemAPIKey,
		setting.PaymentOptionStripeAPISecret,
		setting.PaymentOptionStripeWebhookSecret,
	)
	seedComplianceOptions(t, db)
	previousRevision := setting.CurrentPaymentRuntime().Revision()
	previousOptionMap := map[string]string{}
	for _, key := range []string{"CreemApiKey", "StripeApiSecret", "StripeWebhookSecret"} {
		common.OptionMapRWMutex.RLock()
		previousOptionMap[key] = common.OptionMap[key]
		common.OptionMapRWMutex.RUnlock()
	}

	injected := errors.New("injected payment publish failure")
	previousPublish := paymentFundingOptionPublish
	paymentFundingOptionPublish = func(key string, value string) error {
		return injected
	}
	t.Cleanup(func() { paymentFundingOptionPublish = previousPublish })

	_, err := UpdatePaymentFundingOptionsBulk(map[string]string{
		string(setting.PaymentOptionCreemAPIKey):         "creem_n2b_abort",
		string(setting.PaymentOptionStripeAPISecret):     "sk_test_n2b_abort",
		string(setting.PaymentOptionStripeWebhookSecret): "whsec_n2b_abort",
	})
	require.ErrorIs(t, err, injected)

	// The whole group aborted: the runtime still serves the previous
	// generation and none of the saved keys became observable.
	snapshot := setting.CurrentPaymentRuntime()
	assert.Equal(t, previousRevision, snapshot.Revision())
	paymentConfig := setting.CapturePaymentConfig()
	assert.NotEqual(t, "creem_n2b_abort", paymentConfig.CreemApiKey())
	assert.NotEqual(t, "sk_test_n2b_abort", paymentConfig.StripeApiSecret())
	assert.NotEqual(t, "whsec_n2b_abort", paymentConfig.StripeWebhookSecret())

	// OptionMap keeps the existing semantics: the failed key's own write was
	// rolled back, and no later key was published at all.
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	for key, previous := range previousOptionMap {
		assert.Equal(t, previous, common.OptionMap[key], "OptionMap entry %s must not carry the aborted value", key)
	}
}

func TestPaymentFundingBulkRejectsRuntimeInvalidValueBeforeCommit(t *testing.T) {
	db, _ := userFundingPolicyTestDB(t, operation_setting.UserFundingModeDisabled)
	capturePaymentRuntimeKeys(t, setting.PaymentOptionStripeUnitPrice)
	previousRevision := setting.CurrentPaymentRuntime().Revision()

	_, err := UpdatePaymentFundingOptionsBulk(map[string]string{
		string(setting.PaymentOptionStripeUnitPrice): "-5",
	})
	require.Error(t, err)

	// The invalid value never reached the database, OptionMap, or the runtime.
	var count int64
	require.NoError(t, db.Model(&Option{}).Where("key = ?", string(setting.PaymentOptionStripeUnitPrice)).Count(&count).Error)
	assert.Zero(t, count)
	assert.Equal(t, previousRevision, setting.CurrentPaymentRuntime().Revision())
	common.OptionMapRWMutex.RLock()
	_, present := common.OptionMap[string(setting.PaymentOptionStripeUnitPrice)]
	common.OptionMapRWMutex.RUnlock()
	assert.False(t, present)
}

func TestUpdateOptionMapBridgesSinglePaymentKey(t *testing.T) {
	_, _ = userFundingPolicyTestDB(t, operation_setting.UserFundingModeDisabled)
	capturePaymentRuntimeKeys(t, setting.PaymentOptionWaffoMerchantID)
	previousMerchantID := setting.WaffoMerchantId
	t.Cleanup(func() { setting.WaffoMerchantId = previousMerchantID })

	require.NoError(t, updateOptionMap("WaffoMerchantId", "merchant_n2b_single"))
	assert.Equal(t, "merchant_n2b_single", setting.CapturePaymentConfig().WaffoMerchantId())

	// Clearing the value removes the key from the generation; the accessor
	// then falls back to the legacy variable, which the legacy publish also
	// updated.
	require.NoError(t, updateOptionMap("WaffoMerchantId", ""))
	_, present := setting.CurrentPaymentRuntime().Value(setting.PaymentOptionWaffoMerchantID)
	assert.False(t, present)
	assert.Equal(t, "", setting.CapturePaymentConfig().WaffoMerchantId())
}

func TestLoadOptionsBridgesPersistedPaymentKeysIntoRuntime(t *testing.T) {
	db, _ := userFundingPolicyTestDB(t, operation_setting.UserFundingModeDisabled)
	capturePaymentRuntimeKeys(t, setting.PaymentOptionEpayID, setting.PaymentOptionStripePriceID)
	previousEpayID, previousPriceID := operation_setting.EpayId, setting.StripePriceId
	t.Cleanup(func() {
		operation_setting.EpayId = previousEpayID
		setting.StripePriceId = previousPriceID
	})
	require.NoError(t, db.Create(&Option{Key: "EpayId", Value: "epay_n2b_loaded"}).Error)
	require.NoError(t, db.Create(&Option{Key: "StripePriceId", Value: "price_n2b_loaded"}).Error)

	loadOptionsFromDatabase()

	paymentConfig := setting.CapturePaymentConfig()
	assert.Equal(t, "epay_n2b_loaded", paymentConfig.EpayId())
	assert.Equal(t, "price_n2b_loaded", paymentConfig.StripePriceId())
}

func TestLoadOptionsKeepsRuntimeBehindOnInvalidPersistedValue(t *testing.T) {
	db, _ := userFundingPolicyTestDB(t, operation_setting.UserFundingModeEnabled)
	capturePaymentRuntimeKeys(t, setting.PaymentOptionStripeUnitPrice)
	previousUnitPrice := setting.StripeUnitPrice
	t.Cleanup(func() { setting.StripeUnitPrice = previousUnitPrice })
	require.NoError(t, db.Create(&Option{Key: "StripeUnitPrice", Value: "-5"}).Error)

	loadOptionsFromDatabase()

	// The runtime refused the invalid value: the key stays absent from the
	// generation and funding fails closed, matching the existing payment
	// funding load-failure behavior.
	_, present := setting.CurrentPaymentRuntime().Value(setting.PaymentOptionStripeUnitPrice)
	assert.False(t, present)
	local := operation_setting.GetUserFundingSetting()
	assert.Equal(t, operation_setting.UserFundingModeDisabled, local.Mode)
}
