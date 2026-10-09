package middleware

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
)

const (
	playgroundMaxBodyBytes  = 32 << 20
	playgroundMaxImageBytes = 10 << 20
	playgroundMaxMediaBytes = 20 << 20
	playgroundMaxImages     = 4
)

// PlaygroundMediaGuard bounds this browser client's inline payloads before
// content logging, distribution or dispatch. It never modifies the JSON or
// relaxes the common relay's provider and strict-budget admission rules.
func PlaygroundMediaGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || contentType != "application/json" {
			abortWithOpenAiMessage(c, http.StatusUnsupportedMediaType, common.TranslateMessage(c, i18n.MsgInvalidParams), types.ErrorCode("playground_invalid_media"))
			return
		}
		if c.Request.ContentLength > playgroundMaxBodyBytes {
			abortWithOpenAiMessage(c, http.StatusRequestEntityTooLarge, common.TranslateMessage(c, i18n.MsgInvalidParams), types.ErrorCode("playground_payload_too_large"))
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, playgroundMaxBodyBytes)
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			status := http.StatusBadRequest
			if common.IsRequestBodyTooLargeError(err) {
				status = http.StatusRequestEntityTooLarge
			}
			abortWithOpenAiMessage(c, status, common.TranslateMessage(c, i18n.MsgInvalidParams), types.ErrorCode("playground_invalid_media"))
			return
		}
		body, err := storage.Bytes()
		if err != nil || len(body) > playgroundMaxBodyBytes || !validPlaygroundMedia(body) {
			abortWithOpenAiMessage(c, http.StatusBadRequest, common.TranslateMessage(c, i18n.MsgInvalidParams), types.ErrorCode("playground_invalid_media"))
			return
		}
		reader, err := storage.NewReader()
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, common.TranslateMessage(c, i18n.MsgOperationFailed))
			return
		}
		defer reader.Close()
		c.Request.Body = reader
		c.Next()
	}
}

func validPlaygroundMedia(body []byte) bool {
	if !utf8.Valid(body) || common.ValidateUniqueJSONKeys(body) != nil {
		return false
	}
	var request map[string]json.RawMessage
	if common.Unmarshal(body, &request) != nil {
		return false
	}
	for key := range request {
		if !playgroundProtocolFieldName(key) {
			return false
		}
	}
	var messages []map[string]json.RawMessage
	if common.Unmarshal(request["messages"], &messages) != nil {
		return false
	}
	count, total := 0, 0
	for _, message := range messages {
		for key := range message {
			if !playgroundProtocolFieldName(key) {
				return false
			}
		}
		content := bytes.TrimSpace(message["content"])
		if len(content) == 0 || bytes.Equal(content, []byte("null")) {
			continue
		}
		if content[0] == '"' {
			var value string
			if common.Unmarshal(content, &value) != nil {
				return false
			}
			continue
		}
		if content[0] != '[' {
			return false
		}
		var parts []map[string]json.RawMessage
		if common.Unmarshal(content, &parts) != nil {
			return false
		}
		for _, part := range parts {
			var kind string
			if common.Unmarshal(part["type"], &kind) != nil {
				return false
			}
			switch kind {
			case "text":
				for key := range part {
					if key != "type" && key != "text" && key != "cache_control" {
						return false
					}
				}
				var text string
				if common.Unmarshal(part["text"], &text) != nil {
					return false
				}
				continue
			case "image_url", "file":
				var role string
				if common.Unmarshal(message["role"], &role) != nil || role != "user" {
					return false
				}
				for key := range part {
					if key != "type" && key != kind && key != "cache_control" {
						return false
					}
				}
			default:
				return false
			}
			var media map[string]json.RawMessage
			if common.Unmarshal(part[kind], &media) != nil {
				return false
			}
			var dataURL string
			if kind == "image_url" {
				for key := range media {
					if key != "url" && key != "detail" {
						return false
					}
				}
				if common.Unmarshal(media["url"], &dataURL) != nil {
					return false
				}
			} else {
				for key := range media {
					if key != "filename" && key != "file_data" {
						return false
					}
				}
				var name string
				if common.Unmarshal(media["filename"], &name) != nil || common.Unmarshal(media["file_data"], &dataURL) != nil {
					return false
				}
				if name == "" || len(name) > 255 || !utf8.ValidString(name) || strings.ContainsAny(name, "\x00\r\n/\\") || !strings.HasSuffix(strings.ToLower(name), ".pdf") {
					return false
				}
			}
			count++
			if count > playgroundMaxImages {
				return false
			}
			header, encoded, ok := strings.Cut(dataURL, ",")
			if !ok || !strings.HasPrefix(header, "data:") || !strings.HasSuffix(header, ";base64") {
				return false
			}
			mediaType := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
			switch mediaType {
			case "image/png", "image/jpeg", "image/webp", "image/gif", "application/pdf":
			default:
				return false
			}
			if len(encoded) > base64.StdEncoding.EncodedLen(playgroundMaxImageBytes) {
				return false
			}
			if (kind == "file") != (mediaType == "application/pdf") {
				return false
			}
			decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
			if err != nil || len(decoded) == 0 || len(decoded) > playgroundMaxImageBytes || http.DetectContentType(decoded) != mediaType {
				return false
			}
			total += len(decoded)
			if total > playgroundMaxMediaBytes {
				return false
			}
		}
	}
	return true
}

// Public Chat protocol fields at the root/message envelope use ASCII lowercase
// names. Arbitrary nested tool schemas and metadata are intentionally untouched.
// Reject aliases before original-body passthrough can disagree with Go structs.
func playgroundProtocolFieldName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' {
			return false
		}
	}
	return true
}
