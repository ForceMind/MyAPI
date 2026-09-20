package model

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"gorm.io/gorm"
)

var group2model2channels map[string]map[string][]int // enabled channel IDs from abilities
var group2model2routingCandidates map[string]map[string][]ChannelRoutingCandidate
var channelsIDM map[int]*Channel // all channels include disabled
// channel2advancedCustomConfig caches parsed Advanced Custom (type 58) configs so
// path-aware selection avoids re-parsing JSON per request. Refreshed on full sync.
var channel2advancedCustomConfig map[int]*dto.AdvancedCustomConfig
var channelSyncLock sync.RWMutex
var channelCacheRefreshLock sync.Mutex
var channelCacheDataGeneration uint64
var channelCachePublishGeneration uint64
var channelCacheObservedCommittedEpoch int64
var channelCachePublishedEpoch int64
var channelCacheDB *gorm.DB

const (
	ChannelRoutingSourceCache    = "cache"
	ChannelRoutingSourceDatabase = "database"
)

// ChannelCachePublicationState reports whether the runtime routing cache has
// published the latest committed channel/ability generation.
type ChannelCachePublicationState struct {
	DataGeneration        uint64 `json:"data_generation"`
	PublishedGeneration   uint64 `json:"published_generation"`
	ClusterCommittedEpoch int64  `json:"cluster_committed_epoch"`
	LocalPublishedEpoch   int64  `json:"local_published_epoch"`
	CacheEnabled          bool   `json:"cache_enabled"`
	CachePending          bool   `json:"cache_pending"`
}

// ChannelCacheRefreshError means the database mutation committed, but the
// runtime cache could not publish that committed generation.
type ChannelCacheRefreshError struct {
	Err error
}

func (e *ChannelCacheRefreshError) Error() string {
	return "channel data committed but cache refresh failed: " + e.Err.Error()
}

func (e *ChannelCacheRefreshError) Unwrap() error {
	return e.Err
}

func IsChannelCacheRefreshError(err error) bool {
	var refreshErr *ChannelCacheRefreshError
	return errors.As(err, &refreshErr)
}

// channelCredentialCacheGeneration fences full-cache snapshots against
// credential writes that commit while InitChannelCache is reading the DB.
// It is protected by channelSyncLock.
var channelCredentialCacheGeneration uint64

type channelCacheSnapshotRow struct {
	Channel
	AbilityChannelID *int    `gorm:"column:ability_channel_id"`
	AbilityGroup     *string `gorm:"column:ability_group"`
	AbilityModel     *string `gorm:"column:ability_model"`
	AbilityEnabled   *bool   `gorm:"column:ability_enabled"`
	AbilityPriority  *int64  `gorm:"column:ability_priority"`
	AbilityWeight    *uint   `gorm:"column:ability_weight"`
}

func InitChannelCache() error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	channelSyncLock.Lock()
	if channelCacheDB != DB {
		channelCacheDB = DB
		channelCacheObservedCommittedEpoch = 0
		channelCachePublishedEpoch = 0
		channelCacheDataGeneration = 0
		channelCachePublishGeneration = 0
	}
	channelSyncLock.Unlock()
	if err := ensureChannelRoutingSchema(DB); err != nil {
		return err
	}
	if !common.MemoryCacheEnabled {
		if committedEpoch, err := GetCommittedChannelRoutingEpoch(DB); err == nil {
			observeCommittedChannelRoutingEpoch(committedEpoch)
		}
		InvalidatePricingCache()
		return nil
	}

	channelCacheRefreshLock.Lock()
	defer channelCacheRefreshLock.Unlock()

	for {
		channelSyncLock.RLock()
		credentialGeneration := channelCredentialCacheGeneration
		channelSyncLock.RUnlock()

		rows, snapshotEpoch, err := loadChannelCacheSnapshot()
		if err != nil {
			common.SysError("failed to load channel cache snapshot: " + err.Error())
			return err
		}
		newChannelIDToChannel, newGroup2Model2Channels, newGroup2Model2RoutingCandidates, newChannel2AdvancedCustomConfig := buildChannelCacheSnapshot(rows)

		latestEpoch, err := GetCommittedChannelRoutingEpoch(DB)
		if err != nil {
			return err
		}
		if latestEpoch != snapshotEpoch {
			continue
		}

		channelSyncLock.Lock()
		if channelCredentialCacheGeneration != credentialGeneration {
			channelSyncLock.Unlock()
			continue
		}
		for id, channel := range newChannelIDToChannel {
			if channel.ChannelInfo.IsMultiKey {
				channel.Keys = channel.GetKeys()
				if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
					if oldChannel, ok := channelsIDM[id]; ok && oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
		group2model2channels = newGroup2Model2Channels
		group2model2routingCandidates = newGroup2Model2RoutingCandidates
		channelsIDM = newChannelIDToChannel
		channel2advancedCustomConfig = newChannel2AdvancedCustomConfig
		if snapshotEpoch > channelCacheObservedCommittedEpoch {
			channelCacheObservedCommittedEpoch = snapshotEpoch
		}
		channelCachePublishedEpoch = snapshotEpoch
		channelCacheDataGeneration = uint64(channelCacheObservedCommittedEpoch)
		channelCachePublishGeneration = uint64(channelCachePublishedEpoch)
		channelSyncLock.Unlock()
		break
	}

	// Lock ordering: InvalidatePricingCache acquires updatePricingLock, and
	// GetPricing (holding updatePricingLock) nests channelSyncLock.RLock via
	// loadPricingAdvancedCustomConfigs. channelSyncLock MUST be released before
	// invalidating the pricing cache, otherwise the reversed order deadlocks.
	InvalidatePricingCache()
	common.SysLog("channels synced from database")
	return nil
}

func loadChannelCacheSnapshot() ([]channelCacheSnapshotRow, int64, error) {
	groupColumn := "`group`"
	if DB.Dialector.Name() == "postgres" {
		groupColumn = `"group"`
	}
	selectColumns := "channels.*, " +
		"abilities.channel_id AS ability_channel_id, " +
		"abilities." + groupColumn + " AS ability_group, " +
		"abilities.model AS ability_model, " +
		"abilities.enabled AS ability_enabled, " +
		"abilities.priority AS ability_priority, " +
		"abilities.weight AS ability_weight"
	for attempt := 0; attempt < 16; attempt++ {
		beforeEpoch, err := GetCommittedChannelRoutingEpoch(DB)
		if err != nil {
			return nil, 0, err
		}
		rows := make([]channelCacheSnapshotRow, 0)
		if err := DB.Table("channels").
			Select(selectColumns).
			Joins("LEFT JOIN abilities ON abilities.channel_id = channels.id").
			Order("channels.id ASC").
			Find(&rows).Error; err != nil {
			return nil, 0, err
		}
		afterEpoch, err := GetCommittedChannelRoutingEpoch(DB)
		if err != nil {
			return nil, 0, err
		}
		if beforeEpoch == afterEpoch {
			return rows, afterEpoch, nil
		}
	}
	return nil, 0, errors.New("channel routing snapshot changed too frequently")
}

func buildChannelCacheSnapshot(rows []channelCacheSnapshotRow) (
	map[int]*Channel,
	map[string]map[string][]int,
	map[string]map[string][]ChannelRoutingCandidate,
	map[int]*dto.AdvancedCustomConfig,
) {
	newChannelIDToChannel := make(map[int]*Channel)
	newChannel2AdvancedCustomConfig := make(map[int]*dto.AdvancedCustomConfig)
	for i := range rows {
		row := &rows[i]
		if _, exists := newChannelIDToChannel[row.Id]; exists {
			continue
		}
		channelCopy := row.Channel
		newChannelIDToChannel[row.Id] = &channelCopy
		if row.Type != constant.ChannelTypeAdvancedCustom {
			continue
		}
		config, err := parseChannelAdvancedCustomRoutingConfig(&channelCopy)
		if err == nil && config != nil {
			newChannel2AdvancedCustomConfig[row.Id] = config
		}
	}

	newGroup2Model2Channels := make(map[string]map[string][]int)
	newGroup2Model2RoutingCandidates := make(map[string]map[string][]ChannelRoutingCandidate)
	seenCandidates := make(map[string]map[string]map[int]struct{})
	for i := range rows {
		row := &rows[i]
		if row.AbilityChannelID == nil || row.AbilityGroup == nil || row.AbilityModel == nil || row.AbilityEnabled == nil || !*row.AbilityEnabled {
			continue
		}
		channel, ok := newChannelIDToChannel[*row.AbilityChannelID]
		if !ok || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		if channel.Type == constant.ChannelTypeAdvancedCustom && newChannel2AdvancedCustomConfig[channel.Id] == nil {
			continue
		}

		group := *row.AbilityGroup
		modelName := *row.AbilityModel
		if newGroup2Model2Channels[group] == nil {
			newGroup2Model2Channels[group] = make(map[string][]int)
			newGroup2Model2RoutingCandidates[group] = make(map[string][]ChannelRoutingCandidate)
			seenCandidates[group] = make(map[string]map[int]struct{})
		}
		if seenCandidates[group][modelName] == nil {
			seenCandidates[group][modelName] = make(map[int]struct{})
		}
		if _, duplicate := seenCandidates[group][modelName][channel.Id]; duplicate {
			continue
		}
		seenCandidates[group][modelName][channel.Id] = struct{}{}

		priority := int64(0)
		if row.AbilityPriority != nil {
			priority = *row.AbilityPriority
		}
		weight := uint(0)
		if row.AbilityWeight != nil {
			weight = *row.AbilityWeight
		}
		newGroup2Model2Channels[group][modelName] = append(newGroup2Model2Channels[group][modelName], channel.Id)
		newGroup2Model2RoutingCandidates[group][modelName] = append(
			newGroup2Model2RoutingCandidates[group][modelName],
			ChannelRoutingCandidate{
				ChannelID:   channel.Id,
				ChannelName: channel.Name,
				ChannelType: channel.Type,
				Priority:    priority,
				Weight:      weight,
			},
		)
	}

	for group, model2Channels := range newGroup2Model2Channels {
		for modelName, channelIDs := range model2Channels {
			sort.Ints(channelIDs)
			newGroup2Model2Channels[group][modelName] = channelIDs
			candidates := newGroup2Model2RoutingCandidates[group][modelName]
			sort.Slice(candidates, func(i, j int) bool {
				if candidates[i].Priority != candidates[j].Priority {
					return candidates[i].Priority > candidates[j].Priority
				}
				return candidates[i].ChannelID < candidates[j].ChannelID
			})
			newGroup2Model2RoutingCandidates[group][modelName] = candidates
		}
	}
	return newChannelIDToChannel, newGroup2Model2Channels, newGroup2Model2RoutingCandidates, newChannel2AdvancedCustomConfig
}

func GetChannelCachePublicationState() ChannelCachePublicationState {
	state, _ := LoadChannelCachePublicationState()
	return state
}

func LoadChannelCachePublicationState() (ChannelCachePublicationState, error) {
	if DB == nil {
		channelSyncLock.RLock()
		state := channelCachePublicationStateLocked()
		channelSyncLock.RUnlock()
		return state, gorm.ErrInvalidDB
	}
	committedEpoch, err := GetCommittedChannelRoutingEpoch(DB)
	if err != nil {
		channelSyncLock.RLock()
		state := channelCachePublicationStateLocked()
		channelSyncLock.RUnlock()
		return state, err
	}
	observeCommittedChannelRoutingEpoch(committedEpoch)
	channelSyncLock.RLock()
	state := channelCachePublicationStateLocked()
	channelSyncLock.RUnlock()
	return state, nil
}

func channelCachePublicationStateLocked() ChannelCachePublicationState {
	return ChannelCachePublicationState{
		DataGeneration:        uint64(channelCacheObservedCommittedEpoch),
		PublishedGeneration:   uint64(channelCachePublishedEpoch),
		ClusterCommittedEpoch: channelCacheObservedCommittedEpoch,
		LocalPublishedEpoch:   channelCachePublishedEpoch,
		CacheEnabled:          common.MemoryCacheEnabled,
		CachePending:          common.MemoryCacheEnabled && channelCachePublishedEpoch < channelCacheObservedCommittedEpoch,
	}
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		if err := InitChannelCache(); err != nil {
			common.SysError("failed to sync channels from database: " + err.Error())
		}
	}
}

func GetRandomSatisfiedChannel(group string, modelName string, retry int, requestPath string) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannel(group, modelName, retry, requestPath)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	policy := getCachedChannelRoutingPolicy(group, modelName, requestPath)
	channelID, found, err := SelectChannelFromRoutingPolicy(policy, retry)
	if err != nil || !found {
		return nil, err
	}
	channel, ok := channelsIDM[channelID]
	if !ok {
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelID)
	}
	return channel, nil
}

func getCachedChannelRoutingPolicy(group string, modelName string, requestPath string) ChannelRoutingPolicy {
	candidates := filterRoutingCandidatesByRequestPathAndModel(
		group2model2routingCandidates[group][modelName],
		requestPath,
		modelName,
	)
	if len(candidates) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(modelName)
		if normalizedModel != "" && normalizedModel != modelName {
			candidates = filterRoutingCandidatesByRequestPathAndModel(
				group2model2routingCandidates[group][normalizedModel],
				requestPath,
				modelName,
			)
		}
	}
	return BuildChannelRoutingPolicy(candidates)
}

// filterRoutingCandidatesByRequestPathAndModel excludes Advanced Custom
// channels whose settings are missing, invalid, or do not serve the requested
// path/model. The cached candidate slice is never mutated.
func filterRoutingCandidatesByRequestPathAndModel(candidates []ChannelRoutingCandidate, requestPath string, modelName string) []ChannelRoutingCandidate {
	if len(candidates) == 0 {
		return candidates
	}
	filtered := make([]ChannelRoutingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		channel, ok := channelsIDM[candidate.ChannelID]
		if !ok {
			// Keep the candidate so the downstream consistency error is preserved.
			filtered = append(filtered, candidate)
			continue
		}
		if channel.Type != constant.ChannelTypeAdvancedCustom {
			filtered = append(filtered, candidate)
			continue
		}
		config := channel2advancedCustomConfig[candidate.ChannelID]
		if config == nil {
			continue
		}
		if requestPath == "" || config.SupportsPathForModel(requestPath, modelName) {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func removeChannelFromRoutingCacheLocked(channelID int) {
	for group, model2Channels := range group2model2channels {
		for modelName, channelIDs := range model2Channels {
			filtered := channelIDs[:0]
			for _, id := range channelIDs {
				if id != channelID {
					filtered = append(filtered, id)
				}
			}
			group2model2channels[group][modelName] = filtered
		}
	}
	for group, model2Candidates := range group2model2routingCandidates {
		for modelName, candidates := range model2Candidates {
			filtered := candidates[:0]
			for _, candidate := range candidates {
				if candidate.ChannelID != channelID {
					filtered = append(filtered, candidate)
				}
			}
			group2model2routingCandidates[group][modelName] = filtered
		}
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	if channel == nil {
		channelSyncLock.Unlock()
		return
	}

	if channelsIDM == nil {
		channelsIDM = make(map[int]*Channel)
	}
	if oldChannel, ok := channelsIDM[channel.Id]; ok {
		logger.LogDebug(nil, "CacheUpdateChannel before: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, oldChannel.ChannelInfo.MultiKeyPollingIndex)
		if oldChannel.Key != channel.Key {
			channelCredentialCacheGeneration++
		}
	}
	channelsIDM[channel.Id] = channel
	if channel2advancedCustomConfig == nil {
		channel2advancedCustomConfig = make(map[int]*dto.AdvancedCustomConfig)
	}
	delete(channel2advancedCustomConfig, channel.Id)
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		config, err := parseChannelAdvancedCustomRoutingConfig(channel)
		if err == nil && config != nil {
			channel2advancedCustomConfig[channel.Id] = config
		}
	}
	if channel.Status != common.ChannelStatusEnabled {
		removeChannelFromRoutingCacheLocked(channel.Id)
	} else {
		for _, model2Candidates := range group2model2routingCandidates {
			for modelName, candidates := range model2Candidates {
				for i := range candidates {
					if candidates[i].ChannelID == channel.Id {
						candidates[i].ChannelName = channel.Name
						candidates[i].ChannelType = channel.Type
					}
				}
				model2Candidates[modelName] = candidates
			}
		}
	}
	channelCachePublishedEpoch = channelCacheObservedCommittedEpoch
	channelCacheDataGeneration = uint64(channelCacheObservedCommittedEpoch)
	channelCachePublishGeneration = uint64(channelCachePublishedEpoch)
	logger.LogDebug(nil, "CacheUpdateChannel after: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)
	// Lock ordering: do NOT hold channelSyncLock while calling
	// InvalidatePricingCache. GetPricing acquires updatePricingLock first and then
	// channelSyncLock.RLock (via loadPricingAdvancedCustomConfigs); acquiring
	// updatePricingLock while holding channelSyncLock would be an AB-BA deadlock.
	channelSyncLock.Unlock()
	InvalidatePricingCache()
}

// CacheUpdateChannelKeyWithContext replaces only the credential in an existing
// cache entry. It performs no database/pricing rebuild and cannot keep a quota
// sampling lock waiting indefinitely for a concurrent full cache refresh.
func CacheUpdateChannelKeyWithContext(ctx context.Context, id int, key string) error {
	if !common.MemoryCacheEnabled {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if channelSyncLock.TryLock() {
			defer channelSyncLock.Unlock()
			if channel, ok := channelsIDM[id]; ok {
				updated := *channel
				updated.Key = key
				updated.Keys = nil
				channelsIDM[id] = &updated
				channelCredentialCacheGeneration++
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
