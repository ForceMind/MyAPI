package gemini

// This file contains the compatibility boundary for Google's Antigravity
// managed agent (the Gemini Interactions API). It intentionally does not make
// Antigravity look like a normal GenerateContent model: the request and
// response lifecycle is agentic and has different safety and billing
// semantics. Wiring it into the relay adaptor requires a dedicated interaction
// lifecycle and is therefore kept out of the existing Gemini wire path.

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/ForceMind/MyAPI/common"
)

const (
	// AntigravityAgentPreview is the currently documented managed agent name.
	// Keep this configurable because the preview identifier can change.
	AntigravityAgentPreview = "antigravity-preview-05-2026"
	// AntigravityInteractionsPath is relative to the Gemini API base URL.
	AntigravityInteractionsPath = "/v1beta/interactions"
	// AntigravityEnvironmentRemote is the managed Linux sandbox. MyAPI does not
	// infer or mount a user's local filesystem into an interaction.
	AntigravityEnvironmentRemote = "remote"
	// AntigravityMaxInputBytes bounds accidental oversized agent prompts.
	AntigravityMaxInputBytes = 4 << 20
	// AntigravityMaxTotalTokens is a conservative upper bound for the optional
	// agent budget. The service may apply a lower plan-specific limit.
	AntigravityMaxTotalTokens = 10_000_000
)

// AntigravityConfig contains non-secret connection and agent settings. The
// API key remains in the channel credential and is never copied here.
type AntigravityConfig struct {
	BaseURL        string
	Agent          string
	Environment    string
	Model          string
	MaxTotalTokens int
}

// AntigravityInteractionRequest is the minimal request supported by the
// managed Antigravity agent. Additional tools and stateful continuation are
// intentionally not represented until the relay lifecycle supports them.
type AntigravityInteractionRequest struct {
	Agent       string                   `json:"agent"`
	Input       string                   `json:"input"`
	Environment string                   `json:"environment,omitempty"`
	AgentConfig *AntigravityAgentConfig `json:"agent_config,omitempty"`
}

// AntigravityAgentConfig selects the managed agent's underlying model and
// optional total-token budget.
type AntigravityAgentConfig struct {
	Type           string `json:"type"`
	Model          string `json:"model,omitempty"`
	MaxTotalTokens int   `json:"max_total_tokens,omitempty"`
}

// SupportedAntigravityModels mirrors the models documented for the preview
// agent. Unknown models are rejected to avoid implying unsupported routing.
var SupportedAntigravityModels = map[string]struct{}{
	"gemini-3.7-flash":      {},
	"gemini-3.6-flash":      {},
	"gemini-3.5-flash":      {},
	"gemini-3.5-flash-lite": {},
}

// ValidateGenerationConfig rejects options that the Interactions API documents
// as unsupported. It is kept as a standalone guard so a future relay adapter
// cannot silently forward an OpenAI/Gemini generation config that Antigravity
// will reject with HTTP 400.
func ValidateAntigravityGenerationConfig(options map[string]any) error {
	for _, name := range []string{"temperature", "top_p", "top_k", "stop_sequences", "max_output_tokens"} {
		if _, exists := options[name]; exists {
			return fmt.Errorf("antigravity: generation option %q is unsupported", name)
		}
	}
	return nil
}

func (c AntigravityConfig) Validate() error {
	baseURL, err := normalizeAntigravityBaseURL(c.BaseURL)
	if err != nil {
		return err
	}
	if baseURL == "" {
		return errors.New("antigravity: base URL is required")
	}
	agent := strings.TrimSpace(c.Agent)
	if agent == "" {
		return errors.New("antigravity: agent is required")
	}
	environment := strings.TrimSpace(c.Environment)
	if environment != "" && environment != AntigravityEnvironmentRemote {
		return errors.New("antigravity: only the remote environment is supported")
	}
	if model := strings.TrimSpace(c.Model); model != "" {
		if _, ok := SupportedAntigravityModels[model]; !ok {
			return fmt.Errorf("antigravity: unsupported model %q", model)
		}
	}
	if c.MaxTotalTokens < 0 || c.MaxTotalTokens > AntigravityMaxTotalTokens {
		return fmt.Errorf("antigravity: max_total_tokens must be between 0 and %d", AntigravityMaxTotalTokens)
	}
	return nil
}

// Endpoint returns the Interactions API URL after validating the configured
// base. Credentials and query parameters are deliberately rejected.
func (c AntigravityConfig) Endpoint() (string, error) {
	baseURL, err := normalizeAntigravityBaseURL(c.BaseURL)
	if err != nil {
		return "", err
	}
	if err := c.Validate(); err != nil {
		return "", err
	}
	return baseURL + AntigravityInteractionsPath, nil
}

// BuildInteractionRequest validates and serializes a text/image-free prompt.
// The current relay boundary only accepts text; multimodal and stateful
// continuation need explicit lifecycle handling before being enabled.
func (c AntigravityConfig) BuildInteractionRequest(input string) ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errors.New("antigravity: input is required")
	}
	if len([]byte(input)) > AntigravityMaxInputBytes {
		return nil, fmt.Errorf("antigravity: input exceeds %d bytes", AntigravityMaxInputBytes)
	}
	request := AntigravityInteractionRequest{
		Agent:       strings.TrimSpace(c.Agent),
		Input:       input,
		Environment: AntigravityEnvironmentRemote,
	}
	if model := strings.TrimSpace(c.Model); model != "" || c.MaxTotalTokens > 0 {
		request.AgentConfig = &AntigravityAgentConfig{
			Type:           "dynamic",
			Model:          strings.TrimSpace(c.Model),
			MaxTotalTokens: c.MaxTotalTokens,
		}
	}
	return common.Marshal(request)
}

func normalizeAntigravityBaseURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("antigravity: base URL is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" {
		return "", errors.New("antigravity: base URL must use HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("antigravity: base URL must not contain credentials or query parameters")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
