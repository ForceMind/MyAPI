package dto

import (
	"testing"

	kitutil "github.com/ForceMind/MyAPI/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealtimeSessionAutomaticResponseEvidence(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		disabled bool
	}{
		{`{}`, false},
		{`{"AutomaticResponseDisabled":true}`, false},
		{`{"turn_detection":null}`, true},
		{`{"turn_detection":{"type":"server_vad"}}`, false},
		{`{"turn_detection":{"type":"server_vad","create_response":false}}`, true},
		{`{"turn_detection":{"create_response":null}}`, false},
		{`{"turn_detection":{"create_response":true}}`, false},
		{`{"audio":{"input":{"turn_detection":null}}}`, true},
		{`{"audio":{"input":{"turn_detection":{"create_response":false}}}}`, true},
		{`{"turn_detection":null,"audio":{"input":{"turn_detection":{"create_response":true}}}}`, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			var session RealtimeSession
			require.NoError(t, kitutil.Unmarshal([]byte(tc.raw), &session))
			assert.Equal(t, tc.disabled, session.AutomaticResponseDisabled)
			encoded, err := kitutil.Marshal(session)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "AutomaticResponseDisabled")
			require.NoError(t, kitutil.Unmarshal([]byte(`{}`), &session))
			assert.False(t, session.AutomaticResponseDisabled, "missing new state cannot reuse old proof")
		})
	}
}
