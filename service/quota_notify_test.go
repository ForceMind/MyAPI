package service

import (
	"testing"

	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionQuotaNotificationPreservesWideRemaining(t *testing.T) {
	registered := config.GlobalConfig.Get("general_setting")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})
	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"quota_display_type": operation_setting.QuotaDisplayTypeTokens,
	}))

	const (
		wideRemaining = int64(-4_294_967_313)
		prompt        = "您的订阅额度即将用尽"
		topUpLink     = "https://notification.example.invalid/wallet"
	)
	for _, testCase := range []struct {
		name       string
		notifyType string
		content    string
		values     []interface{}
	}{
		{
			name: "bark", notifyType: dto.NotifyTypeBark,
			content: "{{value}}，剩余额度：{{value}}，请及时充值",
			values:  []interface{}{prompt, "-4294967313"},
		},
		{
			name: "gotify", notifyType: dto.NotifyTypeGotify,
			content: "{{value}}，当前剩余额度为 {{value}}，请及时充值。",
			values:  []interface{}{prompt, "-4294967313"},
		},
		{
			name: "default", notifyType: "",
			content: "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>",
			values:  []interface{}{prompt, "-4294967313", topUpLink, topUpLink},
		},
		{
			name: "webhook", notifyType: dto.NotifyTypeWebhook,
			content: "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>",
			values:  []interface{}{prompt, "-4294967313", topUpLink, topUpLink},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			notification := buildSubscriptionQuotaNotification(testCase.notifyType, wideRemaining, topUpLink)
			require.Equal(t, dto.Notify{
				Type: dto.NotifyTypeQuotaExceed, Title: prompt,
				Content: testCase.content, Values: testCase.values,
			}, notification)
		})
	}

	ordinary := buildSubscriptionQuotaNotification(dto.NotifyTypeEmail, 125, topUpLink)
	require.Equal(t, []interface{}{prompt, "125", topUpLink, topUpLink}, ordinary.Values)
}
