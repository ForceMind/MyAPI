package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRealtimeDispatchPreservesSegmentsAndTerminalOwnership(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, terminal := range []string{"reported", "zero", "unknown", "manual"} {
			t.Run(string(mode)+"/"+terminal, func(t *testing.T) {
				db := setupPostConsumeModeDB(t, mode)
				require.NoError(t, db.AutoMigrate(&model.UsageReviewDecision{}, &model.TokenBudget{}, &model.TokenBudgetReservation{}, &model.TokenBudgetPolicyChange{}, &model.Log{}))
				user, token := seedAuthoritativeBilling(t, db, "rt-dispatch", 1000, 1000, false)
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest("GET", "/v1/realtime?model=fixture-model", nil)
				info := authoritativeRelay(user, token, "rt-dispatch")
				info.RelayFormat = types.RelayFormatOpenAIRealtime
				info.ChannelMeta = &relaycommon.ChannelMeta{}
				info.StartTime = time.Now()
				ctx.Set(common.RequestIdKey, info.RequestId)
				session, apiErr := NewBillingSession(ctx, info, 100)
				require.Nil(t, apiErr)
				info.Billing = session
				require.NoError(t, PrepareRealtimeUsageDispatch(ctx, info))
				require.ErrorIs(t, PrepareRealtimeUsageDispatch(ctx, info), model.ErrAccountQuotaUsageUnresolved)
				require.NoError(t, session.Reserve(150))
				require.NoError(t, session.Reserve(150))
				ObserveTextUsageDispatchResponse(info, 429)
				require.ErrorIs(t, session.Refund(ctx), model.ErrAccountQuotaUsageUnresolved)
				view, err := model.GetUsageReview(context.Background(), db, user.Id, info.RequestId)
				require.NoError(t, err)
				assert.True(t, view.TextDispatchPending)
				assert.EqualValues(t, 150, view.ReservedQuota)
				used := 80
				switch terminal {
				case "reported":
					require.NoError(t, session.Settle(80))
					require.NoError(t, session.Settle(80))
					assert.False(t, FinalizeRealtimeUsageDispatch(ctx, info))
				case "zero":
					used = 0
					require.NoError(t, session.Settle(0))
					assert.False(t, FinalizeRealtimeUsageDispatch(ctx, info))
				case "unknown":
					used = 150
					assert.True(t, FinalizeRealtimeUsageDispatch(ctx, info))
					assert.True(t, FinalizeRealtimeUsageDispatch(ctx, info))
					require.ErrorIs(t, session.Reserve(200), model.ErrAccountQuotaUsageUnresolved)
					require.ErrorIs(t, session.Settle(0), model.ErrAccountQuotaUsageUnresolved)
					require.ErrorIs(t, session.Refund(ctx), model.ErrAccountQuotaUsageUnresolved)
					view, err = model.GetUsageReview(context.Background(), db, user.Id, info.RequestId)
					require.NoError(t, err)
					assert.Equal(t, "usage_unknown", view.State)
					assert.Nil(t, view.ActualQuota)
				case "manual":
					used = 20
					_, err = model.RecoverTextDispatchUsage(context.Background(), db, user.Id, info.RequestId, 20, "synthetic ended session evidence")
					require.ErrorIs(t, err, model.ErrAccountQuotaMutationIneligible)
					root := model.User{Username: "rt-root", AffCode: "rt-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
					require.NoError(t, db.Create(&root).Error)
					for range 2 {
						_, err = model.RecoverTextDispatchUsage(context.Background(), db, root.Id, info.RequestId, 20, "synthetic ended session evidence")
						require.NoError(t, err)
					}
					require.Error(t, session.Reserve(200))
					require.Error(t, session.Settle(21))
				}
				uq, remain, consumed := loadPostConsumeBalances(t, db, user, token)
				assert.Equal(t, 1000-used, uq)
				assert.Equal(t, 1000-used, remain)
				assert.Equal(t, used, consumed)
			})
		}
	}
}

func TestRealtimeDispatchFailedPersistenceRetainsHoldUntilRetry(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			require.NoError(t, db.AutoMigrate(&model.Log{}))
			user, token := seedAuthoritativeBilling(t, db, "rt-failure", 1000, 1000, false)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
			info := authoritativeRelay(user, token, "rt-failure")
			info.RelayFormat = types.RelayFormatOpenAIRealtime
			info.ChannelMeta = &relaycommon.ChannelMeta{}
			info.StartTime = time.Now()
			session, apiErr := NewBillingSession(ctx, info, 100)
			require.Nil(t, apiErr)
			info.Billing = session
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("rt-persistence-fail", func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && (tx.Statement.Schema.Table == "legacy_usage_reservations" || tx.Statement.Schema.Table == "account_quota_terminal_recovery_obligations") {
					tx.AddError(model.ErrAccountQuotaMutationCASLost)
				}
			}))
			require.Error(t, PrepareRealtimeUsageDispatch(ctx, info))
			assert.True(t, FinalizeRealtimeUsageDispatch(ctx, info))
			assert.False(t, session.usageHoldPersisted)
			require.ErrorIs(t, session.Refund(ctx), model.ErrAccountQuotaUsageUnresolved)
			require.NoError(t, db.Callback().Update().Remove("rt-persistence-fail"))
			assert.True(t, FinalizeRealtimeUsageDispatch(ctx, info))
			assert.True(t, session.usageHoldPersisted)
			uq, remain, consumed := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 900, uq)
			assert.Equal(t, 900, remain)
			assert.Equal(t, 100, consumed)
		})
	}
}
