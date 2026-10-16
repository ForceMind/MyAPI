package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSubscriptionShareTokenLifecycleFixture(t *testing.T) (*SubscriptionSharePool, User, []Token) {
	t.Helper()
	db := openAccountQuotaTestDB(t)
	useQuotaProjectionModelDB(t, db)
	require.NoError(t, db.AutoMigrate(
		&SubscriptionSharePool{},
		&SubscriptionShareAllocation{},
		&SubscriptionShareAllocationUsage{},
		&SubscriptionShareSampleApplication{},
	))
	owner := User{Username: "share-delete-owner", AffCode: "share-delete-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&owner).Error)
	tokens := []Token{
		{UserId: owner.Id, Key: "share-delete-single", Name: "single", Status: common.TokenStatusEnabled, ExpiredTime: -1},
		{UserId: owner.Id, Key: "share-delete-batch-zero", Name: "batch-zero", Status: common.TokenStatusEnabled, ExpiredTime: -1},
		{UserId: owner.Id, Key: "share-delete-batch-nonzero", Name: "batch-nonzero", Status: common.TokenStatusEnabled, ExpiredTime: -1},
	}
	require.NoError(t, db.Create(&tokens).Error)
	pool := &SubscriptionSharePool{
		DescriptorKey: "share-delete-descriptor", SubjectRef: "sub_012345678901234567890123456789012345678",
		SnapshotID: 1, ChannelID: 1, ChannelName: "private", WindowSource: codexQuotaWindowPrimarySource,
		WindowType: "weekly", WindowSeconds: 604800, PlanType: "plus", Revision: 1, CurrentGeneration: 1,
		SelectedObservedAt: 100, ConfiguredAt: 100, MaxAgeSeconds: SubscriptionShareDefaultMaxAge, Enabled: true, Approximate: true,
	}
	require.NoError(t, db.Create(pool).Error)
	return pool, owner, tokens
}

func TestSubscriptionShareTokenDeletionRequiresZeroShareAndPreservesHistory(t *testing.T) {
	pool, owner, tokens := newSubscriptionShareTokenLifecycleFixture(t)
	allocation := SubscriptionShareAllocation{PoolID: pool.ID, TokenID: tokens[0].Id, UserID: owner.Id, ShareBPS: 4000, CreatedAt: 100, UpdatedAt: 100}
	require.NoError(t, DB.Create(&allocation).Error)
	usage := SubscriptionShareAllocationUsage{AllocationID: allocation.ID, Generation: 1, EstimatedUsedMicro: 12_345_678}
	require.NoError(t, DB.Create(&usage).Error)
	receipt := SubscriptionShareSampleApplication{PoolID: pool.ID, Generation: 1, SnapshotID: 9, FromSnapshotID: 8, BeforeUsedMicro: 10, AfterUsedMicro: 20, DeltaMicro: 10, AllocationJSON: "[]", Reason: "positive_delta", CreatedAt: 110}
	require.NoError(t, DB.Create(&receipt).Error)

	err := DeleteTokenById(tokens[0].Id, owner.Id)
	require.ErrorIs(t, err, ErrSubscriptionShareDeleteDenied)
	var stored Token
	require.NoError(t, DB.Unscoped().First(&stored, tokens[0].Id).Error)
	assert.False(t, stored.DeletedAt.Valid)
	var storedUsage SubscriptionShareAllocationUsage
	require.NoError(t, DB.First(&storedUsage, usage.ID).Error)
	assert.Equal(t, usage.EstimatedUsedMicro, storedUsage.EstimatedUsedMicro)
	var storedReceipt SubscriptionShareSampleApplication
	require.NoError(t, DB.First(&storedReceipt, receipt.ID).Error)
	assert.Equal(t, receipt, storedReceipt)

	require.NoError(t, DB.Model(&SubscriptionShareAllocation{}).Where("id = ?", allocation.ID).Update("share_bps", 0).Error)
	require.NoError(t, DeleteTokenById(tokens[0].Id, owner.Id))
	require.NoError(t, DB.Unscoped().First(&stored, tokens[0].Id).Error)
	assert.True(t, stored.DeletedAt.Valid)
	require.NoError(t, DB.First(&storedUsage, usage.ID).Error)
	assert.Equal(t, usage.EstimatedUsedMicro, storedUsage.EstimatedUsedMicro)
	require.NoError(t, DB.First(&storedReceipt, receipt.ID).Error)
	assert.Equal(t, receipt, storedReceipt)
}

func TestSubscriptionShareBatchTokenDeletionIsAtomic(t *testing.T) {
	pool, owner, tokens := newSubscriptionShareTokenLifecycleFixture(t)
	allocations := []SubscriptionShareAllocation{
		{PoolID: pool.ID, TokenID: tokens[1].Id, UserID: owner.Id, ShareBPS: 0, CreatedAt: 100, UpdatedAt: 100},
		{PoolID: pool.ID, TokenID: tokens[2].Id, UserID: owner.Id, ShareBPS: 1, CreatedAt: 100, UpdatedAt: 100},
	}
	require.NoError(t, DB.Create(&allocations).Error)

	deleted, err := BatchDeleteTokens([]int{tokens[1].Id, tokens[2].Id}, owner.Id)
	require.ErrorIs(t, err, ErrSubscriptionShareDeleteDenied)
	assert.Zero(t, deleted)
	for _, token := range tokens[1:] {
		var stored Token
		require.NoError(t, DB.Unscoped().First(&stored, token.Id).Error)
		assert.False(t, stored.DeletedAt.Valid)
	}
}
