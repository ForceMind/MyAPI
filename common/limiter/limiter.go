package limiter

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/go-redis/redis/v8"
)

//go:embed lua/rate_limit.lua
var rateLimitScript string

var redisRateLimitScript = redis.NewScript(rateLimitScript)

// MaxExactInteger is the largest integer that the Redis Lua runtime can
// represent exactly. Limiter configuration values must stay within this
// boundary because Redis executes Lua numbers as IEEE-754 doubles.
const MaxExactInteger int64 = 1<<53 - 1

type RedisLimiter struct {
	client *redis.Client
}

func New(_ context.Context, r *redis.Client) *RedisLimiter {
	return &RedisLimiter{client: r}
}

func (rl *RedisLimiter) Allow(ctx context.Context, key string, opts ...Option) (bool, error) {
	// 默认配置
	config := &Config{
		Capacity:  10,
		Rate:      1,
		Requested: 1,
	}

	// 应用选项模式
	for _, opt := range opts {
		opt(config)
	}
	if rl == nil || rl.client == nil {
		return false, errors.New("rate limit failed: redis client is nil")
	}
	if err := ValidateConfig(*config); err != nil {
		return false, err
	}

	// 执行限流
	result, err := redisRateLimitScript.Run(
		ctx,
		rl.client,
		[]string{key},
		config.Requested,
		config.Rate,
		config.Capacity,
	).Int()

	if err != nil {
		return false, fmt.Errorf("rate limit failed: %w", err)
	}
	return result == 1, nil
}

// Config 配置选项模式
type Config struct {
	Capacity  int64
	Rate      int64
	Requested int64
}

// ValidateConfig verifies that a limiter configuration can be passed to the
// Redis Lua runtime without changing its integer values.
func ValidateConfig(config Config) error {
	if config.Capacity <= 0 || config.Rate <= 0 || config.Requested <= 0 {
		return errors.New("rate limit failed: capacity, rate, and requested must be positive")
	}
	if config.Requested > config.Capacity {
		return errors.New("rate limit failed: requested must not exceed capacity")
	}
	if config.Capacity > MaxExactInteger || config.Rate > MaxExactInteger || config.Requested > MaxExactInteger {
		return fmt.Errorf("rate limit failed: capacity, rate, and requested must not exceed %d", MaxExactInteger)
	}
	return nil
}

type Option func(*Config)

func WithCapacity(c int64) Option {
	return func(cfg *Config) { cfg.Capacity = c }
}

func WithRate(r int64) Option {
	return func(cfg *Config) { cfg.Rate = r }
}

func WithRequested(n int64) Option {
	return func(cfg *Config) { cfg.Requested = n }
}
