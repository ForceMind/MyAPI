package common

import (
	"math"
	"sync"
	"time"
)

type InMemoryRateLimiter struct {
	store          map[string]*[]int64
	expiresAt      map[string]int64
	reserved       map[string]map[uint64]int64
	reservationSeq uint64
	mutex          sync.Mutex
	cleanupStarted bool
}

// RateLimitReservation 是 Reserve 返回的在途预留句柄。零值表示未预留，
// Commit/Release 对零值是无害空操作。
type RateLimitReservation struct {
	key string
	id  uint64
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	l.mutex.Lock()
	if l.store == nil {
		l.store = make(map[string]*[]int64)
		l.expiresAt = make(map[string]int64)
		l.reserved = make(map[string]map[uint64]int64)
	}
	if expirationDuration > 0 && !l.cleanupStarted {
		cleanupInterval := min(expirationDuration, time.Minute)
		l.cleanupStarted = true
		go l.clearExpiredItems(cleanupInterval)
	}
	l.mutex.Unlock()
}

func (l *InMemoryRateLimiter) clearExpiredItems(cleanupInterval time.Duration) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for now := range ticker.C {
		l.clearExpiredItemsAt(now.Unix())
	}
}

func (l *InMemoryRateLimiter) clearExpiredItemsAt(now int64) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	for key, expiresAt := range l.expiresAt {
		if expiresAt <= now {
			delete(l.store, key)
			delete(l.expiresAt, key)
			delete(l.reserved, key)
		}
	}
}

func rateLimitExpiry(now, duration int64) int64 {
	if duration <= 0 {
		return now
	}
	if duration > math.MaxInt64-now {
		return math.MaxInt64
	}
	return now + duration
}

// Request parameter duration's unit is seconds
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	if maxRequestNum <= 0 {
		return true
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	// [old <-- new]
	queue, ok := l.store[key]
	now := time.Now().Unix()
	if ok {
		*queue = pruneRateLimitQueue(*queue, now, duration)
		if len(*queue) < maxRequestNum {
			*queue = append(*queue, now)
			l.expiresAt[key] = rateLimitExpiry(now, duration)
			return true
		}
		l.expiresAt[key] = rateLimitExpiry((*queue)[len(*queue)-1], duration)
		return false
	} else {
		// Do not preallocate from the configured ceiling. A large administrative
		// limit must not turn the first request for every key into a huge allocation.
		s := make([]int64, 0, 1)
		l.store[key] = &s
		*(l.store[key]) = append(*(l.store[key]), now)
		l.expiresAt[key] = rateLimitExpiry(now, duration)
	}
	return true
}

// Allow reports whether a request would fit in the current window without
// recording it. This is used when the event is only countable after its
// outcome is known, such as successful upstream requests.
func (l *InMemoryRateLimiter) Allow(key string, maxRequestNum int, duration int64) bool {
	if maxRequestNum <= 0 {
		return true
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	queue, ok := l.store[key]
	if !ok || len(*queue) == 0 {
		return true
	}
	now := time.Now().Unix()
	*queue = pruneRateLimitQueue(*queue, now, duration)
	if len(*queue) == 0 {
		delete(l.store, key)
		delete(l.expiresAt, key)
		return true
	}
	l.expiresAt[key] = rateLimitExpiry((*queue)[len(*queue)-1], duration)
	if len(*queue) < maxRequestNum {
		return true
	}
	return false
}

func pruneRateLimitQueue(queue []int64, now, duration int64) []int64 {
	firstActive := 0
	for firstActive < len(queue) && now-queue[firstActive] >= duration {
		firstActive++
	}
	return queue[firstActive:]
}

// pruneReserved 删除窗口外的在途预留并返回剩余数量。结果未知的预留由此随
// 窗口过期自然回收，语义与 pruneRateLimitQueue 对已过账记录的裁剪一致。
func pruneReserved(reservations map[uint64]int64, now, duration int64) int {
	active := 0
	for id, at := range reservations {
		if now-at >= duration {
			delete(reservations, id)
		} else {
			active++
		}
	}
	return active
}

// Reserve 在窗口内原子占位：窗口内已提交记录数 + 在途预留数永不超过
// maxRequestNum。maxRequestNum<=0 时保持“不限制”语义，返回零值预留并放行。
// 调用方必须在请求结束后按结果结算：成功 Commit，失败 Release；结果未知
// （无法执行结算的路径）则不结算，预留随窗口过期自然回收。
func (l *InMemoryRateLimiter) Reserve(key string, maxRequestNum int, duration int64) (RateLimitReservation, bool) {
	if maxRequestNum <= 0 {
		return RateLimitReservation{}, true
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.store == nil {
		// 与 Request 一样假定 Init 已执行；此处兜底避免零值 limiter 直接 panic。
		l.store = make(map[string]*[]int64)
		l.expiresAt = make(map[string]int64)
		l.reserved = make(map[string]map[uint64]int64)
	}
	now := time.Now().Unix()
	active := 0
	if queue, ok := l.store[key]; ok {
		*queue = pruneRateLimitQueue(*queue, now, duration)
		active = len(*queue)
	}
	if reservations, ok := l.reserved[key]; ok {
		active += pruneReserved(reservations, now, duration)
		if len(reservations) == 0 {
			delete(l.reserved, key)
		}
	}
	if active >= maxRequestNum {
		l.expiresAt[key] = rateLimitExpiry(now, duration)
		return RateLimitReservation{}, false
	}
	l.reservationSeq++
	if l.reserved[key] == nil {
		l.reserved[key] = make(map[uint64]int64)
	}
	l.reserved[key][l.reservationSeq] = now
	l.expiresAt[key] = rateLimitExpiry(now, duration)
	return RateLimitReservation{key: key, id: l.reservationSeq}, true
}

// Commit 把在途预留转正为窗口内的已提交记录。预留已随窗口过期时，仅在窗口
// 仍有空位时补记，绝不突破 maxRequestNum 上限。零值预留为空操作。
func (l *InMemoryRateLimiter) Commit(reservation RateLimitReservation, maxRequestNum int, duration int64) {
	if reservation.id == 0 || maxRequestNum <= 0 {
		return
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	now := time.Now().Unix()
	if reservations, ok := l.reserved[reservation.key]; ok {
		if _, found := reservations[reservation.id]; found {
			delete(reservations, reservation.id)
			if len(reservations) == 0 {
				delete(l.reserved, reservation.key)
			}
			// 预留转正：占用数不变，必然有空位。
			l.appendLocked(reservation.key, now, duration)
			return
		}
	}
	// 预留已随窗口过期：仅在窗口仍有空位时补记，硬限额优先于计数完整。
	active := 0
	if queue, ok := l.store[reservation.key]; ok {
		*queue = pruneRateLimitQueue(*queue, now, duration)
		active = len(*queue)
	}
	if reservations, ok := l.reserved[reservation.key]; ok {
		active += pruneReserved(reservations, now, duration)
		if len(reservations) == 0 {
			delete(l.reserved, reservation.key)
		}
	}
	if active < maxRequestNum {
		l.appendLocked(reservation.key, now, duration)
	}
}

// Release 释放失败请求的在途预留，额度立即可被新请求使用。零值预留或预留
// 已不存在时为无害空操作。
func (l *InMemoryRateLimiter) Release(reservation RateLimitReservation) {
	if reservation.id == 0 {
		return
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	reservations, ok := l.reserved[reservation.key]
	if !ok {
		return
	}
	if _, found := reservations[reservation.id]; !found {
		return
	}
	delete(reservations, reservation.id)
	if len(reservations) == 0 {
		delete(l.reserved, reservation.key)
		// 既无在途预留又无已提交记录时，连带清理键的过期记账。
		if queue, qok := l.store[reservation.key]; !qok || len(*queue) == 0 {
			delete(l.store, reservation.key)
			delete(l.expiresAt, reservation.key)
		}
	}
}

// appendLocked 向窗口追加一条已提交记录并续期。调用方必须持有锁且已确认
// 窗口内有空位。
func (l *InMemoryRateLimiter) appendLocked(key string, now, duration int64) {
	queue, ok := l.store[key]
	if !ok {
		// 与 Request 一致：不按配置上限预分配。
		s := make([]int64, 0, 1)
		l.store[key] = &s
		queue = l.store[key]
	}
	*queue = append(*queue, now)
	l.expiresAt[key] = rateLimitExpiry(now, duration)
}
