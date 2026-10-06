package model

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func routingDraw(value uint64) channelRoutingRandomInt {
	return func(max *big.Int) (*big.Int, error) {
		return new(big.Int).SetUint64(value), nil
	}
}

func TestBuildChannelRoutingPolicyContract(t *testing.T) {
	policy := BuildChannelRoutingPolicy([]ChannelRoutingCandidate{
		{ChannelID: 3, Priority: 10, Weight: 0},
		{ChannelID: 1, Priority: 10, Weight: 0},
		{ChannelID: 4, Priority: 5, Weight: 30},
		{ChannelID: 2, Priority: 5, Weight: 0},
	})

	require.Len(t, policy.Tiers, 2)
	assert.Equal(t, int64(10), policy.Tiers[0].Priority)
	assert.Equal(t, []int{1, 3}, []int{
		policy.Tiers[0].Candidates[0].ChannelID,
		policy.Tiers[0].Candidates[1].ChannelID,
	})
	assert.Equal(t, uint(1), policy.Tiers[0].Candidates[0].EffectiveWeight)
	assert.Equal(t, 0.5, policy.Tiers[0].Candidates[0].ExpectedShare)
	assert.Equal(t, int64(5), policy.Tiers[1].Priority)
	assert.Equal(t, uint(0), policy.Tiers[1].Candidates[0].EffectiveWeight)
	assert.Equal(t, 0.0, policy.Tiers[1].Candidates[0].ExpectedShare)
	assert.Equal(t, uint(30), policy.Tiers[1].Candidates[1].EffectiveWeight)
	assert.Equal(t, 1.0, policy.Tiers[1].Candidates[1].ExpectedShare)
}

func TestSelectChannelFromRoutingPolicyAllZeroTierIsEven(t *testing.T) {
	policy := BuildChannelRoutingPolicy([]ChannelRoutingCandidate{
		{ChannelID: 2, Priority: 0, Weight: 0},
		{ChannelID: 1, Priority: 0, Weight: 0},
	})

	first, found, err := selectChannelFromRoutingPolicy(policy, 0, routingDraw(0))
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 1, first)

	second, found, err := selectChannelFromRoutingPolicy(policy, 0, routingDraw(1))
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 2, second)
}

func TestSelectChannelFromRoutingPolicySingleChannelIsCertain(t *testing.T) {
	policy := BuildChannelRoutingPolicy([]ChannelRoutingCandidate{
		{ChannelID: 7, Priority: 0, Weight: 0},
	})

	selected, found, err := selectChannelFromRoutingPolicy(policy, 0, nil)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 7, selected)
	assert.Equal(t, 1.0, policy.Tiers[0].Candidates[0].ExpectedShare)
}

func TestSelectChannelFromRoutingPolicyUsesPriorityFallbackTiers(t *testing.T) {
	policy := BuildChannelRoutingPolicy([]ChannelRoutingCandidate{
		{ChannelID: 1, Priority: 100, Weight: 1},
		{ChannelID: 2, Priority: 50, Weight: 1},
		{ChannelID: 3, Priority: 0, Weight: 1},
	})

	for retry, expected := range []int{1, 2, 3, 3} {
		selected, found, err := selectChannelFromRoutingPolicy(policy, retry, routingDraw(0))
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, expected, selected)
	}
}

func TestSelectChannelFromRoutingPolicyWeightBoundaries(t *testing.T) {
	policy := BuildChannelRoutingPolicy([]ChannelRoutingCandidate{
		{ChannelID: 1, Priority: 0, Weight: 2},
		{ChannelID: 2, Priority: 0, Weight: 0},
		{ChannelID: 3, Priority: 0, Weight: 3},
	})

	cases := []struct {
		draw     uint64
		expected int
	}{
		{draw: 0, expected: 1},
		{draw: 1, expected: 1},
		{draw: 2, expected: 3},
		{draw: 4, expected: 3},
	}
	for _, test := range cases {
		selected, found, err := selectChannelFromRoutingPolicy(policy, 0, routingDraw(test.draw))
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, test.expected, selected)
	}
}

func TestSelectChannelFromRoutingPolicyIsOverflowSafe(t *testing.T) {
	maxWeight := ^uint(0)
	policy := BuildChannelRoutingPolicy([]ChannelRoutingCandidate{
		{ChannelID: 1, Priority: 0, Weight: maxWeight},
		{ChannelID: 2, Priority: 0, Weight: maxWeight},
	})

	selected, found, err := selectChannelFromRoutingPolicy(policy, 0, func(max *big.Int) (*big.Int, error) {
		bitSize := uint(65)
		if ^uint(0) == uint(^uint32(0)) {
			bitSize = 33
		}
		expectedTotal := new(big.Int).Lsh(big.NewInt(1), bitSize)
		expectedTotal.Sub(expectedTotal, big.NewInt(2))
		assert.Equal(t, expectedTotal, max)
		return new(big.Int).SetUint64(uint64(maxWeight)), nil
	})

	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 2, selected)
	assert.InDelta(t, 0.5, policy.Tiers[0].Candidates[0].ExpectedShare, 1e-12)
	assert.InDelta(t, 0.5, policy.Tiers[0].Candidates[1].ExpectedShare, 1e-12)
}

func TestDatabaseAndCacheRoutingUseSameZeroWeightContract(t *testing.T) {
	truncateTables(t)
	previousMemoryCache := common.MemoryCacheEnabled
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCache
	})

	priority := int64(10)
	zero := uint(0)
	positive := uint(7)
	channels := []*Channel{
		{Id: 1, Type: constant.ChannelTypeOpenAI, Key: "key-1", Status: common.ChannelStatusEnabled, Name: "zero", Group: "default", Models: "gpt-test", Priority: &priority, Weight: &zero},
		{Id: 2, Type: constant.ChannelTypeOpenAI, Key: "key-2", Status: common.ChannelStatusEnabled, Name: "positive", Group: "default", Models: "gpt-test", Priority: &priority, Weight: &positive},
	}
	for _, channel := range channels {
		require.NoError(t, DB.Create(channel).Error)
		require.NoError(t, channel.AddAbilities(nil))
	}

	common.MemoryCacheEnabled = false
	for range 10 {
		selected, err := GetRandomSatisfiedChannel("default", "gpt-test", 0, "")
		require.NoError(t, err)
		require.NotNil(t, selected)
		assert.Equal(t, 2, selected.Id)
	}

	common.MemoryCacheEnabled = true
	InitChannelCache()
	for range 10 {
		selected, err := GetRandomSatisfiedChannel("default", "gpt-test", 0, "")
		require.NoError(t, err)
		require.NotNil(t, selected)
		assert.Equal(t, 2, selected.Id)
	}
}

func TestChannelListSortingTreatsNullAsZeroAndKeepsPaginationStable(t *testing.T) {
	truncateTables(t)
	priorityOne := int64(1)
	priorityZero := int64(0)
	priorityNegative := int64(-1)
	weightZero := uint(0)
	weightFive := uint(5)
	for _, channel := range []*Channel{
		{Id: 1, Key: "1", Name: "nil", Priority: &priorityZero, Weight: &weightZero},
		{Id: 2, Key: "2", Name: "zero", Priority: &priorityZero, Weight: &weightZero},
		{Id: 3, Key: "3", Name: "positive-priority", Priority: &priorityOne, Weight: &weightZero},
		{Id: 4, Key: "4", Name: "positive-weight", Priority: &priorityZero, Weight: &weightFive},
		{Id: 5, Key: "5", Name: "negative-priority", Priority: &priorityNegative, Weight: &weightZero},
	} {
		require.NoError(t, DB.Create(channel).Error)
	}
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 1).Updates(map[string]any{
		"priority": nil,
		"weight":   nil,
	}).Error)

	pageOne, err := GetAllChannels(0, 2, false, false)
	require.NoError(t, err)
	assert.Equal(t, []int{3, 1}, []int{pageOne[0].Id, pageOne[1].Id})
	pageTwo, err := GetAllChannels(2, 2, false, false)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 4}, []int{pageTwo[0].Id, pageTwo[1].Id})
	pageThree, err := GetAllChannels(4, 2, false, false)
	require.NoError(t, err)
	require.Len(t, pageThree, 1)
	assert.Equal(t, 5, pageThree[0].Id)

	weightDescending, err := GetAllChannels(0, 20, false, false, NewChannelSortOptions("weight", "desc", false))
	require.NoError(t, err)
	assert.Equal(t, []int{4, 1, 2, 3, 5}, []int{
		weightDescending[0].Id,
		weightDescending[1].Id,
		weightDescending[2].Id,
		weightDescending[3].Id,
		weightDescending[4].Id,
	})
	weightAscending, err := GetAllChannels(0, 20, false, false, NewChannelSortOptions("weight", "asc", false))
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3, 5, 4}, []int{
		weightAscending[0].Id,
		weightAscending[1].Id,
		weightAscending[2].Id,
		weightAscending[3].Id,
		weightAscending[4].Id,
	})
}

func TestChannelNullableSortSQLIsPortableAcrossDialects(t *testing.T) {
	sqliteDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := sqliteDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	for _, test := range []struct {
		name      string
		dialector gorm.Dialector
	}{
		{name: "sqlite", dialector: sqlite.Open(":memory:")},
		{name: "mysql", dialector: mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true})},
		{name: "postgres", dialector: postgres.New(postgres.Config{Conn: sqlDB})},
	} {
		t.Run(test.name, func(t *testing.T) {
			dryDB, openErr := gorm.Open(test.dialector, &gorm.Config{DryRun: true, DisableAutomaticPing: true})
			require.NoError(t, openErr)
			var channels []*Channel
			statement := NewChannelSortOptions("priority", "desc", false).
				Apply(dryDB.Model(&Channel{})).
				Limit(2).
				Offset(2).
				Find(&channels).
				Statement
			generatedSQL := statement.SQL.String()
			require.Contains(t, generatedSQL, "COALESCE(priority, 0) DESC")
			require.Contains(t, generatedSQL, "id")
			require.Contains(t, generatedSQL, "LIMIT")
			require.Contains(t, generatedSQL, "OFFSET")
		})
	}
}

func TestChannelNullableSortAgainstConfiguredDatabases(t *testing.T) {
	for _, test := range []struct {
		name      string
		envName   string
		dialector func(string) gorm.Dialector
	}{
		{name: "mysql", envName: "TEST_MYSQL_DSN", dialector: func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{name: "postgres", envName: "TEST_POSTGRES_DSN", dialector: func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.envName))
			if dsn == "" {
				t.Skipf("%s is not configured; actual %s nullable sorting was not run", test.envName, test.name)
			}
			db, err := gorm.Open(test.dialector(dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Ping())
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

			tableName := fmt.Sprintf("wp3_r1_channel_sort_%d", time.Now().UnixNano())
			require.NoError(t, db.Exec("CREATE TABLE "+tableName+" (id BIGINT PRIMARY KEY, priority BIGINT NULL, weight BIGINT NULL)").Error)
			t.Cleanup(func() { _ = db.Exec("DROP TABLE IF EXISTS " + tableName).Error })
			for _, values := range [][]any{
				{int64(1), nil, nil},
				{int64(2), int64(0), int64(0)},
				{int64(3), int64(1), int64(0)},
				{int64(4), nil, int64(5)},
			} {
				require.NoError(t, db.Exec(
					"INSERT INTO "+tableName+" (id, priority, weight) VALUES (?, ?, ?)",
					values...,
				).Error)
			}

			type sortRow struct {
				ID int64 `gorm:"column:id"`
			}
			var firstPage []sortRow
			require.NoError(t, NewChannelSortOptions("", "", false).
				Apply(db.Table(tableName)).Limit(2).Find(&firstPage).Error)
			assert.Equal(t, []int64{3, 1}, []int64{firstPage[0].ID, firstPage[1].ID})
			var secondPage []sortRow
			require.NoError(t, NewChannelSortOptions("", "", false).
				Apply(db.Table(tableName)).Limit(2).Offset(2).Find(&secondPage).Error)
			assert.Equal(t, []int64{2, 4}, []int64{secondPage[0].ID, secondPage[1].ID})
			var weightOrder []sortRow
			require.NoError(t, NewChannelSortOptions("weight", "desc", false).
				Apply(db.Table(tableName)).Find(&weightOrder).Error)
			assert.Equal(t, []int64{4, 1, 2, 3}, []int64{
				weightOrder[0].ID,
				weightOrder[1].ID,
				weightOrder[2].ID,
				weightOrder[3].ID,
			})
		})
	}
}

func routingPolicyFromCacheForTest(group string, modelName string, requestPath string) ChannelRoutingPolicy {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	return getCachedChannelRoutingPolicy(group, modelName, requestPath)
}

func createRoutingChannelAndAbility(t *testing.T, channel *Channel, ability Ability) {
	t.Helper()
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&ability).Error)
}

func TestRoutingReadsMalformedAdvancedSettingsWithoutWritingChannel(t *testing.T) {
	truncateTables(t)
	previousMemoryCache := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })

	priority := int64(10)
	weight := uint(3)
	baseURL := "https://routing.invalid.example"
	channel := &Channel{
		Id:            91,
		Type:          constant.ChannelTypeAdvancedCustom,
		Key:           "must-remain-secret",
		Status:        common.ChannelStatusEnabled,
		Name:          "malformed-routing-settings",
		BaseURL:       &baseURL,
		Models:        "advanced-model",
		Group:         "default",
		Priority:      &priority,
		Weight:        &weight,
		OtherSettings: `{"advanced_custom":`,
	}
	createRoutingChannelAndAbility(t, channel, Ability{
		Group: "default", Model: "advanced-model", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: weight,
	})

	config, err := parseChannelAdvancedCustomRoutingConfig(channel)
	require.ErrorIs(t, err, ErrInvalidChannelRoutingSettings)
	assert.Nil(t, config)

	policy, err := GetChannelRoutingPolicy("default", "advanced-model", "/v1/responses")
	require.NoError(t, err)
	assert.Empty(t, policy.Tiers)

	common.MemoryCacheEnabled = true
	InitChannelCache()
	selected, err := GetRandomSatisfiedChannel("default", "advanced-model", 0, "/v1/responses")
	require.NoError(t, err)
	assert.Nil(t, selected)

	persisted, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, channel.Key, persisted.Key)
	assert.Equal(t, channel.Models, persisted.Models)
	assert.Equal(t, channel.Group, persisted.Group)
	assert.Equal(t, channel.Name, persisted.Name)
	assert.Equal(t, channel.OtherSettings, persisted.OtherSettings)
	require.NotNil(t, persisted.BaseURL)
	assert.Equal(t, baseURL, *persisted.BaseURL)
}

func TestDatabaseAndCacheRoutingCandidatesUseAbilityAuthority(t *testing.T) {
	truncateTables(t)
	previousMemoryCache := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })

	channelPriority := int64(999)
	channelWeight := uint(999)
	tierHigh := int64(20)
	tierLow := int64(10)
	zero := uint(0)
	positive := uint(5)
	rows := []struct {
		channel *Channel
		ability Ability
	}{
		{
			channel: &Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Key: "1", Status: common.ChannelStatusEnabled, Name: "first", Group: "default,default", Models: "contract-model,contract-model", Priority: &channelPriority, Weight: &channelWeight},
			ability: Ability{Group: "default", Model: "contract-model", ChannelId: 1, Enabled: true, Priority: &tierHigh, Weight: zero},
		},
		{
			channel: &Channel{Id: 2, Type: constant.ChannelTypeOpenAI, Key: "2", Status: common.ChannelStatusEnabled, Name: "second", Group: "wrong", Models: "wrong", Priority: &channelPriority, Weight: &channelWeight},
			ability: Ability{Group: "default", Model: "contract-model", ChannelId: 2, Enabled: true, Priority: &tierHigh, Weight: zero},
		},
		{
			channel: &Channel{Id: 3, Type: constant.ChannelTypeOpenAI, Key: "3", Status: common.ChannelStatusEnabled, Name: "ability-disabled", Group: "default", Models: "contract-model"},
			ability: Ability{Group: "default", Model: "contract-model", ChannelId: 3, Enabled: false, Priority: &channelPriority, Weight: channelWeight},
		},
		{
			channel: &Channel{Id: 4, Type: constant.ChannelTypeOpenAI, Key: "4", Status: common.ChannelStatusManuallyDisabled, Name: "channel-disabled", Group: "default", Models: "contract-model"},
			ability: Ability{Group: "default", Model: "contract-model", ChannelId: 4, Enabled: true, Priority: &channelPriority, Weight: channelWeight},
		},
		{
			channel: &Channel{Id: 5, Type: constant.ChannelTypeOpenAI, Key: "5", Status: common.ChannelStatusEnabled, Name: "weighted", Group: "wrong", Models: "wrong", Priority: &channelPriority, Weight: &zero},
			ability: Ability{Group: "default", Model: "contract-model", ChannelId: 5, Enabled: true, Priority: &tierLow, Weight: positive},
		},
		{
			channel: &Channel{Id: 6, Type: constant.ChannelTypeOpenAI, Key: "6", Status: common.ChannelStatusEnabled, Name: "zero-low", Group: "default", Models: "contract-model", Priority: &channelPriority, Weight: &channelWeight},
			ability: Ability{Group: "default", Model: "contract-model", ChannelId: 6, Enabled: true, Priority: &tierLow, Weight: zero},
		},
	}
	for _, row := range rows {
		createRoutingChannelAndAbility(t, row.channel, row.ability)
	}

	databasePolicy, err := GetChannelRoutingPolicy("default", "contract-model", "")
	require.NoError(t, err)
	common.MemoryCacheEnabled = true
	InitChannelCache()
	cachePolicy := routingPolicyFromCacheForTest("default", "contract-model", "")
	require.Equal(t, databasePolicy, cachePolicy)
	require.Len(t, cachePolicy.Tiers, 2)
	assert.Equal(t, []int{1, 2}, []int{
		cachePolicy.Tiers[0].Candidates[0].ChannelID,
		cachePolicy.Tiers[0].Candidates[1].ChannelID,
	})
	assert.Equal(t, uint(1), cachePolicy.Tiers[0].Candidates[0].EffectiveWeight)
	assert.Equal(t, uint(1), cachePolicy.Tiers[0].Candidates[1].EffectiveWeight)
	assert.Equal(t, []int{5, 6}, []int{
		cachePolicy.Tiers[1].Candidates[0].ChannelID,
		cachePolicy.Tiers[1].Candidates[1].ChannelID,
	})
	assert.Equal(t, uint(5), cachePolicy.Tiers[1].Candidates[0].EffectiveWeight)
	assert.Equal(t, uint(0), cachePolicy.Tiers[1].Candidates[1].EffectiveWeight)

	for _, policy := range []ChannelRoutingPolicy{databasePolicy, cachePolicy} {
		first, found, err := selectChannelFromRoutingPolicy(policy, 0, routingDraw(0))
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, 1, first)
		second, found, err := selectChannelFromRoutingPolicy(policy, 0, routingDraw(1))
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, 2, second)
		fallback, found, err := selectChannelFromRoutingPolicy(policy, 1, routingDraw(0))
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, 5, fallback)
	}

	channelSyncLock.RLock()
	assert.Equal(t, []int{1, 2, 5, 6}, group2model2channels["default"]["contract-model"])
	channelSyncLock.RUnlock()
}

func TestDatabaseAndCacheRoutingMatchNormalizedModelAndAdvancedPath(t *testing.T) {
	truncateTables(t)
	previousMemoryCache := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })

	priority := int64(10)
	weight := uint(1)
	createRoutingChannelAndAbility(t, &Channel{
		Id: 20, Type: constant.ChannelTypeOpenAI, Key: "20", Status: common.ChannelStatusEnabled,
		Name: "normalized", Group: "unrelated", Models: "unrelated",
	}, Ability{
		Group: "default", Model: "gpt-4o-gizmo-*", ChannelId: 20,
		Enabled: true, Priority: &priority, Weight: weight,
	})

	settingsBytes, err := common.Marshal(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{
			{IncomingPath: "/v1/responses", Models: []string{"advanced-model"}},
		}},
	})
	require.NoError(t, err)
	createRoutingChannelAndAbility(t, &Channel{
		Id: 21, Type: constant.ChannelTypeAdvancedCustom, Key: "21", Status: common.ChannelStatusEnabled,
		Name: "advanced", Group: "unrelated", Models: "unrelated", OtherSettings: string(settingsBytes),
	}, Ability{
		Group: "default", Model: "advanced-model", ChannelId: 21,
		Enabled: true, Priority: &priority, Weight: weight,
	})

	common.MemoryCacheEnabled = true
	InitChannelCache()
	for _, test := range []struct {
		name        string
		modelName   string
		requestPath string
		expectedIDs []int
	}{
		{name: "normalized model", modelName: "gpt-4o-gizmo-tenant", expectedIDs: []int{20}},
		{name: "advanced matching path", modelName: "advanced-model", requestPath: "/v1/responses", expectedIDs: []int{21}},
		{name: "advanced rejected path", modelName: "advanced-model", requestPath: "/v1/chat/completions", expectedIDs: []int{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			databasePolicy, policyErr := GetChannelRoutingPolicy("default", test.modelName, test.requestPath)
			require.NoError(t, policyErr)
			cachePolicy := routingPolicyFromCacheForTest("default", test.modelName, test.requestPath)
			require.Equal(t, databasePolicy, cachePolicy)
			actualIDs := make([]int, 0)
			for _, tier := range databasePolicy.Tiers {
				for _, candidate := range tier.Candidates {
					actualIDs = append(actualIDs, candidate.ChannelID)
				}
			}
			assert.Equal(t, test.expectedIDs, actualIDs)
		})
	}
}

func setupChannelCacheSnapshotTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousMemoryCache := common.MemoryCacheEnabled
	channelSyncLock.Lock()
	previousChannels := channelsIDM
	previousGroups := group2model2channels
	previousCandidates := group2model2routingCandidates
	previousConfigs := channel2advancedCustomConfig
	previousDataGeneration := channelCacheDataGeneration
	previousPublishGeneration := channelCachePublishGeneration
	previousObservedEpoch := channelCacheObservedCommittedEpoch
	previousPublishedEpoch := channelCachePublishedEpoch
	channelSyncLock.Unlock()

	db, err := gorm.Open(sqlite.Open("file:channel-cache-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db
	channelSyncLock.Lock()
	channelCacheObservedCommittedEpoch = 0
	channelCachePublishedEpoch = 0
	channelCacheDataGeneration = 0
	channelCachePublishGeneration = 0
	channelSyncLock.Unlock()
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		channelSyncLock.Lock()
		channelsIDM = previousChannels
		group2model2channels = previousGroups
		group2model2routingCandidates = previousCandidates
		channel2advancedCustomConfig = previousConfigs
		channelCacheDataGeneration = previousDataGeneration
		channelCachePublishGeneration = previousPublishGeneration
		channelCacheObservedCommittedEpoch = previousObservedEpoch
		channelCachePublishedEpoch = previousPublishedEpoch
		channelSyncLock.Unlock()
		DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCache
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func TestInitChannelCacheQueryFailurePreservesLastGoodSnapshot(t *testing.T) {
	db := setupChannelCacheSnapshotTest(t)
	priority := int64(10)
	weight := uint(3)
	channel := &Channel{Id: 301, Type: constant.ChannelTypeOpenAI, Key: "key", Status: common.ChannelStatusEnabled, Name: "last-good"}
	createRoutingChannelAndAbility(t, channel, Ability{
		Group: "default", Model: "snapshot-model", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: weight,
	})
	require.NoError(t, InitChannelCache())
	beforePolicy := routingPolicyFromCacheForTest("default", "snapshot-model", "")
	channelSyncLock.RLock()
	beforePublishGeneration := channelCachePublishGeneration
	channelSyncLock.RUnlock()

	queryFailure := errors.New("forced channel snapshot query failure")
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:fail_channel_cache_snapshot", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "JOIN abilities") {
			tx.AddError(queryFailure)
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, db.Callback().Query().Remove("test:fail_channel_cache_snapshot"))
	})

	err := InitChannelCache()
	require.ErrorIs(t, err, queryFailure)
	assert.Equal(t, beforePolicy, routingPolicyFromCacheForTest("default", "snapshot-model", ""))
	cachedChannel, cacheErr := CacheGetChannel(channel.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, "last-good", cachedChannel.Name)
	channelSyncLock.RLock()
	assert.Equal(t, beforePublishGeneration, channelCachePublishGeneration)
	channelSyncLock.RUnlock()
}

func TestConcurrentChannelCacheRefreshCannotPublishOlderSnapshotLast(t *testing.T) {
	db := setupChannelCacheSnapshotTest(t)
	priority := int64(10)
	initialWeight := uint(1)
	channel := &Channel{Id: 302, Type: constant.ChannelTypeOpenAI, Key: "key", Status: common.ChannelStatusEnabled, Name: "generation"}
	createRoutingChannelAndAbility(t, channel, Ability{
		Group: "default", Model: "generation-model", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: initialWeight,
	})
	require.NoError(t, InitChannelCache())

	firstSnapshotRead := make(chan struct{})
	releaseFirstSnapshot := make(chan struct{})
	var snapshotReads atomic.Int32
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:block_old_channel_cache_snapshot", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "JOIN abilities") && snapshotReads.Add(1) == 1 {
			close(firstSnapshotRead)
			<-releaseFirstSnapshot
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, db.Callback().Query().Remove("test:block_old_channel_cache_snapshot"))
	})

	firstDone := make(chan error, 1)
	go func() { firstDone <- InitChannelCache() }()
	<-firstSnapshotRead
	updatedWeight := uint(9)
	_, _, err := runChannelRoutingTransaction(context.Background(), func(tx *gorm.DB) (bool, error) {
		result := tx.Model(&Ability{}).Where("channel_id = ?", channel.Id).Update("weight", updatedWeight)
		return result.RowsAffected > 0, result.Error
	})
	require.NoError(t, err)
	secondDone := make(chan error, 1)
	go func() { secondDone <- InitChannelCache() }()
	close(releaseFirstSnapshot)

	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)
	policy := routingPolicyFromCacheForTest("default", "generation-model", "")
	require.Len(t, policy.Tiers, 1)
	require.Len(t, policy.Tiers[0].Candidates, 1)
	assert.Equal(t, updatedWeight, policy.Tiers[0].Candidates[0].Weight)
	assert.GreaterOrEqual(t, snapshotReads.Load(), int32(3))
}

func TestChannelCachePublicationGenerationsRemainPendingUntilSuccessfulPublish(t *testing.T) {
	db := setupChannelCacheSnapshotTest(t)
	priority := int64(10)
	weight := uint(1)
	channel := &Channel{Id: 303, Type: constant.ChannelTypeOpenAI, Key: "key", Status: common.ChannelStatusEnabled, Name: "generation-state"}
	createRoutingChannelAndAbility(t, channel, Ability{
		Group: "default", Model: "generation-state-model", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: weight,
	})
	require.NoError(t, InitChannelCache())
	published := GetChannelCachePublicationState()
	assert.False(t, published.CachePending)
	assert.Equal(t, published.DataGeneration, published.PublishedGeneration)

	updatedWeight := uint(7)
	_, _, err := runChannelRoutingTransaction(context.Background(), func(tx *gorm.DB) (bool, error) {
		result := tx.Model(&Ability{}).Where("channel_id = ?", channel.Id).Update("weight", updatedWeight)
		return result.RowsAffected > 0, result.Error
	})
	require.NoError(t, err)
	dirty := GetChannelCachePublicationState()
	assert.True(t, dirty.CachePending)
	assert.Greater(t, dirty.DataGeneration, dirty.PublishedGeneration)

	queryFailure := errors.New("forced pending generation publish failure")
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:fail_pending_generation_publish", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "JOIN abilities") {
			tx.AddError(queryFailure)
		}
	}))
	callbackRegistered := true
	t.Cleanup(func() {
		if callbackRegistered {
			require.NoError(t, db.Callback().Query().Remove("test:fail_pending_generation_publish"))
		}
	})

	require.ErrorIs(t, InitChannelCache(), queryFailure)
	failed := GetChannelCachePublicationState()
	assert.Equal(t, dirty, failed)

	require.NoError(t, db.Callback().Query().Remove("test:fail_pending_generation_publish"))
	callbackRegistered = false
	require.NoError(t, InitChannelCache())
	repaired := GetChannelCachePublicationState()
	assert.False(t, repaired.CachePending)
	assert.Equal(t, repaired.DataGeneration, repaired.PublishedGeneration)
	policy := routingPolicyFromCacheForTest("default", "generation-state-model", "")
	require.Len(t, policy.Tiers, 1)
	assert.Equal(t, updatedWeight, policy.Tiers[0].Candidates[0].Weight)
}

func TestChannelRoutingEpochAgainstConfiguredDatabases(t *testing.T) {
	for _, test := range []struct {
		name      string
		envName   string
		dialector func(string) gorm.Dialector
	}{
		{name: "mysql", envName: "TEST_MYSQL_DSN", dialector: func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{name: "postgres", envName: "TEST_POSTGRES_DSN", dialector: func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.envName))
			if dsn == "" {
				t.Skipf("%s is not configured; actual %s routing epoch test was not run", test.envName, test.name)
			}
			db, err := gorm.Open(test.dialector(dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Ping())
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			resetChannelRoutingSchemaStateForTest(db)
			require.NoError(t, ensureChannelRoutingSchema(db))
			beforeEpoch, err := GetCommittedChannelRoutingEpoch(db)
			require.NoError(t, err)

			rollback := errors.New("rollback routing epoch dialect test")
			err = db.Transaction(func(tx *gorm.DB) error {
				nextEpoch, advanceErr := advanceChannelRoutingEpoch(tx)
				require.NoError(t, advanceErr)
				assert.Equal(t, beforeEpoch+1, nextEpoch)
				return rollback
			})
			require.ErrorIs(t, err, rollback)
			afterEpoch, err := GetCommittedChannelRoutingEpoch(db)
			require.NoError(t, err)
			assert.Equal(t, beforeEpoch, afterEpoch)
		})
	}
}
