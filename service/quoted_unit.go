package service

import (
	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/ForceMind/MyAPI/types"
)

// requestQuotaUnit does not even read the mutable global for captured quotes.
// A zero fallback detects legacy callers; captured units are always positive.
func requestQuotaUnit(priceData types.PriceData) float64 {
	if unit := priceData.QuotedQuotaUnit(0); unit != 0 {
		return unit
	}
	return common.QuotaPerUnit
}

func requestAudioRatios(priceData types.PriceData, modelName string) (float64, float64, float64) {
	if priceData.QuotedQuotaUnit(0) != 0 {
		return priceData.CompletionRatio, priceData.AudioRatio, priceData.AudioCompletionRatio
	}
	return ratio_setting.GetCompletionRatio(modelName),
		ratio_setting.GetAudioRatio(modelName), ratio_setting.GetAudioCompletionRatio(modelName)
}
