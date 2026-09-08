package sora

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/relay/channel"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ channel.TaskSubmitResponseParser = (*TaskAdaptor)(nil)

type trackedSoraTaskSubmitBody struct {
	reader io.Reader
	closed bool
}

func (body *trackedSoraTaskSubmitBody) Read(data []byte) (int, error) {
	return body.reader.Read(data)
}

func (body *trackedSoraTaskSubmitBody) Close() error {
	body.closed = true
	return nil
}

type failingSoraTaskSubmitBody struct {
	closed bool
}

func (*failingSoraTaskSubmitBody) Read([]byte) (int, error) {
	return 0, errors.New("read failed with sk-sensitive-upstream-value")
}

func (body *failingSoraTaskSubmitBody) Close() error {
	body.closed = true
	return nil
}

func TestSoraBuildRequestBodyReturnsReplayablePassThroughBody(t *testing.T) {
	payload := []byte("opaque-sora-request-body")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/octet-stream")
	defer common.CleanupBodyStorage(c)

	info := &relaycommon.RelayInfo{}
	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	replayable, ok := body.(common.ReplayableBody)
	require.True(t, ok)

	sent, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, payload, sent)
	assert.EqualValues(t, len(payload), replayable.Size())

	replayBody, err := replayable.NewReader()
	require.NoError(t, err)
	replay, err := io.ReadAll(replayBody)
	require.NoError(t, err)
	require.NoError(t, replayBody.Close())
	assert.Equal(t, payload, replay)
}

func TestParseTaskSubmitResponseAcceptsStrictSoraSuccessAndRedactsUpstreamFields(t *testing.T) {
	input := channel.TaskSubmitParseInput{
		HTTPStatus:         http.StatusOK,
		UpstreamRequestID:  "header-request-id",
		PublicTaskID:       "task_public_1",
		OriginPublicTaskID: "task_origin_1",
		Body:               []byte(`{"id":"sora-upstream-1","task_id":"sora-upstream-1","object":"video","model":"sora-2","status":"queued","progress":0,"created_at":1700000000,"remixed_from_video_id":"sora-upstream-origin","prompt":"sensitive prompt","media_url":"https://sensitive.example"}`),
	}

	first := (&TaskAdaptor{}).ParseTaskSubmitResponse(input)
	second := (&TaskAdaptor{}).ParseTaskSubmitResponse(input)

	require.NoError(t, first.Validate())
	require.Equal(t, first, second)
	require.Equal(t, channel.TaskSubmitAccepted, first.Disposition)
	require.Equal(t, "sora-upstream-1", first.ProviderOperationID)
	require.Equal(t, "sora-upstream-1", first.LegacyPollingID)
	require.Equal(t, "header-request-id", first.UpstreamRequestID)
	require.JSONEq(t, `{"object":"video","model":"sora-2","status":"queued","progress":0,"created_at":1700000000,"remixed_from_video_id":"task_origin_1"}`, string(first.TaskData))
	require.NotContains(t, string(first.TaskData), "sora-upstream-1")
	require.NotContains(t, string(first.TaskData), "sora-upstream-origin")
	require.NotContains(t, string(first.TaskData), "sensitive")

	var legacy responseTask
	require.NoError(t, common.Unmarshal(first.LegacyResponse.Body, &legacy))
	require.Equal(t, "task_public_1", legacy.ID)
	require.Equal(t, "task_public_1", legacy.TaskID)
	require.Equal(t, "task_origin_1", legacy.RemixedFromVideoID)
	require.Empty(t, legacy.Error)
	require.NotContains(t, string(first.LegacyResponse.Body), "sora-upstream-1")
	require.NotContains(t, string(first.LegacyResponse.Body), "sora-upstream-origin")
	require.NotContains(t, string(first.LegacyResponse.Body), "sensitive")
}

func TestSoraTaskSubmitParserTreatsUnverifiedResponsesAsUnknown(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		input      channel.TaskSubmitParseInput
		code       string
		providerID string
	}{
		{"non 200", channel.TaskSubmitParseInput{HTTPStatus: http.StatusBadGateway, Body: []byte(`{"id":"sora-upstream-1","status":"queued"}`)}, "unverified_response", "sora-upstream-1"},
		{"top level error", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"id":"sora-upstream-1","status":"queued","error":{"message":"sk-sensitive-upstream-value"}}`)}, "unverified_response", "sora-upstream-1"},
		{"conflicting identifiers", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"id":"sora-upstream-1","task_id":"sora-upstream-2","status":"queued"}`)}, "unverified_response", ""},
		{"malformed", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"id":`)}, "unmarshal_response_body_failed", ""},
		{"null id", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"id":null,"status":"queued"}`)}, "invalid_response", ""},
		{"empty id", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"id":"","status":"queued"}`)}, "invalid_response", ""},
		{"overlong id", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"id":"` + strings.Repeat("a", channel.TaskSubmitProviderOperationIDMaxLength+1) + `","status":"queued"}`)}, "invalid_response", ""},
		{"unsafe id", channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"id":"sora/unsafe","status":"queued"}`)}, "invalid_response", ""},
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
			require.Empty(t, result.TaskData)
			require.Nil(t, result.LegacyResponse)
		})
	}
}

func TestSoraTaskSubmitParserDoesNotUseUnsafeOriginID(t *testing.T) {
	result := (&TaskAdaptor{}).ParseTaskSubmitResponse(channel.TaskSubmitParseInput{
		HTTPStatus:         http.StatusOK,
		PublicTaskID:       "task_public_1",
		OriginPublicTaskID: "sora-upstream-origin",
		Body:               []byte(`{"id":"sora-upstream-1","status":"queued","remixed_from_video_id":"another-upstream-origin"}`),
	})

	require.NoError(t, result.Validate())
	require.Equal(t, channel.TaskSubmitAccepted, result.Disposition)
	require.NotContains(t, string(result.TaskData), "origin")
	require.NotContains(t, string(result.LegacyResponse.Body), "origin")
}

func TestSoraFetchTaskRejectsUnsafePollingIDBeforeRequest(t *testing.T) {
	for _, taskID := range []string{"", ".", "..", "sora/task", "sora?task", "sora task", "sora\ttask", "任务-1"} {
		_, err := (&TaskAdaptor{}).FetchTask("https://example.com", "unused", map[string]any{"task_id": taskID}, "")
		require.Error(t, err, taskID)
	}
	uri, err := buildSoraTaskFetchURL("https://example.com", "sora_task-1")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/v1/videos/sora_task-1", uri)
}

func TestSoraDoResponseUsesParserClosesBodyAndPreservesSafeErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("accepted response", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		body := &trackedSoraTaskSubmitBody{reader: strings.NewReader(`{"id":"sora-upstream-1","status":"queued","remixed_from_video_id":"sora-upstream-origin","prompt":"sensitive prompt"}`)}
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
		info.PublicTaskID = "task_public_1"
		info.OriginTaskID = "task_origin_1"

		taskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{StatusCode: http.StatusOK, Body: body}, info)

		require.Nil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "sora-upstream-1", taskID)
		require.NotContains(t, string(taskData), "sensitive")
		require.Contains(t, string(taskData), "task_origin_1")
		require.NotContains(t, string(taskData), "sora-upstream-origin")
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Contains(t, recorder.Body.String(), "task_origin_1")
		require.NotContains(t, recorder.Body.String(), "sora-upstream-origin")
		require.NotContains(t, recorder.Body.String(), "sensitive")
	})

	t.Run("non 200 preserves retry status without leaking body", func(t *testing.T) {
		body := &trackedSoraTaskSubmitBody{reader: strings.NewReader(`{"id":"sora-upstream-1","status":"queued","message":"sk-sensitive-upstream-value"}`)}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusServiceUnavailable, Body: body}, nil)

		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "fail_to_fetch_task", taskErr.Code)
		require.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
		require.False(t, taskErr.LocalError)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
	})

	t.Run("malformed response preserves legacy error mapping without leaking body", func(t *testing.T) {
		body := &trackedSoraTaskSubmitBody{reader: strings.NewReader(`{"id":"sk-sensitive-upstream-value"`)}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusOK, Body: body}, nil)

		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "unmarshal_response_body_failed", taskErr.Code)
		require.Equal(t, http.StatusInternalServerError, taskErr.StatusCode)
		require.False(t, taskErr.LocalError)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
	})

	t.Run("missing task relay info is safe", func(t *testing.T) {
		body := &trackedSoraTaskSubmitBody{reader: strings.NewReader(`{"id":"sora-upstream-1","status":"queued"}`)}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusOK, Body: body}, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "invalid_response", taskErr.Code)
	})

	t.Run("read failure closes body and redacts error", func(t *testing.T) {
		body := &failingSoraTaskSubmitBody{}
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{StatusCode: http.StatusOK, Body: body}, nil)
		require.NotNil(t, taskErr)
		require.True(t, body.closed)
		require.Equal(t, "read_response_body_failed", taskErr.Code)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
	})
}
