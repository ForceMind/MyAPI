package middleware

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

const (
	fullContentLogEnabledEnv  = "FULL_CONTENT_LOG_ENABLED"
	fullContentLogDirEnv      = "FULL_CONTENT_LOG_DIR"
	fullContentLogMaxMBEnv    = "FULL_CONTENT_LOG_MAX_MB"
	fullContentLogMaxFilesEnv = "FULL_CONTENT_LOG_MAX_FILES"
	defaultFullContentLogMB   = 100
)

type fullContentLogEntry struct {
	Timestamp   string              `json:"timestamp"`
	RequestID   string              `json:"request_id"`
	Phase       string              `json:"phase"`
	Sequence    int64               `json:"sequence,omitempty"`
	Method      string              `json:"method"`
	Path        string              `json:"path"`
	ContentType string              `json:"content_type,omitempty"`
	Encoding    string              `json:"encoding,omitempty"`
	Body        string              `json:"body,omitempty"`
	BodyBytes   int64               `json:"body_bytes,omitempty"`
	Status      int                 `json:"status,omitempty"`
	DurationMS  int64               `json:"duration_ms,omitempty"`
	UserID      int                 `json:"user_id,omitempty"`
	TokenID     int                 `json:"token_id,omitempty"`
	TokenName   string              `json:"token_name,omitempty"`
	ClientIP    string              `json:"client_ip,omitempty"`
	Headers     map[string][]string `json:"headers,omitempty"`
	Query       map[string][]string `json:"query,omitempty"`
	Error       string              `json:"error,omitempty"`
}

type fullContentFileWriter struct {
	mu       sync.Mutex
	dir      string
	maxBytes int64
	maxFiles int
	file     *os.File
	size     int64
}

type fullContentResponseWriter struct {
	gin.ResponseWriter
	context    *gin.Context
	logWriter  *fullContentFileWriter
	startedAt  time.Time
	sequence   int64
	bodyBytes  int64
	writeError atomic.Bool
}

var (
	fullContentLogLifecycleMu sync.RWMutex
	fullContentLogWriters     sync.Map
)

func FullContentLogger() gin.HandlerFunc {
	if !fullContentLogEnabled() {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	writer := &fullContentFileWriter{
		dir:      FullContentLogDirectory(),
		maxBytes: int64(fullContentLogEnvInt(fullContentLogMaxMBEnv, defaultFullContentLogMB)) << 20,
		maxFiles: fullContentLogEnvInt(fullContentLogMaxFilesEnv, 0),
	}
	fullContentLogWriters.Store(writer, struct{}{})

	return func(c *gin.Context) {
		startedAt := time.Now()
		responseWriter := &fullContentResponseWriter{
			ResponseWriter: c.Writer,
			context:        c,
			logWriter:      writer,
			startedAt:      startedAt,
		}
		c.Writer = responseWriter

		requestEntry := newFullContentLogEntry(c, "request")
		requestEntry.ContentType = c.GetHeader("Content-Type")
		requestEntry.Headers = redactFullContentLogValues(c.Request.Header)
		requestEntry.Query = redactFullContentLogValues(c.Request.URL.Query())
		if c.Request.Body != nil {
			storage, err := common.GetBodyStorage(c)
			if err != nil {
				requestEntry.Error = err.Error()
			} else if body, err := storage.Bytes(); err != nil {
				requestEntry.Error = err.Error()
			} else {
				requestEntry.BodyBytes = int64(len(body))
				requestEntry.Body, requestEntry.Encoding = encodeFullContentLogBody(requestEntry.ContentType, body, true)

				// GetBodyStorage consumes and closes the original request body. Restore a
				// fresh reader so handlers that read c.Request.Body directly still work.
				reader, replayErr := storage.NewReader()
				if replayErr != nil {
					requestEntry.Error = replayErr.Error()
				} else {
					c.Request.Body = reader
					c.Request.GetBody = storage.NewReader
					c.Request.ContentLength = storage.Size()
					defer reader.Close()
				}
			}
		}
		writeFullContentLogEntry(writer, requestEntry)

		c.Next()

		endEntry := newFullContentLogEntry(c, "response_end")
		endEntry.Status = c.Writer.Status()
		endEntry.Headers = redactFullContentLogValues(c.Writer.Header())
		endEntry.Sequence = atomic.LoadInt64(&responseWriter.sequence)
		endEntry.BodyBytes = atomic.LoadInt64(&responseWriter.bodyBytes)
		endEntry.DurationMS = time.Since(startedAt).Milliseconds()
		if responseWriter.writeError.Load() {
			endEntry.Error = "one or more response chunks could not be written to the content log"
		}
		writeFullContentLogEntry(writer, endEntry)
	}
}

func (w *fullContentResponseWriter) Write(body []byte) (int, error) {
	n, err := w.ResponseWriter.Write(body)
	if n > 0 {
		w.logResponseChunk(body[:n])
	}
	return n, err
}

func (w *fullContentResponseWriter) WriteString(body string) (int, error) {
	n, err := w.ResponseWriter.WriteString(body)
	if n > 0 {
		w.logResponseChunk([]byte(body[:n]))
	}
	return n, err
}

func (w *fullContentResponseWriter) logResponseChunk(body []byte) {
	entry := newFullContentLogEntry(w.context, "response_chunk")
	entry.Sequence = atomic.AddInt64(&w.sequence, 1)
	entry.Status = w.ResponseWriter.Status()
	entry.ContentType = w.ResponseWriter.Header().Get("Content-Type")
	entry.BodyBytes = int64(len(body))
	entry.Body, entry.Encoding = encodeFullContentLogBody(entry.ContentType, body, false)
	atomic.AddInt64(&w.bodyBytes, int64(len(body)))
	if err := w.logWriter.write(entry); err != nil {
		w.writeError.Store(true)
		common.SysError(fmt.Sprintf("failed to write full content response log: %v", err))
	}
}

func newFullContentLogEntry(c *gin.Context, phase string) fullContentLogEntry {
	return fullContentLogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		RequestID: c.GetString(common.RequestIdKey),
		Phase:     phase,
		Method:    c.Request.Method,
		Path:      c.Request.URL.Path,
		UserID:    c.GetInt("id"),
		TokenID:   c.GetInt("token_id"),
		TokenName: c.GetString("token_name"),
		ClientIP:  c.ClientIP(),
	}
}

func writeFullContentLogEntry(writer *fullContentFileWriter, entry fullContentLogEntry) {
	if err := writer.write(entry); err != nil {
		common.SysError(fmt.Sprintf("failed to write full content log: %v", err))
	}
}

func (w *fullContentFileWriter) write(entry fullContentLogEntry) error {
	encoded, err := common.Marshal(entry)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	fullContentLogLifecycleMu.RLock()
	defer fullContentLogLifecycleMu.RUnlock()

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.ensureFile(int64(len(encoded))); err != nil {
		return err
	}
	n, err := w.file.Write(encoded)
	w.size += int64(n)
	return err
}

func (w *fullContentFileWriter) ensureFile(nextBytes int64) error {
	rotate := w.file == nil
	if !rotate && w.maxBytes > 0 && w.size > 0 && w.size+nextBytes > w.maxBytes {
		rotate = true
	}
	if !rotate {
		return nil
	}

	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
	}

	if err := os.MkdirAll(w.dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(w.dir, 0700); err != nil {
		return err
	}

	filename := fmt.Sprintf("full-content-%s.jsonl", time.Now().UTC().Format("20060102T150405.000000000Z"))
	path := filepath.Join(w.dir, filename)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return err
	}
	w.file = file
	w.size = 0

	if w.maxFiles > 0 {
		w.removeExpiredFiles()
	}
	return nil
}

func (w *fullContentFileWriter) removeExpiredFiles() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && strings.HasPrefix(name, "full-content-") && strings.HasSuffix(name, ".jsonl") {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	for len(files) > w.maxFiles {
		_ = os.Remove(filepath.Join(w.dir, files[0]))
		files = files[1:]
	}
}

func fullContentLogEnabled() bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(fullContentLogEnabledEnv)))
	return err == nil && enabled
}

// FullContentLogEnabled reports whether complete relay content logging is on.
func FullContentLogEnabled() bool {
	return fullContentLogEnabled()
}

// FullContentLogDirectory returns the directory shared by the logger and the
// administrator log viewer.
func FullContentLogDirectory() string {
	logDir := strings.TrimSpace(os.Getenv(fullContentLogDirEnv))
	if logDir != "" {
		return filepath.Clean(logDir)
	}
	baseDir := "logs"
	if common.LogDir != nil && strings.TrimSpace(*common.LogDir) != "" {
		baseDir = strings.TrimSpace(*common.LogDir)
	}
	return filepath.Join(baseDir, "full-content")
}

// FullContentLogFilePath resolves a managed JSONL file without allowing path
// traversal or non-log files.
func FullContentLogFilePath(filename string) (string, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" || filepath.Base(filename) != filename || !strings.HasPrefix(filename, "full-content-") || !strings.HasSuffix(filename, ".jsonl") {
		return "", fmt.Errorf("invalid full content log filename")
	}
	return filepath.Join(FullContentLogDirectory(), filename), nil
}

// DeleteFullContentLogFile safely closes a matching active writer before the
// file is removed. Future writes automatically open a new file.
func DeleteFullContentLogFile(filename string) (int64, error) {
	path, err := FullContentLogFilePath(filename)
	if err != nil {
		return 0, err
	}

	fullContentLogLifecycleMu.Lock()
	defer fullContentLogLifecycleMu.Unlock()
	closeFullContentLogWriters(filename)

	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("full content log is not a regular file")
	}
	if err := os.Remove(path); err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// DeleteAllFullContentLogFiles closes every active writer and removes all
// managed JSONL files while holding the lifecycle lock.
func DeleteAllFullContentLogFiles() (int, int64, error) {
	fullContentLogLifecycleMu.Lock()
	defer fullContentLogLifecycleMu.Unlock()
	closeFullContentLogWriters("")

	entries, err := os.ReadDir(FullContentLogDirectory())
	if os.IsNotExist(err) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}

	deleted := 0
	var freedBytes int64
	for _, entry := range entries {
		path, resolveErr := FullContentLogFilePath(entry.Name())
		if resolveErr != nil {
			continue
		}
		info, infoErr := os.Lstat(path)
		if infoErr != nil || !info.Mode().IsRegular() {
			continue
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return deleted, freedBytes, removeErr
		}
		deleted++
		freedBytes += info.Size()
	}
	return deleted, freedBytes, nil
}

func closeFullContentLogWriters(filename string) {
	fullContentLogWriters.Range(func(key, _ any) bool {
		writer, ok := key.(*fullContentFileWriter)
		if !ok {
			return true
		}
		writer.mu.Lock()
		defer writer.mu.Unlock()
		if writer.file == nil {
			return true
		}
		if filename != "" && filepath.Base(writer.file.Name()) != filename {
			return true
		}
		_ = writer.file.Close()
		writer.file = nil
		writer.size = 0
		return true
	})
}

func fullContentLogEnvInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func encodeFullContentLogBody(contentType string, body []byte, redactJSON bool) (string, string) {
	if redactJSON && (strings.Contains(strings.ToLower(contentType), "json") || looksLikeJSON(body)) {
		var value any
		if err := common.Unmarshal(body, &value); err == nil {
			redactFullContentLogSecrets(value)
			if encoded, err := common.Marshal(value); err == nil {
				return string(encoded), "json"
			}
		}
	}
	if utf8.Valid(body) {
		return string(body), "utf-8"
	}
	return base64.StdEncoding.EncodeToString(body), "base64"
}

func looksLikeJSON(body []byte) bool {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func redactFullContentLogSecrets(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isFullContentLogSecretKey(key) {
				typed[key] = "[REDACTED]"
				continue
			}
			redactFullContentLogSecrets(child)
		}
	case []any:
		for _, child := range typed {
			redactFullContentLogSecrets(child)
		}
	}
}

func isFullContentLogSecretKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	switch normalized {
	case "api_key", "x_api_key", "key", "authorization", "proxy_authorization", "x_auth_token", "cookie", "set_cookie", "token", "access_token", "refresh_token", "password", "secret", "client_secret", "session_secret", "jwt":
		return true
	default:
		return false
	}
}

func redactFullContentLogValues(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	redacted := make(map[string][]string, len(values))
	for key, entries := range values {
		if isFullContentLogSecretKey(key) {
			redacted[key] = []string{"[REDACTED]"}
			continue
		}
		redacted[key] = append([]string(nil), entries...)
	}
	return redacted
}
