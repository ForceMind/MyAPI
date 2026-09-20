package model

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func typedBulkItem(key string, valueType string, value string) TypedBulkOption {
	return TypedBulkOption{Key: key, Type: valueType, Value: []byte(value)}
}

func snapshotFamilyConfig(t *testing.T, family string) map[string]string {
	t.Helper()
	registered := config.GlobalConfig.Get(family)
	require.NotNil(t, registered, family)
	exported, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	return exported
}

func restoreFamilyConfig(t *testing.T, family string, snapshot map[string]string) {
	t.Helper()
	registered := config.GlobalConfig.Get(family)
	require.NotNil(t, registered, family)
	require.NoError(t, config.UpdateConfigFromMap(registered, snapshot))
}

func swapOptionMapForTest(t *testing.T, initial map[string]string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = initial
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})
}

func countOptions(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var persisted int64
	require.NoError(t, db.Model(&Option{}).Count(&persisted).Error)
	return persisted
}

func TestTypedBulkRejectsUnknownKeysBeforeAnyWrite(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	beforeGeneral := snapshotFamilyConfig(t, "general_setting")
	beforeMonitor := snapshotFamilyConfig(t, "monitor_setting")
	swapOptionMapForTest(t, map[string]string{"general_setting.docs_link": "https://before"})
	t.Cleanup(func() {
		restoreFamilyConfig(t, "general_setting", beforeGeneral)
		restoreFamilyConfig(t, "monitor_setting", beforeMonitor)
	})

	for _, testCase := range []struct {
		name string
		key  string
	}{
		{name: "no family separator", key: "Notice"},
		{name: "empty field", key: "general_setting."},
		{name: "unregistered family", key: "no_such_family.field"},
		{name: "unknown field", key: "general_setting.no_such_field"},
		{name: "excluded performance family", key: "performance_setting.disk_cache_enabled"},
		{name: "excluded channel affinity family", key: "channel_affinity_setting.rules"},
		{name: "excluded payment family", key: "payment_setting.amount_options"},
		{name: "excluded user funding family", key: "user_funding_setting.mode"},
		{name: "funding mode flat key", key: "user_funding_setting"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, err := UpdateOptionsTypedBulk([]TypedBulkOption{
				typedBulkItem(testCase.key, TypedBulkValueTypeString, `"x"`),
			}, nil)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrTypedBulkUnknownKey)
			var fieldErr *TypedBulkFieldError
			require.ErrorAs(t, err, &fieldErr)
			assert.Equal(t, testCase.key, fieldErr.Key)
		})
	}

	assert.Zero(t, countOptions(t, db), "DB must stay unchanged")
	assert.Equal(t, beforeGeneral, snapshotFamilyConfig(t, "general_setting"), "runtime must stay unchanged")
	assert.Equal(t, beforeMonitor, snapshotFamilyConfig(t, "monitor_setting"), "runtime must stay unchanged")
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "https://before", common.OptionMap["general_setting.docs_link"], "OptionMap must stay unchanged")
	_, leaked := common.OptionMap["payment_setting.amount_options"]
	assert.False(t, leaked)
	common.OptionMapRWMutex.RUnlock()
}

func TestTypedBulkRejectsDuplicateKeysBeforeAnyWrite(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	beforeGeneral := snapshotFamilyConfig(t, "general_setting")
	swapOptionMapForTest(t, map[string]string{})
	t.Cleanup(func() { restoreFamilyConfig(t, "general_setting", beforeGeneral) })

	_, _, err := UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `"https://a"`),
		typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `"https://b"`),
	}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTypedBulkDuplicateKey)

	assert.Zero(t, countOptions(t, db))
	assert.Equal(t, beforeGeneral, snapshotFamilyConfig(t, "general_setting"))
	common.OptionMapRWMutex.RLock()
	_, exists := common.OptionMap["general_setting.docs_link"]
	assert.False(t, exists)
	common.OptionMapRWMutex.RUnlock()
}

func TestTypedBulkRejectsInvalidValueBeforeAnyWrite(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	beforeGeneral := snapshotFamilyConfig(t, "general_setting")
	beforeMonitor := snapshotFamilyConfig(t, "monitor_setting")
	swapOptionMapForTest(t, map[string]string{"general_setting.docs_link": "https://before"})
	t.Cleanup(func() {
		restoreFamilyConfig(t, "general_setting", beforeGeneral)
		restoreFamilyConfig(t, "monitor_setting", beforeMonitor)
	})

	// One invalid item (channel test concurrency below the 1..32 bound) must
	// fail the whole request before anything is written.
	_, _, err := UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `"https://after"`),
		typedBulkItem("monitor_setting.channel_test_concurrency", TypedBulkValueTypeNumber, `0`),
	}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTypedBulkInvalidValue)
	var fieldErr *TypedBulkFieldError
	require.ErrorAs(t, err, &fieldErr)
	assert.Equal(t, "monitor_setting.channel_test_concurrency", fieldErr.Key)

	assert.Zero(t, countOptions(t, db), "DB must stay unchanged")
	assert.Equal(t, beforeGeneral, snapshotFamilyConfig(t, "general_setting"), "runtime must stay unchanged")
	assert.Equal(t, beforeMonitor, snapshotFamilyConfig(t, "monitor_setting"), "runtime must stay unchanged")
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "https://before", common.OptionMap["general_setting.docs_link"], "OptionMap must stay unchanged")
	common.OptionMapRWMutex.RUnlock()
}

func TestTypedBulkRejectsMalformedShapesAndOutOfRangeValues(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	beforeGeneral := snapshotFamilyConfig(t, "general_setting")
	swapOptionMapForTest(t, map[string]string{})
	t.Cleanup(func() { restoreFamilyConfig(t, "general_setting", beforeGeneral) })

	oversizedString := `"` + strings.Repeat("s", MaxTypedBulkValueLength+1) + `"`
	oversizedElement := `["` + strings.Repeat("e", MaxTypedBulkStringListItemLength+1) + `"]`
	oversizedList := `[` + strings.Repeat(`"x",`, MaxTypedBulkStringListItems) + `"x"]`
	for _, testCase := range []struct {
		name      string
		items     []TypedBulkOption
		expectErr error
	}{
		{name: "no items", items: nil, expectErr: ErrTypedBulkItemCount},
		{
			name: "too many items",
			items: func() []TypedBulkOption {
				items := make([]TypedBulkOption, 0, MaxTypedBulkItems+1)
				for i := 0; i <= MaxTypedBulkItems; i++ {
					items = append(items, typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `"x"`))
				}
				return items
			}(),
			expectErr: ErrTypedBulkItemCount,
		},
		{name: "string declared but number sent", items: []TypedBulkOption{typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `5`)}, expectErr: ErrTypedBulkInvalidValue},
		{name: "number declared but string sent", items: []TypedBulkOption{typedBulkItem("general_setting.ping_interval_seconds", TypedBulkValueTypeNumber, `"60"`)}, expectErr: ErrTypedBulkInvalidValue},
		{name: "boolean declared but string sent", items: []TypedBulkOption{typedBulkItem("general_setting.ping_interval_enabled", TypedBulkValueTypeBoolean, `"true"`)}, expectErr: ErrTypedBulkInvalidValue},
		{name: "string_list declared but string sent", items: []TypedBulkOption{typedBulkItem("qwen.sync_image_models", TypedBulkValueTypeStringList, `"z-image"`)}, expectErr: ErrTypedBulkInvalidValue},
		{name: "string_list with non-string element", items: []TypedBulkOption{typedBulkItem("qwen.sync_image_models", TypedBulkValueTypeStringList, `["ok",1]`)}, expectErr: ErrTypedBulkInvalidValue},
		{name: "oversized string", items: []TypedBulkOption{typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, oversizedString)}, expectErr: ErrTypedBulkInvalidValue},
		{name: "oversized string_list element", items: []TypedBulkOption{typedBulkItem("qwen.sync_image_models", TypedBulkValueTypeStringList, oversizedElement)}, expectErr: ErrTypedBulkInvalidValue},
		{name: "too many string_list elements", items: []TypedBulkOption{typedBulkItem("qwen.sync_image_models", TypedBulkValueTypeStringList, oversizedList)}, expectErr: ErrTypedBulkInvalidValue},
		{name: "number beyond magnitude bound", items: []TypedBulkOption{typedBulkItem("general_setting.custom_currency_exchange_rate", TypedBulkValueTypeNumber, `1e16`)}, expectErr: ErrTypedBulkInvalidValue},
		{name: "unknown value type", items: []TypedBulkOption{typedBulkItem("general_setting.docs_link", "object", `{}`)}, expectErr: ErrTypedBulkInvalidValue},
		{name: "negative expected revision", items: []TypedBulkOption{typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `"x"`)}, expectErr: ErrTypedBulkInvalidValue},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var expected *int64
			if testCase.name == "negative expected revision" {
				negative := int64(-1)
				expected = &negative
			}
			_, _, err := UpdateOptionsTypedBulk(testCase.items, expected)
			require.Error(t, err)
			assert.ErrorIs(t, err, testCase.expectErr)
		})
	}

	assert.Zero(t, countOptions(t, db))
	assert.Equal(t, beforeGeneral, snapshotFamilyConfig(t, "general_setting"))
	common.OptionMapRWMutex.RLock()
	assert.Empty(t, common.OptionMap)
	common.OptionMapRWMutex.RUnlock()
}

// TestTypedBulkCommitsPublishesAndBumpsRevision covers one
// reflection-delegating family (general_setting parses fields through the
// config reflection candidate) and one hand-rolled MapConfig family
// (monitor_setting) in a single request.
func TestTypedBulkCommitsPublishesAndBumpsRevision(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	beforeGeneral := snapshotFamilyConfig(t, "general_setting")
	beforeMonitor := snapshotFamilyConfig(t, "monitor_setting")
	swapOptionMapForTest(t, map[string]string{})
	t.Cleanup(func() {
		restoreFamilyConfig(t, "general_setting", beforeGeneral)
		restoreFamilyConfig(t, "monitor_setting", beforeMonitor)
	})

	revision, applied, err := UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `"https://docs.example.com"`),
		typedBulkItem("general_setting.ping_interval_enabled", TypedBulkValueTypeBoolean, `true`),
		typedBulkItem("general_setting.ping_interval_seconds", TypedBulkValueTypeNumber, `120`),
		typedBulkItem("general_setting.custom_currency_exchange_rate", TypedBulkValueTypeNumber, `1.5`),
		typedBulkItem("monitor_setting.channel_test_concurrency", TypedBulkValueTypeNumber, `8`),
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), revision)
	assert.Equal(t, []string{
		"general_setting.custom_currency_exchange_rate",
		"general_setting.docs_link",
		"general_setting.ping_interval_enabled",
		"general_setting.ping_interval_seconds",
		"monitor_setting.channel_test_concurrency",
	}, applied)

	// Database state.
	persisted := make(map[string]string)
	var rows []Option
	require.NoError(t, db.Find(&rows).Error)
	for _, row := range rows {
		persisted[row.Key] = row.Value
	}
	assert.Equal(t, "https://docs.example.com", persisted["general_setting.docs_link"])
	assert.Equal(t, "true", persisted["general_setting.ping_interval_enabled"])
	assert.Equal(t, "120", persisted["general_setting.ping_interval_seconds"])
	assert.Equal(t, "1.5", persisted["general_setting.custom_currency_exchange_rate"])
	assert.Equal(t, "8", persisted["monitor_setting.channel_test_concurrency"])
	assert.Equal(t, "1", persisted[typedBulkRevisionOptionKey])

	// Runtime generations.
	general := operation_setting.GetGeneralSetting()
	assert.Equal(t, "https://docs.example.com", general.DocsLink)
	assert.True(t, general.PingIntervalEnabled)
	assert.Equal(t, 120, general.PingIntervalSeconds)
	assert.Equal(t, 1.5, general.CustomCurrencyExchangeRate)
	assert.Equal(t, 8, operation_setting.GetMonitorSetting().ChannelTestConcurrency)

	// OptionMap projection.
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "https://docs.example.com", common.OptionMap["general_setting.docs_link"])
	assert.Equal(t, "true", common.OptionMap["general_setting.ping_interval_enabled"])
	assert.Equal(t, "120", common.OptionMap["general_setting.ping_interval_seconds"])
	assert.Equal(t, "8", common.OptionMap["monitor_setting.channel_test_concurrency"])
	_, revisionLeaked := common.OptionMap[typedBulkRevisionOptionKey]
	assert.False(t, revisionLeaked)
	common.OptionMapRWMutex.RUnlock()

	current, err := CurrentTypedBulkRevision()
	require.NoError(t, err)
	assert.Equal(t, int64(1), current)
}

func TestTypedBulkStringListIsPersistedCanonically(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	beforeQwen := snapshotFamilyConfig(t, "qwen")
	swapOptionMapForTest(t, map[string]string{})
	t.Cleanup(func() { restoreFamilyConfig(t, "qwen", beforeQwen) })

	_, _, err := UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("qwen.sync_image_models", TypedBulkValueTypeStringList, `["z-image","custom-x"]`),
	}, nil)
	require.NoError(t, err)

	var row Option
	require.NoError(t, db.Where("key = ?", "qwen.sync_image_models").First(&row).Error)
	assert.JSONEq(t, `["z-image","custom-x"]`, row.Value)
	common.OptionMapRWMutex.RLock()
	assert.JSONEq(t, `["z-image","custom-x"]`, common.OptionMap["qwen.sync_image_models"])
	common.OptionMapRWMutex.RUnlock()
	assert.JSONEq(t, `["z-image","custom-x"]`, snapshotFamilyConfig(t, "qwen")["sync_image_models"])
}

func TestTypedBulkGroupRatioAliasDualWrite(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	beforeGroupRatio := snapshotFamilyConfig(t, "group_ratio_setting")
	swapOptionMapForTest(t, map[string]string{})
	t.Cleanup(func() { restoreFamilyConfig(t, "group_ratio_setting", beforeGroupRatio) })

	_, applied, err := UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("group_ratio_setting.group_ratio", TypedBulkValueTypeString, `"{\"default\":1,\"vip\":2}"`),
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"GroupRatio", "group_ratio_setting.group_ratio"}, applied)

	// Both the canonical flat key and the compatibility alias are persisted so
	// a reload cannot prefer a stale canonical row.
	var rows []Option
	require.NoError(t, db.Find(&rows).Error)
	persisted := make(map[string]string, len(rows))
	for _, row := range rows {
		persisted[row.Key] = row.Value
	}
	assert.JSONEq(t, `{"default":1,"vip":2}`, persisted["GroupRatio"])
	assert.Equal(t, persisted["GroupRatio"], persisted["group_ratio_setting.group_ratio"])

	// Runtime generation and OptionMap agree on one meaning.
	assert.JSONEq(t, `{"default":1,"vip":2}`, ratio_setting.GroupRatio2JSONString())
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, common.OptionMap["GroupRatio"], common.OptionMap["group_ratio_setting.group_ratio"])
	assert.JSONEq(t, `{"default":1,"vip":2}`, common.OptionMap["GroupRatio"])
	common.OptionMapRWMutex.RUnlock()
}

// TestTypedBulkAccessProfileFamily covers a MapConfig family without a
// side-effect-free ValidateConfigMap: per-key validation still applies.
func TestTypedBulkAccessProfileFamily(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	beforeProfiles := snapshotFamilyConfig(t, "access_profile_setting")
	swapOptionMapForTest(t, map[string]string{})
	t.Cleanup(func() { restoreFamilyConfig(t, "access_profile_setting", beforeProfiles) })

	_, _, err := UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("access_profile_setting.profiles", TypedBulkValueTypeString, `"{\"standard\":{\"label\":\"Standard access\",\"route_groups\":[\"default\"]}}"`),
	}, nil)
	require.NoError(t, err)

	profiles := setting.GetAccessProfileSetting()
	require.Len(t, profiles.Profiles, 1)
	assert.Equal(t, "Standard access", profiles.Profiles["standard"].Label)
	assert.Equal(t, []string{"default"}, profiles.Profiles["standard"].RouteGroups)
	common.OptionMapRWMutex.RLock()
	assert.Contains(t, common.OptionMap["access_profile_setting.profiles"], "Standard access")
	common.OptionMapRWMutex.RUnlock()

	// An invalid profile document is rejected before any write.
	_, _, err = UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("access_profile_setting.profiles", TypedBulkValueTypeString, `"{\"broken\":{\"label\":\"\"}}"`),
	}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTypedBulkInvalidValue)
	assert.Equal(t, "Standard access", setting.GetAccessProfileSetting().Profiles["standard"].Label)
}

func TestTypedBulkAccessProfileFamilyPublishesTierAndProfileTogether(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	beforeProfiles := snapshotFamilyConfig(t, "access_profile_setting")
	swapOptionMapForTest(t, map[string]string{})
	t.Cleanup(func() { restoreFamilyConfig(t, "access_profile_setting", beforeProfiles) })

	_, _, err := UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("access_profile_setting.profiles", TypedBulkValueTypeString, `"{\"standard\":{\"label\":\"Standard access\",\"route_groups\":[\"default\"],\"model_allowlist\":[\"gpt-5\"]}}"`),
		typedBulkItem("access_profile_setting.account_tiers", TypedBulkValueTypeString, `"{\"standard\":{\"label\":\"Standard account\",\"route_groups\":[\"default\"],\"model_allowlist\":[\"gpt-5\"]}}"`),
	}, nil)
	require.NoError(t, err)

	registry := setting.GetAccessProfileSetting()
	assert.Equal(t, []string{"default"}, registry.Profiles["standard"].RouteGroups)
	assert.Equal(t, []string{"gpt-5"}, registry.Profiles["standard"].ModelAllowlist)
	assert.Equal(t, []string{"default"}, registry.AccountTiers["standard"].RouteGroups)
	assert.Equal(t, []string{"gpt-5"}, registry.AccountTiers["standard"].ModelAllowlist)
}

func TestTypedBulkPersistenceFailureRollsBackEverything(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	beforeGeneral := snapshotFamilyConfig(t, "general_setting")
	swapOptionMapForTest(t, map[string]string{"general_setting.docs_link": "https://before"})
	t.Cleanup(func() { restoreFamilyConfig(t, "general_setting", beforeGeneral) })

	writeErr := errors.New("injected typed bulk persistence failure")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail-typed-bulk-option-create", func(tx *gorm.DB) {
		tx.AddError(writeErr)
	}))

	_, _, err := UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `"https://after"`),
	}, nil)
	require.ErrorIs(t, err, writeErr)

	assert.Zero(t, countOptions(t, db), "rolled back transaction must persist nothing")
	assert.Equal(t, beforeGeneral, snapshotFamilyConfig(t, "general_setting"), "runtime must stay unchanged")
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "https://before", common.OptionMap["general_setting.docs_link"], "OptionMap must stay unchanged")
	common.OptionMapRWMutex.RUnlock()
	current, currentErr := CurrentTypedBulkRevision()
	require.NoError(t, currentErr)
	assert.Zero(t, current, "revision must not advance on rollback")
}

// TestTypedBulkPublishFailureAbortsGroup documents the post-commit boundary:
// the failed family and all later families keep their previous runtime
// generation and OptionMap values, while the committed database state stands
// (same boundary as the payment funding bulk).
func TestTypedBulkPublishFailureAbortsGroup(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	beforeGeneral := snapshotFamilyConfig(t, "general_setting")
	beforeMonitor := snapshotFamilyConfig(t, "monitor_setting")
	swapOptionMapForTest(t, map[string]string{})
	t.Cleanup(func() {
		restoreFamilyConfig(t, "general_setting", beforeGeneral)
		restoreFamilyConfig(t, "monitor_setting", beforeMonitor)
	})

	publishErr := errors.New("injected monitor publish failure")
	originalPublish := typedBulkFamilyPublish
	monitorConfig := config.GlobalConfig.Get("monitor_setting")
	require.NotNil(t, monitorConfig)
	typedBulkFamilyPublish = func(cfg interface{}, values map[string]string) error {
		if cfg == monitorConfig {
			return publishErr
		}
		return originalPublish(cfg, values)
	}
	t.Cleanup(func() { typedBulkFamilyPublish = originalPublish })

	// general_setting sorts before monitor_setting, so the abort must leave
	// general published but monitor on its previous generation.
	_, _, err := UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `"https://published"`),
		typedBulkItem("monitor_setting.channel_test_concurrency", TypedBulkValueTypeNumber, `9`),
	}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTypedBulkPublishFailed)
	assert.ErrorIs(t, err, publishErr)

	// The database commit stands, including the single revision bump.
	var docsRow Option
	require.NoError(t, db.Where("key = ?", "general_setting.docs_link").First(&docsRow).Error)
	assert.Equal(t, "https://published", docsRow.Value)
	current, currentErr := CurrentTypedBulkRevision()
	require.NoError(t, currentErr)
	assert.Equal(t, int64(1), current)

	// The published family moved; the failed family stayed on the old generation.
	assert.Equal(t, "https://published", operation_setting.GetGeneralSetting().DocsLink)
	assert.Equal(t, beforeMonitor, snapshotFamilyConfig(t, "monitor_setting"))
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "https://published", common.OptionMap["general_setting.docs_link"])
	_, monitorLeaked := common.OptionMap["monitor_setting.channel_test_concurrency"]
	assert.False(t, monitorLeaked, "failed family must not reach OptionMap")
	common.OptionMapRWMutex.RUnlock()
}

func TestTypedBulkRevisionCAS(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	beforeGeneral := snapshotFamilyConfig(t, "general_setting")
	swapOptionMapForTest(t, map[string]string{})
	t.Cleanup(func() { restoreFamilyConfig(t, "general_setting", beforeGeneral) })

	zero := int64(0)
	revision, _, err := UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `"https://a"`),
	}, &zero)
	require.NoError(t, err)
	assert.Equal(t, int64(1), revision)

	one := int64(1)
	revision, _, err = UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `"https://b"`),
	}, &one)
	require.NoError(t, err)
	assert.Equal(t, int64(2), revision)

	// A stale expected revision conflicts and writes nothing.
	_, _, err = UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `"https://c"`),
	}, &one)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTypedBulkRevisionConflict)
	var conflictErr *TypedBulkRevisionConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Equal(t, int64(1), conflictErr.Expected)
	assert.Equal(t, int64(2), conflictErr.Actual)

	var row Option
	require.NoError(t, db.Where("key = ?", "general_setting.docs_link").First(&row).Error)
	assert.Equal(t, "https://b", row.Value, "conflicting request must not write the DB")
	assert.Equal(t, "https://b", operation_setting.GetGeneralSetting().DocsLink, "conflicting request must not publish")
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "https://b", common.OptionMap["general_setting.docs_link"], "conflicting request must not touch OptionMap")
	common.OptionMapRWMutex.RUnlock()
	current, currentErr := CurrentTypedBulkRevision()
	require.NoError(t, currentErr)
	assert.Equal(t, int64(2), current, "conflict must not advance the revision")
}

// TestTypedBulkConcurrentConflictSerialized runs two revision-guarded bulks
// concurrently: exactly one commits, the other conflicts, and the revision
// advances exactly once. The assertions hold for either scheduling order.
func TestTypedBulkConcurrentConflictSerialized(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	beforeGeneral := snapshotFamilyConfig(t, "general_setting")
	swapOptionMapForTest(t, map[string]string{})
	t.Cleanup(func() { restoreFamilyConfig(t, "general_setting", beforeGeneral) })

	zero := int64(0)
	revision, _, err := UpdateOptionsTypedBulk([]TypedBulkOption{
		typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `"https://base"`),
	}, &zero)
	require.NoError(t, err)
	require.Equal(t, int64(1), revision)

	const contenders = 2
	values := []string{"https://first", "https://second"}
	var waitGroup sync.WaitGroup
	successes := make(chan int64, contenders)
	conflicts := make(chan error, contenders)
	for i := 0; i < contenders; i++ {
		waitGroup.Add(1)
		go func(value string) {
			defer waitGroup.Done()
			expected := int64(1)
			revision, _, err := UpdateOptionsTypedBulk([]TypedBulkOption{
				typedBulkItem("general_setting.docs_link", TypedBulkValueTypeString, `"`+value+`"`),
			}, &expected)
			if err == nil {
				successes <- revision
				return
			}
			conflicts <- err
		}(values[i])
	}
	waitGroup.Wait()
	close(successes)
	close(conflicts)

	var successRevisions []int64
	for revision := range successes {
		successRevisions = append(successRevisions, revision)
	}
	var conflictCount int
	for err := range conflicts {
		assert.ErrorIs(t, err, ErrTypedBulkRevisionConflict)
		conflictCount++
	}
	require.Len(t, successRevisions, 1, "exactly one concurrent writer may commit")
	assert.Equal(t, int64(2), successRevisions[0])
	assert.Equal(t, 1, conflictCount, "the other writer must observe the revision conflict")

	var row Option
	require.NoError(t, db.Where("key = ?", "general_setting.docs_link").First(&row).Error)
	assert.Contains(t, values, row.Value)
	assert.Equal(t, row.Value, operation_setting.GetGeneralSetting().DocsLink, "runtime must match the winning commit")
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, row.Value, common.OptionMap["general_setting.docs_link"], "OptionMap must match the winning commit")
	common.OptionMapRWMutex.RUnlock()
	current, currentErr := CurrentTypedBulkRevision()
	require.NoError(t, currentErr)
	assert.Equal(t, int64(2), current, "revision advances exactly once")
}
