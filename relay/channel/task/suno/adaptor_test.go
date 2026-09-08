package suno

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/relay/channel"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var _ channel.TaskSubmitResponseParser = (*TaskAdaptor)(nil)

type trackedSunoTaskSubmitBody struct {
	reader io.Reader
	closed bool
}

func (body *trackedSunoTaskSubmitBody) Read(data []byte) (int, error) {
	return body.reader.Read(data)
}

func (body *trackedSunoTaskSubmitBody) Close() error {
	body.closed = true
	return nil
}

type failingSunoTaskSubmitBody struct {
	closed bool
}

func (*failingSunoTaskSubmitBody) Read([]byte) (int, error) {
	return 0, errors.New("read failed with sk-sensitive-upstream-value")
}

func (body *failingSunoTaskSubmitBody) Close() error {
	body.closed = true
	return nil
}

func TestParseTaskSubmitResponseAcceptsStrictSunoSuccessAndRedactsUpstreamFields(t *testing.T) {
	input := channel.TaskSubmitParseInput{
		HTTPStatus:        http.StatusOK,
		UpstreamRequestID: "header-request-id",
		PublicTaskID:      "task_public_1",
		Body:              []byte(`{"code":"success","message":"sk-sensitive-upstream-value","data":"suno-upstream-1","prompt":"sensitive prompt","unknown":"discard"}`),
	}

	first := (&TaskAdaptor{}).ParseTaskSubmitResponse(input)
	second := (&TaskAdaptor{}).ParseTaskSubmitResponse(input)

	require.NoError(t, first.Validate())
	require.Equal(t, first, second)
	require.Equal(t, channel.TaskSubmitAccepted, first.Disposition)
	require.Equal(t, "suno-upstream-1", first.ProviderOperationID)
	require.Equal(t, "suno-upstream-1", first.LegacyPollingID)
	require.Equal(t, "header-request-id", first.UpstreamRequestID)
	require.JSONEq(t, `{"code":"success"}`, string(first.TaskData))
	require.NotContains(t, string(first.TaskData), "suno-upstream-1")
	require.NotContains(t, string(first.TaskData), "sensitive")
	require.JSONEq(t, `{"code":"success","message":"","data":"task_public_1"}`, string(first.LegacyResponse.Body))
	require.NotContains(t, string(first.LegacyResponse.Body), "sensitive")
}

func TestSunoTaskSubmitParserTreatsUnverifiedResponsesAsUnknown(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		input      channel.TaskSubmitParseInput
		code       string
		providerID string
	}{
		{"provider failure", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"code":"failed","data":"suno-upstream-1","message":"sk-sensitive-upstream-value"}`)}, "failed", "suno-upstream-1"},
		{"non 200", channel.TaskSubmitParseInput{HTTPStatus: http.StatusBadGateway, Body: []byte(`{"code":"success","data":"suno-upstream-1"}`)}, "unverified_response", "suno-upstream-1"},
		{"malformed", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"code":`)}, "unmarshal_response_body_failed", ""},
		{"null data", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"code":"success","data":null}`)}, "invalid_response", ""},
		{"empty data", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"code":"success","data":""}`)}, "invalid_response", ""},
		{"overlong data", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"code":"success","data":"` + strings.Repeat("a", channel.TaskSubmitProviderOperationIDMaxLength+1) + `"}`)}, "invalid_response", ""},
		{"unsafe data", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"code":"success","data":"suno/unsafe"}`)}, "invalid_response", ""},
		{"empty body", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK}, "invalid_response", ""},
		{"oversized body", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1))}, "invalid_response", ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result := (&TaskAdaptor{}).ParseTaskSubmitResponse(testCase.input)
			require.NoError(t, result.Validate())
			require.Equal(t, channel.TaskSubmitUnknown, result.Disposition)
			require.Equal(t, testCase.code, result.Problem.OutcomeCode)
			require.Equal(t, testCase.providerID, result.ProviderOperationID)
			require.Equal(t, testCase.providerID, result.LegacyPollingID)
			require.NotContains(t, result.Problem.SafeMessage, "sk-sensitive-upstream-value")
			require.Empty(t, result.TaskData)
			require.Nil(t, result.LegacyResponse)
		})
	}
}

func TestSunoFetchTaskRejectsUnsafePollingIDsBeforeRequest(t *testing.T) {
	for _, taskID := range []string{"", ".", "..", "suno/task", "suno?task", "suno task", "suno\ttask", "任务-1"} {
		_, err := (&TaskAdaptor{}).FetchTask("https://example.com", "unused", map[string]any{"ids": []string{taskID}}, "")
		require.Error(t, err, taskID)
	}
	_, err := (&TaskAdaptor{}).FetchTask("https://example.com", "unused", map[string]any{"ids": []any{"suno-upstream-1"}}, "")
	require.Error(t, err)
}

func TestSunoDoResponseUsesParserClosesBodyAndPreservesSafeErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("accepted response", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/suno/submit/music", nil)
		body := &trackedSunoTaskSubmitBody{reader: strings.NewReader(`{"code":"success","message":"sensitive prompt","data":"suno-upstream-1"}`)}
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
		info.PublicTaskID = "task_public_1"

		taskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{StatusCode: http.StatusOK, Body: body}, info)

		require.Nil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "suno-upstream-1", taskID)
		require.JSONEq(t, `{"code":"success"}`, string(taskData))
		require.JSONEq(t, `{"code":"success","message":"","data":"task_public_1"}`, recorder.Body.String())
		require.NotContains(t, recorder.Body.String(), "sensitive")
	})

	t.Run("provider failure preserves legacy code and status without leaking body", func(t *testing.T) {
		body := &trackedSunoTaskSubmitBody{reader: strings.NewReader(`{"code":"failed","message":"sk-sensitive-upstream-value"}`)}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusOK, Body: body}, nil)

		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "failed", taskErr.Code)
		require.Equal(t, http.StatusInternalServerError, taskErr.StatusCode)
		require.False(t, taskErr.LocalError)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
	})

	t.Run("non 200 preserves retry status without leaking body", func(t *testing.T) {
		body := &trackedSunoTaskSubmitBody{reader: strings.NewReader(`{"code":"success","data":"suno-upstream-1","message":"sk-sensitive-upstream-value"}`)}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusServiceUnavailable, Body: body}, nil)

		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "fail_to_fetch_task", taskErr.Code)
		require.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
	})

	t.Run("malformed response preserves legacy error mapping without leaking body", func(t *testing.T) {
		body := &trackedSunoTaskSubmitBody{reader: strings.NewReader(`{"code":"sk-sensitive-upstream-value"`)}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusOK, Body: body}, nil)

		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "unmarshal_response_body_failed", taskErr.Code)
		require.Equal(t, http.StatusInternalServerError, taskErr.StatusCode)
		require.False(t, taskErr.LocalError)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
	})

	t.Run("missing task relay info is safe", func(t *testing.T) {
		body := &trackedSunoTaskSubmitBody{reader: strings.NewReader(`{"code":"success","data":"suno-upstream-1"}`)}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusOK, Body: body}, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "invalid_response", taskErr.Code)
	})

	t.Run("read failure closes body and redacts error", func(t *testing.T) {
		body := &failingSunoTaskSubmitBody{}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusOK, Body: body}, nil)
		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "read_response_body_failed", taskErr.Code)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
	})
}
