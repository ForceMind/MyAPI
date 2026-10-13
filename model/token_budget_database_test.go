package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	sqlitedriver "github.com/glebarez/go-sqlite"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestTokenBudgetConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/budget.db")
			if engine.env != "" {
				if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
					t.Skip("requires explicitly configured disposable B2 database")
				}
				dsn := strings.TrimSpace(os.Getenv(engine.env))
				require.NotEmpty(t, dsn)
				var err error
				dialector, err = b2SubmissionDatabaseDialector(engine.name, dsn)
				require.NoError(t, err)
			}
			db, err := gorm.Open(dialector, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("tb_%d_", time.Now().UnixNano())}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&TokenBudgetPolicyChange{}, &TokenBudgetReservation{}, &TokenBudget{}, &Token{}, &User{}, &ChannelQuotaSnapshot{}))
				require.NoError(t, sqlDB.Close())
			})
			seedTokenBudgetDB(t, db)
			require.NoError(t, db.AutoMigrate(&TokenBudget{}, &TokenBudgetReservation{}, &TokenBudgetPolicyChange{}))
			tokenBudgetLifecycleContract(t, db)
			tokenBudgetChatLifecycleContract(t, db)
			tokenBudgetSendCancelRaceContract(t, db, engine.name)
			feeBudgetConfiguredLifecycle(t, db)
			accountQuotaThresholdConfiguredContract(t, db)
		})
	}
}

func tokenBudgetSendCancelRaceContract(t *testing.T, db *gorm.DB, engine string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, db.Create(&Token{Id: 31, UserId: 2, Key: "budget-send-cancel-race", Status: common.TokenStatusEnabled, ExpiredTime: -1}).Error)
	_, err := ConfigureTokenBudget(ctx, db, 1, TokenBudgetPolicyInput{ID: strings.Repeat("9", 64), TokenID: 31, Enabled: true, Limit: TokenBudgetOpenAIChatContext})
	require.NoError(t, err)
	request := TokenBudgetReservation{RequestID: "chat-send-cancel-race", TokenID: 31, UserID: 2, ChannelID: 7, ModelName: TokenBudgetOpenAIChatModel,
		BoundSource: TokenBudgetBoundOpenAIChat, PayloadSHA256: strings.Repeat("b", 64), InputTokens: TokenBudgetOpenAIChatContext, MaxOutputTokens: 20}
	require.NoError(t, ReserveTokenBudget(ctx, db, request))
	type mutationResult struct {
		action string
		err    error
	}
	start := make(chan struct{})
	results := make(chan mutationResult, 2)
	var wg sync.WaitGroup
	for _, action := range []string{"send", "cancel"} {
		wg.Add(1)
		go func(action string) {
			defer wg.Done()
			<-start
			_, mutationErr := MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: 31, RequestID: request.RequestID, Action: action})
			results <- mutationResult{action: action, err: mutationErr}
		}(action)
	}
	close(start)
	wg.Wait()
	close(results)
	var winner, loser mutationResult
	for result := range results {
		if result.err == nil {
			require.Empty(t, winner.action, "send and cancellation cannot both commit")
			winner = result
		} else {
			require.Empty(t, loser.action, "send and cancellation must have one loser")
			loser = result
		}
	}
	require.NotEmpty(t, winner.action, "one serialized mutation must commit")
	require.NotEmpty(t, loser.action, "one serialized mutation must lose")
	expectedLoserErr := ErrTokenBudgetConflict
	if winner.action == "send" {
		expectedLoserErr = ErrTokenBudgetPending
	}
	if engine != "sqlite" || errors.Is(loser.err, expectedLoserErr) {
		require.ErrorIs(t, loser.err, expectedLoserErr)
	} else {
		var sqliteErr *sqlitedriver.Error
		require.ErrorAs(t, loser.err, &sqliteErr, "SQLite may only replace the serialized business result with a typed lock error")
		code := sqliteErr.Code() & 0xff
		require.True(t, code == 5 || code == 6, "SQLite loser must report SQLITE_BUSY or SQLITE_LOCKED, got code %d", sqliteErr.Code())
	}
	var raced TokenBudgetReservation
	require.NoError(t, db.First(&raced, "request_id = ?", request.RequestID).Error)
	if winner.action == "send" {
		assert.Equal(t, TokenBudgetSent, raced.State)
		_, err = MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: 31, RequestID: request.RequestID, Action: "settle", Input: 0, Output: 0})
		require.NoError(t, err)
	} else {
		assert.Equal(t, TokenBudgetCancelled, raced.State)
	}
	budget, err := LookupTokenBudget(ctx, db, 31)
	require.NoError(t, err)
	assert.Zero(t, budget.Reserved)
	assert.Empty(t, budget.PendingRequestID)
}
