package service

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func subscriptionShareDispatchFixture(t *testing.T) (*gorm.DB, *relaycommon.RelayInfo, *http.Client, *http.Request, int64) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/share.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	entities := []any{&model.User{}, &model.Token{}, &model.TokenBudget{}, &model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.ChannelQuotaIdentityKeyRegistry{}, &model.ChannelQuotaIdentityKeyVersion{}, &model.ChannelQuotaIdentityAlias{}}
	entities = append(entities, model.SubscriptionShareModels()...)
	require.NoError(t, db.AutoMigrate(entities...))
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "share-root", AffCode: "share-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.User{Id: 2, Username: "share-owner", AffCode: "share-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}).Error)
	for _, token := range []model.Token{{Id: 11, UserId: 2, Key: "share-bound", Status: common.TokenStatusEnabled, ExpiredTime: -1}, {Id: 12, UserId: 2, Key: "share-unbound", Status: common.TokenStatusEnabled, ExpiredTime: -1}} {
		require.NoError(t, db.Create(&token).Error)
	}
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "active:v1:"+base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("s", 32))))
	ring, err := common.LoadChannelQuotaIdentityKeyring()
	require.NoError(t, err)
	require.NoError(t, model.EnsureChannelQuotaIdentityKeyring(context.Background(), db, ring))
	account, err := model.ResolveChannelQuotaIdentity(context.Background(), db, ring, "channel_type_57", common.ChannelQuotaIdentityKindProviderAccount, []byte("share-account-a"))
	require.NoError(t, err)
	now, err := model.ReadDatabaseUnixTime(context.Background(), db)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Channel{Id: 7, Name: "Synthetic subscription", Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Key: "synthetic"}).Error)
	row := subscriptionShareDispatchSample(t, db, account.SubjectRef, now-20, now+3600, 10)
	require.NoError(t, model.ConfigureSubscriptionShare(context.Background(), db, 1, 11, model.SubscriptionShareConfigInput{ID: strings.Repeat("a", 64), SnapshotID: row.Id, ShareBPS: 2000, ConfirmedApproximate: true}, now-10))
	subscriptionShareDispatchSample(t, db, account.SubjectRef, now-5, now+3600, 10)
	require.NoError(t, model.RefreshSubscriptionShareProjection(context.Background(), db, account.SubjectRef, now))
	key := `{"access_token":"synthetic-access-a","account_id":"share-account-a"}`
	info := &relaycommon.RelayInfo{TokenId: 11, UserId: 2, RequestId: "share-dispatch-request", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 7, ChannelType: constant.ChannelTypeCodex, ApiKey: key}}
	request, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", strings.NewReader(`{"model":"gpt-5-codex"}`))
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer synthetic-access-a")
	request.Header.Set("ChatGPT-Account-ID", "share-account-a")
	return db, info, &http.Client{Transport: &http.Transport{}}, request, now
}

func subscriptionShareDispatchSample(t *testing.T, db *gorm.DB, subject string, at, reset int64, used float64) model.ChannelQuotaSnapshot {
	t.Helper()
	total := float64(100)
	row := model.ChannelQuotaSnapshot{ChannelId: 7, SubjectRef: subject, IdentityQuality: model.ChannelQuotaIdentityQualityProviderConfirmed, ObservedAt: at, SampleID: "sample-" + subject[:8] + "-" + strconv.FormatInt(at, 10), Used: &used, Total: &total, Available: 100 - used, MetricType: "codex_rate_limit", WindowType: "five_hour", WindowSeconds: 18000, PlanType: "plus", ResetAt: reset, Source: "codex_wham_usage_primary", Status: "success", Unit: "percent", CodexObservationQualified: true, CodexThresholdQualified: true}
	require.NoError(t, db.Create(&row).Error)
	return row
}

func TestSubscriptionShareFinalDispatchIdentityReplayAndProvenUnsent(t *testing.T) {
	db, info, client, request, _ := subscriptionShareDispatchFixture(t)
	ctx := context.Background()
	attempt, err := PrepareSubscriptionShareDispatch(ctx, client, request, info)
	require.NoError(t, err)
	require.Len(t, attempt, 64)
	assert.Nil(t, request.GetBody)
	var count int64
	require.NoError(t, db.Model(&model.SubscriptionShareAttempt{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	_, err = PrepareSubscriptionShareDispatch(ctx, client, request, info)
	require.ErrorIs(t, err, model.ErrSubscriptionShareConflict, "a repeated prepare must not authorize a second send")
	info.RequestId = "share-wrong-header"
	request.Header.Set("ChatGPT-Account-ID", "another-account")
	_, err = PrepareSubscriptionShareDispatch(ctx, client, request, info)
	require.ErrorIs(t, err, model.ErrSubscriptionShareUnavailable)
	require.NoError(t, db.Model(&model.SubscriptionShareAttempt{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, CancelSubscriptionShareUnsent(ctx, attempt))
	var saved model.SubscriptionShareAttempt
	require.NoError(t, db.First(&saved, "attempt_id = ?", attempt).Error)
	assert.Equal(t, model.SubscriptionShareAttemptNotSent, saved.State)
}

func TestSubscriptionShareFinalDispatchDoesNotGateUnrelatedUnregisteredAccount(t *testing.T) {
	db, info, client, request, _ := subscriptionShareDispatchFixture(t)
	info.TokenId, info.RequestId = 12, "share-unrelated-request"
	info.ApiKey = `{"access_token":"synthetic-access-b","account_id":"share-unregistered-b"}`
	request.Header.Set("Authorization", "Bearer synthetic-access-b")
	request.Header.Set("ChatGPT-Account-ID", "share-unregistered-b")
	attempt, err := PrepareSubscriptionShareDispatch(context.Background(), client, request, info)
	require.NoError(t, err)
	assert.Empty(t, attempt)
	var count int64
	require.NoError(t, db.Model(&model.SubscriptionShareAttempt{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSubscriptionShareFinalDispatchRejectsUnallocatedKeyAndBoundProviderBypass(t *testing.T) {
	_, info, client, request, _ := subscriptionShareDispatchFixture(t)
	info.TokenId = 12
	_, err := PrepareSubscriptionShareDispatch(context.Background(), client, request, info)
	require.ErrorIs(t, err, model.ErrSubscriptionShareDenied)
	info.TokenId, info.ChannelType = 11, constant.ChannelTypeOpenAI
	_, err = PrepareSubscriptionShareDispatch(context.Background(), client, request, info)
	require.ErrorIs(t, err, model.ErrSubscriptionShareDenied)
}

func TestSubscriptionShareRoutingContinuesAfterAnUnallocatedAccount(t *testing.T) {
	db, _, _, _, now := subscriptionShareDispatchFixture(t)
	ring, err := common.LoadChannelQuotaIdentityKeyring()
	require.NoError(t, err)
	account, err := model.ResolveChannelQuotaIdentity(context.Background(), db, ring, "channel_type_57", common.ChannelQuotaIdentityKindProviderAccount, []byte("share-account-b"))
	require.NoError(t, err)
	row := subscriptionShareDispatchSample(t, db, account.SubjectRef, now-20, now+3600, 5)
	require.NoError(t, model.ConfigureSubscriptionShare(context.Background(), db, 1, 12, model.SubscriptionShareConfigInput{ID: strings.Repeat("b", 64), SnapshotID: row.Id, ShareBPS: 5000, ConfirmedApproximate: true}, now-10))
	subscriptionShareDispatchSample(t, db, account.SubjectRef, now-5, now+3600, 5)
	require.NoError(t, model.RefreshSubscriptionShareProjection(context.Background(), db, account.SubjectRef, now))
	baseURL := "https://chatgpt.com"
	channel := &model.Channel{Id: 7, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, BaseURL: &baseURL,
		Key:         `{"access_token":"synthetic-access-a","account_id":"share-account-a"}` + "\n" + `{"access_token":"synthetic-access-b","account_id":"share-account-b"}`,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModePolling}}
	ctx := common.WithSubscriptionShareAdmission(context.Background(), common.SubscriptionShareAdmission{TokenID: 12, Bound: true})
	excluded, eligible, err := CodexQuotaEligibleKeys(ctx, channel)
	require.NoError(t, err)
	assert.True(t, eligible)
	assert.True(t, excluded[0])
	assert.False(t, excluded[1])
	channel.Key = `{"access_token":"synthetic-unregistered","account_id":"unregistered"}` + "\n" + `{"access_token":"synthetic-access-b","account_id":"share-account-b"}`
	excluded, eligible, err = CodexQuotaEligibleKeys(ctx, channel)
	require.NoError(t, err)
	assert.True(t, eligible)
	assert.True(t, excluded[0])
	assert.False(t, excluded[1])
}

func TestSubscriptionShareLocalRefusalClearsOnlyUnsentDispatchEvidence(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			require.NoError(t, db.AutoMigrate(&model.UsageReviewDecision{}, &model.TokenBudget{}, &model.TokenBudgetReservation{}, &model.TokenBudgetPolicyChange{}, &model.Log{}))
			user, token := seedAuthoritativeBilling(t, db, "share-refusal", 1000, 1000, false)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := authoritativeRelay(user, token, "share-local-refusal")
			info.ChannelMeta, info.StartTime = &relaycommon.ChannelMeta{}, time.Now()
			c.Set(common.RequestIdKey, info.RequestId)
			session, apiErr := NewBillingSession(c, info, 100)
			require.Nil(t, apiErr)
			info.Billing = session
			request := httptest.NewRequest(http.MethodPost, "https://example.invalid/synthetic", nil)
			require.NoError(t, PrepareTextUsageDispatch(c, request, info))
			require.True(t, TextUsageDispatchNeedsReview(info))
			// A local refusal proves that this request never invoked transport.
			require.NoError(t, CancelTextUsageDispatchBeforeSend(info))
			assert.False(t, TextUsageDispatchNeedsReview(info))
			assert.False(t, FinalizeTextUsageDispatch(c, info))
			require.NoError(t, session.Refund(c))
			var restored model.User
			require.NoError(t, db.First(&restored, user.Id).Error)
			assert.EqualValues(t, 1000, restored.Quota)
		})
	}
}
