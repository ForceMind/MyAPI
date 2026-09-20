package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	promptLearningPolicyScopeMaxBytes = 128
	promptLearningReceiptMaxBytes     = 256
	promptLearningTextMaxBytes        = 64 * 1024
	promptInstructionContentMaxBytes  = 256 * 1024
	promptInstructionCommandMaxBytes  = 128
	promptInstructionTargetMaxBytes   = 256

	promptLearningSampleStateActive  = "active"
	promptLearningSampleStateDeleted = "deleted"

	promptInstructionVersionSourceManual    = "manual"
	promptInstructionVersionSourceGenerated = "generated"

	promptInstructionApplicationStatePending         = "pending"
	promptInstructionApplicationStateApplied         = "applied"
	promptInstructionApplicationStateRollbackPending = "rollback_pending"
	promptInstructionApplicationStateRolledBack      = "rolled_back"
	promptInstructionApplicationStateFailed          = "failed"
	promptInstructionApplicationStateOutcomeUnknown  = "outcome_unknown"
)

var (
	ErrPromptLearningPolicyNotFound = errors.New("prompt learning policy not found")
	ErrPromptLearningPolicyDisabled = errors.New("prompt learning policy is disabled")
	ErrPromptLearningPolicyStale    = errors.New("prompt learning policy generation is stale")
	ErrPromptLearningSampleConflict = errors.New("prompt learning sample occurrence conflicts")
	ErrPromptInstructionNotFound    = errors.New("prompt instruction version not found")
	ErrPromptInstructionConflict    = errors.New("prompt instruction command conflicts")
	ErrPromptInstructionImmutable   = errors.New("prompt instruction version is immutable")
	ErrPromptInstructionApplication = errors.New("invalid prompt instruction application transition")
	ErrPromptLearningInvalidInput   = errors.New("invalid prompt learning input")
)

// PromptLearningPolicy is the revocable, server-owned authorization boundary
// for one user and one opaque subject scope. It deliberately contains no raw
// conversation data, file path, provider credential, or model request.
// Enabled is always explicitly written by application logic; no database
// default is used so SQLite, MySQL and PostgreSQL share the same behavior.
type PromptLearningPolicy struct {
	ID         int64  `gorm:"primaryKey"`
	UserID     int    `gorm:"not null;uniqueIndex:idx_prompt_learning_policy_scope,priority:1;index"`
	ScopeRef   string `gorm:"type:varchar(128);not null;uniqueIndex:idx_prompt_learning_policy_scope,priority:2"`
	Enabled    bool   `gorm:"not null"`
	Generation int64  `gorm:"type:bigint;not null"`
	CreatedAt  int64  `gorm:"type:bigint;not null"`
	UpdatedAt  int64  `gorm:"type:bigint;not null"`
	RevokedAt  int64  `gorm:"type:bigint;not null"`
}

// PromptLearningSample stores only a bounded second-pass-redacted text and
// HMAC-derived references produced by service/promptlearning. The unique
// occurrence fingerprint gives the primary database authority over replay and
// conflict decisions; raw request payloads and credentials never enter this
// table.
type PromptLearningSample struct {
	ID                    int64  `gorm:"primaryKey"`
	PolicyID              int64  `gorm:"type:bigint;not null;index"`
	UserID                int    `gorm:"not null;uniqueIndex:idx_prompt_learning_sample_occurrence,priority:1;index"`
	ScopeRef              string `gorm:"type:varchar(128);not null;uniqueIndex:idx_prompt_learning_sample_occurrence,priority:2;index"`
	OccurrenceFingerprint string `gorm:"type:char(64);not null;uniqueIndex:idx_prompt_learning_sample_occurrence,priority:3"`
	SemanticFingerprint   string `gorm:"type:char(64);not null;index"`
	SourceReceipt         string `gorm:"type:varchar(256);not null"`
	RedactedText          string `gorm:"type:text;not null"`
	Confidence            string `gorm:"type:varchar(16);not null"`
	PolicyGeneration      int64  `gorm:"type:bigint;not null;index"`
	State                 string `gorm:"type:varchar(16);not null;index"`
	CreatedAt             int64  `gorm:"type:bigint;not null;index"`
	UpdatedAt             int64  `gorm:"type:bigint;not null"`
}

// PromptInstructionVersion is an immutable, scope-isolated instruction text.
// Manual edits and future generated drafts both append a new version; they
// cannot overwrite a reviewed historical version. CommandID is a caller's
// stable idempotency key, never an instruction body or filesystem path.
type PromptInstructionVersion struct {
	ID          int64  `gorm:"primaryKey"`
	UserID      int    `gorm:"not null;uniqueIndex:idx_prompt_instruction_command,priority:1;index"`
	ScopeRef    string `gorm:"type:varchar(128);not null;uniqueIndex:idx_prompt_instruction_command,priority:2;index"`
	CommandID   string `gorm:"type:varchar(128);not null;uniqueIndex:idx_prompt_instruction_command,priority:3"`
	ParentID    *int64 `gorm:"type:bigint;index"`
	Content     string `gorm:"type:text;not null"`
	ContentHash string `gorm:"type:char(64);not null;index"`
	Source      string `gorm:"type:varchar(16);not null"`
	CreatedBy   int    `gorm:"not null"`
	CreatedAt   int64  `gorm:"type:bigint;not null;index"`
}

// PromptInstructionApplication is a durable request to apply or roll back an
// immutable version. TargetReceipt and backup receipt are HMAC-derived opaque
// references; this table never stores a host path, a file body or credentials.
// The actual filesystem bridge remains a separate P4 responsibility.
type PromptInstructionApplication struct {
	ID                      int64  `gorm:"primaryKey"`
	UserID                  int    `gorm:"not null;uniqueIndex:idx_prompt_instruction_application_command,priority:1;index"`
	ScopeRef                string `gorm:"type:varchar(128);not null;uniqueIndex:idx_prompt_instruction_application_command,priority:2;index"`
	CommandID               string `gorm:"type:varchar(128);not null;uniqueIndex:idx_prompt_instruction_application_command,priority:3"`
	VersionID               int64  `gorm:"type:bigint;not null;index"`
	TargetReceipt           string `gorm:"type:char(64);not null;index"`
	AuthorizationGeneration int64  `gorm:"type:bigint;not null"`
	AuthorizationExpiresAt  int64  `gorm:"type:bigint;not null;index"`
	State                   string `gorm:"type:varchar(24);not null;index"`
	BackupReceipt           string `gorm:"type:char(64);not null;default:''"`
	AppliedContentHash      string `gorm:"type:char(64);not null;default:''"`
	RollbackOfID            *int64 `gorm:"type:bigint;index"`
	LockVersion             int64  `gorm:"type:bigint;not null"`
	CreatedBy               int    `gorm:"not null"`
	CreatedAt               int64  `gorm:"type:bigint;not null;index"`
	UpdatedAt               int64  `gorm:"type:bigint;not null"`
}

type PromptLearningPolicyInput struct {
	UserID   int
	ScopeRef string
	Enabled  bool
}

type PromptLearningSampleInput struct {
	UserID                int
	ScopeRef              string
	OccurrenceFingerprint string
	SemanticFingerprint   string
	SourceReceipt         string
	RedactedText          string
	Confidence            string
	PolicyGeneration      int64
}

type PromptInstructionVersionInput struct {
	UserID    int
	ScopeRef  string
	CommandID string
	ParentID  *int64
	Content   string
	Source    string
	CreatedBy int
}

type PromptInstructionApplicationInput struct {
	UserID                  int
	ScopeRef                string
	CommandID               string
	VersionID               int64
	TargetReceipt           string
	AuthorizationGeneration int64
	AuthorizationExpiresAt  int64
	RollbackOfID            *int64
	CreatedBy               int
}

func (PromptLearningPolicy) TableName() string { return "prompt_learning_policies" }

func (PromptLearningSample) TableName() string { return "prompt_learning_samples" }

func (PromptInstructionVersion) TableName() string { return "prompt_instruction_versions" }

func (PromptInstructionApplication) TableName() string { return "prompt_instruction_applications" }

func (*PromptInstructionVersion) BeforeUpdate(*gorm.DB) error { return ErrPromptInstructionImmutable }

func (*PromptInstructionVersion) BeforeDelete(*gorm.DB) error { return ErrPromptInstructionImmutable }

// UpsertPromptLearningPolicy creates a disabled/enabled policy only when the
// caller explicitly chooses its state. A changed state increments Generation;
// any previously admitted sample carries the old generation for audit.
func UpsertPromptLearningPolicy(ctx context.Context, input PromptLearningPolicyInput) (*PromptLearningPolicy, error) {
	if DB == nil || !validPromptLearningPolicyInput(input) {
		return nil, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().Unix()
	var result PromptLearningPolicy
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing PromptLearningPolicy
		err := lockForUpdate(tx).Where("user_id = ? AND scope_ref = ?", input.UserID, input.ScopeRef).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			candidate := PromptLearningPolicy{UserID: input.UserID, ScopeRef: input.ScopeRef, Enabled: input.Enabled, Generation: 1, CreatedAt: now, UpdatedAt: now}
			if !input.Enabled {
				candidate.RevokedAt = now
			}
			if createErr := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; createErr != nil {
				return createErr
			}
			if candidate.ID != 0 {
				result = candidate
				return nil
			}
			if err := lockForUpdate(tx).Where("user_id = ? AND scope_ref = ?", input.UserID, input.ScopeRef).First(&existing).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if existing.Enabled != input.Enabled {
			existing.Enabled = input.Enabled
			existing.Generation++
			existing.UpdatedAt = now
			if input.Enabled {
				existing.RevokedAt = 0
			} else {
				existing.RevokedAt = now
				// A policy revoke prevents a pending worker from ever reaching a
				// model call. Submitted work deliberately is not rewritten here:
				// its external outcome may already exist and must remain explicit.
				if err := tx.Model(&PromptLearningRun{}).
					Where("user_id = ? AND scope_ref = ? AND state IN ?", input.UserID, input.ScopeRef, []string{promptLearningRunStatePending, promptLearningRunStateLeased, promptLearningRunStatePreparing}).
					Updates(map[string]any{"state": promptLearningRunStateCancelled, "lease_owner": "", "lease_until": 0, "updated_at": now}).Error; err != nil {
					return err
				}
			}
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
		}
		result = existing
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func GetPromptLearningPolicy(ctx context.Context, userID int, scopeRef string) (*PromptLearningPolicy, error) {
	if DB == nil || userID <= 0 || !validPromptLearningText(scopeRef, promptLearningPolicyScopeMaxBytes) {
		return nil, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var policy PromptLearningPolicy
	err := DB.WithContext(ctx).Where("user_id = ? AND scope_ref = ?", userID, scopeRef).First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPromptLearningPolicyNotFound
	}
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

// RecordPromptLearningSample persists a pre-redacted trusted sample only
// while its exact policy generation remains enabled. Identical occurrences
// replay without another write; a different payload under the same occurrence
// is a conflict and is never silently overwritten.
func RecordPromptLearningSample(ctx context.Context, input PromptLearningSampleInput) (*PromptLearningSample, bool, error) {
	if DB == nil || !validPromptLearningSampleInput(input) {
		return nil, false, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().Unix()
	var result PromptLearningSample
	created := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var policy PromptLearningPolicy
		if err := lockForUpdate(tx).Where("user_id = ? AND scope_ref = ?", input.UserID, input.ScopeRef).First(&policy).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPromptLearningPolicyNotFound
			}
			return err
		}
		if !policy.Enabled {
			return ErrPromptLearningPolicyDisabled
		}
		if policy.Generation != input.PolicyGeneration {
			return ErrPromptLearningPolicyStale
		}

		var existing PromptLearningSample
		err := tx.Where("user_id = ? AND scope_ref = ? AND occurrence_fingerprint = ?", input.UserID, input.ScopeRef, input.OccurrenceFingerprint).First(&existing).Error
		if err == nil {
			if existing.PolicyID == policy.ID && existing.PolicyGeneration == input.PolicyGeneration && existing.SemanticFingerprint == input.SemanticFingerprint && existing.SourceReceipt == input.SourceReceipt && existing.RedactedText == input.RedactedText && existing.Confidence == input.Confidence {
				result = existing
				return nil
			}
			return ErrPromptLearningSampleConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		candidate := PromptLearningSample{
			PolicyID: policy.ID, UserID: input.UserID, ScopeRef: input.ScopeRef,
			OccurrenceFingerprint: input.OccurrenceFingerprint, SemanticFingerprint: input.SemanticFingerprint,
			SourceReceipt: input.SourceReceipt, RedactedText: input.RedactedText, Confidence: input.Confidence,
			PolicyGeneration: input.PolicyGeneration, State: promptLearningSampleStateActive, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&candidate).Error; err != nil {
			return err
		}
		result, created = candidate, true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &result, created, nil
}

func ListPromptLearningSamples(ctx context.Context, userID int, scopeRef string, offset, limit int) ([]PromptLearningSample, int64, error) {
	if DB == nil || userID <= 0 || !validPromptLearningText(scopeRef, promptLearningPolicyScopeMaxBytes) || offset < 0 || limit <= 0 || limit > 200 {
		return nil, 0, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := DB.WithContext(ctx).Model(&PromptLearningSample{}).Where("user_id = ? AND scope_ref = ? AND state = ?", userID, scopeRef, promptLearningSampleStateActive)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]PromptLearningSample, 0)
	if err := query.Order("id ASC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// CreatePromptInstructionVersion appends one immutable manual or generated
// version. Retried commands return their original version only when every
// immutable field agrees; a reuse with different content or parent is a
// conflict and never replaces the previous version.
func CreatePromptInstructionVersion(ctx context.Context, input PromptInstructionVersionInput) (*PromptInstructionVersion, bool, error) {
	if DB == nil || !validPromptInstructionVersionInput(input) {
		return nil, false, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	content := normalizePromptInstructionContent(input.Content)
	contentHash := promptInstructionContentHash(content)
	now := time.Now().Unix()
	var result PromptInstructionVersion
	created := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing PromptInstructionVersion
		err := tx.Where("user_id = ? AND scope_ref = ? AND command_id = ?", input.UserID, input.ScopeRef, input.CommandID).First(&existing).Error
		if err == nil {
			if promptInstructionVersionMatches(existing, input, contentHash) {
				result = existing
				return nil
			}
			return ErrPromptInstructionConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if input.ParentID != nil {
			var parent PromptInstructionVersion
			if err := tx.Where("id = ? AND user_id = ? AND scope_ref = ?", *input.ParentID, input.UserID, input.ScopeRef).First(&parent).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrPromptInstructionNotFound
				}
				return err
			}
		}
		candidate := PromptInstructionVersion{
			UserID: input.UserID, ScopeRef: input.ScopeRef, CommandID: input.CommandID, ParentID: input.ParentID,
			Content: content, ContentHash: contentHash, Source: input.Source, CreatedBy: input.CreatedBy, CreatedAt: now,
		}
		if err := tx.Create(&candidate).Error; err != nil {
			return err
		}
		result, created = candidate, true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &result, created, nil
}

func GetPromptInstructionVersion(ctx context.Context, userID int, scopeRef string, versionID int64) (*PromptInstructionVersion, error) {
	if DB == nil || userID <= 0 || versionID <= 0 || !validPromptLearningText(scopeRef, promptLearningPolicyScopeMaxBytes) {
		return nil, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var version PromptInstructionVersion
	err := DB.WithContext(ctx).Where("id = ? AND user_id = ? AND scope_ref = ?", versionID, userID, scopeRef).First(&version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPromptInstructionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &version, nil
}

func ListPromptInstructionVersions(ctx context.Context, userID int, scopeRef string, offset, limit int) ([]PromptInstructionVersion, int64, error) {
	if DB == nil || userID <= 0 || !validPromptLearningText(scopeRef, promptLearningPolicyScopeMaxBytes) || offset < 0 || limit <= 0 || limit > 200 {
		return nil, 0, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := DB.WithContext(ctx).Model(&PromptInstructionVersion{}).Where("user_id = ? AND scope_ref = ?", userID, scopeRef)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]PromptInstructionVersion, 0)
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// CreatePromptInstructionApplication records a future application request only
// when its policy is still enabled and its exact generation is current. The
// executor must recheck these values before touching any authorized file.
func CreatePromptInstructionApplication(ctx context.Context, input PromptInstructionApplicationInput) (*PromptInstructionApplication, bool, error) {
	if DB == nil || !validPromptInstructionApplicationInput(input) {
		return nil, false, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().Unix()
	if input.AuthorizationExpiresAt <= now {
		return nil, false, ErrPromptLearningInvalidInput
	}
	var result PromptInstructionApplication
	created := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var policy PromptLearningPolicy
		if err := lockForUpdate(tx).Where("user_id = ? AND scope_ref = ?", input.UserID, input.ScopeRef).First(&policy).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPromptLearningPolicyNotFound
			}
			return err
		}
		if !policy.Enabled {
			return ErrPromptLearningPolicyDisabled
		}
		if policy.Generation != input.AuthorizationGeneration {
			return ErrPromptLearningPolicyStale
		}
		var existing PromptInstructionApplication
		err := tx.Where("user_id = ? AND scope_ref = ? AND command_id = ?", input.UserID, input.ScopeRef, input.CommandID).First(&existing).Error
		if err == nil {
			if promptInstructionApplicationMatches(existing, input) {
				result = existing
				return nil
			}
			return ErrPromptInstructionConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var version PromptInstructionVersion
		if err := tx.Where("id = ? AND user_id = ? AND scope_ref = ?", input.VersionID, input.UserID, input.ScopeRef).First(&version).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPromptInstructionNotFound
			}
			return err
		}
		if input.RollbackOfID != nil {
			var rollbackOf PromptInstructionApplication
			if err := tx.Where("id = ? AND user_id = ? AND scope_ref = ?", *input.RollbackOfID, input.UserID, input.ScopeRef).First(&rollbackOf).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrPromptInstructionNotFound
				}
				return err
			}
			if rollbackOf.State != promptInstructionApplicationStateApplied {
				return ErrPromptInstructionApplication
			}
		}
		candidate := PromptInstructionApplication{
			UserID: input.UserID, ScopeRef: input.ScopeRef, CommandID: input.CommandID, VersionID: input.VersionID,
			TargetReceipt: input.TargetReceipt, AuthorizationGeneration: input.AuthorizationGeneration, AuthorizationExpiresAt: input.AuthorizationExpiresAt,
			State: promptInstructionApplicationStatePending, RollbackOfID: input.RollbackOfID, LockVersion: 1, CreatedBy: input.CreatedBy, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&candidate).Error; err != nil {
			return err
		}
		result, created = candidate, true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &result, created, nil
}

// TransitionPromptInstructionApplication advances one recorded application
// using an optimistic lock. Only the future authorized executor can supply a
// backup/content receipt, and terminal outcomes never return to pending.
func TransitionPromptInstructionApplication(ctx context.Context, applicationID, expectedLockVersion int64, targetState, backupReceipt, appliedContentHash string) (bool, error) {
	if DB == nil || applicationID <= 0 || expectedLockVersion <= 0 || !validPromptInstructionApplicationState(targetState) || !validOptionalPromptInstructionFingerprint(backupReceipt) || !validOptionalPromptInstructionFingerprint(appliedContentHash) {
		return false, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().Unix()
	updated := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var application PromptInstructionApplication
		if err := lockForUpdate(tx).Where("id = ?", applicationID).First(&application).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPromptInstructionNotFound
			}
			return err
		}
		if application.LockVersion != expectedLockVersion || !validPromptInstructionApplicationTransition(application.State, targetState) {
			return ErrPromptInstructionApplication
		}
		values := map[string]any{"state": targetState, "backup_receipt": backupReceipt, "applied_content_hash": appliedContentHash, "updated_at": now, "lock_version": expectedLockVersion + 1}
		result := tx.Model(&PromptInstructionApplication{}).Where("id = ? AND lock_version = ?", applicationID, expectedLockVersion).Updates(values)
		if result.Error != nil {
			return result.Error
		}
		updated = result.RowsAffected == 1
		if !updated {
			return ErrPromptInstructionApplication
		}
		return nil
	})
	return updated, err
}

func validPromptLearningPolicyInput(input PromptLearningPolicyInput) bool {
	return input.UserID > 0 && validPromptLearningText(input.ScopeRef, promptLearningPolicyScopeMaxBytes)
}

func validPromptLearningSampleInput(input PromptLearningSampleInput) bool {
	return input.UserID > 0 && input.PolicyGeneration > 0 &&
		validPromptLearningText(input.ScopeRef, promptLearningPolicyScopeMaxBytes) &&
		validPromptLearningFingerprint(input.OccurrenceFingerprint) &&
		validPromptLearningFingerprint(input.SemanticFingerprint) &&
		validPromptLearningText(input.SourceReceipt, promptLearningReceiptMaxBytes) &&
		validPromptLearningText(input.RedactedText, promptLearningTextMaxBytes) &&
		validPromptLearningConfidence(input.Confidence)
}

func validPromptInstructionVersionInput(input PromptInstructionVersionInput) bool {
	return input.UserID > 0 && input.CreatedBy > 0 &&
		validPromptLearningText(input.ScopeRef, promptLearningPolicyScopeMaxBytes) &&
		validPromptLearningText(input.CommandID, promptInstructionCommandMaxBytes) &&
		validPromptInstructionContent(input.Content) &&
		(input.Source == promptInstructionVersionSourceManual || input.Source == promptInstructionVersionSourceGenerated)
}

func validPromptInstructionApplicationInput(input PromptInstructionApplicationInput) bool {
	return input.UserID > 0 && input.CreatedBy > 0 && input.VersionID > 0 && input.AuthorizationGeneration > 0 && input.AuthorizationExpiresAt > 0 &&
		validPromptLearningText(input.ScopeRef, promptLearningPolicyScopeMaxBytes) &&
		validPromptLearningText(input.CommandID, promptInstructionCommandMaxBytes) &&
		validPromptInstructionFingerprint(input.TargetReceipt)
}

func validPromptInstructionContent(value string) bool {
	if len(value) == 0 || len(value) > promptInstructionContentMaxBytes || !utf8.ValidString(value) || strings.TrimSpace(value) == "" {
		return false
	}
	for _, char := range value {
		if (char <= 0x1f && char != '\n' && char != '\r' && char != '\t') || (char >= 0x7f && char <= 0x9f) || unicode.Is(unicode.Cf, char) {
			return false
		}
	}
	return true
}

func normalizePromptInstructionContent(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func promptInstructionContentHash(content string) string {
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}

func promptInstructionVersionMatches(existing PromptInstructionVersion, input PromptInstructionVersionInput, contentHash string) bool {
	if existing.ContentHash != contentHash || existing.Source != input.Source || existing.CreatedBy != input.CreatedBy {
		return false
	}
	if existing.ParentID == nil || input.ParentID == nil {
		return existing.ParentID == nil && input.ParentID == nil
	}
	return *existing.ParentID == *input.ParentID
}

func promptInstructionApplicationMatches(existing PromptInstructionApplication, input PromptInstructionApplicationInput) bool {
	if existing.VersionID != input.VersionID || existing.TargetReceipt != input.TargetReceipt || existing.AuthorizationGeneration != input.AuthorizationGeneration || existing.AuthorizationExpiresAt != input.AuthorizationExpiresAt || existing.CreatedBy != input.CreatedBy {
		return false
	}
	if existing.RollbackOfID == nil || input.RollbackOfID == nil {
		return existing.RollbackOfID == nil && input.RollbackOfID == nil
	}
	return *existing.RollbackOfID == *input.RollbackOfID
}

func validPromptInstructionFingerprint(value string) bool {
	return validPromptLearningFingerprint(value)
}

func validOptionalPromptInstructionFingerprint(value string) bool {
	return value == "" || validPromptInstructionFingerprint(value)
}

func validPromptInstructionApplicationState(value string) bool {
	switch value {
	case promptInstructionApplicationStatePending, promptInstructionApplicationStateApplied, promptInstructionApplicationStateRollbackPending, promptInstructionApplicationStateRolledBack, promptInstructionApplicationStateFailed, promptInstructionApplicationStateOutcomeUnknown:
		return true
	default:
		return false
	}
}

func validPromptInstructionApplicationTransition(current, target string) bool {
	switch current {
	case promptInstructionApplicationStatePending:
		return target == promptInstructionApplicationStateApplied || target == promptInstructionApplicationStateFailed || target == promptInstructionApplicationStateOutcomeUnknown
	case promptInstructionApplicationStateApplied:
		return target == promptInstructionApplicationStateRollbackPending
	case promptInstructionApplicationStateRollbackPending:
		return target == promptInstructionApplicationStateRolledBack || target == promptInstructionApplicationStateFailed || target == promptInstructionApplicationStateOutcomeUnknown
	default:
		return false
	}
}

func validPromptLearningFingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func validPromptLearningConfidence(value string) bool {
	switch value {
	case "trusted", "medium", "low":
		return true
	default:
		return false
	}
}

func validPromptLearningText(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) {
		return false
	}
	for _, char := range value {
		if char <= 0x1f || (char >= 0x7f && char <= 0x9f) || unicode.Is(unicode.Cf, char) {
			return false
		}
	}
	return strings.TrimSpace(value) == value
}
