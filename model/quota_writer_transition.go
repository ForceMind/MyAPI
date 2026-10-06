package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	QuotaWriterTransitionStatusApplying  = "applying"
	QuotaWriterTransitionStatusSucceeded = "succeeded"
	QuotaWriterTransitionStatusFailed    = "failed"

	quotaWriterClusterDrainAckOptionKey = "quota_writer_cluster_drain_ack"
	quotaWriterAckNoteMaxLen            = 512
	quotaWriterTransitionFailureMaxLen  = 2048
)

var (
	// ErrQuotaWriterTransitionConflict means the persisted epoch row no longer
	// matches the caller's expected epoch/lock version. No evidence row is
	// written for conflicts: the winner's transition is the evidence.
	ErrQuotaWriterTransitionConflict = errors.New("quota writer mode transition conflicts with the current epoch state")
	// ErrQuotaWriterTransitionInvalid means the requested mode pair is not an
	// allowed transition (legacy->bridge->authoritative only, never downgraded).
	ErrQuotaWriterTransitionInvalid = errors.New("quota writer mode transition is not allowed")
)

// QuotaWriterTransitionConditionsError carries the audit check names that
// blocked an apply. A failed evidence row is persisted alongside it.
type QuotaWriterTransitionConditionsError struct {
	Missing []string
}

func (e *QuotaWriterTransitionConditionsError) Error() string {
	return "quota writer mode transition preconditions are not met: " + strings.Join(e.Missing, ",")
}

// QuotaWriterModeTransition is the durable evidence of one mode switch
// attempt. Operator-visible text fields are length-bounded; audit snapshots
// are JSON produced by common.Marshal.
type QuotaWriterModeTransition struct {
	ID              int64  `json:"id" gorm:"primaryKey"`
	OperatorUserId  int    `json:"operator_user_id" gorm:"not null;index"`
	FromMode        string `json:"from_mode" gorm:"type:varchar(24);not null"`
	ToMode          string `json:"to_mode" gorm:"type:varchar(24);not null"`
	FromEpoch       int64  `json:"from_epoch" gorm:"type:bigint;not null"`
	ToEpoch         int64  `json:"to_epoch" gorm:"type:bigint;not null"`
	Status          string `json:"status" gorm:"type:varchar(16);not null;index"`
	PreAudit        string `json:"pre_audit" gorm:"type:text;not null"`
	PostAudit       string `json:"post_audit" gorm:"type:text;not null"`
	ClusterDrainAck bool   `json:"cluster_drain_ack" gorm:"not null;default:false"`
	AckNote         string `json:"ack_note" gorm:"type:varchar(512);not null;default:''"`
	FailureReason   string `json:"failure_reason" gorm:"type:varchar(2048);not null;default:''"`
	CreatedAt       int64  `json:"created_at" gorm:"type:bigint;not null"`
	FinishedAt      int64  `json:"finished_at" gorm:"type:bigint;not null"`
}

func (QuotaWriterModeTransition) TableName() string { return "quota_writer_mode_transitions" }

func ListQuotaWriterModeTransitions(db *gorm.DB, startIdx int, pageSize int) ([]QuotaWriterModeTransition, int64, error) {
	if db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	var total int64
	if err := db.Model(&QuotaWriterModeTransition{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []QuotaWriterModeTransition
	if err := db.Order("id DESC").Offset(startIdx).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ---------------------------------------------------------------------------
// Process-local in-flight billing session counter
// ---------------------------------------------------------------------------

// quotaWriterInflightSessions counts BillingSessions this process has created
// and not yet carried to a terminal wrap-up (settle/refund, success or
// failure). It is deliberately process-local; multi-process clusters still
// rely on the cluster drain acknowledgement.
var quotaWriterInflightSessions atomic.Int64

func TrackQuotaWriterInflightStart() {
	quotaWriterInflightSessions.Add(1)
}

func TrackQuotaWriterInflightFinish() {
	if quotaWriterInflightSessions.Add(-1) >= 0 {
		return
	}
	quotaWriterInflightSessions.Add(1)
	common.SysError("quota writer inflight counter underflowed; clamped to zero")
}

func QuotaWriterInflightSessions() int64 {
	return quotaWriterInflightSessions.Load()
}

// ---------------------------------------------------------------------------
// Cluster drain acknowledgement (Option table, epoch-bound)
// ---------------------------------------------------------------------------

type QuotaWriterClusterDrainAck struct {
	OperatorUserId int    `json:"operator_user_id"`
	Epoch          int64  `json:"epoch"`
	Note           string `json:"note"`
	Timestamp      int64  `json:"timestamp"`
}

func normalizeQuotaWriterAckNote(note string) (string, error) {
	note = strings.TrimSpace(note)
	if len(note) == 0 || len(note) > quotaWriterAckNoteMaxLen {
		return "", fmt.Errorf("%w: cluster drain ack note must be 1-%d characters", ErrQuotaWriterTransitionInvalid, quotaWriterAckNoteMaxLen)
	}
	return note, nil
}

// RecordQuotaWriterClusterDrainAck upserts the operator acknowledgement bound
// to the given epoch. A later successful transition advances the epoch, which
// invalidates older acknowledgements naturally.
func RecordQuotaWriterClusterDrainAck(db *gorm.DB, operatorUserId int, epoch int64, note string) (*QuotaWriterClusterDrainAck, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	note, err := normalizeQuotaWriterAckNote(note)
	if err != nil {
		return nil, err
	}
	if epoch <= 0 {
		return nil, fmt.Errorf("%w: ack epoch must be positive", ErrQuotaWriterTransitionInvalid)
	}
	ack := &QuotaWriterClusterDrainAck{OperatorUserId: operatorUserId, Epoch: epoch, Note: note, Timestamp: time.Now().Unix()}
	data, err := common.Marshal(ack)
	if err != nil {
		return nil, err
	}
	option := Option{Key: quotaWriterClusterDrainAckOptionKey, Value: string(data)}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&option).Error; err != nil {
		return nil, err
	}
	return ack, nil
}

func loadQuotaWriterClusterDrainAck(db *gorm.DB) (*QuotaWriterClusterDrainAck, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if !db.Migrator().HasTable(&Option{}) {
		return nil, nil
	}
	var option Option
	result := db.Where("key = ?", quotaWriterClusterDrainAckOptionKey).Limit(1).Find(&option)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 || strings.TrimSpace(option.Value) == "" {
		return nil, nil
	}
	var ack QuotaWriterClusterDrainAck
	if err := common.Unmarshal([]byte(option.Value), &ack); err != nil {
		return nil, fmt.Errorf("quota writer cluster drain ack record is corrupted: %w", err)
	}
	return &ack, nil
}

func quotaWriterClusterDrainAckValid(db *gorm.DB, epoch int64) (bool, error) {
	ack, err := loadQuotaWriterClusterDrainAck(db)
	if err != nil || ack == nil {
		return false, err
	}
	return ack.Epoch == epoch && strings.TrimSpace(ack.Note) != "", nil
}

// ---------------------------------------------------------------------------
// Apply path
// ---------------------------------------------------------------------------

type QuotaWriterModeTransitionInput struct {
	TargetMode          QuotaWriterMode
	ExpectedEpoch       int64
	OperatorUserId      int
	ClusterDrainAckNote string
}

// publishQuotaWriterEpochToRedis mirrors the freshly committed epoch into the
// Redis key that cache scripts fence on (quotaWriterEpochRedisKey, plain
// decimal). Redis-disabled deployments skip the mirror exactly like the audit
// skips the consistency read.
func publishQuotaWriterEpochToRedis(ctx context.Context, epoch int64) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}
	publishCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return common.RDB.Set(publishCtx, quotaWriterEpochRedisKey, strconv.FormatInt(epoch, 10), 0).Err()
}

// ApplyQuotaWriterModeTransition performs the gated mode switch that
// PlanQuotaWriterModeTransition only previews. The epoch row is locked and
// CAS-updated in one transaction together with the applying evidence row;
// Redis publication and the post-audit run after commit and only flip the
// evidence status, never the committed mode.
func ApplyQuotaWriterModeTransition(ctx context.Context, db *gorm.DB, input QuotaWriterModeTransitionInput) (*QuotaWriterModeTransition, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !validQuotaWriterMode(input.TargetMode) {
		return nil, fmt.Errorf("%w: invalid quota writer target mode %q", ErrQuotaWriterTransitionInvalid, input.TargetMode)
	}
	note := strings.TrimSpace(input.ClusterDrainAckNote)
	if len(note) > quotaWriterAckNoteMaxLen {
		return nil, fmt.Errorf("%w: cluster drain ack note must be at most %d characters", ErrQuotaWriterTransitionInvalid, quotaWriterAckNoteMaxLen)
	}
	quotaWriterTransitionApplyMu.Lock()
	defer quotaWriterTransitionApplyMu.Unlock()

	var (
		transition    *QuotaWriterModeTransition
		newEpoch      int64
		conditionsErr *QuotaWriterTransitionConditionsError
		failedAudit   *DurableQuotaWriteAudit
		failedFrom    QuotaWriterEpoch
	)
	txErr := db.Transaction(func(tx *gorm.DB) error {
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		var state QuotaWriterEpoch
		if err := lockForUpdate(tx).Where("id = ?", quotaWriterEpochSingletonID).First(&state).Error; err != nil {
			return fmt.Errorf("%w: %w", ErrQuotaWriterEpochUnavailable, err)
		}
		if state.SchemaVersion != QuotaWriterEpochSchemaVersion || !validQuotaWriterMode(QuotaWriterMode(state.Mode)) || state.Epoch <= 0 || state.LockVersion <= 0 {
			return fmt.Errorf("%w: invalid persisted state", ErrQuotaWriterEpochUnavailable)
		}
		currentMode := QuotaWriterMode(state.Mode)
		if state.Epoch != input.ExpectedEpoch {
			return fmt.Errorf("%w: expected epoch %d, current epoch %d", ErrQuotaWriterTransitionConflict, input.ExpectedEpoch, state.Epoch)
		}
		switch currentMode {
		case QuotaWriterModeLegacy:
			if input.TargetMode != QuotaWriterModeBridge {
				return fmt.Errorf("%w: legacy_must_transition_to_bridge", ErrQuotaWriterTransitionInvalid)
			}
		case QuotaWriterModeBridge:
			if input.TargetMode != QuotaWriterModeAuthoritative {
				return fmt.Errorf("%w: bridge_must_transition_to_authoritative", ErrQuotaWriterTransitionInvalid)
			}
		default:
			return fmt.Errorf("%w: authoritative_mode_is_not_downgradable", ErrQuotaWriterTransitionInvalid)
		}
		if state.Epoch == int64(^uint64(0)>>1) {
			return fmt.Errorf("%w: epoch_exhausted", ErrQuotaWriterTransitionInvalid)
		}

		ackNote := ""
		if currentMode == QuotaWriterModeLegacy {
			ackValid, err := quotaWriterClusterDrainAckValid(tx, state.Epoch)
			if err != nil {
				return err
			}
			if !ackValid {
				if note == "" {
					conditionsErr = &QuotaWriterTransitionConditionsError{Missing: []string{"cluster_drain_ack"}}
					failedFrom = state
					return errQuotaWriterTransitionConditionsFailed
				}
				if _, err := RecordQuotaWriterClusterDrainAck(tx, input.OperatorUserId, state.Epoch, note); err != nil {
					return err
				}
				ackNote = note
			}
		} else if note != "" {
			// bridge -> authoritative also accepts the operator acknowledgement
			// at apply time, bound to the current (bridge) epoch.
			if _, err := RecordQuotaWriterClusterDrainAck(tx, input.OperatorUserId, state.Epoch, note); err != nil {
				return err
			}
			ackNote = note
		}

		audit, auditErr := CanEnableDurableQuotaWrites(ctx, tx)
		if currentMode == QuotaWriterModeBridge {
			if auditErr != nil {
				return auditErr
			}
			if !audit.CanEnable {
				conditionsErr = &QuotaWriterTransitionConditionsError{Missing: append([]string(nil), audit.MissingOrFailedChecks...)}
				failedAudit = &audit
				failedFrom = state
				return errQuotaWriterTransitionConditionsFailed
			}
		}
		preAudit, err := common.Marshal(audit)
		if err != nil {
			return err
		}
		transition = &QuotaWriterModeTransition{
			OperatorUserId: input.OperatorUserId, FromMode: state.Mode, ToMode: string(input.TargetMode),
			FromEpoch: state.Epoch, ToEpoch: state.Epoch + 1, Status: QuotaWriterTransitionStatusApplying,
			PreAudit: string(preAudit), PostAudit: "", ClusterDrainAck: true, AckNote: ackNote,
			CreatedAt: now,
		}
		if err := tx.Create(transition).Error; err != nil {
			return err
		}
		result := tx.Model(&QuotaWriterEpoch{}).
			Where("id = ? AND epoch = ? AND lock_version = ?", quotaWriterEpochSingletonID, state.Epoch, state.LockVersion).
			Updates(map[string]interface{}{
				"mode": string(input.TargetMode), "epoch": state.Epoch + 1,
				"lock_version": state.LockVersion + 1, "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: epoch row compare-and-swap lost", ErrQuotaWriterTransitionConflict)
		}
		newEpoch = state.Epoch + 1
		return nil
	})
	if txErr != nil {
		if errors.Is(txErr, errQuotaWriterTransitionConditionsFailed) && conditionsErr != nil {
			evidence := buildFailedQuotaWriterTransition(db, input, failedFrom, failedAudit, conditionsErr.Missing, note)
			if evidence != nil {
				return evidence, conditionsErr
			}
			return nil, conditionsErr
		}
		return nil, txErr
	}

	return finishQuotaWriterTransition(ctx, db, transition, input.TargetMode, newEpoch)
}

var errQuotaWriterTransitionConditionsFailed = errors.New("quota writer transition conditions failed")

// quotaWriterTransitionApplyMu serializes applies inside this process. Apply
// is a rare administrative operation; the in-process ordering keeps the
// losing caller on a clean epoch-conflict path instead of a database lock
// error (SQLite has no row locks), while the persisted compare-and-swap
// remains the cross-process guard.
var quotaWriterTransitionApplyMu sync.Mutex

// buildFailedQuotaWriterTransition persists the failed evidence row outside
// the rolled-back transition transaction. Evidence persistence is best-effort:
// the operator-facing error already carries the missing checklist.
func buildFailedQuotaWriterTransition(db *gorm.DB, input QuotaWriterModeTransitionInput, from QuotaWriterEpoch, audit *DurableQuotaWriteAudit, missing []string, note string) *QuotaWriterModeTransition {
	now := time.Now().Unix()
	if ts, err := taskRecoveryDBTimestamp(db); err == nil {
		now = ts
	}
	preAudit := "{}"
	if audit != nil {
		if data, err := common.Marshal(audit); err == nil {
			preAudit = string(data)
		}
	}
	reason := strings.Join(missing, ",")
	if len(reason) > quotaWriterTransitionFailureMaxLen {
		reason = reason[:quotaWriterTransitionFailureMaxLen]
	}
	evidence := &QuotaWriterModeTransition{
		OperatorUserId: input.OperatorUserId, FromMode: from.Mode, ToMode: string(input.TargetMode),
		FromEpoch: from.Epoch, ToEpoch: from.Epoch + 1, Status: QuotaWriterTransitionStatusFailed,
		PreAudit: preAudit, PostAudit: "", ClusterDrainAck: false, AckNote: note,
		FailureReason: reason, CreatedAt: now, FinishedAt: now,
	}
	if err := db.Create(evidence).Error; err != nil {
		common.SysError("failed to persist quota writer transition failure evidence: " + err.Error())
		return nil
	}
	return evidence
}

// finishQuotaWriterTransition publishes the new epoch and runs the
// post-transition audit. The post-audit verifies the committed readback and
// the Redis mirror; it deliberately does not re-require audit.CanEnable,
// because a successful transition advances the epoch and thereby invalidates
// the epoch-bound cluster drain acknowledgement by design. Failures only mark
// the evidence row failed; the committed mode is never rolled back
// (authoritative is not downgradable, and a failed legacy->bridge post-audit
// is left for manual recovery).
func finishQuotaWriterTransition(ctx context.Context, db *gorm.DB, transition *QuotaWriterModeTransition, target QuotaWriterMode, newEpoch int64) (*QuotaWriterModeTransition, error) {
	var failureReasons []string
	if err := publishQuotaWriterEpochToRedis(ctx, newEpoch); err != nil {
		failureReasons = append(failureReasons, "redis_epoch_publish")
	}
	postAudit, postAuditErr := CanEnableDurableQuotaWrites(ctx, db)
	if postAuditErr != nil {
		failureReasons = append(failureReasons, "post_audit_error")
	}
	state, stateErr := GetQuotaWriterEpochState(db)
	if stateErr != nil || state.Mode != string(target) || state.Epoch != newEpoch {
		failureReasons = append(failureReasons, "epoch_readback")
	}

	postAuditJSON, err := common.Marshal(postAudit)
	if err != nil {
		postAuditJSON = []byte("{}")
	}
	finishedAt := time.Now().Unix()
	if ts, tsErr := taskRecoveryDBTimestamp(db); tsErr == nil {
		finishedAt = ts
	}
	status := QuotaWriterTransitionStatusSucceeded
	reason := ""
	if len(failureReasons) > 0 {
		status = QuotaWriterTransitionStatusFailed
		reason = strings.Join(failureReasons, ",")
		if len(reason) > quotaWriterTransitionFailureMaxLen {
			reason = reason[:quotaWriterTransitionFailureMaxLen]
		}
	}
	result := db.Model(&QuotaWriterModeTransition{}).Where("id = ? AND status = ?", transition.ID, QuotaWriterTransitionStatusApplying).
		Updates(map[string]interface{}{
			"status": status, "post_audit": string(postAuditJSON), "failure_reason": reason, "finished_at": finishedAt,
		})
	if result.Error != nil {
		common.SysError("failed to finalize quota writer transition evidence: " + result.Error.Error())
		return transition, result.Error
	}
	transition.Status = status
	transition.PostAudit = string(postAuditJSON)
	transition.FailureReason = reason
	transition.FinishedAt = finishedAt
	if status == QuotaWriterTransitionStatusFailed {
		return transition, fmt.Errorf("quota writer post-transition audit failed: %s", reason)
	}
	return transition, nil
}

// ---------------------------------------------------------------------------
// Drain driver (recovery operation surface)
// ---------------------------------------------------------------------------

type QuotaWriterDrainReport struct {
	Rounds              int   `json:"rounds"`
	BatchQueueRemaining int   `json:"batch_queue_remaining"`
	BalanceDrainPending int64 `json:"balance_drain_pending"`
	BalanceDrainInflight bool `json:"balance_drain_inflight"`
	ProjectionPending   int64 `json:"projection_pending"`
	Complete            bool  `json:"complete"`
}

func quotaWriterBatchQueueRemaining() int {
	remaining := 0
	for _, i := range []int{BatchUpdateTypeUserQuota, BatchUpdateTypeTokenQuota} {
		batchUpdateLocks[i].Lock()
		remaining += len(batchUpdateStores[i])
		batchUpdateLocks[i].Unlock()
	}
	return remaining
}

func (r *QuotaWriterDrainReport) refresh(db *gorm.DB) error {
	r.BatchQueueRemaining = quotaWriterBatchQueueRemaining()
	pending, inflight, err := quotaBalanceBatchDrainAudit(db)
	if err != nil {
		return err
	}
	r.BalanceDrainPending = pending
	r.BalanceDrainInflight = inflight
	if err := db.Model(&QuotaProjectionObligation{}).Where("state <> ?", string(QuotaProjectionObligationStateApplied)).Count(&r.ProjectionPending).Error; err != nil {
		return err
	}
	r.Complete = r.BatchQueueRemaining == 0 && r.BalanceDrainPending == 0 && !r.BalanceDrainInflight && r.ProjectionPending == 0
	return nil
}

// DriveQuotaWriterDrains synchronously drives the existing drain workers to
// zero within a bounded number of rounds: each round runs the existing batch
// update flush (which persists and applies balance drain generations) and one
// bounded projection obligation pass. It adds no new daemon and copies no
// business logic; the projection pass honors the existing recovery gate.
// Balance generation application is legacy-only by design (see
// applyQuotaBalanceBatchGenerationWithDB), so pre-draining before a
// legacy->bridge apply is the supported recovery order.
func DriveQuotaWriterDrains(ctx context.Context, db *gorm.DB, budget int) (*QuotaWriterDrainReport, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if budget <= 0 {
		budget = 1
	}
	if budget > 100 {
		budget = 100
	}
	report := &QuotaWriterDrainReport{}
	for round := 0; round < budget; round++ {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		report.Rounds++
		batchUpdate()
		if _, err := RunQuotaProjectionObligations(ctx, db, "quota-writer-drain", 25); err != nil {
			common.SysError("quota writer drain projection pass: " + err.Error())
		}
		if err := report.refresh(db); err != nil {
			return report, err
		}
		if report.Complete {
			return report, nil
		}
	}
	return report, nil
}
