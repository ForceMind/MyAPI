package vidu

import (
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

type trackedViduTaskSubmitBody struct {
	reader    io.Reader
	bytesRead int
	closed    bool
}

func (body *trackedViduTaskSubmitBody) Read(data []byte) (int, error) {
	n, err := body.reader.Read(data)
	body.bytesRead += n
	return n, err
}

func (body *trackedViduTaskSubmitBody) Close() error {
	body.closed = true
	return nil
}

func viduTestRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}

func TestViduTaskSubmitParserAcceptedIsPureAndRedactsUpstreamFields(t *testing.T) {
	adaptor := &TaskAdaptor{ChannelType: 1, baseURL: "https://fixture.example"}
	adaptorBefore := *adaptor
	body := []byte(`{"task_id":"vidu-upstream-1","state":"created","prompt":"sensitive prompt","payload":"sk-sensitive-upstream-value","unknown":"discard"}`)
	bodyBefore := append([]byte(nil), body...)
	input := channel.TaskSubmitParseInput{
		HTTPStatus:        http.StatusOK,
		Body:              body,
		UpstreamRequestID: "header-request-id",
		PublicTaskID:      "task_public_1",
		OriginModelName:   "vidu-origin",
		SubmittedAtUnix:   1_700_000_000,
	}

	first := adaptor.ParseTaskSubmitResponse(input)
	second := adaptor.ParseTaskSubmitResponse(input)

	require.NoError(t, first.Validate())
	require.Equal(t, first, second)
	require.Equal(t, adaptorBefore, *adaptor)
	require.Equal(t, bodyBefore, input.Body)
	require.Equal(t, channel.TaskSubmitAccepted, first.Disposition)
	require.Equal(t, "vidu-upstream-1", first.ProviderOperationID)
	require.Equal(t, "vidu-upstream-1", first.LegacyPollingID)
	require.Equal(t, "header-request-id", first.UpstreamRequestID)
	require.JSONEq(t, `{"state":"created"}`, string(first.TaskData))
	require.NotContains(t, string(first.TaskData), "vidu-upstream-1")
	require.NotContains(t, string(first.TaskData), "sensitive prompt")
	require.NotContains(t, string(first.TaskData), "sk-sensitive-upstream-value")

	var legacy relaydto.OpenAIVideo
	require.NoError(t, common.Unmarshal(first.LegacyResponse.Body, &legacy))
	require.Equal(t, "task_public_1", legacy.ID)
	require.Equal(t, "task_public_1", legacy.TaskID)
	require.Equal(t, "vidu-origin", legacy.Model)
	require.Equal(t, relaydto.VideoStatusQueued, legacy.Status)
	require.EqualValues(t, 1_700_000_000, legacy.CreatedAt)
	require.NotContains(t, string(first.LegacyResponse.Body), "vidu-upstream-1")
	require.NotContains(t, string(first.LegacyResponse.Body), "sensitive prompt")
	require.NotContains(t, string(first.LegacyResponse.Body), "sk-sensitive-upstream-value")
}

func TestViduTaskSubmitParserClassifiesOnlyVerifiedResponses(t *testing.T) {
	const sensitiveValue = "sk-sensitive-upstream-value"
	tests := []struct {
		name         string
		input        channel.TaskSubmitParseInput
		wantCode     string
		wantStatus   int
		wantLocal    bool
		wantProvider string
	}{
		{
			name:         "created state with error envelope remains unknown",
			input:        channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"task_id":"vidu-error-1","state":"created","error":{"message":"` + sensitiveValue + `"}}`)},
			wantCode:     "unverified_response",
			wantStatus:   http.StatusBadGateway,
			wantProvider: "vidu-error-1",
		},
		{
			name:         "created state with error code remains unknown",
			input:        channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"task_id":"vidu-error-code-1","state":"created","err_code":"failed"}`)},
			wantCode:     "unverified_response",
			wantStatus:   http.StatusBadGateway,
			wantProvider: "vidu-error-code-1",
		},
		{
			name:         "failed state remains unknown",
			input:        channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"task_id":"vidu-failed-1","state":"failed","err_code":"failed","error":{"message":"` + sensitiveValue + `"}}`)},
			wantCode:     "task_failed",
			wantStatus:   http.StatusBadRequest,
			wantLocal:    true,
			wantProvider: "vidu-failed-1",
		},
		{
			name:         "non created lifecycle remains unknown",
			input:        channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"task_id":"vidu-queued-1","state":"queueing"}`)},
			wantCode:     "invalid_response",
			wantStatus:   http.StatusBadGateway,
			wantProvider: "vidu-queued-1",
		},
		{
			name:         "non 200 is unknown with safe reference",
			input:        channel.TaskSubmitParseInput{HTTPStatus: http.StatusBadGateway, Body: []byte(`{"task_id":"vidu-status-1","state":"created"}`)},
			wantCode:     "unverified_response",
			wantStatus:   http.StatusBadGateway,
			wantProvider: "vidu-status-1",
		},
		{
			name:       "missing task id is unknown",
			input:      channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"state":"created"}`)},
			wantCode:   "invalid_response",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:         "null state is unknown with safe reference",
			input:        channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"task_id":"vidu-null-1","state":null}`)},
			wantCode:     "invalid_response",
			wantStatus:   http.StatusBadGateway,
			wantProvider: "vidu-null-1",
		},
		{
			name:       "unsafe task id is unknown",
			input:      channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"task_id":"vidu/unsafe","state":"created"}`)},
			wantCode:   "invalid_response",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "malformed body is unknown",
			input:      channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"task_id":`)},
			wantCode:   "unmarshal_response_body_failed",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "oversized body is unknown",
			input:      channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1))},
			wantCode:   "invalid_response",
			wantStatus: http.StatusBadGateway,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := (&TaskAdaptor{}).ParseTaskSubmitResponse(test.input)

			require.NoError(t, result.Validate())
			require.Equal(t, channel.TaskSubmitUnknown, result.Disposition)
			require.Equal(t, test.wantCode, result.Problem.OutcomeCode)
			require.Equal(t, test.wantStatus, result.Problem.StatusCode)
			require.Equal(t, test.wantLocal, result.Problem.Local)
			require.Equal(t, test.wantProvider, result.ProviderOperationID)
			require.NotContains(t, result.Problem.SafeMessage, sensitiveValue)
		})
	}
}

func TestViduTaskSubmitDoResponseUsesBoundedPureParser(t *testing.T) {
	const sensitiveValue = "sk-sensitive-upstream-value"
	body := &trackedViduTaskSubmitBody{reader: strings.NewReader(
		sensitiveValue + strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1024),
	)}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
		StatusCode: http.StatusOK,
		Body:       body,
	}, viduTestRelayInfo())

	require.True(t, body.closed)
	require.Equal(t, channel.MaxTaskSubmitResponseBytes+1, body.bytesRead)
	require.NotNil(t, taskErr)
	require.NotContains(t, taskErr.Message, sensitiveValue)
}

func TestViduTaskSubmitDoResponsePreservesLegacyMappings(t *testing.T) {
	t.Run("accepted response writes the safe legacy envelope", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		info := viduTestRelayInfo()
		info.PublicTaskID = "task_public_1"
		info.OriginModelName = "vidu-origin"

		upstreamTaskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"task_id":"vidu-upstream-1","state":"created","prompt":"sensitive prompt"}`)),
		}, info)

		require.Nil(t, taskErr)
		require.Equal(t, "vidu-upstream-1", upstreamTaskID)
		require.JSONEq(t, `{"state":"created"}`, string(taskData))
		require.Equal(t, http.StatusOK, recorder.Code)
		require.NotContains(t, recorder.Body.String(), "sensitive prompt")
	})

	t.Run("failed response preserves local task failed error", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"task_id":"vidu-upstream-1","state":"failed","err_code":"failed","error":{"message":"sensitive"}}`)),
		}, viduTestRelayInfo())

		require.NotNil(t, taskErr)
		require.Equal(t, "task_failed", taskErr.Code)
		require.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		require.True(t, taskErr.LocalError)
	})

	t.Run("non 200 preserves legacy fetch error", func(t *testing.T) {
		body := &trackedViduTaskSubmitBody{reader: strings.NewReader(`{"task_id":"vidu-upstream-1","state":"created"}`)}
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       body,
		}, viduTestRelayInfo())

		require.True(t, body.closed)
		require.NotNil(t, taskErr)
		require.Equal(t, "fail_to_fetch_task", taskErr.Code)
		require.Equal(t, http.StatusBadGateway, taskErr.StatusCode)
		require.False(t, taskErr.LocalError)
	})

	t.Run("nil task relay info cannot panic or write success", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		body := &trackedViduTaskSubmitBody{reader: strings.NewReader(`{"task_id":"vidu-upstream-1","state":"created"}`)}
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       body,
		}, info)

		require.True(t, body.closed)
		require.NotNil(t, taskErr)
		require.Equal(t, "invalid_response", taskErr.Code)
		require.Equal(t, http.StatusInternalServerError, taskErr.StatusCode)
		require.Empty(t, recorder.Body.String())
	})
}

func TestBuildViduTaskFetchURLRejectsUnsafeTaskIDs(t *testing.T) {
	url, err := buildViduTaskFetchURL("https://fixture.example", "vidu-upstream-1")
	require.NoError(t, err)
	require.Equal(t, "https://fixture.example/ent/v2/tasks/vidu-upstream-1/creations", url)

	for _, taskID := range []string{"", "..", "vidu/unsafe", "vidu?query"} {
		_, err := buildViduTaskFetchURL("https://fixture.example", taskID)
		require.Error(t, err, taskID)
	}
}
