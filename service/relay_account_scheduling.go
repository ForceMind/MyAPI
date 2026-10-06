package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
)

const relayDispatchChannelKey = "relay_dispatch_channel"

func RelayAccountSchedulingSupported(channel *model.Channel, path string) bool {
	return channel != nil && (channel.Type == constant.ChannelTypeOpenAI && (path == "/v1/chat/completions" || path == "/v1/responses") || channel.Type == constant.ChannelTypeCodex && path == "/v1/responses")
}

// RelayAccountIdentity reuses Codex's existing opaque account reference. Generic
// providers have only credential identity: no claim of provider account linkage.
// Neither reference is exposed in diagnostics or API responses.
func RelayAccountIdentity(channel *model.Channel, key string) string {
	if channel == nil || strings.TrimSpace(key) == "" {
		return ""
	}
	if channel.Type == constant.ChannelTypeCodex {
		var credential struct {
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
		}
		if common.UnmarshalJsonStr(key, &credential) != nil || strings.TrimSpace(credential.AccessToken) == "" {
			return ""
		}
		return model.ChannelQuotaAccountRef("codex", credential.AccountID)
	}
	if channel.Type == constant.ChannelTypeOpenAI {
		return model.ChannelQuotaAccountRef("relay_openai_credential", key)
	}
	return ""
}

// RelayCooldownEligibleKeys is shared by preview, initial selection and final
// dispatch. Disabled means no new database read, preserving the legacy default.
func RelayCooldownEligibleKeys(ctx context.Context, channel *model.Channel, path string, excluded map[int]bool) (map[int]bool, bool, int64, error) {
	if common.RelayFailureCooldownSeconds <= 0 || !RelayAccountSchedulingSupported(channel, path) {
		return excluded, true, 0, nil
	}
	holds, err := model.ReadRelayAccountHolds(ctx, model.DB, channel)
	if err != nil {
		return excluded, false, 0, err
	}
	result := make(map[int]bool, len(excluded))
	for index, blocked := range excluded {
		result[index] = blocked
	}
	byIdentity := make(map[string]int64, len(holds))
	for _, hold := range holds {
		byIdentity[hold.Identity] = hold.Until
	}
	keys := []string{channel.Key}
	if channel.ChannelInfo.IsMultiKey {
		keys = channel.GetKeys()
	}
	eligible := false
	var recovery int64
	for index, key := range keys {
		identity := RelayAccountIdentity(channel, key)
		until := byIdentity[identity]
		status, has := channel.ChannelInfo.MultiKeyStatusList[index]
		enabled := !channel.ChannelInfo.IsMultiKey || !has || status == common.ChannelStatusEnabled
		if until > 0 {
			result[index] = true
			if !excluded[index] && enabled && (recovery == 0 || until < recovery) {
				recovery = until
			}
		}
		if !result[index] && identity != "" && enabled {
			eligible = true
		}
	}
	return result, eligible, recovery, nil
}

// Retry-After accepts delta seconds and HTTP dates. Malformed/negative/past
// values use the configured fallback. Valid huge values are capped to 300s.
func relayCooldownSeconds(retryAfter string, fallback int, now time.Time) int {
	if fallback <= 0 {
		return 0
	}
	if fallback > 300 {
		fallback = 300
	}
	value := strings.TrimSpace(retryAfter)
	if value == "" {
		return fallback
	}
	digits := true
	for _, r := range value {
		if r < '0' || r > '9' {
			digits = false
			break
		}
	}
	if digits {
		seconds, err := strconv.ParseUint(value, 10, 64)
		if err != nil || seconds > 300 {
			return 300
		}
		return int(seconds)
	}
	date, err := http.ParseTime(value)
	if err != nil || !date.After(now) {
		return fallback
	}
	remaining := date.Sub(now)
	if remaining >= 300*time.Second {
		return 300
	}
	return int((remaining + time.Second - 1) / time.Second)
}

// ObserveRelayTransientFailure changes FUTURE eligibility only. It never
// clears dispatch uncertainty, refunds funds, or authorizes a current retry.
func ObserveRelayTransientFailure(c *gin.Context, response *http.Response, transportErr error) {
	if c == nil || c.Request == nil || common.RelayFailureCooldownSeconds <= 0 {
		return
	}
	state := RelayFailoverFromContext(c.Request.Context())
	if state == nil || state.clientContext != nil && state.clientContext.Err() != nil || errors.Is(transportErr, context.Canceled) {
		return
	}
	raw, ok := c.Get(relayDispatchChannelKey)
	channel, okChannel := raw.(*model.Channel)
	if !ok || !okChannel || !RelayAccountSchedulingSupported(channel, c.Request.URL.Path) {
		return
	}
	if transportErr != nil && !relayTransientTransportError(transportErr) {
		return
	}
	seconds := common.RelayFailureCooldownSeconds
	if transportErr == nil {
		if response == nil {
			return
		}
		if response.StatusCode != http.StatusTooManyRequests && (response.StatusCode < 500 || response.StatusCode > 599) {
			return
		}
		if response.StatusCode == http.StatusTooManyRequests {
			seconds = relayCooldownSeconds(response.Header.Get("Retry-After"), seconds, time.Now())
		}
	}
	if seconds <= 0 {
		return
	}
	key := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	identity := RelayAccountIdentity(channel, key)
	if identity == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 2*time.Second)
	defer cancel()
	recorded, err := model.RecordRelayAccountHold(ctx, model.DB, channel, key, identity, seconds)
	if err != nil {
		common.SysError("relay transient hold persistence failed")
		return
	}
	if recorded {
		c.Set("relay_cooldown_seconds", seconds)
		if len(state.Attempts) > 0 {
			state.Attempts[len(state.Attempts)-1].CooldownSeconds = seconds
		}
		if _, err := model.CleanupRelayAccountHolds(ctx, model.DB, 32); err != nil {
			common.SysError("relay transient hold cleanup failed")
		}
	}
}

// Do errors can be local request validation failures. Only recognizable
// network failures may cool an account; an unknown error is not health evidence.
func relayTransientTransportError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var addressErr *net.AddrError
	if errors.As(err, &addressErr) {
		return false
	}
	var operationErr *net.OpError
	var dnsErr *net.DNSError
	return errors.As(err, &operationErr) || errors.As(err, &dnsErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded)
}
