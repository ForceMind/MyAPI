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
	_, thresholdActive := common.AccountQuotaThresholdFromContext(ctx)
	candidates := make([]model.ChannelRoutingCandidate, 0)
	rejected := view.Policy.Rejected
	for _, tier := range view.Policy.Tiers {
		for _, candidate := range tier.Candidates {
			value := candidate.ChannelRoutingCandidate
			if candidate.ChannelType == constant.ChannelTypeCodex || thresholdActive {
				channel, lookupErr := model.CacheGetChannel(candidate.ChannelID)
				if lookupErr != nil {
					value.RouteError = "channel_state_unavailable"
				} else {
					_, usable, checkErr := CodexQuotaEligibleKeys(ctx, channel)
					if checkErr != nil {
						value.RouteError = "account_state_unavailable"
					} else if !usable {
						value.RouteError = "account_not_eligible"
					}
				}
			}
			candidates = append(candidates, value)
		}
	}
	candidates = append(candidates, rejected...)
	view.Policy = model.BuildChannelRoutingPolicy(candidates)
	return view, nil
}

func getRandomQuotaSatisfiedChannel(ctx context.Context, group, modelName string, retry int, requestPath string) (*model.Channel, error) {
	view, err := GetAvailableChannelRoutingPolicy(ctx, group, modelName, requestPath)
	if err != nil {
		return nil, err
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
