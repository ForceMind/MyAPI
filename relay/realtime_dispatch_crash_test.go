package relay

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRealtimeDispatchSurvivesProcessExit(t *testing.T) {
	if os.Getenv("MYAPI_REALTIME_CRASH_CHILD") == "1" {
		scenario := os.Getenv("MYAPI_REALTIME_SCENARIO")
		complete := scenario == "complete"
		db, err := gorm.Open(sqlite.Open(os.Getenv("MYAPI_REALTIME_CRASH_DB")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		model.DB, model.LOG_DB = db, db
		common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
		model.InitColumnNamesForTest()
		common.RedisEnabled = false
		common.BatchUpdateEnabled = false
		require.True(t, model.RefreshAccountQuotaSettlementIntentSchemaCapability(db))
		var user model.User
		var token model.Token
		require.NoError(t, db.First(&user).Error)
		require.NoError(t, db.First(&token).Error)
		accepted := make(chan *websocket.Conn, 1)
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if e == nil {
				accepted <- conn
			}
		}))
		client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(downstream.URL, "http"), nil)
		require.NoError(t, err)
		// Keep both fixture endpoints owned until the relay has fully settled.
		// Otherwise the unused client can be collected and close mid-response.
		defer client.Close()
		defer downstream.Close()
		peer := <-accepted
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("GET", "/v1/realtime?model=fixture-model", nil)
		common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
		common.SetContextKey(ctx, constant.ContextKeyChannelId, 1)
		common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, os.Getenv("MYAPI_REALTIME_CRASH_UPSTREAM"))
		common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "fixture-model")
		info := &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, RequestId: "ws-crash-request", OriginModelName: "fixture-model", UserQuota: 1000,
			UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}, RelayMode: relayconstant.RelayModeRealtime, RelayFormat: types.RelayFormatOpenAIRealtime, RequestURLPath: "/v1/realtime?model=fixture-model", ClientWs: peer, StartTime: time.Now(),
			PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, AudioRatio: 1, AudioCompletionRatio: 1, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
		require.NoError(t, info.PriceData.CaptureQuotaUnit(100))
		initial := 100
		if complete {
			initial = 5 // Both completed segments must raise the reservation.
		}
		ctx.Set(common.RequestIdKey, info.RequestId)
		session, apiErr := service.NewBillingSession(ctx, info, initial)
		require.Nil(t, apiErr)
		info.Billing = session
		info.UserQuota = 1 << 30 // Notification snapshot only; SQL quota is 1000.
		done := make(chan *types.NewAPIError, 1)
		go func() { done <- WssHelper(ctx, info) }()
		require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create"}`)))
		if scenario == "known-crash" {
			require.NoError(t, client.SetReadDeadline(time.Now().Add(15*time.Second)))
			for {
				_, frame, err := client.ReadMessage()
				require.NoError(t, err)
				if strings.Contains(string(frame), `"type":"response.done"`) {
					break
				}
			}
			require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create"}`)))
		}
		if complete {
			require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create"}`)))
		}
		select {
		case err := <-done:
			if complete {
				require.Nil(t, err)
				return
			}
			t.Fatalf("relay stopped before crash barrier: %v", err)
		case <-time.After(30 * time.Second):
			t.Fatal("parent did not reach crash barrier")
		}
		return
	}
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, scenario := range []string{"crash", "complete", "known-crash"} {
			t.Run(fmt.Sprintf("%s/%s", mode, scenario), func(t *testing.T) {
				complete := scenario == "complete"
				path := filepath.Join(t.TempDir(), "crash.db")
				db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
				require.NoError(t, err)
				require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.LegacyUsageReservation{}, &model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{}, &model.AccountQuotaTerminalRecoveryObligation{}, &model.AccountQuotaSettlementIntent{}, &model.AccountQuotaSettlementFact{}, &model.AccountQuotaRefundFact{}, &model.QuotaWriterEpoch{}, &model.QuotaProjectionObligation{}, &model.QuotaWorkCursor{}, &model.UsageReviewDecision{}, &model.TokenBudget{}, &model.TokenBudgetReservation{}, &model.TokenBudgetPolicyChange{}))
				require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
				require.NoError(t, db.Model(&model.QuotaWriterEpoch{}).Where("id = ?", 1).Updates(map[string]any{"mode": string(mode), "epoch": 17}).Error)
				user := model.User{Username: "crash-user", AffCode: "crash-user", Status: common.UserStatusEnabled, Quota: 1000}
				require.NoError(t, db.Create(&user).Error)
				token := model.Token{UserId: user.Id, Key: "crash-fixture-key", Status: common.TokenStatusEnabled, RemainQuota: 1000, ExpiredTime: -1}
				require.NoError(t, db.Create(&token).Error)
				require.NoError(t, db.Create(&model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Name: "isolated-realtime"}).Error)
				pool, err := db.DB()
				require.NoError(t, err)
				require.NoError(t, pool.Close())
				received := make(chan string, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
					if err != nil {
						return
					}
					defer conn.Close()
					_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
					_, message, err := conn.ReadMessage()
					if err != nil {
						received <- "read-error:" + err.Error()
						return
					}
					if scenario == "known-crash" {
						created := `{"type":"response.created","response":{"id":"known","status":"in_progress"}}`
						done := `{"type":"response.done","response":{"id":"known","status":"completed","usage":{"total_tokens":8,"input_tokens":8,"output_tokens":0,"input_token_details":{"text_tokens":8,"audio_tokens":0},"output_token_details":{"text_tokens":0,"audio_tokens":0}}}}`
						if conn.WriteMessage(websocket.TextMessage, []byte(created)) != nil || conn.WriteMessage(websocket.TextMessage, []byte(done)) != nil {
							return
						}
						_, message, err = conn.ReadMessage()
						if err != nil {
							received <- "read-error:" + err.Error()
							return
						}
					}
					received <- string(message)
					_, _, err = conn.ReadMessage()
					if !complete || err != nil {
						return
					}
					for index, count := range []int{8, 12} {
						created := fmt.Sprintf(`{"type":"response.created","response":{"id":"r%d","status":"in_progress"}}`, index)
						done := fmt.Sprintf(`{"type":"response.done","response":{"id":"r%d","status":"completed","usage":{"total_tokens":%d,"input_tokens":%d,"output_tokens":0,"input_token_details":{"text_tokens":%d,"audio_tokens":0},"output_token_details":{"text_tokens":0,"audio_tokens":0}}}}`, index, count, count, count)
						if conn.WriteMessage(websocket.TextMessage, []byte(created)) != nil || conn.WriteMessage(websocket.TextMessage, []byte(done)) != nil {
							return
						}
					}
					_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				}))
				defer server.Close()
				cmd := exec.Command(os.Args[0], "-test.run=^TestRealtimeDispatchSurvivesProcessExit$", "-test.v")
				cmd.Env = append(os.Environ(), "MYAPI_REALTIME_CRASH_CHILD=1", "MYAPI_REALTIME_CRASH_DB="+path, "MYAPI_REALTIME_CRASH_UPSTREAM="+server.URL)
				cmd.Env = append(cmd.Env, "MYAPI_REALTIME_SCENARIO="+scenario)
				var output bytes.Buffer
				cmd.Stdout = &output
				cmd.Stderr = &output
				require.NoError(t, cmd.Start())
				killed := false
				defer func() {
					if !killed {
						_ = cmd.Process.Kill()
						_ = cmd.Wait()
					}
				}()
				var message string
				select {
				case message = <-received:
				case <-time.After(15 * time.Second):
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					killed = true
					t.Fatalf("no upstream frame: %s", output.String())
				}
				if complete {
					err = cmd.Wait()
					killed = true
					require.NoError(t, err, "%s", output.String())
				} else {
					require.NoError(t, cmd.Process.Kill())
					_ = cmd.Wait()
				}
				killed = true
				require.JSONEq(t, `{"type":"response.create"}`, message)
				reopened, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
				require.NoError(t, err)
				freshPool, err := reopened.DB()
				require.NoError(t, err)
				defer freshPool.Close()
				if complete {
					require.NoError(t, reopened.First(&user, user.Id).Error)
					require.NoError(t, reopened.First(&token, token.Id).Error)
					assert.Equal(t, 980, user.Quota, "%s", output.String())
					assert.Equal(t, 980, token.RemainQuota)
					assert.Equal(t, 20, token.UsedQuota)
					if mode == model.QuotaWriterModeLegacy {
						row, err := model.FindLegacyUsageReservation(context.Background(), reopened, "ws-crash-request")
						require.NoError(t, err)
						assert.Equal(t, model.LegacyUsageSettled, row.State)
						require.NotNil(t, row.ActualQuota)
						assert.EqualValues(t, 20, *row.ActualQuota)
					} else {
						var terminal model.AccountQuotaMutationReceipt
						require.NoError(t, reopened.Where("request_id = ? AND phase = ?", "ws-crash-request", model.AccountQuotaPhaseSettle).First(&terminal).Error)
						assert.EqualValues(t, 20, terminal.RequestedQuota)
					}
					return
				}
				var metadata string
				if mode == model.QuotaWriterModeLegacy {
					row, err := model.FindLegacyUsageReservation(context.Background(), reopened, "ws-crash-request")
					require.NoError(t, err)
					assert.EqualValues(t, 100, row.ReservedQuota)
					assert.Nil(t, row.ActualQuota)
					metadata = row.ReviewMetadata
				} else {
					var row model.AccountQuotaTerminalRecoveryObligation
					require.NoError(t, reopened.Where("request_id = ?", "ws-crash-request").First(&row).Error)
					metadata = row.ReviewMetadata
				}
				pending, err := model.TextDispatchPending(metadata)
				require.NoError(t, err)
				assert.True(t, pending, "confirmed forwarded generation must leave durable uncertainty before abrupt process exit")
				if mode == model.QuotaWriterModeLegacy {
					fact, e := model.EnsureAccountQuotaRefundFact(context.Background(), reopened, model.AccountQuotaRefundFactInput{EventKey: "crash-refund", Kind: model.AccountQuotaRefundFactKindLegacyWallet, RequestID: "ws-crash-request", UserID: user.Id, TokenID: token.Id, WalletQuota: 100, TokenQuota: 100})
					if e == nil {
						_, _, e = model.RecoverAccountQuotaRefundFact(context.Background(), reopened, fact, "crash-fixture")
					}
					assert.ErrorIs(t, e, model.ErrAccountQuotaUsageUnresolved, "fresh process cannot refund forwarded generation without usage proof")
				} else {
					var row model.AccountQuotaTerminalRecoveryObligation
					require.NoError(t, reopened.Where("request_id = ?", "ws-crash-request").First(&row).Error)
					_, e := model.RefundAccountQuota(context.Background(), reopened, model.AccountQuotaTerminalInput{RequestID: "ws-crash-request", ReserveReceiptID: row.ReserveReceiptID, AuditKey: "crash-fixture"})
					assert.ErrorIs(t, e, model.ErrAccountQuotaUsageUnresolved, "fresh process cannot refund forwarded generation without usage proof")
				}
				require.NoError(t, reopened.First(&user, user.Id).Error)
				require.NoError(t, reopened.First(&token, token.Id).Error)
				assert.Equal(t, 900, user.Quota)
				assert.Equal(t, 900, token.RemainQuota)

				if scenario == "known-crash" {
					var evidence struct {
						Checkpoint hosttypes.RealtimeUsageCheckpoint `json:"realtime_usage_checkpoint"`
						Quota      int                               `json:"known_realtime_quota"`
					}
					require.NoError(t, common.UnmarshalJsonStr(metadata, &evidence))
					assert.Equal(t, hosttypes.RealtimeUsageCheckpoint{Responses: 1, Quota: 8, Input: 8, Total: 8, InputText: 8}, evidence.Checkpoint)
					assert.Equal(t, 8, evidence.Quota)
					oldDB, oldLogDB := model.DB, model.LOG_DB
					model.DB, model.LOG_DB = reopened, reopened
					defer func() { model.DB, model.LOG_DB = oldDB, oldLogDB }()
					require.True(t, model.RefreshAccountQuotaSettlementIntentSchemaCapability(reopened))
					root := model.User{Username: "known-root", AffCode: "known-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
					require.NoError(t, reopened.Create(&root).Error)
					_, reviewErr := model.RecoverTextDispatchUsage(context.Background(), reopened, root.Id, "ws-crash-request", 0, "synthetic ended session evidence")
					require.ErrorIs(t, reviewErr, model.ErrAccountQuotaMutationConflict, "known work cannot disappear after restart")
					require.NoError(t, reopened.First(&user, user.Id).Error)
					require.NoError(t, reopened.First(&token, token.Id).Error)
					assert.Equal(t, 900, user.Quota)
					assert.Equal(t, 900, token.RemainQuota)
					for range 2 {
						_, reviewErr = model.RecoverTextDispatchUsage(context.Background(), reopened, root.Id, "ws-crash-request", 8, "synthetic final evidence: first response eight, second no consumption")
						require.NoError(t, reviewErr)
					}
					require.NoError(t, reopened.First(&user, user.Id).Error)
					require.NoError(t, reopened.First(&token, token.Id).Error)
					assert.Equal(t, 992, user.Quota)
					assert.Equal(t, 992, token.RemainQuota)
					assert.Equal(t, 8, token.UsedQuota)
					reserveID := int64(0)
					if mode == model.QuotaWriterModeAuthoritative {
						var row model.AccountQuotaTerminalRecoveryObligation
						require.NoError(t, reopened.Where("request_id = ?", "ws-crash-request").First(&row).Error)
						reserveID = row.ReserveReceiptID
					}
					require.Error(t, model.RecordRealtimeUsageCheckpoint(context.Background(), reopened, "ws-crash-request", user.Id, token.Id, reserveID, evidence.Checkpoint), "late checkpoint cannot overwrite manual terminal ownership")

				}
			})
		}
	}
}
