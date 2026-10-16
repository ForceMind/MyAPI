package service

import (
	"context"

	"github.com/ForceMind/MyAPI/model"
)

func GetTokenModelLimits(ctx context.Context, userID, tokenID int) (*model.TokenModelLimitsView, error) {
	return model.ReadTokenModelLimits(ctx, userID, tokenID)
}

func SetTokenModelLimits(ctx context.Context, userID, tokenID int, command model.TokenModelLimitsCommand) (*model.TokenModelLimitsView, error) {
	return model.UpdateTokenModelLimits(ctx, userID, tokenID, command)
}
