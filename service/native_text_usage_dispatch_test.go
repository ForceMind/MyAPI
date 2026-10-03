package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeTextDispatchScopeUsesValidatedGenerationRequest(t *testing.T) {
	for _, tc := range []struct {
		path         string
		gemini, hold bool
	}{
		{"/v1/messages", false, true}, {"/v1/completions", false, true},
		{"/v1beta/models/gemini-fixture:generateContent?trace=synthetic", true, true},
		{"/v1/models/gemini-fixture:streamGenerateContent", true, true},
		{"/v1beta/models/gemini-fixture", true, true}, // Existing adaptor chooses generateContent itself.
		{"/v1beta/models/gemini-fixture:generateContent", false, false},
		{"/unrelated/models/gemini-fixture:generateContent", true, false},
		{"/v1/messages/count_tokens", false, false}, {"/v1/messages/batches", false, false},
		{"/v1/embeddings", false, false}, {"/api/channel/test", true, false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", tc.path, nil)
			info := &relaycommon.RelayInfo{Billing: &BillingSession{}}
			if tc.gemini {
				info.Request = &dto.GeminiChatRequest{}
			}
			assert.Equal(t, tc.hold, textUsageDispatchSession(ctx, info) != nil)
			if tc.hold {
				info.IsChannelTest = true
				assert.Nil(t, textUsageDispatchSession(ctx, info))
				info.IsChannelTest = false
				info.StrictTokenBudget = true
				assert.Nil(t, textUsageDispatchSession(ctx, info))
				info.StrictTokenBudget = false
				info.PriceData.UsePrice = true
				assert.Nil(t, textUsageDispatchSession(ctx, info))
				info.PriceData.UsePrice = false
				ctx.Request.Method = "GET"
				assert.Nil(t, textUsageDispatchSession(ctx, info))
			}
		})
	}
}

func TestNativeTextDispatchUnknownHoldsAndRootRecoveryIsIdempotent(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, path := range []string{"/v1/messages", "/v1/completions", "/v1beta/models/gemini-fixture:generateContent", "/v1/models/gemini-fixture:streamGenerateContent"} {
			t.Run(string(mode)+path, func(t *testing.T) {
				db := setupPostConsumeModeDB(t, mode)
				require.NoError(t, db.AutoMigrate(&model.UsageReviewDecision{}, &model.TokenBudget{}, &model.TokenBudgetReservation{}, &model.TokenBudgetPolicyChange{}, &model.Log{}))
				user, token := seedAuthoritativeBilling(t, db, "native-dispatch", 1000, 1000, false)
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest("POST", path+"?trace=fixture", nil)
				info := authoritativeRelay(user, token, "native-dispatch-request")
				info.ChannelMeta = &relaycommon.ChannelMeta{}
				info.StartTime = time.Now()
				if strings.Contains(path, "/models/") {
					info.Request = &dto.GeminiChatRequest{}
				}
				ctx.Set(common.RequestIdKey, info.RequestId)
				session, apiErr := NewBillingSession(ctx, info, 100)
				require.Nil(t, apiErr)
				info.Billing = session
				request, err := http.NewRequest("POST", "https://example.invalid/synthetic", strings.NewReader("synthetic"))
				require.NoError(t, err)
				require.NoError(t, PrepareTextUsageDispatch(ctx, request, info))
				assert.Nil(t, request.GetBody)
				ObserveTextUsageDispatchResponse(info, 200) // Response body may be truncated or invalid.
				require.True(t, FinalizeTextUsageDispatch(ctx, info))
				assert.False(t, session.NeedsRefund())
				require.ErrorIs(t, session.Refund(ctx), model.ErrAccountQuotaUsageUnresolved)
				view, err := model.GetUsageReview(context.Background(), db, user.Id, info.RequestId)
				require.NoError(t, err)
				assert.Nil(t, view.ActualQuota)
				assert.EqualValues(t, 100, view.ReservedQuota)
				root := model.User{Username: "native-reviewer", AffCode: "native-reviewer", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
				require.NoError(t, db.Create(&root).Error)
				for range 2 {
					review, err := model.ReconcileUsageReview(context.Background(), db, root.Id, info.RequestId, 20, "verified synthetic native response evidence")
					require.NoError(t, err)
					require.NoError(t, model.ProjectUsageReviewDecision(context.Background(), db, db, review.Decision.ID))
				}
				uq, remaining, used := loadPostConsumeBalances(t, db, user, token)
				assert.Equal(t, 980, uq)
				assert.Equal(t, 980, remaining)
				assert.Equal(t, 20, used)
			})
		}
	}
}
