package system_setting

import (
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOIDCSettings_GetEffectiveDisplayName(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		want        string
	}{
		{name: "blank falls back to OIDC", displayName: "", want: "OIDC"},
		{name: "custom name is returned verbatim", displayName: "Acme SSO", want: "Acme SSO"},
		{name: "whitespace-only falls back to OIDC", displayName: "   ", want: "OIDC"},
		{name: "surrounding whitespace is trimmed", displayName: "  Acme SSO  ", want: "Acme SSO"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &OIDCSettings{DisplayName: tt.displayName}
			assert.Equal(t, tt.want, s.GetEffectiveDisplayName())
		})
	}
}

func TestOIDCSettings_DisplayNamePersistenceRoundTrip(t *testing.T) {
	settings := newManagedOIDCSettings(OIDCSettings{DisplayName: "  Acme SSO  "})
	manager := config.NewConfigManager()
	manager.Register("oidc", settings)

	saved := make(map[string]string)
	require.NoError(t, manager.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	require.Equal(t, "  Acme SSO  ", saved["oidc.display_name"])

	require.NoError(t, settings.UpdateConfigMap(map[string]string{"display_name": ""}))
	require.NoError(t, manager.LoadFromDB(saved))
	snapshot := settings.snapshot()
	assert.Equal(t, "  Acme SSO  ", snapshot.DisplayName)
	assert.Equal(t, "Acme SSO", snapshot.GetEffectiveDisplayName())
}
