package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	taskdto "github.com/ForceMind/MyAPI/dto"
	appI18n "github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func setupDurableRelayTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.NoError(t, appI18n.Init())
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("2b", 32))
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	t.Cleanup(func() {
		_ = sqlDB.Close()
		model.DB = oldDB
		model.LOG_DB = oldLogDB
	})

	err = db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.SubscriptionPlan{},
		&model.UserSubscription{},
		&model.Channel{},
		&model.Task{},
		&model.TaskRecoveryIdentity{},
		&model.TaskSubmissionOperation{},
		&model.TaskSubmissionAttempt{},
		&model.TaskBillingEvent{},
		&model.TaskBillingLogOutbox{},
		&model.QuotaMutationReceipt{},
		&model.Log{},
	)
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	return db
}

func TestRejectDurableTaskQuotaClamp_ReturnsHTTP400AndPersistsAdminAudit(t *testing.T) {
	db := setupDurableRelayTestDB(t)
	user := model.User{Username: "clamp-audit-user", Status: common.UserStatusEnabled, Quota: 1000}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"prompt":"oversized"}`))
	c.Set(common.RequestIdKey, "req-clamp-audit")
	clamp := &common.QuotaClamp{Op: "QuotaFromFloat", Kind: common.QuotaClampOverflow, Original: 1e30, Clamped: common.MaxQuota}

	handled := rejectDurableTaskQuotaClamp(c, &relaycommon.RelayInfo{
		UserId: user.Id, OriginModelName: "kling", QuotaClamp: clamp,
	})
	require.True(t, handled)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "quota_out_of_range")

	var audit model.Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", user.Id, model.LogTypeSystem).First(&audit).Error)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(audit.Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "req-clamp-audit", adminInfo["request_id"])
	assert.NotNil(t, adminInfo["quota_saturation"])
}

func TestRelayTask_GateOff_ByDefault(t *testing.T) {
	// By default, common.IsTaskRecoveryNewSubmissionEnabled() must be false
	require.False(t, common.IsTaskRecoveryNewSubmissionEnabled())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"prompt":"test"}`))
	req.Header.Set("Idempotency-Key", "test-key-gate-off")
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	// RelayTask should attempt legacy flow which fails with invalid_api_platform (or legacy error)
	// because durable ingress is gated off.
	RelayTask(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid_api_platform")
}

func TestRelayTask_GateOn_MissingIdempotencyKey_FallsThrough(t *testing.T) {
	prev := common.TaskRecoveryNewSubmissionsEnabled
	common.TaskRecoveryNewSubmissionsEnabled = true
	t.Cleanup(func() { common.TaskRecoveryNewSubmissionsEnabled = prev })
	require.True(t, common.IsTaskRecoveryNewSubmissionEnabled())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"prompt":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	// No Idempotency-Key header -> falls through to legacy flow
	RelayTask(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid_api_platform")
}

func TestRelayTask_GateOn_InvalidIdempotencyKey(t *testing.T) {
	db := setupDurableRelayTestDB(t)
	prev := common.TaskRecoveryNewSubmissionsEnabled
	common.TaskRecoveryNewSubmissionsEnabled = true
	t.Cleanup(func() { common.TaskRecoveryNewSubmissionsEnabled = prev })
	require.True(t, common.IsTaskRecoveryNewSubmissionEnabled())

	user := model.User{Username: "test_user_inv", Status: common.UserStatusEnabled, Quota: 100000}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "sk-test-inv", Status: common.TokenStatusEnabled, RemainQuota: 100000, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"prompt":"test"}`))
	// Invalid key with space
	req.Header.Set("Idempotency-Key", "invalid key with space")
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Set("id", user.Id)
	c.Set("token_id", token.Id)
	c.Set("token_group", "default")

	// Channel
	channel := model.Channel{
		Type:    constant.ChannelTypeKling,
		Name:    "test-kling",
		Key:     "test-key",
		BaseURL: common.GetPointer("https://api.test.com"),
		Status:  common.ChannelStatusEnabled,
		Group:   "default",
		Models:  "kling",
		Other:   `{"channel_type":"kling"}`,
	}
	require.NoError(t, db.Create(&channel).Error)

	RelayTask(c)
	// Invalid idempotency key should produce 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Contains(t, w.Body.String(), "invalid_idempotency_key")
}

func TestRelayTask_GateOn_MultipleIdempotencyKeys(t *testing.T) {
	prev := common.TaskRecoveryNewSubmissionsEnabled
	common.TaskRecoveryNewSubmissionsEnabled = true
	t.Cleanup(func() { common.TaskRecoveryNewSubmissionsEnabled = prev })

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"prompt":"test"}`))
	req.Header.Add("Idempotency-Key", "first-key")
	req.Header.Add("Idempotency-Key", "second-key")
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	RelayTask(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Contains(t, w.Body.String(), "invalid_idempotency_key")
}

func TestRelayTask_GateOn_AliasIdempotencyKeyRejected(t *testing.T) {
	prev := common.TaskRecoveryNewSubmissionsEnabled
	common.TaskRecoveryNewSubmissionsEnabled = true
	t.Cleanup(func() { common.TaskRecoveryNewSubmissionsEnabled = prev })

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"prompt":"test"}`))
	req.Header.Set("Idempotency-Key", "valid-key")
	req.Header.Set("X-Idempotency-Key", "alias-key")
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	RelayTask(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Contains(t, w.Body.String(), "invalid_idempotency_key")
}

func TestRelayTask_GateOn_OnlyAliasIdempotencyKeyRejected(t *testing.T) {
	prev := common.TaskRecoveryNewSubmissionsEnabled
	common.TaskRecoveryNewSubmissionsEnabled = true
	t.Cleanup(func() { common.TaskRecoveryNewSubmissionsEnabled = prev })

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"prompt":"test"}`))
	req.Header.Set("X-Idempotency-Key", "alias-only-key")
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	RelayTask(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Contains(t, w.Body.String(), "invalid_idempotency_key")
}

func TestRelayTask_GateOn_SuccessAcceptedAndReplay(t *testing.T) {
	db := setupDurableRelayTestDB(t)
	prev := common.TaskRecoveryNewSubmissionsEnabled
	common.TaskRecoveryNewSubmissionsEnabled = true
	t.Cleanup(func() { common.TaskRecoveryNewSubmissionsEnabled = prev })

	oldPrice, hasOldPrice := ratio_setting.GetModelPrice("kling", false)
	_ = ratio_setting.UpdateModelPriceByJSONString(`{"kling": 0.05}`)
	t.Cleanup(func() {
		if hasOldPrice {
			_ = ratio_setting.UpdateModelPriceByJSONString(fmt.Sprintf(`{"kling": %f}`, oldPrice))
		}
	})

	var mockCallCount int32
	mockClient := &http.Client{
		Transport: testRoundTripper(func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&mockCallCount, 1)
			body := `{"code":0,"message":"SUCCEED","data":{"task_id":"upstream-kling-1001"}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		}),
	}
	cleanupClient := service.SetHttpClientForTest(mockClient)
	t.Cleanup(cleanupClient)

	user := model.User{Username: "test_user_e2e", Status: common.UserStatusEnabled, Quota: 500000}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "sk-test-e2e", Status: common.TokenStatusEnabled, RemainQuota: 500000, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)

	channel := model.Channel{
		Type:    constant.ChannelTypeKling,
		Name:    "test-kling-e2e",
		Key:     "sk-mock-key",
		BaseURL: common.GetPointer("https://api.klingai.com"),
		Status:  common.ChannelStatusEnabled,
		Group:   "default",
		Models:  "kling",
		Other:   `{"channel_type":"kling"}`,
	}
	require.NoError(t, db.Create(&channel).Error)

	gin.SetMode(gin.TestMode)

	// 1. Initial successful submission
	payload := `{"model":"kling","prompt":"a serene mountain lake at sunrise"}`
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	req1 := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(payload))
	req1.Header.Set("Idempotency-Key", "e2e-idempotency-key-01")
	req1.Header.Set("Content-Type", "application/json")
	c1.Request = req1
	c1.Set("id", user.Id)
	c1.Set("token_id", token.Id)
	c1.Set("token_group", "default")
	require.Nil(t, middleware.SetupContextForSelectedChannel(c1, &channel, "kling"))

	RelayTask(c1)

	require.Equal(t, http.StatusAccepted, w1.Code)
	assert.Equal(t, "no-store", w1.Header().Get("Cache-Control"))
	location1 := w1.Header().Get("Location")
	assert.NotEmpty(t, location1)
	assert.True(t, strings.HasPrefix(location1, "/v1/task-operations/"))

	var resp1 taskdto.TaskOperationResponse
	require.NoError(t, common.Unmarshal(w1.Body.Bytes(), &resp1))
	assert.Equal(t, "accepted", resp1.Status)
	assert.NotEmpty(t, resp1.ID)
	assert.Equal(t, "/v1/task-operations/"+resp1.ID, location1)
	assert.Equal(t, int32(1), atomic.LoadInt32(&mockCallCount))

	// Verify user quota was deducted
	var refreshedUser model.User
	require.NoError(t, db.First(&refreshedUser, user.Id).Error)
	assert.Less(t, refreshedUser.Quota, 500000)
	deductedQuota := 500000 - refreshedUser.Quota

	// 2. Idempotent Replay (same key and body)
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	req2 := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(payload))
	req2.Header.Set("Idempotency-Key", "e2e-idempotency-key-01")
	req2.Header.Set("Content-Type", "application/json")
	c2.Request = req2
	c2.Set("id", user.Id)
	c2.Set("token_id", token.Id)
	c2.Set("token_group", "default")
	require.Nil(t, middleware.SetupContextForSelectedChannel(c2, &channel, "kling"))

	RelayTask(c2)

	require.Equal(t, http.StatusAccepted, w2.Code)
	assert.Equal(t, "no-store", w2.Header().Get("Cache-Control"))
	assert.Equal(t, location1, w2.Header().Get("Location"))

	var resp2 taskdto.TaskOperationResponse
	require.NoError(t, common.Unmarshal(w2.Body.Bytes(), &resp2))
	assert.Equal(t, resp1.ID, resp2.ID)
	assert.Equal(t, "accepted", resp2.Status)

	// Verify mock server was NOT called again (still 1) and NO double deduction
	assert.Equal(t, int32(1), atomic.LoadInt32(&mockCallCount))
	var refreshedUser2 model.User
	require.NoError(t, db.First(&refreshedUser2, user.Id).Error)
	assert.Equal(t, 500000-deductedQuota, refreshedUser2.Quota)

	// 3. Conflict (same key, different body)
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	conflictPayload := `{"model":"kling","prompt":"different prompt"}`
	req3 := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(conflictPayload))
	req3.Header.Set("Idempotency-Key", "e2e-idempotency-key-01")
	req3.Header.Set("Content-Type", "application/json")
	c3.Request = req3
	c3.Set("id", user.Id)
	c3.Set("token_id", token.Id)
	c3.Set("token_group", "default")
	require.Nil(t, middleware.SetupContextForSelectedChannel(c3, &channel, "kling"))

	RelayTask(c3)

	assert.Equal(t, http.StatusConflict, w3.Code)
	assert.Equal(t, "no-store", w3.Header().Get("Cache-Control"))
	assert.Contains(t, w3.Body.String(), "idempotency_conflict")

	// Quota still unchanged
	var refreshedUser3 model.User
	require.NoError(t, db.First(&refreshedUser3, user.Id).Error)
	assert.Equal(t, 500000-deductedQuota, refreshedUser3.Quota)
}

func TestRejectDurableTaskQuotaClamp_LogDBUnavailableDoesNotFallbackOrChange400(t *testing.T) {
	db := setupDurableRelayTestDB(t)
	model.LOG_DB = nil
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	clamp := &common.QuotaClamp{Op: "QuotaFromFloat", Kind: common.QuotaClampOverflow, Original: 1e30, Clamped: common.MaxQuota}
	require.True(t, rejectDurableTaskQuotaClamp(c, &relaycommon.RelayInfo{UserId: 1, OriginModelName: "kling", QuotaClamp: clamp}))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "quota_out_of_range")
	var count int64
	require.NoError(t, db.Model(&model.Log{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestRejectDurableTaskQuotaClamp_LogDBFailureDoesNotChange400(t *testing.T) {
	db := setupDurableRelayTestDB(t)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail-clamp-log", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "logs" {
			tx.AddError(errors.New("injected log failure"))
		}
	}))
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	clamp := &common.QuotaClamp{Op: "QuotaFromFloat", Kind: common.QuotaClampOverflow, Original: 1e30, Clamped: common.MaxQuota}
	require.True(t, rejectDurableTaskQuotaClamp(c, &relaycommon.RelayInfo{UserId: 1, OriginModelName: "kling", QuotaClamp: clamp}))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "quota_out_of_range")
}
