package service

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskSubmissionProtocolBindsTheDedicatedHMACAndClearsRawHeaders(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	headers := http.Header{
		"iDeMpOtEnCy-KeY": {"client-request-1"},
	}
	protocol, err := ParseTaskSubmissionProtocol(headers, " post ", " VIDEO.CREATE ")
	require.NoError(t, err)
	require.NotNil(t, protocol)
	expectedHash, err := model.HashTaskSubmissionIdempotencyKey("client-request-1")
	require.NoError(t, err)
	assert.Equal(t, &TaskSubmissionProtocol{
		HTTPMethod:         http.MethodPost,
		OperationKind:      "video.create",
		IdempotencyKeyHash: expectedHash,
	}, protocol)
	assert.Empty(t, headers, "the raw key must not remain in a successful request context")
}

func TestParseTaskSubmissionProtocolRejectsAmbiguousOrMissingHeaders(t *testing.T) {
	validHasher := func(string) (string, error) { return strings.Repeat("a", taskSubmissionDigestLength), nil }
	for _, testCase := range []struct {
		name    string
		headers http.Header
		err     error
	}{
		{"missing", nil, ErrTaskSubmissionProtocolIdempotencyMissing},
		{"canonical field without a value", http.Header{"Idempotency-Key": nil}, ErrTaskSubmissionProtocolIdempotencyMissing},
		{"repeated values", http.Header{"Idempotency-Key": {"first", "second"}}, ErrTaskSubmissionProtocolIdempotencyMultiple},
		{"duplicate canonical casing", http.Header{"Idempotency-Key": {"first"}, "idempotency-key": {"second"}}, ErrTaskSubmissionProtocolIdempotencyMultiple},
		{"legacy alias", http.Header{"X-Idempotency-Key": {"legacy"}}, ErrTaskSubmissionProtocolIdempotencyAlias},
		{"legacy alias casing", http.Header{"x-iDeMpOtEnCy-kEy": {"legacy"}}, ErrTaskSubmissionProtocolIdempotencyAlias},
		{"legacy alias underscore", http.Header{"X_Idempotency_Key": {"legacy"}}, ErrTaskSubmissionProtocolIdempotencyAlias},
		{"canonical underscore", http.Header{"Idempotency_Key": {"canonical"}}, ErrTaskSubmissionProtocolIdempotencyAlias},
		{"canonical and legacy fields", http.Header{"Idempotency-Key": {"canonical"}, "X-Idempotency-Key": {"legacy"}}, ErrTaskSubmissionProtocolIdempotencyAlias},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := parseTaskSubmissionProtocolWithHasher(testCase.headers, http.MethodPost, "video.create", validHasher)
			assert.ErrorIs(t, err, testCase.err)
		})
	}
}

func TestParseTaskSubmissionProtocolRejectsInvalidIdempotencyKeysWithoutReflection(t *testing.T) {
	validHasher := func(string) (string, error) { return strings.Repeat("a", taskSubmissionDigestLength), nil }
	for _, testCase := range []struct {
		name string
		key  string
	}{
		{"empty", ""},
		{"leading space", " canary"},
		{"trailing space", "canary "},
		{"tab", "key\tvalue"},
		{"control", "key\x1fvalue"},
		{"non-ascii", "key-\u4e2d"},
		{"too long", strings.Repeat("a", taskSubmissionIdempotencyKeyMaxLength+1)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := parseTaskSubmissionProtocolWithHasher(http.Header{"Idempotency-Key": {testCase.key}}, http.MethodPost, "video.create", validHasher)
			assert.ErrorIs(t, err, ErrTaskSubmissionProtocolIdempotencyInvalid)
			if testCase.key != "" {
				assert.NotContains(t, err.Error(), testCase.key)
			}
		})
	}
}

func TestParseTaskSubmissionProtocolRejectsInvalidScopeAndHashingFailures(t *testing.T) {
	validHeader := http.Header{"Idempotency-Key": {"request-secret"}}
	validHasher := func(string) (string, error) { return strings.Repeat("a", taskSubmissionDigestLength), nil }

	_, err := parseTaskSubmissionProtocolWithHasher(validHeader, http.MethodGet, "video.create", validHasher)
	assert.ErrorIs(t, err, ErrTaskSubmissionProtocolMethod)
	_, err = parseTaskSubmissionProtocolWithHasher(validHeader, http.MethodPost, "unknown.kind", validHasher)
	assert.ErrorIs(t, err, ErrTaskSubmissionProtocolOperationKind)
	_, err = parseTaskSubmissionProtocolWithHasher(validHeader, http.MethodPost, "video.create", nil)
	assert.ErrorIs(t, err, ErrTaskSubmissionProtocolIdempotencyHash)
	_, err = parseTaskSubmissionProtocolWithHasher(validHeader, http.MethodPost, "video.create", func(string) (string, error) {
		return "", errors.New("request-secret must not appear")
	})
	assert.ErrorIs(t, err, ErrTaskSubmissionProtocolIdempotencyHash)
	assert.NotContains(t, err.Error(), "request-secret")
	_, err = parseTaskSubmissionProtocolWithHasher(validHeader, http.MethodPost, "video.create", func(string) (string, error) {
		return strings.Repeat("A", taskSubmissionDigestLength), nil
	})
	assert.ErrorIs(t, err, ErrTaskSubmissionProtocolIdempotencyHash)
}

func TestParseTaskSubmissionProtocolHasStableOutputForCanonicalHeaderCasing(t *testing.T) {
	hasher := func(rawKey string) (string, error) {
		if rawKey != "same-request" {
			return "", errors.New("unexpected key")
		}
		return strings.Repeat("b", taskSubmissionDigestLength), nil
	}
	first, err := parseTaskSubmissionProtocolWithHasher(http.Header{"Idempotency-Key": {"same-request"}}, "POST", "video.remix", hasher)
	require.NoError(t, err)
	second, err := parseTaskSubmissionProtocolWithHasher(http.Header{"IDEMPOTENCY-KEY": {"same-request"}}, "post", " VIDEO.REMIX ", hasher)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestParseTaskSubmissionProtocolAcceptsEveryFrozenOperationKindAndKeyBoundary(t *testing.T) {
	validHasher := func(string) (string, error) { return strings.Repeat("c", taskSubmissionDigestLength), nil }
	for _, operationKind := range []string{
		model.TaskSubmissionOperationKindVideoCreate,
		model.TaskSubmissionOperationKindVideoRemix,
		model.TaskSubmissionOperationKindSunoMusic,
		model.TaskSubmissionOperationKindSunoLyrics,
	} {
		t.Run(operationKind, func(t *testing.T) {
			protocol, err := parseTaskSubmissionProtocolWithHasher(
				http.Header{"Idempotency-Key": {strings.Repeat("k", taskSubmissionIdempotencyKeyMaxLength)}},
				http.MethodPost,
				operationKind,
				validHasher,
			)
			require.NoError(t, err)
			assert.Equal(t, operationKind, protocol.OperationKind)
		})
	}

	for _, value := range []string{
		strings.Repeat("d", taskSubmissionDigestLength-1),
		strings.Repeat("d", taskSubmissionDigestLength+1),
		strings.Repeat("D", taskSubmissionDigestLength),
	} {
		_, err := parseTaskSubmissionProtocolWithHasher(
			http.Header{"Idempotency-Key": {"valid-key"}},
			http.MethodPost,
			model.TaskSubmissionOperationKindVideoCreate,
			func(string) (string, error) { return value, nil },
		)
		assert.ErrorIs(t, err, ErrTaskSubmissionProtocolIdempotencyHash)
	}

	for _, key := range []string{"key\x7fvalue", "key\rvalue", "key\nvalue"} {
		_, err := parseTaskSubmissionProtocolWithHasher(
			http.Header{"Idempotency-Key": {key}},
			http.MethodPost,
			model.TaskSubmissionOperationKindVideoCreate,
			validHasher,
		)
		assert.ErrorIs(t, err, ErrTaskSubmissionProtocolIdempotencyInvalid)
	}
}

func TestHasTaskSubmissionIdempotencyHeader(t *testing.T) {
	assert.False(t, HasTaskSubmissionIdempotencyHeader(nil))
	assert.False(t, HasTaskSubmissionIdempotencyHeader(http.Header{}))
	assert.False(t, HasTaskSubmissionIdempotencyHeader(http.Header{"Authorization": {"Bearer xxx"}}))

	assert.True(t, HasTaskSubmissionIdempotencyHeader(http.Header{"Idempotency-Key": {"k"}}))
	assert.True(t, HasTaskSubmissionIdempotencyHeader(http.Header{"idempotency-key": {"k"}}))
	assert.True(t, HasTaskSubmissionIdempotencyHeader(http.Header{"IDEMPOTENCY-KEY": {"k"}}))
	assert.True(t, HasTaskSubmissionIdempotencyHeader(http.Header{"idempotency_key": {"k"}}))
	assert.True(t, HasTaskSubmissionIdempotencyHeader(http.Header{"X-Idempotency-Key": {"k"}}))
	assert.True(t, HasTaskSubmissionIdempotencyHeader(http.Header{"x_idempotency_key": {"k"}}))
}

