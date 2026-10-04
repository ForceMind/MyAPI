package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestChannelModelDiscoveryHTTPPersistence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelModelDiscovery{}))
	oldDB := model.DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() { model.DB = oldDB; common.SetDatabaseTypes(oldMain, oldLog); require.NoError(t, pool.Close()) })
	malformedEntries := map[int32]string{
		8:  `{"data":[{}]}`,
		9:  `{"data":[null]}`,
		10: `{"data":[{"id":""}]}`,
		11: `{"data":[{"id":null}]}`,
		12: `{"data":[{"id":"   "}]}`,
		13: `{"data":[{"id":"partial-must-not-persist"},{}]}`,
	}
	var mode atomic.Int32
	started, resume := make(chan struct{}, 1), make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer synthetic-secret" {
			http.Error(w, "unexpected request", 400)
			return
		}
		if malformed, exists := malformedEntries[mode.Load()]; exists {
			_, _ = fmt.Fprint(w, malformed)
			return
		}
		switch mode.Load() {
		case 1:
			http.Error(w, "do not disclose synthetic-secret or raw body", 502)
		case 2:
			_, _ = fmt.Fprint(w, `{"data":[]}`)
		case 3:
			_, _ = fmt.Fprint(w, `{"error":{"message":"synthetic-secret"}}`)
		case 6:
			ids := make([]OpenAIModel, 255)
			for i := range ids {
				ids[i].ID = fmt.Sprintf("%03d-", i) + strings.Repeat("x", 251)
			}
			body, err := common.Marshal(map[string]any{"data": ids})
			if err != nil {
				http.Error(w, "synthetic encoding failed", 500)
				return
			}
			_, _ = w.Write(body)
		case 7:
			_, _ = fmt.Fprintf(w, `{"data":[{"id":"%s"}]}`, strings.Repeat("x", 256))
		case 5:
			started <- struct{}{}
			<-r.Context().Done()
		case 4:
			started <- struct{}{}
			<-resume
			_, _ = fmt.Fprint(w, `{"data":[{"id":"obsolete"}]}`)
		default:
			_, _ = fmt.Fprint(w, `{"data":[{"id":"z"},{"id":"a"},{"id":"z"}]}`)
		}
	}))
	defer upstream.Close()
	defer func() {
		select {
		case resume <- struct{}{}:
		default:
		}
	}()
	restoreClient := service.SetHttpClientForTest(upstream.Client())
	defer restoreClient()
	baseURL := upstream.URL
	channel := model.Channel{Type: constant.ChannelTypeOpenAI, Key: "synthetic-secret", BaseURL: &baseURL, Models: "enabled-only"}
	require.NoError(t, db.Create(&channel).Error)
	router := gin.New()
	router.GET("/api/channel/model-discovery/:id", GetChannelModelDiscovery)
	router.POST("/api/channel/model-discovery/:id", RefreshChannelModelDiscovery)
	request := func(method string, id int) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/api/channel/model-discovery/"+strconv.Itoa(id), nil))
		return response
	}
	decode := func(response *httptest.ResponseRecorder) (bool, model.ChannelModelDiscoverySnapshot) {
		t.Helper()
		require.Equal(t, 200, response.Code)
		var envelope struct {
			Success bool                                `json:"success"`
			Data    model.ChannelModelDiscoverySnapshot `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
		assert.NotContains(t, response.Body.String(), "synthetic-secret")
		return envelope.Success, envelope.Data
	}
	_, view := decode(request(http.MethodGet, channel.Id))
	assert.Equal(t, "never_checked", view.Status)
	ok, view := decode(request(http.MethodPost, channel.Id))
	require.True(t, ok)
	assert.Equal(t, []string{"a", "z"}, view.Models)
	assert.False(t, view.Stale)
	_, repeated := decode(request(http.MethodPost, channel.Id))
	assert.Equal(t, view.Models, repeated.Models)
	require.NoError(t, db.First(&channel, channel.Id).Error)
	assert.Equal(t, "enabled-only", channel.Models)

	for _, invalidMode := range []int32{6, 7, 8, 9, 10, 11, 12, 13} {
		mode.Store(invalidMode)
		success, rejected := decode(request(http.MethodPost, channel.Id))
		assert.False(t, success, "valid HTTP response containing invalid persistence evidence must fail safely")
		assert.Equal(t, "failed", rejected.Status)
		assert.True(t, rejected.Stale)
		assert.Equal(t, view.Models, rejected.Models)
		assert.Equal(t, repeated.FetchedAt, rejected.FetchedAt)
		assert.Equal(t, view.Source, rejected.Source)
	}
	mode.Store(1)
	ok, failed := decode(request(http.MethodPost, channel.Id))
	assert.False(t, ok)
	assert.Equal(t, view.Models, failed.Models)
	assert.Equal(t, "failed", failed.Status)
	assert.True(t, failed.Stale)
	assert.Equal(t, repeated.FetchedAt, failed.FetchedAt)
	mode.Store(3)
	ok, malformed := decode(request(http.MethodPost, channel.Id))
	assert.False(t, ok)
	assert.Equal(t, view.Models, malformed.Models)
	mode.Store(2)
	ok, empty := decode(request(http.MethodPost, channel.Id))
	assert.True(t, ok)
	assert.Equal(t, "empty", empty.Status)
	assert.Equal(t, []string{}, empty.Models)
	assert.False(t, empty.Stale)
	mode.Store(4)
	completed := make(chan *httptest.ResponseRecorder, 1)
	go func() { completed <- request(http.MethodPost, channel.Id) }()
	<-started
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("key", "changed-secret").Error)
	resume <- struct{}{}
	ok, changed := decode(<-completed)
	assert.False(t, ok)
	assert.Equal(t, "configuration_changed", changed.Status)
	assert.Empty(t, changed.Models)
	assert.True(t, changed.Stale)

	// Cancellation must terminate metadata I/O and retain a terminal failed
	// attempt in SQL rather than leaving the row permanently refreshing.
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("key", "synthetic-secret").Error)
	mode.Store(5)
	cancelCtx, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	cancelledResponse := make(chan struct{})
	go func() {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/channel/model-discovery/"+strconv.Itoa(channel.Id), nil).WithContext(cancelCtx)
		router.ServeHTTP(response, req)
		close(cancelledResponse)
	}()
	<-started
	cancelRequest()
	<-cancelledResponse
	cancelledView, err := model.GetChannelModelDiscovery(context.Background(), channel.Id)
	require.NoError(t, err)
	assert.Equal(t, "failed", cancelledView.Status)
	assert.True(t, cancelledView.Stale)
	manual := model.Channel{Type: constant.ChannelTypeAnthropic, Key: "not-used", Models: "manual-model"}
	require.NoError(t, db.Create(&manual).Error)
	ok, view = decode(request(http.MethodPost, manual.Id))
	assert.True(t, ok)
	assert.Equal(t, "manual_unverified", view.Status)
	assert.Empty(t, view.Models)

	// The existing Codex metadata adapter is exercised without contacting any
	// real account or release host. Unknown requests fail closed in the transport.
	var codexRequests int
	codexClient := &http.Client{Transport: quotaSamplingRoundTripper(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Path {
		case "/repos/openai/codex/releases/latest":
			body = `{"name":"0.100.0","draft":false,"prerelease":false}`
		case "/backend-api/codex/models":
			codexRequests++
			if r.Header.Get("Authorization") != "Bearer synthetic-codex" || r.Header.Get("ChatGPT-Account-Id") != "synthetic-account" {
				return nil, fmt.Errorf("unexpected synthetic Codex identity")
			}
			body = `{"models":[{"slug":"codex-model"}]}`
		default:
			return nil, fmt.Errorf("unexpected synthetic request path")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	restoreCodex := service.SetHttpClientForTest(codexClient)
	defer restoreCodex()
	codex := model.Channel{Type: constant.ChannelTypeCodex, Key: `{"access_token":"synthetic-codex","account_id":"synthetic-account"}`, Models: "enabled-codex"}
	require.NoError(t, db.Create(&codex).Error)
	ok, view = decode(request(http.MethodPost, codex.Id))
	require.True(t, ok)
	assert.Equal(t, "codex_models", view.Source)
	assert.Equal(t, []string{"codex-model"}, view.Models)
	assert.Equal(t, 1, codexRequests)
	assert.Equal(t, http.StatusNotFound, request(http.MethodGet, channel.Id+manual.Id+10000).Code)
	assert.Equal(t, http.StatusBadRequest, request(http.MethodGet, -1).Code)
}

func TestChannelModelDiscoveryHTTPBounds(t *testing.T) {
	client := &http.Client{Transport: quotaSamplingRoundTripper(func(r *http.Request) (*http.Response, error) {
		if err := r.Context().Err(); err != nil {
			return nil, err
		}
		_, hasDeadline := r.Context().Deadline()
		assert.True(t, hasDeadline, "model discovery must have its own request deadline")
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Repeat(" ", (4<<20)+1))), Request: r}, nil
	})}
	restore := service.SetHttpClientForTest(client)
	defer restore()
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Key: "synthetic-key"}
	_, err := getFetchModelsResponseBody(context.Background(), http.MethodGet, "http://synthetic.invalid/v1/models", channel, nil)
	require.ErrorContains(t, err, "exceeds limit")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = getFetchModelsResponseBody(ctx, http.MethodGet, "http://synthetic.invalid/v1/models", channel, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestExplicitModelRoutesDoNotAutoEnableDiscoveredModels(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"actual"},{"id":"new-upstream-model"}]}`))
	}))
	defer upstream.Close()
	settings := dto.ChannelOtherSettings{UpstreamModelUpdateAutoSyncEnabled: true, ModelRoutes: []dto.ModelRoute{{PublicModel: "public", UpstreamModel: "actual", Match: "exact", Endpoint: "/v1/responses"}}}
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Key: "synthetic", BaseURL: &upstream.URL, Models: "public", Group: "default", Status: common.ChannelStatusEnabled}
	channel.SetOtherSettings(settings)
	require.NoError(t, db.Create(channel).Error)
	changed, added, err := checkAndPersistChannelUpstreamModelUpdates(channel, &settings, true, true)
	require.NoError(t, err)
	require.False(t, changed)
	require.Zero(t, added)
	require.Equal(t, "public", channel.Models)
	require.Contains(t, settings.UpstreamModelUpdateLastDetectedModels, "new-upstream-model")
}
