package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestTokenBudgetConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/budget.db")
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
			db, err := gorm.Open(dialector, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("tb_%d_", time.Now().UnixNano())}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&TokenBudgetPolicyChange{}, &TokenBudgetReservation{}, &TokenBudget{}, &Token{}, &User{}))
				require.NoError(t, sqlDB.Close())
			})
			seedTokenBudgetDB(t, db)
			require.NoError(t, db.AutoMigrate(&TokenBudget{}, &TokenBudgetReservation{}, &TokenBudgetPolicyChange{}))
			tokenBudgetLifecycleContract(t, db)
			feeBudgetConfiguredLifecycle(t, db)
		})
	}
}
