package service

import (
	"bytes"
	"context"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type snapshotGuardPollingAdaptor struct {
	result                   *relaycommon.TaskInfo
	adjustQuota, adjustCalls int
}

func (a *snapshotGuardPollingAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *snapshotGuardPollingAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"snapshot_fixture":true}`))}, nil
}
func (a *snapshotGuardPollingAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return a.result, nil
}
func (a *snapshotGuardPollingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	a.adjustCalls++
	return a.adjustQuota
}

func seedSnapshotGuardTask(t *testing.T, bc *model.TaskBillingContext) (*model.Task, *model.Log) {
	t.Helper()
	truncate(t)
	oldModels, oldGroups, oldSpecial := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString(), ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModels))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldSpecial))
	})
	// Valid old per-call snapshots must not consult these unusable live rates.
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"snapshot-guard-model":0}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	seedUser(t, 86, 10000)
	seedChannel(t, 86)
	seedToken(t, 86, 86, "snapshot-guard-token", 10000)
	seedChargedAccounting(t, 86, 86, 86, 500, 1)
	task := makeTask(86, 86, 500, 86, BillingSourceWallet, 0)
	task.TaskID = "snapshot-guard-task"
	task.PrivateData.UpstreamTaskID = "snapshot-guard-upstream"
	task.PrivateData.BillingContext = bc
	require.NoError(t, model.DB.Create(task).Error)
	initialLog := &model.Log{UserId: 86, ChannelId: 86, Type: model.LogTypeConsume, Quota: 500, PromptTokens: 100, CompletionTokens: 20, CreatedAt: time.Now().Unix()}
	require.NoError(t, model.LOG_DB.Create(initialLog).Error)
	return task, initialLog
}

func TestTaskPollingAuditsInvalidSnapshotsBeforeEarlyReturns(t *testing.T) {
	for _, tc := range []struct {
		name                string
		version             int
		complete, perCall   bool
		groupRatio          float64
		tokens, adjustQuota int
		withClamp           bool
		wantSource          string
	}{
		{"unknown per-call", 7, true, true, 1, 100, 300, false, "unsupported_snapshot"},
		{"invalid per-call", 1, true, true, -1, 100, 300, false, "invalid_snapshot"},
		{"unknown without usage", 7, true, false, 1, 0, 0, true, "unsupported_snapshot"},
		{"invalid without usage", 1, false, false, 1, 0, 0, false, "invalid_snapshot"},
		{"invalid per-call without usage", 1, true, true, -1, 0, 0, true, "invalid_snapshot"},
		{"unknown before adaptor override", 7, true, false, 1, 100, 300, false, "unsupported_snapshot"},
		{"valid legacy zero model per-call", 0, false, true, 1, 100, 300, false, ""},
		{"valid versioned free per-call", 1, true, true, 0, 100, 300, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bc := &model.TaskBillingContext{Version: tc.version, Complete: tc.complete, PerCallBilling: tc.perCall,
				ModelPrice: 0.2, ModelRatio: 0, GroupRatio: tc.groupRatio, OriginModelName: "snapshot-guard-model"}
			task, initialLog := seedSnapshotGuardTask(t, bc)
			result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, TaskID: task.GetUpstreamTaskID(), TotalTokens: tc.tokens, Reason: "upstream-sensitive-reason"}
			if tc.withClamp {
				_, result.QuotaClamp = common.QuotaFromFloatChecked(1e100)
			}
			adaptor := &snapshotGuardPollingAdaptor{result: result, adjustQuota: tc.adjustQuota}
			var warnings bytes.Buffer
			common.LogWriterMu.Lock()
			oldWriter := gin.DefaultErrorWriter
			gin.DefaultErrorWriter = &warnings
			common.LogWriterMu.Unlock()
			t.Cleanup(func() {
				common.LogWriterMu.Lock()
				gin.DefaultErrorWriter = oldWriter
				common.LogWriterMu.Unlock()
			})
			statsBefore, err := model.SumUsedQuota(model.LogTypeConsume, 0, 0, "", "", "", 86, "")
			require.NoError(t, err)

			err = updateVideoSingleTask(context.Background(), adaptor, &model.Channel{Id: 86}, task.GetUpstreamTaskID(), map[string]*model.Task{task.GetUpstreamTaskID(): task})

			require.NoError(t, err)
			var stored model.Task
			require.NoError(t, model.DB.First(&stored, task.ID).Error)
			assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), stored.Status)
			assert.Equal(t, bc, stored.PrivateData.BillingContext)
			assert.Equal(t, 500, stored.Quota)
			assert.Equal(t, 10000, getUserQuota(t, 86))
			assert.Equal(t, 10000, getTokenRemainQuota(t, 86))
			assert.Equal(t, 500, getTokenUsedQuota(t, 86))
			used, requests := getUserUsageAccounting(t, 86)
			assert.Equal(t, 500, used)
			assert.Equal(t, 1, requests)
			assert.Equal(t, int64(500), getChannelUsedQuota(t, 86))
			assert.Zero(t, adaptor.adjustCalls, "invalid snapshots and per-call tasks must not invoke repricing")
			statsAfter, err := model.SumUsedQuota(model.LogTypeConsume, 0, 0, "", "", "", 86, "")
			require.NoError(t, err)
			assert.Equal(t, statsBefore, statsAfter)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("id > ?", initialLog.Id).Find(&logs).Error)
			if tc.wantSource == "" {
				assert.Empty(t, logs)
				return
			}
			require.Len(t, logs, 1)
			assert.Equal(t, model.LogTypeSystem, logs[0].Type)
			assert.Zero(t, logs[0].Quota)
			assert.Equal(t, "计费快照不可用，保持预扣额度", logs[0].Content)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
			assert.Equal(t, tc.wantSource, other["billing_rate_source"])
			assert.NotContains(t, other, "model_price")
			assert.NotContains(t, other, "model_ratio")
			assert.NotContains(t, other, "group_ratio")
			assert.NotContains(t, logs[0].Other, result.Reason)
			assert.Equal(t, 1, strings.Count(warnings.String(), "task billing snapshot unavailable:"))
			if tc.withClamp {
				admin, ok := other["admin_info"].(map[string]any)
				require.True(t, ok)
				assert.Contains(t, admin, "quota_saturation")
				assert.Equal(t, 1, strings.Count(warnings.String(), "quota saturation on task log:"))
			}
		})
	}
}

func TestTaskTokenEntryAuditsInvalidSnapshotsWithoutUsageOrPerCallRepricing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tokens  int
		perCall bool
		value   float64
	}{
		{"nan per-call", 100, true, math.NaN()},
		{"infinity no usage", 0, false, math.Inf(1)},
		{"negative infinity per-call no usage", 0, true, math.Inf(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task, initialLog := seedSnapshotGuardTask(t, nil)
			// JSON cannot persist these values; exercise the internal metadata
			// boundary without claiming they came from a successful JSON read.
			task.PrivateData.BillingContext = &model.TaskBillingContext{Version: 1, Complete: true, PerCallBilling: tc.perCall,
				ModelPrice: 0.2, ModelRatio: 0, GroupRatio: tc.value, OriginModelName: "snapshot-guard-model"}
			_, firstClamp := common.QuotaFromFloatChecked(1e100)
			_, laterClamp := common.QuotaFromFloatChecked(1e200)

			RecalculateTaskQuotaByTokens(context.Background(), task, tc.tokens, firstClamp, laterClamp)

			assert.Equal(t, 500, getTaskQuota(t, task.ID))
			assert.Equal(t, 10000, getUserQuota(t, 86))
			assert.Equal(t, 10000, getTokenRemainQuota(t, 86))
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("id > ?", initialLog.Id).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Equal(t, model.LogTypeSystem, logs[0].Type)
			assert.Zero(t, logs[0].Quota)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other), "non-finite snapshot rates must not discard JSON audit metadata")
			assert.Equal(t, "invalid_snapshot", other["billing_rate_source"])
			assert.NotContains(t, other, "group_ratio")
			admin, ok := other["admin_info"].(map[string]any)
			require.True(t, ok)
			saturation, ok := admin["quota_saturation"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, firstClamp.Original, saturation["original"])
		})
	}
}
