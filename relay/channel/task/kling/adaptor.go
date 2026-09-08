package kling

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"

	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"

	"github.com/ForceMind/MyAPI/constant"
	taskdto "github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/relay/channel"
	taskcommon "github.com/ForceMind/MyAPI/relay/channel/task/taskcommon"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/service"
)

// ============================
// Request / Response structures
// ============================

type TrajectoryPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type DynamicMask struct {
	Mask         string            `json:"mask,omitempty"`
	Trajectories []TrajectoryPoint `json:"trajectories,omitempty"`
}

type CameraConfig struct {
	Horizontal float64 `json:"horizontal,omitempty"`
	Vertical   float64 `json:"vertical,omitempty"`
	Pan        float64 `json:"pan,omitempty"`
	Tilt       float64 `json:"tilt,omitempty"`
	Roll       float64 `json:"roll,omitempty"`
	Zoom       float64 `json:"zoom,omitempty"`
}

type CameraControl struct {
	Type   string        `json:"type,omitempty"`
	Config *CameraConfig `json:"config,omitempty"`
}

type requestPayload struct {
	Prompt         string         `json:"prompt,omitempty"`
	Image          string         `json:"image,omitempty"`
	ImageTail      string         `json:"image_tail,omitempty"`
	NegativePrompt string         `json:"negative_prompt,omitempty"`
	Mode           string         `json:"mode,omitempty"`
	Duration       string         `json:"duration,omitempty"`
	AspectRatio    string         `json:"aspect_ratio,omitempty"`
	ModelName      string         `json:"model_name,omitempty"`
	Model          string         `json:"model,omitempty"` // Compatible with upstreams that only recognize "model"
	CfgScale       float64        `json:"cfg_scale,omitempty"`
	StaticMask     string         `json:"static_mask,omitempty"`
	DynamicMasks   []DynamicMask  `json:"dynamic_masks,omitempty"`
	CameraControl  *CameraControl `json:"camera_control,omitempty"`
	CallbackUrl    string         `json:"callback_url,omitempty"`
	ExternalTaskId string         `json:"external_task_id,omitempty"`
}

type responsePayload struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	TaskId    string `json:"task_id"`
	RequestId string `json:"request_id"`
	Data      struct {
		TaskId        string `json:"task_id"`
		TaskStatus    string `json:"task_status"`
		TaskStatusMsg string `json:"task_status_msg"`
		TaskInfo      struct {
			ExternalTaskId string `json:"external_task_id"`
		} `json:"task_info"`
		WatermarkInfo struct {
			Enabled bool `json:"enabled"`
		} `json:"watermark_info"`
		TaskResult struct {
			Videos []struct {
				Id           string `json:"id"`
				Url          string `json:"url"`
				WatermarkUrl string `json:"watermark_url"`
				Duration     string `json:"duration"`
			} `json:"videos"`
			Images []struct {
				Index        int    `json:"index"`
				Url          string `json:"url"`
				WatermarkUrl string `json:"watermark_url"`
			} `json:"images"`
		} `json:"task_result"`
		CreatedAt          int64  `json:"created_at"`
		UpdatedAt          int64  `json:"updated_at"`
		FinalUnitDeduction string `json:"final_unit_deduction"`
	} `json:"data"`
}

// klingTaskSubmitPersistence is the bounded snapshot needed by the legacy
// video response converter before its first polling update. The provider task
// id is held separately in Task.PrivateData; arbitrary upstream response
// fields, including prompts, URLs, messages, and extensions, are excluded.
type klingTaskSubmitPersistence struct {
	Code int `json:"code"`
	Data struct {
		TaskStatus string `json:"task_status,omitempty"`
	} `json:"data"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

var _ channel.TaskSubmitResponseParser = (*TaskAdaptor)(nil)

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey

	// apiKey format: "access_key|secret_key"
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	// Use the standard validation method for TaskSubmitReq
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	path := lo.Ternary(info.Action == constant.TaskActionGenerate, "/v1/videos/image2video", "/v1/videos/text2video")

	if isNewAPIRelay(info.ApiKey) {
		return fmt.Sprintf("%s/kling%s", a.baseURL, path), nil
	}

	return fmt.Sprintf("%s%s", a.baseURL, path), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	token, err := a.createJWTToken()
	if err != nil {
		return fmt.Errorf("failed to create JWT token: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "kling-sdk/1.0")
	return nil
}

// BuildRequestBody converts request into Kling specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	body, err := a.convertToRequestPayload(&req, info)
	if err != nil {
		return nil, err
	}
	if body.Image == "" && body.ImageTail == "" {
		c.Set("action", constant.TaskActionTextGenerate)
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	if action := c.GetString("action"); action != "" {
		info.Action = action
	}
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	if resp == nil || resp.Body == nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("invalid Kling task submission response"),
			"invalid_response",
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, channel.MaxTaskSubmitResponseBytes+1))
	if err != nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("failed to read Kling task submission response"),
			"read_response_body_failed",
			http.StatusInternalServerError,
		)
	}
	modelName := ""
	if c != nil {
		modelName = c.GetString("model")
	}
	if modelName == "" && info != nil {
		modelName = info.OriginModelName
	}
	upstreamRequestID := ""
	if resp.Header != nil {
		upstreamRequestID = resp.Header.Get(common.RequestIdKey)
	}
	parseInput := channel.TaskSubmitParseInput{
		HTTPStatus:        resp.StatusCode,
		Body:              responseBody,
		UpstreamRequestID: upstreamRequestID,
		ClientModelName:   modelName,
		SubmittedAtUnix:   common.GetTimestamp(),
	}
	if info != nil {
		parseInput.OriginModelName = info.OriginModelName
		if info.TaskRelayInfo != nil {
			parseInput.PublicTaskID = info.PublicTaskID
		}
	}
	result := a.ParseTaskSubmitResponse(parseInput)
	if err := result.Validate(); err != nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("invalid Kling task submission parse result"),
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
		problem := result.Problem
		if problem.Local {
			return "", nil, service.TaskErrorWrapperLocal(fmt.Errorf("%s", problem.SafeMessage), problem.OutcomeCode, problem.StatusCode)
		}
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("%s", problem.SafeMessage), problem.OutcomeCode, problem.StatusCode)
	}
	if c == nil || info == nil || info.TaskRelayInfo == nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("invalid Kling task submission response context"),
			"invalid_response",
			http.StatusInternalServerError,
		)
	}
	c.Data(result.LegacyResponse.StatusCode, result.LegacyResponse.ContentType, result.LegacyResponse.Body)
	return result.LegacyPollingID, result.TaskData, nil
}

// ParseTaskSubmitResponse interprets one complete Kling submission response.
// It is intentionally independent of Gin, HTTP writers, persistence, billing,
// clocks, and network activity.
func (_ *TaskAdaptor) ParseTaskSubmitResponse(input channel.TaskSubmitParseInput) channel.TaskSubmitParseResult {
	providerTaskID := ""
	upstreamRequestID := ""
	if channel.IsValidTaskSubmitToken(input.UpstreamRequestID, channel.TaskSubmitUpstreamRequestIDMaxLength, false) {
		upstreamRequestID = input.UpstreamRequestID
	}
	unknown := func(code, message string) channel.TaskSubmitParseResult {
		return channel.TaskSubmitParseResult{
			Disposition:         channel.TaskSubmitUnknown,
			ProviderOperationID: providerTaskID,
			LegacyPollingID:     providerTaskID,
			UpstreamRequestID:   upstreamRequestID,
			Problem: &channel.TaskSubmitProblem{
				OutcomeCode: code,
				SafeMessage: message,
				StatusCode:  http.StatusBadGateway,
			},
		}
	}
	if len(input.Body) == 0 || len(input.Body) > channel.MaxTaskSubmitResponseBytes || !utf8.Valid(input.Body) {
		return unknown("invalid_response", "invalid Kling task submission response")
	}

	var responseObject map[string]json.RawMessage
	if err := common.Unmarshal(input.Body, &responseObject); err != nil || responseObject == nil {
		return unknown("unmarshal_response_body_failed", "invalid Kling task submission response")
	}
	codeBody, ok := responseObject["code"]
	if !ok {
		return unknown("invalid_response", "invalid Kling task submission response")
	}
	var code *int
	if err := common.Unmarshal(codeBody, &code); err != nil || code == nil {
		return unknown("invalid_response", "invalid Kling task submission response")
	}
	if requestIDBody, exists := responseObject["request_id"]; exists && upstreamRequestID == "" {
		var requestID string
		if err := common.Unmarshal(requestIDBody, &requestID); err == nil &&
			channel.IsValidTaskSubmitToken(requestID, channel.TaskSubmitUpstreamRequestIDMaxLength, false) {
			upstreamRequestID = requestID
		}
	}

	dataObject, dataPresent, dataObjectValid := parseKlingTaskSubmitData(responseObject["data"])
	if !dataObjectValid {
		return unknown("invalid_response", "invalid Kling task submission response")
	}
	topLevelTaskID, topLevelTaskIDPresent, topLevelTaskIDValid := parseKlingTaskSubmitTaskID(responseObject["task_id"])
	if !topLevelTaskIDValid {
		return unknown("invalid_response", "invalid Kling task submission response")
	}
	dataTaskID := ""
	dataTaskIDPresent := false
	if dataPresent {
		var dataTaskIDValid bool
		dataTaskID, dataTaskIDPresent, dataTaskIDValid = parseKlingTaskSubmitTaskID(dataObject["task_id"])
		if !dataTaskIDValid {
			return unknown("invalid_response", "invalid Kling task submission response")
		}
	}
	if topLevelTaskIDPresent && dataTaskIDPresent && topLevelTaskID != dataTaskID {
		return unknown("unverified_response", "Kling returned conflicting task submission evidence")
	}
	if dataTaskIDPresent {
		providerTaskID = dataTaskID
	} else if topLevelTaskIDPresent {
		providerTaskID = topLevelTaskID
	}

	if input.HTTPStatus != http.StatusOK {
		return unknown("unverified_response", "unverified Kling task submission response")
	}
	if *code != 0 {
		// The public success shape establishes code == 0, but no rejection
		// allowlist is yet verified. Preserve the old local-error behavior for
		// the gate-off bridge while keeping durable disposition conservative.
		return channel.TaskSubmitParseResult{
			Disposition:         channel.TaskSubmitUnknown,
			ProviderOperationID: providerTaskID,
			LegacyPollingID:     providerTaskID,
			UpstreamRequestID:   upstreamRequestID,
			Problem: &channel.TaskSubmitProblem{
				OutcomeCode: "task_failed",
				SafeMessage: "Kling did not confirm the task submission",
				StatusCode:  http.StatusBadRequest,
				Local:       true,
			},
		}
	}
	if !dataPresent || !dataTaskIDPresent || providerTaskID == "" {
		return unknown("invalid_response", "Kling did not return a valid task id")
	}

	taskStatus := ""
	if taskStatusBody, exists := dataObject["task_status"]; exists {
		var upstreamTaskStatus string
		if err := common.Unmarshal(taskStatusBody, &upstreamTaskStatus); err == nil {
			switch upstreamTaskStatus {
			case "submitted", "processing", "succeed", "failed":
				taskStatus = upstreamTaskStatus
			}
		}
	}
	taskData := klingTaskSubmitPersistence{Code: *code}
	taskData.Data.TaskStatus = taskStatus
	persistedTaskData, err := common.Marshal(taskData)
	if err != nil {
		return unknown("marshal_task_data_failed", "failed to build Kling task submission data")
	}

	modelName := input.ClientModelName
	if modelName == "" {
		modelName = input.OriginModelName
	}
	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = input.PublicTaskID
	openAIResp.TaskID = input.PublicTaskID
	openAIResp.Model = modelName
	openAIResp.CreatedAt = input.SubmittedAtUnix
	legacyBody, err := common.Marshal(openAIResp)
	if err != nil {
		return unknown("marshal_legacy_response_failed", "failed to build Kling task submission response")
	}

	return channel.TaskSubmitParseResult{
		Disposition:         channel.TaskSubmitAccepted,
		ProviderOperationID: providerTaskID,
		LegacyPollingID:     providerTaskID,
		UpstreamRequestID:   upstreamRequestID,
		TaskData:            persistedTaskData,
		LegacyResponse: &channel.LegacyTaskSubmitResponse{
			StatusCode:  http.StatusOK,
			ContentType: "application/json; charset=utf-8",
			Body:        legacyBody,
		},
	}
}

// parseKlingTaskSubmitData distinguishes an omitted or null data field from
// malformed non-object data. Only a verified success requires data.task_id;
// an error envelope remains unknown until a rejection contract is verified.
func parseKlingTaskSubmitData(dataBody json.RawMessage) (map[string]json.RawMessage, bool, bool) {
	if dataBody == nil {
		return nil, false, true
	}
	if string(bytes.TrimSpace(dataBody)) == "null" {
		return nil, false, true
	}
	var dataObject map[string]json.RawMessage
	if err := common.Unmarshal(dataBody, &dataObject); err != nil || dataObject == nil {
		return nil, false, false
	}
	return dataObject, true, true
}

// parseKlingTaskSubmitTaskID treats a present but null, malformed, or unsafe
// task id as unverified evidence. This prevents alternate top-level and data
// shapes from being silently ignored during durable migration.
func parseKlingTaskSubmitTaskID(taskIDBody json.RawMessage) (string, bool, bool) {
	if taskIDBody == nil {
		return "", false, true
	}
	var taskID string
	if err := common.Unmarshal(taskIDBody, &taskID); err != nil || !isSafeKlingTaskID(taskID) {
		return "", true, false
	}
	return taskID, true, true
}

func isSafeKlingTaskID(taskID string) bool {
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

func buildKlingTaskFetchURL(baseURL, path, taskID string, useNewAPIRelay bool) (string, error) {
	if !isSafeKlingTaskID(taskID) {
		return "", fmt.Errorf("invalid task_id")
	}
	if useNewAPIRelay {
		return fmt.Sprintf("%s/kling%s/%s", baseURL, path, url.PathEscape(taskID)), nil
	}
	return fmt.Sprintf("%s%s/%s", baseURL, path, url.PathEscape(taskID)), nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || !isSafeKlingTaskID(taskID) {
		return nil, fmt.Errorf("invalid task_id")
	}
	action, ok := body["action"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid action")
	}
	path := lo.Ternary(action == constant.TaskActionGenerate, "/v1/videos/image2video", "/v1/videos/text2video")
	requestURL, err := buildKlingTaskFetchURL(baseUrl, path, taskID, isNewAPIRelay(key))
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	token, err := a.createJWTTokenWithKey(key)
	if err != nil {
		token = key
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "kling-sdk/1.0")

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{"kling-v1", "kling-v1-6", "kling-v2-master"}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "kling"
}

// ============================
// helpers
// ============================

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*requestPayload, error) {
	authoritativeModel := info.UpstreamModelName
	if authoritativeModel == "" {
		authoritativeModel = "kling-v1"
	}
	authoritativeMode := taskcommon.DefaultString(req.Mode, "std")
	authoritativeDuration := fmt.Sprintf("%d", taskcommon.DefaultInt(req.Duration, 5))
	r := requestPayload{
		Prompt:         req.Prompt,
		Image:          req.Image,
		Mode:           authoritativeMode,
		Duration:       authoritativeDuration,
		AspectRatio:    a.getAspectRatio(req.Size),
		ModelName:      authoritativeModel,
		Model:          authoritativeModel,
		CfgScale:       0.5,
		StaticMask:     "",
		DynamicMasks:   []DynamicMask{},
		CameraControl:  nil,
		CallbackUrl:    "",
		ExternalTaskId: "",
	}
	metadata := make(map[string]any, len(req.Metadata))
	for key, value := range req.Metadata {
		switch key {
		case "model", "model_name", "req_key", "prompt", "image", "mode", "duration":
			continue
		default:
			metadata[key] = value
		}
	}
	if err := taskcommon.UnmarshalMetadata(metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	// Routing and billing have already selected UpstreamModelName. Provider
	// metadata may configure other Kling options, but it cannot change either
	// model alias after that selection.
	r.ModelName = authoritativeModel
	r.Model = authoritativeModel
	r.Prompt = req.Prompt
	r.Image = req.Image
	r.Mode = authoritativeMode
	r.Duration = authoritativeDuration
	return &r, nil
}

func (a *TaskAdaptor) getAspectRatio(size string) string {
	switch size {
	case "1024x1024", "512x512":
		return "1:1"
	case "1280x720", "1920x1080":
		return "16:9"
	case "720x1280", "1080x1920":
		return "9:16"
	default:
		return "1:1"
	}
}

// ============================
// JWT helpers
// ============================

func (a *TaskAdaptor) createJWTToken() (string, error) {
	return a.createJWTTokenWithKey(a.apiKey)
}

func (a *TaskAdaptor) createJWTTokenWithKey(apiKey string) (string, error) {
	if isNewAPIRelay(apiKey) {
		return apiKey, nil // new api relay
	}
	keyParts := strings.Split(apiKey, "|")
	if len(keyParts) != 2 {
		return "", errors.New("invalid api_key, required format is accessKey|secretKey")
	}
	accessKey := strings.TrimSpace(keyParts[0])
	if len(keyParts) == 1 {
		return accessKey, nil
	}
	secretKey := strings.TrimSpace(keyParts[1])
	now := time.Now().Unix()
	claims := jwt.MapClaims{
		"iss": accessKey,
		"exp": now + 1800, // 30 minutes
		"nbf": now - 5,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["typ"] = "JWT"
	return token.SignedString([]byte(secretKey))
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	taskInfo := &relaycommon.TaskInfo{}
	resPayload := responsePayload{}
	err := common.Unmarshal(respBody, &resPayload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}
	taskInfo.Code = resPayload.Code
	taskInfo.TaskID = resPayload.Data.TaskId
	taskInfo.Reason = resPayload.Data.TaskStatusMsg
	//任务状态，枚举值：submitted（已提交）、processing（处理中）、succeed（成功）、failed（失败）
	status := resPayload.Data.TaskStatus
	switch status {
	case "submitted":
		taskInfo.Status = model.TaskStatusSubmitted
	case "processing":
		taskInfo.Status = model.TaskStatusInProgress
	case "succeed":
		taskInfo.Status = model.TaskStatusSuccess
		if videos := resPayload.Data.TaskResult.Videos; len(videos) > 0 {
			video := videos[0]
			taskInfo.Url = video.Url
		}
		// Range errors carry +/-Inf and still require saturation auditing;
		// syntax errors contain no usable deduction and keep the precharge.
		if tokens, err := strconv.ParseFloat(resPayload.Data.FinalUnitDeduction, 64); err == nil || errors.Is(err, strconv.ErrRange) {
			// 上游返回的扣费数值，饱和转换防止超大数值回绕成负数
			rounded, clamp := common.QuotaFromFloatChecked(math.Ceil(tokens))
			taskInfo.QuotaClamp = clamp
			if rounded > 0 {
				taskInfo.CompletionTokens = rounded
				taskInfo.TotalTokens = rounded
			}
		}
	case "failed":
		taskInfo.Status = model.TaskStatusFailure
	default:
		return nil, fmt.Errorf("unknown task status: %s", status)
	}
	return taskInfo, nil
}

func isNewAPIRelay(apiKey string) bool {
	return strings.HasPrefix(apiKey, "sk-")
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var klingResp responsePayload
	if err := common.Unmarshal(originTask.Data, &klingResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal kling task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = klingResp.Data.CreatedAt
	openAIVideo.CompletedAt = klingResp.Data.UpdatedAt

	if len(klingResp.Data.TaskResult.Videos) > 0 {
		video := klingResp.Data.TaskResult.Videos[0]
		if video.Url != "" {
			openAIVideo.SetMetadata("url", video.Url)
		}
		if video.Duration != "" {
			openAIVideo.Seconds = video.Duration
		}
	}

	if klingResp.Code != 0 && klingResp.Message != "" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: klingResp.Message,
			Code:    fmt.Sprintf("%d", klingResp.Code),
		}
	}

	// https://app.klingai.com/cn/dev/document-api/apiReference/model/textToVideo
	if data := klingResp.Data; data.TaskStatus == "failed" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: data.TaskStatusMsg,
		}
	}
	return common.Marshal(openAIVideo)
}
