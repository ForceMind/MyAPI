package controller

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"

	"github.com/gin-gonic/gin"
)

const maxQuotaChangesLimit = 2000

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
	SuccessCount    int   `json:"success_count"`
	ErrorCount      int   `json:"error_count"`
	UnsupportedCount int   `json:"unsupported_count,omitempty"`
	InvalidCount    int   `json:"invalid_count"`
	ResetBoundaries int   `json:"reset_boundaries"`
	SpanSeconds     int64 `json:"span_seconds"`
}

type quotaChangeItem struct {
	ChannelID          int                    `json:"channel_id"`
	Name               string                 `json:"name"`
	AccountLabel       string                 `json:"account_label"`
	MetricType         string                 `json:"metric_type"`
	WindowType         string                 `json:"window_type"`
	Source             string                 `json:"source,omitempty"`
	Unit               string                 `json:"unit"`
	Currency           string                 `json:"currency,omitempty"`
	PlanType           string                 `json:"plan_type,omitempty"`
	CurrentAvailable   *float64               `json:"current_available,omitempty"`
	CurrentTotal       *float64               `json:"current_total,omitempty"`
	PreviousAvailable  *float64               `json:"previous_available,omitempty"`
	ChangePerMinute    *float64               `json:"change_per_minute,omitempty"`
	AbsChangePerMinute *float64               `json:"abs_change_per_minute,omitempty"`
	Direction          string                 `json:"direction"`
	SampleSpanSeconds  int64                  `json:"sample_span_seconds"`
	ObservedAt         int64                  `json:"observed_at"`
	Status             string                 `json:"status"`
	// Alert is a read-only snapshot of the configured quota threshold state.
	// It is deliberately derived from the latest normalized observation and
	// never triggers notification, routing, or channel state changes.
	Alert              *quotaHistoryAlert     `json:"alert,omitempty"`
	DataQuality        quotaChangeDataQuality `json:"data_quality"`
}

type quotaChangeGroupKey struct {
	ChannelID  int
	MetricType string
	WindowType string
	Source     string
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
		common.ApiError(c, errors.New("invalid range; use 24h, 7d, 30d, 90d, or custom"))
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
	items, quality := buildQuotaChangeItems(rows)
	sortQuotaChangeItems(items, c.Query("sort"))
	if len(items) > limit {
		items = items[:limit]
	}
	response := gin.H{
		"items":        items,
		"range":        rangeName,
		"start":        start,
		"end":          end,
		"generated_at": now,
		"data_quality": quality,
	}
	response["summary"] = quotaChangeSummary(items)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

func quotaChangeRangeSeconds(value string) (int64, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
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
	groups := make(map[quotaChangeGroupKey][]model.ChannelQuotaAggregateRow)
	for _, row := range rows {
		key := quotaChangeGroupKey{row.ChannelID, row.MetricType, row.WindowType, row.Source}
		groups[key] = append(groups[key], row)
	}
	items := make([]quotaChangeItem, 0, len(groups))
	global := quotaChangeDataQuality{}
	for _, groupRows := range groups {
		sort.SliceStable(groupRows, func(i, j int) bool {
			if groupRows[i].ObservedAt == groupRows[j].ObservedAt {
				return groupRows[i].ID < groupRows[j].ID
			}
			return groupRows[i].ObservedAt < groupRows[j].ObservedAt
		})
		item := buildQuotaChangeItem(groupRows)
		items = append(items, item)
		global.SuccessCount += item.DataQuality.SuccessCount
		global.ErrorCount += item.DataQuality.ErrorCount
		global.UnsupportedCount += item.DataQuality.UnsupportedCount
		global.InvalidCount += item.DataQuality.InvalidCount
		global.ResetBoundaries += item.DataQuality.ResetBoundaries
		if item.DataQuality.SpanSeconds > global.SpanSeconds {
			global.SpanSeconds = item.DataQuality.SpanSeconds
		}
	}
	return items, global
}

func buildQuotaChangeItem(rows []model.ChannelQuotaAggregateRow) quotaChangeItem {
	latest := rows[len(rows)-1]
	name := strings.TrimSpace(latest.ChannelName)
	if name == "" {
		name = "Channel #" + strconv.Itoa(latest.ChannelID)
	}
	item := quotaChangeItem{
		ChannelID: latest.ChannelID, Name: name, AccountLabel: name,
		MetricType: latest.MetricType, WindowType: latest.WindowType, Source: latest.Source,
		Unit: latest.Unit, Currency: latest.Currency, PlanType: latest.PlanType,
		ObservedAt: latest.ObservedAt, Status: latest.Status, Direction: "unavailable",
	}
	// Keep the global quota-change view consistent with the channel history
	// endpoint: failed, unsupported, or total-less observations report an
	// unavailable/disabled alert rather than carrying forward stale state.
	item.Alert = quotaChangeAlertForRow(latest)
	if item.Status == "" {
		item.Status = "unknown"
	}
	quality := &item.DataQuality
	var previousSuccess *model.ChannelQuotaAggregateRow
	var lastValidResetAt int64
	hasValidReset := false
	for _, row := range rows {
		if row.Status == "success" {
			if !finiteQuotaChangeValue(row.Available) {
				quality.InvalidCount++
				continue
			}
			quality.SuccessCount++
			if hasValidReset && lastValidResetAt != row.ResetAt && (lastValidResetAt != 0 || row.ResetAt != 0) {
				quality.ResetBoundaries++
			}
			lastValidResetAt = row.ResetAt
			hasValidReset = true
		} else if row.Status == "unsupported" {
			quality.UnsupportedCount++
		} else {
			quality.ErrorCount++
		}
	}
	if latest.Status == "success" && finiteQuotaChangeValue(latest.Available) {
		item.CurrentAvailable = quotaChangeFloatPtr(latest.Available)
		if latest.Total != nil && finiteQuotaChangeValue(*latest.Total) {
			item.CurrentTotal = quotaChangeFloatPtr(*latest.Total)
		}
		for index := len(rows) - 2; index >= 0; index-- {
			row := rows[index]
			if row.Status != "success" || !finiteQuotaChangeValue(row.Available) {
				continue
			}
			if row.ResetAt != latest.ResetAt && (row.ResetAt != 0 || latest.ResetAt != 0) {
				break
			}
			previousSuccess = &row
			break
		}
	}
	if previousSuccess != nil {
		item.PreviousAvailable = quotaChangeFloatPtr(previousSuccess.Available)
		span := latest.ObservedAt - previousSuccess.ObservedAt
		if span > 0 {
			item.SampleSpanSeconds = span
			quality.SpanSeconds = span
			rate := (latest.Available - previousSuccess.Available) / (float64(span) / 60)
			if finiteQuotaChangeValue(rate) {
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
			}
		}
	} else if item.CurrentAvailable != nil {
		item.Direction = "unknown"
	}
	return item
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
	for _, item := range items {
		if item.ChangePerMinute == nil || item.AbsChangePerMinute == nil {
			continue
		}
		absRate, rate := *item.AbsChangePerMinute, *item.ChangePerMinute
		if maxAbs == nil || absRate > *maxAbs {
			maxAbs = quotaChangeFloatPtr(absRate)
		}
		if rate < 0 && (maxDrop == nil || rate < *maxDrop) {
			maxDrop = quotaChangeFloatPtr(rate)
		}
		if rate > 0 && (maxIncrease == nil || rate > *maxIncrease) {
			maxIncrease = quotaChangeFloatPtr(rate)
		}
	}
	return gin.H{"max_abs_change_per_minute": maxAbs, "max_drop_per_minute": maxDrop, "max_increase_per_minute": maxIncrease}
}
