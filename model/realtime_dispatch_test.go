package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRealtimeDispatchProtocolEvidenceIsNarrow(t *testing.T) {
	for _, tc := range []struct {
		metadata         string
		allowed, invalid bool
	}{
		{``, false, false},
		{`{"version":1,"text_dispatch_pending":true}`, false, false},
		{`{"version":1,"text_dispatch_pending":true,"dispatch_protocol":"openai_realtime"}`, true, false},
		{`{"version":1,"text_dispatch_pending":false,"dispatch_protocol":"openai_realtime"}`, false, false},
		{`{"version":2,"text_dispatch_pending":true,"dispatch_protocol":"openai_realtime"}`, false, true},
		{`{"version":1,"text_dispatch_pending":true,"dispatch_protocol":"other"}`, false, true},
		{`{"version":1,"text_dispatch_pending":true,"dispatch_protocol":42}`, false, true},
		{`{"version":1,"text_dispatch_pending":true,"strict_token_budget":true,"dispatch_protocol":"openai_realtime"}`, false, true},
		{`{"version":1,"text_dispatch_pending":true,"strict_token_budget":null,"dispatch_protocol":"openai_realtime"}`, false, true},
		{`{"version":1,"text_dispatch_pending":true,"strict_token_budget":false,"dispatch_protocol":"openai_realtime"}`, true, false},
	} {
		got, err := RealtimeDispatchPending(tc.metadata)
		assert.Equal(t, tc.allowed, got)
		assert.Equal(t, tc.invalid, err != nil)
	}
}

func verifyRealtimeDispatchExtension(t *testing.T, db *gorm.DB, prefix string) {
	t.Helper()
	ctx := context.Background()
	pricing := `{"version":1,"quota_unit":100,"quota_unit_captured":true,"model_ratio":1,"completion_ratio":1,"group_ratio":1}`
	for index, mode := range []QuotaWriterMode{QuotaWriterModeLegacy, QuotaWriterModeAuthoritative} {
		t.Run("realtime-dispatch-"+string(mode), func(t *testing.T) {
			state, err := GetQuotaWriterEpochState(db)
			require.NoError(t, err)
			setQuotaWriterStateForTest(t, db, mode, state.Epoch+1)
			id := fmt.Sprintf("%s%d", prefix, index)
			user := User{Username: id, AffCode: id, Status: common.UserStatusEnabled, Quota: 1000}
			require.NoError(t, db.Create(&user).Error)
			token := Token{UserId: user.Id, Key: id, Status: common.TokenStatusEnabled, RemainQuota: 1000, ExpiredTime: -1}
			require.NoError(t, db.Create(&token).Error)
			var receiptID int64
			if mode == QuotaWriterModeLegacy {
				require.NoError(t, db.Model(&user).Update("quota", 900).Error)
				require.NoError(t, db.Model(&token).Updates(map[string]any{"remain_quota": 900, "used_quota": 100}).Error)
				_, err := PrepareLegacyUsageReservation(ctx, db, LegacyUsageReservation{RequestID: id, UserID: user.Id, TokenID: token.Id, FundingSource: "wallet", ReservedQuota: 100, TokenReservedQuota: 100})
				require.NoError(t, err)
			} else {
				receipt, err := ReserveAccountQuota(ctx, db, AccountQuotaReserveInput{RequestID: id, UserID: user.Id, TokenID: token.Id, RequestedQuota: 100, BillingPreference: "wallet_only", BillingContext: AccountBillingContext{Version: 1, OriginModelName: "fixture", BillingPreference: "wallet_only"}})
				require.NoError(t, err)
				receiptID = receipt.ID
			}
			require.NoError(t, SetRealtimeDispatchEvidence(ctx, db, id, user.Id, token.Id, receiptID, pricing))
			require.ErrorIs(t, SetTextDispatchEvidence(ctx, db, id, user.Id, token.Id, receiptID, "", false), ErrAccountQuotaUsageUnresolved)
			for range 2 {
				if mode == QuotaWriterModeLegacy {
					_, err = ExtendLegacyUsageReservation(ctx, db, id, user.Id, token.Id, 200, true)
				} else {
					var receipt *AccountQuotaMutationReceipt
					receipt, err = ExtendAccountQuotaReservation(ctx, db, receiptID, 200)
					if err == nil {
						receiptID = receipt.ID
					}
				}
				require.NoError(t, err)
			}
			var metadata string
			if mode == QuotaWriterModeLegacy {
				row, err := FindLegacyUsageReservation(ctx, db, id)
				require.NoError(t, err)
				metadata = row.ReviewMetadata
				assert.EqualValues(t, 200, row.ReservedQuota)
				require.ErrorIs(t, validateLegacyUsageRefund(db, id), ErrAccountQuotaUsageUnresolved)
			} else {
				var row AccountQuotaTerminalRecoveryObligation
				require.NoError(t, db.Where("request_id = ?", id).First(&row).Error)
				metadata = row.ReviewMetadata
				assert.Equal(t, receiptID, row.ReserveReceiptID)
				_, err = RefundAccountQuota(ctx, db, AccountQuotaTerminalInput{RequestID: id, ReserveReceiptID: receiptID, AuditKey: "unsafe-realtime-refund"})
				require.ErrorIs(t, err, ErrAccountQuotaUsageUnresolved)
			}
			pending, err := RealtimeDispatchPending(metadata)
			require.NoError(t, err)
			assert.True(t, pending)
			require.NoError(t, db.First(&user, user.Id).Error)
			require.NoError(t, db.First(&token, token.Id).Error)
			assert.Equal(t, 800, user.Quota)
			assert.Equal(t, 800, token.RemainQuota)
			assert.Equal(t, 200, token.UsedQuota)
		})
	}
}

func TestRealtimeDispatchExtensionsKeepDurableProtection(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	require.NoError(t, db.AutoMigrate(&AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}))
	verifyRealtimeDispatchExtension(t, db, "rt-ext-")
}
