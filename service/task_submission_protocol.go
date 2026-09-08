package service

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/model"
)

const (
	taskSubmissionIdempotencyHeader       = "Idempotency-Key"
	taskSubmissionIdempotencyHeaderAlias  = "X-Idempotency-Key"
	taskSubmissionIdempotencyKeyMaxLength = 255
	taskSubmissionDigestLength            = 64
)

var (
	ErrTaskSubmissionProtocolMethod              = errors.New("task submission protocol requires POST")
	ErrTaskSubmissionProtocolOperationKind       = errors.New("task submission protocol operation kind is invalid")
	ErrTaskSubmissionProtocolIdempotencyMissing  = errors.New("task submission protocol idempotency key is missing")
	ErrTaskSubmissionProtocolIdempotencyMultiple = errors.New("task submission protocol idempotency key must have exactly one value")
	ErrTaskSubmissionProtocolIdempotencyAlias    = errors.New("task submission protocol does not accept an idempotency key alias")
	ErrTaskSubmissionProtocolIdempotencyInvalid  = errors.New("task submission protocol idempotency key is invalid")
	ErrTaskSubmissionProtocolIdempotencyHash     = errors.New("task submission protocol could not hash the idempotency key")
)

type taskSubmissionIdempotencyKeyHasher func(rawKey string) (string, error)

// TaskSubmissionProtocol is the stable B2 scope material after a request's
// client Idempotency-Key has been validated and digested. The caller combines
// it with the authenticated token ID and a canonical request fingerprint; this
// pure boundary intentionally does not parse a body, access a database, or
// decide routing, billing, or dispatch.
type TaskSubmissionProtocol struct {
	HTTPMethod         string
	OperationKind      string
	IdempotencyKeyHash string
}

// ParseTaskSubmissionProtocol accepts exactly one canonical Idempotency-Key
// field and binds it to the deployment-scoped task-recovery HMAC. Header names
// are case-insensitive, but the historical X-Idempotency-Key alias is
// deliberately rejected: accepting both spellings would make a client-visible
// idempotency scope ambiguous. On success it removes every actual-casing
// canonical key from headers, so later request contexts and accidental
// forwarding cannot retain the raw client key. The returned value and every
// error contain no raw key.
func ParseTaskSubmissionProtocol(headers http.Header, method, operationKind string) (*TaskSubmissionProtocol, error) {
	return parseTaskSubmissionProtocolWithHasher(headers, method, operationKind, model.HashTaskSubmissionIdempotencyKey)
}

// parseTaskSubmissionProtocolWithHasher keeps the production binding above
// non-bypassable while allowing deterministic unit tests to isolate header
// syntax and hash-error behavior without reading deployment configuration.
func parseTaskSubmissionProtocolWithHasher(headers http.Header, method, operationKind string, hashKey taskSubmissionIdempotencyKeyHasher) (*TaskSubmissionProtocol, error) {
	canonicalMethod := strings.ToUpper(strings.TrimSpace(method))
	if canonicalMethod != http.MethodPost {
		return nil, ErrTaskSubmissionProtocolMethod
	}

	canonicalOperationKind := strings.ToLower(strings.TrimSpace(operationKind))
	if !validTaskSubmissionProtocolOperationKind(canonicalOperationKind) {
		return nil, ErrTaskSubmissionProtocolOperationKind
	}

	key, err := taskSubmissionIdempotencyKeyFromHeader(headers)
	if err != nil {
		return nil, err
	}
	if hashKey == nil {
		return nil, ErrTaskSubmissionProtocolIdempotencyHash
	}
	keyHash, err := hashKey(key)
	if err != nil || !validTaskSubmissionProtocolDigest(keyHash) {
		return nil, ErrTaskSubmissionProtocolIdempotencyHash
	}
	removeTaskSubmissionIdempotencyKey(headers)

	return &TaskSubmissionProtocol{
		HTTPMethod:         canonicalMethod,
		OperationKind:      canonicalOperationKind,
		IdempotencyKeyHash: keyHash,
	}, nil
}

func removeTaskSubmissionIdempotencyKey(headers http.Header) {
	for name := range headers {
		if strings.EqualFold(name, taskSubmissionIdempotencyHeader) {
			delete(headers, name)
		}
	}
}

func taskSubmissionIdempotencyKeyFromHeader(headers http.Header) (string, error) {
	var values []string
	for name, headerValues := range headers {
		normalizedName := strings.ToLower(strings.ReplaceAll(name, "_", "-"))
		switch normalizedName {
		case strings.ToLower(taskSubmissionIdempotencyHeaderAlias):
			return "", ErrTaskSubmissionProtocolIdempotencyAlias
		case strings.ToLower(taskSubmissionIdempotencyHeader):
			// HTTP header names are case-insensitive, but an underscore spelling
			// is a different field that must not be silently accepted as a
			// scope-bearing Idempotency-Key.
			if !strings.EqualFold(name, taskSubmissionIdempotencyHeader) {
				return "", ErrTaskSubmissionProtocolIdempotencyAlias
			}
			values = append(values, headerValues...)
		}
	}
	if len(values) == 0 {
		return "", ErrTaskSubmissionProtocolIdempotencyMissing
	}
	if len(values) != 1 {
		return "", ErrTaskSubmissionProtocolIdempotencyMultiple
	}
	if !validTaskSubmissionIdempotencyKey(values[0]) {
		return "", ErrTaskSubmissionProtocolIdempotencyInvalid
	}
	return values[0], nil
}

func validTaskSubmissionIdempotencyKey(value string) bool {
	if len(value) == 0 || len(value) > taskSubmissionIdempotencyKeyMaxLength {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func validTaskSubmissionProtocolOperationKind(value string) bool {
	switch value {
	case model.TaskSubmissionOperationKindVideoCreate,
		model.TaskSubmissionOperationKindVideoRemix,
		model.TaskSubmissionOperationKindSunoMusic,
		model.TaskSubmissionOperationKindSunoLyrics:
		return true
	default:
		return false
	}
}

func validTaskSubmissionProtocolDigest(value string) bool {
	if len(value) != taskSubmissionDigestLength {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
