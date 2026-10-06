package common

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryRateLimiterReserveCommitReleaseCycle(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	reservationA, allowed := limiter.Reserve("success", 2, 60)
	require.True(t, allowed)
	reservationB, allowed := limiter.Reserve("success", 2, 60)
	require.True(t, allowed)

	// 在途预留同样占用额度：committed 0 + reserved 2 达到上限。
	_, allowed = limiter.Reserve("success", 2, 60)
	assert.False(t, allowed)

	limiter.Commit(reservationA, 2, 60)
	queue, ok := limiter.store["success"]
	require.True(t, ok)
	require.Len(t, *queue, 1)

	// 重复 commit 是空操作，不重复计数。
	limiter.Commit(reservationA, 2, 60)
	require.Len(t, *limiter.store["success"], 1)

	// 释放失败请求的预留后额度立即可用。
	limiter.Release(reservationB)
	assert.NotContains(t, limiter.reserved, "success")
	reservationC, allowed := limiter.Reserve("success", 2, 60)
	require.True(t, allowed)

	// 重复 release 是空操作。
	limiter.Release(reservationB)
	limiter.Release(reservationC)
	assert.NotContains(t, limiter.reserved, "success")
}

func TestInMemoryRateLimiterReserveIsAtomicUnderConcurrency(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)
	const (
		requestCount = 50
		maxSuccess   = 10
	)

	allowedCount := 0
	var allowedMutex sync.Mutex
	reservations := make(chan RateLimitReservation, requestCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(requestCount)
	for range requestCount {
		go func() {
			defer waitGroup.Done()
			reservation, allowed := limiter.Reserve("concurrent", maxSuccess, 60)
			if allowed {
				allowedMutex.Lock()
				allowedCount++
				allowedMutex.Unlock()
				reservations <- reservation
			}
		}()
	}
	waitGroup.Wait()
	close(reservations)
	require.Equal(t, maxSuccess, allowedCount, "并发预留数必须严格等于上限，不超卖")

	for reservation := range reservations {
		limiter.Commit(reservation, maxSuccess, 60)
	}
	require.Len(t, *limiter.store["concurrent"], maxSuccess)
	_, allowed := limiter.Reserve("concurrent", maxSuccess, 60)
	assert.False(t, allowed)
}

func TestInMemoryRateLimiterUnreleasedReservationExpiresWithWindow(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	// 结果未知的预留：不 commit 也不 release。
	reservation, allowed := limiter.Reserve("unknown", 1, 60)
	require.True(t, allowed)
	_, allowed = limiter.Reserve("unknown", 1, 60)
	assert.False(t, allowed)

	// 窗口过期后由窗口裁剪自然回收（直接操纵存储时间戳，无需真实等待）。
	now := time.Now().Unix()
	limiter.reserved["unknown"][reservation.id] = now - 120
	reservationAfter, allowed := limiter.Reserve("unknown", 1, 60)
	require.True(t, allowed)
	limiter.Release(reservationAfter)

	// 清理协程路径：整个键随过期记账被回收。
	reservation, allowed = limiter.Reserve("unknown-cleanup", 1, 60)
	require.True(t, allowed)
	limiter.clearExpiredItemsAt(now + 120)
	assert.NotContains(t, limiter.reserved, "unknown-cleanup")
	_, allowed = limiter.Reserve("unknown-cleanup", 1, 60)
	assert.True(t, allowed)
}

func TestInMemoryRateLimiterCommitWithoutReservationNeverExceedsLimit(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	// 窗口已被已提交记录占满时，过期预留的补记被丢弃，绝不突破上限。
	require.True(t, limiter.Request("overflow", 2, 60))
	require.True(t, limiter.Request("overflow", 2, 60))
	staleReservation := RateLimitReservation{key: "overflow", id: 999}
	limiter.Commit(staleReservation, 2, 60)
	assert.Len(t, *limiter.store["overflow"], 2)

	// 窗口有空位时允许补记（请求耗时超过窗口的极端情况）。
	limiter.Commit(RateLimitReservation{key: "fresh", id: 998}, 2, 60)
	assert.Len(t, *limiter.store["fresh"], 1)

	// 零值预留是无害空操作。
	limiter.Commit(RateLimitReservation{}, 2, 60)
	limiter.Release(RateLimitReservation{})
	_, exists := limiter.store[""]
	assert.False(t, exists)
}

func TestInMemoryRateLimiterReserveZeroLimitStaysUnlimited(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	for range 20 {
		reservation, allowed := limiter.Reserve("unlimited", 0, 60)
		require.True(t, allowed)
		assert.Equal(t, RateLimitReservation{}, reservation)
		limiter.Commit(reservation, 0, 60)
	}
	_, exists := limiter.store["unlimited"]
	assert.False(t, exists, "上限为 0 保持旧版语义：不限制且不记账")
}

func TestInMemoryRateLimiterReservationSharesWindowWithRequestAndAllow(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	// 预留转正后与既有 Request/Allow 共用同一窗口存储。
	reservation, allowed := limiter.Reserve("shared", 1, 60)
	require.True(t, allowed)
	limiter.Commit(reservation, 1, 60)
	assert.False(t, limiter.Allow("shared", 1, 60))
	assert.False(t, limiter.Request("shared", 1, 60))

	// 既有 Request 写入的记录也计入预留判定。
	require.True(t, limiter.Request("shared-request", 1, 60))
	_, allowed = limiter.Reserve("shared-request", 1, 60)
	assert.False(t, allowed)
}
