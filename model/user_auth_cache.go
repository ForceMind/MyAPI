package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/ForceMind/MyAPI/common"

	"gorm.io/gorm"
)

// User auth cache fencing uses three Redis keys per user: the cached user
// hash, a short-lived pending fence published before a restrictive database
// transaction, and a monotonic committed version floor published after
// commit. Cache writes below either floor are rejected, readers below the
// effective floor fall back to the database, and the pending fence outlives
// every user-hash TTL so a rolled-back transaction heals without allowing a
// stale snapshot to re-authorize the user.

var ErrUserAuthCachePending = errors.New("user authentication state update is pending")

var ErrUserAuthVersionConflict = errors.New("user authentication version update conflicted")

// Redis Lua numbers cannot distinguish adjacent int64 values above 2^53.
// Versions and epochs therefore remain canonical decimal strings and are
// ordered by length and lexicographic value.
const redisCanonicalVersionPrelude = `
local versionMaximum = '9223372036854775807'
local function canonicalNonnegativeInteger(value)
  if type(value) ~= 'string' or (value ~= '0' and string.match(value, '^[1-9]%d*$') == nil) then
    return nil
  end
  if #value > #versionMaximum or (#value == #versionMaximum and value > versionMaximum) then
    return nil
  end
  return value
end
local function canonicalPositiveInteger(value)
  local canonical = canonicalNonnegativeInteger(value)
  if canonical == nil or canonical == '0' then return nil end
  return canonical
end
local function compareCanonicalNonnegative(left, right)
  if #left < #right then return -1 end
  if #left > #right then return 1 end
  if left < right then return -1 end
  if left > right then return 1 end
  return 0
end
`

func getUserAuthFenceKey(userId int) string {
	return fmt.Sprintf("auth:user:fence:%d", userId)
}

func getUserAuthVersionKey(userId int) string {
	return fmt.Sprintf("auth:user:version:%d", userId)
}

// A pending fence only covers the interval between publishing the next
// version and the surrounding database transaction reaching a decision. Its
// TTL must outlive every user hash that could have been populated before the
// fence, while still allowing an automatically rolled-back transaction to
// recover without an operator repairing Redis.
func userAuthFenceTTLSeconds() int {
	cacheTTL := userCacheTTLSeconds()
	extra := cacheTTL
	if extra < 60 {
		extra = 60
	}
	return cacheTTL + extra
}

func writeUserCache(user *UserBase, includeQuota bool) error {
	return writeUserCacheWithQuotaBalanceOwner(user, includeQuota, nil)
}

func writeUserCacheWithQuotaBalanceOwner(user *UserBase, includeQuota bool, balanceLock *quotaBalanceSubjectLock) error {
	if user == nil || user.Id <= 0 || !common.RedisEnabled {
		return nil
	}
	state, err := GetQuotaWriterEpochState(DB)
	if err != nil {
		return err
	}
	user.CacheSchema = userCacheSchemaVersion
	user.QuotaWriterEpoch = state.Epoch
	if user.AuthVersion <= 0 {
		return fmt.Errorf("invalid user auth version")
	}
	if includeQuota {
		if _, err := common.CheckedQuotaBalance(user.Quota, 0); err != nil {
			return err
		}
		if user.QuotaVersion < 0 {
			return ErrQuotaWriterEpochUnavailable
		}
	}
	includeQuotaArg := "0"
	if includeQuota {
		includeQuotaArg = "1"
	}
	ttl := userCacheTTLSeconds()
	balanceLockKey, err := quotaBalanceSubjectLockKey(BatchUpdateTypeUserQuota, user.Id)
	if err != nil {
		return err
	}
	balanceLockToken := ""
	if balanceLock != nil {
		balanceLockToken = balanceLock.Token
	}
	script := redisExactQuotaPrelude + `
local balanceOwner = redis.call('GET', KEYS[5])
local ownerSupplied = ARGV[16] ~= ''
local ownerPresent = balanceOwner ~= false
if ownerSupplied then
  if not ownerPresent or balanceOwner ~= ARGV[16] then return 3 end
elseif ownerPresent then
  return 3
end
local hasBalanceOwner = ownerSupplied
local incoming = canonicalPositiveInteger(ARGV[1])
local subjectID = canonicalPositiveInteger(ARGV[2])
local incomingEpoch = canonicalPositiveInteger(ARGV[14])
local incomingQV = canonicalNonnegativeInteger(ARGV[13])
local cacheSchema = canonicalPositiveInteger(ARGV[10])
local cacheTTL = canonicalPositiveInteger(ARGV[15])
local globalEpoch = canonicalNonnegativeInteger(redis.call('GET', KEYS[4]) or '0')
local pending = canonicalNonnegativeInteger(redis.call('GET', KEYS[2]) or '0')
local committed = canonicalNonnegativeInteger(redis.call('GET', KEYS[3]) or '0')
local current = canonicalNonnegativeInteger(redis.call('HGET', KEYS[1], 'AuthVersion') or '0')
if incoming == nil or subjectID == nil or incomingEpoch == nil or incomingQV == nil or cacheSchema == nil or cacheTTL == nil
  or (ARGV[11] ~= '0' and ARGV[11] ~= '1') or globalEpoch == nil
  or pending == nil or committed == nil or current == nil then
  return redis.error_reply('invalid cache version')
end
local incomingQuota = nil
if ARGV[11] == '1' then
  incomingQuota = quotaStateInteger(ARGV[12], false)
  if incomingQuota == nil then return redis.error_reply('invalid quota state') end
end
local globalOrder = compareCanonicalNonnegative(globalEpoch, incomingEpoch)
if globalOrder > 0 then
  return 2
end
if compareCanonicalNonnegative(pending, incoming) > 0
  or compareCanonicalNonnegative(committed, incoming) > 0
  or compareCanonicalNonnegative(current, incoming) > 0 then
  return 0
end
local exists = redis.call('EXISTS', KEYS[1]) == 1
if exists and redis.call('HGET', KEYS[1], 'Id') ~= ARGV[2] then
  return 4
end
local replaceCache = exists and redis.call('HGET', KEYS[1], 'CacheSchema') ~= ARGV[10]
local writeQuota = false
local currentEpoch = nil
local currentQV = nil
if exists and not replaceCache then
  local currentQuota = quotaStateInteger(redis.call('HGET', KEYS[1], 'Quota'), false)
  currentEpoch = canonicalPositiveInteger(redis.call('HGET', KEYS[1], 'QuotaWriterEpoch'))
  currentQV = canonicalNonnegativeInteger(redis.call('HGET', KEYS[1], 'QuotaVersion'))
  if currentQuota == nil or currentEpoch == nil or currentQV == nil then
    return redis.error_reply('invalid cached quota state')
  end
end
if hasBalanceOwner and ARGV[11] == '1' then
  if not exists or replaceCache then
    writeQuota = true
  else
    local epochOrder = compareCanonicalNonnegative(incomingEpoch, currentEpoch)
    writeQuota = epochOrder > 0 or (epochOrder == 0 and compareCanonicalNonnegative(incomingQV, currentQV) > 0)
  end
end
-- A metadata reader cannot certify accounting state that has no expiry.
-- Leave it cold so the subject owner can recover pending generations.
if exists and not hasBalanceOwner and redis.call('PTTL', KEYS[1]) <= 0 then
  replaceCache = true
end
if globalOrder < 0 then
  redis.call('SET', KEYS[4], ARGV[14])
end
if compareCanonicalNonnegative(committed, incoming) < 0 then
  redis.call('SET', KEYS[3], ARGV[1])
end
if pending ~= '0' and compareCanonicalNonnegative(pending, incoming) <= 0 then
  redis.call('DEL', KEYS[2])
end
if replaceCache then
  redis.call('DEL', KEYS[1])
end
if redis.call('EXISTS', KEYS[1]) == 0 and (not hasBalanceOwner or ARGV[11] ~= '1') then
  return 1
end
redis.call('HSET', KEYS[1],
  'Id', ARGV[2], 'Group', ARGV[3], 'AccountTierID', ARGV[4], 'Email', ARGV[5],
  'Status', ARGV[6], 'Role', ARGV[7], 'Username', ARGV[8],
  'Setting', ARGV[9], 'AuthVersion', ARGV[1], 'CacheSchema', ARGV[10])
if writeQuota then
  redis.call('HSET', KEYS[1], 'Quota', ARGV[12], 'QuotaVersion', ARGV[13], 'QuotaWriterEpoch', ARGV[14])
end
redis.call('EXPIRE', KEYS[1], ARGV[15])
return 1`
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := common.RDB.Eval(ctx, script,
		[]string{getUserCacheKey(user.Id), getUserAuthFenceKey(user.Id), getUserAuthVersionKey(user.Id), quotaWriterEpochRedisKey, balanceLockKey},
		user.AuthVersion, user.Id, user.Group, user.AccountTierID, user.Email, user.Status, user.Role,
		user.Username, user.Setting, user.CacheSchema, includeQuotaArg, user.Quota, user.QuotaVersion, user.QuotaWriterEpoch, ttl, balanceLockToken,
	).Int()
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrUserAuthCachePending
	}
	if result == 2 {
		return ErrQuotaWriterEpochMismatch
	}
	if result == 3 {
		return ErrQuotaBalanceMutationUnknown
	}
	if result == 4 {
		return ErrQuotaCacheIdentityMismatch
	}
	return nil
}

var hydrateUserQuotaCacheScript = redisExactQuotaPrelude + `
local incomingQuota = quotaStateInteger(ARGV[3], false)
local incomingQV = canonicalNonnegativeInteger(ARGV[4])
local incomingEpoch = canonicalPositiveInteger(ARGV[5])
local subjectID = canonicalPositiveInteger(ARGV[1])
local cacheSchema = canonicalPositiveInteger(ARGV[2])
local cacheTTL = canonicalPositiveInteger(ARGV[6])
if incomingQuota == nil or incomingQV == nil or incomingEpoch == nil or subjectID == nil
  or cacheSchema == nil or cacheTTL == nil then
  return redis.error_reply('invalid quota cache state')
end
local globalEpoch = canonicalNonnegativeInteger(redis.call('GET', KEYS[2]) or '0')
if globalEpoch == nil then return redis.error_reply('invalid quota writer epoch') end
if redis.call('EXISTS', KEYS[1]) == 0 then
  return 2
end
if redis.call('HGET', KEYS[1], 'Id') ~= ARGV[1] then
  return 3
end
if redis.call('HGET', KEYS[1], 'CacheSchema') ~= ARGV[2] then
  redis.call('DEL', KEYS[1])
  return 2
end
local currentEpoch = canonicalPositiveInteger(redis.call('HGET', KEYS[1], 'QuotaWriterEpoch'))
local currentQV = canonicalNonnegativeInteger(redis.call('HGET', KEYS[1], 'QuotaVersion'))
local currentQuota = quotaStateInteger(redis.call('HGET', KEYS[1], 'Quota'), false)
if currentEpoch == nil or currentQV == nil or currentQuota == nil then
  return redis.error_reply('invalid quota cache state')
end
local globalOrder = compareCanonicalNonnegative(globalEpoch, incomingEpoch)
if globalOrder > 0 then
  return 0
end
local epochOrder = compareCanonicalNonnegative(incomingEpoch, currentEpoch)
if epochOrder < 0 or (epochOrder == 0 and compareCanonicalNonnegative(incomingQV, currentQV) <= 0) then
  redis.call('EXPIRE', KEYS[1], ARGV[6])
  return 0
end
if globalOrder < 0 then
  redis.call('SET', KEYS[2], ARGV[5])
end
redis.call('HSET', KEYS[1], 'Quota', ARGV[3], 'QuotaVersion', ARGV[4], 'QuotaWriterEpoch', ARGV[5])
redis.call('EXPIRE', KEYS[1], ARGV[6])
return 1`

func hydrateUserQuotaCacheRedisAtEpoch(userId int, quota int64, quotaVersion, writerEpoch int64) error {
	if !common.RedisEnabled || userId <= 0 {
		return nil
	}
	if common.RDB == nil || writerEpoch <= 0 || quotaVersion < 0 {
		return ErrQuotaWriterEpochUnavailable
	}
	if _, err := common.CheckedQuotaBalance(quota, 0); err != nil {
		return err
	}
	ttl := userCacheTTLSeconds()
	result, err := common.RDB.Eval(context.Background(), hydrateUserQuotaCacheScript,
		[]string{getUserCacheKey(userId), quotaWriterEpochRedisKey},
		strconv.Itoa(userId), strconv.Itoa(userCacheSchemaVersion), strconv.FormatInt(quota, 10),
		strconv.FormatInt(quotaVersion, 10), strconv.FormatInt(writerEpoch, 10), ttl,
	).Int()
	if err == nil && result == 3 {
		return ErrQuotaCacheIdentityMismatch
	}
	return err
}

func getUserAuthVersionFloor(userId int) (int64, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	values, err := common.RDB.MGet(context.Background(), getUserAuthFenceKey(userId), getUserAuthVersionKey(userId)).Result()
	if err != nil {
		return 0, err
	}
	var floor int64
	for _, value := range values {
		if value == nil {
			continue
		}
		parsed, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
		if err != nil {
			return 0, err
		}
		if parsed > floor {
			floor = parsed
		}
	}
	return floor, nil
}

// SetUserAuthVersionFence publishes a fail-closed version before a restrictive
// database update. Pending fences expire only after every pre-existing user
// hash must have expired; a committed update is promoted separately to a
// permanent monotonic version floor.
func SetUserAuthVersionFence(userId int, authVersion int64) error {
	if !common.RedisEnabled {
		return nil
	}
	if userId <= 0 || authVersion <= 0 {
		return fmt.Errorf("invalid user auth fence")
	}
	const script = redisCanonicalVersionPrelude + `
local current = canonicalNonnegativeInteger(redis.call('GET', KEYS[1]) or '0')
local incoming = canonicalPositiveInteger(ARGV[1])
if current == nil or incoming == nil then return redis.error_reply('invalid auth version') end
local versionOrder = compareCanonicalNonnegative(current, incoming)
if versionOrder < 0 then
  redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
elseif versionOrder == 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
elseif redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
return 1`
	return common.RDB.Eval(context.Background(), script, []string{getUserAuthFenceKey(userId)}, authVersion, userAuthFenceTTLSeconds()).Err()
}

// publishCommittedUserAuthVersion records the durable lower bound used to
// reject an arbitrarily delayed cache fill after a committed security change.
// It also removes this transaction's now-obsolete pending fence.
func publishCommittedUserAuthVersion(userId int, authVersion int64) error {
	if !common.RedisEnabled {
		return nil
	}
	if userId <= 0 || authVersion <= 0 {
		return fmt.Errorf("invalid committed user auth version")
	}
	const script = redisCanonicalVersionPrelude + `
local incoming = canonicalPositiveInteger(ARGV[1])
local committed = canonicalNonnegativeInteger(redis.call('GET', KEYS[1]) or '0')
local pending = canonicalNonnegativeInteger(redis.call('GET', KEYS[2]) or '0')
if incoming == nil or committed == nil or pending == nil then return redis.error_reply('invalid auth version') end
if compareCanonicalNonnegative(committed, incoming) < 0 then
  redis.call('SET', KEYS[1], ARGV[1])
end
if pending ~= '0' and compareCanonicalNonnegative(pending, incoming) <= 0 then
  redis.call('DEL', KEYS[2])
end
return 1`
	return common.RDB.Eval(context.Background(), script,
		[]string{getUserAuthVersionKey(userId), getUserAuthFenceKey(userId)}, authVersion,
	).Err()
}

// IncrementUserAuthVersionWithTx locks the user, publishes the next deny
// fence, then persists the version in the caller's transaction. Unscoped is
// intentional so the same fail-closed path also covers hard deletion of an
// already soft-deleted user.
func IncrementUserAuthVersionWithTx(tx *gorm.DB, userId int) (int64, error) {
	if tx == nil || userId <= 0 {
		return 0, fmt.Errorf("invalid user auth version update")
	}
	for range 3 {
		var user User
		if err := lockForUpdate(tx.Unscoped()).Select("id", "auth_version").Where("id = ?", userId).First(&user).Error; err != nil {
			return 0, err
		}
		current := user.AuthVersion
		if current < 1 {
			current = 1
		}
		if current == math.MaxInt64 {
			return 0, ErrUserAuthVersionConflict
		}
		next := current + 1
		if err := SetUserAuthVersionFence(userId, next); err != nil {
			return 0, err
		}
		result := tx.Unscoped().Model(&User{}).
			Where("id = ? AND auth_version = ?", userId, user.AuthVersion).
			Update("auth_version", next)
		if result.Error != nil {
			return 0, result.Error
		}
		if result.RowsAffected == 1 {
			return next, nil
		}
	}
	return 0, ErrUserAuthVersionConflict
}

// BumpUserAuthVersion is the transaction-owning variant used by password,
// role, status and security-factor changes outside another transaction.
func BumpUserAuthVersion(userId int) (int64, error) {
	var next int64
	if err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		next, err = IncrementUserAuthVersionWithTx(tx, userId)
		return err
	}); err != nil {
		return 0, err
	}
	if err := PublishUserAuthCache(userId); err != nil {
		return next, err
	}
	return next, nil
}

// PublishUserAuthCache refreshes the current database state after a successful
// auth-sensitive transaction without touching the cached quota field.
func PublishUserAuthCache(userId int) error {
	user, err := GetUserById(userId, false)
	if err != nil {
		return err
	}
	return updateUserCache(*user)
}

// InitializeUserAuthVersions must run after AutoMigrate when upgrading an
// existing database. It is idempotent and portable across all supported DBs.
func InitializeUserAuthVersions() error {
	return DB.Model(&User{}).Where("auth_version IS NULL OR auth_version < ?", 1).Update("auth_version", 1).Error
}

func updateUserCacheFieldAtVersion(userId int, field string, value interface{}, authVersion int64) error {
	if !common.RedisEnabled {
		return nil
	}
	if userId <= 0 || authVersion <= 0 {
		return fmt.Errorf("invalid user auth version")
	}
	script := redisExactQuotaPrelude + `
local incoming = canonicalPositiveInteger(ARGV[1])
local pending = canonicalNonnegativeInteger(redis.call('GET', KEYS[2]) or '0')
local committed = canonicalNonnegativeInteger(redis.call('GET', KEYS[3]) or '0')
local cacheSchema = canonicalPositiveInteger(ARGV[4])
local subjectID = canonicalPositiveInteger(ARGV[5])
if incoming == nil or pending == nil or committed == nil or cacheSchema == nil or subjectID == nil then
  return redis.error_reply('invalid auth version')
end
local exists = redis.call('EXISTS', KEYS[1]) == 1
if exists and redis.call('HGET', KEYS[1], 'Id') ~= ARGV[5] then
  return 2
end
if exists and redis.call('HGET', KEYS[1], 'CacheSchema') ~= ARGV[4] then
  redis.call('DEL', KEYS[1])
  return 3
end
local current = canonicalNonnegativeInteger(redis.call('HGET', KEYS[1], 'AuthVersion') or '0')
if current == nil then return redis.error_reply('invalid auth version') end
if exists then
  local currentQuota = quotaStateInteger(redis.call('HGET', KEYS[1], 'Quota'), false)
  local currentQV = canonicalNonnegativeInteger(redis.call('HGET', KEYS[1], 'QuotaVersion'))
  local currentEpoch = canonicalPositiveInteger(redis.call('HGET', KEYS[1], 'QuotaWriterEpoch'))
  if currentQuota == nil or currentQV == nil or currentEpoch == nil then
    return redis.error_reply('invalid cached quota state')
  end
end
if compareCanonicalNonnegative(pending, incoming) > 0
  or compareCanonicalNonnegative(committed, incoming) > 0
  or compareCanonicalNonnegative(current, incoming) > 0 then
  return 0
end
if compareCanonicalNonnegative(committed, incoming) < 0 then
  redis.call('SET', KEYS[3], ARGV[1])
end
if pending ~= '0' and compareCanonicalNonnegative(pending, incoming) <= 0 then
  redis.call('DEL', KEYS[2])
end
if redis.call('EXISTS', KEYS[1]) == 0 then
  return 1
end
if compareCanonicalNonnegative(current, incoming) ~= 0 then
  return 1
end
redis.call('HSET', KEYS[1], ARGV[2], ARGV[3], 'CacheSchema', ARGV[4])
return 1`
	result, err := common.RDB.Eval(context.Background(), script,
		[]string{getUserCacheKey(userId), getUserAuthFenceKey(userId), getUserAuthVersionKey(userId)},
		authVersion, field, value, userCacheSchemaVersion, userId,
	).Int()
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrUserAuthCachePending
	}
	if result == 2 {
		return ErrQuotaCacheIdentityMismatch
	}
	return nil
}
