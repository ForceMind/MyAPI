package limiter

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func s2cLimiterRedisFixtureOptions(address string) (*redis.Options, error) {
	host, port, err := net.SplitHostPort(address)
	number, portErr := strconv.Atoi(port)
	if err != nil || !net.ParseIP(host).IsLoopback() || portErr != nil || number <= 0 || number > 65535 {
		return nil, errors.New("S2-C limiter fixture requires a literal loopback TCP address and valid port")
	}
	return &redis.Options{Addr: address, DB: 14, MaxRetries: -1}, nil
}

func newTestClient(t *testing.T, server *miniredis.Miniredis) *redis.Client {
	t.Helper()

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client
}

func TestNewUsesProvidedRedisClient(t *testing.T) {
	ctx := context.Background()
	serverA := miniredis.RunT(t)
	serverB := miniredis.RunT(t)
	clientA := newTestClient(t, serverA)
	clientB := newTestClient(t, serverB)

	limiterA := New(ctx, clientA)
	allowed, err := limiterA.Allow(ctx, "bucket", WithCapacity(1), WithRate(1), WithRequested(1))
	require.NoError(t, err)
	require.True(t, allowed)

	limiterB := New(ctx, clientB)
	allowed, err = limiterB.Allow(ctx, "bucket", WithCapacity(1), WithRate(1), WithRequested(1))
	require.NoError(t, err)
	require.True(t, allowed)

	require.True(t, serverA.Exists("bucket"))
	require.True(t, serverB.Exists("bucket"))
}

func TestAllowRecoversAfterRedisScriptFlush(t *testing.T) {
	ctx := context.Background()
	server := miniredis.RunT(t)
	client := newTestClient(t, server)
	limiter := New(ctx, client)

	allowed, err := limiter.Allow(ctx, "bucket", WithCapacity(2), WithRate(1), WithRequested(1))
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, client.ScriptFlush(ctx).Err())

	allowed, err = limiter.Allow(ctx, "bucket", WithCapacity(2), WithRate(1), WithRequested(1))
	require.NoError(t, err)
	require.True(t, allowed)
}

func TestAllowSetsAndRenewsBucketTTL(t *testing.T) {
	ctx := context.Background()
	server := miniredis.RunT(t)
	client := newTestClient(t, server)
	limiter := New(ctx, client)
	baseTime := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	server.SetTime(baseTime)

	allowed, err := limiter.Allow(ctx, "bucket", WithCapacity(4), WithRate(1), WithRequested(4))
	require.NoError(t, err)
	require.True(t, allowed)

	ttl, err := client.TTL(ctx, "bucket").Result()
	require.NoError(t, err)
	require.Equal(t, 4*time.Second, ttl)

	server.FastForward(3 * time.Second)
	server.SetTime(baseTime.Add(3 * time.Second))
	allowed, err = limiter.Allow(ctx, "bucket", WithCapacity(4), WithRate(1), WithRequested(1))
	require.NoError(t, err)
	require.True(t, allowed)

	ttl, err = client.TTL(ctx, "bucket").Result()
	require.NoError(t, err)
	require.Equal(t, 2*time.Second, ttl)

	server.FastForward(time.Second)
	require.True(t, server.Exists("bucket"))
	server.FastForward(time.Second)
	require.False(t, server.Exists("bucket"))

	allowed, err = limiter.Allow(ctx, "minimum-ttl", WithCapacity(1), WithRate(2), WithRequested(1))
	require.NoError(t, err)
	require.True(t, allowed)
	ttl, err = client.TTL(ctx, "minimum-ttl").Result()
	require.NoError(t, err)
	require.Equal(t, time.Second, ttl)

	allowed, err = limiter.Allow(ctx, "long-refill", WithCapacity(25*60*60+1), WithRate(1), WithRequested(25*60*60))
	require.NoError(t, err)
	require.True(t, allowed)
	ttl, err = client.TTL(ctx, "long-refill").Result()
	require.NoError(t, err)
	require.Equal(t, 25*time.Hour, ttl)

	server.FastForward(24 * time.Hour)
	require.True(t, server.Exists("long-refill"))
	server.FastForward(time.Hour)
	require.False(t, server.Exists("long-refill"))
}

func TestAllowRejectsInvalidConfigWithoutWritingBucket(t *testing.T) {
	ctx := context.Background()
	server := miniredis.RunT(t)
	client := newTestClient(t, server)
	limiter := New(ctx, client)

	for _, testCase := range []struct {
		name      string
		rate      int64
		capacity  int64
		requested int64
	}{
		{
			name:      "zero rate",
			rate:      0,
			capacity:  1,
			requested: 1,
		},
		{
			name:      "negative rate",
			rate:      -1,
			capacity:  1,
			requested: 1,
		},
		{
			name:      "negative requested",
			rate:      1,
			capacity:  1,
			requested: -1,
		},
		{
			name:      "unsafe capacity",
			rate:      1,
			capacity:  MaxExactInteger + 1,
			requested: 1,
		},
		{
			name:      "unsafe rate",
			rate:      MaxExactInteger + 1,
			capacity:  1,
			requested: 1,
		},
		{
			name:      "unsafe requested",
			rate:      1,
			capacity:  MaxExactInteger,
			requested: MaxExactInteger + 1,
		},
		{
			name:      "zero capacity",
			rate:      1,
			capacity:  0,
			requested: 1,
		},
		{
			name:      "requested exceeds capacity",
			rate:      1,
			capacity:  1,
			requested: 2,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			key := "bucket:" + testCase.name
			allowed, err := limiter.Allow(ctx, key,
				WithCapacity(testCase.capacity),
				WithRate(testCase.rate),
				WithRequested(testCase.requested),
			)
			require.Error(t, err)
			require.False(t, allowed)
			require.False(t, server.Exists(key))
		})
	}

	validKey := "bucket:existing"
	allowed, err := limiter.Allow(ctx, validKey, WithCapacity(2), WithRate(1), WithRequested(1))
	require.NoError(t, err)
	require.True(t, allowed)
	beforeState, err := client.HGetAll(ctx, validKey).Result()
	require.NoError(t, err)
	beforeTTL, err := client.PTTL(ctx, validKey).Result()
	require.NoError(t, err)

	allowed, err = limiter.Allow(ctx, validKey, WithCapacity(2), WithRate(1), WithRequested(3))
	require.Error(t, err)
	require.False(t, allowed)
	afterState, err := client.HGetAll(ctx, validKey).Result()
	require.NoError(t, err)
	afterTTL, err := client.PTTL(ctx, validKey).Result()
	require.NoError(t, err)
	require.Equal(t, beforeState, afterState)
	require.Equal(t, beforeTTL, afterTTL)

	allowed, err = New(ctx, nil).Allow(ctx, "bucket:nil-client", WithCapacity(1), WithRate(1), WithRequested(1))
	require.Error(t, err)
	require.False(t, allowed)
	require.False(t, server.Exists("bucket:nil-client"))
}

func TestAllowLuaValidationAndInvalidBucketStateDoNotWrite(t *testing.T) {
	ctx := context.Background()
	server := miniredis.RunT(t)
	client := newTestClient(t, server)
	limiter := New(ctx, client)

	for _, testCase := range []struct {
		name string
		args []interface{}
	}{
		{name: "zero rate", args: []interface{}{1, 0, 1}},
		{name: "negative requested", args: []interface{}{-1, 1, 1}},
		{name: "non-integer requested", args: []interface{}{"1.5", 1, 1}},
		{name: "requested exceeds capacity", args: []interface{}{2, 1, 1}},
		{name: "unsafe capacity", args: []interface{}{1, 1, MaxExactInteger + 1}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			key := "lua-invalid:" + testCase.name
			err := client.Eval(ctx, rateLimitScript, []string{key}, testCase.args...).Err()
			require.Error(t, err)
			require.False(t, server.Exists(key))
		})
	}

	for _, testCase := range []struct {
		name     string
		tokens   string
		lastTime string
	}{
		{name: "negative tokens", tokens: "-1", lastTime: "1"},
		{name: "too many tokens", tokens: "2", lastTime: "1"},
		{name: "invalid last time", tokens: "1", lastTime: "not-a-number"},
		{name: "future last time", tokens: "1", lastTime: "9007199254740991"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			key := "invalid-state:" + testCase.name
			server.HSet(key, "tokens", testCase.tokens)
			server.HSet(key, "last_time", testCase.lastTime)

			allowed, err := limiter.Allow(ctx, key, WithCapacity(1), WithRate(1), WithRequested(1))
			require.Error(t, err)
			require.False(t, allowed)
			require.Equal(t, testCase.tokens, server.HGet(key, "tokens"))
			require.Equal(t, testCase.lastTime, server.HGet(key, "last_time"))
		})
	}
}

func TestS2CLimiterRedisTargetSafety(t *testing.T) {
	for _, address := range []string{"", "localhost:6379", "192.0.2.1:6379", "/tmp/redis.sock", "127.0.0.1:0", "127.0.0.1:65536"} {
		t.Run(address, func(t *testing.T) {
			_, err := s2cLimiterRedisFixtureOptions(address)
			require.Error(t, err)
		})
	}
}

// Only disposable CI Redis 7 instances are allowed. Require the entire
// instance to be empty before any write, use database 14, never FLUSH or
// delete keys, and let the service/container lifecycle own cleanup. SCRIPT
// FLUSH below clears only the script cache, which is the recovery contract
// under test. PEXPIRETIME checks absolute expiration instants without sleeps.
func TestS2CLimiterConfiguredRedis(t *testing.T) {
	if os.Getenv("MYAPI_S2C_REDIS_TESTS") != "1" {
		t.Skip("disposable Redis tests require MYAPI_S2C_REDIS_TESTS=1")
	}
	options, err := s2cLimiterRedisFixtureOptions(os.Getenv("MYAPI_S2C_REDIS_ADDR"))
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx := context.Background()

	keyspace, err := client.Info(ctx, "keyspace").Result()
	require.NoError(t, err)
	for _, line := range strings.Split(keyspace, "\n") {
		require.False(t, strings.HasPrefix(strings.TrimSpace(line), "db"), "refusing a non-empty Redis instance")
	}
	count, err := client.DBSize(ctx).Result()
	require.NoError(t, err)
	require.Zero(t, count)

	limiter := New(ctx, client)
	flushKey := "myapi:s2c:limiter:script-flush"
	allowed, err := limiter.Allow(ctx, flushKey, WithCapacity(2), WithRate(1), WithRequested(1))
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, client.ScriptFlush(ctx).Err())
	allowed, err = limiter.Allow(ctx, flushKey, WithCapacity(2), WithRate(1), WithRequested(1))
	require.NoError(t, err)
	require.True(t, allowed)

	for _, testCase := range []struct {
		name      string
		capacity  int64
		rate      int64
		requested int64
		ttl       time.Duration
	}{
		{name: "short-ttl", capacity: 4, rate: 1, requested: 4, ttl: 4 * time.Second},
		{name: "long-ttl", capacity: 25*60*60 + 1, rate: 1, requested: 25 * 60 * 60, ttl: 25 * time.Hour},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			key := "myapi:s2c:limiter:" + testCase.name
			before, err := client.Time(ctx).Result()
			require.NoError(t, err)
			allowed, err := limiter.Allow(ctx, key,
				WithCapacity(testCase.capacity),
				WithRate(testCase.rate),
				WithRequested(testCase.requested),
			)
			require.NoError(t, err)
			require.True(t, allowed)
			expiresAt, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
			require.NoError(t, err)
			after, err := client.Time(ctx).Result()
			require.NoError(t, err)

			require.GreaterOrEqual(t, expiresAt, before.Add(testCase.ttl).UnixMilli())
			require.LessOrEqual(t, expiresAt, after.Add(testCase.ttl).UnixMilli())
		})
	}

	for _, testCase := range []struct {
		name      string
		capacity  int64
		rate      int64
		requested int64
	}{
		{name: "go-zero-rate", capacity: 1, rate: 0, requested: 1},
		{name: "go-negative-requested", capacity: 1, rate: 1, requested: -1},
		{name: "go-requested-exceeds-capacity", capacity: 1, rate: 1, requested: 2},
		{name: "go-unsafe-capacity", capacity: MaxExactInteger + 1, rate: 1, requested: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			key := "myapi:s2c:limiter:" + testCase.name
			allowed, err := limiter.Allow(ctx, key,
				WithCapacity(testCase.capacity),
				WithRate(testCase.rate),
				WithRequested(testCase.requested),
			)
			require.Error(t, err)
			require.False(t, allowed)
			exists, err := client.Exists(ctx, key).Result()
			require.NoError(t, err)
			require.Zero(t, exists)
		})
	}

	for _, testCase := range []struct {
		name string
		args []interface{}
	}{
		{name: "lua-invalid", args: []interface{}{1, 0, 1}},
		{name: "lua-non-integer", args: []interface{}{"1.5", 1, 1}},
		{name: "lua-requested-exceeds-capacity", args: []interface{}{2, 1, 1}},
		{name: "lua-unsafe", args: []interface{}{1, 1, MaxExactInteger + 1}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			key := "myapi:s2c:limiter:" + testCase.name
			err := client.Eval(ctx, rateLimitScript, []string{key}, testCase.args...).Err()
			require.Error(t, err)
			exists, err := client.Exists(ctx, key).Result()
			require.NoError(t, err)
			require.Zero(t, exists)
		})
	}
}
