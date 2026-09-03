package common

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryRateLimiterLargeLimitDoesNotPreallocateConfiguredCapacity(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	require.True(t, limiter.Request("large-limit", math.MaxInt32, 60))
	queue, ok := limiter.store["large-limit"]
	require.True(t, ok)
	require.Len(t, *queue, 1)
	assert.LessOrEqual(t, cap(*queue), 2)
}

func TestInMemoryRateLimiterAllowDoesNotRecordOutcome(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	assert.True(t, limiter.Allow("success", 1, 60))
	_, exists := limiter.store["success"]
	assert.False(t, exists)
	require.True(t, limiter.Request("success", 1, 60))
	assert.False(t, limiter.Allow("success", 1, 60))
}

func TestInMemoryRateLimiterCleanupUsesEachKeysOwnWindow(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)
	require.True(t, limiter.Request("short", 2, 60))
	require.True(t, limiter.Request("long", 2, 3600))

	now := time.Now().Unix()
	limiter.clearExpiredItemsAt(now + 120)
	_, shortExists := limiter.store["short"]
	_, longExists := limiter.store["long"]
	assert.False(t, shortExists)
	assert.True(t, longExists)
}

func TestInMemoryRateLimiterDurationIncreaseRefreshesExistingKeyExpiry(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)
	require.True(t, limiter.Request("dynamic", 1, 60))
	originalExpiry := limiter.expiresAt["dynamic"]
	assert.False(t, limiter.Allow("dynamic", 1, 3600))
	assert.Greater(t, limiter.expiresAt["dynamic"], originalExpiry)

	limiter.clearExpiredItemsAt(originalExpiry + 1)
	_, exists := limiter.store["dynamic"]
	assert.True(t, exists, "a longer active window must not be cleared at the old shorter expiry")
}

func TestInMemoryRateLimiterCountDecreaseStillHonorsActiveEntries(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)
	now := time.Now().Unix()
	queue := []int64{now - 120, now - 30, now - 20}
	limiter.store["dynamic-count"] = &queue
	limiter.expiresAt["dynamic-count"] = now + 40

	assert.False(t, limiter.Allow("dynamic-count", 1, 60))
	assert.False(t, limiter.Request("dynamic-count", 1, 60))
	require.Len(t, *limiter.store["dynamic-count"], 2)
}
