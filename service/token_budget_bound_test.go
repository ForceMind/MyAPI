package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A local TLS fixture with an ephemeral test-only certificate. No DNS lookup,
// real upstream identity, or actual OpenAI request is used by these tests.
func tokenBudgetCountTestClient(t *testing.T, handler http.HandlerFunc) *http.Client {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"api.openai.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	require.NoError(t, err)
	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	server.StartTLS()
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	t.Cleanup(func() { transport.CloseIdleConnections(); server.Close() })
	return &http.Client{Transport: transport, Timeout: 2 * time.Second}
}

func tokenBudgetOutboundFixture(t *testing.T, body string) (*http.Request, *relaycommon.RelayInfo) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", strings.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer isolated-fixture")
	info := &relaycommon.RelayInfo{RequestId: "budget-bound-fixture", UserId: 2, TokenId: 11,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ChannelId: 7, UpstreamModelName: "budget-fixture"}}
	return request, info
}

func TestTokenBudgetBoundUsesExactCountAndFreezesOutboundBody(t *testing.T) {
	client := tokenBudgetCountTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/responses/input_tokens", r.URL.Path)
		assert.Equal(t, "Bearer isolated-fixture", r.Header.Get("Authorization"))
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		var payload map[string]any
		assert.NoError(t, common.Unmarshal(body, &payload))
		assert.Equal(t, map[string]any{"model": "budget-fixture", "input": "hello", "instructions": "system"}, payload)
		_, _ = io.WriteString(w, `{"object":"response.input_tokens","input_tokens":0}`)
	})
	body := `{"model":"budget-fixture","input":"hello","instructions":"system","max_output_tokens":20,"stream":true,"store":false}`
	request, info := tokenBudgetOutboundFixture(t, body)
	info.SetEstimatePromptTokens(999)
	bound, err := CountTokenBudgetBound(context.Background(), client, request, info)
	require.NoError(t, err)
	assert.Zero(t, bound.InputTokens, "explicit zero count must not become the estimate")
	assert.EqualValues(t, 20, bound.MaxOutputTokens)
	assert.Len(t, bound.PayloadSHA256, 64)
	assert.Nil(t, request.GetBody, "possible dispatched POST must not be transparently replayable")
	outbound, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	assert.Equal(t, body, string(outbound), "generation uses precisely the validated bytes")
}

func TestTokenBudgetBoundRejectsMissingAndMalformedEvidence(t *testing.T) {
	for _, body := range []string{`{}`, `{"object":"response.input_tokens"}`, `{"object":"response.input_tokens","input_tokens":null}`, `{"object":"response.input_tokens","input_tokens":-1}`, `{"object":"response.input_tokens","input_tokens":1,"input_tokens":2}`, `{"object":"wrong","input_tokens":4}`, strings.Repeat("x", 16385)} {
		t.Run(body[:min(35, len(body))], func(t *testing.T) {
			client := tokenBudgetCountTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
			request, info := tokenBudgetOutboundFixture(t, `{"model":"budget-fixture","input":"hello","max_output_tokens":20}`)
			_, err := CountTokenBudgetBound(context.Background(), client, request, info)
			require.ErrorIs(t, err, ErrTokenBudgetCountUnavailable)
		})
	}
}

func TestTokenBudgetBoundRejectsUnsupportedPayloadWithoutRequest(t *testing.T) {
	client := tokenBudgetCountTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("unsupported payload reached the upstream fixture")
	})
	for _, body := range []string{
		`{"model":"budget-fixture","input":"hello"}`,
		`{"model":"budget-fixture","input":"hello","max_output_tokens":0}`,
		`{"model":"budget-fixture","input":"hello","max_output_tokens":20,"previous_response_id":"prior"}`,
		`{"model":"budget-fixture","input":[{"type":"message","role":"user","id":"external","content":"hi"}],"max_output_tokens":20}`,
		`{"model":"budget-fixture","input":[{"type":"input_image","image_url":"https://example.com/private"}],"max_output_tokens":20}`,
		`{"model":"budget-fixture","input":"hello","max_output_tokens":20,"max_output_tokens":30}`,
		`{"model":"other-model","input":"hello","max_output_tokens":20}`,
	} {
		request, info := tokenBudgetOutboundFixture(t, body)
		_, err := CountTokenBudgetBound(context.Background(), client, request, info)
		require.ErrorIs(t, err, ErrTokenBudgetUnsupported)
	}
}

func TestTokenBudgetBoundCannotFollowRedirectOrTrustInsecureCachedClient(t *testing.T) {
	client := tokenBudgetCountTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://unrelated.invalid/count", http.StatusTemporaryRedirect)
	})
	request, info := tokenBudgetOutboundFixture(t, `{"model":"budget-fixture","input":"hello","max_output_tokens":20}`)
	_, err := CountTokenBudgetBound(context.Background(), client, request, info)
	require.ErrorIs(t, err, ErrTokenBudgetCountUnavailable)
	insecure := *client
	insecureTransport := client.Transport.(*http.Transport).Clone()
	insecureTransport.TLSClientConfig = insecureTransport.TLSClientConfig.Clone()
	insecureTransport.TLSClientConfig.InsecureSkipVerify = true
	insecure.Transport = insecureTransport
	_, err = CountTokenBudgetBound(context.Background(), &insecure, request, info)
	require.ErrorIs(t, err, ErrTokenBudgetUnsupported)
	request.URL.Host = "compatible.invalid"
	_, err = CountTokenBudgetBound(context.Background(), client, request, info)
	require.ErrorIs(t, err, ErrTokenBudgetUnsupported)
}
