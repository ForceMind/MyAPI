package ali

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

type trackedResponseBody struct {
	reader io.Reader
	closed bool
}

func (body *trackedResponseBody) Read(data []byte) (int, error) {
	return body.reader.Read(data)
}

func (body *trackedResponseBody) Close() error {
	body.closed = true
	return nil
}

type failingResponseBody struct {
	closed bool
}

func (*failingResponseBody) Read([]byte) (int, error) {
	return 0, errors.New("read failed with sk-sensitive-upstream-value")
}

func (body *failingResponseBody) Close() error {
	body.closed = true
	return nil
}

func testRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}

func TestConvertToAliRequestWan27I2VBuildsMediaFromImage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:    "wan2.7-i2v",
		Prompt:   "animate the first frame",
		Image:    "https://example.com/first.png",
		Size:     "720p",
		Duration: 10,
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, "wan2.7-i2v", aliReq.Model)
	require.Equal(t, "720P", aliReq.Parameters.Resolution)
	require.Equal(t, 10, aliReq.Parameters.Duration)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/first.png"},
	}, aliReq.Input.Media)
	require.Empty(t, aliReq.Input.ImgURL)

	body, err := common.Marshal(aliReq)
	require.NoError(t, err)
	require.Contains(t, string(body), `"media"`)
	require.NotContains(t, string(body), `"img_url"`)
}

func TestConvertToAliRequestWan27I2VBuildsFirstAndLastFrameFromImages(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Prompt: "interpolate between frames",
		Images: []string{
			"https://example.com/first.png",
			"https://example.com/last.png",
		},
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/first.png"},
		{Type: "last_frame", URL: "https://example.com/last.png"},
	}, aliReq.Input.Media)
}

func TestConvertToAliRequestWan27I2VPrefersImageBeforeImagesAndInputReference(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:          "wan2.7-i2v",
		Prompt:         "use the direct image",
		Image:          " https://example.com/direct.png ",
		Images:         []string{"https://example.com/images-first.png", " https://example.com/images-last.png "},
		InputReference: "https://example.com/input-reference.png",
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/direct.png"},
		{Type: "last_frame", URL: "https://example.com/images-last.png"},
	}, aliReq.Input.Media)
}

func TestConvertToAliRequestWan27I2VFallsBackToFirstNonEmptyImage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Prompt: "skip blank images",
		Image:  " ",
		Images: []string{
			" ",
			" https://example.com/first.png ",
			" https://example.com/last.png ",
		},
		InputReference: "https://example.com/input-reference.png",
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/first.png"},
		{Type: "last_frame", URL: "https://example.com/last.png"},
	}, aliReq.Input.Media)
}

func TestConvertToAliRequestWan27I2VKeepsExplicitMetadataMedia(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:          "wan2.7-i2v",
		Prompt:         "continue the clip",
		Image:          "https://example.com/direct.png",
		Images:         []string{"https://example.com/images-first.png", "https://example.com/images-last.png"},
		InputReference: "https://example.com/input-reference.png",
		Metadata: map[string]interface{}{
			"input": map[string]interface{}{
				"media": []interface{}{
					map[string]interface{}{
						"type": "first_clip",
						"url":  "https://example.com/input.mp4",
					},
				},
			},
		},
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_clip", URL: "https://example.com/input.mp4"},
	}, aliReq.Input.Media)
	require.Empty(t, aliReq.Input.ImgURL)

	body, err := common.Marshal(aliReq)
	require.NoError(t, err)
	require.Contains(t, string(body), `"media"`)
	require.NotContains(t, string(body), `"img_url"`)
}

func TestConvertToAliRequestWan27I2VRequiresMedia(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Prompt: "animate without a frame",
	}

	_, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "requires image"))
}

func TestConvertToAliRequestWan25I2VKeepsLegacyImgURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.5-i2v-preview",
		Prompt: "animate the first frame",
		Image:  "https://example.com/first.png",
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, "https://example.com/first.png", aliReq.Input.ImgURL)
	require.Empty(t, aliReq.Input.Media)

	body, err := common.Marshal(aliReq)
	require.NoError(t, err)
	require.Contains(t, string(body), `"img_url"`)
	require.NotContains(t, string(body), `"media"`)
}

func TestParseTaskSubmitResponseAcceptedIsPureAndDeterministic(t *testing.T) {
	adaptor := &TaskAdaptor{ChannelType: 17, apiKey: "fixture-key", baseURL: "https://fixture.example"}
	adaptorBefore := *adaptor
	body := []byte(`{"output":{"task_id":"ali-upstream-1","task_status":"PENDING","orig_prompt":"sensitive prompt","video_url":"https://sensitive.example/video","unknown":"discard"},"request_id":"body-request-id","unknown_top":"discard"}`)
	bodyBefore := append([]byte(nil), body...)
	input := channel.TaskSubmitParseInput{
		HTTPStatus:        http.StatusOK,
		Body:              body,
		UpstreamRequestID: "header-request-id",
		PublicTaskID:      "task_public_1",
		OriginModelName:   "wan-origin",
		ClientModelName:   "wan-client",
		SubmittedAtUnix:   1_700_000_000,
	}

	first := adaptor.ParseTaskSubmitResponse(input)
	second := adaptor.ParseTaskSubmitResponse(input)

	require.NoError(t, first.Validate())
	require.Equal(t, first, second)
	require.Equal(t, adaptorBefore, *adaptor)
	require.Equal(t, bodyBefore, input.Body)
	require.Equal(t, channel.TaskSubmitAccepted, first.Disposition)
	require.Equal(t, "ali-upstream-1", first.ProviderOperationID)
	require.Equal(t, "ali-upstream-1", first.LegacyPollingID)
	require.Equal(t, "header-request-id", first.UpstreamRequestID)
	require.JSONEq(t, `{"output":{"task_status":"PENDING"}}`, string(first.TaskData))
	require.NotContains(t, string(first.TaskData), "sensitive prompt")
	require.NotContains(t, string(first.TaskData), "sensitive.example")
	require.NotContains(t, string(first.TaskData), "unknown")
	var persisted AliVideoResponse
	require.NoError(t, common.Unmarshal(first.TaskData, &persisted))
	require.Equal(t, "PENDING", persisted.Output.TaskStatus)
	require.Empty(t, persisted.Output.TaskID)
	require.Empty(t, persisted.Output.VideoURL)

	var legacy relaydto.OpenAIVideo
	require.NoError(t, common.Unmarshal(first.LegacyResponse.Body, &legacy))
	require.Equal(t, "task_public_1", legacy.ID)
	require.Equal(t, "task_public_1", legacy.TaskID)
	require.Equal(t, "wan-client", legacy.Model)
	require.Equal(t, relaydto.VideoStatusQueued, legacy.Status)
	require.EqualValues(t, 1_700_000_000, legacy.CreatedAt)
}

func TestParseTaskSubmitResponseClassifiesSafeNonAcceptedResults(t *testing.T) {
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
			name:       "structured provider error remains unknown",
			status:     http.StatusBadRequest,
			body:       []byte(`{"code":"InvalidParameter","message":"` + sensitiveValue + `"}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "ali_api_error",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:           "provider code and task id remain unknown with references",
			status:         http.StatusOK,
			body:           []byte(`{"code":"InvalidParameter","message":"failed","output":{"task_id":"ali-conflict-1"},"request_id":"body-request-id"}`),
			headerID:       "header-request-id",
			want:           channel.TaskSubmitUnknown,
			wantCode:       "ali_api_error",
			wantStatus:     http.StatusOK,
			wantProviderID: "ali-conflict-1",
			wantRequestID:  "header-request-id",
		},
		{
			name:           "null provider code and task id remain unknown",
			status:         http.StatusOK,
			body:           []byte(`{"code":null,"output":{"task_id":"ali-null-code-1"},"request_id":"body-request-id"}`),
			want:           channel.TaskSubmitUnknown,
			wantCode:       "ali_api_error",
			wantStatus:     http.StatusOK,
			wantProviderID: "ali-null-code-1",
			wantRequestID:  "body-request-id",
		},
		{
			name:           "empty provider code and task id remain unknown",
			status:         http.StatusOK,
			body:           []byte(`{"code":"","output":{"task_id":"ali-empty-code-1"}}`),
			want:           channel.TaskSubmitUnknown,
			wantCode:       "ali_api_error",
			wantStatus:     http.StatusOK,
			wantProviderID: "ali-empty-code-1",
		},
		{
			name:           "malformed provider code and task id remain unknown",
			status:         http.StatusOK,
			body:           []byte(`{"code":{},"output":{"task_id":"ali-malformed-code-1"}}`),
			want:           channel.TaskSubmitUnknown,
			wantCode:       "ali_api_error",
			wantStatus:     http.StatusOK,
			wantProviderID: "ali-malformed-code-1",
		},
		{
			name:           "server error code remains unknown with references",
			status:         http.StatusInternalServerError,
			body:           []byte(`{"code":"InternalError","output":{"task_id":"ali-server-1"},"request_id":"body-request-id"}`),
			want:           channel.TaskSubmitUnknown,
			wantCode:       "ali_api_error",
			wantStatus:     http.StatusInternalServerError,
			wantProviderID: "ali-server-1",
			wantRequestID:  "body-request-id",
		},
		{
			name:           "invalid HTTP status remains unknown with references",
			status:         0,
			body:           []byte(`{"output":{"task_id":"ali-status-1","task_status":"PENDING"},"request_id":"body-request-id"}`),
			want:           channel.TaskSubmitUnknown,
			wantCode:       "unverified_response",
			wantStatus:     http.StatusBadGateway,
			wantProviderID: "ali-status-1",
			wantRequestID:  "body-request-id",
		},
		{
			name:          "malformed response preserves header request id",
			status:        http.StatusOK,
			body:          []byte(`{"message":"` + sensitiveValue),
			headerID:      "header-request-id",
			want:          channel.TaskSubmitUnknown,
			wantCode:      "unmarshal_response_body_failed",
			wantStatus:    http.StatusInternalServerError,
			wantRequestID: "header-request-id",
		},
		{
			name:          "non UTF-8 response preserves header request id",
			status:        http.StatusOK,
			body:          []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'},
			headerID:      "header-request-id",
			want:          channel.TaskSubmitUnknown,
			wantCode:      "unmarshal_response_body_failed",
			wantStatus:    http.StatusInternalServerError,
			wantRequestID: "header-request-id",
		},
		{
			name:       "empty provider operation id",
			status:     http.StatusOK,
			body:       []byte(`{"output":{"task_id":"","task_status":"PENDING"}}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:           "unverified HTTP status",
			status:         http.StatusAccepted,
			body:           []byte(`{"output":{"task_id":"ali-upstream-1","task_status":"PENDING"}}`),
			want:           channel.TaskSubmitUnknown,
			wantCode:       "unverified_response",
			wantStatus:     http.StatusBadGateway,
			wantProviderID: "ali-upstream-1",
		},
		{
			name:       "oversized provider operation id",
			status:     http.StatusOK,
			body:       []byte(`{"output":{"task_id":"` + strings.Repeat("a", channel.TaskSubmitProviderOperationIDMaxLength+1) + `"}}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "provider operation id contains whitespace",
			status:     http.StatusOK,
			body:       []byte(`{"output":{"task_id":"ali upstream","task_status":"PENDING"}}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "provider operation id contains control character",
			status:     http.StatusOK,
			body:       []byte("{\"output\":{\"task_id\":\"ali\\u0001upstream\",\"task_status\":\"PENDING\"}}"),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "provider operation id contains unicode format character",
			status:     http.StatusOK,
			body:       []byte(`{"output":{"task_id":"ali\u200dupstream","task_status":"PENDING"}}`),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "oversized response body",
			status:     http.StatusOK,
			body:       []byte(strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1)),
			want:       channel.TaskSubmitUnknown,
			wantCode:   "invalid_response",
			wantStatus: http.StatusInternalServerError,
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

func TestTaskSubmitParseResultRejectsBrokenInvariants(t *testing.T) {
	tests := []channel.TaskSubmitParseResult{
		{Disposition: channel.TaskSubmitAccepted},
		{
			Disposition:         channel.TaskSubmitRejected,
			ProviderOperationID: "must-not-exist",
			Problem: &channel.TaskSubmitProblem{
				OutcomeCode: "provider_rejected",
				SafeMessage: "provider rejected the task submission",
				StatusCode:  http.StatusBadRequest,
			},
		},
		{
			Disposition: channel.TaskSubmitUnknown,
			Problem: &channel.TaskSubmitProblem{
				OutcomeCode: strings.Repeat("x", channel.TaskSubmitOutcomeCodeMaxLength+1),
				SafeMessage: "invalid response",
				StatusCode:  http.StatusBadGateway,
			},
		},
		{
			Disposition:       channel.TaskSubmitUnknown,
			UpstreamRequestID: "request id",
			Problem: &channel.TaskSubmitProblem{
				OutcomeCode: "invalid_response",
				SafeMessage: "invalid response",
				StatusCode:  http.StatusBadGateway,
			},
		},
		{
			Disposition: channel.TaskSubmitUnknown,
			Problem: &channel.TaskSubmitProblem{
				OutcomeCode: "invalid response",
				SafeMessage: "invalid response",
				StatusCode:  http.StatusBadGateway,
			},
		},
		{
			Disposition: channel.TaskSubmitUnknown,
			Problem: &channel.TaskSubmitProblem{
				OutcomeCode: "invalid_response",
				SafeMessage: "invalid\nresponse",
				StatusCode:  http.StatusBadGateway,
			},
		},
		{
			Disposition:         channel.TaskSubmitAccepted,
			ProviderOperationID: "ali-upstream-1",
			LegacyPollingID:     "ali\u200dupstream",
			TaskData:            []byte(`{"output":{}}`),
			LegacyResponse: &channel.LegacyTaskSubmitResponse{
				StatusCode:  http.StatusOK,
				ContentType: "application/json; charset=utf-8",
				Body:        []byte(`{}`),
			},
		},
		{
			Disposition:         channel.TaskSubmitAccepted,
			ProviderOperationID: "ali-upstream-1",
			TaskData:            []byte(`{"output":{}}`),
			LegacyResponse: &channel.LegacyTaskSubmitResponse{
				StatusCode:  http.StatusOK,
				ContentType: "application/json\r\nX-Injected: true",
				Body:        []byte(`{}`),
			},
		},
	}

	for _, result := range tests {
		require.Error(t, result.Validate())
	}
}

func TestTaskSubmitParseResultRequiresSafeJSONLegacySuccess(t *testing.T) {
	valid := func() channel.TaskSubmitParseResult {
		return channel.TaskSubmitParseResult{
			Disposition:         channel.TaskSubmitAccepted,
			ProviderOperationID: "ali-upstream-1",
			LegacyPollingID:     "ali-upstream-1",
			UpstreamRequestID:   "request-1",
			TaskData:            []byte(`{"output":{"task_status":"PENDING"}}`),
			LegacyResponse: &channel.LegacyTaskSubmitResponse{
				StatusCode:  http.StatusOK,
				ContentType: "application/json; charset=utf-8",
				Body:        []byte(`{"id":"task_public_1"}`),
			},
		}
	}
	require.NoError(t, valid().Validate())

	tests := []struct {
		name   string
		mutate func(*channel.TaskSubmitParseResult)
	}{
		{"accepted non 2xx status", func(result *channel.TaskSubmitParseResult) {
			result.LegacyResponse.StatusCode = http.StatusBadRequest
		}},
		{"malformed content type", func(result *channel.TaskSubmitParseResult) {
			result.LegacyResponse.ContentType = "application/json; charset"
		}},
		{"non JSON content type", func(result *channel.TaskSubmitParseResult) {
			result.LegacyResponse.ContentType = "text/plain"
		}},
		{"oversized content type", func(result *channel.TaskSubmitParseResult) {
			result.LegacyResponse.ContentType = "application/json; x=" + strings.Repeat("a", channel.TaskSubmitContentTypeMaxLength)
		}},
		{"malformed task data", func(result *channel.TaskSubmitParseResult) {
			result.TaskData = []byte(`{`)
		}},
		{"array task data", func(result *channel.TaskSubmitParseResult) {
			result.TaskData = []byte(`[]`)
		}},
		{"malformed legacy body", func(result *channel.TaskSubmitParseResult) {
			result.LegacyResponse.Body = []byte(`{`)
		}},
		{"array legacy body", func(result *channel.TaskSubmitParseResult) {
			result.LegacyResponse.Body = []byte(`[]`)
		}},
		{"provider id with whitespace", func(result *channel.TaskSubmitParseResult) {
			result.ProviderOperationID = "ali upstream"
		}},
		{"empty legacy polling id", func(result *channel.TaskSubmitParseResult) {
			result.LegacyPollingID = ""
		}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result := valid()
			testCase.mutate(&result)
			require.Error(t, result.Validate())
		})
	}
}

func TestAliTaskIDUsesStrictASCIIPathSegment(t *testing.T) {
	for _, taskID := range []string{"ali-123", "ALI_123", "01234567-89ab-cdef-0123-456789abcdef"} {
		require.True(t, isSafeAliTaskID(taskID), taskID)
		fetchURL, err := buildAliTaskFetchURL("https://example.com", taskID)
		require.NoError(t, err)
		require.Equal(t, "https://example.com/api/v1/tasks/"+taskID, fetchURL)
	}

	for _, taskID := range []string{
		"", ".", "..", "ali/task", "ali?task", "ali#task", "ali%2ftask", `ali\task`,
		"ali task", "ali\ttask", "ali\x01task", "ali\u200dtask", "任务-1",
	} {
		require.False(t, isSafeAliTaskID(taskID), taskID)
		_, err := buildAliTaskFetchURL("https://example.com", taskID)
		require.Error(t, err)
		_, err = (&TaskAdaptor{}).FetchTask("https://example.com", "unused", map[string]any{"task_id": taskID}, "")
		require.Error(t, err, "invalid IDs must fail before any HTTP request")
	}
}

func TestDoResponsePreservesAliLegacySuccessAndClosesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	ctx.Set("model", "wan-client")
	responseBody := &trackedResponseBody{reader: strings.NewReader(
		`{"output":{"task_id":"ali-upstream-1","task_status":"PENDING"},"request_id":"ali-request-1"}`,
	)}
	info := testRelayInfo()
	info.PublicTaskID = "task_public_1"
	info.OriginModelName = "wan-origin"

	taskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
		StatusCode: http.StatusOK,
		Body:       responseBody,
	}, info)

	require.Nil(t, taskErr)
	require.True(t, responseBody.closed)
	require.Equal(t, "ali-upstream-1", taskID)
	require.JSONEq(t, `{"output":{"task_status":"PENDING"}}`, string(taskData))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
	var legacy relaydto.OpenAIVideo
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &legacy))
	require.Equal(t, "task_public_1", legacy.ID)
	require.Equal(t, "task_public_1", legacy.TaskID)
	require.Equal(t, "wan-client", legacy.Model)
}

func TestDoResponseReturnsSafeProblemAndAlwaysClosesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("non 200 preserves legacy status without leaking body", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		responseBody := &trackedResponseBody{reader: strings.NewReader(
			`{"code":"InvalidParameter","message":"sk-sensitive-upstream-value"}`,
		)}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       responseBody,
		}, testRelayInfo())

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "fail_to_fetch_task", taskErr.Code)
		require.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		require.NotContains(t, taskErr.Message, "sk-sensitive-upstream-value")
		require.Empty(t, recorder.Body.Bytes())
	})

	t.Run("read failure", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		responseBody := &failingResponseBody{}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody,
		}, testRelayInfo())

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
		responseBody := &trackedResponseBody{reader: strings.NewReader(`{"output":`)}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody,
		}, testRelayInfo())

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "unmarshal_response_body_failed", taskErr.Code)
		require.Empty(t, recorder.Body.Bytes())
	})

	t.Run("oversized response", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		responseBody := &trackedResponseBody{reader: strings.NewReader(strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1))}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody,
		}, testRelayInfo())

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "invalid_response", taskErr.Code)
		require.Empty(t, recorder.Body.Bytes())
	})

	t.Run("nil response context", func(t *testing.T) {
		responseBody := &trackedResponseBody{reader: strings.NewReader(
			`{"output":{"task_id":"ali-upstream-1","task_status":"PENDING"}}`,
		)}

		_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody,
			Header:     nil,
		}, testRelayInfo())

		require.NotNil(t, taskErr)
		require.True(t, responseBody.closed)
		require.Equal(t, "invalid_response", taskErr.Code)
	})
}
