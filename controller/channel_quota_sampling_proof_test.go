package controller

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
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestScheduledCodexSampleBindsTargetToConcreteNativeSnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	previousDB := model.DB
	previousInterval := common.RequestInterval
	previousTLSInsecure := common.TLSInsecureSkipVerify
	model.DB = db
	common.RequestInterval = 0
	common.TLSInsecureSkipVerify = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RequestInterval = previousInterval
		common.TLSInsecureSkipVerify = previousTLSInsecure
	})
	setupChannelQuotaIdentityFixture(t, db)

	client := codexSamplingProofClient(t, func(w http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/backend-api/wham/usage", request.URL.Path)
		assert.Equal(t, "Bearer proof-access", request.Header.Get("Authorization"))
		assert.Equal(t, "proof-account", request.Header.Get("ChatGPT-Account-ID"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":10,"reset_at":1900000000,"limit_window_seconds":18000},"secondary_window":null}}`)
	})
	t.Cleanup(service.SetHttpClientForTest(client))
	baseURL := "https://chatgpt.com"
	channel := &model.Channel{
		Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, BaseURL: &baseURL,
		Key: `{"access_token":"proof-access","account_id":"proof-account","type":"codex"}`,
	}
	require.NoError(t, db.Create(channel).Error)

	summary, err := runChannelQuotaSnapshotSyncOnce(context.Background(), 1, nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Sampled)
	require.Zero(t, summary.PersistFailed)

	var target model.ChannelQuotaSamplingTarget
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Take(&target).Error)
	require.Equal(t, int64(1), target.EvidenceVersion)
	require.NotZero(t, target.VerifiedSnapshotID)
	require.Equal(t, "sampled", target.LastResult)
	require.NotEmpty(t, target.ConfirmedSubjectRef)

	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.First(&snapshot, target.VerifiedSnapshotID).Error)
	assert.Equal(t, channel.Id, snapshot.ChannelId)
	assert.Equal(t, target.ConfirmedSubjectRef, snapshot.SubjectRef)
	assert.Equal(t, model.ChannelQuotaIdentityQualityProviderConfirmed, snapshot.IdentityQuality)
	assert.Equal(t, "codex_rate_limit", snapshot.MetricType)
	assert.Equal(t, "codex_wham_usage_primary", snapshot.Source)
	assert.Equal(t, "success", snapshot.Status)
	assert.True(t, snapshot.CodexObservationQualified)
	assert.True(t, snapshot.CodexThresholdQualified)
	assert.NotEmpty(t, snapshot.SampleID)
}

func TestCodexDuplicateSampleDoesNotProduceProofID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	previousDB := model.DB
	previousTLSInsecure := common.TLSInsecureSkipVerify
	model.DB = db
	common.TLSInsecureSkipVerify = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.TLSInsecureSkipVerify = previousTLSInsecure
	})
	setupChannelQuotaIdentityFixture(t, db)

	client := codexSamplingProofClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":10,"reset_at":1900000000,"limit_window_seconds":18000},"secondary_window":null}}`)
	})
	t.Cleanup(service.SetHttpClientForTest(client))
	credential := `{"access_token":"proof-access","account_id":"proof-account","type":"codex"}`
	identity, err := resolveQuotaSamplingIdentity(context.Background(), constant.ChannelTypeCodex, common.ChannelQuotaIdentityKindCredential, []byte(credential))
	require.NoError(t, err)
	baseURL := "https://chatgpt.com"
	channel := &model.Channel{Id: 782, Type: constant.ChannelTypeCodex, BaseURL: &baseURL}
	proofSampleID := "preexisting-value"

	require.NoError(t, sampleCodexCredentialUsage(context.Background(), channel, credential, identity,
		func(string) bool { return false }, nil, nil, &proofSampleID))
	assert.Empty(t, proofSampleID)
	var snapshots int64
	require.NoError(t, db.Model(&model.ChannelQuotaSnapshot{}).Count(&snapshots).Error)
	assert.Zero(t, snapshots)
}

func TestScheduledCodexRotationRequiresNewLifecycleSampleForProof(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.SystemTaskLock{}))
	previousDB := model.DB
	previousInterval := common.RequestInterval
	previousMemoryCache := common.MemoryCacheEnabled
	previousTLSInsecure := common.TLSInsecureSkipVerify
	model.DB = db
	common.RequestInterval = 0
	common.MemoryCacheEnabled = false
	common.TLSInsecureSkipVerify = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RequestInterval = previousInterval
		common.MemoryCacheEnabled = previousMemoryCache
		common.TLSInsecureSkipVerify = previousTLSInsecure
	})
	setupChannelQuotaIdentityFixture(t, db)
	baseURL := "https://chatgpt.com"
	channel := &model.Channel{
		Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, BaseURL: &baseURL,
		Key: `{"access_token":"old-access","refresh_token":"old-refresh","account_id":"rotation-account","type":"codex"}`,
	}
	require.NoError(t, db.Create(channel).Error)
	establishCodexControllerCredentialEvidence(t, channel.Id, channel.Key, "rotation-account")
	var originalTarget model.ChannelQuotaSamplingTarget
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Take(&originalTarget).Error)

	client := codexSamplingProofClient(t, func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/oauth/token":
			_, _ = io.WriteString(w, `{"access_token":"`+codexControllerAccountJWT(t, "rotation-account")+`","refresh_token":"new-refresh","expires_in":3600}`)
		case "/backend-api/wham/usage":
			if request.Header.Get("Authorization") == "Bearer old-access" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{}`)
				return
			}
			_, _ = io.WriteString(w, `{"plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":10,"reset_at":1900000000,"limit_window_seconds":18000},"secondary_window":null}}`)
		default:
			http.NotFound(w, request)
		}
	})
	t.Cleanup(service.SetHttpClientForTest(client))

	summary, err := runChannelQuotaSnapshotSyncOnce(context.Background(), 1, nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Sampled)
	require.Zero(t, summary.PersistFailed)
	var rotatedTarget model.ChannelQuotaSamplingTarget
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Take(&rotatedTarget).Error)
	assert.NotEqual(t, originalTarget.SubjectRef, rotatedTarget.SubjectRef)
	assert.Equal(t, int64(2), rotatedTarget.EvidenceVersion)
	assert.Zero(t, rotatedTarget.VerifiedSnapshotID, "the pre-rotation request cannot attest the replacement lifecycle")

	summary, err = runChannelQuotaSnapshotSyncOnce(context.Background(), 1, nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Sampled)
	require.Zero(t, summary.PersistFailed)
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Take(&rotatedTarget).Error)
	assert.Equal(t, int64(2), rotatedTarget.EvidenceVersion)
	assert.NotZero(t, rotatedTarget.VerifiedSnapshotID, "a later request started under the replacement lifecycle may establish proof")
}

// This fixture keeps the production transport type and certificate checks.
// Dialing is redirected to an isolated TLS server without DNS or network use.
func codexSamplingProofClient(t *testing.T, handler http.HandlerFunc) *http.Client {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	certificate := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"chatgpt.com", "auth.openai.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA: true, BasicConstraintsValid: true,
	}
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
	t.Cleanup(func() {
		transport.CloseIdleConnections()
		server.Close()
	})
	return &http.Client{Transport: transport, Timeout: 2 * time.Second}
}
