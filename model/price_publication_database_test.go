package model

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestPricePublicationConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/publication.db")
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
			// Isolated fixture tables; no existing installation/options touched.
			prefix := fmt.Sprintf("pp_%d_", time.Now().UnixNano())
			db, err := gorm.Open(dialector, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: prefix}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			oldDB := DB
			DB = db
			previous := snapshotFamilyConfig(t, "billing_setting")
			previousPublication := pricePublicationRuntime.Load()
			t.Cleanup(func() {
				DB = oldDB
				restoreFamilyConfig(t, "billing_setting", previous)
				pricePublicationRuntime.Store(previousPublication)
				clearPricingRuntimeUnavailable()
				require.NoError(t, db.Migrator().DropTable(&PricePublication{}, &OfficialPriceVersion{}, &Option{}))
				require.NoError(t, sqlDB.Close())
			})
			swapOptionMapForTest(t, map[string]string{})
			require.NoError(t, db.AutoMigrate(&Option{}, &OfficialPriceVersion{}, &PricePublication{}))
			require.NoError(t, db.AutoMigrate(&Option{}, &OfficialPriceVersion{}, &PricePublication{}))
			ctx := context.Background()
			source, err := StoreOfficialPriceVersion(ctx, db, "publication contract fixture", 100)
			require.NoError(t, err)
			command := publicationCommandForTest(t, ctx, "a", source.ContentSHA256)
			command.Changes[0].Locked = true
			_, err = ApplyPricePublication(ctx, command)
			require.NoError(t, err)
			_, err = ApplyPricePublication(ctx, command)
			require.NoError(t, err)
			require.ErrorIs(t, UpdateOption(publicationExpressionKey, `{}`), ErrPricePublicationLocked)
			snapshot, err := ReadPricePublicationSnapshot(ctx)
			require.NoError(t, err)
			assert.EqualValues(t, 1, snapshot.State.Revision)
			digest, err := snapshot.Digest()
			require.NoError(t, err)
			rollback := PricePublicationCommand{ID: strings.Repeat("b", 64), ActorID: 1, ExpectedDigest: digest, Action: "rollback", RollbackOf: command.ID}
			_, err = ApplyPricePublication(ctx, rollback)
			require.NoError(t, err)
			snapshot, err = ReadPricePublicationSnapshot(ctx)
			require.NoError(t, err)
			assert.Empty(t, snapshot.Expressions)
			assert.EqualValues(t, 2, snapshot.State.Revision)
			var count int64
			require.NoError(t, db.Model(&PricePublication{}).Count(&count).Error)
			assert.EqualValues(t, 2, count)
		})
	}
}
