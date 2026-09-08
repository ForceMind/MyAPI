package system_setting

import (
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedDiscordSettingsPublishesDetachedGeneration(t *testing.T) {
	state := newManagedDiscordSettings(DiscordSettings{
		Enabled:      false,
		ClientId:     "before-id",
		ClientSecret: "before-secret",
	})
	manager := config.NewConfigManager()
	manager.Register("discord", state)

	require.NoError(t, manager.LoadFromDB(map[string]string{
		"discord.enabled":       "true",
		"discord.client_id":     "after-id",
		"discord.client_secret": "after-secret",
	}))

	snapshot := state.detachedSnapshot()
	assert.True(t, snapshot.Enabled)
	assert.Equal(t, "after-id", snapshot.ClientId)
	assert.Equal(t, "after-secret", snapshot.ClientSecret)
	snapshot.ClientSecret = "caller-mutation"
	assert.Equal(t, "after-secret", state.snapshot().ClientSecret)

	require.Error(t, state.UpdateConfigMap(map[string]string{"enabled": "invalid"}))
	assert.Equal(t, "after-secret", state.snapshot().ClientSecret)
}

func TestManagedOIDCSettingsExportsAndPublishesCompleteGeneration(t *testing.T) {
	state := newManagedOIDCSettings(OIDCSettings{
		DisplayName:   "Before",
		ClientId:      "before-id",
		ClientSecret:  "before-secret",
		TokenEndpoint: "https://before.example.test/token",
	})
	manager := config.NewConfigManager()
	manager.Register("oidc", state)

	require.NoError(t, manager.LoadFromDB(map[string]string{
		"oidc.enabled":                "true",
		"oidc.display_name":           "After",
		"oidc.authorization_endpoint": "https://after.example.test/authorize",
	}))

	snapshot := state.detachedSnapshot()
	assert.True(t, snapshot.Enabled)
	assert.Equal(t, "After", snapshot.DisplayName)
	assert.Equal(t, "before-id", snapshot.ClientId)
	assert.Equal(t, "before-secret", snapshot.ClientSecret)
	assert.Equal(t, "https://before.example.test/token", snapshot.TokenEndpoint)
	assert.Equal(t, "https://after.example.test/authorize", snapshot.AuthorizationEndpoint)

	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, "true", exported["enabled"])
	assert.Equal(t, "before-secret", exported["client_secret"])
	assert.Equal(t, "https://after.example.test/authorize", exported["authorization_endpoint"])

	snapshot.ClientId = "caller-mutation"
	assert.Equal(t, "before-id", state.snapshot().ClientId)
	require.Error(t, state.UpdateConfigMap(map[string]string{"enabled": "invalid"}))
	assert.Equal(t, "before-id", state.snapshot().ClientId)
}
