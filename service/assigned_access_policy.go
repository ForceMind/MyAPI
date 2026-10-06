package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/service/accesspolicy"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

var ErrAssignedAccessDenied = errors.New("assigned_access_denied")

// AssignedAccessPolicyView is a detached administrative view. A removal retains
// its revision tombstone, so an old editor cannot resurrect a removed policy.
type AssignedAccessPolicyView struct {
	Subject   string `json:"subject"`
	SubjectID int    `json:"subject_id"`
	Revision  int64  `json:"revision"`
	Assigned  bool   `json:"assigned"`
	accesspolicy.AssignedPolicy
}

type AssignedModelAccess struct {
	Model   string   `json:"model"`
	Allowed bool     `json:"allowed"`
	Reasons []string `json:"reasons"`
}

type AssignedAccessState struct {
	User  AssignedAccessPolicyView
	Token AssignedAccessPolicyView
}

// DecodeAssignedAccessPolicyJSON rejects ambiguous policy input before Go's
// case-insensitive struct decoder can apply last-wins or alias semantics.
// Request bodies and persisted envelopes share the portable TEXT-size bound.
func DecodeAssignedAccessPolicyJSON(raw []byte, value any) error {
	if len(raw) > model.MaxAssignedAccessPolicyJSONBytes {
		return model.ErrAssignedAccessPolicyInvalid
	}
	if _, err := common.CanonicalJSONObjectDigest(raw); err != nil {
		return model.ErrAssignedAccessPolicyInvalid
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(raw, &fields); err != nil {
		return model.ErrAssignedAccessPolicyInvalid
	}
	for key := range fields {
		switch key {
		case "enabled", "public_models", "upstream_models", "channel_ids", "expected_revision":
		default:
			return model.ErrAssignedAccessPolicyInvalid
		}
	}
	if err := common.DecodeJsonStrict(bytes.NewReader(raw), value); err != nil {
		return model.ErrAssignedAccessPolicyInvalid
	}
	return nil
}

func ReadAssignedAccessPolicy(ctx context.Context, subject string, id int) (AssignedAccessPolicyView, error) {
	row, err := model.ReadAssignedAccessPolicy(ctx, model.DB, subject, id)
	if err != nil {
		return AssignedAccessPolicyView{}, err
	}
	return assignedAccessPolicyView(row)
}

func assignedAccessPolicyView(row model.AssignedAccessPolicy) (AssignedAccessPolicyView, error) {
	view := AssignedAccessPolicyView{Subject: row.SubjectType, SubjectID: row.SubjectID, Revision: row.Revision, Assigned: row.Assigned}
	if !row.Assigned {
		view.Enabled = true
		return view, nil
	}
	// Invalid persisted data is an authorization failure, never inheritance.
	if err := DecodeAssignedAccessPolicyJSON([]byte(row.PolicyJSON), &view.AssignedPolicy); err != nil {
		return AssignedAccessPolicyView{}, ErrAssignedAccessDenied
	}
	policy, err := accesspolicy.NormalizeAssignedPolicy(view.AssignedPolicy)
	if err != nil {
		return AssignedAccessPolicyView{}, ErrAssignedAccessDenied
	}
	view.AssignedPolicy = policy
	return view, nil
}

func LoadAssignedAccessState(ctx context.Context, userID, tokenID int) (AssignedAccessState, error) {
	state := AssignedAccessState{
		User:  AssignedAccessPolicyView{Subject: "user", SubjectID: userID, AssignedPolicy: accesspolicy.AssignedPolicy{Enabled: true}},
		Token: AssignedAccessPolicyView{Subject: "token", SubjectID: tokenID, AssignedPolicy: accesspolicy.AssignedPolicy{Enabled: true}},
	}
	if userID <= 0 {
		return state, nil
	}
	// Both subject constraints come from one statement's snapshot. Two reads
	// could otherwise combine an old user grant with a newly widened Key.
	rows, err := model.ReadAssignedAccessPolicies(ctx, model.DB, userID, tokenID)
	if err != nil {
		return state, err
	}
	for _, row := range rows {
		view, err := assignedAccessPolicyView(row)
		if err != nil {
			return state, err
		}
		if row.SubjectType == "user" {
			state.User = view
		} else {
			state.Token = view
		}
	}
	return state, nil
}
func (state AssignedAccessState) Assigned() bool { return state.User.Assigned || state.Token.Assigned }
func (state AssignedAccessState) Reasons(publicModel, upstreamModel string, channelID int) []string {
	reasons := make([]string, 0)
	for _, view := range []AssignedAccessPolicyView{state.User, state.Token} {
		if view.Assigned {
			reasons = append(reasons, accesspolicy.EvaluateAssignedPolicy(view.AssignedPolicy, publicModel, upstreamModel, channelID)...)
		}
	}
	sort.Strings(reasons)
	return slices.Compact(reasons)
}

func AssignedAccessProtocolSupported(channelType int, path string) bool {
	return channelType == constant.ChannelTypeOpenAI && (path == "/v1/chat/completions" || path == "/v1/responses") || channelType == constant.ChannelTypeCodex && path == "/v1/responses"
}

// ValidateAssignedAccessSelection protects every selected channel, including
// affinity, explicit channel IDs, retries and fixed-key task paths. Diagnostic
// callers are excluded by the middleware entry point, not by client input.
func ValidateAssignedAccessSelection(c *gin.Context, channel *model.Channel, publicModel string) error {
	if c == nil || c.Request == nil || c.GetInt("id") <= 0 {
		return nil
	}
	state, err := LoadAssignedAccessState(c.Request.Context(), c.GetInt("id"), c.GetInt("token_id"))
	if err != nil {
		return err
	}
	if !state.Assigned() {
		return nil
	}
	if channel == nil || !AssignedAccessProtocolSupported(channel.Type, c.Request.URL.Path) {
		return ErrAssignedAccessDenied
	}
	route, err := model.ResolveChannelModelRoute(channel, publicModel, c.Request.URL.Path)
	if err != nil || len(state.Reasons(publicModel, route.UpstreamModel, channel.Id)) > 0 {
		return ErrAssignedAccessDenied
	}
	return nil
}

func AssignedAccessGroups(user *model.User, token *model.Token) ([]string, error) {
	if user == nil {
		return nil, ErrAssignedAccessDenied
	}
	if token == nil {
		groups := make([]string, 0)
		for group := range GetUserUsableGroups(user.Group) {
			if group != "auto" {
				groups = append(groups, group)
			}
		}
		sort.Strings(groups)
		return groups, nil
	}
	if token.Group == "auto" {
		if token.AutoGroups == "" {
			return GetUserAutoGroup(user.Group), nil
		}
		groups, err := token.GetAutoGroups()
		if err != nil {
			return nil, err
		}
		return FilterUserTokenAutoGroups(user.Group, groups), nil
	}
	group := token.Group
	if group == "" {
		group = user.Group
	}
	if !GroupInUserUsableGroups(user.Group, group) {
		return []string{}, nil
	}
	return []string{group}, nil
}

func assignedLegacyPolicyAllows(user *model.User, token *model.Token, group, publicModel, upstreamModel string) bool {
	if token != nil && token.ModelLimitsEnabled {
		allowed := token.GetModelLimitsMap()
		for _, name := range []string{publicModel, upstreamModel} {
			if !allowed[ratio_setting.FormatMatchingModelName(name)] {
				return false
			}
		}
	}
	if !setting.AccessPolicyEnforcedForGroup(group) {
		return true
	}
	profileID, tokenGroup := "", group
	if token != nil {
		profileID, tokenGroup = token.AccessProfileID, token.Group
	}
	for _, name := range []string{publicModel, upstreamModel} {
		decision, err := accesspolicy.EvaluateRequest(accesspolicy.PolicyModeEnforce, accesspolicy.SnapshotSource{AccountTierID: user.AccountTierID, AccessProfileID: profileID, LegacyTokenGroup: tokenGroup, UsingGroup: group, UsingModel: name, LegacyAllowed: true}, 0)
		if err != nil || len(accesspolicy.BlockingFindings(decision)) > 0 {
			return false
		}
	}
	return true
}

// ValidateAssignedAccessDispatch is independent from failover. It reloads the
// authoritative user, Key, assignments and actual channel before new send;
// cached auth or a pre-mutation request context cannot grant assigned access.
// It cannot recall a request which an external provider has already accepted.
func ValidateAssignedAccessDispatch(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	if c == nil || c.Request == nil || info == nil || info.UserId <= 0 || info.IsChannelTest && info.TokenId == 0 {
		return nil
	}
	state, err := LoadAssignedAccessState(c.Request.Context(), info.UserId, info.TokenId)
	if err != nil {
		return err
	}
	if !state.Assigned() {
		return nil
	}
	if info.ChannelMeta == nil || !AssignedAccessProtocolSupported(info.ChannelType, c.Request.URL.Path) {
		return ErrAssignedAccessDenied
	}
	user, err := model.GetUserById(info.UserId, false)
	if err != nil || user.Status != common.UserStatusEnabled {
		return ErrAssignedAccessDenied
	}
	var token *model.Token
	if info.TokenId > 0 {
		token, err = model.GetTokenById(info.TokenId)
		if err != nil || token.UserId != user.Id || token.Key != info.TokenKey || token.Status != common.TokenStatusEnabled || token.ExpiredTime != -1 && token.ExpiredTime <= time.Now().Unix() {
			return ErrAssignedAccessDenied
		}
		if limits := token.GetIpLimits(); len(limits) > 0 && !common.IsIpInCIDRList(net.ParseIP(c.ClientIP()), limits) {
			return ErrAssignedAccessDenied
		}
	}
	groups, err := AssignedAccessGroups(user, token)
	if err != nil {
		return err
	}
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if selected := common.GetContextKeyString(c, constant.ContextKeyAutoGroup); selected != "" {
		group = selected
	}
	if !slices.Contains(groups, group) {
		return ErrAssignedAccessDenied
	}
	channel, err := model.GetChannelById(info.ChannelId, true)
	if err != nil || channel.Status != common.ChannelStatusEnabled || channel.Type != info.ChannelType || !slices.Contains(channel.GetGroups(), group) || !slices.Contains(channel.GetModels(), info.OriginModelName) && !slices.Contains(channel.GetModels(), ratio_setting.FormatMatchingModelName(info.OriginModelName)) {
		return ErrAssignedAccessDenied
	}
	var abilityCount int64
	if err := model.DB.WithContext(c.Request.Context()).Model(&model.Ability{}).Where(map[string]any{"group": group, "channel_id": channel.Id, "enabled": true}).Where("model IN ?", []string{info.OriginModelName, ratio_setting.FormatMatchingModelName(info.OriginModelName)}).Count(&abilityCount).Error; err != nil || abilityCount == 0 {
		return ErrAssignedAccessDenied
	}
	if channel.GetBaseURL() != info.ChannelBaseUrl {
		return ErrAssignedAccessDenied
	}
	credentialEnabled := false
	if channel.ChannelInfo.IsMultiKey {
		for index, key := range channel.GetKeys() {
			status, exists := channel.ChannelInfo.MultiKeyStatusList[index]
			if key == info.ApiKey && (!exists || status == common.ChannelStatusEnabled) {
				credentialEnabled = true
				break
			}
		}
	} else {
		credentialEnabled = channel.Key != "" && channel.Key == info.ApiKey
	}
	if !credentialEnabled {
		return ErrAssignedAccessDenied
	}
	route, err := model.ResolveChannelModelRoute(channel, info.OriginModelName, c.Request.URL.Path)
	if err != nil || route.UpstreamModel != info.UpstreamModelName {
		return ErrAssignedAccessDenied
	}
	if len(state.Reasons(info.OriginModelName, route.UpstreamModel, channel.Id)) > 0 || !assignedLegacyPolicyAllows(user, token, group, info.OriginModelName, route.UpstreamModel) {
		return ErrAssignedAccessDenied
	}
	// A frozen basic route and its post-override body must agree. Unknown body
	// evidence cannot authorize a target merely because the public alias passed.
	raw, ok := c.Get("model_route")
	if !ok || raw == nil || ValidateAssignedModelRouteDispatch(c, req, info) != nil {
		return ErrAssignedAccessDenied
	}
	return nil
}

// AssignedModelAvailability uses the same subject intersection as dispatch and
// returns only public names and stable reason codes. Assigned dispatch requires
// unambiguous post-override JSON evidence bounded by the existing 1 MiB canonical
// validator; this read-only projection does not promise future quota or health.
// A public name is available
// only when some real, currently enabled channel/target can serve it.
func AssignedModelAvailability(ctx context.Context, user *model.User, token *model.Token, groups, names []string, override *AssignedAccessPolicyView) ([]AssignedModelAccess, AssignedAccessState, error) {
	tokenID := 0
	if token != nil {
		tokenID = token.Id
	}
	state, err := LoadAssignedAccessState(ctx, user.Id, tokenID)
	if err != nil {
		return nil, state, err
	}
	if override != nil {
		if override.Subject == "user" {
			state.User = *override
		} else {
			state.Token = *override
		}
	}
	result := make([]AssignedModelAccess, 0, len(names))
	for _, name := range names {
		entry := AssignedModelAccess{Model: name, Reasons: []string{}}
		if user.Status != common.UserStatusEnabled || token != nil && (token.Status != common.TokenStatusEnabled || token.ExpiredTime != -1 && token.ExpiredTime <= time.Now().Unix()) {
			entry.Reasons = []string{"credentials_revoked"}
			result = append(result, entry)
			continue
		}
		for _, group := range groups {
			for _, endpoint := range []string{"/v1/chat/completions", "/v1/responses"} {
				// The database routing view joins enabled abilities to enabled channels.
				// Reuse runtime health/quota eligibility instead of inferring availability
				// from a channel's comma-separated Models field alone.
				policy, err := model.ReadAssignedAccessRoutingPolicy(group, name, endpoint)
				if err != nil {
					return nil, state, err
				}
				candidates := append([]model.ChannelRoutingCandidate{}, policy.Rejected...)
				for _, tier := range policy.Tiers {
					for _, candidate := range tier.Candidates {
						candidates = append(candidates, candidate.ChannelRoutingCandidate)
					}
				}
				for _, candidate := range candidates {
					if state.Assigned() && !AssignedAccessProtocolSupported(candidate.ChannelType, endpoint) {
						entry.Reasons = append(entry.Reasons, "policy_protocol_unsupported")
						continue
					}
					target := name
					if candidate.ModelRoute != nil {
						target = candidate.ModelRoute.UpstreamModel
					}
					reasons := state.Reasons(name, target, candidate.ChannelID)
					if len(reasons) > 0 {
						entry.Reasons = append(entry.Reasons, reasons...)
						continue
					}
					if candidate.RouteError != "" {
						continue
					}
					checked := evaluateRelayRoutingCandidate(ctx, candidate, endpoint)
					if checked.RouteError != "" {
						continue
					}
					channel, err := model.GetChannelById(candidate.ChannelID, true)
					if err != nil {
						return nil, state, err
					}
					usableKey := channel.Key != ""
					if channel.ChannelInfo.IsMultiKey {
						usableKey = false
						for index, key := range channel.GetKeys() {
							status, exists := channel.ChannelInfo.MultiKeyStatusList[index]
							if key != "" && (!exists || status == common.ChannelStatusEnabled) {
								usableKey = true
								break
							}
						}
					}
					if !usableKey {
						continue
					}
					if !assignedLegacyPolicyAllows(user, token, group, name, target) {
						entry.Reasons = append(entry.Reasons, "legacy_policy_denied")
						continue
					}
					entry.Allowed = true
				}
			}
		}
		if entry.Allowed {
			entry.Reasons = []string{}
		} else if len(entry.Reasons) == 0 {
			entry.Reasons = []string{"no_available_route"}
		}
		sort.Strings(entry.Reasons)
		entry.Reasons = slices.Compact(entry.Reasons)
		result = append(result, entry)
	}
	return result, state, nil
}

type assignedAccessRoutingContextKey struct{}

func assignedAccessRoutingContext(c *gin.Context, ctx context.Context) (context.Context, error) {
	if c == nil || c.GetInt("id") <= 0 {
		return ctx, nil
	}
	state, err := LoadAssignedAccessState(ctx, c.GetInt("id"), c.GetInt("token_id"))
	if err != nil {
		return ctx, err
	}
	if !state.Assigned() {
		return ctx, nil
	}
	return context.WithValue(ctx, assignedAccessRoutingContextKey{}, state), nil
}

func assignedAccessRoutingCandidate(ctx context.Context, value model.ChannelRoutingCandidate, path string) model.ChannelRoutingCandidate {
	state, ok := ctx.Value(assignedAccessRoutingContextKey{}).(AssignedAccessState)
	if !ok || !state.Assigned() {
		return value
	}
	if !AssignedAccessProtocolSupported(value.ChannelType, path) {
		value.RouteError = "policy_protocol_unsupported"
		return value
	}
	if value.ModelRoute == nil || len(state.Reasons(value.ModelRoute.RequestedModel, value.ModelRoute.UpstreamModel, value.ChannelID)) > 0 {
		value.RouteError = "assigned_access_denied"
	}
	return value
}
