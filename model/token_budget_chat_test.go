package model

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTokenBudgetChatContextLifecycle(t *testing.T) {
	tokenBudgetChatLifecycleContract(t, setupTokenBudgetDB(t))
}

func tokenBudgetChatLifecycleContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, db.Create(&Token{Id: 21, UserId: 2, Key: "chat-budget-fixture", Status: common.TokenStatusEnabled, ExpiredTime: -1}).Error)
	_, err := ConfigureTokenBudget(ctx, db, 1, TokenBudgetPolicyInput{ID: strings.Repeat("7", 64), TokenID: 21, Enabled: true, Limit: 2 * TokenBudgetOpenAIChatContext})
	require.NoError(t, err)
	request := TokenBudgetReservation{RequestID: "chat-context", TokenID: 21, UserID: 2, ChannelID: 7, ModelName: TokenBudgetOpenAIChatModel,
		BoundSource: TokenBudgetBoundOpenAIChat, PayloadSHA256: strings.Repeat("b", 64), InputTokens: TokenBudgetOpenAIChatContext, MaxOutputTokens: 20, Reserved: 1}
	for _, change := range []func(*TokenBudgetReservation){
		func(r *TokenBudgetReservation) { r.ModelName = "gpt-6.1-sol-alias" },
		func(r *TokenBudgetReservation) { r.InputTokens-- },
		func(r *TokenBudgetReservation) { r.MaxOutputTokens = TokenBudgetOpenAIChatMaxOutput + 1 },
	} {
		invalid := request
		change(&invalid)
		require.ErrorIs(t, ReserveTokenBudget(ctx, db, invalid), ErrTokenBudgetInvalid)
	}
	require.NoError(t, ReserveTokenBudget(ctx, db, request))
	var row TokenBudgetReservation
	require.NoError(t, db.First(&row, "request_id = ?", request.RequestID).Error)
	assert.Equal(t, TokenBudgetOpenAIChatContext, row.Reserved, "caller cannot shrink the reservation; output is already inside context")
	_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: 21, RequestID: request.RequestID, Action: "send"})
	require.NoError(t, err)
	_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: 21, RequestID: request.RequestID, Action: "settle", Input: 0, Output: 0})
	require.NoError(t, err)
	// The independent input ceiling cannot be W-M: actual output may be zero.
	request.RequestID = "chat-full-input"
	require.NoError(t, ReserveTokenBudget(ctx, db, request))
	_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: 21, RequestID: request.RequestID, Action: "send"})
	require.NoError(t, err)
	_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: 21, RequestID: request.RequestID, Action: "settle", Input: TokenBudgetOpenAIChatContext, Output: 0})
	require.NoError(t, err)
	request.RequestID = "chat-over-total"
	require.NoError(t, ReserveTokenBudget(ctx, db, request))
	_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: 21, RequestID: request.RequestID, Action: "send"})
	require.NoError(t, err)
	rowPtr, err := MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: 21, RequestID: request.RequestID, Action: "settle", Input: TokenBudgetOpenAIChatContext, Output: 1})
	require.ErrorIs(t, err, ErrTokenBudgetBound)
	require.Equal(t, TokenBudgetUnknown, rowPtr.State)
	require.Nil(t, rowPtr.ActualInput)
	require.Equal(t, TokenBudgetOpenAIChatContext, rowPtr.Reserved)
	recovery := TokenBudgetMutation{TokenID: 21, RequestID: request.RequestID, Action: "reconcile", Input: 10, Output: 2, ActorID: 1, Evidence: "synthetic verified Chat accounting"}
	for range 2 {
		_, err = MutateTokenBudgetRequest(ctx, db, recovery)
		require.NoError(t, err)
	}
	require.ErrorIs(t, ReserveTokenBudget(ctx, db, TokenBudgetReservation{RequestID: "cannot-fit", TokenID: 21, UserID: 2, ChannelID: 7, ModelName: TokenBudgetOpenAIChatModel, BoundSource: TokenBudgetBoundOpenAIChat, PayloadSHA256: strings.Repeat("b", 64), InputTokens: TokenBudgetOpenAIChatContext, MaxOutputTokens: 20}), ErrTokenBudgetExceeded)
	policy, err := LookupTokenBudget(ctx, db, 21)
	require.NoError(t, err)
	_, err = ConfigureTokenBudget(ctx, db, 1, TokenBudgetPolicyInput{ID: strings.Repeat("8", 64), TokenID: 21, ExpectedRevision: policy.Revision, Enabled: true, Limit: policy.Used + TokenBudgetOpenAIChatContext})
	require.NoError(t, err)
	var wg sync.WaitGroup
	admitted := make(chan string, 2)
	for _, id := range []string{"chat-concurrent-a", "chat-concurrent-b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			r := request
			r.RequestID = id
			if ReserveTokenBudget(ctx, db, r) == nil {
				admitted <- id
			}
		}(id)
	}
	wg.Wait()
	close(admitted)
	var ids []string
	for id := range admitted {
		ids = append(ids, id)
	}
	require.Len(t, ids, 1, "one conservative reservation serializes concurrent admission")
	_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: 21, RequestID: ids[0], Action: "cancel"})
	require.NoError(t, err)
}
