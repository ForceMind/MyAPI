package logger

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type concurrentLogBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *concurrentLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *concurrentLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func isolateLogger(t *testing.T) *concurrentLogBuffer {
	t.Helper()
	output := &concurrentLogBuffer{}
	logStateLock.Lock()
	oldCount, oldWorking := logCount, setupLogWorking
	logCount, setupLogWorking = 0, false
	logStateLock.Unlock()
	currentLogPathMu.Lock()
	oldPath, oldFile := currentLogPath, currentLogFile
	currentLogPath, currentLogFile = "", nil
	currentLogPathMu.Unlock()
	common.LogWriterMu.Lock()
	oldWriter, oldErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = output, output
	common.LogWriterMu.Unlock()
	oldDir := *common.LogDir
	*common.LogDir = ""
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = oldWriter, oldErrorWriter
		common.LogWriterMu.Unlock()
		currentLogPathMu.Lock()
		file := currentLogFile
		currentLogPath, currentLogFile = oldPath, oldFile
		currentLogPathMu.Unlock()
		if file != nil {
			require.NoError(t, file.Close())
		}
		*common.LogDir = oldDir
		logStateLock.Lock()
		logCount, setupLogWorking = oldCount, oldWorking
		logStateLock.Unlock()
	})
	return output
}

func TestConcurrentLogInfoAndWarnPreserveOutput(t *testing.T) {
	output := isolateLogger(t)
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	go func() { defer workers.Done(); <-start; LogInfo(context.Background(), "concurrent info") }()
	go func() { defer workers.Done(); <-start; LogWarn(context.Background(), "concurrent warning") }()
	close(start)
	workers.Wait()
	logs := output.String()
	assert.Equal(t, 2, strings.Count(logs, "\n"))
	assert.Regexp(t, `(?m)^\[INFO\] \d{4}/\d{2}/\d{2} - \d{2}:\d{2}:\d{2} \| SYSTEM \| concurrent info $`, logs)
	assert.Regexp(t, `(?m)^\[WARN\] \d{4}/\d{2}/\d{2} - \d{2}:\d{2}:\d{2} \| SYSTEM \| concurrent warning $`, logs)
}

type blockedLogWriter struct {
	io.Writer
	entered, release chan struct{}
	once             sync.Once
}

func (w *blockedLogWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return w.Writer.Write(p)
}

func TestBlockedInfoWriterDoesNotBlockWarning(t *testing.T) {
	output := isolateLogger(t)
	writer := &blockedLogWriter{Writer: output, entered: make(chan struct{}), release: make(chan struct{})}
	common.LogWriterMu.Lock()
	gin.DefaultWriter = writer
	common.LogWriterMu.Unlock()
	var release sync.Once
	var workers sync.WaitGroup
	t.Cleanup(func() { release.Do(func() { close(writer.release) }); workers.Wait() })
	workers.Add(1)
	go func() { defer workers.Done(); LogInfo(context.Background(), "blocked info") }()
	select {
	case <-writer.entered:
	case <-time.After(time.Second):
		t.Fatal("info writer was not entered")
	}
	warningDone := make(chan struct{})
	workers.Add(1)
	go func() { defer workers.Done(); LogWarn(context.Background(), "independent warning"); close(warningDone) }()
	select {
	case <-warningDone:
	case <-time.After(time.Second):
		t.Fatal("warning was blocked by unrelated info I/O")
	}
	assert.Contains(t, output.String(), "| independent warning \n")
	release.Do(func() { close(writer.release) })
	workers.Wait()
	assert.Contains(t, output.String(), "| blocked info \n")
}

func TestManualSetupLoggerPreservesReservedRotation(t *testing.T) {
	for _, mode := range []string{"disabled", "busy", "success"} {
		t.Run(mode, func(t *testing.T) {
			output := isolateLogger(t)
			if mode != "disabled" {
				*common.LogDir = t.TempDir()
			}
			logStateLock.Lock()
			setupLogWorking = true // Simulate an existing automatic reservation, not owned by this manual call.
			logStateLock.Unlock()
			LogInfo(context.Background(), "before manual setup")
			if mode == "busy" {
				setupLogLock.Lock()
			}
			SetupLogger()
			if mode == "busy" {
				setupLogLock.Unlock()
			}
			logStateLock.Lock()
			reserved := setupLogWorking
			logStateLock.Unlock()
			assert.True(t, reserved, "manual setup must not release another task's reservation")
			assert.Contains(t, output.String(), "| before manual setup \n")
			if mode != "success" {
				assert.Empty(t, GetCurrentLogPath())
				return
			}
			LogWarn(context.Background(), "after manual setup")
			content, err := os.ReadFile(GetCurrentLogPath())
			require.NoError(t, err)
			assert.Contains(t, string(content), "[WARN]")
			assert.Contains(t, string(content), "| after manual setup \n")
		})
	}
}

func TestAutomaticRotationThresholdAndReservationRelease(t *testing.T) {
	output := isolateLogger(t)
	*common.LogDir = t.TempDir()
	logStateLock.Lock()
	logCount = maxLogCount - 1
	logStateLock.Unlock()
	LogInfo(context.Background(), "at rotation threshold")
	logStateLock.Lock()
	countAtThreshold, reservedAtThreshold := logCount, setupLogWorking
	logStateLock.Unlock()
	assert.Equal(t, maxLogCount, countAtThreshold)
	assert.False(t, reservedAtThreshold, "no automatic rotation may be reserved exactly at the threshold")
	assert.Empty(t, GetCurrentLogPath(), "the existing threshold is strictly greater than maxLogCount")
	LogWarn(context.Background(), "cross rotation threshold")
	require.Eventually(t, func() bool {
		logStateLock.Lock()
		working := setupLogWorking
		logStateLock.Unlock()
		return !working && GetCurrentLogPath() != ""
	}, time.Second, time.Millisecond, "the reserved rotation must finish and publish its file")
	assert.Contains(t, output.String(), "| at rotation threshold \n")
	assert.Contains(t, output.String(), "| cross rotation threshold \n")
	LogInfo(context.Background(), "after automatic rotation")
	content, err := os.ReadFile(GetCurrentLogPath())
	require.NoError(t, err)
	assert.Contains(t, string(content), "| after automatic rotation \n")
}
