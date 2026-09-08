package oauth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type serverAddressRoundTripper func(*http.Request) (*http.Response, error)

func (f serverAddressRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withServerAddressTokenTransport(t *testing.T, wantEndpoint, wantRedirectURI string) {
	t.Helper()
	previousTransport := http.DefaultTransport
	http.DefaultTransport = serverAddressRoundTripper(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, wantEndpoint, req.URL.String())
		require.NoError(t, req.ParseForm())
		assert.Equal(t, wantRedirectURI, req.PostForm.Get("redirect_uri"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"test-access-token"}`)),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
}

func TestOIDCExchangeTokenUsesCurrentServerAddress(t *testing.T) {
	settings := system_setting.GetOIDCSettings()
	previousAddress := system_setting.GetServerAddress()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("oidc"), map[string]string{
			"client_id":      settings.ClientId,
			"client_secret":  settings.ClientSecret,
			"token_endpoint": settings.TokenEndpoint,
		}))
		system_setting.SetServerAddress(previousAddress)
	})

	system_setting.SetServerAddress("https://dashboard.example.test")
	require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("oidc"), map[string]string{
		"client_id":      "client-id",
		"client_secret":  "client-secret",
		"token_endpoint": "https://oidc.example.test/token",
	}))
	withServerAddressTokenTransport(t, "https://oidc.example.test/token", "https://dashboard.example.test/oauth/oidc")

	token, err := (&OIDCProvider{}).ExchangeToken(context.Background(), "authorization-code", nil)
	require.NoError(t, err)
	assert.Equal(t, "test-access-token", token.AccessToken)
}

func TestDiscordExchangeTokenUsesCurrentServerAddress(t *testing.T) {
	settings := system_setting.GetDiscordSettings()
	previousAddress := system_setting.GetServerAddress()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("discord"), map[string]string{
			"client_id":     settings.ClientId,
			"client_secret": settings.ClientSecret,
		}))
		system_setting.SetServerAddress(previousAddress)
	})

	system_setting.SetServerAddress("https://dashboard.example.test")
	require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("discord"), map[string]string{
		"client_id":     "client-id",
		"client_secret": "client-secret",
	}))
	withServerAddressTokenTransport(t, "https://discord.com/api/v10/oauth2/token", "https://dashboard.example.test/oauth/discord")

	token, err := (&DiscordProvider{}).ExchangeToken(context.Background(), "authorization-code", nil)
	require.NoError(t, err)
	assert.Equal(t, "test-access-token", token.AccessToken)
}

func TestGenericOAuthExchangeTokenUsesCurrentServerAddress(t *testing.T) {
	previousAddress := system_setting.GetServerAddress()
	t.Cleanup(func() { system_setting.SetServerAddress(previousAddress) })

	system_setting.SetServerAddress("https://dashboard.example.test")
	provider := NewGenericOAuthProvider(&model.CustomOAuthProvider{
		Name:          "Example",
		Slug:          "example",
		ClientId:      "client-id",
		ClientSecret:  "client-secret",
		TokenEndpoint: "https://generic.example.test/token",
		AuthStyle:     AuthStyleInParams,
	})
	withServerAddressTokenTransport(t, provider.config.TokenEndpoint, "https://dashboard.example.test/oauth/example")

	token, err := provider.ExchangeToken(context.Background(), "authorization-code", nil)
	require.NoError(t, err)
	assert.Equal(t, "test-access-token", token.AccessToken)
}

func TestGenericOAuthExchangeTokenPreservesRawServerAddressConcatenation(t *testing.T) {
	previousAddress := system_setting.GetServerAddress()
	t.Cleanup(func() { system_setting.SetServerAddress(previousAddress) })

	provider := NewGenericOAuthProvider(&model.CustomOAuthProvider{
		Name:          "Example",
		Slug:          "example",
		ClientId:      "client-id",
		ClientSecret:  "client-secret",
		TokenEndpoint: "https://generic.example.test/token",
		AuthStyle:     AuthStyleInParams,
	})
	for _, testCase := range []struct {
		name          string
		serverAddress string
		redirectURI   string
	}{
		{
			name:          "trailing slash is retained",
			serverAddress: "https://dashboard.example.test/",
			redirectURI:   "https://dashboard.example.test//oauth/example",
		},
		{
			name:          "empty address produces relative URI",
			serverAddress: "",
			redirectURI:   "/oauth/example",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			system_setting.SetServerAddress(testCase.serverAddress)
			withServerAddressTokenTransport(t, provider.config.TokenEndpoint, testCase.redirectURI)

			token, err := provider.ExchangeToken(context.Background(), "authorization-code", nil)
			require.NoError(t, err)
			assert.Equal(t, "test-access-token", token.AccessToken)
		})
	}
}
