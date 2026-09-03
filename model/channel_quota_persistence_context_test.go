package model

import (
	"context"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestQuotaPersistenceContextBoundsExhaustedConnectionPool(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSnapshot{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	heldConnection, err := sqlDB.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = heldConnection.Close(); _ = sqlDB.Close() })
	for _, test := range []struct {
		name  string
		write func(context.Context) error
	}{
		{"snapshot", func(ctx context.Context) error {
			return RecordChannelQuotaSnapshotWithContext(ctx, &ChannelQuotaSnapshot{ChannelId: 9, Available: 10})
		}},
		{"balance", func(ctx context.Context) error { return (&Channel{Id: 9}).UpdateBalanceWithContext(ctx, 10) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			started := time.Now()
			require.ErrorIs(t, test.write(ctx), context.DeadlineExceeded)
			require.Less(t, time.Since(started), time.Second)
		})
	}
}

func TestQuotaCacheCredentialUpdateIsBoundedAndPreservesChannelSettings(t *testing.T) {
	previousEnabled, previousChannels := common.MemoryCacheEnabled, channelsIDM
	common.MemoryCacheEnabled = true
	original := &Channel{Id: 9, Name: "keep configured name", Key: "fixture-old", Models: "keep-model"}
	channelsIDM = map[int]*Channel{9: original}
	t.Cleanup(func() { common.MemoryCacheEnabled = previousEnabled; channelsIDM = previousChannels })
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	channelSyncLock.Lock()
	err := CacheUpdateChannelKeyWithContext(ctx, 9, "fixture-new")
	channelSyncLock.Unlock()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, "fixture-old", channelsIDM[9].Key)
	require.NoError(t, CacheUpdateChannelKeyWithContext(context.Background(), 9, "fixture-new"))
	require.Equal(t, "fixture-new", channelsIDM[9].Key)
	require.Equal(t, original.Name, channelsIDM[9].Name)
	require.Equal(t, original.Models, channelsIDM[9].Models)
	require.Equal(t, "fixture-old", original.Key, "readers holding the prior cache entry must not be mutated")
}
