package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelQuotaSamplerIsolatesMultiKeyFailureAndSurvivesReorder(t *testing.T) {
	previousDB := model.DB
	previousInterval := common.RequestInterval
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.ChannelQuotaSamplingTarget{}))
	model.DB = db
	common.RequestInterval = 0
	setupChannelQuotaIdentityFixture(t, db)
	t.Cleanup(func() {
		model.DB = previousDB
		common.RequestInterval = previousInterval
	})

	var disabledRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") == "Bearer disabled-key" {
			disabledRequests.Add(1)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if request.Header.Get("Authorization") == "Bearer key-b" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		switch request.URL.Path {
		case "/v1/dashboard/billing/subscription":
			_, _ = w.Write([]byte(`{"object":"billing_subscription","has_payment_method":true,"hard_limit_usd":10}`))
		case "/v1/dashboard/billing/usage":
			_, _ = w.Write([]byte(`{"object":"list","total_usage":100}`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	baseURL := server.URL
	channel := &model.Channel{
		Type: constant.ChannelTypeCustom, Status: common.ChannelStatusEnabled,
		Name: "multi-quota", Key: "key-a\nkey-b\ndisabled-key", Balance: 77, BaseURL: &baseURL,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 3, MultiKeyStatusList: map[int]int{2: common.ChannelStatusManuallyDisabled}},
	}
	require.NoError(t, db.Create(channel).Error)

	summary, err := runChannelQuotaSnapshotSyncOnce(context.Background(), 2, nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Sampled)
	require.Equal(t, 1, summary.Failed)
	require.Zero(t, disabledRequests.Load())
	var first []model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Order("id").Find(&first).Error)
	require.Len(t, first, 2)
	require.NotEqual(t, first[0].SubjectRef, first[1].SubjectRef)
	for _, snapshot := range first {
		require.Equal(t, model.ChannelQuotaIdentityQualityCredentialScoped, snapshot.IdentityQuality)
	}
	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	require.Equal(t, 77.0, stored.Balance, "scheduled per-key queries must not overwrite the channel aggregate balance")

	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("key", "key-b\nkey-a").Error)
	summary, err = runChannelQuotaSnapshotSyncOnce(context.Background(), 2, nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Sampled)
	require.Equal(t, 1, summary.Failed)
	var all []model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Order("id").Find(&all).Error)
	require.Len(t, all, 4)
	firstSubjects := []string{first[0].SubjectRef, first[1].SubjectRef}
	secondSubjects := []string{all[2].SubjectRef, all[3].SubjectRef}
	sort.Strings(firstSubjects)
	sort.Strings(secondSubjects)
	require.Equal(t, firstSubjects, secondSubjects)
	var targets []model.ChannelQuotaSamplingTarget
	require.NoError(t, db.Where("channel_id = ? AND active = ?", channel.Id, true).Find(&targets).Error)
	require.Len(t, targets, 2)
	for _, target := range targets {
		require.NotContains(t, strings.Join([]string{target.SubjectRef, target.LastResult}, " "), "key-")
	}
}

func TestChannelQuotaSamplerMissingIdentityKeyDoesNotSendCredentials(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	setupChannelQuotaIdentityFixture(t, db)
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	require.NoError(t, db.Create(&model.Channel{Key: "synthetic-secret", Type: constant.ChannelTypeCustom, Status: common.ChannelStatusEnabled}).Error)
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	requests := 0
	client.Transport = quotaSamplingRoundTripper(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, fmt.Errorf("unexpected network request")
	})
	summary, err := runChannelQuotaSnapshotSyncOnce(context.Background(), 1, nil)
	require.Error(t, err)
	require.Zero(t, requests)
	require.False(t, summary.SourceComplete)
	var marker model.ChannelQuotaSnapshot
	require.NoError(t, db.First(&marker).Error)
	require.Equal(t, "identity_unavailable", marker.ErrorCode)
	require.NotContains(t, marker.ErrorMessage, "synthetic-secret")
}

func TestChannelQuotaSamplerEventuallyRequestsEveryKeyBeyondExpansionWindow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	previousDB, previousInterval := model.DB, common.RequestInterval
	model.DB, common.RequestInterval = db, 0
	t.Cleanup(func() { model.DB, common.RequestInterval = previousDB, previousInterval })
	setupChannelQuotaIdentityFixture(t, db)
	keys := make([]string, 40)
	for index := range keys {
		keys[index] = fmt.Sprintf("synthetic-%02d", index)
	}
	requested := make(map[string]bool)
	var requestedMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/dashboard/billing/subscription":
			requestedMu.Lock()
			requested[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")] = true
			requestedMu.Unlock()
			_, _ = w.Write([]byte(`{"has_payment_method":true,"hard_limit_usd":10}`))
		case "/v1/dashboard/billing/usage":
			_, _ = w.Write([]byte(`{"total_usage":100}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base := server.URL
	channel := model.Channel{Type: constant.ChannelTypeCustom, Status: common.ChannelStatusEnabled, Key: strings.Join(keys, "\n"), BaseURL: &base, ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: len(keys)}}
	require.NoError(t, db.Create(&channel).Error)
	// Forty distinct accounts, one attempt per pass; allow one five-window
	// cycle for a remaining unseen key to reenter the bounded expansion set.
	for pass := 0; pass < len(keys)+5; pass++ {
		requestedMu.Lock()
		complete := len(requested) == len(keys)
		requestedMu.Unlock()
		if complete {
			break
		}
		summary, runErr := runChannelQuotaSnapshotSyncOnce(context.Background(), 1, nil)
		require.NoError(t, runErr)
		require.Equal(t, 1, summary.Sampled)
		require.False(t, summary.SourceComplete)
	}
	requestedMu.Lock()
	defer requestedMu.Unlock()
	require.Len(t, requested, len(keys), "keys beyond the first expansion window must not starve")
	for _, key := range keys {
		require.True(t, requested[key], key)
	}
}

func TestChannelQuotaSamplerFoldsDuplicateConfirmedProviderAccount(t *testing.T) {
	previousDB := model.DB
	previousInterval := common.RequestInterval
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.ChannelQuotaSamplingTarget{}))
	model.DB = db
	common.RequestInterval = 0
	setupChannelQuotaIdentityFixture(t, db)
	t.Cleanup(func() {
		model.DB = previousDB
		common.RequestInterval = previousInterval
	})

	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	client.Transport = quotaSamplingRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"code":20000,"message":"ok","data":{"id":"confirmed-account","totalBalance":"10"}}`,
			)),
			Request: request,
		}, nil
	})
	t.Cleanup(func() { client.Transport = previousTransport })

	channel := &model.Channel{
		Type: constant.ChannelTypeSiliconFlow, Status: common.ChannelStatusEnabled,
		Name: "duplicate-account", Key: "key-a\nkey-b",
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2},
	}
	require.NoError(t, db.Create(channel).Error)
	summary, err := runChannelQuotaSnapshotSyncOnce(context.Background(), 2, nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Sampled)
	require.Zero(t, summary.Failed)
	require.Equal(t, 1, summary.DuplicateAccountObservations)
	require.Zero(t, summary.DeferredKeys)
	require.True(t, summary.SourceComplete)

	var snapshots []model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Find(&snapshots).Error)
	require.Len(t, snapshots, 1)
	require.Equal(t, model.ChannelQuotaIdentityQualityProviderConfirmed, snapshots[0].IdentityQuality)
	var targets []model.ChannelQuotaSamplingTarget
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Order("last_result").Find(&targets).Error)
	require.Len(t, targets, 2)
	results := []string{targets[0].LastResult, targets[1].LastResult}
	sort.Strings(results)
	require.Equal(t, []string{"duplicate", "sampled"}, results)
	require.NotEmpty(t, targets[0].ConfirmedSubjectRef)
	require.Equal(t, targets[0].ConfirmedSubjectRef, targets[1].ConfirmedSubjectRef)
	channel.Key = "key-a\nkey-b"
	candidates, totalTargets, err := buildChannelQuotaSamplingCandidates(context.Background(), []*model.Channel{channel})
	require.NoError(t, err)
	require.Equal(t, 1, totalTargets)
	require.Len(t, candidates, 1, "confirmed duplicate credentials should consume one future request budget")
}

func TestChannelQuotaSamplerReportsProviderBudgetDeferredKeys(t *testing.T) {
	previousDB := model.DB
	previousInterval := common.RequestInterval
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.ChannelQuotaSamplingTarget{}))
	model.DB = db
	common.RequestInterval = 0
	setupChannelQuotaIdentityFixture(t, db)
	t.Cleanup(func() { model.DB = previousDB; common.RequestInterval = previousInterval })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/dashboard/billing/subscription":
			_, _ = w.Write([]byte(`{"has_payment_method":true,"hard_limit_usd":10}`))
		case "/v1/dashboard/billing/usage":
			_, _ = w.Write([]byte(`{"total_usage":100}`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	baseURL := server.URL
	channel := &model.Channel{Type: constant.ChannelTypeCustom, Status: common.ChannelStatusEnabled, Key: "key-a\nkey-b", BaseURL: &baseURL, ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2}}
	require.NoError(t, db.Create(channel).Error)
	summary, err := runChannelQuotaSnapshotSyncOnce(context.Background(), 1, nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Sampled)
	require.Equal(t, 1, summary.DeferredKeys)
	require.True(t, summary.BudgetExhausted)
	require.False(t, summary.SourceComplete)
}

func TestChannelQuotaSamplerReportsPerChannelBudgetExhaustion(t *testing.T) {
	previousDB := model.DB
	previousInterval := common.RequestInterval
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.ChannelQuotaSamplingTarget{}))
	model.DB = db
	common.RequestInterval = 0
	setupChannelQuotaIdentityFixture(t, db)
	t.Cleanup(func() { model.DB = previousDB; common.RequestInterval = previousInterval })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/dashboard/billing/subscription":
			_, _ = w.Write([]byte(`{"has_payment_method":true,"hard_limit_usd":10}`))
		case "/v1/dashboard/billing/usage":
			_, _ = w.Write([]byte(`{"total_usage":100}`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	baseURL := server.URL
	channel := &model.Channel{
		Type: constant.ChannelTypeCustom, Status: common.ChannelStatusEnabled,
		Key: "key-a\nkey-b\nkey-c\nkey-d\nkey-e\nkey-f", BaseURL: &baseURL,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 6},
	}
	require.NoError(t, db.Create(channel).Error)

	summary, err := runChannelQuotaSnapshotSyncOnce(context.Background(), 100, nil)
	require.NoError(t, err)
	require.Equal(t, 4, summary.Sampled)
	require.Equal(t, 2, summary.DeferredKeys)
	require.True(t, summary.BudgetExhausted)
	require.False(t, summary.SourceComplete)
}

func TestChannelQuotaSamplingCandidatesCapIdentityWorkAndReportExcessKeys(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSamplingTarget{}))
	model.DB = db
	setupChannelQuotaIdentityFixture(t, db)
	t.Cleanup(func() { model.DB = previousDB })
	keys := make([]string, 40)
	for index := range keys {
		keys[index] = "key-" + strconv.Itoa(index)
	}
	channel := &model.Channel{
		Id: 77, Type: constant.ChannelTypeCustom, Status: common.ChannelStatusEnabled,
		Key:         strings.Join(keys, "\n"),
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: len(keys)},
	}
	require.NoError(t, db.Create(channel).Error)
	candidates, total, err := buildChannelQuotaSamplingCandidates(context.Background(), []*model.Channel{channel})
	require.NoError(t, err)
	require.Equal(t, 40, total)
	require.Len(t, candidates, channelQuotaSamplingPerChannelDefault)
	var activeTargets int64
	require.NoError(t, db.Model(&model.ChannelQuotaSamplingTarget{}).Where("channel_id = ? AND active = ?", channel.Id, true).Count(&activeTargets).Error)
	require.Equal(t, int64(channelQuotaSamplingPerChannelHardMax), activeTargets)
	for _, candidate := range candidates {
		require.NoError(t, model.MarkChannelQuotaSamplingTargetAttempt(
			context.Background(), db, channel.Id, candidate.identity.SubjectRef, "sampled", "",
		))
	}
	next, total, err := buildChannelQuotaSamplingCandidates(context.Background(), []*model.Channel{channel})
	require.NoError(t, err)
	require.Equal(t, 40, total)
	require.Len(t, next, channelQuotaSamplingPerChannelDefault)
	selectedBefore := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		selectedBefore[candidate.identity.SubjectRef] = struct{}{}
	}
	newTargets := 0
	for _, candidate := range next {
		if _, existed := selectedBefore[candidate.identity.SubjectRef]; !existed {
			newTargets++
		}
	}
	require.Equal(t, channelQuotaSamplingPerChannelDefault, newTargets, "never-attempted targets must rotate into the next bounded pass")
}

func TestChannelQuotaSamplingCandidatesDoNotCountPreservedStaleTargetsTwice(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSamplingTarget{}))
	model.DB = db
	setupChannelQuotaIdentityFixture(t, db)
	t.Cleanup(func() { model.DB = previousDB })
	keys := make([]string, 40)
	for index := range keys {
		keys[index] = fmt.Sprintf("key-%02d", index)
	}
	channel := &model.Channel{
		Id: 78, Type: constant.ChannelTypeCustom, Status: common.ChannelStatusEnabled,
		Key: strings.Join(keys, "\n"), ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: len(keys)},
	}
	require.NoError(t, db.Create(channel).Error)
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	require.NoError(t, err)
	oldFirstSubject := ""
	for _, credential := range keys {
		identity, resolveErr := model.ResolveChannelQuotaIdentity(
			context.Background(), db, keyring, "channel_type_"+strconv.Itoa(channel.Type),
			common.ChannelQuotaIdentityKindCredential, []byte(credential),
		)
		require.NoError(t, resolveErr)
		if credential == "key-00" {
			oldFirstSubject = identity.SubjectRef
		}
		require.NoError(t, model.EnsureChannelQuotaSamplingTarget(context.Background(), db, channel.Id, model.ChannelQuotaSamplingIdentity{
			SubjectRef: identity.SubjectRef, IdentityQuality: identity.Quality,
		}))
	}

	candidates, total, err := buildChannelQuotaSamplingCandidates(context.Background(), []*model.Channel{channel})
	require.NoError(t, err)
	require.Equal(t, 40, total)
	require.Len(t, candidates, channelQuotaSamplingPerChannelDefault)

	keys[0] = "key-00-replacement"
	channel.Key = strings.Join(keys, "\n")
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("key", channel.Key).Error)
	candidates, total, err = buildChannelQuotaSamplingCandidates(context.Background(), []*model.Channel{channel})
	require.NoError(t, err)
	require.Equal(t, 40, total)
	require.Len(t, candidates, channelQuotaSamplingPerChannelDefault)
	for _, candidate := range candidates {
		require.NotEqual(t, "key-00", candidate.credential)
	}
	var oldTarget model.ChannelQuotaSamplingTarget
	require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", channel.Id, oldFirstSubject).First(&oldTarget).Error)
	require.False(t, oldTarget.Active)
	var activeTargets int64
	require.NoError(t, db.Model(&model.ChannelQuotaSamplingTarget{}).Where("channel_id = ? AND active = ?", channel.Id, true).Count(&activeTargets).Error)
	require.Equal(t, int64(channelQuotaSamplingPerChannelHardMax), activeTargets)
}

func TestCodexDuplicateFoldLetsLaterUsableCredentialWin(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	model.DB = db
	setupChannelQuotaIdentityFixture(t, db)
	t.Cleanup(func() { model.DB = previousDB })

	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	client.Transport = quotaSamplingRoundTripper(func(request *http.Request) (*http.Response, error) {
		body := `{}`
		if request.Header.Get("Authorization") == "Bearer usable-token" {
			body = `{"rate_limit":{"primary_window":{"used_percent":10,"limit_window_seconds":18000}}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	t.Cleanup(func() { client.Transport = previousTransport })

	baseURL := "https://codex-quota.invalid"
	channel := &model.Channel{Id: 88, Type: constant.ChannelTypeCodex, BaseURL: &baseURL}
	require.NoError(t, db.Create(channel).Error)
	credentials := []string{
		`{"access_token":"invalid-payload","account_id":"same-account"}`,
		`{"access_token":"usable-token","account_id":"same-account"}`,
	}
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	require.NoError(t, err)
	identities := make([]model.ChannelQuotaResolvedIdentity, len(credentials))
	for index, credential := range credentials {
		identities[index], err = model.ResolveChannelQuotaIdentity(
			context.Background(), db, keyring, "channel_type_"+strconv.Itoa(channel.Type),
			common.ChannelQuotaIdentityKindCredential, []byte(credential),
		)
		require.NoError(t, err)
	}
	confirmed := make(map[string]string)
	accept := func(owner string) func(string) bool {
		return func(subject string) bool {
			if _, exists := confirmed[subject]; exists {
				return false
			}
			confirmed[subject] = owner
			return true
		}
	}
	release := func(owner string) func(string) {
		return func(subject string) {
			if confirmed[subject] == owner {
				delete(confirmed, subject)
			}
		}
	}
	err = sampleCodexCredentialUsage(context.Background(), channel, credentials[0], identities[0], accept(identities[0].SubjectRef), release(identities[0].SubjectRef), nil)
	require.ErrorIs(t, err, errChannelQuotaUnsupported)
	err = sampleCodexCredentialUsage(context.Background(), channel, credentials[1], identities[1], accept(identities[1].SubjectRef), release(identities[1].SubjectRef), nil)
	require.NoError(t, err)
	var successes int64
	require.NoError(t, db.Model(&model.ChannelQuotaSnapshot{}).Where("channel_id = ? AND status = ?", channel.Id, "success").Count(&successes).Error)
	require.Equal(t, int64(1), successes)
}

func TestCodexInvalid2xxDoesNotConfirmSamplingTarget(t *testing.T) {
	previousDB := model.DB
	previousInterval := common.RequestInterval
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.ChannelQuotaSamplingTarget{}))
	model.DB = db
	common.RequestInterval = 0
	setupChannelQuotaIdentityFixture(t, db)
	t.Cleanup(func() { model.DB = previousDB; common.RequestInterval = previousInterval })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"login required"}`))
	}))
	defer server.Close()
	baseURL := server.URL
	channel := &model.Channel{
		Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, BaseURL: &baseURL,
		Key: `{"access_token":"invalid-payload","account_id":"unconfirmed-account","type":"codex"}`,
	}
	require.NoError(t, db.Create(channel).Error)

	summary, err := runChannelQuotaSnapshotSyncOnce(context.Background(), 1, nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Failed)
	require.Equal(t, 1, summary.Unsupported)
	var target model.ChannelQuotaSamplingTarget
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&target).Error)
	require.Empty(t, target.ConfirmedSubjectRef)
	require.Equal(t, "unsupported", target.LastResult)
	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&snapshot).Error)
	require.Equal(t, model.ChannelQuotaIdentityQualityCredentialScoped, snapshot.IdentityQuality)
	require.Equal(t, "unsupported", snapshot.Status)
}
