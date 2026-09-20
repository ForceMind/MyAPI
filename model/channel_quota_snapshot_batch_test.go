package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func codexQuotaBatchOptionsForTest() ChannelQuotaSnapshotBatchOptions {
	return ChannelQuotaSnapshotBatchOptions{
		PreviousSources: []string{"codex_wham_usage_primary", "codex_wham_usage_secondary"},
		AbsenceMarker: &ChannelQuotaSnapshotAbsenceMarker{
			Status:       "unsupported",
			ErrorCode:    "window_absent",
			ErrorMessage: "rate limit window absent from latest successful usage response",
		},
	}
}

func codexQuotaSnapshotForTest(channelID int, observedAt int64, sampleID, source, planType, windowType string, windowSeconds int64, used float64) ChannelQuotaSnapshot {
	available := 100 - used
	total := float64(100)
	return ChannelQuotaSnapshot{
		ChannelId: channelID, ObservedAt: observedAt, SampleID: sampleID,
		Available: available, Used: &used, Total: &total, Unit: "percent",
		MetricType: "codex_rate_limit", WindowType: windowType, WindowSeconds: windowSeconds,
		PlanType: planType, Source: source, Status: "success",
	}
}

func setupChannelQuotaBatchSQLite(t *testing.T, channelID int) *gorm.DB {
	t.Helper()
	previousDB := DB
	path := filepath.Join(t.TempDir(), "channel-quota-batch.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSnapshot{}))
	DB = db
	require.NoError(t, ensureChannelQuotaSnapshotDedupeIndex())
	t.Cleanup(func() { DB = previousDB })
	require.NoError(t, db.Create(&Channel{Id: channelID, Type: constant.ChannelTypeCodex, Name: "codex-batch-test", Key: "test"}).Error)
	return db
}

func TestRecordChannelQuotaSnapshotBatchRollsBackPartialWriteAndRetriesSameSample(t *testing.T) {
	db := setupChannelQuotaBatchSQLite(t, 911)
	options := codexQuotaBatchOptionsForTest()
	observedAt := time.Now().Unix()
	previous := []ChannelQuotaSnapshot{
		codexQuotaSnapshotForTest(911, observedAt-1, "sample-old", "codex_wham_usage_primary", "free", "five_hour", 18000, 20),
		codexQuotaSnapshotForTest(911, observedAt-1, "sample-old", "codex_wham_usage_secondary", "free", "weekly", 604800, 40),
	}
	require.NoError(t, RecordChannelQuotaSnapshotBatchWithContext(context.Background(), previous, options))
	current := []ChannelQuotaSnapshot{
		codexQuotaSnapshotForTest(911, observedAt, "sample-new", "codex_wham_usage_primary", "plus", "weekly", 604800, 30),
	}

	createCount := 0
	callbackName := "test:fail_second_channel_quota_batch_create"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		snapshot, ok := tx.Statement.Dest.(*ChannelQuotaSnapshot)
		if !ok || snapshot.SampleID != "sample-new" {
			return
		}
		createCount++
		if createCount == 2 {
			tx.AddError(errors.New("forced channel quota batch item failure"))
		}
	}))
	err := RecordChannelQuotaSnapshotBatchWithContext(context.Background(), current, options)
	require.ErrorContains(t, err, "forced channel quota batch item failure")
	require.NoError(t, db.Callback().Create().Remove(callbackName))

	var count int64
	require.NoError(t, db.Model(&ChannelQuotaSnapshot{}).Where("channel_id = ? AND sample_id = ?", 911, "sample-new").Count(&count).Error)
	require.Zero(t, count)

	require.NoError(t, RecordChannelQuotaSnapshotBatchWithContext(context.Background(), current, options))
	require.NoError(t, db.Model(&ChannelQuotaSnapshot{}).Where("channel_id = ? AND sample_id = ?", 911, "sample-new").Count(&count).Error)
	require.Equal(t, int64(3), count)
	latest, err := ListLatestChannelQuotaSnapshotBatch(context.Background(), 911, observedAt-2, observedAt+1, "codex_rate_limit")
	require.NoError(t, err)
	require.Len(t, latest, 3)
	for _, snapshot := range latest {
		require.Equal(t, "sample-new", snapshot.SampleID)
	}
}

func TestRecordChannelQuotaSnapshotBatchLookupFailureWritesNothing(t *testing.T) {
	db := setupChannelQuotaBatchSQLite(t, 912)
	options := codexQuotaBatchOptionsForTest()
	observedAt := time.Now().Unix()
	previous := []ChannelQuotaSnapshot{
		codexQuotaSnapshotForTest(912, observedAt-1, "sample-old", "codex_wham_usage_primary", "free", "five_hour", 18000, 20),
	}
	require.NoError(t, RecordChannelQuotaSnapshotBatchWithContext(context.Background(), previous, options))
	current := []ChannelQuotaSnapshot{
		codexQuotaSnapshotForTest(912, observedAt, "sample-new", "codex_wham_usage_primary", "plus", "weekly", 604800, 30),
	}

	queryCount := 0
	callbackName := "test:fail_channel_quota_previous_lookup"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "channel_quota_snapshots" {
			return
		}
		queryCount++
		if queryCount == 2 {
			tx.AddError(errors.New("forced previous sample lookup failure"))
		}
	}))
	err := RecordChannelQuotaSnapshotBatchWithContext(context.Background(), current, options)
	require.ErrorContains(t, err, "forced previous sample lookup failure")
	require.NoError(t, db.Callback().Query().Remove(callbackName))

	var count int64
	require.NoError(t, db.Model(&ChannelQuotaSnapshot{}).Where("channel_id = ? AND sample_id = ?", 912, "sample-new").Count(&count).Error)
	require.Zero(t, count)
}

func TestRecordChannelQuotaSnapshotBatchConcurrentSameSampleIsIdempotent(t *testing.T) {
	db := setupChannelQuotaBatchSQLite(t, 913)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	options := codexQuotaBatchOptionsForTest()
	observedAt := time.Now().Unix()
	previous := []ChannelQuotaSnapshot{
		codexQuotaSnapshotForTest(913, observedAt-1, "sample-old", "codex_wham_usage_primary", "free", "five_hour", 18000, 20),
		codexQuotaSnapshotForTest(913, observedAt-1, "sample-old", "codex_wham_usage_secondary", "free", "weekly", 604800, 40),
	}
	require.NoError(t, RecordChannelQuotaSnapshotBatchWithContext(context.Background(), previous, options))
	current := []ChannelQuotaSnapshot{
		codexQuotaSnapshotForTest(913, observedAt, "sample-concurrent", "codex_wham_usage_primary", "plus", "weekly", 604800, 30),
	}

	const workers = 8
	start := make(chan struct{})
	errorsCh := make(chan error, workers)
	var workersWG sync.WaitGroup
	workersWG.Add(workers)
	for range workers {
		go func() {
			defer workersWG.Done()
			<-start
			errorsCh <- RecordChannelQuotaSnapshotBatchWithContext(context.Background(), current, options)
		}()
	}
	close(start)
	workersWG.Wait()
	close(errorsCh)
	for err := range errorsCh {
		require.NoError(t, err)
	}

	var rows []ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ? AND sample_id = ?", 913, "sample-concurrent").Order("id ASC").Find(&rows).Error)
	require.Len(t, rows, 3)
	require.Equal(t, "success", rows[0].Status)
	require.Equal(t, "window_absent", rows[1].ErrorCode)
	require.Equal(t, "window_absent", rows[2].ErrorCode)
}

func TestRecordChannelQuotaSnapshotBatchRejectsConflictingSampleReplay(t *testing.T) {
	setupChannelQuotaBatchSQLite(t, 914)
	options := codexQuotaBatchOptionsForTest()
	observedAt := time.Now().Unix()
	first := []ChannelQuotaSnapshot{
		codexQuotaSnapshotForTest(914, observedAt, "sample-conflict", "codex_wham_usage_primary", "team", "five_hour", 18000, 10),
	}
	require.NoError(t, RecordChannelQuotaSnapshotBatchWithContext(context.Background(), first, options))
	conflict := []ChannelQuotaSnapshot{
		codexQuotaSnapshotForTest(914, observedAt, "sample-conflict", "codex_wham_usage_primary", "team", "five_hour", 18000, 40),
	}
	require.ErrorContains(t, RecordChannelQuotaSnapshotBatchWithContext(context.Background(), conflict, options), "sample_id conflicts")
}

func TestChannelQuotaSnapshotBatchConfiguredMySQLAndPostgres(t *testing.T) {
	for _, database := range []struct {
		name      string
		envName   string
		dialector func(string) gorm.Dialector
	}{
		{name: "mysql", envName: "TEST_MYSQL_DSN", dialector: func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{name: "postgres", envName: "TEST_POSTGRES_DSN", dialector: func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
	} {
		t.Run(database.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(database.envName))
			if dsn == "" {
				t.Skipf("%s is not configured; %s batch transaction integration was not run", database.envName, database.name)
			}
			db, err := gorm.Open(database.dialector(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Ping())
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSnapshot{}))

			channelID := int(1_500_000_000 + time.Now().UnixNano()%100_000_000)
			require.NoError(t, db.Create(&Channel{Id: channelID, Type: constant.ChannelTypeCodex, Name: fmt.Sprintf("codex-batch-%s", database.name), Key: "test"}).Error)
			t.Cleanup(func() {
				_ = db.Where("channel_id = ?", channelID).Delete(&ChannelQuotaSnapshot{}).Error
				_ = db.Where("id = ?", channelID).Delete(&Channel{}).Error
			})
			previousDB := DB
			DB = db
			t.Cleanup(func() { DB = previousDB })

			observedAt := time.Now().Unix()
			options := codexQuotaBatchOptionsForTest()
			previous := []ChannelQuotaSnapshot{
				codexQuotaSnapshotForTest(channelID, observedAt, "sample-old", "codex_wham_usage_primary", "free", "five_hour", 18000, 20),
				codexQuotaSnapshotForTest(channelID, observedAt, "sample-old", "codex_wham_usage_secondary", "free", "weekly", 604800, 40),
			}
			require.NoError(t, RecordChannelQuotaSnapshotBatchWithContext(context.Background(), previous, options))
			current := []ChannelQuotaSnapshot{
				codexQuotaSnapshotForTest(channelID, observedAt, "sample-current", "codex_wham_usage_primary", "plus", "weekly", 604800, 30),
			}
			require.NoError(t, RecordChannelQuotaSnapshotBatchWithContext(context.Background(), current, options))
			require.NoError(t, RecordChannelQuotaSnapshotBatchWithContext(context.Background(), current, options))

			latestSuccess, err := ListLatestSuccessfulChannelQuotaSnapshotBatch(context.Background(), channelID, "codex_rate_limit", options.PreviousSources)
			require.NoError(t, err)
			require.Len(t, latestSuccess, 1)
			require.Equal(t, "sample-current", latestSuccess[0].SampleID)
			latestBatch, err := ListLatestChannelQuotaSnapshotBatch(context.Background(), channelID, observedAt-1, observedAt+1, "codex_rate_limit")
			require.NoError(t, err)
			require.Len(t, latestBatch, 3)
			for _, snapshot := range latestBatch {
				require.Equal(t, "sample-current", snapshot.SampleID)
			}
		})
	}
}
