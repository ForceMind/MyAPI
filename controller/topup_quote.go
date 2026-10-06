package controller

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// Effective values, including legacy fallbacks, are read exactly once under
// the publication barrier. No reader lock is held during DB or SDK work.
type epayQuoteContext struct {
	unit            float64
	unitSnapshot    string
	displayType     string
	price           float64
	discount        float64
	minimum         int64
	groupRatios     map[string]float64
	methodAllowed   bool
	client          *epay.Client
	callbackAddress string
	returnURL       string
}

type stripeOrderQuote struct {
	unitSnapshot          string
	minimum               int64
	chargedMoney          float64
	creditedQuota         decimal.Decimal
	apiSecret, priceID    string
	promotionCodes        bool
	successURL, cancelURL string
}

type pancakeOrderQuote struct {
	unitSnapshot                               string
	unit                                       float64
	displayType                                string
	amount                                     int64
	money                                      float64
	minimum                                    int64
	merchantID, privateKey, productID, storeID string
}

func capturePancakeOrderQuote(amount int64, group string) (pancakeOrderQuote, error) {
	release, err := model.AcquirePricingRuntimeRead()
	if err != nil {
		return pancakeOrderQuote{}, err
	}
	defer release()
	common.OptionMapRWMutex.RLock()
	unit := common.QuotaPerUnit
	common.OptionMapRWMutex.RUnlock()
	if math.IsNaN(unit) || math.IsInf(unit, 0) || unit < 1 || unit > float64(common.MaxQuota/10) {
		return pancakeOrderQuote{}, model.ErrPricingRuntimeUnavailable
	}
	config := setting.CapturePaymentConfig()
	if !isWaffoPancakeTopUpEnabledFromPaymentConfig(config) {
		return pancakeOrderQuote{}, model.ErrPricingRuntimeUnavailable
	}
	quote := pancakeOrderQuote{unit: unit, unitSnapshot: strconv.FormatFloat(unit, 'g', -1, 64), displayType: operation_setting.GetQuotaDisplayType(), minimum: int64(config.WaffoPancakeMinTopUp()),
		merchantID: config.WaffoPancakeMerchantID(), privateKey: config.WaffoPancakePrivateKey(), productID: config.WaffoPancakeProductID(), storeID: config.WaffoPancakeStoreID()}
	if strings.TrimSpace(quote.storeID) == "" || len(quote.storeID) > 128 || len(quote.productID) > 128 {
		return pancakeOrderQuote{}, model.ErrPricingRuntimeUnavailable
	}
	quote.money = getWaffoPancakePayMoney(amount, group, config)
	quote.amount = normalizeWaffoPancakeTopUpAmount(amount)
	if math.IsNaN(quote.money) || math.IsInf(quote.money, 0) {
		return pancakeOrderQuote{}, model.ErrPricingRuntimeUnavailable
	}
	return quote, nil
}

func captureStripeOrderQuote(amount int64, user model.User, successURL, cancelURL string) (stripeOrderQuote, error) {
	release, err := model.AcquirePricingRuntimeRead()
	if err != nil {
		return stripeOrderQuote{}, err
	}
	defer release()
	common.OptionMapRWMutex.RLock()
	unit := common.QuotaPerUnit
	common.OptionMapRWMutex.RUnlock()
	if math.IsNaN(unit) || math.IsInf(unit, 0) || unit < 1 || unit > float64(common.MaxQuota/10) {
		return stripeOrderQuote{}, model.ErrPricingRuntimeUnavailable
	}
	config := setting.CapturePaymentConfig()
	money := GetChargedAmount(float64(amount), user)
	if math.IsNaN(money) || math.IsInf(money, 0) || money <= 0 {
		return stripeOrderQuote{}, model.ErrInvalidTopUpQuota
	}
	if successURL == "" {
		successURL = paymentReturnPath("/usage-logs")
	}
	if cancelURL == "" {
		cancelURL = paymentReturnPath("/wallet")
	}
	minimum := int64(config.StripeMinTopUp())
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		value, err := common.QuotaFromDecimalStrict(decimal.NewFromInt(minimum).Mul(decimal.NewFromFloat(unit)))
		if err != nil {
			return stripeOrderQuote{}, model.ErrPricingRuntimeUnavailable
		}
		minimum = int64(value)
	}
	return stripeOrderQuote{unitSnapshot: strconv.FormatFloat(unit, 'g', -1, 64), minimum: minimum, chargedMoney: money,
		creditedQuota: decimal.NewFromFloat(money).Mul(decimal.NewFromFloat(unit)), apiSecret: config.StripeApiSecret(), priceID: config.StripePriceId(),
		promotionCodes: config.StripePromotionCodesEnabled(), successURL: successURL, cancelURL: cancelURL}, nil
}

var epayPurchase = (*epay.Client).Purchase

func respondTopUpPricingUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "code": "PRICING_RUNTIME_UNAVAILABLE", "message": common.TranslateMessage(c, i18n.MsgPaymentPricingUnavailable)})
}

func rejectInvalidTopUpQuotaForSnapshot(c *gin.Context, userID int, amount int64, unit float64, displayType string) bool {
	quota, err := validateTopUpQuotaForSnapshot(amount, unit, displayType)
	if err == nil {
		err = model.ValidateTopUpQuotaCapacity(userID, quota)
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return true
	}
	return false
}

func captureEpayQuoteContext(amount int64, paymentMethod string) (epayQuoteContext, error) {
	release, err := model.AcquirePricingRuntimeRead()
	if err != nil {
		return epayQuoteContext{}, err
	}
	defer release()
	common.OptionMapRWMutex.RLock()
	unit := common.QuotaPerUnit
	common.OptionMapRWMutex.RUnlock()
	if math.IsNaN(unit) || math.IsInf(unit, 0) || unit < 1 || unit > float64(common.MaxQuota/10) {
		return epayQuoteContext{}, model.ErrPricingRuntimeUnavailable
	}
	config := setting.CapturePaymentConfig()
	quote := epayQuoteContext{unit: unit, unitSnapshot: strconv.FormatFloat(unit, 'g', -1, 64), displayType: operation_setting.GetQuotaDisplayType(), price: config.Price(), discount: 1,
		minimum: int64(config.MinTopUp()), methodAllowed: operation_setting.ContainsPayMethod(paymentMethod), client: GetEpayClient(config),
		callbackAddress: service.GetCallbackAddressFromPaymentConfig(config), returnURL: paymentReturnPath("/usage-logs")}
	if err := common.UnmarshalJsonStr(common.TopupGroupRatio2JSONString(), &quote.groupRatios); err != nil {
		return epayQuoteContext{}, err
	}
	if int64(int(amount)) == amount {
		if discount, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
			if math.IsNaN(discount) || math.IsInf(discount, 0) {
				return epayQuoteContext{}, model.ErrPricingRuntimeUnavailable
			}
			if discount > 0 {
				quote.discount = discount
			}
		}
	}
	if math.IsNaN(quote.price) || math.IsInf(quote.price, 0) || quote.price < 0 || quote.minimum < 0 {
		return epayQuoteContext{}, model.ErrPricingRuntimeUnavailable
	}
	if quote.displayType == operation_setting.QuotaDisplayTypeTokens {
		minimum, err := common.QuotaFromDecimalStrict(decimal.NewFromInt(quote.minimum).Mul(decimal.NewFromFloat(unit)))
		if err != nil {
			return epayQuoteContext{}, model.ErrPricingRuntimeUnavailable
		}
		quote.minimum = int64(minimum)
	}
	return quote, nil
}

func (quote epayQuoteContext) storedAmount(amount int64) int64 {
	if quote.displayType == operation_setting.QuotaDisplayTypeTokens {
		return decimal.NewFromInt(amount).Div(decimal.NewFromFloat(quote.unit)).IntPart()
	}
	return amount
}

func (quote epayQuoteContext) payMoney(amount int64, group string) (float64, error) {
	quantity := decimal.NewFromInt(amount)
	if quote.displayType == operation_setting.QuotaDisplayTypeTokens {
		quantity = quantity.Div(decimal.NewFromFloat(quote.unit))
	}
	ratio := quote.groupRatios[group]
	if ratio == 0 {
		ratio = 1
	}
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
		return 0, model.ErrPricingRuntimeUnavailable
	}
	money := quantity.Mul(decimal.NewFromFloat(quote.price)).Mul(decimal.NewFromFloat(ratio)).Mul(decimal.NewFromFloat(quote.discount)).InexactFloat64()
	if math.IsNaN(money) || math.IsInf(money, 0) {
		return 0, model.ErrPricingRuntimeUnavailable
	}
	return money, nil
}
