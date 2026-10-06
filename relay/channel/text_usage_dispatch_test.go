package channel

import (
	"bytes"
	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrdinaryTextTransportTracksUncertainSuccessAndDisablesReplay(t *testing.T) {
	service.InitHttpClient()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, `not-json`)
	}))
	defer upstream.Close()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions?trace=fixture", nil)
	request, err := http.NewRequest("POST", upstream.URL, bytes.NewReader([]byte(`{"model":"synthetic"}`)))
	require.NoError(t, err)
	require.NotNil(t, request.GetBody)
	info := textDispatchTransportFixture(t, ctx)
	response, err := doRequest(ctx, request, info)
	require.NoError(t, err)
	defer response.Body.Close()
	assert.EqualValues(t, 1, calls.Load())
	assert.Nil(t, request.GetBody)
	assert.True(t, service.TextUsageDispatchNeedsReview(info), "a 200 response is not proof of parseable final usage")
}

func TestOrdinaryTextTransportDoesNotReplayAnAmbiguousIdempotentPost(t *testing.T) {
	service.InitHttpClient()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/warm" {
			_, _ = io.WriteString(w, "warm")
			return
		}
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = connection.Close()
		}
	}))
	defer upstream.Close()
	// A reused connection plus an idempotency header would otherwise allow the
	// transport to replay a POST whose body may already have been processed.
	response, err := service.GetHttpClient().Get(upstream.URL + "/warm")
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, response.Body)
	require.NoError(t, response.Body.Close())
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	request, err := http.NewRequest("POST", upstream.URL, bytes.NewReader([]byte(`{"model":"synthetic"}`)))
	require.NoError(t, err)
	request.Header.Set("Idempotency-Key", "synthetic-dispatch")
	info := textDispatchTransportFixture(t, ctx)
	_, err = doRequest(ctx, request, info)
	require.Error(t, err)
	assert.EqualValues(t, 1, calls.Load())
	assert.Nil(t, request.GetBody)
	assert.True(t, service.TextUsageDispatchNeedsReview(info))
}

// A real isolated billing session is necessary now that sending requires durable
// evidence. No external provider, credential or real account is used.
func textDispatchTransportFixture(t *testing.T, ctx *gin.Context) *relaycommon.RelayInfo {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "dispatch.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.AssignedAccessPolicy{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.LegacyUsageReservation{}, &model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{}, &model.AccountQuotaTerminalRecoveryObligation{}, &model.AccountQuotaSettlementIntent{}, &model.AccountQuotaSettlementFact{}, &model.AccountQuotaRefundFact{}, &model.QuotaWriterEpoch{}, &model.QuotaProjectionObligation{}, &model.QuotaWorkCursor{}, &model.QuotaBalanceBatchDrain{}, &model.QuotaBalanceBatchSubject{}, &model.Log{}, &model.Channel{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	require.True(t, model.RefreshAccountQuotaSettlementIntentSchemaCapability(db))
	require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
	require.NoError(t, db.Model(&model.QuotaWriterEpoch{}).Where("id = ?", 1).Updates(map[string]any{"mode": string(model.QuotaWriterModeAuthoritative), "epoch": int64(31), "lock_version": gorm.Expr("lock_version + ?", 1)}).Error)
	oldDB, oldLog, oldRedis := model.DB, model.LOG_DB, common.RedisEnabled
	model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
	model.InitColumnNamesForTest()
	t.Cleanup(func() { model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLog, oldRedis })
	user := model.User{Username: "dispatch", AffCode: "dispatch", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000, AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "synthetic-dispatch", Status: common.TokenStatusEnabled, RemainQuota: 1000, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	info := &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, RequestId: "synthetic-dispatch", OriginModelName: "fixture", UserQuota: 1000, StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{}, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
	session, apiErr := service.NewBillingSession(ctx, info, 100)
	require.Nil(t, apiErr)
	info.Billing = session
	t.Cleanup(func() { service.FinalizeTextUsageDispatch(ctx, info) })
	return info
}

func TestOrdinaryTextTransportDoesNotSendWithoutDurableEvidence(t *testing.T) {
	service.InitHttpClient()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
	defer upstream.Close()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	info := textDispatchTransportFixture(t, ctx)
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("synthetic-dispatch-write-failure", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "account_quota_terminal_recovery_obligations" {
			tx.AddError(model.ErrAccountQuotaMutationCASLost)
		}
	}))
	request, err := http.NewRequest("POST", upstream.URL, bytes.NewReader([]byte("synthetic")))
	require.NoError(t, err)
	_, err = doRequest(ctx, request, info)
	require.Error(t, err)
	assert.Zero(t, calls.Load())
	require.NoError(t, model.DB.Callback().Update().Remove("synthetic-dispatch-write-failure"))
	assert.True(t, service.FinalizeTextUsageDispatch(ctx, info))
	require.ErrorIs(t, info.Billing.Refund(ctx), model.ErrAccountQuotaUsageUnresolved)
}

func TestNativeTextTransportDoesNotReplayOrRefundAcceptedPost(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1beta/models/gemini-fixture:generateContent"} {
		t.Run(path, func(t *testing.T) {
			service.InitHttpClient()
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, "warm")
					return
				}
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				connection, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = connection.Close()
				}
			}))
			defer upstream.Close()
			warm, err := service.GetHttpClient().Get(upstream.URL)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, warm.Body)
			require.NoError(t, warm.Body.Close())
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", path+"?trace=fixture", nil)
			info := textDispatchTransportFixture(t, ctx)
			if path != "/v1/messages" {
				info.Request = &dto.GeminiChatRequest{}
			}
			request, err := http.NewRequest("POST", upstream.URL, bytes.NewReader([]byte("synthetic")))
			require.NoError(t, err)
			request.Header.Set("Idempotency-Key", "synthetic-native-dispatch")
			_, err = doRequest(ctx, request, info)
			require.Error(t, err)
			assert.EqualValues(t, 1, calls.Load())
			assert.Nil(t, request.GetBody)
			assert.True(t, service.FinalizeTextUsageDispatch(ctx, info))
			require.ErrorIs(t, info.Billing.Refund(ctx), model.ErrAccountQuotaUsageUnresolved)
		})
	}
}
