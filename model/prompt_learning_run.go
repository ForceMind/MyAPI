package model

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	promptLearningRunCommandMaxBytes       = 128
	promptLearningRunModelMaxBytes         = 256
	promptLearningRunTemplateMaxBytes      = 128
	promptLearningRunRunnerMaxBytes        = 128
	promptLearningRunMaxSamples            = 10_000
	promptLearningRunMaxTokens             = 1_000_000
	promptLearningRunMaxAttempts           = 3
	promptLearningRunMaximumLeaseSecs      = 60 * 60
	promptLearningRunStatePending          = "pending"
	promptLearningRunStateLeased           = "leased"
	promptLearningRunStatePreparing        = "preparing"
	promptLearningRunStateSubmitted        = "submitted"
	promptLearningRunStateSucceeded        = "succeeded"
	promptLearningRunStateSkipped          = "skipped_no_new_data"
	promptLearningRunStateCancelled        = "cancelled"
	promptLearningRunStateFailed           = "failed"
	promptLearningRunStateSubmitUnknown    = "submission_unknown"
	promptLearningRunStateOutcomeUnknown   = "outcome_unknown"
	promptLearningRunStateManuallyResolved = "manually_resolved"
)

var ErrPromptLearningRun = errors.New("invalid prompt learning run transition")

// PromptLearningRun is the durable, scope-isolated unit a future worker will
// use for one analysis request. It freezes only bounded metadata: sample
// bounds/count, a baseline version, selected model/template and limits. Raw
// prompts, upstream request/response bodies, credentials and price data do
// not belong in this table.
type PromptLearningRun struct {
	ID                int64  `gorm:"primaryKey"`
	UserID            int    `gorm:"not null;uniqueIndex:idx_prompt_learning_run_command,priority:1;index"`
	ScopeRef          string `gorm:"type:varchar(128);not null;uniqueIndex:idx_prompt_learning_run_command,priority:2;index"`
	CommandID         string `gorm:"type:varchar(128);not null;uniqueIndex:idx_prompt_learning_run_command,priority:3"`
	PolicyGeneration  int64  `gorm:"type:bigint;not null;index"`
	BaselineVersionID *int64 `gorm:"type:bigint;index"`
	SampleUpperID     int64  `gorm:"type:bigint;not null"`
	SampleCount       int    `gorm:"not null"`
	ModelRef          string `gorm:"type:varchar(256);not null"`
	TemplateVersion   string `gorm:"type:varchar(128);not null"`
	InputTokenLimit   int    `gorm:"not null"`
	OutputTokenLimit  int    `gorm:"not null"`
	MaximumAttempts   int    `gorm:"not null"`
	State             string `gorm:"type:varchar(24);not null;index"`
	LeaseOwner        string `gorm:"type:varchar(128);not null;default:''"`
	LeaseUntil        int64  `gorm:"type:bigint;not null;index"`
	LeaseFence        int64  `gorm:"type:bigint;not null"`
	CreatedAt         int64  `gorm:"type:bigint;not null;index"`
	UpdatedAt         int64  `gorm:"type:bigint;not null"`
}

type PromptLearningRunInput struct {
	UserID            int
	ScopeRef          string
	CommandID         string
	PolicyGeneration  int64
	BaselineVersionID *int64
	SampleUpperID     int64
	SampleCount       int
	ModelRef          string
	TemplateVersion   string
	InputTokenLimit   int
	OutputTokenLimit  int
	MaximumAttempts   int
}

func (PromptLearningRun) TableName() string { return "prompt_learning_runs" }

// CreatePromptLearningRun records one idempotent, not-yet-executed analysis
// run. It verifies the policy generation and baseline scope in the same
// transaction, so a policy revoke or cross-scope version can never be queued
// as a valid future model request.
func CreatePromptLearningRun(ctx context.Context, input PromptLearningRunInput) (*PromptLearningRun, bool, error) {
	if DB == nil || !validPromptLearningRunInput(input) {
		return nil, false, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().Unix()
	var result PromptLearningRun
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
		var existing PromptLearningRun
		err := tx.Where("user_id = ? AND scope_ref = ? AND command_id = ?", input.UserID, input.ScopeRef, input.CommandID).First(&existing).Error
		if err == nil {
			if promptLearningRunMatches(existing, input) {
				result = existing
				return nil
			}
			return ErrPromptInstructionConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if input.BaselineVersionID != nil {
			var version PromptInstructionVersion
			if err := tx.Where("id = ? AND user_id = ? AND scope_ref = ?", *input.BaselineVersionID, input.UserID, input.ScopeRef).First(&version).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrPromptInstructionNotFound
				}
				return err
			}
		}
		candidate := PromptLearningRun{
			UserID: input.UserID, ScopeRef: input.ScopeRef, CommandID: input.CommandID, PolicyGeneration: input.PolicyGeneration,
			BaselineVersionID: input.BaselineVersionID, SampleUpperID: input.SampleUpperID, SampleCount: input.SampleCount,
			ModelRef: input.ModelRef, TemplateVersion: input.TemplateVersion, InputTokenLimit: input.InputTokenLimit,
			OutputTokenLimit: input.OutputTokenLimit, MaximumAttempts: input.MaximumAttempts,
			State: promptLearningRunStatePending, LeaseFence: 1, CreatedAt: now, UpdatedAt: now,
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

// ListPromptLearningRuns returns only one user's own opaque scope. The view
// layer decides which safe fields to disclose; command IDs and lease owners
// remain internal operational data.
func ListPromptLearningRuns(ctx context.Context, userID int, scopeRef string, offset, limit int) ([]PromptLearningRun, int64, error) {
	if DB == nil || userID <= 0 || !validPromptLearningText(scopeRef, promptLearningPolicyScopeMaxBytes) || offset < 0 || limit <= 0 || limit > 200 {
		return nil, 0, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := DB.WithContext(ctx).Model(&PromptLearningRun{}).Where("user_id = ? AND scope_ref = ?", userID, scopeRef)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]PromptLearningRun, 0)
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// CancelPromptLearningRun cancels only work that has not crossed the external
// submission boundary. A submitted/unknown run is intentionally left intact:
// an upstream request may already have consumed quota and its result must be
// reconciled rather than rewritten as cancelled.
func CancelPromptLearningRun(ctx context.Context, userID int, scopeRef string, runID int64) (bool, error) {
	if DB == nil || userID <= 0 || runID <= 0 || !validPromptLearningText(scopeRef, promptLearningPolicyScopeMaxBytes) {
		return false, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().Unix()
	cancelled := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run PromptLearningRun
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ? AND scope_ref = ?", runID, userID, scopeRef).First(&run).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPromptInstructionNotFound
			}
			return err
		}
		switch run.State {
		case promptLearningRunStatePending, promptLearningRunStateLeased, promptLearningRunStatePreparing:
		default:
			return nil
		}
		update := tx.Model(&PromptLearningRun{}).Where("id = ? AND state = ? AND lease_fence = ?", run.ID, run.State, run.LeaseFence).Updates(map[string]any{
			"state": promptLearningRunStateCancelled, "lease_owner": "", "lease_until": 0, "updated_at": now,
		})
		if update.Error != nil {
			return update.Error
		}
		cancelled = update.RowsAffected == 1
		if !cancelled {
			return ErrPromptLearningRun
		}
		return nil
	})
	return cancelled, err
}

// ClaimPromptLearningRun leases only a pending run. The worker must present
// the returned fence for every later transition. Expired pre-submission work
// is separately recovered to pending; submitted work is never retried here.
func ClaimPromptLearningRun(ctx context.Context, userID int, scopeRef string, runID int64, owner string, leaseUntil int64) (*PromptLearningRun, bool, error) {
	if DB == nil || userID <= 0 || runID <= 0 || !validPromptLearningText(scopeRef, promptLearningPolicyScopeMaxBytes) || !validPromptLearningText(owner, promptLearningRunRunnerMaxBytes) {
		return nil, false, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().Unix()
	if leaseUntil <= now || leaseUntil-now > promptLearningRunMaximumLeaseSecs {
		return nil, false, ErrPromptLearningInvalidInput
	}
	var result PromptLearningRun
	claimed := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run PromptLearningRun
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ? AND scope_ref = ?", runID, userID, scopeRef).First(&run).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPromptInstructionNotFound
			}
			return err
		}
		if run.State != promptLearningRunStatePending {
			return nil
		}
		var policy PromptLearningPolicy
		if err := lockForUpdate(tx).Where("user_id = ? AND scope_ref = ?", userID, scopeRef).First(&policy).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPromptLearningPolicyNotFound
			}
			return err
		}
		if !policy.Enabled {
			return ErrPromptLearningPolicyDisabled
		}
		if policy.Generation != run.PolicyGeneration {
			return ErrPromptLearningPolicyStale
		}
		fence := run.LeaseFence + 1
		update := tx.Model(&PromptLearningRun{}).Where("id = ? AND state = ? AND lease_fence = ?", run.ID, promptLearningRunStatePending, run.LeaseFence).Updates(map[string]any{
			"state": promptLearningRunStateLeased, "lease_owner": owner, "lease_until": leaseUntil, "lease_fence": fence, "updated_at": now,
		})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrPromptLearningRun
		}
		run.State, run.LeaseOwner, run.LeaseUntil, run.LeaseFence, run.UpdatedAt = promptLearningRunStateLeased, owner, leaseUntil, fence, now
		result, claimed = run, true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if !claimed {
		return nil, false, nil
	}
	return &result, true, nil
}

// TransitionPromptLearningRun advances a fenced lease through the analysis
// state machine. It intentionally has no model or billing integration: a
// future executor may only call it around an already-authorized relay request.
func TransitionPromptLearningRun(ctx context.Context, userID int, scopeRef string, runID, expectedFence int64, targetState string) (bool, error) {
	if DB == nil || userID <= 0 || runID <= 0 || expectedFence <= 0 || !validPromptLearningText(scopeRef, promptLearningPolicyScopeMaxBytes) || !validPromptLearningRunState(targetState) {
		return false, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().Unix()
	updated := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run PromptLearningRun
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ? AND scope_ref = ?", runID, userID, scopeRef).First(&run).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPromptInstructionNotFound
			}
			return err
		}
		if run.LeaseFence != expectedFence || !validPromptLearningRunTransition(run.State, targetState) {
			return ErrPromptLearningRun
		}
		if targetState == promptLearningRunStatePreparing || targetState == promptLearningRunStateSubmitted {
			var policy PromptLearningPolicy
			if err := lockForUpdate(tx).Where("user_id = ? AND scope_ref = ?", userID, scopeRef).First(&policy).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrPromptLearningPolicyNotFound
				}
				return err
			}
			if !policy.Enabled {
				return ErrPromptLearningPolicyDisabled
			}
			if policy.Generation != run.PolicyGeneration {
				return ErrPromptLearningPolicyStale
			}
		}
		values := map[string]any{"state": targetState, "updated_at": now}
		if promptLearningRunTerminal(targetState) {
			values["lease_owner"] = ""
			values["lease_until"] = 0
		}
		update := tx.Model(&PromptLearningRun{}).Where("id = ? AND lease_fence = ? AND state = ?", runID, expectedFence, run.State).Updates(values)
		if update.Error != nil {
			return update.Error
		}
		updated = update.RowsAffected == 1
		if !updated {
			return ErrPromptLearningRun
		}
		return nil
	})
	return updated, err
}

// RecoverExpiredPromptLearningRuns only requeues work before submission. The
// absence of an upstream receipt in leased/preparing states makes it safe to
// retry; submitted work remains an explicit unknown/outcome workflow.
func RecoverExpiredPromptLearningRuns(ctx context.Context, now int64, limit int) (int64, error) {
	if DB == nil || now <= 0 || limit <= 0 || limit > 200 {
		return 0, ErrPromptLearningInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var runs []PromptLearningRun
	if err := DB.WithContext(ctx).Where("state IN ? AND lease_until > 0 AND lease_until < ?", []string{promptLearningRunStateLeased, promptLearningRunStatePreparing}, now).Order("id ASC").Limit(limit).Find(&runs).Error; err != nil {
		return 0, err
	}
	var recovered int64
	for _, run := range runs {
		update := DB.WithContext(ctx).Model(&PromptLearningRun{}).Where("id = ? AND state = ? AND lease_fence = ? AND lease_until < ?", run.ID, run.State, run.LeaseFence, now).Updates(map[string]any{
			"state": promptLearningRunStatePending, "lease_owner": "", "lease_until": 0, "updated_at": now,
		})
		if update.Error != nil {
			return recovered, update.Error
		}
		recovered += update.RowsAffected
	}
	return recovered, nil
}

func validPromptLearningRunInput(input PromptLearningRunInput) bool {
	return input.UserID > 0 && input.PolicyGeneration > 0 && input.SampleUpperID >= 0 && input.SampleCount >= 0 && input.SampleCount <= promptLearningRunMaxSamples &&
		input.InputTokenLimit > 0 && input.InputTokenLimit <= promptLearningRunMaxTokens && input.OutputTokenLimit > 0 && input.OutputTokenLimit <= promptLearningRunMaxTokens &&
		input.MaximumAttempts > 0 && input.MaximumAttempts <= promptLearningRunMaxAttempts &&
		validPromptLearningText(input.ScopeRef, promptLearningPolicyScopeMaxBytes) && validPromptLearningText(input.CommandID, promptLearningRunCommandMaxBytes) &&
		validPromptLearningText(input.ModelRef, promptLearningRunModelMaxBytes) && validPromptLearningText(input.TemplateVersion, promptLearningRunTemplateMaxBytes)
}

func promptLearningRunMatches(existing PromptLearningRun, input PromptLearningRunInput) bool {
	if existing.PolicyGeneration != input.PolicyGeneration || existing.SampleUpperID != input.SampleUpperID || existing.SampleCount != input.SampleCount || existing.ModelRef != input.ModelRef || existing.TemplateVersion != input.TemplateVersion || existing.InputTokenLimit != input.InputTokenLimit || existing.OutputTokenLimit != input.OutputTokenLimit || existing.MaximumAttempts != input.MaximumAttempts {
		return false
	}
	if existing.BaselineVersionID == nil || input.BaselineVersionID == nil {
		return existing.BaselineVersionID == nil && input.BaselineVersionID == nil
	}
	return *existing.BaselineVersionID == *input.BaselineVersionID
}

func validPromptLearningRunState(value string) bool {
	switch value {
	case promptLearningRunStatePending, promptLearningRunStateLeased, promptLearningRunStatePreparing, promptLearningRunStateSubmitted, promptLearningRunStateSucceeded, promptLearningRunStateSkipped, promptLearningRunStateCancelled, promptLearningRunStateFailed, promptLearningRunStateSubmitUnknown, promptLearningRunStateOutcomeUnknown, promptLearningRunStateManuallyResolved:
		return true
	default:
		return false
	}
}

func promptLearningRunTerminal(state string) bool {
	switch state {
	case promptLearningRunStateSucceeded, promptLearningRunStateSkipped, promptLearningRunStateCancelled, promptLearningRunStateFailed, promptLearningRunStateOutcomeUnknown, promptLearningRunStateManuallyResolved:
		return true
	default:
		return false
	}
}

func validPromptLearningRunTransition(current, target string) bool {
	switch current {
	case promptLearningRunStatePending:
		return target == promptLearningRunStateLeased || target == promptLearningRunStateCancelled
	case promptLearningRunStateLeased:
		return target == promptLearningRunStatePreparing || target == promptLearningRunStateCancelled
	case promptLearningRunStatePreparing:
		return target == promptLearningRunStateSubmitted || target == promptLearningRunStateSkipped || target == promptLearningRunStateCancelled || target == promptLearningRunStateFailed
	case promptLearningRunStateSubmitted:
		return target == promptLearningRunStateSucceeded || target == promptLearningRunStateSubmitUnknown
	case promptLearningRunStateSubmitUnknown:
		return target == promptLearningRunStateOutcomeUnknown || target == promptLearningRunStateManuallyResolved
	default:
		return false
	}
}
