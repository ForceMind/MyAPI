package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/billing_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func pricePublicationTestSetup(t *testing.T) (context.Context, string) {
	t.Helper()
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}, &OfficialPriceVersion{}, &PricePublication{}))
	swapOptionMapForTest(t, map[string]string{})
	previous := snapshotFamilyConfig(t, "billing_setting")
	previousPublication := pricePublicationRuntime.Load()
	t.Cleanup(func() {
		restoreFamilyConfig(t, "billing_setting", previous)
		pricePublicationRuntime.Store(previousPublication)
		clearPricingRuntimeUnavailable()
	})
	require.NoError(t, UpdateOptionsBulk(map[string]string{publicationModeKey: `{"untouched":"ratio"}`, publicationExpressionKey: `{"untouched":"p * 7"}`}))
	source, err := StoreOfficialPriceVersion(context.Background(), db, "fixture source document", 100)
	require.NoError(t, err)
	return context.Background(), source.ContentSHA256
}

func publicationCommandForTest(t *testing.T, ctx context.Context, id, source string) PricePublicationCommand {
	t.Helper()
	snapshot, err := ReadPricePublicationSnapshot(ctx)
	require.NoError(t, err)
	digest, err := snapshot.Digest()
	require.NoError(t, err)
	return PricePublicationCommand{ID: strings.Repeat(id, 64), ActorID: 1, ExpectedDigest: digest, Action: "publish", Changes: []PricePublicationChange{{Model: "fixture", Expression: `tier("base", p * 2 + c * 3)`, SourceSHA256: source}}}
}

func TestPricePublicationAtomicApplyIdempotencyAndRollback(t *testing.T) {
	ctx, source := pricePublicationTestSetup(t)
	command := publicationCommandForTest(t, ctx, "a", source)
	receipt, err := ApplyPricePublication(ctx, command)
	require.NoError(t, err)
	assert.Equal(t, command.ID, receipt.ID)
	assert.Equal(t, "tiered_expr", billing_setting.GetBillingMode("fixture"))
	after, err := ReadPricePublicationSnapshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, "p * 7", after.Expressions["untouched"])
	assert.Equal(t, source, after.State.Models["fixture"].SourceSHA256)
	ref, ok := PublishedModelPriceForExpression("fixture", command.Changes[0].Expression)
	require.True(t, ok)
	assert.Equal(t, receipt.ID, ref.PublicationID)
	_, ok = PublishedModelPriceForExpression("fixture", "p * 999")
	assert.False(t, ok, "manual prices must not inherit an official source claim")
	assert.EqualValues(t, 1, after.State.Revision)
	_, err = ApplyPricePublication(ctx, command)
	require.NoError(t, err)
	var count int64
	require.NoError(t, DB.Model(&PricePublication{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	digest, err := after.Digest()
	require.NoError(t, err)
	rollback := PricePublicationCommand{ID: strings.Repeat("b", 64), ActorID: 1, ExpectedDigest: digest, Action: "rollback", RollbackOf: receipt.ID}
	_, err = ApplyPricePublication(ctx, rollback)
	require.NoError(t, err)
	rolled, err := ReadPricePublicationSnapshot(ctx)
	require.NoError(t, err)
	assert.NotContains(t, rolled.Expressions, "fixture")
	assert.Equal(t, "p * 7", rolled.Expressions["untouched"])
	assert.EqualValues(t, 2, rolled.State.Revision)
	_, err = ApplyPricePublication(ctx, command)
	require.NoError(t, err, "old retries return receipt without republishing old prices")
	assert.Equal(t, "ratio", billing_setting.GetBillingMode("fixture"))
}

func TestPricePublicationLocksAndManualChangesProtectRollback(t *testing.T) {
	ctx, source := pricePublicationTestSetup(t)
	command := publicationCommandForTest(t, ctx, "a", source)
	command.Changes[0].Locked = true
	_, err := ApplyPricePublication(ctx, command)
	require.NoError(t, err)
	require.ErrorIs(t, UpdateOption(publicationExpressionKey, `{"fixture":"p * 999"}`), ErrPricePublicationLocked)
	require.ErrorIs(t, UpdateOption(publicationExpressionKey, `{}`), ErrPricePublicationLocked)
	require.ErrorIs(t, UpdateOption(publicationModeKey, `{}`), ErrPricePublicationLocked)
	encoded, err := common.Marshal(`{"fixture":"p * 999"}`)
	require.NoError(t, err)
	_, _, err = UpdateOptionsTypedBulk([]TypedBulkOption{{Key: publicationExpressionKey, Type: TypedBulkValueTypeString, Value: encoded}}, nil)
	require.ErrorIs(t, err, ErrPricePublicationLocked)
	require.Error(t, UpdateOption(pricePublicationStateKey, `{}`))
	lock := publicationCommandForTest(t, ctx, "b", source)
	lock.Action = "lock"
	lock.Changes = []PricePublicationChange{{Model: "fixture", Locked: false}}
	_, err = ApplyPricePublication(ctx, lock)
	require.NoError(t, err)
	require.NoError(t, UpdateOption(publicationExpressionKey, `{"fixture":"p * 5","untouched":"p * 7"}`))
	rollback := publicationCommandForTest(t, ctx, "c", source)
	rollback.Action = "rollback"
	rollback.Changes = nil
	rollback.RollbackOf = lock.ID
	_, err = ApplyPricePublication(ctx, rollback)
	require.ErrorIs(t, err, ErrPricePublicationConflict)
	assert.Equal(t, "p * 5", billing_setting.GetBillingExprCopy()["fixture"])
}

func TestPricePublicationFailureDoesNotInventRollback(t *testing.T) {
	ctx, source := pricePublicationTestSetup(t)
	command := publicationCommandForTest(t, ctx, "a", source)
	fault := errors.New("injected publication write failure")
	const callback = "test:price-publication-write-failure"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) { tx.AddError(fault) }))
	_, err := ApplyPricePublication(ctx, command)
	require.ErrorIs(t, err, fault)
	require.NoError(t, DB.Callback().Update().Remove(callback))
	var count int64
	require.NoError(t, DB.Model(&PricePublication{}).Count(&count).Error)
	assert.Zero(t, count)
	before, err := ReadPricePublicationSnapshot(ctx)
	require.NoError(t, err)
	assert.Zero(t, before.State.Revision)
	assert.NotContains(t, before.Expressions, "fixture")
	previousPublisher := modelPricingOptionPublish
	t.Cleanup(func() { modelPricingOptionPublish = previousPublisher })
	modelPricingOptionPublish = func(string, string) error { return fault }
	receipt, err := ApplyPricePublication(ctx, command)
	require.ErrorIs(t, err, fault)
	require.NotNil(t, receipt, "post-commit error must include the durable receipt")
	assert.False(t, PricingRuntimeReady())
	require.NoError(t, DB.Model(&PricePublication{}).Count(&count).Error)
	assert.EqualValues(t, 1, count, "runtime failure cannot undo the database commit")
	modelPricingOptionPublish = previousPublisher
	loadOptionsFromDatabase()
	assert.True(t, PricingRuntimeReady())
	assert.Equal(t, "tiered_expr", billing_setting.GetBillingMode("fixture"))
	ref, ok := PublishedModelPriceForExpression("fixture", command.Changes[0].Expression)
	require.True(t, ok)
	assert.Equal(t, command.ID, ref.PublicationID)
}

func TestPricePublicationRejectsStaleOrReusedIdentity(t *testing.T) {
	ctx, source := pricePublicationTestSetup(t)
	first := publicationCommandForTest(t, ctx, "a", source)
	stale := publicationCommandForTest(t, ctx, "b", source)
	_, err := ApplyPricePublication(ctx, first)
	require.NoError(t, err)
	_, err = ApplyPricePublication(ctx, stale)
	require.ErrorIs(t, err, ErrPricePublicationConflict)
	first.ActorID = 2
	_, err = ApplyPricePublication(ctx, first)
	require.ErrorIs(t, err, ErrPricePublicationConflict)
}
