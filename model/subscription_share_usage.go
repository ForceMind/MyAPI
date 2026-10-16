package model

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

// SubscriptionSharePoolUsage is a Root-only read model. Percent amounts use
// millionths of one percentage point, independently of token or money quotas.
type SubscriptionSharePoolUsage struct {
	PoolID                   int64                             `json:"pool_id"`
	SnapshotID               int                               `json:"snapshot_id"`
	Revision                 int64                             `json:"revision"`
	ChannelID                int                               `json:"channel_id"`
	ChannelName              string                            `json:"channel_name"`
	WindowSlot               string                            `json:"window_slot"`
	WindowType               string                            `json:"window_type"`
	WindowSeconds            int64                             `json:"window_seconds"`
	PlanType                 string                            `json:"plan_type"`
	Generation               int64                             `json:"generation"`
	State                    string                            `json:"state"`
	Reason                   string                            `json:"reason"`
	ObservedAt               int64                             `json:"observed_at"`
	ResetAt                  int64                             `json:"reset_at"`
	MaxAgeSeconds            int64                             `json:"max_age_seconds"`
	SampleAgeSeconds         int64                             `json:"sample_age_seconds"`
	AllocatedBPS             int                               `json:"allocated_bps"`
	Approximate              bool                              `json:"approximate"`
	BaselineTime             int64                             `json:"baseline_time"`
	BaselineUsedMicro        *int64                            `json:"baseline_used_micro"`
	InitialBaselineTime      *int64                            `json:"initial_baseline_time"`
	InitialBaselineUsedMicro *int64                            `json:"initial_baseline_used_micro"`
	ObservedUsedMicro        *int64                            `json:"observed_used_micro"`
	EstimatedUsedMicro       *int64                            `json:"estimated_used_micro"`
	UnattributedMicro        *int64                            `json:"unattributed_micro"`
	UnattributedGrowthMicro  *int64                            `json:"unattributed_growth_micro"`
	DecompositionState       string                            `json:"decomposition_state"`
	PendingAttempts          int64                             `json:"pending_attempts"`
	UnsampledAttempts        int64                             `json:"unsampled_attempts"`
	UncertainAttempts        int64                             `json:"uncertain_attempts"`
	Allocations              []SubscriptionShareAllocationView `json:"allocations"`
}

type SubscriptionShareAllocationView struct {
	TokenID            int     `json:"token_id"`
	TokenName          *string `json:"token_name"`
	TokenState         string  `json:"token_state"`
	ShareBPS           int     `json:"share_bps"`
	EstimatedUsedMicro *int64  `json:"estimated_used_micro"`
	PendingAttempts    int64   `json:"pending_attempts"`
	UnsampledAttempts  int64   `json:"unsampled_attempts"`
	UncertainAttempts  int64   `json:"uncertain_attempts"`
}

// ReadSubscriptionSharePoolUsage never advances projections or repairs missing
// evidence. A repeatable read snapshot keeps configuration, generation, usage,
// and request counts together without blocking admission on an account gate.
func ReadSubscriptionSharePoolUsage(ctx context.Context, db *gorm.DB, actorID int, poolID, now int64) (*SubscriptionSharePoolUsage, error) {
	if db == nil || db.Dialector == nil {
		return nil, gorm.ErrInvalidDB
	}
	if actorID <= 0 || poolID <= 0 || now <= 0 {
		return nil, ErrSubscriptionShareInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	options := &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
	if db.Dialector.Name() == "sqlite" {
		// SQLite's deferred read transaction holds one snapshot after its first
		// SELECT; the driver does not implement isolation-level selection.
		options.Isolation = sql.LevelDefault
	}
	var view SubscriptionSharePoolUsage
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var actor User
		if err := tx.Select("id", "role", "status").First(&actor, actorID).Error; err != nil {
			return err
		}
		if actor.Role != common.RoleRootUser || actor.Status != common.UserStatusEnabled {
			return ErrSubscriptionShareDenied
		}
		var pool SubscriptionSharePool
		if err := tx.First(&pool, poolID).Error; err != nil {
			return err
		}
		if pool.WindowSource != codexQuotaWindowPrimarySource && pool.WindowSource != codexQuotaWindowSecondarySource {
			return ErrSubscriptionShareUnavailable
		}
		view = SubscriptionSharePoolUsage{
			PoolID: pool.ID, SnapshotID: pool.SnapshotID, Revision: pool.Revision,
			ChannelID: pool.ChannelID, ChannelName: pool.ChannelName,
			WindowSlot: "primary", WindowType: pool.WindowType, WindowSeconds: pool.WindowSeconds, PlanType: pool.PlanType,
			Generation: pool.CurrentGeneration, MaxAgeSeconds: pool.MaxAgeSeconds,
			Approximate: true, State: "baseline_pending", Reason: "baseline_required",
			DecompositionState: "baseline_pending", Allocations: []SubscriptionShareAllocationView{},
		}
		if pool.WindowSource == codexQuotaWindowSecondarySource {
			view.WindowSlot = "secondary"
		}
		var generation SubscriptionShareGeneration
		if pool.CurrentGeneration > 0 {
			if err := tx.Where("pool_id = ? AND generation = ?", pool.ID, pool.CurrentGeneration).Take(&generation).Error; err != nil {
				return err
			}
			view.SnapshotID, view.ObservedAt, view.ResetAt = generation.LastSnapshotID, generation.ObservedAt, generation.ResetAt
			view.BaselineTime, view.Reason = generation.BaselineTime, generation.Reason
			if generation.BaselineUsedMicro < 0 || generation.BaselineUsedMicro > SubscriptionShareTotalMicro || generation.LastUsedMicro < 0 || generation.LastUsedMicro > SubscriptionShareTotalMicro || generation.UnattributedMicro < 0 || generation.UnattributedMicro > SubscriptionShareTotalMicro {
				return ErrSubscriptionShareUnavailable
			}
			if generation.ObservedAt > 0 && generation.ObservedAt <= now {
				view.SampleAgeSeconds = now - generation.ObservedAt
			}
			if generation.BaselineTime > 0 {
				view.BaselineUsedMicro = &generation.BaselineUsedMicro
				view.ObservedUsedMicro, view.UnattributedMicro = &generation.LastUsedMicro, &generation.UnattributedMicro
				view.DecompositionState = "baseline_unknown"
			}
			switch {
			case generation.BaselineTime == 0:
			case generation.State == SubscriptionShareStateStopped:
				view.State = "discontinuity"
			case generation.State != SubscriptionShareStateReady:
				view.State, view.Reason = "discontinuity", "unsupported_generation_state"
			case generation.ResetAt <= now:
				view.State, view.Reason = "reset_pending", "reset_sample_required"
			case generation.ObservedAt <= 0 || generation.ObservedAt > now || now-generation.ObservedAt > pool.MaxAgeSeconds:
				view.State, view.Reason = "stale", "stale"
			default:
				view.State = "active"
			}
		}
		if !pool.Enabled {
			view.State, view.Reason = "closed", "pool_closed"
		}

		var allocations []SubscriptionShareAllocation
		if err := tx.Where("pool_id = ?", pool.ID).Order("token_id ASC").Find(&allocations).Error; err != nil {
			return err
		}
		var usages []SubscriptionShareAllocationUsage
		allocationIDs := tx.Model(&SubscriptionShareAllocation{}).Select("id").Where("pool_id = ?", pool.ID)
		if err := tx.Where("allocation_id IN (?) AND generation = ?", allocationIDs, pool.CurrentGeneration).Find(&usages).Error; err != nil {
			return err
		}
		usageByAllocation := make(map[int64]int64, len(usages))
		for _, usage := range usages {
			usageByAllocation[usage.AllocationID] = usage.EstimatedUsedMicro
		}
		var tokens []Token
		tokenIDs := tx.Model(&SubscriptionShareAllocation{}).Select("token_id").Where("pool_id = ?", pool.ID)
		if err := tx.Unscoped().Select("id", "name", "status", "expired_time", "deleted_at").Where("id IN (?)", tokenIDs).Find(&tokens).Error; err != nil {
			return err
		}
		tokenByID := make(map[int]Token, len(tokens))
		for _, token := range tokens {
			tokenByID[token.Id] = token
		}
		var counts []struct {
			AllocationID int64
			Pending      int64
			Unsampled    int64
			Uncertain    int64
		}
		windows, attempts := subscriptionShareAttemptTables(tx)
		if err := tx.Table(windows+" AS w").Select("w.allocation_id, SUM(CASE WHEN w.allocated_at = 0 THEN 1 ELSE 0 END) AS pending, SUM(CASE WHEN w.covered_at = 0 THEN 1 ELSE 0 END) AS unsampled, SUM(CASE WHEN w.allocated_at = -1 THEN 1 ELSE 0 END) AS uncertain").Joins("JOIN "+attempts+" AS a ON a.attempt_id = w.attempt_id").Where("w.pool_id = ? AND w.generation = ? AND a.state <> ?", pool.ID, pool.CurrentGeneration, SubscriptionShareAttemptNotSent).Group("w.allocation_id").Scan(&counts).Error; err != nil {
			return err
		}
		countByAllocation := make(map[int64]int, len(counts))
		for index, count := range counts {
			countByAllocation[count.AllocationID] = index
			view.PendingAttempts += count.Pending
			view.UnsampledAttempts += count.Unsampled
			view.UncertainAttempts += count.Uncertain
		}
		totalEstimate, complete := int64(0), generation.BaselineTime > 0
		for _, allocation := range allocations {
			item := SubscriptionShareAllocationView{TokenID: allocation.TokenID, TokenState: "missing", ShareBPS: allocation.ShareBPS}
			if allocation.ShareBPS < 0 || allocation.ShareBPS > 10000 || view.AllocatedBPS > 10000-allocation.ShareBPS {
				return ErrSubscriptionShareUnavailable
			}
			view.AllocatedBPS += allocation.ShareBPS
			if token, exists := tokenByID[allocation.TokenID]; exists {
				item.TokenName = &token.Name
				switch {
				case token.DeletedAt.Valid:
					item.TokenState = "deleted"
				case token.Status == common.TokenStatusDisabled:
					item.TokenState = "paused"
				case token.Status == common.TokenStatusExpired || token.ExpiredTime != -1 && token.ExpiredTime < now:
					item.TokenState = "expired"
				case token.Status == common.TokenStatusExhausted:
					item.TokenState = "exhausted"
				case token.Status == common.TokenStatusEnabled:
					item.TokenState = "enabled"
				default:
					item.TokenState = "unknown"
				}
			}
			if used, exists := usageByAllocation[allocation.ID]; exists && generation.BaselineTime > 0 && used >= 0 && used <= SubscriptionShareTotalMicro {
				item.EstimatedUsedMicro = &used
				// Every valid component is bounded by the native pool, so the
				// subtraction also prevents accumulation overflow on corrupt data.
				if totalEstimate > SubscriptionShareTotalMicro-used {
					complete = false
				} else {
					totalEstimate += used
				}
			} else {
				complete = false
			}
			if index, exists := countByAllocation[allocation.ID]; exists {
				item.PendingAttempts, item.UnsampledAttempts, item.UncertainAttempts = counts[index].Pending, counts[index].Unsampled, counts[index].Uncertain
				delete(countByAllocation, allocation.ID)
			}
			view.Allocations = append(view.Allocations, item)
		}
		if len(countByAllocation) != 0 {
			complete = false
		}
		if generation.BaselineTime == 0 {
			return nil
		}
		if complete {
			view.EstimatedUsedMicro = &totalEstimate
		}
		if !complete || generation.LastUsedMicro < 0 || generation.LastUsedMicro > SubscriptionShareTotalMicro || generation.UnattributedMicro < 0 || generation.UnattributedMicro > SubscriptionShareTotalMicro || totalEstimate != generation.LastUsedMicro-generation.UnattributedMicro {
			view.DecompositionState = "inconsistent"
			return nil
		}
		var baseline SubscriptionShareSampleApplication
		err := tx.Where("pool_id = ? AND generation = ? AND reason IN ?", pool.ID, pool.CurrentGeneration, []string{"baseline", "reset_baseline"}).Order("id ASC").Take(&baseline).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if baseline.CreatedAt <= 0 || baseline.CreatedAt > generation.ObservedAt || baseline.AfterUsedMicro < 0 || baseline.AfterUsedMicro > generation.UnattributedMicro {
			view.DecompositionState = "inconsistent"
			return nil
		}
		growth := generation.UnattributedMicro - baseline.AfterUsedMicro
		view.InitialBaselineTime, view.InitialBaselineUsedMicro = &baseline.CreatedAt, &baseline.AfterUsedMicro
		view.UnattributedGrowthMicro, view.DecompositionState = &growth, "available"
		return nil
	}, options)
	if err != nil {
		return nil, err
	}
	return &view, nil
}
