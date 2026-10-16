package service

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRefreshCodexCredentialElementPreservesOtherAccountsAndRejectsConflicts(t *testing.T) {
	for _, scenario := range []string{"rotate one account", "single formatted credential", "request canceled after rotation", "changed before refresh", "changed during refresh", "duplicate credential", "invalid expiry", "single invalid expiry", "different account", "unknown refreshed account", "unknown existing subject"} {
		t.Run(scenario, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.SystemTaskLock{}))
			previousDB, previousCache := model.DB, common.MemoryCacheEnabled
			model.DB, common.MemoryCacheEnabled = db, false
			t.Cleanup(func() { model.DB, common.MemoryCacheEnabled = previousDB, previousCache; _ = sqlDB.Close() })
			old := `{"access_token":"synthetic-old","refresh_token":"synthetic-refresh","account_id":"account-a"}`
			other := `{"access_token":"synthetic-other","refresh_token":"synthetic-other-refresh","account_id":"account-b"}`
			channel := model.Channel{Type: constant.ChannelTypeCodex, Key: "[" + old + "," + other + "]", ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
			if scenario == "single formatted credential" || scenario == "single invalid expiry" {
				old = " " + strings.ReplaceAll(old, ",", ",\n") + " "
				channel.Key, channel.ChannelInfo.IsMultiKey = old, false
			}
			if scenario == "duplicate credential" {
				channel.Key = "[" + old + "," + old + "]"
			}
			require.NoError(t, db.Create(&channel).Error)
			if scenario != "unknown existing subject" {
				establishCodexCredentialSubjectEvidence(t, db, channel.Id, old, "account-a")
			}
			if scenario == "different account" || scenario == "rotate one account" {
				establishCodexCredentialSubjectEvidence(t, db, channel.Id, other, "account-b")
			}
			if scenario == "changed before refresh" {
				require.NoError(t, db.Model(&channel).Update("key", "operator-before").Error)
			}
			client, err := GetHttpClientWithProxy("")
			require.NoError(t, err)
			previousTransport := client.Transport
			t.Cleanup(func() { client.Transport = previousTransport })
			requests := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client.Transport = codexCredentialRefreshRoundTripper(func(request *http.Request) (*http.Response, error) {
				requests++
				require.NoError(t, request.ParseForm())
				assert.Equal(t, "synthetic-refresh", request.Form.Get("refresh_token"))
				if scenario == "changed during refresh" {
					require.NoError(t, db.Model(&channel).Update("key", "operator-during").Error)
				}
				if scenario == "request canceled after rotation" {
					cancel()
				}
				accountID := "account-a"
				if scenario == "different account" {
					accountID = "account-b"
				}
				accessToken := codexCredentialTestJWT(t, accountID)
				if scenario == "unknown refreshed account" {
					accessToken = "opaque-access-token"
				}
				payload := `{"access_token":"` + accessToken + `","refresh_token":"synthetic-new-refresh","expires_in":3600}`
				if strings.Contains(scenario, "invalid expiry") {
					payload = strings.Replace(payload, "3600", "9223372036854775807", 1)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: request, Body: io.NopCloser(strings.NewReader(payload))}, nil
			})
			var refreshed *CodexOAuthKey
			if scenario == "single invalid expiry" {
				refreshed, _, err = RefreshCodexChannelCredential(ctx, channel.Id, CodexCredentialRefreshOptions{ExpectedKey: &old})
			} else {
				refreshed, err = RefreshCodexChannelCredentialElement(ctx, channel.Id, old, "")
			}
			var persisted model.Channel
			require.NoError(t, db.First(&persisted, channel.Id).Error)
			switch scenario {
			case "rotate one account", "single formatted credential", "request canceled after rotation":
				require.NoError(t, err)
				require.NotNil(t, refreshed)
				assert.Equal(t, "account-a", refreshed.AccountID)
				assert.Equal(t, codexCredentialTestJWT(t, "account-a"), refreshed.AccessToken)
				if scenario == "single formatted credential" {
					require.Len(t, persisted.GetKeys(), 1)
				} else {
					require.Len(t, persisted.GetKeys(), 2)
					assert.Equal(t, other, persisted.GetKeys()[1])
					if scenario == "rotate one account" {
						_, otherErr := trustedCodexCredentialSubject(t.Context(), channel.Id, other)
						require.NoError(t, otherErr)
					}
				}
				assert.NotContains(t, persisted.Key, "synthetic-old")
				assert.Equal(t, 1, requests)
			case "invalid expiry", "single invalid expiry", "different account", "unknown refreshed account":
				assert.Error(t, err)
				assert.Nil(t, refreshed)
				assert.Equal(t, channel.Key, persisted.Key, "untrusted refreshed credentials must not replace the target")
				assert.Equal(t, 1, requests)
			case "changed during refresh":
				var persistenceErr *CodexCredentialPersistenceError
				require.ErrorAs(t, err, &persistenceErr)
				assert.Nil(t, refreshed)
				assert.Equal(t, "operator-during", persisted.Key)
				assert.Equal(t, 1, requests)
			default:
				require.Error(t, err)
				assert.Nil(t, refreshed)
				assert.Zero(t, requests)
				if scenario == "changed before refresh" {
					assert.Equal(t, "operator-before", persisted.Key)
				}
			}
		})
	}
}

func TestReplaceCodexChannelCredentialFromOAuthAllowsConfirmedSameSubject(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	previousDB, previousCache := model.DB, common.MemoryCacheEnabled
	model.DB, common.MemoryCacheEnabled = db, false
	t.Cleanup(func() { model.DB, common.MemoryCacheEnabled = previousDB, previousCache })

	channel := &model.Channel{Type: constant.ChannelTypeCodex, Key: `{"access_token":"old","refresh_token":"old-refresh"}`}
	require.NoError(t, db.Create(channel).Error)
	establishCodexCredentialSubjectEvidence(t, db, channel.Id, channel.Key, "account-a")
	replacement := &CodexOAuthKey{
		AccessToken: codexCredentialTestJWT(t, "account-a"), RefreshToken: "new-refresh", Type: "codex",
	}

	updated, err := ReplaceCodexChannelCredentialFromOAuth(t.Context(), channel.Id, channel.Key, replacement)

	require.NoError(t, err)
	require.True(t, updated)
	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Contains(t, stored.Key, "new-refresh")
	assert.Equal(t, "account-a", replacement.AccountID)
}

func establishCodexCredentialSubjectEvidence(t *testing.T, db *gorm.DB, channelID int, credential, accountID string) {
	t.Helper()
	secret := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("c", 32)))
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "active:credential-test:"+secret)
	require.NoError(t, db.AutoMigrate(
		&model.ChannelQuotaIdentityKeyRegistry{},
		&model.ChannelQuotaIdentityKeyVersion{},
		&model.ChannelQuotaIdentityAlias{},
		&model.ChannelQuotaSamplingTarget{},
		&model.ChannelQuotaSnapshot{},
	))
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	require.NoError(t, err)
	require.NoError(t, model.EnsureChannelQuotaIdentityKeyring(t.Context(), db, keyring))
	provider := "channel_type_" + strconv.Itoa(constant.ChannelTypeCodex)
	credentialIdentity, err := model.ResolveChannelQuotaIdentity(t.Context(), db, keyring, provider, common.ChannelQuotaIdentityKindCredential, []byte(credential))
	require.NoError(t, err)
	accountIdentity, err := model.ResolveChannelQuotaIdentity(t.Context(), db, keyring, provider, common.ChannelQuotaIdentityKindProviderAccount, []byte(accountID))
	require.NoError(t, err)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.ChannelQuotaSamplingTarget{
		ChannelID: channelID, SubjectRef: credentialIdentity.SubjectRef, IdentityQuality: credentialIdentity.Quality,
		ConfirmedSubjectRef: accountIdentity.SubjectRef, Active: true, LastResult: "sampled", CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.ChannelQuotaSnapshot{
		ChannelId: channelID, SubjectRef: accountIdentity.SubjectRef, IdentityQuality: accountIdentity.Quality,
		CodexObservationQualified: true, ObservedAt: now, MetricType: "codex_rate_limit", WindowType: "five_hour",
		WindowSeconds: 18000, ResetAt: now + 18000, Source: "codex_wham_usage_primary", Status: "success", Unit: "percent",
	}).Error)
}

func codexCredentialTestJWT(t *testing.T, accountID string) string {
	t.Helper()
	claims, err := common.Marshal(map[string]any{
		codexJWTClaimPath: map[string]any{"chatgpt_account_id": accountID},
	})
	require.NoError(t, err)
	return "fixture." + base64.RawURLEncoding.EncodeToString(claims) + ".signature"
}
