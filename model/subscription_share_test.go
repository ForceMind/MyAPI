package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type subscriptionShareFixture struct {
	db       *gorm.DB
	root     User
	owner    User
	tokens   []Token
	channels []Channel
	subject  string
}

var subscriptionShareFixtureSequence atomic.Int64

func newSubscriptionShareFixture(t *testing.T, tokenCount, channelCount int) *subscriptionShareFixture {
	return newSubscriptionShareFixtureWithPrefix(t, tokenCount, channelCount, "")
}

func newSubscriptionShareFixtureWithPrefix(t *testing.T, tokenCount, channelCount int, prefix string) *subscriptionShareFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:subscription-share-%s-%d?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"), subscriptionShareFixtureSequence.Add(1))), &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: prefix}})
	require.NoError(t, err)
	entities := []any{&User{}, &Token{}, &Channel{}, &ChannelQuotaSnapshot{}, &TokenBudget{}}
	entities = append(entities, SubscriptionShareModels()...)
	require.NoError(t, db.AutoMigrate(entities...))
	f := &subscriptionShareFixture{db: db}
	f.root = User{Username: "share-root", AffCode: "share-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	f.owner = User{Username: "share-owner", AffCode: "share-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&f.root).Error)
	require.NoError(t, db.Create(&f.owner).Error)
	for index := 0; index < tokenCount; index++ {
		token := Token{UserId: f.owner.Id, Key: fmt.Sprintf("share-key-%d", index), Name: fmt.Sprintf("share-%d", index), Status: common.TokenStatusEnabled, ExpiredTime: -1}
		require.NoError(t, db.Create(&token).Error)
		f.tokens = append(f.tokens, token)
	}
	for index := 0; index < channelCount; index++ {
		channel := Channel{Name: fmt.Sprintf("codex-%d", index), Key: fmt.Sprintf("credential-%d", index), Status: common.ChannelStatusEnabled}
		require.NoError(t, db.Create(&channel).Error)
		f.channels = append(f.channels, channel)
	}
	f.subject, err = newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	return f
}

func TestSubscriptionShareQueriesHonorTablePrefix(t *testing.T) {
	f := newSubscriptionShareFixtureWithPrefix(t, 1, 1, "b2_")
	selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
	configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 10000, '9', 100)
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 1000, 0)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))
	eligible, err := CheckSubscriptionShareEligibility(context.Background(), f.db, f.tokens[0].Id, f.subject, 101)
	require.NoError(t, err)
	assert.True(t, eligible)
	require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("9", 64), "prefixed", fmt.Sprintf("%064x", 901), 102))
	view, err := ReadSubscriptionShares(context.Background(), f.db, f.owner.Id, f.tokens[0].Id, 102)
	require.NoError(t, err)
	require.Len(t, view.Pools, 1)
	assert.Equal(t, int64(1), view.Pools[0].PendingAttempts)
}

func (f *subscriptionShareFixture) snapshot(t *testing.T, channelIndex int, source string, observedAt, resetAt int64, used float64) ChannelQuotaSnapshot {
	t.Helper()
	total := float64(100)
	snapshot := ChannelQuotaSnapshot{CodexObservationQualified: true, CodexThresholdQualified: true, ChannelId: f.channels[channelIndex].Id, ObservedAt: observedAt, Available: 100 - used, Used: &used, Total: &total, Unit: "percent", MetricType: "codex_rate_limit", WindowType: "weekly", PlanType: "plus", WindowSeconds: 604800, ResetAt: resetAt, Source: source, SampleID: fmt.Sprintf("s-%d-%d", observedAt, channelIndex), SubjectRef: f.subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "success"}
	require.NoError(t, f.db.Create(&snapshot).Error)
	return snapshot
}

func configureSubscriptionShareFixture(t *testing.T, f *subscriptionShareFixture, tokenIndex int, poolID int64, snapshotID int, revision int64, shareBPS int, operationByte byte, now int64) {
	t.Helper()
	err := ConfigureSubscriptionShare(context.Background(), f.db, f.root.Id, f.tokens[tokenIndex].Id, SubscriptionShareConfigInput{ID: strings.Repeat(string(operationByte), 64), PoolID: poolID, SnapshotID: snapshotID, ExpectedRevision: revision, ShareBPS: shareBPS, ConfirmedApproximate: true}, now)
	require.NoError(t, err)
}

func TestSubscriptionShareLifecycleConservesApproximateDelta(t *testing.T) {
	f := newSubscriptionShareFixture(t, 2, 2)
	selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 8)
	configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 2500, 'a', 100)
	var pool SubscriptionSharePool
	require.NoError(t, f.db.First(&pool).Error)
	configureSubscriptionShareFixture(t, f, 1, pool.ID, selected.Id, 1, 7500, 'b', 100)

	f.snapshot(t, 1, codexQuotaWindowPrimarySource, 101, 1001, 10)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))
	var generation SubscriptionShareGeneration
	require.NoError(t, f.db.Where("pool_id = ? AND generation = ?", pool.ID, 1).Take(&generation).Error)
	assert.Equal(t, int64(10_000_000), generation.UnattributedMicro)
	assert.Equal(t, int64(101), generation.BaselineTime)

	for index, token := range []Token{f.tokens[0], f.tokens[1], f.tokens[1], f.tokens[1]} {
		require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, token.Id, token.UserId, f.subject, f.channels[1].Id, strings.Repeat("c", 64), fmt.Sprintf("request-%d", index), fmt.Sprintf("%064x", index+1), 110))
	}
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 120, 999, 10)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 120))
	var covered, allocated int64
	require.NoError(t, f.db.Model(&SubscriptionShareAttemptWindow{}).Where("covered_at <> 0").Count(&covered).Error)
	require.NoError(t, f.db.Model(&SubscriptionShareAttemptWindow{}).Where("allocated_at <> 0").Count(&allocated).Error)
	assert.Equal(t, int64(4), covered)
	assert.Zero(t, allocated)

	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 130, 1000, 30)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 130))
	var allocations []SubscriptionShareAllocation
	require.NoError(t, f.db.Order("token_id ASC").Find(&allocations).Error)
	require.Len(t, allocations, 2)
	used := map[int]int64{}
	for _, allocation := range allocations {
		var usage SubscriptionShareAllocationUsage
		require.NoError(t, f.db.Where("allocation_id = ? AND generation = ?", allocation.ID, 1).Take(&usage).Error)
		used[allocation.TokenID] = usage.EstimatedUsedMicro
	}
	assert.Equal(t, int64(5_000_000), used[f.tokens[0].Id])
	assert.Equal(t, int64(15_000_000), used[f.tokens[1].Id])

	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 140, 1000, 35)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 140))
	require.NoError(t, f.db.Where("pool_id = ? AND generation = ?", pool.ID, 1).Take(&generation).Error)
	assert.Equal(t, int64(15_000_000), generation.UnattributedMicro)
	var applications []SubscriptionShareSampleApplication
	require.NoError(t, f.db.Where("pool_id = ?", pool.ID).Order("snapshot_id ASC").Find(&applications).Error)
	require.Len(t, applications, 4)
	assert.Equal(t, "baseline", applications[0].Reason)
	assert.Equal(t, "zero_delta", applications[1].Reason)
	assert.Equal(t, int64(20_000_000), applications[2].DeltaMicro)
	assert.Equal(t, int64(4), applications[2].AttemptCount)
	assert.Equal(t, int64(5_000_000), applications[3].UnattributedDeltaMicro)

	ownerView, err := ReadSubscriptionShares(context.Background(), f.db, f.owner.Id, f.tokens[0].Id, 140)
	require.NoError(t, err)
	require.Len(t, ownerView.Pools, 1)
	assert.Equal(t, "active", ownerView.Pools[0].State)
	assert.Equal(t, "primary", ownerView.Pools[0].WindowSlot)
	assert.Equal(t, int64(5_000_000), ownerView.Pools[0].EstimatedUsedMicro)
	assert.Zero(t, ownerView.Pools[0].PendingAttempts)
}

func TestSubscriptionShareAttemptIdentityAndUnsampledCap(t *testing.T) {
	f := newSubscriptionShareFixture(t, 1, 1)
	selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
	configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 10000, 'c', 100)
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 1000, 0)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))

	firstID := fmt.Sprintf("%064x", 1)
	require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("d", 64), "request-1", firstID, 110))
	err := AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("d", 64), "request-1", firstID, 110)
	require.ErrorIs(t, err, ErrSubscriptionShareConflict)
	for index := 2; index <= 8; index++ {
		require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("d", 64), fmt.Sprintf("request-%d", index), fmt.Sprintf("%064x", index), 110))
	}
	err = AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("d", 64), "request-9", fmt.Sprintf("%064x", 9), 110)
	require.ErrorIs(t, err, ErrSubscriptionShareAwaitingSample)

	require.NoError(t, MarkSubscriptionShareAttemptNotSent(context.Background(), f.db, firstID, 111))
	require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("d", 64), "request-9", fmt.Sprintf("%064x", 9), 112))
	err = AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("d", 64), "request-1", firstID, 113)
	require.ErrorIs(t, err, ErrSubscriptionShareConflict)
	var attemptCount int64
	require.NoError(t, f.db.Model(&SubscriptionShareAttempt{}).Count(&attemptCount).Error)
	assert.Equal(t, int64(9), attemptCount)
}

func TestSubscriptionShareDiscontinuityAndTrustedReset(t *testing.T) {
	f := newSubscriptionShareFixture(t, 1, 1)
	selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 8)
	configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 10000, 'd', 100)
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 1000, 10)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 110, 1002, 12)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 110))
	var pool SubscriptionSharePool
	require.NoError(t, f.db.First(&pool).Error)
	assert.Equal(t, int64(1), pool.CurrentGeneration)

	require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("e", 64), "pending-before-discontinuity", fmt.Sprintf("%064x", 21), 115))
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 120, 1001, 11)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 120))
	eligible, err := CheckSubscriptionShareEligibility(context.Background(), f.db, f.tokens[0].Id, f.subject, 120)
	assert.False(t, eligible)
	require.ErrorIs(t, err, ErrSubscriptionShareUnavailable)

	// A reset timestamp change observed just before the previously published
	// boundary is a discontinuity, not permission to clear the old estimate.
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 999, 2000, 3)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 999))
	require.NoError(t, f.db.First(&pool).Error)
	assert.Equal(t, int64(1), pool.CurrentGeneration)

	// The first qualified sample at the old boundary establishes the next
	// generation and treats its current usage as unattributed baseline.
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 1000, 2000, 3)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 1000))
	require.NoError(t, f.db.First(&pool).Error)
	assert.Equal(t, int64(2), pool.CurrentGeneration)
	var oldUncertain int64
	require.NoError(t, f.db.Model(&SubscriptionShareAttemptWindow{}).Where("pool_id = ? AND generation = ? AND allocated_at = ?", pool.ID, 1, -1).Count(&oldUncertain).Error)
	assert.Equal(t, int64(1), oldUncertain)
	eligible, err = CheckSubscriptionShareEligibility(context.Background(), f.db, f.tokens[0].Id, f.subject, 1000)
	require.NoError(t, err)
	assert.True(t, eligible)
}

func TestSubscriptionShareDoesNotBridgeFailedOrLongGapSamples(t *testing.T) {
	f := newSubscriptionShareFixture(t, 1, 1)
	selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
	configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 10000, '7', 100)
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 1000, 10)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))
	require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("7", 64), "before-failure", fmt.Sprintf("%064x", 701), 105))

	// This is the actual normalized sampler failure shape: the generic source
	// has no window descriptor, but it still interrupts every managed slot.
	failure := ChannelQuotaSnapshot{ChannelId: f.channels[0].Id, ObservedAt: 110, MetricType: "codex_rate_limit", WindowType: "none", Unit: "percent", Source: "codex_wham_usage", SampleID: "failed-intermediate", SubjectRef: f.subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "error", ErrorCode: "upstream_http"}
	require.NoError(t, f.db.Create(&failure).Error)
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 111, 1000, 20)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 111))
	var pool SubscriptionSharePool
	require.NoError(t, f.db.First(&pool).Error)
	var generation SubscriptionShareGeneration
	require.NoError(t, f.db.Where("pool_id = ? AND generation = ?", pool.ID, 1).Take(&generation).Error)
	assert.Equal(t, SubscriptionShareStateStopped, generation.State)
	assert.Equal(t, "sample_failed", generation.Reason)
	var failureApplication SubscriptionShareSampleApplication
	require.NoError(t, f.db.Where("snapshot_id = ?", failure.Id).Take(&failureApplication).Error)
	assert.Equal(t, "sample_failed", failureApplication.Reason)
	assert.Equal(t, int64(20_000_000), generation.LastUsedMicro)
	assert.Equal(t, int64(20_000_000), generation.UnattributedMicro)
	var allocation SubscriptionShareAllocation
	require.NoError(t, f.db.Where("pool_id = ?", pool.ID).Take(&allocation).Error)
	var usage SubscriptionShareAllocationUsage
	require.NoError(t, f.db.Where("allocation_id = ? AND generation = ?", allocation.ID, 1).Take(&usage).Error)
	assert.Zero(t, usage.EstimatedUsedMicro)
	var window SubscriptionShareAttemptWindow
	require.NoError(t, f.db.Take(&window).Error)
	assert.Zero(t, window.AllocatedAt)

	other := newSubscriptionShareFixture(t, 1, 1)
	otherSelected := other.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 2000, 0)
	configureSubscriptionShareFixture(t, other, 0, 0, otherSelected.Id, 0, 10000, '8', 100)
	other.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 2000, 10)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), other.db, other.subject, 101))
	other.snapshot(t, 0, codexQuotaWindowPrimarySource, 500, 2000, 20)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), other.db, other.subject, 500))
	pool = SubscriptionSharePool{}
	generation = SubscriptionShareGeneration{}
	require.NoError(t, other.db.First(&pool).Error)
	require.NoError(t, other.db.Where("pool_id = ? AND generation = ?", pool.ID, 1).Take(&generation).Error)
	assert.Equal(t, SubscriptionShareStateStopped, generation.State)
	assert.Equal(t, "long_gap", generation.Reason)
}

func TestSubscriptionShareConsumesChangedAndAbsentSlotEvents(t *testing.T) {
	t.Run("changed descriptor", func(t *testing.T) {
		f := newSubscriptionShareFixture(t, 1, 1)
		selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
		configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 10000, '9', 100)
		f.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 1000, 10)
		require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))
		require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("9", 64), "before-replacement", fmt.Sprintf("%064x", 9001), 105))

		changed := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 110, 1000, 20)
		require.NoError(t, f.db.Model(&changed).Updates(map[string]any{"window_type": "five_hour", "window_seconds": int64(18000)}).Error)
		f.snapshot(t, 0, codexQuotaWindowPrimarySource, 111, 1000, 30)
		require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 111))

		var generation SubscriptionShareGeneration
		require.NoError(t, f.db.Take(&generation).Error)
		assert.Equal(t, SubscriptionShareStateStopped, generation.State)
		assert.Equal(t, "window_changed", generation.Reason)
		var application SubscriptionShareSampleApplication
		require.NoError(t, f.db.Where("snapshot_id = ?", changed.Id).Take(&application).Error)
		assert.Equal(t, "window_changed", application.Reason)
		var pool SubscriptionSharePool
		require.NoError(t, f.db.First(&pool).Error)
		require.NoError(t, ConfigureSubscriptionShare(context.Background(), f.db, f.root.Id, f.tokens[0].Id, SubscriptionShareConfigInput{ID: strings.Repeat("8", 64), PoolID: pool.ID, SnapshotID: changed.Id, ExpectedRevision: pool.Revision, ShareBPS: 10000, ConfirmedApproximate: true, ReplaceWindow: true}, 111))
		var uncertain int64
		require.NoError(t, f.db.Model(&SubscriptionShareAttemptWindow{}).Where("generation = ? AND allocated_at = ?", 1, -1).Count(&uncertain).Error)
		assert.Equal(t, int64(1), uncertain)
		var audit SubscriptionShareAudit
		require.NoError(t, f.db.Where("id = ?", strings.Repeat("8", 64)).Take(&audit).Error)
		assert.Equal(t, int64(1), audit.UncertainAttempts)
	})

	t.Run("window absent", func(t *testing.T) {
		f := newSubscriptionShareFixture(t, 1, 1)
		selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
		configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 10000, 'a', 100)
		f.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 1000, 10)
		require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))

		absence := ChannelQuotaSnapshot{CodexObservationQualified: true, CodexThresholdQualified: true, ChannelId: f.channels[0].Id, ObservedAt: 110, MetricType: "codex_rate_limit", WindowType: "weekly", PlanType: "plus", WindowSeconds: 604800, ResetAt: 1000, Unit: "percent", Source: codexQuotaWindowPrimarySource, SampleID: "absence", SubjectRef: f.subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "unsupported", ErrorCode: "window_absent"}
		require.NoError(t, f.db.Create(&absence).Error)
		f.snapshot(t, 0, codexQuotaWindowPrimarySource, 111, 1000, 20)
		require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 111))

		var generation SubscriptionShareGeneration
		require.NoError(t, f.db.Take(&generation).Error)
		assert.Equal(t, SubscriptionShareStateStopped, generation.State)
		assert.Equal(t, "window_absent", generation.Reason)
		var application SubscriptionShareSampleApplication
		require.NoError(t, f.db.Where("snapshot_id = ?", absence.Id).Take(&application).Error)
		assert.Equal(t, "window_absent", application.Reason)
	})
}

func TestSubscriptionShareConfigurationBoundariesAndCAS(t *testing.T) {
	f := newSubscriptionShareFixture(t, 2, 1)
	selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
	configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 10000, 'e', 100)
	var pool SubscriptionSharePool
	require.NoError(t, f.db.First(&pool).Error)
	configureSubscriptionShareFixture(t, f, 1, pool.ID, selected.Id, 1, 0, 'f', 100)

	err := ConfigureSubscriptionShare(context.Background(), f.db, f.root.Id, f.tokens[1].Id, SubscriptionShareConfigInput{ID: strings.Repeat("1", 64), PoolID: pool.ID, SnapshotID: selected.Id, ExpectedRevision: 2, ShareBPS: 1, ConfirmedApproximate: true}, 100)
	require.ErrorIs(t, err, ErrSubscriptionShareInvalid)
	err = ConfigureSubscriptionShare(context.Background(), f.db, f.root.Id, f.tokens[0].Id, SubscriptionShareConfigInput{ID: strings.Repeat("2", 64), PoolID: pool.ID, SnapshotID: selected.Id, ExpectedRevision: 1, ShareBPS: 9000, ConfirmedApproximate: true}, 100)
	require.ErrorIs(t, err, ErrSubscriptionShareConflict)

	input := SubscriptionShareConfigInput{ID: strings.Repeat("3", 64), PoolID: pool.ID, SnapshotID: selected.Id, ExpectedRevision: 2, ShareBPS: 9000, ConfirmedApproximate: true}
	require.NoError(t, ConfigureSubscriptionShare(context.Background(), f.db, f.root.Id, f.tokens[0].Id, input, 100))
	require.NoError(t, ConfigureSubscriptionShare(context.Background(), f.db, f.root.Id, f.tokens[0].Id, input, 500))
	input.ShareBPS = 8000
	err = ConfigureSubscriptionShare(context.Background(), f.db, f.root.Id, f.tokens[0].Id, input, 100)
	require.ErrorIs(t, err, ErrSubscriptionShareConflict)

	baseline := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 1000, 5)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))
	var allocation SubscriptionShareAllocation
	require.NoError(t, f.db.Where("pool_id = ? AND token_id = ?", pool.ID, f.tokens[0].Id).Take(&allocation).Error)
	require.NoError(t, f.db.Model(&SubscriptionShareAllocationUsage{}).Where("allocation_id = ? AND generation = ?", allocation.ID, 1).Update("estimated_used_micro", int64(1_234_567)).Error)
	emergency := SubscriptionShareConfigInput{ID: strings.Repeat("6", 64), PoolID: pool.ID, SnapshotID: baseline.Id, ExpectedRevision: 3, ShareBPS: 0, ConfirmedApproximate: true}
	require.NoError(t, ConfigureSubscriptionShare(context.Background(), f.db, f.root.Id, f.tokens[0].Id, emergency, 2000))
	var usage SubscriptionShareAllocationUsage
	require.NoError(t, f.db.Where("allocation_id = ? AND generation = ?", allocation.ID, 1).Take(&usage).Error)
	assert.Equal(t, int64(1_234_567), usage.EstimatedUsedMicro)
}

func TestSubscriptionShareConfigurationSerializesTwoWriters(t *testing.T) {
	f := newSubscriptionShareFixture(t, 2, 1)
	selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
	configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 4000, 'a', 100)
	var pool SubscriptionSharePool
	require.NoError(t, f.db.First(&pool).Error)
	configureSubscriptionShareFixture(t, f, 1, pool.ID, selected.Id, 1, 4000, 'b', 100)

	start := make(chan struct{})
	errorsByWriter := make(chan error, 2)
	for index := 0; index < 2; index++ {
		index := index
		go func() {
			<-start
			errorsByWriter <- ConfigureSubscriptionShare(context.Background(), f.db, f.root.Id, f.tokens[index].Id, SubscriptionShareConfigInput{ID: strings.Repeat(fmt.Sprintf("%x", index+12), 64), PoolID: pool.ID, SnapshotID: selected.Id, ExpectedRevision: 2, ShareBPS: 5000, ConfirmedApproximate: true}, 100)
		}()
	}
	close(start)
	results := []error{<-errorsByWriter, <-errorsByWriter}
	successes, conflicts := 0, 0
	for _, err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrSubscriptionShareConflict) {
			conflicts++
		} else {
			require.NoError(t, err)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)
	var allocated int64
	require.NoError(t, f.db.Model(&SubscriptionShareAllocation{}).Where("pool_id = ?", pool.ID).Select("COALESCE(SUM(share_bps),0)").Scan(&allocated).Error)
	assert.Equal(t, int64(9000), allocated)
}

func TestSubscriptionShareAdmissionAcrossAllPoolsIsAtomic(t *testing.T) {
	f := newSubscriptionShareFixture(t, 1, 2)
	primary := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
	secondary := f.snapshot(t, 0, codexQuotaWindowSecondarySource, 100, 2000, 0)
	configureSubscriptionShareFixture(t, f, 0, 0, primary.Id, 0, 5000, '4', 100)
	configureSubscriptionShareFixture(t, f, 0, 0, secondary.Id, 0, 5000, '5', 100)
	f.snapshot(t, 1, codexQuotaWindowPrimarySource, 101, 1000, 0)
	f.snapshot(t, 1, codexQuotaWindowSecondarySource, 101, 2000, 0)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))
	var secondaryPool SubscriptionSharePool
	require.NoError(t, f.db.Where("window_source = ?", codexQuotaWindowSecondarySource).Take(&secondaryPool).Error)
	var allocation SubscriptionShareAllocation
	require.NoError(t, f.db.Where("pool_id = ? AND token_id = ?", secondaryPool.ID, f.tokens[0].Id).Take(&allocation).Error)
	require.NoError(t, f.db.Model(&SubscriptionShareAllocationUsage{}).Where("allocation_id = ? AND generation = ?", allocation.ID, 1).Update("estimated_used_micro", int64(50_000_000)).Error)

	err := AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[1].Id, strings.Repeat("f", 64), "atomic-rejection", fmt.Sprintf("%064x", 99), 110)
	require.ErrorIs(t, err, ErrSubscriptionShareExceeded)
	var attempts int64
	require.NoError(t, f.db.Model(&SubscriptionShareAttempt{}).Count(&attempts).Error)
	assert.Zero(t, attempts)
}

func TestSubscriptionShareRejectedAdmissionCommitsProjection(t *testing.T) {
	t.Run("exceeded", func(t *testing.T) {
		f := newSubscriptionShareFixture(t, 1, 1)
		selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
		configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 2000, '1', 100)
		f.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 1000, 10)
		require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))
		require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("1", 64), "before-cap", fmt.Sprintf("%064x", 1001), 105))
		capSample := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 110, 1000, 30)

		err := AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("1", 64), "rejected", fmt.Sprintf("%064x", 1002), 111)
		require.ErrorIs(t, err, ErrSubscriptionShareExceeded)
		var usage SubscriptionShareAllocationUsage
		require.NoError(t, f.db.Take(&usage).Error)
		assert.Equal(t, int64(20_000_000), usage.EstimatedUsedMicro)
		var application SubscriptionShareSampleApplication
		require.NoError(t, f.db.Where("snapshot_id = ?", capSample.Id).Take(&application).Error)
		assert.Equal(t, "positive_delta", application.Reason)

		err = AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("1", 64), "rejected-again", fmt.Sprintf("%064x", 1003), 112)
		require.ErrorIs(t, err, ErrSubscriptionShareExceeded)
		var applications int64
		require.NoError(t, f.db.Model(&SubscriptionShareSampleApplication{}).Where("snapshot_id = ?", capSample.Id).Count(&applications).Error)
		assert.Equal(t, int64(1), applications)
	})

	t.Run("failed sample", func(t *testing.T) {
		f := newSubscriptionShareFixture(t, 1, 1)
		selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
		configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 10000, '4', 100)
		f.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 1000, 10)
		require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))
		failure := ChannelQuotaSnapshot{ChannelId: f.channels[0].Id, ObservedAt: 110, MetricType: "codex_rate_limit", WindowType: "none", Unit: "percent", Source: "codex_wham_usage", SampleID: "admission-failure", SubjectRef: f.subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "error", ErrorCode: "upstream_http"}
		require.NoError(t, f.db.Create(&failure).Error)
		eligible, err := CheckSubscriptionShareEligibility(context.Background(), f.db, f.tokens[0].Id, f.subject, 111)
		assert.False(t, eligible)
		require.ErrorIs(t, err, ErrSubscriptionShareUnavailable)
		var application SubscriptionShareSampleApplication
		require.NoError(t, f.db.Where("snapshot_id = ?", failure.Id).Take(&application).Error)
		assert.Equal(t, "sample_failed", application.Reason)
	})
}

func TestSubscriptionShareIDCursorAndSecondPrecisionBoundary(t *testing.T) {
	t.Run("late older evidence stops projection", func(t *testing.T) {
		f := newSubscriptionShareFixture(t, 1, 1)
		selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
		configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 10000, '2', 100)
		f.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 1000, 10)
		require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))
		f.snapshot(t, 0, codexQuotaWindowPrimarySource, 120, 1000, 20)
		require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 120))
		late := ChannelQuotaSnapshot{ChannelId: f.channels[0].Id, ObservedAt: 110, MetricType: "codex_rate_limit", WindowType: "none", Unit: "percent", Source: "codex_wham_usage", SampleID: "late-failure", SubjectRef: f.subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "error", ErrorCode: "upstream_http"}
		require.NoError(t, f.db.Create(&late).Error)
		var protected int64
		require.NoError(t, ProtectedSubscriptionShareEvidenceQuery(f.db).Where("id = ?", late.Id).Count(&protected).Error)
		assert.Equal(t, int64(1), protected)
		f.snapshot(t, 0, codexQuotaWindowPrimarySource, 130, 1000, 30)
		require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 130))
		var generation SubscriptionShareGeneration
		require.NoError(t, f.db.Take(&generation).Error)
		assert.Equal(t, SubscriptionShareStateStopped, generation.State)
		assert.Equal(t, "evidence_time_regression", generation.Reason)
		var application SubscriptionShareSampleApplication
		require.NoError(t, f.db.Where("snapshot_id = ?", late.Id).Take(&application).Error)
		assert.Equal(t, "evidence_time_regression", application.Reason)
	})

	t.Run("same-second attempt is not assigned backward", func(t *testing.T) {
		f := newSubscriptionShareFixture(t, 1, 1)
		selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
		configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 10000, '3', 100)
		f.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 1000, 10)
		require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))
		require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("3", 64), "same-second", fmt.Sprintf("%064x", 1100), 110))
		f.snapshot(t, 0, codexQuotaWindowPrimarySource, 110, 1000, 20)
		require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 110))
		var usage SubscriptionShareAllocationUsage
		require.NoError(t, f.db.Take(&usage).Error)
		assert.Zero(t, usage.EstimatedUsedMicro)
		var window SubscriptionShareAttemptWindow
		require.NoError(t, f.db.Take(&window).Error)
		assert.Zero(t, window.CoveredAt)
		assert.Zero(t, window.AllocatedAt)
	})
}

func TestSubscriptionShareCandidatesAreNotTruncatedBeforeDescriptorDeduplication(t *testing.T) {
	f := newSubscriptionShareFixture(t, 1, 1)
	total := float64(100)
	used := float64(10)
	rows := make([]ChannelQuotaSnapshot, 0, 257)
	rows = append(rows, ChannelQuotaSnapshot{CodexObservationQualified: true, CodexThresholdQualified: true, ChannelId: f.channels[0].Id, ObservedAt: 200, Available: 90, Used: &used, Total: &total, Unit: "percent", MetricType: "codex_rate_limit", WindowType: "weekly", PlanType: "plus", WindowSeconds: 604800, ResetAt: 1000, Source: codexQuotaWindowSecondarySource, SampleID: "candidate-secondary", SubjectRef: f.subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "success"})
	for index := 0; index < 256; index++ {
		rows = append(rows, ChannelQuotaSnapshot{CodexObservationQualified: true, CodexThresholdQualified: true, ChannelId: f.channels[0].Id, ObservedAt: 200, Available: 90, Used: &used, Total: &total, Unit: "percent", MetricType: "codex_rate_limit", WindowType: "weekly", PlanType: "plus", WindowSeconds: 604800, ResetAt: 1000, Source: codexQuotaWindowPrimarySource, SampleID: fmt.Sprintf("candidate-primary-%03d", index), SubjectRef: f.subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "success"})
	}
	require.NoError(t, f.db.CreateInBatches(&rows, 100).Error)
	view, err := ReadSubscriptionShares(context.Background(), f.db, f.root.Id, f.tokens[0].Id, 200)
	require.NoError(t, err)
	require.Len(t, view.Pools, 2)
	assert.Equal(t, "primary", view.Pools[0].WindowSlot)
	assert.Equal(t, "secondary", view.Pools[1].WindowSlot)
}

func TestSubscriptionShareExplicitObservationRebaselinePreservesUsage(t *testing.T) {
	f := newSubscriptionShareFixture(t, 1, 1)
	selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
	configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 10000, '5', 100)
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 1000, 10)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))
	require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("5", 64), "allocated", fmt.Sprintf("%064x", 5001), 105))
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 110, 1000, 20)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 110))
	require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("5", 64), "uncertain", fmt.Sprintf("%064x", 5002), 111))
	failure := ChannelQuotaSnapshot{ChannelId: f.channels[0].Id, ObservedAt: 112, MetricType: "codex_rate_limit", WindowType: "none", Unit: "percent", Source: "codex_wham_usage", SampleID: "rebaseline-failure", SubjectRef: f.subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "error", ErrorCode: "upstream_http"}
	require.NoError(t, f.db.Create(&failure).Error)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 112))
	lower := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 115, 1000, 15)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 115))
	var pool SubscriptionSharePool
	require.NoError(t, f.db.First(&pool).Error)
	view, err := ReadSubscriptionShares(context.Background(), f.db, f.root.Id, f.tokens[0].Id, 115)
	require.NoError(t, err)
	require.Len(t, view.Pools, 1)
	assert.False(t, view.Pools[0].RebaselineRequired)
	err = ConfigureSubscriptionShare(context.Background(), f.db, f.root.Id, f.tokens[0].Id, SubscriptionShareConfigInput{ID: strings.Repeat("7", 64), PoolID: pool.ID, SnapshotID: lower.Id, ExpectedRevision: pool.Revision, ShareBPS: 10000, ConfirmedApproximate: true, ReplaceWindow: true}, 115)
	require.ErrorIs(t, err, ErrSubscriptionShareConflict)
	fresh := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 120, 1000, 25)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 120))
	require.NoError(t, f.db.First(&pool).Error)
	require.NoError(t, ConfigureSubscriptionShare(context.Background(), f.db, f.root.Id, f.tokens[0].Id, SubscriptionShareConfigInput{ID: strings.Repeat("6", 64), PoolID: pool.ID, SnapshotID: fresh.Id, ExpectedRevision: pool.Revision, ShareBPS: 10000, ConfirmedApproximate: true, ReplaceWindow: true}, 120))

	var generation SubscriptionShareGeneration
	require.NoError(t, f.db.Where("pool_id = ?", pool.ID).Take(&generation).Error)
	assert.Equal(t, int64(1), generation.Generation)
	assert.Equal(t, SubscriptionShareStateReady, generation.State)
	var usage SubscriptionShareAllocationUsage
	require.NoError(t, f.db.Take(&usage).Error)
	assert.Equal(t, int64(10_000_000), usage.EstimatedUsedMicro)
	var uncertain int64
	require.NoError(t, f.db.Model(&SubscriptionShareAttemptWindow{}).Where("allocated_at = ?", -1).Count(&uncertain).Error)
	assert.Equal(t, int64(1), uncertain)
	require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("5", 64), "after-rebaseline", fmt.Sprintf("%064x", 5003), 121))
}
