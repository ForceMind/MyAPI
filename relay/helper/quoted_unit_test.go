package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relaytypes "github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuoteHelpersCaptureQuotaUnit(t *testing.T) {
	oldUnit := common.QuotaPerUnit
	oldPrices := ratio_setting.ModelPrice2JSONString()
	oldRatios := ratio_setting.ModelRatio2JSONString()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		common.QuotaPerUnit = oldUnit
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"unit-fixed-fixture":2}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"unit-ratio-fixture":1}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"unit-tiered-fixture":"tiered_expr"}`,
		"billing_setting.billing_expr":    `{"unit-tiered-fixture":"p * 2"}`,
		"group_ratio_setting.group_ratio": `{"default":1}`,
	}))
	t.Cleanup(func() { operation_setting.DeleteToolPriceForTest("quote_capture_tool") })

	for _, test := range []struct {
		name, model string
		perCall     bool
	}{
		{"fixed text", "unit-fixed-fixture", false},
		{"token ratio", "unit-ratio-fixture", false},
		{"per call", "unit-fixed-fixture", true},
		{"tiered expression", "unit-tiered-fixture", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			common.QuotaPerUnit = 100
			operation_setting.SetToolPriceForTest("quote_capture_tool", 5)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("group", "default")
			info := &relaycommon.RelayInfo{
				OriginModelName: test.model, UserGroup: "default", UsingGroup: "default",
			}
			if test.perCall {
				price, err := ModelPriceHelperPerCall(ctx, info)
				require.NoError(t, err)
				common.QuotaPerUnit = 400
				assert.Equal(t, 100.0, price.QuotedQuotaUnit(common.QuotaPerUnit))
				assert.Equal(t, 200, price.Quota)
				operation_setting.SetToolPriceForTest("quote_capture_tool", 50)
				customPrice, captured := price.QuotedToolPrice("quote_capture_tool")
				assert.True(t, captured)
				assert.Equal(t, 5.0, customPrice)
				toolPrice, captured := price.QuotedToolPrice("web_search")
				assert.True(t, captured)
				assert.Equal(t, operation_setting.GetToolPrice("web_search"), toolPrice)
				return
			}
			price, err := ModelPriceHelper(ctx, info, 1_000_000, &relaytypes.TokenCountMeta{})
			require.NoError(t, err)
			common.QuotaPerUnit = 400
			assert.Equal(t, 100.0, price.QuotedQuotaUnit(common.QuotaPerUnit))
			assert.Equal(t, 100.0, info.PriceData.QuotedQuotaUnit(common.QuotaPerUnit))
			operation_setting.SetToolPriceForTest("quote_capture_tool", 50)
			customPrice, captured := info.PriceData.QuotedToolPrice("quote_capture_tool")
			assert.True(t, captured)
			assert.Equal(t, 5.0, customPrice)
			toolPrice, captured := info.PriceData.QuotedToolPrice("web_search")
			assert.True(t, captured)
			assert.Equal(t, operation_setting.GetToolPrice("web_search"), toolPrice)
			if info.TieredBillingSnapshot != nil {
				assert.Equal(t, info.TieredBillingSnapshot.QuotaPerUnit, price.QuotedQuotaUnit(common.QuotaPerUnit))
				assert.Equal(t, 200, price.QuotaToPreConsume)
			} else if price.UsePrice {
				assert.Equal(t, 200, price.QuotaToPreConsume)
			}
		})
	}
}
