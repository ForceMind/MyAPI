package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"

	"gorm.io/gorm"
)

const DefaultLogProjectionBackfillBatchSize = 100

var (
	startClickHouseProjectionMaterialize = model.StartClickHouseCanonicalProjectionMaterialize
	findClickHouseProjectionMutations    = model.FindClickHouseCanonicalProjectionMutations
)

type LogProjectionBackfillBatchResult struct {
	Phase                 string `json:"phase"`
	Status                string `json:"status"`
	Processed             int    `json:"processed"`
	LastID                int    `json:"last_id"`
	LastEventID           string `json:"last_event_id"`
	QuarantinedEvents     int64  `json:"quarantined_events,omitempty"`
	MaterializeMutationID string `json:"materialize_mutation_id,omitempty"`
	MaterializeStatus     string `json:"materialize_status,omitempty"`
	NoOp                  bool   `json:"no_op,omitempty"`
}

func ShouldScheduleLogProjectionBackfill(ctx context.Context, mainDB *gorm.DB) bool {
	if !common.IsTaskRecoveryObligationRecoveryEnabled() || mainDB == nil {
		return false
	}
	state, err := model.GetLogProjectionBackfillState(ctx, mainDB)
	if err != nil {
		common.SysError("load log projection backfill schedule state: " + err.Error())
		return false
	}
	return !model.IsLogProjectionBackfillTerminal(state)
}

func RunLogProjectionBackfillBatch(ctx context.Context, mainDB *gorm.DB, logDB *gorm.DB, taskID string, runnerID string, fenceToken int64, limit int) (LogProjectionBackfillBatchResult, error) {
	if mainDB == nil || logDB == nil || logDB.Dialector == nil || taskID == "" || runnerID == "" || fenceToken <= 0 {
		return LogProjectionBackfillBatchResult{}, gorm.ErrInvalidDB
	}
	if limit <= 0 {
		limit = DefaultLogProjectionBackfillBatchSize
	}
	if limit > 1000 {
		limit = 1000
	}

	state, err := model.GetOrCreateLogProjectionBackfillState(ctx, mainDB, logDB)
	if err != nil {
		return LogProjectionBackfillBatchResult{}, err
	}
	if model.IsLogProjectionBackfillTerminal(state) {
		return logProjectionBackfillResult(state, 0, true), nil
	}
	save := func() error {
		return model.SaveLogProjectionBackfillStateFenced(ctx, mainDB, state, taskID, runnerID, fenceToken, state.LockVersion)
	}
	fail := func(runErr error, manualReview bool, processed int) (LogProjectionBackfillBatchResult, error) {
		state.Status = model.LogProjectionBackfillStatusFailed
		if errors.Is(runErr, model.ErrLogProjectionMaintenanceRequired) {
			state.Status = model.LogProjectionBackfillStatusMaintenanceRequired
		} else if manualReview {
			state.Status = model.LogProjectionBackfillStatusManualReview
		}
		state.LastError = runErr.Error()
		if saveErr := save(); saveErr != nil {
			return logProjectionBackfillResult(state, processed, false), errors.Join(runErr, saveErr)
		}
		return logProjectionBackfillResult(state, processed, false), runErr
	}
	if state.SchemaVersion != model.LogProjectionBackfillSchemaVersion {
		return fail(fmt.Errorf("unsupported log projection backfill schema version %d", state.SchemaVersion), true, 0)
	}

	state.Status = model.LogProjectionBackfillStatusRunning
	state.LastError = ""
	if err := save(); err != nil {
		return logProjectionBackfillResult(state, 0, false), err
	}

	dialect := logDB.Dialector.Name()
	switch state.Phase {
	case model.LogProjectionBackfillPhaseRelationalIdentity:
		if dialect == string(common.DatabaseTypeClickHouse) {
			return fail(errors.New("relational backfill state cannot run against ClickHouse"), true, 0)
		}
		next, processed, runErr := model.BackfillLogProjectionIdentityBatch(ctx, logDB, *state, limit)
		state.LastID = next.LastID
		state.QuarantinedEvents = next.QuarantinedEvents
		if runErr != nil {
			return fail(runErr, false, processed)
		}
		if next.Complete {
			state.Phase = model.LogProjectionBackfillPhaseRelationalIndexes
		}
		state.Status = model.LogProjectionBackfillStatusPending
		if err := save(); err != nil {
			return logProjectionBackfillResult(state, processed, false), err
		}
		return logProjectionBackfillResult(state, processed, false), nil

	case model.LogProjectionBackfillPhaseRelationalIndexes:
		if dialect == string(common.DatabaseTypeClickHouse) {
			return fail(errors.New("relational index state cannot run against ClickHouse"), true, 0)
		}
		done, manualReview, runErr := model.CreateNextLogProjectionIndex(ctx, logDB)
		if runErr != nil {
			return fail(runErr, manualReview, 0)
		}
		if done {
			markLogProjectionBackfillCompleted(state)
		} else {
			state.Status = model.LogProjectionBackfillStatusPending
		}
		if err := save(); err != nil {
			return logProjectionBackfillResult(state, 0, false), err
		}
		return logProjectionBackfillResult(state, 0, false), nil

	case model.LogProjectionBackfillPhaseClickHouseIdentity:
		if dialect != string(common.DatabaseTypeClickHouse) {
			return fail(errors.New("ClickHouse backfill state requires a ClickHouse log database"), true, 0)
		}
		next, processed, runErr := model.BackfillClickHouseProjectionIdentityBatch(ctx, logDB, *state, limit)
		state.LastEventID = next.LastEventID
		state.QuarantinedEvents = next.QuarantinedEvents
		if runErr != nil {
			return fail(runErr, false, processed)
		}
		if next.Complete {
			if state.LastEventID == "" {
				markLogProjectionBackfillCompleted(state)
				state.MaterializeStatus = model.LogProjectionMaterializeStatusCompleted
			} else {
				state.Phase = model.LogProjectionBackfillPhaseClickHouseMaterial
				state.Status = model.LogProjectionBackfillStatusPending
				state.MaterializeStatus = model.LogProjectionMaterializeStatusPending
			}
		} else {
			state.Status = model.LogProjectionBackfillStatusPending
		}
		if err := save(); err != nil {
			return logProjectionBackfillResult(state, processed, false), err
		}
		return logProjectionBackfillResult(state, processed, false), nil

	case model.LogProjectionBackfillPhaseClickHouseMaterial:
		if dialect != string(common.DatabaseTypeClickHouse) {
			return fail(errors.New("ClickHouse materialization state requires a ClickHouse log database"), true, 0)
		}
		if state.MaterializeMutationID == "" && state.MaterializeStatus == model.LogProjectionMaterializeStatusStarting {
			mutations, runErr := findClickHouseProjectionMutations(ctx, logDB, state.MaterializeRequestedAt)
			if runErr != nil {
				return fail(runErr, false, 0)
			}
			mutation, identifyErr := identifyClickHouseProjectionMutation(state, mutations)
			if identifyErr != nil {
				return fail(identifyErr, true, 0)
			}
			state.MaterializeMutationID = mutation.MutationID
			state.MaterializeStatus = model.LogProjectionMaterializeStatusRunning
			state.Status = model.LogProjectionBackfillStatusPending
			if err := save(); err != nil {
				return logProjectionBackfillResult(state, 0, false), err
			}
			return logProjectionBackfillResult(state, 0, false), nil
		}
		if state.MaterializeMutationID == "" {
			state.MaterializeGeneration = time.Now().UnixNano()
			state.MaterializeRequestedAt = time.Now().UnixMilli()
			state.MaterializeStatus = model.LogProjectionMaterializeStatusStarting
			state.Status = model.LogProjectionBackfillStatusPending
			if err := save(); err != nil {
				return logProjectionBackfillResult(state, 0, false), err
			}
			if runErr := startClickHouseProjectionMaterialize(ctx, logDB); runErr != nil {
				return fail(runErr, true, 0)
			}
			mutations, runErr := findClickHouseProjectionMutations(ctx, logDB, state.MaterializeRequestedAt)
			if runErr != nil {
				return fail(runErr, false, 0)
			}
			mutation, identifyErr := identifyClickHouseProjectionMutation(state, mutations)
			if identifyErr != nil {
				return fail(identifyErr, true, 0)
			}
			state.MaterializeMutationID = mutation.MutationID
			state.MaterializeStatus = model.LogProjectionMaterializeStatusRunning
			state.Status = model.LogProjectionBackfillStatusPending
			if err := save(); err != nil {
				return logProjectionBackfillResult(state, 0, false), err
			}
			return logProjectionBackfillResult(state, 0, false), nil
		}

		mutation, runErr := model.GetClickHouseCanonicalProjectionMutation(ctx, logDB, state.MaterializeMutationID)
		if runErr != nil {
			return fail(runErr, false, 0)
		}
		manualReview, reconcileErr := reconcileClickHouseProjectionMutation(state, mutation)
		if reconcileErr != nil {
			return fail(reconcileErr, manualReview, 0)
		}
		if err := save(); err != nil {
			return logProjectionBackfillResult(state, 0, false), err
		}
		return logProjectionBackfillResult(state, 0, false), nil

	case model.LogProjectionBackfillPhaseCompleted:
		markLogProjectionBackfillCompleted(state)
		if err := save(); err != nil {
			return logProjectionBackfillResult(state, 0, false), err
		}
		return logProjectionBackfillResult(state, 0, true), nil
	default:
		return fail(fmt.Errorf("unknown log projection backfill phase %q", state.Phase), true, 0)
	}
}

func identifyClickHouseProjectionMutation(state *model.LogProjectionBackfillState, mutations []model.ClickHouseProjectionMutation) (*model.ClickHouseProjectionMutation, error) {
	if state == nil || state.MaterializeGeneration == 0 || state.MaterializeRequestedAt == 0 {
		return nil, errors.New("ClickHouse projection materialization generation is incomplete")
	}
	if len(mutations) != 1 || mutations[0].MutationID == "" {
		return nil, fmt.Errorf("ClickHouse projection materialization generation %d has %d identifiable mutations", state.MaterializeGeneration, len(mutations))
	}
	return &mutations[0], nil
}

func reconcileClickHouseProjectionMutation(state *model.LogProjectionBackfillState, mutation *model.ClickHouseProjectionMutation) (bool, error) {
	if mutation == nil || mutation.MutationID == "" {
		state.MaterializeStatus = model.LogProjectionMaterializeStatusFailed
		return true, errors.New("ClickHouse projection mutation status is unavailable")
	}
	if mutation.LatestFailReason != "" {
		state.MaterializeStatus = model.LogProjectionMaterializeStatusFailed
		return true, errors.New(mutation.LatestFailReason)
	}
	if mutation.IsDone != 0 {
		state.MaterializeStatus = model.LogProjectionMaterializeStatusCompleted
		markLogProjectionBackfillCompleted(state)
		return false, nil
	}
	state.MaterializeStatus = model.LogProjectionMaterializeStatusRunning
	state.Status = model.LogProjectionBackfillStatusPending
	return false, nil
}

func markLogProjectionBackfillCompleted(state *model.LogProjectionBackfillState) {
	state.Phase = model.LogProjectionBackfillPhaseCompleted
	state.Status = model.LogProjectionBackfillStatusCompleted
	state.LastError = ""
	if state.CompletedAt == 0 {
		state.CompletedAt = common.GetTimestamp()
	}
}

func logProjectionBackfillResult(state *model.LogProjectionBackfillState, processed int, noOp bool) LogProjectionBackfillBatchResult {
	return LogProjectionBackfillBatchResult{
		Phase:                 state.Phase,
		Status:                state.Status,
		Processed:             processed,
		LastID:                state.LastID,
		LastEventID:           state.LastEventID,
		QuarantinedEvents:     state.QuarantinedEvents,
		MaterializeMutationID: state.MaterializeMutationID,
		MaterializeStatus:     state.MaterializeStatus,
		NoOp:                  noOp,
	}
}
