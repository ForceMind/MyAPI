package service

import (
	"context"

	"github.com/ForceMind/MyAPI/model"
)

func GetTokenMaintenance(ctx context.Context, actorID, tokenID int) (*model.TokenMaintenanceView, error) {
	now, err := model.ReadDatabaseUnixTime(ctx, model.DB)
	if err != nil {
		return nil, err
	}
	return model.ReadTokenMaintenance(ctx, model.DB, actorID, tokenID, now)
}

func PreviewTokenAccountMigration(ctx context.Context, actorID, tokenID int, input model.TokenAccountMigrationPreviewInput) (*model.TokenAccountMigrationPreview, error) {
	now, err := model.ReadDatabaseUnixTime(ctx, model.DB)
	if err != nil {
		return nil, err
	}
	return model.PreviewTokenAccountMigration(ctx, model.DB, actorID, tokenID, input, now)
}

func CommitTokenAccountMigration(ctx context.Context, actorID, tokenID int, command model.TokenAccountMigrationCommand) (*model.TokenMaintenanceOperationView, bool, error) {
	now, err := model.ReadDatabaseUnixTime(ctx, model.DB)
	if err != nil {
		return nil, false, err
	}
	return model.CommitTokenAccountMigration(ctx, model.DB, actorID, tokenID, command, now)
}

func GetTokenMaintenanceOperation(ctx context.Context, actorID, tokenID int, operationID string) (*model.TokenMaintenanceOperationView, error) {
	return model.ReadTokenMaintenanceOperation(ctx, model.DB, actorID, tokenID, operationID)
}
