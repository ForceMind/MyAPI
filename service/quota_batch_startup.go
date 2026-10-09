package service

import (
	"errors"

	"github.com/ForceMind/MyAPI/model"
	"gorm.io/gorm"
)

// ValidateQuotaBatchStartupConfiguration is a read-only startup check, not a
// writer-mode migration. It never drains, clears, or rewrites accounting facts.
// Native installations keep their historical unset=false configuration; the
// cache-free deployment tools separately require an explicit reviewed choice.
func ValidateQuotaBatchStartupConfiguration(db *gorm.DB, configured string, redisAvailable bool) (bool, error) {
	switch configured {
	case "", "false":
		return false, nil
	case "true":
	default:
		return false, errors.New("BATCH_UPDATE_ENABLED must be exactly true or false")
	}
	state, err := model.GetQuotaWriterEpochState(db)
	if err != nil {
		return false, errors.New("cannot verify quota writer mode for batch startup; review the database and cache configuration before restarting")
	}
	if redisAvailable {
		return true, nil
	}
	switch model.QuotaWriterMode(state.Mode) {
	case model.QuotaWriterModeAuthoritative:
		// Authoritative quota commits to SQL. Missing Redis only delays its
		// separately durable cache projection; it must not block startup.
		return true, nil
	case model.QuotaWriterModeLegacy, model.QuotaWriterModeBridge:
		// Bridge is a reviewed transition, not proof that legacy cache work
		// was drained. Never authorize a no-cache batch writer from this state.
		return false, errors.New("legacy or bridge quota batching requires Redis; restore the reviewed cache configuration before restarting; do not disable batching or clear accounting queues to bypass this check")
	default:
		return false, errors.New("cannot verify quota writer mode for batch startup; review the database and cache configuration before restarting")
	}
}
