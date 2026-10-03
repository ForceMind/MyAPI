package dto

import (
	"testing"

	kitutil "github.com/ForceMind/MyAPI/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealtimeRawUsagePreservesMissingVersusReportedZero(t *testing.T) {
	for _, tc := range []struct {
		name, raw  string
		incomplete bool
	}{
		{"empty", `{}`, true},
		{"null input", `{"total_tokens":0,"input_tokens":null,"output_tokens":0}`, true},
		{"missing output", `{"total_tokens":0,"input_tokens":0}`, true},
		{"explicit zero", `{"total_tokens":0,"input_tokens":0,"output_tokens":0}`, false},
		{"missing modalities", `{"total_tokens":15,"input_tokens":10,"output_tokens":5}`, true},
		{"text", `{"total_tokens":15,"input_tokens":10,"output_tokens":5,"input_token_details":{"text_tokens":10},"output_token_details":{"text_tokens":5}}`, false},
		{"contradictory total", `{"total_tokens":16,"input_tokens":10,"output_tokens":5,"input_token_details":{"text_tokens":10},"output_token_details":{"text_tokens":5}}`, true},
		{"negative modality", `{"total_tokens":5,"input_tokens":5,"output_tokens":0,"input_token_details":{"text_tokens":6,"audio_tokens":-1}}`, true},
		{"null modality", `{"total_tokens":0,"input_tokens":0,"output_tokens":0,"input_token_details":{"text_tokens":null}}`, true},
		{"null details", `{"total_tokens":0,"input_tokens":0,"output_tokens":0,"output_token_details":null}`, true},
		{"category mismatch", `{"total_tokens":5,"input_tokens":5,"output_tokens":0,"input_token_details":{"text_tokens":4}}`, true},
		{"cached over input", `{"total_tokens":5,"input_tokens":5,"output_tokens":0,"input_token_details":{"text_tokens":5,"cached_tokens":6}}`, true},
		{"cache subset", `{"total_tokens":5,"input_tokens":5,"output_tokens":0,"input_token_details":{"text_tokens":3,"audio_tokens":2,"cached_tokens":4,"cached_tokens_details":{"text_tokens":2,"audio_tokens":2,"image_tokens":0}}}`, false},
		{"cache modality overflow", `{"total_tokens":5,"input_tokens":5,"output_tokens":0,"input_token_details":{"text_tokens":3,"audio_tokens":2,"cached_tokens":4,"cached_tokens_details":{"text_tokens":4}}}`, true},
		{"untrusted evidence", `{"RawUsageObserved":false,"UsageIncomplete":false,"CachedTokensReported":true}`, true},
		{"oversized", `{"total_tokens":2147483648,"input_tokens":2147483648,"output_tokens":0,"input_token_details":{"text_tokens":2147483648}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var usage RealtimeUsage
			require.NoError(t, kitutil.Unmarshal([]byte(tc.raw), &usage))
			assert.True(t, usage.RawUsageObserved)
			assert.Equal(t, tc.incomplete, usage.UsageIncomplete)
			encoded, err := kitutil.Marshal(usage)
			require.NoError(t, err)
			for _, field := range []string{"RawUsageObserved", "UsageIncomplete", "CachedTokensReported"} {
				assert.NotContains(t, string(encoded), field)
			}
		})
	}
	var usage RealtimeUsage
	require.NoError(t, kitutil.Unmarshal([]byte(`{"input_tokens":1,"output_tokens":0,"total_tokens":1,"input_token_details":{"text_tokens":1,"cached_tokens":0}}`), &usage))
	assert.True(t, usage.CachedTokensReported)
	require.NoError(t, kitutil.Unmarshal([]byte(`{}`), &usage))
	assert.False(t, usage.CachedTokensReported, "reusing a DTO must not preserve an earlier proof")
	assert.True(t, usage.UsageIncomplete)
}
