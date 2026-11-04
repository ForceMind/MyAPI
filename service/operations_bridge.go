package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"encoding/json"

	"github.com/ForceMind/MyAPI/common"
)

const OperationsApprovalScope = "operations.upgrade.approve"

var operationsIdentifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
var operationsPlanID = regexp.MustCompile(`^[a-f0-9]{32,64}$`)
var operationsDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var operationsCode = regexp.MustCompile(`^OPERATIONS_[A-Z0-9_]{1,80}$`)
var operationsSHA = regexp.MustCompile(`^[a-f0-9]{40}$`)
var operationsGitHubID = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
var operationsVersion = regexp.MustCompile(`^0\.2\.0-beta\.[1-9][0-9]{0,8}$`)
var operationsMachineCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,100}$`)

type OperationsError struct {
	Status int
	Code   string
}

func (err *OperationsError) Error() string { return err.Code }

func ValidOperationsPlan(id, digest string) bool {
	return operationsPlanID.MatchString(id) && operationsDigest.MatchString(digest)
}

func OperationsApprovalScopeFor(id, digest string) string {
	if !ValidOperationsPlan(id, digest) {
		return ""
	}
	return OperationsApprovalScope + ":" + id + ":" + digest
}

func IsOperationsApprovalScope(scope string) bool {
	parts := strings.Split(scope, ":")
	return len(parts) == 3 && parts[0] == OperationsApprovalScope && ValidOperationsPlan(parts[1], parts[2])
}

// OperationsBridge has a fixed Unix transport and no caller-controlled URL,
// headers or method. Credentials and raw daemon errors never leave this layer.
func OperationsBridge(ctx context.Context, actorID int, action, planID, digest string) (json.RawMessage, error) {
	unavailable := &OperationsError{Status: http.StatusServiceUnavailable, Code: "OPERATIONS_UNAVAILABLE"}
	socket := os.Getenv("MYAPI_OPERATIONS_SOCKET")
	credentialFile := os.Getenv("MYAPI_OPERATIONS_BRIDGE_TOKEN_FILE")
	instanceID := os.Getenv("MYAPI_OPERATIONS_INSTANCE_ID")
	if actorID <= 0 || !filepath.IsAbs(socket) || !filepath.IsAbs(credentialFile) || !operationsIdentifier.MatchString(instanceID) {
		return nil, unavailable
	}
	info, err := os.Lstat(credentialFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() < 32 || info.Size() > 256 {
		return nil, unavailable
	}
	file, err := os.Open(credentialFile)
	if err != nil {
		return nil, unavailable
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, unavailable
	}
	credential, err := io.ReadAll(io.LimitReader(file, 257))
	if err != nil || len(credential) > 256 {
		return nil, unavailable
	}
	secret := strings.TrimSpace(string(credential))
	if len(secret) < 32 || strings.ContainsAny(secret, "\r\n\t ") {
		return nil, unavailable
	}
	method, path := http.MethodGet, "/v1/plans"
	var body []byte
	switch action {
	case "plans":
	case "instances":
		path = "/v1/instances"
	case "plan", "approve":
		if !operationsPlanID.MatchString(planID) {
			return nil, &OperationsError{Status: http.StatusBadRequest, Code: "OPERATIONS_INVALID_PLAN"}
		}
		path += "/" + planID
		if action == "approve" {
			if !ValidOperationsPlan(planID, digest) {
				return nil, &OperationsError{Status: http.StatusBadRequest, Code: "OPERATIONS_INVALID_PLAN"}
			}
			method, path = http.MethodPost, path+"/approve"
			body, err = common.Marshal(map[string]string{"digest": digest})
			if err != nil {
				return nil, unavailable
			}
		}
	default:
		return nil, unavailable
	}
	transport := &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socket)
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return errors.New("operations redirect forbidden")
	}}
	request, err := http.NewRequestWithContext(ctx, method, "http://operations.local"+path, bytes.NewReader(body))
	if err != nil {
		return nil, unavailable
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	request.Header.Set("X-MyAPI-Actor-ID", strconv.Itoa(actorID))
	request.Header.Set("X-MyAPI-Instance-ID", instanceID)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, unavailable
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 {
		return nil, unavailable
	}
	var envelope struct {
		Success bool            `json:"success"`
		Code    string          `json:"code"`
		Data    json.RawMessage `json:"data"`
	}
	if common.Unmarshal(data, &envelope) != nil {
		return nil, unavailable
	}
	if response.StatusCode != http.StatusOK || !envelope.Success || len(envelope.Data) == 0 {
		if response.StatusCode >= 400 && response.StatusCode <= 503 && operationsCode.MatchString(envelope.Code) {
			return nil, &OperationsError{Status: response.StatusCode, Code: envelope.Code}
		}
		return nil, unavailable
	}
	// The private daemon is part of this release. Refuse unexpected primitive
	// payloads; the daemon itself constructs the public, secret-free DTOs.
	trimmed := bytes.TrimSpace(envelope.Data)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return nil, unavailable
	}
	safe, err := safeOperationsData(action, envelope.Data)
	if err != nil {
		return nil, unavailable
	}
	return safe, nil
}
