package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func editionCompatibilityFixture() model.EditionCompatibilityFacts {
	password, passkey := true, false
	return model.EditionCompatibilityFacts{
		SchemaVersion: 1, ReadOnly: true, Schema: "existing", Source: "configured-primary", Database: "sqlite", Complete: true, LoginContinuity: "not-verified",
		TopUps: &model.EditionOrderFacts{}, SubscriptionOrders: &model.EditionOrderFacts{},
		Authentication: &model.EditionAuthenticationFacts{
			PasswordLoginEnabled: &password, PasskeyEnabled: &passkey, PasswordLoginSource: "persisted", PasskeySource: "startup-default",
			EnabledUsers: 1, EnabledRootUsers: 1, PasswordCandidates: 1, LocalCandidates: 1, RootLocalCandidates: 1,
		},
	}
}

func TestEditionCompatibilityConfigurationDecision(t *testing.T) {
	cases := []struct {
		name     string
		change   func(*model.EditionCompatibilityFacts)
		state    string
		blockers []string
		unknowns []string
	}{
		{"configured password path", func(_ *model.EditionCompatibilityFacts) {}, "compatible", []string{}, []string{}},
		{"configured passkey path remains supported", func(f *model.EditionCompatibilityFacts) {
			disabled, enabled := false, true
			f.Authentication.PasswordLoginEnabled = &disabled
			f.Authentication.PasskeyEnabled = &enabled
			f.Authentication.PasswordCandidates = 0
			f.Authentication.PasskeyCandidates = 1
		}, "compatible", []string{}, []string{}},
		{"known terminal orders are not pending", func(f *model.EditionCompatibilityFacts) {
			f.TopUps.Total, f.TopUps.Terminal = 3, 3
			f.SubscriptionOrders.Total, f.SubscriptionOrders.Terminal = 2, 2
		}, "compatible", []string{}, []string{}},
		{"pending top up", func(f *model.EditionCompatibilityFacts) { f.TopUps.Total, f.TopUps.Pending = 1, 1 }, "blocked", []string{"pending-top-ups"}, []string{}},
		{"pending subscription purchase", func(f *model.EditionCompatibilityFacts) {
			f.SubscriptionOrders.Total, f.SubscriptionOrders.Pending = 1, 1
		}, "blocked", []string{"pending-subscription-orders"}, []string{}},
		{"unknown order status remains unknown", func(f *model.EditionCompatibilityFacts) { f.TopUps.Total, f.TopUps.Unknown = 1, 1 }, "unverifiable", []string{}, []string{"order-status-unverifiable"}},
		{"unknown does not hide a known pending order", func(f *model.EditionCompatibilityFacts) {
			f.TopUps.Total, f.TopUps.Pending = 1, 1
			f.SubscriptionOrders.Total, f.SubscriptionOrders.Unknown = 1, 1
		}, "unverifiable", []string{"pending-top-ups"}, []string{"order-status-unverifiable"}},
		{"only external account has no local path", func(f *model.EditionCompatibilityFacts) {
			f.Authentication.PasswordCandidates, f.Authentication.LocalCandidates, f.Authentication.RootLocalCandidates = 0, 0, 0
			f.Authentication.MissingLocalCandidates = 1
		}, "blocked", []string{"missing-local-login"}, []string{}},
		{"unknown password option", func(f *model.EditionCompatibilityFacts) {
			f.Authentication.PasswordLoginEnabled = nil
			f.Authentication.PasswordLoginSource = "invalid"
		}, "unverifiable", []string{}, []string{"authentication-unverifiable"}},
		{"unknown account facts", func(f *model.EditionCompatibilityFacts) { f.Authentication.UnknownUsers = 1 }, "unverifiable", []string{}, []string{"authentication-unverifiable"}},
		{"bad credential facts", func(f *model.EditionCompatibilityFacts) { f.Authentication.UnknownCredentialRecords = 1 }, "unverifiable", []string{}, []string{"authentication-unverifiable"}},
		{"missing schema section", func(f *model.EditionCompatibilityFacts) { f.Authentication = nil }, "unverifiable", []string{}, []string{"schema-unverifiable"}},
		{"partial schema cannot claim empty", func(f *model.EditionCompatibilityFacts) {
			f.Schema, f.Complete, f.TopUps, f.SubscriptionOrders, f.Authentication = "partial", false, nil, nil, nil
		}, "unverifiable", []string{}, []string{"schema-unverifiable"}},
		{"new database needs setup rather than old account evidence", func(f *model.EditionCompatibilityFacts) {
			f.Schema, f.TopUps, f.SubscriptionOrders, f.Authentication = "fresh", nil, nil, nil
		}, "compatible", []string{}, []string{}},
		{"new process memory is not evidence of an existing running database", func(f *model.EditionCompatibilityFacts) {
			f.Schema, f.Source, f.TopUps, f.SubscriptionOrders, f.Authentication = "fresh", "new-anonymous-memory", nil, nil, nil
		}, "unverifiable", []string{}, []string{"database-unavailable"}},
		{"non-readonly evidence rejected", func(f *model.EditionCompatibilityFacts) { f.ReadOnly = false }, "unverifiable", []string{}, []string{"database-unavailable"}},
		{"overlapping or inconsistent order counts rejected", func(f *model.EditionCompatibilityFacts) {
			f.TopUps.Total, f.TopUps.Pending, f.TopUps.Terminal = 1, 1, 1
		}, "unverifiable", []string{}, []string{"order-status-unverifiable"}},
		{"negative count rejected", func(f *model.EditionCompatibilityFacts) { f.TopUps.Total = -1 }, "unverifiable", []string{}, []string{"order-status-unverifiable"}},
		{"invented local candidate rejected", func(f *model.EditionCompatibilityFacts) { f.Authentication.LocalCandidates = 2 }, "unverifiable", []string{}, []string{"authentication-unverifiable"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts := editionCompatibilityFixture()
			tc.change(&facts)
			r := assessLiteEditionCompatibility(newEditionCompatibilityReport("full", "lite"), facts, nil)
			require.NotNil(t, r.Facts)
			assert.Equal(t, tc.state, r.State)
			assert.Equal(t, tc.blockers, r.Blockers)
			assert.Equal(t, tc.unknowns, r.Unknowns)
			assert.False(t, r.RuntimeLoginVerified)
			assert.Equal(t, "not-verified", r.LoginContinuity)
			assert.Equal(t, "configuration", r.Scope)
			assert.Equal(t, facts, *r.Facts)
			assert.Equal(t, map[string]int{"compatible": 0, "blocked": 1, "unverifiable": 2}[tc.state], r.ExitCode())
		})
	}
}

func TestEditionCompatibilityFullAndAliasContract(t *testing.T) {
	t.Setenv("SQL_DSN", "not-a-database-with-private-secret")
	r := CheckEditionCompatibility(context.Background(), "LAN", "full")
	assert.Equal(t, "compatible", r.State)
	require.NotNil(t, r.SourceEdition)
	assert.Equal(t, "lite", *r.SourceEdition)
	assert.Nil(t, r.Facts, "Full does not need the feature-reduction database inspection")
	assert.False(t, r.RuntimeLoginVerified)
	for _, tc := range []struct{ source, target, reason string }{{"not-an-edition", "lite", "invalid-source-edition"}, {"full", "not-an-edition", "invalid-target-edition"}, {"full", "", "invalid-target-edition"}} {
		r := CheckEditionCompatibility(context.Background(), tc.source, tc.target)
		assert.Equal(t, "unverifiable", r.State)
		assert.Contains(t, r.Unknowns, tc.reason)
		assert.Nil(t, r.Facts)
		assert.Equal(t, 2, r.ExitCode())
	}
}

func TestEditionCompatibilityUnavailableFactsNeverExposeDriverDetails(t *testing.T) {
	facts := model.EditionCompatibilityFacts{SchemaVersion: 1, ReadOnly: true, Schema: "unavailable", Source: "configured-primary", Database: "mysql", LoginContinuity: "not-verified"}
	r := assessLiteEditionCompatibility(newEditionCompatibilityReport("full", "lite"), facts, errors.New("mysql://private-password@private-host"))
	encoded, err := common.Marshal(r)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-password")
	assert.NotContains(t, string(encoded), "private-host")
	assert.Contains(t, string(encoded), `"top_ups":null`)
	assert.Contains(t, string(encoded), `"authentication":null`)
	assert.Equal(t, "unverifiable", r.State)
	assert.Equal(t, []string{"database-unavailable"}, r.Unknowns)
	assert.NotContains(t, strings.Join(r.Unknowns, ","), "mysql://")
}

func TestEditionCompatibilityAnonymousInitializationIsNotRunningDatabaseProof(t *testing.T) {
	t.Setenv("SQL_DSN", "local")
	oldPath := common.SQLitePath
	common.SQLitePath = ":memory:"
	t.Cleanup(func() { common.SQLitePath = oldPath })
	probe := CheckEditionCompatibility(context.Background(), "full", "lite")
	assert.Equal(t, "unverifiable", probe.State)
	assert.Equal(t, "configuration", probe.Scope)
	require.NotNil(t, probe.Facts)
	assert.Equal(t, "new-anonymous-memory", probe.Facts.Source)
	assert.Nil(t, probe.Facts.TopUps)
	assert.Nil(t, probe.Facts.Authentication)
	startup := CheckEditionStartupConfiguration(context.Background(), "lite", "lite")
	assert.Equal(t, "compatible", startup.State)
	assert.Equal(t, "initialization", startup.Scope)
	assert.False(t, startup.RuntimeLoginVerified)
	require.NotNil(t, startup.Facts)
	assert.Equal(t, "new-anonymous-memory", startup.Facts.Source)
	assert.Nil(t, startup.Facts.Authentication)
	cross := CheckEditionStartupConfiguration(context.Background(), "full", "lite")
	assert.Equal(t, "unverifiable", cross.State, "new memory cannot stand in for an existing Full source")
	assert.Equal(t, "configuration", cross.Scope)
}
