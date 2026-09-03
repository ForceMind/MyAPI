package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskTokenBillingSnapshotCompatibilityAndZeroRates(t *testing.T) {
	for _, tc := range []struct {
		name                                                                                   string
		change                                                                                 func(*model.TaskBillingContext)
		noContext, noUsage, perCall, inMemoryOnly, deletedConfig, disabledConfig, specialGroup bool
		missingGroupConfig, missingUserGroup, missingUser                                      bool
		wantQuota                                                                              int
		wantSource                                                                             string
		wantModelRatio, wantGroupRatio                                                         float64
		noLog                                                                                  bool
	}{
		{name: "new frozen rates", wantQuota: 300, wantSource: "snapshot_v1", wantModelRatio: 2, wantGroupRatio: 0.5},
		{name: "deleted configuration", deletedConfig: true, wantQuota: 300, wantSource: "snapshot_v1", wantModelRatio: 2, wantGroupRatio: 0.5},
		{name: "deleted group configuration", missingGroupConfig: true, wantQuota: 300, wantSource: "snapshot_v1", wantModelRatio: 2, wantGroupRatio: 0.5},
		{name: "changed user group", specialGroup: true, wantQuota: 300, wantSource: "snapshot_v1", wantModelRatio: 2, wantGroupRatio: 0.5},
		{name: "complete legacy deleted configuration", change: func(bc *model.TaskBillingContext) { bc.Version = 0; bc.Complete = false }, deletedConfig: true, wantQuota: 300, wantSource: "legacy_snapshot", wantModelRatio: 2, wantGroupRatio: 0.5},
		{name: "nil legacy context", noContext: true, wantQuota: 400, wantSource: "legacy_current", wantModelRatio: 4, wantGroupRatio: 1},
		{name: "legacy current special user group", noContext: true, specialGroup: true, wantQuota: 100, wantSource: "legacy_current", wantModelRatio: 4, wantGroupRatio: 0.25},
		{name: "legacy missing group ratio", change: func(bc *model.TaskBillingContext) { bc.Version = 0; bc.Complete = false; bc.GroupRatio = 0 }, wantQuota: 1200, wantSource: "legacy_current", wantModelRatio: 4, wantGroupRatio: 1},
		{name: "legacy ambiguous zero model", change: func(bc *model.TaskBillingContext) { bc.Version = 0; bc.Complete = false; bc.ModelRatio = 0 }, wantQuota: 1200, wantSource: "legacy_current", wantModelRatio: 4, wantGroupRatio: 1},
		{name: "legacy current disabled", change: func(bc *model.TaskBillingContext) { bc.Version = 0; bc.Complete = false; bc.ModelRatio = 0 }, disabledConfig: true, wantQuota: 500, wantSource: "legacy_current"},
		{name: "legacy group configuration missing", noContext: true, missingGroupConfig: true, wantQuota: 500, wantSource: "legacy_current"},
		{name: "legacy user group unavailable", noContext: true, missingUserGroup: true, wantQuota: 500, wantSource: "legacy_current"},
		{name: "legacy user unavailable", noContext: true, missingUser: true, wantQuota: 500, wantSource: "legacy_current"},
		{name: "explicit free model", change: func(bc *model.TaskBillingContext) { bc.ModelRatio = 0 }, wantQuota: 0, wantSource: "snapshot_v1", wantModelRatio: 0, wantGroupRatio: 0.5},
		{name: "explicit free group", change: func(bc *model.TaskBillingContext) { bc.GroupRatio = 0 }, wantQuota: 0, wantSource: "snapshot_v1", wantModelRatio: 2, wantGroupRatio: 0},
		{name: "free with unavailable usage", change: func(bc *model.TaskBillingContext) { bc.ModelRatio = 0 }, noUsage: true, wantQuota: 500, noLog: true},
		{name: "positive rates truncated to zero keep legacy behavior", change: func(bc *model.TaskBillingContext) { bc.ModelRatio = 0.000001 }, wantQuota: 500, noLog: true},
		{name: "per-call is not repriced", perCall: true, wantQuota: 500, noLog: true},
		{name: "future snapshot", change: func(bc *model.TaskBillingContext) { bc.Version = 2 }, wantQuota: 500, wantSource: "unsupported_snapshot"},
		{name: "incomplete new snapshot", change: func(bc *model.TaskBillingContext) { bc.Complete = false }, wantQuota: 500, wantSource: "invalid_snapshot"},
		{name: "negative model rate", change: func(bc *model.TaskBillingContext) { bc.ModelRatio = -1 }, wantQuota: 500, wantSource: "invalid_snapshot"},
		{name: "negative group rate", change: func(bc *model.TaskBillingContext) { bc.GroupRatio = -1 }, wantQuota: 500, wantSource: "invalid_snapshot"},
		{name: "nonfinite model rate", change: func(bc *model.TaskBillingContext) { bc.ModelRatio = math.NaN() }, inMemoryOnly: true, wantQuota: 500, wantSource: "invalid_snapshot"},
		{name: "nonfinite group rate", change: func(bc *model.TaskBillingContext) { bc.GroupRatio = math.Inf(1) }, inMemoryOnly: true, wantQuota: 500, wantSource: "invalid_snapshot"},
		{name: "nonfinite logged price", change: func(bc *model.TaskBillingContext) { bc.ModelPrice = math.Inf(-1) }, inMemoryOnly: true, wantQuota: 500, wantSource: "invalid_snapshot"},
		{name: "invalid zero other ratio", change: func(bc *model.TaskBillingContext) { bc.OtherRatios["duration"] = 0 }, wantQuota: 500, wantSource: "invalid_snapshot"},
		{name: "invalid infinite other ratio", change: func(bc *model.TaskBillingContext) { bc.OtherRatios["duration"] = math.Inf(1) }, inMemoryOnly: true, wantQuota: 500, wantSource: "invalid_snapshot"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			oldModels, oldGroups, oldSpecial := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString(), ratio_setting.GroupGroupRatio2JSONString()
			oldConsume, oldExport := common.LogConsumeEnabled, common.DataExportEnabled
			common.LogConsumeEnabled, common.DataExportEnabled = true, false
			t.Cleanup(func() {
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModels))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroups))
				require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldSpecial))
				common.LogConsumeEnabled, common.DataExportEnabled = oldConsume, oldExport
			})
			models := `{"b1-snapshot":4}`
			if tc.deletedConfig {
				models = `{}`
			}
			if tc.disabledConfig {
				models = `{"b1-snapshot":0}`
			}
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(models))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"paid":1}`))
			if tc.missingGroupConfig {
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{}`))
			}
			require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"paid":0.25},"paid":{"paid":0.75}}`))
			if !tc.missingUser {
				seedUser(t, 84, 10000)
			}
			seedChannel(t, 84)
			seedToken(t, 84, 84, "b1-compat-token", 10000)
			seedChargedAccounting(t, 84, 84, 84, 500, 1)
			task := makeTask(84, 84, 500, 84, BillingSourceWallet, 0)
			task.Properties.OriginModelName = "b1-snapshot"
			if tc.specialGroup {
				task.Group = "paid"
				require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 84).Update("group", "vip").Error)
			}
			if tc.missingUserGroup {
				require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 84).Update("group", "").Error)
			}
			bc := &model.TaskBillingContext{Version: model.TaskBillingContextVersion, Complete: true, ModelPrice: -1, ModelRatio: 2, GroupRatio: 0.5, OriginModelName: "b1-snapshot", OtherRatios: map[string]float64{"duration": 3}, PerCallBilling: tc.perCall}
			if tc.change != nil {
				tc.change(bc)
			}
			task.PrivateData.BillingContext = bc
			if tc.noContext || tc.inMemoryOnly {
				task.PrivateData.BillingContext = nil
			}
			require.NoError(t, model.DB.Create(task).Error)
			var storedBefore model.Task
			require.NoError(t, model.DB.First(&storedBefore, task.ID).Error)
			if tc.inMemoryOnly {
				task.PrivateData.BillingContext = bc
			}
			initialLog := model.Log{UserId: 84, ChannelId: 84, Type: model.LogTypeConsume, Quota: 500, PromptTokens: 100, CompletionTokens: 20, CreatedAt: time.Now().Unix()}
			require.NoError(t, model.LOG_DB.Create(&initialLog).Error)
			statsBefore, err := model.SumUsedQuota(model.LogTypeConsume, 0, 0, "", "", "", 84, "")
			require.NoError(t, err)
			tokens := 100
			if tc.noUsage {
				tokens = 0
			}

			RecalculateTaskQuotaByTokens(context.Background(), task, tokens)

			var storedAfter model.Task
			require.NoError(t, model.DB.First(&storedAfter, task.ID).Error)
			assert.Equal(t, storedBefore.PrivateData.BillingContext, storedAfter.PrivateData.BillingContext, "settlement must never overwrite submission history")
			assert.Equal(t, tc.wantQuota, storedAfter.Quota)
			if !tc.missingUser {
				assert.Equal(t, 10000+500-tc.wantQuota, getUserQuota(t, 84))
				used, requests := getUserUsageAccounting(t, 84)
				assert.Equal(t, tc.wantQuota, used)
				assert.Equal(t, 1, requests)
			}
			assert.Equal(t, 10000+500-tc.wantQuota, getTokenRemainQuota(t, 84))
			assert.Equal(t, tc.wantQuota, getTokenUsedQuota(t, 84))
			assert.Equal(t, int64(tc.wantQuota), getChannelUsedQuota(t, 84))
			if tc.wantQuota == 500 {
				statsAfter, err := model.SumUsedQuota(model.LogTypeConsume, 0, 0, "", "", "", 84, "")
				require.NoError(t, err)
				assert.Equal(t, statsBefore, statsAfter)
			}
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("id > ?", initialLog.Id).Find(&logs).Error)
			if tc.noLog {
				assert.Empty(t, logs)
				return
			}
			require.Len(t, logs, 1)
			wantType, wantLogQuota := model.LogTypeSystem, 0
			if tc.wantQuota < 500 {
				wantType, wantLogQuota = model.LogTypeRefund, 500-tc.wantQuota
			}
			if tc.wantQuota > 500 {
				wantType, wantLogQuota = model.LogTypeConsume, tc.wantQuota-500
			}
			assert.Equal(t, wantType, logs[0].Type)
			assert.Equal(t, wantLogQuota, logs[0].Quota)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
			assert.Equal(t, tc.wantSource, other["billing_rate_source"])
			assert.Equal(t, float64(tc.wantQuota), other["actual_quota"])
			if tc.wantQuota != 500 {
				assert.Equal(t, tc.wantModelRatio, other["model_ratio"])
				assert.Equal(t, tc.wantGroupRatio, other["group_ratio"])
			} else {
				assert.NotContains(t, other, "model_ratio", "unusable rates must not masquerade as applied prices")
				assert.NotContains(t, other, "group_ratio")
			}
		})
	}
}

func TestTaskFreeSnapshotDoesNotRefundUnusableUsage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value float64
	}{
		{"nan", math.NaN()}, {"negative infinity", math.Inf(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 85, 10000)
			seedChannel(t, 85)
			seedToken(t, 85, 85, "free-invalid-usage", 10000)
			seedChargedAccounting(t, 85, 85, 85, 500, 1)
			task := makeTask(85, 85, 500, 85, BillingSourceWallet, 0)
			task.PrivateData.BillingContext = &model.TaskBillingContext{
				Version: model.TaskBillingContextVersion, Complete: true,
				ModelPrice: -1, ModelRatio: 0, GroupRatio: 1, OriginModelName: "free-snapshot",
			}
			require.NoError(t, model.DB.Create(task).Error)
			_, clamp := common.QuotaFromFloatChecked(tc.value)
			require.NotNil(t, clamp)

			RecalculateTaskQuotaByTokens(context.Background(), task, 0, clamp)

			assert.Equal(t, 500, getTaskQuota(t, task.ID))
			assert.Equal(t, 10000, getUserQuota(t, 85))
			assert.Equal(t, 10000, getTokenRemainQuota(t, 85))
			log := getLastLog(t)
			require.NotNil(t, log)
			assert.Equal(t, model.LogTypeSystem, log.Type)
			assert.Zero(t, log.Quota)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
			admin, ok := other["admin_info"].(map[string]any)
			require.True(t, ok)
			assert.Contains(t, admin, "quota_saturation")
		})
	}
}
