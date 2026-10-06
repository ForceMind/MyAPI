package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDetailedChannelTestSuccessIsReplayedByOneClickTest(t *testing.T) {
	require.NoError(t, i18n.Init())
	type upstreamRequest struct {
		Path   string
		Model  string
		Stream bool
	}
	codexEvent, err := common.Marshal(map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id": "resp-fixture", "object": "response", "model": "gpt-5-codex", "status": "completed",
			"output": []any{},
			"usage":  map[string]int{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
		},
	})
	require.NoError(t, err)
	requests := make(chan upstreamRequest, 6)
	var holdNextCodex atomic.Bool
	codexTestStarted := make(chan struct{}, 1)
	resumeCodexTest := make(chan struct{}, 1)
	defer func() {
		select {
		case resumeCodexTest <- struct{}{}:
		default:
		}
	}()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := common.DecodeJson(r.Body, &body); err != nil {
			http.Error(w, "invalid synthetic request", http.StatusBadRequest)
			return
		}
		requests <- upstreamRequest{Path: r.URL.Path, Model: body.Model, Stream: body.Stream}
		w.Header().Set("Content-Type", "text/event-stream")
		if r.URL.Path == "/backend-api/codex/responses" {
			if holdNextCodex.CompareAndSwap(true, false) {
				codexTestStarted <- struct{}{}
				<-resumeCodexTest
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", codexEvent)
			return
		}
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-fixture\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.Log{}))
	responseTimeWrites := make(chan struct{}, 5)
	require.NoError(t, db.Callback().Update().After("gorm:update").Register("test:channel_response_time_saved", func(tx *gorm.DB) {
		if tx.Statement.Table != "channels" {
			return
		}
		for _, selected := range tx.Statement.Selects {
			if selected == "response_time" {
				responseTimeWrites <- struct{}{}
				return
			}
		}
	}))
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousMemory, previousConsumeLog := common.RedisEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousModelRatios := ratio_setting.ModelRatio2JSONString()
	previousGroupRatios := ratio_setting.GroupRatio2JSONString()
	previousStreamingTimeout := constant.StreamingTimeout
	previousMode := gin.Mode()
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled = false, false, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitColumnNamesForTest()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o-mini":1,"gpt-5-codex":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	constant.StreamingTimeout = 30
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled = previousRedis, previousMemory, previousConsumeLog
		common.SetDatabaseTypes(previousMain, previousLog)
		model.InitColumnNamesForTest()
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatios))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatios))
		constant.StreamingTimeout = previousStreamingTimeout
		gin.SetMode(previousMode)
		require.NoError(t, sqlDB.Close())
	})

	user := model.User{Username: "test-options-user", AffCode: "test-options-aff", Group: "default", Status: common.UserStatusEnabled, Quota: 1000000, AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	baseURL := upstream.URL
	channel := model.Channel{Type: constant.ChannelTypeOpenAI, Name: "synthetic-openai", Key: "synthetic-key", BaseURL: &baseURL, Models: "gpt-4o-mini"}
	require.NoError(t, db.Create(&channel).Error)

	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("id", user.Id); c.Next() })
	engine.POST("/api/channel/test/:id", TestChannel)
	engine.GET("/api/channel/test/:id", TestChannel)
	request := func(channelID int, method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/channel/test/"+strconv.Itoa(channelID), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		select {
		case <-responseTimeWrites:
		case <-time.After(2 * time.Second):
			t.Fatalf("channel response-time update did not finish: %s", response.Body.String())
		}
		return response
	}

	detailed := request(channel.Id, http.MethodPost, `{"model":"gpt-4o-mini","endpoint_type":"openai","stream":true}`)
	var detailedBody struct {
		Success      bool `json:"success"`
		OptionsSaved bool `json:"options_saved"`
	}
	require.NoError(t, common.Unmarshal(detailed.Body.Bytes(), &detailedBody))
	require.True(t, detailedBody.Success, detailed.Body.String())
	assert.True(t, detailedBody.OptionsSaved)
	assert.Equal(t, upstreamRequest{Path: "/v1/chat/completions", Model: "gpt-4o-mini", Stream: true}, <-requests)

	saved, err := model.GetLastSuccessfulChannelTestOptions(channel.Id)
	require.NoError(t, err)
	require.Equal(t, &model.ChannelTestOptions{Model: "gpt-4o-mini", EndpointType: "openai", Stream: true, ChannelType: channel.Type}, saved)

	oneClick := request(channel.Id, http.MethodGet, "")
	var oneClickBody struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(oneClick.Body.Bytes(), &oneClickBody))
	require.True(t, oneClickBody.Success, oneClick.Body.String())
	assert.Equal(t, upstreamRequest{Path: "/v1/chat/completions", Model: "gpt-4o-mini", Stream: true}, <-requests)

	codex := model.Channel{
		Type: constant.ChannelTypeCodex, Name: "synthetic-codex",
		Key:     `{"access_token":"synthetic-token","account_id":"synthetic-account"}`,
		BaseURL: &baseURL, Models: "gpt-5-codex",
	}
	require.NoError(t, db.Create(&codex).Error)
	codexRequest := upstreamRequest{Path: "/backend-api/codex/responses", Model: "gpt-5-codex", Stream: true}
	firstCodexTest := request(codex.Id, http.MethodGet, "")
	var firstCodexBody struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(firstCodexTest.Body.Bytes(), &firstCodexBody))
	require.True(t, firstCodexBody.Success, firstCodexTest.Body.String())
	assert.Equal(t, codexRequest, <-requests)

	codexDetailed := request(codex.Id, http.MethodPost, `{"model":"gpt-5-codex","endpoint_type":"openai-response","stream":true}`)
	require.NoError(t, common.Unmarshal(codexDetailed.Body.Bytes(), &detailedBody))
	require.True(t, detailedBody.Success, codexDetailed.Body.String())
	assert.True(t, detailedBody.OptionsSaved)
	assert.Equal(t, codexRequest, <-requests)
	savedCodex, err := model.GetLastSuccessfulChannelTestOptions(codex.Id)
	require.NoError(t, err)
	require.Equal(t, &model.ChannelTestOptions{Model: "gpt-5-codex", EndpointType: "openai-response", Stream: true, ChannelType: codex.Type}, savedCodex)

	codexOneClick := request(codex.Id, http.MethodGet, "")
	require.NoError(t, common.Unmarshal(codexOneClick.Body.Bytes(), &oneClickBody))
	require.True(t, oneClickBody.Success, codexOneClick.Body.String())
	assert.Equal(t, codexRequest, <-requests)

	// A concurrent administrator edit can remove the tested model after the
	// request was sent but before the successful upstream response arrives.
	holdNextCodex.Store(true)
	staleResponse := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/api/channel/test/"+strconv.Itoa(codex.Id),
			strings.NewReader(`{"model":"gpt-5-codex","endpoint_type":"openai-response","stream":true}`))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		staleResponse <- response
	}()
	select {
	case <-codexTestStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("held Codex test did not reach the synthetic upstream")
	}
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", codex.Id).Update("models", "gpt-5").Error)
	resumeCodexTest <- struct{}{}
	var staleDetailed *httptest.ResponseRecorder
	select {
	case staleDetailed = <-staleResponse:
	case <-time.After(5 * time.Second):
		t.Fatal("held Codex test did not complete")
	}
	select {
	case <-responseTimeWrites:
	case <-time.After(2 * time.Second):
		t.Fatal("held Codex response time was not saved")
	}
	require.NoError(t, common.Unmarshal(staleDetailed.Body.Bytes(), &detailedBody))
	assert.True(t, detailedBody.Success, staleDetailed.Body.String())
	assert.False(t, detailedBody.OptionsSaved)
	assert.Equal(t, codexRequest, <-requests)
	savedCodex, err = model.GetLastSuccessfulChannelTestOptions(codex.Id)
	require.NoError(t, err)
	assert.Equal(t, &model.ChannelTestOptions{Model: "gpt-5-codex", EndpointType: "openai-response", Stream: true, ChannelType: codex.Type}, savedCodex)
}

func TestResolveChannelTestOptionsReplaysLastSuccessfulManualChoicesAndDefaultsCodexToStream(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})

	channel := model.Channel{Type: constant.ChannelTypeCodex, Name: "codex", Key: "private-key", Models: "gpt-5-codex,gpt-5"}
	require.NoError(t, db.Create(&channel).Error)

	defaultOptions, err := resolveChannelTestOptions(&channel, url.Values{})
	require.NoError(t, err)
	assert.True(t, defaultOptions.Stream)
	assert.Empty(t, defaultOptions.Model)

	saved := model.ChannelTestOptions{Model: "gpt-5-codex", EndpointType: "openai-response", Stream: true, ChannelType: channel.Type}
	require.NoError(t, model.SaveLastSuccessfulChannelTestOptions(channel.Id, saved))
	replayed, err := resolveChannelTestOptions(&channel, url.Values{})
	require.NoError(t, err)
	assert.Equal(t, saved, replayed)

	manual, err := resolveChannelTestOptions(&channel, url.Values{
		"model":         {"gpt-5"},
		"endpoint_type": {"openai"},
		"stream":        {"false"},
	})
	require.NoError(t, err)
	assert.Equal(t, "gpt-5", manual.Model)
	assert.Equal(t, "openai", manual.EndpointType)
	assert.False(t, manual.Stream)

	channel.Models = "gpt-5"
	stale, err := resolveChannelTestOptions(&channel, url.Values{})
	require.NoError(t, err)
	assert.Empty(t, stale.Model)
	assert.True(t, stale.Stream)

	channel.Type = constant.ChannelTypeOpenAI
	wrongType, err := resolveChannelTestOptions(&channel, url.Values{})
	require.NoError(t, err)
	assert.False(t, wrongType.Stream)
}

func TestDetailedChannelTestFailureDoesNotReplaceLastSuccessfulOptions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	previousDB := model.DB
	previousMemory := common.MemoryCacheEnabled
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemory
		require.NoError(t, sqlDB.Close())
	})

	channel := model.Channel{Type: constant.ChannelTypeMidjourney, Name: "unsupported", Key: "private-key", Models: "last-good-model,new-model"}
	require.NoError(t, db.Create(&channel).Error)
	previous := model.ChannelTestOptions{Model: "last-good-model", Stream: false, ChannelType: channel.Type}
	require.NoError(t, model.SaveLastSuccessfulChannelTestOptions(channel.Id, previous))

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}
	context.Set("id", 1)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test/"+strconv.Itoa(channel.Id), strings.NewReader(`{"model":"new-model","stream":true}`))

	TestChannel(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	loaded, err := model.GetLastSuccessfulChannelTestOptions(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, &previous, loaded)
}
