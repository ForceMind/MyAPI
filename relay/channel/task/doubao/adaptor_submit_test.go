package doubao

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

type doubaoTrackedSubmitBody struct {
	reader io.Reader
	closed bool
}

func (body *doubaoTrackedSubmitBody) Read(data []byte) (int, error) { return body.reader.Read(data) }
func (body *doubaoTrackedSubmitBody) Close() error                  { body.closed = true; return nil }

type doubaoFailingSubmitBody struct{ closed bool }

func (*doubaoFailingSubmitBody) Read([]byte) (int, error) {
	return 0, errors.New("sk-sensitive-upstream-value")
}
func (body *doubaoFailingSubmitBody) Close() error { body.closed = true; return nil }

func doubaoTestRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
}

func TestDoubaoTaskSubmitParserAcceptsOnlySafeSuccess(t *testing.T) {
	body := []byte(`{"id":"doubao-task_1","prompt":"sensitive prompt","video_url":"https://sensitive.example/video","unknown":"discard"}`)
	input := channel.TaskSubmitParseInput{
		HTTPStatus:        http.StatusOK,
		Body:              body,
		UpstreamRequestID: "header-request-id",
		PublicTaskID:      "task_public_1",
		OriginModelName:   "doubao-origin",
		SubmittedAtUnix:   1_700_000_000,
	}

	first := (&TaskAdaptor{}).ParseTaskSubmitResponse(input)
	second := (&TaskAdaptor{}).ParseTaskSubmitResponse(input)

	require.NoError(t, first.Validate())
	require.Equal(t, first, second)
	require.Equal(t, channel.TaskSubmitAccepted, first.Disposition)
	require.Equal(t, "doubao-task_1", first.ProviderOperationID)
	require.Equal(t, "doubao-task_1", first.LegacyPollingID)
	require.Equal(t, "header-request-id", first.UpstreamRequestID)
	require.JSONEq(t, `{}`, string(first.TaskData))
	require.NotContains(t, string(first.TaskData), "sensitive")

	var legacy relaydto.OpenAIVideo
	require.NoError(t, common.Unmarshal(first.LegacyResponse.Body, &legacy))
	require.Equal(t, "task_public_1", legacy.ID)
	require.Equal(t, "task_public_1", legacy.TaskID)
	require.Equal(t, "doubao-origin", legacy.Model)
	require.EqualValues(t, 1_700_000_000, legacy.CreatedAt)
	require.NotContains(t, string(first.LegacyResponse.Body), "sensitive")
}

func TestDoubaoTaskSubmitParserClassifiesUnverifiedResponses(t *testing.T) {
	tests := []struct {
		name  string
		input channel.TaskSubmitParseInput
		code  string
	}{
		{"non 200", channel.TaskSubmitParseInput{HTTPStatus: http.StatusBadGateway, Body: []byte(`{"id":"doubao-task_1"}`)}, "unverified_response"},
		{"conflicting error", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"id":"doubao-task_1","error":{"message":"sk-sensitive-upstream-value"}}`)}, "unverified_response"},
		{"provider failure code", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"id":"doubao-task_1","code":"InvalidRequest"}`)}, "unverified_response"},
		{"null id", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"id":null}`)}, "invalid_response"},
		{"empty id", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"id":""}`)}, "invalid_response"},
		{"unsafe id", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"id":"doubao/task"}`)}, "invalid_response"},
		{"malformed body", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"id":`)}, "unmarshal_response_body_failed"},
		{"oversized body", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1))}, "invalid_response"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result := (&TaskAdaptor{}).ParseTaskSubmitResponse(testCase.input)
			require.NoError(t, result.Validate())
			require.Equal(t, channel.TaskSubmitUnknown, result.Disposition)
			require.Equal(t, testCase.code, result.Problem.OutcomeCode)
			require.NotContains(t, result.Problem.SafeMessage, "sk-sensitive-upstream-value")
			require.Nil(t, result.LegacyResponse)
			require.Empty(t, result.TaskData)
		})
	}
}

func TestDoubaoTaskIDSafetyAndFetchURL(t *testing.T) {
	uri, err := buildDoubaoTaskFetchURL("https://example.com", "doubao-task_1")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/api/v3/contents/generations/tasks/doubao-task_1", uri)

	for _, taskID := range []string{"", ".", "..", "doubao/task", "doubao?next=1", "doubao task", "任务-1"} {
		require.False(t, isSafeDoubaoTaskID(taskID), taskID)
		_, err := buildDoubaoTaskFetchURL("https://example.com", taskID)
		require.Error(t, err)
		_, err = (&TaskAdaptor{}).FetchTask("https://example.com", "unused", map[string]any{"task_id": taskID}, "")
		require.Error(t, err)
	}
}

func TestDoubaoDoResponsePreservesLegacyBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("success closes body and writes sanitized response", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		body := &doubaoTrackedSubmitBody{reader: strings.NewReader(`{"id":"doubao-task_1","prompt":"sensitive prompt"}`)}
		info := doubaoTestRelayInfo()
		info.PublicTaskID = "task_public_1"
		info.OriginModelName = "doubao-origin"

		taskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{StatusCode: http.StatusOK, Body: body}, info)

		require.Nil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "doubao-task_1", taskID)
		require.JSONEq(t, `{}`, string(taskData))
		require.NotContains(t, recorder.Body.String(), "sensitive")
	})

	t.Run("non 200 preserves legacy status without body leak", func(t *testing.T) {
		body := &doubaoTrackedSubmitBody{reader: strings.NewReader(`{"id":"doubao-task_1","message":"sk-sensitive-upstream-value"}`)}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusServiceUnavailable, Body: body}, nil)
		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "fail_to_fetch_task", taskErr.Code)
		require.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
	})

	t.Run("read failure closes body and redacts error", func(t *testing.T) {
		body := &doubaoFailingSubmitBody{}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusOK, Body: body}, nil)
		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "read_response_body_failed", taskErr.Code)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
	})

	t.Run("malformed response preserves legacy error code", func(t *testing.T) {
		body := &doubaoTrackedSubmitBody{reader: strings.NewReader(`{"id":`)}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusOK, Body: body}, nil)
		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "unmarshal_response_body_failed", taskErr.Code)
		require.Equal(t, http.StatusInternalServerError, taskErr.StatusCode)
	})

	t.Run("missing relay info is safe", func(t *testing.T) {
		body := &doubaoTrackedSubmitBody{reader: strings.NewReader(`{"id":"doubao-task_1"}`)}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusOK, Body: body}, nil)
		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "invalid_response", taskErr.Code)
	})
}
