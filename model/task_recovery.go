package model

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	taskSubmissionPublicIDRandomLength           = 32
	taskRecoveryDigestLength                     = sha256.Size * 2
	TaskSubmissionTerminalRetentionSeconds int64 = 180 * 24 * 60 * 60
	TaskRecoveryPayloadVersion                   = 1
	taskBillingQuotaMin                    int64 = -(1<<31 - 1)
	taskBillingQuotaMax                    int64 = 1<<31 - 1
	taskBillingAuditCommandIDMaxLength           = 48
	taskRecoveryMaxInt64                   int64 = 1<<63 - 1
	taskRecoveryAttemptCountMax                  = 1<<31 - 1
	// Runtime configuration may choose lower values, but model callers may
	// never exceed these technical bounds.
	TaskRecoveryMaxProcessingLeaseSeconds int64 = 300
	TaskRecoveryMaxRetryDelaySeconds      int64 = 24 * 60 * 60

	TaskSubmissionResolutionSourceProviderVerified = "provider_verified"
	TaskSubmissionResolutionSourceManualAudit      = "manual_audit"

	TaskSubmissionOperationKindVideoCreate = "video.create"
	TaskSubmissionOperationKindVideoRemix  = "video.remix"
	TaskSubmissionOperationKindSunoMusic   = "suno.music"
	TaskSubmissionOperationKindSunoLyrics  = "suno.lyrics"
)

var (
	ErrTaskRecoveryInvalidRecord         = errors.New("invalid task recovery record")
	ErrTaskRecoveryInvalidTransition     = errors.New("invalid task recovery state transition")
	ErrTaskSubmissionOperationNotFound   = errors.New("task submission operation not found")
	ErrTaskSubmissionTaskNotFound        = errors.New("task submission operation task not found")
	ErrTaskSubmissionTaskIDMismatch      = errors.New("task submission operation public id does not match task id")
	ErrTaskSubmissionIdempotencyConflict = errors.New("task submission idempotency key belongs to a different request")
	ErrTaskSubmissionAttemptConflict     = errors.New("task submission attempt belongs to a different immutable request")
	ErrTaskBillingEventNotFound          = errors.New("task billing event not found")
	ErrTaskBillingEventConflict          = errors.New("task billing event key belongs to a different immutable event")
	ErrTaskBillingLogOutboxConflict      = errors.New("task billing log outbox already has a different immutable projection")
	errTaskSubmissionDispatchCASLost     = errors.New("task submission dispatch compare-and-swap lost")
)

const (
	taskRecoveryControlledWriteSetting = "task-recovery:controlled-write"
	taskRecoveryMigrationWriteSetting  = "task-recovery:migration-write"
	taskRecoveryUpdateGuardCallback    = "task-recovery:protect-durable-update"
	taskRecoveryDeleteGuardCallback    = "task-recovery:protect-durable-delete"
)

type taskRecoveryControlledWriteMarkerType struct {
	value byte
}

var taskRecoveryControlledWriteMarker = &taskRecoveryControlledWriteMarkerType{}
var taskRecoveryMigrationWriteMarker = &taskRecoveryControlledWriteMarkerType{value: 1}

var taskRecoverySavepointSequence uint64

var taskRecoveryProtectedTables = []string{
	"task_recovery_identities",
	"task_submission_operations",
	"task_submission_attempts",
	"task_terminal_observations",
	"task_billing_events",
	"task_billing_log_outboxes",
	"quota_mutation_receipts",
	"user_quota_mutation_receipts",
}

// registerTaskRecoveryGormGuards closes the normal GORM Table(...).Update
// and Delete bypass around model hooks and <-:create tags. State CAS helpers
// below hold the private marker while performing their bounded updates. Raw
// SQL remains a privileged migration/test boundary and is never used by the
// task-recovery business write path.
func registerTaskRecoveryGormGuards(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if db.Callback().Update().Get(taskRecoveryUpdateGuardCallback) == nil {
		if err := db.Callback().Update().Before("gorm:update").Register(taskRecoveryUpdateGuardCallback, taskRecoveryGormUpdateGuard); err != nil {
			return err
		}
	}
	if db.Callback().Delete().Get(taskRecoveryDeleteGuardCallback) == nil {
		if err := db.Callback().Delete().Before("gorm:delete").Register(taskRecoveryDeleteGuardCallback, taskRecoveryGormDeleteGuard); err != nil {
			return err
		}
	}
	return nil
}

func taskRecoveryControlledWrite(tx *gorm.DB) *gorm.DB {
	// Set stores its value on Statement.Settings. Always allocate an isolated
	// statement first: a caller may pass a clone==0 chained GORM handle, where
	// Set would otherwise leave the private marker on that handle and authorize
	// a later unrelated Table(...).Update/Delete call.
	return tx.Session(&gorm.Session{NewDB: true}).Set(taskRecoveryControlledWriteSetting, taskRecoveryControlledWriteMarker)
}

func taskRecoveryMigrationWrite(tx *gorm.DB) *gorm.DB {
	return tx.Session(&gorm.Session{NewDB: true}).Set(taskRecoveryMigrationWriteSetting, taskRecoveryMigrationWriteMarker)
}

func taskRecoveryGormUpdateGuard(tx *gorm.DB) {
	if tx == nil {
		return
	}
	table := taskRecoveryGormStatementTable(tx)
	switch table {
	case "task_recovery_identities":
		tx.AddError(ErrTaskRecoveryIdentityImmutable)
		return
	case "quota_mutation_receipts":
		tx.AddError(ErrQuotaMutationReceiptImmutable)
		return
	case "user_quota_mutation_receipts":
		tx.AddError(ErrUserQuotaMutationReceiptImmutable)
		return
	case "task_terminal_observations":
		if taskRecoveryMigrationWriteAllowed(tx) {
			if !validTaskTerminalObservationMigrationUpdate(tx) {
				tx.AddError(fmt.Errorf("%w: terminal observation migration update is not bounded", ErrTaskRecoveryInvalidRecord))
			}
			return
		}
		if !taskRecoveryControlledWriteAllowed(tx) || !validTaskTerminalObservationControlledUpdate(tx) {
			tx.AddError(fmt.Errorf("%w: terminal observations may only change through bounded state CAS", ErrTaskRecoveryInvalidRecord))
		}
		return
	}
	if taskRecoveryControlledWriteAllowed(tx) {
		return
	}
	switch table {
	case "task_submission_operations", "task_submission_attempts", "task_billing_events", "task_billing_log_outboxes":
		tx.AddError(fmt.Errorf("%w: durable task-recovery records may only change through their controlled state CAS", ErrTaskRecoveryInvalidRecord))
	case "":
		if taskRecoveryGormHasDynamicTableExpression(tx) {
			tx.AddError(fmt.Errorf("%w: dynamic table expressions are not permitted for unguarded updates or deletes", ErrTaskRecoveryInvalidRecord))
		}
	}
}

func taskRecoveryGormDeleteGuard(tx *gorm.DB) {
	if tx == nil {
		return
	}
	table := taskRecoveryGormStatementTable(tx)
	switch table {
	case "task_recovery_identities":
		tx.AddError(ErrTaskRecoveryIdentityImmutable)
	case "quota_mutation_receipts":
		tx.AddError(ErrQuotaMutationReceiptImmutable)
	case "user_quota_mutation_receipts":
		tx.AddError(ErrUserQuotaMutationReceiptImmutable)
	case "task_submission_operations", "task_submission_attempts", "task_terminal_observations", "task_billing_events", "task_billing_log_outboxes":
		tx.AddError(fmt.Errorf("%w: durable task-recovery records cannot be deleted", ErrTaskRecoveryInvalidRecord))
	case "":
		if taskRecoveryGormHasDynamicTableExpression(tx) {
			tx.AddError(fmt.Errorf("%w: dynamic table expressions are not permitted for unguarded updates or deletes", ErrTaskRecoveryInvalidRecord))
		}
	}
}

func validTaskTerminalObservationControlledUpdate(tx *gorm.DB) bool {
	updates, ok := tx.Statement.Dest.(map[string]interface{})
	if !ok || len(updates) == 0 {
		return false
	}
	allowed := map[string]struct{}{
		"state": {}, "applied_at": {}, "updated_at": {}, "next_attempt_at": {}, "last_error_code": {}, "lock_version": {}, "attempt_count": {},
		"conflict_count": {}, "last_conflict_fingerprint": {}, "clamp_op": {}, "clamp_kind": {}, "clamp_original": {}, "clamp_clamped": {},
	}
	for column := range updates {
		if _, exists := allowed[column]; !exists {
			return false
		}
	}
	return taskRecoveryWhereHasColumn(tx, "id") && taskRecoveryWhereHasColumn(tx, "lock_version") &&
		(taskRecoveryWhereHasColumn(tx, "state") || taskRecoveryWhereHasColumn(tx, "conflict_count"))
}

func validTaskTerminalObservationMigrationUpdate(tx *gorm.DB) bool {
	updates, ok := tx.Statement.Dest.(map[string]interface{})
	if !ok || len(updates) == 0 {
		return false
	}
	for column := range updates {
		if column != "request_id" && column != "retention_until" {
			return false
		}
	}
	return taskRecoveryWhereHasColumn(tx, "id")
}

func taskRecoveryWhereHasColumn(tx *gorm.DB, column string) bool {
	whereClause, ok := tx.Statement.Clauses["WHERE"]
	if !ok {
		return false
	}
	return taskRecoveryExpressionHasColumn(whereClause.Expression, column)
}

func taskRecoveryExpressionHasColumn(expression clause.Expression, column string) bool {
	switch value := expression.(type) {
	case clause.Where:
		for _, child := range value.Exprs {
			if taskRecoveryExpressionHasColumn(child, column) {
				return true
			}
		}
	case clause.AndConditions:
		for _, child := range value.Exprs {
			if taskRecoveryExpressionHasColumn(child, column) {
				return true
			}
		}
	case clause.OrConditions:
		for _, child := range value.Exprs {
			if taskRecoveryExpressionHasColumn(child, column) {
				return true
			}
		}
	case clause.Expr:
		return taskRecoverySQLMentionsColumn(value.SQL, column)
	case clause.Eq:
		return taskRecoveryColumnName(value.Column) == column
	case clause.IN:
		return taskRecoveryColumnName(value.Column) == column
	}
	return false
}

func taskRecoverySQLMentionsColumn(sqlText, column string) bool {
	column = strings.ToLower(column)
	for _, token := range strings.FieldsFunc(strings.ToLower(sqlText), func(character rune) bool {
		return (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_'
	}) {
		if token == column {
			return true
		}
	}
	return false
}

func taskRecoveryColumnName(value interface{}) string {
	switch column := value.(type) {
	case clause.Column:
		return strings.ToLower(column.Name)
	case string:
		return strings.ToLower(column)
	default:
		return ""
	}
}

func taskRecoveryControlledWriteAllowed(tx *gorm.DB) bool {
	value, ok := tx.Get(taskRecoveryControlledWriteSetting)
	return ok && value == taskRecoveryControlledWriteMarker
}

func taskRecoveryMigrationWriteAllowed(tx *gorm.DB) bool {
	value, ok := tx.Get(taskRecoveryMigrationWriteSetting)
	return ok && value == taskRecoveryMigrationWriteMarker
}

func taskRecoveryGormStatementTable(tx *gorm.DB) string {
	if tx == nil || tx.Statement == nil {
		return ""
	}
	candidates := []string{tx.Statement.Table}
	if tx.Statement.Schema != nil {
		candidates = append(candidates, tx.Statement.Schema.Table)
	}
	if tx.Statement.TableExpr != nil {
		candidates = append(candidates, tx.Statement.TableExpr.SQL)
	}
	for _, candidate := range candidates {
		for _, table := range taskRecoveryProtectedTables {
			if taskRecoveryTableExpressionContains(candidate, table) {
				return table
			}
		}
	}
	return ""
}

func taskRecoveryGormHasDynamicTableExpression(tx *gorm.DB) bool {
	return tx != nil && tx.Statement != nil && tx.Statement.TableExpr != nil && len(tx.Statement.TableExpr.Vars) > 0
}

// taskRecoveryTableExpressionContains deliberately recognizes the protected
// table as a SQL identifier rather than trusting Statement.Table alone. GORM
// replaces Statement.Table with an alias for Table("... AS alias"), while the
// original table remains in TableExpr.SQL.
func taskRecoveryTableExpressionContains(expression, table string) bool {
	expression = strings.ToLower(expression)
	table = strings.ToLower(table)
	for offset := 0; ; {
		index := strings.Index(expression[offset:], table)
		if index < 0 {
			return false
		}
		index += offset
		end := index + len(table)
		if (index == 0 || !taskRecoverySQLIdentifierCharacter(expression[index-1])) &&
			(end == len(expression) || !taskRecoverySQLIdentifierCharacter(expression[end])) {
			return true
		}
		offset = end
	}
}

func taskRecoverySQLIdentifierCharacter(character byte) bool {
	return character >= 'a' && character <= 'z' ||
		character >= '0' && character <= '9' ||
		character == '_'
}

// TaskSubmissionOperationStatus is the durable, client-queryable lifecycle of
// one modern task submission. SubmissionUnknown and OutcomeUnknown are both
// intentionally non-terminal: neither state may expire or trigger an automatic
// resend/refund without a separately audited resolution.
type TaskSubmissionOperationStatus string

const (
	TaskSubmissionOperationStatusPrepared          TaskSubmissionOperationStatus = "prepared"
	TaskSubmissionOperationStatusReserved          TaskSubmissionOperationStatus = "reserved"
	TaskSubmissionOperationStatusDispatching       TaskSubmissionOperationStatus = "dispatching"
	TaskSubmissionOperationStatusAccepted          TaskSubmissionOperationStatus = "accepted"
	TaskSubmissionOperationStatusRejected          TaskSubmissionOperationStatus = "rejected"
	TaskSubmissionOperationStatusSubmissionUnknown TaskSubmissionOperationStatus = "submission_unknown"
	TaskSubmissionOperationStatusOutcomeUnknown    TaskSubmissionOperationStatus = "outcome_unknown"
	TaskSubmissionOperationStatusSucceeded         TaskSubmissionOperationStatus = "succeeded"
	TaskSubmissionOperationStatusFailed            TaskSubmissionOperationStatus = "failed"
	TaskSubmissionOperationStatusCanceled          TaskSubmissionOperationStatus = "canceled"
)

func (status TaskSubmissionOperationStatus) Valid() bool {
	switch status {
	case TaskSubmissionOperationStatusPrepared,
		TaskSubmissionOperationStatusReserved,
		TaskSubmissionOperationStatusDispatching,
		TaskSubmissionOperationStatusAccepted,
		TaskSubmissionOperationStatusRejected,
		TaskSubmissionOperationStatusSubmissionUnknown,
		TaskSubmissionOperationStatusOutcomeUnknown,
		TaskSubmissionOperationStatusSucceeded,
		TaskSubmissionOperationStatusFailed,
		TaskSubmissionOperationStatusCanceled:
		return true
	default:
		return false
	}
}

func (status TaskSubmissionOperationStatus) Terminal() bool {
	switch status {
	case TaskSubmissionOperationStatusRejected,
		TaskSubmissionOperationStatusSucceeded,
		TaskSubmissionOperationStatusFailed,
		TaskSubmissionOperationStatusCanceled:
		return true
	default:
		return false
	}
}

func CanTransitionTaskSubmissionOperation(from, to TaskSubmissionOperationStatus) bool {
	switch from {
	case TaskSubmissionOperationStatusPrepared:
		return to == TaskSubmissionOperationStatusReserved ||
			to == TaskSubmissionOperationStatusRejected ||
			to == TaskSubmissionOperationStatusCanceled
	case TaskSubmissionOperationStatusReserved:
		return to == TaskSubmissionOperationStatusDispatching ||
			to == TaskSubmissionOperationStatusRejected ||
			to == TaskSubmissionOperationStatusCanceled
	case TaskSubmissionOperationStatusDispatching:
		return to == TaskSubmissionOperationStatusAccepted ||
			to == TaskSubmissionOperationStatusRejected ||
			to == TaskSubmissionOperationStatusSubmissionUnknown
	case TaskSubmissionOperationStatusSubmissionUnknown:
		return to == TaskSubmissionOperationStatusAccepted ||
			to == TaskSubmissionOperationStatusRejected
	case TaskSubmissionOperationStatusAccepted:
		return to == TaskSubmissionOperationStatusOutcomeUnknown ||
			to == TaskSubmissionOperationStatusSucceeded ||
			to == TaskSubmissionOperationStatusFailed
	case TaskSubmissionOperationStatusOutcomeUnknown:
		return to == TaskSubmissionOperationStatusSucceeded ||
			to == TaskSubmissionOperationStatusFailed
	default:
		return false
	}
}

// TaskSubmissionOperation is the durable intent created before an upstream
// request can be dispatched. It stores only digests and safe references: raw
// idempotency keys, request bodies, prompts, responses and provider credentials
// must never be placed in this table.
type TaskSubmissionOperation struct {
	ID                 int64                         `json:"id" gorm:"primaryKey"`
	PublicID           string                        `json:"public_id" gorm:"type:varchar(48);not null;uniqueIndex:uidx_task_submission_public_id;<-:create"`
	UserID             int                           `json:"user_id" gorm:"not null;index;<-:create"`
	TokenID            int                           `json:"token_id" gorm:"not null;uniqueIndex:uidx_task_submission_idempotency,priority:1;<-:create"`
	HTTPMethod         string                        `json:"http_method" gorm:"type:varchar(16);not null;uniqueIndex:uidx_task_submission_idempotency,priority:2;<-:create"`
	OperationKind      string                        `json:"operation_kind" gorm:"type:varchar(48);not null;uniqueIndex:uidx_task_submission_idempotency,priority:3;<-:create"`
	IdempotencyKeyHash string                        `json:"-" gorm:"type:char(64);not null;uniqueIndex:uidx_task_submission_idempotency,priority:4;<-:create"`
	RequestFingerprint string                        `json:"-" gorm:"type:char(64);not null;<-:create"`
	RequestID          string                        `json:"request_id" gorm:"type:varchar(64);not null;default:'';<-:create"`
	Status             TaskSubmissionOperationStatus `json:"status" gorm:"type:varchar(32);not null;index:idx_task_submission_status_created,priority:1;<-:create"`
	BillingPreference  string                        `json:"billing_preference,omitempty" gorm:"type:varchar(32);not null;default:'';<-:create"`
	BillingSource      string                        `json:"billing_source,omitempty" gorm:"type:varchar(32);not null;default:'';<-:create"`
	SubscriptionID     int                           `json:"subscription_id,omitempty" gorm:"not null;default:0;<-:create"`
	ReservedQuota      int64                         `json:"reserved_quota,omitempty" gorm:"type:bigint;not null;default:0;<-:create"`
	EstimatedQuota     int64                         `json:"estimated_quota,omitempty" gorm:"type:bigint;not null;default:0;<-:create"`
	BillingVersion     int                           `json:"billing_version,omitempty" gorm:"not null;default:0;<-:create"`
	FreeModel          bool                          `json:"free_model,omitempty" gorm:"not null;default:false;<-:create"`
	TaskID             *int64                        `json:"task_id,omitempty" gorm:"uniqueIndex:uidx_task_submission_task;<-:create"`
	ReasonCode         string                        `json:"reason_code,omitempty" gorm:"type:varchar(64);<-:create"`
	ResolutionSource   string                        `json:"resolution_source,omitempty" gorm:"type:varchar(32);<-:create"`
	DispatchStartedAt  *int64                        `json:"dispatch_started_at,omitempty" gorm:"type:bigint;index;<-:create"`
	ResolvedAt         *int64                        `json:"resolved_at,omitempty" gorm:"type:bigint;<-:create"`
	RetentionUntil     *int64                        `json:"retention_until,omitempty" gorm:"type:bigint;index;<-:create"`
	LockVersion        int64                         `json:"lock_version" gorm:"type:bigint;not null;<-:create"`
	CreatedAt          int64                         `json:"created_at" gorm:"type:bigint;index:idx_task_submission_status_created,priority:2;<-:create"`
	UpdatedAt          int64                         `json:"updated_at" gorm:"type:bigint;<-:create"`
}

func (operation *TaskSubmissionOperation) BeforeCreate(tx *gorm.DB) error {
	if operation == nil {
		return ErrTaskRecoveryInvalidRecord
	}
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	if operation.PublicID == "" {
		publicID, err := GenerateTaskSubmissionOperationPublicID()
		if err != nil {
			return err
		}
		operation.PublicID = publicID
	}
	operation.HTTPMethod = strings.ToUpper(strings.TrimSpace(operation.HTTPMethod))
	operation.OperationKind = strings.ToLower(strings.TrimSpace(operation.OperationKind))
	operation.IdempotencyKeyHash = strings.ToLower(operation.IdempotencyKeyHash)
	operation.RequestFingerprint = strings.ToLower(operation.RequestFingerprint)
	operation.RequestID = strings.TrimSpace(operation.RequestID)
	if operation.RequestID == "" && validTaskRecoveryDigest(operation.RequestFingerprint) {
		operation.RequestID = "taskreq_" + operation.RequestFingerprint[:48]
	}
	if operation.Status == "" {
		operation.Status = TaskSubmissionOperationStatusPrepared
	}
	if operation.Status != TaskSubmissionOperationStatusPrepared || operation.TaskID != nil || operation.ReasonCode != "" || operation.ResolutionSource != "" ||
		operation.BillingPreference != "" || operation.BillingSource != "" || operation.SubscriptionID != 0 || operation.ReservedQuota != 0 || operation.EstimatedQuota != 0 || operation.BillingVersion != 0 || operation.FreeModel ||
		operation.DispatchStartedAt != nil || operation.ResolvedAt != nil || operation.RetentionUntil != nil {
		return fmt.Errorf("%w: new task submission operations must start prepared without a task or retention deadline", ErrTaskRecoveryInvalidRecord)
	}
	if operation.LockVersion == 0 {
		operation.LockVersion = 1
	}
	if operation.LockVersion != 1 {
		return fmt.Errorf("%w: new task submission operation lock version must be one", ErrTaskRecoveryInvalidRecord)
	}
	if operation.CreatedAt != 0 || operation.UpdatedAt != 0 {
		return fmt.Errorf("%w: task submission timestamps are assigned by the database clock", ErrTaskRecoveryInvalidRecord)
	}
	if operation.UserID <= 0 || operation.TokenID <= 0 || int64(operation.UserID) > taskBillingQuotaMax || int64(operation.TokenID) > taskBillingQuotaMax ||
		operation.HTTPMethod == "" || !validTaskSubmissionOperationKind(operation.OperationKind) || len(operation.RequestID) > 64 {
		return fmt.Errorf("%w: task submission scope is incomplete", ErrTaskRecoveryInvalidRecord)
	}
	if !validTaskRecoveryDigest(operation.IdempotencyKeyHash) || !validTaskRecoveryDigest(operation.RequestFingerprint) {
		return fmt.Errorf("%w: task submission digests must be lowercase SHA-256 hex", ErrTaskRecoveryInvalidRecord)
	}
	if !validTaskSubmissionPublicID(operation.PublicID) ||
		len(operation.HTTPMethod) > 16 || len(operation.OperationKind) > 48 {
		return fmt.Errorf("%w: task submission identifier exceeds its database bound", ErrTaskRecoveryInvalidRecord)
	}
	if err := validateTaskSubmissionOperationOwner(tx, operation.UserID, operation.TokenID); err != nil {
		return err
	}
	if err := EnsureTaskRecoveryIdentity(tx); err != nil {
		return err
	}
	now, err := taskRecoveryDBTimestamp(tx)
	if err != nil {
		return err
	}
	operation.CreatedAt = now
	operation.UpdatedAt = now
	return nil
}

func (operation *TaskSubmissionOperation) BeforeUpdate(_ *gorm.DB) error {
	return fmt.Errorf("%w: task submission operations may only change through the state CAS", ErrTaskRecoveryInvalidRecord)
}

func (operation *TaskSubmissionOperation) BeforeDelete(_ *gorm.DB) error {
	return fmt.Errorf("%w: task submission operations require a separate retention cleanup boundary", ErrTaskRecoveryInvalidRecord)
}

func GenerateTaskSubmissionOperationPublicID() (string, error) {
	key, err := common.GenerateRandomCharsKey(taskSubmissionPublicIDRandomLength)
	if err != nil {
		return "", fmt.Errorf("generate task submission public id: %w", err)
	}
	return "task_" + strings.ToLower(key), nil
}

// HashTaskSubmissionIdempotencyKey produces the only representation of a
// client key that may be persisted. The deployment secret prevents low-entropy
// client keys from becoming an offline lookup table if the database is copied.
func HashTaskSubmissionIdempotencyKey(rawKey string) (string, error) {
	if rawKey == "" {
		return "", fmt.Errorf("%w: empty idempotency key", ErrTaskRecoveryInvalidRecord)
	}
	return common.HashTaskRecoveryIdempotencyKey(rawKey)
}

// FingerprintTaskSubmissionRequest hashes an already canonicalized request.
// Canonicalization belongs to the protocol layer; this helper never persists
// or returns the original request bytes.
func FingerprintTaskSubmissionRequest(canonicalRequest []byte) string {
	digest := sha256.Sum256(canonicalRequest)
	return hex.EncodeToString(digest[:])
}

type TaskSubmissionIdempotencyScope struct {
	TokenID            int
	HTTPMethod         string
	OperationKind      string
	IdempotencyKeyHash string
}

func FindTaskSubmissionOperationByIdempotencyScope(tx *gorm.DB, scope TaskSubmissionIdempotencyScope) (*TaskSubmissionOperation, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	scope.HTTPMethod = strings.ToUpper(strings.TrimSpace(scope.HTTPMethod))
	scope.OperationKind = strings.ToLower(strings.TrimSpace(scope.OperationKind))
	scope.IdempotencyKeyHash = strings.ToLower(scope.IdempotencyKeyHash)
	if scope.TokenID <= 0 || int64(scope.TokenID) > taskBillingQuotaMax || scope.HTTPMethod == "" || !validTaskSubmissionOperationKind(scope.OperationKind) || !validTaskRecoveryDigest(scope.IdempotencyKeyHash) {
		return nil, ErrTaskRecoveryInvalidRecord
	}
	var operation TaskSubmissionOperation
	err := tx.Where(
		"token_id = ? AND http_method = ? AND operation_kind = ? AND idempotency_key_hash = ?",
		scope.TokenID,
		scope.HTTPMethod,
		scope.OperationKind,
		scope.IdempotencyKeyHash,
	).First(&operation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &operation, nil
}

// validateTaskSubmissionOperationOwner makes the durable operation's user and
// token references authoritative at creation time. Both lookups intentionally
// use GORM's normal soft-delete scope: a deleted user or token cannot begin a
// new billable operation, while an already persisted operation remains
// queryable through its own durable identity.
func validateTaskSubmissionOperationOwner(tx *gorm.DB, userID, tokenID int) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	var user User
	if err := tx.Select("id").Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: task submission user does not exist", ErrTaskRecoveryInvalidRecord)
		}
		return err
	}
	var token Token
	if err := tx.Select("id", "user_id").Where("id = ? AND user_id = ?", tokenID, userID).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: task submission token does not belong to its user", ErrTaskRecoveryInvalidRecord)
		}
		return err
	}
	return nil
}

func loadTaskSubmissionOperationForReplay(tx *gorm.DB, scope TaskSubmissionIdempotencyScope) (*TaskSubmissionOperation, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	var operation TaskSubmissionOperation
	err := lockForUpdate(tx).Where(
		"token_id = ? AND http_method = ? AND operation_kind = ? AND idempotency_key_hash = ?",
		scope.TokenID,
		scope.HTTPMethod,
		scope.OperationKind,
		scope.IdempotencyKeyHash,
	).First(&operation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &operation, nil
}

// CreateOrLoadTaskSubmissionOperation creates the durable idempotency owner or
// safely returns its existing record. A duplicate scope with a different
// request fingerprint (or owner) is a protocol conflict and never starts a
// second operation. ON CONFLICT DO NOTHING keeps PostgreSQL transactions
// usable for the required post-conflict read.
func CreateOrLoadTaskSubmissionOperation(tx *gorm.DB, candidate *TaskSubmissionOperation) (*TaskSubmissionOperation, error) {
	operation, _, err := createOrLoadTaskSubmissionOperation(tx, candidate)
	return operation, err
}

// createOrLoadTaskSubmissionOperation additionally reports whether this call
// inserted the operation. The public operation-only API intentionally discards
// that fact; only the atomic T0 intent API may use it to authorize subsequent
// work.
func createOrLoadTaskSubmissionOperation(tx *gorm.DB, candidate *TaskSubmissionOperation) (*TaskSubmissionOperation, bool, error) {
	if tx == nil {
		return nil, false, gorm.ErrInvalidDB
	}
	if candidate == nil {
		return nil, false, ErrTaskRecoveryInvalidRecord
	}
	if candidate.ID != 0 || candidate.PublicID != "" {
		// The owner proof below intentionally compares a newly generated public
		// ID with the stored row. Accepting a caller-provided persisted ID or
		// public ID would let MySQL's client-found-rows mode misclassify a
		// duplicate ON DUPLICATE KEY UPDATE as a new owner.
		return nil, false, fmt.Errorf("%w: task submission operation candidates must not carry a persisted identifier", ErrTaskRecoveryInvalidRecord)
	}
	writeDB := tx.Session(&gorm.Session{NewDB: true})
	record := *candidate
	publicID, err := GenerateTaskSubmissionOperationPublicID()
	if err != nil {
		return nil, false, err
	}
	record.PublicID = publicID
	createResult := writeDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	if createResult.Error != nil {
		return nil, false, createResult.Error
	}
	existing, err := loadTaskSubmissionOperationForReplay(writeDB, TaskSubmissionIdempotencyScope{
		TokenID:            record.TokenID,
		HTTPMethod:         record.HTTPMethod,
		OperationKind:      record.OperationKind,
		IdempotencyKeyHash: record.IdempotencyKeyHash,
	})
	if err != nil {
		return nil, false, err
	}
	if existing == nil {
		// A collision on the independently generated public ID must never be
		// mistaken for an idempotency replay.
		return nil, false, ErrTaskSubmissionIdempotencyConflict
	}
	if err := validateStoredTaskSubmissionOperation(writeDB, existing); err != nil {
		return existing, false, err
	}
	if existing.UserID != record.UserID || existing.RequestFingerprint != record.RequestFingerprint {
		return existing, false, ErrTaskSubmissionIdempotencyConflict
	}
	created := createResult.RowsAffected == 1 && record.ID > 0 &&
		existing.ID == record.ID && existing.PublicID == record.PublicID
	return existing, created, nil
}

func GetTaskSubmissionOperationByPublicID(tx *gorm.DB, publicID string) (*TaskSubmissionOperation, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return nil, ErrTaskRecoveryInvalidRecord
	}
	var operation TaskSubmissionOperation
	err := tx.Where("public_id = ?", publicID).First(&operation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &operation, nil
}

type TaskSubmissionOperationTransition struct {
	From             TaskSubmissionOperationStatus
	To               TaskSubmissionOperationStatus
	TaskID           *int64
	ReasonCode       string
	ResolutionSource string
	AuditCommandID   string
	TransitionedAt   int64
	RetentionUntil   *int64
	ExpectedVersion  int64
}

// TransitionTaskSubmissionOperation applies one permitted transition with a
// status CAS. An accepted transition requires an already-created formal Task
// whose public TaskID equals the operation PublicID; this API never creates a
// placeholder task.
func TransitionTaskSubmissionOperation(tx *gorm.DB, operationID int64, transition TaskSubmissionOperationTransition) (bool, error) {
	if tx == nil {
		return false, gorm.ErrInvalidDB
	}
	if operationID <= 0 || transition.ExpectedVersion <= 0 || transition.ExpectedVersion == taskRecoveryMaxInt64 ||
		!CanTransitionTaskSubmissionOperation(transition.From, transition.To) || transition.To == TaskSubmissionOperationStatusDispatching {
		return false, ErrTaskRecoveryInvalidTransition
	}
	transition.ReasonCode = strings.TrimSpace(transition.ReasonCode)
	transition.ResolutionSource = strings.TrimSpace(transition.ResolutionSource)
	transition.AuditCommandID = strings.TrimSpace(transition.AuditCommandID)
	if len(transition.ReasonCode) > 64 || len(transition.ResolutionSource) > 32 || len(transition.AuditCommandID) > taskBillingAuditCommandIDMaxLength {
		return false, fmt.Errorf("%w: transition audit code exceeds its database bound", ErrTaskRecoveryInvalidRecord)
	}
	if !transition.To.Terminal() && transition.RetentionUntil != nil {
		return false, fmt.Errorf("%w: active task submission operations cannot expire", ErrTaskRecoveryInvalidRecord)
	}
	if transition.ResolutionSource == TaskSubmissionResolutionSourceManualAudit {
		if !validTaskBillingAuditCommandID(transition.AuditCommandID) {
			return false, fmt.Errorf("%w: manual task resolution requires a valid audit command id", ErrTaskRecoveryInvalidRecord)
		}
	}
	resolvesUnknown := transition.From == TaskSubmissionOperationStatusSubmissionUnknown || transition.From == TaskSubmissionOperationStatusOutcomeUnknown
	if resolvesUnknown && transition.ResolutionSource != TaskSubmissionResolutionSourceProviderVerified && transition.ResolutionSource != TaskSubmissionResolutionSourceManualAudit {
		return false, fmt.Errorf("%w: unknown task state requires provider_verified or manual_audit resolution", ErrTaskRecoveryInvalidRecord)
	}
	if transition.ResolutionSource != "" && transition.ResolutionSource != TaskSubmissionResolutionSourceProviderVerified && transition.ResolutionSource != TaskSubmissionResolutionSourceManualAudit {
		return false, fmt.Errorf("%w: unsupported task resolution source", ErrTaskRecoveryInvalidRecord)
	}
	transitionedAt, err := taskRecoveryValidatedTime(tx, transition.TransitionedAt)
	if err != nil {
		return false, err
	}
	transition.TransitionedAt = transitionedAt

	updates := map[string]interface{}{
		"status":            transition.To,
		"reason_code":       transition.ReasonCode,
		"resolution_source": transition.ResolutionSource,
		"updated_at":        transition.TransitionedAt,
		"lock_version":      gorm.Expr("lock_version + ?", 1),
	}
	if transition.To.Terminal() {
		if transitionedAt > taskRecoveryMaxInt64-TaskSubmissionTerminalRetentionSeconds {
			return false, fmt.Errorf("%w: terminal task submission retention timestamp overflows", ErrTaskRecoveryInvalidRecord)
		}
		retentionUntil := transitionedAt + TaskSubmissionTerminalRetentionSeconds
		if transition.RetentionUntil != nil && *transition.RetentionUntil != retentionUntil {
			return false, fmt.Errorf("%w: terminal task submission retention must be exactly 180 days", ErrTaskRecoveryInvalidRecord)
		}
		updates["resolved_at"] = transitionedAt
		updates["retention_until"] = retentionUntil
	}
	if transition.To == TaskSubmissionOperationStatusAccepted {
		if transition.TaskID == nil || *transition.TaskID <= 0 {
			return false, fmt.Errorf("%w: accepted operation requires a formal task", ErrTaskRecoveryInvalidRecord)
		}
		var operation TaskSubmissionOperation
		if err := tx.Select("id", "public_id", "user_id", "status", "lock_version").Where("id = ?", operationID).First(&operation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		if operation.Status != transition.From || operation.LockVersion != transition.ExpectedVersion {
			return false, nil
		}
		var task Task
		if err := tx.Select("id", "task_id", "user_id").Where("id = ?", *transition.TaskID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, ErrTaskSubmissionTaskNotFound
			}
			return false, err
		}
		if task.TaskID != operation.PublicID {
			return false, ErrTaskSubmissionTaskIDMismatch
		}
		if task.UserId != operation.UserID {
			return false, fmt.Errorf("%w: formal task owner does not match its submission operation", ErrTaskRecoveryInvalidRecord)
		}
		updates["task_id"] = *transition.TaskID
	} else if transition.TaskID != nil {
		return false, fmt.Errorf("%w: only acceptance can attach a task", ErrTaskRecoveryInvalidRecord)
	}

	result := taskRecoveryControlledWrite(tx).Table("task_submission_operations").
		Where("id = ? AND status = ? AND lock_version = ? AND updated_at <= ?", operationID, transition.From, transition.ExpectedVersion, transitionedAt).
		Updates(updates)
	return result.RowsAffected == 1, result.Error
}

// TaskSubmissionDispatchTransition is the one atomic v1 hand-off from a
// reserved operation and its prepared sole attempt to dispatching. The same
// database timestamp is written to both records so a crash cannot create an
// operation that appears delivered without its dispatch authorization.
type TaskSubmissionDispatchTransition struct {
	ExpectedOperationVersion int64
	ExpectedAttemptVersion   int64
	TransitionedAt           int64
}

// StartTaskSubmissionDispatch atomically starts the single v1 dispatch. The
// generic operation and attempt transition APIs intentionally reject a move to
// dispatching so callers cannot split this critical transition across writes.
func StartTaskSubmissionDispatch(tx *gorm.DB, operationID int64, transition TaskSubmissionDispatchTransition) (bool, error) {
	if tx == nil {
		return false, gorm.ErrInvalidDB
	}
	if operationID <= 0 || transition.ExpectedOperationVersion <= 0 || transition.ExpectedAttemptVersion <= 0 ||
		transition.ExpectedOperationVersion == taskRecoveryMaxInt64 || transition.ExpectedAttemptVersion == taskRecoveryMaxInt64 {
		return false, ErrTaskRecoveryInvalidTransition
	}

	won := false
	err := taskRecoveryDispatchTransaction(tx, func(writeDB *gorm.DB) error {
		transitionedAt, err := taskRecoveryValidatedTime(writeDB, transition.TransitionedAt)
		if err != nil {
			return err
		}

		var operation TaskSubmissionOperation
		err = lockForUpdate(writeDB).Where("id = ?", operationID).First(&operation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errTaskSubmissionDispatchCASLost
		}
		if err != nil {
			return err
		}
		if err := validateStoredTaskSubmissionOperation(writeDB, &operation); err != nil {
			return err
		}
		if operation.Status != TaskSubmissionOperationStatusReserved || operation.LockVersion != transition.ExpectedOperationVersion {
			return errTaskSubmissionDispatchCASLost
		}

		var attempt TaskSubmissionAttempt
		err = lockForUpdate(writeDB).Where("operation_id = ?", operationID).First(&attempt).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: reserved task submission operation is missing its v1 attempt", ErrTaskRecoveryInvalidRecord)
		}
		if err != nil {
			return err
		}
		if err := validateStoredTaskSubmissionAttempt(writeDB, &attempt); err != nil {
			return err
		}
		if attempt.Status != TaskSubmissionAttemptStatusPrepared || attempt.LockVersion != transition.ExpectedAttemptVersion {
			return errTaskSubmissionDispatchCASLost
		}

		operationResult := taskRecoveryControlledWrite(writeDB).Table("task_submission_operations").
			Where("id = ? AND status = ? AND lock_version = ? AND updated_at <= ?", operation.ID, TaskSubmissionOperationStatusReserved, transition.ExpectedOperationVersion, transitionedAt).
			Updates(map[string]interface{}{
				"status":              TaskSubmissionOperationStatusDispatching,
				"dispatch_started_at": transitionedAt,
				"updated_at":          transitionedAt,
				"lock_version":        gorm.Expr("lock_version + ?", 1),
			})
		if operationResult.Error != nil {
			return operationResult.Error
		}
		if operationResult.RowsAffected != 1 {
			return errTaskSubmissionDispatchCASLost
		}

		attemptResult := taskRecoveryControlledWrite(writeDB).Table("task_submission_attempts").
			Where("id = ? AND operation_id = ? AND status = ? AND lock_version = ? AND updated_at <= ?", attempt.ID, operation.ID, TaskSubmissionAttemptStatusPrepared, transition.ExpectedAttemptVersion, transitionedAt).
			Updates(map[string]interface{}{
				"status":       TaskSubmissionAttemptStatusDispatching,
				"started_at":   transitionedAt,
				"updated_at":   transitionedAt,
				"lock_version": gorm.Expr("lock_version + ?", 1),
			})
		if attemptResult.Error != nil {
			return attemptResult.Error
		}
		if attemptResult.RowsAffected != 1 {
			return errTaskSubmissionDispatchCASLost
		}
		won = true
		return nil
	})
	if errors.Is(err, errTaskSubmissionDispatchCASLost) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return won, nil
}

// taskRecoveryDispatchTransaction retains the all-or-nothing dispatch
// invariant when a caller has deliberately disabled GORM nested transactions.
// In that case GORM would otherwise invoke the callback directly inside the
// caller's transaction, allowing a CAS-loss sentinel to be swallowed after
// the operation update but before the attempt update.
func taskRecoveryDispatchTransaction(tx *gorm.DB, fn func(*gorm.DB) error) (err error) {
	return taskRecoveryAtomicTransaction(tx, "task_recovery_dispatch", fn)
}

type TaskSubmissionAttemptStatus string

const (
	TaskSubmissionAttemptStatusPrepared          TaskSubmissionAttemptStatus = "prepared"
	TaskSubmissionAttemptStatusDispatching       TaskSubmissionAttemptStatus = "dispatching"
	TaskSubmissionAttemptStatusAccepted          TaskSubmissionAttemptStatus = "accepted"
	TaskSubmissionAttemptStatusRejected          TaskSubmissionAttemptStatus = "rejected"
	TaskSubmissionAttemptStatusSubmissionUnknown TaskSubmissionAttemptStatus = "submission_unknown"
)

func (status TaskSubmissionAttemptStatus) Valid() bool {
	switch status {
	case TaskSubmissionAttemptStatusPrepared,
		TaskSubmissionAttemptStatusDispatching,
		TaskSubmissionAttemptStatusAccepted,
		TaskSubmissionAttemptStatusRejected,
		TaskSubmissionAttemptStatusSubmissionUnknown:
		return true
	default:
		return false
	}
}

func CanTransitionTaskSubmissionAttempt(from, to TaskSubmissionAttemptStatus) bool {
	switch from {
	case TaskSubmissionAttemptStatusPrepared:
		return to == TaskSubmissionAttemptStatusDispatching
	case TaskSubmissionAttemptStatusDispatching:
		return to == TaskSubmissionAttemptStatusAccepted ||
			to == TaskSubmissionAttemptStatusRejected ||
			to == TaskSubmissionAttemptStatusSubmissionUnknown
	case TaskSubmissionAttemptStatusSubmissionUnknown:
		return to == TaskSubmissionAttemptStatusAccepted || to == TaskSubmissionAttemptStatusRejected
	default:
		return false
	}
}

// TaskSubmissionAttempt contains only routing classification and stable
// upstream references. Raw upstream requests/responses and credentials are
// prohibited. V1 creates one attempt and forbids automatic redispatch after it
// reaches Dispatching; OperationID is unique because v1 permits exactly one.
type TaskSubmissionAttempt struct {
	ID                  int64                       `json:"id" gorm:"primaryKey"`
	OperationID         int64                       `json:"operation_id" gorm:"not null;uniqueIndex:uidx_task_submission_attempt;<-:create"`
	AttemptNo           int                         `json:"attempt_no" gorm:"not null;<-:create"`
	Status              TaskSubmissionAttemptStatus `json:"status" gorm:"type:varchar(32);not null;index;<-:create"`
	ChannelID           int                         `json:"channel_id" gorm:"not null;index;<-:create"`
	Provider            string                      `json:"provider" gorm:"type:varchar(64);not null;<-:create"`
	RequestClass        string                      `json:"request_class" gorm:"type:varchar(48);not null;<-:create"`
	ProviderOperationID string                      `json:"provider_operation_id,omitempty" gorm:"type:varchar(191);<-:create"`
	UpstreamRequestID   string                      `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);<-:create"`
	TaskPlatform        string                      `json:"task_platform,omitempty" gorm:"type:varchar(30);not null;default:'';<-:create"`
	TaskAction          string                      `json:"task_action,omitempty" gorm:"type:varchar(40);not null;default:'';<-:create"`
	OutcomeCode         string                      `json:"outcome_code,omitempty" gorm:"type:varchar(64);<-:create"`
	StartedAt           *int64                      `json:"started_at,omitempty" gorm:"type:bigint;<-:create"`
	FinishedAt          *int64                      `json:"finished_at,omitempty" gorm:"type:bigint;<-:create"`
	LockVersion         int64                       `json:"lock_version" gorm:"type:bigint;not null;<-:create"`
	CreatedAt           int64                       `json:"created_at" gorm:"type:bigint;index;<-:create"`
	UpdatedAt           int64                       `json:"updated_at" gorm:"type:bigint;<-:create"`
}

func (attempt *TaskSubmissionAttempt) BeforeCreate(tx *gorm.DB) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	if err := normalizeTaskSubmissionAttemptForCreate(attempt); err != nil {
		return err
	}
	var operation TaskSubmissionOperation
	if err := tx.Select("id", "status", "task_id", "lock_version").Where("id = ?", attempt.OperationID).First(&operation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskSubmissionOperationNotFound
		}
		return err
	}
	if err := validateTaskSubmissionAttemptCreationOperation(&operation); err != nil {
		return err
	}
	now, err := taskRecoveryDBTimestamp(tx)
	if err != nil {
		return err
	}
	attempt.CreatedAt = now
	attempt.UpdatedAt = now
	return nil
}

func normalizeTaskSubmissionAttemptForCreate(attempt *TaskSubmissionAttempt) error {
	if attempt == nil {
		return ErrTaskRecoveryInvalidRecord
	}
	if attempt.Status == "" {
		attempt.Status = TaskSubmissionAttemptStatusPrepared
	}
	attempt.Provider = strings.ToLower(strings.TrimSpace(attempt.Provider))
	attempt.RequestClass = strings.ToLower(strings.TrimSpace(attempt.RequestClass))
	if attempt.OperationID <= 0 || attempt.AttemptNo != 1 || attempt.ChannelID <= 0 || int64(attempt.ChannelID) > taskBillingQuotaMax ||
		attempt.Provider == "" || attempt.RequestClass == "" || attempt.Status != TaskSubmissionAttemptStatusPrepared {
		return fmt.Errorf("%w: new task submission attempt is incomplete or not prepared", ErrTaskRecoveryInvalidRecord)
	}
	if attempt.ProviderOperationID != "" || attempt.UpstreamRequestID != "" || attempt.TaskPlatform != "" || attempt.TaskAction != "" || attempt.OutcomeCode != "" || attempt.StartedAt != nil || attempt.FinishedAt != nil {
		return fmt.Errorf("%w: a prepared task submission attempt cannot contain an upstream outcome", ErrTaskRecoveryInvalidRecord)
	}
	if len(attempt.Provider) > 64 || len(attempt.RequestClass) > 48 {
		return fmt.Errorf("%w: task submission attempt classification exceeds its database bound", ErrTaskRecoveryInvalidRecord)
	}
	if attempt.LockVersion == 0 {
		attempt.LockVersion = 1
	}
	if attempt.LockVersion != 1 {
		return fmt.Errorf("%w: new task submission attempt lock version must be one", ErrTaskRecoveryInvalidRecord)
	}
	if attempt.CreatedAt != 0 || attempt.UpdatedAt != 0 {
		return fmt.Errorf("%w: task submission attempt timestamps are assigned by the database clock", ErrTaskRecoveryInvalidRecord)
	}
	return nil
}

func validateTaskSubmissionAttemptCreationOperation(operation *TaskSubmissionOperation) error {
	if operation == nil || operation.Status != TaskSubmissionOperationStatusPrepared || operation.TaskID != nil || operation.LockVersion != 1 {
		return fmt.Errorf("%w: v1 attempts may only be created for a new prepared operation", ErrTaskRecoveryInvalidRecord)
	}
	return nil
}

func findTaskSubmissionAttemptByOperationID(tx *gorm.DB, operationID int64) (*TaskSubmissionAttempt, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	if operationID <= 0 {
		return nil, ErrTaskRecoveryInvalidRecord
	}
	var attempt TaskSubmissionAttempt
	err := tx.Where("operation_id = ?", operationID).First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &attempt, nil
}

func loadTaskSubmissionAttemptForReplay(tx *gorm.DB, operationID int64) (*TaskSubmissionAttempt, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	if operationID <= 0 {
		return nil, ErrTaskRecoveryInvalidRecord
	}
	var attempt TaskSubmissionAttempt
	err := lockForUpdate(tx).Where("operation_id = ?", operationID).First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &attempt, nil
}

// CreateOrLoadTaskSubmissionAttempt records the one v1 dispatch authorization
// for an operation. It first checks an existing row without locking a missing
// unique key, then serializes creation through the operation row. This avoids
// MySQL gap-lock deadlocks while still making PostgreSQL conflict recovery safe.
func CreateOrLoadTaskSubmissionAttempt(tx *gorm.DB, candidate *TaskSubmissionAttempt) (*TaskSubmissionAttempt, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	if candidate == nil {
		return nil, ErrTaskRecoveryInvalidRecord
	}
	writeDB := tx.Session(&gorm.Session{NewDB: true})
	record := *candidate
	if err := normalizeTaskSubmissionAttemptForCreate(&record); err != nil {
		return nil, err
	}
	if existing, err := findTaskSubmissionAttemptByOperationID(writeDB, record.OperationID); err != nil {
		return nil, err
	} else if existing != nil {
		if err := validateStoredTaskSubmissionAttempt(writeDB, existing); err != nil {
			return existing, err
		}
		if !taskSubmissionAttemptSemanticallyMatches(existing, &record) {
			return existing, ErrTaskSubmissionAttemptConflict
		}
		return existing, nil
	}

	var operation TaskSubmissionOperation
	if err := lockForUpdate(writeDB).Select("id", "status", "task_id", "lock_version").Where("id = ?", record.OperationID).First(&operation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTaskSubmissionOperationNotFound
		}
		return nil, err
	}
	if existing, err := findTaskSubmissionAttemptByOperationID(writeDB, record.OperationID); err != nil {
		return nil, err
	} else if existing != nil {
		if err := validateStoredTaskSubmissionAttempt(writeDB, existing); err != nil {
			return existing, err
		}
		if !taskSubmissionAttemptSemanticallyMatches(existing, &record) {
			return existing, ErrTaskSubmissionAttemptConflict
		}
		return existing, nil
	}
	if err := validateTaskSubmissionAttemptCreationOperation(&operation); err != nil {
		return nil, err
	}
	now, err := taskRecoveryDBTimestamp(writeDB)
	if err != nil {
		return nil, err
	}
	record.CreatedAt = now
	record.UpdatedAt = now
	if err := writeDB.Session(&gorm.Session{SkipHooks: true}).Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
		return nil, err
	}
	existing, err := loadTaskSubmissionAttemptForReplay(writeDB, record.OperationID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrTaskSubmissionAttemptConflict
	}
	if err := validateStoredTaskSubmissionAttempt(writeDB, existing); err != nil {
		return existing, err
	}
	if !taskSubmissionAttemptSemanticallyMatches(existing, &record) {
		return existing, ErrTaskSubmissionAttemptConflict
	}
	return existing, nil
}

// TaskSubmissionIntentResult is the atomic T0 snapshot returned before any
// reserve or upstream dispatch. Owner is true only when this call inserted
// both the operation and its sole initial attempt.
type TaskSubmissionIntentResult struct {
	Operation *TaskSubmissionOperation
	Attempt   *TaskSubmissionAttempt
	Owner     bool
}

// CreateOrLoadTaskSubmissionIntent atomically creates an operation and its
// sole v1 attempt, or returns the already-persisted pair for an idempotent
// replay. Once an operation exists, its persisted attempt is authoritative:
// the current routing candidate is deliberately ignored so routing drift does
// not turn a client replay into an attempt conflict.
//
// A replay whose operation is missing its attempt is rejected as an invalid
// durable record. Choosing a new channel at that point would invent history
// and cannot safely repair a submission created outside this transaction.
func CreateOrLoadTaskSubmissionIntent(tx *gorm.DB, operationCandidate *TaskSubmissionOperation, attemptCandidate *TaskSubmissionAttempt) (*TaskSubmissionIntentResult, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	if operationCandidate == nil || attemptCandidate == nil {
		return nil, ErrTaskRecoveryInvalidRecord
	}

	var result *TaskSubmissionIntentResult
	var conflictResult *TaskSubmissionIntentResult
	err := taskSubmissionIntentTransaction(tx, func(writeDB *gorm.DB) error {
		operation, created, err := createOrLoadTaskSubmissionOperation(writeDB, operationCandidate)
		if err != nil {
			if operation != nil && errors.Is(err, ErrTaskSubmissionIdempotencyConflict) {
				conflictResult = &TaskSubmissionIntentResult{Operation: operation}
			}
			return err
		}

		if !created {
			attempt, err := loadTaskSubmissionAttemptForReplay(writeDB, operation.ID)
			if err != nil {
				return err
			}
			if attempt == nil {
				return fmt.Errorf("%w: task submission operation is missing its atomic v1 attempt", ErrTaskRecoveryInvalidRecord)
			}
			if err := validateStoredTaskSubmissionAttempt(writeDB, attempt); err != nil {
				return err
			}
			result = &TaskSubmissionIntentResult{Operation: operation, Attempt: attempt}
			return nil
		}

		attemptRecord := *attemptCandidate
		attemptRecord.OperationID = operation.ID
		if err := writeDB.Create(&attemptRecord).Error; err != nil {
			return err
		}
		attempt, err := loadTaskSubmissionAttemptForReplay(writeDB, operation.ID)
		if err != nil {
			return err
		}
		if attempt == nil || attemptRecord.ID <= 0 || attempt.ID != attemptRecord.ID ||
			!taskSubmissionAttemptSemanticallyMatches(attempt, &attemptRecord) {
			return ErrTaskSubmissionAttemptConflict
		}
		if err := validateStoredTaskSubmissionAttempt(writeDB, attempt); err != nil {
			return err
		}
		result = &TaskSubmissionIntentResult{Operation: operation, Attempt: attempt, Owner: true}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrTaskSubmissionIdempotencyConflict) {
			return conflictResult, err
		}
		return nil, err
	}
	return result, nil
}

// taskSubmissionIntentTransaction preserves T0 all-or-nothing behavior even
// when a caller has disabled GORM nested transactions and then handles the
// returned error without aborting its outer transaction.
func taskSubmissionIntentTransaction(tx *gorm.DB, fn func(*gorm.DB) error) (err error) {
	return taskRecoveryAtomicTransaction(tx, "task_submission_intent", fn)
}

// taskRecoveryAtomicTransaction uses a direct, generated savepoint for an
// existing transaction. The GORM PostgreSQL and glebarez SQLite SavePoint
// dialect methods discard Exec errors, so this must not call tx.SavePoint or
// tx.RollbackTo when a caller may otherwise catch an error and commit the
// outer transaction. SAVEPOINT and ROLLBACK TO SAVEPOINT are supported by all
// three project database dialects, and savepoint names are generated solely
// from a fixed prefix plus a monotonic decimal counter.
func taskRecoveryAtomicTransaction(tx *gorm.DB, prefix string, fn func(*gorm.DB) error) (err error) {
	if tx == nil || tx.Statement == nil {
		return gorm.ErrInvalidDB
	}
	if _, alreadyTransactional := tx.Statement.ConnPool.(gorm.TxCommitter); !alreadyTransactional {
		return tx.Transaction(fn)
	}

	savepoint := fmt.Sprintf("%s_%d", prefix, atomic.AddUint64(&taskRecoverySavepointSequence, 1))
	if err = taskRecoveryTransactionControl(tx, "SAVEPOINT "+savepoint); err != nil {
		return err
	}
	panicked := true
	defer func() {
		if !panicked && err == nil {
			return
		}
		rollbackErr := taskRecoveryTransactionControl(tx, "ROLLBACK TO SAVEPOINT "+savepoint)
		if rollbackErr == nil {
			return
		}
		// If the savepoint could not be rolled back, invalidate the outer
		// transaction rather than leaving a partial operation/attempt pair
		// committable. Preserve an in-flight panic after performing this best
		// effort outer rollback; a recovered caller then observes Commit fail.
		outerRollbackErr := tx.Rollback().Error
		if panicked {
			return
		}
		if outerRollbackErr != nil {
			err = fmt.Errorf("%w: rollback durable task-recovery savepoint: %v; rollback outer transaction: %v", err, rollbackErr, outerRollbackErr)
			return
		}
		err = fmt.Errorf("%w: rollback durable task-recovery savepoint: %v", err, rollbackErr)
	}()
	err = fn(tx.Session(&gorm.Session{NewDB: true}))
	panicked = false
	return err
}

// taskRecoveryTransactionControl executes a generated transaction-control
// statement without GORM's prepared-statement wrapper. MySQL does not permit
// SAVEPOINT or ROLLBACK TO SAVEPOINT through the prepared-statement protocol;
// the clone keeps ordinary business statements on the caller's prepared pool
// while using the underlying transaction only for this checked control SQL.
func taskRecoveryTransactionControl(tx *gorm.DB, statement string) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	if tx.Statement == nil {
		return gorm.ErrInvalidDB
	}
	requestContext := tx.Statement.Context
	if requestContext == nil {
		requestContext = context.Background()
	}
	// A Context makes GORM clone Statement immediately. NewDB alone retains
	// the original Statement until Exec calls getInstance, so changing the
	// ConnPool here would otherwise turn the caller's outer transaction from a
	// PreparedStmtTX into its raw transaction permanently.
	controlDB := tx.Session(&gorm.Session{NewDB: true, Context: requestContext})
	if controlDB == nil || controlDB.Statement == nil {
		return gorm.ErrInvalidDB
	}
	if preparedTx, ok := controlDB.Statement.ConnPool.(*gorm.PreparedStmtTX); ok {
		if preparedTx == nil || preparedTx.Tx == nil {
			return gorm.ErrInvalidTransaction
		}
		controlDB.Statement.ConnPool = preparedTx.Tx
	}
	return controlDB.Exec(statement).Error
}

func taskSubmissionAttemptSemanticallyMatches(existing, candidate *TaskSubmissionAttempt) bool {
	return existing != nil && candidate != nil &&
		existing.OperationID == candidate.OperationID &&
		existing.AttemptNo == candidate.AttemptNo &&
		existing.ChannelID == candidate.ChannelID &&
		existing.Provider == candidate.Provider &&
		existing.RequestClass == candidate.RequestClass
}

func (attempt *TaskSubmissionAttempt) BeforeUpdate(_ *gorm.DB) error {
	return fmt.Errorf("%w: task submission attempts may only change through the state CAS", ErrTaskRecoveryInvalidRecord)
}

func (attempt *TaskSubmissionAttempt) BeforeDelete(_ *gorm.DB) error {
	return fmt.Errorf("%w: task submission attempts are immutable audit records", ErrTaskRecoveryInvalidRecord)
}

type TaskSubmissionAttemptTransition struct {
	From                TaskSubmissionAttemptStatus
	To                  TaskSubmissionAttemptStatus
	ProviderOperationID string
	UpstreamRequestID   string
	TaskPlatform        string
	TaskAction          string
	OutcomeCode         string
	ExpectedVersion     int64
	TransitionedAt      int64
}

func TransitionTaskSubmissionAttempt(tx *gorm.DB, attemptID int64, transition TaskSubmissionAttemptTransition) (bool, error) {
	if tx == nil {
		return false, gorm.ErrInvalidDB
	}
	if attemptID <= 0 || transition.ExpectedVersion <= 0 || transition.ExpectedVersion == taskRecoveryMaxInt64 ||
		!CanTransitionTaskSubmissionAttempt(transition.From, transition.To) || transition.To == TaskSubmissionAttemptStatusDispatching {
		return false, ErrTaskRecoveryInvalidTransition
	}
	transition.ProviderOperationID = strings.TrimSpace(transition.ProviderOperationID)
	transition.UpstreamRequestID = strings.TrimSpace(transition.UpstreamRequestID)
	transition.TaskPlatform = strings.ToLower(strings.TrimSpace(transition.TaskPlatform))
	transition.TaskAction = strings.TrimSpace(transition.TaskAction)
	transition.OutcomeCode = strings.TrimSpace(transition.OutcomeCode)
	if len(transition.ProviderOperationID) > 191 || len(transition.UpstreamRequestID) > 128 || len(transition.TaskPlatform) > 30 || len(transition.TaskAction) > 40 || len(transition.OutcomeCode) > 64 {
		return false, fmt.Errorf("%w: attempt upstream reference or outcome code exceeds its database bound", ErrTaskRecoveryInvalidRecord)
	}
	transitionedAt, err := taskRecoveryValidatedTime(tx, transition.TransitionedAt)
	if err != nil {
		return false, err
	}
	transition.TransitionedAt = transitionedAt

	var attempt TaskSubmissionAttempt
	if err := tx.Select("id", "status", "provider_operation_id", "upstream_request_id", "lock_version").Where("id = ?", attemptID).First(&attempt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if attempt.Status != transition.From || attempt.LockVersion != transition.ExpectedVersion {
		return false, nil
	}

	updates := map[string]interface{}{
		"status":       transition.To,
		"outcome_code": transition.OutcomeCode,
		"updated_at":   transition.TransitionedAt,
		"lock_version": gorm.Expr("lock_version + ?", 1),
	}
	if transition.To == TaskSubmissionAttemptStatusAccepted || transition.To == TaskSubmissionAttemptStatusSubmissionUnknown {
		providerOperationID, err := preserveTaskSubmissionAttemptReference(attempt.ProviderOperationID, transition.ProviderOperationID, "provider operation id")
		if err != nil {
			return false, err
		}
		upstreamRequestID, err := preserveTaskSubmissionAttemptReference(attempt.UpstreamRequestID, transition.UpstreamRequestID, "upstream request id")
		if err != nil {
			return false, err
		}
		if transition.To == TaskSubmissionAttemptStatusAccepted && providerOperationID == "" {
			return false, fmt.Errorf("%w: an accepted task submission attempt requires a provider operation id", ErrTaskRecoveryInvalidRecord)
		}
		updates["provider_operation_id"] = providerOperationID
		updates["upstream_request_id"] = upstreamRequestID
		if transition.TaskPlatform != "" || transition.TaskAction != "" {
			if transition.TaskPlatform == "" || transition.TaskAction == "" {
				return false, fmt.Errorf("%w: task recovery classification must be complete", ErrTaskRecoveryInvalidRecord)
			}
			updates["task_platform"] = transition.TaskPlatform
			updates["task_action"] = transition.TaskAction
		}
	} else if transition.ProviderOperationID != "" || transition.UpstreamRequestID != "" {
		return false, fmt.Errorf("%w: upstream references may only be recorded for accepted or unknown submissions", ErrTaskRecoveryInvalidRecord)
	}
	if transition.To == TaskSubmissionAttemptStatusAccepted || transition.To == TaskSubmissionAttemptStatusRejected {
		updates["finished_at"] = transition.TransitionedAt
	}
	result := taskRecoveryControlledWrite(tx).Table("task_submission_attempts").
		Where("id = ? AND status = ? AND lock_version = ? AND updated_at <= ?", attemptID, transition.From, transition.ExpectedVersion, transitionedAt).
		Updates(updates)
	return result.RowsAffected == 1, result.Error
}

func preserveTaskSubmissionAttemptReference(existing, provided, fieldName string) (string, error) {
	if existing != "" && provided != "" && existing != provided {
		return "", fmt.Errorf("%w: %s cannot change once recorded", ErrTaskRecoveryInvalidRecord, fieldName)
	}
	if existing != "" {
		return existing, nil
	}
	return provided, nil
}

func validateStoredTaskSubmissionAttempt(tx *gorm.DB, attempt *TaskSubmissionAttempt) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	if attempt == nil || attempt.ID <= 0 || attempt.OperationID <= 0 || attempt.AttemptNo != 1 ||
		attempt.ChannelID <= 0 || int64(attempt.ChannelID) > taskBillingQuotaMax || !attempt.Status.Valid() ||
		attempt.Provider == "" || attempt.Provider != strings.ToLower(strings.TrimSpace(attempt.Provider)) ||
		attempt.RequestClass == "" || attempt.RequestClass != strings.ToLower(strings.TrimSpace(attempt.RequestClass)) ||
		attempt.ProviderOperationID != strings.TrimSpace(attempt.ProviderOperationID) ||
		attempt.UpstreamRequestID != strings.TrimSpace(attempt.UpstreamRequestID) ||
		attempt.TaskPlatform != strings.ToLower(strings.TrimSpace(attempt.TaskPlatform)) || attempt.TaskAction != strings.TrimSpace(attempt.TaskAction) ||
		attempt.OutcomeCode != strings.TrimSpace(attempt.OutcomeCode) ||
		len(attempt.Provider) > 64 || len(attempt.RequestClass) > 48 || len(attempt.ProviderOperationID) > 191 ||
		len(attempt.UpstreamRequestID) > 128 || len(attempt.TaskPlatform) > 30 || len(attempt.TaskAction) > 40 || len(attempt.OutcomeCode) > 64 ||
		attempt.LockVersion <= 0 || attempt.CreatedAt <= 0 || attempt.UpdatedAt < attempt.CreatedAt {
		return fmt.Errorf("%w: stored task submission attempt is not a valid immutable v1 record", ErrTaskRecoveryInvalidRecord)
	}
	if (attempt.StartedAt != nil && *attempt.StartedAt <= 0) || (attempt.FinishedAt != nil && *attempt.FinishedAt <= 0) ||
		(attempt.StartedAt != nil && (*attempt.StartedAt < attempt.CreatedAt || *attempt.StartedAt > attempt.UpdatedAt)) ||
		(attempt.FinishedAt != nil && (*attempt.FinishedAt < attempt.CreatedAt || *attempt.FinishedAt > attempt.UpdatedAt)) ||
		(attempt.StartedAt != nil && attempt.FinishedAt != nil && *attempt.FinishedAt < *attempt.StartedAt) {
		return fmt.Errorf("%w: stored task submission attempt has invalid timestamps", ErrTaskRecoveryInvalidRecord)
	}
	switch attempt.Status {
	case TaskSubmissionAttemptStatusPrepared:
		if attempt.ProviderOperationID != "" || attempt.UpstreamRequestID != "" || attempt.TaskPlatform != "" || attempt.TaskAction != "" || attempt.OutcomeCode != "" || attempt.StartedAt != nil || attempt.FinishedAt != nil {
			return fmt.Errorf("%w: stored prepared task submission attempt contains an outcome", ErrTaskRecoveryInvalidRecord)
		}
	case TaskSubmissionAttemptStatusDispatching:
		if attempt.ProviderOperationID != "" || attempt.UpstreamRequestID != "" || attempt.OutcomeCode != "" || attempt.StartedAt == nil || attempt.FinishedAt != nil {
			return fmt.Errorf("%w: stored dispatching task submission attempt has an invalid outcome", ErrTaskRecoveryInvalidRecord)
		}
	case TaskSubmissionAttemptStatusSubmissionUnknown:
		if attempt.StartedAt == nil || attempt.FinishedAt != nil {
			return fmt.Errorf("%w: stored unknown task submission attempt has invalid timestamps", ErrTaskRecoveryInvalidRecord)
		}
	case TaskSubmissionAttemptStatusAccepted:
		if attempt.ProviderOperationID == "" || attempt.StartedAt == nil || attempt.FinishedAt == nil {
			return fmt.Errorf("%w: stored accepted task submission attempt is incomplete", ErrTaskRecoveryInvalidRecord)
		}
	case TaskSubmissionAttemptStatusRejected:
		if attempt.StartedAt == nil || attempt.FinishedAt == nil {
			return fmt.Errorf("%w: stored rejected task submission attempt is incomplete", ErrTaskRecoveryInvalidRecord)
		}
	}

	// A locking read, because a caller that lost the idempotency race reaches
	// here inside a transaction whose MySQL REPEATABLE READ snapshot predates
	// the winner's commit. A plain read returns no row there and the loser
	// reports a missing operation instead of replaying the winner's.
	var operation TaskSubmissionOperation
	if err := lockForUpdate(tx).Where("id = ?", attempt.OperationID).First(&operation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskSubmissionOperationNotFound
		}
		return err
	}
	return validateStoredTaskSubmissionOperation(tx, &operation)
}

type TaskBillingEventType string

const (
	TaskBillingEventTypeReserve              TaskBillingEventType = "reserve"
	TaskBillingEventTypeSubmissionAdjustment TaskBillingEventType = "submission_adjustment"
	TaskBillingEventTypeTerminalSettlement   TaskBillingEventType = "terminal_settlement"
	TaskBillingEventTypeRefund               TaskBillingEventType = "refund"
	TaskBillingEventTypeManualResolution     TaskBillingEventType = "manual_resolution"
)

func (eventType TaskBillingEventType) Valid() bool {
	switch eventType {
	case TaskBillingEventTypeReserve,
		TaskBillingEventTypeSubmissionAdjustment,
		TaskBillingEventTypeTerminalSettlement,
		TaskBillingEventTypeRefund,
		TaskBillingEventTypeManualResolution:
		return true
	default:
		return false
	}
}

type TaskBillingEventState string

const (
	TaskBillingEventStatePending      TaskBillingEventState = "pending"
	TaskBillingEventStateClaimed      TaskBillingEventState = "claimed"
	TaskBillingEventStateRetryable    TaskBillingEventState = "retryable"
	TaskBillingEventStateApplied      TaskBillingEventState = "applied"
	TaskBillingEventStateManualReview TaskBillingEventState = "manual_review"
)

func (state TaskBillingEventState) Valid() bool {
	switch state {
	case TaskBillingEventStatePending,
		TaskBillingEventStateClaimed,
		TaskBillingEventStateRetryable,
		TaskBillingEventStateApplied,
		TaskBillingEventStateManualReview:
		return true
	default:
		return false
	}
}

func CanTransitionTaskBillingEvent(from, to TaskBillingEventState) bool {
	switch from {
	case TaskBillingEventStatePending, TaskBillingEventStateRetryable:
		return to == TaskBillingEventStateClaimed
	case TaskBillingEventStateClaimed:
		return to == TaskBillingEventStateApplied ||
			to == TaskBillingEventStateRetryable ||
			to == TaskBillingEventStateManualReview
	default:
		return false
	}
}

// TaskBillingEventPayload is the immutable v1 application snapshot used when
// applying or auditing an authoritative billing event. Every field is a bounded
// identifier, classification or amount already present in the event; there is
// deliberately no request body, prompt, provider response or credential field.
type TaskBillingEventPayload struct {
	Version           int                  `json:"version"`
	BillingEventID    string               `json:"billing_event_id"`
	EventKey          string               `json:"event_key"`
	EventType         TaskBillingEventType `json:"event_type"`
	OperationID       int64                `json:"operation_id,omitempty"`
	TaskID            int64                `json:"task_id,omitempty"`
	UserID            int                  `json:"user_id"`
	TokenID           int                  `json:"token_id"`
	ChannelID         int                  `json:"channel_id,omitempty"`
	BillingSource     string               `json:"billing_source"`
	SubscriptionID    int                  `json:"subscription_id,omitempty"`
	QuotaDelta        int64                `json:"quota_delta"`
	RequestID         string               `json:"request_id,omitempty"`
	NodeName          string               `json:"node_name,omitempty"`
	ReasonCode        string               `json:"reason_code,omitempty"`
	ResolutionSource  string               `json:"resolution_source,omitempty"`
	AuditCommandID    string               `json:"audit_command_id,omitempty"`
	EvidenceID        string               `json:"evidence_id,omitempty"`
	EvidenceHash      string               `json:"evidence_hash,omitempty"`
	EvidenceVersion   int                  `json:"evidence_version,omitempty"`
	StatisticsVersion int                  `json:"statistics_version,omitempty"`
	StatisticsApplied bool                 `json:"statistics_applied,omitempty"`
}

func (payload *TaskBillingEventPayload) Scan(value interface{}) error {
	*payload = TaskBillingEventPayload{}
	data, err := taskRecoveryTextValue(value)
	if err != nil || len(data) == 0 {
		return err
	}
	return common.Unmarshal(data, payload)
}

func (payload TaskBillingEventPayload) Value() (driver.Value, error) {
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// TaskBillingEvent is the authoritative main-database ledger event. EventKey
// is a deterministic exactly-once business key; EventID is the stable value
// copied to the log outbox and projected Log row. Audit fields are bounded
// codes/references and intentionally exclude arbitrary request/provider data.
type TaskBillingEvent struct {
	ID                int64                   `json:"id" gorm:"primaryKey"`
	EventID           string                  `json:"event_id" gorm:"type:varchar(64);not null;uniqueIndex:uidx_task_billing_event_id;<-:create"`
	EventKey          string                  `json:"event_key" gorm:"type:varchar(128);not null;uniqueIndex:uidx_task_billing_event_key;<-:create"`
	OperationID       *int64                  `json:"operation_id,omitempty" gorm:"index;<-:create"`
	TaskID            *int64                  `json:"task_id,omitempty" gorm:"index;<-:create"`
	EventType         TaskBillingEventType    `json:"event_type" gorm:"type:varchar(32);not null;index;<-:create"`
	State             TaskBillingEventState   `json:"state" gorm:"type:varchar(24);not null;index:idx_task_billing_ready,priority:1;<-:create"`
	UserID            int                     `json:"user_id" gorm:"not null;index;<-:create"`
	TokenID           int                     `json:"token_id" gorm:"not null;index;<-:create"`
	ChannelID         int                     `json:"channel_id" gorm:"index;<-:create"`
	BillingSource     string                  `json:"billing_source" gorm:"type:varchar(32);not null;<-:create"`
	SubscriptionID    int                     `json:"subscription_id,omitempty" gorm:"index;<-:create"`
	QuotaDelta        int64                   `json:"quota_delta" gorm:"type:bigint;not null;<-:create"`
	RequestID         string                  `json:"request_id,omitempty" gorm:"type:varchar(64);index;<-:create"`
	NodeName          string                  `json:"node_name,omitempty" gorm:"type:varchar(128);<-:create"`
	ReasonCode        string                  `json:"reason_code,omitempty" gorm:"type:varchar(64);<-:create"`
	ResolutionSource  string                  `json:"resolution_source,omitempty" gorm:"type:varchar(32);<-:create"`
	AuditCommandID    string                  `json:"audit_command_id,omitempty" gorm:"type:varchar(48);<-:create"`
	EvidenceID        string                  `json:"evidence_id,omitempty" gorm:"type:varchar(191);not null;default:'';<-:create"`
	EvidenceHash      string                  `json:"evidence_hash,omitempty" gorm:"type:char(64);not null;default:'';<-:create"`
	EvidenceVersion   int                     `json:"evidence_version,omitempty" gorm:"not null;default:0;<-:create"`
	StatisticsVersion int                     `json:"statistics_version,omitempty" gorm:"not null;default:0;<-:create"`
	StatisticsApplied bool                    `json:"statistics_applied,omitempty" gorm:"not null;default:false;<-:create"`
	ClaimedBy         string                  `json:"claimed_by,omitempty" gorm:"type:varchar(128);index;<-:create"`
	ClaimedUntil      int64                   `json:"claimed_until,omitempty" gorm:"type:bigint;index;<-:create"`
	AttemptCount      int                     `json:"attempt_count" gorm:"not null;<-:create"`
	NextAttemptAt     int64                   `json:"next_attempt_at,omitempty" gorm:"type:bigint;index:idx_task_billing_ready,priority:2;<-:create"`
	LastErrorCode     string                  `json:"last_error_code,omitempty" gorm:"type:varchar(64);<-:create"`
	LastErrorAt       int64                   `json:"last_error_at,omitempty" gorm:"type:bigint;<-:create"`
	AppliedAt         *int64                  `json:"applied_at,omitempty" gorm:"type:bigint;<-:create"`
	PayloadVersion    int                     `json:"payload_version" gorm:"not null;<-:create"`
	Payload           TaskBillingEventPayload `json:"-" gorm:"type:text;not null;<-:create"`
	LockVersion       int64                   `json:"lock_version" gorm:"type:bigint;not null;<-:create"`
	CreatedAt         int64                   `json:"created_at" gorm:"type:bigint;index;<-:create"`
	UpdatedAt         int64                   `json:"updated_at" gorm:"type:bigint;<-:create"`
}

func (event *TaskBillingEvent) BeforeCreate(tx *gorm.DB) error {
	if event == nil {
		return ErrTaskRecoveryInvalidRecord
	}
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	if event.EventID == "" {
		eventID, err := generateTaskBillingEventID()
		if err != nil {
			return err
		}
		event.EventID = eventID
	}
	providedEventKey := strings.TrimSpace(event.EventKey)
	event.BillingSource = strings.ToLower(strings.TrimSpace(event.BillingSource))
	event.ReasonCode = strings.ToLower(strings.TrimSpace(event.ReasonCode))
	if event.ReasonCode == "" {
		event.ReasonCode = "task_" + string(event.EventType)
	}
	if !validTaskTerminalReasonCode(event.ReasonCode) {
		return fmt.Errorf("%w: task billing reason code is not a safe internal code", ErrTaskRecoveryInvalidRecord)
	}
	event.ResolutionSource = strings.TrimSpace(event.ResolutionSource)
	event.AuditCommandID = strings.ToLower(strings.TrimSpace(event.AuditCommandID))
	event.EvidenceID = strings.TrimSpace(event.EvidenceID)
	event.EvidenceHash = strings.ToLower(strings.TrimSpace(event.EvidenceHash))
	if event.State == "" {
		event.State = TaskBillingEventStatePending
	}
	if event.State != TaskBillingEventStatePending || !event.EventType.Valid() || event.UserID <= 0 || event.TokenID <= 0 || event.ChannelID <= 0 || event.SubscriptionID < 0 ||
		int64(event.UserID) > taskBillingQuotaMax || int64(event.TokenID) > taskBillingQuotaMax || int64(event.ChannelID) > taskBillingQuotaMax || int64(event.SubscriptionID) > taskBillingQuotaMax || event.BillingSource == "" {
		return fmt.Errorf("%w: new task billing event is incomplete or not pending", ErrTaskRecoveryInvalidRecord)
	}
	if event.ClaimedBy != "" || event.ClaimedUntil != 0 || event.AttemptCount != 0 || event.NextAttemptAt != 0 || event.LastErrorCode != "" || event.LastErrorAt != 0 || event.AppliedAt != nil {
		return fmt.Errorf("%w: a pending task billing event cannot contain processing state", ErrTaskRecoveryInvalidRecord)
	}
	if (event.OperationID == nil || *event.OperationID <= 0) && (event.TaskID == nil || *event.TaskID <= 0) {
		return fmt.Errorf("%w: task billing event requires an operation or task reference", ErrTaskRecoveryInvalidRecord)
	}
	if (event.OperationID != nil && *event.OperationID <= 0) || (event.TaskID != nil && *event.TaskID <= 0) {
		return fmt.Errorf("%w: task billing event contains an invalid subject reference", ErrTaskRecoveryInvalidRecord)
	}
	if !validTaskBillingEventID(event.EventID) ||
		len(event.BillingSource) > 32 || len(event.RequestID) > 64 || len(event.NodeName) > 128 ||
		len(event.ReasonCode) > 64 || len(event.ResolutionSource) > 32 || len(event.EvidenceID) > 191 || !validTaskMutationEvidence(event.EvidenceID, event.EvidenceHash, event.EvidenceVersion) {
		return fmt.Errorf("%w: task billing event identifier exceeds its database bound", ErrTaskRecoveryInvalidRecord)
	}
	if event.QuotaDelta < taskBillingQuotaMin || event.QuotaDelta > taskBillingQuotaMax {
		return fmt.Errorf("%w: task billing quota delta exceeds the int32 ledger boundary", ErrTaskRecoveryInvalidRecord)
	}
	if (event.StatisticsApplied && event.StatisticsVersion != 1) || (!event.StatisticsApplied && event.StatisticsVersion != 0) {
		return fmt.Errorf("%w: task billing event statistics evidence is invalid", ErrTaskRecoveryInvalidRecord)
	}
	if event.EventType == TaskBillingEventTypeReserve && event.QuotaDelta > 0 {
		return fmt.Errorf("%w: reserve events must decrease the available quota", ErrTaskRecoveryInvalidRecord)
	}
	if event.EventType == TaskBillingEventTypeRefund && event.QuotaDelta < 0 {
		return fmt.Errorf("%w: refund events must restore available quota", ErrTaskRecoveryInvalidRecord)
	}
	if event.BillingSource != "wallet" && event.BillingSource != "subscription" {
		return fmt.Errorf("%w: unsupported task billing source", ErrTaskRecoveryInvalidRecord)
	}
	if (event.BillingSource == "wallet" && event.SubscriptionID != 0) || (event.BillingSource == "subscription" && event.SubscriptionID <= 0) {
		return fmt.Errorf("%w: task billing subscription does not match its funding source", ErrTaskRecoveryInvalidRecord)
	}
	if event.BillingSource == "subscription" {
		var subscription UserSubscription
		if err := tx.Select("id", "user_id").Where("id = ? AND user_id = ?", event.SubscriptionID, event.UserID).First(&subscription).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: task billing subscription does not belong to its user", ErrTaskRecoveryInvalidRecord)
			}
			return err
		}
	}
	if event.ResolutionSource != "" && event.ResolutionSource != TaskSubmissionResolutionSourceProviderVerified && event.ResolutionSource != TaskSubmissionResolutionSourceManualAudit {
		return fmt.Errorf("%w: unsupported task billing resolution source", ErrTaskRecoveryInvalidRecord)
	}
	if event.EventType == TaskBillingEventTypeManualResolution {
		if !validTaskBillingAuditCommandID(event.AuditCommandID) || event.ResolutionSource != TaskSubmissionResolutionSourceManualAudit {
			return fmt.Errorf("%w: manual billing events require a stable audit command id", ErrTaskRecoveryInvalidRecord)
		}
	} else if event.AuditCommandID != "" || event.ResolutionSource == TaskSubmissionResolutionSourceManualAudit {
		return fmt.Errorf("%w: manual audit metadata is only valid for manual billing events", ErrTaskRecoveryInvalidRecord)
	}
	if event.LockVersion == 0 {
		event.LockVersion = 1
	}
	if event.LockVersion != 1 {
		return fmt.Errorf("%w: new task billing event lock version must be one", ErrTaskRecoveryInvalidRecord)
	}
	if event.CreatedAt != 0 || event.UpdatedAt != 0 {
		return fmt.Errorf("%w: task billing event timestamps are assigned by the database clock", ErrTaskRecoveryInvalidRecord)
	}

	var operation TaskSubmissionOperation
	if event.OperationID != nil {
		if err := tx.Where("id = ?", *event.OperationID).First(&operation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTaskSubmissionOperationNotFound
			}
			return err
		}
	}
	if event.TaskID == nil && operation.TaskID != nil {
		event.TaskID = operation.TaskID
	}
	var task Task
	if event.TaskID != nil {
		if err := tx.Select("id", "task_id", "user_id", "channel_id").Where("id = ?", *event.TaskID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTaskSubmissionTaskNotFound
			}
			return err
		}
		if operation.ID == 0 {
			if err := tx.Where("public_id = ?", task.TaskID).First(&operation).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrTaskSubmissionOperationNotFound
				}
				return err
			}
			event.OperationID = &operation.ID
		}
		if task.TaskID != operation.PublicID || operation.TaskID == nil || *operation.TaskID != task.ID {
			return ErrTaskSubmissionTaskIDMismatch
		}
		if task.UserId != event.UserID || (task.ChannelId > 0 && task.ChannelId != event.ChannelID) {
			return fmt.Errorf("%w: task billing event does not match the formal task owner or channel", ErrTaskRecoveryInvalidRecord)
		}
	}
	if err := validateStoredTaskSubmissionOperation(tx, &operation); err != nil {
		return err
	}
	if operation.UserID != event.UserID || operation.TokenID != event.TokenID {
		return fmt.Errorf("%w: task billing event does not match the operation owner", ErrTaskRecoveryInvalidRecord)
	}
	var attempt TaskSubmissionAttempt
	if err := tx.Where("operation_id = ?", operation.ID).First(&attempt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: task billing events require the v1 submission attempt", ErrTaskRecoveryInvalidRecord)
		}
		return err
	}
	if err := validateStoredTaskSubmissionAttempt(tx, &attempt); err != nil {
		return err
	}
	if attempt.ChannelID != event.ChannelID {
		return fmt.Errorf("%w: task billing event does not match the submission attempt channel", ErrTaskRecoveryInvalidRecord)
	}
	if event.ResolutionSource == TaskSubmissionResolutionSourceProviderVerified &&
		(event.EvidenceVersion != 1 || event.EvidenceID == "" || (event.EvidenceID != attempt.ProviderOperationID && event.EvidenceID != attempt.UpstreamRequestID)) {
		return fmt.Errorf("%w: provider-verified billing event evidence is not bound to its attempt", ErrTaskRecoveryInvalidRecord)
	}

	canonicalEventKey := "task:" + operation.PublicID + ":" + string(event.EventType)
	if event.EventType == TaskBillingEventTypeManualResolution {
		canonicalEventKey += ":" + event.AuditCommandID
	}
	canonicalEventKey += ":v1"
	if len(canonicalEventKey) > 128 {
		return fmt.Errorf("%w: canonical task billing event key exceeds its database bound", ErrTaskRecoveryInvalidRecord)
	}
	if providedEventKey != "" && providedEventKey != canonicalEventKey {
		return fmt.Errorf("%w: task billing event key is not canonical", ErrTaskRecoveryInvalidRecord)
	}
	event.EventKey = canonicalEventKey
	if event.RequestID == "" {
		event.RequestID = stableTaskBillingRequestID("", canonicalEventKey)
	}
	if event.Payload == (TaskBillingEventPayload{}) {
		event.Payload = taskBillingEventPayloadFromRecord(event)
	}
	if event.PayloadVersion == 0 {
		event.PayloadVersion = event.Payload.Version
	}
	if event.PayloadVersion != TaskRecoveryPayloadVersion || event.Payload != taskBillingEventPayloadFromRecord(event) {
		return fmt.Errorf("%w: task billing event payload must be the immutable v1 record snapshot", ErrTaskRecoveryInvalidRecord)
	}
	now, err := taskRecoveryDBTimestamp(tx)
	if err != nil {
		return err
	}
	event.CreatedAt = now
	event.UpdatedAt = now
	return nil
}

func (event *TaskBillingEvent) BeforeUpdate(_ *gorm.DB) error {
	return fmt.Errorf("%w: task billing events may only change through the state CAS", ErrTaskRecoveryInvalidRecord)
}

func (event *TaskBillingEvent) BeforeDelete(_ *gorm.DB) error {
	return fmt.Errorf("%w: task billing events are immutable ledger records", ErrTaskRecoveryInvalidRecord)
}

func generateTaskBillingEventID() (string, error) {
	key, err := common.GenerateRandomCharsKey(taskSubmissionPublicIDRandomLength)
	if err != nil {
		return "", fmt.Errorf("generate task billing event id: %w", err)
	}
	return "billing_evt_" + strings.ToLower(key), nil
}

type TaskRecoveryProcessingLease struct {
	WorkerID     string
	LeaseSeconds int64
}

type TaskBillingEventTransition struct {
	From              TaskBillingEventState
	To                TaskBillingEventState
	Lease             TaskRecoveryProcessingLease
	WorkerID          string
	RetryDelaySeconds int64
	LastErrorCode     string
	ExpectedVersion   int64
	TransitionedAt    int64
}

func TransitionTaskBillingEvent(tx *gorm.DB, eventID int64, transition TaskBillingEventTransition) (bool, error) {
	if tx == nil {
		return false, gorm.ErrInvalidDB
	}
	if eventID <= 0 || transition.ExpectedVersion <= 0 || transition.ExpectedVersion == taskRecoveryMaxInt64 ||
		!CanTransitionTaskBillingEvent(transition.From, transition.To) {
		return false, ErrTaskRecoveryInvalidTransition
	}
	transitionedAt, err := taskRecoveryValidatedTime(tx, transition.TransitionedAt)
	if err != nil {
		return false, err
	}
	updates := map[string]interface{}{
		"state":        transition.To,
		"updated_at":   transitionedAt,
		"lock_version": gorm.Expr("lock_version + ?", 1),
	}
	query := taskRecoveryControlledWrite(tx).Table("task_billing_events").Where(
		"id = ? AND state = ? AND lock_version = ? AND updated_at <= ?", eventID, transition.From, transition.ExpectedVersion, transitionedAt,
	)
	if transition.To == TaskBillingEventStateClaimed {
		lease, err := validateTaskRecoveryProcessingLease(transition.Lease, transitionedAt)
		if err != nil {
			return false, err
		}
		if transition.WorkerID != "" || transition.RetryDelaySeconds != 0 || transition.LastErrorCode != "" {
			return false, fmt.Errorf("%w: claim transitions accept only a processing lease", ErrTaskRecoveryInvalidRecord)
		}
		query = query.Where("next_attempt_at <= ? AND attempt_count < ?", transitionedAt, taskRecoveryAttemptCountMax)
		updates["claimed_by"] = lease.WorkerID
		updates["claimed_until"] = lease.LeaseUntil
		updates["attempt_count"] = gorm.Expr("attempt_count + ?", 1)
		updates["next_attempt_at"] = 0
	} else {
		workerID, err := validateTaskRecoveryWorkerID(transition.WorkerID)
		if err != nil {
			return false, err
		}
		if transition.Lease != (TaskRecoveryProcessingLease{}) {
			return false, fmt.Errorf("%w: completion transitions use the current persisted lease", ErrTaskRecoveryInvalidRecord)
		}
		query = query.Where("claimed_by = ? AND claimed_until > ?", workerID, transitionedAt)
		updates["claimed_by"] = ""
		updates["claimed_until"] = 0
		switch transition.To {
		case TaskBillingEventStateApplied:
			if transition.RetryDelaySeconds != 0 || transition.LastErrorCode != "" {
				return false, fmt.Errorf("%w: applied events cannot retain a retry failure", ErrTaskRecoveryInvalidRecord)
			}
			updates["applied_at"] = transitionedAt
		case TaskBillingEventStateRetryable:
			errorCode, nextAttemptAt, err := validateTaskRecoveryFailure(transition.LastErrorCode, transition.RetryDelaySeconds, transitionedAt)
			if err != nil {
				return false, err
			}
			updates["next_attempt_at"] = nextAttemptAt
			updates["last_error_code"] = errorCode
			updates["last_error_at"] = transitionedAt
		case TaskBillingEventStateManualReview:
			errorCode := strings.TrimSpace(transition.LastErrorCode)
			if errorCode == "" || len(errorCode) > 64 || transition.RetryDelaySeconds != 0 {
				return false, fmt.Errorf("%w: manual review requires a bounded failure code", ErrTaskRecoveryInvalidRecord)
			}
			updates["next_attempt_at"] = 0
			updates["last_error_code"] = errorCode
			updates["last_error_at"] = transitionedAt
		}
	}
	result := query.Updates(updates)
	return result.RowsAffected == 1, result.Error
}

func ReclaimExpiredTaskBillingEvent(tx *gorm.DB, eventID int64, lease TaskRecoveryProcessingLease, expectedVersion, checkedAt int64) (bool, error) {
	if tx == nil {
		return false, gorm.ErrInvalidDB
	}
	if eventID <= 0 || expectedVersion <= 0 || expectedVersion == taskRecoveryMaxInt64 {
		return false, ErrTaskRecoveryInvalidTransition
	}
	checkedAt, err := taskRecoveryValidatedTime(tx, checkedAt)
	if err != nil {
		return false, err
	}
	validatedLease, err := validateTaskRecoveryProcessingLease(lease, checkedAt)
	if err != nil {
		return false, err
	}
	result := taskRecoveryControlledWrite(tx).Table("task_billing_events").Where(
		"id = ? AND state = ? AND lock_version = ? AND updated_at <= ? AND claimed_until > 0 AND claimed_until <= ? AND attempt_count < ?",
		eventID, TaskBillingEventStateClaimed, expectedVersion, checkedAt, checkedAt, taskRecoveryAttemptCountMax,
	).Updates(map[string]interface{}{
		"claimed_by":      validatedLease.WorkerID,
		"claimed_until":   validatedLease.LeaseUntil,
		"attempt_count":   gorm.Expr("attempt_count + ?", 1),
		"last_error_code": "lease_expired",
		"last_error_at":   checkedAt,
		"updated_at":      checkedAt,
		"lock_version":    gorm.Expr("lock_version + ?", 1),
	})
	return result.RowsAffected == 1, result.Error
}

func taskRecoveryValidatedTime(tx *gorm.DB, provided int64) (int64, error) {
	if provided < 0 {
		return 0, fmt.Errorf("%w: task recovery timestamps cannot be negative", ErrTaskRecoveryInvalidRecord)
	}
	databaseNow, err := taskRecoveryDBTimestamp(tx)
	if err != nil {
		return 0, err
	}
	if provided != 0 && provided != databaseNow {
		return 0, fmt.Errorf("%w: supplied task recovery time does not match the database clock", ErrTaskRecoveryInvalidRecord)
	}
	return databaseNow, nil
}

func taskRecoveryDBTimestamp(tx *gorm.DB) (int64, error) {
	if tx == nil {
		return 0, gorm.ErrInvalidDB
	}
	tx = tx.Session(&gorm.Session{NewDB: true})
	var query string
	switch tx.Dialector.Name() {
	case "postgres":
		query = "SELECT FLOOR(EXTRACT(EPOCH FROM clock_timestamp()))::bigint"
	case "mysql":
		query = "SELECT UNIX_TIMESTAMP()"
	case "sqlite":
		query = "SELECT strftime('%s','now')"
	default:
		return 0, fmt.Errorf("%w: unsupported task recovery database clock dialect %q", ErrTaskRecoveryInvalidRecord, tx.Dialector.Name())
	}
	var databaseNow int64
	if err := tx.Raw(query).Scan(&databaseNow).Error; err != nil {
		return 0, fmt.Errorf("read task recovery database clock: %w", err)
	}
	if databaseNow <= 0 {
		return 0, fmt.Errorf("%w: task recovery database clock returned a non-positive timestamp", ErrTaskRecoveryInvalidRecord)
	}
	return databaseNow, nil
}

func validateTaskRecoveryWorkerID(workerID string) (string, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || len(workerID) > 128 {
		return "", fmt.Errorf("%w: task recovery worker id is missing or exceeds its database bound", ErrTaskRecoveryInvalidRecord)
	}
	return workerID, nil
}

type taskRecoveryValidatedLease struct {
	WorkerID   string
	LeaseUntil int64
}

func validateTaskRecoveryProcessingLease(lease TaskRecoveryProcessingLease, checkedAt int64) (taskRecoveryValidatedLease, error) {
	workerID, err := validateTaskRecoveryWorkerID(lease.WorkerID)
	if err != nil {
		return taskRecoveryValidatedLease{}, err
	}
	if lease.LeaseSeconds <= 0 || lease.LeaseSeconds > TaskRecoveryMaxProcessingLeaseSeconds ||
		checkedAt > taskRecoveryMaxInt64-lease.LeaseSeconds {
		return taskRecoveryValidatedLease{}, fmt.Errorf("%w: task recovery lease duration is outside its safe boundary", ErrTaskRecoveryInvalidRecord)
	}
	return taskRecoveryValidatedLease{WorkerID: workerID, LeaseUntil: checkedAt + lease.LeaseSeconds}, nil
}

func validateTaskRecoveryFailure(errorCode string, retryDelaySeconds, transitionedAt int64) (string, int64, error) {
	errorCode = strings.TrimSpace(errorCode)
	if errorCode == "" || len(errorCode) > 64 || retryDelaySeconds <= 0 || retryDelaySeconds > TaskRecoveryMaxRetryDelaySeconds ||
		transitionedAt > taskRecoveryMaxInt64-retryDelaySeconds {
		return "", 0, fmt.Errorf("%w: retry requires a bounded failure code and delay", ErrTaskRecoveryInvalidRecord)
	}
	return errorCode, transitionedAt + retryDelaySeconds, nil
}

func taskBillingEventPayloadFromRecord(event *TaskBillingEvent) TaskBillingEventPayload {
	payload := TaskBillingEventPayload{
		Version:           TaskRecoveryPayloadVersion,
		BillingEventID:    event.EventID,
		EventKey:          event.EventKey,
		EventType:         event.EventType,
		UserID:            event.UserID,
		TokenID:           event.TokenID,
		ChannelID:         event.ChannelID,
		BillingSource:     event.BillingSource,
		SubscriptionID:    event.SubscriptionID,
		QuotaDelta:        event.QuotaDelta,
		RequestID:         event.RequestID,
		NodeName:          event.NodeName,
		ReasonCode:        event.ReasonCode,
		ResolutionSource:  event.ResolutionSource,
		AuditCommandID:    event.AuditCommandID,
		EvidenceID:        event.EvidenceID,
		EvidenceHash:      event.EvidenceHash,
		EvidenceVersion:   event.EvidenceVersion,
		StatisticsVersion: event.StatisticsVersion,
		StatisticsApplied: event.StatisticsApplied,
	}
	if event.OperationID != nil {
		payload.OperationID = *event.OperationID
	}
	if event.TaskID != nil {
		payload.TaskID = *event.TaskID
	}
	return payload
}

func GetTaskBillingEventByEventID(tx *gorm.DB, stableEventID string) (*TaskBillingEvent, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	stableEventID = strings.TrimSpace(stableEventID)
	if stableEventID == "" {
		return nil, ErrTaskRecoveryInvalidRecord
	}
	var event TaskBillingEvent
	err := tx.Where("event_id = ?", stableEventID).First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func loadTaskBillingEventByEventKeyForReplay(tx *gorm.DB, eventKey string) (*TaskBillingEvent, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	var event TaskBillingEvent
	err := lockForUpdate(tx).Where("event_key = ?", eventKey).First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func loadTaskBillingEventByIDForReplay(tx *gorm.DB, stableEventID string) (*TaskBillingEvent, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	stableEventID = strings.TrimSpace(stableEventID)
	if stableEventID == "" {
		return nil, ErrTaskRecoveryInvalidRecord
	}
	var event TaskBillingEvent
	err := lockForUpdate(tx).Where("event_id = ?", stableEventID).First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func validateStoredTaskSubmissionOperation(tx *gorm.DB, operation *TaskSubmissionOperation) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	if operation == nil || operation.ID <= 0 || !validTaskSubmissionPublicID(operation.PublicID) ||
		operation.UserID <= 0 || operation.TokenID <= 0 || int64(operation.UserID) > taskBillingQuotaMax || int64(operation.TokenID) > taskBillingQuotaMax ||
		operation.HTTPMethod == "" || operation.HTTPMethod != strings.ToUpper(operation.HTTPMethod) || !validTaskSubmissionOperationKind(operation.OperationKind) ||
		operation.RequestID != strings.TrimSpace(operation.RequestID) || len(operation.RequestID) > 64 ||
		!validTaskRecoveryDigest(operation.IdempotencyKeyHash) || !validTaskRecoveryDigest(operation.RequestFingerprint) ||
		!operation.Status.Valid() || operation.LockVersion <= 0 || operation.CreatedAt <= 0 || operation.UpdatedAt < operation.CreatedAt ||
		operation.ReasonCode != strings.TrimSpace(operation.ReasonCode) || operation.ResolutionSource != strings.TrimSpace(operation.ResolutionSource) ||
		len(operation.HTTPMethod) > 16 || len(operation.OperationKind) > 48 || len(operation.ReasonCode) > 64 || len(operation.ResolutionSource) > 32 ||
		(operation.DispatchStartedAt != nil && (*operation.DispatchStartedAt <= 0 || *operation.DispatchStartedAt < operation.CreatedAt || *operation.DispatchStartedAt > operation.UpdatedAt)) ||
		(operation.ResolvedAt != nil && (*operation.ResolvedAt <= 0 || *operation.ResolvedAt < operation.CreatedAt || *operation.ResolvedAt > operation.UpdatedAt)) {
		return fmt.Errorf("%w: stored task submission operation is not a valid immutable v1 record", ErrTaskRecoveryInvalidRecord)
	}
	if operation.ResolutionSource != "" &&
		operation.ResolutionSource != TaskSubmissionResolutionSourceProviderVerified &&
		operation.ResolutionSource != TaskSubmissionResolutionSourceManualAudit {
		return fmt.Errorf("%w: stored task submission operation has an unsupported resolution source", ErrTaskRecoveryInvalidRecord)
	}
	selectionFrozen := operation.BillingPreference != "" || operation.BillingSource != "" || operation.SubscriptionID != 0
	if selectionFrozen {
		if common.NormalizeBillingPreference(operation.BillingPreference) != operation.BillingPreference ||
			(operation.BillingSource != "wallet" && operation.BillingSource != "subscription") ||
			(operation.BillingSource == "wallet" && operation.SubscriptionID != 0) ||
			(operation.BillingSource == "subscription" && operation.SubscriptionID <= 0) || operation.ReservedQuota < 0 || operation.ReservedQuota > taskBillingQuotaMax || operation.EstimatedQuota < 0 || operation.EstimatedQuota > taskBillingQuotaMax || (operation.BillingVersion != 0 && operation.BillingVersion != 2 && operation.BillingVersion != 3) {
			return fmt.Errorf("%w: stored task submission operation has an invalid billing selection", ErrTaskRecoveryInvalidRecord)
		}
	}
	if operation.Status == TaskSubmissionOperationStatusPrepared && selectionFrozen {
		return fmt.Errorf("%w: prepared task submission operation cannot have a billing selection", ErrTaskRecoveryInvalidRecord)
	}
	if operation.Status.Terminal() {
		if operation.ResolvedAt == nil || *operation.ResolvedAt <= 0 || operation.RetentionUntil == nil ||
			*operation.ResolvedAt > taskRecoveryMaxInt64-TaskSubmissionTerminalRetentionSeconds ||
			*operation.RetentionUntil != *operation.ResolvedAt+TaskSubmissionTerminalRetentionSeconds {
			return fmt.Errorf("%w: stored terminal task submission operation has an invalid retention deadline", ErrTaskRecoveryInvalidRecord)
		}
	} else if operation.ResolvedAt != nil || operation.RetentionUntil != nil {
		return fmt.Errorf("%w: stored active task submission operation cannot have a retention deadline", ErrTaskRecoveryInvalidRecord)
	}

	requiresDispatch := operation.Status == TaskSubmissionOperationStatusDispatching ||
		operation.Status == TaskSubmissionOperationStatusSubmissionUnknown ||
		operation.Status == TaskSubmissionOperationStatusAccepted ||
		operation.Status == TaskSubmissionOperationStatusOutcomeUnknown ||
		operation.Status == TaskSubmissionOperationStatusSucceeded ||
		operation.Status == TaskSubmissionOperationStatusFailed
	if requiresDispatch && (operation.DispatchStartedAt == nil || *operation.DispatchStartedAt <= 0) {
		return fmt.Errorf("%w: stored task submission operation is missing its dispatch timestamp", ErrTaskRecoveryInvalidRecord)
	}
	if (operation.Status == TaskSubmissionOperationStatusPrepared || operation.Status == TaskSubmissionOperationStatusReserved || operation.Status == TaskSubmissionOperationStatusCanceled) && operation.DispatchStartedAt != nil {
		return fmt.Errorf("%w: stored task submission operation has an impossible dispatch timestamp", ErrTaskRecoveryInvalidRecord)
	}

	requiresTask := operation.Status == TaskSubmissionOperationStatusAccepted || operation.Status == TaskSubmissionOperationStatusOutcomeUnknown ||
		operation.Status == TaskSubmissionOperationStatusSucceeded || operation.Status == TaskSubmissionOperationStatusFailed
	if requiresTask && (operation.TaskID == nil || *operation.TaskID <= 0) {
		return fmt.Errorf("%w: stored task submission operation is missing its formal task", ErrTaskRecoveryInvalidRecord)
	}
	if !requiresTask && operation.TaskID != nil {
		return fmt.Errorf("%w: stored task submission operation has an impossible formal task", ErrTaskRecoveryInvalidRecord)
	}
	if operation.TaskID == nil {
		return nil
	}
	var task Task
	if err := tx.Select("id", "task_id", "user_id").Where("id = ?", *operation.TaskID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskSubmissionTaskNotFound
		}
		return err
	}
	if task.TaskID != operation.PublicID || task.UserId != operation.UserID {
		return ErrTaskSubmissionTaskIDMismatch
	}
	return nil
}

func validateStoredTaskBillingEventState(event *TaskBillingEvent) error {
	if event == nil || event.AttemptCount < 0 || event.AttemptCount > taskRecoveryAttemptCountMax ||
		event.ClaimedBy != strings.TrimSpace(event.ClaimedBy) || len(event.ClaimedBy) > 128 ||
		event.ClaimedUntil < 0 || event.NextAttemptAt < 0 || event.LastErrorAt < 0 ||
		event.LastErrorCode != strings.TrimSpace(event.LastErrorCode) || len(event.LastErrorCode) > 64 ||
		(event.LastErrorCode == "") != (event.LastErrorAt == 0) ||
		(event.AppliedAt != nil && (*event.AppliedAt <= 0 || *event.AppliedAt > event.UpdatedAt)) {
		return fmt.Errorf("%w: stored task billing event has invalid processing state", ErrTaskRecoveryInvalidRecord)
	}
	switch event.State {
	case TaskBillingEventStatePending:
		if event.ClaimedBy != "" || event.ClaimedUntil != 0 || event.AttemptCount != 0 || event.NextAttemptAt != 0 ||
			event.LastErrorCode != "" || event.LastErrorAt != 0 || event.AppliedAt != nil {
			return fmt.Errorf("%w: stored pending task billing event contains processing state", ErrTaskRecoveryInvalidRecord)
		}
	case TaskBillingEventStateClaimed:
		if event.ClaimedBy == "" || event.ClaimedUntil <= event.UpdatedAt || event.AttemptCount <= 0 || event.NextAttemptAt != 0 || event.AppliedAt != nil ||
			(event.LastErrorAt != 0 && event.LastErrorAt > event.UpdatedAt) {
			return fmt.Errorf("%w: stored claimed task billing event has an invalid lease", ErrTaskRecoveryInvalidRecord)
		}
	case TaskBillingEventStateRetryable:
		if event.ClaimedBy != "" || event.ClaimedUntil != 0 || event.AttemptCount <= 0 || event.NextAttemptAt <= event.UpdatedAt ||
			event.LastErrorCode == "" || event.LastErrorAt <= 0 || event.LastErrorAt > event.UpdatedAt || event.AppliedAt != nil {
			return fmt.Errorf("%w: stored retryable task billing event has an invalid retry", ErrTaskRecoveryInvalidRecord)
		}
	case TaskBillingEventStateApplied:
		if event.ClaimedBy != "" || event.ClaimedUntil != 0 || event.AttemptCount <= 0 || event.NextAttemptAt != 0 || event.AppliedAt == nil ||
			(event.LastErrorAt != 0 && event.LastErrorAt > *event.AppliedAt) {
			return fmt.Errorf("%w: stored applied task billing event has an invalid settlement", ErrTaskRecoveryInvalidRecord)
		}
	case TaskBillingEventStateManualReview:
		if event.ClaimedBy != "" || event.ClaimedUntil != 0 || event.AttemptCount <= 0 || event.NextAttemptAt != 0 ||
			event.LastErrorCode == "" || event.LastErrorAt <= 0 || event.LastErrorAt > event.UpdatedAt || event.AppliedAt != nil {
			return fmt.Errorf("%w: stored manual-review task billing event has an invalid failure state", ErrTaskRecoveryInvalidRecord)
		}
	}
	return nil
}

func validateStoredTaskBillingEvent(tx *gorm.DB, event *TaskBillingEvent) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	if event == nil || event.ID <= 0 || !validTaskBillingEventID(event.EventID) || event.EventKey == "" ||
		!event.EventType.Valid() || !event.State.Valid() || event.OperationID == nil || *event.OperationID <= 0 ||
		event.UserID <= 0 || event.TokenID <= 0 || event.ChannelID <= 0 || event.SubscriptionID < 0 ||
		int64(event.UserID) > taskBillingQuotaMax || int64(event.TokenID) > taskBillingQuotaMax || int64(event.ChannelID) > taskBillingQuotaMax || int64(event.SubscriptionID) > taskBillingQuotaMax ||
		event.BillingSource == "" || event.LockVersion <= 0 || event.CreatedAt <= 0 || event.UpdatedAt < event.CreatedAt ||
		event.QuotaDelta < taskBillingQuotaMin || event.QuotaDelta > taskBillingQuotaMax ||
		len(event.EventKey) > 128 || len(event.BillingSource) > 32 || len(event.RequestID) > 64 || len(event.NodeName) > 128 ||
		len(event.ReasonCode) > 64 || len(event.ResolutionSource) > 32 || len(event.AuditCommandID) > taskBillingAuditCommandIDMaxLength || len(event.EvidenceID) > 191 ||
		!validTaskMutationEvidence(event.EvidenceID, event.EvidenceHash, event.EvidenceVersion) || event.PayloadVersion != TaskRecoveryPayloadVersion || event.Payload != taskBillingEventPayloadFromRecord(event) {
		return fmt.Errorf("%w: stored task billing event is not a valid immutable v1 record", ErrTaskRecoveryInvalidRecord)
	}
	if event.BillingSource != "wallet" && event.BillingSource != "subscription" ||
		(event.BillingSource == "wallet" && event.SubscriptionID != 0) ||
		(event.BillingSource == "subscription" && event.SubscriptionID <= 0) ||
		(event.EventType == TaskBillingEventTypeReserve && event.QuotaDelta > 0) ||
		(event.EventType == TaskBillingEventTypeRefund && event.QuotaDelta < 0) ||
		(event.ResolutionSource != "" && event.ResolutionSource != TaskSubmissionResolutionSourceProviderVerified && event.ResolutionSource != TaskSubmissionResolutionSourceManualAudit) ||
		(event.EventType == TaskBillingEventTypeManualResolution && (!validTaskBillingAuditCommandID(event.AuditCommandID) || event.ResolutionSource != TaskSubmissionResolutionSourceManualAudit)) ||
		(event.EventType != TaskBillingEventTypeManualResolution && (event.AuditCommandID != "" || event.ResolutionSource == TaskSubmissionResolutionSourceManualAudit)) {
		return fmt.Errorf("%w: stored task billing event has invalid funding, amount, or audit metadata", ErrTaskRecoveryInvalidRecord)
	}
	if (event.StatisticsApplied && event.StatisticsVersion != 1) || (!event.StatisticsApplied && event.StatisticsVersion != 0) {
		return fmt.Errorf("%w: stored task billing event has invalid statistics evidence", ErrTaskRecoveryInvalidRecord)
	}
	if err := validateStoredTaskBillingEventState(event); err != nil {
		return err
	}

	var operation TaskSubmissionOperation
	if err := tx.Where("id = ?", *event.OperationID).First(&operation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskSubmissionOperationNotFound
		}
		return err
	}
	if err := validateStoredTaskSubmissionOperation(tx, &operation); err != nil {
		return err
	}
	if operation.UserID != event.UserID || operation.TokenID != event.TokenID {
		return fmt.Errorf("%w: stored task billing event does not match its operation owner", ErrTaskRecoveryInvalidRecord)
	}
	if event.TaskID != nil {
		if *event.TaskID <= 0 || operation.TaskID == nil || *operation.TaskID != *event.TaskID {
			return ErrTaskSubmissionTaskIDMismatch
		}
		var task Task
		if err := tx.Select("id", "task_id", "user_id", "channel_id").Where("id = ?", *event.TaskID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTaskSubmissionTaskNotFound
			}
			return err
		}
		if task.TaskID != operation.PublicID || task.UserId != event.UserID || (task.ChannelId > 0 && task.ChannelId != event.ChannelID) {
			return ErrTaskSubmissionTaskIDMismatch
		}
	}
	var attempt TaskSubmissionAttempt
	if err := tx.Where("operation_id = ?", operation.ID).First(&attempt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: stored task billing event is missing its v1 submission attempt", ErrTaskRecoveryInvalidRecord)
		}
		return err
	}
	if err := validateStoredTaskSubmissionAttempt(tx, &attempt); err != nil {
		return err
	}
	if attempt.ChannelID != event.ChannelID {
		return fmt.Errorf("%w: stored task billing event does not match its submission attempt channel", ErrTaskRecoveryInvalidRecord)
	}
	if event.ResolutionSource == TaskSubmissionResolutionSourceProviderVerified &&
		(event.EvidenceVersion != 1 || event.EvidenceID == "" || (event.EvidenceID != attempt.ProviderOperationID && event.EvidenceID != attempt.UpstreamRequestID)) {
		return fmt.Errorf("%w: stored provider-verified billing event evidence is not bound to its attempt", ErrTaskRecoveryInvalidRecord)
	}
	canonicalEventKey := "task:" + operation.PublicID + ":" + string(event.EventType)
	if event.EventType == TaskBillingEventTypeManualResolution {
		canonicalEventKey += ":" + event.AuditCommandID
	}
	canonicalEventKey += ":v1"
	if event.EventKey != canonicalEventKey {
		return fmt.Errorf("%w: stored task billing event key is not canonical", ErrTaskRecoveryInvalidRecord)
	}
	return nil
}

// CreateOrLoadTaskBillingEvent preserves the single canonical event for one
// operation/type business action. The Task reference is deliberately excluded
// from the semantic comparison: an event may be recorded before formal Task
// creation and later replayed through that Task's public operation reference.
func CreateOrLoadTaskBillingEvent(tx *gorm.DB, candidate *TaskBillingEvent) (*TaskBillingEvent, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	if candidate == nil {
		return nil, ErrTaskRecoveryInvalidRecord
	}
	writeDB := tx.Session(&gorm.Session{NewDB: true})
	record := *candidate
	if err := writeDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
		return nil, err
	}
	existing, err := loadTaskBillingEventByEventKeyForReplay(writeDB, record.EventKey)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		// Do not turn a collision on EventID into a false event replay.
		return nil, ErrTaskBillingEventConflict
	}
	if err := validateStoredTaskBillingEvent(writeDB, existing); err != nil {
		return existing, err
	}
	if !taskBillingEventSemanticallyMatches(existing, &record) {
		return existing, ErrTaskBillingEventConflict
	}
	return existing, nil
}

func taskBillingEventSemanticallyMatches(existing, candidate *TaskBillingEvent) bool {
	if existing == nil || candidate == nil || existing.OperationID == nil || candidate.OperationID == nil {
		return false
	}
	return existing.EventKey == candidate.EventKey &&
		*existing.OperationID == *candidate.OperationID &&
		existing.EventType == candidate.EventType &&
		existing.UserID == candidate.UserID &&
		existing.TokenID == candidate.TokenID &&
		existing.ChannelID == candidate.ChannelID &&
		existing.BillingSource == candidate.BillingSource &&
		existing.SubscriptionID == candidate.SubscriptionID &&
		existing.QuotaDelta == candidate.QuotaDelta &&
		existing.RequestID == candidate.RequestID &&
		existing.NodeName == candidate.NodeName &&
		existing.ReasonCode == candidate.ReasonCode &&
		existing.ResolutionSource == candidate.ResolutionSource &&
		existing.AuditCommandID == candidate.AuditCommandID &&
		existing.EvidenceID == candidate.EvidenceID &&
		existing.EvidenceHash == candidate.EvidenceHash &&
		existing.EvidenceVersion == candidate.EvidenceVersion &&
		existing.StatisticsVersion == candidate.StatisticsVersion &&
		existing.StatisticsApplied == candidate.StatisticsApplied
}

type TaskBillingLogOutboxState string

const (
	TaskBillingLogOutboxStatePending     TaskBillingLogOutboxState = "pending"
	TaskBillingLogOutboxStateClaimed     TaskBillingLogOutboxState = "claimed"
	TaskBillingLogOutboxStateRetryable   TaskBillingLogOutboxState = "retryable"
	TaskBillingLogOutboxStateDelivered   TaskBillingLogOutboxState = "delivered"
	TaskBillingLogOutboxStateQuarantined TaskBillingLogOutboxState = "quarantined"
)

func (state TaskBillingLogOutboxState) Valid() bool {
	switch state {
	case TaskBillingLogOutboxStatePending,
		TaskBillingLogOutboxStateClaimed,
		TaskBillingLogOutboxStateRetryable,
		TaskBillingLogOutboxStateDelivered,
		TaskBillingLogOutboxStateQuarantined:
		return true
	default:
		return false
	}
}

func CanTransitionTaskBillingLogOutbox(from, to TaskBillingLogOutboxState) bool {
	switch from {
	case TaskBillingLogOutboxStatePending, TaskBillingLogOutboxStateRetryable:
		return to == TaskBillingLogOutboxStateClaimed
	case TaskBillingLogOutboxStateClaimed:
		return to == TaskBillingLogOutboxStateRetryable || to == TaskBillingLogOutboxStateDelivered || to == TaskBillingLogOutboxStateQuarantined
	default:
		return false
	}
}

// TaskBillingLogPayload is the immutable, complete v1 Log projection copied to
// the outbox in the same main-database transaction as its billing event. The
// producer must supply already-sanitized billing log text; raw request bodies,
// prompts, upstream responses and credentials are forbidden.
type TaskBillingLogPayload struct {
	Version           int    `json:"version"`
	BillingEventID    string `json:"billing_event_id"`
	UserID            int    `json:"user_id"`
	CreatedAt         int64  `json:"created_at"`
	Type              int    `json:"type"`
	Content           string `json:"content"`
	Username          string `json:"username"`
	TokenName         string `json:"token_name"`
	ModelName         string `json:"model_name"`
	Quota             int    `json:"quota"`
	PromptTokens      int    `json:"prompt_tokens"`
	CompletionTokens  int    `json:"completion_tokens"`
	UseTime           int    `json:"use_time"`
	IsStream          bool   `json:"is_stream"`
	ChannelID         int    `json:"channel_id"`
	TokenID           int    `json:"token_id"`
	Group             string `json:"group"`
	IP                string `json:"ip"`
	RequestID         string `json:"request_id"`
	UpstreamRequestID string `json:"upstream_request_id"`
	Other             string `json:"other"`
}

// NewTaskBillingLogOutbox derives every accounting field from an authoritative
// event. The caller may provide only sanitized display and usage details; the
// hook reloads the event and verifies the complete projection at insert time.
func NewTaskBillingLogOutbox(event *TaskBillingEvent, details TaskBillingLogPayload) (*TaskBillingLogOutbox, error) {
	if event == nil || event.ID <= 0 || event.EventID == "" {
		return nil, fmt.Errorf("%w: a persisted billing event is required for its log projection", ErrTaskRecoveryInvalidRecord)
	}
	if event.QuotaDelta < taskBillingQuotaMin || event.QuotaDelta > taskBillingQuotaMax {
		return nil, fmt.Errorf("%w: billing event quota cannot be projected outside the int32 boundary", ErrTaskRecoveryInvalidRecord)
	}
	details.Version = TaskRecoveryPayloadVersion
	details.BillingEventID = event.EventID
	details.UserID = event.UserID
	details.CreatedAt = event.CreatedAt
	details.ChannelID = event.ChannelID
	details.TokenID = event.TokenID
	details.RequestID = event.RequestID
	switch {
	case event.QuotaDelta < 0:
		details.Type = LogTypeConsume
		details.Quota = int(-event.QuotaDelta)
	case event.QuotaDelta > 0:
		details.Type = LogTypeRefund
		details.Quota = int(event.QuotaDelta)
	default:
		details.Type = LogTypeSystem
		details.Quota = 0
	}
	if err := validateTaskBillingLogPayload(details); err != nil {
		return nil, err
	}
	return &TaskBillingLogOutbox{
		BillingEventID: event.EventID,
		PayloadVersion: TaskRecoveryPayloadVersion,
		Payload:        details,
	}, nil
}

func taskBillingLogPayloadMatchesEvent(payload TaskBillingLogPayload, event *TaskBillingEvent) bool {
	if event == nil || payload.Version != TaskRecoveryPayloadVersion || payload.BillingEventID != event.EventID ||
		payload.UserID != event.UserID || payload.CreatedAt != event.CreatedAt || payload.ChannelID != event.ChannelID ||
		payload.TokenID != event.TokenID || payload.RequestID != event.RequestID {
		return false
	}
	switch {
	case event.QuotaDelta < 0:
		return payload.Type == LogTypeConsume && int64(payload.Quota) == -event.QuotaDelta
	case event.QuotaDelta > 0:
		return payload.Type == LogTypeRefund && int64(payload.Quota) == event.QuotaDelta
	default:
		return payload.Type == LogTypeSystem && payload.Quota == 0
	}
}

func validateTaskBillingLogPayload(payload TaskBillingLogPayload) error {
	if payload.Version != TaskRecoveryPayloadVersion || payload.BillingEventID == "" || payload.UserID <= 0 || payload.CreatedAt <= 0 ||
		int64(payload.UserID) > taskBillingQuotaMax || payload.TokenID <= 0 || int64(payload.TokenID) > taskBillingQuotaMax ||
		payload.ChannelID < 0 || int64(payload.ChannelID) > taskBillingQuotaMax || payload.Quota < 0 || int64(payload.Quota) > taskBillingQuotaMax ||
		payload.PromptTokens < 0 || int64(payload.PromptTokens) > taskBillingQuotaMax ||
		payload.CompletionTokens < 0 || int64(payload.CompletionTokens) > taskBillingQuotaMax ||
		payload.UseTime < 0 || int64(payload.UseTime) > taskBillingQuotaMax {
		return fmt.Errorf("%w: billing log projection contains an invalid value", ErrTaskRecoveryInvalidRecord)
	}
	if payload.Content == "" || payload.ModelName == "" || payload.Group == "" {
		return fmt.Errorf("%w: billing log projection is incomplete", ErrTaskRecoveryInvalidRecord)
	}
	if len(payload.BillingEventID) > 64 || len(payload.Content) > 4096 || len(payload.Username) > 191 || len(payload.TokenName) > 191 ||
		len(payload.ModelName) > 191 || len(payload.Group) > 191 || len(payload.IP) > 64 || len(payload.RequestID) > 64 ||
		len(payload.UpstreamRequestID) > 128 || len(payload.Other) > 16*1024 {
		return fmt.Errorf("%w: billing log projection exceeds its storage boundary", ErrTaskRecoveryInvalidRecord)
	}
	return nil
}

func (payload *TaskBillingLogPayload) Scan(value interface{}) error {
	*payload = TaskBillingLogPayload{}
	data, err := taskRecoveryTextValue(value)
	if err != nil || len(data) == 0 {
		return err
	}
	return common.Unmarshal(data, payload)
}

func (payload TaskBillingLogPayload) Value() (driver.Value, error) {
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// TaskBillingLogOutbox gives each authoritative billing event exactly one
// durable log projection. BillingEventID is the stable event identifier copied
// to Log.BillingEventID; consumers must tolerate at-least-once delivery and
// deduplicate by that value.
type TaskBillingLogOutbox struct {
	ID             int64                     `json:"id" gorm:"primaryKey"`
	BillingEventID string                    `json:"billing_event_id" gorm:"type:varchar(64);not null;uniqueIndex:uidx_task_billing_outbox_event;<-:create"`
	State          TaskBillingLogOutboxState `json:"state" gorm:"type:varchar(24);not null;index:idx_task_billing_outbox_ready,priority:1;<-:create"`
	ClaimedBy      string                    `json:"claimed_by,omitempty" gorm:"type:varchar(128);index;<-:create"`
	ClaimedUntil   int64                     `json:"claimed_until,omitempty" gorm:"type:bigint;index;<-:create"`
	AttemptCount   int                       `json:"attempt_count" gorm:"not null;<-:create"`
	NextAttemptAt  int64                     `json:"next_attempt_at,omitempty" gorm:"type:bigint;index:idx_task_billing_outbox_ready,priority:2;<-:create"`
	LastErrorCode  string                    `json:"last_error_code,omitempty" gorm:"type:varchar(64);<-:create"`
	LastErrorAt    int64                     `json:"last_error_at,omitempty" gorm:"type:bigint;<-:create"`
	DeliveredAt    *int64                    `json:"delivered_at,omitempty" gorm:"type:bigint;<-:create"`
	PayloadVersion int                       `json:"payload_version" gorm:"not null;<-:create"`
	Payload        TaskBillingLogPayload     `json:"-" gorm:"type:text;not null;<-:create"`
	LockVersion    int64                     `json:"lock_version" gorm:"type:bigint;not null;<-:create"`
	CreatedAt      int64                     `json:"created_at" gorm:"type:bigint;index;<-:create"`
	UpdatedAt      int64                     `json:"updated_at" gorm:"type:bigint;<-:create"`
}

func (outbox *TaskBillingLogOutbox) BeforeCreate(tx *gorm.DB) error {
	if outbox == nil {
		return ErrTaskRecoveryInvalidRecord
	}
	outbox.BillingEventID = strings.TrimSpace(outbox.BillingEventID)
	if outbox.State == "" {
		outbox.State = TaskBillingLogOutboxStatePending
	}
	if !validTaskBillingEventID(outbox.BillingEventID) || outbox.State != TaskBillingLogOutboxStatePending {
		return fmt.Errorf("%w: new billing log outbox entry is incomplete or not pending", ErrTaskRecoveryInvalidRecord)
	}
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	if outbox.ClaimedBy != "" || outbox.ClaimedUntil != 0 || outbox.AttemptCount != 0 || outbox.NextAttemptAt != 0 || outbox.LastErrorCode != "" || outbox.LastErrorAt != 0 || outbox.DeliveredAt != nil {
		return fmt.Errorf("%w: a pending billing log outbox entry cannot contain delivery state", ErrTaskRecoveryInvalidRecord)
	}
	var event TaskBillingEvent
	if err := tx.Where("event_id = ?", outbox.BillingEventID).First(&event).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskBillingEventNotFound
		}
		return err
	}
	if err := validateStoredTaskBillingEvent(tx, &event); err != nil {
		return err
	}
	if outbox.PayloadVersion == 0 {
		outbox.PayloadVersion = outbox.Payload.Version
	}
	if outbox.PayloadVersion != TaskRecoveryPayloadVersion || !taskBillingLogPayloadMatchesEvent(outbox.Payload, &event) {
		return fmt.Errorf("%w: billing log outbox requires a matching immutable v1 projection", ErrTaskRecoveryInvalidRecord)
	}
	if err := validateTaskBillingLogPayload(outbox.Payload); err != nil {
		return err
	}
	if outbox.LockVersion == 0 {
		outbox.LockVersion = 1
	}
	if outbox.LockVersion != 1 {
		return fmt.Errorf("%w: new billing log outbox lock version must be one", ErrTaskRecoveryInvalidRecord)
	}
	if outbox.CreatedAt != 0 || outbox.UpdatedAt != 0 {
		return fmt.Errorf("%w: billing log outbox timestamps are assigned by the database clock", ErrTaskRecoveryInvalidRecord)
	}
	now, err := taskRecoveryDBTimestamp(tx)
	if err != nil {
		return err
	}
	outbox.CreatedAt = now
	outbox.UpdatedAt = now
	return nil
}

func (outbox *TaskBillingLogOutbox) BeforeUpdate(_ *gorm.DB) error {
	return fmt.Errorf("%w: billing log outbox entries may only change through the state CAS", ErrTaskRecoveryInvalidRecord)
}

func (outbox *TaskBillingLogOutbox) BeforeDelete(_ *gorm.DB) error {
	return fmt.Errorf("%w: billing log outbox entries are immutable delivery records", ErrTaskRecoveryInvalidRecord)
}

func loadTaskBillingLogOutboxForReplay(tx *gorm.DB, billingEventID string) (*TaskBillingLogOutbox, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	billingEventID = strings.TrimSpace(billingEventID)
	if !validTaskBillingEventID(billingEventID) {
		return nil, ErrTaskRecoveryInvalidRecord
	}
	var outbox TaskBillingLogOutbox
	err := lockForUpdate(tx).Where("billing_event_id = ?", billingEventID).First(&outbox).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &outbox, nil
}

func validateStoredTaskBillingLogOutbox(outbox *TaskBillingLogOutbox, event *TaskBillingEvent) error {
	if outbox == nil || outbox.ID <= 0 || !validTaskBillingEventID(outbox.BillingEventID) || !outbox.State.Valid() ||
		outbox.LockVersion <= 0 || outbox.CreatedAt <= 0 || outbox.UpdatedAt < outbox.CreatedAt ||
		outbox.PayloadVersion != TaskRecoveryPayloadVersion || !taskBillingLogPayloadMatchesEvent(outbox.Payload, event) {
		return fmt.Errorf("%w: stored task billing log outbox is not a valid immutable v1 projection", ErrTaskRecoveryInvalidRecord)
	}
	if err := validateStoredTaskBillingLogOutboxState(outbox); err != nil {
		return err
	}
	return validateTaskBillingLogPayload(outbox.Payload)
}

func validateStoredTaskBillingLogOutboxState(outbox *TaskBillingLogOutbox) error {
	if outbox == nil || outbox.AttemptCount < 0 || outbox.AttemptCount > taskRecoveryAttemptCountMax ||
		outbox.ClaimedBy != strings.TrimSpace(outbox.ClaimedBy) || len(outbox.ClaimedBy) > 128 ||
		outbox.ClaimedUntil < 0 || outbox.NextAttemptAt < 0 || outbox.LastErrorAt < 0 ||
		outbox.LastErrorCode != strings.TrimSpace(outbox.LastErrorCode) || len(outbox.LastErrorCode) > 64 ||
		(outbox.LastErrorCode == "") != (outbox.LastErrorAt == 0) ||
		(outbox.DeliveredAt != nil && (*outbox.DeliveredAt <= 0 || *outbox.DeliveredAt > outbox.UpdatedAt)) {
		return fmt.Errorf("%w: stored task billing log outbox has invalid delivery state", ErrTaskRecoveryInvalidRecord)
	}
	switch outbox.State {
	case TaskBillingLogOutboxStatePending:
		if outbox.ClaimedBy != "" || outbox.ClaimedUntil != 0 || outbox.AttemptCount != 0 || outbox.NextAttemptAt != 0 ||
			outbox.LastErrorCode != "" || outbox.LastErrorAt != 0 || outbox.DeliveredAt != nil {
			return fmt.Errorf("%w: stored pending task billing log outbox contains delivery state", ErrTaskRecoveryInvalidRecord)
		}
	case TaskBillingLogOutboxStateClaimed:
		if outbox.ClaimedBy == "" || outbox.ClaimedUntil <= outbox.UpdatedAt || outbox.AttemptCount <= 0 || outbox.NextAttemptAt != 0 || outbox.DeliveredAt != nil ||
			(outbox.LastErrorAt != 0 && outbox.LastErrorAt > outbox.UpdatedAt) {
			return fmt.Errorf("%w: stored claimed task billing log outbox has an invalid lease", ErrTaskRecoveryInvalidRecord)
		}
	case TaskBillingLogOutboxStateRetryable:
		if outbox.ClaimedBy != "" || outbox.ClaimedUntil != 0 || outbox.AttemptCount <= 0 || outbox.NextAttemptAt <= outbox.UpdatedAt ||
			outbox.LastErrorCode == "" || outbox.LastErrorAt <= 0 || outbox.LastErrorAt > outbox.UpdatedAt || outbox.DeliveredAt != nil {
			return fmt.Errorf("%w: stored retryable task billing log outbox has an invalid retry", ErrTaskRecoveryInvalidRecord)
		}
	case TaskBillingLogOutboxStateDelivered:
		if outbox.ClaimedBy != "" || outbox.ClaimedUntil != 0 || outbox.AttemptCount <= 0 || outbox.NextAttemptAt != 0 || outbox.DeliveredAt == nil ||
			(outbox.LastErrorAt != 0 && outbox.LastErrorAt > *outbox.DeliveredAt) {
			return fmt.Errorf("%w: stored delivered task billing log outbox has an invalid receipt", ErrTaskRecoveryInvalidRecord)
		}
	case TaskBillingLogOutboxStateQuarantined:
		if outbox.ClaimedBy != "" || outbox.ClaimedUntil != 0 || outbox.AttemptCount <= 0 || outbox.NextAttemptAt != 0 ||
			outbox.LastErrorCode == "" || outbox.LastErrorAt <= 0 || outbox.LastErrorAt > outbox.UpdatedAt || outbox.DeliveredAt != nil {
			return fmt.Errorf("%w: stored quarantined task billing log outbox has invalid terminal state", ErrTaskRecoveryInvalidRecord)
		}
	}
	return nil
}

// CreateOrLoadTaskBillingLogOutbox gives an authoritative billing event one
// durable projection receipt. Its immutable payload must match exactly on a
// retry; different display or usage details cannot silently attach to the
// existing event.
func CreateOrLoadTaskBillingLogOutbox(tx *gorm.DB, candidate *TaskBillingLogOutbox) (*TaskBillingLogOutbox, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	if candidate == nil {
		return nil, ErrTaskRecoveryInvalidRecord
	}
	writeDB := tx.Session(&gorm.Session{NewDB: true})
	record := *candidate
	record.BillingEventID = strings.TrimSpace(record.BillingEventID)
	event, err := loadTaskBillingEventByIDForReplay(writeDB, record.BillingEventID)
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, ErrTaskBillingEventNotFound
	}
	if err := validateStoredTaskBillingEvent(writeDB, event); err != nil {
		return nil, err
	}
	if err := writeDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
		return nil, err
	}
	existing, err := loadTaskBillingLogOutboxForReplay(writeDB, record.BillingEventID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrTaskBillingLogOutboxConflict
	}
	if err := validateStoredTaskBillingLogOutbox(existing, event); err != nil {
		return existing, err
	}
	if existing.PayloadVersion != record.PayloadVersion || existing.Payload != record.Payload {
		return existing, ErrTaskBillingLogOutboxConflict
	}
	return existing, nil
}

type TaskBillingLogOutboxTransition struct {
	From              TaskBillingLogOutboxState
	To                TaskBillingLogOutboxState
	Lease             TaskRecoveryProcessingLease
	WorkerID          string
	RetryDelaySeconds int64
	LastErrorCode     string
	ExpectedVersion   int64
	TransitionedAt    int64
}

func TransitionTaskBillingLogOutbox(tx *gorm.DB, outboxID int64, transition TaskBillingLogOutboxTransition) (bool, error) {
	if tx == nil {
		return false, gorm.ErrInvalidDB
	}
	if outboxID <= 0 || transition.ExpectedVersion <= 0 || transition.ExpectedVersion == taskRecoveryMaxInt64 ||
		!CanTransitionTaskBillingLogOutbox(transition.From, transition.To) {
		return false, ErrTaskRecoveryInvalidTransition
	}
	transitionedAt, err := taskRecoveryValidatedTime(tx, transition.TransitionedAt)
	if err != nil {
		return false, err
	}
	updates := map[string]interface{}{
		"state":        transition.To,
		"updated_at":   transitionedAt,
		"lock_version": gorm.Expr("lock_version + ?", 1),
	}
	query := taskRecoveryControlledWrite(tx).Table("task_billing_log_outboxes").Where(
		"id = ? AND state = ? AND lock_version = ? AND updated_at <= ?", outboxID, transition.From, transition.ExpectedVersion, transitionedAt,
	)
	if transition.To == TaskBillingLogOutboxStateClaimed {
		lease, err := validateTaskRecoveryProcessingLease(transition.Lease, transitionedAt)
		if err != nil {
			return false, err
		}
		if transition.WorkerID != "" || transition.RetryDelaySeconds != 0 || transition.LastErrorCode != "" {
			return false, fmt.Errorf("%w: claim transitions accept only a processing lease", ErrTaskRecoveryInvalidRecord)
		}
		query = query.Where("next_attempt_at <= ? AND attempt_count < ?", transitionedAt, taskRecoveryAttemptCountMax)
		updates["claimed_by"] = lease.WorkerID
		updates["claimed_until"] = lease.LeaseUntil
		updates["attempt_count"] = gorm.Expr("attempt_count + ?", 1)
		updates["next_attempt_at"] = 0
	} else {
		workerID, err := validateTaskRecoveryWorkerID(transition.WorkerID)
		if err != nil {
			return false, err
		}
		if transition.Lease != (TaskRecoveryProcessingLease{}) {
			return false, fmt.Errorf("%w: completion transitions use the current persisted lease", ErrTaskRecoveryInvalidRecord)
		}
		query = query.Where("claimed_by = ? AND claimed_until > ?", workerID, transitionedAt)
		updates["claimed_by"] = ""
		updates["claimed_until"] = 0
		switch transition.To {
		case TaskBillingLogOutboxStateDelivered:
			if transition.RetryDelaySeconds != 0 || transition.LastErrorCode != "" {
				return false, fmt.Errorf("%w: delivered outbox entries cannot retain a retry failure", ErrTaskRecoveryInvalidRecord)
			}
			updates["delivered_at"] = transitionedAt
		case TaskBillingLogOutboxStateRetryable:
			errorCode, nextAttemptAt, err := validateTaskRecoveryFailure(transition.LastErrorCode, transition.RetryDelaySeconds, transitionedAt)
			if err != nil {
				return false, err
			}
			updates["next_attempt_at"] = nextAttemptAt
			updates["last_error_code"] = errorCode
			updates["last_error_at"] = transitionedAt
		case TaskBillingLogOutboxStateQuarantined:
			errorCode := strings.TrimSpace(transition.LastErrorCode)
			if transition.RetryDelaySeconds != 0 || errorCode == "" || len(errorCode) > 64 {
				return false, fmt.Errorf("%w: quarantined outbox requires one terminal error code", ErrTaskRecoveryInvalidRecord)
			}
			updates["next_attempt_at"] = 0
			updates["last_error_code"] = errorCode
			updates["last_error_at"] = transitionedAt
		}
	}
	result := query.Updates(updates)
	return result.RowsAffected == 1, result.Error
}

func ReclaimExpiredTaskBillingLogOutbox(tx *gorm.DB, outboxID int64, lease TaskRecoveryProcessingLease, expectedVersion, checkedAt int64) (bool, error) {
	if tx == nil {
		return false, gorm.ErrInvalidDB
	}
	if outboxID <= 0 || expectedVersion <= 0 || expectedVersion == taskRecoveryMaxInt64 {
		return false, ErrTaskRecoveryInvalidTransition
	}
	checkedAt, err := taskRecoveryValidatedTime(tx, checkedAt)
	if err != nil {
		return false, err
	}
	validatedLease, err := validateTaskRecoveryProcessingLease(lease, checkedAt)
	if err != nil {
		return false, err
	}
	result := taskRecoveryControlledWrite(tx).Table("task_billing_log_outboxes").Where(
		"id = ? AND state = ? AND lock_version = ? AND updated_at <= ? AND claimed_until > 0 AND claimed_until <= ? AND attempt_count < ?",
		outboxID, TaskBillingLogOutboxStateClaimed, expectedVersion, checkedAt, checkedAt, taskRecoveryAttemptCountMax,
	).Updates(map[string]interface{}{
		"claimed_by":      validatedLease.WorkerID,
		"claimed_until":   validatedLease.LeaseUntil,
		"attempt_count":   gorm.Expr("attempt_count + ?", 1),
		"last_error_code": "lease_expired",
		"last_error_at":   checkedAt,
		"updated_at":      checkedAt,
		"lock_version":    gorm.Expr("lock_version + ?", 1),
	})
	return result.RowsAffected == 1, result.Error
}

func taskRecoveryTextValue(value interface{}) ([]byte, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return typed, nil
	case string:
		return []byte(typed), nil
	default:
		return nil, fmt.Errorf("unsupported task recovery payload database value %T", value)
	}
}

func validTaskRecoveryDigest(value string) bool {
	if len(value) != taskRecoveryDigestLength || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validTaskSubmissionOperationKind(value string) bool {
	switch value {
	case TaskSubmissionOperationKindVideoCreate,
		TaskSubmissionOperationKindVideoRemix,
		TaskSubmissionOperationKindSunoMusic,
		TaskSubmissionOperationKindSunoLyrics:
		return true
	default:
		return false
	}
}

func validTaskSubmissionPublicID(value string) bool {
	return validTaskRecoveryStableID(value, "task_")
}

// ValidTaskSubmissionPublicID reports whether value is a stable public task
// identifier. It performs no database lookup and does not disclose task
// ownership; protocol layers use it only to validate route-bound identifiers.
func ValidTaskSubmissionPublicID(value string) bool {
	return validTaskSubmissionPublicID(value)
}

func validTaskBillingEventID(value string) bool {
	return validTaskRecoveryStableID(value, "billing_evt_")
}

func validTaskRecoveryStableID(value, prefix string) bool {
	if len(value) != len(prefix)+taskSubmissionPublicIDRandomLength || !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, character := range value[len(prefix):] {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}

func validTaskBillingAuditCommandID(value string) bool {
	if value == "" || len(value) > taskBillingAuditCommandIDMaxLength {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}
