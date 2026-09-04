package model_setting

import (
	"net/http"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeSettingsWriteHeadersMergesConfiguredValuesIntoSingleHeader(t *testing.T) {
	settings := &ClaudeSettings{
		HeadersSettings: map[string]map[string][]string{
			"claude-3-7-sonnet-20250219-thinking": {
				"anthropic-beta": {
					"token-efficient-tools-2025-02-19",
				},
			},
		},
	}

	headers := http.Header{}
	headers.Set("anthropic-beta", "output-128k-2025-02-19")

	settings.WriteHeaders("claude-3-7-sonnet-20250219-thinking", &headers)

	got := headers.Values("anthropic-beta")
	if len(got) != 1 {
		t.Fatalf("expected a single merged header value, got %v", got)
	}
	expected := "output-128k-2025-02-19,token-efficient-tools-2025-02-19"
	if got[0] != expected {
		t.Fatalf("expected merged header %q, got %q", expected, got[0])
	}
}

func TestClaudeSettingsWriteHeadersDeduplicatesAcrossCommaSeparatedAndRepeatedValues(t *testing.T) {
	settings := &ClaudeSettings{
		HeadersSettings: map[string]map[string][]string{
			"claude-3-7-sonnet-20250219-thinking": {
				"anthropic-beta": {
					"token-efficient-tools-2025-02-19",
					"computer-use-2025-01-24",
				},
			},
		},
	}

	headers := http.Header{}
	headers.Add("anthropic-beta", "output-128k-2025-02-19, token-efficient-tools-2025-02-19")
	headers.Add("anthropic-beta", "token-efficient-tools-2025-02-19")

	settings.WriteHeaders("claude-3-7-sonnet-20250219-thinking", &headers)

	got := headers.Values("anthropic-beta")
	if len(got) != 1 {
		t.Fatalf("expected duplicate values to collapse into one header, got %v", got)
	}
	expected := "output-128k-2025-02-19,token-efficient-tools-2025-02-19,computer-use-2025-01-24"
	if got[0] != expected {
		t.Fatalf("expected deduplicated merged header %q, got %q", expected, got[0])
	}
}

func TestValidateClaudeDefaultMaxTokens(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr string
	}{
		{name: "positive default", value: `{"default": 8192}`},
		{name: "zero allowed", value: `{"default": 0}`},
		{name: "zero model override allowed", value: `{"default": 8192, "claude-test": 0}`},
		{name: "empty map allowed", value: `{}`},
		{name: "negative default rejected", value: `{"default": -1}`, wantErr: `negative Claude default max_tokens -1 for "default"`},
		{name: "negative model override rejected", value: `{"default": 8192, "claude-test": -5}`, wantErr: `negative Claude default max_tokens -5 for "claude-test"`},
		{name: "above billing limit rejected", value: `{"default": 1073741824}`, wantErr: "exceeds limit"},
		{name: "non-integer rejected", value: `{"default": "high"}`, wantErr: "JSON map of model to integer"},
		{name: "null rejected", value: `null`, wantErr: "JSON map of model to integer"},
		{name: "malformed rejected", value: `{`, wantErr: "JSON map of model to integer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateClaudeDefaultMaxTokens(tt.value)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateClaudeThinkingAdapterBudgetTokensPercentage(t *testing.T) {
	for _, value := range []float64{0, -0.1, 1.1} {
		require.Error(t, ValidateClaudeThinkingAdapterBudgetTokensPercentage(value))
	}
	for _, value := range []float64{0.01, 0.8, 1} {
		require.NoError(t, ValidateClaudeThinkingAdapterBudgetTokensPercentage(value))
	}
}

func TestGetClaudeSettingsReturnsDeepCopyWithoutChangingExportedState(t *testing.T) {
	registered := config.GlobalConfig.Get("claude")
	require.NotNil(t, registered)
	original, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.UpdateConfigFromMap(registered, original)) })

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"model_headers_settings": `{"claude-test":{"anthropic-beta":["one","two"]}}`,
		"default_max_tokens":     `{}`,
	}))

	first := GetClaudeSettings()
	assert.Equal(t, ClaudeDefaultMaxTokens, first.DefaultMaxTokens["default"])
	first.HeadersSettings["claude-test"]["anthropic-beta"][0] = "changed"
	first.HeadersSettings["claude-test"]["new-header"] = []string{"added"}
	first.HeadersSettings["new-model"] = map[string][]string{"x": {"y"}}
	first.DefaultMaxTokens["default"] = 1

	second := GetClaudeSettings()
	assert.Equal(t, []string{"one", "two"}, second.HeadersSettings["claude-test"]["anthropic-beta"])
	assert.NotContains(t, second.HeadersSettings["claude-test"], "new-header")
	assert.NotContains(t, second.HeadersSettings, "new-model")
	assert.Equal(t, ClaudeDefaultMaxTokens, second.DefaultMaxTokens["default"])

	exported, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, exported["default_max_tokens"], "getter-only default must not be published")
}

func TestClaudeManagedConfigHandlesNullDefaultWithoutPublishingGetterDefault(t *testing.T) {
	registered := config.GlobalConfig.Get("claude")
	require.NotNil(t, registered)
	original, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.UpdateConfigFromMap(registered, original)) })

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{"default_max_tokens": "null"}))
	assert.Equal(t, ClaudeDefaultMaxTokens, GetClaudeSettings().GetDefaultMaxTokens("unknown"))

	exported, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	assert.Equal(t, "null", exported["default_max_tokens"])
}

func TestClaudeManagedConfigHeadersPreserveEmptyAndNilSlicesOnExport(t *testing.T) {
	registered := config.GlobalConfig.Get("claude")
	require.NotNil(t, registered)
	original, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.UpdateConfigFromMap(registered, original)) })

	const headersJSON = `{"claude-test":{"empty":[],"nil":null}}`
	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"model_headers_settings": headersJSON,
	}))

	exported, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	assert.JSONEq(t, headersJSON, exported["model_headers_settings"])
}

func TestClaudeManagedConfigValidationIsPureAndFailedUpdateDoesNotPublish(t *testing.T) {
	registered := config.GlobalConfig.Get("claude")
	require.NotNil(t, registered)
	original, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.UpdateConfigFromMap(registered, original)) })

	valid := map[string]string{
		"thinking_adapter_enabled":                  "false",
		"thinking_adapter_budget_tokens_percentage": "0.5",
		"default_max_tokens":                        `{"default":4096}`,
	}
	require.NoError(t, config.ValidateConfigFromMap(registered, valid))
	afterValidation, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	assert.Equal(t, original, afterValidation)

	invalid := map[string]string{
		"thinking_adapter_enabled":                  "false",
		"thinking_adapter_budget_tokens_percentage": "0",
	}
	require.Error(t, config.UpdateConfigFromMap(registered, invalid))
	afterFailure, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	assert.Equal(t, original, afterFailure)

	require.NoError(t, config.UpdateConfigFromMap(registered, valid))
	published := GetClaudeSettings()
	assert.False(t, published.ThinkingAdapterEnabled)
	assert.Equal(t, 0.5, published.ThinkingAdapterBudgetTokensPercentage)
	assert.Equal(t, 4096, published.GetDefaultMaxTokens("unknown"))
}

func TestClaudeManagedConfigPublishesWholeGenerations(t *testing.T) {
	registered := config.GlobalConfig.Get("claude")
	require.NotNil(t, registered)
	original, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.UpdateConfigFromMap(registered, original)) })

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"thinking_adapter_enabled": "true",
		"default_max_tokens":       `{"default":1111}`,
	}))

	start := make(chan struct{})
	updateErrCh := make(chan error, 1)
	snapshotCh := make(chan *ClaudeSettings, 1)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		updateErrCh <- config.UpdateConfigFromMap(registered, map[string]string{
			"thinking_adapter_enabled": "false",
			"default_max_tokens":       `{"default":2222}`,
		})
	}()
	go func() {
		defer wg.Done()
		<-start
		snapshotCh <- GetClaudeSettings()
	}()
	close(start)
	wg.Wait()
	require.NoError(t, <-updateErrCh)

	observed := <-snapshotCh
	switch observed.GetDefaultMaxTokens("unknown") {
	case 1111:
		assert.True(t, observed.ThinkingAdapterEnabled)
	case 2222:
		assert.False(t, observed.ThinkingAdapterEnabled)
	default:
		t.Fatalf("unexpected Claude generation: %+v", observed)
	}
	published := GetClaudeSettings()
	assert.False(t, published.ThinkingAdapterEnabled)
	assert.Equal(t, 2222, published.GetDefaultMaxTokens("unknown"))
}
