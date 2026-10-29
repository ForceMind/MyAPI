package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/ForceMind/MyAPI/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newUpstreamDiagnosticTestObserver(t *testing.T) (*upstreamDiagnosticObserver, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(fullContentLogDirEnv, dir)
	t.Setenv(fullContentLogMaxMBEnv, "1")
	t.Setenv(fullContentLogMaxFilesEnv, "2")
	o := &upstreamDiagnosticObserver{writer: getFullContentLogWriter(), identity: fullContentRequestIdentity{requestID: "synthetic-diagnostic-request"}}
	return o, dir
}

func readUpstreamDiagnosticTestRecords(t *testing.T, dir string) []fullContentLogEntry {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "full-content-*.jsonl"))
	require.NoError(t, err)
	var records []fullContentLogEntry
	for _, file := range files {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			var entry fullContentLogEntry
			require.NoError(t, common.Unmarshal([]byte(line), &entry))
			records = append(records, entry)
		}
	}
	return records
}

func TestUpstreamDiagnosticRequiresBothSwitchesAndOmitsPlayground(t *testing.T) {
	for _, test := range []struct {
		name, content, diagnostic string
		playground, wantObserver  bool
	}{
		{name: "default off", content: "true"},
		{name: "explicit off", content: "true", diagnostic: "false"},
		{name: "invalid stays off", content: "true", diagnostic: "invalid"},
		{name: "content logging off", content: "false", diagnostic: "true"},
		{name: "playground remains omitted", content: "true", diagnostic: "true", playground: true},
		{name: "explicitly enabled", content: "true", diagnostic: "true", wantObserver: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(fullContentLogEnabledEnv, test.content)
			t.Setenv(upstreamDiagnosticEnv, test.diagnostic)
			t.Setenv(fullContentLogDirEnv, t.TempDir())
			engine := gin.New()
			if test.playground {
				engine.Use(func(c *gin.Context) { c.Set("playground_original_path", "/v1/chat/completions"); c.Next() })
			}
			engine.Use(FullContentLogger())
			engine.GET("/diagnostic", func(c *gin.Context) {
				_, ok := c.Value(common.UpstreamDiagnosticContextKey).(common.UpstreamDiagnosticObserver)
				assert.Equal(t, test.wantObserver, ok)
				c.Status(http.StatusNoContent)
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/diagnostic", nil))
			assert.Equal(t, http.StatusNoContent, response.Code)
		})
	}
}

func TestUpstreamDiagnosticPreservesRawUsageStatesAndStreamBytes(t *testing.T) {
	for _, protocol := range []struct {
		name, contentType, requestBody string
		sse                            bool
	}{
		{name: "explicit SSE", contentType: "text/event-stream; charset=utf-8", requestBody: `{"stream":true}`, sse: true},
		{name: "empty Content-Type SSE", requestBody: `{"stream":true}`, sse: true},
		{name: "whitespace Content-Type SSE", contentType: " \t ", requestBody: `{"stream":true}`, sse: true},
		{name: "explicit JSON takes precedence", contentType: "application/json", requestBody: `{"stream":true}`},
		{name: "empty Content-Type JSON", requestBody: `{"stream":false}`},
	} {
		t.Run(protocol.name, func(t *testing.T) {
			for _, test := range []struct {
				name, fragment, state string
			}{
				{name: "usage missing", state: "missing"},
				{name: "usage null", fragment: `,"usage":null`, state: "null"},
				{name: "cache missing", fragment: `,"usage":{"input_tokens":10}`, state: "present"},
				{name: "cache null", fragment: `,"usage":{"input_tokens_details":{"cached_tokens":null}}`, state: "present"},
				{name: "cache zero", fragment: `,"usage":{"input_tokens_details":{"cached_tokens":0}}`, state: "present"},
				{name: "cache positive", fragment: `,"usage":{"input_tokens_details":{"cached_tokens":7}}`, state: "present"},
				{name: "exact large number and decimal", fragment: `,"usage":{"input_tokens":9007199254740993,"ratio":1.00}`, state: "present"},
			} {
				t.Run(test.name, func(t *testing.T) {
					o, dir := newUpstreamDiagnosticTestObserver(t)
					req, err := http.NewRequest(http.MethodPost, "http://synthetic.invalid/backend-api/codex/responses", strings.NewReader(protocol.requestBody))
					require.NoError(t, err)
					req.GetBody = nil
					observe := o.ObserveRequest(req, 17)
					_, err = io.ReadAll(req.Body)
					require.NoError(t, err)
					require.NoError(t, req.Body.Close())
					assert.Nil(t, req.GetBody, "diagnostics must never restore replay")
					sse := "data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"text\":\"must-not-log-output\"}]" + test.fragment + "}}\r\n\r\ndata: [DONE]\r\n\r\n"
					wire := sse
					if !protocol.sse {
						wire = `{"output":[{"text":"must-not-log-output"}]` + test.fragment + `}`
					}
					response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {protocol.contentType}}, Body: io.NopCloser(iotest.OneByteReader(strings.NewReader(wire)))}
					observe(response)
					returned, err := io.ReadAll(response.Body)
					require.NoError(t, err)
					require.NoError(t, response.Body.Close())
					assert.Equal(t, wire, string(returned), "observation must leave the handler's exact bytes intact")
					assert.Equal(t, protocol.contentType, response.Header.Get("Content-Type"), "fallback must not rewrite upstream headers")
					records := readUpstreamDiagnosticTestRecords(t, dir)
					require.Len(t, records, 2)
					assert.Equal(t, "upstream_request", records[0].Phase)
					assert.Equal(t, "upstream_usage", records[1].Phase)
					assert.NotContains(t, records[1].Body, "must-not-log-output")
					var evidence upstreamDiagnosticUsage
					require.NoError(t, common.Unmarshal([]byte(records[1].Body), &evidence))
					assert.Equal(t, test.state, evidence.UsageState)
					if protocol.sse {
						assert.Equal(t, "response.usage", evidence.UsageSource)
						assert.Equal(t, "response.completed", evidence.EventType)
					} else {
						assert.Equal(t, "usage", evidence.UsageSource)
						assert.Equal(t, "other", evidence.EventType)
					}
					if test.fragment == "" {
						assert.Empty(t, evidence.Usage)
					} else {
						assert.Equal(t, strings.TrimPrefix(test.fragment, `,"usage":`), string(evidence.Usage), "raw evidence must preserve both presence and number lexemes")
					}
				})
			}
		})
	}
}

func TestUpstreamDiagnosticEmptyContentTypeRequiresKnownProtocol(t *testing.T) {
	for _, test := range []struct{ name, requestBody, contentType string }{
		{name: "stream absent", requestBody: `{}`},
		{name: "stream null", requestBody: `{"stream":null}`},
		{name: "invalid stream type", requestBody: `{"stream":"true"}`},
		{name: "nonempty unsupported header unchanged", requestBody: `{"stream":true}`, contentType: "application/octet-stream"},
	} {
		t.Run(test.name, func(t *testing.T) {
			o, dir := newUpstreamDiagnosticTestObserver(t)
			req, err := http.NewRequest(http.MethodPost, "http://synthetic.invalid/responses", strings.NewReader(test.requestBody))
			require.NoError(t, err)
			observe := o.ObserveRequest(req, 1)
			_, err = io.ReadAll(req.Body)
			require.NoError(t, err)
			wire := "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens_details\":{\"cached_tokens\":7}}}}\n\n"
			response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {test.contentType}}, Body: io.NopCloser(strings.NewReader(wire))}
			observe(response)
			got, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			assert.Equal(t, wire, string(got))
			records := readUpstreamDiagnosticTestRecords(t, dir)
			require.Len(t, records, 2)
			assert.Equal(t, "unsupported_content_type", records[1].Capture)
			assert.Empty(t, records[1].Body, "unknown protocol must remain unobserved, not fabricate a cache zero")
		})
	}
}

func TestUpstreamDiagnosticRedactsSecretsWithoutChangingRequest(t *testing.T) {
	o, dir := newUpstreamDiagnosticTestObserver(t)
	body := `{"input":[{"role":"user","content":"synthetic"}],"api_key":"secret-a","nested":[{"refresh_token":"secret-b","idempotency_key":"secret-c"}],"counter":9007199254740993}`
	req, err := http.NewRequest(http.MethodPost, "http://secret-user:secret-pass@synthetic.invalid/backend-api/codex/responses?api_key=secret-query", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer secret-auth")
	req.Header.Set("Cookie", "secret-cookie")
	observe := o.ObserveRequest(req, 11)
	read, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.NoError(t, req.Body.Close())
	assert.Equal(t, body, string(read))
	assert.Equal(t, "Bearer secret-auth", req.Header.Get("Authorization"))
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":9007199254740993,"access_token":"secret-d"},"output":"secret-response"}`))}
	observe(response)
	_, err = io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	records := readUpstreamDiagnosticTestRecords(t, dir)
	require.Len(t, records, 2)
	for _, record := range records {
		encoded, err := common.Marshal(record)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "secret-")
		assert.Contains(t, record.Body, "9007199254740993")
		assert.Equal(t, "synthetic-diagnostic-request", record.RequestID)
		assert.Equal(t, 11, record.ChannelID)
		assert.EqualValues(t, 1, record.Attempt)
		assert.Equal(t, "/backend-api/codex/responses", record.Path)
		assert.Empty(t, record.Headers)
		assert.Empty(t, record.Query)
	}
}

func TestUpstreamDiagnosticRecordsUsageEventsWithoutOverwritingEarlierEvidence(t *testing.T) {
	o, dir := newUpstreamDiagnosticTestObserver(t)
	req, err := http.NewRequest(http.MethodPost, "http://synthetic.invalid/responses", strings.NewReader(`{}`))
	require.NoError(t, err)
	observe := o.ObserveRequest(req, 1)
	_, err = io.ReadAll(req.Body)
	require.NoError(t, err)
	sse := "data: {\"type\":\"response.in_progress\",\"usage\":{\"cached_tokens\":7}}\n\n" +
		"data: {\"type\":\"response.completed\",\"usage\":{\"cached_tokens\":3},\"response\":{\"usage\":{\"input_tokens_details\":{\"cached_tokens\":0}}}}\n\n" +
		"data: {\"type\":\"response.done\",\"response\":{}}\n\ndata: [DONE]\n\n"
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(sse))}
	observe(response)
	returned, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, sse, string(returned))
	records := readUpstreamDiagnosticTestRecords(t, dir)
	require.Len(t, records, 5)
	var first, second, third, fourth upstreamDiagnosticUsage
	require.NoError(t, common.Unmarshal([]byte(records[1].Body), &first))
	require.NoError(t, common.Unmarshal([]byte(records[2].Body), &second))
	require.NoError(t, common.Unmarshal([]byte(records[3].Body), &third))
	require.NoError(t, common.Unmarshal([]byte(records[4].Body), &fourth))
	assert.JSONEq(t, `{"cached_tokens":7}`, string(first.Usage))
	assert.Equal(t, "usage", first.UsageSource)
	assert.JSONEq(t, `{"cached_tokens":3}`, string(second.Usage))
	assert.Equal(t, "usage", second.UsageSource)
	assert.JSONEq(t, `{"input_tokens_details":{"cached_tokens":0}}`, string(third.Usage))
	assert.Equal(t, "response.usage", third.UsageSource)
	assert.Equal(t, "missing", fourth.UsageState)
	assert.Empty(t, fourth.Usage)
	for i, record := range records[1:] {
		assert.EqualValues(t, i+1, record.Sequence)
	}
}

func TestUpstreamDiagnosticOmitsIncompleteAndOversizedBodies(t *testing.T) {
	for _, test := range []struct {
		name, body, want string
		partial          bool
	}{
		{name: "invalid", body: `{"api_key":"never-log-this"`, want: "invalid"},
		{name: "oversized", body: `{"input":"` + strings.Repeat("x", upstreamDiagnosticMaxBytes) + `","api_key":"never-log-this"}`, want: "oversized"},
		{name: "partial", body: `{"api_key":"never-log-this"}`, want: "partial", partial: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			o, dir := newUpstreamDiagnosticTestObserver(t)
			req, err := http.NewRequest(http.MethodPost, "http://synthetic.invalid/responses", strings.NewReader(test.body))
			require.NoError(t, err)
			observe := o.ObserveRequest(req, 1)
			if !test.partial {
				_, err = io.ReadAll(req.Body)
				require.NoError(t, err)
			}
			require.NoError(t, req.Body.Close())
			observe(nil)
			records := readUpstreamDiagnosticTestRecords(t, dir)
			require.Len(t, records, 1)
			assert.Equal(t, test.want, records[0].Capture)
			assert.Empty(t, records[0].Body)
		})
	}
}

func TestUpstreamDiagnosticDoesNotInventUsageOnEarlyCloseOrReadFailure(t *testing.T) {
	for _, test := range []struct {
		name     string
		failRead bool
	}{
		{name: "early close"}, {name: "read failure", failRead: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			o, dir := newUpstreamDiagnosticTestObserver(t)
			req, err := http.NewRequest(http.MethodPost, "http://synthetic.invalid/responses", strings.NewReader(`{}`))
			require.NoError(t, err)
			observe := o.ObserveRequest(req, 1)
			_, err = io.ReadAll(req.Body)
			require.NoError(t, err)
			response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(iotest.ErrReader(errors.New("synthetic read failure")))}
			observe(response)
			if test.failRead {
				_, err = io.ReadAll(response.Body)
				require.EqualError(t, err, "synthetic read failure")
			}
			require.NoError(t, response.Body.Close())
			records := readUpstreamDiagnosticTestRecords(t, dir)
			require.Len(t, records, 2)
			assert.Equal(t, "partial", records[1].Capture)
			assert.Empty(t, records[1].Body)
		})
	}
}
