package operation_setting

import (
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedGeneralSettingPreservesConfigContract(t *testing.T) {
	state := newManagedGeneralSetting(defaultGeneralSetting)

	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"docs_link":                     "https://github.com/ForceMind/MyAPI#readme",
		"ping_interval_enabled":         "false",
		"ping_interval_seconds":         "60",
		"quota_display_type":            QuotaDisplayTypeUSD,
		"custom_currency_symbol":        "¤",
		"custom_currency_exchange_rate": "1",
	}, exported)

	before := state.current.Load()
	require.NoError(t, state.ValidateConfigMap(map[string]string{
		"docs_link":                     "https://docs.example.test",
		"ping_interval_enabled":         "true",
		"ping_interval_seconds":         "30.000000",
		"quota_display_type":            QuotaDisplayTypeCustom,
		"custom_currency_symbol":        "C",
		"custom_currency_exchange_rate": "2.5",
	}))
	assert.Same(t, before, state.current.Load())

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"docs_link":              "https://docs.example.test",
		"ping_interval_seconds":  "30.000000",
		"quota_display_type":     QuotaDisplayTypeCustom,
		"custom_currency_symbol": "C",
	}))
	assert.Equal(t, GeneralSetting{
		DocsLink:                   "https://docs.example.test",
		PingIntervalEnabled:        false,
		PingIntervalSeconds:        30,
		QuotaDisplayType:           QuotaDisplayTypeCustom,
		CustomCurrencySymbol:       "C",
		CustomCurrencyExchangeRate: 1,
	}, state.snapshot())
}

func TestManagedGeneralSettingRejectsInvalidScalarsWithoutPublishing(t *testing.T) {
	state := newManagedGeneralSetting(defaultGeneralSetting)
	before := state.current.Load()

	for _, update := range []map[string]string{
		{"ping_interval_enabled": "not-a-bool"},
		{"ping_interval_seconds": "1.5"},
		{"custom_currency_exchange_rate": "NaN"},
		{"custom_currency_exchange_rate": "+Inf"},
	} {
		require.Error(t, state.ValidateConfigMap(update))
		assert.Same(t, before, state.current.Load())
		require.Error(t, state.UpdateConfigMap(update))
		assert.Same(t, before, state.current.Load())
	}
}

func TestGetGeneralSettingReturnsDetachedSnapshot(t *testing.T) {
	registered := config.GlobalConfig.Get("general_setting")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"docs_link":          "https://snapshot.example.test",
		"quota_display_type": QuotaDisplayTypeCustom,
	}))
	detached := GetGeneralSetting()
	detached.DocsLink = "mutated-by-caller"
	detached.QuotaDisplayType = QuotaDisplayTypeTokens

	current := GetGeneralSetting()
	assert.Equal(t, "https://snapshot.example.test", current.DocsLink)
	assert.Equal(t, QuotaDisplayTypeCustom, current.QuotaDisplayType)
}

func TestManagedGeneralSettingPublishesWholeGeneration(t *testing.T) {
	state := newManagedGeneralSetting(GeneralSetting{
		DocsLink:                   "one",
		PingIntervalEnabled:        false,
		PingIntervalSeconds:        1,
		QuotaDisplayType:           QuotaDisplayTypeUSD,
		CustomCurrencySymbol:       "A",
		CustomCurrencyExchangeRate: 1,
	})
	first := GeneralSetting{
		DocsLink:                   "one",
		PingIntervalEnabled:        false,
		PingIntervalSeconds:        1,
		QuotaDisplayType:           QuotaDisplayTypeUSD,
		CustomCurrencySymbol:       "A",
		CustomCurrencyExchangeRate: 1,
	}
	second := GeneralSetting{
		DocsLink:                   "two",
		PingIntervalEnabled:        true,
		PingIntervalSeconds:        2,
		QuotaDisplayType:           QuotaDisplayTypeCustom,
		CustomCurrencySymbol:       "B",
		CustomCurrencyExchangeRate: 2,
	}
	updates := []map[string]string{
		{
			"docs_link":                     first.DocsLink,
			"ping_interval_enabled":         "false",
			"ping_interval_seconds":         "1",
			"quota_display_type":            first.QuotaDisplayType,
			"custom_currency_symbol":        first.CustomCurrencySymbol,
			"custom_currency_exchange_rate": "1",
		},
		{
			"docs_link":                     second.DocsLink,
			"ping_interval_enabled":         "true",
			"ping_interval_seconds":         "2",
			"quota_display_type":            second.QuotaDisplayType,
			"custom_currency_symbol":        second.CustomCurrencySymbol,
			"custom_currency_exchange_rate": "2",
		},
	}

	var writers sync.WaitGroup
	writerErr := make(chan error, 1)
	started := make(chan struct{})
	writers.Add(1)
	go func() {
		defer writers.Done()
		for i := 0; i < 100; i++ {
			if err := state.UpdateConfigMap(updates[i%len(updates)]); err != nil {
				writerErr <- err
				return
			}
			if i == 0 {
				close(started)
			}
		}
	}()
	<-started
	for i := 0; i < 1000; i++ {
		snapshot := state.snapshot()
		assert.True(t, snapshot == first || snapshot == second, "observed mixed generation: %#v", snapshot)
	}
	writers.Wait()
	select {
	case err := <-writerErr:
		require.NoError(t, err)
	default:
	}
}
