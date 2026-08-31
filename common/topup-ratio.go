package common

import (
	"fmt"
	"math"
	"sync"
)

var topupGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}
var topupGroupRatioMutex sync.RWMutex

func TopupGroupRatio2JSONString() string {
	topupGroupRatioMutex.RLock()
	defer topupGroupRatioMutex.RUnlock()
	jsonBytes, err := Marshal(topupGroupRatio)
	if err != nil {
		SysError("error marshalling topup group ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateTopupGroupRatioByJSONString(jsonStr string) error {
	var next map[string]float64
	if err := Unmarshal([]byte(jsonStr), &next); err != nil {
		return err
	}
	if len(next) == 0 {
		return fmt.Errorf("topup group ratio must not be empty")
	}
	for name, ratio := range next {
		if ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			return fmt.Errorf("topup group ratio %q must be finite and non-negative", name)
		}
	}

	topupGroupRatioMutex.Lock()
	defer topupGroupRatioMutex.Unlock()
	topupGroupRatio = next
	return nil
}

func GetTopupGroupRatio(name string) float64 {
	topupGroupRatioMutex.RLock()
	defer topupGroupRatioMutex.RUnlock()
	ratio, ok := topupGroupRatio[name]
	if !ok {
		SysError("topup group ratio not found: " + name)
		return 1
	}
	return ratio
}
