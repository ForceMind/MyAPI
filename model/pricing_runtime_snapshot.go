package model

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/billing_setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
)

// pricingRuntimeMu keeps one relay's mode, expression, and ratio reads on the
// same side of an option-bulk publication. It does not serialize other
// processes or replace the request's existing settlement PriceData snapshot.
var pricingRuntimeMu sync.RWMutex
var pricingRuntimeFault atomic.Bool

// Every persisted option writer already shares optionMutationLock. Pair its
// existing ordering mutex with one runtime reader barrier, including typed,
// funding and remediation writes; publishers must not reacquire this barrier.
type pricingOptionMutationLock struct{ mutation sync.Mutex }

func (l *pricingOptionMutationLock) Lock() {
	l.mutation.Lock()
	pricingRuntimeMu.Lock()
}

func (l *pricingOptionMutationLock) Unlock() {
	pricingRuntimeMu.Unlock()
	l.mutation.Unlock()
}

// CaptureQuotaUnitForOrder freezes the unit before a new order is created.
// Controllers must pass this same value through quote, validation and Insert.
func CaptureQuotaUnitForOrder() (string, error) {
	pricingRuntimeMu.RLock()
	defer pricingRuntimeMu.RUnlock()
	if pricingRuntimeFault.Load() {
		return "", ErrPricingRuntimeUnavailable
	}
	common.OptionMapRWMutex.RLock()
	unit := strconv.FormatFloat(common.QuotaPerUnit, 'g', -1, 64)
	common.OptionMapRWMutex.RUnlock()
	if err := validateOptionValue("QuotaPerUnit", unit); err != nil {
		return "", ErrPricingRuntimeUnavailable
	}
	return unit, nil
}

var ErrPricingRuntimeUnavailable = errors.New("pricing runtime publication is incomplete")

// AcquirePricingRuntimeRead is held only while building a quote, never during
// a merchant/upstream request or database settlement. Readers run concurrently.
func AcquirePricingRuntimeRead() (func(), error) {
	pricingRuntimeMu.RLock()
	if pricingRuntimeFault.Load() {
		pricingRuntimeMu.RUnlock()
		return nil, ErrPricingRuntimeUnavailable
	}
	return pricingRuntimeMu.RUnlock, nil
}

// PricingRuntimeReady reports whether this process has a complete local price
// generation. A successful full options reload clears a publication fault;
// an unrelated single-key write must not clear it.
func PricingRuntimeReady() bool {
	return !pricingRuntimeFault.Load()
}

func markPricingRuntimeUnavailable() {
	pricingRuntimeFault.Store(true)
}

func clearPricingRuntimeUnavailable() {
	pricingRuntimeFault.Store(false)
}

// modelPricingOptionPublish is the legacy per-field publisher. Keeping the
// boundary explicit lets failure-path tests pause a batch between fields.
var modelPricingOptionPublish = updateOptionMap

type ModelPricingSnapshot struct {
	Unavailable          bool
	Billing              billing_setting.BillingSnapshot
	ModelPrice           float64
	HasModelPrice        bool
	ModelRatio           float64
	HasModelRatio        bool
	MatchingModelName    string
	CompletionRatio      float64
	CacheRatio           float64
	CacheCreationRatio   float64
	ImageRatio           float64
	AudioRatio           float64
	AudioCompletionRatio float64
}

// CaptureModelPricingSnapshot pairs the billing-mode generation with all
// ratio fields used to estimate and later settle one model request.
func CaptureModelPricingSnapshot(modelName string) ModelPricingSnapshot {
	pricingRuntimeMu.RLock()
	defer pricingRuntimeMu.RUnlock()

	modelPrice, hasModelPrice := ratio_setting.GetModelPrice(modelName, false)
	modelRatio, hasModelRatio, matchingName := ratio_setting.GetModelRatio(modelName)
	cacheRatio, _ := ratio_setting.GetCacheRatio(modelName)
	cacheCreationRatio, _ := ratio_setting.GetCreateCacheRatio(modelName)
	imageRatio, _ := ratio_setting.GetImageRatio(modelName)
	return ModelPricingSnapshot{
		Unavailable:          pricingRuntimeFault.Load(),
		Billing:              billing_setting.GetBillingSnapshot(),
		ModelPrice:           modelPrice,
		HasModelPrice:        hasModelPrice,
		ModelRatio:           modelRatio,
		HasModelRatio:        hasModelRatio,
		MatchingModelName:    matchingName,
		CompletionRatio:      ratio_setting.GetCompletionRatio(modelName),
		CacheRatio:           cacheRatio,
		CacheCreationRatio:   cacheCreationRatio,
		ImageRatio:           imageRatio,
		AudioRatio:           ratio_setting.GetAudioRatio(modelName),
		AudioCompletionRatio: ratio_setting.GetAudioCompletionRatio(modelName),
	}
}

// CaptureOptionMapForPricingRead keeps the administrator's old-value CAS view
// on one side of a local pricing batch publication.
func CaptureOptionMapForPricingRead() (map[string]string, error) {
	pricingRuntimeMu.RLock()
	defer pricingRuntimeMu.RUnlock()
	if pricingRuntimeFault.Load() {
		return nil, ErrPricingRuntimeUnavailable
	}
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	values := make(map[string]string, len(common.OptionMap))
	for key, value := range common.OptionMap {
		values[key] = value
	}
	return values, nil
}

func CapturePricingSyncData() (map[string]any, error) {
	pricingRuntimeMu.RLock()
	defer pricingRuntimeMu.RUnlock()
	if pricingRuntimeFault.Load() {
		return nil, ErrPricingRuntimeUnavailable
	}
	data := billing_setting.GetPricingSyncData(map[string]any(ratio_setting.GetExposedData()))
	data["image_ratio"] = ratio_setting.GetImageRatioCopy()
	data["audio_ratio"] = ratio_setting.GetAudioRatioCopy()
	data["audio_completion_ratio"] = ratio_setting.GetAudioCompletionRatioCopy()
	return data, nil
}

func CaptureExposedPricingData() (map[string]any, bool, error) {
	pricingRuntimeMu.RLock()
	defer pricingRuntimeMu.RUnlock()
	if pricingRuntimeFault.Load() {
		return nil, false, ErrPricingRuntimeUnavailable
	}
	if !ratio_setting.IsExposeRatioEnabled() {
		return nil, false, nil
	}
	return map[string]any(ratio_setting.GetExposedData()), true, nil
}

func CapturePublicPricing() ([]Pricing, []PricingVendor, error) {
	pricingRuntimeMu.RLock()
	defer pricingRuntimeMu.RUnlock()
	if pricingRuntimeFault.Load() {
		return nil, nil, ErrPricingRuntimeUnavailable
	}
	return GetPricing(), GetVendors(), nil
}

func isModelPricingOptionKey(key string) bool {
	switch key {
	case "ModelRatio", "ModelPrice", "CompletionRatio", "CacheRatio", "CreateCacheRatio", "ImageRatio", "AudioRatio", "AudioCompletionRatio", "ExposeRatioEnabled", "billing_setting.billing_mode", "billing_setting.billing_expr":
		return true
	default:
		return false
	}
}

// isPricingRuntimeOptionKey covers the persisted price, group multiplier,
// tool surcharge, and free-model pre-consume decisions used by requests. It
// does not change which fields belong to the dedicated model-price batch.
func isPricingRuntimeOptionKey(key string) bool {
	if IsPaymentFundingOptionKey(key) || key == "TopupGroupRatio" || key == "DisplayInCurrencyEnabled" || key == "general_setting.quota_display_type" || key == "payment_setting.amount_discount" {
		return true
	}
	if isModelPricingOptionKey(key) || key == operation_setting.ToolPriceOptionKey || key == "quota_setting.enable_free_model_pre_consume" || key == "QuotaPerUnit" {
		return true
	}
	_, groupRatio := groupRatioOptionPairForKey(key)
	return groupRatio
}

func validateModelPricingOptions(values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	billingValues := make(map[string]string, 2)
	for key, value := range values {
		if err := validateOptionValue(key, value); err != nil {
			return err
		}
		switch key {
		case "billing_setting.billing_mode":
			billingValues["billing_mode"] = value
		case "billing_setting.billing_expr":
			billingValues["billing_expr"] = value
		}
	}
	if len(billingValues) > 0 {
		registered := config.GlobalConfig.Get("billing_setting")
		if registered == nil {
			return errors.New("billing setting is not registered")
		}
		if err := config.ValidateConfigFromMap(registered, billingValues); err != nil {
			return err
		}
	}
	return nil
}

// publishModelPricingOptions holds the reader barrier over every field from
// one committed option batch via its caller's optionMutationLock. An unexpected publication error remains a
// reported post-commit failure, not a false rollback claim.
func publishModelPricingOptions(values map[string]string) error {
	if err := validateModelPricingOptions(values); err != nil {
		markPricingRuntimeUnavailable()
		return err
	}
	exposure, changesExposure := values["ExposeRatioEnabled"]
	if changesExposure {
		enabled, _ := strconv.ParseBool(strings.TrimSpace(exposure)) // validated above
		if !enabled {
			// Hide the public ratio view before changing any map. If a later
			// publication fails, prices may be partial but remain unexposed.
			if err := modelPricingOptionPublish("ExposeRatioEnabled", exposure); err != nil {
				pricingRuntimeFault.Store(true)
				return err
			}
		}
	}
	for _, key := range []string{
		"billing_setting.billing_expr", "billing_setting.billing_mode",
		"ModelPrice", "ModelRatio", "CompletionRatio", "CacheRatio",
		"CreateCacheRatio", "ImageRatio", "AudioRatio", "AudioCompletionRatio",
	} {
		if value, ok := values[key]; ok {
			if err := modelPricingOptionPublish(key, value); err != nil {
				pricingRuntimeFault.Store(true)
				return err
			}
		}
	}
	if changesExposure {
		enabled, _ := strconv.ParseBool(strings.TrimSpace(exposure)) // validated above
		if enabled {
			// Reveal only after the pricing maps have all been published.
			if err := modelPricingOptionPublish("ExposeRatioEnabled", exposure); err != nil {
				pricingRuntimeFault.Store(true)
				return err
			}
		}
	}
	return nil
}
