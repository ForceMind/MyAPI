package gemini

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAntigravityConfigBuildsBoundedInteractionRequest(t *testing.T) {
	config := AntigravityConfig{
		BaseURL:        " https://generativelanguage.googleapis.com/ ",
		Agent:          AntigravityAgentPreview,
		Environment:    AntigravityEnvironmentRemote,
		Model:          "gemini-3.7-flash",
		MaxTotalTokens: 4096,
	}
	body, err := config.BuildInteractionRequest("  inspect this repository  ")
	if err != nil {
		t.Fatalf("BuildInteractionRequest() error = %v", err)
	}
	var got AntigravityInteractionRequest
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Agent != AntigravityAgentPreview || got.Input != "inspect this repository" || got.Environment != AntigravityEnvironmentRemote {
		t.Fatalf("unexpected request: %+v", got)
	}
	if got.AgentConfig == nil || got.AgentConfig.Type != "antigravity" || got.AgentConfig.MaxTotalTokens != 4096 {
		t.Fatalf("unexpected agent config: %+v", got.AgentConfig)
	}
}

func TestAntigravityRequestContainsOnlySupportedLifecycleFields(t *testing.T) {
	config := AntigravityConfig{
		BaseURL: "https://example.test",
		Agent:   AntigravityAgentPreview,
	}
	body, err := config.BuildInteractionRequest("hello")
	if err != nil {
		t.Fatalf("BuildInteractionRequest() error = %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, name := range []string{"agent", "input", "environment"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("request is missing supported field %q: %s", name, body)
		}
	}
	if _, ok := fields["agent_config"]; ok {
		t.Errorf("agent_config should be omitted when no optional config is set: %s", body)
	}

	// These fields belong to stateful/tool-enabled interaction lifecycles. They
	// must not be accidentally inherited from an OpenAI or Gemini request until
	// a dedicated Antigravity lifecycle explicitly supports them.
	for _, name := range []string{
		"previous_interaction_id", "stream", "tools", "tool_config", "response_format",
		"background", "safety_settings", "generation_config", "temperature", "top_p",
		"top_k", "stop_sequences", "max_output_tokens",
	} {
		if _, ok := fields[name]; ok {
			t.Errorf("unsupported lifecycle field %q leaked into request: %s", name, body)
		}
	}
}

func TestAntigravityRejectsUnsafeOrUnsupportedConfiguration(t *testing.T) {
	tests := []AntigravityConfig{
		{BaseURL: "http://example.test", Agent: AntigravityAgentPreview},
		{BaseURL: "https://user:secret@example.test", Agent: AntigravityAgentPreview},
		{BaseURL: "https://example.test?key=secret", Agent: AntigravityAgentPreview},
		{BaseURL: "https://example.test", Agent: AntigravityAgentPreview, Environment: "local"},
		{BaseURL: "https://example.test", Agent: AntigravityAgentPreview, Model: "claude-opus"},
		{BaseURL: "https://example.test", Agent: AntigravityAgentPreview, MaxTotalTokens: AntigravityMaxTotalTokens + 1},
	}
	for _, config := range tests {
		if err := config.Validate(); err == nil {
			t.Fatalf("expected config to be rejected: %+v", config)
		}
	}
}

func TestAntigravityRejectsEmptyAndOversizedInput(t *testing.T) {
	config := AntigravityConfig{BaseURL: "https://example.test", Agent: AntigravityAgentPreview}
	if _, err := config.BuildInteractionRequest(" \n\t"); err == nil {
		t.Fatal("expected empty input to fail")
	}
	if _, err := config.BuildInteractionRequest(strings.Repeat("x", AntigravityMaxInputBytes+1)); err == nil {
		t.Fatal("expected oversized input to fail")
	}
}

func TestAntigravityEndpointUsesInteractionsPath(t *testing.T) {
	endpoint, err := (AntigravityConfig{BaseURL: "https://example.test/api", Agent: AntigravityAgentPreview}).Endpoint()
	if err != nil {
		t.Fatalf("Endpoint() error = %v", err)
	}
	if endpoint != "https://example.test/api/v1beta/interactions" {
		t.Fatalf("unexpected endpoint: %s", endpoint)
	}
}

func TestValidateAntigravityGenerationConfig(t *testing.T) {
	for _, name := range []string{"temperature", "top_p", "top_k", "stop_sequences", "max_output_tokens"} {
		if err := ValidateAntigravityGenerationConfig(map[string]any{name: 100}); err == nil {
			t.Fatalf("expected unsupported generation option %q to fail", name)
		}
	}
	if err := ValidateAntigravityGenerationConfig(map[string]any{"max_total_tokens": 100}); err != nil {
		t.Fatalf("expected agent budget option to be allowed: %v", err)
	}
}

func TestAntigravityEndpointNeverCarriesCredentialOrQueryData(t *testing.T) {
	for _, baseURL := range []string{
		"https://user:secret@example.test",
		"https://example.test?key=secret",
		"https://example.test#fragment",
	} {
		config := AntigravityConfig{BaseURL: baseURL, Agent: AntigravityAgentPreview}
		if endpoint, err := config.Endpoint(); err == nil {
			t.Errorf("Endpoint(%q) unexpectedly succeeded with %q", baseURL, endpoint)
		}
	}
}
