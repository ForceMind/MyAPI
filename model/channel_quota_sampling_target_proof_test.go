package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

type legacyChannelQuotaSamplingTargetProof struct {
	ID                  int64  `gorm:"primaryKey"`
	ChannelID           int    `gorm:"not null;uniqueIndex:uidx_channel_quota_sampling_target,priority:1"`
	SubjectRef          string `gorm:"type:char(43);not null;uniqueIndex:uidx_channel_quota_sampling_target,priority:2"`
	IdentityQuality     string `gorm:"type:varchar(24);not null"`
	ConfirmedSubjectRef string `gorm:"type:char(43);not null;default:''"`
	Active              bool   `gorm:"not null"`
	LastAttemptAt       int64  `gorm:"type:bigint;not null;default:0"`
	LastResult          string `gorm:"type:varchar(24);not null;default:''"`
	CreatedAt           int64  `gorm:"type:bigint;not null"`
	UpdatedAt           int64  `gorm:"type:bigint;not null"`
}

func TestSamplingTargetProofCannotBeBorrowedAfterReactivation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSamplingTarget{}, &ChannelQuotaSnapshot{}))
	require.NoError(t, db.Create(&Channel{Id: 71, Key: "fixture"}).Error)
	credentialA, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	credentialB, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	account, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)

	for _, credential := range []string{credentialA, credentialB} {
		require.NoError(t, EnsureChannelQuotaSamplingTarget(t.Context(), db, 71, ChannelQuotaSamplingIdentity{
			SubjectRef: credential, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped,
		}))
	}
	var targetA ChannelQuotaSamplingTarget
	require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", 71, credentialA).First(&targetA).Error)
	require.Equal(t, int64(1), targetA.EvidenceVersion)
	oldSnapshot := samplingTargetProofSnapshot(71, account, "sample-a", time.Now().Add(-time.Hour).Unix())
	require.NoError(t, db.Create(&oldSnapshot).Error)
	require.NoError(t, MarkChannelQuotaSamplingTargetAttempt(t.Context(), db, 71, credentialA, "sampled", account,
		ChannelQuotaSamplingProof{ExpectedVersion: targetA.EvidenceVersion, SampleID: oldSnapshot.SampleID}))

	_, err = SyncChannelQuotaSamplingTargets(t.Context(), db, 71, "fixture", true, []ChannelQuotaSamplingIdentity{{
		SubjectRef: credentialB, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped,
	}})
	require.NoError(t, err)
	var targetB ChannelQuotaSamplingTarget
	require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", 71, credentialB).First(&targetB).Error)
	freshSnapshot := samplingTargetProofSnapshot(71, account, "sample-b", time.Now().Unix())
	require.NoError(t, db.Create(&freshSnapshot).Error)
	require.NoError(t, MarkChannelQuotaSamplingTargetAttempt(t.Context(), db, 71, credentialB, "sampled", account,
		ChannelQuotaSamplingProof{ExpectedVersion: targetB.EvidenceVersion, SampleID: freshSnapshot.SampleID}))

	targets, err := SyncChannelQuotaSamplingTargets(t.Context(), db, 71, "fixture", true, []ChannelQuotaSamplingIdentity{
		{SubjectRef: credentialA, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped},
		{SubjectRef: credentialB, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped},
	})
	require.NoError(t, err)
	require.Len(t, targets, 2)
	require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", 71, credentialA).First(&targetA).Error)
	assert.Equal(t, int64(2), targetA.EvidenceVersion)
	assert.Zero(t, targetA.VerifiedSnapshotID)
	assert.Equal(t, account, targetA.ConfirmedSubjectRef, "reactivation may retain historical identity without retaining current proof")

	require.NoError(t, MarkChannelQuotaSamplingTargetAttempt(t.Context(), db, 71, credentialA, "sampled", account))
	require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", 71, credentialA).First(&targetA).Error)
	assert.Zero(t, targetA.VerifiedSnapshotID, "a legacy mark cannot borrow the other target's fresh snapshot")
}

func TestSamplingTargetProofRequiresExactVersionAndNativeSnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSamplingTarget{}, &ChannelQuotaSnapshot{}))
	require.NoError(t, db.Create(&Channel{Id: 72, Key: "fixture"}).Error)
	credential, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	account, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	require.NoError(t, EnsureChannelQuotaSamplingTarget(t.Context(), db, 72, ChannelQuotaSamplingIdentity{
		SubjectRef: credential, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped,
	}))
	var target ChannelQuotaSamplingTarget
	require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", 72, credential).First(&target).Error)

	wrongChannel := samplingTargetProofSnapshot(73, account, "shared-sample", time.Now().Unix())
	require.NoError(t, db.Create(&wrongChannel).Error)
	err = MarkChannelQuotaSamplingTargetAttempt(t.Context(), db, 72, credential, "sampled", account,
		ChannelQuotaSamplingProof{ExpectedVersion: target.EvidenceVersion, SampleID: wrongChannel.SampleID})
	assert.ErrorIs(t, err, ErrChannelQuotaSamplingProofInvalid)

	verified := samplingTargetProofSnapshot(72, account, "verified-sample", time.Now().Unix())
	require.NoError(t, db.Create(&verified).Error)
	require.NoError(t, MarkChannelQuotaSamplingTargetAttempt(t.Context(), db, 72, credential, "sampled", account,
		ChannelQuotaSamplingProof{ExpectedVersion: target.EvidenceVersion, SampleID: verified.SampleID}))
	require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", 72, credential).First(&target).Error)
	assert.Equal(t, verified.Id, target.VerifiedSnapshotID)
	require.NoError(t, MarkChannelQuotaSamplingTargetAttempt(t.Context(), db, 72, credential, "busy", ""))
	require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", 72, credential).First(&target).Error)
	assert.Zero(t, target.VerifiedSnapshotID)

	nonNative := samplingTargetProofSnapshot(72, account, "non-native", time.Now().Unix())
	nonNative.Source = "compatible_proxy"
	require.NoError(t, db.Create(&nonNative).Error)
	err = MarkChannelQuotaSamplingTargetAttempt(t.Context(), db, 72, credential, "sampled", account,
		ChannelQuotaSamplingProof{ExpectedVersion: target.EvidenceVersion, SampleID: nonNative.SampleID})
	assert.ErrorIs(t, err, ErrChannelQuotaSamplingProofInvalid)
	require.NoError(t, MarkChannelQuotaSamplingTargetAttempt(t.Context(), db, 72, credential, "sampled", account,
		ChannelQuotaSamplingProof{ExpectedVersion: target.EvidenceVersion, SampleID: verified.SampleID}))

	_, err = SyncChannelQuotaSamplingTargets(t.Context(), db, 72, "fixture", true, nil)
	require.NoError(t, err)
	_, err = SyncChannelQuotaSamplingTargets(t.Context(), db, 72, "fixture", true, []ChannelQuotaSamplingIdentity{{
		SubjectRef: credential, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped,
	}})
	require.NoError(t, err)
	err = MarkChannelQuotaSamplingTargetAttempt(t.Context(), db, 72, credential, "sampled", account,
		ChannelQuotaSamplingProof{ExpectedVersion: 1, SampleID: verified.SampleID})
	assert.ErrorIs(t, err, ErrChannelQuotaSamplingProofStale)
	require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", 72, credential).First(&target).Error)
	assert.Equal(t, int64(2), target.EvidenceVersion)
	assert.Zero(t, target.VerifiedSnapshotID)
}

func TestEnsureSamplingTargetUpgradesLegacyProofState(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSamplingTarget{}))
	credential, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	require.NoError(t, db.Create(&ChannelQuotaSamplingTarget{
		ChannelID: 73, SubjectRef: credential, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped,
		Active: true, EvidenceVersion: 0, VerifiedSnapshotID: 99, CreatedAt: 1, UpdatedAt: 1,
	}).Error)

	require.NoError(t, EnsureChannelQuotaSamplingTarget(t.Context(), db, 73, ChannelQuotaSamplingIdentity{
		SubjectRef: credential, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped,
	}))
	var target ChannelQuotaSamplingTarget
	require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", 73, credential).Take(&target).Error)
	assert.Equal(t, int64(1), target.EvidenceVersion)
	assert.Zero(t, target.VerifiedSnapshotID)
}

func TestSamplingTargetProofConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{
		{name: "sqlite"},
		{name: "mysql", env: "MYAPI_B2_MYSQL_DSN"},
		{name: "postgres", env: "MYAPI_B2_POSTGRES_DSN"},
	} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(filepath.Join(t.TempDir(), "sampling-proof.db"))
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
			prefix := fmt.Sprintf("sp_%x_", time.Now().UnixNano())
			db, err := gorm.Open(dialector, &gorm.Config{
				Logger: logger.Default.LogMode(logger.Silent), NamingStrategy: schema.NamingStrategy{TablePrefix: prefix},
			})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Ping())
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&ChannelQuotaSamplingTarget{}, &ChannelQuotaSnapshot{}, &Channel{}))
				require.NoError(t, sqlDB.Close())
			})

			require.NoError(t, db.Migrator().DropTable(&ChannelQuotaSamplingTarget{}))
			require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSnapshot{}))
			channel := Channel{Name: "sampling-proof-" + engine.name, Type: 57, Key: "fixture"}
			require.NoError(t, db.Create(&channel).Error)
			credential, err := newChannelQuotaIdentitySubjectRef()
			require.NoError(t, err)
			account, err := newChannelQuotaIdentitySubjectRef()
			require.NoError(t, err)
			legacy := legacyChannelQuotaSamplingTargetProof{
				ChannelID: channel.Id, SubjectRef: credential, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped,
				ConfirmedSubjectRef: account, Active: true, LastResult: "sampled", CreatedAt: 1, UpdatedAt: 1,
			}
			require.NoError(t, db.Table("channel_quota_sampling_targets").AutoMigrate(&legacyChannelQuotaSamplingTargetProof{}))
			require.NoError(t, db.Table("channel_quota_sampling_targets").Create(&legacy).Error)

			require.NoError(t, db.AutoMigrate(&ChannelQuotaSamplingTarget{}))
			require.NoError(t, db.AutoMigrate(&ChannelQuotaSamplingTarget{}), "sampling target migration must be repeatable")
			require.True(t, db.Migrator().HasColumn(&ChannelQuotaSamplingTarget{}, "evidence_version"))
			require.True(t, db.Migrator().HasColumn(&ChannelQuotaSamplingTarget{}, "verified_snapshot_id"))
			var target ChannelQuotaSamplingTarget
			require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", channel.Id, credential).Take(&target).Error)
			require.Zero(t, target.EvidenceVersion)
			require.Zero(t, target.VerifiedSnapshotID)

			require.NoError(t, EnsureChannelQuotaSamplingTarget(t.Context(), db, channel.Id, ChannelQuotaSamplingIdentity{
				SubjectRef: credential, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped,
			}))
			require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", channel.Id, credential).Take(&target).Error)
			require.Equal(t, int64(1), target.EvidenceVersion)
			snapshot := samplingTargetProofSnapshot(channel.Id, account, "configured-proof", time.Now().Unix())
			require.NoError(t, db.Create(&snapshot).Error)
			require.NoError(t, MarkChannelQuotaSamplingTargetAttempt(t.Context(), db, channel.Id, credential, "sampled", account,
				ChannelQuotaSamplingProof{ExpectedVersion: target.EvidenceVersion, SampleID: snapshot.SampleID}))
			require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", channel.Id, credential).Take(&target).Error)
			require.Equal(t, snapshot.Id, target.VerifiedSnapshotID)

			_, err = SyncChannelQuotaSamplingTargets(t.Context(), db, channel.Id, channel.Key, true, nil)
			require.NoError(t, err)
			_, err = SyncChannelQuotaSamplingTargets(t.Context(), db, channel.Id, channel.Key, true, []ChannelQuotaSamplingIdentity{{
				SubjectRef: credential, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped,
			}})
			require.NoError(t, err)
			require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", channel.Id, credential).Take(&target).Error)
			require.Equal(t, int64(2), target.EvidenceVersion)
			require.Zero(t, target.VerifiedSnapshotID)
			err = MarkChannelQuotaSamplingTargetAttempt(t.Context(), db, channel.Id, credential, "sampled", account,
				ChannelQuotaSamplingProof{ExpectedVersion: 1, SampleID: snapshot.SampleID})
			require.ErrorIs(t, err, ErrChannelQuotaSamplingProofStale)
		})
	}
}

func samplingTargetProofSnapshot(channelID int, subjectRef, sampleID string, observedAt int64) ChannelQuotaSnapshot {
	total := 100.0
	used := 10.0
	return ChannelQuotaSnapshot{
		ChannelId: channelID, SubjectRef: subjectRef, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed,
		SampleID: sampleID, ObservedAt: observedAt, Available: 90, Used: &used, Total: &total, Unit: "percent",
		MetricType: "codex_rate_limit", WindowType: "five_hour", WindowSeconds: 18000, ResetAt: observedAt + 18000,
		Source: codexQuotaWindowPrimarySource, Status: "success", CodexObservationQualified: true, CodexThresholdQualified: true,
	}
}
