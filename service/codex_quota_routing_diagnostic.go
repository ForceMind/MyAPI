package service

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
)

const (
	CodexRouteKeyEligible           = "eligible"
	CodexRouteKeyQuotaExhausted     = "quota_exhausted"
	CodexRouteKeyCredentialInvalid  = "credential_invalid"
	CodexRouteKeyCredentialDisabled = "credential_disabled"
)

// Diagnostic records contain only key indexes and normalized routing facts.
// Raw keys, provider account IDs and non-reversible identity references never
// leave this service boundary or appear in default JSON/debug output.
type CodexQuotaKeyDiagnostic struct {
	Index      int    `json:"-"`
	ReasonCode string `json:"-"`
	ResetAt    int64  `json:"-"`
	ObservedAt int64  `json:"-"`
	Source     string `json:"-"`
}

type CodexQuotaRoutingDiagnostic struct {
	ChannelID      int                       `json:"-"`
	ChannelEnabled bool                      `json:"-"`
	HasEligibleKey bool                      `json:"-"`
	RouteEligible  bool                      `json:"-"`
	AsOf           int64                     `json:"-"`
	Keys           []CodexQuotaKeyDiagnostic `json:"-"`
	excluded       map[int]bool
}

func (CodexQuotaRoutingDiagnostic) String() string {
	return "CodexQuotaRoutingDiagnostic{Private:[REDACTED]}"
}
func (d CodexQuotaRoutingDiagnostic) GoString() string { return d.String() }

// InspectCodexQuotaRouting uses exactly the same account eligibility decision
// as ordinary relay routing. It is DB-only: no upstream request, key rotation,
// polling cursor, channel status or quota marker is changed by a read.
func InspectCodexQuotaRouting(ctx context.Context, channel *model.Channel) (*CodexQuotaRoutingDiagnostic, error) {
	return inspectCodexQuotaRouting(ctx, channel, true)
}

func inspectCodexQuotaRouting(ctx context.Context, channel *model.Channel, includeDetails bool) (*CodexQuotaRoutingDiagnostic, error) {
	if channel == nil || channel.Type != constant.ChannelTypeCodex {
		return nil, ErrCodexQuotaRoutingUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var keyring *common.ChannelQuotaIdentityKeyring
	if os.Getenv(common.ChannelQuotaIdentityKeysEnv) != "" {
		loaded, err := common.LoadChannelQuotaIdentityKeyring()
		if err != nil {
			return nil, err
		}
		keyring = &loaded
	}
	routingNow, err := model.ReadDatabaseUnixTime(ctx, model.DB)
	if err != nil {
		return nil, err
	}
	if keyring == nil {
		required, err := model.HasVersionedCodexQuotaEvidence(ctx, model.DB)
		if err != nil {
			return nil, err
		}
		if required {
			return nil, ErrCodexQuotaRoutingUnavailable
		}
	}
	keys := []string{channel.Key}
	if channel.ChannelInfo.IsMultiKey {
		keys = channel.GetKeys()
	}
	diagnostic := &CodexQuotaRoutingDiagnostic{
		ChannelID: channel.Id, ChannelEnabled: channel.Status == common.ChannelStatusEnabled,
		AsOf:     routingNow,
		excluded: make(map[int]bool, len(keys)),
	}
	if includeDetails {
		diagnostic.Keys = make([]CodexQuotaKeyDiagnostic, 0, len(keys))
	}
	accountStates := make(map[string]model.CodexQuotaRouteState, len(keys))
	for index, key := range keys {
		item := CodexQuotaKeyDiagnostic{Index: index}
		if channel.ChannelInfo.IsMultiKey {
			if status, exists := channel.ChannelInfo.MultiKeyStatusList[index]; exists && status != common.ChannelStatusEnabled {
				item.ReasonCode = CodexRouteKeyCredentialDisabled
				if includeDetails {
					diagnostic.Keys = append(diagnostic.Keys, item)
				}
				continue
			}
		}
		var credential struct {
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
		}
		if err := common.UnmarshalJsonStr(strings.TrimSpace(key), &credential); err != nil ||
			strings.TrimSpace(credential.AccessToken) == "" || strings.TrimSpace(credential.AccountID) == "" {
			item.ReasonCode = CodexRouteKeyCredentialInvalid
			diagnostic.excluded[index] = true
			if includeDetails {
				diagnostic.Keys = append(diagnostic.Keys, item)
			}
			continue
		}
		accountID := strings.TrimSpace(credential.AccountID)
		state, known := accountStates[accountID]
		if !known {
			subjectRef := ""
			if keyring != nil {
				identity, found, err := model.LookupChannelQuotaIdentity(ctx, model.DB, *keyring,
					"channel_type_"+strconv.Itoa(channel.Type), common.ChannelQuotaIdentityKindProviderAccount, []byte(accountID))
				if err != nil {
					return nil, err
				}
				if found {
					subjectRef = identity.SubjectRef
				}
			}
			legacyRef := model.ChannelQuotaAccountRef("codex", accountID)
			state, err = model.ReadCodexQuotaRouteState(ctx, model.DB, subjectRef, legacyRef, routingNow)
			if err != nil {
				return nil, err
			}
			accountStates[accountID] = state
		}
		if state.Blocked {
			item.ReasonCode = CodexRouteKeyQuotaExhausted
			item.ResetAt, item.ObservedAt, item.Source = state.ResetAt, state.ObservedAt, state.Source
			diagnostic.excluded[index] = true
		} else {
			item.ReasonCode = CodexRouteKeyEligible
			diagnostic.HasEligibleKey = true
		}
		if includeDetails {
			diagnostic.Keys = append(diagnostic.Keys, item)
		}
	}
	diagnostic.RouteEligible = diagnostic.ChannelEnabled && diagnostic.HasEligibleKey
	return diagnostic, nil
}
