package common

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
)

func TestRedisStartupConfiguration(t *testing.T) {
	previousClient, previousEnabled, previousFrequency := RDB, RedisEnabled, SyncFrequency
	t.Cleanup(func() {
		if RDB != nil && RDB != previousClient {
			_ = RDB.Close()
		}
		RDB, RedisEnabled, SyncFrequency = previousClient, previousEnabled, previousFrequency
	})
	RDB = nil

	t.Run("absent cache is explicit", func(t *testing.T) {
		t.Setenv("REDIS_CONN_STRING", "")
		require.NoError(t, InitRedisClient())
		require.False(t, RedisEnabled)
		require.Nil(t, RDB)
	})

	t.Run("invalid configuration returns without credentials", func(t *testing.T) {
		t.Setenv("REDIS_CONN_STRING", "unsupported://synthetic-private-password@private.invalid")
		err := InitRedisClient()
		require.ErrorContains(t, err, "Redis connection configuration")
		require.NotContains(t, err.Error(), "synthetic-private-password")
		require.NotContains(t, err.Error(), "private.invalid")
		require.Nil(t, RDB)
	})

	t.Run("failed authentication leaves no available cache", func(t *testing.T) {
		server := miniredis.RunT(t)
		server.RequireAuth("synthetic-required-password")
		t.Setenv("REDIS_CONN_STRING", "redis://:synthetic-wrong-password@"+server.Addr())
		err := InitRedisClient()
		require.ErrorContains(t, err, "before database initialization")
		require.NotContains(t, err.Error(), "synthetic-wrong-password")
		require.NotContains(t, err.Error(), server.Addr())
		require.False(t, RedisEnabled)
		require.Nil(t, RDB)
	})

	t.Run("verified cache becomes available", func(t *testing.T) {
		server := miniredis.RunT(t)
		t.Setenv("REDIS_CONN_STRING", "redis://"+server.Addr())
		require.NoError(t, InitRedisClient())
		require.True(t, RedisEnabled)
		require.NotNil(t, RDB)
	})

	t.Run("invalid reinitialization revokes the old client", func(t *testing.T) {
		server := miniredis.RunT(t)
		t.Setenv("REDIS_CONN_STRING", "redis://"+server.Addr())
		require.NoError(t, InitRedisClient())
		previousVerified := RDB
		require.NotNil(t, previousVerified)
		t.Setenv("REDIS_CONN_STRING", "unsupported://synthetic-private-password@private.invalid")
		err := InitRedisClient()
		require.Error(t, err)
		require.False(t, RedisEnabled)
		require.Nil(t, RDB)
		require.ErrorContains(t, previousVerified.Ping(context.Background()).Err(), "closed")
	})
}
