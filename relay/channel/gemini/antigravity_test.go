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
	if err := ValidateAntigravityGenerationConfig(map[string]any{"max_output_tokens": 100}); err == nil {
		t.Fatal("expected unsupported generation option to fail")
	}
	if err := ValidateAntigravityGenerationConfig(map[string]any{"max_total_tokens": 100}); err != nil {
		t.Fatalf("expected agent budget option to be allowed: %v", err)
	}
}
