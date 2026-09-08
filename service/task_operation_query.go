package service

import (
	"errors"

	"github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/model"
)

var (
	ErrTaskOperationNotFound    = errors.New("task operation not found")
	ErrTaskOperationUnavailable = errors.New("task operation unavailable")
)

// GetTaskOperation returns the public owner-scoped view of one durable task
// submission. A nil tokenID scopes by dashboard user only; an API token must
// match both user and token IDs.
func GetTaskOperation(publicID string, userID int, tokenID *int) (*dto.TaskOperationResponse, error) {
	operation, err := model.ReadTaskOperationForOwner(publicID, userID, tokenID)
	if err != nil {
		return nil, ErrTaskOperationUnavailable
	}
	if operation == nil {
		return nil, ErrTaskOperationNotFound
	}
	return &dto.TaskOperationResponse{
		ID:                operation.PublicID,
		Object:            dto.TaskOperationObject,
		Kind:              operation.OperationKind,
		Status:            string(operation.Status),
		CreatedAt:         operation.CreatedAt,
		UpdatedAt:         operation.UpdatedAt,
		DispatchStartedAt: operation.DispatchStartedAt,
		ResolvedAt:        operation.ResolvedAt,
	}, nil
}
