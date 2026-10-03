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

func TestCodexOAuthTokenExpiryRequiresRepresentablePositiveSeconds(t *testing.T) {
	for _, flow := range []string{"refresh", "authorization-code"} {
		for _, tc := range []struct {
			raw     string
			seconds int64
		}{
			{"missing", 0}, {"0", 0}, {"-1", 0}, {"null", 0}, {"9223372037", 0}, {"9223372036854775807", 0},
			{"1", 1}, {"3600", 3600}, {"9223372036", 9223372036},
		} {
			t.Run(flow+"/"+tc.raw, func(t *testing.T) {
				restore := SetHttpClientForTest(&http.Client{Transport: codexCredentialRefreshRoundTripper(func(r *http.Request) (*http.Response, error) {
					payload := fmt.Sprintf(`{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","expires_in":%s}`, tc.raw)
					if tc.raw == "missing" {
						payload = `{"access_token":"synthetic-access","refresh_token":"synthetic-refresh"}`
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Request: r, Body: io.NopCloser(strings.NewReader(payload))}, nil
				})})
				t.Cleanup(restore)
				before := time.Now()
				var result *CodexOAuthTokenResult
				var err error
				if flow == "refresh" {
					result, err = RefreshCodexOAuthToken(context.Background(), "synthetic-old")
				} else {
					result, err = ExchangeCodexAuthorizationCode(context.Background(), "synthetic-code", "synthetic-verifier")
				}
				if tc.seconds == 0 {
					assert.Error(t, err)
					assert.Nil(t, result)
					return
				}
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.False(t, result.ExpiresAt.Before(before.Add(time.Duration(tc.seconds)*time.Second)))
				assert.False(t, result.ExpiresAt.After(time.Now().Add(time.Duration(tc.seconds)*time.Second)))
			})
		}
	}
}
