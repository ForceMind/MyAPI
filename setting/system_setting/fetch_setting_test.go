package system_setting

import (
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedFetchSettingExportsDefaultsAndPreservesPartialUpdates(t *testing.T) {
	state := newManagedFetchSetting(defaultFetchSetting)

	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, "true", exported["enable_ssrf_protection"])
	assert.Equal(t, "false", exported["allow_private_ip"])
	assert.Equal(t, "[]", exported["domain_list"])
	assert.Equal(t, "[]", exported["ip_list"])
	assert.Equal(t, `["80","443","8080","8443"]`, exported["allowed_ports"])

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"allow_private_ip":   "true",
		"unknown_legacy_key": "ignored",
	}))
	snapshot := state.snapshot()
	assert.True(t, snapshot.AllowPrivateIp)
	assert.True(t, snapshot.EnableSSRFProtection)
	assert.Equal(t, []string{"80", "443", "8080", "8443"}, snapshot.AllowedPorts)
}

func TestManagedFetchSettingUsesGenericNullAndScalarParsers(t *testing.T) {
	state := newManagedFetchSetting(FetchSetting{
		EnableSSRFProtection:   true,
		DomainList:             []string{"before.example.test"},
		IpList:                 []string{},
		AllowedPorts:           []string{"443"},
		ApplyIPFilterForDomain: true,
	})

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"enable_ssrf_protection": "false",
		"domain_list":            "null",
		"ip_list":                `["203.0.113.0/24"]`,
		"allowed_ports":          `[]`,
	}))
	snapshot := state.snapshot()
	assert.False(t, snapshot.EnableSSRFProtection)
	assert.Nil(t, snapshot.DomainList)
	assert.Equal(t, []string{"203.0.113.0/24"}, snapshot.IpList)
	assert.NotNil(t, snapshot.AllowedPorts)
	assert.Empty(t, snapshot.AllowedPorts)

	before := state.current.Load()
	require.Error(t, state.ValidateConfigMap(map[string]string{"enable_ssrf_protection": "invalid"}))
	assert.Same(t, before, state.current.Load())
	require.Error(t, state.UpdateConfigMap(map[string]string{"allowed_ports": `{"not":"a list"}`}))
	assert.Same(t, before, state.current.Load())
}

func TestManagedFetchSettingDetachedSnapshotCopiesSlices(t *testing.T) {
	state := newManagedFetchSetting(FetchSetting{
		DomainList:   []string{"domain.example.test"},
		IpList:       []string{"203.0.113.0/24"},
		AllowedPorts: []string{"443"},
	})

	detached := state.detachedSnapshot()
	detached.DomainList[0] = "caller.example.test"
	detached.IpList = append(detached.IpList, "198.51.100.0/24")
	detached.AllowedPorts[0] = "8443"

	snapshot := state.snapshot()
	assert.Equal(t, []string{"domain.example.test"}, snapshot.DomainList)
	assert.Equal(t, []string{"203.0.113.0/24"}, snapshot.IpList)
	assert.Equal(t, []string{"443"}, snapshot.AllowedPorts)
}

func TestGetFetchSettingReturnsDetachedSnapshot(t *testing.T) {
	registered := config.GlobalConfig.Get("fetch_setting")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"domain_list": `["source.example.test"]`,
	}))
	detached := GetFetchSetting()
	detached.DomainList[0] = "caller.example.test"
	detached.AllowedPorts = append(detached.AllowedPorts, "65535")

	snapshot := fetchSettingState.snapshot()
	assert.Equal(t, []string{"source.example.test"}, snapshot.DomainList)
	assert.NotContains(t, snapshot.AllowedPorts, "65535")
}

func TestManagedFetchSettingPublishesWholeGenerations(t *testing.T) {
	state := newManagedFetchSetting(FetchSetting{AllowedPorts: []string{"443"}})
	first := FetchSetting{AllowedPorts: []string{"443"}}
	second := FetchSetting{AllowedPorts: []string{"8443"}}
	updates := []map[string]string{
		{"allowed_ports": `["443"]`},
		{"allowed_ports": `["8443"]`},
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
		assert.True(t, (snapshot.AllowedPorts[0] == first.AllowedPorts[0]) || (snapshot.AllowedPorts[0] == second.AllowedPorts[0]))
	}
	writers.Wait()
	select {
	case err := <-writerErr:
		require.NoError(t, err)
	default:
	}
}

func TestManagedFetchSettingConfigManagerLoadAndExport(t *testing.T) {
	manager := config.NewConfigManager()
	state := newManagedFetchSetting(defaultFetchSetting)
	manager.Register("fetch_setting", state)

	require.NoError(t, manager.LoadFromDB(map[string]string{
		"fetch_setting.domain_list":   `["safe.example.test"]`,
		"fetch_setting.allowed_ports": `["443"]`,
		"fetch_setting.unknown":       "ignored",
	}))
	assert.Equal(t, []string{"safe.example.test"}, state.snapshot().DomainList)
	assert.Equal(t, []string{"443"}, state.snapshot().AllowedPorts)

	saved := make(map[string]string)
	require.NoError(t, manager.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	assert.Equal(t, `["safe.example.test"]`, saved["fetch_setting.domain_list"])
	assert.Equal(t, `["443"]`, saved["fetch_setting.allowed_ports"])
}
