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

	"github.com/google/uuid"
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

// A valid custom relay does not need a balance route. Keep that absence
// distinct from broken configuration and a failed configured query.
var errChannelBalanceNotConfigured = fmt.Errorf("balance query route not configured: %w", errChannelQuotaUnsupported)

// Balance endpoints are provider-controlled and some providers do not close
// stalled connections promptly.  A bounded request context keeps the
// scheduled sampler from accumulating goroutines while preserving the
// existing RelayTimeout setting as an additional client-level limit.
const channelBalanceRequestTimeout = 30 * time.Second

type channelBalanceResult struct {
	Balance           float64
	RawResponse       string
	ProviderAccountID string
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
	result, err := queryChannelSiliconFlowBalance(ctx, channel)
	return result.Balance, err
}

func queryChannelSiliconFlowBalance(ctx context.Context, channel *model.Channel) (channelBalanceResult, error) {
	url := "https://api.siliconflow.cn/v1/user/info"
	body, err := GetResponseBody(ctx, "GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return channelBalanceResult{}, err
	}
	response := SiliconFlowUsageResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return channelBalanceResult{}, err
	}
	if response.Code != 20000 {
		return channelBalanceResult{}, fmt.Errorf("code: %d, message: %s", response.Code, response.Message)
	}
	balance, err := strconv.ParseFloat(response.Data.TotalBalance, 64)
	if err != nil {
		return channelBalanceResult{}, err
	}
	return channelBalanceResult{Balance: balance, ProviderAccountID: strings.TrimSpace(response.Data.ID)}, nil
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
	var settings dto.ChannelOtherSettings
	if err := common.UnmarshalJsonStr(channel.OtherSettings, &settings); err != nil {
		return channelBalanceResult{}, sanitizeFetchModelsError(err, key)
	}
	if err := settings.AdvancedCustom.Validate(); err != nil {
		return channelBalanceResult{}, sanitizeFetchModelsError(err, key)
	}
	if _, configured := settings.AdvancedCustom.BalanceRoute(); !configured {
		return channelBalanceResult{}, errChannelBalanceNotConfigured
	}
	info := &relaycommon.RelayInfo{
		RelayFormat:    types.RelayFormatOpenAI,
		RelayMode:      relayconstant.RelayModeUnknown,
		RequestURLPath: dto.AdvancedCustomBalancePath,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:          constant.ChannelTypeAdvancedCustom,
			ChannelBaseUrl:       channel.GetBaseURL(),
			ApiKey:               key,
			ChannelOtherSettings: settings,
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

func queryChannelBalanceForCredential(ctx context.Context, channel *model.Channel, credential string) (channelBalanceResult, error) {
	if channel == nil || strings.TrimSpace(credential) == "" {
		return channelBalanceResult{}, errors.New("missing quota sampling credential")
	}
	copyChannel := *channel
	copyChannel.Key = credential
	if copyChannel.Type == constant.ChannelTypeAdvancedCustom {
		return fetchAdvancedCustomBalance(ctx, &copyChannel)
	}
	if copyChannel.Type == constant.ChannelTypeSiliconFlow {
		return queryChannelSiliconFlowBalance(ctx, &copyChannel)
	}
	balance, err := updateStandardChannelBalance(ctx, &copyChannel)
	return channelBalanceResult{Balance: balance}, err
}

func resolveQuotaSamplingIdentity(ctx context.Context, channelType int, identityKind string, material []byte) (model.ChannelQuotaResolvedIdentity, error) {
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	if err != nil {
		return model.ChannelQuotaResolvedIdentity{}, err
	}
	provider := "channel_type_" + strconv.Itoa(channelType)
	return model.ResolveChannelQuotaIdentity(ctx, model.DB, keyring, provider, identityKind, material)
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
		if errors.Is(queryErr, errChannelBalanceNotConfigured) {
			snapshot.Status = "unsupported"
			snapshot.ErrorCode = "quota_not_configured"
			snapshot.ErrorMessage = "balance query route is not configured"
		} else if errors.Is(queryErr, errChannelQuotaUnsupported) {
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

func recordChannelBalanceSnapshotWithIdentity(channel *model.Channel, result channelBalanceResult, queryErr error, identity model.ChannelQuotaResolvedIdentity, sampleID string, identityErr error) error {
	if channel == nil {
		return nil
	}
	persistCtx, cancel := context.WithTimeout(context.Background(), channelQuotaPersistenceTimeout)
	defer cancel()
	observedAt, err := model.ReadDatabaseUnixTime(persistCtx, model.DB)
	if err != nil {
		return err
	}
	snapshot := &model.ChannelQuotaSnapshot{
		ChannelId:       channel.Id,
		ObservedAt:      observedAt,
		SampleID:        sampleID,
		SubjectRef:      identity.SubjectRef,
		IdentityQuality: identity.Quality,
		Unit:            "usd",
		MetricType:      "balance",
		WindowType:      "none",
		Source:          fmt.Sprintf("channel_type_%d", channel.Type),
	}
	if identityErr != nil || identity.SubjectRef == "" {
		snapshot.Status = "error"
		snapshot.ErrorCode = "identity_unavailable"
		snapshot.ErrorMessage = "quota sampling identity is unavailable"
		return recordQuotaSamplingSnapshots([]model.ChannelQuotaSnapshot{*snapshot})
	}
	if queryErr != nil {
		if errors.Is(queryErr, errChannelBalanceNotConfigured) {
			snapshot.Status = "unsupported"
			snapshot.ErrorCode = "quota_not_configured"
			snapshot.ErrorMessage = "balance query route is not configured"
		} else if errors.Is(queryErr, errChannelQuotaUnsupported) {
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

func parseQuotaHistoryGranularity(value string, start, end int64) (service.QuotaHistoryGranularity, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return service.QuotaHistoryRaw, nil
	}
	if value == string(service.QuotaHistoryAuto) {
		switch {
		case end-start <= 6*60*60:
			return service.QuotaHistoryMinute, nil
		case end-start <= 24*60*60:
			return service.QuotaHistoryFiveMinutes, nil
		case end-start <= 7*24*60*60:
			return service.QuotaHistoryFifteenMinutes, nil
		case end-start <= 30*24*60*60:
			return service.QuotaHistoryHour, nil
		case end-start <= 90*24*60*60:
			return service.QuotaHistoryDay, nil
		default:
			return service.QuotaHistoryWeek, nil
		}
	}
	granularity := service.QuotaHistoryGranularity(value)
	switch granularity {
	case service.QuotaHistoryRaw, service.QuotaHistoryMinute, service.QuotaHistoryFiveMinutes, service.QuotaHistoryFifteenMinutes,
		service.QuotaHistoryHour, service.QuotaHistoryDay, service.QuotaHistoryWeek:
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
		if parseErr != nil || parsed <= 0 || parsed > service.MaxQuotaHistoryPointLimit {
			common.ApiError(c, fmt.Errorf("quota history limit must be between 1 and %d", service.MaxQuotaHistoryPointLimit))
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
	seriesFilter := model.ChannelQuotaSnapshotQuery{
		MetricType: strings.TrimSpace(c.Query("metric_type")),
		WindowType: strings.TrimSpace(c.Query("window_type")),
		Source:     strings.TrimSpace(c.Query("source")),
		PlanType:   strings.TrimSpace(c.Query("plan_type")),
		Unit:       strings.TrimSpace(c.Query("unit")),
		Currency:   strings.TrimSpace(c.Query("currency")),
	}
	seriesFilter.WindowSeconds, err = parseQuotaHistoryWindowSeconds(c.Query("window_seconds"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	requestContext := context.Background()
	if c.Request != nil && c.Request.Context() != nil {
		requestContext = c.Request.Context()
	}
	response, err := service.GetChannelQuotaHistory(requestContext, service.ChannelQuotaHistoryRequest{
		ChannelID:            id,
		Start:                start,
		End:                  end,
		Limit:                limit,
		Granularity:          granularity,
		RequestedGranularity: strings.ToLower(strings.TrimSpace(c.Query("granularity"))),
		TimezoneOffset:       timezoneOffset,
		RateWindowSeconds:    rateWindowSeconds,
		EWMAHalfLifeSeconds:  ewmaHalfLifeSeconds,
		SeriesFilter:         seriesFilter,
	})
	if err != nil {
		common.ApiError(c, err)
		return
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

const channelQuotaSamplingPerChannelDefault = 4
const channelQuotaSamplingPerChannelHardMax = 32

type channelQuotaSamplingCandidate struct {
	channel    *model.Channel
	credential string
	identity   model.ChannelQuotaResolvedIdentity
	target     model.ChannelQuotaSamplingTarget
}

func buildChannelQuotaSamplingCandidates(ctx context.Context, channels []*model.Channel) ([]channelQuotaSamplingCandidate, int, error) {
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	if err != nil {
		return nil, 0, err
	}
	perChannel := make([][]channelQuotaSamplingCandidate, 0, len(channels))
	totalTargets := 0
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		keys := channel.GetEnabledQuotaSamplingKeys()
		sort.Strings(keys)
		excessKeys := 0
		var expansion []model.ChannelQuotaSamplingExpansion
		completeCredentialSet := len(keys) <= channelQuotaSamplingPerChannelHardMax
		if !completeCredentialSet {
			excessKeys = len(keys) - channelQuotaSamplingPerChannelHardMax
			if channel.QuotaSamplingCursor < 0 {
				return nil, 0, errors.New("invalid quota expansion cursor")
			}
			start := int(channel.QuotaSamplingCursor % int64(len(keys)))
			selected := make([]string, channelQuotaSamplingPerChannelHardMax)
			for index := range selected {
				selected[index] = keys[(start+index)%len(keys)]
			}
			expansion = []model.ChannelQuotaSamplingExpansion{{ExpectedCursor: channel.QuotaSamplingCursor, NextCursor: int64((start + len(selected)) % len(keys))}}
			keys = selected
		}
		bySubject := make(map[string]channelQuotaSamplingCandidate, len(keys))
		identities := make([]model.ChannelQuotaSamplingIdentity, 0, len(keys))
		for _, credential := range keys {
			identity, resolveErr := model.ResolveChannelQuotaIdentity(ctx, model.DB, keyring, "channel_type_"+strconv.Itoa(channel.Type), common.ChannelQuotaIdentityKindCredential, []byte(credential))
			if resolveErr != nil {
				return nil, 0, resolveErr
			}
			identities = append(identities, model.ChannelQuotaSamplingIdentity{SubjectRef: identity.SubjectRef, IdentityQuality: identity.Quality})
			bySubject[identity.SubjectRef] = channelQuotaSamplingCandidate{channel: channel, credential: credential, identity: identity}
		}
		targets, syncErr := model.SyncChannelQuotaSamplingTargets(ctx, model.DB, channel.Id, channel.Key, completeCredentialSet, identities, expansion...)
		if syncErr != nil {
			return nil, 0, syncErr
		}
		if len(expansion) == 1 {
			channel.QuotaSamplingCursor = expansion[0].NextCursor
		}
		targets = collapseConfirmedQuotaSamplingTargets(targets)
		totalTargets += len(targets) + excessKeys
		if len(targets) > channelQuotaSamplingPerChannelDefault {
			targets = targets[:channelQuotaSamplingPerChannelDefault]
		}
		channelCandidates := make([]channelQuotaSamplingCandidate, 0, len(targets))
		for _, target := range targets {
			candidate, ok := bySubject[target.SubjectRef]
			if !ok {
				continue
			}
			candidate.target = target
			channelCandidates = append(channelCandidates, candidate)
		}
		perChannel = append(perChannel, channelCandidates)
	}
	candidates := make([]channelQuotaSamplingCandidate, 0)
	for round := 0; round < channelQuotaSamplingPerChannelDefault; round++ {
		for _, channelCandidates := range perChannel {
			if round < len(channelCandidates) {
				candidates = append(candidates, channelCandidates[round])
			}
		}
	}
	return candidates, totalTargets, nil
}

func collapseConfirmedQuotaSamplingTargets(targets []model.ChannelQuotaSamplingTarget) []model.ChannelQuotaSamplingTarget {
	result := make([]model.ChannelQuotaSamplingTarget, 0, len(targets))
	confirmedIndex := make(map[string]int)
	for _, target := range targets {
		if target.ConfirmedSubjectRef == "" {
			result = append(result, target)
			continue
		}
		index, exists := confirmedIndex[target.ConfirmedSubjectRef]
		if !exists {
			confirmedIndex[target.ConfirmedSubjectRef] = len(result)
			result = append(result, target)
			continue
		}
		if quotaSamplingTargetPreference(target) < quotaSamplingTargetPreference(result[index]) ||
			(quotaSamplingTargetPreference(target) == quotaSamplingTargetPreference(result[index]) && target.SubjectRef < result[index].SubjectRef) {
			result[index] = target
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].LastAttemptAt != result[j].LastAttemptAt {
			return result[i].LastAttemptAt < result[j].LastAttemptAt
		}
		return result[i].SubjectRef < result[j].SubjectRef
	})
	return result
}

func quotaSamplingTargetPreference(target model.ChannelQuotaSamplingTarget) int {
	switch target.LastResult {
	case "sampled":
		return 0
	case "duplicate":
		return 1
	case "":
		return 2
	case "unsupported":
		return 3
	case "busy":
		return 4
	default:
		return 5
	}
}

func markQuotaSamplingAttempt(channelID int, subjectRef, result, confirmedSubjectRef string, proof ...model.ChannelQuotaSamplingProof) error {
	ctx, cancel := context.WithTimeout(context.Background(), channelQuotaPersistenceTimeout)
	defer cancel()
	return model.MarkChannelQuotaSamplingTargetAttempt(ctx, model.DB, channelID, subjectRef, result, confirmedSubjectRef, proof...)
}

// runChannelQuotaSnapshotSyncOnce samples enabled credential targets using
// the normalized balance query path.  The per-channel polling lock prevents a
// scheduled pass from racing with a manual balance query or multi-key state
// update.  A busy channel is skipped and retried on the next scheduled pass;
// this avoids waiting behind a potentially slow provider request.
func runChannelQuotaSnapshotSyncOnce(ctx context.Context, maxChannels int, report func(processed, total int)) (summary channelQuotaSnapshotSyncSummary, runErr error) {
	credentialAttempts := 0
	perChannelBudgetExhausted := false
	candidateLimitReached := false
	defer func() {
		summary.Deferred = summary.Considered - summary.Sampled - summary.Failed - summary.DuplicateAccountObservations
		if summary.Deferred < 0 {
			summary.Deferred = 0
		}
		summary.DeferredKeys = summary.Deferred
		summary.BudgetExhausted = errors.Is(runErr, context.DeadlineExceeded) ||
			(summary.DeferredKeys > 0 && (credentialAttempts >= maxChannels || perChannelBudgetExhausted))
		summary.SourceComplete = runErr == nil && summary.DeferredKeys == 0 && !summary.BudgetExhausted && !candidateLimitReached
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
	candidateLimitReached = len(channels) >= fetchLimit
	candidates, totalTargets, err := buildChannelQuotaSamplingCandidates(ctx, channels)
	if err != nil {
		for _, channel := range channels {
			_ = recordChannelBalanceSnapshotWithIdentity(channel, channelBalanceResult{}, err, model.ChannelQuotaResolvedIdentity{}, uuid.NewString(), err)
		}
		return summary, err
	}
	perChannelBudgetExhausted = totalTargets > len(candidates)
	// Keep extra candidates so busy polling locks cannot block other accounts.
	// The budget counts credential attempts: a provider query may require more
	// than one HTTP request (balance/usage, or OAuth refresh plus retry).
	summary.Considered = totalTargets
	if report != nil {
		report(0, summary.Considered)
	}
	var firstPersistErr error
	confirmedAccounts := make(map[string]string)
	for index, candidate := range candidates {
		channel := candidate.channel
		if credentialAttempts >= maxChannels {
			break
		}
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if channel == nil || channel.Status != common.ChannelStatusEnabled {
			summary.Skipped++
			if report != nil {
				report(index+1, summary.Considered)
			}
			continue
		}
		lock := model.GetChannelPollingLock(channel.Id)
		if !lock.TryLock() {
			summary.Skipped++
			if markErr := markQuotaSamplingAttempt(channel.Id, candidate.identity.SubjectRef, "busy", ""); markErr != nil {
				summary.PersistFailed++
				if firstPersistErr == nil {
					firstPersistErr = markErr
				}
			}
			if report != nil {
				report(index+1, summary.Considered)
			}
			continue
		}
		var queryErr error
		var persistErr error
		duplicateAccount := false
		confirmedSubjectForTarget := ""
		verifiedSampleID := ""
		credentialAttempts++
		if channel.Type == constant.ChannelTypeCodex {
			// Codex OAuth exposes subscription windows through the official WHAM
			// usage endpoint rather than the generic balance endpoint. Keep this
			// branch inside the same bounded polling lock and sampler budget.
			attemptIdentity := candidate.identity
			samplingErr := sampleCodexCredentialUsage(ctx, channel, candidate.credential, candidate.identity, func(subjectRef string) bool {
				confirmedSubjectForTarget = subjectRef
				accountKey := strconv.Itoa(channel.Id) + "\x00" + subjectRef
				if _, exists := confirmedAccounts[accountKey]; exists {
					duplicateAccount = true
					return false
				}
				confirmedAccounts[accountKey] = candidate.identity.SubjectRef
				return true
			}, func(subjectRef string) {
				accountKey := strconv.Itoa(channel.Id) + "\x00" + subjectRef
				if confirmedAccounts[accountKey] == candidate.identity.SubjectRef {
					delete(confirmedAccounts, accountKey)
				}
				if confirmedSubjectForTarget == subjectRef {
					confirmedSubjectForTarget = ""
				}
			}, &attemptIdentity, &verifiedSampleID)
			if attemptIdentity.SubjectRef != candidate.identity.SubjectRef {
				// The request began under the old target lifecycle. Keep candidate.target
				// unchanged so this response cannot attest a replacement target version;
				// the replacement must produce its own proof on a later sampling pass.
				persistCtx, persistCancel := context.WithTimeout(context.Background(), channelQuotaPersistenceTimeout)
				ensureErr := model.EnsureChannelQuotaSamplingTarget(
					persistCtx, model.DB, channel.Id,
					model.ChannelQuotaSamplingIdentity{SubjectRef: attemptIdentity.SubjectRef, IdentityQuality: attemptIdentity.Quality},
				)
				persistErr = errors.Join(persistErr, ensureErr)
				persistCancel()
				candidate.identity = attemptIdentity
			}
			queryErr = samplingErr
			var classifiedErr *channelQuotaSamplingError
			if errors.As(samplingErr, &classifiedErr) {
				queryErr = classifiedErr.QueryErr
				persistErr = errors.Join(persistErr, classifiedErr.PersistErr)
			}
		} else {
			requestCtx, cancel := context.WithTimeout(ctx, channelBalanceRequestTimeout)
			result, balanceErr := queryChannelBalanceForCredential(requestCtx, channel, candidate.credential)
			cancel()
			queryErr = balanceErr
			observationIdentity := candidate.identity
			if balanceErr == nil && result.ProviderAccountID != "" {
				observationIdentity, balanceErr = resolveQuotaSamplingIdentity(ctx, channel.Type, common.ChannelQuotaIdentityKindProviderAccount, []byte(result.ProviderAccountID))
				queryErr = balanceErr
				if balanceErr == nil {
					confirmedSubjectForTarget = observationIdentity.SubjectRef
				}
			}
			if queryErr == nil && observationIdentity.Quality == model.ChannelQuotaIdentityQualityProviderConfirmed {
				accountKey := strconv.Itoa(channel.Id) + "\x00" + observationIdentity.SubjectRef
				if _, exists := confirmedAccounts[accountKey]; exists {
					duplicateAccount = true
				}
			}
			if !duplicateAccount {
				persistErr = recordChannelBalanceSnapshotWithIdentity(channel, result, queryErr, observationIdentity, uuid.NewString(), nil)
				if persistErr == nil && queryErr == nil && observationIdentity.Quality == model.ChannelQuotaIdentityQualityProviderConfirmed {
					accountKey := strconv.Itoa(channel.Id) + "\x00" + observationIdentity.SubjectRef
					confirmedAccounts[accountKey] = candidate.identity.SubjectRef
				}
			}
		}
		lock.Unlock()
		resultState := "sampled"
		if duplicateAccount {
			resultState = "duplicate"
		} else if queryErr != nil {
			resultState = "failed"
			if errors.Is(queryErr, errChannelQuotaUnsupported) {
				resultState = "unsupported"
			}
		}
		var proof []model.ChannelQuotaSamplingProof
		if resultState == "sampled" && verifiedSampleID != "" && confirmedSubjectForTarget != "" &&
			candidate.target.SubjectRef == candidate.identity.SubjectRef && candidate.target.EvidenceVersion > 0 {
			proof = append(proof, model.ChannelQuotaSamplingProof{
				ExpectedVersion: candidate.target.EvidenceVersion,
				SampleID:        verifiedSampleID,
			})
		}
		if markErr := markQuotaSamplingAttempt(channel.Id, candidate.identity.SubjectRef, resultState, confirmedSubjectForTarget, proof...); markErr != nil {
			persistErr = errors.Join(persistErr, markErr)
		}
		if duplicateAccount {
			summary.DuplicateAccountObservations++
		} else if queryErr != nil {
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
		if common.RequestInterval > 0 && index+1 < len(candidates) && credentialAttempts < maxChannels {
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
	if service.UpstreamAcceptanceModeEnabled() {
		return
	}
	for {
		time.Sleep(time.Duration(frequency) * time.Minute)
		common.SysLog("updating all channels")
		_ = updateAllChannelsBalance()
		common.SysLog("channels update done")
	}
}
