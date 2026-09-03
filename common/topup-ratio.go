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
	next, err := parseTopupGroupRatioJSON(jsonStr)
	if err != nil {
		return err
	}

	topupGroupRatioMutex.Lock()
	defer topupGroupRatioMutex.Unlock()
	topupGroupRatio = next
	return nil
}

func ValidateTopupGroupRatioJSON(jsonStr string) error {
	_, err := parseTopupGroupRatioJSON(jsonStr)
	return err
}

func parseTopupGroupRatioJSON(jsonStr string) (map[string]float64, error) {
	var next map[string]float64
	if err := Unmarshal([]byte(jsonStr), &next); err != nil {
		return nil, err
	}
	if len(next) == 0 {
		return nil, fmt.Errorf("topup group ratio must not be empty")
	}
	for name, ratio := range next {
		if ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			return nil, fmt.Errorf("topup group ratio %q must be finite and non-negative", name)
		}
	}
	return next, nil
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
