package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogIdentityKeepsRenamedKeysSeparateFromNamesAndUnknownHistory(t *testing.T) {
	db := useLogDedupTestDB(t)
	for _, row := range []Log{
		{UserId: 1, Username: "owner", TokenId: 7, TokenName: "renamed", Type: LogTypeConsume, Quota: 3, PromptTokens: 10, CompletionTokens: 2, CreatedAt: 100, RequestId: "selected", UpstreamRequestId: "up-selected", Other: `{"usage_accuracy":"reported"}`},
		{UserId: 1, Username: "owner", TokenId: 7, TokenName: "original", Type: LogTypeError, CreatedAt: 100, RequestId: "unknown-usage", Other: `{"usage_outcome":"unknown"}`},
		{UserId: 1, Username: "owner", TokenId: 8, TokenName: "renamed", Type: LogTypeConsume, Quota: 100, CreatedAt: 100, RequestId: "same-name"},
		{UserId: 1, Username: "owner", TokenId: 0, TokenName: "renamed", Type: LogTypeConsume, Quota: 200, CreatedAt: 100, RequestId: "historical-no-identity"},
		{UserId: 2, Username: "other", TokenId: 9, TokenName: "renamed", Type: LogTypeConsume, Quota: 500, CreatedAt: 100, RequestId: "foreign-owner"},
	} {
		require.NoError(t, CreateLog(db, &row))
	}
	identity := LogIdentityFilter{TokenID: 7}
	rows, total, err := GetAllLogs(0, 0, 0, "", "", "", 0, 1, 0, "", "", "", "", identity)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, rows, 1)
	assert.Equal(t, 7, rows[0].TokenId)
	rows, total, err = GetAllLogs(0, 0, 0, "", "", "", 1, 1, 0, "", "", "", "", identity)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, rows, 1)
	assert.Equal(t, 7, rows[0].TokenId)
	_, total, err = GetUserLogs(1, 0, 0, 0, "", "", 0, 20, "", "", "", "", LogIdentityFilter{TokenID: 9})
	require.NoError(t, err)
	assert.Zero(t, total)
	stat, err := GetLogTokenStatistics(t.Context(), db, LogTokenStatisticsFilter{UserID: 1, TokenID: 7})
	require.NoError(t, err)
	assert.EqualValues(t, 2, stat.Records)
	assert.EqualValues(t, 1, stat.UnknownRecords)
	assert.Nil(t, stat.TotalTokens)
	assert.Equal(t, "12", stat.KnownTotalTokens)
	identity.RequestID, identity.UpstreamRequestID = "selected", "up-selected"
	quota, err := SumUsedQuotaContext(t.Context(), 0, 0, 0, "", "", "", 0, "", identity)
	require.NoError(t, err)
	assert.Equal(t, 3, quota.Quota)
	identity.UserID = 2
	quota, err = SumUsedQuotaContext(t.Context(), 0, 0, 0, "", "", "", 0, "", identity)
	require.NoError(t, err)
	assert.Zero(t, quota.Quota)
	assert.Zero(t, quota.Rpm)
}
