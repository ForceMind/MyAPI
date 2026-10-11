package model

import (
	"math"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

// applyLegacyUserWalletDeltaTx keeps the wallet update in the caller's business
// transaction. The caller retains its funding, amount and replay policies.
// An exclusive upper bound of zero means there is no additional business cap.
func applyLegacyUserWalletDeltaTx(tx *gorm.DB, userID int, delta, maxQuotaExclusive int64, metadata map[string]interface{}) error {
	if delta < int64(common.MinQuota) || delta > int64(common.MaxQuota) {
		return ErrUserQuotaMutationInvalidInput
	}
	var user User
	if err := lockForUpdate(tx).Select("id", "quota", "quota_version").Where("id = ?", userID).First(&user).Error; err != nil {
		return err
	}
	target, err := common.CheckedQuotaBalance(user.Quota, delta)
	if err != nil {
		return err
	}
	if maxQuotaExclusive > 0 && target >= maxQuotaExclusive {
		return ErrUserQuotaUpperBoundExceeded
	}
	if user.QuotaVersion < 0 || user.QuotaVersion == math.MaxInt64 {
		return ErrUserQuotaMutationConflict
	}
	updates := make(map[string]interface{}, len(metadata)+2)
	for key, value := range metadata {
		updates[key] = value
	}
	updates["quota"] = target
	updates["quota_version"] = user.QuotaVersion + 1
	result := tx.Model(&User{}).
		Where("id = ? AND quota = ? AND quota_version = ?", user.Id, user.Quota, user.QuotaVersion).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrUserQuotaMutationConflict
	}
	return nil
}
