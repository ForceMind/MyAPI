package service

import (
	"testing"

	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/system_setting"
	"github.com/stretchr/testify/assert"
)

func TestGetCallbackAddressPrefersCustomAddressAndPreservesRawDefault(t *testing.T) {
	previousServerAddress := system_setting.GetServerAddress()
	previousCustomCallbackAddress := operation_setting.CustomCallbackAddress
	t.Cleanup(func() {
		system_setting.SetServerAddress(previousServerAddress)
		operation_setting.CustomCallbackAddress = previousCustomCallbackAddress
	})

	system_setting.SetServerAddress("https://dashboard.example.test/")
	operation_setting.CustomCallbackAddress = ""
	assert.Equal(t, "https://dashboard.example.test/", GetCallbackAddress())

	operation_setting.CustomCallbackAddress = "https://callback.example.test/custom"
	assert.Equal(t, "https://callback.example.test/custom", GetCallbackAddress())

	operation_setting.CustomCallbackAddress = ""
	system_setting.SetServerAddress("")
	assert.Empty(t, GetCallbackAddress())
}

func TestPaymentReturnURLTrimsOnlyTrailingSlashesFromDefaultAddress(t *testing.T) {
	previousServerAddress := system_setting.GetServerAddress()
	t.Cleanup(func() { system_setting.SetServerAddress(previousServerAddress) })

	testCases := []struct {
		name          string
		serverAddress string
		suffix        string
		want          string
	}{
		{
			name:          "trailing slash",
			serverAddress: "https://dashboard.example.test/",
			suffix:        "/wallet",
			want:          "https://dashboard.example.test/wallet",
		},
		{
			name:          "empty address",
			serverAddress: "",
			suffix:        "/wallet",
			want:          "/wallet",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			system_setting.SetServerAddress(testCase.serverAddress)
			assert.Equal(t, testCase.want, PaymentReturnURL(testCase.suffix))
		})
	}
}
