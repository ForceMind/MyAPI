package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/model_setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/performance_setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/ForceMind/MyAPI/setting/system_setting"
	"gorm.io/gorm"
)

type Option struct {
	Key   string `json:"key" gorm:"primaryKey"`
	Value string `json:"value"`
}

const (
	groupRatioOptionKey            = "GroupRatio"
	groupRatioOptionAlias          = "group_ratio_setting.group_ratio"
	groupGroupRatioOptionKey       = "GroupGroupRatio"
	groupGroupRatioOptionAlias     = "group_ratio_setting.group_group_ratio"
	groupRatioAliasConflictLogText = "conflicting group ratio option aliases"
)

type groupRatioOptionPair struct {
	canonical string
	alias     string
}

var groupRatioOptionPairs = []groupRatioOptionPair{
	{canonical: groupRatioOptionKey, alias: groupRatioOptionAlias},
	{canonical: groupGroupRatioOptionKey, alias: groupGroupRatioOptionAlias},
}

// optionMutationLock orders database snapshots/commits and their in-process
// publication. Its production implementation is a non-reentrant mutex.
var optionMutationLock sync.Locker = &sync.Mutex{}

func AllOption() ([]*Option, error) {
	var options []*Option
	var err error
	err = DB.Find(&options).Error
	return options, err
}

func InitOptionMap() {
	optionMutationLock.Lock()
	defer optionMutationLock.Unlock()
	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)
	rateLimitConfig := setting.GetModelRequestRateLimitConfig()

	// 添加原有的系统配置
	common.OptionMap["FileUploadPermission"] = strconv.Itoa(common.FileUploadPermission)
	common.OptionMap["FileDownloadPermission"] = strconv.Itoa(common.FileDownloadPermission)
	common.OptionMap["ImageUploadPermission"] = strconv.Itoa(common.ImageUploadPermission)
	common.OptionMap["ImageDownloadPermission"] = strconv.Itoa(common.ImageDownloadPermission)
	common.OptionMap["PasswordLoginEnabled"] = strconv.FormatBool(common.PasswordLoginEnabled)
	common.OptionMap["PasswordRegisterEnabled"] = strconv.FormatBool(common.PasswordRegisterEnabled)
	common.OptionMap["EmailVerificationEnabled"] = strconv.FormatBool(common.EmailVerificationEnabled)
	common.OptionMap["GitHubOAuthEnabled"] = strconv.FormatBool(common.GitHubOAuthEnabled)
	common.OptionMap["LinuxDOOAuthEnabled"] = strconv.FormatBool(common.LinuxDOOAuthEnabled)
	common.OptionMap["TelegramOAuthEnabled"] = strconv.FormatBool(common.TelegramOAuthEnabled)
	common.OptionMap["WeChatAuthEnabled"] = strconv.FormatBool(common.WeChatAuthEnabled)
	common.OptionMap["TurnstileCheckEnabled"] = strconv.FormatBool(common.TurnstileCheckEnabled)
	common.OptionMap["RegisterEnabled"] = strconv.FormatBool(common.RegisterEnabled)
	common.OptionMap["AutomaticDisableChannelEnabled"] = strconv.FormatBool(common.AutomaticDisableChannelEnabled)
	common.OptionMap["AutomaticEnableChannelEnabled"] = strconv.FormatBool(common.AutomaticEnableChannelEnabled)
	common.OptionMap["LogConsumeEnabled"] = strconv.FormatBool(common.LogConsumeEnabled)
	common.OptionMap["DisplayInCurrencyEnabled"] = strconv.FormatBool(common.DisplayInCurrencyEnabled)
	common.OptionMap["DisplayTokenStatEnabled"] = strconv.FormatBool(common.DisplayTokenStatEnabled)
	common.OptionMap["DrawingEnabled"] = strconv.FormatBool(common.DrawingEnabled)
	common.OptionMap["TaskEnabled"] = strconv.FormatBool(common.TaskEnabled)
	common.OptionMap["DataExportEnabled"] = strconv.FormatBool(common.DataExportEnabled)
	common.OptionMap["ChannelDisableThreshold"] = strconv.FormatFloat(common.ChannelDisableThreshold, 'f', -1, 64)
	common.OptionMap["EmailDomainRestrictionEnabled"] = strconv.FormatBool(common.EmailDomainRestrictionEnabled)
	common.OptionMap["EmailAliasRestrictionEnabled"] = strconv.FormatBool(common.EmailAliasRestrictionEnabled)
	common.OptionMap["EmailDomainWhitelist"] = strings.Join(common.EmailDomainWhitelist, ",")
	common.OptionMap["SMTPServer"] = ""
	common.OptionMap["SMTPFrom"] = ""
	common.OptionMap["SMTPPort"] = strconv.Itoa(common.SMTPPort)
	common.OptionMap["SMTPAccount"] = ""
	common.OptionMap["SMTPToken"] = ""
	common.OptionMap["SMTPSSLEnabled"] = strconv.FormatBool(common.SMTPSSLEnabled)
	common.OptionMap["SMTPStartTLSEnabled"] = strconv.FormatBool(common.SMTPStartTLSEnabled)
	common.OptionMap["SMTPInsecureSkipVerify"] = strconv.FormatBool(common.SMTPInsecureSkipVerify)
	common.OptionMap["SMTPForceAuthLogin"] = strconv.FormatBool(common.SMTPForceAuthLogin)
	common.OptionMap["Notice"] = ""
	common.OptionMap["About"] = ""
	common.OptionMap["HomePageContent"] = ""
	common.OptionMap["Footer"] = common.Footer
	common.OptionMap["SystemName"] = common.SystemName
	common.OptionMap["Logo"] = common.Logo
	common.OptionMap["ServerAddress"] = system_setting.GetServerAddress()
	common.OptionMap["WorkerUrl"] = system_setting.WorkerUrl
	common.OptionMap["WorkerValidKey"] = system_setting.WorkerValidKey
	common.OptionMap["WorkerAllowHttpImageRequestEnabled"] = strconv.FormatBool(system_setting.WorkerAllowHttpImageRequestEnabled)
	common.OptionMap["PayAddress"] = ""
	common.OptionMap["CustomCallbackAddress"] = ""
	common.OptionMap["EpayId"] = ""
	common.OptionMap["EpayKey"] = ""
	common.OptionMap["Price"] = strconv.FormatFloat(operation_setting.Price, 'f', -1, 64)
	common.OptionMap["USDExchangeRate"] = strconv.FormatFloat(operation_setting.USDExchangeRate, 'f', -1, 64)
	common.OptionMap["MinTopUp"] = strconv.Itoa(operation_setting.MinTopUp)
	common.OptionMap["StripeMinTopUp"] = strconv.Itoa(setting.StripeMinTopUp)
	common.OptionMap["StripeApiSecret"] = setting.StripeApiSecret
	common.OptionMap["StripeWebhookSecret"] = setting.StripeWebhookSecret
	common.OptionMap["StripePriceId"] = setting.StripePriceId
	common.OptionMap["StripeUnitPrice"] = strconv.FormatFloat(setting.StripeUnitPrice, 'f', -1, 64)
	common.OptionMap["StripePromotionCodesEnabled"] = strconv.FormatBool(setting.StripePromotionCodesEnabled)
	common.OptionMap["CreemApiKey"] = setting.CreemApiKey
	common.OptionMap["CreemProducts"] = setting.CreemProducts
	common.OptionMap["CreemTestMode"] = strconv.FormatBool(setting.CreemTestMode)
	common.OptionMap["CreemWebhookSecret"] = setting.CreemWebhookSecret
	common.OptionMap["WaffoEnabled"] = strconv.FormatBool(setting.WaffoEnabled)
	common.OptionMap["WaffoApiKey"] = setting.WaffoApiKey
	common.OptionMap["WaffoPrivateKey"] = setting.WaffoPrivateKey
	common.OptionMap["WaffoPublicCert"] = setting.WaffoPublicCert
	common.OptionMap["WaffoSandboxPublicCert"] = setting.WaffoSandboxPublicCert
	common.OptionMap["WaffoSandboxApiKey"] = setting.WaffoSandboxApiKey
	common.OptionMap["WaffoSandboxPrivateKey"] = setting.WaffoSandboxPrivateKey
	common.OptionMap["WaffoSandbox"] = strconv.FormatBool(setting.WaffoSandbox)
	common.OptionMap["WaffoMerchantId"] = setting.WaffoMerchantId
	common.OptionMap["WaffoNotifyUrl"] = setting.WaffoNotifyUrl
	common.OptionMap["WaffoReturnUrl"] = setting.WaffoReturnUrl
	common.OptionMap["WaffoSubscriptionReturnUrl"] = setting.WaffoSubscriptionReturnUrl
	common.OptionMap["WaffoCurrency"] = setting.WaffoCurrency
	common.OptionMap["WaffoUnitPrice"] = strconv.FormatFloat(setting.WaffoUnitPrice, 'f', -1, 64)
	common.OptionMap["WaffoMinTopUp"] = strconv.Itoa(setting.WaffoMinTopUp)
	common.OptionMap["WaffoPayMethods"] = setting.WaffoPayMethods2JsonString()
	common.OptionMap["WaffoPancakeMerchantID"] = setting.WaffoPancakeMerchantID
	common.OptionMap["WaffoPancakePrivateKey"] = setting.WaffoPancakePrivateKey
	common.OptionMap["WaffoPancakeReturnURL"] = setting.WaffoPancakeReturnURL
	common.OptionMap["WaffoPancakeUnitPrice"] = strconv.FormatFloat(setting.WaffoPancakeUnitPrice, 'f', -1, 64)
	common.OptionMap["WaffoPancakeMinTopUp"] = strconv.Itoa(setting.WaffoPancakeMinTopUp)
	common.OptionMap["WaffoPancakeStoreID"] = setting.WaffoPancakeStoreID
	common.OptionMap["WaffoPancakeProductID"] = setting.WaffoPancakeProductID
	common.OptionMap["TopupGroupRatio"] = common.TopupGroupRatio2JSONString()
	common.OptionMap["Chats"] = setting.Chats2JsonString()
	common.OptionMap["AutoGroups"] = setting.AutoGroups2JsonString()
	common.OptionMap["DefaultUseAutoGroup"] = strconv.FormatBool(setting.DefaultUseAutoGroup)
	common.OptionMap["MaxTokenAutoGroups"] = strconv.Itoa(setting.GetMaxTokenAutoGroups())
	common.OptionMap["PayMethods"] = operation_setting.PayMethods2JsonString()
	common.OptionMap["GitHubClientId"] = ""
	common.OptionMap["GitHubClientSecret"] = ""
	common.OptionMap["TelegramBotToken"] = ""
	common.OptionMap["TelegramBotName"] = ""
	common.OptionMap["WeChatServerAddress"] = ""
	common.OptionMap["WeChatServerToken"] = ""
	common.OptionMap["WeChatAccountQRCodeImageURL"] = ""
	common.OptionMap["TurnstileSiteKey"] = ""
	common.OptionMap["TurnstileSecretKey"] = ""
	common.OptionMap["QuotaForNewUser"] = strconv.Itoa(common.QuotaForNewUser)
	common.OptionMap["QuotaForInviter"] = strconv.Itoa(common.QuotaForInviter)
	common.OptionMap["QuotaForInvitee"] = strconv.Itoa(common.QuotaForInvitee)
	common.OptionMap["QuotaRemindThreshold"] = strconv.Itoa(common.QuotaRemindThreshold)
	common.OptionMap["PreConsumedQuota"] = strconv.Itoa(common.PreConsumedQuota)
	// Provider quota snapshots are enabled by default and can be adjusted from
	// the administrator monitoring settings page. Environment variables remain
	// deployment-level overrides for operators that need a hard disable/bound.
	common.OptionMap["ChannelQuotaSyncEnabled"] = "true"
	common.OptionMap["ChannelQuotaSyncIntervalMinutes"] = "1"
	common.OptionMap["ChannelQuotaSyncMaxChannels"] = "100"
	if quotaAlertJSON, err := common.MarshalChannelQuotaAlertSettings(common.ChannelQuotaAlertSettings{
		Enabled:          common.ChannelQuotaAlertEnabled,
		WarningPercent:   common.ChannelQuotaAlertWarningPercent,
		CriticalPercent:  common.ChannelQuotaAlertCriticalPercent,
		CooldownSeconds:  common.ChannelQuotaAlertCooldownSeconds,
		NotifyOnRecovery: common.ChannelQuotaAlertNotifyOnRecovery,
	}); err == nil {
		common.OptionMap[common.ChannelQuotaAlertSettingsOptionKey] = quotaAlertJSON
	}
	common.OptionMap["ModelRequestRateLimitCount"] = strconv.Itoa(rateLimitConfig.Total)
	common.OptionMap["ModelRequestRateLimitDurationMinutes"] = strconv.Itoa(rateLimitConfig.DurationMinutes)
	common.OptionMap["ModelRequestRateLimitSuccessCount"] = strconv.Itoa(rateLimitConfig.Success)
	rateLimitGroupJSON, err := common.Marshal(rateLimitConfig.Group)
	if err == nil {
		common.OptionMap["ModelRequestRateLimitGroup"] = string(rateLimitGroupJSON)
	}
	common.OptionMap["ModelRatio"] = ratio_setting.ModelRatio2JSONString()
	common.OptionMap["ModelPrice"] = ratio_setting.ModelPrice2JSONString()
	common.OptionMap["CacheRatio"] = ratio_setting.CacheRatio2JSONString()
	common.OptionMap["CreateCacheRatio"] = ratio_setting.CreateCacheRatio2JSONString()
	common.OptionMap["GroupRatio"] = ratio_setting.GroupRatio2JSONString()
	common.OptionMap["GroupGroupRatio"] = ratio_setting.GroupGroupRatio2JSONString()
	common.OptionMap["UserUsableGroups"] = setting.UserUsableGroups2JSONString()
	common.OptionMap["CompletionRatio"] = ratio_setting.CompletionRatio2JSONString()
	common.OptionMap["ImageRatio"] = ratio_setting.ImageRatio2JSONString()
	common.OptionMap["AudioRatio"] = ratio_setting.AudioRatio2JSONString()
	common.OptionMap["AudioCompletionRatio"] = ratio_setting.AudioCompletionRatio2JSONString()
	common.OptionMap["TopUpLink"] = common.TopUpLink
	//common.OptionMap["ChatLink"] = common.ChatLink
	//common.OptionMap["ChatLink2"] = common.ChatLink2
	common.OptionMap["QuotaPerUnit"] = strconv.FormatFloat(common.QuotaPerUnit, 'f', -1, 64)
	common.OptionMap["RetryTimes"] = strconv.Itoa(common.RetryTimes)
	common.OptionMap["DataExportInterval"] = strconv.Itoa(common.DataExportInterval)
	common.OptionMap["DataExportDefaultTime"] = common.DataExportDefaultTime
	common.OptionMap["DefaultCollapseSidebar"] = strconv.FormatBool(common.DefaultCollapseSidebar)
	common.OptionMap["MjNotifyEnabled"] = strconv.FormatBool(setting.MjNotifyEnabled)
	common.OptionMap["MjAccountFilterEnabled"] = strconv.FormatBool(setting.MjAccountFilterEnabled)
	common.OptionMap["MjModeClearEnabled"] = strconv.FormatBool(setting.MjModeClearEnabled)
	common.OptionMap["MjForwardUrlEnabled"] = strconv.FormatBool(setting.MjForwardUrlEnabled)
	common.OptionMap["MjActionCheckSuccessEnabled"] = strconv.FormatBool(setting.MjActionCheckSuccessEnabled)
	common.OptionMap["CheckSensitiveEnabled"] = strconv.FormatBool(setting.CheckSensitiveEnabled)
	common.OptionMap["DemoSiteEnabled"] = strconv.FormatBool(operation_setting.DemoSiteEnabled)
	common.OptionMap["SelfUseModeEnabled"] = strconv.FormatBool(operation_setting.SelfUseModeEnabled)
	common.OptionMap["ModelRequestRateLimitEnabled"] = strconv.FormatBool(rateLimitConfig.Enabled)
	common.OptionMap["CheckSensitiveOnPromptEnabled"] = strconv.FormatBool(setting.CheckSensitiveOnPromptEnabled)
	common.OptionMap["StopOnSensitiveEnabled"] = strconv.FormatBool(setting.StopOnSensitiveEnabled)
	common.OptionMap["SensitiveWords"] = setting.SensitiveWordsToString()
	common.OptionMap["StreamCacheQueueLength"] = strconv.Itoa(setting.StreamCacheQueueLength)
	common.OptionMap["AutomaticDisableKeywords"] = operation_setting.AutomaticDisableKeywordsToString()
	common.OptionMap["AutomaticDisableStatusCodes"] = operation_setting.AutomaticDisableStatusCodesToString()
	common.OptionMap["AutomaticRetryStatusCodes"] = operation_setting.AutomaticRetryStatusCodesToString()
	common.OptionMap["ExposeRatioEnabled"] = strconv.FormatBool(ratio_setting.IsExposeRatioEnabled())

	// 自动添加所有注册的模型配置
	modelConfigs := config.GlobalConfig.ExportAllConfigs()
	for k, v := range modelConfigs {
		common.OptionMap[k] = v
	}
	// The flat option names are canonical. Keep the compatibility aliases in
	// the same snapshot so readers never observe two meanings for one setting.
	common.OptionMap[groupRatioOptionAlias] = common.OptionMap[groupRatioOptionKey]
	common.OptionMap[groupGroupRatioOptionAlias] = common.OptionMap[groupGroupRatioOptionKey]

	common.OptionMapRWMutex.Unlock()
	loadOptionsFromDatabaseLocked()
}

func loadOptionsFromDatabase() {
	optionMutationLock.Lock()
	defer optionMutationLock.Unlock()
	loadOptionsFromDatabaseLocked()
}

// loadOptionsFromDatabaseLocked requires optionMutationLock. Keep the read
// and publication in the same sequence as writers so an older snapshot cannot
// overwrite a newer local commit. OptionMap's lock is not held during I/O.
func loadOptionsFromDatabaseLocked() {
	options, err := AllOption()
	if err != nil {
		common.SysLog("failed to load options from database: " + err.Error())
		return
	}
	rateLimitValues := make(map[string]string)
	groupRatioValues := make(map[string]string, len(groupRatioOptionPairs)*2)
	for _, option := range options {
		if _, ok := groupRatioOptionPairForKey(option.Key); ok {
			groupRatioValues[option.Key] = option.Value
			continue
		}
		if isModelRequestRateLimitOption(option.Key) {
			rateLimitValues[option.Key] = option.Value
			continue
		}
		err := updateOptionMap(option.Key, option.Value)
		if err != nil {
			common.SysLog("failed to update option map: " + err.Error())
		}
	}
	for _, pair := range groupRatioOptionPairs {
		publishLoadedGroupRatioOptionPair(pair, groupRatioValues)
	}
	if len(rateLimitValues) > 0 {
		if err := publishModelRequestRateLimitOptions(rateLimitValues, nil); err != nil {
			common.SysLog("failed to update model request rate limit options: " + err.Error())
		}
	}
}

func SyncOptions(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing options from database")
		loadOptionsFromDatabase()
	}
}

func validateOptionValue(key string, value string) error {
	if err := common.ValidateChannelQuotaAlertOptionValue(key, value); err != nil {
		return err
	}
	if key == operation_setting.ToolPriceOptionKey {
		return operation_setting.ValidateToolPricesJSON(value)
	}
	if key == operation_setting.ChannelTestConcurrencyOptionKey {
		return operation_setting.ValidateChannelTestConcurrency(value)
	}
	if key == "MaxTokenAutoGroups" {
		return setting.ValidateMaxTokenAutoGroups(value)
	}
	if key == "AutoGroups" {
		return setting.ValidateAutoGroupsJSON(value)
	}
	if key == "TopupGroupRatio" {
		return common.ValidateTopupGroupRatioJSON(value)
	}
	if key == "PayMethods" {
		return operation_setting.ValidatePayMethodsJSON(value)
	}
	if key == "AutomaticDisableStatusCodes" || key == "AutomaticRetryStatusCodes" {
		_, err := operation_setting.ParseHTTPStatusCodeRanges(value)
		return err
	}
	if key == "gemini.safety_settings" {
		return model_setting.ValidateGeminiSafetySettings(value)
	}
	if key == "claude.default_max_tokens" {
		return model_setting.ValidateClaudeDefaultMaxTokens(value)
	}
	if key == "claude.thinking_adapter_budget_tokens_percentage" || key == "gemini.thinking_adapter_budget_tokens_percentage" {
		percentage, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		if key == "claude.thinking_adapter_budget_tokens_percentage" {
			return model_setting.ValidateClaudeThinkingAdapterBudgetTokensPercentage(percentage)
		}
		return model_setting.ValidateGeminiThinkingAdapterBudgetTokensPercentage(percentage)
	}
	if isModelRequestRateLimitOption(key) {
		rateLimitConfig := setting.GetModelRequestRateLimitConfig()
		_, err := applyModelRequestRateLimitOptionValue(&rateLimitConfig, key, value)
		if err != nil {
			return err
		}
		return setting.ValidateModelRequestRateLimitConfig(rateLimitConfig)
	}
	switch key {
	case "ModelRatio", "ModelPrice", "CompletionRatio", "CacheRatio", "CreateCacheRatio", "ImageRatio", "AudioRatio", "AudioCompletionRatio":
		return ratio_setting.ValidateRatioMapJSON(value)
	case "GroupRatio", "group_ratio_setting.group_ratio":
		return ratio_setting.ValidateRatioMapJSON(value)
	case "GroupGroupRatio", "group_ratio_setting.group_group_ratio":
		return ratio_setting.ValidateNestedRatioMapJSON(value)
	case "group_ratio_setting.group_special_usable_group":
		return ratio_setting.ValidateGroupSpecialUsableGroupJSON(value)
	case "Chats":
		return setting.ValidateChatsJSON(value)
	case "UserUsableGroups":
		return setting.ValidateUserUsableGroupsJSON(value)
	}
	if key == "ChannelQuotaSyncEnabled" {
		if _, err := strconv.ParseBool(strings.TrimSpace(value)); err != nil {
			return err
		}
	}
	if key == "ChannelQuotaSyncIntervalMinutes" {
		minutes, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || minutes < 1 || minutes > 1440 {
			return gorm.ErrInvalidData
		}
	}
	if key == "ChannelQuotaSyncMaxChannels" {
		maxChannels, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || maxChannels < 1 || maxChannels > 1000 {
			return gorm.ErrInvalidData
		}
	}
	if key == "access_profile_setting.profiles" {
		return setting.ValidateAccessProfileDefinitionsJSON(value)
	}
	parts := strings.SplitN(key, ".", 2)
	if len(parts) == 2 {
		if cfg := config.GlobalConfig.Get(parts[0]); cfg != nil {
			err := config.ValidateConfigFromMap(cfg, map[string]string{parts[1]: value})
			if err != nil && err != config.ErrMapConfigValidationUnsupported {
				return err
			}
		}
	}
	return nil
}

func isModelRequestRateLimitOption(key string) bool {
	switch key {
	case "ModelRequestRateLimitEnabled", "ModelRequestRateLimitDurationMinutes", "ModelRequestRateLimitCount", "ModelRequestRateLimitSuccessCount", "ModelRequestRateLimitGroup":
		return true
	default:
		return false
	}
}

func applyModelRequestRateLimitOptionValue(config *setting.ModelRequestRateLimitConfig, key, value string) (bool, error) {
	switch key {
	case "ModelRequestRateLimitEnabled":
		enabled, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return true, err
		}
		config.Enabled = enabled
	case "ModelRequestRateLimitDurationMinutes":
		duration, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return true, err
		}
		config.DurationMinutes = duration
	case "ModelRequestRateLimitCount":
		count, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return true, err
		}
		config.Total = count
	case "ModelRequestRateLimitSuccessCount":
		count, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return true, err
		}
		config.Success = count
	case "ModelRequestRateLimitGroup":
		group, err := setting.ParseModelRequestRateLimitGroupJSON(value)
		if err != nil {
			return true, err
		}
		config.Group = group
	default:
		return false, nil
	}
	return true, nil
}

func prepareModelRequestRateLimitConfig(values map[string]string) (setting.ModelRequestRateLimitConfig, error) {
	config := setting.GetModelRequestRateLimitConfig()
	for key, value := range values {
		if _, err := applyModelRequestRateLimitOptionValue(&config, key, value); err != nil {
			return setting.ModelRequestRateLimitConfig{}, err
		}
	}
	if err := setting.ValidateModelRequestRateLimitConfig(config); err != nil {
		return setting.ModelRequestRateLimitConfig{}, err
	}
	return config, nil
}

func publishModelRequestRateLimitOptions(values map[string]string, prepared *setting.ModelRequestRateLimitConfig) error {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()

	var config setting.ModelRequestRateLimitConfig
	if prepared == nil {
		var err error
		config, err = prepareModelRequestRateLimitConfig(values)
		if err != nil {
			return err
		}
	} else {
		config = *prepared
	}
	if err := setting.ApplyModelRequestRateLimitConfig(config); err != nil {
		return err
	}
	for key, value := range values {
		common.OptionMap[key] = value
	}
	return nil
}

func UpdateOption(key string, value string) error {
	return UpdateOptionsBulk(map[string]string{key: value})
}

// UpdateOptionsBulk persists multiple key/value pairs in a single database
// transaction, then dispatches them through updateOptionMap in one pass. If
// any DB write fails the whole transaction rolls back and no in-memory state
// is touched — safe for callers that must commit a set of related options
// atomically (e.g. payment gateway binding).
func UpdateOptionsBulk(values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(values)+len(groupRatioOptionPairs)*2)
	groupRatioValues := make(map[string]string, len(groupRatioOptionPairs))
	for key, value := range values {
		var err error
		value, err = normalizeOptionValue(key, value)
		if err != nil {
			return err
		}
		if !isModelRequestRateLimitOption(key) {
			if err := validateOptionValue(key, value); err != nil {
				return err
			}
		}
		if pair, ok := groupRatioOptionPairForKey(key); ok {
			if previous, exists := groupRatioValues[pair.canonical]; exists && previous != value {
				return fmt.Errorf("%s: %s and %s contain different values", groupRatioAliasConflictLogText, pair.canonical, pair.alias)
			}
			groupRatioValues[pair.canonical] = value
			continue
		}
		normalized[key] = value
	}
	for _, pair := range groupRatioOptionPairs {
		if value, ok := groupRatioValues[pair.canonical]; ok {
			normalized[pair.canonical] = value
			normalized[pair.alias] = value
		}
	}
	// Serialize commits and their complete local publication with reloads.
	// This is a single-process ordering guarantee, not a cross-instance lock
	// or an atomic read snapshot across all configuration fields.
	optionMutationLock.Lock()
	defer optionMutationLock.Unlock()
	rateLimitValues := make(map[string]string)
	for key, value := range normalized {
		if isModelRequestRateLimitOption(key) {
			rateLimitValues[key] = value
		}
	}
	var rateLimitConfig *setting.ModelRequestRateLimitConfig
	if len(rateLimitValues) > 0 {
		config, err := prepareModelRequestRateLimitConfig(rateLimitValues)
		if err != nil {
			return err
		}
		rateLimitConfig = &config
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		keys := make([]string, 0, len(normalized))
		for key := range normalized {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := normalized[k]
			option := Option{Key: k}
			if err := tx.FirstOrCreate(&option, Option{Key: k}).Error; err != nil {
				return err
			}
			option.Value = v
			if err := tx.Save(&option).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for k, v := range normalized {
		if _, ok := groupRatioOptionPairForKey(k); ok {
			continue
		}
		if isModelRequestRateLimitOption(k) {
			rateLimitValues[k] = v
			continue
		}
		if err := updateOptionMap(k, v); err != nil {
			return err
		}
	}
	for _, pair := range groupRatioOptionPairs {
		if value, ok := groupRatioValues[pair.canonical]; ok {
			if err := publishGroupRatioOptionPair(pair, value); err != nil {
				return err
			}
		}
	}
	if len(rateLimitValues) > 0 {
		if err := publishModelRequestRateLimitOptions(rateLimitValues, rateLimitConfig); err != nil {
			return err
		}
	}
	return nil
}

func updateOptionMap(key string, value string) (err error) {
	value, err = normalizeOptionValue(key, value)
	if err != nil {
		return err
	}
	if err = validateOptionValue(key, value); err != nil {
		return err
	}
	if pair, ok := groupRatioOptionPairForKey(key); ok {
		return publishGroupRatioOptionPair(pair, value)
	}
	if key == retiredThemeOptionKey {
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, key)
		common.OptionMapRWMutex.Unlock()
		return nil
	}
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	if key == "access_profile_setting.profiles" {
		// Publish exactly once through the profile registry's synchronization;
		// the generic reflective writer must never see its mutable fields.
		if err := setting.UpdateAccessProfileDefinitionsByJSONString(value); err != nil {
			return err
		}
		common.OptionMap[key] = value
		return nil
	}
	previousValue, hadPreviousValue := common.OptionMap[key]
	common.OptionMap[key] = value
	defer func() {
		if err == nil {
			return
		}
		if hadPreviousValue {
			common.OptionMap[key] = previousValue
		} else {
			delete(common.OptionMap, key)
		}
	}()

	// 检查是否是模型配置 - 使用更规范的方式处理
	if handled, configErr := handleConfigUpdate(key, value); handled {
		return configErr
	}

	// 处理传统配置项...
	if strings.HasSuffix(key, "Permission") {
		intValue, _ := strconv.Atoi(value)
		switch key {
		case "FileUploadPermission":
			common.FileUploadPermission = intValue
		case "FileDownloadPermission":
			common.FileDownloadPermission = intValue
		case "ImageUploadPermission":
			common.ImageUploadPermission = intValue
		case "ImageDownloadPermission":
			common.ImageDownloadPermission = intValue
		}
	}
	if strings.HasSuffix(key, "Enabled") || key == "DefaultCollapseSidebar" || key == "DefaultUseAutoGroup" || key == "SMTPForceAuthLogin" || key == "SMTPInsecureSkipVerify" {
		boolValue := value == "true"
		switch key {
		case "PasswordRegisterEnabled":
			common.PasswordRegisterEnabled = boolValue
		case "PasswordLoginEnabled":
			common.PasswordLoginEnabled = boolValue
		case "EmailVerificationEnabled":
			common.EmailVerificationEnabled = boolValue
		case "GitHubOAuthEnabled":
			common.GitHubOAuthEnabled = boolValue
		case "LinuxDOOAuthEnabled":
			common.LinuxDOOAuthEnabled = boolValue
		case "WeChatAuthEnabled":
			common.WeChatAuthEnabled = boolValue
		case "TelegramOAuthEnabled":
			common.TelegramOAuthEnabled = boolValue
		case "TurnstileCheckEnabled":
			common.TurnstileCheckEnabled = boolValue
		case "RegisterEnabled":
			common.RegisterEnabled = boolValue
		case "EmailDomainRestrictionEnabled":
			common.EmailDomainRestrictionEnabled = boolValue
		case "EmailAliasRestrictionEnabled":
			common.EmailAliasRestrictionEnabled = boolValue
		case "AutomaticDisableChannelEnabled":
			common.AutomaticDisableChannelEnabled = boolValue
		case "AutomaticEnableChannelEnabled":
			common.AutomaticEnableChannelEnabled = boolValue
		case "LogConsumeEnabled":
			common.LogConsumeEnabled = boolValue
		case "DisplayInCurrencyEnabled":
			// 兼容旧字段：同步到新配置 general_setting.quota_display_type（运行时生效）
			// true -> USD, false -> TOKENS
			newVal := "USD"
			if !boolValue {
				newVal = "TOKENS"
			}
			if cfg := config.GlobalConfig.Get("general_setting"); cfg != nil {
				_ = config.UpdateConfigFromMap(cfg, map[string]string{"quota_display_type": newVal})
			}
		case "DisplayTokenStatEnabled":
			common.DisplayTokenStatEnabled = boolValue
		case "DrawingEnabled":
			common.DrawingEnabled = boolValue
		case "TaskEnabled":
			common.TaskEnabled = boolValue
		case "DataExportEnabled":
			common.DataExportEnabled = boolValue
		case "DefaultCollapseSidebar":
			common.DefaultCollapseSidebar = boolValue
		case "MjNotifyEnabled":
			setting.MjNotifyEnabled = boolValue
		case "MjAccountFilterEnabled":
			setting.MjAccountFilterEnabled = boolValue
		case "MjModeClearEnabled":
			setting.MjModeClearEnabled = boolValue
		case "MjForwardUrlEnabled":
			setting.MjForwardUrlEnabled = boolValue
		case "MjActionCheckSuccessEnabled":
			setting.MjActionCheckSuccessEnabled = boolValue
		case "CheckSensitiveEnabled":
			setting.CheckSensitiveEnabled = boolValue
		case "DemoSiteEnabled":
			operation_setting.DemoSiteEnabled = boolValue
		case "SelfUseModeEnabled":
			operation_setting.SelfUseModeEnabled = boolValue
		case "CheckSensitiveOnPromptEnabled":
			setting.CheckSensitiveOnPromptEnabled = boolValue
		case "ModelRequestRateLimitEnabled":
			enabled, parseErr := strconv.ParseBool(strings.TrimSpace(value))
			if parseErr != nil {
				err = parseErr
			} else {
				err = setting.SetModelRequestRateLimitEnabled(enabled)
			}
		case "StopOnSensitiveEnabled":
			setting.StopOnSensitiveEnabled = boolValue
		case "SMTPSSLEnabled":
			common.SMTPSSLEnabled = boolValue
		case "SMTPStartTLSEnabled":
			common.SMTPStartTLSEnabled = boolValue
		case "SMTPInsecureSkipVerify":
			common.SMTPInsecureSkipVerify = boolValue
		case "SMTPForceAuthLogin":
			common.SMTPForceAuthLogin = boolValue
		case "WorkerAllowHttpImageRequestEnabled":
			system_setting.WorkerAllowHttpImageRequestEnabled = boolValue
		case "DefaultUseAutoGroup":
			setting.DefaultUseAutoGroup = boolValue
		case "ExposeRatioEnabled":
			ratio_setting.SetExposeRatioEnabled(boolValue)
		}
	}
	switch key {
	case common.ChannelQuotaAlertSettingsOptionKey:
		settings, parseErr := common.ParseChannelQuotaAlertSettings(value)
		if parseErr != nil {
			return parseErr
		}
		common.ChannelQuotaAlertEnabled = settings.Enabled
		common.ChannelQuotaAlertWarningPercent = settings.WarningPercent
		common.ChannelQuotaAlertCriticalPercent = settings.CriticalPercent
		common.ChannelQuotaAlertCooldownSeconds = settings.CooldownSeconds
		common.ChannelQuotaAlertNotifyOnRecovery = settings.NotifyOnRecovery
	case common.ChannelQuotaAlertEnabledOptionKey:
		common.ChannelQuotaAlertEnabled, err = strconv.ParseBool(strings.TrimSpace(value))
	case common.ChannelQuotaAlertWarningPercentOptionKey:
		common.ChannelQuotaAlertWarningPercent, err = strconv.ParseFloat(strings.TrimSpace(value), 64)
	case common.ChannelQuotaAlertCriticalPercentOptionKey:
		common.ChannelQuotaAlertCriticalPercent, err = strconv.ParseFloat(strings.TrimSpace(value), 64)
	case "EmailDomainWhitelist":
		common.EmailDomainWhitelist = strings.Split(value, ",")
	case "SMTPServer":
		common.SMTPServer = value
	case "SMTPPort":
		intValue, _ := strconv.Atoi(value)
		common.SMTPPort = intValue
	case "SMTPAccount":
		common.SMTPAccount = value
	case "SMTPFrom":
		common.SMTPFrom = value
	case "SMTPToken":
		common.SMTPToken = value
	case "ServerAddress":
		system_setting.SetServerAddress(value)
	case "WorkerUrl":
		system_setting.WorkerUrl = value
	case "WorkerValidKey":
		system_setting.WorkerValidKey = value
	case "PayAddress":
		operation_setting.PayAddress = value
	case "Chats":
		err = setting.UpdateChatsByJsonString(value)
	case "AutoGroups":
		err = setting.UpdateAutoGroupsByJsonString(value)
	case "MaxTokenAutoGroups":
		err = setting.UpdateMaxTokenAutoGroups(value)
	case "CustomCallbackAddress":
		operation_setting.CustomCallbackAddress = value
	case "EpayId":
		operation_setting.EpayId = value
	case "EpayKey":
		operation_setting.EpayKey = value
	case "Price":
		operation_setting.Price, _ = strconv.ParseFloat(value, 64)
	case "USDExchangeRate":
		operation_setting.USDExchangeRate, _ = strconv.ParseFloat(value, 64)
	case "MinTopUp":
		operation_setting.MinTopUp, _ = strconv.Atoi(value)
	case "StripeApiSecret":
		setting.StripeApiSecret = value
	case "StripeWebhookSecret":
		setting.StripeWebhookSecret = value
	case "StripePriceId":
		setting.StripePriceId = value
	case "StripeUnitPrice":
		setting.StripeUnitPrice, _ = strconv.ParseFloat(value, 64)
	case "StripeMinTopUp":
		setting.StripeMinTopUp, _ = strconv.Atoi(value)
	case "StripePromotionCodesEnabled":
		setting.StripePromotionCodesEnabled = value == "true"
	case "CreemApiKey":
		setting.CreemApiKey = value
	case "CreemProducts":
		setting.CreemProducts = value
	case "CreemTestMode":
		setting.CreemTestMode = value == "true"
	case "CreemWebhookSecret":
		setting.CreemWebhookSecret = value
	case "WaffoEnabled":
		setting.WaffoEnabled = value == "true"
	case "WaffoApiKey":
		setting.WaffoApiKey = value
	case "WaffoPrivateKey":
		setting.WaffoPrivateKey = value
	case "WaffoPublicCert":
		setting.WaffoPublicCert = value
	case "WaffoSandboxPublicCert":
		setting.WaffoSandboxPublicCert = value
	case "WaffoSandboxApiKey":
		setting.WaffoSandboxApiKey = value
	case "WaffoSandboxPrivateKey":
		setting.WaffoSandboxPrivateKey = value
	case "WaffoSandbox":
		setting.WaffoSandbox = value == "true"
	case "WaffoMerchantId":
		setting.WaffoMerchantId = value
	case "WaffoNotifyUrl":
		setting.WaffoNotifyUrl = value
	case "WaffoReturnUrl":
		setting.WaffoReturnUrl = value
	case "WaffoSubscriptionReturnUrl":
		setting.WaffoSubscriptionReturnUrl = value
	case "WaffoCurrency":
		setting.WaffoCurrency = value
	case "WaffoUnitPrice":
		setting.WaffoUnitPrice, _ = strconv.ParseFloat(value, 64)
	case "WaffoMinTopUp":
		setting.WaffoMinTopUp, _ = strconv.Atoi(value)
	case "WaffoPancakeMerchantID":
		setting.WaffoPancakeMerchantID = value
	case "WaffoPancakePrivateKey":
		setting.WaffoPancakePrivateKey = value
	case "WaffoPancakeReturnURL":
		setting.WaffoPancakeReturnURL = value
	case "WaffoPancakeStoreID":
		setting.WaffoPancakeStoreID = value
	case "WaffoPancakeProductID":
		setting.WaffoPancakeProductID = value
	case "WaffoPancakeUnitPrice":
		setting.WaffoPancakeUnitPrice, _ = strconv.ParseFloat(value, 64)
	case "WaffoPancakeMinTopUp":
		setting.WaffoPancakeMinTopUp, _ = strconv.Atoi(value)
	case "TopupGroupRatio":
		err = common.UpdateTopupGroupRatioByJSONString(value)
	case "GitHubClientId":
		common.GitHubClientId = value
	case "GitHubClientSecret":
		common.GitHubClientSecret = value
	case "LinuxDOClientId":
		common.LinuxDOClientId = value
	case "LinuxDOClientSecret":
		common.LinuxDOClientSecret = value
	case "LinuxDOMinimumTrustLevel":
		common.LinuxDOMinimumTrustLevel, _ = strconv.Atoi(value)
	case "Footer":
		common.Footer = value
	case "SystemName":
		common.SystemName = value
	case "Logo":
		common.Logo = value
	case "WeChatServerAddress":
		common.WeChatServerAddress = value
	case "WeChatServerToken":
		common.WeChatServerToken = value
	case "WeChatAccountQRCodeImageURL":
		common.WeChatAccountQRCodeImageURL = value
	case "TelegramBotToken":
		common.TelegramBotToken = value
	case "TelegramBotName":
		common.TelegramBotName = value
	case "TurnstileSiteKey":
		common.TurnstileSiteKey = value
	case "TurnstileSecretKey":
		common.TurnstileSecretKey = value
	case "QuotaForNewUser":
		common.QuotaForNewUser, _ = strconv.Atoi(value)
	case "QuotaForInviter":
		common.QuotaForInviter, _ = strconv.Atoi(value)
	case "QuotaForInvitee":
		common.QuotaForInvitee, _ = strconv.Atoi(value)
	case "QuotaRemindThreshold":
		common.QuotaRemindThreshold, _ = strconv.Atoi(value)
	case "PreConsumedQuota":
		common.PreConsumedQuota, _ = strconv.Atoi(value)
	case "ModelRequestRateLimitCount":
		count, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr != nil {
			err = parseErr
		} else {
			err = setting.SetModelRequestRateLimitCount(count)
		}
	case "ModelRequestRateLimitDurationMinutes":
		duration, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr != nil {
			err = parseErr
		} else {
			err = setting.SetModelRequestRateLimitDurationMinutes(duration)
		}
	case "ModelRequestRateLimitSuccessCount":
		count, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr != nil {
			err = parseErr
		} else {
			err = setting.SetModelRequestRateLimitSuccessCount(count)
		}
	case "ModelRequestRateLimitGroup":
		err = setting.UpdateModelRequestRateLimitGroupByJSONString(value)
	case "RetryTimes":
		common.RetryTimes, _ = strconv.Atoi(value)
	case "DataExportInterval":
		common.DataExportInterval, _ = strconv.Atoi(value)
	case "DataExportDefaultTime":
		common.DataExportDefaultTime = value
	case "ModelRatio":
		err = ratio_setting.UpdateModelRatioByJSONString(value)
	case "GroupRatio":
		err = ratio_setting.UpdateGroupRatioByJSONString(value)
	case "GroupGroupRatio":
		err = ratio_setting.UpdateGroupGroupRatioByJSONString(value)
	case "UserUsableGroups":
		err = setting.UpdateUserUsableGroupsByJSONString(value)
	case "CompletionRatio":
		err = ratio_setting.UpdateCompletionRatioByJSONString(value)
	case "ModelPrice":
		err = ratio_setting.UpdateModelPriceByJSONString(value)
	case "CacheRatio":
		err = ratio_setting.UpdateCacheRatioByJSONString(value)
	case "CreateCacheRatio":
		err = ratio_setting.UpdateCreateCacheRatioByJSONString(value)
	case "ImageRatio":
		err = ratio_setting.UpdateImageRatioByJSONString(value)
	case "AudioRatio":
		err = ratio_setting.UpdateAudioRatioByJSONString(value)
	case "AudioCompletionRatio":
		err = ratio_setting.UpdateAudioCompletionRatioByJSONString(value)
	case "TopUpLink":
		common.TopUpLink = value
	//case "ChatLink":
	//	common.ChatLink = value
	//case "ChatLink2":
	//	common.ChatLink2 = value
	case "ChannelDisableThreshold":
		common.ChannelDisableThreshold, _ = strconv.ParseFloat(value, 64)
	case "QuotaPerUnit":
		common.QuotaPerUnit, _ = strconv.ParseFloat(value, 64)
	case "SensitiveWords":
		setting.SensitiveWordsFromString(value)
	case "AutomaticDisableKeywords":
		operation_setting.AutomaticDisableKeywordsFromString(value)
	case "AutomaticDisableStatusCodes":
		err = operation_setting.AutomaticDisableStatusCodesFromString(value)
	case "AutomaticRetryStatusCodes":
		err = operation_setting.AutomaticRetryStatusCodesFromString(value)
	case "StreamCacheQueueLength":
		setting.StreamCacheQueueLength, _ = strconv.Atoi(value)
	case "PayMethods":
		err = operation_setting.UpdatePayMethodsByJsonString(value)
	case "WaffoPayMethods":
		// WaffoPayMethods is read directly from OptionMap via setting.GetWaffoPayMethods().
		// The value is already stored in OptionMap at the top of this function (line: common.OptionMap[key] = value).
		// No additional in-memory variable to update.
	}
	if key == common.ChannelQuotaAlertEnabledOptionKey || key == common.ChannelQuotaAlertWarningPercentOptionKey || key == common.ChannelQuotaAlertCriticalPercentOptionKey {
		if settingsJSON, marshalErr := common.MarshalChannelQuotaAlertSettings(common.ChannelQuotaAlertSettings{
			Enabled:          common.ChannelQuotaAlertEnabled,
			WarningPercent:   common.ChannelQuotaAlertWarningPercent,
			CriticalPercent:  common.ChannelQuotaAlertCriticalPercent,
			CooldownSeconds:  common.ChannelQuotaAlertCooldownSeconds,
			NotifyOnRecovery: common.ChannelQuotaAlertNotifyOnRecovery,
		}); marshalErr == nil {
			common.OptionMap[common.ChannelQuotaAlertSettingsOptionKey] = settingsJSON
		}
	}
	return err
}

func normalizeOptionValue(key, value string) (string, error) {
	if pair, ok := groupRatioOptionPairForKey(key); ok {
		return normalizeGroupRatioOptionValue(pair, value)
	}
	if key == "access_profile_setting.profiles" {
		return setting.NormalizeAccessProfileDefinitionsJSON(value)
	}
	if strings.TrimSpace(value) != "null" {
		return value, nil
	}
	switch key {
	case "Chats", "AutoGroups", "PayMethods":
		return "[]", nil
	case "UserUsableGroups":
		return "{}", nil
	default:
		return value, nil
	}
}

func groupRatioOptionPairForKey(key string) (groupRatioOptionPair, bool) {
	for _, pair := range groupRatioOptionPairs {
		if key == pair.canonical || key == pair.alias {
			return pair, true
		}
	}
	return groupRatioOptionPair{}, false
}

func normalizeGroupRatioOptionValue(pair groupRatioOptionPair, value string) (string, error) {
	if pair.canonical == groupRatioOptionKey {
		if err := ratio_setting.ValidateRatioMapJSON(value); err != nil {
			return "", err
		}
		var ratios map[string]float64
		if err := common.Unmarshal([]byte(value), &ratios); err != nil {
			return "", err
		}
		for key, ratio := range ratios {
			if ratio == 0 {
				ratios[key] = 0
			}
		}
		encoded, err := common.Marshal(ratios)
		return string(encoded), err
	}
	if err := ratio_setting.ValidateNestedRatioMapJSON(value); err != nil {
		return "", err
	}
	var ratios map[string]map[string]float64
	if err := common.Unmarshal([]byte(value), &ratios); err != nil {
		return "", err
	}
	for _, nested := range ratios {
		for key, ratio := range nested {
			if ratio == 0 {
				nested[key] = 0
			}
		}
	}
	encoded, err := common.Marshal(ratios)
	return string(encoded), err
}

func publishGroupRatioOptionPair(pair groupRatioOptionPair, value string) error {
	value, err := normalizeGroupRatioOptionValue(pair, value)
	if err != nil {
		return err
	}
	if pair.canonical == groupRatioOptionKey {
		err = ratio_setting.UpdateGroupRatioByJSONString(value)
	} else {
		err = ratio_setting.UpdateGroupGroupRatioByJSONString(value)
	}
	if err != nil {
		return err
	}
	common.OptionMapRWMutex.Lock()
	common.OptionMap[pair.canonical] = value
	common.OptionMap[pair.alias] = value
	common.OptionMapRWMutex.Unlock()
	return nil
}

func publishLoadedGroupRatioOptionPair(pair groupRatioOptionPair, values map[string]string) {
	canonicalValue, hasCanonical := values[pair.canonical]
	aliasValue, hasAlias := values[pair.alias]

	normalizedCanonical, canonicalErr := "", error(nil)
	if hasCanonical {
		normalizedCanonical, canonicalErr = normalizeGroupRatioOptionValue(pair, canonicalValue)
	}
	normalizedAlias, aliasErr := "", error(nil)
	if hasAlias {
		normalizedAlias, aliasErr = normalizeGroupRatioOptionValue(pair, aliasValue)
	}

	selected := ""
	switch {
	case hasCanonical && canonicalErr == nil:
		selected = normalizedCanonical
		if hasAlias && aliasErr != nil {
			common.SysLog(fmt.Sprintf("warning: invalid group ratio option alias %s: %v; valid canonical %s remains authoritative", pair.alias, aliasErr, pair.canonical))
		} else if hasAlias && normalizedCanonical != normalizedAlias {
			common.SysLog(fmt.Sprintf("warning: %s: valid canonical %s takes precedence over %s", groupRatioAliasConflictLogText, pair.canonical, pair.alias))
		}
	case hasCanonical && canonicalErr != nil && hasAlias && aliasErr == nil:
		selected = normalizedAlias
		common.SysLog(fmt.Sprintf("warning: invalid canonical group ratio option %s: %v; falling back to valid alias %s", pair.canonical, canonicalErr, pair.alias))
	case hasCanonical && canonicalErr != nil && hasAlias:
		common.SysLog(fmt.Sprintf("warning: both group ratio option aliases for %s are invalid; retaining last valid runtime value (canonical: %v; alias: %v)", pair.canonical, canonicalErr, aliasErr))
	case hasCanonical && canonicalErr != nil:
		common.SysLog(fmt.Sprintf("warning: invalid canonical group ratio option %s: %v; retaining last valid runtime value", pair.canonical, canonicalErr))
	case hasAlias && aliasErr == nil:
		selected = normalizedAlias
	case hasAlias:
		common.SysLog(fmt.Sprintf("warning: invalid group ratio option alias %s: %v; retaining last valid runtime value", pair.alias, aliasErr))
	}

	if selected == "" {
		if pair.canonical == groupRatioOptionKey {
			selected = ratio_setting.GroupRatio2JSONString()
		} else {
			selected = ratio_setting.GroupGroupRatio2JSONString()
		}
	}
	if err := publishGroupRatioOptionPair(pair, selected); err != nil {
		common.SysLog(fmt.Sprintf("failed to publish last valid %s runtime snapshot: %v", pair.canonical, err))
	}
}

// handleConfigUpdate 处理分层配置更新，返回是否已处理
func handleConfigUpdate(key, value string) (bool, error) {
	if key == operation_setting.ToolPriceOptionKey {
		operation_setting.LoadToolPricesFromJSONString(value)
		return true, nil
	}

	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return false, nil // 不是分层配置
	}

	configName := parts[0]
	configKey := parts[1]

	// 获取配置对象
	cfg := config.GlobalConfig.Get(configName)
	if cfg == nil {
		return false, nil // 未注册的配置
	}

	// 更新配置
	configMap := map[string]string{
		configKey: value,
	}
	if err := config.UpdateConfigFromMap(cfg, configMap); err != nil {
		return true, err
	}

	// 特定配置的后处理
	if configName == "performance_setting" {
		performance_setting.UpdateAndSync()
	} else if configName == "billing_setting" {
		InvalidatePricingCache()
		ratio_setting.InvalidateExposedDataCache()
	}

	return true, nil // 已处理
}
