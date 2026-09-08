package oauth

import (
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOIDCProvider_GetName(t *testing.T) {
	settings := system_setting.GetOIDCSettings()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("oidc"), map[string]string{
			"display_name": settings.DisplayName,
		}))
	})

	p := &OIDCProvider{}

	require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("oidc"), map[string]string{"display_name": ""}))
	assert.Equal(t, "OIDC", p.GetName())

	require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("oidc"), map[string]string{"display_name": "  Acme SSO  "}))
	assert.Equal(t, "Acme SSO", p.GetName())
}
