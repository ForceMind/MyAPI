package common

import (
	"testing"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedCodexDefaultSessionRespectsOverridesAndFinalMap(t *testing.T) {
	for _, test := range []struct {
		name string
		info *RelayInfo
		want map[string]interface{}
	}{
		{name: "missing session", info: &RelayInfo{ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeCodex}}, want: map[string]interface{}{"session_id": "synthetic-generated-session"}},
		{name: "explicit client session", info: &RelayInfo{ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeCodex}, RequestHeaders: map[string]string{"Session_id": "synthetic-client"}}, want: map[string]interface{}{"session_id": "{client_header:session_id}"}},
		{name: "explicit thread", info: &RelayInfo{ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeCodex}, RequestHeaders: map[string]string{"Thread_id": "synthetic-thread"}}, want: map[string]interface{}{"thread_id": "{client_header:thread_id}"}},
		{name: "operator session", info: &RelayInfo{ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeCodex, HeadersOverride: map[string]interface{}{"Session_id": "synthetic-operator"}}}, want: map[string]interface{}{"session_id": "synthetic-operator"}},
		{name: "final runtime map includes absence", info: &RelayInfo{ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeCodex}, UseRuntimeHeadersOverride: true, RuntimeHeadersOverride: map[string]interface{}{}}, want: map[string]interface{}{}},
		{name: "other channel", info: &RelayInfo{ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}, want: map[string]interface{}{}},
		{name: "channel test", info: &RelayInfo{ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeCodex}, IsChannelTest: true}, want: map[string]interface{}{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			SeedCodexDefaultSessionHeader(test.info, "synthetic-generated-session")
			assert.Equal(t, test.want, GetEffectiveHeaderOverride(test.info))
		})
	}
}

func TestSeedCodexDefaultSessionParameterDeletionIsFinal(t *testing.T) {
	info := &RelayInfo{ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeCodex, ParamOverride: map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{"mode": "delete_header", "path": "Session_id"},
			map[string]interface{}{"mode": "delete", "path": "prompt_cache_key"},
		},
	}}}
	SeedCodexDefaultSessionHeader(info, "synthetic-generated-session")
	body, err := ApplyParamOverrideWithRelayInfo([]byte(`{"prompt_cache_key":"synthetic-generated-key"}`), info)
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(body))
	assert.Empty(t, GetEffectiveHeaderOverride(info))
	SeedCodexDefaultSessionHeader(info, "must-not-return")
	assert.Empty(t, GetEffectiveHeaderOverride(info))
}
