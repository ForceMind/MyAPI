package model

import (
	"context"
	"strconv"
	"time"

	"github.com/ForceMind/MyAPI/common"
)

// LegacyBatchQuotaTrustEligible checks live balances in one read-only Redis
// operation. Database fallbacks may omit pending batch deltas and cannot prove
// trust eligibility. A miss or uncertainty leaves admission to real reserve.
// This is the existing high-balance optimization, not an atomic reservation.
func LegacyBatchQuotaTrustEligible(userID, tokenID int, tokenKey string, unlimited bool, writerEpoch int64, threshold int) (bool, error) {
	if !common.RedisEnabled || common.RDB == nil || userID <= 0 || tokenID <= 0 || tokenKey == "" || writerEpoch <= 0 || threshold <= 0 {
		return false, nil
	}
	userLockKey, err := quotaBalanceSubjectLockKey(BatchUpdateTypeUserQuota, userID)
	if err != nil {
		return false, err
	}
	tokenLockKey, err := quotaBalanceSubjectLockKey(BatchUpdateTypeTokenQuota, tokenID)
	if err != nil {
		return false, err
	}
	script := redisExactQuotaPrelude + `
if redis.call('PTTL', KEYS[1]) <= 0 or redis.call('PTTL', KEYS[2]) <= 0 then return 0 end
for index = 4, 6 do
  if redis.call('EXISTS', KEYS[index]) ~= 0 then return 0 end
end
local epoch = canonicalPositiveInteger(redis.call('GET', KEYS[3]))
if epoch == nil or epoch ~= ARGV[5] then return 0 end
local user = redis.call('HMGET', KEYS[1], 'Id', 'CacheSchema', 'QuotaWriterEpoch', 'QuotaVersion', 'AuthVersion', 'Quota')
local token = redis.call('HMGET', KEYS[2], 'Id', 'UserId', 'CacheSchema', 'QuotaWriterEpoch', 'QuotaVersion', 'RemainQuota', 'UsedQuota')
if user[1] ~= ARGV[1] or token[1] ~= ARGV[2] or token[2] ~= ARGV[1]
  or user[2] ~= ARGV[3] or token[3] ~= ARGV[4]
  or user[3] ~= epoch or token[4] ~= epoch then return 0 end
if canonicalNonnegativeInteger(user[4]) == nil or canonicalNonnegativeInteger(token[5]) == nil then return 0 end
local authVersion = canonicalPositiveInteger(user[5])
local pending = canonicalNonnegativeInteger(redis.call('GET', KEYS[7]) or '0')
local committed = canonicalNonnegativeInteger(redis.call('GET', KEYS[8]) or '0')
if authVersion == nil or pending == nil or committed == nil
  or compareCanonicalNonnegative(pending, authVersion) > 0
  or compareCanonicalNonnegative(committed, authVersion) > 0 then return 0 end
local userQuota = quotaStateInteger(user[6], false)
local tokenQuota = quotaStateInteger(token[6], false)
local tokenUsed = quotaStateInteger(token[7], true)
local threshold = quotaStateInteger(ARGV[6], true)
if userQuota == nil or tokenQuota == nil or tokenUsed == nil or threshold == nil
  or threshold <= 0 or userQuota <= threshold then return 0 end
if ARGV[7] == 'false' then
  if tokenQuota <= threshold then return 0 end
elseif ARGV[7] ~= 'true' then
  return 0
end
return 1`
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	result, err := common.RDB.Eval(ctx, script, []string{
		getUserCacheKey(userID), getTokenCacheKey(tokenKey), quotaWriterEpochRedisKey,
		userLockKey, tokenLockKey, getTokenCacheFenceKey(tokenKey),
		getUserAuthFenceKey(userID), getUserAuthVersionKey(userID),
	}, userID, tokenID, userCacheSchemaVersion, tokenCacheSchemaVersion,
		strconv.FormatInt(writerEpoch, 10), threshold, strconv.FormatBool(unlimited)).Int()
	return result == 1 && err == nil, err
}
