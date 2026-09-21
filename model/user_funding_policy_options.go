package model

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"gorm.io/gorm"
)

var paymentFundingOptionKeys = map[string]struct{}{
	string(setting.PaymentOptionPayAddress):                  {},
	string(setting.PaymentOptionCustomCallbackAddress):       {},
	string(setting.PaymentOptionEpayID):                      {},
	string(setting.PaymentOptionEpayKey):                     {},
	string(setting.PaymentOptionPrice):                       {},
	string(setting.PaymentOptionUSDExchangeRate):             {},
	string(setting.PaymentOptionMinTopUp):                    {},
	string(setting.PaymentOptionPayMethods):                  {},
	string(setting.PaymentOptionStripeAPISecret):             {},
	string(setting.PaymentOptionStripeWebhookSecret):         {},
	string(setting.PaymentOptionStripePriceID):               {},
	string(setting.PaymentOptionStripeUnitPrice):             {},
	string(setting.PaymentOptionStripeMinTopUp):              {},
	string(setting.PaymentOptionStripePromotionCodesEnabled): {},
	string(setting.PaymentOptionCreemAPIKey):                 {},
	string(setting.PaymentOptionCreemProducts):               {},
	string(setting.PaymentOptionCreemTestMode):               {},
	string(setting.PaymentOptionCreemWebhookSecret):          {},
	string(setting.PaymentOptionWaffoEnabled):                {},
	string(setting.PaymentOptionWaffoAPIKey):                 {},
	string(setting.PaymentOptionWaffoPrivateKey):             {},
	string(setting.PaymentOptionWaffoPublicCert):             {},
	string(setting.PaymentOptionWaffoSandboxPublicCert):      {},
	string(setting.PaymentOptionWaffoSandboxAPIKey):          {},
	string(setting.PaymentOptionWaffoSandboxPrivateKey):      {},
	string(setting.PaymentOptionWaffoSandbox):                {},
	string(setting.PaymentOptionWaffoMerchantID):             {},
	string(setting.PaymentOptionWaffoNotifyURL):              {},
	string(setting.PaymentOptionWaffoReturnURL):              {},
	string(setting.PaymentOptionWaffoSubscriptionReturnURL):  {},
	string(setting.PaymentOptionWaffoCurrency):               {},
	string(setting.PaymentOptionWaffoUnitPrice):              {},
	string(setting.PaymentOptionWaffoMinTopUp):               {},
	string(setting.PaymentOptionWaffoPayMethods):             {},
	string(setting.PaymentOptionWaffoPancakeMerchantID):      {},
	string(setting.PaymentOptionWaffoPancakePrivateKey):      {},
	string(setting.PaymentOptionWaffoPancakeReturnURL):       {},
	string(setting.PaymentOptionWaffoPancakeUnitPrice):       {},
	string(setting.PaymentOptionWaffoPancakeMinTopUp):        {},
	string(setting.PaymentOptionWaffoPancakeStoreID):         {},
	string(setting.PaymentOptionWaffoPancakeProductID):       {},
	string(setting.PaymentOptionTopupGroupRatio):             {},
	string(setting.PaymentOptionAmountOptions):               {},
	string(setting.PaymentOptionAmountDiscount):              {},
	operation_setting.UserFundingModeOptionKey:               {},
}

func IsPaymentFundingOptionKey(key string) bool {
	_, ok := paymentFundingOptionKeys[key]
	return ok
}

func normalizePaymentFundingOptions(values map[string]string) (map[string]string, error) {
	normalized := make(map[string]string, len(values))
	groupedConfigValues := make(map[string]map[string]string)
	for key, value := range values {
		if !IsPaymentFundingOptionKey(key) {
			return nil, fmt.Errorf("unsupported payment funding option %q", key)
		}
		var err error
		value, err = normalizeOptionValue(key, value)
		if err != nil {
			return nil, err
		}
		if err := validateOptionValue(key, value); err != nil {
			return nil, err
		}
		normalized[key] = value
		parts := strings.SplitN(key, ".", 2)
		if len(parts) == 2 {
			if groupedConfigValues[parts[0]] == nil {
				groupedConfigValues[parts[0]] = make(map[string]string)
			}
			groupedConfigValues[parts[0]][parts[1]] = value
		}
	}
	for name, configValues := range groupedConfigValues {
		registered := config.GlobalConfig.Get(name)
		if registered == nil {
			continue
		}
		if err := config.ValidateConfigFromMap(registered, configValues); err != nil && err != config.ErrMapConfigValidationUnsupported {
			return nil, err
		}
	}
	return normalized, nil
}

func optionValueTx(tx *gorm.DB, key string) (string, bool, error) {
	var option Option
	if err := optionKeyQuery(tx, key).First(&option).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return option.Value, true, nil
}

func fundingEnablementReadyTx(tx *gorm.DB, pending map[string]string) (bool, error) {
	read := func(key string) (string, error) {
		if value, ok := pending[key]; ok {
			return value, nil
		}
		value, _, err := optionValueTx(tx, key)
		return value, err
	}
	confirmed, err := read(string(setting.PaymentOptionComplianceConfirmed))
	if err != nil {
		return false, err
	}
	termsVersion, err := read(string(setting.PaymentOptionComplianceTermsVersion))
	if err != nil {
		return false, err
	}
	return confirmed == "true" && termsVersion == operation_setting.CurrentComplianceTermsVersion, nil
}

func persistOptionValuesTx(tx *gorm.DB, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := values[key]
		stored := Option{Key: key}
		if err := optionKeyQuery(tx, key).FirstOrCreate(&stored, Option{Key: key}).Error; err != nil {
			return err
		}
		if err := optionKeyQuery(tx.Model(&Option{}), key).Update("value", value).Error; err != nil {
			return err
		}
	}
	return nil
}

func publishCommittedUserFundingState(state UserFundingStateSnapshot) error {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	if err := PublishUserFundingState(state); err != nil {
		return err
	}
	common.OptionMap[operation_setting.UserFundingModeOptionKey] = string(state.Mode)
	common.OptionMap[operation_setting.UserFundingEpochOptionKey] = strconv.FormatUint(state.Epoch, 10)
	return nil
}

// UpdatePaymentFundingOptionsBulk commits payment configuration and the funding
// state revision together. Runtime publication happens after commit, with the
// funding mode/epoch published last as the activation barrier.
//
// Payment runtime publication uses one candidate for the whole group: the
// candidate is prepared (and thereby fully validated) before any in-memory
// write, aborted on the first publication failure, and published with a
// single CAS only after every key and the funding barrier have published.
// If publication fails after the database commit, the runtime stays on the
// previous generation — the previous generation remains the authoritative
// read and no partial group of new values ever becomes observable.
func UpdatePaymentFundingOptionsBulk(values map[string]string) (UserFundingStateSnapshot, error) {
	normalized, err := normalizePaymentFundingOptions(values)
	if err != nil {
		return failClosedUserFundingState(0), err
	}
	// Validate against the payment runtime schema before committing, so a
	// value the runtime cannot represent fails the whole save instead of
	// being persisted without a matching runtime generation.
	for key, value := range normalized {
		if _, err := setting.PaymentMutationFromCanonicalOption(setting.PaymentOptionKey(key), value); err != nil {
			return failClosedUserFundingState(0), err
		}
	}
	optionMutationLock.Lock()
	defer optionMutationLock.Unlock()

	var next UserFundingStateSnapshot
	err = DB.Transaction(func(tx *gorm.DB) error {
		current, option, err := readUserFundingStateTx(tx, true)
		if err != nil {
			return err
		}
		targetMode := current.Mode
		if rawMode, ok := normalized[operation_setting.UserFundingModeOptionKey]; ok {
			targetMode, err = operation_setting.NormalizeUserFundingMode(operation_setting.UserFundingMode(rawMode))
			if err != nil {
				return err
			}
		}
		if targetMode == operation_setting.UserFundingModeEnabled && current.Mode != operation_setting.UserFundingModeEnabled {
			ready, err := fundingEnablementReadyTx(tx, normalized)
			if err != nil {
				return err
			}
			if !ready {
				return ErrUserFundingUnavailable
			}
		}
		next, err = nextUserFundingStateFromCurrentTx(tx, current, targetMode)
		if err != nil {
			return err
		}
		persisted := make(map[string]string, len(normalized))
		for key, value := range normalized {
			if key != operation_setting.UserFundingModeOptionKey {
				persisted[key] = value
			}
		}
		if err := persistOptionValuesTx(tx, persisted); err != nil {
			return err
		}
		return saveUserFundingStateTx(tx, option, next)
	})
	if err != nil {
		return failClosedUserFundingState(0), err
	}

	mutations, err := buildPaymentFundingMutations(normalized, next)
	if err != nil {
		common.SysError(fmt.Sprintf("payment runtime: failed to build funding mutations after commit: %v", err))
		return next, err
	}
	candidate, err := setting.PreparePaymentCandidate(setting.CurrentPaymentRuntime(), mutations)
	if err != nil {
		common.SysError(fmt.Sprintf("payment runtime: failed to prepare funding candidate after commit: %v", err))
		return next, err
	}
	candidatePublished := false
	defer func() {
		if !candidatePublished {
			candidate.Abort()
		}
	}()

	keys := make([]string, 0, len(normalized))
	for key := range normalized {
		if key != operation_setting.UserFundingModeOptionKey {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := paymentFundingOptionPublish(key, normalized[key]); err != nil {
			common.SysError(fmt.Sprintf("payment runtime: aborted funding publication; runtime stays on the previous generation: %v", err))
			return next, err
		}
	}
	if err := publishCommittedUserFundingState(next); err != nil {
		common.SysError(fmt.Sprintf("payment runtime: aborted funding publication at barrier; runtime stays on the previous generation: %v", err))
		return next, err
	}
	if err := candidate.Publish(); err != nil {
		common.SysError(fmt.Sprintf("payment runtime: funding candidate publish failed after commit; runtime stays on the previous generation: %v", err))
		return next, err
	}
	candidatePublished = true
	return next, nil
}

func TransitionUserFundingMode(mode operation_setting.UserFundingMode) (UserFundingStateSnapshot, error) {
	return UpdatePaymentFundingOptionsBulk(map[string]string{
		operation_setting.UserFundingModeOptionKey: string(mode),
	})
}

func PersistSetupFundingOptionsTx(
	tx *gorm.DB,
	selfUseModeEnabled bool,
	demoSiteEnabled bool,
	mode operation_setting.UserFundingMode,
) (UserFundingStateSnapshot, error) {
	values := map[string]string{
		"SelfUseModeEnabled": strconv.FormatBool(selfUseModeEnabled),
		"DemoSiteEnabled":    strconv.FormatBool(demoSiteEnabled),
	}
	if err := persistOptionValuesTx(tx, values); err != nil {
		return failClosedUserFundingState(0), err
	}
	return InitializeUserFundingStateTx(tx, mode)
}

func PublishSetupFundingOptions(
	selfUseModeEnabled bool,
	demoSiteEnabled bool,
	state UserFundingStateSnapshot,
) error {
	optionMutationLock.Lock()
	defer optionMutationLock.Unlock()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	operation_setting.SelfUseModeEnabled = selfUseModeEnabled
	operation_setting.DemoSiteEnabled = demoSiteEnabled
	common.OptionMap["SelfUseModeEnabled"] = strconv.FormatBool(selfUseModeEnabled)
	common.OptionMap["DemoSiteEnabled"] = strconv.FormatBool(demoSiteEnabled)
	common.OptionMapRWMutex.Unlock()
	return publishCommittedUserFundingState(state)
}
