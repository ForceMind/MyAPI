package controller

import (
	"context"
	"errors"
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
	"github.com/google/uuid"
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
	currentSnapshots, err := model.ListLatestChannelQuotaSnapshotBatch(
		c.Request.Context(), id, start, end, "codex_rate_limit",
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pointsByBatch := make(map[string]gin.H, len(snapshots))
	pointOrder := make([]string, 0, len(snapshots))
	successCount, errorCount, unsupportedCount := 0, 0, 0
	for _, snapshot := range snapshots {
		batchKey := codexHistoryBatchKey(snapshot)
		point, exists := pointsByBatch[batchKey]
		if !exists {
			point = gin.H{"timestamp": snapshot.ObservedAt}
			if snapshot.SampleID != "" {
				point["sample_id"] = snapshot.SampleID
			}
			pointsByBatch[batchKey] = point
			pointOrder = append(pointOrder, batchKey)
		}
		success, failed, unsupported := applyCodexUsageHistorySnapshot(point, snapshot)
		successCount += success
		errorCount += failed
		unsupportedCount += unsupported
	}
	points := make([]gin.H, 0, len(pointOrder))
	for _, batchKey := range pointOrder {
		points = append(points, pointsByBatch[batchKey])
	}
	dataQuality := gin.H{"success_count": successCount, "error_count": errorCount, "unsupported_count": unsupportedCount}
	if len(snapshots) > 1 {
		dataQuality["span_seconds"] = snapshots[len(snapshots)-1].ObservedAt - snapshots[0].ObservedAt
	} else {
		dataQuality["span_seconds"] = int64(0)
	}
	var current gin.H
	if len(currentSnapshots) > 0 {
		current = gin.H{"timestamp": currentSnapshots[0].ObservedAt}
		if currentSnapshots[0].SampleID != "" {
			current["sample_id"] = currentSnapshots[0].SampleID
		}
		for _, snapshot := range currentSnapshots {
			applyCodexUsageHistorySnapshot(current, snapshot)
		}
		for _, kind := range []string{"primary", "secondary"} {
			if current[kind+"_status"] != "unavailable" {
				continue
			}
			if lastSeenAt := lastCodexPointObservation(points, kind+"_used_percent"); lastSeenAt > 0 {
				current[kind+"_last_seen_at"] = lastSeenAt
			}
		}
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
	if len(currentSnapshots) > 0 {
		latestObservedAt = currentSnapshots[0].ObservedAt
	}
	summary["latest_observed_at"] = latestObservedAt
	response := gin.H{
		"channel_id": id, "start": start, "end": end, "limit": limit,
		"points": points, "current": current, "data_quality": dataQuality,
		"summary": summary,
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

func codexHistoryBatchKey(snapshot model.ChannelQuotaSnapshot) string {
	if snapshot.SampleID != "" {
		return "sample:" + snapshot.SampleID
	}
	return "legacy:" + strconv.FormatInt(snapshot.ObservedAt, 10)
}

func applyCodexUsageHistorySnapshot(point gin.H, snapshot model.ChannelQuotaSnapshot) (success, failed, unsupported int) {
	kind := codexSnapshotWindowKind(snapshot.Source)
	switch {
	case snapshot.Status == "success":
		point["status"] = "success"
		if snapshot.PlanType != "" {
			point["plan_type"] = snapshot.PlanType
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
			point[kind+"_status"] = "success"
		}
		return 1, 0, 0
	case snapshot.ErrorCode == "window_absent" && kind != "":
		if _, hasCurrentValue := point[kind+"_used_percent"]; !hasCurrentValue {
			point[kind+"_status"] = "unavailable"
			point[kind+"_ended_at"] = snapshot.ObservedAt
			point[kind+"_error_code"] = snapshot.ErrorCode
		}
		if _, hasStatus := point["status"]; !hasStatus {
			point["status"] = "unsupported"
		}
		return 0, 0, 1
	case snapshot.Status == "unsupported":
		point["status"] = "unsupported"
		if snapshot.ErrorCode != "" {
			point["error_code"] = snapshot.ErrorCode
		}
		return 0, 0, 1
	default:
		point["status"] = "error"
		if snapshot.ErrorCode != "" {
			point["error_code"] = snapshot.ErrorCode
		}
		return 0, 1, 0
	}
}

func codexSnapshotWindowKind(source string) string {
	if strings.HasSuffix(source, "_primary") {
		return "primary"
	}
	if strings.HasSuffix(source, "_secondary") {
		return "secondary"
	}
	return ""
}

func lastCodexPointObservation(points []gin.H, key string) int64 {
	for index := len(points) - 1; index >= 0; index-- {
		if _, exists := points[index][key]; !exists {
			continue
		}
		timestamp, _ := points[index]["timestamp"].(int64)
		return timestamp
	}
	return 0
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
		refreshedKey, _, refreshErr := service.RefreshCodexChannelCredential(ctx, ch.Id, service.CodexCredentialRefreshOptions{ExpectedKey: &ch.Key})
		if refreshErr != nil {
			var persistErr *service.CodexCredentialPersistenceError
			if errors.As(refreshErr, &persistErr) {
				common.SysError("Codex credential refresh persistence failed")
				if snapshotErr := recordCodexCredentialPersistenceFailure(ch.Id); snapshotErr != nil {
					common.SysError("failed to record Codex credential persistence failure")
				}
				c.JSON(http.StatusOK, gin.H{"success": false, "message": userMessage})
				return
			}
		} else {
			ctx2, cancel2 := context.WithTimeout(c.Request.Context(), 15*time.Second)
			defer cancel2()
			statusCode, body, err = fetch(ctx2, client, ch.GetBaseURL(), refreshedKey.AccessToken, refreshedKey.AccountID)
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

// channelQuotaSamplingError keeps an upstream query failure separate from a
// history persistence failure.  The background sampler uses this distinction
// to report provider outages as Failed and database/write problems as
// PersistFailed; callers still receive one error value from the sampler.
type channelQuotaSamplingError struct {
	QueryErr   error
	PersistErr error
}

func (err *channelQuotaSamplingError) Error() string {
	if err == nil {
		return ""
	}
	var parts []string
	if err.QueryErr != nil {
		parts = append(parts, fmt.Sprintf("Codex usage request failed: %v", err.QueryErr))
	}
	if err.PersistErr != nil {
		parts = append(parts, fmt.Sprintf("quota snapshot persistence failed: %v", err.PersistErr))
	}
	return strings.Join(parts, "; ")
}

// Unwrap preserves errors.Is/errors.As behavior for the primary failure while
// the typed fields retain both failure classes for the sampler summary.
func (err *channelQuotaSamplingError) Unwrap() error {
	if err == nil {
		return nil
	}
	if err.QueryErr != nil {
		return err.QueryErr
	}
	return err.PersistErr
}

func newChannelQuotaSamplingError(queryErr, persistErr error) error {
	if queryErr == nil && persistErr == nil {
		return nil
	}
	return &channelQuotaSamplingError{QueryErr: queryErr, PersistErr: persistErr}
}

func recordCodexUsageSnapshots(channelID, statusCode int, body []byte) error {
	return recordCodexUsageSnapshotsForAccount(channelID, "", statusCode, body)
}

func recordCodexUsageSnapshotsForAccount(channelID int, accountID string, statusCode int, body []byte) error {
	return recordCodexUsageSnapshotsAtForAccount(channelID, accountID, time.Now().Unix(), statusCode, body)
}

func recordCodexUsageSnapshotsAt(channelID int, observedAt int64, statusCode int, body []byte) error {
	return recordCodexUsageSnapshotsAtForAccount(channelID, "", observedAt, statusCode, body)
}

func recordCodexUsageSnapshotsAtForAccount(channelID int, accountID string, observedAt int64, statusCode int, body []byte) error {
	return recordCodexUsageSnapshotsAtWithSampleIDForAccount(channelID, accountID, observedAt, uuid.NewString(), statusCode, body)
}

func recordCodexUsageSnapshotsAtWithSampleID(channelID int, observedAt int64, sampleID string, statusCode int, body []byte) error {
	return recordCodexUsageSnapshotsAtWithSampleIDForAccount(channelID, "", observedAt, sampleID, statusCode, body)
}

func recordCodexUsageSnapshotsAtWithSampleIDForAccount(channelID int, accountID string, observedAt int64, sampleID string, statusCode int, body []byte) error {
	snapshots := normalizeCodexUsageSnapshots(channelID, observedAt, statusCode, body)
	accountRef := model.ChannelQuotaAccountRef("codex", accountID)
	for index := range snapshots {
		snapshots[index].AccountRef = accountRef
	}
	return recordCodexSnapshotBatch(snapshots, sampleID)
}

func recordCodexSnapshotBatch(snapshots []model.ChannelQuotaSnapshot, sampleID string) error {
	if len(snapshots) == 0 {
		return nil
	}
	for index := range snapshots {
		snapshots[index].SampleID = sampleID
	}
	ctx, cancel := context.WithTimeout(context.Background(), channelQuotaPersistenceTimeout)
	defer cancel()
	return model.RecordChannelQuotaSnapshotBatchWithContext(ctx, snapshots, model.ChannelQuotaSnapshotBatchOptions{
		PreviousSources: []string{"codex_wham_usage_primary", "codex_wham_usage_secondary"},
		AbsenceMarker: &model.ChannelQuotaSnapshotAbsenceMarker{
			Status: "unsupported", ErrorCode: "window_absent",
			ErrorMessage: "rate limit window absent from latest successful usage response",
		},
	})
}

const channelQuotaPersistenceTimeout = 5 * time.Second

// A completed request must still record its outcome when its network context
// expires. This separate, short budget also bounds connection-pool waits.
func recordQuotaSamplingSnapshots(snapshots []model.ChannelQuotaSnapshot) error {
	ctx, cancel := context.WithTimeout(context.Background(), channelQuotaPersistenceTimeout)
	defer cancel()
	var firstErr error
	for index := range snapshots {
		if err := model.RecordChannelQuotaSnapshotWithContext(ctx, &snapshots[index]); err != nil && firstErr == nil {
			firstErr = err
		}
		if ctx.Err() != nil {
			break
		}
	}
	return firstErr
}

func recordCodexCredentialPersistenceFailure(channelID int) error {
	snapshots := normalizeCodexUsageSnapshots(channelID, time.Now().Unix(), 0, nil)
	snapshots[0].ErrorCode = "credential_persist_failed"
	snapshots[0].ErrorMessage = "refreshed quota credentials could not be saved"
	return recordCodexSnapshotBatch(snapshots, uuid.NewString())
}

// sampleCodexChannelUsage records one normalized official WHAM usage sample for
// the bounded background quota sampler. It deliberately shares the same
// endpoint and normalization contract as the admin usage view, but never
// returns or persists the OAuth credential or raw provider response.
const codexQuotaSamplingRequestTimeout = 20 * time.Second

func sampleCodexChannelUsage(ctx context.Context, ch *model.Channel) error {
	if ch == nil {
		return fmt.Errorf("nil Codex channel")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// One deadline covers the original request, credential refresh and retry.
	// Never renew the budget after a 401 or let the relay's unlimited timeout
	// keep the background sampler and channel polling lock occupied forever.
	ctx, cancel := context.WithTimeout(ctx, codexQuotaSamplingRequestTimeout)
	defer cancel()
	oauthKey, err := codex.ParseOAuthKey(strings.TrimSpace(ch.Key))
	if err != nil {
		persistErr := recordCodexUsageSnapshots(ch.Id, 0, nil)
		return newChannelQuotaSamplingError(fmt.Errorf("parse Codex OAuth key: %w", err), persistErr)
	}
	accessToken := strings.TrimSpace(oauthKey.AccessToken)
	accountID := strings.TrimSpace(oauthKey.AccountID)
	if accessToken == "" || accountID == "" {
		persistErr := recordCodexUsageSnapshots(ch.Id, 0, nil)
		return newChannelQuotaSamplingError(fmt.Errorf("Codex OAuth key is missing access_token or account_id"), persistErr)
	}
	client, err := service.GetHttpClientWithProxy(ch.GetSetting().Proxy)
	if err != nil {
		persistErr := recordCodexUsageSnapshots(ch.Id, 0, nil)
		return newChannelQuotaSamplingError(err, persistErr)
	}
	statusCode, body, err := service.FetchCodexWhamUsage(ctx, client, ch.GetBaseURL(), accessToken, accountID)
	if err == nil && (statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden) && strings.TrimSpace(oauthKey.RefreshToken) != "" {
		refreshedKey, _, refreshErr := service.RefreshCodexChannelCredential(ctx, ch.Id, service.CodexCredentialRefreshOptions{ExpectedKey: &ch.Key})
		if refreshErr != nil {
			var persistErr *service.CodexCredentialPersistenceError
			if errors.As(refreshErr, &persistErr) {
				snapshotErr := recordCodexCredentialPersistenceFailure(ch.Id)
				return newChannelQuotaSamplingError(errors.New("Codex credential refresh could not be persisted"), errors.Join(persistErr, snapshotErr))
			}
			err = refreshErr
		} else {
			accountID = strings.TrimSpace(refreshedKey.AccountID)
			if accountID == "" {
				persistErr := recordCodexUsageSnapshots(ch.Id, 0, nil)
				return newChannelQuotaSamplingError(errors.New("refreshed Codex credential is missing account_id"), persistErr)
			}
			statusCode, body, err = service.FetchCodexWhamUsage(ctx, client, ch.GetBaseURL(), refreshedKey.AccessToken, accountID)
		}
	}
	if err != nil {
		snapshots := normalizeCodexUsageSnapshots(ch.Id, time.Now().Unix(), 0, nil)
		if errors.Is(err, context.DeadlineExceeded) {
			snapshots[0].ErrorCode = "upstream_timeout"
			snapshots[0].ErrorMessage = "quota sampling request timed out"
		} else if errors.Is(err, context.Canceled) {
			snapshots[0].ErrorCode = "sampling_canceled"
			snapshots[0].ErrorMessage = "quota sampling request canceled"
		}
		// Persist the safe failure marker independently of the expired request
		// context, so a timed-out account yields its turn in the next batch.
		persistErr := recordCodexSnapshotBatch(snapshots, uuid.NewString())
		return newChannelQuotaSamplingError(err, persistErr)
	}
	persistErr := recordCodexUsageSnapshotsForAccount(ch.Id, accountID, statusCode, body)
	if statusCode < 200 || statusCode >= 300 {
		return newChannelQuotaSamplingError(fmt.Errorf("Codex usage upstream status %d", statusCode), persistErr)
	}
	// A successful HTTP response is not necessarily a usable quota sample.  In
	// particular, providers can return an HTML/login payload or a JSON shape
	// without rate-limit windows while still responding with 2xx.  Classify
	// those observations as unsupported so the scheduled sampler reports them
	// in Unsupported instead of falsely counting them as Sampled.
	if !codexUsageResponseSupportsRateLimit(statusCode, body) {
		return newChannelQuotaSamplingError(errChannelQuotaUnsupported, persistErr)
	}
	return newChannelQuotaSamplingError(nil, persistErr)
}

// codexUsageResponseSupportsRateLimit mirrors the normalization contract used
// for persistence and reports whether at least one finite rate-limit window was
// found.  Non-2xx responses are handled by the caller as query failures and
// therefore are not classified as unsupported here.
func codexUsageResponseSupportsRateLimit(statusCode int, body []byte) bool {
	if statusCode < 200 || statusCode >= 300 {
		return false
	}
	for _, snapshot := range normalizeCodexUsageSnapshots(0, 1, statusCode, body) {
		if snapshot.Status == "success" {
			return true
		}
	}
	return false
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
	switch seconds {
	case 5 * 60 * 60:
		return "five_hour"
	case 7 * 24 * 60 * 60:
		return "weekly"
	case 24 * 60 * 60:
		return "daily"
	default:
		if seconds > 0 {
			return "custom"
		}
		return "unknown"
	}
}

func ptrCodexFloat(value float64) *float64 { return &value }
