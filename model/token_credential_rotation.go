package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

const (
	tokenCredentialRotationPreviewMaxAge   int64 = 300
	tokenCredentialRotationStateCommitted        = "committed"
	tokenCredentialRotationSettlementMode        = "authoritative_persistent"
	tokenCredentialRotationRevisionVersion       = 1
	tokenCredentialRotationRequestVersion        = 1
	// Credential versions cross the API as JSON numbers. Keep every exposed
	// before/after value within JavaScript's exact integer range.
	tokenCredentialVersionMaxExactJSON int64 = 1<<53 - 1
)

var (
	ErrTokenCredentialVersionChanged            = errors.New("token credential version changed")
	ErrTokenCredentialRotationOperationConflict = errors.New("token credential rotation operation conflicts with an existing request")
	ErrTokenCredentialRotationOutcomeUnknown    = errors.New("token credential rotation committed but outcome is unknown")
)

type TokenCredentialRotationEffects struct {
	TokenIDPreserved      bool   `json:"token_id_preserved"`
	OldSecretInvalidation string `json:"old_secret_invalidation"`
	AcceptedSettlement    string `json:"accepted_settlement"`
	WriterChanged         bool   `json:"writer_changed"`
}

type TokenCredentialRotationPreview struct {
	Status                  string                          `json:"status"`
	PreviewRevision         string                          `json:"preview_revision"`
	ExpiresAt               int64                           `json:"expires_at"`
	CredentialVersionBefore int64                           `json:"credential_version_before"`
	CredentialVersionAfter  int64                           `json:"credential_version_after"`
	Blockers                []TokenMaintenanceBlocker       `json:"blockers"`
	Effects                 *TokenCredentialRotationEffects `json:"effects,omitempty"`
}

type TokenCredentialRotationCommand struct {
	OperationID     string `json:"operation_id"`
	PreviewRevision string `json:"preview_revision"`
	Confirmed       bool   `json:"confirmed"`
}

type TokenCredentialRotationOperationView struct {
	OperationID             string `json:"operation_id"`
	State                   string `json:"state"`
	TokenID                 int    `json:"token_id"`
	CredentialVersionBefore int64  `json:"credential_version_before"`
	CredentialVersionAfter  int64  `json:"credential_version_after"`
	MaskedKey               string `json:"masked_key"`
	CreatedAt               int64  `json:"created_at"`
	OldSecretInvalidatedAt  int64  `json:"old_secret_invalidated_at"`
	SettlementMode          string `json:"settlement_mode"`
	WriterChanged           bool   `json:"writer_changed"`
}

// TokenCredentialRotationReceipt is the immutable, secret-free record of one
// credential rotation. The new secret is returned only from the first
// successful CommitTokenCredentialRotation call and is never persisted here.
type TokenCredentialRotationReceipt struct {
	OperationID             string `json:"-" gorm:"type:char(64);primaryKey;<-:create"`
	RequestDigest           string `json:"-" gorm:"type:char(64);not null;<-:create"`
	ActorID                 int    `json:"-" gorm:"not null;index;<-:create"`
	TokenID                 int    `json:"-" gorm:"not null;index;<-:create"`
	State                   string `json:"-" gorm:"type:varchar(16);not null;<-:create"`
	CredentialVersionBefore int64  `json:"-" gorm:"type:bigint;not null;<-:create"`
	CredentialVersionAfter  int64  `json:"-" gorm:"type:bigint;not null;<-:create"`
	KeyFingerprint          string `json:"-" gorm:"type:char(64);not null;<-:create"`
	MaskedKey               string `json:"-" gorm:"type:varchar(32);not null;<-:create"`
	PreviewRevision         string `json:"-" gorm:"type:char(64);not null;<-:create"`
	CreatedAt               int64  `json:"-" gorm:"type:bigint;not null;<-:create"`
	OldSecretInvalidatedAt  int64  `json:"-" gorm:"type:bigint;not null;<-:create"`
}

func (TokenCredentialRotationReceipt) TableName() string {
	return "token_credential_rotation_receipts"
}

type tokenCredentialRotationEvidenceDigest struct {
	Domain string `json:"domain"`
	Count  int    `json:"count"`
	Digest string `json:"digest"`
}

type tokenCredentialRotationState struct {
	token           Token
	writer          QuotaWriterEpoch
	blockers        []TokenMaintenanceBlocker
	evidence        []tokenCredentialRotationEvidenceDigest
	expiresAt       int64
	previewRevision string
}

type tokenCredentialRotationRevisionRow struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind,omitempty"`
	State       string `json:"state"`
	LockVersion int64  `json:"lock_version"`
	UpdatedAt   int64  `json:"updated_at"`
}

func (*TokenCredentialRotationReceipt) BeforeUpdate(*gorm.DB) error {
	return ErrAccountQuotaReceiptImmutable
}

func (*TokenCredentialRotationReceipt) BeforeDelete(*gorm.DB) error {
	return ErrAccountQuotaReceiptImmutable
}

// ValidateTokenCredentialVersion is the send-boundary generation guard. A
// successful check is the logical admission point for that outbound attempt.
func ValidateTokenCredentialVersion(ctx context.Context, db *gorm.DB, tokenID, userID int, expected int64) error {
	if db == nil || db.Dialector == nil {
		return gorm.ErrInvalidDB
	}
	if tokenID <= 0 || userID <= 0 || expected <= 0 || expected > tokenCredentialVersionMaxExactJSON {
		return ErrTokenCredentialVersionChanged
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var token Token
	if err := db.WithContext(ctx).Select("id", "user_id", "credential_version").
		Where("id = ? AND user_id = ?", tokenID, userID).Take(&token).Error; err != nil {
		return err
	}
	if token.CredentialVersion <= 0 || token.CredentialVersion > tokenCredentialVersionMaxExactJSON || token.CredentialVersion != expected {
		return ErrTokenCredentialVersionChanged
	}
	return nil
}

func tokenCredentialRotationAddBlocker(blockers *[]TokenMaintenanceBlocker, code, state string) {
	for _, blocker := range *blockers {
		if blocker.Code == code && blocker.State == state {
			return
		}
	}
	*blockers = append(*blockers, tokenMaintenanceBlocker(code, state))
}

func tokenCredentialRotationSortBlockers(blockers []TokenMaintenanceBlocker) {
	sort.Slice(blockers, func(i, j int) bool {
		if blockers[i].State != blockers[j].State {
			return blockers[i].State < blockers[j].State
		}
		return blockers[i].Code < blockers[j].Code
	})
}

func tokenCredentialRotationEvidence(domain string, rows any, count int) (tokenCredentialRotationEvidenceDigest, error) {
	digest, err := subscriptionShareDigest(struct {
		Version int    `json:"version"`
		Domain  string `json:"domain"`
		Rows    any    `json:"rows"`
	}{Version: 1, Domain: domain, Rows: rows})
	if err != nil {
		return tokenCredentialRotationEvidenceDigest{}, err
	}
	return tokenCredentialRotationEvidenceDigest{Domain: domain, Count: count, Digest: digest}, nil
}

func tokenCredentialRotationAppendEvidence(state *tokenCredentialRotationState, domain string, rows any, count int) error {
	evidence, err := tokenCredentialRotationEvidence(domain, rows, count)
	if err != nil {
		return err
	}
	state.evidence = append(state.evidence, evidence)
	return nil
}

func tokenCredentialRotationTableReady(db *gorm.DB, value any) bool {
	return db != nil && db.Dialector != nil && db.Migrator().HasTable(value)
}

func readTokenCredentialRotationWriter(ctx context.Context, db *gorm.DB, locked bool, state *tokenCredentialRotationState) error {
	if !tokenCredentialRotationTableReady(db, &QuotaWriterEpoch{}) {
		tokenCredentialRotationAddBlocker(&state.blockers, "quota_writer_state_unknown", TokenMaintenanceStatusUnknown)
		return nil
	}
	var rows []QuotaWriterEpoch
	query := tokenMaintenanceQuery(db.WithContext(ctx), locked).Order("id").Limit(2)
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != 1 {
		tokenCredentialRotationAddBlocker(&state.blockers, "quota_writer_state_unknown", TokenMaintenanceStatusUnknown)
		return tokenCredentialRotationAppendEvidence(state, "quota_writer", rows, len(rows))
	}
	state.writer = rows[0]
	mode, err := validateQuotaWriterEpochFields(int64(rows[0].ID), int64(rows[0].SchemaVersion), rows[0].Mode, rows[0].Epoch, rows[0].LockVersion)
	if err != nil {
		tokenCredentialRotationAddBlocker(&state.blockers, "quota_writer_state_unknown", TokenMaintenanceStatusUnknown)
	} else if mode != QuotaWriterModeAuthoritative {
		tokenCredentialRotationAddBlocker(&state.blockers, "quota_writer_mode_not_authoritative", TokenMaintenanceStatusBlocked)
	}
	return tokenCredentialRotationAppendEvidence(state, "quota_writer", rows, len(rows))
}

func readTokenCredentialRotationBackfills(ctx context.Context, db *gorm.DB, locked bool, state *tokenCredentialRotationState) error {
	for _, value := range []any{&QuotaWorkCursor{}, &AccountQuotaMutationReceipt{}, &AccountQuotaReservationHead{}, &QuotaBalanceBatchDrain{}, &QuotaBalanceBatchSubject{}} {
		if !tokenCredentialRotationTableReady(db, value) {
			tokenCredentialRotationAddBlocker(&state.blockers, "quota_maintenance_backfill_unknown", TokenMaintenanceStatusUnknown)
			return nil
		}
	}
	var cursors []QuotaWorkCursor
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Where("name IN ?", []string{
		quotaWorkCursorAccountMigration, quotaWorkCursorBalanceMigration,
	}).Order("name").Find(&cursors).Error; err != nil {
		return err
	}
	complete := len(cursors) == 2
	for _, cursor := range cursors {
		if !cursor.Complete {
			complete = false
		}
	}
	if err := tokenCredentialRotationAppendEvidence(state, "quota_maintenance_cursor", cursors, len(cursors)); err != nil {
		return err
	}

	var invalidReceipt AccountQuotaMutationReceipt
	result := tokenMaintenanceQuery(db.WithContext(ctx), locked).Select("id").
		Where("fingerprint_version <> ?", AccountQuotaFingerprintVersion).Limit(1).Find(&invalidReceipt)
	if result.Error != nil {
		return result.Error
	}
	complete = complete && result.RowsAffected == 0

	lastReceiptID := int64(0)
	for complete {
		var receipts []AccountQuotaMutationReceipt
		if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Select("id", "request_id").
			Where("phase = ? AND id > ?", AccountQuotaPhaseReserve, lastReceiptID).Order("id").Limit(100).Find(&receipts).Error; err != nil {
			return err
		}
		if len(receipts) == 0 {
			break
		}
		requestIDs := make([]string, 0, len(receipts))
		for _, receipt := range receipts {
			lastReceiptID = receipt.ID
			requestIDs = append(requestIDs, receipt.RequestID)
		}
		var heads []AccountQuotaReservationHead
		if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Select("id", "request_id").Where("request_id IN ?", requestIDs).Find(&heads).Error; err != nil {
			return err
		}
		have := make(map[string]struct{}, len(heads))
		for _, head := range heads {
			have[head.RequestID] = struct{}{}
		}
		for _, requestID := range requestIDs {
			if _, ok := have[requestID]; !ok {
				complete = false
				break
			}
		}
		if len(receipts) < 100 {
			break
		}
	}

	var invalidGeneration QuotaBalanceBatchDrain
	result = tokenMaintenanceQuery(db.WithContext(ctx), locked).Select("id").Where(
		"schema_version < ? OR lock_version < ? OR (cache_applied = ? AND subject_id = ?)",
		quotaBalanceBatchDrainLegacySchemaVersion, 1, false, 0,
	).Limit(1).Find(&invalidGeneration)
	if result.Error != nil {
		return result.Error
	}
	complete = complete && result.RowsAffected == 0
	lastGenerationID := int64(0)
	for complete {
		var generations []QuotaBalanceBatchDrain
		if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Select("id").Where("id > ?", lastGenerationID).Order("id").Limit(100).Find(&generations).Error; err != nil {
			return err
		}
		if len(generations) == 0 {
			break
		}
		ids := make([]int64, 0, len(generations))
		for _, generation := range generations {
			lastGenerationID = generation.ID
			ids = append(ids, generation.ID)
		}
		var subjects []QuotaBalanceBatchSubject
		if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Select("generation_id").Where("generation_id IN ?", ids).Find(&subjects).Error; err != nil {
			return err
		}
		have := make(map[int64]struct{}, len(subjects))
		for _, subject := range subjects {
			have[subject.GenerationID] = struct{}{}
		}
		for _, id := range ids {
			if _, ok := have[id]; !ok {
				complete = false
				break
			}
		}
		if len(generations) < 100 {
			break
		}
	}
	if !complete {
		tokenCredentialRotationAddBlocker(&state.blockers, "quota_maintenance_backfill_incomplete", TokenMaintenanceStatusUnknown)
	}
	return nil
}

func readTokenCredentialRotationLegacyUsage(ctx context.Context, db *gorm.DB, tokenID int, locked bool, state *tokenCredentialRotationState) error {
	if !tokenCredentialRotationTableReady(db, &LegacyUsageReservation{}) {
		tokenCredentialRotationAddBlocker(&state.blockers, "credential_rotation_schema_unavailable", TokenMaintenanceStatusUnknown)
		return nil
	}
	var rows []tokenCredentialRotationRevisionRow
	query := tokenMaintenanceQuery(db.WithContext(ctx), locked).Model(&LegacyUsageReservation{}).
		Select("id", "state", "lock_version", "updated_at").Where("token_id = ?", tokenID).Order("id")
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		switch row.State {
		case LegacyUsageAdmissionFailed, LegacyUsageSettled, LegacyUsageRefunded:
		case LegacyUsagePreparing, LegacyUsagePrepared:
			tokenCredentialRotationAddBlocker(&state.blockers, "legacy_usage_pending", TokenMaintenanceStatusBlocked)
		case LegacyUsageUnknown, LegacyUsageReviewPending:
			tokenCredentialRotationAddBlocker(&state.blockers, "legacy_usage_unknown", TokenMaintenanceStatusUnknown)
		default:
			tokenCredentialRotationAddBlocker(&state.blockers, "legacy_usage_unknown", TokenMaintenanceStatusUnknown)
		}
	}
	return tokenCredentialRotationAppendEvidence(state, "legacy_usage", rows, len(rows))
}

func tokenCredentialRotationLegacySettlementKind(kind string) bool {
	return kind == AccountQuotaSettlementKindLegacyWallet ||
		kind == AccountQuotaSettlementKindLegacySelfUse ||
		kind == AccountQuotaSettlementKindLegacySubscription
}

func readTokenCredentialRotationSettlements(ctx context.Context, db *gorm.DB, tokenID int, locked bool, state *tokenCredentialRotationState) error {
	for _, source := range []struct {
		domain string
		model  any
	}{
		{domain: "legacy_settlement_intent", model: &AccountQuotaSettlementIntent{}},
		{domain: "legacy_settlement_fact", model: &AccountQuotaSettlementFact{}},
	} {
		if !tokenCredentialRotationTableReady(db, source.model) {
			tokenCredentialRotationAddBlocker(&state.blockers, "credential_rotation_schema_unavailable", TokenMaintenanceStatusUnknown)
			continue
		}
		var rows []tokenCredentialRotationRevisionRow
		query := tokenMaintenanceQuery(db.WithContext(ctx), locked).Model(source.model).
			Select("id", "kind", "state", "lock_version", "updated_at").Where("token_id = ?", tokenID).Order("id")
		if err := query.Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if row.Kind == AccountQuotaSettlementKindAuthoritative {
				continue
			}
			if !tokenCredentialRotationLegacySettlementKind(row.Kind) {
				tokenCredentialRotationAddBlocker(&state.blockers, "legacy_settlement_unknown", TokenMaintenanceStatusUnknown)
				continue
			}
			switch row.State {
			case AccountQuotaSettlementApplied:
			case AccountQuotaSettlementPending, AccountQuotaSettlementMaterialized, AccountQuotaSettlementClaimed, AccountQuotaSettlementRetryable:
				tokenCredentialRotationAddBlocker(&state.blockers, "legacy_settlement_pending", TokenMaintenanceStatusBlocked)
			case AccountQuotaSettlementManual:
				tokenCredentialRotationAddBlocker(&state.blockers, "legacy_settlement_unknown", TokenMaintenanceStatusUnknown)
			default:
				tokenCredentialRotationAddBlocker(&state.blockers, "legacy_settlement_unknown", TokenMaintenanceStatusUnknown)
			}
		}
		if err := tokenCredentialRotationAppendEvidence(state, source.domain, rows, len(rows)); err != nil {
			return err
		}
	}
	return nil
}

func tokenCredentialRotationLegacyRefundKind(kind string) bool {
	return kind == AccountQuotaRefundFactKindLegacyWallet ||
		kind == AccountQuotaRefundFactKindLegacySelfUse ||
		kind == AccountQuotaRefundFactKindLegacySubscription
}

func readTokenCredentialRotationRefunds(ctx context.Context, db *gorm.DB, tokenID int, locked bool, state *tokenCredentialRotationState) error {
	if !tokenCredentialRotationTableReady(db, &AccountQuotaRefundFact{}) {
		tokenCredentialRotationAddBlocker(&state.blockers, "credential_rotation_schema_unavailable", TokenMaintenanceStatusUnknown)
		return nil
	}
	var rows []tokenCredentialRotationRevisionRow
	query := tokenMaintenanceQuery(db.WithContext(ctx), locked).Model(&AccountQuotaRefundFact{}).
		Select("id", "kind", "state", "lock_version", "updated_at").Where("token_id = ?", tokenID).Order("id")
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if row.Kind == AccountQuotaRefundFactKindAuthoritative {
			continue
		}
		if !tokenCredentialRotationLegacyRefundKind(row.Kind) {
			tokenCredentialRotationAddBlocker(&state.blockers, "legacy_refund_unknown", TokenMaintenanceStatusUnknown)
			continue
		}
		switch row.State {
		case AccountQuotaRefundFactApplied:
		case AccountQuotaRefundFactPending, AccountQuotaRefundFactClaimed, AccountQuotaRefundFactRetryable:
			tokenCredentialRotationAddBlocker(&state.blockers, "legacy_refund_pending", TokenMaintenanceStatusBlocked)
		case AccountQuotaRefundFactManual:
			tokenCredentialRotationAddBlocker(&state.blockers, "legacy_refund_unknown", TokenMaintenanceStatusUnknown)
		default:
			tokenCredentialRotationAddBlocker(&state.blockers, "legacy_refund_unknown", TokenMaintenanceStatusUnknown)
		}
	}
	return tokenCredentialRotationAppendEvidence(state, "legacy_refund", rows, len(rows))
}

func readTokenCredentialRotationProjections(ctx context.Context, db *gorm.DB, tokenID int, locked bool, state *tokenCredentialRotationState) error {
	if !tokenCredentialRotationTableReady(db, &QuotaProjectionObligation{}) {
		tokenCredentialRotationAddBlocker(&state.blockers, "credential_rotation_schema_unavailable", TokenMaintenanceStatusUnknown)
		return nil
	}
	var rows []tokenCredentialRotationRevisionRow
	query := tokenMaintenanceQuery(db.WithContext(ctx), locked).Model(&QuotaProjectionObligation{}).
		Select("id", "state", "lock_version", "updated_at").Where("token_id = ?", tokenID).Order("id")
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		switch QuotaProjectionObligationState(row.State) {
		case QuotaProjectionObligationStateApplied:
		case QuotaProjectionObligationStatePending, QuotaProjectionObligationStateClaimed, QuotaProjectionObligationStateRetryable:
			tokenCredentialRotationAddBlocker(&state.blockers, "quota_projection_pending", TokenMaintenanceStatusBlocked)
		case QuotaProjectionObligationStateManual:
			tokenCredentialRotationAddBlocker(&state.blockers, "quota_projection_unknown", TokenMaintenanceStatusUnknown)
		default:
			tokenCredentialRotationAddBlocker(&state.blockers, "quota_projection_unknown", TokenMaintenanceStatusUnknown)
		}
	}
	return tokenCredentialRotationAppendEvidence(state, "quota_projection", rows, len(rows))
}

type tokenCredentialRotationBatchRevision struct {
	ID                 int64  `json:"id"`
	SchemaVersion      int    `json:"schema_version"`
	WriterEpoch        int64  `json:"writer_epoch"`
	PayloadFingerprint string `json:"payload_fingerprint"`
	State              string `json:"state"`
	CacheApplied       bool   `json:"cache_applied"`
	LockVersion        int64  `json:"lock_version"`
	UpdatedAt          int64  `json:"updated_at"`
}

func readTokenCredentialRotationBatches(ctx context.Context, db *gorm.DB, tokenID int, locked bool, state *tokenCredentialRotationState) error {
	if !tokenCredentialRotationTableReady(db, &QuotaBalanceBatchDrain{}) || !tokenCredentialRotationTableReady(db, &QuotaBalanceBatchSubject{}) {
		tokenCredentialRotationAddBlocker(&state.blockers, "credential_rotation_schema_unavailable", TokenMaintenanceStatusUnknown)
		return nil
	}
	var generations []QuotaBalanceBatchDrain
	query := tokenMaintenanceQuery(db.WithContext(ctx), locked).
		Where("state NOT IN ?", []string{quotaBalanceBatchDrainApplied, quotaBalanceBatchDrainCancelled}).Order("id")
	if err := query.Find(&generations).Error; err != nil {
		return err
	}
	revisions := make([]tokenCredentialRotationBatchRevision, 0, len(generations))
	tokenKind, err := quotaBalanceStoredSubjectKind(BatchUpdateTypeTokenQuota)
	if err != nil {
		return err
	}
	for _, generation := range generations {
		revisions = append(revisions, tokenCredentialRotationBatchRevision{
			ID: generation.ID, SchemaVersion: generation.SchemaVersion, WriterEpoch: generation.WriterEpoch,
			PayloadFingerprint: generation.PayloadFingerprint, State: generation.State, CacheApplied: generation.CacheApplied,
			LockVersion: generation.LockVersion, UpdatedAt: generation.UpdatedAt,
		})
		valid := validateQuotaBalancePayload(generation.SchemaVersion, generation.Payload) == nil
		fingerprint, fingerprintErr := quotaBalanceBatchPayloadFingerprintForSchema(generation.SchemaVersion, generation.Payload)
		valid = valid && fingerprintErr == nil && fingerprint == generation.PayloadFingerprint
		expectedSubjects, subjectErr := quotaBalanceGenerationSubjects(generation.ID, generation.Payload)
		valid = valid && subjectErr == nil
		var storedSubjects []QuotaBalanceBatchSubject
		if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).
			Where("generation_id = ?", generation.ID).Order("subject_kind, subject_id").Find(&storedSubjects).Error; err != nil {
			return err
		}
		if len(storedSubjects) != len(expectedSubjects) {
			valid = false
		}
		expected := make(map[string]struct{}, len(expectedSubjects))
		for _, subject := range expectedSubjects {
			expected[fmt.Sprintf("%d:%d", subject.SubjectKind, subject.SubjectID)] = struct{}{}
		}
		for _, subject := range storedSubjects {
			key := fmt.Sprintf("%d:%d", subject.SubjectKind, subject.SubjectID)
			if subject.GenerationID != generation.ID {
				valid = false
			}
			if _, ok := expected[key]; !ok {
				valid = false
			}
			delete(expected, key)
		}
		if len(expected) != 0 {
			valid = false
		}
		if !valid {
			tokenCredentialRotationAddBlocker(&state.blockers, "legacy_quota_generation_unknown", TokenMaintenanceStatusUnknown)
			continue
		}
		_, affectsToken := generation.Payload.TokenQuota[tokenID]
		if !affectsToken {
			for _, subject := range storedSubjects {
				if subject.SubjectKind == tokenKind && subject.SubjectID == tokenID {
					affectsToken = true
					break
				}
			}
		}
		if !affectsToken {
			continue
		}
		switch generation.State {
		case quotaBalanceBatchDrainPending, quotaBalanceBatchDrainApplying, quotaBalanceBatchDrainCompensating, quotaBalanceBatchDrainFailed:
			tokenCredentialRotationAddBlocker(&state.blockers, "legacy_quota_generation_pending", TokenMaintenanceStatusBlocked)
		case quotaBalanceBatchDrainUnknown:
			tokenCredentialRotationAddBlocker(&state.blockers, "legacy_quota_generation_unknown", TokenMaintenanceStatusUnknown)
		default:
			tokenCredentialRotationAddBlocker(&state.blockers, "legacy_quota_generation_unknown", TokenMaintenanceStatusUnknown)
		}
	}
	return tokenCredentialRotationAppendEvidence(state, "legacy_quota_generation", revisions, len(revisions))
}

func tokenCredentialRotationTaskLifecycle(status TaskStatus, progress string) (terminal, uncertain bool) {
	progressTerminal := progress == "100%"
	switch status {
	case TaskStatusSuccess, TaskStatusFailure:
		return progressTerminal, !progressTerminal
	case TaskStatusNotStart, TaskStatusSubmitted, TaskStatusQueued, TaskStatusInProgress:
		return false, progress == "" || progressTerminal
	default:
		return false, true
	}
}

func readTokenCredentialRotationAsync(ctx context.Context, db *gorm.DB, token Token, locked bool, state *tokenCredentialRotationState) error {
	for _, value := range []any{&Task{}, &TaskSubmissionOperation{}, &Midjourney{}} {
		if !tokenCredentialRotationTableReady(db, value) {
			tokenCredentialRotationAddBlocker(&state.blockers, "credential_rotation_schema_unavailable", TokenMaintenanceStatusUnknown)
			return nil
		}
	}
	var ownOperations []TaskSubmissionOperation
	if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Select("id", "public_id", "user_id", "token_id", "status", "task_id").
		Where("token_id = ?", token.Id).Order("id").Find(&ownOperations).Error; err != nil {
		return err
	}
	linkedTaskIDs := make([]int64, 0, len(ownOperations))
	for _, operation := range ownOperations {
		if operation.TaskID != nil && *operation.TaskID > 0 {
			linkedTaskIDs = append(linkedTaskIDs, *operation.TaskID)
		}
	}
	linkedTasks := make(map[int64]Task, len(linkedTaskIDs))
	if len(linkedTaskIDs) > 0 {
		var tasks []Task
		// Token is already locked by the commit path. Complete links a Task only
		// after its insert commits while holding that same Token lock. A plain read
		// observes immutable owner/Token identity without adding Task -> Token locks.
		if err := db.WithContext(ctx).Select("id", "task_id", "user_id", "status", "progress", "private_data").
			Where("id IN ?", linkedTaskIDs).Find(&tasks).Error; err != nil {
			return err
		}
		for _, task := range tasks {
			linkedTasks[task.ID] = task
		}
	}
	for _, operation := range ownOperations {
		if operation.UserID != token.UserId || !operation.Status.Valid() || !validTaskSubmissionPublicID(operation.PublicID) {
			tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_identity_unknown", TokenMaintenanceStatusUnknown)
			continue
		}
		var linkedTask Task
		if operation.TaskID != nil && *operation.TaskID > 0 {
			linkedTask = linkedTasks[*operation.TaskID]
			if linkedTask.ID == 0 || linkedTask.TaskID != operation.PublicID || linkedTask.UserId != operation.UserID || linkedTask.PrivateData.TokenId != operation.TokenID {
				tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_identity_unknown", TokenMaintenanceStatusUnknown)
				continue
			}
		}
		switch operation.Status {
		case TaskSubmissionOperationStatusPrepared, TaskSubmissionOperationStatusReserved, TaskSubmissionOperationStatusDispatching:
			if operation.TaskID != nil {
				tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_identity_unknown", TokenMaintenanceStatusUnknown)
			} else {
				tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_operation_pending", TokenMaintenanceStatusBlocked)
			}
		case TaskSubmissionOperationStatusSubmissionUnknown, TaskSubmissionOperationStatusOutcomeUnknown:
			tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_operation_unknown", TokenMaintenanceStatusUnknown)
		case TaskSubmissionOperationStatusAccepted:
			if operation.TaskID == nil || linkedTask.ID == 0 {
				tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_identity_unknown", TokenMaintenanceStatusUnknown)
				continue
			}
			terminal, uncertain := tokenCredentialRotationTaskLifecycle(linkedTask.Status, linkedTask.Progress)
			if uncertain || terminal {
				tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_identity_unknown", TokenMaintenanceStatusUnknown)
			} else {
				tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_operation_pending", TokenMaintenanceStatusBlocked)
			}
		case TaskSubmissionOperationStatusSucceeded, TaskSubmissionOperationStatusFailed:
			if operation.TaskID == nil || linkedTask.ID == 0 {
				tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_identity_unknown", TokenMaintenanceStatusUnknown)
				continue
			}
			terminal, uncertain := tokenCredentialRotationTaskLifecycle(linkedTask.Status, linkedTask.Progress)
			expectedStatus := TaskStatus(TaskStatusSuccess)
			if operation.Status == TaskSubmissionOperationStatusFailed {
				expectedStatus = TaskStatus(TaskStatusFailure)
			}
			if uncertain || !terminal || linkedTask.Status != expectedStatus {
				tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_identity_unknown", TokenMaintenanceStatusUnknown)
			}
		case TaskSubmissionOperationStatusRejected, TaskSubmissionOperationStatusCanceled:
			if operation.TaskID != nil {
				tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_identity_unknown", TokenMaintenanceStatusUnknown)
			}
		default:
			tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_identity_unknown", TokenMaintenanceStatusUnknown)
		}
	}

	lastID := int64(0)
	for {
		var tasks []Task
		query := db.WithContext(ctx).Select("id", "task_id", "user_id", "status", "progress", "private_data").
			Where("id > ? AND (progress IS NULL OR progress <> ? OR status IS NULL OR status NOT IN ?)", lastID, "100%", []TaskStatus{TaskStatusFailure, TaskStatusSuccess}).
			Order("id").Limit(100)
		if err := query.Find(&tasks).Error; err != nil {
			tokenCredentialRotationAddBlocker(&state.blockers, "legacy_async_identity_unknown", TokenMaintenanceStatusUnknown)
			return nil
		}
		if len(tasks) == 0 {
			break
		}
		ids := make([]int64, 0, len(tasks))
		for _, task := range tasks {
			ids = append(ids, task.ID)
			lastID = task.ID
		}
		var operations []TaskSubmissionOperation
		if err := tokenMaintenanceQuery(db.WithContext(ctx), locked).Select("id", "public_id", "user_id", "token_id", "status", "task_id").
			Where("task_id IN ?", ids).Order("id").Find(&operations).Error; err != nil {
			return err
		}
		byTaskID := make(map[int64]TaskSubmissionOperation, len(operations))
		for _, operation := range operations {
			if operation.TaskID == nil || *operation.TaskID <= 0 {
				tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_identity_unknown", TokenMaintenanceStatusUnknown)
				continue
			}
			if _, duplicate := byTaskID[*operation.TaskID]; duplicate {
				tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_identity_unknown", TokenMaintenanceStatusUnknown)
			}
			byTaskID[*operation.TaskID] = operation
		}
		for _, task := range tasks {
			operation, durable := byTaskID[task.ID]
			if durable {
				if operation.PublicID != task.TaskID || operation.UserID != task.UserId || operation.TokenID <= 0 ||
					task.PrivateData.TokenId != operation.TokenID || !operation.Status.Valid() {
					tokenCredentialRotationAddBlocker(&state.blockers, "durable_async_identity_unknown", TokenMaintenanceStatusUnknown)
				}
				continue
			}
			_, uncertain := tokenCredentialRotationTaskLifecycle(task.Status, task.Progress)
			switch task.PrivateData.TokenId {
			case token.Id:
				if uncertain {
					tokenCredentialRotationAddBlocker(&state.blockers, "legacy_async_identity_unknown", TokenMaintenanceStatusUnknown)
				} else {
					tokenCredentialRotationAddBlocker(&state.blockers, "legacy_async_task_pending", TokenMaintenanceStatusBlocked)
				}
			case 0:
				if task.UserId == token.UserId {
					tokenCredentialRotationAddBlocker(&state.blockers, "legacy_async_identity_unknown", TokenMaintenanceStatusUnknown)
				}
			}
		}
		if len(tasks) < 100 {
			break
		}
	}

	var midjourneyRows []Midjourney
	if err := db.WithContext(ctx).Select("id", "user_id", "token_id", "dispatch_token_id", "status", "progress").
		Where("progress IS NULL OR progress <> ? OR status IS NULL OR status NOT IN ?", "100%", []string{string(TaskStatusFailure), string(TaskStatusSuccess)}).
		Where("dispatch_token_id = ? OR token_id = ? OR (dispatch_token_id = ? AND token_id = ? AND user_id = ?)", token.Id, token.Id, 0, 0, token.UserId).
		Order("id").Find(&midjourneyRows).Error; err != nil {
		return err
	}
	for _, task := range midjourneyRows {
		_, uncertain := tokenCredentialRotationTaskLifecycle(TaskStatus(task.Status), task.Progress)
		if task.DispatchTokenID == token.Id || task.TokenId == token.Id {
			if uncertain {
				tokenCredentialRotationAddBlocker(&state.blockers, "midjourney_task_unknown", TokenMaintenanceStatusUnknown)
			} else {
				tokenCredentialRotationAddBlocker(&state.blockers, "midjourney_task_pending", TokenMaintenanceStatusBlocked)
			}
		} else {
			tokenCredentialRotationAddBlocker(&state.blockers, "legacy_async_identity_unknown", TokenMaintenanceStatusUnknown)
		}
	}
	return nil
}

func tokenCredentialRotationRevision(actorID int, state tokenCredentialRotationState) (string, error) {
	payload := struct {
		Version           int                                     `json:"version"`
		ActorID           int                                     `json:"actor_id"`
		TokenID           int                                     `json:"token_id"`
		TokenOwnerID      int                                     `json:"token_owner_id"`
		CredentialVersion int64                                   `json:"credential_version"`
		CredentialHMAC    string                                  `json:"credential_hmac"`
		Writer            QuotaWriterEpoch                        `json:"writer"`
		Evidence          []tokenCredentialRotationEvidenceDigest `json:"evidence"`
		ExpiresAt         int64                                   `json:"expires_at"`
	}{
		Version: tokenCredentialRotationRevisionVersion, ActorID: actorID,
		TokenID: state.token.Id, TokenOwnerID: state.token.UserId,
		CredentialVersion: state.token.CredentialVersion,
		CredentialHMAC:    common.GenerateHMAC("myapi/token-credential-rotation/revision/v1:" + state.token.Key),
		Writer:            state.writer, Evidence: state.evidence, ExpiresAt: state.expiresAt,
	}
	return subscriptionShareDigest(payload)
}

func buildTokenCredentialRotationState(ctx context.Context, db *gorm.DB, actorID int, token Token, now int64, locked bool) (tokenCredentialRotationState, error) {
	state := tokenCredentialRotationState{
		token: token, blockers: []TokenMaintenanceBlocker{}, evidence: []tokenCredentialRotationEvidenceDigest{},
	}
	if db == nil || db.Dialector == nil {
		return state, gorm.ErrInvalidDB
	}
	if now <= 0 || now > math.MaxInt64-tokenCredentialRotationPreviewMaxAge {
		return state, ErrTokenMaintenanceInvalid
	}
	state.expiresAt = (now/tokenCredentialRotationPreviewMaxAge + 1) * tokenCredentialRotationPreviewMaxAge
	if !tokenCredentialRotationTableReady(db, &TokenCredentialRotationReceipt{}) {
		tokenCredentialRotationAddBlocker(&state.blockers, "credential_rotation_schema_unavailable", TokenMaintenanceStatusUnknown)
	}
	if token.CredentialVersion <= 0 {
		tokenCredentialRotationAddBlocker(&state.blockers, "token_credential_version_unknown", TokenMaintenanceStatusUnknown)
	} else if token.CredentialVersion >= tokenCredentialVersionMaxExactJSON {
		tokenCredentialRotationAddBlocker(&state.blockers, "credential_version_overflow", TokenMaintenanceStatusBlocked)
	}
	if token.Key == "" {
		tokenCredentialRotationAddBlocker(&state.blockers, "token_credential_state_unknown", TokenMaintenanceStatusUnknown)
	}
	readers := []func(context.Context, *gorm.DB, int, bool, *tokenCredentialRotationState) error{
		readTokenCredentialRotationLegacyUsage,
		readTokenCredentialRotationSettlements,
		readTokenCredentialRotationRefunds,
		readTokenCredentialRotationProjections,
		readTokenCredentialRotationBatches,
	}
	if err := readTokenCredentialRotationWriter(ctx, db, locked, &state); err != nil {
		return state, err
	}
	if err := readTokenCredentialRotationBackfills(ctx, db, locked, &state); err != nil {
		return state, err
	}
	for _, read := range readers {
		if err := read(ctx, db, token.Id, locked, &state); err != nil {
			return state, err
		}
	}
	if err := readTokenCredentialRotationDispatchAdmissions(ctx, db, token, locked, &state); err != nil {
		return state, err
	}
	if err := readTokenCredentialRotationAsync(ctx, db, token, locked, &state); err != nil {
		return state, err
	}
	sort.Slice(state.evidence, func(i, j int) bool { return state.evidence[i].Domain < state.evidence[j].Domain })
	tokenCredentialRotationSortBlockers(state.blockers)
	if tokenMaintenanceStatus(state.blockers) == TokenMaintenanceStatusReady {
		revision, err := tokenCredentialRotationRevision(actorID, state)
		if err != nil {
			return state, err
		}
		state.previewRevision = revision
	}
	return state, nil
}

func tokenCredentialRotationEffects() *TokenCredentialRotationEffects {
	return &TokenCredentialRotationEffects{
		TokenIDPreserved: true, OldSecretInvalidation: "immediate",
		AcceptedSettlement: "continues", WriterChanged: false,
	}
}

func tokenCredentialRotationPreviewFromState(state tokenCredentialRotationState) *TokenCredentialRotationPreview {
	status := tokenMaintenanceStatus(state.blockers)
	preview := &TokenCredentialRotationPreview{
		Status: status, ExpiresAt: state.expiresAt,
		CredentialVersionBefore: state.token.CredentialVersion,
		Blockers:                state.blockers,
	}
	if state.token.CredentialVersion > 0 && state.token.CredentialVersion < tokenCredentialVersionMaxExactJSON {
		preview.CredentialVersionAfter = state.token.CredentialVersion + 1
	}
	if status == TokenMaintenanceStatusReady {
		preview.PreviewRevision = state.previewRevision
		preview.Effects = tokenCredentialRotationEffects()
	}
	return preview
}

// PreviewTokenCredentialRotation performs database-only qualification. It
// never refreshes projections, drains queues, or mutates accounting history.
func PreviewTokenCredentialRotation(ctx context.Context, db *gorm.DB, actorID, tokenID int, now int64) (*TokenCredentialRotationPreview, error) {
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
	state, err := buildTokenCredentialRotationState(ctx, db, actorID, token, now, false)
	if err != nil {
		return nil, err
	}
	return tokenCredentialRotationPreviewFromState(state), nil
}

func tokenCredentialRotationRequestDigest(actorID, tokenID int, command TokenCredentialRotationCommand) (string, error) {
	return subscriptionShareDigest(struct {
		Version         int    `json:"version"`
		ActorID         int    `json:"actor_id"`
		TokenID         int    `json:"token_id"`
		OperationID     string `json:"operation_id"`
		PreviewRevision string `json:"preview_revision"`
		Confirmed       bool   `json:"confirmed"`
	}{tokenCredentialRotationRequestVersion, actorID, tokenID, command.OperationID, command.PreviewRevision, command.Confirmed})
}

func tokenCredentialRotationReceiptView(receipt TokenCredentialRotationReceipt) (*TokenCredentialRotationOperationView, error) {
	if !canonicalLowerHex(receipt.OperationID, 64) || !canonicalLowerHex(receipt.RequestDigest, 64) ||
		receipt.ActorID <= 0 || receipt.TokenID <= 0 || receipt.State != tokenCredentialRotationStateCommitted ||
		receipt.CredentialVersionBefore <= 0 || receipt.CredentialVersionBefore >= tokenCredentialVersionMaxExactJSON ||
		receipt.CredentialVersionAfter != receipt.CredentialVersionBefore+1 ||
		!canonicalLowerHex(receipt.KeyFingerprint, 64) || !canonicalLowerHex(receipt.PreviewRevision, 64) ||
		receipt.MaskedKey == "" || receipt.CreatedAt <= 0 || receipt.OldSecretInvalidatedAt <= 0 {
		return nil, ErrTokenMaintenanceUnknown
	}
	return &TokenCredentialRotationOperationView{
		OperationID: receipt.OperationID, State: receipt.State, TokenID: receipt.TokenID,
		CredentialVersionBefore: receipt.CredentialVersionBefore, CredentialVersionAfter: receipt.CredentialVersionAfter,
		MaskedKey: receipt.MaskedKey, CreatedAt: receipt.CreatedAt,
		OldSecretInvalidatedAt: receipt.OldSecretInvalidatedAt,
		SettlementMode:         tokenCredentialRotationSettlementMode, WriterChanged: false,
	}, nil
}

func readTokenCredentialRotationReceipt(db *gorm.DB, actorID, tokenID int, operationID string, locked bool) (TokenCredentialRotationReceipt, bool, error) {
	var receipt TokenCredentialRotationReceipt
	query := tokenMaintenanceQuery(db, locked).Where("operation_id = ?", operationID).Limit(1).Find(&receipt)
	if query.Error != nil {
		return receipt, false, query.Error
	}
	if query.RowsAffected == 0 {
		return receipt, false, nil
	}
	if receipt.ActorID != actorID || receipt.TokenID != tokenID {
		return receipt, true, ErrTokenCredentialRotationOperationConflict
	}
	return receipt, true, nil
}

func ReadTokenCredentialRotationOperation(ctx context.Context, db *gorm.DB, actorID, tokenID int, operationID string) (*TokenCredentialRotationOperationView, error) {
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
	var receipt TokenCredentialRotationReceipt
	if err := db.WithContext(ctx).Where("operation_id = ? AND actor_id = ? AND token_id = ?", operationID, actorID, tokenID).Take(&receipt).Error; err != nil {
		return nil, err
	}
	return tokenCredentialRotationReceiptView(receipt)
}

func newTokenCredentialRotationKey(tx *gorm.DB) (string, error) {
	for range 8 {
		candidate, err := common.GenerateKey()
		if err != nil {
			return "", err
		}
		var count int64
		if err := tx.Unscoped().Model(&Token{}).Where(map[string]any{"key": candidate}).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
	}
	return "", ErrTokenMaintenanceUnknown
}

func invalidateTokenCredentialRotationCaches(ctx context.Context, lock *quotaBalanceSubjectLock, oldKey, newKey string) error {
	if !common.RedisEnabled {
		return nil
	}
	if common.RDB == nil || lock == nil || oldKey == "" || newKey == "" || oldKey == newKey {
		return ErrBatchQuotaCacheUnavailable
	}
	const script = `
if redis.call('GET', KEYS[1]) ~= ARGV[1] then
  return 0
end
redis.call('SET', KEYS[2], ARGV[1], 'EX', ARGV[2])
redis.call('SET', KEYS[4], ARGV[1], 'EX', ARGV[2])
redis.call('DEL', KEYS[3], KEYS[5])
return 1`
	if ctx == nil {
		ctx = context.Background()
	}
	cacheCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	result, err := common.RDB.Eval(cacheCtx, script, []string{
		lock.Key,
		getTokenCacheFenceKey(oldKey), getTokenCacheKey(oldKey),
		getTokenCacheFenceKey(newKey), getTokenCacheKey(newKey),
	}, lock.Token, strconv.Itoa(tokenCacheFenceSeconds)).Int()
	if err != nil {
		return fmt.Errorf("%w: credential cache invalidation: %v", ErrBatchQuotaCacheUnavailable, err)
	}
	if result != 1 {
		return ErrQuotaBalanceMutationUnknown
	}
	return nil
}

// CommitTokenCredentialRotation changes only the token secret and credential
// version, then creates an immutable receipt in the same database transaction.
// The raw secret is returned only for the fresh, acknowledged commit.
func CommitTokenCredentialRotation(ctx context.Context, db *gorm.DB, actorID, tokenID int, command TokenCredentialRotationCommand, now int64) (*TokenCredentialRotationOperationView, string, bool, error) {
	if db == nil || db.Dialector == nil {
		return nil, "", false, gorm.ErrInvalidDB
	}
	if now <= 0 || !canonicalLowerHex(command.OperationID, 64) || !canonicalLowerHex(command.PreviewRevision, 64) || !command.Confirmed {
		return nil, "", false, ErrTokenMaintenanceInvalid
	}
	requestDigest, err := tokenCredentialRotationRequestDigest(actorID, tokenID, command)
	if err != nil {
		return nil, "", false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, _, err := loadTokenMaintenanceOwner(ctx, db, actorID, tokenID, false); err != nil {
		return nil, "", false, err
	}
	if existing, found, err := readTokenCredentialRotationReceipt(db.WithContext(ctx), actorID, tokenID, command.OperationID, false); err != nil {
		return nil, "", false, err
	} else if found {
		if existing.RequestDigest != requestDigest {
			return nil, "", false, ErrTokenCredentialRotationOperationConflict
		}
		view, viewErr := tokenCredentialRotationReceiptView(existing)
		return view, "", true, viewErr
	}

	var balanceLock *quotaBalanceSubjectLock
	if common.RedisEnabled {
		balanceLock, err = acquireQuotaBalanceSubjectLockContext(ctx, BatchUpdateTypeTokenQuota, tokenID)
		if err != nil {
			return nil, "", false, errors.Join(ErrTokenMaintenancePreconditionUnknown, err)
		}
		defer releaseQuotaBalanceSubjectLock(balanceLock)
	}

	var result *TokenCredentialRotationOperationView
	var candidateKey string
	var oldKey string
	var replayed bool
	var freshCommitted bool
	var receiptAttempted bool
	err = runChannelQuotaIdentityTransaction(ctx, db, func(tx *gorm.DB) error {
		result = nil
		replayed = false
		freshCommitted = false
		receiptAttempted = false
		_, token, loadErr := loadTokenMaintenanceOwner(ctx, tx, actorID, tokenID, true)
		if loadErr != nil {
			return loadErr
		}
		existing, found, readErr := readTokenCredentialRotationReceipt(tx.WithContext(ctx), actorID, tokenID, command.OperationID, true)
		if readErr != nil {
			return readErr
		}
		if found {
			if existing.RequestDigest != requestDigest {
				return ErrTokenCredentialRotationOperationConflict
			}
			view, viewErr := tokenCredentialRotationReceiptView(existing)
			if viewErr != nil {
				return viewErr
			}
			result, replayed = view, true
			return nil
		}

		// First acquire every qualification lock. The database clock is read only
		// after these reads, then the complete state is rebuilt under the held
		// locks so expiry and CAS validation use the final transaction time.
		if _, buildErr := buildTokenCredentialRotationState(ctx, tx, actorID, token, now, true); buildErr != nil {
			return buildErr
		}
		commitNow, clockErr := taskRecoveryDBTimestamp(tx)
		if clockErr != nil {
			return clockErr
		}
		state, buildErr := buildTokenCredentialRotationState(ctx, tx, actorID, token, commitNow, true)
		if buildErr != nil {
			return buildErr
		}
		switch tokenMaintenanceStatus(state.blockers) {
		case TokenMaintenanceStatusUnknown:
			return ErrTokenMaintenancePreconditionUnknown
		case TokenMaintenanceStatusBlocked:
			return ErrTokenMaintenanceBlocked
		}
		if state.previewRevision == "" {
			return ErrTokenMaintenancePreconditionUnknown
		}
		if state.previewRevision != command.PreviewRevision {
			return ErrTokenMaintenanceConflict
		}
		if candidateKey == "" {
			candidateKey, buildErr = newTokenCredentialRotationKey(tx)
			if buildErr != nil {
				return buildErr
			}
		}
		oldKey = token.Key
		if cacheErr := invalidateTokenCredentialRotationCaches(ctx, balanceLock, oldKey, candidateKey); cacheErr != nil {
			return errors.Join(ErrTokenMaintenancePreconditionUnknown, cacheErr)
		}
		updated := tx.Model(&Token{}).Where(map[string]any{
			"id": token.Id, "user_id": token.UserId, "key": token.Key, "credential_version": token.CredentialVersion,
		}).Updates(map[string]any{"key": candidateKey, "credential_version": token.CredentialVersion + 1})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrTokenMaintenanceConflict
		}
		receipt := TokenCredentialRotationReceipt{
			OperationID: command.OperationID, RequestDigest: requestDigest,
			ActorID: actorID, TokenID: tokenID, State: tokenCredentialRotationStateCommitted,
			CredentialVersionBefore: token.CredentialVersion, CredentialVersionAfter: token.CredentialVersion + 1,
			KeyFingerprint: common.GenerateHMAC("myapi/token-credential-rotation/receipt/v1:" + candidateKey),
			MaskedKey:      MaskTokenKey(candidateKey), PreviewRevision: command.PreviewRevision,
			CreatedAt: commitNow, OldSecretInvalidatedAt: commitNow,
		}
		receiptAttempted = true
		if createErr := tx.Create(&receipt).Error; createErr != nil {
			if tokenAccountMigrationDuplicateReceipt(createErr) {
				return ErrTokenCredentialRotationOperationConflict
			}
			return createErr
		}
		view, viewErr := tokenCredentialRotationReceiptView(receipt)
		if viewErr != nil {
			return viewErr
		}
		result, freshCommitted = view, true
		return nil
	})
	if err != nil {
		if receiptAttempted {
			stored, found, readErr := readTokenCredentialRotationReceipt(db.WithContext(ctx), actorID, tokenID, command.OperationID, false)
			switch {
			case readErr != nil:
				return nil, "", false, errors.Join(ErrTokenCredentialRotationOutcomeUnknown, err, readErr)
			case found && stored.RequestDigest == requestDigest:
				if _, viewErr := tokenCredentialRotationReceiptView(stored); viewErr != nil {
					return nil, "", false, errors.Join(ErrTokenCredentialRotationOutcomeUnknown, err, viewErr)
				}
				return nil, "", false, errors.Join(ErrTokenCredentialRotationOutcomeUnknown, err)
			case found:
				return nil, "", false, ErrTokenCredentialRotationOperationConflict
			}
		}
		return nil, "", false, err
	}
	if replayed {
		return result, "", true, nil
	}
	if !freshCommitted || result == nil || candidateKey == "" || oldKey == "" {
		return nil, "", false, ErrTokenCredentialRotationOutcomeUnknown
	}
	if cacheErr := invalidateTokenCredentialRotationCaches(ctx, balanceLock, oldKey, candidateKey); cacheErr != nil {
		return nil, "", false, errors.Join(ErrTokenCredentialRotationOutcomeUnknown, cacheErr)
	}
	return result, candidateKey, false, nil
}
