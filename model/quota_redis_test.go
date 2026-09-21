package model

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func s2cRedisFixtureOptions(address string) (*redis.Options, error) {
	host, port, err := net.SplitHostPort(address)
	number, portErr := strconv.Atoi(port)
	if err != nil || !net.ParseIP(host).IsLoopback() || portErr != nil || number <= 0 || number > 65535 {
		return nil, errors.New("S2-C Redis fixture requires a literal loopback TCP address and valid port")
	}
	return &redis.Options{Addr: address, DB: 15, MaxRetries: -1}, nil
}

func TestS2CQuotaRedisTargetSafety(t *testing.T) {
	for _, address := range []string{"", "localhost:6379", "192.0.2.1:6379", "/tmp/redis.sock", "127.0.0.1:0", "127.0.0.1:65536"} {
		t.Run(address, func(t *testing.T) {
			_, err := s2cRedisFixtureOptions(address)
			require.Error(t, err)
		})
	}
}

// Only disposable CI Redis 7 instances are allowed. Require the entire instance
// to be empty before any write, use database 15, never FLUSH or delete keys, and
// let the service/container lifecycle own cleanup. PEXPIRETIME checks exact
// expiration instants, rather than comparing elapsed TTLs or using sleeps.
func TestS2CQuotaConfiguredRedis(t *testing.T) {
	if os.Getenv("MYAPI_S2C_REDIS_TESTS") != "1" {
		t.Skip("disposable Redis tests require MYAPI_S2C_REDIS_TESTS=1")
	}
	options, err := s2cRedisFixtureOptions(os.Getenv("MYAPI_S2C_REDIS_ADDR"))
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

	for _, operation := range []struct {
		name, script   string
		token, reserve bool
	}{
		{"user-reserve", userQuotaReserveScript, false, true},
		{"user-delta", userQuotaDeltaScript, false, false},
		{"token-reserve", tokenQuotaReserveScript, true, true},
		{"token-delta", tokenQuotaDeltaScript, true, false},
	} {
		for _, scenario := range []string{"valid", "negative", "missing-argument", "fraction", "oversized", "missing-field", "malformed-field", "noncanonical-field", "result-overflow"} {
			t.Run(operation.name+"/"+scenario, func(t *testing.T) {
				key := "myapi:s2c:" + operation.name + ":" + scenario
				writerEpoch := int64(7)
				before := map[string]string{"Id": "42", "CacheSchema": strconv.Itoa(userCacheSchemaVersion), "Quota": "100", "RemainQuota": "100", "UsedQuota": "20", "AccessedTime": "99", "Name": "preserve", "QuotaWriterEpoch": strconv.FormatInt(writerEpoch, 10)}
				third := userCacheSchemaVersion
				if operation.token {
					third = 123
				}
				args := []interface{}{10, 42, third, writerEpoch}
				want := 1
				field := "Quota"
				if operation.token {
					field = "UsedQuota"
				}
				switch scenario {
				case "negative":
					args[0] = -10
					if operation.reserve {
						want = -2
					}
				case "missing-argument":
					args, want = args[:2], -2
				case "fraction":
					args[0], want = "1.5", -2
				case "oversized":
					args[0], want = int64(common.MaxQuota)+1, -2
				case "missing-field":
					delete(before, field)
					want = -1
				case "malformed-field":
					before[field], want = "bad", -1
				case "noncanonical-field":
					before[field], want = "01", -1
				case "result-overflow":
					if operation.token {
						before["UsedQuota"], want = strconv.Itoa(common.MaxQuota), -2
						if !operation.reserve {
							before["UsedQuota"] = strconv.Itoa(common.MinQuota)
						}
					} else if operation.reserve {
						before["Quota"], want = "-10", 0 // Insufficient debt balance; never a credit.
					} else {
						before["Quota"], want = strconv.Itoa(common.MaxQuota), -2
					}
				}
				require.NoError(t, client.HSet(ctx, key, before).Err())
				require.NoError(t, client.Set(ctx, quotaWriterEpochRedisKey, writerEpoch, 0).Err())
				require.NoError(t, client.Expire(ctx, key, time.Minute).Err())
				expires, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
				require.NoError(t, err)
				result, err := client.Eval(ctx, operation.script, []string{key, quotaWriterEpochRedisKey}, args...).Int()
				require.NoError(t, err)
				assert.Equal(t, want, result)
				expected := make(map[string]string, len(before))
				for name, value := range before {
					expected[name] = value
				}
				if want == 1 {
					delta := 10
					if operation.reserve || scenario == "negative" {
						delta = -10
					}
					if operation.token {
						expected["RemainQuota"], expected["UsedQuota"], expected["AccessedTime"] = strconv.Itoa(100+delta), strconv.Itoa(20-delta), "123"
					} else {
						expected["Quota"] = strconv.Itoa(100 + delta)
					}
				}
				after, err := client.HGetAll(ctx, key).Result()
				require.NoError(t, err)
				assert.Equal(t, expected, after)
				afterExpiry, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
				require.NoError(t, err)
				assert.Equal(t, expires, afterExpiry, fmt.Sprintf("%s must not refresh expiration", scenario))
			})
		}
	}
}
