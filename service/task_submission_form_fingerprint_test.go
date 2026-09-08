package service

import (
	"bytes"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validTaskSubmissionNonJSONFingerprintInput() TaskSubmissionFingerprintInput {
	return TaskSubmissionFingerprintInput{
		TokenID: 41,
		Protocol: &TaskSubmissionProtocol{
			HTTPMethod:         http.MethodPost,
			OperationKind:      model.TaskSubmissionOperationKindVideoCreate,
			IdempotencyKeyHash: strings.Repeat("a", taskSubmissionDigestLength),
		},
	}
}

func TestFingerprintTaskSubmissionFormRequestCanonicalizesURLDecodingAndFieldOrder(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	input := validTaskSubmissionNonJSONFingerprintInput()
	input.ContentType = taskSubmissionFormContentFamily
	input.Body = []byte("model=fixture&prompt=hello+world&empty=")
	first, err := FingerprintTaskSubmissionFormRequest(input)
	require.NoError(t, err)

	input.ContentType = `Application/X-Www-Form-Urlencoded; Charset="UTF-8"`
	input.Body = []byte("empty=&prompt=hello%20world&model=fixture")
	second, err := FingerprintTaskSubmissionFormRequest(input)
	require.NoError(t, err)
	assert.Equal(t, first, second)

	input.Body = []byte("empty=&prompt=hello%20world&model=changed")
	different, err := FingerprintTaskSubmissionFormRequest(input)
	require.NoError(t, err)
	assert.NotEqual(t, first, different)
}

func TestFingerprintTaskSubmissionFormRequestBindsTheExistingScope(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	input := validTaskSubmissionNonJSONFingerprintInput()
	input.ContentType = taskSubmissionFormContentFamily
	input.Body = []byte("model=fixture&prompt=hello")
	baseline, err := FingerprintTaskSubmissionFormRequest(input)
	require.NoError(t, err)

	input.TokenID++
	otherToken, err := FingerprintTaskSubmissionFormRequest(input)
	require.NoError(t, err)
	assert.NotEqual(t, baseline, otherToken)

	input = validTaskSubmissionNonJSONFingerprintInput()
	input.Protocol.OperationKind = model.TaskSubmissionOperationKindVideoRemix
	input.OriginTaskPublicID = "task_" + strings.Repeat("b", 32)
	input.ContentType = taskSubmissionFormContentFamily
	input.Body = []byte("model=fixture&prompt=hello")
	firstOrigin, err := FingerprintTaskSubmissionFormRequest(input)
	require.NoError(t, err)
	input.OriginTaskPublicID = "task_" + strings.Repeat("c", 32)
	secondOrigin, err := FingerprintTaskSubmissionFormRequest(input)
	require.NoError(t, err)
	assert.NotEqual(t, firstOrigin, secondOrigin)
}

func TestFingerprintTaskSubmissionFormRequestRejectsAmbiguousOrUnsafeInputs(t *testing.T) {
	validInput := validTaskSubmissionNonJSONFingerprintInput()
	validInput.ContentType = taskSubmissionFormContentFamily
	validInput.Body = []byte("prompt=secret-canary")

	tooManyFields := make([]string, taskSubmissionFingerprintMaxFormFields+1)
	for index := range tooManyFields {
		tooManyFields[index] = "field" + strconv.Itoa(index) + "=value"
	}
	tooLarge := "prompt=" + strings.Repeat("a", taskSubmissionFingerprintMaxCanonicalBytes+1)

	for _, testCase := range []struct {
		name string
		body string
		err  error
	}{
		{"empty body", "", ErrTaskSubmissionFingerprintBody},
		{"malformed percent escape", "prompt=%", ErrTaskSubmissionFingerprintBody},
		{"invalid decoded utf8", "prompt=%ff", ErrTaskSubmissionFingerprintBody},
		{"duplicate field", "prompt=first&prompt=second", ErrTaskSubmissionFingerprintBody},
		{"empty field name", "=secret-canary", ErrTaskSubmissionFingerprintBody},
		{"too many fields", strings.Join(tooManyFields, "&"), ErrTaskSubmissionFingerprintBody},
		{"too large", tooLarge, ErrTaskSubmissionFingerprintBody},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			input := validInput
			input.Body = []byte(testCase.body)
			_, err := FingerprintTaskSubmissionFormRequest(input)
			require.ErrorIs(t, err, testCase.err)
			assert.Equal(t, testCase.err.Error(), err.Error())
			assert.NotContains(t, err.Error(), "secret-canary")
		})
	}

	for _, contentType := range []string{
		"application/x-www-form-urlencoded; charset=latin-1",
		"application/x-www-form-urlencoded; profile=secret-canary",
		"application/x-www-form-urlencoded; charset=utf-8; charset=utf-8",
		"application/x-www-form-urlencoded; charset*=utf-8''utf-8",
		"multipart/form-data; boundary=secret-canary",
	} {
		input := validInput
		input.ContentType = contentType
		_, err := FingerprintTaskSubmissionFormRequest(input)
		require.ErrorIs(t, err, ErrTaskSubmissionFingerprintContentType)
		assert.Equal(t, ErrTaskSubmissionFingerprintContentType.Error(), err.Error())
		assert.NotContains(t, err.Error(), "secret-canary")
	}
}

func TestFingerprintTaskSubmissionNonJSONRequestRejectsSunoUntilItsOwnContractExists(t *testing.T) {
	formInput := validTaskSubmissionNonJSONFingerprintInput()
	formInput.Protocol.OperationKind = model.TaskSubmissionOperationKindSunoMusic
	formInput.ContentType = taskSubmissionFormContentFamily
	formInput.Body = []byte("prompt=hello")
	_, err := FingerprintTaskSubmissionFormRequest(formInput)
	require.ErrorIs(t, err, ErrTaskSubmissionFingerprintContentType)

	body, contentType := buildTaskSubmissionMultipartBody(t, "suno-boundary", []taskSubmissionMultipartFixturePart{
		{name: "prompt", omitContentType: true, data: []byte("hello")},
	})
	multipartInput := validTaskSubmissionNonJSONFingerprintInput()
	multipartInput.Protocol.OperationKind = model.TaskSubmissionOperationKindSunoLyrics
	multipartInput.ContentType = contentType
	multipartInput.Body = body
	_, err = FingerprintTaskSubmissionMultipartRequest(multipartInput)
	require.ErrorIs(t, err, ErrTaskSubmissionFingerprintContentType)
}

type taskSubmissionMultipartFixturePart struct {
	name              string
	fileName          string
	isFile            bool
	contentType       string
	omitContentType   bool
	data              []byte
	extraHeaderValues map[string][]string
}

func buildTaskSubmissionMultipartBody(t *testing.T, boundary string, parts []taskSubmissionMultipartFixturePart) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.SetBoundary(boundary))
	for _, fixture := range parts {
		parameters := map[string]string{"name": fixture.name}
		if fixture.isFile {
			parameters["filename"] = fixture.fileName
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", parameters))
		if !fixture.omitContentType {
			header.Set("Content-Type", fixture.contentType)
		}
		for name, values := range fixture.extraHeaderValues {
			for _, value := range values {
				header.Add(name, value)
			}
		}
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = part.Write(fixture.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return body.Bytes(), taskSubmissionMultipartContentFamily + "; boundary=" + boundary
}

func TestFingerprintTaskSubmissionMultipartRequestIgnoresWireBoundaryAndPartOrder(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	firstBody, firstContentType := buildTaskSubmissionMultipartBody(t, "first-boundary", []taskSubmissionMultipartFixturePart{
		{name: "prompt", omitContentType: true, data: []byte("hello world")},
		{name: "image", fileName: "input.png", isFile: true, contentType: "image/png", data: []byte{0, 1, 2, 3}},
	})
	secondBody, secondContentType := buildTaskSubmissionMultipartBody(t, "second-boundary", []taskSubmissionMultipartFixturePart{
		{name: "image", fileName: "input.png", isFile: true, contentType: "image/png", data: []byte{0, 1, 2, 3}},
		{name: "prompt", contentType: "text/plain; charset=UTF-8", data: []byte("hello world")},
	})

	input := validTaskSubmissionNonJSONFingerprintInput()
	input.ContentType = firstContentType
	input.Body = firstBody
	first, err := FingerprintTaskSubmissionMultipartRequest(input)
	require.NoError(t, err)

	input.ContentType = secondContentType
	input.Body = secondBody
	second, err := FingerprintTaskSubmissionMultipartRequest(input)
	require.NoError(t, err)
	assert.Equal(t, first, second)

	changedBody, changedContentType := buildTaskSubmissionMultipartBody(t, "third-boundary", []taskSubmissionMultipartFixturePart{
		{name: "image", fileName: "other.png", isFile: true, contentType: "image/png", data: []byte{0, 1, 2, 3}},
		{name: "prompt", omitContentType: true, data: []byte("hello world")},
	})
	input.ContentType = changedContentType
	input.Body = changedBody
	changed, err := FingerprintTaskSubmissionMultipartRequest(input)
	require.NoError(t, err)
	assert.NotEqual(t, first, changed)
}

func TestFingerprintTaskSubmissionMultipartRequestRejectsUnsafeParts(t *testing.T) {
	validInput := validTaskSubmissionNonJSONFingerprintInput()

	tooManyParts := make([]taskSubmissionMultipartFixturePart, taskSubmissionFingerprintMaxMultipartParts+1)
	for index := range tooManyParts {
		tooManyParts[index] = taskSubmissionMultipartFixturePart{
			name:            "field" + strconv.Itoa(index),
			omitContentType: true,
			data:            []byte("value"),
		}
	}
	for _, testCase := range []struct {
		name  string
		parts []taskSubmissionMultipartFixturePart
	}{
		{
			name: "duplicate part name",
			parts: []taskSubmissionMultipartFixturePart{
				{name: "prompt", omitContentType: true, data: []byte("first")},
				{name: "prompt", omitContentType: true, data: []byte("second")},
			},
		},
		{
			name: "unknown part header",
			parts: []taskSubmissionMultipartFixturePart{
				{name: "prompt", omitContentType: true, data: []byte("secret-canary"), extraHeaderValues: map[string][]string{"X-Untrusted": {"secret-canary"}}},
			},
		},
		{
			name: "non utf8 text field",
			parts: []taskSubmissionMultipartFixturePart{
				{name: "prompt", omitContentType: true, data: []byte{0xff}},
			},
		},
		{
			name: "unsupported field media type",
			parts: []taskSubmissionMultipartFixturePart{
				{name: "prompt", contentType: "application/json", data: []byte("secret-canary")},
			},
		},
		{
			name: "unsafe file name",
			parts: []taskSubmissionMultipartFixturePart{
				{name: "image", fileName: "../secret-canary", isFile: true, contentType: "image/png", data: []byte("image")},
			},
		},
		{
			name: "empty file name is ambiguous",
			parts: []taskSubmissionMultipartFixturePart{
				{name: "prompt", fileName: "", isFile: true, contentType: "text/plain; charset=utf-8", data: []byte("secret-canary")},
			},
		},
		{
			name:  "too many parts",
			parts: tooManyParts,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			body, contentType := buildTaskSubmissionMultipartBody(t, "fixture-boundary", testCase.parts)
			input := validInput
			input.ContentType = contentType
			input.Body = body
			_, err := FingerprintTaskSubmissionMultipartRequest(input)
			require.ErrorIs(t, err, ErrTaskSubmissionFingerprintBody)
			assert.Equal(t, ErrTaskSubmissionFingerprintBody.Error(), err.Error())
			assert.NotContains(t, err.Error(), "secret-canary")
		})
	}

	validBody, validContentType := buildTaskSubmissionMultipartBody(t, "good-boundary", []taskSubmissionMultipartFixturePart{
		{name: "prompt", omitContentType: true, data: []byte("hello")},
	})
	for _, contentType := range []string{
		"multipart/form-data",
		"multipart/form-data; boundary*=utf-8''secret-canary",
		"multipart/form-data; boundary=one; charset=utf-8",
		"application/json",
	} {
		input := validInput
		input.ContentType = contentType
		input.Body = validBody
		_, err := FingerprintTaskSubmissionMultipartRequest(input)
		require.ErrorIs(t, err, ErrTaskSubmissionFingerprintContentType)
		assert.Equal(t, ErrTaskSubmissionFingerprintContentType.Error(), err.Error())
		assert.NotContains(t, err.Error(), "secret-canary")
	}

	input := validInput
	input.ContentType = validContentType
	input.Body = validBody[:len(validBody)-5]
	_, err := FingerprintTaskSubmissionMultipartRequest(input)
	require.ErrorIs(t, err, ErrTaskSubmissionFingerprintBody)
}
