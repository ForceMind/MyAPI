package common

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// DiskCacheType 磁盘缓存类型
type DiskCacheType string

const (
	DiskCacheTypeBody DiskCacheType = "body" // 请求体缓存
	DiskCacheTypeFile DiskCacheType = "file" // 文件数据缓存
)

// diskCacheDir is the canonical cache directory for new MyAPI deployments.
// legacyDiskCacheDir is intentionally retained for read/cleanup compatibility
// with files written by earlier releases.  Existing file paths are persisted
// by some task payloads, so callers must continue to be able to read/remove
// those paths without a database migration.
const (
	diskCacheDir       = "my-api-body-cache"
	legacyDiskCacheDir = "new-api-body-cache"
)

// diskCacheBaseDir 解析缓存根目录：空路径表示系统临时目录。
func diskCacheBaseDir(cachePath string) string {
	if cachePath == "" {
		return os.TempDir()
	}
	return cachePath
}

// diskCacheDirFor 返回指定缓存根目录下的规范缓存目录。
func diskCacheDirFor(cachePath string) string {
	return filepath.Join(diskCacheBaseDir(cachePath), diskCacheDir)
}

func diskCacheDirs() []string {
	cachePath := GetDiskCachePath()
	base := diskCacheBaseDir(cachePath)
	canonical := filepath.Join(base, diskCacheDir)
	legacy := filepath.Join(base, legacyDiskCacheDir)
	if canonical == legacy {
		return []string{canonical}
	}
	return []string{canonical, legacy}
}

// GetDiskCacheDir 获取统一的磁盘缓存目录（生效代）
// 注意：每次调用都会按生效代重新计算
func GetDiskCacheDir() string {
	return diskCacheDirFor(GetDiskCachePath())
}

// EnsureDiskCacheDir 确保缓存目录存在（生效代目录）
func EnsureDiskCacheDir() error {
	dir := GetDiskCacheDir()
	return os.MkdirAll(dir, 0755)
}

// CreateDiskCacheFileIn 在指定缓存根目录下创建磁盘缓存文件。
// cachePath 为空时使用系统临时目录。传入的路径必须来自调用方决策时
// 使用的同一生效代快照，保证一次决策的目录选择一致。
func CreateDiskCacheFileIn(cacheType DiskCacheType, cachePath string) (string, *os.File, error) {
	dir := diskCacheDirFor(cachePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	filename := fmt.Sprintf("%s-%s-%d.tmp", cacheType, uuid.New().String()[:8], time.Now().UnixNano())
	filePath := filepath.Join(dir, filename)

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0600)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create cache file: %w", err)
	}

	return filePath, file, nil
}

// CreateDiskCacheFile 创建磁盘缓存文件（使用生效代目录）
// cacheType: 缓存类型（body/file）
// 返回文件路径和文件句柄
func CreateDiskCacheFile(cacheType DiskCacheType) (string, *os.File, error) {
	return CreateDiskCacheFileIn(cacheType, GetDiskCachePath())
}

// WriteDiskCacheFile 写入数据到磁盘缓存文件
// 返回文件路径
func WriteDiskCacheFile(cacheType DiskCacheType, data []byte) (string, error) {
	filePath, file, err := CreateDiskCacheFile(cacheType)
	if err != nil {
		return "", err
	}

	_, err = file.Write(data)
	if err != nil {
		file.Close()
		os.Remove(filePath)
		return "", fmt.Errorf("failed to write cache file: %w", err)
	}

	if err := file.Close(); err != nil {
		os.Remove(filePath)
		return "", fmt.Errorf("failed to close cache file: %w", err)
	}

	return filePath, nil
}

// WriteDiskCacheFileString 写入字符串到磁盘缓存文件
func WriteDiskCacheFileString(cacheType DiskCacheType, data string) (string, error) {
	return WriteDiskCacheFile(cacheType, []byte(data))
}

// ReadDiskCacheFile 读取磁盘缓存文件
func ReadDiskCacheFile(filePath string) ([]byte, error) {
	return os.ReadFile(filePath)
}

// ReadDiskCacheFileString 读取磁盘缓存文件为字符串
func ReadDiskCacheFileString(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RemoveDiskCacheFile 删除磁盘缓存文件
func RemoveDiskCacheFile(filePath string) error {
	return os.Remove(filePath)
}

// CleanupOldDiskCacheFiles 清理旧的缓存文件
// maxAge: 文件最大存活时间
// 注意：此函数只删除文件，不更新统计（因为无法知道每个文件的原始大小）
func CleanupOldDiskCacheFiles(maxAge time.Duration) error {
	now := time.Now()
	var firstErr error
	for _, dir := range diskCacheDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue // 目录不存在，无需清理
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if now.Sub(info.ModTime()) > maxAge {
				// 注意：后台清理任务删除文件时，由于无法得知原始 base64Size，
				// 只能按磁盘文件大小扣减。这在目前 base64 存储模式下是准确的。
				if err := os.Remove(filepath.Join(dir, entry.Name())); err == nil {
					DecrementDiskFiles(info.Size())
				}
			}
		}
	}
	return firstErr
}

// GetDiskCacheInfo 获取磁盘缓存目录信息
func GetDiskCacheInfo() (fileCount int, totalSize int64, err error) {
	var firstErr error
	for _, dir := range diskCacheDirs() {
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			if firstErr == nil {
				firstErr = readErr
			}
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				continue
			}
			fileCount++
			totalSize += info.Size()
		}
	}
	return fileCount, totalSize, firstErr
}

// ShouldUseDiskCache 判断是否应该使用磁盘缓存。
// 单次决策读取同一生效代快照，enabled/threshold/max 来自同一代，不混代。
func ShouldUseDiskCache(dataSize int64) bool {
	config := GetDiskCacheConfig()
	if !config.Enabled {
		return false
	}
	threshold := int64(config.ThresholdMB) << 20
	if dataSize < threshold {
		return false
	}
	maxBytes := int64(config.MaxSizeMB) << 20
	currentUsage := atomic.LoadInt64(&diskCacheStats.CurrentDiskUsageBytes)
	return currentUsage+dataSize <= maxBytes
}
