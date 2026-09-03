package ratio_setting

import (
	"fmt"
	"math"

	"github.com/ForceMind/MyAPI/common"
)

func ValidateRatioMapJSON(jsonStr string) error {
	var ratios map[string]any
	if err := common.Unmarshal([]byte(jsonStr), &ratios); err != nil {
		return err
	}
	if ratios == nil {
		return fmt.Errorf("ratio map must be a JSON object")
	}
	for name, ratio := range ratios {
		value, ok := ratio.(float64)
		if !ok {
			return fmt.Errorf("ratio %s must be a number", name)
		}
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("ratio %s must be a finite non-negative number", name)
		}
	}
	return nil
}

func ValidateNestedRatioMapJSON(jsonStr string) error {
	var ratios map[string]any
	if err := common.Unmarshal([]byte(jsonStr), &ratios); err != nil {
		return err
	}
	if ratios == nil {
		return fmt.Errorf("nested ratio map must be a JSON object")
	}
	for outerName, nestedValue := range ratios {
		nested, ok := nestedValue.(map[string]any)
		if !ok {
			return fmt.Errorf("nested ratio %s must be a JSON object", outerName)
		}
		for innerName, ratio := range nested {
			value, ok := ratio.(float64)
			if !ok {
				return fmt.Errorf("ratio %s.%s must be a number", outerName, innerName)
			}
			if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("ratio %s.%s must be a finite non-negative number", outerName, innerName)
			}
		}
	}
	return nil
}
