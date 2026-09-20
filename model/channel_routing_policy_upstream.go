package model

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// UpdateChannelRoutingState locks and reloads the latest channel, applies one
// mutation, and persists channel models/settings plus abilities atomically.
// routingChanged controls whether the persistent routing epoch advances.
func UpdateChannelRoutingState(
	ctx context.Context,
	channelID int,
	mutate func(channel *Channel) (routingChanged bool, err error),
) (*Channel, bool, error) {
	if channelID <= 0 {
		return nil, false, errors.New("channel id is required")
	}
	if mutate == nil {
		return nil, false, errors.New("channel routing mutation callback is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	pollingLock := GetChannelPollingLock(channelID)
	pollingLock.Lock()
	defer pollingLock.Unlock()

	var updated Channel
	_, routingChanged, err := runChannelRoutingTransaction(ctx, func(tx *gorm.DB) (bool, error) {
		if err := lockForUpdate(tx).First(&updated, "id = ?", channelID).Error; err != nil {
			return false, err
		}
		changed, err := mutate(&updated)
		if err != nil {
			return false, err
		}
		if err := tx.Model(&Channel{}).
			Where("id = ?", channelID).
			Select("models", "settings").
			Updates(Channel{Models: updated.Models, OtherSettings: updated.OtherSettings}).Error; err != nil {
			return false, err
		}
		if changed {
			if err := updated.UpdateAbilities(tx); err != nil {
				return false, err
			}
		}
		return changed, nil
	})
	if err != nil {
		return nil, false, err
	}
	return &updated, routingChanged, nil
}
