package ratio_setting_test

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/ForceMind/MyAPI/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupRatioSettingKeepsPublicThreeFieldDTOShape(t *testing.T) {
	groupRatio := types.NewRWMap[string, float64]()
	groupRatio.AddAll(map[string]float64{"default": 1})
	groupGroupRatio := types.NewRWMap[string, map[string]float64]()
	groupGroupRatio.AddAll(map[string]map[string]float64{"vip": {"default": 0.5}})
	specialUsableGroup := types.NewRWMap[string, map[string]string]()
	specialUsableGroup.AddAll(map[string]map[string]string{"vip": {"+:target": "Target"}})

	// Positional construction is intentional: it protects the public DTO from
	// gaining fields that would break existing external unkeyed literals.
	setting := ratio_setting.GroupRatioSetting{
		groupRatio,
		groupGroupRatio,
		specialUsableGroup,
	}

	encoded, err := common.Marshal(&setting)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"group_ratio":{"default":1},
		"group_group_ratio":{"vip":{"default":0.5}},
		"group_special_usable_group":{"vip":{"+:target":"Target"}}
	}`, string(encoded))

	var decoded ratio_setting.GroupRatioSetting
	require.NoError(t, common.Unmarshal(encoded, &decoded))
	ratio, ok := decoded.GroupRatio.Get("default")
	require.True(t, ok)
	assert.Equal(t, 1.0, ratio)
	nested, ok := decoded.GroupGroupRatio.Get("vip")
	require.True(t, ok)
	assert.Equal(t, 0.5, nested["default"])
	special, ok := decoded.GroupSpecialUsableGroup.Get("vip")
	require.True(t, ok)
	assert.Equal(t, "Target", special["+:target"])
}
