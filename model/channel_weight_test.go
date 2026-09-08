package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectWeightedChannelAtBoundariesAndZeroWeight(t *testing.T) {
	candidates := []WeightedChannelCandidate{
		{ChannelID: 1, Weight: 0},
		{ChannelID: 2, Weight: 2},
		{ChannelID: 3, Weight: 3},
	}
	for _, test := range []struct {
		draw uint64
		want int
	}{
		{draw: 0, want: 2},
		{draw: 1, want: 2},
		{draw: 2, want: 3},
		{draw: 4, want: 3},
	} {
		got, ok := selectWeightedChannelAt(candidates, test.draw)
		require.True(t, ok)
		assert.Equal(t, test.want, got)
	}
	_, ok := selectWeightedChannelAt(candidates, 5)
	assert.False(t, ok)
}

func TestSelectWeightedChannelClampsLegacyOversizedWeight(t *testing.T) {
	candidates := []WeightedChannelCandidate{
		{ChannelID: 1, Weight: MaxChannelRoutingWeight + 99},
		{ChannelID: 2, Weight: 1},
	}
	assert.Equal(t, uint64(MaxChannelRoutingWeight+1), channelWeightTotal(candidates))
	got, ok := selectWeightedChannelAt(candidates, uint64(MaxChannelRoutingWeight))
	require.True(t, ok)
	assert.Equal(t, 2, got)
}

func TestSelectWeightedChannelAllZeroUsesEqualFallback(t *testing.T) {
	candidates := []WeightedChannelCandidate{{ChannelID: 7}, {ChannelID: 8}}
	assert.Equal(t, uint64(2), channelWeightTotal(candidates))
	first, ok := selectWeightedChannelAt(candidates, 0)
	require.True(t, ok)
	second, ok := selectWeightedChannelAt(candidates, 1)
	require.True(t, ok)
	assert.Equal(t, 7, first)
	assert.Equal(t, 8, second)
}
