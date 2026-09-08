package model

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const channelRoutingFixtureDatabase = "myapi_routing_test"

func channelRoutingDatabaseDialector(engine, dsn string) (gorm.Dialector, error) {
	unsafeTarget := errors.New("channel routing database tests require a literal loopback address and database " + channelRoutingFixtureDatabase)
	switch engine {
	case "mysql":
		config, err := mysqldriver.ParseDSN(dsn)
		if err != nil {
			return nil, errors.New("invalid channel routing MySQL fixture DSN")
		}
		host, _, err := net.SplitHostPort(config.Addr)
		if err != nil || config.Net != "tcp" || !net.ParseIP(host).IsLoopback() || config.DBName != channelRoutingFixtureDatabase {
			return nil, unsafeTarget
		}
		// MySQL may report a matched no-op UPDATE as affected when this option
		// is set. Claim must still read back the authoritative binding.
		config.ClientFoundRows = true
		return mysql.New(mysql.Config{DSN: config.FormatDSN()}), nil
	case "postgres":
		config, err := pgx.ParseConfig(dsn)
		if err != nil {
			return nil, errors.New("invalid channel routing PostgreSQL fixture DSN")
		}
		if !net.ParseIP(config.Host).IsLoopback() || config.Database != channelRoutingFixtureDatabase {
			return nil, unsafeTarget
		}
		for _, fallback := range config.Fallbacks {
			if !net.ParseIP(fallback.Host).IsLoopback() {
				return nil, unsafeTarget
			}
		}
		config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		return postgres.New(postgres.Config{Conn: stdlib.OpenDB(*config)}), nil
	default:
		return nil, errors.New("unsupported channel routing database engine")
	}
}

func TestChannelRoutingDatabaseMatrix(t *testing.T) {
	if os.Getenv("MYAPI_ROUTING_DATABASE_TESTS") != "1" {
		t.Skip("disposable channel routing database tests require MYAPI_ROUTING_DATABASE_TESTS=1")
	}

	fixtures := []struct {
		name   string
		engine string
		env    string
		typeID common.DatabaseType
	}{
		{name: "sqlite", engine: "sqlite", typeID: common.DatabaseTypeSQLite},
		{name: "mysql", engine: "mysql", env: "MYAPI_ROUTING_MYSQL_DSN", typeID: common.DatabaseTypeMySQL},
		{name: "postgres", engine: "postgres", env: "MYAPI_ROUTING_POSTGRES_DSN", typeID: common.DatabaseTypePostgreSQL},
	}

	runExternal := false
	for _, fixture := range fixtures[1:] {
		if os.Getenv(fixture.env) != "" {
			runExternal = true
		}
	}
	require.True(t, runExternal, "configure at least one routing database DSN when MYAPI_ROUTING_DATABASE_TESTS=1")

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			var dialector gorm.Dialector
			if fixture.engine == "sqlite" {
				dialector = sqlite.Open("file:channel-routing-matrix?mode=memory&cache=shared")
			} else {
				dsn := os.Getenv(fixture.env)
				if dsn == "" {
					t.Skip(fixture.env + " is not configured")
				}
				var err error
				dialector, err = channelRoutingDatabaseDialector(fixture.engine, dsn)
				require.NoError(t, err)
			}
			db, err := gorm.Open(dialector, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(2)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

			if fixture.engine != "sqlite" {
				tables, err := db.Migrator().GetTables()
				require.NoError(t, err)
				require.Empty(t, tables, "refusing a non-empty routing fixture database; no tables are dropped")
			}
			runChannelRoutingDatabaseMatrix(t, db, fixture.typeID)
		})
	}
}

func runChannelRoutingDatabaseMatrix(t *testing.T, db *gorm.DB, databaseType common.DatabaseType) {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousMemoryCache := common.MemoryCacheEnabled
	channelSyncLock.Lock()
	previousChannels, previousGroups, previousAdvanced := channelsIDM, group2model2channels, channel2advancedCustomConfig
	channelSyncLock.Unlock()

	DB, LOG_DB = db, db
	common.SetDatabaseTypes(databaseType, databaseType)
	common.MemoryCacheEnabled = false
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.MemoryCacheEnabled = previousMemoryCache
		channelSyncLock.Lock()
		channelsIDM, group2model2channels, channel2advancedCustomConfig = previousChannels, previousGroups, previousAdvanced
		channelSyncLock.Unlock()
		initCol()
	})

	require.NoError(t, db.AutoMigrate(&ChannelRoutingSession{}, &Channel{}, &Ability{}))
	first, err := ClaimChannelRoutingSession(context.Background(), "migration", 7, 100, 200)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ChannelRoutingSession{}, &Channel{}, &Ability{}))
	var persisted ChannelRoutingSession
	require.NoError(t, db.Where("key_hash = ?", "migration").First(&persisted).Error)
	assert.Equal(t, first, persisted, "repeated migration must retain routing bindings")

	runChannelRoutingSessionDatabaseContract(t)
	runChannelRoutingValuesDatabaseContract(t, db)
	runChannelQuotaSeriesCatalogueDatabaseContract(t, db)
}

func runChannelRoutingSessionDatabaseContract(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	start := make(chan struct{})
	results := make(chan ChannelRoutingSession, 2)
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	for _, channelID := range []int{31, 32} {
		workers.Add(1)
		go func(candidate int) {
			defer workers.Done()
			<-start
			binding, err := ClaimChannelRoutingSession(ctx, "same-user-hash", candidate, 100, 200)
			if err != nil {
				errors <- err
				return
			}
			results <- binding
		}(channelID)
	}
	close(start)
	workers.Wait()
	close(results)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	var winners []ChannelRoutingSession
	for binding := range results {
		winners = append(winners, binding)
	}
	require.Len(t, winners, 2)
	assert.Equal(t, winners[0], winners[1], "all claimants must observe the same winner")

	other, err := ClaimChannelRoutingSession(ctx, "other-user-hash", 33, 100, 200)
	require.NoError(t, err)
	assert.Equal(t, 33, other.ChannelId)
	assert.NotEqual(t, winners[0].KeyHash, other.KeyHash)

	switched, won, err := SwitchChannelRoutingSession(ctx, winners[0], 34, 110, 210)
	require.NoError(t, err)
	require.True(t, won)
	assert.Equal(t, 34, switched.ChannelId)
	stale, won, err := SwitchChannelRoutingSession(ctx, winners[0], 35, 111, 211)
	require.NoError(t, err)
	assert.False(t, won)
	assert.Equal(t, switched, stale)

	old, err := ClaimChannelRoutingSession(ctx, "expired-aba", 40, 100, 105)
	require.NoError(t, err)
	deleted, err := DeleteExpiredChannelRoutingSessions(ctx, 106, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	replacement, err := ClaimChannelRoutingSession(ctx, "expired-aba", 41, 106, 206)
	require.NoError(t, err)
	require.NotEqual(t, old.LeaseID, replacement.LeaseID)

	current, won, err := SwitchChannelRoutingSession(ctx, old, 42, 107, 207)
	require.NoError(t, err)
	assert.False(t, won)
	assert.Equal(t, replacement, current)
	require.NoError(t, TouchChannelRoutingSession(ctx, old, 107, 300))
	require.NoError(t, DeleteChannelRoutingSessionIfCurrent(ctx, old))
	var afterOldOperations ChannelRoutingSession
	require.NoError(t, DB.Where("key_hash = ?", replacement.KeyHash).First(&afterOldOperations).Error)
	assert.Equal(t, replacement, afterOldOperations, "old generation operations must not affect a replacement")
}

func runChannelRoutingValuesDatabaseContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	priority, weight := int64(9), uint(0)
	channel := Channel{
		Type:     1,
		Key:      "routing-fixture-secret",
		Status:   common.ChannelStatusEnabled,
		Name:     "routing-fixture",
		Models:   "routing-model",
		Group:    "routing-group",
		Priority: &priority,
		Weight:   &weight,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.UpdateAbilities(nil))

	updated, err := UpdateChannelRoutingValuesCAS(channel.Id, 12, 7, 9, 0)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, channel.Name, updated.Name)
	assert.Equal(t, channel.Key, updated.Key)
	assert.Equal(t, int64(12), updated.GetPriority())
	assert.Equal(t, 7, updated.GetWeight())
	var ability Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&ability).Error)
	assert.Equal(t, int64(12), *ability.Priority)
	assert.Equal(t, uint(7), ability.Weight)

	_, err = UpdateChannelRoutingValuesCAS(channel.Id, 13, 8, 9, 0)
	require.ErrorIs(t, err, ErrChannelRoutingConflict)
	var unchanged Channel
	require.NoError(t, db.First(&unchanged, channel.Id).Error)
	assert.Equal(t, int64(12), unchanged.GetPriority())
	assert.Equal(t, 7, unchanged.GetWeight())
	assert.Equal(t, channel.Name, unchanged.Name)
	assert.Equal(t, channel.Key, unchanged.Key)

	// Distinct raw legacy values must remain distinct CAS baselines in every dialect.
	legacyPriority, legacyWeight := MaxChannelRoutingPriority+42, MaxChannelRoutingWeight+42
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]interface{}{
		"priority": legacyPriority + 1, "weight": legacyWeight + 1,
	}).Error)
	_, err = UpdateChannelRoutingValuesCAS(channel.Id, 12, 7, legacyPriority, legacyWeight+1)
	require.ErrorIs(t, err, ErrChannelRoutingConflict)
	_, err = UpdateChannelRoutingValuesCAS(channel.Id, 12, 7, legacyPriority+1, legacyWeight)
	require.ErrorIs(t, err, ErrChannelRoutingConflict)
	_, err = UpdateChannelRoutingValuesCAS(channel.Id, 12, 7, legacyPriority+1, legacyWeight+1)
	require.NoError(t, err)

	zero, positive := int64(12), int64(12)
	zeroWeight, positiveWeight := uint(0), uint(3)
	second := Channel{Type: 1, Key: "routing-second-secret", Status: common.ChannelStatusEnabled, Name: "routing-second", Models: "routing-model", Group: "routing-group", Priority: &positive, Weight: &positiveWeight}
	channel.Priority, channel.Weight = &zero, &zeroWeight
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Select("priority", "weight").Updates(&channel).Error)
	require.NoError(t, channel.UpdateAbilities(nil))
	require.NoError(t, db.Create(&second).Error)
	require.NoError(t, second.UpdateAbilities(nil))

	for _, memoryCache := range []bool{false, true} {
		common.MemoryCacheEnabled = memoryCache
		if memoryCache {
			InitChannelCache()
		}
		candidates, err := ListEligibleChannelRoutingCandidates("routing-group", "routing-model", "")
		require.NoError(t, err)
		require.Len(t, candidates, 2)
		weights := make([]WeightedChannelCandidate, 0, len(candidates))
		for _, candidate := range candidates {
			weights = append(weights, WeightedChannelCandidate{ChannelID: candidate.ID, Weight: candidate.Weight})
		}
		selected, ok := selectWeightedChannelAt(weights, 0)
		require.True(t, ok)
		assert.Equal(t, second.Id, selected, "a zero-weight candidate must receive no share while another has weight")
	}
}
