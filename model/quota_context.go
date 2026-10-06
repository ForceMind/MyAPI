package model

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

// QuotaMutationContext supplies the durable business identity required by
// non-legacy quota writers. EventKey must be stable across retries.
type QuotaMutationContext struct {
	Context    context.Context
	EventKey   string
	ReasonCode string
}

func quotaMutationContextValues(input QuotaMutationContext) (context.Context, context.CancelFunc, string, string, error) {
	ctx := input.Context
	cancel := func() {}
	if ctx == nil {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	}
	eventKey := strings.TrimSpace(input.EventKey)
	reason := strings.TrimSpace(input.ReasonCode)
	if eventKey == "" || len(eventKey) > 64 {
		return ctx, cancel, "", "", ErrAccountQuotaMutationInvalidInput
	}
	if reason == "" {
		reason = "quota_context_mutation"
	}
	if len(reason) > 64 {
		return ctx, cancel, "", "", ErrAccountQuotaMutationInvalidInput
	}
	return ctx, cancel, eventKey, reason, nil
}

func currentQuotaWriterMode(db *gorm.DB) (QuotaWriterMode, error) {
	state, err := GetQuotaWriterEpochState(db)
	if err != nil {
		return "", err
	}
	return QuotaWriterMode(state.Mode), nil
}

func requireLegacyQuotaWriterCall() error {
	mode, err := currentQuotaWriterMode(DB)
	if err != nil {
		// Pre-WP3 databases and isolated legacy unit fixtures may not have the
		// epoch table yet. Once the authority table exists, failures remain
		// fail-closed and non-legacy modes are always rejected below.
		if DB != nil && !DB.Migrator().HasTable(&QuotaWriterEpoch{}) {
			return nil
		}
		return err
	}
	if mode != QuotaWriterModeLegacy {
		return ErrLegacyQuotaWriterModeDisabled
	}
	return nil
}

func mutateLegacyUserQuotaWithContext(ctx context.Context, id int, delta int64, requireSufficient bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	lock, err := prepareLegacySettlementSubject(ctx, DB.WithContext(ctx), BatchUpdateTypeUserQuota, id, "")
	if err != nil {
		return false, err
	}
	defer releaseQuotaBalanceSubjectLock(lock)
	applied := false
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}
		target, err := checkedLegacyBalanceTarget(user.Quota, int(delta))
		if err != nil {
			return err
		}
		if requireSufficient && target < 0 {
			return nil
		}
		result := tx.Model(&User{}).Where("id = ? AND quota = ? AND quota_version = ?", user.Id, user.Quota, user.QuotaVersion).
			Updates(map[string]any{"quota": target, "quota_version": user.QuotaVersion + 1})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountQuotaMutationCASLost
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if lock != nil && applied {
		var user User
		if err := DB.WithContext(ctx).First(&user, id).Error; err != nil {
			return false, err
		}
		if err := writeUserCacheWithQuotaBalanceOwner(user.ToBaseUser(), true, lock); err != nil {
			return false, err
		}
	}
	return applied, nil
}

func mutateLegacyTokenQuotaWithContext(ctx context.Context, tokenID int, key string, delta int64, requireSufficient bool, unlimited bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	var identity Token
	if err := DB.WithContext(ctx).Select("id", "key").Where("id = ?", tokenID).First(&identity).Error; err != nil {
		return false, err
	}
	if key != "" && identity.Key != key {
		return false, ErrAccountQuotaTerminalRecoveryConflict
	}
	lock, err := prepareLegacySettlementSubject(ctx, DB.WithContext(ctx), BatchUpdateTypeTokenQuota, tokenID, identity.Key)
	if err != nil {
		return false, err
	}
	defer releaseQuotaBalanceSubjectLock(lock)
	applied := false
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var token Token
		if err := lockForUpdate(tx).Where("id = ?", tokenID).First(&token).Error; err != nil {
			return err
		}
		remain, err := checkedLegacyBalanceTarget(token.RemainQuota, int(delta))
		if err != nil {
			return err
		}
		if requireSufficient && !unlimited && remain < 0 {
			return nil
		}
		used, err := checkedLegacyBalanceTarget(token.UsedQuota, -int(delta))
		if err != nil || used < 0 {
			return ErrAccountQuotaMutationIneligible
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		result := tx.Model(&Token{}).Where("id = ? AND remain_quota = ? AND used_quota = ? AND quota_version = ?", token.Id, token.RemainQuota, token.UsedQuota, token.QuotaVersion).
			Updates(map[string]any{"remain_quota": remain, "used_quota": used, "quota_version": token.QuotaVersion + 1, "accessed_time": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountQuotaMutationCASLost
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if lock != nil && applied {
		var token Token
		if err := DB.WithContext(ctx).First(&token, tokenID).Error; err != nil {
			return false, err
		}
		if _, err := cacheInitTokenWithQuotaBalanceOwner(token, lock); err != nil {
			return false, err
		}
	}
	return applied, nil
}

func mutateUserQuotaWithContext(input QuotaMutationContext, id int, delta int64) error {
	ctx, cancel, eventKey, reason, err := quotaMutationContextValues(input)
	defer cancel()
	if err != nil {
		return err
	}
	mode, err := currentQuotaWriterMode(DB.WithContext(ctx))
	if err != nil {
		return err
	}
	switch mode {
	case QuotaWriterModeLegacy:
		_, err = mutateLegacyUserQuotaWithContext(ctx, id, delta, false)
		return err
	case QuotaWriterModeAuthoritative:
		_, err = MutateUserQuota(DB.WithContext(ctx), UserQuotaMutationInput{UserID: id, Delta: delta, MutationType: "quota_context", BusinessEventKey: eventKey, ReasonCode: reason})
		return err
	default:
		return ErrDurableQuotaWriterModeDisabled
	}
}

func IncreaseUserQuotaWithContext(input QuotaMutationContext, id int, quota int) error {
	if quota < 0 || quota > common.MaxQuota {
		return ErrAccountQuotaMutationInvalidInput
	}
	return mutateUserQuotaWithContext(input, id, int64(quota))
}

func DecreaseUserQuotaWithContext(input QuotaMutationContext, id int, quota int) error {
	if quota < 0 || quota > common.MaxQuota {
		return ErrAccountQuotaMutationInvalidInput
	}
	return mutateUserQuotaWithContext(input, id, -int64(quota))
}

func TryReserveUserQuotaWithContext(input QuotaMutationContext, id int, quota int) (bool, error) {
	if quota < 0 || quota > common.MaxQuota {
		return false, ErrAccountQuotaMutationInvalidInput
	}
	ctx, cancel, eventKey, reason, err := quotaMutationContextValues(input)
	defer cancel()
	if err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	mode, err := currentQuotaWriterMode(DB.WithContext(ctx))
	if err != nil {
		return false, err
	}
	if mode == QuotaWriterModeLegacy {
		return mutateLegacyUserQuotaWithContext(ctx, id, -int64(quota), true)
	}
	if mode != QuotaWriterModeAuthoritative {
		return false, ErrDurableQuotaWriterModeDisabled
	}
	_, err = MutateUserQuota(DB.WithContext(ctx), UserQuotaMutationInput{UserID: id, Delta: -int64(quota), MutationType: "reserve", BusinessEventKey: eventKey, ReasonCode: reason})
	if errors.Is(err, ErrInsufficientUserQuota) {
		return false, nil
	}
	return err == nil, err
}

func mutateTokenQuotaWithContext(input QuotaMutationContext, tokenID int, key string, quota int64) error {
	ctx, cancel, eventKey, _, err := quotaMutationContextValues(input)
	defer cancel()
	if err != nil {
		return err
	}
	mode, err := currentQuotaWriterMode(DB.WithContext(ctx))
	if err != nil {
		return err
	}
	if mode == QuotaWriterModeLegacy {
		_, err = mutateLegacyTokenQuotaWithContext(ctx, tokenID, key, quota, false, false)
		return err
	}
	if mode != QuotaWriterModeAuthoritative {
		return ErrDurableQuotaWriterModeDisabled
	}
	var token Token
	if err := DB.WithContext(ctx).Select("id", "user_id").Where("id = ?", tokenID).First(&token).Error; err != nil {
		return err
	}
	_, err = MutateTokenQuotaAuthoritative(ctx, DB.WithContext(ctx), eventKey, token.UserId, tokenID, quota)
	_ = key
	return err
}

func IncreaseTokenQuotaWithContext(input QuotaMutationContext, tokenID int, key string, quota int) error {
	if quota < 0 || quota > common.MaxQuota {
		return ErrAccountQuotaMutationInvalidInput
	}
	return mutateTokenQuotaWithContext(input, tokenID, key, int64(quota))
}

func DecreaseTokenQuotaWithContext(input QuotaMutationContext, tokenID int, key string, quota int) error {
	if quota < 0 || quota > common.MaxQuota {
		return ErrAccountQuotaMutationInvalidInput
	}
	return mutateTokenQuotaWithContext(input, tokenID, key, -int64(quota))
}

func TryReserveTokenQuotaWithContext(input QuotaMutationContext, tokenID int, key string, quota int, unlimited bool) (bool, error) {
	if quota < 0 || quota > common.MaxQuota {
		return false, ErrAccountQuotaMutationInvalidInput
	}
	ctx, cancel, _, _, err := quotaMutationContextValues(input)
	defer cancel()
	if err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	mode, err := currentQuotaWriterMode(DB.WithContext(ctx))
	if err != nil {
		return false, err
	}
	if mode == QuotaWriterModeLegacy {
		return mutateLegacyTokenQuotaWithContext(ctx, tokenID, key, -int64(quota), true, unlimited)
	}
	if mode != QuotaWriterModeAuthoritative {
		return false, ErrDurableQuotaWriterModeDisabled
	}
	err = mutateTokenQuotaWithContext(input, tokenID, key, -int64(quota))
	if errors.Is(err, ErrAccountQuotaMutationInsufficient) {
		return false, nil
	}
	return err == nil, err
}
