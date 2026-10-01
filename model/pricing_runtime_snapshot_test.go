package model

import (
	"errors"
	"github.com/ForceMind/MyAPI/common"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelPricingSnapshotFollowsCommittedOptionBatch(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	swapOptionMapForTest(t, map[string]string{})
	previousModelRatio := ratio_setting.ModelRatio2JSONString()
	previousCompletionRatio := ratio_setting.CompletionRatio2JSONString()
	previousCacheRatio := ratio_setting.CacheRatio2JSONString()
	previousBilling := snapshotFamilyConfig(t, "billing_setting")
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatio))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(previousCompletionRatio))
		require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(previousCacheRatio))
		restoreFamilyConfig(t, "billing_setting", previousBilling)
	})

	const modelName = "pricing-bulk-snapshot-fixture"
	first := map[string]string{
		"ModelRatio":                   `{"pricing-bulk-snapshot-fixture":2}`,
		"CompletionRatio":              `{"pricing-bulk-snapshot-fixture":3}`,
		"CacheRatio":                   `{"pricing-bulk-snapshot-fixture":0.5}`,
		"billing_setting.billing_mode": `{"pricing-bulk-snapshot-fixture":"ratio"}`,
		"billing_setting.billing_expr": `{"pricing-bulk-snapshot-fixture":"tier(\"base\", p * 2)"}`,
	}
	require.NoError(t, UpdateOptionsBulk(first))
	before := CaptureModelPricingSnapshot(modelName)
	assert.False(t, before.HasModelPrice)
	assert.True(t, before.HasModelRatio)
	assert.Equal(t, 2.0, before.ModelRatio)
	assert.Equal(t, 3.0, before.CompletionRatio)
	assert.Equal(t, 0.5, before.CacheRatio)
	assert.Equal(t, "ratio", before.Billing.GetBillingMode(modelName))
	beforeExpr, ok := before.Billing.GetBillingExpr(modelName)
	require.True(t, ok)
	assert.Equal(t, `tier("base", p * 2)`, beforeExpr)

	writeErr := errors.New("injected pricing save failure")
	const callbackName = "test:pricing-snapshot-save-failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		tx.AddError(writeErr)
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove(callbackName)) })
	require.ErrorIs(t, UpdateOptionsBulk(map[string]string{
		"ModelRatio":      `{"pricing-bulk-snapshot-fixture":7}`,
		"CompletionRatio": `{"pricing-bulk-snapshot-fixture":9}`,
	}), writeErr)

	after := CaptureModelPricingSnapshot(modelName)
	assert.Equal(t, before.ModelRatio, after.ModelRatio)
	assert.Equal(t, before.CompletionRatio, after.CompletionRatio)
	assert.Equal(t, before.CacheRatio, after.CacheRatio)
	assert.Equal(t, before.Billing.GetBillingMode(modelName), after.Billing.GetBillingMode(modelName))
	var persisted Option
	require.NoError(t, db.Where("key = ?", "ModelRatio").First(&persisted).Error)
	assert.Equal(t, first["ModelRatio"], persisted.Value)
}

func TestModelPricingCaptureCannotObservePartialBatchPublication(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	swapOptionMapForTest(t, map[string]string{})
	previousModelRatio := ratio_setting.ModelRatio2JSONString()
	previousCompletionRatio := ratio_setting.CompletionRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatio))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(previousCompletionRatio))
	})

	const modelName = "pricing-capture-batch-fixture"
	require.NoError(t, UpdateOptionsBulk(map[string]string{
		"ModelRatio":      `{"pricing-capture-batch-fixture":2}`,
		"CompletionRatio": `{"pricing-capture-batch-fixture":3}`,
	}))
	reached := make(chan struct{})
	release := make(chan struct{})
	resume := sync.OnceFunc(func() { close(release) })
	previousPublisher := modelPricingOptionPublish
	modelPricingOptionPublish = func(key, value string) error {
		if err := previousPublisher(key, value); err != nil {
			return err
		}
		if key == "ModelRatio" {
			close(reached)
			<-release
		}
		return nil
	}
	var worker sync.WaitGroup
	t.Cleanup(func() {
		resume()
		worker.Wait()
		modelPricingOptionPublish = previousPublisher
	})
	result := make(chan error, 1)
	worker.Add(1)
	go func() {
		defer worker.Done()
		result <- UpdateOptionsBulk(map[string]string{
			"ModelRatio":      `{"pricing-capture-batch-fixture":7}`,
			"CompletionRatio": `{"pricing-capture-batch-fixture":9}`,
		})
	}()
	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		t.Fatal("pricing batch did not reach the publication barrier")
	}
	if pricingRuntimeMu.TryRLock() {
		pricingRuntimeMu.RUnlock()
		t.Fatal("a relay could capture pricing between fields of one batch")
	}
	resume()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("pricing batch did not finish")
	}
	snapshot := CaptureModelPricingSnapshot(modelName)
	assert.Equal(t, 7.0, snapshot.ModelRatio)
	assert.Equal(t, 9.0, snapshot.CompletionRatio)
}

func TestModelPricingReloadPublishesCommittedFieldsTogether(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	swapOptionMapForTest(t, map[string]string{})
	previousModelRatio := ratio_setting.ModelRatio2JSONString()
	previousCompletionRatio := ratio_setting.CompletionRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatio))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(previousCompletionRatio))
	})
	require.NoError(t, db.Create(&[]Option{
		{Key: "ModelRatio", Value: `{"pricing-reload-fixture":4}`},
		{Key: "CompletionRatio", Value: `{"pricing-reload-fixture":6}`},
	}).Error)

	loadOptionsFromDatabase()
	snapshot := CaptureModelPricingSnapshot("pricing-reload-fixture")
	assert.True(t, snapshot.HasModelRatio)
	assert.Equal(t, 4.0, snapshot.ModelRatio)
	assert.Equal(t, 6.0, snapshot.CompletionRatio)
}

func TestModelPricingInvalidExpressionRejectsEntireOptionBatch(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	swapOptionMapForTest(t, map[string]string{})
	previousModelRatio := ratio_setting.ModelRatio2JSONString()
	previousBilling := snapshotFamilyConfig(t, "billing_setting")
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatio))
		restoreFamilyConfig(t, "billing_setting", previousBilling)
	})

	require.Error(t, UpdateOptionsBulk(map[string]string{
		"ModelRatio":                   `{"pricing-invalid-expr-fixture":7}`,
		"billing_setting.billing_expr": `{"pricing-invalid-expr-fixture":"tier(\"base\", p / (c - c))"}`,
	}))
	assert.Zero(t, countOptions(t, db))
	assert.Equal(t, previousModelRatio, ratio_setting.ModelRatio2JSONString())
	assert.Equal(t, previousBilling, snapshotFamilyConfig(t, "billing_setting"))
}

func TestPricingPostCommitFailureBlocksQuotesUntilCompleteReload(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	swapOptionMapForTest(t, map[string]string{})
	previousFault, previousUnit := pricingRuntimeFault.Load(), common.QuotaPerUnit
	previousRatio := ratio_setting.ModelRatio2JSONString()
	previousCompletion := ratio_setting.CompletionRatio2JSONString()
	previousPublisher := modelPricingOptionPublish
	pricingRuntimeFault.Store(false)
	t.Cleanup(func() {
		modelPricingOptionPublish = previousPublisher
		pricingRuntimeFault.Store(previousFault)
		common.QuotaPerUnit = previousUnit
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousRatio))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(previousCompletion))
	})
	require.NoError(t, UpdateOptionsBulk(map[string]string{"QuotaPerUnit": "100", "ModelRatio": `{"frozen":2}`, "CompletionRatio": `{"frozen":3}`}))
	publicationErr := errors.New("synthetic publication failed")
	modelPricingOptionPublish = func(key, value string) error {
		if key == "CompletionRatio" {
			return publicationErr
		}
		return previousPublisher(key, value)
	}
	require.ErrorIs(t, UpdateOptionsBulk(map[string]string{"QuotaPerUnit": "200", "ModelRatio": `{"frozen":7}`, "CompletionRatio": `{"frozen":9}`}), publicationErr)
	var committed Option
	require.NoError(t, db.Where("key = ?", "CompletionRatio").First(&committed).Error)
	assert.Equal(t, `{"frozen":9}`, committed.Value, "a publication error is not a database rollback")
	_, err := CaptureQuotaUnitForOrder()
	require.ErrorIs(t, err, ErrPricingRuntimeUnavailable)
	release, err := AcquirePricingRuntimeRead()
	require.ErrorIs(t, err, ErrPricingRuntimeUnavailable)
	assert.Nil(t, release)
	require.NoError(t, UpdateOption("SystemName", "unrelated write"))
	assert.False(t, PricingRuntimeReady(), "an unrelated write must not clear a partial pricing generation")
	modelPricingOptionPublish = previousPublisher
	loadOptionsFromDatabase()
	unit, err := CaptureQuotaUnitForOrder()
	require.NoError(t, err)
	assert.Equal(t, "200", unit)
	snapshot := CaptureModelPricingSnapshot("frozen")
	assert.False(t, snapshot.Unavailable)
	assert.Equal(t, 7.0, snapshot.ModelRatio)
	assert.Equal(t, 9.0, snapshot.CompletionRatio)
}
