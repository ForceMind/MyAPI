package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relay/channel/advancedcustom"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/operation_setting"

	"github.com/shopspring/decimal"

	"github.com/gin-gonic/gin"
)

// Preserve the established channel-balance billing behavior for compatibility.

type OpenAISubscriptionResponse struct {
	Object             string  `json:"object"`
	HasPaymentMethod   bool    `json:"has_payment_method"`
	SoftLimitUSD       float64 `json:"soft_limit_usd"`
	HardLimitUSD       float64 `json:"hard_limit_usd"`
	SystemHardLimitUSD float64 `json:"system_hard_limit_usd"`
	AccessUntil        int64   `json:"access_until"`
}

type OpenAIUsageDailyCost struct {
	Timestamp float64 `json:"timestamp"`
	LineItems []struct {
		Name string  `json:"name"`
		Cost float64 `json:"cost"`
	}
}

type OpenAICreditGrants struct {
	Object         string  `json:"object"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
	TotalAvailable float64 `json:"total_available"`
}

const maxChannelBalanceResponseBytes = 256 << 10
const maxAdvancedCustomBalanceResponseBytes = maxChannelBalanceResponseBytes

// errChannelQuotaUnsupported distinguishes a provider that has no supported
// balance endpoint from a transient query failure. Unsupported is persisted as
// a first-class status so operators are not told to repair a healthy channel.
var errChannelQuotaUnsupported = errors.New("channel quota unsupported")

// Balance endpoints are provider-controlled and some providers do not close
// stalled connections promptly.  A bounded request context keeps the
// scheduled sampler from accumulating goroutines while preserving the
// existing RelayTimeout setting as an additional client-level limit.
const channelBalanceRequestTimeout = 30 * time.Second

type channelBalanceResult struct {
	Balance     float64
	RawResponse string
}

type OpenAIUsageResponse struct {
	Object string `json:"object"`
	//DailyCosts []OpenAIUsageDailyCost `json:"daily_costs"`
	TotalUsage float64 `json:"total_usage"` // unit: 0.01 dollar
}

type OpenAISBUsageResponse struct {
	Msg  string `json:"msg"`
	Data *struct {
		Credit string `json:"credit"`
	} `json:"data"`
}

type AIProxyUserOverviewResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	ErrorCode int    `json:"error_code"`
	Data      struct {
		TotalPoints float64 `json:"totalPoints"`
	} `json:"data"`
}

type API2GPTUsageResponse struct {
	Object         string  `json:"object"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
	TotalRemaining float64 `json:"total_remaining"`
}

type APGC2DGPTUsageResponse struct {
	//Grants         interface{} `json:"grants"`
	Object         string  `json:"object"`
	TotalAvailable float64 `json:"total_available"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
}

type SiliconFlowUsageResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  bool   `json:"status"`
	Data    struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Image         string `json:"image"`
		Email         string `json:"email"`
		IsAdmin       bool   `json:"isAdmin"`
		Balance       string `json:"balance"`
		Status        string `json:"status"`
		Introduction  string `json:"introduction"`
		Role          string `json:"role"`
		ChargeBalance string `json:"chargeBalance"`
		TotalBalance  string `json:"totalBalance"`
		Category      string `json:"category"`
	} `json:"data"`
}

type DeepSeekUsageResponse struct {
	IsAvailable  bool `json:"is_available"`
	BalanceInfos []struct {
		Currency        string `json:"currency"`
		TotalBalance    string `json:"total_balance"`
		GrantedBalance  string `json:"granted_balance"`
		ToppedUpBalance string `json:"topped_up_balance"`
	} `json:"balance_infos"`
}

type OpenRouterCreditResponse struct {
	Data struct {
		TotalCredits float64 `json:"total_credits"`
		TotalUsage   float64 `json:"total_usage"`
	} `json:"data"`
}

// GetAuthHeader get auth header
func GetAuthHeader(token string) http.Header {
	h := http.Header{}
	h.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	return h
}

// GetClaudeAuthHeader get claude auth header
func GetClaudeAuthHeader(token string) http.Header {
	h := http.Header{}
	h.Add("x-api-key", token)
	h.Add("anthropic-version", "2023-06-01")
	return h
}

func GetResponseBody(ctx context.Context, method, url string, channel *model.Channel, headers http.Header) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	for k := range headers {
		req.Header.Add(k, headers.Get(k))
	}
	client, err := service.GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxChannelBalanceResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxChannelBalanceResponseBytes {
		return nil, fmt.Errorf("balance response exceeds %d bytes", maxChannelBalanceResponseBytes)
	}
	return body, nil
}

func updateChannelCloseAIBalance(ctx context.Context, channel *model.Channel) (float64, error) {
	url := fmt.Sprintf("%s/dashboard/billing/credit_grants", channel.GetBaseURL())
	body, err := GetResponseBody(ctx, "GET", url, channel, GetAuthHeader(channel.Key))

	if err != nil {
		return 0, err
	}
	response := OpenAICreditGrants{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	return response.TotalAvailable, nil
}

func updateChannelOpenAISBBalance(ctx context.Context, channel *model.Channel) (float64, error) {
	url := fmt.Sprintf("https://api.openai-sb.com/sb-api/user/status?api_key=%s", channel.Key)
	body, err := GetResponseBody(ctx, "GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := OpenAISBUsageResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if response.Data == nil {
		return 0, errors.New(response.Msg)
	}
	balance, err := strconv.ParseFloat(response.Data.Credit, 64)
	if err != nil {
		return 0, err
	}
	return balance, nil
}

func updateChannelAIProxyBalance(ctx context.Context, channel *model.Channel) (float64, error) {
	url := "https://aiproxy.io/api/report/getUserOverview"
	headers := http.Header{}
	headers.Add("Api-Key", channel.Key)
	body, err := GetResponseBody(ctx, "GET", url, channel, headers)
	if err != nil {
		return 0, err
	}
	response := AIProxyUserOverviewResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if !response.Success {
		return 0, fmt.Errorf("code: %d, message: %s", response.ErrorCode, response.Message)
	}
	return response.Data.TotalPoints, nil
}

func updateChannelAPI2GPTBalance(ctx context.Context, channel *model.Channel) (float64, error) {
	url := "https://api.api2gpt.com/dashboard/billing/credit_grants"
	body, err := GetResponseBody(ctx, "GET", url, channel, GetAuthHeader(channel.Key))

	if err != nil {
		return 0, err
	}
	response := API2GPTUsageResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	return response.TotalRemaining, nil
}

func updateChannelSiliconFlowBalance(ctx context.Context, channel *model.Channel) (float64, error) {
	url := "https://api.siliconflow.cn/v1/user/info"
	body, err := GetResponseBody(ctx, "GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := SiliconFlowUsageResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if response.Code != 20000 {
		return 0, fmt.Errorf("code: %d, message: %s", response.Code, response.Message)
	}
	balance, err := strconv.ParseFloat(response.Data.TotalBalance, 64)
	if err != nil {
		return 0, err
	}
	return balance, nil
}

func updateChannelDeepSeekBalance(ctx context.Context, channel *model.Channel) (float64, error) {
	url := "https://api.deepseek.com/user/balance"
	body, err := GetResponseBody(ctx, "GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := DeepSeekUsageResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	index := -1
	for i, balanceInfo := range response.BalanceInfos {
		if balanceInfo.Currency == "CNY" {
			index = i
			break
		}
	}
	if index == -1 {
		return 0, errors.New("currency CNY not found")
	}
	balance, err := strconv.ParseFloat(response.BalanceInfos[index].TotalBalance, 64)
	if err != nil {
		return 0, err
	}
	return balance, nil
}

func updateChannelAIGC2DBalance(ctx context.Context, channel *model.Channel) (float64, error) {
	url := "https://api.aigc2d.com/dashboard/billing/credit_grants"
	body, err := GetResponseBody(ctx, "GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := APGC2DGPTUsageResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	return response.TotalAvailable, nil
}

func updateChannelOpenRouterBalance(ctx context.Context, channel *model.Channel) (float64, error) {
	url := "https://openrouter.ai/api/v1/credits"
	body, err := GetResponseBody(ctx, "GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := OpenRouterCreditResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	balance := response.Data.TotalCredits - response.Data.TotalUsage
	return balance, nil
}

func updateChannelMoonshotBalance(ctx context.Context, channel *model.Channel) (float64, error) {
	url := "https://api.moonshot.cn/v1/users/me/balance"
	body, err := GetResponseBody(ctx, "GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}

	type MoonshotBalanceData struct {
		AvailableBalance float64 `json:"available_balance"`
		VoucherBalance   float64 `json:"voucher_balance"`
		CashBalance      float64 `json:"cash_balance"`
	}

	type MoonshotBalanceResponse struct {
		Code   int                 `json:"code"`
		Data   MoonshotBalanceData `json:"data"`
		Scode  string              `json:"scode"`
		Status bool                `json:"status"`
	}

	response := MoonshotBalanceResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if !response.Status || response.Code != 0 {
		return 0, fmt.Errorf("failed to update moonshot balance, status: %v, code: %d, scode: %s", response.Status, response.Code, response.Scode)
	}
	availableBalanceCny := response.Data.AvailableBalance
	availableBalanceUsd := decimal.NewFromFloat(availableBalanceCny).Div(decimal.NewFromFloat(operation_setting.Price)).InexactFloat64()
	return availableBalanceUsd, nil
}

func fetchAdvancedCustomBalance(ctx context.Context, channel *model.Channel) (channelBalanceResult, error) {
	key := strings.TrimSpace(channel.Key)
	info := &relaycommon.RelayInfo{
		RelayFormat:    types.RelayFormatOpenAI,
		RelayMode:      relayconstant.RelayModeUnknown,
		RequestURLPath: dto.AdvancedCustomBalancePath,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:          constant.ChannelTypeAdvancedCustom,
			ChannelBaseUrl:       channel.GetBaseURL(),
			ApiKey:               key,
			ChannelOtherSettings: channel.GetOtherSettings(),
		},
	}
	requestURL, headers, err := (&advancedcustom.Adaptor{}).BuildBalanceRequest(info)
	if err != nil {
		return channelBalanceResult{}, sanitizeFetchModelsError(err, key)
	}
	if err := applyFetchModelsHeaderOverrides(channel, key, headers); err != nil {
		return channelBalanceResult{}, sanitizeFetchModelsError(err, key)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return channelBalanceResult{}, sanitizeFetchModelsError(err, key)
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
		if strings.EqualFold(name, "Host") {
			request.Host = headers.Get(name)
		}
	}
	client, err := service.GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return channelBalanceResult{}, sanitizeFetchModelsError(err, key)
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return channelBalanceResult{}, context.DeadlineExceeded
		}
		if errors.Is(err, context.Canceled) {
			return channelBalanceResult{}, context.Canceled
		}
		return channelBalanceResult{}, sanitizeAdvancedCustomRequestError(err, key, requestURL)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return channelBalanceResult{}, fmt.Errorf("status code: %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAdvancedCustomBalanceResponseBytes+1))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return channelBalanceResult{}, context.DeadlineExceeded
		}
		if errors.Is(err, context.Canceled) {
			return channelBalanceResult{}, context.Canceled
		}
		return channelBalanceResult{}, sanitizeAdvancedCustomRequestError(err, key, requestURL)
	}
	if len(body) > maxAdvancedCustomBalanceResponseBytes {
		return channelBalanceResult{}, fmt.Errorf("balance response exceeds %d bytes", maxAdvancedCustomBalanceResponseBytes)
	}

	var validated json.RawMessage
	if err := common.Unmarshal(body, &validated); err != nil {
		return channelBalanceResult{}, fmt.Errorf("invalid balance JSON response: %w", err)
	}
	if common.GetJsonType(validated) == "object" {
		var creditSummary struct {
			Object         string          `json:"object"`
			TotalAvailable json.RawMessage `json:"total_available"`
		}
		if err := common.Unmarshal(body, &creditSummary); err != nil {
			return channelBalanceResult{}, fmt.Errorf("invalid balance JSON response: %w", err)
		}
		if creditSummary.Object == "credit_summary" &&
			common.GetJsonType(creditSummary.TotalAvailable) == "number" {
			var balance float64
			if err := common.Unmarshal(creditSummary.TotalAvailable, &balance); err == nil &&
				balance >= 0 &&
				!math.IsNaN(balance) &&
				!math.IsInf(balance, 0) {
				return channelBalanceResult{Balance: balance}, nil
			}
		}
	}

	formatted, err := common.IndentJson(body)
	if err != nil {
		return channelBalanceResult{}, fmt.Errorf("invalid balance JSON response: %w", err)
	}
	return channelBalanceResult{RawResponse: string(formatted)}, nil
}

func updateChannelBalance(channel *model.Channel) (channelBalanceResult, error) {
	return updateChannelBalanceWithContext(context.Background(), channel)
}

func updateChannelBalanceWithContext(ctx context.Context, channel *model.Channel) (channelBalanceResult, error) {
	if channel == nil {
		return channelBalanceResult{}, errors.New("nil channel")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, channelBalanceRequestTimeout)
	defer cancel()
	var result channelBalanceResult
	var err error
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		result, err = fetchAdvancedCustomBalance(ctx, channel)
	} else {
		result.Balance, err = updateStandardChannelBalance(ctx, channel)
	}
	if err != nil || result.RawResponse != "" {
		return result, err
	}
	persistCtx, persistCancel := context.WithTimeout(context.Background(), channelQuotaPersistenceTimeout)
	defer persistCancel()
	return result, newChannelQuotaSamplingError(nil, channel.UpdateBalanceWithContext(persistCtx, result.Balance))
}

// withChannelPollingLock runs one channel operation while holding the same
// non-reentrant lock used by the scheduled sampler and multi-key state flow.
// Keeping the operation as a callback makes it explicit that all related work
// (provider query and normalized snapshot persistence) shares one critical
// section.
func withChannelPollingLock(channelID int, operation func()) {
	lock := model.GetChannelPollingLock(channelID)
	lock.Lock()
	defer lock.Unlock()
	operation()
}

// updateChannelBalanceWithPollingLock serializes balance requests initiated
// outside the scheduled sampler and persists their normalized observation
// before releasing the lock. The sampler already owns the channel lock while
// it invokes updateChannelBalance and recordChannelBalanceSnapshot, so it must
// continue to call those raw helpers directly rather than this wrapper
// (otherwise a non-reentrant mutex would deadlock).
func updateChannelBalanceWithPollingLock(channel *model.Channel) (result channelBalanceResult, queryErr, persistErr error) {
	if channel == nil {
		return channelBalanceResult{}, errors.New("nil channel"), nil
	}
	withChannelPollingLock(channel.Id, func() {
		result, queryErr = updateChannelBalance(channel)
		var classified *channelQuotaSamplingError
		if errors.As(queryErr, &classified) {
			queryErr, persistErr = classified.QueryErr, classified.PersistErr
		}
		persistErr = errors.Join(persistErr, recordChannelBalanceSnapshot(channel, result, queryErr))
	})
	return result, queryErr, persistErr
}

// recordChannelBalanceSnapshot persists only the normalized result of a
// balance query. Upstream response bodies and credentials are never written
// to the quota history table. The persistence error is returned so scheduled
// sampling can report a history write failure without changing the existing
// manual balance response semantics (manual callers intentionally ignore it).
func recordChannelBalanceSnapshot(channel *model.Channel, result channelBalanceResult, queryErr error) error {
	if channel == nil {
		return nil
	}
	snapshot := &model.ChannelQuotaSnapshot{
		ChannelId:  channel.Id,
		ObservedAt: time.Now().Unix(),
		Unit:       "usd",
		MetricType: "balance",
		WindowType: "none",
		Source:     fmt.Sprintf("channel_type_%d", channel.Type),
	}
	if queryErr != nil {
		if errors.Is(queryErr, errChannelQuotaUnsupported) {
			snapshot.Status = "unsupported"
			snapshot.ErrorCode = "quota_unsupported"
			snapshot.ErrorMessage = "provider does not expose a supported balance endpoint"
		} else if errors.Is(queryErr, context.DeadlineExceeded) {
			snapshot.Status = "error"
			snapshot.ErrorCode = "upstream_timeout"
			snapshot.ErrorMessage = "quota sampling request timed out"
		} else if errors.Is(queryErr, context.Canceled) {
			snapshot.Status = "error"
			snapshot.ErrorCode = "sampling_canceled"
			snapshot.ErrorMessage = "quota sampling request canceled"
		} else {
			snapshot.Status = "error"
			snapshot.ErrorCode = "query_failed"
			snapshot.ErrorMessage = "balance query failed"
		}
		return recordQuotaSamplingSnapshots([]model.ChannelQuotaSnapshot{*snapshot})
	}
	if result.RawResponse != "" {
		snapshot.Status = "unsupported"
		snapshot.ErrorCode = "unstructured_response"
		snapshot.ErrorMessage = "upstream did not return a numeric balance"
		return recordQuotaSamplingSnapshots([]model.ChannelQuotaSnapshot{*snapshot})
	}
	if math.IsNaN(result.Balance) || math.IsInf(result.Balance, 0) {
		snapshot.Status = "error"
		snapshot.ErrorCode = "invalid_balance"
		snapshot.ErrorMessage = "upstream returned an invalid balance"
		return recordQuotaSamplingSnapshots([]model.ChannelQuotaSnapshot{*snapshot})
	}
	snapshot.Available = result.Balance
	return recordQuotaSamplingSnapshots([]model.ChannelQuotaSnapshot{*snapshot})
}

func updateStandardChannelBalance(ctx context.Context, channel *model.Channel) (float64, error) {
	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() == "" {
		channel.BaseURL = &baseURL
	}
	switch channel.Type {
	case constant.ChannelTypeOpenAI:
		if channel.GetBaseURL() != "" {
			baseURL = channel.GetBaseURL()
		}
	case constant.ChannelTypeAzure:
		return 0, errChannelQuotaUnsupported
	case constant.ChannelTypeCustom:
		baseURL = channel.GetBaseURL()
	//case common.ChannelTypeOpenAISB:
	//	return updateChannelOpenAISBBalance(ctx, channel)
	case constant.ChannelTypeAIProxy:
		return updateChannelAIProxyBalance(ctx, channel)
	case constant.ChannelTypeAPI2GPT:
		return updateChannelAPI2GPTBalance(ctx, channel)
	case constant.ChannelTypeAIGC2D:
		return updateChannelAIGC2DBalance(ctx, channel)
	case constant.ChannelTypeSiliconFlow:
		return updateChannelSiliconFlowBalance(ctx, channel)
	case constant.ChannelTypeDeepSeek:
		return updateChannelDeepSeekBalance(ctx, channel)
	case constant.ChannelTypeOpenRouter:
		return updateChannelOpenRouterBalance(ctx, channel)
	case constant.ChannelTypeMoonshot:
		return updateChannelMoonshotBalance(ctx, channel)
	default:
		return 0, errChannelQuotaUnsupported
	}
	url := fmt.Sprintf("%s/v1/dashboard/billing/subscription", baseURL)

	body, err := GetResponseBody(ctx, "GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	subscription := OpenAISubscriptionResponse{}
	err = common.Unmarshal(body, &subscription)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	startDate := fmt.Sprintf("%s-01", now.Format("2006-01"))
	endDate := now.Format("2006-01-02")
	if !subscription.HasPaymentMethod {
		startDate = now.AddDate(0, 0, -100).Format("2006-01-02")
	}
	url = fmt.Sprintf("%s/v1/dashboard/billing/usage?start_date=%s&end_date=%s", baseURL, startDate, endDate)
	body, err = GetResponseBody(ctx, "GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	usage := OpenAIUsageResponse{}
	err = common.Unmarshal(body, &usage)
	if err != nil {
		return 0, err
	}
	balance := subscription.HardLimitUSD - usage.TotalUsage/100
	return balance, nil
}

func UpdateChannelBalance(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "多密钥渠道不支持余额查询",
		})
		return
	}
	result, err, _ := updateChannelBalanceWithPollingLock(channel)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response := gin.H{
		"success": true,
		"message": "",
	}
	if result.RawResponse == "" {
		response["balance"] = result.Balance
	} else {
		response["raw_response"] = result.RawResponse
	}
	c.JSON(http.StatusOK, response)
}

type quotaHistoryGranularity string

const (
	quotaHistoryRaw                quotaHistoryGranularity = "raw"
	quotaHistoryMinute             quotaHistoryGranularity = "minute"
	quotaHistoryFiveMinutes        quotaHistoryGranularity = "5m"
	quotaHistoryFifteenMinutes     quotaHistoryGranularity = "15m"
	quotaHistoryHour               quotaHistoryGranularity = "hour"
	quotaHistoryDay                quotaHistoryGranularity = "day"
	quotaHistoryWeek               quotaHistoryGranularity = "week"
	quotaHistoryAuto               quotaHistoryGranularity = "auto"
	maxQuotaHistoryPointLimit                              = 5000
	maxQuotaHistoryRawObservations                         = 150000
)

func parseQuotaHistoryGranularity(value string, start, end int64) (quotaHistoryGranularity, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return quotaHistoryRaw, nil
	}
	if value == string(quotaHistoryAuto) {
		switch {
		case end-start <= 6*60*60:
			return quotaHistoryMinute, nil
		case end-start <= 24*60*60:
			return quotaHistoryFiveMinutes, nil
		case end-start <= 7*24*60*60:
			return quotaHistoryFifteenMinutes, nil
		case end-start <= 30*24*60*60:
			return quotaHistoryHour, nil
		case end-start <= 90*24*60*60:
			return quotaHistoryDay, nil
		default:
			return quotaHistoryWeek, nil
		}
	}
	granularity := quotaHistoryGranularity(value)
	switch granularity {
	case quotaHistoryRaw, quotaHistoryMinute, quotaHistoryFiveMinutes, quotaHistoryFifteenMinutes,
		quotaHistoryHour, quotaHistoryDay, quotaHistoryWeek:
		return granularity, nil
	default:
		return "", errors.New("invalid granularity; use raw, minute, 5m, 15m, hour, day, week, or auto")
	}
}

func parseQuotaHistoryTimezoneOffset(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(value)
	if err != nil || offset < -840 || offset > 840 {
		return 0, errors.New("timezone_offset must be minutes between -840 and 840")
	}
	return offset, nil
}

func parseQuotaHistoryWindowSeconds(value string) (*int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds < 0 {
		return nil, errors.New("window_seconds must be a non-negative integer")
	}
	return &seconds, nil
}

func parseQuotaAnalysisDuration(value string, maximum int64, field string) (int64, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return maximum, nil
	}
	presets := map[string]int64{
		"1m":  60,
		"5m":  5 * 60,
		"15m": 15 * 60,
		"1h":  60 * 60,
		"6h":  6 * 60 * 60,
		"24h": 24 * 60 * 60,
		"1d":  24 * 60 * 60,
		"7d":  7 * 24 * 60 * 60,
		"30d": 30 * 24 * 60 * 60,
		"90d": 90 * 24 * 60 * 60,
	}
	seconds, ok := presets[value]
	if !ok {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s must be seconds or a supported preset", field)
		}
		seconds = parsed
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("%s must be positive", field)
	}
	if seconds > 180*24*60*60 {
		return 0, fmt.Errorf("%s cannot exceed 180 days", field)
	}
	if maximum <= 0 || seconds > maximum {
		return 0, fmt.Errorf("%s cannot exceed the selected range", field)
	}
	return seconds, nil
}

func defaultQuotaAnalysisHalfLife(rateWindowSeconds int64) int64 {
	if rateWindowSeconds <= 2 {
		return 1
	}
	return rateWindowSeconds / 2
}

func resolveQuotaHistorySeriesFilter(filter model.ChannelQuotaSnapshotQuery, latest model.ChannelQuotaSnapshot) model.ChannelQuotaSnapshotQuery {
	filter.ExactIdentity = true
	if filter.MetricType == "" {
		filter.MetricType = latest.MetricType
	}
	if filter.WindowType == "" {
		filter.WindowType = latest.WindowType
	}
	if filter.Source == "" {
		filter.Source = latest.Source
	}
	if filter.PlanType == "" {
		filter.PlanType = latest.PlanType
	}
	if filter.Unit == "" {
		filter.Unit = latest.Unit
	}
	if filter.Currency == "" {
		filter.Currency = latest.Currency
	}
	if filter.WindowSeconds == nil {
		windowSeconds := latest.WindowSeconds
		filter.WindowSeconds = &windowSeconds
	}
	return filter
}

func quotaHistoryBucketStart(timestamp int64, granularity quotaHistoryGranularity, timezoneOffset int) int64 {
	local := time.Unix(timestamp, 0).UTC().Add(time.Duration(timezoneOffset) * time.Minute)
	var bucket time.Time
	switch granularity {
	case quotaHistoryMinute:
		bucket = local.Truncate(time.Minute)
	case quotaHistoryFiveMinutes:
		bucket = local.Truncate(5 * time.Minute)
	case quotaHistoryFifteenMinutes:
		bucket = local.Truncate(15 * time.Minute)
	case quotaHistoryHour:
		bucket = local.Truncate(time.Hour)
	case quotaHistoryDay:
		bucket = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	case quotaHistoryWeek:
		day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
		daysSinceMonday := (int(day.Weekday()) + 6) % 7
		bucket = day.AddDate(0, 0, -daysSinceMonday)
	default:
		return timestamp
	}
	return bucket.Add(-time.Duration(timezoneOffset) * time.Minute).Unix()
}

// aggregateQuotaHistorySnapshots keeps the actual latest observation in each
// bucket. In particular, a later failed sample is never replaced by an older
// success: consumers need to know that the newest provider observation did
// not yield a numeric value. The history endpoint adds bucket counts and
// continuity markers on top of this legacy helper.
func aggregateQuotaHistorySnapshots(snapshots []model.ChannelQuotaSnapshot, granularity quotaHistoryGranularity, timezoneOffset int) []model.ChannelQuotaSnapshot {
	if granularity == quotaHistoryRaw || len(snapshots) < 2 {
		return snapshots
	}
	aggregated := make([]model.ChannelQuotaSnapshot, 0, len(snapshots))
	// A time bucket can contain multiple independent provider series (for
	// example Codex primary/secondary windows or a plan/unit transition). Keep
	// those series separate while aggregating so one observation cannot replace
	// another merely because they share a timestamp bucket.
	type seriesKey struct {
		Bucket        int64
		MetricType    string
		WindowType    string
		Source        string
		PlanType      string
		Unit          string
		Currency      string
		WindowSeconds int64
	}
	indices := make(map[seriesKey]int, len(snapshots))
	for _, snapshot := range snapshots {
		bucket := quotaHistoryBucketStart(snapshot.ObservedAt, granularity, timezoneOffset)
		snapshot.ObservedAt = bucket
		key := seriesKey{
			Bucket:        bucket,
			MetricType:    snapshot.MetricType,
			WindowType:    snapshot.WindowType,
			Source:        snapshot.Source,
			PlanType:      snapshot.PlanType,
			Unit:          snapshot.Unit,
			Currency:      snapshot.Currency,
			WindowSeconds: snapshot.WindowSeconds,
		}
		if index, ok := indices[key]; ok {
			aggregated[index] = snapshot
			continue
		}
		indices[key] = len(aggregated)
		aggregated = append(aggregated, snapshot)
	}
	return aggregated
}

const (
	quotaHistoryMinimumRateSpanSeconds = int64(time.Hour / time.Second)
	quotaHistoryForecastMaximumDays    = 365 * 100
)

// quotaHistoryDataQuality describes the observations used by the derived
// metrics. Invalid numeric values are counted separately and never participate
// in rate or forecast calculations.
type quotaHistoryDataQuality struct {
	SampleCount         int   `json:"sample_count"`
	SuccessCount        int   `json:"success_count"`
	ErrorCount          int   `json:"error_count"`
	UnsupportedCount    int   `json:"unsupported_count,omitempty"`
	InvalidCount        int   `json:"invalid_count"`
	ResetBoundaries     int   `json:"reset_boundaries"`
	ObservedSpanSeconds int64 `json:"observed_span_seconds"`
	SpanSeconds         int64 `json:"span_seconds"`
}

type quotaHistoryDerivedMetrics struct {
	DropRatePerDay     *float64                `json:"drop_rate_per_day"`
	ForecastZeroAt     *int64                  `json:"forecast_zero_at"`
	ForecastConfidence string                  `json:"forecast_confidence"`
	DataQuality        quotaHistoryDataQuality `json:"data_quality"`
}

type quotaHistoryAlert struct {
	Enabled          bool     `json:"enabled"`
	Status           string   `json:"status"`
	RatioPercent     *float64 `json:"ratio_percent,omitempty"`
	WarningPercent   float64  `json:"warning_percent"`
	CriticalPercent  float64  `json:"critical_percent"`
	CooldownSeconds  int64    `json:"cooldown_seconds"`
	NotifyOnRecovery bool     `json:"notify_on_recovery"`
}

// deriveQuotaHistoryAlert exposes an opt-in, read-only threshold status. It
// intentionally does not send notifications, disable channels, or alter
// routing. Without a provider-reported total quota the ratio is undefined and
// the status remains unavailable, avoiding misleading currency-specific
// absolute thresholds.
func deriveQuotaHistoryAlert(snapshot *model.ChannelQuotaSnapshot) quotaHistoryAlert {
	alert := quotaHistoryAlert{
		Enabled:          common.ChannelQuotaAlertEnabled,
		WarningPercent:   common.ChannelQuotaAlertWarningPercent,
		CriticalPercent:  common.ChannelQuotaAlertCriticalPercent,
		CooldownSeconds:  common.ChannelQuotaAlertCooldownSeconds,
		NotifyOnRecovery: common.ChannelQuotaAlertNotifyOnRecovery,
		Status:           "disabled",
	}
	if !alert.Enabled {
		return alert
	}
	alert.Status = "unavailable"
	if snapshot == nil || !service.QuotaSnapshotUsable(*snapshot) || snapshot.Total == nil ||
		!finiteQuotaValue(snapshot.Available) || !finiteQuotaValue(*snapshot.Total) || *snapshot.Total <= 0 {
		return alert
	}
	ratio := snapshot.Available / *snapshot.Total * 100
	if !finiteQuotaValue(ratio) {
		return alert
	}
	alert.RatioPercent = &ratio
	switch {
	case ratio <= alert.CriticalPercent:
		alert.Status = "critical"
	case ratio <= alert.WarningPercent:
		alert.Status = "warning"
	default:
		alert.Status = "healthy"
	}
	return alert
}

func finiteQuotaValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// deriveQuotaHistoryMetrics computes conservative, read-only trend indicators.
// A reset, failed observation, unsupported observation, or invalid numeric
// observation breaks the rate segment. This prevents a forecast from bridging
// an unobserved period and presenting it as continuous provider data.
func deriveQuotaHistoryMetrics(snapshots []model.ChannelQuotaSnapshot) quotaHistoryDerivedMetrics {
	return quotaHistoryMetricsFromConsumption(service.DeriveQuotaConsumption(snapshots, ""))
}

func quotaHistoryMetricsFromConsumption(consumption service.QuotaConsumptionResult) quotaHistoryDerivedMetrics {
	metrics := quotaHistoryDerivedMetrics{ForecastConfidence: "insufficient"}
	quality := &metrics.DataQuality
	if len(consumption.Observations) == 0 {
		return metrics
	}
	observations := consumption.Observations
	quality.ObservedSpanSeconds = observations[len(observations)-1].Snapshot.ObservedAt - observations[0].Snapshot.ObservedAt
	quality.ResetBoundaries = consumption.Summary.ResetBoundaries
	segment := make([]model.ChannelQuotaSnapshot, 0, len(observations))
	for _, observation := range observations {
		snapshot := observation.Snapshot
		quality.SampleCount++
		if snapshot.Status == "unsupported" {
			quality.UnsupportedCount++
			segment = segment[:0]
			continue
		}
		if snapshot.Status != "success" {
			quality.ErrorCount++
			segment = segment[:0]
			continue
		}
		if !service.QuotaSnapshotUsable(snapshot) {
			quality.InvalidCount++
			segment = segment[:0]
			continue
		}
		quality.SuccessCount++
		if observation.ContinuityBreak {
			segment = segment[:0]
		}
		segment = append(segment, snapshot)
	}
	if len(segment) < 2 {
		return metrics
	}
	first, last := segment[0], segment[len(segment)-1]
	span := last.ObservedAt - first.ObservedAt
	if span <= 0 {
		return metrics
	}
	quality.SpanSeconds = span
	if span < quotaHistoryMinimumRateSpanSeconds {
		return metrics
	}
	rate := (last.Available - first.Available) / (float64(span) / (24 * 60 * 60))
	if !finiteQuotaValue(rate) {
		return metrics
	}
	metrics.DropRatePerDay = &rate
	if len(segment) >= 3 && span >= 24*60*60 {
		metrics.ForecastConfidence = "high"
	} else {
		metrics.ForecastConfidence = "low"
	}
	if rate >= 0 || last.Available <= 0 {
		return metrics
	}
	daysUntilZero := last.Available / -rate
	if !finiteQuotaValue(daysUntilZero) || daysUntilZero <= 0 || daysUntilZero > quotaHistoryForecastMaximumDays {
		return metrics
	}
	secondsUntilZero := daysUntilZero * 24 * 60 * 60
	if secondsUntilZero > float64(quotaHistoryForecastMaximumDays*24*60*60) {
		return metrics
	}
	forecast := last.ObservedAt + int64(secondsUntilZero)
	metrics.ForecastZeroAt = &forecast
	return metrics
}

type quotaHistoryPoint struct {
	Timestamp         int64    `json:"timestamp"`
	ObservedAt        int64    `json:"observed_at"`
	Status            string   `json:"status"`
	Available         *float64 `json:"available,omitempty"`
	Used              *float64 `json:"used,omitempty"`
	UsedSource        string   `json:"used_source,omitempty"`
	Total             *float64 `json:"total,omitempty"`
	ResetAt           *int64   `json:"reset_at,omitempty"`
	ErrorCode         string   `json:"error_code,omitempty"`
	EventSource       string   `json:"event_source,omitempty"`
	SampleCount       int      `json:"sample_count"`
	SuccessCount      int      `json:"success_count"`
	FailedCount       int      `json:"failed_count"`
	UnsupportedCount  int      `json:"unsupported_count"`
	Reset             bool     `json:"reset"`
	ContinuityBreak   bool     `json:"continuity_break"`
	Consumption       *float64 `json:"consumption,omitempty"`
	RatePerMinute     *float64 `json:"rate_per_minute,omitempty"`
	PeakRatePerMinute *float64 `json:"peak_rate_per_minute,omitempty"`
	ObservedSeconds   int64    `json:"observed_seconds"`
	IntervalCount     int      `json:"interval_count"`
	Gap               bool     `json:"gap"`
	Recovery          bool     `json:"recovery"`
	BaselineChange    bool     `json:"baseline_change"`
	PeriodStart       int64    `json:"period_start"`
	PeriodEnd         int64    `json:"period_end"`
}

type quotaHistoryBucket struct {
	timestamp         int64
	selected          model.ChannelQuotaSnapshot
	hasSelected       bool
	sampleCount       int
	successCount      int
	failedCount       int
	unsupportedCount  int
	resetCount        int
	continuityBreak   bool
	consumption       *float64
	peakRatePerMinute *float64
	observedSeconds   int64
	intervalCount     int
	gap               bool
	recovery          bool
	baselineChange    bool
}

type quotaHistoryValueSummary struct {
	Start         *float64 `json:"start,omitempty"`
	End           *float64 `json:"end,omitempty"`
	Change        *float64 `json:"change,omitempty"`
	ChangePercent *float64 `json:"change_percent,omitempty"`
	Minimum       *float64 `json:"minimum,omitempty"`
	Maximum       *float64 `json:"maximum,omitempty"`
	Samples       int      `json:"samples"`
}

type quotaHistoryConsumptionSummary = service.QuotaConsumptionSummary

type quotaHistorySummary struct {
	// Legacy available and forecast fields remain for existing callers. New
	// consumers use Available, Used, Consumption, and the method-specific,
	// reset-aware ETA values under data.analysis.
	StartAvailable     *float64                       `json:"start_available,omitempty"`
	EndAvailable       *float64                       `json:"end_available,omitempty"`
	Change             *float64                       `json:"change,omitempty"`
	ChangePercent      *float64                       `json:"change_percent,omitempty"`
	Minimum            *float64                       `json:"minimum,omitempty"`
	Maximum            *float64                       `json:"maximum,omitempty"`
	Available          quotaHistoryValueSummary       `json:"available"`
	Used               *quotaHistoryValueSummary      `json:"used,omitempty"`
	Total              *quotaHistoryValueSummary      `json:"total,omitempty"`
	Consumption        quotaHistoryConsumptionSummary `json:"consumption"`
	DropRatePerDay     *float64                       `json:"drop_rate_per_day,omitempty"`
	ForecastZeroAt     *int64                         `json:"forecast_zero_at,omitempty"`
	ForecastConfidence string                         `json:"forecast_confidence"`
	DataQuality        quotaHistoryDataQuality        `json:"data_quality"`
}

func quotaHistoryRequestContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil && c.Request.Context() != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func quotaHistoryRangeSeconds(value string) (int64, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1h":
		return int64(time.Hour / time.Second), true
	case "6h":
		return int64(6 * time.Hour / time.Second), true
	case "24h", "1d":
		return int64(24 * time.Hour / time.Second), true
	case "7d":
		return int64(7 * 24 * time.Hour / time.Second), true
	case "30d":
		return int64(30 * 24 * time.Hour / time.Second), true
	case "90d":
		return int64(90 * 24 * time.Hour / time.Second), true
	default:
		return 0, false
	}
}

func quotaHistorySuccessFilter(filter model.ChannelQuotaSnapshotQuery) model.ChannelQuotaSnapshotQuery {
	filter.Sources = nil
	filter.Statuses = []string{"success"}
	return filter
}

func quotaHistoryFailureSources(source string) []string {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil
	}
	sources := []string{source}
	for _, suffix := range []string{"_primary", "_secondary"} {
		if base := strings.TrimSuffix(source, suffix); base != source && base != "" {
			sources = append(sources, base)
			break
		}
	}
	return sources
}

func quotaHistoryEventFilter(identity model.ChannelQuotaSnapshotQuery) model.ChannelQuotaSnapshotQuery {
	return model.ChannelQuotaSnapshotQuery{
		EventMetadata: true,
		MetricType:    identity.MetricType,
		WindowType:    identity.WindowType,
		PlanType:      identity.PlanType,
		Unit:          identity.Unit,
		Currency:      identity.Currency,
		WindowSeconds: identity.WindowSeconds,
		Sources:       quotaHistoryFailureSources(identity.Source),
		Statuses:      []string{"error", "unsupported"},
	}
}

func quotaHistorySeriesID(identity model.ChannelQuotaSnapshotQuery) string {
	parts := []string{
		identity.MetricType,
		identity.WindowType,
		identity.Source,
		identity.PlanType,
		identity.Unit,
		identity.Currency,
	}
	if identity.WindowSeconds != nil {
		parts = append(parts, strconv.FormatInt(*identity.WindowSeconds, 10))
	} else {
		parts = append(parts, "")
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "quota_" + hex.EncodeToString(digest[:12])
}

func quotaHistoryLatestSnapshot(left, right *model.ChannelQuotaSnapshot) *model.ChannelQuotaSnapshot {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if right.ObservedAt > left.ObservedAt || (right.ObservedAt == left.ObservedAt && right.Id > left.Id) {
		return right
	}
	return left
}

func quotaHistoryLastRow(rows []model.ChannelQuotaSnapshot) *model.ChannelQuotaSnapshot {
	if len(rows) == 0 {
		return nil
	}
	row := rows[len(rows)-1]
	return &row
}

func mergeQuotaHistorySnapshots(left, right []model.ChannelQuotaSnapshot) []model.ChannelQuotaSnapshot {
	merged := make([]model.ChannelQuotaSnapshot, 0, len(left)+len(right))
	merged = append(merged, left...)
	merged = append(merged, right...)
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].ObservedAt == merged[j].ObservedAt {
			return merged[i].Id < merged[j].Id
		}
		return merged[i].ObservedAt < merged[j].ObservedAt
	})
	return merged
}

func quotaHistorySnapshotStatus(snapshot model.ChannelQuotaSnapshot) string {
	if snapshot.Status == "success" && !service.QuotaSnapshotUsable(snapshot) {
		return "error"
	}
	if snapshot.Status == "" {
		return "unknown"
	}
	return snapshot.Status
}

func quotaHistorySnapshotUsed(snapshot model.ChannelQuotaSnapshot) (float64, string, bool) {
	if snapshot.Used != nil && finiteQuotaValue(*snapshot.Used) {
		return *snapshot.Used, "reported", true
	}
	if snapshot.Total == nil || !finiteQuotaValue(*snapshot.Total) || !finiteQuotaValue(snapshot.Available) {
		return 0, "", false
	}
	used := *snapshot.Total - snapshot.Available
	if !finiteQuotaValue(used) || used < 0 {
		return 0, "", false
	}
	return used, "derived", true
}

func quotaHistoryFloatPointer(value float64) *float64 {
	return &value
}

func quotaHistoryInt64Pointer(value int64) *int64 {
	return &value
}

func quotaHistorySnapshotIsNewer(candidate, selected model.ChannelQuotaSnapshot) bool {
	return candidate.ObservedAt > selected.ObservedAt ||
		(candidate.ObservedAt == selected.ObservedAt && candidate.Id >= selected.Id)
}

func (bucket *quotaHistoryBucket) add(snapshot model.ChannelQuotaSnapshot, reset bool) {
	bucket.sampleCount++
	status := quotaHistorySnapshotStatus(snapshot)
	switch status {
	case "success":
		bucket.successCount++
	case "unsupported":
		bucket.unsupportedCount++
		bucket.continuityBreak = true
	default:
		bucket.failedCount++
		bucket.continuityBreak = true
	}
	if reset {
		bucket.resetCount++
		bucket.continuityBreak = true
	}
	if !bucket.hasSelected || quotaHistorySnapshotIsNewer(snapshot, bucket.selected) {
		bucket.selected = snapshot
		bucket.hasSelected = true
	}
}

func quotaHistoryPointFromBucket(bucket quotaHistoryBucket, seriesSource string) quotaHistoryPoint {
	snapshot := bucket.selected
	status := quotaHistorySnapshotStatus(snapshot)
	point := quotaHistoryPoint{
		Timestamp:         bucket.timestamp,
		ObservedAt:        snapshot.ObservedAt,
		Status:            status,
		SampleCount:       bucket.sampleCount,
		SuccessCount:      bucket.successCount,
		FailedCount:       bucket.failedCount,
		UnsupportedCount:  bucket.unsupportedCount,
		Reset:             bucket.resetCount > 0,
		ContinuityBreak:   bucket.continuityBreak,
		Consumption:       bucket.consumption,
		PeakRatePerMinute: bucket.peakRatePerMinute,
		ObservedSeconds:   bucket.observedSeconds,
		IntervalCount:     bucket.intervalCount,
		Gap:               bucket.gap,
		Recovery:          bucket.recovery,
		BaselineChange:    bucket.baselineChange,
		PeriodStart:       bucket.timestamp,
		PeriodEnd:         snapshot.ObservedAt,
	}
	if bucket.consumption != nil && bucket.observedSeconds > 0 {
		point.RatePerMinute = quotaHistoryFloatPointer(*bucket.consumption / (float64(bucket.observedSeconds) / 60))
	}
	if status == "success" {
		point.Available = quotaHistoryFloatPointer(snapshot.Available)
		if used, source, ok := quotaHistorySnapshotUsed(snapshot); ok {
			point.Used = quotaHistoryFloatPointer(used)
			point.UsedSource = source
		}
		if snapshot.Total != nil && finiteQuotaValue(*snapshot.Total) {
			point.Total = quotaHistoryFloatPointer(*snapshot.Total)
		}
	} else {
		point.ErrorCode = snapshot.ErrorCode
		if point.ErrorCode == "" && snapshot.Status == "success" {
			point.ErrorCode = "invalid_balance"
		}
	}
	if snapshot.ResetAt > 0 {
		point.ResetAt = quotaHistoryInt64Pointer(snapshot.ResetAt)
	}
	if snapshot.Source != "" && snapshot.Source != seriesSource {
		point.EventSource = snapshot.Source
	}
	return point
}

func buildQuotaHistoryPoints(snapshots []model.ChannelQuotaSnapshot, granularity quotaHistoryGranularity, timezoneOffset int, seriesSource string) []quotaHistoryPoint {
	return quotaHistoryPointsFromObservations(service.DeriveQuotaConsumption(snapshots, "").Observations, granularity, timezoneOffset, seriesSource)
}

func quotaHistoryPointsFromObservations(observations []service.QuotaConsumptionObservation, granularity quotaHistoryGranularity, timezoneOffset int, seriesSource string) []quotaHistoryPoint {
	if len(observations) == 0 {
		return []quotaHistoryPoint{}
	}
	if granularity == quotaHistoryRaw {
		points := make([]quotaHistoryPoint, 0, len(observations))
		for _, observation := range observations {
			bucket := quotaHistoryBucket{timestamp: observation.Snapshot.ObservedAt}
			bucket.addObservation(observation)
			points = append(points, quotaHistoryPointFromBucket(bucket, seriesSource))
		}
		return points
	}
	buckets := make(map[int64]*quotaHistoryBucket, len(observations))
	for _, observation := range observations {
		timestamp := quotaHistoryBucketStart(observation.Snapshot.ObservedAt, granularity, timezoneOffset)
		bucket, ok := buckets[timestamp]
		if !ok {
			bucket = &quotaHistoryBucket{timestamp: timestamp}
			buckets[timestamp] = bucket
		}
		bucket.addObservation(observation)
	}
	timestamps := make([]int64, 0, len(buckets))
	for timestamp := range buckets {
		timestamps = append(timestamps, timestamp)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
	points := make([]quotaHistoryPoint, 0, len(timestamps))
	for _, timestamp := range timestamps {
		points = append(points, quotaHistoryPointFromBucket(*buckets[timestamp], seriesSource))
	}
	return points
}

func (bucket *quotaHistoryBucket) addObservation(observation service.QuotaConsumptionObservation) {
	bucket.add(observation.Snapshot, observation.Reset)
	bucket.continuityBreak = bucket.continuityBreak || observation.ContinuityBreak
	bucket.gap = bucket.gap || observation.Gap
	bucket.recovery = bucket.recovery || observation.Recovery
	bucket.baselineChange = bucket.baselineChange || observation.BaselineChange
	if observation.Consumption == nil {
		return
	}
	if bucket.consumption == nil {
		bucket.consumption = quotaHistoryFloatPointer(0)
	}
	*bucket.consumption += *observation.Consumption
	bucket.observedSeconds += observation.ObservedSeconds
	bucket.intervalCount++
	if observation.RatePerMinute != nil && (bucket.peakRatePerMinute == nil || *observation.RatePerMinute > *bucket.peakRatePerMinute) {
		bucket.peakRatePerMinute = quotaHistoryFloatPointer(*observation.RatePerMinute)
	}
}

func limitQuotaHistoryPoints(points []quotaHistoryPoint, limit int) ([]quotaHistoryPoint, bool) {
	if len(points) <= limit {
		return points, false
	}
	// Compact contiguous groups rather than selecting isolated points. All
	// interval consumption and failure/reset markers survive the point budget.
	limited := make([]quotaHistoryPoint, 0, limit)
	for index := 0; index < limit; index++ {
		start, end := index*len(points)/limit, (index+1)*len(points)/limit
		point := points[end-1]
		point.PeriodStart = points[start].PeriodStart
		point.Consumption = nil
		point.RatePerMinute = nil
		point.PeakRatePerMinute = nil
		point.ObservedSeconds, point.IntervalCount = 0, 0
		point.SampleCount, point.SuccessCount, point.FailedCount, point.UnsupportedCount = 0, 0, 0, 0
		for _, member := range points[start:end] {
			point.SampleCount += member.SampleCount
			point.SuccessCount += member.SuccessCount
			point.FailedCount += member.FailedCount
			point.UnsupportedCount += member.UnsupportedCount
			point.Reset = point.Reset || member.Reset
			point.ContinuityBreak = point.ContinuityBreak || member.ContinuityBreak
			point.Gap = point.Gap || member.Gap
			point.Recovery = point.Recovery || member.Recovery
			point.BaselineChange = point.BaselineChange || member.BaselineChange
			if member.Consumption != nil {
				if point.Consumption == nil {
					point.Consumption = quotaHistoryFloatPointer(0)
				}
				*point.Consumption += *member.Consumption
				point.ObservedSeconds += member.ObservedSeconds
				point.IntervalCount += member.IntervalCount
			}
			if member.PeakRatePerMinute != nil && (point.PeakRatePerMinute == nil || *member.PeakRatePerMinute > *point.PeakRatePerMinute) {
				point.PeakRatePerMinute = quotaHistoryFloatPointer(*member.PeakRatePerMinute)
			}
		}
		if point.Consumption != nil && point.ObservedSeconds > 0 {
			point.RatePerMinute = quotaHistoryFloatPointer(*point.Consumption / (float64(point.ObservedSeconds) / 60))
		}
		limited = append(limited, point)
	}
	return limited, true
}

func latestContinuousQuotaHistorySegment(observations []service.QuotaConsumptionObservation) []model.ChannelQuotaSnapshot {
	segment := make([]model.ChannelQuotaSnapshot, 0, len(observations))
	var latestSegment []model.ChannelQuotaSnapshot
	for _, observation := range observations {
		snapshot := observation.Snapshot
		if quotaHistorySnapshotStatus(snapshot) != "success" {
			if len(segment) > 0 {
				latestSegment = append([]model.ChannelQuotaSnapshot(nil), segment...)
			}
			segment = nil
			continue
		}
		if observation.ContinuityBreak {
			segment = nil
		}
		segment = append(segment, snapshot)
	}
	if len(segment) == 0 {
		return latestSegment
	}
	return segment
}

func quotaHistoryValueSummaryFor(snapshots []model.ChannelQuotaSnapshot, value func(model.ChannelQuotaSnapshot) (float64, bool)) quotaHistoryValueSummary {
	summary := quotaHistoryValueSummary{}
	for _, snapshot := range snapshots {
		if quotaHistorySnapshotStatus(snapshot) != "success" {
			continue
		}
		current, ok := value(snapshot)
		if !ok || !finiteQuotaValue(current) {
			continue
		}
		summary.Samples++
		if summary.Start == nil {
			summary.Start = quotaHistoryFloatPointer(current)
			summary.Minimum = quotaHistoryFloatPointer(current)
			summary.Maximum = quotaHistoryFloatPointer(current)
		}
		summary.End = quotaHistoryFloatPointer(current)
		if current < *summary.Minimum {
			summary.Minimum = quotaHistoryFloatPointer(current)
		}
		if current > *summary.Maximum {
			summary.Maximum = quotaHistoryFloatPointer(current)
		}
	}
	if summary.Start != nil && summary.End != nil {
		change := *summary.End - *summary.Start
		summary.Change = quotaHistoryFloatPointer(change)
		if *summary.Start != 0 {
			percent := change / *summary.Start * 100
			summary.ChangePercent = quotaHistoryFloatPointer(percent)
		}
	}
	return summary
}

func deriveQuotaHistorySummary(snapshots []model.ChannelQuotaSnapshot, metrics quotaHistoryDerivedMetrics, unit string) *quotaHistorySummary {
	return quotaHistorySummaryFromConsumption(service.DeriveQuotaConsumption(snapshots, unit), metrics)
}

func quotaHistorySummaryFromConsumption(consumption service.QuotaConsumptionResult, metrics quotaHistoryDerivedMetrics) *quotaHistorySummary {
	segment := latestContinuousQuotaHistorySegment(consumption.Observations)
	available := quotaHistoryValueSummaryFor(segment, func(snapshot model.ChannelQuotaSnapshot) (float64, bool) {
		return snapshot.Available, finiteQuotaValue(snapshot.Available)
	})
	if len(consumption.Observations) == 0 {
		return nil
	}
	used := quotaHistoryValueSummaryFor(segment, func(snapshot model.ChannelQuotaSnapshot) (float64, bool) {
		value, _, ok := quotaHistorySnapshotUsed(snapshot)
		return value, ok
	})
	total := quotaHistoryValueSummaryFor(segment, func(snapshot model.ChannelQuotaSnapshot) (float64, bool) {
		if snapshot.Total == nil || !finiteQuotaValue(*snapshot.Total) {
			return 0, false
		}
		return *snapshot.Total, true
	})
	summary := &quotaHistorySummary{
		StartAvailable:     available.Start,
		EndAvailable:       available.End,
		Change:             available.Change,
		ChangePercent:      available.ChangePercent,
		Minimum:            available.Minimum,
		Maximum:            available.Maximum,
		Available:          available,
		Consumption:        consumption.Summary,
		DropRatePerDay:     metrics.DropRatePerDay,
		ForecastZeroAt:     metrics.ForecastZeroAt,
		ForecastConfidence: metrics.ForecastConfidence,
		DataQuality:        metrics.DataQuality,
	}
	if used.Samples > 0 {
		summary.Used = &used
	}
	if total.Samples > 0 {
		summary.Total = &total
	}
	return summary
}

func quotaHistoryCurrent(snapshot *model.ChannelQuotaSnapshot, seriesSource string) gin.H {
	if snapshot == nil {
		return nil
	}
	status := quotaHistorySnapshotStatus(*snapshot)
	current := gin.H{"observed_at": snapshot.ObservedAt, "status": status}
	if snapshot.Source != "" && snapshot.Source != seriesSource {
		current["event_source"] = snapshot.Source
	}
	if status != "success" {
		errorCode := snapshot.ErrorCode
		if errorCode == "" && snapshot.Status == "success" {
			errorCode = "invalid_balance"
		}
		current["error_code"] = errorCode
		return current
	}
	current["available"] = snapshot.Available
	if used, source, ok := quotaHistorySnapshotUsed(*snapshot); ok {
		current["used"] = used
		current["used_source"] = source
	}
	if snapshot.Total != nil && finiteQuotaValue(*snapshot.Total) {
		current["total"] = *snapshot.Total
	}
	if snapshot.ResetAt > 0 {
		current["reset_at"] = snapshot.ResetAt
	}
	return current
}

func quotaHistoryMetadata(response gin.H, identity model.ChannelQuotaSnapshotQuery) {
	if identity.MetricType == "" {
		return
	}
	response["series_id"] = quotaHistorySeriesID(identity)
	response["metric_type"] = identity.MetricType
	response["window_type"] = identity.WindowType
	response["source"] = identity.Source
	response["plan_type"] = identity.PlanType
	response["unit"] = identity.Unit
	response["currency"] = identity.Currency
	if identity.WindowSeconds != nil {
		response["window_seconds"] = *identity.WindowSeconds
	}
	response["series"] = gin.H{
		"id":             response["series_id"],
		"metric_type":    identity.MetricType,
		"window_type":    identity.WindowType,
		"source":         identity.Source,
		"plan_type":      identity.PlanType,
		"unit":           identity.Unit,
		"currency":       identity.Currency,
		"window_seconds": response["window_seconds"],
	}
}

// GetChannelQuotaHistory returns normalized quota observations for a channel.
// It deliberately excludes raw upstream responses and credentials. The range
// is fully read before aggregation; when an explicit safety budget would be
// exceeded, the response is marked incomplete instead of pretending that the
// newest observations represent the requested range.
func GetChannelQuotaHistory(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if _, err = model.CacheGetChannel(id); err != nil {
		common.ApiError(c, err)
		return
	}
	consumptionBasis := service.QuotaConsumptionBasisAuto
	if value := strings.ToLower(strings.TrimSpace(c.Query("consumption_basis"))); value != "" {
		if value != string(service.QuotaConsumptionBasisAvailable) {
			common.ApiError(c, errors.New("invalid consumption_basis; use available or omit it"))
			return
		}
		consumptionBasis = service.QuotaConsumptionBasisAvailable
	}
	exactIdentity := false
	if value := strings.TrimSpace(c.Query("exact_identity")); value != "" {
		exactIdentity, err = strconv.ParseBool(value)
		if err != nil {
			common.ApiError(c, errors.New("invalid exact_identity flag"))
			return
		}
	}
	now := time.Now().Unix()
	end := now
	start := now - 30*24*60*60
	if value := strings.TrimSpace(c.Query("range")); value != "" && value != "custom" {
		seconds, ok := quotaHistoryRangeSeconds(value)
		if !ok {
			common.ApiError(c, errors.New("invalid range; use 1h, 6h, 24h, 7d, 30d, or 90d"))
			return
		}
		start = end - seconds
	}
	for key, target := range map[string]*int64{"start": &start, "end": &end} {
		value := strings.TrimSpace(c.Query(key))
		if value == "" {
			continue
		}
		parsed, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil {
			parsedTime, timeErr := time.Parse(time.RFC3339, value)
			if timeErr != nil {
				common.ApiError(c, errors.New("invalid "+key+" timestamp"))
				return
			}
			parsed = parsedTime.Unix()
		}
		*target = parsed
	}
	if start < 0 || end < start {
		common.ApiError(c, errors.New("invalid quota history time range"))
		return
	}
	if end-start > 180*24*60*60 {
		common.ApiError(c, errors.New("quota history range cannot exceed 180 days"))
		return
	}
	limit := 500
	if value := strings.TrimSpace(c.Query("limit")); value != "" {
		parsed, parseErr := strconv.Atoi(value)
		if parseErr != nil || parsed <= 0 || parsed > maxQuotaHistoryPointLimit {
			common.ApiError(c, fmt.Errorf("quota history limit must be between 1 and %d", maxQuotaHistoryPointLimit))
			return
		}
		limit = parsed
	}
	granularity, err := parseQuotaHistoryGranularity(c.Query("granularity"), start, end)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	timezoneOffset, err := parseQuotaHistoryTimezoneOffset(c.Query("timezone_offset"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rateWindowSeconds, err := parseQuotaAnalysisDuration(c.Query("rate_window"), end-start, "rate_window")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ewmaHalfLifeSeconds := defaultQuotaAnalysisHalfLife(rateWindowSeconds)
	if value := strings.TrimSpace(c.Query("ewma_half_life")); value != "" {
		ewmaHalfLifeSeconds, err = parseQuotaAnalysisDuration(value, rateWindowSeconds, "ewma_half_life")
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	analysisStart := end - rateWindowSeconds
	seriesFilter := model.ChannelQuotaSnapshotQuery{
		ExactIdentity: exactIdentity,
		MetricType:    strings.TrimSpace(c.Query("metric_type")),
		WindowType:    strings.TrimSpace(c.Query("window_type")),
		Source:        strings.TrimSpace(c.Query("source")),
		PlanType:      strings.TrimSpace(c.Query("plan_type")),
		Unit:          strings.TrimSpace(c.Query("unit")),
		Currency:      strings.TrimSpace(c.Query("currency")),
	}
	seriesFilter.WindowSeconds, err = parseQuotaHistoryWindowSeconds(c.Query("window_seconds"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Resolve the chart identity from the newest successful sample, not a
	// generic failure marker. A current failure is queried separately below and
	// still wins the current/alert state, while the historical line remains the
	// actual provider window that operators selected.
	successCandidateFilter := quotaHistorySuccessFilter(seriesFilter)
	latestSuccessRows, err := model.ListChannelQuotaSnapshotsWithQuery(id, start, end, successCandidateFilter, 1)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	identity := seriesFilter
	latestSuccess := quotaHistoryLastRow(latestSuccessRows)
	if latestSuccess != nil {
		identity = resolveQuotaHistorySeriesFilter(identity, *latestSuccess)
	}
	identity.Sources = nil
	identity.Statuses = nil

	if latestSuccess == nil {
		latestRows, latestErr := model.ListChannelQuotaSnapshotsWithQuery(id, start, end, seriesFilter, 1)
		if latestErr != nil {
			common.ApiError(c, latestErr)
			return
		}
		if latest := quotaHistoryLastRow(latestRows); latest != nil {
			identity = resolveQuotaHistorySeriesFilter(identity, *latest)
			identity.Sources = nil
			identity.Statuses = nil
		}
	}

	response := gin.H{
		"channel_id":            id,
		"start":                 start,
		"end":                   end,
		"limit":                 limit,
		"granularity":           string(granularity),
		"requested_granularity": strings.ToLower(strings.TrimSpace(c.Query("granularity"))),
		"timezone_offset":       timezoneOffset,
		"raw_observation_limit": maxQuotaHistoryRawObservations,
		"points":                []quotaHistoryPoint{},
		"analysis":              service.AnalyzeQuotaConsumption(service.QuotaConsumptionResult{}, analysisStart, end, ewmaHalfLifeSeconds),
	}
	quotaHistoryMetadata(response, identity)
	if response["requested_granularity"] == "" {
		response["requested_granularity"] = string(quotaHistoryRaw)
	}

	if identity.MetricType == "" {
		response["raw_observations"] = 0
		response["available_points"] = 0
		response["returned_points"] = 0
		response["source_complete"] = true
		response["points_complete"] = true
		response["complete"] = true
		response["truncated"] = false
		response["alert"] = deriveQuotaHistoryAlert(nil)
		c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
		return
	}

	successFilter := quotaHistorySuccessFilter(identity)
	eventFilter := quotaHistoryEventFilter(identity)
	requestContext := quotaHistoryRequestContext(c)
	successCount, err := model.CountChannelQuotaSnapshotsWithQuery(requestContext, id, start, end, successFilter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	eventCount, err := model.CountChannelQuotaSnapshotsWithQuery(requestContext, id, start, end, eventFilter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rawObservations := successCount + eventCount
	response["raw_observations"] = rawObservations

	latestEventRows, err := model.ListChannelQuotaSnapshotsWithQuery(id, start, end, eventFilter, 1)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	latestSnapshot := quotaHistoryLatestSnapshot(latestSuccess, quotaHistoryLastRow(latestEventRows))
	response["current"] = quotaHistoryCurrent(latestSnapshot, identity.Source)
	response["alert"] = deriveQuotaHistoryAlert(latestSnapshot)

	if rawObservations > maxQuotaHistoryRawObservations {
		analysis := response["analysis"].(service.QuotaAnalysis)
		analysis.Complete = false
		response["analysis"] = analysis
		response["available_points"] = 0
		response["returned_points"] = 0
		response["source_complete"] = false
		response["points_complete"] = false
		response["complete"] = false
		response["truncated"] = true
		response["truncation_reason"] = "raw_observation_limit"
		response["resolution_hint"] = "Use a shorter time range."
		c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
		return
	}

	successes, err := model.ListChannelQuotaSnapshotsForHistory(requestContext, id, start, end, successFilter, maxQuotaHistoryRawObservations)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	events, err := model.ListChannelQuotaSnapshotsForHistory(requestContext, id, start, end, eventFilter, maxQuotaHistoryRawObservations)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !successes.Complete || !events.Complete {
		// A concurrent sampler can add rows between Count and List. Do not use
		// the partial prefix from either query as a history range.
		analysis := response["analysis"].(service.QuotaAnalysis)
		analysis.Complete = false
		response["analysis"] = analysis
		response["available_points"] = 0
		response["returned_points"] = 0
		response["source_complete"] = false
		response["points_complete"] = false
		response["complete"] = false
		response["truncated"] = true
		response["truncation_reason"] = "raw_observation_limit"
		response["resolution_hint"] = "Use a shorter time range."
		c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
		return
	}

	snapshots := mergeQuotaHistorySnapshots(successes.Snapshots, events.Snapshots)
	// Count/list are separate read-only queries; report the actual stable end of
	// the loaded range if a sampler appended a same-second observation meanwhile.
	response["raw_observations"] = len(snapshots)
	latestSnapshot = quotaHistoryLatestSnapshot(latestSnapshot, quotaHistoryLastRow(snapshots))
	response["current"] = quotaHistoryCurrent(latestSnapshot, identity.Source)
	response["alert"] = deriveQuotaHistoryAlert(latestSnapshot)
	consumption := service.DeriveQuotaConsumptionWithBasis(snapshots, identity.Unit, consumptionBasis)
	response["analysis"] = service.AnalyzeQuotaConsumption(consumption, analysisStart, end, ewmaHalfLifeSeconds)
	metrics := quotaHistoryMetricsFromConsumption(consumption)
	points := quotaHistoryPointsFromObservations(consumption.Observations, granularity, timezoneOffset, identity.Source)
	availablePoints := len(points)
	points, pointTruncated := limitQuotaHistoryPoints(points, limit)
	response["points"] = points
	response["available_points"] = availablePoints
	response["returned_points"] = len(points)
	response["source_complete"] = true
	response["points_complete"] = !pointTruncated
	response["complete"] = !pointTruncated
	response["truncated"] = pointTruncated
	if pointTruncated {
		response["truncation_reason"] = "point_limit"
	}
	response["data_quality"] = metrics.DataQuality
	if summary := quotaHistorySummaryFromConsumption(consumption, metrics); summary != nil {
		response["summary"] = summary
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

func updateAllChannelsBalance() error {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue
		}
		if channel.ChannelInfo.IsMultiKey {
			continue // skip multi-key channels
		}
		// TODO: support Azure
		//if channel.Type != common.ChannelTypeOpenAI && channel.Type != common.ChannelTypeCustom {
		//	continue
		//}
		result, err, _ := updateChannelBalanceWithPollingLock(channel)
		if err != nil {
			continue
		} else if result.RawResponse == "" {
			// err is nil & balance <= 0 means quota is used up
			if result.Balance <= 0 {
				service.DisableChannel(*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", channel.GetAutoBan()), "余额不足")
			}
		}
		time.Sleep(common.RequestInterval)
	}
	return nil
}

func UpdateAllChannelsBalance(c *gin.Context) {
	// TODO: make it async
	err := updateAllChannelsBalance()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

// runChannelQuotaSnapshotSyncOnce samples enabled single-key channels using
// the normalized balance query path.  The per-channel polling lock prevents a
// scheduled pass from racing with a manual balance query or multi-key state
// update.  A busy channel is skipped and retried on the next scheduled pass;
// this avoids waiting behind a potentially slow provider request.
func runChannelQuotaSnapshotSyncOnce(ctx context.Context, maxChannels int, report func(processed, total int)) (summary channelQuotaSnapshotSyncSummary, runErr error) {
	defer func() {
		summary.Deferred = summary.Considered - summary.Sampled - summary.Failed - summary.Skipped
		if summary.Deferred < 0 {
			summary.Deferred = 0
		}
		summary.BudgetExhausted = errors.Is(runErr, context.DeadlineExceeded)
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	if maxChannels <= 0 || maxChannels > channelQuotaSnapshotSyncMaxChannels {
		maxChannels = channelQuotaSnapshotSyncMaxChannels
	}
	fetchLimit := maxChannels * 4
	if fetchLimit < maxChannels {
		fetchLimit = maxChannels
	}
	channels, err := model.GetChannelsForQuotaSnapshotSyncContext(ctx, fetchLimit)
	if err != nil {
		return summary, err
	}
	// The model query selects single-key accounts by their oldest recorded
	// attempt. Keep extra candidates so busy polling locks do not prevent other
	// accounts from being sampled, with a hard cap on actual provider requests.
	summary.Considered = len(channels)
	if report != nil {
		report(0, summary.Considered)
	}
	var firstPersistErr error
	for index, channel := range channels {
		if summary.Sampled+summary.Failed >= maxChannels {
			break
		}
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if channel == nil || channel.Status != common.ChannelStatusEnabled || channel.ChannelInfo.IsMultiKey {
			summary.Skipped++
			if report != nil {
				report(index+1, summary.Considered)
			}
			continue
		}
		lock := model.GetChannelPollingLock(channel.Id)
		if !lock.TryLock() {
			summary.Skipped++
			if report != nil {
				report(index+1, summary.Considered)
			}
			continue
		}
		var queryErr error
		var persistErr error
		if channel.Type == constant.ChannelTypeCodex {
			// Codex OAuth exposes subscription windows through the official WHAM
			// usage endpoint rather than the generic balance endpoint. Keep this
			// branch inside the same bounded polling lock and sampler budget.
			samplingErr := sampleCodexChannelUsage(ctx, channel)
			queryErr = samplingErr
			var classifiedErr *channelQuotaSamplingError
			if errors.As(samplingErr, &classifiedErr) {
				queryErr = classifiedErr.QueryErr
				persistErr = classifiedErr.PersistErr
			}
		} else {
			result, balanceErr := updateChannelBalanceWithContext(ctx, channel)
			queryErr = balanceErr
			var classifiedErr *channelQuotaSamplingError
			if errors.As(balanceErr, &classifiedErr) {
				queryErr, persistErr = classifiedErr.QueryErr, classifiedErr.PersistErr
			}
			persistErr = errors.Join(persistErr, recordChannelBalanceSnapshot(channel, result, queryErr))
		}
		lock.Unlock()
		if queryErr != nil {
			summary.Failed++
			if errors.Is(queryErr, context.DeadlineExceeded) {
				summary.TimedOut++
			}
			if errors.Is(queryErr, errChannelQuotaUnsupported) {
				summary.Unsupported++
			}
		} else {
			summary.Sampled++
		}
		if persistErr != nil {
			summary.PersistFailed++
			if firstPersistErr == nil {
				firstPersistErr = persistErr
			}
		}
		if report != nil {
			report(index+1, summary.Considered)
		}
		if err := ctx.Err(); err != nil {
			return summary, errors.Join(err, firstPersistErr)
		}
		if common.RequestInterval > 0 && index+1 < len(channels) && summary.Sampled+summary.Failed < maxChannels {
			delay := common.RequestInterval
			// Do not let a legacy request interval turn a bounded task into a
			// multi-hour run. Operators needing a slower cadence should configure
			// CHANNEL_QUOTA_SYNC_INTERVAL instead.
			if delay > 10*time.Second {
				delay = 10 * time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return summary, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if firstPersistErr != nil {
		return summary, fmt.Errorf("quota snapshot persistence failed: %w", firstPersistErr)
	}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	return summary, nil
}

func AutomaticallyUpdateChannels(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Minute)
		common.SysLog("updating all channels")
		_ = updateAllChannelsBalance()
		common.SysLog("channels update done")
	}
}
