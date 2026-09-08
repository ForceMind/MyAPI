package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedChannelRoutingPolicyPublishesCompleteValidatedGeneration(t *testing.T) {
	state := newManagedChannelRoutingPolicy(defaultChannelRoutingPolicy)
	valid := `{"enabled":true,"sticky_enabled":false,"session_ttl_seconds":7200,"quota_max_age_seconds":120}`
	require.NoError(t, state.UpdateConfigMap(map[string]string{"policy": valid}))
	assert.Equal(t, ChannelRoutingPolicy{
		Enabled: true, StickyEnabled: false, SessionTTLSeconds: 7200, QuotaMaxAgeSeconds: 120,
	}, state.snapshot())

	before := state.snapshot()
	err := state.UpdateConfigMap(map[string]string{"policy": `{"enabled":false,"sticky_enabled":true,"session_ttl_seconds":1,"quota_max_age_seconds":120}`})
	require.Error(t, err)
	assert.Equal(t, before, state.snapshot())
}

func TestManagedChannelRoutingPolicyRejectsMissingAndInvalidPolicy(t *testing.T) {
	state := newManagedChannelRoutingPolicy(defaultChannelRoutingPolicy)
	require.Error(t, state.ValidateConfigMap(nil))
	require.Error(t, state.ValidateConfigMap(map[string]string{"policy": "{"}))
	require.Error(t, state.ValidateConfigMap(map[string]string{"policy": `{"session_ttl_seconds":3600,"quota_max_age_seconds":86401}`}))
}
