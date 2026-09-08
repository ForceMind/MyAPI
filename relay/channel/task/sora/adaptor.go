package sora

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relay/channel"
	taskcommon "github.com/ForceMind/MyAPI/relay/channel/task/taskcommon"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string    `json:"type"`                // "text" or "image_url"
	Text     string    `json:"text,omitempty"`      // for text type
	ImageURL *ImageURL `json:"image_url,omitempty"` // for image_url type
}

type ImageURL struct {
	URL string `json:"url"`
}

type responseTask struct {
	ID                 string `json:"id"`
	TaskID             string `json:"task_id,omitempty"` //兼容旧接口
	Object             string `json:"object"`
	Model              string `json:"model"`
	Status             string `json:"status"`
	Progress           int    `json:"progress"`
	CreatedAt          int64  `json:"created_at"`
	CompletedAt        int64  `json:"completed_at,omitempty"`
	ExpiresAt          int64  `json:"expires_at,omitempty"`
	Seconds            string `json:"seconds,omitempty"`
	Size               string `json:"size,omitempty"`
	RemixedFromVideoID string `json:"remixed_from_video_id,omitempty"`
	Error              *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

type soraTaskSubmitPersistence struct {
	Object             string `json:"object"`
	Model              string `json:"model"`
	Status             string `json:"status"`
	Progress           int    `json:"progress"`
	CreatedAt          int64  `json:"created_at"`
	CompletedAt        int64  `json:"completed_at,omitempty"`
	ExpiresAt          int64  `json:"expires_at,omitempty"`
	Seconds            string `json:"seconds,omitempty"`
	Size               string `json:"size,omitempty"`
	RemixedFromVideoID string `json:"remixed_from_video_id,omitempty"`
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

func validateRemixRequest(c *gin.Context) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	// 存储原始请求到 context，与 ValidateMultipartDirect 路径保持一致
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if info.Action == constant.TaskActionRemix {
		return validateRemixRequest(c)
	}
	return relaycommon.ValidateMultipartDirect(c, info)
}

// EstimateBilling 根据用户请求的 seconds 和 size 计算 OtherRatios。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	// remix 路径的 OtherRatios 已在 ResolveOriginTask 中设置
	if info.Action == constant.TaskActionRemix {
		return nil
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	seconds, _ := strconv.Atoi(req.Seconds)
	if seconds == 0 {
		seconds = req.Duration
	}
	if seconds <= 0 {
		seconds = 4
	}

	size := req.Size
	if size == "" {
		size = "720x1280"
	}

	ratios := map[string]float64{
		"seconds": float64(seconds),
		"size":    1,
	}
	if size == "1792x1024" || size == "1024x1792" {
		ratios["size"] = 1.666667
	}
	return ratios
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.Action == constant.TaskActionRemix {
		return fmt.Sprintf("%s/v1/videos/%s/remix", a.baseURL, info.OriginTaskID), nil
	}
	return fmt.Sprintf("%s/v1/videos", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}
	cachedBody, err := storage.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "read_body_bytes_failed")
	}
	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "application/json") {
		var bodyMap map[string]interface{}
		if err := common.Unmarshal(cachedBody, &bodyMap); err == nil {
			bodyMap["model"] = info.UpstreamModelName
			if newBody, err := common.Marshal(bodyMap); err == nil {
				return bytes.NewReader(newBody), nil
			}
		}
		return bytes.NewReader(cachedBody), nil
	}

	if strings.Contains(contentType, "multipart/form-data") {
		formData, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return bytes.NewReader(cachedBody), nil
		}
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		writer.WriteField("model", info.UpstreamModelName)
		for key, values := range formData.Value {
			if key == "model" {
				continue
			}
			for _, v := range values {
				writer.WriteField(key, v)
			}
		}
		for fieldName, fileHeaders := range formData.File {
			for _, fh := range fileHeaders {
				f, err := fh.Open()
				if err != nil {
					continue
				}
				ct := fh.Header.Get("Content-Type")
				if ct == "" || ct == "application/octet-stream" {
					buf512 := make([]byte, 512)
					n, _ := io.ReadFull(f, buf512)
					ct = http.DetectContentType(buf512[:n])
					// Re-open after sniffing so the full content is copied below
					f.Close()
					f, err = fh.Open()
					if err != nil {
						continue
					}
				}
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fh.Filename))
				h.Set("Content-Type", ct)
				part, err := writer.CreatePart(h)
				if err != nil {
					f.Close()
					continue
				}
				io.Copy(part, f)
				f.Close()
			}
		}
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return &buf, nil
	}

	return common.NewReplayableBodyReader(storage), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	if resp == nil || resp.Body == nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("invalid Sora task submission response"),
			"invalid_response",
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, channel.MaxTaskSubmitResponseBytes+1))
	if err != nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("failed to read Sora task submission response"),
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
			parseInput.OriginPublicTaskID = info.OriginTaskID
		}
	}
	result := a.ParseTaskSubmitResponse(parseInput)
	if err := result.Validate(); err != nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("invalid Sora task submission parse result"),
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
			fmt.Errorf("invalid Sora task submission response context"),
			"invalid_response",
			http.StatusInternalServerError,
		)
	}
	c.Data(result.LegacyResponse.StatusCode, result.LegacyResponse.ContentType, result.LegacyResponse.Body)
	return result.LegacyPollingID, result.TaskData, nil
}

// ParseTaskSubmitResponse interprets one complete Sora submission response.
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
		return unknown("invalid_response", "invalid Sora task submission response", http.StatusInternalServerError)
	}

	var responseObject map[string]json.RawMessage
	if err := common.Unmarshal(input.Body, &responseObject); err != nil || responseObject == nil {
		return unknown("unmarshal_response_body_failed", "invalid Sora task submission response", http.StatusInternalServerError)
	}
	id, idPresent, idValid := parseSoraTaskSubmitID(responseObject["id"])
	legacyID, legacyIDPresent, legacyIDValid := parseSoraTaskSubmitID(responseObject["task_id"])
	if !idValid || !legacyIDValid {
		return unknown("invalid_response", "invalid Sora task submission response", http.StatusInternalServerError)
	}
	if idPresent && legacyIDPresent && id != legacyID {
		return unknown("unverified_response", "Sora returned conflicting task submission identifiers", http.StatusBadGateway)
	}
	if idPresent {
		providerTaskID = id
	} else if legacyIDPresent {
		providerTaskID = legacyID
	}
	if input.HTTPStatus != http.StatusOK {
		return unknown("unverified_response", "unverified Sora task submission response", http.StatusBadGateway)
	}
	if errorBody, hasError := responseObject["error"]; hasError && string(bytes.TrimSpace(errorBody)) != "null" {
		return unknown("unverified_response", "Sora returned conflicting task submission evidence", http.StatusBadGateway)
	}

	var response responseTask
	if err := common.Unmarshal(input.Body, &response); err != nil {
		return unknown("unmarshal_response_body_failed", "invalid Sora task submission response", http.StatusInternalServerError)
	}
	if !isSafeSoraTaskID(providerTaskID) || !isSoraSubmissionStatus(response.Status) {
		return unknown("invalid_response", "invalid Sora task submission response", http.StatusInternalServerError)
	}
	remixedFromVideoID := ""
	if isSafeSoraPublicTaskID(input.OriginPublicTaskID) {
		remixedFromVideoID = input.OriginPublicTaskID
	}

	persistedResponse := soraTaskSubmitPersistence{
		Object:             response.Object,
		Model:              response.Model,
		Status:             response.Status,
		Progress:           response.Progress,
		CreatedAt:          response.CreatedAt,
		CompletedAt:        response.CompletedAt,
		ExpiresAt:          response.ExpiresAt,
		Seconds:            response.Seconds,
		Size:               response.Size,
		RemixedFromVideoID: remixedFromVideoID,
	}
	taskData, err := common.Marshal(persistedResponse)
	if err != nil {
		return unknown("marshal_task_data_failed", "failed to build Sora task submission data", http.StatusInternalServerError)
	}
	legacyResponse := responseTask{
		ID:                 input.PublicTaskID,
		TaskID:             input.PublicTaskID,
		Object:             persistedResponse.Object,
		Model:              persistedResponse.Model,
		Status:             persistedResponse.Status,
		Progress:           persistedResponse.Progress,
		CreatedAt:          persistedResponse.CreatedAt,
		CompletedAt:        persistedResponse.CompletedAt,
		ExpiresAt:          persistedResponse.ExpiresAt,
		Seconds:            persistedResponse.Seconds,
		Size:               persistedResponse.Size,
		RemixedFromVideoID: persistedResponse.RemixedFromVideoID,
	}
	legacyBody, err := common.Marshal(legacyResponse)
	if err != nil {
		return unknown("marshal_legacy_response_failed", "failed to build Sora task submission response", http.StatusInternalServerError)
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

func parseSoraTaskSubmitID(taskIDBody json.RawMessage) (string, bool, bool) {
	if taskIDBody == nil {
		return "", false, true
	}
	var taskID string
	if err := common.Unmarshal(taskIDBody, &taskID); err != nil || !isSafeSoraTaskID(taskID) {
		return "", true, false
	}
	return taskID, true, true
}

func isSoraSubmissionStatus(status string) bool {
	switch status {
	case "queued", "pending", "processing", "in_progress":
		return true
	default:
		return false
	}
}

func isSafeSoraTaskID(taskID string) bool {
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

func isSafeSoraPublicTaskID(taskID string) bool {
	return strings.HasPrefix(taskID, "task_") && isSafeSoraTaskID(taskID)
}

func buildSoraTaskFetchURL(baseURL, taskID string) (string, error) {
	if !isSafeSoraTaskID(taskID) {
		return "", fmt.Errorf("invalid task_id")
	}
	return fmt.Sprintf("%s/v1/videos/%s", baseURL, url.PathEscape(taskID)), nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri, err := buildSoraTaskFetchURL(baseUrl, taskID)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

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

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	switch resTask.Status {
	case "queued", "pending":
		taskResult.Status = model.TaskStatusQueued
	case "processing", "in_progress":
		taskResult.Status = model.TaskStatusInProgress
	case "completed":
		taskResult.Status = model.TaskStatusSuccess
		// Url intentionally left empty — the caller constructs the proxy URL using the public task ID
	case "failed", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		if resTask.Error != nil {
			taskResult.Reason = resTask.Error.Message
		} else {
			taskResult.Reason = "task failed"
		}
	default:
	}
	if resTask.Progress > 0 && resTask.Progress < 100 {
		taskResult.Progress = fmt.Sprintf("%d%%", resTask.Progress)
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	data := task.Data
	var err error
	if data, err = sjson.SetBytes(data, "id", task.TaskID); err != nil {
		return nil, errors.Wrap(err, "set id failed")
	}
	return data, nil
}
