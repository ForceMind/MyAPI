package ionet

import (
	"errors"
	"math"
	"net/url"
	"strings"
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

func TestClientMakeRequestAPIErrorFallbacks(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		expectedMessage string
		expectedDetails string
	}{
		{
			name:            "detail message",
			body:            `{"detail":"upstream rejected request"}`,
			expectedMessage: "upstream rejected request",
		},
		{
			name:            "empty response body",
			expectedMessage: "API request failed with status 422",
		},
		{
			name:            "empty detail",
			body:            `{"detail":""}`,
			expectedMessage: "API request failed with status 422",
			expectedDetails: `{"detail":""}`,
		},
		{
			name:            "malformed response body",
			body:            `{"detail":`,
			expectedMessage: "API request failed with status 422",
			expectedDetails: `{"detail":`,
		},
		{
			name:            "non string detail",
			body:            `{"detail":123}`,
			expectedMessage: "API request failed with status 422",
			expectedDetails: `{"detail":123}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeHTTPClient{response: &HTTPResponse{StatusCode: 422, Body: []byte(tt.body)}}
			api := NewClientWithConfig("test-key", "https://ionet.example/api", client)

			_, err := api.makeRequest("GET", "/deployments", nil)

			require.Error(t, err)
			apiErr, ok := err.(*APIError)
			require.True(t, ok)
			assert.Equal(t, 422, apiErr.Code)
			assert.Equal(t, tt.expectedMessage, apiErr.Message)
			assert.Equal(t, tt.expectedDetails, apiErr.Details)
		})
	}
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
