package service

import (
	"context"
	"net/http"
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
