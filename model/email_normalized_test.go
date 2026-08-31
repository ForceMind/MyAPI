package model

import (
	"errors"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newEmailMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previousDB := DB
	previousType := common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
	})
	return db
}

func TestEnsureUserNormalizedEmailBackfillsMixedCaseAndAllowsEmpty(t *testing.T) {
	db := newEmailMigrationDB(t)
	require.NoError(t, db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, email VARCHAR(50))").Error)
	require.NoError(t, db.Exec("INSERT INTO users (id, email) VALUES (1, '  Mixed@Example.COM '), (2, ''), (3, NULL)").Error)

	require.NoError(t, ensureUserNormalizedEmail())
	var normalized string
	require.NoError(t, db.Table("users").Where("id = 1").Pluck("email_normalized", &normalized).Error)
	require.Equal(t, "mixed@example.com", normalized)
	var nullCount int64
	require.NoError(t, db.Table("users").Where("id IN (2, 3) AND email_normalized IS NULL").Count(&nullCount).Error)
	require.Equal(t, int64(2), nullCount)
	// A second startup migration is idempotent.
	require.NoError(t, ensureUserNormalizedEmail())
}

func TestEnsureUserNormalizedEmailFailsOnLegacyConflict(t *testing.T) {
	db := newEmailMigrationDB(t)
	require.NoError(t, db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, email VARCHAR(50))").Error)
	require.NoError(t, db.Exec("INSERT INTO users (id, email) VALUES (1, 'User@Example.com'), (2, ' user@example.com ')").Error)

	err := ensureUserNormalizedEmail()
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrEmailAlreadyTaken))
	require.Contains(t, err.Error(), "user@example.com")
}

func TestEnsureUserNormalizedEmailFailsOnOversizedLegacyValue(t *testing.T) {
	db := newEmailMigrationDB(t)
	require.NoError(t, db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, email VARCHAR(255))").Error)
	require.NoError(t, db.Exec("INSERT INTO users (id, email) VALUES (1, ?)", "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz@example.com").Error)

	err := ensureUserNormalizedEmail()
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds 50")
}

func TestUserEmailNormalizedIsWrittenByCreateAndUpdate(t *testing.T) {
	db := newEmailMigrationDB(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, ensureUserNormalizedEmail())
	user := &User{Username: "email-key-user", Password: "hash", Email: " Mixed@Example.com "}
	require.NoError(t, db.Create(user).Error)
	var key string
	require.NoError(t, db.Table("users").Where("id = ?", user.Id).Pluck("email_normalized", &key).Error)
	require.Equal(t, "mixed@example.com", key)
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("email", "New@Example.COM").Error)
	require.NoError(t, db.Table("users").Where("id = ?", user.Id).Pluck("email_normalized", &key).Error)
	require.Equal(t, "new@example.com", key)
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("email", "").Error)
	var row struct {
		Value *string `gorm:"column:email_normalized"`
	}
	require.NoError(t, db.Table("users").Select("email_normalized").Where("id = ?", user.Id).Scan(&row).Error)
	require.Nil(t, row.Value)
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{"email": " Map@Example.COM "}).Error)
	require.NoError(t, db.Table("users").Where("id = ?", user.Id).Pluck("email_normalized", &key).Error)
	require.Equal(t, "map@example.com", key)
}

func TestEmailReadPathsUseNormalizedKeyForMixedCaseLegacyRows(t *testing.T) {
	db := newEmailMigrationDB(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	// Simulate a legacy mixed-case row immediately after the compatibility
	// column has been introduced but before its backfill runs.
	require.NoError(t, db.Exec("INSERT INTO users (username, password, email) VALUES (?, ?, ?)", "legacy-read-user", "hash", "Legacy@Example.COM").Error)
	require.NoError(t, ensureUserNormalizedEmail())

	var user User
	user.Email = " LEGACY@example.com "
	require.NoError(t, user.FillUserByEmail())
	require.Equal(t, "legacy-read-user", user.Username)
	found, err := GetUniqueUserByEmail("LEGACY@EXAMPLE.COM")
	require.NoError(t, err)
	require.Equal(t, user.Id, found.Id)
	count, err := CountUsersByEmail(" legacy@example.com ")
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}
