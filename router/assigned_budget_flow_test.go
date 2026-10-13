package router

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type assignedUnknownAccountingHold struct {
	UserQuota        int64
	UserUsedQuota    int64
	UserRequestCount int
	TokenRemainQuota int64
	TokenUsedQuota   int64
	Legacy           *model.LegacyUsageReservation
	Head             *model.AccountQuotaReservationHead
	ReserveReceipt   *model.AccountQuotaMutationReceipt
	TerminalReceipts int64
	SettlementFacts  int64
}

func (f *chatBudgetFlowFixture) loadAssignedUnknownAccountingHold(t *testing.T, mode model.QuotaWriterMode, tokenID int, requestID string) assignedUnknownAccountingHold {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.First(&user, f.user.Id).Error)
	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	hold := assignedUnknownAccountingHold{
		UserQuota:        user.Quota,
		UserUsedQuota:    user.UsedQuota,
		UserRequestCount: user.RequestCount,
		TokenRemainQuota: token.RemainQuota,
		TokenUsedQuota:   token.UsedQuota,
	}
	// The writer's initial internal-quota hold is 100 in this fixture. The
	// strict Token budget independently reserves input 100 + max output 20.
	require.Equal(t, int64(1000000-100), hold.UserQuota)
	require.Zero(t, hold.UserUsedQuota)
	require.Zero(t, hold.UserRequestCount)
	require.Equal(t, int64(1000000-100), hold.TokenRemainQuota)
	require.Equal(t, int64(100), hold.TokenUsedQuota)

	if mode == model.QuotaWriterModeLegacy {
		legacy, err := model.FindLegacyUsageReservation(context.Background(), model.DB, requestID)
		require.NoError(t, err)
		require.Equal(t, model.LegacyUsageUnknown, legacy.State)
		require.Equal(t, int64(100), legacy.ReservedQuota)
		require.Equal(t, int64(100), legacy.TokenReservedQuota)
		require.Nil(t, legacy.ActualQuota)
		hold.Legacy = legacy
		return hold
	}

	var head model.AccountQuotaReservationHead
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&head).Error)
	require.Positive(t, head.RootReceiptID)
	require.Equal(t, head.RootReceiptID, head.CurrentReceiptID)
	require.Zero(t, head.TerminalReceiptID)
	require.Equal(t, int64(100), head.AppliedQuota)
	var reserve model.AccountQuotaMutationReceipt
	require.NoError(t, model.DB.First(&reserve, head.CurrentReceiptID).Error)
	require.Equal(t, head.RootReceiptID, reserve.ID)
	require.Equal(t, requestID, reserve.RequestID)
	require.Equal(t, model.AccountQuotaPhaseReserve, reserve.Phase)
	require.Equal(t, int64(100), reserve.RequestedQuota)
	require.Equal(t, int64(100), reserve.AppliedQuota)
	require.NoError(t, model.DB.Model(&model.AccountQuotaMutationReceipt{}).
		Where("request_id = ? AND phase IN ?", requestID, []string{model.AccountQuotaPhaseSettle, model.AccountQuotaPhaseRefund}).
		Count(&hold.TerminalReceipts).Error)
	require.Zero(t, hold.TerminalReceipts)
	require.NoError(t, model.DB.Model(&model.AccountQuotaSettlementFact{}).Where("request_id = ?", requestID).Count(&hold.SettlementFacts).Error)
	require.Zero(t, hold.SettlementFacts)
	hold.Head = &head
	hold.ReserveReceipt = &reserve
	return hold
}

func TestAssignedAccessStrictBudgetApplicationFlow(t *testing.T) {
	for _, protocol := range []string{"responses", "chat"} {
		t.Run(protocol, func(t *testing.T) {
			f := setupChatBudgetFlow(t)
			token := f.token(t, model.TokenBudgetOpenAIChatModel)
			policy := fmt.Sprintf(`{"enabled":true,"public_models":[%q],"upstream_models":[%q],"channel_ids":[%d]}`, model.TokenBudgetOpenAIChatModel, model.TokenBudgetOpenAIChatModel, f.channel.Id)
			_, err := model.ConfigureAssignedAccessPolicy(context.Background(), model.DB, "token", token.Id, 0, policy)
			require.NoError(t, err)
			var counts, generations atomic.Int32
			client := chatBudgetFlowTLSClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "api.openai.com", r.Host)
				assert.Equal(t, "Bearer synthetic-only", r.Header.Get("Authorization"))
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.EqualValues(t, len(body), r.ContentLength, "guard must preserve exact body framing")
				var fields map[string]any
				require.NoError(t, common.Unmarshal(body, &fields))
				assert.Equal(t, model.TokenBudgetOpenAIChatModel, fields["model"])
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/responses/input_tokens":
					require.Equal(t, "responses", protocol)
					counts.Add(1)
					assert.Equal(t, "hello", fields["input"])
					_, _ = io.WriteString(w, `{"object":"response.input_tokens","input_tokens":100}`)
				case "/v1/responses":
					require.Equal(t, "responses", protocol)
					f.assertTokenBudgetSent(t, token.Id)
					generations.Add(1)
					assert.Equal(t, "hello", fields["input"])
					_, _ = io.WriteString(w, `{"id":"assigned-responses","object":"response","status":"completed","model":"gpt-6.1-sol","service_tier":"default","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":100,"output_tokens":10,"total_tokens":110,"input_tokens_details":{"cached_tokens":40,"cache_write_tokens":30},"output_tokens_details":{"reasoning_tokens":4}}}`)
				case "/v1/chat/completions":
					require.Equal(t, "chat", protocol)
					f.assertTokenBudgetSent(t, token.Id)
					generations.Add(1)
					_, _ = io.WriteString(w, chatBudgetFlowResponse(chatFlowUsage, false, false))
				default:
					t.Errorf("unexpected assigned-budget fixture path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			})
			t.Cleanup(service.SetHttpClientForTest(client))
			path, body := "/v1/responses", `{"model":"gpt-6.1-sol","input":"hello","max_output_tokens":20,"service_tier":"default"}`
			if protocol == "chat" {
				path, body = "/v1/chat/completions", chatFlowRequest
			}
			response := f.request(http.MethodPost, path, "sk-"+token.Key, body)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.EqualValues(t, 1, generations.Load())
			var row model.TokenBudgetReservation
			require.NoError(t, model.DB.Where("token_id = ?", token.Id).First(&row).Error)
			require.Equal(t, model.TokenBudgetSettled, row.State)
			require.NotNil(t, row.ActualInput)
			assert.EqualValues(t, 100, *row.ActualInput)
			require.NotNil(t, row.ActualOutput)
			assert.EqualValues(t, 10, *row.ActualOutput)
			require.NotNil(t, row.ActualFeeUSD)
			assert.Equal(t, "0.000239", *row.ActualFeeUSD)
			if protocol == "responses" {
				assert.EqualValues(t, 1, counts.Load())
				assert.Equal(t, model.TokenBudgetBoundOpenAIResponses, row.BoundSource)
				assert.EqualValues(t, 120, row.Reserved)
				assert.Equal(t, "0.00045", row.FeeReservedUSD)
			} else {
				assert.Zero(t, counts.Load())
				assert.Equal(t, model.TokenBudgetBoundOpenAIChat, row.BoundSource)
				assert.EqualValues(t, 1050000, row.Reserved)
				assert.Equal(t, "5.2502", row.FeeReservedUSD)
			}
			budget, err := model.LookupTokenBudget(context.Background(), model.DB, token.Id)
			require.NoError(t, err)
			assert.EqualValues(t, 110, budget.Used)
			assert.Zero(t, budget.Reserved)
			assert.Equal(t, "0.000239", budget.FeeUsedUSD)
			assert.Equal(t, "0", budget.FeeReservedUSD)
			assert.Empty(t, budget.PendingRequestID)
		})
	}
}

func TestAssignedAccessStrictChatBudgetKeepsUnknownHold(t *testing.T) {
	f := setupChatBudgetFlow(t)
	token := f.token(t, model.TokenBudgetOpenAIChatModel)
	_, err := model.ConfigureAssignedAccessPolicy(context.Background(), model.DB, "token", token.Id, 0, `{"enabled":true,"public_models":["gpt-6.1-sol"],"upstream_models":["gpt-6.1-sol"],"channel_ids":null}`)
	require.NoError(t, err)
	f.setResponder(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatBudgetFlowResponse(strings.Replace(chatFlowUsage, `,"cache_write_tokens":30`, "", 1), false, false))
	})
	response := f.request(http.MethodPost, "/v1/chat/completions", "sk-"+token.Key, chatFlowRequest)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.EqualValues(t, 1, f.hits.Load())
	var row model.TokenBudgetReservation
	require.NoError(t, model.DB.Where("token_id = ?", token.Id).First(&row).Error)
	assert.Equal(t, model.TokenBudgetUnknown, row.State)
	assert.Nil(t, row.ActualInput)
	assert.Nil(t, row.ActualFeeUSD)
	budget, err := model.LookupTokenBudget(context.Background(), model.DB, token.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 1050000, budget.Reserved)
	assert.Equal(t, "5.2502", budget.FeeReservedUSD)
	assert.Zero(t, budget.Used)
	assert.Equal(t, "0", budget.FeeUsedUSD)
	assert.Equal(t, row.RequestID, budget.PendingRequestID)
}

func TestAssignedAccessStrictResponsesUnknownReopenRecoveryBothWriters(t *testing.T) {
	oldCountToken := constant.CountToken
	constant.CountToken = false
	t.Cleanup(func() { constant.CountToken = oldCountToken })
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			f := setupChatBudgetFlow(t)
			require.NoError(t, model.DB.Model(&model.QuotaWriterEpoch{}).Where("id = ?", 1).Updates(map[string]any{
				"mode": string(mode), "epoch": int64(41), "lock_version": gorm.Expr("lock_version + ?", 1),
			}).Error)

			key := fmt.Sprintf("responsesrestart%s%d", mode, f.serial.Add(1))
			token := model.Token{UserId: f.user.Id, Name: key, Key: key, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000000,
				ModelLimitsEnabled: true, ModelLimits: model.TokenBudgetOpenAIChatModel}
			require.NoError(t, model.DB.Create(&token).Error)
			budgetBody := fmt.Sprintf(`{"id":"%064x","expected_revision":0,"enabled":true,"limit":2000000,"confirmed":true}`, f.serial.Add(1))
			budgetResponse := f.request(http.MethodPut, fmt.Sprintf("/api/token/%d/budget", token.Id), *f.root.AccessToken, budgetBody)
			require.Equal(t, http.StatusOK, budgetResponse.Code, budgetResponse.Body.String())
			f.currentTokenID.Store(int64(token.Id))
			policy := fmt.Sprintf(`{"enabled":true,"public_models":[%q],"upstream_models":[%q],"channel_ids":[%d]}`, model.TokenBudgetOpenAIChatModel, model.TokenBudgetOpenAIChatModel, f.channel.Id)
			_, err := model.ConfigureAssignedAccessPolicy(context.Background(), model.DB, "token", token.Id, 0, policy)
			require.NoError(t, err)

			var counts, generations atomic.Int32
			client := chatBudgetFlowTLSClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "api.openai.com", r.Host)
				assert.Equal(t, "Bearer synthetic-only", r.Header.Get("Authorization"))
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.EqualValues(t, len(body), r.ContentLength, "guard must preserve exact body framing")
				var fields map[string]any
				require.NoError(t, common.Unmarshal(body, &fields))
				assert.Equal(t, model.TokenBudgetOpenAIChatModel, fields["model"])
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/responses/input_tokens":
					counts.Add(1)
					assert.Equal(t, "hello", fields["input"])
					_, _ = io.WriteString(w, `{"object":"response.input_tokens","input_tokens":100}`)
				case "/v1/responses":
					f.assertTokenBudgetSent(t, token.Id)
					generations.Add(1)
					assert.Equal(t, "hello", fields["input"])
					_, _ = io.WriteString(w, `{"id":"assigned-responses-unknown","object":"response","status":"completed","model":"gpt-6.1-sol","service_tier":"default","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`)
				default:
					t.Errorf("unexpected Responses restart fixture path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			})
			t.Cleanup(service.SetHttpClientForTest(client))
			requestBody := `{"model":"gpt-6.1-sol","input":"hello","max_output_tokens":20,"service_tier":"default"}`
			response := f.request(http.MethodPost, "/v1/responses", "sk-"+token.Key, requestBody)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.EqualValues(t, 1, counts.Load())
			require.EqualValues(t, 1, generations.Load())

			var reservation model.TokenBudgetReservation
			require.NoError(t, model.DB.Where("token_id = ?", token.Id).First(&reservation).Error)
			require.Equal(t, model.TokenBudgetUnknown, reservation.State)
			require.Equal(t, model.TokenBudgetBoundOpenAIResponses, reservation.BoundSource)
			require.EqualValues(t, 100, reservation.InputTokens)
			require.EqualValues(t, 20, reservation.MaxOutputTokens)
			require.EqualValues(t, 120, reservation.Reserved)
			require.Nil(t, reservation.ActualInput)
			require.Nil(t, reservation.ActualOutput)
			budget, err := model.LookupTokenBudget(context.Background(), model.DB, token.Id)
			require.NoError(t, err)
			require.Zero(t, budget.Used)
			require.EqualValues(t, 120, budget.Reserved)
			require.Equal(t, reservation.RequestID, budget.PendingRequestID)
			review, err := model.GetUsageReview(context.Background(), model.DB, f.root.Id, reservation.RequestID)
			require.NoError(t, err)
			require.Equal(t, model.AccountQuotaTerminalRecoveryUsageUnknown, review.State)
			require.Nil(t, review.ActualQuota)
			require.NotNil(t, review.TokenBudget)
			require.Equal(t, model.TokenBudgetUnknown, review.TokenBudget.State)
			beforeRestart := f.loadAssignedUnknownAccountingHold(t, mode, token.Id, reservation.RequestID)
			var consumeLogs int64
			require.NoError(t, model.DB.Model(&model.Log{}).Where("request_id = ? AND type = ?", reservation.RequestID, model.LogTypeConsume).Count(&consumeLogs).Error)
			require.Zero(t, consumeLogs)

			f.restartDatabase(t)
			blocked := f.request(http.MethodPost, "/v1/responses", "sk-"+token.Key, requestBody)
			require.Equal(t, http.StatusConflict, blocked.Code, blocked.Body.String())
			var blockedPayload struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(blocked.Body.Bytes(), &blockedPayload))
			require.Equal(t, "token_budget_pending", blockedPayload.Error.Code)
			require.EqualValues(t, 1, counts.Load(), "persisted pending admission must run before input counting")
			require.EqualValues(t, 1, generations.Load(), "persisted pending admission must block generation transport")
			var reservations int64
			require.NoError(t, model.DB.Model(&model.TokenBudgetReservation{}).Where("token_id = ?", token.Id).Count(&reservations).Error)
			require.EqualValues(t, 1, reservations, "the blocked request cannot create a second reservation")
			afterRestart := f.loadAssignedUnknownAccountingHold(t, mode, token.Id, reservation.RequestID)
			require.Equal(t, beforeRestart, afterRestart, "database reopen and same-Key rejection must preserve the original accounting hold")

			evidence := "reliable internal quota 120 with exact Responses input 100 and output 10 after database reopen"
			recoveryBody := fmt.Sprintf(`{"request_id":%q,"action":"reconcile","actual_quota":120,"actual_input_tokens":100,"actual_output_tokens":10,"evidence_reference":%q,"confirmed_reliable_evidence":true}`, reservation.RequestID, evidence)
			path := fmt.Sprintf("/api/token/%d/budget/recover", token.Id)
			for range 2 {
				recovered := f.request(http.MethodPost, path, *f.root.AccessToken, recoveryBody)
				require.Equal(t, http.StatusOK, recovered.Code, recovered.Body.String())
			}

			require.NoError(t, model.DB.Where("request_id = ?", reservation.RequestID).First(&reservation).Error)
			require.Equal(t, model.TokenBudgetSettled, reservation.State)
			require.NotNil(t, reservation.ActualInput)
			require.EqualValues(t, 100, *reservation.ActualInput)
			require.NotNil(t, reservation.ActualOutput)
			require.EqualValues(t, 10, *reservation.ActualOutput)
			budget, err = model.LookupTokenBudget(context.Background(), model.DB, token.Id)
			require.NoError(t, err)
			require.EqualValues(t, 110, budget.Used)
			require.Zero(t, budget.Reserved)
			require.Empty(t, budget.PendingRequestID)

			var decisions int64
			require.NoError(t, model.DB.Model(&model.UsageReviewDecision{}).Where("request_id = ?", reservation.RequestID).Count(&decisions).Error)
			require.EqualValues(t, 1, decisions)
			var decision model.UsageReviewDecision
			require.NoError(t, model.DB.Where("request_id = ?", reservation.RequestID).First(&decision).Error)
			require.Equal(t, f.root.Id, decision.ActorID)
			require.Equal(t, f.user.Id, decision.UserID)
			require.Equal(t, token.Id, decision.TokenID)
			require.Equal(t, f.channel.Id, decision.ChannelID)
			require.Equal(t, model.TokenBudgetOpenAIChatModel, decision.ModelName)
			require.Equal(t, int64(120), decision.ActualQuota)
			require.Equal(t, evidence, decision.EvidenceReference)
			require.Len(t, decision.EvidenceDigest, 64)
			require.NotNil(t, decision.ActualInputTokens)
			require.Equal(t, int64(100), *decision.ActualInputTokens)
			require.NotNil(t, decision.ActualOutputTokens)
			require.Equal(t, int64(10), *decision.ActualOutputTokens)
			require.Nil(t, decision.ActualFeeUSD)
			require.True(t, decision.StatisticsApplied)
			require.True(t, decision.LogProjected)
			var logs []model.Log
			require.NoError(t, model.DB.Where("request_id = ? AND type = ?", reservation.RequestID, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.Equal(t, 120, logs[0].Quota)
			require.Equal(t, 100, logs[0].PromptTokens)
			require.Equal(t, 10, logs[0].CompletionTokens)
			require.Contains(t, logs[0].Other, `"token_counts_confirmed":true`)
			require.Contains(t, logs[0].Other, `"settlement_status":"manually_reconciled"`)

			require.NoError(t, model.DB.First(&f.user, f.user.Id).Error)
			require.Equal(t, int64(1000000-120), f.user.Quota)
			require.Equal(t, int64(120), f.user.UsedQuota)
			require.Equal(t, 1, f.user.RequestCount)
			require.NoError(t, model.DB.First(&token, token.Id).Error)
			require.Equal(t, int64(1000000-120), token.RemainQuota)
			require.Equal(t, int64(120), token.UsedQuota)

			if mode == model.QuotaWriterModeLegacy {
				legacy, err := model.FindLegacyUsageReservation(context.Background(), model.DB, reservation.RequestID)
				require.NoError(t, err)
				require.Equal(t, model.LegacyUsageSettled, legacy.State)
				require.Equal(t, int64(100), legacy.ReservedQuota)
				require.Equal(t, int64(100), legacy.TokenReservedQuota)
				require.NotNil(t, legacy.ActualQuota)
				require.EqualValues(t, 120, *legacy.ActualQuota)
				return
			}
			// Manual authoritative reconciliation applies one terminal receipt
			// directly. The automatic settlement-intent/fact path is not involved.
			var facts int64
			require.NoError(t, model.DB.Model(&model.AccountQuotaSettlementFact{}).Where("request_id = ?", reservation.RequestID).Count(&facts).Error)
			require.Zero(t, facts)
			var intents int64
			require.NoError(t, model.DB.Model(&model.AccountQuotaSettlementIntent{}).Where("request_id = ?", reservation.RequestID).Count(&intents).Error)
			require.Zero(t, intents)
			var terminalReceipts []model.AccountQuotaMutationReceipt
			require.NoError(t, model.DB.Where("request_id = ? AND phase = ?", reservation.RequestID, model.AccountQuotaPhaseSettle).Find(&terminalReceipts).Error)
			require.Len(t, terminalReceipts, 1)
			var head model.AccountQuotaReservationHead
			require.NoError(t, model.DB.Where("request_id = ?", reservation.RequestID).First(&head).Error)
			require.Equal(t, beforeRestart.Head.RootReceiptID, head.RootReceiptID)
			require.Equal(t, beforeRestart.Head.CurrentReceiptID, head.CurrentReceiptID)
			require.Equal(t, terminalReceipts[0].ID, head.TerminalReceiptID)
			require.Equal(t, int64(100), head.AppliedQuota)
			require.Equal(t, beforeRestart.Head.CurrentReceiptID, terminalReceipts[0].ParentReceiptID)
			require.Equal(t, model.AccountQuotaPhaseSettle, terminalReceipts[0].Phase)
			expectedAuditKey := fmt.Sprintf("usage-review:%d:%s", f.root.Id, decision.EvidenceDigest)
			require.Equal(t, expectedAuditKey, terminalReceipts[0].AuditKey)
			require.Equal(t, int64(120), terminalReceipts[0].RequestedQuota)
			require.Equal(t, int64(120), terminalReceipts[0].AppliedQuota)
			require.Equal(t, terminalReceipts[0].RequestedDeltas, terminalReceipts[0].AppliedDeltas)
			require.Equal(t, int64(-20), terminalReceipts[0].AppliedDeltas.UserQuota)
			require.Equal(t, int64(-20), terminalReceipts[0].AppliedDeltas.TokenRemainQuota)
			require.Equal(t, int64(20), terminalReceipts[0].AppliedDeltas.TokenUsedQuota)
			require.Zero(t, terminalReceipts[0].AppliedDeltas.SubscriptionAmountUsed)
			require.Equal(t, int64(1000000-100), terminalReceipts[0].Before.User.Quota)
			require.Equal(t, int64(1000000-120), terminalReceipts[0].After.User.Quota)
			require.Equal(t, int64(1000000-100), terminalReceipts[0].Before.Token.RemainQuota)
			require.Equal(t, int64(1000000-120), terminalReceipts[0].After.Token.RemainQuota)
			require.Equal(t, int64(100), terminalReceipts[0].Before.Token.UsedQuota)
			require.Equal(t, int64(120), terminalReceipts[0].After.Token.UsedQuota)
			var obligation model.AccountQuotaTerminalRecoveryObligation
			require.NoError(t, model.DB.Where("request_id = ?", reservation.RequestID).First(&obligation).Error)
			require.Equal(t, beforeRestart.Head.CurrentReceiptID, obligation.ReserveReceiptID)
			require.Equal(t, model.AccountQuotaPhaseSettle, obligation.Phase)
			require.Equal(t, expectedAuditKey, obligation.AuditKey)
			require.Equal(t, int64(120), obligation.ActualQuota)
			require.Equal(t, head.WriterEpoch, obligation.WriterEpoch)
			require.Equal(t, head.ReserveFingerprint, obligation.RequestFingerprint)
			require.Equal(t, model.AccountQuotaTerminalRecoveryApplied, obligation.State)
			require.Equal(t, terminalReceipts[0].ID, obligation.TerminalReceiptID)
			require.Empty(t, obligation.LastError)
		})
	}
}
