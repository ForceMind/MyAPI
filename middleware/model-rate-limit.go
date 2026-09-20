package middleware

import (
	"context"
	crand "crypto/rand"
	_ "embed"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/common/limiter"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
	modelRateLimitTimeFormat              = "2006-01-02T15:04:05.000Z"
	// modelSuccessReservationKeySuffix 是在途预留 ZSET 的键后缀。已提交成功记录
	// 仍存放在旧版 recordRedisRequest 使用的列表键上，滚动升级期间新旧实例
	// 共享同一份已提交数据。
	modelSuccessReservationKeySuffix = ":rsv"
)

//go:embed lua/model_success_reserve.lua
var modelSuccessReserveLua string

//go:embed lua/model_success_commit.lua
var modelSuccessCommitLua string

//go:embed lua/model_success_release.lua
var modelSuccessReleaseLua string

var (
	redisModelSuccessReserveScript = redis.NewScript(modelSuccessReserveLua)
	redisModelSuccessCommitScript  = redis.NewScript(modelSuccessCommitLua)
	redisModelSuccessReleaseScript = redis.NewScript(modelSuccessReleaseLua)
)

var modelSuccessReservationFallbackSequence atomic.Uint64

// newModelSuccessReservationToken 生成一次在途预留的唯一令牌。跨进程唯一性
// 由 crypto/rand 保证；随机源不可用时退化为纳秒时间戳加原子序号。
func newModelSuccessReservationToken() string {
	var randomBytes [16]byte
	if _, err := crand.Read(randomBytes[:]); err == nil {
		return hex.EncodeToString(randomBytes[:])
	}
	return fmt.Sprintf("fallback-%d-%d", time.Now().UnixNano(), modelSuccessReservationFallbackSequence.Add(1))
}

// reserveRedisModelSuccess 在窗口内原子占位：已提交成功数 + 在途预留数永不超过
// maxCount。窗口维度键与旧版 recordRedisRequest 完全一致，已提交列表中的旧
// 格式时间戳继续计入窗口。
func reserveRedisModelSuccess(ctx context.Context, rdb *redis.Client, key, token string, maxCount int, durationSeconds int64, now time.Time) (bool, error) {
	result, err := redisModelSuccessReserveScript.Run(
		ctx,
		rdb,
		[]string{key, key + modelSuccessReservationKeySuffix},
		maxCount,
		durationSeconds,
		now.UnixMilli(),
		token,
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// commitRedisModelSuccess 把在途预留转正为已提交成功记录。预留已随窗口过期
// 时返回 committed=false 且不再补记（硬限额优先于计数完整）。
func commitRedisModelSuccess(ctx context.Context, rdb *redis.Client, key, token string, maxCount int, durationSeconds int64, now time.Time) (bool, error) {
	result, err := redisModelSuccessCommitScript.Run(
		ctx,
		rdb,
		[]string{key, key + modelSuccessReservationKeySuffix},
		token,
		now.Format(modelRateLimitTimeFormat),
		maxCount,
		durationSeconds,
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// releaseRedisModelSuccess 释放失败请求的在途预留；预留不存在时为无害空操作。
func releaseRedisModelSuccess(ctx context.Context, rdb *redis.Client, key, token string, durationSeconds int64) error {
	return redisModelSuccessReleaseScript.Run(
		ctx,
		rdb,
		[]string{key + modelSuccessReservationKeySuffix},
		token,
		durationSeconds,
	).Err()
}

// Redis限流处理器
func redisRateLimitHandler(durationSeconds int64, durationMinutes, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		ctx := context.Background()
		rdb := common.RDB

		// 1. 成功请求数硬限额：请求进入时在窗口内原子预留（reserve），
		//    已提交 + 在途预留永不超过上限，并发下不超卖。
		//    successMaxCount<=0 或窗口<=0 时保持旧版“不限制”语义。
		successKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitSuccessCountMark, userId)
		reservationToken := ""
		if successMaxCount > 0 && durationSeconds > 0 {
			reservationToken = newModelSuccessReservationToken()
			allowed, err := reserveRedisModelSuccess(ctx, rdb, successKey, reservationToken, successMaxCount, durationSeconds, time.Now().UTC())
			if err != nil {
				// 与旧版 checkRedisRateLimit 一致：Redis 故障 fail-close（500）。
				fmt.Println("检查成功请求数限制失败:", err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}
			if !allowed {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", durationMinutes, successMaxCount))
				return
			}
		}

		// 2.检查总请求数限制并记录总请求（当totalMaxCount为0时会自动跳过，使用令牌桶限流器
		if totalMaxCount > 0 {
			totalKey := fmt.Sprintf("rateLimit:%s", userId)
			// 初始化
			tb := limiter.New(ctx, rdb)
			allowed, err := tb.Allow(
				ctx,
				totalKey,
				limiter.WithCapacity(int64(totalMaxCount)*durationSeconds),
				limiter.WithRate(int64(totalMaxCount)),
				limiter.WithRequested(durationSeconds),
			)

			if err != nil {
				fmt.Println("检查总请求数限制失败:", err.Error())
				if reservationToken != "" {
					// 总限额检查失败，本次请求不会成功，释放成功数预留。
					if releaseErr := releaseRedisModelSuccess(ctx, rdb, successKey, reservationToken, durationSeconds); releaseErr != nil {
						fmt.Println("释放成功请求数预留失败:", releaseErr.Error())
					}
				}
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}

			if !allowed {
				if reservationToken != "" {
					// 被总限额拒绝的请求必然失败，释放成功数预留。
					if releaseErr := releaseRedisModelSuccess(ctx, rdb, successKey, reservationToken, durationSeconds); releaseErr != nil {
						fmt.Println("释放成功请求数预留失败:", releaseErr.Error())
					}
				}
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", durationMinutes, totalMaxCount))
				return
			}
		}

		// 3. 处理请求
		c.Next()

		// 4. 结算成功数预留：
		//    成功（状态<400，与既有判定一致）→ commit 转正；
		//    失败 → release 释放，额度立即可用；
		//    结果未知（panic 穿透本中间件等无法执行到这里的路径）→ 保留预留，
		//    随窗口过期由键 TTL 与窗口裁剪自然回收，无需额外守护。
		if reservationToken == "" {
			return
		}
		if c.Writer.Status() < 400 {
			committed, err := commitRedisModelSuccess(ctx, rdb, successKey, reservationToken, successMaxCount, durationSeconds, time.Now().UTC())
			if err != nil {
				fmt.Println("记录成功请求数失败:", err.Error())
			} else if !committed {
				fmt.Println("成功请求数预留已随窗口过期，按硬限额语义不再补记")
			}
		} else {
			if err := releaseRedisModelSuccess(ctx, rdb, successKey, reservationToken, durationSeconds); err != nil {
				fmt.Println("释放成功请求数预留失败:", err.Error())
			}
		}
	}
}

// 内存限流处理器
func memoryRateLimitHandler(durationSeconds int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(durationSeconds) * time.Second)

	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		totalKey := ModelRequestRateLimitCountMark + userId
		successKey := ModelRequestRateLimitSuccessCountMark + userId

		// 1. 检查总请求数限制（当totalMaxCount为0时跳过）
		if totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, totalMaxCount, durationSeconds) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 2. 成功数硬限额：进入时原子预留；并发在途 + 已提交永不超过上限。
		//    successMaxCount<=0 时 Reserve 返回零值预留并放行，保持旧版语义。
		reservation, allowed := inMemoryRateLimiter.Reserve(successKey, successMaxCount, durationSeconds)
		if !allowed {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 3. 处理请求
		c.Next()

		// 4. 结算：成功（状态<400，与既有判定一致）→ commit；失败 → release；
		//    结果未知（panic 穿透等路径）→ 零值之外的预留保留，随窗口过期
		//    由清理协程与窗口裁剪自然回收。
		if c.Writer.Status() < 400 {
			inMemoryRateLimiter.Commit(reservation, successMaxCount, durationSeconds)
		} else {
			inMemoryRateLimiter.Release(reservation)
		}
	}
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 获取分组
		group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		if group == "" {
			group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		}

		limit := setting.ResolveModelRequestRateLimit(group)
		if !limit.Enabled {
			c.Next()
			return
		}
		durationSeconds := int64(limit.DurationMinutes) * 60

		// 根据存储类型选择并执行限流处理器
		if common.RedisEnabled {
			redisRateLimitHandler(durationSeconds, limit.DurationMinutes, limit.Total, limit.Success)(c)
		} else {
			memoryRateLimitHandler(durationSeconds, limit.Total, limit.Success)(c)
		}
	}
}
