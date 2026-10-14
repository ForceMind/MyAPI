package model

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func personalFundingStateReadForTest(statement *gorm.Statement) bool {
	if statement == nil || statement.Table != "options" {
		return false
	}
	for _, value := range statement.Vars {
		if key, ok := value.(string); ok && key == UserFundingStateOptionKey {
			return true
		}
	}
	return false
}

func TestPersonalUsageAdmissionFundingStateBoundary(t *testing.T) {
	t.Run("configured funding does not read options", func(t *testing.T) {
		db, _ := userFundingPolicyTestDB(t, operation_setting.UserFundingModeDisabled)
		reads := 0
		callback := "personal-usage-configured-funding"
		require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
			if tx.Statement.Table == "options" {
				reads++
			}
		}))
		t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

		eligible, err := selfUseNoBalanceAdmissionTx(db, &User{SelfUseNoBalance: false, UsagePolicyRevision: -1})
		require.NoError(t, err)
		assert.False(t, eligible)
		assert.Zero(t, reads)
	})

	t.Run("invalid revision fails before options", func(t *testing.T) {
		for _, revision := range []int64{0, -1} {
			t.Run(fmt.Sprintf("revision_%d", revision), func(t *testing.T) {
				db, _ := userFundingPolicyTestDB(t, operation_setting.UserFundingModeDisabled)
				reads := 0
				callback := "personal-usage-invalid-revision"
				require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table == "options" {
						reads++
					}
				}))
				t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

				eligible, err := selfUseNoBalanceAdmissionTx(db, &User{SelfUseNoBalance: true, UsagePolicyRevision: revision})
				assert.False(t, eligible)
				require.ErrorIs(t, err, ErrUserUsagePolicyInvalid)
				assert.Zero(t, reads)
			})
		}
	})

	t.Run("valid policy reads authoritative state key once", func(t *testing.T) {
		db, _ := userFundingPolicyTestDB(t, operation_setting.UserFundingModeDisabled)
		stateReads := 0
		callback := "personal-usage-authoritative-state"
		require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
			if personalFundingStateReadForTest(tx.Statement) {
				stateReads++
			}
		}))
		t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

		eligible, err := selfUseNoBalanceAdmissionTx(db, &User{SelfUseNoBalance: true, UsagePolicyRevision: 1})
		require.NoError(t, err)
		assert.True(t, eligible)
		assert.Equal(t, 1, stateReads, "projection and legacy compatibility reads are separate from the authoritative state-key read")
	})

	t.Run("storage error preserves identity", func(t *testing.T) {
		db, _ := userFundingPolicyTestDB(t, operation_setting.UserFundingModeDisabled)
		storageErr := errors.New("personal funding state unavailable")
		reads := 0
		callback := "personal-usage-state-storage-error"
		require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
			if tx.Statement.Table == "options" {
				reads++
				tx.AddError(storageErr)
			}
		}))
		t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

		eligible, err := selfUseNoBalanceAdmissionTx(db, &User{SelfUseNoBalance: true, UsagePolicyRevision: 1})
		assert.False(t, eligible)
		require.ErrorIs(t, err, storageErr)
		assert.Equal(t, 1, reads)
	})

	t.Run("invalid authoritative state maps to funding unavailable", func(t *testing.T) {
		db, state := userFundingPolicyTestDB(t, operation_setting.UserFundingModeDisabled)
		raw, err := encodeUserFundingState(UserFundingStateSnapshot{Mode: operation_setting.UserFundingMode("invalid"), Epoch: state.Epoch})
		require.NoError(t, err)
		require.NoError(t, db.Model(&Option{}).Where("key = ?", UserFundingStateOptionKey).Update("value", raw).Error)

		eligible, err := selfUseNoBalanceAdmissionTx(db, &User{SelfUseNoBalance: true, UsagePolicyRevision: 1})
		assert.False(t, eligible)
		require.Equal(t, ErrUserFundingUnavailable, err)
	})
}
