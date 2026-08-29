// Package tokenhub contains the provider-neutral configuration contract for
// TokenHub-compatible gateways. Transport and request conversion stay in the
// existing OpenAI/Anthropic adaptors until a channel is explicitly selected;
// this package deliberately does not read local credentials or log secrets.
package tokenhub

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
)

type Protocol string

const (
	ProtocolOpenAI    Protocol = "openai"
	ProtocolResponses Protocol = "responses"
	ProtocolAnthropic Protocol = "anthropic"
)

const (
	defaultModelListPath = "/v1/models"
	defaultHealthPath    = "/health"
	maxAliasCount        = 256
	maxModelBodyBytes    = 2 << 20
)

// Config is the stable boundary between MyAPI routing policy and a TokenHub
// upstream. APIKey is held by the caller and is never included in errors or
// String methods.
type Config struct {
	BaseURL             string
	Region              string
	Protocol            Protocol
	ModelListPath       string
	HealthPath          string
	AllowModelDiscovery bool
	ModelAliases        map[string]string
}

func (c Config) Validate() error {
	baseURL, err := NormalizeBaseURL(c.BaseURL)
	if err != nil {
		return err
	}
	if baseURL == "" {
		return errors.New("tokenhub: base URL is required")
	}
	switch c.Protocol {
	case "", ProtocolOpenAI, ProtocolResponses, ProtocolAnthropic:
	default:
		return errors.New("tokenhub: unsupported protocol")
	}
	if c.ModelListPath != "" {
		if err := validatePath(c.ModelListPath, "model list path"); err != nil {
			return err
		}
	}
	if c.HealthPath != "" {
		if err := validatePath(c.HealthPath, "health path"); err != nil {
			return err
		}
	}
	if len(c.ModelAliases) > maxAliasCount {
		return fmt.Errorf("tokenhub: too many model aliases (maximum %d)", maxAliasCount)
	}
	for alias, target := range c.ModelAliases {
		if strings.TrimSpace(alias) == "" || strings.TrimSpace(target) == "" {
			return errors.New("tokenhub: model aliases must have non-empty names")
		}
	}
	return nil
}

// FromSettings converts persisted channel settings into a validated runtime
// config. It does not inspect or copy the channel credential.
func FromSettings(settings *dto.TokenHubSettings) (Config, error) {
	if settings == nil || !settings.Enabled {
		return Config{}, errors.New("tokenhub: configuration is not enabled")
	}
	c := Config{
		BaseURL:             settings.BaseURL,
		Region:              strings.TrimSpace(settings.Region),
		Protocol:            Protocol(strings.ToLower(strings.TrimSpace(settings.Protocol))),
		ModelListPath:       strings.TrimSpace(settings.ModelListPath),
		HealthPath:          strings.TrimSpace(settings.HealthPath),
		AllowModelDiscovery: settings.AllowModelDiscovery,
		ModelAliases:        cloneAliases(settings.ModelAliases),
	}
	if baseURL, err := NormalizeBaseURL(c.BaseURL); err == nil {
		c.BaseURL = baseURL
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func cloneAliases(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return output
}

func validatePath(value, label string) error {
	if value == "" || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "?#") {
		return fmt.Errorf("tokenhub: %s must be an absolute path without query parameters", label)
	}
	clean := path.Clean(value)
	if clean != value || strings.Contains(clean, "..") {
		return fmt.Errorf("tokenhub: %s contains an invalid path", label)
	}
	return nil
}

func (c Config) modelListPath() string {
	if c.ModelListPath == "" {
		return defaultModelListPath
	}
	return c.ModelListPath
}

func (c Config) healthPath() string {
	if c.HealthPath == "" {
		return defaultHealthPath
	}
	return c.HealthPath
}

func (c Config) Endpoint(requestPath string) (string, error) {
	baseURL, err := NormalizeBaseURL(c.BaseURL)
	if err != nil {
		return "", err
	}
	if err := validatePath(requestPath, "request path"); err != nil {
		return "", err
	}
	return baseURL + requestPath, nil
}

// ResolveModel applies an optional administrative alias without changing the
// caller's model string when no alias is configured.
func (c Config) ResolveModel(model string) string {
	model = strings.TrimSpace(model)
	if target, ok := c.ModelAliases[model]; ok && strings.TrimSpace(target) != "" {
		return strings.TrimSpace(target)
	}
	return model
}

// Model is the provider-neutral subset used by model discovery and UI
// metadata. Unknown fields are intentionally ignored for compatibility.
type Model struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by,omitempty"`
}

type modelListResponse struct {
	Data []Model `json:"data"`
}

type HealthSnapshot struct {
	Healthy      bool
	StatusCode   int
	Latency      time.Duration
	CheckedAt    time.Time
	FailureCause string
}

// Client provides bounded, secret-safe probes for an explicitly configured
// TokenHub endpoint. Routing and billing stay in MyAPI's existing services.
type Client struct {
	Config Config
	apiKey string
	http   *http.Client
}

// NewClient creates a probe client. The credential is kept private so a
// caller cannot accidentally serialize or print it as part of Client state.
func NewClient(config Config, apiKey string, httpClient *http.Client) Client {
	return Client{Config: config, apiKey: apiKey, http: httpClient}
}

func (c Client) client() *http.Client {
	if c.http != nil {
		return c.http
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (c Client) request(ctx context.Context, requestPath string) (*http.Response, error) {
	if err := c.Config.Validate(); err != nil {
		return nil, err
	}
	endpoint, err := c.Config.Endpoint(requestPath)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.New("tokenhub: failed to create request")
	}
	if strings.TrimSpace(c.apiKey) != "" {
		if c.Config.Protocol == ProtocolAnthropic {
			req.Header.Set("x-api-key", c.apiKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		}
	}
	req.Header.Set("Accept", "application/json")
	return c.client().Do(req)
}

func (c Client) Models(ctx context.Context) ([]Model, error) {
	if !c.Config.AllowModelDiscovery {
		return nil, errors.New("tokenhub: model discovery is disabled")
	}
	resp, err := c.request(ctx, c.Config.modelListPath())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("tokenhub: model discovery returned HTTP %d", resp.StatusCode)
	}
	var payload modelListResponse
	if err := common.DecodeJson(io.LimitReader(resp.Body, maxModelBodyBytes), &payload); err != nil {
		return nil, errors.New("tokenhub: invalid model discovery response")
	}
	models := make([]Model, 0, len(payload.Data))
	for _, model := range payload.Data {
		if strings.TrimSpace(model.ID) != "" {
			models = append(models, model)
		}
	}
	return models, nil
}

func (c Client) Health(ctx context.Context) HealthSnapshot {
	snapshot := HealthSnapshot{CheckedAt: time.Now()}
	started := time.Now()
	resp, err := c.request(ctx, c.Config.healthPath())
	snapshot.Latency = time.Since(started)
	if err != nil {
		snapshot.FailureCause = err.Error()
		return snapshot
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	snapshot.StatusCode = resp.StatusCode
	snapshot.Healthy = resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
	if !snapshot.Healthy {
		snapshot.FailureCause = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return snapshot
}

// NormalizeBaseURL validates and canonicalizes a configured gateway endpoint.
// Only HTTP(S) URLs with a host are accepted; credentials and query strings
// are rejected so an accidental secret cannot be embedded in the endpoint.
func NormalizeBaseURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("tokenhub: base URL is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("tokenhub: base URL must be an HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("tokenhub: base URL must not contain credentials or query parameters")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
