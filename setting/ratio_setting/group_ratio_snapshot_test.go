package ratio_setting

import (
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupRatioCompatibilitySnapshotIsDetached(t *testing.T) {
	previous, err := groupRatioSetting.ExportConfigMap()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, groupRatioSetting.UpdateConfigMap(previous)) })

	require.NoError(t, groupRatioSetting.UpdateConfigMap(map[string]string{
		"group_ratio":                "{\"live\":0}",
		"group_group_ratio":          "{\"vip\":{\"live\":0.5}}",
		"group_special_usable_group": "{\"vip\":{\"target\":\"mapped\"}}",
	}))

	compatibility := GetGroupRatioSetting()
	compatibility.GroupRatio.Set("live", 9)
	compatibility.GroupRatio.Clear()
	nestedRatio, ok := compatibility.GroupGroupRatio.Get("vip")
	require.True(t, ok)
	nestedRatio["live"] = 9
	compatibility.GroupGroupRatio.Set("other", map[string]float64{"value": 9})
	special, ok := compatibility.GroupSpecialUsableGroup.Get("vip")
	require.True(t, ok)
	special["target"] = "changed"
	compatibility.GroupSpecialUsableGroup.Clear()

	detachedExport, err := compatibility.ExportConfigMap()
	require.NoError(t, err)
	assert.JSONEq(t, "{}", detachedExport["group_ratio"])
	assert.JSONEq(t, "{\"vip\":{\"live\":9},\"other\":{\"value\":9}}", detachedExport["group_group_ratio"])
	assert.JSONEq(t, "{}", detachedExport["group_special_usable_group"])

	ratioCopy := GetGroupRatioCopy()
	ratioCopy["live"] = 8
	assert.Equal(t, 0.0, GetGroupRatio("live"))
	ratio, ok := GetGroupGroupRatio("vip", "live")
	require.True(t, ok)
	assert.Equal(t, 0.5, ratio)
	liveSpecial, ok := GetGroupSpecialUsableGroup("vip")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"target": "mapped"}, liveSpecial)

	liveSpecial["target"] = "mutated"
	secondRead, ok := GetGroupSpecialUsableGroup("vip")
	require.True(t, ok)
	assert.Equal(t, "mapped", secondRead["target"])

	require.NoError(t, compatibility.UpdateConfigMap(map[string]string{
		"group_ratio":                "{\"detached\":0.25}",
		"group_group_ratio":          "{\"detached\":{\"target\":0.75}}",
		"group_special_usable_group": "{\"detached\":{\"+:target\":\"Detached\"}}",
	}))
	detachedExport, err = compatibility.ExportConfigMap()
	require.NoError(t, err)
	assert.JSONEq(t, "{\"detached\":0.25}", detachedExport["group_ratio"])
	assert.JSONEq(t, "{\"detached\":{\"target\":0.75}}", detachedExport["group_group_ratio"])
	assert.JSONEq(t, "{\"detached\":{\"+:target\":\"Detached\"}}", detachedExport["group_special_usable_group"])

	assert.Equal(t, 0.0, GetGroupRatio("live"))
	ratio, ok = GetGroupGroupRatio("vip", "live")
	require.True(t, ok)
	assert.Equal(t, 0.5, ratio)
	secondRead, ok = GetGroupSpecialUsableGroup("vip")
	require.True(t, ok)
	assert.Equal(t, "mapped", secondRead["target"])
}

func TestDetachedGroupRatioSettingUpdateIsStrictAndInitializesNilMaps(t *testing.T) {
	previousLive, err := groupRatioSetting.ExportConfigMap()
	require.NoError(t, err)
	detached := &GroupRatioSetting{}

	require.NoError(t, detached.UpdateConfigMap(map[string]string{
		"group_ratio": "{\"zero\":0}",
	}))
	exported, err := detached.ExportConfigMap()
	require.NoError(t, err)
	assert.JSONEq(t, "{\"zero\":0}", exported["group_ratio"])
	assert.JSONEq(t, "{}", exported["group_group_ratio"])
	assert.JSONEq(t, "{}", exported["group_special_usable_group"])

	require.Error(t, detached.ValidateConfigMap(map[string]string{
		"group_special_usable_group": "{\"member\":{\"vip\":\"direct\",\"+:vip\":\"added\"}}",
	}))
	afterValidation, err := detached.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, exported, afterValidation)

	require.Error(t, detached.UpdateConfigMap(map[string]string{
		"group_ratio":                "{\"changed\":2}",
		"group_special_usable_group": "{\"member\":{\"vip\":\"direct\",\"+:vip\":\"added\"}}",
	}))
	afterFailure, err := detached.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, exported, afterFailure)

	afterLive, err := groupRatioSetting.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, previousLive, afterLive)
}

func TestDetachedGroupRatioSettingExportPropagatesInvalidNumberErrors(t *testing.T) {
	detached := &GroupRatioSetting{}
	_, err := detached.ExportConfigMap()
	require.NoError(t, err)

	detached.GroupRatio.Set("invalid", math.NaN())
	exported, err := detached.ExportConfigMap()
	require.Error(t, err)
	assert.Nil(t, exported)

	detached.GroupRatio.Clear()
	detached.GroupRatio.Set("valid", 0)
	detached.GroupGroupRatio.Set("vip", map[string]float64{"invalid": math.Inf(1)})
	exported, err = detached.ExportConfigMap()
	require.Error(t, err)
	assert.Nil(t, exported)

	detached.GroupGroupRatio.Clear()
	detached.GroupGroupRatio.Set("vip", map[string]float64{"valid": 0.5})
	exported, err = detached.ExportConfigMap()
	require.NoError(t, err)
	assert.JSONEq(t, "{\"valid\":0}", exported["group_ratio"])
	assert.JSONEq(t, "{\"vip\":{\"valid\":0.5}}", exported["group_group_ratio"])
	assert.JSONEq(t, "{}", exported["group_special_usable_group"])
}

func TestManagedGroupRatioSettingDeepCopiesInputs(t *testing.T) {
	group := map[string]float64{"default": 1}
	nested := map[string]map[string]float64{"vip": {"default": 0.5}}
	special := map[string]map[string]string{"vip": {"target": "mapped"}}
	setting := newManagedGroupRatioSetting(group, nested, special)

	group["default"] = 9
	nested["vip"]["default"] = 9
	special["vip"]["target"] = "changed"

	snapshot := setting.snapshot()
	assert.Equal(t, 1.0, snapshot.groupRatio["default"])
	assert.Equal(t, 0.5, snapshot.groupGroupRatio["vip"]["default"])
	assert.Equal(t, "mapped", snapshot.groupSpecialUsableGroup["vip"]["target"])

	update := groupRatioUpdate{
		groupGroupRatio:         map[string]map[string]float64{"vip": {"updated": 0}},
		groupGroupRatioSet:      true,
		groupSpecialUsableGroup: map[string]map[string]string{"vip": {"updated": "yes"}},
		groupSpecialSet:         true,
	}
	setting.apply(update)
	update.groupGroupRatio["vip"]["updated"] = 9
	update.groupSpecialUsableGroup["vip"]["updated"] = "changed"

	snapshot = setting.snapshot()
	assert.Equal(t, 0.0, snapshot.groupGroupRatio["vip"]["updated"])
	assert.Equal(t, "yes", snapshot.groupSpecialUsableGroup["vip"]["updated"])
}

func TestManagedGroupRatioConcurrentPartialUpdatesDoNotLoseFields(t *testing.T) {
	setting := newManagedGroupRatioSetting(
		map[string]float64{"old": 1},
		map[string]map[string]float64{"old": {"value": 1}},
		map[string]map[string]string{"old": {"value": "one"}},
	)
	start := make(chan struct{})
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		errors <- setting.UpdateConfigMap(map[string]string{"group_ratio": "{\"new\":0}"})
	}()
	go func() {
		defer workers.Done()
		<-start
		errors <- setting.UpdateConfigMap(map[string]string{
			"group_group_ratio": "{\"new\":{\"value\":0.25}}",
		})
	}()
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}

	exported, err := setting.ExportConfigMap()
	require.NoError(t, err)
	assert.JSONEq(t, "{\"new\":0}", exported["group_ratio"])
	assert.JSONEq(t, "{\"new\":{\"value\":0.25}}", exported["group_group_ratio"])
	assert.JSONEq(t, "{\"old\":{\"value\":\"one\"}}", exported["group_special_usable_group"])
}

func TestManagedGroupRatioExportUsesOneGeneration(t *testing.T) {
	setting := newManagedGroupRatioSetting(
		map[string]float64{"generation": 1},
		map[string]map[string]float64{"generation": {"value": 1}},
		map[string]map[string]string{"generation": {"value": "one"}},
	)
	start := make(chan struct{})
	exported := make(chan map[string]string, 1)
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		errors <- setting.UpdateConfigMap(map[string]string{
			"group_ratio":                "{\"generation\":2}",
			"group_group_ratio":          "{\"generation\":{\"value\":2}}",
			"group_special_usable_group": "{\"generation\":{\"value\":\"two\"}}",
		})
	}()
	go func() {
		defer workers.Done()
		<-start
		values, err := setting.ExportConfigMap()
		errors <- err
		exported <- values
	}()
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}

	values := <-exported
	switch values["group_ratio"] {
	case "{\"generation\":1}":
		assert.JSONEq(t, "{\"generation\":{\"value\":1}}", values["group_group_ratio"])
		assert.JSONEq(t, "{\"generation\":{\"value\":\"one\"}}", values["group_special_usable_group"])
	case "{\"generation\":2}":
		assert.JSONEq(t, "{\"generation\":{\"value\":2}}", values["group_group_ratio"])
		assert.JSONEq(t, "{\"generation\":{\"value\":\"two\"}}", values["group_special_usable_group"])
	default:
		t.Fatalf("unexpected group ratio generation: %s", values["group_ratio"])
	}
}

func TestManagedGroupRatioStrictFailurePublishesNothing(t *testing.T) {
	setting := newManagedGroupRatioSetting(
		map[string]float64{"old": 1},
		map[string]map[string]float64{"old": {"value": 1}},
		map[string]map[string]string{"old": {"value": "one"}},
	)
	before, err := setting.ExportConfigMap()
	require.NoError(t, err)
	invalid := map[string]string{
		"group_ratio":                "{\"new\":0}",
		"group_group_ratio":          "{\"new\":{\"value\":-1}}",
		"group_special_usable_group": "{\"new\":{\"value\":\"two\"}}",
	}

	require.Error(t, setting.ValidateConfigMap(invalid))
	afterValidation, err := setting.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, before, afterValidation)

	require.Error(t, setting.UpdateConfigMap(invalid))
	afterUpdate, err := setting.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, before, afterUpdate)

	invalidSpecial := map[string]string{
		"group_ratio":                "{\"new\":0}",
		"group_special_usable_group": "{\"member\":{\"vip\":\"direct\",\"-:vip\":\"removed\"}}",
	}
	require.Error(t, setting.UpdateConfigMap(invalidSpecial))
	afterSpecialFailure, err := setting.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, before, afterSpecialFailure)
}

func TestGroupRatioJSONShapes(t *testing.T) {
	previous, err := groupRatioSetting.ExportConfigMap()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, groupRatioSetting.UpdateConfigMap(previous)) })

	for _, update := range []func(string) error{
		UpdateGroupRatioByJSONString,
		UpdateGroupGroupRatioByJSONString,
		UpdateGroupSpecialUsableGroupByJSONString,
	} {
		require.NoError(t, update("{}"))
		require.Error(t, update("[]"))
		require.Error(t, update("null"))
	}

	require.NoError(t, UpdateGroupRatioByJSONString("{\"zero\":0}"))
	assert.Equal(t, 0.0, GetGroupRatio("zero"))
	require.NoError(t, UpdateGroupGroupRatioByJSONString("{\"vip\":{\"zero\":0}}"))
	ratio, ok := GetGroupGroupRatio("vip", "zero")
	require.True(t, ok)
	assert.Equal(t, 0.0, ratio)
	require.Error(t, UpdateGroupGroupRatioByJSONString("{\"vip\":null}"))
	require.Error(t, UpdateGroupSpecialUsableGroupByJSONString("{\"vip\":null}"))
}

func TestValidateGroupSpecialUsableGroupJSONRejectsAmbiguousTargets(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		value      string
		errorMatch string
	}{
		{
			name:       "empty outer user group",
			value:      "{\"\":{\"vip\":\"VIP\"}}",
			errorMatch: "user group must not be empty",
		},
		{
			name:       "blank outer user group",
			value:      "{\"  \":{\"vip\":\"VIP\"}}",
			errorMatch: "user group must not be empty",
		},
		{
			name:       "empty direct target",
			value:      "{\"member\":{\"\":\"empty\"}}",
			errorMatch: "target must not be empty",
		},
		{
			name:       "empty add target",
			value:      "{\"member\":{\"+:\":\"empty\"}}",
			errorMatch: "target must not be empty",
		},
		{
			name:       "empty remove target",
			value:      "{\"member\":{\"-:\":\"\"}}",
			errorMatch: "target must not be empty",
		},
		{
			name:       "direct and add duplicate",
			value:      "{\"member\":{\"vip\":\"direct\",\"+:vip\":\"add\"}}",
			errorMatch: "defines target \"vip\" more than once",
		},
		{
			name:       "direct and remove duplicate",
			value:      "{\"member\":{\"vip\":\"direct\",\"-:vip\":\"\"}}",
			errorMatch: "defines target \"vip\" more than once",
		},
		{
			name:       "add and remove conflict",
			value:      "{\"member\":{\"+:vip\":\"add\",\"-:vip\":\"\"}}",
			errorMatch: "defines target \"vip\" more than once",
		},
		{
			name:       "all forms duplicate",
			value:      "{\"member\":{\"vip\":\"direct\",\"+:vip\":\"add\",\"-:vip\":\"\"}}",
			errorMatch: "defines target \"vip\" more than once",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateGroupSpecialUsableGroupJSON(testCase.value)
			require.ErrorContains(t, err, testCase.errorMatch)
		})
	}

	require.NoError(t, ValidateGroupSpecialUsableGroupJSON(
		"{\"member\":{\"+:vip\":\"VIP\",\"-:legacy\":\"\",\"direct\":\"Direct\"}}",
	))
}
