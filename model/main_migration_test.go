package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEnsureSubscriptionPlanTableSQLiteAddsRequiredColumnsToExistingRows(t *testing.T) {
	previousDB := DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
	})

	// An older installation can have rows while missing newly introduced
	// required columns. SQLite only permits adding NOT NULL columns when a
	// default is supplied.
	require.NoError(t, db.Exec("CREATE TABLE subscription_plans (id INTEGER PRIMARY KEY)").Error)
	require.NoError(t, db.Exec("INSERT INTO subscription_plans (id) VALUES (1)").Error)
	require.NoError(t, ensureSubscriptionPlanTableSQLite())

	var plan SubscriptionPlan
	require.NoError(t, db.First(&plan, 1).Error)
	require.Empty(t, plan.Title)
	require.Zero(t, plan.PriceAmount)
	require.NoError(t, ensureSubscriptionPlanTableSQLite())
	var count int64
	require.NoError(t, db.Model(&SubscriptionPlan{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}
