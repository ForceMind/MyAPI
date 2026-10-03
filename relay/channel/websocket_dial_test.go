package channel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cancelledRealtimeDialAdaptor struct {
	Adaptor
	url          string
	preparations *atomic.Int32
}

func (a cancelledRealtimeDialAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	if a.preparations != nil {
		a.preparations.Add(1)
	}
	return a.url, nil
}
func (a cancelledRealtimeDialAdaptor) SetupRequestHeader(_ *gin.Context, headers *http.Header, _ *relaycommon.RelayInfo) error {
	if a.preparations != nil {
		a.preparations.Add(1)
	}
	headers.Set("X-Trace", "adapter-value")
	return nil
}

func TestCancelledRealtimeHandshakeDoesNotContactUpstream(t *testing.T) {
	var requests, preparations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(server.Close)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil).WithContext(cancelled)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	conn, err := DoWssRequest(cancelledRealtimeDialAdaptor{url: "ws" + strings.TrimPrefix(server.URL, "http"), preparations: &preparations}, ctx, info, nil)
	if conn != nil {
		_ = conn.Close()
	}
	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, requests.Load())
	assert.Zero(t, preparations.Load(), "cancellation precedes URL/header preparation too")
}

func TestRealtimeHandshakeCancellationClosesOpenedTransport(t *testing.T) {
	for _, mode := range []string{"direct", "proxy", "tls", "custom-tls"} {
		t.Run(mode, func(t *testing.T) {
			started := make(chan struct{})
			observedCancel := make(chan bool, 1)
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				select {
				case <-r.Context().Done():
					observedCancel <- true
				case <-release:
					observedCancel <- false
					http.Error(w, "synthetic refusal", http.StatusBadGateway)
				}
			})
			server := httptest.NewUnstartedServer(handler)
			if strings.Contains(mode, "tls") {
				server.StartTLS()
			} else {
				server.Start()
			}
			t.Cleanup(func() { unblock(); server.Close() })
			base := *websocket.DefaultDialer
			base.Proxy = nil
			target := "ws" + strings.TrimPrefix(server.URL, "http")
			if mode == "proxy" {
				proxyURL, err := url.Parse(server.URL)
				require.NoError(t, err)
				base.Proxy = http.ProxyURL(proxyURL)
				target = "ws://unused.invalid/realtime" // Only the loopback CONNECT proxy is contacted.
			}
			if strings.Contains(mode, "tls") {
				roots := x509.NewCertPool()
				roots.AddCert(server.Certificate())
				base.TLSClientConfig = &tls.Config{RootCAs: roots}
				if mode == "custom-tls" {
					tlsDialer := &tls.Dialer{Config: base.TLSClientConfig}
					base.NetDialTLSContext = tlsDialer.DialContext
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			result := make(chan error, 1)
			go func() {
				conn, err := dialWebsocketWithContext(ctx, &base, target, nil)
				if conn != nil {
					_ = conn.Close()
				}
				result <- err
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				unblock()
				t.Fatal("handshake did not reach controlled peer")
			}
			cancel()
			select {
			case err := <-result:
				assert.ErrorIs(t, err, context.Canceled)
			case <-time.After(5 * time.Second):
				unblock()
				<-result
				t.Fatal("cancellation did not interrupt opened handshake")
			}
			select {
			case closed := <-observedCancel:
				assert.True(t, closed, "peer must see cancellation, not a forced test release")
			case <-time.After(5 * time.Second):
				unblock()
				t.Fatal("upstream transport stayed open")
			}
		})
	}
}

func TestRealtimeSuccessfulHandshakeTransfersConnectionOwnership(t *testing.T) {
	seenHeader := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHeader <- r.Header.Get("X-Trace")
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		kind, payload, err := conn.ReadMessage()
		if err == nil {
			_ = conn.WriteMessage(kind, payload)
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	base := *websocket.DefaultDialer
	base.Proxy = nil
	var calls atomic.Int32
	base.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		calls.Add(1)
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	conn, err := dialWebsocketWithContext(ctx, &base, "ws"+strings.TrimPrefix(server.URL, "http"), http.Header{"X-Trace": []string{"retained"}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	assert.EqualValues(t, 1, calls.Load(), "configured custom dialer is preserved")
	assert.Equal(t, "retained", <-seenHeader)
	// The caller owns the established socket; only the existing realtime handler
	// may decide how its later request cancellation closes that live stream.
	cancel()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("synthetic echo")))
	_, echoed, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "synthetic echo", string(echoed))
}

func TestRealtimeHandshakePreservesFinalHeaderOverride(t *testing.T) {
	seen := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("X-Trace")
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(server.Close)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{HeadersOverride: map[string]any{"X-Trace": "final-value"}}}
	conn, err := DoWssRequest(cancelledRealtimeDialAdaptor{url: "ws" + strings.TrimPrefix(server.URL, "http")}, ctx, info, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	assert.Equal(t, "final-value", <-seen)
}

func TestRealtimeLegacyCustomDialCancellationClosesBeforeUpgrade(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected handshake", http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	base := *websocket.DefaultDialer
	base.Proxy = nil
	base.NetDialContext = nil
	base.NetDial = func(network, address string) (net.Conn, error) {
		conn, err := net.Dial(network, address)
		cancel() // Legacy callback has no Context parameter; caller still cancels.
		return conn, err
	}
	conn, err := dialWebsocketWithContext(ctx, &base, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if conn != nil {
		_ = conn.Close()
	}
	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, requests.Load())
}

type cancelAfterHandshakeConn struct {
	net.Conn
	cancel context.CancelFunc
}

func (c cancelAfterHandshakeConn) SetDeadline(deadline time.Time) error {
	err := c.Conn.SetDeadline(deadline)
	if deadline.IsZero() {
		c.cancel()
	} // Gorilla clears this only after validating HTTP 101.
	return err
}

func TestRealtimeAcceptedHandshakeIsNotReclassifiedAsNoDispatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	base := *websocket.DefaultDialer
	base.Proxy = nil
	base.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return cancelAfterHandshakeConn{Conn: conn, cancel: cancel}, nil
	}
	conn, err := dialWebsocketWithContext(ctx, &base, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if conn != nil {
		t.Cleanup(func() { _ = conn.Close() })
	}
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	assert.NoError(t, err, "known accepted handshake must still reach existing session finalization")
	assert.NotNil(t, conn, "do not turn a known accepted connection into the pre-dispatch refund path")
}
