package model

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These fixtures reproduce populated quota columns without quota_version.
// They test additive migration, not a complete historical deployment upgrade.
type taskQuotaLegacyUser struct {
	Id          int
	Username    string
	Password    string
	AffCode     string
	AccessToken *string
	Quota       int
	UsedQuota   int
}

func (taskQuotaLegacyUser) TableName() string { return "users" }

type taskQuotaLegacyToken struct {
	Id          int
	UserId      int
	Key         string
	RemainQuota int
	UsedQuota   int
}

func (taskQuotaLegacyToken) TableName() string { return "tokens" }

type taskQuotaLegacySubscription struct {
	Id          int
	UserId      int
	AmountTotal int64 `gorm:"type:bigint;not null;default:0"`
	AmountUsed  int64 `gorm:"type:bigint;not null;default:0"`
}

func (taskQuotaLegacySubscription) TableName() string { return "user_subscriptions" }

func createTaskQuotaLegacyFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&taskQuotaLegacyUser{}, &taskQuotaLegacyToken{}, &taskQuotaLegacySubscription{}))
	require.NoError(t, db.Create(&taskQuotaLegacyUser{Id: 41001, Username: "qr-legacy", Password: "synthetic-fixture", AffCode: "qr-legacy", Quota: 900, UsedQuota: 100}).Error)
	require.NoError(t, db.Create(&taskQuotaLegacyToken{Id: 41001, UserId: 41001, Key: "qr-legacy", RemainQuota: 700, UsedQuota: 300}).Error)
	require.NoError(t, db.Create(&taskQuotaLegacySubscription{Id: 41001, UserId: 41001, AmountTotal: 1200, AmountUsed: 400}).Error)
}

func assertTaskQuotaLegacyFixtureMigrated(t *testing.T, db *gorm.DB) {
	t.Helper()
	var user User
	var token Token
	var subscription UserSubscription
	require.NoError(t, db.First(&user, 41001).Error)
	require.NoError(t, db.First(&token, 41001).Error)
	require.NoError(t, db.First(&subscription, 41001).Error)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 100, user.UsedQuota)
	assert.Zero(t, user.QuotaVersion)
	assert.Equal(t, 700, token.RemainQuota)
	assert.Equal(t, 300, token.UsedQuota)
	assert.Zero(t, token.QuotaVersion)
	assert.Equal(t, int64(1200), subscription.AmountTotal)
	assert.Equal(t, int64(400), subscription.AmountUsed)
	assert.Zero(t, subscription.QuotaVersion)
}

func TestTaskQuotaReservationLegacyMigrationSQLite(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	createTaskQuotaLegacyFixture(t, db)
	for range 2 {
		require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &UserSubscription{}))
		assertTaskQuotaLegacyFixtureMigrated(t, db)
	}
}

func TestTaskQuotaReservationLegacyMigrationConfiguredDatabases(t *testing.T) {
	if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
		t.Skip("disposable B2 database tests require MYAPI_B2_DATABASE_TESTS=1")
	}
	for _, engine := range []struct{ name, env string }{
		{"mysql", "MYAPI_B2_MYSQL_DSN"},
		{"postgres", "MYAPI_B2_POSTGRES_DSN"},
	} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			require.NotEmpty(t, dsn, "%s must be configured when MYAPI_B2_DATABASE_TESTS=1", engine.env)
			dialector, err := b2SubmissionDatabaseDialector(engine.name, dsn)
			require.NoError(t, err)
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			require.NoError(t, registerTaskRecoveryGormGuards(db))
			createTaskQuotaLegacyFixture(t, db)
			for range 2 {
				require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &UserSubscription{}))
				assertTaskQuotaLegacyFixtureMigrated(t, db)
			}
		})
	}
}
