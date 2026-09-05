package model

import (
	"crypto/subtle"
	"errors"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrTaskRecoveryIdentityMismatch  = errors.New("task recovery database identity does not match this deployment")
	ErrTaskRecoveryIdentityMissing   = errors.New("task recovery identity is missing for existing submission records")
	ErrTaskRecoveryIdentityImmutable = errors.New("task recovery database identity is immutable")
)

// TaskRecoveryIdentity is a singleton binding the primary database to the
// dedicated idempotency key. It stores a verifier, never the key itself.
// V1 has no automatic rotation/reset/delete path: changing this identity could
// turn a replay into another billable operation.
type TaskRecoveryIdentity struct {
	ID              int    `json:"-" gorm:"primaryKey;autoIncrement:false;<-:create"`
	ProtocolVersion int    `json:"-" gorm:"not null;<-:create"`
	KeyVerifier     string `json:"-" gorm:"type:char(64);not null;<-:create"`
	CreatedAt       int64  `json:"-" gorm:"type:bigint;not null;<-:create"`
}

func (identity *TaskRecoveryIdentity) BeforeCreate(_ *gorm.DB) error {
	verifier, err := common.TaskRecoveryIdempotencyKeyVerifier()
	if err != nil {
		return err
	}
	if identity.ID != 1 || identity.ProtocolVersion != 1 || identity.KeyVerifier != verifier || identity.CreatedAt <= 0 {
		return ErrTaskRecoveryIdentityMismatch
	}
	return nil
}

func (*TaskRecoveryIdentity) BeforeUpdate(_ *gorm.DB) error {
	return ErrTaskRecoveryIdentityImmutable
}

func (*TaskRecoveryIdentity) BeforeDelete(_ *gorm.DB) error {
	return ErrTaskRecoveryIdentityImmutable
}

// EnsureTaskRecoveryIdentity runs on startup when the deployment requests
// recovery and before every operation insert. A row absent from an otherwise
// populated operation store is ambiguous, so it is never silently rebound.
// The unique singleton arbitrates concurrent first-starts across SQL dialects.
func EnsureTaskRecoveryIdentity(tx *gorm.DB) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	// Discard the caller's model/WHERE scope without leaving its transaction.
	// In particular, operation-create hooks must query the identity table.
	tx = tx.Session(&gorm.Session{NewDB: true})
	verifier, err := common.TaskRecoveryIdempotencyKeyVerifier()
	if err != nil {
		return err
	}
	var identity TaskRecoveryIdentity
	err = tx.Where("id = ?", 1).Take(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var operations int64
		if err := tx.Model(&TaskSubmissionOperation{}).Count(&operations).Error; err != nil {
			return err
		}
		if operations != 0 {
			return ErrTaskRecoveryIdentityMissing
		}
		candidate := TaskRecoveryIdentity{
			ID: 1, ProtocolVersion: 1, KeyVerifier: verifier, CreatedAt: common.GetTimestamp(),
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}}, DoNothing: true,
		}).Create(&candidate).Error; err != nil {
			return err
		}
		err = tx.Where("id = ?", 1).Take(&identity).Error
	}
	if err != nil {
		return err
	}
	if identity.ProtocolVersion != 1 || subtle.ConstantTimeCompare([]byte(identity.KeyVerifier), []byte(verifier)) != 1 {
		return ErrTaskRecoveryIdentityMismatch
	}
	return nil
}
