package helper

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type geminiActionReadProbe struct {
	reader io.Reader
	reads  int
}

func (r *geminiActionReadProbe) Read(p []byte) (int, error) { r.reads++; return r.reader.Read(p) }
func (r *geminiActionReadProbe) Close() error               { return nil }

func TestUnsupportedGeminiActionCannotBecomeGeneration(t *testing.T) {
	for _, route := range []string{
		"/v1beta/models/gemini-fixture:countTokens",
		"/v1/models/gemini-fixture:countTokens?trace=fixture",
		"/v1beta/models/gemini-fixture:unknownOperation",
		"/v1beta/models/gemini-fixture:generateContent/extra",
		"/v1beta/models/gemini-fixture:embedContentExtra",
		"/v1beta/models/gemini-fixture:countTokens:generateContent",
		"/v1beta/models/gemini-fixture:",
	} {
		t.Run(route, func(t *testing.T) {
			body := &geminiActionReadProbe{reader: strings.NewReader(`{"contents":[{"parts":[{"text":"synthetic count only"}]}]}`)}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", route, body)
			ctx.Request.Header.Set("Content-Type", "application/json")
			request, err := GetAndValidateRequest(ctx, types.RelayFormatGemini)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsupported Gemini operation")
			assert.Nil(t, request)
			assert.Zero(t, body.reads, "reject the operation before decoding, estimating or reserving a generation request")
		})
	}
}

func TestExistingGeminiActionsAndLegacyDefaultRemainValid(t *testing.T) {
	for _, route := range []string{
		"/v1beta/models/gemini-fixture:generateContent",
		"/v1/models/gemini-fixture:streamGenerateContent?alt=sse",
		"/v1beta/models/imagen-fixture:predict",
		"/v1beta/models/gemini-fixture",
		"/api/channel/test",
	} {
		t.Run(route, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", route, strings.NewReader(`{"contents":[{"parts":[{"text":"synthetic"}]}]}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			request, err := GetAndValidateRequest(ctx, types.RelayFormatGemini)
			require.NoError(t, err)
			assert.IsType(t, &dto.GeminiChatRequest{}, request)
		})
	}
	for _, action := range []string{"embedContent", "batchEmbedContents"} {
		t.Run(action, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1beta/models/gemini-embedding-fixture:"+action, strings.NewReader(`{}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			request, err := GetAndValidateRequest(ctx, types.RelayFormatGemini)
			require.NoError(t, err)
			if action == "embedContent" {
				assert.IsType(t, &dto.GeminiEmbeddingRequest{}, request)
			} else {
				assert.IsType(t, &dto.GeminiBatchEmbeddingRequest{}, request)
			}
		})
	}
}
