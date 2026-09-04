package operation_setting

import (
	"os"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func replaceMonitorSettingForTest(t *testing.T, setting MonitorSetting) {
	t.Helper()
	previous := monitorSetting.snapshot()
	monitorSetting.publish(setting)
	t.Cleanup(func() { monitorSetting.publish(previous) })
}

func clearMonitorEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"CHANNEL_TEST_FREQUENCY", "CHANNEL_TEST_ENABLED"} {
		value, wasSet := os.LookupEnv(key)
		require.NoError(t, os.Unsetenv(key))
		t.Cleanup(func() {
			if wasSet {
				require.NoError(t, os.Setenv(key, value))
				return
			}
			require.NoError(t, os.Unsetenv(key))
		})
	}
}

func TestGetMonitorSetting_EnvironmentOverrideIsNotPersisted(t *testing.T) {
	clearMonitorEnvironment(t)
	replaceMonitorSettingForTest(t, MonitorSetting{
		AutoTestChannelEnabled: false,
		AutoTestChannelMinutes: 20,
		ChannelTestMode:        ChannelTestModeAutoBanOnly,
		ChannelTestConcurrency: 8,
	})

	require.NoError(t, os.Setenv("CHANNEL_TEST_FREQUENCY", "5"))
	require.NoError(t, os.Setenv("CHANNEL_TEST_ENABLED", "false"))
	effective := GetMonitorSetting()
	require.NotNil(t, effective)
	assert.False(t, effective.AutoTestChannelEnabled)
	assert.Equal(t, float64(5), effective.AutoTestChannelMinutes)
	assert.Equal(t, ChannelTestModeScheduledAll, effective.ChannelTestMode)

	require.NoError(t, os.Unsetenv("CHANNEL_TEST_FREQUENCY"))
	require.NoError(t, os.Unsetenv("CHANNEL_TEST_ENABLED"))
	restored := GetMonitorSetting()
	assert.Equal(t, MonitorSetting{
		AutoTestChannelEnabled: false,
		AutoTestChannelMinutes: 20,
		ChannelTestMode:        ChannelTestModeAutoBanOnly,
		ChannelTestConcurrency: 8,
	}, *restored)
}

func TestGetMonitorSetting_NormalizationDoesNotChangeExport(t *testing.T) {
	clearMonitorEnvironment(t)
	replaceMonitorSettingForTest(t, MonitorSetting{
		AutoTestChannelEnabled: true,
		AutoTestChannelMinutes: 12,
		ChannelTestMode:        "invalid",
		ChannelTestConcurrency: MaxChannelTestConcurrency + 1,
	})

	effective := GetMonitorSetting()
	assert.Equal(t, ChannelTestModeScheduledAll, effective.ChannelTestMode)
	assert.Equal(t, MaxChannelTestConcurrency, effective.ChannelTestConcurrency)

	exported, err := monitorSetting.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, "invalid", exported["channel_test_mode"])
	assert.Equal(t, "33", exported["channel_test_concurrency"])
}

func TestGetMonitorSetting_ReturnValueIsIsolated(t *testing.T) {
	clearMonitorEnvironment(t)
	replaceMonitorSettingForTest(t, defaultMonitorSetting)

	first := GetMonitorSetting()
	first.AutoTestChannelEnabled = true
	first.AutoTestChannelMinutes = 999
	first.ChannelTestMode = ChannelTestModePassiveRecovery
	first.ChannelTestConcurrency = MaxChannelTestConcurrency

	assert.Equal(t, defaultMonitorSetting, *GetMonitorSetting())
}

func TestManagedMonitorSetting_ValidationAndFailedUpdateDoNotPublish(t *testing.T) {
	clearMonitorEnvironment(t)
	replaceMonitorSettingForTest(t, MonitorSetting{
		AutoTestChannelEnabled: true,
		AutoTestChannelMinutes: 11,
		ChannelTestMode:        ChannelTestModeAutoBanOnly,
		ChannelTestConcurrency: 4,
	})
	before, err := monitorSetting.ExportConfigMap()
	require.NoError(t, err)

	invalid := map[string]string{
		"auto_test_channel_enabled": "false",
		"auto_test_channel_minutes": "not-a-number",
	}
	require.Error(t, monitorSetting.ValidateConfigMap(invalid))
	require.Error(t, config.ValidateConfigFromMap(config.GlobalConfig.Get("monitor_setting"), invalid))
	require.Error(t, monitorSetting.UpdateConfigMap(invalid))
	after, err := monitorSetting.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestManagedMonitorSetting_PublishesCompleteGenerations(t *testing.T) {
	clearMonitorEnvironment(t)
	old := MonitorSetting{
		AutoTestChannelEnabled: false,
		AutoTestChannelMinutes: 11,
		ChannelTestMode:        ChannelTestModeAutoBanOnly,
		ChannelTestConcurrency: 2,
	}
	new := MonitorSetting{
		AutoTestChannelEnabled: true,
		AutoTestChannelMinutes: 22,
		ChannelTestMode:        ChannelTestModePassiveRecovery,
		ChannelTestConcurrency: 3,
	}
	setting := newManagedMonitorSetting(old)
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	readResult := make(chan MonitorSetting, 1)
	writeErr := make(chan error, 1)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		ready <- struct{}{}
		<-start
		writeErr <- setting.UpdateConfigMap(map[string]string{
			"auto_test_channel_enabled": "true",
			"auto_test_channel_minutes": "22",
			"channel_test_mode":         ChannelTestModePassiveRecovery,
			"channel_test_concurrency":  "3",
		})
	}()
	go func() {
		defer workers.Done()
		ready <- struct{}{}
		<-start
		readResult <- setting.snapshot()
	}()
	<-ready
	<-ready
	close(start)
	workers.Wait()
	require.NoError(t, <-writeErr)
	observed := <-readResult
	assert.True(t, observed == old || observed == new, "read mixed monitor generation: %#v", observed)
}

func TestManagedMonitorSetting_ConcurrentPartialUpdatesPreserveBothFields(t *testing.T) {
	setting := newManagedMonitorSetting(MonitorSetting{
		AutoTestChannelEnabled: false,
		AutoTestChannelMinutes: 1,
		ChannelTestMode:        ChannelTestModeScheduledAll,
		ChannelTestConcurrency: 1,
	})
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	writeErr := make(chan error, 2)
	var writers sync.WaitGroup
	writers.Add(2)
	go func() {
		defer writers.Done()
		ready <- struct{}{}
		<-start
		writeErr <- setting.UpdateConfigMap(map[string]string{"auto_test_channel_enabled": "true"})
	}()
	go func() {
		defer writers.Done()
		ready <- struct{}{}
		<-start
		writeErr <- setting.UpdateConfigMap(map[string]string{"auto_test_channel_minutes": "2"})
	}()
	<-ready
	<-ready
	close(start)
	writers.Wait()
	require.NoError(t, <-writeErr)
	require.NoError(t, <-writeErr)
	assert.Equal(t, MonitorSetting{
		AutoTestChannelEnabled: true,
		AutoTestChannelMinutes: 2,
		ChannelTestMode:        ChannelTestModeScheduledAll,
		ChannelTestConcurrency: 1,
	}, setting.snapshot())
}

func TestMonitorSetting_GlobalConfigPublishesSnapshot(t *testing.T) {
	clearMonitorEnvironment(t)
	replaceMonitorSettingForTest(t, defaultMonitorSetting)

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"monitor_setting.auto_test_channel_enabled": "true",
		"monitor_setting.auto_test_channel_minutes": "15",
		"monitor_setting.channel_test_mode":         ChannelTestModePassiveRecovery,
		"monitor_setting.channel_test_concurrency":  "6",
	}))
	assert.Equal(t, MonitorSetting{
		AutoTestChannelEnabled: true,
		AutoTestChannelMinutes: 15,
		ChannelTestMode:        ChannelTestModePassiveRecovery,
		ChannelTestConcurrency: 6,
	}, *GetMonitorSetting())

	exported := config.GlobalConfig.ExportAllConfigs()
	assert.Equal(t, "true", exported["monitor_setting.auto_test_channel_enabled"])
	assert.Equal(t, "15", exported["monitor_setting.auto_test_channel_minutes"])
	assert.Equal(t, ChannelTestModePassiveRecovery, exported["monitor_setting.channel_test_mode"])
	assert.Equal(t, "6", exported["monitor_setting.channel_test_concurrency"])
}

func TestGetMonitorSettingNormalizesChannelTestConcurrency(t *testing.T) {
	clearMonitorEnvironment(t)
	for _, test := range []struct {
		name        string
		concurrency int
		want        int
	}{
		{name: "missing uses safe default", concurrency: 0, want: DefaultChannelTestConcurrency},
		{name: "configured value is preserved", concurrency: 8, want: 8},
		{name: "oversized value is capped", concurrency: MaxChannelTestConcurrency + 1, want: MaxChannelTestConcurrency},
	} {
		t.Run(test.name, func(t *testing.T) {
			replaceMonitorSettingForTest(t, MonitorSetting{ChannelTestConcurrency: test.concurrency})
			assert.Equal(t, test.want, GetMonitorSetting().ChannelTestConcurrency)
		})
	}
}

func TestValidateChannelTestConcurrency(t *testing.T) {
	require.NoError(t, ValidateChannelTestConcurrency("1"))
	require.NoError(t, ValidateChannelTestConcurrency("32"))
	assert.Error(t, ValidateChannelTestConcurrency("0"))
	assert.Error(t, ValidateChannelTestConcurrency("33"))
	assert.Error(t, ValidateChannelTestConcurrency("1.5"))
}
