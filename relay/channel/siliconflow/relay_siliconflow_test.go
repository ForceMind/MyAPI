package siliconflow

import (
	"errors"
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

func TestSiliconflowRerankHandlerReturnsUsageBodyAndStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	body := &siliconflowTestReadCloser{Reader: strings.NewReader(`{
		"results":[{"document":{"text":"matched document"},"index":2,"relevance_score":0.875}],
		"meta":{"tokens":{"input_tokens":11,"output_tokens":3}}
	}`)}
	response := &http.Response{
		StatusCode: http.StatusCreated,
		Header:     http.Header{"X-Provider-Trace": []string{"trace-123"}},
		Body:       body,
	}

	usage, apiErr := siliconflowRerankHandler(context, nil, response)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 11, usage.PromptTokens)
	assert.Equal(t, 3, usage.CompletionTokens)
	assert.Equal(t, 14, usage.TotalTokens)
	assert.True(t, body.closed)
	assert.Equal(t, http.StatusCreated, recorder.Code)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "trace-123", recorder.Header().Get("X-Provider-Trace"))

	var converted dto.RerankResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &converted))
	require.Len(t, converted.Results, 1)
	assert.Equal(t, 2, converted.Results[0].Index)
	assert.Equal(t, 0.875, converted.Results[0].RelevanceScore)
	assert.Equal(t, map[string]any{"text": "matched document"}, converted.Results[0].Document)
	assert.Equal(t, *usage, converted.Usage)
}

func TestSiliconflowRerankHandlerRejectsReaderAndMalformedResponses(t *testing.T) {
	t.Run("reader error", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		response := &http.Response{StatusCode: http.StatusOK, Body: &siliconflowTestReadCloser{Reader: siliconflowErrorReader{}}}

		usage, apiErr := siliconflowRerankHandler(context, nil, response)
		assert.Nil(t, usage)
		require.NotNil(t, apiErr)
		assert.Equal(t, types.ErrorCodeReadResponseBodyFailed, apiErr.GetErrorCode())
		assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		body := &siliconflowTestReadCloser{Reader: strings.NewReader(`{"results":`)}
		response := &http.Response{StatusCode: http.StatusOK, Body: body}

		usage, apiErr := siliconflowRerankHandler(context, nil, response)
		assert.Nil(t, usage)
		require.NotNil(t, apiErr)
		assert.Equal(t, types.ErrorCodeBadResponseBody, apiErr.GetErrorCode())
		assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
		assert.True(t, body.closed)
	})
}

func TestSiliconflowRerankHandlerPreservesStructuredProviderErrorZeroValueBehavior(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	response := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"code":30001,"message":"provider rejected request"}`)),
	}

	usage, apiErr := siliconflowRerankHandler(context, nil, response)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, dto.Usage{}, *usage)
	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	var converted dto.RerankResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &converted))
	assert.Empty(t, converted.Results)
	assert.Equal(t, dto.Usage{}, converted.Usage)
}

type siliconflowTestReadCloser struct {
	io.Reader
	closed bool
}

func (r *siliconflowTestReadCloser) Close() error {
	r.closed = true
	return nil
}

type siliconflowErrorReader struct{}

func (siliconflowErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("synthetic upstream read failure")
}
