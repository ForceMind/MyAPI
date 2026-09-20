package performance_setting

import (
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
)

// PerformanceSetting 性能设置配置
type PerformanceSetting struct {
	// DiskCacheEnabled 是否启用磁盘缓存（磁盘换内存）
	DiskCacheEnabled bool `json:"disk_cache_enabled"`
	// DiskCacheThresholdMB 触发磁盘缓存的请求体大小阈值（MB）
	DiskCacheThresholdMB int `json:"disk_cache_threshold_mb"`
	// DiskCacheMaxSizeMB 磁盘缓存最大总大小（MB）
	DiskCacheMaxSizeMB int `json:"disk_cache_max_size_mb"`
	// DiskCachePath 磁盘缓存目录
	DiskCachePath string `json:"disk_cache_path"`

	// MonitorEnabled 是否启用性能监控
	MonitorEnabled bool `json:"monitor_enabled"`
	// MonitorCPUThreshold CPU 使用率阈值（%）
	MonitorCPUThreshold int `json:"monitor_cpu_threshold"`
	// MonitorMemoryThreshold 内存使用率阈值（%）
	MonitorMemoryThreshold int `json:"monitor_memory_threshold"`
	// MonitorDiskThreshold 磁盘使用率阈值（%）
	MonitorDiskThreshold int `json:"monitor_disk_threshold"`
}

// 默认配置
var defaultPerformanceSetting = PerformanceSetting{
	DiskCacheEnabled:     false,
	DiskCacheThresholdMB: 10,   // 超过 10MB 使用磁盘缓存
	DiskCacheMaxSizeMB:   1024, // 最大 1GB 磁盘缓存
	DiskCachePath:        "",   // 空表示使用系统临时目录

	MonitorEnabled:         true,
	MonitorCPUThreshold:    90,
	MonitorMemoryThreshold: 90,
	MonitorDiskThreshold:   95,
}

// performanceSettingGeneration 发布后不可变。
// 磁盘缓存四字段与监控四字段属于同一代：单次决策（落盘判定、监控阈值判定）
// 必须读取同一代快照，不得在代切换期间混代。
type performanceSettingGeneration struct {
	setting PerformanceSetting
}

// managedPerformanceSetting 持有注册到通用配置管理器的同步运行时快照。
type managedPerformanceSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[performanceSettingGeneration]
}

func newManagedPerformanceSetting(initial PerformanceSetting) *managedPerformanceSetting {
	state := &managedPerformanceSetting{}
	state.current.Store(&performanceSettingGeneration{setting: initial})
	return state
}

func (s *managedPerformanceSetting) snapshot() PerformanceSetting {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.setting
		}
	}
	return defaultPerformanceSetting
}

func (s *managedPerformanceSetting) candidate(values map[string]string) (PerformanceSetting, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return PerformanceSetting{}, err
	}
	return candidate, nil
}

func (s *managedPerformanceSetting) ExportConfigMap() (map[string]string, error) {
	setting := s.snapshot()
	return config.ConfigToMap(&setting)
}

func (s *managedPerformanceSetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedPerformanceSetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&performanceSettingGeneration{setting: candidate})
	return nil
}

var performanceSettingState = newManagedPerformanceSetting(defaultPerformanceSetting)

var _ config.ValidatingMapConfig = (*managedPerformanceSetting)(nil)

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("performance_setting", performanceSettingState)
	// 同步初始配置到 common 包
	UpdateAndSync()
}

// GetPerformanceSetting 获取性能设置的分离快照。
// 返回值是当代配置的副本，调用方修改它不会影响运行时状态；
// 后续保存产生的新代不会渗透进已取出的快照。
func GetPerformanceSetting() *PerformanceSetting {
	setting := performanceSettingState.snapshot()
	return &setting
}

// UpdateAndSync 把当前配置代同步到 common 包投影。
//
// 生效语义（与 docs/FULL_PRERELEASE_EXECUTION_PLAN.md D14/D15 合同一致）：
//   - 热字段（disk_cache_enabled / disk_cache_threshold_mb、monitor_*）保存后
//     立即对“新决策/新请求”生效；
//   - 磁盘放置字段（disk_cache_path / disk_cache_max_size_mb）只更新配置代，
//     不迁移正在使用的磁盘缓存；须通过维护重建端点
//     （POST /api/option/disk_cache/rebuild，最终调用 common.RebuildDiskCache）
//     或进程重启才会进入生效代。禁止在普通保存路径上偷偷换目录写文件。
//
// 当配置从数据库加载或经保存通道更新后，需要调用此函数同步。
func UpdateAndSync() {
	setting := performanceSettingState.snapshot()
	common.SetDiskCacheConfig(common.DiskCacheConfig{
		Enabled:     setting.DiskCacheEnabled,
		ThresholdMB: setting.DiskCacheThresholdMB,
		MaxSizeMB:   setting.DiskCacheMaxSizeMB,
		Path:        setting.DiskCachePath,
	})

	common.SetPerformanceMonitorConfig(common.PerformanceMonitorConfig{
		Enabled:         setting.MonitorEnabled,
		CPUThreshold:    setting.MonitorCPUThreshold,
		MemoryThreshold: setting.MonitorMemoryThreshold,
		DiskThreshold:   setting.MonitorDiskThreshold,
	})
}

// GetCacheStats 获取缓存统计信息（代理到 common 包）
func GetCacheStats() common.DiskCacheStats {
	return common.GetDiskCacheStats()
}

// ResetStats 重置统计信息
func ResetStats() {
	common.ResetDiskCacheStats()
}
