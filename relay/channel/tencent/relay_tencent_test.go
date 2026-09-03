package tencent

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTencentHandlerReturnsConvertedResponseAndUsage(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	response := &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     http.Header{"X-Provider-Trace": []string{"trace-456"}},
		Body: io.NopCloser(strings.NewReader(`{
			"Response":{
				"Id":"tencent-response-id",
				"Choices":[{"FinishReason":"stop","Message":{"Role":"assistant","Content":"hello from Tencent"}}],
				"Usage":{"PromptTokens":7,"CompletionTokens":5,"TotalTokens":12}
			}
		}`)),
	}

	usage, apiErr := tencentHandler(context, nil, response)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 7, usage.PromptTokens)
	assert.Equal(t, 5, usage.CompletionTokens)
	assert.Equal(t, 12, usage.TotalTokens)
	assert.Equal(t, http.StatusAccepted, recorder.Code)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "trace-456", recorder.Header().Get("X-Provider-Trace"))

	var converted dto.OpenAITextResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &converted))
	assert.Equal(t, "tencent-response-id", converted.Id)
	assert.Equal(t, "chat.completion", converted.Object)
	require.Len(t, converted.Choices, 1)
	assert.Equal(t, "assistant", converted.Choices[0].Message.Role)
	assert.Equal(t, "hello from Tencent", converted.Choices[0].Message.StringContent())
	assert.Equal(t, "stop", converted.Choices[0].FinishReason)
	assert.Equal(t, *usage, converted.Usage)
}

func TestTencentHandlerReturnsProviderError(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	response := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body: io.NopCloser(strings.NewReader(`{
			"Response":{"Error":{"Code":4100,"Message":"provider validation failed"}}
		}`)),
	}

	usage, apiErr := tencentHandler(context, nil, response)
	assert.Nil(t, usage)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCode("4100"), apiErr.GetErrorCode())
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Equal(t, "provider validation failed", apiErr.Error())
	assert.Empty(t, recorder.Body.String())
}

func TestTencentHandlerRejectsMalformedResponse(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"Response":`)),
	}

	usage, apiErr := tencentHandler(context, nil, response)
	assert.Nil(t, usage)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeBadResponseBody, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
}
