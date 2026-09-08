package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
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

const maxChannelQuotaSeriesCatalogueItems = 2000
const maxChannelQuotaSeriesCatalogueCandidates = 20000

// ChannelQuotaSeriesCatalogueRow is one distinct successful historical quota
// series joined to its current channel name. It contains only series identity
// metadata; observation values and provider response details are deliberately
// excluded so catalogue reads stay bounded independently of history density.
type ChannelQuotaSeriesCatalogueRow struct {
	ChannelID     int    `gorm:"column:channel_id"`
	ChannelName   string `gorm:"column:channel_name"`
	MetricType    string `gorm:"column:metric_type"`
	WindowType    string `gorm:"column:window_type"`
	Source        string `gorm:"column:source"`
	PlanType      string `gorm:"column:plan_type"`
	Unit          string `gorm:"column:unit"`
	Currency      string `gorm:"column:currency"`
	WindowSeconds int64  `gorm:"column:window_seconds"`
}

type channelQuotaSeriesCatalogueCandidate struct {
	ChannelQuotaSeriesCatalogueRow `gorm:"embedded"`
	ExistingChannelID              *int   `gorm:"column:existing_channel_id"`
	Status                         string `gorm:"column:status"`
}

// ChannelQuotaSeriesCatalogueResult distinguishes the bounded source scan from
// the final item limit. Rows contains only identities discovered in the newest
// candidate window and is always trimmed to the requested item limit.
type ChannelQuotaSeriesCatalogueResult struct {
	Rows           []ChannelQuotaSeriesCatalogueRow
	ScanLimit      int
	ScannedItems   int
	SourceComplete bool
	ItemsComplete  bool
}

// ListChannelQuotaSeriesCatalogue lists series found in the newest bounded
// window of historical snapshots. The inner subquery is deliberately ordered
// only by observed_at so every database can serve the bound from the existing
// retention index before status filtering or the channel join. Only identity,
// status, and channel-existence metadata is projected; values used for quota
// calculations never enter the catalogue read. When the source window is
// truncated, SourceComplete is false because an older series may be hidden.
func ListChannelQuotaSeriesCatalogue(ctx context.Context, limit int) (ChannelQuotaSeriesCatalogueResult, error) {
	return listChannelQuotaSeriesCatalogue(ctx, limit, maxChannelQuotaSeriesCatalogueCandidates)
}

func listChannelQuotaSeriesCatalogue(ctx context.Context, limit, candidateLimit int) (ChannelQuotaSeriesCatalogueResult, error) {
	if DB == nil {
		return ChannelQuotaSeriesCatalogueResult{}, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 || limit > maxChannelQuotaSeriesCatalogueItems {
		limit = maxChannelQuotaSeriesCatalogueItems
	}
	if candidateLimit <= 0 || candidateLimit > maxChannelQuotaSeriesCatalogueCandidates {
		candidateLimit = maxChannelQuotaSeriesCatalogueCandidates
	}
	recent := DB.WithContext(ctx).
		Table("channel_quota_snapshots AS snapshots").
		Select(`snapshots.channel_id, snapshots.metric_type,
			snapshots.window_type, snapshots.source,
			snapshots.plan_type, snapshots.unit, snapshots.currency,
			snapshots.window_seconds, snapshots.status`).
		Order("snapshots.observed_at DESC").
		Limit(candidateLimit + 1)
	candidates := make([]channelQuotaSeriesCatalogueCandidate, 0, candidateLimit+1)
	err := DB.WithContext(ctx).
		Table("(?) AS recent", recent).
		Select(`recent.channel_id, channels.id AS existing_channel_id,
			channels.name AS channel_name, recent.metric_type, recent.window_type,
			recent.source, recent.plan_type, recent.unit, recent.currency,
			recent.window_seconds, recent.status`).
		Joins("LEFT JOIN channels ON channels.id = recent.channel_id").
		Find(&candidates).Error
	if err != nil {
		return ChannelQuotaSeriesCatalogueResult{}, err
	}
	sourceComplete := len(candidates) <= candidateLimit
	scannedItems := len(candidates)
	if !sourceComplete {
		candidates = candidates[:candidateLimit]
		scannedItems = candidateLimit
	}
	type seriesKey struct {
		ChannelID     int
		MetricType    string
		WindowType    string
		Source        string
		PlanType      string
		Unit          string
		Currency      string
		WindowSeconds int64
	}
	identities := make(map[seriesKey]ChannelQuotaSeriesCatalogueRow, len(candidates))
	for _, candidate := range candidates {
		if candidate.Status != "success" || candidate.ExistingChannelID == nil {
			continue
		}
		row := candidate.ChannelQuotaSeriesCatalogueRow
		key := seriesKey{
			ChannelID: row.ChannelID, MetricType: row.MetricType, WindowType: row.WindowType,
			Source: row.Source, PlanType: row.PlanType, Unit: row.Unit,
			Currency: row.Currency, WindowSeconds: row.WindowSeconds,
		}
		identities[key] = row
	}
	rows := make([]ChannelQuotaSeriesCatalogueRow, 0, len(identities))
	for _, row := range identities {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		left, right := rows[i], rows[j]
		if left.ChannelID != right.ChannelID {
			return left.ChannelID < right.ChannelID
		}
		if left.MetricType != right.MetricType {
			return left.MetricType < right.MetricType
		}
		if left.WindowType != right.WindowType {
			return left.WindowType < right.WindowType
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		if left.PlanType != right.PlanType {
			return left.PlanType < right.PlanType
		}
		if left.Unit != right.Unit {
			return left.Unit < right.Unit
		}
		if left.Currency != right.Currency {
			return left.Currency < right.Currency
		}
		return left.WindowSeconds < right.WindowSeconds
	})
	itemsComplete := sourceComplete && len(rows) <= limit
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return ChannelQuotaSeriesCatalogueResult{
		Rows: rows, ScanLimit: candidateLimit, ScannedItems: scannedItems,
		SourceComplete: sourceComplete, ItemsComplete: itemsComplete,
	}, nil
}

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
		query = query.Where("(snapshots.window_type = ? OR (snapshots.window_type = ? AND snapshots.status IN ?))", windowType, "none", []string{"error", "unsupported"})
	}
	if source != "" {
		sources := []string{source}
		for _, suffix := range []string{"_primary", "_secondary"} {
			if base := strings.TrimSuffix(source, suffix); base != source {
				sources = append(sources, base)
				break
			}
		}
		query = query.Where("snapshots.source IN ?", sources)
	}
	rows := make([]ChannelQuotaAggregateRow, 0)
	err := query.Order("snapshots.observed_at DESC, snapshots.id DESC").Limit(maxChannelQuotaAggregateRows + 1).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) > maxChannelQuotaAggregateRows {
		return nil, fmt.Errorf("quota changes observation limit exceeded; select fewer channels or a shorter range")
	}
	return rows, nil
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
	// Keep the column definition free of a unique constraint. SQLite cannot
	// add a UNIQUE column with ALTER TABLE during AutoMigrate; the migration
	// creates the unique index separately after the nullable column exists.
	DedupeKey *string   `json:"-" gorm:"size:64"`
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
	return RecordChannelQuotaSnapshotWithContext(context.Background(), snapshot)
}

// RecordChannelQuotaSnapshotWithContext bounds both deduplication queries and
// insertion, including time spent waiting for an available database connection.
func RecordChannelQuotaSnapshotWithContext(ctx context.Context, snapshot *ChannelQuotaSnapshot) error {
	if snapshot == nil || DB == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	db := DB.WithContext(ctx)
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
	lookup := db.Where(
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
	if err := db.Create(snapshot).Error; err != nil {
		// Two workers can both miss the pre-insert lookup. The unique digest is
		// the database-level arbiter in that race; recover the winner instead of
		// surfacing a transient duplicate-key failure to the sampler.
		var concurrent ChannelQuotaSnapshot
		if concurrentLookup := db.Where("dedupe_key = ?", dedupeKey).Order("id ASC").First(&concurrent); concurrentLookup.Error == nil {
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
// long 30/90-day window and dropping the current trend endpoint. History
// callers that need an entire time range must use
// ListChannelQuotaSnapshotsForHistory instead: its completeness result makes
// a bounded scan explicit rather than silently returning only recent rows.
// ChannelQuotaSnapshotQuery selects one normalized quota series. Empty string
// fields are treated as wildcards; WindowSeconds is nil when no window length
// was requested. Callers that need a single series can first resolve the
// latest row and then fill the omitted fields from its metadata.
type ChannelQuotaSnapshotQuery struct {
	// ExactIdentity makes resolved empty metadata values meaningful instead of
	// treating them as wildcards that could mix units or subscription plans.
	ExactIdentity bool
	// EventMetadata allows missing metadata on a failed provider query while
	// rejecting failures that explicitly identify another plan or window.
	EventMetadata bool
	MetricType    string
	WindowType    string
	Source        string
	// Sources is a multi-source selector for callers that need a small,
	// explicit source family (for example a Codex window and its generic
	// failure marker). It takes precedence over Source when non-empty.
	Sources       []string
	PlanType      string
	Unit          string
	Currency      string
	WindowSeconds *int64
	// Statuses narrows a query to explicit normalized states. An empty slice
	// preserves the legacy wildcard behavior.
	Statuses []string
}

// ChannelQuotaSnapshotHistoryResult describes a deliberately bounded full
// range read. Complete is false when more than the caller's maxRows matched; Snapshots is
// then nil so callers cannot accidentally calculate a trend from a partial
// range.
type ChannelQuotaSnapshotHistoryResult struct {
	Snapshots []ChannelQuotaSnapshot
	Complete  bool
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
	query := channelQuotaSnapshotQuery(DB.Model(&ChannelQuotaSnapshot{}).Where("channel_id = ?", channelID), start, end, filter)
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

// CountChannelQuotaSnapshotsWithQuery counts every observation in a selected
// time range. It is intentionally separate from the legacy bounded list API
// so history endpoints can reject or flag a dense range before allocating its
// raw observations.
func CountChannelQuotaSnapshotsWithQuery(ctx context.Context, channelID int, start, end int64, filter ChannelQuotaSnapshotQuery) (int64, error) {
	if DB == nil {
		return 0, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var count int64
	query := channelQuotaSnapshotQuery(DB.WithContext(ctx).Model(&ChannelQuotaSnapshot{}).Where("channel_id = ?", channelID), start, end, filter)
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ListChannelQuotaSnapshotsForHistory reads an entire selected range in
// chronological order, subject to the caller's explicit maxRows budget. It
// reads one extra row to detect a concurrent insert after a preceding count.
// A non-complete result never contains a partial slice.
func ListChannelQuotaSnapshotsForHistory(ctx context.Context, channelID int, start, end int64, filter ChannelQuotaSnapshotQuery, maxRows int) (ChannelQuotaSnapshotHistoryResult, error) {
	if DB == nil {
		return ChannelQuotaSnapshotHistoryResult{}, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if maxRows <= 0 {
		return ChannelQuotaSnapshotHistoryResult{}, fmt.Errorf("history maxRows must be positive")
	}
	query := channelQuotaSnapshotQuery(DB.WithContext(ctx).Model(&ChannelQuotaSnapshot{}).Where("channel_id = ?", channelID), start, end, filter)
	var snapshots []ChannelQuotaSnapshot
	if err := query.Order("observed_at ASC, id ASC").Limit(maxRows + 1).Find(&snapshots).Error; err != nil {
		return ChannelQuotaSnapshotHistoryResult{}, err
	}
	if len(snapshots) > maxRows {
		return ChannelQuotaSnapshotHistoryResult{Complete: false}, nil
	}
	return ChannelQuotaSnapshotHistoryResult{Snapshots: snapshots, Complete: true}, nil
}

func channelQuotaSnapshotQuery(query *gorm.DB, start, end int64, filter ChannelQuotaSnapshotQuery) *gorm.DB {
	if start > 0 {
		query = query.Where("observed_at >= ?", start)
	}
	if end > 0 {
		query = query.Where("observed_at <= ?", end)
	}
	if filter.ExactIdentity || filter.MetricType != "" {
		query = query.Where("metric_type = ?", filter.MetricType)
	}
	if filter.EventMetadata {
		query = query.Where("window_type IN ?", []string{filter.WindowType, "", "none"})
	} else if filter.ExactIdentity || filter.WindowType != "" {
		query = query.Where("window_type = ?", filter.WindowType)
	}
	if len(filter.Sources) > 0 {
		query = query.Where("source IN ?", filter.Sources)
	} else if filter.ExactIdentity || filter.Source != "" {
		query = query.Where("source = ?", filter.Source)
	}
	if filter.EventMetadata {
		query = query.Where("plan_type IN ?", []string{filter.PlanType, ""})
	} else if filter.ExactIdentity || filter.PlanType != "" {
		query = query.Where("plan_type = ?", filter.PlanType)
	}
	if filter.EventMetadata {
		query = query.Where("unit IN ?", []string{filter.Unit, ""})
	} else if filter.ExactIdentity || filter.Unit != "" {
		query = query.Where("unit = ?", filter.Unit)
	}
	if filter.EventMetadata {
		query = query.Where("currency IN ?", []string{filter.Currency, ""})
	} else if filter.ExactIdentity || filter.Currency != "" {
		query = query.Where("currency = ?", filter.Currency)
	}
	if filter.WindowSeconds != nil {
		if filter.EventMetadata {
			query = query.Where("window_seconds IN ?", []int64{*filter.WindowSeconds, 0})
		} else {
			query = query.Where("window_seconds = ?", *filter.WindowSeconds)
		}
	}
	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}
	return query
}
