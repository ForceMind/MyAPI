package model

import (
	"context"
	"fmt"
	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextDispatchDurableEvidenceBlocksRefundAfterSessionLoss(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "recovery"}[recovery], func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			f := newAccountQuotaFixture(t, db, "dispatch-durable", 1000, 1000, false, 0, 0, false)
			reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(f, "dispatch-durable", "wallet_only", 100))
			require.NoError(t, err)
			// A persisted pre-send marker must protect the reservation without a live session.
			require.NoError(t, db.Model(&AccountQuotaTerminalRecoveryObligation{}).Where("request_id = ?", reserve.RequestID).Update("review_metadata", `{"version":1,"text_dispatch_pending":true}`).Error)
			input := AccountQuotaTerminalInput{RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "must-not-refund"}
			if recovery {
				_, _, err = EnsureAccountQuotaRefundRecovery(context.Background(), db, input, nil)
			} else {
				_, err = RefundAccountQuota(context.Background(), db, input)
			}
			require.ErrorIs(t, err, ErrAccountQuotaUsageUnresolved)
			user, token, _ := loadAccountBalances(t, db, f)
			assert.Equal(t, 900, user.Quota)
			assert.Equal(t, 900, token.RemainQuota)
		})
	}
}

func TestTextDispatchDurableTransitions(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	require.NoError(t, db.AutoMigrate(&LegacyUsageReservation{}))
	verifyTextDispatchTransitions(t, db, "td-local")
}

// Shared by the existing disposable SQLite/MySQL/PostgreSQL contract.
func verifyTextDispatchTransitions(t *testing.T, db *gorm.DB, namespace string) {
	t.Helper()
	ctx := context.Background()
	for _, fixedPrice := range []bool{false, true} {
		pricing := `{"version":1,"model":"fixture","quota_unit":500000,"quota_unit_captured":true,"model_ratio":1,"completion_ratio":1,"group_ratio":1}`
		if fixedPrice {
			pricing = `{"version":1,"model":"fixture","upstream_model":"actual-fixture","quota_unit":500000,"quota_unit_captured":true,"fixed_price":true,"model_price":0.01,"price_ratios":{"quality":2},"group_ratio":1}`
		}
		for _, legacy := range []bool{false, true} {
			suffix := "auth"
			if legacy {
				suffix = "legacy"
			}
			if fixedPrice {
				suffix += "-fixed"
			}
			t.Run("text-dispatch-"+suffix, func(t *testing.T) {
				name := fmt.Sprintf("td%x", time.Now().UnixNano())
				user := User{Username: name, AffCode: name, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000, AuthVersion: 1}
				require.NoError(t, db.Create(&user).Error)
				token := Token{UserId: user.Id, Key: name, Status: common.TokenStatusEnabled, RemainQuota: 1000, ExpiredTime: -1}
				require.NoError(t, db.Create(&token).Error)
				requestID := namespace + "-" + suffix
				var receiptID int64
				if legacy {
					_, err := PrepareLegacyUsageReservation(ctx, db, LegacyUsageReservation{RequestID: requestID, UserID: user.Id, TokenID: token.Id, FundingSource: "wallet", ReservedQuota: 100, TokenReservedQuota: 100})
					require.NoError(t, err)
				} else {
					receipt, err := ReserveAccountQuota(ctx, db, AccountQuotaReserveInput{RequestID: requestID, UserID: user.Id, TokenID: token.Id, RequestedQuota: 100, BillingPreference: "wallet_only", BillingContext: AccountBillingContext{Version: 1, OriginModelName: "fixture", BillingPreference: "wallet_only"}})
					require.NoError(t, err)
					receiptID = receipt.ID
				}
				require.Error(t, SetTextDispatchEvidence(ctx, db, requestID, user.Id+100000, token.Id, receiptID, pricing, true))
				require.NoError(t, SetTextDispatchEvidence(ctx, db, requestID, user.Id, token.Id, receiptID, pricing, true))
				require.ErrorIs(t, SetTextDispatchEvidence(ctx, db, requestID, user.Id, token.Id, receiptID, pricing, true), ErrAccountQuotaUsageUnresolved)
				if legacy {
					require.ErrorIs(t, validateLegacyUsageRefund(db, requestID), ErrAccountQuotaUsageUnresolved)
				} else {
					_, err := ExtendAccountQuotaReservation(ctx, db, receiptID, 150)
					require.ErrorIs(t, err, ErrAccountQuotaUsageUnresolved)
					_, _, err = EnsureAccountQuotaRefundRecovery(ctx, db, AccountQuotaTerminalInput{RequestID: requestID, ReserveReceiptID: receiptID, AuditKey: "unsafe-refund"}, nil)
					require.ErrorIs(t, err, ErrAccountQuotaUsageUnresolved)
				}
				require.NoError(t, SetTextDispatchEvidence(ctx, db, requestID, user.Id, token.Id, receiptID, "", false))
				if legacy {
					require.NoError(t, validateLegacyUsageRefund(db, requestID))
				}
				// A known refusal permits a fresh attempt, not a replay of uncertainty.
				require.NoError(t, SetTextDispatchEvidence(ctx, db, requestID, user.Id, token.Id, receiptID, pricing, true))
				if legacy {
					require.NoError(t, UpdateLegacyUsageReservation(ctx, db, requestID, 100, 100, LegacyUsageUnknown, "missing", nil, pricing))
				} else {
					_, err := HoldAccountQuotaUnknownUsage(ctx, db, requestID, receiptID, "missing", pricing)
					require.NoError(t, err)
				}
				if fixedPrice {
					var metadata string
					if legacy {
						var row LegacyUsageReservation
						require.NoError(t, db.Where("request_id = ?", requestID).First(&row).Error)
						metadata = row.ReviewMetadata
					} else {
						var row AccountQuotaTerminalRecoveryObligation
						require.NoError(t, db.Where("request_id = ?", requestID).First(&row).Error)
						metadata = row.ReviewMetadata
					}
					require.Contains(t, metadata, `"fixed_price":true`)
					require.Contains(t, metadata, `"upstream_model":"actual-fixture"`)
					require.Contains(t, metadata, `"quality":2`)
				}
				require.ErrorIs(t, SetTextDispatchEvidence(ctx, db, requestID, user.Id, token.Id, receiptID, "", false), ErrAccountQuotaUsageUnresolved, "a late refusal cannot reopen a held lifecycle")
			})
		}
	}
}

func TestTextDispatchCorruptMarkerCannotAuthorizeRefund(t *testing.T) {
	for _, metadata := range []string{`null`, `[]`, `{"text_dispatch_pending":null}`, `{"text_dispatch_pending":"false"}`, `{"text_dispatch_pending":0}`} {
		_, err := TextDispatchPending(metadata)
		require.ErrorIs(t, err, ErrAccountQuotaUsageUnresolved)
	}
	for _, metadata := range []string{"", `{}`, `{"version":1}`, `{"text_dispatch_pending":false}`} {
		pending, err := TextDispatchPending(metadata)
		require.NoError(t, err)
		assert.False(t, pending)
	}
}
