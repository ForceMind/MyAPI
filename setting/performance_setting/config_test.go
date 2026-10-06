package performance_setting

import (
	"strconv"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func intString(value int) string {
	return strconv.Itoa(value)
}

func updatePerformanceSettingForTest(t *testing.T, values map[string]string) {
	t.Helper()
	require.NoError(t, performanceSettingState.UpdateConfigMap(values))
}

// restorePerformanceSettingForTest 在测试结束后恢复原配置代并同步 common 投影。
func restorePerformanceSettingForTest(t *testing.T) {
	original := GetPerformanceSetting()
	originalDesired := common.GetDiskCacheDesiredConfig()
	originalMonitor := common.GetPerformanceMonitorConfig()
	t.Cleanup(func() {
		require.NoError(t, performanceSettingState.UpdateConfigMap(map[string]string{
			"disk_cache_enabled":       boolString(original.DiskCacheEnabled),
			"disk_cache_threshold_mb":  intString(original.DiskCacheThresholdMB),
			"disk_cache_max_size_mb":   intString(original.DiskCacheMaxSizeMB),
			"disk_cache_path":          original.DiskCachePath,
			"monitor_enabled":          boolString(original.MonitorEnabled),
			"monitor_cpu_threshold":    intString(original.MonitorCPUThreshold),
			"monitor_memory_threshold": intString(original.MonitorMemoryThreshold),
			"monitor_disk_threshold":   intString(original.MonitorDiskThreshold),
		}))
		UpdateAndSync()
		common.SetDiskCacheConfig(originalDesired)
		common.SetPerformanceMonitorConfig(originalMonitor)
		_, _ = common.RebuildDiskCache()
	})
}

// 保存后：getter 读到新代；新代发布前取出的旧快照不被渗透（不可变代）。
func TestPerformanceSettingGenerationIsImmutable(t *testing.T) {
	restorePerformanceSettingForTest(t)

	before := GetPerformanceSetting()
	beforeThreshold := before.DiskCacheThresholdMB

	updatePerformanceSettingForTest(t, map[string]string{
		"disk_cache_threshold_mb": intString(beforeThreshold + 7),
	})

	assert.Equal(t, beforeThreshold, before.DiskCacheThresholdMB, "已取出的快照不得被新代渗透")
	assert.Equal(t, beforeThreshold+7, GetPerformanceSetting().DiskCacheThresholdMB, "保存后新请求必须读到新代")
}

// getter 返回分离快照：修改返回值不影响运行时状态。
func TestGetPerformanceSettingReturnsDetachedSnapshot(t *testing.T) {
	setting := GetPerformanceSetting()
	originalThreshold := setting.DiskCacheThresholdMB

	setting.DiskCacheThresholdMB = originalThreshold + 1000
	setting.DiskCachePath = "/detached/mutation"
	setting.MonitorCPUThreshold = -1

	fresh := GetPerformanceSetting()
	assert.Equal(t, originalThreshold, fresh.DiskCacheThresholdMB)
	assert.NotEqual(t, "/detached/mutation", fresh.DiskCachePath)
	assert.NotEqual(t, -1, fresh.MonitorCPUThreshold)
}

// UpdateAndSync 把新代发布到 common 投影：热字段立即生效，
// 磁盘放置字段只进配置代，经维护重建才进生效代。
func TestUpdateAndSyncPublishesGenerationSemantics(t *testing.T) {
	restorePerformanceSettingForTest(t)

	newDir := t.TempDir()
	updatePerformanceSettingForTest(t, map[string]string{
		"disk_cache_enabled":      "true",
		"disk_cache_threshold_mb": "5",
		"disk_cache_max_size_mb":  "777",
		"disk_cache_path":         newDir,
		"monitor_cpu_threshold":   "42",
	})
	UpdateAndSync()

	desired := common.GetDiskCacheDesiredConfig()
	assert.Equal(t, 777, desired.MaxSizeMB)
	assert.Equal(t, newDir, desired.Path)

	active := common.GetDiskCacheConfig()
	assert.True(t, active.Enabled, "enabled 热字段保存后立即生效")
	assert.Equal(t, 5, active.ThresholdMB, "threshold 热字段保存后立即生效")
	assert.NotEqual(t, 777, active.MaxSizeMB, "max size 放置字段不得在普通保存路径迁移")
	assert.NotEqual(t, newDir, active.Path, "path 放置字段不得在普通保存路径迁移")
	assert.True(t, common.DiskCachePlacementPending())

	monitor := common.GetPerformanceMonitorConfig()
	assert.Equal(t, 42, monitor.CPUThreshold, "监控字段与磁盘缓存字段同族同代发布")

	// 维护重建：放置字段进入生效代。
	rebuilt, err := common.RebuildDiskCache()
	require.NoError(t, err)
	assert.Equal(t, newDir, rebuilt.Path)
	assert.Equal(t, 777, rebuilt.MaxSizeMB)
	assert.False(t, common.DiskCachePlacementPending())
}

// 非法值不发布：ValidateConfigMap/UpdateConfigMap 拒绝后保持原代。
func TestPerformanceSettingRejectsInvalidValuesWithoutPublishing(t *testing.T) {
	before := GetPerformanceSetting()

	err := performanceSettingState.ValidateConfigMap(map[string]string{
		"disk_cache_threshold_mb": "not-a-number",
	})
	require.Error(t, err)

	err = performanceSettingState.UpdateConfigMap(map[string]string{
		"disk_cache_threshold_mb": "not-a-number",
	})
	require.Error(t, err)

	assert.Equal(t, *before, *GetPerformanceSetting(), "校验失败不得发布部分更新的代")
}

// 注册到全局配置管理器的形态必须是 ValidatingMapConfig，
// 保证通用加载/导出/校验通道都经过不可变代。
func TestPerformanceSettingRegisteredAsValidatingMapConfig(t *testing.T) {
	registered := config.GlobalConfig.Get("performance_setting")
	require.NotNil(t, registered)
	_, ok := registered.(config.ValidatingMapConfig)
	assert.True(t, ok, "performance_setting 必须以 ValidatingMapConfig 注册")
}
