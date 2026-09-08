package model

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupChannelRoutingSessionTestDB(t *testing.T) {
	t.Helper()
	previous := DB
	db, err := gorm.Open(sqlite.Open("file:channel-routing-session?mode=memory&cache=shared&_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	require.NoError(t, db.Migrator().DropTable(&ChannelRoutingSession{}))
	require.NoError(t, db.AutoMigrate(&ChannelRoutingSession{}))
	DB = db
	t.Cleanup(func() {
		DB = previous
		require.NoError(t, sqlDB.Close())
	})
}

func TestChannelRoutingSessionClaimAndCAS(t *testing.T) {
	setupChannelRoutingSessionTestDB(t)
	ctx := context.Background()
	first, err := ClaimChannelRoutingSession(ctx, "digest", 11, 100, 200)
	require.NoError(t, err)
	assert.Equal(t, 11, first.ChannelId)

	winner, err := ClaimChannelRoutingSession(ctx, "digest", 12, 100, 200)
	require.NoError(t, err)
	assert.Equal(t, first, winner)

	switched, won, err := SwitchChannelRoutingSession(ctx, first, 12, 110, 210)
	require.NoError(t, err)
	require.True(t, won)
	assert.Equal(t, 12, switched.ChannelId)
	assert.Equal(t, int64(2), switched.Version)

	current, won, err := SwitchChannelRoutingSession(ctx, first, 13, 110, 210)
	require.NoError(t, err)
	assert.False(t, won)
	assert.Equal(t, switched, current)
}

func TestChannelRoutingSessionExpiredClaimAndCleanup(t *testing.T) {
	setupChannelRoutingSessionTestDB(t)
	ctx := context.Background()
	first, err := ClaimChannelRoutingSession(ctx, "expired", 1, 100, 110)
	require.NoError(t, err)
	replacement, err := ClaimChannelRoutingSession(ctx, "expired", 2, 111, 211)
	require.NoError(t, err)
	assert.Equal(t, 2, replacement.ChannelId)
	assert.Equal(t, first.Version+1, replacement.Version)

	_, err = ClaimChannelRoutingSession(ctx, "delete", 3, 100, 105)
	require.NoError(t, err)
	deleted, err := DeleteExpiredChannelRoutingSessions(ctx, 106, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
}

func TestChannelRoutingSessionCompletionOrdering(t *testing.T) {
	setupChannelRoutingSessionTestDB(t)
	ctx := context.Background()

	binding, err := ClaimChannelRoutingSession(ctx, "success-first", 1, 100, 200)
	require.NoError(t, err)
	require.NoError(t, TouchChannelRoutingSession(ctx, binding, 110, 210))
	require.NoError(t, ExpireChannelRoutingSessionIfCurrent(ctx, binding, 111))
	var successWinner ChannelRoutingSession
	require.NoError(t, DB.First(&successWinner, "key_hash = ?", binding.KeyHash).Error)
	assert.Greater(t, successWinner.ExpiresAt, int64(111))
	assert.Zero(t, successWinner.FailureAt)

	binding, err = ClaimChannelRoutingSession(ctx, "failure-first", 2, 100, 200)
	require.NoError(t, err)
	require.NoError(t, ExpireChannelRoutingSessionIfCurrent(ctx, binding, 110))
	require.NoError(t, TouchChannelRoutingSession(ctx, binding, 111, 211))
	var restored ChannelRoutingSession
	require.NoError(t, DB.First(&restored, "key_hash = ?", binding.KeyHash).Error)
	assert.Equal(t, 2, restored.ChannelId)
	assert.Equal(t, int64(211), restored.ExpiresAt)
	assert.Zero(t, restored.FailureAt)

	naturallyExpired, err := ClaimChannelRoutingSession(ctx, "natural-expiry", 3, 100, 105)
	require.NoError(t, err)
	require.NoError(t, TouchChannelRoutingSession(ctx, naturallyExpired, 106, 206))
	var unchanged ChannelRoutingSession
	require.NoError(t, DB.First(&unchanged, "key_hash = ?", naturallyExpired.KeyHash).Error)
	assert.Equal(t, int64(105), unchanged.ExpiresAt)
	assert.Zero(t, unchanged.FailureAt)
}
