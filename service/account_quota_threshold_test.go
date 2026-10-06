package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func recordThresholdAccount(t *testing.T, db *gorm.DB, channelID int, account, sample string, used float64, at int64) model.ChannelQuotaSnapshot {
	t.Helper()
	total := float64(100)
	row := model.ChannelQuotaSnapshot{ChannelId: channelID, AccountRef: model.ChannelQuotaAccountRef("codex", account), ObservedAt: at, SampleID: sample, Available: 100 - used, Used: &used, Total: &total, MetricType: "codex_rate_limit", WindowType: "five_hour", WindowSeconds: 18000, ResetAt: at + 18000, Source: "codex_wham_usage_primary", Status: "success", Unit: "percent", CodexThresholdQualified: true}
	require.NoError(t, db.Create(&row).Error)
	return row
}

func TestAccountThresholdRoutingSkipsLowAndUnsupportedChannels(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	createCodexQuotaRoutingChannel(t, db, 7101, 100, `{"access_token":"fixture-a","account_id":"threshold-a"}`)
	createCodexQuotaRoutingChannel(t, db, 7102, 50, `{"access_token":"fixture-b","account_id":"threshold-b"}`)
	createCodexQuotaRoutingChannel(t, db, 7103, 200, "synthetic-direct-api")
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 7103).Update("type", constant.ChannelTypeOpenAI).Error)
	require.NoError(t, model.InitChannelCache())
	now, err := model.ReadDatabaseUnixTime(context.Background(), db)
	require.NoError(t, err)
	recordThresholdAccount(t, db, 7101, "threshold-a", "low-a", 90, now)
	recordThresholdAccount(t, db, 7102, "threshold-b", "healthy-b", 10, now)
	ctx := common.WithAccountQuotaThreshold(context.Background(), common.AccountQuotaThreshold{MinimumRemainingBPS: 2000, MaxAgeSeconds: 300})
	selected, err := getRandomQuotaSatisfiedChannel(ctx, "default", "gpt-5-codex", 0, "/v1/responses")
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 7102, selected.Id)
	channel, err := model.CacheGetChannel(7101)
	require.NoError(t, err)
	_, eligible, err := CodexQuotaEligibleKeys(context.Background(), channel)
	require.NoError(t, err)
	assert.True(t, eligible, "disabled threshold preserves the old non-exhausted routing contract")
	multi := *channel
	multi.Key += "\n" + `{"access_token":"fixture-b","account_id":"threshold-b"}`
	multi.ChannelInfo = model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModePolling}
	excluded, eligible, err := CodexQuotaEligibleKeys(ctx, &multi)
	require.NoError(t, err)
	require.True(t, eligible)
	assert.True(t, excluded[0])
	assert.False(t, excluded[1])
	assert.Zero(t, multi.ChannelInfo.MultiKeyPollingIndex, "eligibility does not rotate credentials")
}

func TestAccountThresholdDispatchChecksExactIdentityAndPolicyRevision(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.TokenBudget{}, &model.TokenBudgetReservation{}, &model.TokenBudgetPolicyChange{}, &model.ChannelQuotaSnapshot{}))
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "threshold-root", AffCode: "threshold-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.User{Id: 2, Username: "threshold-owner", AffCode: "threshold-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 11, UserId: 2, Key: "thresholdfixture", Status: common.TokenStatusEnabled, ExpiredTime: -1}).Error)
	policy, err := model.ConfigureTokenBudget(context.Background(), db, 1, model.TokenBudgetPolicyInput{ID: strings.Repeat("a", 64), TokenID: 11, AccountThreshold: &model.AccountQuotaThresholdPolicyInput{Enabled: true, MinimumRemainingBPS: 2000, MaxAgeSeconds: 300}})
	require.NoError(t, err)
	now, err := model.ReadDatabaseUnixTime(context.Background(), db)
	require.NoError(t, err)
	recordThresholdAccount(t, db, 7, "threshold-a", "dispatch-healthy", 10, now)
	key := `{"access_token":"fixture-a","account_id":"threshold-a"}`
	info := &relaycommon.RelayInfo{UserId: 2, TokenId: 11, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 7, ChannelType: constant.ChannelTypeCodex, ApiKey: key}}
	request, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer fixture-a")
	request.Header.Set("chatgpt-account-id", "threshold-a")
	client := &http.Client{Transport: &http.Transport{}}
	ctx := common.WithAccountQuotaThreshold(context.Background(), common.AccountQuotaThreshold{TokenID: 11, Revision: policy.Revision, MinimumRemainingBPS: 2000, MaxAgeSeconds: 300})
	require.NoError(t, ValidateAccountQuotaThresholdDispatch(ctx, client, request, info))
	request.Header.Set("chatgpt-account-id", "another-account")
	require.Error(t, ValidateAccountQuotaThresholdDispatch(ctx, client, request, info))
	request.Header.Set("chatgpt-account-id", "threshold-a")
	_, err = model.ConfigureTokenBudget(context.Background(), db, 1, model.TokenBudgetPolicyInput{ID: strings.Repeat("b", 64), TokenID: 11, ExpectedRevision: policy.Revision, AccountThreshold: &model.AccountQuotaThresholdPolicyInput{Enabled: true, MinimumRemainingBPS: 3000, MaxAgeSeconds: 300}})
	require.NoError(t, err)
	require.Error(t, ValidateAccountQuotaThresholdDispatch(ctx, client, request, info), "a changed configuration cannot use the stale admission policy")
}

func TestAccountThresholdCredentialFailureDoesNotPoisonHealthyAlias(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	secret := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("t", 32)))
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "active:v1:"+secret)
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}, &model.ChannelQuotaIdentityKeyRegistry{}, &model.ChannelQuotaIdentityKeyVersion{}, &model.ChannelQuotaIdentityAlias{}))
	ring, err := common.LoadChannelQuotaIdentityKeyring()
	require.NoError(t, err)
	require.NoError(t, model.EnsureChannelQuotaIdentityKeyring(context.Background(), db, ring))
	keyA := `{"access_token":"fixture-a","account_id":"threshold-shared"}`
	keyB := `{"access_token":"fixture-b","account_id":"threshold-shared"}`
	account, err := model.ResolveChannelQuotaIdentity(context.Background(), db, ring, "channel_type_57", common.ChannelQuotaIdentityKindProviderAccount, []byte("threshold-shared"))
	require.NoError(t, err)
	failed, err := model.ResolveChannelQuotaIdentity(context.Background(), db, ring, "channel_type_57", common.ChannelQuotaIdentityKindCredential, []byte(keyA))
	require.NoError(t, err)
	now, err := model.ReadDatabaseUnixTime(context.Background(), db)
	require.NoError(t, err)
	row := recordThresholdAccount(t, db, 7, "temporary", "versioned-account", 10, now)
	require.NoError(t, db.Model(&model.ChannelQuotaSnapshot{}).Where("id = ?", row.Id).Updates(map[string]any{"account_ref": "", "subject_ref": account.SubjectRef, "identity_quality": account.Quality}).Error)
	require.NoError(t, db.Create(&model.ChannelQuotaSnapshot{ChannelId: 7, SubjectRef: failed.SubjectRef, IdentityQuality: failed.Quality, ObservedAt: now, SampleID: "credential-failure", MetricType: "codex_rate_limit", Unit: "percent", Source: "codex_wham_usage", Status: "error"}).Error)
	baseURL := "https://chatgpt.com"
	channel := &model.Channel{BaseURL: &baseURL, Id: 7, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Key: fmt.Sprintf("%s\n%s", keyA, keyB), ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2}}
	ctx := common.WithAccountQuotaThreshold(context.Background(), common.AccountQuotaThreshold{MinimumRemainingBPS: 2000, MaxAgeSeconds: 300})
	excluded, eligible, err := CodexQuotaEligibleKeys(ctx, channel)
	require.NoError(t, err)
	assert.True(t, eligible)
	assert.True(t, excluded[0])
	assert.False(t, excluded[1])
}
