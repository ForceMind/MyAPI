package controller

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFullContentLogTestFile(t *testing.T, dir string, records []fullContentLogRecord) string {
	t.Helper()
	lines := make([]string, 0, len(records))
	for _, record := range records {
		encoded, err := common.Marshal(record)
		require.NoError(t, err)
		lines = append(lines, string(encoded))
	}
	path := filepath.Join(dir, "full-content-20260827T120000.000000000Z.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600))
	return path
}

func TestQueryFullContentLogsFiltersAndPaginates(t *testing.T) {
	dir := t.TempDir()
	writeFullContentLogTestFile(t, dir, []fullContentLogRecord{
		{
			Timestamp: "2026-08-27T10:00:00Z", RequestID: "older-request", Phase: "request",
			Method: "POST", Path: "/v1/responses", Encoding: "json",
			Body: `{"model":"gpt-5.6-sol","input":"old"}`, BodyBytes: 42,
			TokenID: 3, TokenName: "secondary",
		},
		{Timestamp: "2026-08-27T10:00:01Z", RequestID: "older-request", Phase: "response_end", Status: 200, Sequence: 1, BodyBytes: 5, DurationMS: 120},
		{
			Timestamp: "2026-08-27T11:00:00Z", RequestID: "newer-request", Phase: "request",
			Method: "POST", Path: "/v1/chat/completions", Encoding: "json",
			Body: `{"model":"gpt-5.6-luna","messages":[]}`, BodyBytes: 45,
			TokenID: 5, TokenName: "primary-test",
		},
		{Timestamp: "2026-08-27T11:00:01Z", RequestID: "newer-request", Phase: "response_end", Status: 200, Sequence: 4, BodyBytes: 321, DurationMS: 456},
	})

	response, err := queryFullContentLogs(dir, fullContentLogQuery{Page: 1, PageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, 2, response.Total)
	require.Len(t, response.Items, 1)
	assert.Equal(t, "newer-request", response.Items[0].RequestID)
	assert.Equal(t, "gpt-5.6-luna", response.Items[0].Model)
	assert.Equal(t, int64(4), response.Items[0].ChunkCount)
	assert.Equal(t, 1, response.Files.Count)
	require.Equal(t, []string{"gpt-5.6-luna", "gpt-5.6-sol"}, response.Facets.Models)
	require.Len(t, response.Facets.Tokens, 2)
	assert.Equal(t, FullContentLogTokenOption{ID: 5, Name: "primary-test"}, response.Facets.Tokens[0])

	response, err = queryFullContentLogs(dir, fullContentLogQuery{
		Page: 1, PageSize: 20, Model: "LUNA", Token: "primary", RequestID: "newer",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, response.Total)
	require.Len(t, response.Items, 1)
	assert.Equal(t, 5, response.Items[0].TokenID)

	response, err = queryFullContentLogs(dir, fullContentLogQuery{
		Page: 1, PageSize: 20, StartTimestamp: 1787826600,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, response.Total)
	assert.Equal(t, "newer-request", response.Items[0].RequestID)
}

func TestQueryFullContentLogsRefreshesSummaryCacheWhenFileChanges(t *testing.T) {
	dir := t.TempDir()
	path := writeFullContentLogTestFile(t, dir, []fullContentLogRecord{
		{Timestamp: "2026-08-27T10:00:00Z", RequestID: "cached-request", Phase: "request", Method: "POST", Path: "/v1/responses", Encoding: "json", Body: `{"model":"gpt-5.6-sol"}`},
	})

	first, err := queryFullContentLogs(dir, fullContentLogQuery{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, 1, first.Total)

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	require.NoError(t, err)
	encoded, err := common.Marshal(fullContentLogRecord{
		Timestamp: "2026-08-27T10:00:01Z", RequestID: "new-request", Phase: "request", Method: "POST", Path: "/v1/chat/completions", Encoding: "json", Body: `{"model":"gpt-5.6-luna"}`,
	})
	require.NoError(t, err)
	_, err = file.Write(append([]byte("\n"), append(encoded, '\n')...))
	require.NoError(t, err)
	require.NoError(t, file.Close())

	second, err := queryFullContentLogs(dir, fullContentLogQuery{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, 2, second.Total)
}

func TestLoadFullContentLogDetailReconstructsStreamingResponse(t *testing.T) {
	dir := t.TempDir()
	writeFullContentLogTestFile(t, dir, []fullContentLogRecord{
		{
			Timestamp: "2026-08-27T11:00:00Z", RequestID: "request-123", Phase: "request",
			Method: "POST", Path: "/v1/responses", ContentType: "application/json", Encoding: "json",
			Body: `{"model":"gpt-5.6-luna","input":"hello"}`, BodyBytes: 46,
			Headers: map[string][]string{"User-Agent": {"OpenClaw-AI/1.0"}},
			Query:   map[string][]string{"trace": {"enabled"}},
		},
		{Timestamp: "2026-08-27T11:00:01Z", RequestID: "request-123", Phase: "response_chunk", Sequence: 2, ContentType: "text/event-stream", Encoding: "utf-8", Body: "second"},
		{Timestamp: "2026-08-27T11:00:01Z", RequestID: "request-123", Phase: "response_chunk", Sequence: 1, ContentType: "text/event-stream", Encoding: "utf-8", Body: "first-"},
		{
			Timestamp: "2026-08-27T11:00:02Z", RequestID: "request-123", Phase: "response_end",
			Sequence: 2, Status: 200, BodyBytes: 12, DurationMS: 900,
			Headers: map[string][]string{"Content-Type": {"text/event-stream"}, "X-Upstream-Request-Id": {"upstream-123"}},
		},
	})

	detail, found, err := loadFullContentLogDetail(dir, "request-123")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "gpt-5.6-luna", detail.Model)
	assert.Equal(t, "first-second", detail.ResponseBody)
	assert.Equal(t, "utf-8", detail.ResponseEncoding)
	assert.Equal(t, "text/event-stream", detail.ResponseContentType)
	assert.Equal(t, int64(2), detail.ChunkCount)
	assert.Equal(t, []string{"OpenClaw-AI/1.0"}, detail.RequestHeaders["User-Agent"])
	assert.Equal(t, []string{"enabled"}, detail.Query["trace"])
	assert.Equal(t, []string{"upstream-123"}, detail.ResponseHeaders["X-Upstream-Request-Id"])
}

func TestJoinFullContentLogChunksEncodesBinaryResponse(t *testing.T) {
	binary := []byte{0xff, 0x00, 0x01}
	body, encoding, contentType, err := joinFullContentLogChunks([]fullContentLogRecord{
		{Sequence: 1, Encoding: "base64", Body: base64.StdEncoding.EncodeToString(binary), ContentType: "application/octet-stream"},
	})

	require.NoError(t, err)
	assert.Equal(t, "base64", encoding)
	assert.Equal(t, "application/octet-stream", contentType)
	assert.Equal(t, base64.StdEncoding.EncodeToString(binary), body)
}

func TestJoinFullContentLogChunksWithLimitBoundsLargeResponse(t *testing.T) {
	body, encoding, _, truncated, err := joinFullContentLogChunksWithLimit([]fullContentLogRecord{
		{Sequence: 1, Encoding: "utf-8", Body: "你好"},
		{Sequence: 2, Encoding: "utf-8", Body: " world"},
	}, 5)

	require.NoError(t, err)
	assert.Equal(t, "utf-8", encoding)
	assert.True(t, truncated)
	assert.Equal(t, "你", body)
}

func TestTruncateFullContentLogBodyPreservesBinaryEncoding(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03})
	truncated, wasTruncated := truncateFullContentLogBody(encoded, "base64", 2)
	require.True(t, wasTruncated)
	decoded, err := base64.StdEncoding.DecodeString(truncated)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02}, decoded)
}
