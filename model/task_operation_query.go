package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

var ErrTaskOperationQueryUnavailable = errors.New("task operation query unavailable")

// TaskOperationQueryRow contains only fields approved for the public owner
// query. Keeping this projection separate prevents internal operation fields
// from being serialized by a controller accidentally.
type TaskOperationQueryRow struct {
	PublicID          string                        `gorm:"column:public_id" json:"-"`
	OperationKind     string                        `gorm:"column:operation_kind" json:"-"`
	Status            TaskSubmissionOperationStatus `gorm:"column:status" json:"-"`
	CreatedAt         int64                         `gorm:"column:created_at" json:"-"`
	UpdatedAt         int64                         `gorm:"column:updated_at" json:"-"`
	DispatchStartedAt *int64                        `gorm:"column:dispatch_started_at" json:"-"`
	ResolvedAt        *int64                        `gorm:"column:resolved_at" json:"-"`
}

// TaskOperationAPIIdentity is the authoritative primary-database view needed
// to authenticate this read-only endpoint. It intentionally excludes the raw
// token key and quota data.
type TaskOperationAPIIdentity struct {
	TokenID     int     `gorm:"column:token_id"`
	UserID      int     `gorm:"column:user_id"`
	TokenStatus int     `gorm:"column:token_status"`
	UserStatus  int     `gorm:"column:user_status"`
	AllowIPs    *string `gorm:"column:allow_ips"`
}

// ReadTaskOperationAPIIdentity looks up both the relay token and its owning
// user from the primary database without consulting or populating Redis.
// Normal GORM scope excludes soft-deleted rows.
func ReadTaskOperationAPIIdentity(key string) (*TaskOperationAPIIdentity, error) {
	if DB == nil {
		return nil, ErrTaskOperationQueryUnavailable
	}
	if strings.TrimSpace(key) == "" {
		return nil, nil
	}
	var identity TaskOperationAPIIdentity
	err := DB.Model(&Token{}).
		Select("tokens.id AS token_id", "tokens.user_id AS user_id", "tokens.status AS token_status", "users.status AS user_status", "tokens.allow_ips AS allow_ips").
		Joins("JOIN users ON users.id = tokens.user_id AND users.deleted_at IS NULL").
		Where(&Token{Key: key}).
		Take(&identity).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, ErrTaskOperationQueryUnavailable
	}
	return &identity, nil
}

// ReadTaskOperationForOwner performs one bounded, read-only primary-database
// query. The public ID and every applicable owner condition are part of that
// same SQL statement, so an ownership mismatch is indistinguishable from an
// absent resource. A nil tokenID denotes a dashboard-session query.
func ReadTaskOperationForOwner(publicID string, userID int, tokenID *int) (*TaskOperationQueryRow, error) {
	publicID = strings.TrimSpace(publicID)
	if DB == nil {
		return nil, ErrTaskOperationQueryUnavailable
	}
	if publicID == "" || len(publicID) > 48 || userID <= 0 || (tokenID != nil && *tokenID <= 0) {
		return nil, nil
	}

	query := DB.Model(&TaskSubmissionOperation{}).
		Select("public_id", "operation_kind", "status", "created_at", "updated_at", "dispatch_started_at", "resolved_at").
		Where("public_id = ? AND user_id = ?", publicID, userID)
	if tokenID != nil {
		query = query.Where("token_id = ?", *tokenID)
	}
	var operation TaskOperationQueryRow
	if err := query.Take(&operation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, ErrTaskOperationQueryUnavailable
	}
	if !validTaskOperationQueryRow(&operation) {
		return nil, ErrTaskOperationQueryUnavailable
	}
	return &operation, nil
}

// validTaskOperationQueryRow checks every durable invariant that can be
// established from the deliberately narrow public projection. It does not
// fetch private recovery, task, billing or owner fields.
func validTaskOperationQueryRow(operation *TaskOperationQueryRow) bool {
	if operation == nil || !validTaskSubmissionPublicID(operation.PublicID) ||
		!validTaskSubmissionOperationKind(operation.OperationKind) ||
		!operation.Status.Valid() || operation.CreatedAt <= 0 || operation.UpdatedAt < operation.CreatedAt ||
		!taskOperationQueryTimestampInBounds(operation.DispatchStartedAt, operation.CreatedAt, operation.UpdatedAt) ||
		!taskOperationQueryTimestampInBounds(operation.ResolvedAt, operation.CreatedAt, operation.UpdatedAt) {
		return false
	}

	requiresDispatch := operation.Status == TaskSubmissionOperationStatusDispatching ||
		operation.Status == TaskSubmissionOperationStatusSubmissionUnknown ||
		operation.Status == TaskSubmissionOperationStatusAccepted ||
		operation.Status == TaskSubmissionOperationStatusOutcomeUnknown ||
		operation.Status == TaskSubmissionOperationStatusSucceeded ||
		operation.Status == TaskSubmissionOperationStatusFailed
	if requiresDispatch && operation.DispatchStartedAt == nil {
		return false
	}
	if (operation.Status == TaskSubmissionOperationStatusPrepared ||
		operation.Status == TaskSubmissionOperationStatusReserved ||
		operation.Status == TaskSubmissionOperationStatusCanceled) && operation.DispatchStartedAt != nil {
		return false
	}
	return operation.Status.Terminal() == (operation.ResolvedAt != nil)
}

func taskOperationQueryTimestampInBounds(timestamp *int64, createdAt, updatedAt int64) bool {
	return timestamp == nil || (*timestamp > 0 && *timestamp >= createdAt && *timestamp <= updatedAt)
}
