package middleware

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCodexQuotaMiddlewareDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.NoError(t, i18n.Init())
	previousDB := model.DB
	previousCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousCache
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	return db
}

func setupCodexQuotaMiddlewareTest(t *testing.T) (*gorm.DB, *model.Channel) {
	t.Helper()
	db := setupCodexQuotaMiddlewareDB(t)
	channel := &model.Channel{
		Type: constant.ChannelTypeCodex, Name: "quota exhausted Codex",
		Key:    `{"access_token":"fixture-access","account_id":"fixture-account"}`,
		Status: common.ChannelStatusEnabled, Models: "gpt-5-codex", Group: "default",
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, model.RecordChannelQuotaSnapshotWithContext(context.Background(), &model.ChannelQuotaSnapshot{
		ChannelId: channel.Id, AccountRef: model.ChannelQuotaAccountRef("codex", "fixture-account"),
		ObservedAt: time.Now().Unix(), SampleID: "exhausted-sample", Available: 0,
		MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary",
		Status: "success", Unit: "percent", ResetAt: time.Now().Add(time.Hour).Unix(),
	}))
	return db, channel
}

func TestCodexQuotaSetupRejectsExhaustedAccountBeforeSelectingKey(t *testing.T) {
	_, channel := setupCodexQuotaMiddlewareTest(t)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")

	apiErr := SetupContextForSelectedChannel(ctx, channel, "gpt-5-codex")
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	assert.Equal(t, types.ErrorCodeChannelNoAvailableKey, apiErr.GetErrorCode())
	_, selected := common.GetContextKey(ctx, constant.ContextKeyChannelKey)
	assert.False(t, selected)
}

func TestCodexQuotaSetupRejectsFreshZeroWithAlreadyPassedResetBeforeSelectingKey(t *testing.T) {
	db, channel := setupCodexQuotaMiddlewareTest(t)
	require.NoError(t, db.Model(&model.ChannelQuotaSnapshot{}).
		Where("channel_id = ?", channel.Id).
		Update("reset_at", time.Now().Add(-time.Minute).Unix()).Error)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")

	apiErr := SetupContextForSelectedChannel(ctx, channel, "gpt-5-codex")
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	_, selected := common.GetContextKey(ctx, constant.ContextKeyChannelKey)
	assert.False(t, selected)
}

func TestCodexQuotaAdministratorDiagnosticCanProbeExhaustedAccount(t *testing.T) {
	_, channel := setupCodexQuotaMiddlewareTest(t)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	require.Nil(t, SetupContextForChannelTest(ctx, channel, "gpt-5-codex"))
	selected, ok := common.GetContextKey(ctx, constant.ContextKeyChannelKey)
	require.True(t, ok)
	assert.Equal(t, channel.Key, selected)
}

func TestCodexQuotaDistributorRejectsPinnedExhaustedChannelBeforeHandler(t *testing.T) {
	_, channel := setupCodexQuotaMiddlewareTest(t)
	gin.SetMode(gin.TestMode)
	called := false
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, strconv.Itoa(channel.Id))
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		c.Next()
	})
	router.Use(Distribute())
	router.POST("/v1/responses", func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5-codex","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.False(t, called)
	assert.Contains(t, response.Body.String(), string(types.ErrorCodeChannelNoAvailableKey))
}

func TestCodexQuotaDistributorHoldsUnknownResetUntilHealthySample(t *testing.T) {
	db := setupCodexQuotaMiddlewareDB(t)
	channel := &model.Channel{
		Type: constant.ChannelTypeCodex, Name: "unknown reset Codex", Status: common.ChannelStatusEnabled,
		Key:    `{"access_token":"fixture-access","account_id":"unknown-reset-account"}`,
		Models: "gpt-5-codex", Group: "default",
	}
	require.NoError(t, db.Create(channel).Error)
	accountRef := model.ChannelQuotaAccountRef("codex", "unknown-reset-account")
	require.NoError(t, model.RecordChannelQuotaSnapshotWithContext(context.Background(), &model.ChannelQuotaSnapshot{
		ChannelId: channel.Id, AccountRef: accountRef, ObservedAt: time.Now().Add(-time.Hour).Unix(),
		SampleID: "old-usage-limit", MetricType: "codex_rate_limit", WindowType: "none",
		Source: model.CodexQuotaRouteLimitSource, Status: "error", ErrorCode: model.CodexQuotaRouteLimitCode,
		Unit: "percent", ResetAt: 0,
	}))
	gin.SetMode(gin.TestMode)
	called := 0
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, strconv.Itoa(channel.Id))
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		c.Next()
	})
	router.Use(Distribute())
	router.POST("/v1/responses", func(c *gin.Context) {
		called++
		c.Status(http.StatusOK)
	})
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5-codex","input":"hello"}`))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	blocked := request()
	assert.Equal(t, http.StatusServiceUnavailable, blocked.Code)
	assert.Zero(t, called)

	require.NoError(t, model.RecordChannelQuotaSnapshotWithContext(context.Background(), &model.ChannelQuotaSnapshot{
		ChannelId: channel.Id, AccountRef: accountRef, ObservedAt: time.Now().Unix(),
		SampleID: "new-healthy-usage", MetricType: "codex_rate_limit", WindowType: "five_hour",
		Source: "codex_wham_usage_primary", Status: "success", Unit: "percent", Available: 60,
		ResetAt: time.Now().Add(time.Hour).Unix(),
	}))
	allowed := request()
	assert.Equal(t, http.StatusOK, allowed.Code)
	assert.Equal(t, 1, called)
}

func TestCodexQuotaDistributorPollingSkipsExhaustedKeyBeforeHandler(t *testing.T) {
	db := setupCodexQuotaMiddlewareDB(t)
	channel := &model.Channel{
		Type: constant.ChannelTypeCodex, Name: "polling Codex accounts", Status: common.ChannelStatusEnabled,
		Key:    `{"access_token":"fixture-a","account_id":"account-a"}` + "\n" + `{"access_token":"fixture-b","account_id":"account-b"}`,
		Models: "gpt-5-codex", Group: "default",
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModePolling},
	}
	require.NoError(t, db.Create(channel).Error)
	now := time.Now().Unix()
	for _, item := range []struct {
		account   string
		available float64
	}{
		{account: "account-a", available: 60},
		{account: "account-b", available: 60},
	} {
		require.NoError(t, model.RecordChannelQuotaSnapshotWithContext(context.Background(), &model.ChannelQuotaSnapshot{
			ChannelId: channel.Id, AccountRef: model.ChannelQuotaAccountRef("codex", item.account),
			ObservedAt: now - 1, SampleID: "polling-" + item.account, Available: item.available,
			MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary",
			Status: "success", Unit: "percent", ResetAt: now + 3600,
		}))
	}
	require.NoError(t, service.RecordCodexUsageLimit(context.Background(), channel.Id,
		`{"access_token":"fixture-a","account_id":"account-a"}`))
	gin.SetMode(gin.TestMode)
	fixed, _ := gin.CreateTestContext(httptest.NewRecorder())
	fixed.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	apiErr := SetupContextForFixedChannelKey(fixed, channel, "gpt-5-codex", 0)
	require.NotNil(t, apiErr, "a frozen key cannot bypass a known quota hold")
	assert.Empty(t, common.GetContextKeyString(fixed, constant.ContextKeyChannelKey))
	fixed, _ = gin.CreateTestContext(httptest.NewRecorder())
	fixed.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	pollingIndex := channel.ChannelInfo.MultiKeyPollingIndex
	require.Nil(t, SetupContextForFixedChannelKey(fixed, channel, "gpt-5-codex", 1))
	assert.Contains(t, common.GetContextKeyString(fixed, constant.ContextKeyChannelKey), `"account_id":"account-b"`)
	assert.Equal(t, pollingIndex, channel.ChannelInfo.MultiKeyPollingIndex)
	selected := ""
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, strconv.Itoa(channel.Id))
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		c.Next()
	})
	router.Use(Distribute())
	router.POST("/v1/responses", func(c *gin.Context) {
		selected = common.GetContextKeyString(c, constant.ContextKeyChannelKey)
		c.Status(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5-codex","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, selected, `"account_id":"account-b"`)
	assert.NotContains(t, selected, `"account_id":"account-a"`)
	require.NoError(t, service.RecordCodexUsageLimit(context.Background(), channel.Id,
		`{"access_token":"fixture-b","account_id":"account-b"}`))
	selected = ""
	request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5-codex","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.Empty(t, selected)
}

func TestCodexQuotaDistributorAcceptsPairedAbsenceAfterOldZeroAnd429(t *testing.T) {
	db := setupCodexQuotaMiddlewareDB(t)
	channel := &model.Channel{
		Type: constant.ChannelTypeCodex, Name: "changed-plan Codex", Status: common.ChannelStatusEnabled,
		Key:    `{"access_token":"fixture-access","account_id":"changed-plan-account"}`,
		Models: "gpt-5-codex", Group: "default",
	}
	require.NoError(t, db.Create(channel).Error)
	accountRef := model.ChannelQuotaAccountRef("codex", "changed-plan-account")
	now := time.Now().Unix()
	for _, snapshot := range []model.ChannelQuotaSnapshot{
		{ChannelId: channel.Id, AccountRef: accountRef, ObservedAt: now - 30, SampleID: "old-zero",
			MetricType: "codex_rate_limit", WindowType: "weekly", Source: "codex_wham_usage_secondary", Status: "success", Unit: "percent", Available: 0},
		{ChannelId: channel.Id, AccountRef: accountRef, ObservedAt: now - 20, SampleID: "provider-429",
			MetricType: "codex_rate_limit", WindowType: "none", Source: model.CodexQuotaRouteLimitSource,
			Status: "error", ErrorCode: model.CodexQuotaRouteLimitCode, Unit: "percent"},
		{ChannelId: channel.Id, AccountRef: accountRef, ObservedAt: now - 10, SampleID: "new-plan-batch",
			MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary", Status: "success", Unit: "percent", Available: 70},
		{ChannelId: channel.Id, AccountRef: accountRef, ObservedAt: now - 10, SampleID: "new-plan-batch",
			MetricType: "codex_rate_limit", WindowType: "weekly", Source: "codex_wham_usage_secondary", Status: "unsupported", ErrorCode: "window_absent", Unit: "percent"},
	} {
		item := snapshot
		require.NoError(t, model.RecordChannelQuotaSnapshotWithContext(context.Background(), &item))
	}
	gin.SetMode(gin.TestMode)
	called := false
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, strconv.Itoa(channel.Id))
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		c.Next()
	})
	router.Use(Distribute())
	router.POST("/v1/responses", func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5-codex","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.True(t, called)
}

func TestCodexQuotaAffinityBypassesExhaustedAccountForHealthyChannel(t *testing.T) {
	db := setupCodexQuotaMiddlewareDB(t)
	priorityA, priorityB := int64(100), int64(50)
	weight := uint(1)
	for _, item := range []struct {
		name     string
		account  string
		priority *int64
	}{
		{name: "preferred", account: "account-a", priority: &priorityA},
		{name: "fallback", account: "account-b", priority: &priorityB},
	} {
		channel := &model.Channel{
			Type: constant.ChannelTypeCodex, Name: item.name, Status: common.ChannelStatusEnabled,
			Key:    fmt.Sprintf(`{"access_token":"fixture-%s","account_id":"%s"}`, item.name, item.account),
			Models: "gpt-5-codex", Group: "default", Priority: item.priority, Weight: &weight,
		}
		require.NoError(t, db.Create(channel).Error)
		require.NoError(t, db.Create(&model.Ability{
			Group: "default", Model: "gpt-5-codex", ChannelId: channel.Id,
			Enabled: true, Priority: item.priority, Weight: weight,
		}).Error)
	}

	previous := operation_setting.GetChannelAffinitySetting()
	registry := config.GlobalConfig.Get("channel_affinity_setting")
	require.NotNil(t, registry)
	originalRules, err := common.Marshal(previous.Rules)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registry, map[string]string{
			"enabled": strconv.FormatBool(previous.Enabled), "rules": string(originalRules),
		}))
	})
	rule := operation_setting.ChannelAffinityRule{
		Name: "codex-quota-affinity-test", ModelRegex: []string{"^gpt-5-codex$"}, PathRegex: []string{"^/v1/responses$"},
		KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Affinity-Test"}},
		TTLSeconds: 60, IncludeRuleName: true, IncludeModelName: true,
	}
	rules, err := common.Marshal([]operation_setting.ChannelAffinityRule{rule})
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(registry, map[string]string{"enabled": "true", "rules": string(rules)}))
	gin.SetMode(gin.TestMode)
	selected := make([]int, 0, 2)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		c.Next()
	})
	router.Use(Distribute())
	router.POST("/v1/responses", func(c *gin.Context) {
		selected = append(selected, c.GetInt("channel_id"))
		c.Status(http.StatusOK)
	})
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5-codex","input":"hello"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Affinity-Test", t.Name())
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	first := request()
	require.Equal(t, http.StatusOK, first.Code)
	require.Len(t, selected, 1)
	preferredID := selected[0]
	var preferred model.Channel
	require.NoError(t, db.Where("id = ?", preferredID).Take(&preferred).Error)
	assert.Equal(t, "preferred", preferred.Name)
	require.NoError(t, model.RecordChannelQuotaSnapshotWithContext(context.Background(), &model.ChannelQuotaSnapshot{
		ChannelId: preferredID, AccountRef: model.ChannelQuotaAccountRef("codex", "account-a"),
		ObservedAt: time.Now().Unix(), SampleID: "affinity-exhausted", Available: 0,
		MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary",
		Status: "success", Unit: "percent", ResetAt: time.Now().Add(time.Hour).Unix(),
	}))
	second := request()
	require.Equal(t, http.StatusOK, second.Code)
	require.Len(t, selected, 2)
	var fallback model.Channel
	require.NoError(t, db.Where("id = ?", selected[1]).Take(&fallback).Error)
	assert.Equal(t, "fallback", fallback.Name)
	assert.NotEqual(t, preferredID, fallback.Id)
}

func TestAccountThresholdPinnedFixedAndDiagnosticCannotBypassLowWindow(t *testing.T) {
	db := setupCodexQuotaMiddlewareDB(t)
	baseURL := "https://chatgpt.com"
	channel := &model.Channel{Type: constant.ChannelTypeCodex, BaseURL: &baseURL, Name: "threshold accounts", Status: common.ChannelStatusEnabled, Models: "gpt-5-codex", Group: "default", Key: `{"access_token":"fixture-a","account_id":"threshold-low"}` + "\n" + `{"access_token":"fixture-b","account_id":"threshold-high"}`, ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModePolling}}
	require.NoError(t, db.Create(channel).Error)
	now, err := model.ReadDatabaseUnixTime(context.Background(), db)
	require.NoError(t, err)
	for _, account := range []struct {
		id   string
		used float64
	}{{"threshold-low", 90}, {"threshold-high", 10}} {
		total := float64(100)
		used := account.used
		require.NoError(t, db.Create(&model.ChannelQuotaSnapshot{ChannelId: channel.Id, AccountRef: model.ChannelQuotaAccountRef("codex", account.id), ObservedAt: now, SampleID: account.id, Available: 100 - used, Used: &used, Total: &total, MetricType: "codex_rate_limit", WindowType: "five_hour", WindowSeconds: 18000, ResetAt: now + 18000, Source: "codex_wham_usage_primary", Status: "success", Unit: "percent", CodexThresholdQualified: true}).Error)
	}
	attach := func(c *gin.Context) {
		c.Request = c.Request.WithContext(common.WithAccountQuotaThreshold(c.Request.Context(), common.AccountQuotaThreshold{MinimumRemainingBPS: 2000, MaxAgeSeconds: 300}))
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	}
	for _, index := range []int{0, 1} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		attach(c)
		err := SetupContextForFixedChannelKey(c, channel, "gpt-5-codex", index)
		if index == 0 {
			require.NotNil(t, err)
			assert.Empty(t, common.GetContextKeyString(c, constant.ContextKeyChannelKey))
		} else {
			require.Nil(t, err)
			assert.Contains(t, common.GetContextKeyString(c, constant.ContextKeyChannelKey), "threshold-high")
		}
	}
	low := *channel
	low.Key = strings.Split(channel.Key, "\n")[0]
	low.ChannelInfo = model.ChannelInfo{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	attach(c)
	require.NotNil(t, SetupContextForChannelTest(c, &low, "gpt-5-codex"), "a token-bound policy cannot borrow the administrator diagnostic bypass")
	selected := ""
	router := gin.New()
	router.Use(func(c *gin.Context) {
		attach(c)
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, strconv.Itoa(channel.Id))
		c.Next()
	})
	router.Use(Distribute())
	router.POST("/v1/responses", func(c *gin.Context) {
		selected = common.GetContextKeyString(c, constant.ContextKeyChannelKey)
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5-codex","input":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, selected, "threshold-high")
}
