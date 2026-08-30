package model

// AccessProfileMetadata gives the legacy token group a stable domain meaning
// without changing the persisted Token schema or routing contract. The
// identifier remains presentation metadata until a future access_profile_id
// migration is explicitly approved.
type AccessProfileMetadata struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// AccountTierMetadata gives the legacy user group a separate account-level
// meaning. Account tiers and key access profiles intentionally have different
// identities even when old installations use the same names (default/vip).
type AccountTierMetadata struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

func ResolveAccessProfile(groupName, configuredDescription string) AccessProfileMetadata {
	profile := AccessProfileMetadata{
		ID:          groupName,
		Kind:        "custom",
		Label:       groupName,
		Description: configuredDescription,
	}
	switch groupName {
	case "", "default":
		profile.ID = "standard"
		profile.Kind = "standard"
		profile.Label = "Standard access"
		profile.Description = "Uses the standard channel pool and billing rules."
	case "vip":
		profile.ID = "priority"
		profile.Kind = "priority"
		profile.Label = "Priority access"
		profile.Description = "Uses the priority channel pool when your account allows it."
	case "auto":
		profile.ID = "automatic"
		profile.Kind = "automatic"
		profile.Label = "Automatic routing"
		profile.Description = "Tries eligible channel groups in order and can fail over when enabled."
	}
	return profile
}

func ResolveAccountTier(groupName, configuredDescription string) AccountTierMetadata {
	tier := AccountTierMetadata{
		ID:          groupName,
		Kind:        "custom",
		Label:       groupName,
		Description: configuredDescription,
	}
	switch groupName {
	case "", "default":
		tier.ID = "standard"
		tier.Kind = "standard"
		tier.Label = "Standard account"
		tier.Description = "Controls account quota, channel eligibility, and available features."
	case "vip":
		tier.ID = "priority"
		tier.Kind = "priority"
		tier.Label = "Priority account"
		tier.Description = "Uses the priority account quota, channel eligibility, and feature rules."
	}
	return tier
}
