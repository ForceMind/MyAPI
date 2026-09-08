package hailuo

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

var _ channel.TaskSubmitResponseParser = (*TaskAdaptor)(nil)

type trackedHailuoTaskSubmitResponseBody struct {
	reader io.Reader
	closed bool
}

func (body *trackedHailuoTaskSubmitResponseBody) Read(data []byte) (int, error) {
	return body.reader.Read(data)
}

func (body *trackedHailuoTaskSubmitResponseBody) Close() error {
	body.closed = true
	return nil
}

type failingHailuoTaskSubmitResponseBody struct {
	closed bool
}

func (*failingHailuoTaskSubmitResponseBody) Read([]byte) (int, error) {
	return 0, errors.New("read failed with sk-sensitive-upstream-value")
}

func (body *failingHailuoTaskSubmitResponseBody) Close() error {
	body.closed = true
	return nil
}

func TestParseTaskSubmitResponseAcceptedIsPureAndRedactsUpstreamFields(t *testing.T) {
	adaptor := &TaskAdaptor{ChannelType: 17, apiKey: "fixture-key", baseURL: "https://fixture.example"}
	adaptorBefore := *adaptor
	body := []byte(`{"task_id":"hailuo-upstream-1","base_resp":{"status_code":0,"status_msg":"sk-sensitive-upstream-value"},"prompt":"sensitive prompt","unknown":"discard"}`)
	bodyBefore := append([]byte(nil), body...)
	input := channel.TaskSubmitParseInput{
		HTTPStatus:        http.StatusOK,
		Body:              body,
		UpstreamRequestID: "header-request-id",
		PublicTaskID:      "task_public_1",
		OriginModelName:   "hailuo-origin",
		SubmittedAtUnix:   1_700_000_000,
	}

	first := adaptor.ParseTaskSubmitResponse(input)
	second := adaptor.ParseTaskSubmitResponse(input)

	require.NoError(t, first.Validate())
	require.Equal(t, first, second)
	require.Equal(t, adaptorBefore, *adaptor)
	require.Equal(t, bodyBefore, input.Body)
	require.Equal(t, channel.TaskSubmitAccepted, first.Disposition)
	require.Equal(t, "hailuo-upstream-1", first.ProviderOperationID)
	require.Equal(t, "hailuo-upstream-1", first.LegacyPollingID)
	require.Equal(t, "header-request-id", first.UpstreamRequestID)
	require.JSONEq(t, `{"base_resp":{"status_code":0}}`, string(first.TaskData))
	require.NotContains(t, string(first.TaskData), "hailuo-upstream-1")
	require.NotContains(t, string(first.TaskData), "sensitive prompt")
	require.NotContains(t, string(first.TaskData), "sk-sensitive-upstream-value")

	var legacy relaydto.OpenAIVideo
	require.NoError(t, common.Unmarshal(first.LegacyResponse.Body, &legacy))
	require.Equal(t, "task_public_1", legacy.ID)
	require.Equal(t, "task_public_1", legacy.TaskID)
	require.Equal(t, "hailuo-origin", legacy.Model)
	require.Equal(t, relaydto.VideoStatusQueued, legacy.Status)
	require.EqualValues(t, 1_700_000_000, legacy.CreatedAt)
	require.NotContains(t, string(first.LegacyResponse.Body), "hailuo-upstream-1")
	require.NotContains(t, string(first.LegacyResponse.Body), "sensitive prompt")
	require.NotContains(t, string(first.LegacyResponse.Body), "sk-sensitive-upstream-value")
}

func TestParseTaskSubmitResponseClassifiesOnlyVerifiedResponses(t *testing.T) {
	const sensitiveValue = "sk-sensitive-upstream-value"
	tests := []struct {
		name         string
		input        channel.TaskSubmitParseInput
		want         channel.TaskSubmitDisposition
		wantCode     string
		wantRequest  string
		wantProvider string
	}{
		{
			name: "provider error remains unknown until rejection contract is verified",
			input: channel.TaskSubmitParseInput{
				HTTPStatus: http.StatusOK,
				Body:       []byte(`{"base_resp":{"status_code":1008,"status_msg":"` + sensitiveValue + `"}}`),
			},
			want:     channel.TaskSubmitUnknown,
			wantCode: "1008",
		},
		{
			name: "provider error with task id remains unknown",
			input: channel.TaskSubmitParseInput{
				HTTPStatus: http.StatusOK,
				Body:       []byte(`{"task_id":"hailuo-conflict-1","base_resp":{"status_code":1008,"status_msg":"` + sensitiveValue + `"}}`),
			},
			want:         channel.TaskSubmitUnknown,
			wantCode:     "1008",
			wantProvider: "hailuo-conflict-1",
		},
		{
			name: "non 200 is unknown",
			input: channel.TaskSubmitParseInput{
				HTTPStatus: http.StatusBadGateway,
				Body:       []byte(`{"task_id":"hailuo-upstream-1","base_resp":{"status_code":0}}`),
			},
			want:         channel.TaskSubmitUnknown,
			wantCode:     "unverified_response",
			wantProvider: "hailuo-upstream-1",
		},
		{
			name: "array body is unknown",
			input: channel.TaskSubmitParseInput{
				HTTPStatus: http.StatusOK,
				Body:       []byte(`[]`),
			},
			want:     channel.TaskSubmitUnknown,
			wantCode: "unmarshal_response_body_failed",
		},
		{
			name: "malformed body is unknown",
			input: channel.TaskSubmitParseInput{
				HTTPStatus: http.StatusOK,
				Body:       []byte(`{"base_resp":`),
			},
			want:     channel.TaskSubmitUnknown,
			wantCode: "unmarshal_response_body_failed",
		},
		{
			name: "non utf8 body is unknown",
			input: channel.TaskSubmitParseInput{
				HTTPStatus: http.StatusOK,
				Body:       []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'},
			},
			want:     channel.TaskSubmitUnknown,
			wantCode: "invalid_response",
		},
		{
			name: "oversized body is unknown",
			input: channel.TaskSubmitParseInput{
				HTTPStatus: http.StatusOK,
				Body:       []byte(strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1)),
			},
			want:     channel.TaskSubmitUnknown,
			wantCode: "invalid_response",
		},
		{
			name: "missing task id is unknown",
			input: channel.TaskSubmitParseInput{
				HTTPStatus: http.StatusOK,
				Body:       []byte(`{"base_resp":{"status_code":0}}`),
			},
			want:     channel.TaskSubmitUnknown,
			wantCode: "invalid_response",
		},
		{
			name: "unsafe task id is unknown",
			input: channel.TaskSubmitParseInput{
				HTTPStatus: http.StatusOK,
				Body:       []byte(`{"task_id":"hailuo/unsafe","base_resp":{"status_code":0}}`),
			},
			want:     channel.TaskSubmitUnknown,
			wantCode: "invalid_response",
		},
		{
			name: "missing provider status is unknown",
			input: channel.TaskSubmitParseInput{
				HTTPStatus: http.StatusOK,
				Body:       []byte(`{"task_id":"hailuo-upstream-1","base_resp":{}}`),
			},
			want:     channel.TaskSubmitUnknown,
			wantCode: "invalid_response",
		},
		{
			name: "null provider status is unknown",
			input: channel.TaskSubmitParseInput{
				HTTPStatus: http.StatusOK,
				Body:       []byte(`{"task_id":"hailuo-upstream-1","base_resp":{"status_code":null}}`),
			},
			want:     channel.TaskSubmitUnknown,
			wantCode: "invalid_response",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result := (&TaskAdaptor{}).ParseTaskSubmitResponse(testCase.input)

			require.NoError(t, result.Validate())
			require.Equal(t, testCase.want, result.Disposition)
			require.Equal(t, testCase.wantCode, result.Problem.OutcomeCode)
			require.Equal(t, testCase.wantRequest, result.UpstreamRequestID)
			require.NotContains(t, result.Problem.SafeMessage, sensitiveValue)
			require.Equal(t, testCase.wantProvider, result.ProviderOperationID)
			require.Equal(t, testCase.wantProvider, result.LegacyPollingID)
			require.Empty(t, result.TaskData)
			require.Nil(t, result.LegacyResponse)
		})
	}
}

func TestHailuoTaskSubmitParserDropsUnsafeOptionalRequestID(t *testing.T) {
	result := (&TaskAdaptor{}).ParseTaskSubmitResponse(channel.TaskSubmitParseInput{
		HTTPStatus:        http.StatusOK,
		UpstreamRequestID: "unsafe header id",
		Body:              []byte(`{"task_id":"hailuo-upstream-1","base_resp":{"status_code":0}}`),
	})

	require.NoError(t, result.Validate())
	require.Equal(t, channel.TaskSubmitAccepted, result.Disposition)
	require.Empty(t, result.UpstreamRequestID)
}

func TestDoResponseUsesHailuoParserAndAlwaysClosesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("accepted response", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		responseBody := &trackedHailuoTaskSubmitResponseBody{reader: strings.NewReader(
			`{"task_id":"hailuo-upstream-1","base_resp":{"status_code":0,"status_msg":"sensitive message"}}`,
		)}
		info := &relaycommon.RelayInfo{
			ChannelMeta:     &relaycommon.ChannelMeta{},
			TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
			OriginModelName: "hailuo-origin",
		}
		info.PublicTaskID = "task_public_1"

		taskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody,
		}, info)

		require.Nil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "hailuo-upstream-1", taskID)
		require.JSONEq(t, `{"base_resp":{"status_code":0}}`, string(taskData))
		require.NotContains(t, string(taskData), "sensitive message")
		require.Equal(t, http.StatusOK, recorder.Code)
		var legacy relaydto.OpenAIVideo
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &legacy))
		require.Equal(t, "task_public_1", legacy.ID)
		require.Equal(t, "hailuo-origin", legacy.Model)
	})

	t.Run("provider error preserves legacy code and status without leaking body", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		responseBody := &trackedHailuoTaskSubmitResponseBody{reader: strings.NewReader(
			`{"base_resp":{"status_code":1008,"status_msg":"sk-sensitive-upstream-value"}}`,
		)}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody,
		}, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}})

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "1008", taskErr.Code)
		require.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		require.False(t, taskErr.LocalError)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
		require.Empty(t, recorder.Body.Bytes())
	})

	t.Run("non 200 preserves legacy status without leaking body", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		responseBody := &trackedHailuoTaskSubmitResponseBody{reader: strings.NewReader(
			`{"task_id":"hailuo-upstream-1","base_resp":{"status_code":0},"message":"sk-sensitive-upstream-value"}`,
		)}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       responseBody,
		}, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}})

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "fail_to_fetch_task", taskErr.Code)
		require.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
		require.Empty(t, recorder.Body.Bytes())
	})

	t.Run("missing task relay info is safe and does not write", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		responseBody := &trackedHailuoTaskSubmitResponseBody{reader: strings.NewReader(
			`{"task_id":"hailuo-upstream-1","base_resp":{"status_code":0}}`,
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

	t.Run("read failure", func(t *testing.T) {
		responseBody := &failingHailuoTaskSubmitResponseBody{}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody,
		}, nil)

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "read_response_body_failed", taskErr.Code)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
	})
}

func TestHailuoTaskIDSafetyAndFetchURL(t *testing.T) {
	uri, err := buildHailuoTaskFetchURL("https://example.com", "hailuo-task_1")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/v1/query/video_generation?task_id=hailuo-task_1", uri)

	for _, taskID := range []string{"", ".", "..", "hailuo/task", "hailuo?next=1", "hailuo task", "hailuo\ttask", "hailuo\x01task", "hailuo\u200dtask", "任务-1"} {
		require.False(t, isSafeHailuoTaskID(taskID), taskID)
		_, err := buildHailuoTaskFetchURL("https://example.com", taskID)
		require.Error(t, err)
		_, err = (&TaskAdaptor{}).FetchTask("https://example.com", "unused", map[string]any{"task_id": taskID}, "")
		require.Error(t, err, "invalid IDs must fail before any HTTP request")
	}
}
