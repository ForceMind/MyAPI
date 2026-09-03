package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relay/helper"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskSubmissionBillingContextCapturesFinalSpecialGroupAndFreeRates(t *testing.T) {
	for _, tc := range []struct {
		name, modelRatios, groupRatios, specialRatios string
		wantModelRatio, wantGroupRatio                float64
		wantFree                                      bool
	}{
		{"special routed group", `{"task-snapshot-model":2}`, `{"route":1}`, `{"vip":{"route":0.5},"route":{"route":0.9}}`, 2, 0.5, false},
		{"free model", `{"task-snapshot-model":0}`, `{"route":1}`, `{}`, 0, 1, true},
		{"free group", `{"task-snapshot-model":2}`, `{"route":0}`, `{}`, 2, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldModelRatios := ratio_setting.ModelRatio2JSONString()
			oldGroupRatios := ratio_setting.GroupRatio2JSONString()
			oldSpecialRatios := ratio_setting.GroupGroupRatio2JSONString()
			quotaSettings := operation_setting.GetQuotaSetting()
			oldFreePreconsume := quotaSettings.EnableFreeModelPreConsume
			quotaSettings.EnableFreeModelPreConsume = false
			t.Cleanup(func() {
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatios))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatios))
				require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldSpecialRatios))
				quotaSettings.EnableFreeModelPreConsume = oldFreePreconsume
			})
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(tc.modelRatios))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(tc.groupRatios))
			require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(tc.specialRatios))
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("auto_group", "route")
			info := &relaycommon.RelayInfo{OriginModelName: "task-snapshot-model", UserGroup: "vip", UsingGroup: "initial"}
			price, err := helper.ModelPriceHelperPerCall(ctx, info)
			require.NoError(t, err)
			info.PriceData = price
			info.PriceData.AddOtherRatio("duration", 3)
			context := model.NewTaskBillingContext(info)
			assert.Equal(t, "route", info.UsingGroup)
			assert.Equal(t, tc.wantFree, price.FreeModel)
			assert.Equal(t, tc.wantModelRatio, context.ModelRatio)
			assert.Equal(t, tc.wantGroupRatio, context.GroupRatio)
			assert.Equal(t, -1.0, context.ModelPrice)
			assert.True(t, context.Complete)
			assert.Equal(t, model.TaskBillingContextVersion, context.Version)
			info.PriceData.AddOtherRatio("duration", 7)
			assert.Equal(t, map[string]float64{"duration": 3}, context.OtherRatios)
		})
	}
}
