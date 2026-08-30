package controller

import (
	"context"
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

const maxAdvancedCustomBalanceResponseBytes = 256 << 10

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

func GetResponseBody(method, url string, channel *model.Channel, headers http.Header) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), channelBalanceRequestTimeout)
	defer cancel()
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
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	err = res.Body.Close()
	if err != nil {
		return nil, err
	}
	return body, nil
}

func updateChannelCloseAIBalance(channel *model.Channel) (float64, error) {
	url := fmt.Sprintf("%s/dashboard/billing/credit_grants", channel.GetBaseURL())
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))

	if err != nil {
		return 0, err
	}
	response := OpenAICreditGrants{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(response.TotalAvailable)
	return response.TotalAvailable, nil
}

func updateChannelOpenAISBBalance(channel *model.Channel) (float64, error) {
	url := fmt.Sprintf("https://api.openai-sb.com/sb-api/user/status?api_key=%s", channel.Key)
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
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
	channel.UpdateBalance(balance)
	return balance, nil
}

func updateChannelAIProxyBalance(channel *model.Channel) (float64, error) {
	url := "https://aiproxy.io/api/report/getUserOverview"
	headers := http.Header{}
	headers.Add("Api-Key", channel.Key)
	body, err := GetResponseBody("GET", url, channel, headers)
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
	channel.UpdateBalance(response.Data.TotalPoints)
	return response.Data.TotalPoints, nil
}

func updateChannelAPI2GPTBalance(channel *model.Channel) (float64, error) {
	url := "https://api.api2gpt.com/dashboard/billing/credit_grants"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))

	if err != nil {
		return 0, err
	}
	response := API2GPTUsageResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(response.TotalRemaining)
	return response.TotalRemaining, nil
}

func updateChannelSiliconFlowBalance(channel *model.Channel) (float64, error) {
	url := "https://api.siliconflow.cn/v1/user/info"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
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
	channel.UpdateBalance(balance)
	return balance, nil
}

func updateChannelDeepSeekBalance(channel *model.Channel) (float64, error) {
	url := "https://api.deepseek.com/user/balance"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
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
	channel.UpdateBalance(balance)
	return balance, nil
}

func updateChannelAIGC2DBalance(channel *model.Channel) (float64, error) {
	url := "https://api.aigc2d.com/dashboard/billing/credit_grants"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := APGC2DGPTUsageResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(response.TotalAvailable)
	return response.TotalAvailable, nil
}

func updateChannelOpenRouterBalance(channel *model.Channel) (float64, error) {
	url := "https://openrouter.ai/api/v1/credits"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := OpenRouterCreditResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	balance := response.Data.TotalCredits - response.Data.TotalUsage
	channel.UpdateBalance(balance)
	return balance, nil
}

func updateChannelMoonshotBalance(channel *model.Channel) (float64, error) {
	url := "https://api.moonshot.cn/v1/users/me/balance"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
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
	channel.UpdateBalance(availableBalanceUsd)
	return availableBalanceUsd, nil
}

func fetchAdvancedCustomBalance(channel *model.Channel) (channelBalanceResult, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), channelBalanceRequestTimeout)
	defer cancel()
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
		return channelBalanceResult{}, sanitizeAdvancedCustomRequestError(err, key, requestURL)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return channelBalanceResult{}, fmt.Errorf("status code: %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAdvancedCustomBalanceResponseBytes+1))
	if err != nil {
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
				channel.UpdateBalance(balance)
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
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		return fetchAdvancedCustomBalance(channel)
	}
	balance, err := updateStandardChannelBalance(channel)
	return channelBalanceResult{Balance: balance}, err
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
		snapshot.Status = "error"
		snapshot.ErrorCode = "query_failed"
		snapshot.ErrorMessage = "balance query failed"
		return model.RecordChannelQuotaSnapshot(snapshot)
	}
	if result.RawResponse != "" {
		snapshot.Status = "unsupported"
		snapshot.ErrorCode = "unstructured_response"
		snapshot.ErrorMessage = "upstream did not return a numeric balance"
		return model.RecordChannelQuotaSnapshot(snapshot)
	}
	if math.IsNaN(result.Balance) || math.IsInf(result.Balance, 0) {
		snapshot.Status = "error"
		snapshot.ErrorCode = "invalid_balance"
		snapshot.ErrorMessage = "upstream returned an invalid balance"
		return model.RecordChannelQuotaSnapshot(snapshot)
	}
	snapshot.Available = result.Balance
	return model.RecordChannelQuotaSnapshot(snapshot)
}

func updateStandardChannelBalance(channel *model.Channel) (float64, error) {
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
		return 0, errors.New("尚未实现")
	case constant.ChannelTypeCustom:
		baseURL = channel.GetBaseURL()
	//case common.ChannelTypeOpenAISB:
	//	return updateChannelOpenAISBBalance(channel)
	case constant.ChannelTypeAIProxy:
		return updateChannelAIProxyBalance(channel)
	case constant.ChannelTypeAPI2GPT:
		return updateChannelAPI2GPTBalance(channel)
	case constant.ChannelTypeAIGC2D:
		return updateChannelAIGC2DBalance(channel)
	case constant.ChannelTypeSiliconFlow:
		return updateChannelSiliconFlowBalance(channel)
	case constant.ChannelTypeDeepSeek:
		return updateChannelDeepSeekBalance(channel)
	case constant.ChannelTypeOpenRouter:
		return updateChannelOpenRouterBalance(channel)
	case constant.ChannelTypeMoonshot:
		return updateChannelMoonshotBalance(channel)
	default:
		return 0, errors.New("尚未实现")
	}
	url := fmt.Sprintf("%s/v1/dashboard/billing/subscription", baseURL)

	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
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
	body, err = GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	usage := OpenAIUsageResponse{}
	err = common.Unmarshal(body, &usage)
	if err != nil {
		return 0, err
	}
	balance := subscription.HardLimitUSD - usage.TotalUsage/100
	channel.UpdateBalance(balance)
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
	result, err := updateChannelBalance(channel)
	recordChannelBalanceSnapshot(channel, result, err)
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
	quotaHistoryRaw  quotaHistoryGranularity = "raw"
	quotaHistoryHour quotaHistoryGranularity = "hour"
	quotaHistoryDay  quotaHistoryGranularity = "day"
	quotaHistoryWeek quotaHistoryGranularity = "week"
	quotaHistoryAuto quotaHistoryGranularity = "auto"
)

func parseQuotaHistoryGranularity(value string, start, end int64) (quotaHistoryGranularity, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return quotaHistoryRaw, nil
	}
	if value == string(quotaHistoryAuto) {
		switch {
		case end-start <= 48*60*60:
			return quotaHistoryHour, nil
		case end-start <= 14*24*60*60:
			return quotaHistoryDay, nil
		default:
			return quotaHistoryWeek, nil
		}
	}
	granularity := quotaHistoryGranularity(value)
	switch granularity {
	case quotaHistoryRaw, quotaHistoryHour, quotaHistoryDay, quotaHistoryWeek:
		return granularity, nil
	default:
		return "", errors.New("invalid granularity; use raw, hour, day, week, or auto")
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

func quotaHistoryBucketStart(timestamp int64, granularity quotaHistoryGranularity, timezoneOffset int) int64 {
	local := time.Unix(timestamp, 0).UTC().Add(time.Duration(timezoneOffset) * time.Minute)
	var bucket time.Time
	switch granularity {
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

// aggregateQuotaHistorySnapshots keeps the latest observation in each bucket.
// Failed observations remain visible when a bucket has no successful sample;
// a failure never becomes a numeric zero.
func aggregateQuotaHistorySnapshots(snapshots []model.ChannelQuotaSnapshot, granularity quotaHistoryGranularity, timezoneOffset int) []model.ChannelQuotaSnapshot {
	if granularity == quotaHistoryRaw || len(snapshots) < 2 {
		return snapshots
	}
	aggregated := make([]model.ChannelQuotaSnapshot, 0, len(snapshots))
	indices := make(map[int64]int, len(snapshots))
	for _, snapshot := range snapshots {
		bucket := quotaHistoryBucketStart(snapshot.ObservedAt, granularity, timezoneOffset)
		snapshot.ObservedAt = bucket
		if index, ok := indices[bucket]; ok {
			previous := aggregated[index]
			if previous.Status == "success" && snapshot.Status != "success" {
				continue
			}
			aggregated[index] = snapshot
			continue
		}
		indices[bucket] = len(aggregated)
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
	SuccessCount    int   `json:"success_count"`
	ErrorCount      int   `json:"error_count"`
	InvalidCount    int   `json:"invalid_count"`
	ResetBoundaries int   `json:"reset_boundaries"`
	SpanSeconds     int64 `json:"span_seconds"`
}

type quotaHistoryDerivedMetrics struct {
	DropRatePerDay     *float64                `json:"drop_rate_per_day"`
	ForecastZeroAt     *int64                  `json:"forecast_zero_at"`
	ForecastConfidence string                  `json:"forecast_confidence"`
	DataQuality        quotaHistoryDataQuality `json:"data_quality"`
}

type quotaHistoryAlert struct {
	Enabled         bool     `json:"enabled"`
	Status          string   `json:"status"`
	RatioPercent    *float64 `json:"ratio_percent,omitempty"`
	WarningPercent  float64  `json:"warning_percent"`
	CriticalPercent float64  `json:"critical_percent"`
}

// deriveQuotaHistoryAlert exposes an opt-in, read-only threshold status. It
// intentionally does not send notifications, disable channels, or alter
// routing. Without a provider-reported total quota the ratio is undefined and
// the status remains unavailable, avoiding misleading currency-specific
// absolute thresholds.
func deriveQuotaHistoryAlert(snapshot *model.ChannelQuotaSnapshot) quotaHistoryAlert {
	alert := quotaHistoryAlert{
		Enabled:         common.ChannelQuotaAlertEnabled,
		WarningPercent:  common.ChannelQuotaAlertWarningPercent,
		CriticalPercent: common.ChannelQuotaAlertCriticalPercent,
		Status:          "disabled",
	}
	if !alert.Enabled {
		return alert
	}
	alert.Status = "unavailable"
	if snapshot == nil || snapshot.Status != "success" || snapshot.Total == nil ||
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
// A non-zero reset_at change starts a new segment so a provider quota reset is
// never interpreted as consumption. Failed observations are ignored for the
// slope but remain visible in data quality counts.
func deriveQuotaHistoryMetrics(snapshots []model.ChannelQuotaSnapshot) quotaHistoryDerivedMetrics {
	metrics := quotaHistoryDerivedMetrics{ForecastConfidence: "insufficient"}
	quality := &metrics.DataQuality
	valid := make([]model.ChannelQuotaSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.Status != "success" {
			quality.ErrorCount++
			continue
		}
		if !finiteQuotaValue(snapshot.Available) {
			quality.InvalidCount++
			continue
		}
		quality.SuccessCount++
		valid = append(valid, snapshot)
	}
	if len(valid) == 0 {
		return metrics
	}
	sort.SliceStable(valid, func(i, j int) bool {
		return valid[i].ObservedAt < valid[j].ObservedAt
	})
	segment := make([]model.ChannelQuotaSnapshot, 0, len(valid))
	for _, snapshot := range valid {
		if len(segment) > 0 && snapshot.ResetAt != segment[len(segment)-1].ResetAt &&
			(snapshot.ResetAt != 0 || segment[len(segment)-1].ResetAt != 0) {
			quality.ResetBoundaries++
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

// GetChannelQuotaHistory returns normalized quota observations for a channel.
// It deliberately excludes any raw upstream response data.
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
	now := time.Now().Unix()
	end := now
	start := now - 30*24*60*60
	if value := strings.TrimSpace(c.Query("range")); value != "" {
		var seconds int64
		switch strings.ToLower(value) {
		case "24h", "1d":
			seconds = 24 * 60 * 60
		case "7d":
			seconds = 7 * 24 * 60 * 60
		case "30d":
			seconds = 30 * 24 * 60 * 60
		case "90d":
			seconds = 90 * 24 * 60 * 60
		default:
			common.ApiError(c, errors.New("invalid range; use 24h, 7d, 30d, or 90d"))
			return
		}
		start = end - seconds
	}
	if value := strings.TrimSpace(c.Query("start")); value != "" {
		parsed, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil {
			if timestamp, timeErr := time.Parse(time.RFC3339, value); timeErr == nil {
				parsed = timestamp.Unix()
			} else {
				common.ApiError(c, errors.New("invalid start timestamp"))
				return
			}
		}
		start = parsed
	}
	if value := strings.TrimSpace(c.Query("end")); value != "" {
		parsed, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil {
			if timestamp, timeErr := time.Parse(time.RFC3339, value); timeErr == nil {
				parsed = timestamp.Unix()
			} else {
				common.ApiError(c, errors.New("invalid end timestamp"))
				return
			}
		}
		end = parsed
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
		if parseErr != nil || parsed <= 0 {
			common.ApiError(c, errors.New("invalid quota history limit"))
			return
		}
		limit = parsed
	}
	if limit > 2000 {
		limit = 2000
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
	snapshots, err := model.ListChannelQuotaSnapshots(id, start, end, c.Query("metric_type"), c.Query("window_type"), limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	snapshots = aggregateQuotaHistorySnapshots(snapshots, granularity, timezoneOffset)
	points := make([]gin.H, 0, len(snapshots))
	for _, snapshot := range snapshots {
		point := gin.H{
			"timestamp": snapshot.ObservedAt,
			"status":    snapshot.Status,
		}
		if snapshot.Status == "success" {
			if finiteQuotaValue(snapshot.Available) {
				point["available"] = snapshot.Available
			} else {
				// A corrupt/legacy row must not make JSON encoding fail with NaN
				// or Infinity, and must never be shown as a numeric zero.
				point["status"] = "error"
				point["error_code"] = "invalid_balance"
			}
			if snapshot.Used != nil {
				if finiteQuotaValue(*snapshot.Used) {
					point["used"] = *snapshot.Used
				}
			}
			if snapshot.Total != nil {
				if finiteQuotaValue(*snapshot.Total) {
					point["total"] = *snapshot.Total
				}
			}
		}
		if snapshot.ResetAt > 0 {
			point["reset_at"] = snapshot.ResetAt
		}
		if snapshot.ErrorCode != "" {
			point["error_code"] = snapshot.ErrorCode
		}
		points = append(points, point)
	}
	response := gin.H{
		"channel_id":      id,
		"start":           start,
		"end":             end,
		"limit":           limit,
		"granularity":     string(granularity),
		"timezone_offset": timezoneOffset,
		"points":          points,
	}
	metrics := deriveQuotaHistoryMetrics(snapshots)
	response["data_quality"] = metrics.DataQuality
	var latestSnapshot *model.ChannelQuotaSnapshot
	if len(snapshots) > 0 {
		latestSnapshot = &snapshots[len(snapshots)-1]
	}
	// Use the newest observation, including a failed one, so a stale balance
	// cannot be presented as a current alert state after an upstream failure.
	response["alert"] = deriveQuotaHistoryAlert(latestSnapshot)
	var firstSuccess, lastSuccess *model.ChannelQuotaSnapshot
	var minimum, maximum float64
	for index := range snapshots {
		if snapshots[index].Status != "success" || !finiteQuotaValue(snapshots[index].Available) {
			continue
		}
		if firstSuccess == nil {
			firstSuccess = &snapshots[index]
			minimum, maximum = snapshots[index].Available, snapshots[index].Available
		}
		lastSuccess = &snapshots[index]
		if snapshots[index].Available < minimum {
			minimum = snapshots[index].Available
		}
		if snapshots[index].Available > maximum {
			maximum = snapshots[index].Available
		}
	}
	if firstSuccess != nil && lastSuccess != nil {
		change := lastSuccess.Available - firstSuccess.Available
		changePercent := float64(0)
		if firstSuccess.Available != 0 {
			changePercent = change / firstSuccess.Available * 100
		}
		response["summary"] = gin.H{
			"start_available":     firstSuccess.Available,
			"end_available":       lastSuccess.Available,
			"change":              change,
			"change_percent":      changePercent,
			"minimum":             minimum,
			"maximum":             maximum,
			"drop_rate_per_day":   metrics.DropRatePerDay,
			"forecast_zero_at":    metrics.ForecastZeroAt,
			"forecast_confidence": metrics.ForecastConfidence,
			"data_quality":        metrics.DataQuality,
		}
	}
	if len(snapshots) > 0 {
		last := snapshots[len(snapshots)-1]
		response["unit"] = last.Unit
		response["currency"] = last.Currency
		response["metric_type"] = last.MetricType
		response["window_type"] = last.WindowType
		response["source"] = last.Source
		if last.Status == "success" && finiteQuotaValue(last.Available) {
			current := gin.H{"available": last.Available, "observed_at": last.ObservedAt, "status": last.Status}
			if last.Total != nil && finiteQuotaValue(*last.Total) {
				current["total"] = *last.Total
			}
			response["current"] = current
		} else {
			errorCode := last.ErrorCode
			status := last.Status
			if status == "success" {
				status = "error"
				errorCode = "invalid_balance"
			}
			response["current"] = gin.H{"observed_at": last.ObservedAt, "status": status, "error_code": errorCode}
		}
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
		result, err := updateChannelBalance(channel)
		recordChannelBalanceSnapshot(channel, result, err)
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
func runChannelQuotaSnapshotSyncOnce(ctx context.Context, maxChannels int, report func(processed, total int)) (channelQuotaSnapshotSyncSummary, error) {
	summary := channelQuotaSnapshotSyncSummary{}
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
	channels, err := model.GetChannelsForQuotaSnapshotSync(fetchLimit)
	if err != nil {
		return summary, err
	}
	// The model query is bounded. Multi-key entries are skipped because their
	// aggregate quota is ambiguous; scan a small multiple of the target so a
	// few such channels do not prevent eligible single-key channels from being
	// sampled. A hard attempt cap keeps provider fan-out bounded.
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
			queryErr = sampleCodexChannelUsage(ctx, channel)
		} else {
			result, balanceErr := updateChannelBalance(channel)
			queryErr = balanceErr
			persistErr = recordChannelBalanceSnapshot(channel, result, balanceErr)
		}
		lock.Unlock()
		if queryErr != nil {
			summary.Failed++
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
		if common.RequestInterval > 0 {
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
