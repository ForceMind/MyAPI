package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateCodexOAuthAuthorizationFlowUsesPKCE(t *testing.T) {
	flow, err := CreateCodexOAuthAuthorizationFlow()
	require.NoError(t, err)
	require.NotEmpty(t, flow.State)
	require.NotEmpty(t, flow.Verifier)

	sum := sha256.Sum256([]byte(flow.Verifier))
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(sum[:]), flow.Challenge)

	parsed, err := url.Parse(flow.AuthorizeURL)
	require.NoError(t, err)
	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "auth.openai.com", parsed.Host)
	assert.Equal(t, "/oauth/authorize", parsed.Path)
	assert.Equal(t, flow.State, parsed.Query().Get("state"))
	assert.Equal(t, flow.Challenge, parsed.Query().Get("code_challenge"))
	assert.Equal(t, "S256", parsed.Query().Get("code_challenge_method"))
	assert.Equal(t, codexOAuthRedirectURI, parsed.Query().Get("redirect_uri"))
}

func TestBuildCodexOAuthAuthorizeURLUsesCallerState(t *testing.T) {
	flow, err := CreateCodexOAuthAuthorizationFlow()
	require.NoError(t, err)
	authorizeURL, err := BuildCodexOAuthAuthorizeURL("server-auth-flow-token", flow.Challenge)
	require.NoError(t, err)
	parsed, err := url.Parse(authorizeURL)
	require.NoError(t, err)
	assert.Equal(t, "server-auth-flow-token", parsed.Query().Get("state"))
}

func TestExchangeCodexAuthorizationCode(t *testing.T) {
	var received url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		received = r.Form
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"refresh","expires_in":3600}`))
	}))
	defer server.Close()

	result, err := exchangeCodexAuthorizationCode(
		context.Background(),
		server.Client(),
		server.URL,
		"client",
		"authorization-code",
		"verifier",
		"http://localhost/callback",
	)
	require.NoError(t, err)
	assert.Equal(t, "access", result.AccessToken)
	assert.Equal(t, "refresh", result.RefreshToken)
	assert.WithinDuration(t, time.Now().Add(time.Hour), result.ExpiresAt, 2*time.Second)
	assert.Equal(t, "authorization_code", received.Get("grant_type"))
	assert.Equal(t, "authorization-code", received.Get("code"))
	assert.Equal(t, "verifier", received.Get("code_verifier"))
	assert.Equal(t, "http://localhost/callback", received.Get("redirect_uri"))
}
