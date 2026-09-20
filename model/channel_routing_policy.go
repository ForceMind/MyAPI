package model

import (
	cryptorand "crypto/rand"
	"errors"
	"math/big"
	"sort"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
)

// ChannelRoutingCandidate is the non-sensitive input used to build the runtime
// routing policy and the administrator routing preview.
type ChannelRoutingCandidate struct {
	ChannelID   int
	ChannelName string
	ChannelType int
	Priority    int64
	Weight      uint
}

// ChannelRoutingCandidatePolicy describes one candidate's effective share
// within a single priority tier.
type ChannelRoutingCandidatePolicy struct {
	ChannelRoutingCandidate
	EffectiveWeight uint
	ExpectedShare   float64
}

// ChannelRoutingTier is one priority fallback layer. Higher priority tiers are
// attempted before lower priority tiers.
type ChannelRoutingTier struct {
	Priority   int64
	Candidates []ChannelRoutingCandidatePolicy
}

// ChannelRoutingPolicy is ordered from the highest priority tier to the lowest.
type ChannelRoutingPolicy struct {
	Tiers []ChannelRoutingTier
}

// ChannelRoutingPolicySnapshot identifies the source and committed generation
// used to construct a runtime routing policy.
type ChannelRoutingPolicySnapshot struct {
	Policy                ChannelRoutingPolicy
	Source                string
	Generation            int64
	DataGeneration        uint64
	PublishedGeneration   uint64
	ClusterCommittedEpoch int64
	LocalPublishedEpoch   int64
	CacheEnabled          bool
	CachePending          bool
}

type channelRoutingRandomInt func(max *big.Int) (*big.Int, error)

var ErrInvalidChannelRoutingSettings = errors.New("invalid channel routing settings")

// parseChannelAdvancedCustomRoutingConfig parses only the routing-relevant
// settings and never mutates the Channel or persists fallback values.
func parseChannelAdvancedCustomRoutingConfig(channel *Channel) (*dto.AdvancedCustomConfig, error) {
	if channel == nil {
		return nil, ErrInvalidChannelRoutingSettings
	}
	if strings.TrimSpace(channel.OtherSettings) == "" {
		return nil, nil
	}
	settings := dto.ChannelOtherSettings{}
	if err := common.UnmarshalJsonStr(channel.OtherSettings, &settings); err != nil {
		return nil, ErrInvalidChannelRoutingSettings
	}
	return settings.AdvancedCustom, nil
}

// BuildChannelRoutingPolicy applies the routing contract without reading state:
// priorities are fallback tiers, all-zero tiers are shared equally, and zero
// weight candidates receive no traffic when any positive weight is present.
func BuildChannelRoutingPolicy(candidates []ChannelRoutingCandidate) ChannelRoutingPolicy {
	if len(candidates) == 0 {
		return ChannelRoutingPolicy{Tiers: []ChannelRoutingTier{}}
	}

	candidatesByPriority := make(map[int64][]ChannelRoutingCandidate)
	priorities := make([]int64, 0)
	for _, candidate := range candidates {
		if _, exists := candidatesByPriority[candidate.Priority]; !exists {
			priorities = append(priorities, candidate.Priority)
		}
		candidatesByPriority[candidate.Priority] = append(candidatesByPriority[candidate.Priority], candidate)
	}
	sort.Slice(priorities, func(i, j int) bool {
		return priorities[i] > priorities[j]
	})

	policy := ChannelRoutingPolicy{Tiers: make([]ChannelRoutingTier, 0, len(priorities))}
	for _, priority := range priorities {
		tierCandidates := candidatesByPriority[priority]
		sort.Slice(tierCandidates, func(i, j int) bool {
			return tierCandidates[i].ChannelID < tierCandidates[j].ChannelID
		})

		hasPositiveWeight := false
		for _, candidate := range tierCandidates {
			if candidate.Weight > 0 {
				hasPositiveWeight = true
				break
			}
		}

		totalWeight := new(big.Int)
		effectiveWeights := make([]uint, len(tierCandidates))
		for i, candidate := range tierCandidates {
			effectiveWeight := candidate.Weight
			if !hasPositiveWeight {
				effectiveWeight = 1
			}
			effectiveWeights[i] = effectiveWeight
			totalWeight.Add(totalWeight, new(big.Int).SetUint64(uint64(effectiveWeight)))
		}

		tier := ChannelRoutingTier{
			Priority:   priority,
			Candidates: make([]ChannelRoutingCandidatePolicy, 0, len(tierCandidates)),
		}
		for i, candidate := range tierCandidates {
			effectiveWeight := effectiveWeights[i]
			expectedShare := 0.0
			if effectiveWeight > 0 && totalWeight.Sign() > 0 {
				expectedShare, _ = new(big.Rat).SetFrac(
					new(big.Int).SetUint64(uint64(effectiveWeight)),
					totalWeight,
				).Float64()
			}
			tier.Candidates = append(tier.Candidates, ChannelRoutingCandidatePolicy{
				ChannelRoutingCandidate: candidate,
				EffectiveWeight:         effectiveWeight,
				ExpectedShare:           expectedShare,
			})
		}
		policy.Tiers = append(policy.Tiers, tier)
	}
	return policy
}

// SelectChannelFromRoutingPolicy selects from the retry's fallback tier using
// the same policy for both cache-backed and database-backed routing.
func SelectChannelFromRoutingPolicy(policy ChannelRoutingPolicy, retry int) (int, bool, error) {
	return selectChannelFromRoutingPolicy(policy, retry, func(max *big.Int) (*big.Int, error) {
		return cryptorand.Int(cryptorand.Reader, max)
	})
}

func selectChannelFromRoutingPolicy(policy ChannelRoutingPolicy, retry int, randomInt channelRoutingRandomInt) (int, bool, error) {
	if len(policy.Tiers) == 0 {
		return 0, false, nil
	}
	if retry < 0 {
		retry = 0
	}
	if retry >= len(policy.Tiers) {
		retry = len(policy.Tiers) - 1
	}

	tier := policy.Tiers[retry]
	positiveCandidates := 0
	soleChannelID := 0
	totalWeight := new(big.Int)
	for _, candidate := range tier.Candidates {
		if candidate.EffectiveWeight == 0 {
			continue
		}
		positiveCandidates++
		soleChannelID = candidate.ChannelID
		totalWeight.Add(totalWeight, new(big.Int).SetUint64(uint64(candidate.EffectiveWeight)))
	}
	if positiveCandidates == 0 || totalWeight.Sign() <= 0 {
		return 0, false, errors.New("routing tier has no effective weight")
	}
	if positiveCandidates == 1 {
		return soleChannelID, true, nil
	}
	if randomInt == nil {
		return 0, false, errors.New("routing random source is nil")
	}

	draw, err := randomInt(totalWeight)
	if err != nil {
		return 0, false, err
	}
	if draw == nil || draw.Sign() < 0 || draw.Cmp(totalWeight) >= 0 {
		return 0, false, errors.New("routing random source returned an out-of-range value")
	}

	remaining := new(big.Int).Set(draw)
	for _, candidate := range tier.Candidates {
		if candidate.EffectiveWeight == 0 {
			continue
		}
		weight := new(big.Int).SetUint64(uint64(candidate.EffectiveWeight))
		if remaining.Cmp(weight) < 0 {
			return candidate.ChannelID, true, nil
		}
		remaining.Sub(remaining, weight)
	}
	return 0, false, errors.New("channel not found")
}
