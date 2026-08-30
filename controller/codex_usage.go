package controller

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relay/channel/codex"
	"github.com/ForceMind/MyAPI/service"

	"github.com/gin-gonic/gin"
)

func GetCodexChannelUsage(c *gin.Context) {
	fetchCodexChannelWhamData(
		c,
		service.FetchCodexWhamUsage,
		"failed to fetch codex usage",
		"获取用量信息失败，请稍后重试",
		true,
	)
}

// GetCodexChannelUsageHistory serves normalized snapshots collected by the
// usage endpoint. It reuses the bounded history response contract while
// enforcing the Codex metric scope for callers that do not pass a filter.
func GetCodexChannelUsageHistory(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel not found"})
		return
	}
	if channel.Type != constant.ChannelTypeCodex {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not Codex"})
		return
	}
	if channel.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "multi-key channel is not supported"})
		return
	}
	now := time.Now().Unix()
	end, start := now, now-30*24*60*60
	query := c.Request.URL.Query()
	if value := strings.TrimSpace(query.Get("range")); value != "" {
		seconds := map[string]int64{"24h": 24 * 60 * 60, "1d": 24 * 60 * 60, "7d": 7 * 24 * 60 * 60, "30d": 30 * 24 * 60 * 60, "90d": 90 * 24 * 60 * 60}[strings.ToLower(value)]
		if seconds == 0 {
			common.ApiError(c, fmt.Errorf("invalid range; use 24h, 7d, 30d, or 90d"))
			return
		}
		start = end - seconds
	}
	for key, target := range map[string]*int64{"start": &start, "end": &end} {
		if value := strings.TrimSpace(query.Get(key)); value != "" {
			parsed, parseErr := strconv.ParseInt(value, 10, 64)
			if parseErr != nil {
				parsedTime, timeErr := time.Parse(time.RFC3339, value)
				if timeErr != nil {
					common.ApiError(c, fmt.Errorf("invalid %s timestamp", key))
					return
				}
				parsed = parsedTime.Unix()
			}
			*target = parsed
		}
	}
	if start < 0 || end < start || end-start > 180*24*60*60 {
		common.ApiError(c, fmt.Errorf("invalid Codex usage history time range"))
		return
	}
	limit := 500
	if value := strings.TrimSpace(query.Get("limit")); value != "" {
		parsed, parseErr := strconv.Atoi(value)
		if parseErr != nil || parsed <= 0 {
			common.ApiError(c, fmt.Errorf("invalid quota history limit"))
			return
		}
		limit = parsed
	}
	if limit > 2000 {
		limit = 2000
	}
	snapshots, err := model.ListChannelQuotaSnapshots(id, start, end, "codex_rate_limit", strings.TrimSpace(query.Get("window_type")), limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pointsByTimestamp := make(map[int64]gin.H, len(snapshots))
	pointOrder := make([]int64, 0, len(snapshots))
	successCount, errorCount, unsupportedCount := 0, 0, 0
	for _, snapshot := range snapshots {
		point, exists := pointsByTimestamp[snapshot.ObservedAt]
		if !exists {
			point = gin.H{"timestamp": snapshot.ObservedAt, "status": snapshot.Status}
			pointsByTimestamp[snapshot.ObservedAt] = point
			pointOrder = append(pointOrder, snapshot.ObservedAt)
		}
		if snapshot.PlanType != "" {
			point["plan_type"] = snapshot.PlanType
		}
		if snapshot.Status == "success" {
			successCount++
			kind := ""
			if strings.HasSuffix(snapshot.Source, "_primary") {
				kind = "primary"
			} else if strings.HasSuffix(snapshot.Source, "_secondary") {
				kind = "secondary"
			}
			if kind != "" {
				if snapshot.Used != nil && finiteQuotaValue(*snapshot.Used) {
					point[kind+"_used_percent"] = *snapshot.Used
				}
				if finiteQuotaValue(snapshot.Available) {
					point[kind+"_available_percent"] = snapshot.Available
				}
				if snapshot.ResetAt > 0 {
					point[kind+"_reset_at"] = snapshot.ResetAt
				}
				point[kind+"_window_type"] = snapshot.WindowType
				point[kind+"_window_seconds"] = snapshot.WindowSeconds
			}
		} else if snapshot.Status == "unsupported" {
			unsupportedCount++
		} else {
			errorCount++
		}
		if snapshot.ErrorCode != "" {
			point["error_code"] = snapshot.ErrorCode
		}
	}
	points := make([]gin.H, 0, len(pointOrder))
	for _, timestamp := range pointOrder {
		points = append(points, pointsByTimestamp[timestamp])
	}
	dataQuality := gin.H{"success_count": successCount, "error_count": errorCount, "unsupported_count": unsupportedCount}
	if len(snapshots) > 1 {
		dataQuality["span_seconds"] = snapshots[len(snapshots)-1].ObservedAt - snapshots[0].ObservedAt
	} else {
		dataQuality["span_seconds"] = int64(0)
	}
	var current gin.H
	if len(points) > 0 {
		current = points[len(points)-1]
	}
	primaryFirst, primaryLast := firstCodexPointValue(points, "primary_used_percent")
	secondaryFirst, secondaryLast := firstCodexPointValue(points, "secondary_used_percent")
	summary := gin.H{"point_count": len(points), "success_count": successCount, "latest_observed_at": end}
	if primaryFirst != nil && primaryLast != nil {
		summary["primary_change_percent"] = *primaryLast - *primaryFirst
	}
	if secondaryFirst != nil && secondaryLast != nil {
		summary["secondary_change_percent"] = *secondaryLast - *secondaryFirst
	}
	latestObservedAt := int64(0)
	if len(snapshots) > 0 {
		latestObservedAt = snapshots[len(snapshots)-1].ObservedAt
	}
	summary["latest_observed_at"] = latestObservedAt
	response := gin.H{
		"channel_id": id, "start": start, "end": end, "limit": limit,
		"points": points, "current": current, "data_quality": dataQuality,
		"summary": summary,
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

func firstCodexPointValue(points []gin.H, key string) (*float64, *float64) {
	var first, last *float64
	for _, point := range points {
		value, ok := point[key].(float64)
		if !ok || !finiteCodexPercent(value) {
			continue
		}
		if first == nil {
			v := value
			first = &v
		}
		v := value
		last = &v
	}
	return first, last
}

func GetCodexChannelRateLimitResetCredits(c *gin.Context) {
	fetchCodexChannelWhamData(
		c,
		service.FetchCodexWhamRateLimitResetCredits,
		"failed to fetch codex reset credits",
		"获取重置次数详情失败，请稍后重试",
		false,
	)
}

func ResetCodexChannelUsage(c *gin.Context) {
	fetchCodexChannelWhamData(
		c,
		service.ConsumeCodexWhamRateLimitResetCredit,
		"failed to reset codex usage",
		"重置用量失败，请稍后重试",
		false,
	)
}

type codexWhamFetchFunc func(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
) (statusCode int, body []byte, err error)

func fetchCodexChannelWhamData(
	c *gin.Context,
	fetch codexWhamFetchFunc,
	logPrefix string,
	userMessage string,
	recordUsage bool,
) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}

	ch, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if ch == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel not found"})
		return
	}
	if ch.Type != constant.ChannelTypeCodex {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not Codex"})
		return
	}
	if ch.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "multi-key channel is not supported"})
		return
	}

	oauthKey, err := codex.ParseOAuthKey(strings.TrimSpace(ch.Key))
	if err != nil {
		common.SysError("failed to parse oauth key: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "解析凭证失败，请检查渠道配置"})
		return
	}
	accessToken := strings.TrimSpace(oauthKey.AccessToken)
	accountID := strings.TrimSpace(oauthKey.AccountID)
	if accessToken == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "codex channel: access_token is required"})
		return
	}
	if accountID == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "codex channel: account_id is required"})
		return
	}

	client, err := service.GetHttpClientWithProxy(ch.GetSetting().Proxy)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	statusCode, body, err := fetch(ctx, client, ch.GetBaseURL(), accessToken, accountID)
	if err != nil {
		common.SysError(logPrefix + ": " + err.Error())
		if recordUsage {
			recordCodexUsageSnapshots(channelId, 0, nil)
		}
		c.JSON(http.StatusOK, gin.H{"success": false, "message": userMessage})
		return
	}

	if (statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden) && strings.TrimSpace(oauthKey.RefreshToken) != "" {
		refreshCtx, refreshCancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer refreshCancel()

		res, refreshErr := service.RefreshCodexOAuthTokenWithProxy(refreshCtx, oauthKey.RefreshToken, ch.GetSetting().Proxy)
		if refreshErr == nil {
			oauthKey.AccessToken = res.AccessToken
			oauthKey.RefreshToken = res.RefreshToken
			oauthKey.LastRefresh = time.Now().Format(time.RFC3339)
			oauthKey.Expired = res.ExpiresAt.Format(time.RFC3339)
			if strings.TrimSpace(oauthKey.Type) == "" {
				oauthKey.Type = "codex"
			}

			encoded, encErr := common.Marshal(oauthKey)
			if encErr == nil {
				_ = model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("key", string(encoded)).Error
				model.InitChannelCache()
			}

			ctx2, cancel2 := context.WithTimeout(c.Request.Context(), 15*time.Second)
			defer cancel2()
			statusCode, body, err = fetch(ctx2, client, ch.GetBaseURL(), oauthKey.AccessToken, accountID)
			if err != nil {
				common.SysError(logPrefix + " after refresh: " + err.Error())
				if recordUsage {
					recordCodexUsageSnapshots(channelId, 0, nil)
				}
				c.JSON(http.StatusOK, gin.H{"success": false, "message": userMessage})
				return
			}
		}
	}

	var payload any
	if common.Unmarshal(body, &payload) != nil {
		payload = string(body)
	}

	ok := statusCode >= 200 && statusCode < 300
	if recordUsage {
		recordCodexUsageSnapshots(channelId, statusCode, body)
	}
	resp := gin.H{
		"success":         ok,
		"message":         "",
		"upstream_status": statusCode,
		"data":            payload,
	}
	if !ok {
		resp["message"] = fmt.Sprintf("upstream status: %d", statusCode)
	}
	c.JSON(http.StatusOK, resp)
}

// codexUsagePayload intentionally models only the fields exposed by the
// official Wham usage endpoint. Raw payloads and credentials are never
// persisted; only normalized rate-limit observations are stored.
type codexUsagePayload struct {
	PlanType  string               `json:"plan_type"`
	RateLimit *codexUsageRateLimit `json:"rate_limit"`
}

type codexUsageRateLimit struct {
	PlanType        string                     `json:"plan_type"`
	PrimaryWindow   *codexUsageRateLimitWindow `json:"primary_window"`
	SecondaryWindow *codexUsageRateLimitWindow `json:"secondary_window"`
}

type codexUsageRateLimitWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	ResetAt            int64   `json:"reset_at"`
	LimitWindowSeconds int64   `json:"limit_window_seconds"`
}

func recordCodexUsageSnapshots(channelID, statusCode int, body []byte) {
	snapshots := normalizeCodexUsageSnapshots(channelID, time.Now().Unix(), statusCode, body)
	for index := range snapshots {
		_ = model.RecordChannelQuotaSnapshot(&snapshots[index])
	}
}

func normalizeCodexUsageSnapshots(channelID int, observedAt int64, statusCode int, body []byte) []model.ChannelQuotaSnapshot {
	if statusCode < 200 || statusCode >= 300 {
		// Keep a failure marker so callers do not mistake an old successful
		// observation for the current account state.
		errorCode := "upstream_http"
		errorMessage := fmt.Sprintf("upstream status %d", statusCode)
		if statusCode == 0 {
			errorCode = "upstream_transport"
			errorMessage = "upstream request failed"
		}
		return []model.ChannelQuotaSnapshot{{
			ChannelId: channelID, ObservedAt: observedAt,
			MetricType: "codex_rate_limit", WindowType: "none", Unit: "percent",
			Source: "codex_wham_usage", Status: "error",
			ErrorCode: errorCode, ErrorMessage: errorMessage,
		}}
	}
	var payload codexUsagePayload
	if err := common.Unmarshal(body, &payload); err != nil || payload.RateLimit == nil {
		return []model.ChannelQuotaSnapshot{{
			ChannelId: channelID, ObservedAt: observedAt,
			MetricType: "codex_rate_limit", WindowType: "none", Unit: "percent",
			Source: "codex_wham_usage", Status: "unsupported",
			ErrorCode: "invalid_payload", ErrorMessage: "usage response did not contain a supported rate limit",
		}}
	}
	planType := strings.TrimSpace(payload.PlanType)
	if planType == "" {
		planType = strings.TrimSpace(payload.RateLimit.PlanType)
	}
	snapshots := make([]model.ChannelQuotaSnapshot, 0, 2)
	for _, item := range []struct {
		window *codexUsageRateLimitWindow
		kind   string
	}{
		{payload.RateLimit.PrimaryWindow, "primary"},
		{payload.RateLimit.SecondaryWindow, "secondary"},
	} {
		if item.window == nil || !finiteCodexPercent(item.window.UsedPercent) {
			continue
		}
		used := item.window.UsedPercent
		available := 100 - used
		windowType := codexWindowType(item.window.LimitWindowSeconds)
		snapshots = append(snapshots, model.ChannelQuotaSnapshot{
			ChannelId: channelID, ObservedAt: observedAt,
			Available: available, Used: &used, Total: ptrCodexFloat(100),
			Unit: "percent", MetricType: "codex_rate_limit", WindowType: windowType,
			WindowSeconds: item.window.LimitWindowSeconds, ResetAt: item.window.ResetAt,
			PlanType: planType, Source: "codex_wham_usage_" + item.kind, Status: "success",
		})
	}
	if len(snapshots) == 0 {
		return []model.ChannelQuotaSnapshot{{
			ChannelId: channelID, ObservedAt: observedAt,
			MetricType: "codex_rate_limit", WindowType: "none", Unit: "percent",
			Source: "codex_wham_usage", Status: "unsupported",
			ErrorCode: "invalid_payload", ErrorMessage: "usage response contained no valid rate limit windows",
		}}
	}
	return snapshots
}

func finiteCodexPercent(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 100
}

func codexWindowType(seconds int64) string {
	switch {
	case seconds >= 7*24*60*60:
		return "weekly"
	case seconds >= 24*60*60:
		return "daily"
	case seconds > 0:
		return "five_hour"
	default:
		return "unknown"
	}
}

func ptrCodexFloat(value float64) *float64 { return &value }
