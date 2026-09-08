package vertex

import (
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

const vertexTestOperationName = "projects/demo-project/locations/us-central1/publishers/google/models/veo-3.1-generate-preview/operations/a1b07c8e-7b5a-4aba-bb34-3e1ccb8afcc8"

type trackedVertexTaskSubmitBody struct {
	reader    io.Reader
	bytesRead int
	closed    bool
}

func (body *trackedVertexTaskSubmitBody) Read(data []byte) (int, error) {
	n, err := body.reader.Read(data)
	body.bytesRead += n
	return n, err
}

func (body *trackedVertexTaskSubmitBody) Close() error {
	body.closed = true
	return nil
}

func vertexTestRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}

func TestVertexTaskSubmitParserAcceptedIsPureAndRedactsUpstreamFields(t *testing.T) {
	adaptor := &TaskAdaptor{ChannelType: 1, apiKey: "fixture-key", baseURL: "https://fixture.example"}
	adaptorBefore := *adaptor
	body := []byte(`{"name":"` + vertexTestOperationName + `","prompt":"sensitive prompt","unknown":"discard"}`)
	bodyBefore := append([]byte(nil), body...)
	input := channel.TaskSubmitParseInput{
		HTTPStatus:        http.StatusOK,
		Body:              body,
		UpstreamRequestID: "header-request-id",
		PublicTaskID:      "task_public_1",
		OriginModelName:   "vertex-origin",
		SubmittedAtUnix:   1_700_000_000,
	}

	first := adaptor.ParseTaskSubmitResponse(input)
	second := adaptor.ParseTaskSubmitResponse(input)

	require.NoError(t, first.Validate())
	require.Equal(t, first, second)
	require.Equal(t, adaptorBefore, *adaptor)
	require.Equal(t, bodyBefore, input.Body)
	require.Equal(t, channel.TaskSubmitAccepted, first.Disposition)
	require.Equal(t, vertexTestOperationName, first.ProviderOperationID)
	require.Equal(t, taskcommon.EncodeLocalTaskID(vertexTestOperationName), first.LegacyPollingID)
	require.Equal(t, "header-request-id", first.UpstreamRequestID)
	require.JSONEq(t, `{}`, string(first.TaskData))
	require.NotContains(t, string(first.TaskData), vertexTestOperationName)

	var legacy relaydto.OpenAIVideo
	require.NoError(t, common.Unmarshal(first.LegacyResponse.Body, &legacy))
	require.Equal(t, "task_public_1", legacy.ID)
	require.Equal(t, "task_public_1", legacy.TaskID)
	require.Equal(t, "vertex-origin", legacy.Model)
	require.Equal(t, relaydto.VideoStatusQueued, legacy.Status)
	require.EqualValues(t, 1_700_000_000, legacy.CreatedAt)
	require.NotContains(t, string(first.LegacyResponse.Body), vertexTestOperationName)
	require.NotContains(t, string(first.LegacyResponse.Body), "sensitive prompt")
	require.NotContains(t, string(first.LegacyResponse.Body), "sk-sensitive-upstream-value")
}

func TestVertexTaskSubmitParserClassifiesOnlyVerifiedResponses(t *testing.T) {
	const sensitiveValue = "sk-sensitive-upstream-value"
	tests := []struct {
		name         string
		input        channel.TaskSubmitParseInput
		wantCode     string
		wantStatus   int
		wantProvider string
	}{
		{
			name:         "error envelope with operation name is unknown",
			input:        channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"name":"` + vertexTestOperationName + `","error":{"message":"` + sensitiveValue + `"}}`)},
			wantCode:     "unverified_response",
			wantStatus:   http.StatusInternalServerError,
			wantProvider: vertexTestOperationName,
		},
		{
			name:         "non 200 is unknown with safe operation reference",
			input:        channel.TaskSubmitParseInput{HTTPStatus: http.StatusBadGateway, Body: []byte(`{"name":"` + vertexTestOperationName + `"}`)},
			wantCode:     "unverified_response",
			wantStatus:   http.StatusBadGateway,
			wantProvider: vertexTestOperationName,
		},
		{
			name:       "missing name is unknown",
			input:      channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{}`)},
			wantCode:   "invalid_response",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "null name is unknown",
			input:      channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"name":null}`)},
			wantCode:   "invalid_response",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "unsafe operation name is unknown",
			input:      channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"name":"projects/demo/locations/us-central1/publishers/google/models/veo/operations/unsafe?query"}`)},
			wantCode:   "invalid_response",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "malformed body is unknown",
			input:      channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(`{"name":`)},
			wantCode:   "unmarshal_response_failed",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "oversized body is unknown",
			input:      channel.TaskSubmitParseInput{HTTPStatus: http.StatusOK, Body: []byte(strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1))},
			wantCode:   "invalid_response",
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := (&TaskAdaptor{}).ParseTaskSubmitResponse(test.input)

			require.NoError(t, result.Validate())
			require.Equal(t, channel.TaskSubmitUnknown, result.Disposition)
			require.Equal(t, test.wantCode, result.Problem.OutcomeCode)
			require.Equal(t, test.wantStatus, result.Problem.StatusCode)
			require.Equal(t, test.wantProvider, result.ProviderOperationID)
			require.NotContains(t, result.Problem.SafeMessage, sensitiveValue)
		})
	}
}

func TestVertexTaskSubmitDoResponseUsesBoundedPureParser(t *testing.T) {
	const sensitiveValue = "sk-sensitive-upstream-value"
	body := &trackedVertexTaskSubmitBody{reader: strings.NewReader(
		sensitiveValue + strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1024),
	)}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
		StatusCode: http.StatusOK,
		Body:       body,
	}, vertexTestRelayInfo())

	require.True(t, body.closed)
	require.Equal(t, channel.MaxTaskSubmitResponseBytes+1, body.bytesRead)
	require.NotNil(t, taskErr)
	require.NotContains(t, taskErr.Message, sensitiveValue)
}

func TestVertexTaskSubmitDoResponsePreservesLegacyMappings(t *testing.T) {
	t.Run("accepted response writes the safe legacy envelope", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		info := vertexTestRelayInfo()
		info.PublicTaskID = "task_public_1"
		info.OriginModelName = "vertex-origin"

		upstreamTaskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"name":"` + vertexTestOperationName + `","prompt":"sensitive prompt"}`)),
		}, info)

		require.Nil(t, taskErr)
		require.Equal(t, taskcommon.EncodeLocalTaskID(vertexTestOperationName), upstreamTaskID)
		require.JSONEq(t, `{}`, string(taskData))
		require.Equal(t, http.StatusOK, recorder.Code)
		require.NotContains(t, recorder.Body.String(), "sensitive prompt")
	})

	t.Run("non 200 preserves legacy fetch error", func(t *testing.T) {
		body := &trackedVertexTaskSubmitBody{reader: strings.NewReader(`{"name":"` + vertexTestOperationName + `"}`)}
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       body,
		}, vertexTestRelayInfo())

		require.True(t, body.closed)
		require.NotNil(t, taskErr)
		require.Equal(t, "fail_to_fetch_task", taskErr.Code)
		require.Equal(t, http.StatusBadGateway, taskErr.StatusCode)
		require.False(t, taskErr.LocalError)
	})

	t.Run("nil task relay info cannot panic or write success", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		body := &trackedVertexTaskSubmitBody{reader: strings.NewReader(`{"name":"` + vertexTestOperationName + `"}`)}
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

func TestBuildFetchOperationURLRejectsUnsafeOperationNames(t *testing.T) {
	url, err := buildFetchOperationURL("https://fixture.example", vertexTestOperationName)
	require.NoError(t, err)
	require.Equal(t, "https://fixture.example/v1/projects/demo-project/locations/us-central1/publishers/google/models/veo-3.1-generate-preview:fetchPredictOperation", url)

	for _, operationName := range []string{"", "../unsafe", "projects/demo/locations/us-central1/publishers/google/models/veo/operations/unsafe?query"} {
		_, err := buildFetchOperationURL("https://fixture.example", operationName)
		require.Error(t, err, operationName)
	}
}
