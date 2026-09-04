package controller

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"

	"github.com/gin-gonic/gin"
)

const maxQuotaChangesLimit = 2000
const maxQuotaOverviewPoints = 120

// GetChannelQuotaSamplingStatus exposes only the effective sampler settings so
// administrators can understand an empty trend panel without inspecting the
// deployment environment. It never starts a task or returns credentials.
func GetChannelQuotaSamplingStatus(c *gin.Context) {
	handler := channelQuotaSnapshotSyncHandler{}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":          handler.Enabled(),
			"interval_seconds": int64(handler.Interval() / time.Second),
			"max_channels":     channelQuotaSnapshotSyncMaxChannelsConfigured(),
		},
	})
}

type quotaChangeDataQuality struct {
	SuccessCount     int   `json:"success_count"`
	ErrorCount       int   `json:"error_count"`
	UnsupportedCount int   `json:"unsupported_count,omitempty"`
	InvalidCount     int   `json:"invalid_count"`
	ResetBoundaries  int   `json:"reset_boundaries"`
	SpanSeconds      int64 `json:"span_seconds"`
}

type quotaChangeItem struct {
	ChannelID              int                             `json:"channel_id"`
	Name                   string                          `json:"name"`
	AccountLabel           string                          `json:"account_label"`
	MetricType             string                          `json:"metric_type"`
	WindowType             string                          `json:"window_type"`
	Source                 string                          `json:"source,omitempty"`
	Unit                   string                          `json:"unit"`
	Currency               string                          `json:"currency,omitempty"`
	PlanType               string                          `json:"plan_type,omitempty"`
	WindowSeconds          int64                           `json:"window_seconds,omitempty"`
	CurrentAvailable       *float64                        `json:"current_available,omitempty"`
	CurrentTotal           *float64                        `json:"current_total,omitempty"`
	PreviousAvailable      *float64                        `json:"previous_available,omitempty"`
	ChangePerMinute        *float64                        `json:"change_per_minute,omitempty"`
	AbsChangePerMinute     *float64                        `json:"abs_change_per_minute,omitempty"`
	Direction              string                          `json:"direction"`
	SampleSpanSeconds      int64                           `json:"sample_span_seconds"`
	ObservedAt             int64                           `json:"observed_at"`
	Status                 string                          `json:"status"`
	Consumption            service.QuotaConsumptionSummary `json:"consumption"`
	Analysis               *service.QuotaAnalysis          `json:"analysis,omitempty"`
	OverviewPoints         *[]service.QuotaOverviewPoint   `json:"overview_points,omitempty"`
	PeakAbsChangePerMinute *float64                        `json:"peak_abs_change_per_minute,omitempty"`
	PeakDropPerMinute      *float64                        `json:"peak_drop_per_minute,omitempty"`
	PeakIncreasePerMinute  *float64                        `json:"peak_increase_per_minute,omitempty"`
	// Alert is a read-only snapshot of the configured quota threshold state.
	// It is deliberately derived from the latest normalized observation and
	// never triggers notification, routing, or channel state changes.
	Alert       *quotaHistoryAlert     `json:"alert,omitempty"`
	DataQuality quotaChangeDataQuality `json:"data_quality"`

	analysisRows []model.ChannelQuotaAggregateRow
}

type quotaChangeGroupKey struct {
	ChannelID     int
	MetricType    string
	WindowType    string
	Source        string
	PlanType      string
	Unit          string
	Currency      string
	WindowSeconds int64
}

// GetChannelQuotaChanges returns one redacted trend item per channel/metric/
// window/source group. It is intentionally read-only: no provider is queried
// and no credentials or raw provider responses are returned.
func GetChannelQuotaChanges(c *gin.Context) {
	now := time.Now().Unix()
	start, end := now-24*60*60, now
	rangeName := strings.TrimSpace(c.Query("range"))
	if rangeName == "" {
		rangeName = "24h"
	}
	if seconds, ok := quotaChangeRangeSeconds(rangeName); ok {
		start = end - seconds
	} else if !strings.EqualFold(rangeName, "custom") {
		common.ApiError(c, errors.New("invalid range; use 1h, 6h, 24h, 7d, 30d, 90d, or custom"))
		return
	}
	for key, target := range map[string]*int64{"start": &start, "end": &end} {
		value := strings.TrimSpace(c.Query(key))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			parsedTime, timeErr := time.Parse(time.RFC3339, value)
			if timeErr != nil {
				common.ApiError(c, errors.New("invalid "+key+" timestamp"))
				return
			}
			parsed = parsedTime.Unix()
		}
		*target = parsed
	}
	if start < 0 || end < start || end-start > 180*24*60*60 {
		common.ApiError(c, errors.New("invalid quota changes time range"))
		return
	}
	rateWindowSeconds, err := parseQuotaAnalysisDuration(c.Query("rate_window"), end-start, "rate_window")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ewmaHalfLifeSeconds := defaultQuotaAnalysisHalfLife(rateWindowSeconds)
	if value := strings.TrimSpace(c.Query("ewma_half_life")); value != "" {
		ewmaHalfLifeSeconds, err = parseQuotaAnalysisDuration(value, rateWindowSeconds, "ewma_half_life")
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	analysisStart := end - rateWindowSeconds
	limit := 20
	if value := strings.TrimSpace(c.Query("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			common.ApiError(c, errors.New("invalid quota changes limit"))
			return
		}
		limit = parsed
	}
	if limit > maxQuotaChangesLimit {
		limit = maxQuotaChangesLimit
	}
	overviewPointLimit := 0
	if value := strings.TrimSpace(c.Query("overview_points")); value != "" {
		overviewPointLimit, err = strconv.Atoi(value)
		if err != nil || overviewPointLimit <= 0 || overviewPointLimit > maxQuotaOverviewPoints {
			common.ApiError(c, fmt.Errorf("overview_points must be between 1 and %d", maxQuotaOverviewPoints))
			return
		}
	}
	channelIDs, err := parseQuotaChangeChannelIDs(c.Query("channel_ids"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	requestContext := context.Background()
	if c.Request != nil && c.Request.Context() != nil {
		requestContext = c.Request.Context()
	}
	rows, err := model.ListChannelQuotaAggregateRows(requestContext, start, end, channelIDs,
		strings.TrimSpace(c.Query("metric_type")), strings.TrimSpace(c.Query("window_type")), strings.TrimSpace(c.Query("source")))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items, quality := buildQuotaChangeItemsLightweight(rows)
	summary := quotaChangeSummary(items)
	totalItems := len(items)
	items = finalizeQuotaChangeItems(items, c.Query("sort"), limit, analysisStart, end, ewmaHalfLifeSeconds, overviewPointLimit, service.AnalyzeQuotaConsumption)
	response := gin.H{
		"items":                  items,
		"range":                  rangeName,
		"start":                  start,
		"end":                    end,
		"generated_at":           now,
		"rate_window_seconds":    rateWindowSeconds,
		"ewma_half_life_seconds": ewmaHalfLifeSeconds,
		"data_quality":           quality,
		"total_items":            totalItems,
		"returned_items":         len(items),
		"source_complete":        true,
		"items_complete":         totalItems == len(items),
	}
	response["summary"] = summary
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

func quotaChangeRangeSeconds(value string) (int64, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1h":
		return 60 * 60, true
	case "6h":
		return 6 * 60 * 60, true
	case "24h", "1d":
		return 24 * 60 * 60, true
	case "7d":
		return 7 * 24 * 60 * 60, true
	case "30d":
		return 30 * 24 * 60 * 60, true
	case "90d":
		return 90 * 24 * 60 * 60, true
	default:
		return 0, false
	}
}

func parseQuotaChangeChannelIDs(value string) ([]int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	ids := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 {
			return nil, errors.New("invalid channel_ids")
		}
		if _, ok := seen[id]; !ok {
			ids = append(ids, id)
			seen[id] = struct{}{}
		}
	}
	return ids, nil
}

func finiteQuotaChangeValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func buildQuotaChangeItems(rows []model.ChannelQuotaAggregateRow) ([]quotaChangeItem, quotaChangeDataQuality) {
	if len(rows) == 0 {
		return []quotaChangeItem{}, quotaChangeDataQuality{}
	}
	start, end := rows[0].ObservedAt, rows[0].ObservedAt
	for _, row := range rows[1:] {
		if row.ObservedAt < start {
			start = row.ObservedAt
		}
		if row.ObservedAt > end {
			end = row.ObservedAt
		}
	}
	return buildQuotaChangeItemsWithAnalysis(rows, start, end, defaultQuotaAnalysisHalfLife(end-start))
}

func buildQuotaChangeItemsWithAnalysis(rows []model.ChannelQuotaAggregateRow, analysisStart, analysisEnd, ewmaHalfLifeSeconds int64) ([]quotaChangeItem, quotaChangeDataQuality) {
	items, quality := buildQuotaChangeItemsLightweight(rows)
	attachQuotaChangeAnalysis(items, analysisStart, analysisEnd, ewmaHalfLifeSeconds, 0, service.AnalyzeQuotaConsumption)
	return items, quality
}

func buildQuotaChangeItemsLightweight(rows []model.ChannelQuotaAggregateRow) ([]quotaChangeItem, quotaChangeDataQuality) {
	groups := make(map[quotaChangeGroupKey][]model.ChannelQuotaAggregateRow)
	keyFor := func(row model.ChannelQuotaAggregateRow) quotaChangeGroupKey {
		return quotaChangeGroupKey{row.ChannelID, row.MetricType, row.WindowType, row.Source, row.PlanType, row.Unit, row.Currency, row.WindowSeconds}
	}
	global := quotaChangeDataQuality{}
	for _, row := range rows {
		// Keep provider account series independent. A channel may expose more
		// than one subscription plan/window, and unit or currency changes must
		// never be interpreted as a balance movement within one series.
		switch {
		case row.Status == "unsupported":
			global.UnsupportedCount++
		case row.Status != "success":
			global.ErrorCount++
		case !service.QuotaSnapshotUsable(quotaChangeSnapshot(row)):
			global.InvalidCount++
		default:
			global.SuccessCount++
		}
		if row.Status != "success" {
			continue
		}
		key := keyFor(row)
		groups[key] = append(groups[key], row)
	}
	// A generic provider failure has no plan/window identity. Attach it as an
	// event to known matching series; it is not an additional account.
	type accountKey struct {
		channelID  int
		metricType string
	}
	accounts := make(map[accountKey][]quotaChangeGroupKey)
	for key := range groups {
		account := accountKey{key.ChannelID, key.MetricType}
		accounts[account] = append(accounts[account], key)
	}
	orphanEvents := make(map[quotaChangeGroupKey][]model.ChannelQuotaAggregateRow)
	for _, row := range rows {
		if row.Status == "success" {
			continue
		}
		matched := false
		for _, key := range accounts[accountKey{row.ChannelID, row.MetricType}] {
			if key.Source != row.Source && !(strings.TrimSuffix(key.Source, "_primary") == row.Source || strings.TrimSuffix(key.Source, "_secondary") == row.Source) {
				continue
			}
			if row.WindowType != "" && row.WindowType != "none" && key.WindowType != row.WindowType {
				continue
			}
			if row.PlanType != "" && key.PlanType != row.PlanType {
				continue
			}
			if row.Unit != "" && key.Unit != row.Unit {
				continue
			}
			if row.Currency != "" && key.Currency != row.Currency {
				continue
			}
			if row.WindowSeconds != 0 && key.WindowSeconds != row.WindowSeconds {
				continue
			}
			groups[key] = append(groups[key], row)
			matched = true
		}
		if !matched {
			key := keyFor(row)
			orphanEvents[key] = append(orphanEvents[key], row)
		}
	}
	for key, events := range orphanEvents {
		groups[key] = append(groups[key], events...)
	}
	items := make([]quotaChangeItem, 0, len(groups))
	for _, groupRows := range groups {
		sort.SliceStable(groupRows, func(i, j int) bool {
			if groupRows[i].ObservedAt == groupRows[j].ObservedAt {
				return groupRows[i].ID < groupRows[j].ID
			}
			return groupRows[i].ObservedAt < groupRows[j].ObservedAt
		})
		item := buildQuotaChangeItem(groupRows)
		items = append(items, item)
		global.ResetBoundaries += item.DataQuality.ResetBoundaries
		if item.DataQuality.SpanSeconds > global.SpanSeconds {
			global.SpanSeconds = item.DataQuality.SpanSeconds
		}
	}
	return items, global
}

func buildQuotaChangeItem(rows []model.ChannelQuotaAggregateRow) quotaChangeItem {
	latest := rows[len(rows)-1]
	identity := latest
	snapshots := make([]model.ChannelQuotaSnapshot, 0, len(rows))
	for _, row := range rows {
		snapshots = append(snapshots, quotaChangeSnapshot(row))
		if row.Status == "success" {
			identity = row
		}
	}
	consumption := service.DeriveQuotaConsumption(snapshots, identity.Unit)
	name := strings.TrimSpace(identity.ChannelName)
	if name == "" {
		name = "Channel #" + strconv.Itoa(latest.ChannelID)
	}
	item := quotaChangeItem{
		ChannelID: latest.ChannelID, Name: name, AccountLabel: name,
		MetricType: identity.MetricType, WindowType: identity.WindowType, Source: identity.Source,
		Unit: identity.Unit, Currency: identity.Currency, PlanType: identity.PlanType,
		WindowSeconds: identity.WindowSeconds,
		ObservedAt:    latest.ObservedAt, Status: quotaHistorySnapshotStatus(quotaChangeSnapshot(latest)), Direction: "unavailable",
		Consumption:  consumption.Summary,
		analysisRows: rows,
	}
	// Keep the global quota-change view consistent with the channel history
	// endpoint: failed, unsupported, or total-less observations report an
	// unavailable/disabled alert rather than carrying forward stale state.
	item.Alert = quotaChangeAlertForRow(latest)
	if item.Status == "" {
		item.Status = "unknown"
	}
	quality := &item.DataQuality
	quality.ResetBoundaries = consumption.Summary.ResetBoundaries
	quality.SpanSeconds = latest.ObservedAt - rows[0].ObservedAt
	for _, row := range rows {
		if row.Status == "success" {
			if !service.QuotaSnapshotUsable(quotaChangeSnapshot(row)) {
				quality.InvalidCount++
				continue
			}
			quality.SuccessCount++
		} else if row.Status == "unsupported" {
			quality.UnsupportedCount++
		} else {
			quality.ErrorCount++
		}
	}
	for _, observation := range consumption.Observations {
		if observation.AvailableChangePerMinute == nil {
			continue
		}
		rate := *observation.AvailableChangePerMinute
		absRate := math.Abs(rate)
		if item.PeakAbsChangePerMinute == nil || absRate > *item.PeakAbsChangePerMinute {
			item.PeakAbsChangePerMinute = quotaChangeFloatPtr(absRate)
		}
		if rate < 0 && (item.PeakDropPerMinute == nil || rate < *item.PeakDropPerMinute) {
			item.PeakDropPerMinute = quotaChangeFloatPtr(rate)
		}
		if rate > 0 && (item.PeakIncreasePerMinute == nil || rate > *item.PeakIncreasePerMinute) {
			item.PeakIncreasePerMinute = quotaChangeFloatPtr(rate)
		}
	}
	if service.QuotaSnapshotUsable(quotaChangeSnapshot(latest)) {
		item.CurrentAvailable = quotaChangeFloatPtr(latest.Available)
		if latest.Total != nil && finiteQuotaChangeValue(*latest.Total) {
			item.CurrentTotal = quotaChangeFloatPtr(*latest.Total)
		}
	}
	lastObservation := consumption.Observations[len(consumption.Observations)-1]
	if len(rows) > 1 && lastObservation.AvailableChangePerMinute != nil {
		previous := rows[len(rows)-2]
		item.PreviousAvailable = quotaChangeFloatPtr(previous.Available)
		span := latest.ObservedAt - previous.ObservedAt
		item.SampleSpanSeconds = span
		rate := *lastObservation.AvailableChangePerMinute
		item.ChangePerMinute = quotaChangeFloatPtr(rate)
		absRate := math.Abs(rate)
		item.AbsChangePerMinute = quotaChangeFloatPtr(absRate)
		switch {
		case absRate < 1e-12:
			item.Direction = "stable"
		case rate < 0:
			item.Direction = "decrease"
		default:
			item.Direction = "increase"
		}
	} else if item.CurrentAvailable != nil {
		item.Direction = "unknown"
	}
	return item
}

type quotaChangeAnalyzer func(service.QuotaConsumptionResult, int64, int64, int64) service.QuotaAnalysis

func attachQuotaChangeAnalysis(items []quotaChangeItem, analysisStart, analysisEnd, ewmaHalfLifeSeconds int64, overviewPointLimit int, analyze quotaChangeAnalyzer) {
	for index := range items {
		snapshots := make([]model.ChannelQuotaSnapshot, 0, len(items[index].analysisRows))
		for _, row := range items[index].analysisRows {
			snapshots = append(snapshots, quotaChangeSnapshot(row))
		}
		consumption := service.DeriveQuotaConsumption(snapshots, items[index].Unit)
		analysis := analyze(consumption, analysisStart, analysisEnd, ewmaHalfLifeSeconds)
		items[index].Analysis = &analysis
		if overviewPointLimit > 0 {
			points := service.BuildQuotaOverviewPoints(consumption.Observations, overviewPointLimit)
			items[index].OverviewPoints = &points
		}
		items[index].analysisRows = nil
	}
}

func finalizeQuotaChangeItems(items []quotaChangeItem, sortValue string, limit int, analysisStart, analysisEnd, ewmaHalfLifeSeconds int64, overviewPointLimit int, analyze quotaChangeAnalyzer) []quotaChangeItem {
	sortQuotaChangeItems(items, sortValue)
	if limit > 0 && len(items) > limit {
		for index := limit; index < len(items); index++ {
			items[index].Analysis = nil
			items[index].OverviewPoints = nil
			items[index].analysisRows = nil
		}
		items = append([]quotaChangeItem(nil), items[:limit]...)
	}
	attachQuotaChangeAnalysis(items, analysisStart, analysisEnd, ewmaHalfLifeSeconds, overviewPointLimit, analyze)
	return items
}

func quotaChangeSnapshot(row model.ChannelQuotaAggregateRow) model.ChannelQuotaSnapshot {
	return model.ChannelQuotaSnapshot{Id: row.ID, ChannelId: row.ChannelID, ObservedAt: row.ObservedAt,
		Available: row.Available, Used: row.Used, Total: row.Total, Unit: row.Unit, Currency: row.Currency,
		MetricType: row.MetricType, WindowType: row.WindowType, Source: row.Source, PlanType: row.PlanType,
		WindowSeconds: row.WindowSeconds, ResetAt: row.ResetAt, Status: row.Status, ErrorCode: row.ErrorCode}
}

func quotaChangeAlertForRow(row model.ChannelQuotaAggregateRow) *quotaHistoryAlert {
	alert := deriveQuotaHistoryAlert(&model.ChannelQuotaSnapshot{
		Available: row.Available,
		Total:     row.Total,
		Status:    row.Status,
	})
	return &alert
}

func quotaChangeFloatPtr(value float64) *float64 { return &value }

func sortQuotaChangeItems(items []quotaChangeItem, value string) {
	sort.SliceStable(items, func(i, j int) bool {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "change_asc":
			return quotaChangeRate(items[i]) < quotaChangeRate(items[j])
		case "change_desc":
			return quotaChangeRate(items[i]) > quotaChangeRate(items[j])
		case "observed_desc":
			return items[i].ObservedAt > items[j].ObservedAt
		case "channel", "channel_asc":
			if items[i].Name == items[j].Name {
				return items[i].ChannelID < items[j].ChannelID
			}
			return items[i].Name < items[j].Name
		default: // The overview is most useful when the largest movement is first.
			return quotaChangeAbsRate(items[i]) > quotaChangeAbsRate(items[j])
		}
	})
}

func quotaChangeRate(item quotaChangeItem) float64 {
	if item.ChangePerMinute == nil {
		return 0
	}
	return *item.ChangePerMinute
}

func quotaChangeAbsRate(item quotaChangeItem) float64 {
	if item.AbsChangePerMinute == nil {
		return 0
	}
	return *item.AbsChangePerMinute
}

func quotaChangeSummary(items []quotaChangeItem) gin.H {
	var maxAbs, maxDrop, maxIncrease *float64
	units := make(map[string]struct{})
	for _, item := range items {
		if item.PeakAbsChangePerMinute != nil {
			units[item.Unit+"\x00"+item.Currency] = struct{}{}
		}
		if item.PeakAbsChangePerMinute != nil && (maxAbs == nil || *item.PeakAbsChangePerMinute > *maxAbs) {
			maxAbs = quotaChangeFloatPtr(*item.PeakAbsChangePerMinute)
		}
		if item.PeakDropPerMinute != nil && (maxDrop == nil || *item.PeakDropPerMinute < *maxDrop) {
			maxDrop = quotaChangeFloatPtr(*item.PeakDropPerMinute)
		}
		if item.PeakIncreasePerMinute != nil && (maxIncrease == nil || *item.PeakIncreasePerMinute > *maxIncrease) {
			maxIncrease = quotaChangeFloatPtr(*item.PeakIncreasePerMinute)
		}
	}
	// Different currencies/units cannot be ranked as one numeric rate.
	if len(units) > 1 {
		maxAbs, maxDrop, maxIncrease = nil, nil, nil
	}
	return gin.H{"max_abs_change_per_minute": maxAbs, "max_drop_per_minute": maxDrop, "max_increase_per_minute": maxIncrease, "mixed_units": len(units) > 1}
}
