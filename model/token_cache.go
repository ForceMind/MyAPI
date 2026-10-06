package model

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ForceMind/MyAPI/common"
)

// tokenCacheSchemaVersion is bumped whenever a cached token gains a field
// that affects authorization. Version 2 adds AccessProfileID: treating an old
// projection as complete would make a cache hit silently fall back to the
// legacy Group and could evaluate a different access profile from the one
// persisted on the token.
const tokenCacheSchemaVersion = 2

func getTokenCacheKey(key string) string {
	return fmt.Sprintf("token:%s", common.GenerateHMAC(key))
}

func getTokenCacheFenceKey(key string) string {
	return fmt.Sprintf("token:fence:%s", common.GenerateHMAC(key))
}

func tokenCacheTTLSeconds() int {
	ttl := common.RedisKeyCacheSeconds()
	if ttl <= 0 {
		return 60
	}
	return ttl
}

// tokenCacheFenceSeconds must outlive a token mutation's database write plus
// any in-flight reader's DB-read-to-cache-init gap. The fence is not deleted
// after commit; it expires naturally so a reader holding a pre-mutation
// snapshot cannot publish it right after the mutation cleared the cache.
// While the fence exists readers simply serve the database without caching.
const tokenCacheFenceSeconds = 10

// invalidateTokenCacheForMutation is called before a token metadata mutation
// writes to the database: it raises the fence and drops the cached hash so no
// reader can act on (or re-publish) the pre-mutation state.
func invalidateTokenCacheForMutation(key string) error {
	if !common.RedisEnabled || key == "" {
		return nil
	}
	ctx := context.Background()
	err := common.RDB.Set(ctx, getTokenCacheFenceKey(key), 1, time.Duration(tokenCacheFenceSeconds)*time.Second).Err()
	if err != nil {
		return err
	}
	return common.RDB.Del(ctx, getTokenCacheKey(key)).Err()
}

// cacheInitToken publishes a database snapshot only when no mutation fence is
// active and the hash is cold. An existing hash only gets its TTL refreshed:
// its RemainQuota may already be ahead of this snapshot because atomic
// pre-consume decrements Redis first, so a snapshot must never overwrite any
// field of a live hash.
// 返回值：0=被 fence 拦截，1=完成初始化，2=哈希已存在，仅刷新 TTL。
func cacheInitToken(token Token) (int, error) {
	return cacheInitTokenWithQuotaBalanceOwner(token, nil)
}

func cacheInitTokenWithQuotaBalanceOwner(token Token, balanceLock *quotaBalanceSubjectLock) (int, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	state, err := GetQuotaWriterEpochState(DB)
	if err != nil {
		return 0, err
	}
	allowIps := ""
	if token.AllowIps != nil {
		allowIps = *token.AllowIps
	}
	balanceLockKey, err := quotaBalanceSubjectLockKey(BatchUpdateTypeTokenQuota, token.Id)
	if err != nil {
		return 0, err
	}
	balanceLockToken := ""
	if balanceLock != nil {
		balanceLockToken = balanceLock.Token
	}
	const script = `
local balanceOwner = redis.call('GET', KEYS[4])
if (balanceOwner and balanceOwner ~= ARGV[21]) or (not balanceOwner and ARGV[21] ~= '') then
  return 4
end
if redis.call('EXISTS', KEYS[2]) == 1 and ARGV[21] == '' then
  return 0
end
local incomingEpoch = tonumber(ARGV[20])
local globalEpoch = tonumber(redis.call('GET', KEYS[3]) or '0')
if globalEpoch > incomingEpoch then
  return 3
end
if globalEpoch < incomingEpoch then
  redis.call('SET', KEYS[3], ARGV[20])
end
if redis.call('EXISTS', KEYS[1]) == 1 then
  local currentEpoch = tonumber(redis.call('HGET', KEYS[1], 'QuotaWriterEpoch') or '-1')
  if redis.call('HGET', KEYS[1], 'CacheSchema') == ARGV[19] and currentEpoch == incomingEpoch then
    redis.call('EXPIRE', KEYS[1], ARGV[17])
    return 2
  end
  if currentEpoch > incomingEpoch then
    return 3
  end
  redis.call('DEL', KEYS[1])
end
redis.call('HSET', KEYS[1],
  'Id', ARGV[1], 'UserId', ARGV[2], 'Status', ARGV[3], 'Name', ARGV[4],
  'CreatedTime', ARGV[5], 'AccessedTime', ARGV[6], 'ExpiredTime', ARGV[7],
  'UnlimitedQuota', ARGV[8], 'ModelLimitsEnabled', ARGV[9], 'ModelLimits', ARGV[10],
  'AllowIps', ARGV[11], 'Group', ARGV[12], 'CrossGroupRetry', ARGV[13],
  'AutoGroups', ARGV[14], 'RemainQuota', ARGV[15], 'UsedQuota', ARGV[16],
  'QuotaVersion', ARGV[18], 'CacheSchema', ARGV[19], 'QuotaWriterEpoch', ARGV[20],
  'AccessProfileID', ARGV[22])
redis.call('EXPIRE', KEYS[1], ARGV[17])
if ARGV[21] ~= '' then
  redis.call('DEL', KEYS[2])
end
return 1`

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := common.RDB.Eval(ctx, script, []string{
		getTokenCacheKey(token.Key), getTokenCacheFenceKey(token.Key), quotaWriterEpochRedisKey, balanceLockKey,
	},
		token.Id, token.UserId, token.Status, token.Name,
		token.CreatedTime, token.AccessedTime, token.ExpiredTime,
		strconv.FormatBool(token.UnlimitedQuota), strconv.FormatBool(token.ModelLimitsEnabled),
		token.ModelLimits, allowIps, token.Group, strconv.FormatBool(token.CrossGroupRetry),
		token.AutoGroups, token.RemainQuota, token.UsedQuota,
		tokenCacheTTLSeconds(), strconv.FormatInt(token.QuotaVersion, 10), tokenCacheSchemaVersion,
		strconv.FormatInt(state.Epoch, 10),
		balanceLockToken,
		token.AccessProfileID,
	).Int()
	if result == 3 && err == nil {
		return result, ErrQuotaWriterEpochMismatch
	}
	if result == 4 && err == nil {
		return result, ErrQuotaBalanceMutationUnknown
	}
	return result, err
}

// HydrateTokenQuotaCache updates an existing token projection with epoch and
// QuotaVersion anti-rollback. Cache misses are intentionally not materialized.
func HydrateTokenQuotaCache(key string, id int, remainQuota, usedQuota int, quotaVersion int64) error {
	if !common.RedisEnabled || key == "" || id <= 0 {
		return nil
	}
	state, err := GetQuotaWriterEpochState(DB)
	if err != nil {
		return err
	}
	return hydrateTokenQuotaCacheRedisAtEpoch(key, id, remainQuota, usedQuota, quotaVersion, state.Epoch)
}

func hydrateTokenQuotaCacheRedisAtEpoch(key string, id int, remainQuota, usedQuota int, quotaVersion, writerEpoch int64) error {
	if !common.RedisEnabled || key == "" || id <= 0 {
		return nil
	}
	if common.RDB == nil || writerEpoch <= 0 || quotaVersion < 0 {
		return ErrQuotaWriterEpochUnavailable
	}
	const script = `
if redis.call('EXISTS', KEYS[1]) == 0 then
  return 2
end
if redis.call('HGET', KEYS[1], 'Id') ~= ARGV[1] or redis.call('HGET', KEYS[1], 'CacheSchema') ~= ARGV[6] then
  redis.call('DEL', KEYS[1])
  return 2
end
local incomingEpoch = tonumber(ARGV[5])
local globalEpoch = tonumber(redis.call('GET', KEYS[2]) or '0')
if globalEpoch > incomingEpoch then
  return 0
end
if globalEpoch < incomingEpoch then
  redis.call('SET', KEYS[2], ARGV[5])
end
local currentEpoch = tonumber(redis.call('HGET', KEYS[1], 'QuotaWriterEpoch') or '-1')
if currentEpoch == -1 then
  redis.call('DEL', KEYS[1])
  return 2
end
local currentQV = tonumber(redis.call('HGET', KEYS[1], 'QuotaVersion') or '-1')
local incomingQV = tonumber(ARGV[4])
if incomingEpoch < currentEpoch or (incomingEpoch == currentEpoch and incomingQV <= currentQV) then
  redis.call('EXPIRE', KEYS[1], ARGV[7])
  return 0
end
redis.call('HSET', KEYS[1], 'RemainQuota', ARGV[2], 'UsedQuota', ARGV[3],
  'QuotaVersion', ARGV[4], 'QuotaWriterEpoch', ARGV[5])
redis.call('EXPIRE', KEYS[1], ARGV[7])
return 1`

	_, err := common.RDB.Eval(context.Background(), script,
		[]string{getTokenCacheKey(key), quotaWriterEpochRedisKey},
		strconv.Itoa(id), strconv.Itoa(remainQuota), strconv.Itoa(usedQuota),
		strconv.FormatInt(quotaVersion, 10), strconv.FormatInt(writerEpoch, 10),
		strconv.Itoa(tokenCacheSchemaVersion), tokenCacheTTLSeconds(),
	).Int()
	return err
}

// InvalidateTokenQuotaCache invalidates the token cache for mutation
func InvalidateTokenQuotaCache(key string) error {
	return invalidateTokenCacheForMutation(key)
}

// cacheGetTokenByKey 从缓存读取 token；不完整的哈希（如仅有配额字段）会被拒绝。
func cacheGetTokenByKey(key string) (*Token, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	var token Token
	if err := common.RedisHGetObj(getTokenCacheKey(key), &token); err != nil {
		return nil, err
	}
	if token.Id <= 0 {
		return nil, fmt.Errorf("token cache is incomplete")
	}
	values, err := common.RDB.HMGet(context.Background(), getTokenCacheKey(key), "CacheSchema", "QuotaWriterEpoch").Result()
	if err != nil || len(values) != 2 {
		return nil, fmt.Errorf("token cache epoch metadata is unavailable")
	}
	cacheSchema, schemaErr := strconv.Atoi(fmt.Sprint(values[0]))
	cacheEpoch, epochErr := strconv.ParseInt(fmt.Sprint(values[1]), 10, 64)
	redisEpoch, redisEpochErr := common.RDB.Get(context.Background(), quotaWriterEpochRedisKey).Int64()
	if schemaErr != nil || epochErr != nil || redisEpochErr != nil || cacheSchema != tokenCacheSchemaVersion || cacheEpoch <= 0 || cacheEpoch != redisEpoch {
		_ = common.RedisDelKey(getTokenCacheKey(key))
		return nil, ErrQuotaWriterEpochMismatch
	}
	token.Key = key
	return &token, nil
}
