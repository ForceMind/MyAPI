package suno

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/relay/channel"
	taskcommon "github.com/ForceMind/MyAPI/relay/channel/task/taskcommon"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/service"

	"github.com/gin-gonic/gin"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
}

var _ channel.TaskSubmitResponseParser = (*TaskAdaptor)(nil)

// ParseTaskResult is not used for Suno tasks.
// Suno polling uses a dedicated batch-fetch path (service.UpdateSunoTasks) that
// receives dto.TaskResponse[[]dto.SunoDataResponse] from the upstream /fetch API.
// This differs from the per-task polling used by video adaptors.
func (a *TaskAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return nil, fmt.Errorf("suno uses batch polling via UpdateSunoTasks, ParseTaskResult is not applicable")
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	action := strings.ToUpper(c.Param("action"))

	var sunoRequest *dto.SunoSubmitReq
	err := common.UnmarshalBodyReusable(c, &sunoRequest)
	if err != nil {
		taskErr = service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		return
	}
	err = actionValidate(c, sunoRequest, action)
	if err != nil {
		taskErr = service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		return
	}

	//if sunoRequest.ContinueClipId != "" {
	//	if sunoRequest.TaskID == "" {
	//		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("task id is empty"), "invalid_request", http.StatusBadRequest)
	//		return
	//	}
	//	info.OriginTaskID = sunoRequest.TaskID
	//}

	info.Action = action
	c.Set("task_request", sunoRequest)
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := info.ChannelBaseUrl
	fullRequestURL := fmt.Sprintf("%s%s", baseURL, "/suno/submit/"+info.Action)
	return fullRequestURL, nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	req.Header.Set("Accept", c.Request.Header.Get("Accept"))
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	sunoRequest, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("task_request not found in context")
	}
	data, err := common.Marshal(sunoRequest)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	if resp == nil || resp.Body == nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("invalid Suno task submission response"),
			"invalid_response",
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, channel.MaxTaskSubmitResponseBytes+1))
	if err != nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("failed to read Suno task submission response"),
			"read_response_body_failed",
			http.StatusInternalServerError,
		)
	}
	upstreamRequestID := ""
	if resp.Header != nil {
		upstreamRequestID = resp.Header.Get(common.RequestIdKey)
	}
	parseInput := channel.TaskSubmitParseInput{
		HTTPStatus:        resp.StatusCode,
		Body:              responseBody,
		UpstreamRequestID: upstreamRequestID,
	}
	if info != nil {
		if info.TaskRelayInfo != nil {
			parseInput.PublicTaskID = info.PublicTaskID
		}
	}
	result := a.ParseTaskSubmitResponse(parseInput)
	if err := result.Validate(); err != nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("invalid Suno task submission parse result"),
			"invalid_response",
			http.StatusInternalServerError,
		)
	}
	if result.Disposition != channel.TaskSubmitAccepted {
		if resp.StatusCode != http.StatusOK {
			return "", nil, service.TaskErrorWrapper(
				fmt.Errorf("task submission upstream returned an unexpected HTTP status"),
				"fail_to_fetch_task",
				resp.StatusCode,
			)
		}
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("%s", result.Problem.SafeMessage),
			result.Problem.OutcomeCode,
			result.Problem.StatusCode,
		)
	}
	if c == nil || info == nil || info.TaskRelayInfo == nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("invalid Suno task submission response context"),
			"invalid_response",
			http.StatusInternalServerError,
		)
	}
	c.Data(result.LegacyResponse.StatusCode, result.LegacyResponse.ContentType, result.LegacyResponse.Body)
	return result.LegacyPollingID, result.TaskData, nil
}

type sunoTaskSubmitPersistence struct {
	Code string `json:"code"`
}

type sunoTaskSubmitLegacyResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

// ParseTaskSubmitResponse interprets one complete Suno submission response.
// It is intentionally independent of Gin, HTTP writers, persistence, billing,
// clocks, and network activity.
func (_ *TaskAdaptor) ParseTaskSubmitResponse(input channel.TaskSubmitParseInput) channel.TaskSubmitParseResult {
	providerTaskID := ""
	upstreamRequestID := ""
	if channel.IsValidTaskSubmitToken(input.UpstreamRequestID, channel.TaskSubmitUpstreamRequestIDMaxLength, false) {
		upstreamRequestID = input.UpstreamRequestID
	}
	unknown := func(code, message string, statusCode int) channel.TaskSubmitParseResult {
		return channel.TaskSubmitParseResult{
			Disposition:         channel.TaskSubmitUnknown,
			ProviderOperationID: providerTaskID,
			LegacyPollingID:     providerTaskID,
			UpstreamRequestID:   upstreamRequestID,
			Problem: &channel.TaskSubmitProblem{
				OutcomeCode: code,
				SafeMessage: message,
				StatusCode:  statusCode,
			},
		}
	}
	if len(input.Body) == 0 || len(input.Body) > channel.MaxTaskSubmitResponseBytes || !utf8.Valid(input.Body) {
		return unknown("invalid_response", "invalid Suno task submission response", http.StatusInternalServerError)
	}

	var response dto.TaskResponse[string]
	if err := common.Unmarshal(input.Body, &response); err != nil {
		return unknown("unmarshal_response_body_failed", "invalid Suno task submission response", http.StatusInternalServerError)
	}
	if response.Data != "" && isSafeSunoTaskID(response.Data) {
		providerTaskID = response.Data
	}
	if input.HTTPStatus != http.StatusOK {
		return unknown("unverified_response", "unverified Suno task submission response", http.StatusBadGateway)
	}
	if response.Code != dto.TaskSuccessCode {
		if channel.IsValidTaskSubmitToken(response.Code, channel.TaskSubmitOutcomeCodeMaxLength, false) {
			return unknown(response.Code, "Suno did not confirm the task submission", http.StatusInternalServerError)
		}
		return unknown("invalid_response", "invalid Suno task submission response", http.StatusInternalServerError)
	}
	if !isSafeSunoTaskID(response.Data) {
		return unknown("invalid_response", "invalid Suno task submission response", http.StatusInternalServerError)
	}

	taskData, err := common.Marshal(sunoTaskSubmitPersistence{Code: dto.TaskSuccessCode})
	if err != nil {
		return unknown("marshal_task_data_failed", "failed to build Suno task submission data", http.StatusInternalServerError)
	}
	legacyBody, err := common.Marshal(sunoTaskSubmitLegacyResponse{
		Code:    dto.TaskSuccessCode,
		Message: "",
		Data:    input.PublicTaskID,
	})
	if err != nil {
		return unknown("marshal_legacy_response_failed", "failed to build Suno task submission response", http.StatusInternalServerError)
	}
	return channel.TaskSubmitParseResult{
		Disposition:         channel.TaskSubmitAccepted,
		ProviderOperationID: providerTaskID,
		LegacyPollingID:     providerTaskID,
		UpstreamRequestID:   upstreamRequestID,
		TaskData:            taskData,
		LegacyResponse: &channel.LegacyTaskSubmitResponse{
			StatusCode:  http.StatusOK,
			ContentType: "application/json; charset=utf-8",
			Body:        legacyBody,
		},
	}
}

func isSafeSunoTaskID(taskID string) bool {
	if taskID == "." || taskID == ".." ||
		!channel.IsValidTaskSubmitToken(taskID, channel.TaskSubmitProviderOperationIDMaxLength, false) {
		return false
	}
	for index := 0; index < len(taskID); index++ {
		character := taskID[index]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	rawIDs, ok := body["ids"].([]string)
	if !ok || len(rawIDs) == 0 {
		return nil, fmt.Errorf("invalid task ids")
	}
	for _, taskID := range rawIDs {
		if !isSafeSunoTaskID(taskID) {
			return nil, fmt.Errorf("invalid task_id")
		}
	}
	requestUrl := fmt.Sprintf("%s/suno/fetch", baseUrl)
	byteBody, err := common.Marshal(map[string][]string{"ids": rawIDs})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", requestUrl, bytes.NewBuffer(byteBody))
	if err != nil {
		common.SysLog(fmt.Sprintf("Get Task error: %v", err))
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func actionValidate(c *gin.Context, sunoRequest *dto.SunoSubmitReq, action string) (err error) {
	switch action {
	case constant.SunoActionMusic:
		if sunoRequest.Mv == "" {
			sunoRequest.Mv = "chirp-v3-0"
		}
	case constant.SunoActionLyrics:
		if sunoRequest.Prompt == "" {
			err = fmt.Errorf("prompt_empty")
			return
		}
	default:
		err = fmt.Errorf("invalid_action")
	}
	return
}
