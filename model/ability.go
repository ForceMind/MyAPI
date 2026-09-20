package model

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"

	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Ability struct {
	Group     string  `json:"group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model     string  `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	ChannelId int     `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool    `json:"enabled"`
	Priority  *int64  `json:"priority" gorm:"bigint;default:0;index"`
	Weight    uint    `json:"weight" gorm:"default:0;index"`
	Tag       *string `json:"tag" gorm:"index"`
}

type AbilityWithChannel struct {
	Ability
	ChannelType int `json:"channel_type"`
}

func GetAllEnableAbilityWithChannels() ([]AbilityWithChannel, error) {
	var abilities []AbilityWithChannel
	err := DB.Table("abilities").
		Select("abilities.*, channels.type as channel_type").
		Joins("left join channels on abilities.channel_id = channels.id").
		Where("abilities.enabled = ?", true).
		Scan(&abilities).Error
	return abilities, err
}

func GetGroupEnabledModels(group string) []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where(commonGroupCol+" = ? and enabled = ?", group, true).Distinct("model").Pluck("model", &models)
	return models
}

func GetEnabledModels() []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where("enabled = ?", true).Distinct("model").Pluck("model", &models)
	return models
}

func GetAllEnableAbilities() []Ability {
	var abilities []Ability
	DB.Find(&abilities, "enabled = ?", true)
	return abilities
}

func GetChannelRoutingPolicy(group string, modelName string, requestPath string) (ChannelRoutingPolicy, error) {
	candidates, _, err := loadChannelRoutingCandidates(group, modelName, requestPath)
	if err != nil {
		return ChannelRoutingPolicy{}, err
	}
	return BuildChannelRoutingPolicy(candidates), nil
}

// GetRuntimeChannelRoutingPolicy returns the same routing view used by runtime
// selection. Cache-backed reads use one last-good published snapshot; database
// reads use one joined statement for channels and abilities.
func GetRuntimeChannelRoutingPolicy(group string, modelName string, requestPath string) (ChannelRoutingPolicySnapshot, error) {
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		policy := getCachedChannelRoutingPolicy(group, modelName, requestPath)
		localPublishedEpoch := channelCachePublishedEpoch
		channelSyncLock.RUnlock()

		clusterCommittedEpoch, err := GetCommittedChannelRoutingEpoch(DB)
		if err != nil {
			return ChannelRoutingPolicySnapshot{}, err
		}
		observeCommittedChannelRoutingEpoch(clusterCommittedEpoch)
		return ChannelRoutingPolicySnapshot{
			Policy:                policy,
			Source:                ChannelRoutingSourceCache,
			Generation:            localPublishedEpoch,
			DataGeneration:        uint64(clusterCommittedEpoch),
			PublishedGeneration:   uint64(localPublishedEpoch),
			ClusterCommittedEpoch: clusterCommittedEpoch,
			LocalPublishedEpoch:   localPublishedEpoch,
			CacheEnabled:          true,
			CachePending:          localPublishedEpoch < clusterCommittedEpoch,
		}, nil
	}

	candidates, snapshotEpoch, err := loadChannelRoutingCandidates(group, modelName, requestPath)
	if err != nil {
		return ChannelRoutingPolicySnapshot{}, err
	}
	channelSyncLock.RLock()
	localPublishedEpoch := channelCachePublishedEpoch
	channelSyncLock.RUnlock()
	return ChannelRoutingPolicySnapshot{
		Policy:                BuildChannelRoutingPolicy(candidates),
		Source:                ChannelRoutingSourceDatabase,
		Generation:            snapshotEpoch,
		DataGeneration:        uint64(snapshotEpoch),
		PublishedGeneration:   uint64(localPublishedEpoch),
		ClusterCommittedEpoch: snapshotEpoch,
		LocalPublishedEpoch:   localPublishedEpoch,
		CacheEnabled:          false,
		CachePending:          false,
	}, nil
}

type channelRoutingCandidateRow struct {
	AbilityModel    string `gorm:"column:ability_model"`
	ChannelID       int    `gorm:"column:channel_id"`
	ChannelName     string `gorm:"column:channel_name"`
	ChannelType     int    `gorm:"column:channel_type"`
	ChannelSettings string `gorm:"column:channel_settings"`
	Priority        *int64 `gorm:"column:priority"`
	Weight          uint   `gorm:"column:weight"`
}

func loadChannelRoutingCandidates(group string, modelName string, requestPath string) ([]ChannelRoutingCandidate, int64, error) {
	if err := ensureChannelRoutingSchema(DB); err != nil {
		return nil, 0, err
	}
	for attempt := 0; attempt < 16; attempt++ {
		beforeEpoch, err := GetCommittedChannelRoutingEpoch(DB)
		if err != nil {
			return nil, 0, err
		}
		candidates, err := queryChannelRoutingCandidates(group, modelName, requestPath)
		if err != nil {
			return nil, 0, err
		}
		afterEpoch, err := GetCommittedChannelRoutingEpoch(DB)
		if err != nil {
			return nil, 0, err
		}
		if beforeEpoch == afterEpoch {
			return candidates, afterEpoch, nil
		}
	}
	return nil, 0, errors.New("channel routing policy changed too frequently")
}

func queryChannelRoutingCandidates(group string, modelName string, requestPath string) ([]ChannelRoutingCandidate, error) {
	normalizedModel := ratio_setting.FormatMatchingModelName(modelName)
	abilityModels := []string{modelName}
	if normalizedModel != "" && normalizedModel != modelName {
		abilityModels = append(abilityModels, normalizedModel)
	}

	rows := make([]channelRoutingCandidateRow, 0)
	groupColumn := "abilities.`group`"
	if DB.Dialector.Name() == "postgres" {
		groupColumn = `abilities."group"`
	}
	selectColumns := "abilities.model AS ability_model, " +
		"abilities.channel_id AS channel_id, " +
		"abilities.priority AS priority, " +
		"abilities.weight AS weight, " +
		"channels.name AS channel_name, " +
		"channels.type AS channel_type, " +
		"channels.settings AS channel_settings"
	if err := DB.Table("abilities").
		Select(selectColumns).
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where(groupColumn+" = ? AND abilities.model IN ? AND abilities.enabled = ? AND channels.status = ?", group, abilityModels, true, common.ChannelStatusEnabled).
		Order("abilities.priority DESC").
		Order("abilities.channel_id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	exactCandidates := make([]ChannelRoutingCandidate, 0, len(rows))
	normalizedCandidates := make([]ChannelRoutingCandidate, 0, len(rows))
	exactSeen := make(map[int]struct{}, len(rows))
	normalizedSeen := make(map[int]struct{}, len(rows))
	for _, row := range rows {
		if row.ChannelType == constant.ChannelTypeAdvancedCustom {
			config, parseErr := parseChannelAdvancedCustomRoutingConfig(&Channel{
				Id:            row.ChannelID,
				Type:          row.ChannelType,
				OtherSettings: row.ChannelSettings,
			})
			if parseErr != nil || config == nil {
				continue
			}
			if requestPath != "" && !config.SupportsPathForModel(requestPath, modelName) {
				continue
			}
		}
		priority := int64(0)
		if row.Priority != nil {
			priority = *row.Priority
		}
		candidate := ChannelRoutingCandidate{
			ChannelID:   row.ChannelID,
			ChannelName: row.ChannelName,
			ChannelType: row.ChannelType,
			Priority:    priority,
			Weight:      row.Weight,
		}
		if row.AbilityModel == modelName {
			if _, duplicate := exactSeen[row.ChannelID]; duplicate {
				continue
			}
			exactSeen[row.ChannelID] = struct{}{}
			exactCandidates = append(exactCandidates, candidate)
			continue
		}
		if _, duplicate := normalizedSeen[row.ChannelID]; duplicate {
			continue
		}
		normalizedSeen[row.ChannelID] = struct{}{}
		normalizedCandidates = append(normalizedCandidates, candidate)
	}
	if len(exactCandidates) > 0 {
		return exactCandidates, nil
	}
	return normalizedCandidates, nil
}

func GetChannel(group string, modelName string, retry int, requestPath string) (*Channel, error) {
	policy, err := GetChannelRoutingPolicy(group, modelName, requestPath)
	if err != nil {
		return nil, err
	}
	channelID, found, err := SelectChannelFromRoutingPolicy(policy, retry)
	if err != nil || !found {
		return nil, err
	}
	channel := &Channel{}
	if err := DB.First(channel, "id = ?", channelID).Error; err != nil {
		return nil, err
	}
	return channel, nil
}

func (channel *Channel) buildAbilities() []Ability {
	models := strings.Split(channel.Models, ",")
	groups := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models))
	for _, modelName := range models {
		for _, group := range groups {
			key := group + "|" + modelName
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			abilities = append(abilities, Ability{
				Group:     group,
				Model:     modelName,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			})
		}
	}
	return abilities
}

func (channel *Channel) addAbilitiesWithDB(db *gorm.DB) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	abilities := channel.buildAbilities()
	if len(abilities) == 0 {
		return false, nil
	}
	for _, chunk := range lo.Chunk(abilities, 50) {
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error; err != nil {
			return false, err
		}
	}
	return true, nil
}

func (channel *Channel) AddAbilities(tx *gorm.DB) error {
	if tx != nil {
		changed, err := channel.addAbilitiesWithDB(tx)
		if err != nil || !changed || isChannelRoutingManagedTransaction(tx) {
			return err
		}
		if err := ensureChannelRoutingSchema(DB); err != nil {
			return err
		}
		_, err = advanceChannelRoutingEpoch(tx)
		return err
	}
	_, _, err := runChannelRoutingTransaction(context.Background(), func(tx *gorm.DB) (bool, error) {
		return channel.addAbilitiesWithDB(tx)
	})
	return err
}

func (channel *Channel) DeleteAbilities() error {
	_, _, err := runChannelRoutingTransaction(context.Background(), func(tx *gorm.DB) (bool, error) {
		result := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{})
		return result.RowsAffected > 0, result.Error
	})
	return err
}

func (channel *Channel) replaceAbilitiesWithDB(tx *gorm.DB) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	if err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error; err != nil {
		return err
	}
	_, err := channel.addAbilitiesWithDB(tx)
	return err
}

// UpdateAbilities updates abilities of this channel. A caller-owned transaction
// is responsible for advancing the routing epoch; standalone updates do so in
// the same transaction as the ability replacement.
func (channel *Channel) UpdateAbilities(tx *gorm.DB) error {
	if tx != nil {
		if err := channel.replaceAbilitiesWithDB(tx); err != nil {
			return err
		}
		if isChannelRoutingManagedTransaction(tx) {
			return nil
		}
		if err := ensureChannelRoutingSchema(DB); err != nil {
			return err
		}
		_, err := advanceChannelRoutingEpoch(tx)
		return err
	}
	_, _, err := runChannelRoutingTransaction(context.Background(), func(tx *gorm.DB) (bool, error) {
		return true, channel.replaceAbilitiesWithDB(tx)
	})
	return err
}

func UpdateAbilityStatus(channelId int, status bool) error {
	_, _, err := runChannelRoutingTransaction(context.Background(), func(tx *gorm.DB) (bool, error) {
		result := tx.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status)
		return result.RowsAffected > 0, result.Error
	})
	return err
}

func updateAbilityStatusWithDB(db *gorm.DB, channelId int, status bool) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	return db.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityStatusByTag(tag string, status bool) error {
	_, _, err := runChannelRoutingTransaction(context.Background(), func(tx *gorm.DB) (bool, error) {
		result := tx.Model(&Ability{}).Where("tag = ?", tag).Select("enabled").Update("enabled", status)
		return result.RowsAffected > 0, result.Error
	})
	return err
}

func updateAbilityStatusByTagWithDB(db *gorm.DB, tag string, status bool) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	return db.Model(&Ability{}).Where("tag = ?", tag).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityByTag(tag string, newTag *string, priority *int64, weight *uint) error {
	_, _, err := runChannelRoutingTransaction(context.Background(), func(tx *gorm.DB) (bool, error) {
		updates := make(map[string]any)
		if newTag != nil {
			updates["tag"] = newTag
		}
		if priority != nil {
			updates["priority"] = priority
		}
		if weight != nil {
			updates["weight"] = *weight
		}
		if len(updates) == 0 {
			return false, nil
		}
		result := tx.Model(&Ability{}).Where("tag = ?", tag).Updates(updates)
		return result.RowsAffected > 0, result.Error
	})
	return err
}

func updateAbilityByTagWithDB(db *gorm.DB, tag string, newTag *string, priority *int64, weight *uint) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	updates := make(map[string]any)
	if newTag != nil {
		updates["tag"] = newTag
	}
	if priority != nil {
		updates["priority"] = priority
	}
	if weight != nil {
		updates["weight"] = *weight
	}
	if len(updates) == 0 {
		return nil
	}
	return db.Model(&Ability{}).Where("tag = ?", tag).Updates(updates).Error
}

var fixLock = sync.Mutex{}

func FixAbility() (int, int, error) {
	if !fixLock.TryLock() {
		return 0, 0, errors.New("已经有一个修复任务在运行中，请稍后再试")
	}
	defer fixLock.Unlock()

	var channels []*Channel
	if err := DB.Find(&channels).Error; err != nil {
		return 0, 0, err
	}
	_, changed, err := runChannelRoutingTransaction(context.Background(), func(tx *gorm.DB) (bool, error) {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Ability{}).Error; err != nil {
			return false, err
		}
		for _, channel := range channels {
			if _, err := channel.addAbilitiesWithDB(tx); err != nil {
				return false, err
			}
		}
		return true, nil
	})
	if err != nil {
		return 0, len(channels), err
	}
	if changed {
		if err := InitChannelCache(); err != nil {
			return len(channels), 0, &ChannelCacheRefreshError{Err: err}
		}
	}
	return len(channels), 0, nil
}
