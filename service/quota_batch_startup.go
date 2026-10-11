package service

import (
	"github.com/ForceMind/MyAPI/model"
	"gorm.io/gorm"
)

// ValidateQuotaBatchStartupConfiguration is a read-only startup check, not a
// writer-mode migration. It never drains, clears, or rewrites accounting facts.
// Native installations keep their historical unset=false configuration; the
// cache-free deployment tools separately require an explicit reviewed choice.
func ValidateQuotaBatchStartupConfiguration(db *gorm.DB, configured string, redisAvailable bool) (bool, error) {
	enabled, err := model.ParseQuotaBatchUpdateEnabled(configured)
	if err != nil || !enabled {
		return enabled, err
	}
	state, err := model.GetQuotaWriterEpochState(db)
	if err != nil {
		return false, model.ErrQuotaWriterStartupUnverifiable
	}
	if err := model.ValidateQuotaBatchWriterMode(model.QuotaWriterMode(state.Mode), redisAvailable); err != nil {
		return false, err
	}
	return true, nil
}
