package common

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withDiskCacheConfig 保存指定配置代并按需重建，测试结束后恢复原配置代并重建。
// applyPlacement 为 true 时等价于“保存 + 维护重建”，为 false 时只是普通保存。
func withDiskCacheConfig(t *testing.T, config DiskCacheConfig, applyPlacement bool) {
	t.Helper()
	original := GetDiskCacheDesiredConfig()
	SetDiskCacheConfig(config)
	if applyPlacement {
		_, err := RebuildDiskCache()
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		SetDiskCacheConfig(original)
		_, _ = RebuildDiskCache()
	})
}

// 保存后热字段（Enabled/ThresholdMB）立即进入生效代，
// 放置字段（Path/MaxSizeMB）保持旧生效代。
func TestSetDiskCacheConfigHotFieldsApplyImmediately(t *testing.T) {
	base := DiskCacheConfig{Enabled: false, ThresholdMB: 10, MaxSizeMB: 128, Path: ""}
	withDiskCacheConfig(t, base, true)

	saved := DiskCacheConfig{Enabled: true, ThresholdMB: 3, MaxSizeMB: 4096, Path: t.TempDir()}
	SetDiskCacheConfig(saved)

	active := GetDiskCacheConfig()
	assert.True(t, active.Enabled, "enabled 是热字段，保存后应立即生效")
	assert.Equal(t, 3, active.ThresholdMB, "threshold 是热字段，保存后应立即生效")
	assert.Equal(t, 128, active.MaxSizeMB, "max size 是放置字段，普通保存不得迁移生效代")
	assert.Equal(t, "", active.Path, "path 是放置字段，普通保存不得迁移生效代")

	desired := GetDiskCacheDesiredConfig()
	assert.Equal(t, saved, desired, "配置代必须完整记录已保存值")
	assert.True(t, DiskCachePlacementPending(), "放置字段已保存未生效时必须可核对")
}

// 维护重建成功：生效代一次性切换到配置代，新决策写入新目录。
func TestRebuildDiskCacheAppliesPlacement(t *testing.T) {
	base := DiskCacheConfig{Enabled: true, ThresholdMB: 0, MaxSizeMB: 64, Path: ""}
	withDiskCacheConfig(t, base, true)

	newDir := t.TempDir()
	SetDiskCacheConfig(DiskCacheConfig{Enabled: true, ThresholdMB: 0, MaxSizeMB: 256, Path: newDir})
	require.True(t, DiskCachePlacementPending())

	active, err := RebuildDiskCache()
	require.NoError(t, err)
	assert.Equal(t, newDir, active.Path)
	assert.Equal(t, 256, active.MaxSizeMB)
	assert.False(t, DiskCachePlacementPending())
	assert.Equal(t, active, GetDiskCacheConfig())

	storage, err := CreateBodyStorage([]byte(`{"model":"rebuild-check"}`))
	require.NoError(t, err)
	require.True(t, storage.IsDisk())
	disk, ok := storage.(*diskStorage)
	require.True(t, ok)
	assert.Equal(t, diskCacheDirFor(newDir), filepath.Dir(disk.filePath), "重建后新请求必须写入新目录")
	require.NoError(t, storage.Close())
}

// 维护重建失败：保留旧生效代并返回错误，不进行任何迁移。
func TestRebuildDiskCacheFailureKeepsActiveGeneration(t *testing.T) {
	baseDir := t.TempDir()
	base := DiskCacheConfig{Enabled: true, ThresholdMB: 0, MaxSizeMB: 64, Path: baseDir}
	withDiskCacheConfig(t, base, true)

	// 用一个已存在的普通文件作为缓存根目录，MkdirAll 必然失败。
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocker, []byte("block"), 0600))

	SetDiskCacheConfig(DiskCacheConfig{Enabled: true, ThresholdMB: 0, MaxSizeMB: 512, Path: blocker})
	active, err := RebuildDiskCache()
	require.Error(t, err)
	assert.Equal(t, base, active, "重建失败必须返回并保留旧生效代")
	assert.Equal(t, base, GetDiskCacheConfig(), "重建失败不得切换生效代")

	// 旧生效代仍然可用：新文件仍写入旧目录。
	storage, err := CreateBodyStorage([]byte(`{"model":"still-old"}`))
	require.NoError(t, err)
	require.True(t, storage.IsDisk())
	disk, ok := storage.(*diskStorage)
	require.True(t, ok)
	assert.Equal(t, diskCacheDirFor(baseDir), filepath.Dir(disk.filePath))
	require.NoError(t, storage.Close())
}

// 进行中的磁盘缓存实例不被换目录：换代后旧实例仍按自己的绝对路径读写/删除。
func TestOpenDiskStorageInstanceSurvivesRebuild(t *testing.T) {
	dirA := t.TempDir()
	withDiskCacheConfig(t, DiskCacheConfig{Enabled: true, ThresholdMB: 0, MaxSizeMB: 64, Path: dirA}, true)

	payload := []byte(`{"model":"in-flight"}`)
	oldStorage, err := CreateBodyStorage(payload)
	require.NoError(t, err)
	require.True(t, oldStorage.IsDisk())
	oldDisk, ok := oldStorage.(*diskStorage)
	require.True(t, ok)
	assert.Equal(t, diskCacheDirFor(dirA), filepath.Dir(oldDisk.filePath))

	// 保存 + 维护重建到 dirB：只影响新决策。
	dirB := t.TempDir()
	SetDiskCacheConfig(DiskCacheConfig{Enabled: true, ThresholdMB: 0, MaxSizeMB: 64, Path: dirB})
	_, err = RebuildDiskCache()
	require.NoError(t, err)

	data, err := oldStorage.Bytes()
	require.NoError(t, err)
	assert.Equal(t, payload, data, "换代后旧实例必须仍可从原文件读取")

	newStorage, err := CreateBodyStorage(payload)
	require.NoError(t, err)
	require.True(t, newStorage.IsDisk())
	newDisk, ok := newStorage.(*diskStorage)
	require.True(t, ok)
	assert.Equal(t, diskCacheDirFor(dirB), filepath.Dir(newDisk.filePath), "新请求必须使用新生效代目录")

	require.NoError(t, oldStorage.Close())
	_, err = os.Stat(oldDisk.filePath)
	assert.ErrorIs(t, err, os.ErrNotExist, "旧实例关闭必须清理自己原目录下的文件")
	require.NoError(t, newStorage.Close())
}

// 单次决策使用同一快照：CreateBodyStorage 的判定与目录选择来自同一生效代。
func TestCreateBodyStorageUsesSingleSnapshot(t *testing.T) {
	dir := t.TempDir()
	// enabled=true、threshold=0 但 max=0MB：容量不足以落盘，必须整体判定为内存存储。
	// 若决策混代（enabled/threshold 与 max 来自不同代），行为会与此断言不一致。
	withDiskCacheConfig(t, DiskCacheConfig{Enabled: true, ThresholdMB: 0, MaxSizeMB: 0, Path: dir}, true)

	storage, err := CreateBodyStorage([]byte(`{"model":"snapshot"}`))
	require.NoError(t, err)
	assert.False(t, storage.IsDisk(), "同代快照下 max=0 必须拒绝落盘并整体回退内存")
	require.NoError(t, storage.Close())
}

// 代切换期间并发读写不混代：任何一次 GetDiskCacheConfig 都必须返回某个
// 完整代，而不是两个代的字段拼接。
func TestConcurrentGenerationSwitchDoesNotMixGenerations(t *testing.T) {
	genA := DiskCacheConfig{Enabled: true, ThresholdMB: 1, MaxSizeMB: 64, Path: t.TempDir()}
	genB := DiskCacheConfig{Enabled: true, ThresholdMB: 9, MaxSizeMB: 512, Path: t.TempDir()}
	withDiskCacheConfig(t, genA, true)

	const iterations = 200
	var wg sync.WaitGroup
	stop := make(chan struct{})
	errCh := make(chan string, 1)

	// 读方：每次快照必须是一个完整的已定义生效代。保存后、维护重建前
	// 热字段来自新配置而放置字段仍来自旧配置，这是刻意保留的过渡代，
	// 不能误判为字段撕裂。
	hotBPlacementA := DiskCacheConfig{Enabled: genB.Enabled, ThresholdMB: genB.ThresholdMB, MaxSizeMB: genA.MaxSizeMB, Path: genA.Path}
	hotAPlacementB := DiskCacheConfig{Enabled: genA.Enabled, ThresholdMB: genA.ThresholdMB, MaxSizeMB: genB.MaxSizeMB, Path: genB.Path}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			config := GetDiskCacheConfig()
			consistent := config == genA || config == genB || config == hotBPlacementA || config == hotAPlacementB
			if !consistent {
				select {
				case errCh <- "mixed generation observed":
				default:
				}
				return
			}
		}
	}()

	// 写方：交替保存并重建两个完整代。
	for i := 0; i < iterations; i++ {
		if i%2 == 0 {
			SetDiskCacheConfig(genB)
		} else {
			SetDiskCacheConfig(genA)
		}
		if _, err := RebuildDiskCache(); err != nil {
			t.Fatalf("rebuild failed: %v", err)
		}
	}
	close(stop)
	wg.Wait()

	select {
	case msg := <-errCh:
		t.Fatal(msg)
	default:
	}
}
