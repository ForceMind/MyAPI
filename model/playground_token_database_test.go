package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestPlaygroundTokenConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/playground-token.db")
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
			db, err := gorm.Open(dialector, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("pgkey_%d_", time.Now().UnixNano())}})
			require.NoError(t, err)
			conn, err := db.DB()
			require.NoError(t, err)
			previousDB, previousRedis := DB, common.RedisEnabled
			DB, common.RedisEnabled = db, false
			t.Cleanup(func() {
				DB, common.RedisEnabled = previousDB, previousRedis
				require.NoError(t, db.Migrator().DropTable(&Token{}))
				require.NoError(t, conn.Close())
			})
			require.NoError(t, db.AutoMigrate(&Token{}))
			key := &Token{UserId: 7, Key: "synthetic_owned_key", Name: "Owned", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100}
			require.NoError(t, db.Create(key).Error)
			loaded, err := GetTokenByIds(key.Id, 7)
			require.NoError(t, err)
			require.NoError(t, ValidateTokenSnapshot(loaded))
			require.Equal(t, key.Id, loaded.Id)
			_, err = GetTokenByIds(key.Id, 8)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			require.NoError(t, db.Model(key).Update("expired_time", time.Now().Add(-time.Hour).Unix()).Error)
			loaded, err = GetTokenByIds(key.Id, 7)
			require.NoError(t, err)
			require.ErrorIs(t, ValidateTokenSnapshot(loaded), ErrTokenInvalid)
			require.NoError(t, db.First(loaded, key.Id).Error)
			require.Equal(t, common.TokenStatusExpired, loaded.Status)
			require.NoError(t, db.Model(key).Updates(map[string]any{"expired_time": int64(-1), "status": common.TokenStatusEnabled, "remain_quota": 0}).Error)
			loaded, err = GetTokenByIds(key.Id, 7)
			require.NoError(t, err)
			require.ErrorIs(t, ValidateTokenSnapshot(loaded), ErrTokenInvalid)
			require.NoError(t, db.First(loaded, key.Id).Error)
			require.Equal(t, common.TokenStatusExhausted, loaded.Status)
			require.NoError(t, db.Delete(key).Error)
			_, err = GetTokenByIds(key.Id, 7)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		})
	}
}
