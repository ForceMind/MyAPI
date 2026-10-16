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

func TestTokenModelLimitsConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/model-limits.db")
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
			db, err := gorm.Open(dialector, &gorm.Config{
				Logger:         logger.Default.LogMode(logger.Silent),
				NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("ml_%x_", time.Now().UnixNano())},
			})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&Token{}, &User{}))
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
			tokenModelLimitsDatabaseContract(t, db)
		})
	}
}

func tokenModelLimitsDatabaseContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	oldDB, oldRedis := DB, common.RedisEnabled
	DB, common.RedisEnabled = db, false
	t.Cleanup(func() { DB, common.RedisEnabled = oldDB, oldRedis })
	owner := User{Username: "model-owner", AffCode: "model-owner", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	other := User{Username: "model-other", AffCode: "model-other", Status: common.UserStatusEnabled, Role: common.RoleRootUser}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&other).Error)
	ips := "127.0.0.1\n10.0.0.0/8"
	token := Token{UserId: owner.Id, Key: "synthetic-model-scope-secret", Name: "Keep original name", Status: common.TokenStatusDisabled, ExpiredTime: 2000000000,
		RemainQuota: 1<<54 + 101, UsedQuota: 1<<54 + 103, QuotaVersion: 27, UnlimitedQuota: false,
		ModelLimitsEnabled: true, ModelLimits: "ModelA,Retired/Model", AllowIps: &ips, Group: "auto", AccessProfileID: "preserved-profile", CrossGroupRetry: true, AutoGroups: `["default","extra"]`}
	require.NoError(t, db.Create(&token).Error)
	ctx := context.Background()
	view, err := ReadTokenModelLimits(ctx, owner.Id, token.Id)
	require.NoError(t, err)
	assert.Equal(t, []string{"ModelA", "Retired/Model"}, view.Models)
	assert.True(t, view.ModelLimitsEnabled)
	encoded, err := common.Marshal(view)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), token.Key)
	assert.NotContains(t, string(encoded), "remain_quota")
	assert.NotContains(t, string(encoded), "allow_ips")
	_, err = ReadTokenModelLimits(ctx, other.Id, token.Id)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "Root has no implicit ownership override")
	_, err = UpdateTokenModelLimits(ctx, other.Id, token.Id, TokenModelLimitsCommand{ExpectedRevision: view.Revision, Models: []string{"blocked-other-owner"}})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// Consumption after opening the form is outside the metadata CAS domain.
	require.NoError(t, db.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]any{"remain_quota": token.RemainQuota - 7, "used_quota": token.UsedQuota + 7, "quota_version": token.QuotaVersion + 1}).Error)
	var before Token
	require.NoError(t, db.First(&before, token.Id).Error)
	next, err := UpdateTokenModelLimits(ctx, owner.Id, token.Id, TokenModelLimitsCommand{ExpectedRevision: view.Revision, ModelLimitsEnabled: true, Models: []string{"ModelB", "Retired/Model"}})
	require.NoError(t, err)
	assert.NotEqual(t, view.Revision, next.Revision)
	var after Token
	require.NoError(t, db.First(&after, token.Id).Error)
	assert.Equal(t, "ModelB,Retired/Model", after.ModelLimits)
	after.ModelLimits = before.ModelLimits
	assert.Equal(t, before, after, "only model scope changes; all balances and identities stay exact")
	staleMetadata := before
	staleMetadata.Name = "Metadata-only edit"
	require.NoError(t, staleMetadata.UpdateMetadata(false, false))
	require.NoError(t, db.First(&after, token.Id).Error)
	assert.Equal(t, "ModelB,Retired/Model", after.ModelLimits, "an older general editor cannot overwrite the newer dedicated scope")
	assert.True(t, after.ModelLimitsEnabled)
	assert.Equal(t, staleMetadata.Name, after.Name)
	staleMetadata.Status = common.TokenStatusEnabled
	require.NoError(t, staleMetadata.UpdateStatus())
	require.NoError(t, db.First(&after, token.Id).Error)
	assert.True(t, after.ModelLimitsEnabled)
	assert.Equal(t, "ModelB,Retired/Model", after.ModelLimits, "a late status-only write cannot replace model scope")
	assert.Equal(t, common.TokenStatusEnabled, after.Status)
	staleMetadata.ModelLimitsEnabled = false
	require.NoError(t, staleMetadata.UpdateMetadata(true, false))
	require.NoError(t, db.First(&after, token.Id).Error)
	assert.False(t, after.ModelLimitsEnabled)
	assert.Equal(t, "ModelB,Retired/Model", after.ModelLimits, "an enabled-only metadata write preserves the newer saved list")
	staleMetadata.ModelLimits = "ModelB,Retired/Model"
	staleMetadata.ModelLimitsEnabled = true
	require.NoError(t, staleMetadata.UpdateMetadata(true, true))
	// Refresh after that explicit name edit before exercising a case-only CAS.
	next, err = ReadTokenModelLimits(ctx, owner.Id, token.Id)
	require.NoError(t, err)

	// Case-only metadata changes must conflict even under a case-insensitive
	// MySQL collation: compare the digest in Go under the row lock.
	require.NoError(t, db.Model(&Token{}).Where("id = ?", token.Id).Update("model_limits", "modelb,Retired/Model").Error)
	_, err = UpdateTokenModelLimits(ctx, owner.Id, token.Id, TokenModelLimitsCommand{ExpectedRevision: next.Revision, Models: []string{}})
	require.ErrorIs(t, err, ErrTokenModelLimitsConflict)
	current, err := ReadTokenModelLimits(ctx, owner.Id, token.Id)
	require.NoError(t, err)
	require.NoError(t, db.Model(&Token{}).Where("id = ?", token.Id).Update("name", "Changed elsewhere").Error)
	_, err = UpdateTokenModelLimits(ctx, owner.Id, token.Id, TokenModelLimitsCommand{ExpectedRevision: current.Revision, Models: []string{}})
	require.ErrorIs(t, err, ErrTokenModelLimitsConflict)
	current, err = ReadTokenModelLimits(ctx, owner.Id, token.Id)
	require.NoError(t, err)

	// Explicitly disabling the additional restriction retains its saved list.
	current, err = UpdateTokenModelLimits(ctx, owner.Id, token.Id, TokenModelLimitsCommand{ExpectedRevision: current.Revision, Models: current.Models})
	require.NoError(t, err)
	assert.False(t, current.ModelLimitsEnabled)
	assert.Equal(t, []string{"modelb", "Retired/Model"}, current.Models)
	current, err = UpdateTokenModelLimits(ctx, owner.Id, token.Id, TokenModelLimitsCommand{ExpectedRevision: current.Revision, ModelLimitsEnabled: true, Models: []string{}})
	require.NoError(t, err)
	assert.True(t, current.ModelLimitsEnabled, "enabled with no allowed models is the existing deny-all state")
	assert.Empty(t, current.Models)
	require.NoError(t, db.First(&after, token.Id).Error)
	assert.Empty(t, after.GetModelLimitsMap())
	assert.True(t, after.ModelLimitsEnabled)

	require.NoError(t, db.Model(&User{}).Where("id = ?", owner.Id).Update("status", common.UserStatusDisabled).Error)
	_, err = ReadTokenModelLimits(ctx, owner.Id, token.Id)
	require.ErrorIs(t, err, ErrTokenModelLimitsDenied)
	_, err = UpdateTokenModelLimits(ctx, owner.Id, token.Id, TokenModelLimitsCommand{ExpectedRevision: current.Revision, Models: []string{"not-written"}})
	require.ErrorIs(t, err, ErrTokenModelLimitsDenied)
	var unchanged Token
	require.NoError(t, db.First(&unchanged, token.Id).Error)
	assert.Equal(t, after, unchanged)
}

func TestTokenModelLimitsRejectsAmbiguousOrOversizedIdentifiers(t *testing.T) {
	for _, test := range []struct {
		name     string
		models   []string
		revision string
		invalid  bool
	}{
		{"explicit empty set", []string{}, strings.Repeat("a", 64), false},
		{"case remains distinct", []string{"ModelA", "modela"}, strings.Repeat("a", 64), false},
		{"portable text boundary", []string{strings.Repeat("a", MaxTokenModelLimitsBytes)}, strings.Repeat("a", 64), false},
		{"portable text overflow includes delimiter", []string{strings.Repeat("a", MaxTokenModelLimitsBytes), "b"}, strings.Repeat("a", 64), true},
		{"utf8 bytes count", []string{strings.Repeat("界", MaxTokenModelLimitsBytes/3+1)}, strings.Repeat("a", 64), true},
		{"empty identifier", []string{""}, strings.Repeat("a", 64), true},
		{"blank identifier", []string{"  "}, strings.Repeat("a", 64), true},
		{"embedded space", []string{"model name"}, strings.Repeat("a", 64), true},
		{"unicode space", []string{"model\u00a0name"}, strings.Repeat("a", 64), true},
		{"javascript whitespace", []string{"model\ufeffname"}, strings.Repeat("a", 64), true},
		{"csv ambiguity", []string{"a,b"}, strings.Repeat("a", 64), true},
		{"postgres nul", []string{"a\x00b"}, strings.Repeat("a", 64), true},
		{"line separator", []string{"a\nb"}, strings.Repeat("a", 64), true},
		{"duplicate", []string{"a", "a"}, strings.Repeat("a", 64), true},
		{"missing revision", []string{"a"}, "", true},
		{"noncanonical revision", []string{"a"}, strings.Repeat("A", 64), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := TokenModelLimitsCommand{ExpectedRevision: test.revision, ModelLimitsEnabled: true, Models: test.models}
			err := command.Validate()
			if err == nil {
				err = command.ValidateNewModels(nil)
			}
			if test.invalid {
				require.ErrorIs(t, err, ErrTokenModelLimitsInvalid)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTokenModelLimitsRetainsLegacyIdentifiersWithoutNormalizingThem(t *testing.T) {
	command := TokenModelLimitsCommand{ExpectedRevision: strings.Repeat("a", 64), ModelLimitsEnabled: true, Models: []string{" legacy model ", "legacy\x01model", "", "new-model"}}
	require.NoError(t, command.Validate())
	require.NoError(t, command.ValidateNewModels([]string{" legacy model ", "legacy\x01model", ""}))
	command.Models = append(command.Models, "new bad model")
	require.ErrorIs(t, command.ValidateNewModels([]string{" legacy model ", "legacy\x01model", ""}), ErrTokenModelLimitsInvalid)
}

func TestTokenModelLimitsInvalidatesAuthorizationWithoutOverwritingPendingBalance(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "model-limits-cache")
	require.NoError(t, DecreaseTokenQuota(fixture.Token.Id, fixture.Token.Key, 17))
	view, err := ReadTokenModelLimits(t.Context(), fixture.User.Id, fixture.Token.Id)
	require.NoError(t, err)
	_, err = UpdateTokenModelLimits(t.Context(), fixture.User.Id, fixture.Token.Id, TokenModelLimitsCommand{ExpectedRevision: view.Revision, ModelLimitsEnabled: true, Models: []string{"OnlyModel"}})
	require.NoError(t, err)
	assert.False(t, server.Exists(getTokenCacheKey(fixture.Token.Key)))
	assert.True(t, server.Exists(getTokenCacheFenceKey(fixture.Token.Key)))
	loaded, err := GetTokenByKey(fixture.Token.Key, false)
	require.NoError(t, err)
	assert.True(t, loaded.ModelLimitsEnabled)
	assert.Equal(t, []string{"OnlyModel"}, loaded.GetModelLimits())
	flushQuotaBalanceBatchGenerations()
	var stored Token
	require.NoError(t, DB.First(&stored, fixture.Token.Id).Error)
	assert.Equal(t, fixture.Token.RemainQuota-17, stored.RemainQuota)
	assert.Equal(t, fixture.Token.UsedQuota+17, stored.UsedQuota)
}

func TestTokenModelLimitsCommittedCacheFailureRequiresReadback(t *testing.T) {
	_, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, "model-limits-cache-unknown")
	view, err := ReadTokenModelLimits(t.Context(), fixture.User.Id, fixture.Token.Id)
	require.NoError(t, err)
	count := 0
	common.RDB.AddHook(rejectNthTokenCacheEvalHook{count: &count, failAt: 3, err: errors.New("synthetic post-commit model scope failure")})
	result, err := UpdateTokenModelLimits(t.Context(), fixture.User.Id, fixture.Token.Id, TokenModelLimitsCommand{ExpectedRevision: view.Revision, ModelLimitsEnabled: true, Models: []string{"CommittedModel"}})
	require.ErrorIs(t, err, ErrTokenMetadataMutationCommittedCacheFailure)
	assert.Nil(t, result)
	actual, err := ReadTokenModelLimits(t.Context(), fixture.User.Id, fixture.Token.Id)
	require.NoError(t, err)
	assert.True(t, actual.ModelLimitsEnabled)
	assert.Equal(t, []string{"CommittedModel"}, actual.Models)
	assert.NotEqual(t, view.Revision, actual.Revision)
	_, err = UpdateTokenModelLimits(t.Context(), fixture.User.Id, fixture.Token.Id, TokenModelLimitsCommand{ExpectedRevision: view.Revision, Models: []string{}})
	require.ErrorIs(t, err, ErrTokenModelLimitsConflict)
}
