package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validTaskSubmissionFingerprintInput() TaskSubmissionFingerprintInput {
	return TaskSubmissionFingerprintInput{
		TokenID: 41,
		Protocol: &TaskSubmissionProtocol{
			HTTPMethod:         http.MethodPost,
			OperationKind:      model.TaskSubmissionOperationKindVideoCreate,
			IdempotencyKeyHash: strings.Repeat("a", taskSubmissionDigestLength),
		},
		ContentType: "application/json",
		Body:        []byte(`{"model":"fixture","input":{"prompt":"hello"}}`),
	}
}

func TestFingerprintTaskSubmissionJSONRequestCanonicalEquivalenceAndKnownAnswer(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	input := validTaskSubmissionFingerprintInput()
	fingerprint, err := FingerprintTaskSubmissionJSONRequest(input)
	require.NoError(t, err)
	assert.Equal(t, "a5b31f62aac805d185c98e7c3648c653f070f2f3df67cb377817ed00e6f39e2d", fingerprint)

	input.ContentType = "Application/JSON; Charset=UTF-8"
	input.Body = []byte(" { \"input\" : { \"prompt\" : \"hello\" }, \"model\" : \"fixture\" } \n")
	equivalent, err := FingerprintTaskSubmissionJSONRequest(input)
	require.NoError(t, err)
	assert.Equal(t, fingerprint, equivalent)
	input.ContentType = `application/json;charset="UTF-8"`
	quotedCharset, err := FingerprintTaskSubmissionJSONRequest(input)
	require.NoError(t, err)
	assert.Equal(t, fingerprint, quotedCharset)

	input.Body = []byte(`{"model":"fixture","input":{"prompt":"hello"},"n":1.0}`)
	different, err := FingerprintTaskSubmissionJSONRequest(input)
	require.NoError(t, err)
	assert.NotEqual(t, fingerprint, different)
}

func TestFingerprintTaskSubmissionJSONRequestBindsTokenKindAndRemixRoute(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	input := validTaskSubmissionFingerprintInput()
	baseline, err := FingerprintTaskSubmissionJSONRequest(input)
	require.NoError(t, err)

	input.TokenID++
	otherToken, err := FingerprintTaskSubmissionJSONRequest(input)
	require.NoError(t, err)
	assert.NotEqual(t, baseline, otherToken)

	input = validTaskSubmissionFingerprintInput()
	input.Protocol.IdempotencyKeyHash = strings.Repeat("b", taskSubmissionDigestLength)
	otherIdempotencyScope, err := FingerprintTaskSubmissionJSONRequest(input)
	require.NoError(t, err)
	assert.Equal(t, baseline, otherIdempotencyScope, "the separately persisted idempotency scope is validated but is not request body identity")

	input = validTaskSubmissionFingerprintInput()
	input.Protocol.OperationKind = model.TaskSubmissionOperationKindSunoMusic
	otherKind, err := FingerprintTaskSubmissionJSONRequest(input)
	require.NoError(t, err)
	assert.NotEqual(t, baseline, otherKind)

	input.Protocol.OperationKind = model.TaskSubmissionOperationKindVideoRemix
	input.OriginTaskPublicID = "task_" + strings.Repeat("b", 32)
	firstOrigin, err := FingerprintTaskSubmissionJSONRequest(input)
	require.NoError(t, err)
	input.OriginTaskPublicID = "task_" + strings.Repeat("c", 32)
	secondOrigin, err := FingerprintTaskSubmissionJSONRequest(input)
	require.NoError(t, err)
	assert.NotEqual(t, firstOrigin, secondOrigin)
}

func TestFingerprintTaskSubmissionJSONRequestRejectsUnsupportedContentFamilies(t *testing.T) {
	input := validTaskSubmissionFingerprintInput()
	for _, contentType := range []string{
		"",
		"text/json",
		"application/problem+json",
		"application/json; charset=iso-8859-1",
		"application/json; charset=iso-8859-1; charset*=utf-8''utf-8",
		"application/json; charset=utf-8; charset=utf-8",
		"application/json; charset*0=utf-; charset*1=8",
		"application/json; profile=secret-canary",
		"application/json; profile*=bogus",
		"application/json; charset",
		"application/json; charset==utf-8",
		"application/json; charset=utf-8\r\nprofile=secret-canary",
		"multipart/form-data; boundary=secret-canary",
		"application/octet-stream",
	} {
		input.ContentType = contentType
		_, err := FingerprintTaskSubmissionJSONRequest(input)
		require.ErrorIs(t, err, ErrTaskSubmissionFingerprintContentType)
		assert.Equal(t, ErrTaskSubmissionFingerprintContentType.Error(), err.Error())
		assert.NotContains(t, err.Error(), "secret-canary")
	}
}

func TestFingerprintTaskSubmissionJSONRequestRejectsUnvalidatedProtocolAndRoutes(t *testing.T) {
	input := validTaskSubmissionFingerprintInput()
	input.TokenID = 0
	_, err := FingerprintTaskSubmissionJSONRequest(input)
	assert.ErrorIs(t, err, ErrTaskSubmissionFingerprintProtocol)

	input = validTaskSubmissionFingerprintInput()
	input.Protocol.HTTPMethod = http.MethodGet
	_, err = FingerprintTaskSubmissionJSONRequest(input)
	assert.ErrorIs(t, err, ErrTaskSubmissionFingerprintProtocol)

	input = validTaskSubmissionFingerprintInput()
	input.Protocol.IdempotencyKeyHash = "invalid-secret-canary"
	_, err = FingerprintTaskSubmissionJSONRequest(input)
	require.ErrorIs(t, err, ErrTaskSubmissionFingerprintProtocol)
	assert.NotContains(t, err.Error(), "secret-canary")

	input = validTaskSubmissionFingerprintInput()
	input.OriginTaskPublicID = "task_" + strings.Repeat("b", 32)
	_, err = FingerprintTaskSubmissionJSONRequest(input)
	assert.ErrorIs(t, err, ErrTaskSubmissionFingerprintRoute)

	input.Protocol.OperationKind = model.TaskSubmissionOperationKindVideoRemix
	input.OriginTaskPublicID = "upstream-secret-canary"
	_, err = FingerprintTaskSubmissionJSONRequest(input)
	require.ErrorIs(t, err, ErrTaskSubmissionFingerprintRoute)
	assert.NotContains(t, err.Error(), "secret-canary")
}

func TestFingerprintTaskSubmissionJSONRequestRejectsBodyAndSecretFailuresWithoutReflection(t *testing.T) {
	input := validTaskSubmissionFingerprintInput()
	input.Body = []byte(`{"prompt":"secret-canary","prompt":"other"}`)
	_, err := FingerprintTaskSubmissionJSONRequest(input)
	require.ErrorIs(t, err, ErrTaskSubmissionFingerprintBody)
	assert.Equal(t, ErrTaskSubmissionFingerprintBody.Error(), err.Error())
	assert.NotContains(t, err.Error(), "secret-canary")

	input = validTaskSubmissionFingerprintInput()
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", "invalid")
	_, err = FingerprintTaskSubmissionJSONRequest(input)
	assert.ErrorIs(t, err, ErrTaskSubmissionFingerprintSecret)
}
