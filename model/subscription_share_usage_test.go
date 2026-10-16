package model

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type subscriptionShareUsageReadBarrierKey struct{}

// This contract runs on all three configured disposable databases. A writer
// commits between the reader's generation and usage queries; the response must
// retain the earlier complete snapshot, rather than joining different commits.
func subscriptionSharePoolUsageSnapshotContract(t *testing.T, db *gorm.DB, actorID int, poolID, now int64) {
	t.Helper()
	if db.Dialector.Name() == "sqlite" {
		require.NoError(t, db.Exec("PRAGMA journal_mode = WAL").Error)
	}
	before, err := ReadSubscriptionSharePoolUsage(context.Background(), db, actorID, poolID, now)
	require.NoError(t, err)
	require.Equal(t, "available", before.DecompositionState)
	var allocation SubscriptionShareAllocation
	require.NoError(t, db.Where("pool_id = ?", poolID).Order("id").First(&allocation).Error)
	reached, resume := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	readContext, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	var reachedOnce, resumeOnce sync.Once
	callback := "test:pool-usage-snapshot"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(query *gorm.DB) {
		if query.Statement.Context.Value(subscriptionShareUsageReadBarrierKey{}) == true && query.Statement.Schema != nil && query.Statement.Schema.Name == "SubscriptionShareGeneration" {
			reachedOnce.Do(func() {
				close(reached)
				<-resume
			})
		}
	}))
	t.Cleanup(func() {
		resumeOnce.Do(func() { close(resume) })
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("pool usage reader did not stop after cancellation")
		}
		require.NoError(t, db.Callback().Query().Remove(callback))
	})
	type result struct {
		view *SubscriptionSharePoolUsage
		err  error
	}
	results := make(chan result, 1)
	go func() {
		defer close(done)
		ctx := context.WithValue(readContext, subscriptionShareUsageReadBarrierKey{}, true)
		view, readErr := ReadSubscriptionSharePoolUsage(ctx, db, actorID, poolID, now)
		results <- result{view, readErr}
	}()
	select {
	case <-reached:
	case early := <-results:
		require.NoError(t, early.err)
		t.Fatal("pool usage read did not reach its generation query")
	case <-readContext.Done():
		t.Fatal("pool usage reader did not reach its generation query before the deadline")
	}
	change := func(delta int64) error {
		return db.WithContext(readContext).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&SubscriptionShareAllocationUsage{}).Where("allocation_id = ? AND generation = ?", allocation.ID, before.Generation).Update("estimated_used_micro", gorm.Expr("estimated_used_micro + ?", delta)).Error; err != nil {
				return err
			}
			return tx.Model(&SubscriptionShareGeneration{}).Where("pool_id = ? AND generation = ?", poolID, before.Generation).Update("last_used_micro", gorm.Expr("last_used_micro + ?", delta)).Error
		})
	}
	require.NoError(t, change(1_000_000))
	resumeOnce.Do(func() { close(resume) })
	var read result
	select {
	case read = <-results:
	case <-readContext.Done():
		t.Fatal("pool usage reader did not complete before the deadline")
	}
	require.NoError(t, read.err)
	assert.Equal(t, before, read.view, "the reader must not splice a later usage commit into an earlier generation")
	after, err := ReadSubscriptionSharePoolUsage(context.Background(), db, actorID, poolID, now)
	require.NoError(t, err)
	require.NotNil(t, after.EstimatedUsedMicro)
	assert.Equal(t, *before.EstimatedUsedMicro+1_000_000, *after.EstimatedUsedMicro)
	require.NoError(t, change(-1_000_000))
}

func newSubscriptionShareUsageFixture(t *testing.T) (*subscriptionShareFixture, SubscriptionSharePool) {
	t.Helper()
	f := newSubscriptionShareFixtureWithPrefix(t, 2, 1, "usage_")
	selected := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 35)
	configureSubscriptionShareFixture(t, f, 0, 0, selected.Id, 0, 2000, '1', 100)
	var pool SubscriptionSharePool
	require.NoError(t, f.db.First(&pool).Error)
	configureSubscriptionShareFixture(t, f, 1, pool.ID, selected.Id, 1, 5000, '2', 100)
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 101, 1000, 35)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 101))
	for index, token := range []Token{f.tokens[0], f.tokens[1], f.tokens[1]} {
		require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, token.Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("a", 64), fmt.Sprintf("usage-request-%d", index), fmt.Sprintf("%064x", 100+index), 105))
	}
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 110, 1000, 50)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 110))
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 112, 1000, 55)
	require.NoError(t, RefreshSubscriptionShareProjection(context.Background(), f.db, f.subject, 112))
	return f, pool
}

func TestSubscriptionSharePoolUsageReadOnlyDecomposition(t *testing.T) {
	f, pool := newSubscriptionShareUsageFixture(t)
	ctx := context.Background()
	for index, token := range f.tokens {
		require.NoError(t, AdmitSubscriptionShareAttempt(ctx, f.db, token.Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("a", 64), fmt.Sprintf("pending-%d", index), fmt.Sprintf("%064x", 200+index), 115))
	}
	// A future persisted sample remains unprojected: this GET must not apply it.
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 116, 1000, 60)
	var beforeGeneration SubscriptionShareGeneration
	var beforeUsage []SubscriptionShareAllocationUsage
	var beforeApplications []SubscriptionShareSampleApplication
	var beforeWindows []SubscriptionShareAttemptWindow
	require.NoError(t, f.db.First(&beforeGeneration).Error)
	require.NoError(t, f.db.Order("id").Find(&beforeUsage).Error)
	require.NoError(t, f.db.Order("id").Find(&beforeApplications).Error)
	require.NoError(t, f.db.Order("id").Find(&beforeWindows).Error)
	sqlDB, err := f.db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, f.db.Exec("PRAGMA query_only = ON").Error)
	t.Cleanup(func() { require.NoError(t, f.db.Exec("PRAGMA query_only = OFF").Error) })
	view, err := ReadSubscriptionSharePoolUsage(ctx, f.db, f.root.Id, pool.ID, 117)
	require.NoError(t, err)
	assert.Equal(t, "available", view.DecompositionState)
	assert.Equal(t, "active", view.State)
	assert.EqualValues(t, 7000, view.AllocatedBPS)
	require.NotNil(t, view.InitialBaselineUsedMicro)
	assert.EqualValues(t, 35_000_000, *view.InitialBaselineUsedMicro)
	assert.EqualValues(t, 101, *view.InitialBaselineTime)
	assert.EqualValues(t, 15_000_000, *view.EstimatedUsedMicro)
	assert.EqualValues(t, 40_000_000, *view.UnattributedMicro)
	assert.EqualValues(t, 5_000_000, *view.UnattributedGrowthMicro)
	assert.EqualValues(t, 55_000_000, *view.ObservedUsedMicro)
	assert.EqualValues(t, 2, view.PendingAttempts)
	assert.EqualValues(t, 2, view.UnsampledAttempts)
	require.Len(t, view.Allocations, 2)
	assert.EqualValues(t, 5_000_000, *view.Allocations[0].EstimatedUsedMicro)
	assert.EqualValues(t, 10_000_000, *view.Allocations[1].EstimatedUsedMicro)
	assert.EqualValues(t, 1, view.Allocations[0].PendingAttempts)
	encoded, err := common.Marshal(view)
	require.NoError(t, err)
	for _, private := range []string{f.tokens[0].Key, f.channels[0].Key, f.subject, "credential_ref", "request_id", "attempt_id", "subject_ref", `"key":`, "pending-0"} {
		assert.NotContains(t, string(encoded), private)
	}
	var afterGeneration SubscriptionShareGeneration
	var afterUsage []SubscriptionShareAllocationUsage
	var afterApplications []SubscriptionShareSampleApplication
	var afterWindows []SubscriptionShareAttemptWindow
	require.NoError(t, f.db.First(&afterGeneration).Error)
	require.NoError(t, f.db.Order("id").Find(&afterUsage).Error)
	require.NoError(t, f.db.Order("id").Find(&afterApplications).Error)
	require.NoError(t, f.db.Order("id").Find(&afterWindows).Error)
	assert.Equal(t, beforeGeneration, afterGeneration)
	assert.Equal(t, beforeUsage, afterUsage)
	assert.Equal(t, beforeApplications, afterApplications)
	assert.Equal(t, beforeWindows, afterWindows)
}

func TestSubscriptionSharePoolUsageRootOnlyAndRetainedIdentities(t *testing.T) {
	f, pool := newSubscriptionShareUsageFixture(t)
	ctx := context.Background()
	_, err := ReadSubscriptionSharePoolUsage(ctx, f.db, f.owner.Id, pool.ID, 113)
	require.ErrorIs(t, err, ErrSubscriptionShareDenied)
	require.NoError(t, f.db.Model(&User{}).Where("id = ?", f.owner.Id).Update("role", common.RoleAdminUser).Error)
	_, err = ReadSubscriptionSharePoolUsage(ctx, f.db, f.owner.Id, pool.ID, 113)
	require.ErrorIs(t, err, ErrSubscriptionShareDenied)
	require.NoError(t, f.db.Model(&SubscriptionShareAllocation{}).Where("pool_id = ?", pool.ID).Update("share_bps", 0).Error)
	require.NoError(t, f.db.Delete(&f.tokens[0]).Error)
	require.NoError(t, f.db.Unscoped().Delete(&f.tokens[1]).Error)
	require.NoError(t, f.db.Model(&SubscriptionSharePool{}).Where("id = ?", pool.ID).Update("enabled", false).Error)
	view, err := ReadSubscriptionSharePoolUsage(ctx, f.db, f.root.Id, pool.ID, 113)
	require.NoError(t, err)
	assert.Equal(t, "closed", view.State)
	require.Len(t, view.Allocations, 2)
	assert.Equal(t, "deleted", view.Allocations[0].TokenState)
	assert.Equal(t, f.tokens[0].Name, *view.Allocations[0].TokenName)
	assert.Equal(t, "missing", view.Allocations[1].TokenState)
	assert.Nil(t, view.Allocations[1].TokenName)
	assert.EqualValues(t, 15_000_000, *view.EstimatedUsedMicro)
	assert.Zero(t, view.AllocatedBPS)
	require.NoError(t, f.db.Model(&User{}).Where("id = ?", f.root.Id).Update("status", common.UserStatusDisabled).Error)
	_, err = ReadSubscriptionSharePoolUsage(ctx, f.db, f.root.Id, pool.ID, 113)
	require.ErrorIs(t, err, ErrSubscriptionShareDenied)
}

func TestSubscriptionSharePoolUsageEvidenceMissingAndBaselinePending(t *testing.T) {
	f, pool := newSubscriptionShareUsageFixture(t)
	ctx := context.Background()
	// Evidence may be absent on an imported installation. Do not reconstruct it
	// from the mutable recovery baseline or rewrite historical receipts.
	require.NoError(t, f.db.Session(&gorm.Session{SkipHooks: true}).Where("pool_id = ? AND reason = ?", pool.ID, "baseline").Delete(&SubscriptionShareSampleApplication{}).Error)
	view, err := ReadSubscriptionSharePoolUsage(ctx, f.db, f.root.Id, pool.ID, 113)
	require.NoError(t, err)
	assert.Equal(t, "baseline_unknown", view.DecompositionState)
	assert.Nil(t, view.InitialBaselineUsedMicro)
	assert.Nil(t, view.UnattributedGrowthMicro)
	assert.EqualValues(t, 15_000_000, *view.EstimatedUsedMicro)
	var firstUsage SubscriptionShareAllocationUsage
	require.NoError(t, f.db.Order("id").First(&firstUsage).Error)
	require.NoError(t, f.db.Delete(&firstUsage).Error)
	view, err = ReadSubscriptionSharePoolUsage(ctx, f.db, f.root.Id, pool.ID, 113)
	require.NoError(t, err)
	assert.Equal(t, "inconsistent", view.DecompositionState)
	assert.Nil(t, view.EstimatedUsedMicro)
	assert.Nil(t, view.Allocations[0].EstimatedUsedMicro)

	pending := newSubscriptionShareFixture(t, 1, 1)
	selected := pending.snapshot(t, 0, codexQuotaWindowPrimarySource, 100, 1000, 0)
	configureSubscriptionShareFixture(t, pending, 0, 0, selected.Id, 0, 10000, '3', 100)
	require.NoError(t, pending.db.First(&pool).Error)
	view, err = ReadSubscriptionSharePoolUsage(ctx, pending.db, pending.root.Id, pool.ID, 100)
	require.NoError(t, err)
	assert.Equal(t, "baseline_pending", view.DecompositionState)
	assert.Nil(t, view.ObservedUsedMicro)
	assert.Nil(t, view.BaselineUsedMicro)
	assert.Nil(t, view.Allocations[0].EstimatedUsedMicro)
}

func TestSubscriptionSharePoolUsageRebaselineKeepsOriginalDecomposition(t *testing.T) {
	f, pool := newSubscriptionShareUsageFixture(t)
	ctx := context.Background()
	failure := ChannelQuotaSnapshot{ChannelId: f.channels[0].Id, ObservedAt: 114, MetricType: "codex_rate_limit", WindowType: "none", Unit: "percent", Source: "codex_wham_usage", SampleID: "usage-failure", SubjectRef: f.subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, Status: "error", ErrorCode: "upstream_http"}
	require.NoError(t, f.db.Create(&failure).Error)
	require.NoError(t, RefreshSubscriptionShareProjection(ctx, f.db, f.subject, 114))
	fresh := f.snapshot(t, 0, codexQuotaWindowPrimarySource, 120, 1000, 60)
	require.NoError(t, RefreshSubscriptionShareProjection(ctx, f.db, f.subject, 120))
	require.NoError(t, f.db.First(&pool).Error)
	require.NoError(t, ConfigureSubscriptionShare(ctx, f.db, f.root.Id, f.tokens[0].Id, SubscriptionShareConfigInput{ID: strings.Repeat("4", 64), PoolID: pool.ID, SnapshotID: fresh.Id, ExpectedRevision: pool.Revision, ShareBPS: 2000, ConfirmedApproximate: true, ReplaceWindow: true}, 120))
	view, err := ReadSubscriptionSharePoolUsage(ctx, f.db, f.root.Id, pool.ID, 121)
	require.NoError(t, err)
	assert.Equal(t, "available", view.DecompositionState)
	assert.EqualValues(t, 60_000_000, *view.BaselineUsedMicro)
	assert.EqualValues(t, 35_000_000, *view.InitialBaselineUsedMicro)
	assert.EqualValues(t, 10_000_000, *view.UnattributedGrowthMicro)
	assert.EqualValues(t, 15_000_000, *view.EstimatedUsedMicro)
	assert.EqualValues(t, 1, view.Generation)
	assert.EqualValues(t, 120, view.BaselineTime)
	assert.EqualValues(t, 101, *view.InitialBaselineTime)
}

func TestSubscriptionSharePoolUsageResetUsesNewZeroBaseline(t *testing.T) {
	f, pool := newSubscriptionShareUsageFixture(t)
	ctx := context.Background()
	// Timely samples preserve continuity until the first credible new cycle.
	for _, observed := range []int64{350, 600, 850} {
		f.snapshot(t, 0, codexQuotaWindowPrimarySource, observed, 1000, 55)
		require.NoError(t, RefreshSubscriptionShareProjection(ctx, f.db, f.subject, observed))
	}
	f.snapshot(t, 0, codexQuotaWindowPrimarySource, 1001, 1900, 0)
	require.NoError(t, RefreshSubscriptionShareProjection(ctx, f.db, f.subject, 1001))
	view, err := ReadSubscriptionSharePoolUsage(ctx, f.db, f.root.Id, pool.ID, 1002)
	require.NoError(t, err)
	assert.EqualValues(t, 2, view.Generation)
	assert.Equal(t, "available", view.DecompositionState)
	require.NotNil(t, view.InitialBaselineUsedMicro)
	assert.Zero(t, *view.InitialBaselineUsedMicro)
	assert.Zero(t, *view.EstimatedUsedMicro)
	assert.Zero(t, *view.UnattributedGrowthMicro)
	assert.EqualValues(t, 7000, view.AllocatedBPS)
}

func TestSubscriptionSharePoolUsageInvalidEvidenceIsNotActive(t *testing.T) {
	for _, test := range []struct {
		name      string
		updates   map[string]any
		wantError bool
	}{
		{"negative baseline", map[string]any{"baseline_used_micro": -1}, true},
		{"out of range last usage", map[string]any{"last_used_micro": SubscriptionShareTotalMicro + 1}, true},
		{"negative unattributed", map[string]any{"unattributed_micro": -1}, true},
		{"unknown generation state", map[string]any{"state": "future_state"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, pool := newSubscriptionShareUsageFixture(t)
			require.NoError(t, f.db.Model(&SubscriptionShareGeneration{}).Where("pool_id = ?", pool.ID).Updates(test.updates).Error)
			view, err := ReadSubscriptionSharePoolUsage(context.Background(), f.db, f.root.Id, pool.ID, 113)
			if test.wantError {
				require.ErrorIs(t, err, ErrSubscriptionShareUnavailable)
				assert.Nil(t, view)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "discontinuity", view.State)
			assert.Equal(t, "unsupported_generation_state", view.Reason)
		})
	}
}

func TestSubscriptionSharePoolUsageKeepsOrphanRequestCountsVisible(t *testing.T) {
	f, pool := newSubscriptionShareUsageFixture(t)
	require.NoError(t, AdmitSubscriptionShareAttempt(context.Background(), f.db, f.tokens[0].Id, f.owner.Id, f.subject, f.channels[0].Id, strings.Repeat("a", 64), "orphan-pending", fmt.Sprintf("%064x", 500), 115))
	// A malformed import can lose its allocation relationship. The all-pool
	// request still exists, and must not disappear from the Root observation.
	require.NoError(t, f.db.Model(&SubscriptionShareAttemptWindow{}).Where("attempt_id = ?", fmt.Sprintf("%064x", 500)).Update("allocation_id", 999999).Error)
	view, err := ReadSubscriptionSharePoolUsage(context.Background(), f.db, f.root.Id, pool.ID, 116)
	require.NoError(t, err)
	assert.Equal(t, "inconsistent", view.DecompositionState)
	assert.EqualValues(t, 1, view.PendingAttempts)
	assert.EqualValues(t, 1, view.UnsampledAttempts)
	assert.Zero(t, view.Allocations[0].PendingAttempts)
	assert.Nil(t, view.EstimatedUsedMicro)
}

func TestSubscriptionSharePoolUsageExpirationMatchesTokenAdmissionBoundary(t *testing.T) {
	f, pool := newSubscriptionShareUsageFixture(t)
	require.NoError(t, f.db.Model(&Token{}).Where("id = ?", f.tokens[0].Id).Update("expired_time", 113).Error)
	view, err := ReadSubscriptionSharePoolUsage(context.Background(), f.db, f.root.Id, pool.ID, 113)
	require.NoError(t, err)
	assert.Equal(t, "enabled", view.Allocations[0].TokenState)
	view, err = ReadSubscriptionSharePoolUsage(context.Background(), f.db, f.root.Id, pool.ID, 114)
	require.NoError(t, err)
	assert.Equal(t, "expired", view.Allocations[0].TokenState)
}
