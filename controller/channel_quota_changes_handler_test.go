package controller

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func quotaChangesErrorMessage(t *testing.T, query string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/channel/quota/changes?"+query, nil)
	GetChannelQuotaChanges(ctx)
	require.Equal(t, 200, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success)
	return response.Message
}

func TestGetChannelQuotaChangesRejectsInvalidRange(t *testing.T) {
	require.Contains(t, quotaChangesErrorMessage(t, "range=year"), "invalid range")
}

func TestGetChannelQuotaChangesRejectsInvalidTimestamps(t *testing.T) {
	require.Contains(t, quotaChangesErrorMessage(t, "range=custom&start=not-a-time"), "invalid start timestamp")
	require.Contains(t, quotaChangesErrorMessage(t, "range=custom&end=not-a-time"), "invalid end timestamp")
}

func TestGetChannelQuotaChangesRejectsUnsafeTimeRange(t *testing.T) {
	require.Contains(t, quotaChangesErrorMessage(t, "range=custom&start=1&end=15552002"), "invalid quota changes time range")
	require.Contains(t, quotaChangesErrorMessage(t, "range=custom&start=10&end=9"), "invalid quota changes time range")
}

func TestGetChannelQuotaChangesRejectsInvalidLimitAndChannelIDs(t *testing.T) {
	require.Contains(t, quotaChangesErrorMessage(t, "limit=0"), "invalid quota changes limit")
	require.Contains(t, quotaChangesErrorMessage(t, "limit=not-a-number"), "invalid quota changes limit")
	message := quotaChangesErrorMessage(t, "channel_ids=1,nope")
	require.True(t, strings.Contains(message, "invalid channel_ids"))
}

func TestQuotaChangeQueryHelpersNormalizeAliasesAndDeduplicateIDs(t *testing.T) {
	seconds, ok := quotaChangeRangeSeconds("1h")
	require.True(t, ok)
	require.EqualValues(t, 60*60, seconds)
	seconds, ok = quotaChangeRangeSeconds("6h")
	require.True(t, ok)
	require.EqualValues(t, 6*60*60, seconds)
	seconds, ok = quotaChangeRangeSeconds("1d")
	require.True(t, ok)
	require.EqualValues(t, 24*60*60, seconds)
	ids, err := parseQuotaChangeChannelIDs("7, 7, 9")
	require.NoError(t, err)
	require.Equal(t, []int{7, 9}, ids)
	_, err = parseQuotaChangeChannelIDs("0")
	require.Error(t, err)
}
