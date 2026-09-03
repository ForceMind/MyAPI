package ratio_setting

import (
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRatioMapJSON(t *testing.T) {
	require.NoError(t, ValidateRatioMapJSON(`{"model":1.25,"free":0}`))
	require.Error(t, ValidateRatioMapJSON(`{"model":"not-a-number"}`))
	require.Error(t, ValidateRatioMapJSON(`{"model":1,"partially_wrong":"not-a-number"}`))
	require.Error(t, ValidateRatioMapJSON(`{"model":-0.1}`))
	require.Error(t, ValidateRatioMapJSON(`{"model":null}`))
	require.Error(t, ValidateRatioMapJSON(`null`))
}

func TestValidateNestedRatioMapJSON(t *testing.T) {
	require.NoError(t, ValidateNestedRatioMapJSON(`{"vip":{"default":0.9}}`))
	require.Error(t, ValidateNestedRatioMapJSON(`{"vip":{"default":"not-a-number"}}`))
	require.Error(t, ValidateNestedRatioMapJSON(`{"vip":1}`))
	require.Error(t, ValidateNestedRatioMapJSON(`{"vip":{"default":-0.1}}`))
	require.Error(t, ValidateNestedRatioMapJSON(`{"vip":null}`))
	require.Error(t, ValidateNestedRatioMapJSON(`null`))
}

func TestRatioJSONSettersRejectInvalidValuesWithoutChangingLiveMaps(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		get       func() string
		update    func(string) error
		baseline  string
		wrongType string
		negative  string
	}{
		{name: "model price", get: ModelPrice2JSONString, update: UpdateModelPriceByJSONString, baseline: `{"old":1}`, wrongType: `{"new":2,"wrong":"x"}`, negative: `{"new":-0.1}`},
		{name: "model ratio", get: ModelRatio2JSONString, update: UpdateModelRatioByJSONString, baseline: `{"old":1}`, wrongType: `{"new":2,"wrong":"x"}`, negative: `{"new":-0.1}`},
		{name: "completion ratio", get: CompletionRatio2JSONString, update: UpdateCompletionRatioByJSONString, baseline: `{"old":1}`, wrongType: `{"new":2,"wrong":"x"}`, negative: `{"new":-0.1}`},
		{name: "cache ratio", get: CacheRatio2JSONString, update: UpdateCacheRatioByJSONString, baseline: `{"old":1}`, wrongType: `{"new":2,"wrong":"x"}`, negative: `{"new":-0.1}`},
		{name: "create cache ratio", get: CreateCacheRatio2JSONString, update: UpdateCreateCacheRatioByJSONString, baseline: `{"old":1}`, wrongType: `{"new":2,"wrong":"x"}`, negative: `{"new":-0.1}`},
		{name: "image ratio", get: ImageRatio2JSONString, update: UpdateImageRatioByJSONString, baseline: `{"old":1}`, wrongType: `{"new":2,"wrong":"x"}`, negative: `{"new":-0.1}`},
		{name: "audio ratio", get: AudioRatio2JSONString, update: UpdateAudioRatioByJSONString, baseline: `{"old":1}`, wrongType: `{"new":2,"wrong":"x"}`, negative: `{"new":-0.1}`},
		{name: "audio completion ratio", get: AudioCompletionRatio2JSONString, update: UpdateAudioCompletionRatioByJSONString, baseline: `{"old":1}`, wrongType: `{"new":2,"wrong":"x"}`, negative: `{"new":-0.1}`},
		{name: "group ratio", get: GroupRatio2JSONString, update: UpdateGroupRatioByJSONString, baseline: `{"old":1}`, wrongType: `{"new":2,"wrong":"x"}`, negative: `{"new":-0.1}`},
		{name: "special group ratio", get: GroupGroupRatio2JSONString, update: UpdateGroupGroupRatioByJSONString, baseline: `{"vip":{"old":1}}`, wrongType: `{"vip":{"new":2,"wrong":"x"}}`, negative: `{"vip":{"new":-0.1}}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			previous := testCase.get()
			t.Cleanup(func() {
				require.NoError(t, testCase.update(previous))
			})

			require.NoError(t, testCase.update(testCase.baseline))
			require.Error(t, testCase.update(testCase.wrongType))
			assert.JSONEq(t, testCase.baseline, testCase.get())
			require.Error(t, testCase.update(testCase.negative))
			assert.JSONEq(t, testCase.baseline, testCase.get())
		})
	}
}

func TestGroupRatioConfigLoadRejectsNegativeValuesWithoutChangingLiveMaps(t *testing.T) {
	previousGroup := GroupRatio2JSONString()
	previousSpecial := GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupRatioByJSONString(previousGroup))
		require.NoError(t, UpdateGroupGroupRatioByJSONString(previousSpecial))
	})

	const baselineGroup = `{"default":1}`
	const baselineSpecial = `{"vip":{"default":0.9}}`
	require.NoError(t, UpdateGroupRatioByJSONString(baselineGroup))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(baselineSpecial))

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.group_ratio":       `{"default":-0.1}`,
		"group_ratio_setting.group_group_ratio": `{"vip":{"default":-0.1}}`,
	}))
	assert.JSONEq(t, baselineGroup, GroupRatio2JSONString())
	assert.JSONEq(t, baselineSpecial, GroupGroupRatio2JSONString())
}

func TestGroupRatioReadsFailClosedForCorruptedInMemoryValues(t *testing.T) {
	previousGroup := GroupRatio2JSONString()
	previousSpecial := GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupRatioByJSONString(previousGroup))
		require.NoError(t, UpdateGroupGroupRatioByJSONString(previousSpecial))
	})

	groupRatioMap.Set("corrupted", -1)
	assert.Equal(t, 1.0, GetGroupRatio("corrupted"))
	groupGroupRatioMap.Set("vip", map[string]float64{"corrupted": -1})
	_, ok := GetGroupGroupRatio("vip", "corrupted")
	assert.False(t, ok)
}
