package service

import (
	"testing"

	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBillingSessionInflightTrackingSymmetric covers the process-local quota
// writer in-flight counter: a created session increments it exactly once and
// every terminal wrap-up (settle success, settle failure, refund failure)
// decrements it exactly once, including idempotent repeats.
func TestBillingSessionInflightTrackingSymmetric(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
	user, token := seedAuthoritativeBilling(t, db, "inflight", 1000, 500, false)
	baseline := model.QuotaWriterInflightSessions()

	// Failed creation must not move the counter.
	_, apiErr := NewBillingSession(overflowTestGinContext(), authoritativeRelay(user, token, "inflight-too-big"), 100000)
	require.NotNil(t, apiErr)
	assert.Equal(t, baseline, model.QuotaWriterInflightSessions())

	// Successful creation + settle wrap-up is symmetric.
	session, apiErr := NewBillingSession(overflowTestGinContext(), authoritativeRelay(user, token, "inflight-settle"), 100)
	require.Nil(t, apiErr)
	assert.Equal(t, baseline+1, model.QuotaWriterInflightSessions())
	require.NoError(t, session.Settle(100))
	assert.Equal(t, baseline, model.QuotaWriterInflightSessions())
	require.NoError(t, session.Settle(100), "idempotent repeat settle stays a no-op")
	assert.Equal(t, baseline, model.QuotaWriterInflightSessions(), "the counter is released exactly once")

	// Settle error wrap-up (missing request id on a non-zero delta) still
	// releases the counter via the deferred finish.
	requestlessRelay := &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key,
		RequestId: "", OriginModelName: "fixture-model", UserQuota: user.Quota,
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
	settleSession, apiErr := NewBillingSession(overflowTestGinContext(), requestlessRelay, 100)
	require.Nil(t, apiErr)
	assert.Equal(t, baseline+1, model.QuotaWriterInflightSessions())
	require.Error(t, settleSession.Settle(50))
	assert.Equal(t, baseline, model.QuotaWriterInflightSessions())

	// Refund error wrap-up (missing request id) also releases the counter.
	refundSession, apiErr := NewBillingSession(overflowTestGinContext(), requestlessRelay, 100)
	require.Nil(t, apiErr)
	assert.Equal(t, baseline+1, model.QuotaWriterInflightSessions())
	require.Error(t, refundSession.Refund(nil))
	assert.Equal(t, baseline, model.QuotaWriterInflightSessions())
}
