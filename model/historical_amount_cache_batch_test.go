package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMaxQuotaBalance = int64(1<<53) - 1

func TestHistoricalAmountCacheAllowsSmallDeltaFromWideBalance(t *testing.T) {
	server := useUserCacheMiniRedis(t)
	ctx := context.Background()
	const (
		userID      = 9201
		writerEpoch = int64(7)
	)
	key := getUserCacheKey(userID)
	require.NoError(t, common.RDB.HSet(ctx, key, map[string]string{
		"Id":               strconv.Itoa(userID),
		"CacheSchema":      strconv.Itoa(userCacheSchemaVersion),
		"Quota":            strconv.FormatInt(testMaxQuotaBalance, 10),
		"QuotaWriterEpoch": strconv.FormatInt(writerEpoch, 10),
	}).Err())
	require.NoError(t, common.RDB.Set(ctx, quotaWriterEpochRedisKey, writerEpoch, 0).Err())

	result, err := common.RDB.Eval(ctx, userQuotaDeltaScript,
		[]string{key, quotaWriterEpochRedisKey}, -1, userID, userCacheSchemaVersion, writerEpoch).Int()
	status, err := quotaResultFromLua(result, err)
	require.NoError(t, err)
	assert.Equal(t, cacheQuotaOK, status)
	assert.Equal(t, strconv.FormatInt(testMaxQuotaBalance-1, 10), server.HGet(key, "Quota"))
}

func TestHistoricalAmountCacheRejectsUsedBoundaryBeforeAnyWrite(t *testing.T) {
	server := useUserCacheMiniRedis(t)
	ctx := context.Background()
	const (
		tokenID     = 9202
		writerEpoch = int64(11)
	)
	key := "historical-amount-token-boundary"
	before := map[string]string{
		"Id":               strconv.Itoa(tokenID),
		"CacheSchema":      strconv.Itoa(tokenCacheSchemaVersion),
		"RemainQuota":      "100",
		"UsedQuota":        strconv.FormatInt(testMaxQuotaBalance, 10),
		"AccessedTime":     "123",
		"QuotaWriterEpoch": strconv.FormatInt(writerEpoch, 10),
	}
	require.NoError(t, common.RDB.HSet(ctx, key, before).Err())

	result, evalErr := common.RDB.Eval(ctx, tokenQuotaReserveScript,
		[]string{key, quotaWriterEpochRedisKey}, 1, tokenID, 456, writerEpoch).Int()
	status, err := quotaResultFromLua(result, evalErr)
	assert.ErrorIs(t, err, errInvalidQuotaCacheOperation)
	assert.NotEqual(t, cacheQuotaOK, status)
	after, readErr := common.RDB.HGetAll(ctx, key).Result()
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
	assert.False(t, server.Exists(quotaWriterEpochRedisKey), "target validation must finish before the first Redis write")
}

func TestHistoricalAmountTokenDeltaRejectsUnrepresentableInverseBeforeAnyWrite(t *testing.T) {
	server := useUserCacheMiniRedis(t)
	ctx := context.Background()
	const (
		tokenID     = 9207
		writerEpoch = int64(13)
	)
	key := "historical-amount-token-minimum-delta"
	before := map[string]string{
		"Id":               strconv.Itoa(tokenID),
		"CacheSchema":      strconv.Itoa(tokenCacheSchemaVersion),
		"RemainQuota":      "5000000000",
		"UsedQuota":        "5000000000",
		"AccessedTime":     "123",
		"QuotaWriterEpoch": strconv.FormatInt(writerEpoch, 10),
	}
	require.NoError(t, common.RDB.HSet(ctx, key, before).Err())

	result, evalErr := common.RDB.Eval(ctx, tokenQuotaDeltaScript,
		[]string{key, quotaWriterEpochRedisKey}, common.MinQuota, tokenID, 456, writerEpoch).Int()
	status, err := quotaResultFromLua(result, evalErr)
	assert.ErrorIs(t, err, errInvalidQuotaCacheOperation)
	assert.NotEqual(t, cacheQuotaOK, status)
	after, readErr := common.RDB.HGetAll(ctx, key).Result()
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
	assert.False(t, server.Exists(quotaWriterEpochRedisKey))
}

func TestHistoricalAmountCacheOrdersAdjacentVersionsAboveDoublePrecision(t *testing.T) {
	server := useUserCacheMiniRedis(t)
	ctx := context.Background()
	const (
		userID        = 9203
		currentEpoch  = int64(1 << 53)
		incomingEpoch = currentEpoch + 1
	)
	key := getUserCacheKey(userID)
	require.NoError(t, common.RDB.HSet(ctx, key, map[string]string{
		"Id":               strconv.Itoa(userID),
		"CacheSchema":      strconv.Itoa(userCacheSchemaVersion),
		"Quota":            "100",
		"QuotaVersion":     "99",
		"QuotaWriterEpoch": strconv.FormatInt(currentEpoch, 10),
	}).Err())
	require.NoError(t, common.RDB.Set(ctx, quotaWriterEpochRedisKey, currentEpoch, 0).Err())
	require.NoError(t, common.RDB.Expire(ctx, key, time.Minute).Err())

	require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(userID, 200, 1, incomingEpoch))
	assert.Equal(t, "200", server.HGet(key, "Quota"))
	assert.Equal(t, "1", server.HGet(key, "QuotaVersion"))
	assert.Equal(t, strconv.FormatInt(incomingEpoch, 10), server.HGet(key, "QuotaWriterEpoch"))
	globalEpoch, err := server.Get(quotaWriterEpochRedisKey)
	require.NoError(t, err)
	assert.Equal(t, strconv.FormatInt(incomingEpoch, 10), globalEpoch)
}

func TestHistoricalAmountCacheOrdersAdjacentQuotaAndAuthVersionsAboveDoublePrecision(t *testing.T) {
	server := useUserCacheMiniRedis(t)
	ctx := context.Background()
	const (
		userID          = 9204
		tokenID         = 9205
		writerEpoch     = int64(17)
		currentVersion  = int64(1 << 53)
		incomingVersion = currentVersion + 1
	)
	require.NoError(t, common.RDB.Set(ctx, quotaWriterEpochRedisKey, writerEpoch, 0).Err())

	userKey := getUserCacheKey(userID)
	require.NoError(t, common.RDB.HSet(ctx, userKey, map[string]string{
		"Id": strconv.Itoa(userID), "CacheSchema": strconv.Itoa(userCacheSchemaVersion),
		"Quota": "100", "QuotaVersion": strconv.FormatInt(currentVersion, 10),
		"QuotaWriterEpoch": strconv.FormatInt(writerEpoch, 10),
	}).Err())
	require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(userID, 200, incomingVersion, writerEpoch))
	assert.Equal(t, "200", server.HGet(userKey, "Quota"))
	assert.Equal(t, strconv.FormatInt(incomingVersion, 10), server.HGet(userKey, "QuotaVersion"))

	tokenKey := "historical-adjacent-token"
	tokenCacheKey := getTokenCacheKey(tokenKey)
	require.NoError(t, common.RDB.HSet(ctx, tokenCacheKey, map[string]string{
		"Id": strconv.Itoa(tokenID), "CacheSchema": strconv.Itoa(tokenCacheSchemaVersion),
		"RemainQuota": "100", "UsedQuota": "10", "QuotaVersion": strconv.FormatInt(currentVersion, 10),
		"QuotaWriterEpoch": strconv.FormatInt(writerEpoch, 10),
	}).Err())
	require.NoError(t, hydrateTokenQuotaCacheRedisAtEpoch(tokenKey, tokenID, 90, 20, incomingVersion, writerEpoch))
	assert.Equal(t, "90", server.HGet(tokenCacheKey, "RemainQuota"))
	assert.Equal(t, "20", server.HGet(tokenCacheKey, "UsedQuota"))
	assert.Equal(t, strconv.FormatInt(incomingVersion, 10), server.HGet(tokenCacheKey, "QuotaVersion"))

	require.NoError(t, common.RDB.Set(ctx, getUserAuthFenceKey(userID), currentVersion, 0).Err())
	require.NoError(t, SetUserAuthVersionFence(userID, incomingVersion))
	authFence, err := server.Get(getUserAuthFenceKey(userID))
	require.NoError(t, err)
	assert.Equal(t, strconv.FormatInt(incomingVersion, 10), authFence)
}

func TestHistoricalAmountV3RowsRemainByteAndStateStableDuringInitialization(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "historical-v3-stability")
	type v3Payload struct {
		UserQuota      map[int]int    `json:"user_quota,omitempty"`
		TokenQuota     map[int]int    `json:"token_quota,omitempty"`
		TokenCacheKeys map[int]string `json:"token_cache_keys,omitempty"`
	}

	now := GetDBTimestamp()
	for index, state := range []string{
		quotaBalanceBatchDrainPending,
		quotaBalanceBatchDrainApplying,
		quotaBalanceBatchDrainFailed,
		quotaBalanceBatchDrainCompensating,
		quotaBalanceBatchDrainApplied,
		quotaBalanceBatchDrainCancelled,
	} {
		legacy := v3Payload{UserQuota: map[int]int{fixture.User.Id: -(index + 1)}}
		data, err := common.Marshal(legacy)
		require.NoError(t, err)
		digest := sha256.Sum256(data)
		fingerprint := hex.EncodeToString(digest[:])
		payload := QuotaBalanceBatchPayload{UserQuota: map[int]int64{fixture.User.Id: -int64(index + 1)}}
		row := QuotaBalanceBatchDrain{
			SchemaVersion: 3, GenerationKey: fmt.Sprintf("historical-v3-%d", index), WriterEpoch: 1,
			Payload: payload, PayloadFingerprint: fingerprint, CacheApplied: true, State: state,
			LockVersion: 1, CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, dbFixture.DB.Create(&row).Error)
		var before struct {
			SchemaVersion      int
			Payload            string
			PayloadFingerprint string
			State              string
		}
		require.NoError(t, dbFixture.DB.Table("quota_balance_batch_drains").Select("schema_version, payload, payload_fingerprint, state").Where("id = ?", row.ID).Scan(&before).Error)

		require.NoError(t, InitializeQuotaBalanceBatchDrainsWithDB(dbFixture.DB))

		var after struct {
			SchemaVersion      int
			Payload            string
			PayloadFingerprint string
			State              string
		}
		require.NoError(t, dbFixture.DB.Table("quota_balance_batch_drains").Select("schema_version, payload, payload_fingerprint, state").Where("id = ?", row.ID).Scan(&after).Error)
		assert.Equal(t, before, after)
	}
}

func TestHistoricalAmountV3FingerprintExecutesWithoutIdentityRewrite(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "historical-v3-replay")
	state, err := GetQuotaWriterEpochState(dbFixture.DB)
	require.NoError(t, err)
	type v3Payload struct {
		UserQuota      map[int]int    `json:"user_quota,omitempty"`
		TokenQuota     map[int]int    `json:"token_quota,omitempty"`
		TokenCacheKeys map[int]string `json:"token_cache_keys,omitempty"`
	}
	legacy := v3Payload{UserQuota: map[int]int{fixture.User.Id: -1}}
	data, err := common.Marshal(legacy)
	require.NoError(t, err)
	digest := sha256.Sum256(data)
	fingerprint := hex.EncodeToString(digest[:])
	generation := QuotaBalanceBatchDrain{
		SchemaVersion: quotaBalanceBatchDrainLegacySchemaVersion,
		GenerationKey: "historical-v3-replay", WriterEpoch: state.Epoch,
		Payload:            QuotaBalanceBatchPayload{UserQuota: map[int]int64{fixture.User.Id: -1}},
		PayloadFingerprint: fingerprint, CacheApplied: true, State: quotaBalanceBatchDrainPending,
		LockVersion: 1, CreatedAt: GetDBTimestamp(), UpdatedAt: GetDBTimestamp(),
	}
	require.NoError(t, dbFixture.DB.Create(&generation).Error)
	require.NoError(t, applyQuotaBalanceBatchGenerationWithDB(dbFixture.DB, generation.ID))
	require.NoError(t, applyQuotaBalanceBatchGenerationWithDB(dbFixture.DB, generation.ID))

	var stored QuotaBalanceBatchDrain
	require.NoError(t, dbFixture.DB.First(&stored, generation.ID).Error)
	assert.Equal(t, quotaBalanceBatchDrainLegacySchemaVersion, stored.SchemaVersion)
	assert.Equal(t, fingerprint, stored.PayloadFingerprint)
	assert.Equal(t, quotaBalanceBatchDrainApplied, stored.State)
	var user User
	require.NoError(t, dbFixture.DB.First(&user, fixture.User.Id).Error)
	assert.EqualValues(t, 999, user.Quota)
}

func TestHistoricalAmountV4WideAggregateReplaysOnce(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "historical-v4-wide")
	const (
		startingQuota = int64(5_000_000_000)
		wideDelta     = int64(-3_000_000_000)
	)
	require.NoError(t, dbFixture.DB.Model(&User{}).Where("id = ?", fixture.User.Id).Update("quota", startingQuota).Error)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUserQuota][fixture.User.Id] = wideDelta
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	require.NoError(t, persistQuotaBalanceBatchGeneration())

	var generation QuotaBalanceBatchDrain
	require.NoError(t, dbFixture.DB.Where("state = ?", quotaBalanceBatchDrainPending).First(&generation).Error)
	assert.Equal(t, quotaBalanceBatchDrainSchemaVersion, generation.SchemaVersion)
	assert.Equal(t, wideDelta, generation.Payload.UserQuota[fixture.User.Id])
	legacyData, err := common.Marshal(generation.Payload)
	require.NoError(t, err)
	legacyDigest := sha256.Sum256(legacyData)
	assert.NotEqual(t, hex.EncodeToString(legacyDigest[:]), generation.PayloadFingerprint, "v4 identity must include its schema/domain")
	assert.Empty(t, batchUpdateStores[BatchUpdateTypeUserQuota], "persisted aggregate must leave the in-memory queue exactly once")

	require.NoError(t, applyQuotaBalanceBatchGenerationWithDB(dbFixture.DB, generation.ID))
	require.NoError(t, applyQuotaBalanceBatchGenerationWithDB(dbFixture.DB, generation.ID))

	var user User
	require.NoError(t, dbFixture.DB.First(&user, fixture.User.Id).Error)
	assert.EqualValues(t, startingQuota+wideDelta, user.Quota)
	var stored QuotaBalanceBatchDrain
	require.NoError(t, dbFixture.DB.First(&stored, generation.ID).Error)
	assert.Equal(t, quotaBalanceBatchDrainApplied, stored.State)
	assert.Equal(t, wideDelta, stored.Payload.UserQuota[fixture.User.Id])
}

func TestHistoricalAmountCacheSchemasReplaceOnlyTheTouchedKey(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "historical-cache-schema")
	ctx := context.Background()
	const sentinelKey = "historical-cache-schema:sentinel"
	require.NoError(t, common.RDB.Set(ctx, sentinelKey, "keep", 0).Err())

	userKey := getUserCacheKey(fixture.User.Id)
	tokenKey := getTokenCacheKey(fixture.Token.Key)
	require.NoError(t, common.RDB.HSet(ctx, userKey, "CacheSchema", userCacheSchemaVersion-1).Err())
	require.NoError(t, common.RDB.HDel(ctx, userKey, "QuotaWriterEpoch", "QuotaVersion").Err())
	require.NoError(t, common.RDB.HSet(ctx, tokenKey, "CacheSchema", 1).Err())
	require.NoError(t, common.RDB.HDel(ctx, tokenKey, "QuotaWriterEpoch", "QuotaVersion").Err())

	wideUser := fixture.User
	wideUser.Quota = 4_000_000_000
	wideUser.QuotaVersion++
	require.NoError(t, populateUserCache(wideUser))
	assert.False(t, server.Exists(userKey), "an unowned schema refresh must not publish a balance")
	seedUserQuotaCacheForTest(t, wideUser.ToBaseUser())
	wideToken := fixture.Token
	wideToken.RemainQuota = 4_000_000_000
	wideToken.UsedQuota = 3_000_000_000
	wideToken.QuotaVersion++
	result, err := cacheInitToken(wideToken)
	require.NoError(t, err)
	assert.Zero(t, result)
	assert.False(t, server.Exists(tokenKey), "an unowned schema refresh must not publish a balance")
	seedTokenQuotaCacheForTest(t, wideToken)

	assert.Equal(t, strconv.Itoa(userCacheSchemaVersion), server.HGet(userKey, "CacheSchema"))
	assert.Equal(t, "4000000000", server.HGet(userKey, "Quota"))
	assert.Equal(t, strconv.Itoa(tokenCacheSchemaVersion), server.HGet(tokenKey, "CacheSchema"))
	assert.Equal(t, "4000000000", server.HGet(tokenKey, "RemainQuota"))
	assert.Equal(t, "3000000000", server.HGet(tokenKey, "UsedQuota"))
	sentinel, err := server.Get(sentinelKey)
	require.NoError(t, err)
	assert.Equal(t, "keep", sentinel, "schema migration must not clear unrelated cache keys")
}

func TestHistoricalAmountStatisticsAggregateWidensAndRejectsOutOfRangeDelta(t *testing.T) {
	const subjectID = 9206
	batchUpdateLocks[BatchUpdateTypeUsedQuota].Lock()
	previous := batchUpdateStores[BatchUpdateTypeUsedQuota]
	batchUpdateStores[BatchUpdateTypeUsedQuota] = make(map[int]int64)
	batchUpdateLocks[BatchUpdateTypeUsedQuota].Unlock()
	t.Cleanup(func() {
		batchUpdateLocks[BatchUpdateTypeUsedQuota].Lock()
		batchUpdateStores[BatchUpdateTypeUsedQuota] = previous
		batchUpdateLocks[BatchUpdateTypeUsedQuota].Unlock()
	})

	require.NoError(t, addNewRecord(BatchUpdateTypeUsedQuota, subjectID, 1_500_000_000))
	require.NoError(t, addNewRecord(BatchUpdateTypeUsedQuota, subjectID, 1_500_000_000))
	assert.Equal(t, int64(3_000_000_000), batchUpdateStores[BatchUpdateTypeUsedQuota][subjectID])

	err := addNewRecord(BatchUpdateTypeUsedQuota, subjectID, testMaxQuotaBalance)
	require.ErrorIs(t, err, common.ErrQuotaBalanceOutOfRange)
	assert.Equal(t, int64(3_000_000_000), batchUpdateStores[BatchUpdateTypeUsedQuota][subjectID], "failed statistics aggregation must leave the prior value intact")

	batchUpdateLocks[BatchUpdateTypeUsedQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUsedQuota][subjectID] = 0
	batchUpdateLocks[BatchUpdateTypeUsedQuota].Unlock()
	require.NoError(t, addNewRecord(BatchUpdateTypeUsedQuota, subjectID, -1))
	assert.Equal(t, int64(-1), batchUpdateStores[BatchUpdateTypeUsedQuota][subjectID], "refund-shaped deltas remain valid until checked against the persisted used state")
}
