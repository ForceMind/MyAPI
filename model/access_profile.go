package model

import (
	"strings"

	"github.com/ForceMind/MyAPI/setting"
	"gorm.io/gorm"
)

// AccessProfileMetadata gives the legacy token group a stable domain meaning
// while retaining the legacy Token group and routing contract. The stable
// identifier is persisted additively; old clients and routing continue to use
// group until a future policy migration is explicitly approved.
type AccessProfileMetadata struct {
	ID               string   `json:"id"`
	Kind             string   `json:"kind"`
	Label            string   `json:"label"`
	Description      string   `json:"description"`
	RouteGroups      []string `json:"route_groups,omitempty"`
	ModelAllowlist   []string `json:"model_allowlist,omitempty"`
	FallbackProfiles []string `json:"fallback_profiles,omitempty"`
	Enabled          bool     `json:"enabled"`
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

// EffectiveAccessProfileID returns the stable identity for a token while
// keeping the legacy group as the source of truth during the compatibility
// period. An empty group is the standard profile.
func EffectiveAccessProfileID(groupName string) string {
	return ResolveAccessProfile(strings.TrimSpace(groupName), "").ID
}

// EffectiveAccountTierID returns the stable account-tier identity for a user.
// It deliberately does not influence channel routing; that remains the job of
// the token access profile and the legacy group compatibility path.
func EffectiveAccountTierID(groupName string) string {
	return ResolveAccountTier(strings.TrimSpace(groupName), "").ID
}

// MigrateAccessProfileIdentifiers backfills the additive identity columns on
// existing installations. It is idempotent and derives values from legacy
// group fields; no credentials or request data are touched.
func MigrateAccessProfileIdentifiers() error {
	if DB == nil {
		return nil
	}
	groupColumn := commonGroupCol
	if strings.TrimSpace(groupColumn) == "" {
		// Lightweight model tests may set DB directly without running InitDB.
		groupColumn = "`group`"
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		const batchSize = 500
		var users []struct {
			Id            int
			LegacyGroup   string `gorm:"column:legacy_group"`
			AccountTierID string `gorm:"column:account_tier_id"`
		}
		if err := tx.Model(&User{}).
			Select("id, "+groupColumn+" AS legacy_group, account_tier_id").
			FindInBatches(&users, batchSize, func(batchTx *gorm.DB, _ int) error {
				for _, user := range users {
					want := EffectiveAccountTierID(user.LegacyGroup)
					if strings.TrimSpace(user.AccountTierID) == want {
						continue
					}
					if err := tx.Model(&User{}).Where("id = ?", user.Id).Update("account_tier_id", want).Error; err != nil {
						return err
					}
				}
				return nil
			}).Error; err != nil {
			return err
		}

		var tokens []struct {
			Id              int
			LegacyGroup     string `gorm:"column:legacy_group"`
			AccessProfileID string `gorm:"column:access_profile_id"`
		}
		if err := tx.Model(&Token{}).
			Select("id, "+groupColumn+" AS legacy_group, access_profile_id").
			FindInBatches(&tokens, batchSize, func(batchTx *gorm.DB, _ int) error {
				for _, token := range tokens {
					want := EffectiveAccessProfileID(token.LegacyGroup)
					if strings.TrimSpace(token.AccessProfileID) == want {
						continue
					}
					if err := tx.Model(&Token{}).Where("id = ?", token.Id).Update("access_profile_id", want).Error; err != nil {
						return err
					}
				}
				return nil
			}).Error; err != nil {
			return err
		}
		return nil
	})
}

func ResolveAccessProfile(groupName, configuredDescription string) AccessProfileMetadata {
	profile := AccessProfileMetadata{
		ID:          groupName,
		Kind:        "custom",
		Label:       groupName,
		Description: configuredDescription,
		Enabled:     true,
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
	if configured, ok := setting.GetAccessProfileDefinition(profile.ID); ok {
		if strings.TrimSpace(configured.Label) != "" {
			profile.Label = configured.Label
		}
		if strings.TrimSpace(configured.Description) != "" {
			profile.Description = configured.Description
		}
		profile.RouteGroups = append([]string(nil), configured.RouteGroups...)
		profile.ModelAllowlist = append([]string(nil), configured.ModelAllowlist...)
		profile.FallbackProfiles = append([]string(nil), configured.FallbackProfiles...)
		if configured.Enabled != nil {
			profile.Enabled = *configured.Enabled
		}
	}
	return profile
}

// ResolveAccessProfileID resolves an explicitly persisted profile identity.
// Stable built-in IDs are mapped through their legacy groups so configured
// labels and policy metadata remain consistent; unknown IDs remain readable as
// custom profiles during the compatibility period.
func ResolveAccessProfileID(profileID, fallbackGroup, configuredDescription string) AccessProfileMetadata {
	id := strings.TrimSpace(profileID)
	if id == "" {
		return ResolveAccessProfile(fallbackGroup, configuredDescription)
	}
	switch id {
	case "standard":
		return ResolveAccessProfile("default", configuredDescription)
	case "priority":
		return ResolveAccessProfile("vip", configuredDescription)
	case "automatic":
		return ResolveAccessProfile("auto", configuredDescription)
	default:
		return ResolveAccessProfile(id, configuredDescription)
	}
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

// ResolveAccountTierID resolves a persisted stable account-tier identifier
// without inferring it from the legacy routing group. Built-in identifiers are
// mapped to their canonical labels; custom identifiers remain visible as
// custom tiers so administrators can explain them consistently across APIs.
func ResolveAccountTierID(id, configuredDescription string) AccountTierMetadata {
	id = strings.TrimSpace(id)
	switch id {
	case "", "standard":
		return ResolveAccountTier("default", configuredDescription)
	case "priority":
		return ResolveAccountTier("vip", configuredDescription)
	default:
		return ResolveAccountTier(id, configuredDescription)
	}
}
