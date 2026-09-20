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
	"gorm.io/gorm/clause"
)

// ChannelQuotaAccountRef returns a non-reversible reference to an upstream
// account identity. Empty inputs intentionally remain unknown rather than
// collapsing unrelated credentials into a shared identity.
func ChannelQuotaAccountRef(provider, accountID string) string {
	provider = strings.TrimSpace(provider)
	accountID = strings.TrimSpace(accountID)
	if provider == "" || accountID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(provider + "\x00" + accountID))
	return hex.EncodeToString(digest[:])
}

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
	ChannelId  int      `json:"channel_id" gorm:"index:idx_channel_quota_observed,priority:1;index:idx_channel_quota_metric,priority:1;index:idx_channel_quota_dedupe,priority:1;index:idx_channel_quota_sample,priority:1"`
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
	SampleID      string `json:"sample_id,omitempty" gorm:"size:36;index:idx_channel_quota_sample,priority:2;<-:create"`
	// AccountRef is a non-reversible, provider-namespaced reference to a
	// confirmed upstream account identity. It is empty when the sampler cannot
	// prove account ownership; such snapshots must never be merged with a known
	// account's alert state.
	AccountRef   string `json:"-" gorm:"size:64;index:idx_channel_quota_dedupe,priority:11"`
	Status       string `json:"status" gorm:"size:16;default:'success';index"`
	ErrorCode    string `json:"error_code,omitempty" gorm:"size:64"`
	ErrorMessage string `json:"error_message,omitempty" gorm:"size:255"`
	// DedupeKey is nullable so AutoMigrate can add it without rewriting old
	// rows. New rows use a unique digest of the sample id, complete series
	// identity, and observation time, allowing concurrent samplers to converge on all
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

// ChannelQuotaSnapshotAbsenceMarker configures the non-value event appended for
// a series that was present in the previous successful sample but is absent
// from the new successful sample.
type ChannelQuotaSnapshotAbsenceMarker struct {
	Status       string
	ErrorCode    string
	ErrorMessage string
}

// ChannelQuotaSnapshotBatchOptions controls atomic provider-sample recording.
// PreviousSources limits absence detection to the provider's stable source
// family. AbsenceMarker may be nil for providers without window lifecycles.
type ChannelQuotaSnapshotBatchOptions struct {
	PreviousSources []string
	AbsenceMarker   *ChannelQuotaSnapshotAbsenceMarker
}

// ListLatestSuccessfulChannelQuotaSnapshotBatch returns every successful
// series from the latest successful immutable sample. Legacy rows without a
// sample id fall back to their second-resolution observation boundary.
func ListLatestSuccessfulChannelQuotaSnapshotBatch(ctx context.Context, channelID int, metricType string, sources []string) ([]ChannelQuotaSnapshot, error) {
	if DB == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return listLatestChannelQuotaSnapshotBatchWithDB(
		DB.WithContext(ctx), channelID, nil, 0, 0, metricType, sources, []string{"success"},
	)
}

// ListLatestChannelQuotaSnapshotBatch returns every row from the latest sample
// event in the selected range. It is used for current-state responses so two
// responses observed in the same second remain separate.
func ListLatestChannelQuotaSnapshotBatch(ctx context.Context, channelID int, start, end int64, metricType string) ([]ChannelQuotaSnapshot, error) {
	if DB == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return listLatestChannelQuotaSnapshotBatchWithDB(
		DB.WithContext(ctx), channelID, nil, start, end, metricType, nil, nil,
	)
}

func listLatestChannelQuotaSnapshotBatchWithDB(db *gorm.DB, channelID int, accountRef *string, start, end int64, metricType string, sources, statuses []string) ([]ChannelQuotaSnapshot, error) {
	query := db.Model(&ChannelQuotaSnapshot{}).Where("channel_id = ?", channelID)
	if accountRef != nil {
		if *accountRef == "" {
			query = query.Where("(account_ref = ? OR account_ref IS NULL)", "")
		} else {
			query = query.Where("account_ref = ?", *accountRef)
		}
	}
	if start > 0 {
		query = query.Where("observed_at >= ?", start)
	}
	if end > 0 {
		query = query.Where("observed_at <= ?", end)
	}
	if metricType != "" {
		query = query.Where("metric_type = ?", metricType)
	}
	if len(sources) > 0 {
		query = query.Where("source IN ?", sources)
	}
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}
	var latest ChannelQuotaSnapshot
	if err := query.Order("observed_at DESC, id DESC").First(&latest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return []ChannelQuotaSnapshot{}, nil
		}
		return nil, err
	}
	batchQuery := db.Model(&ChannelQuotaSnapshot{}).Where("channel_id = ?", channelID)
	if accountRef != nil {
		if *accountRef == "" {
			batchQuery = batchQuery.Where("(account_ref = ? OR account_ref IS NULL)", "")
		} else {
			batchQuery = batchQuery.Where("account_ref = ?", *accountRef)
		}
	}
	if metricType != "" {
		batchQuery = batchQuery.Where("metric_type = ?", metricType)
	}
	if len(sources) > 0 {
		batchQuery = batchQuery.Where("source IN ?", sources)
	}
	if len(statuses) > 0 {
		batchQuery = batchQuery.Where("status IN ?", statuses)
	}
	if latest.SampleID != "" {
		batchQuery = batchQuery.Where("sample_id = ?", latest.SampleID)
	} else {
		batchQuery = batchQuery.Where("(sample_id = ? OR sample_id IS NULL) AND observed_at = ?", "", latest.ObservedAt)
	}
	var snapshots []ChannelQuotaSnapshot
	if err := batchQuery.Order("id ASC").Find(&snapshots).Error; err != nil {
		return nil, err
	}
	return snapshots, nil
}

const channelQuotaSnapshotBatchTransactionAttempts = 5

// RecordChannelQuotaSnapshotBatchWithContext atomically reads the previous
// successful sample, derives absence markers, and writes the complete immutable
// sample. A replay with the same sample id is accepted only when its provider
// rows are byte-for-byte equivalent at the normalized field level.
func RecordChannelQuotaSnapshotBatchWithContext(ctx context.Context, snapshots []ChannelQuotaSnapshot, options ChannelQuotaSnapshotBatchOptions) error {
	if len(snapshots) == 0 || DB == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	normalized := make([]ChannelQuotaSnapshot, len(snapshots))
	copy(normalized, snapshots)
	channelID := normalized[0].ChannelId
	observedAt := normalized[0].ObservedAt
	sampleID := strings.TrimSpace(normalized[0].SampleID)
	if channelID <= 0 || observedAt <= 0 || sampleID == "" || len(sampleID) > 36 {
		return fmt.Errorf("channel quota snapshot batch requires channel_id, observed_at, and sample_id")
	}
	if options.AbsenceMarker != nil && (options.AbsenceMarker.Status == "" || options.AbsenceMarker.Status == "success") {
		return fmt.Errorf("channel quota absence marker requires a non-success status")
	}
	for index := range normalized {
		if normalized[index].ChannelId != channelID || normalized[index].ObservedAt != observedAt || strings.TrimSpace(normalized[index].SampleID) != sampleID {
			return fmt.Errorf("channel quota snapshot batch identity mismatch")
		}
		normalized[index].SampleID = sampleID
		if err := normalizeChannelQuotaSnapshot(&normalized[index]); err != nil {
			return err
		}
	}

	var lastErr error
	for attempt := 0; attempt < channelQuotaSnapshotBatchTransactionAttempts; attempt++ {
		err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := lockChannelQuotaSnapshotBatch(tx, channelID); err != nil {
				return err
			}
			var existing []ChannelQuotaSnapshot
			if err := tx.Where("channel_id = ? AND sample_id = ?", channelID, sampleID).Order("id ASC").Find(&existing).Error; err != nil {
				return err
			}
			if len(existing) > 0 {
				if channelQuotaSnapshotBatchEquivalent(normalized, existing, options.AbsenceMarker) {
					return nil
				}
				return fmt.Errorf("channel quota snapshot sample_id conflicts with existing batch")
			}

			complete := normalized
			if options.AbsenceMarker != nil && channelQuotaSnapshotsContainSuccess(normalized) {
				previous, err := listLatestChannelQuotaSnapshotBatchWithDB(
					tx, channelID, &normalized[0].AccountRef, 0, 0, normalized[0].MetricType, options.PreviousSources, []string{"success"},
				)
				if err != nil {
					return err
				}
				complete, err = appendChannelQuotaSnapshotAbsenceMarkers(normalized, previous, *options.AbsenceMarker)
				if err != nil {
					return err
				}
			}
			for index := range complete {
				if err := recordChannelQuotaSnapshotWithDB(tx, &complete[index]); err != nil {
					return err
				}
				if err := recordChannelQuotaAlertForSnapshot(tx, &complete[index]); err != nil {
					return err
				}
			}
			return nil
		})
		if err == nil {
			return nil
		}
		lastErr = err
		if !channelQuotaSnapshotBatchRetryable(DB, err) || attempt+1 == channelQuotaSnapshotBatchTransactionAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 5 * time.Millisecond):
		}
	}
	return lastErr
}

func lockChannelQuotaSnapshotBatch(tx *gorm.DB, channelID int) error {
	var channel Channel
	if tx.Dialector.Name() != "sqlite" {
		return lockForUpdate(tx).Select("id").Where("id = ?", channelID).First(&channel).Error
	}
	// SQLite has no SELECT FOR UPDATE. A no-op row update acquires its write
	// lock before the previous-sample read, so a competing batch retries the
	// entire transaction against the newly committed predecessor.
	if err := tx.Model(&Channel{}).Where("id = ?", channelID).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
		// Some lightweight controller tests migrate only the snapshot table. The
		// production schema always has channels, but absence of that optional
		// coordination row should not make a failure marker impossible to save.
		if !strings.Contains(strings.ToLower(err.Error()), "no such table: channels") {
			return err
		}
		return nil
	}
	return tx.Select("id").Where("id = ?", channelID).First(&channel).Error
}

func channelQuotaSnapshotBatchRetryable(db *gorm.DB, err error) bool {
	if db == nil || err == nil || db.Dialector == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	switch db.Dialector.Name() {
	case "sqlite":
		return strings.Contains(message, "sqlite_busy") || strings.Contains(message, "sqlite_locked") ||
			strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked")
	case "mysql":
		return strings.Contains(message, "error 1213") || strings.Contains(message, "error 1205") ||
			strings.Contains(message, "deadlock found") || strings.Contains(message, "lock wait timeout")
	case "postgres":
		return strings.Contains(message, "sqlstate 40001") || strings.Contains(message, "sqlstate 40p01")
	default:
		return false
	}
}

func channelQuotaSnapshotsContainSuccess(snapshots []ChannelQuotaSnapshot) bool {
	for _, snapshot := range snapshots {
		if snapshot.Status == "success" {
			return true
		}
	}
	return false
}

type channelQuotaSnapshotSeriesIdentity struct {
	AccountRef    string
	MetricType    string
	WindowType    string
	Source        string
	PlanType      string
	Unit          string
	Currency      string
	WindowSeconds int64
}

func channelQuotaSnapshotSeriesKey(snapshot ChannelQuotaSnapshot) channelQuotaSnapshotSeriesIdentity {
	return channelQuotaSnapshotSeriesIdentity{
		AccountRef: snapshot.AccountRef,
		MetricType: snapshot.MetricType, WindowType: snapshot.WindowType, Source: snapshot.Source,
		PlanType: snapshot.PlanType, Unit: snapshot.Unit, Currency: snapshot.Currency,
		WindowSeconds: snapshot.WindowSeconds,
	}
}

func appendChannelQuotaSnapshotAbsenceMarkers(current, previous []ChannelQuotaSnapshot, marker ChannelQuotaSnapshotAbsenceMarker) ([]ChannelQuotaSnapshot, error) {
	currentSeries := make(map[channelQuotaSnapshotSeriesIdentity]struct{}, len(current))
	for _, snapshot := range current {
		if snapshot.Status == "success" {
			currentSeries[channelQuotaSnapshotSeriesKey(snapshot)] = struct{}{}
		}
	}
	complete := make([]ChannelQuotaSnapshot, len(current), len(current)+len(previous))
	copy(complete, current)
	marked := make(map[channelQuotaSnapshotSeriesIdentity]struct{}, len(previous))
	for _, snapshot := range previous {
		series := channelQuotaSnapshotSeriesKey(snapshot)
		if snapshot.Status != "success" {
			continue
		}
		if _, exists := currentSeries[series]; exists {
			continue
		}
		if _, exists := marked[series]; exists {
			continue
		}
		marked[series] = struct{}{}
		absence := ChannelQuotaSnapshot{
			ChannelId: current[0].ChannelId, ObservedAt: current[0].ObservedAt, SampleID: current[0].SampleID,
			AccountRef: snapshot.AccountRef, MetricType: snapshot.MetricType, WindowType: snapshot.WindowType, Source: snapshot.Source,
			PlanType: snapshot.PlanType, Unit: snapshot.Unit, Currency: snapshot.Currency,
			WindowSeconds: snapshot.WindowSeconds, ResetAt: snapshot.ResetAt, Status: marker.Status,
			ErrorCode: marker.ErrorCode, ErrorMessage: marker.ErrorMessage,
		}
		if err := normalizeChannelQuotaSnapshot(&absence); err != nil {
			return nil, err
		}
		complete = append(complete, absence)
	}
	return complete, nil
}

func channelQuotaSnapshotBatchEquivalent(expected, existing []ChannelQuotaSnapshot, absenceMarker *ChannelQuotaSnapshotAbsenceMarker) bool {
	counts := make(map[string]int, len(expected))
	for index := range expected {
		counts[channelQuotaSnapshotContentKey(&expected[index])]++
	}
	actualCount := 0
	for index := range existing {
		if absenceMarker != nil && existing[index].Status == absenceMarker.Status && existing[index].ErrorCode == absenceMarker.ErrorCode {
			continue
		}
		key := channelQuotaSnapshotContentKey(&existing[index])
		if counts[key] == 0 {
			return false
		}
		counts[key]--
		actualCount++
	}
	if actualCount != len(expected) {
		return false
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func channelQuotaSnapshotContentKey(snapshot *ChannelQuotaSnapshot) string {
	used, total := "nil", "nil"
	if snapshot.Used != nil {
		used = strconv.FormatUint(math.Float64bits(*snapshot.Used), 10)
	}
	if snapshot.Total != nil {
		total = strconv.FormatUint(math.Float64bits(*snapshot.Total), 10)
	}
	return strings.Join([]string{
		strconv.Itoa(snapshot.ChannelId), strconv.FormatInt(snapshot.ObservedAt, 10), snapshot.SampleID, snapshot.AccountRef,
		snapshot.MetricType, snapshot.WindowType, snapshot.Source, snapshot.PlanType, snapshot.Unit, snapshot.Currency,
		strconv.FormatInt(snapshot.WindowSeconds, 10), strconv.FormatInt(snapshot.ResetAt, 10), snapshot.Status,
		snapshot.ErrorCode, snapshot.ErrorMessage, strconv.FormatUint(math.Float64bits(snapshot.Available), 10), used, total,
	}, "\x00")
}

func recordChannelQuotaSnapshotWithDB(db *gorm.DB, snapshot *ChannelQuotaSnapshot) error {
	if err := normalizeChannelQuotaSnapshot(snapshot); err != nil {
		return err
	}
	var existing ChannelQuotaSnapshot
	if err := db.Where("dedupe_key = ?", *snapshot.DedupeKey).Order("id ASC").First(&existing).Error; err == nil {
		snapshot.Id = existing.Id
		snapshot.CreatedAt = existing.CreatedAt
		return nil
	} else if err != gorm.ErrRecordNotFound {
		return err
	}
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(snapshot)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	if err := db.Where("dedupe_key = ?", *snapshot.DedupeKey).Order("id ASC").First(&existing).Error; err != nil {
		return err
	}
	snapshot.Id = existing.Id
	snapshot.CreatedAt = existing.CreatedAt
	return nil
}

func normalizeChannelQuotaSnapshot(snapshot *ChannelQuotaSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("nil quota snapshot")
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
	return nil
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
	if err := normalizeChannelQuotaSnapshot(snapshot); err != nil {
		return err
	}
	dedupeKey := *snapshot.DedupeKey
	// Legacy callers without sample ids retain one point per series/time bucket.
	// Batch-aware callers include sample_id so separate same-second provider
	// responses remain distinct while an exact batch replay is idempotent.
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
	)
	if snapshot.AccountRef == "" {
		lookup = lookup.Where("(account_ref = ? OR account_ref IS NULL)", "")
	} else {
		lookup = lookup.Where("account_ref = ?", snapshot.AccountRef)
	}
	if snapshot.SampleID == "" {
		lookup = lookup.Where("sample_id = ? OR sample_id IS NULL", "")
	} else {
		lookup = lookup.Where("sample_id = ?", snapshot.SampleID)
	}
	lookup = lookup.Order("id ASC").First(&existing)
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
		snapshot.SampleID,
		snapshot.AccountRef,
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

// ListChannelQuotaSnapshotsWithQuery returns the most recent bounded batches
// in oldest-first order, optionally constrained to one complete provider
// series. It is intentionally additive so older callers keep their existing
// behavior when they do not need series metadata.
func ListChannelQuotaSnapshotsWithQuery(channelID int, start, end int64, filter ChannelQuotaSnapshotQuery, limit int) ([]ChannelQuotaSnapshot, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	if limit <= 0 || limit > 2000 {
		limit = 2000
	}
	query := channelQuotaSnapshotQuery(DB.Model(&ChannelQuotaSnapshot{}).Where("channel_id = ?", channelID), start, end, filter)
	// A provider sample is immutable and may contain multiple rows (for
	// example Codex primary and secondary windows). Read one extra row to find
	// the batch boundary, then expand every selected batch before returning. A
	// row limit must never expose only half of a sample.
	var candidates []ChannelQuotaSnapshot
	if err := query.Order("observed_at DESC, id DESC").Limit(limit + 1).Find(&candidates).Error; err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return []ChannelQuotaSnapshot{}, nil
	}
	selected := make([]ChannelQuotaSnapshot, 0, limit)
	selectedKeys := make(map[string]struct{}, limit)
	for _, candidate := range candidates {
		key := channelQuotaSnapshotBatchKey(candidate)
		if _, exists := selectedKeys[key]; exists {
			continue
		}
		if len(selected) == limit {
			break
		}
		selectedKeys[key] = struct{}{}
		selected = append(selected, candidate)
	}

	conditions := make([]string, 0, len(selected))
	args := make([]any, 0, len(selected)*2)
	for _, batch := range selected {
		if batch.SampleID != "" {
			conditions = append(conditions, "sample_id = ?")
			args = append(args, batch.SampleID)
			continue
		}
		conditions = append(conditions, "(sample_id = ? OR sample_id IS NULL) AND observed_at = ?")
		args = append(args, "", batch.ObservedAt)
	}
	batchQuery := channelQuotaSnapshotQuery(DB.Model(&ChannelQuotaSnapshot{}).Where("channel_id = ?", channelID), start, end, filter)
	var snapshots []ChannelQuotaSnapshot
	if err := batchQuery.Where("("+strings.Join(conditions, ") OR (")+")", args...).Order("observed_at ASC, id ASC").Find(&snapshots).Error; err != nil {
		return nil, err
	}
	return snapshots, nil
}

func channelQuotaSnapshotBatchKey(snapshot ChannelQuotaSnapshot) string {
	if snapshot.SampleID != "" {
		return "sample:" + snapshot.SampleID
	}
	return "legacy:" + strconv.FormatInt(snapshot.ObservedAt, 10)
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
