package operation_setting

import (
	"math"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preserveToolPrices(t *testing.T) {
	t.Helper()
	state := toolPriceSetting.current.Load()
	original := make(map[string]float64, len(state.overrides))
	for key, price := range state.overrides {
		original[key] = price
	}
	t.Cleanup(func() {
		toolPriceSetting.replaceOverrides(original)
	})
}

func replaceToolPricesForTest(prices map[string]float64) {
	toolPriceSetting.replaceOverrides(prices)
}

func toolPriceOverridesForTest() map[string]float64 {
	state := toolPriceSetting.current.Load()
	result := make(map[string]float64, len(state.overrides))
	for key, price := range state.overrides {
		result[key] = price
	}
	return result
}

func TestToolPriceHardcodedFallbacksSurviveMissingOperatorConfig(t *testing.T) {
	preserveToolPrices(t)
	replaceToolPricesForTest(map[string]float64{})

	expectedDefaults := map[string]float64{
		"web_search":         10,
		"web_search_preview": 10,
		"file_search":        2.5,
		"google_search":      14,
		"image_generation":   150,
	}
	for name, expected := range expectedDefaults {
		assert.Equal(t, expected, GetToolPrice(name), name)
	}
	assert.Equal(t, 25.0, GetToolPriceForModel("web_search_preview", "gpt-4o-2024-11-20"))
	assert.Equal(t, 25.0, GetToolPriceForModel("web_search_preview", "gpt-4.1-mini"))
}

func TestToolPriceOperatorOverridePrecedenceAndExplicitZero(t *testing.T) {
	preserveToolPrices(t)
	replaceToolPricesForTest(map[string]float64{
		"image_generation":                 0,
		"web_search":                       12,
		"web_search_preview":               0,
		"web_search_preview:gpt-4o*":       30,
		"web_search_preview:gpt-4o-mini*":  0,
		"web_search_preview:custom-model*": 7,
	})

	assert.Equal(t, 0.0, GetToolPrice("image_generation"))
	assert.Equal(t, 12.0, GetToolPrice("web_search"))
	assert.Equal(t, 0.0, GetToolPriceForModel("web_search_preview", "o1"))
	assert.Equal(t, 30.0, GetToolPriceForModel("web_search_preview", "gpt-4o"))
	assert.Equal(t, 0.0, GetToolPriceForModel("web_search_preview", "gpt-4o-mini"))
	assert.Equal(t, 25.0, GetToolPriceForModel("web_search_preview", "gpt-4.1"))
	assert.Equal(t, 7.0, GetToolPriceForModel("web_search_preview", "custom-model-v2"))

	DeleteToolPriceForTest("web_search_preview:gpt-4o*")
	assert.Equal(t, 25.0, GetToolPriceForModel("web_search_preview", "gpt-4o"))

	DeleteToolPriceForTest("web_search")
	assert.Equal(t, 10.0, GetToolPrice("web_search"))
}

func TestToolPriceCustomFunctionHasNoHardcodedFallback(t *testing.T) {
	preserveToolPrices(t)
	replaceToolPricesForTest(map[string]float64{})

	assert.Equal(t, 0.0, GetToolPrice("lookup_customer"))

	SetToolPriceForTest("lookup_customer", 5)
	assert.Equal(t, 5.0, GetToolPrice("lookup_customer"))

	SetToolPriceForTest("lookup_customer", 0)
	assert.Equal(t, 0.0, GetToolPrice("lookup_customer"))
}

func TestValidateToolPricesJSON(t *testing.T) {
	valid := []string{
		`{}`,
		`{"web_search":0}`,
		`{"web_search":10,"custom_fn":2.5}`,
	}
	for _, value := range valid {
		assert.NoError(t, ValidateToolPricesJSON(value), value)
	}

	invalid := []string{
		`null`,
		`[]`,
		`{"web_search":null}`,
		`{"web_search":true}`,
		`{"web_search":"0"}`,
		`{"web_search":-1}`,
		`{"web_search":1e999}`,
		`{"web_search":`,
	}
	for _, value := range invalid {
		assert.Error(t, ValidateToolPricesJSON(value), value)
	}
}

func TestToolPriceSettingPublicDTOJSONCompatibility(t *testing.T) {
	value := ToolPriceSetting{Prices: map[string]float64{
		"web_search": 0,
		"custom_fn":  2.5,
	}}
	raw, err := common.Marshal(value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prices":{"custom_fn":2.5,"web_search":0}}`, string(raw))

	var decoded ToolPriceSetting
	require.NoError(t, common.Unmarshal(raw, &decoded))
	assert.Equal(t, value.Prices, decoded.Prices)
}

func TestLoadToolPricesFromJSONStringReplacesMapAndKeepsValidSiblings(t *testing.T) {
	preserveToolPrices(t)

	LoadToolPricesFromJSONString(`{
		"web_search": 0,
		"custom_fn": 3,
		"file_search": null,
		"google_search": -1,
		"image_generation": "0"
	}`)

	overrides := toolPriceOverridesForTest()
	require.Len(t, overrides, 2)
	assert.Equal(t, 0.0, overrides["web_search"])
	assert.Equal(t, 3.0, overrides["custom_fn"])
	assert.Equal(t, 0.0, GetToolPrice("web_search"))
	assert.Equal(t, 3.0, GetToolPrice("custom_fn"))
	assert.Equal(t, 2.5, GetToolPrice("file_search"))
	assert.Equal(t, 14.0, GetToolPrice("google_search"))
	assert.Equal(t, 150.0, GetToolPrice("image_generation"))

	LoadToolPricesFromJSONString(`{"image_generation":0}`)
	overrides = toolPriceOverridesForTest()
	require.Len(t, overrides, 1)
	assert.NotContains(t, overrides, "web_search")
	assert.NotContains(t, overrides, "custom_fn")
	assert.Equal(t, 10.0, GetToolPrice("web_search"))
	assert.Equal(t, 0.0, GetToolPrice("custom_fn"))
	assert.Equal(t, 0.0, GetToolPrice("image_generation"))
}

func TestRebuildToolPriceIndexIgnoresInvalidDirectValues(t *testing.T) {
	preserveToolPrices(t)
	replaceToolPricesForTest(map[string]float64{
		"web_search":       -1,
		"file_search":      math.Inf(1),
		"image_generation": math.NaN(),
		"custom_fn":        math.NaN(),
	})

	assert.Equal(t, 10.0, GetToolPrice("web_search"))
	assert.Equal(t, 2.5, GetToolPrice("file_search"))
	assert.Equal(t, 150.0, GetToolPrice("image_generation"))
	assert.Equal(t, 0.0, GetToolPrice("custom_fn"))
}

func TestToolPriceConfigManagerLoadAndExportUseSamePublishedOverrides(t *testing.T) {
	preserveToolPrices(t)

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		ToolPriceOptionKey: `{"web_search":0,"custom_fn":7}`,
	}))

	assert.Equal(t, 0.0, GetToolPrice("web_search"))
	assert.Equal(t, 7.0, GetToolPrice("custom_fn"))

	var exported map[string]float64
	require.NoError(t, common.UnmarshalJsonStr(config.GlobalConfig.ExportAllConfigs()[ToolPriceOptionKey], &exported))
	assert.Equal(t, map[string]float64{"web_search": 0, "custom_fn": 7}, exported)
}

func TestToolPriceDirectConfigUpdateAndSaveUsePublishedOverrides(t *testing.T) {
	preserveToolPrices(t)
	cfg := config.GlobalConfig.Get("tool_price_setting")

	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		"prices": `{"web_search_preview:gpt-4o-mini*":0,"custom_fn":9}`,
	}))
	assert.Equal(t, 0.0, GetToolPriceForModel("web_search_preview", "gpt-4o-mini-2026"))
	assert.Equal(t, 9.0, GetToolPrice("custom_fn"))

	var savedRaw string
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if key == ToolPriceOptionKey {
			savedRaw = value
		}
		return nil
	}))
	var saved map[string]float64
	require.NoError(t, common.UnmarshalJsonStr(savedRaw, &saved))
	assert.Equal(t, map[string]float64{
		"web_search_preview:gpt-4o-mini*": 0,
		"custom_fn":                       9,
	}, saved)
}

func TestToolPriceMapConfigRejectsInvalidUpdatesWithoutPublishing(t *testing.T) {
	setting := newManagedToolPriceSetting()
	require.NoError(t, config.UpdateConfigFromMap(setting, map[string]string{
		"prices": `{"web_search":0,"custom_fn":9}`,
	}))

	beforeState := setting.current.Load()
	beforeExport, err := setting.ExportConfigMap()
	require.NoError(t, err)

	invalid := []string{
		`{"custom_fn":`,
		`null`,
		`{"custom_fn":-1}`,
		`{"custom_fn":1e999}`,
	}
	for _, raw := range invalid {
		t.Run(raw, func(t *testing.T) {
			err := config.UpdateConfigFromMap(setting, map[string]string{"prices": raw})
			require.Error(t, err)

			afterState := setting.current.Load()
			assert.Same(t, beforeState, afterState)
			assert.Equal(t, 0.0, setting.getPriceForModel("web_search", ""))
			assert.Equal(t, 9.0, setting.getPriceForModel("custom_fn", ""))

			afterExport, exportErr := setting.ExportConfigMap()
			require.NoError(t, exportErr)
			assert.Equal(t, beforeExport, afterExport)
		})
	}
}

func TestToolPriceConcurrentPublicationKeepsOverridesAndIndexPaired(t *testing.T) {
	setting := newManagedToolPriceSetting()
	start := make(chan struct{})
	errs := make(chan string, 1)
	var workers sync.WaitGroup

	report := func(message string) {
		select {
		case errs <- message:
		default:
		}
	}

	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 1000; i++ {
			raw := `{"custom_fn":1}`
			if i%2 == 1 {
				raw = `{"custom_fn":2}`
			}
			if err := config.UpdateConfigFromMap(setting, map[string]string{"prices": raw}); err != nil {
				report(err.Error())
				return
			}
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 1000; i++ {
			state := setting.current.Load()
			override := state.overrides["custom_fn"]
			indexed := state.index.defaults["custom_fn"]
			if override != indexed {
				report("published overrides and lookup index came from different generations")
				return
			}

			exported, err := setting.ExportConfigMap()
			if err != nil {
				report(err.Error())
				return
			}
			var prices map[string]float64
			if err := common.UnmarshalJsonStr(exported["prices"], &prices); err != nil {
				report(err.Error())
				return
			}
			price := prices["custom_fn"]
			if price != 0 && price != 1 && price != 2 {
				report("export observed an invalid generation")
				return
			}
			livePrice := setting.getPriceForModel("custom_fn", "")
			if livePrice != 0 && livePrice != 1 && livePrice != 2 {
				report("lookup observed an invalid generation")
				return
			}
		}
	}()

	close(start)
	workers.Wait()
	select {
	case message := <-errs:
		require.Fail(t, message)
	default:
	}
}
