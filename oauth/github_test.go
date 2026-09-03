package oauth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type gitHubRoundTripper func(*http.Request) (*http.Response, error)

func (f gitHubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGitHubProviderJSONRequestAndResponseContract(t *testing.T) {
	originalTransport := http.DefaultTransport
	originalClientID := common.GitHubClientId
	originalClientSecret := common.GitHubClientSecret
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
		common.GitHubClientId = originalClientID
		common.GitHubClientSecret = originalClientSecret
	})

	common.GitHubClientId = "client-id"
	common.GitHubClientSecret = "client-secret"
	requestCount := 0
	http.DefaultTransport = gitHubRoundTripper(func(req *http.Request) (*http.Response, error) {
		requestCount++
		switch req.URL.String() {
		case "https://github.com/login/oauth/access_token":
			assert.Equal(t, http.MethodPost, req.Method)
			assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
			assert.Equal(t, "application/json", req.Header.Get("Accept"))

			var values map[string]string
			require.NoError(t, common.DecodeJson(req.Body, &values))
			assert.Equal(t, map[string]string{
				"client_id":     "client-id",
				"client_secret": "client-secret",
				"code":          "authorization-code",
			}, values)

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"access_token":"access-token","scope":"read:user","token_type":"bearer","unknown":"accepted"} {"trailing":"accepted"}`,
				)),
				Request: req,
			}, nil
		case "https://api.github.com/user":
			assert.Equal(t, http.MethodGet, req.Method)
			assert.Equal(t, "Bearer access-token", req.Header.Get("Authorization"))

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"id":42,"login":"octocat","name":"Octo Cat","email":"octo@example.com","unknown":"accepted"}`,
				)),
				Request: req,
			}, nil
		default:
			require.FailNow(t, "unexpected OAuth request", req.URL.String())
			return nil, nil
		}
	})

	provider := &GitHubProvider{}
	token, err := provider.ExchangeToken(context.Background(), "authorization-code", nil)
	require.NoError(t, err)
	assert.Equal(t, &OAuthToken{
		AccessToken: "access-token",
		TokenType:   "bearer",
		Scope:       "read:user",
	}, token)

	user, err := provider.GetUserInfo(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, &OAuthUser{
		ProviderUserID: "42",
		Username:       "octocat",
		DisplayName:    "Octo Cat",
		Email:          "octo@example.com",
		Extra: map[string]any{
			"legacy_id": "octocat",
		},
	}, user)
	assert.Equal(t, 2, requestCount)
}
