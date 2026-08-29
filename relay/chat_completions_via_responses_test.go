package relay

import (
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsResponsesEventStreamContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "plain", contentType: "text/event-stream", want: true},
		{name: "mixed case with charset", contentType: "Text/Event-Stream; charset=utf-8", want: true},
		{name: "json", contentType: "application/json", want: false},
		{name: "empty", contentType: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isResponsesEventStreamContentType(tt.contentType))
		})
	}
}

func TestResponsesUpstreamIsStream(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        bool
	}{
		{name: "header SSE", contentType: "text/event-stream", body: "event: response.created\n", want: true},
		{name: "headerless event SSE", body: "event: response.created\ndata: {}\n\n", want: true},
		{name: "headerless data SSE", body: "data: {}\n\n", want: true},
		{name: "BOM and whitespace SSE", body: "\xef\xbb\xbf \r\nevent: response.created\n", want: true},
		{name: "headerless JSON object", body: `  {"id":"resp_1"}`, want: false},
		{name: "headerless JSON array", body: `[]`, want: false},
		{name: "empty", body: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := &trackingReadCloser{Reader: strings.NewReader(tt.body)}
			resp := &http.Response{
				Header: http.Header{"Content-Type": []string{tt.contentType}},
				Body:   body,
			}

			require.Equal(t, tt.want, responsesUpstreamIsStream(resp))
			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.body, string(got), "stream detection must preserve the complete response body")
			require.NoError(t, resp.Body.Close())
			assert.True(t, body.closed, "replacement body must close the original body")
		})
	}
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestRecalcQuotaFromRatiosIgnoresInvalidMultipliers(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota: 100,
		},
	}
	info.PriceData.AddOtherRatio("duration", 2)

	quota, ok := recalcQuotaFromRatios(info, map[string]float64{
		"duration": 3,
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	})

	require.True(t, ok)
	assert.Equal(t, 150, quota)
	assert.True(t, info.PriceData.HasOtherRatio("duration"))
}

func TestRecalcQuotaFromRatiosRejectsAllInvalidAdjustedRatios(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota: 100,
		},
	}
	info.PriceData.AddOtherRatio("duration", 2)

	quota, ok := recalcQuotaFromRatios(info, map[string]float64{
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	})

	require.False(t, ok)
	assert.Equal(t, 0, quota)
	assert.True(t, info.PriceData.HasOtherRatio("duration"))
}
