package service

import (
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/system_setting"
)

// GetCallbackAddress captures a fresh payment runtime snapshot. Handlers that
// already captured one must use GetCallbackAddressFromPaymentConfig so the
// callback address comes from the same configuration generation as the rest
// of the request.
func GetCallbackAddress() string {
	return GetCallbackAddressFromPaymentConfig(setting.CapturePaymentConfig())
}

// GetCallbackAddressFromPaymentConfig resolves the callback address from the
// supplied payment runtime snapshot.
func GetCallbackAddressFromPaymentConfig(paymentConfig setting.PaymentConfig) string {
	if paymentConfig.CustomCallbackAddress() == "" {
		return system_setting.GetServerAddress()
	}
	return paymentConfig.CustomCallbackAddress()
}
