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
	now, err := taskRecoveryDBTimestamp(db)
	if err != nil {
		return err
	}
	candidate := newLegacyQuotaWriterEpoch(now)
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
		return err
	}
	_, err = GetQuotaWriterEpochState(db)
	return err
}

func GetQuotaWriterEpochState(db *gorm.DB) (*QuotaWriterEpoch, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var state QuotaWriterEpoch
	if err := db.Where("id = ?", quotaWriterEpochSingletonID).First(&state).Error; err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQuotaWriterEpochUnavailable, err)
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

// ProductionQuotaWriterRegistrations is deliberately explicit. WP3-A only
// migrates durable task receipts and the public MutateUserQuota wrapper; every
// legacy writer remains false until its owning WP3-B/C migration is complete.
func ProductionQuotaWriterRegistrations() []QuotaWriterRegistration {
	return []QuotaWriterRegistration{
		{Name: "durable_task_receipts", Migrated: true},
		{Name: "mutate_user_quota", Migrated: true},
		{Name: "increase_user_quota", Migrated: false},
		{Name: "decrease_user_quota", Migrated: false},
		{Name: "delta_update_user_quota", Migrated: false},
		{Name: "try_reserve_user_quota", Migrated: false},
		{Name: "increase_token_quota", Migrated: false},
		{Name: "decrease_token_quota", Migrated: false},
		{Name: "try_reserve_token_quota", Migrated: false},
		{Name: "credit_recharge_redemption_checkin", Migrated: false},
		{Name: "subscription_wallet_overflow", Migrated: false},
		{Name: "billing_session_callers", Migrated: false},
		{Name: "admin_quota_mutations", Migrated: false},
	}
}

type DurableQuotaWriteAudit struct {
	CanEnable             bool                      `json:"can_enable"`
	State                 *QuotaWriterEpoch         `json:"state,omitempty"`
	Writers               []QuotaWriterRegistration `json:"writers"`
	AllWritersMigrated    bool                      `json:"all_writers_migrated"`
	BatchQueueEmpty       bool                      `json:"batch_queue_empty"`
	ProjectionPending     int64                     `json:"projection_pending"`
	RedisEpochConsistent  bool                      `json:"redis_epoch_consistent"`
	ClusterDrainAck       bool                      `json:"cluster_drain_ack"`
	InflightZero          bool                      `json:"inflight_zero"`
	MissingOrFailedChecks []string                  `json:"missing_or_failed_checks,omitempty"`
}

func quotaBatchQueueEmpty() bool {
	for i := 0; i < BatchUpdateTypeCount; i++ {
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
	audit := DurableQuotaWriteAudit{Writers: ProductionQuotaWriterRegistrations()}
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
	// WP3-A has no distributed drain/inflight proof source. Missing evidence is
	// explicit and fail-closed; local process state must never stand in for it.
	audit.MissingOrFailedChecks = append(audit.MissingOrFailedChecks, "cluster_drain_ack", "inflight_zero")
	audit.CanEnable = audit.AllWritersMigrated && audit.ClusterDrainAck && audit.InflightZero &&
		audit.BatchQueueEmpty && audit.ProjectionPending == 0 && audit.RedisEpochConsistent
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
