package console_setting

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedConsoleSettingExportsDefaultsAndPartialUpdatesPreserveTheCandidate(t *testing.T) {
	state := newManagedConsoleSetting(defaultConsoleSetting)

	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"api_info":              "",
		"uptime_kuma_groups":    "",
		"announcements":         "",
		"faq":                   "",
		"api_info_enabled":      "true",
		"uptime_kuma_enabled":   "true",
		"announcements_enabled": "true",
		"faq_enabled":           "true",
	}, exported)

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"api_info":           `[{"url":"https://api.example.test"}]`,
		"faq_enabled":        "false",
		"unknown_legacy_key": "ignored",
	}))
	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"announcements": `[{"content":"Release"}]`,
	}))
	assert.Equal(t, ConsoleSetting{
		ApiInfo:              `[{"url":"https://api.example.test"}]`,
		UptimeKumaGroups:     "",
		Announcements:        `[{"content":"Release"}]`,
		FAQ:                  "",
		ApiInfoEnabled:       true,
		UptimeKumaEnabled:    true,
		AnnouncementsEnabled: true,
		FAQEnabled:           false,
	}, state.snapshot())
}

func TestManagedConsoleSettingValidationAndDetachedSnapshotsDoNotPublish(t *testing.T) {
	initial := ConsoleSetting{
		ApiInfo:              `[{"url":"https://before.example.test"}]`,
		UptimeKumaGroups:     "groups",
		Announcements:        "announcements",
		FAQ:                  "faq",
		ApiInfoEnabled:       true,
		UptimeKumaEnabled:    false,
		AnnouncementsEnabled: true,
		FAQEnabled:           false,
	}
	state := newManagedConsoleSetting(initial)
	before := state.current.Load()

	require.NoError(t, state.ValidateConfigMap(map[string]string{
		"api_info_enabled": "false",
		"announcements":    `[{"content":"Candidate"}]`,
	}))
	assert.Same(t, before, state.current.Load())
	assert.Equal(t, initial, state.snapshot())

	require.Error(t, state.UpdateConfigMap(map[string]string{"faq_enabled": "not-a-bool"}))
	assert.Same(t, before, state.current.Load())

	snapshot := state.detachedSnapshot()
	snapshot.ApiInfo = "caller-local"
	snapshot.ApiInfoEnabled = false
	assert.Equal(t, initial, state.snapshot())
}

func TestConsoleContentHelpersUseTheSuppliedSnapshot(t *testing.T) {
	setting := ConsoleSetting{
		ApiInfo: `[{"route":"API"}]`,
		Announcements: `[
			{"content":"Older","publishDate":"2026-01-01T00:00:00Z"},
			{"content":"Newer","publishDate":"2026-02-01T00:00:00Z"}
		]`,
		FAQ:              `[{"question":"Q","answer":"A"}]`,
		UptimeKumaGroups: `[{"categoryName":"Status"}]`,
	}

	apiInfo := GetApiInfoForConsoleSetting(setting)
	assert.Equal(t, "API", apiInfo[0]["route"])
	apiInfo[0]["route"] = "caller-local"
	assert.Equal(t, "API", GetApiInfoForConsoleSetting(setting)[0]["route"])

	announcements := GetAnnouncementsForConsoleSetting(setting)
	require.Len(t, announcements, 2)
	assert.Equal(t, "Newer", announcements[0]["content"])
	assert.Equal(t, "Older", announcements[1]["content"])
	assert.Equal(t, "Q", GetFAQForConsoleSetting(setting)[0]["question"])
	assert.Equal(t, "Status", GetUptimeKumaGroupsForConsoleSetting(setting)[0]["categoryName"])
}

func TestManagedConsoleSettingConcurrentPartialUpdatesRetainAllFields(t *testing.T) {
	state := newManagedConsoleSetting(defaultConsoleSetting)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var workers sync.WaitGroup

	for _, update := range []map[string]string{
		{
			"api_info":         `[{"url":"https://api.example.test"}]`,
			"api_info_enabled": "false",
		},
		{
			"faq":         `[{"question":"Q","answer":"A"}]`,
			"faq_enabled": "false",
		},
	} {
		update := update
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			errs <- state.UpdateConfigMap(update)
		}()
	}

	close(start)
	workers.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	assert.Equal(t, ConsoleSetting{
		ApiInfo:              `[{"url":"https://api.example.test"}]`,
		UptimeKumaGroups:     "",
		Announcements:        "",
		FAQ:                  `[{"question":"Q","answer":"A"}]`,
		ApiInfoEnabled:       false,
		UptimeKumaEnabled:    true,
		AnnouncementsEnabled: true,
		FAQEnabled:           false,
	}, state.snapshot())
}
