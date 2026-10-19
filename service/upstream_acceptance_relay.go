package service

import (
	"context"
	"time"

	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
)

const upstreamAcceptanceRelayFinishKey = "upstream_acceptance_operation_finish"

// The request-level finisher lives until the relay handler has also finished
// its normal logging/settlement defers; HTTP bodies have their own drain slots.
func BindUpstreamAcceptanceRelayOperation(c *gin.Context, channelID int, accountID string) error {
	if !UpstreamAcceptanceModeEnabled() {
		return nil
	}
	ctx, finish, err := BeginUpstreamAcceptanceOperation(c.Request.Context(), channelID, accountID)
	if err != nil {
		return err
	}
	c.Request = c.Request.WithContext(ctx)
	if _, exists := c.Get(upstreamAcceptanceRelayFinishKey); !exists {
		c.Set(upstreamAcceptanceRelayFinishKey, finish)
	}
	return nil
}

func FinishUpstreamAcceptanceRelayOperation(c *gin.Context) {
	if value, ok := c.Get(upstreamAcceptanceRelayFinishKey); ok {
		value.(func())()
	}
}

// This early check avoids normal dispatch bookkeeping for already exhausted
// runs. It does not reserve a request: the final atomic claim still decides.
func CheckUpstreamAcceptanceDispatch(ctx context.Context, kind string) error {
	if !UpstreamAcceptanceModeEnabled() {
		return nil
	}
	upstreamAcceptance.Lock()
	defer upstreamAcceptance.Unlock()
	op, ok := ctx.Value(upstreamAcceptanceOperationKey{}).(*upstreamAcceptanceOperation)
	current := upstreamAcceptance.run
	if !ok || current == nil || op.run != current || current.stopping || current.drainError ||
		current.owner != upstreamAcceptance.owner || current.ctx.Err() != nil || ctx.Err() != nil {
		return ErrUpstreamAcceptanceDenied
	}
	readCtx, cancel := context.WithTimeout(ctx, upstreamAcceptanceWriteTimeout)
	defer cancel()
	run, err := model.GetUpstreamAcceptanceRun(readCtx, current.id)
	if err != nil || run.OwnerID != current.owner || run.Status != model.UpstreamAcceptanceRunActive ||
		run.DeadlineAtUnix <= time.Now().Unix() || run.Total >= run.MaxTotal {
		return ErrUpstreamAcceptanceDenied
	}
	switch kind {
	case model.UpstreamAcceptanceDispatchGeneration:
		if run.Generation >= run.MaxGeneration || run.GenerationInFlight != 0 {
			return ErrUpstreamAcceptanceDenied
		}
	case model.UpstreamAcceptanceDispatchSample:
		if run.Sample >= run.MaxSample {
			return ErrUpstreamAcceptanceDenied
		}
	default:
		return ErrUpstreamAcceptanceDenied
	}
	return nil
}
