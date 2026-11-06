package controller

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"

	"github.com/gin-gonic/gin"
)

type fullContentLogRecord struct {
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

type FullContentLogSummary struct {
	Timestamp     string `json:"timestamp"`
	RequestID     string `json:"request_id"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	Model         string `json:"model,omitempty"`
	Status        int    `json:"status"`
	DurationMS    int64  `json:"duration_ms"`
	RequestBytes  int64  `json:"request_bytes"`
	ResponseBytes int64  `json:"response_bytes"`
	ChunkCount    int64  `json:"chunk_count"`
	UserID        int    `json:"user_id,omitempty"`
	TokenID       int    `json:"token_id,omitempty"`
	TokenName     string `json:"token_name,omitempty"`
	ClientIP      string `json:"client_ip,omitempty"`
	Error         string `json:"error,omitempty"`
}

type FullContentLogFileInfo struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

type FullContentLogFileStats struct {
	Count     int                      `json:"count"`
	TotalSize int64                    `json:"total_size"`
	Files     []FullContentLogFileInfo `json:"files"`
}

type FullContentLogTokenOption struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type FullContentLogFacets struct {
	Models []string                    `json:"models"`
	Tokens []FullContentLogTokenOption `json:"tokens"`
}

type FullContentLogListResponse struct {
	Enabled  bool                    `json:"enabled"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
	Total    int                     `json:"total"`
	Items    []FullContentLogSummary `json:"items"`
	Files    FullContentLogFileStats `json:"files"`
	Facets   FullContentLogFacets    `json:"facets"`
}

type FullContentLogDetail struct {
	FullContentLogSummary
	RequestContentType     string              `json:"request_content_type,omitempty"`
	RequestEncoding        string              `json:"request_encoding,omitempty"`
	RequestBody            string              `json:"request_body"`
	ResponseContentType    string              `json:"response_content_type,omitempty"`
	ResponseEncoding       string              `json:"response_encoding,omitempty"`
	ResponseBody           string              `json:"response_body"`
	ResponseBodyTotalBytes int64               `json:"response_body_total_bytes,omitempty"`
	ResponseBodyTruncated  bool                `json:"response_body_truncated,omitempty"`
	RequestHeaders         map[string][]string `json:"request_headers"`
	ResponseHeaders        map[string][]string `json:"response_headers"`
	Query                  map[string][]string `json:"query"`
}

type fullContentLogQuery struct {
	Page           int
	PageSize       int
	StartTimestamp int64
	EndTimestamp   int64
	Model          string
	Token          string
	RequestID      string
}

type fullContentLogAggregate struct {
	Summary    FullContentLogSummary
	HasRequest bool
}

// Full-content logs are append-only JSONL files. Keep a small in-process index
// of request summaries so mobile refreshes do not rescan every body and SSE
// chunk on each request. The snapshot is invalidated when a file's size or
// modification time changes.
type fullContentLogSummaryCacheEntry struct {
	signature string
	items     []FullContentLogSummary
}

var fullContentLogSummaryCache = struct {
	sync.RWMutex
	entries map[string]fullContentLogSummaryCacheEntry
}{entries: make(map[string]fullContentLogSummaryCacheEntry)}

func ListFullContentLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	query := fullContentLogQuery{
		Page:           pageInfo.GetPage(),
		PageSize:       pageInfo.GetPageSize(),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		Model:          c.Query("model"),
		Token:          c.Query("token"),
		RequestID:      c.Query("request_id"),
	}

	response, err := queryFullContentLogs(middleware.FullContentLogDirectory(), query)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response.Enabled = middleware.FullContentLogEnabled()
	common.ApiSuccess(c, response)
}

func GetFullContentLogDetail(c *gin.Context) {
	requestID := strings.TrimSpace(c.Param("request_id"))
	if requestID == "" {
		common.ApiErrorMsg(c, "request id is required")
		return
	}
	maxResponseBytes := int64(1024 * 1024)
	if value := strings.TrimSpace(c.Query("max_response_bytes")); value != "" {
		if parsed, parseErr := strconv.ParseInt(value, 10, 64); parseErr == nil {
			maxResponseBytes = parsed
		}
	}
	if maxResponseBytes < 0 {
		maxResponseBytes = 0
	}
	if maxResponseBytes > 16*1024*1024 {
		maxResponseBytes = 16 * 1024 * 1024
	}
	detail, found, err := loadFullContentLogDetailWithLimit(middleware.FullContentLogDirectory(), requestID, maxResponseBytes)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "full content log not found"})
		return
	}
	common.ApiSuccess(c, detail)
}

func ListFullContentLogFiles(c *gin.Context) {
	stats, err := listFullContentLogFiles(middleware.FullContentLogDirectory())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}

func DownloadFullContentLogFile(c *gin.Context) {
	filename := c.Param("filename")
	path, err := middleware.FullContentLogFilePath(filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "full content log file not found"})
		return
	}
	if err != nil || !info.Mode().IsRegular() {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid full content log file"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.FileAttachment(path, filename)
}

func DeleteFullContentLogFile(c *gin.Context) {
	deletedBytes, err := middleware.DeleteFullContentLogFile(c.Param("filename"))
	if os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "full content log file not found"})
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"deleted_bytes": deletedBytes})
}

func DeleteAllFullContentLogFiles(c *gin.Context) {
	deletedCount, freedBytes, err := middleware.DeleteAllFullContentLogFiles()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"deleted_count": deletedCount,
		"freed_bytes":   freedBytes,
	})
}

func queryFullContentLogs(dir string, query fullContentLogQuery) (FullContentLogListResponse, error) {
	files, err := listFullContentLogFiles(dir)
	if err != nil {
		return FullContentLogListResponse{}, err
	}
	signature := fullContentLogFilesSignature(files)
	summaries, found := getCachedFullContentLogSummaries(dir, signature)
	if !found {
		aggregates := make(map[string]*fullContentLogAggregate)
		err = visitFullContentLogRecords(dir, func(record fullContentLogRecord) error {
			aggregate := aggregates[record.RequestID]
			if aggregate == nil {
				aggregate = &fullContentLogAggregate{}
				aggregates[record.RequestID] = aggregate
			}
			switch record.Phase {
			case "request":
				aggregate.HasRequest = true
				aggregate.Summary = FullContentLogSummary{
					Timestamp:    record.Timestamp,
					RequestID:    record.RequestID,
					Method:       record.Method,
					Path:         record.Path,
					Model:        fullContentLogModel(record.Body, record.Encoding),
					RequestBytes: record.BodyBytes,
					UserID:       record.UserID,
					TokenID:      record.TokenID,
					TokenName:    record.TokenName,
					ClientIP:     record.ClientIP,
					Error:        record.Error,
				}
			case "response_chunk":
				if aggregate.Summary.Error == "" && record.Error != "" {
					aggregate.Summary.Error = record.Error
				}
			case "response_end":
				aggregate.Summary.Status = record.Status
				aggregate.Summary.DurationMS = record.DurationMS
				aggregate.Summary.ResponseBytes = record.BodyBytes
				aggregate.Summary.ChunkCount = record.Sequence
				if record.Error != "" {
					aggregate.Summary.Error = record.Error
				}
			}
			return nil
		})
		if err != nil {
			return FullContentLogListResponse{}, err
		}
		summaries = make([]FullContentLogSummary, 0, len(aggregates))
		for _, aggregate := range aggregates {
			if aggregate.HasRequest {
				summaries = append(summaries, aggregate.Summary)
			}
		}
		setCachedFullContentLogSummaries(dir, signature, summaries)
	}

	items := make([]FullContentLogSummary, 0, len(summaries))
	modelSet := make(map[string]struct{})
	tokenSet := make(map[string]FullContentLogTokenOption)
	for _, summary := range summaries {
		if summary.Model != "" {
			modelSet[summary.Model] = struct{}{}
		}
		if summary.TokenID != 0 || summary.TokenName != "" {
			key := fmt.Sprintf("%010d:%s", summary.TokenID, summary.TokenName)
			tokenSet[key] = FullContentLogTokenOption{ID: summary.TokenID, Name: summary.TokenName}
		}
		if !matchesFullContentLogQuery(summary, query) {
			continue
		}
		items = append(items, summary)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Timestamp > items[j].Timestamp
	})

	total := len(items)
	start := (query.Page - 1) * query.PageSize
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	end := start + query.PageSize
	if end > total {
		end = total
	}
	items = items[start:end]

	models := make([]string, 0, len(modelSet))
	for model := range modelSet {
		models = append(models, model)
	}
	sort.Strings(models)
	tokens := make([]FullContentLogTokenOption, 0, len(tokenSet))
	for _, token := range tokenSet {
		tokens = append(tokens, token)
	}
	sort.Slice(tokens, func(i, j int) bool {
		left := strings.ToLower(tokens[i].Name)
		right := strings.ToLower(tokens[j].Name)
		if left == right {
			return tokens[i].ID < tokens[j].ID
		}
		return left < right
	})
	return FullContentLogListResponse{
		Page:     query.Page,
		PageSize: query.PageSize,
		Total:    total,
		Items:    items,
		Files:    files,
		Facets:   FullContentLogFacets{Models: models, Tokens: tokens},
	}, nil
}

func fullContentLogFilesSignature(stats FullContentLogFileStats) string {
	var builder strings.Builder
	for _, file := range stats.Files {
		builder.WriteString(file.Name)
		builder.WriteByte(':')
		builder.WriteString(strconv.FormatInt(file.Size, 10))
		builder.WriteByte(':')
		builder.WriteString(strconv.FormatInt(file.ModifiedAt.UnixNano(), 10))
		builder.WriteByte(';')
	}
	return builder.String()
}

func getCachedFullContentLogSummaries(dir string, signature string) ([]FullContentLogSummary, bool) {
	fullContentLogSummaryCache.RLock()
	entry, ok := fullContentLogSummaryCache.entries[dir]
	fullContentLogSummaryCache.RUnlock()
	if !ok || entry.signature != signature {
		return nil, false
	}
	items := append([]FullContentLogSummary(nil), entry.items...)
	return items, true
}

func setCachedFullContentLogSummaries(dir string, signature string, items []FullContentLogSummary) {
	fullContentLogSummaryCache.Lock()
	fullContentLogSummaryCache.entries[dir] = fullContentLogSummaryCacheEntry{
		signature: signature,
		items:     append([]FullContentLogSummary(nil), items...),
	}
	fullContentLogSummaryCache.Unlock()
}

func loadFullContentLogDetail(dir string, requestID string) (FullContentLogDetail, bool, error) {
	return loadFullContentLogDetailWithLimit(dir, requestID, 0)
}

func loadFullContentLogDetailWithLimit(dir string, requestID string, maxResponseBytes int64) (FullContentLogDetail, bool, error) {
	var requestRecord *fullContentLogRecord
	var endRecord *fullContentLogRecord
	chunks := make([]fullContentLogRecord, 0)
	var collectedResponseBytes int64
	collectorTruncated := false
	err := visitFullContentLogRecords(dir, func(record fullContentLogRecord) error {
		if record.RequestID != requestID {
			return nil
		}
		switch record.Phase {
		case "request":
			copy := record
			requestRecord = &copy
		case "response_chunk":
			if maxResponseBytes > 0 {
				remaining := maxResponseBytes - collectedResponseBytes
				if remaining <= 0 {
					collectorTruncated = collectorTruncated || record.Body != ""
					record.Body = ""
				} else if record.Encoding == "base64" {
					// Keep a valid base64 prefix while bounding retained input.
					decodedBytes := int64(base64.StdEncoding.DecodedLen(len(record.Body)))
					if strings.HasSuffix(record.Body, "==") {
						decodedBytes -= 2
					} else if strings.HasSuffix(record.Body, "=") {
						decodedBytes--
					}
					collectorTruncated = collectorTruncated || decodedBytes > remaining
					maxEncoded := int((remaining+2)/3) * 4
					if maxEncoded < len(record.Body) {
						maxEncoded -= maxEncoded % 4
						if maxEncoded < 0 {
							maxEncoded = 0
						}
						record.Body = record.Body[:maxEncoded]
					}
					decodedEstimate := decodedBytes
					if decodedEstimate > remaining {
						decodedEstimate = remaining
					}
					collectedResponseBytes += decodedEstimate
				} else {
					if int64(len(record.Body)) > remaining {
						collectorTruncated = true
						record.Body = truncateUTF8Prefix(record.Body, remaining)
					}
					collectedResponseBytes += int64(len(record.Body))
				}
			}
			chunks = append(chunks, record)
		case "response_end":
			copy := record
			endRecord = &copy
		}
		return nil
	})
	if err != nil {
		return FullContentLogDetail{}, false, err
	}
	if requestRecord == nil {
		return FullContentLogDetail{}, false, nil
	}

	sort.SliceStable(chunks, func(i, j int) bool {
		return chunks[i].Sequence < chunks[j].Sequence
	})
	responseBody, responseEncoding, responseContentType, responseBodyTruncated, err := joinFullContentLogChunksWithLimit(chunks, maxResponseBytes)
	if err != nil {
		return FullContentLogDetail{}, false, err
	}

	summary := FullContentLogSummary{
		Timestamp:    requestRecord.Timestamp,
		RequestID:    requestRecord.RequestID,
		Method:       requestRecord.Method,
		Path:         requestRecord.Path,
		Model:        fullContentLogModel(requestRecord.Body, requestRecord.Encoding),
		RequestBytes: requestRecord.BodyBytes,
		UserID:       requestRecord.UserID,
		TokenID:      requestRecord.TokenID,
		TokenName:    requestRecord.TokenName,
		ClientIP:     requestRecord.ClientIP,
		Error:        requestRecord.Error,
	}
	if endRecord != nil {
		summary.Status = endRecord.Status
		summary.DurationMS = endRecord.DurationMS
		summary.ResponseBytes = endRecord.BodyBytes
		summary.ChunkCount = endRecord.Sequence
		if endRecord.Error != "" {
			summary.Error = endRecord.Error
		}
	}

	var responseHeaders map[string][]string
	if endRecord != nil {
		responseHeaders = endRecord.Headers
	}
	responseTotalBytes := int64(len(responseBody))
	if endRecord != nil && endRecord.BodyBytes > 0 {
		responseTotalBytes = endRecord.BodyBytes
	}
	return FullContentLogDetail{
		FullContentLogSummary:  summary,
		RequestContentType:     requestRecord.ContentType,
		RequestEncoding:        requestRecord.Encoding,
		RequestBody:            requestRecord.Body,
		ResponseContentType:    responseContentType,
		ResponseEncoding:       responseEncoding,
		ResponseBody:           responseBody,
		ResponseBodyTotalBytes: responseTotalBytes,
		ResponseBodyTruncated:  responseBodyTruncated || collectorTruncated,
		RequestHeaders:         requestRecord.Headers,
		ResponseHeaders:        responseHeaders,
		Query:                  requestRecord.Query,
	}, true, nil
}

func truncateUTF8Prefix(value string, maxBytes int64) string {
	if maxBytes <= 0 {
		return ""
	}
	if int64(len(value)) <= maxBytes {
		return value
	}
	limit := int(maxBytes)
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		_, size := utf8.DecodeLastRuneInString(value[:limit])
		if size <= 0 || size > limit {
			break
		}
		limit -= size
	}
	return value[:limit]
}

func truncateFullContentLogBody(body string, encoding string, maxBytes int64) (string, bool) {
	if maxBytes <= 0 {
		return body, false
	}
	if encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(body)
		if err != nil || int64(len(decoded)) <= maxBytes {
			return body, false
		}
		return base64.StdEncoding.EncodeToString(decoded[:int(maxBytes)]), true
	}
	if int64(len(body)) <= maxBytes {
		return body, false
	}
	limit := int(maxBytes)
	if !utf8.ValidString(body[:limit]) {
		for limit > 0 && !utf8.ValidString(body[:limit]) {
			_, size := utf8.DecodeLastRuneInString(body[:limit])
			if size <= 0 || size > limit {
				break
			}
			limit -= size
		}
	}
	return body[:limit], true
}

func visitFullContentLogRecords(dir string, visit func(fullContentLogRecord) error) error {
	files, err := fullContentLogFilePaths(dir)
	if err != nil {
		return err
	}
	for _, path := range files {
		file, openErr := os.Open(path)
		if openErr != nil {
			if os.IsNotExist(openErr) {
				continue
			}
			return openErr
		}
		reader := bufio.NewReader(file)
		for {
			line, readErr := reader.ReadBytes('\n')
			if len(bytes.TrimSpace(line)) > 0 {
				var record fullContentLogRecord
				if unmarshalErr := common.Unmarshal(bytes.TrimSpace(line), &record); unmarshalErr == nil && record.RequestID != "" {
					if visitErr := visit(record); visitErr != nil {
						_ = file.Close()
						return visitErr
					}
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					_ = file.Close()
					return readErr
				}
				break
			}
		}
		if closeErr := file.Close(); closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func fullContentLogFilePaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type().IsRegular() && isFullContentLogFilename(entry.Name()) {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func listFullContentLogFiles(dir string) (FullContentLogFileStats, error) {
	paths, err := fullContentLogFilePaths(dir)
	if err != nil {
		return FullContentLogFileStats{}, err
	}
	files := make([]FullContentLogFileInfo, 0, len(paths))
	var totalSize int64
	for _, path := range paths {
		info, statErr := os.Stat(path)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return FullContentLogFileStats{}, statErr
		}
		files = append(files, FullContentLogFileInfo{
			Name:       filepath.Base(path),
			Size:       info.Size(),
			ModifiedAt: info.ModTime(),
		})
		totalSize += info.Size()
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name > files[j].Name
	})
	return FullContentLogFileStats{Count: len(files), TotalSize: totalSize, Files: files}, nil
}

func matchesFullContentLogQuery(summary FullContentLogSummary, query fullContentLogQuery) bool {
	if !containsFold(summary.Model, query.Model) || !containsFold(summary.RequestID, query.RequestID) {
		return false
	}
	if query.Token != "" {
		tokenID := strconv.Itoa(summary.TokenID)
		if !containsFold(summary.TokenName, query.Token) && tokenID != strings.TrimSpace(query.Token) {
			return false
		}
	}
	if query.StartTimestamp == 0 && query.EndTimestamp == 0 {
		return true
	}
	timestamp, err := time.Parse(time.RFC3339Nano, summary.Timestamp)
	if err != nil {
		return false
	}
	unix := timestamp.Unix()
	return (query.StartTimestamp == 0 || unix >= query.StartTimestamp) && (query.EndTimestamp == 0 || unix <= query.EndTimestamp)
}

func fullContentLogModel(body string, encoding string) string {
	if encoding != "json" || strings.TrimSpace(body) == "" {
		return ""
	}
	var payload map[string]any
	if err := common.Unmarshal([]byte(body), &payload); err != nil {
		return ""
	}
	model, _ := payload["model"].(string)
	return model
}

func joinFullContentLogChunks(chunks []fullContentLogRecord) (string, string, string, error) {
	body, encoding, contentType, _, err := joinFullContentLogChunksWithLimit(chunks, 0)
	return body, encoding, contentType, err
}

// joinFullContentLogChunksWithLimit combines response chunks while enforcing
// a decoded-byte limit. This keeps detail requests bounded even when a single
// request contains a very large SSE stream; the summary still reports the
// complete byte count from the response_end record.
func joinFullContentLogChunksWithLimit(chunks []fullContentLogRecord, maxBytes int64) (string, string, string, bool, error) {
	var combined bytes.Buffer
	allUTF8 := true
	contentType := ""
	var written int64
	truncated := false
	for _, chunk := range chunks {
		if contentType == "" {
			contentType = chunk.ContentType
		}
		if chunk.Encoding == "base64" {
			decoded, err := base64.StdEncoding.DecodeString(chunk.Body)
			if err != nil {
				return "", "", "", false, fmt.Errorf("invalid base64 response chunk: %w", err)
			}
			allUTF8 = false
			if maxBytes > 0 && written+int64(len(decoded)) > maxBytes {
				remaining := maxBytes - written
				if remaining > 0 {
					combined.Write(decoded[:remaining])
					written += remaining
				}
				truncated = true
				continue
			}
			combined.Write(decoded)
			written += int64(len(decoded))
			continue
		}
		body := chunk.Body
		if maxBytes > 0 && written+int64(len(body)) > maxBytes {
			remaining := maxBytes - written
			if remaining > 0 {
				body = body[:remaining]
				for !utf8.ValidString(body) && len(body) > 0 {
					_, size := utf8.DecodeLastRuneInString(body)
					body = body[:len(body)-size]
				}
				combined.WriteString(body)
			}
			written = maxBytes
			truncated = true
			continue
		}
		combined.WriteString(body)
		written += int64(len(body))
	}
	if allUTF8 {
		return combined.String(), "utf-8", contentType, truncated, nil
	}
	return base64.StdEncoding.EncodeToString(combined.Bytes()), "base64", contentType, truncated, nil
}

func containsFold(value string, filter string) bool {
	filter = strings.TrimSpace(filter)
	return filter == "" || strings.Contains(strings.ToLower(value), strings.ToLower(filter))
}

func isFullContentLogFilename(filename string) bool {
	return filepath.Base(filename) == filename && strings.HasPrefix(filename, "full-content-") && strings.HasSuffix(filename, ".jsonl")
}

// Saved segments deliberately do not use the preview loader: even one saved
// chunk can exceed the preview limit. Only record metadata and one page are kept.
type fullContentLogSegment struct {
	RequestID       string  `json:"request_id"`
	Part            string  `json:"part"`
	RequestedOffset string  `json:"requested_offset"`
	Offset          string  `json:"offset"`
	NextOffset      *string `json:"next_offset"`
	TotalBytes      string  `json:"total_bytes"`
	ExpectedBytes   *string `json:"expected_bytes"`
	HasMore         bool    `json:"has_more"`
	Snapshot        string  `json:"snapshot"`
	Encoding        string  `json:"encoding"`
	ContentType     string  `json:"content_type"`
	Body            string  `json:"body"`
	SourceComplete  *bool   `json:"source_complete"`
}

type savedLogSegmentError struct {
	status int
	code   string
}

func (e *savedLogSegmentError) Error() string { return e.code }
func savedLogConflict(code string) error {
	return &savedLogSegmentError{http.StatusConflict, "FULL_CONTENT_SEGMENT_" + code}
}

// GetFullContentLogSegment reads only locally saved bytes. The enclosing router
// retains AdminAuth and DisableCache; the snapshot is a version, not authority.
func GetFullContentLogSegment(c *gin.Context) {
	part, rawOffset, rawLimit, snapshot := c.Query("part"), c.DefaultQuery("offset", "0"), c.DefaultQuery("limit", "1048576"), c.Query("snapshot")
	offset, offsetErr := strconv.ParseInt(rawOffset, 10, 64)
	limit, limitErr := strconv.ParseInt(rawLimit, 10, 64)
	validSnapshot := snapshot == ""
	if len(snapshot) == 64 {
		_, err := hex.DecodeString(snapshot)
		validSnapshot = err == nil && snapshot == strings.ToLower(snapshot)
	}
	if (part != "request" && part != "response") || offsetErr != nil || offset < 0 || strconv.FormatInt(offset, 10) != rawOffset || limitErr != nil || limit < 4 || limit > 1<<20 || strconv.FormatInt(limit, 10) != rawLimit || !validSnapshot || (offset > 0 && snapshot == "") || strings.TrimSpace(c.Param("request_id")) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "FULL_CONTENT_SEGMENT_INVALID", "message": common.TranslateMessage(c, i18n.MsgInvalidParams)})
		return
	}
	// Length-prefixing via the JSON wrapper makes actor/session components
	// unambiguous without depending on any secret or process-local cursor state.
	actor, err := common.Marshal([]any{c.GetInt("id"), c.GetInt("role"), c.GetString("session_id")})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "FULL_CONTENT_SEGMENT_READ_FAILED", "message": common.TranslateMessage(c, i18n.MsgOperationFailed)})
		return
	}
	page, err := readFullContentLogSegment(c.Request.Context(), middleware.FullContentLogDirectory(), c.Param("request_id"), part, offset, limit, snapshot, string(actor))
	if err != nil {
		if c.Request.Context().Err() != nil {
			return
		}
		status, code, message := http.StatusInternalServerError, "FULL_CONTENT_SEGMENT_READ_FAILED", i18n.MsgOperationFailed
		var segmentErr *savedLogSegmentError
		if errors.As(err, &segmentErr) {
			status, code = segmentErr.status, segmentErr.code
			if status == http.StatusNotFound {
				message = i18n.MsgNotFound
			}
		}
		c.JSON(status, gin.H{"success": false, "code": code, "message": common.TranslateMessage(c, message)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": page})
}

type savedLogRecord struct {
	RequestID                    string  `json:"request_id"`
	Phase                        string  `json:"phase"`
	Sequence                     *int64  `json:"sequence"`
	BodyBytes                    *int64  `json:"body_bytes"`
	Encoding                     string  `json:"encoding"`
	ContentType                  string  `json:"content_type"`
	Capture                      string  `json:"capture_state"`
	Error                        string  `json:"error"`
	Body                         *string `json:"body"`
	path                         string
	fileInfo                     os.FileInfo
	bodyOffset, bodyLength, size int64
	digest                       [32]byte
}

type savedLogSnapshot struct {
	chunks                        []savedLogRecord
	total                         int64
	expected                      *int64
	complete                      *bool
	encoding, contentType, digest string
}

// savedLogLine exposes a single JSONL record without buffering the whole line.
// A writer's unfinished final line is distinguishable from a complete record.
type savedLogLine struct {
	source               *bufio.Reader
	pending              []byte
	done                 bool
	err                  error
	rawLength, delivered int64
	digest               hash.Hash
	ctx                  context.Context
}

func (l *savedLogLine) Read(p []byte) (int, error) {
	if err := l.ctx.Err(); err != nil {
		return 0, err
	}
	if len(l.pending) == 0 && !l.done {
		fragment, err := l.source.ReadSlice('\n')
		l.rawLength += int64(len(fragment))
		if len(fragment) > 0 && fragment[len(fragment)-1] == '\n' {
			fragment = fragment[:len(fragment)-1]
			l.done = true
		}
		if err != nil && err != bufio.ErrBufferFull {
			l.done = true
			if err != io.EOF {
				l.err = err
			}
		}
		l.pending = fragment
	}
	if len(l.pending) == 0 {
		if l.err != nil {
			return 0, l.err
		}
		return 0, io.EOF
	}
	n := copy(p, l.pending)
	l.pending = l.pending[n:]
	l.delivered += int64(n)
	_, _ = l.digest.Write(p[:n])
	return n, nil
}

// streamSavedLogString is only a JSONL body locator/string reader. The complete
// projected record is validated and decoded by common.* below. Small escape
// tokens are decoded with common.Unmarshal, including paired UTF-16 surrogates.
// raw optionally preserves a metadata token; decoded receives bounded chunks.
func streamSavedLogString(r *bufio.Reader, raw, decoded io.Writer) error {
	opening, err := r.ReadByte()
	if err != nil {
		return err
	}
	if opening != '"' {
		return savedLogConflict("CORRUPT")
	}
	if raw != nil {
		if _, err := raw.Write([]byte{'"'}); err != nil {
			return err
		}
	}
	for {
		available, err := r.Peek(1)
		if err != nil {
			return err
		}
		available, _ = r.Peek(r.Buffered())
		n := 0
		for n < len(available) && available[n] != '"' && available[n] != '\\' && available[n] >= 0x20 {
			n++
		}
		if n > 0 {
			// A read buffer may end in the middle of a UTF-8 rune.
			if !utf8.Valid(available[:n]) {
				for n > 0 && !utf8.Valid(available[:n]) && len(available)-n < utf8.UTFMax {
					n--
				}
				if n == 0 {
					peek, _ := r.Peek(utf8.UTFMax)
					_, size := utf8.DecodeRune(peek)
					if size <= 1 {
						return savedLogConflict("CORRUPT")
					}
					available, n = peek, size
				} else if !utf8.Valid(available[:n]) {
					return savedLogConflict("CORRUPT")
				}
			}
			if raw != nil {
				if _, err := raw.Write(available[:n]); err != nil {
					return err
				}
			}
			if decoded != nil {
				if _, err := decoded.Write(available[:n]); err != nil {
					return err
				}
			}
			_, _ = r.Discard(n)
			continue
		}
		ch, _ := r.ReadByte()
		if ch == '"' {
			if raw != nil {
				_, err = raw.Write([]byte{'"'})
			}
			return err
		}
		if ch != '\\' {
			return savedLogConflict("CORRUPT")
		}
		token := []byte{'"', '\\'}
		escape, err := r.ReadByte()
		if err != nil {
			return err
		}
		token = append(token, escape)
		if escape == 'u' {
			var digits [4]byte
			if _, err := io.ReadFull(r, digits[:]); err != nil {
				return err
			}
			token = append(token, digits[:]...)
			code, err := strconv.ParseUint(string(digits[:]), 16, 16)
			if err != nil {
				return savedLogConflict("CORRUPT")
			}
			if code >= 0xd800 && code <= 0xdbff {
				peek, _ := r.Peek(6)
				if len(peek) == 6 && peek[0] == '\\' && peek[1] == 'u' {
					low, err := strconv.ParseUint(string(peek[2:]), 16, 16)
					if err == nil && low >= 0xdc00 && low <= 0xdfff {
						token = append(token, peek...)
						_, _ = r.Discard(6)
					}
				}
			}
		}
		token = append(token, '"')
		var value string
		if err := common.Unmarshal(token, &value); err != nil {
			return savedLogConflict("CORRUPT")
		}
		if raw != nil {
			if _, err := raw.Write(token[1 : len(token)-1]); err != nil {
				return err
			}
		}
		if decoded != nil {
			if _, err := io.WriteString(decoded, value); err != nil {
				return err
			}
		}
	}
}

// Only top-level body strings are replaced. All other syntax and metadata,
// including unknown nested fields and duplicate body keys, reach common's full
// JSON validation unchanged. Raw record hashing also includes the saved body.
func projectSavedLogRecord(line *savedLogLine, base int64, target string) (savedLogRecord, bool, error) {
	r := bufio.NewReaderSize(line, 32<<10)
	var metadata bytes.Buffer
	record := savedLogRecord{}
	depth, last, key, matched := 0, byte(0), "", false
	for {
		ch, err := r.Peek(1)
		if err == io.EOF {
			break
		}
		if err != nil {
			return record, matched, err
		}
		if ch[0] == '"' {
			isKey := depth == 1 && (last == '{' || last == ',')
			isBody := depth == 1 && last == ':' && key == "body"
			start := base + line.delivered - int64(r.Buffered())
			var token bytes.Buffer
			var raw io.Writer = &metadata
			if isKey || (depth == 1 && last == ':' && key == "request_id") {
				raw = &token
			}
			if isBody {
				raw = nil
				metadata.WriteString(`""`)
			}
			if err := streamSavedLogString(r, raw, nil); err != nil {
				return record, matched, err
			}
			if raw == &token {
				metadata.Write(token.Bytes())
				var value string
				if err := common.Unmarshal(token.Bytes(), &value); err != nil {
					return record, matched, err
				}
				if isKey {
					key = value
				} else {
					matched = matched || value == target
				}
			}
			if isBody {
				record.bodyOffset = start
				record.bodyLength = base + line.delivered - int64(r.Buffered()) - start
			}
			last = '"'
			continue
		}
		chByte, _ := r.ReadByte()
		metadata.WriteByte(chByte)
		switch chByte {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
		if chByte != ' ' && chByte != '\t' && chByte != '\r' {
			last = chByte
		}
	}
	if line.err != nil {
		return record, matched, line.err
	}
	if err := common.ValidateUniqueJSONKeys(metadata.Bytes()); err != nil {
		return record, matched, savedLogConflict("CORRUPT")
	}
	if err := common.Unmarshal(metadata.Bytes(), &record); err != nil {
		return record, matched, savedLogConflict("CORRUPT")
	}
	if record.Body != nil && *record.Body != "" {
		return record, matched, savedLogConflict("CORRUPT")
	}
	copy(record.digest[:], line.digest.Sum(nil))
	return record, matched || record.RequestID == target, nil
}

// The JSON string can contain escaped newlines in base64. Keep only one quartet
// pending so binary decoding never allocates a complete saved chunk.
type savedLogBase64Writer struct {
	output  io.Writer
	quartet [4]byte
	count   int
	ended   bool
}

func (w *savedLogBase64Writer) Write(p []byte) (int, error) {
	for _, ch := range p {
		if ch == '\r' || ch == '\n' {
			continue
		}
		if w.ended {
			return 0, savedLogConflict("CORRUPT")
		}
		w.quartet[w.count] = ch
		w.count++
		if w.count == 4 {
			var decoded [3]byte
			n, err := base64.StdEncoding.Decode(decoded[:], w.quartet[:])
			if err != nil {
				return 0, savedLogConflict("CORRUPT")
			}
			if _, err := w.output.Write(decoded[:n]); err != nil {
				return 0, err
			}
			w.ended = w.quartet[3] == '='
			w.count = 0
		}
	}
	return len(p), nil
}

type savedLogByteRange struct {
	position, start, end int64
	page                 []byte
}

func (w *savedLogByteRange) Write(p []byte) (int, error) {
	if int64(len(p)) > math.MaxInt64-w.position {
		return 0, savedLogConflict("CORRUPT")
	}
	from, to := max(w.start-w.position, 0), min(w.end-w.position, int64(len(p)))
	if from < to {
		w.page = append(w.page, p[from:to]...)
	}
	w.position += int64(len(p))
	return len(p), nil
}

type savedLogContextWriter struct {
	ctx    context.Context
	output io.Writer
}

func (w savedLogContextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.output.Write(p)
}

func decodeSavedLogBody(ctx context.Context, file *os.File, record savedLogRecord, output io.Writer) error {
	output = savedLogContextWriter{ctx, output}
	if err := ctx.Err(); err != nil {
		return err
	}
	if record.bodyLength == 0 {
		return nil
	}
	r := bufio.NewReaderSize(io.NewSectionReader(file, record.bodyOffset, record.bodyLength), 32<<10)
	if record.Encoding == "base64" {
		writer := &savedLogBase64Writer{output: output}
		if err := streamSavedLogString(r, nil, writer); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return savedLogConflict("CORRUPT")
			}
			return err
		}
		if writer.count != 0 {
			return savedLogConflict("CORRUPT")
		}
		return ctx.Err()
	}
	if record.Encoding != "" && record.Encoding != "utf-8" && record.Encoding != "json" && record.Encoding != "omitted_playground_content" {
		return savedLogConflict("CORRUPT")
	}
	if err := streamSavedLogString(r, nil, output); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return savedLogConflict("CORRUPT")
		}
		return err
	}
	return ctx.Err()
}

func scanSavedLogSnapshot(ctx context.Context, dir, requestID, part, actor string) (savedLogSnapshot, error) {
	result := savedLogSnapshot{encoding: "utf-8"}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	paths, err := fullContentLogFilePaths(dir)
	if err != nil {
		return result, err
	}
	version := sha256.New()
	identity, err := common.Marshal([]string{requestID, part, actor})
	if err != nil {
		return result, err
	}
	_, _ = version.Write(identity)
	requests, ends := 0, 0
	var request, end *savedLogRecord
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		before, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return result, err
		}
		if !before.Mode().IsRegular() {
			continue
		}
		file, err := os.Open(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return result, err
		}
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) {
			_ = file.Close()
			return result, savedLogConflict("CHANGED")
		}
		r := bufio.NewReaderSize(file, 32<<10)
		var position int64
		for {
			if _, err := r.Peek(1); err != nil {
				if err != io.EOF {
					_ = file.Close()
					return result, err
				}
				break
			}
			line := &savedLogLine{source: r, digest: sha256.New(), ctx: ctx}
			record, matched, parseErr := projectSavedLogRecord(line, position, requestID)
			// Drain malformed/unrelated lines without retaining their bytes.
			_, drainErr := io.Copy(io.Discard, line)
			position += line.rawLength
			if drainErr != nil {
				_ = file.Close()
				return result, drainErr
			}
			if !matched {
				continue
			}
			if parseErr != nil {
				_ = file.Close()
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				return result, savedLogConflict("CORRUPT")
			}
			if record.RequestID != requestID || (record.BodyBytes != nil && *record.BodyBytes < 0) || (record.Sequence != nil && *record.Sequence < 0) {
				_ = file.Close()
				return result, savedLogConflict("CORRUPT")
			}
			record.path, record.fileInfo = path, info
			counter := &savedLogByteRange{}
			if err := decodeSavedLogBody(ctx, file, record, counter); err != nil {
				_ = file.Close()
				return result, err
			}
			record.size = counter.position
			_, _ = version.Write(record.digest[:])
			switch record.Phase {
			case "request":
				requests++
				copy := record
				request = &copy
			case "response_end":
				ends++
				copy := record
				end = &copy
			case "response_chunk":
				result.chunks = append(result.chunks, record)
			}
		}
		if err := file.Close(); err != nil {
			return result, err
		}
	}
	if requests > 1 || ends > 1 {
		return result, savedLogConflict("AMBIGUOUS")
	}
	if request == nil || (part == "response" && len(result.chunks) == 0 && end == nil) {
		return result, &savedLogSegmentError{http.StatusNotFound, "FULL_CONTENT_SEGMENT_NOT_FOUND"}
	}
	if part == "request" {
		result.chunks = []savedLogRecord{*request}
		result.expected = request.BodyBytes
	} else {
		sort.SliceStable(result.chunks, func(i, j int) bool { return savedLogSequence(result.chunks[i]) < savedLogSequence(result.chunks[j]) })
		if end != nil {
			result.expected = end.BodyBytes
		}
	}
	knownSequence, contiguous, captureComplete := true, true, true
	for i, record := range result.chunks {
		if record.size > math.MaxInt64-result.total {
			return result, savedLogConflict("CORRUPT")
		}
		result.total += record.size
		if record.Encoding == "base64" {
			result.encoding = "base64"
		}
		if result.contentType == "" {
			result.contentType = record.ContentType
		}
		if record.Sequence == nil {
			knownSequence = false
		} else if *record.Sequence != int64(i+1) {
			contiguous = false
		}
		if (record.Capture != "" && record.Capture != "complete") || record.Encoding == "omitted_playground_content" || record.Error != "" {
			captureComplete = false
		}
		if record.BodyBytes != nil && *record.BodyBytes != record.size {
			captureComplete = false
		}
	}
	if part == "request" && result.expected != nil {
		complete := result.total == *result.expected && captureComplete
		result.complete = &complete
	} else if part == "response" && end != nil && result.expected != nil {
		if (end.Capture != "" && end.Capture != "complete") || end.Error != "" {
			captureComplete = false
		}
		if end.Sequence != nil && knownSequence {
			complete := contiguous && *end.Sequence == int64(len(result.chunks)) && result.total == *result.expected && captureComplete
			result.complete = &complete
		}
	}
	result.digest = hex.EncodeToString(version.Sum(nil))
	return result, nil
}
func savedLogSequence(record savedLogRecord) int64 {
	if record.Sequence == nil {
		return 0
	}
	return *record.Sequence
}

func readFullContentLogSegment(ctx context.Context, dir, requestID, part string, offset, limit int64, snapshot, actor string) (fullContentLogSegment, error) {
	page := fullContentLogSegment{RequestID: requestID, Part: part, RequestedOffset: strconv.FormatInt(offset, 10)}
	first, err := scanSavedLogSnapshot(ctx, dir, requestID, part, actor)
	if err != nil {
		var segmentErr *savedLogSegmentError
		if snapshot != "" && errors.As(err, &segmentErr) && segmentErr.status == http.StatusNotFound {
			return page, savedLogConflict("CHANGED")
		}
		return page, err
	}
	if snapshot != "" && snapshot != first.digest {
		return page, savedLogConflict("CHANGED")
	}
	return readSavedSnapshotSegment(ctx, dir, requestID, part, offset, limit, actor, first)
}

// readSavedSnapshotSegment only publishes a page after revalidating the saved
// records against the first snapshot, even when file identity and size survive.
func readSavedSnapshotSegment(ctx context.Context, dir, requestID, part string, offset, limit int64, actor string, first savedLogSnapshot) (fullContentLogSegment, error) {
	page := fullContentLogSegment{RequestID: requestID, Part: part, RequestedOffset: strconv.FormatInt(offset, 10)}
	if offset > first.total {
		return page, &savedLogSegmentError{http.StatusRequestedRangeNotSatisfiable, "FULL_CONTENT_SEGMENT_RANGE"}
	}
	start := offset
	if first.encoding == "utf-8" {
		start = max(offset-3, 0)
	}
	end := first.total
	if limit <= math.MaxInt64-offset {
		end = min(end, offset+limit)
	}
	collector := &savedLogByteRange{start: start, end: end}
	for _, record := range first.chunks {
		if collector.position+record.size > start && collector.position < end {
			file, err := os.Open(record.path)
			if err != nil {
				return page, savedLogConflict("CHANGED")
			}
			info, err := file.Stat()
			if err != nil || !os.SameFile(info, record.fileInfo) {
				_ = file.Close()
				return page, savedLogConflict("CHANGED")
			}
			err = decodeSavedLogBody(ctx, file, record, collector)
			closeErr := file.Close()
			if err != nil {
				return page, err
			}
			if closeErr != nil {
				return page, closeErr
			}
		} else {
			collector.position += record.size
		}
	}
	second, err := scanSavedLogSnapshot(ctx, dir, requestID, part, actor)
	if err != nil {
		var segmentErr *savedLogSegmentError
		if errors.As(err, &segmentErr) && segmentErr.status == http.StatusNotFound {
			return page, savedLogConflict("CHANGED")
		}
		return page, err
	}
	if first.digest != second.digest {
		return page, savedLogConflict("CHANGED")
	}
	body, actual := collector.page, offset
	if first.encoding == "utf-8" && offset < first.total {
		i := int(offset - start)
		if i >= len(body) {
			return page, savedLogConflict("CHANGED")
		}
		for i > 0 && !utf8.RuneStart(body[i]) {
			i--
		}
		actual = start + int64(i)
		body = body[i:]
		if int64(len(body)) > limit {
			body = body[:limit]
		}
		for len(body) > 0 && !utf8.Valid(body) {
			body = body[:len(body)-1]
		}
		if len(body) == 0 {
			return page, savedLogConflict("CORRUPT")
		}
	} else if offset == first.total {
		body = nil
	}
	page.Offset, page.TotalBytes, page.Snapshot = strconv.FormatInt(actual, 10), strconv.FormatInt(first.total, 10), first.digest
	page.Encoding, page.ContentType, page.SourceComplete = first.encoding, first.contentType, first.complete
	if first.expected != nil {
		expected := strconv.FormatInt(*first.expected, 10)
		page.ExpectedBytes = &expected
	}
	page.HasMore = actual+int64(len(body)) < first.total
	if page.HasMore {
		next := strconv.FormatInt(actual+int64(len(body)), 10)
		page.NextOffset = &next
	}
	if first.encoding == "base64" {
		page.Body = base64.StdEncoding.EncodeToString(body)
	} else {
		page.Body = string(body)
	}
	return page, nil
}
