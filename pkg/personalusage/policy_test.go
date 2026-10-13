package personalusage

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateAdmission(t *testing.T) {
	tests := []struct {
		name         string
		noBalance    bool
		revision     int64
		fundingState *FundingState
		want         AdmissionDecision
	}{
		{
			name:         "configured funding ignores revision and funding state",
			noBalance:    false,
			revision:     -1,
			fundingState: &FundingState{Disabled: true},
			want:         ConfiguredFunding,
		},
		{
			name:      "zero revision is invalid",
			noBalance: true,
			revision:  0,
			want:      InvalidPolicy,
		},
		{
			name:      "negative revision is invalid",
			noBalance: true,
			revision:  -1,
			want:      InvalidPolicy,
		},
		{
			name:      "missing funding state requests loading",
			noBalance: true,
			revision:  1,
			want:      NeedFundingState,
		},
		{
			name:         "invalid funding state is unavailable",
			noBalance:    true,
			revision:     1,
			fundingState: &FundingState{Disabled: true},
			want:         FundingUnavailable,
		},
		{
			name:         "valid disabled funding state selects self use",
			noBalance:    true,
			revision:     1,
			fundingState: &FundingState{Valid: true, Disabled: true},
			want:         SelfUseFunding,
		},
		{
			name:         "valid non-disabled state covers enabled and retirement",
			noBalance:    true,
			revision:     1,
			fundingState: &FundingState{Valid: true},
			want:         ConfiguredFunding,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, EvaluateAdmission(test.noBalance, test.revision, test.fundingState))
		})
	}
}

func TestSupportsMeteredRequest(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		rawQuery string
		want     bool
	}{
		{name: "chat completions", method: "POST", path: "/v1/chat/completions", want: true},
		{name: "responses", method: "POST", path: "/v1/responses", want: true},
		{name: "responses compact", method: "POST", path: "/v1/responses/compact", want: true},
		{name: "other method", method: "GET", path: "/v1/responses", want: false},
		{name: "raw query", method: "POST", path: "/v1/responses", rawQuery: "trace=1", want: false},
		{name: "other path", method: "POST", path: "/v1/images/generations", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, SupportsMeteredRequest(test.method, test.path, test.rawQuery))
		})
	}
}

func TestProductionDependenciesAreStandardLibraryOnly(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if and .DepOnly (not .Standard)}}{{.ImportPath}}{{end}}", ".")
	command.Env = make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GOWORK=") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "GOWORK=off")

	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Empty(t, strings.Fields(string(output)), "production package must depend only on the standard library")
}
