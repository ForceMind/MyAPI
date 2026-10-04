package model

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestChannelModelDiscoveryConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/discovery.db")
			if engine.env != "" {
				if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
					t.Skip("requires explicitly configured disposable loopback B2 database")
				}
				dsn := strings.TrimSpace(os.Getenv(engine.env))
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
			oldDB := DB
			oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
			DB = db
			common.SetDatabaseTypes(common.DatabaseType(engine.name), common.DatabaseType(engine.name))
			t.Cleanup(func() { DB = oldDB; common.SetDatabaseTypes(oldMain, oldLog); require.NoError(t, pool.Close()) })
			require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelModelDiscovery{}))
			require.NoError(t, db.AutoMigrate(&ChannelModelDiscovery{}))
			channel := Channel{Type: constant.ChannelTypeOpenAI, Key: "synthetic-private-key", Models: "manual-route"}
			require.NoError(t, db.Create(&channel).Error)
			t.Cleanup(func() {
				require.NoError(t, db.Where("channel_id = ?", channel.Id).Delete(&ChannelModelDiscovery{}).Error)
				require.NoError(t, db.Delete(&Channel{}, channel.Id).Error)
			})
			ctx := context.Background()
			view, err := GetChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			assert.Equal(t, "never_checked", view.Status)
			assert.Empty(t, view.Models)
			assert.True(t, view.Stale)
			first, err := BeginChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			applied, err := CompleteChannelModelDiscovery(ctx, first, []string{"b", "a", "a", " "}, true)
			require.NoError(t, err)
			require.True(t, applied)
			view, err = GetChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			assert.Equal(t, []string{"a", "b"}, view.Models)
			assert.False(t, view.Stale)
			assert.Equal(t, "success", view.Status)

			// Fixed fixture times prove replay performs no timestamp rewrite,
			// without sleeping or depending on same-second execution.
			expiredAt := time.Now().Unix() - ChannelModelDiscoveryFreshnessSeconds - 1
			require.NoError(t, db.Model(&ChannelModelDiscovery{}).Where("channel_id = ?", channel.Id).Updates(map[string]any{"fetched_at": expiredAt, "checked_at": expiredAt + 1}).Error)
			beforeReplay, err := GetChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			assert.True(t, beforeReplay.Stale, "successful evidence expires after the display freshness window")
			assert.Equal(t, "success", beforeReplay.Status)
			assert.Equal(t, "openai_models", beforeReplay.Source)
			assert.Equal(t, []string{"a", "b"}, beforeReplay.Models)
			assert.Equal(t, expiredAt, beforeReplay.FetchedAt)
			assert.Equal(t, expiredAt+1, beforeReplay.CheckedAt)
			applied, err = CompleteChannelModelDiscovery(ctx, first, []string{"b", "a"}, true)
			require.NoError(t, err)
			assert.True(t, applied)
			applied, err = CompleteChannelModelDiscovery(ctx, first, []string{"changed-replay"}, true)
			require.NoError(t, err)
			assert.False(t, applied)
			applied, err = CompleteChannelModelDiscovery(ctx, first, nil, false)
			require.NoError(t, err)
			assert.False(t, applied)
			view, err = GetChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			assert.Equal(t, beforeReplay, view)
			fetchedAt := view.FetchedAt
			failed, err := BeginChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			applied, err = CompleteChannelModelDiscovery(ctx, failed, nil, false)
			require.NoError(t, err)
			require.True(t, applied)
			view, err = GetChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			assert.Equal(t, []string{"a", "b"}, view.Models)
			assert.True(t, view.Stale)
			assert.Equal(t, "failed", view.Status)
			assert.Equal(t, fetchedAt, view.FetchedAt)

			require.NoError(t, db.Model(&ChannelModelDiscovery{}).Where("channel_id = ?", channel.Id).Update("checked_at", int64(125)).Error)
			failedBeforeReplay, err := GetChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			applied, err = CompleteChannelModelDiscovery(ctx, failed, nil, false)
			require.NoError(t, err)
			assert.True(t, applied)
			applied, err = CompleteChannelModelDiscovery(ctx, failed, []string{"late-success"}, true)
			require.NoError(t, err)
			assert.False(t, applied)
			view, err = GetChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			assert.Equal(t, failedBeforeReplay, view)
			older, err := BeginChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			newer, err := BeginChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			applied, err = CompleteChannelModelDiscovery(ctx, newer, []string{"new"}, true)
			require.NoError(t, err)
			require.True(t, applied)
			applied, err = CompleteChannelModelDiscovery(ctx, older, []string{"old"}, true)
			require.NoError(t, err)
			assert.False(t, applied)
			view, err = GetChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			assert.Equal(t, []string{"new"}, view.Models)
			changed, err := BeginChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("key", "replacement-private-key").Error)
			applied, err = CompleteChannelModelDiscovery(ctx, changed, []string{"obsolete"}, true)
			require.NoError(t, err)
			assert.False(t, applied)
			view, err = GetChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			assert.Equal(t, "configuration_changed", view.Status)
			assert.True(t, view.Stale)
			assert.Equal(t, []string{"new"}, view.Models)
			empty, err := BeginChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			applied, err = CompleteChannelModelDiscovery(ctx, empty, nil, true)
			require.NoError(t, err)
			require.True(t, applied)
			view, err = GetChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			assert.Equal(t, "empty", view.Status)
			assert.False(t, view.Stale)
			assert.Equal(t, []string{}, view.Models)
			// Startup migration preserves evidence, and metadata never changes routes.
			require.NoError(t, db.AutoMigrate(&ChannelModelDiscovery{}))
			restarted, err := GetChannelModelDiscovery(ctx, channel.Id)
			require.NoError(t, err)
			assert.Equal(t, view, restarted)
			require.NoError(t, db.First(&channel, channel.Id).Error)
			assert.Equal(t, "manual-route", channel.Models)
			var row ChannelModelDiscovery
			require.NoError(t, db.First(&row, "channel_id = ?", channel.Id).Error)
			persisted, err := common.Marshal(row)
			require.NoError(t, err)
			assert.NotContains(t, string(persisted), "private-key")
		})
	}
}
