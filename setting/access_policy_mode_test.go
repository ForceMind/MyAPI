package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccessPolicyModeValidation(t *testing.T) {
	state := newManagedAccessPolicyModeSetting(defaultAccessPolicyModeSetting)

	require.NoError(t, state.UpdateConfigMap(map[string]string{"mode": "audit"}))
	assert.Equal(t, AccessPolicyModeAudit, state.generation().setting.Mode)

	require.Error(t, state.UpdateConfigMap(map[string]string{"mode": "strict"}))
	// The rejected candidate must not replace the published generation.
	assert.Equal(t, AccessPolicyModeAudit, state.generation().setting.Mode)

	// Group names with control characters or whitespace-only entries in the
	// JSON form are rejected; flat comma lists tolerate benign spacing.
	require.Error(t, state.UpdateConfigMap(map[string]string{"enforce_groups": "bad\tgroup"}))
	require.Error(t, state.UpdateConfigMap(map[string]string{"enforce_groups": `["  "]`}))
	assert.Equal(t, AccessPolicyModeAudit, state.generation().setting.Mode)
}

func TestAccessPolicyEnforceScope(t *testing.T) {
	state := newManagedAccessPolicyModeSetting(defaultAccessPolicyModeSetting)
	accessPolicyModeState = state
	t.Cleanup(func() {
		accessPolicyModeState = newManagedAccessPolicyModeSetting(defaultAccessPolicyModeSetting)
	})

	// Audit mode never enforces, even with a scoped list.
	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"mode":           "audit",
		"enforce_groups": "vip",
	}))
	assert.False(t, AccessPolicyEnforcedForGroup("vip"))

	// Enforce with an empty scope enforces nowhere.
	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"mode":           "enforce",
		"enforce_groups": "",
	}))
	assert.False(t, AccessPolicyEnforcedForGroup("vip"))

	// Enforce applies only inside the scope.
	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"mode":           "enforce",
		"enforce_groups": "vip, enterprise",
	}))
	assert.True(t, AccessPolicyEnforcedForGroup("vip"))
	assert.True(t, AccessPolicyEnforcedForGroup("enterprise"))
	assert.False(t, AccessPolicyEnforcedForGroup("default"))

	// JSON array form is accepted for structured callers.
	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"enforce_groups": `["vip"]`,
	}))
	assert.True(t, AccessPolicyEnforcedForGroup("vip"))
	assert.False(t, AccessPolicyEnforcedForGroup("enterprise"))
}

func TestAccessPolicyModeDefaultIsOff(t *testing.T) {
	assert.Equal(t, AccessPolicyModeOff, GetAccessPolicyMode())
}
