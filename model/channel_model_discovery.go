package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"gorm.io/gorm"
)

// ChannelModelDiscoveryFreshnessSeconds is the display freshness window,
// matching the existing default 30-minute model-update check cadence. Expiry
// only marks evidence stale when read: it never fetches, deletes, enables or
// disables a model, changes routing, or rewrites persisted evidence.
const ChannelModelDiscoveryFreshnessSeconds int64 = 30 * 60

// ChannelModelDiscovery is server-owned evidence, never a routing permission.
// Both fingerprints are private digests; credentials and responses are not stored.
type ChannelModelDiscovery struct {
	ChannelID           int    `gorm:"primaryKey;autoIncrement:false"`
	ModelsJSON          string `gorm:"type:text"`
	Source              string `gorm:"type:varchar(32)"`
	Status              string `gorm:"type:varchar(32)"`
	EvidenceFingerprint string `json:"-" gorm:"type:char(64)"`
	AttemptFingerprint  string `json:"-" gorm:"type:char(64)"`
	Attempt             int64
	FetchedAt           int64
	CheckedAt           int64
}

type ChannelModelDiscoverySnapshot struct {
	Models    []string `json:"models"`
	Source    string   `json:"source"`
	Status    string   `json:"status"`
	FetchedAt int64    `json:"fetched_at"`
	CheckedAt int64    `json:"checked_at"`
	Stale     bool     `json:"stale"`
}

type ChannelModelDiscoveryAttempt struct {
	Channel     *Channel
	Sequence    int64
	Fingerprint string
}

func ChannelModelDiscoverySource(channel *Channel) string {
	switch channel.Type {
	case constant.ChannelTypeOpenAI:
		return "openai_models"
	case constant.ChannelTypeCodex:
		return "codex_models"
	default:
		return "manual"
	}
}

func channelModelDiscoveryFingerprint(channel *Channel) string {
	// Deliberately excludes enabled models, mappings and polling cursor: fetching
	// evidence must not invalidate itself or claim those fields grant permission.
	data, _ := common.Marshal(struct {
		Type           int
		Key            string
		BaseURL        *string
		Organization   *string
		Setting        *string
		HeaderOverride *string
		Other          string
		MultiKey       bool
		MultiKeyMode   constant.MultiKeyMode
		KeyStatus      map[int]int
	}{channel.Type, channel.Key, channel.BaseURL, channel.OpenAIOrganization,
		channel.Setting, channel.HeaderOverride, channel.Other, channel.ChannelInfo.IsMultiKey, channel.ChannelInfo.MultiKeyMode,
		channel.ChannelInfo.MultiKeyStatusList})
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func channelModelDiscoverySnapshot(channel *Channel, row *ChannelModelDiscovery) (ChannelModelDiscoverySnapshot, error) {
	view := ChannelModelDiscoverySnapshot{Models: []string{}, Source: ChannelModelDiscoverySource(channel), Status: "never_checked", Stale: true}
	if row == nil {
		if view.Source == "manual" {
			view.Status = "manual_unverified"
		}
		return view, nil
	}
	view.Source, view.Status = row.Source, row.Status
	view.FetchedAt, view.CheckedAt = row.FetchedAt, row.CheckedAt
	if row.ModelsJSON != "" {
		if err := common.UnmarshalJsonStr(row.ModelsJSON, &view.Models); err != nil {
			return view, err
		}
		if view.Models == nil {
			view.Models = []string{}
		}
	}
	fingerprint := channelModelDiscoveryFingerprint(channel)
	view.Stale = row.FetchedAt == 0 || row.FetchedAt <= time.Now().Unix()-ChannelModelDiscoveryFreshnessSeconds ||
		row.EvidenceFingerprint != fingerprint || (row.Status != "success" && row.Status != "empty")
	if row.AttemptFingerprint != fingerprint {
		view.Status = "configuration_changed"
	}
	if ChannelModelDiscoverySource(channel) == "manual" {
		view.Status = "manual_unverified"
		view.Stale = true
	}
	return view, nil
}

func GetChannelModelDiscovery(ctx context.Context, channelID int) (ChannelModelDiscoverySnapshot, error) {
	var view ChannelModelDiscoverySnapshot
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
			return err
		}
		var row ChannelModelDiscovery
		err := tx.First(&row, "channel_id = ?", channelID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			view, err = channelModelDiscoverySnapshot(&channel, nil)
			return err
		}
		if err != nil {
			return err
		}
		view, err = channelModelDiscoverySnapshot(&channel, &row)
		return err
	})
	return view, err
}

// BeginChannelModelDiscovery assigns a durable sequence before network I/O.
// Later starts supersede earlier completions, even within the same second.
func BeginChannelModelDiscovery(ctx context.Context, channelID int) (ChannelModelDiscoveryAttempt, error) {
	var attempt ChannelModelDiscoveryAttempt
	pollingLock := GetChannelPollingLock(channelID)
	pollingLock.Lock()
	defer pollingLock.Unlock()
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
			return err
		}
		var row ChannelModelDiscovery
		err := tx.First(&row, "channel_id = ?", channelID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row.ChannelID = channelID
		if row.Attempt < 0 || row.Attempt == math.MaxInt64 {
			return errors.New("discovery attempt sequence exhausted")
		}
		row.Attempt++
		row.AttemptFingerprint = channelModelDiscoveryFingerprint(&channel)
		row.CheckedAt = time.Now().Unix()
		row.Status = "refreshing"
		if row.Source == "" {
			row.Source = ChannelModelDiscoverySource(&channel)
		}
		if ChannelModelDiscoverySource(&channel) == "manual" {
			row.Status = "manual_unverified"
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		attempt = ChannelModelDiscoveryAttempt{Channel: &channel, Sequence: row.Attempt, Fingerprint: row.AttemptFingerprint}
		return nil
	})
	return attempt, err
}

// CompleteChannelModelDiscovery rejects superseded/configuration-mismatched
// responses atomically. Failed fetches retain the last successful catalogue.
// Terminal attempts accept identical replay without writes and reject changes.
func CompleteChannelModelDiscovery(ctx context.Context, attempt ChannelModelDiscoveryAttempt, models []string, succeeded bool) (bool, error) {
	if attempt.Channel == nil || attempt.Channel.Id <= 0 || attempt.Sequence <= 0 || attempt.Fingerprint == "" {
		return false, errors.New("invalid discovery attempt")
	}
	channelID := attempt.Channel.Id
	pollingLock := GetChannelPollingLock(channelID)
	pollingLock.Lock()
	defer pollingLock.Unlock()
	applied := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
			return err
		}
		var row ChannelModelDiscovery
		if err := tx.First(&row, "channel_id = ?", channelID).Error; err != nil {
			return err
		}
		if row.Attempt != attempt.Sequence || row.AttemptFingerprint != attempt.Fingerprint || channelModelDiscoveryFingerprint(&channel) != attempt.Fingerprint {
			return nil
		}
		previous := row
		row.Status = "failed"
		if succeeded {
			unique := make(map[string]struct{}, len(models))
			ids := make([]string, 0, len(models))
			for _, id := range models {
				id = strings.TrimSpace(id)
				if id != "" {
					if _, exists := unique[id]; !exists {
						unique[id] = struct{}{}
						ids = append(ids, id)
					}
				}
			}
			sort.Strings(ids)
			data, err := common.Marshal(ids)
			if err != nil {
				return err
			}
			row.ModelsJSON, row.Source = string(data), ChannelModelDiscoverySource(&channel)
			row.EvidenceFingerprint = attempt.Fingerprint
			row.FetchedAt = time.Now().Unix()
			row.Status = "success"
			if len(ids) == 0 {
				row.Status = "empty"
			}
		}
		row.CheckedAt = time.Now().Unix()
		if previous.Status != "refreshing" {
			applied = previous.Status == row.Status && previous.ModelsJSON == row.ModelsJSON &&
				previous.EvidenceFingerprint == row.EvidenceFingerprint && previous.Source == row.Source
			return nil
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}
