package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRealtimeCheckpointPreservesFrozenKnownPrefix(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, scenario := range []string{"fractional", "tiered", "zero", "write-failure", "unknown", "unpriced-image"} {
			t.Run(string(mode)+"/"+scenario, func(t *testing.T) {
				db := setupPostConsumeModeDB(t, mode)
				require.NoError(t, db.AutoMigrate(&model.UsageReviewDecision{}, &model.TokenBudget{}, &model.TokenBudgetReservation{}, &model.TokenBudgetPolicyChange{}, &model.Log{}))
				user, token := seedAuthoritativeBilling(t, db, "checkpoint", 1000, 1000, false)
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
				info := authoritativeRelay(user, token, "checkpoint")
				info.RelayFormat = types.RelayFormatOpenAIRealtime
				info.ChannelMeta = &relaycommon.ChannelMeta{}
				info.StartTime = time.Now()
				info.PriceData = hosttypes.PriceData{ModelRatio: 0.05, CompletionRatio: 1, AudioRatio: 1, AudioCompletionRatio: 1, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}
				if scenario == "write-failure" {
					info.PriceData.ModelRatio = 1
				}
				require.NoError(t, info.PriceData.CaptureQuotaUnit(1_000_000))
				if scenario == "tiered" {
					expression := `len <= 10 ? p + cr * 0.5 : (p + cr * 0.5) * 10`
					info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), QuotaPerUnit: 1_000_000, GroupRatio: 1}
				}
				session, apiErr := NewBillingSession(ctx, info, 100)
				require.Nil(t, apiErr)
				info.Billing = session
				info.UserQuota = 1 << 30
				require.NoError(t, PrepareRealtimeUsageDispatch(ctx, info))
				raw := `{"total_tokens":8,"input_tokens":8,"output_tokens":0,"input_token_details":{"text_tokens":8,"audio_tokens":0,"cached_tokens":2,"cached_tokens_details":{"text_tokens":2,"audio_tokens":0}},"output_token_details":{"text_tokens":0,"audio_tokens":0}}`
				if scenario == "zero" {
					raw = `{"total_tokens":0,"input_tokens":0,"output_tokens":0,"input_token_details":{"text_tokens":0,"audio_tokens":0},"output_token_details":{"text_tokens":0,"audio_tokens":0}}`
				}
				if scenario == "unpriced-image" {
					raw = `{"total_tokens":8,"input_tokens":8,"output_tokens":0,"input_token_details":{"text_tokens":0,"audio_tokens":0,"image_tokens":8},"output_token_details":{"text_tokens":0,"audio_tokens":0}}`
				}
				var usage dto.RealtimeUsage
				require.NoError(t, common.UnmarshalJsonStr(raw, &usage))
				require.True(t, usage.RawUsageObserved)
				require.False(t, usage.UsageIncomplete)
				if scenario == "unknown" {
					require.Error(t, RecordRealtimeUsageCheckpoint(ctx, info, nil))
					estimate := usage
					estimate.RawUsageObserved = false
					require.Error(t, RecordRealtimeUsageCheckpoint(ctx, info, &estimate))
					estimate = usage
					estimate.UsageIncomplete = true
					require.Error(t, RecordRealtimeUsageCheckpoint(ctx, info, &estimate))
					assert.Nil(t, info.RealtimeCheckpoint)
					return
				}
				if scenario == "unpriced-image" {
					require.Error(t, RecordRealtimeUsageCheckpoint(ctx, info, &usage))
					assert.Nil(t, info.RealtimeCheckpoint)
					assert.True(t, FinalizeRealtimeUsageDispatch(ctx, info))
					uq, remain, used := loadPostConsumeBalances(t, db, user, token)
					assert.Equal(t, 900, uq)
					assert.Equal(t, 900, remain)
					assert.Equal(t, 100, used)
					return
				}
				if scenario == "write-failure" {
					require.NoError(t, db.Callback().Update().Before("gorm:update").Register("checkpoint-write-fail", func(tx *gorm.DB) {
						if tx.Statement.Schema != nil && (tx.Statement.Schema.Table == "legacy_usage_reservations" || tx.Statement.Schema.Table == "account_quota_terminal_recovery_obligations") {
							tx.AddError(model.ErrAccountQuotaMutationCASLost)
						}
					}))
					require.Error(t, RecordRealtimeUsageCheckpoint(ctx, info, &usage))
					require.NotNil(t, info.RealtimeCheckpoint)
					assert.Equal(t, 8, info.RealtimeCheckpoint.Total)
					require.NoError(t, db.Callback().Update().Remove("checkpoint-write-fail"))
				} else {
					for range 2 {
						require.NoError(t, RecordRealtimeTieredResponse(info, &usage))
						require.NoError(t, RecordRealtimeUsageCheckpoint(ctx, info, &usage))
					}
				}
				expectedQuota, expectedTokens, responses := 1, 16, 2
				if scenario == "tiered" {
					expectedQuota = 14
				} // Two short contexts, cache included once: (6+2*.5)*2.
				if scenario == "zero" {
					expectedQuota, expectedTokens = 0, 0
				}
				if scenario == "write-failure" {
					expectedQuota, expectedTokens, responses = 8, 8, 1
				}
				require.NotNil(t, info.RealtimeCheckpoint)
				assert.Equal(t, expectedQuota, info.RealtimeCheckpoint.Quota, "normal pricing rounds cumulative .4+.4 once, not each segment")
				assert.Equal(t, expectedTokens, info.RealtimeCheckpoint.Total)
				assert.Equal(t, responses, info.RealtimeCheckpoint.Responses)
				assert.True(t, FinalizeRealtimeUsageDispatch(ctx, info), "unknown tail must keep the reservation")
				require.Error(t, RecordRealtimeUsageCheckpoint(ctx, info, &usage), "late automatic checkpoint cannot overwrite held evidence")
				view, err := model.GetUsageReview(context.Background(), db, user.Id, info.RequestId)
				require.NoError(t, err)
				assert.Nil(t, view.ActualQuota)
				var saved struct {
					Checkpoint hosttypes.RealtimeUsageCheckpoint `json:"realtime_usage_checkpoint"`
					Quota      int                               `json:"known_realtime_quota"`
				}
				require.NoError(t, common.UnmarshalJsonStr(view.ReviewMetadata, &saved))
				assert.Equal(t, *info.RealtimeCheckpoint, saved.Checkpoint)
				assert.Equal(t, expectedQuota, saved.Quota)
				uq, remain, used := loadPostConsumeBalances(t, db, user, token)
				assert.Equal(t, 900, uq)
				assert.Equal(t, 900, remain)
				assert.Equal(t, 100, used)
			})
		}
	}
}
