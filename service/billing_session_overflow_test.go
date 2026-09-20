package service

import (
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func overflowTestGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	return c
}

// seedOverflowSubscription seeds an exhausted active subscription so the
// subscription_first preference must fall back to the wallet (or be blocked
// when the plan disallows wallet overflow).
func seedOverflowSubscription(t *testing.T, db *gorm.DB, userId int, total, used int64, allowOverflow bool) {
	t.Helper()
	plan := &model.SubscriptionPlan{Title: "overflow-plan", PriceAmount: 1, Currency: "USD", DurationUnit: model.SubscriptionDurationDay, DurationValue: 1, Enabled: true, TotalAmount: total}
	plan.NormalizeDefaults()
	require.NoError(t, db.Create(plan).Error)
	sub := &model.UserSubscription{
		UserId: userId, PlanId: plan.Id, AmountTotal: total, AmountUsed: used,
		StartTime: 1, EndTime: 1<<31 - 1, Status: "active", AllowWalletOverflow: allowOverflow,
	}
	require.NoError(t, db.Create(sub).Error)
}

func subscriptionFirstRelay(user *model.User, token *model.Token, requestID string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, TokenUnlimited: token.UnlimitedQuota,
		RequestId: requestID, OriginModelName: "fixture-model", UserQuota: user.Quota,
		UserSetting: dto.UserSetting{BillingPreference: "subscription_first"}}
}

// TestLegacyBillingSessionSubscriptionOverflowToWallet 覆盖 overflow 的 legacy
// 路径：订阅额度耗尽且套餐允许钱包溢出时，预扣回退到钱包并精确扣减；
// 非 legacy 模式经 newAuthoritativeBillingSession 的 Reserve 内核分流
// （bridge fail-closed 已由 TestAuthoritativeBillingSession* 覆盖）。
func TestLegacyBillingSessionSubscriptionOverflowToWallet(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPreConsumeRecord{}))
	user, token := seedAuthoritativeBilling(t, db, "legacy-overflow", 1000, 500, false)
	seedOverflowSubscription(t, db, user.Id, 100, 100, true)

	session, apiErr := NewBillingSession(overflowTestGinContext(), subscriptionFirstRelay(user, token, "legacy-overflow-req"), 100)
	require.Nil(t, apiErr)
	assert.Equal(t, BillingSourceWallet, session.funding.Source())
	assert.Equal(t, 100, session.GetPreConsumedQuota())

	require.NoError(t, db.First(user, user.Id).Error)
	assert.Equal(t, 900, user.Quota, "overflow 后钱包精确扣减预扣额度")
}

// TestLegacyBillingSessionSubscriptionOverflowBlocked 订阅耗尽且套餐禁止钱包
// 溢出时：返回订阅额度不足，钱包零写。
func TestLegacyBillingSessionSubscriptionOverflowBlocked(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPreConsumeRecord{}))
	user, token := seedAuthoritativeBilling(t, db, "legacy-overflow-blocked", 1000, 500, false)
	seedOverflowSubscription(t, db, user.Id, 100, 100, false)

	_, apiErr := NewBillingSession(overflowTestGinContext(), subscriptionFirstRelay(user, token, "legacy-overflow-blocked-req"), 100)
	require.NotNil(t, apiErr)

	require.NoError(t, db.First(user, user.Id).Error)
	assert.Equal(t, 1000, user.Quota, "禁止溢出时钱包不得扣减")
}
