package common

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

// DiskCacheConfig 磁盘缓存配置（由 performance_setting 包更新）
type DiskCacheConfig struct {
	// Enabled 是否启用磁盘缓存
	Enabled bool
	// ThresholdMB 触发磁盘缓存的请求体大小阈值（MB）
	ThresholdMB int
	// MaxSizeMB 磁盘缓存最大总大小（MB）
	MaxSizeMB int
	// Path 磁盘缓存目录
	Path string
}

// 磁盘缓存配置采用“配置代 / 生效代”双快照：
//
//   - desired（配置代）：最近一次保存的完整配置，由 SetDiskCacheConfig 发布。
//   - active（生效代）：磁盘缓存每次决策实际使用的快照。
//
// 热字段（Enabled、ThresholdMB）在保存后立即进入生效代，新请求使用新代规则；
// 磁盘放置字段（Path、MaxSizeMB）不在普通保存路径上迁移——它们只在显式维护
// 重建（RebuildDiskCache）或进程重启（启动时按已保存代重建）后进入生效代，
// 避免普通保存偷偷把正在使用的缓存换目录/换容量。
var (
	diskCacheDesired atomic.Value // DiskCacheConfig，配置代
	diskCacheActive  atomic.Value // DiskCacheConfig，生效代
	// diskCacheConfigMu 串行化保存与重建的“读-改-写”，读路径走原子快照无锁。
	diskCacheConfigMu sync.Mutex
)

func defaultDiskCacheConfig() DiskCacheConfig {
	return DiskCacheConfig{
		Enabled:     false,
		ThresholdMB: 10,
		MaxSizeMB:   1024,
		Path:        "",
	}
}

func init() {
	diskCacheDesired.Store(defaultDiskCacheConfig())
	diskCacheActive.Store(defaultDiskCacheConfig())
}

// GetDiskCacheConfig 获取磁盘缓存生效代快照。
// 单次调用返回同一代的全部字段，调用方应在一个决策内复用该快照，
// 而不是逐字段多次读取（避免代切换期间混代）。
func GetDiskCacheConfig() DiskCacheConfig {
	return diskCacheActive.Load().(DiskCacheConfig)
}

// GetDiskCacheDesiredConfig 获取磁盘缓存配置代（最近保存值）快照。
// 用于维护端点与状态展示比对“已保存未生效”的放置字段。
func GetDiskCacheDesiredConfig() DiskCacheConfig {
	return diskCacheDesired.Load().(DiskCacheConfig)
}

// DiskCachePlacementPending 报告配置代中的磁盘放置字段（Path/MaxSizeMB）
// 是否尚未进入生效代，即需要维护重建或重启才能生效。
func DiskCachePlacementPending() bool {
	desired := GetDiskCacheDesiredConfig()
	active := GetDiskCacheConfig()
	return desired.Path != active.Path || desired.MaxSizeMB != active.MaxSizeMB
}

// SetDiskCacheConfig 保存新的配置代。
// 热字段（Enabled、ThresholdMB）立即并入生效代，对新决策生效；
// 放置字段（Path、MaxSizeMB）只更新配置代，不迁移生效代。
func SetDiskCacheConfig(config DiskCacheConfig) {
	diskCacheConfigMu.Lock()
	defer diskCacheConfigMu.Unlock()
	diskCacheDesired.Store(config)
	active := diskCacheActive.Load().(DiskCacheConfig)
	active.Enabled = config.Enabled
	active.ThresholdMB = config.ThresholdMB
	diskCacheActive.Store(active)
}

// RebuildDiskCache 按配置代重建磁盘缓存放置（维护操作，带排空语义）。
//
// 先在旧生效代继续服务的前提下校验新目录可创建、可写；校验成功后一次性把
// 配置代的 Path/MaxSizeMB 换入生效代，此后新决策使用新目录/新容量。
// 已经打开的磁盘缓存实例按绝对路径读写/删除自己的文件，不受影响、不迁移。
// 校验失败时保留旧生效代并返回错误。
//
// 返回重建后的生效代快照。
func RebuildDiskCache() (DiskCacheConfig, error) {
	diskCacheConfigMu.Lock()
	defer diskCacheConfigMu.Unlock()

	desired := diskCacheDesired.Load().(DiskCacheConfig)
	if err := validateDiskCachePlacement(desired); err != nil {
		return diskCacheActive.Load().(DiskCacheConfig), err
	}
	diskCacheActive.Store(desired)
	return desired, nil
}

// validateDiskCachePlacement 校验目标缓存目录可创建且可写。
// 只在 RebuildDiskCache 持有写锁时调用；不做任何状态切换。
func validateDiskCachePlacement(config DiskCacheConfig) error {
	dir := diskCacheDirFor(config.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create disk cache directory %s: %w", dir, err)
	}
	probe, err := os.CreateTemp(dir, ".rebuild-probe-*")
	if err != nil {
		return fmt.Errorf("failed to write probe file in disk cache directory %s: %w", dir, err)
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		os.Remove(probePath)
		return fmt.Errorf("failed to close probe file in disk cache directory %s: %w", dir, err)
	}
	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("failed to remove probe file in disk cache directory %s: %w", dir, err)
	}
	return nil
}

// IsDiskCacheEnabled 是否启用磁盘缓存（生效代）
func IsDiskCacheEnabled() bool {
	return GetDiskCacheConfig().Enabled
}

// GetDiskCacheThresholdBytes 获取磁盘缓存阈值（字节，生效代）
func GetDiskCacheThresholdBytes() int64 {
	return int64(GetDiskCacheConfig().ThresholdMB) << 20
}

// GetDiskCacheMaxSizeBytes 获取磁盘缓存最大大小（字节，生效代）
func GetDiskCacheMaxSizeBytes() int64 {
	return int64(GetDiskCacheConfig().MaxSizeMB) << 20
}

// GetDiskCachePath 获取磁盘缓存目录（生效代）
func GetDiskCachePath() string {
	return GetDiskCacheConfig().Path
}

// DiskCacheStats 磁盘缓存统计信息
type DiskCacheStats struct {
	// 当前活跃的磁盘缓存文件数
	ActiveDiskFiles int64 `json:"active_disk_files"`
	// 当前磁盘缓存总大小（字节）
	CurrentDiskUsageBytes int64 `json:"current_disk_usage_bytes"`
	// 当前内存缓存数量
	ActiveMemoryBuffers int64 `json:"active_memory_buffers"`
	// 当前内存缓存总大小（字节）
	CurrentMemoryUsageBytes int64 `json:"current_memory_usage_bytes"`
	// 磁盘缓存命中次数
	DiskCacheHits int64 `json:"disk_cache_hits"`
	// 内存缓存命中次数
	MemoryCacheHits int64 `json:"memory_cache_hits"`
	// 磁盘缓存最大限制（字节）
	DiskCacheMaxBytes int64 `json:"disk_cache_max_bytes"`
	// 磁盘缓存阈值（字节）
	DiskCacheThresholdBytes int64 `json:"disk_cache_threshold_bytes"`
}

var diskCacheStats DiskCacheStats

// GetDiskCacheStats 获取缓存统计信息
func GetDiskCacheStats() DiskCacheStats {
	config := GetDiskCacheConfig()
	stats := DiskCacheStats{
		ActiveDiskFiles:         atomic.LoadInt64(&diskCacheStats.ActiveDiskFiles),
		CurrentDiskUsageBytes:   atomic.LoadInt64(&diskCacheStats.CurrentDiskUsageBytes),
		ActiveMemoryBuffers:     atomic.LoadInt64(&diskCacheStats.ActiveMemoryBuffers),
		CurrentMemoryUsageBytes: atomic.LoadInt64(&diskCacheStats.CurrentMemoryUsageBytes),
		DiskCacheHits:           atomic.LoadInt64(&diskCacheStats.DiskCacheHits),
		MemoryCacheHits:         atomic.LoadInt64(&diskCacheStats.MemoryCacheHits),
		DiskCacheMaxBytes:       int64(config.MaxSizeMB) << 20,
		DiskCacheThresholdBytes: int64(config.ThresholdMB) << 20,
	}
	return stats
}

// IncrementDiskFiles 增加磁盘文件计数
func IncrementDiskFiles(size int64) {
	atomic.AddInt64(&diskCacheStats.ActiveDiskFiles, 1)
	atomic.AddInt64(&diskCacheStats.CurrentDiskUsageBytes, size)
}

// DecrementDiskFiles 减少磁盘文件计数
func DecrementDiskFiles(size int64) {
	if atomic.AddInt64(&diskCacheStats.ActiveDiskFiles, -1) < 0 {
		atomic.StoreInt64(&diskCacheStats.ActiveDiskFiles, 0)
	}
	if atomic.AddInt64(&diskCacheStats.CurrentDiskUsageBytes, -size) < 0 {
		atomic.StoreInt64(&diskCacheStats.CurrentDiskUsageBytes, 0)
	}
}

// IncrementMemoryBuffers 增加内存缓存计数
func IncrementMemoryBuffers(size int64) {
	atomic.AddInt64(&diskCacheStats.ActiveMemoryBuffers, 1)
	atomic.AddInt64(&diskCacheStats.CurrentMemoryUsageBytes, size)
}

// DecrementMemoryBuffers 减少内存缓存计数
func DecrementMemoryBuffers(size int64) {
	atomic.AddInt64(&diskCacheStats.ActiveMemoryBuffers, -1)
	atomic.AddInt64(&diskCacheStats.CurrentMemoryUsageBytes, -size)
}

// IncrementDiskCacheHits 增加磁盘缓存命中次数
func IncrementDiskCacheHits() {
	atomic.AddInt64(&diskCacheStats.DiskCacheHits, 1)
}

// IncrementMemoryCacheHits 增加内存缓存命中次数
func IncrementMemoryCacheHits() {
	atomic.AddInt64(&diskCacheStats.MemoryCacheHits, 1)
}

// ResetDiskCacheStats 重置命中统计信息（不重置当前使用量）
func ResetDiskCacheStats() {
	atomic.StoreInt64(&diskCacheStats.DiskCacheHits, 0)
	atomic.StoreInt64(&diskCacheStats.MemoryCacheHits, 0)
}

// ResetDiskCacheUsage 重置磁盘缓存使用量统计（用于清理缓存后）
func ResetDiskCacheUsage() {
	atomic.StoreInt64(&diskCacheStats.ActiveDiskFiles, 0)
	atomic.StoreInt64(&diskCacheStats.CurrentDiskUsageBytes, 0)
}

// SyncDiskCacheStats 从实际磁盘状态同步统计信息
// 用于修正统计与实际不符的情况
func SyncDiskCacheStats() {
	fileCount, totalSize, err := GetDiskCacheInfo()
	if err != nil {
		return
	}
	atomic.StoreInt64(&diskCacheStats.ActiveDiskFiles, int64(fileCount))
	atomic.StoreInt64(&diskCacheStats.CurrentDiskUsageBytes, totalSize)
}

// IsDiskCacheAvailable 检查是否可以创建新的磁盘缓存
func IsDiskCacheAvailable(requestSize int64) bool {
	config := GetDiskCacheConfig()
	if !config.Enabled {
		return false
	}
	maxBytes := int64(config.MaxSizeMB) << 20
	currentUsage := atomic.LoadInt64(&diskCacheStats.CurrentDiskUsageBytes)
	return currentUsage+requestSize <= maxBytes
}
