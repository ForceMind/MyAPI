package channel

import (
	"fmt"
	"mime"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ForceMind/MyAPI/common"
)

type TaskSubmitDisposition string

const (
	TaskSubmitAccepted TaskSubmitDisposition = "accepted"
	TaskSubmitRejected TaskSubmitDisposition = "rejected"
	TaskSubmitUnknown  TaskSubmitDisposition = "unknown"

	TaskSubmitProviderOperationIDMaxLength = 191
	TaskSubmitUpstreamRequestIDMaxLength   = 128
	TaskSubmitOutcomeCodeMaxLength         = 64
	TaskSubmitProblemMessageMaxLength      = 512
	TaskSubmitContentTypeMaxLength         = 128
	MaxTaskSubmitResponseBytes             = 1 << 20
)

// TaskSubmitParseInput contains only immutable values needed to interpret one
// upstream response. Time is supplied by the caller to keep parsers
// deterministic and free of process-global dependencies.
type TaskSubmitParseInput struct {
	HTTPStatus        int
	Body              []byte
	UpstreamRequestID string
	PublicTaskID      string
	// OriginPublicTaskID is an already-authorized local task identifier for a
	// derived request (for example, a video remix). Parsers must never replace
	// it with an upstream source identifier in a client-visible response.
	OriginPublicTaskID string
	OriginModelName    string
	ClientModelName    string
	SubmittedAtUnix    int64
}

// LegacyTaskSubmitResponse preserves the existing client response without
// giving a parser access to a writable HTTP boundary.
type LegacyTaskSubmitResponse struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

// TaskSubmitProblem is deliberately safe for client-facing legacy errors. It
// must contain a bounded diagnostic, never the raw upstream response body.
type TaskSubmitProblem struct {
	OutcomeCode string
	SafeMessage string
	StatusCode  int
	Local       bool
}

// TaskSubmitParseResult is the complete, side-effect-free interpretation of a
// single upstream submission response.
type TaskSubmitParseResult struct {
	Disposition         TaskSubmitDisposition
	ProviderOperationID string
	LegacyPollingID     string
	UpstreamRequestID   string
	TaskData            []byte
	LegacyResponse      *LegacyTaskSubmitResponse
	Problem             *TaskSubmitProblem
}

func (result TaskSubmitParseResult) Validate() error {
	if !IsValidTaskSubmitToken(result.UpstreamRequestID, TaskSubmitUpstreamRequestIDMaxLength, true) {
		return fmt.Errorf("upstream request id exceeds its boundary")
	}

	switch result.Disposition {
	case TaskSubmitAccepted:
		if !IsValidTaskSubmitToken(result.ProviderOperationID, TaskSubmitProviderOperationIDMaxLength, false) {
			return fmt.Errorf("accepted result requires a bounded provider operation id")
		}
		if !IsValidTaskSubmitToken(result.LegacyPollingID, TaskSubmitProviderOperationIDMaxLength, false) {
			return fmt.Errorf("accepted result requires a bounded legacy polling id")
		}
		if len(result.TaskData) == 0 || len(result.TaskData) > MaxTaskSubmitResponseBytes {
			return fmt.Errorf("accepted result requires bounded task data")
		}
		if !isTaskSubmitJSONObject(result.TaskData) {
			return fmt.Errorf("accepted result task data must be a JSON object")
		}
		if result.LegacyResponse == nil || result.Problem != nil {
			return fmt.Errorf("accepted result requires only a legacy success response")
		}
		if result.LegacyResponse.StatusCode < 200 || result.LegacyResponse.StatusCode >= 300 {
			return fmt.Errorf("legacy success response must use a 2xx status")
		}
		if !isSafeTaskSubmitJSONContentType(result.LegacyResponse.ContentType) ||
			len(result.LegacyResponse.Body) == 0 || len(result.LegacyResponse.Body) > MaxTaskSubmitResponseBytes {
			return fmt.Errorf("legacy success response is incomplete or exceeds its boundary")
		}
		if !isTaskSubmitJSONObject(result.LegacyResponse.Body) {
			return fmt.Errorf("legacy success response body must be a JSON object")
		}
	case TaskSubmitRejected, TaskSubmitUnknown:
		if result.LegacyResponse != nil || len(result.TaskData) != 0 || result.Problem == nil {
			return fmt.Errorf("non-accepted result requires only a safe problem")
		}
		if result.Disposition == TaskSubmitRejected && (result.ProviderOperationID != "" || result.LegacyPollingID != "") {
			return fmt.Errorf("rejected result cannot carry a provider operation id")
		}
		if !IsValidTaskSubmitToken(result.ProviderOperationID, TaskSubmitProviderOperationIDMaxLength, true) ||
			!IsValidTaskSubmitToken(result.LegacyPollingID, TaskSubmitProviderOperationIDMaxLength, true) {
			return fmt.Errorf("non-accepted result contains an invalid provider reference")
		}
		if !IsValidTaskSubmitToken(result.Problem.OutcomeCode, TaskSubmitOutcomeCodeMaxLength, false) ||
			len(result.Problem.SafeMessage) > TaskSubmitProblemMessageMaxLength ||
			!isSafeTaskSubmitText(result.Problem.SafeMessage, false) ||
			result.Problem.StatusCode < 100 || result.Problem.StatusCode > 599 {
			return fmt.Errorf("task submit problem is incomplete or exceeds its boundary")
		}
	default:
		return fmt.Errorf("invalid task submit disposition %q", result.Disposition)
	}
	return nil
}

// IsValidTaskSubmitToken reports whether a value is safe to persist as a
// bounded identifier. Tokens cannot contain whitespace, control characters,
// or invisible Unicode formatting characters.
func IsValidTaskSubmitToken(value string, limit int, allowEmpty bool) bool {
	if limit < 0 || len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	if value == "" {
		return allowEmpty
	}
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return false
		}
	}
	return true
}

func isSafeTaskSubmitText(value string, allowEmpty bool) bool {
	if !utf8.ValidString(value) || strings.TrimSpace(value) == "" {
		return allowEmpty && value == ""
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return false
		}
	}
	return true
}

func isSafeTaskSubmitJSONContentType(value string) bool {
	if len(value) > TaskSubmitContentTypeMaxLength || !isSafeTaskSubmitText(value, false) {
		return false
	}
	mediaType, parameters, err := mime.ParseMediaType(value)
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return false
	}
	for name, parameter := range parameters {
		if !strings.EqualFold(name, "charset") || !strings.EqualFold(parameter, "utf-8") {
			return false
		}
	}
	return true
}

func isTaskSubmitJSONObject(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	var object map[string]any
	if err := common.Unmarshal(data, &object); err != nil {
		return false
	}
	return object != nil
}
