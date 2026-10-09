package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestQuotaBatchStartupConfiguration(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeBridge, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			// Persisted historical obligations are neither cleared nor reclassified by
			// a startup verdict, including a rejected attempt to change deployment.
			fact := model.AccountQuotaSettlementFact{EventKey: "startup-history", RequestID: "startup-history", State: model.AccountQuotaSettlementRetryable, LastError: "existing cache failure", Delta: 60}
			require.NoError(t, db.Create(&fact).Error)
			drain := model.QuotaBalanceBatchDrain{SchemaVersion: 1, GenerationKey: "startup-history", WriterEpoch: 41, State: "unknown", CacheApplied: true, LastError: "existing uncertain cache mutation", LockVersion: 1}
			require.NoError(t, db.Create(&drain).Error)
			for _, redisAvailable := range []bool{false, true} {
				enabled, err := ValidateQuotaBatchStartupConfiguration(db, "true", redisAvailable)
				if redisAvailable || mode == model.QuotaWriterModeAuthoritative {
					require.NoError(t, err)
					require.True(t, enabled)
				} else {
					require.ErrorContains(t, err, "requires Redis")
					require.False(t, enabled)
				}
			}
			for _, configured := range []string{"", "false"} {
				enabled, err := ValidateQuotaBatchStartupConfiguration(db, configured, false)
				require.NoError(t, err)
				require.False(t, enabled)
			}
			var storedFact model.AccountQuotaSettlementFact
			var storedDrain model.QuotaBalanceBatchDrain
			require.NoError(t, db.First(&storedFact, fact.ID).Error)
			require.NoError(t, db.First(&storedDrain, drain.ID).Error)
			require.Equal(t, fact, storedFact)
			require.Equal(t, drain, storedDrain)
		})
	}
}

func TestQuotaBatchStartupRejectsAmbiguousConfigurationWithoutLeakingErrors(t *testing.T) {
	for _, configured := range []string{"TRUE", "False", "1", " true", "redis://secret@private.invalid"} {
		enabled, err := ValidateQuotaBatchStartupConfiguration(nil, configured, false)
		require.ErrorContains(t, err, "must be exactly true or false")
		require.False(t, enabled)
		require.NotContains(t, err.Error(), "private.invalid")
	}
	for _, redisAvailable := range []bool{false, true} {
		enabled, err := ValidateQuotaBatchStartupConfiguration(nil, "true", redisAvailable)
		require.ErrorContains(t, err, "cannot verify quota writer mode")
		require.False(t, enabled)
	}
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
	require.NoError(t, db.Model(&model.QuotaWriterEpoch{}).Where("id = ?", 1).Update("mode", "unknown").Error)
	enabled, err := ValidateQuotaBatchStartupConfiguration(db, "true", false)
	require.ErrorContains(t, err, "cannot verify quota writer mode")
	require.False(t, enabled)
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("startup-private-read-error", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && strings.Contains(tx.Statement.Schema.Table, "quota_writer_epochs") {
			tx.AddError(errors.New("private SQL and redis://secret@private.invalid"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("startup-private-read-error") })
	enabled, err = ValidateQuotaBatchStartupConfiguration(db, "true", false)
	require.ErrorContains(t, err, "cannot verify quota writer mode")
	require.NotContains(t, err.Error(), "private")
	require.NotContains(t, err.Error(), "SQL")
	require.NotContains(t, err.Error(), "redis://")
	require.False(t, enabled)
}
