package config

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testConfigWithMap struct {
	Modes map[string]string `json:"modes"`
	Exprs map[string]string `json:"exprs"`
	Name  string            `json:"name"`
}

func TestUpdateConfigFromMap_MapReplacement(t *testing.T) {
	cfg := &testConfigWithMap{
		Modes: map[string]string{
			"model-a": "tiered_expr",
			"model-b": "tiered_expr",
		},
		Exprs: map[string]string{
			"model-a": "p * 5 + c * 25",
			"model-b": "p * 10 + c * 50",
		},
		Name: "billing",
	}

	// Simulate removing model-a: new value only has model-b
	err := UpdateConfigFromMap(cfg, map[string]string{
		"modes": `{"model-b": "tiered_expr"}`,
		"exprs": `{"model-b": "p * 10 + c * 50"}`,
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"model-b": "tiered_expr"}, cfg.Modes)
	assert.Equal(t, map[string]string{"model-b": "p * 10 + c * 50"}, cfg.Exprs)
	assert.Equal(t, "billing", cfg.Name)
}

func TestUpdateConfigFromMap_EmptyMapClearsAll(t *testing.T) {
	cfg := &testConfigWithMap{
		Modes: map[string]string{
			"model-a": "tiered_expr",
		},
		Exprs: map[string]string{
			"model-a": "p * 5 + c * 25",
		},
	}

	err := UpdateConfigFromMap(cfg, map[string]string{
		"modes": `{}`,
		"exprs": `{}`,
	})
	require.NoError(t, err)

	assert.Empty(t, cfg.Modes)
	assert.Empty(t, cfg.Exprs)
}

func TestUpdateConfigFromMap_ScalarFieldsUnchanged(t *testing.T) {
	cfg := &testConfigWithMap{
		Modes: map[string]string{"m": "v"},
		Name:  "old",
	}

	err := UpdateConfigFromMap(cfg, map[string]string{
		"name": "new",
	})
	require.NoError(t, err)

	assert.Equal(t, "new", cfg.Name)
	assert.Equal(t, map[string]string{"m": "v"}, cfg.Modes)
}

type atomicTestConfig struct {
	Name    string         `json:"name"`
	Enabled bool           `json:"enabled"`
	Count   int            `json:"count"`
	Limit   uint           `json:"limit"`
	Numbers []int          `json:"numbers"`
	Details *atomicDetails `json:"details"`
}

type atomicDetails struct {
	Label string `json:"label"`
}

func TestUpdateConfigFromMap_SliceTypeErrorIsAtomic(t *testing.T) {
	cfg := &atomicTestConfig{Name: "original", Numbers: []int{7, 8, 9}}
	want := cloneAtomicTestConfig(cfg)

	err := UpdateConfigFromMap(cfg, map[string]string{
		"numbers": `[1, "not-an-int", 3]`,
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, `config field "numbers"`)
	assert.Equal(t, want, cfg)
}

func TestUpdateConfigFromMap_MalformedNilPointerIsAtomic(t *testing.T) {
	cfg := &atomicTestConfig{Name: "original"}
	want := cloneAtomicTestConfig(cfg)

	err := UpdateConfigFromMap(cfg, map[string]string{
		"details": `{"label":`,
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, `config field "details"`)
	assert.Equal(t, want, cfg)
	assert.Nil(t, cfg.Details)
}

func TestUpdateConfigFromMap_ValidAndInvalidFieldsLeaveWholeConfigUnchanged(t *testing.T) {
	cfg := &atomicTestConfig{
		Name:    "original",
		Enabled: true,
		Count:   4,
		Numbers: []int{7, 8, 9},
		Details: &atomicDetails{Label: "kept"},
	}
	want := cloneAtomicTestConfig(cfg)

	err := UpdateConfigFromMap(cfg, map[string]string{
		"name":    "updated",
		"enabled": "not-a-bool",
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, `config field "enabled"`)
	assert.Equal(t, want, cfg)
}

func TestUpdateConfigFromMap_RejectsFractionalIntegersAndNonFiniteFloats(t *testing.T) {
	type numericConfig struct {
		Count int     `json:"count"`
		Ratio float64 `json:"ratio"`
	}

	for _, update := range []map[string]string{
		{"count": "12.5"},
		{"ratio": "NaN"},
		{"ratio": "+Inf"},
	} {
		cfg := &numericConfig{Count: 7, Ratio: 0.5}
		require.Error(t, UpdateConfigFromMap(cfg, update))
		assert.Equal(t, &numericConfig{Count: 7, Ratio: 0.5}, cfg)
	}
}

func TestUpdateConfigFromMap_SuccessContracts(t *testing.T) {
	cfg := &atomicTestConfig{
		Name:    "original",
		Enabled: true,
		Count:   1,
		Limit:   2,
		Numbers: []int{7, 8, 9},
		Details: &atomicDetails{Label: "old"},
	}

	err := UpdateConfigFromMap(cfg, map[string]string{
		"name":    "updated",
		"count":   "12.000000",
		"limit":   "24.000000",
		"numbers": `[]`,
		"details": `{"label":"new"}`,
	})

	require.NoError(t, err)
	assert.Equal(t, "updated", cfg.Name)
	assert.True(t, cfg.Enabled, "an omitted field must remain unchanged")
	assert.Equal(t, 12, cfg.Count)
	assert.Equal(t, uint(24), cfg.Limit)
	assert.Empty(t, cfg.Numbers)
	assert.Equal(t, &atomicDetails{Label: "new"}, cfg.Details)
}

func TestValidateConfigFromMap_DoesNotChangeConfig(t *testing.T) {
	cfg := &atomicTestConfig{Name: "original", Numbers: []int{7, 8, 9}}
	want := cloneAtomicTestConfig(cfg)

	err := ValidateConfigFromMap(cfg, map[string]string{
		"name":    "updated",
		"numbers": `[1, "not-an-int"]`,
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, `config field "numbers"`)
	assert.Equal(t, want, cfg)

	require.NoError(t, ValidateConfigFromMap(cfg, map[string]string{
		"name":    "updated",
		"numbers": `[1, 2]`,
	}))
	assert.Equal(t, want, cfg)
}

type testMapConfig struct {
	updateCalled bool
	update       map[string]string
	exportErr    error
	onExport     func()
	onUpdate     func()
}

type testValidatingMapConfig struct {
	testMapConfig
	validateCalled bool
	validate       map[string]string
	validateErr    error
}

func (cfg *testValidatingMapConfig) ValidateConfigMap(update map[string]string) error {
	cfg.validateCalled = true
	cfg.validate = update
	return cfg.validateErr
}

func (cfg *testMapConfig) ExportConfigMap() (map[string]string, error) {
	if cfg.onExport != nil {
		cfg.onExport()
	}
	return nil, cfg.exportErr
}

func (cfg *testMapConfig) UpdateConfigMap(update map[string]string) error {
	if cfg.onUpdate != nil {
		cfg.onUpdate()
	}
	cfg.updateCalled = true
	cfg.update = update
	return nil
}

func TestUpdateConfigFromMap_MapConfigDelegatesUpdate(t *testing.T) {
	cfg := &testMapConfig{}
	update := map[string]string{"name": "updated"}

	require.NoError(t, UpdateConfigFromMap(cfg, update))
	assert.True(t, cfg.updateCalled)
	assert.Equal(t, update, cfg.update)
}

func TestValidateConfigFromMap_MapConfigDoesNotInvokeUpdate(t *testing.T) {
	cfg := &testMapConfig{}

	err := ValidateConfigFromMap(cfg, map[string]string{"name": "updated"})

	require.ErrorIs(t, err, ErrMapConfigValidationUnsupported)
	assert.False(t, cfg.updateCalled)
}

func TestValidateConfigFromMap_ValidatingMapConfigDelegatesPureValidation(t *testing.T) {
	sentinel := errors.New("invalid managed config")
	cfg := &testValidatingMapConfig{validateErr: sentinel}
	update := map[string]string{"name": "updated"}

	err := ValidateConfigFromMap(cfg, update)

	require.ErrorIs(t, err, sentinel)
	assert.True(t, cfg.validateCalled)
	assert.Equal(t, update, cfg.validate)
	assert.False(t, cfg.updateCalled)
}

func TestConfigManagerDoesNotHoldRegistryLockAcrossCallbacks(t *testing.T) {
	t.Run("save callback", func(t *testing.T) {
		manager := NewConfigManager()
		manager.Register("base", &testConfigWithMap{Name: "base"})
		requireConfigOperationCompletes(t, func() error {
			return manager.SaveToDB(func(_, _ string) error {
				manager.Register("registered-during-save", &testConfigWithMap{})
				return nil
			})
		})
	})

	t.Run("MapConfig update", func(t *testing.T) {
		manager := NewConfigManager()
		managed := &testMapConfig{}
		managed.onUpdate = func() {
			manager.Register("registered-during-load", &testConfigWithMap{})
		}
		manager.Register("managed", managed)
		requireConfigOperationCompletes(t, func() error {
			return manager.LoadFromDB(map[string]string{"managed.name": "updated"})
		})
	})

	t.Run("MapConfig export", func(t *testing.T) {
		manager := NewConfigManager()
		managed := &testMapConfig{}
		managed.onExport = func() {
			manager.Register("registered-during-export", &testConfigWithMap{})
		}
		manager.Register("managed", managed)
		requireConfigOperationCompletes(t, func() error {
			manager.ExportAllConfigs()
			return nil
		})
	})
}

func TestConfigManagerSaveCallbackCanReenterLoadAndUsesExportedSnapshot(t *testing.T) {
	manager := NewConfigManager()
	first := &testConfigWithMap{Name: "first-before"}
	second := &testConfigWithMap{Name: "second-before"}
	manager.Register("first", first)
	manager.Register("second", second)

	saved := make(map[string]string)
	reentered := false
	requireConfigOperationCompletes(t, func() error {
		return manager.SaveToDB(func(key, value string) error {
			saved[key] = value
			if reentered {
				return nil
			}
			reentered = true
			return manager.LoadFromDB(map[string]string{
				"first.name":  "first-after",
				"second.name": "second-after",
			})
		})
	})

	assert.Equal(t, "first-before", saved["first.name"])
	assert.Equal(t, "second-before", saved["second.name"])
	assert.Equal(t, "first-after", first.Name)
	assert.Equal(t, "second-after", second.Name)
}

func TestConfigManagerSaveExportErrorSkipsCallbackAndPreservesError(t *testing.T) {
	manager := NewConfigManager()
	sentinel := errors.New("export failed")
	manager.Register("managed", &testMapConfig{exportErr: sentinel})

	callbackCalls := 0
	err := configOperationResult(t, func() error {
		return manager.SaveToDB(func(_, _ string) error {
			callbackCalls++
			return nil
		})
	})

	require.ErrorIs(t, err, sentinel)
	assert.Zero(t, callbackCalls)
}

func TestConfigManagerSaveCallbackErrorReleasesOperationLock(t *testing.T) {
	manager := NewConfigManager()
	manager.Register("base", &testConfigWithMap{Name: "before"})
	sentinel := errors.New("update failed")

	err := configOperationResult(t, func() error {
		return manager.SaveToDB(func(_, _ string) error { return sentinel })
	})
	require.ErrorIs(t, err, sentinel)

	requireConfigOperationCompletes(t, func() error {
		return manager.LoadFromDB(map[string]string{"base.name": "after"})
	})
	requireConfigOperationCompletes(t, func() error {
		return manager.SaveToDB(func(_, _ string) error { return nil })
	})
}

func requireConfigOperationCompletes(t *testing.T, operation func() error) {
	t.Helper()
	require.NoError(t, configOperationResult(t, operation))
}

func configOperationResult(t *testing.T, operation func() error) error {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- operation() }()
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("configuration manager callback deadlocked on the registry lock")
		return nil
	}
}

func cloneAtomicTestConfig(cfg *atomicTestConfig) *atomicTestConfig {
	clone := *cfg
	clone.Numbers = append([]int(nil), cfg.Numbers...)
	if cfg.Details != nil {
		details := *cfg.Details
		clone.Details = &details
	}
	return &clone
}
