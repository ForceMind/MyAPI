package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func deliverVerifiedPancakeFixture(t *testing.T, event *service.WaffoPancakeWebhookEvent) int {
	t.Helper()
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/waffo/pancake/webhook/test", nil)
	// Synthetic, already-verified event: exercises fulfillment/ACK only. The
	// provider's private signing key is unavailable and is never substituted.
	fulfillWaffoPancakeWebhook(c, event, "synthetic verified fixture")
	return response.Code
}

func TestWaffoPancakeWebhookDatabaseFailureIsRetryable(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		name := "topup"
		if subscription {
			name = "subscription"
		}
		for _, fault := range []string{"lookup", "write", "commit"} {
			t.Run(name+"/"+fault, func(t *testing.T) {
				db := paymentWebhookTestDB(t)
				tradeNo, queryTable, writeTable := "WAFFO_PANCAKE-fixture", "top_ups", "users"
				if subscription {
					tradeNo, queryTable, writeTable = "WAFFO_PANCAKE_SUB-fixture", "subscription_orders", "subscription_orders"
					plan := model.SubscriptionPlan{Title: "fixture", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 1000, Enabled: true}
					require.NoError(t, db.Create(&plan).Error)
					require.NoError(t, db.Create(&model.SubscriptionOrder{UserId: 1, PlanId: plan.Id, TradeNo: tradeNo, PaymentProvider: model.PaymentProviderWaffoPancake, Status: common.TopUpStatusPending}).Error)
				} else {
					require.NoError(t, db.Create(&model.TopUp{UserId: 1, TradeNo: tradeNo, Amount: 1, PaymentProvider: model.PaymentProviderWaffoPancake, Status: common.TopUpStatusPending}).Error)
				}
				event := &service.WaffoPancakeWebhookEvent{EventType: "order.completed", Mode: "test", Data: service.WaffoPancakeWebhookData{OrderMerchantExternalID: tradeNo, MerchantProvidedBuyerIdentity: "my-api-user-1"}}
				injected := errors.New("injected pancake database failure")
				var restore func()
				switch fault {
				case "lookup":
					require.NoError(t, db.Callback().Query().Before("gorm:query").Register("fixture:pancake-read", func(tx *gorm.DB) {
						if tx.Statement.Table == queryTable {
							tx.AddError(injected)
						}
					}))
					// Prove the service retains both retry classification and DB cause.
					resolve := service.ResolveWaffoPancakeTradeNo
					if subscription {
						resolve = service.ResolveWaffoPancakeSubscriptionTradeNo
					}
					_, err := resolve(event)
					assert.ErrorIs(t, err, service.ErrWaffoPancakeOrderLookupFailed)
					assert.ErrorIs(t, err, injected)
					restore = func() { require.NoError(t, db.Callback().Query().Remove("fixture:pancake-read")) }
				case "write":
					require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fixture:pancake-write", func(tx *gorm.DB) {
						if tx.Statement.Table == writeTable {
							tx.AddError(injected)
						}
					}))
					restore = func() { require.NoError(t, db.Callback().Update().Remove("fixture:pancake-write")) }
				case "commit":
					pool := db.Statement.ConnPool
					sqlDB, err := db.DB()
					require.NoError(t, err)
					db.Statement.ConnPool = webhookFailCommitPool{sqlDB}
					restore = func() { db.Statement.ConnPool = pool }
				}
				code := deliverVerifiedPancakeFixture(t, event)
				restore()
				assert.Equal(t, http.StatusInternalServerError, code)
				var user model.User
				require.NoError(t, db.First(&user, 1).Error)
				assert.Equal(t, 100, user.Quota)
				var count int64
				require.NoError(t, db.Model(&model.UserSubscription{}).Count(&count).Error)
				assert.Zero(t, count)
				require.NoError(t, db.Model(&model.Log{}).Count(&count).Error)
				assert.Zero(t, count)
				assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, event))
				assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, event))
				if subscription {
					require.NoError(t, db.Model(&model.UserSubscription{}).Count(&count).Error)
					assert.EqualValues(t, 1, count)
				} else {
					require.NoError(t, db.First(&user, 1).Error)
					assert.Equal(t, 100+int(common.QuotaPerUnit), user.Quota)
				}
			})
		}
	}
}

func TestWaffoPancakeWebhookRejectsForeignOrMissingOrders(t *testing.T) {
	db := paymentWebhookTestDB(t)
	for _, subscription := range []bool{false, true} {
		tradeNo := "WAFFO_PANCAKE-foreign"
		if subscription {
			tradeNo = "WAFFO_PANCAKE_SUB-foreign"
		}
		event := &service.WaffoPancakeWebhookEvent{Data: service.WaffoPancakeWebhookData{OrderMerchantExternalID: tradeNo, MerchantProvidedBuyerIdentity: "my-api-user-1"}}
		// A truly missing order retains the permanent rejection ACK.
		assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, event))
		if subscription {
			require.NoError(t, db.Create(&model.SubscriptionOrder{UserId: 1, TradeNo: tradeNo, PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending}).Error)
		} else {
			require.NoError(t, db.Create(&model.TopUp{UserId: 1, TradeNo: tradeNo, Amount: 1, PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending}).Error)
		}
		assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, event))
		target := any(&model.TopUp{})
		if subscription {
			target = &model.SubscriptionOrder{}
		}
		require.NoError(t, db.Model(target).Where("trade_no = ?", tradeNo).Update("payment_provider", model.PaymentProviderWaffoPancake).Error)
		event.Data.MerchantProvidedBuyerIdentity = "my-api-user-2"
		assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, event))
		var pending int64
		require.NoError(t, db.Model(target).Where("trade_no = ? AND status = ?", tradeNo, common.TopUpStatusPending).Count(&pending).Error)
		assert.EqualValues(t, 1, pending)
	}
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 100, user.Quota)
}

func TestWaffoPancakeWebhookDoesNotBypassSignature(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	merchant, key, product := setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID
	setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID = "fixture", "fixture", "fixture"
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID = merchant, key, product
	})
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Params = gin.Params{{Key: "env", Value: "test"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/waffo/pancake/webhook/test", strings.NewReader(`{"mode":"test"}`))
	WaffoPancakeWebhook(c)
	assert.Equal(t, http.StatusUnauthorized, response.Code)
}
