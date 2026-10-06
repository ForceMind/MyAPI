package service

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPancakeResponseBoundaryStopsSDKUnboundedRead(t *testing.T) {
	base := codexCredentialRefreshRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: r, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", int(maxWaffoPancakeResponseBytes)+1)))}, nil
	})
	transport := waffoPancakeBoundedTransport{base: base}
	request, err := http.NewRequest(http.MethodPost, "https://synthetic.invalid", nil)
	require.NoError(t, err)
	response, err := transport.RoundTrip(request)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	var limitErr *http.MaxBytesError
	require.ErrorAs(t, err, &limitErr)
	assert.EqualValues(t, maxWaffoPancakeResponseBytes, len(body))
}
