package openai

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
