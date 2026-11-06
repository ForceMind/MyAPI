package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This is the public wire contract, not a reference to a future controller type.
// The feature test must compile against the old router and fail on its 404.
type savedContentSegmentHTTPEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
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
	} `json:"data"`
}

func requestSavedContentHTTP(engine *gin.Engine, path, accessToken string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "http://myapi.local"+path, nil)
	if accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

func writeSavedContentHTTPRecord(t *testing.T, file *os.File, record map[string]any) {
	t.Helper()
	encoded, err := common.Marshal(record)
	require.NoError(t, err)
	_, err = file.Write(append(encoded, '\n'))
	require.NoError(t, err)
}

func readSavedContentSegmentHTTP(t *testing.T, engine *gin.Engine, path, accessToken string) savedContentSegmentHTTPEnvelope {
	t.Helper()
	response := requestSavedContentHTTP(engine, path, accessToken)
	require.Equal(t, http.StatusOK, response.Code, "authorized saved segment read: %s", path)
	assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
	var envelope savedContentSegmentHTTPEnvelope
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
	require.True(t, envelope.Success)
	return envelope
}

func TestFullContentLogSegmentsReadSavedTailBeyondPreview(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })
	// Reuse the existing real SetApiRouter/JWT/database fixture. No test-only
	// route or authorization context is installed for the missing endpoint.
	fixture := newManagementHTTPFixture(t)
	dir := t.TempDir()
	t.Setenv("FULL_CONTENT_LOG_DIR", dir)
	file, err := os.OpenFile(filepath.Join(dir, "full-content-20261106T000000.000000000Z.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })
	const requestID = "segments-large-saved-response"
	const contentType = "text/plain; charset=utf-8"
	requestBody := `{"model":"saved-segments-model","input":"saved content only"}`
	writeSavedContentHTTPRecord(t, file, map[string]any{
		"timestamp": "2026-11-06T00:00:00Z", "request_id": requestID, "phase": "request",
		"method": http.MethodPost, "path": "/v1/responses", "encoding": "json",
		"content_type": "application/json", "body": requestBody, "body_bytes": len(requestBody),
	})
	// Seventeen saved 1MiB chunks cross the old HTTP preview's 16MiB ceiling.
	// Reuse one chunk value instead of constructing a whole response in the fixture.
	chunk := strings.Repeat("a", 1<<20)
	for sequence := 1; sequence <= 17; sequence++ {
		writeSavedContentHTTPRecord(t, file, map[string]any{
			"timestamp": "2026-11-06T00:00:01Z", "request_id": requestID, "phase": "response_chunk",
			"sequence": sequence, "encoding": "utf-8", "content_type": contentType,
			"body": chunk, "body_bytes": len(chunk),
		})
	}
	tail := "tail:終\n!"
	tailStart := int64(17 * len(chunk))
	total := tailStart + int64(len(tail))
	require.Greater(t, total, int64(16<<20))
	writeSavedContentHTTPRecord(t, file, map[string]any{
		"timestamp": "2026-11-06T00:00:01Z", "request_id": requestID, "phase": "response_chunk",
		"sequence": 18, "encoding": "utf-8", "content_type": contentType,
		"body": tail, "body_bytes": len(tail),
	})
	writeSavedContentHTTPRecord(t, file, map[string]any{
		"timestamp": "2026-11-06T00:00:02Z", "request_id": requestID, "phase": "response_end",
		"sequence": 18, "status": http.StatusOK, "body_bytes": total,
	})
	require.NoError(t, file.Close())

	// Establish that this exact Admin JWT is authorized and the saved fixture
	// is reachable through the old production detail route before the red test.
	preview := requestSavedContentHTTP(fixture.engine, "/api/full-content-logs/"+requestID+"?max_response_bytes=4", fixture.adminSession)
	require.Equal(t, http.StatusOK, preview.Code)
	var oldDetail struct {
		Success bool `json:"success"`
		Data    struct {
			Body  string `json:"response_body"`
			Total int64  `json:"response_body_total_bytes"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(preview.Body.Bytes(), &oldDetail))
	require.True(t, oldDetail.Success)
	assert.Equal(t, "aaaa", oldDetail.Data.Body)
	assert.Equal(t, total, oldDetail.Data.Total)

	path := "/api/full-content-logs/" + requestID + "/segments?part=response&offset=0&limit=64"
	first := readSavedContentSegmentHTTP(t, fixture.engine, path, fixture.adminSession)
	assert.Equal(t, requestID, first.Data.RequestID)
	assert.Equal(t, "response", first.Data.Part)
	assert.Equal(t, "0", first.Data.RequestedOffset)
	assert.Equal(t, "0", first.Data.Offset)
	assert.Equal(t, strings.Repeat("a", 64), first.Data.Body)
	assert.Equal(t, strconv.FormatInt(total, 10), first.Data.TotalBytes)
	require.NotNil(t, first.Data.ExpectedBytes)
	assert.Equal(t, strconv.FormatInt(total, 10), *first.Data.ExpectedBytes)
	require.NotNil(t, first.Data.NextOffset)
	assert.Equal(t, "64", *first.Data.NextOffset)
	assert.True(t, first.Data.HasMore)
	require.Regexp(t, "^[0-9a-f]{64}$", first.Data.Snapshot)
	assert.Equal(t, "utf-8", first.Data.Encoding)
	assert.Equal(t, contentType, first.Data.ContentType)
	require.NotNil(t, first.Data.SourceComplete)
	assert.True(t, *first.Data.SourceComplete, "this proves saved capture completeness, not business/usage settlement")

	path = "/api/full-content-logs/" + requestID + "/segments?part=response&offset=" + strconv.FormatInt(tailStart, 10) + "&limit=64&snapshot=" + first.Data.Snapshot
	last := readSavedContentSegmentHTTP(t, fixture.engine, path, fixture.adminSession)
	assert.Equal(t, tail, last.Data.Body)
	assert.Equal(t, strconv.FormatInt(tailStart, 10), last.Data.RequestedOffset)
	assert.Equal(t, strconv.FormatInt(tailStart, 10), last.Data.Offset)
	assert.Equal(t, first.Data.Snapshot, last.Data.Snapshot)
	assert.False(t, last.Data.HasMore)
	assert.Nil(t, last.Data.NextOffset)

	// An arbitrary byte jump inside the 3-byte rune backs up one byte, and
	// a 4-byte page ends safely after the rune plus newline without losing '!'.
	runeStart := tailStart + int64(len("tail:"))
	requested := runeStart + 1
	path = "/api/full-content-logs/" + requestID + "/segments?part=response&offset=" + strconv.FormatInt(requested, 10) + "&limit=4&snapshot=" + first.Data.Snapshot
	aligned := readSavedContentSegmentHTTP(t, fixture.engine, path, fixture.adminSession)
	assert.Equal(t, strconv.FormatInt(requested, 10), aligned.Data.RequestedOffset)
	assert.Equal(t, strconv.FormatInt(runeStart, 10), aligned.Data.Offset)
	assert.Equal(t, "終\n", aligned.Data.Body)
	assert.Len(t, []byte(aligned.Data.Body), 4)
	assert.True(t, aligned.Data.HasMore)
	require.NotNil(t, aligned.Data.NextOffset)
	assert.Equal(t, strconv.FormatInt(total-1, 10), *aligned.Data.NextOffset)

	path = "/api/full-content-logs/" + requestID + "/segments?part=response&offset=" + *aligned.Data.NextOffset + "&limit=4&snapshot=" + first.Data.Snapshot
	finalByte := readSavedContentSegmentHTTP(t, fixture.engine, path, fixture.adminSession)
	assert.Equal(t, "!", finalByte.Data.Body)
	assert.Equal(t, strconv.FormatInt(total-1, 10), finalByte.Data.Offset)
	assert.False(t, finalByte.Data.HasMore)
	assert.Nil(t, finalByte.Data.NextOffset)
	assert.Equal(t, strconv.FormatInt(total, 10), finalByte.Data.TotalBytes)
}

func TestFullContentLogSegmentsRequireExistingAdminAuthority(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })
	fixture := newManagementHTTPFixture(t)
	member := &model.User{Username: "saved-body-member", AffCode: "saved-body-member", Group: "default", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, model.DB.Create(member).Error)
	_, memberSession := createCodexLocalRouteSession(t, member)
	dir := t.TempDir()
	t.Setenv("FULL_CONTENT_LOG_DIR", dir)
	file, err := os.OpenFile(filepath.Join(dir, "full-content-20261106T000000.000000000Z.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })
	const marker = "saved-body-admin-only"
	for _, record := range []map[string]any{
		{"timestamp": "2026-11-06T00:00:00Z", "request_id": "segments-private", "phase": "request", "method": http.MethodPost, "path": "/v1/responses", "encoding": "utf-8", "body": marker, "body_bytes": len(marker)},
		{"timestamp": "2026-11-06T00:00:01Z", "request_id": "segments-private", "phase": "response_chunk", "sequence": 1, "encoding": "utf-8", "body": marker, "body_bytes": len(marker)},
		{"timestamp": "2026-11-06T00:00:02Z", "request_id": "segments-private", "phase": "response_end", "sequence": 1, "body_bytes": len(marker)},
	} {
		writeSavedContentHTTPRecord(t, file, record)
	}
	require.NoError(t, file.Close())
	for _, actor := range []struct {
		name       string
		credential string
		status     int
	}{{"anonymous", "", http.StatusUnauthorized}, {"ordinary member", memberSession, http.StatusForbidden}} {
		t.Run(actor.name, func(t *testing.T) {
			for _, path := range []string{"/api/full-content-logs/segments-private", "/api/full-content-logs/segments-private/segments?part=response&offset=0&limit=64"} {
				response := requestSavedContentHTTP(fixture.engine, path, actor.credential)
				assert.NotContains(t, response.Body.String(), marker)
				assert.NotContains(t, response.Body.String(), `"body"`)
				assert.Equal(t, actor.status, response.Code, path)
			}
		})
	}
}

func TestFullContentLogSegmentsValidateHTTPParametersAndSnapshotAuthority(t *testing.T) {
	previousMode := gin.Mode()
	fixture := newManagementHTTPFixture(t)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	dir := t.TempDir()
	t.Setenv("FULL_CONTENT_LOG_DIR", dir)
	file, err := os.Create(filepath.Join(dir, "full-content-20261106T010000.000000000Z.jsonl"))
	require.NoError(t, err)
	body := strings.Repeat("p", 1<<20) + "tail"
	writeSavedContentHTTPRecord(t, file, map[string]any{"request_id": "http-range", "phase": "request", "encoding": "utf-8", "body": body, "body_bytes": len(body)})
	require.NoError(t, file.Close())
	base := "/api/full-content-logs/http-range/segments"
	first := readSavedContentSegmentHTTP(t, fixture.engine, base+"?part=request", fixture.adminSession)
	assert.Equal(t, strings.Repeat("p", 1<<20), first.Data.Body)
	assert.Equal(t, "1048580", first.Data.TotalBytes)
	require.Equal(t, pointerSavedContentHTTPString("1048576"), first.Data.NextOffset)
	assert.True(t, first.Data.HasMore)
	tail := readSavedContentSegmentHTTP(t, fixture.engine, base+"?part=request&offset=1048576&limit=4&snapshot="+first.Data.Snapshot, fixture.adminSession)
	assert.Equal(t, "tail", tail.Data.Body)
	assert.False(t, tail.Data.HasMore)
	empty := readSavedContentSegmentHTTP(t, fixture.engine, base+"?part=request&offset=1048580&limit=4&snapshot="+first.Data.Snapshot, fixture.adminSession)
	assert.Empty(t, empty.Data.Body)
	assert.Equal(t, "1048580", empty.Data.Offset)
	assert.Nil(t, empty.Data.NextOffset)

	for _, query := range []string{
		"", "?part=unknown", "?part=request&offset=-1", "?part=request&offset=01", "?part=request&offset=%2B1",
		"?part=request&offset=9223372036854775808", "?part=request&offset=1", "?part=request&limit=0", "?part=request&limit=3",
		"?part=request&limit=1048577", "?part=request&limit=4.0", "?part=request&snapshot=abc", "?part=request&snapshot=" + strings.Repeat("A", 64),
	} {
		t.Run(query, func(t *testing.T) {
			response := requestSavedContentHTTP(fixture.engine, base+query, fixture.adminSession)
			require.Equal(t, http.StatusBadRequest, response.Code)
			var failure struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &failure))
			assert.False(t, failure.Success)
			assert.Equal(t, "FULL_CONTENT_SEGMENT_INVALID", failure.Code)
			assert.NotContains(t, response.Body.String(), `"body"`)
			assert.NotContains(t, response.Body.String(), dir)
		})
	}
	// A second real session of the same actor must not inherit a cursor.
	secondSessionActor := *fixture.admin
	secondSessionActor.Username += "-segments-second-session"
	_, secondSession := createCodexLocalRouteSession(t, &secondSessionActor)
	for _, tc := range []struct {
		name, path, credential, code string
		status                       int
	}{
		{"beyond total", base + "?part=request&offset=1048581&limit=4&snapshot=" + first.Data.Snapshot, fixture.adminSession, "FULL_CONTENT_SEGMENT_RANGE", http.StatusRequestedRangeNotSatisfiable},
		{"another actor", base + "?part=request&offset=1048576&limit=4&snapshot=" + first.Data.Snapshot, fixture.session, "FULL_CONTENT_SEGMENT_CHANGED", http.StatusConflict},
		{"another session", base + "?part=request&offset=1048576&limit=4&snapshot=" + first.Data.Snapshot, secondSession, "FULL_CONTENT_SEGMENT_CHANGED", http.StatusConflict},
		{"missing target", "/api/full-content-logs/absent/segments?part=request", fixture.adminSession, "FULL_CONTENT_SEGMENT_NOT_FOUND", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := requestSavedContentHTTP(fixture.engine, tc.path, tc.credential)
			require.Equal(t, tc.status, response.Code)
			var failure struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &failure))
			assert.False(t, failure.Success)
			assert.Equal(t, tc.code, failure.Code)
			assert.NotContains(t, response.Body.String(), `"body"`)
			assert.NotContains(t, response.Body.String(), dir)
		})
	}
}

func pointerSavedContentHTTPString(value string) *string { return &value }
