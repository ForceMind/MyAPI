package model

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestRelayAccountHoldConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialect gorm.Dialector = sqlite.Open(t.TempDir() + "/holds.db")
			if engine.env != "" {
				if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
					t.Skip("requires explicitly configured disposable B2 database")
				}
				var err error
				dialect, err = b2SubmissionDatabaseDialector(engine.name, strings.TrimSpace(os.Getenv(engine.env)))
				require.NoError(t, err)
			}
			cfg := &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("rh_%d_", time.Now().UnixNano())}}
			db, err := gorm.Open(dialect, cfg)
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&RelayAccountHold{}, &Channel{}))
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&Channel{}, &RelayAccountHold{}))
			require.NoError(t, db.AutoMigrate(&Channel{}, &RelayAccountHold{}))
			channel := &Channel{Id: 1, Type: 1, Key: "synthetic-one", Status: common.ChannelStatusEnabled, Models: "model", Group: "default"}
			require.NoError(t, db.Create(channel).Error)
			ctx := context.Background()
			identity := ChannelQuotaAccountRef("test", "synthetic-one")
			recorded, err := RecordRelayAccountHold(ctx, db, channel, channel.Key, identity, 240)
			require.NoError(t, err)
			require.True(t, recorded)
			holds, err := ReadRelayAccountHolds(ctx, db, channel)
			require.NoError(t, err)
			require.Len(t, holds, 1)
			first := holds[0].Until
			recorded, err = RecordRelayAccountHold(ctx, db, channel, channel.Key, identity, 1)
			require.NoError(t, err)
			require.True(t, recorded)
			holds, err = ReadRelayAccountHolds(ctx, db, channel)
			require.NoError(t, err)
			require.Len(t, holds, 1)
			assert.Equal(t, first, holds[0].Until, "a late shorter hold cannot shorten the current hold")
			// Parallel updates serialize and the longer committed expiry wins.
			var wg sync.WaitGroup
			start := make(chan struct{})
			results := make(chan error, 2)
			parallelNow, err := ReadDatabaseUnixTime(ctx, db)
			require.NoError(t, err)
			for _, seconds := range []int{1, 300} {
				wg.Add(1)
				go func(seconds int) {
					defer wg.Done()
					<-start
					_, err := RecordRelayAccountHold(ctx, db, channel, channel.Key, identity, seconds)
					results <- err
				}(seconds)
			}
			close(start)
			wg.Wait()
			close(results)
			for err := range results {
				require.NoError(t, err)
			}
			holds, err = ReadRelayAccountHolds(ctx, db, channel)
			require.NoError(t, err)
			require.Len(t, holds, 1)
			assert.GreaterOrEqual(t, holds[0].Until, parallelNow+300)
			first = holds[0].Until
			require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("name", "renamed display").Error)
			holds, err = ReadRelayAccountHolds(ctx, db, channel)
			require.NoError(t, err)
			require.Len(t, holds, 1)
			// A fresh connection (restart) observes the persisted hold without extending it.
			reopened, err := gorm.Open(dialect, cfg)
			require.NoError(t, err)
			reopenedSQL, err := reopened.DB()
			require.NoError(t, err)
			defer reopenedSQL.Close()
			holds, err = ReadRelayAccountHolds(ctx, reopened, channel)
			require.NoError(t, err)
			require.Len(t, holds, 1)
			assert.Equal(t, first, holds[0].Until)
			routed := *channel
			routed.OtherSettings = `{"model_routes":[{"public_model":"public","upstream_model":"a","match":"exact"}]}`
			require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("settings", `{"model_routes":[{"public_model":"public","upstream_model":"b","match":"exact"}]}`).Error)
			recorded, err = RecordRelayAccountHold(ctx, db, &routed, routed.Key, identity, 300)
			require.NoError(t, err)
			assert.False(t, recorded, "old failure must not hold a replacement model route")
			require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("settings", "").Error)
			require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("key", "replacement").Error)
			recorded, err = RecordRelayAccountHold(ctx, db, channel, channel.Key, identity, 300)
			require.NoError(t, err)
			assert.False(t, recorded, "late failure of replaced credential must not add a hold")
			require.NoError(t, db.Model(&RelayAccountHold{}).Where("channel_id = ?", channel.Id).Update("hold_until", 1).Error)
			holds, err = ReadRelayAccountHolds(ctx, db, channel)
			require.NoError(t, err)
			assert.Empty(t, holds)
			removed, err := CleanupRelayAccountHolds(ctx, db, 1)
			require.NoError(t, err)
			assert.EqualValues(t, 1, removed)
			removed, err = CleanupRelayAccountHolds(ctx, db, 1)
			require.NoError(t, err)
			assert.Zero(t, removed)
			encoded, err := common.Marshal(RelayAccountHold{ChannelID: 1, Identity: identity, Until: 100})
			require.NoError(t, err)
			assert.JSONEq(t, "{}", string(encoded))
		})
	}
}

func TestRelayHoldConfigDigestIsPureAndIgnoresDisplayObservations(t *testing.T) {
	channel := &Channel{Type: 1, Name: "before", OtherSettings: `{"model_routes":[{"public_model":"public","upstream_model":"a","match":"exact"}]}`}
	original := RelayChannelConfigDigest(channel)
	require.NotEmpty(t, original)
	channel.Name = "after"
	channel.TestTime = 100
	channel.UsedQuota = 1000
	channel.OtherSettings = `{"upstream_model_update_last_check_time":100,"model_routes":[{"public_model":"public","upstream_model":"a","match":"exact"}]}`
	assert.Equal(t, original, RelayChannelConfigDigest(channel))
	channel.OtherSettings = `{"model_routes":[{"public_model":"public","upstream_model":"b","match":"exact"}]}`
	assert.NotEqual(t, original, RelayChannelConfigDigest(channel))
	old := DB
	DB = nil
	t.Cleanup(func() { DB = old })
	channel.OtherSettings = `{broken`
	assert.Empty(t, RelayChannelConfigDigest(channel))
	assert.Equal(t, `{broken`, channel.OtherSettings)
	channel.OtherSettings = ""
	channel.Setting = common.GetPointer(`{broken`)
	assert.Empty(t, RelayChannelConfigDigest(channel))
	assert.Equal(t, `{broken`, *channel.Setting)
}
