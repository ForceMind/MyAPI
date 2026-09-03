package setting

import (
	"sync"

	"github.com/ForceMind/MyAPI/common"
)

var userUsableGroups = map[string]string{
	"default": "默认分组",
	"vip":     "vip分组",
}
var userUsableGroupsMutex sync.RWMutex

func GetUserUsableGroupsCopy() map[string]string {
	userUsableGroupsMutex.RLock()
	defer userUsableGroupsMutex.RUnlock()

	copyUserUsableGroups := make(map[string]string)
	for k, v := range userUsableGroups {
		copyUserUsableGroups[k] = v
	}
	return copyUserUsableGroups
}

func UserUsableGroups2JSONString() string {
	userUsableGroupsMutex.RLock()
	defer userUsableGroupsMutex.RUnlock()

	jsonBytes, err := common.Marshal(userUsableGroups)
	if err != nil {
		common.SysLog("error marshalling user groups: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateUserUsableGroupsByJSONString(jsonStr string) error {
	var groups map[string]string
	if err := common.Unmarshal([]byte(jsonStr), &groups); err != nil {
		return err
	}
	if groups == nil {
		groups = map[string]string{}
	}
	userUsableGroupsMutex.Lock()
	userUsableGroups = groups
	userUsableGroupsMutex.Unlock()
	return nil
}

func ValidateUserUsableGroupsJSON(value string) error {
	var groups map[string]string
	return common.Unmarshal([]byte(value), &groups)
}

func GetUsableGroupDescription(groupName string) string {
	userUsableGroupsMutex.RLock()
	defer userUsableGroupsMutex.RUnlock()

	if desc, ok := userUsableGroups[groupName]; ok {
		return desc
	}
	return groupName
}
