package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFrozenOfficialSourceCostCannotUseMutableDisplayedRates(t *testing.T) {
	path := t.TempDir() + "/frozen.db"
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.OfficialPriceVersion{}))
	document := officialPriceFixtureHeader + officialPriceFixtureRow
	digest := sha256.Sum256([]byte(document))
	snapshot, _ := textCostFixture()
	snapshot.sourceDocument = document
	snapshot.ContentSHA256 = hex.EncodeToString(digest[:])
	snapshot.FetchedAt = 100
	snapshot.Models[0].ShortContext.Input = "999" // mutable displayed DTO is not the evidence
	ctx := context.Background()
	frozen, err := FreezeOpenAIPriceSource(ctx, db, snapshot)
	require.NoError(t, err)
	assert.Equal(t, "2.00", frozen.Models[0].ShortContext.Input)
	frozen.Models[0].ShortContext.Input = "123"
	*snapshot.Models[0].ShortContext.CacheWrite = "999"
	_, usage := textCostFixture()
	cost, err := CalculateFrozenOpenAITextCost(ctx, db, frozen.ContentSHA256, "fixture-model", "short", usage)
	require.NoError(t, err)
	// Fixture document quotes cache-read .10, not the display fixture's .20.
	assert.Equal(t, "0.000239", cost.TotalUSD)
	assert.Equal(t, frozen.ContentSHA256, cost.SourceSHA256)
	require.NoError(t, sqlDB.Close())
	db, err = gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err = db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.OfficialPriceVersion{}))
	cost, err = CalculateFrozenOpenAITextCost(ctx, db, frozen.ContentSHA256, "fixture-model", "short", usage)
	require.NoError(t, err)
	assert.Equal(t, "0.000239", cost.TotalUSD, "reopening the database must not switch the frozen source")
	snapshot.FetchedAt = 200
	replay, err := FreezeOpenAIPriceSource(ctx, db, snapshot)
	require.NoError(t, err)
	assert.EqualValues(t, 100, replay.FetchedAt)
	snapshot.ContentSHA256 = strings.Repeat("b", 64)
	_, err = FreezeOpenAIPriceSource(ctx, db, snapshot)
	require.Error(t, err)
	clientPosted, _ := textCostFixture()
	_, err = FreezeOpenAIPriceSource(ctx, db, clientPosted)
	require.Error(t, err, "JSON/client-only prices have no retained server source")
	_, err = CalculateFrozenOpenAITextCost(ctx, db, strings.Repeat("0", 64), "fixture-model", "short", usage)
	require.Error(t, err, "unknown version must not fall back to latest rates")
}
