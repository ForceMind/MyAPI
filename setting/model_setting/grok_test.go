package model_setting

import (
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedGrokSettingsExportsDefaultsAndPreservesPartialUpdates(t *testing.T) {
	state := newManagedGrokSettings(defaultGrokSettings)

	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"violation_deduction_enabled": "true",
		"violation_deduction_amount":  "0.05",
	}, exported)

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"violation_deduction_enabled": "false",
		"unknown_legacy_key":          "ignored",
	}))
	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"violation_deduction_amount": "0.125",
	}))
	assert.Equal(t, GrokSettings{
		ViolationDeductionEnabled: false,
		ViolationDeductionAmount:  0.125,
	}, state.snapshot())
}

func TestManagedGrokSettingsInvalidCandidatesDoNotPublish(t *testing.T) {
	state := newManagedGrokSettings(defaultGrokSettings)
	before := state.current.Load()

	for _, update := range []map[string]string{
		{"violation_deduction_enabled": "not-a-bool"},
		{"violation_deduction_amount": "not-a-number"},
		{"violation_deduction_amount": "NaN"},
		{"violation_deduction_amount": "null"},
	} {
		require.Error(t, state.ValidateConfigMap(update))
		assert.Same(t, before, state.current.Load())
		require.Error(t, state.UpdateConfigMap(update))
		assert.Same(t, before, state.current.Load())
	}
}

func TestGetGrokSettingsReturnsDetachedSnapshot(t *testing.T) {
	registered := config.GlobalConfig.Get("grok")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"violation_deduction_enabled": "false",
		"violation_deduction_amount":  "0.125",
	}))
	detached := GetGrokSettings()
	detached.ViolationDeductionEnabled = true
	detached.ViolationDeductionAmount = 9.5

	assert.Equal(t, GrokSettings{
		ViolationDeductionEnabled: false,
		ViolationDeductionAmount:  0.125,
	}, grokSettingsState.snapshot())
}

func TestManagedGrokSettingsPublishesWholeGeneration(t *testing.T) {
	state := newManagedGrokSettings(GrokSettings{ViolationDeductionEnabled: false, ViolationDeductionAmount: 0.01})
	first := GrokSettings{ViolationDeductionEnabled: false, ViolationDeductionAmount: 0.01}
	second := GrokSettings{ViolationDeductionEnabled: true, ViolationDeductionAmount: 0.5}
	updates := []map[string]string{
		{"violation_deduction_enabled": "false", "violation_deduction_amount": "0.01"},
		{"violation_deduction_enabled": "true", "violation_deduction_amount": "0.5"},
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
		assert.True(t, snapshot == first || snapshot == second, "observed mixed generation: %#v", snapshot)
	}
	writers.Wait()
	select {
	case err := <-writerErr:
		require.NoError(t, err)
	default:
	}
}

func TestManagedGrokSettingsConfigManagerLoadAndExport(t *testing.T) {
	manager := config.NewConfigManager()
	state := newManagedGrokSettings(defaultGrokSettings)
	manager.Register("grok", state)

	require.NoError(t, manager.LoadFromDB(map[string]string{
		"grok.violation_deduction_enabled": "false",
		"grok.violation_deduction_amount":  "0.25",
		"grok.unknown":                     "ignored",
	}))
	assert.Equal(t, GrokSettings{
		ViolationDeductionEnabled: false,
		ViolationDeductionAmount:  0.25,
	}, state.snapshot())

	saved := make(map[string]string)
	require.NoError(t, manager.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	assert.Equal(t, map[string]string{
		"grok.violation_deduction_enabled": "false",
		"grok.violation_deduction_amount":  "0.25",
	}, saved)
}
