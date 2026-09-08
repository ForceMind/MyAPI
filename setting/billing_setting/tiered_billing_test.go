package billing_setting

import (
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedBillingSettingExportsDefaultsAndPreservesPartialUpdates(t *testing.T) {
	state := newManagedBillingSetting(defaultBillingSetting)

	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, "{}", exported["billing_mode"])
	assert.Equal(t, "{}", exported["billing_expr"])

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"billing_mode":       `{"model-a":"tiered_expr"}`,
		"unknown_legacy_key": "ignored",
	}))
	snapshot := state.snapshot()
	assert.Equal(t, BillingModeTieredExpr, snapshot.GetBillingMode("model-a"))
	assert.Empty(t, snapshot.billingExpr)
}

func TestManagedBillingSettingUsesGenericNullAndInvalidParsers(t *testing.T) {
	state := newManagedBillingSetting(BillingSetting{
		BillingMode: map[string]string{"model-a": BillingModeTieredExpr},
		BillingExpr: map[string]string{"model-a": `tier("base", p * 2)`},
	})

	require.NoError(t, state.UpdateConfigMap(map[string]string{"billing_mode": "null"}))
	snapshot := state.snapshot()
	assert.Nil(t, snapshot.billingMode)
	assert.Equal(t, `tier("base", p * 2)`, snapshot.billingExpr["model-a"])

	before := state.current.Load()
	require.Error(t, state.ValidateConfigMap(map[string]string{"billing_expr": "{"}))
	assert.Same(t, before, state.current.Load())
	require.Error(t, state.UpdateConfigMap(map[string]string{"billing_expr": `["not","a map"]`}))
	assert.Same(t, before, state.current.Load())
}

func TestManagedBillingSettingDetachesMaps(t *testing.T) {
	state := newManagedBillingSetting(BillingSetting{
		BillingMode: map[string]string{"model-a": BillingModeTieredExpr},
		BillingExpr: map[string]string{"model-a": `tier("base", p * 2)`},
	})

	snapshot := state.snapshot()
	modes := snapshot.GetBillingModeCopy()
	exprs := snapshot.GetBillingExprCopy()
	modes["model-a"] = BillingModeRatio
	exprs["model-a"] = "caller-mutation"

	current := state.snapshot()
	assert.Equal(t, BillingModeTieredExpr, current.GetBillingMode("model-a"))
	assert.Equal(t, `tier("base", p * 2)`, current.billingExpr["model-a"])
}

func TestManagedBillingSettingPublishesWholeGenerations(t *testing.T) {
	state := newManagedBillingSetting(defaultBillingSetting)
	updates := []map[string]string{
		{
			"billing_mode": `{"model-a":"tiered_expr"}`,
			"billing_expr": `{"model-a":"tier(\"first\", p * 2)"}`,
		},
		{
			"billing_mode": `{"model-a":"ratio"}`,
			"billing_expr": `{"model-a":"tier(\"second\", p * 3)"}`,
		},
	}

	var writers sync.WaitGroup
	writerErr := make(chan error, 1)
	started := make(chan struct{})
	writers.Add(1)
	go func() {
		defer writers.Done()
		for i := 0; i < 100; i++ {
			if err := state.UpdateConfigMap(updates[i%len(updates)]); err != nil {
				writerErr <- err
				return
			}
			if i == 0 {
				close(started)
			}
		}
	}()
	<-started
	for i := 0; i < 1000; i++ {
		snapshot := state.snapshot()
		mode := snapshot.GetBillingMode("model-a")
		expr, ok := snapshot.GetBillingExpr("model-a")
		assert.True(t, ok)
		assert.True(t, (mode == BillingModeTieredExpr && expr == `tier("first", p * 2)`) || (mode == BillingModeRatio && expr == `tier("second", p * 3)`))
	}
	writers.Wait()
	select {
	case err := <-writerErr:
		require.NoError(t, err)
	default:
	}
}

func TestManagedBillingSettingConfigManagerLoadAndSave(t *testing.T) {
	manager := config.NewConfigManager()
	state := newManagedBillingSetting(defaultBillingSetting)
	manager.Register("billing_setting", state)

	require.NoError(t, manager.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"model-a":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"model-a":"tier(\"base\", p * 2)"}`,
		"billing_setting.unknown":      "ignored",
	}))
	snapshot := state.snapshot()
	assert.Equal(t, BillingModeTieredExpr, snapshot.GetBillingMode("model-a"))
	assert.Equal(t, `tier("base", p * 2)`, snapshot.billingExpr["model-a"])

	saved := make(map[string]string)
	require.NoError(t, manager.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	assert.Equal(t, `{"model-a":"tiered_expr"}`, saved["billing_setting.billing_mode"])
	assert.Equal(t, `{"model-a":"tier(\"base\", p * 2)"}`, saved["billing_setting.billing_expr"])
}

func TestGlobalBillingCopiesAreDetached(t *testing.T) {
	registered := config.GlobalConfig.Get("billing_setting")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"billing_mode": `{"model-a":"tiered_expr"}`,
		"billing_expr": `{"model-a":"tier(\"base\", p * 2)"}`,
	}))
	modes := GetBillingModeCopy()
	exprs := GetBillingExprCopy()
	modes["model-a"] = BillingModeRatio
	exprs["model-a"] = "caller-mutation"

	snapshot := GetBillingSnapshot()
	assert.Equal(t, BillingModeTieredExpr, snapshot.GetBillingMode("model-a"))
	expr, ok := snapshot.GetBillingExpr("model-a")
	require.True(t, ok)
	assert.Equal(t, `tier("base", p * 2)`, expr)
}
