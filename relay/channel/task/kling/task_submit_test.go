package kling

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/relay/channel"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relaydto "github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type klingTrackedResponseBody struct {
	reader io.Reader
	closed bool
}

func (body *klingTrackedResponseBody) Read(data []byte) (int, error) {
	return body.reader.Read(data)
}

func (body *klingTrackedResponseBody) Close() error {
	body.closed = true
	return nil
}

type klingFailingResponseBody struct {
	closed bool
}

func (*klingFailingResponseBody) Read([]byte) (int, error) {
	return 0, errors.New("read failed with sk-sensitive-upstream-value")
}

func (body *klingFailingResponseBody) Close() error {
	body.closed = true
	return nil
}

func klingTestRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}

func TestKlingTaskSubmitParserAcceptedIsPureAndDeterministic(t *testing.T) {
	adaptor := &TaskAdaptor{ChannelType: 17, apiKey: "fixture-key", baseURL: "https://fixture.example"}
	adaptorBefore := *adaptor
	body := []byte(`{"code":0,"message":"sk-sensitive-upstream-value","request_id":"body-request-id","unknown_top":"discard","data":{"task_id":"kling-upstream-1","task_status":"submitted","prompt":"sensitive prompt","task_result":{"videos":[{"url":"https://sensitive.example/video"}]},"unknown":"discard"}}`)
	bodyBefore := append([]byte(nil), body...)
	input := channel.TaskSubmitParseInput{
		HTTPStatus:        http.StatusOK,
		Body:              body,
		UpstreamRequestID: "header-request-id",
		PublicTaskID:      "task_public_1",
		OriginModelName:   "kling-origin",
		ClientModelName:   "kling-client",
		SubmittedAtUnix:   1_700_000_000,
	}

	first := adaptor.ParseTaskSubmitResponse(input)
	second := adaptor.ParseTaskSubmitResponse(input)

	require.NoError(t, first.Validate())
	require.Equal(t, first, second)
	require.Equal(t, adaptorBefore, *adaptor)
	require.Equal(t, bodyBefore, input.Body)
	require.Equal(t, channel.TaskSubmitAccepted, first.Disposition)
	require.Equal(t, "kling-upstream-1", first.ProviderOperationID)
	require.Equal(t, "kling-upstream-1", first.LegacyPollingID)
	require.Equal(t, "header-request-id", first.UpstreamRequestID)
	require.JSONEq(t, `{"code":0,"data":{"task_status":"submitted"}}`, string(first.TaskData))
	require.NotContains(t, string(first.TaskData), "sensitive prompt")
	require.NotContains(t, string(first.TaskData), "sensitive.example")
	require.NotContains(t, string(first.TaskData), "unknown")

	var persisted responsePayload
	require.NoError(t, common.Unmarshal(first.TaskData, &persisted))
	require.Zero(t, persisted.Code)
	require.Empty(t, persisted.Data.TaskId)
	require.Equal(t, "submitted", persisted.Data.TaskStatus)

	var legacy relaydto.OpenAIVideo
	require.NoError(t, common.Unmarshal(first.LegacyResponse.Body, &legacy))
	require.Equal(t, "task_public_1", legacy.ID)
	require.Equal(t, "task_public_1", legacy.TaskID)
	require.Equal(t, "kling-client", legacy.Model)
	require.Equal(t, relaydto.VideoStatusQueued, legacy.Status)
	require.EqualValues(t, 1_700_000_000, legacy.CreatedAt)
}

func TestKlingTaskSubmitParserClassifiesSafeNonAcceptedResults(t *testing.T) {
	const sensitiveValue = "sk-sensitive-upstream-value"
	adaptor := &TaskAdaptor{}
	tests := []struct {
		name           string
		status         int
		body           []byte
		headerID       string
		want           channel.TaskSubmitDisposition
		wantCode       string
		wantStatus     int
		wantProviderID string
		wantRequestID  string
	}{
		{
			name:       "provider error remains unknown until rejection contract is verified",
			status:     http.StatusOK,
			body:       []byte(`{"code":400,"message":"` + sensitiveValue + `"}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "task_failed",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:           "provider error with task id remains unknown",
			status:         http.StatusOK,
			body:           []byte(`{"code":400,"message":"` + sensitiveValue + `","data":{"task_id":"kling-conflict-1"}}`),
			headerID:       "header-request-id",
			want:           channel.TaskSubmitUnknown,
			wantCode:       "task_failed",
			wantStatus:     http.StatusBadRequest,
			wantProviderID: "kling-conflict-1",
			wantRequestID:  "header-request-id",
		},
		{
			name:       "null success code is unknown",
			status:     http.StatusOK,
			body:       []byte(`{"code":null,"data":{"task_id":"kling-null-code-1"}}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:           "provider error with top level task id remains unknown",
			status:         http.StatusOK,
			body:           []byte(`{"code":400,"task_id":"kling-top-level-1"}`),
			want:           channel.TaskSubmitUnknown,
			wantCode:       "task_failed",
			wantStatus:     http.StatusBadRequest,
			wantProviderID: "kling-top-level-1",
		},
		{
			name:       "provider error with malformed top level task id remains unknown",
			status:     http.StatusOK,
			body:       []byte(`{"code":400,"task_id":null}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "success with conflicting task ids is unknown",
			status:     http.StatusOK,
			body:       []byte(`{"code":0,"task_id":"kling-top-1","data":{"task_id":"kling-data-1"}}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "unverified_response",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:           "non OK success envelope remains unknown",
			status:         http.StatusAccepted,
			body:           []byte(`{"code":0,"request_id":"body-request-id","data":{"task_id":"kling-status-1"}}`),
			want:           channel.TaskSubmitUnknown,
			wantCode:       "unverified_response",
			wantStatus:     http.StatusBadGateway,
			wantProviderID: "kling-status-1",
			wantRequestID:  "body-request-id",
		},
		{
			name:       "malformed response",
			status:     http.StatusOK,
			body:       []byte(`{"code":0,"message":"` + sensitiveValue),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "unmarshal_response_body_failed",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "missing code",
			status:     http.StatusOK,
			body:       []byte(`{"data":{"task_id":"kling-upstream-1"}}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "invalid code type",
			status:     http.StatusOK,
			body:       []byte(`{"code":"0","data":{"task_id":"kling-upstream-1"}}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "empty task id",
			status:     http.StatusOK,
			body:       []byte(`{"code":0,"data":{"task_id":""}}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "unsafe task id",
			status:     http.StatusOK,
			body:       []byte(`{"code":0,"data":{"task_id":"kling/unsafe"}}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "malformed data shape",
			status:     http.StatusOK,
			body:       []byte(`{"code":0,"data":[]}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "oversized response",
			status:     http.StatusOK,
			body:       []byte(strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1)),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusBadGateway,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result := adaptor.ParseTaskSubmitResponse(channel.TaskSubmitParseInput{
				HTTPStatus:        testCase.status,
				Body:              testCase.body,
				UpstreamRequestID: testCase.headerID,
			})

			require.NoError(t, result.Validate())
			require.Equal(t, testCase.want, result.Disposition)
			require.Equal(t, testCase.wantCode, result.Problem.OutcomeCode)
			require.Equal(t, testCase.wantStatus, result.Problem.StatusCode)
			require.Equal(t, testCase.wantProviderID, result.ProviderOperationID)
			require.Equal(t, testCase.wantProviderID, result.LegacyPollingID)
			require.Equal(t, testCase.wantRequestID, result.UpstreamRequestID)
			require.NotContains(t, result.Problem.SafeMessage, sensitiveValue)
			require.Nil(t, result.LegacyResponse)
			require.Empty(t, result.TaskData)
		})
	}
}

func TestKlingTaskSubmitParserUsesSafeBodyRequestIDWhenHeaderIsInvalid(t *testing.T) {
	result := (&TaskAdaptor{}).ParseTaskSubmitResponse(channel.TaskSubmitParseInput{
		HTTPStatus:        http.StatusOK,
		UpstreamRequestID: "header request id",
		Body:              []byte(`{"code":0,"request_id":"body-request-id","data":{"task_id":"kling-upstream-1"}}`),
	})

	require.NoError(t, result.Validate())
	require.Equal(t, channel.TaskSubmitAccepted, result.Disposition)
	require.Equal(t, "body-request-id", result.UpstreamRequestID)
}

func TestKlingTaskIDUsesStrictASCIIPathSegment(t *testing.T) {
	for _, taskID := range []string{"kling-123", "KLING_123", "01234567-89ab-cdef-0123-456789abcdef"} {
		require.True(t, isSafeKlingTaskID(taskID), taskID)
		fetchURL, err := buildKlingTaskFetchURL("https://example.com", "/v1/videos/text2video", taskID, false)
		require.NoError(t, err)
		require.Equal(t, "https://example.com/v1/videos/text2video/"+taskID, fetchURL)
		newRelayURL, err := buildKlingTaskFetchURL("https://example.com", "/v1/videos/text2video", taskID, true)
		require.NoError(t, err)
		require.Equal(t, "https://example.com/kling/v1/videos/text2video/"+taskID, newRelayURL)
	}

	for _, taskID := range []string{
		"", ".", "..", "kling/task", "kling?task", "kling#task", "kling%2ftask", `kling\\task`,
		"kling task", "kling\ttask", "kling\x01task", "kling\u200dtask", "任务-1",
	} {
		require.False(t, isSafeKlingTaskID(taskID), taskID)
		_, err := buildKlingTaskFetchURL("https://example.com", "/v1/videos/text2video", taskID, false)
		require.Error(t, err)
		_, err = (&TaskAdaptor{}).FetchTask("https://example.com", "unused", map[string]any{"task_id": taskID, "action": "text"}, "")
		require.Error(t, err, "invalid IDs must fail before any HTTP request")
	}
}

func TestKlingDoResponsePreservesLegacySuccessAndClosesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	ctx.Set("model", "kling-client")
	responseBody := &klingTrackedResponseBody{reader: strings.NewReader(
		`{"code":0,"message":"sk-sensitive-upstream-value","data":{"task_id":"kling-upstream-1","task_status":"submitted","prompt":"sensitive prompt"}}`,
	)}
	info := klingTestRelayInfo()
	info.PublicTaskID = "task_public_1"
	info.OriginModelName = "kling-origin"

	taskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
		StatusCode: http.StatusOK,
		Body:       responseBody,
	}, info)

	require.Nil(t, taskErr)
	require.True(t, responseBody.closed)
	require.Equal(t, "kling-upstream-1", taskID)
	require.JSONEq(t, `{"code":0,"data":{"task_status":"submitted"}}`, string(taskData))
	require.NotContains(t, string(taskData), "sk-sensitive-upstream-value")
	require.NotContains(t, string(taskData), "sensitive prompt")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
	require.NotContains(t, recorder.Body.String(), "sk-sensitive-upstream-value")
	var legacy relaydto.OpenAIVideo
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &legacy))
	require.Equal(t, "task_public_1", legacy.ID)
	require.Equal(t, "task_public_1", legacy.TaskID)
	require.Equal(t, "kling-client", legacy.Model)
}

func TestKlingDoResponseReturnsSafeProblemAndAlwaysClosesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("provider error preserves legacy local contract without leaking body", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		responseBody := &klingTrackedResponseBody{reader: strings.NewReader(
			`{"code":400,"message":"sk-sensitive-upstream-value"}`,
		)}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody,
		}, klingTestRelayInfo())

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "task_failed", taskErr.Code)
		require.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		require.True(t, taskErr.LocalError)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
		require.Empty(t, recorder.Body.Bytes())
	})

	t.Run("non 200 preserves legacy status without leaking body", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		responseBody := &klingTrackedResponseBody{reader: strings.NewReader(
			`{"code":0,"data":{"task_id":"kling-upstream-1"},"message":"sk-sensitive-upstream-value"}`,
		)}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       responseBody,
		}, klingTestRelayInfo())

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "fail_to_fetch_task", taskErr.Code)
		require.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
		require.Empty(t, recorder.Body.Bytes())
	})

	t.Run("read failure", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		responseBody := &klingFailingResponseBody{}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody,
		}, klingTestRelayInfo())

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "read_response_body_failed", taskErr.Code)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
		require.Empty(t, recorder.Body.Bytes())
	})

	t.Run("malformed response", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		responseBody := &klingTrackedResponseBody{reader: strings.NewReader(`{"code":`)}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody,
		}, klingTestRelayInfo())

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "unmarshal_response_body_failed", taskErr.Code)
		require.Empty(t, recorder.Body.Bytes())
	})

	t.Run("oversized response", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		responseBody := &klingTrackedResponseBody{reader: strings.NewReader(strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1))}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody,
		}, klingTestRelayInfo())

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "invalid_response", taskErr.Code)
		require.Empty(t, recorder.Body.Bytes())
	})

	t.Run("nil response context", func(t *testing.T) {
		responseBody := &klingTrackedResponseBody{reader: strings.NewReader(
			`{"code":0,"data":{"task_id":"kling-upstream-1"}}`,
		)}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody,
		}, klingTestRelayInfo())

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "invalid_response", taskErr.Code)
	})

	t.Run("missing task relay info is safe and does not write", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		responseBody := &klingTrackedResponseBody{reader: strings.NewReader(
			`{"code":0,"data":{"task_id":"kling-upstream-1"}}`,
		)}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody,
		}, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "invalid_response", taskErr.Code)
		require.Empty(t, recorder.Body.Bytes())
	})
}
