package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"gorm.io/gorm"
)

// TaskPollingContext is the immutable durable metadata loaded for one public
// task ID. Polling code must use this snapshot instead of re-reading mutable
// Task billing fields or issuing per-task durable metadata queries.
type TaskPollingContext struct {
	Operation      *model.TaskSubmissionOperation
	Attempt        *model.TaskSubmissionAttempt
	ReserveReceipt *model.QuotaMutationReceipt
	ValidationErr  error
}

func (c *TaskPollingContext) Durable() bool {
	return c != nil && c.Operation != nil
}

type TaskPollingContextSet struct {
	durableSchema bool
	byPublicID    map[string]*TaskPollingContext
}

func (set *TaskPollingContextSet) ForTask(task *model.Task) *TaskPollingContext {
	if set == nil || task == nil {
		return &TaskPollingContext{}
	}
	if context, ok := set.byPublicID[task.TaskID]; ok {
		return context
	}
	return &TaskPollingContext{}
}

type TaskPollingSchemaCapability struct {
	Durable bool
}

func InspectTaskPollingSchema(db *gorm.DB) (TaskPollingSchemaCapability, error) {
	if db == nil {
		return TaskPollingSchemaCapability{}, gorm.ErrInvalidDB
	}
	tableNames, err := db.Migrator().GetTables()
	if err != nil {
		return TaskPollingSchemaCapability{}, fmt.Errorf("inspect task polling schema: %w", err)
	}
	existingTables := make(map[string]struct{}, len(tableNames))
	for _, tableName := range tableNames {
		existingTables[strings.ToLower(strings.TrimSpace(tableName))] = struct{}{}
	}
	requiredTables := []string{"task_submission_operations", "task_submission_attempts", "quota_mutation_receipts"}
	present := 0
	for _, tableName := range requiredTables {
		if _, ok := existingTables[tableName]; ok {
			present++
		}
	}
	if present == 0 {
		return TaskPollingSchemaCapability{}, nil
	}
	if present != len(requiredTables) {
		return TaskPollingSchemaCapability{}, errors.New("task polling durable schema is incomplete")
	}
	return TaskPollingSchemaCapability{Durable: true}, nil
}

// LoadTaskPollingContexts performs one error-reporting schema capability check
// and three batch reads. A confirmed pre-durable schema is legacy-only; partial
// schema and probe/query failures are fail-closed.
func LoadTaskPollingContexts(db *gorm.DB, tasks []*model.Task) (*TaskPollingContextSet, error) {
	capability, err := InspectTaskPollingSchema(db)
	if err != nil {
		return nil, err
	}
	return loadTaskPollingContextsWithCapability(db, tasks, capability)
}

func loadTaskPollingContextsWithCapability(db *gorm.DB, tasks []*model.Task, capability TaskPollingSchemaCapability) (*TaskPollingContextSet, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	set := &TaskPollingContextSet{byPublicID: make(map[string]*TaskPollingContext, len(tasks))}
	for _, task := range tasks {
		if task != nil && strings.TrimSpace(task.TaskID) != "" {
			set.byPublicID[task.TaskID] = &TaskPollingContext{}
		}
	}
	if !capability.Durable {
		return set, nil
	}
	set.durableSchema = true
	if len(set.byPublicID) == 0 {
		return set, nil
	}

	publicIDs := make([]string, 0, len(set.byPublicID))
	for publicID := range set.byPublicID {
		publicIDs = append(publicIDs, publicID)
	}
	var operations []model.TaskSubmissionOperation
	if err := db.Where("public_id IN ?", publicIDs).Find(&operations).Error; err != nil {
		return nil, fmt.Errorf("load task polling operations: %w", err)
	}
	operationByID := make(map[int64]*model.TaskSubmissionOperation, len(operations))
	for i := range operations {
		operation := &operations[i]
		context, ok := set.byPublicID[operation.PublicID]
		if !ok {
			return nil, errors.New("task polling operation returned an unexpected public id")
		}
		if context.Operation != nil {
			return nil, errors.New("duplicate task polling operation public id")
		}
		context.Operation = operation
		operationByID[operation.ID] = operation
	}
	if len(operationByID) == 0 {
		return set, nil
	}

	operationIDs := make([]int64, 0, len(operationByID))
	for operationID := range operationByID {
		operationIDs = append(operationIDs, operationID)
	}
	attemptByOperation := make(map[int64]*model.TaskSubmissionAttempt, len(operationIDs))
	var attempts []model.TaskSubmissionAttempt
	if err := db.Where("operation_id IN ?", operationIDs).Find(&attempts).Error; err != nil {
		return nil, fmt.Errorf("load task polling attempts: %w", err)
	}
	for i := range attempts {
		attempt := &attempts[i]
		if _, ok := operationByID[attempt.OperationID]; !ok {
			return nil, errors.New("task polling attempt returned an unexpected operation id")
		}
		if attemptByOperation[attempt.OperationID] != nil {
			return nil, errors.New("duplicate task polling attempt")
		}
		attemptByOperation[attempt.OperationID] = attempt
	}

	receiptByOperation := make(map[int64]*model.QuotaMutationReceipt, len(operationIDs))
	var receipts []model.QuotaMutationReceipt
	if err := db.Where("operation_id IN ? AND mutation_type = ?", operationIDs, string(model.TaskBillingEventTypeReserve)).Find(&receipts).Error; err != nil {
		return nil, fmt.Errorf("load task polling reserve receipts: %w", err)
	}
	for i := range receipts {
		receipt := &receipts[i]
		if _, ok := operationByID[receipt.OperationID]; !ok {
			return nil, errors.New("task polling receipt returned an unexpected operation id")
		}
		if receiptByOperation[receipt.OperationID] != nil {
			return nil, errors.New("duplicate task polling reserve receipt")
		}
		receiptByOperation[receipt.OperationID] = receipt
	}

	taskByPublicID := make(map[string]*model.Task, len(tasks))
	for _, task := range tasks {
		if task != nil && task.TaskID != "" {
			if taskByPublicID[task.TaskID] != nil && taskByPublicID[task.TaskID].ID != task.ID {
				return nil, errors.New("duplicate public task id in polling batch")
			}
			taskByPublicID[task.TaskID] = task
		}
	}
	for _, operation := range operationByID {
		context := set.byPublicID[operation.PublicID]
		context.Attempt = attemptByOperation[operation.ID]
		context.ReserveReceipt = receiptByOperation[operation.ID]
		context.ValidationErr = validateTaskPollingContext(taskByPublicID[operation.PublicID], context)
	}
	return set, nil
}

func validateTaskPollingContext(task *model.Task, context *TaskPollingContext) error {
	if task == nil || context == nil || context.Operation == nil {
		return errors.New("task polling context is incomplete")
	}
	operation := context.Operation
	attempt := context.Attempt
	receipt := context.ReserveReceipt
	if operation.TaskID == nil || *operation.TaskID != task.ID || operation.PublicID != task.TaskID || operation.UserID != task.UserId {
		return errors.New("task polling operation does not match task")
	}
	if attempt == nil || attempt.OperationID != operation.ID || attempt.ChannelID != task.ChannelId || attempt.Status != model.TaskSubmissionAttemptStatusAccepted {
		return errors.New("task polling attempt does not match task")
	}
	expectedUpstreamID := strings.TrimSpace(task.PrivateData.UpstreamTaskID)
	if expectedUpstreamID != "" && expectedUpstreamID != strings.TrimSpace(attempt.ProviderOperationID) {
		return errors.New("task polling upstream id does not match accepted attempt")
	}
	if attempt.TaskPlatform != "" && attempt.TaskPlatform != strings.ToLower(strings.TrimSpace(string(task.Platform))) {
		return errors.New("task polling platform does not match accepted attempt")
	}
	if receipt == nil || receipt.OperationID != operation.ID || receipt.OperationPublicID != operation.PublicID || receipt.UserID != operation.UserID || receipt.TokenID != operation.TokenID || receipt.ChannelID != attempt.ChannelID || receipt.MutationType != string(model.TaskBillingEventTypeReserve) {
		return errors.New("task polling reserve receipt does not match operation")
	}
	if receipt.OperationRequestFingerprint != operation.RequestFingerprint || receipt.RequestFingerprintVersion != operation.BillingVersion || receipt.EstimatedQuota != operation.EstimatedQuota || receipt.Quota != operation.ReservedQuota || receipt.FreeModel != operation.FreeModel || receipt.BillingSource != operation.BillingSource || receipt.SubscriptionID != operation.SubscriptionID || receipt.BillingPreference != operation.BillingPreference {
		return errors.New("task polling immutable billing metadata is inconsistent")
	}
	if receipt.RequestFingerprintVersion != 2 && receipt.RequestFingerprintVersion != 3 {
		return errors.New("task polling billing version is unsupported")
	}
	if receipt.EstimatedQuota < 0 || receipt.EstimatedQuota > int64(common.MaxQuota) || receipt.Quota < 0 || receipt.Quota > int64(common.MaxQuota) {
		return errors.New("task polling immutable quota is invalid")
	}
	return nil
}

func LoadTaskPollingContext(db *gorm.DB, task *model.Task) (*TaskPollingContext, error) {
	set, err := LoadTaskPollingContexts(db, []*model.Task{task})
	if err != nil {
		return nil, err
	}
	return set.ForTask(task), nil
}

type taskPollingKey struct {
	ChannelID  int
	UpstreamID string
}

type taskPollingEntry struct {
	Task    *model.Task
	Context *TaskPollingContext
}

type taskPollingBatch struct {
	channelTasks map[int][]taskPollingKey
	entries      map[taskPollingKey]*taskPollingEntry
}

func newTaskPollingBatch(tasks []*model.Task, contexts *TaskPollingContextSet) (*taskPollingBatch, map[taskPollingKey][]*model.Task) {
	batch := &taskPollingBatch{
		channelTasks: make(map[int][]taskPollingKey),
		entries:      make(map[taskPollingKey]*taskPollingEntry),
	}
	duplicates := make(map[taskPollingKey][]*model.Task)
	for _, task := range tasks {
		if task == nil {
			continue
		}
		upstreamID := strings.TrimSpace(task.GetUpstreamTaskID())
		if upstreamID == "" {
			continue
		}
		key := taskPollingKey{ChannelID: task.ChannelId, UpstreamID: upstreamID}
		if existing := batch.entries[key]; existing != nil {
			if len(duplicates[key]) == 0 {
				duplicates[key] = append(duplicates[key], existing.Task)
			}
			duplicates[key] = append(duplicates[key], task)
			delete(batch.entries, key)
			continue
		}
		if len(duplicates[key]) != 0 {
			duplicates[key] = append(duplicates[key], task)
			continue
		}
		batch.entries[key] = &taskPollingEntry{Task: task, Context: contexts.ForTask(task)}
	}
	for key := range batch.entries {
		batch.channelTasks[key.ChannelID] = append(batch.channelTasks[key.ChannelID], key)
	}
	return batch, duplicates
}

func pollingBatchFromLegacyMaps(taskChannelM map[int][]string, taskM map[string]*model.Task, contexts *TaskPollingContextSet) (*taskPollingBatch, error) {
	batch := &taskPollingBatch{channelTasks: make(map[int][]taskPollingKey), entries: make(map[taskPollingKey]*taskPollingEntry)}
	for channelID, upstreamIDs := range taskChannelM {
		seen := make(map[string]struct{}, len(upstreamIDs))
		for _, upstreamID := range upstreamIDs {
			upstreamID = strings.TrimSpace(upstreamID)
			if upstreamID == "" {
				return nil, errors.New("empty upstream id in polling channel batch")
			}
			if _, ok := seen[upstreamID]; ok {
				return nil, errors.New("duplicate upstream id in polling channel batch")
			}
			seen[upstreamID] = struct{}{}
			task := taskM[upstreamID]
			if task == nil {
				return nil, fmt.Errorf("task %s not found in polling map", upstreamID)
			}
			if task.ChannelId != channelID {
				return nil, fmt.Errorf("task %s channel mismatch", task.TaskID)
			}
			if strings.TrimSpace(task.GetUpstreamTaskID()) != upstreamID {
				return nil, fmt.Errorf("task %s upstream id mismatch", task.TaskID)
			}
			key := taskPollingKey{ChannelID: channelID, UpstreamID: upstreamID}
			batch.channelTasks[channelID] = append(batch.channelTasks[channelID], key)
			batch.entries[key] = &taskPollingEntry{Task: task, Context: contexts.ForTask(task)}
		}
	}
	return batch, nil
}
