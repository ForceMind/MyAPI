package service

import (
	"context"
	"strings"

	"github.com/ForceMind/MyAPI/model"
)

const (
	EditionConfigurationCompatible   = "compatible"
	EditionConfigurationBlocked      = "blocked"
	EditionConfigurationUnverifiable = "unverifiable"
)

// EditionCompatibilityReport describes configuration compatibility, never a
// successful login or permission to change a running deployment.
type EditionCompatibilityReport struct {
	SchemaVersion        int                              `json:"schema_version"`
	Check                string                           `json:"check"`
	ReadOnly             bool                             `json:"read_only"`
	TargetEdition        *string                          `json:"target_edition"`
	SourceEdition        *string                          `json:"source_edition"`
	State                string                           `json:"state"`
	Scope                string                           `json:"scope"`
	LoginContinuity      string                           `json:"login_continuity"`
	RuntimeLoginVerified bool                             `json:"runtime_login_verified"`
	Facts                *model.EditionCompatibilityFacts `json:"facts"`
	Blockers             []string                         `json:"blockers"`
	Unknowns             []string                         `json:"unknowns"`
	Warnings             []string                         `json:"warnings"`
}

func (r EditionCompatibilityReport) ExitCode() int {
	switch r.State {
	case EditionConfigurationCompatible:
		return 0
	case EditionConfigurationBlocked:
		return 1
	default:
		return 2
	}
}

func compatibilityEdition(value string, allowDefault bool) *string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" && allowDefault {
		value = "full"
	}
	if value == "lan" {
		value = "lite"
	}
	if value != "full" && value != "lite" {
		return nil
	}
	return &value
}

func newEditionCompatibilityReport(source, target string) EditionCompatibilityReport {
	r := EditionCompatibilityReport{
		SchemaVersion: 1, Check: "edition-compatibility", ReadOnly: true,
		SourceEdition: compatibilityEdition(source, true), TargetEdition: compatibilityEdition(target, false),
		State: EditionConfigurationUnverifiable, Scope: "configuration", LoginContinuity: "not-verified",
		Blockers: []string{}, Unknowns: []string{}, Warnings: []string{"local-login-not-verified"},
	}
	if r.SourceEdition == nil {
		r.Unknowns = append(r.Unknowns, "invalid-source-edition")
	}
	if r.TargetEdition == nil {
		r.Unknowns = append(r.Unknowns, "invalid-target-edition")
	}
	return r
}

// CheckEditionCompatibility inspects only a Lite target. Full retains its
// existing startup and payment/authentication behavior without this reduction
// check opening a database or interpreting a historical source edition.
func CheckEditionCompatibility(ctx context.Context, source, target string) EditionCompatibilityReport {
	return checkEditionCompatibility(ctx, source, target, false)
}

// CheckEditionStartupConfiguration also recognizes an explicitly new anonymous
// memory database. That initialization is not an inspection of another running
// process's memory and is never accepted by the standalone readiness command.
func CheckEditionStartupConfiguration(ctx context.Context, source, target string) EditionCompatibilityReport {
	return checkEditionCompatibility(ctx, source, target, true)
}

func checkEditionCompatibility(ctx context.Context, source, target string, initializing bool) EditionCompatibilityReport {
	r := newEditionCompatibilityReport(source, target)
	if len(r.Unknowns) != 0 {
		return r
	}
	if *r.TargetEdition == "full" {
		r.State = EditionConfigurationCompatible
		r.Warnings = append(r.Warnings, "full-edition-no-feature-reduction")
		return r
	}
	facts, err := model.InspectEditionCompatibilityPreMigration(ctx)
	if initializing && *r.SourceEdition == "lite" && *r.TargetEdition == "lite" && err == nil && facts.SchemaVersion == 1 && facts.ReadOnly && facts.Complete && facts.Schema == "fresh" && facts.Source == "new-anonymous-memory" && facts.Database == "sqlite" {
		r.Facts, r.Scope, r.State = &facts, "initialization", EditionConfigurationCompatible
		return r
	}
	return assessLiteEditionCompatibility(r, facts, err)
}

func assessLiteEditionCompatibility(r EditionCompatibilityReport, facts model.EditionCompatibilityFacts, inspectErr error) EditionCompatibilityReport {
	r.Facts = &facts
	if inspectErr != nil || !facts.Complete || facts.SchemaVersion != 1 || !facts.ReadOnly || facts.LoginContinuity != "not-verified" {
		if facts.Schema == "partial" {
			r.Unknowns = append(r.Unknowns, "schema-unverifiable")
		} else {
			r.Unknowns = append(r.Unknowns, "database-unavailable")
		}
		return r
	}
	if facts.Schema == "fresh" {
		if facts.Source != "configured-primary" {
			r.Unknowns = append(r.Unknowns, "database-unavailable")
			return r
		}
		r.State = EditionConfigurationCompatible
		return r
	}
	if facts.Schema != "existing" || facts.TopUps == nil || facts.SubscriptionOrders == nil || facts.Authentication == nil {
		r.Unknowns = append(r.Unknowns, "schema-unverifiable")
		return r
	}
	for _, order := range []*model.EditionOrderFacts{facts.TopUps, facts.SubscriptionOrders} {
		if order.Total < 0 || order.Pending < 0 || order.Terminal < 0 || order.Unknown < 0 ||
			order.Terminal > order.Total || order.Pending > order.Total-order.Terminal || order.Unknown != order.Total-order.Terminal-order.Pending {
			r.Unknowns = append(r.Unknowns, "order-status-unverifiable")
			return r
		}
	}
	if facts.TopUps.Pending > 0 {
		r.Blockers = append(r.Blockers, "pending-top-ups")
	}
	if facts.SubscriptionOrders.Pending > 0 {
		r.Blockers = append(r.Blockers, "pending-subscription-orders")
	}
	if facts.TopUps.Unknown > 0 || facts.SubscriptionOrders.Unknown > 0 {
		r.Unknowns = append(r.Unknowns, "order-status-unverifiable")
	}
	a := facts.Authentication
	if a.PasswordLoginEnabled == nil || a.PasskeyEnabled == nil ||
		a.EnabledUsers < 0 || a.EnabledRootUsers < 0 || a.PasswordCandidates < 0 || a.PasskeyCandidates < 0 ||
		a.LocalCandidates < 0 || a.RootLocalCandidates < 0 || a.MissingLocalCandidates < 0 ||
		a.UnknownUsers < 0 || a.UnknownCredentialRecords < 0 || a.EnabledRootUsers > a.EnabledUsers ||
		a.LocalCandidates > a.EnabledUsers || a.RootLocalCandidates > a.EnabledRootUsers ||
		a.RootLocalCandidates > a.LocalCandidates || a.PasswordCandidates > a.LocalCandidates || a.PasskeyCandidates > a.LocalCandidates ||
		a.UnknownUsers > 0 || a.UnknownCredentialRecords > 0 || a.MissingLocalCandidates != a.EnabledUsers-a.LocalCandidates {
		r.Unknowns = append(r.Unknowns, "authentication-unverifiable")
	} else if a.MissingLocalCandidates > 0 {
		r.Blockers = append(r.Blockers, "missing-local-login")
	}
	if len(r.Unknowns) != 0 {
		return r
	}
	if len(r.Blockers) != 0 {
		r.State = EditionConfigurationBlocked
	} else {
		r.State = EditionConfigurationCompatible
	}
	return r
}
