package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLegacyExtensionPreservesAtomicReservationEvidence(t *testing.T) {
	for _, scenario := range []string{"journal-failure", "external-hold"} {
		t.Run(scenario, func(t *testing.T) {
			db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
			user, token := seedAuthoritativeBilling(t, db, "extension-atomic", 1000, 1000, false)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
			info := authoritativeRelay(user, token, "extension-atomic-request")
			session, apiErr := NewBillingSession(ctx, info, 100)
			require.Nil(t, apiErr)
			info.Billing = session
			if scenario == "external-hold" {
				require.NoError(t, model.UpdateLegacyUsageReservation(context.Background(), db, info.RequestId, 100, 100, model.LegacyUsageUnknown, "missing", nil))
			} else {
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("extension-journal-fail", func(tx *gorm.DB) {
					if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "legacy_usage_reservations" {
						tx.AddError(model.ErrAccountQuotaMutationCASLost)
					}
				}))
				t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove("extension-journal-fail")) })
			}
			require.Error(t, session.Reserve(200))
			uq, remaining, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 900, uq, "failed extension must not change wallet")
			assert.Equal(t, 900, remaining, "failed extension must not change key quota")
			assert.Equal(t, 100, used)
			row, err := model.FindLegacyUsageReservation(context.Background(), db, info.RequestId)
			require.NoError(t, err)
			assert.EqualValues(t, 100, row.ReservedQuota)
			assert.EqualValues(t, 100, row.TokenReservedQuota)
		})
	}
}

func TestLegacyExtendedReservationFinalizesExactCumulativeAmount(t *testing.T) {
	for _, source := range []string{"wallet", "subscription"} {
		for _, terminal := range []string{"settle", "refund"} {
			t.Run(source+"/"+terminal, func(t *testing.T) {
				db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
				require.NoError(t, db.AutoMigrate(&model.SubscriptionPreConsumeRecord{}))
				user, token := seedAuthoritativeBilling(t, db, "extend-final", 1000, 1000, false)
				info := authoritativeRelay(user, token, "extend-final")
				var sub model.UserSubscription
				if source == "subscription" {
					plan := model.SubscriptionPlan{Title: "extension", Enabled: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetNever}
					require.NoError(t, db.Create(&plan).Error)
					sub = model.UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), Status: "active"}
					require.NoError(t, db.Create(&sub).Error)
					info.UserSetting.BillingPreference = "subscription_only"
				}
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
				session, apiErr := NewBillingSession(ctx, info, 100)
				require.Nil(t, apiErr)
				for range 2 {
					require.NoError(t, session.Reserve(200))
				}
				assert.Equal(t, 200, session.GetPreConsumedQuota())
				used := 150
				if terminal == "refund" {
					used = 0
					require.NoError(t, session.Refund(ctx))
					require.NoError(t, session.Refund(ctx))
				} else {
					require.NoError(t, session.Settle(150))
					require.NoError(t, session.Settle(150))
				}
				uq, remaining, actualUsed := loadPostConsumeBalances(t, db, user, token)
				wantWallet := 1000 - used
				if source == "subscription" {
					wantWallet = 1000
					require.NoError(t, db.First(&sub, sub.Id).Error)
					assert.EqualValues(t, used, sub.AmountUsed)
				}
				assert.Equal(t, wantWallet, uq)
				assert.Equal(t, 1000-used, remaining)
				assert.Equal(t, used, actualUsed)
			})
		}
	}
}
