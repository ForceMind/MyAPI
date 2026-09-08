package model

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"gorm.io/gorm"
)

const (
	MinChannelRoutingPriority int64 = -1_000_000
	MaxChannelRoutingPriority int64 = 1_000_000
)

var ErrChannelRoutingConflict = errors.New("channel routing values changed")

func boundedChannelRoutingPriority(priority int64) int64 {
	if priority < MinChannelRoutingPriority {
		return MinChannelRoutingPriority
	}
	if priority > MaxChannelRoutingPriority {
		return MaxChannelRoutingPriority
	}
	return priority
}

type ChannelRoutingCandidate struct {
	ID         int
	Name       string
	Type       int
	Status     int
	Priority   int64
	Weight     uint
	IsMultiKey bool
	// LegacyPriority is populated only by the redacted management projection
	// when a stored priority is outside the bounded editor range.
	LegacyPriority string
	LegacyWeight   string
}

func channelRoutingCandidate(channel *Channel, priority *int64, weight uint) ChannelRoutingCandidate {
	value := int64(0)
	if priority != nil {
		value = *priority
	}
	return ChannelRoutingCandidate{
		ID: channel.Id, Name: channel.Name, Type: channel.Type, Status: channel.Status,
		Priority: value, Weight: weight, IsMultiKey: channel.ChannelInfo.IsMultiKey,
	}
}

// ListEligibleChannelRoutingCandidates mirrors the exact-model-then-normalized
// lookup and path filtering used by the existing selector while returning a
// detached, credential-free view.
func ListEligibleChannelRoutingCandidates(group, modelName, requestPath string) ([]ChannelRoutingCandidate, error) {
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		defer channelSyncLock.RUnlock()
		ids := filterChannelsByRequestPathAndModel(group2model2channels[group][modelName], requestPath, modelName)
		if len(ids) == 0 {
			normalized := ratio_setting.FormatMatchingModelName(modelName)
			ids = filterChannelsByRequestPathAndModel(group2model2channels[group][normalized], requestPath, modelName)
		}
		result := make([]ChannelRoutingCandidate, 0, len(ids))
		for _, id := range ids {
			channel, ok := channelsIDM[id]
			if !ok {
				return nil, errors.New("channel cache is inconsistent")
			}
			weight := uint(0)
			if channel.Weight != nil {
				weight = *channel.Weight
			}
			result = append(result, channelRoutingCandidate(channel, channel.Priority, weight))
		}
		return result, nil
	}

	loadAbilities := func(candidateModel string) ([]Ability, error) {
		var abilities []Ability
		err := DB.Where(&Ability{Group: group, Model: candidateModel, Enabled: true}).
			Order("priority DESC, channel_id ASC").Find(&abilities).Error
		if err != nil {
			return nil, err
		}
		return filterAbilitiesByRequestPathAndModel(abilities, requestPath, modelName), nil
	}
	abilities, err := loadAbilities(modelName)
	if err != nil {
		return nil, err
	}
	if len(abilities) == 0 {
		normalized := ratio_setting.FormatMatchingModelName(modelName)
		if normalized != "" && normalized != modelName {
			abilities, err = loadAbilities(normalized)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(abilities) == 0 {
		return []ChannelRoutingCandidate{}, nil
	}
	ids := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		ids = append(ids, ability.ChannelId)
	}
	var channels []Channel
	if err := DB.Where("id IN ? AND status = ?", ids, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return nil, err
	}
	byID := make(map[int]*Channel, len(channels))
	for i := range channels {
		byID[channels[i].Id] = &channels[i]
	}
	result := make([]ChannelRoutingCandidate, 0, len(abilities))
	for _, ability := range abilities {
		if channel := byID[ability.ChannelId]; channel != nil {
			result = append(result, channelRoutingCandidate(channel, ability.Priority, ability.Weight))
		}
	}
	return result, nil
}

func ListChannelRoutingManagementChannels() ([]ChannelRoutingCandidate, error) {
	var channels []Channel
	if err := DB.Select("id", "name", "type", "status", "priority", "weight", "channel_info").
		Order("id ASC").Find(&channels).Error; err != nil {
		return nil, err
	}
	result := make([]ChannelRoutingCandidate, 0, len(channels))
	for i := range channels {
		weight := uint(0)
		if channels[i].Weight != nil {
			weight = *channels[i].Weight
		}
		legacyWeight := ""
		if weight > MaxChannelRoutingWeight {
			legacyWeight = strconv.FormatUint(uint64(weight), 10)
			weight = MaxChannelRoutingWeight
		}
		candidate := channelRoutingCandidate(&channels[i], channels[i].Priority, weight)
		candidate.LegacyWeight = legacyWeight
		if candidate.Priority < MinChannelRoutingPriority || candidate.Priority > MaxChannelRoutingPriority {
			candidate.LegacyPriority = strconv.FormatInt(candidate.Priority, 10)
			candidate.Priority = boundedChannelRoutingPriority(candidate.Priority)
		}
		result = append(result, candidate)
	}
	return result, nil
}

// ListRecentChannelQuotaSnapshotsForRouting reads only a small newest slice per
// channel. The caller derives latest-per-series state and keeps a short cache;
// this method never invokes an upstream quota endpoint.
func ListRecentChannelQuotaSnapshotsForRouting(ctx context.Context, channelIDs []int, perChannelLimit int) (map[int][]ChannelQuotaSnapshot, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if perChannelLimit <= 0 || perChannelLimit > 64 {
		perChannelLimit = 32
	}
	unique := make(map[int]struct{}, len(channelIDs))
	ids := make([]int, 0, len(channelIDs))
	for _, id := range channelIDs {
		if id <= 0 {
			continue
		}
		if _, ok := unique[id]; ok {
			continue
		}
		unique[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Ints(ids)
	rows := make(map[int][]ChannelQuotaSnapshot, len(ids))
	for _, id := range ids {
		var snapshots []ChannelQuotaSnapshot
		if err := DB.WithContext(ctx).Where("channel_id = ?", id).
			Order("observed_at DESC, id DESC").Limit(perChannelLimit).Find(&snapshots).Error; err != nil {
			return nil, err
		}
		rows[id] = snapshots
	}
	return rows, nil
}

func UpdateChannelRoutingValuesCAS(channelID int, priority int64, weight uint, expectedPriority int64, expectedWeight uint) (*Channel, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var updated Channel
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", channelID).First(&updated).Error; err != nil {
			return err
		}
		currentPriority := updated.GetPriority()
		currentWeight := uint(0)
		if updated.Weight != nil {
			currentWeight = *updated.Weight
		}
		// Both legacy values use exact decimal-string CAS tokens in the management
		// API, while new values remain bounded.
		if currentPriority != expectedPriority || currentWeight != expectedWeight {
			return ErrChannelRoutingConflict
		}
		updated.Priority = &priority
		updated.Weight = &weight
		if err := tx.Model(&Channel{}).Where("id = ?", channelID).
			Select("priority", "weight").Updates(&updated).Error; err != nil {
			return err
		}
		return updated.UpdateAbilities(tx)
	})
	if err != nil {
		return nil, err
	}
	markChannelCacheMutation()
	if err := initChannelCache(); err != nil {
		return nil, err
	}
	return &updated, nil
}

func RoutingComparisonKey(parts ...string) string {
	return strings.Join(parts, "|")
}
