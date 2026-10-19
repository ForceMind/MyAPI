package channel

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Both existing writers run through the production relay and subscription
// share gates. Only the upstream TLS server and test database are synthetic.
func TestUpstreamAcceptanceDispatchUnsentCanRefund(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, refusal := range []string{"transport_policy", "late_budget"} {
			t.Run(string(mode)+"/"+refusal, func(t *testing.T) {
				fixture := newSubscriptionShareSendFixture(t, mode, "acceptance-unsent-"+string(mode)+"-"+refusal, 5000, false)
				_, calls := subscriptionShareTLSClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
				run := startAcceptanceDispatchFixture(t, fixture)
				var injected atomic.Bool
				if refusal == "transport_policy" {
					transport, ok := service.GetHttpClient().Transport.(*http.Transport)
					require.True(t, ok)
					// Native-source qualification still succeeds with the real CA
					// and standard verification. The final acceptance transport
					// policy rejects this additional callback before any send.
					transport.TLSClientConfig.VerifyConnection = func(tls.ConnectionState) error { return nil }
				} else {
					const callbackName = "test:acceptance_budget_competitor"
					attemptTable := fixture.db.NamingStrategy.TableName("SubscriptionShareAttempt")
					require.NoError(t, fixture.db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
						if tx.Statement.Table != attemptTable || !injected.CompareAndSwap(false, true) {
							return
						}
						// Deterministic competing-budget state, not another real
						// upstream call. Stay in the share transaction and use a
						// fresh Statement so this cannot corrupt its pending INSERT.
						result := tx.Session(&gorm.Session{NewDB: true}).Model(&model.UpstreamAcceptanceRun{}).
							Where("id = ?", run.ID).Updates(map[string]any{"total": int64(1), "generation": int64(1)})
						if result.Error != nil {
							tx.AddError(result.Error)
						} else if result.RowsAffected != 1 {
							tx.AddError(errors.New("synthetic competing budget was not recorded"))
						}
					}))
					t.Cleanup(func() { require.NoError(t, fixture.db.Callback().Create().Remove(callbackName)) })
				}
				response, err := DoApiRequest(subscriptionShareNativeAdaptor{}, fixture.ctx, fixture.info, bytes.NewReader([]byte(`{"model":"gpt-5-codex"}`)))
				require.Error(t, err)
				var apiError *types.NewAPIError
				require.True(t, errors.As(err, &apiError))
				assert.Equal(t, types.ErrorCode("upstream_acceptance_denied"), apiError.GetErrorCode())
				assert.Equal(t, http.StatusForbidden, apiError.StatusCode)
				assert.Nil(t, response)
				assert.Zero(t, calls.Load())
				var attempts []model.SubscriptionShareAttempt
				require.NoError(t, fixture.db.Find(&attempts).Error)
				require.Len(t, attempts, 1, "the share gate must run before the final acceptance refusal")
				assert.Equal(t, model.SubscriptionShareAttemptNotSent, attempts[0].State)
				assert.False(t, service.TextUsageDispatchNeedsReview(fixture.info))
				assert.True(t, fixture.info.Billing.NeedsRefund())
				require.NoError(t, fixture.info.Billing.Refund(fixture.ctx))
				var user model.User
				var token model.Token
				require.NoError(t, fixture.db.First(&user, fixture.user.Id).Error)
				require.NoError(t, fixture.db.First(&token, fixture.token.Id).Error)
				assert.Equal(t, fixture.initial, user.Quota)
				assert.Equal(t, fixture.initial, token.RemainQuota)
				assert.Equal(t, fixture.token.Id, token.Id)
				assert.Equal(t, fixture.token.Key, token.Key)
				var stored model.UpstreamAcceptanceRun
				require.NoError(t, fixture.db.First(&stored, "id = ?", run.ID).Error)
				expected := int64(0)
				if refusal == "late_budget" {
					assert.True(t, injected.Load())
					expected = 1
				}
				assert.Equal(t, expected, stored.Total)
				assert.Equal(t, expected, stored.Generation)
				assert.Zero(t, stored.GenerationInFlight)
			})
		}
	}
}

func TestUpstreamAcceptanceDispatchSentUnknownCannotRefund(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			fixture := newSubscriptionShareSendFixture(t, mode, "acceptance-sent-"+string(mode), 5000, false)
			_, calls := subscriptionShareTLSClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				connection, _, err := w.(http.Hijacker).Hijack()
				if assert.NoError(t, err) {
					_ = connection.Close()
				}
			}))
			run := startAcceptanceDispatchFixture(t, fixture)
			response, err := DoApiRequest(subscriptionShareNativeAdaptor{}, fixture.ctx, fixture.info, bytes.NewReader([]byte(`{"model":"gpt-5-codex"}`)))
			require.Error(t, err)
			assert.Nil(t, response)
			assert.Equal(t, int32(1), calls.Load(), "a possibly sent request must never be transparently retried")
			var attempt model.SubscriptionShareAttempt
			require.NoError(t, fixture.db.Take(&attempt).Error)
			assert.Equal(t, model.SubscriptionShareAttemptDispatching, attempt.State)
			assert.True(t, service.TextUsageDispatchNeedsReview(fixture.info))
			require.ErrorIs(t, fixture.info.Billing.Refund(fixture.ctx), model.ErrAccountQuotaUsageUnresolved)
			var stored model.UpstreamAcceptanceRun
			require.NoError(t, fixture.db.First(&stored, "id = ?", run.ID).Error)
			assert.Equal(t, int64(1), stored.Total)
			assert.Equal(t, int64(1), stored.Generation)
			assert.Zero(t, stored.GenerationInFlight)
			var token model.Token
			require.NoError(t, fixture.db.First(&token, fixture.token.Id).Error)
			assert.Equal(t, fixture.token.Id, token.Id)
			assert.Equal(t, fixture.token.Key, token.Key)
		})
	}
}

func TestUpstreamAcceptanceDispatchBodyAndHandlerDrain(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			fixture := newSubscriptionShareSendFixture(t, mode, "acceptance-drain-"+string(mode), 5000, false)
			_, calls := subscriptionShareTLSClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, "synthetic-response-body")
			}))
			run := startAcceptanceDispatchFixture(t, fixture)
			response, err := DoApiRequest(subscriptionShareNativeAdaptor{}, fixture.ctx, fixture.info, bytes.NewReader([]byte(`{"model":"gpt-5-codex"}`)))
			require.NoError(t, err)
			t.Cleanup(func() { _ = response.Body.Close() })
			status, err := service.GetUpstreamAcceptanceStatus(context.Background(), run.ID)
			require.NoError(t, err)
			assert.Equal(t, 1, status.Operations)
			assert.Equal(t, 1, status.HTTPInFlight)
			assert.Equal(t, int64(1), status.Run.GenerationInFlight)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			assert.Equal(t, "synthetic-response-body", string(body))
			require.NoError(t, response.Body.Close())
			status, err = service.StopUpstreamAcceptanceRun(context.Background(), run.ID)
			require.NoError(t, err)
			assert.Equal(t, model.UpstreamAcceptanceRunStopping, status.Run.Status)
			assert.Equal(t, 1, status.Operations, "the outer request still owns its accounting and logging completion")
			assert.Zero(t, status.HTTPInFlight)
			assert.Zero(t, status.Run.GenerationInFlight)
			// This fixture deliberately has no parsed provider usage. Preserve
			// that uncertainty before simulating the outer controller's defer.
			assert.True(t, service.FinalizeTextUsageDispatch(fixture.ctx, fixture.info))
			service.FinishUpstreamAcceptanceRelayOperation(fixture.ctx)
			status, err = service.GetUpstreamAcceptanceStatus(context.Background(), run.ID)
			require.NoError(t, err)
			assert.Equal(t, model.UpstreamAcceptanceRunStopped, status.Run.Status)
			assert.Zero(t, status.Operations)
			assert.Zero(t, status.HTTPInFlight)
			assert.False(t, status.DrainError)
			assert.Equal(t, int64(1), status.Run.Total)
			assert.Equal(t, int64(1), status.Run.Generation)
			assert.Equal(t, int32(1), calls.Load())
			var token model.Token
			require.NoError(t, fixture.db.First(&token, fixture.token.Id).Error)
			assert.Equal(t, fixture.token.Id, token.Id)
			assert.Equal(t, fixture.token.Key, token.Key)
		})
	}
}

func startAcceptanceDispatchFixture(t *testing.T, fixture *subscriptionShareSendFixture) *model.UpstreamAcceptanceRun {
	t.Helper()
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
	require.NoError(t, fixture.db.AutoMigrate(&model.UpstreamAcceptanceRun{}))
	run, err := service.StartUpstreamAcceptanceRun(context.Background(), fixture.info.ChannelId, service.UpstreamAcceptanceLimits{
		DurationSeconds: 120, MaxTotal: 1, MaxGeneration: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		service.FinalizeTextUsageDispatch(fixture.ctx, fixture.info)
		service.FinishUpstreamAcceptanceRelayOperation(fixture.ctx)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := service.StopUpstreamAcceptanceRun(ctx, run.ID)
		assert.NoError(t, err)
	})
	return run
}
