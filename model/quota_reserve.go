package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

type cacheQuotaResult int

const (
	cacheQuotaInsufficient cacheQuotaResult = iota
	cacheQuotaOK
	cacheQuotaMiss
)

var errInvalidQuotaCacheOperation = errors.New("invalid quota cache operation")

// Validate canonical integer strings before the first Redis write. Lua numbers
// are doubles; restricting quota arithmetic to the shared int32 policy bounds
// also keeps every addition exact. Redis scripts do not roll back earlier writes
// when a later HINCRBY fails.
var quotaCacheScriptPrelude = fmt.Sprintf(`
local minimum = %d
local maximum = %d
local function integerText(value)
  return type(value) == 'string' and (value == '0' or string.match(value, '^%%-?[1-9]%%d*$') ~= nil)
end
local function quotaInteger(value)
  if not integerText(value) then return nil end
  local number = tonumber(value)
  if number == nil or number < minimum or number > maximum then return nil end
  return number
end
local function inRange(value)
  return value >= minimum and value <= maximum
end
local function validTimestamp(value)
  return integerText(value) and string.sub(value, 1, 1) ~= '-'
    and (#value < 19 or (#value == 19 and value <= '9223372036854775807'))
end
if #KEYS ~= 2 or #ARGV ~= 4 or not integerText(ARGV[2]) or ARGV[2] == '0'
  or string.sub(ARGV[2], 1, 1) == '-' or not integerText(ARGV[4])
  or string.sub(ARGV[4], 1, 1) == '-' or ARGV[4] == '0' then
  return -2
end
local amount = quotaInteger(ARGV[1])
if amount == nil then return -2 end
local globalEpoch = redis.call('GET', KEYS[2])
if not globalEpoch then
  redis.call('SET', KEYS[2], ARGV[4])
elseif globalEpoch ~= ARGV[4] then
  return -3
end
`, common.MinQuota, common.MaxQuota)

var userQuotaReserveScript = quotaCacheScriptPrelude + `
if amount < 0 or not integerText(ARGV[3]) then return -2 end
if redis.call('HGET', KEYS[1], 'Id') ~= ARGV[2]
  or redis.call('HGET', KEYS[1], 'CacheSchema') ~= ARGV[3] then
  return -1
end
if redis.call('HGET', KEYS[1], 'QuotaWriterEpoch') ~= ARGV[4] then return -3 end
local quota = quotaInteger(redis.call('HGET', KEYS[1], 'Quota'))
if quota == nil then return -1 end
if quota < amount then
  return 0
end
redis.call('HINCRBY', KEYS[1], 'Quota', string.format('%.0f', -amount))
return 1`

var userQuotaDeltaScript = quotaCacheScriptPrelude + `
if not integerText(ARGV[3]) then return -2 end
if redis.call('HGET', KEYS[1], 'Id') ~= ARGV[2]
  or redis.call('HGET', KEYS[1], 'CacheSchema') ~= ARGV[3] then
  return -1
end
if redis.call('HGET', KEYS[1], 'QuotaWriterEpoch') ~= ARGV[4] then return -3 end
local quota = quotaInteger(redis.call('HGET', KEYS[1], 'Quota'))
if quota == nil then return -1 end
if not inRange(quota + amount) then return -2 end
redis.call('HINCRBY', KEYS[1], 'Quota', ARGV[1])
return 1`

var tokenQuotaReserveScript = quotaCacheScriptPrelude + `
if amount < 0 or not validTimestamp(ARGV[3]) then return -2 end
if redis.call('HGET', KEYS[1], 'Id') ~= ARGV[2] then
  return -1
end
if redis.call('HGET', KEYS[1], 'QuotaWriterEpoch') ~= ARGV[4] then return -3 end
local remain = quotaInteger(redis.call('HGET', KEYS[1], 'RemainQuota'))
local used = quotaInteger(redis.call('HGET', KEYS[1], 'UsedQuota'))
if remain == nil or used == nil then return -1 end
if remain < amount then
  return 0
end
if not inRange(used + amount) then return -2 end
redis.call('HINCRBY', KEYS[1], 'RemainQuota', string.format('%.0f', -amount))
redis.call('HINCRBY', KEYS[1], 'UsedQuota', ARGV[1])
redis.call('HSET', KEYS[1], 'AccessedTime', ARGV[3])
return 1`

var tokenQuotaDeltaScript = quotaCacheScriptPrelude + `
if not validTimestamp(ARGV[3]) then return -2 end
if redis.call('HGET', KEYS[1], 'Id') ~= ARGV[2] then
  return -1
end
if redis.call('HGET', KEYS[1], 'QuotaWriterEpoch') ~= ARGV[4] then return -3 end
local remain = quotaInteger(redis.call('HGET', KEYS[1], 'RemainQuota'))
local used = quotaInteger(redis.call('HGET', KEYS[1], 'UsedQuota'))
if remain == nil or used == nil then return -1 end
if not inRange(remain + amount) or not inRange(used - amount) then return -2 end
redis.call('HINCRBY', KEYS[1], 'RemainQuota', ARGV[1])
redis.call('HINCRBY', KEYS[1], 'UsedQuota', string.format('%.0f', -amount))
redis.call('HSET', KEYS[1], 'AccessedTime', ARGV[3])
return 1`

func quotaJournaledMutationScript(base string) string {
	script := strings.Replace(base, "if #KEYS ~= 2", "if #KEYS ~= 4", 1)
	script = strings.Replace(script, "#ARGV ~= 4", "#ARGV ~= 5", 1)
	script = "if redis.call('GET', KEYS[4]) ~= ARGV[#ARGV] then return -6 end\nlocal journal = redis.call('GET', KEYS[3])\nif journal == 'applied' then return 2 end\nif journal ~= 'prepared' then return -4 end\n" + script
	index := strings.LastIndex(script, "return 1")
	return script[:index] + "redis.call('HSET', KEYS[1], 'QuotaBalanceGeneration', KEYS[3])\nredis.call('SET', KEYS[3], 'applied', 'EX', 604800)\nreturn 1" + script[index+len("return 1"):]
}

func quotaJournaledCompensationScript(base string) string {
	script := strings.Replace(base, "if #KEYS ~= 2", "if #KEYS ~= 4", 1)
	script = strings.Replace(script, "#ARGV ~= 4", "#ARGV ~= 5", 1)
	script = "if redis.call('GET', KEYS[4]) ~= ARGV[#ARGV] then return -6 end\nif redis.call('GET', KEYS[3]) ~= 'applied' then return 2 end\nif redis.call('HGET', KEYS[1], 'QuotaBalanceGeneration') ~= KEYS[3] then return -5 end\n" + script
	index := strings.LastIndex(script, "return 1")
	return script[:index] + "redis.call('HDEL', KEYS[1], 'QuotaBalanceGeneration')\nredis.call('SET', KEYS[3], 'compensated', 'EX', 604800)\nreturn 1" + script[index+len("return 1"):]
}

var (
	userQuotaReserveJournaledScript  = quotaJournaledMutationScript(userQuotaReserveScript)
	userQuotaDeltaJournaledScript    = quotaJournaledMutationScript(userQuotaDeltaScript)
	tokenQuotaReserveJournaledScript = quotaJournaledMutationScript(tokenQuotaReserveScript)
	tokenQuotaDeltaJournaledScript   = quotaJournaledMutationScript(tokenQuotaDeltaScript)
	userQuotaCompensationScript      = quotaJournaledCompensationScript(userQuotaDeltaScript)
	tokenQuotaCompensationScript     = quotaJournaledCompensationScript(tokenQuotaDeltaScript)
)

func quotaResultFromLua(result int, err error) (cacheQuotaResult, error) {
	if err != nil {
		return cacheQuotaMiss, err
	}
	switch result {
	case -6, -5:
		return cacheQuotaMiss, ErrQuotaBalanceMutationUnknown
	case -4:
		return cacheQuotaMiss, ErrQuotaBalanceMutationUnknown
	case -3:
		return cacheQuotaMiss, ErrQuotaWriterEpochMismatch
	case -2:
		return cacheQuotaMiss, errInvalidQuotaCacheOperation
	case 1:
		return cacheQuotaOK, nil
	case 2:
		return cacheQuotaOK, nil
	case 0:
		return cacheQuotaInsufficient, nil
	default:
		return cacheQuotaMiss, nil
	}
}

func quotaBalanceJournalRedisKey(generationKey string) string {
	return "quota:balance:journal:" + generationKey
}

func quotaJournalSubjectLock(kind, id int, provided []string) (*quotaBalanceSubjectLock, string, error) {
	if len(provided) > 0 && provided[0] != "" {
		key, err := quotaBalanceSubjectLockKey(kind, id)
		if err != nil {
			return nil, "", err
		}
		return &quotaBalanceSubjectLock{Kind: kind, ID: id, Key: key, Token: provided[0]}, provided[0], nil
	}
	lock, err := acquireQuotaBalanceSubjectLock(kind, id)
	if err != nil {
		return nil, "", err
	}
	return lock, lock.Token, nil
}

func cacheTryReserveUserQuotaJournaled(userID int, amount int64, generationKey string, lockToken ...string) (cacheQuotaResult, error) {
	lock, token, err := quotaJournalSubjectLock(BatchUpdateTypeUserQuota, userID, lockToken)
	if err != nil {
		return cacheQuotaMiss, err
	}
	if len(lockToken) == 0 {
		defer releaseQuotaBalanceSubjectLock(lock)
	}
	epoch, err := quotaWriterEpochForLegacyCache(DB)
	if err != nil {
		return cacheQuotaMiss, err
	}
	result, err := common.RDB.Eval(context.Background(), userQuotaReserveJournaledScript,
		[]string{getUserCacheKey(userID), quotaWriterEpochRedisKey, quotaBalanceJournalRedisKey(generationKey), lock.Key}, amount, userID, userCacheSchemaVersion, epoch, token).Int()
	return quotaResultFromLua(result, err)
}

func cacheApplyUserQuotaDeltaJournaled(userID int, delta int64, generationKey string, lockToken ...string) (cacheQuotaResult, error) {
	lock, token, err := quotaJournalSubjectLock(BatchUpdateTypeUserQuota, userID, lockToken)
	if err != nil {
		return cacheQuotaMiss, err
	}
	if len(lockToken) == 0 {
		defer releaseQuotaBalanceSubjectLock(lock)
	}
	epoch, err := quotaWriterEpochForLegacyCache(DB)
	if err != nil {
		return cacheQuotaMiss, err
	}
	result, err := common.RDB.Eval(context.Background(), userQuotaDeltaJournaledScript,
		[]string{getUserCacheKey(userID), quotaWriterEpochRedisKey, quotaBalanceJournalRedisKey(generationKey), lock.Key}, delta, userID, userCacheSchemaVersion, epoch, token).Int()
	return quotaResultFromLua(result, err)
}

func cacheTryReserveTokenQuotaJournaled(id int, key string, amount int64, generationKey string, lockToken ...string) (cacheQuotaResult, error) {
	lock, token, err := quotaJournalSubjectLock(BatchUpdateTypeTokenQuota, id, lockToken)
	if err != nil {
		return cacheQuotaMiss, err
	}
	if len(lockToken) == 0 {
		defer releaseQuotaBalanceSubjectLock(lock)
	}
	epoch, err := quotaWriterEpochForLegacyCache(DB)
	if err != nil {
		return cacheQuotaMiss, err
	}
	result, err := common.RDB.Eval(context.Background(), tokenQuotaReserveJournaledScript,
		[]string{getTokenCacheKey(key), quotaWriterEpochRedisKey, quotaBalanceJournalRedisKey(generationKey), lock.Key}, amount, id, common.GetTimestamp(), epoch, token).Int()
	return quotaResultFromLua(result, err)
}

func cacheApplyTokenQuotaDeltaJournaled(id int, key string, delta int64, generationKey string, lockToken ...string) (cacheQuotaResult, error) {
	lock, token, err := quotaJournalSubjectLock(BatchUpdateTypeTokenQuota, id, lockToken)
	if err != nil {
		return cacheQuotaMiss, err
	}
	if len(lockToken) == 0 {
		defer releaseQuotaBalanceSubjectLock(lock)
	}
	epoch, err := quotaWriterEpochForLegacyCache(DB)
	if err != nil {
		return cacheQuotaMiss, err
	}
	result, err := common.RDB.Eval(context.Background(), tokenQuotaDeltaJournaledScript,
		[]string{getTokenCacheKey(key), quotaWriterEpochRedisKey, quotaBalanceJournalRedisKey(generationKey), lock.Key}, delta, id, common.GetTimestamp(), epoch, token).Int()
	return quotaResultFromLua(result, err)
}

func compensateUserQuotaDeltaJournaled(userID int, delta int64, generationKey string, lockToken ...string) (cacheQuotaResult, error) {
	lock, token, err := quotaJournalSubjectLock(BatchUpdateTypeUserQuota, userID, lockToken)
	if err != nil {
		return cacheQuotaMiss, err
	}
	if len(lockToken) == 0 {
		defer releaseQuotaBalanceSubjectLock(lock)
	}
	epoch, err := quotaWriterEpochForLegacyCache(DB)
	if err != nil {
		return cacheQuotaMiss, err
	}
	result, err := common.RDB.Eval(context.Background(), userQuotaCompensationScript,
		[]string{getUserCacheKey(userID), quotaWriterEpochRedisKey, quotaBalanceJournalRedisKey(generationKey), lock.Key}, delta, userID, userCacheSchemaVersion, epoch, token).Int()
	return quotaResultFromLua(result, err)
}

func compensateTokenQuotaDeltaJournaled(id int, key string, delta int64, generationKey string, lockToken ...string) (cacheQuotaResult, error) {
	lock, token, err := quotaJournalSubjectLock(BatchUpdateTypeTokenQuota, id, lockToken)
	if err != nil {
		return cacheQuotaMiss, err
	}
	if len(lockToken) == 0 {
		defer releaseQuotaBalanceSubjectLock(lock)
	}
	epoch, err := quotaWriterEpochForLegacyCache(DB)
	if err != nil {
		return cacheQuotaMiss, err
	}
	result, err := common.RDB.Eval(context.Background(), tokenQuotaCompensationScript,
		[]string{getTokenCacheKey(key), quotaWriterEpochRedisKey, quotaBalanceJournalRedisKey(generationKey), lock.Key}, delta, id, common.GetTimestamp(), epoch, token).Int()
	return quotaResultFromLua(result, err)
}

func cacheTryReserveUserQuota(userID int, amount int64) (cacheQuotaResult, error) {
	if amount < 0 || amount > common.MaxQuota {
		return cacheQuotaMiss, errInvalidQuotaCacheOperation
	}
	epoch, err := quotaWriterEpochForLegacyCache(DB)
	if err != nil {
		_ = invalidateUserCache(userID)
		return cacheQuotaMiss, err
	}
	result, err := common.RDB.Eval(context.Background(), userQuotaReserveScript,
		[]string{getUserCacheKey(userID), quotaWriterEpochRedisKey}, amount, userID, userCacheSchemaVersion, epoch).Int()
	parsed, parseErr := quotaResultFromLua(result, err)
	if errors.Is(parseErr, ErrQuotaWriterEpochMismatch) {
		_ = invalidateUserCache(userID)
	}
	return parsed, parseErr
}

func cacheApplyUserQuotaDelta(userID int, delta int64) (cacheQuotaResult, error) {
	if delta < common.MinQuota || delta > common.MaxQuota {
		return cacheQuotaMiss, errInvalidQuotaCacheOperation
	}
	epoch, err := quotaWriterEpochForLegacyCache(DB)
	if err != nil {
		_ = invalidateUserCache(userID)
		return cacheQuotaMiss, err
	}
	result, err := common.RDB.Eval(context.Background(), userQuotaDeltaScript,
		[]string{getUserCacheKey(userID), quotaWriterEpochRedisKey}, delta, userID, userCacheSchemaVersion, epoch).Int()
	parsed, parseErr := quotaResultFromLua(result, err)
	if errors.Is(parseErr, ErrQuotaWriterEpochMismatch) {
		_ = invalidateUserCache(userID)
	}
	return parsed, parseErr
}

func cacheTryReserveTokenQuota(id int, key string, amount int64) (cacheQuotaResult, error) {
	if amount < 0 || amount > common.MaxQuota {
		return cacheQuotaMiss, errInvalidQuotaCacheOperation
	}
	epoch, err := quotaWriterEpochForLegacyCache(DB)
	if err != nil {
		_ = invalidateTokenCacheForMutation(key)
		return cacheQuotaMiss, err
	}
	result, err := common.RDB.Eval(context.Background(), tokenQuotaReserveScript,
		[]string{getTokenCacheKey(key), quotaWriterEpochRedisKey}, amount, id, common.GetTimestamp(), epoch).Int()
	parsed, parseErr := quotaResultFromLua(result, err)
	if errors.Is(parseErr, ErrQuotaWriterEpochMismatch) {
		_ = invalidateTokenCacheForMutation(key)
	}
	return parsed, parseErr
}

func cacheApplyTokenQuotaDelta(id int, key string, delta int64) (cacheQuotaResult, error) {
	if delta < common.MinQuota || delta > common.MaxQuota {
		return cacheQuotaMiss, errInvalidQuotaCacheOperation
	}
	epoch, err := quotaWriterEpochForLegacyCache(DB)
	if err != nil {
		_ = invalidateTokenCacheForMutation(key)
		return cacheQuotaMiss, err
	}
	result, err := common.RDB.Eval(context.Background(), tokenQuotaDeltaScript,
		[]string{getTokenCacheKey(key), quotaWriterEpochRedisKey}, delta, id, common.GetTimestamp(), epoch).Int()
	parsed, parseErr := quotaResultFromLua(result, err)
	if errors.Is(parseErr, ErrQuotaWriterEpochMismatch) {
		_ = invalidateTokenCacheForMutation(key)
	}
	return parsed, parseErr
}

// persistUserQuotaDelta 把已在缓存侧预扣成功的增量落库；批量模式下入队，
// 直写模式下要求行存在（用户已删除时报错，交由调用方补偿缓存）。
func persistUserQuotaDelta(id int, delta int) error {
	if common.BatchUpdateEnabled {
		return errors.New("batch user quota persistence requires a pre-created durable generation")
	}
	result := DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", delta))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func persistTokenQuotaDelta(id int, delta int) error {
	if common.BatchUpdateEnabled {
		return errors.New("batch token quota persistence requires a pre-created durable generation")
	}
	result := DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota + ?", delta),
			"used_quota":    gorm.Expr("used_quota - ?", delta),
			"accessed_time": common.GetTimestamp(),
		},
	)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func reserveUserQuotaDB(id int, quota int) (bool, error) {
	result := DB.Model(&User{}).
		Where("id = ? AND quota >= ?", id, quota).
		Update("quota", gorm.Expr("quota - ?", quota))
	return result.RowsAffected == 1, result.Error
}

func reserveTokenQuotaDB(id int, quota int) (bool, error) {
	result := DB.Model(&Token{}).
		Where("id = ? AND remain_quota >= ?", id, quota).
		Updates(map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota - ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": common.GetTimestamp(),
		})
	return result.RowsAffected == 1, result.Error
}

func TryReserveUserQuota(id int, quota int) (bool, error) {
	if err := requireLegacyQuotaWriterCall(); err != nil {
		return false, err
	}
	return tryReserveUserQuotaLegacy(id, quota)
}

func TryReserveTokenQuota(id int, key string, quota int, unlimited bool) (bool, error) {
	if err := requireLegacyQuotaWriterCall(); err != nil {
		return false, err
	}
	return tryReserveTokenQuotaLegacy(id, key, quota, unlimited)
}

// TryReserveUserQuota atomically checks and deducts a user's wallet quota.
// 缓存命中时以缓存余额为准（避免批量模式下过期的数据库余额放大并发超扣）；
// Redis 异常或水合失败时降级为数据库条件更新，保证服务可用。
func tryReserveUserQuotaLegacy(id int, quota int) (bool, error) {
	if quota < 0 {
		return false, errors.New("quota 不能为负数！")
	}
	if quota > common.MaxQuota {
		return false, errInvalidQuotaCacheOperation
	}
	if quota == 0 {
		return true, nil
	}
	if !common.RedisEnabled {
		if common.BatchUpdateEnabled {
			return false, ErrBatchQuotaCacheUnavailable
		}
		return reserveUserQuotaDB(id, quota)
	}

	if common.BatchUpdateEnabled {
		result, err := applyLegacyBalanceCacheMutation(BatchUpdateTypeUserQuota, id, -quota, "",
			func(generationKey, lockToken string) (cacheQuotaResult, error) {
				return cacheTryReserveUserQuotaJournaled(id, int64(quota), generationKey, lockToken)
			},
			func(generationKey, lockToken string) (cacheQuotaResult, error) {
				return cacheApplyUserQuotaDeltaJournaled(id, int64(quota), generationKey, lockToken)
			})
		if err != nil || result == cacheQuotaMiss {
			if errors.Is(err, ErrQuotaBalanceMutationUnknown) || errors.Is(err, ErrQuotaBalanceSubjectBusy) {
				return false, err
			}
			return false, fmt.Errorf("%w: user cache reserve: %v", ErrBatchQuotaCacheUnavailable, err)
		}
		return result == cacheQuotaOK, nil
	}

	result, err := cacheTryReserveUserQuota(id, int64(quota))
	if errors.Is(err, ErrLegacyQuotaWriterModeDisabled) {
		return false, err
	}
	if err == nil && result == cacheQuotaMiss {
		if _, hydrateErr := GetUserCache(id); hydrateErr == nil {
			result, err = cacheTryReserveUserQuota(id, int64(quota))
		}
	}
	if errors.Is(err, errInvalidQuotaCacheOperation) {
		return false, err
	}
	if errors.Is(err, ErrLegacyQuotaWriterModeDisabled) {
		return false, err
	}
	if err != nil || result == cacheQuotaMiss {
		if err != nil {
			common.SysLog("user quota cache reserve unavailable, falling back to database: " + err.Error())
		}
		return reserveUserQuotaDB(id, quota)
	}
	if result == cacheQuotaInsufficient {
		return false, nil
	}
	if err = persistUserQuotaDelta(id, -quota); err != nil {
		compensated, compensateErr := cacheApplyUserQuotaDelta(id, int64(quota))
		if compensateErr != nil || compensated != cacheQuotaOK {
			common.SysError(fmt.Sprintf("failed to compensate reserved user quota: result=%d error=%v", compensated, compensateErr))
		}
		return false, err
	}
	return true, nil
}

// TryReserveTokenQuota atomically checks and deducts a token quota. Unlimited
// tokens skip the balance check but still update remain/used accounting.
func tryReserveTokenQuotaLegacy(id int, key string, quota int, unlimited bool) (bool, error) {
	if quota < 0 {
		return false, errors.New("quota 不能为负数！")
	}
	if quota > common.MaxQuota {
		return false, errInvalidQuotaCacheOperation
	}
	if quota == 0 {
		return true, nil
	}
	if !common.RedisEnabled {
		if common.BatchUpdateEnabled {
			return false, ErrBatchQuotaCacheUnavailable
		}
		if unlimited {
			return true, decreaseTokenQuota(id, quota)
		}
		return reserveTokenQuotaDB(id, quota)
	}

	if unlimited {
		if common.BatchUpdateEnabled {
			result, err := applyLegacyBalanceCacheMutation(BatchUpdateTypeTokenQuota, id, -quota, key,
				func(generationKey, lockToken string) (cacheQuotaResult, error) {
					return cacheApplyTokenQuotaDeltaJournaled(id, key, -int64(quota), generationKey, lockToken)
				},
				func(generationKey, lockToken string) (cacheQuotaResult, error) {
					return compensateTokenQuotaDeltaJournaled(id, key, int64(quota), generationKey, lockToken)
				})
			if err != nil || result != cacheQuotaOK {
				if errors.Is(err, ErrQuotaBalanceMutationUnknown) || errors.Is(err, ErrQuotaBalanceSubjectBusy) {
					return false, err
				}
				return false, fmt.Errorf("%w: unlimited token cache reserve: result=%d error=%v", ErrBatchQuotaCacheUnavailable, result, err)
			}
			return true, nil
		}
		result, err := cacheApplyTokenQuotaDelta(id, key, int64(-quota))
		if errors.Is(err, ErrLegacyQuotaWriterModeDisabled) {
			return false, err
		}
		if err != nil || result == cacheQuotaMiss {
			return true, decreaseTokenQuota(id, quota)
		}
		if err = persistTokenQuotaDelta(id, -quota); err != nil {
			compensated, compensateErr := cacheApplyTokenQuotaDelta(id, key, int64(quota))
			if compensateErr != nil || compensated != cacheQuotaOK {
				common.SysError(fmt.Sprintf("failed to compensate reserved unlimited token quota: result=%d error=%v", compensated, compensateErr))
			}
			return false, err
		}
		return true, nil
	}

	if common.BatchUpdateEnabled {
		result, err := applyLegacyBalanceCacheMutation(BatchUpdateTypeTokenQuota, id, -quota, key,
			func(generationKey, lockToken string) (cacheQuotaResult, error) {
				return cacheTryReserveTokenQuotaJournaled(id, key, int64(quota), generationKey, lockToken)
			},
			func(generationKey, lockToken string) (cacheQuotaResult, error) {
				return compensateTokenQuotaDeltaJournaled(id, key, int64(quota), generationKey, lockToken)
			})
		if err != nil || result == cacheQuotaMiss {
			if errors.Is(err, ErrQuotaBalanceMutationUnknown) || errors.Is(err, ErrQuotaBalanceSubjectBusy) {
				return false, err
			}
			return false, fmt.Errorf("%w: token cache reserve: %v", ErrBatchQuotaCacheUnavailable, err)
		}
		return result == cacheQuotaOK, nil
	}

	result, err := cacheTryReserveTokenQuota(id, key, int64(quota))
	if errors.Is(err, ErrLegacyQuotaWriterModeDisabled) {
		return false, err
	}
	if err == nil && result == cacheQuotaMiss {
		if _, hydrateErr := GetTokenByKey(key, true); hydrateErr == nil {
			result, err = cacheTryReserveTokenQuota(id, key, int64(quota))
		}
	}
	if errors.Is(err, errInvalidQuotaCacheOperation) {
		return false, err
	}
	if errors.Is(err, ErrLegacyQuotaWriterModeDisabled) {
		return false, err
	}
	if err != nil || result == cacheQuotaMiss {
		if err != nil {
			common.SysLog("token quota cache reserve unavailable, falling back to database: " + err.Error())
		}
		return reserveTokenQuotaDB(id, quota)
	}
	if result == cacheQuotaInsufficient {
		return false, nil
	}
	if err = persistTokenQuotaDelta(id, -quota); err != nil {
		compensated, compensateErr := cacheApplyTokenQuotaDelta(id, key, int64(quota))
		if compensateErr != nil || compensated != cacheQuotaOK {
			common.SysError(fmt.Sprintf("failed to compensate reserved token quota: result=%d error=%v", compensated, compensateErr))
		}
		return false, err
	}
	return true, nil
}
