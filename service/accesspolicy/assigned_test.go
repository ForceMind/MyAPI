package accesspolicy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAssignedPolicyPreservesPresenceAndCopiesSets(t *testing.T) {
	input := AssignedPolicy{
		Enabled:        true,
		PublicModels:   []string{"z-model", "a-model", "a-model"},
		UpstreamModels: []string{},
		ChannelIDs:     []int{9, 3, 9},
	}
	got, err := NormalizeAssignedPolicy(input)
	require.NoError(t, err)
	assert.Equal(t, AssignedPolicy{Enabled: true, PublicModels: []string{"a-model", "z-model"}, UpstreamModels: []string{}, ChannelIDs: []int{3, 9}}, got)
	input.PublicModels[0], input.ChannelIDs[0] = "changed", 100
	assert.Equal(t, []string{"a-model", "z-model"}, got.PublicModels)
	assert.Equal(t, []int{3, 9}, got.ChannelIDs)
	again, err := NormalizeAssignedPolicy(got)
	require.NoError(t, err)
	assert.Equal(t, got, again)
	got, err = NormalizeAssignedPolicy(AssignedPolicy{Enabled: true})
	require.NoError(t, err)
	assert.Nil(t, got.PublicModels)
	assert.Nil(t, got.UpstreamModels)
	assert.Nil(t, got.ChannelIDs)
	got, err = NormalizeAssignedPolicy(AssignedPolicy{PublicModels: []string{}, ChannelIDs: []int{}})
	require.NoError(t, err)
	assert.Equal(t, []string{}, got.PublicModels)
	assert.Equal(t, []int{}, got.ChannelIDs)
	assert.False(t, got.Enabled)
}

func TestNormalizeAssignedPolicyRejectsInvalidValuesBeforeDeduplication(t *testing.T) {
	tooMany := make([]string, MaxListValues+1)
	for i := range tooMany {
		tooMany[i] = "same-model"
	}
	tooManyIDs := make([]int, MaxListValues+1)
	for i := range tooManyIDs {
		tooManyIDs[i] = 1
	}
	for _, test := range []struct {
		name   string
		policy AssignedPolicy
	}{
		{"empty public model", AssignedPolicy{PublicModels: []string{""}}},
		{"oversized public model", AssignedPolicy{PublicModels: []string{strings.Repeat("a", MaxModelLength+1)}}},
		{"upstream control character", AssignedPolicy{UpstreamModels: []string{"model\n"}}},
		{"upstream format character", AssignedPolicy{UpstreamModels: []string{"model\u200b"}}},
		{"invalid utf8", AssignedPolicy{PublicModels: []string{string([]byte{0xff})}}},
		{"too many public models", AssignedPolicy{PublicModels: tooMany}},
		{"too many upstream models", AssignedPolicy{UpstreamModels: tooMany}},
		{"too many channels", AssignedPolicy{ChannelIDs: tooManyIDs}},
		{"zero channel", AssignedPolicy{ChannelIDs: []int{1, 0}}},
		{"negative channel", AssignedPolicy{ChannelIDs: []int{-1}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NormalizeAssignedPolicy(test.policy)
			require.ErrorIs(t, err, ErrInvalidList)
		})
	}
	_, err := NormalizeAssignedPolicy(AssignedPolicy{PublicModels: tooMany[:MaxListValues], UpstreamModels: []string{strings.Repeat("a", MaxModelLength)}, ChannelIDs: tooManyIDs[:MaxListValues]})
	require.NoError(t, err)
}

func TestEvaluateAssignedPolicyUsesExactPublicUpstreamAndChannelConstraints(t *testing.T) {
	for _, test := range []struct {
		name     string
		policy   AssignedPolicy
		public   string
		upstream string
		channel  int
		reasons  []string
	}{
		{"inherit", AssignedPolicy{Enabled: true}, "public", "upstream", 7, []string{}},
		{"disabled denies", AssignedPolicy{}, "public", "upstream", 7, []string{"policy_disabled"}},
		{"disabled precedes dimensions", AssignedPolicy{PublicModels: []string{}, UpstreamModels: []string{}, ChannelIDs: []int{}}, "public", "upstream", 7, []string{"policy_disabled"}},
		{"explicit empties deny", AssignedPolicy{Enabled: true, PublicModels: []string{}, UpstreamModels: []string{}, ChannelIDs: []int{}}, "public", "upstream", 7, []string{"public_model_denied", "upstream_model_denied", "channel_denied"}},
		{"all dimensions match", AssignedPolicy{Enabled: true, PublicModels: []string{"public"}, UpstreamModels: []string{"upstream"}, ChannelIDs: []int{7}}, "public", "upstream", 7, []string{}},
		{"public is not upstream", AssignedPolicy{Enabled: true, PublicModels: []string{"upstream"}}, "public", "upstream", 7, []string{"public_model_denied"}},
		{"upstream is not public", AssignedPolicy{Enabled: true, UpstreamModels: []string{"public"}}, "public", "upstream", 7, []string{"upstream_model_denied"}},
		{"models are case sensitive", AssignedPolicy{Enabled: true, PublicModels: []string{"Public"}}, "public", "upstream", 7, []string{"public_model_denied"}},
		{"no wildcard expansion", AssignedPolicy{Enabled: true, PublicModels: []string{"*"}}, "public", "upstream", 7, []string{"public_model_denied"}},
		{"candidate channel denied", AssignedPolicy{Enabled: true, ChannelIDs: []int{8}}, "public", "upstream", 7, []string{"channel_denied"}},
		{"missing upstream denied", AssignedPolicy{Enabled: true, UpstreamModels: []string{"upstream"}}, "public", "", 7, []string{"upstream_model_denied"}},
		{"missing channel denied", AssignedPolicy{Enabled: true, ChannelIDs: []int{7}}, "public", "upstream", 0, []string{"channel_denied"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy, err := NormalizeAssignedPolicy(test.policy)
			require.NoError(t, err)
			assert.Equal(t, test.reasons, EvaluateAssignedPolicy(policy, test.public, test.upstream, test.channel))
		})
	}
}
