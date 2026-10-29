package openai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/stretchr/testify/require"
)

func TestMyAPIUsageCompatibilityComparisonProbe(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	input := os.Getenv("CACHE_COMPARE_INPUT")
	if input == "" {
		input = filepath.Join("..", "..", "..", "tools", "cache-compat-compare", "fixtures.json")
	}
	data, err := os.ReadFile(input)
	require.NoError(t, err)
	var fixtures struct {
		Usage []struct {
			ID       string            `json:"id"`
			RawState string            `json:"raw_cached_state"`
			Events   []json.RawMessage `json:"events"`
		} `json:"usage"`
	}
	require.NoError(t, common.Unmarshal(data, &fixtures))
	require.Len(t, fixtures.Usage, 8)
	rows := make([]map[string]any, 0, len(fixtures.Usage))
	for _, sample := range fixtures.Usage {
		t.Run(sample.ID, func(t *testing.T) {
			var wire strings.Builder
			wire.WriteString("data: {\"type\":\"response.created\",\"response\":{\"id\":\"synthetic-comparison\",\"model\":\"gpt-6.1-sol\",\"usage\":null}}\n\n")
			for _, event := range sample.Events {
				compact, err := common.Marshal(event)
				require.NoError(t, err)
				wire.WriteString("data: ")
				wire.Write(compact)
				wire.WriteString("\n\n")
			}
			wire.WriteString("data: [DONE]\n\n")
			c, recorder, response, info := newResponsesChatTestContext(t, wire.String(), true)
			info.UpstreamModelName = "gpt-6.1-sol"
			usage, apiErr := OaiResponsesToChatStreamHandler(c, info, response)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.Contains(t, recorder.Body.String(), "data: [DONE]")
			row := map[string]any{"id": sample.ID, "raw_cached_state": sample.RawState, "client_usage_frames": 0, "client_cache_state": "missing"}
			for _, line := range strings.Split(recorder.Body.String(), "\n") {
				if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
					continue
				}
				var frame map[string]json.RawMessage
				require.NoError(t, common.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame))
				raw, ok := frame["usage"]
				if !ok || string(raw) == "null" {
					continue
				}
				row["client_usage_frames"] = row["client_usage_frames"].(int) + 1
				var counts struct {
					Input   int                        `json:"prompt_tokens"`
					Output  int                        `json:"completion_tokens"`
					Details map[string]json.RawMessage `json:"prompt_tokens_details"`
				}
				require.NoError(t, common.Unmarshal(raw, &counts))
				row["client_input"] = counts.Input
				row["client_output"] = counts.Output
				if value, exists := counts.Details["cached_tokens"]; exists {
					if string(value) == "null" {
						row["client_cache_state"] = "null"
					} else {
						var n int
						require.NoError(t, common.Unmarshal(value, &n))
						row["client_cache_value"] = n
						row["client_cache_state"] = "zero"
						if n > 0 {
							row["client_cache_state"] = "positive"
						}
					}
				}
			}
			// Protect existing authoritative canonical contracts. Alternate shapes
			// are measured, not declared correct merely because this probe ran.
			switch sample.ID {
			case "canonical_positive", "later_terminal_null":
				require.Equal(t, 7, usage.PromptTokensDetails.CachedTokens)
			case "canonical_zero", "later_terminal_zero":
				require.Zero(t, usage.PromptTokensDetails.CachedTokens)
			}
			row["cache_evidence_state"] = "absent"
			if usage.BillingUsage != nil && usage.BillingUsage.ResponsesTextEvidence != nil {
				row["cache_evidence_state"] = "unknown"
				if n := usage.BillingUsage.ResponsesTextEvidence.CacheRead; n != nil {
					row["cache_evidence_value"] = *n
					row["cache_evidence_state"] = "zero"
					if *n > 0 {
						row["cache_evidence_state"] = "positive"
					}
				}
			}
			rows = append(rows, row)
		})
	}
	if directory := os.Getenv("CACHE_COMPARE_REPORT_DIR"); directory != "" {
		hash := sha256.Sum256(data)
		encoded, err := common.Marshal(map[string]any{"implementation": "myapi", "source_sha": os.Getenv("CACHE_COMPARE_MYAPI_SHA"), "input_sha256": hex.EncodeToString(hash[:]), "layer": "actual Responses-to-Chat stream handler; no network", "rows": rows})
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(directory, 0700))
		file, err := os.OpenFile(filepath.Join(directory, "myapi-usage.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		require.NoError(t, err)
		_, err = file.Write(encoded)
		require.NoError(t, err)
		require.NoError(t, file.Close())
	}
}
