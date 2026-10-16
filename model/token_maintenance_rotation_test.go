package model

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type tokenMaintenanceRotationStoredState struct {
	Token          Token
	Writer         QuotaWriterEpoch
	Cursors        []QuotaWorkCursor
	Policy         AssignedAccessPolicy
	Allocations    []SubscriptionShareAllocation
	Usages         []SubscriptionShareAllocationUsage
	ReceiptCount   int64
	AdmissionCount int64
}

func newTokenMaintenanceRotationFixture(t *testing.T, namespace string) tokenAccountMigrationFixture {
	t.Helper()
	db := newTokenAccountMigrationSQLiteDB(t)
	require.NoError(t, registerTaskRecoveryGormGuards(db))
	require.NoError(t, db.AutoMigrate(tokenCredentialRotationTestEntities()...))
	now, err := ReadDatabaseUnixTime(context.Background(), db)
	require.NoError(t, err)
	require.NoError(t, db.Create(&QuotaWriterEpoch{
		ID: quotaWriterEpochSingletonID, SchemaVersion: QuotaWriterEpochSchemaVersion,
		Mode: string(QuotaWriterModeAuthoritative), Epoch: 1, LockVersion: 1, UpdatedAt: now,
	}).Error)
	for _, name := range []string{quotaWorkCursorAccountMigration, quotaWorkCursorBalanceMigration} {
		require.NoError(t, db.Create(&QuotaWorkCursor{Name: name, Complete: true, UpdatedAt: now}).Error)
	}
	return newTokenAccountMigrationFixture(t, db, namespace)
}

func readTokenMaintenanceRotationStoredState(t *testing.T, fixture tokenAccountMigrationFixture) tokenMaintenanceRotationStoredState {
	t.Helper()
	state := tokenMaintenanceRotationStoredState{}
	require.NoError(t, fixture.db.Unscoped().First(&state.Token, fixture.token.Id).Error)
	require.NoError(t, fixture.db.First(&state.Writer, quotaWriterEpochSingletonID).Error)
	require.NoError(t, fixture.db.Order("name").Find(&state.Cursors).Error)
	require.NoError(t, fixture.db.Where("subject_type = ? AND subject_id = ?", "token", fixture.token.Id).Take(&state.Policy).Error)
	require.NoError(t, fixture.db.Where("token_id = ?", fixture.token.Id).Order("id").Find(&state.Allocations).Error)
	allocationIDs := make([]int64, 0, len(state.Allocations))
	for _, allocation := range state.Allocations {
		allocationIDs = append(allocationIDs, allocation.ID)
	}
	require.NotEmpty(t, allocationIDs)
	require.NoError(t, fixture.db.Where("allocation_id IN ?", allocationIDs).Order("id").Find(&state.Usages).Error)
	require.NoError(t, fixture.db.Model(&TokenCredentialRotationReceipt{}).Count(&state.ReceiptCount).Error)
	require.NoError(t, fixture.db.Model(&TokenCredentialDispatchAdmission{}).Count(&state.AdmissionCount).Error)
	return state
}

func TestTokenMaintenanceRotationQualification(t *testing.T) {
	tests := []struct {
		name            string
		mode            QuotaWriterMode
		dropLegacyUsage bool
		wantStatus      string
		wantCommit      bool
		wantBlocker     TokenMaintenanceBlocker
	}{
		{
			name: "authoritative ready", mode: QuotaWriterModeAuthoritative,
			wantStatus: TokenMaintenanceStatusReady, wantCommit: true,
		},
		{
			name: "legacy blocked", mode: QuotaWriterModeLegacy,
			wantStatus:  TokenMaintenanceStatusBlocked,
			wantBlocker: tokenMaintenanceBlocker("quota_writer_mode_not_authoritative", TokenMaintenanceStatusBlocked),
		},
		{
			name: "bridge blocked", mode: QuotaWriterModeBridge,
			wantStatus:  TokenMaintenanceStatusBlocked,
			wantBlocker: tokenMaintenanceBlocker("quota_writer_mode_not_authoritative", TokenMaintenanceStatusBlocked),
		},
		{
			name: "missing qualification table is unknown", mode: QuotaWriterModeAuthoritative, dropLegacyUsage: true,
			wantStatus:  TokenMaintenanceStatusUnknown,
			wantBlocker: tokenMaintenanceBlocker("credential_rotation_schema_unavailable", TokenMaintenanceStatusUnknown),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTokenMaintenanceRotationFixture(t, test.name)
			require.NoError(t, fixture.db.Model(&QuotaWriterEpoch{}).Where("id = ?", quotaWriterEpochSingletonID).Updates(map[string]any{
				"mode": string(test.mode), "updated_at": fixture.now,
			}).Error)
			if test.dropLegacyUsage {
				require.NoError(t, fixture.db.Migrator().DropTable(&LegacyUsageReservation{}))
			}
			before := readTokenMaintenanceRotationStoredState(t, fixture)
			require.Zero(t, before.ReceiptCount)
			require.Zero(t, before.AdmissionCount)

			view, err := ReadTokenMaintenance(context.Background(), fixture.db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			require.NotNil(t, view)
			assert.Equal(t, test.wantStatus, view.Rotation.Status)
			assert.Equal(t, test.wantCommit, view.Rotation.CommitSupported)
			assert.Equal(t, TokenMaintenanceStatusReady, view.Migration.Status)
			require.NotNil(t, view.Migration.Source)
			require.NotEmpty(t, view.Migration.Candidates)
			if test.wantStatus == TokenMaintenanceStatusReady {
				assert.NotNil(t, view.Rotation.Blockers)
				assert.Empty(t, view.Rotation.Blockers)
			} else {
				assert.Contains(t, view.Rotation.Blockers, test.wantBlocker)
			}

			after := readTokenMaintenanceRotationStoredState(t, fixture)
			assert.Equal(t, before, after)
		})
	}
}

func TestTokenMaintenanceRotationQueryFailureIsolation(t *testing.T) {
	tests := []struct {
		name         string
		injected     error
		localUnknown bool
	}{
		{name: "qualification query error", injected: errors.New("injected credential rotation qualification failure"), localUnknown: true},
		{name: "context canceled", injected: context.Canceled},
		{name: "context deadline exceeded", injected: context.DeadlineExceeded},
		{name: "record not found", injected: gorm.ErrRecordNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTokenMaintenanceRotationFixture(t, test.name)
			before := readTokenMaintenanceRotationStoredState(t, fixture)
			baseline, err := ReadTokenMaintenance(context.Background(), fixture.db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			require.Equal(t, TokenMaintenanceStatusReady, baseline.Rotation.Status)
			require.True(t, baseline.Rotation.CommitSupported)
			require.Equal(t, TokenMaintenanceStatusReady, baseline.Migration.Status)

			const callbackName = "test:token-maintenance-rotation-query-failure"
			callbackRegistered := true
			require.NoError(t, fixture.db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement.Table == (QuotaWriterEpoch{}).TableName() {
					tx.AddError(test.injected)
				}
			}))
			t.Cleanup(func() {
				if callbackRegistered {
					_ = fixture.db.Callback().Query().Remove(callbackName)
				}
			})

			if test.localUnknown {
				view, readErr := ReadTokenMaintenance(context.Background(), fixture.db, fixture.root.Id, fixture.token.Id, fixture.now)
				require.NoError(t, readErr)
				require.NotNil(t, view)
				assert.Equal(t, TokenMaintenanceStatusUnknown, view.Rotation.Status)
				assert.False(t, view.Rotation.CommitSupported)
				assert.Contains(t, view.Rotation.Blockers, tokenMaintenanceBlocker("token_credential_state_unknown", TokenMaintenanceStatusUnknown))
				assert.Equal(t, baseline.Migration, view.Migration)
			} else {
				_, readErr := ReadTokenMaintenance(context.Background(), fixture.db, fixture.root.Id, fixture.token.Id, fixture.now)
				require.ErrorIs(t, readErr, test.injected)
			}

			_, previewErr := PreviewTokenCredentialRotation(context.Background(), fixture.db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.ErrorIs(t, previewErr, test.injected)

			require.NoError(t, fixture.db.Callback().Query().Remove(callbackName))
			callbackRegistered = false
			after := readTokenMaintenanceRotationStoredState(t, fixture)
			assert.Equal(t, before, after)
		})
	}
}
