package operation_setting

import (
	"strconv"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func updateChannelAffinitySettingForTest(t *testing.T, values map[string]string) {
	t.Helper()
	require.NoError(t, channelAffinitySettingState.UpdateConfigMap(values))
}

func restoreChannelAffinitySettingForTest(t *testing.T) {
	original := GetChannelAffinitySetting()
	t.Cleanup(func() {
		require.NoError(t, channelAffinitySettingState.UpdateConfigMap(map[string]string{
			"max_entries":         intStringForAffinity(original.MaxEntries),
			"default_ttl_seconds": intStringForAffinity(original.DefaultTTLSeconds),
		}))
	})
}

func intStringForAffinity(value int) string {
	return strconv.Itoa(value)
}

// 保存后：getter 读到新代；旧快照不被渗透（不可变代）。
func TestChannelAffinitySettingGenerationIsImmutable(t *testing.T) {
	restoreChannelAffinitySettingForTest(t)

	before := GetChannelAffinitySetting()
	beforeMax := before.MaxEntries

	updateChannelAffinitySettingForTest(t, map[string]string{
		"max_entries": intStringForAffinity(beforeMax + 11),
	})

	assert.Equal(t, beforeMax, before.MaxEntries, "已取出的快照不得被新代渗透")
	assert.Equal(t, beforeMax+11, GetChannelAffinitySetting().MaxEntries, "保存后新请求必须读到新代")
}

// getter 返回深拷贝分离快照：修改返回值（含规则内嵌套结构）不影响运行时状态。
func TestGetChannelAffinitySettingReturnsDetachedDeepSnapshot(t *testing.T) {
	setting := GetChannelAffinitySetting()
	require.NotEmpty(t, setting.Rules)
	originalMax := setting.MaxEntries
	originalFirstModelRegex := setting.Rules[0].ModelRegex[0]

	setting.MaxEntries = -1
	setting.Rules[0].ModelRegex[0] = "^mutated$"
	setting.Rules[0].ParamOverrideTemplate["operations"] = "mutated"
	setting.Rules = append(setting.Rules, ChannelAffinityRule{Name: "mutated-rule"})

	fresh := GetChannelAffinitySetting()
	assert.Equal(t, originalMax, fresh.MaxEntries)
	assert.Equal(t, originalFirstModelRegex, fresh.Rules[0].ModelRegex[0])
	assert.IsType(t, []interface{}{}, fresh.Rules[0].ParamOverrideTemplate["operations"],
		"模板深拷贝不得被调用方修改渗透")
	assert.NotEqual(t, len(setting.Rules), len(fresh.Rules))
}

// 非法值不发布：校验/更新失败后保持原代。
func TestChannelAffinitySettingRejectsInvalidValuesWithoutPublishing(t *testing.T) {
	before := GetChannelAffinitySetting()

	err := channelAffinitySettingState.ValidateConfigMap(map[string]string{
		"max_entries": "not-a-number",
	})
	require.Error(t, err)

	err = channelAffinitySettingState.UpdateConfigMap(map[string]string{
		"max_entries": "not-a-number",
	})
	require.Error(t, err)

	after := GetChannelAffinitySetting()
	assert.Equal(t, before.MaxEntries, after.MaxEntries)
	assert.Equal(t, len(before.Rules), len(after.Rules), "校验失败不得发布部分更新的代")
}

// 规则经 JSON 配置进出：rules 键更新后深拷贝发布，导出走同一代。
func TestChannelAffinitySettingRulesRoundTrip(t *testing.T) {
	restoreChannelAffinitySettingForTest(t)

	before := GetChannelAffinitySetting()
	rulesJSON, err := config.ConfigToMap(before)
	require.NoError(t, err)
	require.Contains(t, rulesJSON, "rules")

	updateChannelAffinitySettingForTest(t, map[string]string{
		"rules": rulesJSON["rules"],
	})

	after := GetChannelAffinitySetting()
	assert.Equal(t, len(before.Rules), len(after.Rules))
	assert.Equal(t, before.Rules[0].Name, after.Rules[0].Name)
}

// 注册形态必须是 ValidatingMapConfig。
func TestChannelAffinitySettingRegisteredAsValidatingMapConfig(t *testing.T) {
	registered := config.GlobalConfig.Get("channel_affinity_setting")
	require.NotNil(t, registered)
	_, ok := registered.(config.ValidatingMapConfig)
	assert.True(t, ok, "channel_affinity_setting 必须以 ValidatingMapConfig 注册")
}
