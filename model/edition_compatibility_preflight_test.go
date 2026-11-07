package model

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/elliptic"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const editionFixturePassword = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func editionFixtureKey(t *testing.T) (string, string) {
	t.Helper()
	curve := elliptic.P256()
	encoded, err := webauthncbor.Marshal(map[int64]any{1: int64(2), 3: int64(-7), -1: int64(1), -2: curve.Params().Gx.FillBytes(make([]byte, 32)), -3: curve.Params().Gy.FillBytes(make([]byte, 32))})
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString([]byte("synthetic-credential-only")), base64.StdEncoding.EncodeToString(encoded)
}

func editionCreateSchema(t *testing.T, db *gorm.DB, dialect common.DatabaseType) {
	t.Helper()
	statements := []string{
		"CREATE TABLE users (id BIGINT PRIMARY KEY,role BIGINT,status BIGINT,password TEXT,deleted_at TIMESTAMP NULL)",
		"CREATE TABLE top_ups (id BIGINT PRIMARY KEY,status TEXT)",
		"CREATE TABLE subscription_orders (id BIGINT PRIMARY KEY,status TEXT)",
		"CREATE TABLE options (" + quotaExecutionQuote(dialect, "key") + " VARCHAR(255)," + quotaExecutionQuote(dialect, "value") + " TEXT)",
		"CREATE TABLE passkey_credentials (user_id BIGINT,credential_id TEXT,public_key TEXT,deleted_at TIMESTAMP NULL)",
		"CREATE TABLE user_subscriptions (id BIGINT PRIMARY KEY,status TEXT)",
	}
	for _, statement := range statements {
		require.NoError(t, db.Exec(statement).Error)
	}
}

func editionFixtureRows(t *testing.T, db *gorm.DB, dialect common.DatabaseType) map[string][]string {
	t.Helper()
	result := map[string][]string{}
	for _, table := range []string{"users", "top_ups", "subscription_orders", "options", "passkey_credentials", "user_subscriptions"} {
		rows, err := db.Raw("SELECT * FROM " + quotaExecutionQuote(dialect, table)).Rows()
		require.NoError(t, err)
		columns, err := rows.Columns()
		require.NoError(t, err)
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			require.NoError(t, rows.Scan(pointers...))
			parts := make([]string, len(values))
			for i, value := range values {
				if value == nil {
					parts[i] = "NULL"
				} else if b, ok := value.([]byte); ok {
					parts[i] = "bytes:" + string(b)
				} else {
					parts[i] = fmt.Sprintf("%T:%v", value, value)
				}
			}
			result[table] = append(result[table], strings.Join(parts, "\x00"))
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		sort.Strings(result[table])
	}
	return result
}

func editionFixtureFiles(t *testing.T, directory string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	result := map[string][]byte{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, e := os.ReadFile(filepath.Join(directory, entry.Name()))
		require.NoError(t, e)
		result[entry.Name()] = content
	}
	return result
}

func editionSeedCase(t *testing.T, db *gorm.DB, dialect common.DatabaseType, name string) {
	t.Helper()
	credential, key := editionFixtureKey(t)
	require.NoError(t, db.Exec("INSERT INTO users(id,role,status,password) VALUES(1,100,1,?)", editionFixturePassword).Error)
	require.NoError(t, db.Exec("INSERT INTO user_subscriptions(id,status) VALUES(1,'active')").Error)
	option := func(key, value any) {
		require.NoError(t, db.Exec("INSERT INTO options("+quotaExecutionQuote(dialect, "key")+","+quotaExecutionQuote(dialect, "value")+") VALUES(?,?)", key, value).Error)
	}
	switch name {
	case "terminal-and-local-candidates":
		for i, state := range []string{"success", "failed", "expired"} {
			require.NoError(t, db.Exec("INSERT INTO top_ups(id,status) VALUES(?,?)", i+1, state).Error)
			require.NoError(t, db.Exec("INSERT INTO subscription_orders(id,status) VALUES(?,?)", i+1, state).Error)
		}
		option("passkey.enabled", "true")
		require.NoError(t, db.Exec("INSERT INTO users(id,role,status,password) VALUES(2,1,1,''),(3,1,2,'broken')").Error)
		require.NoError(t, db.Exec("INSERT INTO users(id,role,status,password,deleted_at) VALUES(4,1,1,'broken','2020-01-01 00:00:00')").Error)
		for _, id := range []int{2, 3, 4} {
			value := key
			if id != 2 {
				value = "broken"
			}
			require.NoError(t, db.Exec("INSERT INTO passkey_credentials(user_id,credential_id,public_key) VALUES(?,?,?)", id, credential, value).Error)
		}
	case "pending-and-unknown":
		require.NoError(t, db.Exec("INSERT INTO top_ups(id,status) VALUES(1,'pending'),(2,NULL),(3,'SUCCESS')").Error)
		require.NoError(t, db.Exec("INSERT INTO subscription_orders(id,status) VALUES(1,'pending'),(2,'expired')").Error)
	case "invalid-flags":
		option("PasswordLoginEnabled", "true")
		option("PasswordLoginEnabled", "false")
		option("passkey.enabled", nil)
	case "malformed-local-records":
		option("passkey.enabled", "true")
		require.NoError(t, db.Exec("INSERT INTO users(id,role,status,password) VALUES(2,1,1,'broken'),(3,1,NULL,'')").Error)
		require.NoError(t, db.Exec("INSERT INTO passkey_credentials(user_id,credential_id,public_key) VALUES(2,?,?),(NULL,'bad','bad')", credential, base64.StdEncoding.EncodeToString([]byte{0xa0})).Error)
	case "known-no-local-path":
		option("PasswordLoginEnabled", "false")
		require.NoError(t, db.Exec("INSERT INTO passkey_credentials(user_id,credential_id,public_key) VALUES(1,?,?)", credential, key).Error)
	}
}

func editionAssertCase(t *testing.T, facts EditionCompatibilityFacts, name string) {
	t.Helper()
	require.True(t, facts.ReadOnly)
	require.True(t, facts.Complete)
	require.Equal(t, "existing", facts.Schema)
	require.Equal(t, "configured-primary", facts.Source)
	require.Equal(t, "not-verified", facts.LoginContinuity)
	require.NotNil(t, facts.TopUps)
	require.NotNil(t, facts.SubscriptionOrders)
	require.NotNil(t, facts.Authentication)
	auth := facts.Authentication
	switch name {
	case "terminal-and-local-candidates":
		assert.Equal(t, EditionOrderFacts{Total: 3, Terminal: 3}, *facts.TopUps)
		assert.Equal(t, EditionOrderFacts{Total: 3, Terminal: 3}, *facts.SubscriptionOrders)
		assert.EqualValues(t, 2, auth.EnabledUsers)
		assert.EqualValues(t, 1, auth.EnabledRootUsers)
		assert.EqualValues(t, 1, auth.PasswordCandidates)
		assert.EqualValues(t, 1, auth.PasskeyCandidates)
		assert.EqualValues(t, 2, auth.LocalCandidates)
		assert.EqualValues(t, 1, auth.RootLocalCandidates)
		assert.Zero(t, auth.UnknownUsers)
		assert.Zero(t, auth.UnknownCredentialRecords)
	case "pending-and-unknown":
		assert.Equal(t, EditionOrderFacts{Total: 3, Pending: 1, Unknown: 2}, *facts.TopUps)
		assert.Equal(t, EditionOrderFacts{Total: 2, Pending: 1, Terminal: 1}, *facts.SubscriptionOrders)
	case "invalid-flags":
		assert.Nil(t, auth.PasswordLoginEnabled)
		assert.Nil(t, auth.PasskeyEnabled)
		assert.Equal(t, "invalid", auth.PasswordLoginSource)
		assert.Equal(t, "invalid", auth.PasskeySource)
	case "malformed-local-records":
		assert.EqualValues(t, 2, auth.EnabledUsers)
		assert.EqualValues(t, 2, auth.UnknownUsers)
		assert.EqualValues(t, 2, auth.UnknownCredentialRecords)
		assert.EqualValues(t, 1, auth.LocalCandidates)
	case "known-no-local-path":
		require.NotNil(t, auth.PasswordLoginEnabled)
		assert.False(t, *auth.PasswordLoginEnabled)
		require.NotNil(t, auth.PasskeyEnabled)
		assert.False(t, *auth.PasskeyEnabled)
		assert.EqualValues(t, 1, auth.MissingLocalCandidates)
		assert.Zero(t, auth.LocalCandidates)
		assert.Zero(t, auth.UnknownUsers)
	}
	encoded, err := common.Marshal(facts)
	require.NoError(t, err)
	assert.False(t, bytes.Contains(encoded, []byte(editionFixturePassword)))
	credential, public := editionFixtureKey(t)
	assert.NotContains(t, string(encoded), credential)
	assert.NotContains(t, string(encoded), public)
	assert.NotContains(t, string(encoded), "public_key")
}

func TestEditionCompatibilitySQLite(t *testing.T) {
	for _, name := range []string{"terminal-and-local-candidates", "pending-and-unknown", "invalid-flags", "malformed-local-records", "known-no-local-path"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "edition.db")
			db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
			require.NoError(t, err)
			editionCreateSchema(t, db, common.DatabaseTypeSQLite)
			editionSeedCase(t, db, common.DatabaseTypeSQLite, name)
			native, err := db.DB()
			require.NoError(t, err)
			beforeRows := editionFixtureRows(t, db, common.DatabaseTypeSQLite)
			require.NoError(t, native.Close())
			before := editionFixtureFiles(t, filepath.Dir(path))
			previous := common.SQLitePath
			common.SQLitePath = path
			t.Cleanup(func() { common.SQLitePath = previous })
			t.Setenv("SQL_DSN", "local")
			baseline := common.PasswordLoginEnabled
			common.PasswordLoginEnabled = true
			t.Cleanup(func() { common.PasswordLoginEnabled = baseline })
			facts, err := InspectEditionCompatibilityPreMigration(context.Background())
			require.NoError(t, err)
			editionAssertCase(t, facts, name)
			assert.Equal(t, before, editionFixtureFiles(t, filepath.Dir(path)))
			reader, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
			require.NoError(t, err)
			assert.Equal(t, beforeRows, editionFixtureRows(t, reader, common.DatabaseTypeSQLite))
			pool, err := reader.DB()
			require.NoError(t, err)
			require.NoError(t, pool.Close())
		})
	}
}

func TestEditionCompatibilityFreshPartialAndMemory(t *testing.T) {
	for _, test := range []struct {
		name    string
		sql     string
		schema  string
		success bool
	}{
		{"new-file", "", "fresh", true}, {"empty-catalog", "PRAGMA user_version=1", "fresh", true}, {"foreign-schema", "CREATE TABLE unrelated(id INTEGER)", "partial", false}, {"partial-schema", "CREATE TABLE users(id INTEGER)", "partial", false}, {"view-is-not-base", "CREATE VIEW users AS SELECT 1 id", "partial", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "edition.db")
			if test.sql != "" {
				db, err := sql.Open(sqlite.DriverName, path)
				require.NoError(t, err)
				_, err = db.Exec(test.sql)
				require.NoError(t, err)
				require.NoError(t, db.Close())
			}
			before := editionFixtureFiles(t, filepath.Dir(path))
			old := common.SQLitePath
			common.SQLitePath = path
			t.Cleanup(func() { common.SQLitePath = old })
			t.Setenv("SQL_DSN", "local")
			facts, err := InspectEditionCompatibilityPreMigration(context.Background())
			if test.success {
				require.NoError(t, err)
				assert.True(t, facts.Complete)
			} else {
				require.ErrorIs(t, err, ErrEditionCompatibilityUnverifiable)
				assert.False(t, facts.Complete)
				assert.Nil(t, facts.Authentication)
				assert.NotContains(t, err.Error(), path)
			}
			assert.Equal(t, test.schema, facts.Schema)
			assert.Equal(t, before, editionFixtureFiles(t, filepath.Dir(path)))
		})
	}
	for _, dsn := range []string{":memory:", "file:shared?mode=memory&cache=shared", "file::memory:?cache=shared"} {
		t.Run(dsn, func(t *testing.T) {
			old := common.SQLitePath
			common.SQLitePath = dsn
			t.Cleanup(func() { common.SQLitePath = old })
			t.Setenv("SQL_DSN", "local")
			facts, err := InspectEditionCompatibilityPreMigration(context.Background())
			if dsn == ":memory:" {
				require.NoError(t, err)
				assert.Equal(t, "new-anonymous-memory", facts.Source)
				assert.Equal(t, "fresh", facts.Schema)
			} else {
				require.ErrorIs(t, err, ErrEditionCompatibilityUnverifiable)
				assert.Equal(t, "unavailable", facts.Schema)
				assert.False(t, facts.Complete)
			}
		})
	}
}

func TestEditionCompatibilityMissingColumnAndCancelledRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "edition.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	editionCreateSchema(t, db, common.DatabaseTypeSQLite)
	require.NoError(t, db.Exec("ALTER TABLE passkey_credentials RENAME TO saved_credentials").Error)
	require.NoError(t, db.Exec("CREATE TABLE passkey_credentials(user_id INTEGER,credential_id TEXT,deleted_at TIMESTAMP)").Error)
	native, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, native.Close())
	old := common.SQLitePath
	common.SQLitePath = path
	t.Cleanup(func() { common.SQLitePath = old })
	t.Setenv("SQL_DSN", "local")
	before := editionFixtureFiles(t, filepath.Dir(path))
	facts, err := InspectEditionCompatibilityPreMigration(context.Background())
	require.ErrorIs(t, err, ErrEditionCompatibilityUnverifiable)
	assert.Equal(t, "partial", facts.Schema)
	assert.Nil(t, facts.TopUps)
	assert.Nil(t, facts.SubscriptionOrders)
	assert.Nil(t, facts.Authentication)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	facts, err = InspectEditionCompatibilityPreMigration(ctx)
	require.ErrorIs(t, err, ErrEditionCompatibilityUnverifiable)
	assert.Equal(t, "unavailable", facts.Schema)
	assert.False(t, facts.Complete)
	assert.Equal(t, before, editionFixtureFiles(t, filepath.Dir(path)))
}

func TestEditionCompatibilityReadsWALWithoutChangingFilesOrRows(t *testing.T) {
	for _, clean := range []bool{false, true} {
		t.Run(fmt.Sprintf("clean-%t", clean), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "edition.db")
			db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.Exec("PRAGMA journal_mode=WAL").Error)
			editionCreateSchema(t, db, common.DatabaseTypeSQLite)
			editionSeedCase(t, db, common.DatabaseTypeSQLite, "terminal-and-local-candidates")
			native, err := db.DB()
			require.NoError(t, err)
			native.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = native.Close() })
			beforeRows := editionFixtureRows(t, db, common.DatabaseTypeSQLite)
			if clean {
				require.NoError(t, native.Close())
			}
			before := editionFixtureFiles(t, filepath.Dir(path))
			old := common.SQLitePath
			common.SQLitePath = path
			t.Cleanup(func() { common.SQLitePath = old })
			t.Setenv("SQL_DSN", "local")
			previous := common.PasswordLoginEnabled
			common.PasswordLoginEnabled = true
			t.Cleanup(func() { common.PasswordLoginEnabled = previous })
			facts, err := InspectEditionCompatibilityPreMigration(context.Background())
			require.NoError(t, err)
			editionAssertCase(t, facts, "terminal-and-local-candidates")
			assert.Equal(t, before, editionFixtureFiles(t, filepath.Dir(path)))
			if !clean {
				assert.Equal(t, beforeRows, editionFixtureRows(t, db, common.DatabaseTypeSQLite))
			}
		})
	}
}

func TestEditionCompatibilityCredentialStructure(t *testing.T) {
	credential, public := editionFixtureKey(t)
	require.True(t, editionPasswordCandidate(editionFixturePassword))
	require.True(t, editionPasskeyCandidate(credential, public))
	ed := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)).Public().(ed25519.PublicKey)
	for _, key := range []map[int64]any{
		{1: int64(1), 3: int64(-8), -1: int64(6), -2: []byte(ed)},
		{1: int64(3), 3: int64(-257), -1: bytes.Repeat([]byte{0xab}, 128), -2: []byte{1, 0, 1}},
	} {
		cose, err := webauthncbor.Marshal(key)
		require.NoError(t, err)
		assert.True(t, editionPasskeyCandidate(credential, base64.StdEncoding.EncodeToString(cose)))
	}
	for _, value := range []string{"", "not-a-hash", editionFixturePassword + "extra"} {
		assert.False(t, editionPasswordCandidate(value))
	}
	for _, test := range []struct{ name, credential, key string }{
		{"bad-base64", "?", public}, {"empty-id", "", public}, {"unsupported-cose", credential, base64.StdEncoding.EncodeToString([]byte{0xa0})}, {"invalid-point", credential, base64.StdEncoding.EncodeToString([]byte{0xa3, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01})},
	} {
		t.Run(test.name, func(t *testing.T) { assert.False(t, editionPasskeyCandidate(test.credential, test.key)) })
	}
}

func TestEditionCompatibilityNamedMemoryDoesNotInspectAnotherConnection(t *testing.T) {
	dsn := "file:edition-preserved-memory?mode=memory&cache=shared"
	db, err := sql.Open(sqlite.DriverName, dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec("CREATE TABLE sentinel(value TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO sentinel VALUES('preserve-memory-owner')")
	require.NoError(t, err)
	old := common.SQLitePath
	common.SQLitePath = dsn
	t.Cleanup(func() { common.SQLitePath = old })
	t.Setenv("SQL_DSN", "local")
	facts, err := InspectEditionCompatibilityPreMigration(context.Background())
	require.ErrorIs(t, err, ErrEditionCompatibilityUnverifiable)
	assert.False(t, facts.Complete)
	assert.Equal(t, "unavailable", facts.Schema)
	assert.Nil(t, facts.TopUps)
	var value string
	require.NoError(t, db.QueryRow("SELECT value FROM sentinel").Scan(&value))
	assert.Equal(t, "preserve-memory-owner", value)
}
