package model

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTokenBudgetDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/token-budget.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	seedTokenBudgetDB(t, db)
	return db
}

func seedTokenBudgetDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &TokenBudget{}, &TokenBudgetReservation{}, &TokenBudgetPolicyChange{}))
	require.NoError(t, db.Create(&User{Id: 1, Username: "budget-root", AffCode: "budroot", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&User{Id: 2, Username: "budget-owner", AffCode: "budowner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&Token{Id: 11, UserId: 2, Key: "budget-fixture", Status: common.TokenStatusEnabled, ExpiredTime: -1}).Error)
}

func budgetReservationFixture(id string) TokenBudgetReservation {
	return TokenBudgetReservation{RequestID: id, TokenID: 11, UserID: 2, ChannelID: 7, ModelName: "budget-fixture", BoundSource: TokenBudgetBoundOpenAIResponses, PayloadSHA256: strings.Repeat("b", 64), InputTokens: 10, MaxOutputTokens: 20}
}

func TestTokenBudgetPersistentLifecycle(t *testing.T) {
	db := setupTokenBudgetDB(t)
	tokenBudgetLifecycleContract(t, db)
}

func tokenBudgetLifecycleContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	config := TokenBudgetPolicyInput{ID: strings.Repeat("a", 64), TokenID: 11, Limit: 100, Enabled: true}
	_, err := ConfigureTokenBudget(ctx, db, 2, config)
	require.Error(t, err, "ownership does not grant administrative limit writes")
	state, err := ConfigureTokenBudget(ctx, db, 1, config)
	require.NoError(t, err)
	assert.EqualValues(t, 1, state.Revision)
	_, err = ConfigureTokenBudget(ctx, db, 1, config)
	require.NoError(t, err, "identical operation is a receipt replay")
	request := budgetReservationFixture("budget-request")
	require.NoError(t, ReserveTokenBudget(ctx, db, request))
	require.ErrorIs(t, ReserveTokenBudget(ctx, db, request), ErrTokenBudgetDuplicate)
	require.ErrorIs(t, ReserveTokenBudget(ctx, db, budgetReservationFixture("other-request")), ErrTokenBudgetPending)
	config.ID, config.ExpectedRevision, config.Enabled = strings.Repeat("c", 64), 2, false
	_, err = ConfigureTokenBudget(ctx, db, 1, config)
	require.ErrorIs(t, err, ErrTokenBudgetPending)
	mutation := TokenBudgetMutation{TokenID: 11, RequestID: request.RequestID, Action: "send"}
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.ErrorIs(t, err, ErrTokenBudgetDuplicate, "replay cannot authorize a second send")
	mutation.Action, mutation.Reason = "hold", "missing"
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err, "hold replay also works on MySQL unchanged rows")
	mutation.Action = "cancel"
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.ErrorIs(t, err, ErrTokenBudgetPending)
	mutation.Action, mutation.Input, mutation.Output = "settle", 10, 5
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.ErrorIs(t, err, ErrTokenBudgetPending)
	mutation.Action, mutation.ActorID, mutation.Evidence = "reconcile", 2, "isolated provider usage evidence"
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.Error(t, err)
	mutation.ActorID = 1
	row, err := MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	require.NotNil(t, row.ActualInput)
	assert.EqualValues(t, 10, *row.ActualInput)
	assert.Equal(t, TokenBudgetSettled, row.State)
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	mutation.Output = 6
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.ErrorIs(t, err, ErrTokenBudgetConflict)
	var budget TokenBudget
	require.NoError(t, db.First(&budget, "token_id = ?", 11).Error)
	assert.EqualValues(t, 15, budget.Used)
	assert.Zero(t, budget.Reserved)
	assert.Empty(t, budget.PendingRequestID)
	assert.NoError(t, db.Model(&TokenBudgetPolicyChange{}).Where("id = ?", strings.Repeat("a", 64)).Count(new(int64)).Error)
	require.Error(t, db.Model(&TokenBudgetPolicyChange{}).Where("id = ?", strings.Repeat("a", 64)).Update("actor_id", 2).Error)
}

func TestTokenBudgetBoundsAndFailedTerminalRemainHeld(t *testing.T) {
	db := setupTokenBudgetDB(t)
	ctx := context.Background()
	_, err := ConfigureTokenBudget(ctx, db, 1, TokenBudgetPolicyInput{ID: strings.Repeat("a", 64), TokenID: 11, Limit: 30, Enabled: true})
	require.NoError(t, err)
	request := budgetReservationFixture("bound-request")
	require.NoError(t, ReserveTokenBudget(ctx, db, request))
	mutation := TokenBudgetMutation{TokenID: 11, RequestID: request.RequestID, Action: "send"}
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	mutation.Action, mutation.Input, mutation.Output = "settle", 10, 25
	row, err := MutateTokenBudgetRequest(ctx, db, mutation)
	require.ErrorIs(t, err, ErrTokenBudgetBound)
	require.NotNil(t, row)
	assert.Equal(t, TokenBudgetUnknown, row.State)
	assert.Nil(t, row.ActualOutput, "bound breach is held, never clamped into a charge")
	assert.EqualValues(t, 25, *row.ObservedOutput)
	require.ErrorIs(t, ReserveTokenBudget(ctx, db, budgetReservationFixture("blocked-after-bound")), ErrTokenBudgetPending)
	mutation.Action, mutation.ActorID, mutation.Evidence = "reconcile", 1, "verified actual total exceeds allowance"
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	var budget TokenBudget
	require.NoError(t, db.First(&budget, "token_id = ?", 11).Error)
	assert.EqualValues(t, 35, budget.Used, "actual evidence must not be truncated to the limit")
	require.ErrorIs(t, ReserveTokenBudget(ctx, db, budgetReservationFixture("blocked-after-recovery")), ErrTokenBudgetExceeded)
}

func TestTokenBudgetConcurrentAdmissionAndAtomicFailure(t *testing.T) {
	db := setupTokenBudgetDB(t)
	ctx := context.Background()
	_, err := ConfigureTokenBudget(ctx, db, 1, TokenBudgetPolicyInput{ID: strings.Repeat("a", 64), TokenID: 11, Limit: 100, Enabled: true})
	require.NoError(t, err)
	var wg sync.WaitGroup
	results := make(chan string, 2)
	for _, id := range []string{"concurrent-a", "concurrent-b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if ReserveTokenBudget(ctx, db, budgetReservationFixture(id)) == nil {
				results <- id
			}
		}(id)
	}
	wg.Wait()
	close(results)
	var admitted []string
	for id := range results {
		admitted = append(admitted, id)
	}
	require.Len(t, admitted, 1)
	mutation := TokenBudgetMutation{TokenID: 11, RequestID: admitted[0], Action: "send"}
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("budget-fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "token_budgets" {
			tx.AddError(errors.New("isolated budget write failure"))
		}
	}))
	mutation.Action, mutation.Input, mutation.Output = "settle", 10, 5
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.Error(t, err)
	require.NoError(t, db.Callback().Update().Remove("budget-fail"))
	var row TokenBudgetReservation
	require.NoError(t, db.First(&row, "request_id = ?", admitted[0]).Error)
	assert.Equal(t, TokenBudgetSent, row.State)
	assert.Nil(t, row.ActualInput)
	require.ErrorIs(t, ReserveTokenBudget(ctx, db, budgetReservationFixture("after-write-failure")), ErrTokenBudgetPending)
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
}

func TestTokenBudgetReopenPreservesSentAndDeletedKeyRecovery(t *testing.T) {
	db := setupTokenBudgetDB(t)
	ctx := context.Background()
	_, err := ConfigureTokenBudget(ctx, db, 1, TokenBudgetPolicyInput{ID: strings.Repeat("a", 64), TokenID: 11, Limit: 100, Enabled: true})
	require.NoError(t, err)
	request := budgetReservationFixture("restart-request")
	require.NoError(t, ReserveTokenBudget(ctx, db, request))
	mutation := TokenBudgetMutation{TokenID: 11, RequestID: request.RequestID, Action: "send"}
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	oldSQL, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, oldSQL.Close())
	reopened, err := gorm.Open(db.Dialector, &gorm.Config{})
	require.NoError(t, err)
	newSQL, err := reopened.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, newSQL.Close()) })
	require.ErrorIs(t, ReserveTokenBudget(ctx, reopened, budgetReservationFixture("after-restart")), ErrTokenBudgetPending)
	require.NoError(t, reopened.Delete(&Token{}, 11).Error)
	mutation.Action, mutation.Input, mutation.Output, mutation.ActorID, mutation.Evidence = "reconcile", 10, 5, 1, "reliable evidence after restart"
	_, err = MutateTokenBudgetRequest(ctx, reopened, mutation)
	require.NoError(t, err, "soft deletion must not erase the outstanding obligation")
}

func TestTokenBudgetReportedZeroAndPreDispatchCancellation(t *testing.T) {
	db := setupTokenBudgetDB(t)
	ctx := context.Background()
	_, err := ConfigureTokenBudget(ctx, db, 1, TokenBudgetPolicyInput{ID: strings.Repeat("a", 64), TokenID: 11, Limit: 100, Enabled: true})
	require.NoError(t, err)
	request := budgetReservationFixture("never-sent")
	require.NoError(t, ReserveTokenBudget(ctx, db, request))
	mutation := TokenBudgetMutation{TokenID: 11, RequestID: request.RequestID, Action: "cancel"}
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	request.RequestID, request.InputTokens = "reported-zero", 0
	require.NoError(t, ReserveTokenBudget(ctx, db, request))
	mutation.RequestID, mutation.Action = request.RequestID, "send"
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	mutation.Action = "settle"
	row, err := MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	require.NotNil(t, row.ActualInput)
	assert.Zero(t, *row.ActualInput)
	var budget TokenBudget
	require.NoError(t, db.First(&budget, "token_id = ?", 11).Error)
	assert.Zero(t, budget.Used)
	assert.Zero(t, budget.Reserved)
	assert.Empty(t, budget.PendingRequestID)
}

func TestTokenBudgetOwnerReadsAndAuditedUnsentRecovery(t *testing.T) {
	db := setupTokenBudgetDB(t)
	ctx := context.Background()
	view, err := ReadTokenBudget(ctx, db, 2, 11)
	require.NoError(t, err)
	assert.False(t, view.Policy.Enabled)
	_, err = ConfigureTokenBudget(ctx, db, 1, TokenBudgetPolicyInput{ID: strings.Repeat("a", 64), TokenID: 11, Limit: 100, Enabled: true})
	require.NoError(t, err)
	require.NoError(t, db.Create(&User{Id: 3, Username: "budget-stranger", AffCode: "budother", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}).Error)
	_, err = ReadTokenBudget(ctx, db, 3, 11)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, ReserveTokenBudget(ctx, db, budgetReservationFixture("abandoned-before-send")))
	require.NoError(t, db.Model(&Token{}).Where("id = ?", 11).Update("status", common.TokenStatusDisabled).Error)
	_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: 11, RequestID: "abandoned-before-send", Action: "send"})
	require.ErrorIs(t, err, ErrAccountQuotaMutationIneligible, "revocation after reservation must prevent dispatch")
	view, err = ReadTokenBudget(ctx, db, 2, 11)
	require.NoError(t, err)
	require.NotNil(t, view.Pending)
	assert.EqualValues(t, 30, view.Policy.Reserved)
	mutation := TokenBudgetMutation{TokenID: 11, RequestID: "abandoned-before-send", Action: "cancel", ActorID: 2, Evidence: "checked pre-dispatch state"}
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.Error(t, err)
	mutation.ActorID = 1
	row, err := MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	assert.Equal(t, mutation.Evidence, row.EvidenceReference)
	assert.NotEmpty(t, row.EvidenceDigest)
	assert.Nil(t, row.ActualInput)
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.NoError(t, err)
	mutation.Action, mutation.ActorID, mutation.Evidence = "send", 0, ""
	_, err = MutateTokenBudgetRequest(ctx, db, mutation)
	require.Error(t, err, "a concurrent worker must not send after pre-dispatch cancellation")
}
