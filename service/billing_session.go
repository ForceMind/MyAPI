package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/types"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// BillingSession — 统一计费会话
// ---------------------------------------------------------------------------

// BillingSession 封装单次请求的预扣费/结算/退款生命周期。
// 实现 relaycommon.BillingSettler 接口。
type BillingSession struct {
	relayInfo                  *relaycommon.RelayInfo
	funding                    FundingSource
	preConsumedQuota           int  // 实际预扣额度（信任用户可能为 0）
	tokenConsumed              int  // 令牌额度实际扣减量
	extraReserved              int  // 发送前补充预扣的额度（订阅退款时需要单独回滚）
	trusted                    bool // 是否命中信任额度旁路
	fundingSettled             bool // funding.Settle 已成功，资金来源已提交
	settled                    bool // Settle 全部完成（资金 + 令牌）
	refunded                   bool // Refund 已调用
	refundRecoveryScheduled    bool
	refundIntentErr            error
	settlementPending          bool
	settlementIntentReady      bool
	settlementInput            *model.AccountQuotaSettlementFactInput
	settlementInputFingerprint string
	settlementIntent           *model.AccountQuotaSettlementIntent
	settlementFact             *model.AccountQuotaSettlementFact
	settlementManualEvidence   *model.AccountQuotaTerminalRecoveryObligation
	settledActualQuota         int
	settledActualQuotaSet      bool
	writerMode                 model.QuotaWriterMode
	reserveReceipt             *model.AccountQuotaMutationReceipt
	terminalReceipt            *model.AccountQuotaMutationReceipt
	inflightOnce               sync.Once
	inflightTracked            bool
	mu                         sync.Mutex
}

const authoritativeRefundTimeout = 2 * time.Second

var (
	authoritativeRefundAccountQuota  = model.RefundAccountQuota
	ensureAccountQuotaRefundRecovery = model.EnsureAccountQuotaRefundRecovery
	legacyTokenQuotaRefund           = model.IncreaseTokenQuota
	findAccountQuotaTerminalReceipt  = model.FindAccountQuotaTerminalReceipt
)

func billingOperationContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	base := context.Background()
	if parent != nil {
		base = context.WithoutCancel(parent)
	}
	return context.WithTimeout(base, timeout)
}

func enqueueAccountQuotaSettlementRecovery(parent context.Context) error {
	ctx, cancel := billingOperationContext(parent, 2*time.Second)
	defer cancel()
	_, _, err := EnqueueSystemTaskContext(ctx, model.SystemTaskTypeAccountQuotaRefundRecovery, accountQuotaRefundRecoveryHandler{}.NewPayload())
	return err
}

// finishInflight decrements the process-local quota-writer in-flight counter
// exactly once per created session. It is deferred in both Settle and Refund
// so every exit path — success, failure, or early idempotent return — closes
// the counter symmetrically.
func (s *BillingSession) finishInflight() {
	if !s.inflightTracked {
		return
	}
	s.inflightOnce.Do(model.TrackQuotaWriterInflightFinish)
}

// Settle 根据实际消耗额度进行结算。结算意图先持久化；若同步完成失败，
// 后台恢复使用同一事实身份重放，不再改写已确认的 actualQuota。
func (s *BillingSession) Settle(actualQuota int) error {
	return s.SettleWithContext(context.Background(), actualQuota)
}

// SettleWithContext preserves context values but detaches request cancellation.
// Once upstream usage is known, settlement is an accounting obligation that
// must be persisted even if the client disconnects.
func (s *BillingSession) SettleWithContext(parent context.Context, actualQuota int) error {
	defer s.finishInflight()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled {
		if s.settledActualQuotaSet && s.settledActualQuota == actualQuota {
			return nil
		}
		return model.ErrAccountQuotaMutationConflict
	}
	ctx, cancel := billingOperationContext(parent, 5*time.Second)
	defer cancel()
	requestID := strings.TrimSpace(s.relayInfo.RequestId)
	if requestID == "" && s.reserveReceipt != nil {
		requestID = s.reserveReceipt.RequestID
	}
	if s.writerMode == model.QuotaWriterModeAuthoritative || s.writerMode == model.QuotaWriterModeBridge {
		if s.refunded || s.refundRecoveryScheduled || s.reserveReceipt == nil {
			return model.ErrAccountQuotaMutationTerminal
		}
		if requestID == "" {
			s.settlementPending = true
			return errors.Join(model.ErrAccountQuotaSettlementManualRequired, model.ErrAccountQuotaMutationInvalidInput)
		}
		factInput := model.AccountQuotaSettlementFactInput{
			EventKey: "billing-settlement:" + requestID + ":v2", RequestID: requestID,
			Kind: model.AccountQuotaSettlementKindAuthoritative, UserID: s.reserveReceipt.UserID,
			TokenID: s.reserveReceipt.TokenID, SubscriptionID: s.reserveReceipt.SubscriptionID,
			ReserveReceiptID: s.reserveReceipt.ID, WriterEpoch: s.reserveReceipt.WriterEpoch, ActualQuota: int64(actualQuota),
		}
		return s.settleWithFact(ctx, requestID, factInput)
	}
	delta := actualQuota - s.preConsumedQuota
	if delta == 0 {
		s.settled = true
		s.settledActualQuota = actualQuota
		s.settledActualQuotaSet = true
		return nil
	}
	if requestID == "" {
		s.settlementPending = true
		return errors.Join(model.ErrAccountQuotaSettlementManualRequired, model.ErrAccountQuotaMutationInvalidInput)
	}
	factInput := model.AccountQuotaSettlementFactInput{
		EventKey: "billing-settlement:" + requestID + ":v1", RequestID: requestID,
		UserID: s.relayInfo.UserId, TokenID: s.relayInfo.TokenId, Delta: int64(delta), ApplyToken: !s.relayInfo.IsPlayground,
	}
	switch funding := s.funding.(type) {
	case *WalletFunding:
		factInput.Kind = model.AccountQuotaSettlementKindLegacyWallet
	case *SubscriptionFunding:
		factInput.Kind = model.AccountQuotaSettlementKindLegacySubscription
		factInput.SubscriptionID = funding.subscriptionId
	default:
		return fmt.Errorf("unsupported legacy settlement funding source: %s", s.funding.Source())
	}
	return s.settleWithFact(ctx, requestID, factInput)
}

func (s *BillingSession) settleWithFact(ctx context.Context, requestID string, factInput model.AccountQuotaSettlementFactInput) error {
	normalizedInput, inputFingerprint, err := model.NormalizeAccountQuotaSettlementFactInput(factInput)
	if err != nil {
		return err
	}
	if s.settlementInput != nil {
		if s.settlementInputFingerprint == "" {
			_, storedFingerprint, fingerprintErr := model.NormalizeAccountQuotaSettlementFactInput(*s.settlementInput)
			if fingerprintErr != nil {
				return errors.Join(model.ErrAccountQuotaMutationConflict, fingerprintErr)
			}
			s.settlementInputFingerprint = storedFingerprint
		}
		if s.settlementInputFingerprint != inputFingerprint {
			return model.ErrAccountQuotaMutationConflict
		}
	}
	factInput = normalizedInput
	s.settlementPending = true
	if !s.settlementIntentReady {
		return s.settleLegacyFactWithoutIntent(ctx, requestID, factInput)
	}
	intent, err := model.EnsureAccountQuotaSettlementIntent(ctx, model.DB, factInput)
	if err != nil {
		if errors.Is(err, model.ErrAccountQuotaTerminalRecoveryConflict) || errors.Is(err, model.ErrAccountQuotaMutationInvalidInput) {
			return err
		}
		fact, factErr := model.EnsureAccountQuotaSettlementFact(ctx, model.DB, factInput)
		if factErr == nil {
			s.settlementInput = &factInput
			s.settlementInputFingerprint = inputFingerprint
			s.settlementFact = fact
			return s.settleAuthoritativeEmergencyFact(ctx, requestID, fact)
		}
		evidence, evidenceErr := model.PersistAccountQuotaSettlementManualEvidence(ctx, model.DB, model.AccountQuotaTerminalInput{
			RequestID: requestID, ReserveReceiptID: factInput.ReserveReceiptID, ActualQuota: factInput.ActualQuota,
		}, errors.Join(err, factErr))
		if evidenceErr == nil {
			s.settlementInput = &factInput
			s.settlementInputFingerprint = inputFingerprint
			s.settlementManualEvidence = evidence
			return errors.Join(model.ErrAccountQuotaSettlementManualRequired, err, factErr)
		}
		return errors.Join(err, factErr, evidenceErr)
	}
	s.settlementInput = &factInput
	s.settlementInputFingerprint = inputFingerprint
	s.settlementIntent = intent
	for attempt := 0; attempt < 3; attempt++ {
		storedIntent, storedFact, recoverErr := model.RecoverAccountQuotaSettlementIntent(ctx, model.DB, intent, "billing-session:"+requestID)
		if storedIntent != nil {
			s.settlementIntent = storedIntent
			intent = storedIntent
			if storedIntent.State == model.AccountQuotaSettlementManual {
				return errors.Join(model.ErrAccountQuotaSettlementManualRequired, recoverErr)
			}
		}
		if storedFact != nil {
			s.settlementFact = storedFact
			s.fundingSettled = storedFact.FundingApplied
			if storedFact.State == model.AccountQuotaSettlementApplied {
				if storedFact.Kind == model.AccountQuotaSettlementKindAuthoritative {
					terminal, terminalErr := findAccountQuotaTerminalReceipt(model.DB.WithContext(ctx), requestID)
					if terminalErr != nil || terminal == nil || terminal.ID != storedFact.TerminalReceiptID {
						err = errors.Join(model.ErrAccountQuotaSettlementPending, terminalErr)
					} else {
						s.terminalReceipt = terminal
						if terminal.BillingSource == BillingSourceSubscription {
							s.relayInfo.SubscriptionPostDelta += terminal.AppliedDeltas.SubscriptionAmountUsed
						}
						s.settled = true
						s.settledActualQuota = int(terminal.RequestedQuota)
						s.settledActualQuotaSet = true
						s.settlementPending = false
						return nil
					}
				} else {
					s.settled = true
					s.settledActualQuota = s.preConsumedQuota + int(storedFact.Delta)
					s.settledActualQuotaSet = true
					s.settlementPending = false
					if storedFact.Kind == model.AccountQuotaSettlementKindLegacySubscription {
						s.relayInfo.SubscriptionPostDelta += storedFact.Delta
					}
					return nil
				}
			}
			if storedFact.State == model.AccountQuotaSettlementManual {
				return errors.Join(model.ErrAccountQuotaSettlementManualRequired, recoverErr)
			}
		}
		if recoverErr != nil {
			err = recoverErr
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return errors.Join(model.ErrAccountQuotaSettlementPending, ctx.Err())
			case <-time.After(time.Duration(attempt+1) * 10 * time.Millisecond):
			}
			intent, err = model.EnsureAccountQuotaSettlementIntent(ctx, model.DB, factInput)
			if err != nil {
				break
			}
		}
	}
	enqueueErr := enqueueAccountQuotaSettlementRecovery(ctx)
	return errors.Join(model.ErrAccountQuotaSettlementPending, err, enqueueErr)
}

func (s *BillingSession) settleAuthoritativeEmergencyFact(ctx context.Context, requestID string, fact *model.AccountQuotaSettlementFact) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		stored, recoverErr := model.RecoverAccountQuotaSettlementFact(ctx, model.DB, fact, "billing-session-emergency:"+requestID)
		if stored != nil {
			s.settlementFact = stored
			fact = stored
			if stored.State == model.AccountQuotaSettlementApplied {
				terminal, terminalErr := findAccountQuotaTerminalReceipt(model.DB.WithContext(ctx), requestID)
				if terminalErr == nil && terminal != nil && terminal.ID == stored.TerminalReceiptID {
					s.terminalReceipt = terminal
					if terminal.BillingSource == BillingSourceSubscription {
						s.relayInfo.SubscriptionPostDelta += terminal.AppliedDeltas.SubscriptionAmountUsed
					}
					s.settled = true
					s.settledActualQuota = int(terminal.RequestedQuota)
					s.settledActualQuotaSet = true
					s.settlementPending = false
					return nil
				}
				lastErr = errors.Join(model.ErrAccountQuotaSettlementPending, terminalErr)
			} else if stored.State == model.AccountQuotaSettlementManual {
				return errors.Join(model.ErrAccountQuotaSettlementManualRequired, recoverErr)
			}
		}
		if recoverErr != nil {
			lastErr = recoverErr
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return errors.Join(model.ErrAccountQuotaSettlementPending, ctx.Err())
			case <-time.After(time.Duration(attempt+1) * 10 * time.Millisecond):
			}
		}
	}
	enqueueErr := enqueueAccountQuotaSettlementRecovery(ctx)
	return errors.Join(model.ErrAccountQuotaSettlementPending, lastErr, enqueueErr)
}

// settleLegacyFactWithoutIntent preserves startup compatibility for old
// legacy databases that have not yet run the additive settlement-inbox
// migration. Authoritative mode never takes this fallback.
func (s *BillingSession) settleLegacyFactWithoutIntent(ctx context.Context, requestID string, factInput model.AccountQuotaSettlementFactInput) error {
	fact, err := model.EnsureAccountQuotaSettlementFact(ctx, model.DB, factInput)
	if err != nil {
		return err
	}
	s.settlementInput = &factInput
	for attempt := 0; attempt < 3; attempt++ {
		stored, recoverErr := model.RecoverAccountQuotaSettlementFact(ctx, model.DB, fact, "billing-session:"+requestID)
		if stored != nil {
			s.settlementFact = stored
			s.fundingSettled = stored.FundingApplied
			if stored.State == model.AccountQuotaSettlementApplied {
				s.settled = true
				s.settledActualQuota = s.preConsumedQuota + int(stored.Delta)
				s.settledActualQuotaSet = true
				s.settlementPending = false
				if stored.Kind == model.AccountQuotaSettlementKindLegacySubscription {
					s.relayInfo.SubscriptionPostDelta += stored.Delta
				}
				return nil
			}
			if stored.State == model.AccountQuotaSettlementManual {
				return errors.Join(model.ErrAccountQuotaSettlementManualRequired, recoverErr)
			}
		}
		if recoverErr != nil {
			err = recoverErr
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return errors.Join(model.ErrAccountQuotaSettlementPending, ctx.Err())
			case <-time.After(time.Duration(attempt+1) * 10 * time.Millisecond):
			}
			fact, err = model.EnsureAccountQuotaSettlementFact(ctx, model.DB, factInput)
			if err != nil {
				break
			}
		}
	}
	enqueueErr := enqueueAccountQuotaSettlementRecovery(ctx)
	return errors.Join(model.ErrAccountQuotaSettlementPending, err, enqueueErr)
}

func (s *BillingSession) settlementAuditInfo() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settlementInput == nil && !s.settled {
		return nil
	}
	state := model.AccountQuotaSettlementPending
	if s.settled {
		state = model.AccountQuotaSettlementApplied
	} else if s.settlementManualEvidence != nil {
		state = model.AccountQuotaSettlementManual
	} else if s.settlementFact != nil && s.settlementFact.State == model.AccountQuotaSettlementApplied {
		state = "applied_readback_pending"
	} else if s.settlementIntent == nil && s.settlementFact != nil {
		state = s.settlementFact.State
	} else if s.settlementIntent == nil {
		state = "persistence_failed"
	} else {
		state = s.settlementIntent.State
	}
	info := map[string]interface{}{"state": state}
	if s.settlementInput != nil {
		info["event_key"] = s.settlementInput.EventKey
		actualQuota := s.settlementInput.ActualQuota
		if s.settlementInput.Kind == model.AccountQuotaSettlementKindLegacyWallet || s.settlementInput.Kind == model.AccountQuotaSettlementKindLegacySubscription {
			actualQuota = int64(s.preConsumedQuota) + s.settlementInput.Delta
		}
		info["actual_quota"] = actualQuota
		if s.relayInfo.BillingSource == BillingSourceSubscription && state != model.AccountQuotaSettlementApplied {
			info["intended_subscription_post_delta"] = actualQuota - s.relayInfo.SubscriptionPreConsumed
		}
	}
	if s.settlementIntent != nil {
		info["intent_id"] = s.settlementIntent.ID
		if s.settlementIntent.FactID > 0 {
			info["fact_id"] = s.settlementIntent.FactID
		}
		if s.settlementIntent.TerminalReceiptID > 0 {
			info["terminal_receipt_id"] = s.settlementIntent.TerminalReceiptID
		}
	}
	if s.settlementFact != nil && s.settlementFact.ID > 0 {
		info["fact_id"] = s.settlementFact.ID
	}
	if s.settlementManualEvidence != nil {
		info["manual_evidence_id"] = s.settlementManualEvidence.ID
	}
	return info
}

// Refund records a durable failure fact before applying a refund. A nil result
// proves the refund is terminally applied; ErrAccountQuotaRefundPending proves
// that recovery is durable but not complete.
func (s *BillingSession) Refund(c *gin.Context) error {
	defer s.finishInflight()
	s.mu.Lock()
	if s.settled || s.refunded || s.settlementPending || s.reserveReceipt == nil && (s.writerMode == model.QuotaWriterModeAuthoritative || s.writerMode == model.QuotaWriterModeBridge) {
		s.mu.Unlock()
		return nil
	}
	if s.refundRecoveryScheduled {
		err := s.refundIntentErr
		if err == nil {
			err = model.ErrAccountQuotaRefundPending
		}
		s.mu.Unlock()
		return errors.Join(model.ErrAccountQuotaRefundPending, err)
	}
	if !s.needsRefundLocked() {
		s.mu.Unlock()
		return nil
	}
	baseCtx := context.Background()
	if c != nil && c.Request != nil {
		baseCtx = context.WithoutCancel(c.Request.Context())
	}
	ctx, cancel := context.WithTimeout(baseCtx, authoritativeRefundTimeout)
	defer cancel()
	requestID := strings.TrimSpace(s.relayInfo.RequestId)
	if requestID == "" && s.reserveReceipt != nil {
		requestID = s.reserveReceipt.RequestID
	}
	if requestID == "" {
		err := model.ErrAccountQuotaMutationInvalidInput
		s.refundIntentErr = err
		s.mu.Unlock()
		return err
	}

	var factInput model.AccountQuotaRefundFactInput
	if s.writerMode == model.QuotaWriterModeAuthoritative || s.writerMode == model.QuotaWriterModeBridge {
		factInput = model.AccountQuotaRefundFactInput{
			EventKey: "billing-refund:" + requestID + ":v1", Kind: model.AccountQuotaRefundFactKindAuthoritative,
			RequestID: requestID, ReserveReceiptID: s.reserveReceipt.ID, AuditKey: "upstream-failure:" + requestID,
			WriterEpoch: s.reserveReceipt.WriterEpoch, UserID: s.reserveReceipt.UserID, TokenID: s.reserveReceipt.TokenID,
		}
	} else if funding, ok := s.funding.(*WalletFunding); ok {
		factInput = model.AccountQuotaRefundFactInput{
			EventKey: "billing-refund:" + requestID + ":v2", Kind: model.AccountQuotaRefundFactKindLegacyWallet,
			RequestID: requestID, UserID: s.relayInfo.UserId, TokenID: s.relayInfo.TokenId,
			WalletQuota: int64(funding.consumed), TokenQuota: int64(s.tokenConsumed),
		}
	} else if funding, ok := s.funding.(*SubscriptionFunding); ok {
		factInput = model.AccountQuotaRefundFactInput{
			EventKey: "billing-refund:" + requestID + ":v2", Kind: model.AccountQuotaRefundFactKindLegacySubscription,
			RequestID: requestID, UserID: s.relayInfo.UserId, TokenID: s.relayInfo.TokenId, SubscriptionID: funding.subscriptionId,
			SubscriptionQuota: funding.preConsumed, TokenQuota: int64(s.tokenConsumed),
		}
	} else {
		s.refunded = true
		s.mu.Unlock()
		return nil
	}

	fact, err := model.EnsureAccountQuotaRefundFact(ctx, model.DB, factInput)
	if err != nil {
		s.refundIntentErr = err
		s.refundRecoveryScheduled = true
		if taskErr := enqueueAccountQuotaSettlementRecovery(ctx); taskErr != nil {
			common.SysError("error enqueueing durable refund recovery task: " + taskErr.Error())
		}
		s.mu.Unlock()
		return err
	}
	if fact.State == model.AccountQuotaRefundFactApplied {
		s.refunded = true
		s.refundIntentErr = nil
		s.mu.Unlock()
		return nil
	}
	if fact.State == model.AccountQuotaRefundFactManual {
		s.refundIntentErr = model.ErrAccountQuotaRefundManualRequired
		s.refundRecoveryScheduled = true
		s.mu.Unlock()
		return model.ErrAccountQuotaRefundManualRequired
	}
	s.refundRecoveryScheduled = true
	var stored *model.AccountQuotaRefundFact
	var terminal *model.AccountQuotaMutationReceipt
	var runErr error
	for attempt := 0; attempt < 3; attempt++ {
		stored, terminal, runErr = model.RecoverAccountQuotaRefundFact(ctx, model.DB, fact, "billing-session:"+requestID)
		if stored != nil && stored.State == model.AccountQuotaRefundFactApplied {
			break
		}
		if stored != nil && stored.State == model.AccountQuotaRefundFactManual {
			runErr = errors.Join(model.ErrAccountQuotaRefundManualRequired, runErr)
			break
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
			fact, err = model.EnsureAccountQuotaRefundFact(ctx, model.DB, factInput)
			if err != nil {
				runErr = err
				break
			}
		}
	}
	if stored != nil && stored.State == model.AccountQuotaRefundFactApplied {
		s.refunded = true
		s.refundRecoveryScheduled = false
		s.refundIntentErr = nil
		s.terminalReceipt = terminal
		s.mu.Unlock()
		return nil
	}
	if stored != nil && stored.State == model.AccountQuotaRefundFactManual {
		s.refundIntentErr = errors.Join(model.ErrAccountQuotaRefundManualRequired, runErr)
		s.mu.Unlock()
		return s.refundIntentErr
	}
	if runErr == nil {
		runErr = model.ErrAccountQuotaRefundPending
	}
	s.refundIntentErr = runErr
	if taskErr := enqueueAccountQuotaSettlementRecovery(ctx); taskErr != nil {
		common.SysError("error enqueueing durable refund recovery task: " + taskErr.Error())
	}
	s.mu.Unlock()
	return errors.Join(model.ErrAccountQuotaRefundPending, runErr)
}

// NeedsRefund 返回是否存在需要退还的预扣状态。
func (s *BillingSession) NeedsRefund() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.needsRefundLocked()
}

func (s *BillingSession) needsRefundLocked() bool {
	if s.writerMode == model.QuotaWriterModeAuthoritative || s.writerMode == model.QuotaWriterModeBridge {
		return !s.settled && !s.refunded && !s.refundRecoveryScheduled && s.reserveReceipt != nil && s.reserveReceipt.AppliedQuota > 0
	}
	if s.settled || s.refunded || s.fundingSettled || s.settlementPending {
		// fundingSettled 时资金来源已提交结算，不能再退预扣费
		return false
	}
	if s.tokenConsumed > 0 {
		return true
	}
	// 订阅可能在 tokenConsumed=0 时仍预扣了额度
	if sub, ok := s.funding.(*SubscriptionFunding); ok && sub.preConsumed > 0 {
		return true
	}
	return false
}

// GetPreConsumedQuota 返回实际预扣的额度。
func (s *BillingSession) GetPreConsumedQuota() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.preConsumedQuota
}

func (s *BillingSession) Reserve(targetQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writerMode == model.QuotaWriterModeAuthoritative || s.writerMode == model.QuotaWriterModeBridge {
		if s.writerMode != model.QuotaWriterModeAuthoritative {
			return model.ErrDurableQuotaWriterModeDisabled
		}
		if s.settled || s.refunded || s.refundRecoveryScheduled || s.trusted || s.reserveReceipt == nil || targetQuota <= s.preConsumedQuota {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		receipt, err := model.ExtendAccountQuotaReservation(ctx, model.DB, s.reserveReceipt.ID, int64(targetQuota))
		cancel()
		if err != nil {
			return err
		}
		s.reserveReceipt = receipt
		s.preConsumedQuota = int(receipt.AppliedQuota)
		s.tokenConsumed = int(receipt.AppliedQuota)
		if funding, ok := s.funding.(*SubscriptionFunding); ok {
			funding.preConsumed = receipt.AppliedQuota
			funding.amount = receipt.AppliedQuota
			if receipt.After.Subscription != nil {
				funding.AmountUsedAfter = receipt.After.Subscription.AmountUsed
			}
		} else if funding, ok := s.funding.(*WalletFunding); ok {
			funding.consumed = int(receipt.AppliedQuota)
		}
		s.syncRelayInfo()
		return nil
	}

	if s.settled || s.refunded || s.trusted || targetQuota <= s.preConsumedQuota {
		return nil
	}

	delta := targetQuota - s.preConsumedQuota
	if delta <= 0 {
		return nil
	}

	if err := s.reserveFunding(delta); err != nil {
		return err
	}
	if err := s.reserveToken(delta); err != nil {
		s.rollbackFundingReserve(delta)
		return err
	}

	s.preConsumedQuota += delta
	s.tokenConsumed += delta
	s.extraReserved += delta
	s.syncRelayInfo()
	return nil
}

// ---------------------------------------------------------------------------
// PreConsume — 统一预扣费入口（含信任额度旁路）
// ---------------------------------------------------------------------------

// preConsume 执行预扣费：信任检查 -> 令牌预扣 -> 资金来源预扣。
// 任一步骤失败时原子回滚已完成的步骤。
//
// 本函数仅服务 legacy writer 模式：authoritative/bridge 会话的预扣经
// ReserveAccountQuota 内核（newAuthoritativeBillingSession），不可能到达这里；
// 到达即说明调用方绕过工厂直接构造会话，fail-closed 而非悄悄按 legacy 写入。
func (s *BillingSession) preConsume(c *gin.Context, quota int) *types.NewAPIError {
	if s.writerMode == model.QuotaWriterModeAuthoritative || s.writerMode == model.QuotaWriterModeBridge {
		return types.NewError(model.ErrDurableQuotaWriterModeDisabled, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	effectiveQuota := quota

	// ---- 信任额度旁路 ----
	if s.shouldTrust(c) {
		s.trusted = true
		effectiveQuota = 0
		logger.LogInfo(c, fmt.Sprintf("用户 %d 额度充足, 信任且不需要预扣费 (funding=%s)", s.relayInfo.UserId, s.funding.Source()))
	} else if effectiveQuota > 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 需要预扣费 %s (funding=%s)", s.relayInfo.UserId, logger.FormatQuota(effectiveQuota), s.funding.Source()))
	}

	// ---- 1) 预扣令牌额度 ----
	if effectiveQuota > 0 {
		if err := PreConsumeTokenQuota(s.relayInfo, effectiveQuota); err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		s.tokenConsumed = effectiveQuota
	}

	// ---- 2) 预扣资金来源 ----
	if err := s.funding.PreConsume(effectiveQuota); err != nil {
		// 资金预扣失败时，只有令牌回滚被证明成功后才能清零本地状态。
		if s.tokenConsumed > 0 && !s.relayInfo.IsPlayground {
			rollbackErr := legacyTokenQuotaRefund(s.relayInfo.TokenId, s.relayInfo.TokenKey, s.tokenConsumed)
			if rollbackErr == nil {
				s.tokenConsumed = 0
			} else {
				// 该补偿路径仅在 legacy writer 模式下可达（preConsume 入口已对
				// authoritative/bridge fail-closed）。补偿事实使用稳定业务键
				// "billing-token-rollback:{requestID}"，不再附加时间戳：
				// 带时间戳的键会让每次重试生成新的补偿事实，恢复时重复退回
				// token 额度。缺少稳定请求身份时拒绝编造不稳定键，保持 token
				// 扣减可见并交给恢复任务与人工核对。
				requestID := strings.TrimSpace(s.relayInfo.RequestId)
				trackCtx, cancel := context.WithTimeout(context.Background(), authoritativeRefundTimeout)
				var trackErr error
				if requestID == "" {
					trackErr = model.ErrAccountQuotaMutationInvalidInput
				} else {
					factInput := model.AccountQuotaRefundFactInput{
						EventKey: "billing-token-rollback:" + requestID, Kind: model.AccountQuotaRefundFactKindLegacyWallet, RequestID: requestID,
						UserID: s.relayInfo.UserId, TokenID: s.relayInfo.TokenId, TokenQuota: int64(s.tokenConsumed),
					}
					var fact *model.AccountQuotaRefundFact
					fact, trackErr = model.EnsureAccountQuotaRefundFact(trackCtx, model.DB, factInput)
					if trackErr == nil {
						stored, _, recoverErr := model.RecoverAccountQuotaRefundFact(trackCtx, model.DB, fact, "billing-session:"+requestID)
						if recoverErr == nil && stored != nil && stored.State == model.AccountQuotaRefundFactApplied {
							s.tokenConsumed = 0
						} else {
							trackErr = errors.Join(model.ErrAccountQuotaRefundPending, recoverErr)
						}
					}
				}
				cancel()
				if s.tokenConsumed > 0 {
					s.refundRecoveryScheduled = true
					s.refundIntentErr = errors.Join(rollbackErr, trackErr)
					if taskErr := enqueueAccountQuotaSettlementRecovery(trackCtx); taskErr != nil {
						s.refundIntentErr = errors.Join(s.refundIntentErr, taskErr)
					}
					return types.NewError(errors.Join(err, s.refundIntentErr), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
				}
			}
		}
		// TODO: model 层应定义哨兵错误（如 ErrNoActiveSubscription），用 errors.Is 替代字符串匹配
		if errors.Is(err, ErrInsufficientWalletQuota) {
			userQuota, quotaErr := model.GetUserQuota(s.relayInfo.UserId, false)
			if quotaErr != nil {
				userQuota = 0
			}
			return types.NewErrorWithStatusCode(
				fmt.Errorf("用户额度不足, 剩余额度: %s", logger.FormatQuota(userQuota)),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "no active subscription") || strings.Contains(errMsg, "subscription quota insufficient") {
			return types.NewErrorWithStatusCode(fmt.Errorf("订阅额度不足或未配置订阅: %s", errMsg), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}

	s.preConsumedQuota = effectiveQuota

	// ---- 同步 RelayInfo 兼容字段 ----
	s.syncRelayInfo()

	return nil
}

// reserveFunding 保持 legacy Reserve 补扣语义，且仅在 legacy writer 模式下可达：
// Reserve 对 authoritative/bridge 已提前分流（authoritative 走
// ExtendAccountQuotaReservation 内核，bridge fail-closed），model 层
// DecreaseUserQuota/IncreaseUserQuota 亦经 requireLegacyQuotaWriterCall 在非
// legacy 模式下 fail-closed。无需在此重复守卫。
func (s *BillingSession) reserveFunding(delta int) error {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		// 与结算补扣（SettleBilling 正差额 → WalletFunding.Settle）语义一致：
		// 全额无条件扣减，余额不足的部分记为欠费（余额可为负），不中断请求，
		// 保证日志记录的预扣额度与用户余额的实际变动始终对账一致。
		// DecreaseUserQuota 仅在数据库错误时失败。
		if err := model.DecreaseUserQuota(funding.userId, delta, false); err != nil {
			return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		funding.consumed += delta
		return nil
	case *SubscriptionFunding:
		if err := model.PostConsumeUserSubscriptionDelta(funding.subscriptionId, int64(delta)); err != nil {
			return types.NewErrorWithStatusCode(
				fmt.Errorf("订阅额度不足或未配置订阅: %s", err.Error()),
				types.ErrorCodeInsufficientUserQuota,
				http.StatusForbidden,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithNoRecordErrorLog(),
			)
		}
		return nil
	default:
		return types.NewError(fmt.Errorf("unsupported funding source: %s", s.funding.Source()), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
}

// rollbackFundingReserve 与 reserveFunding 同处 legacy Reserve 分支，可达性守卫相同
//（见 reserveFunding 注释）；保持 legacy 回滚语义。
func (s *BillingSession) rollbackFundingReserve(delta int) {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		if err := model.IncreaseUserQuota(funding.userId, delta, false); err != nil {
			common.SysLog("error rolling back wallet funding reserve: " + err.Error())
		} else {
			funding.consumed -= delta
		}
	case *SubscriptionFunding:
		if err := model.PostConsumeUserSubscriptionDelta(funding.subscriptionId, -int64(delta)); err != nil {
			common.SysLog("error rolling back subscription funding reserve: " + err.Error())
		}
	}
}

func (s *BillingSession) reserveToken(delta int) error {
	if delta <= 0 || s.relayInfo.IsPlayground {
		return nil
	}
	if err := PreConsumeTokenQuota(s.relayInfo, delta); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	return nil
}

// shouldTrust 统一信任额度检查，适用于钱包和订阅。
func (s *BillingSession) shouldTrust(c *gin.Context) bool {
	// 异步任务（ForcePreConsume=true）必须预扣全额，不允许信任旁路
	if s.relayInfo.ForcePreConsume {
		return false
	}

	trustQuota := common.GetTrustQuota()
	if trustQuota <= 0 {
		return false
	}

	// 检查令牌是否充足
	tokenTrusted := s.relayInfo.TokenUnlimited
	if !tokenTrusted {
		tokenQuota := c.GetInt("token_quota")
		tokenTrusted = tokenQuota > trustQuota
	}
	if !tokenTrusted {
		return false
	}

	switch s.funding.Source() {
	case BillingSourceWallet:
		return s.relayInfo.UserQuota > trustQuota
	case BillingSourceSubscription:
		// 订阅不能启用信任旁路。原因：
		// 1. PreConsumeUserSubscription 要求 amount>0 来创建预扣记录并锁定订阅
		// 2. SubscriptionFunding.PreConsume 忽略参数，始终用 s.amount 预扣
		// 3. 若信任旁路将 effectiveQuota 设为 0，会导致 preConsumedQuota 与实际订阅预扣不一致
		return false
	default:
		return false
	}
}

// syncRelayInfo 将 BillingSession 的状态同步到 RelayInfo 的兼容字段上。
func (s *BillingSession) syncRelayInfo() {
	info := s.relayInfo
	info.FinalPreConsumedQuota = s.preConsumedQuota
	info.BillingSource = s.funding.Source()

	if sub, ok := s.funding.(*SubscriptionFunding); ok {
		info.SubscriptionId = sub.subscriptionId
		info.SubscriptionPreConsumed = sub.preConsumed + int64(s.extraReserved)
		info.SubscriptionPostDelta = 0
		info.SubscriptionAmountTotal = sub.AmountTotal
		info.SubscriptionAmountUsedAfterPreConsume = sub.AmountUsedAfter + int64(s.extraReserved)
		info.SubscriptionPlanId = sub.PlanId
		info.SubscriptionPlanTitle = sub.PlanTitle
	} else {
		info.SubscriptionId = 0
		info.SubscriptionPreConsumed = 0
	}
}

// ---------------------------------------------------------------------------
// NewBillingSession 工厂 — 根据计费偏好创建会话并处理回退
// ---------------------------------------------------------------------------

func newAuthoritativeBillingSession(c *gin.Context, relayInfo *relaycommon.RelayInfo, preConsumedQuota int, mode model.QuotaWriterMode) (*BillingSession, *types.NewAPIError) {
	if mode != model.QuotaWriterModeAuthoritative {
		return nil, types.NewError(model.ErrDurableQuotaWriterModeDisabled, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	if !model.AccountQuotaSettlementIntentSchemaReady() {
		return nil, types.NewError(model.ErrQuotaMaintenanceBackfillIncomplete, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	var ctx context.Context
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	preference := common.NormalizeBillingPreference(relayInfo.UserSetting.BillingPreference)
	channelID := 0
	if relayInfo.ChannelMeta != nil {
		channelID = relayInfo.ChannelId
	}
	receipt, err := model.ReserveAccountQuota(ctx, model.DB, model.AccountQuotaReserveInput{
		RequestID: relayInfo.RequestId, UserID: relayInfo.UserId, TokenID: relayInfo.TokenId, RequestedQuota: int64(preConsumedQuota),
		BillingPreference: preference, TrustQuota: int64(common.GetTrustQuota()), AllowTrust: !relayInfo.ForcePreConsume,
		Playground: relayInfo.IsPlayground,
		BillingContext: model.AccountBillingContext{Version: 1, ModelName: relayInfo.OriginModelName, OriginModelName: relayInfo.OriginModelName,
			UsingGroup: relayInfo.UsingGroup, BillingPreference: preference, ChannelID: channelID, ForcePreConsume: relayInfo.ForcePreConsume,
			Playground: relayInfo.IsPlayground, FreeModel: relayInfo.PriceData.FreeModel},
	})
	if err != nil {
		if errors.Is(err, model.ErrAccountQuotaMutationInsufficient) {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		return nil, types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	session := &BillingSession{relayInfo: relayInfo, writerMode: mode, reserveReceipt: receipt, settlementIntentReady: true,
		preConsumedQuota: int(receipt.AppliedQuota), tokenConsumed: int(receipt.AppliedQuota), trusted: receipt.AppliedQuota == 0 && preConsumedQuota > 0}
	if receipt.BillingSource == BillingSourceSubscription {
		funding := &SubscriptionFunding{requestId: receipt.RequestID, userId: receipt.UserID, modelName: relayInfo.OriginModelName, amount: receipt.AppliedQuota,
			subscriptionId: receipt.SubscriptionID, preConsumed: receipt.AppliedQuota}
		if receipt.After.Subscription != nil {
			funding.AmountTotal = receipt.After.Subscription.AmountTotal
			funding.AmountUsedAfter = receipt.After.Subscription.AmountUsed
		}
		if planInfo, planErr := model.GetSubscriptionPlanInfoByUserSubscriptionId(receipt.SubscriptionID); planErr == nil && planInfo != nil {
			funding.PlanId, funding.PlanTitle = planInfo.PlanId, planInfo.PlanTitle
		}
		session.funding = funding
	} else if receipt.BillingSource == BillingSourceWallet {
		session.funding = &WalletFunding{userId: receipt.UserID, consumed: int(receipt.AppliedQuota)}
	} else {
		session.funding = &FreeFunding{}
	}
	session.syncRelayInfo()
	return session, nil
}

// NewBillingSession 根据用户计费偏好创建 BillingSession，处理 subscription_first / wallet_first 的回退。
// 创建成功的会话计入 quota writer 进程级在途计数，由 Settle/Refund 收尾对称归还。
func NewBillingSession(c *gin.Context, relayInfo *relaycommon.RelayInfo, preConsumedQuota int) (*BillingSession, *types.NewAPIError) {
	session, apiErr := newBillingSession(c, relayInfo, preConsumedQuota)
	if apiErr == nil && session != nil {
		model.TrackQuotaWriterInflightStart()
		session.inflightTracked = true
	}
	return session, apiErr
}

func newBillingSession(c *gin.Context, relayInfo *relaycommon.RelayInfo, preConsumedQuota int) (*BillingSession, *types.NewAPIError) {
	if relayInfo == nil {
		return nil, types.NewError(fmt.Errorf("relayInfo is nil"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	mode := model.QuotaWriterModeLegacy
	if model.DB != nil && model.DB.Migrator().HasTable(&model.QuotaWriterEpoch{}) {
		state, stateErr := model.GetQuotaWriterEpochState(model.DB)
		if stateErr != nil {
			return nil, types.NewError(stateErr, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		mode = model.QuotaWriterMode(state.Mode)
	}
	if mode != model.QuotaWriterModeLegacy {
		return newAuthoritativeBillingSession(c, relayInfo, preConsumedQuota, mode)
	}

	pref := common.NormalizeBillingPreference(relayInfo.UserSetting.BillingPreference)

	// 钱包路径需要先检查用户额度
	tryWallet := func() (*BillingSession, *types.NewAPIError) {
		userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if userQuota <= 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("用户额度不足, 剩余额度: %s", logger.FormatQuota(userQuota)),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		if userQuota-preConsumedQuota < 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("预扣费额度失败, 用户剩余额度: %s, 需要预扣费额度: %s", logger.FormatQuota(userQuota), logger.FormatQuota(preConsumedQuota)),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		relayInfo.UserQuota = userQuota

		session := &BillingSession{
			relayInfo: relayInfo,
			funding:   &WalletFunding{userId: relayInfo.UserId},
		}
		if apiErr := session.preConsume(c, preConsumedQuota); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	trySubscription := func() (*BillingSession, *types.NewAPIError) {
		subConsume := int64(preConsumedQuota)
		if subConsume <= 0 {
			subConsume = 1
		}
		session := &BillingSession{
			relayInfo: relayInfo,
			funding: &SubscriptionFunding{
				requestId: relayInfo.RequestId,
				userId:    relayInfo.UserId,
				modelName: relayInfo.OriginModelName,
				amount:    subConsume,
			},
		}
		// 必须传 subConsume 而非 preConsumedQuota，保证 SubscriptionFunding.amount、
		// preConsume 参数和 FinalPreConsumedQuota 三者一致，避免订阅多扣费。
		if apiErr := session.preConsume(c, int(subConsume)); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	switch pref {
	case "subscription_only":
		return trySubscription()
	case "wallet_only":
		return tryWallet()
	case "wallet_first":
		session, err := tryWallet()
		if err != nil {
			if err.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				return trySubscription()
			}
			return nil, err
		}
		return session, nil
	case "subscription_first":
		fallthrough
	default:
		hasSub, subCheckErr := model.HasActiveUserSubscription(relayInfo.UserId)
		if subCheckErr != nil {
			return nil, types.NewError(subCheckErr, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if !hasSub {
			return tryWallet()
		}
		session, apiErr := trySubscription()
		if apiErr != nil {
			if apiErr.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				// 仅当用户的活跃订阅允许钱包回退时才回退到钱包，否则返回订阅额度不足错误
				allowOverflow, overflowErr := model.UserActiveSubscriptionsAllowWalletOverflow(relayInfo.UserId)
				if overflowErr != nil {
					return nil, types.NewError(overflowErr, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
				}
				if allowOverflow {
					return tryWallet()
				}
				return nil, apiErr
			}
			return nil, apiErr
		}
		return session, nil
	}
}
