package controller

import (
	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminCompleteTopUpMissingSnapshotRequiresReconciliation(t *testing.T) {
	db := paymentWebhookTestDB(t)
	order := model.TopUp{UserId: 1, TradeNo: "legacy-no-unit", Amount: 2, PaymentProvider: model.PaymentProviderEpay, Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(&order).Error)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/topup/complete", strings.NewReader(`{"trade_no":"legacy-no-unit"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	AdminCompleteTopUp(c)
	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "TOPUP_QUOTA_UNIT_UNRESOLVED")
	require.NoError(t, db.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 100, user.Quota)
}
