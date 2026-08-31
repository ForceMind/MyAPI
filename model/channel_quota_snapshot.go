package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ChannelQuotaAggregateRow is the redacted projection used by the global
// quota-change view. It deliberately contains no channel key, settings, or
// upstream response data. Account identity is represented by the channel's
// operator-provided name and stable numeric id.
type ChannelQuotaAggregateRow struct {
	ID            int      `gorm:"column:id"`
	ChannelID     int      `gorm:"column:channel_id"`
	ChannelName   string   `gorm:"column:channel_name"`
	ObservedAt    int64    `gorm:"column:observed_at"`
	Available     float64  `gorm:"column:available"`
	Used          *float64 `gorm:"column:used"`
	Total         *float64 `gorm:"column:total"`
	Unit          string   `gorm:"column:unit"`
	Currency      string   `gorm:"column:currency"`
	MetricType    string   `gorm:"column:metric_type"`
	WindowType    string   `gorm:"column:window_type"`
	PlanType      string   `gorm:"column:plan_type"`
	WindowSeconds int64    `gorm:"column:window_seconds"`
	ResetAt       int64    `gorm:"column:reset_at"`
	Source        string   `gorm:"column:source"`
	Status        string   `gorm:"column:status"`
	ErrorCode     string   `gorm:"column:error_code"`
}

const maxChannelQuotaAggregateRows = 200000

// ListChannelQuotaAggregateRows returns a bounded, redacted set of quota
// observations for all channels in one JOIN query. The newest rows are read
// first so a busy installation still gets current observations; callers sort
// rows within each metric/window group before deriving rates.
func ListChannelQuotaAggregateRows(ctx context.Context, start, end int64, channelIDs []int, metricType, windowType, source string) ([]ChannelQuotaAggregateRow, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := DB.WithContext(ctx).
		Table("channel_quota_snapshots AS snapshots").
		Select(`snapshots.id, snapshots.channel_id, channels.name AS channel_name,
			snapshots.observed_at, snapshots.available, snapshots.used, snapshots.total,
			snapshots.unit, snapshots.currency, snapshots.metric_type, snapshots.window_type,
			snapshots.plan_type, snapshots.window_seconds, snapshots.reset_at, snapshots.source,
			snapshots.status, snapshots.error_code`).
		Joins("JOIN channels ON channels.id = snapshots.channel_id")
	if start > 0 {
		query = query.Where("snapshots.observed_at >= ?", start)
	}
	if end > 0 {
		query = query.Where("snapshots.observed_at <= ?", end)
	}
	if len(channelIDs) > 0 {
		query = query.Where("snapshots.channel_id IN ?", channelIDs)
	}
	if metricType != "" {
		query = query.Where("snapshots.metric_type = ?", metricType)
	}
	if windowType != "" {
		query = query.Where("snapshots.window_type = ?", windowType)
	}
	if source != "" {
		query = query.Where("snapshots.source = ?", source)
	}
	rows := make([]ChannelQuotaAggregateRow, 0)
	err := query.Order("snapshots.observed_at DESC, snapshots.id DESC").Limit(maxChannelQuotaAggregateRows).Find(&rows).Error
	return rows, err
}

// ChannelQuotaSnapshot stores a normalized point-in-time view of an upstream
// channel account's available quota. Raw upstream responses and credentials
// are intentionally not persisted here.
type ChannelQuotaSnapshot struct {
	Id         int      `json:"id" gorm:"primaryKey"`
	ChannelId  int      `json:"channel_id" gorm:"index:idx_channel_quota_observed,priority:1;index:idx_channel_quota_metric,priority:1;index:idx_channel_quota_dedupe,priority:1"`
	ObservedAt int64    `json:"observed_at" gorm:"bigint;index:idx_channel_quota_observed,priority:2;index:idx_channel_quota_metric,priority:4;index:idx_channel_quota_retention;index:idx_channel_quota_dedupe,priority:2"`
	Available  float64  `json:"available"`
	Used       *float64 `json:"used,omitempty"`
	Total      *float64 `json:"total,omitempty"`
	Unit       string   `json:"unit" gorm:"size:32;default:'usd';index:idx_channel_quota_dedupe,priority:6"`
	Currency   string   `json:"currency,omitempty" gorm:"size:8;index:idx_channel_quota_dedupe,priority:7"`
	MetricType string   `json:"metric_type" gorm:"size:32;default:'balance';index:idx_channel_quota_metric,priority:2;index:idx_channel_quota_dedupe,priority:3"`
	WindowType string   `json:"window_type,omitempty" gorm:"size:32;default:'none';index:idx_channel_quota_metric,priority:3;index:idx_channel_quota_dedupe,priority:4"`
	// PlanType and WindowSeconds are populated for provider-specific rate-limit
	// observations (for example Codex OAuth). They remain empty/zero for the
	// generic balance snapshots.
	PlanType      string `json:"plan_type,omitempty" gorm:"size:32;index:idx_channel_quota_dedupe,priority:5"`
	WindowSeconds int64  `json:"window_seconds,omitempty" gorm:"bigint;index:idx_channel_quota_dedupe,priority:8"`
	ResetAt       int64  `json:"reset_at,omitempty" gorm:"bigint;index:idx_channel_quota_dedupe,priority:9"`
	Source        string `json:"source,omitempty" gorm:"size:64;index:idx_channel_quota_dedupe,priority:10"`
	Status        string `json:"status" gorm:"size:16;default:'success';index"`
	ErrorCode     string `json:"error_code,omitempty" gorm:"size:64"`
	ErrorMessage  string `json:"error_message,omitempty" gorm:"size:255"`
	// DedupeKey is nullable so AutoMigrate can add it without rewriting old
	// rows. New rows use a unique digest of the complete series identity and
	// observation time, allowing concurrent samplers to converge safely on all
	// supported SQL dialects without a wide dialect-sensitive composite index.
	DedupeKey *string   `json:"-" gorm:"size:64;uniqueIndex:idx_channel_quota_dedupe_key"`
	CreatedAt time.Time `json:"created_at"`
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
	dedupeKey := channelQuotaSnapshotDedupeKey(snapshot)
	snapshot.DedupeKey = &dedupeKey
	// A sampler retry can produce the same observation more than once. Query
	// the complete series identity before inserting so all supported SQL
	// dialects converge on one point per channel/series/time bucket without
	// requiring a dialect-specific upsert or rewriting existing rows.
	var existing ChannelQuotaSnapshot
	lookup := DB.Where(
		"channel_id = ? AND observed_at = ? AND metric_type = ? AND window_type = ? AND source = ? AND plan_type = ? AND unit = ? AND currency = ? AND window_seconds = ? AND reset_at = ?",
		snapshot.ChannelId,
		snapshot.ObservedAt,
		snapshot.MetricType,
		snapshot.WindowType,
		snapshot.Source,
		snapshot.PlanType,
		snapshot.Unit,
		snapshot.Currency,
		snapshot.WindowSeconds,
		snapshot.ResetAt,
	).Order("id ASC").First(&existing)
	if lookup.Error == nil {
		snapshot.Id = existing.Id
		snapshot.CreatedAt = existing.CreatedAt
		return nil
	}
	if lookup.Error != gorm.ErrRecordNotFound {
		return lookup.Error
	}
	if err := DB.Create(snapshot).Error; err != nil {
		// Two workers can both miss the pre-insert lookup. The unique digest is
		// the database-level arbiter in that race; recover the winner instead of
		// surfacing a transient duplicate-key failure to the sampler.
		var concurrent ChannelQuotaSnapshot
		if concurrentLookup := DB.Where("dedupe_key = ?", dedupeKey).Order("id ASC").First(&concurrent); concurrentLookup.Error == nil {
			snapshot.Id = concurrent.Id
			snapshot.CreatedAt = concurrent.CreatedAt
			return nil
		}
		return err
	}
	return nil
}

func channelQuotaSnapshotDedupeKey(snapshot *ChannelQuotaSnapshot) string {
	parts := []string{
		strconv.Itoa(snapshot.ChannelId),
		strconv.FormatInt(snapshot.ObservedAt, 10),
		snapshot.MetricType,
		snapshot.WindowType,
		snapshot.Source,
		snapshot.PlanType,
		snapshot.Unit,
		snapshot.Currency,
		strconv.FormatInt(snapshot.WindowSeconds, 10),
		strconv.FormatInt(snapshot.ResetAt, 10),
	}
	var builder strings.Builder
	for _, part := range parts {
		fmt.Fprintf(&builder, "%d:", len(part))
		builder.WriteString(part)
	}
	digest := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(digest[:])
}

// ListChannelQuotaSnapshots returns the most recent bounded observations in
// oldest-first order. Selecting the newest rows before reversing prevents a
// high-frequency sampler from filling the limit with only the beginning of a
// long 30/90-day window and dropping the current trend endpoint.
// ChannelQuotaSnapshotQuery selects one normalized quota series. Empty string
// fields are treated as wildcards; WindowSeconds is nil when no window length
// was requested. Callers that need a single series can first resolve the
// latest row and then fill the omitted fields from its metadata.
type ChannelQuotaSnapshotQuery struct {
	MetricType    string
	WindowType    string
	Source        string
	PlanType      string
	Unit          string
	Currency      string
	WindowSeconds *int64
}

// ListChannelQuotaSnapshots keeps the legacy query surface for existing
// callers while routing through the series-aware implementation.
func ListChannelQuotaSnapshots(channelID int, start, end int64, metricType, windowType string, limit int) ([]ChannelQuotaSnapshot, error) {
	return ListChannelQuotaSnapshotsWithQuery(channelID, start, end, ChannelQuotaSnapshotQuery{
		MetricType: metricType,
		WindowType: windowType,
	}, limit)
}

// ListChannelQuotaSnapshotsWithQuery returns the most recent bounded
// observations in oldest-first order, optionally constrained to one complete
// provider series. It is intentionally additive so older callers keep their
// existing behavior when they do not need series metadata.
func ListChannelQuotaSnapshotsWithQuery(channelID int, start, end int64, filter ChannelQuotaSnapshotQuery, limit int) ([]ChannelQuotaSnapshot, error) {
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
	if filter.MetricType != "" {
		query = query.Where("metric_type = ?", filter.MetricType)
	}
	if filter.WindowType != "" {
		query = query.Where("window_type = ?", filter.WindowType)
	}
	if filter.Source != "" {
		query = query.Where("source = ?", filter.Source)
	}
	if filter.PlanType != "" {
		query = query.Where("plan_type = ?", filter.PlanType)
	}
	if filter.Unit != "" {
		query = query.Where("unit = ?", filter.Unit)
	}
	if filter.Currency != "" {
		query = query.Where("currency = ?", filter.Currency)
	}
	if filter.WindowSeconds != nil {
		query = query.Where("window_seconds = ?", *filter.WindowSeconds)
	}
	var snapshots []ChannelQuotaSnapshot
	err := query.Order("observed_at DESC, id DESC").Limit(limit).Find(&snapshots).Error
	if err != nil {
		return nil, err
	}
	for left, right := 0, len(snapshots)-1; left < right; left, right = left+1, right-1 {
		snapshots[left], snapshots[right] = snapshots[right], snapshots[left]
	}
	return snapshots, err
}
