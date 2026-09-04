package passkey

import (
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildWebAuthnWithSettingsUsesProvidedSnapshot(t *testing.T) {
	previousAddress := system_setting.GetServerAddress()
	t.Cleanup(func() { system_setting.SetServerAddress(previousAddress) })

	settings := &system_setting.PasskeySettings{
		RPDisplayName: "Snapshot RP",
		RPID:          "passkey.example.test:8443",
		Origins:       "https://passkey.example.test:8443",
	}
	req := httptest.NewRequest("GET", "https://request.example.test/login", nil)

	// A later global-address generation must not affect a caller-owned settings snapshot.
	system_setting.SetServerAddress("https://changed-global.example.test:9443")
	webAuthn, err := BuildWebAuthnWithSettings(req, settings)

	require.NoError(t, err)
	require.NotNil(t, webAuthn)
	assert.Equal(t, "Snapshot RP", webAuthn.Config.RPDisplayName)
	assert.Equal(t, "passkey.example.test", webAuthn.Config.RPID)
	assert.Equal(t, []string{"https://passkey.example.test:8443"}, webAuthn.Config.RPOrigins)
}

func TestBuildWebAuthnWithSettingsDerivesOriginFromRequestHost(t *testing.T) {
	settings := &system_setting.PasskeySettings{}
	req := httptest.NewRequest("GET", "https://login.example.test:8443/login", nil)

	webAuthn, err := BuildWebAuthnWithSettings(req, settings)

	require.NoError(t, err)
	require.NotNil(t, webAuthn)
	assert.Equal(t, "login.example.test", webAuthn.Config.RPID)
	assert.Equal(t, []string{"https://login.example.test:8443"}, webAuthn.Config.RPOrigins)
}

func TestBuildWebAuthnWithSettingsRejectsNonLocalhostHTTPWithoutInsecureOrigin(t *testing.T) {
	settings := &system_setting.PasskeySettings{}
	req := httptest.NewRequest("GET", "http://passkey.example.test/login", nil)

	webAuthn, err := BuildWebAuthnWithSettings(req, settings)

	require.Error(t, err)
	assert.Nil(t, webAuthn)
	assert.Contains(t, err.Error(), "仅支持 HTTPS")
}

func TestBuildWebAuthnWithSettingsRemovesPortFromRPID(t *testing.T) {
	settings := &system_setting.PasskeySettings{
		RPID:    "login.example.test:8443",
		Origins: "https://login.example.test:8443",
	}
	req := httptest.NewRequest("GET", "https://login.example.test:8443/login", nil)

	webAuthn, err := BuildWebAuthnWithSettings(req, settings)

	require.NoError(t, err)
	require.NotNil(t, webAuthn)
	assert.Equal(t, "login.example.test", webAuthn.Config.RPID)
}
