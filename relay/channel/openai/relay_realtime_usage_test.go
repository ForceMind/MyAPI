package openai

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relay/helper"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func realtimeSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	accepted := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			accepted <- conn
		}
	}))
	t.Cleanup(server.Close)
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	peer := <-accepted
	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, peer.SetReadDeadline(time.Now().Add(5*time.Second)))
	t.Cleanup(func() { _ = client.Close(); _ = peer.Close() })
	return client, peer
}

type realtimeFailingReservation struct {
	calls int
	fail  bool
}

type observedRealtimeCredentialError struct {
	once     sync.Once
	observed chan struct{}
}

func (e *observedRealtimeCredentialError) Error() string {
	e.once.Do(func() { close(e.observed) })
	return "synthetic credential rotation"
}

func (*realtimeFailingReservation) Settle(int) error          { return nil }
func (*realtimeFailingReservation) Refund(*gin.Context) error { return nil }
func (*realtimeFailingReservation) NeedsRefund() bool         { return false }
func (*realtimeFailingReservation) GetPreConsumedQuota() int  { return 100 }
func (s *realtimeFailingReservation) Reserve(int) error {
	s.calls++
	if s.fail {
		return errors.New("synthetic reservation failure")
	}
	return nil
}

func TestRealtimeReservationFailureCountsReportedUsageOnce(t *testing.T) {
	_, downstream := realtimeSocketPair(t)
	upstream, provider := realtimeSocketPair(t)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	settler := &realtimeFailingReservation{fail: true}
	info := &relaycommon.RelayInfo{ClientWs: downstream, TargetWs: upstream, StartTime: time.Now(), Billing: settler,
		PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, AudioRatio: 1, AudioCompletionRatio: 1,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
	require.NoError(t, info.PriceData.CaptureQuotaUnit(100))
	payload, err := common.Marshal(dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseDone,
		Response: &dto.RealtimeResponse{Usage: &dto.RealtimeUsage{TotalTokens: 8, InputTokens: 8,
			InputTokenDetails: dto.InputTokenDetails{TextTokens: 8}}}})
	require.NoError(t, err)
	require.NoError(t, provider.WriteMessage(websocket.TextMessage, payload))
	apiErr, usage := OpenaiRealtimeHandler(ctx, info)
	require.Nil(t, apiErr, "known usage remains available to final settlement")
	require.NotNil(t, usage)
	assert.Equal(t, 8, usage.TotalTokens)
	assert.Equal(t, 8, usage.InputTokens)
	assert.Equal(t, 8, usage.InputTokenDetails.TextTokens)
	assert.Equal(t, 1, settler.calls, "failed segment must not be retried again by the tail flush")
}

func TestRealtimeCancellationStopsBothReaders(t *testing.T) {
	_, downstream := realtimeSocketPair(t)
	upstream, _ := realtimeSocketPair(t)
	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil).WithContext(requestCtx)
	info := &relaycommon.RelayInfo{ClientWs: downstream, TargetWs: upstream, StartTime: time.Now()}
	apiErr, usage := OpenaiRealtimeHandler(ctx, info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Zero(t, usage.TotalTokens)
	_, clientErr := downstream.UnderlyingConn().Read(make([]byte, 1))
	_, upstreamErr := upstream.UnderlyingConn().Read(make([]byte, 1))
	assert.ErrorIs(t, clientErr, net.ErrClosed)
	assert.ErrorIs(t, upstreamErr, net.ErrClosed)
}

func TestRealtimeCredentialRotationBlocksNewClientFramesButForwardsProviderFrames(t *testing.T) {
	priorDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = priorDB })
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	token := model.Token{UserId: 81, Key: "realtime-generation", CredentialVersion: 3}
	require.NoError(t, db.Create(&token).Error)

	client, downstream := realtimeSocketPair(t)
	upstream, provider := realtimeSocketPair(t)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	info := &relaycommon.RelayInfo{
		ClientWs: downstream, TargetWs: upstream, StartTime: time.Now(),
		UserId: token.UserId, TokenId: token.Id, TokenCredentialVersion: 3,
	}
	finished := make(chan struct{})
	go func() {
		OpenaiRealtimeHandler(ctx, info)
		close(finished)
	}()

	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("credential_version", 4).Error)
	providerEvent := []byte(`{"type":"session.created","session":{}}`)
	require.NoError(t, provider.WriteMessage(websocket.TextMessage, providerEvent))
	_, forwarded, err := client.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, providerEvent, forwarded, "provider output accepted before rotation remains deliverable")

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"session.update","session":{}}`)))
	_, _, err = provider.ReadMessage()
	require.Error(t, err, "a frame from the old API-key generation must not reach the provider")
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("realtime handler did not stop after rejecting the stale client frame")
	}
}

func TestRealtimeCredentialRotationDrainsAcceptedResponseAndUsage(t *testing.T) {
	client, downstream := realtimeSocketPair(t)
	upstream, provider := realtimeSocketPair(t)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	settler := &realtimeFailingReservation{}
	info := &relaycommon.RelayInfo{ClientWs: downstream, TargetWs: upstream, StartTime: time.Now(), Billing: settler,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "fixture-model"},
		PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, AudioRatio: 1, AudioCompletionRatio: 1,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
	require.NoError(t, info.PriceData.CaptureQuotaUnit(100))

	credentialObserved := make(chan struct{})
	credentialErr := &observedRealtimeCredentialError{observed: credentialObserved}
	var validations atomic.Int32
	validator := func(context.Context, *relaycommon.RelayInfo) error {
		if validations.Add(1) == 1 {
			return nil
		}
		return credentialErr
	}
	forwardEntered := make(chan struct{})
	releaseForward := make(chan struct{})
	drainInspected := make(chan bool, 1)
	forwardMessage := func(c *gin.Context, destination *websocket.Conn, message string) error {
		if strings.Contains(message, `"type":"response.done"`) {
			close(forwardEntered)
			<-releaseForward
		}
		return helper.WssString(c, destination, message)
	}
	finished := make(chan *dto.RealtimeUsage, 1)
	go func() {
		_, usage := openaiRealtimeHandler(ctx, info, realtimeHandlerOptions{
			credentialValidator: validator,
			forwardMessage:      forwardMessage,
			drainInspected: func(drained bool) {
				drainInspected <- drained
			},
		})
		finished <- usage
	}()

	accepted := []byte(`{"type":"response.create"}`)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, accepted))
	_, forwarded, err := provider.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, accepted, forwarded)
	created := []byte(`{"type":"response.created","response":{"id":"accepted","status":"in_progress"}}`)
	require.NoError(t, provider.WriteMessage(websocket.TextMessage, created))
	_, forwarded, err = client.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, created, forwarded)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"session.update","session":{}}`)))
	<-credentialObserved // Error formatting occurs after the handler enters credential-drain mode.
	done := []byte(`{"type":"response.done","response":{"id":"accepted","status":"completed","usage":{"total_tokens":8,"input_tokens":8,"output_tokens":0,"input_token_details":{"text_tokens":8}}}}`)
	require.NoError(t, provider.WriteMessage(websocket.TextMessage, done))
	<-forwardEntered
	require.False(t, <-drainInspected, "a handled target event remains in flight until it is forwarded successfully")
	close(releaseForward)
	_, forwarded, err = client.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, done, forwarded)

	usage := <-finished
	require.NotNil(t, usage)
	require.Equal(t, 8, usage.TotalTokens)
	require.Equal(t, 8, usage.InputTokens)
	require.False(t, usage.UsageIncomplete)
	require.Equal(t, 1, settler.calls)
	_, _, err = provider.ReadMessage()
	require.Error(t, err, "the stale client frame must not reach the provider")
}

func TestRealtimeCredentialRotationSignalsDrainAfterConcurrentTargetForward(t *testing.T) {
	client, downstream := realtimeSocketPair(t)
	upstream, provider := realtimeSocketPair(t)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	settler := &realtimeFailingReservation{}
	info := &relaycommon.RelayInfo{ClientWs: downstream, TargetWs: upstream, StartTime: time.Now(), Billing: settler,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "fixture-model"},
		PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, AudioRatio: 1, AudioCompletionRatio: 1,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
	require.NoError(t, info.PriceData.CaptureQuotaUnit(100))

	credentialObserved := make(chan struct{})
	credentialErr := &observedRealtimeCredentialError{observed: credentialObserved}
	var validations atomic.Int32
	validator := func(context.Context, *relaycommon.RelayInfo) error {
		if validations.Add(1) == 1 {
			return nil
		}
		return credentialErr
	}
	targetForwarded := make(chan struct{})
	releaseTarget := make(chan struct{})
	drainInspected := make(chan bool, 1)
	finished := make(chan *dto.RealtimeUsage, 1)
	go func() {
		_, usage := openaiRealtimeHandler(ctx, info, realtimeHandlerOptions{
			credentialValidator: validator,
			targetForwarded: func(event *dto.RealtimeEvent) {
				if event.Type == dto.RealtimeEventTypeResponseDone {
					close(targetForwarded)
					<-releaseTarget
				}
			},
			drainInspected: func(drained bool) {
				drainInspected <- drained
			},
		})
		finished <- usage
	}()

	accepted := []byte(`{"type":"response.create"}`)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, accepted))
	_, _, err := provider.ReadMessage()
	require.NoError(t, err)
	created := []byte(`{"type":"response.created","response":{"id":"accepted","status":"in_progress"}}`)
	require.NoError(t, provider.WriteMessage(websocket.TextMessage, created))
	_, _, err = client.ReadMessage()
	require.NoError(t, err)
	done := []byte(`{"type":"response.done","response":{"id":"accepted","status":"completed","usage":{"total_tokens":8,"input_tokens":8,"output_tokens":0,"input_token_details":{"text_tokens":8}}}}`)
	require.NoError(t, provider.WriteMessage(websocket.TextMessage, done))
	<-targetForwarded
	_, forwarded, err := client.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, done, forwarded)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"session.update","session":{}}`)))
	<-credentialObserved
	require.False(t, <-drainInspected, "the main loop must wait while the forwarded target event is still publishing its completion signal")
	close(releaseTarget)

	usage := <-finished
	require.NotNil(t, usage)
	require.Equal(t, 8, usage.TotalTokens)
	require.False(t, usage.UsageIncomplete)
	require.Equal(t, 1, settler.calls)
}

func TestRealtimeForwardsReportedResponsesThenStopsReaders(t *testing.T) {
	for _, exit := range []string{"cancel", "client close", "provider close", "invalid JSON", "missing response"} {
		t.Run(exit, func(t *testing.T) {
			client, downstream := realtimeSocketPair(t)
			upstream, provider := realtimeSocketPair(t)
			requestCtx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil).WithContext(requestCtx)
			settler := &realtimeFailingReservation{}
			info := &relaycommon.RelayInfo{ClientWs: downstream, TargetWs: upstream, StartTime: time.Now(), Billing: settler,
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "fixture-model"},
				PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, AudioRatio: 1, AudioCompletionRatio: 1,
					GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
			expression := `len <= 10 ? tier("short", p) : tier("long", p * 10)`
			info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expression,
				ExprHash: billingexpr.ExprHashString(expression), QuotaPerUnit: 1_000_000, GroupRatio: 1}
			require.NoError(t, info.PriceData.CaptureQuotaUnit(100))
			finished := make(chan *dto.RealtimeUsage, 1)
			finishedWithoutError := make(chan bool, 1)
			go func() {
				apiErr, usage := OpenaiRealtimeHandler(ctx, info)
				finishedWithoutError <- apiErr == nil
				finished <- usage
			}()

			// Both directions are active; acknowledgements prove forwarding completed
			// before the reported responses and cancellation, with no timing sleeps.
			clientMessage, err := common.Marshal(dto.RealtimeEvent{Type: dto.RealtimeEventTypeSessionUpdate,
				Session: &dto.RealtimeSession{InputAudioFormat: "pcm16"}})
			require.NoError(t, err)
			require.NoError(t, client.WriteMessage(websocket.TextMessage, clientMessage))
			_, forwardedClient, err := provider.ReadMessage()
			require.NoError(t, err)
			assert.Equal(t, clientMessage, forwardedClient)
			payload, err := common.Marshal(dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseDone,
				Response: &dto.RealtimeResponse{Usage: &dto.RealtimeUsage{TotalTokens: 10, InputTokens: 8, OutputTokens: 2,
					InputTokenDetails: dto.InputTokenDetails{TextTokens: 8, CachedTokens: 2,
						CachedTokensDetails: dto.NewCachedTokenDetails(2, 0, 0)}, OutputTokenDetails: dto.OutputTokenDetails{TextTokens: 2}}}})
			require.NoError(t, err)
			for range 2 { // two distinct responses with the same usage, not a replay
				require.NoError(t, provider.WriteMessage(websocket.TextMessage, payload))
				_, forwarded, err := client.ReadMessage()
				require.NoError(t, err)
				assert.Equal(t, payload, forwarded)
			}
			switch exit {
			case "cancel":
				cancel()
			case "client close":
				require.NoError(t, client.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(5*time.Second)))
			case "provider close":
				require.NoError(t, provider.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(5*time.Second)))
			case "invalid JSON":
				require.NoError(t, provider.WriteMessage(websocket.TextMessage, []byte(`{invalid`)))
			case "missing response":
				require.NoError(t, provider.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.done"}`)))
			}
			usage := <-finished
			require.True(t, <-finishedWithoutError, "known usage must reach final settlement instead of triggering a full refund")
			require.NotNil(t, usage)
			assert.Equal(t, 20, usage.TotalTokens)
			assert.Equal(t, 16, usage.InputTokens)
			assert.Equal(t, 4, usage.OutputTokens)
			assert.Equal(t, 16, usage.InputTokenDetails.TextTokens)
			assert.Equal(t, 4, usage.InputTokenDetails.CachedTokens)
			require.NotNil(t, usage.InputTokenDetails.CachedTokensDetails)
			assert.Equal(t, 4, *usage.InputTokenDetails.CachedTokensDetails.TextTokens)
			assert.Equal(t, 4, usage.OutputTokenDetails.TextTokens)
			assert.Equal(t, 2, settler.calls)
			require.NotNil(t, info.RealtimeTieredPricing)
			assert.Equal(t, 16, info.RealtimeTieredPricing.Quota)
			assert.Equal(t, 2, info.RealtimeTieredPricing.Responses)
			_, err = downstream.UnderlyingConn().Read(make([]byte, 1))
			assert.ErrorIs(t, err, net.ErrClosed)
			_, err = upstream.UnderlyingConn().Read(make([]byte, 1))
			assert.ErrorIs(t, err, net.ErrClosed)
		})
	}
}

func TestRealtimeCacheAggregationDoesNotFillUnknownPartsOrAliasSource(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{UsePrice: true, PriceData: hosttypes.PriceData{UsePrice: true}}
	total := &dto.RealtimeUsage{}
	first := &dto.RealtimeUsage{TotalTokens: 10, InputTokens: 7, OutputTokens: 3,
		InputTokenDetails: dto.InputTokenDetails{TextTokens: 3, AudioTokens: 2, ImageTokens: 2, CachedTokens: 5,
			CachedTokensDetails: dto.NewCachedTokenDetails(2, 1, 2)},
		OutputTokenDetails: dto.OutputTokenDetails{TextTokens: 1, AudioTokens: 1, ImageTokens: 1}}
	require.NoError(t, preConsumeUsage(ctx, info, first, total))
	*first.InputTokenDetails.CachedTokensDetails.TextTokens = 99
	second := &dto.RealtimeUsage{TotalTokens: 3, InputTokens: 2, OutputTokens: 1,
		InputTokenDetails:  dto.InputTokenDetails{TextTokens: 1, AudioTokens: 1, CachedTokens: 1},
		OutputTokenDetails: dto.OutputTokenDetails{TextTokens: 1}}
	require.NoError(t, preConsumeUsage(ctx, info, second, total))
	assert.Equal(t, 13, total.TotalTokens, "cached subcategories must not add to total tokens")
	assert.Equal(t, 9, total.InputTokens)
	assert.Equal(t, 4, total.OutputTokens)
	assert.Equal(t, 6, total.InputTokenDetails.CachedTokens)
	require.NotNil(t, total.InputTokenDetails.CachedTokensDetails)
	assert.Equal(t, 2, *total.InputTokenDetails.CachedTokensDetails.TextTokens)
	assert.Equal(t, 1, *total.InputTokenDetails.CachedTokensDetails.AudioTokens)
	assert.Equal(t, 2, *total.InputTokenDetails.CachedTokensDetails.ImageTokens)
	assert.Equal(t, 2, total.InputTokenDetails.ImageTokens)
	assert.Equal(t, 1, total.OutputTokenDetails.ImageTokens)
}

func TestRealtimeRawInvalidSegmentKeepsEarlierCountsButPreventsConfirmedSettlement(t *testing.T) {
	for _, raw := range []string{
		`{"type":"response.done","response":{"usage":{}}}`,
		`{"type":"response.done","response":{"usage":{"total_tokens":"invalid"}}}`,
		`{"type":"response.done"}`,
		`{invalid`,
	} {
		t.Run(raw, func(t *testing.T) {
			client, downstream := realtimeSocketPair(t)
			upstream, provider := realtimeSocketPair(t)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
			info := &relaycommon.RelayInfo{ClientWs: downstream, TargetWs: upstream, StartTime: time.Now(), UsePrice: true, PriceData: hosttypes.PriceData{UsePrice: true}}
			finished := make(chan *dto.RealtimeUsage, 1)
			go func() { _, usage := OpenaiRealtimeHandler(ctx, info); finished <- usage }()
			valid := []byte(`{"type":"response.done","response":{"usage":{"total_tokens":8,"input_tokens":8,"output_tokens":0,"input_token_details":{"text_tokens":8}}}}`)
			require.NoError(t, provider.WriteMessage(websocket.TextMessage, valid))
			_, forwarded, err := client.ReadMessage()
			require.NoError(t, err)
			assert.Equal(t, valid, forwarded)
			require.NoError(t, provider.WriteMessage(websocket.TextMessage, []byte(raw)))
			usage := <-finished
			require.NotNil(t, usage)
			assert.Equal(t, 8, usage.InputTokens)
			assert.Equal(t, 8, usage.TotalTokens)
			assert.True(t, usage.UsageIncomplete)
			assert.True(t, info.RealtimeUsageUnverified)
		})
	}
}

func TestRealtimeAggregationRejectsOverflowWithoutPartialCounters(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{UsePrice: true, PriceData: hosttypes.PriceData{UsePrice: true}}
	total := &dto.RealtimeUsage{TotalTokens: 2147483647, InputTokens: 2147483647,
		InputTokenDetails: dto.InputTokenDetails{TextTokens: 2147483647, CachedTokens: 1, CachedTokensDetails: dto.NewCachedTokenDetails(1, 0, 0)}}
	usage := &dto.RealtimeUsage{TotalTokens: 1, InputTokens: 1, InputTokenDetails: dto.InputTokenDetails{TextTokens: 1, CachedTokens: 1, CachedTokensDetails: dto.NewCachedTokenDetails(1, 0, 0)}}
	require.Error(t, preConsumeUsage(ctx, info, usage, total, true))
	assert.Equal(t, 2147483647, total.TotalTokens)
	assert.Equal(t, 2147483647, total.InputTokenDetails.TextTokens)
	assert.Equal(t, 1, *total.InputTokenDetails.CachedTokensDetails.TextTokens)
	assert.True(t, total.UsageIncomplete)
	assert.True(t, info.RealtimeUsageUnverified)
	assert.False(t, info.RealtimeReportedUsage)
}

func TestRealtimePendingResponseCannotBorrowEarlierReportedUsage(t *testing.T) {
	for _, mode := range []string{"client requested", "server started"} {
		t.Run(mode, func(t *testing.T) {
			client, downstream := realtimeSocketPair(t)
			upstream, provider := realtimeSocketPair(t)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
			info := &relaycommon.RelayInfo{ClientWs: downstream, TargetWs: upstream, StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "fixture-model"}, UsePrice: true, PriceData: hosttypes.PriceData{UsePrice: true}}
			finished := make(chan *dto.RealtimeUsage, 1)
			go func() { _, usage := OpenaiRealtimeHandler(ctx, info); finished <- usage }()
			valid := []byte(`{"type":"response.done","response":{"id":"finished-response","usage":{"total_tokens":8,"input_tokens":8,"output_tokens":0,"input_token_details":{"text_tokens":8}}}}`)
			require.NoError(t, provider.WriteMessage(websocket.TextMessage, valid))
			_, _, err := client.ReadMessage()
			require.NoError(t, err)
			if mode == "client requested" {
				require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","event_id":"new-request"}`)))
				_, _, err = provider.ReadMessage()
			} else {
				require.NoError(t, provider.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.created","response":{"id":"unfinished-response","status":"in_progress"}}`)))
				_, _, err = client.ReadMessage()
			}
			require.NoError(t, err, "pending work was actually forwarded")
			require.NoError(t, provider.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(5*time.Second)))
			usage := <-finished
			require.NotNil(t, usage)
			assert.Equal(t, 8, usage.TotalTokens, "do not invent or erase token counts")
			assert.True(t, info.RealtimeUsageUnverified, "earlier completed response does not prove later inference was free")
			assert.True(t, usage.UsageIncomplete)
		})
	}
}

func TestRealtimeLifecycleWireCompletionAndAutomaticAmbiguity(t *testing.T) {
	for _, mode := range []string{"manual complete", "automatic concurrent complete", "automatic concurrent partial", "manual audio disabled", "mixed audio unknown"} {
		t.Run(mode, func(t *testing.T) {
			client, downstream := realtimeSocketPair(t)
			upstream, provider := realtimeSocketPair(t)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
			info := &relaycommon.RelayInfo{ClientWs: downstream, TargetWs: upstream, StartTime: time.Now(), InputAudioFormat: "pcm16",
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "fixture-model"}, UsePrice: true, PriceData: hosttypes.PriceData{UsePrice: true}}
			finished := make(chan *dto.RealtimeUsage, 1)
			go func() { _, usage := OpenaiRealtimeHandler(ctx, info); finished <- usage }()
			detection := `{"type":"server_vad"}`
			if mode == "manual audio disabled" {
				detection = "null"
			}
			session := []byte(`{"type":"session.created","session":{"turn_detection":` + detection + `}}`)
			require.NoError(t, provider.WriteMessage(websocket.TextMessage, session))
			_, forwarded, err := client.ReadMessage()
			require.NoError(t, err)
			assert.Equal(t, session, forwarded)
			if mode != "manual complete" {
				audio := []byte(`{"type":"input_audio_buffer.append","audio":"AAAAAA=="}`)
				require.NoError(t, client.WriteMessage(websocket.TextMessage, audio))
				_, forwarded, err = provider.ReadMessage()
				require.NoError(t, err)
				assert.Equal(t, audio, forwarded)
			}
			concurrent := strings.HasPrefix(mode, "automatic concurrent")
			if !concurrent {
				request := []byte(`{"type":"response.create"}`)
				require.NoError(t, client.WriteMessage(websocket.TextMessage, request))
				_, forwarded, err = provider.ReadMessage()
				require.NoError(t, err)
				assert.Equal(t, request, forwarded)
			}
			ids := []string{"a"}
			if concurrent {
				ids = append(ids, "b")
			}
			for _, id := range ids {
				created := []byte(`{"type":"response.created","response":{"id":"` + id + `","status":"in_progress"}}`)
				require.NoError(t, provider.WriteMessage(websocket.TextMessage, created))
				_, forwarded, err = client.ReadMessage()
				require.NoError(t, err)
				assert.Equal(t, created, forwarded)
			}
			completed := 0
			for index := len(ids) - 1; index >= 0; index-- {
				if mode == "automatic concurrent partial" && completed == 1 {
					break
				}
				done := []byte(`{"type":"response.done","response":{"id":"` + ids[index] + `","status":"completed","usage":{"input_tokens":8,"output_tokens":0,"total_tokens":8,"input_token_details":{"text_tokens":8}}}}`)
				require.NoError(t, provider.WriteMessage(websocket.TextMessage, done))
				_, forwarded, err = client.ReadMessage()
				require.NoError(t, err)
				assert.Equal(t, done, forwarded)
				completed++
			}
			require.NoError(t, provider.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(5*time.Second)))
			usage := <-finished
			require.NotNil(t, usage)
			assert.Equal(t, completed*8, usage.TotalTokens)
			unknown := mode == "automatic concurrent partial" || mode == "mixed audio unknown"
			assert.Equal(t, unknown, info.RealtimeUsageUnverified)
			assert.Equal(t, unknown, usage.UsageIncomplete)
		})
	}
}
