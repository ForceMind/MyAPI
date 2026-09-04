package ratio_setting

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/types"
)

var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9,
	},
}

var defaultGroupSpecialUsableGroup = map[string]map[string]string{}

// GroupRatioSetting is a compatibility DTO. GetGroupRatioSetting returns a
// detached copy, so callers may mutate these maps without changing live state.
type GroupRatioSetting struct {
	GroupRatio              *types.RWMap[string, float64]            `json:"group_ratio"`
	GroupGroupRatio         *types.RWMap[string, map[string]float64] `json:"group_group_ratio"`
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string]  `json:"group_special_usable_group"`
}

// groupRatioSnapshot is immutable after publication.
type groupRatioSnapshot struct {
	groupRatio              map[string]float64
	groupGroupRatio         map[string]map[string]float64
	groupSpecialUsableGroup map[string]map[string]string
}

type managedGroupRatioSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[groupRatioSnapshot]
}

type groupRatioUpdate struct {
	groupRatio              map[string]float64
	groupRatioSet           bool
	groupGroupRatio         map[string]map[string]float64
	groupGroupRatioSet      bool
	groupSpecialUsableGroup map[string]map[string]string
	groupSpecialSet         bool
}

func newManagedGroupRatioSetting(
	groupRatio map[string]float64,
	groupGroupRatio map[string]map[string]float64,
	groupSpecialUsableGroup map[string]map[string]string,
) *managedGroupRatioSetting {
	setting := &managedGroupRatioSetting{}
	setting.current.Store(&groupRatioSnapshot{
		groupRatio:              cloneFlatMap(groupRatio),
		groupGroupRatio:         cloneNestedMap(groupGroupRatio),
		groupSpecialUsableGroup: cloneNestedMap(groupSpecialUsableGroup),
	})
	return setting
}

var groupRatioSetting = newManagedGroupRatioSetting(
	defaultGroupRatio,
	defaultGroupGroupRatio,
	defaultGroupSpecialUsableGroup,
)

var _ config.ValidatingMapConfig = (*managedGroupRatioSetting)(nil)
var _ config.ValidatingMapConfig = (*GroupRatioSetting)(nil)

func init() {
	config.GlobalConfig.Register("group_ratio_setting", groupRatioSetting)
}

func cloneFlatMap[V any](source map[string]V) map[string]V {
	clone := make(map[string]V, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func cloneNestedMap[V any](source map[string]map[string]V) map[string]map[string]V {
	clone := make(map[string]map[string]V, len(source))
	for outerKey, nested := range source {
		clone[outerKey] = cloneFlatMap(nested)
	}
	return clone
}

func (s *managedGroupRatioSetting) snapshot() *groupRatioSnapshot {
	if snapshot := s.current.Load(); snapshot != nil {
		return snapshot
	}
	return &groupRatioSnapshot{
		groupRatio:              cloneFlatMap(defaultGroupRatio),
		groupGroupRatio:         cloneNestedMap(defaultGroupGroupRatio),
		groupSpecialUsableGroup: cloneNestedMap(defaultGroupSpecialUsableGroup),
	}
}

func GetGroupRatioSetting() *GroupRatioSetting {
	snapshot := groupRatioSetting.snapshot()
	groupRatio := types.NewRWMap[string, float64]()
	groupRatio.AddAll(cloneFlatMap(snapshot.groupRatio))
	groupGroupRatio := types.NewRWMap[string, map[string]float64]()
	groupGroupRatio.AddAll(cloneNestedMap(snapshot.groupGroupRatio))
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(cloneNestedMap(snapshot.groupSpecialUsableGroup))
	return &GroupRatioSetting{
		GroupRatio:              groupRatio,
		GroupGroupRatio:         groupGroupRatio,
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
	}
}

func (s *managedGroupRatioSetting) ExportConfigMap() (map[string]string, error) {
	snapshot := s.snapshot()
	groupRatio, err := common.Marshal(snapshot.groupRatio)
	if err != nil {
		return nil, err
	}
	groupGroupRatio, err := common.Marshal(snapshot.groupGroupRatio)
	if err != nil {
		return nil, err
	}
	groupSpecialUsableGroup, err := common.Marshal(snapshot.groupSpecialUsableGroup)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"group_ratio":                string(groupRatio),
		"group_group_ratio":          string(groupGroupRatio),
		"group_special_usable_group": string(groupSpecialUsableGroup),
	}, nil
}

func (s *managedGroupRatioSetting) ValidateConfigMap(values map[string]string) error {
	_, err := parseGroupRatioUpdate(values)
	return err
}

func (s *managedGroupRatioSetting) UpdateConfigMap(values map[string]string) error {
	update, err := parseGroupRatioUpdate(values)
	if err != nil {
		return err
	}
	s.apply(update)
	return nil
}

// The compatibility DTO owns its MapConfig operations. It is deliberately
// disconnected from the registered managed configuration. Its RWMaps retain
// their own locking, but the detached DTO does not promise a cross-field
// transaction when callers concurrently mutate it.
func (s *GroupRatioSetting) ExportConfigMap() (map[string]string, error) {
	if s == nil {
		return map[string]string{
			"group_ratio":                "{}",
			"group_group_ratio":          "{}",
			"group_special_usable_group": "{}",
		}, nil
	}
	s.ensureMaps()
	groupRatio, err := common.Marshal(s.GroupRatio.ReadAll())
	if err != nil {
		return nil, err
	}
	groupGroupRatio, err := common.Marshal(cloneNestedMap(s.GroupGroupRatio.ReadAll()))
	if err != nil {
		return nil, err
	}
	groupSpecialUsableGroup, err := common.Marshal(cloneNestedMap(s.GroupSpecialUsableGroup.ReadAll()))
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"group_ratio":                string(groupRatio),
		"group_group_ratio":          string(groupGroupRatio),
		"group_special_usable_group": string(groupSpecialUsableGroup),
	}, nil
}

func (s *GroupRatioSetting) ValidateConfigMap(values map[string]string) error {
	_, err := parseGroupRatioUpdate(values)
	return err
}

func (s *GroupRatioSetting) UpdateConfigMap(values map[string]string) error {
	if s == nil {
		return fmt.Errorf("group ratio setting must not be nil")
	}
	update, err := parseGroupRatioUpdate(values)
	if err != nil {
		return err
	}

	s.ensureMaps()
	if update.groupRatioSet {
		s.GroupRatio.Clear()
		s.GroupRatio.AddAll(cloneFlatMap(update.groupRatio))
	}
	if update.groupGroupRatioSet {
		s.GroupGroupRatio.Clear()
		s.GroupGroupRatio.AddAll(cloneNestedMap(update.groupGroupRatio))
	}
	if update.groupSpecialSet {
		s.GroupSpecialUsableGroup.Clear()
		s.GroupSpecialUsableGroup.AddAll(cloneNestedMap(update.groupSpecialUsableGroup))
	}
	return nil
}

func (s *GroupRatioSetting) ensureMaps() {
	if s.GroupRatio == nil {
		s.GroupRatio = types.NewRWMap[string, float64]()
	}
	if s.GroupGroupRatio == nil {
		s.GroupGroupRatio = types.NewRWMap[string, map[string]float64]()
	}
	if s.GroupSpecialUsableGroup == nil {
		s.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
	}
}

func parseGroupRatioUpdate(values map[string]string) (groupRatioUpdate, error) {
	var update groupRatioUpdate
	if raw, ok := values["group_ratio"]; ok {
		if err := ValidateRatioMapJSON(raw); err != nil {
			return groupRatioUpdate{}, err
		}
		if err := common.Unmarshal([]byte(raw), &update.groupRatio); err != nil {
			return groupRatioUpdate{}, err
		}
		update.groupRatioSet = true
	}
	if raw, ok := values["group_group_ratio"]; ok {
		if err := ValidateNestedRatioMapJSON(raw); err != nil {
			return groupRatioUpdate{}, err
		}
		if err := common.Unmarshal([]byte(raw), &update.groupGroupRatio); err != nil {
			return groupRatioUpdate{}, err
		}
		update.groupGroupRatio = cloneNestedMap(update.groupGroupRatio)
		update.groupGroupRatioSet = true
	}
	if raw, ok := values["group_special_usable_group"]; ok {
		if err := ValidateGroupSpecialUsableGroupJSON(raw); err != nil {
			return groupRatioUpdate{}, err
		}
		if err := common.Unmarshal([]byte(raw), &update.groupSpecialUsableGroup); err != nil {
			return groupRatioUpdate{}, err
		}
		update.groupSpecialUsableGroup = cloneNestedMap(update.groupSpecialUsableGroup)
		update.groupSpecialSet = true
	}
	return update, nil
}

func (s *managedGroupRatioSetting) apply(update groupRatioUpdate) {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	latest := s.snapshot()
	candidate := &groupRatioSnapshot{
		groupRatio:              latest.groupRatio,
		groupGroupRatio:         latest.groupGroupRatio,
		groupSpecialUsableGroup: latest.groupSpecialUsableGroup,
	}
	if update.groupRatioSet {
		candidate.groupRatio = cloneFlatMap(update.groupRatio)
	}
	if update.groupGroupRatioSet {
		candidate.groupGroupRatio = cloneNestedMap(update.groupGroupRatio)
	}
	if update.groupSpecialSet {
		candidate.groupSpecialUsableGroup = cloneNestedMap(update.groupSpecialUsableGroup)
	}
	s.current.Store(candidate)
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
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("group special usable user group must not be empty")
		}
		if group == nil {
			return fmt.Errorf("group special usable group %s must be a JSON object", name)
		}
		seenTargets := make(map[string]struct{}, len(group))
		for rawTarget := range group {
			target := rawTarget
			if strings.HasPrefix(target, "+:") || strings.HasPrefix(target, "-:") {
				target = target[2:]
			}
			if strings.TrimSpace(target) == "" {
				return fmt.Errorf("group special usable group %q target must not be empty", name)
			}
			if _, exists := seenTargets[target]; exists {
				return fmt.Errorf("group special usable group %q defines target %q more than once", name, target)
			}
			seenTargets[target] = struct{}{}
		}
	}
	return nil
}

func GetGroupRatioCopy() map[string]float64 {
	return cloneFlatMap(groupRatioSetting.snapshot().groupRatio)
}

func ContainsGroupRatio(name string) bool {
	_, ok := groupRatioSetting.snapshot().groupRatio[name]
	return ok
}

func GroupRatio2JSONString() string {
	bytes, err := common.Marshal(groupRatioSetting.snapshot().groupRatio)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	return groupRatioSetting.UpdateConfigMap(map[string]string{"group_ratio": jsonStr})
}

func GetGroupRatio(name string) float64 {
	ratio, ok := groupRatioSetting.snapshot().groupRatio[name]
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
	group, ok := groupRatioSetting.snapshot().groupGroupRatio[userGroup]
	if !ok {
		return -1, false
	}
	ratio, ok := group[usingGroup]
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
	bytes, err := common.Marshal(groupRatioSetting.snapshot().groupGroupRatio)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	return groupRatioSetting.UpdateConfigMap(map[string]string{"group_group_ratio": jsonStr})
}

func GetGroupSpecialUsableGroup(userGroup string) (map[string]string, bool) {
	group, ok := groupRatioSetting.snapshot().groupSpecialUsableGroup[userGroup]
	if !ok {
		return nil, false
	}
	return cloneFlatMap(group), true
}

func GroupSpecialUsableGroup2JSONString() string {
	bytes, err := common.Marshal(groupRatioSetting.snapshot().groupSpecialUsableGroup)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func UpdateGroupSpecialUsableGroupByJSONString(jsonStr string) error {
	return groupRatioSetting.UpdateConfigMap(map[string]string{"group_special_usable_group": jsonStr})
}

func CheckGroupRatio(jsonStr string) error {
	return ValidateRatioMapJSON(jsonStr)
}
