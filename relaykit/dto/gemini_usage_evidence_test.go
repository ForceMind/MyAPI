package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiRawUsageRequiresReportedCounts(t *testing.T) {
	for _, tc := range []struct {
		name, raw  string
		incomplete bool
	}{
		{"empty", `{}`, true},
		{"missing prompt", `{"candidatesTokenCount":5,"totalTokenCount":5}`, true},
		{"null output", `{"promptTokenCount":10,"candidatesTokenCount":null,"totalTokenCount":10}`, true},
		{"missing total", `{"promptTokenCount":10,"candidatesTokenCount":5}`, true},
		{"contradictory total", `{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":1}`, true},
		{"invalid cache", `{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15,"cachedContentTokenCount":11}`, true},
		{"null optional count", `{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15,"cachedContentTokenCount":null}`, true},
		{"missing modality count", `{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15,"promptTokensDetails":[{"modality":"AUDIO"}]}`, true},
		{"negative modality cannot cancel", `{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15,"promptTokensDetails":[{"modality":"TEXT","tokenCount":12},{"modality":"TEXT","tokenCount":-2}]}`, true},
		{"tool and thinking", `{"promptTokenCount":10,"toolUsePromptTokenCount":2,"candidatesTokenCount":5,"thoughtsTokenCount":3,"totalTokenCount":20}`, false},
		{"reported", `{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}`, false},
		{"reported zero", `{"promptTokenCount":0,"candidatesTokenCount":0,"totalTokenCount":0}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw GeminiUsageMetadata
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &raw))
			billing := NewGeminiChatBillingUsage(&raw)
			require.NotNil(t, billing)
			assert.Equal(t, tc.incomplete, billing.Incomplete)
			assert.False(t, billing.Estimated)
		})
	}
}

func TestGeminiWireCannotInjectBillingMetadata(t *testing.T) {
	var raw GeminiUsageMetadata
	require.NoError(t, json.Unmarshal([]byte(`{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15,"billing_usage":{"source":"oai_chat","openai_usage":{"prompt_tokens":1,"completion_tokens":1}}}`), &raw))
	assert.Nil(t, raw.BillingUsage)
}
