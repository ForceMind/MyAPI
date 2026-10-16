package model

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
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
	"gorm.io/gorm/schema"
)

func tokenAccountMigrationTestEntities() []any {
	entities := []any{
		&User{}, &Token{}, &Channel{}, &ChannelQuotaSnapshot{}, &ChannelQuotaIdentityKeyRegistry{},
		&ChannelQuotaIdentityKeyVersion{}, &ChannelQuotaIdentityAlias{}, &ChannelQuotaSamplingTarget{},
		&AssignedAccessPolicy{}, &TokenAccountMigrationReceipt{}, &TokenBudget{}, &TokenBudgetReservation{}, &TokenBudgetPolicyChange{},
	}
	return append(entities, SubscriptionShareModels()...)
}

func newTokenAccountMigrationSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/token-account-migration.db?_pragma=busy_timeout(5000)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(tokenAccountMigrationTestEntities()...))
	return db
}

type tokenAccountMigrationFixture struct {
	db               *gorm.DB
	root             User
	token            Token
	sourceChannel    Channel
	targetChannel    Channel
	sourceSubject    string
	targetSubject    string
	sourcePool       SubscriptionSharePool
	targetPool       SubscriptionSharePool
	sourceAllocation SubscriptionShareAllocation
	targetAllocation SubscriptionShareAllocation
	sourceUsage      SubscriptionShareAllocationUsage
	targetUsage      SubscriptionShareAllocationUsage
	now              int64
}

func TestNormalizeTokenAccountMigrationInput(t *testing.T) {
	valid := TokenAccountMigrationPreviewInput{TargetChannelID: 9, TargetShares: []TokenAccountMigrationTargetShare{
		{SourcePoolID: 2, TargetPoolID: 12, ShareBPS: 2500},
		{SourcePoolID: 1, TargetPoolID: 11, ShareBPS: 7500},
	}}
	normalized, err := normalizeTokenAccountMigrationInput(valid)
	require.NoError(t, err)
	require.Equal(t, int64(1), normalized.TargetShares[0].SourcePoolID)
	require.Equal(t, int64(2), normalized.TargetShares[1].SourcePoolID)
	require.Equal(t, int64(2), valid.TargetShares[0].SourcePoolID, "normalization must not mutate the caller")

	invalid := []TokenAccountMigrationPreviewInput{
		{},
		{TargetChannelID: 9},
		{TargetChannelID: 9, TargetShares: []TokenAccountMigrationTargetShare{{SourcePoolID: 0, TargetPoolID: 1, ShareBPS: 1}}},
		{TargetChannelID: 9, TargetShares: []TokenAccountMigrationTargetShare{{SourcePoolID: 1, TargetPoolID: 0, ShareBPS: 1}}},
		{TargetChannelID: 9, TargetShares: []TokenAccountMigrationTargetShare{{SourcePoolID: 1, TargetPoolID: 2, ShareBPS: 0}}},
		{TargetChannelID: 9, TargetShares: []TokenAccountMigrationTargetShare{{SourcePoolID: 1, TargetPoolID: 2, ShareBPS: 1}, {SourcePoolID: 1, TargetPoolID: 3, ShareBPS: 1}}},
		{TargetChannelID: 9, TargetShares: []TokenAccountMigrationTargetShare{{SourcePoolID: 1, TargetPoolID: 2, ShareBPS: 1}, {SourcePoolID: 3, TargetPoolID: 2, ShareBPS: 1}}},
		{TargetChannelID: 9, TargetShares: []TokenAccountMigrationTargetShare{{SourcePoolID: 1, TargetPoolID: 2, ShareBPS: 1}, {SourcePoolID: 3, TargetPoolID: 4, ShareBPS: 1}, {SourcePoolID: 5, TargetPoolID: 6, ShareBPS: 1}}},
	}
	for index, input := range invalid {
		_, err := normalizeTokenAccountMigrationInput(input)
		require.ErrorIs(t, err, ErrTokenMaintenanceInvalid, "case %d", index)
	}
}

func TestTokenMaintenanceRequiresEnabledRootOwner(t *testing.T) {
	db := newTokenAccountMigrationSQLiteDB(t)
	root := User{Username: "maintenance-root", AffCode: "maintenance-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	otherRoot := User{Username: "maintenance-other-root", AffCode: "maintenance-other-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	ordinary := User{Username: "maintenance-user", AffCode: "maintenance-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	disabledRoot := User{Username: "maintenance-disabled-root", AffCode: "maintenance-disabled-root", Role: common.RoleRootUser, Status: common.UserStatusDisabled}
	for _, user := range []*User{&root, &otherRoot, &ordinary, &disabledRoot} {
		require.NoError(t, db.Create(user).Error)
	}
	rootToken := Token{UserId: root.Id, Key: "maintenance-root-key", Name: "root key", Status: common.TokenStatusEnabled, ExpiredTime: -1}
	ordinaryToken := Token{UserId: ordinary.Id, Key: "maintenance-user-key", Name: "user key", Status: common.TokenStatusEnabled, ExpiredTime: -1}
	disabledRootToken := Token{UserId: disabledRoot.Id, Key: "maintenance-disabled-key", Name: "disabled root key", Status: common.TokenStatusEnabled, ExpiredTime: -1}
	for _, token := range []*Token{&rootToken, &ordinaryToken, &disabledRootToken} {
		require.NoError(t, db.Create(token).Error)
	}

	for _, testCase := range []struct {
		name    string
		actorID int
		tokenID int
	}{
		{name: "ordinary user own token", actorID: ordinary.Id, tokenID: ordinaryToken.Id},
		{name: "root other user token", actorID: otherRoot.Id, tokenID: rootToken.Id},
		{name: "disabled root own token", actorID: disabledRoot.Id, tokenID: disabledRootToken.Id},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			input := TokenAccountMigrationPreviewInput{TargetChannelID: 1, TargetShares: []TokenAccountMigrationTargetShare{{SourcePoolID: 1, TargetPoolID: 2, ShareBPS: 1}}}
			_, err := ReadTokenMaintenance(context.Background(), db, testCase.actorID, testCase.tokenID, 1_900_000_000)
			require.ErrorIs(t, err, ErrTokenMaintenanceDenied)
			_, err = PreviewTokenAccountMigration(context.Background(), db, testCase.actorID, testCase.tokenID, input, 1_900_000_000)
			require.ErrorIs(t, err, ErrTokenMaintenanceDenied)
			_, _, err = CommitTokenAccountMigration(context.Background(), db, testCase.actorID, testCase.tokenID, TokenAccountMigrationCommand{
				TokenAccountMigrationPreviewInput: input,
				OperationID:                       strings.Repeat("1", 64),
				PreviewRevision:                   strings.Repeat("2", 64),
				Confirmed:                         true,
			}, 1_900_000_000)
			require.ErrorIs(t, err, ErrTokenMaintenanceDenied)
			_, err = ReadTokenMaintenanceOperation(context.Background(), db, testCase.actorID, testCase.tokenID, strings.Repeat("1", 64))
			require.ErrorIs(t, err, ErrTokenMaintenanceDenied)
		})
	}
}

func TestTokenAccountMigrationConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/token-account-migration.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
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
			prefix := fmt.Sprintf("tm_%x_", time.Now().UnixNano())
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), NamingStrategy: schema.NamingStrategy{TablePrefix: prefix}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Ping())
			entities := tokenAccountMigrationTestEntities()
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(entities...))
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(entities...))
			require.NoError(t, db.AutoMigrate(entities...), "migration must be repeatable")
			fixture := newTokenAccountMigrationFixture(t, db, engine.name)
			tokenAccountMigrationContract(t, fixture)
			tokenAccountMigrationConcurrencyContract(t, db, engine.name)
		})
	}
}

func tokenAccountMigrationConcurrencyContract(t *testing.T, db *gorm.DB, namespace string) {
	t.Helper()
	t.Run("concurrent different operations have one winner", func(t *testing.T) {
		fixture := newTokenAccountMigrationFixture(t, db, namespace+"-different")
		input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS}}}
		preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
		require.NoError(t, err)
		type outcome struct {
			replayed bool
			err      error
		}
		start := make(chan struct{})
		results := make(chan outcome, 2)
		for index := 0; index < 2; index++ {
			operationID := fmt.Sprintf("%064x", int64(fixture.token.Id)*100+int64(index+1))
			go func() {
				<-start
				_, replayed, commitErr := CommitTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, TokenAccountMigrationCommand{
					TokenAccountMigrationPreviewInput: input, OperationID: operationID, PreviewRevision: preview.PreviewRevision, Confirmed: true,
				}, fixture.now)
				results <- outcome{replayed: replayed, err: commitErr}
			}()
		}
		close(start)
		succeeded, rejected := 0, 0
		for range 2 {
			result := <-results
			switch {
			case result.err == nil:
				succeeded++
				assert.False(t, result.replayed)
			case errors.Is(result.err, ErrTokenMaintenanceConflict), errors.Is(result.err, ErrTokenMaintenanceBlocked):
				rejected++
			default:
				require.NoError(t, result.err)
			}
		}
		assert.Equal(t, 1, succeeded)
		assert.Equal(t, 1, rejected)
		var receipts int64
		require.NoError(t, db.Model(&TokenAccountMigrationReceipt{}).Where("token_id = ?", fixture.token.Id).Count(&receipts).Error)
		assert.Equal(t, int64(1), receipts)
	})

	t.Run("concurrent identical operation replays", func(t *testing.T) {
		fixture := newTokenAccountMigrationFixture(t, db, namespace+"-replay")
		input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS}}}
		preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
		require.NoError(t, err)
		operationID := fmt.Sprintf("%064x", int64(fixture.token.Id)*100+99)
		start := make(chan struct{})
		type outcome struct {
			replayed bool
			err      error
		}
		results := make(chan outcome, 2)
		for range 2 {
			go func() {
				<-start
				_, replayed, commitErr := CommitTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, TokenAccountMigrationCommand{
					TokenAccountMigrationPreviewInput: input, OperationID: operationID, PreviewRevision: preview.PreviewRevision, Confirmed: true,
				}, fixture.now)
				results <- outcome{replayed: replayed, err: commitErr}
			}()
		}
		close(start)
		replays := 0
		for range 2 {
			result := <-results
			require.NoError(t, result.err)
			if result.replayed {
				replays++
			}
		}
		assert.Equal(t, 1, replays)
		var receipts int64
		require.NoError(t, db.Model(&TokenAccountMigrationReceipt{}).Where("operation_id = ?", operationID).Count(&receipts).Error)
		assert.Equal(t, int64(1), receipts)
	})
}

func newTokenAccountMigrationFixture(t *testing.T, db *gorm.DB, namespace string) tokenAccountMigrationFixture {
	t.Helper()
	ctx := context.Background()
	key := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x5a}, 32))
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "active:v1:"+key)
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	require.NoError(t, err)
	now, err := ReadDatabaseUnixTime(ctx, db)
	require.NoError(t, err)
	prefix := fmt.Sprintf("tm%s%x", namespace[:1], time.Now().UnixNano())
	if len(prefix) > 18 {
		prefix = prefix[:18]
	}
	fixture := tokenAccountMigrationFixture{db: db, now: now}
	fixture.root = User{Username: prefix + "r", AffCode: prefix + "r", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&fixture.root).Error)
	fixture.token = Token{UserId: fixture.root.Id, Key: prefix + "-api-key", Name: "migrating-key", Status: common.TokenStatusEnabled, ExpiredTime: -1}
	require.NoError(t, db.Create(&fixture.token).Error)
	fixture.sourceChannel = Channel{Name: "source", Type: constant.ChannelTypeCodex, Key: prefix + "-source-credential", Status: common.ChannelStatusEnabled}
	fixture.targetChannel = Channel{Name: "target", Type: constant.ChannelTypeCodex, Key: prefix + "-target-credential", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&fixture.sourceChannel).Error)
	require.NoError(t, db.Create(&fixture.targetChannel).Error)

	channels := []*Channel{&fixture.sourceChannel, &fixture.targetChannel}
	subjects := make([]string, 0, 2)
	for index, channel := range channels {
		credential, err := ResolveChannelQuotaIdentity(ctx, db, keyring, "channel_type_57", common.ChannelQuotaIdentityKindCredential, []byte(channel.Key))
		require.NoError(t, err)
		account, err := ResolveChannelQuotaIdentity(ctx, db, keyring, "channel_type_57", common.ChannelQuotaIdentityKindProviderAccount, []byte(fmt.Sprintf("%s-account-%d", prefix, index)))
		require.NoError(t, err)
		require.NoError(t, EnsureChannelQuotaSamplingTarget(ctx, db, channel.Id, ChannelQuotaSamplingIdentity{SubjectRef: credential.SubjectRef, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped}))
		subjects = append(subjects, account.SubjectRef)
	}
	fixture.sourceSubject, fixture.targetSubject = subjects[0], subjects[1]

	makeSnapshot := func(channel Channel, subject, sample string, observedAt int64, used float64) ChannelQuotaSnapshot {
		total := float64(100)
		snapshot := ChannelQuotaSnapshot{
			CodexObservationQualified: true, CodexThresholdQualified: true, ChannelId: channel.Id,
			ObservedAt: observedAt, Available: 100 - used, Used: &used, Total: &total, Unit: "percent",
			MetricType: "codex_rate_limit", WindowType: "five_hour", PlanType: "plus", WindowSeconds: 18_000,
			ResetAt: now + 10_000, Source: codexQuotaWindowPrimarySource, SampleID: sample,
			SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "success",
		}
		require.NoError(t, db.Create(&snapshot).Error)
		return snapshot
	}
	sourceInitial := makeSnapshot(fixture.sourceChannel, fixture.sourceSubject, prefix+"-s0", now-2, 10)
	targetInitial := makeSnapshot(fixture.targetChannel, fixture.targetSubject, prefix+"-t0", now-2, 20)
	fixtureOperationBase := int64(fixture.token.Id) * 1000
	require.NoError(t, ConfigureSubscriptionShare(ctx, db, fixture.root.Id, fixture.token.Id, SubscriptionShareConfigInput{
		ID: fmt.Sprintf("%064x", fixtureOperationBase+1), SnapshotID: sourceInitial.Id, ShareBPS: 2500, ConfirmedApproximate: true,
	}, now-2))
	require.NoError(t, ConfigureSubscriptionShare(ctx, db, fixture.root.Id, fixture.token.Id, SubscriptionShareConfigInput{
		ID: fmt.Sprintf("%064x", fixtureOperationBase+2), SnapshotID: targetInitial.Id, ShareBPS: 0, ConfirmedApproximate: true,
	}, now-2))
	sourceLatest := makeSnapshot(fixture.sourceChannel, fixture.sourceSubject, prefix+"-s1", now-1, 10)
	targetLatest := makeSnapshot(fixture.targetChannel, fixture.targetSubject, prefix+"-t1", now-1, 20)
	require.NoError(t, RefreshSubscriptionShareProjection(ctx, db, fixture.sourceSubject, now))
	require.NoError(t, RefreshSubscriptionShareProjection(ctx, db, fixture.targetSubject, now))
	for index, item := range []struct {
		channel  Channel
		snapshot ChannelQuotaSnapshot
	}{{fixture.sourceChannel, sourceLatest}, {fixture.targetChannel, targetLatest}} {
		var target ChannelQuotaSamplingTarget
		require.NoError(t, db.Where("channel_id = ? AND active = ?", item.channel.Id, true).Take(&target).Error)
		require.NoError(t, MarkChannelQuotaSamplingTargetAttempt(ctx, db, item.channel.Id, target.SubjectRef, "sampled", subjects[index], ChannelQuotaSamplingProof{
			ExpectedVersion: target.EvidenceVersion,
			SampleID:        item.snapshot.SampleID,
		}))
	}
	require.NoError(t, db.Where("subject_ref = ?", fixture.sourceSubject).Take(&fixture.sourcePool).Error)
	require.NoError(t, db.Where("subject_ref = ?", fixture.targetSubject).Take(&fixture.targetPool).Error)
	require.NoError(t, db.Where("pool_id = ? AND token_id = ?", fixture.sourcePool.ID, fixture.token.Id).Take(&fixture.sourceAllocation).Error)
	require.NoError(t, db.Where("pool_id = ? AND token_id = ?", fixture.targetPool.ID, fixture.token.Id).Take(&fixture.targetAllocation).Error)
	require.NoError(t, db.Where("allocation_id = ? AND generation = ?", fixture.sourceAllocation.ID, fixture.sourcePool.CurrentGeneration).Take(&fixture.sourceUsage).Error)
	require.NoError(t, db.Where("allocation_id = ? AND generation = ?", fixture.targetAllocation.ID, fixture.targetPool.CurrentGeneration).Take(&fixture.targetUsage).Error)
	fixture.sourceUsage.EstimatedUsedMicro = 765_432
	fixture.targetUsage.EstimatedUsedMicro = 1_234_567
	require.NoError(t, db.Model(&SubscriptionShareAllocationUsage{}).Where("id = ?", fixture.sourceUsage.ID).Update("estimated_used_micro", fixture.sourceUsage.EstimatedUsedMicro).Error)
	require.NoError(t, db.Model(&SubscriptionShareAllocationUsage{}).Where("id = ?", fixture.targetUsage.ID).Update("estimated_used_micro", fixture.targetUsage.EstimatedUsedMicro).Error)
	policyJSON, err := common.Marshal(tokenMaintenanceAssignedPolicy{
		Enabled: true, PublicModels: []string{"public-model"}, UpstreamModels: []string{"upstream-model"}, ChannelIDs: []int{fixture.sourceChannel.Id},
	})
	require.NoError(t, err)
	_, err = ConfigureAssignedAccessPolicy(ctx, db, "token", fixture.token.Id, 0, string(policyJSON))
	require.NoError(t, err)
	return fixture
}

type tokenAccountMigrationSecondSlot struct {
	sourcePool       SubscriptionSharePool
	targetPool       SubscriptionSharePool
	sourceAllocation SubscriptionShareAllocation
	targetAllocation SubscriptionShareAllocation
}

func addTokenAccountMigrationSecondSlot(t *testing.T, fixture tokenAccountMigrationFixture) tokenAccountMigrationSecondSlot {
	t.Helper()
	ctx := context.Background()
	makeSnapshot := func(channel Channel, subject, sample string, observedAt int64, used float64) ChannelQuotaSnapshot {
		total := float64(100)
		snapshot := ChannelQuotaSnapshot{
			CodexObservationQualified: true, CodexThresholdQualified: true, ChannelId: channel.Id,
			ObservedAt: observedAt, Available: 100 - used, Used: &used, Total: &total, Unit: "percent",
			MetricType: "codex_rate_limit", WindowType: "weekly", PlanType: "plus", WindowSeconds: 604_800,
			ResetAt: fixture.now + 20_000, Source: codexQuotaWindowSecondarySource, SampleID: sample,
			SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "success",
		}
		require.NoError(t, fixture.db.Create(&snapshot).Error)
		return snapshot
	}
	sourceInitial := makeSnapshot(fixture.sourceChannel, fixture.sourceSubject, fmt.Sprintf("sec-s0-%d", fixture.token.Id), fixture.now-2, 30)
	targetInitial := makeSnapshot(fixture.targetChannel, fixture.targetSubject, fmt.Sprintf("sec-t0-%d", fixture.token.Id), fixture.now-2, 40)
	fixtureOperationBase := int64(fixture.token.Id) * 1000
	require.NoError(t, ConfigureSubscriptionShare(ctx, fixture.db, fixture.root.Id, fixture.token.Id, SubscriptionShareConfigInput{
		ID: fmt.Sprintf("%064x", fixtureOperationBase+3), SnapshotID: sourceInitial.Id, ShareBPS: 3000, ConfirmedApproximate: true,
	}, fixture.now-2))
	require.NoError(t, ConfigureSubscriptionShare(ctx, fixture.db, fixture.root.Id, fixture.token.Id, SubscriptionShareConfigInput{
		ID: fmt.Sprintf("%064x", fixtureOperationBase+4), SnapshotID: targetInitial.Id, ShareBPS: 0, ConfirmedApproximate: true,
	}, fixture.now-2))
	var sourceTarget, targetTarget ChannelQuotaSamplingTarget
	require.NoError(t, fixture.db.Where("channel_id = ? AND active = ?", fixture.sourceChannel.Id, true).Take(&sourceTarget).Error)
	require.NoError(t, fixture.db.Where("channel_id = ? AND active = ?", fixture.targetChannel.Id, true).Take(&targetTarget).Error)
	var sourceProof, targetProof ChannelQuotaSnapshot
	require.NoError(t, fixture.db.First(&sourceProof, sourceTarget.VerifiedSnapshotID).Error)
	require.NoError(t, fixture.db.First(&targetProof, targetTarget.VerifiedSnapshotID).Error)
	makeSnapshot(fixture.sourceChannel, fixture.sourceSubject, sourceProof.SampleID, fixture.now-1, 30)
	makeSnapshot(fixture.targetChannel, fixture.targetSubject, targetProof.SampleID, fixture.now-1, 40)
	require.NoError(t, RefreshSubscriptionShareProjection(ctx, fixture.db, fixture.sourceSubject, fixture.now))
	require.NoError(t, RefreshSubscriptionShareProjection(ctx, fixture.db, fixture.targetSubject, fixture.now))
	var slot tokenAccountMigrationSecondSlot
	require.NoError(t, fixture.db.Where("subject_ref = ? AND window_source = ?", fixture.sourceSubject, codexQuotaWindowSecondarySource).Take(&slot.sourcePool).Error)
	require.NoError(t, fixture.db.Where("subject_ref = ? AND window_source = ?", fixture.targetSubject, codexQuotaWindowSecondarySource).Take(&slot.targetPool).Error)
	require.NoError(t, fixture.db.Where("pool_id = ? AND token_id = ?", slot.sourcePool.ID, fixture.token.Id).Take(&slot.sourceAllocation).Error)
	require.NoError(t, fixture.db.Where("pool_id = ? AND token_id = ?", slot.targetPool.ID, fixture.token.Id).Take(&slot.targetAllocation).Error)
	return slot
}

func TestTokenAccountMigrationTwoSlotAtomicMapping(t *testing.T) {
	db := newTokenAccountMigrationSQLiteDB(t)
	fixture := newTokenAccountMigrationFixture(t, db, "two-slot")
	secondary := addTokenAccountMigrationSecondSlot(t, fixture)
	complete := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{
		{SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS},
		{SourcePoolID: secondary.sourcePool.ID, TargetPoolID: secondary.targetPool.ID, ShareBPS: secondary.sourceAllocation.ShareBPS},
	}}
	preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, complete, fixture.now)
	require.NoError(t, err)
	require.Equal(t, TokenMaintenanceStatusReady, preview.Status)
	require.Len(t, preview.Changes.Allocations, 2)

	assertUnchanged := func(t *testing.T) {
		var sourcePrimary, sourceSecondary, targetPrimary, targetSecondary SubscriptionShareAllocation
		require.NoError(t, db.First(&sourcePrimary, fixture.sourceAllocation.ID).Error)
		require.NoError(t, db.First(&sourceSecondary, secondary.sourceAllocation.ID).Error)
		require.NoError(t, db.First(&targetPrimary, fixture.targetAllocation.ID).Error)
		require.NoError(t, db.First(&targetSecondary, secondary.targetAllocation.ID).Error)
		assert.Equal(t, fixture.sourceAllocation.ShareBPS, sourcePrimary.ShareBPS)
		assert.Equal(t, secondary.sourceAllocation.ShareBPS, sourceSecondary.ShareBPS)
		assert.Zero(t, targetPrimary.ShareBPS)
		assert.Zero(t, targetSecondary.ShareBPS)
		var receipts int64
		require.NoError(t, db.Model(&TokenAccountMigrationReceipt{}).Count(&receipts).Error)
		assert.Zero(t, receipts)
	}

	omitted := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: complete.TargetShares[:1]}
	omittedPreview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, omitted, fixture.now)
	require.NoError(t, err)
	assert.Equal(t, TokenMaintenanceStatusBlocked, omittedPreview.Status)
	_, _, err = CommitTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, TokenAccountMigrationCommand{
		TokenAccountMigrationPreviewInput: omitted, OperationID: strings.Repeat("c", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
	}, fixture.now)
	require.ErrorIs(t, err, ErrTokenMaintenanceConflict)
	assertUnchanged(t)

	crossed := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{
		{SourcePoolID: fixture.sourcePool.ID, TargetPoolID: secondary.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS},
		{SourcePoolID: secondary.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: secondary.sourceAllocation.ShareBPS},
	}}
	crossedPreview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, crossed, fixture.now)
	require.NoError(t, err)
	assert.Equal(t, TokenMaintenanceStatusBlocked, crossedPreview.Status)
	_, _, err = CommitTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, TokenAccountMigrationCommand{
		TokenAccountMigrationPreviewInput: crossed, OperationID: strings.Repeat("d", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
	}, fixture.now)
	require.ErrorIs(t, err, ErrTokenMaintenanceConflict)
	assertUnchanged(t)

	_, replayed, err := CommitTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, TokenAccountMigrationCommand{
		TokenAccountMigrationPreviewInput: complete, OperationID: strings.Repeat("e", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
	}, fixture.now)
	require.NoError(t, err)
	assert.False(t, replayed)
	var sourcePrimary, sourceSecondary, targetPrimary, targetSecondary SubscriptionShareAllocation
	require.NoError(t, db.First(&sourcePrimary, fixture.sourceAllocation.ID).Error)
	require.NoError(t, db.First(&sourceSecondary, secondary.sourceAllocation.ID).Error)
	require.NoError(t, db.First(&targetPrimary, fixture.targetAllocation.ID).Error)
	require.NoError(t, db.First(&targetSecondary, secondary.targetAllocation.ID).Error)
	assert.Zero(t, sourcePrimary.ShareBPS)
	assert.Zero(t, sourceSecondary.ShareBPS)
	assert.Equal(t, fixture.sourceAllocation.ShareBPS, targetPrimary.ShareBPS)
	assert.Equal(t, secondary.sourceAllocation.ShareBPS, targetSecondary.ShareBPS)
}

func TestTokenAccountMigrationRejectsStalePreviewWithoutWrites(t *testing.T) {
	for _, scenario := range []string{"source share", "target share", "target usage", "policy", "target credential"} {
		t.Run(scenario, func(t *testing.T) {
			db := newTokenAccountMigrationSQLiteDB(t)
			fixture := newTokenAccountMigrationFixture(t, db, "stale-"+scenario)
			input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{
				SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS,
			}}}
			preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
			require.NoError(t, err)
			require.Equal(t, TokenMaintenanceStatusReady, preview.Status)
			switch scenario {
			case "source share":
				require.NoError(t, db.Model(&SubscriptionShareAllocation{}).Where("id = ?", fixture.sourceAllocation.ID).Update("share_bps", 2000).Error)
			case "target share":
				require.NoError(t, db.Model(&SubscriptionShareAllocation{}).Where("id = ?", fixture.targetAllocation.ID).Update("share_bps", 100).Error)
			case "target usage":
				require.NoError(t, db.Model(&SubscriptionShareAllocationUsage{}).Where("id = ?", fixture.targetUsage.ID).Update("estimated_used_micro", fixture.targetUsage.EstimatedUsedMicro+1).Error)
			case "policy":
				policyJSON, marshalErr := common.Marshal(tokenMaintenanceAssignedPolicy{Enabled: true, PublicModels: []string{"public-model"}, UpstreamModels: []string{"upstream-model"}, ChannelIDs: []int{fixture.sourceChannel.Id, fixture.targetChannel.Id}})
				require.NoError(t, marshalErr)
				_, err = ConfigureAssignedAccessPolicy(context.Background(), db, "token", fixture.token.Id, 1, string(policyJSON))
				require.NoError(t, err)
			case "target credential":
				keyring, keyErr := common.LoadChannelQuotaIdentityKeyring()
				require.NoError(t, keyErr)
				newKey := fixture.targetChannel.Key + "-rotated"
				identity, resolveErr := ResolveChannelQuotaIdentity(context.Background(), db, keyring, "channel_type_57", common.ChannelQuotaIdentityKindCredential, []byte(newKey))
				require.NoError(t, resolveErr)
				var target ChannelQuotaSamplingTarget
				require.NoError(t, db.Where("channel_id = ? AND confirmed_subject_ref = ?", fixture.targetChannel.Id, fixture.targetSubject).Take(&target).Error)
				require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
					if err := tx.Model(&Channel{}).Where("id = ? AND key = ?", fixture.targetChannel.Id, fixture.targetChannel.Key).Update("key", newKey).Error; err != nil {
						return err
					}
					return tx.Model(&ChannelQuotaSamplingTarget{}).Where("id = ? AND subject_ref = ?", target.ID, target.SubjectRef).Updates(map[string]any{
						"subject_ref": identity.SubjectRef, "evidence_version": target.EvidenceVersion + 1,
						"verified_snapshot_id": 0, "updated_at": target.UpdatedAt + 1,
					}).Error
				}))
			}

			var expectedSource, expectedTarget SubscriptionShareAllocation
			require.NoError(t, db.First(&expectedSource, fixture.sourceAllocation.ID).Error)
			require.NoError(t, db.First(&expectedTarget, fixture.targetAllocation.ID).Error)
			var expectedUsage SubscriptionShareAllocationUsage
			require.NoError(t, db.First(&expectedUsage, fixture.targetUsage.ID).Error)
			var expectedPolicy AssignedAccessPolicy
			require.NoError(t, db.Where("subject_type = ? AND subject_id = ?", "token", fixture.token.Id).Take(&expectedPolicy).Error)
			var expectedSourcePool, expectedTargetPool SubscriptionSharePool
			require.NoError(t, db.First(&expectedSourcePool, fixture.sourcePool.ID).Error)
			require.NoError(t, db.First(&expectedTargetPool, fixture.targetPool.ID).Error)

			_, _, err = CommitTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, TokenAccountMigrationCommand{
				TokenAccountMigrationPreviewInput: input, OperationID: strings.Repeat("f", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
			}, fixture.now)
			require.ErrorIs(t, err, ErrTokenMaintenanceConflict)
			var actualSource, actualTarget SubscriptionShareAllocation
			require.NoError(t, db.First(&actualSource, fixture.sourceAllocation.ID).Error)
			require.NoError(t, db.First(&actualTarget, fixture.targetAllocation.ID).Error)
			assert.Equal(t, expectedSource.ShareBPS, actualSource.ShareBPS)
			assert.Equal(t, expectedTarget.ShareBPS, actualTarget.ShareBPS)
			var actualUsage SubscriptionShareAllocationUsage
			require.NoError(t, db.First(&actualUsage, fixture.targetUsage.ID).Error)
			assert.Equal(t, expectedUsage.EstimatedUsedMicro, actualUsage.EstimatedUsedMicro)
			var actualPolicy AssignedAccessPolicy
			require.NoError(t, db.Where("subject_type = ? AND subject_id = ?", "token", fixture.token.Id).Take(&actualPolicy).Error)
			assert.Equal(t, expectedPolicy.Revision, actualPolicy.Revision)
			var actualSourcePool, actualTargetPool SubscriptionSharePool
			require.NoError(t, db.First(&actualSourcePool, fixture.sourcePool.ID).Error)
			require.NoError(t, db.First(&actualTargetPool, fixture.targetPool.ID).Error)
			assert.Equal(t, expectedSourcePool.Revision, actualSourcePool.Revision)
			assert.Equal(t, expectedTargetPool.Revision, actualTargetPool.Revision)
			var receipts int64
			require.NoError(t, db.Model(&TokenAccountMigrationReceipt{}).Count(&receipts).Error)
			assert.Zero(t, receipts)
		})
	}
}

func TestTokenAccountMigrationCapacityAndUserPolicyBlockers(t *testing.T) {
	t.Run("capacity", func(t *testing.T) {
		db := newTokenAccountMigrationSQLiteDB(t)
		fixture := newTokenAccountMigrationFixture(t, db, "capacity")
		other := Token{UserId: fixture.root.Id, Key: fmt.Sprintf("capacity-other-%d", fixture.token.Id), Name: "other", Status: common.TokenStatusEnabled, ExpiredTime: -1}
		require.NoError(t, db.Create(&other).Error)
		allocation := SubscriptionShareAllocation{PoolID: fixture.targetPool.ID, TokenID: other.Id, UserID: fixture.root.Id, ShareBPS: 8000, CreatedAt: fixture.now, UpdatedAt: fixture.now}
		require.NoError(t, db.Create(&allocation).Error)
		require.NoError(t, db.Create(&SubscriptionShareAllocationUsage{AllocationID: allocation.ID, Generation: fixture.targetPool.CurrentGeneration}).Error)
		input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS}}}
		preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
		require.NoError(t, err)
		assert.Equal(t, TokenMaintenanceStatusBlocked, preview.Status)
		assert.Contains(t, preview.Blockers, tokenMaintenanceBlocker("target_capacity_exceeded", TokenMaintenanceStatusBlocked))
		assert.Empty(t, preview.PreviewRevision)
	})

	t.Run("user policy", func(t *testing.T) {
		db := newTokenAccountMigrationSQLiteDB(t)
		fixture := newTokenAccountMigrationFixture(t, db, "user-policy")
		policyJSON, err := common.Marshal(tokenMaintenanceAssignedPolicy{Enabled: true, ChannelIDs: []int{fixture.sourceChannel.Id}})
		require.NoError(t, err)
		_, err = ConfigureAssignedAccessPolicy(context.Background(), db, "user", fixture.root.Id, 0, string(policyJSON))
		require.NoError(t, err)
		input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS}}}
		preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
		require.NoError(t, err)
		assert.Equal(t, TokenMaintenanceStatusBlocked, preview.Status)
		assert.Contains(t, preview.Blockers, tokenMaintenanceBlocker("user_policy_target_denied", TokenMaintenanceStatusBlocked))
		assert.Empty(t, preview.PreviewRevision)
	})
}

func TestTokenAccountMigrationBudgetMutualExclusionAndPendingState(t *testing.T) {
	for _, scenario := range []string{"token budget enabled", "fee budget enabled", "disabled budget pending fields", "unresolved reservation"} {
		t.Run(scenario, func(t *testing.T) {
			db := newTokenAccountMigrationSQLiteDB(t)
			fixture := newTokenAccountMigrationFixture(t, db, "budget-"+scenario)
			budget := TokenBudget{
				TokenID: fixture.token.Id, UserID: fixture.root.Id, FeeLimitUSD: "0", FeeUsedUSD: "0", FeeReservedUSD: "0",
				AccountMinRemainingBPS: 2000, AccountMaxAgeSeconds: 300, Revision: 1,
			}
			switch scenario {
			case "token budget enabled":
				budget.Enabled, budget.Limit = true, 1000
			case "fee budget enabled":
				budget.FeeEnabled, budget.FeeLimitUSD = true, "1"
			case "disabled budget pending fields":
				budget.PendingRequestID, budget.Reserved = "disabled-budget-pending", 10
			case "unresolved reservation":
				require.NoError(t, db.Create(&TokenBudgetReservation{
					RequestID: "unresolved-budget-reservation", TokenID: fixture.token.Id, UserID: fixture.root.Id,
					ChannelID: fixture.sourceChannel.Id, ModelName: "fixture", PayloadSHA256: strings.Repeat("9", 64),
					BoundSource: TokenBudgetBoundOpenAIResponses, InputTokens: 1, MaxOutputTokens: 1,
					Reserved: 2, State: TokenBudgetSent, FeeReservedUSD: "0", CreatedAt: fixture.now, UpdatedAt: fixture.now,
				}).Error)
			}
			require.NoError(t, db.Create(&budget).Error)
			input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{
				SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS,
			}}}
			preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
			require.NoError(t, err)
			expectedCode, expectedStatus := "", TokenMaintenanceStatusBlocked
			switch scenario {
			case "token budget enabled":
				expectedCode = "token_budget_conflicts_with_subscription_share"
			case "fee budget enabled":
				expectedCode = "fee_budget_conflicts_with_subscription_share"
			case "disabled budget pending fields":
				expectedCode, expectedStatus = "token_budget_pending_unknown", TokenMaintenanceStatusUnknown
			case "unresolved reservation":
				expectedCode, expectedStatus = "token_budget_reservation_unknown", TokenMaintenanceStatusUnknown
			}
			assert.Equal(t, expectedStatus, preview.Status)
			assert.Contains(t, preview.Blockers, tokenMaintenanceBlocker(expectedCode, expectedStatus))
			assert.Empty(t, preview.PreviewRevision)
			maintenance, err := ReadTokenMaintenance(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			assert.Equal(t, expectedStatus, maintenance.Migration.Status)
			assert.Contains(t, maintenance.Migration.Blockers, tokenMaintenanceBlocker(expectedCode, expectedStatus))
		})
	}
}

func TestTokenAccountMigrationRejectsBudgetPolicyChangeAfterPreview(t *testing.T) {
	db := newTokenAccountMigrationSQLiteDB(t)
	fixture := newTokenAccountMigrationFixture(t, db, "budget-stale")
	budget := TokenBudget{
		TokenID: fixture.token.Id, UserID: fixture.root.Id, FeeLimitUSD: "0", FeeUsedUSD: "0", FeeReservedUSD: "0",
		AccountMinRemainingBPS: 2000, AccountMaxAgeSeconds: 300, Revision: 1,
	}
	require.NoError(t, db.Create(&budget).Error)
	input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{
		SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS,
	}}}
	preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
	require.NoError(t, err)
	require.Equal(t, TokenMaintenanceStatusReady, preview.Status)
	require.NoError(t, db.Model(&TokenBudget{}).Where("token_id = ? AND revision = ?", fixture.token.Id, 1).Updates(map[string]any{"token_limit": int64(1), "revision": int64(2)}).Error)

	_, _, err = CommitTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, TokenAccountMigrationCommand{
		TokenAccountMigrationPreviewInput: input, OperationID: strings.Repeat("6", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
	}, fixture.now)
	require.ErrorIs(t, err, ErrTokenMaintenanceConflict)
	var source, target SubscriptionShareAllocation
	require.NoError(t, db.First(&source, fixture.sourceAllocation.ID).Error)
	require.NoError(t, db.First(&target, fixture.targetAllocation.ID).Error)
	assert.Equal(t, fixture.sourceAllocation.ShareBPS, source.ShareBPS)
	assert.Zero(t, target.ShareBPS)
	var receipts int64
	require.NoError(t, db.Model(&TokenAccountMigrationReceipt{}).Count(&receipts).Error)
	assert.Zero(t, receipts)
}

func TestTokenAccountMigrationRevalidatesDatabaseTimeAfterLocks(t *testing.T) {
	db := newTokenAccountMigrationSQLiteDB(t)
	fixture := newTokenAccountMigrationFixture(t, db, "fresh-clock")
	observedAt, resetAt := fixture.now-100, fixture.now-5
	for _, pool := range []SubscriptionSharePool{fixture.sourcePool, fixture.targetPool} {
		require.NoError(t, db.Model(&ChannelQuotaSnapshot{}).Where("id = ?", pool.SnapshotID).Updates(map[string]any{"observed_at": observedAt, "reset_at": resetAt}).Error)
		require.NoError(t, db.Model(&SubscriptionShareGeneration{}).Where("pool_id = ? AND generation = ?", pool.ID, pool.CurrentGeneration).Updates(map[string]any{"observed_at": observedAt, "reset_at": resetAt}).Error)
	}
	previewNow := fixture.now - 10
	input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{
		SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS,
	}}}
	preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, previewNow)
	require.NoError(t, err)
	require.Equal(t, TokenMaintenanceStatusReady, preview.Status)

	_, _, err = CommitTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, TokenAccountMigrationCommand{
		TokenAccountMigrationPreviewInput: input, OperationID: strings.Repeat("7", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
	}, previewNow)
	require.ErrorIs(t, err, ErrTokenMaintenancePreconditionUnknown)
	require.ErrorIs(t, err, ErrTokenMaintenanceUnknown)
	var source, target SubscriptionShareAllocation
	require.NoError(t, db.First(&source, fixture.sourceAllocation.ID).Error)
	require.NoError(t, db.First(&target, fixture.targetAllocation.ID).Error)
	assert.Equal(t, fixture.sourceAllocation.ShareBPS, source.ShareBPS)
	assert.Zero(t, target.ShareBPS)
	var sourcePool, targetPool SubscriptionSharePool
	require.NoError(t, db.First(&sourcePool, fixture.sourcePool.ID).Error)
	require.NoError(t, db.First(&targetPool, fixture.targetPool.ID).Error)
	assert.Equal(t, fixture.sourcePool.Revision, sourcePool.Revision)
	assert.Equal(t, fixture.targetPool.Revision, targetPool.Revision)
	policy, err := ReadAssignedAccessPolicy(context.Background(), db, "token", fixture.token.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(1), policy.Revision)
	var receipts int64
	require.NoError(t, db.Model(&TokenAccountMigrationReceipt{}).Count(&receipts).Error)
	assert.Zero(t, receipts)
}

func TestTokenAccountMigrationRejectsPendingBudgetAfterPreviewWithoutWrites(t *testing.T) {
	db := newTokenAccountMigrationSQLiteDB(t)
	fixture := newTokenAccountMigrationFixture(t, db, "budget-pending-after-preview")
	input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{
		SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS,
	}}}
	preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
	require.NoError(t, err)
	require.Equal(t, TokenMaintenanceStatusReady, preview.Status)
	require.NoError(t, db.Create(&TokenBudgetReservation{
		RequestID: "pending-after-preview", TokenID: fixture.token.Id, UserID: fixture.root.Id,
		ChannelID: fixture.sourceChannel.Id, ModelName: "fixture", PayloadSHA256: strings.Repeat("9", 64),
		BoundSource: TokenBudgetBoundOpenAIResponses, InputTokens: 1, MaxOutputTokens: 1,
		Reserved: 2, State: TokenBudgetSent, FeeReservedUSD: "0", CreatedAt: fixture.now, UpdatedAt: fixture.now,
	}).Error)

	_, _, err = CommitTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, TokenAccountMigrationCommand{
		TokenAccountMigrationPreviewInput: input, OperationID: strings.Repeat("9", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
	}, fixture.now)
	require.ErrorIs(t, err, ErrTokenMaintenanceConflict)
	assert.False(t, errors.Is(err, ErrTokenMaintenancePreconditionUnknown))
	var source, target SubscriptionShareAllocation
	require.NoError(t, db.First(&source, fixture.sourceAllocation.ID).Error)
	require.NoError(t, db.First(&target, fixture.targetAllocation.ID).Error)
	assert.Equal(t, fixture.sourceAllocation.ShareBPS, source.ShareBPS)
	assert.Equal(t, fixture.targetAllocation.ShareBPS, target.ShareBPS)
	var sourcePool, targetPool SubscriptionSharePool
	require.NoError(t, db.First(&sourcePool, fixture.sourcePool.ID).Error)
	require.NoError(t, db.First(&targetPool, fixture.targetPool.ID).Error)
	assert.Equal(t, fixture.sourcePool.Revision, sourcePool.Revision)
	assert.Equal(t, fixture.targetPool.Revision, targetPool.Revision)
	policy, err := ReadAssignedAccessPolicy(context.Background(), db, "token", fixture.token.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(1), policy.Revision)
	var receipts int64
	require.NoError(t, db.Model(&TokenAccountMigrationReceipt{}).Count(&receipts).Error)
	assert.Zero(t, receipts)
}

func TestTokenAccountMigrationRequiresCredentialBoundSnapshotProof(t *testing.T) {
	t.Run("reactivated credential without proof", func(t *testing.T) {
		db := newTokenAccountMigrationSQLiteDB(t)
		fixture := newTokenAccountMigrationFixture(t, db, "missing-proof")
		var target ChannelQuotaSamplingTarget
		require.NoError(t, db.Where("channel_id = ? AND active = ?", fixture.targetChannel.Id, true).Take(&target).Error)
		require.NoError(t, db.Model(&ChannelQuotaSamplingTarget{}).Where("id = ?", target.ID).Updates(map[string]any{
			"evidence_version": target.EvidenceVersion + 1, "verified_snapshot_id": 0,
		}).Error)
		input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{
			SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS,
		}}}
		preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
		require.NoError(t, err)
		assert.Equal(t, TokenMaintenanceStatusUnknown, preview.Status)
		assert.Contains(t, preview.Blockers, tokenMaintenanceBlocker("credential_evidence_unverified", TokenMaintenanceStatusUnknown))
		assert.Empty(t, preview.PreviewRevision)
	})

	t.Run("pool snapshot must share the verified sample", func(t *testing.T) {
		db := newTokenAccountMigrationSQLiteDB(t)
		fixture := newTokenAccountMigrationFixture(t, db, "proof-sample")
		input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{
			SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS,
		}}}
		before, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
		require.NoError(t, err)
		require.Equal(t, TokenMaintenanceStatusReady, before.Status)

		var current ChannelQuotaSnapshot
		require.NoError(t, db.First(&current, fixture.targetPool.SnapshotID).Error)
		current.Id = 0
		current.SampleID = fmt.Sprintf("new-proof-sample-%d", fixture.token.Id)
		current.ObservedAt = fixture.now
		current.ResetAt = fixture.now + 10_000
		require.NoError(t, db.Create(&current).Error)
		require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), db, fixture.targetSubject, fixture.now))

		after, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
		require.NoError(t, err)
		assert.Equal(t, TokenMaintenanceStatusUnknown, after.Status)
		assert.Contains(t, after.Blockers, tokenMaintenanceBlocker("target_credential_baseline_mismatch", TokenMaintenanceStatusUnknown))
		assert.Empty(t, after.PreviewRevision)

		_, _, err = CommitTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, TokenAccountMigrationCommand{
			TokenAccountMigrationPreviewInput: input, OperationID: strings.Repeat("8", 64), PreviewRevision: before.PreviewRevision, Confirmed: true,
		}, fixture.now)
		require.ErrorIs(t, err, ErrTokenMaintenanceConflict)
		var source, target SubscriptionShareAllocation
		require.NoError(t, db.First(&source, fixture.sourceAllocation.ID).Error)
		require.NoError(t, db.First(&target, fixture.targetAllocation.ID).Error)
		assert.Equal(t, fixture.sourceAllocation.ShareBPS, source.ShareBPS)
		assert.Zero(t, target.ShareBPS)
	})
}

func TestTokenAccountMigrationTargetHistoricalUncertaintyBlocksPreview(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		coveredAt   int64
		allocatedAt int64
	}{
		{name: "pending", coveredAt: 1, allocatedAt: 0},
		{name: "unsampled", coveredAt: 0, allocatedAt: 1},
		{name: "uncertain", coveredAt: 1, allocatedAt: -1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			db := newTokenAccountMigrationSQLiteDB(t)
			fixture := newTokenAccountMigrationFixture(t, db, "target-history-"+scenario.name)
			attempt := SubscriptionShareAttempt{
				AttemptID: strings.Repeat("1", 63) + fmt.Sprintf("%x", len(scenario.name)), RequestID: "target-old-history",
				TokenID: fixture.token.Id, UserID: fixture.root.Id, SubjectRef: fixture.targetSubject,
				ChannelID: fixture.targetChannel.Id, CredentialRef: strings.Repeat("2", 64),
				State: SubscriptionShareAttemptDispatching, CreatedAt: fixture.now - 2, UpdatedAt: fixture.now - 2,
			}
			require.Len(t, attempt.AttemptID, 64)
			require.NoError(t, db.Create(&attempt).Error)
			require.NoError(t, db.Create(&SubscriptionShareAttemptWindow{
				AttemptID: attempt.AttemptID, PoolID: fixture.targetPool.ID, AllocationID: fixture.targetAllocation.ID,
				Generation: fixture.targetPool.CurrentGeneration, CoveredAt: scenario.coveredAt, AllocatedAt: scenario.allocatedAt,
			}).Error)
			input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS}}}
			preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
			require.NoError(t, err)
			assert.Equal(t, TokenMaintenanceStatusUnknown, preview.Status)
			assert.Contains(t, preview.Blockers, tokenMaintenanceBlocker("target_usage_history_unknown", TokenMaintenanceStatusUnknown))
			assert.Empty(t, preview.PreviewRevision)
		})
	}
}

func TestTokenAccountMigrationReceiptFailureRollsBackEverything(t *testing.T) {
	db := newTokenAccountMigrationSQLiteDB(t)
	fixture := newTokenAccountMigrationFixture(t, db, "receipt-rollback")
	input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS}}}
	preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
	require.NoError(t, err)
	require.Equal(t, TokenMaintenanceStatusReady, preview.Status)
	var sourcePoolBefore, targetPoolBefore SubscriptionSharePool
	require.NoError(t, db.First(&sourcePoolBefore, fixture.sourcePool.ID).Error)
	require.NoError(t, db.First(&targetPoolBefore, fixture.targetPool.ID).Error)
	injected := errors.New("injected receipt persistence failure")
	callbackName := "test:token-account-migration-receipt-failure"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "TokenAccountMigrationReceipt" {
			tx.AddError(injected)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Create().Remove(callbackName)) })
	_, _, err = CommitTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, TokenAccountMigrationCommand{
		TokenAccountMigrationPreviewInput: input, OperationID: strings.Repeat("3", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
	}, fixture.now)
	require.ErrorIs(t, err, injected)
	var sourceAfter, targetAfter SubscriptionShareAllocation
	require.NoError(t, db.First(&sourceAfter, fixture.sourceAllocation.ID).Error)
	require.NoError(t, db.First(&targetAfter, fixture.targetAllocation.ID).Error)
	assert.Equal(t, fixture.sourceAllocation.ShareBPS, sourceAfter.ShareBPS)
	assert.Equal(t, fixture.targetAllocation.ShareBPS, targetAfter.ShareBPS)
	var sourcePoolAfter, targetPoolAfter SubscriptionSharePool
	require.NoError(t, db.First(&sourcePoolAfter, fixture.sourcePool.ID).Error)
	require.NoError(t, db.First(&targetPoolAfter, fixture.targetPool.ID).Error)
	assert.Equal(t, sourcePoolBefore.Revision, sourcePoolAfter.Revision)
	assert.Equal(t, targetPoolBefore.Revision, targetPoolAfter.Revision)
	policy, err := ReadAssignedAccessPolicy(context.Background(), db, "token", fixture.token.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(1), policy.Revision)
	var receipts int64
	require.NoError(t, db.Model(&TokenAccountMigrationReceipt{}).Count(&receipts).Error)
	assert.Zero(t, receipts)
}

func TestTokenAccountMigrationCommittedReceiptDecodeFailureRemainsResultUnknown(t *testing.T) {
	db := newTokenAccountMigrationSQLiteDB(t)
	fixture := newTokenAccountMigrationFixture(t, db, "corrupt-committed-receipt")
	input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{
		SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS,
	}}}
	preview, err := PreviewTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, input, fixture.now)
	require.NoError(t, err)
	require.Equal(t, TokenMaintenanceStatusReady, preview.Status)
	command := TokenAccountMigrationCommand{
		TokenAccountMigrationPreviewInput: input, OperationID: strings.Repeat("a", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
	}
	_, replayed, err := CommitTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, command, fixture.now)
	require.NoError(t, err)
	assert.False(t, replayed)
	require.NoError(t, db.Exec("UPDATE token_account_migration_receipts SET changes_json = ? WHERE operation_id = ?", "{", command.OperationID).Error)

	_, _, err = CommitTokenAccountMigration(context.Background(), db, fixture.root.Id, fixture.token.Id, command, fixture.now)
	require.ErrorIs(t, err, ErrTokenMaintenanceUnknown)
	assert.False(t, errors.Is(err, ErrTokenMaintenancePreconditionUnknown))
	_, err = ReadTokenMaintenanceOperation(context.Background(), db, fixture.root.Id, fixture.token.Id, command.OperationID)
	require.ErrorIs(t, err, ErrTokenMaintenanceUnknown)
	assert.False(t, errors.Is(err, ErrTokenMaintenancePreconditionUnknown))
	var source, target SubscriptionShareAllocation
	require.NoError(t, db.First(&source, fixture.sourceAllocation.ID).Error)
	require.NoError(t, db.First(&target, fixture.targetAllocation.ID).Error)
	assert.Zero(t, source.ShareBPS)
	assert.Equal(t, fixture.sourceAllocation.ShareBPS, target.ShareBPS)
}

func tokenAccountMigrationContract(t *testing.T, fixture tokenAccountMigrationFixture) {
	t.Helper()
	ctx := context.Background()
	input := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.targetChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{
		SourcePoolID: fixture.sourcePool.ID, TargetPoolID: fixture.targetPool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS,
	}}}
	budget := TokenBudget{
		TokenID: fixture.token.Id, UserID: fixture.root.Id, FeeLimitUSD: "0", FeeUsedUSD: "0", FeeReservedUSD: "0",
		AccountMinRemainingBPS: 2000, AccountMaxAgeSeconds: 300, Revision: 1,
	}
	require.NoError(t, fixture.db.Create(&budget).Error)
	for index, state := range []string{TokenBudgetSettled, TokenBudgetCancelled} {
		require.NoError(t, fixture.db.Create(&TokenBudgetReservation{
			RequestID: fmt.Sprintf("%064x", int64(fixture.token.Id)*10_000+int64(index+1)), TokenID: fixture.token.Id, UserID: fixture.root.Id,
			ChannelID: fixture.sourceChannel.Id, ModelName: "terminal-fixture", PayloadSHA256: strings.Repeat("9", 64),
			BoundSource: TokenBudgetBoundOpenAIResponses, InputTokens: 1, MaxOutputTokens: 1,
			State: state, FeeReservedUSD: "0", CreatedAt: fixture.now, UpdatedAt: fixture.now,
		}).Error)
	}
	maintenance, err := ReadTokenMaintenance(ctx, fixture.db, fixture.root.Id, fixture.token.Id, fixture.now)
	require.NoError(t, err)
	assert.False(t, maintenance.Rotation.CommitSupported)
	assert.Equal(t, TokenMaintenanceStatusUnknown, maintenance.Rotation.Status, "missing rotation evidence must not hide the ready migration")
	assert.Contains(t, maintenance.Rotation.Blockers, tokenMaintenanceBlocker("quota_writer_state_unknown", TokenMaintenanceStatusUnknown))
	assert.Equal(t, TokenMaintenanceStatusReady, maintenance.Migration.Status)
	require.NotNil(t, maintenance.Migration.Source)
	assert.Equal(t, fixture.sourceChannel.Id, maintenance.Migration.Source.ChannelID)
	require.Len(t, maintenance.Migration.Candidates, 1)
	assert.Equal(t, fixture.targetChannel.Id, maintenance.Migration.Candidates[0].ChannelID)
	assert.Equal(t, TokenMaintenanceStatusReady, maintenance.Migration.Candidates[0].Status)

	preview, err := PreviewTokenAccountMigration(ctx, fixture.db, fixture.root.Id, fixture.token.Id, input, fixture.now)
	require.NoError(t, err)
	assert.Equal(t, TokenMaintenanceStatusReady, preview.Status)
	assert.Len(t, preview.PreviewRevision, 64)
	assert.True(t, preview.Changes.Policy.Changed)
	require.Len(t, preview.Changes.Allocations, 1)
	assert.True(t, preview.Changes.Allocations[0].TargetExisting)

	// Only the real terminal reservation states are ignored. A sent reservation
	// remains unknown even while both budget features are disabled.
	pendingReservation := TokenBudgetReservation{
		RequestID: fmt.Sprintf("%064x", int64(fixture.token.Id)*10_000+3), TokenID: fixture.token.Id, UserID: fixture.root.Id,
		ChannelID: fixture.sourceChannel.Id, ModelName: "pending-fixture", PayloadSHA256: strings.Repeat("8", 64),
		BoundSource: TokenBudgetBoundOpenAIResponses, InputTokens: 1, MaxOutputTokens: 1, Reserved: 2,
		State: TokenBudgetSent, FeeReservedUSD: "0", CreatedAt: fixture.now, UpdatedAt: fixture.now,
	}
	require.NoError(t, fixture.db.Create(&pendingReservation).Error)
	unknown, err := PreviewTokenAccountMigration(ctx, fixture.db, fixture.root.Id, fixture.token.Id, input, fixture.now)
	require.NoError(t, err)
	assert.Equal(t, TokenMaintenanceStatusUnknown, unknown.Status)
	assert.Contains(t, unknown.Blockers, tokenMaintenanceBlocker("token_budget_reservation_unknown", TokenMaintenanceStatusUnknown))
	require.NoError(t, fixture.db.Delete(&pendingReservation).Error)

	// An unresolved historical attribution blocks without refreshing or writing
	// any projection row. Removing the synthetic evidence restores the preview.
	attempt := SubscriptionShareAttempt{AttemptID: strings.Repeat("e", 64), RequestID: "unknown-history", TokenID: fixture.token.Id, UserID: fixture.root.Id, SubjectRef: fixture.sourceSubject, ChannelID: fixture.sourceChannel.Id, CredentialRef: strings.Repeat("f", 64), State: SubscriptionShareAttemptDispatching, CreatedAt: fixture.now - 1, UpdatedAt: fixture.now - 1}
	require.NoError(t, fixture.db.Create(&attempt).Error)
	window := SubscriptionShareAttemptWindow{AttemptID: attempt.AttemptID, PoolID: fixture.sourcePool.ID, AllocationID: fixture.sourceAllocation.ID, Generation: fixture.sourcePool.CurrentGeneration, CoveredAt: fixture.now - 1, AllocatedAt: -1}
	require.NoError(t, fixture.db.Create(&window).Error)
	unknown, err = PreviewTokenAccountMigration(ctx, fixture.db, fixture.root.Id, fixture.token.Id, input, fixture.now)
	require.NoError(t, err)
	assert.Equal(t, TokenMaintenanceStatusUnknown, unknown.Status)
	assert.Empty(t, unknown.PreviewRevision)
	require.NoError(t, fixture.db.Delete(&window).Error)
	require.NoError(t, fixture.db.Delete(&attempt).Error)
	preview, err = PreviewTokenAccountMigration(ctx, fixture.db, fixture.root.Id, fixture.token.Id, input, fixture.now)
	require.NoError(t, err)
	require.Equal(t, TokenMaintenanceStatusReady, preview.Status)

	command := TokenAccountMigrationCommand{
		TokenAccountMigrationPreviewInput: input,
		OperationID:                       strings.Repeat("a", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
	}
	receipt, replayed, err := CommitTokenAccountMigration(ctx, fixture.db, fixture.root.Id, fixture.token.Id, command, fixture.now)
	require.NoError(t, err)
	assert.False(t, replayed)
	assert.Equal(t, "committed", receipt.State)
	assert.Equal(t, fixture.token.Id, receipt.TokenID)
	assert.Equal(t, fixture.sourceChannel.Id, receipt.SourceChannelID)
	assert.Equal(t, fixture.targetChannel.Id, receipt.TargetChannelID)

	var sourceAfter, targetAfter SubscriptionShareAllocation
	require.NoError(t, fixture.db.Where("id = ?", fixture.sourceAllocation.ID).Take(&sourceAfter).Error)
	require.NoError(t, fixture.db.Where("id = ?", fixture.targetAllocation.ID).Take(&targetAfter).Error)
	assert.Zero(t, sourceAfter.ShareBPS)
	assert.Equal(t, fixture.sourceAllocation.ShareBPS, targetAfter.ShareBPS)
	assert.Equal(t, fixture.targetAllocation.ID, targetAfter.ID, "the existing target allocation must be reused")
	var targetUsageAfter SubscriptionShareAllocationUsage
	require.NoError(t, fixture.db.Where("id = ?", fixture.targetUsage.ID).Take(&targetUsageAfter).Error)
	assert.Equal(t, fixture.targetUsage.EstimatedUsedMicro, targetUsageAfter.EstimatedUsedMicro)
	var tokenAfter Token
	require.NoError(t, fixture.db.First(&tokenAfter, fixture.token.Id).Error)
	assert.Equal(t, fixture.token.Key, tokenAfter.Key)
	assert.Equal(t, fixture.token.Id, tokenAfter.Id)

	policy, err := ReadAssignedAccessPolicy(ctx, fixture.db, "token", fixture.token.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(2), policy.Revision)
	var policyValue tokenMaintenanceAssignedPolicy
	require.NoError(t, common.UnmarshalJsonStr(policy.PolicyJSON, &policyValue))
	assert.Equal(t, []int{fixture.targetChannel.Id}, policyValue.ChannelIDs)
	assert.Equal(t, []string{"public-model"}, policyValue.PublicModels)
	assert.Equal(t, []string{"upstream-model"}, policyValue.UpstreamModels)

	replayedReceipt, replayed, err := CommitTokenAccountMigration(ctx, fixture.db, fixture.root.Id, fixture.token.Id, command, fixture.now+1)
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Equal(t, receipt, replayedReceipt)
	readReceipt, err := ReadTokenMaintenanceOperation(ctx, fixture.db, fixture.root.Id, fixture.token.Id, command.OperationID)
	require.NoError(t, err)
	assert.Equal(t, receipt, readReceipt)

	var stored TokenAccountMigrationReceipt
	require.NoError(t, fixture.db.First(&stored, "operation_id = ?", command.OperationID).Error)
	require.ErrorIs(t, fixture.db.Model(&stored).Update("state", "changed").Error, ErrAccountQuotaReceiptImmutable)
	require.ErrorIs(t, fixture.db.Delete(&stored).Error, ErrAccountQuotaReceiptImmutable)

	// Returning B to A reuses both original allocation rows and their usage.
	reverseInput := TokenAccountMigrationPreviewInput{TargetChannelID: fixture.sourceChannel.Id, TargetShares: []TokenAccountMigrationTargetShare{{
		SourcePoolID: fixture.targetPool.ID, TargetPoolID: fixture.sourcePool.ID, ShareBPS: fixture.sourceAllocation.ShareBPS,
	}}}
	reversePreview, err := PreviewTokenAccountMigration(ctx, fixture.db, fixture.root.Id, fixture.token.Id, reverseInput, fixture.now+1)
	require.NoError(t, err)
	require.Equal(t, TokenMaintenanceStatusReady, reversePreview.Status)
	reverseReceipt, reverseReplay, err := CommitTokenAccountMigration(ctx, fixture.db, fixture.root.Id, fixture.token.Id, TokenAccountMigrationCommand{
		TokenAccountMigrationPreviewInput: reverseInput, OperationID: strings.Repeat("b", 64), PreviewRevision: reversePreview.PreviewRevision, Confirmed: true,
	}, fixture.now+1)
	require.NoError(t, err)
	assert.False(t, reverseReplay)
	assert.Equal(t, fixture.targetChannel.Id, reverseReceipt.SourceChannelID)
	assert.Equal(t, fixture.sourceChannel.Id, reverseReceipt.TargetChannelID)
	require.NoError(t, fixture.db.Where("id = ?", fixture.sourceAllocation.ID).Take(&sourceAfter).Error)
	require.NoError(t, fixture.db.Where("id = ?", fixture.targetAllocation.ID).Take(&targetAfter).Error)
	assert.Equal(t, fixture.sourceAllocation.ShareBPS, sourceAfter.ShareBPS)
	assert.Zero(t, targetAfter.ShareBPS)
	var sourceUsageAfter SubscriptionShareAllocationUsage
	require.NoError(t, fixture.db.Where("id = ?", fixture.sourceUsage.ID).Take(&sourceUsageAfter).Error)
	require.NoError(t, fixture.db.Where("id = ?", fixture.targetUsage.ID).Take(&targetUsageAfter).Error)
	assert.Equal(t, fixture.sourceUsage.EstimatedUsedMicro, sourceUsageAfter.EstimatedUsedMicro)
	assert.Equal(t, fixture.targetUsage.EstimatedUsedMicro, targetUsageAfter.EstimatedUsedMicro)
}
