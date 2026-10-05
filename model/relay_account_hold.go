package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RelayAccountHold records transient transport health, never quota exhaustion.
// Identity and config digests are internal and must not be serialized to clients.
type RelayAccountHold struct {
	ChannelID    int    `json:"-" gorm:"primaryKey;autoIncrement:false"`
	Identity     string `json:"-" gorm:"primaryKey;type:varchar(64)"`
	ConfigDigest string `json:"-" gorm:"primaryKey;type:char(64)"`
	Until        int64  `json:"-" gorm:"column:hold_until;type:bigint;not null;index:idx_relay_account_hold_expiry"`
}

func (RelayAccountHold) String() string     { return "RelayAccountHold{Private:[REDACTED]}" }
func (h RelayAccountHold) GoString() string { return h.String() }

// RelayChannelConfigDigest excludes operational counters, but includes every
// dispatch-affecting channel setting. A config replacement retires old holds.
func RelayChannelConfigDigest(channel *Channel) string {
	if channel == nil {
		return ""
	}
	// Do not use legacy settings getters here: malformed JSON can cause those
	// getters to save a fallback channel, including while holding a transaction.
	var other dto.ChannelOtherSettings
	var setting dto.ChannelSettings
	var param, header map[string]any
	var mapping map[string]string
	if channel.OtherSettings != "" && common.UnmarshalJsonStr(channel.OtherSettings, &other) != nil {
		return ""
	}
	for _, field := range []struct {
		raw    *string
		target any
	}{{channel.Setting, &setting}, {channel.ParamOverride, &param}, {channel.HeaderOverride, &header}, {channel.ModelMapping, &mapping}} {
		if field.raw != nil && *field.raw != "" && common.UnmarshalJsonStr(*field.raw, field.target) != nil {
			return ""
		}
	}
	data, err := common.Marshal([]any{other.ModelRoutes, other.AllowServiceTier, other.DisableStore, other.AllowSafetyIdentifier, other.AllowIncludeObfuscation, other.AdvancedCustom, other.TokenHub, channel.Type, channel.GetBaseURL(), channel.OpenAIOrganization, setting, param, header, channel.GetModels(), channel.GetGroups(), mapping})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// RecordRelayAccountHold serializes with channel edits and cannot shorten a
// newer hold. The selected raw credential is checked, never stored in this row.
func RecordRelayAccountHold(ctx context.Context, db *gorm.DB, selected *Channel, key, identity string, seconds int) (bool, error) {
	if db == nil || selected == nil || key == "" || identity == "" || seconds < 1 || seconds > 300 {
		return false, errors.New("invalid relay hold")
	}
	digest := RelayChannelConfigDigest(selected)
	if digest == "" {
		return false, errors.New("invalid relay configuration")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	recorded := false
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		recorded = false
		err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var current Channel
			if err := lockForUpdate(tx).First(&current, selected.Id).Error; err != nil {
				return err
			}
			if current.Status != common.ChannelStatusEnabled || RelayChannelConfigDigest(&current) != digest {
				return nil
			}
			keys := []string{current.Key}
			if current.ChannelInfo.IsMultiKey {
				keys = current.GetKeys()
			}
			found := false
			for index, candidate := range keys {
				status, has := current.ChannelInfo.MultiKeyStatusList[index]
				if candidate == key && (!current.ChannelInfo.IsMultiKey || !has || status == common.ChannelStatusEnabled) {
					found = true
					break
				}
			}
			if !found {
				return nil
			}
			now, err := ReadDatabaseUnixTime(ctx, tx)
			if err != nil {
				return err
			}
			hold := RelayAccountHold{ChannelID: selected.Id, Identity: identity, ConfigDigest: digest, Until: now + int64(seconds)}
			// OnConflict uses the dialect's native conflict syntax; the assignment itself
			// is portable SQL and uses bound values rather than engine-specific aliases.
			err = tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "channel_id"}, {Name: "identity"}, {Name: "config_digest"}}, DoUpdates: clause.Assignments(map[string]any{"hold_until": gorm.Expr("CASE WHEN ? < ? THEN ? ELSE ? END", clause.Column{Table: clause.CurrentTable, Name: "hold_until"}, hold.Until, hold.Until, clause.Column{Table: clause.CurrentTable, Name: "hold_until"})})}).Create(&hold).Error
			recorded = err == nil
			return err
		})
		if err == nil || !channelQuotaIdentityRetryable(db, err) || attempt == 3 {
			return recorded, err
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 5 * time.Millisecond):
		}
	}
	return recorded, err
}

func ReadRelayAccountHolds(ctx context.Context, db *gorm.DB, channel *Channel) ([]RelayAccountHold, error) {
	var holds []RelayAccountHold
	digest := RelayChannelConfigDigest(channel)
	if digest == "" {
		return nil, errors.New("invalid relay configuration")
	}
	now, err := ReadDatabaseUnixTime(ctx, db)
	if err != nil {
		return nil, err
	}
	err = db.WithContext(ctx).Where("channel_id = ? AND config_digest = ? AND hold_until > ?", channel.Id, digest, now).Find(&holds).Error
	return holds, err
}

// CleanupRelayAccountHolds removes at most limit expired rows. Reads never
// extend a hold; restart uses the persisted expiry and requires no recovery write.
func CleanupRelayAccountHolds(ctx context.Context, db *gorm.DB, limit int) (int64, error) {
	if limit < 1 || limit > 1000 {
		return 0, errors.New("invalid relay hold cleanup limit")
	}
	now, err := ReadDatabaseUnixTime(ctx, db)
	if err != nil {
		return 0, err
	}
	var rows []RelayAccountHold
	if err := db.WithContext(ctx).Where("hold_until <= ?", now).Order("hold_until").Limit(limit).Find(&rows).Error; err != nil {
		return 0, err
	}
	var removed int64
	for _, row := range rows {
		result := db.WithContext(ctx).Where("channel_id = ? AND identity = ? AND config_digest = ? AND hold_until <= ?", row.ChannelID, row.Identity, row.ConfigDigest, now).Delete(&RelayAccountHold{})
		if result.Error != nil {
			return removed, result.Error
		}
		removed += result.RowsAffected
	}
	return removed, nil
}
