package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
)

type QuotaHistoryGranularity string

const (
	QuotaHistoryRaw                QuotaHistoryGranularity = "raw"
	QuotaHistoryMinute             QuotaHistoryGranularity = "minute"
	QuotaHistoryFiveMinutes        QuotaHistoryGranularity = "5m"
	QuotaHistoryFifteenMinutes     QuotaHistoryGranularity = "15m"
	QuotaHistoryHour               QuotaHistoryGranularity = "hour"
	QuotaHistoryDay                QuotaHistoryGranularity = "day"
	QuotaHistoryWeek               QuotaHistoryGranularity = "week"
	QuotaHistoryAuto               QuotaHistoryGranularity = "auto"
	MaxQuotaHistoryPointLimit                              = 5000
	MaxQuotaHistoryRawObservations                         = 150000
)

func resolveQuotaHistorySeriesFilter(filter model.ChannelQuotaSnapshotQuery, latest model.ChannelQuotaSnapshot) model.ChannelQuotaSnapshotQuery {
	filter.ExactIdentity = true
	if filter.MetricType == "" {
		filter.MetricType = latest.MetricType
	}
	if filter.WindowType == "" {
		filter.WindowType = latest.WindowType
	}
	if filter.Source == "" {
		filter.Source = latest.Source
	}
	if filter.PlanType == "" {
		filter.PlanType = latest.PlanType
	}
	if filter.Unit == "" {
		filter.Unit = latest.Unit
	}
	if filter.Currency == "" {
		filter.Currency = latest.Currency
	}
	if filter.WindowSeconds == nil {
		windowSeconds := latest.WindowSeconds
		filter.WindowSeconds = &windowSeconds
	}
	return filter
}

func quotaHistoryBucketStart(timestamp int64, granularity QuotaHistoryGranularity, timezoneOffset int) int64 {
	local := time.Unix(timestamp, 0).UTC().Add(time.Duration(timezoneOffset) * time.Minute)
	var bucket time.Time
	switch granularity {
	case QuotaHistoryMinute:
		bucket = local.Truncate(time.Minute)
	case QuotaHistoryFiveMinutes:
		bucket = local.Truncate(5 * time.Minute)
	case QuotaHistoryFifteenMinutes:
		bucket = local.Truncate(15 * time.Minute)
	case QuotaHistoryHour:
		bucket = local.Truncate(time.Hour)
	case QuotaHistoryDay:
		bucket = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	case QuotaHistoryWeek:
		day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
		daysSinceMonday := (int(day.Weekday()) + 6) % 7
		bucket = day.AddDate(0, 0, -daysSinceMonday)
	default:
		return timestamp
	}
	return bucket.Add(-time.Duration(timezoneOffset) * time.Minute).Unix()
}

// aggregateQuotaHistorySnapshots keeps the actual latest observation in each
// bucket. In particular, a later failed sample is never replaced by an older
// success: consumers need to know that the newest provider observation did
// not yield a numeric value. The history endpoint adds bucket counts and
// continuity markers on top of this legacy helper.
func aggregateQuotaHistorySnapshots(snapshots []model.ChannelQuotaSnapshot, granularity QuotaHistoryGranularity, timezoneOffset int) []model.ChannelQuotaSnapshot {
	if granularity == QuotaHistoryRaw || len(snapshots) < 2 {
		return snapshots
	}
	aggregated := make([]model.ChannelQuotaSnapshot, 0, len(snapshots))
	// A time bucket can contain multiple independent provider series (for
	// example Codex primary/secondary windows or a plan/unit transition). Keep
	// those series separate while aggregating so one observation cannot replace
	// another merely because they share a timestamp bucket.
	type seriesKey struct {
		Bucket        int64
		MetricType    string
		WindowType    string
		Source        string
		PlanType      string
		Unit          string
		Currency      string
		WindowSeconds int64
	}
	indices := make(map[seriesKey]int, len(snapshots))
	for _, snapshot := range snapshots {
		bucket := quotaHistoryBucketStart(snapshot.ObservedAt, granularity, timezoneOffset)
		snapshot.ObservedAt = bucket
		key := seriesKey{
			Bucket:        bucket,
			MetricType:    snapshot.MetricType,
			WindowType:    snapshot.WindowType,
			Source:        snapshot.Source,
			PlanType:      snapshot.PlanType,
			Unit:          snapshot.Unit,
			Currency:      snapshot.Currency,
			WindowSeconds: snapshot.WindowSeconds,
		}
		if index, ok := indices[key]; ok {
			aggregated[index] = snapshot
			continue
		}
		indices[key] = len(aggregated)
		aggregated = append(aggregated, snapshot)
	}
	return aggregated
}

const (
	quotaHistoryMinimumRateSpanSeconds = int64(time.Hour / time.Second)
	quotaHistoryForecastMaximumDays    = 365 * 100
)

// quotaHistoryDataQuality describes the observations used by the derived
// metrics. Invalid numeric values are counted separately and never participate
// in rate or forecast calculations.
type quotaHistoryDataQuality struct {
	SampleCount         int   `json:"sample_count"`
	SuccessCount        int   `json:"success_count"`
	ErrorCount          int   `json:"error_count"`
	UnsupportedCount    int   `json:"unsupported_count,omitempty"`
	InvalidCount        int   `json:"invalid_count"`
	ResetBoundaries     int   `json:"reset_boundaries"`
	ObservedSpanSeconds int64 `json:"observed_span_seconds"`
	SpanSeconds         int64 `json:"span_seconds"`
}

type quotaHistoryDerivedMetrics struct {
	DropRatePerDay     *float64                `json:"drop_rate_per_day"`
	ForecastZeroAt     *int64                  `json:"forecast_zero_at"`
	ForecastConfidence string                  `json:"forecast_confidence"`
	DataQuality        quotaHistoryDataQuality `json:"data_quality"`
}

type QuotaHistoryAlert struct {
	Enabled          bool     `json:"enabled"`
	Status           string   `json:"status"`
	RatioPercent     *float64 `json:"ratio_percent,omitempty"`
	WarningPercent   float64  `json:"warning_percent"`
	CriticalPercent  float64  `json:"critical_percent"`
	CooldownSeconds  int64    `json:"cooldown_seconds"`
	NotifyOnRecovery bool     `json:"notify_on_recovery"`
}

// DeriveQuotaHistoryAlert exposes an opt-in, read-only threshold status. It
// intentionally does not send notifications, disable channels, or alter
// routing. Without a provider-reported total quota the ratio is undefined and
// the status remains unavailable, avoiding misleading currency-specific
// absolute thresholds.
func DeriveQuotaHistoryAlert(snapshot *model.ChannelQuotaSnapshot) QuotaHistoryAlert {
	alert := QuotaHistoryAlert{
		Enabled:          common.ChannelQuotaAlertEnabled,
		WarningPercent:   common.ChannelQuotaAlertWarningPercent,
		CriticalPercent:  common.ChannelQuotaAlertCriticalPercent,
		CooldownSeconds:  common.ChannelQuotaAlertCooldownSeconds,
		NotifyOnRecovery: common.ChannelQuotaAlertNotifyOnRecovery,
		Status:           "disabled",
	}
	if !alert.Enabled {
		return alert
	}
	alert.Status = "unavailable"
	if snapshot == nil || !QuotaSnapshotUsable(*snapshot) || snapshot.Total == nil ||
		!FiniteQuotaValue(snapshot.Available) || !FiniteQuotaValue(*snapshot.Total) || *snapshot.Total <= 0 {
		return alert
	}
	ratio := snapshot.Available / *snapshot.Total * 100
	if !FiniteQuotaValue(ratio) {
		return alert
	}
	alert.RatioPercent = &ratio
	switch {
	case ratio <= alert.CriticalPercent:
		alert.Status = "critical"
	case ratio <= alert.WarningPercent:
		alert.Status = "warning"
	default:
		alert.Status = "healthy"
	}
	return alert
}

func FiniteQuotaValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// deriveQuotaHistoryMetrics computes conservative, read-only trend indicators.
// A reset, failed observation, unsupported observation, or invalid numeric
// observation breaks the rate segment. This prevents a forecast from bridging
// an unobserved period and presenting it as continuous provider data.
func deriveQuotaHistoryMetrics(snapshots []model.ChannelQuotaSnapshot) quotaHistoryDerivedMetrics {
	return quotaHistoryMetricsFromConsumption(DeriveQuotaConsumption(snapshots, ""))
}

func quotaHistoryMetricsFromConsumption(consumption QuotaConsumptionResult) quotaHistoryDerivedMetrics {
	metrics := quotaHistoryDerivedMetrics{ForecastConfidence: "insufficient"}
	quality := &metrics.DataQuality
	if len(consumption.Observations) == 0 {
		return metrics
	}
	observations := consumption.Observations
	quality.ObservedSpanSeconds = observations[len(observations)-1].Snapshot.ObservedAt - observations[0].Snapshot.ObservedAt
	quality.ResetBoundaries = consumption.Summary.ResetBoundaries
	segment := make([]model.ChannelQuotaSnapshot, 0, len(observations))
	for _, observation := range observations {
		snapshot := observation.Snapshot
		quality.SampleCount++
		if snapshot.Status == "unsupported" {
			quality.UnsupportedCount++
			segment = segment[:0]
			continue
		}
		if snapshot.Status != "success" {
			quality.ErrorCount++
			segment = segment[:0]
			continue
		}
		if !QuotaSnapshotUsable(snapshot) {
			quality.InvalidCount++
			segment = segment[:0]
			continue
		}
		quality.SuccessCount++
		if observation.ContinuityBreak {
			segment = segment[:0]
		}
		segment = append(segment, snapshot)
	}
	if len(segment) < 2 {
		return metrics
	}
	first, last := segment[0], segment[len(segment)-1]
	span := last.ObservedAt - first.ObservedAt
	if span <= 0 {
		return metrics
	}
	quality.SpanSeconds = span
	if span < quotaHistoryMinimumRateSpanSeconds {
		return metrics
	}
	rate := (last.Available - first.Available) / (float64(span) / (24 * 60 * 60))
	if !FiniteQuotaValue(rate) {
		return metrics
	}
	metrics.DropRatePerDay = &rate
	if len(segment) >= 3 && span >= 24*60*60 {
		metrics.ForecastConfidence = "high"
	} else {
		metrics.ForecastConfidence = "low"
	}
	if rate >= 0 || last.Available <= 0 {
		return metrics
	}
	daysUntilZero := last.Available / -rate
	if !FiniteQuotaValue(daysUntilZero) || daysUntilZero <= 0 || daysUntilZero > quotaHistoryForecastMaximumDays {
		return metrics
	}
	secondsUntilZero := daysUntilZero * 24 * 60 * 60
	if secondsUntilZero > float64(quotaHistoryForecastMaximumDays*24*60*60) {
		return metrics
	}
	forecast := last.ObservedAt + int64(secondsUntilZero)
	metrics.ForecastZeroAt = &forecast
	return metrics
}

type quotaHistoryPoint struct {
	Timestamp         int64    `json:"timestamp"`
	ObservedAt        int64    `json:"observed_at"`
	Status            string   `json:"status"`
	Available         *float64 `json:"available,omitempty"`
	Used              *float64 `json:"used,omitempty"`
	UsedSource        string   `json:"used_source,omitempty"`
	Total             *float64 `json:"total,omitempty"`
	ResetAt           *int64   `json:"reset_at,omitempty"`
	ErrorCode         string   `json:"error_code,omitempty"`
	EventSource       string   `json:"event_source,omitempty"`
	SampleCount       int      `json:"sample_count"`
	SuccessCount      int      `json:"success_count"`
	FailedCount       int      `json:"failed_count"`
	UnsupportedCount  int      `json:"unsupported_count"`
	Reset             bool     `json:"reset"`
	ContinuityBreak   bool     `json:"continuity_break"`
	Consumption       *float64 `json:"consumption,omitempty"`
	RatePerMinute     *float64 `json:"rate_per_minute,omitempty"`
	PeakRatePerMinute *float64 `json:"peak_rate_per_minute,omitempty"`
	ObservedSeconds   int64    `json:"observed_seconds"`
	IntervalCount     int      `json:"interval_count"`
	Gap               bool     `json:"gap"`
	Recovery          bool     `json:"recovery"`
	BaselineChange    bool     `json:"baseline_change"`
	PeriodStart       int64    `json:"period_start"`
	PeriodEnd         int64    `json:"period_end"`
}

type quotaHistoryBucket struct {
	timestamp         int64
	selected          model.ChannelQuotaSnapshot
	hasSelected       bool
	sampleCount       int
	successCount      int
	failedCount       int
	unsupportedCount  int
	resetCount        int
	continuityBreak   bool
	consumption       *float64
	peakRatePerMinute *float64
	observedSeconds   int64
	intervalCount     int
	gap               bool
	recovery          bool
	baselineChange    bool
}

type quotaHistoryValueSummary struct {
	Start         *float64 `json:"start,omitempty"`
	End           *float64 `json:"end,omitempty"`
	Change        *float64 `json:"change,omitempty"`
	ChangePercent *float64 `json:"change_percent,omitempty"`
	Minimum       *float64 `json:"minimum,omitempty"`
	Maximum       *float64 `json:"maximum,omitempty"`
	Samples       int      `json:"samples"`
}

type quotaHistoryConsumptionSummary = QuotaConsumptionSummary

type quotaHistorySummary struct {
	// Legacy available and forecast fields remain for existing callers. New
	// consumers use Available, Used, Consumption, and the method-specific,
	// reset-aware ETA values under data.analysis.
	StartAvailable     *float64                       `json:"start_available,omitempty"`
	EndAvailable       *float64                       `json:"end_available,omitempty"`
	Change             *float64                       `json:"change,omitempty"`
	ChangePercent      *float64                       `json:"change_percent,omitempty"`
	Minimum            *float64                       `json:"minimum,omitempty"`
	Maximum            *float64                       `json:"maximum,omitempty"`
	Available          quotaHistoryValueSummary       `json:"available"`
	Used               *quotaHistoryValueSummary      `json:"used,omitempty"`
	Total              *quotaHistoryValueSummary      `json:"total,omitempty"`
	Consumption        quotaHistoryConsumptionSummary `json:"consumption"`
	DropRatePerDay     *float64                       `json:"drop_rate_per_day,omitempty"`
	ForecastZeroAt     *int64                         `json:"forecast_zero_at,omitempty"`
	ForecastConfidence string                         `json:"forecast_confidence"`
	DataQuality        quotaHistoryDataQuality        `json:"data_quality"`
}

func quotaHistorySuccessFilter(filter model.ChannelQuotaSnapshotQuery) model.ChannelQuotaSnapshotQuery {
	filter.Sources = nil
	filter.Statuses = []string{"success"}
	return filter
}

func quotaHistoryFailureSources(source string) []string {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil
	}
	sources := []string{source}
	for _, suffix := range []string{"_primary", "_secondary"} {
		if base := strings.TrimSuffix(source, suffix); base != source && base != "" {
			sources = append(sources, base)
			break
		}
	}
	return sources
}

func quotaHistoryEventFilter(identity model.ChannelQuotaSnapshotQuery) model.ChannelQuotaSnapshotQuery {
	return model.ChannelQuotaSnapshotQuery{
		EventMetadata: true,
		MetricType:    identity.MetricType,
		WindowType:    identity.WindowType,
		PlanType:      identity.PlanType,
		Unit:          identity.Unit,
		Currency:      identity.Currency,
		WindowSeconds: identity.WindowSeconds,
		Sources:       quotaHistoryFailureSources(identity.Source),
		Statuses:      []string{"error", "unsupported"},
	}
}

func quotaHistorySeriesID(identity model.ChannelQuotaSnapshotQuery) string {
	parts := []string{
		identity.MetricType,
		identity.WindowType,
		identity.Source,
		identity.PlanType,
		identity.Unit,
		identity.Currency,
	}
	if identity.WindowSeconds != nil {
		parts = append(parts, strconv.FormatInt(*identity.WindowSeconds, 10))
	} else {
		parts = append(parts, "")
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "quota_" + hex.EncodeToString(digest[:12])
}

func quotaHistoryLatestSnapshot(left, right *model.ChannelQuotaSnapshot) *model.ChannelQuotaSnapshot {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if right.ObservedAt > left.ObservedAt || (right.ObservedAt == left.ObservedAt && right.Id > left.Id) {
		return right
	}
	return left
}

func quotaHistoryLastRow(rows []model.ChannelQuotaSnapshot) *model.ChannelQuotaSnapshot {
	if len(rows) == 0 {
		return nil
	}
	row := rows[len(rows)-1]
	return &row
}

func mergeQuotaHistorySnapshots(left, right []model.ChannelQuotaSnapshot) []model.ChannelQuotaSnapshot {
	merged := make([]model.ChannelQuotaSnapshot, 0, len(left)+len(right))
	merged = append(merged, left...)
	merged = append(merged, right...)
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].ObservedAt == merged[j].ObservedAt {
			return merged[i].Id < merged[j].Id
		}
		return merged[i].ObservedAt < merged[j].ObservedAt
	})
	return merged
}

func QuotaHistorySnapshotStatus(snapshot model.ChannelQuotaSnapshot) string {
	if snapshot.Status == "success" && !QuotaSnapshotUsable(snapshot) {
		return "error"
	}
	if snapshot.Status == "" {
		return "unknown"
	}
	return snapshot.Status
}

func quotaHistorySnapshotUsed(snapshot model.ChannelQuotaSnapshot) (float64, string, bool) {
	if snapshot.Used != nil && FiniteQuotaValue(*snapshot.Used) {
		return *snapshot.Used, "reported", true
	}
	if snapshot.Total == nil || !FiniteQuotaValue(*snapshot.Total) || !FiniteQuotaValue(snapshot.Available) {
		return 0, "", false
	}
	used := *snapshot.Total - snapshot.Available
	if !FiniteQuotaValue(used) || used < 0 {
		return 0, "", false
	}
	return used, "derived", true
}

func quotaHistoryFloatPointer(value float64) *float64 {
	return &value
}

func quotaHistoryInt64Pointer(value int64) *int64 {
	return &value
}

func quotaHistorySnapshotIsNewer(candidate, selected model.ChannelQuotaSnapshot) bool {
	return candidate.ObservedAt > selected.ObservedAt ||
		(candidate.ObservedAt == selected.ObservedAt && candidate.Id >= selected.Id)
}

func (bucket *quotaHistoryBucket) add(snapshot model.ChannelQuotaSnapshot, reset bool) {
	bucket.sampleCount++
	status := QuotaHistorySnapshotStatus(snapshot)
	switch status {
	case "success":
		bucket.successCount++
	case "unsupported":
		bucket.unsupportedCount++
		bucket.continuityBreak = true
	default:
		bucket.failedCount++
		bucket.continuityBreak = true
	}
	if reset {
		bucket.resetCount++
		bucket.continuityBreak = true
	}
	if !bucket.hasSelected || quotaHistorySnapshotIsNewer(snapshot, bucket.selected) {
		bucket.selected = snapshot
		bucket.hasSelected = true
	}
}

func quotaHistoryPointFromBucket(bucket quotaHistoryBucket, seriesSource string) quotaHistoryPoint {
	snapshot := bucket.selected
	status := QuotaHistorySnapshotStatus(snapshot)
	point := quotaHistoryPoint{
		Timestamp:         bucket.timestamp,
		ObservedAt:        snapshot.ObservedAt,
		Status:            status,
		SampleCount:       bucket.sampleCount,
		SuccessCount:      bucket.successCount,
		FailedCount:       bucket.failedCount,
		UnsupportedCount:  bucket.unsupportedCount,
		Reset:             bucket.resetCount > 0,
		ContinuityBreak:   bucket.continuityBreak,
		Consumption:       bucket.consumption,
		PeakRatePerMinute: bucket.peakRatePerMinute,
		ObservedSeconds:   bucket.observedSeconds,
		IntervalCount:     bucket.intervalCount,
		Gap:               bucket.gap,
		Recovery:          bucket.recovery,
		BaselineChange:    bucket.baselineChange,
		PeriodStart:       bucket.timestamp,
		PeriodEnd:         snapshot.ObservedAt,
	}
	if bucket.consumption != nil && bucket.observedSeconds > 0 {
		point.RatePerMinute = quotaHistoryFloatPointer(*bucket.consumption / (float64(bucket.observedSeconds) / 60))
	}
	if status == "success" {
		point.Available = quotaHistoryFloatPointer(snapshot.Available)
		if used, source, ok := quotaHistorySnapshotUsed(snapshot); ok {
			point.Used = quotaHistoryFloatPointer(used)
			point.UsedSource = source
		}
		if snapshot.Total != nil && FiniteQuotaValue(*snapshot.Total) {
			point.Total = quotaHistoryFloatPointer(*snapshot.Total)
		}
	} else {
		point.ErrorCode = snapshot.ErrorCode
		if point.ErrorCode == "" && snapshot.Status == "success" {
			point.ErrorCode = "invalid_balance"
		}
	}
	if snapshot.ResetAt > 0 {
		point.ResetAt = quotaHistoryInt64Pointer(snapshot.ResetAt)
	}
	if snapshot.Source != "" && snapshot.Source != seriesSource {
		point.EventSource = snapshot.Source
	}
	return point
}

func buildQuotaHistoryPoints(snapshots []model.ChannelQuotaSnapshot, granularity QuotaHistoryGranularity, timezoneOffset int, seriesSource string) []quotaHistoryPoint {
	return quotaHistoryPointsFromObservations(DeriveQuotaConsumption(snapshots, "").Observations, granularity, timezoneOffset, seriesSource)
}

func quotaHistoryPointsFromObservations(observations []QuotaConsumptionObservation, granularity QuotaHistoryGranularity, timezoneOffset int, seriesSource string) []quotaHistoryPoint {
	if len(observations) == 0 {
		return []quotaHistoryPoint{}
	}
	if granularity == QuotaHistoryRaw {
		points := make([]quotaHistoryPoint, 0, len(observations))
		for _, observation := range observations {
			bucket := quotaHistoryBucket{timestamp: observation.Snapshot.ObservedAt}
			bucket.addObservation(observation)
			points = append(points, quotaHistoryPointFromBucket(bucket, seriesSource))
		}
		return points
	}
	buckets := make(map[int64]*quotaHistoryBucket, len(observations))
	for _, observation := range observations {
		timestamp := quotaHistoryBucketStart(observation.Snapshot.ObservedAt, granularity, timezoneOffset)
		bucket, ok := buckets[timestamp]
		if !ok {
			bucket = &quotaHistoryBucket{timestamp: timestamp}
			buckets[timestamp] = bucket
		}
		bucket.addObservation(observation)
	}
	timestamps := make([]int64, 0, len(buckets))
	for timestamp := range buckets {
		timestamps = append(timestamps, timestamp)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
	points := make([]quotaHistoryPoint, 0, len(timestamps))
	for _, timestamp := range timestamps {
		points = append(points, quotaHistoryPointFromBucket(*buckets[timestamp], seriesSource))
	}
	return points
}

func (bucket *quotaHistoryBucket) addObservation(observation QuotaConsumptionObservation) {
	bucket.add(observation.Snapshot, observation.Reset)
	bucket.continuityBreak = bucket.continuityBreak || observation.ContinuityBreak
	bucket.gap = bucket.gap || observation.Gap
	bucket.recovery = bucket.recovery || observation.Recovery
	bucket.baselineChange = bucket.baselineChange || observation.BaselineChange
	if observation.Consumption == nil {
		return
	}
	if bucket.consumption == nil {
		bucket.consumption = quotaHistoryFloatPointer(0)
	}
	*bucket.consumption += *observation.Consumption
	bucket.observedSeconds += observation.ObservedSeconds
	bucket.intervalCount++
	if observation.RatePerMinute != nil && (bucket.peakRatePerMinute == nil || *observation.RatePerMinute > *bucket.peakRatePerMinute) {
		bucket.peakRatePerMinute = quotaHistoryFloatPointer(*observation.RatePerMinute)
	}
}

func limitQuotaHistoryPoints(points []quotaHistoryPoint, limit int) ([]quotaHistoryPoint, bool) {
	if len(points) <= limit {
		return points, false
	}
	// Compact contiguous groups rather than selecting isolated points. All
	// interval consumption and failure/reset markers survive the point budget.
	limited := make([]quotaHistoryPoint, 0, limit)
	for index := 0; index < limit; index++ {
		start, end := index*len(points)/limit, (index+1)*len(points)/limit
		point := points[end-1]
		point.PeriodStart = points[start].PeriodStart
		point.Consumption = nil
		point.RatePerMinute = nil
		point.PeakRatePerMinute = nil
		point.ObservedSeconds, point.IntervalCount = 0, 0
		point.SampleCount, point.SuccessCount, point.FailedCount, point.UnsupportedCount = 0, 0, 0, 0
		for _, member := range points[start:end] {
			point.SampleCount += member.SampleCount
			point.SuccessCount += member.SuccessCount
			point.FailedCount += member.FailedCount
			point.UnsupportedCount += member.UnsupportedCount
			point.Reset = point.Reset || member.Reset
			point.ContinuityBreak = point.ContinuityBreak || member.ContinuityBreak
			point.Gap = point.Gap || member.Gap
			point.Recovery = point.Recovery || member.Recovery
			point.BaselineChange = point.BaselineChange || member.BaselineChange
			if member.Consumption != nil {
				if point.Consumption == nil {
					point.Consumption = quotaHistoryFloatPointer(0)
				}
				*point.Consumption += *member.Consumption
				point.ObservedSeconds += member.ObservedSeconds
				point.IntervalCount += member.IntervalCount
			}
			if member.PeakRatePerMinute != nil && (point.PeakRatePerMinute == nil || *member.PeakRatePerMinute > *point.PeakRatePerMinute) {
				point.PeakRatePerMinute = quotaHistoryFloatPointer(*member.PeakRatePerMinute)
			}
		}
		if point.Consumption != nil && point.ObservedSeconds > 0 {
			point.RatePerMinute = quotaHistoryFloatPointer(*point.Consumption / (float64(point.ObservedSeconds) / 60))
		}
		limited = append(limited, point)
	}
	return limited, true
}

func latestContinuousQuotaHistorySegment(observations []QuotaConsumptionObservation) []model.ChannelQuotaSnapshot {
	segment := make([]model.ChannelQuotaSnapshot, 0, len(observations))
	var latestSegment []model.ChannelQuotaSnapshot
	for _, observation := range observations {
		snapshot := observation.Snapshot
		if QuotaHistorySnapshotStatus(snapshot) != "success" {
			if len(segment) > 0 {
				latestSegment = append([]model.ChannelQuotaSnapshot(nil), segment...)
			}
			segment = nil
			continue
		}
		if observation.ContinuityBreak {
			segment = nil
		}
		segment = append(segment, snapshot)
	}
	if len(segment) == 0 {
		return latestSegment
	}
	return segment
}

func quotaHistoryValueSummaryFor(snapshots []model.ChannelQuotaSnapshot, value func(model.ChannelQuotaSnapshot) (float64, bool)) quotaHistoryValueSummary {
	summary := quotaHistoryValueSummary{}
	for _, snapshot := range snapshots {
		if QuotaHistorySnapshotStatus(snapshot) != "success" {
			continue
		}
		current, ok := value(snapshot)
		if !ok || !FiniteQuotaValue(current) {
			continue
		}
		summary.Samples++
		if summary.Start == nil {
			summary.Start = quotaHistoryFloatPointer(current)
			summary.Minimum = quotaHistoryFloatPointer(current)
			summary.Maximum = quotaHistoryFloatPointer(current)
		}
		summary.End = quotaHistoryFloatPointer(current)
		if current < *summary.Minimum {
			summary.Minimum = quotaHistoryFloatPointer(current)
		}
		if current > *summary.Maximum {
			summary.Maximum = quotaHistoryFloatPointer(current)
		}
	}
	if summary.Start != nil && summary.End != nil {
		change := *summary.End - *summary.Start
		summary.Change = quotaHistoryFloatPointer(change)
		if *summary.Start != 0 {
			percent := change / *summary.Start * 100
			summary.ChangePercent = quotaHistoryFloatPointer(percent)
		}
	}
	return summary
}

func deriveQuotaHistorySummary(snapshots []model.ChannelQuotaSnapshot, metrics quotaHistoryDerivedMetrics, unit string) *quotaHistorySummary {
	return quotaHistorySummaryFromConsumption(DeriveQuotaConsumption(snapshots, unit), metrics)
}

func quotaHistorySummaryFromConsumption(consumption QuotaConsumptionResult, metrics quotaHistoryDerivedMetrics) *quotaHistorySummary {
	segment := latestContinuousQuotaHistorySegment(consumption.Observations)
	available := quotaHistoryValueSummaryFor(segment, func(snapshot model.ChannelQuotaSnapshot) (float64, bool) {
		return snapshot.Available, FiniteQuotaValue(snapshot.Available)
	})
	if len(consumption.Observations) == 0 {
		return nil
	}
	used := quotaHistoryValueSummaryFor(segment, func(snapshot model.ChannelQuotaSnapshot) (float64, bool) {
		value, _, ok := quotaHistorySnapshotUsed(snapshot)
		return value, ok
	})
	total := quotaHistoryValueSummaryFor(segment, func(snapshot model.ChannelQuotaSnapshot) (float64, bool) {
		if snapshot.Total == nil || !FiniteQuotaValue(*snapshot.Total) {
			return 0, false
		}
		return *snapshot.Total, true
	})
	summary := &quotaHistorySummary{
		StartAvailable:     available.Start,
		EndAvailable:       available.End,
		Change:             available.Change,
		ChangePercent:      available.ChangePercent,
		Minimum:            available.Minimum,
		Maximum:            available.Maximum,
		Available:          available,
		Consumption:        consumption.Summary,
		DropRatePerDay:     metrics.DropRatePerDay,
		ForecastZeroAt:     metrics.ForecastZeroAt,
		ForecastConfidence: metrics.ForecastConfidence,
		DataQuality:        metrics.DataQuality,
	}
	if used.Samples > 0 {
		summary.Used = &used
	}
	if total.Samples > 0 {
		summary.Total = &total
	}
	return summary
}

func quotaHistoryCurrent(snapshot *model.ChannelQuotaSnapshot, seriesSource string) map[string]any {
	if snapshot == nil {
		return nil
	}
	status := QuotaHistorySnapshotStatus(*snapshot)
	current := map[string]any{"observed_at": snapshot.ObservedAt, "status": status}
	if snapshot.Source != "" && snapshot.Source != seriesSource {
		current["event_source"] = snapshot.Source
	}
	if status != "success" {
		errorCode := snapshot.ErrorCode
		if errorCode == "" && snapshot.Status == "success" {
			errorCode = "invalid_balance"
		}
		current["error_code"] = errorCode
		return current
	}
	current["available"] = snapshot.Available
	if used, source, ok := quotaHistorySnapshotUsed(*snapshot); ok {
		current["used"] = used
		current["used_source"] = source
	}
	if snapshot.Total != nil && FiniteQuotaValue(*snapshot.Total) {
		current["total"] = *snapshot.Total
	}
	if snapshot.ResetAt > 0 {
		current["reset_at"] = snapshot.ResetAt
	}
	return current
}

func quotaHistoryMetadata(response map[string]any, identity model.ChannelQuotaSnapshotQuery) {
	if identity.MetricType == "" {
		return
	}
	response["series_id"] = quotaHistorySeriesID(identity)
	response["metric_type"] = identity.MetricType
	response["window_type"] = identity.WindowType
	response["source"] = identity.Source
	response["plan_type"] = identity.PlanType
	response["unit"] = identity.Unit
	response["currency"] = identity.Currency
	if identity.WindowSeconds != nil {
		response["window_seconds"] = *identity.WindowSeconds
	}
	response["series"] = map[string]any{
		"id":             response["series_id"],
		"metric_type":    identity.MetricType,
		"window_type":    identity.WindowType,
		"source":         identity.Source,
		"plan_type":      identity.PlanType,
		"unit":           identity.Unit,
		"currency":       identity.Currency,
		"window_seconds": response["window_seconds"],
	}
}

// ChannelQuotaHistoryRequest contains the normalized, bounded parameters
// accepted by the HTTP handler. It does not carry credentials or transport state.
type ChannelQuotaHistoryRequest struct {
	ChannelID            int
	Start                int64
	End                  int64
	Limit                int
	Granularity          QuotaHistoryGranularity
	RequestedGranularity string
	TimezoneOffset       int
	RateWindowSeconds    int64
	EWMAHalfLifeSeconds  int64
	SeriesFilter         model.ChannelQuotaSnapshotQuery
}

// GetChannelQuotaHistory reads stored observations and builds the existing
// history response without querying providers or changing persisted state.
func GetChannelQuotaHistory(ctx context.Context, request ChannelQuotaHistoryRequest) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	id, start, end := request.ChannelID, request.Start, request.End
	limit, granularity := request.Limit, request.Granularity
	timezoneOffset := request.TimezoneOffset
	analysisStart := end - request.RateWindowSeconds
	ewmaHalfLifeSeconds := request.EWMAHalfLifeSeconds
	seriesFilter := request.SeriesFilter
	// Resolve the chart identity from the newest successful sample, not a
	// generic failure marker. A current failure is queried separately below and
	// still wins the current/alert state, while the historical line remains the
	// actual provider window that operators selected.
	successCandidateFilter := quotaHistorySuccessFilter(seriesFilter)
	latestSuccessRows, err := model.ListChannelQuotaSnapshotsWithQuery(id, start, end, successCandidateFilter, 1)
	if err != nil {
		return nil, err
	}
	identity := seriesFilter
	latestSuccess := quotaHistoryLastRow(latestSuccessRows)
	if latestSuccess != nil {
		identity = resolveQuotaHistorySeriesFilter(identity, *latestSuccess)
	}
	identity.Sources = nil
	identity.Statuses = nil

	if latestSuccess == nil {
		latestRows, latestErr := model.ListChannelQuotaSnapshotsWithQuery(id, start, end, seriesFilter, 1)
		if latestErr != nil {
			return nil, latestErr
		}
		if latest := quotaHistoryLastRow(latestRows); latest != nil {
			identity = resolveQuotaHistorySeriesFilter(identity, *latest)
			identity.Sources = nil
			identity.Statuses = nil
		}
	}

	response := map[string]any{
		"channel_id":            id,
		"start":                 start,
		"end":                   end,
		"limit":                 limit,
		"granularity":           string(granularity),
		"requested_granularity": request.RequestedGranularity,
		"timezone_offset":       timezoneOffset,
		"raw_observation_limit": MaxQuotaHistoryRawObservations,
		"points":                []quotaHistoryPoint{},
		"analysis":              AnalyzeQuotaConsumption(QuotaConsumptionResult{}, analysisStart, end, ewmaHalfLifeSeconds),
	}
	quotaHistoryMetadata(response, identity)
	if response["requested_granularity"] == "" {
		response["requested_granularity"] = string(QuotaHistoryRaw)
	}

	if identity.MetricType == "" {
		response["raw_observations"] = 0
		response["available_points"] = 0
		response["returned_points"] = 0
		response["source_complete"] = true
		response["points_complete"] = true
		response["complete"] = true
		response["truncated"] = false
		response["alert"] = DeriveQuotaHistoryAlert(nil)
		return response, nil
	}

	successFilter := quotaHistorySuccessFilter(identity)
	eventFilter := quotaHistoryEventFilter(identity)
	requestContext := ctx
	successCount, err := model.CountChannelQuotaSnapshotsWithQuery(requestContext, id, start, end, successFilter)
	if err != nil {
		return nil, err
	}
	eventCount, err := model.CountChannelQuotaSnapshotsWithQuery(requestContext, id, start, end, eventFilter)
	if err != nil {
		return nil, err
	}
	rawObservations := successCount + eventCount
	response["raw_observations"] = rawObservations

	latestEventRows, err := model.ListChannelQuotaSnapshotsWithQuery(id, start, end, eventFilter, 1)
	if err != nil {
		return nil, err
	}
	latestSnapshot := quotaHistoryLatestSnapshot(latestSuccess, quotaHistoryLastRow(latestEventRows))
	response["current"] = quotaHistoryCurrent(latestSnapshot, identity.Source)
	response["alert"] = DeriveQuotaHistoryAlert(latestSnapshot)

	if rawObservations > MaxQuotaHistoryRawObservations {
		analysis := response["analysis"].(QuotaAnalysis)
		analysis.Complete = false
		response["analysis"] = analysis
		response["available_points"] = 0
		response["returned_points"] = 0
		response["source_complete"] = false
		response["points_complete"] = false
		response["complete"] = false
		response["truncated"] = true
		response["truncation_reason"] = "raw_observation_limit"
		response["resolution_hint"] = "Use a shorter time range."
		return response, nil
	}

	successes, err := model.ListChannelQuotaSnapshotsForHistory(requestContext, id, start, end, successFilter, MaxQuotaHistoryRawObservations)
	if err != nil {
		return nil, err
	}
	events, err := model.ListChannelQuotaSnapshotsForHistory(requestContext, id, start, end, eventFilter, MaxQuotaHistoryRawObservations)
	if err != nil {
		return nil, err
	}
	if !successes.Complete || !events.Complete {
		// A concurrent sampler can add rows between Count and List. Do not use
		// the partial prefix from either query as a history range.
		analysis := response["analysis"].(QuotaAnalysis)
		analysis.Complete = false
		response["analysis"] = analysis
		response["available_points"] = 0
		response["returned_points"] = 0
		response["source_complete"] = false
		response["points_complete"] = false
		response["complete"] = false
		response["truncated"] = true
		response["truncation_reason"] = "raw_observation_limit"
		response["resolution_hint"] = "Use a shorter time range."
		return response, nil
	}

	snapshots := mergeQuotaHistorySnapshots(successes.Snapshots, events.Snapshots)
	// Count/list are separate read-only queries; report the actual stable end of
	// the loaded range if a sampler appended a same-second observation meanwhile.
	response["raw_observations"] = len(snapshots)
	latestSnapshot = quotaHistoryLatestSnapshot(latestSnapshot, quotaHistoryLastRow(snapshots))
	response["current"] = quotaHistoryCurrent(latestSnapshot, identity.Source)
	response["alert"] = DeriveQuotaHistoryAlert(latestSnapshot)
	consumption := DeriveQuotaConsumption(snapshots, identity.Unit)
	response["analysis"] = AnalyzeQuotaConsumption(consumption, analysisStart, end, ewmaHalfLifeSeconds)
	metrics := quotaHistoryMetricsFromConsumption(consumption)
	points := quotaHistoryPointsFromObservations(consumption.Observations, granularity, timezoneOffset, identity.Source)
	availablePoints := len(points)
	points, pointTruncated := limitQuotaHistoryPoints(points, limit)
	response["points"] = points
	response["available_points"] = availablePoints
	response["returned_points"] = len(points)
	response["source_complete"] = true
	response["points_complete"] = !pointTruncated
	response["complete"] = !pointTruncated
	response["truncated"] = pointTruncated
	if pointTruncated {
		response["truncation_reason"] = "point_limit"
	}
	response["data_quality"] = metrics.DataQuality
	if summary := quotaHistorySummaryFromConsumption(consumption, metrics); summary != nil {
		response["summary"] = summary
	}
	return response, nil
}
