package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
)

// publishPaymentOptionsForTest publishes canonical payment options into the
// payment runtime (as the save bridge would after a committed save) and
// restores the previous generation content for those keys on cleanup.
func publishPaymentOptionsForTest(t *testing.T, values map[string]string) {
	t.Helper()
	canonical := setting.CurrentPaymentRuntime().CanonicalOptions()
	mutations := make([]setting.PaymentMutation, 0, len(values))
	for key, value := range values {
		mutation, err := setting.PaymentMutationFromCanonicalOption(setting.PaymentOptionKey(key), value)
		require.NoError(t, err)
		mutations = append(mutations, mutation)
	}
	_, err := setting.PublishPaymentCanonicalMutations(mutations)
	require.NoError(t, err)
	t.Cleanup(func() {
		restore := make([]setting.PaymentMutation, 0, len(values))
		for key := range values {
			optionKey := setting.PaymentOptionKey(key)
			if previous, ok := canonical[key]; ok {
				mutation, err := setting.PaymentMutationFromCanonicalOption(optionKey, previous)
				require.NoError(t, err)
				restore = append(restore, mutation)
			} else {
				restore = append(restore, setting.PaymentMutation{Key: optionKey, Action: setting.PaymentMutationClear})
			}
		}
		_, err := setting.PublishPaymentCanonicalMutations(restore)
		require.NoError(t, err)
	})
}

func TestStripeWebhookVerifiesWithSavedSnapshotSecret(t *testing.T) {
	_ = paymentWebhookTestDB(t)
	previousSecret := setting.StripeWebhookSecret
	setting.StripeWebhookSecret = "whsec_n2b_legacy_stale"
	t.Cleanup(func() { setting.StripeWebhookSecret = previousSecret })
	publishPaymentOptionsForTest(t, map[string]string{
		"StripeWebhookSecret": "whsec_n2b_snapshot",
	})

	// An unknown trade number keeps the assertion on the verification path:
	// after signature verification the funding decision short-circuits with
	// 200, so the outcome depends only on which secret the handler used.
	assert.Equal(t, http.StatusBadRequest, deliverStripeFixture(t, "checkout.session.completed", "stripe-n2b-unknown", "complete", "whsec_n2b_legacy_stale"))
	assert.Equal(t, http.StatusOK, deliverStripeFixture(t, "checkout.session.completed", "stripe-n2b-unknown", "complete", "whsec_n2b_snapshot"))
}

type paymentSnapshotTransport func(*http.Request) (*http.Response, error)

func (f paymentSnapshotTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestStripeOrderBuildsPerRequestClientFromPinnedSnapshot(t *testing.T) {
	previousSecret, previousPrice := setting.StripeApiSecret, setting.StripePriceId
	setting.StripeApiSecret, setting.StripePriceId = "sk_legacy_stale", "price_legacy_stale"
	t.Cleanup(func() { setting.StripeApiSecret, setting.StripePriceId = previousSecret, previousPrice })
	publishPaymentOptionsForTest(t, map[string]string{
		"StripeApiSecret": "sk_test_n2b_order_v1",
		"StripePriceId":   "price_n2b_order_v1",
	})

	// The request pins its configuration; a later save must not leak into it.
	pinned := setting.CapturePaymentConfig()
	publishPaymentOptionsForTest(t, map[string]string{
		"StripeApiSecret": "sk_test_n2b_order_v2",
		"StripePriceId":   "price_n2b_order_v2",
	})

	var seenAuthorization, seenBody string
	transport := http.DefaultTransport
	http.DefaultTransport = paymentSnapshotTransport(func(r *http.Request) (*http.Response, error) {
		seenAuthorization = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		seenBody = string(body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"id":"cs_n2b","object":"checkout.session","url":"https://checkout.stripe.test/pay/cs_n2b"}`)),
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = transport })

	stripeKeyBefore := stripe.Key
	payLink, err := genStripeLink(pinned, "ref_n2b_pinned", "", "n2b@example.test", 2, "", "")
	require.NoError(t, err)
	assert.Equal(t, "https://checkout.stripe.test/pay/cs_n2b", payLink)
	assert.Equal(t, "Bearer sk_test_n2b_order_v1", seenAuthorization)
	assert.Contains(t, seenBody, "price_n2b_order_v1")
	assert.NotContains(t, seenBody, "price_n2b_order_v2")
	assert.NotContains(t, seenBody, "price_legacy_stale")
	assert.Equal(t, stripeKeyBefore, stripe.Key, "the stripe.Key SDK global must stay untouched")

	// A fresh capture observes the saved v2 generation.
	fresh := setting.CapturePaymentConfig()
	assert.Equal(t, "sk_test_n2b_order_v2", fresh.StripeApiSecret())
	assert.Greater(t, fresh.Revision(), pinned.Revision())
}

func TestCreemWebhookVerifiesWithSavedSnapshotSecret(t *testing.T) {
	_ = paymentWebhookTestDB(t)
	previousSecret := setting.CreemWebhookSecret
	setting.CreemWebhookSecret = "creem_n2b_legacy_stale"
	t.Cleanup(func() { setting.CreemWebhookSecret = previousSecret })
	publishPaymentOptionsForTest(t, map[string]string{
		"CreemWebhookSecret": "creem_n2b_snapshot",
	})

	// Like the Stripe case, an unknown request id keeps the assertion on the
	// verification path (funding decision short-circuits with 200).
	body := `{"id":"evt_n2b","eventType":"checkout.completed","object":{"request_id":"creem-n2b-unknown","order":{"id":"order_n2b","status":"paid","type":"onetime","amount_paid":100,"currency":"USD"},"product":{"id":"prod_n2b"},"customer":{"email":"n2b@example.test","name":"N2B"}}}`
	deliver := func(secret string) int {
		request := httptest.NewRequest(http.MethodPost, "/api/creem/webhook", strings.NewReader(body))
		request.Header.Set(CreemSignatureHeader, generateCreemSignature(body, secret))
		response := httptest.NewRecorder()
		router := gin.New()
		router.POST("/api/creem/webhook", CreemWebhook)
		router.ServeHTTP(response, request)
		return response.Code
	}

	assert.Equal(t, http.StatusUnauthorized, deliver("creem_n2b_legacy_stale"))
	assert.Equal(t, http.StatusOK, deliver("creem_n2b_snapshot"))
}

func TestPinnedSnapshotImmuneToMidRequestSecretRotation(t *testing.T) {
	publishPaymentOptionsForTest(t, map[string]string{
		"CreemWebhookSecret": "creem_n2b_pinned_v1",
	})
	pinned := setting.CapturePaymentConfig()

	// The operator saves a rotated secret while the request is in flight.
	publishPaymentOptionsForTest(t, map[string]string{
		"CreemWebhookSecret": "creem_n2b_pinned_v2",
	})

	payload := "n2b-pinned-payload"
	signatureV1 := generateCreemSignature(payload, "creem_n2b_pinned_v1")
	assert.True(t, verifyCreemSignature(payload, signatureV1, pinned), "the in-flight request must keep its captured generation")
	assert.False(t, verifyCreemSignature(payload, signatureV1, setting.CapturePaymentConfig()), "a fresh capture must observe the rotated generation")
}

func TestEpayClientBuiltFromSnapshotCredentials(t *testing.T) {
	previousAddress, previousID, previousKey := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey
	operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = "https://legacy-stale.example.test", "legacy_stale_id", "legacy_stale_key"
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = previousAddress, previousID, previousKey
	})
	publishPaymentOptionsForTest(t, map[string]string{
		"PayAddress": "https://pay-n2b.example.test",
		"EpayId":     "epay_n2b_id",
		"EpayKey":    "epay_n2b_key",
	})

	pinned := setting.CapturePaymentConfig()
	publishPaymentOptionsForTest(t, map[string]string{
		"PayAddress": "https://pay-n2b-v2.example.test",
		"EpayId":     "epay_n2b_id_v2",
		"EpayKey":    "epay_n2b_key_v2",
	})

	client := GetEpayClient(pinned)
	require.NotNil(t, client, "snapshot credentials must build the epay client even when legacy variables differ")
	assert.True(t, isEpayWebhookConfiguredFromPaymentConfig(pinned))
	assert.Equal(t, "https://pay-n2b.example.test", pinned.PayAddress())
	assert.Equal(t, "https://pay-n2b-v2.example.test", setting.CapturePaymentConfig().PayAddress())
}

func TestStripeWebhookSignatureFixtureStillVerifies(t *testing.T) {
	// Guard the fixture itself: a payload signed with the pinned snapshot
	// secret must round-trip through stripe-go's verifier exactly as the
	// handler drives it.
	publishPaymentOptionsForTest(t, map[string]string{
		"StripeWebhookSecret": "whsec_n2b_fixture_guard",
	})
	pinned := setting.CapturePaymentConfig()
	payload, err := common.Marshal(map[string]any{"id": "evt_guard", "object": "event", "type": "fixture.guard"})
	require.NoError(t, err)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: "whsec_n2b_fixture_guard"})
	_, err = webhook.ConstructEventWithOptions(payload, signed.Header, pinned.StripeWebhookSecret(), webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	require.NoError(t, err)
}
