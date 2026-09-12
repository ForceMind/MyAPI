package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TaskTerminalObservationState string

const (
	TaskTerminalObservationPending      TaskTerminalObservationState = "pending"
	TaskTerminalObservationRetryable    TaskTerminalObservationState = "retryable"
	TaskTerminalObservationApplied      TaskTerminalObservationState = "applied"
	TaskTerminalObservationManualReview TaskTerminalObservationState = "manual_review"
)

const (
	taskTerminalObservationMaxTaskDataBytes       = 60 * 1024
	taskTerminalObservationMaxOperationalURLBytes = 8 * 1024
)

var (
	ErrTaskTerminalObservationConflict     = errors.New("task terminal observation conflict")
	ErrTaskTerminalObservationManualReview = errors.New("task terminal observation requires manual review")
)

type TaskTerminalObservation struct {
	ID                      int64                        `gorm:"primaryKey"`
	OperationID             int64                        `gorm:"not null;uniqueIndex"`
	TaskID                  int64                        `gorm:"not null;index"`
	Outcome                 string                       `gorm:"type:varchar(16);not null"`
	ActualQuota             int64                        `gorm:"type:bigint;not null"`
	TaskStatus              TaskStatus                   `gorm:"type:varchar(20);not null"`
	ReasonCode              string                       `gorm:"type:varchar(64);not null"`
	RequestID               string                       `gorm:"type:varchar(64);not null;default:''"`
	ResolutionSource        string                       `gorm:"type:varchar(32);not null;default:''"`
	EvidenceID              string                       `gorm:"type:varchar(191);not null;default:''"`
	EvidenceHash            string                       `gorm:"type:char(64);not null;default:''"`
	EvidenceVersion         int                          `gorm:"not null;default:0"`
	ResultURL               string                       `gorm:"type:text"`
	TaskData                string                       `json:"-" gorm:"type:text"`
	TaskStartTime           int64                        `json:"-" gorm:"type:bigint;not null;default:0"`
	TaskFinishTime          int64                        `json:"-" gorm:"type:bigint;not null;default:0"`
	OperationalResultURL    string                       `json:"-" gorm:"type:text"`
	TaskUpstreamID          string                       `json:"-" gorm:"type:varchar(191);not null;default:''"`
	ClampOp                 string                       `gorm:"type:varchar(32);not null;default:''"`
	ClampKind               string                       `gorm:"type:varchar(32);not null;default:''"`
	ClampOriginal           string                       `gorm:"type:varchar(64);not null;default:''"`
	ClampClamped            int                          `gorm:"not null;default:0"`
	ConflictCount           int                          `gorm:"not null;default:0"`
	LastConflictFingerprint string                       `gorm:"type:char(64);not null;default:''"`
	Fingerprint             string                       `gorm:"type:char(64);not null"`
	State                   TaskTerminalObservationState `gorm:"type:varchar(24);not null;index"`
	AppliedAt               *int64                       `gorm:"type:bigint"`
	CreatedAt               int64                        `gorm:"type:bigint;not null"`
	UpdatedAt               int64                        `gorm:"type:bigint;not null"`
	LockVersion             int64                        `gorm:"type:bigint;not null"`
	RetentionUntil          int64                        `gorm:"type:bigint;index"`
	AttemptCount            int                          `gorm:"not null;default:0"`
	NextAttemptAt           int64                        `gorm:"type:bigint;not null;default:0;index"`
	LastErrorCode           string                       `gorm:"type:varchar(64);not null;default:''"`
}

type TaskTerminalObservationInput struct {
	OperationID          int64
	TaskID               int64
	Outcome              string
	ActualQuota          int64
	ReasonCode           string
	RequestID            string
	ManualReview         bool
	ResolutionSource     string
	EvidenceID           string
	EvidenceHash         string
	EvidenceVersion      int
	ResultURL            string
	TaskData             string
	TaskStartTime        int64
	TaskFinishTime       int64
	OperationalResultURL string
	TaskUpstreamID       string
	QuotaClamp           *common.QuotaClamp
}

func CreateOrLoadTaskTerminalObservation(db *gorm.DB, input TaskTerminalObservationInput) (*TaskTerminalObservation, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	input.Outcome = strings.ToLower(strings.TrimSpace(input.Outcome))
	input.ReasonCode = strings.TrimSpace(input.ReasonCode)
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.ResolutionSource = strings.TrimSpace(input.ResolutionSource)
	input.EvidenceID = strings.TrimSpace(input.EvidenceID)
	input.EvidenceHash = strings.ToLower(strings.TrimSpace(input.EvidenceHash))
	input.ResultURL = strings.TrimSpace(input.ResultURL)
	input.TaskData = strings.TrimSpace(input.TaskData)
	input.OperationalResultURL = strings.TrimSpace(input.OperationalResultURL)
	input.TaskUpstreamID = strings.TrimSpace(input.TaskUpstreamID)
	if input.RequestID == "" {
		input.RequestID = stableTaskBillingRequestID("", fmt.Sprintf("terminal:%d:%d", input.OperationID, input.TaskID))
	}
	if input.QuotaClamp != nil {
		input.ManualReview = true
	}
	if input.OperationID <= 0 || input.TaskID < 0 || (input.TaskID == 0 && !input.ManualReview) || input.ActualQuota < 0 || input.ActualQuota > int64(common.MaxQuota) ||
		(input.Outcome != "succeeded" && input.Outcome != "failed") || !validTaskTerminalReasonCode(input.ReasonCode) || len(input.RequestID) > 64 || len(input.ResolutionSource) > 32 || len(input.EvidenceID) > 191 ||
		(input.EvidenceHash != "" && !validTaskRecoveryDigest(input.EvidenceHash)) || !validSanitizedTerminalResultURL(input.ResultURL) || !validTerminalTaskProjection(input.TaskData, input.TaskStartTime, input.TaskFinishTime, input.OperationalResultURL, input.TaskUpstreamID) ||
		(!input.ManualReview && (input.ResolutionSource != TaskSubmissionResolutionSourceProviderVerified || input.EvidenceVersion != 1 || input.EvidenceID == "")) {
		return nil, ErrTaskRecoveryInvalidRecord
	}
	status := TaskStatus(TaskStatusSuccess)
	if input.Outcome == "failed" {
		status = TaskStatusFailure
	}
	payload := struct {
		Version              int    `json:"version"`
		OperationID          int64  `json:"operation_id"`
		TaskID               int64  `json:"task_id"`
		Outcome              string `json:"outcome"`
		ActualQuota          int64  `json:"actual_quota"`
		EvidenceID           string `json:"evidence_id"`
		EvidenceHash         string `json:"evidence_hash"`
		EvidenceVersion      int    `json:"evidence_version"`
		ReasonCode           string `json:"reason_code"`
		RequestID            string `json:"request_id"`
		ResolutionSource     string `json:"resolution_source"`
		ResultURL            string `json:"result_url"`
		TaskData             string `json:"task_data"`
		TaskStartTime        int64  `json:"task_start_time"`
		TaskFinishTime       int64  `json:"task_finish_time"`
		OperationalResultURL string `json:"operational_result_url"`
		TaskUpstreamID       string `json:"task_upstream_id"`
	}{2, input.OperationID, input.TaskID, input.Outcome, input.ActualQuota, input.EvidenceID, input.EvidenceHash, input.EvidenceVersion, input.ReasonCode, input.RequestID, input.ResolutionSource, input.ResultURL, input.TaskData, input.TaskStartTime, input.TaskFinishTime, input.OperationalResultURL, input.TaskUpstreamID}
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	fingerprint := hex.EncodeToString(digest[:])
	var result TaskTerminalObservation
	conflict := false
	err = db.Transaction(func(tx *gorm.DB) error {
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		taskFinishTime := input.TaskFinishTime
		if taskFinishTime == 0 {
			taskFinishTime = now
		}
		candidate := TaskTerminalObservation{OperationID: input.OperationID, TaskID: input.TaskID, Outcome: input.Outcome, ActualQuota: input.ActualQuota, TaskStatus: status, ReasonCode: input.ReasonCode, RequestID: input.RequestID, ResolutionSource: input.ResolutionSource, EvidenceID: input.EvidenceID, EvidenceHash: input.EvidenceHash, EvidenceVersion: input.EvidenceVersion, ResultURL: input.ResultURL, TaskData: input.TaskData, TaskStartTime: input.TaskStartTime, TaskFinishTime: taskFinishTime, OperationalResultURL: input.OperationalResultURL, TaskUpstreamID: input.TaskUpstreamID, Fingerprint: fingerprint, State: TaskTerminalObservationPending, CreatedAt: now, UpdatedAt: now, LockVersion: 1, RetentionUntil: now + TaskSubmissionTerminalRetentionSeconds}
		if input.ManualReview {
			candidate.State = TaskTerminalObservationManualReview
		}
		if input.QuotaClamp != nil {
			candidate.ClampOp = input.QuotaClamp.Op
			candidate.ClampKind = string(input.QuotaClamp.Kind)
			candidate.ClampOriginal = fmt.Sprint(input.QuotaClamp.Original)
			candidate.ClampClamped = input.QuotaClamp.Clamped
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("operation_id = ?", input.OperationID).First(&result).Error; err != nil {
			return err
		}
		if result.Fingerprint != fingerprint {
			updates := map[string]interface{}{"conflict_count": gorm.Expr("conflict_count + 1"), "last_conflict_fingerprint": fingerprint, "updated_at": now, "lock_version": gorm.Expr("lock_version + 1")}
			if result.State != TaskTerminalObservationApplied {
				updates["state"] = TaskTerminalObservationManualReview
				updates["next_attempt_at"] = 0
			}
			updateResult := taskRecoveryControlledWrite(tx).Table("task_terminal_observations").Where("id = ? AND lock_version = ? AND conflict_count < ?", result.ID, result.LockVersion, common.MaxQuota).Updates(updates)
			if updateResult.Error != nil {
				return updateResult.Error
			}
			if updateResult.RowsAffected != 1 {
				return ErrTaskRecoveryInvalidRecord
			}
			conflict = true
			return nil
		}
		if input.ManualReview && (result.State == TaskTerminalObservationPending || result.State == TaskTerminalObservationRetryable) {
			updates := map[string]interface{}{"state": TaskTerminalObservationManualReview, "updated_at": now, "next_attempt_at": 0, "lock_version": gorm.Expr("lock_version + 1")}
			if input.QuotaClamp != nil {
				updates["clamp_op"] = input.QuotaClamp.Op
				updates["clamp_kind"] = string(input.QuotaClamp.Kind)
				updates["clamp_original"] = fmt.Sprint(input.QuotaClamp.Original)
				updates["clamp_clamped"] = input.QuotaClamp.Clamped
			}
			r := taskRecoveryControlledWrite(tx).Table("task_terminal_observations").Where("id = ? AND state = ? AND lock_version = ?", result.ID, result.State, result.LockVersion).Updates(updates)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return ErrTaskQuotaReservationCASLost
			}
			result.State = TaskTerminalObservationManualReview
		}
		return nil
	})
	if err == nil && conflict {
		err = ErrTaskTerminalObservationConflict
	}
	return &result, err
}

type TaskTerminalApplyResult struct {
	Observation *TaskTerminalObservation
	Receipt     *QuotaMutationReceipt
	Outbox      *TaskBillingLogOutbox
}

func ApplyTaskTerminalObservation(db *gorm.DB, observationID int64) (*TaskTerminalApplyResult, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var identity TaskTerminalObservation
	if err := db.Where("id = ?", observationID).First(&identity).Error; err != nil {
		return nil, err
	}
	if err := validateStoredTaskTerminalObservation(&identity); err != nil {
		if markErr := MarkTaskTerminalObservationManualReview(db, identity.ID, identity.LockVersion); markErr != nil {
			return nil, fmt.Errorf("invalid terminal observation: %v; mark manual review: %w", err, markErr)
		}
		return nil, ErrTaskTerminalObservationManualReview
	}
	if identity.State == TaskTerminalObservationManualReview {
		return nil, ErrTaskTerminalObservationManualReview
	}
	var output TaskTerminalApplyResult
	err := db.Transaction(func(tx *gorm.DB) error {
		var operation TaskSubmissionOperation
		var task Task
		var observation TaskTerminalObservation

		// Identity reads above are non-locking. Economic locks always follow the
		// project-wide order User -> Token -> Subscription -> Operation -> Attempt -> Task -> Observation.
		if err := tx.Session(&gorm.Session{NewDB: true}).Where("id = ?", identity.OperationID).First(&operation).Error; err != nil {
			return err
		}
		var evidenceAttempt TaskSubmissionAttempt
		if err := tx.Session(&gorm.Session{NewDB: true}).Select("provider_operation_id", "upstream_request_id").Where("operation_id = ?", identity.OperationID).First(&evidenceAttempt).Error; err != nil {
			return err
		}
		if identity.ResolutionSource != TaskSubmissionResolutionSourceProviderVerified || identity.EvidenceVersion != 1 || identity.EvidenceID == "" ||
			(identity.EvidenceID != evidenceAttempt.ProviderOperationID && identity.EvidenceID != evidenceAttempt.UpstreamRequestID) {
			return ErrTaskRecoveryInvalidRecord
		}
		reserve, err := findTaskQuotaReservation(tx, identity.OperationID, operation.UserID, operation.TokenID)
		if err != nil {
			return err
		}
		if reserve == nil {
			return ErrTaskQuotaReservationNotFound
		}
		mutationType := string(TaskBillingEventTypeTerminalSettlement)
		if identity.Outcome == "failed" {
			mutationType = string(TaskBillingEventTypeRefund)
		}
		receipt, err := findTaskQuotaReceiptByType(tx, operation.ID, mutationType, operation.UserID, operation.TokenID)
		if err != nil {
			return err
		}
		if receipt == nil {
			bc := TaskBillingContext(reserve.BillingContext)
			if identity.Outcome == "succeeded" {
				receipt, err = SettleTaskQuotaReservation(tx, TaskQuotaSettlementInput{OperationID: operation.ID, UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: reserve.ChannelID, ExpectedOperationVersion: operation.LockVersion, ActualQuota: identity.ActualQuota, ReasonCode: identity.ReasonCode, BillingContext: bc, TargetOperationStatus: TaskSubmissionOperationStatusSucceeded, ResolutionSource: identity.ResolutionSource, RequireStatisticsEvidence: true, EvidenceID: identity.EvidenceID, EvidenceHash: identity.EvidenceHash, EvidenceVersion: identity.EvidenceVersion, RequestID: identity.RequestID})
			} else {
				receipt, err = ReleaseTaskQuotaReservation(tx, TaskQuotaReleaseInput{OperationID: operation.ID, UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: reserve.ChannelID, ExpectedOperationVersion: operation.LockVersion, ReasonCode: identity.ReasonCode, BillingContext: bc, TargetOperationStatus: TaskSubmissionOperationStatusFailed, ResolutionSource: identity.ResolutionSource, RequireStatisticsEvidence: true, EvidenceID: identity.EvidenceID, EvidenceHash: identity.EvidenceHash, EvidenceVersion: identity.EvidenceVersion, RequestID: identity.RequestID})
			}
			if err != nil {
				return err
			}
		} else {
			if err := lockTaskMutationSubjectsForReplay(tx, reserve); err != nil {
				return err
			}
			if err := lockForUpdate(tx).Where("id = ?", identity.OperationID).First(&operation).Error; err != nil {
				return err
			}
		}
		if err := validateTerminalReceiptAgainstObservation(receipt, reserve, &identity); err != nil {
			return err
		}
		outbox, err := ensureTaskMutationOutbox(tx, receipt)
		if err != nil {
			return err
		}
		if err := tx.Where("id = ?", identity.OperationID).First(&operation).Error; err != nil {
			return err
		}
		if (identity.Outcome == "succeeded" && operation.Status != TaskSubmissionOperationStatusSucceeded) || (identity.Outcome == "failed" && operation.Status != TaskSubmissionOperationStatusFailed) {
			return ErrTaskRecoveryInvalidRecord
		}
		if err := lockForUpdate(tx).Where("id = ? AND task_id = ?", identity.TaskID, operation.PublicID).First(&task).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("id = ?", identity.ID).First(&observation).Error; err != nil {
			return err
		}
		if err := validateStoredTaskTerminalObservation(&observation); err != nil {
			return err
		}
		if observation.Fingerprint != identity.Fingerprint || observation.OperationID != operation.ID || observation.TaskID != task.ID {
			return ErrTaskRecoveryInvalidRecord
		}
		if observation.State == TaskTerminalObservationManualReview {
			return ErrTaskTerminalObservationManualReview
		}
		finalQuota := 0
		if identity.Outcome == "succeeded" {
			finalQuota = int(receipt.Quota)
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		task.PrivateData.ResultURL = observation.OperationalResultURL
		if observation.TaskUpstreamID != "" {
			task.PrivateData.UpstreamTaskID = observation.TaskUpstreamID
		}
		task.PrivateData.BillingPreference = reserve.BillingPreference
		task.PrivateData.BillingSource = reserve.BillingSource
		task.PrivateData.SubscriptionId = reserve.SubscriptionID
		task.PrivateData.FreeModel = reserve.FreeModel
		task.PrivateData.TokenId = reserve.TokenID
		billingContext := TaskBillingContext(reserve.BillingContext)
		task.PrivateData.BillingContext = &billingContext
		taskData := task.Data
		if observation.TaskData != "" {
			taskData = []byte(observation.TaskData)
		}
		finishTime := observation.TaskFinishTime
		if finishTime == 0 {
			finishTime = now
		}
		updatedTask := tx.Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]interface{}{"status": observation.TaskStatus, "quota": finalQuota, "progress": "100%", "start_time": observation.TaskStartTime, "finish_time": finishTime, "updated_at": now, "data": taskData, "private_data": task.PrivateData, "fail_reason": func() string {
			if identity.Outcome == "failed" {
				return observation.ReasonCode
			}
			return ""
		}()})
		if updatedTask.Error != nil {
			return updatedTask.Error
		}
		if updatedTask.RowsAffected != 1 {
			return ErrTaskQuotaReservationCASLost
		}
		if observation.State == TaskTerminalObservationPending || observation.State == TaskTerminalObservationRetryable {
			appliedAt := now
			cas := taskRecoveryControlledWrite(tx).Table("task_terminal_observations").Where("id = ? AND state = ? AND lock_version = ?", observation.ID, observation.State, observation.LockVersion).Updates(map[string]interface{}{"state": TaskTerminalObservationApplied, "applied_at": appliedAt, "updated_at": now, "next_attempt_at": 0, "last_error_code": "", "lock_version": gorm.Expr("lock_version + 1")})
			if cas.Error != nil {
				return cas.Error
			}
			if cas.RowsAffected != 1 {
				return ErrTaskQuotaReservationCASLost
			}
			observation.State = TaskTerminalObservationApplied
			observation.AppliedAt = &appliedAt
		}
		output = TaskTerminalApplyResult{Observation: &observation, Receipt: receipt, Outbox: outbox}
		return nil
	})
	if err == nil {
		return &output, nil
	}
	if terminalApplyPermanentError(err) {
		if markErr := MarkTaskTerminalObservationManualReview(db, identity.ID, identity.LockVersion); markErr != nil {
			return nil, fmt.Errorf("terminal apply failed: %v; mark manual review: %w", err, markErr)
		}
		return nil, ErrTaskTerminalObservationManualReview
	}
	return nil, err
}

func validateTerminalReceiptAgainstObservation(receipt, reserve *QuotaMutationReceipt, observation *TaskTerminalObservation) error {
	if receipt == nil || reserve == nil || observation == nil || receipt.OperationID != observation.OperationID || receipt.ChannelID != reserve.ChannelID ||
		receipt.RequestID != observation.RequestID || receipt.EvidenceID != observation.EvidenceID || receipt.EvidenceHash != observation.EvidenceHash || receipt.EvidenceVersion != observation.EvidenceVersion {
		return ErrTaskRecoveryInvalidRecord
	}
	if observation.Outcome == "succeeded" {
		if receipt.MutationType != string(TaskBillingEventTypeTerminalSettlement) || receipt.Quota != observation.ActualQuota {
			return ErrTaskRecoveryInvalidRecord
		}
		return nil
	}
	if observation.Outcome != "failed" || observation.ActualQuota != 0 || receipt.MutationType != string(TaskBillingEventTypeRefund) || receipt.Quota != reserve.Quota {
		return ErrTaskRecoveryInvalidRecord
	}
	return nil
}

func terminalApplyPermanentError(err error) bool {
	return errors.Is(err, ErrTaskTerminalObservationManualReview) || errors.Is(err, ErrTaskRecoveryInvalidRecord) || errors.Is(err, ErrTaskQuotaReservationNotFound) ||
		errors.Is(err, ErrTaskQuotaReservationInvalidInput) || errors.Is(err, ErrTaskQuotaReleaseInvalidInput) || errors.Is(err, ErrTaskQuotaSettlementInvalidInput) ||
		errors.Is(err, ErrTaskSubmissionTaskNotFound) || errors.Is(err, ErrTaskSubmissionTaskIDMismatch) || errors.Is(err, gorm.ErrRecordNotFound)
}

func MarkTaskTerminalObservationManualReview(db *gorm.DB, observationID, expectedVersion int64) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	now, err := taskRecoveryDBTimestamp(db)
	if err != nil {
		return err
	}
	res := taskRecoveryControlledWrite(db).Table("task_terminal_observations").Where("id = ? AND state IN ? AND lock_version = ?", observationID, []TaskTerminalObservationState{TaskTerminalObservationPending, TaskTerminalObservationRetryable}, expectedVersion).Updates(map[string]interface{}{"state": TaskTerminalObservationManualReview, "updated_at": now, "next_attempt_at": 0, "lock_version": gorm.Expr("lock_version + 1")})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 1 {
		return nil
	}
	var current TaskTerminalObservation
	if err := db.Where("id = ?", observationID).First(&current).Error; err != nil {
		return err
	}
	if current.State == TaskTerminalObservationApplied || current.State == TaskTerminalObservationManualReview {
		return nil
	}
	return ErrTaskQuotaReservationCASLost
}

func lockTaskMutationSubjectsForReplay(tx *gorm.DB, reserve *QuotaMutationReceipt) error {
	var user User
	if err := lockForUpdate(tx).Where("id = ?", reserve.UserID).First(&user).Error; err != nil {
		return err
	}
	var token Token
	if err := lockForUpdate(tx).Where("id = ?", reserve.TokenID).First(&token).Error; err != nil {
		return err
	}
	if reserve.BillingSource == "subscription" {
		var subscription UserSubscription
		if err := lockForUpdate(tx).Where("id = ?", reserve.SubscriptionID).First(&subscription).Error; err != nil {
			return err
		}
	}
	var operation TaskSubmissionOperation
	if err := lockForUpdate(tx).Where("id = ?", reserve.OperationID).First(&operation).Error; err != nil {
		return err
	}
	var attempt TaskSubmissionAttempt
	return lockForUpdate(tx).Where("operation_id = ?", reserve.OperationID).First(&attempt).Error
}

func ensureTaskMutationOutbox(tx *gorm.DB, receipt *QuotaMutationReceipt) (*TaskBillingLogOutbox, error) {
	if receipt == nil {
		return nil, ErrTaskRecoveryInvalidRecord
	}
	var existing TaskBillingLogOutbox
	err := tx.Where("billing_event_id = ?", receipt.BillingEventID).First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var event TaskBillingEvent
	if err := tx.Where("event_id = ?", receipt.BillingEventID).First(&event).Error; err != nil {
		return nil, err
	}
	return createTaskMutationOutbox(tx, &event, receipt, "task billing mutation")
}

func applyTaskStatistics(tx *gorm.DB, userID, channelID int, delta int64, requestDelta int) error {
	userQuery := tx.Model(&User{}).Where("id = ?", userID)
	if delta > 0 {
		userQuery = userQuery.Where("used_quota <= ?", int64(common.MaxQuota)-delta)
	}
	if delta < 0 {
		userQuery = userQuery.Where("used_quota >= ?", int64(common.MinQuota)-delta)
	}
	updates := map[string]interface{}{"used_quota": gorm.Expr("used_quota + ?", delta)}
	if requestDelta != 0 {
		userQuery = userQuery.Where("request_count <= ?", common.MaxQuota-requestDelta)
		updates["request_count"] = gorm.Expr("request_count + ?", requestDelta)
	}
	userResult := userQuery.Updates(updates)
	if userResult.Error != nil {
		return userResult.Error
	}
	if userResult.RowsAffected != 1 {
		return ErrTaskTerminalObservationManualReview
	}
	query := tx.Model(&Channel{}).Where("id = ?", channelID)
	if delta > 0 {
		query = query.Where("used_quota <= ?", int64(math.MaxInt64)-delta)
	}
	if delta < 0 {
		query = query.Where("used_quota >= ?", int64(math.MinInt64)-delta)
	}
	channelResult := query.Update("used_quota", gorm.Expr("used_quota + ?", delta))
	if channelResult.Error != nil {
		return channelResult.Error
	}
	if channelResult.RowsAffected != 1 {
		return ErrTaskTerminalObservationManualReview
	}
	return nil
}

func (o *TaskTerminalObservation) BeforeCreate(tx *gorm.DB) error {
	if o == nil {
		return ErrTaskRecoveryInvalidRecord
	}
	clampPresent := o.ClampOp != "" || o.ClampKind != "" || o.ClampOriginal != "" || o.ClampClamped != 0
	if o.OperationID <= 0 || o.TaskID < 0 || (o.TaskID == 0 && o.State != TaskTerminalObservationManualReview) || o.ActualQuota < 0 || o.ActualQuota > int64(common.MaxQuota) || !validTaskRecoveryDigest(o.Fingerprint) || o.LockVersion != 1 || o.CreatedAt <= 0 || o.UpdatedAt != o.CreatedAt ||
		(o.State != TaskTerminalObservationPending && o.State != TaskTerminalObservationManualReview) || o.AppliedAt != nil || o.ConflictCount != 0 || o.LastConflictFingerprint != "" || o.AttemptCount != 0 || o.NextAttemptAt != 0 || o.LastErrorCode != "" ||
		!validTaskTerminalReasonCode(o.ReasonCode) || o.RequestID == "" || len(o.RequestID) > 64 || len(o.EvidenceID) > 191 || o.EvidenceVersion < 0 ||
		(o.EvidenceHash != "" && !validTaskRecoveryDigest(o.EvidenceHash)) || (o.EvidenceVersion == 0 && (o.EvidenceID != "" || o.EvidenceHash != "")) ||
		(o.State != TaskTerminalObservationManualReview && (o.ResolutionSource != TaskSubmissionResolutionSourceProviderVerified || o.EvidenceVersion != 1 || o.EvidenceID == "")) || !validSanitizedTerminalResultURL(o.ResultURL) || !validTerminalTaskProjection(o.TaskData, o.TaskStartTime, o.TaskFinishTime, o.OperationalResultURL, o.TaskUpstreamID) || o.RetentionUntil != o.CreatedAt+TaskSubmissionTerminalRetentionSeconds ||
		(clampPresent && (o.ClampOp == "" || o.ClampKind == "" || len(o.ClampOp) > 32 || len(o.ClampKind) > 32 || int64(o.ClampClamped) < int64(common.MinQuota) || int64(o.ClampClamped) > int64(common.MaxQuota))) ||
		(o.Outcome == "succeeded" && o.TaskStatus != TaskStatusSuccess) || (o.Outcome == "failed" && o.TaskStatus != TaskStatusFailure) || (o.Outcome != "succeeded" && o.Outcome != "failed") ||
		(o.ResolutionSource != "" && o.ResolutionSource != TaskSubmissionResolutionSourceProviderVerified && o.ResolutionSource != TaskSubmissionResolutionSourceManualAudit) {
		return ErrTaskRecoveryInvalidRecord
	}
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	var operation TaskSubmissionOperation
	if err := tx.Select("id", "public_id", "task_id").Where("id = ?", o.OperationID).First(&operation).Error; err != nil {
		return err
	}
	if o.TaskID > 0 {
		var task Task
		if err := tx.Select("id", "task_id").Where("id = ?", o.TaskID).First(&task).Error; err != nil {
			return err
		}
		if operation.TaskID == nil || *operation.TaskID != task.ID || operation.PublicID != task.TaskID {
			return ErrTaskSubmissionTaskIDMismatch
		}
	}
	if o.State != TaskTerminalObservationManualReview {
		var attempt TaskSubmissionAttempt
		if err := tx.Select("provider_operation_id", "upstream_request_id").Where("operation_id = ?", o.OperationID).First(&attempt).Error; err != nil {
			return err
		}
		if o.EvidenceID != attempt.ProviderOperationID && o.EvidenceID != attempt.UpstreamRequestID {
			return ErrTaskRecoveryInvalidRecord
		}
	}
	return nil
}
func (*TaskTerminalObservation) BeforeUpdate(_ *gorm.DB) error { return ErrTaskRecoveryInvalidRecord }
func (*TaskTerminalObservation) BeforeDelete(_ *gorm.DB) error { return ErrTaskRecoveryInvalidRecord }

func validTaskTerminalReasonCode(code string) bool {
	if code == "" || len(code) > 64 || code != strings.ToLower(strings.TrimSpace(code)) {
		return false
	}
	for _, c := range code {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '.' && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

func ensureTaskTerminalObservationSchemaWithDB(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&TaskTerminalObservation{}) {
		return nil
	}
	var rows []TaskTerminalObservation
	if err := db.Select("id", "operation_id", "task_id", "request_id", "created_at", "retention_until").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		updates := map[string]interface{}{}
		if row.RequestID == "" {
			updates["request_id"] = stableTaskBillingRequestID("", fmt.Sprintf("terminal:%d:%d", row.OperationID, row.TaskID))
		}
		if row.RetentionUntil == 0 && row.CreatedAt > 0 {
			updates["retention_until"] = row.CreatedAt + TaskSubmissionTerminalRetentionSeconds
		}
		if len(updates) > 0 {
			if err := taskRecoveryMigrationWrite(db).Table("task_terminal_observations").Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func validateStoredTaskTerminalObservation(o *TaskTerminalObservation) error {
	if o == nil {
		return ErrTaskRecoveryInvalidRecord
	}
	clampPresent := o.ClampOp != "" || o.ClampKind != "" || o.ClampOriginal != "" || o.ClampClamped != 0
	if o.ID <= 0 || o.OperationID <= 0 || o.TaskID < 0 || (o.TaskID == 0 && o.State != TaskTerminalObservationManualReview) || o.ActualQuota < 0 || o.ActualQuota > int64(common.MaxQuota) || !validTaskRecoveryDigest(o.Fingerprint) || o.LockVersion <= 0 ||
		o.CreatedAt <= 0 || o.UpdatedAt < o.CreatedAt || o.RetentionUntil != o.CreatedAt+TaskSubmissionTerminalRetentionSeconds || !validTaskTerminalReasonCode(o.ReasonCode) || o.RequestID == "" || len(o.RequestID) > 64 ||
		(o.State != TaskTerminalObservationPending && o.State != TaskTerminalObservationRetryable && o.State != TaskTerminalObservationApplied && o.State != TaskTerminalObservationManualReview) ||
		(o.State == TaskTerminalObservationApplied) != (o.AppliedAt != nil) || o.ConflictCount < 0 || (o.LastConflictFingerprint != "" && !validTaskRecoveryDigest(o.LastConflictFingerprint)) ||
		(o.Outcome == "succeeded" && o.TaskStatus != TaskStatusSuccess) || (o.Outcome == "failed" && o.TaskStatus != TaskStatusFailure) || (o.Outcome != "succeeded" && o.Outcome != "failed") || !validSanitizedTerminalResultURL(o.ResultURL) ||
		len(o.EvidenceID) > 191 || o.EvidenceVersion < 0 || (o.EvidenceHash != "" && !validTaskRecoveryDigest(o.EvidenceHash)) || (o.EvidenceVersion == 0 && (o.EvidenceID != "" || o.EvidenceHash != "")) ||
		(o.State != TaskTerminalObservationManualReview && (o.ResolutionSource != TaskSubmissionResolutionSourceProviderVerified || o.EvidenceVersion != 1 || o.EvidenceID == "")) || !validTerminalTaskProjection(o.TaskData, o.TaskStartTime, o.TaskFinishTime, o.OperationalResultURL, o.TaskUpstreamID) || (clampPresent && (o.ClampOp == "" || o.ClampKind == "")) ||
		(o.ResolutionSource != "" && o.ResolutionSource != TaskSubmissionResolutionSourceProviderVerified && o.ResolutionSource != TaskSubmissionResolutionSourceManualAudit) {
		return ErrTaskRecoveryInvalidRecord
	}
	switch o.State {
	case TaskTerminalObservationPending:
		if o.AttemptCount != 0 || o.NextAttemptAt != 0 || o.LastErrorCode != "" {
			return ErrTaskRecoveryInvalidRecord
		}
	case TaskTerminalObservationRetryable:
		if o.AttemptCount <= 0 || o.NextAttemptAt <= o.UpdatedAt || o.LastErrorCode != "terminal_apply_transient" || o.AppliedAt != nil {
			return ErrTaskRecoveryInvalidRecord
		}
	case TaskTerminalObservationApplied:
		if o.NextAttemptAt != 0 || o.LastErrorCode != "" {
			return ErrTaskRecoveryInvalidRecord
		}
	case TaskTerminalObservationManualReview:
		if o.NextAttemptAt != 0 || o.AppliedAt != nil {
			return ErrTaskRecoveryInvalidRecord
		}
	}
	return nil
}

func MarkTaskTerminalObservationRetryable(db *gorm.DB, observationID, expectedVersion int64, retryDelaySeconds int64) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if retryDelaySeconds <= 0 || retryDelaySeconds > TaskRecoveryMaxRetryDelaySeconds {
		return ErrTaskRecoveryInvalidRecord
	}
	now, err := taskRecoveryDBTimestamp(db)
	if err != nil {
		return err
	}
	if now > taskRecoveryMaxInt64-retryDelaySeconds {
		return ErrTaskRecoveryInvalidRecord
	}
	retryAt := now + retryDelaySeconds
	res := taskRecoveryControlledWrite(db).Table("task_terminal_observations").Where("id = ? AND state IN ? AND lock_version = ? AND attempt_count < ?", observationID, []TaskTerminalObservationState{TaskTerminalObservationPending, TaskTerminalObservationRetryable}, expectedVersion, common.MaxQuota).Updates(map[string]interface{}{"state": TaskTerminalObservationRetryable, "attempt_count": gorm.Expr("attempt_count + 1"), "next_attempt_at": retryAt, "last_error_code": "terminal_apply_transient", "updated_at": now, "lock_version": gorm.Expr("lock_version + 1")})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 1 {
		return nil
	}
	var current TaskTerminalObservation
	if err := db.Where("id = ?", observationID).First(&current).Error; err != nil {
		return err
	}
	if current.State == TaskTerminalObservationApplied || current.State == TaskTerminalObservationManualReview || current.State == TaskTerminalObservationRetryable {
		return nil
	}
	return ErrTaskQuotaReservationCASLost
}

func validTerminalTaskProjection(data string, startTime, finishTime int64, operationalResultURL, upstreamTaskID string) bool {
	if startTime < 0 || finishTime < 0 || (startTime > 0 && finishTime > 0 && finishTime < startTime) || len(data) > taskTerminalObservationMaxTaskDataBytes || len(operationalResultURL) > taskTerminalObservationMaxOperationalURLBytes || len(upstreamTaskID) > 191 {
		return false
	}
	if data != "" {
		var decoded interface{}
		if err := common.Unmarshal([]byte(data), &decoded); err != nil {
			return false
		}
	}
	if operationalResultURL == "" {
		return true
	}
	parsed, err := url.Parse(operationalResultURL)
	if err != nil || parsed.User != nil {
		return false
	}
	if parsed.IsAbs() {
		return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
	}
	return parsed.Host == "" && strings.HasPrefix(parsed.Path, "/")
}

func validSanitizedTerminalResultURL(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 4096 {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.User == nil && parsed.IsAbs() && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && parsed.RawQuery == "" && parsed.Fragment == "" && parsed.RawFragment == "" && !parsed.ForceQuery
}

// EnsureTaskMutationOutbox reconstructs a missing projection only from a
// validated immutable receipt and its authoritative event.
func EnsureTaskMutationOutbox(tx *gorm.DB, receipt *QuotaMutationReceipt) (*TaskBillingLogOutbox, error) {
	return ensureTaskMutationOutbox(tx, receipt)
}
