package model

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ForceMind/MyAPI/common"
)

// tokenCacheSchemaVersion is bumped whenever a cached token gains a field or
// value domain that affects authorization. Version 3 widens quota state while
// preserving lazy per-key replacement of older hashes.
const tokenCacheSchemaVersion = 3

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

// tokenCacheFenceSeconds limits cache publication during ordinary mutation
// windows. A finite fence cannot cover every delayed reader. Authorization
// reads therefore consult the database even on a cache hit; cold accounting
// snapshots still require their own consistency guarantees.
// While the fence exists readers serve the database without caching.
const tokenCacheFenceSeconds = 10

// invalidateTokenCacheForMutation is called before a token metadata mutation
// writes to the database: it raises the fence and drops the cached hash to
// suppress publication during that window. It is not an authorization source.
func invalidateTokenCacheForMutation(key string) error {
	if !common.RedisEnabled || key == "" {
		return nil
	}
	if common.RDB == nil {
		return ErrBatchQuotaCacheUnavailable
	}
	// Fence publication and hash removal must be one Redis operation. Otherwise
	// a failed DEL can leave an old authorization hash readable behind a fence.
	script := `
local ttl = tonumber(ARGV[1])
if ttl == nil or ttl <= 0 or ttl ~= math.floor(ttl) then
  return redis.error_reply('invalid token cache fence ttl')
end
redis.call('SET', KEYS[1], '1', 'EX', ttl)
redis.call('DEL', KEYS[2])
return 1`
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := common.RDB.Eval(ctx, script, []string{
		getTokenCacheFenceKey(key), getTokenCacheKey(key),
	}, tokenCacheFenceSeconds).Err()
	if err != nil {
		common.SysError("failed to invalidate token cache for mutation: " + err.Error())
		return ErrBatchQuotaCacheUnavailable
	}
	return nil
}

// cacheInitToken only refreshes an existing hash. A delayed reader cannot
// publish a cold accounting snapshot; the subject owner must first recover
// pending generations and read the current database state before rebuilding.
// An existing hash's fields are preserved because it may contain batch deltas.
// 返回值：0=未发布，1=owner完成初始化，2=哈希已存在，仅刷新 TTL。
func cacheInitToken(token Token) (int, error) {
	return cacheInitTokenWithQuotaBalanceOwner(token, nil)
}

func cacheInitTokenWithQuotaBalanceOwner(token Token, balanceLock *quotaBalanceSubjectLock) (int, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	if _, err := common.CheckedQuotaBalance(token.RemainQuota, 0); err != nil {
		return 0, err
	}
	if _, err := common.CheckedQuotaUsed(token.UsedQuota, 0); err != nil {
		return 0, err
	}
	if token.QuotaVersion < 0 {
		return 0, ErrQuotaWriterEpochUnavailable
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
	script := redisExactQuotaPrelude + `
local balanceOwner = redis.call('GET', KEYS[4])
if (balanceOwner and balanceOwner ~= ARGV[21]) or (not balanceOwner and ARGV[21] ~= '') then
  return 4
end
if redis.call('EXISTS', KEYS[2]) == 1 and ARGV[21] == '' then
  return 0
end
local remainQuota = quotaStateInteger(ARGV[15], false)
local usedQuota = quotaStateInteger(ARGV[16], true)
local quotaVersion = canonicalNonnegativeInteger(ARGV[18])
local incomingEpoch = canonicalPositiveInteger(ARGV[20])
local tokenID = canonicalPositiveInteger(ARGV[1])
local cacheTTL = canonicalPositiveInteger(ARGV[17])
local cacheSchema = canonicalPositiveInteger(ARGV[19])
local globalEpoch = canonicalNonnegativeInteger(redis.call('GET', KEYS[3]) or '0')
if remainQuota == nil or usedQuota == nil or quotaVersion == nil or incomingEpoch == nil
  or tokenID == nil or cacheTTL == nil or cacheSchema == nil or globalEpoch == nil then
  return redis.error_reply('invalid token quota cache state')
end
local globalOrder = compareCanonicalNonnegative(globalEpoch, incomingEpoch)
if globalOrder > 0 then
  return 3
end
if redis.call('EXISTS', KEYS[1]) == 1 then
  if redis.call('HGET', KEYS[1], 'Id') ~= ARGV[1] then
    return 5
  end
  if redis.call('HGET', KEYS[1], 'CacheSchema') ~= ARGV[19] then
    if globalOrder < 0 then redis.call('SET', KEYS[3], ARGV[20]) end
    redis.call('DEL', KEYS[1])
  else
    local currentEpoch = canonicalPositiveInteger(redis.call('HGET', KEYS[1], 'QuotaWriterEpoch'))
    if currentEpoch == nil then return redis.error_reply('invalid cached writer epoch') end
    local currentRemain = quotaStateInteger(redis.call('HGET', KEYS[1], 'RemainQuota'), false)
    local currentUsed = quotaStateInteger(redis.call('HGET', KEYS[1], 'UsedQuota'), true)
    local currentQV = canonicalNonnegativeInteger(redis.call('HGET', KEYS[1], 'QuotaVersion'))
    if currentRemain == nil or currentUsed == nil or currentQV == nil then
      return redis.error_reply('invalid cached token quota state')
    end
    local epochOrder = compareCanonicalNonnegative(currentEpoch, incomingEpoch)
    if epochOrder == 0 then
      if ARGV[21] == '' and redis.call('PTTL', KEYS[1]) <= 0 then
        redis.call('DEL', KEYS[1])
        return 0
      end
      if globalOrder < 0 then redis.call('SET', KEYS[3], ARGV[20]) end
      redis.call('EXPIRE', KEYS[1], ARGV[17])
      return 2
    end
    if epochOrder > 0 then
      return 3
    end
    if globalOrder < 0 then redis.call('SET', KEYS[3], ARGV[20]) end
    redis.call('DEL', KEYS[1])
  end
elseif globalOrder < 0 then
  redis.call('SET', KEYS[3], ARGV[20])
end
if ARGV[21] == '' then
  return 0
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
	if result == 5 && err == nil {
		return result, ErrQuotaCacheIdentityMismatch
	}
	return result, err
}

// HydrateTokenQuotaCache updates an existing bridge/authoritative projection
// with epoch and QuotaVersion anti-rollback. Legacy balances use the subject
// owner. Cache misses are intentionally not materialized.
func HydrateTokenQuotaCache(key string, id int, remainQuota, usedQuota int64, quotaVersion int64) error {
	if !common.RedisEnabled || key == "" || id <= 0 {
		return nil
	}
	state, err := GetQuotaWriterEpochState(DB)
	if err != nil {
		return err
	}
	if _, err := requireQuotaProjectionEpoch(DB, state.Epoch); err != nil {
		return err
	}
	return hydrateTokenQuotaCacheRedisAtEpoch(key, id, remainQuota, usedQuota, quotaVersion, state.Epoch)
}

func hydrateTokenQuotaCacheRedisAtEpoch(key string, id int, remainQuota, usedQuota int64, quotaVersion, writerEpoch int64) error {
	if !common.RedisEnabled || key == "" || id <= 0 {
		return nil
	}
	if common.RDB == nil || writerEpoch <= 0 || quotaVersion < 0 {
		return ErrQuotaWriterEpochUnavailable
	}
	if _, err := common.CheckedQuotaBalance(remainQuota, 0); err != nil {
		return err
	}
	if _, err := common.CheckedQuotaUsed(usedQuota, 0); err != nil {
		return err
	}
	script := redisExactQuotaPrelude + `
local remainQuota = quotaStateInteger(ARGV[2], false)
local usedQuota = quotaStateInteger(ARGV[3], true)
local incomingQV = canonicalNonnegativeInteger(ARGV[4])
local incomingEpoch = canonicalPositiveInteger(ARGV[5])
local tokenID = canonicalPositiveInteger(ARGV[1])
local cacheSchema = canonicalPositiveInteger(ARGV[6])
local cacheTTL = canonicalPositiveInteger(ARGV[7])
if remainQuota == nil or usedQuota == nil or incomingQV == nil or incomingEpoch == nil
  or tokenID == nil or cacheSchema == nil or cacheTTL == nil then
  return redis.error_reply('invalid token quota cache state')
end
local globalEpoch = canonicalNonnegativeInteger(redis.call('GET', KEYS[2]) or '0')
if globalEpoch == nil then return redis.error_reply('invalid quota writer epoch') end
if redis.call('EXISTS', KEYS[1]) == 0 then
  return 2
end
if redis.call('HGET', KEYS[1], 'Id') ~= ARGV[1] then
  return 3
end
if redis.call('HGET', KEYS[1], 'CacheSchema') ~= ARGV[6] then
  redis.call('DEL', KEYS[1])
  return 2
end
local currentEpoch = canonicalPositiveInteger(redis.call('HGET', KEYS[1], 'QuotaWriterEpoch'))
local currentQV = canonicalNonnegativeInteger(redis.call('HGET', KEYS[1], 'QuotaVersion'))
local currentRemain = quotaStateInteger(redis.call('HGET', KEYS[1], 'RemainQuota'), false)
local currentUsed = quotaStateInteger(redis.call('HGET', KEYS[1], 'UsedQuota'), true)
if currentEpoch == nil or currentQV == nil or currentRemain == nil or currentUsed == nil then
  return redis.error_reply('invalid token quota cache state')
end
local globalOrder = compareCanonicalNonnegative(globalEpoch, incomingEpoch)
if globalOrder > 0 then
  return 0
end
local epochOrder = compareCanonicalNonnegative(incomingEpoch, currentEpoch)
if epochOrder < 0 or (epochOrder == 0 and compareCanonicalNonnegative(incomingQV, currentQV) <= 0) then
  redis.call('EXPIRE', KEYS[1], ARGV[7])
  return 0
end
if globalOrder < 0 then
  redis.call('SET', KEYS[2], ARGV[5])
end
redis.call('HSET', KEYS[1], 'RemainQuota', ARGV[2], 'UsedQuota', ARGV[3],
  'QuotaVersion', ARGV[4], 'QuotaWriterEpoch', ARGV[5])
redis.call('EXPIRE', KEYS[1], ARGV[7])
return 1`

	result, err := common.RDB.Eval(context.Background(), script,
		[]string{getTokenCacheKey(key), quotaWriterEpochRedisKey},
		strconv.Itoa(id), strconv.FormatInt(remainQuota, 10), strconv.FormatInt(usedQuota, 10),
		strconv.FormatInt(quotaVersion, 10), strconv.FormatInt(writerEpoch, 10),
		strconv.Itoa(tokenCacheSchemaVersion), tokenCacheTTLSeconds(),
	).Int()
	if err == nil && result == 3 {
		return ErrQuotaCacheIdentityMismatch
	}
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
	if common.RDB == nil {
		return nil, ErrBatchQuotaCacheUnavailable
	}
	ctx := context.Background()
	fenced, err := common.RDB.Exists(ctx, getTokenCacheFenceKey(key)).Result()
	if err != nil {
		return nil, err
	}
	if fenced != 0 {
		return nil, fmt.Errorf("token cache mutation is fenced")
	}
	var token Token
	if err := common.RedisHGetObj(getTokenCacheKey(key), &token); err != nil {
		return nil, err
	}
	if token.Id <= 0 {
		return nil, fmt.Errorf("token cache is incomplete")
	}
	if _, err := common.CheckedQuotaBalance(token.RemainQuota, 0); err != nil {
		return nil, err
	}
	if _, err := common.CheckedQuotaUsed(token.UsedQuota, 0); err != nil {
		return nil, err
	}
	values, err := common.RDB.HMGet(ctx, getTokenCacheKey(key), "CacheSchema", "QuotaWriterEpoch").Result()
	if err != nil || len(values) != 2 {
		return nil, fmt.Errorf("token cache epoch metadata is unavailable")
	}
	cacheSchema, schemaErr := strconv.Atoi(fmt.Sprint(values[0]))
	cacheEpoch, epochErr := strconv.ParseInt(fmt.Sprint(values[1]), 10, 64)
	redisEpoch, redisEpochErr := common.RDB.Get(ctx, quotaWriterEpochRedisKey).Int64()
	if schemaErr != nil || epochErr != nil || redisEpochErr != nil || cacheSchema != tokenCacheSchemaVersion || cacheEpoch <= 0 || cacheEpoch != redisEpoch {
		_ = common.RedisDelKey(getTokenCacheKey(key))
		return nil, ErrQuotaWriterEpochMismatch
	}
	fenced, err = common.RDB.Exists(ctx, getTokenCacheFenceKey(key)).Result()
	if err != nil {
		return nil, err
	}
	if fenced != 0 {
		return nil, fmt.Errorf("token cache mutation is fenced")
	}
	token.Key = key
	return &token, nil
}
