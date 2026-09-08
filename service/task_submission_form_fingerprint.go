package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
)

const (
	taskSubmissionFormContentFamily                = "application/x-www-form-urlencoded"
	taskSubmissionMultipartContentFamily           = "multipart/form-data"
	taskSubmissionFingerprintMaxCanonicalBytes     = 1 << 20
	taskSubmissionFingerprintMaxFormFields         = 128
	taskSubmissionFingerprintMaxMultipartParts     = 64
	taskSubmissionFingerprintMaxFieldNameBytes     = 128
	taskSubmissionFingerprintMaxFileNameBytes      = 255
	taskSubmissionFormCanonicalizationVersion      = "myapi-task-submission-form-v1"
	taskSubmissionMultipartCanonicalizationVersion = "myapi-task-submission-multipart-v1"
)

type taskSubmissionCanonicalMultipartPart struct {
	name        string
	kind        string
	contentType string
	fileName    string
	size        int
	digest      [sha256.Size]byte
}

// FingerprintTaskSubmissionFormRequest canonicalizes an
// application/x-www-form-urlencoded task request and returns only its scoped
// fingerprint. It is a pure protocol boundary: it neither reads a request nor
// invokes routing, billing, persistence, or an upstream provider.
func FingerprintTaskSubmissionFormRequest(input TaskSubmissionFingerprintInput) (string, error) {
	if !validTaskSubmissionFingerprintProtocol(input.TokenID, input.Protocol) {
		return "", ErrTaskSubmissionFingerprintProtocol
	}
	if !validTaskSubmissionNonJSONOperationKind(input.Protocol.OperationKind) {
		return "", ErrTaskSubmissionFingerprintContentType
	}
	if !validTaskSubmissionFormContentType(input.ContentType) {
		return "", ErrTaskSubmissionFingerprintContentType
	}

	bodyDigest, err := canonicalTaskSubmissionFormDigest(input.Body)
	if err != nil {
		return "", ErrTaskSubmissionFingerprintBody
	}
	return fingerprintTaskSubmissionCanonicalBody(input, taskSubmissionFormContentFamily, bodyDigest)
}

// FingerprintTaskSubmissionMultipartRequest canonicalizes a bounded
// multipart/form-data task request independently from its wire boundary. It
// accepts only form-data text and file parts with explicit, limited metadata;
// unknown headers, part types, duplicate names, and malformed bodies fail
// closed. It does not read an HTTP request or persist any source material.
func FingerprintTaskSubmissionMultipartRequest(input TaskSubmissionFingerprintInput) (string, error) {
	if !validTaskSubmissionFingerprintProtocol(input.TokenID, input.Protocol) {
		return "", ErrTaskSubmissionFingerprintProtocol
	}
	if !validTaskSubmissionNonJSONOperationKind(input.Protocol.OperationKind) {
		return "", ErrTaskSubmissionFingerprintContentType
	}
	boundary, ok := taskSubmissionMultipartBoundary(input.ContentType)
	if !ok {
		return "", ErrTaskSubmissionFingerprintContentType
	}

	bodyDigest, err := canonicalTaskSubmissionMultipartDigest(input.Body, boundary)
	if err != nil {
		return "", ErrTaskSubmissionFingerprintBody
	}
	return fingerprintTaskSubmissionCanonicalBody(input, taskSubmissionMultipartContentFamily, bodyDigest)
}

func fingerprintTaskSubmissionCanonicalBody(input TaskSubmissionFingerprintInput, contentFamily string, bodyDigest [sha256.Size]byte) (string, error) {
	routeBinding, err := taskSubmissionFingerprintRouteBinding(input.Protocol.OperationKind, input.OriginTaskPublicID)
	if err != nil {
		return "", err
	}
	fingerprint, err := common.HashTaskRecoveryRequestFingerprint(common.TaskRecoveryRequestFingerprintMaterial{
		TokenID:             int64(input.TokenID),
		HTTPMethod:          input.Protocol.HTTPMethod,
		OperationKind:       input.Protocol.OperationKind,
		RouteBinding:        routeBinding,
		ContentFamily:       contentFamily,
		CanonicalBodyDigest: bodyDigest,
	})
	if err != nil {
		return "", ErrTaskSubmissionFingerprintSecret
	}
	return fingerprint, nil
}

func validTaskSubmissionFormContentType(contentType string) bool {
	return validTaskSubmissionUTF8TextContentType(contentType, taskSubmissionFormContentFamily)
}

// Only the video request adapters currently have a documented legacy
// form/multipart shape. Suno's non-JSON behavior remains outside B1b until a
// separate, adapter-specific body/header contract is frozen.
func validTaskSubmissionNonJSONOperationKind(operationKind string) bool {
	switch operationKind {
	case model.TaskSubmissionOperationKindVideoCreate, model.TaskSubmissionOperationKindVideoRemix:
		return true
	default:
		return false
	}
}

func validTaskSubmissionUTF8TextContentType(contentType, expectedMediaType string) bool {
	for index := 0; index < len(contentType); index++ {
		if contentType[index] != '\t' && (contentType[index] < 0x20 || contentType[index] > 0x7e) {
			return false
		}
	}
	raw := strings.Trim(contentType, " \t")
	parts := strings.Split(raw, ";")
	if len(parts) == 0 || len(parts) > 2 || !strings.EqualFold(strings.Trim(parts[0], " \t"), expectedMediaType) {
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
	if err != nil || mediaType != expectedMediaType || len(parameters) != wantParameters {
		return false
	}
	return wantParameters == 0 || strings.EqualFold(parameters["charset"], "utf-8")
}

func canonicalTaskSubmissionFormDigest(body []byte) ([sha256.Size]byte, error) {
	if len(body) == 0 || len(body) > taskSubmissionFingerprintMaxCanonicalBytes || !utf8.Valid(body) {
		return [sha256.Size]byte{}, ErrTaskSubmissionFingerprintBody
	}
	values, err := url.ParseQuery(string(body))
	if err != nil || len(values) == 0 || len(values) > taskSubmissionFingerprintMaxFormFields {
		return [sha256.Size]byte{}, ErrTaskSubmissionFingerprintBody
	}

	keys := make([]string, 0, len(values))
	for key, fieldValues := range values {
		if !validTaskSubmissionFieldName(key) || len(fieldValues) != 1 || !utf8.ValidString(fieldValues[0]) {
			return [sha256.Size]byte{}, ErrTaskSubmissionFingerprintBody
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	hasher := sha256.New()
	writeTaskSubmissionCanonicalFrame(hasher, []byte(taskSubmissionFormCanonicalizationVersion))
	writeTaskSubmissionCanonicalCount(hasher, len(keys))
	for _, key := range keys {
		writeTaskSubmissionCanonicalFrame(hasher, []byte(key))
		writeTaskSubmissionCanonicalFrame(hasher, []byte(values.Get(key)))
	}
	return taskSubmissionCanonicalHash(hasher), nil
}

func taskSubmissionMultipartBoundary(contentType string) (string, bool) {
	if !validTaskSubmissionMIMEHeaderValue(contentType) {
		return "", false
	}
	raw := strings.Trim(contentType, " \t")
	parts := strings.Split(raw, ";")
	if len(parts) != 2 || !strings.EqualFold(strings.Trim(parts[0], " \t"), taskSubmissionMultipartContentFamily) {
		return "", false
	}
	parameter := strings.Trim(parts[1], " \t")
	separator := strings.IndexByte(parameter, '=')
	if separator <= 0 || separator != strings.LastIndexByte(parameter, '=') {
		return "", false
	}
	name := strings.Trim(parameter[:separator], " \t")
	if strings.Contains(name, "*") || !strings.EqualFold(name, "boundary") {
		return "", false
	}
	mediaType, parameters, err := mime.ParseMediaType(raw)
	if err != nil || mediaType != taskSubmissionMultipartContentFamily || len(parameters) != 1 {
		return "", false
	}
	boundary, ok := parameters["boundary"]
	if !ok || len(boundary) == 0 || len(boundary) > 70 || !validTaskSubmissionMIMEHeaderValue(boundary) {
		return "", false
	}
	return boundary, true
}

func canonicalTaskSubmissionMultipartDigest(body []byte, boundary string) ([sha256.Size]byte, error) {
	if len(body) == 0 || len(body) > taskSubmissionFingerprintMaxCanonicalBytes {
		return [sha256.Size]byte{}, ErrTaskSubmissionFingerprintBody
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	parts := make([]taskSubmissionCanonicalMultipartPart, 0)
	names := make(map[string]struct{})
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil || len(parts) >= taskSubmissionFingerprintMaxMultipartParts {
			return [sha256.Size]byte{}, ErrTaskSubmissionFingerprintBody
		}

		canonicalPart, partErr := canonicalTaskSubmissionMultipartPart(part)
		closeErr := part.Close()
		if partErr != nil || closeErr != nil {
			return [sha256.Size]byte{}, ErrTaskSubmissionFingerprintBody
		}
		if _, exists := names[canonicalPart.name]; exists {
			return [sha256.Size]byte{}, ErrTaskSubmissionFingerprintBody
		}
		names[canonicalPart.name] = struct{}{}
		parts = append(parts, canonicalPart)
	}
	if len(parts) == 0 {
		return [sha256.Size]byte{}, ErrTaskSubmissionFingerprintBody
	}

	sort.Slice(parts, func(first, second int) bool {
		return parts[first].name < parts[second].name
	})
	hasher := sha256.New()
	writeTaskSubmissionCanonicalFrame(hasher, []byte(taskSubmissionMultipartCanonicalizationVersion))
	writeTaskSubmissionCanonicalCount(hasher, len(parts))
	for _, part := range parts {
		writeTaskSubmissionCanonicalFrame(hasher, []byte(part.name))
		writeTaskSubmissionCanonicalFrame(hasher, []byte(part.kind))
		writeTaskSubmissionCanonicalFrame(hasher, []byte(part.contentType))
		writeTaskSubmissionCanonicalFrame(hasher, []byte(part.fileName))
		writeTaskSubmissionCanonicalCount(hasher, part.size)
		writeTaskSubmissionCanonicalFrame(hasher, part.digest[:])
	}
	return taskSubmissionCanonicalHash(hasher), nil
}

func canonicalTaskSubmissionMultipartPart(part *multipart.Part) (taskSubmissionCanonicalMultipartPart, error) {
	var disposition string
	var contentType string
	contentTypeProvided := false
	for name, values := range part.Header {
		if len(values) != 1 {
			return taskSubmissionCanonicalMultipartPart{}, ErrTaskSubmissionFingerprintBody
		}
		switch strings.ToLower(name) {
		case "content-disposition":
			disposition = values[0]
		case "content-type":
			contentType = values[0]
			contentTypeProvided = true
		default:
			return taskSubmissionCanonicalMultipartPart{}, ErrTaskSubmissionFingerprintBody
		}
	}

	name, fileName, isFile, ok := taskSubmissionMultipartDisposition(disposition)
	if !ok {
		return taskSubmissionCanonicalMultipartPart{}, ErrTaskSubmissionFingerprintBody
	}
	canonicalContentType, ok := taskSubmissionMultipartPartContentType(contentType, contentTypeProvided, isFile)
	if !ok {
		return taskSubmissionCanonicalMultipartPart{}, ErrTaskSubmissionFingerprintBody
	}
	data, err := io.ReadAll(io.LimitReader(part, taskSubmissionFingerprintMaxCanonicalBytes+1))
	if err != nil || len(data) > taskSubmissionFingerprintMaxCanonicalBytes || (!isFile && !utf8.Valid(data)) {
		return taskSubmissionCanonicalMultipartPart{}, ErrTaskSubmissionFingerprintBody
	}

	kind := "field"
	if isFile {
		kind = "file"
	}
	return taskSubmissionCanonicalMultipartPart{
		name:        name,
		kind:        kind,
		contentType: canonicalContentType,
		fileName:    fileName,
		size:        len(data),
		digest:      sha256.Sum256(data),
	}, nil
}

func taskSubmissionMultipartDisposition(value string) (name, fileName string, isFile, ok bool) {
	if !validTaskSubmissionMIMEHeaderValue(value) || strings.Contains(value, "*") {
		return "", "", false, false
	}
	disposition, parameters, err := mime.ParseMediaType(value)
	if err != nil || disposition != "form-data" || len(parameters) == 0 || len(parameters) > 2 {
		return "", "", false, false
	}
	name, ok = parameters["name"]
	if !ok || !validTaskSubmissionFieldName(name) {
		return "", "", false, false
	}
	fileName, isFile = parameters["filename"]
	// Go's form consumer treats both an absent filename and filename="" as a
	// text value. Reject the ambiguous wire spelling rather than fingerprinting
	// it as a file while downstream code treats it as a field.
	if isFile && (fileName == "" || !validTaskSubmissionFileName(fileName)) {
		return "", "", false, false
	}
	for parameterName := range parameters {
		if parameterName != "name" && parameterName != "filename" {
			return "", "", false, false
		}
	}
	return name, fileName, isFile, true
}

func taskSubmissionMultipartPartContentType(contentType string, provided, isFile bool) (string, bool) {
	if !provided {
		if isFile {
			return "application/octet-stream", true
		}
		return "text/plain;charset=utf-8", true
	}
	if !validTaskSubmissionMIMEHeaderValue(contentType) || strings.Contains(contentType, "*") {
		return "", false
	}
	mediaType, parameters, err := mime.ParseMediaType(strings.Trim(contentType, " \t"))
	if err != nil || mediaType == "" {
		return "", false
	}
	if isFile {
		return mediaType, len(parameters) == 0
	}
	if mediaType != "text/plain" || len(parameters) > 1 {
		return "", false
	}
	if len(parameters) == 1 && !strings.EqualFold(parameters["charset"], "utf-8") {
		return "", false
	}
	return "text/plain;charset=utf-8", true
}

func validTaskSubmissionMIMEHeaderValue(value string) bool {
	if value == "" || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character != '\t' && (unicode.IsControl(character) || character == 0x7f) {
			return false
		}
	}
	return true
}

func validTaskSubmissionFieldName(value string) bool {
	if len(value) == 0 || len(value) > taskSubmissionFingerprintMaxFieldNameBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validTaskSubmissionFileName(value string) bool {
	if len(value) > taskSubmissionFingerprintMaxFileNameBytes || !utf8.ValidString(value) || strings.ContainsAny(value, "/\\") {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func writeTaskSubmissionCanonicalFrame(hasher hash.Hash, value []byte) {
	writeTaskSubmissionCanonicalCount(hasher, len(value))
	_, _ = hasher.Write(value)
}

func writeTaskSubmissionCanonicalCount(hasher hash.Hash, count int) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(count))
	_, _ = hasher.Write(encoded[:])
}

func taskSubmissionCanonicalHash(hasher hash.Hash) [sha256.Size]byte {
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return digest
}
