package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSubscriptionShareConnectionPoolsPersistedIdentity(t *testing.T) {
	f := newSubscriptionShareFixtureWithPrefix(t, 2, 2, "associated_")
	for i := range f.channels {
		require.NoError(t, f.db.Model(&f.channels[i]).Update("name", "same-name").Error)
	}
	pool := SubscriptionSharePool{DescriptorKey: "first", SubjectRef: f.subject, ChannelID: f.channels[0].Id, ChannelName: "same-name", Enabled: true}
	candidate := SubscriptionSharePool{DescriptorKey: "candidate", SubjectRef: f.subject, ChannelID: f.channels[0].Id, Enabled: true}
	other := SubscriptionSharePool{DescriptorKey: "other", SubjectRef: f.subject, ChannelID: f.channels[1].Id, ChannelName: "same-name", Enabled: true}
	closed := SubscriptionSharePool{DescriptorKey: "closed", SubjectRef: f.subject, ChannelID: f.channels[0].Id, Enabled: false}
	for _, item := range []*SubscriptionSharePool{&pool, &candidate, &other, &closed} {
		require.NoError(t, f.db.Create(item).Error)
	}
	for _, item := range []SubscriptionShareAllocation{
		{PoolID: pool.ID, TokenID: f.tokens[0].Id, UserID: f.owner.Id, ShareBPS: 1000},
		{PoolID: pool.ID, TokenID: f.tokens[1].Id, UserID: f.owner.Id, ShareBPS: 0},
		{PoolID: other.ID, TokenID: f.tokens[0].Id, UserID: f.owner.Id, ShareBPS: 1000},
		{PoolID: closed.ID, TokenID: f.tokens[1].Id, UserID: f.owner.Id, ShareBPS: 0},
	} {
		require.NoError(t, f.db.Create(&item).Error)
	}
	// Token lifecycle does not erase historical associations or duplicate pools.
	require.NoError(t, f.db.Model(&f.tokens[0]).Update("expired_time", 1).Error)
	require.NoError(t, f.db.Delete(&f.tokens[1]).Error)
	view, err := ReadSubscriptionShareConnectionPools(context.Background(), f.db, f.root.Id, f.channels[0].Id, 1)
	require.NoError(t, err)
	require.Equal(t, f.channels[0].Id, view.ChannelID)
	require.Len(t, view.Items, 2)
	require.Equal(t, closed.ID, view.Items[0].PoolID)
	require.False(t, view.Items[0].Enabled)
	require.Equal(t, pool.ID, view.Items[1].PoolID)
	require.False(t, view.HasMore)
	empty, err := ReadSubscriptionShareConnectionPools(context.Background(), f.db, f.root.Id, f.channels[0].Id, 2)
	require.NoError(t, err)
	require.Empty(t, empty.Items)
	_, err = ReadSubscriptionShareConnectionPools(context.Background(), f.db, f.owner.Id, f.channels[0].Id, 1)
	require.ErrorIs(t, err, ErrSubscriptionShareDenied)
	require.NoError(t, f.db.Model(&f.root).Update("role", common.RoleAdminUser).Error)
	_, err = ReadSubscriptionShareConnectionPools(context.Background(), f.db, f.root.Id, f.channels[0].Id, 1)
	require.ErrorIs(t, err, ErrSubscriptionShareDenied)
	require.NoError(t, f.db.Model(&f.root).Updates(map[string]any{"role": common.RoleRootUser, "status": common.UserStatusDisabled}).Error)
	_, err = ReadSubscriptionShareConnectionPools(context.Background(), f.db, f.root.Id, f.channels[0].Id, 1)
	require.ErrorIs(t, err, ErrSubscriptionShareDenied)
}

func TestSubscriptionShareConnectionPoolsEmptyAndInvalid(t *testing.T) {
	f := newSubscriptionShareFixture(t, 0, 1)
	view, err := ReadSubscriptionShareConnectionPools(context.Background(), f.db, f.root.Id, f.channels[0].Id, 1)
	require.NoError(t, err)
	require.Empty(t, view.Items)
	require.False(t, view.HasMore)
	_, err = ReadSubscriptionShareConnectionPools(context.Background(), f.db, f.root.Id, f.channels[0].Id+1, 1)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = ReadSubscriptionShareConnectionPools(context.Background(), f.db, f.root.Id, f.channels[0].Id, 0)
	require.ErrorIs(t, err, ErrSubscriptionShareInvalid)
}

func TestSubscriptionShareConnectionPoolsPagination(t *testing.T) {
	f := newSubscriptionShareFixture(t, 1, 1)
	var ids []int64
	for index := 0; index < 21; index++ {
		pool := SubscriptionSharePool{DescriptorKey: fmt.Sprintf("page-%d", index), SubjectRef: f.subject, ChannelID: f.channels[0].Id, Enabled: true}
		require.NoError(t, f.db.Create(&pool).Error)
		ids = append(ids, pool.ID)
		require.NoError(t, f.db.Create(&SubscriptionShareAllocation{PoolID: pool.ID, TokenID: f.tokens[0].Id, UserID: f.owner.Id, ShareBPS: 0}).Error)
	}
	first, err := ReadSubscriptionShareConnectionPools(context.Background(), f.db, f.root.Id, f.channels[0].Id, 1)
	require.NoError(t, err)
	require.Len(t, first.Items, 20)
	require.True(t, first.HasMore)
	require.Equal(t, ids[20], first.Items[0].PoolID)
	second, err := ReadSubscriptionShareConnectionPools(context.Background(), f.db, f.root.Id, f.channels[0].Id, 2)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	require.False(t, second.HasMore)
	require.Equal(t, ids[0], second.Items[0].PoolID)
}
