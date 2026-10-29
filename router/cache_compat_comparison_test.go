package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/require"
)

// Observation-only: this probe does not assert that a missing identity is
// correct. The comparison report classifies differences for maintainer review.
func TestMyAPICacheIdentityComparisonProbe(t *testing.T) {
	input := os.Getenv("CACHE_COMPARE_INPUT")
	if input == "" {
		input = filepath.Join("..", "tools", "cache-compat-compare", "fixtures.json")
	}
	data, err := os.ReadFile(input)
	require.NoError(t, err)
	var fixtures struct {
		Requests []struct {
			ID       string            `json:"id"`
			APIKeyID int               `json:"api_key_id"`
			Account  string            `json:"account"`
			Headers  map[string]string `json:"headers"`
			Payload  json.RawMessage   `json:"payload"`
		} `json:"requests"`
	}
	require.NoError(t, common.Unmarshal(data, &fixtures))
	require.Len(t, fixtures.Requests, 6)
	rows := make([]map[string]any, 0, len(fixtures.Requests))
	for _, sample := range fixtures.Requests {
		t.Run(sample.ID, func(t *testing.T) {
			fixture := newCodexSessionHeadersFixture(t, "gpt-6.1-sol", false, false, "", func(f *codexSessionHeadersFixture) {
				f.compareAPIKeyID, f.compareAccountID = sample.APIKeyID, sample.Account
				f.responseStatus = http.StatusBadRequest
				f.responseSSE = `{"error":{"type":"invalid_request_error","message":"synthetic comparison stop"}}`
				f.observedBodies = make(chan []byte, 1)
			})
			response := fixture.request(t, "/v1/chat/completions", sample.Payload, len(sample.Headers) > 0)
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.EqualValues(t, 1, fixture.calls.Load(), "one synthetic send, never a replay")
			require.Zero(t, fixture.external.Load(), "no supplier or other network destination")
			body := <-fixture.observedBodies
			headers := <-fixture.observed
			var payload map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(body, &payload))
			require.JSONEq(t, `true`, string(payload["stream"]))
			require.JSONEq(t, `false`, string(payload["store"]))
			var key string
			keyState := "missing"
			if raw, ok := payload["prompt_cache_key"]; ok {
				keyState = "null"
				if string(raw) != "null" {
					require.NoError(t, common.Unmarshal(raw, &key))
					keyState = "empty"
					if key != "" {
						keyState = "string"
					}
				}
			}
			if sample.ID == "explicit_cache_key" {
				require.Equal(t, "synthetic-explicit-cache-key", key)
			}
			if len(sample.Headers) > 0 {
				require.Equal(t, sample.Headers["Session_id"], headers.Get("Session_id"))
			}
			row := map[string]any{"id": sample.ID, "api_key_id": sample.APIKeyID, "account": sample.Account, "cache_key_state": keyState, "session_state": "missing", "send_count": fixture.calls.Load(), "external_dials": fixture.external.Load()}
			if key != "" {
				hash := sha256.Sum256([]byte(key))
				row["cache_key_sha256"] = hex.EncodeToString(hash[:])
			}
			if session := headers.Get("Session_id"); session != "" {
				hash := sha256.Sum256([]byte(session))
				row["session_state"] = "string"
				row["session_sha256"] = hex.EncodeToString(hash[:])
			}
			rows = append(rows, row)
		})
	}
	if directory := os.Getenv("CACHE_COMPARE_REPORT_DIR"); directory != "" {
		hash := sha256.Sum256(data)
		encoded, err := common.Marshal(map[string]any{"implementation": "myapi", "source_sha": os.Getenv("CACHE_COMPARE_MYAPI_SHA"), "input_sha256": hex.EncodeToString(hash[:]), "layer": "real router and Codex outbound boundary; synthetic HTTP400 stops before reply parsing", "rows": rows})
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(directory, 0700))
		file, err := os.OpenFile(filepath.Join(directory, "myapi-request.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		require.NoError(t, err)
		_, err = file.Write(encoded)
		require.NoError(t, err)
		require.NoError(t, file.Close())
	}
}
