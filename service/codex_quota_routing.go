package service

import (
	"context"
	"errors"
	"strings"

	"github.com/ForceMind/MyAPI/common"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
)

var ErrCodexQuotaRoutingUnavailable = errors.New("Codex quota routing state is unavailable")

// CodexQuotaEligibleKeys evaluates the provider-confirmed account attached to
// each enabled credential. A 100% active window or exact usage-limit marker
// excludes that credential until reset or a later healthy provider sample.
// Invalid credentials are not counted as usable; unrelated providers and
// generic 429 observations do not change eligibility.
func CodexQuotaEligibleKeys(ctx context.Context, channel *model.Channel) (map[int]bool, bool, error) {
	if channel == nil {
		return nil, false, ErrCodexQuotaRoutingUnavailable
	}
	if channel.Type != constant.ChannelTypeCodex {
		if _, active := common.AccountQuotaThresholdFromContext(ctx); active {
			return nil, false, nil
		}
		return nil, true, nil
	}
	if policy, active := common.AccountQuotaThresholdFromContext(ctx); active {
		if !common.ValidAccountQuotaThreshold(policy) {
			return nil, false, model.ErrAccountQuotaThresholdInvalid
		}
		if !accountThresholdChannelSupported(channel) {
			return nil, false, nil
		}
	}
	diagnostic, err := inspectCodexQuotaRouting(ctx, channel, false)
	if err != nil {
		return nil, false, err
	}
	return diagnostic.excluded, diagnostic.HasEligibleKey, nil
}

// GetAvailableChannelRoutingPolicy applies the existing account eligibility gate
// to the shared model/endpoint policy. Preview and selection use this same view;
// final dispatch rechecks account state because observations can change.
func GetAvailableChannelRoutingPolicy(ctx context.Context, group, modelName, requestPath string) (model.ChannelRoutingPolicySnapshot, error) {
	view, err := model.GetRuntimeChannelRoutingPolicy(group, modelName, requestPath)
	if err != nil {
		return view, err
	}
	candidates := make([]model.ChannelRoutingCandidate, 0)
	rejected := view.Policy.Rejected
	for _, tier := range view.Policy.Tiers {
		for _, candidate := range tier.Candidates {
			candidates = append(candidates, evaluateRelayRoutingCandidate(ctx, candidate.ChannelRoutingCandidate, requestPath))
		}
	}
	candidates = append(candidates, rejected...)
	view.Policy = model.BuildChannelRoutingPolicy(candidates)
	return view, nil
}

// evaluateRelayRoutingCandidate intersects quota, transient health and request
// exclusions once. Preview and runtime selection consume the same result.
func evaluateRelayRoutingCandidate(ctx context.Context, value model.ChannelRoutingCandidate, path string) model.ChannelRoutingCandidate {
	state := RelayFailoverFromContext(ctx)
	if state != nil && (value.ModelRoute == nil || value.ModelRoute.UpstreamModel != state.Target) {
		value.RouteError = "retry_target_mismatch"
		return value
	}
	_, thresholdActive := common.AccountQuotaThresholdFromContext(ctx)
	needsQuota := value.ChannelType == constant.ChannelTypeCodex || thresholdActive
	needsHealth := common.RelayFailureCooldownSeconds > 0 && (value.ChannelType == constant.ChannelTypeOpenAI || value.ChannelType == constant.ChannelTypeCodex)
	if !needsQuota && !needsHealth && state == nil {
		return value
	}
	channel, err := model.CacheGetChannel(value.ChannelID)
	if err != nil {
		value.RouteError = "channel_state_unavailable"
		return value
	}
	excluded, eligible, err := CodexQuotaEligibleKeys(ctx, channel)
	if err != nil {
		value.RouteError = "account_state_unavailable"
		return value
	}
	if !eligible {
		value.RouteError = "account_not_eligible"
		return value
	}
	excluded, eligible, recovery, err := RelayCooldownEligibleKeys(ctx, channel, path, excluded)
	value.CooldownUntil = recovery
	if err != nil {
		value.RouteError = "account_state_unavailable"
		return value
	}
	if !eligible {
		value.RouteError = "account_not_eligible"
		if recovery > 0 {
			value.RouteError = "account_cooling_down"
		}
		return value
	}
	if _, eligible := RelayFailoverEligibleKeys(ctx, channel, excluded); !eligible {
		value.RouteError = "request_failed_credentials"
	}
	return value
}

func getRandomQuotaSatisfiedChannel(ctx context.Context, group, modelName string, retry int, requestPath string) (*model.Channel, error) {
	view, err := GetAvailableChannelRoutingPolicy(ctx, group, modelName, requestPath)
	if err != nil {
		return nil, err
	}
	if RelayFailoverFromContext(ctx) != nil {
		retry = 0
	}
	channelID, found, err := model.SelectChannelFromRoutingPolicy(view.Policy, retry)
	if err != nil || !found {
		return nil, err
	}
	return model.CacheGetChannel(channelID)
}

func accountThresholdChannelSupported(channel *model.Channel) bool {
	return channel != nil && channel.Type == constant.ChannelTypeCodex && !common.TLSInsecureSkipVerify && strings.TrimRight(strings.TrimSpace(channel.GetBaseURL()), "/") == "https://chatgpt.com" && len(channel.GetHeaderOverride()) == 0 && len(channel.GetParamOverride()) == 0
}
