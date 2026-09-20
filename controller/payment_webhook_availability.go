package controller

import (
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const userFundingEpochContextKey = "user_funding_epoch"

func UserFundingMutationGate(c *gin.Context) {
	snapshot := service.CurrentUserFundingSnapshot()
	if snapshot.Capabilities.CanTopUp {
		c.Set(userFundingEpochContextKey, snapshot.State.Epoch)
		c.Next()
		return
	}
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"success": false,
		"message": common.TranslateMessage(c, i18n.MsgPaymentComplianceRequired),
		"code":    service.UserFundingUnavailableCode,
	})
}

func userFundingEpoch(c *gin.Context) uint64 {
	if c == nil {
		return 0
	}
	epoch, _ := c.Get(userFundingEpochContextKey)
	value, _ := epoch.(uint64)
	return value
}

// Payment availability reads come in two forms:
//   - the zero-argument helpers capture a fresh payment runtime snapshot;
//   - the FromPaymentConfig variants read from a snapshot the request already
//     captured, so every payment field in one request comes from the same
//     configuration generation.
// Order and webhook handlers must capture once and use the FromPaymentConfig
// variants for every check in that request.

func isPaymentComplianceConfirmed() bool {
	return operation_setting.IsPaymentComplianceConfirmed()
}

func isStripeTopUpEnabledFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	return strings.TrimSpace(paymentConfig.StripeApiSecret()) != "" &&
		strings.TrimSpace(paymentConfig.StripeWebhookSecret()) != "" &&
		strings.TrimSpace(paymentConfig.StripePriceId()) != ""
}

func isStripeTopUpEnabled() bool {
	return isStripeTopUpEnabledFromPaymentConfig(setting.CapturePaymentConfig())
}

func isStripeWebhookConfiguredFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	return strings.TrimSpace(paymentConfig.StripeWebhookSecret()) != ""
}

func isStripeWebhookConfigured() bool {
	return isStripeWebhookConfiguredFromPaymentConfig(setting.CapturePaymentConfig())
}

func isStripeWebhookEnabledFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	// Subscription plans have their own prices; recharge catalog configuration
	// must not disable fulfillment of their already-created orders.
	return isPaymentComplianceConfirmed() && strings.TrimSpace(paymentConfig.StripeApiSecret()) != "" && isStripeWebhookConfiguredFromPaymentConfig(paymentConfig)
}

func isStripeWebhookEnabled() bool {
	return isStripeWebhookEnabledFromPaymentConfig(setting.CapturePaymentConfig())
}

func isCreemTopUpEnabledFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	products := strings.TrimSpace(paymentConfig.CreemProducts())
	return strings.TrimSpace(paymentConfig.CreemApiKey()) != "" &&
		products != "" &&
		products != "[]"
}

func isCreemTopUpEnabled() bool {
	return isCreemTopUpEnabledFromPaymentConfig(setting.CapturePaymentConfig())
}

func isCreemWebhookConfiguredFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	return strings.TrimSpace(paymentConfig.CreemWebhookSecret()) != ""
}

func isCreemWebhookConfigured() bool {
	return isCreemWebhookConfiguredFromPaymentConfig(setting.CapturePaymentConfig())
}

func isCreemWebhookEnabledFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	return isPaymentComplianceConfirmed() && strings.TrimSpace(paymentConfig.CreemApiKey()) != "" && isCreemWebhookConfiguredFromPaymentConfig(paymentConfig)
}

func isCreemWebhookEnabled() bool {
	return isCreemWebhookEnabledFromPaymentConfig(setting.CapturePaymentConfig())
}

func isWaffoTopUpEnabledFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	if !paymentConfig.WaffoEnabled() {
		return false
	}

	return isWaffoWebhookConfiguredFromPaymentConfig(paymentConfig)
}

func isWaffoTopUpEnabled() bool {
	return isWaffoTopUpEnabledFromPaymentConfig(setting.CapturePaymentConfig())
}

func isWaffoWebhookConfiguredFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	if paymentConfig.WaffoSandbox() {
		return strings.TrimSpace(paymentConfig.WaffoSandboxApiKey()) != "" &&
			strings.TrimSpace(paymentConfig.WaffoSandboxPrivateKey()) != "" &&
			strings.TrimSpace(paymentConfig.WaffoSandboxPublicCert()) != ""
	}

	return strings.TrimSpace(paymentConfig.WaffoApiKey()) != "" &&
		strings.TrimSpace(paymentConfig.WaffoPrivateKey()) != "" &&
		strings.TrimSpace(paymentConfig.WaffoPublicCert()) != ""
}

func isWaffoWebhookConfigured() bool {
	return isWaffoWebhookConfiguredFromPaymentConfig(setting.CapturePaymentConfig())
}

func isWaffoWebhookEnabledFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	return isWaffoTopUpEnabledFromPaymentConfig(paymentConfig)
}

func isWaffoWebhookEnabled() bool {
	return isWaffoWebhookEnabledFromPaymentConfig(setting.CapturePaymentConfig())
}

func isWaffoPancakeTopUpEnabledFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	// Presence-of-credentials = enabled. Webhook public keys ship inside
	// the SDK; mode (test/prod) is read from each event.
	return strings.TrimSpace(paymentConfig.WaffoPancakeMerchantID()) != "" &&
		strings.TrimSpace(paymentConfig.WaffoPancakePrivateKey()) != "" &&
		strings.TrimSpace(paymentConfig.WaffoPancakeProductID()) != ""
}

func isWaffoPancakeTopUpEnabled() bool {
	return isWaffoPancakeTopUpEnabledFromPaymentConfig(setting.CapturePaymentConfig())
}

func isWaffoPancakeWebhookConfiguredFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	return isWaffoPancakeTopUpEnabledFromPaymentConfig(paymentConfig)
}

func isWaffoPancakeWebhookConfigured() bool {
	return isWaffoPancakeWebhookConfiguredFromPaymentConfig(setting.CapturePaymentConfig())
}

func isWaffoPancakeWebhookEnabledFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	return isWaffoPancakeTopUpEnabledFromPaymentConfig(paymentConfig)
}

func isWaffoPancakeWebhookEnabled() bool {
	return isWaffoPancakeWebhookEnabledFromPaymentConfig(setting.CapturePaymentConfig())
}

func isEpayTopUpEnabledFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	return isEpayWebhookConfiguredFromPaymentConfig(paymentConfig) && len(operation_setting.GetPayMethods()) > 0
}

func isEpayTopUpEnabled() bool {
	return isEpayTopUpEnabledFromPaymentConfig(setting.CapturePaymentConfig())
}

func isEpayWebhookConfiguredFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	return strings.TrimSpace(paymentConfig.PayAddress()) != "" &&
		strings.TrimSpace(paymentConfig.EpayId()) != "" &&
		strings.TrimSpace(paymentConfig.EpayKey()) != ""
}

func isEpayWebhookConfigured() bool {
	return isEpayWebhookConfiguredFromPaymentConfig(setting.CapturePaymentConfig())
}

func isEpayWebhookEnabledFromPaymentConfig(paymentConfig setting.PaymentConfig) bool {
	return isEpayTopUpEnabledFromPaymentConfig(paymentConfig)
}

func isEpayWebhookEnabled() bool {
	return isEpayWebhookEnabledFromPaymentConfig(setting.CapturePaymentConfig())
}
