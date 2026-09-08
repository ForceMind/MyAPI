package gemini

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/relay/channel"
	taskcommon "github.com/ForceMind/MyAPI/relay/channel/task/taskcommon"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relaydto "github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type geminiTrackedSubmitBody struct {
	reader io.Reader
	closed bool
}

func (body *geminiTrackedSubmitBody) Read(data []byte) (int, error) { return body.reader.Read(data) }
func (body *geminiTrackedSubmitBody) Close() error                  { body.closed = true; return nil }

type geminiFailingSubmitBody struct{ closed bool }

func (*geminiFailingSubmitBody) Read([]byte) (int, error) {
	return 0, errors.New("sk-sensitive-upstream-value")
}
func (body *geminiFailingSubmitBody) Close() error { body.closed = true; return nil }

func geminiTestRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
}

func TestGeminiTaskSubmitParserAcceptsOnlySafeSuccess(t *testing.T) {
	operationName := "models/veo-3.0-generate-001/operations/op_123"
	body := []byte(`{"name":"` + operationName + `","prompt":"sensitive prompt","video":"https://sensitive.example/video","unknown":"discard"}`)
	input := channel.TaskSubmitParseInput{
		HTTPStatus:        http.StatusOK,
		Body:              body,
		UpstreamRequestID: "header-request-id",
		PublicTaskID:      "task_public_1",
		OriginModelName:   "veo-origin",
		SubmittedAtUnix:   1_700_000_000,
	}

	first := (&TaskAdaptor{}).ParseTaskSubmitResponse(input)
	second := (&TaskAdaptor{}).ParseTaskSubmitResponse(input)

	require.NoError(t, first.Validate())
	require.Equal(t, first, second)
	require.Equal(t, channel.TaskSubmitAccepted, first.Disposition)
	require.Equal(t, operationName, first.ProviderOperationID)
	require.Equal(t, taskcommon.EncodeLocalTaskID(operationName), first.LegacyPollingID)
	require.Equal(t, "header-request-id", first.UpstreamRequestID)
	require.JSONEq(t, `{}`, string(first.TaskData))
	require.NotContains(t, string(first.TaskData), "sensitive")

	var legacy relaydto.OpenAIVideo
	require.NoError(t, common.Unmarshal(first.LegacyResponse.Body, &legacy))
	require.Equal(t, "task_public_1", legacy.ID)
	require.Equal(t, "task_public_1", legacy.TaskID)
	require.Equal(t, "veo-origin", legacy.Model)
	require.EqualValues(t, 1_700_000_000, legacy.CreatedAt)
	require.NotContains(t, string(first.LegacyResponse.Body), "sensitive")
}

func TestGeminiTaskSubmitParserClassifiesUnverifiedResponses(t *testing.T) {
	validName := "models/veo-3.0-generate-001/operations/op_123"
	tests := []struct {
		name  string
		input channel.TaskSubmitParseInput
		code  string
	}{
		{"non 200", channel.TaskSubmitParseInput{HTTPStatus: http.StatusBadGateway, Body: []byte(`{"name":"` + validName + `"}`)}, "unverified_response"},
		{"conflicting error", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"name":"` + validName + `","error":{"message":"sk-sensitive-upstream-value"}}`)}, "unverified_response"},
		{"null name", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"name":null}`)}, "invalid_response"},
		{"empty name", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"name":""}`)}, "invalid_response"},
		{"unsafe name", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"name":"models/veo/operations/op/extra"}`)}, "invalid_response"},
		{"malformed body", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"name":`)}, "unmarshal_response_failed"},
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

func TestGeminiTaskIDSafetyAndFetchURL(t *testing.T) {
	operationName := "models/veo-3.0-generate-001/operations/op_123"
	taskID := taskcommon.EncodeLocalTaskID(operationName)
	uri, err := buildGeminiTaskFetchURL("https://example.com", "v1beta", taskID)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/v1beta/"+operationName, uri)

	for _, unsafeName := range []string{"", "models/../operations/op", "models/veo/operations/op/extra", "models/veo/operations/op?next=1", "models/任务/operations/op"} {
		unsafeTaskID := taskcommon.EncodeLocalTaskID(unsafeName)
		require.False(t, isSafeGeminiOperationName(unsafeName), unsafeName)
		_, err := buildGeminiTaskFetchURL("https://example.com", "v1beta", unsafeTaskID)
		require.Error(t, err)
		_, err = (&TaskAdaptor{}).FetchTask("https://example.com", "unused", map[string]any{"task_id": unsafeTaskID}, "")
		require.Error(t, err)
	}
}

func TestGeminiDoResponsePreservesLegacyBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	operationName := "models/veo-3.0-generate-001/operations/op_123"
	t.Run("success closes body and writes sanitized response", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		body := &geminiTrackedSubmitBody{reader: strings.NewReader(`{"name":"` + operationName + `","prompt":"sensitive prompt"}`)}
		info := geminiTestRelayInfo()
		info.PublicTaskID = "task_public_1"
		info.OriginModelName = "veo-origin"

		taskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{StatusCode: http.StatusOK, Body: body}, info)

		require.Nil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, taskcommon.EncodeLocalTaskID(operationName), taskID)
		require.JSONEq(t, `{}`, string(taskData))
		require.NotContains(t, recorder.Body.String(), "sensitive")
	})

	t.Run("non 200 preserves legacy status without body leak", func(t *testing.T) {
		body := &geminiTrackedSubmitBody{reader: strings.NewReader(`{"name":"` + operationName + `","message":"sk-sensitive-upstream-value"}`)}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusServiceUnavailable, Body: body}, nil)
		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "fail_to_fetch_task", taskErr.Code)
		require.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
	})

	t.Run("read failure closes body and redacts error", func(t *testing.T) {
		body := &geminiFailingSubmitBody{}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusOK, Body: body}, nil)
		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "read_response_body_failed", taskErr.Code)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
	})

	t.Run("malformed response preserves legacy error code", func(t *testing.T) {
		body := &geminiTrackedSubmitBody{reader: strings.NewReader(`{"name":`)}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusOK, Body: body}, nil)
		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "unmarshal_response_failed", taskErr.Code)
		require.Equal(t, http.StatusInternalServerError, taskErr.StatusCode)
	})

	t.Run("missing relay info is safe", func(t *testing.T) {
		body := &geminiTrackedSubmitBody{reader: strings.NewReader(`{"name":"` + operationName + `"}`)}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusOK, Body: body}, nil)
		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "invalid_response", taskErr.Code)
	})
}
