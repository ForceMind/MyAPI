package setting

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/require"
)

func TestChannelQuotaAlertDeliverySettingsRequireSignedPublicHTTPS(t *testing.T) {
	valid := ChannelQuotaAlertDeliverySettings{
		WebhookURL:    "https://alerts.example.com/my-api",
		WebhookSecret: "0123456789abcdef",
	}
	require.NoError(t, ValidateChannelQuotaAlertDeliverySettings(valid))

	for _, test := range []struct {
		name     string
		settings ChannelQuotaAlertDeliverySettings
	}{
		{name: "http", settings: ChannelQuotaAlertDeliverySettings{WebhookURL: "http://alerts.example.com/hook", WebhookSecret: valid.WebhookSecret}},
		{name: "loopback", settings: ChannelQuotaAlertDeliverySettings{WebhookURL: "https://127.0.0.1/hook", WebhookSecret: valid.WebhookSecret}},
		{name: "private ipv4", settings: ChannelQuotaAlertDeliverySettings{WebhookURL: "https://10.0.0.1/hook", WebhookSecret: valid.WebhookSecret}},
		{name: "private ipv6", settings: ChannelQuotaAlertDeliverySettings{WebhookURL: "https://[::1]/hook", WebhookSecret: valid.WebhookSecret}},
		{name: "userinfo", settings: ChannelQuotaAlertDeliverySettings{WebhookURL: "https://token@alerts.example.com/hook", WebhookSecret: valid.WebhookSecret}},
		{name: "missing secret", settings: ChannelQuotaAlertDeliverySettings{WebhookURL: valid.WebhookURL}},
		{name: "short secret", settings: ChannelQuotaAlertDeliverySettings{WebhookURL: valid.WebhookURL, WebhookSecret: "too-short"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, ValidateChannelQuotaAlertDeliverySettings(test.settings))
		})
	}
}

func TestChannelQuotaAlertDeliverySettingsRoundTripAndHiddenOptionRead(t *testing.T) {
	previousMap := common.OptionMap
	settings := ChannelQuotaAlertDeliverySettings{
		WebhookURL:    "https://alerts.example.com:8443/hooks/quota?tenant=one",
		WebhookSecret: "0123456789abcdef",
	}
	raw, err := MarshalChannelQuotaAlertDeliverySettings(settings)
	require.NoError(t, err)
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{ChannelQuotaAlertDeliveryOptionKey: raw}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousMap
		common.OptionMapRWMutex.Unlock()
	})

	loaded, err := GetChannelQuotaAlertDeliverySettings()
	require.NoError(t, err)
	require.Equal(t, settings, loaded)
	require.True(t, loaded.Configured())
	require.Equal(t, "alerts.example.com:8443", loaded.EndpointHost())
}

func TestChannelQuotaAlertDeliverySettingsRejectUnknownJSONField(t *testing.T) {
	_, err := ParseChannelQuotaAlertDeliverySettings(`{"webhook_url":"https://alerts.example.com/hook","webhook_secret":"0123456789abcdef","allow_private":true}`)
	require.Error(t, err)
}
