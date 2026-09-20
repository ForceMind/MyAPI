package setting

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
)

// ChannelQuotaAlertDeliveryOptionKey ends in Secret so the generic option
// listing never returns the webhook credential or a URL containing a token.
// URL and secret are stored in one JSON value to publish one runtime generation.
const ChannelQuotaAlertDeliveryOptionKey = "ChannelQuotaAlertDeliveryConfigSecret"

const (
	maxChannelQuotaAlertWebhookURLBytes = 2048
	maxChannelQuotaAlertSecretBytes     = 1024
	minChannelQuotaAlertSecretBytes     = 16
)

type ChannelQuotaAlertDeliverySettings struct {
	WebhookURL    string `json:"webhook_url"`
	WebhookSecret string `json:"webhook_secret"`
}

func ParseChannelQuotaAlertDeliverySettings(raw string) (ChannelQuotaAlertDeliverySettings, error) {
	var settings ChannelQuotaAlertDeliverySettings
	if err := common.DecodeJsonStrict(strings.NewReader(raw), &settings); err != nil {
		return ChannelQuotaAlertDeliverySettings{}, fmt.Errorf("invalid channel quota alert delivery settings: %w", err)
	}
	settings.WebhookURL = strings.TrimSpace(settings.WebhookURL)
	if err := ValidateChannelQuotaAlertDeliverySettings(settings); err != nil {
		return ChannelQuotaAlertDeliverySettings{}, err
	}
	return settings, nil
}

func MarshalChannelQuotaAlertDeliverySettings(settings ChannelQuotaAlertDeliverySettings) (string, error) {
	settings.WebhookURL = strings.TrimSpace(settings.WebhookURL)
	if err := ValidateChannelQuotaAlertDeliverySettings(settings); err != nil {
		return "", err
	}
	data, err := common.Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("marshal channel quota alert delivery settings: %w", err)
	}
	return string(data), nil
}

// ValidateChannelQuotaAlertDeliverySettings is intentionally stricter than
// the general fetch setting: quota alerts always require HTTPS, a signing
// secret, and a public network target. DNS is checked again immediately before
// dialing by the delivery client to prevent rebinding.
func ValidateChannelQuotaAlertDeliverySettings(settings ChannelQuotaAlertDeliverySettings) error {
	webhookURL := strings.TrimSpace(settings.WebhookURL)
	if webhookURL == "" && settings.WebhookSecret == "" {
		return nil
	}
	if webhookURL == "" || settings.WebhookSecret == "" {
		return errors.New("channel quota alert webhook URL and secret must be configured together")
	}
	if len(webhookURL) > maxChannelQuotaAlertWebhookURLBytes {
		return errors.New("channel quota alert webhook URL is too long")
	}
	if len(settings.WebhookSecret) < minChannelQuotaAlertSecretBytes || len(settings.WebhookSecret) > maxChannelQuotaAlertSecretBytes {
		return errors.New("channel quota alert webhook secret must contain between 16 and 1024 bytes")
	}
	parsed, err := url.ParseRequestURI(webhookURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("channel quota alert webhook URL is invalid")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return errors.New("channel quota alert webhook URL must use HTTPS")
	}
	if parsed.User != nil {
		return errors.New("channel quota alert webhook URL must not contain user information")
	}
	if parsed.Fragment != "" {
		return errors.New("channel quota alert webhook URL must not contain a fragment")
	}
	host := parsed.Hostname()
	if host == "" {
		return errors.New("channel quota alert webhook URL has no host")
	}
	port := 443
	if parsed.Port() != "" {
		port, err = strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			return errors.New("channel quota alert webhook URL has an invalid port")
		}
	}
	protection := &common.SSRFProtection{
		AllowPrivateIp:   false,
		DomainFilterMode: false,
		IpFilterMode:     false,
	}
	if err := protection.ValidateNetworkTarget(host, port); err != nil {
		return fmt.Errorf("channel quota alert webhook target is not allowed: %w", err)
	}
	return nil
}

func GetChannelQuotaAlertDeliverySettings() (ChannelQuotaAlertDeliverySettings, error) {
	common.OptionMapRWMutex.Lock()
	raw := ""
	if common.OptionMap != nil {
		raw = common.OptionMap[ChannelQuotaAlertDeliveryOptionKey]
	}
	// Compatibility with the earlier two-key WIP. New writes always use the
	// atomic hidden option above.
	legacyURL := common.ChannelQuotaAlertWebhookURL
	legacySecret := common.ChannelQuotaAlertWebhookSecret
	common.OptionMapRWMutex.Unlock()
	if strings.TrimSpace(raw) != "" {
		return ParseChannelQuotaAlertDeliverySettings(raw)
	}
	settings := ChannelQuotaAlertDeliverySettings{
		WebhookURL:    strings.TrimSpace(legacyURL),
		WebhookSecret: legacySecret,
	}
	if settings.WebhookURL == "" && settings.WebhookSecret == "" {
		return settings, nil
	}
	if err := ValidateChannelQuotaAlertDeliverySettings(settings); err != nil {
		return ChannelQuotaAlertDeliverySettings{}, err
	}
	return settings, nil
}

func (settings ChannelQuotaAlertDeliverySettings) Configured() bool {
	return strings.TrimSpace(settings.WebhookURL) != "" && settings.WebhookSecret != ""
}

func (settings ChannelQuotaAlertDeliverySettings) EndpointHost() string {
	parsed, err := url.Parse(strings.TrimSpace(settings.WebhookURL))
	if err != nil {
		return ""
	}
	return net.JoinHostPort(parsed.Hostname(), effectiveChannelQuotaAlertPort(parsed))
}

func effectiveChannelQuotaAlertPort(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	if parsed.Port() != "" {
		return parsed.Port()
	}
	return "443"
}
