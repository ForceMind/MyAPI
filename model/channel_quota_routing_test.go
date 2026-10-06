package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func recordCodexRouteSnapshot(t *testing.T, db *gorm.DB, snapshot ChannelQuotaSnapshot) {
	t.Helper()
	if snapshot.MetricType == "" {
		snapshot.MetricType = "codex_rate_limit"
	}
	if snapshot.Unit == "" {
		snapshot.Unit = "percent"
	}
	if snapshot.WindowType == "" {
		snapshot.WindowType = "five_hour"
	}
	if snapshot.Status == "" {
		snapshot.Status = "success"
	}
	require.NoError(t, normalizeChannelQuotaSnapshot(&snapshot))
	require.NoError(t, db.Create(&snapshot).Error)
}

func TestCodexQuotaRoutingExcludesKnownExhaustedAccountUntilReset(t *testing.T) {
	db := openQuotaIdentityDB(t)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
	accountA := ChannelQuotaAccountRef("codex", "account-a")
	accountB := ChannelQuotaAccountRef("codex", "account-b")
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: accountA, ObservedAt: 100, Available: 0,
		Source: "codex_wham_usage_primary", ResetAt: 300,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 2, AccountRef: accountB, ObservedAt: 100, Available: 70,
		Source: "codex_wham_usage_primary", ResetAt: 300,
	})
	state, err := ReadCodexQuotaRouteState(context.Background(), db, "", accountA, 200)
	require.NoError(t, err)
	assert.True(t, state.Blocked)
	assert.Equal(t, int64(300), state.ResetAt)
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", accountB, 200)
	require.NoError(t, err)
	assert.False(t, state.Blocked)

	// A later failed poll cannot erase the last confirmed 100% observation.
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: accountA, ObservedAt: 101, Status: "error",
		Source: "codex_wham_usage_primary", ErrorCode: "upstream_http",
	})
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", accountA, 200)
	require.NoError(t, err)
	assert.True(t, state.Blocked)
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", accountA, 300)
	require.NoError(t, err)
	assert.False(t, state.Blocked)
}

func TestCodexQuotaRoutingKeepsZeroObservedAfterReportedReset(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		observedAt int64
		resetAt    int64
		blocked    bool
	}{
		{"old zero expires at reset", 100, 150, false},
		{"zero observed at reset stays blocked", 150, 150, true},
		{"zero observed after reset stays blocked", 180, 150, true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db := openQuotaIdentityDB(t)
			require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
			account := ChannelQuotaAccountRef("codex", "post-reset-zero")
			recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
				ChannelId: 1, AccountRef: account, ObservedAt: testCase.observedAt,
				Available: 0, Source: "codex_wham_usage_primary", ResetAt: testCase.resetAt,
			})
			state, err := ReadCodexQuotaRouteState(context.Background(), db, "", account, 200)
			require.NoError(t, err)
			assert.Equal(t, testCase.blocked, state.Blocked)
		})
	}
}

func TestCodexQuotaRoutingKeepsUsageLimitObservedAfterReportedReset(t *testing.T) {
	db := openQuotaIdentityDB(t)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
	account := ChannelQuotaAccountRef("codex", "post-reset-429")
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 180,
		Source: CodexQuotaRouteLimitSource, Status: "error", ErrorCode: CodexQuotaRouteLimitCode,
		ResetAt: 150,
	})
	state, err := ReadCodexQuotaRouteState(context.Background(), db, "", account, 200)
	require.NoError(t, err)
	assert.True(t, state.Blocked)
}

func TestCodexQuotaRoutingGenericMarkerCannotHideKnownUsageLimit(t *testing.T) {
	db := openQuotaIdentityDB(t)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
	account := ChannelQuotaAccountRef("codex", "exact-429-account")
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 100,
		Source: CodexQuotaRouteLimitSource, Status: "error", ErrorCode: CodexQuotaRouteLimitCode,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 110,
		Source: CodexQuotaRouteLimitSource, Status: "error", ErrorCode: "ordinary_rate_limit",
	})
	state, err := ReadCodexQuotaRouteState(context.Background(), db, "", account, 120)
	require.NoError(t, err)
	assert.True(t, state.Blocked, "a generic error is not evidence that the exact usage limit recovered")
	assert.Equal(t, CodexQuotaRouteLimitSource, state.Source)
}

func TestCodexQuotaRoutingUsesConfirmedIdentityAndEveryWindow(t *testing.T) {
	db := openQuotaIdentityDB(t)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
	subject, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	legacyA := ChannelQuotaAccountRef("codex", "account-a")
	legacyB := ChannelQuotaAccountRef("codex", "account-b")
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed,
		ObservedAt: 100, Available: 80, Source: "codex_wham_usage_primary", ResetAt: 300,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 2, AccountRef: legacyB, ObservedAt: 150, Available: 0,
		Source: "codex_wham_usage_secondary", WindowType: "weekly", ResetAt: 500,
	})
	state, err := ReadCodexQuotaRouteState(context.Background(), db, subject, legacyA, 200)
	require.NoError(t, err)
	assert.False(t, state.Blocked, "another legacy account must not poison the confirmed subject")

	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 3, SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed,
		ObservedAt: 160, Available: 0, Source: "codex_wham_usage_secondary", WindowType: "weekly", ResetAt: 500,
	})
	state, err = ReadCodexQuotaRouteState(context.Background(), db, subject, legacyA, 200)
	require.NoError(t, err)
	assert.True(t, state.Blocked)
	assert.Equal(t, int64(500), state.ResetAt)

	// Credential-scoped observations do not assert confirmed account capacity.
	otherSubject, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 4, SubjectRef: otherSubject, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped,
		ObservedAt: 170, Available: 0, Source: "codex_wham_usage_primary", ResetAt: 600,
	})
	state, err = ReadCodexQuotaRouteState(context.Background(), db, otherSubject, "", 200)
	require.NoError(t, err)
	assert.False(t, state.Blocked)
}

func TestCodexQuotaRoutingReopensOnNewHealthySampleAndExactMarkerExpiry(t *testing.T) {
	db := openQuotaIdentityDB(t)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
	account := ChannelQuotaAccountRef("codex", "account-a")
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 100, Available: 0,
		Source: "codex_wham_usage_primary", ResetAt: 300,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 110, Available: 65,
		Source: "codex_wham_usage_primary", ResetAt: 310,
	})
	state, err := ReadCodexQuotaRouteState(context.Background(), db, "", account, 200)
	require.NoError(t, err)
	assert.False(t, state.Blocked)

	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 120, Status: "error",
		Source: CodexQuotaRouteLimitSource, ErrorCode: CodexQuotaRouteLimitCode, ResetAt: 240,
	})
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", account, 200)
	require.NoError(t, err)
	assert.True(t, state.Blocked)
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", account, 240)
	require.NoError(t, err)
	assert.False(t, state.Blocked)

	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 250, Status: "error",
		Source: CodexQuotaRouteLimitSource, ErrorCode: "ordinary_rate_limit", ResetAt: 350,
	})
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", account, 260)
	require.NoError(t, err)
	assert.False(t, state.Blocked, "generic 429 is not an exhausted-account proof")

	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 270, Status: "error",
		Source: CodexQuotaRouteLimitSource, ErrorCode: CodexQuotaRouteLimitCode, ResetAt: 350,
	})
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", account, 280)
	require.NoError(t, err)
	assert.True(t, state.Blocked)
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 2, AccountRef: account, ObservedAt: 275, Available: 65,
		Source: "codex_wham_usage_primary", ResetAt: 360,
	})
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", account, 280)
	require.NoError(t, err)
	assert.False(t, state.Blocked, "a later healthy provider observation clears the temporary 429 marker")
}

func TestCodexQuotaRoutingDoesNotTreatSameSecondHealthyWriteAsRecovery(t *testing.T) {
	db := openQuotaIdentityDB(t)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
	account := ChannelQuotaAccountRef("codex", "same-second-account")
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 100,
		Source: CodexQuotaRouteLimitSource, Status: "error", ErrorCode: CodexQuotaRouteLimitCode,
		ResetAt: 300,
	})
	// The sampler observed this capacity during the same second as the 429.
	// Its later database ID proves commit order, not observation order.
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 100, Available: 80,
		Source: "codex_wham_usage_primary", ResetAt: 400,
	})

	state, err := ReadCodexQuotaRouteState(context.Background(), db, "", account, 101)
	require.NoError(t, err)
	assert.True(t, state.Blocked)
	assert.Equal(t, CodexQuotaRouteLimitSource, state.Source)

	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 102, Available: 75,
		Source: "codex_wham_usage_primary", ResetAt: 400,
	})
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", account, 103)
	require.NoError(t, err)
	assert.False(t, state.Blocked)
}

func TestCodexQuotaRoutingNeedsFreshRecoveryForEveryObservedWindow(t *testing.T) {
	db := openQuotaIdentityDB(t)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
	account := ChannelQuotaAccountRef("codex", "two-window-account")
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 90, Available: 70,
		Source: "codex_wham_usage_secondary", WindowType: "weekly", ResetAt: 500,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 100, Status: "error",
		Source: CodexQuotaRouteLimitSource, ErrorCode: CodexQuotaRouteLimitCode,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 110, Available: 80,
		Source: "codex_wham_usage_primary", ResetAt: 300,
	})
	state, err := ReadCodexQuotaRouteState(context.Background(), db, "", account, 120)
	require.NoError(t, err)
	assert.True(t, state.Blocked, "one new healthy window cannot clear an unknown usage-limit marker while another observed window is stale")

	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 115, Available: 60,
		Source: "codex_wham_usage_secondary", WindowType: "weekly", ResetAt: 500,
	})
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", account, 120)
	require.NoError(t, err)
	assert.False(t, state.Blocked, "both observed windows now have post-429 positive capacity")
}

func TestCodexQuotaRoutingAcceptsOnlyPairedWindowAbsenceAsRecovery(t *testing.T) {
	db := openQuotaIdentityDB(t)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
	account := ChannelQuotaAccountRef("codex", "changed-plan-account")
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 90, Available: 60,
		Source: "codex_wham_usage_secondary", WindowType: "weekly", ResetAt: 500,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 100, Status: "error",
		Source: CodexQuotaRouteLimitSource, ErrorCode: CodexQuotaRouteLimitCode,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 110, SampleID: "fresh-provider-batch", Available: 80,
		Source: "codex_wham_usage_primary", ResetAt: 300,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 110, SampleID: "unpaired-batch", Status: "unsupported",
		Source: "codex_wham_usage_secondary", WindowType: "weekly", ErrorCode: "window_absent",
	})
	state, err := ReadCodexQuotaRouteState(context.Background(), db, "", account, 120)
	require.NoError(t, err)
	assert.True(t, state.Blocked, "a standalone absence marker cannot clear an account-level 429")

	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 111, SampleID: "paired-provider-batch", Available: 70,
		Source: "codex_wham_usage_primary", ResetAt: 320,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 111, SampleID: "paired-provider-batch", Status: "unsupported",
		Source: "codex_wham_usage_secondary", WindowType: "weekly", ErrorCode: "window_absent",
	})
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", account, 120)
	require.NoError(t, err)
	assert.False(t, state.Blocked, "a paired fresh provider batch proves the old window no longer applies")
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 112, Status: "unsupported",
		Source: codexQuotaWindowSecondarySource, WindowType: "weekly", ErrorCode: "other_unsupported",
	})
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", account, 120)
	require.NoError(t, err)
	assert.False(t, state.Blocked, "a generic unsupported event cannot erase paired window-absence evidence")
}

func TestCodexQuotaRoutingPairedAbsenceClearsOldZeroAndUnknown429(t *testing.T) {
	db := openQuotaIdentityDB(t)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
	account := ChannelQuotaAccountRef("codex", "zero-window-plan-change")
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 90, Available: 0,
		Source: codexQuotaWindowSecondarySource, WindowType: "weekly", ResetAt: 0,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 100, Status: "error",
		Source: CodexQuotaRouteLimitSource, ErrorCode: CodexQuotaRouteLimitCode,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 105, SampleID: "unpaired-zero-window", Status: "unsupported",
		Source: codexQuotaWindowSecondarySource, WindowType: "weekly", ErrorCode: "window_absent",
	})
	state, err := ReadCodexQuotaRouteState(context.Background(), db, "", account, 120)
	require.NoError(t, err)
	assert.True(t, state.Blocked, "an unpaired absence cannot clear an exhausted window")
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 110, SampleID: "paired-zero-window", Available: 80,
		Source: codexQuotaWindowPrimarySource, ResetAt: 300,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 110, SampleID: "paired-zero-window", Status: "unsupported",
		Source: codexQuotaWindowSecondarySource, WindowType: "weekly", ErrorCode: "window_absent",
	})
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", account, 120)
	require.NoError(t, err)
	assert.False(t, state.Blocked, "paired provider evidence retires the old exhausted window and unknown 429")
}

func TestCodexQuotaRoutingDoesNotTrustFutureHealthyObservation(t *testing.T) {
	db := openQuotaIdentityDB(t)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
	account := ChannelQuotaAccountRef("codex", "future-sample-account")
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 100, Status: "error",
		Source: CodexQuotaRouteLimitSource, ErrorCode: CodexQuotaRouteLimitCode,
	})
	recordCodexRouteSnapshot(t, db, ChannelQuotaSnapshot{
		ChannelId: 1, AccountRef: account, ObservedAt: 110, Available: 80,
		Source: "codex_wham_usage_primary", ResetAt: 300,
	})
	state, err := ReadCodexQuotaRouteState(context.Background(), db, "", account, 105)
	require.NoError(t, err)
	assert.True(t, state.Blocked)
	state, err = ReadCodexQuotaRouteState(context.Background(), db, "", account, 111)
	require.NoError(t, err)
	assert.False(t, state.Blocked)
}

func TestCodexQuotaRoutingFailsClosedOnMissingIdentityOrDatabase(t *testing.T) {
	_, err := ReadCodexQuotaRouteState(context.Background(), nil, "", "", 200)
	require.Error(t, err)
	db := openQuotaIdentityDB(t)
	_, err = ReadCodexQuotaRouteState(context.Background(), db, "", "", 200)
	require.ErrorIs(t, err, ErrCodexQuotaRouteIdentity)
}
