package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidateTokenCredentialDispatchRequiresCurrentGeneration(t *testing.T) {
	priorDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = priorDB })
	require.NoError(t, db.AutoMigrate(&model.Token{}))

	token := model.Token{UserId: 71, Key: "credential-dispatch-fixture", Status: common.TokenStatusEnabled, CredentialVersion: 4}
	require.NoError(t, db.Create(&token).Error)
	info := &relaycommon.RelayInfo{TokenId: token.Id, UserId: token.UserId, TokenCredentialVersion: 4}

	require.NoError(t, ValidateTokenCredentialDispatch(context.Background(), info))
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("credential_version", 5).Error)
	require.ErrorIs(t, ValidateTokenCredentialDispatch(context.Background(), info), model.ErrTokenCredentialVersionChanged)

	info.TokenCredentialVersion = 0
	require.ErrorIs(t, ValidateTokenCredentialDispatch(context.Background(), info), model.ErrTokenCredentialVersionChanged)
	info.TokenCredentialVersion = 5
	info.UserId++
	err = ValidateTokenCredentialDispatch(context.Background(), info)
	require.Error(t, err)
	require.False(t, errors.Is(err, model.ErrTokenCredentialVersionChanged), "owner lookup failures remain distinguishable from generation mismatches")
}

func TestValidateTokenCredentialDispatchAllowsNonTokenOperations(t *testing.T) {
	require.NoError(t, ValidateTokenCredentialDispatch(context.Background(), &relaycommon.RelayInfo{}))
}

func TestBeginTokenCredentialDispatchAdmissionUsesAuthoritativeWriterOnly(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			require.NoError(t, db.AutoMigrate(&model.TokenCredentialDispatchAdmission{}))
			user, token := seedAuthoritativeBilling(t, db, "dispatch-admission-"+string(mode), 1000, 500, false)
			require.NoError(t, db.Model(token).Update("credential_version", int64(3)).Error)
			info := &relaycommon.RelayInfo{
				UserId: user.Id, TokenId: token.Id, TokenCredentialVersion: 3,
				RequestId: "dispatch-admission-" + string(mode),
			}

			require.NoError(t, BeginTokenCredentialDispatchAdmission(context.Background(), info, model.TokenCredentialDispatchAdmissionKindLegacyTask))
			var count int64
			require.NoError(t, db.Model(&model.TokenCredentialDispatchAdmission{}).Count(&count).Error)
			if mode == model.QuotaWriterModeAuthoritative {
				require.Len(t, info.TokenCredentialDispatchAdmissionID, 64)
				require.Equal(t, int64(1), count)
				require.ErrorIs(t, BeginTokenCredentialDispatchAdmission(context.Background(), info, model.TokenCredentialDispatchAdmissionKindLegacyTask), model.ErrTokenMaintenancePreconditionUnknown)
				require.NoError(t, db.Model(&model.TokenCredentialDispatchAdmission{}).Count(&count).Error)
				require.Equal(t, int64(1), count)
			} else {
				require.Empty(t, info.TokenCredentialDispatchAdmissionID)
				require.Zero(t, count)
			}
		})
	}
}

func TestCompleteTokenCredentialDispatchAdmissionSurvivesRequestCancellation(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeAuthoritative)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TokenCredentialDispatchAdmission{}))
	user, token := seedAuthoritativeBilling(t, db, "dispatch-complete-canceled", 1000, 500, false)
	require.NoError(t, db.Model(token).Update("credential_version", int64(3)).Error)
	info := &relaycommon.RelayInfo{
		UserId: user.Id, TokenId: token.Id, TokenCredentialVersion: 3,
		RequestId: "dispatch-complete-canceled",
	}
	require.NoError(t, BeginTokenCredentialDispatchAdmission(context.Background(), info, model.TokenCredentialDispatchAdmissionKindLegacyTask))

	task := &model.Task{
		TaskID: "task_dispatch_complete_canceled", UserId: user.Id,
		PrivateData: model.TaskPrivateData{TokenId: token.Id},
	}
	require.NoError(t, task.Insert())
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, CompleteTokenCredentialDispatchAdmission(canceled, info, task.ID))

	var admission model.TokenCredentialDispatchAdmission
	require.NoError(t, db.Where("admission_id = ?", info.TokenCredentialDispatchAdmissionID).Take(&admission).Error)
	require.Equal(t, model.TokenCredentialDispatchAdmissionStateLinked, admission.State)
	require.Equal(t, task.ID, admission.LinkedRecordID)
}
