package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func tokenCredentialRotationTestEntities() []any {
	return []any{
		&User{}, &Token{}, &QuotaWriterEpoch{}, &QuotaWorkCursor{},
		&AccountQuotaMutationReceipt{}, &AccountQuotaReservationHead{},
		&QuotaBalanceBatchDrain{}, &QuotaBalanceBatchSubject{},
		&LegacyUsageReservation{}, &AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{},
		&AccountQuotaRefundFact{}, &QuotaProjectionObligation{},
		&Task{}, &TaskSubmissionOperation{}, &Midjourney{},
		&TokenCredentialDispatchAdmission{}, &TokenCredentialRotationReceipt{},
	}
}

type tokenCredentialRotationFixture struct {
	db    *gorm.DB
	root  User
	token Token
	now   int64
}

func newTokenCredentialRotationSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/token-credential-rotation.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, registerTaskRecoveryGormGuards(db))
	require.NoError(t, db.AutoMigrate(tokenCredentialRotationTestEntities()...))
	return db
}

func newTokenCredentialRotationFixture(t *testing.T, db *gorm.DB, namespace string) tokenCredentialRotationFixture {
	t.Helper()
	now, err := ReadDatabaseUnixTime(context.Background(), db)
	require.NoError(t, err)
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	if len(suffix) > 16 {
		suffix = suffix[len(suffix)-16:]
	}
	root := User{
		Username: "rotation-" + namespace + "-" + suffix,
		AffCode:  "r" + suffix,
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(&root).Error)
	token := Token{
		UserId: root.Id, Key: "rotation-key-" + namespace + "-" + suffix,
		Name: "rotation fixture", Status: common.TokenStatusEnabled, ExpiredTime: -1,
		CredentialVersion: 1,
	}
	require.NoError(t, db.Create(&token).Error)
	var writerCount int64
	require.NoError(t, db.Model(&QuotaWriterEpoch{}).Count(&writerCount).Error)
	if writerCount == 0 {
		require.NoError(t, db.Create(&QuotaWriterEpoch{
			ID: quotaWriterEpochSingletonID, SchemaVersion: QuotaWriterEpochSchemaVersion,
			Mode: string(QuotaWriterModeAuthoritative), Epoch: 1, LockVersion: 1, UpdatedAt: now,
		}).Error)
	} else {
		require.NoError(t, db.Model(&QuotaWriterEpoch{}).Where("id = ?", quotaWriterEpochSingletonID).Updates(map[string]any{
			"schema_version": QuotaWriterEpochSchemaVersion, "mode": string(QuotaWriterModeAuthoritative),
			"epoch": gorm.Expr("epoch + ?", 1), "lock_version": gorm.Expr("lock_version + ?", 1), "updated_at": now,
		}).Error)
	}
	for _, name := range []string{quotaWorkCursorAccountMigration, quotaWorkCursorBalanceMigration} {
		require.NoError(t, db.Save(&QuotaWorkCursor{Name: name, Complete: true, UpdatedAt: now}).Error)
	}
	return tokenCredentialRotationFixture{db: db, root: root, token: token, now: now}
}

func tokenCredentialRotationBlocker(preview *TokenCredentialRotationPreview, code string) bool {
	if preview == nil {
		return false
	}
	for _, blocker := range preview.Blockers {
		if blocker.Code == code {
			return true
		}
	}
	return false
}

func TestTokenCredentialVersionJSONExactBoundary(t *testing.T) {
	tests := []struct {
		name            string
		version         int64
		guardAccepted   bool
		rotationStatus  string
		rotationAfter   int64
		rotationBlocker string
	}{
		{name: "zero", version: 0, rotationStatus: TokenMaintenanceStatusUnknown, rotationBlocker: "token_credential_version_unknown"},
		{name: "negative", version: -1, rotationStatus: TokenMaintenanceStatusUnknown, rotationBlocker: "token_credential_version_unknown"},
		{name: "max int64", version: math.MaxInt64, rotationStatus: TokenMaintenanceStatusBlocked, rotationBlocker: "credential_version_overflow"},
		{name: "above JSON exact maximum", version: tokenCredentialVersionMaxExactJSON + 1, rotationStatus: TokenMaintenanceStatusBlocked, rotationBlocker: "credential_version_overflow"},
		{name: "JSON exact maximum", version: tokenCredentialVersionMaxExactJSON, guardAccepted: true, rotationStatus: TokenMaintenanceStatusBlocked, rotationBlocker: "credential_version_overflow"},
		{name: "last rotatable version", version: tokenCredentialVersionMaxExactJSON - 1, guardAccepted: true, rotationStatus: TokenMaintenanceStatusReady, rotationAfter: tokenCredentialVersionMaxExactJSON},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newTokenCredentialRotationSQLiteDB(t)
			fixture := newTokenCredentialRotationFixture(t, db, "version-boundary")
			require.NoError(t, db.Model(&Token{}).Where("id = ?", fixture.token.Id).Update("credential_version", test.version).Error)

			guardErr := ValidateTokenCredentialVersion(context.Background(), db, fixture.token.Id, fixture.root.Id, test.version)
			if test.guardAccepted {
				require.NoError(t, guardErr)
			} else {
				require.ErrorIs(t, guardErr, ErrTokenCredentialVersionChanged)
			}

			preview, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			assert.Equal(t, test.rotationStatus, preview.Status)
			assert.Equal(t, test.rotationAfter, preview.CredentialVersionAfter)
			if test.rotationBlocker != "" {
				assert.True(t, tokenCredentialRotationBlocker(preview, test.rotationBlocker))
			}
			if test.version == tokenCredentialVersionMaxExactJSON-1 {
				previousRedisEnabled, previousRDB := common.RedisEnabled, common.RDB
				common.RedisEnabled, common.RDB = false, nil
				t.Cleanup(func() { common.RedisEnabled, common.RDB = previousRedisEnabled, previousRDB })
				view, _, replayed, commitErr := CommitTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, TokenCredentialRotationCommand{
					OperationID: strings.Repeat("e", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
				}, fixture.now)
				require.NoError(t, commitErr)
				assert.False(t, replayed)
				require.NotNil(t, view)
				assert.Equal(t, tokenCredentialVersionMaxExactJSON, view.CredentialVersionAfter)
				require.NoError(t, ValidateTokenCredentialVersion(context.Background(), db, fixture.token.Id, fixture.root.Id, tokenCredentialVersionMaxExactJSON))
				postCommitPreview, previewErr := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
				require.NoError(t, previewErr)
				assert.Equal(t, TokenMaintenanceStatusBlocked, postCommitPreview.Status)
				assert.True(t, tokenCredentialRotationBlocker(postCommitPreview, "credential_version_overflow"))
				var beforeAdmission Token
				require.NoError(t, db.First(&beforeAdmission, fixture.token.Id).Error)

				admission, admissionErr := BeginTokenCredentialDispatchAdmission(context.Background(), db, fixture.root.Id, fixture.token.Id, tokenCredentialVersionMaxExactJSON, "request-at-json-boundary", TokenCredentialDispatchAdmissionKindLegacyTask)
				require.NoError(t, admissionErr)
				require.NotNil(t, admission)
				postAdmissionPreview, previewErr := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
				require.NoError(t, previewErr)
				assert.Equal(t, TokenMaintenanceStatusUnknown, postAdmissionPreview.Status)
				assert.True(t, tokenCredentialRotationBlocker(postAdmissionPreview, "credential_version_overflow"))
				assert.True(t, tokenCredentialRotationBlocker(postAdmissionPreview, "credential_dispatch_admission_open"))
				var afterAdmission Token
				require.NoError(t, db.First(&afterAdmission, fixture.token.Id).Error)
				assert.Equal(t, beforeAdmission.Key, afterAdmission.Key)
				assert.Equal(t, beforeAdmission.CredentialVersion, afterAdmission.CredentialVersion)
			}
		})
	}
}

func TestTokenCredentialRotationReceiptRejectsInexactVersion(t *testing.T) {
	receipt := TokenCredentialRotationReceipt{
		OperationID: strings.Repeat("a", 64), RequestDigest: strings.Repeat("b", 64),
		ActorID: 1, TokenID: 2, State: tokenCredentialRotationStateCommitted,
		CredentialVersionBefore: tokenCredentialVersionMaxExactJSON - 1,
		CredentialVersionAfter:  tokenCredentialVersionMaxExactJSON,
		KeyFingerprint:          strings.Repeat("c", 64), MaskedKey: "abcd**********wxyz",
		PreviewRevision: strings.Repeat("d", 64), CreatedAt: 1, OldSecretInvalidatedAt: 1,
	}
	view, err := tokenCredentialRotationReceiptView(receipt)
	require.NoError(t, err)
	assert.Equal(t, tokenCredentialVersionMaxExactJSON, view.CredentialVersionAfter)

	for _, before := range []int64{0, -1, tokenCredentialVersionMaxExactJSON, tokenCredentialVersionMaxExactJSON + 1, math.MaxInt64} {
		invalid := receipt
		invalid.CredentialVersionBefore = before
		if before == math.MaxInt64 {
			invalid.CredentialVersionAfter = math.MinInt64
		} else {
			invalid.CredentialVersionAfter = before + 1
		}
		_, err := tokenCredentialRotationReceiptView(invalid)
		require.ErrorIs(t, err, ErrTokenMaintenanceUnknown)
	}
}

func assertTokenCredentialRotationCommitRejectedWithoutWrite(t *testing.T, fixture tokenCredentialRotationFixture, previewRevision string, expected error) {
	t.Helper()
	previousRedisEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = previousRedisEnabled, previousRDB })
	var before Token
	require.NoError(t, fixture.db.First(&before, fixture.token.Id).Error)
	_, secret, replayed, err := CommitTokenCredentialRotation(context.Background(), fixture.db, fixture.root.Id, fixture.token.Id, TokenCredentialRotationCommand{
		OperationID: strings.Repeat("f", 64), PreviewRevision: previewRevision, Confirmed: true,
	}, fixture.now)
	require.ErrorIs(t, err, expected)
	assert.Empty(t, secret)
	assert.False(t, replayed)
	var after Token
	require.NoError(t, fixture.db.First(&after, fixture.token.Id).Error)
	assert.Equal(t, before, after)
	var receiptCount int64
	require.NoError(t, fixture.db.Model(&TokenCredentialRotationReceipt{}).Where("token_id = ?", fixture.token.Id).Count(&receiptCount).Error)
	assert.Zero(t, receiptCount)
}

func TestTokenCredentialRotationRejectsNonAuthoritativeWriterWithoutWrite(t *testing.T) {
	for _, mode := range []QuotaWriterMode{QuotaWriterModeLegacy, QuotaWriterModeBridge} {
		t.Run(string(mode), func(t *testing.T) {
			db := newTokenCredentialRotationSQLiteDB(t)
			fixture := newTokenCredentialRotationFixture(t, db, "writer-"+string(mode))
			ready, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			require.Equal(t, TokenMaintenanceStatusReady, ready.Status)
			require.NoError(t, db.Model(&QuotaWriterEpoch{}).Where("id = ?", quotaWriterEpochSingletonID).Update("mode", string(mode)).Error)

			blocked, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			assert.Equal(t, TokenMaintenanceStatusBlocked, blocked.Status)
			assert.True(t, tokenCredentialRotationBlocker(blocked, "quota_writer_mode_not_authoritative"))
			assertTokenCredentialRotationCommitRejectedWithoutWrite(t, fixture, ready.PreviewRevision, ErrTokenMaintenanceBlocked)
		})
	}
}

func TestTokenCredentialRotationRejectsUnavailableBackfillWithoutWrite(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, tokenCredentialRotationFixture)
		code  string
	}{
		{
			name: "missing schema",
			setup: func(t *testing.T, fixture tokenCredentialRotationFixture) {
				require.NoError(t, fixture.db.Migrator().DropTable(&QuotaWorkCursor{}))
			},
			code: "quota_maintenance_backfill_unknown",
		},
		{
			name: "incomplete cursor",
			setup: func(t *testing.T, fixture tokenCredentialRotationFixture) {
				require.NoError(t, fixture.db.Model(&QuotaWorkCursor{}).Where("name = ?", quotaWorkCursorAccountMigration).Update("complete", false).Error)
			},
			code: "quota_maintenance_backfill_incomplete",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newTokenCredentialRotationSQLiteDB(t)
			fixture := newTokenCredentialRotationFixture(t, db, "backfill")
			ready, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			require.Equal(t, TokenMaintenanceStatusReady, ready.Status)
			test.setup(t, fixture)

			unknown, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			assert.Equal(t, TokenMaintenanceStatusUnknown, unknown.Status)
			assert.True(t, tokenCredentialRotationBlocker(unknown, test.code))
			assertTokenCredentialRotationCommitRejectedWithoutWrite(t, fixture, ready.PreviewRevision, ErrTokenMaintenancePreconditionUnknown)
		})
	}
}

func TestTokenCredentialRotationRejectsMissingReceiptTableWithoutWrite(t *testing.T) {
	db := newTokenCredentialRotationSQLiteDB(t)
	fixture := newTokenCredentialRotationFixture(t, db, "missing-receipt")
	ready, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
	require.NoError(t, err)
	require.Equal(t, TokenMaintenanceStatusReady, ready.Status)
	require.NotEmpty(t, ready.PreviewRevision)
	require.NoError(t, db.Migrator().DropTable(&TokenCredentialRotationReceipt{}))

	unknown, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
	require.NoError(t, err)
	require.NotNil(t, unknown)
	assert.Equal(t, TokenMaintenanceStatusUnknown, unknown.Status)
	assert.True(t, tokenCredentialRotationBlocker(unknown, "credential_rotation_schema_unavailable"))
	assert.Empty(t, unknown.PreviewRevision)
	assert.Nil(t, unknown.Effects)

	var before Token
	require.NoError(t, db.First(&before, fixture.token.Id).Error)
	_, secret, replayed, err := CommitTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, TokenCredentialRotationCommand{
		OperationID: strings.Repeat("f", 64), PreviewRevision: ready.PreviewRevision, Confirmed: true,
	}, fixture.now)
	require.Error(t, err)
	assert.Empty(t, secret)
	assert.False(t, replayed)
	var after Token
	require.NoError(t, db.First(&after, fixture.token.Id).Error)
	assert.Equal(t, before.Key, after.Key)
	assert.Equal(t, before.CredentialVersion, after.CredentialVersion)
}

func TestTokenCredentialRotationRejectsNonterminalLegacyObligationsWithoutWrite(t *testing.T) {
	tests := []struct {
		name  string
		code  string
		setup func(*testing.T, tokenCredentialRotationFixture)
	}{
		{
			name: "settlement",
			code: "legacy_settlement_pending",
			setup: func(t *testing.T, fixture tokenCredentialRotationFixture) {
				require.NoError(t, fixture.db.Create(&AccountQuotaSettlementFact{
					EventKey: "rotation-settlement", RequestID: "rotation-settlement", Kind: AccountQuotaSettlementKindLegacyWallet,
					UserID: fixture.root.Id, TokenID: fixture.token.Id, WriterEpoch: 1, ApplyToken: true,
					RequestFingerprint: strings.Repeat("1", 64), State: AccountQuotaSettlementPending,
					LockVersion: 1, CreatedAt: fixture.now, UpdatedAt: fixture.now,
				}).Error)
			},
		},
		{
			name: "refund",
			code: "legacy_refund_pending",
			setup: func(t *testing.T, fixture tokenCredentialRotationFixture) {
				require.NoError(t, fixture.db.Create(&AccountQuotaRefundFact{
					EventKey: "rotation-refund", RequestID: "rotation-refund", Kind: AccountQuotaRefundFactKindLegacyWallet,
					WriterEpoch: 1, UserID: fixture.root.Id, TokenID: fixture.token.Id, TokenQuota: 1,
					RequestFingerprint: strings.Repeat("2", 64), State: AccountQuotaRefundFactPending,
					LockVersion: 1, CreatedAt: fixture.now, UpdatedAt: fixture.now,
				}).Error)
			},
		},
		{
			name: "projection",
			code: "quota_projection_pending",
			setup: func(t *testing.T, fixture tokenCredentialRotationFixture) {
				require.NoError(t, fixture.db.Create(&QuotaProjectionObligation{
					SchemaVersion: quotaProjectionObligationSchemaVersion, ReceiptKind: quotaProjectionReceiptKindAccount,
					ReceiptID: 1, EventKey: "rotation-projection", WriterEpoch: 1,
					UserID: fixture.root.Id, TokenID: fixture.token.Id, ExpectedUserVersion: 1, ExpectedTokenVersion: 1,
					State: string(QuotaProjectionObligationStatePending), Fence: 1, LockVersion: 1,
					CreatedAt: fixture.now, UpdatedAt: fixture.now,
				}).Error)
			},
		},
		{
			name: "legacy quota batch",
			code: "legacy_quota_generation_pending",
			setup: func(t *testing.T, fixture tokenCredentialRotationFixture) {
				payload := QuotaBalanceBatchPayload{
					TokenQuota:     map[int]int64{fixture.token.Id: 1},
					TokenCacheKeys: map[int]string{fixture.token.Id: getTokenCacheKey(fixture.token.Key)},
				}
				fingerprint, err := quotaBalanceBatchPayloadFingerprintForSchema(quotaBalanceBatchDrainSchemaVersion, payload)
				require.NoError(t, err)
				tokenKind, err := quotaBalanceStoredSubjectKind(BatchUpdateTypeTokenQuota)
				require.NoError(t, err)
				generation := QuotaBalanceBatchDrain{
					SchemaVersion: quotaBalanceBatchDrainSchemaVersion, GenerationKey: "rotation-batch", WriterEpoch: 1,
					Payload: payload, PayloadFingerprint: fingerprint, SubjectKind: tokenKind, SubjectID: fixture.token.Id,
					State: quotaBalanceBatchDrainPending, LockVersion: 1, CreatedAt: fixture.now, UpdatedAt: fixture.now,
				}
				require.NoError(t, fixture.db.Transaction(func(tx *gorm.DB) error {
					if err := tx.Create(&generation).Error; err != nil {
						return err
					}
					return tx.Create(&QuotaBalanceBatchSubject{GenerationID: generation.ID, SubjectKind: tokenKind, SubjectID: fixture.token.Id}).Error
				}))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newTokenCredentialRotationSQLiteDB(t)
			fixture := newTokenCredentialRotationFixture(t, db, "obligation")
			ready, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			require.Equal(t, TokenMaintenanceStatusReady, ready.Status)
			test.setup(t, fixture)

			blocked, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			assert.Equal(t, TokenMaintenanceStatusBlocked, blocked.Status)
			assert.True(t, tokenCredentialRotationBlocker(blocked, test.code))
			assertTokenCredentialRotationCommitRejectedWithoutWrite(t, fixture, ready.PreviewRevision, ErrTokenMaintenanceBlocked)
		})
	}
}

func TestTokenCredentialDispatchAdmissionLifecycleAndCrashReplayFailClosed(t *testing.T) {
	db := newTokenCredentialRotationSQLiteDB(t)
	fixture := newTokenCredentialRotationFixture(t, db, "admission")
	ctx := context.Background()

	admission, err := BeginTokenCredentialDispatchAdmission(ctx, db, fixture.root.Id, fixture.token.Id, 1, "request-admission-1", TokenCredentialDispatchAdmissionKindLegacyTask)
	require.NoError(t, err)
	require.NotNil(t, admission)
	assert.True(t, canonicalLowerHex(admission.AdmissionID, 64))
	assert.Equal(t, TokenCredentialDispatchAdmissionStateOpen, admission.State)

	replayed, err := BeginTokenCredentialDispatchAdmission(ctx, db, fixture.root.Id, fixture.token.Id, 1, "request-admission-1", TokenCredentialDispatchAdmissionKindLegacyTask)
	require.ErrorIs(t, err, ErrTokenMaintenancePreconditionUnknown)
	require.NotNil(t, replayed)
	assert.Equal(t, admission.AdmissionID, replayed.AdmissionID)

	preview, err := PreviewTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, fixture.now)
	require.NoError(t, err)
	assert.Equal(t, TokenMaintenanceStatusUnknown, preview.Status)
	assert.True(t, tokenCredentialRotationBlocker(preview, "credential_dispatch_admission_open"))

	err = CompleteTokenCredentialDispatchAdmission(ctx, db, fixture.root.Id, fixture.token.Id, admission.AdmissionID, 404)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	task := Task{
		TaskID: "legacy-admission-task", UserId: fixture.root.Id, Status: TaskStatusInProgress, Progress: "20%",
		PrivateData: TaskPrivateData{TokenId: fixture.token.Id},
	}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, CompleteTokenCredentialDispatchAdmission(ctx, db, fixture.root.Id, fixture.token.Id, admission.AdmissionID, task.ID))
	require.NoError(t, CompleteTokenCredentialDispatchAdmission(ctx, db, fixture.root.Id, fixture.token.Id, admission.AdmissionID, task.ID), "same link is idempotent")

	var stored TokenCredentialDispatchAdmission
	require.NoError(t, db.Where("admission_id = ?", admission.AdmissionID).Take(&stored).Error)
	assert.Equal(t, TokenCredentialDispatchAdmissionStateLinked, stored.State)
	assert.Equal(t, task.ID, stored.LinkedRecordID)
	assert.NotEmpty(t, stored.LinkedIdentityHash)
	for _, mutation := range []struct {
		name string
		run  func() error
	}{
		{
			name: "model update column",
			run: func() error {
				return db.Model(&TokenCredentialDispatchAdmission{}).Where("admission_id = ?", admission.AdmissionID).
					UpdateColumn("state", TokenCredentialDispatchAdmissionStateUnknown).Error
			},
		},
		{
			name: "table update column",
			run: func() error {
				return db.Table("token_credential_dispatch_admissions").Where("admission_id = ?", admission.AdmissionID).
					UpdateColumn("state", TokenCredentialDispatchAdmissionStateUnknown).Error
			},
		},
		{
			name: "aliased table update column",
			run: func() error {
				return db.Table("token_credential_dispatch_admissions AS dispatch_admissions").
					Where("dispatch_admissions.admission_id = ?", admission.AdmissionID).
					UpdateColumn("state", TokenCredentialDispatchAdmissionStateUnknown).Error
			},
		},
		{
			name: "model delete",
			run: func() error {
				return db.Where("admission_id = ?", admission.AdmissionID).Delete(&TokenCredentialDispatchAdmission{}).Error
			},
		},
		{
			name: "table delete",
			run: func() error {
				return db.Table("token_credential_dispatch_admissions").Where("admission_id = ?", admission.AdmissionID).Delete(nil).Error
			},
		},
		{
			name: "aliased table delete",
			run: func() error {
				return db.Table("token_credential_dispatch_admissions AS dispatch_admissions").
					Where("dispatch_admissions.admission_id = ?", admission.AdmissionID).Delete(nil).Error
			},
		},
	} {
		t.Run("immutable "+mutation.name, func(t *testing.T) {
			require.ErrorIs(t, mutation.run(), ErrAccountQuotaReceiptImmutable)
		})
	}
	var afterBypassAttempts TokenCredentialDispatchAdmission
	require.NoError(t, db.Where("admission_id = ?", admission.AdmissionID).Take(&afterBypassAttempts).Error)
	assert.Equal(t, stored, afterBypassAttempts)

	preview, err = PreviewTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, fixture.now)
	require.NoError(t, err)
	assert.Equal(t, TokenMaintenanceStatusBlocked, preview.Status)
	assert.True(t, tokenCredentialRotationBlocker(preview, "legacy_async_task_pending"))

	require.NoError(t, db.Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]any{"status": TaskStatusSuccess, "progress": "100%"}).Error)
	preview, err = PreviewTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, fixture.now)
	require.NoError(t, err)
	assert.Equal(t, TokenMaintenanceStatusReady, preview.Status)
}

func TestTokenCredentialDispatchAdmissionUnknownIsFailClosed(t *testing.T) {
	db := newTokenCredentialRotationSQLiteDB(t)
	fixture := newTokenCredentialRotationFixture(t, db, "unknown")
	ctx := context.Background()
	admission, err := BeginTokenCredentialDispatchAdmission(ctx, db, fixture.root.Id, fixture.token.Id, 1, "request-unknown-1", TokenCredentialDispatchAdmissionKindLegacyTask)
	require.NoError(t, err)
	require.NoError(t, MarkTokenCredentialDispatchAdmissionUnknown(ctx, db, fixture.root.Id, fixture.token.Id, admission.AdmissionID, "provider_outcome_unknown"))
	require.NoError(t, MarkTokenCredentialDispatchAdmissionUnknown(ctx, db, fixture.root.Id, fixture.token.Id, admission.AdmissionID, "task_persist_failed"), "unknown replay preserves the first evidence")

	replayed, err := BeginTokenCredentialDispatchAdmission(ctx, db, fixture.root.Id, fixture.token.Id, 1, "request-unknown-1", TokenCredentialDispatchAdmissionKindLegacyTask)
	require.ErrorIs(t, err, ErrTokenMaintenancePreconditionUnknown)
	require.NotNil(t, replayed)
	assert.Equal(t, admission.AdmissionID, replayed.AdmissionID)

	preview, err := PreviewTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, fixture.now)
	require.NoError(t, err)
	assert.Equal(t, TokenMaintenanceStatusUnknown, preview.Status)
	assert.True(t, tokenCredentialRotationBlocker(preview, "credential_dispatch_admission_unknown"))

	var token Token
	require.NoError(t, db.First(&token, fixture.token.Id).Error)
	assert.Equal(t, int64(1), token.CredentialVersion)
	assert.Equal(t, fixture.token.Key, token.Key)
}

func TestTokenCredentialDispatchAdmissionMidjourneyUsesImmutableDispatchToken(t *testing.T) {
	db := newTokenCredentialRotationSQLiteDB(t)
	fixture := newTokenCredentialRotationFixture(t, db, "midjourney")
	ctx := context.Background()
	admission, err := BeginTokenCredentialDispatchAdmission(ctx, db, fixture.root.Id, fixture.token.Id, 1, "request-mj-1", TokenCredentialDispatchAdmissionKindMidjourney)
	require.NoError(t, err)

	legacyBillingOnly := Midjourney{UserId: fixture.root.Id, MjId: "mj-billing-only", TokenId: fixture.token.Id, Status: string(TaskStatusInProgress), Progress: "5%"}
	require.NoError(t, db.Create(&legacyBillingOnly).Error)
	err = CompleteTokenCredentialDispatchAdmission(ctx, db, fixture.root.Id, fixture.token.Id, admission.AdmissionID, int64(legacyBillingOnly.Id))
	require.ErrorIs(t, err, ErrTokenMaintenanceConflict)

	linked := Midjourney{
		UserId: fixture.root.Id, Code: 1, MjId: "mj-dispatch-linked", DispatchTokenID: fixture.token.Id,
		TokenId: 0, Status: string(TaskStatusInProgress), Progress: "5%",
	}
	require.NoError(t, db.Create(&linked).Error)
	require.NoError(t, CompleteTokenCredentialDispatchAdmission(ctx, db, fixture.root.Id, fixture.token.Id, admission.AdmissionID, int64(linked.Id)))

	var stored Midjourney
	require.NoError(t, db.First(&stored, linked.Id).Error)
	assert.Zero(t, stored.TokenId, "admission linkage must not alter the billing/refund token marker")
	assert.Equal(t, fixture.token.Id, stored.DispatchTokenID)
}

func TestTokenCredentialDispatchAdmissionMidjourneyDeterministicRejection(t *testing.T) {
	for _, code := range []int{23, 24} {
		t.Run(fmt.Sprintf("code %d", code), func(t *testing.T) {
			db := newTokenCredentialRotationSQLiteDB(t)
			fixture := newTokenCredentialRotationFixture(t, db, fmt.Sprintf("midjourney-rejection-%d", code))
			admission, err := BeginTokenCredentialDispatchAdmission(
				context.Background(), db, fixture.root.Id, fixture.token.Id, 1,
				fmt.Sprintf("request-mj-rejection-%d", code), TokenCredentialDispatchAdmissionKindMidjourney,
			)
			require.NoError(t, err)
			rejection := Midjourney{
				UserId: fixture.root.Id, Code: code, DispatchTokenID: fixture.token.Id,
				Status: string(TaskStatusFailure), Progress: "100%",
			}
			require.NoError(t, db.Create(&rejection).Error)
			require.NoError(t, CompleteTokenCredentialDispatchAdmission(
				context.Background(), db, fixture.root.Id, fixture.token.Id, admission.AdmissionID, int64(rejection.Id),
			))

			preview, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			assert.Equal(t, TokenMaintenanceStatusReady, preview.Status)
			assert.False(t, tokenCredentialRotationBlocker(preview, "credential_dispatch_link_unknown"))
		})
	}
}

func TestTokenCredentialDispatchAdmissionMidjourneyAcceptedCodeNormalization(t *testing.T) {
	for _, code := range []int{21, 22} {
		t.Run(fmt.Sprintf("code %d to 1", code), func(t *testing.T) {
			db := newTokenCredentialRotationSQLiteDB(t)
			fixture := newTokenCredentialRotationFixture(t, db, fmt.Sprintf("midjourney-accepted-%d", code))
			admission, err := BeginTokenCredentialDispatchAdmission(
				context.Background(), db, fixture.root.Id, fixture.token.Id, 1,
				fmt.Sprintf("request-mj-accepted-%d", code), TokenCredentialDispatchAdmissionKindMidjourney,
			)
			require.NoError(t, err)
			task := Midjourney{
				UserId: fixture.root.Id, Code: code, MjId: fmt.Sprintf("mj-accepted-%d", code),
				DispatchTokenID: fixture.token.Id, Status: string(TaskStatusInProgress), Progress: "5%",
			}
			require.NoError(t, db.Create(&task).Error)
			require.NoError(t, CompleteTokenCredentialDispatchAdmission(
				context.Background(), db, fixture.root.Id, fixture.token.Id, admission.AdmissionID, int64(task.Id),
			))
			var linked TokenCredentialDispatchAdmission
			require.NoError(t, db.Where("admission_id = ?", admission.AdmissionID).Take(&linked).Error)
			require.NotEmpty(t, linked.LinkedIdentityHash)

			require.NoError(t, db.Model(&Midjourney{}).Where("id = ?", task.Id).Updates(map[string]any{
				"code": 1, "status": string(TaskStatusSuccess), "progress": "100%",
			}).Error)
			identityAfterPolling, err := tokenCredentialDispatchLinkedIdentity(
				context.Background(), db, linked, int64(task.Id),
			)
			require.NoError(t, err)
			assert.Equal(t, linked.LinkedIdentityHash, identityAfterPolling)
			preview, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			assert.Equal(t, TokenMaintenanceStatusReady, preview.Status)
			assert.False(t, tokenCredentialRotationBlocker(preview, "credential_dispatch_link_unknown"))
		})
	}
}

func TestTokenCredentialDispatchAdmissionMidjourneyAmbiguousResponseStaysUnknown(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		mjID     string
		status   string
		progress string
	}{
		{name: "accepted response missing provider id", code: 1, status: string(TaskStatusInProgress), progress: "0%"},
		{name: "queue full without terminal failure", code: 23, status: string(TaskStatusInProgress), progress: "0%"},
		{name: "sensitive rejection without terminal progress", code: 24, status: string(TaskStatusFailure), progress: "50%"},
		{name: "undocumented rejection", code: 25, status: string(TaskStatusFailure), progress: "100%"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newTokenCredentialRotationSQLiteDB(t)
			fixture := newTokenCredentialRotationFixture(t, db, "midjourney-ambiguous")
			admission, err := BeginTokenCredentialDispatchAdmission(
				context.Background(), db, fixture.root.Id, fixture.token.Id, 1,
				"request-mj-ambiguous-"+strings.ReplaceAll(test.name, " ", "-"), TokenCredentialDispatchAdmissionKindMidjourney,
			)
			require.NoError(t, err)
			task := Midjourney{
				UserId: fixture.root.Id, Code: test.code, MjId: test.mjID, DispatchTokenID: fixture.token.Id,
				Status: test.status, Progress: test.progress,
			}
			require.NoError(t, db.Create(&task).Error)
			err = CompleteTokenCredentialDispatchAdmission(
				context.Background(), db, fixture.root.Id, fixture.token.Id, admission.AdmissionID, int64(task.Id),
			)
			require.ErrorIs(t, err, ErrTokenMaintenanceConflict)
			require.NoError(t, MarkTokenCredentialDispatchAdmissionUnknown(
				context.Background(), db, fixture.root.Id, fixture.token.Id, admission.AdmissionID, "task_link_failed",
			))

			preview, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			assert.Equal(t, TokenMaintenanceStatusUnknown, preview.Status)
			assert.True(t, tokenCredentialRotationBlocker(preview, "credential_dispatch_admission_unknown"))
		})
	}
}

func TestTokenCredentialRotationRejectsSplitTaskTerminalSignals(t *testing.T) {
	tests := []struct {
		name         string
		status       TaskStatus
		progress     string
		missingToken bool
	}{
		{name: "progress terminal while status active", status: TaskStatusInProgress, progress: "100%"},
		{name: "status terminal while progress active and identity missing", status: TaskStatusSuccess, progress: "50%", missingToken: true},
	}
	for _, test := range tests {
		t.Run("task "+test.name, func(t *testing.T) {
			db := newTokenCredentialRotationSQLiteDB(t)
			fixture := newTokenCredentialRotationFixture(t, db, "task-signals")
			tokenID := fixture.token.Id
			if test.missingToken {
				tokenID = 0
			}
			require.NoError(t, db.Create(&Task{
				TaskID: "split-task", UserId: fixture.root.Id, Status: test.status, Progress: test.progress,
				PrivateData: TaskPrivateData{TokenId: tokenID},
			}).Error)
			preview, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			assert.Equal(t, TokenMaintenanceStatusUnknown, preview.Status)
			assert.True(t, tokenCredentialRotationBlocker(preview, "legacy_async_identity_unknown"))
		})

		t.Run("midjourney "+test.name, func(t *testing.T) {
			db := newTokenCredentialRotationSQLiteDB(t)
			fixture := newTokenCredentialRotationFixture(t, db, "mj-signals")
			dispatchTokenID := fixture.token.Id
			if test.missingToken {
				dispatchTokenID = 0
			}
			require.NoError(t, db.Create(&Midjourney{
				UserId: fixture.root.Id, MjId: "split-mj", DispatchTokenID: dispatchTokenID,
				Status: string(test.status), Progress: test.progress,
			}).Error)
			preview, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			assert.Equal(t, TokenMaintenanceStatusUnknown, preview.Status)
			expectedCode := "midjourney_task_unknown"
			if test.missingToken {
				expectedCode = "legacy_async_identity_unknown"
			}
			assert.True(t, tokenCredentialRotationBlocker(preview, expectedCode))
		})
	}
}

func TestTokenCredentialRotationRejectsDurableTerminalContradiction(t *testing.T) {
	db := newTokenCredentialRotationSQLiteDB(t)
	fixture := newTokenCredentialRotationFixture(t, db, "durable-contradiction")
	publicID := "task_" + strings.Repeat("d", 32)
	task := Task{
		TaskID: publicID, UserId: fixture.root.Id, Status: TaskStatusInProgress, Progress: "100%",
		PrivateData: TaskPrivateData{TokenId: fixture.token.Id},
	}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Exec(`INSERT INTO task_submission_operations
		(public_id, user_id, token_id, http_method, operation_kind, idempotency_key_hash, request_fingerprint,
		 request_id, status, task_id, lock_version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		publicID, fixture.root.Id, fixture.token.Id, "POST", TaskSubmissionOperationKindVideoCreate,
		strings.Repeat("1", 64), strings.Repeat("2", 64), "durable-contradiction",
		TaskSubmissionOperationStatusSucceeded, task.ID, int64(1), fixture.now, fixture.now,
	).Error)

	preview, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
	require.NoError(t, err)
	assert.Equal(t, TokenMaintenanceStatusUnknown, preview.Status)
	assert.True(t, tokenCredentialRotationBlocker(preview, "durable_async_identity_unknown"))
}

func TestTokenCredentialRotationPreservesIDAndHidesSecretOnReplay(t *testing.T) {
	db := newTokenCredentialRotationSQLiteDB(t)
	fixture := newTokenCredentialRotationFixture(t, db, "commit")
	assertTokenCredentialRotationCommitContract(t, fixture)
}

func assertTokenCredentialRotationCommitContract(t *testing.T, fixture tokenCredentialRotationFixture) {
	t.Helper()
	db := fixture.db
	ctx := context.Background()
	previousRedisEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = previousRedisEnabled, previousRDB })
	var tokenBefore Token
	require.NoError(t, db.First(&tokenBefore, fixture.token.Id).Error)

	preview, err := PreviewTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, fixture.now)
	require.NoError(t, err)
	require.Equal(t, TokenMaintenanceStatusReady, preview.Status)
	require.NotEmpty(t, preview.PreviewRevision)
	require.NotNil(t, preview.Effects)

	command := TokenCredentialRotationCommand{
		OperationID: strings.Repeat("a", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
	}
	view, rawKey, replayed, err := CommitTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, command, fixture.now)
	require.NoError(t, err)
	assert.False(t, replayed)
	require.NotNil(t, view)
	assert.Len(t, rawKey, 48)
	assert.Equal(t, fixture.token.Id, view.TokenID)
	assert.Equal(t, int64(1), view.CredentialVersionBefore)
	assert.Equal(t, int64(2), view.CredentialVersionAfter)

	var stored Token
	require.NoError(t, db.First(&stored, fixture.token.Id).Error)
	assert.Equal(t, fixture.token.Id, stored.Id)
	assert.Equal(t, rawKey, stored.Key)
	assert.NotEqual(t, fixture.token.Key, stored.Key)
	assert.Equal(t, int64(2), stored.CredentialVersion)

	replayView, replaySecret, replayed, err := CommitTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, command, fixture.now)
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Empty(t, replaySecret)
	assert.Equal(t, view, replayView)

	receiptView, err := ReadTokenCredentialRotationOperation(ctx, db, fixture.root.Id, fixture.token.Id, command.OperationID)
	require.NoError(t, err)
	assert.Equal(t, view, receiptView)
	var receipt TokenCredentialRotationReceipt
	require.NoError(t, db.Where("operation_id = ?", command.OperationID).Take(&receipt).Error)
	assert.NotContains(t, fmt.Sprintf("%+v", receipt), rawKey)
	assert.NotContains(t, receipt.MaskedKey, rawKey)

	tokenAfterFirstCommit := stored
	conflictingCommand := TokenCredentialRotationCommand{
		OperationID: command.OperationID, PreviewRevision: strings.Repeat("f", 64), Confirmed: true,
	}
	_, conflictingSecret, conflictingReplay, err := CommitTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, conflictingCommand, fixture.now)
	require.ErrorIs(t, err, ErrTokenCredentialRotationOperationConflict)
	assert.Empty(t, conflictingSecret)
	assert.False(t, conflictingReplay)
	var tokenAfterConflict Token
	require.NoError(t, db.First(&tokenAfterConflict, fixture.token.Id).Error)
	assert.Equal(t, tokenAfterFirstCommit, tokenAfterConflict, "operation-id conflict must write nothing")

	staleCommand := TokenCredentialRotationCommand{
		OperationID: strings.Repeat("b", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
	}
	_, staleSecret, staleReplay, err := CommitTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, staleCommand, fixture.now)
	require.ErrorIs(t, err, ErrTokenMaintenanceConflict)
	assert.Empty(t, staleSecret)
	assert.False(t, staleReplay)
	var staleReceiptCount int64
	require.NoError(t, db.Model(&TokenCredentialRotationReceipt{}).Where("operation_id = ?", staleCommand.OperationID).Count(&staleReceiptCount).Error)
	assert.Zero(t, staleReceiptCount)
	var tokenAfterStale Token
	require.NoError(t, db.First(&tokenAfterStale, fixture.token.Id).Error)
	assert.Equal(t, tokenAfterFirstCommit, tokenAfterStale, "stale preview must write nothing")

	secondPreview, err := PreviewTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, fixture.now)
	require.NoError(t, err)
	require.Equal(t, TokenMaintenanceStatusReady, secondPreview.Status)
	require.NotEmpty(t, secondPreview.PreviewRevision)
	assert.NotEqual(t, preview.PreviewRevision, secondPreview.PreviewRevision)
	secondCommand := TokenCredentialRotationCommand{
		OperationID: strings.Repeat("c", 64), PreviewRevision: secondPreview.PreviewRevision, Confirmed: true,
	}
	secondView, secondRawKey, secondReplayed, err := CommitTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, secondCommand, fixture.now)
	require.NoError(t, err)
	assert.False(t, secondReplayed)
	require.NotNil(t, secondView)
	assert.Len(t, secondRawKey, 48)
	assert.NotEqual(t, rawKey, secondRawKey)
	assert.Equal(t, fixture.token.Id, secondView.TokenID)
	assert.Equal(t, int64(2), secondView.CredentialVersionBefore)
	assert.Equal(t, int64(3), secondView.CredentialVersionAfter)

	var tokenAfterSecondCommit Token
	require.NoError(t, db.First(&tokenAfterSecondCommit, fixture.token.Id).Error)
	assert.Equal(t, fixture.token.Id, tokenAfterSecondCommit.Id)
	assert.Equal(t, secondRawKey, tokenAfterSecondCommit.Key)
	assert.Equal(t, int64(3), tokenAfterSecondCommit.CredentialVersion)
	preserved := tokenAfterSecondCommit
	preserved.Key = tokenBefore.Key
	preserved.CredentialVersion = tokenBefore.CredentialVersion
	assert.Equal(t, tokenBefore, preserved, "rotation must preserve token identity and configuration")

	secondReplayView, secondReplaySecret, secondReplayed, err := CommitTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, secondCommand, fixture.now)
	require.NoError(t, err)
	assert.True(t, secondReplayed)
	assert.Empty(t, secondReplaySecret)
	assert.Equal(t, secondView, secondReplayView)

	secondReceiptView, err := ReadTokenCredentialRotationOperation(ctx, db, fixture.root.Id, fixture.token.Id, secondCommand.OperationID)
	require.NoError(t, err)
	assert.Equal(t, secondView, secondReceiptView)
	var receiptBeforeImmutableChecks TokenCredentialRotationReceipt
	require.NoError(t, db.Where("operation_id = ?", command.OperationID).Take(&receiptBeforeImmutableChecks).Error)
	updateErr := db.Model(&TokenCredentialRotationReceipt{}).
		Where("operation_id = ?", command.OperationID).
		Update("masked_key", "mutated").Error
	require.ErrorIs(t, updateErr, ErrAccountQuotaReceiptImmutable)
	deleteErr := db.Where("operation_id = ?", command.OperationID).Delete(&TokenCredentialRotationReceipt{}).Error
	require.ErrorIs(t, deleteErr, ErrAccountQuotaReceiptImmutable)
	for _, mutation := range []struct {
		name string
		run  func() error
	}{
		{
			name: "model update column",
			run: func() error {
				return db.Model(&TokenCredentialRotationReceipt{}).Where("operation_id = ?", command.OperationID).
					UpdateColumn("masked_key", "mutated").Error
			},
		},
		{
			name: "table update column",
			run: func() error {
				return db.Table("token_credential_rotation_receipts").Where("operation_id = ?", command.OperationID).
					UpdateColumn("masked_key", "mutated").Error
			},
		},
		{
			name: "aliased table update column",
			run: func() error {
				return db.Table("token_credential_rotation_receipts AS rotation_receipts").
					Where("rotation_receipts.operation_id = ?", command.OperationID).
					UpdateColumn("masked_key", "mutated").Error
			},
		},
		{
			name: "table delete",
			run: func() error {
				return db.Table("token_credential_rotation_receipts").Where("operation_id = ?", command.OperationID).Delete(nil).Error
			},
		},
		{
			name: "aliased table delete",
			run: func() error {
				return db.Table("token_credential_rotation_receipts AS rotation_receipts").
					Where("rotation_receipts.operation_id = ?", command.OperationID).Delete(nil).Error
			},
		},
	} {
		t.Run("immutable receipt "+mutation.name, func(t *testing.T) {
			require.ErrorIs(t, mutation.run(), ErrAccountQuotaReceiptImmutable)
		})
	}
	var receiptAfterImmutableChecks TokenCredentialRotationReceipt
	require.NoError(t, db.Where("operation_id = ?", command.OperationID).Take(&receiptAfterImmutableChecks).Error)
	assert.Equal(t, receiptBeforeImmutableChecks, receiptAfterImmutableChecks)

	var receiptCount int64
	require.NoError(t, db.Model(&TokenCredentialRotationReceipt{}).Where("token_id = ?", fixture.token.Id).Count(&receiptCount).Error)
	assert.Equal(t, int64(2), receiptCount)
}

func TestTokenCredentialRotationAdmissionAndCommitSerialize(t *testing.T) {
	db := newTokenCredentialRotationSQLiteDB(t)
	fixture := newTokenCredentialRotationFixture(t, db, "barrier")
	assertTokenCredentialRotationAdmissionAndCommitSerialize(t, fixture)
}

func assertTokenCredentialRotationAdmissionAndCommitSerialize(t *testing.T, fixture tokenCredentialRotationFixture) {
	t.Helper()
	db := fixture.db
	ctx := context.Background()
	previousRedisEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = previousRedisEnabled, previousRDB })
	preview, err := PreviewTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, fixture.now)
	require.NoError(t, err)
	require.Equal(t, TokenMaintenanceStatusReady, preview.Status)

	type beginResult struct {
		admission *TokenCredentialDispatchAdmission
		err       error
	}
	type commitResult struct {
		view *TokenCredentialRotationOperationView
		err  error
	}
	start := make(chan struct{})
	beginDone := make(chan beginResult, 1)
	commitDone := make(chan commitResult, 1)
	go func() {
		<-start
		admission, beginErr := BeginTokenCredentialDispatchAdmission(ctx, db, fixture.root.Id, fixture.token.Id, 1, "request-barrier", TokenCredentialDispatchAdmissionKindLegacyTask)
		beginDone <- beginResult{admission: admission, err: beginErr}
	}()
	go func() {
		<-start
		view, _, _, commitErr := CommitTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, TokenCredentialRotationCommand{
			OperationID: strings.Repeat("d", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
		}, fixture.now)
		commitDone <- commitResult{view: view, err: commitErr}
	}()
	close(start)
	begin := <-beginDone
	commit := <-commitDone

	switch {
	case begin.err == nil:
		require.NotNil(t, begin.admission)
		require.ErrorIs(t, commit.err, ErrTokenMaintenancePreconditionUnknown)
		var token Token
		require.NoError(t, db.First(&token, fixture.token.Id).Error)
		assert.Equal(t, int64(1), token.CredentialVersion)
	case commit.err == nil:
		require.NotNil(t, commit.view)
		require.ErrorIs(t, begin.err, ErrTokenCredentialVersionChanged)
		var count int64
		require.NoError(t, db.Model(&TokenCredentialDispatchAdmission{}).Where("token_id = ?", fixture.token.Id).Count(&count).Error)
		assert.Zero(t, count)
	default:
		require.NoError(t, begin.err)
		require.NoError(t, commit.err)
	}
}

func assertTokenCredentialRotationLegacySchemaUpgrade(t *testing.T, db *gorm.DB, namespace string) {
	t.Helper()
	require.NoError(t, db.Migrator().DropColumn(&Token{}, "CredentialVersion"))
	require.False(t, db.Migrator().HasColumn(&Token{}, "CredentialVersion"))
	owner := User{
		Username: "rotation-legacy-" + namespace,
		AffCode:  "rotlegacy" + namespace,
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(&owner).Error)
	const legacyTokenID = 720001
	legacyKey := "rotation-legacy-key-" + namespace
	legacyName := "legacy rotation metadata " + namespace
	legacyAllowIPs := "192.0.2.10"
	legacyAutoGroups := `["priority","default"]`
	require.NoError(t, db.Model(&Token{}).Create(map[string]any{
		"id": legacyTokenID, "user_id": owner.Id, "key": legacyKey,
		"status": common.TokenStatusEnabled, "name": legacyName,
		"created_time": int64(101), "accessed_time": int64(202), "expired_time": int64(303),
		"remain_quota": int64(4_000_000_004), "quota_version": int64(17), "used_quota": int64(500_000_005),
		"unlimited_quota": true, "model_limits_enabled": true, "model_limits": "gpt-4.1,gpt-5",
		"allow_ips": legacyAllowIPs, "group": "auto", "access_profile_id": "legacy-profile",
		"cross_group_retry": true, "auto_groups": legacyAutoGroups,
	}).Error)

	require.NoError(t, db.AutoMigrate(&Token{}))
	require.True(t, db.Migrator().HasColumn(&Token{}, "CredentialVersion"))
	var migrated Token
	require.NoError(t, db.First(&migrated, legacyTokenID).Error)
	assert.Equal(t, legacyTokenID, migrated.Id)
	assert.Equal(t, owner.Id, migrated.UserId)
	assert.Equal(t, legacyKey, migrated.Key)
	assert.Equal(t, int64(1), migrated.CredentialVersion)
	assert.Equal(t, common.TokenStatusEnabled, migrated.Status)
	assert.Equal(t, legacyName, migrated.Name)
	assert.Equal(t, int64(101), migrated.CreatedTime)
	assert.Equal(t, int64(202), migrated.AccessedTime)
	assert.Equal(t, int64(303), migrated.ExpiredTime)
	assert.Equal(t, int64(4_000_000_004), migrated.RemainQuota)
	assert.Equal(t, int64(17), migrated.QuotaVersion)
	assert.Equal(t, int64(500_000_005), migrated.UsedQuota)
	assert.True(t, migrated.UnlimitedQuota)
	assert.True(t, migrated.ModelLimitsEnabled)
	assert.Equal(t, "gpt-4.1,gpt-5", migrated.ModelLimits)
	require.NotNil(t, migrated.AllowIps)
	assert.Equal(t, legacyAllowIPs, *migrated.AllowIps)
	assert.Equal(t, "auto", migrated.Group)
	assert.Equal(t, "legacy-profile", migrated.AccessProfileID)
	assert.True(t, migrated.CrossGroupRetry)
	assert.Equal(t, legacyAutoGroups, migrated.AutoGroups)
}

func TestTokenCredentialRotationConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/token-credential-rotation.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
			if engine.env != "" {
				if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
					t.Skip("requires explicitly configured disposable B2 database")
				}
				dsn := strings.TrimSpace(os.Getenv(engine.env))
				require.NotEmpty(t, dsn)
				var err error
				dialector, err = b2SubmissionDatabaseDialector(engine.name, dsn)
				require.NoError(t, err)
			}
			prefix := fmt.Sprintf("tcr_%x_", time.Now().UnixNano())
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), NamingStrategy: schema.NamingStrategy{TablePrefix: prefix}})
			require.NoError(t, err)
			require.NoError(t, registerTaskRecoveryGormGuards(db))
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Ping())
			entities := tokenCredentialRotationTestEntities()
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(entities...))
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(entities...))
			require.NoError(t, db.AutoMigrate(entities...), "migration must be repeatable")
			assertTokenCredentialRotationLegacySchemaUpgrade(t, db, engine.name)

			fixture := newTokenCredentialRotationFixture(t, db, engine.name)
			admission, err := BeginTokenCredentialDispatchAdmission(context.Background(), db, fixture.root.Id, fixture.token.Id, 1, "configured-"+engine.name, TokenCredentialDispatchAdmissionKindLegacyTask)
			require.NoError(t, err)
			var openAdmission TokenCredentialDispatchAdmission
			require.NoError(t, db.Where("admission_id = ?", admission.AdmissionID).Take(&openAdmission).Error)
			require.Empty(t, openAdmission.LinkedIdentityHash, "an open admission must round-trip its optional link identity without fixed-width padding")
			require.NoError(t, validateTokenCredentialDispatchAdmission(openAdmission))
			task := Task{TaskID: "configured-task-" + engine.name, UserId: fixture.root.Id, Status: TaskStatusSuccess, Progress: "100%", PrivateData: TaskPrivateData{TokenId: fixture.token.Id}}
			require.NoError(t, db.Create(&task).Error)
			require.NoError(t, CompleteTokenCredentialDispatchAdmission(context.Background(), db, fixture.root.Id, fixture.token.Id, admission.AdmissionID, task.ID))
			preview, err := PreviewTokenCredentialRotation(context.Background(), db, fixture.root.Id, fixture.token.Id, fixture.now)
			require.NoError(t, err)
			require.Equal(t, TokenMaintenanceStatusReady, preview.Status)
			assert.NotEmpty(t, preview.PreviewRevision)
			assertTokenCredentialRotationCommitContract(t, fixture)

			barrierFixture := newTokenCredentialRotationFixture(t, db, engine.name+"-barrier")
			assertTokenCredentialRotationAdmissionAndCommitSerialize(t, barrierFixture)

			if engine.name == "mysql" {
				foundRowsConfig, err := mysqldriver.ParseDSN(strings.TrimSpace(os.Getenv(engine.env)))
				require.NoError(t, err)
				foundRowsConfig.ClientFoundRows = true
				foundRowsDialector, err := b2SubmissionDatabaseDialector(engine.name, foundRowsConfig.FormatDSN())
				require.NoError(t, err)
				foundRowsDB, err := gorm.Open(foundRowsDialector, &gorm.Config{
					Logger: logger.Default.LogMode(logger.Silent), NamingStrategy: schema.NamingStrategy{TablePrefix: prefix},
				})
				require.NoError(t, err)
				require.NoError(t, registerTaskRecoveryGormGuards(foundRowsDB))
				foundRowsSQLDB, err := foundRowsDB.DB()
				require.NoError(t, err)
				require.NoError(t, foundRowsSQLDB.Ping())
				t.Cleanup(func() { require.NoError(t, foundRowsSQLDB.Close()) })

				foundRowsFixture := newTokenCredentialRotationFixture(t, foundRowsDB, engine.name+"-found-rows")
				first, err := BeginTokenCredentialDispatchAdmission(context.Background(), foundRowsDB, foundRowsFixture.root.Id, foundRowsFixture.token.Id, 1, "configured-mysql-found-rows", TokenCredentialDispatchAdmissionKindLegacyTask)
				require.NoError(t, err)
				replayed, err := BeginTokenCredentialDispatchAdmission(context.Background(), foundRowsDB, foundRowsFixture.root.Id, foundRowsFixture.token.Id, 1, "configured-mysql-found-rows", TokenCredentialDispatchAdmissionKindLegacyTask)
				require.ErrorIs(t, err, ErrTokenMaintenancePreconditionUnknown)
				require.NotNil(t, replayed)
				assert.Equal(t, first.AdmissionID, replayed.AdmissionID)
				var admissionCount int64
				require.NoError(t, foundRowsDB.Model(&TokenCredentialDispatchAdmission{}).
					Where("token_id = ?", foundRowsFixture.token.Id).Count(&admissionCount).Error)
				assert.Equal(t, int64(1), admissionCount, "clientFoundRows must not authorize or persist a second send admission")
			}
		})
	}
}

type tokenCredentialRotationRejectNthEvalHook struct {
	count  atomic.Int32
	failAt int32
	err    error
}

func (hook *tokenCredentialRotationRejectNthEvalHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if cmd.Name() == "eval" && hook.count.Add(1) == hook.failAt {
		return ctx, hook.err
	}
	return ctx, nil
}

func (*tokenCredentialRotationRejectNthEvalHook) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (*tokenCredentialRotationRejectNthEvalHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (*tokenCredentialRotationRejectNthEvalHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func TestTokenCredentialRotationConfiguredRedis(t *testing.T) {
	if os.Getenv("MYAPI_S2C_REDIS_TESTS") != "1" {
		t.Skip("disposable Redis tests require MYAPI_S2C_REDIS_TESTS=1")
	}
	options, err := s2cRedisFixtureOptions(os.Getenv("MYAPI_S2C_REDIS_ADDR"))
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx := context.Background()
	serverInfo, err := client.Info(ctx, "server").Result()
	require.NoError(t, err)
	var redisVersion string
	for _, line := range strings.Split(serverInfo, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "redis_version:") {
			redisVersion = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "redis_version:"))
			break
		}
	}
	require.NotEmpty(t, redisVersion)
	major, err := strconv.Atoi(strings.SplitN(redisVersion, ".", 2)[0])
	require.NoError(t, err)
	require.Equal(t, 7, major, "credential rotation cache contract requires Redis 7")
	keyspace, err := client.Info(ctx, "keyspace").Result()
	require.NoError(t, err)
	for _, line := range strings.Split(keyspace, "\n") {
		require.False(t, strings.HasPrefix(strings.TrimSpace(line), "db"), "refusing a non-empty Redis instance")
	}
	count, err := client.DBSize(ctx).Result()
	require.NoError(t, err)
	require.Zero(t, count)
	runTokenCredentialRotationRedisContract(t, client)
}

func TestTokenCredentialRotationMiniRedis(t *testing.T) {
	server := miniredis.RunT(t)
	server.SetTime(time.Unix(1_700_000_000, 0))
	options, err := s2cRedisFixtureOptions(server.Addr())
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	runTokenCredentialRotationRedisContract(t, client)
}

func runTokenCredentialRotationRedisContract(t *testing.T, client *redis.Client) {
	t.Helper()
	previousRedisEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() { common.RedisEnabled, common.RDB = previousRedisEnabled, previousRDB })

	t.Run("old and new credentials are fenced", func(t *testing.T) {
		db := newTokenCredentialRotationSQLiteDB(t)
		fixture := newTokenCredentialRotationFixture(t, db, "redis-success")
		ctx := context.Background()
		oldCacheKey := getTokenCacheKey(fixture.token.Key)
		require.NoError(t, client.HSet(ctx, oldCacheKey, "Id", fixture.token.Id, "UserId", fixture.root.Id).Err())
		preview, err := PreviewTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, fixture.now)
		require.NoError(t, err)
		require.Equal(t, TokenMaintenanceStatusReady, preview.Status)
		command := TokenCredentialRotationCommand{
			OperationID: strings.Repeat("1", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
		}
		view, rawKey, replayed, err := CommitTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, command, fixture.now)
		require.NoError(t, err)
		require.NotNil(t, view)
		assert.False(t, replayed)
		assert.Len(t, rawKey, 48)
		newCacheKey := getTokenCacheKey(rawKey)
		assert.NotEqual(t, oldCacheKey, newCacheKey)
		oldFence, err := client.Get(ctx, getTokenCacheFenceKey(fixture.token.Key)).Result()
		require.NoError(t, err)
		newFence, err := client.Get(ctx, getTokenCacheFenceKey(rawKey)).Result()
		require.NoError(t, err)
		assert.NotEmpty(t, oldFence)
		assert.Equal(t, oldFence, newFence)
		assert.Greater(t, client.TTL(ctx, getTokenCacheFenceKey(fixture.token.Key)).Val(), time.Duration(0))
		assert.Greater(t, client.TTL(ctx, getTokenCacheFenceKey(rawKey)).Val(), time.Duration(0))
		assert.Zero(t, client.Exists(ctx, oldCacheKey, newCacheKey).Val())
		lockKey, lockErr := quotaBalanceSubjectLockKey(BatchUpdateTypeTokenQuota, fixture.token.Id)
		require.NoError(t, lockErr)
		assert.Zero(t, client.Exists(ctx, lockKey).Val())

		replayView, replaySecret, replayed, err := CommitTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, command, fixture.now)
		require.NoError(t, err)
		assert.True(t, replayed)
		assert.Empty(t, replaySecret)
		assert.Equal(t, view, replayView)
	})

	t.Run("subject lock failure writes nothing", func(t *testing.T) {
		db := newTokenCredentialRotationSQLiteDB(t)
		fixture := newTokenCredentialRotationFixture(t, db, "redis-lock")
		ctx := context.Background()
		preview, err := PreviewTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, fixture.now)
		require.NoError(t, err)
		require.Equal(t, TokenMaintenanceStatusReady, preview.Status)
		lockKey, err := quotaBalanceSubjectLockKey(BatchUpdateTypeTokenQuota, fixture.token.Id)
		require.NoError(t, err)
		require.NoError(t, client.Set(ctx, lockKey, "other-owner", time.Minute).Err())
		t.Cleanup(func() { _ = client.Del(context.Background(), lockKey).Err() })
		commitCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		_, rawKey, replayed, err := CommitTokenCredentialRotation(commitCtx, db, fixture.root.Id, fixture.token.Id, TokenCredentialRotationCommand{
			OperationID: strings.Repeat("2", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
		}, fixture.now)
		require.ErrorIs(t, err, ErrTokenMaintenancePreconditionUnknown)
		assert.Empty(t, rawKey)
		assert.False(t, replayed)
		var stored Token
		require.NoError(t, db.First(&stored, fixture.token.Id).Error)
		assert.Equal(t, fixture.token.Key, stored.Key)
		assert.Equal(t, int64(1), stored.CredentialVersion)
		var receipts int64
		require.NoError(t, db.Model(&TokenCredentialRotationReceipt{}).Where("token_id = ?", fixture.token.Id).Count(&receipts).Error)
		assert.Zero(t, receipts)
		require.NoError(t, client.Del(ctx, lockKey).Err())
	})

	t.Run("pre-commit cache failure writes nothing", func(t *testing.T) {
		db := newTokenCredentialRotationSQLiteDB(t)
		fixture := newTokenCredentialRotationFixture(t, db, "redis-precommit")
		ctx := context.Background()
		oldCacheKey := getTokenCacheKey(fixture.token.Key)
		require.NoError(t, client.HSet(ctx, oldCacheKey, "Id", fixture.token.Id, "UserId", fixture.root.Id).Err())
		preview, err := PreviewTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, fixture.now)
		require.NoError(t, err)
		require.Equal(t, TokenMaintenanceStatusReady, preview.Status)
		hook := &tokenCredentialRotationRejectNthEvalHook{failAt: 1, err: errors.New("synthetic pre-commit credential cache failure")}
		client.AddHook(hook)
		_, rawKey, replayed, err := CommitTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, TokenCredentialRotationCommand{
			OperationID: strings.Repeat("4", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
		}, fixture.now)
		require.ErrorIs(t, err, ErrTokenMaintenancePreconditionUnknown)
		assert.Empty(t, rawKey)
		assert.False(t, replayed)
		assert.Equal(t, int32(2), hook.count.Load(), "failed pre-commit invalidation and lock release")
		var stored Token
		require.NoError(t, db.First(&stored, fixture.token.Id).Error)
		assert.Equal(t, fixture.token.Key, stored.Key)
		assert.Equal(t, int64(1), stored.CredentialVersion)
		var receipts int64
		require.NoError(t, db.Model(&TokenCredentialRotationReceipt{}).Where("token_id = ?", fixture.token.Id).Count(&receipts).Error)
		assert.Zero(t, receipts)
		assert.Equal(t, int64(1), client.Exists(ctx, oldCacheKey).Val())
		lockKey, lockErr := quotaBalanceSubjectLockKey(BatchUpdateTypeTokenQuota, fixture.token.Id)
		require.NoError(t, lockErr)
		assert.Zero(t, client.Exists(ctx, lockKey).Val())
	})

	t.Run("post-commit cache failure has unknown outcome and secret-free recovery", func(t *testing.T) {
		db := newTokenCredentialRotationSQLiteDB(t)
		fixture := newTokenCredentialRotationFixture(t, db, "redis-postcommit")
		ctx := context.Background()
		oldCacheKey := getTokenCacheKey(fixture.token.Key)
		require.NoError(t, client.HSet(ctx, oldCacheKey, "Id", fixture.token.Id, "UserId", fixture.root.Id).Err())
		preview, err := PreviewTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, fixture.now)
		require.NoError(t, err)
		require.Equal(t, TokenMaintenanceStatusReady, preview.Status)
		command := TokenCredentialRotationCommand{
			OperationID: strings.Repeat("3", 64), PreviewRevision: preview.PreviewRevision, Confirmed: true,
		}
		hook := &tokenCredentialRotationRejectNthEvalHook{failAt: 2, err: errors.New("synthetic post-commit credential cache failure")}
		client.AddHook(hook)
		view, rawKey, replayed, err := CommitTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, command, fixture.now)
		require.ErrorIs(t, err, ErrTokenCredentialRotationOutcomeUnknown)
		assert.Nil(t, view)
		assert.Empty(t, rawKey)
		assert.False(t, replayed)
		assert.Equal(t, int32(3), hook.count.Load(), "pre-commit invalidation, failed post-commit invalidation, and lock release")

		var stored Token
		require.NoError(t, db.First(&stored, fixture.token.Id).Error)
		assert.NotEqual(t, fixture.token.Key, stored.Key)
		assert.Equal(t, int64(2), stored.CredentialVersion)
		var receipt TokenCredentialRotationReceipt
		require.NoError(t, db.Where("operation_id = ?", command.OperationID).Take(&receipt).Error)
		assert.NotContains(t, fmt.Sprintf("%+v", receipt), stored.Key)
		receiptView, err := ReadTokenCredentialRotationOperation(ctx, db, fixture.root.Id, fixture.token.Id, command.OperationID)
		require.NoError(t, err)
		assert.Equal(t, stored.Id, receiptView.TokenID)
		assert.Equal(t, int64(2), receiptView.CredentialVersionAfter)
		replayView, replaySecret, replayed, err := CommitTokenCredentialRotation(ctx, db, fixture.root.Id, fixture.token.Id, command, fixture.now)
		require.NoError(t, err)
		assert.True(t, replayed)
		assert.Empty(t, replaySecret)
		assert.Equal(t, receiptView, replayView)
	})
}
