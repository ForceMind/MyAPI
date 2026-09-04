package middleware

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullContentLoggerCapturesRequestAndStreamingResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logDir := t.TempDir()
	t.Setenv(fullContentLogEnabledEnv, "true")
	t.Setenv(fullContentLogDirEnv, logDir)
	t.Setenv(fullContentLogMaxMBEnv, "1")
	t.Setenv(fullContentLogMaxFilesEnv, "0")

	engine := gin.New()
	engine.Use(BodyStorageCleanup())
	group := engine.Group("/v1")
	group.Use(func(c *gin.Context) {
		c.Set(common.RequestIdKey, "request-123")
		c.Set("id", 7)
		c.Set("token_id", 9)
		c.Set("token_name", "integration-test")
		c.Next()
	})
	group.Use(FullContentLogger())

	var handlerBody string
	group.POST("/responses", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		handlerBody = string(body)
		c.Header("Content-Type", "text/event-stream")
		_, err = c.Writer.WriteString("data: first\n\n")
		require.NoError(t, err)
		c.Writer.Flush()
		_, err = c.Writer.Write([]byte("data: second\n\n"))
		require.NoError(t, err)
	})

	requestJSON := `{"messages":[{"role":"user","content":"hello"}],"api_key":"should-hide","nested":{"password":"also-hide","access_token":"access-hide","refresh_token":"refresh-hide","id_token":"identity-hide"}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/responses?api_key=query-secret", strings.NewReader(requestJSON))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer header-secret")
	request.Header.Set("Cookie", "session=cookie-secret")
	request.Header.Set("User-Agent", "OpenClaw-AI/1.0")
	request.Header.Set("X-Request-Source", "openclaw")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, requestJSON, handlerBody, "logging must leave a replayable request body for downstream handlers")
	assert.Equal(t, "data: first\n\ndata: second\n\n", recorder.Body.String())

	files, err := filepath.Glob(filepath.Join(logDir, "full-content-*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)

	dirInfo, err := os.Stat(logDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), dirInfo.Mode().Perm())
	fileInfo, err := os.Stat(files[0])
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), fileInfo.Mode().Perm())

	logBytes, err := os.ReadFile(files[0])
	require.NoError(t, err)
	logText := string(logBytes)
	assert.NotContains(t, logText, "should-hide")
	assert.NotContains(t, logText, "also-hide")
	assert.NotContains(t, logText, "access-hide")
	assert.NotContains(t, logText, "refresh-hide")
	assert.NotContains(t, logText, "identity-hide")
	assert.NotContains(t, logText, "header-secret")
	assert.NotContains(t, logText, "cookie-secret")
	assert.NotContains(t, logText, "query-secret")

	lines := strings.Split(strings.TrimSpace(logText), "\n")
	require.Len(t, lines, 4)
	entries := make([]fullContentLogEntry, 0, len(lines))
	for _, line := range lines {
		var entry fullContentLogEntry
		require.NoError(t, common.Unmarshal([]byte(line), &entry))
		entries = append(entries, entry)
	}

	requestEntry := entries[0]
	assert.Equal(t, "request", requestEntry.Phase)
	assert.Equal(t, "request-123", requestEntry.RequestID)
	assert.Equal(t, http.MethodPost, requestEntry.Method)
	assert.Equal(t, "/v1/responses", requestEntry.Path)
	assert.Equal(t, "json", requestEntry.Encoding)
	assert.Equal(t, int64(len(requestJSON)), requestEntry.BodyBytes)
	assert.Contains(t, requestEntry.Body, `"content":"hello"`)
	assert.Contains(t, requestEntry.Body, `"api_key":"[REDACTED]"`)
	assert.Contains(t, requestEntry.Body, `"password":"[REDACTED]"`)
	assert.Contains(t, requestEntry.Body, `"access_token":"[REDACTED]"`)
	assert.Contains(t, requestEntry.Body, `"refresh_token":"[REDACTED]"`)
	assert.Contains(t, requestEntry.Body, `"id_token":"[REDACTED]"`)
	assert.Equal(t, 7, requestEntry.UserID)
	assert.Equal(t, 9, requestEntry.TokenID)
	assert.Equal(t, "integration-test", requestEntry.TokenName)
	assert.Equal(t, []string{"OpenClaw-AI/1.0"}, requestEntry.Headers["User-Agent"])
	assert.Equal(t, []string{"openclaw"}, requestEntry.Headers["X-Request-Source"])
	assert.Equal(t, []string{"[REDACTED]"}, requestEntry.Headers["Authorization"])
	assert.Equal(t, []string{"[REDACTED]"}, requestEntry.Headers["Cookie"])
	assert.Equal(t, []string{"[REDACTED]"}, requestEntry.Query["api_key"])

	assert.Equal(t, "response_chunk", entries[1].Phase)
	assert.Equal(t, int64(1), entries[1].Sequence)
	assert.Equal(t, "data: first\n\n", entries[1].Body)
	assert.Equal(t, "response_chunk", entries[2].Phase)
	assert.Equal(t, int64(2), entries[2].Sequence)
	assert.Equal(t, "data: second\n\n", entries[2].Body)

	endEntry := entries[3]
	assert.Equal(t, "response_end", endEntry.Phase)
	assert.Equal(t, http.StatusOK, endEntry.Status)
	assert.Equal(t, int64(2), endEntry.Sequence)
	assert.Equal(t, int64(len("data: first\n\ndata: second\n\n")), endEntry.BodyBytes)
	assert.Equal(t, []string{"text/event-stream"}, endEntry.Headers["Content-Type"])
}

func TestFullContentLoggerDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logDir := filepath.Join(t.TempDir(), "disabled")
	t.Setenv(fullContentLogEnabledEnv, "false")
	t.Setenv(fullContentLogDirEnv, logDir)

	engine := gin.New()
	engine.Use(FullContentLogger())
	engine.POST("/v1/responses", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"hello"}`))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	_, err := os.Stat(logDir)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestFullContentLoggerReusesWriterForMatchingConfiguration(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv(fullContentLogEnabledEnv, "true")
	t.Setenv(fullContentLogDirEnv, logDir)
	t.Setenv(fullContentLogMaxMBEnv, "7")
	t.Setenv(fullContentLogMaxFilesEnv, "3")

	first := getFullContentLogWriter()
	second := getFullContentLogWriter()

	assert.Same(t, first, second)
	assert.Equal(t, logDir, first.dir)
	assert.Equal(t, int64(7<<20), first.maxBytes)
	assert.Equal(t, 3, first.maxFiles)
}

func TestFullContentWriterRotationKeepsConfiguredFileLimit(t *testing.T) {
	writer := &fullContentFileWriter{
		dir:      t.TempDir(),
		maxBytes: 1,
		maxFiles: 2,
	}

	for sequence := int64(1); sequence <= 4; sequence++ {
		require.NoError(t, writer.write(fullContentLogEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			RequestID: "rotation-test",
			Phase:     "response_chunk",
			Sequence:  sequence,
			Body:      "x",
		}))
	}

	files, err := filepath.Glob(filepath.Join(writer.dir, "full-content-*.jsonl"))
	require.NoError(t, err)
	assert.Len(t, files, 2)
}

func TestEncodeFullContentLogBodyUsesBase64ForBinaryData(t *testing.T) {
	body := []byte{0xff, 0x00, 0x01}
	encoded, encoding := encodeFullContentLogBody("application/octet-stream", body, false)

	assert.Equal(t, "base64", encoding)
	assert.Equal(t, base64.StdEncoding.EncodeToString(body), encoded)
}

func TestDeleteFullContentLogFileRotatesActiveWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logDir := t.TempDir()
	t.Setenv(fullContentLogEnabledEnv, "true")
	t.Setenv(fullContentLogDirEnv, logDir)

	engine := gin.New()
	engine.Use(BodyStorageCleanup(), FullContentLogger())
	engine.POST("/v1/responses", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"first"}`))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)

	files, err := filepath.Glob(filepath.Join(logDir, "full-content-*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	deletedBytes, err := DeleteFullContentLogFile(filepath.Base(files[0]))
	require.NoError(t, err)
	assert.Positive(t, deletedBytes)
	_, err = os.Stat(files[0])
	assert.ErrorIs(t, err, os.ErrNotExist)

	request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"second"}`))
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	files, err = filepath.Glob(filepath.Join(logDir, "full-content-*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
}

func TestFullContentLogFilePathRejectsTraversal(t *testing.T) {
	t.Setenv(fullContentLogDirEnv, t.TempDir())

	_, err := FullContentLogFilePath("../full-content-stolen.jsonl")
	assert.Error(t, err)
	_, err = FullContentLogFilePath("oneapi.log")
	assert.Error(t, err)
}
