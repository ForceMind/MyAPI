package controller

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func querySelectedCodexHistory(t *testing.T, channelID int, start, end int64, seriesID string, windowType ...string) map[string]any {
	t.Helper()
	query := url.Values{"start": {strconv.FormatInt(start, 10)}, "end": {strconv.FormatInt(end, 10)}}
	if seriesID != "" {
		query.Set("series_id", seriesID)
	}
	if len(windowType) > 0 {
		query.Set("window_type", windowType[0])
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channelID)}}
	c.Request = httptest.NewRequest("GET", "/api/channel/"+strconv.Itoa(channelID)+"/codex/usage/history?"+query.Encode(), nil)
	GetCodexChannelUsageHistory(c)
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	var result map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
	return result
}

func TestCodexHistorySelectsOneAccountAndKeepsItsLatestFailure(t *testing.T) {
	for _, mode := range []struct {
		name      string
		versioned bool
	}{{"legacy", false}, {"versioned", true}} {
		t.Run(mode.name, func(t *testing.T) {
			t.Setenv("CHANNEL_QUOTA_IDENTITY_KEYS", "")
			db := setupCodexUsageHistoryTestDB(t, 993)
			if mode.versioned {
				setupChannelQuotaIdentityFixture(t, db)
			}
			recordAccount := recordCodexUsageSnapshotsAtWithSampleIDForAccount
			if mode.versioned {
				recordAccount = func(channelID int, accountID string, at int64, sampleID string, status int, body []byte) error {
					keyring, err := common.LoadChannelQuotaIdentityKeyring()
					if err != nil {
						return err
					}
					identity, err := model.ResolveChannelQuotaIdentity(context.Background(), db, keyring, "codex", common.ChannelQuotaIdentityKindProviderAccount, []byte(accountID))
					if err != nil {
						return err
					}
					return recordCodexUsageSnapshotsForIdentity(channelID, identity, at, sampleID, status, body)
				}
			}
			info, err := common.Marshal(model.ChannelInfo{IsMultiKey: true})
			require.NoError(t, err)
			require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 993).Update("channel_info", string(info)).Error)
			at := time.Now().Unix() - 10
			require.NoError(t, recordAccount(993, "fixture-account-a", at, "account-a-success", 200, []byte(`{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":20,"limit_window_seconds":18000},"secondary_window":{"used_percent":40,"limit_window_seconds":604800}}}`)))
			require.NoError(t, recordAccount(993, "fixture-account-a", at+1, "account-a-error", 503, nil))
			require.NoError(t, recordAccount(993, "fixture-account-b", at+2, "account-b-success", 200, []byte(`{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":90,"limit_window_seconds":18000},"secondary_window":{"used_percent":95,"limit_window_seconds":604800}}}`)))
			require.NoError(t, recordCodexUsageSnapshotsAtWithSampleID(993, at+3, "unknown-account-error", 503, nil))
			var anchor model.ChannelQuotaSnapshot
			require.NoError(t, db.Where("sample_id = ?", "account-a-success").First(&anchor).Error)
			seriesID := model.ChannelQuotaSnapshotSeriesID(anchor)
			result := querySelectedCodexHistory(t, 993, at-1, at+4, seriesID)
			require.Equal(t, true, result["success"], result)
			data := result["data"].(map[string]any)
			assert.Equal(t, seriesID, data["series_id"])
			assert.Equal(t, float64(1), data["data_quality"].(map[string]any)["identity_unavailable_count"])
			points := data["points"].([]any)
			require.Len(t, points, 2)
			assert.Equal(t, float64(20), points[0].(map[string]any)["primary_used_percent"])
			assert.Equal(t, float64(40), points[0].(map[string]any)["secondary_used_percent"])
			current := data["current"].(map[string]any)
			assert.Equal(t, "account-a-error", current["sample_id"])
			assert.Equal(t, "error", current["status"])
			assert.NotContains(t, current, "primary_used_percent")
			windowResult := querySelectedCodexHistory(t, 993, at-1, at+4, seriesID, "five_hour")
			require.Equal(t, true, windowResult["success"], windowResult)
			windowData := windowResult["data"].(map[string]any)
			assert.Len(t, windowData["points"].([]any), 1)
			assert.Equal(t, "account-a-error", windowData["current"].(map[string]any)["sample_id"])
			encoded, err := common.Marshal(data)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "account-b-success")
			assert.NotContains(t, string(encoded), "fixture-account-a")
			if anchor.AccountRef != "" {
				assert.NotContains(t, string(encoded), anchor.AccountRef)
			}
			if mode.versioned {
				require.NotEmpty(t, anchor.SubjectRef)
				assert.NotContains(t, string(encoded), anchor.SubjectRef)
			}
		})
	}
}

func TestCodexHistoryAfterAccountChangeDoesNotMixOldAccount(t *testing.T) {
	t.Setenv("CHANNEL_QUOTA_IDENTITY_KEYS", "")
	setupCodexUsageHistoryTestDB(t, 994)
	at := time.Now().Unix() - 10
	require.NoError(t, recordCodexUsageSnapshotsAtWithSampleIDForAccount(994, "old-account", at, "old-account-sample", 200, []byte(`{"rate_limit":{"primary_window":{"used_percent":80,"limit_window_seconds":18000}}}`)))
	require.NoError(t, recordCodexUsageSnapshotsAtWithSampleIDForAccount(994, "new-account", at+1, "new-account-sample", 200, []byte(`{"rate_limit":{"primary_window":{"used_percent":10,"limit_window_seconds":18000}}}`)))
	result := querySelectedCodexHistory(t, 994, at-1, at+2, "")
	require.Equal(t, true, result["success"], result)
	data := result["data"].(map[string]any)
	points := data["points"].([]any)
	require.Len(t, points, 1)
	assert.Equal(t, "new-account-sample", points[0].(map[string]any)["sample_id"])
}

func TestCodexHistoryRejectsMissingInvalidOrUnrelatedSeries(t *testing.T) {
	t.Setenv("CHANNEL_QUOTA_IDENTITY_KEYS", "")
	db := setupCodexUsageHistoryTestDB(t, 995)
	info, err := common.Marshal(model.ChannelInfo{IsMultiKey: true})
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 995).Update("channel_info", string(info)).Error)
	at := time.Now().Unix() - 10
	for _, tc := range []struct{ seriesID, code string }{
		{"", "quota_history_series_required"},
		{"account-id-is-not-a-series", "quota_history_invalid_series"},
		{strings.Repeat("A", 64), "quota_history_invalid_series"},
		{strings.Repeat("a", 64), "quota_history_series_not_found"},
	} {
		result := querySelectedCodexHistory(t, 995, at-1, at+1, tc.seriesID)
		assert.Equal(t, false, result["success"])
		assert.Equal(t, tc.code, result["code"])
		assert.NotContains(t, result, "data")
	}
	require.NoError(t, db.Create(&model.Channel{Id: 996, Type: constant.ChannelTypeCodex, Key: "fixture"}).Error)
	require.NoError(t, recordCodexUsageSnapshotsAtWithSampleIDForAccount(996, "another-account", at, "another-channel", 200, []byte(`{"rate_limit":{"primary_window":{"used_percent":10,"limit_window_seconds":18000}}}`)))
	var other model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", 996).First(&other).Error)
	result := querySelectedCodexHistory(t, 995, at-1, at+1, model.ChannelQuotaSnapshotSeriesID(other))
	assert.Equal(t, false, result["success"])
	assert.Equal(t, "quota_history_series_not_found", result["code"])
}
