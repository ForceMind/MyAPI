package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"sort"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SubscriptionShareTotalMicro       int64 = 100_000_000
	SubscriptionShareDefaultMaxAge    int64 = 300
	SubscriptionShareMaxUnsampledKey        = 8
	SubscriptionShareMaxUnsampledPool       = 64

	SubscriptionShareStateAwaitingBaseline = "awaiting_baseline"
	SubscriptionShareStateReady            = "ready"
	SubscriptionShareStateStopped          = "stopped"

	SubscriptionShareAttemptDispatching = "dispatching"
	SubscriptionShareAttemptNotSent     = "not_sent"
)

var (
	ErrSubscriptionShareInvalid        = errors.New("invalid subscription share input")
	ErrSubscriptionShareConflict       = errors.New("subscription share revision or evidence conflict")
	ErrSubscriptionShareDenied         = errors.New("subscription share access denied")
	ErrSubscriptionShareUnavailable    = errors.New("subscription share sample is unavailable")
	ErrSubscriptionShareExceeded       = errors.New("subscription share is exhausted")
	ErrSubscriptionShareAwaitingSample = errors.New("subscription share is awaiting a fresh sample")
	ErrSubscriptionShareDeleteDenied   = errors.New("subscription share allocation must be set to zero before token deletion")
)

// SubscriptionShareModels is the complete migration set for this feature.
func SubscriptionShareModels() []any {
	return []any{
		&SubscriptionShareAccountGate{}, &SubscriptionSharePool{}, &SubscriptionShareGeneration{},
		&SubscriptionShareAllocation{}, &SubscriptionShareAllocationUsage{}, &SubscriptionShareAttempt{},
		&SubscriptionShareAttemptWindow{}, &SubscriptionShareAudit{}, &SubscriptionShareSampleApplication{},
	}
}

// SubscriptionShareAccountGate serializes all pools for one provider-confirmed account.
type SubscriptionShareAccountGate struct {
	SubjectRef string `json:"-" gorm:"type:char(43);primaryKey"`
	Revision   int64  `json:"-" gorm:"type:bigint;not null"`
	UpdatedAt  int64  `json:"-" gorm:"type:bigint;not null"`
}

// SubscriptionSharePool identifies an approximate native subscription window.
// The descriptor deliberately has no upstream limitId because that identity is unavailable.
type SubscriptionSharePool struct {
	ID                 int64  `json:"pool_id" gorm:"primaryKey"`
	DescriptorKey      string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	SubjectRef         string `json:"-" gorm:"type:char(43);not null;index"`
	SnapshotID         int    `json:"snapshot_id" gorm:"not null"`
	ChannelID          int    `json:"-" gorm:"not null"`
	ChannelName        string `json:"channel_name" gorm:"type:varchar(255);not null"`
	WindowSource       string `json:"window_source" gorm:"type:varchar(64);not null"`
	WindowType         string `json:"window_type" gorm:"type:varchar(32);not null"`
	WindowSeconds      int64  `json:"window_seconds" gorm:"type:bigint;not null"`
	PlanType           string `json:"plan_type" gorm:"type:varchar(32);not null"`
	Revision           int64  `json:"revision" gorm:"type:bigint;not null"`
	CurrentGeneration  int64  `json:"-" gorm:"type:bigint;not null"`
	SelectedObservedAt int64  `json:"-" gorm:"type:bigint;not null"`
	ConfiguredAt       int64  `json:"-" gorm:"type:bigint;not null"`
	MaxAgeSeconds      int64  `json:"-" gorm:"type:bigint;not null"`
	Enabled            bool   `json:"-" gorm:"not null"`
	Approximate        bool   `json:"approximate" gorm:"not null"`
}

type SubscriptionShareGeneration struct {
	ID                int64  `json:"-" gorm:"primaryKey"`
	PoolID            int64  `json:"-" gorm:"not null;uniqueIndex:,composite:subscription_share_generation,priority:1"`
	Generation        int64  `json:"-" gorm:"type:bigint;not null;uniqueIndex:,composite:subscription_share_generation,priority:2"`
	BaselineTime      int64  `json:"baseline_time" gorm:"type:bigint;not null"`
	BaselineUsedMicro int64  `json:"-" gorm:"type:bigint;not null"`
	LastUsedMicro     int64  `json:"-" gorm:"type:bigint;not null"`
	UnattributedMicro int64  `json:"unattributed_micro" gorm:"type:bigint;not null"`
	LastSnapshotID    int    `json:"-" gorm:"not null"`
	ObservedAt        int64  `json:"observed_at" gorm:"type:bigint;not null"`
	ResetAt           int64  `json:"reset_at" gorm:"type:bigint;not null"`
	State             string `json:"state" gorm:"type:varchar(32);not null"`
	Reason            string `json:"reason" gorm:"type:varchar(64);not null"`
	DescriptorKey     string `json:"-" gorm:"type:char(64)"`
	WindowSource      string `json:"-" gorm:"type:varchar(64)"`
	WindowType        string `json:"-" gorm:"type:varchar(32)"`
	WindowSeconds     int64  `json:"-" gorm:"type:bigint"`
	PlanType          string `json:"-" gorm:"type:varchar(32)"`
	StoppedAt         int64  `json:"-" gorm:"type:bigint"`
	StoppedSnapshotID int    `json:"-"`
}

type SubscriptionShareAllocation struct {
	ID        int64 `json:"-" gorm:"primaryKey"`
	PoolID    int64 `json:"-" gorm:"not null;uniqueIndex:,composite:subscription_share_allocation,priority:1;index"`
	TokenID   int   `json:"-" gorm:"not null;uniqueIndex:,composite:subscription_share_allocation,priority:2;index"`
	UserID    int   `json:"-" gorm:"not null;index"`
	ShareBPS  int   `json:"share_bps" gorm:"not null"`
	CreatedAt int64 `json:"-" gorm:"type:bigint;not null"`
	UpdatedAt int64 `json:"-" gorm:"type:bigint;not null"`
}

type SubscriptionShareAllocationUsage struct {
	ID                 int64 `json:"-" gorm:"primaryKey"`
	AllocationID       int64 `json:"-" gorm:"not null;uniqueIndex:,composite:subscription_share_usage,priority:1"`
	Generation         int64 `json:"-" gorm:"type:bigint;not null;uniqueIndex:,composite:subscription_share_usage,priority:2"`
	EstimatedUsedMicro int64 `json:"estimated_used_micro" gorm:"type:bigint;not null"`
}

// SubscriptionShareAttempt is global by attempt id. CredentialRef must be an opaque identity reference.
type SubscriptionShareAttempt struct {
	AttemptID     string `json:"-" gorm:"type:char(64);primaryKey"`
	RequestID     string `json:"-" gorm:"type:varchar(128);not null"`
	TokenID       int    `json:"-" gorm:"not null;index"`
	UserID        int    `json:"-" gorm:"not null"`
	SubjectRef    string `json:"-" gorm:"type:char(43);not null;index"`
	ChannelID     int    `json:"-" gorm:"not null"`
	CredentialRef string `json:"-" gorm:"type:varchar(128);not null"`
	State         string `json:"-" gorm:"type:varchar(16);not null;index"`
	CreatedAt     int64  `json:"-" gorm:"type:bigint;not null"`
	UpdatedAt     int64  `json:"-" gorm:"type:bigint;not null"`
}

// CoveredAt releases the concurrency slot. AllocatedAt separately records a positive-delta attribution.
type SubscriptionShareAttemptWindow struct {
	ID           int64  `json:"-" gorm:"primaryKey"`
	AttemptID    string `json:"-" gorm:"type:char(64);not null;uniqueIndex:,composite:subscription_share_attempt_window,priority:1"`
	PoolID       int64  `json:"-" gorm:"not null;uniqueIndex:,composite:subscription_share_attempt_window,priority:2;index"`
	AllocationID int64  `json:"-" gorm:"not null;index"`
	Generation   int64  `json:"-" gorm:"type:bigint;not null;index"`
	CoveredAt    int64  `json:"-" gorm:"type:bigint;not null"`
	AllocatedAt  int64  `json:"-" gorm:"type:bigint;not null"`
}

type SubscriptionShareAudit struct {
	ID                     string `json:"-" gorm:"type:char(64);primaryKey"`
	Digest                 string `json:"-" gorm:"type:char(64);not null"`
	ActorID                int    `json:"-" gorm:"not null"`
	TokenID                int    `json:"-" gorm:"not null;index"`
	PoolID                 int64  `json:"-" gorm:"not null"`
	SnapshotID             int    `json:"-" gorm:"not null"`
	ExpectedRevision       int64  `json:"-" gorm:"type:bigint;not null"`
	BeforeShareBPS         int    `json:"-" gorm:"not null"`
	AfterShareBPS          int    `json:"-" gorm:"not null"`
	BeforeRevision         int64  `json:"-" gorm:"type:bigint;not null"`
	AfterRevision          int64  `json:"-" gorm:"type:bigint;not null"`
	CreatedAt              int64  `json:"-" gorm:"type:bigint;not null"`
	BeforeDescriptor       string `json:"-" gorm:"type:char(64)"`
	AfterDescriptor        string `json:"-" gorm:"type:char(64)"`
	BeforeGeneration       int64  `json:"-" gorm:"type:bigint"`
	AfterGeneration        int64  `json:"-" gorm:"type:bigint"`
	AllocationJSON         string `json:"-" gorm:"type:text"`
	Action                 string `json:"-" gorm:"type:varchar(32)"`
	BeforeUsedMicro        int64  `json:"-" gorm:"type:bigint"`
	AfterUsedMicro         int64  `json:"-" gorm:"type:bigint"`
	UnattributedDeltaMicro int64  `json:"-" gorm:"type:bigint"`
	UncertainAttempts      int64  `json:"-" gorm:"type:bigint"`
}

func (*SubscriptionShareAudit) BeforeUpdate(*gorm.DB) error { return ErrAccountQuotaReceiptImmutable }
func (*SubscriptionShareAudit) BeforeDelete(*gorm.DB) error { return ErrAccountQuotaReceiptImmutable }

// SubscriptionShareSampleApplication is the immutable attribution evidence
// for one persisted native sample. AllocationJSON contains only allocation ids,
// attempt counts, and fixed-point micro-percent amounts.
type SubscriptionShareSampleApplication struct {
	ID                     int64  `json:"-" gorm:"primaryKey"`
	PoolID                 int64  `json:"-" gorm:"not null;uniqueIndex:,composite:subscription_share_sample_application,priority:1"`
	Generation             int64  `json:"-" gorm:"type:bigint;not null;uniqueIndex:,composite:subscription_share_sample_application,priority:2"`
	SnapshotID             int    `json:"-" gorm:"not null;uniqueIndex:,composite:subscription_share_sample_application,priority:3"`
	FromSnapshotID         int    `json:"-" gorm:"not null"`
	BeforeUsedMicro        int64  `json:"-" gorm:"type:bigint;not null"`
	AfterUsedMicro         int64  `json:"-" gorm:"type:bigint;not null"`
	DeltaMicro             int64  `json:"-" gorm:"type:bigint;not null"`
	AttemptCount           int64  `json:"-" gorm:"type:bigint;not null"`
	AllocationJSON         string `json:"-" gorm:"type:text;not null"`
	UnattributedDeltaMicro int64  `json:"-" gorm:"type:bigint;not null"`
	Reason                 string `json:"-" gorm:"type:varchar(64);not null"`
	CreatedAt              int64  `json:"-" gorm:"type:bigint;not null"`
}

func (*SubscriptionShareSampleApplication) BeforeUpdate(*gorm.DB) error {
	return ErrAccountQuotaReceiptImmutable
}
func (*SubscriptionShareSampleApplication) BeforeDelete(*gorm.DB) error {
	return ErrAccountQuotaReceiptImmutable
}

type subscriptionShareSampleAllocation struct {
	AllocationID   int64 `json:"allocation_id"`
	AttemptCount   int64 `json:"attempt_count"`
	AllocatedMicro int64 `json:"allocated_micro"`
}

// ProtectedSubscriptionShareEvidenceQuery conservatively protects every
// snapshot at or after the oldest active projection cursor. Retention may keep
// extra unrelated rows, but cannot skip an unprocessed failure or conflict.
func ProtectedSubscriptionShareEvidenceQuery(db *gorm.DB) *gorm.DB {
	if db == nil {
		return db
	}
	generationTable := db.NamingStrategy.TableName("SubscriptionShareGeneration")
	poolTable := db.NamingStrategy.TableName("SubscriptionSharePool")
	oldestCursor := db.Table(generationTable+" AS g").Select("MIN(g.observed_at)").Joins("JOIN "+poolTable+" AS p ON p.id = g.pool_id AND p.current_generation = g.generation").Where("p.enabled = ?", true)
	oldestID := db.Table(generationTable+" AS g").Select("MIN(g.last_snapshot_id)").Joins("JOIN "+poolTable+" AS p ON p.id = g.pool_id AND p.current_generation = g.generation").Where("p.enabled = ?", true)
	return db.Model(&ChannelQuotaSnapshot{}).Where("observed_at >= (?) OR id > (?)", oldestCursor, oldestID)
}

func subscriptionShareAttemptTables(db *gorm.DB) (string, string) {
	return db.NamingStrategy.TableName("SubscriptionShareAttemptWindow"), db.NamingStrategy.TableName("SubscriptionShareAttempt")
}

const subscriptionShareAttemptWindowUpdateBatchSize = 500

// updateSubscriptionShareAttemptWindows applies one evidence transition in
// bounded statements. coveredAt only fills an unset coverage time; allocatedAt
// records the later attribution or uncertainty decision for every selected row.
func updateSubscriptionShareAttemptWindows(tx *gorm.DB, ids []int64, coveredAt int64, allocatedAt *int64) error {
	for start := 0; start < len(ids); start += subscriptionShareAttemptWindowUpdateBatchSize {
		end := min(start+subscriptionShareAttemptWindowUpdateBatchSize, len(ids))
		batch := ids[start:end]
		if coveredAt != 0 {
			if err := tx.Model(&SubscriptionShareAttemptWindow{}).Where("id IN ? AND covered_at = 0", batch).Update("covered_at", coveredAt).Error; err != nil {
				return err
			}
		}
		if allocatedAt != nil {
			if err := tx.Model(&SubscriptionShareAttemptWindow{}).Where("id IN ?", batch).Update("allocated_at", *allocatedAt).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func finalizeSubscriptionShareUncertainAttempts(tx *gorm.DB, poolID, generation, coveredAt int64) ([]subscriptionShareSampleAllocation, int64, error) {
	windowTable, attemptTable := subscriptionShareAttemptTables(tx)
	var rows []struct {
		WindowID     int64 `gorm:"column:window_id"`
		AllocationID int64 `gorm:"column:allocation_id"`
	}
	if err := lockForUpdate(tx).Table(windowTable+" AS w").Select("w.id AS window_id, w.allocation_id").Joins("JOIN "+attemptTable+" AS a ON a.attempt_id = w.attempt_id").Where("w.pool_id = ? AND w.generation = ? AND w.allocated_at = 0 AND a.state <> ?", poolID, generation, SubscriptionShareAttemptNotSent).Order("w.allocation_id ASC, w.id ASC").Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	evidence := make([]subscriptionShareSampleAllocation, 0)
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.WindowID)
		if len(evidence) == 0 || evidence[len(evidence)-1].AllocationID != row.AllocationID {
			evidence = append(evidence, subscriptionShareSampleAllocation{AllocationID: row.AllocationID, AttemptCount: 1})
		} else {
			evidence[len(evidence)-1].AttemptCount++
		}
	}
	uncertain := int64(-1)
	if err := updateSubscriptionShareAttemptWindows(tx, ids, coveredAt, &uncertain); err != nil {
		return nil, 0, err
	}
	return evidence, int64(len(ids)), nil
}

func runSubscriptionShareTransaction(ctx context.Context, db *gorm.DB, operation func(*gorm.DB) error) error {
	return runChannelQuotaIdentityTransaction(ctx, db, operation)
}

type SubscriptionShareConfigInput struct {
	ID                   string `json:"id"`
	PoolID               int64  `json:"pool_id"`
	SnapshotID           int    `json:"snapshot_id"`
	ExpectedRevision     int64  `json:"expected_revision"`
	ShareBPS             int    `json:"share_bps"`
	ConfirmedApproximate bool   `json:"confirmed_approximate"`
	ReplaceWindow        bool   `json:"replace_window"`
}

type SubscriptionShareCloseInput struct {
	ID               string `json:"id"`
	PoolID           int64  `json:"pool_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	ConfirmedClose   bool   `json:"confirmed_close"`
}

type SubscriptionSharePoolView struct {
	PoolID              int64  `json:"pool_id"`
	SnapshotID          int    `json:"snapshot_id"`
	Revision            int64  `json:"revision"`
	ChannelName         string `json:"channel_name"`
	WindowSource        string `json:"window_source"`
	WindowSlot          string `json:"window_slot"`
	WindowType          string `json:"window_type"`
	WindowSeconds       int64  `json:"window_seconds"`
	PlanType            string `json:"plan_type"`
	ResetAt             int64  `json:"reset_at"`
	ObservedAt          int64  `json:"observed_at"`
	State               string `json:"state"`
	Reason              string `json:"reason"`
	AllocatedBPS        int    `json:"allocated_bps"`
	ShareBPS            int    `json:"share_bps"`
	EstimatedUsedMicro  int64  `json:"estimated_used_micro"`
	UnattributedMicro   int64  `json:"unattributed_micro"`
	PendingAttempts     int64  `json:"pending_attempts"`
	UnsampledAttempts   int64  `json:"unsampled_attempts"`
	Approximate         bool   `json:"approximate"`
	BaselineTime        int64  `json:"baseline_time"`
	MaxAgeSeconds       int64  `json:"max_age_seconds"`
	SampleAgeSeconds    int64  `json:"sample_age_seconds"`
	ReplacementRequired bool   `json:"replacement_required"`
	RebaselineRequired  bool   `json:"rebaseline_required"`
	UncertainAttempts   int64  `json:"uncertain_attempts"`
}

type SubscriptionShareView struct {
	TokenID int                         `json:"token_id"`
	Pools   []SubscriptionSharePoolView `json:"pools"`
}

func subscriptionShareDescriptorKey(subject, source, windowType string, windowSeconds int64, planType string) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{subject, source, windowType, strconv.FormatInt(windowSeconds, 10), planType}, "\x00")))
	return fmt.Sprintf("%x", digest[:])
}

func subscriptionShareDigest(value any) (string, error) {
	encoded, err := common.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest[:]), nil
}

func subscriptionShareUsedMicro(snapshot ChannelQuotaSnapshot) (int64, error) {
	if snapshot.Used == nil || math.IsNaN(*snapshot.Used) || math.IsInf(*snapshot.Used, 0) || *snapshot.Used < 0 || *snapshot.Used > 100 {
		return 0, ErrSubscriptionShareUnavailable
	}
	value, err := decimal.NewFromString(strconv.FormatFloat(*snapshot.Used, 'f', -1, 64))
	if err != nil {
		return 0, ErrSubscriptionShareUnavailable
	}
	micro := value.Mul(decimal.NewFromInt(1_000_000))
	if !micro.Equal(micro.Truncate(0)) || micro.IsNegative() || micro.GreaterThan(decimal.NewFromInt(SubscriptionShareTotalMicro)) {
		return 0, ErrSubscriptionShareUnavailable
	}
	return micro.IntPart(), nil
}

func subscriptionShareProportionalMicro(delta, count, total int64) (int64, error) {
	if delta < 0 || delta > SubscriptionShareTotalMicro || count < 0 || total <= 0 || count > total {
		return 0, ErrSubscriptionShareInvalid
	}
	hi, lo := bits.Mul64(uint64(delta), uint64(count))
	quotient, _ := bits.Div64(hi, lo, uint64(total))
	if quotient > uint64(delta) {
		return 0, ErrSubscriptionShareInvalid
	}
	return int64(quotient), nil
}

func validSubscriptionShareSnapshot(snapshot ChannelQuotaSnapshot, now int64) bool {
	return validSubscriptionShareObservation(snapshot) &&
		snapshot.ObservedAt <= now && now-snapshot.ObservedAt <= SubscriptionShareDefaultMaxAge && snapshot.ResetAt > now
}

func validSubscriptionShareObservation(snapshot ChannelQuotaSnapshot) bool {
	return snapshot.Id > 0 && snapshot.CodexObservationQualified && snapshot.CodexThresholdQualified &&
		snapshot.IdentityQuality == ChannelQuotaIdentityQualityProviderConfirmed && canonicalChannelQuotaIdentitySubject(snapshot.SubjectRef) &&
		snapshot.MetricType == "codex_rate_limit" && (snapshot.Source == codexQuotaWindowPrimarySource || snapshot.Source == codexQuotaWindowSecondarySource) &&
		snapshot.Status == "success" && snapshot.Unit == "percent" && snapshot.Currency == "" && snapshot.Total != nil && *snapshot.Total == 100 &&
		snapshot.Used != nil && !math.IsNaN(*snapshot.Used) && !math.IsInf(*snapshot.Used, 0) && *snapshot.Used >= 0 && *snapshot.Used <= 100 &&
		!math.IsNaN(snapshot.Available) && !math.IsInf(snapshot.Available, 0) && math.Abs(snapshot.Available-(100-*snapshot.Used)) <= 1e-9 &&
		snapshot.WindowType != "" && snapshot.WindowType != "none" && snapshot.WindowType != "unknown" && snapshot.PlanType != "" && snapshot.WindowSeconds > 0 &&
		snapshot.ObservedAt > 0 && snapshot.ResetAt > snapshot.ObservedAt && snapshot.ResetAt-snapshot.ObservedAt <= snapshot.WindowSeconds
}

func lockSubscriptionShareGate(tx *gorm.DB, subjectRef string, now int64) error {
	// Under MySQL REPEATABLE READ, an ordinary query before this gate may have
	// established a stale read view. Callers must use locking reads after the
	// gate for rows that decide admission, allocation, replay, or finalization;
	// acquiring the gate does not refresh that read view.
	gate := SubscriptionShareAccountGate{SubjectRef: subjectRef, Revision: 1, UpdatedAt: now}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "subject_ref"}}, DoNothing: true}).Create(&gate).Error; err != nil {
		return err
	}
	return lockForUpdate(tx).Where("subject_ref = ?", subjectRef).Take(&gate).Error
}

func ConfigureSubscriptionShare(ctx context.Context, db *gorm.DB, actorID, tokenID int, input SubscriptionShareConfigInput, now int64) error {
	if db == nil || db.Dialector == nil {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if actorID <= 0 || tokenID <= 0 || now <= 0 || !canonicalLowerHex(input.ID, 64) || input.PoolID < 0 || input.SnapshotID <= 0 || input.ExpectedRevision < 0 || input.ShareBPS < 0 || input.ShareBPS > 10000 || !input.ConfirmedApproximate {
		return ErrSubscriptionShareInvalid
	}
	digest, err := subscriptionShareDigest(struct {
		ActorID int
		TokenID int
		Input   SubscriptionShareConfigInput
	}{actorID, tokenID, input})
	if err != nil {
		return err
	}
	return runSubscriptionShareTransaction(ctx, db, func(tx *gorm.DB) error {
		var actor User
		if err := tx.First(&actor, actorID).Error; err != nil {
			return err
		}
		if actor.Status != common.UserStatusEnabled || actor.Role != common.RoleRootUser {
			return ErrSubscriptionShareDenied
		}
		var token Token
		if err := lockForUpdate(tx).Unscoped().First(&token, tokenID).Error; err != nil {
			return err
		}
		if token.DeletedAt.Valid {
			return ErrSubscriptionShareDenied
		}
		var replay SubscriptionShareAudit
		replayErr := lockForUpdate(tx).First(&replay, "id = ?", input.ID).Error
		if replayErr == nil {
			if replay.Digest != digest {
				return ErrSubscriptionShareConflict
			}
			return nil
		}
		if !errors.Is(replayErr, gorm.ErrRecordNotFound) {
			return replayErr
		}
		var budget TokenBudget
		budgetErr := lockForUpdate(tx).First(&budget, "token_id = ?", tokenID).Error
		if budgetErr != nil && !errors.Is(budgetErr, gorm.ErrRecordNotFound) {
			return budgetErr
		}
		if budgetErr == nil && (budget.Enabled || budget.FeeEnabled) {
			return ErrSubscriptionShareConflict
		}
		var snapshot ChannelQuotaSnapshot
		if err := tx.First(&snapshot, input.SnapshotID).Error; err != nil {
			return err
		}
		if (input.PoolID == 0 || input.ReplaceWindow) && !validSubscriptionShareSnapshot(snapshot, now) || input.PoolID != 0 && !input.ReplaceWindow && !validSubscriptionShareObservation(snapshot) {
			return ErrSubscriptionShareUnavailable
		}
		snapshotUsedMicro, err := subscriptionShareUsedMicro(snapshot)
		if err != nil {
			return err
		}
		descriptor := subscriptionShareDescriptorKey(snapshot.SubjectRef, snapshot.Source, snapshot.WindowType, snapshot.WindowSeconds, snapshot.PlanType)
		gateSubject := snapshot.SubjectRef
		if input.PoolID != 0 {
			var existing struct {
				SubjectRef    string
				DescriptorKey string
				WindowSource  string
			}
			if err := tx.Model(&SubscriptionSharePool{}).Select("subject_ref", "descriptor_key", "window_source").Where("id = ?", input.PoolID).Take(&existing).Error; err != nil {
				return err
			}
			if existing.SubjectRef != snapshot.SubjectRef || input.ReplaceWindow && existing.WindowSource != snapshot.Source || !input.ReplaceWindow && existing.DescriptorKey != descriptor {
				return ErrSubscriptionShareConflict
			}
			gateSubject = existing.SubjectRef
		}
		if err := lockSubscriptionShareGate(tx, gateSubject, now); err != nil {
			return err
		}
		var pool SubscriptionSharePool
		if input.PoolID == 0 {
			if input.ReplaceWindow {
				return ErrSubscriptionShareInvalid
			}
			var occupied []SubscriptionSharePool
			if err := lockForUpdate(tx).Select("id").Where("subject_ref = ? AND window_source = ? AND enabled = ?", snapshot.SubjectRef, snapshot.Source, true).Order("id ASC").Limit(1).Find(&occupied).Error; err != nil {
				return err
			}
			if len(occupied) != 0 {
				return ErrSubscriptionShareConflict
			}
			var duplicate []SubscriptionSharePool
			if err := lockForUpdate(tx).Select("id").Where("descriptor_key = ?", descriptor).Order("id ASC").Limit(1).Find(&duplicate).Error; err != nil {
				return err
			}
			if len(duplicate) != 0 || input.ExpectedRevision != 0 {
				return ErrSubscriptionShareConflict
			}
			var channel Channel
			if err := tx.Select("id", "name").First(&channel, snapshot.ChannelId).Error; err != nil {
				return err
			}
			pool = SubscriptionSharePool{DescriptorKey: descriptor, SubjectRef: snapshot.SubjectRef, SnapshotID: snapshot.Id, ChannelID: snapshot.ChannelId, ChannelName: channel.Name, WindowSource: snapshot.Source, WindowType: snapshot.WindowType, WindowSeconds: snapshot.WindowSeconds, PlanType: snapshot.PlanType, Revision: 1, CurrentGeneration: 1, SelectedObservedAt: snapshot.ObservedAt, ConfiguredAt: now, MaxAgeSeconds: SubscriptionShareDefaultMaxAge, Enabled: true, Approximate: true}
			if err := tx.Create(&pool).Error; err != nil {
				return err
			}
			if err := tx.Create(&SubscriptionShareGeneration{PoolID: pool.ID, Generation: 1, LastSnapshotID: snapshot.Id, ObservedAt: snapshot.ObservedAt, ResetAt: snapshot.ResetAt, State: SubscriptionShareStateAwaitingBaseline, Reason: "baseline_required", DescriptorKey: descriptor, WindowSource: snapshot.Source, WindowType: snapshot.WindowType, WindowSeconds: snapshot.WindowSeconds, PlanType: snapshot.PlanType}).Error; err != nil {
				return err
			}
		} else {
			if err := lockForUpdate(tx).First(&pool, input.PoolID).Error; err != nil {
				return err
			}
			if pool.SubjectRef != snapshot.SubjectRef || pool.Revision != input.ExpectedRevision || !pool.Enabled || !pool.Approximate || input.ReplaceWindow && pool.WindowSource != snapshot.Source || !input.ReplaceWindow && pool.DescriptorKey != descriptor {
				return ErrSubscriptionShareConflict
			}
		}
		beforeRevision, beforeDescriptor, beforeGeneration := pool.Revision, pool.DescriptorKey, pool.CurrentGeneration
		if input.PoolID == 0 {
			beforeRevision, beforeDescriptor, beforeGeneration = 0, "", 0
		}
		rebaseline := input.ReplaceWindow && pool.DescriptorKey == descriptor
		var rebaselineGeneration SubscriptionShareGeneration
		rebaselineBeforeUsed := int64(0)
		uncertainCount := int64(0)
		if rebaseline {
			if err := lockForUpdate(tx).Where("pool_id = ? AND generation = ?", pool.ID, pool.CurrentGeneration).Take(&rebaselineGeneration).Error; err != nil {
				return err
			}
			resetEquivalent := snapshot.ResetAt >= rebaselineGeneration.ResetAt-2 && snapshot.ResetAt <= rebaselineGeneration.ResetAt+2
			if rebaselineGeneration.State != SubscriptionShareStateStopped || snapshot.ObservedAt <= rebaselineGeneration.StoppedAt || snapshot.Id != rebaselineGeneration.LastSnapshotID || !resetEquivalent || snapshotUsedMicro < rebaselineGeneration.LastUsedMicro {
				return ErrSubscriptionShareConflict
			}
			rebaselineBeforeUsed = rebaselineGeneration.LastUsedMicro
		}

		var poolAllocations []SubscriptionShareAllocation
		if err := lockForUpdate(tx).Where("pool_id = ?", pool.ID).Order("id ASC").Find(&poolAllocations).Error; err != nil {
			return err
		}
		var allocation SubscriptionShareAllocation
		allocationFound := false
		beforeShare := 0
		var allocated int64
		for _, item := range poolAllocations {
			if item.ShareBPS < 0 || item.ShareBPS > 10000 {
				return ErrSubscriptionShareUnavailable
			}
			if item.TokenID == tokenID {
				allocation, allocationFound = item, true
				if item.UserID != token.UserId {
					return ErrSubscriptionShareConflict
				}
				beforeShare = item.ShareBPS
				continue
			}
			allocated += int64(item.ShareBPS)
			if allocated > 10000 {
				return ErrSubscriptionShareInvalid
			}
		}
		if rebaseline && (!allocationFound || input.ShareBPS != beforeShare) {
			return ErrSubscriptionShareConflict
		}
		if allocated+int64(input.ShareBPS) > 10000 {
			return ErrSubscriptionShareInvalid
		}
		if allocationFound {
			if err := tx.Model(&SubscriptionShareAllocation{}).Where("id = ?", allocation.ID).Updates(map[string]any{"share_bps": input.ShareBPS, "updated_at": now}).Error; err != nil {
				return err
			}
		} else {
			allocation = SubscriptionShareAllocation{PoolID: pool.ID, TokenID: tokenID, UserID: token.UserId, ShareBPS: input.ShareBPS, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&allocation).Error; err != nil {
				return err
			}
			if !input.ReplaceWindow {
				if err := tx.Create(&SubscriptionShareAllocationUsage{AllocationID: allocation.ID, Generation: pool.CurrentGeneration}).Error; err != nil {
					return err
				}
			}
		}
		replacementEvidence := ""
		if input.PoolID != 0 {
			updates := map[string]any{"revision": pool.Revision + 1}
			if input.ReplaceWindow && !rebaseline {
				updates["descriptor_key"], updates["snapshot_id"], updates["window_type"], updates["window_seconds"], updates["plan_type"], updates["configured_at"], updates["current_generation"] = descriptor, snapshot.Id, snapshot.WindowType, snapshot.WindowSeconds, snapshot.PlanType, now, pool.CurrentGeneration+1
			} else if rebaseline {
				updates["snapshot_id"] = snapshot.Id
			}
			result := tx.Model(&SubscriptionSharePool{}).Where("id = ? AND revision = ?", pool.ID, pool.Revision).Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrSubscriptionShareConflict
			}
			pool.Revision++
			if input.ReplaceWindow && !rebaseline {
				_, uncertainCount, err = finalizeSubscriptionShareUncertainAttempts(tx, pool.ID, pool.CurrentGeneration, snapshot.ObservedAt)
				if err != nil {
					return err
				}
				pool.CurrentGeneration++
				if err := tx.Create(&SubscriptionShareGeneration{PoolID: pool.ID, Generation: pool.CurrentGeneration, LastSnapshotID: snapshot.Id, ObservedAt: snapshot.ObservedAt, ResetAt: snapshot.ResetAt, State: SubscriptionShareStateAwaitingBaseline, Reason: "baseline_required", DescriptorKey: descriptor, WindowSource: snapshot.Source, WindowType: snapshot.WindowType, WindowSeconds: snapshot.WindowSeconds, PlanType: snapshot.PlanType}).Error; err != nil {
					return err
				}
				var all []SubscriptionShareAllocation
				if err := lockForUpdate(tx).Where("pool_id = ?", pool.ID).Order("id ASC").Find(&all).Error; err != nil {
					return err
				}
				for _, item := range all {
					if err := tx.Create(&SubscriptionShareAllocationUsage{AllocationID: item.ID, Generation: pool.CurrentGeneration}).Error; err != nil {
						return err
					}
				}
				type replacementAllocation struct {
					AllocationID int64 `json:"allocation_id"`
					TokenID      int   `json:"token_id"`
					UserID       int   `json:"user_id"`
					ShareBPS     int   `json:"share_bps"`
				}
				evidence := make([]replacementAllocation, 0, len(all))
				for _, item := range all {
					evidence = append(evidence, replacementAllocation{item.ID, item.TokenID, item.UserID, item.ShareBPS})
				}
				encoded, err := common.Marshal(evidence)
				if err != nil {
					return err
				}
				replacementEvidence = string(encoded)
			} else if rebaseline {
				_, uncertainCount, err = finalizeSubscriptionShareUncertainAttempts(tx, pool.ID, pool.CurrentGeneration, snapshot.ObservedAt)
				if err != nil {
					return err
				}
				unattributedDelta := int64(0)
				if snapshotUsedMicro > rebaselineGeneration.LastUsedMicro {
					unattributedDelta = snapshotUsedMicro - rebaselineGeneration.LastUsedMicro
					rebaselineGeneration.LastUsedMicro = snapshotUsedMicro
					rebaselineGeneration.UnattributedMicro += unattributedDelta
				}
				rebaselineGeneration.BaselineTime, rebaselineGeneration.BaselineUsedMicro = snapshot.ObservedAt, snapshotUsedMicro
				rebaselineGeneration.LastSnapshotID, rebaselineGeneration.ObservedAt = snapshot.Id, snapshot.ObservedAt
				rebaselineGeneration.ResetAt, rebaselineGeneration.State, rebaselineGeneration.Reason = snapshot.ResetAt, SubscriptionShareStateReady, "observation_rebaseline"
				if err := tx.Model(&SubscriptionShareGeneration{}).Where("id = ?", rebaselineGeneration.ID).Updates(map[string]any{"baseline_time": rebaselineGeneration.BaselineTime, "baseline_used_micro": rebaselineGeneration.BaselineUsedMicro, "last_used_micro": rebaselineGeneration.LastUsedMicro, "unattributed_micro": rebaselineGeneration.UnattributedMicro, "last_snapshot_id": snapshot.Id, "observed_at": snapshot.ObservedAt, "reset_at": snapshot.ResetAt, "state": rebaselineGeneration.State, "reason": rebaselineGeneration.Reason}).Error; err != nil {
					return err
				}
			}
		}
		audit := SubscriptionShareAudit{ID: input.ID, Digest: digest, ActorID: actorID, TokenID: tokenID, PoolID: pool.ID, SnapshotID: input.SnapshotID, ExpectedRevision: input.ExpectedRevision, BeforeShareBPS: beforeShare, AfterShareBPS: input.ShareBPS, BeforeRevision: beforeRevision, AfterRevision: pool.Revision, CreatedAt: now, BeforeDescriptor: beforeDescriptor, AfterDescriptor: descriptor, BeforeGeneration: beforeGeneration, AfterGeneration: pool.CurrentGeneration, AllocationJSON: replacementEvidence}
		if input.PoolID == 0 {
			audit.Action = "pool_creation"
		} else if input.ReplaceWindow && !rebaseline {
			audit.Action, audit.UncertainAttempts = "window_replacement", uncertainCount
		} else if rebaseline {
			audit.Action, audit.BeforeUsedMicro, audit.AfterUsedMicro = "observation_rebaseline", rebaselineBeforeUsed, snapshotUsedMicro
			if snapshotUsedMicro > audit.BeforeUsedMicro {
				audit.UnattributedDeltaMicro = snapshotUsedMicro - audit.BeforeUsedMicro
			}
			audit.UncertainAttempts = uncertainCount
		} else if !allocationFound {
			audit.Action = "allocation_create"
		} else {
			audit.Action = "allocation_update"
		}
		created := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&audit)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected != 1 {
			return ErrSubscriptionShareConflict
		}
		return nil
	})
}

// CloseSubscriptionSharePool permanently retires one managed descriptor while
// retaining its allocation, usage, attempt, and sample evidence.
func CloseSubscriptionSharePool(ctx context.Context, db *gorm.DB, actorID int, input SubscriptionShareCloseInput, now int64) error {
	if db == nil || db.Dialector == nil {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if actorID <= 0 || now <= 0 || !canonicalLowerHex(input.ID, 64) || input.PoolID <= 0 || input.ExpectedRevision <= 0 || !input.ConfirmedClose {
		return ErrSubscriptionShareInvalid
	}
	digest, err := subscriptionShareDigest(struct {
		ActorID int
		Input   SubscriptionShareCloseInput
	}{actorID, input})
	if err != nil {
		return err
	}
	return runSubscriptionShareTransaction(ctx, db, func(tx *gorm.DB) error {
		var actor User
		if err := tx.First(&actor, actorID).Error; err != nil {
			return err
		}
		if actor.Status != common.UserStatusEnabled || actor.Role != common.RoleRootUser {
			return ErrSubscriptionShareDenied
		}
		var identity SubscriptionSharePool
		if err := tx.Select("id", "subject_ref").First(&identity, input.PoolID).Error; err != nil {
			return err
		}
		if err := lockSubscriptionShareGate(tx, identity.SubjectRef, now); err != nil {
			return err
		}
		var pool SubscriptionSharePool
		if err := lockForUpdate(tx).First(&pool, input.PoolID).Error; err != nil {
			return err
		}
		if pool.SubjectRef != identity.SubjectRef {
			return ErrSubscriptionShareConflict
		}
		var replay SubscriptionShareAudit
		replayErr := lockForUpdate(tx).First(&replay, "id = ?", input.ID).Error
		if replayErr == nil {
			if replay.Digest != digest || replay.PoolID != pool.ID || replay.Action != "pool_close" {
				return ErrSubscriptionShareConflict
			}
			return nil
		}
		if !errors.Is(replayErr, gorm.ErrRecordNotFound) {
			return replayErr
		}
		if !pool.Enabled || pool.Revision != input.ExpectedRevision || pool.CurrentGeneration <= 0 {
			return ErrSubscriptionShareConflict
		}
		var generation SubscriptionShareGeneration
		if err := lockForUpdate(tx).Where("pool_id = ? AND generation = ?", pool.ID, pool.CurrentGeneration).Take(&generation).Error; err != nil {
			return err
		}
		var allocations []SubscriptionShareAllocation
		if err := lockForUpdate(tx).Where("pool_id = ?", pool.ID).Order("id ASC").Find(&allocations).Error; err != nil {
			return err
		}
		type closeAllocation struct {
			AllocationID int64 `json:"allocation_id"`
			TokenID      int   `json:"token_id"`
			UserID       int   `json:"user_id"`
			ShareBPS     int   `json:"share_bps"`
		}
		allocationEvidence := make([]closeAllocation, 0, len(allocations))
		for _, allocation := range allocations {
			allocationEvidence = append(allocationEvidence, closeAllocation{allocation.ID, allocation.TokenID, allocation.UserID, allocation.ShareBPS})
		}
		encoded, err := common.Marshal(allocationEvidence)
		if err != nil {
			return err
		}
		_, uncertainCount, err := finalizeSubscriptionShareUncertainAttempts(tx, pool.ID, pool.CurrentGeneration, now)
		if err != nil {
			return err
		}
		if err := tx.Model(&SubscriptionShareGeneration{}).Where("id = ?", generation.ID).Updates(map[string]any{
			"state": SubscriptionShareStateStopped, "reason": "pool_closed", "stopped_at": now, "stopped_snapshot_id": generation.LastSnapshotID,
		}).Error; err != nil {
			return err
		}
		result := tx.Model(&SubscriptionSharePool{}).Where("id = ? AND revision = ? AND enabled = ?", pool.ID, pool.Revision, true).Updates(map[string]any{"enabled": false, "revision": pool.Revision + 1})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrSubscriptionShareConflict
		}
		audit := SubscriptionShareAudit{
			ID: input.ID, Digest: digest, ActorID: actorID, PoolID: pool.ID, SnapshotID: pool.SnapshotID,
			ExpectedRevision: input.ExpectedRevision, BeforeRevision: pool.Revision, AfterRevision: pool.Revision + 1,
			CreatedAt: now, BeforeDescriptor: pool.DescriptorKey, AfterDescriptor: pool.DescriptorKey,
			BeforeGeneration: pool.CurrentGeneration, AfterGeneration: pool.CurrentGeneration,
			AllocationJSON: string(encoded), Action: "pool_close", UncertainAttempts: uncertainCount,
		}
		created := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&audit)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected != 1 {
			return ErrSubscriptionShareConflict
		}
		return nil
	})
}

func HasSubscriptionShareBinding(ctx context.Context, db *gorm.DB, tokenID int) (bool, error) {
	if db == nil || tokenID <= 0 {
		if db == nil {
			return false, gorm.ErrInvalidDB
		}
		return false, ErrSubscriptionShareInvalid
	}
	var count int64
	enabledPools := db.WithContext(ctx).Model(&SubscriptionSharePool{}).Select("id").Where("enabled = ?", true)
	err := db.WithContext(ctx).Model(&SubscriptionShareAllocation{}).Where("token_id = ? AND pool_id IN (?)", tokenID, enabledPools).Count(&count).Error
	return count > 0, err
}

func HasSubscriptionSharePools(ctx context.Context, db *gorm.DB) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	var count int64
	err := db.WithContext(ctx).Model(&SubscriptionSharePool{}).Where("enabled = ?", true).Count(&count).Error
	return count > 0, err
}

func SubscriptionShareDispatchRequired(ctx context.Context, db *gorm.DB, tokenID int, subjectRef string) (bool, error) {
	bound, err := HasSubscriptionShareBinding(ctx, db, tokenID)
	if err != nil || bound {
		return bound, err
	}
	if subjectRef == "" {
		return false, nil
	}
	if !canonicalChannelQuotaIdentitySubject(subjectRef) {
		return false, ErrSubscriptionShareInvalid
	}
	var count int64
	err = db.WithContext(ctx).Model(&SubscriptionSharePool{}).Where("subject_ref = ? AND enabled = ?", subjectRef, true).Count(&count).Error
	return count > 0, err
}

// CheckSubscriptionShareEligibility is a non-reserving routing check. Final
// dispatch must still call AdmitSubscriptionShareAttempt in the same selected
// account context immediately before the upstream request.
func CheckSubscriptionShareEligibility(ctx context.Context, db *gorm.DB, tokenID int, subjectRef string, now int64) (bool, error) {
	if db == nil || db.Dialector == nil {
		return false, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if tokenID <= 0 || now <= 0 {
		return false, ErrSubscriptionShareInvalid
	}
	if subjectRef == "" {
		bound, err := HasSubscriptionShareBinding(ctx, db, tokenID)
		if err != nil {
			return false, err
		}
		if bound {
			return false, ErrSubscriptionShareDenied
		}
		managed, err := HasSubscriptionSharePools(ctx, db)
		if err != nil {
			return false, err
		}
		if managed {
			return false, ErrSubscriptionShareUnavailable
		}
		return true, nil
	}
	if !canonicalChannelQuotaIdentitySubject(subjectRef) {
		return false, ErrSubscriptionShareInvalid
	}
	eligible := false
	var decisionErr error
	err := runSubscriptionShareTransaction(ctx, db, func(tx *gorm.DB) error {
		eligible, decisionErr = false, nil
		var token Token
		if err := lockForUpdate(tx).First(&token, tokenID).Error; err != nil {
			return err
		}
		if token.Status != common.TokenStatusEnabled {
			return ErrSubscriptionShareDenied
		}
		if err := lockSubscriptionShareGate(tx, subjectRef, now); err != nil {
			return err
		}
		var bindings []SubscriptionShareAllocation
		enabledPoolIDs := tx.Model(&SubscriptionSharePool{}).Select("id").Where("enabled = ?", true)
		if err := tx.Where("token_id = ? AND pool_id IN (?)", tokenID, enabledPoolIDs).Find(&bindings).Error; err != nil {
			return err
		}
		var pools []SubscriptionSharePool
		if err := lockForUpdate(tx).Where("subject_ref = ? AND enabled = ?", subjectRef, true).Order("id ASC").Find(&pools).Error; err != nil {
			return err
		}
		if len(pools) == 0 {
			eligible = len(bindings) == 0
			if !eligible {
				return ErrSubscriptionShareDenied
			}
			return nil
		}
		backlog, err := refreshSubscriptionSharePools(tx, subjectRef, now, pools)
		if err != nil {
			return err
		}
		if backlog {
			decisionErr = ErrSubscriptionShareAwaitingSample
			return nil
		}
		if err := lockForUpdate(tx).Where("subject_ref = ? AND enabled = ?", subjectRef, true).Order("id ASC").Find(&pools).Error; err != nil {
			return err
		}
		byPool := make(map[int64]SubscriptionShareAllocation, len(bindings))
		for _, binding := range bindings {
			byPool[binding.PoolID] = binding
		}
		windowTable, attemptTable := subscriptionShareAttemptTables(tx)
		for _, pool := range pools {
			allocation, ok := byPool[pool.ID]
			if !ok || allocation.UserID != token.UserId {
				decisionErr = ErrSubscriptionShareDenied
				return nil
			}
			var generation SubscriptionShareGeneration
			if err := tx.Where("pool_id = ? AND generation = ?", pool.ID, pool.CurrentGeneration).Take(&generation).Error; err != nil {
				return err
			}
			if generation.BaselineTime == 0 {
				decisionErr = ErrSubscriptionShareAwaitingSample
				return nil
			}
			if generation.State != SubscriptionShareStateReady || generation.ObservedAt <= 0 || generation.ObservedAt > now || now-generation.ObservedAt > pool.MaxAgeSeconds || generation.ResetAt <= now {
				decisionErr = ErrSubscriptionShareUnavailable
				return nil
			}
			var usage SubscriptionShareAllocationUsage
			err := tx.Where("allocation_id = ? AND generation = ?", allocation.ID, pool.CurrentGeneration).Take(&usage).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				decisionErr = ErrSubscriptionShareUnavailable
				return nil
			}
			if err != nil {
				return err
			}
			if usage.EstimatedUsedMicro < 0 {
				decisionErr = ErrSubscriptionShareUnavailable
				return nil
			}
			if allocation.ShareBPS == 0 || usage.EstimatedUsedMicro >= int64(allocation.ShareBPS)*10_000 {
				decisionErr = ErrSubscriptionShareExceeded
				return nil
			}
			var keyUnsampled, poolUnsampled int64
			if err := tx.Table(windowTable+" AS w").Joins("JOIN "+attemptTable+" AS a ON a.attempt_id = w.attempt_id").Where("w.pool_id = ? AND w.generation = ? AND w.allocation_id = ? AND w.covered_at = 0 AND a.state <> ?", pool.ID, pool.CurrentGeneration, allocation.ID, SubscriptionShareAttemptNotSent).Count(&keyUnsampled).Error; err != nil {
				return err
			}
			if err := tx.Table(windowTable+" AS w").Joins("JOIN "+attemptTable+" AS a ON a.attempt_id = w.attempt_id").Where("w.pool_id = ? AND w.generation = ? AND w.covered_at = 0 AND a.state <> ?", pool.ID, pool.CurrentGeneration, SubscriptionShareAttemptNotSent).Count(&poolUnsampled).Error; err != nil {
				return err
			}
			if keyUnsampled >= SubscriptionShareMaxUnsampledKey || poolUnsampled >= SubscriptionShareMaxUnsampledPool {
				decisionErr = ErrSubscriptionShareAwaitingSample
				return nil
			}
		}
		eligible = true
		return nil
	})
	if err == nil && decisionErr != nil {
		return false, decisionErr
	}
	return eligible, err
}

func validSubscriptionShareAttemptIdentity(requestID, attemptID, credentialRef string) bool {
	return canonicalLowerHex(attemptID, 64) && requestID != "" && len(requestID) <= 128 && credentialRef != "" && len(credentialRef) <= 128 && strings.TrimSpace(credentialRef) == credentialRef
}

func AdmitSubscriptionShareAttempt(ctx context.Context, db *gorm.DB, tokenID, userID int, subjectRef string, channelID int, credentialRef, requestID, attemptID string, now int64) error {
	if db == nil || db.Dialector == nil {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if tokenID <= 0 || userID <= 0 || channelID <= 0 || now <= 0 || !canonicalChannelQuotaIdentitySubject(subjectRef) || !validSubscriptionShareAttemptIdentity(requestID, attemptID, credentialRef) {
		return ErrSubscriptionShareInvalid
	}
	var decisionErr error
	err := runSubscriptionShareTransaction(ctx, db, func(tx *gorm.DB) error {
		decisionErr = nil
		var token Token
		if err := lockForUpdate(tx).First(&token, tokenID).Error; err != nil {
			return err
		}
		if token.UserId != userID || token.Status != common.TokenStatusEnabled {
			return ErrSubscriptionShareDenied
		}
		if err := lockSubscriptionShareGate(tx, subjectRef, now); err != nil {
			return err
		}
		var existing SubscriptionShareAttempt
		err := tx.First(&existing, "attempt_id = ?", attemptID).Error
		if err == nil {
			if existing.RequestID != requestID || existing.TokenID != tokenID || existing.UserID != userID || existing.SubjectRef != subjectRef || existing.ChannelID != channelID || existing.CredentialRef != credentialRef {
				return ErrSubscriptionShareConflict
			}
			return ErrSubscriptionShareConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var allBindings []SubscriptionShareAllocation
		enabledPoolIDs := tx.Model(&SubscriptionSharePool{}).Select("id").Where("enabled = ?", true)
		if err := tx.Where("token_id = ? AND pool_id IN (?)", tokenID, enabledPoolIDs).Find(&allBindings).Error; err != nil {
			return err
		}
		var pools []SubscriptionSharePool
		if err := lockForUpdate(tx).Where("subject_ref = ? AND enabled = ?", subjectRef, true).Order("id ASC").Find(&pools).Error; err != nil {
			return err
		}
		if len(pools) == 0 {
			if len(allBindings) != 0 {
				return ErrSubscriptionShareDenied
			}
			return nil
		}
		backlog, err := refreshSubscriptionSharePools(tx, subjectRef, now, pools)
		if err != nil {
			return err
		}
		if backlog {
			decisionErr = ErrSubscriptionShareAwaitingSample
			return nil
		}
		if err := lockForUpdate(tx).Where("subject_ref = ? AND enabled = ?", subjectRef, true).Order("id ASC").Find(&pools).Error; err != nil {
			return err
		}
		allocationByPool := make(map[int64]SubscriptionShareAllocation, len(allBindings))
		for _, allocation := range allBindings {
			allocationByPool[allocation.PoolID] = allocation
		}
		selected := make([]SubscriptionShareAllocation, 0, len(pools))
		windowTable, attemptTable := subscriptionShareAttemptTables(tx)
		for _, pool := range pools {
			allocation, ok := allocationByPool[pool.ID]
			if !ok || allocation.UserID != userID {
				decisionErr = ErrSubscriptionShareDenied
				return nil
			}
			if pool.CurrentGeneration <= 0 {
				decisionErr = ErrSubscriptionShareAwaitingSample
				return nil
			}
			var generation SubscriptionShareGeneration
			if err := lockForUpdate(tx).Where("pool_id = ? AND generation = ?", pool.ID, pool.CurrentGeneration).Take(&generation).Error; err != nil {
				return err
			}
			if generation.State != SubscriptionShareStateReady || generation.ObservedAt <= 0 || generation.ObservedAt > now || now-generation.ObservedAt > pool.MaxAgeSeconds || generation.ResetAt <= now {
				decisionErr = ErrSubscriptionShareUnavailable
				return nil
			}
			var usage SubscriptionShareAllocationUsage
			err := lockForUpdate(tx).Where("allocation_id = ? AND generation = ?", allocation.ID, pool.CurrentGeneration).Take(&usage).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				decisionErr = ErrSubscriptionShareUnavailable
				return nil
			}
			if err != nil {
				return err
			}
			if usage.EstimatedUsedMicro < 0 {
				decisionErr = ErrSubscriptionShareUnavailable
				return nil
			}
			if allocation.ShareBPS == 0 || usage.EstimatedUsedMicro >= int64(allocation.ShareBPS)*10_000 {
				decisionErr = ErrSubscriptionShareExceeded
				return nil
			}
			var keyUnsampled, poolUnsampled int64
			base := tx.Table(windowTable+" AS w").Joins("JOIN "+attemptTable+" AS a ON a.attempt_id = w.attempt_id").Where("w.pool_id = ? AND w.generation = ? AND w.covered_at = 0 AND a.state <> ?", pool.ID, pool.CurrentGeneration, SubscriptionShareAttemptNotSent)
			if err := base.Where("w.allocation_id = ?", allocation.ID).Count(&keyUnsampled).Error; err != nil {
				return err
			}
			if err := tx.Table(windowTable+" AS w").Joins("JOIN "+attemptTable+" AS a ON a.attempt_id = w.attempt_id").Where("w.pool_id = ? AND w.generation = ? AND w.covered_at = 0 AND a.state <> ?", pool.ID, pool.CurrentGeneration, SubscriptionShareAttemptNotSent).Count(&poolUnsampled).Error; err != nil {
				return err
			}
			if keyUnsampled >= SubscriptionShareMaxUnsampledKey || poolUnsampled >= SubscriptionShareMaxUnsampledPool {
				decisionErr = ErrSubscriptionShareAwaitingSample
				return nil
			}
			selected = append(selected, allocation)
		}
		attempt := SubscriptionShareAttempt{AttemptID: attemptID, RequestID: requestID, TokenID: tokenID, UserID: userID, SubjectRef: subjectRef, ChannelID: channelID, CredentialRef: credentialRef, State: SubscriptionShareAttemptDispatching, CreatedAt: now, UpdatedAt: now}
		created := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "attempt_id"}}, DoNothing: true}).Create(&attempt)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected != 1 {
			return ErrSubscriptionShareConflict
		}
		for index, pool := range pools {
			if err := tx.Create(&SubscriptionShareAttemptWindow{AttemptID: attemptID, PoolID: pool.ID, AllocationID: selected[index].ID, Generation: pool.CurrentGeneration}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil && decisionErr != nil {
		return decisionErr
	}
	return err
}

func MarkSubscriptionShareAttemptNotSent(ctx context.Context, db *gorm.DB, attemptID string, now int64) error {
	if db == nil || now <= 0 {
		if db == nil {
			return gorm.ErrInvalidDB
		}
		return ErrSubscriptionShareInvalid
	}
	if !canonicalLowerHex(attemptID, 64) {
		return ErrSubscriptionShareInvalid
	}
	return runSubscriptionShareTransaction(ctx, db, func(tx *gorm.DB) error {
		var attempt SubscriptionShareAttempt
		if err := tx.First(&attempt, "attempt_id = ?", attemptID).Error; err != nil {
			return err
		}
		if err := lockSubscriptionShareGate(tx, attempt.SubjectRef, now); err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&attempt, "attempt_id = ?", attemptID).Error; err != nil {
			return err
		}
		if attempt.State == SubscriptionShareAttemptNotSent {
			return nil
		}
		if attempt.State != SubscriptionShareAttemptDispatching {
			return ErrSubscriptionShareConflict
		}
		var windows []SubscriptionShareAttemptWindow
		if err := lockForUpdate(tx).Where("attempt_id = ?", attemptID).Order("id ASC").Find(&windows).Error; err != nil {
			return err
		}
		for _, window := range windows {
			if window.AllocatedAt != 0 {
				return ErrSubscriptionShareConflict
			}
		}
		updated := tx.Model(&SubscriptionShareAttempt{}).Where("attempt_id = ? AND state = ?", attemptID, SubscriptionShareAttemptDispatching).Updates(map[string]any{"state": SubscriptionShareAttemptNotSent, "updated_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrSubscriptionShareConflict
		}
		return nil
	})
}

func RefreshSubscriptionShareProjection(ctx context.Context, db *gorm.DB, subjectRef string, now int64) error {
	if db == nil || db.Dialector == nil {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !canonicalChannelQuotaIdentitySubject(subjectRef) || now <= 0 {
		return ErrSubscriptionShareInvalid
	}
	for {
		backlog := false
		err := runSubscriptionShareTransaction(ctx, db, func(tx *gorm.DB) error {
			if err := lockSubscriptionShareGate(tx, subjectRef, now); err != nil {
				return err
			}
			var pools []SubscriptionSharePool
			if err := lockForUpdate(tx).Where("subject_ref = ? AND enabled = ?", subjectRef, true).Order("id ASC").Find(&pools).Error; err != nil {
				return err
			}
			var err error
			backlog, err = refreshSubscriptionSharePools(tx, subjectRef, now, pools)
			return err
		})
		if err != nil || !backlog {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func refreshSubscriptionSharePools(tx *gorm.DB, subjectRef string, now int64, pools []SubscriptionSharePool) (bool, error) {
	backlog := false
	for index := range pools {
		pool := &pools[index]
		var generation SubscriptionShareGeneration
		if err := lockForUpdate(tx).Where("pool_id = ? AND generation = ?", pool.ID, pool.CurrentGeneration).Take(&generation).Error; err != nil {
			return false, err
		}
		var snapshots []ChannelQuotaSnapshot
		// Follow the complete event stream for this slot. A generic sampler
		// failure applies to every slot, while a slot-specific absence or a
		// changed descriptor must stop the old pool instead of being filtered
		// out and allowing a later success to bridge the discontinuity.
		query := tx.Where("subject_ref = ? AND identity_quality = ? AND metric_type = ? AND source IN ? AND id > ?", subjectRef, ChannelQuotaIdentityQualityProviderConfirmed, "codex_rate_limit", []string{pool.WindowSource, "codex_wham_usage"}, generation.LastSnapshotID)
		if generation.BaselineTime == 0 {
			// Observation time has second precision. Requiring a strictly later
			// timestamp prevents an already persisted same-second row from being
			// mistaken for post-enable evidence.
			query = query.Where("observed_at > ?", pool.ConfiguredAt)
		}
		if err := query.Order("id ASC").Limit(257).Find(&snapshots).Error; err != nil {
			return false, err
		}
		if len(snapshots) > 256 {
			backlog = true
			snapshots = snapshots[:256]
		}
		for _, snapshot := range snapshots {
			if err := applySubscriptionShareSnapshot(tx, pool, &generation, snapshot); err != nil {
				return false, err
			}
		}
	}
	return backlog, nil
}

func applySubscriptionShareSnapshot(tx *gorm.DB, pool *SubscriptionSharePool, generation *SubscriptionShareGeneration, snapshot ChannelQuotaSnapshot) error {
	fromSnapshotID, beforeUsed := generation.LastSnapshotID, generation.LastUsedMicro
	windowTable, attemptTable := subscriptionShareAttemptTables(tx)
	qualified := validSubscriptionShareObservation(snapshot)
	usedMicro, usedErr := subscriptionShareUsedMicro(snapshot)
	if usedErr != nil {
		qualified = false
	}
	descriptorMatched := snapshot.Source == pool.WindowSource && snapshot.WindowType == pool.WindowType && snapshot.WindowSeconds == pool.WindowSeconds && snapshot.PlanType == pool.PlanType
	chronological := snapshot.ObservedAt > generation.ObservedAt
	resetEquivalent := snapshot.ResetAt >= generation.ResetAt-2 && snapshot.ResetAt <= generation.ResetAt+2
	zeroWindowShift := qualified && generation.LastUsedMicro == 0 && usedMicro == 0 && snapshot.ObservedAt < generation.ResetAt && snapshot.ResetAt > generation.ResetAt
	duplicateObservation := snapshot.ObservedAt == generation.ObservedAt && qualified && descriptorMatched && resetEquivalent && usedMicro == generation.LastUsedMicro
	if duplicateObservation {
		generation.LastSnapshotID = snapshot.Id
		if err := tx.Model(&SubscriptionShareGeneration{}).Where("id = ?", generation.ID).Update("last_snapshot_id", snapshot.Id).Error; err != nil {
			return err
		}
		if err := tx.Model(&SubscriptionSharePool{}).Where("id = ?", pool.ID).Update("snapshot_id", snapshot.Id).Error; err != nil {
			return err
		}
		pool.SnapshotID = snapshot.Id
		return recordSubscriptionShareSampleApplication(tx, pool.ID, generation.Generation, snapshot, fromSnapshotID, beforeUsed, beforeUsed, 0, nil, 0, "duplicate_observation")
	}
	credibleReset := chronological && qualified && descriptorMatched && snapshot.ResetAt > generation.ResetAt+2 && snapshot.ObservedAt >= generation.ResetAt
	if credibleReset {
		uncertainEvidence, _, err := finalizeSubscriptionShareUncertainAttempts(tx, pool.ID, pool.CurrentGeneration, snapshot.ObservedAt)
		if err != nil {
			return err
		}
		newGeneration := pool.CurrentGeneration + 1
		next := SubscriptionShareGeneration{PoolID: pool.ID, Generation: newGeneration, BaselineTime: snapshot.ObservedAt, BaselineUsedMicro: usedMicro, LastUsedMicro: usedMicro, UnattributedMicro: usedMicro, LastSnapshotID: snapshot.Id, ObservedAt: snapshot.ObservedAt, ResetAt: snapshot.ResetAt, State: SubscriptionShareStateReady, Reason: "eligible", DescriptorKey: pool.DescriptorKey, WindowSource: pool.WindowSource, WindowType: pool.WindowType, WindowSeconds: pool.WindowSeconds, PlanType: pool.PlanType}
		if err := tx.Create(&next).Error; err != nil {
			return err
		}
		result := tx.Model(&SubscriptionSharePool{}).Where("id = ? AND current_generation = ?", pool.ID, pool.CurrentGeneration).Updates(map[string]any{"current_generation": newGeneration, "snapshot_id": snapshot.Id})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrSubscriptionShareConflict
		}
		var allocations []SubscriptionShareAllocation
		if err := tx.Where("pool_id = ?", pool.ID).Find(&allocations).Error; err != nil {
			return err
		}
		for _, allocation := range allocations {
			if err := tx.Create(&SubscriptionShareAllocationUsage{AllocationID: allocation.ID, Generation: newGeneration}).Error; err != nil {
				return err
			}
		}
		pool.CurrentGeneration = newGeneration
		*generation = next
		return recordSubscriptionShareSampleApplication(tx, pool.ID, newGeneration, snapshot, fromSnapshotID, beforeUsed, usedMicro, 0, uncertainEvidence, usedMicro, "reset_baseline")
	}

	state, reason := generation.State, generation.Reason
	applicationReason := "zero_delta"
	unattributedDelta := int64(0)
	allocationEvidence := []subscriptionShareSampleAllocation{}
	advanceUsed := false
	if !chronological {
		state, reason = SubscriptionShareStateStopped, "evidence_time_conflict"
		if snapshot.ObservedAt < generation.ObservedAt {
			reason = "evidence_time_regression"
		}
		applicationReason = reason
	} else if snapshot.Source == pool.WindowSource && snapshot.Status == "unsupported" && snapshot.ErrorCode == "window_absent" {
		state, reason = SubscriptionShareStateStopped, "window_absent"
		applicationReason = reason
	} else if !qualified {
		state, reason = SubscriptionShareStateStopped, "sample_failed"
		applicationReason = reason
	} else if !descriptorMatched || !resetEquivalent && !zeroWindowShift {
		state, reason = SubscriptionShareStateStopped, "window_changed"
		applicationReason = reason
	} else if snapshot.ObservedAt-generation.ObservedAt > pool.MaxAgeSeconds {
		state, reason = SubscriptionShareStateStopped, "long_gap"
		applicationReason = reason
		if usedMicro >= generation.LastUsedMicro {
			unattributedDelta = usedMicro - generation.LastUsedMicro
			generation.UnattributedMicro += unattributedDelta
			advanceUsed = true
		}
	} else if generation.BaselineTime == 0 && generation.State == SubscriptionShareStateAwaitingBaseline {
		generation.BaselineTime, generation.BaselineUsedMicro = snapshot.ObservedAt, usedMicro
		generation.LastUsedMicro, generation.UnattributedMicro = usedMicro, usedMicro
		state, reason = SubscriptionShareStateReady, "eligible"
		applicationReason, unattributedDelta = "baseline", usedMicro
		advanceUsed = true
	} else if generation.State != SubscriptionShareStateReady {
		state, reason = generation.State, generation.Reason
		applicationReason = "stopped_" + generation.Reason
		if qualified && usedMicro >= generation.LastUsedMicro {
			unattributedDelta = usedMicro - generation.LastUsedMicro
			generation.UnattributedMicro += unattributedDelta
			advanceUsed = true
		}
	} else if usedMicro < generation.LastUsedMicro {
		state, reason = SubscriptionShareStateStopped, "anomalous_decrease"
		applicationReason = reason
	} else if usedMicro > generation.LastUsedMicro {
		delta := usedMicro - generation.LastUsedMicro
		applicationReason = "positive_delta"
		var rows []struct {
			WindowID     int64
			AllocationID int64
		}
		if err := tx.Table(windowTable+" AS w").Select("w.id AS window_id, w.allocation_id").Joins("JOIN "+attemptTable+" AS a ON a.attempt_id = w.attempt_id").Where("w.pool_id = ? AND w.generation = ? AND w.allocated_at = 0 AND a.state <> ? AND a.created_at < ?", pool.ID, pool.CurrentGeneration, SubscriptionShareAttemptNotSent, snapshot.ObservedAt).Order("w.id ASC").Scan(&rows).Error; err != nil {
			return err
		}
		counts := make(map[int64]int64)
		for _, row := range rows {
			counts[row.AllocationID]++
		}
		attributed := int64(0)
		if len(rows) > 0 {
			total := int64(len(rows))
			for allocationID, count := range counts {
				share, err := subscriptionShareProportionalMicro(delta, count, total)
				if err != nil {
					return err
				}
				allocationEvidence = append(allocationEvidence, subscriptionShareSampleAllocation{AllocationID: allocationID, AttemptCount: count, AllocatedMicro: share})
				if share == 0 {
					continue
				}
				result := tx.Model(&SubscriptionShareAllocationUsage{}).Where("allocation_id = ? AND generation = ? AND estimated_used_micro >= 0", allocationID, pool.CurrentGeneration).Update("estimated_used_micro", gorm.Expr("estimated_used_micro + ?", share))
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return ErrSubscriptionShareConflict
				}
				attributed += share
			}
			ids := make([]int64, 0, len(rows))
			for _, row := range rows {
				ids = append(ids, row.WindowID)
			}
			allocatedAt := snapshot.ObservedAt
			if err := updateSubscriptionShareAttemptWindows(tx, ids, snapshot.ObservedAt, &allocatedAt); err != nil {
				return err
			}
		}
		unattributedDelta = delta - attributed
		generation.UnattributedMicro += unattributedDelta
		advanceUsed = true
	} else {
		advanceUsed = true
	}
	if qualified && generation.BaselineTime > 0 && advanceUsed {
		generation.LastUsedMicro = usedMicro
	}
	if zeroWindowShift {
		generation.ResetAt = snapshot.ResetAt
	}
	if state == SubscriptionShareStateStopped && generation.State != SubscriptionShareStateStopped {
		generation.StoppedAt, generation.StoppedSnapshotID = snapshot.ObservedAt, snapshot.Id
	}
	generation.LastSnapshotID, generation.State, generation.Reason = snapshot.Id, state, reason
	if chronological {
		generation.ObservedAt = snapshot.ObservedAt
	}
	if err := tx.Model(&SubscriptionShareGeneration{}).Where("id = ?", generation.ID).Updates(map[string]any{"baseline_time": generation.BaselineTime, "baseline_used_micro": generation.BaselineUsedMicro, "last_used_micro": generation.LastUsedMicro, "unattributed_micro": generation.UnattributedMicro, "last_snapshot_id": generation.LastSnapshotID, "observed_at": generation.ObservedAt, "reset_at": generation.ResetAt, "state": generation.State, "reason": generation.Reason, "stopped_at": generation.StoppedAt, "stopped_snapshot_id": generation.StoppedSnapshotID}).Error; err != nil {
		return err
	}
	if chronological && qualified && descriptorMatched {
		if err := tx.Model(&SubscriptionSharePool{}).Where("id = ?", pool.ID).Update("snapshot_id", snapshot.Id).Error; err != nil {
			return err
		}
		pool.SnapshotID = snapshot.Id
	}
	if state == SubscriptionShareStateReady {
		var coverIDs []int64
		if err := tx.Table(windowTable+" AS w").Joins("JOIN "+attemptTable+" AS a ON a.attempt_id = w.attempt_id").Where("w.pool_id = ? AND w.generation = ? AND w.covered_at = 0 AND a.state <> ? AND a.created_at < ?", pool.ID, pool.CurrentGeneration, SubscriptionShareAttemptNotSent, snapshot.ObservedAt).Order("w.id ASC").Pluck("w.id", &coverIDs).Error; err != nil {
			return err
		}
		if len(allocationEvidence) == 0 {
			var covered []subscriptionShareSampleAllocation
			if err := tx.Table(windowTable+" AS w").Select("w.allocation_id, COUNT(*) AS attempt_count").Joins("JOIN "+attemptTable+" AS a ON a.attempt_id = w.attempt_id").Where("w.pool_id = ? AND w.generation = ? AND w.covered_at = 0 AND a.state <> ? AND a.created_at < ?", pool.ID, pool.CurrentGeneration, SubscriptionShareAttemptNotSent, snapshot.ObservedAt).Group("w.allocation_id").Scan(&covered).Error; err != nil {
				return err
			}
			allocationEvidence = covered
		}
		if err := updateSubscriptionShareAttemptWindows(tx, coverIDs, snapshot.ObservedAt, nil); err != nil {
			return err
		}
	}
	afterUsed, delta := beforeUsed, int64(0)
	if qualified {
		afterUsed, delta = usedMicro, usedMicro-beforeUsed
	}
	return recordSubscriptionShareSampleApplication(tx, pool.ID, generation.Generation, snapshot, fromSnapshotID, beforeUsed, afterUsed, delta, allocationEvidence, unattributedDelta, applicationReason)
}

func recordSubscriptionShareSampleApplication(tx *gorm.DB, poolID, generation int64, snapshot ChannelQuotaSnapshot, fromSnapshotID int, beforeUsed, afterUsed, delta int64, allocations []subscriptionShareSampleAllocation, unattributedDelta int64, reason string) error {
	sort.Slice(allocations, func(i, j int) bool { return allocations[i].AllocationID < allocations[j].AllocationID })
	encoded, err := common.Marshal(allocations)
	if err != nil {
		return err
	}
	record := SubscriptionShareSampleApplication{PoolID: poolID, Generation: generation, SnapshotID: snapshot.Id, FromSnapshotID: fromSnapshotID, BeforeUsedMicro: beforeUsed, AfterUsedMicro: afterUsed, DeltaMicro: delta, AllocationJSON: string(encoded), UnattributedDeltaMicro: unattributedDelta, Reason: reason, CreatedAt: snapshot.ObservedAt}
	for _, allocation := range allocations {
		record.AttemptCount += allocation.AttemptCount
	}
	result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "pool_id"}, {Name: "generation"}, {Name: "snapshot_id"}}, DoNothing: true}).Create(&record)
	return result.Error
}

func ReadSubscriptionShares(ctx context.Context, db *gorm.DB, actorID, tokenID int, now int64) (*SubscriptionShareView, error) {
	if db == nil || actorID <= 0 || tokenID <= 0 || now <= 0 {
		if db == nil {
			return nil, gorm.ErrInvalidDB
		}
		return nil, ErrSubscriptionShareInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var actor User
	if err := db.WithContext(ctx).First(&actor, actorID).Error; err != nil {
		return nil, err
	}
	if actor.Status != common.UserStatusEnabled {
		return nil, ErrSubscriptionShareDenied
	}
	var token Token
	if err := db.WithContext(ctx).Unscoped().First(&token, tokenID).Error; err != nil {
		return nil, err
	}
	if token.DeletedAt.Valid || actor.Role != common.RoleRootUser && token.UserId != actorID {
		return nil, gorm.ErrRecordNotFound
	}
	var allocations []SubscriptionShareAllocation
	query := db.WithContext(ctx).Where("token_id = ?", tokenID)
	if actor.Role != common.RoleRootUser {
		query = query.Where("user_id = ?", actorID)
	}
	if err := query.Order("pool_id ASC").Find(&allocations).Error; err != nil {
		return nil, err
	}
	poolIDs := make([]int64, 0, len(allocations))
	for _, allocation := range allocations {
		poolIDs = append(poolIDs, allocation.PoolID)
	}
	var pools []SubscriptionSharePool
	if len(poolIDs) != 0 {
		if err := db.WithContext(ctx).Where("id IN ?", poolIDs).Order("id ASC").Find(&pools).Error; err != nil {
			return nil, err
		}
	}
	seenSubjects := map[string]bool{}
	for _, pool := range pools {
		if !seenSubjects[pool.SubjectRef] {
			if err := RefreshSubscriptionShareProjection(ctx, db, pool.SubjectRef, now); err != nil {
				return nil, err
			}
			seenSubjects[pool.SubjectRef] = true
		}
	}
	view := &SubscriptionShareView{TokenID: tokenID, Pools: []SubscriptionSharePoolView{}}
	if actor.Role == common.RoleRootUser {
		var snapshots []ChannelQuotaSnapshot
		if err := db.WithContext(ctx).Where("identity_quality = ? AND metric_type = ? AND source IN ? AND observed_at BETWEEN ? AND ?", ChannelQuotaIdentityQualityProviderConfirmed, "codex_rate_limit", []string{codexQuotaWindowPrimarySource, codexQuotaWindowSecondarySource}, now-SubscriptionShareDefaultMaxAge, now).Order("observed_at DESC, id DESC").Find(&snapshots).Error; err != nil {
			return nil, err
		}
		var knownPools []SubscriptionSharePool
		if err := db.WithContext(ctx).Find(&knownPools).Error; err != nil {
			return nil, err
		}
		byDescriptor := make(map[string]SubscriptionSharePool, len(knownPools))
		bySlot := make(map[string]SubscriptionSharePool, len(knownPools))
		for _, pool := range knownPools {
			byDescriptor[pool.DescriptorKey] = pool
			if pool.Enabled {
				bySlot[pool.SubjectRef+"\x00"+pool.WindowSource] = pool
			}
		}
		channelNames := map[int]string{}
		seenDescriptors := map[string]bool{}
		allocatedPools := make(map[int64]bool, len(poolIDs))
		tokenShareByPool := make(map[int64]int, len(allocations))
		for _, allocation := range allocations {
			tokenShareByPool[allocation.PoolID] = allocation.ShareBPS
		}
		for _, poolID := range poolIDs {
			allocatedPools[poolID] = true
		}
		for _, snapshot := range snapshots {
			descriptor := subscriptionShareDescriptorKey(snapshot.SubjectRef, snapshot.Source, snapshot.WindowType, snapshot.WindowSeconds, snapshot.PlanType)
			if seenDescriptors[descriptor] {
				continue
			}
			seenDescriptors[descriptor] = true
			if !validSubscriptionShareSnapshot(snapshot, now) {
				continue
			}
			if _, err := subscriptionShareUsedMicro(snapshot); err != nil {
				continue
			}
			name, ok := channelNames[snapshot.ChannelId]
			if !ok {
				var channel Channel
				if err := db.WithContext(ctx).Select("id", "name").First(&channel, snapshot.ChannelId).Error; err != nil {
					continue
				}
				name, channelNames[snapshot.ChannelId] = channel.Name, channel.Name
			}
			candidate := SubscriptionSharePoolView{SnapshotID: snapshot.Id, ChannelName: name, WindowSource: snapshot.Source, WindowSlot: strings.TrimPrefix(snapshot.Source, "codex_wham_usage_"), WindowType: snapshot.WindowType, WindowSeconds: snapshot.WindowSeconds, PlanType: snapshot.PlanType, ResetAt: snapshot.ResetAt, ObservedAt: snapshot.ObservedAt, State: "candidate", Reason: "fresh_qualified_sample", Approximate: true, MaxAgeSeconds: SubscriptionShareDefaultMaxAge, SampleAgeSeconds: now - snapshot.ObservedAt}
			if pool, exists := byDescriptor[descriptor]; exists {
				if !pool.Enabled {
					continue
				}
				if allocatedPools[pool.ID] {
					continue
				}
				candidate.PoolID, candidate.Revision = pool.ID, pool.Revision
				if err := db.WithContext(ctx).Model(&SubscriptionShareAllocation{}).Where("pool_id = ?", pool.ID).Select("COALESCE(SUM(share_bps),0)").Scan(&candidate.AllocatedBPS).Error; err != nil {
					return nil, err
				}
			} else if pool, exists := bySlot[snapshot.SubjectRef+"\x00"+snapshot.Source]; exists {
				candidate.PoolID, candidate.Revision, candidate.ReplacementRequired, candidate.ShareBPS = pool.ID, pool.Revision, true, tokenShareByPool[pool.ID]
				if err := db.WithContext(ctx).Model(&SubscriptionShareAllocation{}).Where("pool_id = ?", pool.ID).Select("COALESCE(SUM(share_bps),0)").Scan(&candidate.AllocatedBPS).Error; err != nil {
					return nil, err
				}
			}
			view.Pools = append(view.Pools, candidate)
		}
	}
	allocationByPool := make(map[int64]SubscriptionShareAllocation, len(allocations))
	for _, allocation := range allocations {
		allocationByPool[allocation.PoolID] = allocation
	}
	if len(poolIDs) != 0 {
		if err := db.WithContext(ctx).Where("id IN ?", poolIDs).Order("id ASC").Find(&pools).Error; err != nil {
			return nil, err
		}
	}
	windowTable, attemptTable := subscriptionShareAttemptTables(db)
	for _, pool := range pools {
		allocation := allocationByPool[pool.ID]
		channelName := pool.ChannelName
		if actor.Role != common.RoleRootUser {
			channelName = ""
		}
		item := SubscriptionSharePoolView{PoolID: pool.ID, SnapshotID: pool.SnapshotID, Revision: pool.Revision, ChannelName: channelName, WindowSource: pool.WindowSource, WindowSlot: strings.TrimPrefix(pool.WindowSource, "codex_wham_usage_"), WindowType: pool.WindowType, WindowSeconds: pool.WindowSeconds, PlanType: pool.PlanType, ShareBPS: allocation.ShareBPS, Approximate: true, State: "baseline_pending", Reason: "baseline_required", MaxAgeSeconds: pool.MaxAgeSeconds}
		if err := db.WithContext(ctx).Model(&SubscriptionShareAllocation{}).Where("pool_id = ?", pool.ID).Select("COALESCE(SUM(share_bps),0)").Scan(&item.AllocatedBPS).Error; err != nil {
			return nil, err
		}
		if pool.CurrentGeneration > 0 {
			var generation SubscriptionShareGeneration
			if err := db.WithContext(ctx).Where("pool_id = ? AND generation = ?", pool.ID, pool.CurrentGeneration).Take(&generation).Error; err != nil {
				return nil, err
			}
			item.ResetAt, item.ObservedAt, item.Reason, item.UnattributedMicro, item.BaselineTime = generation.ResetAt, generation.ObservedAt, generation.Reason, generation.UnattributedMicro, generation.BaselineTime
			if generation.ObservedAt > 0 && generation.ObservedAt <= now {
				item.SampleAgeSeconds = now - generation.ObservedAt
			}
			switch {
			case !pool.Enabled:
				item.State, item.Reason = "closed", "pool_closed"
			case generation.BaselineTime == 0 && generation.State == SubscriptionShareStateAwaitingBaseline:
				item.State = "baseline_pending"
			case generation.State == SubscriptionShareStateStopped:
				item.State = "discontinuity"
				var latest ChannelQuotaSnapshot
				if err := db.WithContext(ctx).First(&latest, pool.SnapshotID).Error; err == nil && latest.Id == generation.LastSnapshotID && validSubscriptionShareSnapshot(latest, now) && subscriptionShareDescriptorKey(latest.SubjectRef, latest.Source, latest.WindowType, latest.WindowSeconds, latest.PlanType) == pool.DescriptorKey && latest.ObservedAt > generation.StoppedAt && latest.ResetAt >= generation.ResetAt-2 && latest.ResetAt <= generation.ResetAt+2 {
					if latestUsed, usedErr := subscriptionShareUsedMicro(latest); usedErr == nil && latestUsed >= generation.LastUsedMicro {
						item.SnapshotID, item.ObservedAt, item.ResetAt, item.SampleAgeSeconds, item.RebaselineRequired = latest.Id, latest.ObservedAt, latest.ResetAt, now-latest.ObservedAt, true
					}
				}
			case generation.ResetAt <= now:
				item.State, item.Reason = "reset_pending", "reset_sample_required"
			case generation.ObservedAt > now || now-generation.ObservedAt > pool.MaxAgeSeconds:
				item.State, item.Reason = "stale", "stale"
			default:
				item.State = "active"
			}
			var usage SubscriptionShareAllocationUsage
			err := db.WithContext(ctx).Where("allocation_id = ? AND generation = ?", allocation.ID, pool.CurrentGeneration).Take(&usage).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrSubscriptionShareUnavailable
			}
			if err != nil {
				return nil, err
			}
			item.EstimatedUsedMicro = usage.EstimatedUsedMicro
			base := db.WithContext(ctx).Table(windowTable+" AS w").Joins("JOIN "+attemptTable+" AS a ON a.attempt_id = w.attempt_id").Where("w.allocation_id = ? AND w.generation = ? AND w.allocated_at = 0 AND a.state <> ?", allocation.ID, pool.CurrentGeneration, SubscriptionShareAttemptNotSent)
			if err := base.Count(&item.PendingAttempts).Error; err != nil {
				return nil, err
			}
			if err := db.WithContext(ctx).Table(windowTable+" AS w").Joins("JOIN "+attemptTable+" AS a ON a.attempt_id = w.attempt_id").Where("w.allocation_id = ? AND w.generation = ? AND w.covered_at = 0 AND a.state <> ?", allocation.ID, pool.CurrentGeneration, SubscriptionShareAttemptNotSent).Count(&item.UnsampledAttempts).Error; err != nil {
				return nil, err
			}
			if err := db.WithContext(ctx).Model(&SubscriptionShareAttemptWindow{}).Where("allocation_id = ? AND allocated_at = ?", allocation.ID, -1).Count(&item.UncertainAttempts).Error; err != nil {
				return nil, err
			}
		}
		view.Pools = append(view.Pools, item)
	}
	return view, nil
}
