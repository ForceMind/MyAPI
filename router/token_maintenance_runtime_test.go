package router

import (
	"encoding/base64"
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
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This opt-in journey runs the production frontend and the application's real
// routes against a disposable database. The browser's other console reads are
// explicitly synthetic; all maintenance and verification calls reach Go.
func TestTokenMaintenanceRuntimeBrowser(t *testing.T) {
	if os.Getenv("MYAPI_CREDENTIAL_BROWSER_TESTS") != "1" {
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
			common.SessionSecret = "maintenance-runtime-isolated-session-secret"
			t.Cleanup(func() {
				common.MemoryCacheEnabled, common.SessionSecret = previousMemory, previousSecret
				common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable = previousCritical, previousGlobal
			})
			t.Setenv(common.ChannelQuotaIdentityKeysEnv, "active:v1:"+base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("m", 32))))
			t.Setenv("MYAPI_EDITION", "full")
			entities := []any{&model.UserSession{}, &model.TwoFA{}, &model.TwoFABackupCode{}, &model.TokenAccountMigrationReceipt{},
				&model.ChannelQuotaIdentityKeyRegistry{}, &model.ChannelQuotaIdentityKeyVersion{}, &model.ChannelQuotaIdentityAlias{},
				&model.ChannelQuotaSamplingTarget{}, &model.ChannelQuotaSnapshot{}, &model.TokenBudgetReservation{},
				&model.QuotaWriterEpoch{}, &model.QuotaWorkCursor{},
				&model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{},
				&model.QuotaBalanceBatchDrain{}, &model.QuotaBalanceBatchSubject{},
				&model.LegacyUsageReservation{}, &model.AccountQuotaSettlementIntent{}, &model.AccountQuotaSettlementFact{},
				&model.AccountQuotaRefundFact{}, &model.QuotaProjectionObligation{},
				&model.Task{}, &model.TaskSubmissionOperation{}, &model.Midjourney{},
				&model.TokenCredentialDispatchAdmission{}, &model.TokenCredentialRotationReceipt{}}
			entities = append(entities, model.SubscriptionShareModels()...)
			require.NoError(t, db.AutoMigrate(entities...))
			root := model.User{Id: 1, Username: "maintenance-runtime-root", AffCode: "maintenance-runtime-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AuthVersion: 1, Group: "default", Quota: 1000, UsedQuota: 21}
			require.NoError(t, db.Create(&root).Error)
			require.NoError(t, db.Create(&model.TwoFA{UserId: root.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}).Error)
			identity, accessToken := createCodexLocalRouteSession(t, &root)
			rotationProof, _, err := service.IssueSecurityProof(identity, "2fa", []string{"token.credential_rotation"})
			require.NoError(t, err)
			token := model.Token{Id: 21, UserId: root.Id, Key: "synthetic-runtime-maintenance-key-never-return", Name: "synthetic-key", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000, UsedQuota: 21, Group: "default"}
			require.NoError(t, db.Create(&token).Error)
			keyring, err := common.LoadChannelQuotaIdentityKeyring()
			require.NoError(t, err)
			now := time.Now().Unix()
			writer := model.QuotaWriterEpoch{ID: 1, SchemaVersion: model.QuotaWriterEpochSchemaVersion, Mode: string(model.QuotaWriterModeLegacy), Epoch: 1, LockVersion: 1, UpdatedAt: now}
			require.NoError(t, db.Create(&writer).Error)
			for _, name := range []string{"account_quota_migration_v2", "quota_balance_migration_v2"} {
				require.NoError(t, db.Create(&model.QuotaWorkCursor{Name: name, Complete: true, UpdatedAt: now}).Error)
			}
			poolIDs := []int64{}
			for index, id := range []int{7, 8} {
				name := "Synthetic source account"
				if index == 1 {
					name = "Synthetic target account"
				}
				channel := model.Channel{Id: id, Type: constant.ChannelTypeCodex, Name: name, Status: common.ChannelStatusEnabled,
					Key: fmt.Sprintf(`{"access_token":"synthetic-access-%d","refresh_token":"synthetic-refresh-%d","account_id":"synthetic-account-%d"}`, id, id, id), Models: "synthetic-text-model", Group: "default"}
				require.NoError(t, db.Create(&channel).Error)
				credential, err := model.ResolveChannelQuotaIdentity(t.Context(), db, keyring, "channel_type_57", common.ChannelQuotaIdentityKindCredential, []byte(channel.Key))
				require.NoError(t, err)
				account, err := model.ResolveChannelQuotaIdentity(t.Context(), db, keyring, "channel_type_57", common.ChannelQuotaIdentityKindProviderAccount, []byte(fmt.Sprintf("synthetic-account-%d", id)))
				require.NoError(t, err)
				require.NoError(t, model.EnsureChannelQuotaSamplingTarget(t.Context(), db, id, model.ChannelQuotaSamplingIdentity{SubjectRef: credential.SubjectRef, IdentityQuality: credential.Quality}))
				used, total := float64(5), float64(100)
				snapshot := model.ChannelQuotaSnapshot{ChannelId: id, SubjectRef: account.SubjectRef, IdentityQuality: model.ChannelQuotaIdentityQualityProviderConfirmed,
					ObservedAt: now - 2, Available: 95, Used: &used, Total: &total, Unit: "percent", MetricType: "codex_rate_limit", WindowType: "five_hour", WindowSeconds: 18000,
					ResetAt: now + 10000, Source: "codex_wham_usage_primary", Status: "success", PlanType: "plus", SampleID: fmt.Sprintf("runtime-%d-a", id), CodexObservationQualified: true, CodexThresholdQualified: true}
				require.NoError(t, db.Create(&snapshot).Error)
				share := 2500
				if index == 1 {
					share = 0
				}
				require.NoError(t, model.ConfigureSubscriptionShare(t.Context(), db, root.Id, token.Id, model.SubscriptionShareConfigInput{ID: fmt.Sprintf("%064x", id), SnapshotID: snapshot.Id, ShareBPS: share, ConfirmedApproximate: true}, now-2))
				var pool model.SubscriptionSharePool
				require.NoError(t, db.Where("subject_ref = ?", account.SubjectRef).Take(&pool).Error)
				poolIDs = append(poolIDs, pool.ID)
				snapshot.Id, snapshot.ObservedAt, snapshot.SampleID = 0, now-1, fmt.Sprintf("runtime-%d-b", id)
				require.NoError(t, db.Create(&snapshot).Error)
				require.NoError(t, model.RefreshSubscriptionShareProjection(t.Context(), db, account.SubjectRef, now))
				var samplingTarget model.ChannelQuotaSamplingTarget
				require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", id, credential.SubjectRef).Take(&samplingTarget).Error)
				require.NoError(t, model.MarkChannelQuotaSamplingTargetAttempt(t.Context(), db, id, credential.SubjectRef, "sampled", account.SubjectRef,
					model.ChannelQuotaSamplingProof{ExpectedVersion: samplingTarget.EvidenceVersion, SampleID: snapshot.SampleID}))
			}
			policy, err := model.ConfigureAssignedAccessPolicy(t.Context(), db, "token", token.Id, 0, `{"enabled":true,"public_models":null,"upstream_models":null,"channel_ids":[7]}`)
			require.NoError(t, err)
			var targetAllocation model.SubscriptionShareAllocation
			require.NoError(t, db.Where("pool_id = ? AND token_id = ?", poolIDs[1], token.Id).Take(&targetAllocation).Error)
			require.NoError(t, db.Model(&model.SubscriptionShareAllocationUsage{}).Where("allocation_id = ?", targetAllocation.ID).Update("estimated_used_micro", int64(2_000_000)).Error)
			var beforeToken model.Token
			require.NoError(t, db.First(&beforeToken, token.Id).Error)
			var beforeUsage []model.SubscriptionShareAllocationUsage
			require.NoError(t, db.Order("id ASC").Find(&beforeUsage).Error)
			var beforeSnapshots []model.ChannelQuotaSnapshot
			require.NoError(t, db.Order("id ASC").Find(&beforeSnapshots).Error)
			var beforeGenerations []model.SubscriptionShareGeneration
			require.NoError(t, db.Order("id ASC").Find(&beforeGenerations).Error)
			engine := gin.New()
			engine.Use(func(c *gin.Context) { common.SetContextKey(c, constant.ContextKeyAuditLogged, true); c.Next() })
			SetApiRouter(engine)
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
			output := os.Getenv("MYAPI_CREDENTIAL_RUNTIME_ARTIFACTS")
			require.NotEmpty(t, output)
			output = filepath.Join(output, fmt.Sprintf("%s-%d", scene.language, scene.width))
			require.NoError(t, os.MkdirAll(output, 0700))
			version, err := os.ReadFile(filepath.Join(repo, "VERSION"))
			require.NoError(t, err)
			config := map[string]any{"origin": server.URL, "language": scene.language, "width": scene.width, "access_token": accessToken,
				"session":     map[string]any{"sid": identity.SessionID, "current": true, "login_method": "password", "expires_at": now + 3600},
				"user":        map[string]any{"id": 1, "username": root.Username, "role": common.RoleRootUser, "status": common.UserStatusEnabled, "group": "default", "auth_version": 1},
				"totp_secret": "JBSWY3DPEHPK3PXP", "rotation_proof": rotationProof, "token_id": token.Id, "target_channel_id": 8, "target_channel_name": "Synthetic target account",
				"output_dir": output, "version": strings.TrimSpace(string(version)), "source_head": os.Getenv("GITHUB_SHA")}
			encoded, err := common.Marshal(config)
			require.NoError(t, err)
			configPath := filepath.Join(t.TempDir(), "private-synthetic-browser.json")
			require.NoError(t, os.WriteFile(configPath, encoded, 0600))
			command := exec.CommandContext(t.Context(), "node", filepath.Join(repo, "tools", "runtime", "credential-maintenance-browser.mjs"), configPath)
			command.Dir = repo
			result, err := command.CombinedOutput()
			require.NoError(t, err, "isolated browser failed: %s", result)
			var afterToken model.Token
			require.NoError(t, db.First(&afterToken, token.Id).Error)
			assert.Equal(t, beforeToken, afterToken, "all Key metadata, secret and financial values stay unchanged")
			var afterUsage []model.SubscriptionShareAllocationUsage
			require.NoError(t, db.Order("id ASC").Find(&afterUsage).Error)
			assert.Equal(t, beforeUsage, afterUsage, "existing source and target usage rows are preserved")
			var afterSnapshots []model.ChannelQuotaSnapshot
			require.NoError(t, db.Order("id ASC").Find(&afterSnapshots).Error)
			assert.Equal(t, beforeSnapshots, afterSnapshots)
			var afterGenerations []model.SubscriptionShareGeneration
			require.NoError(t, db.Order("id ASC").Find(&afterGenerations).Error)
			assert.Equal(t, beforeGenerations, afterGenerations)
			var allocations []model.SubscriptionShareAllocation
			require.NoError(t, db.Order("pool_id ASC").Find(&allocations).Error)
			require.Len(t, allocations, 2)
			assert.Zero(t, allocations[0].ShareBPS)
			assert.Equal(t, 2500, allocations[1].ShareBPS)
			assert.Equal(t, targetAllocation.ID, allocations[1].ID)
			afterPolicy, err := model.ReadAssignedAccessPolicy(t.Context(), db, "token", token.Id)
			require.NoError(t, err)
			assert.Equal(t, policy.Revision+1, afterPolicy.Revision)
			assert.JSONEq(t, `{"enabled":true,"public_models":null,"upstream_models":null,"channel_ids":[8]}`, afterPolicy.PolicyJSON)
			var receiptCount int64
			require.NoError(t, db.Model(&model.TokenAccountMigrationReceipt{}).Count(&receiptCount).Error)
			assert.Equal(t, int64(1), receiptCount)
			var rotationCount int64
			require.NoError(t, db.Model(&model.TokenCredentialRotationReceipt{}).Count(&rotationCount).Error)
			assert.Zero(t, rotationCount, "legacy qualification rejection must not create a rotation receipt")
			var afterWriter model.QuotaWriterEpoch
			require.NoError(t, db.First(&afterWriter, writer.ID).Error)
			assert.Equal(t, writer, afterWriter)
			var afterRoot model.User
			require.NoError(t, db.First(&afterRoot, root.Id).Error)
			assert.Equal(t, root.Quota, afterRoot.Quota)
			assert.Equal(t, root.UsedQuota, afterRoot.UsedQuota)
			t.Log("production frontend and real Go migration/proof/receipt routes passed; isolated history, Key identity, target usage and balances preserved")
		})
	}
}
