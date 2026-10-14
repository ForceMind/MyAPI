package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
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
	if engine == "sqlite" {
		t.Run("attempt updates exceed the real bind limit", func(t *testing.T) {
			subscriptionShareSQLiteVariableLimitLifecycle(t, db, root, owner, channels[0], namespace, base+50)
		})
	}
	if engine == "mysql" {
		t.Run("repeatable-read slot creation", func(t *testing.T) {
			subscriptionShareMySQLRepeatableReadSlotCreation(t, db, config, root, owner, channels[0], namespace, base+100)
		})
		t.Run("repeatable-read allocation cap and audit replay", func(t *testing.T) {
			subscriptionShareMySQLRepeatableReadAllocationAndReplay(t, db, config, root, owner, channels[0], namespace, base+200)
		})
	}

	duplicateGeneration := SubscriptionShareGeneration{PoolID: primaryPool.ID, Generation: 1, State: SubscriptionShareStateReady, Reason: "duplicate"}
	require.Error(t, db.Create(&duplicateGeneration).Error, "pool and generation must be unique")
	duplicateAllocation := SubscriptionShareAllocation{PoolID: primaryPool.ID, TokenID: tokens[0].Id, UserID: owner.Id, ShareBPS: 1, CreatedAt: base + 3, UpdatedAt: base + 3}
	require.Error(t, db.Create(&duplicateAllocation).Error, "pool and token allocation must be unique")
	var existingWindow SubscriptionShareAttemptWindow
	require.NoError(t, db.Where("pool_id = ?", primaryPool.ID).First(&existingWindow).Error)
	duplicateWindow := existingWindow
	duplicateWindow.ID = 0
	require.Error(t, db.Create(&duplicateWindow).Error, "attempt and pool window must be unique")
	var existingApplication SubscriptionShareSampleApplication
	require.NoError(t, db.Where("pool_id = ?", primaryPool.ID).First(&existingApplication).Error)
	duplicateApplication := existingApplication
	duplicateApplication.ID = 0
	require.Error(t, db.Create(&duplicateApplication).Error, "pool, generation, and snapshot application must be unique")

	// A second connection models an independently running application writer,
	// including SQLite's busy handling rather than sharing one connection pool.
	// The PostgreSQL fixture dialector owns a non-nil Conn pool. Recreating
	// only gorm.Config would reuse that pool and close the first writer too.
	reopenDialector := dialector
	if engine != "sqlite" {
		dsnEnv := "MYAPI_B2_MYSQL_DSN"
		if engine == "postgres" {
			dsnEnv = "MYAPI_B2_POSTGRES_DSN"
		}
		reopenDialector, err = b2SubmissionDatabaseDialector(engine, strings.TrimSpace(os.Getenv(dsnEnv)))
		require.NoError(t, err)
	}
	firstSQL, err := db.DB()
	require.NoError(t, err)
	reopened, err := gorm.Open(reopenDialector, &gorm.Config{Logger: config.Logger, NamingStrategy: config.NamingStrategy})
	require.NoError(t, err)
	reopenedSQL, err := reopened.DB()
	require.NoError(t, err)
	require.NotSame(t, firstSQL, reopenedSQL, "independent application writers must own different connection pools")
	t.Cleanup(func() { require.NoError(t, reopenedSQL.Close()) })
	start := make(chan struct{})
	results := make(chan error, 2)
	for index, share := range []int{2000, 7000} {
		index, share := index, share
		go func() {
			<-start
			writer := db
			if index == 1 {
				writer = reopened
			}
			results <- ConfigureSubscriptionShare(ctx, writer, root.Id, tokens[index].Id, SubscriptionShareConfigInput{ID: fmt.Sprintf("%064x", 10+index), PoolID: primaryPool.ID, SnapshotID: positive.Id, ExpectedRevision: 2, ShareBPS: share, ConfirmedApproximate: true}, base+3)
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
	require.NoError(t, RefreshSubscriptionShareProjection(ctx, reopened, subject, base+12))
	var applicationsAfter int64
	require.NoError(t, reopened.Model(&SubscriptionShareSampleApplication{}).Count(&applicationsAfter).Error)
	assert.Equal(t, applicationsBefore, applicationsAfter, "reopening must replay the persisted cursor without double attribution")
	var protected int64
	require.NoError(t, ProtectedSubscriptionShareEvidenceQuery(reopened).Count(&protected).Error)
	assert.Positive(t, protected, "prefixed retention query must protect active unprojected evidence")

	require.NoError(t, db.Where("id = ?", secondaryPool.ID).Take(&secondaryPool).Error)
	closeInput := SubscriptionShareCloseInput{ID: fmt.Sprintf("%064x", 300), PoolID: secondaryPool.ID, ExpectedRevision: secondaryPool.Revision, ConfirmedClose: true}
	require.NoError(t, CloseSubscriptionSharePool(ctx, db, root.Id, closeInput, base+13))
	require.NoError(t, CloseSubscriptionSharePool(ctx, reopened, root.Id, closeInput, base+14), "close replay must be idempotent across connections")
	require.NoError(t, db.Where("id = ?", secondaryPool.ID).Take(&secondaryPool).Error)
	assert.False(t, secondaryPool.Enabled)
	assert.Equal(t, closeInput.ExpectedRevision+1, secondaryPool.Revision)
	var closedGeneration SubscriptionShareGeneration
	require.NoError(t, db.Where("pool_id = ? AND generation = ?", secondaryPool.ID, secondaryPool.CurrentGeneration).Take(&closedGeneration).Error)
	assert.Equal(t, SubscriptionShareStateStopped, closedGeneration.State)
	assert.Equal(t, "pool_closed", closedGeneration.Reason)
	var closedUsage SubscriptionShareAllocationUsage
	require.NoError(t, db.Where("allocation_id = ? AND generation = ?", secondaryAllocation.ID, secondaryPool.CurrentGeneration).Take(&closedUsage).Error)
	assert.Equal(t, int64(50_000_000), closedUsage.EstimatedUsedMicro)
	var closeAudit SubscriptionShareAudit
	require.NoError(t, db.Where("id = ?", closeInput.ID).Take(&closeAudit).Error)
	assert.Equal(t, "pool_close", closeAudit.Action)
	assert.Equal(t, closeInput.ExpectedRevision, closeAudit.BeforeRevision)
	assert.Equal(t, closeInput.ExpectedRevision+1, closeAudit.AfterRevision)
}

func subscriptionShareSQLiteVariableLimitLifecycle(t *testing.T, db *gorm.DB, root, owner User, channel Channel, namespace string, now int64) {
	t.Helper()
	rows, err := db.Raw("PRAGMA compile_options").Rows()
	require.NoError(t, err)
	maxVariables := 0
	for rows.Next() {
		var option string
		require.NoError(t, rows.Scan(&option))
		if strings.HasPrefix(option, "MAX_VARIABLE_NUMBER=") {
			maxVariables, err = strconv.Atoi(strings.TrimPrefix(option, "MAX_VARIABLE_NUMBER="))
			require.NoError(t, err)
		}
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Greater(t, maxVariables, subscriptionShareAttemptWindowUpdateBatchSize)
	pendingCount := maxVariables + 1

	subject, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	token := Token{UserId: owner.Id, Key: namespace + "limit", Name: "limit", Status: common.TokenStatusEnabled, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	total, used := float64(100), float64(0)
	selected := ChannelQuotaSnapshot{CodexObservationQualified: true, CodexThresholdQualified: true, ChannelId: channel.Id, ObservedAt: now, Available: 100, Used: &used, Total: &total, Unit: "percent", MetricType: "codex_rate_limit", WindowType: "weekly", PlanType: "limit", WindowSeconds: 604800, ResetAt: now + 10_000, Source: codexQuotaWindowPrimarySource, SampleID: namespace + "-limit", SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "success"}
	require.LessOrEqual(t, len(selected.SampleID), 36)
	require.NoError(t, db.Create(&selected).Error)
	require.NoError(t, ConfigureSubscriptionShare(context.Background(), db, root.Id, token.Id, SubscriptionShareConfigInput{ID: fmt.Sprintf("%064x", 700), SnapshotID: selected.Id, ShareBPS: 10000, ConfirmedApproximate: true}, now))
	baseline := selected
	baseline.Id, baseline.ObservedAt, baseline.SampleID = 0, now+1, namespace+"-limit-base"
	require.NoError(t, db.Create(&baseline).Error)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), db, subject, now+1))
	var pool SubscriptionSharePool
	require.NoError(t, db.Where("subject_ref = ?", subject).Take(&pool).Error)
	var allocation SubscriptionShareAllocation
	require.NoError(t, db.Where("pool_id = ? AND token_id = ?", pool.ID, token.Id).Take(&allocation).Error)

	insertBatchSize := maxVariables / 10
	if insertBatchSize > 500 {
		insertBatchSize = 500
	}
	require.Positive(t, insertBatchSize)
	for start := 0; start < pendingCount; start += insertBatchSize {
		end := min(start+insertBatchSize, pendingCount)
		attempts := make([]SubscriptionShareAttempt, 0, end-start)
		windows := make([]SubscriptionShareAttemptWindow, 0, end-start)
		for index := start; index < end; index++ {
			attemptID := fmt.Sprintf("%064x", 1_000_000+index)
			attempts = append(attempts, SubscriptionShareAttempt{AttemptID: attemptID, RequestID: fmt.Sprintf("limit-%d", index), TokenID: token.Id, UserID: owner.Id, SubjectRef: subject, ChannelID: channel.Id, CredentialRef: fmt.Sprintf("%064x", 701), State: SubscriptionShareAttemptDispatching, CreatedAt: now + 2, UpdatedAt: now + 2})
			// CoveredAt models requests accumulated across zero-delta samples:
			// their dispatch slots are released while attribution remains pending.
			windows = append(windows, SubscriptionShareAttemptWindow{AttemptID: attemptID, PoolID: pool.ID, AllocationID: allocation.ID, Generation: pool.CurrentGeneration, CoveredAt: now + 2})
		}
		require.NoError(t, db.Create(&attempts).Error)
		require.NoError(t, db.Create(&windows).Error)
	}
	var persisted int64
	require.NoError(t, db.Model(&SubscriptionShareAttemptWindow{}).Where("pool_id = ? AND generation = ? AND allocated_at = 0", pool.ID, pool.CurrentGeneration).Count(&persisted).Error)
	require.Equal(t, int64(pendingCount), persisted)

	positive := selected
	positive.Id, positive.ObservedAt, positive.SampleID = 0, now+3, namespace+"-limit-positive"
	positiveUsed := float64(10)
	positive.Used, positive.Available = &positiveUsed, 90
	require.LessOrEqual(t, len(positive.SampleID), 36)
	require.NoError(t, db.Create(&positive).Error)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), db, subject, now+3))
	var attributed, originalCoverage int64
	require.NoError(t, db.Model(&SubscriptionShareAttemptWindow{}).Where("pool_id = ? AND allocated_at = ?", pool.ID, now+3).Count(&attributed).Error)
	require.NoError(t, db.Model(&SubscriptionShareAttemptWindow{}).Where("pool_id = ? AND covered_at = ?", pool.ID, now+2).Count(&originalCoverage).Error)
	assert.Equal(t, int64(pendingCount), attributed)
	assert.Equal(t, int64(pendingCount), originalCoverage)

	require.NoError(t, db.Model(&SubscriptionShareAttemptWindow{}).Where("pool_id = ?", pool.ID).Update("allocated_at", 0).Error)
	closeInput := SubscriptionShareCloseInput{ID: fmt.Sprintf("%064x", 702), PoolID: pool.ID, ExpectedRevision: pool.Revision, ConfirmedClose: true}
	require.NoError(t, CloseSubscriptionSharePool(context.Background(), db, root.Id, closeInput, now+4))
	var uncertain, preservedCoverage int64
	require.NoError(t, db.Model(&SubscriptionShareAttemptWindow{}).Where("pool_id = ? AND allocated_at = ?", pool.ID, -1).Count(&uncertain).Error)
	require.NoError(t, db.Model(&SubscriptionShareAttemptWindow{}).Where("pool_id = ? AND covered_at = ?", pool.ID, now+2).Count(&preservedCoverage).Error)
	assert.Equal(t, int64(pendingCount), uncertain)
	assert.Equal(t, int64(pendingCount), preservedCoverage)
	var audit SubscriptionShareAudit
	require.NoError(t, db.Where("id = ?", closeInput.ID).Take(&audit).Error)
	assert.Equal(t, int64(pendingCount), audit.UncertainAttempts)
}

func subscriptionShareMySQLRepeatableReadSlotCreation(t *testing.T, db *gorm.DB, config *gorm.Config, root, owner User, channel Channel, namespace string, now int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	subject, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	tokens := []Token{
		{UserId: owner.Id, Key: namespace + "rr-a", Name: "rr-a", Status: common.TokenStatusEnabled, ExpiredTime: -1},
		{UserId: owner.Id, Key: namespace + "rr-b", Name: "rr-b", Status: common.TokenStatusEnabled, ExpiredTime: -1},
	}
	require.NoError(t, db.Create(&tokens).Error)
	total, used := float64(100), float64(0)
	snapshots := []ChannelQuotaSnapshot{
		{CodexObservationQualified: true, CodexThresholdQualified: true, ChannelId: channel.Id, ObservedAt: now, Available: 100, Used: &used, Total: &total, Unit: "percent", MetricType: "codex_rate_limit", WindowType: "weekly", PlanType: "plus", WindowSeconds: 604800, ResetAt: now + 10_000, Source: codexQuotaWindowPrimarySource, SampleID: namespace + "-rr-week", SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "success"},
		{CodexObservationQualified: true, CodexThresholdQualified: true, ChannelId: channel.Id, ObservedAt: now + 1, Available: 100, Used: &used, Total: &total, Unit: "percent", MetricType: "codex_rate_limit", WindowType: "five_hour", PlanType: "plus", WindowSeconds: 18000, ResetAt: now + 10_001, Source: codexQuotaWindowPrimarySource, SampleID: namespace + "-rr-five", SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "success"},
	}
	for _, snapshot := range snapshots {
		require.LessOrEqual(t, len(snapshot.SampleID), 36)
	}
	require.NoError(t, db.Create(&snapshots).Error)
	require.NoError(t, db.Create(&SubscriptionShareAccountGate{SubjectRef: subject, Revision: 1, UpdatedAt: now}).Error)

	writers := []*gorm.DB{subscriptionShareOpenMySQLRepeatableReadWriter(t, config), subscriptionShareOpenMySQLRepeatableReadWriter(t, config)}

	arrived := make(chan struct{}, len(writers))
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseWriters := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseWriters()
	for index, writer := range writers {
		callbackName := fmt.Sprintf("subscription-share-rr-actor-%d-%s", index, namespace)
		var actorRead sync.Once
		require.NoError(t, writer.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
			if tx.Statement.Schema == nil || tx.Statement.Schema.Name != "User" {
				return
			}
			actorRead.Do(func() {
				arrived <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
					tx.AddError(ctx.Err())
				}
			})
		}))
		writer := writer
		t.Cleanup(func() { require.NoError(t, writer.Callback().Query().Remove(callbackName)) })
	}

	start := make(chan struct{})
	results := make(chan error, len(writers))
	var workers sync.WaitGroup
	for index, writer := range writers {
		index, writer := index, writer
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- ConfigureSubscriptionShare(ctx, writer, root.Id, tokens[index].Id, SubscriptionShareConfigInput{ID: fmt.Sprintf("%064x", 400+index), SnapshotID: snapshots[index].Id, ShareBPS: 5000, ConfirmedApproximate: true}, now+2)
		}()
	}
	defer func() {
		cancel()
		releaseWriters()
		workers.Wait()
	}()
	close(start)
	for range writers {
		select {
		case <-arrived:
		case <-ctx.Done():
			releaseWriters()
			require.FailNow(t, "both MySQL writers must establish their repeatable-read snapshots", ctx.Err().Error())
		}
	}
	releaseWriters()

	winners, conflicts := 0, 0
	for range writers {
		select {
		case result := <-results:
			switch {
			case result == nil:
				winners++
			case errors.Is(result, ErrSubscriptionShareConflict):
				conflicts++
			default:
				require.NoError(t, result)
			}
		case <-ctx.Done():
			require.FailNow(t, "MySQL repeatable-read slot creation did not finish", ctx.Err().Error())
		}
	}
	assert.Equal(t, 1, winners)
	assert.Equal(t, 1, conflicts)
	var enabled int64
	require.NoError(t, db.Model(&SubscriptionSharePool{}).Where("subject_ref = ? AND window_source = ? AND enabled = ?", subject, codexQuotaWindowPrimarySource, true).Count(&enabled).Error)
	assert.Equal(t, int64(1), enabled)
}

func subscriptionShareMySQLRepeatableReadAllocationAndReplay(t *testing.T, db *gorm.DB, config *gorm.Config, root, owner User, channel Channel, namespace string, now int64) {
	t.Helper()
	subject, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	tokens := []Token{
		{UserId: owner.Id, Key: namespace + "cap-a", Name: "cap-a", Status: common.TokenStatusEnabled, ExpiredTime: -1},
		{UserId: owner.Id, Key: namespace + "cap-b", Name: "cap-b", Status: common.TokenStatusEnabled, ExpiredTime: -1},
		{UserId: owner.Id, Key: namespace + "cap-c", Name: "cap-c", Status: common.TokenStatusEnabled, ExpiredTime: -1},
	}
	require.NoError(t, db.Create(&tokens).Error)
	total, used := float64(100), float64(0)
	snapshot := ChannelQuotaSnapshot{CodexObservationQualified: true, CodexThresholdQualified: true, ChannelId: channel.Id, ObservedAt: now, Available: 100, Used: &used, Total: &total, Unit: "percent", MetricType: "codex_rate_limit", WindowType: "weekly", PlanType: "team", WindowSeconds: 604800, ResetAt: now + 10_000, Source: codexQuotaWindowPrimarySource, SampleID: namespace + "-rr-cap", SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "success"}
	require.LessOrEqual(t, len(snapshot.SampleID), 36)
	require.NoError(t, db.Create(&snapshot).Error)
	require.NoError(t, ConfigureSubscriptionShare(context.Background(), db, root.Id, tokens[0].Id, SubscriptionShareConfigInput{ID: fmt.Sprintf("%064x", 500), SnapshotID: snapshot.Id, ShareBPS: 4000, ConfirmedApproximate: true}, now))
	var pool SubscriptionSharePool
	require.NoError(t, db.Where("subject_ref = ?", subject).Take(&pool).Error)
	baseline := snapshot
	baseline.Id, baseline.ObservedAt, baseline.SampleID = 0, now+1, namespace+"-rr-base"
	require.NoError(t, db.Create(&baseline).Error)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), db, subject, now+1))

	writer := subscriptionShareOpenMySQLRepeatableReadWriter(t, config)
	capResult := subscriptionShareRunAfterMySQLActorSnapshot(t, writer,
		func(ctx context.Context) error {
			return ConfigureSubscriptionShare(ctx, writer, root.Id, tokens[2].Id, SubscriptionShareConfigInput{ID: fmt.Sprintf("%064x", 502), PoolID: pool.ID, SnapshotID: snapshot.Id, ExpectedRevision: 2, ShareBPS: 6000, ConfirmedApproximate: true}, now+2)
		},
		func(ctx context.Context) error {
			return ConfigureSubscriptionShare(ctx, db, root.Id, tokens[1].Id, SubscriptionShareConfigInput{ID: fmt.Sprintf("%064x", 501), PoolID: pool.ID, SnapshotID: snapshot.Id, ExpectedRevision: 1, ShareBPS: 6000, ConfirmedApproximate: true}, now+1)
		},
	)
	require.ErrorIs(t, capResult, ErrSubscriptionShareInvalid)
	require.NoError(t, db.First(&pool, pool.ID).Error)
	assert.Equal(t, int64(2), pool.Revision)
	var allocations []SubscriptionShareAllocation
	require.NoError(t, db.Where("pool_id = ?", pool.ID).Order("id ASC").Find(&allocations).Error)
	require.Len(t, allocations, 2)
	allocated := 0
	for _, allocation := range allocations {
		allocated += allocation.ShareBPS
		assert.NotEqual(t, tokens[2].Id, allocation.TokenID)
	}
	assert.Equal(t, 10000, allocated)

	configureReplay := SubscriptionShareConfigInput{ID: fmt.Sprintf("%064x", 503), PoolID: pool.ID, SnapshotID: snapshot.Id, ExpectedRevision: 2, ShareBPS: 4000, ConfirmedApproximate: true}
	replayResult := subscriptionShareRunAfterMySQLActorSnapshot(t, writer,
		func(ctx context.Context) error {
			return ConfigureSubscriptionShare(ctx, writer, root.Id, tokens[0].Id, configureReplay, now+3)
		},
		func(ctx context.Context) error {
			return ConfigureSubscriptionShare(ctx, db, root.Id, tokens[0].Id, configureReplay, now+3)
		},
	)
	require.NoError(t, replayResult)
	require.NoError(t, db.First(&pool, pool.ID).Error)
	assert.Equal(t, int64(3), pool.Revision)
	var configureAudits int64
	require.NoError(t, db.Model(&SubscriptionShareAudit{}).Where("id = ?", configureReplay.ID).Count(&configureAudits).Error)
	assert.Equal(t, int64(1), configureAudits)

	attemptID := fmt.Sprintf("%064x", 600)
	require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), db, tokens[0].Id, owner.Id, subject, channel.Id, fmt.Sprintf("%064x", 601), namespace+"-mark", attemptID, now+4))
	markResult := subscriptionShareRunAfterMySQLQuerySnapshot(t, writer, "SubscriptionShareAttempt",
		func(ctx context.Context) error {
			return MarkSubscriptionShareAttemptNotSent(ctx, writer, attemptID, now+5)
		},
		func(ctx context.Context) error {
			positive := snapshot
			positive.Id, positive.ObservedAt, positive.SampleID = 0, now+5, namespace+"-rr-positive"
			positiveUsed := float64(10)
			positive.Used, positive.Available = &positiveUsed, 90
			if err := db.WithContext(ctx).Create(&positive).Error; err != nil {
				return err
			}
			return RefreshSubscriptionShareProjection(ctx, db, subject, now+5)
		},
	)
	require.ErrorIs(t, markResult, ErrSubscriptionShareConflict)
	var allocatedWindow SubscriptionShareAttemptWindow
	require.NoError(t, db.Where("attempt_id = ?", attemptID).Take(&allocatedWindow).Error)
	assert.Equal(t, now+5, allocatedWindow.AllocatedAt)
	var allocatedAttempt SubscriptionShareAttempt
	require.NoError(t, db.Where("attempt_id = ?", attemptID).Take(&allocatedAttempt).Error)
	assert.Equal(t, SubscriptionShareAttemptDispatching, allocatedAttempt.State)

	pendingID := fmt.Sprintf("%064x", 602)
	closeWithPending := SubscriptionShareCloseInput{ID: fmt.Sprintf("%064x", 504), PoolID: pool.ID, ExpectedRevision: 3, ConfirmedClose: true}
	closeResult := subscriptionShareRunAfterMySQLActorSnapshot(t, writer,
		func(ctx context.Context) error {
			return CloseSubscriptionSharePool(ctx, writer, root.Id, closeWithPending, now+6)
		},
		func(ctx context.Context) error {
			return AdmitSubscriptionShareAttempt(ctx, db, tokens[0].Id, owner.Id, subject, channel.Id, fmt.Sprintf("%064x", 603), namespace+"-uncertain", pendingID, now+6)
		},
	)
	require.NoError(t, closeResult)
	var uncertainWindow SubscriptionShareAttemptWindow
	require.NoError(t, db.Where("attempt_id = ?", pendingID).Take(&uncertainWindow).Error)
	assert.Equal(t, int64(-1), uncertainWindow.AllocatedAt)
	var pendingCloseAudit SubscriptionShareAudit
	require.NoError(t, db.Where("id = ?", closeWithPending.ID).Take(&pendingCloseAudit).Error)
	assert.Equal(t, int64(1), pendingCloseAudit.UncertainAttempts)
	require.NoError(t, db.First(&pool, pool.ID).Error)
	assert.False(t, pool.Enabled)
	assert.Equal(t, int64(4), pool.Revision)

	next := snapshot
	next.Id, next.ObservedAt, next.SampleID = 0, now+7, namespace+"-rr-next"
	next.WindowType, next.WindowSeconds, next.ResetAt = "five_hour", 18000, now+10_007
	require.NoError(t, db.Create(&next).Error)
	require.NoError(t, ConfigureSubscriptionShare(context.Background(), db, root.Id, tokens[0].Id, SubscriptionShareConfigInput{ID: fmt.Sprintf("%064x", 505), SnapshotID: next.Id, ShareBPS: 4000, ConfirmedApproximate: true}, now+7))
	var nextPool SubscriptionSharePool
	require.NoError(t, db.Where("subject_ref = ? AND enabled = ?", subject, true).Take(&nextPool).Error)
	closeReplay := SubscriptionShareCloseInput{ID: fmt.Sprintf("%064x", 506), PoolID: nextPool.ID, ExpectedRevision: 1, ConfirmedClose: true}
	replayResult = subscriptionShareRunAfterMySQLActorSnapshot(t, writer,
		func(ctx context.Context) error {
			return CloseSubscriptionSharePool(ctx, writer, root.Id, closeReplay, now+8)
		},
		func(ctx context.Context) error {
			return CloseSubscriptionSharePool(ctx, db, root.Id, closeReplay, now+8)
		},
	)
	require.NoError(t, replayResult)
	require.NoError(t, db.First(&nextPool, nextPool.ID).Error)
	assert.False(t, nextPool.Enabled)
	assert.Equal(t, int64(2), nextPool.Revision)
	var closeAudits int64
	require.NoError(t, db.Model(&SubscriptionShareAudit{}).Where("id = ?", closeReplay.ID).Count(&closeAudits).Error)
	assert.Equal(t, int64(1), closeAudits)
}

func subscriptionShareOpenMySQLRepeatableReadWriter(t *testing.T, config *gorm.Config) *gorm.DB {
	t.Helper()
	mysqlConfig, err := mysqldriver.ParseDSN(strings.TrimSpace(os.Getenv("MYAPI_B2_MYSQL_DSN")))
	require.NoError(t, err)
	if mysqlConfig.Params == nil {
		mysqlConfig.Params = make(map[string]string)
	}
	// MySQL 5.7 names this session variable tx_isolation. Setting it in the
	// connector DSN applies it to every physical connection used by GORM.
	mysqlConfig.Params["tx_isolation"] = "'REPEATABLE-READ'"
	writer, err := gorm.Open(gormmysql.New(gormmysql.Config{DSN: mysqlConfig.FormatDSN()}), &gorm.Config{Logger: config.Logger, NamingStrategy: config.NamingStrategy})
	require.NoError(t, err)
	sqlDB, err := writer.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var isolation string
	require.NoError(t, writer.Raw("SELECT @@tx_isolation").Scan(&isolation).Error)
	require.Equal(t, "REPEATABLE-READ", strings.ToUpper(isolation))
	return writer
}

func subscriptionShareRunAfterMySQLActorSnapshot(t *testing.T, writer *gorm.DB, operation, whileBlocked func(context.Context) error) error {
	t.Helper()
	return subscriptionShareRunAfterMySQLQuerySnapshot(t, writer, "User", operation, whileBlocked)
}

func subscriptionShareRunAfterMySQLQuerySnapshot(t *testing.T, writer *gorm.DB, schemaName string, operation, whileBlocked func(context.Context) error) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	var actorRead, releaseOnce sync.Once
	releaseWriter := func() { releaseOnce.Do(func() { close(release) }) }
	callbackName := fmt.Sprintf("subscription-share-rr-single-%p", release)
	require.NoError(t, writer.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Name != schemaName {
			return
		}
		actorRead.Do(func() {
			arrived <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				tx.AddError(ctx.Err())
			}
		})
	}))
	defer func() { require.NoError(t, writer.Callback().Query().Remove(callbackName)) }()
	result := make(chan error, 1)
	var worker sync.WaitGroup
	worker.Add(1)
	go func() {
		defer worker.Done()
		result <- operation(ctx)
	}()
	defer func() {
		cancel()
		releaseWriter()
		worker.Wait()
	}()
	select {
	case <-arrived:
	case <-ctx.Done():
		require.FailNow(t, "MySQL writer must establish its repeatable-read snapshot", ctx.Err().Error())
	}
	blockedErr := whileBlocked(ctx)
	releaseWriter()
	require.NoError(t, blockedErr)
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		require.FailNow(t, "MySQL repeatable-read operation did not finish", ctx.Err().Error())
		return ctx.Err()
	}
}
