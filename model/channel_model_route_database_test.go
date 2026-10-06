package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestChannelModelRouteConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			databasePath := t.TempDir() + "/model-routes.db"
			var dialector gorm.Dialector = sqlite.Open(databasePath)
			dsn := ""
			if engine.env != "" {
				if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
					t.Skip("requires explicitly configured disposable loopback B2 database")
				}
				dsn = strings.TrimSpace(os.Getenv(engine.env))
				require.NotEmpty(t, dsn)
				var err error
				dialector, err = b2SubmissionDatabaseDialector(engine.name, dsn)
				require.NoError(t, err)
			}
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			pool, err := db.DB()
			require.NoError(t, err)
			pool.SetMaxOpenConns(1)
			oldDB, oldMemory, oldRedis := DB, common.MemoryCacheEnabled, common.RedisEnabled
			oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
			channelSyncLock.Lock()
			oldChannels, oldGroups, oldCandidates, oldConfigs := channelsIDM, group2model2channels, group2model2routingCandidates, channel2advancedCustomConfig
			oldCacheDB := channelCacheDB
			oldData, oldPublished := channelCacheDataGeneration, channelCachePublishGeneration
			oldObservedEpoch, oldPublishedEpoch := channelCacheObservedCommittedEpoch, channelCachePublishedEpoch
			channelSyncLock.Unlock()
			DB = db
			common.MemoryCacheEnabled, common.RedisEnabled = false, false
			common.SetDatabaseTypes(common.DatabaseType(engine.name), common.DatabaseType(engine.name))
			InitColumnNamesForTest()
			t.Cleanup(func() {
				channelSyncLock.Lock()
				channelsIDM, group2model2channels, group2model2routingCandidates, channel2advancedCustomConfig = oldChannels, oldGroups, oldCandidates, oldConfigs
				channelCacheDB = oldCacheDB
				channelCacheDataGeneration, channelCachePublishGeneration = oldData, oldPublished
				channelCacheObservedCommittedEpoch, channelCachePublishedEpoch = oldObservedEpoch, oldPublishedEpoch
				channelSyncLock.Unlock()
				resetChannelRoutingSchemaStateForTest(db)
				DB, common.MemoryCacheEnabled, common.RedisEnabled = oldDB, oldMemory, oldRedis
				common.SetDatabaseTypes(oldMain, oldLog)
				InitColumnNamesForTest()
				require.NoError(t, pool.Close())
			})
			require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
			group := fmt.Sprintf("route-%x", time.Now().UnixNano())
			rules := []dto.ModelRoute{
				{PublicModel: "public-model", UpstreamModel: "chat-target", Match: "exact", Endpoint: "/v1/chat/completions"},
				{PublicModel: "public-model", UpstreamModel: "responses-target", Match: "exact", Endpoint: "/v1/responses"},
				{PublicModel: "legacy-alias", UpstreamModel: "rule-must-not-win", Match: "exact", Priority: 100},
				{PublicModel: "unlisted-", UpstreamModel: "prefix-must-not-enable", Match: "prefix"},
			}
			settings, err := common.Marshal(dto.ChannelOtherSettings{ModelRoutes: rules})
			require.NoError(t, err)
			codexSettings, err := common.Marshal(dto.ChannelOtherSettings{ModelRoutes: []dto.ModelRoute{{PublicModel: "public-model", UpstreamModel: "codex-target", Match: "exact", Endpoint: "/v1/responses"}}})
			require.NoError(t, err)
			exact := `{"legacy-alias":"exact-target"}`
			high, low := int64(20), int64(10)
			weight := uint(1)
			channels := []Channel{
				{Type: constant.ChannelTypeOpenAI, Key: "synthetic-openai", Name: "openai-route-fixture", Status: common.ChannelStatusEnabled, Group: group, Models: "public-model,legacy-alias", ModelMapping: &exact, OtherSettings: string(settings), Priority: &high, Weight: &weight},
				{Type: constant.ChannelTypeCodex, Key: "synthetic-codex", Name: "codex-route-fixture", Status: common.ChannelStatusEnabled, Group: group, Models: "public-model", OtherSettings: string(codexSettings), Priority: &low, Weight: &weight},
			}
			ids := make([]int, 0, len(channels))
			t.Cleanup(func() {
				if len(ids) > 0 {
					require.NoError(t, db.Where("channel_id IN ?", ids).Delete(&Ability{}).Error)
					require.NoError(t, db.Where("id IN ?", ids).Delete(&Channel{}).Error)
				}
			})
			for i := range channels {
				require.NoError(t, db.Create(&channels[i]).Error)
				ids = append(ids, channels[i].Id)
				require.NoError(t, channels[i].AddAbilities(nil))
			}
			// The new rules live in the existing settings TEXT. Repeated startup
			// migration must preserve both those rules and explicit enabled abilities.
			require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
			require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
			resetChannelRoutingSchemaStateForTest(db)
			require.NoError(t, pool.Close())
			dialector = sqlite.Open(databasePath)
			if engine.env != "" {
				dialector, err = b2SubmissionDatabaseDialector(engine.name, dsn)
				require.NoError(t, err)
			}
			db, err = gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			pool, err = db.DB()
			require.NoError(t, err)
			pool.SetMaxOpenConns(1)
			DB = db
			for _, original := range channels {
				var restored Channel
				require.NoError(t, db.First(&restored, original.Id).Error)
				assert.Equal(t, original.OtherSettings, restored.OtherSettings)
				assert.Equal(t, original.Models, restored.Models)
				assert.Equal(t, original.ModelMapping, restored.ModelMapping)
			}
			common.MemoryCacheEnabled = true
			require.NoError(t, InitChannelCache())
			for _, test := range []struct {
				name, model, endpoint string
				targets               []string
				rejected              int
				reason                string
			}{
				{name: "Chat endpoint", model: "public-model", endpoint: "/v1/chat/completions", targets: []string{"chat-target"}, rejected: 1, reason: "explicit_exact"},
				{name: "Responses with Codex fallback", model: "public-model", endpoint: "/v1/responses", targets: []string{"responses-target", "codex-target"}, reason: "explicit_exact"},
				{name: "exact mapping precedes Chat rule", model: "legacy-alias", endpoint: "/v1/chat/completions", targets: []string{"exact-target"}, reason: "explicit_mapping"},
				{name: "exact mapping precedes Responses rule", model: "legacy-alias", endpoint: "/v1/responses", targets: []string{"exact-target"}, reason: "explicit_mapping"},
				{name: "unsupported endpoint", model: "public-model", endpoint: "/v1/embeddings", rejected: 2},
				{name: "prefix does not enable an unlisted name", model: "unlisted-model", endpoint: "/v1/responses"},
				{name: "target ID does not become a public permission", model: "responses-target", endpoint: "/v1/responses"},
			} {
				t.Run(test.name, func(t *testing.T) {
					common.MemoryCacheEnabled = false
					database, err := GetRuntimeChannelRoutingPolicy(group, test.model, test.endpoint)
					require.NoError(t, err)
					assert.Equal(t, ChannelRoutingSourceDatabase, database.Source)
					common.MemoryCacheEnabled = true
					cached, err := GetRuntimeChannelRoutingPolicy(group, test.model, test.endpoint)
					require.NoError(t, err)
					assert.Equal(t, ChannelRoutingSourceCache, cached.Source)
					require.Equal(t, database.Policy, cached.Policy)
					require.Len(t, cached.Policy.Tiers, len(test.targets))
					require.Len(t, cached.Policy.Rejected, test.rejected)
					for _, rejected := range cached.Policy.Rejected {
						assert.Equal(t, ErrModelRouteEndpoint.Error(), rejected.RouteError)
						assert.Nil(t, rejected.ModelRoute)
					}
					for retry, target := range test.targets {
						tier := cached.Policy.Tiers[retry]
						require.Len(t, tier.Candidates, 1)
						route := tier.Candidates[0].ModelRoute
						require.NotNil(t, route)
						assert.Equal(t, test.model, route.RequestedModel)
						assert.Equal(t, target, route.UpstreamModel)
						assert.Equal(t, test.endpoint, route.Endpoint)
						assert.Equal(t, test.reason, route.Reason)
						assert.NotEmpty(t, route.ConfigDigest)
						for _, cacheEnabled := range []bool{false, true} {
							common.MemoryCacheEnabled = cacheEnabled
							selected, err := GetRandomSatisfiedChannel(group, test.model, retry, test.endpoint)
							require.NoError(t, err)
							require.NotNil(t, selected)
							assert.Equal(t, tier.Candidates[0].ChannelID, selected.Id)
							selectedRoute, err := ResolveChannelModelRoute(selected, test.model, test.endpoint)
							require.NoError(t, err)
							assert.Equal(t, *route, selectedRoute)
						}
					}
					if len(test.targets) == 0 {
						for _, cacheEnabled := range []bool{false, true} {
							common.MemoryCacheEnabled = cacheEnabled
							selected, err := GetRandomSatisfiedChannel(group, test.model, 0, test.endpoint)
							require.NoError(t, err)
							assert.Nil(t, selected)
						}
					}
				})
			}
			// Two editors start from the same committed routing snapshot.
			// The second save must not replace either the first rules or the
			// abilities published atomically with the first enabled models.
			var original Channel
			require.NoError(t, db.First(&original, channels[0].Id).Error)
			originalDigest := ChannelRoutingConfigDigest(&original)
			firstEditor, staleEditor := original, original
			firstEditor.Models = original.Models + ",editor-first"
			firstRules := append(append([]dto.ModelRoute{}, rules...), dto.ModelRoute{PublicModel: "editor-first", UpstreamModel: "first-editor-target", Match: "exact", Endpoint: "/v1/responses"})
			firstSettings, err := common.Marshal(dto.ChannelOtherSettings{ModelRoutes: firstRules})
			require.NoError(t, err)
			firstEditor.OtherSettings = string(firstSettings)
			require.NoError(t, firstEditor.UpdateWithRoutingConfig(originalDigest))
			require.NotEqual(t, originalDigest, ChannelRoutingConfigDigest(&firstEditor))
			var committedAbilities []Ability
			require.NoError(t, db.Where("channel_id = ?", original.Id).Order("model ASC").Find(&committedAbilities).Error)
			require.Len(t, committedAbilities, 3)
			assert.Equal(t, []string{"editor-first", "legacy-alias", "public-model"}, []string{committedAbilities[0].Model, committedAbilities[1].Model, committedAbilities[2].Model})
			for _, ability := range committedAbilities {
				assert.Equal(t, group, ability.Group)
				assert.True(t, ability.Enabled)
			}
			committedEpoch, err := GetCommittedChannelRoutingEpoch(db)
			require.NoError(t, err)
			staleEditor.Models = "public-model,editor-stale"
			staleSettings, err := common.Marshal(dto.ChannelOtherSettings{ModelRoutes: []dto.ModelRoute{{PublicModel: "editor-stale", UpstreamModel: "stale-editor-target", Match: "exact", Endpoint: "/v1/responses"}}})
			require.NoError(t, err)
			staleEditor.OtherSettings = string(staleSettings)
			require.ErrorIs(t, staleEditor.UpdateWithRoutingConfig(originalDigest), ErrModelRouteConfigChanged)
			var preserved Channel
			require.NoError(t, db.First(&preserved, original.Id).Error)
			assert.Equal(t, firstEditor.Models, preserved.Models)
			assert.Equal(t, firstEditor.OtherSettings, preserved.OtherSettings)
			assert.Equal(t, ChannelRoutingConfigDigest(&firstEditor), ChannelRoutingConfigDigest(&preserved))
			var preservedAbilities []Ability
			require.NoError(t, db.Where("channel_id = ?", original.Id).Order("model ASC").Find(&preservedAbilities).Error)
			assert.Equal(t, committedAbilities, preservedAbilities)
			afterRejectedEpoch, err := GetCommittedChannelRoutingEpoch(db)
			require.NoError(t, err)
			assert.Equal(t, committedEpoch, afterRejectedEpoch)
			common.MemoryCacheEnabled = true
			require.NoError(t, InitChannelCache())
			current, err := GetRuntimeChannelRoutingPolicy(group, "editor-first", "/v1/responses")
			require.NoError(t, err)
			require.Len(t, current.Policy.Tiers, 1)
			require.Len(t, current.Policy.Tiers[0].Candidates, 1)
			require.NotNil(t, current.Policy.Tiers[0].Candidates[0].ModelRoute)
			assert.Equal(t, "first-editor-target", current.Policy.Tiers[0].Candidates[0].ModelRoute.UpstreamModel)
			rejectedEditor, err := GetRuntimeChannelRoutingPolicy(group, "editor-stale", "/v1/responses")
			require.NoError(t, err)
			assert.Empty(t, rejectedEditor.Policy.Tiers)

		})
	}
}
