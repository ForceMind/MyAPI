package model

import (
	"context"
	"fmt"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChannelRoutingSession stores only a digest of the authenticated session
// identity. Version is the compare-and-swap token used when a failed upstream
// attempt moves a conversation to another channel.
type ChannelRoutingSession struct {
	KeyHash   string `json:"-" gorm:"type:varchar(64);primaryKey"`
	ChannelId int    `json:"channel_id" gorm:"not null;index"`
	Version   int64  `json:"version" gorm:"not null"`
	LeaseID   string `json:"-" gorm:"type:varchar(36);not null"`
	ExpiresAt int64  `json:"expires_at" gorm:"bigint;not null;index"`
	FailureAt int64  `json:"-" gorm:"bigint;not null"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;not null"`
	UpdatedAt int64  `json:"updated_at" gorm:"bigint;not null"`
}

func (ChannelRoutingSession) TableName() string {
	return "channel_routing_sessions"
}

// ClaimChannelRoutingSession atomically creates the first binding. An active
// winner is returned to all racing callers. Expired rows are replaced through
// a bounded version CAS, so no transaction remains open during an upstream
// request.
func ClaimChannelRoutingSession(ctx context.Context, keyHash string, channelID int, now, expiresAt int64) (ChannelRoutingSession, error) {
	if DB == nil {
		return ChannelRoutingSession{}, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if keyHash == "" || channelID <= 0 || expiresAt <= now {
		return ChannelRoutingSession{}, gorm.ErrInvalidData
	}
	db := DB.WithContext(ctx)
	created := ChannelRoutingSession{
		KeyHash: keyHash, ChannelId: channelID, Version: 1,
		LeaseID: common.GetUUID(), ExpiresAt: expiresAt, CreatedAt: now, UpdatedAt: now,
	}
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&created)
	if result.Error != nil {
		return ChannelRoutingSession{}, result.Error
	}
	for attempt := 0; attempt < 4; attempt++ {
		var current ChannelRoutingSession
		if err := db.Where("key_hash = ?", keyHash).First(&current).Error; err != nil {
			return ChannelRoutingSession{}, err
		}
		if current.ExpiresAt > now {
			return current, nil
		}
		updated := db.Model(&ChannelRoutingSession{}).
			Where("key_hash = ? AND lease_id = ? AND version = ? AND expires_at <= ?", keyHash, current.LeaseID, current.Version, now).
			Updates(map[string]interface{}{
				"channel_id": channelID,
				"lease_id":   common.GetUUID(),
				"version":    gorm.Expr("version + ?", 1),
				"expires_at": expiresAt,
				"failure_at": 0,
				"created_at": now,
				"updated_at": now,
			})
		if updated.Error != nil {
			return ChannelRoutingSession{}, updated.Error
		}
		if updated.RowsAffected == 1 {
			if err := db.Where("key_hash = ?", keyHash).First(&current).Error; err != nil {
				return ChannelRoutingSession{}, err
			}
			return current, nil
		}
	}
	return ChannelRoutingSession{}, fmt.Errorf("channel routing session changed concurrently")
}

// SwitchChannelRoutingSession changes an active binding only if the caller
// still owns the observed version. When another caller already won, its row is
// returned with switched=false.
func SwitchChannelRoutingSession(ctx context.Context, expected ChannelRoutingSession, channelID int, now, expiresAt int64) (current ChannelRoutingSession, switched bool, err error) {
	if DB == nil {
		return ChannelRoutingSession{}, false, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if expected.KeyHash == "" || channelID <= 0 || expiresAt <= now {
		return ChannelRoutingSession{}, false, gorm.ErrInvalidData
	}
	db := DB.WithContext(ctx)
	newLeaseID := common.GetUUID()
	result := db.Model(&ChannelRoutingSession{}).
		Where("key_hash = ? AND channel_id = ? AND lease_id = ? AND version = ? AND expires_at > ?", expected.KeyHash, expected.ChannelId, expected.LeaseID, expected.Version, now).
		Updates(map[string]interface{}{
			"channel_id": channelID,
			"lease_id":   newLeaseID,
			"version":    gorm.Expr("version + ?", 1),
			"expires_at": expiresAt,
			"failure_at": 0,
			"updated_at": now,
		})
	if result.Error != nil {
		return ChannelRoutingSession{}, false, result.Error
	}
	if result.RowsAffected == 1 {
		if err := db.Where("key_hash = ?", expected.KeyHash).First(&current).Error; err != nil {
			return ChannelRoutingSession{}, false, err
		}
		return current, current.ChannelId == channelID && current.LeaseID == newLeaseID, nil
	}
	if err := db.Where("key_hash = ?", expected.KeyHash).First(&current).Error; err != nil {
		return ChannelRoutingSession{}, false, err
	}
	return current, false, nil
}

func TouchChannelRoutingSession(ctx context.Context, binding ChannelRoutingSession, now, expiresAt int64) error {
	if DB == nil || binding.KeyHash == "" || expiresAt <= now {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	db := DB.WithContext(ctx)
	result := db.Model(&ChannelRoutingSession{}).
		Where("key_hash = ? AND channel_id = ? AND lease_id = ? AND version = ? AND expires_at > ?", binding.KeyHash, binding.ChannelId, binding.LeaseID, binding.Version, now).
		Updates(map[string]interface{}{
			"lease_id": common.GetUUID(), "version": gorm.Expr("version + ?", 1),
			"expires_at": expiresAt, "updated_at": now,
		})
	if result.Error != nil || result.RowsAffected == 1 {
		return result.Error
	}
	// A concurrent failed request can expire the old generation first. A later
	// successful request may restore its channel only while the authoritative
	// row is still expired; Claim leaves any newer active winner untouched.
	var current ChannelRoutingSession
	if err := db.Where("key_hash = ?", binding.KeyHash).First(&current).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	if current.ExpiresAt > now || current.FailureAt <= 0 {
		return nil
	}
	_, err := ClaimChannelRoutingSession(ctx, binding.KeyHash, binding.ChannelId, now, expiresAt)
	return err
}

func ExpireChannelRoutingSessionIfCurrent(ctx context.Context, binding ChannelRoutingSession, now int64) error {
	if DB == nil || binding.KeyHash == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return DB.WithContext(ctx).Model(&ChannelRoutingSession{}).
		Where("key_hash = ? AND channel_id = ? AND lease_id = ? AND version = ?", binding.KeyHash, binding.ChannelId, binding.LeaseID, binding.Version).
		Updates(map[string]interface{}{
			"lease_id": common.GetUUID(), "version": gorm.Expr("version + ?", 1),
			"expires_at": now, "failure_at": now, "updated_at": now,
		}).Error
}

func DeleteChannelRoutingSessionIfCurrent(ctx context.Context, binding ChannelRoutingSession) error {
	if DB == nil || binding.KeyHash == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return DB.WithContext(ctx).
		Where("key_hash = ? AND channel_id = ? AND lease_id = ? AND version = ?", binding.KeyHash, binding.ChannelId, binding.LeaseID, binding.Version).
		Delete(&ChannelRoutingSession{}).Error
}

func DeleteExpiredChannelRoutingSessions(ctx context.Context, now int64, limit int) (int64, error) {
	if DB == nil {
		return 0, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var hashes []string
	if err := DB.WithContext(ctx).Model(&ChannelRoutingSession{}).
		Where("expires_at <= ?", now).Order("expires_at ASC").Limit(limit).Pluck("key_hash", &hashes).Error; err != nil {
		return 0, err
	}
	if len(hashes) == 0 {
		return 0, nil
	}
	result := DB.WithContext(ctx).Where("key_hash IN ? AND expires_at <= ?", hashes, now).Delete(&ChannelRoutingSession{})
	return result.RowsAffected, result.Error
}
