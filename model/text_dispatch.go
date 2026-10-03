package model

import (
	"context"
	"encoding/json"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

// TextDispatchPending is durable uncertainty, not evidence of consumption.
// Malformed evidence fails closed; old empty metadata remains compatible.
func TextDispatchPending(metadata string) (bool, error) {
	if metadata == "" {
		return false, nil
	}
	var fields map[string]json.RawMessage
	if len(metadata) > 16384 || common.UnmarshalJsonStr(metadata, &fields) != nil || fields == nil {
		return false, ErrAccountQuotaUsageUnresolved
	}
	raw, present := fields["text_dispatch_pending"]
	if !present {
		return false, nil
	}
	var pending *bool
	if common.Unmarshal(raw, &pending) != nil || pending == nil {
		return false, ErrAccountQuotaUsageUnresolved
	}
	return *pending, nil
}

// Only the server-recorded Realtime protocol permits segmented reservations
// while dispatch remains pending. HTTP and malformed evidence stay blocked.
func RealtimeDispatchPending(metadata string) (bool, error) {
	if pending, err := TextDispatchPending(metadata); err != nil || !pending {
		return false, err
	}
	var evidence struct {
		Version  int             `json:"version"`
		Strict   json.RawMessage `json:"strict_token_budget"`
		Protocol *string         `json:"dispatch_protocol"`
	}
	if common.UnmarshalJsonStr(metadata, &evidence) != nil {
		return false, ErrAccountQuotaUsageUnresolved
	}
	if evidence.Protocol == nil {
		return false, nil
	}
	if evidence.Version != 1 || *evidence.Protocol != "openai_realtime" {
		return false, ErrAccountQuotaUsageUnresolved
	}
	if len(evidence.Strict) > 0 {
		var strict *bool
		if common.Unmarshal(evidence.Strict, &strict) != nil || strict == nil || *strict {
			return false, ErrAccountQuotaUsageUnresolved
		}
	}
	return true, nil
}

// SetTextDispatchEvidence records an attempt before the network call, or clears
// it only after a known raw refusal. It changes no balance or actual usage.
// Existing writer rows and locks serialize it against terminal mutations.
func SetTextDispatchEvidence(ctx context.Context, db *gorm.DB, requestID string, userID, tokenID int, reserveID int64, pricing string, pending bool) error {
	return setUsageDispatchEvidence(ctx, db, requestID, userID, tokenID, reserveID, pricing, pending, false)
}

// Realtime never clears its pending marker to extend a reservation. Only a
// confirmed settlement or the existing authorized review closes its lifecycle.
func SetRealtimeDispatchEvidence(ctx context.Context, db *gorm.DB, requestID string, userID, tokenID int, reserveID int64, pricing string) error {
	return setUsageDispatchEvidence(ctx, db, requestID, userID, tokenID, reserveID, pricing, true, true)
}

func setUsageDispatchEvidence(ctx context.Context, db *gorm.DB, requestID string, userID, tokenID int, reserveID int64, pricing string, pending, realtime bool) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	id, err := normalizeAccountRequestID(requestID)
	if err != nil || id != requestID || userID <= 0 || tokenID < 0 || reserveID < 0 {
		return ErrAccountQuotaMutationInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var evidence map[string]json.RawMessage
	if pending {
		if len(pricing) == 0 || len(pricing) > 16384 || common.UnmarshalJsonStr(pricing, &evidence) != nil || evidence == nil {
			return ErrAccountQuotaMutationInvalidInput
		}
		var flags struct {
			Version int  `json:"version"`
			Strict  bool `json:"strict_token_budget"`
			Pending bool `json:"text_dispatch_pending"`
		}
		if common.UnmarshalJsonStr(pricing, &flags) != nil || flags.Version != 1 || flags.Strict || flags.Pending {
			return ErrAccountQuotaMutationInvalidInput
		}
		if _, present := evidence["dispatch_protocol"]; present {
			return ErrAccountQuotaMutationInvalidInput
		}
		if realtime {
			evidence["dispatch_protocol"] = json.RawMessage(`"openai_realtime"`)
		}
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var entity any
		var rowID, version int64
		var old, state string
		if reserveID > 0 {
			var head AccountQuotaReservationHead
			if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&head).Error; err != nil {
				return err
			}
			if head.RequestID != requestID || head.UserID != userID || head.TokenID != tokenID || head.TerminalReceiptID != 0 || head.CurrentReceiptID != reserveID {
				return ErrAccountQuotaMutationConflict
			}
			var receipt AccountQuotaMutationReceipt
			if err := tx.First(&receipt, reserveID).Error; err != nil {
				return err
			}
			if err := validateAccountQuotaHeadReceipt(&head, &receipt); err != nil {
				return err
			}
			var row AccountQuotaTerminalRecoveryObligation
			if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&row).Error; err != nil {
				return err
			}
			if row.State != AccountQuotaTerminalRecoveryOpen || row.WriterEpoch != head.WriterEpoch || row.ReserveReceiptID != reserveID || row.RequestFingerprint != head.ReserveFingerprint {
				return ErrAccountQuotaUsageUnresolved
			}
			entity, rowID, version, old, state = &AccountQuotaTerminalRecoveryObligation{}, row.ID, row.LockVersion, row.ReviewMetadata, row.State
		} else {
			var row LegacyUsageReservation
			if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&row).Error; err != nil {
				return err
			}
			if row.RequestID != requestID || row.UserID != userID || row.TokenID != tokenID {
				return ErrAccountQuotaMutationConflict
			}
			if row.State != LegacyUsagePrepared {
				return ErrAccountQuotaUsageUnresolved
			}
			entity, rowID, version, old, state = &LegacyUsageReservation{}, row.ID, row.LockVersion, row.ReviewMetadata, row.State
		}
		if version <= 0 {
			return ErrAccountQuotaMutationConflict
		}
		wasPending, err := TextDispatchPending(old)
		if err != nil {
			return err
		}
		if pending && wasPending {
			return ErrAccountQuotaUsageUnresolved
		}
		if !pending {
			if realtimePending, err := RealtimeDispatchPending(old); err != nil || realtimePending {
				return ErrAccountQuotaUsageUnresolved
			}
			if !wasPending {
				return nil
			}
			if common.UnmarshalJsonStr(old, &evidence) != nil || evidence == nil {
				return ErrAccountQuotaUsageUnresolved
			}
		}
		if pending {
			evidence["text_dispatch_pending"] = json.RawMessage("true")
		} else {
			evidence["text_dispatch_pending"] = json.RawMessage("false")
		}
		encoded, err := common.Marshal(evidence)
		if err != nil || len(encoded) > 16384 {
			return ErrAccountQuotaMutationInvalidInput
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		result := tx.Model(entity).Where("id = ? AND state = ? AND lock_version = ?", rowID, state, version).Updates(map[string]any{"review_metadata": string(encoded), "lock_version": version + 1, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountQuotaMutationCASLost
		}
		return nil
	})
}
