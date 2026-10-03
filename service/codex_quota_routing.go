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

func getRandomQuotaSatisfiedChannel(ctx context.Context, group, modelName string, retry int, requestPath string) (*model.Channel, error) {
	channel, err := model.GetRandomSatisfiedChannel(group, modelName, retry, requestPath)
	_, thresholdActive := common.AccountQuotaThresholdFromContext(ctx)
	if err != nil || channel == nil || (channel.Type != constant.ChannelTypeCodex && !thresholdActive) {
		return channel, err
	}
	_, usable, err := CodexQuotaEligibleKeys(ctx, channel)
	if err != nil || usable {
		return channel, err
	}
	view, err := model.GetRuntimeChannelRoutingPolicy(group, modelName, requestPath)
	if err != nil {
		return nil, err
	}
	filtered := make([]model.ChannelRoutingCandidate, 0)
	for _, tier := range view.Policy.Tiers {
		for _, candidate := range tier.Candidates {
			if candidate.ChannelType == constant.ChannelTypeCodex || thresholdActive {
				candidateChannel, lookupErr := model.CacheGetChannel(candidate.ChannelID)
				if lookupErr != nil {
					return nil, lookupErr
				}
				_, candidateUsable, checkErr := CodexQuotaEligibleKeys(ctx, candidateChannel)
				if checkErr != nil {
					return nil, checkErr
				}
				if !candidateUsable {
					continue
				}
			}
			filtered = append(filtered, candidate.ChannelRoutingCandidate)
		}
	}
	policy := model.BuildChannelRoutingPolicy(filtered)
	channelID, found, err := model.SelectChannelFromRoutingPolicy(policy, retry)
	if err != nil || !found {
		return nil, err
	}
	return model.CacheGetChannel(channelID)
}

func accountThresholdChannelSupported(channel *model.Channel) bool {
	return channel != nil && channel.Type == constant.ChannelTypeCodex && !common.TLSInsecureSkipVerify && strings.TrimRight(strings.TrimSpace(channel.GetBaseURL()), "/") == "https://chatgpt.com" && len(channel.GetHeaderOverride()) == 0 && len(channel.GetParamOverride()) == 0
}
