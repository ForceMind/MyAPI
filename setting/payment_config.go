package setting

import (
	"github.com/ForceMind/MyAPI/setting/operation_setting"
)

// PaymentConfig is the authoritative per-request view of the payment runtime.
// It pins exactly one PaymentRuntime generation, so every field a request
// reads comes from the same configuration generation and a concurrent save
// can never mix generations mid-request.
//
// The legacy package-level variables remain as a transition write target and
// as the fallback for keys the runtime generation does not contain (never
// persisted, or rejected by runtime validation during option load). Reads
// must go through a captured PaymentConfig, not through the bare variables.
type PaymentConfig struct {
	snapshot PaymentSnapshot
}

// CapturePaymentConfig pins the current payment runtime generation. The
// returned value is detached and safe to hold for the lifetime of a request.
func CapturePaymentConfig() PaymentConfig {
	return PaymentConfig{snapshot: CurrentPaymentRuntime()}
}

// Revision reports the pinned runtime generation revision (0 when the
// runtime has not published any generation yet).
func (config PaymentConfig) Revision() uint64 {
	return config.snapshot.Revision()
}

func (config PaymentConfig) stringValue(key PaymentOptionKey, fallback string) string {
	if value, ok := config.snapshot.Value(key); ok {
		if text, ok := value.StringValue(); ok {
			return text
		}
	}
	return fallback
}

// jsonStringValue renders a JSON-kind runtime value back to its canonical
// string form. Callers that parse the string as JSON observe equivalent
// content to the stored option value.
func (config PaymentConfig) jsonStringValue(key PaymentOptionKey, fallback string) string {
	if value, ok := config.snapshot.Value(key); ok {
		if encoded, ok := value.JSONValue(); ok {
			return string(encoded)
		}
	}
	return fallback
}

func (config PaymentConfig) numberValue(key PaymentOptionKey, fallback float64) float64 {
	if value, ok := config.snapshot.Value(key); ok {
		if number, ok := value.NumberValue(); ok {
			return number
		}
	}
	return fallback
}

func (config PaymentConfig) integerValue(key PaymentOptionKey, fallback int) int {
	if value, ok := config.snapshot.Value(key); ok {
		if integer, ok := value.IntegerValue(); ok {
			return int(integer)
		}
	}
	return fallback
}

func (config PaymentConfig) booleanValue(key PaymentOptionKey, fallback bool) bool {
	if value, ok := config.snapshot.Value(key); ok {
		if boolean, ok := value.BooleanValue(); ok {
			return boolean
		}
	}
	return fallback
}

// Epay / general top-up settings (legacy variables live in operation_setting).

func (config PaymentConfig) PayAddress() string {
	return config.stringValue(PaymentOptionPayAddress, operation_setting.PayAddress)
}

func (config PaymentConfig) CustomCallbackAddress() string {
	return config.stringValue(PaymentOptionCustomCallbackAddress, operation_setting.CustomCallbackAddress)
}

func (config PaymentConfig) EpayId() string {
	return config.stringValue(PaymentOptionEpayID, operation_setting.EpayId)
}

func (config PaymentConfig) EpayKey() string {
	return config.stringValue(PaymentOptionEpayKey, operation_setting.EpayKey)
}

func (config PaymentConfig) Price() float64 {
	return config.numberValue(PaymentOptionPrice, operation_setting.Price)
}

func (config PaymentConfig) USDExchangeRate() float64 {
	return config.numberValue(PaymentOptionUSDExchangeRate, operation_setting.USDExchangeRate)
}

func (config PaymentConfig) MinTopUp() int {
	return config.integerValue(PaymentOptionMinTopUp, operation_setting.MinTopUp)
}

// Stripe settings.

func (config PaymentConfig) StripeApiSecret() string {
	return config.stringValue(PaymentOptionStripeAPISecret, StripeApiSecret)
}

func (config PaymentConfig) StripeWebhookSecret() string {
	return config.stringValue(PaymentOptionStripeWebhookSecret, StripeWebhookSecret)
}

func (config PaymentConfig) StripePriceId() string {
	return config.stringValue(PaymentOptionStripePriceID, StripePriceId)
}

func (config PaymentConfig) StripeUnitPrice() float64 {
	return config.numberValue(PaymentOptionStripeUnitPrice, StripeUnitPrice)
}

func (config PaymentConfig) StripeMinTopUp() int {
	return config.integerValue(PaymentOptionStripeMinTopUp, StripeMinTopUp)
}

func (config PaymentConfig) StripePromotionCodesEnabled() bool {
	return config.booleanValue(PaymentOptionStripePromotionCodesEnabled, StripePromotionCodesEnabled)
}

// Creem settings.

func (config PaymentConfig) CreemApiKey() string {
	return config.stringValue(PaymentOptionCreemAPIKey, CreemApiKey)
}

func (config PaymentConfig) CreemProducts() string {
	return config.jsonStringValue(PaymentOptionCreemProducts, CreemProducts)
}

func (config PaymentConfig) CreemTestMode() bool {
	return config.booleanValue(PaymentOptionCreemTestMode, CreemTestMode)
}

func (config PaymentConfig) CreemWebhookSecret() string {
	return config.stringValue(PaymentOptionCreemWebhookSecret, CreemWebhookSecret)
}

// Waffo settings.

func (config PaymentConfig) WaffoEnabled() bool {
	return config.booleanValue(PaymentOptionWaffoEnabled, WaffoEnabled)
}

func (config PaymentConfig) WaffoApiKey() string {
	return config.stringValue(PaymentOptionWaffoAPIKey, WaffoApiKey)
}

func (config PaymentConfig) WaffoPrivateKey() string {
	return config.stringValue(PaymentOptionWaffoPrivateKey, WaffoPrivateKey)
}

func (config PaymentConfig) WaffoPublicCert() string {
	return config.stringValue(PaymentOptionWaffoPublicCert, WaffoPublicCert)
}

func (config PaymentConfig) WaffoSandboxPublicCert() string {
	return config.stringValue(PaymentOptionWaffoSandboxPublicCert, WaffoSandboxPublicCert)
}

func (config PaymentConfig) WaffoSandboxApiKey() string {
	return config.stringValue(PaymentOptionWaffoSandboxAPIKey, WaffoSandboxApiKey)
}

func (config PaymentConfig) WaffoSandboxPrivateKey() string {
	return config.stringValue(PaymentOptionWaffoSandboxPrivateKey, WaffoSandboxPrivateKey)
}

func (config PaymentConfig) WaffoSandbox() bool {
	return config.booleanValue(PaymentOptionWaffoSandbox, WaffoSandbox)
}

func (config PaymentConfig) WaffoMerchantId() string {
	return config.stringValue(PaymentOptionWaffoMerchantID, WaffoMerchantId)
}

func (config PaymentConfig) WaffoNotifyUrl() string {
	return config.stringValue(PaymentOptionWaffoNotifyURL, WaffoNotifyUrl)
}

func (config PaymentConfig) WaffoReturnUrl() string {
	return config.stringValue(PaymentOptionWaffoReturnURL, WaffoReturnUrl)
}

func (config PaymentConfig) WaffoSubscriptionReturnUrl() string {
	return config.stringValue(PaymentOptionWaffoSubscriptionReturnURL, WaffoSubscriptionReturnUrl)
}

func (config PaymentConfig) WaffoCurrency() string {
	return config.stringValue(PaymentOptionWaffoCurrency, WaffoCurrency)
}

func (config PaymentConfig) WaffoUnitPrice() float64 {
	return config.numberValue(PaymentOptionWaffoUnitPrice, WaffoUnitPrice)
}

func (config PaymentConfig) WaffoMinTopUp() int {
	return config.integerValue(PaymentOptionWaffoMinTopUp, WaffoMinTopUp)
}

// Waffo Pancake settings.

func (config PaymentConfig) WaffoPancakeMerchantID() string {
	return config.stringValue(PaymentOptionWaffoPancakeMerchantID, WaffoPancakeMerchantID)
}

func (config PaymentConfig) WaffoPancakePrivateKey() string {
	return config.stringValue(PaymentOptionWaffoPancakePrivateKey, WaffoPancakePrivateKey)
}

func (config PaymentConfig) WaffoPancakeReturnURL() string {
	return config.stringValue(PaymentOptionWaffoPancakeReturnURL, WaffoPancakeReturnURL)
}

func (config PaymentConfig) WaffoPancakeUnitPrice() float64 {
	return config.numberValue(PaymentOptionWaffoPancakeUnitPrice, WaffoPancakeUnitPrice)
}

func (config PaymentConfig) WaffoPancakeMinTopUp() int {
	return config.integerValue(PaymentOptionWaffoPancakeMinTopUp, WaffoPancakeMinTopUp)
}

func (config PaymentConfig) WaffoPancakeStoreID() string {
	return config.stringValue(PaymentOptionWaffoPancakeStoreID, WaffoPancakeStoreID)
}

func (config PaymentConfig) WaffoPancakeProductID() string {
	return config.stringValue(PaymentOptionWaffoPancakeProductID, WaffoPancakeProductID)
}
