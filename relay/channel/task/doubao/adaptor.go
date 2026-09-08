package doubao

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/ForceMind/MyAPI/common"

	"github.com/ForceMind/MyAPI/constant"
	taskdto "github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relay/channel"
	"github.com/ForceMind/MyAPI/relay/channel/task/taskcommon"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string    `json:"type,omitempty"`
	Text     string    `json:"text,omitempty"`
	ImageURL *MediaURL `json:"image_url,omitempty"`
	VideoURL *MediaURL `json:"video_url,omitempty"`
	AudioURL *MediaURL `json:"audio_url,omitempty"`
	Role     string    `json:"role,omitempty"`
}

type MediaURL struct {
	URL string `json:"url,omitempty"`
}

type requestPayload struct {
	Model                 string         `json:"model"`
	Content               []ContentItem  `json:"content,omitempty"`
	CallbackURL           string         `json:"callback_url,omitempty"`
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame,omitempty"`
	ServiceTier           string         `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after,omitempty"`
	GenerateAudio         *dto.BoolValue `json:"generate_audio,omitempty"`
	Draft                 *dto.BoolValue `json:"draft,omitempty"`
	Tools                 []struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`
	SafetyIdentifier string         `json:"safety_identifier,omitempty"`
	Priority         *dto.IntValue  `json:"priority,omitempty"`
	Resolution       string         `json:"resolution,omitempty"`
	Ratio            string         `json:"ratio,omitempty"`
	Duration         *dto.IntValue  `json:"duration,omitempty"`
	Frames           *dto.IntValue  `json:"frames,omitempty"`
	Seed             *dto.IntValue  `json:"seed,omitempty"`
	CameraFixed      *dto.BoolValue `json:"camera_fixed,omitempty"`
	Watermark        *dto.BoolValue `json:"watermark,omitempty"`
}

type responseTask struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Seed            int    `json:"seed"`
	Resolution      string `json:"resolution"`
	Duration        int    `json:"duration"`
	Ratio           string `json:"ratio"`
	FramesPerSecond int    `json:"framespersecond"`
	ServiceTier     string `json:"service_tier"`
	Tools           []struct {
		Type string `json:"type"`
	} `json:"tools"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		ToolUsage        struct {
			WebSearch int `json:"web_search"`
		} `json:"tool_usage"`
	} `json:"usage"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
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
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	// Accept only POST /v1/video/generations as "generate" action.
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// EstimateBilling 根据请求 metadata 中的输出分辨率与是否包含视频输入，返回相对基准价的计费 OtherRatio。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	hasVideo := hasVideoInMetadata(req.Metadata)
	resolution, _ := req.Metadata["resolution"].(string)
	ratio, ok := GetVideoInputRatio(info.OriginModelName, resolution, hasVideo)
	if !ok || ratio == 1.0 {
		return nil
	}
	return map[string]float64{"video_input": ratio}
}

// hasVideoInMetadata 直接检查 metadata 的 content 数组是否包含 video_url 条目，
// 避免构建完整的上游 requestPayload。
func hasVideoInMetadata(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	contentRaw, ok := metadata["content"]
	if !ok {
		return false
	}
	contentSlice, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range contentSlice {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] == "video_url" {
			return true
		}
		if _, has := itemMap["video_url"]; has {
			return true
		}
	}
	return false
}

// BuildRequestBody converts request into Doubao specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	if info.IsModelMapped {
		body.Model = info.UpstreamModelName
	} else {
		info.UpstreamModelName = body.Model
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	if resp == nil || resp.Body == nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("invalid Doubao task submission response"),
			"invalid_response",
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, channel.MaxTaskSubmitResponseBytes+1))
	if err != nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("failed to read Doubao task submission response"),
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
			fmt.Errorf("invalid Doubao task submission parse result"),
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
			fmt.Errorf("invalid Doubao task submission response context"),
			"invalid_response",
			http.StatusInternalServerError,
		)
	}
	c.Data(result.LegacyResponse.StatusCode, result.LegacyResponse.ContentType, result.LegacyResponse.Body)
	return result.LegacyPollingID, result.TaskData, nil
}

// doubaoTaskSubmitPersistence deliberately excludes the upstream task ID and
// every provider response field. Polling replaces Task.Data with task state.
type doubaoTaskSubmitPersistence struct{}

// ParseTaskSubmitResponse interprets one complete Doubao submission response.
// It has no dependency on Gin, persistence, billing, clocks, or network I/O.
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
		return unknown("invalid_response", "invalid Doubao task submission response", http.StatusInternalServerError)
	}

	var responseObject map[string]json.RawMessage
	if err := common.Unmarshal(input.Body, &responseObject); err != nil || responseObject == nil {
		return unknown("unmarshal_response_body_failed", "invalid Doubao task submission response", http.StatusInternalServerError)
	}
	idBody, idPresent := responseObject["id"]
	if idPresent {
		var taskID string
		if err := common.Unmarshal(idBody, &taskID); err == nil && isSafeDoubaoTaskID(taskID) {
			providerTaskID = taskID
		}
	}
	if input.HTTPStatus != http.StatusOK {
		return unknown("unverified_response", "unverified Doubao task submission response", http.StatusBadGateway)
	}
	// A documented successful submission is an HTTP 200 envelope with an id.
	// Error and code members are contradictory evidence and cannot be accepted.
	if _, hasError := responseObject["error"]; hasError {
		return unknown("unverified_response", "Doubao returned conflicting task submission evidence", http.StatusInternalServerError)
	}
	if _, hasCode := responseObject["code"]; hasCode {
		return unknown("unverified_response", "Doubao returned conflicting task submission evidence", http.StatusInternalServerError)
	}
	if !idPresent || providerTaskID == "" {
		return unknown("invalid_response", "invalid Doubao task submission response", http.StatusInternalServerError)
	}

	taskData, err := common.Marshal(doubaoTaskSubmitPersistence{})
	if err != nil {
		return unknown("marshal_task_data_failed", "failed to build Doubao task submission data", http.StatusInternalServerError)
	}
	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = input.PublicTaskID
	openAIResp.TaskID = input.PublicTaskID
	openAIResp.Model = input.OriginModelName
	openAIResp.CreatedAt = input.SubmittedAtUnix
	legacyBody, err := common.Marshal(openAIResp)
	if err != nil {
		return unknown("marshal_legacy_response_failed", "failed to build Doubao task submission response", http.StatusInternalServerError)
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

func isSafeDoubaoTaskID(taskID string) bool {
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

func buildDoubaoTaskFetchURL(baseURL, taskID string) (string, error) {
	if !isSafeDoubaoTaskID(taskID) {
		return "", fmt.Errorf("invalid task_id")
	}
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseURL, taskID), nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri, err := buildDoubaoTaskFetchURL(baseUrl, taskID)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	r := requestPayload{
		Model:   req.Model,
		Content: []ContentItem{},
	}

	// Add images if present
	if req.HasImage() {
		for _, imgURL := range req.Images {
			r.Content = append(r.Content, ContentItem{
				Type: "image_url",
				ImageURL: &MediaURL{
					URL: imgURL,
				},
			})
		}
	}

	metadata := req.Metadata
	if err := taskcommon.UnmarshalMetadata(metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}

	if sec, _ := strconv.Atoi(req.Seconds); sec > 0 {
		r.Duration = lo.ToPtr(dto.IntValue(sec))
	}

	r.Content = lo.Reject(r.Content, func(c ContentItem, _ int) bool { return c.Type == "text" })
	r.Content = append(r.Content, ContentItem{
		Type: "text",
		Text: req.Prompt,
	})

	return &r, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// Map Doubao status to internal status
	switch resTask.Status {
	case "pending", "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = resTask.Content.VideoURL
		// 解析 usage 信息用于按倍率计费
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
	default:
		// Unknown status, treat as processing
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var dResp responseTask
	if err := common.Unmarshal(originTask.Data, &dResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal doubao task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.SetMetadata("url", dResp.Content.VideoURL)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	if dResp.Status == "failed" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: dResp.Error.Message,
			Code:    dResp.Error.Code,
		}
	}

	return common.Marshal(openAIVideo)
}
