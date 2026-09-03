package common

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceRequestBodyKeepsEveryReplayPathConsistent(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		config     func(string) DiskCacheConfig
		wantOnDisk bool
	}{
		{
			name: "memory",
			config: func(string) DiskCacheConfig {
				return DiskCacheConfig{Enabled: false, ThresholdMB: 10, MaxSizeMB: 1024}
			},
		},
		{
			name: "forced disk",
			config: func(path string) DiskCacheConfig {
				return DiskCacheConfig{Enabled: true, ThresholdMB: 0, MaxSizeMB: 1024, Path: path}
			},
			wantOnDisk: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			originalConfig := GetDiskCacheConfig()
			SetDiskCacheConfig(testCase.config(t.TempDir()))
			t.Cleanup(func() { SetDiskCacheConfig(originalConfig) })
			baseline := GetDiskCacheStats()

			original := []byte(`{"model":"old-model","prompt":"old prompt"}`)
			replacement := []byte(`{"model":"new-model","prompt":"new prompt","duration":0}`)
			request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", &singleReadBody{body: original})
			request.Header.Set("Content-Type", "application/json")
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = request

			var parsedOriginal map[string]any
			require.NoError(t, UnmarshalBodyReusable(context, &parsedOriginal))
			oldStorageValue, exists := context.Get(KeyBodyStorage)
			require.True(t, exists)
			oldStorage, ok := oldStorageValue.(BodyStorage)
			require.True(t, ok)
			assert.Equal(t, testCase.wantOnDisk, oldStorage.IsDisk())
			var oldDiskPath string
			if disk, ok := oldStorage.(*diskStorage); ok {
				oldDiskPath = disk.filePath
				_, err := os.Stat(oldDiskPath)
				require.NoError(t, err)
			}
			oldReplay, err := oldStorage.NewReader()
			require.NoError(t, err)
			context.Set(KeyRequestBody, []byte(`{"stale":true}`))

			require.NoError(t, ReplaceRequestBody(context, replacement))
			_, err = oldStorage.Bytes()
			assert.ErrorIs(t, err, ErrStorageClosed)
			if oldDiskPath != "" {
				_, err = os.Stat(oldDiskPath)
				assert.ErrorIs(t, err, os.ErrNotExist)
			}
			oldReplayBytes, err := io.ReadAll(oldReplay)
			require.NoError(t, err)
			require.NoError(t, oldReplay.Close())
			assert.Equal(t, original, oldReplayBytes, "a reader opened before replacement owns its independent lifetime")
			legacyValue, _ := context.Get(KeyRequestBody)
			assert.Nil(t, legacyValue)

			newStorageValue, exists := context.Get(KeyBodyStorage)
			require.True(t, exists)
			newStorage, ok := newStorageValue.(BodyStorage)
			require.True(t, ok)
			assert.NotSame(t, oldStorage, newStorage)
			assert.Equal(t, testCase.wantOnDisk, newStorage.IsDisk())
			assert.Equal(t, int64(len(replacement)), context.Request.ContentLength)
			require.NotNil(t, context.Request.GetBody)

			replay, err := context.Request.GetBody()
			require.NoError(t, err)
			replayBytes, err := io.ReadAll(replay)
			require.NoError(t, err)
			require.NoError(t, replay.Close())
			assert.Equal(t, replacement, replayBytes)

			directBytes, err := io.ReadAll(context.Request.Body)
			require.NoError(t, err)
			assert.Equal(t, replacement, directBytes)

			var parsedReplacement map[string]any
			require.NoError(t, UnmarshalBodyReusable(context, &parsedReplacement))
			assert.Equal(t, "new-model", parsedReplacement["model"])
			assert.Equal(t, float64(0), parsedReplacement["duration"])
			directAfterUnmarshal, err := io.ReadAll(context.Request.Body)
			require.NoError(t, err)
			assert.Equal(t, replacement, directAfterUnmarshal)

			var newDiskPath string
			if disk, ok := newStorage.(*diskStorage); ok {
				newDiskPath = disk.filePath
				_, err = os.Stat(newDiskPath)
				require.NoError(t, err)
			}
			CleanupBodyStorage(context)
			CleanupBodyStorage(context)
			_, err = newStorage.Bytes()
			assert.ErrorIs(t, err, ErrStorageClosed)
			if newDiskPath != "" {
				_, err = os.Stat(newDiskPath)
				assert.ErrorIs(t, err, os.ErrNotExist)
			}
			bodyStorageValue, _ := context.Get(KeyBodyStorage)
			legacyValue, _ = context.Get(KeyRequestBody)
			assert.Nil(t, bodyStorageValue)
			assert.Nil(t, legacyValue)
			assert.Equal(t, http.NoBody, context.Request.Body)
			assert.Nil(t, context.Request.GetBody)

			after := GetDiskCacheStats()
			assert.Equal(t, baseline.ActiveMemoryBuffers, after.ActiveMemoryBuffers)
			assert.Equal(t, baseline.CurrentMemoryUsageBytes, after.CurrentMemoryUsageBytes)
			assert.Equal(t, baseline.ActiveDiskFiles, after.ActiveDiskFiles)
			assert.Equal(t, baseline.CurrentDiskUsageBytes, after.CurrentDiskUsageBytes)
		})
	}
}

func TestReplaceRequestBodyRejectsNilRequestWithoutChangingCache(t *testing.T) {
	originalConfig := GetDiskCacheConfig()
	SetDiskCacheConfig(DiskCacheConfig{Enabled: false, ThresholdMB: 10, MaxSizeMB: 1024})
	t.Cleanup(func() { SetDiskCacheConfig(originalConfig) })

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	oldStorage, err := CreateBodyStorage([]byte(`{"old":true}`))
	require.NoError(t, err)
	context.Set(KeyBodyStorage, oldStorage)
	legacy := []byte(`{"legacy":true}`)
	context.Set(KeyRequestBody, legacy)
	t.Cleanup(func() { CleanupBodyStorage(context) })

	err = ReplaceRequestBody(context, []byte(`{"new":true}`))
	require.Error(t, err)
	storageValue, exists := context.Get(KeyBodyStorage)
	require.True(t, exists)
	assert.Same(t, oldStorage, storageValue)
	legacyValue, exists := context.Get(KeyRequestBody)
	require.True(t, exists)
	assert.Equal(t, legacy, legacyValue)
	stored, err := oldStorage.Bytes()
	require.NoError(t, err)
	assert.JSONEq(t, `{"old":true}`, string(stored))
}

// singleReadBody avoids net/http automatically populating Request.GetBody in
// this test, so every replay capability must come from the body cache itself.
type singleReadBody struct {
	body []byte
	read bool
}

func (r *singleReadBody) Read(destination []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	r.read = true
	return copy(destination, r.body), io.EOF
}
