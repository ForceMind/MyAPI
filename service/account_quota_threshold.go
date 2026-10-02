package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
)

var ErrAccountQuotaThresholdUnavailable = errors.New("account quota threshold cannot be verified")

// Final DB-only check on the exact credential/header/URL that will be sent.
// Never rotate credentials, poll upstream, or manufacture account consumption.
func ValidateAccountQuotaThresholdDispatch(ctx context.Context, client *http.Client, request *http.Request, info *relaycommon.RelayInfo) error {
	policy, active := common.AccountQuotaThresholdFromContext(ctx)
	if !active {
		return nil
	}
	if !common.ValidAccountQuotaThreshold(policy) || info == nil || info.ChannelMeta == nil || info.ChannelType != constant.ChannelTypeCodex || policy.TokenID <= 0 || policy.TokenID != info.TokenId || policy.Revision <= 0 || client == nil || client.Jar != nil || common.TLSInsecureSkipVerify || !verifiedUpstreamTransport(client.Transport, "chatgpt.com") || request == nil || request.URL == nil {
		return ErrAccountQuotaThresholdUnavailable
	}
	if request.Method != http.MethodPost || request.URL.Scheme != "https" || request.URL.Host != "chatgpt.com" || request.URL.User != nil || request.URL.RawQuery != "" || request.Host != "" && request.Host != "chatgpt.com" || request.Header.Get("Cookie") != "" || len(info.HeadersOverride) != 0 || len(info.ParamOverride) != 0 {
		return ErrAccountQuotaThresholdUnavailable
	}
	switch request.URL.EscapedPath() {
	case "/backend-api/codex/responses", "/backend-api/codex/responses/compact", "/backend-api/codex/alpha/search":
	default:
		return ErrAccountQuotaThresholdUnavailable
	}
	var credential struct {
		AccessToken string `json:"access_token"`
		AccountID   string `json:"account_id"`
	}
	if common.UnmarshalJsonStr(info.ApiKey, &credential) != nil || strings.TrimSpace(credential.AccessToken) == "" || strings.TrimSpace(credential.AccountID) == "" || len(request.Header.Values("Authorization")) != 1 || len(request.Header.Values("ChatGPT-Account-ID")) != 1 || request.Header.Get("Authorization") != "Bearer "+strings.TrimSpace(credential.AccessToken) || request.Header.Get("ChatGPT-Account-ID") != strings.TrimSpace(credential.AccountID) {
		return ErrAccountQuotaThresholdUnavailable
	}
	current, err := model.LookupTokenBudget(ctx, model.DB, info.TokenId)
	if err != nil {
		return err
	}
	if current == nil || current.UserID != info.UserId || !current.AccountThresholdEnabled || current.Revision != policy.Revision || current.AccountMinRemainingBPS != policy.MinimumRemainingBPS || current.AccountMaxAgeSeconds != policy.MaxAgeSeconds {
		return ErrAccountQuotaThresholdUnavailable
	}
	// A synthetic single-key view reuses the existing identity resolution and
	// exact-429 checks without moving a multi-key channel's polling cursor.
	baseURL := "https://chatgpt.com"
	channel := &model.Channel{BaseURL: &baseURL, Id: info.ChannelId, Type: constant.ChannelTypeCodex, Key: info.ApiKey, Status: common.ChannelStatusEnabled}
	_, eligible, err := CodexQuotaEligibleKeys(ctx, channel)
	if err != nil {
		return err
	}
	if !eligible {
		return ErrAccountQuotaThresholdUnavailable
	}
	return nil
}
