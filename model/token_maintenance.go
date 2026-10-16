package model

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"gorm.io/gorm"
)

const (
	TokenMaintenanceStatusReady   = "ready"
	TokenMaintenanceStatusBlocked = "blocked"
	TokenMaintenanceStatusUnknown = "unknown"

	tokenAccountMigrationPreviewMaxAge int64 = 300
)

var (
	ErrTokenMaintenanceInvalid             = errors.New("invalid token maintenance input")
	ErrTokenMaintenanceDenied              = errors.New("token maintenance access denied")
	ErrTokenMaintenanceConflict            = errors.New("token maintenance state conflict")
	ErrTokenMaintenanceBlocked             = errors.New("token maintenance operation is blocked")
	ErrTokenMaintenanceUnknown             = errors.New("token maintenance state is unknown")
	ErrTokenMaintenancePreconditionUnknown = fmt.Errorf("token maintenance precondition is unknown: %w", ErrTokenMaintenanceUnknown)
)

type TokenMaintenanceBlocker struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

type TokenMaintenanceTokenView struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	MaskedKey   string `json:"masked_key"`
	OwnerUserID int    `json:"owner_user_id"`
}

type TokenMaintenancePermissionView struct {
	CanMigrate bool   `json:"can_migrate"`
	Reason     string `json:"reason"`
}

type TokenMaintenanceSourceWindowView struct {
	PoolID     int64  `json:"pool_id"`
	WindowSlot string `json:"window_slot"`
	Revision   int64  `json:"revision"`
	ShareBPS   int    `json:"share_bps"`
	ObservedAt int64  `json:"observed_at"`
	ResetAt    int64  `json:"reset_at"`
}

type TokenMaintenanceTargetWindowView struct {
	PoolID       int64  `json:"pool_id"`
	WindowSlot   string `json:"window_slot"`
	Revision     int64  `json:"revision"`
	ShareBPS     int    `json:"share_bps"`
	AvailableBPS int    `json:"available_bps"`
	ObservedAt   int64  `json:"observed_at"`
	ResetAt      int64  `json:"reset_at"`
}

type TokenMaintenanceSourceView struct {
	ChannelID   int                                `json:"channel_id"`
	ChannelName string                             `json:"channel_name"`
	SubjectHint string                             `json:"subject_hint"`
	Windows     []TokenMaintenanceSourceWindowView `json:"windows"`
}

type TokenMaintenanceCandidateView struct {
	ChannelID      int                                `json:"channel_id"`
	ChannelName    string                             `json:"channel_name"`
	SubjectHint    string                             `json:"subject_hint"`
	CredentialMode string                             `json:"credential_mode"`
	Status         string                             `json:"status"`
	Blockers       []TokenMaintenanceBlocker          `json:"blockers"`
	Windows        []TokenMaintenanceTargetWindowView `json:"windows"`
}

type TokenRotationMaintenanceView struct {
	CommitSupported bool                      `json:"commit_supported"`
	Status          string                    `json:"status"`
	Blockers        []TokenMaintenanceBlocker `json:"blockers"`
}

type TokenAccountMigrationMaintenanceView struct {
	Status     string                          `json:"status"`
	Source     *TokenMaintenanceSourceView     `json:"source"`
	Candidates []TokenMaintenanceCandidateView `json:"candidates"`
	Blockers   []TokenMaintenanceBlocker       `json:"blockers"`
}

type TokenMaintenanceView struct {
	Token       TokenMaintenanceTokenView            `json:"token"`
	Permissions TokenMaintenancePermissionView       `json:"permissions"`
	Rotation    TokenRotationMaintenanceView         `json:"rotation"`
	Migration   TokenAccountMigrationMaintenanceView `json:"migration"`
}

type TokenAccountMigrationTargetShare struct {
	SourcePoolID int64 `json:"source_pool_id"`
	TargetPoolID int64 `json:"target_pool_id"`
	ShareBPS     int   `json:"share_bps"`
}

type TokenAccountMigrationPreviewInput struct {
	TargetChannelID int                                `json:"target_channel_id"`
	TargetShares    []TokenAccountMigrationTargetShare `json:"target_shares"`
}

type TokenAccountMigrationCommand struct {
	TokenAccountMigrationPreviewInput
	OperationID     string `json:"operation_id"`
	PreviewRevision string `json:"preview_revision"`
	Confirmed       bool   `json:"confirmed"`
}

type TokenAccountMigrationPolicyChange struct {
	Changed         bool  `json:"changed"`
	BeforeRevision  int64 `json:"before_revision"`
	AfterRevision   int64 `json:"after_revision"`
	SourceChannelID int   `json:"source_channel_id"`
	TargetChannelID int   `json:"target_channel_id"`
}

type TokenAccountMigrationAllocationChange struct {
	SourcePoolID   int64  `json:"source_pool_id"`
	TargetPoolID   int64  `json:"target_pool_id"`
	WindowSlot     string `json:"window_slot"`
	ShareBPS       int    `json:"share_bps"`
	TargetExisting bool   `json:"target_existing"`
}

type TokenAccountMigrationChanges struct {
	Policy      TokenAccountMigrationPolicyChange       `json:"policy"`
	Allocations []TokenAccountMigrationAllocationChange `json:"allocations"`
}

type TokenAccountMigrationPreview struct {
	Status          string                       `json:"status"`
	PreviewRevision string                       `json:"preview_revision"`
	ExpiresAt       int64                        `json:"expires_at"`
	Blockers        []TokenMaintenanceBlocker    `json:"blockers"`
	Changes         TokenAccountMigrationChanges `json:"changes"`
}

type TokenMaintenanceOperationView struct {
	OperationID          string                       `json:"operation_id"`
	State                string                       `json:"state"`
	TokenID              int                          `json:"token_id"`
	SourceSubjectHint    string                       `json:"source_subject_hint"`
	TargetSubjectHint    string                       `json:"target_subject_hint"`
	SourceChannelID      int                          `json:"source_channel_id"`
	TargetChannelID      int                          `json:"target_channel_id"`
	PolicyBeforeRevision int64                        `json:"policy_before_revision"`
	PolicyAfterRevision  int64                        `json:"policy_after_revision"`
	CreatedAt            int64                        `json:"created_at"`
	Changes              TokenAccountMigrationChanges `json:"changes"`
}

type tokenMaintenanceAssignedPolicy struct {
	Enabled        bool     `json:"enabled"`
	PublicModels   []string `json:"public_models"`
	UpstreamModels []string `json:"upstream_models"`
	ChannelIDs     []int    `json:"channel_ids"`
}

type tokenMaintenancePolicyState struct {
	userRow     AssignedAccessPolicy
	tokenRow    AssignedAccessPolicy
	userPolicy  tokenMaintenanceAssignedPolicy
	tokenPolicy tokenMaintenanceAssignedPolicy
	valid       bool
}

type tokenMaintenanceBudgetReservationRevision struct {
	RequestID      string `json:"request_id"`
	UserID         int    `json:"user_id"`
	ChannelID      int    `json:"channel_id"`
	State          string `json:"state"`
	Reserved       int64  `json:"reserved"`
	FeeEnabled     bool   `json:"fee_enabled"`
	FeeReservedUSD string `json:"fee_reserved_usd"`
	UpdatedAt      int64  `json:"updated_at"`
}

type tokenMaintenanceBudgetState struct {
	Found      bool                                        `json:"found"`
	Policy     TokenBudget                                 `json:"policy"`
	Unresolved []tokenMaintenanceBudgetReservationRevision `json:"unresolved"`
	issues     []TokenMaintenanceBlocker
}

type tokenMaintenanceChannelEvidence struct {
	channel          Channel
	credentialRef    string
	subjectRef       string
	target           ChannelQuotaSamplingTarget
	verifiedSnapshot ChannelQuotaSnapshot
	known            bool
	blockerCode      string
}

type tokenMaintenancePoolState struct {
	pool           SubscriptionSharePool
	generation     SubscriptionShareGeneration
	snapshot       ChannelQuotaSnapshot
	allocation     SubscriptionShareAllocation
	usage          SubscriptionShareAllocationUsage
	totalShareBPS  int
	availableBPS   int
	targetExisting bool
}

type tokenMaintenanceSourceState struct {
	subjectRef string
	channel    tokenMaintenanceChannelEvidence
	pools      []tokenMaintenancePoolState
	issues     []TokenMaintenanceBlocker
}

type tokenAccountMigrationState struct {
	token           Token
	policies        tokenMaintenancePolicyState
	budget          tokenMaintenanceBudgetState
	source          tokenMaintenanceSourceState
	target          tokenMaintenanceChannelEvidence
	targetPools     []tokenMaintenancePoolState
	input           TokenAccountMigrationPreviewInput
	changes         TokenAccountMigrationChanges
	policyJSON      string
	issues          []TokenMaintenanceBlocker
	previewRevision string
	expiresAt       int64
}

func tokenMaintenanceStatus(blockers []TokenMaintenanceBlocker) string {
	status := TokenMaintenanceStatusReady
	for _, blocker := range blockers {
		if blocker.State == TokenMaintenanceStatusUnknown {
			return TokenMaintenanceStatusUnknown
		}
		if blocker.State == TokenMaintenanceStatusBlocked {
			status = TokenMaintenanceStatusBlocked
		}
	}
	return status
}

func tokenMaintenanceBlocker(code, state string) TokenMaintenanceBlocker {
	return TokenMaintenanceBlocker{Code: code, State: state}
}

func tokenMaintenanceSubjectHint(subject string) string {
	if !canonicalChannelQuotaIdentitySubject(subject) {
		return ""
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(subject)))
	return digest[:8] + "…" + digest[len(digest)-8:]
}

func tokenMaintenanceWindowSlot(source string) string {
	return strings.TrimPrefix(source, "codex_wham_usage_")
}

func tokenMaintenanceNativeWindow(source string) bool {
	return source == codexQuotaWindowPrimarySource || source == codexQuotaWindowSecondarySource
}

func tokenMaintenanceQuery(db *gorm.DB, locked bool) *gorm.DB {
	if locked {
		return lockForUpdate(db)
	}
	return db
}

func loadTokenMaintenanceOwner(ctx context.Context, db *gorm.DB, actorID, tokenID int, locked bool) (User, Token, error) {
	if db == nil || db.Dialector == nil {
		return User{}, Token{}, gorm.ErrInvalidDB
	}
	if actorID <= 0 || tokenID <= 0 {
		return User{}, Token{}, ErrTokenMaintenanceInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var actor User
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Select("id", "role", "status").Where("id = ?", actorID).First(&actor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return User{}, Token{}, ErrTokenMaintenanceDenied
		}
		return User{}, Token{}, err
	}
	if actor.Status != common.UserStatusEnabled || actor.Role != common.RoleRootUser {
		return User{}, Token{}, ErrTokenMaintenanceDenied
	}
	var token Token
	if err := tokenMaintenanceQuery(db.WithContext(ctx).Unscoped(), locked).Where("id = ?", tokenID).First(&token).Error; err != nil {
		return User{}, Token{}, err
	}
	if token.DeletedAt.Valid || token.UserId != actorID {
		return User{}, Token{}, ErrTokenMaintenanceDenied
	}
	return actor, token, nil
}

func readTokenMaintenancePolicies(ctx context.Context, db *gorm.DB, userID, tokenID int, locked bool) (tokenMaintenancePolicyState, error) {
	state := tokenMaintenancePolicyState{valid: true}
	var rows []AssignedAccessPolicy
	query := tokenMaintenanceQuery(db.WithContext(ctx), locked).
		Where("(subject_type = ? AND subject_id = ?) OR (subject_type = ? AND subject_id = ?)", "user", userID, "token", tokenID).
		Order("subject_type DESC, subject_id ASC")
	if err := query.Find(&rows).Error; err != nil {
		return state, err
	}
	for _, row := range rows {
		if !validStoredAssignedAccessPolicy(row) {
			state.valid = false
			continue
		}
		switch {
		case row.SubjectType == "user" && row.SubjectID == userID:
			state.userRow = row
		case row.SubjectType == "token" && row.SubjectID == tokenID:
			state.tokenRow = row
		default:
			state.valid = false
		}
	}
	for _, item := range []struct {
		row    AssignedAccessPolicy
		policy *tokenMaintenanceAssignedPolicy
	}{{state.userRow, &state.userPolicy}, {state.tokenRow, &state.tokenPolicy}} {
		if item.row.ID == 0 || !item.row.Assigned {
			continue
		}
		var raw map[string]json.RawMessage
		if common.UnmarshalJsonStr(item.row.PolicyJSON, &raw) != nil || raw == nil {
			state.valid = false
			continue
		}
		for key := range raw {
			if key != "enabled" && key != "public_models" && key != "upstream_models" && key != "channel_ids" {
				state.valid = false
			}
		}
		if common.UnmarshalJsonStr(item.row.PolicyJSON, item.policy) != nil || !validTokenMaintenancePolicy(*item.policy) {
			state.valid = false
		}
	}
	return state, nil
}

func readTokenMaintenanceBudget(ctx context.Context, db *gorm.DB, token Token, locked bool) (tokenMaintenanceBudgetState, error) {
	state := tokenMaintenanceBudgetState{Unresolved: []tokenMaintenanceBudgetReservationRevision{}, issues: []TokenMaintenanceBlocker{}}
	var budgets []TokenBudget
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Where("token_id = ?", token.Id).Limit(2).Find(&budgets).Error; err != nil {
		return state, err
	}
	if len(budgets) > 1 {
		state.issues = append(state.issues, tokenMaintenanceBlocker("token_budget_state_unknown", TokenMaintenanceStatusUnknown))
	} else if len(budgets) == 1 {
		state.Found, state.Policy = true, budgets[0]
		budget := budgets[0]
		if budget.TokenID != token.Id || budget.UserID != token.UserId || budget.Limit < 0 || budget.Used < 0 || budget.Reserved < 0 ||
			budget.Limit > MaxTokenBudget || budget.Used > MaxTokenBudget || budget.Reserved > MaxTokenBudget ||
			budget.Revision <= 0 || budget.Revision >= MaxTokenBudget || validateFeeBudgetState(&budget) != nil {
			state.issues = append(state.issues, tokenMaintenanceBlocker("token_budget_state_unknown", TokenMaintenanceStatusUnknown))
		}
		if budget.Enabled {
			state.issues = append(state.issues, tokenMaintenanceBlocker("token_budget_conflicts_with_subscription_share", TokenMaintenanceStatusBlocked))
		}
		if budget.FeeEnabled {
			state.issues = append(state.issues, tokenMaintenanceBlocker("fee_budget_conflicts_with_subscription_share", TokenMaintenanceStatusBlocked))
		}
		feeReserved, feeErr := feeBudgetDecimal(budget.FeeReservedUSD)
		if budget.PendingRequestID != "" || budget.Reserved != 0 || feeErr != nil || !feeReserved.IsZero() {
			state.issues = append(state.issues, tokenMaintenanceBlocker("token_budget_pending_unknown", TokenMaintenanceStatusUnknown))
		}
	}
	var reservations []TokenBudgetReservation
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).
		Where("token_id = ? AND state NOT IN ?", token.Id, []string{TokenBudgetSettled, TokenBudgetCancelled}).
		Order("request_id ASC").Limit(2).Find(&reservations).Error; err != nil {
		return state, err
	}
	for _, reservation := range reservations {
		state.Unresolved = append(state.Unresolved, tokenMaintenanceBudgetReservationRevision{
			RequestID: reservation.RequestID, UserID: reservation.UserID, ChannelID: reservation.ChannelID,
			State: reservation.State, Reserved: reservation.Reserved, FeeEnabled: reservation.FeeEnabled,
			FeeReservedUSD: reservation.FeeReservedUSD, UpdatedAt: reservation.UpdatedAt,
		})
	}
	if len(reservations) != 0 {
		state.issues = append(state.issues, tokenMaintenanceBlocker("token_budget_reservation_unknown", TokenMaintenanceStatusUnknown))
	}
	return state, nil
}

func validTokenMaintenancePolicy(policy tokenMaintenanceAssignedPolicy) bool {
	seen := make(map[int]struct{}, len(policy.ChannelIDs))
	for _, channelID := range policy.ChannelIDs {
		if channelID <= 0 {
			return false
		}
		if _, exists := seen[channelID]; exists {
			return false
		}
		seen[channelID] = struct{}{}
	}
	return true
}

func tokenMaintenanceContainsID(values []int, value int) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func readTokenMaintenanceChannelEvidence(ctx context.Context, db *gorm.DB, channel Channel, now int64, locked bool) (tokenMaintenanceChannelEvidence, error) {
	evidence := tokenMaintenanceChannelEvidence{channel: channel, blockerCode: "credential_subject_unknown"}
	if channel.Id <= 0 || channel.Type != constant.ChannelTypeCodex || channel.ChannelInfo.IsMultiKey || strings.TrimSpace(channel.Key) == "" {
		evidence.blockerCode = "single_codex_credential_required"
		return evidence, nil
	}
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	if err != nil {
		evidence.blockerCode = "identity_keyring_unavailable"
		return evidence, nil
	}
	resolved, found, err := LookupChannelQuotaIdentity(ctx, db, keyring, fmt.Sprintf("channel_type_%d", channel.Type), common.ChannelQuotaIdentityKindCredential, []byte(channel.Key))
	if err != nil {
		return evidence, err
	}
	if !found || resolved.Quality != ChannelQuotaIdentityQualityCredentialScoped || !canonicalChannelQuotaIdentitySubject(resolved.SubjectRef) {
		return evidence, nil
	}
	evidence.credentialRef = resolved.SubjectRef
	var targets []ChannelQuotaSamplingTarget
	query := tokenMaintenanceQuery(db.WithContext(ctx), locked).
		Where("channel_id = ? AND subject_ref = ? AND identity_quality = ? AND active = ? AND last_result IN ?", channel.Id, resolved.SubjectRef, ChannelQuotaIdentityQualityCredentialScoped, true, []string{"sampled", "duplicate"}).
		Order("id ASC")
	if err := query.Find(&targets).Error; err != nil {
		return evidence, err
	}
	if len(targets) != 1 || !canonicalChannelQuotaIdentitySubject(targets[0].ConfirmedSubjectRef) {
		return evidence, nil
	}
	evidence.target = targets[0]
	evidence.subjectRef = targets[0].ConfirmedSubjectRef
	if targets[0].EvidenceVersion <= 0 || targets[0].VerifiedSnapshotID <= 0 {
		evidence.blockerCode = "credential_evidence_unverified"
		return evidence, nil
	}
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).
		Where("id = ?", targets[0].VerifiedSnapshotID).Take(&evidence.verifiedSnapshot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			evidence.blockerCode = "credential_evidence_unverified"
			return evidence, nil
		}
		return evidence, err
	}
	snapshot := evidence.verifiedSnapshot
	if snapshot.ChannelId != channel.Id || snapshot.SubjectRef != evidence.subjectRef || snapshot.IdentityQuality != ChannelQuotaIdentityQualityProviderConfirmed ||
		snapshot.MetricType != "codex_rate_limit" || !tokenMaintenanceNativeWindow(snapshot.Source) || snapshot.SampleID == "" ||
		!snapshot.CodexObservationQualified || !snapshot.CodexThresholdQualified || snapshot.Status != "success" {
		evidence.blockerCode = "credential_evidence_unverified"
		return evidence, nil
	}
	if !validSubscriptionShareSnapshot(snapshot, now) {
		evidence.blockerCode = "credential_evidence_stale"
		return evidence, nil
	}
	evidence.known = true
	evidence.blockerCode = ""
	return evidence, nil
}

func readTokenMaintenanceChannels(ctx context.Context, db *gorm.DB, now int64, locked bool) ([]tokenMaintenanceChannelEvidence, error) {
	var channels []Channel
	query := tokenMaintenanceQuery(db.WithContext(ctx), locked).
		Where("type = ?", constant.ChannelTypeCodex).
		Order("id ASC")
	if err := query.Find(&channels).Error; err != nil {
		return nil, err
	}
	result := make([]tokenMaintenanceChannelEvidence, 0, len(channels))
	for _, channel := range channels {
		if channel.ChannelInfo.IsMultiKey || strings.TrimSpace(channel.Key) == "" {
			continue
		}
		evidence, err := readTokenMaintenanceChannelEvidence(ctx, db, channel, now, locked)
		if err != nil {
			return nil, err
		}
		result = append(result, evidence)
	}
	return result, nil
}

func readTokenMaintenancePoolState(ctx context.Context, db *gorm.DB, token Token, pool SubscriptionSharePool, now int64, locked bool, requireAllocation bool) (tokenMaintenancePoolState, []TokenMaintenanceBlocker, error) {
	state := tokenMaintenancePoolState{pool: pool}
	issues := make([]TokenMaintenanceBlocker, 0)
	if !pool.Enabled || !pool.Approximate || !canonicalChannelQuotaIdentitySubject(pool.SubjectRef) || !tokenMaintenanceNativeWindow(pool.WindowSource) || pool.CurrentGeneration <= 0 || pool.Revision <= 0 {
		return state, append(issues, tokenMaintenanceBlocker("subscription_pool_invalid", TokenMaintenanceStatusUnknown)), nil
	}
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).
		Where("pool_id = ? AND generation = ?", pool.ID, pool.CurrentGeneration).Take(&state.generation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return state, append(issues, tokenMaintenanceBlocker("subscription_baseline_missing", TokenMaintenanceStatusUnknown)), nil
		}
		return state, issues, err
	}
	generation := state.generation
	if generation.State != SubscriptionShareStateReady || generation.BaselineTime <= 0 || generation.LastSnapshotID <= 0 || generation.LastUsedMicro < 0 || generation.LastUsedMicro > SubscriptionShareTotalMicro || generation.UnattributedMicro < 0 || generation.UnattributedMicro > SubscriptionShareTotalMicro {
		issues = append(issues, tokenMaintenanceBlocker("subscription_baseline_not_ready", TokenMaintenanceStatusUnknown))
	}
	if generation.DescriptorKey != pool.DescriptorKey || generation.WindowSource != pool.WindowSource || generation.WindowType != pool.WindowType || generation.WindowSeconds != pool.WindowSeconds || generation.PlanType != pool.PlanType {
		issues = append(issues, tokenMaintenanceBlocker("subscription_generation_mismatch", TokenMaintenanceStatusUnknown))
	}
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Where("id = ?", generation.LastSnapshotID).Take(&state.snapshot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return state, append(issues, tokenMaintenanceBlocker("subscription_snapshot_missing", TokenMaintenanceStatusUnknown)), nil
		}
		return state, issues, err
	}
	snapshot := state.snapshot
	if pool.SnapshotID != snapshot.Id || snapshot.Id != generation.LastSnapshotID || snapshot.SubjectRef != pool.SubjectRef || snapshot.Source != pool.WindowSource ||
		subscriptionShareDescriptorKey(snapshot.SubjectRef, snapshot.Source, snapshot.WindowType, snapshot.WindowSeconds, snapshot.PlanType) != pool.DescriptorKey || !validSubscriptionShareSnapshot(snapshot, now) {
		issues = append(issues, tokenMaintenanceBlocker("subscription_snapshot_untrusted", TokenMaintenanceStatusUnknown))
	}
	var newer []ChannelQuotaSnapshot
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).
		Where("subject_ref = ? AND identity_quality = ? AND metric_type = ? AND source IN ? AND id > ?", pool.SubjectRef, ChannelQuotaIdentityQualityProviderConfirmed, "codex_rate_limit", []string{pool.WindowSource, "codex_wham_usage"}, generation.LastSnapshotID).
		Order("id ASC").Limit(1).Find(&newer).Error; err != nil {
		return state, issues, err
	}
	if len(newer) != 0 {
		issues = append(issues, tokenMaintenanceBlocker("subscription_projection_pending", TokenMaintenanceStatusUnknown))
	}
	var allocations []SubscriptionShareAllocation
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Where("pool_id = ?", pool.ID).Order("id ASC").Find(&allocations).Error; err != nil {
		return state, issues, err
	}
	for _, allocation := range allocations {
		if allocation.ShareBPS < 0 || allocation.ShareBPS > 10000 || allocation.UserID <= 0 {
			issues = append(issues, tokenMaintenanceBlocker("subscription_allocation_invalid", TokenMaintenanceStatusUnknown))
			continue
		}
		state.totalShareBPS += allocation.ShareBPS
		if allocation.TokenID == token.Id {
			if state.allocation.ID != 0 {
				issues = append(issues, tokenMaintenanceBlocker("subscription_allocation_duplicate", TokenMaintenanceStatusUnknown))
			}
			state.allocation = allocation
			state.targetExisting = true
		}
	}
	if state.totalShareBPS > 10000 {
		issues = append(issues, tokenMaintenanceBlocker("subscription_capacity_invalid", TokenMaintenanceStatusUnknown))
	}
	if state.allocation.ID != 0 {
		if state.allocation.UserID != token.UserId {
			issues = append(issues, tokenMaintenanceBlocker("subscription_allocation_owner_mismatch", TokenMaintenanceStatusUnknown))
		}
		err := tokenMaintenanceQuery(db.WithContext(ctx), locked).
			Where("allocation_id = ? AND generation = ?", state.allocation.ID, pool.CurrentGeneration).Take(&state.usage).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			issues = append(issues, tokenMaintenanceBlocker("subscription_usage_missing", TokenMaintenanceStatusUnknown))
		} else if err != nil {
			return state, issues, err
		} else if state.usage.EstimatedUsedMicro < 0 {
			issues = append(issues, tokenMaintenanceBlocker("subscription_usage_invalid", TokenMaintenanceStatusUnknown))
		}
	} else if requireAllocation {
		issues = append(issues, tokenMaintenanceBlocker("source_allocation_missing", TokenMaintenanceStatusBlocked))
	}
	state.availableBPS = 10000 - (state.totalShareBPS - state.allocation.ShareBPS)
	if state.availableBPS < 0 {
		state.availableBPS = 0
	}
	return state, issues, nil
}

func readTokenMaintenanceSource(ctx context.Context, db *gorm.DB, token Token, policies tokenMaintenancePolicyState, channels []tokenMaintenanceChannelEvidence, expectedPoolIDs []int64, now int64, locked bool) (tokenMaintenanceSourceState, error) {
	state := tokenMaintenanceSourceState{pools: []tokenMaintenancePoolState{}, issues: []TokenMaintenanceBlocker{}}
	var positiveAllocations []SubscriptionShareAllocation
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).
		Where("token_id = ? AND share_bps > ?", token.Id, 0).Order("pool_id ASC").Find(&positiveAllocations).Error; err != nil {
		return state, err
	}
	allocations := positiveAllocations
	poolIDs := make([]int64, 0, len(allocations))
	if len(expectedPoolIDs) == 0 {
		if len(allocations) == 0 {
			state.issues = append(state.issues, tokenMaintenanceBlocker("source_allocation_missing", TokenMaintenanceStatusBlocked))
			return state, nil
		}
		if len(allocations) > 2 {
			state.issues = append(state.issues, tokenMaintenanceBlocker("source_window_count_unsupported", TokenMaintenanceStatusBlocked))
			return state, nil
		}
		for _, allocation := range allocations {
			poolIDs = append(poolIDs, allocation.PoolID)
		}
	} else {
		positiveByPool := make(map[int64]struct{}, len(positiveAllocations))
		positivePoolIDs := make([]int64, 0, len(positiveAllocations))
		for _, allocation := range positiveAllocations {
			positiveByPool[allocation.PoolID] = struct{}{}
			positivePoolIDs = append(positivePoolIDs, allocation.PoolID)
		}
		expectedSubset := true
		for _, poolID := range expectedPoolIDs {
			if _, found := positiveByPool[poolID]; !found {
				expectedSubset = false
				break
			}
		}
		positiveSameSubject := len(positivePoolIDs) != 0
		if positiveSameSubject {
			var positivePools []SubscriptionSharePool
			if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Where("id IN ?", positivePoolIDs).Order("id ASC").Find(&positivePools).Error; err != nil {
				return state, err
			}
			positiveSameSubject = len(positivePools) == len(positivePoolIDs)
			for index, pool := range positivePools {
				if index != 0 && pool.SubjectRef != positivePools[0].SubjectRef {
					positiveSameSubject = false
				}
			}
		}
		if expectedSubset && positiveSameSubject {
			poolIDs = positivePoolIDs
			allocations = positiveAllocations
		} else {
			poolIDs = append(poolIDs, expectedPoolIDs...)
			allocations = nil
			if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).
				Where("token_id = ? AND pool_id IN ?", token.Id, poolIDs).Order("pool_id ASC").Find(&allocations).Error; err != nil {
				return state, err
			}
			state.issues = append(state.issues, tokenMaintenanceBlocker("source_pool_set_invalid", TokenMaintenanceStatusUnknown))
		}
	}
	if len(poolIDs) > 2 {
		state.issues = append(state.issues, tokenMaintenanceBlocker("source_window_count_unsupported", TokenMaintenanceStatusBlocked))
		return state, nil
	}
	allocationByPool := make(map[int64]SubscriptionShareAllocation, len(allocations))
	for _, allocation := range allocations {
		if allocation.ShareBPS <= 0 || allocation.ShareBPS > 10000 || allocation.UserID != token.UserId {
			state.issues = append(state.issues, tokenMaintenanceBlocker("source_allocation_invalid", TokenMaintenanceStatusUnknown))
		}
		allocationByPool[allocation.PoolID] = allocation
	}
	var pools []SubscriptionSharePool
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Where("id IN ?", poolIDs).Order("id ASC").Find(&pools).Error; err != nil {
		return state, err
	}
	if len(pools) != len(poolIDs) {
		state.issues = append(state.issues, tokenMaintenanceBlocker("source_pool_missing", TokenMaintenanceStatusUnknown))
		return state, nil
	}
	seenSlots := make(map[string]struct{}, len(pools))
	for index, pool := range pools {
		if index == 0 {
			state.subjectRef = pool.SubjectRef
		}
		if pool.SubjectRef != state.subjectRef || !pool.Enabled || !tokenMaintenanceNativeWindow(pool.WindowSource) {
			state.issues = append(state.issues, tokenMaintenanceBlocker("source_pool_set_invalid", TokenMaintenanceStatusUnknown))
			return state, nil
		}
		if _, exists := seenSlots[pool.WindowSource]; exists {
			state.issues = append(state.issues, tokenMaintenanceBlocker("source_window_duplicate", TokenMaintenanceStatusUnknown))
			return state, nil
		}
		seenSlots[pool.WindowSource] = struct{}{}
		poolState, issues, err := readTokenMaintenancePoolState(ctx, db, token, pool, now, locked, true)
		if err != nil {
			return state, err
		}
		poolState.allocation = allocationByPool[pool.ID]
		poolState.targetExisting = true
		state.pools = append(state.pools, poolState)
		state.issues = append(state.issues, issues...)
	}
	var allEnabled []SubscriptionSharePool
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).
		Where("subject_ref = ? AND enabled = ? AND window_source IN ?", state.subjectRef, true, []string{codexQuotaWindowPrimarySource, codexQuotaWindowSecondarySource}).
		Order("id ASC").Find(&allEnabled).Error; err != nil {
		return state, err
	}
	if len(allEnabled) != len(state.pools) {
		state.issues = append(state.issues, tokenMaintenanceBlocker("source_pool_set_incomplete", TokenMaintenanceStatusBlocked))
	}
	if err := readTokenMaintenanceAttemptHistory(ctx, db, allocations, locked); err != nil {
		if errors.Is(err, ErrTokenMaintenanceUnknown) {
			state.issues = append(state.issues, tokenMaintenanceBlocker("source_usage_history_unknown", TokenMaintenanceStatusUnknown))
		} else {
			return state, err
		}
	}
	subjectChannels := make([]tokenMaintenanceChannelEvidence, 0)
	for _, channel := range channels {
		if channel.subjectRef == state.subjectRef {
			subjectChannels = append(subjectChannels, channel)
		}
	}
	sourceChannels := subjectChannels
	if policies.tokenRow.Assigned && policies.tokenPolicy.ChannelIDs != nil {
		filtered := make([]tokenMaintenanceChannelEvidence, 0, len(sourceChannels))
		for _, channel := range sourceChannels {
			if tokenMaintenanceContainsID(policies.tokenPolicy.ChannelIDs, channel.channel.Id) {
				filtered = append(filtered, channel)
			}
		}
		sourceChannels = filtered
	}
	if len(sourceChannels) == 1 {
		state.channel = sourceChannels[0]
	} else {
		poolChannelID := 0
		for _, pool := range pools {
			if poolChannelID == 0 {
				poolChannelID = pool.ChannelID
			} else if poolChannelID != pool.ChannelID {
				poolChannelID = -1
			}
		}
		if poolChannelID > 0 {
			for _, channel := range subjectChannels {
				if channel.channel.Id == poolChannelID {
					state.channel = channel
					break
				}
			}
		}
		if state.channel.channel.Id == 0 {
			state.issues = append(state.issues, tokenMaintenanceBlocker("source_channel_ambiguous", TokenMaintenanceStatusUnknown))
		}
	}
	if state.channel.channel.Id != 0 && !state.channel.known {
		state.issues = append(state.issues, tokenMaintenanceBlocker(state.channel.blockerCode, TokenMaintenanceStatusUnknown))
	}
	if state.channel.known {
		for _, pool := range state.pools {
			if pool.snapshot.ChannelId != state.channel.channel.Id || pool.snapshot.SubjectRef != state.channel.subjectRef ||
				pool.snapshot.SampleID == "" || pool.snapshot.SampleID != state.channel.verifiedSnapshot.SampleID {
				state.issues = append(state.issues, tokenMaintenanceBlocker("source_credential_baseline_mismatch", TokenMaintenanceStatusUnknown))
				break
			}
		}
	}
	return state, nil
}

func readTokenMaintenanceAttemptHistory(ctx context.Context, db *gorm.DB, allocations []SubscriptionShareAllocation, locked bool) error {
	allocationIDs := make([]int64, 0, len(allocations))
	for _, allocation := range allocations {
		allocationIDs = append(allocationIDs, allocation.ID)
	}
	if len(allocationIDs) == 0 {
		return nil
	}
	var windows []SubscriptionShareAttemptWindow
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Where("allocation_id IN ?", allocationIDs).Order("id ASC").Find(&windows).Error; err != nil {
		return err
	}
	attemptIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, window := range windows {
		if window.AllocatedAt < -1 || window.AllocatedAt == -1 || window.CoveredAt < 0 {
			return ErrTokenMaintenanceUnknown
		}
		if window.AllocatedAt == 0 || window.CoveredAt == 0 {
			if _, exists := seen[window.AttemptID]; !exists {
				seen[window.AttemptID] = struct{}{}
				attemptIDs = append(attemptIDs, window.AttemptID)
			}
		}
	}
	if len(attemptIDs) == 0 {
		return nil
	}
	var attempts []SubscriptionShareAttempt
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Where("attempt_id IN ?", attemptIDs).Order("attempt_id ASC").Find(&attempts).Error; err != nil {
		return err
	}
	states := make(map[string]string, len(attempts))
	for _, attempt := range attempts {
		states[attempt.AttemptID] = attempt.State
	}
	for _, window := range windows {
		if window.AllocatedAt != 0 && window.CoveredAt != 0 {
			continue
		}
		if states[window.AttemptID] != SubscriptionShareAttemptNotSent {
			return ErrTokenMaintenanceUnknown
		}
	}
	return nil
}

func readTokenMaintenanceTargetPools(ctx context.Context, db *gorm.DB, token Token, source tokenMaintenanceSourceState, target tokenMaintenanceChannelEvidence, now int64, locked bool) ([]tokenMaintenancePoolState, []TokenMaintenanceBlocker, error) {
	issues := make([]TokenMaintenanceBlocker, 0)
	states := make([]tokenMaintenancePoolState, 0)
	existingAllocations := make([]SubscriptionShareAllocation, 0)
	if !target.known {
		issues = append(issues, tokenMaintenanceBlocker(target.blockerCode, TokenMaintenanceStatusUnknown))
		if !canonicalChannelQuotaIdentitySubject(target.subjectRef) {
			return states, issues, nil
		}
	}
	if target.subjectRef == source.subjectRef {
		return states, append(issues, tokenMaintenanceBlocker("target_subject_matches_source", TokenMaintenanceStatusBlocked)), nil
	}
	var pools []SubscriptionSharePool
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).
		Where("subject_ref = ? AND enabled = ? AND window_source IN ?", target.subjectRef, true, []string{codexQuotaWindowPrimarySource, codexQuotaWindowSecondarySource}).
		Order("id ASC").Find(&pools).Error; err != nil {
		return nil, issues, err
	}
	if len(pools) == 0 {
		return states, append(issues, tokenMaintenanceBlocker("target_baseline_missing", TokenMaintenanceStatusUnknown)), nil
	}
	if len(pools) != len(source.pools) || len(pools) > 2 {
		issues = append(issues, tokenMaintenanceBlocker("target_window_set_mismatch", TokenMaintenanceStatusBlocked))
	}
	sourceBySlot := make(map[string]tokenMaintenancePoolState, len(source.pools))
	for _, pool := range source.pools {
		sourceBySlot[pool.pool.WindowSource] = pool
	}
	seenSlots := make(map[string]struct{}, len(pools))
	for _, pool := range pools {
		if _, exists := seenSlots[pool.WindowSource]; exists {
			issues = append(issues, tokenMaintenanceBlocker("target_window_duplicate", TokenMaintenanceStatusUnknown))
			continue
		}
		seenSlots[pool.WindowSource] = struct{}{}
		sourcePool, matches := sourceBySlot[pool.WindowSource]
		if !matches {
			issues = append(issues, tokenMaintenanceBlocker("target_window_set_mismatch", TokenMaintenanceStatusBlocked))
		}
		state, poolIssues, err := readTokenMaintenancePoolState(ctx, db, token, pool, now, locked, false)
		if err != nil {
			return nil, issues, err
		}
		states = append(states, state)
		issues = append(issues, poolIssues...)
		if target.known && (state.snapshot.ChannelId != target.channel.Id || state.snapshot.SubjectRef != target.subjectRef ||
			state.snapshot.SampleID == "" || state.snapshot.SampleID != target.verifiedSnapshot.SampleID) {
			issues = append(issues, tokenMaintenanceBlocker("target_credential_baseline_mismatch", TokenMaintenanceStatusUnknown))
		}
		if state.allocation.ID != 0 {
			existingAllocations = append(existingAllocations, state.allocation)
		}
		if matches && state.availableBPS < sourcePool.allocation.ShareBPS {
			issues = append(issues, tokenMaintenanceBlocker("target_capacity_exceeded", TokenMaintenanceStatusBlocked))
		}
	}
	if err := readTokenMaintenanceAttemptHistory(ctx, db, existingAllocations, locked); err != nil {
		if errors.Is(err, ErrTokenMaintenanceUnknown) {
			issues = append(issues, tokenMaintenanceBlocker("target_usage_history_unknown", TokenMaintenanceStatusUnknown))
		} else {
			return nil, issues, err
		}
	}
	return states, issues, nil
}

func tokenMaintenanceSourceView(source tokenMaintenanceSourceState) *TokenMaintenanceSourceView {
	if source.channel.channel.Id <= 0 || len(source.pools) == 0 {
		return nil
	}
	view := &TokenMaintenanceSourceView{
		ChannelID: source.channel.channel.Id, ChannelName: source.channel.channel.Name,
		SubjectHint: tokenMaintenanceSubjectHint(source.subjectRef), Windows: []TokenMaintenanceSourceWindowView{},
	}
	for _, state := range source.pools {
		view.Windows = append(view.Windows, TokenMaintenanceSourceWindowView{
			PoolID: state.pool.ID, WindowSlot: tokenMaintenanceWindowSlot(state.pool.WindowSource), Revision: state.pool.Revision,
			ShareBPS: state.allocation.ShareBPS, ObservedAt: state.generation.ObservedAt, ResetAt: state.generation.ResetAt,
		})
	}
	sort.Slice(view.Windows, func(i, j int) bool { return view.Windows[i].WindowSlot < view.Windows[j].WindowSlot })
	return view
}

func tokenMaintenanceCandidateView(target tokenMaintenanceChannelEvidence, states []tokenMaintenancePoolState, issues []TokenMaintenanceBlocker) TokenMaintenanceCandidateView {
	view := TokenMaintenanceCandidateView{
		ChannelID: target.channel.Id, ChannelName: target.channel.Name, SubjectHint: tokenMaintenanceSubjectHint(target.subjectRef),
		CredentialMode: "single", Status: tokenMaintenanceStatus(issues), Blockers: issues, Windows: []TokenMaintenanceTargetWindowView{},
	}
	for _, state := range states {
		view.Windows = append(view.Windows, TokenMaintenanceTargetWindowView{
			PoolID: state.pool.ID, WindowSlot: tokenMaintenanceWindowSlot(state.pool.WindowSource), Revision: state.pool.Revision,
			ShareBPS: state.allocation.ShareBPS, AvailableBPS: state.availableBPS, ObservedAt: state.generation.ObservedAt, ResetAt: state.generation.ResetAt,
		})
	}
	sort.Slice(view.Windows, func(i, j int) bool { return view.Windows[i].WindowSlot < view.Windows[j].WindowSlot })
	return view
}

// ReadTokenMaintenance is read-only. In particular, it never refreshes the
// subscription-share projection; unapplied evidence is reported as unknown.
func ReadTokenMaintenance(ctx context.Context, db *gorm.DB, actorID, tokenID int, now int64) (*TokenMaintenanceView, error) {
	if now <= 0 {
		return nil, ErrTokenMaintenanceInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, token, err := loadTokenMaintenanceOwner(ctx, db, actorID, tokenID, false)
	if err != nil {
		return nil, err
	}
	view := &TokenMaintenanceView{
		Token:       TokenMaintenanceTokenView{ID: token.Id, Name: token.Name, MaskedKey: token.GetMaskedKey(), OwnerUserID: token.UserId},
		Permissions: TokenMaintenancePermissionView{CanMigrate: true, Reason: ""},
		Rotation: TokenRotationMaintenanceView{CommitSupported: false, Status: TokenMaintenanceStatusBlocked, Blockers: []TokenMaintenanceBlocker{
			tokenMaintenanceBlocker("rotation_commit_not_supported", TokenMaintenanceStatusBlocked),
			tokenMaintenanceBlocker("token_secret_cache_locator_compatibility_unproven", TokenMaintenanceStatusBlocked),
			tokenMaintenanceBlocker("global_inflight_registry_unavailable", TokenMaintenanceStatusBlocked),
		}},
		Migration: TokenAccountMigrationMaintenanceView{Status: TokenMaintenanceStatusBlocked, Candidates: []TokenMaintenanceCandidateView{}, Blockers: []TokenMaintenanceBlocker{}},
	}
	policies, err := readTokenMaintenancePolicies(ctx, db, token.UserId, token.Id, false)
	if err != nil {
		return nil, err
	}
	budget, err := readTokenMaintenanceBudget(ctx, db, token, false)
	if err != nil {
		return nil, err
	}
	channels, err := readTokenMaintenanceChannels(ctx, db, now, false)
	if err != nil {
		return nil, err
	}
	source, err := readTokenMaintenanceSource(ctx, db, token, policies, channels, nil, now, false)
	if err != nil {
		return nil, err
	}
	if !policies.valid {
		source.issues = append(source.issues, tokenMaintenanceBlocker("assigned_policy_unknown", TokenMaintenanceStatusUnknown))
	}
	source.issues = append(source.issues, budget.issues...)
	view.Migration.Source = tokenMaintenanceSourceView(source)
	view.Migration.Blockers = append(view.Migration.Blockers, source.issues...)
	if tokenMaintenanceStatus(source.issues) == TokenMaintenanceStatusReady {
		for _, target := range channels {
			if target.channel.Status != common.ChannelStatusEnabled || target.channel.Id == source.channel.channel.Id || target.known && target.subjectRef == source.subjectRef {
				continue
			}
			states, issues, err := readTokenMaintenanceTargetPools(ctx, db, token, source, target, now, false)
			if err != nil {
				return nil, err
			}
			_, policyIssues, _, _, err := prepareTokenMaintenancePolicyChange(policies, source.channel.channel.Id, target.channel.Id)
			if err != nil {
				return nil, err
			}
			issues = append(issues, policyIssues...)
			view.Migration.Candidates = append(view.Migration.Candidates, tokenMaintenanceCandidateView(target, states, issues))
		}
	}
	readyCandidate, unknownCandidate := false, false
	for _, candidate := range view.Migration.Candidates {
		readyCandidate = readyCandidate || candidate.Status == TokenMaintenanceStatusReady
		unknownCandidate = unknownCandidate || candidate.Status == TokenMaintenanceStatusUnknown
	}
	sourceStatus := tokenMaintenanceStatus(view.Migration.Blockers)
	switch {
	case sourceStatus != TokenMaintenanceStatusReady:
		view.Migration.Status = sourceStatus
	case readyCandidate:
		view.Migration.Status = TokenMaintenanceStatusReady
	case unknownCandidate:
		view.Migration.Status = TokenMaintenanceStatusUnknown
	default:
		view.Migration.Status = TokenMaintenanceStatusBlocked
		view.Migration.Blockers = append(view.Migration.Blockers, tokenMaintenanceBlocker("migration_target_unavailable", TokenMaintenanceStatusBlocked))
	}
	return view, nil
}

func prepareTokenMaintenancePolicyChange(policies tokenMaintenancePolicyState, sourceChannelID, targetChannelID int) (string, []TokenMaintenanceBlocker, int64, int64, error) {
	issues := make([]TokenMaintenanceBlocker, 0)
	beforeRevision, afterRevision := policies.tokenRow.Revision, policies.tokenRow.Revision
	if !policies.valid {
		return "", append(issues, tokenMaintenanceBlocker("assigned_policy_unknown", TokenMaintenanceStatusUnknown)), beforeRevision, afterRevision, nil
	}
	if policies.userRow.Assigned {
		if !policies.userPolicy.Enabled {
			issues = append(issues, tokenMaintenanceBlocker("user_policy_disabled", TokenMaintenanceStatusBlocked))
		} else if policies.userPolicy.ChannelIDs != nil && !tokenMaintenanceContainsID(policies.userPolicy.ChannelIDs, targetChannelID) {
			issues = append(issues, tokenMaintenanceBlocker("user_policy_target_denied", TokenMaintenanceStatusBlocked))
		}
	}
	if !policies.tokenRow.Assigned {
		return "", issues, beforeRevision, afterRevision, nil
	}
	if !policies.tokenPolicy.Enabled {
		return "", append(issues, tokenMaintenanceBlocker("token_policy_disabled", TokenMaintenanceStatusBlocked)), beforeRevision, afterRevision, nil
	}
	if policies.tokenPolicy.ChannelIDs == nil {
		return "", issues, beforeRevision, afterRevision, nil
	}
	containsSource := tokenMaintenanceContainsID(policies.tokenPolicy.ChannelIDs, sourceChannelID)
	containsTarget := tokenMaintenanceContainsID(policies.tokenPolicy.ChannelIDs, targetChannelID)
	if !containsSource && !containsTarget {
		return "", append(issues, tokenMaintenanceBlocker("token_policy_target_denied", TokenMaintenanceStatusBlocked)), beforeRevision, afterRevision, nil
	}
	if !containsSource {
		return "", issues, beforeRevision, afterRevision, nil
	}
	updated := policies.tokenPolicy
	updated.ChannelIDs = make([]int, 0, len(policies.tokenPolicy.ChannelIDs))
	seen := make(map[int]struct{}, len(policies.tokenPolicy.ChannelIDs))
	for _, channelID := range policies.tokenPolicy.ChannelIDs {
		if channelID == sourceChannelID {
			channelID = targetChannelID
		}
		if _, exists := seen[channelID]; exists {
			continue
		}
		seen[channelID] = struct{}{}
		updated.ChannelIDs = append(updated.ChannelIDs, channelID)
	}
	sort.Ints(updated.ChannelIDs)
	encoded, err := common.Marshal(updated)
	if err != nil {
		return "", nil, 0, 0, err
	}
	afterRevision++
	return string(encoded), issues, beforeRevision, afterRevision, nil
}
