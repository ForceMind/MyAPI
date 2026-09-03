package controller

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81/webhook"
	"gorm.io/gorm"
)

func paymentWebhookTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}, &model.SubscriptionOrder{}, &model.SubscriptionPlan{}, &model.UserSubscription{}))
	oldDB, oldLogDB, oldRedis := model.DB, model.LOG_DB, common.RedisEnabled
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLogDB, oldRedis
		common.SetDatabaseTypes(oldMainType, oldLogType)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "webhook-fixture", Quota: 100}).Error)
	return db
}

// This fixture models a failed commit with a definite rollback; no real payment
// provider or database server is contacted. It is not an ambiguous-commit test.
type webhookFailCommitPool struct{ *sql.DB }

func (p webhookFailCommitPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &webhookFailCommitTx{tx}, nil
}

type webhookFailCommitTx struct{ *sql.Tx }

func (tx webhookFailCommitTx) Commit() error {
	_ = tx.Tx.Rollback()
	return errors.New("injected commit failure")
}

func stripeWebhookFixture(t *testing.T) {
	t.Helper()
	confirmPaymentComplianceForTest(t)
	api, secret, price := setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId
	setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = "sk_test_fixture", "whsec_fixture", "price_fixture"
	t.Cleanup(func() {
		setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = api, secret, price
	})
}

func deliverStripeFixture(t *testing.T, eventType, tradeNo, status, secret string) int {
	t.Helper()
	payload, err := common.Marshal(map[string]any{
		"id": "evt_fixture", "object": "event", "type": eventType,
		"data": map[string]any{"object": map[string]any{
			"id": "cs_fixture", "object": "checkout.session", "client_reference_id": tradeNo,
			"status": status, "payment_status": "paid", "customer": "cus_fixture", "amount_total": 100, "currency": "usd",
		}},
	})
	require.NoError(t, err)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: secret})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(string(payload)))
	request.Header.Set("Stripe-Signature", signed.Header)
	router := gin.New()
	router.POST("/api/stripe/webhook", StripeWebhook)
	router.ServeHTTP(response, request)
	return response.Code
}

func TestStripeWebhookDatabaseFailureIsRetryable(t *testing.T) {
	for _, fault := range []string{"subscription-read", "topup-read", "wallet-write", "topup-write", "commit", "failed-write", "expired-read"} {
		t.Run(fault, func(t *testing.T) {
			db := paymentWebhookTestDB(t)
			stripeWebhookFixture(t)
			order := model.TopUp{UserId: 1, TradeNo: "stripe-fixture", Money: 1, Amount: 1, PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusPending}
			require.NoError(t, db.Create(&order).Error)
			eventType, status := "checkout.session.completed", "complete"
			if fault == "failed-write" {
				eventType = "checkout.session.async_payment_failed"
			}
			if fault == "expired-read" {
				eventType, status = "checkout.session.expired", "expired"
			}
			injected := errors.New("injected database failure")
			var restore func()
			switch fault {
			case "subscription-read", "topup-read", "expired-read":
				table := "subscription_orders"
				if fault == "topup-read" {
					table = "top_ups"
				}
				require.NoError(t, db.Callback().Query().Before("gorm:query").Register("fixture:query-failure", func(tx *gorm.DB) {
					if tx.Statement.Table == table {
						tx.AddError(injected)
					}
				}))
				restore = func() { require.NoError(t, db.Callback().Query().Remove("fixture:query-failure")) }
			case "wallet-write", "topup-write", "failed-write":
				table := "top_ups"
				if fault == "wallet-write" {
					table = "users"
				}
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fixture:write-failure", func(tx *gorm.DB) {
					if tx.Statement.Table == table {
						tx.AddError(injected)
					}
				}))
				restore = func() { require.NoError(t, db.Callback().Update().Remove("fixture:write-failure")) }
			case "commit":
				pool := db.Statement.ConnPool
				sqlDB, err := db.DB()
				require.NoError(t, err)
				db.Statement.ConnPool = webhookFailCommitPool{sqlDB}
				restore = func() { db.Statement.ConnPool = pool }
			}
			code := deliverStripeFixture(t, eventType, order.TradeNo, status, setting.StripeWebhookSecret)
			restore()
			assert.Equal(t, http.StatusInternalServerError, code)
			var user model.User
			require.NoError(t, db.First(&user, 1).Error)
			assert.Equal(t, 100, user.Quota)
			require.NoError(t, db.First(&order, order.Id).Error)
			assert.Equal(t, common.TopUpStatusPending, order.Status)
			var logs int64
			require.NoError(t, db.Model(&model.Log{}).Count(&logs).Error)
			assert.Zero(t, logs)
			// Resending the same signed event after recovery is safe and succeeds.
			assert.Equal(t, http.StatusOK, deliverStripeFixture(t, eventType, order.TradeNo, status, setting.StripeWebhookSecret))
			assert.Equal(t, http.StatusOK, deliverStripeFixture(t, eventType, order.TradeNo, status, setting.StripeWebhookSecret))
			require.NoError(t, db.First(&user, 1).Error)
			expectedQuota := 100
			if status == "complete" && fault != "failed-write" {
				expectedQuota += int(common.QuotaPerUnit)
			}
			assert.Equal(t, expectedQuota, user.Quota)
		})
	}
}

func TestStripeWebhookProtectsSuccessfulAndForeignOrders(t *testing.T) {
	db := paymentWebhookTestDB(t)
	stripeWebhookFixture(t)
	order := model.TopUp{UserId: 1, TradeNo: "stripe-success", Money: 1, Amount: 1, PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(&order).Error)
	assert.Equal(t, http.StatusBadRequest, deliverStripeFixture(t, "checkout.session.completed", order.TradeNo, "complete", "wrong-secret"))
	assert.Equal(t, http.StatusOK, deliverStripeFixture(t, "checkout.session.completed", order.TradeNo, "complete", setting.StripeWebhookSecret))
	for _, event := range []struct{ kind, status string }{{"checkout.session.completed", "complete"}, {"checkout.session.async_payment_succeeded", "complete"}, {"checkout.session.async_payment_failed", "complete"}, {"checkout.session.expired", "expired"}} {
		assert.Equal(t, http.StatusOK, deliverStripeFixture(t, event.kind, order.TradeNo, event.status, setting.StripeWebhookSecret))
	}
	require.NoError(t, db.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 100+int(common.QuotaPerUnit), user.Quota)
	var logs int64
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeTopup).Count(&logs).Error)
	assert.EqualValues(t, 1, logs)
	foreign := model.TopUp{UserId: 1, TradeNo: "creem-foreign", Money: 1, Amount: 1, PaymentProvider: model.PaymentProviderCreem, Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(&foreign).Error)
	assert.Equal(t, http.StatusOK, deliverStripeFixture(t, "checkout.session.completed", foreign.TradeNo, "complete", setting.StripeWebhookSecret))
	require.NoError(t, db.First(&foreign, foreign.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, foreign.Status)
}
