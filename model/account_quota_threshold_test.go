package model

import (
	"context"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func thresholdSnapshot(account, sample, source string, at int64, used float64) ChannelQuotaSnapshot {
	total := float64(100)
	return ChannelQuotaSnapshot{ChannelId: 7, AccountRef: ChannelQuotaAccountRef("codex", account), SampleID: sample, ObservedAt: at, Used: &used, Available: 100 - used, Total: &total, Unit: "percent", MetricType: "codex_rate_limit", Source: source, WindowType: "five_hour", WindowSeconds: 18000, ResetAt: at + 18000, Status: "success", CodexThresholdQualified: true}
}

func TestAccountQuotaThresholdUsesOneFreshCompleteAccountBatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func([]ChannelQuotaSnapshot) []ChannelQuotaSnapshot
		want   bool
		reason string
	}{
		{"healthy", func(rows []ChannelQuotaSnapshot) []ChannelQuotaSnapshot { return rows }, true, "eligible"},
		{"another window below floor", func(rows []ChannelQuotaSnapshot) []ChannelQuotaSnapshot {
			rows[1].Used = common.GetPointer(float64(85))
			rows[1].Available = 15
			return rows
		}, false, "threshold_reached"},
		{"boundary is not above threshold", func(rows []ChannelQuotaSnapshot) []ChannelQuotaSnapshot {
			rows[0].Used = common.GetPointer(float64(80))
			rows[0].Available = 20
			return rows
		}, false, "threshold_reached"},
		{"expired evidence", func(rows []ChannelQuotaSnapshot) []ChannelQuotaSnapshot {
			for i := range rows {
				rows[i].ObservedAt = 600
			}
			return rows
		}, false, "threshold_stale"},
		{"future observation", func(rows []ChannelQuotaSnapshot) []ChannelQuotaSnapshot {
			for i := range rows {
				rows[i].ObservedAt = 1001
			}
			return rows
		}, false, "threshold_stale"},
		{"reset requires a new sample", func(rows []ChannelQuotaSnapshot) []ChannelQuotaSnapshot { rows[0].ResetAt = 1000; return rows }, false, "threshold_reset_or_unknown"},
		{"legacy is not complete evidence", func(rows []ChannelQuotaSnapshot) []ChannelQuotaSnapshot {
			rows[1].CodexThresholdQualified = false
			return rows
		}, false, "threshold_unavailable"},
		{"unknown latest result", func(rows []ChannelQuotaSnapshot) []ChannelQuotaSnapshot {
			return append(rows, ChannelQuotaSnapshot{ChannelId: 7, AccountRef: rows[0].AccountRef, ObservedAt: 1000, SampleID: "failed", Unit: "percent", MetricType: "codex_rate_limit", Source: "codex_wham_usage", Status: "error"})
		}, false, "threshold_unavailable"},
		{"newer partial batch cannot borrow an old window", func(rows []ChannelQuotaSnapshot) []ChannelQuotaSnapshot {
			next := rows[0]
			next.ObservedAt = 1000
			next.SampleID = "partial"
			next.CodexThresholdQualified = false
			return append(rows, next)
		}, false, "threshold_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTokenBudgetDB(t)
			require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
			rows := tc.change([]ChannelQuotaSnapshot{thresholdSnapshot("account-a", "current", codexQuotaWindowPrimarySource, 990, 10), thresholdSnapshot("account-a", "current", codexQuotaWindowSecondarySource, 990, 20)})
			require.NoError(t, db.Create(&rows).Error)
			other := thresholdSnapshot("account-b", "other", codexQuotaWindowPrimarySource, 1000, 100)
			require.NoError(t, db.Create(&other).Error)
			state, err := ReadCodexAccountThresholdState(context.Background(), db, "", ChannelQuotaAccountRef("codex", "account-a"), 1000, common.AccountQuotaThreshold{MinimumRemainingBPS: 2000, MaxAgeSeconds: 300})
			require.NoError(t, err)
			assert.Equal(t, tc.want, state.Eligible)
			assert.Equal(t, tc.reason, state.Reason)
		})
	}
}

func TestAccountQuotaThresholdPolicyIsIndependentAndAudited(t *testing.T) {
	db := setupTokenBudgetDB(t)
	ctx := context.Background()
	input := TokenBudgetPolicyInput{ID: strings.Repeat("e", 64), TokenID: 11, AccountThreshold: &AccountQuotaThresholdPolicyInput{Enabled: true, MinimumRemainingBPS: 0, MaxAgeSeconds: 300}}
	_, err := ConfigureTokenBudget(ctx, db, 2, input)
	require.Error(t, err)
	state, err := ConfigureTokenBudget(ctx, db, 1, input)
	require.NoError(t, err)
	assert.Zero(t, state.AccountMinRemainingBPS, "explicit zero must not turn into the recommended default")
	_, err = ConfigureTokenBudget(ctx, db, 1, input)
	require.NoError(t, err)
	input.ID, input.ExpectedRevision, input.Enabled, input.Limit = strings.Repeat("f", 64), state.Revision, true, 100
	_, err = ConfigureTokenBudget(ctx, db, 1, input)
	require.ErrorIs(t, err, ErrAccountQuotaThresholdCombination)
	input.Enabled = false
	input.Fee = &FeeBudgetPolicyInput{Enabled: true, LimitUSD: "1"}
	_, err = ConfigureTokenBudget(ctx, db, 1, input)
	require.ErrorIs(t, err, ErrAccountQuotaThresholdCombination)
	var audits int64
	require.NoError(t, db.Model(&TokenBudgetPolicyChange{}).Count(&audits).Error)
	assert.EqualValues(t, 1, audits)
	input.Fee = nil
	input.AccountThreshold.MaxAgeSeconds = 0
	_, err = ConfigureTokenBudget(ctx, db, 1, input)
	require.ErrorIs(t, err, ErrAccountQuotaThresholdInvalid)
}

func accountQuotaThresholdConfiguredContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.NoError(t, db.Create(&Token{Id: 42, UserId: 2, Key: "threshold-db-fixture", Status: common.TokenStatusEnabled, ExpiredTime: -1}).Error)
	input := TokenBudgetPolicyInput{ID: strings.Repeat("a", 63) + "4", TokenID: 42, AccountThreshold: &AccountQuotaThresholdPolicyInput{Enabled: true, MinimumRemainingBPS: 2001, MaxAgeSeconds: 120}}
	policy, err := ConfigureTokenBudget(context.Background(), db, 1, input)
	require.NoError(t, err)
	require.True(t, policy.AccountThresholdEnabled)
	require.Equal(t, 2001, policy.AccountMinRemainingBPS)
	require.EqualValues(t, 120, policy.AccountMaxAgeSeconds)
	replay, err := ConfigureTokenBudget(context.Background(), db, 1, input)
	require.NoError(t, err)
	require.Equal(t, policy.Revision, replay.Revision)
	input.ID = strings.Repeat("b", 63) + "4"
	_, err = ConfigureTokenBudget(context.Background(), db, 1, input)
	require.ErrorIs(t, err, ErrTokenBudgetConflict)
	view, err := ReadTokenBudget(context.Background(), db, 2, 42)
	require.NoError(t, err)
	require.Equal(t, 2001, view.Policy.AccountMinRemainingBPS)
	row := thresholdSnapshot("threshold-decimal", "threshold-db", codexQuotaWindowPrimarySource, 1000, 99.99)
	require.NoError(t, db.Create(&row).Error)
	state, err := ReadCodexAccountThresholdState(context.Background(), db, "", row.AccountRef, 1001, common.AccountQuotaThreshold{MinimumRemainingBPS: 1, MaxAgeSeconds: 300})
	require.NoError(t, err)
	assert.False(t, state.Eligible, "reported 99.99% used is not more than 0.01% remaining")
	newer := thresholdSnapshot("threshold-decimal", "threshold-db-new", codexQuotaWindowPrimarySource, 1002, 0)
	require.NoError(t, db.Create(&newer).Error)
	state, err = ReadCodexAccountThresholdState(context.Background(), db, "", row.AccountRef, 1003, common.AccountQuotaThreshold{MinimumRemainingBPS: 2000, MaxAgeSeconds: 300})
	require.NoError(t, err)
	assert.True(t, state.Eligible)
	state, err = ReadCodexAccountThresholdState(context.Background(), db, "", row.AccountRef, newer.ResetAt, common.AccountQuotaThreshold{MinimumRemainingBPS: 2000, MaxAgeSeconds: 3600})
	require.NoError(t, err)
	assert.False(t, state.Eligible, "time passing is not a new full window observation")
}
