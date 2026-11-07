package common

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomEventRenderPreservesRawSSEAndHeaders(t *testing.T) {
	for _, test := range []struct {
		name          string
		data          string
		cachePresent  bool
		cacheControl  []string
		expectedCache []string
		expectedBody  string
	}{
		{
			name: "data zero with default cache control", data: "data:0",
			expectedCache: []string{"no-cache"}, expectedBody: "data:0\n\n",
		},
		{
			name: "carriage return is escaped and line feed retained", data: "data:first\r\ndata:second",
			expectedCache: []string{"no-cache"}, expectedBody: "data:first\\r\ndata:second\n\n",
		},
		{
			name: "event leading raw frame is retained", data: "event: update\ndata:0\n\n",
			expectedCache: []string{"no-cache"}, expectedBody: "event: update\ndata:0\n\n",
		},
		{
			name: "existing cache control values are retained", data: "data:0", cachePresent: true,
			cacheControl: []string{"private", "max-age=60"}, expectedCache: []string{"private", "max-age=60"},
			expectedBody: "data:0\n\n",
		},
		{
			name: "explicit empty cache control value is retained", data: "data:0", cachePresent: true,
			cacheControl: []string{""}, expectedCache: []string{""}, expectedBody: "data:0\n\n",
		},
		{
			name: "present empty cache control list is retained", data: "data:0", cachePresent: true,
			cacheControl: []string{}, expectedCache: []string{}, expectedBody: "data:0\n\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Type", "application/octet-stream")
			if test.cachePresent {
				recorder.Header()["Cache-Control"] = append([]string{}, test.cacheControl...)
			}

			require.NoError(t, (CustomEvent{Data: test.data}).Render(recorder))

			assert.Equal(t, []string{"text/event-stream"}, recorder.Header()["Content-Type"])
			cacheControl, present := recorder.Header()["Cache-Control"]
			assert.True(t, present)
			assert.Equal(t, test.expectedCache, cacheControl)
			assert.Equal(t, test.expectedBody, recorder.Body.String())
		})
	}
}
