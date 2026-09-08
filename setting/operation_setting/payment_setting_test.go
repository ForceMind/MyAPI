package operation_setting

import (
	"reflect"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedPaymentSettingExportsDefaultsAndPreservesPartialUpdates(t *testing.T) {
	state := newManagedPaymentSetting(defaultPaymentSetting)
	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"amount_options":           `[10,20,50,100,200,500]`,
		"amount_discount":          `{}`,
		"compliance_confirmed":     "false",
		"compliance_terms_version": "",
		"compliance_confirmed_at":  "0",
		"compliance_confirmed_by":  "0",
		"compliance_confirmed_ip":  "",
	}, exported)

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"amount_options":       `[1,2]`,
		"compliance_confirmed": "true",
		"unknown_legacy_key":   "ignored",
	}))
	snapshot := state.snapshot()
	assert.Equal(t, []int{1, 2}, snapshot.AmountOptions)
	assert.True(t, snapshot.ComplianceConfirmed)
	assert.Equal(t, defaultPaymentSetting.AmountDiscount, snapshot.AmountDiscount)
	assert.Empty(t, snapshot.ComplianceTermsVersion)
}

func TestManagedPaymentSettingPreservesNullAndRejectsInvalidCandidates(t *testing.T) {
	state := newManagedPaymentSetting(defaultPaymentSetting)
	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"amount_options":  "null",
		"amount_discount": "null",
	}))
	snapshot := state.snapshot()
	assert.Nil(t, snapshot.AmountOptions)
	assert.Nil(t, snapshot.AmountDiscount)

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"amount_options":  `[]`,
		"amount_discount": `{}`,
	}))
	snapshot = state.snapshot()
	assert.NotNil(t, snapshot.AmountOptions)
	assert.Empty(t, snapshot.AmountOptions)
	assert.NotNil(t, snapshot.AmountDiscount)
	assert.Empty(t, snapshot.AmountDiscount)

	before := state.current.Load()
	for _, update := range []map[string]string{
		{"amount_options": "true"},
		{"amount_discount": "true"},
		{"compliance_confirmed": "not-a-bool"},
		{"compliance_confirmed_at": "not-an-int"},
		{"compliance_confirmed_by": "NaN"},
	} {
		require.Error(t, state.ValidateConfigMap(update))
		assert.Same(t, before, state.current.Load())
		require.Error(t, state.UpdateConfigMap(update))
		assert.Same(t, before, state.current.Load())
	}
}

func TestManagedPaymentSettingCopiesInitialAndDetachedSnapshots(t *testing.T) {
	initial := PaymentSetting{
		AmountOptions:  []int{10},
		AmountDiscount: map[int]float64{10: 0.9},
	}
	state := newManagedPaymentSetting(initial)
	initial.AmountOptions[0] = 99
	initial.AmountDiscount[10] = 0.1

	snapshot := state.snapshot()
	assert.Equal(t, []int{10}, snapshot.AmountOptions)
	assert.Equal(t, map[int]float64{10: 0.9}, snapshot.AmountDiscount)
	snapshot.AmountOptions[0] = 88
	snapshot.AmountOptions = append(snapshot.AmountOptions, 77)
	snapshot.AmountDiscount[10] = 0.2
	assert.Equal(t, []int{10}, state.snapshot().AmountOptions)
	assert.Equal(t, map[int]float64{10: 0.9}, state.snapshot().AmountDiscount)
}

func TestGetPaymentSettingReturnsDetachedSnapshot(t *testing.T) {
	registered := config.GlobalConfig.Get("payment_setting")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})
	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"amount_options":  `[1]`,
		"amount_discount": `{"1":0.8}`,
	}))
	detached := GetPaymentSetting()
	detached.AmountOptions[0] = 99
	detached.AmountOptions = append(detached.AmountOptions, 88)
	detached.AmountDiscount[1] = 0.1
	assert.Equal(t, []int{1}, paymentSettingState.snapshot().AmountOptions)
	assert.Equal(t, map[int]float64{1: 0.8}, paymentSettingState.snapshot().AmountDiscount)
}

func TestManagedPaymentSettingPublishesWholeGeneration(t *testing.T) {
	state := newManagedPaymentSetting(PaymentSetting{
		AmountOptions:          []int{1},
		AmountDiscount:         map[int]float64{1: 0.9},
		ComplianceConfirmed:    true,
		ComplianceTermsVersion: CurrentComplianceTermsVersion,
		ComplianceConfirmedAt:  1,
		ComplianceConfirmedBy:  1,
		ComplianceConfirmedIP:  "first",
	})
	first := state.snapshot()
	second := PaymentSetting{
		AmountOptions:          []int{2, 3},
		AmountDiscount:         map[int]float64{2: 0.8},
		ComplianceTermsVersion: "expired",
		ComplianceConfirmedAt:  2,
		ComplianceConfirmedBy:  2,
		ComplianceConfirmedIP:  "second",
	}
	updates := []map[string]string{
		{"amount_options": `[1]`, "amount_discount": `{"1":0.9}`, "compliance_confirmed": "true", "compliance_terms_version": CurrentComplianceTermsVersion, "compliance_confirmed_at": "1", "compliance_confirmed_by": "1", "compliance_confirmed_ip": "first"},
		{"amount_options": `[2,3]`, "amount_discount": `{"2":0.8}`, "compliance_confirmed": "false", "compliance_terms_version": "expired", "compliance_confirmed_at": "2", "compliance_confirmed_by": "2", "compliance_confirmed_ip": "second"},
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
		assert.True(t, reflect.DeepEqual(snapshot, first) || reflect.DeepEqual(snapshot, second), "observed mixed generation: %#v", snapshot)
	}
	writers.Wait()
	select {
	case err := <-writerErr:
		require.NoError(t, err)
	default:
	}
}

func TestManagedPaymentSettingConfigManagerLoadAndExport(t *testing.T) {
	manager := config.NewConfigManager()
	state := newManagedPaymentSetting(defaultPaymentSetting)
	manager.Register("payment_setting", state)
	require.NoError(t, manager.LoadFromDB(map[string]string{
		"payment_setting.amount_options":           `[7,9]`,
		"payment_setting.amount_discount":          `{"7":0.8}`,
		"payment_setting.compliance_confirmed":     "true",
		"payment_setting.compliance_terms_version": CurrentComplianceTermsVersion,
		"payment_setting.compliance_confirmed_at":  "7",
		"payment_setting.compliance_confirmed_by":  "9",
		"payment_setting.compliance_confirmed_ip":  "192.0.2.7",
		"payment_setting.unknown":                  "ignored",
	}))
	assert.Equal(t, PaymentSetting{
		AmountOptions:          []int{7, 9},
		AmountDiscount:         map[int]float64{7: 0.8},
		ComplianceConfirmed:    true,
		ComplianceTermsVersion: CurrentComplianceTermsVersion,
		ComplianceConfirmedAt:  7,
		ComplianceConfirmedBy:  9,
		ComplianceConfirmedIP:  "192.0.2.7",
	}, state.snapshot())
	saved := make(map[string]string)
	require.NoError(t, manager.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	assert.Equal(t, map[string]string{
		"payment_setting.amount_options":           `[7,9]`,
		"payment_setting.amount_discount":          `{"7":0.8}`,
		"payment_setting.compliance_confirmed":     "true",
		"payment_setting.compliance_terms_version": CurrentComplianceTermsVersion,
		"payment_setting.compliance_confirmed_at":  "7",
		"payment_setting.compliance_confirmed_by":  "9",
		"payment_setting.compliance_confirmed_ip":  "192.0.2.7",
	}, saved)
}

func TestPaymentComplianceReadsOneGeneration(t *testing.T) {
	registered := config.GlobalConfig.Get("payment_setting")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})
	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"compliance_confirmed":     "true",
		"compliance_terms_version": CurrentComplianceTermsVersion,
	}))
	assert.True(t, IsPaymentComplianceConfirmed())
	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"compliance_confirmed":     "true",
		"compliance_terms_version": "expired",
	}))
	assert.False(t, IsPaymentComplianceConfirmed())
}
