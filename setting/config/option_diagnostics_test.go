package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type optionDiagnosticsScalarConfig struct {
	Enabled bool     `json:"enabled"`
	Count   int      `json:"count"`
	Ratio   float64  `json:"ratio"`
	Names   []string `json:"names"`
}

type optionDiagnosticsMapConfig struct{}

func (optionDiagnosticsMapConfig) ExportConfigMap() (map[string]string, error) { return nil, nil }
func (optionDiagnosticsMapConfig) UpdateConfigMap(map[string]string) error     { return nil }

type optionDiagnosticsValidatingMapConfig struct{ optionDiagnosticsMapConfig }

func (optionDiagnosticsValidatingMapConfig) ValidateConfigMap(values map[string]string) error {
	panic("option diagnostics must not invoke validating MapConfig")
}

// optionDiagnosticsDescribingMapConfig is the shape every managed runtime
// generation uses: the registered object hides its live value behind a
// wrapper, and only a zero-value schema is offered to diagnostics.
type optionDiagnosticsDescribingMapConfig struct{ optionDiagnosticsValidatingMapConfig }

func (optionDiagnosticsDescribingMapConfig) DiagnosticSchema() any {
	return &optionDiagnosticsScalarConfig{}
}

func TestConfigManagerAssessOptionDiagnosticUsesStableCodes(t *testing.T) {
	manager := NewConfigManager()
	manager.Register("scalar", &optionDiagnosticsScalarConfig{})
	manager.Register("map", optionDiagnosticsMapConfig{})
	manager.Register("validated", optionDiagnosticsValidatingMapConfig{})
	manager.Register("described", optionDiagnosticsDescribingMapConfig{})

	for _, testCase := range []struct {
		key, value, keyClass, validation string
	}{
		{"scalar.enabled", "not-bool", "registered_field", "invalid_boolean"},
		{"scalar.count", "1.5", "registered_field", "invalid_integer"},
		{"scalar.ratio", "NaN", "registered_field", "invalid_finite_number"},
		{"scalar.names", "{", "registered_field", "invalid_json"},
		{"scalar.missing", "x", "unknown_field", ""},
		{"missing.value", "x", "unknown_prefix", ""},
		{"scalar..enabled", "x", "malformed_key", ""},
		{"scalar." + string([]byte{0xff}), "x", "malformed_key", ""},
		{strings.Repeat("a", OptionDiagnosticMaxKeyChars), "x", "legacy_flat", ""},
		{strings.Repeat("a", OptionDiagnosticMaxKeyChars+1), "x", "malformed_key", ""},
		{"scalar." + strings.Repeat("a", OptionDiagnosticMaxKeyChars), "x", "malformed_key", ""},
		{"map.value", "x", "schema_indeterminate", "unsupported"},
		{"validated.level", "unsafe", "schema_indeterminate", "unsupported"},
		{"described.enabled", "not-bool", "registered_field", "invalid_boolean"},
		{"described.names", "{", "registered_field", "invalid_json"},
		{"described.enabled", "true", "registered_field", ""},
		{"described.missing", "x", "unknown_field", ""},
	} {
		t.Run(testCase.key, func(t *testing.T) {
			actual := manager.AssessOptionDiagnostic(testCase.key, testCase.value)
			assert.Equal(t, testCase.keyClass, actual.KeyClass)
			assert.Equal(t, testCase.validation, actual.Validation)
		})
	}

	require.Equal(t, "registered_field", manager.AssessOptionDiagnostic("scalar.enabled", "true").KeyClass)
}
