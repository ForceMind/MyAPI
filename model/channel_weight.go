package model

import "math/rand"

// MaxChannelRoutingWeight is the largest effective weight used by channel
// selection. Existing rows above the bound are clamped while new writes are
// rejected by the routing management API.
const MaxChannelRoutingWeight uint = 1_000_000

type WeightedChannelCandidate struct {
	ChannelID int
	Weight    uint
}

// SelectWeightedChannel applies the same bounded probability semantics to the
// database and memory-cache selection paths. A zero weight has no share when
// any candidate has a positive weight. If every weight is zero, candidates are
// selected uniformly for backward compatibility.
func SelectWeightedChannel(candidates []WeightedChannelCandidate) (int, bool) {
	if len(candidates) == 0 {
		return 0, false
	}
	total := channelWeightTotal(candidates)
	return selectWeightedChannelAt(candidates, uint64(rand.Int63n(int64(total))))
}

func channelWeightTotal(candidates []WeightedChannelCandidate) uint64 {
	var total uint64
	for _, candidate := range candidates {
		weight := candidate.Weight
		if weight > MaxChannelRoutingWeight {
			weight = MaxChannelRoutingWeight
		}
		total += uint64(weight)
	}
	if total == 0 {
		return uint64(len(candidates))
	}
	return total
}

// selectWeightedChannelAt is split out to make the exact [0,total) boundary
// deterministic in tests.
func selectWeightedChannelAt(candidates []WeightedChannelCandidate, draw uint64) (int, bool) {
	total := channelWeightTotal(candidates)
	if total == 0 || draw >= total {
		return 0, false
	}
	allZero := total == uint64(len(candidates))
	if allZero {
		for _, candidate := range candidates {
			if candidate.Weight != 0 {
				allZero = false
				break
			}
		}
	}
	for _, candidate := range candidates {
		weight := candidate.Weight
		if weight > MaxChannelRoutingWeight {
			weight = MaxChannelRoutingWeight
		}
		if allZero {
			weight = 1
		}
		if uint64(weight) > draw {
			return candidate.ChannelID, true
		}
		draw -= uint64(weight)
	}
	return 0, false
}
