package ionet

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeHTTPClient struct {
	requests []*HTTPRequest
	response *HTTPResponse
	err      error
}

func (c *fakeHTTPClient) Do(req *HTTPRequest) (*HTTPResponse, error) {
	c.requests = append(c.requests, req)
	return c.response, c.err
}

func TestClientMakeRequestJSONBodyAndRequestMetadata(t *testing.T) {
	client := &fakeHTTPClient{response: &HTTPResponse{StatusCode: 201, Body: []byte(`{"id":"deployment-1"}`)}}
	api := NewClientWithConfig("test-key", "https://ionet.example/api", client)

	response, err := api.makeRequest("POST", "/deployments", map[string]any{"name": "demo", "count": 2})

	require.NoError(t, err)
	require.Equal(t, client.response, response)
	require.Len(t, client.requests, 1)
	req := client.requests[0]
	assert.Equal(t, "POST", req.Method)
	assert.Equal(t, "https://ionet.example/api/deployments", req.URL)
	assert.Equal(t, "test-key", req.Headers["X-API-KEY"])
	assert.Equal(t, "application/json", req.Headers["Content-Type"])
	assert.JSONEq(t, `{"name":"demo","count":2}`, string(req.Body))
}

func TestClientMakeRequestMarshalErrorDoesNotCallHTTPClient(t *testing.T) {
	client := &fakeHTTPClient{}
	api := NewClientWithConfig("test-key", "https://ionet.example/api", client)

	_, err := api.makeRequest("POST", "/deployments", map[string]float64{"value": math.NaN()})

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to marshal request body")
	assert.Empty(t, client.requests)
}

func TestClientMakeRequestTransportError(t *testing.T) {
	transportErr := errors.New("connection refused")
	client := &fakeHTTPClient{err: transportErr}
	api := NewClientWithConfig("test-key", "https://ionet.example/api", client)

	_, err := api.makeRequest("GET", "/deployments", nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, transportErr)
	assert.ErrorContains(t, err, "request failed")
	require.Len(t, client.requests, 1)
}

func TestClientMakeRequestRejectsEveryNon2xxStatus(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		sensitive string
	}{
		{name: "zero", status: 0},
		{name: "informational", status: 199},
		{
			name:      "redirect with valid business JSON",
			status:    300,
			body:      `{"detail":"valid but sensitive business detail"}`,
			sensitive: "valid but sensitive business detail",
		},
		{
			name:      "server error with valid detail",
			status:    500,
			body:      `{"detail":"detail-secret"}`,
			sensitive: "detail-secret",
		},
		{
			name:      "server error with raw body",
			status:    500,
			body:      `raw-secret-response`,
			sensitive: "raw-secret-response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeHTTPClient{response: &HTTPResponse{StatusCode: tt.status, Body: []byte(tt.body)}}
			api := NewClientWithConfig("test-key", "https://ionet.example/api", client)

			_, err := api.makeRequest("GET", "/deployments", nil)

			require.Error(t, err)
			var apiErr *APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tt.status, apiErr.Code)
			assert.Equal(t, "API request failed with status "+fmt.Sprint(tt.status), apiErr.Message)
			assert.Empty(t, apiErr.Details)
			if tt.sensitive != "" {
				assert.NotContains(t, err.Error(), tt.sensitive)
				assert.NotContains(t, apiErr.Message, tt.sensitive)
				assert.NotContains(t, apiErr.Details, tt.sensitive)
			}
		})
	}
}

func TestClientMakeRequestAcceptsEvery2xxStatus(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNoContent, 299} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			response := &HTTPResponse{StatusCode: status}
			api := NewClientWithConfig("test-key", "https://ionet.example/api", &fakeHTTPClient{response: response})

			actual, err := api.makeRequest("GET", "/deployments", nil)

			require.NoError(t, err)
			assert.Same(t, response, actual)
		})
	}
}

func TestClientMakeRequestRejectsNilDependenciesAndResponse(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var api *Client

		_, err := api.makeRequest("GET", "/deployments", nil)

		assert.ErrorIs(t, err, errNilClient)
	})

	t.Run("nil HTTP client", func(t *testing.T) {
		api := &Client{BaseURL: "https://ionet.example/api"}

		_, err := api.makeRequest("GET", "/deployments", nil)

		assert.ErrorIs(t, err, errNilHTTPClient)
	})

	t.Run("nil response without error", func(t *testing.T) {
		api := NewClientWithConfig("test-key", "https://ionet.example/api", &fakeHTTPClient{})

		_, err := api.makeRequest("GET", "/deployments", nil)

		assert.ErrorIs(t, err, errNilHTTPResponse)
	})
}

func TestDefaultHTTPClientRejectsRedirectsWithoutLeakingSecrets(t *testing.T) {
	const (
		apiKey = "api-key-secret"
		secret = "body-secret"
	)

	for _, status := range []int{
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
	} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var targetVisited atomic.Bool
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				targetVisited.Store(true)
				assert.Empty(t, req.Header.Get("X-API-KEY"))
				w.WriteHeader(http.StatusNoContent)
			}))
			defer target.Close()

			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				assert.Equal(t, apiKey, req.Header.Get("X-API-KEY"))
				http.Redirect(w, req, target.URL, status)
			}))
			defer source.Close()

			client := NewClientWithConfig(apiKey, source.URL, NewDefaultHTTPClient(time.Second))
			response, err := client.makeRequest(http.MethodPost, "", map[string]string{"prompt": secret})

			require.Error(t, err)
			assert.Nil(t, response)
			var apiErr *APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, status, apiErr.Code)
			assert.Empty(t, apiErr.Details)
			assert.False(t, targetVisited.Load())
			assert.NotContains(t, err.Error(), apiKey)
			assert.NotContains(t, err.Error(), secret)
		})
	}
}

func TestDefaultHTTPClientZeroValueAndNilReceiverAreUsable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	request := &HTTPRequest{Method: http.MethodGet, URL: server.URL}
	clients := []*DefaultHTTPClient{{}, nil}
	for i, client := range clients {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			response, err := client.Do(request)

			require.NoError(t, err)
			require.NotNil(t, response)
			assert.Equal(t, http.StatusNoContent, response.StatusCode)
		})
	}
}

func TestDefaultHTTPClientRejectsNilRequest(t *testing.T) {
	var client DefaultHTTPClient

	_, err := client.Do(nil)

	assert.ErrorIs(t, err, errNilHTTPRequest)
}

func TestBuildQueryParamsJSONSlicesAndEscaping(t *testing.T) {
	query := buildQueryParams(map[string]any{
		"ids":  []int{1, 2},
		"tags": []string{"a&b", "x+y"},
	})

	assert.Contains(t, query, "ids=%5B1%2C2%5D")
	assert.Contains(t, query, "tags=%5B%22a%5Cu0026b%22%2C%22x%2By%22%5D")
	values, err := url.ParseQuery(strings.TrimPrefix(query, "?"))
	require.NoError(t, err)
	assert.Equal(t, `[1,2]`, values.Get("ids"))
	assert.Equal(t, `["a\u0026b","x+y"]`, values.Get("tags"))
}

func TestBuildQueryParamsOmitsEmptyAndZeroValues(t *testing.T) {
	query := buildQueryParams(map[string]any{
		"emptyInts":    []int{},
		"emptyStrings": []string{},
		"emptyString":  "",
		"zeroInt":      0,
		"zeroInt64":    int64(0),
		"zeroFloat":    0.0,
		"zeroTime":     time.Time{},
		"false":        false,
	})

	values, err := url.ParseQuery(strings.TrimPrefix(query, "?"))
	require.NoError(t, err)
	assert.Equal(t, "false", values.Get("false"))
	assert.Len(t, values, 1)
}

func TestBuildQueryParamsFormatsTimeAsRFC3339(t *testing.T) {
	timestamp := time.Date(2026, time.September, 4, 12, 34, 56, 0, time.FixedZone("UTC+8", 8*60*60))
	zeroTime := time.Time{}
	var nilTime *time.Time
	query := buildQueryParams(map[string]any{
		"at":           timestamp,
		"pointer":      &timestamp,
		"zeroPointer":  &zeroTime,
		"typedNilTime": nilTime,
	})

	values, err := url.ParseQuery(strings.TrimPrefix(query, "?"))
	require.NoError(t, err)
	assert.Equal(t, timestamp.Format(time.RFC3339), values.Get("at"))
	assert.Equal(t, timestamp.Format(time.RFC3339), values.Get("pointer"))
	assert.Len(t, values, 2)
}
