package jimeng

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
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

type requestPayload struct {
	ReqKey           string   `json:"req_key"`
	BinaryDataBase64 []string `json:"binary_data_base64,omitempty"`
	ImageUrls        []string `json:"image_urls,omitempty"`
	Prompt           string   `json:"prompt,omitempty"`
	Seed             int64    `json:"seed"`
	AspectRatio      string   `json:"aspect_ratio"`
	Frames           int      `json:"frames,omitempty"`
}

// jimengTaskSubmitPersistence is the minimal safe snapshot retained until a
// polling response replaces task data. Upstream task identifiers, prompts,
// messages, and extension fields are intentionally excluded.
type jimengTaskSubmitPersistence struct {
	Code int `json:"code"`
}

type responseTask struct {
	Code int `json:"code"`
	Data struct {
		BinaryDataBase64 []interface{} `json:"binary_data_base64"`
		ImageUrls        interface{}   `json:"image_urls"`
		RespData         string        `json:"resp_data"`
		Status           string        `json:"status"`
		VideoUrl         string        `json:"video_url"`
	} `json:"data"`
	Message     string `json:"message"`
	RequestId   string `json:"request_id"`
	Status      int    `json:"status"`
	TimeElapsed string `json:"time_elapsed"`
}

const (
	// 即梦限制单个文件最大4.7MB https://www.volcengine.com/docs/85621/1747301
	MaxFileSize int64 = 4*1024*1024 + 700*1024 // 4.7MB (4MB + 724KB)
)

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	accessKey   string
	secretKey   string
	baseURL     string
}

var _ channel.TaskSubmitResponseParser = (*TaskAdaptor)(nil)

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl

	// apiKey format: "access_key|secret_key"
	keyParts := strings.Split(info.ApiKey, "|")
	if len(keyParts) == 2 {
		a.accessKey = strings.TrimSpace(keyParts[0])
		a.secretKey = strings.TrimSpace(keyParts[1])
	}
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if isNewAPIRelay(info.ApiKey) {
		return fmt.Sprintf("%s/jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31", a.baseURL), nil
	}
	return fmt.Sprintf("%s/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if isNewAPIRelay(info.ApiKey) {
		req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	} else {
		return a.signRequest(req, a.accessKey, a.secretKey)
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil, fmt.Errorf("invalid request type in context")
	}
	// 支持openai sdk的图片上传方式
	if mf, err := c.MultipartForm(); err == nil {
		if files, exists := mf.File["input_reference"]; exists && len(files) > 0 {
			if len(files) == 1 {
				info.Action = constant.TaskActionGenerate
			} else if len(files) > 1 {
				info.Action = constant.TaskActionFirstTailGenerate
			}

			// 将上传的文件转换为base64格式
			var images []string

			for _, fileHeader := range files {
				// 检查文件大小
				if fileHeader.Size > MaxFileSize {
					return nil, fmt.Errorf("文件 %s 大小超过限制，最大允许 %d MB", fileHeader.Filename, MaxFileSize/(1024*1024))
				}

				file, err := fileHeader.Open()
				if err != nil {
					continue
				}
				fileBytes, err := io.ReadAll(file)
				file.Close()
				if err != nil {
					continue
				}
				// 将文件内容转换为base64
				base64Str := base64.StdEncoding.EncodeToString(fileBytes)
				images = append(images, base64Str)
			}
			req.Images = images
		}
	}

	body, err := a.convertToRequestPayload(&req, info)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
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
			fmt.Errorf("invalid Jimeng task submission response"),
			"invalid_response",
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, channel.MaxTaskSubmitResponseBytes+1))
	if err != nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("failed to read Jimeng task submission response"),
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
			fmt.Errorf("invalid Jimeng task submission parse result"),
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
			fmt.Errorf("invalid Jimeng task submission response context"),
			"invalid_response",
			http.StatusInternalServerError,
		)
	}
	c.Data(result.LegacyResponse.StatusCode, result.LegacyResponse.ContentType, result.LegacyResponse.Body)
	return result.LegacyPollingID, result.TaskData, nil
}

// ParseTaskSubmitResponse interprets one complete Jimeng submission response.
// It is intentionally independent of Gin, HTTP writers, persistence, billing,
// clocks, and network activity.
func (_ *TaskAdaptor) ParseTaskSubmitResponse(input channel.TaskSubmitParseInput) channel.TaskSubmitParseResult {
	providerTaskID := ""
	upstreamRequestID := ""
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
	legacyProviderError := func(code int) channel.TaskSubmitParseResult {
		return unknown(strconv.Itoa(code), "Jimeng did not confirm the task submission", http.StatusInternalServerError)
	}
	if channel.IsValidTaskSubmitToken(input.UpstreamRequestID, channel.TaskSubmitUpstreamRequestIDMaxLength, false) {
		upstreamRequestID = input.UpstreamRequestID
	}
	if len(input.Body) == 0 || len(input.Body) > channel.MaxTaskSubmitResponseBytes || !utf8.Valid(input.Body) {
		return unknown("invalid_response", "invalid Jimeng task submission response", http.StatusBadGateway)
	}

	var responseObject map[string]json.RawMessage
	if err := common.Unmarshal(input.Body, &responseObject); err != nil || responseObject == nil {
		return unknown("unmarshal_response_body_failed", "invalid Jimeng task submission response", http.StatusBadGateway)
	}
	codeBody, ok := responseObject["code"]
	if !ok {
		return unknown("invalid_response", "invalid Jimeng task submission response", http.StatusBadGateway)
	}
	var code *int
	if err := common.Unmarshal(codeBody, &code); err != nil || code == nil {
		return unknown("invalid_response", "invalid Jimeng task submission response", http.StatusBadGateway)
	}
	if requestIDBody, exists := responseObject["request_id"]; exists {
		var requestID string
		if err := common.Unmarshal(requestIDBody, &requestID); err == nil &&
			channel.IsValidTaskSubmitToken(requestID, channel.TaskSubmitUpstreamRequestIDMaxLength, false) &&
			upstreamRequestID == "" {
			upstreamRequestID = requestID
		}
	}
	dataObject, dataPresent, dataObjectValid := parseJimengTaskSubmitData(responseObject["data"])
	if !dataObjectValid {
		return unknown("invalid_response", "invalid Jimeng task submission response", http.StatusBadGateway)
	}
	taskIDPresent := false
	if dataPresent {
		if taskIDBody, exists := dataObject["task_id"]; exists {
			taskIDPresent = true
			var taskID string
			if err := common.Unmarshal(taskIDBody, &taskID); err == nil && isSafeJimengTaskID(taskID) {
				providerTaskID = taskID
			}
		}
	}
	if input.HTTPStatus != http.StatusOK {
		return unknown("unverified_response", "unverified Jimeng task submission response", http.StatusBadGateway)
	}
	if *code != 10000 {
		return legacyProviderError(*code)
	}
	if !dataPresent || !taskIDPresent || providerTaskID == "" {
		return unknown("invalid_response", "invalid Jimeng task submission response", http.StatusBadGateway)
	}

	taskData, err := common.Marshal(jimengTaskSubmitPersistence{Code: *code})
	if err != nil {
		return unknown("marshal_task_data_failed", "failed to build Jimeng task submission data", http.StatusInternalServerError)
	}
	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = input.PublicTaskID
	openAIResp.TaskID = input.PublicTaskID
	openAIResp.Model = input.OriginModelName
	openAIResp.CreatedAt = input.SubmittedAtUnix
	legacyBody, err := common.Marshal(openAIResp)
	if err != nil {
		return unknown("marshal_legacy_response_failed", "failed to build Jimeng task submission response", http.StatusInternalServerError)
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

// parseJimengTaskSubmitData distinguishes an omitted or null data field from
// malformed non-object data. Only a verified success may require a task_id;
// an error envelope can legitimately omit data without proving no submission.
func parseJimengTaskSubmitData(dataBody json.RawMessage) (map[string]json.RawMessage, bool, bool) {
	if dataBody == nil || string(bytes.TrimSpace(dataBody)) == "null" {
		return nil, false, true
	}
	var dataObject map[string]json.RawMessage
	if err := common.Unmarshal(dataBody, &dataObject); err != nil || dataObject == nil {
		return nil, false, false
	}
	return dataObject, true, true
}

func isSafeJimengTaskID(taskID string) bool {
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

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || !isSafeJimengTaskID(taskID) {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/?Action=CVSync2AsyncGetResult&Version=2022-08-31", baseUrl)
	if isNewAPIRelay(key) {
		uri = fmt.Sprintf("%s/jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31", a.baseURL)
	}
	payload := map[string]string{
		"req_key": "jimeng_vgfm_t2v_l20", // This is fixed value from doc: https://www.volcengine.com/docs/85621/1544774
		"task_id": taskID,
	}
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal fetch task payload failed")
	}

	req, err := http.NewRequest(http.MethodPost, uri, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	if isNewAPIRelay(key) {
		req.Header.Set("Authorization", "Bearer "+key)
	} else {
		keyParts := strings.Split(key, "|")
		if len(keyParts) != 2 {
			return nil, fmt.Errorf("invalid api key format for jimeng: expected 'ak|sk'")
		}
		accessKey := strings.TrimSpace(keyParts[0])
		secretKey := strings.TrimSpace(keyParts[1])

		if err := a.signRequest(req, accessKey, secretKey); err != nil {
			return nil, errors.Wrap(err, "sign request failed")
		}
	}
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{"jimeng_vgfm_t2v_l20"}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "jimeng"
}

func (a *TaskAdaptor) signRequest(req *http.Request, accessKey, secretKey string) error {
	var bodyBytes []byte
	var err error

	if req.Body != nil {
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return errors.Wrap(err, "read request body failed")
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Rewind
	} else {
		bodyBytes = []byte{}
	}

	payloadHash := sha256.Sum256(bodyBytes)
	hexPayloadHash := hex.EncodeToString(payloadHash[:])

	t := time.Now().UTC()
	xDate := t.Format("20060102T150405Z")
	shortDate := t.Format("20060102")

	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Date", xDate)
	req.Header.Set("X-Content-Sha256", hexPayloadHash)

	// Sort and encode query parameters to create canonical query string
	queryParams := req.URL.Query()
	sortedKeys := make([]string, 0, len(queryParams))
	for k := range queryParams {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)
	var queryParts []string
	for _, k := range sortedKeys {
		values := queryParams[k]
		sort.Strings(values)
		for _, v := range values {
			queryParts = append(queryParts, fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(v)))
		}
	}
	canonicalQueryString := strings.Join(queryParts, "&")

	headersToSign := map[string]string{
		"host":             req.URL.Host,
		"x-date":           xDate,
		"x-content-sha256": hexPayloadHash,
	}
	if req.Header.Get("Content-Type") != "" {
		headersToSign["content-type"] = req.Header.Get("Content-Type")
	}

	var signedHeaderKeys []string
	for k := range headersToSign {
		signedHeaderKeys = append(signedHeaderKeys, k)
	}
	sort.Strings(signedHeaderKeys)

	var canonicalHeaders strings.Builder
	for _, k := range signedHeaderKeys {
		canonicalHeaders.WriteString(k)
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(strings.TrimSpace(headersToSign[k]))
		canonicalHeaders.WriteString("\n")
	}
	signedHeaders := strings.Join(signedHeaderKeys, ";")

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		req.Method,
		req.URL.Path,
		canonicalQueryString,
		canonicalHeaders.String(),
		signedHeaders,
		hexPayloadHash,
	)

	hashedCanonicalRequest := sha256.Sum256([]byte(canonicalRequest))
	hexHashedCanonicalRequest := hex.EncodeToString(hashedCanonicalRequest[:])

	region := "cn-north-1"
	serviceName := "cv"
	credentialScope := fmt.Sprintf("%s/%s/%s/request", shortDate, region, serviceName)
	stringToSign := fmt.Sprintf("HMAC-SHA256\n%s\n%s\n%s",
		xDate,
		credentialScope,
		hexHashedCanonicalRequest,
	)

	kDate := hmacSHA256([]byte(secretKey), []byte(shortDate))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	kSigning := hmacSHA256(kService, []byte("request"))
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	authorization := fmt.Sprintf("HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey,
		credentialScope,
		signedHeaders,
		signature,
	)
	req.Header.Set("Authorization", authorization)
	return nil
}

func hmacSHA256(key []byte, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*requestPayload, error) {
	r := requestPayload{
		ReqKey: info.UpstreamModelName,
		Prompt: req.Prompt,
	}

	switch req.Duration {
	case 10:
		r.Frames = 241 // 24*10+1 = 241
	default:
		r.Frames = 121 // 24*5+1 = 121
	}
	authoritativeFrames := r.Frames

	// Handle one-of image_urls or binary_data_base64
	if req.HasImage() {
		if strings.HasPrefix(req.Images[0], "http") {
			r.ImageUrls = req.Images
		} else {
			r.BinaryDataBase64 = req.Images
		}
	}
	metadata := make(map[string]any, len(req.Metadata))
	for key, value := range req.Metadata {
		switch key {
		case "model", "model_name", "req_key", "prompt", "frames":
			continue
		default:
			metadata[key] = value
		}
	}
	if err := taskcommon.UnmarshalMetadata(metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	// Routing and billing have already selected UpstreamModelName. Restore it
	// after provider metadata is applied so req_key cannot select another model.
	r.ReqKey = info.UpstreamModelName
	r.Prompt = req.Prompt
	r.Frames = authoritativeFrames

	// 即梦视频3.0 ReqKey转换
	// https://www.volcengine.com/docs/85621/1792707
	imageLen := lo.Max([]int{len(req.Images), len(r.BinaryDataBase64), len(r.ImageUrls)})
	if strings.Contains(r.ReqKey, "jimeng_v30") {
		if r.ReqKey == "jimeng_v30_pro" {
			// 3.0 pro只有固定的jimeng_ti2v_v30_pro
			r.ReqKey = "jimeng_ti2v_v30_pro"
		} else if imageLen > 1 {
			// 多张图片：首尾帧生成
			r.ReqKey = strings.TrimSuffix(strings.Replace(r.ReqKey, "jimeng_v30", "jimeng_i2v_first_tail_v30", 1), "p")
		} else if imageLen == 1 {
			// 单张图片：图生视频
			r.ReqKey = strings.TrimSuffix(strings.Replace(r.ReqKey, "jimeng_v30", "jimeng_i2v_first_v30", 1), "p")
		} else {
			// 无图片：文生视频
			r.ReqKey = strings.Replace(r.ReqKey, "jimeng_v30", "jimeng_t2v_v30", 1)
		}
	}

	return &r, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	taskResult := relaycommon.TaskInfo{}
	if resTask.Code == 10000 {
		taskResult.Code = 0
	} else {
		taskResult.Code = resTask.Code // todo uni code
		taskResult.Reason = resTask.Message
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
	}
	switch resTask.Data.Status {
	case "in_queue":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "done":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
	}
	taskResult.Url = resTask.Data.VideoUrl
	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var jimengResp responseTask
	if err := common.Unmarshal(originTask.Data, &jimengResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal jimeng task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.SetMetadata("url", jimengResp.Data.VideoUrl)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt

	if jimengResp.Code != 10000 {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: jimengResp.Message,
			Code:    fmt.Sprintf("%d", jimengResp.Code),
		}
	}

	return common.Marshal(openAIVideo)
}

func isNewAPIRelay(apiKey string) bool {
	return strings.HasPrefix(apiKey, "sk-")
}
