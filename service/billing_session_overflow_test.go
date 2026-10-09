package service

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
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
	assert.ErrorIs(t, apiErr, model.ErrSubscriptionQuotaInsufficient)
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)

	require.NoError(t, db.First(user, user.Id).Error)
	assert.Equal(t, 1000, user.Quota, "禁止溢出时钱包不得扣减")
	_, remaining, used := loadPostConsumeBalances(t, db, user, token)
	assert.Equal(t, 500, remaining)
	assert.Zero(t, used)
}

func TestLegacyBillingSessionSubscriptionErrorTextCannotAuthorizeWalletFallback(t *testing.T) {
	for _, phrase := range []string{"no active subscription", "subscription quota insufficient", "unclassified storage outage"} {
		t.Run(phrase, func(t *testing.T) {
			db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
			require.NoError(t, db.AutoMigrate(&model.SubscriptionPreConsumeRecord{}))
			user, token := seedAuthoritativeBilling(t, db, "col", 1000, 500, false)
			require.LessOrEqual(t, len(user.Username), model.UserNameMaxLength)
			require.LessOrEqual(t, len(user.AffCode), 32)
			seedOverflowSubscription(t, db, user.Id, 100, 100, true)
			info := subscriptionFirstRelay(user, token, "subscription-error-collision")
			readErr := errors.New("database read failure: " + phrase)
			wrappedErr := fmt.Errorf("pre-consume record lookup: %w", readErr)
			injected, overflowReads := false, 0
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:subscription-error-collision", func(tx *gorm.DB) {
				if !injected && tx.Statement.Table == "subscription_pre_consume_records" {
					injected = true
					tx.AddError(wrappedErr)
				} else if injected && tx.Statement.Table == "user_subscriptions" {
					overflowReads++
				}
			}))

			ctx := overflowTestGinContext()
			session, apiErr := NewBillingSession(ctx, info, 100)
			if session != nil {
				t.Cleanup(func() { require.NoError(t, session.Refund(ctx)) })
			}
			wallet, remaining, used := loadPostConsumeBalances(t, db, user, token)
			var reservation model.LegacyUsageReservation
			require.NoError(t, db.Where("request_id = ?", info.RequestId).First(&reservation).Error)
			var records int64
			require.NoError(t, db.Model(&model.SubscriptionPreConsumeRecord{}).Count(&records).Error)
			t.Logf("admitted=%t wallet=%d key_remaining=%d key_used=%d reservation_state=%s source=%s reserved=%d token_reserved=%d overflow_reads=%d subscription_records=%d",
				session != nil, wallet, remaining, used, reservation.State, reservation.FundingSource,
				reservation.ReservedQuota, reservation.TokenReservedQuota, overflowReads, records)
			require.True(t, injected, "the failure must originate inside the real subscription pre-consume transaction")
			assert.Nil(t, session, "untyped storage errors must not authorize a different funding source")
			if assert.NotNil(t, apiErr) {
				assert.Equal(t, types.ErrorCodeUpdateDataError, apiErr.GetErrorCode())
				assert.ErrorIs(t, apiErr, readErr)
			}
			assert.Equal(t, 1000, wallet)
			assert.Equal(t, 500, remaining, "the attempted Key reserve must be compensated")
			assert.Zero(t, used)
			assert.Equal(t, model.LegacyUsageAdmissionFailed, reservation.State)
			assert.Equal(t, "pending", reservation.FundingSource)
			assert.Zero(t, reservation.ReservedQuota)
			assert.Zero(t, reservation.TokenReservedQuota)
			assert.Zero(t, overflowReads, "storage failures must not even consult wallet-overflow policy")
			assert.Zero(t, records)
		})
	}
}

func TestLegacyBillingSessionWrappedSubscriptionAdmissionErrors(t *testing.T) {
	for _, businessErr := range []error{model.ErrNoActiveSubscription, model.ErrSubscriptionQuotaInsufficient} {
		t.Run(businessErr.Error(), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
			require.NoError(t, db.AutoMigrate(&model.SubscriptionPreConsumeRecord{}))
			user, token := seedAuthoritativeBilling(t, db, "wrap", 1000, 500, false)
			require.LessOrEqual(t, len(user.Username), model.UserNameMaxLength)
			require.LessOrEqual(t, len(user.AffCode), 32)
			info := subscriptionFirstRelay(user, token, "wrapped-subscription-admission")
			info.UserSetting.BillingPreference = "subscription_only"
			wrappedErr := fmt.Errorf("wrapped funding admission: %w", businessErr)
			injected := false
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:wrapped-subscription-admission", func(tx *gorm.DB) {
				if !injected && tx.Statement.Table == "subscription_pre_consume_records" {
					injected = true
					tx.AddError(wrappedErr)
				}
			}))
			session, apiErr := NewBillingSession(overflowTestGinContext(), info, 100)
			require.True(t, injected)
			assert.Nil(t, session)
			require.NotNil(t, apiErr)
			assert.ErrorIs(t, apiErr, businessErr, "identity must survive the model, FundingSource and API wrapper")
			assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
			assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
			assert.EqualError(t, apiErr.Err, "订阅额度不足或未配置订阅: "+wrappedErr.Error())
			wallet, remaining, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 1000, wallet, "subscription_only never falls back to the wallet")
			assert.Equal(t, 500, remaining)
			assert.Zero(t, used)
		})
	}
}

func TestLegacyBillingSessionSubscriptionAdmissionPreferences(t *testing.T) {
	for _, tc := range []struct {
		name         string
		preference   string
		wallet       int
		subscription bool
		subUsed      int64
		wantSource   string
		wantErr      error
	}{
		{name: "subscription-only-absent", preference: "subscription_only", wallet: 1000, wantErr: model.ErrNoActiveSubscription},
		{name: "subscription-only-exhausted", preference: "subscription_only", wallet: 1000, subscription: true, subUsed: 500, wantErr: model.ErrSubscriptionQuotaInsufficient},
		{name: "subscription-first-absent", preference: "subscription_first", wallet: 1000, wantSource: BillingSourceWallet},
		{name: "subscription-first-funded", preference: "subscription_first", wallet: 1000, subscription: true, wantSource: BillingSourceSubscription},
		{name: "wallet-first-fallback", preference: "wallet_first", wallet: 50, subscription: true, wantSource: BillingSourceSubscription},
		{name: "wallet-only", preference: "wallet_only", wallet: 1000, subscription: true, wantSource: BillingSourceWallet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
			require.NoError(t, db.AutoMigrate(&model.SubscriptionPreConsumeRecord{}))
			user, token := seedAuthoritativeBilling(t, db, "pref", tc.wallet, 500, false)
			require.LessOrEqual(t, len(user.Username), model.UserNameMaxLength)
			require.LessOrEqual(t, len(user.AffCode), 32)
			if tc.subscription {
				seedOverflowSubscription(t, db, user.Id, 500, tc.subUsed, true)
			}
			info := subscriptionFirstRelay(user, token, "subscription-preference")
			info.UserSetting.BillingPreference = tc.preference
			ctx := overflowTestGinContext()
			session, apiErr := NewBillingSession(ctx, info, 100)
			wantWallet, wantRemaining, wantUsed := tc.wallet, 500, 0
			if tc.wantErr != nil {
				assert.Nil(t, session)
				require.NotNil(t, apiErr)
				assert.ErrorIs(t, apiErr, tc.wantErr)
				assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
			} else {
				require.Nil(t, apiErr)
				require.NotNil(t, session)
				t.Cleanup(func() { require.NoError(t, session.Refund(ctx)) })
				assert.Equal(t, tc.wantSource, session.funding.Source())
				wantRemaining, wantUsed = 400, 100
				if tc.wantSource == BillingSourceWallet {
					wantWallet -= 100
				}
			}
			wallet, remaining, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, wantWallet, wallet)
			assert.Equal(t, wantRemaining, remaining)
			assert.Equal(t, wantUsed, used)
		})
	}
}

func TestBillingSessionSubscriptionStorageFailureAcrossWriters(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, stage := range []string{"selection", "write"} {
			t.Run(string(mode)+"/"+stage, func(t *testing.T) {
				db := setupPostConsumeModeDB(t, mode)
				require.NoError(t, db.AutoMigrate(&model.SubscriptionPreConsumeRecord{}))
				user, token := seedAuthoritativeBilling(t, db, "store", 1000, 500, false)
				require.LessOrEqual(t, len(user.Username), model.UserNameMaxLength)
				require.LessOrEqual(t, len(user.AffCode), 32)
				seedOverflowSubscription(t, db, user.Id, 500, 0, true)
				info := subscriptionFirstRelay(user, token, "subscription-storage-failure")
				storageErr := errors.New("storage failure mentioning subscription quota insufficient")
				wrappedErr := fmt.Errorf("subscription %s: %w", stage, storageErr)
				injected := false
				if stage == "selection" {
					require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:subscription-selection-failure", func(tx *gorm.DB) {
						_, selectingSubscriptions := tx.Statement.Dest.(*[]model.UserSubscription)
						if !injected && tx.Statement.Table == "user_subscriptions" && selectingSubscriptions {
							injected = true
							tx.AddError(wrappedErr)
						}
					}))
				} else {
					require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:subscription-write-failure", func(tx *gorm.DB) {
						if !injected && tx.Statement.Table == "user_subscriptions" {
							injected = true
							tx.AddError(wrappedErr)
						}
					}))
				}
				session, apiErr := NewBillingSession(overflowTestGinContext(), info, 100)
				require.True(t, injected)
				assert.Nil(t, session)
				require.NotNil(t, apiErr)
				assert.Equal(t, types.ErrorCodeUpdateDataError, apiErr.GetErrorCode())
				assert.ErrorIs(t, apiErr, storageErr)
				assert.NotErrorIs(t, apiErr, model.ErrSubscriptionQuotaInsufficient)
				wallet, remaining, used := loadPostConsumeBalances(t, db, user, token)
				assert.Equal(t, 1000, wallet)
				assert.Equal(t, 500, remaining)
				assert.Zero(t, used)
				var subscription model.UserSubscription
				require.NoError(t, db.Where("user_id = ?", user.Id).First(&subscription).Error)
				assert.Zero(t, subscription.AmountUsed)
				for _, entity := range []any{&model.SubscriptionPreConsumeRecord{}, &model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{}} {
					var count int64
					require.NoError(t, db.Model(entity).Count(&count).Error)
					assert.Zero(t, count, "failed subscription admission must leave no committed reserve in %T", entity)
				}
				if mode == model.QuotaWriterModeLegacy {
					var reservation model.LegacyUsageReservation
					require.NoError(t, db.Where("request_id = ?", info.RequestId).First(&reservation).Error)
					assert.Equal(t, model.LegacyUsageAdmissionFailed, reservation.State)
					assert.Equal(t, "pending", reservation.FundingSource)
					assert.Zero(t, reservation.ReservedQuota)
					assert.Zero(t, reservation.TokenReservedQuota)
				}
			})
		}
	}
}
