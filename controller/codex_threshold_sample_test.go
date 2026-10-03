package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexThresholdSampleRequiresNativeCompleteReportedWindows(t *testing.T) {
	const valid = `{"plan_type":"pro","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":0,"reset_at":200,"limit_window_seconds":3600},"secondary_window":null}}`
	for _, tc := range []struct {
		name, body   string
		native, want bool
	}{
		{"explicit zero and absent secondary", valid, true, true},
		{"compatibility source", valid, false, false},
		{"missing secondary slot", strings.Replace(valid, `,"secondary_window":null`, "", 1), true, false},
		{"missing percentage", strings.Replace(valid, `"used_percent":0,`, "", 1), true, false},
		{"null percentage", strings.Replace(valid, `"used_percent":0`, `"used_percent":null`, 1), true, false},
		{"duplicate percentage", strings.Replace(valid, `"used_percent":0`, `"used_percent":0,"used_percent":5`, 1), true, false},
		{"missing reset", strings.Replace(valid, `"reset_at":200,`, "", 1), true, false},
		{"unreported additional window", strings.Replace(valid, `"secondary_window":null`, `"secondary_window":null,"tertiary_window":null`, 1), true, false},
		{"not allowed", strings.Replace(valid, `"allowed":true`, `"allowed":false`, 1), true, false},
		{"limit reached", strings.Replace(valid, `"limit_reached":false`, `"limit_reached":true`, 1), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := normalizeCodexUsageSnapshots(1, 100, 200, []byte(tc.body))
			require.NotEmpty(t, rows)
			qualifyCodexThresholdSnapshots(rows, 200, []byte(tc.body), tc.native)
			for _, row := range rows {
				assert.Equal(t, tc.want, row.CodexThresholdQualified)
			}
		})
	}
}

func TestCodexMissingPercentageIsNotFullBalanceOrWindowAbsence(t *testing.T) {
	rows := normalizeCodexUsageSnapshots(1, 100, 200, []byte(`{"rate_limit":{"primary_window":{},"secondary_window":{"used_percent":20,"reset_at":200,"limit_window_seconds":3600}}}`))
	require.Len(t, rows, 1)
	assert.Equal(t, "unsupported", rows[0].Status)
	assert.Equal(t, "codex_wham_usage", rows[0].Source)
	assert.Equal(t, "invalid_payload", rows[0].ErrorCode)
	assert.False(t, rows[0].CodexThresholdQualified)
}
