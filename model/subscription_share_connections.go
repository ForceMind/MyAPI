package model

import (
	"context"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

// SubscriptionShareConnectionPool is saved allocation context, not a claim
// that a Key can currently route to this channel. Closed pools are retained.
type SubscriptionShareConnectionPool struct {
	PoolID       int64  `json:"pool_id" gorm:"column:id"`
	ChannelID    int    `json:"channel_id"`
	WindowSource string `json:"window_source"`
	WindowType   string `json:"window_type"`
	Enabled      bool   `json:"enabled"`
}

type SubscriptionShareConnectionPools struct {
	ChannelID int                               `json:"channel_id"`
	Items     []SubscriptionShareConnectionPool `json:"items"`
	HasMore   bool                              `json:"has_more"`
}

func ReadSubscriptionShareConnectionPools(ctx context.Context, db *gorm.DB, actorID, channelID, page int) (*SubscriptionShareConnectionPools, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if actorID <= 0 || channelID <= 0 || page < 1 || page > 1000000 {
		return nil, ErrSubscriptionShareInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var actor User
	if err := db.WithContext(ctx).Select("id", "role", "status").First(&actor, actorID).Error; err != nil {
		return nil, err
	}
	if actor.Role != common.RoleRootUser || actor.Status != common.UserStatusEnabled {
		return nil, ErrSubscriptionShareDenied
	}
	var channel Channel
	if err := db.WithContext(ctx).Select("id").First(&channel, channelID).Error; err != nil {
		return nil, err
	}
	view := &SubscriptionShareConnectionPools{ChannelID: channelID, Items: []SubscriptionShareConnectionPool{}}
	// The subquery excludes candidates without persisted allocations, including
	// candidates returned by ReadSubscriptionShares for unrelated Keys.
	allocations := db.Model(&SubscriptionShareAllocation{}).Select("pool_id")
	err := db.WithContext(ctx).Model(&SubscriptionSharePool{}).
		Select("id", "channel_id", "window_source", "window_type", "enabled").
		Where("channel_id = ? AND id IN (?)", channelID, allocations).
		Order("id DESC").Offset((page - 1) * 20).Limit(21).Scan(&view.Items).Error
	if err != nil {
		return nil, err
	}
	view.HasMore = len(view.Items) > 20
	if view.HasMore {
		view.Items = view.Items[:20]
	}
	return view, nil
}
