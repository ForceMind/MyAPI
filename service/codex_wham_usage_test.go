package service

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexWhamResponsesAreBounded(t *testing.T) {
	oversized := strings.Repeat("x", maxCodexWhamResponseBytes+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(oversized))
	}))
	defer server.Close()

	tests := []struct {
		name  string
		fetch func(context.Context, *http.Client, string, string, string) (int, []byte, error)
	}{
		{name: "usage", fetch: FetchCodexWhamUsage},
		{name: "reset credits", fetch: FetchCodexWhamRateLimitResetCredits},
		{name: "consume reset credit", fetch: ConsumeCodexWhamRateLimitResetCredit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statusCode, body, err := test.fetch(
				context.Background(), http.DefaultClient, server.URL, "access-token", "account-id",
			)
			require.Equal(t, http.StatusOK, statusCode)
			require.Error(t, err)
			require.Contains(t, err.Error(), "response exceeds")
			require.Nil(t, body)
		})
	}
}

func TestCodexWhamResponseAtLimitIsAccepted(t *testing.T) {
	bodyAtLimit := strings.Repeat("x", maxCodexWhamResponseBytes)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(bodyAtLimit))
	}))
	defer server.Close()

	statusCode, body, err := FetchCodexWhamUsage(
		context.Background(), http.DefaultClient, server.URL, "access-token", "account-id",
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.Len(t, body, maxCodexWhamResponseBytes)
}

func TestCodexThresholdSourceRequiresNativeVerifiedTransport(t *testing.T) {
	require.True(t, CodexQuotaSourceQualified(&http.Client{Transport: &http.Transport{}}, "https://chatgpt.com/"))
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	for _, client := range []*http.Client{nil, {Jar: jar}, {Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}, {Transport: &http.Transport{TLSClientConfig: &tls.Config{ServerName: "different.invalid"}}}} {
		require.False(t, CodexQuotaSourceQualified(client, "https://chatgpt.com"))
	}
	for _, origin := range []string{"http://chatgpt.com", "https://chatgpt.com.example.invalid", "https://chatgpt.com:443", "https://proxy.invalid", "https://chatgpt.com/path"} {
		require.False(t, CodexQuotaSourceQualified(&http.Client{Transport: &http.Transport{}}, origin))
	}
}

func TestCodexWhamUsageDoesNotFollowRedirectOrMutateSharedClient(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/redirected" {
			t.Error("usage sampler followed redirect")
		}
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	defer server.Close()
	client := &http.Client{}
	status, _, err := FetchCodexWhamUsage(context.Background(), client, server.URL, "synthetic-access", "synthetic-account")
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, status)
	require.Equal(t, 1, calls)
	require.Nil(t, client.CheckRedirect)
}
