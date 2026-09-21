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
	// Known records whether the stable ID exists in the current registry. It is
	// internal policy state and is intentionally omitted from API metadata.
	Known bool `json:"-"`
}

// AccountTierMetadata gives the legacy user group a separate account-level
// meaning. Account tiers and key access profiles intentionally have different
// identities even when old installations use the same names (default/vip).
type AccountTierMetadata struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Label          string   `json:"label"`
	Description    string   `json:"description"`
	RouteGroups    []string `json:"route_groups,omitempty"`
	ModelAllowlist []string `json:"model_allowlist,omitempty"`
	Enabled        bool     `json:"enabled"`
	Known          bool     `json:"-"`
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

// prepareAccessProfileIdentifiers runs before AutoMigrate. Old tables must
// receive nullable columns without the model's standard default: otherwise
// legacy rows become indistinguishable from explicitly selected standard IDs.
// DDL is intentionally outside the backfill transaction (MySQL implicitly
// commits DDL). A failed/interrupted startup leaves NULLs that the next run can
// safely fill, including when only one of the two columns has been added.
func prepareAccessProfileIdentifiers() error {
	for _, column := range []struct {
		table, name string
		model       interface{}
	}{
		{"users", "account_tier_id", &struct {
			AccountTierID string `gorm:"column:account_tier_id;type:varchar(64)"`
		}{}},
		{"tokens", "access_profile_id", &struct {
			AccessProfileID string `gorm:"column:access_profile_id;type:varchar(64)"`
		}{}},
	} {
		migrator := DB.Table(column.table).Migrator()
		// MySQL's base GORM HasColumn dereferences the parsed model schema;
		// a bare table-name string does not provide one.
		if !migrator.HasTable(column.model) || migrator.HasColumn(column.model, column.name) {
			continue
		}
		if err := migrator.AddColumn(column.model, column.name); err != nil {
			return err
		}
	}
	return MigrateAccessProfileIdentifiers()
}

// MigrateAccessProfileIdentifiers only fills missing identities. Non-empty
// values, including an explicit standard ID for a legacy vip group, survive
// every restart. This migration does not change the legacy routing contract.
func MigrateAccessProfileIdentifiers() error {
	if DB == nil {
		return nil
	}
	groupColumn := commonGroupCol
	if strings.TrimSpace(groupColumn) == "" {
		groupColumn = DB.Statement.Quote("group")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		const batchSize = 500
		var users []struct {
			Id            int
			LegacyGroup   string `gorm:"column:legacy_group"`
			AccountTierID string `gorm:"column:account_tier_id"`
		}
		if tx.Migrator().HasTable(&User{}) {
			selectColumns := "id, account_tier_id"
			if tx.Migrator().HasColumn(&User{}, "Group") {
				selectColumns = "id, " + groupColumn + " AS legacy_group, account_tier_id"
			}
			if err := tx.Unscoped().Model(&User{}).
				Select(selectColumns).
				Where("account_tier_id IS NULL OR TRIM(account_tier_id) = ''").
				FindInBatches(&users, batchSize, func(batchTx *gorm.DB, _ int) error {
					for _, user := range users {
						want := EffectiveAccountTierID(user.LegacyGroup)
						if strings.TrimSpace(user.AccountTierID) != "" {
							continue
						}
						if err := tx.Unscoped().Model(&User{}).Where("id = ?", user.Id).
							Where("account_tier_id IS NULL OR TRIM(account_tier_id) = ''").Update("account_tier_id", want).Error; err != nil {
							return err
						}
					}
					return nil
				}).Error; err != nil {
				return err
			}
		}

		var tokens []struct {
			Id              int
			LegacyGroup     string `gorm:"column:legacy_group"`
			AccessProfileID string `gorm:"column:access_profile_id"`
		}
		if tx.Migrator().HasTable(&Token{}) {
			selectColumns := "id, access_profile_id"
			if tx.Migrator().HasColumn(&Token{}, "Group") {
				selectColumns = "id, " + groupColumn + " AS legacy_group, access_profile_id"
			}
			if err := tx.Unscoped().Model(&Token{}).
				Select(selectColumns).
				Where("access_profile_id IS NULL OR TRIM(access_profile_id) = ''").
				FindInBatches(&tokens, batchSize, func(batchTx *gorm.DB, _ int) error {
					for _, token := range tokens {
						want := EffectiveAccessProfileID(token.LegacyGroup)
						if strings.TrimSpace(token.AccessProfileID) != "" {
							continue
						}
						if err := tx.Unscoped().Model(&Token{}).Where("id = ?", token.Id).
							Where("access_profile_id IS NULL OR TRIM(access_profile_id) = ''").Update("access_profile_id", want).Error; err != nil {
							return err
						}
					}
					return nil
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func ResolveAccessProfile(groupName, configuredDescription string) AccessProfileMetadata {
	return resolveAccessProfile(groupName, configuredDescription, setting.GetAccessProfileSetting())
}

// ResolveAccessProfileWithSnapshot resolves a key policy against one detached
// registry generation. Callers that need to evaluate a tier and profile for a
// single request must pass the same snapshot to both resolvers.
func ResolveAccessProfileWithSnapshot(groupName, configuredDescription string, registry *setting.AccessProfileSetting) AccessProfileMetadata {
	return resolveAccessProfile(groupName, configuredDescription, registry)
}

func resolveAccessProfile(groupName, configuredDescription string, registry *setting.AccessProfileSetting) AccessProfileMetadata {
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
	if registry != nil {
		configured, ok := registry.Profiles[profile.ID]
		if !ok {
			return profile
		}
		profile.Known = true
		if strings.TrimSpace(configured.Label) != "" {
			profile.Label = configured.Label
		}
		if strings.TrimSpace(configured.Description) != "" {
			profile.Description = configured.Description
		}
		// Preserve the configured list presence: an explicitly empty list
		// means "deny everything" under D04 and must not collapse into nil.
		profile.RouteGroups = appendStringList(configured.RouteGroups)
		profile.ModelAllowlist = appendStringList(configured.ModelAllowlist)
		profile.FallbackProfiles = appendStringList(configured.FallbackProfiles)
		if configured.Enabled != nil {
			profile.Enabled = *configured.Enabled
		}
	}
	return profile
}

func appendStringList(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

// ResolveAccessProfileID resolves an explicitly persisted profile identity.
// Stable built-in IDs are mapped through their legacy groups so configured
// labels and policy metadata remain consistent; unknown IDs remain readable as
// custom profiles during the compatibility period.
func ResolveAccessProfileID(profileID, fallbackGroup, configuredDescription string) AccessProfileMetadata {
	return ResolveAccessProfileIDWithSnapshot(profileID, fallbackGroup, configuredDescription, setting.GetAccessProfileSetting())
}

// ResolveAccessProfileIDWithSnapshot resolves an explicit stable identity
// without reading a second registry generation.
func ResolveAccessProfileIDWithSnapshot(profileID, fallbackGroup, configuredDescription string, registry *setting.AccessProfileSetting) AccessProfileMetadata {
	id := strings.TrimSpace(profileID)
	if id == "" {
		return resolveAccessProfile(fallbackGroup, configuredDescription, registry)
	}
	switch id {
	case "standard":
		return resolveAccessProfile("default", configuredDescription, registry)
	case "priority":
		return resolveAccessProfile("vip", configuredDescription, registry)
	case "automatic":
		return resolveAccessProfile("auto", configuredDescription, registry)
	default:
		return resolveAccessProfile(id, configuredDescription, registry)
	}
}

func ResolveAccountTier(groupName, configuredDescription string) AccountTierMetadata {
	return resolveAccountTier(groupName, configuredDescription, setting.GetAccessProfileSetting())
}

// ResolveAccountTierWithSnapshot resolves the account half of a request
// policy against the caller-provided detached registry generation.
func ResolveAccountTierWithSnapshot(groupName, configuredDescription string, registry *setting.AccessProfileSetting) AccountTierMetadata {
	return resolveAccountTier(groupName, configuredDescription, registry)
}

func resolveAccountTier(groupName, configuredDescription string, registry *setting.AccessProfileSetting) AccountTierMetadata {
	tier := AccountTierMetadata{
		ID:          groupName,
		Kind:        "custom",
		Label:       groupName,
		Description: configuredDescription,
		Enabled:     true,
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
	if registry != nil {
		configured, ok := registry.AccountTiers[tier.ID]
		if !ok {
			return tier
		}
		tier.Known = true
		if strings.TrimSpace(configured.Label) != "" {
			tier.Label = configured.Label
		}
		if strings.TrimSpace(configured.Description) != "" {
			tier.Description = configured.Description
		}
		tier.RouteGroups = appendStringList(configured.RouteGroups)
		tier.ModelAllowlist = appendStringList(configured.ModelAllowlist)
		if configured.Enabled != nil {
			tier.Enabled = *configured.Enabled
		}
	}
	return tier
}

// ResolveAccountTierID resolves a persisted stable account-tier identifier
// without inferring it from the legacy routing group. Built-in identifiers are
// mapped to their canonical labels; custom identifiers remain visible as
// custom tiers so administrators can explain them consistently across APIs.
func ResolveAccountTierID(id, configuredDescription string) AccountTierMetadata {
	return ResolveAccountTierIDWithSnapshot(id, configuredDescription, setting.GetAccessProfileSetting())
}

// ResolveAccountTierIDWithSnapshot resolves a persisted tier identity against
// one detached registry generation.
func ResolveAccountTierIDWithSnapshot(id, configuredDescription string, registry *setting.AccessProfileSetting) AccountTierMetadata {
	id = strings.TrimSpace(id)
	switch id {
	case "", "standard":
		return resolveAccountTier("default", configuredDescription, registry)
	case "priority":
		return resolveAccountTier("vip", configuredDescription, registry)
	default:
		return resolveAccountTier(id, configuredDescription, registry)
	}
}
