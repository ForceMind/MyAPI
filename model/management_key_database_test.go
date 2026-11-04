package model

import (
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
	"gorm.io/gorm/schema"
)

func TestManagementKeyConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/management-key.db")
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
			db, err := gorm.Open(dialector, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("mgkey_%d_", time.Now().UnixNano())}})
			require.NoError(t, err)
			conn, err := db.DB()
			require.NoError(t, err)
			previousDB, previousRedis, previousType := DB, common.RedisEnabled, common.MainDatabaseType()
			DB, common.RedisEnabled = db, false
			switch engine.name {
			case "mysql":
				common.SetMainDatabaseType(common.DatabaseTypeMySQL)
			case "postgres":
				common.SetMainDatabaseType(common.DatabaseTypePostgreSQL)
			default:
				common.SetMainDatabaseType(common.DatabaseTypeSQLite)
			}
			t.Cleanup(func() {
				DB, common.RedisEnabled = previousDB, previousRedis
				common.SetMainDatabaseType(previousType)
				require.NoError(t, db.Migrator().DropTable(&ManagementKeyAudit{}, &ManagementKey{}, &User{}))
				require.NoError(t, conn.Close())
			})
			require.NoError(t, db.AutoMigrate(&User{}, &ManagementKey{}, &ManagementKeyAudit{}))
			owner := &User{Username: "mgmt-owner", Password: "unused", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AuthVersion: 1, AffCode: "mgmt-aff"}
			require.NoError(t, db.Create(owner).Error)
			now := time.Now().Unix()
			secret := ManagementKeyPrefix + strings.Repeat("a", 64)
			key := &ManagementKey{OwnerID: owner.Id, Name: "test", Prefix: secret[:19], SecretHash: ManagementKeyDigest(secret), ScopesJSON: `["tokens.read"]`, ChannelIDsJSON: `[]`, TokenIDsJSON: `[]`, CreatedAt: now, ExpiresAt: now + 3600}
			require.NoError(t, InsertManagementKey(key))
			// Repeated additive migration must preserve both credential identity and audit.
			require.NoError(t, db.AutoMigrate(&ManagementKey{}, &ManagementKeyAudit{}))
			loaded, user, err := FindLiveManagementKey(secret, now)
			require.NoError(t, err)
			assert.Equal(t, key.ID, loaded.ID)
			assert.Equal(t, owner.Id, user.Id)
			assert.NotEqual(t, secret, loaded.SecretHash)
			assert.Len(t, loaded.SecretHash, 64)
			raw, err := common.Marshal(loaded)
			require.NoError(t, err)
			assert.NotContains(t, string(raw), secret)
			assert.NotContains(t, string(raw), loaded.SecretHash)
			rows, err := ListManagementKeys(owner.Id + 1)
			require.NoError(t, err)
			assert.Empty(t, rows)
			require.ErrorIs(t, RevokeManagementKey(owner.Id+1, key.ID, now), gorm.ErrRecordNotFound)
			_, _, err = FindLiveManagementKey(secret, now+3600)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			require.NoError(t, RevokeManagementKey(owner.Id, key.ID, now+1))
			require.NoError(t, RevokeManagementKey(owner.Id, key.ID, now+2))
			_, _, err = FindLiveManagementKey(secret, now+2)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			rows, err = ListManagementKeys(owner.Id)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			assert.Equal(t, now+1, rows[0].RevokedAt)
			var audit []ManagementKeyAudit
			require.NoError(t, db.Order("id").Find(&audit).Error)
			require.Len(t, audit, 2)
			assert.Equal(t, "created", audit[0].Action)
			assert.Equal(t, "revoked", audit[1].Action)
			for _, entry := range audit {
				assert.Equal(t, key.ID, entry.KeyID)
				assert.Equal(t, owner.Id, entry.OwnerID)
			}
		})
	}
}
