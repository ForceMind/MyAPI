package model

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestChannelQuotaIdentityConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct {
		name   string
		dsnEnv string
	}{
		{name: "sqlite"},
		{name: "mysql", dsnEnv: "MYAPI_B2_MYSQL_DSN"},
		{name: "postgres", dsnEnv: "MYAPI_B2_POSTGRES_DSN"},
	} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := filepath.Join(t.TempDir(), "quota-identity.db")
			var dialector gorm.Dialector = sqlite.Open(dsn)
			if engine.dsnEnv != "" {
				if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
					t.Skip("external database contract requires MYAPI_B2_DATABASE_TESTS=1")
				}
				dsn = strings.TrimSpace(os.Getenv(engine.dsnEnv))
				require.NotEmpty(t, dsn, "%s must be configured for this database contract", engine.dsnEnv)
				var err error
				dialector, err = b2SubmissionDatabaseDialector(engine.name, dsn)
				require.NoError(t, err, "external quota identity tests require the disposable loopback B2 database")
			}
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Ping())
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

			models := []any{
				&Channel{}, &ChannelQuotaSnapshot{},
				&ChannelQuotaIdentityKeyRegistry{}, &ChannelQuotaIdentityKeyVersion{},
				&ChannelQuotaIdentityAlias{}, &ChannelQuotaSamplingTarget{},
			}
			require.NoError(t, db.AutoMigrate(models...))
			require.NoError(t, db.AutoMigrate(models...), "identity migrations must be repeatable")
			require.True(t, db.Migrator().HasColumn(&ChannelQuotaSnapshot{}, "subject_ref"))
			require.True(t, db.Migrator().HasColumn(&Channel{}, "quota_sampling_cursor"))
			require.True(t, db.Migrator().HasIndex(&ChannelQuotaIdentityAlias{}, "uidx_channel_quota_identity_alias"))
			require.True(t, db.Migrator().HasIndex(&ChannelQuotaSamplingTarget{}, "uidx_channel_quota_sampling_target"))

			ctx := context.Background()
			v1 := common.ChannelQuotaIdentityKeyring{Active: common.ChannelQuotaIdentityKey{
				Version: "quota-v1", Secret: []byte(strings.Repeat("q", 32)),
			}}
			require.NoError(t, EnsureChannelQuotaIdentityKeyring(ctx, db, v1))
			identities := make([]ChannelQuotaResolvedIdentity, 2)
			for index, credential := range []string{"credential-a", "credential-b"} {
				identities[index], err = ResolveChannelQuotaIdentity(ctx, db, v1, "channel_type_57", common.ChannelQuotaIdentityKindCredential, []byte(credential))
				require.NoError(t, err)
			}
			require.NotEqual(t, identities[0].SubjectRef, identities[1].SubjectRef)
			v2 := common.ChannelQuotaIdentityKeyring{
				Active:  common.ChannelQuotaIdentityKey{Version: "quota-v2", Secret: []byte(strings.Repeat("r", 32))},
				Retired: []common.ChannelQuotaIdentityKey{v1.Active},
			}
			require.NoError(t, EnsureChannelQuotaIdentityKeyring(ctx, db, v2))
			rotated, err := ResolveChannelQuotaIdentity(ctx, db, v2, "channel_type_57", common.ChannelQuotaIdentityKindCredential, []byte("credential-a"))
			require.NoError(t, err)
			require.Equal(t, identities[0].SubjectRef, rotated.SubjectRef)

			previousDB := DB
			DB = db
			t.Cleanup(func() { DB = previousDB })
			channel := Channel{Name: "quota-identity-contract-" + engine.name, Type: 57, Key: "test-only"}
			require.NoError(t, db.Create(&channel).Error)
			t.Cleanup(func() {
				_ = db.Where("channel_id = ?", channel.Id).Delete(&ChannelQuotaSamplingTarget{}).Error
				_ = db.Where("channel_id = ?", channel.Id).Delete(&ChannelQuotaSnapshot{}).Error
				_ = db.Where("id = ?", channel.Id).Delete(&Channel{}).Error
			})
			targetIdentities := []ChannelQuotaSamplingIdentity{
				{SubjectRef: identities[1].SubjectRef, IdentityQuality: identities[1].Quality},
				{SubjectRef: identities[0].SubjectRef, IdentityQuality: identities[0].Quality},
			}
			targets, err := SyncChannelQuotaSamplingTargets(ctx, db, channel.Id, channel.Key, true, targetIdentities)
			require.NoError(t, err)
			require.Len(t, targets, 2)
			targets, err = SyncChannelQuotaSamplingTargets(ctx, db, channel.Id, channel.Key, true, []ChannelQuotaSamplingIdentity{targetIdentities[1], targetIdentities[0]})
			require.NoError(t, err)
			require.Len(t, targets, 2, "reordering credentials must not create or retire a target")
			_, err = SyncChannelQuotaSamplingTargets(ctx, db, channel.Id, channel.Key, true, targetIdentities,
				ChannelQuotaSamplingExpansion{ExpectedCursor: 0, NextCursor: 32})
			require.NoError(t, err)
			_, err = SyncChannelQuotaSamplingTargets(ctx, db, channel.Id, channel.Key, true, targetIdentities,
				ChannelQuotaSamplingExpansion{ExpectedCursor: 0, NextCursor: 64})
			require.Error(t, err, "a stale expansion cursor must fail on every dialect")
			var cursorChannel Channel
			require.NoError(t, db.First(&cursorChannel, channel.Id).Error)
			require.EqualValues(t, 32, cursorChannel.QuotaSamplingCursor)
			require.NoError(t, MarkChannelQuotaSamplingTargetAttempt(ctx, db, channel.Id, identities[0].SubjectRef, "sampled", ""))
			var attempted ChannelQuotaSamplingTarget
			require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", channel.Id, identities[0].SubjectRef).First(&attempted).Error)
			require.Equal(t, "sampled", attempted.LastResult)
			observedAt := time.Now().Unix()
			for index, identity := range identities {
				for step := 0; step < 2; step++ {
					require.NoError(t, db.Create(&ChannelQuotaSnapshot{
						ChannelId: channel.Id, SubjectRef: identity.SubjectRef,
						IdentityQuality: identity.Quality, ObservedAt: observedAt + int64(step),
						Available: float64(90 - index*20 - step*10), Unit: "percent",
						MetricType: "codex_rate_limit", WindowType: "weekly",
						Source: "codex_wham_usage_primary", Status: "success",
					}).Error)
				}
			}
			rows, err := ListChannelQuotaAggregateRows(ctx, observedAt, observedAt+1, []int{channel.Id}, "codex_rate_limit", "weekly", "")
			require.NoError(t, err)
			require.Len(t, rows, 4)
			counts := make(map[string]int)
			for _, row := range rows {
				counts[row.SubjectRef]++
			}
			require.Equal(t, 2, counts[identities[0].SubjectRef])
			require.Equal(t, 2, counts[identities[1].SubjectRef])
			encoded, err := common.Marshal(rows)
			require.NoError(t, err)
			for _, identity := range identities {
				require.NotContains(t, string(encoded), identity.SubjectRef)
			}
			confirmed, err := ResolveChannelQuotaIdentity(ctx, db, v2, "channel_type_57", common.ChannelQuotaIdentityKindProviderAccount, []byte("account-a"))
			require.NoError(t, err)
			recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
				ChannelId: channel.Id, SubjectRef: confirmed.SubjectRef, IdentityQuality: confirmed.Quality,
				ObservedAt: observedAt, Available: 0, Source: codexQuotaWindowPrimarySource, ResetAt: observedAt + 3600,
			})
			recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
				ChannelId: channel.Id, SubjectRef: confirmed.SubjectRef, IdentityQuality: confirmed.Quality,
				ObservedAt: observedAt + 1, Status: "error", Source: CodexQuotaRouteLimitSource, ErrorCode: CodexQuotaRouteLimitCode,
			})
			state, err := ReadCodexQuotaRouteState(ctx, db, confirmed.SubjectRef, "", observedAt+2)
			require.NoError(t, err)
			require.True(t, state.Blocked)
			_, err = DeleteOldChannelQuotaSnapshotBatch(ctx, observedAt+2, 100)
			require.NoError(t, err)
			state, err = ReadCodexQuotaRouteState(ctx, db, confirmed.SubjectRef, "", observedAt+2)
			require.NoError(t, err)
			require.True(t, state.Blocked, "retention must preserve the latest exhaustion facts")
		})
	}
}
