package controller

import (
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
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func acceptanceControllerContext(method, body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, "/api/upstream-acceptance/runs", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 42)
	c.Set("role", common.RoleRootUser)
	c.Set("session_id", "acceptance-session")
	c.Set("auth_version", int64(1))
	c.Set("session_version", int64(1))
	return c, recorder
}

func setupAcceptanceControllerDatabase(t *testing.T) *model.Channel {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/acceptance-controller.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	previous := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previous
		require.NoError(t, pool.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.UpstreamAcceptanceRun{}, &model.Channel{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	key, err := common.Marshal(service.CodexOAuthKey{AccessToken: "synthetic-acceptance-access", RefreshToken: "synthetic-acceptance-refresh", AccountID: "synthetic-acceptance-account"})
	require.NoError(t, err)
	baseURL := "http://127.0.0.1:1"
	channel := &model.Channel{Type: constant.ChannelTypeCodex, Key: string(key), Name: "synthetic-acceptance", BaseURL: &baseURL}
	require.NoError(t, db.Create(channel).Error)
	return channel
}

func TestUpstreamAcceptanceControllerRequiresRootSessionAndExplicitMode(t *testing.T) {
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
	for _, handler := range []struct {
		name string
		run  gin.HandlerFunc
	}{
		{"start", StartUpstreamAcceptanceRun}, {"status", GetUpstreamAcceptanceRunStatus}, {"stop", StopUpstreamAcceptanceRun},
	} {
		t.Run(handler.name, func(t *testing.T) {
			for _, denied := range []struct {
				name   string
				change func(*gin.Context)
				status int
			}{
				{"missing_session", func(c *gin.Context) { c.Set("session_id", "") }, http.StatusUnauthorized},
				{"personal_access_token", func(c *gin.Context) { c.Set("use_access_token", true) }, http.StatusUnauthorized},
				{"ordinary_user", func(c *gin.Context) { c.Set("role", common.RoleCommonUser) }, http.StatusForbidden},
				{"admin_user", func(c *gin.Context) { c.Set("role", common.RoleAdminUser) }, http.StatusForbidden},
			} {
				t.Run(denied.name, func(t *testing.T) {
					c, recorder := acceptanceControllerContext(http.MethodPost, `{}`)
					denied.change(c)
					handler.run(c)
					assert.Equal(t, denied.status, recorder.Code)
					assert.Contains(t, recorder.Body.String(), `"code":"upstream_acceptance_denied"`)
				})
			}
			t.Run("mode_disabled", func(t *testing.T) {
				t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "false")
				c, recorder := acceptanceControllerContext(http.MethodPost, `{}`)
				handler.run(c)
				assert.Equal(t, http.StatusForbidden, recorder.Code)
			})
		})
	}
}

func TestUpstreamAcceptanceControllerRejectsAmbiguousInput(t *testing.T) {
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
	for _, body := range []string{
		`{}`, `null`, `{"channel_id":0}`, `{"channel_id":1,"unexpected":"synthetic-secret"}`,
		`{"channel_id":1,"channel_id":2}`, `{"channel_id":1,"limits":{"max_total":1,"max_total":2}}`,
		`{"channel_id":1} {"channel_id":2}`, strings.Repeat(" ", 8193),
	} {
		c, recorder := acceptanceControllerContext(http.MethodPost, body)
		StartUpstreamAcceptanceRun(c)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.NotContains(t, recorder.Body.String(), "synthetic-secret")
	}
}

func TestUpstreamAcceptanceControllerLifecycleDoesNotDispatch(t *testing.T) {
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
	channel := setupAcceptanceControllerDatabase(t)
	c, recorder := acceptanceControllerContext(http.MethodPost, fmt.Sprintf(`{"channel_id":%d,"limits":{"duration_seconds":60,"max_total":5,"max_auth":1,"max_refresh":1,"max_sample":1,"max_models":1,"max_generation":1}}`, channel.Id))
	StartUpstreamAcceptanceRun(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var created struct {
		Success bool                        `json:"success"`
		Data    model.UpstreamAcceptanceRun `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &created))
	require.True(t, created.Success)
	require.NotEmpty(t, created.Data.ID)
	t.Cleanup(func() { _, _ = service.StopUpstreamAcceptanceRun(context.Background(), created.Data.ID) })
	assert.Zero(t, created.Data.Total)
	assert.NotContains(t, recorder.Body.String(), "synthetic-acceptance-access")
	assert.NotContains(t, recorder.Body.String(), "synthetic-acceptance-account")
	assert.NotContains(t, recorder.Body.String(), "owner_id")

	c, recorder = acceptanceControllerContext(http.MethodGet, "")
	c.Params = gin.Params{{Key: "id", Value: created.Data.ID}}
	GetUpstreamAcceptanceRunStatus(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var status struct {
		Success bool                             `json:"success"`
		Data    service.UpstreamAcceptanceStatus `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &status))
	require.True(t, status.Success)
	assert.True(t, status.Data.OwnerCurrent)
	assert.True(t, status.Data.CanDispatch)
	assert.Zero(t, status.Data.Operations)
	assert.Zero(t, status.Data.HTTPInFlight)

	c, recorder = acceptanceControllerContext(http.MethodPost, "")
	c.Params = gin.Params{{Key: "id", Value: created.Data.ID}}
	StopUpstreamAcceptanceRun(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &status))
	require.True(t, status.Success)
	require.NotNil(t, status.Data.Run)
	assert.Equal(t, model.UpstreamAcceptanceRunStopped, status.Data.Run.Status)
	assert.False(t, status.Data.CanDispatch)
	assert.Zero(t, status.Data.Run.Total)
}

func TestUpstreamAcceptanceModeRejectsNewChannelOAuth(t *testing.T) {
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
	for _, handler := range []gin.HandlerFunc{StartCodexOAuth, CompleteCodexOAuth} {
		c, recorder := acceptanceControllerContext(http.MethodPost, `{"input":"synthetic-code#synthetic-state"}`)
		handler(c)
		assert.Equal(t, http.StatusForbidden, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"code":"upstream_acceptance_denied"`)
		assert.NotContains(t, recorder.Body.String(), "synthetic-code")
	}
}

func TestUpstreamAcceptanceModeRejectsUnscopedUsage(t *testing.T) {
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
	channel := setupAcceptanceControllerDatabase(t)
	c, recorder := acceptanceControllerContext(http.MethodGet, "")
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}
	calls := 0
	fetchCodexChannelWhamData(c, func(context.Context, *http.Client, string, string, string) (int, []byte, error) {
		calls++
		return http.StatusOK, []byte(`{}`), nil
	}, "synthetic", "synthetic", false)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Zero(t, calls)
	err := sampleCodexCredentialUsage(context.Background(), channel, channel.Key, model.ChannelQuotaResolvedIdentity{}, nil, nil, nil)
	assert.ErrorIs(t, err, service.ErrUpstreamAcceptanceDenied)
}

func TestUpstreamAcceptanceModePausesQuotaSamplingHandler(t *testing.T) {
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
	t.Setenv("CHANNEL_QUOTA_SYNC_ENABLED", "true")
	setupAcceptanceControllerDatabase(t)
	handler := channelQuotaSnapshotSyncHandler{}
	assert.False(t, handler.Enabled())
	task, err := model.CreateSystemTask(model.SystemTaskTypeChannelQuotaSnapshotSync, handler.NewPayload(), nil)
	require.NoError(t, err)
	claimedTask, claimed, err := model.ClaimSystemTask(task.ID, task.Type, "synthetic-runner", time.Now().Add(time.Minute).Unix())
	require.NoError(t, err)
	require.True(t, claimed)
	handler.Run(context.Background(), claimedTask, "synthetic-runner")
	var stored model.SystemTask
	require.NoError(t, model.DB.Where("id = ?", task.ID).First(&stored).Error)
	assert.Equal(t, model.SystemTaskStatusFailed, stored.Status)
}
