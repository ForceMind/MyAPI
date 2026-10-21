package router

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type offlineResponseWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *offlineResponseWriter) Write(data []byte) (int, error) {
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

// The Node CLI talks to actual setup/login/channel/token/run/logout routes.
// No injected identity, fake JWT, provider transport or background scheduler is
// used. The server's HTTP dialer is a deny-all tripwire, not a host firewall.
func TestUpstreamAcceptanceOfflineScript(t *testing.T) {
	node, err := exec.LookPath("node")
	require.NoError(t, err, "Node is required for the real CLI integration")
	setupRelayRouterTestDB(t)
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.Setup{}, &model.Option{}, &model.UserSession{}, &model.TwoFA{}, &model.UpstreamAcceptanceRun{}))
	require.NoError(t, i18n.Init())
	previousSetup, previousSecret := constant.Setup, common.SessionSecret
	previousMemory := common.MemoryCacheEnabled
	previousTransport := http.DefaultTransport
	previousSelfUse, previousDemo := operation_setting.SelfUseModeEnabled, operation_setting.DemoSiteEnabled
	previousFunding := operation_setting.GetUserFundingSetting()
	previousOptions := common.OptionMap
	constant.Setup, common.MemoryCacheEnabled = false, false
	common.SessionSecret = "synthetic-offline-script-session-secret"
	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
	t.Setenv("MYAPI_EDITION", "full")
	var outbound atomic.Int32
	baseTransport, ok := previousTransport.(*http.Transport)
	require.True(t, ok)
	transport := baseTransport.Clone()
	transport.Proxy = nil
	transport.DialContext = func(context.Context, string, string) (net.Conn, error) {
		outbound.Add(1)
		return nil, errors.New("synthetic offline fixture denies all outbound")
	}
	http.DefaultTransport = transport
	restoreClient := service.SetHttpClientForTest(&http.Client{Transport: transport, Timeout: time.Second})
	t.Cleanup(func() {
		restoreClient()
		http.DefaultTransport = previousTransport
		transport.CloseIdleConnections()
		constant.Setup, common.SessionSecret = previousSetup, previousSecret
		common.MemoryCacheEnabled = previousMemory
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		operation_setting.SelfUseModeEnabled, operation_setting.DemoSiteEnabled = previousSelfUse, previousDemo
		require.NoError(t, operation_setting.PublishUserFundingSnapshot(previousFunding.Mode, previousFunding.Epoch))
	})
	// Positive control establishes that a provider client cannot dial externally.
	_, err = service.GetHttpClient().Get("https://provider.invalid/offline-tripwire")
	require.Error(t, err)
	require.Equal(t, int32(1), outbound.Swap(0))

	engine := gin.New()
	var diagnosticMutex sync.Mutex
	var diagnostic string
	engine.Use(func(c *gin.Context) {
		writer := &offlineResponseWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		c.Next()
		var result struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Code    string `json:"code"`
			Data    struct {
				IDs []int `json:"ids"`
			} `json:"data"`
		}
		if common.Unmarshal(writer.body.Bytes(), &result) == nil && (!result.Success || c.Request.URL.Path == "/api/channel/") {
			diagnosticMutex.Lock()
			diagnostic = c.Request.Method + " " + c.Request.URL.Path + ": " + result.Code + " " + result.Message
			if result.Success {
				diagnostic = fmt.Sprintf("channel returned IDs: %v", result.Data.IDs)
			}
			diagnosticMutex.Unlock()
		}
	})
	SetApiRouter(engine)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	parent := t.TempDir()
	sentinel := filepath.Join(parent, "keep.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("preserve"), 0600))
	script, err := filepath.Abs("../tools/runtime/upstream-acceptance-offline.mjs")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, script, "prepare", "--isolated", "--synthetic", "--base-url", server.URL, "--parent-dir", parent)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.Output()
	diagnosticMutex.Lock()
	lastDiagnostic := diagnostic
	diagnosticMutex.Unlock()
	require.NoError(t, err, "redacted CLI error: %s; synthetic API diagnostic: %s", stderr.String(), lastDiagnostic)
	var report struct {
		Success   bool   `json:"success"`
		StateDir  string `json:"state_dir"`
		ChannelID int    `json:"channel_id"`
		TokenID   int    `json:"token_id"`
		RunID     string `json:"run_id"`
	}
	require.NoError(t, common.Unmarshal(stdout, &report))
	require.True(t, report.Success)
	require.NotEmpty(t, report.StateDir)
	var root model.User
	require.NoError(t, db.First(&root).Error)
	assert.Regexp(t, `^at[a-f0-9]{10}$`, root.Username)
	assert.Equal(t, common.RoleRootUser, root.Role)
	assert.NotEmpty(t, root.Password)
	assert.NotEqual(t, root.Username, root.Password)
	assert.Zero(t, root.UsedQuota)
	assert.Zero(t, root.RequestCount)
	var session model.UserSession
	require.NoError(t, db.First(&session).Error)
	assert.Equal(t, "password", session.LoginMethod)
	assert.Equal(t, model.UserSessionStatusActive, session.Status)
	var channel model.Channel
	require.NoError(t, db.First(&channel, report.ChannelID).Error)
	assert.Contains(t, channel.Key, "synthetic-offline-access-")
	assert.Contains(t, channel.Key, "synthetic-offline-refresh-")
	var token model.Token
	require.NoError(t, db.First(&token, report.TokenID).Error)
	assert.Equal(t, root.Id, token.UserId)
	assert.True(t, token.ModelLimitsEnabled)
	assert.Equal(t, "synthetic-offline-model", token.ModelLimits)
	assert.Zero(t, token.RemainQuota)
	assert.False(t, token.UnlimitedQuota)
	stateBytes, err := os.ReadFile(filepath.Join(report.StateDir, "state.json"))
	require.NoError(t, err)
	var state struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, common.Unmarshal(stateBytes, &state))
	assert.NotContains(t, string(stdout), state.AccessToken)
	assert.NotContains(t, string(stdout), token.Key)
	assert.Zero(t, outbound.Load(), "preparation must never try a server outbound connection")

	command = exec.CommandContext(ctx, node, script, "cleanup", "--state-dir", report.StateDir)
	stderr.Reset()
	command.Stderr = &stderr
	stdout, err = command.Output()
	require.NoError(t, err, "redacted CLI error: %s", stderr.String())
	assert.Contains(t, string(stdout), `"session_revoked":true`)
	_, err = os.Stat(report.StateDir)
	assert.True(t, os.IsNotExist(err), "only script-owned private files are removed")
	content, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	assert.Equal(t, "preserve", string(content))
	require.NoError(t, db.First(&session, "sid = ?", session.SID).Error)
	assert.Equal(t, model.UserSessionStatusRevoked, session.Status)
	require.NoError(t, db.First(&token, report.TokenID).Error)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)
	var run model.UpstreamAcceptanceRun
	require.NoError(t, db.First(&run, "id = ?", report.RunID).Error)
	assert.Equal(t, model.UpstreamAcceptanceRunStopped, run.Status)
	assert.Zero(t, run.Total)
	assert.Zero(t, run.MaxAuth)
	assert.Zero(t, run.MaxRefresh)
	assert.Zero(t, run.MaxSample)
	assert.Zero(t, run.MaxModels)
	assert.Zero(t, run.MaxGeneration)
	assert.Zero(t, run.GenerationInFlight)
	assert.Zero(t, outbound.Load(), "complete lifecycle must have zero server outbound attempts")
	// The test fixture owns the server/database lifecycle; the CLI never deletes
	// an operator's database. t.TempDir removes the DB after the server/pool close.
	t.Log("actual setup/password login/preseed/stop/Key disable/logout; zero server outbound attempts; private files removed, unrelated file retained")
}
