package router

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexUpstreamDiagnosticKeepsStrictGateAndNoReplay(t *testing.T) {
	t.Run("ambiguous upstream disconnect", func(t *testing.T) {
		t.Setenv("FULL_CONTENT_LOG_DIR", t.TempDir())
		fixture := newCodexSessionHeadersFixture(t, "gpt-6.1-sol", false, false, "", func(f *codexSessionHeadersFixture) {
			f.contentLog, f.diagnostics = true, true
		})
		fixture.disconnect.Store(true)
		response := fixture.request(t, "/v1/responses", []byte(`{"model":"gpt-6.1-sol","input":"synthetic","stream":true}`), false)
		assert.NotContains(t, response.Body.String(), "response.completed")
		assert.EqualValues(t, 1, fixture.calls.Load())
		assert.Zero(t, fixture.external.Load())
	})
	t.Run("strict acceptance still refuses override", func(t *testing.T) {
		t.Setenv("FULL_CONTENT_LOG_DIR", t.TempDir())
		fixture := newCodexSessionHeadersFixture(t, "gpt-6.1-sol", false, false, `{"Session_id":"operator-session"}`, func(f *codexSessionHeadersFixture) {
			f.contentLog, f.diagnostics = true, true
		})
		t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
		run, err := service.StartUpstreamAcceptanceRun(context.Background(), fixture.channelID, service.UpstreamAcceptanceLimits{DurationSeconds: 60, MaxTotal: 1, MaxGeneration: 1})
		require.NoError(t, err)
		t.Cleanup(func() {
			_, err := service.StopUpstreamAcceptanceRun(context.Background(), run.ID)
			assert.NoError(t, err)
		})
		response := fixture.request(t, "/v1/responses", []byte(`{"model":"gpt-6.1-sol","input":"synthetic","stream":true}`), false)
		assert.Contains(t, response.Body.String(), "upstream_acceptance_denied")
		assert.Zero(t, fixture.calls.Load())
		assert.Zero(t, fixture.external.Load())
	})
}

func TestCodexUpstreamDiagnosticPreservesWireEvidence(t *testing.T) {
	for _, test := range []struct {
		name, details string
	}{
		{name: "missing"},
		{name: "null", details: `,"input_tokens_details":{"cached_tokens":null}`},
		{name: "zero", details: `,"input_tokens_details":{"cached_tokens":0}`},
		{name: "positive", details: `,"input_tokens_details":{"cached_tokens":7}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			logDir := t.TempDir()
			t.Setenv("FULL_CONTENT_LOG_DIR", logDir)
			t.Setenv("FULL_CONTENT_LOG_MAX_FILES", "0")
			usage := `{"input_tokens":10,"output_tokens":5,"total_tokens":15` + test.details + `}`
			fixture := newCodexSessionHeadersFixture(t, "gpt-6.1-sol", false, false, "", func(f *codexSessionHeadersFixture) {
				f.contentLog, f.diagnostics = true, true
				f.observedBodies = make(chan []byte, 1)
				f.responseSSE = `data: {"type":"response.completed","response":{"id":"synthetic-diagnostic-response","object":"response","status":"completed","model":"gpt-6.1-sol","output":[],"usage":` + usage + `}}` + "\n\ndata: [DONE]\n\n"
			})
			payload := `{"model":"gpt-6.1-sol","messages":[{"role":"system","content":"synthetic stable instructions"},{"role":"user","content":"synthetic hello"}],"stream":true,"stream_options":{"include_usage":true}}`
			response := fixture.request(t, "/v1/chat/completions", []byte(payload), false)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.EqualValues(t, 1, fixture.calls.Load(), "diagnostics must not replay a generation")
			require.Zero(t, fixture.external.Load())
			wire := <-fixture.observedBodies
			var actual map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(wire, &actual))
			assert.NotContains(t, actual, "messages")
			assert.Contains(t, actual, "input")
			assert.JSONEq(t, `false`, string(actual["store"]))

			files, err := filepath.Glob(filepath.Join(logDir, "full-content-*.jsonl"))
			require.NoError(t, err)
			require.Len(t, files, 1)
			logged, err := os.ReadFile(files[0])
			require.NoError(t, err)
			for _, secret := range []string{"synthetic-header-access", "synthetic-header-account", "synthetic-private-cookie", "synthetic-private-api-key", "synthetic-private-proxy"} {
				assert.NotContains(t, string(logged), secret)
			}
			var outbound, upstreamUsage, ingress, downstream int
			for _, line := range strings.Split(strings.TrimSpace(string(logged)), "\n") {
				var record struct {
					RequestID string `json:"request_id"`
					Phase     string `json:"phase"`
					Path      string `json:"path"`
					Body      string `json:"body"`
					ChannelID int    `json:"channel_id"`
					Attempt   int    `json:"upstream_attempt"`
					Capture   string `json:"capture_state"`
				}
				require.NoError(t, common.Unmarshal([]byte(line), &record))
				assert.Equal(t, "synthetic-header-request-0", record.RequestID)
				switch record.Phase {
				case "request":
					ingress++
				case "response_end":
					downstream++
				case "upstream_request":
					outbound++
					assert.Equal(t, "/backend-api/codex/responses", record.Path)
					assert.Equal(t, fixture.channelID, record.ChannelID)
					assert.Equal(t, 1, record.Attempt)
					assert.Equal(t, "complete", record.Capture)
					assert.JSONEq(t, string(wire), record.Body, "capture the actual post-conversion body")
				case "upstream_usage":
					upstreamUsage++
					assert.Equal(t, fixture.channelID, record.ChannelID)
					assert.Equal(t, 1, record.Attempt)
					assert.Equal(t, "complete", record.Capture)
					var evidence struct {
						EventType   string          `json:"event_type"`
						UsageSource string          `json:"usage_source"`
						Usage       json.RawMessage `json:"usage"`
					}
					require.NoError(t, common.Unmarshal([]byte(record.Body), &evidence))
					assert.Equal(t, "response.completed", evidence.EventType)
					assert.Equal(t, "response.usage", evidence.UsageSource)
					assert.JSONEq(t, usage, string(evidence.Usage), "never manufacture a cache zero in raw evidence")
				}
			}
			assert.Equal(t, 1, ingress)
			assert.Equal(t, 1, downstream)
			assert.Equal(t, 1, outbound)
			assert.Equal(t, 1, upstreamUsage)
			if test.name == "positive" {
				assert.Contains(t, response.Body.String(), `"cached_tokens":7`)
			}
		})
	}
}
