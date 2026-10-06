package service

import (
	"testing"

	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// publishCallbackAddressForTest publishes one canonical payment option into
// the payment runtime and restores the previous generation content on
// cleanup.
func publishCallbackAddressForTest(t *testing.T, key, value string) {
	t.Helper()
	optionKey := setting.PaymentOptionKey(key)
	canonical := setting.CurrentPaymentRuntime().CanonicalOptions()
	previous, existed := canonical[key]
	mutation, err := setting.PaymentMutationFromCanonicalOption(optionKey, value)
	require.NoError(t, err)
	_, err = setting.PublishPaymentCanonicalMutation(mutation)
	require.NoError(t, err)
	t.Cleanup(func() {
		restore := setting.PaymentMutation{Key: optionKey, Action: setting.PaymentMutationClear}
		if existed {
			restore, err = setting.PaymentMutationFromCanonicalOption(optionKey, previous)
			require.NoError(t, err)
		}
		_, err := setting.PublishPaymentCanonicalMutation(restore)
		require.NoError(t, err)
	})
}

func TestGetCallbackAddressFromPaymentConfigUsesSnapshot(t *testing.T) {
	previousServerAddress := system_setting.GetServerAddress()
	previousCustom := operation_setting.CustomCallbackAddress
	t.Cleanup(func() {
		system_setting.SetServerAddress(previousServerAddress)
		operation_setting.CustomCallbackAddress = previousCustom
	})
	system_setting.SetServerAddress("https://server-n2b.example.test")
	operation_setting.CustomCallbackAddress = "https://legacy-stale.example.test"
	publishCallbackAddressForTest(t, "CustomCallbackAddress", "https://callback-n2b.example.test")

	pinned := setting.CapturePaymentConfig()
	assert.Equal(t, "https://callback-n2b.example.test", GetCallbackAddressFromPaymentConfig(pinned))
	assert.Equal(t, "https://callback-n2b.example.test", GetCallbackAddress())

	// A mid-request rotation must not leak into the pinned configuration.
	publishCallbackAddressForTest(t, "CustomCallbackAddress", "https://callback-n2b-v2.example.test")
	assert.Equal(t, "https://callback-n2b.example.test", GetCallbackAddressFromPaymentConfig(pinned))
	assert.Equal(t, "https://callback-n2b-v2.example.test", GetCallbackAddressFromPaymentConfig(setting.CapturePaymentConfig()))
}

func TestGetCallbackAddressFromPaymentConfigFallsBackToServerAddress(t *testing.T) {
	previousServerAddress := system_setting.GetServerAddress()
	previousCustom := operation_setting.CustomCallbackAddress
	t.Cleanup(func() {
		system_setting.SetServerAddress(previousServerAddress)
		operation_setting.CustomCallbackAddress = previousCustom
	})
	system_setting.SetServerAddress("https://server-n2b-fallback.example.test")
	operation_setting.CustomCallbackAddress = ""
	publishCallbackAddressForTest(t, "CustomCallbackAddress", "")

	assert.Equal(t, "https://server-n2b-fallback.example.test",
		GetCallbackAddressFromPaymentConfig(setting.CapturePaymentConfig()))
}
