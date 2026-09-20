package operation_setting

import (
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserFundingSettingDefaultsAndValidation(t *testing.T) {
	setting := config.GlobalConfig.Get("user_funding_setting")
	require.NotNil(t, setting)
	baseline, err := config.ConfigToMap(setting)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(setting, baseline))
	})

	assert.Equal(t, UserFundingModeEnabled, GetUserFundingMode())
	for _, mode := range []UserFundingMode{UserFundingModeEnabled, UserFundingModeRetirement, UserFundingModeDisabled} {
		require.NoError(t, SetUserFundingMode(mode))
		assert.Equal(t, mode, GetUserFundingMode())
	}

	assert.Error(t, SetUserFundingMode("invalid"))
	assert.Equal(t, UserFundingModeDisabled, GetUserFundingMode())
}
