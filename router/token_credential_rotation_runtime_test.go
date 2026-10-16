package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Opt-in production frontend -> real session/proof/rotation routes. Fixtures
// and credentials are disposable; unrelated console reads are explicitly
// synthetic. No provider request or production initialization is performed.
func TestTokenCredentialRotationRuntimeBrowser(t *testing.T) {
	if os.Getenv("MYAPI_CREDENTIAL_ROTATION_BROWSER_TESTS") != "1" {
		t.Skip("requires the isolated Actions production build and Chromium")
	}
	for _, scene := range []struct {
		language string
		width    int
	}{{"en", 1440}, {"zh", 390}} {
		t.Run(fmt.Sprintf("%s-%d", scene.language, scene.width), func(t *testing.T) {
			setupRelayRouterTestDB(t)
			require.NoError(t, i18n.Init())
			db := model.DB
			previousMemory, previousSecret := common.MemoryCacheEnabled, common.SessionSecret
			previousCritical, previousGlobal := common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable
			common.MemoryCacheEnabled, common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable = false, false, false
			common.SessionSecret = "rotation-runtime-isolated-session-secret"
			t.Cleanup(func() {
				common.MemoryCacheEnabled, common.SessionSecret = previousMemory, previousSecret
				common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable = previousCritical, previousGlobal
			})
			t.Setenv("MYAPI_EDITION", "full")
			entities := []any{
				&model.UserSession{}, &model.TwoFA{}, &model.TwoFABackupCode{}, &model.UserUsagePolicyChange{},
				&model.ChannelQuotaIdentityKeyRegistry{}, &model.ChannelQuotaIdentityKeyVersion{}, &model.ChannelQuotaIdentityAlias{},
				&model.ChannelQuotaSamplingTarget{}, &model.ChannelQuotaSnapshot{}, &model.TokenBudgetReservation{},
				&model.QuotaWriterEpoch{}, &model.QuotaWorkCursor{},
				&model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{},
				&model.QuotaBalanceBatchDrain{}, &model.QuotaBalanceBatchSubject{},
				&model.LegacyUsageReservation{}, &model.AccountQuotaSettlementIntent{}, &model.AccountQuotaSettlementFact{},
				&model.AccountQuotaRefundFact{}, &model.QuotaProjectionObligation{},
				&model.Task{}, &model.TaskSubmissionOperation{}, &model.Midjourney{},
				&model.TokenCredentialDispatchAdmission{}, &model.TokenCredentialRotationReceipt{},
			}
			entities = append(entities, model.SubscriptionShareModels()...)
			require.NoError(t, db.AutoMigrate(entities...))
			root := model.User{Id: 1, Username: "rotation-runtime-root", AffCode: "rotation-runtime-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AuthVersion: 1, Group: "default", Quota: 1000, UsedQuota: 21}
			require.NoError(t, db.Create(&root).Error)
			require.NoError(t, db.Create(&model.TwoFA{UserId: root.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}).Error)
			identity, accessToken := createCodexLocalRouteSession(t, &root)
			token := model.Token{Id: 21, UserId: root.Id, Key: strings.Repeat("K", 48), Name: "synthetic-key", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000, UsedQuota: 21, CredentialVersion: 1}
			require.NoError(t, db.Create(&token).Error)
			now := time.Now().Unix()
			writer := model.QuotaWriterEpoch{ID: 1, SchemaVersion: model.QuotaWriterEpochSchemaVersion, Mode: string(model.QuotaWriterModeAuthoritative), Epoch: 1, LockVersion: 1, UpdatedAt: now}
			require.NoError(t, db.Create(&writer).Error)
			for _, name := range []string{"account_quota_migration_v2", "quota_balance_migration_v2"} {
				require.NoError(t, db.Create(&model.QuotaWorkCursor{Name: name, Complete: true, UpdatedAt: now}).Error)
			}
			budget := model.TokenBudget{TokenID: token.Id, UserID: root.Id, Limit: 10000, Used: 123, Revision: 1}
			require.NoError(t, db.Create(&budget).Error)
			policy, err := model.ConfigureAssignedAccessPolicy(t.Context(), db, "token", token.Id, 0, `{"enabled":true,"public_models":["synthetic-model"],"upstream_models":null,"channel_ids":null}`)
			require.NoError(t, err)
			history := model.Log{UserId: root.Id, Username: root.Username, TokenId: token.Id, Type: model.LogTypeConsume, Content: "synthetic frozen history", Quota: 7, PromptTokens: 2, CompletionTokens: 3}
			require.NoError(t, model.CreateLog(db, &history))
			preview, err := model.PreviewTokenCredentialRotation(t.Context(), db, root.Id, token.Id, now)
			require.NoError(t, err)
			require.Equal(t, model.TokenMaintenanceStatusReady, preview.Status)
			var beforeToken model.Token
			require.NoError(t, db.First(&beforeToken, token.Id).Error)
			engine := gin.New()
			engine.Use(func(c *gin.Context) { common.SetContextKey(c, constant.ContextKeyAuditLogged, true); c.Next() })
			SetApiRouter(engine)
			engine.GET("/api/rotation-test/auth", middleware.TokenAuth(), func(c *gin.Context) {
				common.ApiSuccess(c, gin.H{"token_id": c.GetInt("token_id")})
			})
			repo, err := filepath.Abs("..")
			require.NoError(t, err)
			dist := filepath.Join(repo, "web", "dist")
			require.FileExists(t, filepath.Join(dist, "index.html"))
			engine.NoRoute(func(c *gin.Context) {
				if c.Request.Method != http.MethodGet || strings.HasPrefix(c.Request.URL.Path, "/api/") {
					c.Status(http.StatusNotFound)
					return
				}
				path := filepath.Join(dist, filepath.Clean("/"+c.Request.URL.Path))
				relative, err := filepath.Rel(dist, path)
				if err != nil || strings.HasPrefix(relative, "..") {
					c.Status(http.StatusForbidden)
					return
				}
				info, err := os.Stat(path)
				if err != nil || info.IsDir() {
					path = filepath.Join(dist, "index.html")
				}
				c.Header("Cache-Control", "no-store")
				c.File(path)
			})
			server := httptest.NewServer(engine)
			t.Cleanup(server.Close)
			output := os.Getenv("MYAPI_ROTATION_RUNTIME_ARTIFACTS")
			require.NotEmpty(t, output)
			output = filepath.Join(output, fmt.Sprintf("%s-%d", scene.language, scene.width))
			require.NoError(t, os.MkdirAll(output, 0700))
			version, err := os.ReadFile(filepath.Join(repo, "VERSION"))
			require.NoError(t, err)
			config := map[string]any{"origin": server.URL, "language": scene.language, "width": scene.width, "access_token": accessToken,
				"session":     map[string]any{"sid": identity.SessionID, "current": true, "login_method": "password", "expires_at": now + 3600},
				"user":        map[string]any{"id": 1, "username": root.Username, "role": common.RoleRootUser, "status": common.UserStatusEnabled, "group": "default", "auth_version": 1},
				"totp_secret": "JBSWY3DPEHPK3PXP", "synthetic_old_key": "sk-" + token.Key, "token_id": token.Id,
				"output_dir": output, "version": strings.TrimSpace(string(version)), "source_head": os.Getenv("GITHUB_SHA")}
			encoded, err := common.Marshal(config)
			require.NoError(t, err)
			configPath := filepath.Join(t.TempDir(), "private-synthetic-rotation-browser.json")
			require.NoError(t, os.WriteFile(configPath, encoded, 0600))
			command := exec.CommandContext(t.Context(), "node", filepath.Join(repo, "tools", "runtime", "credential-rotation-browser.mjs"), configPath)
			command.Dir = repo
			result, err := command.CombinedOutput()
			require.NoError(t, err, "isolated rotation browser failed: %s", result)
			var afterToken model.Token
			require.NoError(t, db.First(&afterToken, token.Id).Error)
			require.NotEqual(t, beforeToken.Key, afterToken.Key)
			assert.Equal(t, int64(2), afterToken.CredentialVersion)
			afterToken.Key, afterToken.CredentialVersion = beforeToken.Key, beforeToken.CredentialVersion
			assert.Equal(t, beforeToken, afterToken, "all non-credential Key fields are preserved")
			var afterRoot model.User
			require.NoError(t, db.First(&afterRoot, root.Id).Error)
			assert.Equal(t, root.Quota, afterRoot.Quota)
			assert.Equal(t, root.UsedQuota, afterRoot.UsedQuota)
			var afterBudget model.TokenBudget
			require.NoError(t, db.First(&afterBudget, "token_id = ?", token.Id).Error)
			assert.Equal(t, budget, afterBudget)
			afterPolicy, err := model.ReadAssignedAccessPolicy(t.Context(), db, "token", token.Id)
			require.NoError(t, err)
			assert.Equal(t, policy, afterPolicy)
			var afterWriter model.QuotaWriterEpoch
			require.NoError(t, db.First(&afterWriter, writer.ID).Error)
			assert.Equal(t, writer, afterWriter)
			var afterHistory model.Log
			require.NoError(t, db.First(&afterHistory, history.Id).Error)
			assert.Equal(t, history, afterHistory)
			var receiptCount int64
			require.NoError(t, db.Model(&model.TokenCredentialRotationReceipt{}).Count(&receiptCount).Error)
			assert.Equal(t, int64(1), receiptCount)
			t.Log("real Root session, dedicated 2FA, fresh rotation, secret-free replay/receipt and old/new Key authentication passed; ID, metadata, policy, budgets, writer and history preserved")
		})
	}
}
