package common

import (
	"math"
	"sync"
	"time"
)

type InMemoryRateLimiter struct {
	store          map[string]*[]int64
	expiresAt      map[string]int64
	mutex          sync.Mutex
	cleanupStarted bool
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	l.mutex.Lock()
	if l.store == nil {
		l.store = make(map[string]*[]int64)
		l.expiresAt = make(map[string]int64)
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
