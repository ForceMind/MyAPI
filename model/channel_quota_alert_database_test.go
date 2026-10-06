package model

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestChannelQuotaAlertConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			path := t.TempDir() + "/events.db"
			newDialect := func() gorm.Dialector {
				if engine.env == "" {
					return sqlite.Open(path)
				}
				if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
					t.Skip("requires explicitly configured disposable B2 database")
				}
				dialect, err := b2SubmissionDatabaseDialector(engine.name, strings.TrimSpace(os.Getenv(engine.env)))
				require.NoError(t, err)
				return dialect
			}
			cfg := &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("qe_%d_", time.Now().UnixNano())}}
			db, err := gorm.Open(newDialect(), cfg)
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			// SQLite write serialization is an explicit fixture property; external
			// engines retain multiple connections to exercise the channel-row lock.
			if engine.name == "sqlite" {
				sqlDB.SetMaxOpenConns(1)
			}
			require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSnapshot{}, &ChannelQuotaAlertState{}, &ChannelQuotaAlertEvent{}))
			previousDB, previousSettings := DB, channelQuotaAlertSettings()
			DB = db
			common.ChannelQuotaAlertEnabled, common.ChannelQuotaAlertWarningPercent, common.ChannelQuotaAlertCriticalPercent = true, 20, 10
			common.ChannelQuotaAlertCooldownSeconds, common.ChannelQuotaAlertNotifyOnRecovery = 60, true
			channel := Channel{Type: 57, Key: "synthetic-only", Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(&channel).Error)
			t.Cleanup(func() {
				DB = previousDB
				common.ChannelQuotaAlertEnabled, common.ChannelQuotaAlertWarningPercent, common.ChannelQuotaAlertCriticalPercent = previousSettings.Enabled, previousSettings.WarningPercent, previousSettings.CriticalPercent
				common.ChannelQuotaAlertCooldownSeconds, common.ChannelQuotaAlertNotifyOnRecovery = previousSettings.CooldownSeconds, previousSettings.NotifyOnRecovery
				require.NoError(t, db.Where("channel_id = ?", channel.Id).Delete(&ChannelQuotaSnapshot{}).Error)
				require.NoError(t, db.Migrator().DropTable(&ChannelQuotaAlertEvent{}, &ChannelQuotaAlertState{}, &Channel{}))
				require.NoError(t, sqlDB.Close())
			})
			total := 100.0
			ref := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
			point := func(sample string, at int64, available float64) ChannelQuotaSnapshot {
				return ChannelQuotaSnapshot{ChannelId: channel.Id, SubjectRef: ref, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, SampleID: sample, ObservedAt: at, Available: available, Total: &total, CodexObservationQualified: true, CodexThresholdQualified: true, MetricType: "codex_rate_limit", WindowType: "five_hour", WindowSeconds: 18000, ResetAt: 19000, Unit: "percent", Source: "codex_wham_usage_primary", Status: "success"}
			}
			ctx := context.Background()
			start := make(chan struct{})
			results := make(chan error, 2)
			var workers sync.WaitGroup
			for range 2 {
				workers.Add(1)
				go func() {
					defer workers.Done()
					<-start
					results <- RecordChannelQuotaSnapshotBatchWithContext(ctx, []ChannelQuotaSnapshot{point("low", 1000, 5)}, ChannelQuotaSnapshotBatchOptions{})
				}()
			}
			close(start)
			workers.Wait()
			close(results)
			for result := range results {
				require.NoError(t, result)
			}
			var events []ChannelQuotaAlertEvent
			require.NoError(t, db.Order("id ASC").Find(&events).Error)
			require.Len(t, events, 1, "concurrent identical observation must not duplicate its event")
			for _, p := range []ChannelQuotaSnapshot{point("zero", 1000, 0), point("same", 1001, 0), point("recovered", 1002, 70)} {
				require.NoError(t, RecordChannelQuotaSnapshotBatchWithContext(ctx, []ChannelQuotaSnapshot{p}, ChannelQuotaSnapshotBatchOptions{}))
			}
			// A second account cannot recover the first; failure and missing-total
			// samples must not create an event or rewrite its trusted state.
			other := point("other-account", 1004, 80)
			other.SubjectRef = base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("b", 32)))
			failed := point("failed", 1005, 100)
			failed.Status = "error"
			unknown := point("unknown", 1006, 100)
			unknown.Total = nil
			unqualified := point("unqualified", 1007, 0)
			unqualified.CodexObservationQualified = false
			expired := point("expired", 1008, 0)
			expired.ResetAt = 1008
			unknownAccount := point("credential-only", 1009, 0)
			unknownAccount.IdentityQuality = ChannelQuotaIdentityQualityCredentialScoped
			for _, p := range []ChannelQuotaSnapshot{other, failed, unknown, unqualified, expired, unknownAccount} {
				require.NoError(t, RecordChannelQuotaSnapshotBatchWithContext(ctx, []ChannelQuotaSnapshot{p}, ChannelQuotaSnapshotBatchOptions{}))
			}
			require.NoError(t, db.Order("id ASC").Find(&events).Error)
			require.Len(t, events, 3)
			require.Equal(t, "exhausted", events[1].Status)
			require.Equal(t, "recovery", events[2].Kind)
			require.Equal(t, events[0].SeriesKey, events[2].SeriesKey)
			require.NoError(t, db.AutoMigrate(&ChannelQuotaAlertState{}, &ChannelQuotaAlertEvent{}), "migration must preserve immutable evidence on restart")
			// Snapshot retention cannot erase the event's in-app meaning.
			require.NoError(t, db.Where("channel_id = ?", channel.Id).Delete(&ChannelQuotaSnapshot{}).Error)
			reopened, err := gorm.Open(newDialect(), cfg)
			require.NoError(t, err)
			reopenedSQL, err := reopened.DB()
			require.NoError(t, err)
			var persisted ChannelQuotaAlertEvent
			require.NoError(t, reopened.First(&persisted, events[1].ID).Error)
			evidence := ReadChannelQuotaAlertEvidence(ctx, persisted)
			require.NotNil(t, evidence)
			require.Zero(t, evidence.Available)
			require.Equal(t, int64(19000), evidence.ResetAt)
			require.NoError(t, reopenedSQL.Close())
			require.NoError(t, sqlDB.Ping(), "reopen must own an independent connection pool")
			claims, err := ClaimChannelQuotaAlertEvents(ctx, "synthetic-worker", 2000, 30, 1)
			require.NoError(t, err)
			require.Len(t, claims, 1)
			won, err := MarkChannelQuotaAlertFailed(ctx, claims[0], "synthetic-worker", "network_error", 2001)
			require.NoError(t, err)
			require.True(t, won)
			won, err = MarkChannelQuotaAlertDelivered(ctx, claims[0], "synthetic-worker", 2002)
			require.NoError(t, err)
			require.False(t, won, "stale claim cannot complete a retry")
			var failedEvent ChannelQuotaAlertEvent
			require.NoError(t, db.First(&failedEvent, claims[0].ID).Error)
			require.Equal(t, "retryable", failedEvent.State)
			require.Equal(t, persisted.EvidenceJSON, events[1].EvidenceJSON)
		})
	}
}
