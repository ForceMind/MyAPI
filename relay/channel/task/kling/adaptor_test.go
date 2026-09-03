package kling

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open Kling test database: " + err.Error())
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get Kling test database: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	// Cache callbacks may outlive an individual subtest. Keep these process
	// lifecycle settings immutable after tests begin instead of racing their
	// reads with per-case restoration.
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true
	common.DataExportEnabled = false
	if err := db.AutoMigrate(&model.Task{}, &model.User{}, &model.Channel{}, &model.Token{}, &model.Log{}); err != nil {
		panic("failed to migrate Kling test database: " + err.Error())
	}
	exitCode := m.Run()
	_ = sqlDB.Close()
	os.Exit(exitCode)
}

func resetKlingPollingTestDB(t *testing.T) {
	t.Helper()
	for _, table := range []string{"logs", "tasks", "tokens", "channels", "users"} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
}

func TestParseTaskResultFinalUnitDeduction(t *testing.T) {
	for _, tc := range []struct {
		name, deduction string
		wantTokens      int
		wantClamp       common.QuotaClampKind
	}{
		{"fraction rounds up", "1.2", 2, ""},
		{"integer unchanged", "12", 12, ""},
		{"zero", "0", 0, ""},
		{"overflow", "1e100", common.MaxQuota, common.QuotaClampOverflow},
		{"positive range overflow", "1e309", common.MaxQuota, common.QuotaClampOverflow},
		{"negative range overflow", "-1e309", 0, common.QuotaClampUnderflow},
		{"nan", "NaN", 0, common.QuotaClampNaN},
		{"positive infinity", "+Inf", common.MaxQuota, common.QuotaClampOverflow},
		{"negative infinity", "-Inf", 0, common.QuotaClampUnderflow},
		{"invalid", "not-a-number", 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := responsePayload{}
			payload.Data.TaskId = "kling-upstream"
			payload.Data.TaskStatus = "succeed"
			payload.Data.FinalUnitDeduction = tc.deduction
			body, err := common.Marshal(payload)
			require.NoError(t, err)
			result, err := (&TaskAdaptor{}).ParseTaskResult(body)
			require.NoError(t, err)
			assert.Equal(t, model.TaskStatusSuccess, result.Status)
			assert.Equal(t, tc.wantTokens, result.TotalTokens)
			assert.Equal(t, tc.wantTokens, result.CompletionTokens)
			if tc.wantClamp == "" {
				assert.Nil(t, result.QuotaClamp)
			} else {
				require.NotNil(t, result.QuotaClamp)
				assert.Equal(t, tc.wantClamp, result.QuotaClamp.Kind)
			}
			encoded, err := common.Marshal(result)
			require.NoError(t, err, "internal saturation metadata must never enter the provider response")
			assert.NotContains(t, string(encoded), "quota_saturation")
			assert.NotContains(t, string(encoded), "QuotaClamp")
		})
	}
}

// The polling fixture replaces only transport; parsing, completion handling,
// token repricing, funding adjustments and log persistence are production code.
type pollingFixtureAdaptor struct {
	TaskAdaptor
	body string
}

func (a *pollingFixtureAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(a.body))}, nil
}

func TestKlingPollingPreservesDeductionSaturationAudit(t *testing.T) {
	for _, tc := range []struct {
		name, deduction        string
		preConsumed, wantQuota int
		ratio                  float64
		wantOriginal           any
		wantKind               string
	}{
		{"normal ceiling", "1.2", 1, 2, 1, nil, ""},
		{"overflow discounted", "1e100", 1000, 2147483, 0.001, float64(1e100), "overflow"},
		{"first clamp survives second overflow", "1e100", 1000, common.MaxQuota, 2, float64(1e100), "overflow"},
		{"overflow discounted zero delta", "1e100", 2147483, 2147483, 0.001, float64(1e100), "overflow"},
		{"positive range overflow discounted", "1e309", 1000, 2147483, 0.001, "+Inf", "overflow"},
		{"positive range overflow zero delta", "1e309", 2147483, 2147483, 0.001, "+Inf", "overflow"},
		{"negative range overflow retains preconsume", "-1e309", 1000, 1000, 0.001, "-Inf", "underflow"},
		{"nan retains preconsume", "NaN", 1000, 1000, 0.001, "NaN", "nan"},
		{"positive infinity discounted", "+Inf", 1000, 2147483, 0.001, "+Inf", "overflow"},
		{"negative infinity retains preconsume", "-Inf", 1000, 1000, 0.001, "-Inf", "underflow"},
		{"invalid retains preconsume", "not-a-number", 1000, 1000, 0.001, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetKlingPollingTestDB(t)
			db := model.DB
			oldAdaptor := service.GetTaskAdaptorFunc
			oldModelRatios := ratio_setting.ModelRatio2JSONString()
			oldGroupRatios := ratio_setting.GroupRatio2JSONString()
			oldGroupGroupRatios := ratio_setting.GroupGroupRatio2JSONString()
			t.Cleanup(func() {
				service.GetTaskAdaptorFunc = oldAdaptor
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatios))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatios))
				require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldGroupGroupRatios))
			})
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"kling-audit-test":1}`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"kling-audit-group":1}`))
			require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))

			user := model.User{Id: 1, Username: "kling-audit-user", Quota: common.MaxQuota, UsedQuota: tc.preConsumed, RequestCount: 1}
			channel := model.Channel{Id: 1, Type: constant.ChannelTypeKling, Name: "kling-audit-channel", Key: "fixture-key", UsedQuota: int64(tc.preConsumed)}
			token := model.Token{Id: 1, UserId: 1, Name: "kling-audit-token", Key: "fixture-token", RemainQuota: common.MaxQuota, UsedQuota: tc.preConsumed}
			require.NoError(t, db.Create(&user).Error)
			require.NoError(t, db.Create(&channel).Error)
			require.NoError(t, db.Create(&token).Error)
			task := &model.Task{
				TaskID: "kling-public-task", UserId: 1, ChannelId: 1, Quota: tc.preConsumed,
				Status: model.TaskStatusInProgress, Group: "kling-audit-group",
				Properties: model.Properties{OriginModelName: "kling-audit-test"},
				PrivateData: model.TaskPrivateData{
					UpstreamTaskID: "kling-upstream", TokenId: 1, BillingSource: service.BillingSourceWallet,
					BillingContext: &model.TaskBillingContext{OriginModelName: "kling-audit-test", ModelRatio: 1, GroupRatio: 1, OtherRatios: map[string]float64{"discount": tc.ratio}},
				},
			}
			require.NoError(t, db.Create(task).Error)
			preconsumeLog := model.Log{
				UserId: 1, ChannelId: 1, Type: model.LogTypeConsume, Quota: tc.preConsumed,
				PromptTokens: 100, CompletionTokens: 20, CreatedAt: time.Now().Unix(),
			}
			require.NoError(t, db.Create(&preconsumeLog).Error)
			statsBefore, err := model.SumUsedQuota(model.LogTypeConsume, 0, 0, "", "", "", 1, "")
			require.NoError(t, err)
			require.Equal(t, model.Stat{Quota: tc.preConsumed, Rpm: 1, Tpm: 120}, statsBefore)
			payload := responsePayload{}
			payload.Data.TaskId, payload.Data.TaskStatus, payload.Data.FinalUnitDeduction = "kling-upstream", "succeed", tc.deduction
			body, err := common.Marshal(payload)
			require.NoError(t, err)
			service.GetTaskAdaptorFunc = func(constant.TaskPlatform) service.TaskPollingAdaptor {
				return &pollingFixtureAdaptor{body: string(body)}
			}
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

			err = service.UpdateVideoTasks(context.Background(), "kling", map[int][]string{1: {"kling-upstream"}}, map[string]*model.Task{"kling-upstream": task})
			require.NoError(t, err)
			statsAfter, err := model.SumUsedQuota(model.LogTypeConsume, 0, 0, "", "", "", 1, "")
			require.NoError(t, err)
			if tc.wantQuota == tc.preConsumed {
				assert.Equal(t, statsBefore, statsAfter, "audit-only logs must not change consumption quota, RPM or TPM")
			} else {
				assert.Equal(t, model.Stat{Quota: tc.wantQuota, Rpm: 2, Tpm: 120}, statsAfter, "positive settlement keeps existing consumption statistics")
			}
			require.NoError(t, db.First(task, task.ID).Error)
			assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
			assert.Equal(t, tc.wantQuota, task.Quota)
			require.NoError(t, db.First(&user, 1).Error)
			require.NoError(t, db.First(&token, 1).Error)
			require.NoError(t, db.First(&channel, 1).Error)
			assert.Equal(t, common.MaxQuota-(tc.wantQuota-tc.preConsumed), user.Quota)
			assert.Equal(t, tc.wantQuota, user.UsedQuota)
			assert.Equal(t, 1, user.RequestCount)
			assert.Equal(t, common.MaxQuota-(tc.wantQuota-tc.preConsumed), token.RemainQuota)
			assert.Equal(t, tc.wantQuota, token.UsedQuota)
			assert.Equal(t, int64(tc.wantQuota), channel.UsedQuota)

			var logs []model.Log
			require.NoError(t, db.Where("id > ?", preconsumeLog.Id).Order("id").Find(&logs).Error)
			if tc.wantKind == "" && tc.wantQuota == tc.preConsumed {
				assert.Empty(t, logs)
				assert.NotContains(t, warnings.String(), "quota saturation")
				return
			}
			require.Len(t, logs, 1)
			assert.Equal(t, tc.wantQuota-tc.preConsumed, logs[0].Quota)
			if tc.wantQuota == tc.preConsumed {
				assert.Equal(t, model.LogTypeSystem, logs[0].Type)
			} else {
				assert.Equal(t, model.LogTypeConsume, logs[0].Type)
			}
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
			assert.Equal(t, "kling-public-task", other["task_id"])
			assert.Equal(t, tc.ratio, other["discount"])
			if tc.wantKind == "" {
				assert.NotContains(t, other, "admin_info")
				return
			}
			admin, ok := other["admin_info"].(map[string]any)
			require.True(t, ok, "provider clamp must survive repricing even when the final quota fits")
			saturation, ok := admin["quota_saturation"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tc.wantKind, saturation["kind"])
			assert.Equal(t, tc.wantOriginal, saturation["original"])
			assert.Equal(t, "QuotaFromFloat", saturation["op"])
			assert.Equal(t, 1, strings.Count(warnings.String(), "quota saturation on task log:"))
			assert.Contains(t, warnings.String(), "task=kling-public-task")
			userLogs, _, err := model.GetUserLogs(1, model.LogTypeUnknown, 0, 0, "", "", 0, 10, "", logs[0].RequestId, "")
			require.NoError(t, err)
			require.Len(t, userLogs, 1)
			var userOther map[string]any
			require.NoError(t, common.UnmarshalJsonStr(userLogs[0].Other, &userOther))
			assert.NotContains(t, userOther, "admin_info")
			assert.Equal(t, "kling-public-task", userOther["task_id"])
		})
	}
}
