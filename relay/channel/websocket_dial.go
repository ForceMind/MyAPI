package channel

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/gorilla/websocket"
)

// DialContext alone applies deadlines but does not interrupt an already-open
// connection on every cancellation during proxy/TLS/HTTP upgrade negotiation.
// This cancellation hook belongs only to the handshake, not the returned socket.
func dialWebsocketWithContext(ctx context.Context, base *websocket.Dialer, url string, headers http.Header) (*websocket.Conn, error) {
	if ctx == nil || base == nil {
		return nil, errors.New("invalid websocket dial context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dialer := *base // Never change shared proxy, TLS or custom dialing configuration.
	var stopCancellation func() bool
	defer func() {
		if stopCancellation != nil {
			stopCancellation()
		}
	}()
	track := func(conn net.Conn, err error) (net.Conn, error) {
		if err != nil {
			if conn != nil {
				_ = conn.Close()
			}
			return nil, err
		}
		if conn == nil {
			return nil, errors.New("websocket dial returned no connection")
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			_ = conn.Close()
			return nil, ctxErr
		}
		stopCancellation = context.AfterFunc(ctx, func() { _ = conn.Close() })
		return conn, nil
	}
	dial := dialer.NetDialContext
	if dial == nil {
		if legacy := dialer.NetDial; legacy != nil {
			dial = func(_ context.Context, network, address string) (net.Conn, error) { return legacy(network, address) }
		} else {
			dial = (&net.Dialer{}).DialContext
		}
	}
	dialer.NetDialContext = func(callCtx context.Context, network, address string) (net.Conn, error) {
		return track(dial(callCtx, network, address))
	}
	if tlsDial := dialer.NetDialTLSContext; tlsDial != nil {
		dialer.NetDialTLSContext = func(callCtx context.Context, network, address string) (net.Conn, error) {
			return track(tlsDial(callCtx, network, address))
		}
	}
	conn, response, err := dialer.DialContext(ctx, url, headers)
	if err != nil && response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	// HTTP 101 has already been validated. Even if cancellation raced that
	// success, retain accepted-connection evidence for normal session cleanup
	// and billing; do not recast it as a request that never reached upstream.
	if err == nil && conn != nil {
		return conn, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if conn != nil {
			_ = conn.Close()
		}
		return nil, ctxErr
	}
	return conn, err
}
