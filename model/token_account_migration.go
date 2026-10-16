package model

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	sqlitedriver "github.com/glebarez/go-sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// TokenAccountMigrationReceipt is the immutable result of one atomic account
// migration. It stores only redacted subject hints and a bounded JSON change
// summary; provider subjects and credentials never enter the receipt.
type TokenAccountMigrationReceipt struct {
	OperationID          string `json:"-" gorm:"type:char(64);primaryKey;<-:create"`
	RequestDigest        string `json:"-" gorm:"type:char(64);not null;<-:create"`
	ActorID              int    `json:"-" gorm:"not null;index;<-:create"`
	TokenID              int    `json:"-" gorm:"not null;index;<-:create"`
	State                string `json:"-" gorm:"type:varchar(16);not null;<-:create"`
	SourceSubjectHint    string `json:"-" gorm:"type:varchar(32);not null;<-:create"`
	TargetSubjectHint    string `json:"-" gorm:"type:varchar(32);not null;<-:create"`
	SourceChannelID      int    `json:"-" gorm:"not null;<-:create"`
	TargetChannelID      int    `json:"-" gorm:"not null;<-:create"`
	PolicyBeforeRevision int64  `json:"-" gorm:"type:bigint;not null;<-:create"`
	PolicyAfterRevision  int64  `json:"-" gorm:"type:bigint;not null;<-:create"`
	PreviewRevision      string `json:"-" gorm:"type:char(64);not null;<-:create"`
	ChangesJSON          string `json:"-" gorm:"type:text;not null;<-:create"`
	CreatedAt            int64  `json:"-" gorm:"type:bigint;not null;<-:create"`
}

func (*TokenAccountMigrationReceipt) BeforeUpdate(*gorm.DB) error {
	return ErrAccountQuotaReceiptImmutable
}

func (*TokenAccountMigrationReceipt) BeforeDelete(*gorm.DB) error {
	return ErrAccountQuotaReceiptImmutable
}

func tokenAccountMigrationDuplicateReceipt(err error) bool {
	var mysqlErr *mysqldriver.MySQLError
	var postgresErr *pgconn.PgError
	var sqliteErr *sqlitedriver.Error
	return errors.Is(err, gorm.ErrDuplicatedKey) ||
		errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 ||
		errors.As(err, &postgresErr) && postgresErr.Code == "23505" ||
		errors.As(err, &sqliteErr) && (sqliteErr.Code() == 1555 || sqliteErr.Code() == 2067)
}

type tokenAccountMigrationPoolRevision struct {
	PoolID             int64  `json:"pool_id"`
	DescriptorKey      string `json:"descriptor_key"`
	Revision           int64  `json:"revision"`
	Generation         int64  `json:"generation"`
	SnapshotID         int    `json:"snapshot_id"`
	GenerationRowID    int64  `json:"generation_row_id"`
	ObservedAt         int64  `json:"observed_at"`
	ResetAt            int64  `json:"reset_at"`
	AllocationID       int64  `json:"allocation_id"`
	AllocationShareBPS int    `json:"allocation_share_bps"`
	UsageID            int64  `json:"usage_id"`
	EstimatedUsedMicro int64  `json:"estimated_used_micro"`
	TotalShareBPS      int    `json:"total_share_bps"`
}

type tokenAccountMigrationRevisionPayload struct {
	Version                  int                                 `json:"version"`
	ActorID                  int                                 `json:"actor_id"`
	TokenID                  int                                 `json:"token_id"`
	TokenOwnerID             int                                 `json:"token_owner_id"`
	SourceSubjectRef         string                              `json:"source_subject_ref"`
	TargetSubjectRef         string                              `json:"target_subject_ref"`
	SourceChannelID          int                                 `json:"source_channel_id"`
	TargetChannelID          int                                 `json:"target_channel_id"`
	SourceCredentialRef      string                              `json:"source_credential_ref"`
	TargetCredentialRef      string                              `json:"target_credential_ref"`
	SourceEvidenceID         int64                               `json:"source_evidence_id"`
	SourceEvidenceVersion    int64                               `json:"source_evidence_version"`
	SourceVerifiedSnapshotID int                                 `json:"source_verified_snapshot_id"`
	SourceEvidenceUpdatedAt  int64                               `json:"source_evidence_updated_at"`
	TargetEvidenceID         int64                               `json:"target_evidence_id"`
	TargetEvidenceVersion    int64                               `json:"target_evidence_version"`
	TargetVerifiedSnapshotID int                                 `json:"target_verified_snapshot_id"`
	TargetEvidenceUpdatedAt  int64                               `json:"target_evidence_updated_at"`
	UserPolicyRevision       int64                               `json:"user_policy_revision"`
	UserPolicyAssigned       bool                                `json:"user_policy_assigned"`
	UserPolicyDigest         string                              `json:"user_policy_digest"`
	TokenPolicyRevision      int64                               `json:"token_policy_revision"`
	TokenPolicyAssigned      bool                                `json:"token_policy_assigned"`
	TokenPolicyDigest        string                              `json:"token_policy_digest"`
	Budget                   tokenMaintenanceBudgetState         `json:"budget"`
	SourcePools              []tokenAccountMigrationPoolRevision `json:"source_pools"`
	TargetPools              []tokenAccountMigrationPoolRevision `json:"target_pools"`
	TargetShares             []TokenAccountMigrationTargetShare  `json:"target_shares"`
}

func normalizeTokenAccountMigrationInput(input TokenAccountMigrationPreviewInput) (TokenAccountMigrationPreviewInput, error) {
	if input.TargetChannelID <= 0 || len(input.TargetShares) == 0 || len(input.TargetShares) > 2 {
		return TokenAccountMigrationPreviewInput{}, ErrTokenMaintenanceInvalid
	}
	result := TokenAccountMigrationPreviewInput{TargetChannelID: input.TargetChannelID, TargetShares: append([]TokenAccountMigrationTargetShare(nil), input.TargetShares...)}
	seenSource := make(map[int64]struct{}, len(result.TargetShares))
	seenTarget := make(map[int64]struct{}, len(result.TargetShares))
	for _, share := range result.TargetShares {
		if share.SourcePoolID <= 0 || share.TargetPoolID <= 0 || share.ShareBPS <= 0 || share.ShareBPS > 10000 {
			return TokenAccountMigrationPreviewInput{}, ErrTokenMaintenanceInvalid
		}
		if _, exists := seenSource[share.SourcePoolID]; exists {
			return TokenAccountMigrationPreviewInput{}, ErrTokenMaintenanceInvalid
		}
		if _, exists := seenTarget[share.TargetPoolID]; exists {
			return TokenAccountMigrationPreviewInput{}, ErrTokenMaintenanceInvalid
		}
		seenSource[share.SourcePoolID] = struct{}{}
		seenTarget[share.TargetPoolID] = struct{}{}
	}
	sort.Slice(result.TargetShares, func(i, j int) bool {
		if result.TargetShares[i].SourcePoolID != result.TargetShares[j].SourcePoolID {
			return result.TargetShares[i].SourcePoolID < result.TargetShares[j].SourcePoolID
		}
		return result.TargetShares[i].TargetPoolID < result.TargetShares[j].TargetPoolID
	})
	return result, nil
}

func buildTokenAccountMigrationState(ctx context.Context, db *gorm.DB, actorID int, token Token, input TokenAccountMigrationPreviewInput, now int64, locked bool) (tokenAccountMigrationState, error) {
	state := tokenAccountMigrationState{token: token, input: input, issues: []TokenMaintenanceBlocker{}, targetPools: []tokenMaintenancePoolState{}}
	policies, err := readTokenMaintenancePolicies(ctx, db, token.UserId, token.Id, locked)
	if err != nil {
		return state, err
	}
	state.policies = policies
	state.budget, err = readTokenMaintenanceBudget(ctx, db, token, locked)
	if err != nil {
		return state, err
	}
	state.issues = append(state.issues, state.budget.issues...)
	// Inventory discovery is read-only. A commit locks only the selected source
	// and target channel rows after both account gates are held.
	channels, err := readTokenMaintenanceChannels(ctx, db, now, false)
	if err != nil {
		return state, err
	}
	sourcePoolIDs := make([]int64, 0, len(input.TargetShares))
	for _, share := range input.TargetShares {
		sourcePoolIDs = append(sourcePoolIDs, share.SourcePoolID)
	}
	state.source, err = readTokenMaintenanceSource(ctx, db, token, policies, channels, sourcePoolIDs, now, locked)
	if err != nil {
		return state, err
	}
	state.issues = append(state.issues, state.source.issues...)
	if !policies.valid {
		state.issues = append(state.issues, tokenMaintenanceBlocker("assigned_policy_unknown", TokenMaintenanceStatusUnknown))
	}
	if locked {
		channelIDs := []int{state.source.channel.channel.Id, input.TargetChannelID}
		sort.Ints(channelIDs)
		lockedChannels := make(map[int]tokenMaintenanceChannelEvidence, 2)
		for _, channelID := range channelIDs {
			if channelID <= 0 || lockedChannels[channelID].channel.Id != 0 {
				continue
			}
			var channel Channel
			if err := lockForUpdate(db.WithContext(ctx)).Where("id = ?", channelID).Take(&channel).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return state, ErrTokenMaintenanceConflict
				}
				return state, err
			}
			evidence, err := readTokenMaintenanceChannelEvidence(ctx, db, channel, now, true)
			if err != nil {
				return state, err
			}
			lockedChannels[channelID] = evidence
		}
		if sourceEvidence := lockedChannels[state.source.channel.channel.Id]; sourceEvidence.channel.Id != 0 {
			state.source.channel = sourceEvidence
			if !sourceEvidence.known || sourceEvidence.subjectRef != state.source.subjectRef {
				state.issues = append(state.issues, tokenMaintenanceBlocker("source_credential_subject_changed", TokenMaintenanceStatusUnknown))
			}
		}
		state.target = lockedChannels[input.TargetChannelID]
	} else {
		for _, channel := range channels {
			if channel.channel.Id == input.TargetChannelID {
				state.target = channel
				break
			}
		}
	}
	if state.target.channel.Id == 0 {
		var channel Channel
		if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Where("id = ?", input.TargetChannelID).Take(&channel).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return state, ErrTokenMaintenanceInvalid
			}
			return state, err
		}
		state.target, err = readTokenMaintenanceChannelEvidence(ctx, db, channel, now, locked)
		if err != nil {
			return state, err
		}
	}
	if state.target.channel.Type != constant.ChannelTypeCodex || state.target.channel.ChannelInfo.IsMultiKey || state.target.channel.Status != common.ChannelStatusEnabled {
		state.issues = append(state.issues, tokenMaintenanceBlocker("target_channel_ineligible", TokenMaintenanceStatusBlocked))
	}
	if len(state.source.pools) != 0 {
		var stateIssues []TokenMaintenanceBlocker
		state.targetPools, stateIssues, err = readTokenMaintenanceTargetPools(ctx, db, token, state.source, state.target, now, locked)
		if err != nil {
			return state, err
		}
		state.issues = append(state.issues, stateIssues...)
	}

	sourceByID := make(map[int64]tokenMaintenancePoolState, len(state.source.pools))
	for _, sourcePool := range state.source.pools {
		sourceByID[sourcePool.pool.ID] = sourcePool
	}
	targetByID := make(map[int64]tokenMaintenancePoolState, len(state.targetPools))
	for _, targetPool := range state.targetPools {
		targetByID[targetPool.pool.ID] = targetPool
	}
	mappingValid := len(input.TargetShares) == len(sourceByID) && len(input.TargetShares) == len(targetByID)
	state.changes.Allocations = []TokenAccountMigrationAllocationChange{}
	for _, share := range input.TargetShares {
		sourcePool, sourceFound := sourceByID[share.SourcePoolID]
		targetPool, targetFound := targetByID[share.TargetPoolID]
		if !sourceFound || !targetFound || sourcePool.pool.WindowSource != targetPool.pool.WindowSource || share.ShareBPS != sourcePool.allocation.ShareBPS {
			mappingValid = false
			continue
		}
		state.changes.Allocations = append(state.changes.Allocations, TokenAccountMigrationAllocationChange{
			SourcePoolID: sourcePool.pool.ID, TargetPoolID: targetPool.pool.ID, WindowSlot: tokenMaintenanceWindowSlot(sourcePool.pool.WindowSource),
			ShareBPS: share.ShareBPS, TargetExisting: targetPool.targetExisting,
		})
	}
	if !mappingValid {
		state.issues = append(state.issues, tokenMaintenanceBlocker("target_share_mapping_mismatch", TokenMaintenanceStatusBlocked))
	}
	policyJSON, policyIssues, beforeRevision, afterRevision, err := prepareTokenMaintenancePolicyChange(policies, state.source.channel.channel.Id, state.target.channel.Id)
	if err != nil {
		return state, err
	}
	state.policyJSON = policyJSON
	state.issues = append(state.issues, policyIssues...)
	state.changes.Policy = TokenAccountMigrationPolicyChange{
		Changed: policyJSON != "", BeforeRevision: beforeRevision, AfterRevision: afterRevision,
		SourceChannelID: state.source.channel.channel.Id, TargetChannelID: state.target.channel.Id,
	}
	state.expiresAt = now + tokenAccountMigrationPreviewMaxAge
	for _, pool := range append(append([]tokenMaintenancePoolState{}, state.source.pools...), state.targetPools...) {
		candidate := pool.generation.ObservedAt + SubscriptionShareDefaultMaxAge
		if pool.generation.ResetAt < candidate {
			candidate = pool.generation.ResetAt
		}
		if candidate > 0 && candidate < state.expiresAt {
			state.expiresAt = candidate
		}
	}
	if state.source.subjectRef != "" && state.target.subjectRef != "" && state.source.channel.channel.Id > 0 && state.target.channel.Id > 0 && len(state.source.pools) != 0 && len(state.targetPools) != 0 {
		state.previewRevision, err = tokenAccountMigrationRevision(actorID, state)
		if err != nil {
			return state, err
		}
	}
	return state, nil
}

func tokenAccountMigrationRevision(actorID int, state tokenAccountMigrationState) (string, error) {
	policyDigest := func(row AssignedAccessPolicy) (string, error) {
		if !row.Assigned {
			return "", nil
		}
		digest, err := common.CanonicalJSONObjectDigest([]byte(row.PolicyJSON))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%x", digest[:]), nil
	}
	userPolicyDigest, err := policyDigest(state.policies.userRow)
	if err != nil {
		return "", err
	}
	tokenPolicyDigest, err := policyDigest(state.policies.tokenRow)
	if err != nil {
		return "", err
	}
	payload := tokenAccountMigrationRevisionPayload{
		Version: 1, ActorID: actorID, TokenID: state.token.Id, TokenOwnerID: state.token.UserId,
		SourceSubjectRef: state.source.subjectRef, TargetSubjectRef: state.target.subjectRef,
		SourceChannelID: state.source.channel.channel.Id, TargetChannelID: state.target.channel.Id,
		SourceCredentialRef: state.source.channel.credentialRef, TargetCredentialRef: state.target.credentialRef,
		SourceEvidenceID: state.source.channel.target.ID, SourceEvidenceVersion: state.source.channel.target.EvidenceVersion,
		SourceVerifiedSnapshotID: state.source.channel.target.VerifiedSnapshotID, SourceEvidenceUpdatedAt: state.source.channel.target.UpdatedAt,
		TargetEvidenceID: state.target.target.ID, TargetEvidenceVersion: state.target.target.EvidenceVersion,
		TargetVerifiedSnapshotID: state.target.target.VerifiedSnapshotID, TargetEvidenceUpdatedAt: state.target.target.UpdatedAt,
		UserPolicyRevision: state.policies.userRow.Revision, UserPolicyAssigned: state.policies.userRow.Assigned, UserPolicyDigest: userPolicyDigest,
		TokenPolicyRevision: state.policies.tokenRow.Revision, TokenPolicyAssigned: state.policies.tokenRow.Assigned, TokenPolicyDigest: tokenPolicyDigest,
		Budget:      state.budget,
		SourcePools: []tokenAccountMigrationPoolRevision{}, TargetPools: []tokenAccountMigrationPoolRevision{}, TargetShares: state.input.TargetShares,
	}
	toRevision := func(pool tokenMaintenancePoolState) tokenAccountMigrationPoolRevision {
		return tokenAccountMigrationPoolRevision{
			PoolID: pool.pool.ID, DescriptorKey: pool.pool.DescriptorKey, Revision: pool.pool.Revision,
			Generation: pool.pool.CurrentGeneration, SnapshotID: pool.snapshot.Id, GenerationRowID: pool.generation.ID,
			ObservedAt: pool.generation.ObservedAt, ResetAt: pool.generation.ResetAt, AllocationID: pool.allocation.ID,
			AllocationShareBPS: pool.allocation.ShareBPS, UsageID: pool.usage.ID, EstimatedUsedMicro: pool.usage.EstimatedUsedMicro,
			TotalShareBPS: pool.totalShareBPS,
		}
	}
	for _, pool := range state.source.pools {
		payload.SourcePools = append(payload.SourcePools, toRevision(pool))
	}
	for _, pool := range state.targetPools {
		payload.TargetPools = append(payload.TargetPools, toRevision(pool))
	}
	sort.Slice(payload.SourcePools, func(i, j int) bool { return payload.SourcePools[i].PoolID < payload.SourcePools[j].PoolID })
	sort.Slice(payload.TargetPools, func(i, j int) bool { return payload.TargetPools[i].PoolID < payload.TargetPools[j].PoolID })
	return subscriptionShareDigest(payload)
}

func tokenAccountMigrationPreviewFromState(state tokenAccountMigrationState) *TokenAccountMigrationPreview {
	status := tokenMaintenanceStatus(state.issues)
	preview := &TokenAccountMigrationPreview{
		Status: status, PreviewRevision: "", ExpiresAt: state.expiresAt,
		Blockers: state.issues, Changes: state.changes,
	}
	if status == TokenMaintenanceStatusReady {
		preview.PreviewRevision = state.previewRevision
	}
	return preview
}

// PreviewTokenAccountMigration performs only database reads. Missing or stale
// provider evidence is returned as an explicit blocker and is never refreshed.
func PreviewTokenAccountMigration(ctx context.Context, db *gorm.DB, actorID, tokenID int, input TokenAccountMigrationPreviewInput, now int64) (*TokenAccountMigrationPreview, error) {
	if now <= 0 {
		return nil, ErrTokenMaintenanceInvalid
	}
	normalized, err := normalizeTokenAccountMigrationInput(input)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, token, err := loadTokenMaintenanceOwner(ctx, db, actorID, tokenID, false)
	if err != nil {
		return nil, err
	}
	state, err := buildTokenAccountMigrationState(ctx, db, actorID, token, normalized, now, false)
	if err != nil {
		return nil, err
	}
	return tokenAccountMigrationPreviewFromState(state), nil
}

func tokenAccountMigrationRequestDigest(actorID, tokenID int, command TokenAccountMigrationCommand, normalized TokenAccountMigrationPreviewInput) (string, error) {
	return subscriptionShareDigest(struct {
		Version         int                                `json:"version"`
		ActorID         int                                `json:"actor_id"`
		TokenID         int                                `json:"token_id"`
		TargetChannelID int                                `json:"target_channel_id"`
		TargetShares    []TokenAccountMigrationTargetShare `json:"target_shares"`
		PreviewRevision string                             `json:"preview_revision"`
		Confirmed       bool                               `json:"confirmed"`
	}{1, actorID, tokenID, normalized.TargetChannelID, normalized.TargetShares, command.PreviewRevision, command.Confirmed})
}

func tokenMaintenanceReceiptView(receipt TokenAccountMigrationReceipt) (*TokenMaintenanceOperationView, error) {
	view := &TokenMaintenanceOperationView{
		OperationID: receipt.OperationID, State: receipt.State, TokenID: receipt.TokenID,
		SourceSubjectHint: receipt.SourceSubjectHint, TargetSubjectHint: receipt.TargetSubjectHint,
		SourceChannelID: receipt.SourceChannelID, TargetChannelID: receipt.TargetChannelID,
		PolicyBeforeRevision: receipt.PolicyBeforeRevision, PolicyAfterRevision: receipt.PolicyAfterRevision,
		CreatedAt: receipt.CreatedAt, Changes: TokenAccountMigrationChanges{Allocations: []TokenAccountMigrationAllocationChange{}},
	}
	if common.UnmarshalJsonStr(receipt.ChangesJSON, &view.Changes) != nil || view.Changes.Allocations == nil {
		return nil, ErrTokenMaintenanceUnknown
	}
	return view, nil
}

func applyTokenAccountMigration(tx *gorm.DB, state tokenAccountMigrationState, now int64) error {
	if state.policyJSON != "" {
		updated := tx.Model(&AssignedAccessPolicy{}).
			Where("id = ? AND revision = ? AND assigned = ?", state.policies.tokenRow.ID, state.policies.tokenRow.Revision, true).
			Updates(map[string]any{"revision": state.policies.tokenRow.Revision + 1, "policy_json": state.policyJSON})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrTokenMaintenanceConflict
		}
	}
	targetByID := make(map[int64]tokenMaintenancePoolState, len(state.targetPools))
	for _, pool := range state.targetPools {
		targetByID[pool.pool.ID] = pool
	}
	sourceByID := make(map[int64]tokenMaintenancePoolState, len(state.source.pools))
	for _, pool := range state.source.pools {
		sourceByID[pool.pool.ID] = pool
	}
	for _, change := range state.changes.Allocations {
		source := sourceByID[change.SourcePoolID]
		target := targetByID[change.TargetPoolID]
		cleared := tx.Model(&SubscriptionShareAllocation{}).
			Where("id = ? AND pool_id = ? AND token_id = ? AND share_bps = ?", source.allocation.ID, source.pool.ID, state.token.Id, change.ShareBPS).
			Updates(map[string]any{"share_bps": 0, "updated_at": now})
		if cleared.Error != nil {
			return cleared.Error
		}
		if cleared.RowsAffected != 1 {
			return ErrTokenMaintenanceConflict
		}
		if target.allocation.ID != 0 {
			if target.allocation.ShareBPS != change.ShareBPS {
				moved := tx.Model(&SubscriptionShareAllocation{}).
					Where("id = ? AND pool_id = ? AND token_id = ? AND share_bps = ?", target.allocation.ID, target.pool.ID, state.token.Id, target.allocation.ShareBPS).
					Updates(map[string]any{"share_bps": change.ShareBPS, "updated_at": now})
				if moved.Error != nil {
					return moved.Error
				}
				if moved.RowsAffected != 1 {
					return ErrTokenMaintenanceConflict
				}
			}
		} else {
			allocation := SubscriptionShareAllocation{PoolID: target.pool.ID, TokenID: state.token.Id, UserID: state.token.UserId, ShareBPS: change.ShareBPS, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&allocation).Error; err != nil {
				return err
			}
			usage := SubscriptionShareAllocationUsage{AllocationID: allocation.ID, Generation: target.pool.CurrentGeneration, EstimatedUsedMicro: 0}
			if err := tx.Create(&usage).Error; err != nil {
				return err
			}
		}
	}
	poolStates := append(append([]tokenMaintenancePoolState{}, state.source.pools...), state.targetPools...)
	sort.Slice(poolStates, func(i, j int) bool { return poolStates[i].pool.ID < poolStates[j].pool.ID })
	for _, pool := range poolStates {
		updated := tx.Model(&SubscriptionSharePool{}).
			Where("id = ? AND revision = ?", pool.pool.ID, pool.pool.Revision).
			Update("revision", pool.pool.Revision+1)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrTokenMaintenanceConflict
		}
	}
	return nil
}

// CommitTokenAccountMigration serializes both provider-confirmed subjects and
// atomically moves policy and allocations while preserving allocation usage.
func CommitTokenAccountMigration(ctx context.Context, db *gorm.DB, actorID, tokenID int, command TokenAccountMigrationCommand, now int64) (*TokenMaintenanceOperationView, bool, error) {
	if db == nil || db.Dialector == nil {
		return nil, false, gorm.ErrInvalidDB
	}
	if now <= 0 || !canonicalLowerHex(command.OperationID, 64) || !canonicalLowerHex(command.PreviewRevision, 64) || !command.Confirmed {
		return nil, false, ErrTokenMaintenanceInvalid
	}
	normalized, err := normalizeTokenAccountMigrationInput(command.TokenAccountMigrationPreviewInput)
	if err != nil {
		return nil, false, err
	}
	requestDigest, err := tokenAccountMigrationRequestDigest(actorID, tokenID, command, normalized)
	if err != nil {
		return nil, false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var result *TokenMaintenanceOperationView
	replayed := false
	err = runSubscriptionShareTransaction(ctx, db, func(tx *gorm.DB) error {
		_, token, err := loadTokenMaintenanceOwner(ctx, tx, actorID, tokenID, true)
		if err != nil {
			return err
		}
		var existing TokenAccountMigrationReceipt
		read := lockForUpdate(tx.WithContext(ctx)).Where("operation_id = ?", command.OperationID).Limit(1).Find(&existing)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 0 {
			if existing.ActorID != actorID || existing.TokenID != tokenID || existing.RequestDigest != requestDigest {
				return ErrTokenMaintenanceConflict
			}
			result, err = tokenMaintenanceReceiptView(existing)
			if err != nil {
				return err
			}
			replayed = true
			return nil
		}
		preliminary, err := buildTokenAccountMigrationState(ctx, tx, actorID, token, normalized, now, false)
		if err != nil {
			return err
		}
		if preliminary.source.subjectRef == "" || preliminary.target.subjectRef == "" {
			return ErrTokenMaintenanceUnknown
		}
		subjects := []string{preliminary.source.subjectRef, preliminary.target.subjectRef}
		sort.Strings(subjects)
		if subjects[0] == subjects[1] {
			return ErrTokenMaintenanceBlocked
		}
		for _, subject := range subjects {
			if err := lockSubscriptionShareGate(tx, subject, now); err != nil {
				return err
			}
		}
		lockedState, err := buildTokenAccountMigrationState(ctx, tx, actorID, token, normalized, now, true)
		if err != nil {
			return err
		}
		if lockedState.source.subjectRef != preliminary.source.subjectRef || lockedState.target.subjectRef != preliminary.target.subjectRef {
			return ErrTokenMaintenanceConflict
		}
		commitNow, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		state, err := buildTokenAccountMigrationState(ctx, tx, actorID, token, normalized, commitNow, true)
		if err != nil {
			return err
		}
		if state.source.subjectRef != lockedState.source.subjectRef || state.target.subjectRef != lockedState.target.subjectRef {
			return ErrTokenMaintenanceConflict
		}
		if state.previewRevision == "" {
			return ErrTokenMaintenanceUnknown
		}
		if state.previewRevision != command.PreviewRevision {
			return ErrTokenMaintenanceConflict
		}
		switch tokenMaintenanceStatus(state.issues) {
		case TokenMaintenanceStatusUnknown:
			return ErrTokenMaintenanceUnknown
		case TokenMaintenanceStatusBlocked:
			return ErrTokenMaintenanceBlocked
		}
		if err := applyTokenAccountMigration(tx, state, commitNow); err != nil {
			return err
		}
		changesJSON, err := common.Marshal(state.changes)
		if err != nil {
			return err
		}
		receipt := TokenAccountMigrationReceipt{
			OperationID: command.OperationID, RequestDigest: requestDigest, ActorID: actorID, TokenID: tokenID, State: "committed",
			SourceSubjectHint: tokenMaintenanceSubjectHint(state.source.subjectRef), TargetSubjectHint: tokenMaintenanceSubjectHint(state.target.subjectRef),
			SourceChannelID: state.source.channel.channel.Id, TargetChannelID: state.target.channel.Id,
			PolicyBeforeRevision: state.changes.Policy.BeforeRevision, PolicyAfterRevision: state.changes.Policy.AfterRevision,
			PreviewRevision: state.previewRevision, ChangesJSON: string(changesJSON), CreatedAt: commitNow,
		}
		if err := tx.Create(&receipt).Error; err != nil {
			if tokenAccountMigrationDuplicateReceipt(err) {
				return ErrTokenMaintenanceConflict
			}
			return err
		}
		result, err = tokenMaintenanceReceiptView(receipt)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return result, replayed, nil
}

func ReadTokenMaintenanceOperation(ctx context.Context, db *gorm.DB, actorID, tokenID int, operationID string) (*TokenMaintenanceOperationView, error) {
	if db == nil || db.Dialector == nil {
		return nil, gorm.ErrInvalidDB
	}
	if !canonicalLowerHex(operationID, 64) {
		return nil, ErrTokenMaintenanceInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, _, err := loadTokenMaintenanceOwner(ctx, db, actorID, tokenID, false); err != nil {
		return nil, err
	}
	var receipt TokenAccountMigrationReceipt
	if err := db.WithContext(ctx).Where("operation_id = ? AND actor_id = ? AND token_id = ?", operationID, actorID, tokenID).Take(&receipt).Error; err != nil {
		return nil, err
	}
	return tokenMaintenanceReceiptView(receipt)
}
