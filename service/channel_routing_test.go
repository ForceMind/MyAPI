package service

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func routingSuccessSnapshot(channelID int, available float64, observedAt int64, unit, source, window string, windowSeconds, resetAt int64) model.ChannelQuotaSnapshot {
	used := 100 - available
	total := float64(100)
	return model.ChannelQuotaSnapshot{
		ChannelId: channelID, Available: available, Used: &used, Total: &total,
		ObservedAt: observedAt, Unit: unit, MetricType: "remaining", WindowType: window,
		WindowSeconds: windowSeconds, ResetAt: resetAt, Source: source, PlanType: "plan",
		Status: "success",
	}
}

func routingCandidates(count int) []model.ChannelRoutingCandidate {
	result := make([]model.ChannelRoutingCandidate, 0, count)
	for id := 1; id <= count; id++ {
		result = append(result, model.ChannelRoutingCandidate{
			ID: id, Name: fmt.Sprintf("channel-%d", id), Status: common.ChannelStatusEnabled,
			Priority: 10, Weight: 1,
		})
	}
	return result
}

func previewShares(preview ChannelRoutingPreview) map[int]float64 {
	shares := make(map[int]float64, len(preview.Candidates))
	for _, candidate := range preview.Candidates {
		shares[candidate.ID] = candidate.Share
	}
	return shares
}

func previewReasons(preview ChannelRoutingPreview) map[int]string {
	reasons := make(map[int]string, len(preview.Candidates))
	for _, candidate := range preview.Candidates {
		reasons[candidate.ID] = candidate.Reason
	}
	return reasons
}

func TestChannelRoutingWorkloadUsesOnlySupportedPaths(t *testing.T) {
	assert.Equal(t, ChannelRoutingWorkloadChat, ChannelRoutingWorkload("/v1/chat/completions"))
	assert.Equal(t, ChannelRoutingWorkloadChat, ChannelRoutingWorkload("/pg/chat/completions"))
	assert.Equal(t, ChannelRoutingWorkloadWork, ChannelRoutingWorkload("/v1/responses"))
	assert.Equal(t, ChannelRoutingWorkloadWork, ChannelRoutingWorkload("/v1/responses/compact"))
	assert.Equal(t, ChannelRoutingWorkloadBalanced, ChannelRoutingWorkload("/v1/messages"))
	assert.Equal(t, ChannelRoutingWorkloadBalanced, ChannelRoutingWorkload("/v1/chat/completions/extra"))
}

func TestDeriveRoutingQuotaProtectsFreshnessIdentityAndResetBoundaries(t *testing.T) {
	now := int64(10_000)
	primary := routingSuccessSnapshot(1, 70, now-10, "percent", "codex_usage_primary", "five_hour", 18_000, now+1_000)
	secondary := routingSuccessSnapshot(1, 20, now-10, "percent", "codex_usage_secondary", "weekly", 604_800, now+2_000)
	assessment := deriveRoutingQuota([]model.ChannelQuotaSnapshot{primary, secondary}, false, now, 60)
	require.NotNil(t, assessment.view.Available)
	assert.Equal(t, "fresh", assessment.view.State)
	assert.Equal(t, float64(20), *assessment.view.Available, "the tightest active window controls routing")
	assert.NotEmpty(t, assessment.comparisonKey)

	expiredReset := secondary
	expiredReset.Available = 0
	expiredReset.ResetAt = now
	assert.Equal(t, "unknown", deriveRoutingQuota([]model.ChannelQuotaSnapshot{expiredReset}, false, now, 60).view.State)

	failure := model.ChannelQuotaSnapshot{Id: 99, ChannelId: 1, ObservedAt: now - 5, Status: "error", Source: "codex_usage", Unit: "percent"}
	assert.Equal(t, "unknown", deriveRoutingQuota([]model.ChannelQuotaSnapshot{primary, failure}, false, now, 60).view.State)
	newerPrimary := primary
	newerPrimary.ObservedAt = now - 1
	olderSecondary := secondary
	olderSecondary.ObservedAt = now - 20
	betweenWindowsFailure := failure
	betweenWindowsFailure.ObservedAt = now - 10
	assert.Equal(t, "unknown", deriveRoutingQuota([]model.ChannelQuotaSnapshot{newerPrimary, olderSecondary, betweenWindowsFailure}, false, now, 60).view.State,
		"a newer success in one window must not revive an older window after the latest fetch error")

	future := primary
	future.ObservedAt = now + 1
	assert.Equal(t, "unknown", deriveRoutingQuota([]model.ChannelQuotaSnapshot{future, primary}, false, now, 60).view.State)

	stale := primary
	stale.ObservedAt = now - 61
	assert.Equal(t, "stale", deriveRoutingQuota([]model.ChannelQuotaSnapshot{stale}, false, now, 60).view.State)
	assert.Equal(t, "unknown", deriveRoutingQuota([]model.ChannelQuotaSnapshot{primary}, true, now, 60).view.State)

	usd := primary
	usd.Unit = "usd"
	assert.NotEqual(t,
		deriveRoutingQuota([]model.ChannelQuotaSnapshot{primary}, false, now, 60).comparisonKey,
		deriveRoutingQuota([]model.ChannelQuotaSnapshot{usd}, false, now, 60).comparisonKey,
		"different units must never be sorted together",
	)
}

func TestBuildRoutingPlanUsesLowAndHighQuotaHalvesWithinHighestPriority(t *testing.T) {
	now := int64(20_000)
	candidates := routingCandidates(5)
	candidates[4].Priority = 9
	rows := make(map[int][]model.ChannelQuotaSnapshot)
	for id, available := range []float64{10, 20, 30, 40, 1} {
		rows[id+1] = []model.ChannelQuotaSnapshot{
			routingSuccessSnapshot(id+1, available, now-1, "percent", "usage", "daily", 86_400, now+1_000),
		}
	}

	chat := buildRoutingPlan(candidates, rows, ChannelRoutingWorkloadChat, true, nil, now, 60)
	assert.Equal(t, "chat_lower_remaining_quota", chat.preview.Reason)
	assert.Equal(t, map[int]float64{1: 0.5, 2: 0.5, 3: 0, 4: 0, 5: 0}, previewShares(chat.preview))
	assert.Equal(t, "lower_priority", previewReasons(chat.preview)[5])

	work := buildRoutingPlan(candidates, rows, ChannelRoutingWorkloadWork, true, nil, now, 60)
	assert.Equal(t, "work_higher_remaining_quota", work.preview.Reason)
	assert.Equal(t, map[int]float64{1: 0, 2: 0, 3: 0.5, 4: 0.5, 5: 0}, previewShares(work.preview))
}

func TestBuildRoutingPlanPreservesWeightAndFallbackContracts(t *testing.T) {
	now := int64(30_000)
	candidates := routingCandidates(2)
	candidates[0].Weight = 0
	candidates[1].Weight = 10
	rows := map[int][]model.ChannelQuotaSnapshot{
		1: {routingSuccessSnapshot(1, 1, now-1, "percent", "usage", "daily", 86_400, now+100)},
		2: {routingSuccessSnapshot(2, 99, now-1, "percent", "usage", "daily", 86_400, now+100)},
	}
	chat := buildRoutingPlan(candidates, rows, ChannelRoutingWorkloadChat, true, nil, now, 60)
	assert.Equal(t, float64(0), previewShares(chat.preview)[1])
	assert.Equal(t, float64(1), previewShares(chat.preview)[2])
	assert.Equal(t, "zero_weight", previewReasons(chat.preview)[1])

	candidates[0].Weight, candidates[1].Weight = 0, 0
	allZero := buildRoutingPlan(candidates, rows, ChannelRoutingWorkloadBalanced, true, nil, now, 60)
	assert.Equal(t, map[int]float64{1: 0.5, 2: 0.5}, previewShares(allZero.preview))

	rows[1][0].Available = 0
	disabled := buildRoutingPlan(candidates, rows, ChannelRoutingWorkloadChat, false, nil, now, 60)
	assert.Equal(t, "smart_routing_disabled", disabled.preview.Reason)
	assert.Equal(t, map[int]float64{1: 0.5, 2: 0.5}, previewShares(disabled.preview), "disabled preview must mirror legacy selection")

	candidates[0].Weight, candidates[1].Weight = 1, 3
	rows[1][0].Available = 10
	rows[2][0].Unit = "usd"
	incompatible := buildRoutingPlan(candidates, rows, ChannelRoutingWorkloadWork, true, nil, now, 60)
	assert.Equal(t, "quota_insufficient_comparable_data", incompatible.preview.Reason)
	assert.Equal(t, map[int]float64{1: 0.25, 2: 0.75}, previewShares(incompatible.preview))
}

func newRoutingSessionContext(path, body string, userID, tokenID int) *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", path, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyUserId, userID)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, tokenID)
	return ctx
}

func TestChannelRoutingSessionHashIsAuthenticatedAndConversationScoped(t *testing.T) {
	first := newRoutingSessionContext("/v1/responses", `{"conversation_id":"conversation-secret"}`, 7, 11)
	firstHash := channelRoutingSessionHash(first)
	require.Len(t, firstHash, 64)
	assert.NotContains(t, firstHash, "conversation-secret")

	compact := newRoutingSessionContext("/v1/responses/compact", `{"conversation_id":"conversation-secret"}`, 7, 11)
	assert.Equal(t, firstHash, channelRoutingSessionHash(compact), "responses and compact must share the conversation binding")
	assert.NotEqual(t, firstHash, channelRoutingSessionHash(newRoutingSessionContext("/v1/responses", `{"conversation_id":"conversation-secret"}`, 8, 11)))
	assert.NotEqual(t, firstHash, channelRoutingSessionHash(newRoutingSessionContext("/v1/responses", `{"conversation_id":"conversation-secret"}`, 7, 12)))
	assert.Empty(t, channelRoutingSessionHash(newRoutingSessionContext("/v1/responses", `{"prompt_cache_key":"shared-cache-bucket"}`, 7, 11)))
	assert.Empty(t, channelRoutingSessionHash(newRoutingSessionContext("/v1/responses", `{}`, 7, 11)))
	assert.Empty(t, channelRoutingSessionHash(newRoutingSessionContext("/v1/chat/completions", `{"conversation_id":"conversation-secret"}`, 7, 0)))

	playground := newRoutingSessionContext("/pg/chat/completions", `{"conversation_id":"conversation-secret"}`, 7, 0)
	playgroundHash := channelRoutingSessionHash(playground)
	require.Len(t, playgroundHash, 64)
	assert.Equal(t, playgroundHash, channelRoutingSessionHash(newRoutingSessionContext("/pg/chat/completions", `{"conversation_id":"conversation-secret"}`, 7, 0)))
	assert.NotEqual(t, playgroundHash, channelRoutingSessionHash(newRoutingSessionContext("/pg/chat/completions", `{"conversation_id":"conversation-secret"}`, 8, 0)))
	assert.NotEqual(t, firstHash, playgroundHash)

	header := newRoutingSessionContext("/v1/responses", `{"conversation_id":"body"}`, 7, 11)
	header.Request.Header.Set("X-Conversation-Id", "lower-priority")
	header.Request.Header.Set("Session_id", "highest-priority")
	assert.Equal(t, "highest-priority", extractChannelRoutingSessionID(header))
}

func setRoutingPolicyForTest(t *testing.T, enabled, sticky bool) {
	t.Helper()
	previous := operation_setting.GetChannelRoutingPolicy()
	apply := func(policy operation_setting.ChannelRoutingPolicy) {
		encoded, err := operation_setting.MarshalChannelRoutingPolicy(policy)
		require.NoError(t, err)
		require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("routing_policy_setting"), map[string]string{"policy": encoded}))
	}
	apply(operation_setting.ChannelRoutingPolicy{Enabled: enabled, StickyEnabled: sticky, SessionTTLSeconds: 3600, QuotaMaxAgeSeconds: 60})
	t.Cleanup(func() { apply(previous) })
}

func setupRoutingSelectorDB(t *testing.T, migrateQuota bool) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousMemory := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open("file:routing-service-"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	if migrateQuota {
		require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	}
	model.DB = db
	common.MemoryCacheEnabled = false
	routingQuotaCache.Lock()
	routingQuotaCache.entries = make(map[int]routingQuotaCacheEntry)
	routingQuotaCache.loading = make(map[int]chan struct{})
	routingQuotaCache.Unlock()
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemory
		sqlDB, sqlErr := db.DB()
		require.NoError(t, sqlErr)
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func addRoutingSelectorChannel(t *testing.T, db *gorm.DB, name, group string, priority int64, weight uint) model.Channel {
	t.Helper()
	channel := model.Channel{
		Type: 1, Key: "test-key", Status: common.ChannelStatusEnabled, Name: name,
		Models: "routing-model", Group: group, Priority: &priority, Weight: &weight,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	return channel
}

func TestSmartRoutingAutoGroupRetryHonorsTokenBoundary(t *testing.T) {
	configureRequestAutoGroupsTest(t)
	setRoutingPolicyForTest(t, true, false)
	db := setupRoutingSelectorDB(t, true)
	first := addRoutingSelectorChannel(t, db, "vip", "vip", 10, 1)
	second := addRoutingSelectorChannel(t, db, "default", "default", 10, 1)

	newParam := func(crossGroup bool) *RetryParam {
		ctx := newRoutingSessionContext("/v1/chat/completions", `{}`, 1, 1)
		common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
		common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, crossGroup)
		return &RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: "routing-model", RequestPath: "/v1/chat/completions", Retry: common.GetPointer(0)}
	}

	noCross := newParam(false)
	selected, group, err := SelectSmartChannelRouting(noCross)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, first.Id, selected.Id)
	assert.Equal(t, "vip", group)
	noCross.ExcludeChannel(first.Id)
	selected, _, err = SelectSmartChannelRouting(noCross)
	require.NoError(t, err)
	assert.Nil(t, selected)

	cross := newParam(true)
	selected, group, err = SelectSmartChannelRouting(cross)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, first.Id, selected.Id)
	cross.ExcludeChannel(first.Id)
	selected, group, err = SelectSmartChannelRouting(cross)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, second.Id, selected.Id)
	assert.Equal(t, "default", group)
	assert.Equal(t, "default", common.GetContextKeyString(cross.Ctx, constant.ContextKeyAutoGroup))
}

func TestSmartRoutingFallsBackWhenQuotaTableIsUnavailable(t *testing.T) {
	setRoutingPolicyForTest(t, true, false)
	db := setupRoutingSelectorDB(t, false)
	channel := addRoutingSelectorChannel(t, db, "fallback", "default", 10, 1)
	ctx := newRoutingSessionContext("/v1/chat/completions", `{}`, 1, 1)
	param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "routing-model", RequestPath: "/v1/chat/completions", Retry: common.GetPointer(0)}

	selected, group, err := SelectSmartChannelRouting(param)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, channel.Id, selected.Id)
	assert.Equal(t, "default", group)
	state := existingChannelRoutingRequestState(ctx)
	require.NotNil(t, state)
	assert.Equal(t, "quota_snapshot_unavailable", state.Reason)

	data, err := GetChannelRoutingManagementData(context.Background())
	require.NoError(t, err)
	require.Len(t, data.Channels, 1)
	assert.Equal(t, "unknown", data.Channels[0].Quota.State)
}

func TestAppendChannelRoutingAdminInfoContainsNoSessionIdentifier(t *testing.T) {
	ctx := newRoutingSessionContext("/v1/responses", `{"conversation_id":"do-not-log"}`, 1, 2)
	ctx.Set(channelRoutingContextKey, &channelRoutingRequestState{
		Workload: ChannelRoutingWorkloadWork, Reason: "session_binding", SelectedGroup: "default",
		SelectedChannel: 9, SessionBound: true, SwitchReason: "upstream_status_500", SwitchCount: 1,
	})
	admin := map[string]interface{}{}
	AppendChannelRoutingAdminInfo(ctx, admin)
	encoded, err := common.Marshal(admin)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "do-not-log")
	assert.Contains(t, string(encoded), "channel_routing")
}

func TestRoutingQuotaCacheWaitCanBeCanceled(t *testing.T) {
	routingQuotaCache.Lock()
	routingQuotaCache.entries = make(map[int]routingQuotaCacheEntry)
	routingQuotaCache.loading = map[int]chan struct{}{77: make(chan struct{})}
	routingQuotaCache.Unlock()
	t.Cleanup(func() {
		routingQuotaCache.Lock()
		for id, wait := range routingQuotaCache.loading {
			close(wait)
			delete(routingQuotaCache.loading, id)
		}
		routingQuotaCache.Unlock()
	})
	ctx := &routingWaitTestContext{Context: context.Background(), entered: make(chan struct{}), done: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		_, err := loadRoutingQuotaRows(ctx, []int{77}, time.Now(), 1)
		result <- err
	}()
	<-ctx.entered
	close(ctx.done)
	require.ErrorIs(t, <-result, context.Canceled)
}

type routingWaitTestContext struct {
	context.Context
	entered chan struct{}
	done    chan struct{}
	once    sync.Once
}

func (c *routingWaitTestContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.done
}

func (c *routingWaitTestContext) Err() error {
	select {
	case <-c.done:
		return context.Canceled
	default:
		return nil
	}
}

func TestRoutingQuotaCacheCoalescesConcurrentChannelLoad(t *testing.T) {
	db := setupRoutingSelectorDB(t, true)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.ChannelQuotaSnapshot{
		ChannelId: 91, ObservedAt: now, Available: 5, Unit: "usd", MetricType: "balance",
		WindowType: "none", Source: "test", Status: "success",
	}).Error)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var queries atomic.Int32
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:routing-quota-load", func(tx *gorm.DB) {
		if tx.Statement.Table != "channel_quota_snapshots" {
			return
		}
		queries.Add(1)
		once.Do(func() {
			close(entered)
			<-release
		})
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:routing-quota-load") })

	type loadResult struct {
		rows map[int][]model.ChannelQuotaSnapshot
		err  error
	}
	results := make(chan loadResult, 2)
	load := func() {
		rows, err := loadRoutingQuotaRows(context.Background(), []int{91}, time.Now(), 1)
		results <- loadResult{rows: rows, err: err}
	}
	go load()
	<-entered
	go load()
	close(release)
	for i := 0; i < 2; i++ {
		result := <-results
		require.NoError(t, result.err)
		require.Len(t, result.rows[91], 1)
	}
	assert.Equal(t, int32(1), queries.Load())
}

func TestRoutingQuotaCacheCoalescesConcurrentLoadErrors(t *testing.T) {
	db := setupRoutingSelectorDB(t, false)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var queries atomic.Int32
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:routing-quota-error-load", func(tx *gorm.DB) {
		if tx.Statement.Table != "channel_quota_snapshots" {
			return
		}
		queries.Add(1)
		once.Do(func() {
			close(entered)
			<-release
		})
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:routing-quota-error-load") })

	results := make(chan error, 2)
	load := func() {
		_, err := loadRoutingQuotaRows(context.Background(), []int{92}, time.Now(), 1)
		results <- err
	}
	go load()
	<-entered
	go load()
	close(release)
	for i := 0; i < 2; i++ {
		require.Error(t, <-results)
	}
	assert.Equal(t, int32(1), queries.Load(), "concurrent callers must share the bounded negative cache")
}

func TestCompleteSmartRoutingUsesExplicitAttemptOutcome(t *testing.T) {
	setRoutingPolicyForTest(t, true, true)
	db := setupRoutingSelectorDB(t, true)
	require.NoError(t, db.AutoMigrate(&model.ChannelRoutingSession{}))
	now := time.Now().Unix()

	failureBinding, err := model.ClaimChannelRoutingSession(context.Background(), strings.Repeat("a", 64), 1, now, now+3600)
	require.NoError(t, err)
	failureContext := newRoutingSessionContext("/v1/responses", `{}`, 1, 1)
	failureContext.Set(channelRoutingContextKey, &channelRoutingRequestState{
		SelectedChannel: 1, SessionBound: true, Binding: failureBinding, AttemptOutcome: "channel_failure",
	})
	CompleteSmartChannelRouting(failureContext)
	var failed model.ChannelRoutingSession
	require.NoError(t, db.First(&failed, "key_hash = ?", failureBinding.KeyHash).Error)
	assert.NotZero(t, failed.FailureAt, "a streamed HTTP 200 channel failure must invalidate the binding")

	pendingBinding, err := model.ClaimChannelRoutingSession(context.Background(), strings.Repeat("b", 64), 2, now, now+3600)
	require.NoError(t, err)
	pendingContext := newRoutingSessionContext("/v1/responses", `{}`, 1, 1)
	pendingContext.Writer.WriteHeader(400)
	pendingContext.Set(channelRoutingContextKey, &channelRoutingRequestState{
		SelectedChannel: 2, SessionBound: true, Binding: pendingBinding, AttemptOutcome: "pending",
	})
	CompleteSmartChannelRouting(pendingContext)
	var pending model.ChannelRoutingSession
	require.NoError(t, db.First(&pending, "key_hash = ?", pendingBinding.KeyHash).Error)
	assert.Equal(t, pendingBinding, pending, "a local/client error must not invalidate a healthy channel binding")
}

func TestSmartRoutingExpiresBindingWhenSelectedChannelDisappearsBeforeLoad(t *testing.T) {
	setRoutingPolicyForTest(t, true, true)
	db := setupRoutingSelectorDB(t, true)
	require.NoError(t, db.AutoMigrate(&model.ChannelRoutingSession{}))
	channel := addRoutingSelectorChannel(t, db, "concurrently-deleted", "default", 10, 1)

	var deleteOnce sync.Once
	var deleteErr error
	require.NoError(t, db.Callback().Create().After("gorm:create").Register("test:routing-delete-selected", func(tx *gorm.DB) {
		if tx.Statement.Table != "channel_routing_sessions" {
			return
		}
		deleteOnce.Do(func() {
			deleteErr = db.Session(&gorm.Session{SkipHooks: true}).Where("id = ?", channel.Id).Delete(&model.Channel{}).Error
		})
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove("test:routing-delete-selected") })

	ctx := newRoutingSessionContext("/v1/responses", `{"conversation_id":"deleted-channel"}`, 8, 12)
	selected, _, err := SelectSmartChannelRouting(&RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: "routing-model",
		RequestPath: "/v1/responses", Retry: common.GetPointer(0),
	})
	assert.Nil(t, selected)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, deleteErr)

	var binding model.ChannelRoutingSession
	require.NoError(t, db.First(&binding, "key_hash = ?", channelRoutingSessionHash(ctx)).Error)
	assert.Equal(t, channel.Id, binding.ChannelId)
	assert.NotZero(t, binding.FailureAt)
	assert.LessOrEqual(t, binding.ExpiresAt, time.Now().Unix())
}
