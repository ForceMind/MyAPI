package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func strictChatStreamFixture(t *testing.T, mode model.QuotaWriterMode) (*gorm.DB, *gin.Context, *relaycommon.RelayInfo, *dto.Usage) {
	t.Helper()
	db, ctx, info, root := strictTokenBudgetFixture(t, mode)
	_, err := model.ConfigureTokenBudget(context.Background(), db, root, model.TokenBudgetPolicyInput{ID: strings.Repeat("d", 64), TokenID: info.TokenId, ExpectedRevision: 1, Enabled: true, Limit: model.TokenBudgetOpenAIChatContext})
	require.NoError(t, err)
	info.RelayMode, info.UpstreamModelName, info.OriginModelName = relayconstant.RelayModeChatCompletions, model.TokenBudgetOpenAIChatModel, model.TokenBudgetOpenAIChatModel
	ctx.Request.URL.Path = "/v1/chat/completions"
	request, _ := chatBudgetOutbound(t, strings.TrimSuffix(chatBudgetBody, "}")+`,"stream":true,"stream_options":{"include_usage":true}}`)
	client := tokenBudgetCountTestClient(t, func(http.ResponseWriter, *http.Request) { t.Error("Chat must not call the Responses count endpoint") })
	require.NoError(t, PrepareTokenBudgetDispatch(ctx, client, request, info))
	info.IsStream = true
	info.StreamStatus = relaycommon.NewStreamStatus()
	usage := chatBudgetUsage(10, 5)
	usage.BillingUsage.ChatTextEvidence.Stream = true
	return db, ctx, info, usage
}

func TestStrictChatStreamCancellationRequiresCompleteEvidence(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, state := range []string{"complete", "cancel-after-complete", "missing-usage", "incomplete-usage", "estimated-usage", "invalid-total", "wrong-tier", "wrong-model", "stream-mismatch", "missing-status", "missing-done", "client-gone", "stream-error", "end-error"} {
			t.Run(string(mode)+"/"+state, func(t *testing.T) {
				db, ctx, info, usage := strictChatStreamFixture(t, mode)
				end := relaycommon.StreamEndReasonDone
				var endErr error
				switch state {
				case "missing-usage":
					usage = nil
				case "incomplete-usage":
					usage.BillingUsage.Incomplete = true
				case "estimated-usage":
					usage.BillingUsage.Estimated = true
				case "invalid-total":
					usage.BillingUsage.ChatTextEvidence.TotalTokens++
				case "wrong-tier":
					usage.BillingUsage.ChatTextEvidence.ServiceTier = "priority"
				case "wrong-model":
					usage.BillingUsage.ChatTextEvidence.Model = "unqualified-model"
				case "stream-mismatch":
					usage.BillingUsage.ChatTextEvidence.Stream = false
				case "missing-done":
					end = relaycommon.StreamEndReasonEOF
				case "client-gone":
					end, endErr = relaycommon.StreamEndReasonClientGone, context.Canceled
				case "stream-error":
					info.StreamStatus.RecordError("synthetic downstream write failure")
				case "end-error":
					endErr = errors.New("synthetic terminal failure")
				}
				info.StreamStatus.SetEndReason(end, endErr)
				if state == "missing-status" {
					info.StreamStatus = nil
				}
				// Every negative case is also cancelled: cancellation never upgrades
				// incomplete, invalid or interrupted evidence into successful usage.
				if state != "complete" {
					cancelled, cancel := context.WithCancel(ctx.Request.Context())
					cancel()
					ctx.Request = ctx.Request.WithContext(cancelled)
				}
				err := SettleTokenBudgetUsage(ctx, info, usage)
				if state == "complete" || state == "cancel-after-complete" {
					require.NoError(t, err)
					for range 2 {
						require.NoError(t, SettleTokenBudgetUsage(ctx, info, usage))
						require.NoError(t, SettleBilling(ctx, info, 15))
					}
					var token model.Token
					var user model.User
					require.NoError(t, db.First(&token, info.TokenId).Error)
					require.NoError(t, db.First(&user, info.UserId).Error)
					assert.Equal(t, 985, token.RemainQuota)
					assert.Equal(t, 15, token.UsedQuota)
					assert.Equal(t, 985, user.Quota)
				} else {
					require.ErrorIs(t, err, model.ErrTokenBudgetPending)
					PostTextConsumeQuota(ctx, info, usage, nil)
					assert.True(t, FinalizeTokenBudgetDispatch(ctx, info))
					assert.False(t, info.Billing.NeedsRefund())
				}
				budget, err := model.LookupTokenBudget(context.Background(), db, info.TokenId)
				require.NoError(t, err)
				var row model.TokenBudgetReservation
				require.NoError(t, db.First(&row, "request_id = ?", info.RequestId).Error)
				if state == "complete" || state == "cancel-after-complete" {
					assert.EqualValues(t, 15, budget.Used)
					assert.Zero(t, budget.Reserved)
					assert.Equal(t, model.TokenBudgetSettled, row.State)
				} else {
					assert.Zero(t, budget.Used)
					assert.EqualValues(t, model.TokenBudgetOpenAIChatContext, budget.Reserved)
					assert.Equal(t, model.TokenBudgetUnknown, row.State)
					assert.Nil(t, row.ActualInput)
				}
			})
		}
	}
}

func TestStrictChatCompletedCancelledSettlementConcurrentReplay(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db, ctx, info, usage := strictChatStreamFixture(t, mode)
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
			cancelled, cancel := context.WithCancel(ctx.Request.Context())
			cancel()
			ctx.Request = ctx.Request.WithContext(cancelled)
			// Serialize SQLite transactions through its single writer connection,
			// while callers race at the actual service/session boundary.
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			start := make(chan struct{})
			results := make(chan error, 2)
			for range 2 {
				c := ctx.Copy()
				relay := &relaycommon.RelayInfo{UserId: info.UserId, TokenId: info.TokenId, RequestId: info.RequestId, StrictTokenBudget: true, IsStream: true, StreamStatus: info.StreamStatus}
				go func() {
					<-start
					if err := SettleTokenBudgetUsage(c, relay, usage); err != nil {
						results <- err
						return
					}
					results <- info.Billing.(*BillingSession).SettleWithContext(cancelled, 15)
				}()
			}
			close(start)
			first, second := <-results, <-results
			require.NoError(t, first)
			require.NoError(t, second)
			require.NoError(t, SettleTokenBudgetUsage(ctx, info, usage))
			require.NoError(t, info.Billing.(*BillingSession).SettleWithContext(cancelled, 15))
			budget, err := model.LookupTokenBudget(context.Background(), db, info.TokenId)
			require.NoError(t, err)
			assert.EqualValues(t, 15, budget.Used)
			assert.Zero(t, budget.Reserved)
			var user model.User
			var token model.Token
			require.NoError(t, db.First(&user, info.UserId).Error)
			require.NoError(t, db.First(&token, info.TokenId).Error)
			assert.Equal(t, 985, user.Quota)
			assert.Equal(t, 985, token.RemainQuota)
			assert.Equal(t, 15, token.UsedQuota)
		})
	}
}

func TestBillingOperationContextRemainsBoundedAfterClientCancellation(t *testing.T) {
	type requestKey struct{}
	parent, cancelParent := context.WithCancel(context.WithValue(context.Background(), requestKey{}, "synthetic-request"))
	cancelParent()
	started := time.Now()
	operation, cancel := billingOperationContext(parent, 5*time.Second)
	defer cancel()
	require.NoError(t, operation.Err())
	assert.Equal(t, "synthetic-request", operation.Value(requestKey{}))
	deadline, ok := operation.Deadline()
	require.True(t, ok)
	assert.False(t, deadline.Before(started))
	assert.False(t, deadline.After(time.Now().Add(5*time.Second)))
	cancel()
	assert.ErrorIs(t, operation.Err(), context.Canceled)
}
