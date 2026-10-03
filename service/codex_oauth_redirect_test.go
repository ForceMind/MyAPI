package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexOAuthTokenExchangeDoesNotFollowRedirects(t *testing.T) {
	for _, flow := range []string{"refresh", "authorization-code"} {
		for _, status := range []int{301, 302, 303, 307, 308} {
			for _, location := range []string{"https://unrelated.invalid/receive", "/other-token-endpoint"} {
				t.Run(fmt.Sprintf("%s/%d/%s", flow, status, location), func(t *testing.T) {
					requests, redirected := 0, 0
					forwardedCredential := false
					base := &http.Client{Transport: codexCredentialRefreshRoundTripper(func(request *http.Request) (*http.Response, error) {
						requests++
						header := make(http.Header)
						if request.URL.String() == codexOAuthTokenURL {
							require.NoError(t, request.ParseForm())
							if flow == "refresh" {
								assert.Equal(t, "synthetic-refresh-only", request.Form.Get("refresh_token"))
							} else {
								assert.Equal(t, "synthetic-code-only", request.Form.Get("code"))
								assert.Equal(t, "synthetic-verifier-only", request.Form.Get("code_verifier"))
							}
							header.Set("Location", location)
							return &http.Response{StatusCode: status, Header: header, Request: request, Body: io.NopCloser(strings.NewReader(`{"error":"redirected"}`))}, nil
						}
						redirected++
						require.NoError(t, request.ParseForm())
						forwardedCredential = request.Form.Get("refresh_token") != "" || request.Form.Get("code") != "" || request.Form.Get("code_verifier") != ""
						return &http.Response{StatusCode: 200, Header: header, Request: request, Body: io.NopCloser(strings.NewReader(`{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","expires_in":3600}`))}, nil
					})}
					restore := SetHttpClientForTest(base)
					t.Cleanup(restore)
					var result *CodexOAuthTokenResult
					var err error
					if flow == "refresh" {
						result, err = RefreshCodexOAuthToken(context.Background(), "synthetic-refresh-only")
					} else {
						result, err = ExchangeCodexAuthorizationCode(context.Background(), "synthetic-code-only", "synthetic-verifier-only")
					}
					assert.Error(t, err, "token POST must not be replayed at a redirect destination")
					assert.Nil(t, result)
					assert.Equal(t, 1, requests)
					assert.False(t, forwardedCredential, "synthetic credential-bearing POST was forwarded")
					assert.Zero(t, redirected, "neither another origin nor another same-origin endpoint may receive token exchange traffic")
					assert.Nil(t, base.CheckRedirect, "OAuth policy must not mutate the shared relay client")
				})
			}
		}
	}
}

func TestCodexOAuthTokenExchangePreservesNormalResponseAndSharedClient(t *testing.T) {
	for _, flow := range []string{"refresh", "authorization-code"} {
		t.Run(flow, func(t *testing.T) {
			requests := 0
			base := &http.Client{Timeout: 3 * time.Second, Transport: codexCredentialRefreshRoundTripper(func(request *http.Request) (*http.Response, error) {
				requests++
				assert.Equal(t, codexOAuthTokenURL, request.URL.String())
				assert.Equal(t, http.MethodPost, request.Method)
				require.NoError(t, request.ParseForm())
				assert.Equal(t, codexOAuthClientID, request.Form.Get("client_id"))
				return &http.Response{StatusCode: 200, Header: make(http.Header), Request: request, Body: io.NopCloser(strings.NewReader(`{"access_token":"synthetic-access","refresh_token":"synthetic-rotated","expires_in":3600}`))}, nil
			})}
			restore := SetHttpClientForTest(base)
			t.Cleanup(restore)
			var result *CodexOAuthTokenResult
			var err error
			if flow == "refresh" {
				result, err = RefreshCodexOAuthToken(context.Background(), "synthetic-refresh")
			} else {
				result, err = ExchangeCodexAuthorizationCode(context.Background(), "synthetic-code", "synthetic-verifier")
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, "synthetic-access", result.AccessToken)
			assert.Equal(t, "synthetic-rotated", result.RefreshToken)
			assert.WithinDuration(t, time.Now().Add(time.Hour), result.ExpiresAt, 2*time.Second)
			assert.Equal(t, 1, requests)
			assert.Nil(t, base.CheckRedirect)
			assert.Equal(t, 3*time.Second, base.Timeout)
		})
	}
}
