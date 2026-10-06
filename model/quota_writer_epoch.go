package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	QuotaWriterEpochSchemaVersion = 1
	quotaWriterEpochSingletonID   = 1
	quotaWriterEpochRedisKey      = "quota:writer:epoch"
)

type QuotaWriterMode string

const (
	QuotaWriterModeLegacy        QuotaWriterMode = "legacy"
	QuotaWriterModeBridge        QuotaWriterMode = "bridge"
	QuotaWriterModeAuthoritative QuotaWriterMode = "authoritative"
)

var (
	ErrQuotaWriterEpochUnavailable    = errors.New("quota writer epoch is unavailable")
	ErrQuotaWriterEpochMismatch       = errors.New("quota writer epoch mismatch")
	ErrDurableQuotaWriterModeDisabled = errors.New("durable quota writes require authoritative writer mode")
	ErrQuotaProjectionModeDisabled    = errors.New("quota projection requires bridge or authoritative writer mode")
	ErrLegacyQuotaWriterModeDisabled  = errors.New("legacy quota writer requires legacy writer mode")
	ErrBatchQuotaCacheUnavailable     = errors.New("batch quota cache is unavailable")
)

// QuotaWriterEpoch is the singleton, main-database authority for quota writer
// mode and epoch. Defaults are applied by application code, not schema tags, so
// AutoMigrate remains stable across SQLite, MySQL 5.7, and PostgreSQL 9.6.
type QuotaWriterEpoch struct {
	ID            int    `json:"id" gorm:"primaryKey"`
	SchemaVersion int    `json:"schema_version" gorm:"not null"`
	Mode          string `json:"mode" gorm:"type:varchar(24);not null"`
	Epoch         int64  `json:"epoch" gorm:"type:bigint;not null"`
	LockVersion   int64  `json:"lock_version" gorm:"type:bigint;not null"`
	UpdatedAt     int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (QuotaWriterEpoch) TableName() string { return "quota_writer_epochs" }

func validQuotaWriterMode(mode QuotaWriterMode) bool {
	return mode == QuotaWriterModeLegacy || mode == QuotaWriterModeBridge || mode == QuotaWriterModeAuthoritative
}

func newLegacyQuotaWriterEpoch(updatedAt int64) QuotaWriterEpoch {
	return QuotaWriterEpoch{
		ID: quotaWriterEpochSingletonID, SchemaVersion: QuotaWriterEpochSchemaVersion,
		Mode: string(QuotaWriterModeLegacy), Epoch: 1, LockVersion: 1, UpdatedAt: updatedAt,
	}
}

// EnsureQuotaWriterEpochStateWithDB creates the default legacy singleton after
// schema migration. It never advances the epoch or changes mode.
func EnsureQuotaWriterEpochStateWithDB(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	publishExpectation, _ := captureUserQuotaBusinessPublishExpectation(db)
	now, err := taskRecoveryDBTimestamp(db)
	if err != nil {
		return err
	}
	candidate := newLegacyQuotaWriterEpoch(now)
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
		return err
	}
	if _, err = GetQuotaWriterEpochState(db); err != nil {
		return err
	}
	if err := EnsureQuotaMaintenanceBackfillCursorsWithDB(db); err != nil {
		return err
	}
	if publishExpectation != nil {
		if err := publishUserQuotaBusinessSchemaReady(publishExpectation); err != nil {
			return err
		}
	}
	return nil
}

func GetQuotaWriterEpochState(db *gorm.DB) (*QuotaWriterEpoch, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var state QuotaWriterEpoch
	if err := db.Where("id = ?", quotaWriterEpochSingletonID).First(&state).Error; err != nil {
		return nil, fmt.Errorf("%w: %w", ErrQuotaWriterEpochUnavailable, err)
	}
	if state.SchemaVersion != QuotaWriterEpochSchemaVersion || !validQuotaWriterMode(QuotaWriterMode(state.Mode)) || state.Epoch <= 0 || state.LockVersion <= 0 {
		return nil, fmt.Errorf("%w: invalid persisted state", ErrQuotaWriterEpochUnavailable)
	}
	return &state, nil
}

func quotaWriterEpochForLegacyCache(db *gorm.DB) (int64, error) {
	state, err := GetQuotaWriterEpochState(db)
	if err != nil {
		return 0, err
	}
	if QuotaWriterMode(state.Mode) != QuotaWriterModeLegacy {
		return 0, ErrLegacyQuotaWriterModeDisabled
	}
	return state.Epoch, nil
}

func requireDurableQuotaWriterEpoch(db *gorm.DB) (*QuotaWriterEpoch, error) {
	state, err := GetQuotaWriterEpochState(db)
	if err != nil {
		return nil, err
	}
	if QuotaWriterMode(state.Mode) != QuotaWriterModeAuthoritative {
		return nil, ErrDurableQuotaWriterModeDisabled
	}
	ctx := context.Background()
	if db.Statement != nil && db.Statement.Context != nil {
		ctx = db.Statement.Context
	}
	complete, err := QuotaMaintenanceBackfillsComplete(ctx, db)
	if err != nil {
		return nil, err
	}
	if !complete {
		return nil, ErrQuotaMaintenanceBackfillIncomplete
	}
	return state, nil
}

func requireQuotaProjectionEpoch(db *gorm.DB, expected int64) (*QuotaWriterEpoch, error) {
	state, err := GetQuotaWriterEpochState(db)
	if err != nil {
		return nil, err
	}
	mode := QuotaWriterMode(state.Mode)
	if mode != QuotaWriterModeBridge && mode != QuotaWriterModeAuthoritative {
		return nil, ErrQuotaProjectionModeDisabled
	}
	if expected <= 0 || state.Epoch != expected {
		return nil, ErrQuotaWriterEpochMismatch
	}
	return state, nil
}

func setQuotaProjectionRecoveryGateForTest(enabled bool) func() {
	previous := common.TaskRecoveryObligationRecoveryEnabled
	common.TaskRecoveryObligationRecoveryEnabled = enabled
	return func() { common.TaskRecoveryObligationRecoveryEnabled = previous }
}

func QuotaProjectionObligationRecoveryEnabled() bool {
	return common.TaskRecoveryObligationRecoveryEnabled
}

type QuotaWriterRegistration struct {
	Name     string `json:"name"`
	Migrated bool   `json:"migrated"`
}

// ProductionQuotaWriterRegistrations is deliberately explicit. WP3-A migrated
// durable task receipts and MutateUserQuota. WP3-B1 routes every production
// BillingSession lifecycle through authoritative account receipts and durable
// terminal facts. WP3-B2 wired the business credit paths (topup / redemption /
// checkin / invite / aff) into the authoritative receipt kernel. WP3-B3 migrated
// the subscription wallet purchase debit and the admin quota endpoints.
//
// The remaining legacy generic writers keep Migrated=true only where every
// production caller was verified per-caller to be either mode-dispatched or
// reachable solely on legacy-only paths with the model-layer
// requireLegacyQuotaWriterCall guard closing non-legacy modes:
//   - increase_user_quota / decrease_user_quota: callers are credit_edge
//     (mode-dispatched), postConsumeQuotaLegacy (mode-dispatched),
//     WalletFunding settle/refund and BillingSession reserve/rollback
//     (legacy-only reachable), taskAdjustFunding (legacy settlement-fact
//     gated), RefundMidjourneyQuota (mode-dispatched); the former admin
//     endpoint callers were migrated in WP3-B3.
//   - try_reserve_user_quota: only WalletFunding.PreConsume (legacy-only
//     reachable; guarded).
//   - increase_token_quota / decrease_token_quota: callers are
//     postConsumeQuotaLegacy (mode-dispatched), taskAdjustTokenQuota (legacy
//     settlement-fact gated), refundMidjourneyTokenQuota (mode-dispatched).
//   - try_reserve_token_quota: only PreConsumeTokenQuota (legacy-only
//     reachable; guarded).
//   - subscription_wallet_overflow: the balance-paid purchase debit is
//     mode-dispatched in PurchaseSubscriptionWithBalance (receipt key
//     "subscription-wallet:{tradeNo}", bridge fail-closed); overflow wallet
//     writes are reachable only via the legacy BillingSession/task legacy
//     facts (guarded) or the authoritative Reserve kernels; subscription-side
//     writes keep their existing CAS + upstream idempotency
//     (SubscriptionPreConsumeRecord / settlement facts).
//   - admin_quota_mutations: all three operations share the durable
//     admin_quota_adjustments identity with mode dispatch (WP3-B3).
//
// DeltaUpdateUserQuota is intentionally absent: repository-wide caller
// analysis found only the helper definition, which delegates to the registered
// increase/decrease writers. This table inventories production write paths
// that must be migrated, not unused convenience helpers.
func ProductionQuotaWriterRegistrations() []QuotaWriterRegistration {
	return []QuotaWriterRegistration{
		{Name: "durable_task_receipts", Migrated: true},
		{Name: "mutate_user_quota", Migrated: true},
		{Name: "increase_user_quota", Migrated: true},
		{Name: "decrease_user_quota", Migrated: true},
		{Name: "try_reserve_user_quota", Migrated: true},
		{Name: "increase_token_quota", Migrated: true},
		{Name: "decrease_token_quota", Migrated: true},
		{Name: "try_reserve_token_quota", Migrated: true},
		{Name: "credit_recharge_redemption_checkin", Migrated: true},
		{Name: "subscription_wallet_overflow", Migrated: true},
		{Name: "billing_session_callers", Migrated: true},
		{Name: "admin_quota_mutations", Migrated: true},
	}
}

type DurableQuotaWriteAudit struct {
	CanEnable                bool                      `json:"can_enable"`
	State                    *QuotaWriterEpoch         `json:"state,omitempty"`
	Writers                  []QuotaWriterRegistration `json:"writers"`
	AllWritersMigrated       bool                      `json:"all_writers_migrated"`
	BatchQueueEmpty          bool                      `json:"batch_queue_empty"`
	ProjectionPending        int64                     `json:"projection_pending"`
	BalanceDrainPending      int64                     `json:"balance_drain_pending"`
	BalanceDrainInflightZero bool                      `json:"balance_drain_inflight_zero"`
	MaintenanceBackfillDone  bool                      `json:"maintenance_backfill_done"`
	RedisEpochConsistent     bool                      `json:"redis_epoch_consistent"`
	ClusterDrainAck          bool                      `json:"cluster_drain_ack"`
	InflightSessions         int64                     `json:"inflight_sessions"`
	InflightZero             bool                      `json:"inflight_zero"`
	MissingOrFailedChecks    []string                  `json:"missing_or_failed_checks,omitempty"`
}

// quotaWriterRegistrationsForAudit is the audit's view of the writer
// registration table. Tests override it to exercise the gated apply path; the
// production table itself is unchanged.
var quotaWriterRegistrationsForAudit = ProductionQuotaWriterRegistrations

func quotaBatchQueueEmpty() bool {
	for _, i := range []int{BatchUpdateTypeUserQuota, BatchUpdateTypeTokenQuota} {
		batchUpdateLocks[i].Lock()
		empty := len(batchUpdateStores[i]) == 0
		batchUpdateLocks[i].Unlock()
		if !empty {
			return false
		}
	}
	return true
}

func redisQuotaEpochConsistent(ctx context.Context, expected int64) (bool, error) {
	value, err := common.RDB.Get(ctx, quotaWriterEpochRedisKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, err
	}
	redisEpoch, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || redisEpoch != expected {
		return false, nil
	}
	for _, pattern := range []string{"user:*", "token:*"} {
		var cursor uint64
		for {
			keys, next, scanErr := common.RDB.Scan(ctx, cursor, pattern, 200).Result()
			if scanErr != nil {
				return false, scanErr
			}
			for _, key := range keys {
				keyType, typeErr := common.RDB.Type(ctx, key).Result()
				if typeErr != nil {
					return false, typeErr
				}
				if keyType != "hash" {
					continue
				}
				value, getErr := common.RDB.HGet(ctx, key, "QuotaWriterEpoch").Result()
				if getErr != nil {
					return false, nil
				}
				cacheEpoch, parseErr := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
				if parseErr != nil || cacheEpoch != expected {
					return false, nil
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	return true, nil
}

// CanEnableDurableQuotaWrites is a fail-closed audit only. It never changes a
// gate, mode, epoch, cache, queue, or obligation.
func CanEnableDurableQuotaWrites(ctx context.Context, db *gorm.DB) (DurableQuotaWriteAudit, error) {
	audit := DurableQuotaWriteAudit{Writers: quotaWriterRegistrationsForAudit()}
	state, err := GetQuotaWriterEpochState(db)
	if err != nil {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "writer_epoch_state")
		return audit, err
	}
	audit.State = state
	audit.AllWritersMigrated = true
	for _, writer := range audit.Writers {
		if !writer.Migrated {
			audit.AllWritersMigrated = false
			break
		}
	}
	if !audit.AllWritersMigrated {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "production_writer_registration")
	}
	audit.BatchQueueEmpty = quotaBatchQueueEmpty()
	if !audit.BatchQueueEmpty {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "batch_queue_empty")
	}
	balanceDrainPending, balanceInflight, drainErr := quotaBalanceBatchDrainAudit(db)
	if drainErr != nil {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "balance_drain_state")
		return audit, drainErr
	}
	audit.BalanceDrainPending = balanceDrainPending
	audit.BalanceDrainInflightZero = !balanceInflight && balanceDrainPending == 0
	if balanceDrainPending != 0 {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "balance_drain_pending")
	}
	if !audit.BalanceDrainInflightZero {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "balance_drain_inflight_zero")
	}
	backfillComplete, backfillErr := QuotaMaintenanceBackfillsComplete(ctx, db)
	if backfillErr != nil {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "maintenance_backfill")
		return audit, backfillErr
	}
	audit.MaintenanceBackfillDone = backfillComplete
	if !backfillComplete {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "maintenance_backfill")
	}
	if err := db.Model(&QuotaProjectionObligation{}).Where("state <> ?", string(QuotaProjectionObligationStateApplied)).Count(&audit.ProjectionPending).Error; err != nil {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "projection_pending")
		return audit, err
	}
	if audit.ProjectionPending != 0 {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "projection_pending")
	}
	if !common.RedisEnabled || common.RDB == nil {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "redis_epoch")
	} else {
		audit.RedisEpochConsistent, err = redisQuotaEpochConsistent(ctx, state.Epoch)
		if err != nil {
			return audit, err
		}
		if !audit.RedisEpochConsistent {
			audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "redis_epoch")
		}
	}
	// A distributed transition requires an explicit operator acknowledgement,
	// persisted in the Option table and bound to the current epoch. The
	// process-local in-flight session counter completes the picture for this
	// process; multi-process fleets are covered by the acknowledgement.
	ackValid, ackErr := quotaWriterClusterDrainAckValid(db, state.Epoch)
	if ackErr != nil {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "cluster_drain_ack")
		return audit, ackErr
	}
	audit.ClusterDrainAck = ackValid
	if !ackValid {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "cluster_drain_ack")
	}
	audit.InflightSessions = QuotaWriterInflightSessions()
	audit.InflightZero = audit.InflightSessions == 0
	if !audit.InflightZero {
		audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "inflight_zero")
	}
	audit.CanEnable = audit.AllWritersMigrated && audit.ClusterDrainAck && audit.InflightZero &&
		audit.BatchQueueEmpty && audit.BalanceDrainInflightZero && audit.MaintenanceBackfillDone && audit.ProjectionPending == 0 && audit.RedisEpochConsistent
	return audit, nil
}

type QuotaWriterModeTransitionPlan struct {
	Current       QuotaWriterEpoch       `json:"current"`
	TargetMode    QuotaWriterMode        `json:"target_mode"`
	ProposedEpoch int64                  `json:"proposed_epoch"`
	Audit         DurableQuotaWriteAudit `json:"audit"`
	Ready         bool                   `json:"ready"`
	Validation    []string               `json:"validation"`
}

// PlanQuotaWriterModeTransition produces a reviewable plan. WP3-A intentionally
// exposes no apply API, so a gate-off process cannot switch modes automatically.
func PlanQuotaWriterModeTransition(ctx context.Context, db *gorm.DB, target QuotaWriterMode) (*QuotaWriterModeTransitionPlan, error) {
	if !validQuotaWriterMode(target) {
		return nil, fmt.Errorf("invalid quota writer target mode %q", target)
	}
	state, err := GetQuotaWriterEpochState(db)
	if err != nil {
		return nil, err
	}
	audit, auditErr := CanEnableDurableQuotaWrites(ctx, db)
	plan := &QuotaWriterModeTransitionPlan{Current: *state, TargetMode: target, ProposedEpoch: state.Epoch, Audit: audit}
	if state.Epoch == int64(^uint64(0)>>1) {
		plan.Validation = append(plan.Validation, "epoch_exhausted")
	} else {
		plan.ProposedEpoch = state.Epoch + 1
	}
	currentMode := QuotaWriterMode(state.Mode)
	switch currentMode {
	case QuotaWriterModeLegacy:
		if target != QuotaWriterModeBridge {
			plan.Validation = append(plan.Validation, "legacy_must_transition_to_bridge")
		}
		plan.Validation = append(plan.Validation, "cluster_drain_ack")
	case QuotaWriterModeBridge:
		if target != QuotaWriterModeAuthoritative {
			plan.Validation = append(plan.Validation, "bridge_must_transition_to_authoritative")
		}
		if !audit.CanEnable {
			plan.Validation = append(plan.Validation, "durable_write_audit_failed")
		}
	case QuotaWriterModeAuthoritative:
		if target != QuotaWriterModeAuthoritative {
			plan.Validation = append(plan.Validation, "authoritative_mode_is_not_downgradable")
		} else {
			plan.Validation = append(plan.Validation, "already_authoritative")
		}
	}
	plan.Ready = auditErr == nil && len(plan.Validation) == 0
	return plan, auditErr
}
