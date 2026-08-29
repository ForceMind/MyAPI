// Package tokenhub contains the provider-neutral configuration contract for
// TokenHub-compatible gateways. Transport and request conversion stay in the
// existing OpenAI/Anthropic adaptors until a channel is explicitly selected;
// this package deliberately does not read local credentials or log secrets.
package tokenhub

import (
	"errors"
	"net/url"
	"strings"
)

type Protocol string

const (
	ProtocolOpenAI    Protocol = "openai"
	ProtocolResponses Protocol = "responses"
	ProtocolAnthropic Protocol = "anthropic"
)

// Config is the stable boundary between MyAPI routing policy and a TokenHub
// upstream. APIKey is held by the caller and is never included in errors or
// String methods.
type Config struct {
	BaseURL  string
	Region   string
	Protocol Protocol
}

func (c Config) Validate() error {
	if _, err := NormalizeBaseURL(c.BaseURL); err != nil {
		return err
	}
	switch c.Protocol {
	case "", ProtocolOpenAI, ProtocolResponses, ProtocolAnthropic:
		return nil
	default:
		return errors.New("tokenhub: unsupported protocol")
	}
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
