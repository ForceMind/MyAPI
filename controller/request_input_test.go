package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRelayInputLimitRejectsBeforeBillingOrUpstreamAndLogsEstimate(t *testing.T) {
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}))
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldRedis, oldMemoryCache := common.RedisEnabled, common.MemoryCacheEnabled
	oldCount, oldErrorLog, oldSensitive := constant.CountToken, constant.ErrorLogEnabled, setting.CheckSensitiveEnabled
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	constant.CountToken, constant.ErrorLogEnabled, setting.CheckSensitiveEnabled = false, true, false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.RedisEnabled, common.MemoryCacheEnabled = oldRedis, oldMemoryCache
		constant.CountToken, constant.ErrorLogEnabled, setting.CheckSensitiveEnabled = oldCount, oldErrorLog, oldSensitive
		require.NoError(t, sqlDB.Close())
	})
	user := model.User{Username: "input-policy-user", AffCode: "input-policy-user", Quota: 500}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Name: "input-policy-key", Key: "synthetic-input-policy", RemainQuota: 500}
	require.NoError(t, db.Create(&token).Error)
	var dispatched atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatched.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-6.1-sol","input":"hello world"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.RequestIdKey, "local-input-limit-rejection")
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-6.1-sol")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyChannelId, 7)
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeCodex)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{InputTokenLimits: map[string]int64{"gpt-6.1-sol": 1}})
	c.Set("token_name", token.Name)
	Relay(c, types.RelayFormatOpenAIResponses)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":"request_input_limit_exceeded"`)
	assert.Zero(t, dispatched.Load())
	var currentUser model.User
	require.NoError(t, db.First(&currentUser, user.Id).Error)
	var currentToken model.Token
	require.NoError(t, db.First(&currentToken, token.Id).Error)
	assert.EqualValues(t, 500, currentUser.Quota)
	assert.Zero(t, currentUser.UsedQuota)
	assert.EqualValues(t, 500, currentToken.RemainQuota)
	assert.Zero(t, currentToken.UsedQuota)
	var logs []model.Log
	require.NoError(t, db.Where("request_id = ?", "local-input-limit-rejection").Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Zero(t, logs[0].Quota)
	assert.Zero(t, logs[0].PromptTokens)
	assert.Contains(t, logs[0].Other, `"input_tokens":9`)
	assert.Contains(t, logs[0].Other, `"usage_outcome":"not_dispatched"`)
}
