package service

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSelfUseSessionKeepsKeyAccountingWithoutWalletAcrossWriters(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, terminal := range []string{"settle", "cancel", "unknown"} {
			t.Run(string(mode)+"-"+terminal, func(t *testing.T) {
				db := setupPostConsumeModeDB(t, mode)
				require.NoError(t, db.AutoMigrate(&model.Option{}, &model.UserUsagePolicyChange{}))
				require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
					_, err := model.InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeDisabled)
					return err
				}))
				user, token := seedAuthoritativeBilling(t, db, "self-use", 0, 100, false)
				require.NoError(t, db.Model(user).Updates(map[string]any{"self_use_no_balance": true, "usage_policy_revision": 1}).Error)
				root := model.User{Username: "self-use-reviewer", AffCode: "self-use-reviewer", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
				require.NoError(t, db.Create(&root).Error)
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				info := authoritativeRelay(user, token, "self-use-request")
				session, apiErr := NewBillingSession(ctx, info, 60)
				require.Nil(t, apiErr)
				require.NotNil(t, session)
				assert.Equal(t, BillingSourceSelfUse, info.BillingSource)
				uq, remaining, used := loadPostConsumeBalances(t, db, user, token)
				assert.Zero(t, uq)
				assert.Equal(t, 40, remaining)
				assert.Equal(t, 60, used)
				_, limitErr := NewBillingSession(ctx, authoritativeRelay(user, token, "key-over-limit"), 101)
				require.NotNil(t, limitErr, "no-balance policy never disables the existing Key allowance")
				// A later policy switch must not reinterpret an admitted request as a
				// wallet debit or invent a refund. New requests observe the new policy.
				_, err := model.ConfigureUserUsagePolicy(context.Background(), db, root.Id, model.UserUsagePolicyInput{ID: strings.Repeat("a", 64), UserID: user.Id, ExpectedRevision: 1, NoBalance: false})
				require.NoError(t, err)
				expected := 50
				switch terminal {
				case "settle":
					require.NoError(t, session.Reserve(70))
					require.NoError(t, session.Settle(50))
					require.NoError(t, session.Settle(50))
					assert.EqualValues(t, 50, session.settlementAuditInfo()["actual_quota"], "audit must show the actual internal usage, not an empty wallet charge")
				case "cancel":
					expected = 0
					require.NoError(t, session.Refund(ctx))
					require.NoError(t, session.Refund(ctx))
				case "unknown":
					require.NoError(t, session.HoldUnknownUsage(context.Background(), "missing"))
					require.ErrorIs(t, session.Settle(50), model.ErrAccountQuotaUsageUnresolved)
					require.ErrorIs(t, session.Refund(ctx), model.ErrAccountQuotaUsageUnresolved)
					uq, remaining, used = loadPostConsumeBalances(t, db, user, token)
					assert.Zero(t, uq)
					assert.Equal(t, 40, remaining)
					assert.Equal(t, 60, used)
					for range 2 {
						if mode == model.QuotaWriterModeLegacy {
							_, err = model.ResolveLegacyUnknownUsage(context.Background(), db, root.Id, info.RequestId, 50, strings.Repeat("b", 64))
						} else {
							_, err = model.ResolveAccountQuotaUnknownUsage(context.Background(), db, root.Id, model.AccountQuotaTerminalInput{RequestID: info.RequestId, ReserveReceiptID: session.reserveReceipt.ID, ActualQuota: 50}, strings.Repeat("b", 64))
						}
						require.NoError(t, err)
					}
				}
				uq, remaining, used = loadPostConsumeBalances(t, db, user, token)
				assert.Zero(t, uq)
				assert.Equal(t, 100-expected, remaining)
				assert.Equal(t, expected, used)
				_, apiErr = NewBillingSession(ctx, authoritativeRelay(user, token, "new-limited-request"), 1)
				require.NotNil(t, apiErr, "restored zero user allowance applies to new requests")
			})
		}
	}
}

func TestSelfUseStoredPolicyCannotWaiveEnabledCommercialWallet(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			require.NoError(t, db.AutoMigrate(&model.Option{}))
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				_, err := model.InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeEnabled)
				return err
			}))
			user, token := seedAuthoritativeBilling(t, db, "self-use-enabled", 0, 100, false)
			require.NoError(t, db.Model(user).Updates(map[string]any{"self_use_no_balance": true, "usage_policy_revision": 1}).Error)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			_, apiErr := NewBillingSession(ctx, authoritativeRelay(user, token, "enabled-no-wallet"), 60)
			require.NotNil(t, apiErr)
			require.NoError(t, db.Model(user).Update("quota", 100).Error)
			session, apiErr := NewBillingSession(ctx, authoritativeRelay(user, token, "enabled-wallet"), 60)
			require.Nil(t, apiErr)
			assert.Equal(t, BillingSourceWallet, session.funding.Source())
			require.NoError(t, session.Settle(50))
			uq, remaining, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 50, uq)
			assert.Equal(t, 50, remaining)
			assert.Equal(t, 50, used)
		})
	}
}

func TestSelfUseSessionRejectsUnsupportedPathAfterFrozenAdmission(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			require.NoError(t, db.AutoMigrate(&model.Option{}, &model.UserUsagePolicyChange{}))
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				_, err := model.InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeDisabled)
				return err
			}))
			user, token := seedAuthoritativeBilling(t, db, "self-use-path", 0, 100, false)
			require.NoError(t, db.Model(user).Updates(map[string]any{"self_use_no_balance": true, "usage_policy_revision": 1}).Error)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
			session, apiErr := NewBillingSession(ctx, authoritativeRelay(user, token, "self-use-unsupported-path"), 60)
			require.Nil(t, session)
			require.NotNil(t, apiErr, "a policy enabled after middleware must still be checked against the captured source before dispatch")
			uq, remaining, used := loadPostConsumeBalances(t, db, user, token)
			assert.Zero(t, uq)
			assert.Equal(t, 100, remaining)
			assert.Zero(t, used)
		})
	}
}
