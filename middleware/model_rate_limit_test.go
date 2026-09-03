package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
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

var modelRateLimitTestUserSequence atomic.Int64

func TestModelRedisRateLimitUsesUTCRegardlessOfLocalTimezone(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	previousLocation := time.Local
	time.Local = time.FixedZone("test-utc-plus-eight", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocation })

	ctx := context.Background()
	recordKey := "rateLimit:model-utc-record"
	recordRedisRequest(ctx, redisClient, recordKey, 2, 60)
	recorded, err := redisClient.LIndex(ctx, recordKey, 0).Result()
	require.NoError(t, err)
	recordedAt, err := time.Parse(modelRateLimitTimeFormat, recorded)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC(), recordedAt, 2*time.Second)

	checkKey := "rateLimit:model-utc-check"
	withinWindow := time.Now().UTC().Add(-30 * time.Second).Format(modelRateLimitTimeFormat)
	_, err = redisServer.Push(checkKey, withinWindow, withinWindow)
	require.NoError(t, err)
	allowed, err := checkRedisRateLimit(ctx, redisClient, checkKey, 2, 60)
	require.NoError(t, err)
	assert.False(t, allowed, "an existing UTC timestamp inside the window must remain limited on a non-UTC host")
}

func TestMemoryModelRateLimitCountsOnlySuccessfulRequests(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	previousConfig := setting.GetModelRequestRateLimitConfig()
	require.NoError(t, setting.ApplyModelRequestRateLimitConfig(setting.ModelRequestRateLimitConfig{
		Enabled:         true,
		DurationMinutes: 1,
		Total:           0,
		Success:         1,
		Group:           map[string][2]int{},
	}))
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		require.NoError(t, setting.ApplyModelRequestRateLimitConfig(previousConfig))
	})

	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	userID := int(2_000_000_000 + modelRateLimitTestUserSequence.Add(1))
	requestsHandled := 0
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", userID)
		c.Set(string(constant.ContextKeyUserGroup), "default")
		c.Next()
	})
	router.Use(ModelRequestRateLimit())
	router.GET("/", func(c *gin.Context) {
		requestsHandled++
		if requestsHandled == 1 {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusInternalServerError, first.Code)
	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusNoContent, second.Code)
	third := httptest.NewRecorder()
	router.ServeHTTP(third, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusTooManyRequests, third.Code)
	assert.Equal(t, 2, requestsHandled)
}
