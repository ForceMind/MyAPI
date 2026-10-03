package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTextDispatchUncertaintyPreservesReservationsAcrossWriters(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, noBalance := range []bool{false, true} {
			funding := "wallet"
			if noBalance {
				funding = "self-use"
			}
			for _, tc := range []struct {
				name               string
				status             int
				sent, settle, held bool
			}{
				{"preflight", 0, false, false, false},
				{"network", 0, true, false, true},
				{"unreadable-success", 200, true, false, true},
				{"redirect", 302, true, false, true},
				{"server-failure", 502, true, false, true},
				{"timeout", 408, true, false, true},
				{"conflict", 409, true, false, true},
				{"rate-refusal", 429, true, false, false},
				{"auth-refusal", 401, true, false, false},
				{"reported-success", 200, true, true, false},
			} {
				t.Run(string(mode)+"/"+funding+"/"+tc.name, func(t *testing.T) {
					db := setupPostConsumeModeDB(t, mode)
					require.NoError(t, db.AutoMigrate(&model.UsageReviewDecision{}, &model.TokenBudget{}, &model.TokenBudgetReservation{}, &model.TokenBudgetPolicyChange{}, &model.Log{}))
					quota := 1000
					if noBalance {
						quota = 0
					}
					user, token := seedAuthoritativeBilling(t, db, "text-dispatch", quota, 1000, false)
					if noBalance {
						require.NoError(t, db.AutoMigrate(&model.Option{}))
						require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
							_, err := model.InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeDisabled)
							return err
						}))
						require.NoError(t, db.Model(user).Updates(map[string]any{"self_use_no_balance": true, "usage_policy_revision": 1}).Error)
					}
					ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
					path := "/v1/chat/completions"
					if !noBalance {
						path += "?trace=synthetic"
					}
					ctx.Request = httptest.NewRequest("POST", path, nil)
					info := authoritativeRelay(user, token, "text-dispatch-request")
					info.ChannelMeta = &relaycommon.ChannelMeta{}
					info.StartTime = time.Now()
					ctx.Set(common.RequestIdKey, info.RequestId)
					session, apiErr := NewBillingSession(ctx, info, 100)
					require.Nil(t, apiErr)
					info.Billing = session
					request, err := http.NewRequest("POST", "https://example.invalid/synthetic", strings.NewReader(`{"model":"synthetic"}`))
					require.NoError(t, err)
					if tc.sent {
						require.NoError(t, PrepareTextUsageDispatch(ctx, request, info))
						assert.Nil(t, request.GetBody)
						if tc.status > 0 {
							ObserveTextUsageDispatchResponse(info, tc.status)
						}
					}
					if tc.settle {
						require.NoError(t, session.Settle(50))
					}
					if tc.held {
						assert.False(t, session.NeedsRefund())
						require.ErrorIs(t, session.Refund(ctx), model.ErrAccountQuotaUsageUnresolved)
					}
					assert.Equal(t, tc.held, FinalizeTextUsageDispatch(ctx, info))
					assert.Equal(t, tc.held, FinalizeTextUsageDispatch(ctx, info))
					used := 0
					if tc.held {
						used = 100
						require.ErrorIs(t, PrepareTextUsageDispatch(ctx, request, info), model.ErrAccountQuotaUsageUnresolved)
						require.ErrorIs(t, session.Refund(ctx), model.ErrAccountQuotaUsageUnresolved)
						view, err := model.GetUsageReview(context.Background(), db, user.Id, info.RequestId)
						require.NoError(t, err)
						assert.Equal(t, "usage_unknown", view.State)
						assert.Nil(t, view.ActualQuota)
						assert.EqualValues(t, 100, view.ReservedQuota)
						if tc.name == "unreadable-success" {
							root := model.User{Username: "dispatch-root", AffCode: "dispatch-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
							require.NoError(t, db.Create(&root).Error)
							for range 2 {
								review, err := model.ReconcileUsageReview(context.Background(), db, root.Id, info.RequestId, 20, "verified synthetic dispatch proof")
								require.NoError(t, err)
								require.NoError(t, model.ProjectUsageReviewDecision(context.Background(), db, db, review.Decision.ID))
							}
							used = 20
						}
					} else if tc.settle {
						used = 50
					} else {
						require.NoError(t, session.Refund(ctx))
						require.NoError(t, session.Refund(ctx))
					}
					uq, remaining, consumed := loadPostConsumeBalances(t, db, user, token)
					if noBalance {
						assert.Zero(t, uq)
					} else {
						assert.Equal(t, 1000-used, uq)
					}
					assert.Equal(t, 1000-used, remaining)
					assert.Equal(t, used, consumed)
				})
			}
		}
	}
}

func TestTextDispatchCancelledBeforeSendCanRefund(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
	user, token := seedAuthoritativeBilling(t, db, "cancel-dispatch", 1000, 1000, false)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	info := authoritativeRelay(user, token, "cancel-dispatch-request")
	session, apiErr := NewBillingSession(ctx, info, 100)
	require.Nil(t, apiErr)
	info.Billing = session
	cancelled, cancel := context.WithCancel(ctx.Request.Context())
	cancel()
	ctx.Request = ctx.Request.WithContext(cancelled)
	request, err := http.NewRequest("POST", "https://example.invalid/synthetic", strings.NewReader("synthetic"))
	require.NoError(t, err)
	require.ErrorIs(t, PrepareTextUsageDispatch(ctx, request, info), context.Canceled)
	assert.False(t, FinalizeTextUsageDispatch(ctx, info))
	require.NoError(t, session.Refund(ctx))
	uq, remaining, used := loadPostConsumeBalances(t, db, user, token)
	assert.Equal(t, 1000, uq)
	assert.Equal(t, 1000, remaining)
	assert.Zero(t, used)
}

func TestTextDispatchHoldWriteFailureStaysProtectedAndRetries(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			user, token := seedAuthoritativeBilling(t, db, "hold-failure", 1000, 1000, false)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			info := authoritativeRelay(user, token, "dispatch-hold-failure")
			info.ChannelMeta = &relaycommon.ChannelMeta{}
			info.StartTime = time.Now()
			ctx.Set(common.RequestIdKey, info.RequestId)
			session, apiErr := NewBillingSession(ctx, info, 100)
			require.Nil(t, apiErr)
			info.Billing = session
			request, err := http.NewRequest("POST", "https://example.invalid/synthetic", strings.NewReader("synthetic"))
			require.NoError(t, err)
			require.NoError(t, PrepareTextUsageDispatch(ctx, request, info))
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("synthetic-hold-write-failure", func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && (tx.Statement.Schema.Table == "legacy_usage_reservations" || tx.Statement.Schema.Table == "account_quota_terminal_recovery_obligations") {
					tx.AddError(model.ErrAccountQuotaMutationCASLost)
				}
			}))
			assert.True(t, FinalizeTextUsageDispatch(ctx, info))
			assert.False(t, session.usageHoldPersisted)
			require.ErrorIs(t, session.Refund(ctx), model.ErrAccountQuotaUsageUnresolved)
			require.NoError(t, db.Callback().Update().Remove("synthetic-hold-write-failure"))
			assert.True(t, FinalizeTextUsageDispatch(ctx, info))
			assert.True(t, session.usageHoldPersisted)
			assert.True(t, FinalizeTextUsageDispatch(ctx, info))
			view, err := model.GetUsageReview(context.Background(), db, user.Id, info.RequestId)
			require.NoError(t, err)
			assert.Equal(t, "usage_unknown", view.State)
			assert.Nil(t, view.ActualQuota)
			uq, remaining, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 900, uq)
			assert.Equal(t, 900, remaining)
			assert.Equal(t, 100, used)
		})
	}
}

func TestTextDispatchSurvivesLossOfLiveSession(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			user, token := seedAuthoritativeBilling(t, db, "dispatch-reopen", 1000, 1000, false)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			info := authoritativeRelay(user, token, "dispatch-reopen")
			info.ChannelMeta = &relaycommon.ChannelMeta{}
			session, apiErr := NewBillingSession(ctx, info, 100)
			require.Nil(t, apiErr)
			info.Billing = session
			t.Cleanup(session.finishInflight)
			request, err := http.NewRequest("POST", "https://example.invalid/synthetic", strings.NewReader("synthetic"))
			require.NoError(t, err)
			require.NoError(t, PrepareTextUsageDispatch(ctx, request, info))
			var database struct{ File string }
			require.NoError(t, db.Raw("PRAGMA database_list").Scan(&database).Error)
			require.NotEmpty(t, database.File)
			reopened, err := gorm.Open(sqlite.Open(database.File), &gorm.Config{})
			require.NoError(t, err)
			pool, err := reopened.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = pool.Close() })
			// No finalizer, in-memory flags, timer, or zero-use guess participates.
			if mode == model.QuotaWriterModeLegacy {
				row, err := model.FindLegacyUsageReservation(context.Background(), reopened, info.RequestId)
				require.NoError(t, err)
				pending, err := model.TextDispatchPending(row.ReviewMetadata)
				require.NoError(t, err)
				assert.True(t, pending)
				_, err = model.EnsureAccountQuotaRefundFact(context.Background(), reopened, model.AccountQuotaRefundFactInput{RequestID: info.RequestId, EventKey: "billing-refund:dispatch-reopen:v1", Kind: model.AccountQuotaRefundFactKindLegacyWallet, UserID: user.Id, TokenID: token.Id, WalletQuota: 100, TokenQuota: 100})
				require.ErrorIs(t, err, model.ErrAccountQuotaUsageUnresolved)
			} else {
				require.NoError(t, model.InitializeAccountQuotaReservationHeadsWithDB(reopened))
				receipt, err := model.FindAccountQuotaReserveReceipt(reopened, info.RequestId)
				require.NoError(t, err)
				_, err = model.RefundAccountQuota(context.Background(), reopened, model.AccountQuotaTerminalInput{RequestID: info.RequestId, ReserveReceiptID: receipt.ID, AuditKey: "unsafe-restart-refund"})
				require.ErrorIs(t, err, model.ErrAccountQuotaUsageUnresolved)
			}
			uq, remaining, used := loadPostConsumeBalances(t, reopened, user, token)
			assert.Equal(t, 900, uq)
			assert.Equal(t, 900, remaining)
			assert.Equal(t, 100, used)
		})
	}
}
