package channel

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
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
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	subscriptionShareNativeAccount = "share-native-account"
	subscriptionShareNativeAccess  = "share-native-access"
)

type subscriptionShareNativeAdaptor struct {
	Adaptor
}

func (subscriptionShareNativeAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return "https://chatgpt.com/backend-api/codex/responses", nil
}

func (subscriptionShareNativeAdaptor) SetupRequestHeader(_ *gin.Context, headers *http.Header, _ *relaycommon.RelayInfo) error {
	headers.Set("Authorization", "Bearer "+subscriptionShareNativeAccess)
	headers.Set("ChatGPT-Account-ID", subscriptionShareNativeAccount)
	headers.Set("Content-Type", "application/json")
	return nil
}

type subscriptionShareSendFixture struct {
	db      *gorm.DB
	ctx     *gin.Context
	info    *relaycommon.RelayInfo
	user    model.User
	token   model.Token
	initial int64
}

func newSubscriptionShareSendFixture(t *testing.T, mode model.QuotaWriterMode, label string, shareBPS int, exhausted bool) *subscriptionShareSendFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/subscription-share-send.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	models := []any{
		&model.User{}, &model.Token{}, &model.TokenBudget{}, &model.Channel{}, &model.ChannelQuotaSnapshot{},
		&model.ChannelQuotaIdentityKeyRegistry{}, &model.ChannelQuotaIdentityKeyVersion{}, &model.ChannelQuotaIdentityAlias{},
		&model.AssignedAccessPolicy{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.UsageReviewDecision{},
		&model.UserQuotaMutationReceipt{}, &model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{},
		&model.AccountQuotaTerminalRecoveryObligation{}, &model.AccountQuotaRefundFact{}, &model.AccountQuotaSettlementIntent{},
		&model.AccountQuotaSettlementFact{}, &model.LegacyUsageReservation{}, &model.SystemTask{}, &model.SystemTaskLock{},
		&model.QuotaWriterEpoch{}, &model.QuotaProjectionObligation{}, &model.QuotaBalanceBatchDrain{},
		&model.QuotaBalanceBatchSubject{}, &model.QuotaWorkCursor{}, &model.AccountQuotaExecutionGate{},
		&model.AccountQuotaExecutionPermit{}, &model.Log{}, &model.BillingLogProjectionIdentity{},
	}
	models = append(models, model.SubscriptionShareModels()...)
	require.NoError(t, db.AutoMigrate(models...))
	require.True(t, model.RefreshAccountQuotaSettlementIntentSchemaCapability(db))
	require.NoError(t, model.EnsureFreshAccountQuotaExecutionGate(context.Background(), db))
	require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
	require.NoError(t, db.Model(&model.QuotaWriterEpoch{}).Where("id = ?", 1).Updates(map[string]any{
		"mode": string(mode), "epoch": int64(71), "lock_version": gorm.Expr("lock_version + ?", 1),
	}).Error)

	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainDatabaseType := common.MainDatabaseType()
	oldRedis, oldRDB := common.RedisEnabled, common.RDB
	oldBatch, oldLogConsume := common.BatchUpdateEnabled, common.LogConsumeEnabled
	model.DB, model.LOG_DB = db, db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	model.InitColumnNamesForTest()
	common.RedisEnabled, common.RDB = false, nil
	common.BatchUpdateEnabled, common.LogConsumeEnabled = false, false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetMainDatabaseType(oldMainDatabaseType)
		common.RedisEnabled, common.RDB = oldRedis, oldRDB
		common.BatchUpdateEnabled, common.LogConsumeEnabled = oldBatch, oldLogConsume
	})

	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "active:v1:"+base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	require.NoError(t, err)
	require.NoError(t, model.EnsureChannelQuotaIdentityKeyring(context.Background(), db, keyring))
	account, err := model.ResolveChannelQuotaIdentity(context.Background(), db, keyring, "channel_type_57", common.ChannelQuotaIdentityKindProviderAccount, []byte(subscriptionShareNativeAccount))
	require.NoError(t, err)

	root := model.User{Username: "share-send-root-" + label, AffCode: "share-send-root-" + label, Role: common.RoleRootUser, Status: common.UserStatusEnabled, Quota: 1000}
	user := model.User{Username: "share-send-owner-" + label, AffCode: "share-send-owner-" + label, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 1000, AuthVersion: 1}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "share-send-token-" + label, Status: common.TokenStatusEnabled, RemainQuota: 1000, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	credential := `{"access_token":"` + subscriptionShareNativeAccess + `","account_id":"` + subscriptionShareNativeAccount + `"}`
	channel := model.Channel{Id: 701, Name: "subscription share TLS fixture", Type: constant.ChannelTypeCodex, Key: credential, Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)

	now, err := model.ReadDatabaseUnixTime(context.Background(), db)
	require.NoError(t, err)
	first := subscriptionShareNativeSample(t, db, channel.Id, account.SubjectRef, now-20, now+3600, 10)
	require.NoError(t, model.ConfigureSubscriptionShare(context.Background(), db, root.Id, token.Id, model.SubscriptionShareConfigInput{
		ID: strings.Repeat("a", 64), SnapshotID: first.Id, ShareBPS: shareBPS, ConfirmedApproximate: true,
	}, now-10))
	subscriptionShareNativeSample(t, db, channel.Id, account.SubjectRef, now-5, now+3600, 10)
	require.NoError(t, model.RefreshSubscriptionShareProjection(context.Background(), db, account.SubjectRef, now))
	var pool model.SubscriptionSharePool
	require.NoError(t, db.Where("subject_ref = ?", account.SubjectRef).Take(&pool).Error)
	if exhausted {
		var allocation model.SubscriptionShareAllocation
		require.NoError(t, db.Where("pool_id = ? AND token_id = ?", pool.ID, token.Id).Take(&allocation).Error)
		require.NoError(t, db.Model(&model.SubscriptionShareAllocationUsage{}).
			Where("allocation_id = ? AND generation = ?", allocation.ID, pool.CurrentGeneration).
			Update("estimated_used_micro", int64(shareBPS)*10_000).Error)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	requestID := "share-send-" + label
	ctx.Set(common.RequestIdKey, requestID)
	info := &relaycommon.RelayInfo{
		UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, RequestId: requestID,
		OriginModelName: "gpt-5-codex", UserQuota: user.Quota, StartTime: time.Now(), ForcePreConsume: true,
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"},
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channel.Id, ChannelType: constant.ChannelTypeCodex, ApiKey: credential},
	}
	session, apiErr := service.NewBillingSession(ctx, info, 100)
	require.Nil(t, apiErr)
	require.Equal(t, 100, session.GetPreConsumedQuota())
	info.Billing = session
	t.Cleanup(func() { service.FinalizeTextUsageDispatch(ctx, info) })
	return &subscriptionShareSendFixture{db: db, ctx: ctx, info: info, user: user, token: token, initial: 1000}
}

func subscriptionShareNativeSample(t *testing.T, db *gorm.DB, channelID int, subject string, observedAt, resetAt int64, used float64) model.ChannelQuotaSnapshot {
	t.Helper()
	total := float64(100)
	snapshot := model.ChannelQuotaSnapshot{
		ChannelId: channelID, SubjectRef: subject, IdentityQuality: model.ChannelQuotaIdentityQualityProviderConfirmed,
		ObservedAt: observedAt, SampleID: "share-native-" + subject[:8] + "-" + big.NewInt(observedAt).String(),
		Used: &used, Total: &total, Available: 100 - used, MetricType: "codex_rate_limit", WindowType: "five_hour",
		WindowSeconds: 18000, PlanType: "plus", ResetAt: resetAt, Source: "codex_wham_usage_primary",
		Status: "success", Unit: "percent", CodexObservationQualified: true, CodexThresholdQualified: true,
	}
	require.NoError(t, db.Create(&snapshot).Error)
	return snapshot
}

func subscriptionShareTLSClient(t *testing.T, handler http.Handler) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "subscription share test CA"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "chatgpt.com"}, DNSNames: []string{"chatgpt.com"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, ca, &leafKey.PublicKey, caKey)
	require.NoError(t, err)
	leafKeyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	require.NoError(t, err)
	certificate, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: leafKeyDER}),
	)
	require.NoError(t, err)

	calls := &atomic.Int32{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler.ServeHTTP(w, r)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}
	server.StartTLS()
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != "chatgpt.com:443" {
				return nil, errors.New("unexpected subscription share TLS target: " + address)
			}
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
		TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
	}
	restore := service.SetHttpClientForTest(&http.Client{Transport: transport})
	t.Cleanup(func() {
		restore()
		transport.CloseIdleConnections()
	})
	return server, calls
}

func TestSubscriptionShareDoApiRequestSendsOnceOverVerifiedNativeTLS(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			fixture := newSubscriptionShareSendFixture(t, mode, "success-"+string(mode), 5000, false)
			_, calls := subscriptionShareTLSClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/backend-api/codex/responses", r.URL.Path)
				assert.Equal(t, "chatgpt.com", r.Host)
				assert.Equal(t, "Bearer "+subscriptionShareNativeAccess, r.Header.Get("Authorization"))
				assert.Equal(t, subscriptionShareNativeAccount, r.Header.Get("ChatGPT-Account-ID"))
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			response, err := DoApiRequest(subscriptionShareNativeAdaptor{}, fixture.ctx, fixture.info, bytes.NewReader([]byte(`{"model":"gpt-5-codex"}`)))
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			assert.EqualValues(t, 1, calls.Load())
			var attempts []model.SubscriptionShareAttempt
			require.NoError(t, fixture.db.Find(&attempts).Error)
			require.Len(t, attempts, 1)
			assert.Equal(t, model.SubscriptionShareAttemptDispatching, attempts[0].State)
		})
	}
}

func TestSubscriptionShareDoApiRequestRefusalDoesNotSendAndCanRefund(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, state := range []struct {
			name      string
			shareBPS  int
			exhausted bool
		}{
			{name: "zero", shareBPS: 0},
			{name: "exhausted", shareBPS: 2000, exhausted: true},
		} {
			t.Run(string(mode)+"/"+state.name, func(t *testing.T) {
				fixture := newSubscriptionShareSendFixture(t, mode, state.name+"-"+string(mode), state.shareBPS, state.exhausted)
				_, calls := subscriptionShareTLSClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
				response, err := DoApiRequest(subscriptionShareNativeAdaptor{}, fixture.ctx, fixture.info, bytes.NewReader([]byte(`{"model":"gpt-5-codex"}`)))
				require.Error(t, err)
				var apiError *types.NewAPIError
				require.True(t, errors.As(err, &apiError), "refusal must identify the actual share gate")
				assert.Equal(t, types.ErrorCode("subscription_share_exceeded"), apiError.GetErrorCode())
				assert.Equal(t, http.StatusForbidden, apiError.StatusCode)
				assert.Nil(t, response)
				assert.Zero(t, calls.Load())
				assert.False(t, service.TextUsageDispatchNeedsReview(fixture.info))
				assert.True(t, fixture.info.Billing.NeedsRefund())
				var attempts int64
				require.NoError(t, fixture.db.Model(&model.SubscriptionShareAttempt{}).Count(&attempts).Error)
				assert.Zero(t, attempts)
				require.NoError(t, fixture.info.Billing.Refund(fixture.ctx))
				var user model.User
				var token model.Token
				require.NoError(t, fixture.db.First(&user, fixture.user.Id).Error)
				require.NoError(t, fixture.db.First(&token, fixture.token.Id).Error)
				assert.Equal(t, fixture.initial, user.Quota)
				assert.Equal(t, fixture.initial, token.RemainQuota)
			})
		}
	}
}

func TestSubscriptionShareDoApiRequestTransportErrorKeepsPossiblySentAttempt(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			fixture := newSubscriptionShareSendFixture(t, mode, "transport-"+string(mode), 5000, false)
			_, calls := subscriptionShareTLSClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				connection, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = connection.Close()
				}
			}))
			response, err := DoApiRequest(subscriptionShareNativeAdaptor{}, fixture.ctx, fixture.info, bytes.NewReader([]byte(`{"model":"gpt-5-codex"}`)))
			require.Error(t, err)
			assert.Nil(t, response)
			assert.EqualValues(t, 1, calls.Load())
			var attempt model.SubscriptionShareAttempt
			require.NoError(t, fixture.db.Take(&attempt).Error)
			assert.Equal(t, model.SubscriptionShareAttemptDispatching, attempt.State)
			assert.True(t, service.TextUsageDispatchNeedsReview(fixture.info))
			require.ErrorIs(t, fixture.info.Billing.Refund(fixture.ctx), model.ErrAccountQuotaUsageUnresolved)
		})
	}
}
