package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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

func TestLoadFullContentLogDetailReportsReadLimitTruncation(t *testing.T) {
	dir := t.TempDir()
	firstChunk := "你好abcdef"
	lastChunk := "tail"
	total := int64(len(firstChunk) + len(lastChunk))
	writeFullContentLogTestFile(t, dir, []fullContentLogRecord{
		{Timestamp: "2026-11-06T00:00:00Z", RequestID: "collector-truncated", Phase: "request", Method: "POST", Path: "/v1/responses", Encoding: "json", Body: `{}`, BodyBytes: 2},
		{Timestamp: "2026-11-06T00:00:01Z", RequestID: "collector-truncated", Phase: "response_chunk", Sequence: 1, Encoding: "utf-8", ContentType: "text/plain; charset=utf-8", Body: firstChunk, BodyBytes: int64(len(firstChunk))},
		{Timestamp: "2026-11-06T00:00:01Z", RequestID: "collector-truncated", Phase: "response_chunk", Sequence: 2, Encoding: "utf-8", ContentType: "text/plain; charset=utf-8", Body: lastChunk, BodyBytes: int64(len(lastChunk))},
		{Timestamp: "2026-11-06T00:00:02Z", RequestID: "collector-truncated", Phase: "response_end", Sequence: 2, Status: 200, BodyBytes: total},
	})

	detail, found, err := loadFullContentLogDetailWithLimit(dir, "collector-truncated", 8)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "你好ab", detail.ResponseBody)
	require.Equal(t, total, detail.ResponseBodyTotalBytes)
	require.True(t, detail.ResponseBodyTruncated, "collector clipped one saved chunk and discarded the following bytes")
}

func writeSavedSegmentRecords(t *testing.T, records ...map[string]any) (string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "full-content-20261106T000000.000000000Z.jsonl")
	var data []byte
	for _, record := range records {
		encoded, err := common.Marshal(record)
		require.NoError(t, err)
		data = append(data, encoded...)
		data = append(data, '\n')
	}
	require.NoError(t, os.WriteFile(path, data, 0600))
	return dir, path
}

func TestFullContentLogSegmentsPreserveCaptureMetadataPresence(t *testing.T) {
	for _, tc := range []struct {
		name         string
		part         string
		body         string
		requestBytes any
		end          map[string]any
		sequence     any
		expected     *string
		complete     *bool
	}{
		{name: "request absent expected", part: "request", body: "saved", requestBytes: nil},
		{name: "request explicit zero", part: "request", body: "", requestBytes: int64(0), expected: pointerSavedSegmentString("0"), complete: pointerSavedSegmentBool(true)},
		{name: "request mismatch is capture incomplete", part: "request", body: "saved", requestBytes: int64(9), expected: pointerSavedSegmentString("9"), complete: pointerSavedSegmentBool(false)},
		{name: "response missing terminal", part: "response", body: "saved", sequence: int64(1)},
		{name: "response absent expected", part: "response", body: "saved", sequence: int64(1), end: map[string]any{"sequence": 1}},
		{name: "response null expected", part: "response", body: "saved", sequence: int64(1), end: map[string]any{"sequence": 1, "body_bytes": nil}},
		{name: "response absent sequence", part: "response", body: "saved", end: map[string]any{"sequence": 1, "body_bytes": 5}, expected: pointerSavedSegmentString("5")},
		{name: "response sequence gap", part: "response", body: "saved", sequence: int64(2), end: map[string]any{"sequence": 2, "body_bytes": 5}, expected: pointerSavedSegmentString("5"), complete: pointerSavedSegmentBool(false)},
		{name: "response explicit zero", part: "response", body: "", sequence: int64(1), end: map[string]any{"sequence": 1, "body_bytes": 0}, expected: pointerSavedSegmentString("0"), complete: pointerSavedSegmentBool(true)},
		{name: "HTTP error is not capture failure", part: "response", body: "saved", sequence: int64(1), end: map[string]any{"sequence": 1, "body_bytes": 5, "status": 502}, expected: pointerSavedSegmentString("5"), complete: pointerSavedSegmentBool(true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := map[string]any{"request_id": "metadata", "phase": "request", "body": tc.body, "encoding": "utf-8"}
			if tc.requestBytes != nil {
				request["body_bytes"] = tc.requestBytes
			}
			chunk := map[string]any{"request_id": "metadata", "phase": "response_chunk", "body": tc.body, "encoding": "utf-8"}
			if tc.sequence != nil {
				chunk["sequence"] = tc.sequence
			}
			records := []map[string]any{request, chunk}
			if tc.end != nil {
				tc.end["request_id"], tc.end["phase"] = "metadata", "response_end"
				records = append(records, tc.end)
			}
			dir, _ := writeSavedSegmentRecords(t, records...)
			page, err := readFullContentLogSegment(context.Background(), dir, "metadata", tc.part, 0, 64, "", "actor")
			require.NoError(t, err)
			assert.Equal(t, tc.body, page.Body)
			assert.Equal(t, strconv.Itoa(len(tc.body)), page.TotalBytes)
			assert.Equal(t, tc.expected, page.ExpectedBytes)
			assert.Equal(t, tc.complete, page.SourceComplete)
		})
	}
}

func pointerSavedSegmentString(value string) *string { return &value }
func pointerSavedSegmentBool(value bool) *bool       { return &value }

func TestFullContentLogSegmentsPreserveMixedBinaryPagesAndSequenceTies(t *testing.T) {
	binary := []byte{0xff, 0, 0x80, '\n', 1}
	dir, _ := writeSavedSegmentRecords(t,
		map[string]any{"request_id": "binary", "phase": "request", "body": "{}"},
		map[string]any{"request_id": "binary", "phase": "response_chunk", "sequence": 2, "body": base64.StdEncoding.EncodeToString(binary)[:4] + "\r\n" + base64.StdEncoding.EncodeToString(binary)[4:], "encoding": "base64", "body_bytes": len(binary)},
		map[string]any{"request_id": "binary", "phase": "response_chunk", "sequence": 1, "body": "abc", "encoding": "utf-8", "body_bytes": 3, "content_type": "application/octet-stream"},
		map[string]any{"request_id": "binary", "phase": "response_chunk", "sequence": 2, "body": "z", "encoding": "utf-8", "body_bytes": 1},
		map[string]any{"request_id": "binary", "phase": "response_end", "sequence": 3, "body_bytes": 9},
	)
	first, err := readFullContentLogSegment(context.Background(), dir, "binary", "response", 0, 4, "", "actor")
	require.NoError(t, err)
	assert.Equal(t, "base64", first.Encoding)
	assert.Equal(t, "application/octet-stream", first.ContentType)
	require.Equal(t, pointerSavedSegmentBool(false), first.SourceComplete, "duplicate sequence is not a complete capture")
	firstBytes, err := base64.StdEncoding.DecodeString(first.Body)
	require.NoError(t, err)
	assert.Equal(t, []byte{'a', 'b', 'c', 0xff}, firstBytes)
	require.Equal(t, pointerSavedSegmentString("4"), first.NextOffset)
	second, err := readFullContentLogSegment(context.Background(), dir, "binary", "response", 4, 4, first.Snapshot, "actor")
	require.NoError(t, err)
	secondBytes, err := base64.StdEncoding.DecodeString(second.Body)
	require.NoError(t, err)
	assert.Equal(t, []byte{0, 0x80, '\n', 1}, secondBytes)
	require.Equal(t, pointerSavedSegmentString("8"), second.NextOffset)
	last, err := readFullContentLogSegment(context.Background(), dir, "binary", "response", 8, 4, first.Snapshot, "actor")
	require.NoError(t, err)
	lastBytes, err := base64.StdEncoding.DecodeString(last.Body)
	require.NoError(t, err)
	assert.Equal(t, []byte{'z'}, lastBytes)
	assert.False(t, last.HasMore)
	assert.Nil(t, last.NextOffset)
	assert.Equal(t, "9", last.TotalBytes)
}

func TestFullContentLogSegmentsIgnoreUnrelatedAppendAndBindActor(t *testing.T) {
	dir, path := writeSavedSegmentRecords(t,
		map[string]any{"request_id": "stable", "phase": "request", "body": "abcdefgh", "body_bytes": 8},
	)
	first, err := readFullContentLogSegment(context.Background(), dir, "stable", "request", 0, 4, "", `{"id":1,"role":10,"sid":"one"}`)
	require.NoError(t, err)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	require.NoError(t, err)
	_, err = file.WriteString("{\"request_id\":\"another\",\"phase\":\"request\",\"body\":\"unrelated\"}\n{\"request_id\":\"pending-other\",\"body\":\"not finished")
	require.NoError(t, err)
	require.NoError(t, file.Close())
	next, err := readFullContentLogSegment(context.Background(), dir, "stable", "request", 4, 4, first.Snapshot, `{"id":1,"role":10,"sid":"one"}`)
	require.NoError(t, err)
	assert.Equal(t, "efgh", next.Body)
	assert.Equal(t, first.Snapshot, next.Snapshot)
	for _, actor := range []string{`{"id":2,"role":10,"sid":"one"}`, `{"id":1,"role":100,"sid":"one"}`, `{"id":1,"role":10,"sid":"two"}`} {
		page, err := readFullContentLogSegment(context.Background(), dir, "stable", "request", 4, 4, first.Snapshot, actor)
		var conflict *savedLogSegmentError
		require.ErrorAs(t, err, &conflict)
		assert.Equal(t, http.StatusConflict, conflict.status)
		assert.Equal(t, "FULL_CONTENT_SEGMENT_CHANGED", conflict.code)
		assert.Empty(t, page.Body)
	}
	// Removing the original target cannot turn a continuation into a new read.
	require.NoError(t, os.WriteFile(path, []byte("{\"request_id\":\"other\",\"phase\":\"request\"}\n"), 0600))
	page, err := readFullContentLogSegment(context.Background(), dir, "stable", "request", 4, 4, first.Snapshot, `{"id":1,"role":10,"sid":"one"}`)
	var conflict *savedLogSegmentError
	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, "FULL_CONTENT_SEGMENT_CHANGED", conflict.code)
	assert.Empty(t, page.Body)
}

func TestFullContentLogSegmentsValidateOriginalJSONAndCrossBufferStrings(t *testing.T) {
	// Position the escape at the physical scanner's last buffer byte, then put
	// a raw rune and paired surrogate in the next buffer. The body is before ID.
	prefix := `{"body":"`
	padding := strings.Repeat("x", (32<<10)-len(prefix)-1)
	raw := prefix + padding + `\\\"\n\uD83D\uDE03終","request_id":"escaped","phase":"request","encoding":"utf-8","unknown":{"body":"not the payload","array":[0,false,null,{"a":"x\\y"}]}}` + "\n"
	expected := padding + "\\\"\n😃終"
	dir := t.TempDir()
	path := filepath.Join(dir, "full-content-20261106T000000.000000000Z.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0600))
	page, err := readFullContentLogSegment(context.Background(), dir, "escaped", "request", 0, 1<<20, "", "actor")
	require.NoError(t, err)
	assert.Equal(t, expected, page.Body)
	assert.Equal(t, strconv.Itoa(len(expected)), page.TotalBytes)
	assert.Nil(t, page.ExpectedBytes)
	assert.Nil(t, page.SourceComplete)

	for _, tc := range []struct{ name, record string }{
		{"duplicate body", `{"request_id":"bad","phase":"request","body":"one","body":"two"}`},
		{"escaped duplicate body", `{"request_id":"bad","phase":"request","body":"one","\u0062ody":"two"}`},
		{"nested duplicate", `{"request_id":"bad","phase":"request","body":"one","extra":{"a":1,"a":2}}`},
		{"invalid nested JSON", `{"request_id":"bad","phase":"request","body":"one","extra":[1,]}`},
		{"invalid body escape", `{"request_id":"bad","phase":"request","body":"\x00"}`},
		{"invalid base64", `{"request_id":"bad","phase":"request","encoding":"base64","body":"YWJj=AAA"}`},
		{"unfinished target", `{"request_id":"bad","phase":"request","body":"pending`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(path, []byte(tc.record+"\n"), 0600))
			page, err := readFullContentLogSegment(context.Background(), dir, "bad", "request", 0, 4, "", "actor")
			var conflict *savedLogSegmentError
			require.ErrorAs(t, err, &conflict)
			assert.Equal(t, http.StatusConflict, conflict.status)
			assert.Equal(t, "FULL_CONTENT_SEGMENT_CORRUPT", conflict.code)
			assert.Empty(t, page.Body)
		})
	}
}

func TestFullContentLogSegmentsDuplicateRequestsAndChangedContent(t *testing.T) {
	request := map[string]any{"request_id": "duplicate", "phase": "request", "body": "abcdefgh"}
	dir, path := writeSavedSegmentRecords(t, request)
	first, err := readFullContentLogSegment(context.Background(), dir, "duplicate", "request", 0, 4, "", "actor")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("{\"request_id\":\"duplicate\",\"phase\":\"request\",\"body\":\"ABCDEFGH\"}\n"), 0600))
	page, err := readFullContentLogSegment(context.Background(), dir, "duplicate", "request", 4, 4, first.Snapshot, "actor")
	var changed *savedLogSegmentError
	require.ErrorAs(t, err, &changed)
	assert.Equal(t, "FULL_CONTENT_SEGMENT_CHANGED", changed.code)
	assert.Empty(t, page.Body)
	encoded, err := common.Marshal(request)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(append(append([]byte{}, encoded...), '\n'), append(encoded, '\n')...), 0600))
	page, err = readFullContentLogSegment(context.Background(), dir, "duplicate", "request", 0, 4, "", "actor")
	var ambiguous *savedLogSegmentError
	require.ErrorAs(t, err, &ambiguous)
	assert.Equal(t, "FULL_CONTENT_SEGMENT_AMBIGUOUS", ambiguous.code)
	assert.Empty(t, page.Body)
}

func TestLoadFullContentLogDetailReadLimitBinaryAndEmptyChunks(t *testing.T) {
	for _, tc := range []struct {
		name      string
		bytes     []byte
		limit     int64
		truncated bool
		expected  []byte
	}{
		{"binary clipped", []byte{0xff, 1, 2, 3, 4}, 4, true, []byte{0xff, 1, 2, 3}},
		{"padded binary exactly retained", []byte{0xff}, 1, false, []byte{0xff}},
		{"binary followed only by empty chunk", []byte{0xff, 1, 2}, 3, false, []byte{0xff, 1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFullContentLogTestFile(t, dir, []fullContentLogRecord{
				{RequestID: "binary-limit", Phase: "request", Body: "{}"},
				{RequestID: "binary-limit", Phase: "response_chunk", Sequence: 1, Encoding: "base64", Body: base64.StdEncoding.EncodeToString(tc.bytes)},
				{RequestID: "binary-limit", Phase: "response_chunk", Sequence: 2, Encoding: "base64", Body: ""},
				{RequestID: "binary-limit", Phase: "response_end", Sequence: 2, BodyBytes: int64(len(tc.bytes))},
			})
			detail, found, err := loadFullContentLogDetailWithLimit(dir, "binary-limit", tc.limit)
			require.NoError(t, err)
			require.True(t, found)
			decoded, err := base64.StdEncoding.DecodeString(detail.ResponseBody)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, decoded)
			assert.Equal(t, tc.truncated, detail.ResponseBodyTruncated)
			assert.Equal(t, int64(len(tc.bytes)), detail.ResponseBodyTotalBytes)
		})
	}
}

func TestFullContentLogSegmentsCancellationDoesNotReturnSavedBody(t *testing.T) {
	dir, _ := writeSavedSegmentRecords(t, map[string]any{"request_id": "cancelled", "phase": "request", "body": "private saved content"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	page, err := readFullContentLogSegment(ctx, dir, "cancelled", "request", 0, 4, "", "actor")
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, page.Body)
}

func TestFullContentLogSegmentsRejectSameFileRewriteBetweenScans(t *testing.T) {
	dir, path := writeSavedSegmentRecords(t, map[string]any{"request_id": "between-scans", "phase": "request", "body": "abcdefgh", "body_bytes": 8})
	first, err := scanSavedLogSnapshot(context.Background(), dir, "between-scans", "request", "actor")
	require.NoError(t, err)
	before, err := os.Stat(path)
	require.NoError(t, err)
	original, err := os.ReadFile(path)
	require.NoError(t, err)
	bodyPosition := bytes.Index(original, []byte("abcdefgh"))
	require.NotEqual(t, -1, bodyPosition)
	file, err := os.OpenFile(path, os.O_WRONLY, 0600)
	require.NoError(t, err)
	written, writeErr := file.WriteAt([]byte("ABCDEFGH"), int64(bodyPosition))
	closeErr := file.Close()
	require.NoError(t, writeErr)
	require.NoError(t, closeErr)
	require.Equal(t, 8, written)
	after, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after))
	require.Equal(t, before.Size(), after.Size())

	page, err := readSavedSnapshotSegment(context.Background(), dir, "between-scans", "request", 4, 4, "actor", first)
	var changed *savedLogSegmentError
	require.ErrorAs(t, err, &changed)
	assert.Equal(t, http.StatusConflict, changed.status)
	assert.Equal(t, "FULL_CONTENT_SEGMENT_CHANGED", changed.code)
	assert.Empty(t, page.Body, "bytes copied after the first scan must not escape on version mismatch")
	assert.Empty(t, page.Snapshot)
	assert.Nil(t, page.NextOffset)
}
