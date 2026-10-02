package model

import (
	"context"
	"errors"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"gorm.io/gorm"
)

const BillingSourceSelfUse = "self_use"

var ErrUserUsagePolicyInvalid = errors.New("invalid user usage policy")
var ErrUserUsagePolicyConflict = errors.New("user usage policy revision conflict")

type UserUsagePolicy struct {
	UserID               int   `json:"user_id"`
	NoBalance            bool  `json:"no_balance"`
	Revision             int64 `json:"revision"`
	LegacyRemainingQuota int   `json:"legacy_remaining_quota"`
}

type UserUsagePolicyInput struct {
	ID               string `json:"id"`
	UserID           int    `json:"user_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	NoBalance        bool   `json:"no_balance"`
}

type UserUsagePolicyChange struct {
	ID         string `gorm:"type:varchar(64);primaryKey"`
	Digest     string `gorm:"type:varchar(64);not null"`
	ActorID    int    `gorm:"not null"`
	UserID     int    `gorm:"not null;index"`
	BeforeJSON string `gorm:"type:text;not null"`
	AfterJSON  string `gorm:"type:text;not null"`
	CreatedAt  int64  `gorm:"type:bigint;not null"`
}

func (*UserUsagePolicyChange) BeforeUpdate(*gorm.DB) error { return ErrAccountQuotaReceiptImmutable }
func (*UserUsagePolicyChange) BeforeDelete(*gorm.DB) error { return ErrAccountQuotaReceiptImmutable }

func userUsagePolicyView(user *User) *UserUsagePolicy {
	return &UserUsagePolicy{UserID: user.Id, NoBalance: user.SelfUseNoBalance, Revision: user.UsagePolicyRevision, LegacyRemainingQuota: user.Quota}
}

func ReadUserUsagePolicy(ctx context.Context, db *gorm.DB, actorID, userID int) (*UserUsagePolicy, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if actorID <= 0 || userID <= 0 {
		return nil, ErrUserUsagePolicyInvalid
	}
	var actor User
	if err := db.WithContext(ctx).First(&actor, actorID).Error; err != nil {
		return nil, err
	}
	if actor.Status != common.UserStatusEnabled || actorID != userID && actor.Role != common.RoleRootUser {
		return nil, gorm.ErrRecordNotFound
	}
	var user User
	if err := db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return nil, err
	}
	return userUsagePolicyView(&user), nil
}

// This changes policy only: it never grants quota, clears usage, or rewrites a
// request's already captured funding source. Existing users remain limited
// until Root explicitly changes this private policy field.
func ConfigureUserUsagePolicy(ctx context.Context, db *gorm.DB, actorID int, input UserUsagePolicyInput) (*UserUsagePolicy, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if actorID <= 0 || !validBudgetDigest(input.ID) || input.UserID <= 0 || input.ExpectedRevision < 0 || input.ExpectedRevision >= MaxTokenBudget {
		return nil, ErrUserUsagePolicyInvalid
	}
	digest, err := budgetDigest(struct {
		Actor int
		Input UserUsagePolicyInput
	}{actorID, input})
	if err != nil {
		return nil, err
	}
	var result *UserUsagePolicy
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorizeUsageReviewer(tx, actorID); err != nil {
			return err
		}
		var user User
		if err := lockForUpdate(tx).First(&user, input.UserID).Error; err != nil {
			return err
		}
		var prior UserUsagePolicyChange
		err := tx.First(&prior, "id = ?", input.ID).Error
		if err == nil {
			if prior.ID != input.ID || prior.Digest != digest {
				return ErrUserUsagePolicyConflict
			}
			result = userUsagePolicyView(&user)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if user.UsagePolicyRevision != input.ExpectedRevision {
			return ErrUserUsagePolicyConflict
		}
		before, err := common.Marshal(userUsagePolicyView(&user))
		if err != nil {
			return err
		}
		previous := user.UsagePolicyRevision
		user.SelfUseNoBalance, user.UsagePolicyRevision = input.NoBalance, previous+1
		after, err := common.Marshal(userUsagePolicyView(&user))
		if err != nil {
			return err
		}
		write := tx.Model(&User{}).Where("id = ? AND usage_policy_revision = ?", user.Id, previous).Updates(map[string]any{"self_use_no_balance": user.SelfUseNoBalance, "usage_policy_revision": user.UsagePolicyRevision})
		if write.Error != nil {
			return write.Error
		}
		if write.RowsAffected != 1 {
			return ErrUserUsagePolicyConflict
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		if err := tx.Create(&UserUsagePolicyChange{ID: input.ID, Digest: digest, ActorID: actorID, UserID: user.Id, BeforeJSON: string(before), AfterJSON: string(after), CreatedAt: now}).Error; err != nil {
			return err
		}
		result = userUsagePolicyView(&user)
		return nil
	})
	return result, err
}

// Read the authoritative funding state at admission. A stored preference is
// effective only in valid disabled mode; stale/invalid state never grants a
// wallet bypass. The selected source is persisted with the request and is not
// recomputed during settlement, cancellation or manual recovery.
func selfUseNoBalanceAdmissionTx(tx *gorm.DB, user *User) (bool, error) {
	if !user.SelfUseNoBalance {
		return false, nil
	}
	if user.UsagePolicyRevision <= 0 {
		return false, ErrUserUsagePolicyInvalid
	}
	state, _, err := readUserFundingStateTx(tx, false)
	if err != nil {
		return false, err
	}
	if !state.Valid {
		return false, ErrUserFundingUnavailable
	}
	return state.Mode == operation_setting.UserFundingModeDisabled, nil
}

func ResolveSelfUseNoBalanceAdmission(ctx context.Context, db *gorm.DB, userID int) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if userID <= 0 {
		return false, ErrUserUsagePolicyInvalid
	}
	var user User
	if err := db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return false, err
	}
	if user.Status != common.UserStatusEnabled {
		return false, ErrAccountQuotaMutationIneligible
	}
	return selfUseNoBalanceAdmissionTx(db.WithContext(ctx), &user)
}

func ValidateUserUsagePolicySchema(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	for _, column := range []string{"self_use_no_balance", "usage_policy_revision"} {
		if !db.Migrator().HasColumn(&User{}, column) {
			return errors.New("user usage policy migration is required")
		}
	}
	if !db.Migrator().HasTable(&UserUsagePolicyChange{}) || !db.Migrator().HasColumn(&LegacyUsageReservation{}, "usage_policy_revision") {
		return errors.New("self-use admission migration is required")
	}
	return nil
}
