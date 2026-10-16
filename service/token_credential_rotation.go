package service

import (
	"context"

	"github.com/ForceMind/MyAPI/model"
)

func PreviewTokenCredentialRotation(ctx context.Context, actorID, tokenID int) (*model.TokenCredentialRotationPreview, error) {
	now, err := model.ReadDatabaseUnixTime(ctx, model.DB)
	if err != nil {
		return nil, err
	}
	return model.PreviewTokenCredentialRotation(ctx, model.DB, actorID, tokenID, now)
}

func CommitTokenCredentialRotation(ctx context.Context, actorID, tokenID int, command model.TokenCredentialRotationCommand) (*model.TokenCredentialRotationOperationView, string, bool, error) {
	now, err := model.ReadDatabaseUnixTime(ctx, model.DB)
	if err != nil {
		return nil, "", false, err
	}
	return model.CommitTokenCredentialRotation(ctx, model.DB, actorID, tokenID, command, now)
}

func GetTokenCredentialRotationOperation(ctx context.Context, actorID, tokenID int, operationID string) (*model.TokenCredentialRotationOperationView, error) {
	return model.ReadTokenCredentialRotationOperation(ctx, model.DB, actorID, tokenID, operationID)
}
