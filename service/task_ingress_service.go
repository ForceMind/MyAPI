package service

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/model"
	"gorm.io/gorm"
)

// TaskIngressRequest encapsulates the parsed HTTP parameters and payload
// required to process a durable task submission ingress.
type TaskIngressRequest struct {
	UserID             int
	TokenID            int
	ChannelID          int
	Provider           string
	OperationKind      string
	OriginTaskPublicID string
	HTTPMethod         string
	Header             http.Header
	ContentType        string
	Body               []byte
	RequestID          string
	EstimatedQuota     int
	FreeModel          bool
	InitialQuotaClamp  *common.QuotaClamp
	BillingContext     model.TaskBillingContext
	Dispatcher         TaskProviderDispatcher
	DB                 *gorm.DB
}

// TaskIngressResult represents the outcome of durable ingress processing.
type TaskIngressResult struct {
	IsReplay  bool
	Response  *dto.TaskOperationResponse
	Conflict  bool
	ErrorCode string
	ErrorMsg  string
}

// TaskIngressService provides durable task ingress lifecycle execution.
type TaskIngressService struct {
	DB *gorm.DB
}

// NewTaskIngressService creates a new TaskIngressService instance.
func NewTaskIngressService(db *gorm.DB) *TaskIngressService {
	return &TaskIngressService{DB: db}
}

// Execute executes task ingress on the service instance.
func (s *TaskIngressService) Execute(ctx context.Context, req TaskIngressRequest) (*TaskIngressResult, error) {
	if req.DB == nil {
		req.DB = s.DB
	}
	return ExecuteTaskIngress(ctx, req)
}

// DetectTaskOperationKind classifies incoming HTTP requests into durable task
// operation kinds and extracts route-bound parameters (such as origin task ID).
func DetectTaskOperationKind(method, path string, params map[string]string) (operationKind string, originTaskPublicID string, ok bool) {
	if strings.ToUpper(strings.TrimSpace(method)) != http.MethodPost {
		return "", "", false
	}

	cleanPath := strings.Split(strings.TrimSpace(path), "?")[0]
	cleanPath = strings.TrimRight(cleanPath, "/")

	// 1. Remix: POST + ends with /remix and contains /videos/
	if strings.HasSuffix(cleanPath, "/remix") && strings.Contains(cleanPath, "/videos/") {
		originID := ""
		if params != nil {
			for _, key := range []string{"video_id", "id", "origin_task_public_id", "origin_task_id", "task_id"} {
				if val, exists := params[key]; exists && strings.TrimSpace(val) != "" {
					originID = strings.TrimSpace(val)
					break
				}
			}
		}
		if originID == "" {
			parts := strings.Split(cleanPath, "/")
			for i := 0; i < len(parts); i++ {
				if parts[i] == "videos" && i+2 < len(parts) && parts[i+2] == "remix" {
					originID = parts[i+1]
					break
				}
			}
		}
		return model.TaskSubmissionOperationKindVideoRemix, originID, true
	}

	// 2. Video create: /video/generations, /videos, /kling/..., /jimeng/...
	if cleanPath == "/video/generations" || strings.HasSuffix(cleanPath, "/video/generations") ||
		cleanPath == "/videos" || strings.HasSuffix(cleanPath, "/videos") ||
		strings.HasPrefix(cleanPath, "/kling/") || strings.Contains(cleanPath, "/kling/") || cleanPath == "/kling" ||
		strings.HasPrefix(cleanPath, "/jimeng/") || strings.Contains(cleanPath, "/jimeng/") || cleanPath == "/jimeng" {
		return model.TaskSubmissionOperationKindVideoCreate, "", true
	}

	// 3. Suno music: /suno/submit/music
	if cleanPath == "/suno/submit/music" || strings.HasSuffix(cleanPath, "/suno/submit/music") {
		return model.TaskSubmissionOperationKindSunoMusic, "", true
	}

	// 4. Suno lyrics: /suno/submit/lyrics
	if cleanPath == "/suno/submit/lyrics" || strings.HasSuffix(cleanPath, "/suno/submit/lyrics") {
		return model.TaskSubmissionOperationKindSunoLyrics, "", true
	}

	return "", "", false
}

// ExecuteTaskIngress executes the complete durable task ingress flow:
// 1. Validates and extracts the Idempotency-Key from headers via ParseTaskSubmissionProtocol.
// 2. Selects the canonical fingerprint generator based on Content-Type (JSON, Form, Multipart).
// 3. Constructs operationCandidate and attemptCandidate, calling CreateOrLoadTaskSubmissionIntent.
// 4. If an idempotency fingerprint conflict occurs, returns Conflict=true (HTTP 409).
// 5. If replaying (!intent.Owner), immediately constructs and returns dto.TaskOperationResponse (IsReplay=true).
// 6. If a new submission (intent.Owner), runs ExecuteTaskSubmissionPipeline (T1 Reserve -> T2 Outbound -> Dispatcher -> T3 Outcome).
// 7. Constructs and returns the final dto.TaskOperationResponse.
func ExecuteTaskIngress(ctx context.Context, req TaskIngressRequest) (*TaskIngressResult, error) {
	db := req.DB
	if db == nil {
		db = model.DB
	}
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if req.UserID <= 0 || req.TokenID <= 0 || req.ChannelID <= 0 {
		return nil, fmt.Errorf("%w: user, token, and channel ids must be positive", ErrTaskSubmissionInvalidInput)
	}
	if strings.TrimSpace(req.Provider) == "" {
		return nil, fmt.Errorf("%w: provider is required", ErrTaskSubmissionInvalidInput)
	}
	if req.Dispatcher == nil {
		return nil, ErrTaskSubmissionDispatcherNil
	}
	if req.InitialQuotaClamp != nil {
		return nil, req.InitialQuotaClamp
	}

	// 1. Validate and strip Idempotency-Key from headers
	protocol, err := ParseTaskSubmissionProtocol(req.Header, req.HTTPMethod, req.OperationKind)
	if err != nil {
		return nil, err
	}

	// 2. Select corresponding fingerprint function based on Content-Type
	fpInput := TaskSubmissionFingerprintInput{
		TokenID:            req.TokenID,
		Protocol:           protocol,
		ContentType:        req.ContentType,
		Body:               req.Body,
		OriginTaskPublicID: req.OriginTaskPublicID,
	}

	fingerprint, err := fingerprintTaskSubmissionBody(fpInput)
	if err != nil {
		return nil, err
	}

	// 3. Construct operationCandidate & attemptCandidate, then call CreateOrLoadTaskSubmissionIntent
	operationCandidate := &model.TaskSubmissionOperation{
		UserID:             req.UserID,
		TokenID:            req.TokenID,
		HTTPMethod:         protocol.HTTPMethod,
		OperationKind:      protocol.OperationKind,
		IdempotencyKeyHash: protocol.IdempotencyKeyHash,
		RequestFingerprint: fingerprint,
		RequestID:          strings.TrimSpace(req.RequestID),
		Status:             model.TaskSubmissionOperationStatusPrepared,
	}

	attemptCandidate := &model.TaskSubmissionAttempt{
		AttemptNo:    1,
		Status:       model.TaskSubmissionAttemptStatusPrepared,
		ChannelID:    req.ChannelID,
		Provider:     req.Provider,
		RequestClass: taskSubmissionRequestClass(protocol.OperationKind),
	}

	intent, err := model.CreateOrLoadTaskSubmissionIntent(db, operationCandidate, attemptCandidate)
	// 4. Handle idempotency conflict (different body for identical key)
	if errors.Is(err, model.ErrTaskSubmissionIdempotencyConflict) {
		return &TaskIngressResult{
			Conflict:  true,
			ErrorCode: "idempotency_conflict",
			ErrorMsg:  err.Error(),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	if intent == nil || intent.Operation == nil {
		return nil, errors.New("task submission intent is unavailable")
	}

	// 5. Idempotent replay: return existing operation DTO without re-executing pipeline
	if !intent.Owner {
		return &TaskIngressResult{
			IsReplay: true,
			Response: buildTaskOperationResponse(intent.Operation),
		}, nil
	}

	// 6. New submission: execute full submission pipeline (T1 -> T2 -> Dispatcher -> T3)
	billingContext := req.BillingContext
	if !billingContext.Complete || billingContext.Version != model.TaskBillingContextVersion {
		originModel := req.OperationKind
		if originModel == "" {
			originModel = "task"
		}
		billingContext = model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			OriginModelName: originModel,
			ModelPrice:      float64(req.EstimatedQuota),
			ModelRatio:      1,
			GroupRatio:      1,
			PerCallBilling:  true,
		}
	}

	pipelineResult, err := ExecuteTaskSubmissionPipeline(ctx, TaskSubmissionPipelineInput{
		DB:             db,
		OperationID:    intent.Operation.ID,
		AttemptID:      intent.Attempt.ID,
		UserID:         req.UserID,
		TokenID:        req.TokenID,
		ChannelID:      req.ChannelID,
		Quota:          int64(req.EstimatedQuota),
		FreeModel:      req.FreeModel,
		BillingContext: billingContext,
		Dispatcher:     req.Dispatcher,
	})
	if err != nil {
		return nil, err
	}

	// 7. Construct final dto.TaskOperationResponse
	op := pipelineResult.Operation
	if op == nil {
		op = intent.Operation
	}
	res := &TaskIngressResult{
		IsReplay: false,
		Response: buildTaskOperationResponse(op),
	}
	if pipelineResult.DispatchResult != nil {
		res.ErrorCode = pipelineResult.DispatchResult.ErrorCode
		res.ErrorMsg = pipelineResult.DispatchResult.ErrorMessage
	}
	return res, nil
}

func fingerprintTaskSubmissionBody(input TaskSubmissionFingerprintInput) (string, error) {
	mediaType, _, _ := mime.ParseMediaType(strings.TrimSpace(input.ContentType))
	if mediaType == "" {
		mediaType = strings.ToLower(strings.TrimSpace(strings.Split(input.ContentType, ";")[0]))
	}

	switch mediaType {
	case "application/json":
		return FingerprintTaskSubmissionJSONRequest(input)
	case "application/x-www-form-urlencoded":
		return FingerprintTaskSubmissionFormRequest(input)
	case "multipart/form-data":
		return FingerprintTaskSubmissionMultipartRequest(input)
	default:
		return "", ErrTaskSubmissionFingerprintContentType
	}
}

func buildTaskOperationResponse(operation *model.TaskSubmissionOperation) *dto.TaskOperationResponse {
	if operation == nil {
		return nil
	}
	return &dto.TaskOperationResponse{
		ID:                operation.PublicID,
		Object:            dto.TaskOperationObject,
		Kind:              operation.OperationKind,
		Status:            string(operation.Status),
		CreatedAt:         operation.CreatedAt,
		UpdatedAt:         operation.UpdatedAt,
		DispatchStartedAt: operation.DispatchStartedAt,
		ResolvedAt:        operation.ResolvedAt,
	}
}

func taskSubmissionRequestClass(operationKind string) string {
	switch operationKind {
	case model.TaskSubmissionOperationKindVideoCreate, model.TaskSubmissionOperationKindVideoRemix:
		return "video"
	case model.TaskSubmissionOperationKindSunoMusic:
		return "music"
	case model.TaskSubmissionOperationKindSunoLyrics:
		return "lyrics"
	default:
		parts := strings.Split(operationKind, ".")
		if len(parts) > 0 && parts[0] != "" {
			return parts[0]
		}
		return "task"
	}
}
