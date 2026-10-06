package model

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func quotaIdentityKeyring(t *testing.T, activeVersion string, activeFill byte, retired ...common.ChannelQuotaIdentityKey) common.ChannelQuotaIdentityKeyring {
	t.Helper()
	keyring := common.ChannelQuotaIdentityKeyring{
		Active:  common.ChannelQuotaIdentityKey{Version: activeVersion, Secret: []byte(strings.Repeat(string(activeFill), 32))},
		Retired: retired,
	}
	require.NoError(t, common.ValidateChannelQuotaIdentityKeyring(keyring))
	return keyring
}

func openQuotaIdentityDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(32)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(
		&ChannelQuotaIdentityKeyRegistry{},
		&ChannelQuotaIdentityKeyVersion{},
		&ChannelQuotaIdentityAlias{},
	))
	return db
}

func TestResolveChannelQuotaIdentityStableAndIsolated(t *testing.T) {
	db := openQuotaIdentityDB(t)
	keyring := quotaIdentityKeyring(t, "v1", 'a')
	first, err := ResolveChannelQuotaIdentity(context.Background(), db, keyring, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential-one"))
	require.NoError(t, err)
	replay, err := ResolveChannelQuotaIdentity(context.Background(), db, keyring, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential-one"))
	require.NoError(t, err)
	otherProvider, err := ResolveChannelQuotaIdentity(context.Background(), db, keyring, "anthropic", common.ChannelQuotaIdentityKindCredential, []byte("credential-one"))
	require.NoError(t, err)
	otherCredential, err := ResolveChannelQuotaIdentity(context.Background(), db, keyring, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential-two"))
	require.NoError(t, err)
	providerAccount, err := ResolveChannelQuotaIdentity(context.Background(), db, keyring, "openai", common.ChannelQuotaIdentityKindProviderAccount, []byte("credential-one"))
	require.NoError(t, err)

	assert.Equal(t, first, replay)
	assert.Equal(t, ChannelQuotaIdentityQualityCredentialScoped, first.Quality)
	assert.Len(t, first.SubjectRef, 43)
	assert.NotEqual(t, first.SubjectRef, otherProvider.SubjectRef)
	assert.NotEqual(t, first.SubjectRef, otherCredential.SubjectRef)
	assert.NotEqual(t, first.SubjectRef, providerAccount.SubjectRef)
	assert.Equal(t, ChannelQuotaIdentityQualityProviderConfirmed, providerAccount.Quality)
}

func TestLookupChannelQuotaIdentityIsReadOnlyAndSurvivesRotation(t *testing.T) {
	db := openQuotaIdentityDB(t)
	v1 := quotaIdentityKeyring(t, "v1", 'a')
	ctx := context.Background()
	provider := "channel_type_57"
	account := []byte("codex-account-1")

	_, found, err := LookupChannelQuotaIdentity(ctx, db, v1, provider, common.ChannelQuotaIdentityKindProviderAccount, account)
	require.NoError(t, err)
	assert.False(t, found)
	var count int64
	require.NoError(t, db.Model(&ChannelQuotaIdentityAlias{}).Count(&count).Error)
	assert.Zero(t, count)

	resolved, err := ResolveChannelQuotaIdentity(ctx, db, v1, provider, common.ChannelQuotaIdentityKindProviderAccount, account)
	require.NoError(t, err)
	rotated := quotaIdentityKeyring(t, "v2", 'b', v1.Active)
	lookup, found, err := LookupChannelQuotaIdentity(ctx, db, rotated, provider, common.ChannelQuotaIdentityKindProviderAccount, account)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, resolved, lookup)
	_, found, err = LookupChannelQuotaIdentity(ctx, db, rotated, "channel_type_20", common.ChannelQuotaIdentityKindProviderAccount, account)
	require.NoError(t, err)
	assert.False(t, found)
	_, found, err = LookupChannelQuotaIdentity(ctx, db, rotated, provider, common.ChannelQuotaIdentityKindProviderAccount, []byte("other-account"))
	require.NoError(t, err)
	assert.False(t, found)
	require.NoError(t, db.Model(&ChannelQuotaIdentityAlias{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestResolveChannelQuotaIdentityRotationAndKeyReorderingPreserveSubject(t *testing.T) {
	db := openQuotaIdentityDB(t)
	v1 := quotaIdentityKeyring(t, "v1", 'a')
	original, err := ResolveChannelQuotaIdentity(context.Background(), db, v1, "siliconflow", common.ChannelQuotaIdentityKindProviderAccount, []byte("account-42"))
	require.NoError(t, err)

	oldKey := v1.Active
	v2 := quotaIdentityKeyring(t, "v2", 'b', oldKey)
	rotated, err := ResolveChannelQuotaIdentity(context.Background(), db, v2, "siliconflow", common.ChannelQuotaIdentityKindProviderAccount, []byte("account-42"))
	require.NoError(t, err)
	assert.Equal(t, original.SubjectRef, rotated.SubjectRef)

	v3 := quotaIdentityKeyring(t, "v3", 'c', v2.Active, oldKey)
	reordered := quotaIdentityKeyring(t, "v3", 'c', oldKey, v2.Active)
	firstOrder, err := ResolveChannelQuotaIdentity(context.Background(), db, v3, "siliconflow", common.ChannelQuotaIdentityKindProviderAccount, []byte("account-42"))
	require.NoError(t, err)
	secondOrder, err := ResolveChannelQuotaIdentity(context.Background(), db, reordered, "siliconflow", common.ChannelQuotaIdentityKindProviderAccount, []byte("account-42"))
	require.NoError(t, err)
	assert.Equal(t, original.SubjectRef, firstOrder.SubjectRef)
	assert.Equal(t, original.SubjectRef, secondOrder.SubjectRef)

	activeOnly := quotaIdentityKeyring(t, "v3", 'c')
	_, err = ResolveChannelQuotaIdentity(context.Background(), db, activeOnly, "siliconflow", common.ChannelQuotaIdentityKindProviderAccount, []byte("account-42"))
	assert.ErrorIs(t, err, ErrChannelQuotaIdentityRegistryConflict)

	var count int64
	require.NoError(t, db.Model(&ChannelQuotaIdentityAlias{}).Where("subject_ref = ?", original.SubjectRef).Count(&count).Error)
	assert.Equal(t, int64(3), count)
}

func TestResolveChannelQuotaIdentityPersistsNoRawIdentityMaterial(t *testing.T) {
	db := openQuotaIdentityDB(t)
	keyring := quotaIdentityKeyring(t, "active-2026", 's')
	material := "sk-sensitive-raw-credential"
	resolved, err := ResolveChannelQuotaIdentity(context.Background(), db, keyring, "openai", common.ChannelQuotaIdentityKindCredential, []byte(material))
	require.NoError(t, err)

	var aliases []ChannelQuotaIdentityAlias
	require.NoError(t, db.Find(&aliases).Error)
	require.Len(t, aliases, 1)
	encodedSecret := base64.RawURLEncoding.EncodeToString(keyring.Active.Secret)
	persisted := aliases[0].KeyVersion + aliases[0].LookupHMAC + aliases[0].SubjectRef
	assert.NotContains(t, persisted, material)
	assert.NotContains(t, persisted, "openai")
	assert.NotContains(t, persisted, encodedSecret)
	assert.Equal(t, resolved.SubjectRef, aliases[0].SubjectRef)
	assert.Len(t, aliases[0].LookupHMAC, 64)
	var versions []ChannelQuotaIdentityKeyVersion
	require.NoError(t, db.Find(&versions).Error)
	require.Len(t, versions, 1)
	registryPersistence := versions[0].Version + versions[0].Fingerprint
	assert.NotContains(t, registryPersistence, material)
	assert.NotContains(t, registryPersistence, encodedSecret)
}

func TestEnsureChannelQuotaIdentityKeyringRegistersWithoutAliases(t *testing.T) {
	db := openQuotaIdentityDB(t)
	v1 := quotaIdentityKeyring(t, "v1", 'a')
	require.NoError(t, EnsureChannelQuotaIdentityKeyring(context.Background(), db, v1))
	require.NoError(t, EnsureChannelQuotaIdentityKeyring(context.Background(), db, v1))
	var aliasCount, versionCount int64
	require.NoError(t, db.Model(&ChannelQuotaIdentityAlias{}).Count(&aliasCount).Error)
	require.NoError(t, db.Model(&ChannelQuotaIdentityKeyVersion{}).Count(&versionCount).Error)
	assert.Zero(t, aliasCount)
	assert.Equal(t, int64(1), versionCount)

	v2 := quotaIdentityKeyring(t, "v2", 'b', v1.Active)
	require.NoError(t, EnsureChannelQuotaIdentityKeyring(context.Background(), db, v2))
	require.NoError(t, db.Model(&ChannelQuotaIdentityKeyVersion{}).Count(&versionCount).Error)
	assert.Equal(t, int64(2), versionCount)
}

func TestEnsureChannelQuotaIdentityKeyringRejectsPartialSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaIdentityKeyRegistry{}, &ChannelQuotaIdentityKeyVersion{}))
	keyring := quotaIdentityKeyring(t, "v1", 'a')
	assert.ErrorIs(t, EnsureChannelQuotaIdentityKeyring(context.Background(), db, keyring), ErrChannelQuotaIdentityRegistryCorrupt)

	require.NoError(t, db.AutoMigrate(&ChannelQuotaIdentityAlias{}))
	require.NoError(t, db.Migrator().DropIndex(&ChannelQuotaIdentityAlias{}, "uidx_channel_quota_identity_alias"))
	assert.ErrorIs(t, EnsureChannelQuotaIdentityKeyring(context.Background(), db, keyring), ErrChannelQuotaIdentityRegistryCorrupt)
}

func TestResolveChannelQuotaIdentityConcurrentCreateIsIdempotent(t *testing.T) {
	db := openQuotaIdentityDB(t)
	keyring := quotaIdentityKeyring(t, "v1", 'a')
	const workers = 24
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	start := make(chan struct{})
	results := make(chan ChannelQuotaResolvedIdentity, workers)
	errors := make(chan error, workers)
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			resolved, err := ResolveChannelQuotaIdentity(ctx, db, keyring, "openai", common.ChannelQuotaIdentityKindCredential, []byte("shared-credential"))
			if err != nil {
				errors <- err
				return
			}
			results <- resolved
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	var subject string
	for result := range results {
		if subject == "" {
			subject = result.SubjectRef
		}
		assert.Equal(t, subject, result.SubjectRef)
	}
	require.NotEmpty(t, subject)
	var count int64
	require.NoError(t, db.Model(&ChannelQuotaIdentityAlias{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestResolveChannelQuotaIdentityRejectsSplitBrainAliases(t *testing.T) {
	db := openQuotaIdentityDB(t)
	v1 := quotaIdentityKeyring(t, "v1", 'a')
	v2 := quotaIdentityKeyring(t, "v2", 'b', v1.Active)
	activeDigest, err := common.ChannelQuotaIdentityLookupHMAC(v2.Active, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential"))
	require.NoError(t, err)
	retiredDigest, err := common.ChannelQuotaIdentityLookupHMAC(v1.Active, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential"))
	require.NoError(t, err)
	require.NoError(t, db.Create(&[]ChannelQuotaIdentityAlias{
		{KeyVersion: "v2", LookupHMAC: activeDigest, SubjectRef: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 32))), CreatedAt: 1},
		{KeyVersion: "v1", LookupHMAC: retiredDigest, SubjectRef: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("b", 32))), CreatedAt: 1},
	}).Error)

	_, err = ResolveChannelQuotaIdentity(context.Background(), db, v2, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential"))
	assert.ErrorIs(t, err, ErrChannelQuotaIdentityAliasConflict)
}

func TestResolveChannelQuotaIdentityNewNodeThenOldNodeConverges(t *testing.T) {
	db := openQuotaIdentityDB(t)
	v1 := quotaIdentityKeyring(t, "v1", 'a')
	v2 := quotaIdentityKeyring(t, "v2", 'b', v1.Active)
	newNode, err := ResolveChannelQuotaIdentity(context.Background(), db, v2, "openai", common.ChannelQuotaIdentityKindCredential, []byte("rolling-credential"))
	require.NoError(t, err)
	oldNode, err := ResolveChannelQuotaIdentity(context.Background(), db, v1, "openai", common.ChannelQuotaIdentityKindCredential, []byte("rolling-credential"))
	require.NoError(t, err)
	assert.Equal(t, newNode.SubjectRef, oldNode.SubjectRef)

	var aliases []ChannelQuotaIdentityAlias
	require.NoError(t, db.Order("key_version ASC").Find(&aliases).Error)
	require.Len(t, aliases, 2)
	assert.Equal(t, []string{"v1", "v2"}, []string{aliases[0].KeyVersion, aliases[1].KeyVersion})
	assert.Equal(t, aliases[0].SubjectRef, aliases[1].SubjectRef)
}

func TestResolveChannelQuotaIdentityConcurrentOverlappingKeyringsConverge(t *testing.T) {
	db := openQuotaIdentityDB(t)
	v1 := quotaIdentityKeyring(t, "v1", 'a')
	v2 := quotaIdentityKeyring(t, "v2", 'b', v1.Active)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	start := make(chan struct{})
	results := make(chan ChannelQuotaResolvedIdentity, 32)
	errors := make(chan error, 32)
	var group sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		keyring := v1
		if worker%2 == 0 {
			keyring = v2
		}
		group.Add(1)
		go func(ring common.ChannelQuotaIdentityKeyring) {
			defer group.Done()
			<-start
			resolved, err := ResolveChannelQuotaIdentity(ctx, db, ring, "openai", common.ChannelQuotaIdentityKindCredential, []byte("rolling-race"))
			if err != nil {
				errors <- err
				return
			}
			results <- resolved
		}(keyring)
	}
	close(start)
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	var subject string
	for result := range results {
		if subject == "" {
			subject = result.SubjectRef
		}
		assert.Equal(t, subject, result.SubjectRef)
	}
	require.NotEmpty(t, subject)
	var aliases []ChannelQuotaIdentityAlias
	require.NoError(t, db.Order("key_version ASC").Find(&aliases).Error)
	require.Len(t, aliases, 2)
	assert.Equal(t, aliases[0].SubjectRef, aliases[1].SubjectRef)
}

func TestResolveChannelQuotaIdentityRegistryRejectsUnsafeKeyChanges(t *testing.T) {
	t.Run("same version different secret", func(t *testing.T) {
		db := openQuotaIdentityDB(t)
		original := quotaIdentityKeyring(t, "v1", 'a')
		_, err := ResolveChannelQuotaIdentity(context.Background(), db, original, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential"))
		require.NoError(t, err)
		replacement := quotaIdentityKeyring(t, "v1", 'b')
		_, err = ResolveChannelQuotaIdentity(context.Background(), db, replacement, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential"))
		assert.ErrorIs(t, err, ErrChannelQuotaIdentityRegistryConflict)
	})

	t.Run("rotation without overlap", func(t *testing.T) {
		db := openQuotaIdentityDB(t)
		v1 := quotaIdentityKeyring(t, "v1", 'a')
		_, err := ResolveChannelQuotaIdentity(context.Background(), db, v1, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential"))
		require.NoError(t, err)
		v2Only := quotaIdentityKeyring(t, "v2", 'b')
		_, err = ResolveChannelQuotaIdentity(context.Background(), db, v2Only, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential"))
		assert.ErrorIs(t, err, ErrChannelQuotaIdentityRegistryConflict)
	})

	t.Run("current keyring removes registered retired key", func(t *testing.T) {
		db := openQuotaIdentityDB(t)
		v1 := quotaIdentityKeyring(t, "v1", 'a')
		v2 := quotaIdentityKeyring(t, "v2", 'b', v1.Active)
		_, err := ResolveChannelQuotaIdentity(context.Background(), db, v2, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential"))
		require.NoError(t, err)
		v2Only := quotaIdentityKeyring(t, "v2", 'b')
		_, err = ResolveChannelQuotaIdentity(context.Background(), db, v2Only, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential"))
		assert.ErrorIs(t, err, ErrChannelQuotaIdentityRegistryConflict)
	})
}

func TestResolveChannelQuotaIdentityRejectsCorruptPersistentRows(t *testing.T) {
	t.Run("invalid subject", func(t *testing.T) {
		db := openQuotaIdentityDB(t)
		keyring := quotaIdentityKeyring(t, "v1", 'a')
		resolved, err := ResolveChannelQuotaIdentity(context.Background(), db, keyring, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential"))
		require.NoError(t, err)
		require.NoError(t, db.Exec("UPDATE channel_quota_identity_aliases SET subject_ref = ? WHERE subject_ref = ?", "not-canonical", resolved.SubjectRef).Error)
		_, err = ResolveChannelQuotaIdentity(context.Background(), db, keyring, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential"))
		assert.ErrorIs(t, err, ErrChannelQuotaIdentityRegistryCorrupt)
	})

	t.Run("invalid fingerprint", func(t *testing.T) {
		db := openQuotaIdentityDB(t)
		keyring := quotaIdentityKeyring(t, "v1", 'a')
		_, err := ResolveChannelQuotaIdentity(context.Background(), db, keyring, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential"))
		require.NoError(t, err)
		require.NoError(t, db.Exec("UPDATE channel_quota_identity_key_versions SET fingerprint = ? WHERE version = ?", strings.Repeat("A", 64), "v1").Error)
		_, err = ResolveChannelQuotaIdentity(context.Background(), db, keyring, "openai", common.ChannelQuotaIdentityKindCredential, []byte("credential"))
		assert.ErrorIs(t, err, ErrChannelQuotaIdentityRegistryCorrupt)
	})

	t.Run("registry gate without versions", func(t *testing.T) {
		db := openQuotaIdentityDB(t)
		nonce := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("n", 32)))
		require.NoError(t, db.Create(&ChannelQuotaIdentityKeyRegistry{
			Id: 1, ProtocolVersion: channelQuotaIdentityRegistryProtocolVersion, ActiveVersion: "v1", BootstrapNonce: nonce, CreatedAt: 1,
		}).Error)
		keyring := quotaIdentityKeyring(t, "v1", 'a')
		err := EnsureChannelQuotaIdentityKeyring(context.Background(), db, keyring)
		assert.ErrorIs(t, err, ErrChannelQuotaIdentityRegistryCorrupt)
	})
}

func TestChannelQuotaIdentityRetryableDatabaseErrors(t *testing.T) {
	db := openQuotaIdentityDB(t)
	assert.True(t, channelQuotaIdentityRetryable(db, &mysql.MySQLError{Number: 1205}))
	assert.True(t, channelQuotaIdentityRetryable(db, &mysql.MySQLError{Number: 1213}))
	assert.False(t, channelQuotaIdentityRetryable(db, &mysql.MySQLError{Number: 1062}))
	assert.True(t, channelQuotaIdentityRetryable(db, &pgconn.PgError{Code: "40001"}))
	assert.True(t, channelQuotaIdentityRetryable(db, &pgconn.PgError{Code: "40P01"}))
	assert.False(t, channelQuotaIdentityRetryable(db, &pgconn.PgError{Code: "23505"}))
	assert.True(t, channelQuotaIdentityRetryable(db, fmt.Errorf("database table is locked")))
}
