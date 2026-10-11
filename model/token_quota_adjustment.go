package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrTokenQuotaAdjustmentConflict       = errors.New("token quota adjustment request conflict")
	ErrTokenQuotaAdjustmentInvalidRequest = errors.New("invalid token quota adjustment request")
)

// TokenQuotaAdjustmentFingerprintVersion identifies the immutable v1 request
// identity. Future writable request fields require a new version; requests
// that omit them must keep using v1 so committed receipts remain replayable.
const TokenQuotaAdjustmentFingerprintVersion = 1

// TokenQuotaAdjustment is the immutable result of a successful explicit Key
// quota edit. It intentionally contains neither the Key secret nor a Token
// snapshot: replay returns this original result alongside the current
// persistent database Token projection, without reading or publishing cache.
type TokenQuotaAdjustment struct {
	ID                 int64  `json:"-" gorm:"primaryKey"`
	UserID             int    `json:"-" gorm:"not null;uniqueIndex:idx_token_quota_adjustment_request,priority:1"`
	RequestIDHash      string `json:"-" gorm:"type:char(64);not null;uniqueIndex:idx_token_quota_adjustment_request,priority:2"`
	RequestID          string `json:"request_id" gorm:"type:varchar(36);not null"`
	TokenID            int    `json:"token_id" gorm:"not null;index"`
	FingerprintVersion int    `json:"-" gorm:"not null"`
	RequestFingerprint string `json:"-" gorm:"type:char(64);not null"`
	RemainQuotaBefore  int64  `json:"remain_quota_before" gorm:"type:bigint;not null"`
	RemainQuotaAfter   int64  `json:"remain_quota_after" gorm:"type:bigint;not null"`
	UsedQuotaSnapshot  int64  `json:"used_quota_snapshot" gorm:"type:bigint;not null"`
	QuotaVersionBefore int64  `json:"-" gorm:"type:bigint;not null"`
	QuotaVersionAfter  int64  `json:"-" gorm:"type:bigint;not null"`
	CreatedTime        int64  `json:"-" gorm:"type:bigint;not null"`
}

func (TokenQuotaAdjustment) TableName() string { return "token_quota_adjustments" }

type TokenQuotaAdjustmentResult struct {
	RequestID          string `json:"request_id"`
	TokenID            int    `json:"token_id"`
	RemainQuotaBefore  int64  `json:"remain_quota_before"`
	RemainQuotaAfter   int64  `json:"remain_quota_after"`
	UsedQuotaSnapshot  int64  `json:"used_quota_snapshot"`
	QuotaVersionBefore string `json:"quota_version_before"`
	QuotaVersionAfter  string `json:"quota_version_after"`
}

type TokenQuotaAdjustmentCommand struct {
	UserID                      int
	RequestID                   string
	FingerprintVersion          int
	RequestFingerprint          string
	ExpectedQuotaVersion        int64
	ExpectedMetadataFingerprint string
}

func (adjustment *TokenQuotaAdjustment) Result() *TokenQuotaAdjustmentResult {
	if adjustment == nil {
		return nil
	}
	return &TokenQuotaAdjustmentResult{
		RequestID:          adjustment.RequestID,
		TokenID:            adjustment.TokenID,
		RemainQuotaBefore:  adjustment.RemainQuotaBefore,
		RemainQuotaAfter:   adjustment.RemainQuotaAfter,
		UsedQuotaSnapshot:  adjustment.UsedQuotaSnapshot,
		QuotaVersionBefore: strconv.FormatInt(adjustment.QuotaVersionBefore, 10),
		QuotaVersionAfter:  strconv.FormatInt(adjustment.QuotaVersionAfter, 10),
	}
}

func canonicalTokenQuotaAdjustmentRequestID(requestID string) (string, error) {
	parsed, err := uuid.Parse(requestID)
	if err != nil || parsed == uuid.Nil || parsed.String() != requestID {
		return "", ErrTokenQuotaAdjustmentInvalidRequest
	}
	return requestID, nil
}

func validTokenQuotaAdjustmentFingerprint(fingerprint string) bool {
	if len(fingerprint) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(fingerprint)
	return err == nil && hex.EncodeToString(decoded) == fingerprint
}

func tokenQuotaAdjustmentRequestHash(requestID string) string {
	digest := sha256.Sum256([]byte(requestID))
	return fmt.Sprintf("%x", digest[:])
}

func normalizeTokenQuotaAdjustmentCommand(command TokenQuotaAdjustmentCommand) (TokenQuotaAdjustmentCommand, string, error) {
	if command.UserID <= 0 || command.FingerprintVersion != TokenQuotaAdjustmentFingerprintVersion ||
		!quotaMutationVersionValid(command.ExpectedQuotaVersion) ||
		!validTokenQuotaAdjustmentFingerprint(command.ExpectedMetadataFingerprint) ||
		!validTokenQuotaAdjustmentFingerprint(command.RequestFingerprint) {
		return command, "", ErrTokenQuotaAdjustmentInvalidRequest
	}
	requestID, err := canonicalTokenQuotaAdjustmentRequestID(command.RequestID)
	if err != nil {
		return command, "", err
	}
	command.RequestID = requestID
	return command, tokenQuotaAdjustmentRequestHash(requestID), nil
}

func findTokenQuotaAdjustment(db *gorm.DB, command TokenQuotaAdjustmentCommand, requestHash string, tokenID int) (*TokenQuotaAdjustment, bool, error) {
	var adjustment TokenQuotaAdjustment
	result := db.Where("user_id = ? AND request_id_hash = ?", command.UserID, requestHash).Limit(1).Find(&adjustment)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	if adjustment.RequestID != command.RequestID || adjustment.TokenID != tokenID ||
		adjustment.FingerprintVersion != command.FingerprintVersion || adjustment.RequestFingerprint != command.RequestFingerprint {
		return nil, false, ErrTokenQuotaAdjustmentConflict
	}
	return &adjustment, true, nil
}

// FindTokenQuotaAdjustmentReplay verifies current ownership before returning a
// committed result. The returned Token is the current persistent database row.
// This deliberately applies neither cache hydration nor current metadata,
// status or quota-version checks, because those may have changed after commit.
func FindTokenQuotaAdjustmentReplay(tokenID int, command TokenQuotaAdjustmentCommand) (*Token, *TokenQuotaAdjustmentResult, bool, error) {
	command, requestHash, err := normalizeTokenQuotaAdjustmentCommand(command)
	if err != nil || tokenID <= 0 {
		if err == nil {
			err = ErrTokenQuotaAdjustmentInvalidRequest
		}
		return nil, nil, false, err
	}
	var current Token
	if err := DB.Where("id = ? AND user_id = ?", tokenID, command.UserID).First(&current).Error; err != nil {
		return nil, nil, false, err
	}
	adjustment, found, err := findTokenQuotaAdjustment(DB, command, requestHash, tokenID)
	if err != nil || !found {
		return &current, nil, false, err
	}
	return &current, adjustment.Result(), true, nil
}
