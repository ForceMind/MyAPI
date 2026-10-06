package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func applyModelSuccessRateLimitConfig(t *testing.T, success int) {
	t.Helper()

	previousConfig := setting.GetModelRequestRateLimitConfig()
	require.NoError(t, setting.ApplyModelRequestRateLimitConfig(setting.ModelRequestRateLimitConfig{
		Enabled:         true,
		DurationMinutes: 1,
		Total:           0,
		Success:         success,
		Group:           map[string][2]int{},
	}))
	t.Cleanup(func() {
		require.NoError(t, setting.ApplyModelRequestRateLimitConfig(previousConfig))
	})
}

func useGinTestMode(t *testing.T) {
	t.Helper()

	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
}

func newModelRateLimitRouter(userID int, handler gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", userID)
		c.Set(string(constant.ContextKeyUserGroup), "default")
		c.Next()
	})
	router.Use(ModelRequestRateLimit())
	router.GET("/", handler)
	return router
}

// runBarrierModelRateLimitConcurrency 以确定性屏障并发压测模型限流中间件：
// 成功进入 handler 的请求先报告 entered 再阻塞在 proceed 上。先收满 admitted
// 个 entered，再收取恰好 total-admitted 个已完成响应（必然是被限流拒绝的），
// 最后 close(proceed) 放行 admitted 个请求完成。全程无 sleep。
func runBarrierModelRateLimitConcurrency(t *testing.T, userID, total, admitted int) (rejected []*httptest.ResponseRecorder, completed []*httptest.ResponseRecorder) {
	t.Helper()

	entered := make(chan struct{}, admitted)
	proceed := make(chan struct{})
	router := newModelRateLimitRouter(userID, func(c *gin.Context) {
		entered <- struct{}{}
		<-proceed
		c.Status(http.StatusNoContent)
	})
	recorders := make(chan *httptest.ResponseRecorder, total)

	var waitGroup sync.WaitGroup
	waitGroup.Add(total)
	for range total {
		go func() {
			defer waitGroup.Done()
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			recorders <- recorder
		}()
	}

	ginHandlerEntered := 0
	for ginHandlerEntered < admitted {
		select {
		case <-entered:
			ginHandlerEntered++
		case recorder := <-recorders:
			rejected = append(rejected, recorder)
		}
	}
	for len(rejected) < total-admitted {
		rejected = append(rejected, <-recorders)
	}
	close(proceed)
	for len(completed) < admitted {
		completed = append(completed, <-recorders)
	}
	waitGroup.Wait()
	return rejected, completed
}

// newBarrierRouterForFollowUp 与 runBarrierModelRateLimitConcurrency 使用同一
// 路由形态，供调用方在并发压测后继续发跟进请求。
func newBarrierRouterForFollowUp(userID int) *gin.Engine {
	return newModelRateLimitRouter(userID, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
}

func TestRedisModelSuccessReserveCommitReleaseCycle(t *testing.T) {
	_, redisClient := useRateLimitMiniRedis(t)
	ctx := context.Background()
	key := "rateLimit:MRRLS:cycle"
	now := time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC)

	tokenA := newModelSuccessReservationToken()
	tokenB := newModelSuccessReservationToken()
	reserved, err := reserveRedisModelSuccess(ctx, redisClient, key, tokenA, 2, 60, now)
	require.NoError(t, err)
	require.True(t, reserved)
	reserved, err = reserveRedisModelSuccess(ctx, redisClient, key, tokenB, 2, 60, now)
	require.NoError(t, err)
	require.True(t, reserved)

	// 在途预留同样占用额度：committed 0 + reserved 2 达到上限。
	reserved, err = reserveRedisModelSuccess(ctx, redisClient, key, newModelSuccessReservationToken(), 2, 60, now)
	require.NoError(t, err)
	assert.False(t, reserved)

	committed, err := commitRedisModelSuccess(ctx, redisClient, key, tokenA, 2, 60, now)
	require.NoError(t, err)
	require.True(t, committed)
	length, err := redisClient.LLen(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), length)
	recorded, err := redisClient.LIndex(ctx, key, 0).Result()
	require.NoError(t, err)
	_, err = time.Parse(modelRateLimitTimeFormat, recorded)
	require.NoError(t, err, "committed entries must keep the legacy timestamp format")

	// 重复 commit 是空操作，不重复计数。
	committed, err = commitRedisModelSuccess(ctx, redisClient, key, tokenA, 2, 60, now)
	require.NoError(t, err)
	assert.False(t, committed)
	length, err = redisClient.LLen(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), length)

	// 释放失败请求的预留后额度立即可用。
	require.NoError(t, releaseRedisModelSuccess(ctx, redisClient, key, tokenB, 60))
	reservationCount, err := redisClient.ZCard(ctx, key+modelSuccessReservationKeySuffix).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), reservationCount)
	reserved, err = reserveRedisModelSuccess(ctx, redisClient, key, newModelSuccessReservationToken(), 2, 60, now)
	require.NoError(t, err)
	assert.True(t, reserved)

	// 释放不存在的预留、commit 不存在的预留均为无害空操作。
	require.NoError(t, releaseRedisModelSuccess(ctx, redisClient, key, "never-reserved", 60))
	committed, err = commitRedisModelSuccess(ctx, redisClient, key, "never-reserved", 2, 60, now)
	require.NoError(t, err)
	assert.False(t, committed)
	length, err = redisClient.LLen(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), length)
}

func TestRedisModelSuccessReserveCountsLegacyCommittedEntries(t *testing.T) {
	_, redisClient := useRateLimitMiniRedis(t)
	ctx := context.Background()
	key := "rateLimit:MRRLS:legacy"
	now := time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC)
	format := func(at time.Time) string { return at.UTC().Format(modelRateLimitTimeFormat) }

	// 模拟旧版 recordRedisRequest 写入的窗口内记录与窗口外记录
	// （LPUSH 最新在前：参数按由旧到新排列）。
	inWindowRecent := format(now.Add(-10 * time.Second))
	inWindowOlder := format(now.Add(-20 * time.Second))
	expired := format(now.Add(-120 * time.Second))
	_, err := redisClient.LPush(ctx, key, expired, inWindowOlder, inWindowRecent).Result()
	require.NoError(t, err)

	// 窗口外记录在预留时被裁剪，窗口内旧记录继续计入。
	reserved, err := reserveRedisModelSuccess(ctx, redisClient, key, "token-1", 3, 60, now)
	require.NoError(t, err)
	require.True(t, reserved)
	entries, err := redisClient.LRange(ctx, key, 0, -1).Result()
	require.NoError(t, err)
	assert.Equal(t, []string{inWindowRecent, inWindowOlder}, entries)

	// committed 2 + reserved 1 达到上限 3。
	reserved, err = reserveRedisModelSuccess(ctx, redisClient, key, "token-2", 3, 60, now)
	require.NoError(t, err)
	assert.False(t, reserved)
}

func TestRedisModelSuccessReserveRejectsInvalidCommittedEntry(t *testing.T) {
	_, redisClient := useRateLimitMiniRedis(t)
	ctx := context.Background()
	key := "rateLimit:MRRLS:garbage"
	now := time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC)

	// 旧版 checkRedisRateLimit 对无法解析的记录报错并 fail-close（500），
	// 新版预留保持同一策略，而不是静默放行。
	_, err := redisClient.LPush(ctx, key, "not-a-timestamp").Result()
	require.NoError(t, err)
	reserved, err := reserveRedisModelSuccess(ctx, redisClient, key, "token-1", 1, 60, now)
	require.Error(t, err)
	assert.False(t, reserved)
	assert.Equal(t, int64(0), redisClient.ZCard(ctx, key+modelSuccessReservationKeySuffix).Val())
}

func TestRedisModelSuccessReserveRejectsInvalidArguments(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	ctx := context.Background()
	nowMs := time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC).UnixMilli()

	for _, testCase := range []struct {
		name string
		args []interface{}
	}{
		{name: "zero max", args: []interface{}{0, 60, nowMs, "token"}},
		{name: "negative max", args: []interface{}{-1, 60, nowMs, "token"}},
		{name: "zero window", args: []interface{}{1, 0, nowMs, "token"}},
		{name: "non-integer now", args: []interface{}{1, 60, "1.5", "token"}},
		{name: "zero now", args: []interface{}{1, 60, 0, "token"}},
		{name: "empty token", args: []interface{}{1, 60, nowMs, ""}},
		{name: "unsafe now", args: []interface{}{1, 60, int64(1)<<53, "token"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			key := "rateLimit:MRRLS:invalid-" + testCase.name
			err := redisClient.Eval(ctx, modelSuccessReserveLua, []string{key, key + modelSuccessReservationKeySuffix}, testCase.args...).Err()
			require.Error(t, err)
			assert.False(t, redisServer.Exists(key))
			assert.False(t, redisServer.Exists(key + modelSuccessReservationKeySuffix))
		})
	}
}

func TestRedisModelSuccessReservationRecoversAfterWindowExpiry(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	ctx := context.Background()
	key := "rateLimit:MRRLS:expiry"
	windowStart := time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC)

	// 结果未知的预留：不 commit 也不 release。
	reserved, err := reserveRedisModelSuccess(ctx, redisClient, key, "unknown-outcome", 1, 60, windowStart)
	require.NoError(t, err)
	require.True(t, reserved)
	reserved, err = reserveRedisModelSuccess(ctx, redisClient, key, "blocked", 1, 60, windowStart)
	require.NoError(t, err)
	require.False(t, reserved)

	// 窗口过期后由窗口裁剪自然回收，无需额外守护。
	reserved, err = reserveRedisModelSuccess(ctx, redisClient, key, "after-window", 1, 60, windowStart.Add(61*time.Second))
	require.NoError(t, err)
	assert.True(t, reserved)

	// 键 TTL 兜底：整个窗口无任何操作时键随 TTL 消失。
	redisServer.FastForward(61 * time.Second)
	assert.False(t, redisServer.Exists(key+modelSuccessReservationKeySuffix))
	assert.False(t, redisServer.Exists(key))
}

func TestRedisModelSuccessReserveIsAtomicUnderConcurrency(t *testing.T) {
	_, redisClient := useRateLimitMiniRedis(t)
	ctx := context.Background()
	key := "rateLimit:MRRLS:concurrent"
	now := time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC)
	const (
		requestCount = 50
		maxSuccess   = 10
	)

	allowedTokens := make(chan string, requestCount)
	errorsFound := make(chan error, requestCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(requestCount)
	for range requestCount {
		go func() {
			defer waitGroup.Done()
			token := newModelSuccessReservationToken()
			reserved, err := reserveRedisModelSuccess(ctx, redisClient, key, token, maxSuccess, 60, now)
			if err != nil {
				errorsFound <- err
				return
			}
			if reserved {
				allowedTokens <- token
			}
		}()
	}
	waitGroup.Wait()
	close(allowedTokens)
	close(errorsFound)
	for err := range errorsFound {
		require.NoError(t, err)
	}

	var tokens []string
	for token := range allowedTokens {
		tokens = append(tokens, token)
	}
	require.Len(t, tokens, maxSuccess, "并发预留数必须严格等于上限，不超卖")

	// 转正后 committed 达到上限，新预留被拒绝。
	for _, token := range tokens {
		committed, err := commitRedisModelSuccess(ctx, redisClient, key, token, maxSuccess, 60, now)
		require.NoError(t, err)
		require.True(t, committed)
	}
	length, err := redisClient.LLen(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(maxSuccess), length)
	reserved, err := reserveRedisModelSuccess(ctx, redisClient, key, newModelSuccessReservationToken(), maxSuccess, 60, now)
	require.NoError(t, err)
	assert.False(t, reserved)
}

func TestRedisModelRateLimitConcurrentRequestsNeverOversell(t *testing.T) {
	useGinTestMode(t)
	_, redisClient := useRateLimitMiniRedis(t)
	applyModelSuccessRateLimitConfig(t, 10)

	userID := int(2_100_000_000 + modelRateLimitTestUserSequence.Add(1))
	rejected, completed := runBarrierModelRateLimitConcurrency(t, userID, 50, 10)
	require.Len(t, rejected, 40)
	require.Len(t, completed, 10)
	for _, recorder := range rejected {
		assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "您已达到请求数限制")
		assert.Contains(t, recorder.Body.String(), `"type":"new_api_error"`)
	}
	for _, recorder := range completed {
		assert.Equal(t, http.StatusNoContent, recorder.Code)
	}

	// 10 个成功全部转正后达到上限，后续请求被拒绝。
	ctx := context.Background()
	successKey := fmt.Sprintf("rateLimit:%s:%d", ModelRequestRateLimitSuccessCountMark, userID)
	length, err := redisClient.LLen(ctx, successKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(10), length)

	followUp := httptest.NewRecorder()
	newBarrierRouterForFollowUp(userID).ServeHTTP(followUp, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusTooManyRequests, followUp.Code)
}

func TestRedisModelRateLimitFailureReleasesReservation(t *testing.T) {
	useGinTestMode(t)
	_, _ = useRateLimitMiniRedis(t)
	applyModelSuccessRateLimitConfig(t, 1)

	userID := int(2_100_000_000 + modelRateLimitTestUserSequence.Add(1))
	requestsHandled := 0
	router := newModelRateLimitRouter(userID, func(c *gin.Context) {
		requestsHandled++
		if requestsHandled <= 2 {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})

	perform := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		return recorder
	}

	// 失败请求释放预留：上限为 1 时后续请求仍可进入。
	assert.Equal(t, http.StatusInternalServerError, perform().Code)
	assert.Equal(t, http.StatusInternalServerError, perform().Code)
	// 成功请求转正占满上限。
	assert.Equal(t, http.StatusNoContent, perform().Code)
	assert.Equal(t, http.StatusTooManyRequests, perform().Code)
	assert.Equal(t, 3, requestsHandled)
}

func TestRedisModelRateLimitUnknownOutcomeKeepsReservationUntilExpiry(t *testing.T) {
	useGinTestMode(t)
	redisServer, _ := useRateLimitMiniRedis(t)
	applyModelSuccessRateLimitConfig(t, 1)

	userID := int(2_100_000_000 + modelRateLimitTestUserSequence.Add(1))
	panicOnce := atomic.Bool{}
	panicOnce.Store(true)
	router := newModelRateLimitRouter(userID, func(c *gin.Context) {
		if panicOnce.CompareAndSwap(true, false) {
			panic("simulated upstream panic")
		}
		c.Status(http.StatusNoContent)
	})

	// 无 Recovery 时 panic 穿透中间件，结算路径不执行：结果未知的预留被保留。
	assert.Panics(t, func() {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})

	// 预留保留期间新请求被拒绝（宁可误拒也不超卖）。
	blocked := httptest.NewRecorder()
	router.ServeHTTP(blocked, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusTooManyRequests, blocked.Code)

	// 窗口过期后预留自然回收（此处经键 TTL，窗口裁剪路径见
	// TestRedisModelSuccessReservationRecoversAfterWindowExpiry）。
	redisServer.FastForward(61 * time.Second)
	recovered := httptest.NewRecorder()
	router.ServeHTTP(recovered, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusNoContent, recovered.Code)
}

func TestRedisModelRateLimitSuccessCountZeroStaysUnlimited(t *testing.T) {
	useGinTestMode(t)
	redisServer, redisClient := useRateLimitMiniRedis(t)
	ctx := context.Background()

	// successMaxCount=0 与窗口=0 均保持旧版“不限制”语义，且不写任何键。
	for _, testCase := range []struct {
		name            string
		durationSeconds int64
		successMaxCount int
	}{
		{name: "zero success count", durationSeconds: 60, successMaxCount: 0},
		{name: "zero window", durationSeconds: 0, successMaxCount: 5},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			userID := int(2_100_000_000 + modelRateLimitTestUserSequence.Add(1))
			handler := redisRateLimitHandler(testCase.durationSeconds, 1, 0, testCase.successMaxCount)
			router := gin.New()
			router.GET("/", func(c *gin.Context) { c.Set("id", userID) }, handler, func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})
			for range 20 {
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
				require.Equal(t, http.StatusNoContent, recorder.Code)
			}
			keys, err := redisClient.Keys(ctx, "*").Result()
			require.NoError(t, err)
			assert.Empty(t, keys)
			assert.Empty(t, redisServer.Keys())
		})
	}
}

func TestMemoryModelRateLimitConcurrentRequestsNeverOversell(t *testing.T) {
	useGinTestMode(t)
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })
	applyModelSuccessRateLimitConfig(t, 10)

	userID := int(2_100_000_000 + modelRateLimitTestUserSequence.Add(1))
	rejected, completed := runBarrierModelRateLimitConcurrency(t, userID, 50, 10)
	require.Len(t, rejected, 40)
	require.Len(t, completed, 10)
	for _, recorder := range rejected {
		assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
		// 内存路径保持旧版行为：429 仅有状态码，无响应体。
		assert.Empty(t, recorder.Body.String())
	}
	for _, recorder := range completed {
		assert.Equal(t, http.StatusNoContent, recorder.Code)
	}

	// 已提交成功数达到上限，后续请求被拒绝。
	followUp := httptest.NewRecorder()
	newBarrierRouterForFollowUp(userID).ServeHTTP(followUp, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusTooManyRequests, followUp.Code)
}

func TestMemoryModelRateLimitFailureReleaseAndUnknownRetain(t *testing.T) {
	useGinTestMode(t)
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })
	applyModelSuccessRateLimitConfig(t, 1)

	t.Run("failure releases the reservation", func(t *testing.T) {
		userID := int(2_100_000_000 + modelRateLimitTestUserSequence.Add(1))
		requestsHandled := 0
		router := newModelRateLimitRouter(userID, func(c *gin.Context) {
			requestsHandled++
			if requestsHandled <= 2 {
				c.Status(http.StatusInternalServerError)
				return
			}
			c.Status(http.StatusNoContent)
		})
		perform := func() *httptest.ResponseRecorder {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			return recorder
		}
		assert.Equal(t, http.StatusInternalServerError, perform().Code)
		assert.Equal(t, http.StatusInternalServerError, perform().Code)
		assert.Equal(t, http.StatusNoContent, perform().Code)
		assert.Equal(t, http.StatusTooManyRequests, perform().Code)
		assert.Equal(t, 3, requestsHandled)
	})

	t.Run("unknown outcome retains the reservation", func(t *testing.T) {
		userID := int(2_100_000_000 + modelRateLimitTestUserSequence.Add(1))
		panicOnce := atomic.Bool{}
		panicOnce.Store(true)
		router := newModelRateLimitRouter(userID, func(c *gin.Context) {
			if panicOnce.CompareAndSwap(true, false) {
				panic("simulated upstream panic")
			}
			c.Status(http.StatusNoContent)
		})
		assert.Panics(t, func() {
			router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		})
		blocked := httptest.NewRecorder()
		router.ServeHTTP(blocked, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusTooManyRequests, blocked.Code)
	})
}

func TestModelSuccessReservationTokenIsUnique(t *testing.T) {
	const count = 1000
	seen := make(map[string]struct{}, count)
	for range count {
		token := newModelSuccessReservationToken()
		require.NotEmpty(t, token)
		_, duplicate := seen[token]
		require.False(t, duplicate)
		seen[token] = struct{}{}
	}
}
