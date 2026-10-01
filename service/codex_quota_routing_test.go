package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createCodexQuotaRoutingChannel(t *testing.T, db *gorm.DB, id int, priority int64, key string) {
	t.Helper()
	weight := uint(1)
	channel := model.Channel{
		Id: id, Type: constant.ChannelTypeCodex, Key: key,
		Status: common.ChannelStatusEnabled, Name: fmt.Sprintf("codex-%d", id),
		Group: "default", Models: "gpt-5-codex", Priority: &priority, Weight: &weight,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: "gpt-5-codex", ChannelId: id,
		Enabled: true, Priority: &priority, Weight: weight,
	}).Error)
}

func recordCodexRoutingUsage(t *testing.T, channelID int, account string, available float64, resetAt int64) {
	t.Helper()
	require.NoError(t, model.RecordChannelQuotaSnapshot(&model.ChannelQuotaSnapshot{
		ChannelId: channelID, AccountRef: model.ChannelQuotaAccountRef("codex", account),
		ObservedAt: time.Now().Unix(), Available: available, Status: "success", Unit: "percent",
		MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary",
		ResetAt: resetAt, SampleID: fmt.Sprintf("sample-%d-%s-%v", channelID, account, available),
	}))
}

func TestCodexQuotaRoutingSkipsExhaustedChannelAndKeepsHealthyFallback(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	createCodexQuotaRoutingChannel(t, db, 3101, 100, `{"access_token":"fixture-a","account_id":"account-a"}`)
	createCodexQuotaRoutingChannel(t, db, 3102, 50, `{"access_token":"fixture-b","account_id":"account-b"}`)
	require.NoError(t, model.InitChannelCache())
	recordCodexRoutingUsage(t, 3101, "account-a", 0, time.Now().Add(time.Hour).Unix())
	recordCodexRoutingUsage(t, 3102, "account-b", 80, time.Now().Add(time.Hour).Unix())

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	retry := 0
	selected, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: "gpt-5-codex", RequestPath: "/v1/responses", Retry: &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 3102, selected.Id)
	assert.Equal(t, "default", group)

	recordCodexRoutingUsage(t, 3102, "account-b", 0, time.Now().Add(time.Hour).Unix())
	selected, _, err = CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: "gpt-5-codex", RequestPath: "/v1/responses", Retry: &retry,
	})
	require.NoError(t, err)
	assert.Nil(t, selected, "all known exhausted accounts must be removed before dispatch")
}

func TestCodexQuotaRoutingKeepsHealthyKeyWithinMultiKeyChannel(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	createCodexQuotaRoutingChannel(t, db, 3201, 100,
		`{"access_token":"fixture-a","account_id":"account-a"}`+"\n"+`{"access_token":"fixture-b","account_id":"account-b"}`)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 3201).Update("channel_info", model.ChannelInfo{
		IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModeRandom,
	}).Error)
	require.NoError(t, model.InitChannelCache())
	recordCodexRoutingUsage(t, 3201, "account-a", 0, time.Now().Add(time.Hour).Unix())
	recordCodexRoutingUsage(t, 3201, "account-b", 80, time.Now().Add(time.Hour).Unix())
	channel, err := model.CacheGetChannel(3201)
	require.NoError(t, err)
	excluded, eligible, err := CodexQuotaEligibleKeys(context.Background(), channel)
	require.NoError(t, err)
	require.True(t, eligible)
	assert.True(t, excluded[0])
	assert.False(t, excluded[1])
	key, index, apiErr := channel.GetNextEnabledKeyExcluding(excluded)
	require.Nil(t, apiErr)
	assert.Contains(t, key, `"account_id":"account-b"`)
	assert.Equal(t, 1, index)
}

func TestCodexUsageLimitMarkerRequiresExactUpstream429AndHoldsUntilReset(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		message string
		want    bool
	}{
		{name: "account usage limit", status: http.StatusTooManyRequests, message: "The usage limit has been reached", want: true},
		{name: "generic burst throttle", status: http.StatusTooManyRequests, message: "Too many requests", want: false},
		{name: "same words but not 429", status: http.StatusBadRequest, message: "The usage limit has been reached", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiErr := types.NewOpenAIError(errors.New(tc.message), types.ErrorCodeBadResponseStatusCode, tc.status)
			assert.Equal(t, tc.want, IsCodexUsageLimitError(apiErr))
		})
	}
	upstream := &http.Response{StatusCode: http.StatusTooManyRequests,
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"The usage limit has been reached","type":"usage_limit_reached","code":"usage_limit_reached"}}`))}
	mapped := RelayErrorHandler(context.Background(), upstream, false)
	require.NotNil(t, mapped)
	ResetStatusCode(mapped, `{"429":"503"}`)
	assert.Equal(t, http.StatusServiceUnavailable, mapped.StatusCode)
	assert.Equal(t, http.StatusTooManyRequests, mapped.UpstreamStatusCode)
	assert.True(t, IsCodexUsageLimitError(mapped), "client status mapping must not erase the provider exhaustion signal")

	db := setupChannelSelectAutoGroupsTest(t)
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	knownReset := time.Now().Add(time.Minute).Unix()
	recordCodexRoutingUsage(t, 3301, "account-a", 0, knownReset)
	require.NoError(t, RecordCodexUsageLimit(context.Background(), 3301, `{"access_token":"fixture-a","account_id":"account-a"}`))
	var marker model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("source = ?", model.CodexQuotaRouteLimitSource).Take(&marker).Error)
	assert.Equal(t, model.CodexQuotaRouteLimitCode, marker.ErrorCode)
	assert.Equal(t, knownReset, marker.ResetAt)
	assert.Equal(t, model.ChannelQuotaAccountRef("codex", "account-a"), marker.AccountRef)
	assert.NotContains(t, marker.ErrorMessage, "fixture-a")
	assert.NotContains(t, marker.ErrorMessage, "account-a")
	state, err := model.ReadCodexQuotaRouteState(context.Background(), db, "", marker.AccountRef, time.Now().Unix())
	require.NoError(t, err)
	assert.True(t, state.Blocked)
}

func TestCodexUsageLimitWithoutKnownResetNeedsFreshProviderRecovery(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	credential := `{"access_token":"fixture-a","account_id":"unknown-reset-account"}`
	require.NoError(t, RecordCodexUsageLimit(context.Background(), 3302, credential))
	var marker model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("source = ?", model.CodexQuotaRouteLimitSource).Take(&marker).Error)
	assert.Zero(t, marker.ResetAt, "an unknown reset is not a five-minute proof of recovery")

	state, err := model.ReadCodexQuotaRouteState(context.Background(), db, "", marker.AccountRef, marker.ObservedAt+3600)
	require.NoError(t, err)
	assert.True(t, state.Blocked, "ordinary traffic must not become a blind recovery probe")

	require.NoError(t, model.RecordChannelQuotaSnapshot(&model.ChannelQuotaSnapshot{
		ChannelId: 3302, AccountRef: marker.AccountRef, ObservedAt: marker.ObservedAt + 1,
		SampleID: "confirmed-recovery", MetricType: "codex_rate_limit", WindowType: "five_hour",
		Source: "codex_wham_usage_primary", Status: "success", Unit: "percent",
		Available: 50, ResetAt: marker.ObservedAt + 7200,
	}))
	state, err = model.ReadCodexQuotaRouteState(context.Background(), db, "", marker.AccountRef, marker.ObservedAt+3600)
	require.NoError(t, err)
	assert.False(t, state.Blocked)
}

func TestCodexQuotaRoutingUsesVersionedConfirmedIdentityWithoutStoringAccountID(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	secret := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("q", 32)))
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "active:v1:"+secret)
	require.NoError(t, db.AutoMigrate(
		&model.ChannelQuotaSnapshot{}, &model.ChannelQuotaIdentityKeyRegistry{},
		&model.ChannelQuotaIdentityKeyVersion{}, &model.ChannelQuotaIdentityAlias{},
	))
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	require.NoError(t, err)
	require.NoError(t, model.EnsureChannelQuotaIdentityKeyring(context.Background(), db, keyring))
	identity, err := model.ResolveChannelQuotaIdentity(context.Background(), db, keyring,
		"channel_type_57", common.ChannelQuotaIdentityKindProviderAccount, []byte("private-account"))
	require.NoError(t, err)
	require.NoError(t, model.RecordChannelQuotaSnapshot(&model.ChannelQuotaSnapshot{
		ChannelId: 3401, SubjectRef: identity.SubjectRef, IdentityQuality: identity.Quality,
		ObservedAt: time.Now().Unix(), SampleID: "f4-exhausted", Available: 0,
		MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary",
		Status: "success", Unit: "percent", ResetAt: time.Now().Add(time.Hour).Unix(),
	}))
	channel := &model.Channel{Id: 3402, Type: constant.ChannelTypeCodex,
		Key: `{"access_token":"fixture-access","account_id":"private-account"}`}
	excluded, eligible, err := CodexQuotaEligibleKeys(context.Background(), channel)
	require.NoError(t, err)
	assert.False(t, eligible)
	assert.True(t, excluded[0])

	require.NoError(t, RecordCodexUsageLimit(context.Background(), channel.Id, channel.Key))
	var marker model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("source = ?", model.CodexQuotaRouteLimitSource).Take(&marker).Error)
	assert.Equal(t, identity.SubjectRef, marker.SubjectRef)
	assert.Empty(t, marker.AccountRef)
	assert.NotContains(t, marker.ErrorMessage, "private-account")
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	excluded, eligible, err = CodexQuotaEligibleKeys(context.Background(), channel)
	require.ErrorIs(t, err, ErrCodexQuotaRoutingUnavailable)
	assert.False(t, eligible, "removing the keyring must not hide known exhaustion")
	assert.Nil(t, excluded)
}

func TestCodexQuotaRoutingFailsClosedWhenDatabaseClockUnavailable(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	channel := &model.Channel{Id: 3501, Type: constant.ChannelTypeCodex,
		Key: `{"access_token":"fixture-access","account_id":"clock-account"}`}
	clockErr := errors.New("database clock unavailable")
	const callbackName = "test:codex-quota-database-clock"
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register(callbackName, func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "strftime('%s','now')") {
			tx.AddError(clockErr)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Row().Remove(callbackName)) })
	excluded, eligible, err := CodexQuotaEligibleKeys(context.Background(), channel)
	require.ErrorIs(t, err, clockErr)
	assert.False(t, eligible)
	assert.Nil(t, excluded)
	err = RecordCodexUsageLimit(context.Background(), channel.Id, channel.Key)
	require.ErrorIs(t, err, clockErr)
	var count int64
	require.NoError(t, db.Model(&model.ChannelQuotaSnapshot{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCodexQuotaRoutingUsesDatabaseClockForResetAndMarkerTime(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	const databaseNow int64 = 1700000000
	channel := &model.Channel{Id: 3502, Type: constant.ChannelTypeCodex,
		Key: `{"access_token":"fixture-access","account_id":"skewed-clock-account"}`}
	require.NoError(t, model.RecordChannelQuotaSnapshot(&model.ChannelQuotaSnapshot{
		ChannelId: channel.Id, AccountRef: model.ChannelQuotaAccountRef("codex", "skewed-clock-account"),
		ObservedAt: databaseNow - 100, Available: 0, ResetAt: databaseNow + 100,
		MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary",
		Status: "success", Unit: "percent", SampleID: "clock-skew-zero",
	}))
	const callbackName = "test:codex-quota-fixed-database-clock"
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.SQL.String() == "SELECT strftime('%s','now')" {
			tx.Statement.SQL.Reset()
			tx.Statement.SQL.WriteString("SELECT 1700000000")
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Row().Remove(callbackName)) })
	excluded, eligible, err := CodexQuotaEligibleKeys(context.Background(), channel)
	require.NoError(t, err)
	assert.False(t, eligible, "a fast node clock must not release a provider reset before database time reaches it")
	assert.True(t, excluded[0])
	require.NoError(t, RecordCodexUsageLimit(context.Background(), channel.Id, channel.Key))
	var marker model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("source = ?", model.CodexQuotaRouteLimitSource).Take(&marker).Error)
	assert.Equal(t, databaseNow, marker.ObservedAt)
	assert.Equal(t, databaseNow+100, marker.ResetAt)
}
