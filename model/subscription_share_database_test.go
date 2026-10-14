package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestSubscriptionShareConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/subscription-share.db")
			if engine.env != "" {
				if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
					t.Skip("requires explicitly configured disposable B2 database")
				}
				dsn := strings.TrimSpace(os.Getenv(engine.env))
				require.NotEmpty(t, dsn)
				var err error
				dialector, err = b2SubmissionDatabaseDialector(engine.name, dsn)
				require.NoError(t, err)
			}
			prefix := fmt.Sprintf("ss_%x_", time.Now().UnixNano())
			config := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), NamingStrategy: schema.NamingStrategy{TablePrefix: prefix}}
			db, err := gorm.Open(dialector, config)
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Ping())
			entities := []any{&User{}, &Token{}, &Channel{}, &ChannelQuotaSnapshot{}, &TokenBudget{}}
			entities = append(entities, SubscriptionShareModels()...)
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(entities...))
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(entities...))
			require.NoError(t, db.AutoMigrate(entities...), "subscription-share migration must be repeatable")
			for _, entity := range SubscriptionShareModels() {
				require.True(t, db.Migrator().HasTable(entity))
			}
			subscriptionShareConfiguredDatabaseContract(t, db, dialector, config, engine.name)
		})
	}
}

func subscriptionShareConfiguredDatabaseContract(t *testing.T, db *gorm.DB, dialector gorm.Dialector, config *gorm.Config, engine string) {
	t.Helper()
	ctx := context.Background()
	namespace := fmt.Sprintf("s%s%x", engine[:1], time.Now().UnixNano())
	if len(namespace) > 18 {
		namespace = namespace[:18]
	}
	root := User{Username: namespace + "r", AffCode: namespace + "r", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	owner := User{Username: namespace + "u", AffCode: namespace + "u", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&owner).Error)
	tokens := []Token{
		{UserId: owner.Id, Key: namespace + "a", Name: "share-a", Status: common.TokenStatusEnabled, ExpiredTime: -1},
		{UserId: owner.Id, Key: namespace + "b", Name: "share-b", Status: common.TokenStatusEnabled, ExpiredTime: -1},
	}
	require.NoError(t, db.Create(&tokens).Error)
	channels := []Channel{
		{Name: "codex-a", Key: "credential-a", Status: common.ChannelStatusEnabled},
		{Name: "codex-b", Key: "credential-b", Status: common.ChannelStatusEnabled},
	}
	require.NoError(t, db.Create(&channels).Error)
	subject, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	base := int64(1_800_000_000)
	sampleSequence := 0
	makeSnapshot := func(channelID int, source string, observedAt, resetAt int64, used float64) ChannelQuotaSnapshot {
		sampleSequence++
		total := float64(100)
		slot := "p"
		if source == codexQuotaWindowSecondarySource {
			slot = "s"
		}
		snapshot := ChannelQuotaSnapshot{CodexObservationQualified: true, CodexThresholdQualified: true, ChannelId: channelID, ObservedAt: observedAt, Available: 100 - used, Used: &used, Total: &total, Unit: "percent", MetricType: "codex_rate_limit", WindowType: "weekly", PlanType: "plus", WindowSeconds: 604800, ResetAt: resetAt, Source: source, SampleID: fmt.Sprintf("%s-%02d-%s", namespace, sampleSequence, slot), SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "success"}
		require.LessOrEqual(t, len(snapshot.SampleID), 36)
		require.NoError(t, db.Create(&snapshot).Error)
		return snapshot
	}
	configure := func(tokenID int, poolID int64, snapshotID int, revision int64, shareBPS int, operation int64, now int64) error {
		return ConfigureSubscriptionShare(ctx, db, root.Id, tokenID, SubscriptionShareConfigInput{ID: fmt.Sprintf("%064x", operation), PoolID: poolID, SnapshotID: snapshotID, ExpectedRevision: revision, ShareBPS: shareBPS, ConfirmedApproximate: true}, now)
	}

	selected := makeSnapshot(channels[0].Id, codexQuotaWindowPrimarySource, base, base+10_000, 5)
	require.NoError(t, configure(tokens[0].Id, 0, selected.Id, 0, 2500, 1, base))
	var primaryPool SubscriptionSharePool
	require.NoError(t, db.Where("window_source = ?", codexQuotaWindowPrimarySource).Take(&primaryPool).Error)
	require.NoError(t, configure(tokens[1].Id, primaryPool.ID, selected.Id, 1, 7500, 2, base))
	makeSnapshot(channels[1].Id, codexQuotaWindowPrimarySource, base+1, base+10_001, 10)
	require.NoError(t, RefreshSubscriptionShareProjection(ctx, db, subject, base+1))
	for index, token := range []Token{tokens[0], tokens[1], tokens[1], tokens[1]} {
		require.NoError(t, AdmitSubscriptionShareAttempt(ctx, db, token.Id, owner.Id, subject, channels[1].Id, fmt.Sprintf("%064x", 90), namespace+fmt.Sprintf("-request-%d", index), fmt.Sprintf("%064x", 100+index), base+2))
	}
	positive := makeSnapshot(channels[0].Id, codexQuotaWindowPrimarySource, base+3, base+10_000, 30)
	require.NoError(t, RefreshSubscriptionShareProjection(ctx, db, subject, base+3))
	var allocations []SubscriptionShareAllocation
	require.NoError(t, db.Where("pool_id = ?", primaryPool.ID).Order("token_id ASC").Find(&allocations).Error)
	require.Len(t, allocations, 2)
	used := map[int]int64{}
	for _, allocation := range allocations {
		var usage SubscriptionShareAllocationUsage
		require.NoError(t, db.Where("allocation_id = ? AND generation = ?", allocation.ID, 1).Take(&usage).Error)
		used[allocation.TokenID] = usage.EstimatedUsedMicro
	}
	assert.Equal(t, int64(5_000_000), used[tokens[0].Id])
	assert.Equal(t, int64(15_000_000), used[tokens[1].Id])
	assert.Equal(t, int64(20_000_000), used[tokens[0].Id]+used[tokens[1].Id])

	start := make(chan struct{})
	results := make(chan error, 2)
	for index, share := range []int{2000, 7000} {
		index, share := index, share
		go func() {
			<-start
			results <- configure(tokens[index].Id, primaryPool.ID, positive.Id, 2, share, int64(10+index), base+3)
		}()
	}
	close(start)
	winners, conflicts := 0, 0
	for range 2 {
		result := <-results
		switch {
		case result == nil:
			winners++
		case errors.Is(result, ErrSubscriptionShareConflict):
			conflicts++
		default:
			require.NoError(t, result)
		}
	}
	assert.Equal(t, 1, winners)
	assert.Equal(t, 1, conflicts)

	secondary := makeSnapshot(channels[0].Id, codexQuotaWindowSecondarySource, base+10, base+20_000, 0)
	require.NoError(t, configure(tokens[0].Id, 0, secondary.Id, 0, 5000, 20, base+10))
	var secondaryPool SubscriptionSharePool
	require.NoError(t, db.Where("window_source = ?", codexQuotaWindowSecondarySource).Take(&secondaryPool).Error)
	require.NoError(t, configure(tokens[1].Id, secondaryPool.ID, secondary.Id, 1, 5000, 21, base+10))
	makeSnapshot(channels[1].Id, codexQuotaWindowSecondarySource, base+11, base+20_000, 0)
	require.NoError(t, RefreshSubscriptionShareProjection(ctx, db, subject, base+11))
	var secondaryAllocation SubscriptionShareAllocation
	require.NoError(t, db.Where("pool_id = ? AND token_id = ?", secondaryPool.ID, tokens[0].Id).Take(&secondaryAllocation).Error)
	require.NoError(t, db.Model(&SubscriptionShareAllocationUsage{}).Where("allocation_id = ? AND generation = ?", secondaryAllocation.ID, 1).Update("estimated_used_micro", int64(50_000_000)).Error)
	var attemptsBefore int64
	require.NoError(t, db.Model(&SubscriptionShareAttempt{}).Count(&attemptsBefore).Error)
	var attemptWindowsBefore int64
	require.NoError(t, db.Model(&SubscriptionShareAttemptWindow{}).Count(&attemptWindowsBefore).Error)
	err = AdmitSubscriptionShareAttempt(ctx, db, tokens[0].Id, owner.Id, subject, channels[0].Id, fmt.Sprintf("%064x", 91), namespace+"-atomic", fmt.Sprintf("%064x", 200), base+12)
	require.ErrorIs(t, err, ErrSubscriptionShareExceeded)
	var attemptsAfter int64
	require.NoError(t, db.Model(&SubscriptionShareAttempt{}).Count(&attemptsAfter).Error)
	assert.Equal(t, attemptsBefore, attemptsAfter)
	var attemptWindowsAfter int64
	require.NoError(t, db.Model(&SubscriptionShareAttemptWindow{}).Count(&attemptWindowsAfter).Error)
	assert.Equal(t, attemptWindowsBefore, attemptWindowsAfter, "rejection in one pool must not leave another pool's attempt window")

	var applicationsBefore int64
	require.NoError(t, db.Model(&SubscriptionShareSampleApplication{}).Count(&applicationsBefore).Error)
	reopened, err := gorm.Open(dialector, config)
	require.NoError(t, err)
	reopenedSQL, err := reopened.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopenedSQL.Close()) })
	require.NoError(t, RefreshSubscriptionShareProjection(ctx, reopened, subject, base+12))
	var applicationsAfter int64
	require.NoError(t, reopened.Model(&SubscriptionShareSampleApplication{}).Count(&applicationsAfter).Error)
	assert.Equal(t, applicationsBefore, applicationsAfter, "reopening must replay the persisted cursor without double attribution")
	var protected int64
	require.NoError(t, ProtectedSubscriptionShareEvidenceQuery(reopened).Count(&protected).Error)
	assert.Positive(t, protected, "prefixed retention query must protect active unprojected evidence")
}
