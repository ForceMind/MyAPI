package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

const (
	ChannelRoutingWorkloadChat     = "chat"
	ChannelRoutingWorkloadWork     = "work"
	ChannelRoutingWorkloadBalanced = "balanced"

	channelRoutingQuotaCacheTTL = 10 * time.Second
	channelRoutingQuotaCacheMax = 2048
	channelRoutingQuotaRowLimit = 32
	channelRoutingContextKey    = "channel_routing_state"
)

type ChannelRoutingQuota struct {
	State         string   `json:"state"`
	Available     *float64 `json:"available,omitempty"`
	Unit          string   `json:"unit,omitempty"`
	ObservedAt    int64    `json:"observed_at,omitempty"`
	ComparisonKey string   `json:"comparison_key,omitempty"`
}

type ChannelRoutingManagementChannel struct {
	ID             int                 `json:"id"`
	Name           string              `json:"name"`
	Type           int                 `json:"type"`
	Status         int                 `json:"status"`
	Priority       int64               `json:"priority"`
	Weight         uint                `json:"weight"`
	LegacyPriority string              `json:"legacy_priority,omitempty"`
	LegacyWeight   string              `json:"legacy_weight,omitempty"`
	Quota          ChannelRoutingQuota `json:"quota"`
}

type ChannelRoutingManagementData struct {
	Policy   operation_setting.ChannelRoutingPolicy `json:"policy"`
	Channels []ChannelRoutingManagementChannel      `json:"channels"`
}

type ChannelRoutingPreviewCandidate struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Share     float64  `json:"share"`
	Reason    string   `json:"reason"`
	Available *float64 `json:"available,omitempty"`
	Unit      string   `json:"unit,omitempty"`
}

type ChannelRoutingPreview struct {
	Workload   string                           `json:"workload"`
	Reason     string                           `json:"reason"`
	Candidates []ChannelRoutingPreviewCandidate `json:"candidates"`
}

type routingQuotaAssessment struct {
	view          ChannelRoutingQuota
	comparisonKey string
	available     float64
}

type routingQuotaCacheEntry struct {
	rows     []model.ChannelQuotaSnapshot
	loadedAt time.Time
	err      error
}

var routingQuotaCache = struct {
	sync.Mutex
	entries map[int]routingQuotaCacheEntry
	loading map[int]chan struct{}
}{entries: make(map[int]routingQuotaCacheEntry), loading: make(map[int]chan struct{})}
var routingSessionCleanupCounter atomic.Uint64

type channelRoutingRequestState struct {
	Workload        string
	Reason          string
	SelectedGroup   string
	SelectedChannel int
	SessionBound    bool
	Binding         model.ChannelRoutingSession
	SwitchReason    string
	SwitchCount     int
	AttemptOutcome  string
}

func ChannelRoutingWorkload(requestPath string) string {
	switch requestPath {
	case "/v1/chat/completions", "/pg/chat/completions":
		return ChannelRoutingWorkloadChat
	case "/v1/responses", "/v1/responses/compact":
		return ChannelRoutingWorkloadWork
	default:
		return ChannelRoutingWorkloadBalanced
	}
}

func ShouldUseSmartChannelRouting(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	if _, specific := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); specific {
		return false
	}
	policy := operation_setting.GetChannelRoutingPolicy()
	return policy.Enabled && ChannelRoutingWorkload(c.Request.URL.Path) != ChannelRoutingWorkloadBalanced
}

func routingSourceFamily(source string) string {
	for _, suffix := range []string{"_primary", "_secondary"} {
		if strings.HasSuffix(source, suffix) {
			return strings.TrimSuffix(source, suffix)
		}
	}
	return source
}

func routingQuotaBaseKey(snapshot model.ChannelQuotaSnapshot) string {
	return strings.Join([]string{
		snapshot.MetricType, routingSourceFamily(snapshot.Source), snapshot.PlanType,
		snapshot.Unit, snapshot.Currency,
	}, "\x1f")
}

func routingQuotaSeriesKey(snapshot model.ChannelQuotaSnapshot) string {
	return strings.Join([]string{
		snapshot.MetricType, snapshot.WindowType, snapshot.Source, snapshot.PlanType,
		snapshot.Unit, snapshot.Currency, strconv.FormatInt(snapshot.WindowSeconds, 10),
	}, "\x1f")
}

func routingQuotaDisplayKey(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}

func deriveRoutingQuota(rows []model.ChannelQuotaSnapshot, multiKey bool, now int64, maxAge int) routingQuotaAssessment {
	unknown := routingQuotaAssessment{view: ChannelRoutingQuota{State: "unknown"}}
	if multiKey || len(rows) == 0 || maxAge <= 0 {
		return unknown
	}
	ordered := append([]model.ChannelQuotaSnapshot(nil), rows...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].ObservedAt == ordered[j].ObservedAt {
			return ordered[i].Id > ordered[j].Id
		}
		return ordered[i].ObservedAt > ordered[j].ObservedAt
	})
	if ordered[0].ObservedAt > now {
		return unknown
	}

	type family struct {
		key       string
		newest    int64
		successes map[string]model.ChannelQuotaSnapshot
	}
	families := make(map[string]*family)
	latestFailureAt := int64(0)
	for _, row := range ordered {
		if row.Status != "success" {
			if row.ObservedAt > latestFailureAt {
				latestFailureAt = row.ObservedAt
			}
			continue
		}
		base := routingQuotaBaseKey(row)
		current := families[base]
		if current == nil {
			current = &family{key: base, successes: make(map[string]model.ChannelQuotaSnapshot)}
			families[base] = current
		}
		series := routingQuotaSeriesKey(row)
		if _, exists := current.successes[series]; !exists {
			current.successes[series] = row
		}
		if row.ObservedAt > current.newest {
			current.newest = row.ObservedAt
		}
	}
	var selected *family
	for _, candidate := range families {
		if selected == nil || candidate.newest > selected.newest ||
			(candidate.newest == selected.newest && len(candidate.successes) > len(selected.successes)) ||
			(candidate.newest == selected.newest && len(candidate.successes) == len(selected.successes) && candidate.key < selected.key) {
			selected = candidate
		}
	}
	if selected == nil {
		return unknown
	}
	if selected.newest < now-int64(maxAge) {
		return routingQuotaAssessment{view: ChannelRoutingQuota{State: "stale", ObservedAt: selected.newest}}
	}

	seriesKeys := make([]string, 0, len(selected.successes))
	available := math.Inf(1)
	observedAt := int64(0)
	oldestObservedAt := int64(0)
	unit := ""
	for series, snapshot := range selected.successes {
		if snapshot.ObservedAt > now || snapshot.ObservedAt < now-int64(maxAge) {
			return routingQuotaAssessment{view: ChannelRoutingQuota{State: "stale", ObservedAt: selected.newest}}
		}
		if snapshot.ResetAt > 0 && snapshot.ResetAt <= now {
			return unknown
		}
		if !QuotaSnapshotUsable(snapshot) {
			return unknown
		}
		if snapshot.Available < available {
			available = snapshot.Available
		}
		if snapshot.ObservedAt > observedAt {
			observedAt = snapshot.ObservedAt
		}
		if oldestObservedAt == 0 || snapshot.ObservedAt < oldestObservedAt {
			oldestObservedAt = snapshot.ObservedAt
		}
		unit = snapshot.Unit
		seriesKeys = append(seriesKeys, series)
	}
	if len(seriesKeys) == 0 || math.IsInf(available, 1) || latestFailureAt >= oldestObservedAt {
		return unknown
	}
	sort.Strings(seriesKeys)
	fullKey := selected.key + "\x1e" + strings.Join(seriesKeys, "\x1e")
	value := available
	state := "fresh"
	if available <= 0 {
		state = "exhausted"
	}
	return routingQuotaAssessment{
		view: ChannelRoutingQuota{
			State: state, Available: &value, Unit: unit, ObservedAt: observedAt,
			ComparisonKey: routingQuotaDisplayKey(fullKey),
		},
		comparisonKey: fullKey,
		available:     available,
	}
}

func loadRoutingQuotaRows(ctx context.Context, channelIDs []int, now time.Time, maxChannels int) (map[int][]model.ChannelQuotaSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result := make(map[int][]model.ChannelQuotaSnapshot, len(channelIDs))
	seen := make(map[int]struct{}, len(channelIDs))
	loadedCount := 0
	for _, id := range channelIDs {
		if id <= 0 {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		if maxChannels > 0 && loadedCount >= maxChannels {
			continue
		}
		loadedCount++
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			routingQuotaCache.Lock()
			entry, ok := routingQuotaCache.entries[id]
			if ok && now.Sub(entry.loadedAt) <= channelRoutingQuotaCacheTTL {
				if entry.err != nil {
					routingQuotaCache.Unlock()
					return nil, entry.err
				}
				result[id] = append([]model.ChannelQuotaSnapshot(nil), entry.rows...)
				routingQuotaCache.Unlock()
				break
			}
			if wait := routingQuotaCache.loading[id]; wait != nil {
				routingQuotaCache.Unlock()
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-wait:
					continue
				}
			}
			wait := make(chan struct{})
			routingQuotaCache.loading[id] = wait
			routingQuotaCache.Unlock()

			loaded, err := model.ListRecentChannelQuotaSnapshotsForRouting(ctx, []int{id}, channelRoutingQuotaRowLimit)
			routingQuotaCache.Lock()
			if err == nil {
				copied := append([]model.ChannelQuotaSnapshot(nil), loaded[id]...)
				result[id] = copied
				routingQuotaCache.entries[id] = routingQuotaCacheEntry{rows: copied, loadedAt: now}
			} else if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				routingQuotaCache.entries[id] = routingQuotaCacheEntry{loadedAt: now, err: err}
			}
			for len(routingQuotaCache.entries) > channelRoutingQuotaCacheMax {
				oldestID := 0
				var oldest time.Time
				for candidateID, cached := range routingQuotaCache.entries {
					if oldestID == 0 || cached.loadedAt.Before(oldest) {
						oldestID, oldest = candidateID, cached.loadedAt
					}
				}
				delete(routingQuotaCache.entries, oldestID)
			}
			delete(routingQuotaCache.loading, id)
			close(wait)
			routingQuotaCache.Unlock()
			if err != nil {
				return nil, err
			}
			break
		}
	}
	return result, nil
}

type routingPlan struct {
	preview ChannelRoutingPreview
	pool    []model.ChannelRoutingCandidate
	byID    map[int]model.ChannelRoutingCandidate
	quota   map[int]routingQuotaAssessment
}

type groupedRoutingPlan struct {
	group string
	plan  routingPlan
}

func buildRoutingPlan(candidates []model.ChannelRoutingCandidate, rows map[int][]model.ChannelQuotaSnapshot, workload string, enabled bool, excluded map[int]struct{}, now int64, maxAge int) routingPlan {
	plan := routingPlan{
		preview: ChannelRoutingPreview{Workload: workload, Candidates: make([]ChannelRoutingPreviewCandidate, 0, len(candidates))},
		byID:    make(map[int]model.ChannelRoutingCandidate, len(candidates)), quota: make(map[int]routingQuotaAssessment, len(candidates)),
	}
	eligible := make([]model.ChannelRoutingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		assessment := deriveRoutingQuota(rows[candidate.ID], candidate.IsMultiKey, now, maxAge)
		plan.byID[candidate.ID] = candidate
		plan.quota[candidate.ID] = assessment
		entry := ChannelRoutingPreviewCandidate{ID: candidate.ID, Name: candidate.Name, Reason: "priority_weight", Available: assessment.view.Available, Unit: assessment.view.Unit}
		if _, skip := excluded[candidate.ID]; skip {
			entry.Reason = "already_tried"
		} else if candidate.Status != common.ChannelStatusEnabled {
			entry.Reason = "no_eligible_channel"
		} else if enabled && (workload == ChannelRoutingWorkloadChat || workload == ChannelRoutingWorkloadWork) && assessment.view.State == "exhausted" {
			entry.Reason = "quota_exhausted"
		} else {
			eligible = append(eligible, candidate)
		}
		plan.preview.Candidates = append(plan.preview.Candidates, entry)
	}
	if len(eligible) == 0 {
		plan.preview.Reason = "no_eligible_channel"
		return plan
	}
	maxPriority := eligible[0].Priority
	for _, candidate := range eligible[1:] {
		if candidate.Priority > maxPriority {
			maxPriority = candidate.Priority
		}
	}
	layer := make([]model.ChannelRoutingCandidate, 0, len(eligible))
	for _, candidate := range eligible {
		if candidate.Priority == maxPriority {
			layer = append(layer, candidate)
		} else {
			for i := range plan.preview.Candidates {
				if plan.preview.Candidates[i].ID == candidate.ID {
					plan.preview.Candidates[i].Reason = "lower_priority"
				}
			}
		}
	}
	// Zero means no probability whenever the selected priority layer contains at
	// least one positive weight. The all-zero compatibility fallback is decided
	// from this complete layer, before quota-based pooling can narrow it.
	layerHasPositiveWeight := false
	for _, candidate := range layer {
		if candidate.Weight > 0 {
			layerHasPositiveWeight = true
			break
		}
	}
	if layerHasPositiveWeight {
		positive := layer[:0]
		for _, candidate := range layer {
			if candidate.Weight > 0 {
				positive = append(positive, candidate)
			} else {
				for i := range plan.preview.Candidates {
					if plan.preview.Candidates[i].ID == candidate.ID {
						plan.preview.Candidates[i].Reason = "zero_weight"
					}
				}
			}
		}
		layer = positive
	}
	plan.pool = layer
	plan.preview.Reason = "balanced_path"
	if !enabled {
		plan.preview.Reason = "smart_routing_disabled"
	} else if workload == ChannelRoutingWorkloadChat || workload == ChannelRoutingWorkloadWork {
		comparable := len(layer) >= 2
		comparisonKey := ""
		for _, candidate := range layer {
			assessment := plan.quota[candidate.ID]
			if assessment.view.State != "fresh" || assessment.comparisonKey == "" {
				comparable = false
				break
			}
			if comparisonKey == "" {
				comparisonKey = assessment.comparisonKey
			} else if comparisonKey != assessment.comparisonKey {
				comparable = false
				break
			}
		}
		if comparable {
			sortedLayer := append([]model.ChannelRoutingCandidate(nil), layer...)
			sort.SliceStable(sortedLayer, func(i, j int) bool {
				left, right := plan.quota[sortedLayer[i].ID].available, plan.quota[sortedLayer[j].ID].available
				if left == right {
					return sortedLayer[i].ID < sortedLayer[j].ID
				}
				return left < right
			})
			half := (len(sortedLayer) + 1) / 2
			pool := make([]model.ChannelRoutingCandidate, 0, half)
			reason := "quota_lower_pool"
			if workload == ChannelRoutingWorkloadChat {
				threshold := plan.quota[sortedLayer[half-1].ID].available
				for _, candidate := range sortedLayer {
					if plan.quota[candidate.ID].available <= threshold {
						pool = append(pool, candidate)
					}
				}
				plan.preview.Reason = "chat_lower_remaining_quota"
			} else {
				reason = "quota_higher_pool"
				threshold := plan.quota[sortedLayer[len(sortedLayer)-half].ID].available
				for _, candidate := range sortedLayer {
					if plan.quota[candidate.ID].available >= threshold {
						pool = append(pool, candidate)
					}
				}
				plan.preview.Reason = "work_higher_remaining_quota"
			}
			plan.pool = pool
			selected := make(map[int]struct{}, len(pool))
			for _, candidate := range pool {
				selected[candidate.ID] = struct{}{}
			}
			for i := range plan.preview.Candidates {
				entry := &plan.preview.Candidates[i]
				if _, ok := selected[entry.ID]; ok {
					entry.Reason = reason
				} else if candidate, ok := plan.byID[entry.ID]; ok && candidate.Priority == maxPriority && entry.Reason == "priority_weight" {
					entry.Reason = "quota_not_selected"
				}
			}
		} else {
			plan.preview.Reason = "quota_insufficient_comparable_data"
			for i := range plan.preview.Candidates {
				entry := &plan.preview.Candidates[i]
				candidate := plan.byID[entry.ID]
				if candidate.Priority != maxPriority || entry.Reason != "priority_weight" {
					continue
				}
				switch plan.quota[entry.ID].view.State {
				case "stale":
					entry.Reason = "quota_stale"
				case "unknown":
					entry.Reason = "quota_unknown"
				}
			}
		}
	}

	var weightTotal uint64
	for _, candidate := range plan.pool {
		weight := candidate.Weight
		if weight > model.MaxChannelRoutingWeight {
			weight = model.MaxChannelRoutingWeight
		}
		weightTotal += uint64(weight)
	}
	allZero := weightTotal == 0
	if allZero {
		weightTotal = uint64(len(plan.pool))
	}
	for i := range plan.preview.Candidates {
		entry := &plan.preview.Candidates[i]
		for _, candidate := range plan.pool {
			if candidate.ID != entry.ID {
				continue
			}
			weight := candidate.Weight
			if weight > model.MaxChannelRoutingWeight {
				weight = model.MaxChannelRoutingWeight
			}
			if allZero {
				weight = 1
			}
			entry.Share = float64(weight) / float64(weightTotal)
			break
		}
	}
	return plan
}

func routingChannelIDs(candidates []model.ChannelRoutingCandidate) []int {
	ids := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	return ids
}

func GetChannelRoutingManagementData(ctx context.Context) (ChannelRoutingManagementData, error) {
	channels, err := model.ListChannelRoutingManagementChannels()
	if err != nil {
		return ChannelRoutingManagementData{}, err
	}
	now := time.Now()
	rows, err := loadRoutingQuotaRows(ctx, routingChannelIDs(channels), now, 1000)
	if err != nil {
		logger.LogWarn(nil, "channel routing management quota snapshot read failed; showing unknown quota: "+err.Error())
		rows = make(map[int][]model.ChannelQuotaSnapshot)
	}
	policy := operation_setting.GetChannelRoutingPolicy()
	data := ChannelRoutingManagementData{Policy: policy, Channels: make([]ChannelRoutingManagementChannel, 0, len(channels))}
	for _, channel := range channels {
		quota := deriveRoutingQuota(rows[channel.ID], channel.IsMultiKey, now.Unix(), policy.QuotaMaxAgeSeconds).view
		data.Channels = append(data.Channels, ChannelRoutingManagementChannel{
			ID: channel.ID, Name: channel.Name, Type: channel.Type, Status: channel.Status,
			Priority: channel.Priority, Weight: channel.Weight, LegacyPriority: channel.LegacyPriority, LegacyWeight: channel.LegacyWeight, Quota: quota,
		})
	}
	return data, nil
}

func PreviewChannelRouting(ctx context.Context, modelName, group, requestPath string) (ChannelRoutingPreview, error) {
	candidates, err := model.ListEligibleChannelRoutingCandidates(group, modelName, requestPath)
	if err != nil {
		return ChannelRoutingPreview{}, err
	}
	now := time.Now()
	rows, err := loadRoutingQuotaRows(ctx, routingChannelIDs(candidates), now, 256)
	if err != nil {
		return ChannelRoutingPreview{}, err
	}
	policy := operation_setting.GetChannelRoutingPolicy()
	workload := ChannelRoutingWorkload(requestPath)
	plan := buildRoutingPlan(candidates, rows, workload, policy.Enabled, nil, now.Unix(), policy.QuotaMaxAgeSeconds)
	return plan.preview, nil
}

func extractChannelRoutingSessionID(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	for _, header := range []string{"Session_id", "Thread_id", "Session-Id", "Thread-Id", "X-Conversation-Id"} {
		value := strings.TrimSpace(c.GetHeader(header))
		if value != "" && len(value) <= 1024 {
			return value
		}
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return ""
	}
	body, err := storage.Bytes()
	if err != nil {
		return ""
	}
	for _, path := range []string{"conversation_id"} {
		value := gjson.GetBytes(body, path)
		if value.Type == gjson.String {
			text := strings.TrimSpace(value.String())
			if text != "" && len(text) <= 1024 {
				return text
			}
		}
	}
	return ""
}

func channelRoutingSessionHash(c *gin.Context) string {
	if c == nil {
		return ""
	}
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	sessionID := extractChannelRoutingSessionID(c)
	if userID <= 0 || sessionID == "" {
		return ""
	}
	principal := "token:" + strconv.Itoa(tokenID)
	if tokenID <= 0 {
		if c.Request == nil || c.Request.URL == nil || c.Request.URL.Path != "/pg/chat/completions" {
			return ""
		}
		principal = "playground"
	}
	identity := strconv.Itoa(userID) + "\x1f" + principal + "\x1f" + strconv.Itoa(len(sessionID)) + ":" + sessionID
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

func getChannelRoutingRequestState(c *gin.Context) *channelRoutingRequestState {
	if c == nil {
		return nil
	}
	if existing, ok := c.Get(channelRoutingContextKey); ok {
		if state, ok := existing.(*channelRoutingRequestState); ok {
			return state
		}
	}
	requestPath := ""
	if c.Request != nil && c.Request.URL != nil {
		requestPath = c.Request.URL.Path
	}
	state := &channelRoutingRequestState{Workload: ChannelRoutingWorkload(requestPath)}
	c.Set(channelRoutingContextKey, state)
	return state
}

func selectRoutingPlanChannel(plan routingPlan) (model.ChannelRoutingCandidate, bool) {
	weighted := make([]model.WeightedChannelCandidate, 0, len(plan.pool))
	for _, candidate := range plan.pool {
		weighted = append(weighted, model.WeightedChannelCandidate{ChannelID: candidate.ID, Weight: candidate.Weight})
	}
	selectedID, ok := model.SelectWeightedChannel(weighted)
	if !ok {
		return model.ChannelRoutingCandidate{}, false
	}
	candidate, ok := plan.byID[selectedID]
	return candidate, ok
}

func routingCandidateUsable(plan routingPlan, channelID int, excluded map[int]struct{}) (model.ChannelRoutingCandidate, bool) {
	if _, skip := excluded[channelID]; skip {
		return model.ChannelRoutingCandidate{}, false
	}
	candidate, ok := plan.byID[channelID]
	if !ok || candidate.Status != common.ChannelStatusEnabled || plan.quota[channelID].view.State == "exhausted" {
		return model.ChannelRoutingCandidate{}, false
	}
	return candidate, true
}

func SelectSmartChannelRouting(param *RetryParam) (*model.Channel, string, error) {
	if param == nil || param.Ctx == nil {
		return nil, "", fmt.Errorf("routing request is missing context")
	}
	policy := operation_setting.GetChannelRoutingPolicy()
	state := getChannelRoutingRequestState(param.Ctx)
	workload := ChannelRoutingWorkload(param.RequestPath)
	groups := []string{param.TokenGroup}
	if param.TokenGroup == "auto" {
		groups = GetRequestAutoGroups(param.Ctx, common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup))
		if len(groups) == 0 {
			return nil, param.TokenGroup, fmt.Errorf("auto groups is not enabled")
		}
		if len(param.excludedChannelIDs) > 0 && state.SelectedGroup != "" {
			currentIndex := -1
			for i, group := range groups {
				if group == state.SelectedGroup {
					currentIndex = i
					break
				}
			}
			if currentIndex >= 0 {
				if common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry) {
					groups = groups[currentIndex:]
				} else {
					groups = groups[currentIndex : currentIndex+1]
				}
			}
		}
	}
	now := time.Now()
	plans := make([]groupedRoutingPlan, 0, len(groups))
	allIDs := make([]int, 0)
	allCandidates := make(map[string][]model.ChannelRoutingCandidate, len(groups))
	for _, group := range groups {
		candidates, err := model.ListEligibleChannelRoutingCandidates(group, param.ModelName, param.RequestPath)
		if err != nil {
			return nil, group, err
		}
		allCandidates[group] = candidates
		allIDs = append(allIDs, routingChannelIDs(candidates)...)
	}
	rows, err := loadRoutingQuotaRows(param.Ctx.Request.Context(), allIDs, now, 256)
	quotaReadFailed := err != nil
	if err != nil {
		// Quota observations are an optimization input. Keep the established
		// priority/weight route available when their local read fails.
		rows = make(map[int][]model.ChannelQuotaSnapshot)
		logger.LogWarn(param.Ctx, "channel routing quota snapshot read failed; using priority and weight: "+err.Error())
	}
	for _, group := range groups {
		plan := buildRoutingPlan(allCandidates[group], rows, workload, true, param.excludedChannelIDs, now.Unix(), policy.QuotaMaxAgeSeconds)
		plans = append(plans, groupedRoutingPlan{group: group, plan: plan})
	}

	var proposed model.ChannelRoutingCandidate
	selectedGroup := ""
	selectedReason := "no_eligible_channel"
	for _, item := range plans {
		candidate, ok := selectRoutingPlanChannel(item.plan)
		if !ok {
			continue
		}
		proposed = candidate
		selectedGroup = item.group
		selectedReason = item.plan.preview.Reason
		break
	}
	if proposed.ID == 0 {
		return nil, param.TokenGroup, nil
	}
	if quotaReadFailed {
		selectedReason = "quota_snapshot_unavailable"
	}

	selected := proposed
	bindingHash := ""
	if policy.StickyEnabled {
		bindingHash = channelRoutingSessionHash(param.Ctx)
	}
	if bindingHash != "" {
		expiresAt := now.Unix() + int64(policy.SessionTTLSeconds)
		binding, claimErr := model.ClaimChannelRoutingSession(param.Ctx.Request.Context(), bindingHash, proposed.ID, now.Unix(), expiresAt)
		if claimErr != nil {
			logger.LogWarn(param.Ctx, "channel routing session claim failed; continuing without sticky binding: "+claimErr.Error())
		} else {
			state.SessionBound = true
			state.Binding = binding
			if routingSessionCleanupCounter.Add(1)%128 == 0 {
				if _, cleanupErr := model.DeleteExpiredChannelRoutingSessions(param.Ctx.Request.Context(), now.Unix(), 256); cleanupErr != nil {
					logger.LogWarn(param.Ctx, "channel routing session cleanup failed: "+cleanupErr.Error())
				}
			}
			newBinding := binding.ChannelId == proposed.ID && binding.CreatedAt == now.Unix()
			for attempt := 0; attempt < 4; attempt++ {
				found := false
				for _, item := range plans {
					if candidate, ok := routingCandidateUsable(item.plan, binding.ChannelId, param.excludedChannelIDs); ok {
						selected, selectedGroup, found = candidate, item.group, true
						break
					}
				}
				if found {
					state.Binding = binding
					if !newBinding {
						selectedReason = "session_binding"
					}
					break
				}
				if state.SwitchReason == "" {
					if assessment, ok := planQuotaForChannel(plans, binding.ChannelId); ok && assessment.view.State == "exhausted" {
						state.SwitchReason = "quota_exhausted"
					} else {
						state.SwitchReason = "binding_ineligible"
					}
				}
				current, switched, switchErr := model.SwitchChannelRoutingSession(param.Ctx.Request.Context(), binding, proposed.ID, now.Unix(), expiresAt)
				if switchErr != nil {
					logger.LogWarn(param.Ctx, "channel routing session switch failed; continuing without sticky binding: "+switchErr.Error())
					state.SessionBound = false
					state.Binding = model.ChannelRoutingSession{}
					selected = proposed
					break
				}
				binding = current
				newBinding = false
				state.Binding = current
				if switched {
					selected = proposed
					state.SwitchCount++
					break
				}
				if attempt == 3 {
					logger.LogWarn(param.Ctx, "channel routing binding changed repeatedly; using current weighted candidate without sticky binding")
					state.SessionBound = false
					state.Binding = model.ChannelRoutingSession{}
					selected = proposed
				}
			}
		}
	}

	channel, err := model.CacheGetChannel(selected.ID)
	if err != nil {
		if (common.MemoryCacheEnabled || errors.Is(err, gorm.ErrRecordNotFound)) && state.SessionBound && state.Binding.KeyHash != "" {
			if expireErr := model.ExpireChannelRoutingSessionIfCurrent(param.Ctx.Request.Context(), state.Binding, now.Unix()); expireErr != nil {
				logger.LogWarn(param.Ctx, "channel routing missing-channel binding cleanup failed: "+expireErr.Error())
			}
		}
		return nil, selectedGroup, err
	}
	state.Workload = workload
	state.Reason = selectedReason
	state.SelectedGroup = selectedGroup
	state.SelectedChannel = selected.ID
	state.AttemptOutcome = "pending"
	if param.TokenGroup == "auto" {
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, selectedGroup)
	}
	return channel, selectedGroup, nil
}

func planQuotaForChannel(plans []groupedRoutingPlan, channelID int) (routingQuotaAssessment, bool) {
	for _, item := range plans {
		assessment, ok := item.plan.quota[channelID]
		if ok {
			return assessment, true
		}
	}
	return routingQuotaAssessment{}, false
}

func MarkSmartChannelRoutingFailure(c *gin.Context, channelID int, reason string) {
	state := existingChannelRoutingRequestState(c)
	if state == nil {
		return
	}
	state.SwitchReason = reason
	state.SelectedChannel = channelID
	state.AttemptOutcome = "channel_failure"
}

func MarkSmartChannelRoutingSuccess(c *gin.Context, channelID int) {
	state := existingChannelRoutingRequestState(c)
	if state == nil {
		return
	}
	state.SelectedChannel = channelID
	state.AttemptOutcome = "success"
}

func CompleteSmartChannelRouting(c *gin.Context) {
	state := existingChannelRoutingRequestState(c)
	if state == nil || !state.SessionBound || state.Binding.KeyHash == "" {
		return
	}
	policy := operation_setting.GetChannelRoutingPolicy()
	now := time.Now().Unix()
	if state.AttemptOutcome == "success" {
		if err := model.TouchChannelRoutingSession(c.Request.Context(), state.Binding, now, now+int64(policy.SessionTTLSeconds)); err != nil {
			logger.LogWarn(c, "channel routing session confirmation failed: "+err.Error())
		}
		return
	}
	if state.AttemptOutcome != "channel_failure" {
		return
	}
	if err := model.ExpireChannelRoutingSessionIfCurrent(c.Request.Context(), state.Binding, now); err != nil {
		logger.LogWarn(c, "channel routing session failure update failed: "+err.Error())
	}
}

func AppendChannelRoutingAdminInfo(c *gin.Context, adminInfo map[string]interface{}) {
	if c == nil || adminInfo == nil {
		return
	}
	state := existingChannelRoutingRequestState(c)
	if state == nil || state.SelectedChannel == 0 {
		return
	}
	info := map[string]interface{}{
		"workload": state.Workload, "reason": state.Reason,
		"channel_id": state.SelectedChannel, "group": state.SelectedGroup,
		"sticky": state.SessionBound,
	}
	if state.SwitchCount > 0 {
		info["switch_count"] = state.SwitchCount
	}
	if state.SwitchReason != "" {
		info["switch_reason"] = state.SwitchReason
	}
	if state.AttemptOutcome != "" {
		info["outcome"] = state.AttemptOutcome
	}
	adminInfo["channel_routing"] = info
}

func existingChannelRoutingRequestState(c *gin.Context) *channelRoutingRequestState {
	if c == nil {
		return nil
	}
	value, ok := c.Get(channelRoutingContextKey)
	if !ok {
		return nil
	}
	state, _ := value.(*channelRoutingRequestState)
	return state
}

func IsSmartChannelRoutingRequest(c *gin.Context) bool {
	state := existingChannelRoutingRequestState(c)
	return state != nil && state.SelectedChannel > 0
}
