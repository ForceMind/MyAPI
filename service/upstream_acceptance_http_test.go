package service

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type acceptanceHTTPTestTransport struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (transport *acceptanceHTTPTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport.roundTrip(req)
}

type acceptanceHTTPTestBody struct {
	reader   io.Reader
	closes   atomic.Int32
	closeErr error
	closed   chan struct{}
}

func (body *acceptanceHTTPTestBody) Read(p []byte) (int, error) { return body.reader.Read(p) }
func (body *acceptanceHTTPTestBody) Close() error {
	if body.closes.Add(1) == 1 && body.closed != nil {
		close(body.closed)
	}
	return body.closeErr
}

func TestAcceptanceHTTPClientPreservesSharedTransport(t *testing.T) {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)
	source := &http.Transport{
		ForceAttemptHTTP2:     true,
		Protocols:             protocols,
		TLSClientConfig:       &tls.Config{NextProtos: []string{"h2", "http/1.1"}},
		ProxyConnectHeader:    http.Header{"X-Test": []string{"original"}},
		ResponseHeaderTimeout: 7 * time.Second,
	}
	redirectErr := errors.New("original redirect policy")
	client := &http.Client{Transport: source, Timeout: 11 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return redirectErr }}
	safe, err := newUpstreamAcceptanceHTTPClient(client, "chatgpt.com")
	require.NoError(t, err)
	transport, ok := safe.Transport.(*http.Transport)
	require.True(t, ok)
	assert.NotSame(t, source, transport)
	assert.True(t, transport.DisableKeepAlives)
	assert.False(t, transport.ForceAttemptHTTP2)
	require.NotNil(t, transport.Protocols)
	assert.True(t, transport.Protocols.HTTP1())
	assert.False(t, transport.Protocols.HTTP2())
	assert.False(t, transport.Protocols.UnencryptedHTTP2())
	require.NotNil(t, transport.TLSNextProto)
	assert.Empty(t, transport.TLSNextProto)
	assert.Equal(t, []string{"http/1.1"}, transport.TLSClientConfig.NextProtos)
	assert.Equal(t, 7*time.Second, transport.ResponseHeaderTimeout)
	assert.Equal(t, client.Timeout, safe.Timeout)
	transport.ProxyConnectHeader.Set("X-Test", "changed")
	transport.TLSClientConfig.NextProtos[0] = "changed"
	assert.Equal(t, "original", source.ProxyConnectHeader.Get("X-Test"))
	assert.Equal(t, []string{"h2", "http/1.1"}, source.TLSClientConfig.NextProtos)
	assert.Nil(t, source.TLSNextProto, "the shared source must not even be initialized for HTTP/2")
	assert.False(t, source.DisableKeepAlives)
	assert.True(t, source.ForceAttemptHTTP2)
	assert.True(t, source.Protocols.HTTP2())
	assert.True(t, source.Protocols.UnencryptedHTTP2())
	assert.ErrorIs(t, client.CheckRedirect(nil, nil), redirectErr)
}

func TestAcceptanceHTTPClientRejectsUnsafePolicies(t *testing.T) {
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	cases := []struct {
		name   string
		client *http.Client
	}{
		{"nil client", nil},
		{"cookie jar", &http.Client{Jar: jar}},
		{"unknown transport", &http.Client{Transport: &acceptanceHTTPTestTransport{}}},
		{"nil transport pointer", &http.Client{Transport: (*http.Transport)(nil)}},
		{"empty shards", &http.Client{Transport: &shardedRoundTripper{}}},
		{"insecure TLS", &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}},
		{"different TLS name", &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{ServerName: "elsewhere.example"}}}},
		{"custom TLS verification", &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{VerifyConnection: func(tls.ConnectionState) error { return nil }}}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := newUpstreamAcceptanceHTTPClient(test.client, "chatgpt.com")
			require.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
			assert.NotContains(t, err.Error(), "elsewhere.example")
		})
	}
	previous := common.TLSInsecureSkipVerify
	common.TLSInsecureSkipVerify = true
	t.Cleanup(func() { common.TLSInsecureSkipVerify = previous })
	_, err = newUpstreamAcceptanceHTTPClient(&http.Client{}, "chatgpt.com")
	require.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
}

func TestAcceptanceHTTPClientExtractsShardWithoutSharingPool(t *testing.T) {
	source := &http.Transport{ResponseHeaderTimeout: 9 * time.Second}
	shards := &shardedRoundTripper{shards: []http.RoundTripper{source}, n: 1}
	safe, err := newUpstreamAcceptanceHTTPClient(&http.Client{Transport: shards}, "chatgpt.com")
	require.NoError(t, err)
	transport, ok := safe.Transport.(*http.Transport)
	require.True(t, ok)
	assert.NotSame(t, source, transport)
	assert.Equal(t, source.ResponseHeaderTimeout, transport.ResponseHeaderTimeout)
	assert.True(t, transport.DisableKeepAlives)
	assert.False(t, source.DisableKeepAlives)
}

func TestAcceptanceHTTPClientDoesNotFollowRedirect(t *testing.T) {
	var calls int
	transport := &acceptanceHTTPTestTransport{roundTrip: func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusTemporaryRedirect, Header: http.Header{"Location": []string{"https://elsewhere.example/secret"}}, Body: io.NopCloser(strings.NewReader("redirect")), Request: req}, nil
	}}
	previous := upstreamAcceptanceTestTransportAllowed
	upstreamAcceptanceTestTransportAllowed = func(candidate http.RoundTripper) bool { return candidate == transport }
	t.Cleanup(func() { upstreamAcceptanceTestTransportAllowed = previous })
	safe, err := newUpstreamAcceptanceHTTPClient(&http.Client{Transport: transport}, "chatgpt.com")
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodGet, "https://chatgpt.com/backend-api/wham/usage", nil)
	require.NoError(t, err)
	resp, err := safe.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
	assert.Equal(t, 1, calls)
	_, err = newUpstreamAcceptanceHTTPClient(&http.Client{Transport: &acceptanceHTTPTestTransport{}}, "chatgpt.com")
	require.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
}

func TestAcceptanceResponseBodyDrainsOnce(t *testing.T) {
	for _, eofFirst := range []bool{true, false} {
		name := "close"
		if eofFirst {
			name = "EOF then close"
		}
		t.Run(name, func(t *testing.T) {
			original := &acceptanceHTTPTestBody{reader: strings.NewReader("response")}
			var finished int
			body := newUpstreamAcceptanceResponseBody(context.Background(), original, func() { finished++ })
			assert.Zero(t, finished)
			if eofFirst {
				data, err := io.ReadAll(body)
				require.NoError(t, err)
				assert.Equal(t, "response", string(data))
				assert.Equal(t, 1, finished)
				assert.Zero(t, original.closes.Load(), "EOF must not suppress the eventual underlying Close")
			}
			require.NoError(t, body.Close())
			require.NoError(t, body.Close())
			assert.Equal(t, int32(1), original.closes.Load())
			assert.Equal(t, 1, finished)
		})
	}
}

func TestAcceptanceResponseBodyCancellationClosesUnreadBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	original := &acceptanceHTTPTestBody{reader: strings.NewReader("unread"), closed: make(chan struct{})}
	finished := make(chan struct{})
	body := newUpstreamAcceptanceResponseBody(ctx, original, func() { close(finished) })
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not drain the response")
	}
	select {
	case <-original.closed:
	default:
		t.Fatal("drain was reported before underlying Close")
	}
	require.NoError(t, body.Close())
	assert.Equal(t, int32(1), original.closes.Load())
}

func TestAcceptanceResponseBodyReadAndCloseErrorsStillDrain(t *testing.T) {
	readErr := errors.New("read failed")
	closeErr := errors.New("close failed")
	original := &acceptanceHTTPTestBody{reader: iotestAcceptanceErrorReader{err: readErr}, closeErr: closeErr}
	finished := 0
	body := newUpstreamAcceptanceResponseBody(context.Background(), original, func() { finished++ })
	_, err := body.Read(make([]byte, 1))
	require.ErrorIs(t, err, readErr)
	assert.Equal(t, 1, finished)
	require.ErrorIs(t, body.Close(), closeErr)
	assert.Equal(t, 1, finished)
}

type iotestAcceptanceErrorReader struct{ err error }

func (reader iotestAcceptanceErrorReader) Read([]byte) (int, error) { return 0, reader.err }
