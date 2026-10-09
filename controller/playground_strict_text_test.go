package controller

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Keep the real official-host/TLS qualification path intact while routing all
// traffic to an isolated local server with an ephemeral test-only certificate.
func playgroundStrictTextClient(t *testing.T, handler http.HandlerFunc) *http.Client {
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
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}

// Exercise the same immutable publication path used by the existing pricing
// controller tests. These synthetic rates exist only in this temporary DB.
func playgroundStrictFeePrices(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.OfficialPriceVersion{}, &model.PricePublication{}))
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error { saved[key] = value; return nil }))
	oldUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	common.OptionMapRWMutex.Lock()
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		state, err := model.ReadPricePublicationSnapshot(context.Background())
		require.NoError(t, err)
		if state.State.Revision == 1 {
			digest, err := state.Digest()
			require.NoError(t, err)
			_, err = model.ApplyPricePublication(context.Background(), model.PricePublicationCommand{ID: strings.Repeat("b", 64), ActorID: 1, ExpectedDigest: digest, Action: "rollback", RollbackOf: strings.Repeat("a", 64)})
			require.NoError(t, err)
		}
		common.QuotaPerUnit = oldUnit
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptions
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"group_ratio_setting.group_ratio": `{"default":1}`}))
	document := `Prices per 1M tokens.
### Standard pricing data
| Model | Short context input | Short context cached input | Short context cache writes | Short context output | Long context input | Long context cached input | Long context cache writes | Long context output |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gpt-6.1-sol | $2 | $0.1 | $2.5 | $10 | $4 | $0.2 | $5 | $15 |

Short context: ≤272K input tokens. Long context: >272K input tokens.
`
	source, err := model.StoreOfficialPriceVersion(context.Background(), db, document, 100)
	require.NoError(t, err)
	preview, err := service.PreviewOpenAIPricePublication(context.Background(), source.ContentSHA256)
	require.NoError(t, err)
	require.Len(t, preview.Rows, 1)
	require.True(t, preview.Rows[0].Eligible)
	_, err = service.ApplyOpenAIPricePublication(context.Background(), 1, service.PricePublicationRequest{ID: strings.Repeat("a", 64), Action: "publish", ExpectedDigest: preview.ExpectedDigest, SourceSHA256: source.ContentSHA256, Confirmed: true, Models: []service.PricePublicationSelection{{Model: playgroundRelayModel, Locked: common.GetPointer(false)}}})
	require.NoError(t, err)
}

func TestPlaygroundStrictTextRelayDispatchesAndSettlesSelectedKey(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, stream := range []bool{false, true} {
			for _, feeOnly := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%v/fee-only=%v", mode, stream, feeOnly), func(t *testing.T) {
					db, user, key, access := playgroundControllerFixture(t, mode)
					require.NoError(t, db.Create(&model.TokenBudget{TokenID: key.Id, UserID: user.Id, Enabled: !feeOnly, Limit: model.TokenBudgetOpenAIChatContext, FeeEnabled: feeOnly, FeeLimitUSD: "10", Revision: 1}).Error)
					expectedQuota := 15
					if feeOnly {
						playgroundStrictFeePrices(t, db)
						expectedQuota = 35
					}
					other := &model.Token{UserId: user.Id, Key: "unselectedstrictfixture", Name: "Unselected ordinary key", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000}
					require.NoError(t, db.Create(other).Error)
					channel := &model.Channel{Id: 51, Name: "Strict native Chat fixture", Type: constant.ChannelTypeOpenAI, Key: "strictupstreamfixture", Status: common.ChannelStatusEnabled, Group: "default", Models: playgroundRelayModel + ",gpt-6.1-sol-alias", BaseURL: common.GetPointer("https://api.openai.com"), OtherSettings: `{"allow_service_tier":true}`}
					require.NoError(t, db.Create(channel).Error)
					require.NoError(t, db.Create(&[]model.Ability{
						{Group: "default", Model: playgroundRelayModel, ChannelId: channel.Id, Enabled: true, Weight: 1},
						{Group: "default", Model: "gpt-6.1-sol-alias", ChannelId: channel.Id, Enabled: true, Weight: 1},
					}).Error)
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-6.1-sol":1,"gpt-6.1-sol-alias":1}`))
					require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"gpt-6.1-sol":1,"gpt-6.1-sol-alias":1}`))
					payload := map[string]any{
						"model":    playgroundRelayModel,
						"messages": []any{map[string]any{"role": "user", "content": "Hello"}},
						"stream":   stream, "max_completion_tokens": 4096, "service_tier": "default",
					}
					if stream {
						payload["stream_options"] = map[string]any{"include_usage": true}
					}
					body, err := common.Marshal(payload)
					require.NoError(t, err)
					var upstreamCalls atomic.Int32
					client := playgroundStrictTextClient(t, func(w http.ResponseWriter, r *http.Request) {
						upstreamCalls.Add(1)
						assert.Equal(t, "api.openai.com", r.Host)
						assert.Equal(t, "/v1/chat/completions", r.URL.Path)
						assert.Equal(t, "Bearer strictupstreamfixture", r.Header.Get("Authorization"))
						assert.Empty(t, r.Header.Get("X-MyAPI-Key-ID"))
						raw, err := io.ReadAll(r.Body)
						assert.NoError(t, err)
						var received map[string]any
						assert.NoError(t, common.Unmarshal(raw, &received))
						// Stream=false is an optional zero value and may be omitted by
						// the provider DTO; every other qualified field is preserved.
						assert.Equal(t, playgroundRelayModel, received["model"])
						assert.Equal(t, payload["messages"], received["messages"])
						assert.EqualValues(t, 4096, received["max_completion_tokens"])
						assert.Equal(t, "default", received["service_tier"])
						for _, field := range []string{"max_tokens", "temperature", "top_p", "frequency_penalty", "presence_penalty", "seed"} {
							assert.NotContains(t, received, field)
						}
						if stream {
							assert.Equal(t, true, received["stream"])
							assert.Equal(t, map[string]any{"include_usage": true}, received["stream_options"])
						} else {
							assert.NotContains(t, received, "stream_options")
						}
						var reservation model.TokenBudgetReservation
						if assert.NoError(t, db.First(&reservation, "request_id = ?", "pg-strict-success").Error) {
							assert.Equal(t, model.TokenBudgetSent, reservation.State)
							assert.Equal(t, key.Id, reservation.TokenID)
							assert.Equal(t, model.TokenBudgetOpenAIChatContext, reservation.Reserved)
							assert.Equal(t, model.TokenBudgetBoundOpenAIChat, reservation.BoundSource)
						}
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							fmt.Fprint(w, "data: {\"id\":\"strict-fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6.1-sol\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Strict text response\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"strict-fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6.1-sol\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"id\":\"strict-fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6.1-sol\",\"service_tier\":\"default\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15,\"prompt_tokens_details\":{\"cached_tokens\":0,\"cache_write_tokens\":0}}}\n\ndata: [DONE]\n\n")
						} else {
							w.Header().Set("Content-Type", "application/json")
							fmt.Fprint(w, `{"id":"strict-fixture","object":"chat.completion","model":"gpt-6.1-sol","service_tier":"default","choices":[{"index":0,"message":{"role":"assistant","content":"Strict text response"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":0,"cache_write_tokens":0}}}`)
						}
					})
					t.Cleanup(service.SetHttpClientForTest(client))
					router := playgroundControllerRouter()
					if !feeOnly {
						for name, invalid := range map[string]string{
							"old-client":        `{"model":"gpt-6.1-sol","messages":[{"role":"user","content":"Hello"}],"temperature":0.7,"top_p":1,"frequency_penalty":0,"presence_penalty":0}`,
							"unsupported-model": strings.Replace(string(body), playgroundRelayModel, "gpt-6.1-sol-alias", 1),
							"image":             strings.Replace(string(body), `"content":"Hello"`, `"content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jU1sAAAAASUVORK5CYII="}}]`, 1),
							"file":              strings.Replace(string(body), `"content":"Hello"`, `"content":[{"type":"file","file":{"filename":"test.pdf","file_data":"data:application/pdf;base64,JVBERi0="}}]`, 1),
						} {
							denied := playgroundControllerRequest(router, http.MethodPost, "/pg/chat/completions", access, key.Id, invalid, "pg-strict-"+name)
							assert.Equal(t, http.StatusBadRequest, denied.Code, denied.Body.String())
							assert.Contains(t, denied.Body.String(), "token_budget_unsupported_request")
						}
					}
					if feeOnly {
						require.NoError(t, db.Model(channel).Update("settings", `{"allow_service_tier":false}`).Error)
						denied := playgroundControllerRequest(router, http.MethodPost, "/pg/chat/completions", access, key.Id, string(body), "pg-strict-fee-missing-tier")
						assert.Equal(t, http.StatusServiceUnavailable, denied.Code, denied.Body.String())
						assert.Contains(t, denied.Body.String(), "token_budget_fee_evidence")
						require.NoError(t, db.Model(channel).Update("settings", `{"allow_service_tier":true}`).Error)
					}
					assert.Zero(t, upstreamCalls.Load())
					response := playgroundControllerRequest(router, http.MethodPost, "/pg/chat/completions", access, key.Id, string(body), "pg-strict-success")
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					assert.Contains(t, response.Body.String(), "Strict text response")
					assert.EqualValues(t, 1, upstreamCalls.Load())
					budget, err := model.LookupTokenBudget(context.Background(), db, key.Id)
					require.NoError(t, err)
					require.NotNil(t, budget)
					assert.EqualValues(t, 15, budget.Used)
					assert.Zero(t, budget.Reserved)
					assert.Empty(t, budget.PendingRequestID)
					if feeOnly {
						assert.False(t, budget.Enabled)
						assert.Equal(t, "0.00007", budget.FeeUsedUSD)
						assert.Equal(t, "0", budget.FeeReservedUSD)
					}
					var reservation model.TokenBudgetReservation
					require.NoError(t, db.First(&reservation, "request_id = ?", "pg-strict-success").Error)
					assert.Equal(t, model.TokenBudgetSettled, reservation.State)
					assert.EqualValues(t, 4096, reservation.MaxOutputTokens)
					require.NotNil(t, reservation.ActualInput)
					require.NotNil(t, reservation.ActualOutput)
					assert.EqualValues(t, 10, *reservation.ActualInput)
					assert.EqualValues(t, 5, *reservation.ActualOutput)
					if feeOnly {
						require.NotNil(t, reservation.ActualFeeUSD)
						assert.Equal(t, "0.00007", *reservation.ActualFeeUSD)
						assert.Contains(t, reservation.FeePriceEvidence, `"version":2`)
					}
					require.NoError(t, db.First(key, key.Id).Error)
					require.NoError(t, db.First(user, user.Id).Error)
					require.NoError(t, db.First(other, other.Id).Error)
					assert.Equal(t, expectedQuota, key.UsedQuota)
					assert.Equal(t, 1000000-expectedQuota, key.RemainQuota)
					assert.Equal(t, 1000000-expectedQuota, user.Quota)
					assert.Equal(t, 1000, other.RemainQuota)
					assert.Zero(t, other.UsedQuota)
					var logs []model.Log
					require.NoError(t, db.Where("request_id = ? AND type = ?", "pg-strict-success", model.LogTypeConsume).Find(&logs).Error)
					require.Len(t, logs, 1)
					assert.Equal(t, key.Id, logs[0].TokenId)
					assert.Equal(t, stream, logs[0].IsStream)
					assert.Equal(t, expectedQuota, logs[0].Quota)
					assert.Contains(t, logs[0].Other, `"bound_source":"openai_chat_context_window"`)
					assert.Contains(t, logs[0].Other, `"actual_input":10`)
				})
			}
		}
	}
}
