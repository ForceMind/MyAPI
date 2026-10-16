package model

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TokenCredentialDispatchAdmissionKind string

const (
	TokenCredentialDispatchAdmissionKindLegacyTask TokenCredentialDispatchAdmissionKind = "legacy_task"
	TokenCredentialDispatchAdmissionKindMidjourney TokenCredentialDispatchAdmissionKind = "midjourney"
)

type TokenCredentialDispatchAdmissionState string

const (
	TokenCredentialDispatchAdmissionStateOpen    TokenCredentialDispatchAdmissionState = "open"
	TokenCredentialDispatchAdmissionStateUnknown TokenCredentialDispatchAdmissionState = "unknown"
	TokenCredentialDispatchAdmissionStateLinked  TokenCredentialDispatchAdmissionState = "linked"

	tokenCredentialDispatchWriteSetting = "token-credential-dispatch-admission:controlled-write"
	tokenCredentialDispatchWriteMarker  = "v1"
)

// TokenCredentialDispatchAdmission closes the interval between accepting a
// legacy asynchronous provider dispatch and persisting the resulting task.
// Rotation locks the same Token row and refuses to proceed while an admission
// is open or has an unknown provider outcome.
type TokenCredentialDispatchAdmission struct {
	ID                  int64                                 `json:"-" gorm:"primaryKey"`
	AdmissionID         string                                `json:"admission_id" gorm:"type:char(64);not null;uniqueIndex;<-:create"`
	RequestIdentityHash string                                `json:"-" gorm:"type:char(64);not null;uniqueIndex:uidx_token_dispatch_req,priority:4;<-:create"`
	Kind                TokenCredentialDispatchAdmissionKind  `json:"kind" gorm:"type:varchar(32);not null;uniqueIndex:uidx_token_dispatch_req,priority:3;index:idx_token_dispatch_gate,priority:3;<-:create"`
	State               TokenCredentialDispatchAdmissionState `json:"state" gorm:"type:varchar(16);not null;index:idx_token_dispatch_gate,priority:2"`
	UserID              int                                   `json:"-" gorm:"not null;index;<-:create"`
	TokenID             int                                   `json:"-" gorm:"not null;uniqueIndex:uidx_token_dispatch_req,priority:1;index:idx_token_dispatch_gate,priority:1;<-:create"`
	CredentialVersion   int64                                 `json:"credential_version" gorm:"type:bigint;not null;uniqueIndex:uidx_token_dispatch_req,priority:2;<-:create"`
	LinkedRecordID      int64                                 `json:"-" gorm:"type:bigint;not null;default:0"`
	LinkedIdentityHash  string                                `json:"-" gorm:"type:char(64);not null;default:''"`
	UnknownReasonCode   string                                `json:"-" gorm:"type:varchar(64);not null;default:''"`
	CreatedAt           int64                                 `json:"-" gorm:"type:bigint;not null;<-:create"`
	UpdatedAt           int64                                 `json:"-" gorm:"type:bigint;not null"`
	LinkedAt            int64                                 `json:"-" gorm:"type:bigint;not null;default:0"`
}

func (TokenCredentialDispatchAdmission) TableName() string {
	return "token_credential_dispatch_admissions"
}

func tokenCredentialDispatchControlledWrite(tx *gorm.DB) *gorm.DB {
	return tx.Session(&gorm.Session{NewDB: true}).Set(tokenCredentialDispatchWriteSetting, tokenCredentialDispatchWriteMarker)
}

func tokenCredentialDispatchWriteAllowed(tx *gorm.DB) bool {
	if tx == nil {
		return false
	}
	marker, ok := tx.Get(tokenCredentialDispatchWriteSetting)
	return ok && marker == tokenCredentialDispatchWriteMarker
}

func (admission *TokenCredentialDispatchAdmission) BeforeCreate(tx *gorm.DB) error {
	if !tokenCredentialDispatchWriteAllowed(tx) {
		return ErrAccountQuotaReceiptImmutable
	}
	if admission == nil || admission.ID != 0 || admission.CreatedAt != 0 || admission.UpdatedAt != 0 || admission.LinkedAt != 0 {
		return ErrTokenMaintenanceInvalid
	}
	if err := validateTokenCredentialDispatchAdmission(*admission); err != nil {
		return err
	}
	now, err := taskRecoveryDBTimestamp(tx)
	if err != nil {
		return err
	}
	admission.CreatedAt = now
	admission.UpdatedAt = now
	return nil
}

func (*TokenCredentialDispatchAdmission) BeforeUpdate(tx *gorm.DB) error {
	if !tokenCredentialDispatchWriteAllowed(tx) {
		return ErrAccountQuotaReceiptImmutable
	}
	return nil
}

func (*TokenCredentialDispatchAdmission) BeforeDelete(*gorm.DB) error {
	return ErrAccountQuotaReceiptImmutable
}

func validTokenCredentialDispatchKind(kind TokenCredentialDispatchAdmissionKind) bool {
	return kind == TokenCredentialDispatchAdmissionKindLegacyTask || kind == TokenCredentialDispatchAdmissionKindMidjourney
}

func validTokenCredentialDispatchReason(code string) bool {
	if code == "" || len(code) > 64 {
		return false
	}
	for _, char := range code {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}

func validateTokenCredentialDispatchAdmission(admission TokenCredentialDispatchAdmission) error {
	if !canonicalLowerHex(admission.AdmissionID, 64) || !canonicalLowerHex(admission.RequestIdentityHash, 64) ||
		!validTokenCredentialDispatchKind(admission.Kind) || admission.UserID <= 0 || admission.TokenID <= 0 ||
		admission.CredentialVersion <= 0 || admission.CredentialVersion > tokenCredentialVersionMaxExactJSON {
		return ErrTokenMaintenanceInvalid
	}
	switch admission.State {
	case TokenCredentialDispatchAdmissionStateOpen:
		if admission.LinkedRecordID != 0 || admission.LinkedIdentityHash != "" || admission.UnknownReasonCode != "" || admission.LinkedAt != 0 {
			return ErrTokenMaintenanceInvalid
		}
	case TokenCredentialDispatchAdmissionStateUnknown:
		if admission.LinkedRecordID != 0 || admission.LinkedIdentityHash != "" || !validTokenCredentialDispatchReason(admission.UnknownReasonCode) || admission.LinkedAt != 0 {
			return ErrTokenMaintenanceInvalid
		}
	case TokenCredentialDispatchAdmissionStateLinked:
		if admission.LinkedRecordID <= 0 || !canonicalLowerHex(admission.LinkedIdentityHash, 64) || admission.UnknownReasonCode != "" || admission.LinkedAt <= 0 {
			return ErrTokenMaintenanceInvalid
		}
	default:
		return ErrTokenMaintenanceInvalid
	}
	if admission.CreatedAt < 0 || admission.UpdatedAt < 0 || admission.UpdatedAt < admission.CreatedAt {
		return ErrTokenMaintenanceInvalid
	}
	return nil
}

func newTokenCredentialDispatchAdmissionID() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func tokenCredentialDispatchRequestIdentityHash(requestIdentity string) (string, error) {
	if requestIdentity == "" || len(requestIdentity) > 512 || strings.TrimSpace(requestIdentity) != requestIdentity {
		return "", ErrTokenMaintenanceInvalid
	}
	return common.GenerateHMAC("myapi/token-credential-dispatch/request/v1:" + requestIdentity), nil
}

func lockTokenCredentialDispatchOwner(ctx context.Context, tx *gorm.DB, userID, tokenID int) (Token, error) {
	if tx == nil || tx.Dialector == nil {
		return Token{}, gorm.ErrInvalidDB
	}
	if userID <= 0 || tokenID <= 0 {
		return Token{}, ErrTokenMaintenanceInvalid
	}
	var user User
	if err := lockForUpdate(tx.WithContext(ctx)).Select("id").Where("id = ?", userID).Take(&user).Error; err != nil {
		return Token{}, err
	}
	var token Token
	if err := lockForUpdate(tx.WithContext(ctx).Unscoped()).Select("id", "user_id", "credential_version", "deleted_at").Where("id = ?", tokenID).Take(&token).Error; err != nil {
		return Token{}, err
	}
	if token.DeletedAt.Valid || token.UserId != userID {
		return Token{}, ErrTokenMaintenanceDenied
	}
	return token, nil
}

func loadTokenCredentialDispatchAdmission(ctx context.Context, tx *gorm.DB, admissionID string, locked bool) (TokenCredentialDispatchAdmission, error) {
	var admission TokenCredentialDispatchAdmission
	query := tx.WithContext(ctx)
	if locked {
		query = lockForUpdate(query)
	}
	if err := query.Where("admission_id = ?", admissionID).Take(&admission).Error; err != nil {
		return admission, err
	}
	return admission, nil
}

// BeginTokenCredentialDispatchAdmission durably registers a legacy async send
// before the provider call. Only the call that creates the row authorizes one
// provider send. A repeated call returns the same row with an error because an
// existing open row may represent a process crash after the provider accepted
// the request. Unknown and linked rows also never authorize another send.
func BeginTokenCredentialDispatchAdmission(ctx context.Context, db *gorm.DB, userID, tokenID int, expectedVersion int64, requestIdentity string, kind TokenCredentialDispatchAdmissionKind) (*TokenCredentialDispatchAdmission, error) {
	if db == nil || db.Dialector == nil {
		return nil, gorm.ErrInvalidDB
	}
	if expectedVersion <= 0 || expectedVersion > tokenCredentialVersionMaxExactJSON || !validTokenCredentialDispatchKind(kind) {
		return nil, ErrTokenMaintenanceInvalid
	}
	requestHash, err := tokenCredentialDispatchRequestIdentityHash(requestIdentity)
	if err != nil {
		return nil, err
	}
	admissionID, err := newTokenCredentialDispatchAdmissionID()
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var result TokenCredentialDispatchAdmission
	err = runChannelQuotaIdentityTransaction(ctx, db, func(tx *gorm.DB) error {
		token, lockErr := lockTokenCredentialDispatchOwner(ctx, tx, userID, tokenID)
		if lockErr != nil {
			return lockErr
		}
		if token.CredentialVersion != expectedVersion || token.CredentialVersion <= 0 || token.CredentialVersion > tokenCredentialVersionMaxExactJSON {
			return ErrTokenCredentialVersionChanged
		}
		candidate := TokenCredentialDispatchAdmission{
			AdmissionID: admissionID, RequestIdentityHash: requestHash,
			Kind: kind, State: TokenCredentialDispatchAdmissionStateOpen,
			UserID: userID, TokenID: tokenID, CredentialVersion: expectedVersion,
		}
		created := tokenCredentialDispatchControlledWrite(tx).Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 1 {
			result = candidate
			return nil
		}
		if err := lockForUpdate(tx.WithContext(ctx)).Where(
			"token_id = ? AND credential_version = ? AND kind = ? AND request_identity_hash = ?",
			tokenID, expectedVersion, kind, requestHash,
		).Take(&result).Error; err != nil {
			return errors.Join(ErrTokenMaintenanceUnknown, err)
		}
		if err := validateTokenCredentialDispatchAdmission(result); err != nil {
			return errors.Join(ErrTokenMaintenanceUnknown, err)
		}
		if result.UserID != userID || result.TokenID != tokenID || result.CredentialVersion != expectedVersion || result.Kind != kind {
			return ErrTokenMaintenanceConflict
		}
		switch result.State {
		case TokenCredentialDispatchAdmissionStateOpen:
			return ErrTokenMaintenancePreconditionUnknown
		case TokenCredentialDispatchAdmissionStateUnknown:
			return ErrTokenMaintenancePreconditionUnknown
		case TokenCredentialDispatchAdmissionStateLinked:
			return ErrTokenMaintenanceConflict
		default:
			return ErrTokenMaintenanceUnknown
		}
	})
	if err != nil {
		if result.AdmissionID != "" {
			copy := result
			return &copy, err
		}
		return nil, err
	}
	copy := result
	return &copy, nil
}

func tokenCredentialDispatchLinkedIdentity(ctx context.Context, tx *gorm.DB, admission TokenCredentialDispatchAdmission, linkedRecordID int64) (string, error) {
	if linkedRecordID <= 0 {
		return "", ErrTokenMaintenanceInvalid
	}
	switch admission.Kind {
	case TokenCredentialDispatchAdmissionKindLegacyTask:
		var task Task
		if err := tx.WithContext(ctx).Select("id", "task_id", "user_id", "private_data").Where("id = ?", linkedRecordID).Take(&task).Error; err != nil {
			return "", err
		}
		if task.ID != linkedRecordID || task.TaskID == "" || task.UserId != admission.UserID || task.PrivateData.TokenId != admission.TokenID {
			return "", ErrTokenMaintenanceConflict
		}
		return common.GenerateHMAC("myapi/token-credential-dispatch/legacy-task/v1:" + task.TaskID), nil
	case TokenCredentialDispatchAdmissionKindMidjourney:
		var task Midjourney
		if err := tx.WithContext(ctx).Select("id", "code", "mj_id", "user_id", "status", "progress", "dispatch_token_id").Where("id = ?", linkedRecordID).Take(&task).Error; err != nil {
			return "", err
		}
		if int64(task.Id) != linkedRecordID || task.UserId != admission.UserID || task.DispatchTokenID != admission.TokenID {
			return "", ErrTokenMaintenanceConflict
		}
		identityClass := "accepted"
		identityCode := 0
		switch task.Code {
		case 1, 21, 22:
			if task.MjId == "" {
				return "", ErrTokenMaintenanceConflict
			}
		case 23, 24:
			if task.Status != string(TaskStatusFailure) || task.Progress != "100%" {
				return "", ErrTokenMaintenanceConflict
			}
			identityClass = "rejected"
			identityCode = task.Code
		default:
			return "", ErrTokenMaintenanceConflict
		}
		return subscriptionShareDigest(struct {
			Version         string `json:"version"`
			RecordID        int64  `json:"record_id"`
			Class           string `json:"class"`
			Code            int    `json:"code"`
			DispatchTokenID int    `json:"dispatch_token_id"`
			MjID            string `json:"mj_id"`
		}{
			Version: "midjourney/v2", RecordID: linkedRecordID, Class: identityClass, Code: identityCode,
			DispatchTokenID: task.DispatchTokenID, MjID: task.MjId,
		})
	default:
		return "", ErrTokenMaintenanceUnknown
	}
}

// CompleteTokenCredentialDispatchAdmission links an admission only after the
// corresponding Task or Midjourney row is committed with the same owner and
// stable Token ID.
func CompleteTokenCredentialDispatchAdmission(ctx context.Context, db *gorm.DB, userID, tokenID int, admissionID string, linkedRecordID int64) error {
	if db == nil || db.Dialector == nil {
		return gorm.ErrInvalidDB
	}
	if !canonicalLowerHex(admissionID, 64) || linkedRecordID <= 0 {
		return ErrTokenMaintenanceInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return runChannelQuotaIdentityTransaction(ctx, db, func(tx *gorm.DB) error {
		token, err := lockTokenCredentialDispatchOwner(ctx, tx, userID, tokenID)
		if err != nil {
			return err
		}
		admission, err := loadTokenCredentialDispatchAdmission(ctx, tx, admissionID, true)
		if err != nil {
			return err
		}
		if err := validateTokenCredentialDispatchAdmission(admission); err != nil {
			return errors.Join(ErrTokenMaintenanceUnknown, err)
		}
		if admission.UserID != userID || admission.TokenID != tokenID {
			return ErrTokenMaintenanceDenied
		}
		if token.CredentialVersion != admission.CredentialVersion {
			return ErrTokenCredentialVersionChanged
		}
		identityHash, err := tokenCredentialDispatchLinkedIdentity(ctx, tx, admission, linkedRecordID)
		if err != nil {
			return err
		}
		if admission.State == TokenCredentialDispatchAdmissionStateLinked {
			if admission.LinkedRecordID == linkedRecordID && admission.LinkedIdentityHash == identityHash {
				return nil
			}
			return ErrTokenMaintenanceConflict
		}
		if admission.State != TokenCredentialDispatchAdmissionStateOpen && admission.State != TokenCredentialDispatchAdmissionStateUnknown {
			return ErrTokenMaintenanceUnknown
		}
		var duplicate TokenCredentialDispatchAdmission
		duplicateQuery := lockForUpdate(tx.WithContext(ctx)).Select("id").
			Where("token_id = ? AND kind = ? AND linked_record_id = ? AND admission_id <> ?", tokenID, admission.Kind, linkedRecordID, admissionID).
			Limit(1).Find(&duplicate)
		if duplicateQuery.Error != nil {
			return duplicateQuery.Error
		}
		if duplicateQuery.RowsAffected != 0 {
			return ErrTokenMaintenanceConflict
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		updated := tokenCredentialDispatchControlledWrite(tx).Model(&TokenCredentialDispatchAdmission{}).
			Where("id = ? AND state IN ? AND linked_record_id = ?", admission.ID, []TokenCredentialDispatchAdmissionState{
				TokenCredentialDispatchAdmissionStateOpen, TokenCredentialDispatchAdmissionStateUnknown,
			}, 0).
			Updates(map[string]any{
				"state": TokenCredentialDispatchAdmissionStateLinked, "linked_record_id": linkedRecordID,
				"linked_identity_hash": identityHash, "unknown_reason_code": "", "linked_at": now, "updated_at": now,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrTokenMaintenanceConflict
		}
		return nil
	})
}

// MarkTokenCredentialDispatchAdmissionUnknown preserves an admission when the
// provider outcome or task persistence result cannot be proven.
func MarkTokenCredentialDispatchAdmissionUnknown(ctx context.Context, db *gorm.DB, userID, tokenID int, admissionID, reasonCode string) error {
	if db == nil || db.Dialector == nil {
		return gorm.ErrInvalidDB
	}
	if !canonicalLowerHex(admissionID, 64) || !validTokenCredentialDispatchReason(reasonCode) {
		return ErrTokenMaintenanceInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return runChannelQuotaIdentityTransaction(ctx, db, func(tx *gorm.DB) error {
		token, err := lockTokenCredentialDispatchOwner(ctx, tx, userID, tokenID)
		if err != nil {
			return err
		}
		admission, err := loadTokenCredentialDispatchAdmission(ctx, tx, admissionID, true)
		if err != nil {
			return err
		}
		if err := validateTokenCredentialDispatchAdmission(admission); err != nil {
			return errors.Join(ErrTokenMaintenanceUnknown, err)
		}
		if admission.UserID != userID || admission.TokenID != tokenID {
			return ErrTokenMaintenanceDenied
		}
		if token.CredentialVersion != admission.CredentialVersion {
			return ErrTokenCredentialVersionChanged
		}
		switch admission.State {
		case TokenCredentialDispatchAdmissionStateUnknown:
			return nil
		case TokenCredentialDispatchAdmissionStateLinked:
			return ErrTokenMaintenanceConflict
		case TokenCredentialDispatchAdmissionStateOpen:
		default:
			return ErrTokenMaintenanceUnknown
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		updated := tokenCredentialDispatchControlledWrite(tx).Model(&TokenCredentialDispatchAdmission{}).
			Where("id = ? AND state = ?", admission.ID, TokenCredentialDispatchAdmissionStateOpen).
			Updates(map[string]any{"state": TokenCredentialDispatchAdmissionStateUnknown, "unknown_reason_code": reasonCode, "updated_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrTokenMaintenanceConflict
		}
		return nil
	})
}

type tokenCredentialDispatchAdmissionRevisionRow struct {
	ID                  int64                                 `json:"id"`
	AdmissionID         string                                `json:"admission_id"`
	RequestIdentityHash string                                `json:"request_identity_hash"`
	Kind                TokenCredentialDispatchAdmissionKind  `json:"kind"`
	State               TokenCredentialDispatchAdmissionState `json:"state"`
	UserID              int                                   `json:"user_id"`
	TokenID             int                                   `json:"token_id"`
	CredentialVersion   int64                                 `json:"credential_version"`
	LinkedRecordID      int64                                 `json:"linked_record_id"`
	LinkedIdentityHash  string                                `json:"linked_identity_hash"`
	UnknownReasonCode   string                                `json:"unknown_reason_code"`
	CreatedAt           int64                                 `json:"created_at"`
	UpdatedAt           int64                                 `json:"updated_at"`
	LinkedAt            int64                                 `json:"linked_at"`
}

func readTokenCredentialRotationDispatchAdmissions(ctx context.Context, db *gorm.DB, token Token, locked bool, state *tokenCredentialRotationState) error {
	if !tokenCredentialRotationTableReady(db, &TokenCredentialDispatchAdmission{}) {
		tokenCredentialRotationAddBlocker(&state.blockers, "credential_dispatch_admission_unavailable", TokenMaintenanceStatusUnknown)
		return nil
	}
	var admissions []TokenCredentialDispatchAdmission
	query := tokenMaintenanceQuery(db.WithContext(ctx), locked).
		Where("token_id = ? AND (state IN ? OR credential_version = ?)", token.Id, []TokenCredentialDispatchAdmissionState{
			TokenCredentialDispatchAdmissionStateOpen, TokenCredentialDispatchAdmissionStateUnknown,
		}, token.CredentialVersion).
		Order("id")
	if err := query.Find(&admissions).Error; err != nil {
		return err
	}
	revisionRows := make([]tokenCredentialDispatchAdmissionRevisionRow, 0, len(admissions))
	for _, admission := range admissions {
		revisionRows = append(revisionRows, tokenCredentialDispatchAdmissionRevisionRow{
			ID: admission.ID, AdmissionID: admission.AdmissionID, RequestIdentityHash: admission.RequestIdentityHash,
			Kind: admission.Kind, State: admission.State, UserID: admission.UserID, TokenID: admission.TokenID,
			CredentialVersion: admission.CredentialVersion, LinkedRecordID: admission.LinkedRecordID,
			LinkedIdentityHash: admission.LinkedIdentityHash, UnknownReasonCode: admission.UnknownReasonCode,
			CreatedAt: admission.CreatedAt, UpdatedAt: admission.UpdatedAt, LinkedAt: admission.LinkedAt,
		})
		if err := validateTokenCredentialDispatchAdmission(admission); err != nil || admission.UserID != token.UserId || admission.TokenID != token.Id || admission.CredentialVersion > token.CredentialVersion {
			tokenCredentialRotationAddBlocker(&state.blockers, "credential_dispatch_admission_unknown", TokenMaintenanceStatusUnknown)
			continue
		}
		switch admission.State {
		case TokenCredentialDispatchAdmissionStateOpen:
			tokenCredentialRotationAddBlocker(&state.blockers, "credential_dispatch_admission_open", TokenMaintenanceStatusUnknown)
		case TokenCredentialDispatchAdmissionStateUnknown:
			tokenCredentialRotationAddBlocker(&state.blockers, "credential_dispatch_admission_unknown", TokenMaintenanceStatusUnknown)
		case TokenCredentialDispatchAdmissionStateLinked:
			if admission.CredentialVersion != token.CredentialVersion {
				continue
			}
			// Every earlier read in the locked commit path is a locking read. Begin
			// and Complete also lock Token first, so this first non-locking read is
			// taken only after the linking transaction committed. It observes the
			// saved Task without introducing a Task -> Token lock-order inversion.
			identityHash, identityErr := tokenCredentialDispatchLinkedIdentity(ctx, db, admission, admission.LinkedRecordID)
			if identityErr != nil || identityHash != admission.LinkedIdentityHash {
				tokenCredentialRotationAddBlocker(&state.blockers, "credential_dispatch_link_unknown", TokenMaintenanceStatusUnknown)
			}
		default:
			tokenCredentialRotationAddBlocker(&state.blockers, "credential_dispatch_admission_unknown", TokenMaintenanceStatusUnknown)
		}
	}
	return tokenCredentialRotationAppendEvidence(state, "credential_dispatch_admissions", revisionRows, len(revisionRows))
}
