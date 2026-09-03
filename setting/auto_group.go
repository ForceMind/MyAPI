package setting

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
)

const DefaultMaxTokenAutoGroups = 5

var autoGroups = []string{
	"default",
}
var autoGroupsMutex sync.RWMutex

var DefaultUseAutoGroup = false

var maxTokenAutoGroups atomic.Int64

func init() {
	maxTokenAutoGroups.Store(DefaultMaxTokenAutoGroups)
}

func ContainsAutoGroup(group string) bool {
	autoGroupsMutex.RLock()
	defer autoGroupsMutex.RUnlock()
	for _, autoGroup := range autoGroups {
		if autoGroup == group {
			return true
		}
	}
	return false
}

func UpdateAutoGroupsByJsonString(jsonString string) error {
	var groups []string
	if err := common.Unmarshal([]byte(jsonString), &groups); err != nil {
		return err
	}
	if groups == nil {
		groups = []string{}
	}
	autoGroupsMutex.Lock()
	autoGroups = groups
	autoGroupsMutex.Unlock()
	return nil
}

func ValidateAutoGroupsJSON(value string) error {
	var groups []string
	return common.Unmarshal([]byte(value), &groups)
}

func AutoGroups2JsonString() string {
	autoGroupsMutex.RLock()
	defer autoGroupsMutex.RUnlock()
	jsonBytes, err := common.Marshal(autoGroups)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

func GetAutoGroups() []string {
	autoGroupsMutex.RLock()
	defer autoGroupsMutex.RUnlock()
	return append([]string(nil), autoGroups...)
}

func GetMaxTokenAutoGroups() int {
	return int(maxTokenAutoGroups.Load())
}

func ValidateMaxTokenAutoGroups(value string) error {
	maxCount, err := strconv.Atoi(value)
	if err != nil || maxCount <= 0 {
		return fmt.Errorf("MaxTokenAutoGroups must be a positive integer")
	}
	return nil
}

func UpdateMaxTokenAutoGroups(value string) error {
	if err := ValidateMaxTokenAutoGroups(value); err != nil {
		return err
	}
	maxCount, _ := strconv.Atoi(value)
	maxTokenAutoGroups.Store(int64(maxCount))
	return nil
}
