package service

import (
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
)

const (
	taskSubmissionJSONContentFamily = "application/json"
	taskSubmissionNoRouteBinding    = "none"
)

var (
	ErrTaskSubmissionFingerprintProtocol    = errors.New("task submission request fingerprint protocol is invalid")
	ErrTaskSubmissionFingerprintContentType = errors.New("task submission request fingerprint content type is unsupported")
	ErrTaskSubmissionFingerprintRoute       = errors.New("task submission request fingerprint route binding is invalid")
	ErrTaskSubmissionFingerprintBody        = errors.New("task submission request fingerprint JSON body is invalid")
	ErrTaskSubmissionFingerprintSecret      = errors.New("task submission request fingerprint secret is unavailable")
)

// TaskSubmissionFingerprintInput is the pure B2 request-fingerprint boundary.
// OriginTaskPublicID is required only for video.remix and prohibited for every
// other v1 operation. Body must be a JSON object; form, multipart, and binary
// request families require separate future canonicalization contracts.
type TaskSubmissionFingerprintInput struct {
	TokenID            int
	Protocol           *TaskSubmissionProtocol
	ContentType        string
	Body               []byte
	OriginTaskPublicID string
}

// FingerprintTaskSubmissionJSONRequest canonicalizes and fingerprints one
// validated JSON task request without returning canonical body bytes. It has no
// router, middleware, database, billing, dispatch, or network side effects.
func FingerprintTaskSubmissionJSONRequest(input TaskSubmissionFingerprintInput) (string, error) {
	if !validTaskSubmissionFingerprintProtocol(input.TokenID, input.Protocol) {
		return "", ErrTaskSubmissionFingerprintProtocol
	}
	if !validTaskSubmissionJSONContentType(input.ContentType) {
		return "", ErrTaskSubmissionFingerprintContentType
	}

	routeBinding, err := taskSubmissionFingerprintRouteBinding(input.Protocol.OperationKind, input.OriginTaskPublicID)
	if err != nil {
		return "", err
	}
	bodyDigest, err := common.CanonicalJSONObjectDigest(input.Body)
	if err != nil {
		return "", ErrTaskSubmissionFingerprintBody
	}
	fingerprint, err := common.HashTaskRecoveryRequestFingerprint(common.TaskRecoveryRequestFingerprintMaterial{
		TokenID:             int64(input.TokenID),
		HTTPMethod:          input.Protocol.HTTPMethod,
		OperationKind:       input.Protocol.OperationKind,
		RouteBinding:        routeBinding,
		ContentFamily:       taskSubmissionJSONContentFamily,
		CanonicalBodyDigest: bodyDigest,
	})
	if err != nil {
		return "", ErrTaskSubmissionFingerprintSecret
	}
	return fingerprint, nil
}

func validTaskSubmissionFingerprintProtocol(tokenID int, protocol *TaskSubmissionProtocol) bool {
	return tokenID > 0 && int64(tokenID) <= 1<<31-1 && protocol != nil &&
		protocol.HTTPMethod == http.MethodPost &&
		validTaskSubmissionProtocolOperationKind(protocol.OperationKind) &&
		validTaskSubmissionProtocolDigest(protocol.IdempotencyKeyHash)
}

func validTaskSubmissionJSONContentType(contentType string) bool {
	for index := 0; index < len(contentType); index++ {
		if contentType[index] != '\t' && (contentType[index] < 0x20 || contentType[index] > 0x7e) {
			return false
		}
	}
	raw := strings.Trim(contentType, " \t")
	parts := strings.Split(raw, ";")
	if len(parts) == 0 || len(parts) > 2 || !strings.EqualFold(strings.Trim(parts[0], " \t"), taskSubmissionJSONContentFamily) {
		return false
	}
	wantParameters := 0
	if len(parts) == 2 {
		parameter := strings.Trim(parts[1], " \t")
		separator := strings.IndexByte(parameter, '=')
		if separator <= 0 || separator != strings.LastIndexByte(parameter, '=') {
			return false
		}
		name := strings.Trim(parameter[:separator], " \t")
		if strings.Contains(name, "*") || !strings.EqualFold(name, "charset") {
			return false
		}
		wantParameters = 1
	}

	mediaType, parameters, err := mime.ParseMediaType(raw)
	if err != nil || mediaType != taskSubmissionJSONContentFamily || len(parameters) != wantParameters {
		return false
	}
	if wantParameters == 1 && !strings.EqualFold(parameters["charset"], "utf-8") {
		return false
	}
	return true
}

func taskSubmissionFingerprintRouteBinding(operationKind, originTaskPublicID string) (string, error) {
	if operationKind != model.TaskSubmissionOperationKindVideoRemix {
		if originTaskPublicID != "" {
			return "", ErrTaskSubmissionFingerprintRoute
		}
		return taskSubmissionNoRouteBinding, nil
	}
	if !model.ValidTaskSubmissionPublicID(originTaskPublicID) {
		return "", ErrTaskSubmissionFingerprintRoute
	}
	return "origin-task:" + originTaskPublicID, nil
}
