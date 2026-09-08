package common

import (
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoConvOptionsUsesNormalizedGeminiSafetySettings(t *testing.T) {
	registered := config.GlobalConfig.Get("gemini")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})
	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"safety_settings": `{"HARM_CATEGORY_HATE_SPEECH":"","HARM_CATEGORY_DANGEROUS_CONTENT":"BLOCK_ONLY_HIGH"}`,
	}))

	options := (&RelayInfo{}).ConvOptions()

	assert.Equal(t, "OFF", options.Gemini.SafetySetting("HARM_CATEGORY_HATE_SPEECH"))
	assert.Equal(t, "BLOCK_ONLY_HIGH", options.Gemini.SafetySetting("HARM_CATEGORY_DANGEROUS_CONTENT"))
}

func TestRelayInfoConvOptionsFreezesGeminiCallbacks(t *testing.T) {
	registered := config.GlobalConfig.Get("gemini")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})
	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"safety_settings":          `{"default":"BLOCK_ONLY_HIGH"}`,
		"supported_imagine_models": `["first-image"]`,
	}))

	options := (&RelayInfo{}).ConvOptions()

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"safety_settings":          `{"default":"OFF"}`,
		"supported_imagine_models": `["second-image"]`,
	}))

	assert.Equal(t, "BLOCK_ONLY_HIGH", options.Gemini.SafetySetting("other"))
	assert.True(t, options.Gemini.SupportsImagine("first-image"))
	assert.False(t, options.Gemini.SupportsImagine("second-image"))

	freshOptions := (&RelayInfo{}).ConvOptions()
	assert.Equal(t, "OFF", freshOptions.Gemini.SafetySetting("other"))
	assert.False(t, freshOptions.Gemini.SupportsImagine("first-image"))
	assert.True(t, freshOptions.Gemini.SupportsImagine("second-image"))
}
