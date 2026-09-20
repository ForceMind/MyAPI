package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	channelQuotaAlertEventStatePending     = "pending"
	channelQuotaAlertEventStateClaimed     = "claimed"
	channelQuotaAlertEventStateRetryable   = "retryable"
	channelQuotaAlertEventStateDelivered   = "delivered"
	channelQuotaAlertEventStateQuarantined = "quarantined"
	channelQuotaAlertMaxAttempts           = 8
)

// ChannelQuotaAlertState is the current trusted status for one immutable
// channel-account quota series. SeriesKey is a digest so account references do
// not leak through indexes or future admin list endpoints.
type ChannelQuotaAlertState struct {
	ID              int64  `gorm:"primaryKey"`
	SeriesKey       string `gorm:"type:char(64);not null;uniqueIndex"`
	ChannelID       int    `gorm:"not null;index"`
	AccountRef      string `gorm:"type:char(64);not null;default:''"`
	MetricType      string `gorm:"type:varchar(32);not null"`
	WindowType      string `gorm:"type:varchar(32);not null"`
	Source          string `gorm:"type:varchar(64);not null"`
	PlanType        string `gorm:"type:varchar(32);not null;default:''"`
	Unit            string `gorm:"type:varchar(32);not null"`
	Currency        string `gorm:"type:varchar(8);not null;default:''"`
	WindowSeconds   int64  `gorm:"type:bigint;not null;default:0"`
	CurrentStatus   string `gorm:"type:varchar(16);not null"`
	ObservedAt      int64  `gorm:"type:bigint;not null"`
	LastDeliveredAt int64  `gorm:"type:bigint;not null;default:0"`
	CreatedAt       int64  `gorm:"type:bigint;not null"`
	UpdatedAt       int64  `gorm:"type:bigint;not null"`
}

// ChannelQuotaAlertEvent is an immutable transactional-outbox event. Its
// delivery fields are advanced only by the bounded CAS helpers below. EventKey
// is stable across retries and is the receiver's at-least-once deduplication ID.
type ChannelQuotaAlertEvent struct {
	ID            int64  `gorm:"primaryKey"`
	EventKey      string `gorm:"type:varchar(128);not null;uniqueIndex"`
	SeriesKey     string `gorm:"type:char(64);not null;index"`
	SnapshotID    int    `gorm:"not null;index"`
	ChannelID     int    `gorm:"not null;index"`
	Status        string `gorm:"type:varchar(16);not null"`
	Kind          string `gorm:"type:varchar(16);not null"`
	State         string `gorm:"type:varchar(24);not null;index:idx_channel_quota_alert_ready,priority:1"`
	ClaimedBy     string `gorm:"type:varchar(128);not null;default:''"`
	ClaimedUntil  int64  `gorm:"type:bigint;not null;default:0;index"`
	AttemptCount  int    `gorm:"not null;default:0"`
	NextAttemptAt int64  `gorm:"type:bigint;not null;default:0;index:idx_channel_quota_alert_ready,priority:2"`
	LastErrorCode string `gorm:"type:varchar(64);not null;default:''"`
	LastErrorAt   int64  `gorm:"type:bigint;not null;default:0"`
	DeliveredAt   *int64 `gorm:"type:bigint"`
	LockVersion   int64  `gorm:"type:bigint;not null;default:1"`
	ObservedAt    int64  `gorm:"type:bigint;not null;index"`
	CreatedAt     int64  `gorm:"type:bigint;not null"`
	UpdatedAt     int64  `gorm:"type:bigint;not null;default:0"`
}

type ChannelQuotaAlertEventFilter struct {
	State     string
	Status    string
	Kind      string
	ChannelID int
}

func ValidChannelQuotaAlertEventFilter(filter ChannelQuotaAlertEventFilter) bool {
	if filter.ChannelID < 0 {
		return false
	}
	if filter.State != "" && filter.State != channelQuotaAlertEventStatePending &&
		filter.State != channelQuotaAlertEventStateClaimed && filter.State != channelQuotaAlertEventStateRetryable &&
		filter.State != channelQuotaAlertEventStateDelivered && filter.State != channelQuotaAlertEventStateQuarantined {
		return false
	}
	if filter.Status != "" && filter.Status != "healthy" && filter.Status != "warning" && filter.Status != "critical" {
		return false
	}
	return filter.Kind == "" || filter.Kind == "threshold" || filter.Kind == "reminder" || filter.Kind == "recovery"
}

// ListChannelQuotaAlertEvents returns delivery history without joining account
// references or upstream credentials. Filters and pagination are bounded so an
// administrator query cannot turn into an unbounded table scan response.
func ListChannelQuotaAlertEvents(ctx context.Context, filter ChannelQuotaAlertEventFilter, offset, limit int) ([]ChannelQuotaAlertEvent, int64, error) {
	if DB == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	if !ValidChannelQuotaAlertEventFilter(filter) || offset < 0 || limit <= 0 || limit > 100 {
		return nil, 0, fmt.Errorf("invalid channel quota alert event query")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := DB.WithContext(ctx).Model(&ChannelQuotaAlertEvent{})
	if filter.State != "" {
		query = query.Where("state = ?", filter.State)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Kind != "" {
		query = query.Where("kind = ?", filter.Kind)
	}
	if filter.ChannelID > 0 {
		query = query.Where("channel_id = ?", filter.ChannelID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var events []ChannelQuotaAlertEvent
	if err := query.Order("observed_at DESC, id DESC").Offset(offset).Limit(limit).Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// GetChannelQuotaAlertDeliverySnapshot loads the immutable source observation
// and re-verifies its channel and series identity before it enters a webhook.
func GetChannelQuotaAlertDeliverySnapshot(ctx context.Context, event ChannelQuotaAlertEvent) (*ChannelQuotaSnapshot, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	if event.ID <= 0 || event.SnapshotID <= 0 || event.ChannelID <= 0 || event.SeriesKey == "" {
		return nil, fmt.Errorf("invalid channel quota alert event")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var snapshot ChannelQuotaSnapshot
	if err := DB.WithContext(ctx).First(&snapshot, "id = ?", event.SnapshotID).Error; err != nil {
		return nil, err
	}
	if snapshot.ChannelId != event.ChannelID || snapshot.AccountRef == "" ||
		channelQuotaAlertSeriesKey(&snapshot) != event.SeriesKey {
		return nil, fmt.Errorf("channel quota alert source identity mismatch")
	}
	return &snapshot, nil
}

func ChannelQuotaAlertFailureQuarantines(event ChannelQuotaAlertEvent) bool {
	return event.AttemptCount >= channelQuotaAlertMaxAttempts
}

func ChannelQuotaAlertMaxAttempts() int {
	return channelQuotaAlertMaxAttempts
}

func channelQuotaAlertSettings() common.ChannelQuotaAlertSettings {
	return common.ChannelQuotaAlertSettings{
		Enabled: common.ChannelQuotaAlertEnabled, WarningPercent: common.ChannelQuotaAlertWarningPercent,
		CriticalPercent: common.ChannelQuotaAlertCriticalPercent, CooldownSeconds: common.ChannelQuotaAlertCooldownSeconds,
		NotifyOnRecovery: common.ChannelQuotaAlertNotifyOnRecovery,
	}
}

func channelQuotaAlertSeriesKey(snapshot *ChannelQuotaSnapshot) string {
	parts := []string{strconv.Itoa(snapshot.ChannelId), snapshot.AccountRef, snapshot.MetricType, snapshot.WindowType, snapshot.Source, snapshot.PlanType, snapshot.Unit, snapshot.Currency, strconv.FormatInt(snapshot.WindowSeconds, 10)}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func recordChannelQuotaAlertForSnapshot(tx *gorm.DB, snapshot *ChannelQuotaSnapshot) error {
	settings := channelQuotaAlertSettings()
	if !settings.Enabled || snapshot == nil || snapshot.Id <= 0 || snapshot.Status != "success" || snapshot.AccountRef == "" {
		return nil
	}
	currentStatus := common.ChannelQuotaAlertStatus(snapshot.Available, snapshot.Total, settings)
	if currentStatus == "" {
		return nil
	}
	seriesKey := channelQuotaAlertSeriesKey(snapshot)
	var state ChannelQuotaAlertState
	err := tx.Where("series_key = ?", seriesKey).First(&state).Error
	firstObservation := err == gorm.ErrRecordNotFound
	if firstObservation {
		state = ChannelQuotaAlertState{SeriesKey: seriesKey, ChannelID: snapshot.ChannelId, AccountRef: snapshot.AccountRef, MetricType: snapshot.MetricType, WindowType: snapshot.WindowType, Source: snapshot.Source, PlanType: snapshot.PlanType, Unit: snapshot.Unit, Currency: snapshot.Currency, WindowSeconds: snapshot.WindowSeconds, CurrentStatus: currentStatus, ObservedAt: snapshot.ObservedAt, CreatedAt: snapshot.ObservedAt, UpdatedAt: snapshot.ObservedAt}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
			return err
		}
		if state.ID == 0 {
			if err := tx.Where("series_key = ?", seriesKey).First(&state).Error; err != nil {
				return err
			}
			firstObservation = false
		}
	} else if err != nil {
		return err
	}
	if !firstObservation && snapshot.ObservedAt <= state.ObservedAt {
		return nil
	}
	previousStatus := state.CurrentStatus
	if firstObservation {
		previousStatus = "healthy"
	}
	if !firstObservation && state.LastDeliveredAt == 0 && previousStatus == currentStatus {
		return tx.Model(&ChannelQuotaAlertState{}).Where("id = ? AND observed_at < ?", state.ID, snapshot.ObservedAt).Updates(map[string]any{"observed_at": snapshot.ObservedAt, "updated_at": snapshot.ObservedAt}).Error
	}
	outcome := common.EvaluateChannelQuotaAlertOccurrenceV2(common.ChannelQuotaAlertOccurrenceInput{SubjectRef: fmt.Sprintf("channel:%d", snapshot.ChannelId), SourceSnapshotRef: fmt.Sprintf("snapshot:%d", snapshot.Id), PreviousStatus: previousStatus, CurrentStatus: currentStatus, ObservedAt: snapshot.ObservedAt, LastDeliveredAt: state.LastDeliveredAt, SourceTrusted: true, HasProviderTotal: true}, settings)
	if outcome.EventKey != "" {
		event := ChannelQuotaAlertEvent{EventKey: outcome.EventKey, SeriesKey: seriesKey, SnapshotID: snapshot.Id, ChannelID: snapshot.ChannelId, Status: outcome.Status, Kind: outcome.Kind, State: channelQuotaAlertEventStatePending, LockVersion: 1, ObservedAt: snapshot.ObservedAt, CreatedAt: snapshot.ObservedAt, UpdatedAt: snapshot.ObservedAt}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event).Error; err != nil {
			return err
		}
	}
	return tx.Model(&ChannelQuotaAlertState{}).Where("id = ? AND observed_at < ?", state.ID, snapshot.ObservedAt).Updates(map[string]any{"current_status": currentStatus, "observed_at": snapshot.ObservedAt, "updated_at": snapshot.ObservedAt}).Error
}

// ClaimChannelQuotaAlertEvents claims ready or expired events with per-row CAS.
// It is intentionally portable: candidate discovery uses ordinary GORM queries,
// while the conditional update is the cross-process ownership authority.
func ClaimChannelQuotaAlertEvents(ctx context.Context, workerID string, now, leaseSeconds int64, limit int) ([]ChannelQuotaAlertEvent, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || now <= 0 || leaseSeconds <= 0 || leaseSeconds > 300 || limit <= 0 || limit > 100 {
		return nil, fmt.Errorf("invalid channel quota alert claim")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	candidateLimit := limit * 4
	var candidates []ChannelQuotaAlertEvent
	if err := DB.WithContext(ctx).Where("(state IN ? AND next_attempt_at <= ?) OR (state = ? AND claimed_until < ?)", []string{channelQuotaAlertEventStatePending, channelQuotaAlertEventStateRetryable}, now, channelQuotaAlertEventStateClaimed, now).Order("next_attempt_at ASC, id ASC").Limit(candidateLimit).Find(&candidates).Error; err != nil {
		return nil, err
	}
	claimed := make([]ChannelQuotaAlertEvent, 0, limit)
	for _, candidate := range candidates {
		if len(claimed) == limit {
			break
		}
		until := now + leaseSeconds
		updates := map[string]any{"state": channelQuotaAlertEventStateClaimed, "claimed_by": workerID, "claimed_until": until, "attempt_count": candidate.AttemptCount + 1, "updated_at": now, "lock_version": candidate.LockVersion + 1}
		result := DB.WithContext(ctx).Model(&ChannelQuotaAlertEvent{}).Where("id = ? AND lock_version = ? AND ((state IN ? AND next_attempt_at <= ?) OR (state = ? AND claimed_until < ?))", candidate.ID, candidate.LockVersion, []string{channelQuotaAlertEventStatePending, channelQuotaAlertEventStateRetryable}, now, channelQuotaAlertEventStateClaimed, now).Updates(updates)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			continue
		}
		candidate.State = channelQuotaAlertEventStateClaimed
		candidate.ClaimedBy = workerID
		candidate.ClaimedUntil = until
		candidate.AttemptCount++
		candidate.UpdatedAt = now
		candidate.LockVersion++
		claimed = append(claimed, candidate)
	}
	return claimed, nil
}

// MarkChannelQuotaAlertDelivered completes a claim and records the actual
// delivery time. The event remains immutable; only delivery bookkeeping moves.
func MarkChannelQuotaAlertDelivered(ctx context.Context, event ChannelQuotaAlertEvent, workerID string, deliveredAt int64) (bool, error) {
	if DB == nil {
		return false, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if event.ID <= 0 || strings.TrimSpace(workerID) == "" || deliveredAt <= 0 {
		return false, fmt.Errorf("invalid channel quota alert delivery")
	}
	won := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&ChannelQuotaAlertEvent{}).Where("id = ? AND state = ? AND claimed_by = ? AND lock_version = ?", event.ID, channelQuotaAlertEventStateClaimed, workerID, event.LockVersion).Updates(map[string]any{"state": channelQuotaAlertEventStateDelivered, "claimed_by": "", "claimed_until": 0, "delivered_at": deliveredAt, "last_error_code": "", "updated_at": deliveredAt, "lock_version": event.LockVersion + 1})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		won = true
		return tx.Model(&ChannelQuotaAlertState{}).Where("series_key = ? AND last_delivered_at < ?", event.SeriesKey, deliveredAt).Updates(map[string]any{"last_delivered_at": deliveredAt, "updated_at": deliveredAt}).Error
	})
	return won, err
}

// MarkChannelQuotaAlertFailed ends a claim. Failures use bounded exponential
// retry; terminal failures stay auditable in quarantined state.
func MarkChannelQuotaAlertFailed(ctx context.Context, event ChannelQuotaAlertEvent, workerID, errorCode string, failedAt int64) (bool, error) {
	if DB == nil {
		return false, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	errorCode = strings.TrimSpace(errorCode)
	if event.ID <= 0 || strings.TrimSpace(workerID) == "" || failedAt <= 0 || errorCode == "" || len(errorCode) > 64 {
		return false, fmt.Errorf("invalid channel quota alert failure")
	}
	state, nextAttemptAt := channelQuotaAlertEventStateRetryable, failedAt+30
	if event.AttemptCount >= channelQuotaAlertMaxAttempts {
		state, nextAttemptAt = channelQuotaAlertEventStateQuarantined, 0
	} else {
		delay := int64(30) << channelQuotaAlertRetryExponent(event.AttemptCount-1)
		if delay > 24*60*60 {
			delay = 24 * 60 * 60
		}
		nextAttemptAt = failedAt + delay
	}
	result := DB.WithContext(ctx).Model(&ChannelQuotaAlertEvent{}).Where("id = ? AND state = ? AND claimed_by = ? AND lock_version = ?", event.ID, channelQuotaAlertEventStateClaimed, workerID, event.LockVersion).Updates(map[string]any{"state": state, "claimed_by": "", "claimed_until": 0, "next_attempt_at": nextAttemptAt, "last_error_code": errorCode, "last_error_at": failedAt, "updated_at": failedAt, "lock_version": event.LockVersion + 1})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func channelQuotaAlertRetryExponent(attempt int) int {
	if attempt < 8 {
		return attempt
	}
	return 8
}

func channelQuotaAlertNow() int64 {
	return time.Now().Unix()
}
