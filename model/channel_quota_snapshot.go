package model

import (
	"context"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
)

// ChannelQuotaSnapshot stores a normalized point-in-time view of an upstream
// channel account's available quota. Raw upstream responses and credentials
// are intentionally not persisted here.
type ChannelQuotaSnapshot struct {
	Id         int      `json:"id" gorm:"primaryKey"`
	ChannelId  int      `json:"channel_id" gorm:"index:idx_channel_quota_observed,priority:1;index:idx_channel_quota_metric,priority:1"`
	ObservedAt int64    `json:"observed_at" gorm:"bigint;index:idx_channel_quota_observed,priority:2;index:idx_channel_quota_metric,priority:4;index:idx_channel_quota_retention"`
	Available  float64  `json:"available"`
	Used       *float64 `json:"used,omitempty"`
	Total      *float64 `json:"total,omitempty"`
	Unit       string   `json:"unit" gorm:"size:32;default:'usd'"`
	Currency   string   `json:"currency,omitempty" gorm:"size:8"`
	MetricType string   `json:"metric_type" gorm:"size:32;default:'balance';index:idx_channel_quota_metric,priority:2"`
	WindowType string   `json:"window_type,omitempty" gorm:"size:32;default:'none';index:idx_channel_quota_metric,priority:3"`
	// PlanType and WindowSeconds are populated for provider-specific rate-limit
	// observations (for example Codex OAuth). They remain empty/zero for the
	// generic balance snapshots.
	PlanType      string    `json:"plan_type,omitempty" gorm:"size:32"`
	WindowSeconds int64     `json:"window_seconds,omitempty" gorm:"bigint"`
	ResetAt       int64     `json:"reset_at,omitempty" gorm:"bigint"`
	Source        string    `json:"source,omitempty" gorm:"size:64"`
	Status        string    `json:"status" gorm:"size:16;default:'success';index"`
	ErrorCode     string    `json:"error_code,omitempty" gorm:"size:64"`
	ErrorMessage  string    `json:"error_message,omitempty" gorm:"size:255"`
	CreatedAt     time.Time `json:"created_at"`
}

// DeleteOldChannelQuotaSnapshotBatch deletes at most limit snapshots observed
// before cutoff. IDs are selected first so the limit is honored consistently
// across SQLite, MySQL, and PostgreSQL dialects.
func DeleteOldChannelQuotaSnapshotBatch(ctx context.Context, cutoff int64, limit int) (int64, error) {
	if DB == nil {
		return 0, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if cutoff <= 0 {
		return 0, nil
	}
	if limit <= 0 {
		limit = 500
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var ids []int
	if err := DB.WithContext(ctx).Model(&ChannelQuotaSnapshot{}).
		Where("observed_at < ?", cutoff).
		Order("observed_at ASC, id ASC").
		Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := DB.WithContext(ctx).Where("id IN ?", ids).Delete(&ChannelQuotaSnapshot{})
	return result.RowsAffected, result.Error
}

func (ChannelQuotaSnapshot) TableName() string {
	return "channel_quota_snapshots"
}

// RecordChannelQuotaSnapshot appends a normalized quota observation. It is
// safe to call when the database has not been initialized (for example in
// lightweight controller tests).
func RecordChannelQuotaSnapshot(snapshot *ChannelQuotaSnapshot) error {
	if snapshot == nil || DB == nil {
		return nil
	}
	if math.IsNaN(snapshot.Available) || math.IsInf(snapshot.Available, 0) {
		return fmt.Errorf("invalid quota snapshot available value")
	}
	if snapshot.Used != nil && (math.IsNaN(*snapshot.Used) || math.IsInf(*snapshot.Used, 0)) {
		return fmt.Errorf("invalid quota snapshot used value")
	}
	if snapshot.Total != nil && (math.IsNaN(*snapshot.Total) || math.IsInf(*snapshot.Total, 0)) {
		return fmt.Errorf("invalid quota snapshot total value")
	}
	if snapshot.ObservedAt == 0 {
		snapshot.ObservedAt = time.Now().Unix()
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}
	if snapshot.Unit == "" {
		snapshot.Unit = "usd"
	}
	if snapshot.MetricType == "" {
		snapshot.MetricType = "balance"
	}
	if snapshot.WindowType == "" {
		snapshot.WindowType = "none"
	}
	if snapshot.Status == "" {
		snapshot.Status = "success"
	}
	return DB.Create(snapshot).Error
}

// ListChannelQuotaSnapshots returns bounded, oldest-first observations. The
// caller is expected to provide a sensible time range; this method enforces a
// hard upper bound to prevent accidental unbounded history responses.
func ListChannelQuotaSnapshots(channelID int, start, end int64, metricType, windowType string, limit int) ([]ChannelQuotaSnapshot, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	if limit <= 0 || limit > 2000 {
		limit = 2000
	}
	query := DB.Model(&ChannelQuotaSnapshot{}).Where("channel_id = ?", channelID)
	if start > 0 {
		query = query.Where("observed_at >= ?", start)
	}
	if end > 0 {
		query = query.Where("observed_at <= ?", end)
	}
	if metricType != "" {
		query = query.Where("metric_type = ?", metricType)
	}
	if windowType != "" {
		query = query.Where("window_type = ?", windowType)
	}
	var snapshots []ChannelQuotaSnapshot
	err := query.Order("observed_at ASC").Limit(limit).Find(&snapshots).Error
	return snapshots, err
}
