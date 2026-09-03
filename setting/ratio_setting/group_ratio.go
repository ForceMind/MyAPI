package ratio_setting

import (
	"fmt"
	"math"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/types"
)

var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var groupRatioMap = types.NewRWMap[string, float64]()

var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9,
	},
}

var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()

var defaultGroupSpecialUsableGroup = map[string]map[string]string{}

type GroupRatioSetting struct {
	GroupRatio              *types.RWMap[string, float64]            `json:"group_ratio"`
	GroupGroupRatio         *types.RWMap[string, map[string]float64] `json:"group_group_ratio"`
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string]  `json:"group_special_usable_group"`
}

var groupRatioSetting GroupRatioSetting

func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioMap.AddAll(defaultGroupRatio)
	groupGroupRatioMap.AddAll(defaultGroupGroupRatio)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
		GroupRatio:              groupRatioMap,
		GroupGroupRatio:         groupGroupRatioMap,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

func GetGroupRatioSetting() *GroupRatioSetting {
	return &groupRatioSetting
}

func (s *GroupRatioSetting) ExportConfigMap() (map[string]string, error) {
	specialGroups := "{}"
	if s != nil && s.GroupSpecialUsableGroup != nil {
		specialGroups = s.GroupSpecialUsableGroup.MarshalJSONString()
	}
	return map[string]string{
		"group_ratio":                GroupRatio2JSONString(),
		"group_group_ratio":          GroupGroupRatio2JSONString(),
		"group_special_usable_group": specialGroups,
	}, nil
}

func (s *GroupRatioSetting) UpdateConfigMap(values map[string]string) error {
	if raw, ok := values["group_ratio"]; ok {
		if err := ValidateRatioMapJSON(raw); err != nil {
			return err
		}
	}
	if raw, ok := values["group_group_ratio"]; ok {
		if err := ValidateNestedRatioMapJSON(raw); err != nil {
			return err
		}
	}
	if raw, ok := values["group_special_usable_group"]; ok {
		if err := ValidateGroupSpecialUsableGroupJSON(raw); err != nil {
			return err
		}
	}

	if raw, ok := values["group_ratio"]; ok {
		if err := UpdateGroupRatioByJSONString(raw); err != nil {
			return err
		}
	}
	if raw, ok := values["group_group_ratio"]; ok {
		if err := UpdateGroupGroupRatioByJSONString(raw); err != nil {
			return err
		}
	}
	if raw, ok := values["group_special_usable_group"]; ok {
		if s.GroupSpecialUsableGroup == nil {
			s.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		}
		if err := types.LoadFromJsonString(s.GroupSpecialUsableGroup, raw); err != nil {
			return err
		}
	}
	return nil
}

func ValidateGroupSpecialUsableGroupJSON(raw string) error {
	var groups map[string]map[string]string
	if err := common.Unmarshal([]byte(raw), &groups); err != nil {
		return err
	}
	if groups == nil {
		return fmt.Errorf("group special usable groups must be a JSON object")
	}
	for name, group := range groups {
		if group == nil {
			return fmt.Errorf("group special usable group %s must be a JSON object", name)
		}
	}
	return nil
}

func GetGroupRatioCopy() map[string]float64 {
	return groupRatioMap.ReadAll()
}

func ContainsGroupRatio(name string) bool {
	_, ok := groupRatioMap.Get(name)
	return ok
}

func GroupRatio2JSONString() string {
	return groupRatioMap.MarshalJSONString()
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	if err := ValidateRatioMapJSON(jsonStr); err != nil {
		return err
	}
	return types.LoadFromJsonString(groupRatioMap, jsonStr)
}

func GetGroupRatio(name string) float64 {
	ratio, ok := groupRatioMap.Get(name)
	if !ok {
		common.SysLog("group ratio not found: " + name)
		return 1
	}
	if ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		common.SysError("invalid group ratio for " + name)
		return 1
	}
	return ratio
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	gp, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return -1, false
	}
	ratio, ok := gp[usingGroup]
	if !ok {
		return -1, false
	}
	if ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		common.SysError("invalid special group ratio for " + userGroup + "/" + usingGroup)
		return -1, false
	}
	return ratio, true
}

func GroupGroupRatio2JSONString() string {
	return groupGroupRatioMap.MarshalJSONString()
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	if err := ValidateNestedRatioMapJSON(jsonStr); err != nil {
		return err
	}
	return types.LoadFromJsonString(groupGroupRatioMap, jsonStr)
}

func CheckGroupRatio(jsonStr string) error {
	return ValidateRatioMapJSON(jsonStr)
}
