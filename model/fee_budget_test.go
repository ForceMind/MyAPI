package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFeeBudgetDecimalPreservesExactTinyAmounts(t *testing.T) {
	for _, value := range []string{"", "00", "+1", "-1", "1e3", "NaN", " 1", "1.", strings.Repeat("1", 129)} {
		_, err := NormalizeFeeBudgetUSD(value)
		require.Error(t, err, value)
	}
	amount, err := NormalizeFeeBudgetUSD("0.0000000000000000000000003600")
	require.NoError(t, err)
	assert.Equal(t, "0.00000000000000000000000036", amount)
	sum, err := feeBudgetAdd(amount, amount)
	require.NoError(t, err)
	assert.Equal(t, "0.00000000000000000000000072", sum)
	require.NoError(t, feeBudgetFits(sum, amount, amount))
	require.ErrorIs(t, feeBudgetFits(amount, amount, amount), ErrFeeBudgetExceeded)
}

func TestFeeBudgetSharesPersistentLifecycleWithoutTokenLimit(t *testing.T) {
	db := setupTokenBudgetDB(t)
	ctx := context.Background()
	policy, err := ConfigureTokenBudget(ctx, db, 1, TokenBudgetPolicyInput{ID: strings.Repeat("a", 64), TokenID: 11, Fee: &FeeBudgetPolicyInput{Enabled: true, LimitUSD: "0.00030"}})
	require.NoError(t, err)
	assert.False(t, policy.Enabled)
	assert.Equal(t, "0.0003", policy.FeeLimitUSD)
	request := budgetReservationFixture("fee-request")
	request.FeeEnabled = true
	request.RequestServiceTier = "default"
	request.FeeReservedUSD = "0.0003"
	request.FeePriceEvidence = `{"currency":"USD","scope":"synthetic"}`
	require.NoError(t, ReserveTokenBudget(ctx, db, request))
	_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: 11, RequestID: request.RequestID, Action: "send"})
	require.NoError(t, err)
	mutation := TokenBudgetMutation{TokenID: 11, RequestID: request.RequestID, Action: "settle", Input: 10, Output: 5}
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.ErrorIs(t, err, ErrFeeBudgetInvalid, "old Token-only settlement cannot release a fee reserve")
	mutation.FeeUSD = common.GetPointer("0.00002")
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	mutation.FeeUSD = common.GetPointer("0.000021")
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.ErrorIs(t, err, ErrTokenBudgetConflict)
	view, err := ReadTokenBudget(ctx, db, 1, 11)
	require.NoError(t, err)
	assert.Equal(t, "0.00002", view.Policy.FeeUsedUSD)
	assert.Equal(t, "0", view.Policy.FeeReservedUSD)
	assert.Empty(t, view.Policy.PendingRequestID)
	assert.EqualValues(t, 15, view.Policy.Used, "actual counts remain available while only the fee limit is enabled")
	// An older Token-only settings command preserves the independently enabled fee policy.
	next, err := ConfigureTokenBudget(ctx, db, 1, TokenBudgetPolicyInput{ID: strings.Repeat("b", 64), TokenID: 11, ExpectedRevision: view.Policy.Revision})
	require.NoError(t, err)
	assert.True(t, next.FeeEnabled)
	assert.Equal(t, "0.00002", next.FeeUsedUSD)
	request.RequestID = "fee-next"
	require.ErrorIs(t, ReserveTokenBudget(ctx, db, request), ErrFeeBudgetExceeded)
}

func TestFeeBudgetOverBoundRetainsReserveUntilAuditedRecovery(t *testing.T) {
	db := setupTokenBudgetDB(t)
	ctx := context.Background()
	_, err := ConfigureTokenBudget(ctx, db, 1, TokenBudgetPolicyInput{ID: strings.Repeat("a", 64), TokenID: 11, Enabled: true, Limit: 100, Fee: &FeeBudgetPolicyInput{Enabled: true, LimitUSD: "0.0003"}})
	require.NoError(t, err)
	request := budgetReservationFixture("fee-bound")
	request.FeeEnabled = true
	request.RequestServiceTier = "default"
	request.FeeReservedUSD = "0.0003"
	request.FeePriceEvidence = `{"currency":"USD","scope":"synthetic"}`
	require.NoError(t, ReserveTokenBudget(ctx, db, request))
	mutation := TokenBudgetMutation{TokenID: 11, RequestID: request.RequestID, Action: "send"}
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	mutation.Action = "settle"
	mutation.Input = 10
	mutation.Output = 5
	mutation.FeeUSD = common.GetPointer("0.0004")
	row, err := MutateTokenBudgetRequest(ctx, db, mutation)
	require.ErrorIs(t, err, ErrTokenBudgetBound)
	assert.Equal(t, TokenBudgetUnknown, row.State)
	budget, err := LookupTokenBudget(ctx, db, 11)
	require.NoError(t, err)
	assert.Zero(t, budget.Used)
	assert.Equal(t, "0", budget.FeeUsedUSD)
	assert.Equal(t, "0.0003", budget.FeeReservedUSD)
	mutation.Action = "reconcile"
	mutation.ActorID = 1
	mutation.Evidence = "verified terminal fee evidence"
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	budget, err = LookupTokenBudget(ctx, db, 11)
	require.NoError(t, err)
	assert.Equal(t, "0.0004", budget.FeeUsedUSD)
	assert.Equal(t, "0", budget.FeeReservedUSD)
	request.RequestID = "fee-after-overrun"
	require.ErrorIs(t, ReserveTokenBudget(ctx, db, request), ErrFeeBudgetExceeded)
}

func feeBudgetConfiguredLifecycle(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, db.Create(&Token{Id: 12, UserId: 2, Key: "fee-database-fixture", Status: common.TokenStatusEnabled, ExpiredTime: -1}).Error)
	_, err := ConfigureTokenBudget(ctx, db, 1, TokenBudgetPolicyInput{ID: strings.Repeat("e", 64), TokenID: 12, Fee: &FeeBudgetPolicyInput{Enabled: true, LimitUSD: "0.001"}})
	require.NoError(t, err)
	request := budgetReservationFixture("fee-db-request")
	request.TokenID = 12
	request.FeeEnabled = true
	request.RequestServiceTier = "default"
	request.FeeReservedUSD = "0.0003"
	request.FeePriceEvidence = `{"scope":"synthetic-three-db"}`
	require.NoError(t, ReserveTokenBudget(ctx, db, request))
	_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: 12, RequestID: request.RequestID, Action: "send"})
	require.NoError(t, err)
	injected := errors.New("synthetic fee budget write interruption")
	callback := "r1:fee-budget-write-failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "TokenBudget" {
			tx.AddError(injected)
		}
	}))
	mutation := TokenBudgetMutation{TokenID: 12, RequestID: request.RequestID, Action: "settle", Input: 10, Output: 5, FeeUSD: common.GetPointer("0.00000000000000000000000036")}
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.Error(t, err)
	require.NoError(t, db.Callback().Update().Remove(callback))
	policy, err := LookupTokenBudget(ctx, db, 12)
	require.NoError(t, err)
	assert.Equal(t, "0", policy.FeeUsedUSD)
	assert.Equal(t, "0.0003", policy.FeeReservedUSD)
	assert.Zero(t, policy.Used)
	for range 2 {
		_, err = MutateTokenBudgetRequest(ctx, db, mutation)
		require.NoError(t, err)
	}
	policy, err = LookupTokenBudget(ctx, db, 12)
	require.NoError(t, err)
	assert.Equal(t, *mutation.FeeUSD, policy.FeeUsedUSD)
	assert.Equal(t, "0", policy.FeeReservedUSD)
	assert.EqualValues(t, 15, policy.Used)
}
