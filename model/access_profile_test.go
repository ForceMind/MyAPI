package model

import "testing"

func TestResolveAccessProfileKeepsLegacyGroupsReadable(t *testing.T) {
	tests := []struct {
		legacy, id, kind, label string
	}{
		{legacy: "", id: "standard", kind: "standard", label: "Standard access"},
		{legacy: "default", id: "standard", kind: "standard", label: "Standard access"},
		{legacy: "vip", id: "priority", kind: "priority", label: "Priority access"},
		{legacy: "auto", id: "automatic", kind: "automatic", label: "Automatic routing"},
		{legacy: "team-a", id: "team-a", kind: "custom", label: "team-a"},
	}
	for _, testCase := range tests {
		profile := ResolveAccessProfile(testCase.legacy, "configured")
		if profile.ID != testCase.id || profile.Kind != testCase.kind || profile.Label != testCase.label {
			t.Fatalf("ResolveAccessProfile(%q) = %#v", testCase.legacy, profile)
		}
	}
	if got := ResolveAccessProfile("team-a", "configured").Description; got != "configured" {
		t.Fatalf("custom profile description = %q", got)
	}
}

func TestResolveAccountTierIsSeparateFromAccessProfile(t *testing.T) {
	tier := ResolveAccountTier("vip", "ignored for built-in tier")
	if tier.ID != "priority" || tier.Kind != "priority" || tier.Label != "Priority account" {
		t.Fatalf("vip account tier = %#v", tier)
	}
	profile := ResolveAccessProfile("vip", "")
	if profile.ID == tier.ID || profile.Label == tier.Label {
		t.Fatalf("account tier and access profile unexpectedly share identity: tier=%#v profile=%#v", tier, profile)
	}
}
