package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/ForceMind/MyAPI/common"
)

// Tests can admit a specific synthetic transport without opening a production
// configuration path for transports whose retry behavior cannot be inspected.
var upstreamAcceptanceTestTransportAllowed func(http.RoundTripper) bool

// DoUpstreamAcceptanceHTTP keeps admission immediately next to the outbound
// call. Source qualification must inspect the original client before this call.
func DoUpstreamAcceptanceHTTP(client *http.Client, req *http.Request, kind string) (*http.Response, error) {
	if client == nil || req == nil || req.URL == nil {
		return nil, fmt.Errorf("%w: invalid HTTP request", ErrUpstreamAcceptanceUnsent)
	}
	if !UpstreamAcceptanceModeEnabled() {
		return client.Do(req)
	}
	safeClient, err := newUpstreamAcceptanceHTTPClient(client, req.URL.Hostname())
	if err != nil {
		return nil, err
	}
	ctx, finish, err := claimUpstreamAcceptanceDispatch(req, kind)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUpstreamAcceptanceUnsent, err)
	}
	if err := ctx.Err(); err != nil {
		finish()
		return nil, fmt.Errorf("%w: %w", ErrUpstreamAcceptanceUnsent, err)
	}
	outbound := req.Clone(ctx)
	outbound.GetBody = nil
	for name := range outbound.Header {
		if strings.EqualFold(name, "Idempotency-Key") || strings.EqualFold(name, "X-Idempotency-Key") {
			delete(outbound.Header, name)
		}
	}
	resp, err := safeClient.Do(outbound)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		finish()
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		finish()
		return nil, fmt.Errorf("%w: missing HTTP response", ErrUpstreamAcceptanceDenied)
	}
	resp.Body = newUpstreamAcceptanceResponseBody(ctx, resp.Body, finish)
	return resp, nil
}

// A fresh HTTP/1 transport with no reusable connections cannot silently replay
// a request on a stale connection or retry an HTTP/2 stream. Constructing it
// explicitly also avoids Clone initializing HTTP/2 state on the shared source.
func newUpstreamAcceptanceHTTPClient(client *http.Client, host string) (*http.Client, error) {
	if client == nil || client.Jar != nil || common.TLSInsecureSkipVerify {
		return nil, fmt.Errorf("%w: unsafe HTTP client", ErrUpstreamAcceptanceUnsent)
	}
	rt := client.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	if shards, ok := rt.(*shardedRoundTripper); ok {
		if shards == nil || len(shards.shards) == 0 {
			return nil, fmt.Errorf("%w: invalid HTTP transport", ErrUpstreamAcceptanceUnsent)
		}
		// Acceptance runs have one generation at a time and need no pool
		// sharding. Reuse the first shard's network policy, never its pool.
		rt = shards.shards[0]
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if upstreamAcceptanceTestTransportAllowed != nil && upstreamAcceptanceTestTransportAllowed(rt) {
		copyClient.Transport = rt
		return &copyClient, nil
	}
	source, ok := rt.(*http.Transport)
	if !ok || source == nil || source.DialTLS != nil || source.DialTLSContext != nil {
		return nil, fmt.Errorf("%w: unsupported HTTP transport", ErrUpstreamAcceptanceUnsent)
	}
	tlsConfig := source.TLSClientConfig
	if tlsConfig != nil && (tlsConfig.InsecureSkipVerify || tlsConfig.ServerName != "" && tlsConfig.ServerName != host || tlsConfig.VerifyConnection != nil || tlsConfig.VerifyPeerCertificate != nil) {
		return nil, fmt.Errorf("%w: unsafe TLS policy", ErrUpstreamAcceptanceUnsent)
	}
	transport := &http.Transport{
		Proxy: source.Proxy, OnProxyConnectResponse: source.OnProxyConnectResponse,
		DialContext: source.DialContext, Dial: source.Dial,
		TLSHandshakeTimeout: source.TLSHandshakeTimeout,
		DisableKeepAlives:   true, DisableCompression: source.DisableCompression,
		MaxConnsPerHost:        source.MaxConnsPerHost,
		ResponseHeaderTimeout:  source.ResponseHeaderTimeout,
		ExpectContinueTimeout:  source.ExpectContinueTimeout,
		ProxyConnectHeader:     source.ProxyConnectHeader.Clone(),
		GetProxyConnectHeader:  source.GetProxyConnectHeader,
		MaxResponseHeaderBytes: source.MaxResponseHeaderBytes,
		WriteBufferSize:        source.WriteBufferSize, ReadBufferSize: source.ReadBufferSize,
		TLSNextProto: make(map[string]func(string, *tls.Conn) http.RoundTripper),
		Protocols:    new(http.Protocols),
	}
	transport.Protocols.SetHTTP1(true)
	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig.Clone()
	} else {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	copyClient.Transport = transport
	return &copyClient, nil
}

type upstreamAcceptanceResponseBody struct {
	body       io.ReadCloser
	finish     func()
	finishOnce sync.Once
	closeOnce  sync.Once
	closeErr   error
	stopMu     sync.Mutex
	stop       func() bool
}

func newUpstreamAcceptanceResponseBody(ctx context.Context, body io.ReadCloser, finish func()) *upstreamAcceptanceResponseBody {
	tracked := &upstreamAcceptanceResponseBody{body: body, finish: finish}
	tracked.stopMu.Lock()
	tracked.stop = context.AfterFunc(ctx, func() { _ = tracked.Close() })
	tracked.stopMu.Unlock()
	return tracked
}

func (body *upstreamAcceptanceResponseBody) Read(p []byte) (int, error) {
	n, err := body.body.Read(p)
	if err != nil {
		body.release()
	}
	return n, err
}

func (body *upstreamAcceptanceResponseBody) Close() error {
	body.closeOnce.Do(func() { body.closeErr = body.body.Close() })
	body.release()
	return body.closeErr
}

func (body *upstreamAcceptanceResponseBody) release() {
	body.finishOnce.Do(func() {
		body.stopMu.Lock()
		stop := body.stop
		body.stopMu.Unlock()
		if stop != nil {
			stop()
		}
		body.finish()
	})
}
