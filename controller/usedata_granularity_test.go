package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type quotaDataResponse struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Data    []model.QuotaData `json:"data"`
}

func runQuotaDataRequest(t *testing.T, target string, handler gin.HandlerFunc) quotaDataResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	handler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload quotaDataResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}

func TestGetAllQuotaDatesAcceptsMinuteGranularityAndTimezone(t *testing.T) {
	setupFlowControllerTestDB(t)

	payload := runQuotaDataRequest(
		t,
		"/api/data?start_timestamp=1000&end_timestamp=2000&granularity=minute&timezone_offset=480",
		GetAllQuotaDates,
	)

	require.True(t, payload.Success, payload.Message)
	require.Len(t, payload.Data, 2)
}

func TestGetAllQuotaDatesPrefersGranularityOverLegacyDefaultTime(t *testing.T) {
	setupFlowControllerTestDB(t)

	payload := runQuotaDataRequest(
		t,
		"/api/data?start_timestamp=1000&end_timestamp=2000&granularity=minute&default_time=invalid",
		GetAllQuotaDates,
	)

	require.True(t, payload.Success, payload.Message)
}

func TestGetAllQuotaDatesRejectsInvalidAggregationOptions(t *testing.T) {
	setupFlowControllerTestDB(t)

	invalidGranularity := runQuotaDataRequest(
		t,
		"/api/data?start_timestamp=1000&end_timestamp=2000&granularity=second",
		GetAllQuotaDates,
	)
	require.False(t, invalidGranularity.Success)
	require.Equal(t, "invalid granularity", invalidGranularity.Message)

	invalidTimezone := runQuotaDataRequest(
		t,
		"/api/data?start_timestamp=1000&end_timestamp=2000&granularity=hour&timezone_offset=900",
		GetAllQuotaDates,
	)
	require.False(t, invalidTimezone.Success)
	require.Equal(t, "invalid timezone_offset", invalidTimezone.Message)

	tooManyBuckets := runQuotaDataRequest(
		t,
		"/api/data?start_timestamp=1000&end_timestamp=100000&granularity=minute",
		GetAllQuotaDates,
	)
	require.False(t, tooManyBuckets.Success)
	require.Equal(t, "time range contains too many buckets", tooManyBuckets.Message)
}

func TestParseQuotaDataRequestCountsAlignedBoundaryBuckets(t *testing.T) {
	setupFlowControllerTestDB(t)

	// 1,500 minute buckets are accepted even though the raw timestamps are not
	// aligned; the following bucket is rejected.
	accepted := runQuotaDataRequest(
		t,
		"/api/data?start_timestamp=61&end_timestamp=90000&granularity=minute",
		GetAllQuotaDates,
	)
	require.True(t, accepted.Success, accepted.Message)

	rejected := runQuotaDataRequest(
		t,
		"/api/data?start_timestamp=61&end_timestamp=90060&granularity=minute",
		GetAllQuotaDates,
	)
	require.False(t, rejected.Success)
	require.Equal(t, "time range contains too many buckets", rejected.Message)
}

func TestGetAllQuotaDatesDoesNotExpandQueryToFirstHourBucket(t *testing.T) {
	setupFlowControllerTestDB(t)

	payload := runQuotaDataRequest(
		t,
		"/api/data?start_timestamp=1150&end_timestamp=2000&granularity=hour",
		GetAllQuotaDates,
	)

	require.True(t, payload.Success, payload.Message)
	require.Len(t, payload.Data, 1)
	require.Equal(t, "gpt-b", payload.Data[0].ModelName)
	require.Equal(t, 70, payload.Data[0].Quota)
}

func TestGetUserQuotaDatesChecksRequestedRangeBeforeSourceNormalization(t *testing.T) {
	setupFlowControllerTestDB(t)

	payload := runQuotaDataRequest(
		t,
		"/api/data/self?start_timestamp=61&end_timestamp=2592061&granularity=week&timezone_offset=480",
		GetUserQuotaDates,
	)

	require.True(t, payload.Success, payload.Message)
	require.Len(t, payload.Data, 1)
}

func TestGetUserQuotaDatesKeepsAuthenticatedUserBoundaryWithMinuteBuckets(t *testing.T) {
	setupFlowControllerTestDB(t)

	payload := runQuotaDataRequest(
		t,
		"/api/data/self?start_timestamp=1000&end_timestamp=2000&granularity=minute",
		GetUserQuotaDates,
	)

	require.True(t, payload.Success, payload.Message)
	require.Len(t, payload.Data, 1)
	require.Equal(t, 1, payload.Data[0].UserID)
	require.Equal(t, "alice", payload.Data[0].Username)
}
