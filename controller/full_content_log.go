package controller

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
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
	RequestContentType    string              `json:"request_content_type,omitempty"`
	RequestEncoding       string              `json:"request_encoding,omitempty"`
	RequestBody           string              `json:"request_body"`
	ResponseContentType   string              `json:"response_content_type,omitempty"`
	ResponseEncoding      string              `json:"response_encoding,omitempty"`
	ResponseBody          string              `json:"response_body"`
	ResponseBodyTotalBytes int64              `json:"response_body_total_bytes,omitempty"`
	ResponseBodyTruncated  bool               `json:"response_body_truncated,omitempty"`
	RequestHeaders        map[string][]string `json:"request_headers"`
	ResponseHeaders       map[string][]string `json:"response_headers"`
	Query                 map[string][]string `json:"query"`
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
					record.Body = ""
				} else if record.Encoding == "base64" {
					// Keep a valid base64 prefix while bounding retained input.
					maxEncoded := int((remaining+2)/3) * 4
					if maxEncoded < len(record.Body) {
						maxEncoded -= maxEncoded % 4
						if maxEncoded < 0 {
							maxEncoded = 0
						}
						record.Body = record.Body[:maxEncoded]
					}
					decodedEstimate := int64(len(record.Body) / 4 * 3)
					if decodedEstimate > remaining {
						decodedEstimate = remaining
					}
					collectedResponseBytes += decodedEstimate
				} else {
					if int64(len(record.Body)) > remaining {
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
		FullContentLogSummary: summary,
		RequestContentType:    requestRecord.ContentType,
		RequestEncoding:       requestRecord.Encoding,
		RequestBody:           requestRecord.Body,
		ResponseContentType:   responseContentType,
		ResponseEncoding:      responseEncoding,
		ResponseBody:          responseBody,
		ResponseBodyTotalBytes: responseTotalBytes,
		ResponseBodyTruncated:  responseBodyTruncated,
		RequestHeaders:        requestRecord.Headers,
		ResponseHeaders:       responseHeaders,
		Query:                 requestRecord.Query,
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
