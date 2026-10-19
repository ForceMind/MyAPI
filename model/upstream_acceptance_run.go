package model

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	UpstreamAcceptanceRunActive   = "active"
	UpstreamAcceptanceRunStopping = "stopping"
	UpstreamAcceptanceRunStopped  = "stopped"
	UpstreamAcceptanceRunExpired  = "expired"

	UpstreamAcceptanceDispatchAuth       = "auth"
	UpstreamAcceptanceDispatchRefresh    = "refresh"
	UpstreamAcceptanceDispatchSample     = "sample"
	UpstreamAcceptanceDispatchModels     = "models"
	UpstreamAcceptanceDispatchGeneration = "generation"

	// This storage bound prevents counter overflow; it is not a provider limit.
	UpstreamAcceptanceMaxCounter = int64(1_000_000)
)

var (
	ErrUpstreamAcceptanceInvalid  = errors.New("invalid upstream acceptance run")
	ErrUpstreamAcceptanceNotFound = errors.New("upstream acceptance run not found")
	ErrUpstreamAcceptanceDenied   = errors.New("upstream acceptance dispatch denied")
	ErrUpstreamAcceptanceStorage  = errors.New("upstream acceptance storage unavailable")
)

// UpstreamAcceptanceRun contains only opaque scope, bounded counters and run
// state. Credentials, email addresses, URLs and response payloads do not belong
// in this table. OwnerID fences a run to the process which explicitly started it.
type UpstreamAcceptanceRun struct {
	ID                 string `json:"id" gorm:"type:varchar(64);primaryKey"`
	OwnerID            string `json:"-" gorm:"type:varchar(64);not null"`
	ChannelID          int    `json:"channel_id" gorm:"not null"`
	AccountRef         string `json:"account_ref" gorm:"type:varchar(128);not null"`
	Status             string `json:"status" gorm:"type:varchar(16);not null"`
	DeadlineAtUnix     int64  `json:"deadline_at_unix" gorm:"not null"`
	MaxTotal           int64  `json:"max_total" gorm:"not null"`
	MaxAuth            int64  `json:"max_auth" gorm:"not null"`
	MaxRefresh         int64  `json:"max_refresh" gorm:"not null"`
	MaxSample          int64  `json:"max_sample" gorm:"not null"`
	MaxModels          int64  `json:"max_models" gorm:"not null"`
	MaxGeneration      int64  `json:"max_generation" gorm:"not null"`
	Total              int64  `json:"total" gorm:"not null"`
	Auth               int64  `json:"auth" gorm:"not null"`
	Refresh            int64  `json:"refresh" gorm:"not null"`
	Sample             int64  `json:"sample" gorm:"not null"`
	Models             int64  `json:"models" gorm:"not null"`
	Generation         int64  `json:"generation" gorm:"not null"`
	GenerationInFlight int64  `json:"generation_in_flight" gorm:"not null"`
	CreatedAtUnix      int64  `json:"created_at_unix" gorm:"not null"`
	UpdatedAtUnix      int64  `json:"updated_at_unix" gorm:"not null"`
	LastDispatchAtUnix int64  `json:"last_dispatch_at_unix" gorm:"not null"`
	StopReason         string `json:"stop_reason,omitempty" gorm:"type:varchar(32);not null"`
}

// acceptanceOpaqueReference excludes arbitrary diagnostics and obvious raw
// credentials/URLs from identifiers. The service derives the account reference.
func acceptanceOpaqueReference(value string, limit int) bool {
	if len(value) == 0 || len(value) > limit {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == ':') {
			return false
		}
	}
	return true
}

func CreateUpstreamAcceptanceRun(ctx context.Context, run *UpstreamAcceptanceRun) error {
	if run == nil || !acceptanceOpaqueReference(run.ID, 64) || !acceptanceOpaqueReference(run.OwnerID, 64) ||
		!acceptanceOpaqueReference(run.AccountRef, 128) || run.ChannelID <= 0 || run.MaxTotal <= 0 || run.MaxTotal > UpstreamAcceptanceMaxCounter {
		return ErrUpstreamAcceptanceInvalid
	}
	for _, limit := range []int64{run.MaxAuth, run.MaxRefresh, run.MaxSample, run.MaxModels, run.MaxGeneration} {
		if limit < 0 || limit > run.MaxTotal {
			return ErrUpstreamAcceptanceInvalid
		}
	}
	if run.Total != 0 || run.Auth != 0 || run.Refresh != 0 || run.Sample != 0 || run.Models != 0 || run.Generation != 0 ||
		run.GenerationInFlight != 0 || run.LastDispatchAtUnix != 0 || run.StopReason != "" ||
		(run.Status != "" && run.Status != UpstreamAcceptanceRunActive) {
		return ErrUpstreamAcceptanceInvalid
	}
	if run.CreatedAtUnix == 0 {
		run.CreatedAtUnix = time.Now().Unix()
	}
	if run.CreatedAtUnix <= 0 || run.DeadlineAtUnix <= run.CreatedAtUnix {
		return ErrUpstreamAcceptanceInvalid
	}
	run.Status = UpstreamAcceptanceRunActive
	run.UpdatedAtUnix = run.CreatedAtUnix
	if DB == nil || DB.WithContext(ctx).Create(run).Error != nil {
		return ErrUpstreamAcceptanceStorage
	}
	return nil
}

func GetUpstreamAcceptanceRun(ctx context.Context, id string) (*UpstreamAcceptanceRun, error) {
	if !acceptanceOpaqueReference(id, 64) {
		return nil, ErrUpstreamAcceptanceInvalid
	}
	if DB == nil {
		return nil, ErrUpstreamAcceptanceStorage
	}
	var run UpstreamAcceptanceRun
	err := DB.WithContext(ctx).Where("id = ?", id).First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUpstreamAcceptanceNotFound
	}
	if err != nil {
		return nil, ErrUpstreamAcceptanceStorage
	}
	return &run, nil
}

// GetUpstreamAcceptanceChannel reads the current credential with the caller's
// finite gate budget; cached or unbounded channel reads cannot authorize a run.
func GetUpstreamAcceptanceChannel(ctx context.Context, channelID int) (*Channel, error) {
	if DB == nil || channelID <= 0 {
		return nil, ErrUpstreamAcceptanceStorage
	}
	var channel Channel
	if DB.WithContext(ctx).Where("id = ?", channelID).First(&channel).Error != nil {
		return nil, ErrUpstreamAcceptanceStorage
	}
	return &channel, nil
}

// ClaimUpstreamAcceptanceDispatch accounts for one physical outbound attempt.
// Failure and retry attempts retain their counts. A single conditional UPDATE
// serializes competing claims on all three databases, including SQLite, without
// a read-then-write race. The transaction returns the claimed snapshot.
func ClaimUpstreamAcceptanceDispatch(ctx context.Context, id, ownerID string, channelID int, accountRef, kind string, now int64) (*UpstreamAcceptanceRun, error) {
	if !acceptanceOpaqueReference(id, 64) || !acceptanceOpaqueReference(ownerID, 64) || channelID <= 0 || !acceptanceOpaqueReference(accountRef, 128) || now <= 0 {
		return nil, ErrUpstreamAcceptanceInvalid
	}
	var column string
	switch kind {
	case UpstreamAcceptanceDispatchAuth, UpstreamAcceptanceDispatchRefresh, UpstreamAcceptanceDispatchSample, UpstreamAcceptanceDispatchModels, UpstreamAcceptanceDispatchGeneration:
		column = kind
	default:
		return nil, ErrUpstreamAcceptanceInvalid
	}
	if DB == nil {
		return nil, ErrUpstreamAcceptanceStorage
	}
	var run UpstreamAcceptanceRun
	denied := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Persist expiry even when the attempted dispatch is rejected. No API
		// transitions an expired run back to active, including after a restart.
		expired := tx.Model(&UpstreamAcceptanceRun{}).
			Where("id = ? AND owner_id = ? AND channel_id = ? AND account_ref = ? AND status = ? AND deadline_at_unix <= ?", id, ownerID, channelID, accountRef, UpstreamAcceptanceRunActive, now).
			Updates(map[string]any{"status": UpstreamAcceptanceRunExpired, "stop_reason": "deadline", "updated_at_unix": now})
		if expired.Error != nil {
			return expired.Error
		}
		query := tx.Model(&UpstreamAcceptanceRun{}).
			Where("id = ? AND owner_id = ? AND channel_id = ? AND account_ref = ? AND status = ? AND deadline_at_unix > ?", id, ownerID, channelID, accountRef, UpstreamAcceptanceRunActive, now).
			Where("total >= 0 AND total < max_total AND max_total <= ?", UpstreamAcceptanceMaxCounter).
			Where(column + " >= 0 AND " + column + " < max_" + column)
		updates := map[string]any{
			"total": gorm.Expr("total + 1"), column: gorm.Expr(column + " + 1"),
			"updated_at_unix": now, "last_dispatch_at_unix": now,
		}
		if kind == UpstreamAcceptanceDispatchGeneration {
			query = query.Where("generation_in_flight = 0")
			updates["generation_in_flight"] = int64(1)
		}
		result := query.Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			denied = true
			return nil
		}
		return tx.Where("id = ?", id).First(&run).Error
	})
	if err != nil {
		return nil, ErrUpstreamAcceptanceStorage
	}
	if denied {
		return nil, ErrUpstreamAcceptanceDenied
	}
	return &run, nil
}

// FinishUpstreamAcceptanceGeneration releases only the in-flight slot, never a
// charged attempt. The caller must release each claim once, including body close.
func FinishUpstreamAcceptanceGeneration(ctx context.Context, id, ownerID string, now int64) error {
	if !acceptanceOpaqueReference(id, 64) || !acceptanceOpaqueReference(ownerID, 64) || now <= 0 {
		return ErrUpstreamAcceptanceInvalid
	}
	if DB == nil {
		return ErrUpstreamAcceptanceStorage
	}
	result := DB.WithContext(ctx).Model(&UpstreamAcceptanceRun{}).
		Where("id = ? AND owner_id = ? AND generation_in_flight = 1", id, ownerID).
		Updates(map[string]any{"generation_in_flight": int64(0), "updated_at_unix": now})
	if result.Error != nil {
		return ErrUpstreamAcceptanceStorage
	}
	if result.RowsAffected != 1 {
		return ErrUpstreamAcceptanceDenied
	}
	return nil
}

// StopUpstreamAcceptanceRun latches the dispatch gate before the service cancels
// and drains operations. The first stop reason is retained on repeated calls.
func StopUpstreamAcceptanceRun(ctx context.Context, id, ownerID string, now int64, reason string) (*UpstreamAcceptanceRun, error) {
	if !acceptanceOpaqueReference(id, 64) || !acceptanceOpaqueReference(ownerID, 64) || now <= 0 {
		return nil, ErrUpstreamAcceptanceInvalid
	}
	switch reason {
	case "manual", "deadline", "budget", "shutdown", "storage", "scope", "dispatch_error":
	default:
		return nil, ErrUpstreamAcceptanceInvalid
	}
	if DB == nil {
		return nil, ErrUpstreamAcceptanceStorage
	}
	var run UpstreamAcceptanceRun
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		status := UpstreamAcceptanceRunStopping
		if reason == "deadline" {
			status = UpstreamAcceptanceRunExpired
		}
		result := tx.Model(&UpstreamAcceptanceRun{}).
			Where("id = ? AND owner_id = ? AND status = ?", id, ownerID, UpstreamAcceptanceRunActive).
			Updates(map[string]any{"status": status, "stop_reason": reason, "updated_at_unix": now})
		if result.Error != nil {
			return result.Error
		}
		return tx.Where("id = ? AND owner_id = ?", id, ownerID).First(&run).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUpstreamAcceptanceDenied
	}
	if err != nil {
		return nil, ErrUpstreamAcceptanceStorage
	}
	return &run, nil
}

// CompleteUpstreamAcceptanceStop requires service evidence that all operations
// and response bodies have drained. The database additionally rejects completion
// while a generation slot is held. Expired state remains terminal and distinct.
func CompleteUpstreamAcceptanceStop(ctx context.Context, id, ownerID string, now int64) (*UpstreamAcceptanceRun, error) {
	if !acceptanceOpaqueReference(id, 64) || !acceptanceOpaqueReference(ownerID, 64) || now <= 0 {
		return nil, ErrUpstreamAcceptanceInvalid
	}
	if DB == nil {
		return nil, ErrUpstreamAcceptanceStorage
	}
	var run UpstreamAcceptanceRun
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&UpstreamAcceptanceRun{}).
			Where("id = ? AND owner_id = ? AND status = ? AND generation_in_flight = 0", id, ownerID, UpstreamAcceptanceRunStopping).
			Updates(map[string]any{"status": UpstreamAcceptanceRunStopped, "updated_at_unix": now})
		if result.Error != nil {
			return result.Error
		}
		return tx.Where("id = ? AND owner_id = ? AND status IN ? AND generation_in_flight = 0", id, ownerID, []string{UpstreamAcceptanceRunStopped, UpstreamAcceptanceRunExpired}).First(&run).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUpstreamAcceptanceDenied
	}
	if err != nil {
		return nil, ErrUpstreamAcceptanceStorage
	}
	return &run, nil
}
