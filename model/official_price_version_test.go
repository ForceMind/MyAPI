package model

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestOfficialPriceVersionRetainsContentAndFirstFetch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/prices.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&OfficialPriceVersion{}))
	require.NoError(t, db.AutoMigrate(&OfficialPriceVersion{}))
	ctx := context.Background()
	// A document above MySQL TEXT's 64 KiB limit must remain representable.
	document := strings.Repeat("public price source\n", 4000)
	first, err := StoreOfficialPriceVersion(ctx, db, document, 100)
	require.NoError(t, err)
	second, err := StoreOfficialPriceVersion(ctx, db, document, 200)
	require.NoError(t, err)
	assert.Equal(t, first.ContentSHA256, second.ContentSHA256)
	assert.EqualValues(t, 100, second.FetchedAt)
	loaded, err := GetOfficialPriceVersion(ctx, db, first.ContentSHA256)
	require.NoError(t, err)
	assert.Equal(t, document, loaded.Document)
	_, err = StoreOfficialPriceVersion(ctx, db, document+"new", 300)
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.Model(&OfficialPriceVersion{}).Count(&count).Error)
	assert.EqualValues(t, 2, count)
	require.NoError(t, db.Model(&OfficialPriceVersion{}).Where("content_sha256 = ?", first.ContentSHA256).Update("document", "tampered").Error)
	_, err = GetOfficialPriceVersion(ctx, db, first.ContentSHA256)
	require.Error(t, err)
	_, err = StoreOfficialPriceVersion(ctx, db, document, 400)
	require.Error(t, err, "idempotent save must not overwrite conflicting stored evidence")
}

func TestOfficialPriceDocumentTypesHaveCapacityAcrossDialects(t *testing.T) {
	parsed, err := schema.Parse(&OfficialPriceVersion{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	for _, test := range []struct {
		name         string
		dialector    gorm.Dialector
		documentType string
	}{
		{"mysql", mysql.New(mysql.Config{SkipInitializeWithVersion: true}), "mediumtext"},
		{"postgres", postgres.New(postgres.Config{}), "varchar(1048576)"},
		{"sqlite", sqlite.Open(":memory:"), "text"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.documentType, test.dialector.DataTypeOf(parsed.FieldsByName["Document"]))
		})
	}
}

func TestOfficialPriceVersionConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{
		{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"},
	} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/contract.db")
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
			db, err := gorm.Open(dialector, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.AutoMigrate(&OfficialPriceVersion{}))
			require.NoError(t, db.AutoMigrate(&OfficialPriceVersion{}))
			// Timestamp is solely a fixture namespace, not a timing assertion.
			document := strings.Repeat("public pricing fixture\n", 4000) + fmt.Sprintf("%s:%d", t.Name(), time.Now().UnixNano())
			version, err := StoreOfficialPriceVersion(context.Background(), db, document, 100)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, db.Where("content_sha256 = ?", version.ContentSHA256).Delete(&OfficialPriceVersion{}).Error)
			})
			replay, err := StoreOfficialPriceVersion(context.Background(), db, document, 200)
			require.NoError(t, err)
			assert.EqualValues(t, 100, replay.FetchedAt)
			loaded, err := GetOfficialPriceVersion(context.Background(), db, version.ContentSHA256)
			require.NoError(t, err)
			assert.Equal(t, document, loaded.Document)
			_, err = GetOfficialPriceVersion(context.Background(), db, strings.ToUpper(version.ContentSHA256))
			require.Error(t, err, "digest lookup semantics must not depend on database collation")
		})
	}
}
