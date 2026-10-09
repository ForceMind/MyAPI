package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Synthetic service regressions: a failed journal acknowledgement must not
// turn applied accounting into another manual settlement. No upstream is sent.
func TestUsageReviewZeroReserveSettlementJournal(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, actual := range []int{0, 60} {
			for _, failJournal := range []bool{false, true} {
				if failJournal && mode != model.QuotaWriterModeLegacy {
					continue
				}
				name := string(mode)
				if actual == 0 {
					name += "/actual-zero"
				} else {
					name += "/actual-sixty"
				}
				if failJournal {
					name += "/journal-failure"
				} else {
					name += "/healthy"
				}
				t.Run(name, func(t *testing.T) {
					db := setupPostConsumeModeDB(t, mode)
					require.NoError(t, db.AutoMigrate(&model.UsageReviewDecision{}, &model.TokenBudget{}, &model.TokenBudgetReservation{}, &model.Log{}))
					trust := common.GetTrustQuota()
					user, token := seedAuthoritativeBilling(t, db, "zero-dispatch", trust+1000, trust+1000, false)
					ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
					ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
					ctx.Set("token_quota", token.RemainQuota)
					info := authoritativeRelay(user, token, "zero-dispatch-request")
					info.ChannelMeta = &relaycommon.ChannelMeta{}
					info.StartTime = time.Now()
					ctx.Set(common.RequestIdKey, info.RequestId)
					session, apiErr := NewBillingSession(ctx, info, 100)
					require.Nil(t, apiErr)
					info.Billing = session
					require.Zero(t, info.FinalPreConsumedQuota)
					req := httptest.NewRequest("POST", "https://synthetic.invalid/never-sent", nil)
					require.NoError(t, PrepareTextUsageDispatch(ctx, req, info))
					if failJournal {
						require.NoError(t, db.Callback().Update().Before("gorm:update").Register("synthetic-journal-failure", func(tx *gorm.DB) {
							if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "legacy_usage_reservations" {
								tx.AddError(errors.New("synthetic journal update unavailable"))
							}
						}))
					}
					err := SettleBilling(ctx, info, actual)
					if failJournal {
						require.ErrorContains(t, err, "synthetic journal update unavailable")
						require.NoError(t, db.Callback().Update().Remove("synthetic-journal-failure"))
						oldLogDB := model.LOG_DB
						model.LOG_DB = db
						t.Cleanup(func() { model.LOG_DB = oldLogDB })
						recordPendingBillingSettlement(ctx, info)
						var log model.Log
						require.NoError(t, db.Where("request_id = ?", info.RequestId).First(&log).Error)
						require.Equal(t, "usage_settlement_pending", log.Content)
					} else {
						require.NoError(t, err)
					}
					require.False(t, FinalizeTextUsageDispatch(ctx, info), "settled in-memory session is not downgraded to unknown")
					view, err := model.GetUsageReview(context.Background(), db, user.Id, info.RequestId)
					require.NoError(t, err)
					require.Equal(t, failJournal && actual == 0, view.CanRecoverTextDispatch)
					if failJournal && actual > 0 {
						require.Equal(t, "applied_journal_pending", view.SettlementStatus)
						require.Equal(t, "automatic_settlement_applied", view.RecoveryBlockReason)
						require.False(t, view.CanReconcileUsage)
						require.False(t, view.TextDispatchPending)
						require.NotNil(t, view.ActualQuota)
						require.EqualValues(t, actual, *view.ActualQuota)
					}
					if failJournal {
						require.Equal(t, model.LegacyUsagePrepared, view.State)
						if actual == 0 {
							require.Nil(t, view.ActualQuota, "no durable accounting fact proves the zero result after journal failure")
							require.Equal(t, "none", view.SettlementStatus)
						}
						row, readErr := model.FindLegacyUsageReservation(context.Background(), db, info.RequestId)
						require.NoError(t, readErr)
						require.Equal(t, model.LegacyUsagePrepared, row.State)
						require.Nil(t, row.ActualQuota, "GET must never repair the journal")
					}
					uq, remaining, used := loadPostConsumeBalances(t, db, user, token)
					require.Equal(t, trust+1000-actual, uq)
					require.Equal(t, trust+1000-actual, remaining)
					require.Equal(t, actual, used)
					t.Logf("state=%s can_recover=%v reserved=%d actual=%d charged=%d journal_failure=%v", view.State, view.CanRecoverTextDispatch, view.ReservedQuota, actual, trust+1000-uq, failJournal)
				})
			}
		}
	}
}
