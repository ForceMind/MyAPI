package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type videoAdapterObservation struct {
	directBody []byte
	replayBody []byte
	request    relaycommon.TaskSubmitReq
	err        error
}

func TestKlingRequestConvertBuildsReplayableUnifiedEnvelope(t *testing.T) {
	originalMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	defer gin.SetMode(originalMode)

	originalBody := `{
		"model":"fallback-model",
		"model_name":"preferred-model",
		"prompt":"top prompt",
		"mode":"pro",
		"image":"top-image",
		"images":["first","second"],
		"size":"1280x720",
		"duration":0,
		"seconds":"0",
		"input_reference":"reference-id",
		"metadata":{
			"model":"metadata-model",
			"model_name":"metadata-model-name",
			"req_key":"metadata-req-key",
			"prompt":"metadata prompt",
			"mode":"metadata-mode",
			"provider_only":"metadata-value",
			"zero_value":7,
			"false_value":true,
			"array_value":["metadata"],
			"nested":{"source":"metadata"}
		},
		"provider_only":"top-value",
		"zero_value":0,
		"false_value":false,
		"array_value":[],
		"nested":{"source":"top"}
	}`

	observation, recorder := exerciseVideoAdapter(t, http.MethodPost, "/kling/v1/videos/text2video", originalBody, KlingRequestConvert())
	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.NoError(t, observation.err)

	assertUnifiedAdapterEnvelope(t, observation, map[string]any{
		"model":           "preferred-model",
		"prompt":          "top prompt",
		"mode":            "pro",
		"image":           "top-image",
		"images":          []any{"first", "second"},
		"size":            "1280x720",
		"duration":        float64(0),
		"seconds":         "0",
		"input_reference": "reference-id",
	}, map[string]any{
		"provider_only": "top-value",
		"zero_value":    float64(0),
		"false_value":   false,
		"array_value":   []any{},
		"nested":        map[string]any{"source": "top"},
	})
}

func TestJimengRequestConvertBuildsReplayableUnifiedEnvelope(t *testing.T) {
	originalMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	defer gin.SetMode(originalMode)

	originalBody := `{
		"req_key":"jimeng-client-model",
		"model":"must-not-win",
		"prompt":"top prompt",
		"duration":5,
		"frames":241,
		"images":["image-a"],
		"metadata":{
			"req_key":"metadata-req-key",
			"model":"metadata-model",
			"model_name":"metadata-model-name",
			"duration":5,
			"provider_only":"metadata-value",
			"false_value":true
		},
		"provider_only":"top-value",
		"false_value":false,
		"array_value":[],
		"nested":{"enabled":false}
	}`

	observation, recorder := exerciseVideoAdapter(t, http.MethodPost, "/jimeng/?Action=CVSync2AsyncSubmitTask", originalBody, JimengRequestConvert())
	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.NoError(t, observation.err)

	assertUnifiedAdapterEnvelope(t, observation, map[string]any{
		"model":    "jimeng-client-model",
		"prompt":   "top prompt",
		"duration": float64(10),
		"images":   []any{"image-a"},
	}, map[string]any{
		"provider_only": "top-value",
		"false_value":   false,
		"array_value":   []any{},
		"nested":        map[string]any{"enabled": false},
	})
	metadata := observation.request.Metadata
	assert.NotContains(t, metadata, "frames")
}

func TestVideoRequestConvertPreservesExistingErrorBranches(t *testing.T) {
	originalMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	defer gin.SetMode(originalMode)

	t.Run("Kling invalid body continues", func(t *testing.T) {
		called := false
		engine := gin.New()
		engine.Use(BodyStorageCleanup())
		engine.POST("/kling/v1/videos/text2video", KlingRequestConvert(), func(c *gin.Context) {
			called = true
			c.Status(http.StatusNoContent)
		})

		request := httptest.NewRequest(http.MethodPost, "/kling/v1/videos/text2video", strings.NewReader(`{"prompt":`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)

		assert.True(t, called)
		assert.Equal(t, http.StatusNoContent, recorder.Code)
	})

	for _, testCase := range []struct {
		name   string
		target string
		body   string
	}{
		{name: "Jimeng missing action aborts", target: "/jimeng/", body: `{"req_key":"model","prompt":"hello"}`},
		{name: "Jimeng invalid body aborts", target: "/jimeng/?Action=CVSync2AsyncSubmitTask", body: `{"prompt":`},
		{name: "Jimeng invalid frames aborts", target: "/jimeng/?Action=CVSync2AsyncSubmitTask", body: `{"req_key":"model","prompt":"hello","frames":242}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			called := false
			engine := gin.New()
			engine.Use(BodyStorageCleanup())
			engine.POST("/jimeng/", JimengRequestConvert(), func(c *gin.Context) {
				called = true
				c.Status(http.StatusNoContent)
			})

			request := httptest.NewRequest(http.MethodPost, testCase.target, strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)

			assert.False(t, called)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
		})
	}
}

func TestFullContentLoggerCapturesOriginalJimengRequestBeforeConversion(t *testing.T) {
	originalMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	defer gin.SetMode(originalMode)

	logDir := t.TempDir()
	t.Setenv(fullContentLogEnabledEnv, "true")
	t.Setenv(fullContentLogDirEnv, logDir)
	t.Setenv(fullContentLogMaxMBEnv, "1")
	t.Setenv(fullContentLogMaxFilesEnv, "0")
	writerConfig := fullContentFileWriterConfig{dir: logDir, maxBytes: 1 << 20, maxFiles: 0}
	t.Cleanup(func() {
		fullContentLogLifecycleMu.Lock()
		defer fullContentLogLifecycleMu.Unlock()
		value, exists := fullContentLogWriterPool.LoadAndDelete(writerConfig)
		if !exists {
			return
		}
		writer := value.(*fullContentFileWriter)
		writer.mu.Lock()
		if writer.file != nil {
			_ = writer.file.Close()
			writer.file = nil
			writer.size = 0
		}
		writer.mu.Unlock()
		fullContentLogWriters.Delete(writer)
	})

	originalBody := `{"req_key":"jimeng-client-model","prompt":"raw client prompt","task_id":"task-42"}`
	var downstreamBody []byte
	var downstreamMethod string
	var downstreamPath string
	engine := gin.New()
	engine.Use(BodyStorageCleanup())
	engine.POST("/jimeng/",
		func(c *gin.Context) {
			c.Set(common.RequestIdKey, "jimeng-log-test")
			c.Set("id", 17)
			c.Set("token_id", 23)
			c.Set("token_name", "fixture-token")
			c.Next()
		},
		FullContentLogger(),
		JimengRequestConvert(),
		func(c *gin.Context) {
			var err error
			downstreamBody, err = io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			downstreamMethod = c.Request.Method
			downstreamPath = c.Request.URL.Path
			c.Set(common.RequestIdKey, "mutated-request-id")
			c.Set("id", 99)
			c.Set("token_id", 98)
			c.Set("token_name", "mutated-token")
			_, err = c.Writer.WriteString("done")
			require.NoError(t, err)
		},
	)

	request := httptest.NewRequest(http.MethodPost, "/jimeng/?Action=CVSync2AsyncGetResult", strings.NewReader(originalBody))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, http.MethodGet, downstreamMethod)
	assert.Equal(t, "/v1/video/generations/task-42", downstreamPath)
	var downstreamEnvelope map[string]any
	require.NoError(t, common.Unmarshal(downstreamBody, &downstreamEnvelope))
	assert.Equal(t, "jimeng-client-model", downstreamEnvelope["model"])
	metadata, ok := downstreamEnvelope["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "task-42", metadata["task_id"])

	files, err := filepath.Glob(filepath.Join(logDir, "full-content-*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	logBytes, err := os.ReadFile(files[0])
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
	require.Len(t, lines, 3)
	for index, line := range lines {
		var entry fullContentLogEntry
		require.NoError(t, common.Unmarshal([]byte(line), &entry))
		assert.Equal(t, http.MethodPost, entry.Method)
		assert.Equal(t, "/jimeng/", entry.Path)
		assert.Equal(t, "jimeng-log-test", entry.RequestID)
		assert.Equal(t, 17, entry.UserID)
		assert.Equal(t, 23, entry.TokenID)
		assert.Equal(t, "fixture-token", entry.TokenName)
		if index == 0 {
			assert.Equal(t, "request", entry.Phase)
			assert.JSONEq(t, originalBody, entry.Body)
		}
	}
}

func exerciseVideoAdapter(t *testing.T, method, target, body string, adapter gin.HandlerFunc) (videoAdapterObservation, *httptest.ResponseRecorder) {
	t.Helper()
	observation := videoAdapterObservation{}
	engine := gin.New()
	engine.Use(BodyStorageCleanup())
	engine.Handle(method, strings.Split(target, "?")[0], adapter, func(c *gin.Context) {
		if c.Request.GetBody == nil {
			observation.err = errors.New("request GetBody is nil")
			return
		}
		replay, err := c.Request.GetBody()
		if err != nil {
			observation.err = err
			return
		}
		observation.replayBody, err = io.ReadAll(replay)
		if closeErr := replay.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			observation.err = err
			return
		}

		observation.directBody, err = io.ReadAll(c.Request.Body)
		if err != nil {
			observation.err = err
			return
		}
		if err := common.UnmarshalBodyReusable(c, &observation.request); err != nil {
			observation.err = err
			return
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return observation, recorder
}

func assertUnifiedAdapterEnvelope(t *testing.T, observation videoAdapterObservation, expectedTopLevel, expectedMetadata map[string]any) {
	t.Helper()
	assert.Equal(t, observation.replayBody, observation.directBody)

	var envelope map[string]any
	require.NoError(t, common.Unmarshal(observation.directBody, &envelope))
	metadata, ok := envelope["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, expectedMetadata, metadata)
	for key, value := range expectedTopLevel {
		assert.Equal(t, value, envelope[key], key)
	}
	for _, key := range []string{"model", "prompt", "mode", "image", "images", "size", "duration", "seconds", "input_reference", "model_name", "req_key"} {
		assert.NotContains(t, metadata, key)
	}
	assert.Equal(t, expectedTopLevel["model"], observation.request.Model)
	assert.Equal(t, expectedTopLevel["prompt"], observation.request.Prompt)
}
