package accesspolicy

import "sort"

// AssignedPolicy is an administrator-owned constraint envelope, separate from
// mutable user and token fields. A nil list inherits existing access; an
// explicitly empty list denies every value in that dimension.
type AssignedPolicy struct {
	Enabled        bool     `json:"enabled"`
	PublicModels   []string `json:"public_models"`
	UpstreamModels []string `json:"upstream_models"`
	ChannelIDs     []int    `json:"channel_ids"`
}

// NormalizeAssignedPolicy applies the same model-name validators as snapshots
// and returns detached, sorted sets while preserving nil versus empty lists.
func NormalizeAssignedPolicy(input AssignedPolicy) (AssignedPolicy, error) {
	public, err := normalizeStringList(StringList{Values: input.PublicModels}, MaxModelLength, false)
	if err != nil {
		return AssignedPolicy{}, err
	}
	upstream, err := normalizeStringList(StringList{Values: input.UpstreamModels}, MaxModelLength, false)
	if err != nil {
		return AssignedPolicy{}, err
	}
	if len(input.ChannelIDs) > MaxListValues {
		return AssignedPolicy{}, ErrInvalidList
	}
	output := AssignedPolicy{
		Enabled:        input.Enabled,
		PublicModels:   public.Values,
		UpstreamModels: upstream.Values,
	}
	if input.ChannelIDs != nil {
		output.ChannelIDs = make([]int, 0, len(input.ChannelIDs))
		seen := make(map[int]struct{}, len(input.ChannelIDs))
		for _, id := range input.ChannelIDs {
			if id <= 0 {
				return AssignedPolicy{}, ErrInvalidList
			}
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				output.ChannelIDs = append(output.ChannelIDs, id)
			}
		}
		sort.Ints(output.ChannelIDs)
	}
	return output, nil
}

// EvaluateAssignedPolicy evaluates a normalized assignment against the exact
// public model, resolved upstream model, and selected channel. It never grants
// access beyond the legacy authorization outcome; an empty result means only
// that this assignment adds no denial. Disabled assignments deny all access.
func EvaluateAssignedPolicy(policy AssignedPolicy, publicModel, upstreamModel string, channelID int) []string {
	if !policy.Enabled {
		return []string{"policy_disabled"}
	}
	reasons := make([]string, 0, 3)
	if policy.PublicModels != nil {
		allowed := false
		for _, value := range policy.PublicModels {
			if value == publicModel {
				allowed = true
				break
			}
		}
		if !allowed {
			reasons = append(reasons, "public_model_denied")
		}
	}
	if policy.UpstreamModels != nil {
		allowed := false
		for _, value := range policy.UpstreamModels {
			if value == upstreamModel {
				allowed = true
				break
			}
		}
		if !allowed {
			reasons = append(reasons, "upstream_model_denied")
		}
	}
	if policy.ChannelIDs != nil {
		allowed := false
		for _, id := range policy.ChannelIDs {
			if id == channelID {
				allowed = true
				break
			}
		}
		if !allowed {
			reasons = append(reasons, "channel_denied")
		}
	}
	return reasons
}
