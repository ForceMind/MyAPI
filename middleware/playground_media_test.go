package middleware

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func playgroundImageRequest(t *testing.T, images []string) []byte {
	t.Helper()
	parts := make([]map[string]any, 0, len(images))
	for _, image := range images {
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": image}})
	}
	body, err := common.Marshal(map[string]any{"model": "synthetic-vision", "messages": []any{map[string]any{"role": "user", "content": parts}}})
	require.NoError(t, err)
	return body
}

func TestPlaygroundMediaGuardPreservesInlineImageAndRejectsInvalidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	image := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII="
	cases := []struct {
		name        string
		body        []byte
		contentType string
		want        int
	}{
		{"image only", playgroundImageRequest(t, []string{image}), "application/json", 200},
		{"four images", playgroundImageRequest(t, []string{image, image, image, image}), "application/json", 200},
		{"too many images", playgroundImageRequest(t, []string{image, image, image, image, image}), "application/json", 400},
		{"remote image denied", playgroundImageRequest(t, []string{"https://example.invalid/private.png"}), "application/json", 400},
		{"mime spoof rejected", playgroundImageRequest(t, []string{strings.Replace(image, "image/png", "image/jpeg", 1)}), "application/json", 400},
		{"bad base64", playgroundImageRequest(t, []string{"data:image/png;base64,!!!!"}), "application/json", 400},
		{"svg rejected", playgroundImageRequest(t, []string{"data:image/svg+xml;base64,PHN2Zy8+"}), "application/json", 400},
		{"foreign opaque file not forwarded", []byte(`{"messages":[{"role":"user","content":[{"type":"file","file":{"file_id":"file_other_account"}}]}]}`), "application/json", 400},
		{"inline PDF", []byte(`{"messages":[{"role":"user","content":[{"type":"file","file":{"filename":"notes.pdf","file_data":"data:application/pdf;base64,JVBERi0xLjQK"}}]}]}`), "application/json", 200},
		{"PDF filename traversal", []byte(`{"messages":[{"role":"user","content":[{"type":"file","file":{"filename":"../notes.pdf","file_data":"data:application/pdf;base64,JVBERi0xLjQK"}}]}]}`), "application/json", 400},
		{"non PDF file", []byte(`{"messages":[{"role":"user","content":[{"type":"file","file":{"filename":"notes.txt","file_data":"data:text/plain;base64,aGVsbG8="}}]}]}`), "application/json", 400},
		{"text remains supported", []byte(`{"model":"synthetic","messages":[{"role":"user","content":"hello"}]}`), "application/json", 200},
		{"malformed json", []byte(`{"messages":`), "application/json", 400},
		{"wrong content type", []byte(`{}`), "text/plain", 415},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(BodyStorageCleanup(), PlaygroundMediaGuard())
			calls := 0
			engine.POST("/pg/chat/completions", func(c *gin.Context) {
				calls++
				actual, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				assert.Equal(t, tc.body, actual)
				c.Status(200)
			})
			request := httptest.NewRequest(http.MethodPost, "/pg/chat/completions", bytes.NewReader(tc.body))
			request.Header.Set("Content-Type", tc.contentType)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			require.Equal(t, tc.want, recorder.Code)
			if tc.want == 200 {
				assert.Equal(t, 1, calls)
			} else {
				assert.Zero(t, calls)
			}
		})
	}
}

func TestPlaygroundMediaGuardRejectsAmbiguousOrOpaqueMedia(t *testing.T) {
	for _, raw := range []string{
		`{"model":"unapproved","Model":"approved","max_tokens":4294967295,"Max_Tokens":10,"messages":[{"role":"user","content":"hello"}]}`,
		`{"messages":[{"role":"user","content":"hello"}],"meſſages":[{"role":"user","content":"hidden"}]}`,

		`{"messages":[{"role":"user","content":[{"type":"image_url","Type":"text","image_url":{"url":"https://example.invalid/private.png"}}]}]}`,
		`{"messages":[{"role":"user","content":[{"type":"file","Type":"text","file":{"file_id":"file_other_account"}}]}]}`,
		`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.invalid/private.png","URL":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII="}}]}]}`,
		`{"messages":[{"role":"user","content":[{"type":"file","file":{"file_id":"","filename":"notes.pdf","file_data":"data:application/pdf;base64,JVBERi0xLjQK"}}]}]}`,
		`{"messages":[{"role":"user","content":[{"type":"image_url","type":"text","image_url":{"url":"https://example.invalid/private.png"}}]}]}`,
	} {
		t.Run(raw, func(t *testing.T) { assert.False(t, validPlaygroundMedia([]byte(raw))) })
	}
}

func TestPlaygroundMediaGuardBoundsDecodedImageAndUnknownLengthBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	data := append([]byte("GIF89a"), bytes.Repeat([]byte{0}, playgroundMaxImageBytes)...)
	body := playgroundImageRequest(t, []string{"data:image/gif;base64," + base64.StdEncoding.EncodeToString(data)})
	assert.False(t, validPlaygroundMedia(body))
	engine := gin.New()
	engine.Use(BodyStorageCleanup(), PlaygroundMediaGuard())
	engine.POST("/pg/chat/completions", func(c *gin.Context) { t.Error("oversize body reached handler") })
	request := httptest.NewRequest(http.MethodPost, "/pg/chat/completions", io.LimitReader(strings.NewReader(strings.Repeat(" ", playgroundMaxBodyBytes+1)), playgroundMaxBodyBytes+1))
	request.ContentLength = -1
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}

func TestPlaygroundContentLogKeepsIdentityWithoutMediaOrResponseBytes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	t.Setenv(fullContentLogEnabledEnv, "true")
	t.Setenv(fullContentLogDirEnv, dir)
	engine := gin.New()
	engine.Use(BodyStorageCleanup())
	engine.Use(func(c *gin.Context) {
		c.Set("playground_original_path", "/pg/chat/completions")
		c.Request.URL.Path = "/v1/chat/completions"
		c.Set("id", 7)
		c.Set("token_id", 19)
		c.Next()
	}, FullContentLogger())
	engine.POST("/pg/chat/completions", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "private-media")
		c.Header("Content-Type", "text/event-stream")
		_, err = c.Writer.WriteString("data: private-response\n\n")
		require.NoError(t, err)
	})
	request := httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(`{"messages":[{"content":"private-media"}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer session-secret")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, 200, recorder.Code)
	files, err := filepath.Glob(filepath.Join(dir, "full-content-*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	logged, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.NotContains(t, string(logged), "private-media")
	assert.NotContains(t, string(logged), "private-response")
	assert.NotContains(t, string(logged), "session-secret")
	assert.Contains(t, string(logged), `"path":"/pg/chat/completions"`)
	assert.Contains(t, string(logged), `"token_id":19`)
	assert.Contains(t, string(logged), "omitted_playground_content")
}

func TestPlaygroundMediaRefusesNonUserRolesInsteadOfDroppingAttachments(t *testing.T) {
	image := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII="
	body := playgroundImageRequest(t, []string{image})
	require.True(t, validPlaygroundMedia(body))
	for _, role := range []string{"system", "developer", "assistant", "tool", "function"} {
		assert.False(t, validPlaygroundMedia(bytes.Replace(body, []byte(`"role":"user"`), []byte(`"role":"`+role+`"`), 1)))
	}
	assert.False(t, validPlaygroundMedia(bytes.Replace(body, []byte(`"role":"user"`), []byte(`"role":"user","Role":"system"`), 1)))
}
