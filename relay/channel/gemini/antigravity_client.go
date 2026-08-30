package gemini

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/ForceMind/MyAPI/common"
)

const (
	// AntigravityDefaultHTTPTimeout bounds a caller that does not provide its
	// own HTTP client. Long-running background interactions should be polled by
	// calling Get rather than keeping a request open indefinitely.
	AntigravityDefaultHTTPTimeout = 60 * time.Second
	// AntigravityMaxResponseBytes prevents a malformed upstream from allocating
	// an unbounded response body in the relay process.
	AntigravityMaxResponseBytes = 16 << 20
)

// AntigravityClientConfig contains connection settings only. APIKey is held
// in memory for the request and is never included in an error, response, or
// persisted model.
type AntigravityClientConfig struct {
	BaseURL     string
	APIKey      string
	APIRevision string
	HTTPClient  *http.Client
}

// AntigravityLifecycleAgentConfig contains the small, explicitly supported
// dynamic-agent subset. The relay does not accept arbitrary tool, safety, or
// generation configuration until each field has a policy and billing model.
type AntigravityLifecycleAgentConfig struct {
	Type                 string `json:"type,omitempty"`
	MaxTotalTokens       int    `json:"max_total_tokens,omitempty"`
	PreviousInteractionID string `json:"previous_interaction_id,omitempty"`
}

// AntigravityLifecycleRequest is the first-phase create payload. It is kept
// separate from the normal Gemini request DTO so this endpoint cannot silently
// inherit GenerateContent fields or route through the regular Gemini path.
//
// EnvironmentID is retained as an optional forward-compatible field for
// providers exposing a named environment. The current Google preview uses the
// "remote" environment; local files, credentials, and process state are never
// mounted by this client.
type AntigravityLifecycleRequest struct {
	Agent                 string                              `json:"agent"`
	Input                 string                              `json:"input"`
	Environment           string                              `json:"environment,omitempty"`
	EnvironmentID         string                              `json:"environment_id,omitempty"`
	Background            bool                                `json:"background,omitempty"`
	PreviousInteractionID string                              `json:"previous_interaction_id,omitempty"`
	AgentConfig           *AntigravityLifecycleAgentConfig    `json:"agent_config,omitempty"`
}

// AntigravityModalityTokens and AntigravityGroundingToolCount model the
// provider usage response without retaining the full upstream payload.
type AntigravityModalityTokens struct {
	Modality string `json:"modality,omitempty"`
	Tokens   int64  `json:"tokens,omitempty"`
}

type AntigravityGroundingToolCount struct {
	Type  string `json:"type,omitempty"`
	Count int64  `json:"count,omitempty"`
}

type AntigravityUsage struct {
	CachedTokensByModality []AntigravityModalityTokens      `json:"cached_tokens_by_modality,omitempty"`
	InputTokensByModality  []AntigravityModalityTokens      `json:"input_tokens_by_modality,omitempty"`
	OutputTokensByModality []AntigravityModalityTokens      `json:"output_tokens_by_modality,omitempty"`
	TotalCachedTokens      int64                            `json:"total_cached_tokens,omitempty"`
	TotalInputTokens       int64                            `json:"total_input_tokens,omitempty"`
	TotalOutputTokens      int64                            `json:"total_output_tokens,omitempty"`
	TotalThoughtTokens     int64                            `json:"total_thought_tokens,omitempty"`
	TotalToolUseTokens     int64                            `json:"total_tool_use_tokens,omitempty"`
	TotalTokens            int64                            `json:"total_tokens,omitempty"`
	GroundingToolCount     []AntigravityGroundingToolCount  `json:"grounding_tool_count,omitempty"`
}

type AntigravityInteractionError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// AntigravityInteraction is the safe, typed subset returned by create/get/
// cancel. Steps and output text are intentionally represented only as fields
// needed by the caller; this client does not store or log them.
type AntigravityInteraction struct {
	ID                    string                       `json:"id,omitempty"`
	Object                string                       `json:"object,omitempty"`
	Status                string                       `json:"status,omitempty"`
	Agent                 string                       `json:"agent,omitempty"`
	Model                 string                       `json:"model,omitempty"`
	EnvironmentID         string                       `json:"environment_id,omitempty"`
	PreviousInteractionID string                       `json:"previous_interaction_id,omitempty"`
	OutputText            string                       `json:"output_text,omitempty"`
	Usage                 *AntigravityUsage            `json:"usage,omitempty"`
	Errors                []AntigravityInteractionError `json:"errors,omitempty"`
}

// AntigravityHTTPError deliberately omits response bodies. Upstream errors
// can contain prompt fragments or provider metadata, and returning only the
// status/code keeps them out of logs and API error strings.
type AntigravityHTTPError struct {
	StatusCode int
	Code       string
}

func (e *AntigravityHTTPError) Error() string {
	if e == nil {
		return "antigravity: upstream request failed"
	}
	if e.Code != "" {
		return fmt.Sprintf("antigravity: upstream request failed (status %d, code %s)", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("antigravity: upstream request failed (status %d)", e.StatusCode)
}

type AntigravityClient struct {
	baseURL     string
	apiKey      string
	apiRevision string
	httpClient  *http.Client
}

func NewAntigravityClient(config AntigravityClientConfig) (*AntigravityClient, error) {
	baseURL, err := normalizeAntigravityBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, errors.New("antigravity: API key is required")
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: AntigravityDefaultHTTPTimeout}
	}
	return &AntigravityClient{
		baseURL:     baseURL,
		apiKey:      config.APIKey,
		apiRevision: strings.TrimSpace(config.APIRevision),
		httpClient:  httpClient,
	}, nil
}

func (r AntigravityLifecycleRequest) Validate() error {
	if strings.TrimSpace(r.Agent) != AntigravityAgentPreview {
		return fmt.Errorf("antigravity: only the documented preview agent %q is supported", AntigravityAgentPreview)
	}
	input := strings.TrimSpace(r.Input)
	if input == "" {
		return errors.New("antigravity: input is required")
	}
	if len([]byte(input)) > AntigravityMaxInputBytes {
		return fmt.Errorf("antigravity: input exceeds %d bytes", AntigravityMaxInputBytes)
	}
	if environment := strings.TrimSpace(r.Environment); environment != "" && environment != AntigravityEnvironmentRemote {
		return errors.New("antigravity: only the remote environment is supported")
	}
	if err := validateInteractionID(r.EnvironmentID, "environment_id"); err != nil {
		return err
	}
	if err := validateInteractionID(r.PreviousInteractionID, "previous_interaction_id"); err != nil {
		return err
	}
	if r.AgentConfig != nil {
		if r.AgentConfig.Type != "" && r.AgentConfig.Type != "dynamic" {
			return errors.New("antigravity: agent_config.type must be dynamic")
		}
		if r.AgentConfig.MaxTotalTokens < 0 || r.AgentConfig.MaxTotalTokens > AntigravityMaxTotalTokens {
			return fmt.Errorf("antigravity: max_total_tokens must be between 0 and %d", AntigravityMaxTotalTokens)
		}
		if err := validateInteractionID(r.AgentConfig.PreviousInteractionID, "agent_config.previous_interaction_id"); err != nil {
			return err
		}
		if r.PreviousInteractionID != "" && r.AgentConfig.PreviousInteractionID != "" &&
			r.PreviousInteractionID != r.AgentConfig.PreviousInteractionID {
			return errors.New("antigravity: previous_interaction_id is specified more than once with different values")
		}
	}
	return nil
}

// Create starts an interaction. Background executions return quickly with an
// in_progress resource; callers can use Get or Poll for state transitions.
func (c *AntigravityClient) Create(ctx context.Context, request AntigravityLifecycleRequest) (*AntigravityInteraction, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if request.Environment == "" {
		request.Environment = AntigravityEnvironmentRemote
	}
	if request.AgentConfig != nil && request.AgentConfig.Type == "" {
		request.AgentConfig.Type = "dynamic"
	}
	return c.doJSON(ctx, http.MethodPost, AntigravityInteractionsPath, request, false)
}

// Get retrieves the current interaction state. It does not persist the body;
// callers decide which fields to expose or log.
func (c *AntigravityClient) Get(ctx context.Context, interactionID string) (*AntigravityInteraction, error) {
	path, err := interactionPath(interactionID)
	if err != nil {
		return nil, err
	}
	return c.doJSON(ctx, http.MethodGet, path, nil, false)
}

// Poll waits for a terminal state with bounded attempts. It is intentionally
// opt-in so a background request never consumes a worker indefinitely.
func (c *AntigravityClient) Poll(ctx context.Context, interactionID string, interval time.Duration, attempts int) (*AntigravityInteraction, error) {
	if attempts <= 0 {
		return nil, errors.New("antigravity: poll attempts must be positive")
	}
	if interval < 0 {
		return nil, errors.New("antigravity: poll interval must not be negative")
	}
	for attempt := 0; attempt < attempts; attempt++ {
		interaction, err := c.Get(ctx, interactionID)
		if err != nil {
			return nil, err
		}
		if isAntigravityTerminalStatus(interaction.Status) || attempt == attempts-1 {
			return interaction, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, errors.New("antigravity: polling failed")
}

// Cancel stops a still-running background interaction using the documented
// POST /interactions/{id}/cancel endpoint.
func (c *AntigravityClient) Cancel(ctx context.Context, interactionID string) (*AntigravityInteraction, error) {
	path, err := interactionPath(interactionID)
	if err != nil {
		return nil, err
	}
	return c.doJSON(ctx, http.MethodPost, path+"/cancel", nil, false)
}

// Delete removes a stored interaction. It is separate from Cancel: cancelling
// stops execution while deleting removes the provider-side record.
func (c *AntigravityClient) Delete(ctx context.Context, interactionID string) error {
	path, err := interactionPath(interactionID)
	if err != nil {
		return err
	}
	_, err = c.doJSON(ctx, http.MethodDelete, path, nil, true)
	return err
}

func (c *AntigravityClient) doJSON(ctx context.Context, method, path string, payload any, allowEmpty bool) (*AntigravityInteraction, error) {
	if c == nil || c.httpClient == nil {
		return nil, errors.New("antigravity: client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var body io.Reader
	if payload != nil {
		encoded, err := common.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("antigravity: encode request: %w", err)
		}
		body = strings.NewReader(string(encoded))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, errors.New("antigravity: build upstream request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiRevision != "" {
		req.Header.Set("Api-Revision", c.apiRevision)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("antigravity: upstream request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newAntigravityHTTPError(resp)
	}
	limited := io.LimitReader(resp.Body, AntigravityMaxResponseBytes)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, errors.New("antigravity: read upstream response")
	}
	if len(data) == 0 && allowEmpty {
		return nil, nil
	}
	if len(data) == 0 {
		return nil, errors.New("antigravity: upstream returned an empty response")
	}
	var interaction AntigravityInteraction
	if err := common.Unmarshal(data, &interaction); err != nil {
		return nil, errors.New("antigravity: decode upstream response")
	}
	return &interaction, nil
}

func newAntigravityHTTPError(resp *http.Response) error {
	if resp == nil {
		return &AntigravityHTTPError{}
	}
	// Do not decode or include the response body; provider errors can contain
	// sensitive prompt/tool details. A stable status is enough for retry policy.
	return &AntigravityHTTPError{StatusCode: resp.StatusCode}
}

func interactionPath(id string) (string, error) {
	id = strings.TrimSpace(id)
	if err := validateInteractionID(id, "interaction_id"); err != nil {
		return "", err
	}
	return AntigravityInteractionsPath + "/" + url.PathEscape(id), nil
}

func validateInteractionID(value, field string) error {
	if value == "" {
		return nil
	}
	if len(value) > 256 {
		return fmt.Errorf("antigravity: %s is too long", field)
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == '/' || r == '\\' || r == '?' || r == '#' {
			return fmt.Errorf("antigravity: %s contains unsupported characters", field)
		}
	}
	return nil
}

func isAntigravityTerminalStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "cancelled", "incomplete":
		return true
	default:
		return false
	}
}
